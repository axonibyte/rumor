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
	"net/http"
	"net/http/httptest"
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
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(ans.code)
		_ = json.NewEncoder(w).Encode(ans.body)
	}))
	t.Cleanup(srv.Close)
	return &adapter{base: srv.URL, client: &http.Client{Timeout: 5 * time.Second}}, &log
}

// ask hands one request to the adapter and returns the answer as a map, which
// is how a fault and a typed response can be read by one helper.
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
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("the answer is not an object: %v", err)
	}
	return out
}

func faultOf(t *testing.T, out map[string]any) (string, string) {
	t.Helper()
	if out["kind"] != string(wire.KindError) {
		t.Fatalf("not a fault: %v", out)
	}
	f, ok := out["fault"].(map[string]any)
	if !ok {
		t.Fatalf("a fault with no fault object: %v", out)
	}
	kind, _ := f["kind"].(string)
	detail, _ := f["detail"].(string)
	return kind, detail
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

func TestAnObservationCarriesItsSubjectInTheQuery(t *testing.T) {
	a, log := stub(t, map[string]answer{
		"GET /events/e7/count": fine(map[string]any{"event": "e7", "count": 3}),
	})
	out := ask(t, a, map[string]any{
		"kind": "observe", "query": "event_count/e7", "source": "api", "token": "t1",
	})
	if out["kind"] != "observe" {
		t.Fatalf("not an observe: %v", out)
	}
	value, _ := out["value"].(map[string]any)
	if value["count"].(float64) != 3 {
		t.Errorf("the value is %v", out["value"])
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
	if !strings.Contains(detail, "event_count/<id>") {
		t.Errorf("the fault does not say the shape: %q", detail)
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
		"kind": "observe", "query": "event_count/e1", "source": "direct",
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
		"kind": "observe", "query": "store_event_count/e2", "source": "direct",
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
