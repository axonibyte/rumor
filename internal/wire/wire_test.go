package wire

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"forge.axonibyte.com/axonibyte/rumor/internal/proof"
)

// wellFormed is one legal message per direction and kind, as a map so a test
// can DELETE a field or ADD one and watch the decoder refuse. Every value is
// plausible rather than minimal: an observe request's source decides whether
// its token is legal, so the fixtures have to satisfy the semantic rules too or
// the missing-field battery below would refuse for the wrong reason.
func wellFormed(direction string, kind Kind) map[string]any {
	env := map[string]any{"kind": string(kind), "contract": ContractVersion, "id": "7"}
	add := func(extra map[string]any) map[string]any {
		out := map[string]any{}
		for k, v := range env {
			out[k] = v
		}
		for k, v := range extra {
			out[k] = v
		}
		return out
	}
	if direction == "request" {
		switch kind {
		case KindDescribe:
			return add(nil)
		case KindExecute:
			return add(map[string]any{"action": "create_event", "args": map[string]any{"capacity": 2}, "token": "t"})
		case KindObserve:
			return add(map[string]any{"query": "event", "source": "api", "token": "t"})
		case KindClock:
			return add(map[string]any{"op": "advance", "t": 30000000000})
		case KindSession:
			return add(map[string]any{"actor": "alice", "context": "api"})
		case KindFlow:
			// THE STEPS ARE A REAL FLOW, not an empty list: a fixture whose
			// steps were `[]` would decode on a transport that dropped the
			// field entirely, and the battery below reads this one as its
			// well-formed control.
			return add(map[string]any{"action": "create_doc", "token": "t", "steps": []any{
				map[string]any{"step": "navigate", "to": "/docs/new"},
				map[string]any{"step": "wait", "until": "doc-created"},
			}})
		}
		return nil
	}
	switch kind {
	case KindDescribe:
		return add(map[string]any{
			"actions":      []any{map[string]any{"name": "create_event", "context": "api"}},
			"observations": []any{map[string]any{"name": "event", "source": "api"}},
			"contexts":     []any{"api"},
			"components":   []any{map[string]any{"name": "store", "stateful": true}},
			"capabilities": map[string]any{
				"faults":  []any{"restart"},
				"volume":  []any{},
				"upgrade": []any{},
			},
		})
	case KindExecute:
		return add(map[string]any{"ids": map[string]any{"event": 4711}, "response": map[string]any{"status": 201}})
	case KindObserve:
		return add(map[string]any{"value": 2})
	case KindClock:
		return add(map[string]any{"t": 30000000000})
	case KindSession:
		return add(map[string]any{"token": "t"})
	case KindFlow:
		return add(map[string]any{"reached": 2, "response": map[string]any{"title": "quarterly"}})
	case KindError:
		return add(map[string]any{"error": map[string]any{"kind": "source", "detail": "no browser here"}})
	}
	return nil
}

func line(t *testing.T, m map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshalling a fixture: %v", err)
	}
	return b
}

func decodeBy(direction string, b []byte) (Message, error) {
	if direction == "response" {
		return DecodeResponse(b)
	}
	return DecodeRequest(b)
}

// TestEveryWellFormedMessageDecodes is the control for the whole file: the
// battery below asserts refusals, and a decoder that refused EVERYTHING would
// pass every one of them.
func TestEveryWellFormedMessageDecodes(t *testing.T) {
	n := 0
	for _, direction := range []string{"request", "response"} {
		for _, kind := range Kinds(direction) {
			fixture := wellFormed(direction, kind)
			if fixture == nil {
				t.Fatalf("no fixture for the %s %s, so the battery below cannot cover it", kind, direction)
			}
			if _, err := decodeBy(direction, line(t, fixture)); err != nil {
				t.Errorf("%s %s: well-formed message refused: %v", kind, direction, err)
			}
			n++
		}
	}
	// 4 request kinds + 5 response kinds. A literal, because the point of the
	// number is to fail when a kind is ADDED to the protocol and not to this
	// file -- deriving it from Kinds() would make it agree with itself.
	if n != 13 {
		t.Errorf("drove %d message kinds, want 13: a kind was added or removed without this file moving", n)
	}
}

// TestMissingRequiredFieldIsRefusedByName deletes each required field in turn.
// It is the battery issue 62 asks for ("refuses an unknown or missing field by
// name"), and it is generated rather than listed so a field added to the schema
// is covered the moment it is added.
func TestMissingRequiredFieldIsRefusedByName(t *testing.T) {
	cases := 0
	for _, direction := range []string{"request", "response"} {
		for _, kind := range Kinds(direction) {
			required, _, ok := Schema(direction, kind)
			if !ok {
				t.Fatalf("no schema for the %s %s", kind, direction)
			}
			for _, field := range required {
				fixture := wellFormed(direction, kind)
				delete(fixture, field)
				_, err := decodeBy(direction, line(t, fixture))
				if err == nil {
					t.Errorf("%s %s without %q: decoded, want a refusal", kind, direction, field)
					continue
				}
				var ref Refusal
				if !asRefusal(err, &ref) {
					t.Errorf("%s %s without %q: %T, want a Refusal", kind, direction, field, err)
					continue
				}
				if ref.Field != field {
					t.Errorf("%s %s without %q: refusal names %q", kind, direction, field, ref.Field)
				}
				if !strings.Contains(ref.Reason, "missing field") {
					t.Errorf("%s %s without %q: reason %q does not say the field is missing", kind, direction, field, ref.Reason)
				}
				cases++
			}
		}
	}
	// VACUITY GUARD: 0 cases and 0 failures read identically. The floor is
	// derived from the schema itself, so it moves when the protocol does.
	floor := 0
	for _, direction := range []string{"request", "response"} {
		for _, kind := range Kinds(direction) {
			required, _, _ := Schema(direction, kind)
			floor += len(required)
		}
	}
	if cases != floor || cases == 0 {
		t.Errorf("drove %d deletions, want %d (one per required field, over every kind)", cases, floor)
	}
}

// TestUnknownFieldIsRefusedByName is the other half of issue 62's sentence, and
// the one the project's own coverage parser argues for: unknown is an error, not
// a skip, because a lenient reader turns `sorce` into an observation of nothing.
func TestUnknownFieldIsRefusedByName(t *testing.T) {
	cases := 0
	for _, direction := range []string{"request", "response"} {
		for _, kind := range Kinds(direction) {
			fixture := wellFormed(direction, kind)
			fixture["sorce"] = "api"
			_, err := decodeBy(direction, line(t, fixture))
			var ref Refusal
			if !asRefusal(err, &ref) {
				t.Errorf("%s %s with an unknown field: %v, want a Refusal", kind, direction, err)
				continue
			}
			if ref.Field != "sorce" || !strings.Contains(ref.Reason, "unknown field") {
				t.Errorf("%s %s with an unknown field: refusal is %+v", kind, direction, ref)
			}
			cases++
		}
	}
	// 11 -> 13 WITH R9.1's `flow`, one case per direction. The count is
	// declared rather than derived for the reason every count in this tree is:
	// a kind added without a fixture would otherwise widen the loop and the
	// battery would report more passes, not a gap.
	if cases != 13 {
		t.Errorf("drove %d kinds, want 13", cases)
	}
}

// TestObserveTokenFollowsTheSource drives both directions of the rule, which is
// the only way to tell a rule from a check that cannot fail the other way: a
// decoder that always demanded a token would pass the first two rows and fail
// the third.
func TestObserveTokenFollowsTheSource(t *testing.T) {
	for _, tc := range []struct {
		source  string
		token   string
		refused bool
	}{
		{"api", "t", false},
		{"browser", "t", false},
		{"direct", "", false},
		{"api", "", true},
		{"browser", "", true},
		{"direct", "t", true},
	} {
		fixture := wellFormed("request", KindObserve)
		fixture["source"] = tc.source
		if tc.token == "" {
			delete(fixture, "token")
		} else {
			fixture["token"] = tc.token
		}
		_, err := DecodeRequest(line(t, fixture))
		if tc.refused && err == nil {
			t.Errorf("source %q token %q: decoded, want a refusal", tc.source, tc.token)
		}
		if !tc.refused && err != nil {
			t.Errorf("source %q token %q: refused: %v", tc.source, tc.token, err)
		}
		if tc.refused && err != nil {
			var ref Refusal
			if asRefusal(err, &ref) && ref.Field != "token" {
				t.Errorf("source %q token %q: refusal names %q, want token", tc.source, tc.token, ref.Field)
			}
		}
	}
}

func TestSourceOutsideTheThreeIsRefusedWithItsValue(t *testing.T) {
	fixture := wellFormed("request", KindObserve)
	fixture["source"] = "db"
	_, err := DecodeRequest(line(t, fixture))
	var ref Refusal
	if !asRefusal(err, &ref) {
		t.Fatalf("source \"db\": %v, want a Refusal", err)
	}
	if ref.Field != "source" || !strings.Contains(ref.Reason, `"db"`) {
		t.Errorf("refusal is %+v; it must name the field and the value it saw", ref)
	}

	// AND IN describe's DECLARED OBSERVATIONS, which is a different code path
	// reading the same three words.
	resp := wellFormed("response", KindDescribe)
	resp["observations"] = []any{map[string]any{"name": "event", "source": "db"}}
	_, err = DecodeResponse(line(t, resp))
	if !asRefusal(err, &ref) {
		t.Fatalf("describe declaring source \"db\": %v, want a Refusal", err)
	}
	if !strings.Contains(ref.Reason, `"event"`) || !strings.Contains(ref.Reason, `"db"`) {
		t.Errorf("refusal %q must name the observation and the source", ref.Reason)
	}
}

// TestPageAndElementAreABrowserObservationsAlone holds the two rules fable's
// r9/120 attaches to the new fields: they say how a BROWSER reads a page, and an
// element is an ID in the same vocabulary as a flow's `until` -- "no CSS
// selectors; one word for one thing".
//
// THE TABLE AND THE STRUCT TAG ARE TWO INSTRUMENTS and this drives the table's
// side: `into` would accept every case below, because each field is a plain
// string on the struct. Only this pass can tell a confused adapter from a
// well-formed message.
func TestPageAndElementAreABrowserObservationsAlone(t *testing.T) {
	for _, c := range []struct {
		name string
		obs  map[string]any
		says string
	}{
		{"page on an api observation",
			map[string]any{"name": "event_count", "source": "api", "page": "/ui/x"},
			"how a BROWSER reads a page"},
		{"element on a direct observation",
			map[string]any{"name": "stored_count", "source": "direct", "element": "count"},
			"how a BROWSER reads a page"},
		{"an element that is a selector",
			map[string]any{"name": "page_count", "source": "browser",
				"page": "/ui/events/{subject}", "element": "#count .n"},
			"never a CSS selector"},
		{"an element carrying a dot",
			map[string]any{"name": "page_count", "source": "browser",
				"page": "/ui/events/{subject}", "element": "count.n"},
			"never a CSS selector"},
		{"a page carrying its own host",
			map[string]any{"name": "page_count", "source": "browser",
				"page": "http://127.0.0.1:8080/ui", "element": "count"},
			"second address"},
		{"type on an api observation",
			map[string]any{"name": "event_count", "source": "api", "type": "number"},
			"how a BROWSER reads a page"},
		{"a type outside the two words",
			map[string]any{"name": "page_count", "source": "browser",
				"page": "/ui/events/{subject}", "element": "count", "type": "integer"},
			"rather than by what the text looks like"},
		{"a type that is a Go type name",
			map[string]any{"name": "page_count", "source": "browser",
				"page": "/ui/events/{subject}", "element": "count", "type": "int"},
			"rather than by what the text looks like"},
	} {
		t.Run(c.name, func(t *testing.T) {
			resp := wellFormed("response", KindDescribe)
			resp["observations"] = []any{c.obs}
			_, err := DecodeResponse(line(t, resp))
			var ref Refusal
			if !asRefusal(err, &ref) {
				t.Fatalf("%s: %v, want a Refusal", c.name, err)
			}
			if ref.Field != "observations" {
				t.Errorf("the refusal names field %q, want observations", ref.Field)
			}
			if !strings.Contains(ref.Reason, c.says) {
				t.Errorf("the refusal %q does not say %q", ref.Reason, c.says)
			}
		})
	}
}

// AND A WELL-FORMED BROWSER OBSERVATION PASSES, which is the control: every case
// above would also be refused by a pass that refused the fields outright, and
// then the rules would be a ban rather than a shape.
func TestAReportedPageAndElementAreAccepted(t *testing.T) {
	resp := wellFormed("response", KindDescribe)
	resp["observations"] = []any{map[string]any{
		"name": "page_count", "source": "browser",
		"page": "/ui/events/{subject}", "element": "count", "type": "number",
	}}
	msg, err := DecodeResponse(line(t, resp))
	if err != nil {
		t.Fatalf("a well-formed browser observation was refused: %v", err)
	}
	got, ok := msg.(DescribeResponse)
	if !ok {
		t.Fatalf("decoded %T, want a DescribeResponse", msg)
	}
	if len(got.Observations) != 1 {
		t.Fatalf("%d observation(s) decoded, want 1", len(got.Observations))
	}
	o := got.Observations[0]
	if o.Page != "/ui/events/{subject}" || o.Element != "count" || o.Type != ObservedNumber {
		t.Errorf("the fields did not survive the decode: %+v", o)
	}
}

func TestFaultKindOutsideTheSixIsRefused(t *testing.T) {
	fixture := wellFormed("response", KindError)
	fixture["error"] = map[string]any{"kind": "oops", "detail": "d"}
	_, err := DecodeResponse(line(t, fixture))
	var ref Refusal
	if !asRefusal(err, &ref) {
		t.Fatalf("error kind \"oops\": %v, want a Refusal", err)
	}
	if !strings.Contains(ref.Reason, `"oops"`) {
		t.Errorf("refusal %q must name the kind it saw", ref.Reason)
	}
	// THE CONTROL: all six pass, so the arm is a set test and not a rejection
	// of everything but the one kind the fixture happens to use.
	for _, k := range []FaultKind{FaultContract, FaultField, FaultSource, FaultSession, FaultAction, FaultInternal} {
		fixture["error"] = map[string]any{"kind": string(k), "detail": "d"}
		if _, err := DecodeResponse(line(t, fixture)); err != nil {
			t.Errorf("error kind %q refused: %v", k, err)
		}
	}
}

// TestTheErrorEnvelopeIsAResponseOnly: an adapter reports that it could not
// act; the engine has nothing to report to an adapter, so `error` is not a
// request kind and the decoder says so rather than decoding it.
func TestTheErrorEnvelopeIsAResponseOnly(t *testing.T) {
	fixture := wellFormed("response", KindError)
	_, err := DecodeRequest(line(t, fixture))
	var ref Refusal
	if !asRefusal(err, &ref) {
		t.Fatalf("error as a request: %v, want a Refusal", err)
	}
	if ref.Kind != KindError || !strings.Contains(ref.Reason, "unknown kind for this direction") {
		t.Errorf("refusal is %+v", ref)
	}
	if _, err := DecodeResponse(line(t, fixture)); err != nil {
		t.Errorf("error as a response: refused: %v", err)
	}
}

func TestANonObjectLineIsRefusedWithoutAKind(t *testing.T) {
	for _, bad := range []string{``, `[]`, `"describe"`, `{`, `{"kind":1}`} {
		_, err := DecodeRequest([]byte(bad))
		var ref Refusal
		if !asRefusal(err, &ref) {
			t.Errorf("%q: %v, want a Refusal", bad, err)
			continue
		}
		if ref.Kind != "" {
			t.Errorf("%q: refusal claims kind %q, which was never read", bad, ref.Kind)
		}
	}
}

// TestTheSchemaAndTheStructsAgree is the two-instrument check: the schema table
// drives the decoder's refusals and the struct tags drive the unmarshal, and
// nothing but this test requires them to be the same schema.
// pairedMessages is every message type this package carries, keyed
// "<direction>/<kind>". It is a FUNCTION and not a literal inside one test
// because two oracles read it: the tag-against-table agreement below, and
// TestEveryKindInTheTablesIsPairedHere, which asks whether this map has fallen
// behind the tables themselves.
func pairedMessages() map[string]Message {
	return map[string]Message{
		"request/describe": DescribeRequest{},
		"request/execute":  ExecuteRequest{},
		"request/observe":  ObserveRequest{},
		"request/clock":    ClockRequest{},
		"request/session":  SessionRequest{},
		// FLOW WAS MISSING FROM THIS MAP UNTIL R9.1's SECOND HEAD, and it is the
		// hole this oracle exists to make impossible: `fdfc6e3` added the kind to
		// both tables, to the structs, to `into` and to `deref`, and every suite
		// here stayed green because the ONLY test that pairs a struct's tags
		// against the table walks a hand-written list. The declared `len(cases)`
		// guard cannot see an omission either -- it agrees with whatever the list
		// holds. So the count moves to 13 and the two entries are listed by name;
		// a kind added to `requests`/`responses` and not here is still invisible,
		// which is what TestEveryKindInTheTablesIsPairedHere below now refuses.
		"request/flow":      FlowRequest{},
		"response/describe": DescribeResponse{},
		"response/execute":  ExecuteResponse{},
		"response/observe":  ObserveResponse{},
		"response/clock":    ClockResponse{},
		"response/session":  SessionResponse{},
		"response/flow":     FlowResponse{},
		"response/error":    ErrorResponse{},
	}
}

func TestTheSchemaAndTheStructsAgree(t *testing.T) {
	cases := pairedMessages()
	if len(cases) != 13 {
		t.Fatalf("%d message types listed, want 13", len(cases))
	}
	for name, msg := range cases {
		direction, kindText, _ := strings.Cut(name, "/")
		required, optional, ok := Schema(direction, Kind(kindText))
		if !ok {
			t.Fatalf("%s: no schema", name)
		}
		want := append(append([]string(nil), required...), optional...)
		sort.Strings(want)
		got := JSONFields(msg)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: struct tags %v, schema %v", name, got, want)
		}

		// AND THE DECODER HANDS BACK THIS CONCRETE TYPE, not a pointer to it.
		//
		// `deref` is a per-type switch and a kind added without a case there
		// comes back as *XRequest while every other kind comes back as a
		// value, so a caller's `switch msg.(type)` over value types silently
		// misses it and falls through to "a kind this adapter does not serve".
		// THAT SHIPPED ONCE: the clock request was added, every suite in this
		// package passed, and the fake's own "every answer satisfies the real
		// decoder" passed too -- because a FAULT satisfies the decoder as
		// happily as an answer does. The arm that was missing is this one: not
		// "did it decode" but "is it the type the table says".
		decoded, err := decodeBy(direction, line(t, wellFormed(direction, Kind(kindText))))
		if err != nil {
			t.Fatalf("%s: the well-formed fixture does not decode: %v", name, err)
		}
		if reflect.TypeOf(decoded) != reflect.TypeOf(msg) {
			t.Errorf("%s: the decoder returns %T, want %T -- a missing `deref` case",
				name, decoded, msg)
		}
	}
}

// TestEveryKindInTheTablesIsPairedHere IS WHAT WOULD HAVE CAUGHT FLOW, and it
// reads the TABLES rather than a second hand-written list, which is the only
// way a list can be checked for an OMISSION: a kind in `requests` needs a
// "request/<kind>" entry and one in `responses` needs "response/<kind>". The
// agreement test above pairs a struct's tags against its shape and is therefore
// silent about a kind nobody listed -- `fdfc6e3` shipped flow in both tables,
// both structs, `into` and `deref` with every suite in this package green.
//
// IT ALSO REFUSES THE OTHER DIRECTION: an entry here for a kind no table
// carries, which would be a pairing against a shape that does not exist and
// would fail inside the agreement test as a missing schema rather than as what
// it is.
func TestEveryKindInTheTablesIsPairedHere(t *testing.T) {
	paired := pairedMessages()
	for direction, table := range map[string]map[Kind]shape{"request": requests, "response": responses} {
		for kind := range table {
			key := direction + "/" + string(kind)
			if _, ok := paired[key]; !ok {
				t.Errorf("%s is in the %s table and not in pairedMessages: its struct tags are "+
					"paired against nothing, which is how flow shipped unchecked", key, direction)
			}
		}
	}
	for key := range paired {
		direction, kindText, _ := strings.Cut(key, "/")
		var table map[Kind]shape
		switch direction {
		case "request":
			table = requests
		case "response":
			table = responses
		default:
			t.Errorf("pairedMessages has %q, whose direction is neither request nor response", key)
			continue
		}
		if _, ok := table[Kind(kindText)]; !ok {
			t.Errorf("pairedMessages has %q and the %s table does not: the pairing has no shape "+
				"to read", key, direction)
		}
	}
	proof.Logf(t, "PROOF %d message type(s) paired, %d request shape(s) and %d response shape(s)",
		len(paired), len(requests), len(responses))
}

// TestADisagreementBetweenTableAndStructIsReportedAsADefectHere drives the one
// path the agreement above is meant to make unreachable, by making it reachable
// for the length of this test: a field the table allows and no struct carries.
// Without this, that branch is prose.
func TestADisagreementBetweenTableAndStructIsReportedAsADefectHere(t *testing.T) {
	saved := requests[KindObserve]
	requests[KindObserve] = shape{
		required: saved.required,
		optional: append(append([]string(nil), saved.optional...), "nonesuch"),
	}
	defer func() { requests[KindObserve] = saved }()

	fixture := wellFormed("request", KindObserve)
	fixture["nonesuch"] = true
	_, err := DecodeRequest(line(t, fixture))
	var ref Refusal
	if !asRefusal(err, &ref) {
		t.Fatalf("%v, want a Refusal", err)
	}
	if !strings.Contains(ref.Reason, "defect in package wire") {
		t.Errorf("reason %q does not report it as this package's defect", ref.Reason)
	}
}

// TestANestedUnknownFieldIsTHEIRFaultNotOURS: check has already approved the
// top-level keys, so an unknown field reaching the unmarshal is either nested
// (the adapter's) or a field the table allows and no struct carries (ours).
// Reporting the first as ours would send an adapter author to read our source.
func TestANestedUnknownFieldIsTheMessagesFaultNotThePackages(t *testing.T) {
	fixture := wellFormed("response", KindDescribe)
	fixture["capabilities"] = map[string]any{
		"fault":   []any{"restart"}, // the field is `faults`
		"volume":  []any{},
		"upgrade": []any{},
	}
	_, err := DecodeResponse(line(t, fixture))
	var ref Refusal
	if !asRefusal(err, &ref) {
		t.Fatalf("%v, want a Refusal", err)
	}
	if ref.Field != "fault" {
		t.Errorf("refusal names %q, want the nested field \"fault\"", ref.Field)
	}
	if !strings.Contains(ref.Reason, "unknown field") || strings.Contains(ref.Reason, "defect in package wire") {
		t.Errorf("reason %q blames the wrong party", ref.Reason)
	}
}

// TestAWrongTypeIsRefusedWithTheFieldAndTheTypes, because "malformed" alone
// sends the reader back to the bytes to find out which field it was.
func TestAWrongTypeIsRefusedWithTheFieldAndTheTypes(t *testing.T) {
	fixture := wellFormed("request", KindExecute)
	fixture["action"] = 7
	_, err := DecodeRequest(line(t, fixture))
	var ref Refusal
	if !asRefusal(err, &ref) {
		t.Fatalf("%v, want a Refusal", err)
	}
	if ref.Field != "action" {
		t.Errorf("refusal names %q, want \"action\"", ref.Field)
	}
	if !strings.Contains(ref.Reason, "wrong type") || !strings.Contains(ref.Reason, "string") {
		t.Errorf("reason %q names neither the fault nor the wanted type", ref.Reason)
	}
}

func TestCheckVersionNamesEveryValueItSaw(t *testing.T) {
	if err := CheckVersion(ContractVersion, ContractVersion); err != nil {
		t.Fatalf("three equal versions refused: %v", err)
	}
	// THE MISMATCH VALUES ARE DERIVED, NOT TYPED. This table held "2" and "3"
	// as versions nobody declared, and the 2026-10-19 bump to contract 2 turned
	// two of its rows into AGREEMENTS -- the test then reported them as
	// wrongly accepted, which is the test going stale rather than the code
	// breaking. A mismatch has to be defined as "not what the engine says",
	// so it is spelled that way and survives every later bump.
	other := ContractVersion + "-not-this"
	another := ContractVersion + "-nor-this"
	for _, tc := range []struct{ adapter, platform string }{
		{other, ContractVersion},
		{ContractVersion, other},
		{other, another},
		{"", ""},
	} {
		err := CheckVersion(tc.adapter, tc.platform)
		if err == nil {
			t.Errorf("adapter %q platform %q: accepted", tc.adapter, tc.platform)
			continue
		}
		text := err.Error()
		for _, want := range []string{
			fmt.Sprintf("%q", ContractVersion),
			fmt.Sprintf("%q", tc.adapter),
			fmt.Sprintf("%q", tc.platform),
		} {
			if !strings.Contains(text, want) {
				t.Errorf("adapter %q platform %q: %q omits %s", tc.adapter, tc.platform, text, want)
			}
		}
	}
}

// TestEncodeFramesOneMessagePerLine: the newline is the framing, so it belongs
// to the encoder and not to every caller's memory.
// TestEveryRequestKindCanBeStamped IS WHAT WOULD HAVE CAUGHT Session.Flow, and
// it is the sibling of TestEveryKindInTheTablesIsPairedHere one layer out: the
// kind was in both shape tables, in `into`, in `deref`, in the fake and on a
// `Session` method, and the stamping switch had no case for it -- so every call
// to `Session.Flow` returned "flow is not a request kind this session can send"
// from the day it shipped, with this package and every suite green.
//
// IT READS THE `requests` TABLE, not a list: a kind added to the protocol and
// not to the switch fails here. And it asserts TWO things, because a case that
// exists is not a case that works -- a branch that forgot `r.Envelope = env`
// would satisfy the first assertion and send a request with no kind, no
// contract and no id, which the adapter would refuse by name three fields at a
// time.
func TestEveryRequestKindCanBeStamped(t *testing.T) {
	paired := pairedMessages()
	for kind := range requests {
		msg, ok := paired["request/"+string(kind)]
		if !ok {
			// Its own oracle above says this better; skipping keeps the two
			// failures from reading as one defect.
			continue
		}
		env := Envelope{Kind: kind, Contract: ContractVersion, ID: "4"}
		body, stamped := stampEnvelope(msg, kind, env)
		if !stamped {
			t.Errorf("%s has no case in stampEnvelope: Session's method for it answers "+
				"%q on every call, which is how flow shipped", kind,
				"is not a request kind this session can send")
			continue
		}
		raw, err := json.Marshal(body)
		if err != nil {
			t.Errorf("%s: the stamped body does not marshal: %v", kind, err)
			continue
		}
		var got map[string]any
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Errorf("%s: %v", kind, err)
			continue
		}
		for field, want := range map[string]any{
			"kind": string(kind), "contract": ContractVersion, "id": "4",
		} {
			if got[field] != want {
				t.Errorf("%s: the stamped request carries %s = %v, want %v -- a case that "+
					"forgot the assignment sends a request with no envelope at all",
					kind, field, got[field], want)
			}
		}
	}
	proof.Logf(t, "PROOF %d request kind(s) stamped, envelope asserted on each", len(requests))
}

// AND A KIND THE SWITCH DOES NOT KNOW IS REPORTED, not stamped with a zero
// value: the `false` return is what `exchange` turns into the refusal, and
// without this case that return is prose.
func TestAnUnknownRequestKindIsNotStamped(t *testing.T) {
	if _, ok := stampEnvelope(DescribeRequest{}, Kind("nonesuch"), Envelope{}); ok {
		t.Error("an unknown kind was stamped, so `exchange` would send it and the adapter would " +
			"answer a fault where this package should have refused")
	}
	// A RESPONSE KIND IS NOT A REQUEST KIND EITHER, driven because `error` is
	// in the responses table and nowhere near this switch.
	if _, ok := stampEnvelope(DescribeRequest{}, KindError, Envelope{}); ok {
		t.Error("the error kind was stamped as a request")
	}
}

// --- `from` and `stopped`, R9.1's second head ------------------------------

func flowRequestLine(t *testing.T, from any, steps int) []byte {
	t.Helper()
	m := wellFormed("request", KindFlow)
	list := make([]any, 0, steps)
	for i := 0; i < steps; i++ {
		list = append(list, map[string]any{"step": "click", "target": "x"})
	}
	m["steps"] = list
	if from != nil {
		m["from"] = from
	}
	return line(t, m)
}

// TestTheResumePositionIsBoundedOnBOTHSides drives both sides of the bound,
// which is the only way a `>=` can be shown to be the right operator: the LAST
// step is inside and must decode, and the step count itself is past the end and
// must be refused. A cell on one side alone passes under `>` just as happily.
func TestTheResumePositionIsBoundedOnBothSides(t *testing.T) {
	const steps = 3
	for _, c := range []struct {
		from   int
		refuse bool
	}{
		{0, false},         // the whole flow
		{1, false},         // inside
		{steps - 1, false}, // the last step, INSIDE
		{steps, true},      // the end, PAST
		{steps + 1, true},
		{-1, true},
	} {
		msg, err := DecodeRequest(flowRequestLine(t, c.from, steps))
		var ref Refusal
		refused := asRefusal(err, &ref)
		if refused != c.refuse {
			t.Errorf("from %d of %d steps: refused = %v, want %v (err %v)", c.from, steps, refused, c.refuse, err)
			continue
		}
		if refused {
			if ref.Field != "from" {
				t.Errorf("from %d: the refusal names %q, want from", c.from, ref.Field)
			}
			continue
		}
		req, ok := msg.(FlowRequest)
		if !ok {
			t.Fatalf("from %d decoded as %T", c.from, msg)
		}
		if req.From != c.from {
			t.Errorf("from %d decoded as %d", c.from, req.From)
		}
	}
}

// AND A FLOW OF NO STEPS IS NOT THE `from` REFUSAL, which is the narrowing the
// bound carries and the case that caught it: `from: 0` satisfies `0 >= 0`, so
// an unguarded bound refused a zero-step flow by naming the wrong field. The
// steps are the fault there, and the adapter is what names them -- this case
// pins only that `from` keeps its hands off.
func TestAFlowWithNoStepsIsNotAFromRefusal(t *testing.T) {
	msg, err := DecodeRequest(flowRequestLine(t, nil, 0))
	if err != nil {
		t.Fatalf("a flow of no steps was refused here: %v -- the zero-step rule is the adapter's", err)
	}
	if req, ok := msg.(FlowRequest); !ok || len(req.Steps) != 0 || req.From != 0 {
		t.Errorf("decoded %#v, want a FlowRequest with no steps and from 0", msg)
	}
}

// TestAnUnknownStopWordIsRefusedWithItsValue is Stop's closed-set arm, the same
// shape Source and the fault kinds already have. It matters more than theirs:
// the engine branches on this word to decide whether a finding is produced, so
// an adapter free to invent one would be deciding a `ui_timeout`.
func TestAnUnknownStopWordIsRefusedWithItsValue(t *testing.T) {
	for _, c := range []struct {
		stopped any
		refuse  bool
	}{
		{nil, false},
		{"", false},
		{"wait", false},
		{"step", false},
		{"timeout", true},
		{"WAIT", true},
	} {
		m := wellFormed("response", KindFlow)
		if c.stopped != nil {
			m["stopped"] = c.stopped
		}
		msg, err := DecodeResponse(line(t, m))
		var ref Refusal
		refused := asRefusal(err, &ref)
		if refused != c.refuse {
			t.Errorf("stopped %v: refused = %v, want %v (err %v)", c.stopped, refused, c.refuse, err)
			continue
		}
		if refused {
			if ref.Field != "stopped" {
				t.Errorf("stopped %v: the refusal names %q, want stopped", c.stopped, ref.Field)
			}
			if !strings.Contains(ref.Reason, fmt.Sprintf("%q", c.stopped)) {
				t.Errorf("stopped %v: the reason %q does not carry the value it refused", c.stopped, ref.Reason)
			}
			continue
		}
		resp, ok := msg.(FlowResponse)
		if !ok {
			t.Fatalf("stopped %v decoded as %T", c.stopped, msg)
		}
		if c.stopped == nil && resp.Stopped != StopNone {
			t.Errorf("an absent `stopped` decoded as %q, want the zero value", resp.Stopped)
		}
	}
}

// A NEGATIVE `reached` IS REFUSED TOO, because it is an index and the engine
// resumes from it: a flow answering -1 would have the engine send `from: -1`,
// which its own sibling clause then refuses one layer later and one message
// too late to say whose fault it was.
func TestANegativeReachedIsRefused(t *testing.T) {
	m := wellFormed("response", KindFlow)
	m["reached"] = -1
	_, err := DecodeResponse(line(t, m))
	var ref Refusal
	if !asRefusal(err, &ref) {
		t.Fatalf("%v, want a Refusal", err)
	}
	if ref.Field != "reached" {
		t.Errorf("the refusal names %q, want reached", ref.Field)
	}
}

func TestEncodeFramesOneMessagePerLine(t *testing.T) {
	var buf bytes.Buffer
	msg := ObserveResponse{Envelope: Envelope{Kind: KindObserve, Contract: ContractVersion, ID: "7"}, Value: 2}
	if err := Encode(&buf, msg); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if err := Encode(&buf, msg); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("two messages wrote %d line(s): %q", len(lines), buf.String())
	}
	back, err := DecodeResponse([]byte(lines[0]))
	if err != nil {
		t.Fatalf("round trip refused: %v", err)
	}
	got, ok := back.(ObserveResponse)
	if !ok {
		t.Fatalf("round trip produced %T", back)
	}
	if got.Env() != msg.Env() {
		t.Errorf("envelope %+v, want %+v", got.Env(), msg.Env())
	}
}

// TestEncodeReportsAFailedWrite, because a message that did not arrive is not a
// message sent: the same rule cmd/rumor applies to its own verdict lines.
func TestEncodeReportsAFailedWrite(t *testing.T) {
	err := Encode(failingWriter{}, DescribeRequest{Envelope: Envelope{Kind: KindDescribe, Contract: ContractVersion, ID: "1"}})
	if err == nil {
		t.Fatal("a failed write reported success")
	}
	if !strings.Contains(err.Error(), "describe") {
		t.Errorf("%q does not name the message kind", err)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, fmt.Errorf("no pipe") }

func asRefusal(err error, out *Refusal) bool {
	if err == nil {
		return false
	}
	ref, ok := err.(Refusal)
	if !ok {
		return false
	}
	*out = ref
	return true
}

// countingCloser is a stdin whose Close is counted, which is how the arm below
// tells "teardown ran once" from "teardown ran and nobody noticed how often".
// A boolean would answer the panic question and not the once question.
type countingCloser struct {
	n int32
}

func (c *countingCloser) Write(p []byte) (int, error) { return len(p), nil }
func (c *countingCloser) Close() error {
	atomic.AddInt32(&c.n, 1)
	return nil
}

// ISSUE: Close WAS IDEMPOTENT IN SEQUENCE AND NOT UNDER CONCURRENCY.
//
// The body was `select { case <-done: return nil; default: close(done) }`,
// which is TWO steps with a gap: two callers could both take the default and
// the second close(done) panics with "close of closed channel". The comment
// beside it said "Close is idempotent", true only of sequential calls, which is
// the shape ratchet raised in R5.4's review (coop r5/118 finding 3) and fable
// ruled a patch after v0.1.27 (r5/119).
//
// TWO ORACLES, because either alone passes a broken implementation:
//   - NO PANIC with N concurrent callers. A recover() here would hide exactly
//     the failure, so there is none: the panic propagates and fails the test.
//   - TEARDOWN RAN EXACTLY ONCE, counted through stdin's Close. Without this a
//     Close that simply never closed anything would pass the panic half.
//
// MUTATION-CHECKED: with the select-then-close restored this fails as
// "close of closed channel" within a run or two; under sync.Once it is green
// across 200 iterations of 8 callers.
func TestCloseIsIdempotentUnderConcurrency(t *testing.T) {
	for i := 0; i < 200; i++ {
		stdin := &countingCloser{}
		cancelled := int32(0)
		s := &Session{
			done:   make(chan struct{}),
			stdin:  stdin,
			cancel: func() { atomic.AddInt32(&cancelled, 1) },
		}
		var wg sync.WaitGroup
		start := make(chan struct{})
		for g := 0; g < 8; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start // release them together, so the gap is actually contested
				if err := s.Close(); err != nil {
					t.Errorf("Close returned %v", err)
				}
			}()
		}
		close(start)
		wg.Wait()

		if got := atomic.LoadInt32(&stdin.n); got != 1 {
			t.Fatalf("iteration %d: stdin closed %d time(s); teardown must run exactly "+
				"once however many callers ask", i, got)
		}
		if got := atomic.LoadInt32(&cancelled); got != 1 {
			t.Fatalf("iteration %d: cancel called %d time(s), want 1", i, got)
		}
		select {
		case <-s.done:
		default:
			t.Fatalf("iteration %d: done is still open after Close returned", i)
		}
	}
}

// slowCloser makes the WINDOW observable. Teardown that completes instantly cannot
// distinguish "Do blocked the other callers" from "the other callers returned early
// and teardown happened to finish before anyone looked".
type slowCloser struct {
	n int32
}

func (c *slowCloser) Write(p []byte) (int, error) { return len(p), nil }
func (c *slowCloser) Close() error {
	time.Sleep(2 * time.Millisecond)
	atomic.AddInt32(&c.n, 1)
	return nil
}

// ROOK'S NOTE at coop r5/181, taken rather than left: Close's comment promises that a
// RETURNED Close means the adapter is stopped, and NOTHING DROVE IT -- the promise
// rested on sync.Once, whose Do blocks every other caller until f returns. A comment
// is an untested claim whoever wrote it, so here is the arm that would see it break.
//
// WHY THE EXISTING CONCURRENCY ARM CANNOT ANSWER THIS: it reads the counters after
// wg.Wait(), by which time every caller has returned anyway, so it would pass against
// a Close that let losers return mid-teardown. The assertion below is made INSIDE each
// caller, because the claim is about what MY OWN Close returning implies -- and it is
// counted rather than t.Errorf'd at the point of failure so the message can name how
// many callers saw it.
//
// MUTATION-CHECKED: replace the sync.Once with a bare CompareAndSwap that tears down
// on the winner and returns immediately on the losers, and this fails with "7 caller(s)
// had Close return while teardown had closed stdin 0 time(s)".
func TestAReturnedCloseMeansTheAdapterIsStopped(t *testing.T) {
	for i := 0; i < 50; i++ {
		stdin := &slowCloser{}
		s := &Session{done: make(chan struct{}), stdin: stdin, cancel: func() {}}
		var wg sync.WaitGroup
		start := make(chan struct{})
		early := int32(0)
		for g := 0; g < 8; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start // released together, so the callers actually contest the gap
				if err := s.Close(); err != nil {
					t.Errorf("Close returned %v", err)
				}
				if atomic.LoadInt32(&stdin.n) == 0 {
					atomic.AddInt32(&early, 1)
				}
			}()
		}
		close(start)
		wg.Wait()
		if got := atomic.LoadInt32(&early); got != 0 {
			t.Fatalf("iteration %d: %d caller(s) had Close return while teardown had "+
				"closed stdin 0 time(s) -- a returned Close must mean stopped", i, got)
		}
		// AND THE POSITIVE HALF, so the arm above cannot pass on a Close that tears
		// down NOTHING: teardown must still have happened, exactly once.
		if got := atomic.LoadInt32(&stdin.n); got != 1 {
			t.Fatalf("iteration %d: stdin closed %d time(s), want 1", i, got)
		}
	}
}
