// Package main's test holds the adapter to R4's contract and to the toy's
// routes, with a STUB toy rather than the real one.
//
// WHY A STUB HERE. What this file tests is the adapter's own job: which route
// each action maps to, which verb, where an id is read back from, and which
// FaultKind each failure gets. A stub makes every one of those assertable and
// lets the unhappy paths exist at all -- a real toy cannot be made to answer
// 500 on demand. The REAL pairing is proven elsewhere and not left to a
// comment: `rumor validate examples/toy` spawns this adapter for real, and four
// of validate's eighteen arms answer only if a describe comes back over a pipe
// from a process that started a toy of its own.
//
// SO THE STUB MUST NOT BECOME THE ORACLE. Every handler below asserts the
// method and path it was reached by, which is the half a stub can get wrong:
// an adapter that called /event/<id>/join would pass a stub that answered
// everything, and this one fails.
package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

type seen struct {
	method string
	path   string
	auth   string
	body   map[string]any
}

// answer is one canned reply: the STATUS as well as the body, because the toy's
// statuses are not uniform -- a created user is 201 and a join is 200 -- and an
// adapter is entitled to care which it got. A stub that answered 200 for
// everything would have hidden that, and did: it failed this suite's session
// case on the first run.
type answer struct {
	code int
	body any
}

func made(body any) answer    { return answer{http.StatusCreated, body} }
func fine(body any) answer    { return answer{http.StatusOK, body} }
func refused(code int) answer { return answer{code, map[string]any{"error": "refused"}} }

// doc is a stub answer served AS A DOCUMENT rather than as JSON, for the one
// route that answers HTML.
//
// WITHOUT IT THE PAGE OBSERVATION COULD ONLY BE DRIVEN ON ITS FAILURE PATH: a
// JSON body carries no #count element, so the extraction would fault and the
// POSITIVE -- the number a reader is actually shown -- would never be read by
// anything. A stub that cannot serve the shape under test turns a missing cell
// into a passing one.
type doc string

func served(body string) answer { return answer{http.StatusOK, doc(body)} }

// stub is a toy-shaped server that records what it was asked and answers from a
// table keyed on "METHOD /path".
func stub(t *testing.T, answers map[string]answer) (*adapter, *[]seen) {
	t.Helper()
	var log []seen
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := seen{method: r.Method, path: r.URL.Path, auth: r.Header.Get("Authorization")}
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&rec.body)
		}
		log = append(log, rec)
		key := r.Method + " " + r.URL.Path
		ans, ok := answers[key]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "the stub has no answer for " + key})
			return
		}
		if d, isDoc := ans.body.(doc); isDoc {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(ans.code)
			_, _ = io.WriteString(w, string(d))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(ans.code)
		_ = json.NewEncoder(w).Encode(ans.body)
	}))
	t.Cleanup(srv.Close)
	return &adapter{base: srv.URL, client: &http.Client{Timeout: 5 * time.Second}}, &log
}

// ask hands one request to the adapter and returns the answer as a map, which
// is how a fault and a typed response can be read by one helper.
//
// AND IT HOLDS EVERY ANSWER TO THE CONTRACT, not only the ones a test reads a
// field out of. A map read cannot notice a field the contract does not declare:
// the adapter emitted the error envelope's body as `fault` where contract.md and
// wire.ErrorResponse both say `error`, and this file asserted `out["fault"]`, so
// the suite agreed with the deviation rather than catching it -- quill's client
// found it instead (coop r5/89). wire.DecodeResponse sets
// DisallowUnknownFields, which is the instrument that client refused the message
// with, so running it over every answer here fails on ANY such divergence and
// not only on the one now fixed.
func ask(t *testing.T, a *adapter, msg map[string]any) map[string]any {
	t.Helper()
	msg["contract"] = wire.ContractVersion
	if _, ok := msg["id"]; !ok {
		msg["id"] = "x1"
	}
	line, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	raw, err := json.Marshal(a.answer(line))
	if err != nil {
		t.Fatalf("the answer does not marshal: %v", err)
	}
	if _, err := wire.DecodeResponse(raw); err != nil {
		t.Fatalf("the answer is not a contract response: %v\n%s", err, raw)
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("the answer is not an object: %v", err)
	}
	return out
}

// faultOf reads the error envelope through the DECODER rather than out of the
// map, for the reason ask's comment gives: the field names then come from
// wire.ErrorResponse, so the contract names them in one place.
func faultOf(t *testing.T, out map[string]any) (string, string) {
	t.Helper()
	if out["kind"] != string(wire.KindError) {
		t.Fatalf("not a fault: %v", out)
	}
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("the answer does not marshal: %v", err)
	}
	msg, err := wire.DecodeResponse(raw)
	if err != nil {
		t.Fatalf("the error envelope is not a contract response: %v\n%s", err, raw)
	}
	resp, ok := msg.(wire.ErrorResponse)
	if !ok {
		t.Fatalf("the error envelope decoded as %T, want wire.ErrorResponse: %v", msg, out)
	}
	return string(resp.Error.Kind), resp.Error.Detail
}

func TestDescribeReportsTheDeclaredSurfaceAndTheContract(t *testing.T) {
	a, _ := stub(t, nil)
	out := ask(t, a, map[string]any{"kind": "describe"})
	if out["kind"] != "describe" || out["contract"] != wire.ContractVersion || out["id"] != "x1" {
		t.Fatalf("the envelope is wrong: %v", out)
	}
	got := map[string]bool{}
	for _, item := range out["actions"].([]any) {
		got[item.(map[string]any)["name"].(string)] = true
	}
	for _, want := range actions {
		if !got[want.Name] {
			t.Errorf("describe omits the action %q", want.Name)
		}
	}
	if n := len(out["actions"].([]any)); n != len(actions) {
		t.Errorf("describe reports %d action(s), the surface declares %d", n, len(actions))
	}
	// THE THREE CAPABILITY LISTS ARE PRESENT AND EMPTY, which contract.md says
	// is an ANSWER rather than an omission: validate's arms 9a-9c fire on an
	// empty list where a declared ask has nothing behind it, and would be
	// vacuous on an absent field.
	caps, ok := out["capabilities"].(map[string]any)
	if !ok {
		t.Fatalf("describe carries no capabilities: %v", out)
	}
	for _, key := range []string{"faults", "volume", "upgrade"} {
		list, present := caps[key]
		if !present {
			t.Errorf("capabilities omits %q, which must be empty rather than absent", key)
			continue
		}
		if items, _ := list.([]any); len(items) != 0 {
			t.Errorf("capabilities.%s is %v; the toy offers none in R5", key, items)
		}
	}
}

func TestEachActionTakesItsOwnRouteAndVerb(t *testing.T) {
	cases := []struct {
		action  string
		args    map[string]any
		method  string
		path    string
		status  int
		creates string
	}{
		{"create_user", map[string]any{"name": "alice"}, http.MethodPost, "/users", http.StatusCreated, "user"},
		{"create_event", map[string]any{"title": "t", "capacity": 2}, http.MethodPost, "/events", http.StatusCreated, "event"},
		{"join_event", map[string]any{"event": "e1"}, http.MethodPost, "/events/e1/join", http.StatusOK, ""},
		{"leave_event", map[string]any{"event": "e1"}, http.MethodDelete, "/events/e1/join", http.StatusOK, ""},
		{"delete_event", map[string]any{"event": "e1"}, http.MethodDelete, "/events/e1", http.StatusOK, ""},
		{"join_event_ui", map[string]any{"event": "e1"}, http.MethodPost, "/ui/events/e1/join", http.StatusOK, ""},
	}
	for _, c := range cases {
		t.Run(c.action, func(t *testing.T) {
			a, log := stub(t, map[string]answer{
				c.method + " " + c.path: {c.status, map[string]any{"id": "made1"}},
			})
			out := ask(t, a, map[string]any{
				"kind": "execute", "action": c.action, "token": "t1", "args": c.args,
			})
			if out["kind"] != "execute" {
				t.Fatalf("%s did not answer an execute: %v", c.action, out)
			}
			if len(*log) != 1 {
				t.Fatalf("%s made %d request(s), want 1: %v", c.action, len(*log), *log)
			}
			got := (*log)[0]
			if got.method != c.method || got.path != c.path {
				t.Errorf("%s called %s %s, want %s %s", c.action, got.method, got.path, c.method, c.path)
			}
			if got.auth != "Bearer t1" {
				t.Errorf("%s sent the authorization %q", c.action, got.auth)
			}
			ids, _ := out["ids"].(map[string]any)
			if c.creates == "" {
				if len(ids) != 0 {
					t.Errorf("%s creates nothing and returned ids %v", c.action, ids)
				}
				return
			}
			if ids[c.creates] != "made1" {
				t.Errorf("%s returned ids %v, want the id under %q", c.action, ids, c.creates)
			}
		})
	}
}

// A REFUSAL BY THE SYSTEM IS A RESULT, NOT A FAULT. The toy saying "at
// capacity" is the system under test behaving, and an adapter that reported it
// as a protocol fault would make every ordinary refusal look like an adapter
// that could not run -- the boundary docs/contract.md draws between a project
// being wrong and a run that could not happen.
func TestASystemRefusalComesBackAsAResultWithItsStatus(t *testing.T) {
	a, _ := stub(t, map[string]answer{"POST /events/e1/join": refused(http.StatusConflict)})
	out := ask(t, a, map[string]any{
		"kind": "execute", "action": "join_event", "token": "t1",
		"args": map[string]any{"event": "e1"},
	})
	if out["kind"] != "execute" {
		t.Fatalf("a 409 did not come back as an execute result: %v", out)
	}
	resp, ok := out["response"].(map[string]any)
	if !ok {
		t.Fatalf("no response object: %v", out)
	}
	if resp["status"].(float64) != float64(http.StatusConflict) {
		t.Errorf("the response carries status %v, want 409", resp["status"])
	}
}

func TestAnUnknownActionIsAFaultThatNamesWhatIsServed(t *testing.T) {
	a, log := stub(t, nil)
	kind, detail := faultOf(t, ask(t, a, map[string]any{
		"kind": "execute", "action": "levitate", "token": "t1", "args": map[string]any{},
	}))
	if kind != string(wire.FaultAction) {
		t.Errorf("an unknown action is fault %q, want %q", kind, wire.FaultAction)
	}
	if !strings.Contains(detail, "create_user") {
		t.Errorf("the fault does not name what is served: %q", detail)
	}
	if len(*log) != 0 {
		t.Errorf("an unknown action reached the toy %d time(s)", len(*log))
	}
}

// TestTheClockIsTheTOYsNumberAndThisAdapterOnlyForwardsIt replaces 101's
// ladder, which lived here while the clock did.
//
// THE CELL IS THE DIVERGENCE AND NOT THE AGREEMENT: the stub answers an
// instant that is NOT the ask and not the sum of the asks, so an adapter
// keeping its own copy -- which is what this one did until R7.4 -- fails here
// whichever arithmetic it chose. An answer equal to the ask would have passed
// against a correct adapter and against two wrong ones.
//
// AND THE REQUEST IS ASSERTED, NOT ONLY THE ANSWER: op and `t` reach the toy
// as sent, because an adapter that forwarded a `set` as an `advance` would
// agree with this stub about every number it returned.
func TestTheClockIsTheTOYsNumberAndThisAdapterOnlyForwardsIt(t *testing.T) {
	const theToysOwnNumber = 777000000000
	for _, c := range []struct {
		op string
		t  int64
	}{
		{"set", 5000000000},
		{"advance", 30000000000},
		// ZERO IS A LEGAL INSTANT and `prune` drops nil and not zero, so this
		// cell says the key arrives rather than being read as absent.
		{"set", 0},
	} {
		a, log := stub(t, map[string]answer{
			"POST /clock": {code: 200, body: map[string]any{"t": theToysOwnNumber}},
		})
		out := ask(t, a, map[string]any{"kind": "clock", "op": c.op, "t": c.t})
		got, ok := out["t"]
		if !ok {
			t.Fatalf("%s %d: the answer carries no `t`: %v", c.op, c.t, out)
		}
		// JSON numbers decode as float64; the comparison is on the int64 the
		// protocol declares, not on whatever the round trip produced.
		if int64(got.(float64)) != theToysOwnNumber {
			t.Errorf("%s %d: the adapter answers %v, want the toy's %d", c.op, c.t, got, theToysOwnNumber)
		}
		if len(*log) != 1 {
			t.Fatalf("%s %d: the toy saw %d request(s), want 1: %v", c.op, c.t, len(*log), *log)
		}
		sent := (*log)[0]
		if sent.method != "POST" || sent.path != "/clock" {
			t.Errorf("%s %d: the toy saw %s %s", c.op, c.t, sent.method, sent.path)
		}
		if op, _ := sent.body["op"].(string); op != c.op {
			t.Errorf("%s %d: the toy was told op %q", c.op, c.t, op)
		}
		sentT, ok := sent.body["t"].(float64)
		if !ok {
			t.Errorf("%s %d: the toy was sent no `t`: %v", c.op, c.t, sent.body)
		} else if int64(sentT) != c.t {
			t.Errorf("%s %d: the toy was sent t=%d", c.op, c.t, int64(sentT))
		}
	}
}

// TestAToyThatRefusesAClockMoveIsAFaultAndNotAnInstant is the other half of
// forwarding: the adapter must not invent a time when the world would not give
// one. The stub answers 400, which is what cmd/toy does for an op it does not
// know -- reachable here because anything may call the toy.
func TestAToyThatRefusesAClockMoveIsAFaultAndNotAnInstant(t *testing.T) {
	a, _ := stub(t, map[string]answer{
		"POST /clock": {code: 400, body: map[string]any{"error": "no"}},
	})
	kind, detail := faultOf(t, ask(t, a, map[string]any{
		"kind": "clock", "op": "advance", "t": 30000000000,
	}))
	if kind != string(wire.FaultInternal) {
		t.Errorf("a refused clock move is fault %q, want %q", kind, wire.FaultInternal)
	}
	for _, want := range []string{"400", "advance", "30000000000"} {
		if !strings.Contains(detail, want) {
			t.Errorf("the fault does not name %q: %q", want, detail)
		}
	}
}

// TestAnUnknownClockOpIsAFaultNamingTheTwo is the refusal beside it: the
// decoder checks field PRESENCE and not a variant's value, so an unknown op
// arrives at the adapter intact.
func TestAnUnknownClockOpIsAFaultNamingTheTwo(t *testing.T) {
	a, log := stub(t, nil)
	kind, detail := faultOf(t, ask(t, a, map[string]any{
		"kind": "clock", "op": "rewind", "t": 1,
	}))
	if kind != string(wire.FaultField) {
		t.Errorf("an unknown clock op is fault %q, want %q", kind, wire.FaultField)
	}
	for _, want := range []string{"rewind", "set", "advance"} {
		if !strings.Contains(detail, want) {
			t.Errorf("the fault does not name %q: %q", want, detail)
		}
	}
	if len(*log) != 0 {
		t.Errorf("a refused clock op reached the toy %d time(s)", len(*log))
	}
}

func TestAnActionNeedingAnEventWithoutOneIsRefusedBeforeTheToy(t *testing.T) {
	a, log := stub(t, nil)
	kind, _ := faultOf(t, ask(t, a, map[string]any{
		"kind": "execute", "action": "join_event", "token": "t1", "args": map[string]any{},
	}))
	if kind != string(wire.FaultField) {
		t.Errorf("a missing event id is fault %q, want %q", kind, wire.FaultField)
	}
	if len(*log) != 0 {
		t.Errorf("a malformed action reached the toy %d time(s): a request with no subject must not", len(*log))
	}
}

// THE SUBJECT ARRIVES IN `args`, which is contract 2's rule. This arm read
// `event_count/e7` until 2026-10-24 and passed, because the adapter split the
// query -- so the suite agreed with the adapter and both disagreed with the
// contract and with examples/toy, which has declared args = ["event"] since
// the contract moved. A walk against the real project faulted at its first
// observation; no arm here could see it, because every arm spoke contract 1.
func TestAnObservationCarriesItsSubjectInArgs(t *testing.T) {
	a, log := stub(t, map[string]answer{
		"GET /events/e7/count": fine(map[string]any{"event": "e7", "count": 3}),
	})
	out := ask(t, a, map[string]any{
		"kind": "observe", "query": "event_count", "source": "api", "token": "t1",
		"args": map[string]any{"event": "e7"},
	})
	if out["kind"] != "observe" {
		t.Fatalf("not an observe: %v", out)
	}
	// THE VALUE IS A SCALAR, which is what the reviewed cases carry
	// (examples/toy/cases/honest-count.json: "value": 1) and what the statute
	// compares to `believed`. This arm asserted a MAP until 2026-10-24 and so
	// agreed with the adapter against the cases.
	if got, ok := out["value"].(float64); !ok || got != 3 {
		t.Errorf("the value is %#v, want the scalar 3", out["value"])
	}
	if len(*log) != 1 || (*log)[0].path != "/events/e7/count" {
		t.Fatalf("the adapter called %v", *log)
	}
}

func TestAParameterizedObservationWithNoSubjectIsRefused(t *testing.T) {
	a, _ := stub(t, nil)
	kind, detail := faultOf(t, ask(t, a, map[string]any{
		"kind": "observe", "query": "event_count", "source": "api", "token": "t1",
	}))
	if kind != string(wire.FaultField) {
		t.Errorf("a subjectless event_count is fault %q, want %q", kind, wire.FaultField)
	}
	if !strings.Contains(detail, "args.event") {
		t.Errorf("the fault does not say where the subject goes: %q", detail)
	}
}

// THE SLASH FORM IS GONE, AND THIS IS THE ARM THAT WOULD HAVE CAUGHT ITS
// SURVIVAL. Before 2026-10-24 the adapter split the query, so this request
// was ANSWERED; now `event_count/e7` is simply not an observation this
// adapter serves, and the name it reports back is the whole query. Without
// this cell the only thing standing between the tree and contract 1's shape
// is the arms above happening to send args -- and arms that send the right
// thing cannot tell you the wrong thing is still accepted.
func TestTheSlashFormQueryIsNoLongerSplit(t *testing.T) {
	a, log := stub(t, map[string]answer{
		"GET /events/e7/count": fine(map[string]any{"event": "e7", "count": 3}),
	})
	kind, detail := faultOf(t, ask(t, a, map[string]any{
		"kind": "observe", "query": "event_count/e7", "source": "api", "token": "t1",
	}))
	if kind != string(wire.FaultInternal) {
		t.Errorf("a slash-form query is fault %q, want %q", kind, wire.FaultInternal)
	}
	if !strings.Contains(detail, "event_count/e7") {
		t.Errorf("the fault does not name the query it refused: %q", detail)
	}
	if len(*log) != 0 {
		t.Errorf("the adapter asked the toy anyway, %d time(s): %v", len(*log), *log)
	}
}

// THE SOURCE IS NOT SUBSTITUTED. An adapter asked for a source it does not
// serve an observation through says FaultSource, rather than answering through
// the channel it does have -- which would be a value the engine attributes to
// the wrong oracle.
func TestTheWrongSourceIsAFaultAndNotAnAnswer(t *testing.T) {
	a, log := stub(t, map[string]answer{
		"GET /events/e1/count": fine(map[string]any{"count": 1}),
	})
	kind, detail := faultOf(t, ask(t, a, map[string]any{
		"kind": "observe", "query": "event_count", "source": "direct",
		"args": map[string]any{"event": "e1"},
	}))
	if kind != string(wire.FaultSource) {
		t.Errorf("a source mismatch is fault %q, want %q", kind, wire.FaultSource)
	}
	if !strings.Contains(detail, "api") || !strings.Contains(detail, "direct") {
		t.Errorf("the fault names neither side: %q", detail)
	}
	if len(*log) != 0 {
		t.Errorf("the adapter asked the toy anyway, %d time(s)", len(*log))
	}
}

// THE DIRECT ORACLE IS THE ENGINE'S AND CARRIES NO TOKEN, which is
// vocabulary.md's rule for `direct`; the store route is the second oracle
// behind what the API reports.
func TestTheDirectObservationGoesToTheStoreRouteWithNoToken(t *testing.T) {
	a, log := stub(t, map[string]answer{
		"GET /store/events/e2/count": fine(map[string]any{"event": "e2", "count": 5}),
	})
	out := ask(t, a, map[string]any{
		"kind": "observe", "query": "store_event_count", "source": "direct",
		"args": map[string]any{"event": "e2"},
	})
	if out["kind"] != "observe" {
		t.Fatalf("not an observe: %v", out)
	}
	if len(*log) != 1 {
		t.Fatalf("the adapter made %d request(s)", len(*log))
	}
	if got := (*log)[0]; got.path != "/store/events/e2/count" || got.auth != "" {
		t.Errorf("the direct observation went to %s with authorization %q", got.path, got.auth)
	}
}

func TestAnUnknownObservationIsRefusedByName(t *testing.T) {
	a, _ := stub(t, nil)
	kind, detail := faultOf(t, ask(t, a, map[string]any{
		"kind": "observe", "query": "the_vibes", "source": "api", "token": "t1",
	}))
	if kind != string(wire.FaultInternal) {
		t.Errorf("an unknown observation is fault %q", kind)
	}
	if !strings.Contains(detail, "the_vibes") {
		t.Errorf("the fault does not name the query: %q", detail)
	}
}

func TestASessionCarriesTheActorAndTheContextAndReturnsTheToken(t *testing.T) {
	a, log := stub(t, map[string]answer{
		"POST /sessions": made(map[string]any{"token": "t9", "user_id": "u1", "context": "api"}),
	})
	out := ask(t, a, map[string]any{"kind": "session", "actor": "alice", "context": "api"})
	if out["kind"] != "session" || out["token"] != "t9" {
		t.Fatalf("the session answer is %v", out)
	}
	if len(*log) != 1 {
		t.Fatalf("the adapter made %d request(s)", len(*log))
	}
	body := (*log)[0].body
	if body["actor"] != "alice" || body["context"] != "api" {
		t.Errorf("the session request carried %v", body)
	}
}

func TestARefusedSessionIsAFaultOfItsOwnKind(t *testing.T) {
	a, _ := stub(t, map[string]answer{"POST /sessions": refused(http.StatusNotFound)})
	kind, detail := faultOf(t, ask(t, a, map[string]any{
		"kind": "session", "actor": "nobody", "context": "api",
	}))
	if kind != string(wire.FaultSession) {
		t.Errorf("a refused session is fault %q, want %q", kind, wire.FaultSession)
	}
	if !strings.Contains(detail, "nobody") {
		t.Errorf("the fault does not name the actor: %q", detail)
	}
}

// A LINE THE DECODER REFUSES STILL GETS AN ANSWER, correlated by id: the
// transport is one response per request, so an adapter that stayed silent on a
// malformed line would leave the engine waiting for a message that never comes.
func TestAMalformedRequestIsAnsweredWithAFieldFaultCarryingItsID(t *testing.T) {
	a, _ := stub(t, nil)
	raw, err := json.Marshal(a.answer([]byte(`{"kind":"observe","contract":"1","id":"z9","source":"api"}`)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out["id"] != "z9" {
		t.Errorf("the fault carries id %v, want z9: a fault nobody can correlate is a fault nobody can act on", out["id"])
	}
	kind, _ := faultOf(t, out)
	if kind != string(wire.FaultField) {
		t.Errorf("a missing required field is fault %q, want %q", kind, wire.FaultField)
	}
}

func TestTheAddressIsReadOutOfTheToysOwnLine(t *testing.T) {
	cases := []struct{ line, want string }{
		{"toy: listening on 127.0.0.1:8080 state=/tmp/x/state.json\n", "127.0.0.1:8080"},
		{"toy: listening on [::1]:9 state=/s\n", "[::1]:9"},
		{"nothing useful here\n", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := addressOf(c.line); got != c.want {
			t.Errorf("addressOf(%q) = %q, want %q", c.line, got, c.want)
		}
	}
}

// prune exists so an omitted optional argument does not travel as a null into
// the toy's strict decoder, which would refuse it by name.
func TestPruneDropsNilsAndKeepsEverythingElse(t *testing.T) {
	got := prune(map[string]any{"a": 1, "b": nil, "c": "", "d": false})
	if _, present := got["b"]; present {
		t.Errorf("prune kept a nil: %v", got)
	}
	for _, key := range []string{"a", "c", "d"} {
		if _, present := got[key]; !present {
			t.Errorf("prune dropped %q, which is not nil: %v", key, got)
		}
	}
}

// fakeToy writes an executable that prints the one line open() parses and then
// sleeps, so the adapter has a live child to kill. A SCRIPT RATHER THAN THE
// REAL TOY because the subject here is the adapter's own housekeeping: a real
// toy would make the arm depend on cmd/toy building and binding a port, and
// neither is what leaked.
func fakeToy(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "faketoy")
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("writing the fake toy: %v", err)
	}
	return path
}

// THE STATE DIRECTORY THE ADAPTER MAKES IS THE ADAPTER'S TO REMOVE.
//
// quill measured the leak at coop r5/102: open() made the directory with
// MkdirTemp and close() only killed the child, so every run left one behind --
// 11 of 11, clean runs included, which is why nothing failed and nobody saw it.
// fable placed the fix at r5/103.
//
// TMPDIR IS REDIRECTED INTO THE TEST'S OWN SANDBOX so the assertion can be
// about the COUNT of directories and not only about the one path we were handed
// back. That is what lets the failing-open cell below say anything at all: when
// open() returns an error it returns no adapter, so there is no name to check
// and only the census can see a leak.
func TestTheAdapterRemovesTheStateDirectoryItMade(t *testing.T) {
	sandbox := t.TempDir()
	t.Setenv("TMPDIR", sandbox)
	t.Setenv("TOY_ADDR", "")

	// COUNTED BY THE ADAPTER'S OWN PREFIX, not by everything in TMPDIR. My
	// first version counted every entry and failed on its own fixture:
	// t.TempDir() honors TMPDIR too, so fakeToy's directory landed in the
	// sandbox and read as a leak. The census has to name what it is about.
	entries := func(where string) int {
		t.Helper()
		got, err := filepath.Glob(filepath.Join(where, "toy-adapter-*"))
		if err != nil {
			t.Fatalf("globbing %s: %v", where, err)
		}
		return len(got)
	}

	t.Run("a clean open and close leaves nothing", func(t *testing.T) {
		before := entries(sandbox)
		a, err := open(fakeToy(t, "#!/bin/sh\necho 'toy: listening on 127.0.0.1:65000 state=x'\nexec sleep 60\n"), "")
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		if a.stateDir == "" {
			t.Fatal("open made no state directory, so this arm proves nothing about removing one")
		}
		if _, err := os.Stat(a.stateDir); err != nil {
			t.Fatalf("the state directory open reported does not exist: %v", err)
		}
		dir := a.stateDir
		a.close()
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("close left %s behind (stat err %v)", dir, err)
		}
		if got := entries(sandbox); got != before {
			t.Errorf("the sandbox holds %d entrie(s), started at %d", got, before)
		}
	})

	// THE CONTROL, and it is the half a blanket RemoveAll would break: a
	// --state path is the CALLER'S directory and close() must not touch it.
	t.Run("a caller-supplied state path is left alone", func(t *testing.T) {
		mine := t.TempDir()
		a, err := open(fakeToy(t, "#!/bin/sh\necho 'toy: listening on 127.0.0.1:65001 state=x'\nexec sleep 60\n"),
			filepath.Join(mine, "state.json"))
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		if a.stateDir != "" {
			t.Errorf("open claimed %q as its own, but the caller named the state path", a.stateDir)
		}
		a.close()
		if _, err := os.Stat(mine); err != nil {
			t.Errorf("close removed the caller's directory: %v", err)
		}
	})

	// AND AN open THAT FAILS AFTER MkdirTemp: the toy exits without printing an
	// address, so open errors on the read -- the path that used to leak with
	// nothing to name.
	t.Run("a failed open leaves nothing", func(t *testing.T) {
		before := entries(sandbox)
		a, err := open(fakeToy(t, "#!/bin/sh\nexit 3\n"), "")
		if err == nil {
			t.Fatal("open succeeded against a toy that prints no address, so this arm cannot reach the leak")
		}
		if a != nil {
			t.Errorf("open returned an adapter alongside an error: %+v", a)
		}
		if got := entries(sandbox); got != before {
			t.Errorf("a failed open left %d entrie(s) in the sandbox, started at %d", got, before)
		}
	})
}

// eventPage is the toy's own page, byte for byte as cmd/toy writes it. It is
// copied rather than built from a shared template on purpose: if the toy's
// markup moves, this stub keeps serving the OLD shape and the extraction test
// goes green against a page no toy produces. The pairing against the real
// template is `rumor validate examples/toy` and the walk, which spawn a toy.
const eventPage = `<!doctype html>
<title>standup</title>
<h1 id="title">standup</h1>
<p id="role">guest</p>
<p id="count">2 of 10</p>
<form id="join" method="post" action="/ui/events/e-1/join?token=t-1">
<button type="submit">join</button>
</form>
`

// TestThePageObservationReadsTheRenderedCount is the browser source's own
// positive, and it reads the number the application SHOWS rather than the one
// it computes -- which is the whole reason a browser observation exists beside
// `event_count` instead of being a second api query.
func TestThePageObservationReadsTheRenderedCount(t *testing.T) {
	a, log := stub(t, map[string]answer{
		"GET /ui/events/e-1": served(eventPage),
	})
	out := ask(t, a, map[string]any{
		"kind": "observe", "query": "page_count", "source": "browser", "token": "t-1",
		"args": map[string]any{"event": "e-1"},
	})
	if out["kind"] != "observe" {
		t.Fatalf("not an observe: %v", out)
	}
	// THE SCALAR AND NOT THE PAIR. The element reads "2 of 10" and the
	// observation's value is the COUNT: the statute compares it to `believed`,
	// which is a number, so handing back "2 of 10" or a map would never equal
	// it and every reading would be a finding (the shape event_count's own
	// value had until 2026-10-24).
	if got, ok := out["value"].(float64); !ok || got != 2 {
		t.Errorf("the value is %#v, want the scalar 2", out["value"])
	}
	if len(*log) != 1 || (*log)[0].path != "/ui/events/e-1" {
		t.Fatalf("the adapter called %v", *log)
	}
	// AND THROUGH THE READER'S OWN SESSION: the page is gated on the role the
	// token's user holds, so a fetch with no Authorization reads a different
	// page -- or none -- than the actor the walk drew.
	if auth := (*log)[0].auth; auth != "Bearer t-1" {
		t.Errorf("the page was fetched with auth %q, want the observer's own token", auth)
	}
}

// TestThePageObservationIsBrowserSourcedAndNotApi pins the SOURCE this query is
// served through, which is the half that keeps `browser` from becoming a label:
// an adapter answering it through `api` would hand back a number read from the
// store while the ledger recorded it as a reading of the page.
func TestThePageObservationIsBrowserSourcedAndNotApi(t *testing.T) {
	a, log := stub(t, map[string]answer{
		"GET /ui/events/e-1": served(eventPage),
	})
	kind, detail := faultOf(t, ask(t, a, map[string]any{
		// THE TOKEN IS HERE TO REACH THE CHECK UNDER TEST. internal/wire
		// refuses an `api` or `browser` observation carrying none -- fault
		// `field`, before this adapter sees the message -- so without it this
		// cell measures the wire's validation and not the adapter's source
		// pairing. Measured: it failed for exactly that reason first.
		"kind": "observe", "query": "page_count", "source": "api", "token": "t-1",
		"args": map[string]any{"event": "e-1"},
	}))
	if kind != string(wire.FaultSource) {
		t.Errorf("a source mismatch is fault %q, want %q", kind, wire.FaultSource)
	}
	if !strings.Contains(detail, "browser") || !strings.Contains(detail, "api") {
		t.Errorf("the fault names neither side: %q", detail)
	}
	if len(*log) != 0 {
		t.Errorf("the adapter fetched the page anyway, %d time(s)", len(*log))
	}
}

// TestAPageWithoutACountElementIsAFaultAndNotZero holds the sentence the
// extraction's own comment makes: a page carrying no #count is a page this
// observation CANNOT READ, and zero is a reading. Without this cell the
// extraction could return (0, true) on a broken template and every walk would
// see an empty event rather than a fault.
func TestAPageWithoutACountElementIsAFaultAndNotZero(t *testing.T) {
	a, _ := stub(t, map[string]answer{
		"GET /ui/events/e-1": served("<!doctype html>\n<title>standup</title>\n<p id=\"role\">guest</p>\n"),
	})
	kind, detail := faultOf(t, ask(t, a, map[string]any{
		"kind": "observe", "query": "page_count", "source": "browser", "token": "t-1",
		"args": map[string]any{"event": "e-1"},
	}))
	if kind != string(wire.FaultInternal) {
		t.Errorf("a page with no #count is fault %q, want %q", kind, wire.FaultInternal)
	}
	if !strings.Contains(detail, "#count") {
		t.Errorf("the fault does not name what was missing: %q", detail)
	}
}

// TestAPageNotServedIsMissingAndNotAFault holds fable's ruling at coop
// rumor/1669, and it REPLACES a cell of mine that asserted the opposite an hour
// earlier: a 404 on the page used to be `internal`, which refused the run,
// because the walk has no skip for an adapter fault.
//
// A PAGE THAT IS NOT SERVED IS WHAT A BROWSER SEES, so it is an answer. The toy
// answers 404 both for a deleted event and for one not visible to this reader's
// role, and `missing` is neither the number nor silence -- which keeps the half
// of the old cell that was right: a hidden page read as 0 would look like an
// empty event.
//
// THE 500 IS THE DISCRIMINATING INPUT and the reason this is a pair: the
// narrowing is "404 alone, not every >= 400", so the cell carries a status the
// narrowing must still refuse. Without it, widening the branch to every error
// would make a BROKEN SERVER read exactly like a deleted event, and nothing
// would say so.
func TestAPageNotServedIsMissingAndNotAFault(t *testing.T) {
	for _, c := range []struct {
		what    string
		code    int
		missing bool
	}{
		{"a 404 is an answer", http.StatusNotFound, true},
		{"a 500 is still a fault", http.StatusInternalServerError, false},
	} {
		t.Run(c.what, func(t *testing.T) {
			a, _ := stub(t, map[string]answer{
				"GET /ui/events/e-1": {c.code, doc("not served\n")},
			})
			out := ask(t, a, map[string]any{
				"kind": "observe", "query": "page_count", "source": "browser", "token": "t-1",
				"args": map[string]any{"event": "e-1"},
			})
			if !c.missing {
				kind, detail := faultOf(t, out)
				if kind != string(wire.FaultInternal) {
					t.Errorf("fault %q, want %q", kind, wire.FaultInternal)
				}
				if !strings.Contains(detail, "500") {
					t.Errorf("the fault does not name the status: %q", detail)
				}
				t.Logf("PROOF %s", c.what)
				return
			}
			if out["kind"] != "observe" {
				t.Fatalf("a 404 did not answer an observe: %v", out)
			}
			if m, ok := out["missing"].(bool); !ok || !m {
				t.Errorf("the answer does not carry missing: %v", out)
			}
			// AND NO VALUE, because the SPELLING is the walk's to write: an
			// adapter reporting the word would make every adapter a second
			// place it could be misspelled.
			if v, present := out["value"]; present && v != nil {
				t.Errorf("the adapter wrote a value beside missing: %#v", v)
			}
			t.Logf("PROOF %s, and the adapter writes no value for it", c.what)
		})
	}
}

// TestAPageObservationWithNoSubjectIsRefusedBeforeTheToy is the subject guard,
// the same one event_count carries: a page_count naming no event has no page to
// read, and asking the toy for /ui/events/ would be a request about nothing.
func TestAPageObservationWithNoSubjectIsRefusedBeforeTheToy(t *testing.T) {
	a, log := stub(t, nil)
	kind, _ := faultOf(t, ask(t, a, map[string]any{
		"kind": "observe", "query": "page_count", "source": "browser", "token": "t-1",
	}))
	if kind != string(wire.FaultField) {
		t.Errorf("a subjectless page_count is fault %q, want %q", kind, wire.FaultField)
	}
	if len(*log) != 0 {
		t.Errorf("the adapter asked the toy anyway, %d time(s)", len(*log))
	}
}

// TestTheCountPatternDISCRIMINATES is the pair ratchet asked for at coop
// r8/160, and the distinction it rests on is theirs: eventPage carries ONE
// number pair and no decoy, so `<p id="count">([0-9]+)` and
// `([0-9]+) of ([0-9]+)` both read 2 off it. A pattern that agrees with itself
// on a page with one candidate says nothing about WHICH candidate it picks --
// so dropping the capacity requirement and dropping the id anchor entirely both
// stayed green while the positive above passed.
//
// FOURTH TIME THIS SHAPE FOR ME, and ratchet named what the first three had in
// common: every one was a predicate's DISCRIMINATION left unheld while its
// REASON sat in a comment. arm 21's `none`, cell 8's context, the gate's two
// arms, and now this. A reason is checkable only against an input it has to
// reject, and a fixture with nothing to reject supplies none.
//
// SO EACH CELL CARRIES THE CANDIDATE ITS PREDICATE MUST REFUSE, which is the
// whole difference between this test and the positive one above.
func TestTheCountPatternDISCRIMINATES(t *testing.T) {
	// A #count OF ANOTHER SHAPE. The element is there, carries a number, and is
	// NOT the pair this page's contract defines -- so the capacity requirement
	// is what refuses it, and without that requirement it would read 3 and the
	// walk would compare a bare attendance figure to a believed count as though
	// the template had not changed under it.
	const otherShape = `<!doctype html>
<h1 id="title">standup</h1>
<p id="count">3</p>
`
	// A DECOY PAIR BEFORE THE ELEMENT. An unanchored pattern reads 5 here, and
	// 5 is a number the page genuinely renders -- just not the one the reader is
	// being told is the count. This is the cell the id anchor exists for.
	const decoyFirst = `<!doctype html>
<h1 id="title">standup</h1>
<p>5 of 9</p>
<p id="count">2 of 10</p>
`
	for _, c := range []struct {
		what, page string
		want       float64 // 0 means: no value, the observation faults
	}{
		{"a #count of another shape yields no value", otherShape, 0},
		{"a decoy pair before the element is not the count", decoyFirst, 2},
	} {
		t.Run(c.what, func(t *testing.T) {
			a, _ := stub(t, map[string]answer{"GET /ui/events/e-1": served(c.page)})
			out := ask(t, a, map[string]any{
				"kind": "observe", "query": "page_count", "source": "browser", "token": "t-1",
				"args": map[string]any{"event": "e-1"},
			})
			if c.want == 0 {
				kind, detail := faultOf(t, out)
				if kind != string(wire.FaultInternal) {
					t.Errorf("fault %q, want %q", kind, wire.FaultInternal)
				}
				if !strings.Contains(detail, "#count") {
					t.Errorf("the fault does not name what was missing: %q", detail)
				}
				t.Logf("PROOF %s", c.what)
				return
			}
			got, ok := out["value"].(float64)
			if !ok || got != c.want {
				t.Errorf("the value is %#v, want %v -- the pattern read the wrong candidate",
					out["value"], c.want)
			}
			t.Logf("PROOF %s: read %v", c.what, got)
		})
	}
}

// TestAToyThatStartsAndNeverPrintsIsRefusedNotWaitedFor holds the bound on the
// readiness read, and the cell's INPUT is the thing the bound exists to refuse:
// a child that STARTS successfully and then never prints a line.
//
// WITHOUT THE BOUND THIS TEST DOES NOT FAIL -- IT HANGS, which is the whole
// shape of the defect. A bare ReadString waits for as long as the child
// declines to speak, and `open` runs before any request exists, so the engine's
// per-request budget never applies. The hang would be the adapter's, held by a
// toy that is alive and silent.
//
// A START THAT FAILS IS A DIFFERENT CELL and already exists above: that one
// returns from `cmd.Start()`. This one needs the start to SUCCEED, which is why
// the stub child sleeps rather than exiting -- an exiting child closes the pipe
// and ReadString returns an error, so an exit tests the error path and says
// nothing about the wait.
func TestAToyThatStartsAndNeverPrintsIsRefusedNotWaitedFor(t *testing.T) {
	dir := t.TempDir()
	silent := filepath.Join(dir, "silent-toy")
	// IT MUST OUTLIVE THE BOUND, so the sleep is far longer than the shortened
	// timeout below; the process is stopped by `open`'s own deferred cleanup.
	// `exec` AND NOT A BARE `sleep`, measured the hard way: without it the
	// shell is the child and the sleep is its GRANDCHILD, so close()'s
	// Process.Kill() reaches the shell alone. The orphaned sleep then holds the
	// stderr it inherited from this test binary, and `go test` waits for that
	// pipe -- the package took 30.007s with a passing test and a 209ms refusal
	// inside it. Kill does not reach descendants; `exec` removes the
	// descendant.
	script := "#!/bin/sh\nexec sleep 30\n"
	if err := os.WriteFile(silent, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	restore := startTimeout
	startTimeout = 200 * time.Millisecond
	t.Cleanup(func() { startTimeout = restore })

	start := time.Now()
	a, err := open(silent, filepath.Join(dir, "state.json"))
	elapsed := time.Since(start)
	if a != nil {
		t.Error("an adapter was returned for a toy that printed no address")
	}
	if err == nil {
		t.Fatal("a silent toy was accepted")
	}
	if !strings.Contains(err.Error(), "printed no address") {
		t.Errorf("the error does not name the expiry: %v", err)
	}
	// AND IT RETURNED ON THE BOUND RATHER THAN ON THE CHILD, which is the half
	// that distinguishes a timeout from a lucky exit: the script sleeps 30s and
	// this must come back in a fraction of that.
	if elapsed > 5*time.Second {
		t.Errorf("open took %s, so it waited for the child rather than for the bound", elapsed)
	}
	t.Logf("PROOF a toy that starts and stays silent is refused in %s, not waited for", elapsed)
}

// TestThePageReadersAreTheElementsAndNotThePage is ratchet's finding at coop
// r8/182: pageAttendees and pageAttendeesRe were in no _test.go at all, and
// neither was pageCount. The producer's half of "absent and empty must not read
// alike" is held in cmd/toy's suite; the consumer's was held by nothing.
//
// THE DOCUMENT IS CRAFTED AND THAT IS THE POINT. ratchet drove two survivors --
// counting `<li>` over the whole page, and a GREEDY
// `(?s)<ul id="attendees">(.*)</ul>` -- and both are EQUIVALENT to the committed
// spelling on a page with ONE list, which is the only page cmd/toy serves today.
// So no fixture taken from the toy can tell them apart, and the cell has to be a
// document written to separate them: two lists, one of them not the attendees.
//
//	the whole-page count   counts the other list's items too          -- reads 6
//	the greedy match       runs to the LAST </ul> and swallows both   -- reads 5
//
// AND THE SECOND RETURN IS THE CLAIM, not a convenience: a page with no
// #attendees element is a page this observation cannot read, and answering 0
// would make a removed element look like an event nobody has joined -- the
// honest reading the plant's finding has to be told apart from. The statute
// rests on that distinction at examples/toy/statute.rego:474, where an empty
// list is `consistent` for every role.
func TestThePageReadersAreTheElementsAndNotThePage(t *testing.T) {
	const twoLists = `<!doctype html>
<h1 id="title">a page with two lists</h1>
<p id="role">guest</p>
<p id="count">2 of 3</p>
<ul id="attendees">
<li>u-1</li>
<li>u-2</li>
</ul>
<ul id="elsewhere">
<li>not an attendee</li>
<li>nor this</li>
<li>nor this either</li>
</ul>
<li>a stray item outside every list</li>
`
	n, ok := pageAttendees(twoLists)
	if !ok {
		t.Fatalf("pageAttendees found no #attendees element in a page that carries one")
	}
	if n != 2 {
		t.Errorf("pageAttendees counted %d where the #attendees list holds 2 and the page holds "+
			"6 list items: a whole-page count reads 6 and a greedy match reads 5, and neither is "+
			"distinguishable from the committed form on the one-list page the toy serves", n)
	}

	// ABSENT IS NOT EMPTY, both ways round.
	const noList = `<!doctype html>
<p id="count">0 of 3</p>
`
	if n, ok := pageAttendees(noList); ok || n != 0 {
		t.Errorf("pageAttendees read (%d, %v) on a page with NO #attendees element, want (0, "+
			"false): reading 0 as an answer makes a removed element look like an event nobody "+
			"has joined", n, ok)
	}
	const emptyList = `<!doctype html>
<ul id="attendees">
</ul>
`
	if n, ok := pageAttendees(emptyList); !ok || n != 0 {
		t.Errorf("pageAttendees read (%d, %v) on an EMPTY #attendees element, want (0, true): an "+
			"empty list is an answer, and the statute's :474 clause turns on it", n, ok)
	}

	// AND THE COUNT READER, which had no cell either.
	if n, ok := pageCount(twoLists); !ok || n != 2 {
		t.Errorf("pageCount read (%d, %v) on `<p id=\"count\">2 of 3</p>`, want (2, true)", n, ok)
	}
	if n, ok := pageCount(noList); !ok || n != 0 {
		t.Errorf("pageCount read (%d, %v) on `<p id=\"count\">0 of 3</p>`, want (0, true): zero "+
			"rendered is an answer", n, ok)
	}
	const noCount = `<!doctype html>
<ul id="attendees">
<li>u-1</li>
</ul>
`
	if n, ok := pageCount(noCount); ok || n != 0 {
		t.Errorf("pageCount read (%d, %v) on a page with no #count element, want (0, false)", n, ok)
	}

	// THE CAPACITY IS REQUIRED ON PURPOSE, which pageCountRe's own comment claims
	// and nothing held: a `#count` element of another shape must not have a
	// number read out of it.
	if n, ok := pageCount(`<p id="count">4</p>`); ok || n != 0 {
		t.Errorf("pageCount read (%d, %v) out of `<p id=\"count\">4</p>`, want (0, false): the "+
			"`N of C` shape is cmd/toy's contract with this reader, and matching a bare number "+
			"would take the wrong one out of some other element that shares the id", n, ok)
	}
	// A PROOF LINE AFTER AN Errorf IS A CLAIM THE RUN JUST DISPROVED: a Logf
	// always runs, so the proof printed beside the failure that refutes it
	// (ratchet, coop r8/184).
	if !t.Failed() {
		t.Logf("PROOF both readers take their own element out of a two-list page: attendees 2 of a " +
			"page holding 6 list items, count 2 of 3, absent reads false where empty reads true, " +
			"and a `#count` without a capacity is not read at all")
	}
}
