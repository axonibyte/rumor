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
	if n != 11 {
		t.Errorf("drove %d message kinds, want 11: a kind was added or removed without this file moving", n)
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
	if cases != 11 {
		t.Errorf("drove %d kinds, want 11", cases)
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
		{"ui", "t", false},
		{"direct", "", false},
		{"api", "", true},
		{"ui", "", true},
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
func TestTheSchemaAndTheStructsAgree(t *testing.T) {
	cases := map[string]Message{
		"request/describe":  DescribeRequest{},
		"request/execute":   ExecuteRequest{},
		"request/observe":   ObserveRequest{},
		"request/clock":     ClockRequest{},
		"request/session":   SessionRequest{},
		"response/describe": DescribeResponse{},
		"response/execute":  ExecuteResponse{},
		"response/observe":  ObserveResponse{},
		"response/clock":    ClockResponse{},
		"response/session":  SessionResponse{},
		"response/error":    ErrorResponse{},
	}
	if len(cases) != 11 {
		t.Fatalf("%d message types listed, want 11", len(cases))
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
	}
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
