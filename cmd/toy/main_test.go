// Package main's test holds the toy to the behavior its statute calls honest.
//
// IT WILL BE EDITED BY THE BUGS, AND THAT IS THE DESIGN. The three ledger bugs
// (R5, epic) land one commit each, and each flips the assertion here that its
// defect contradicts, naming the row it answers. So the diff of a bug's commit
// shows the behavior that changed and nothing else -- and a future reader who
// "fixes" the toy breaks a test that says, in words, that the defect is
// load-bearing. A deliberately buggy application with no such pin is one
// somebody repairs by accident, taking the shrinker's fixtures with it.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type harness struct {
	t   *testing.T
	ts  *httptest.Server
	dir string

	// srv is the handle the lag arms need: the drain is reached directly where
	// an arm wants it without going through a write, and queueDepth is read
	// without the HTTP round trip. There is no stop handle because there is no
	// goroutine -- the queue advances on the request sequence (main.go's
	// advancing, fable's ruling 1 at coop r6/78).
	srv *server
}

func start(t *testing.T) *harness {
	t.Helper()
	dir := t.TempDir()
	srv, err := newServer(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}
	ts := httptest.NewServer(srv.routes())
	t.Cleanup(ts.Close)
	return &harness{t: t, ts: ts, dir: dir, srv: srv}
}

// do sends a request and returns the status and the decoded body. A body that
// is not JSON is returned as a string under the key "raw", so an HTML page and
// an API error can be asserted by one helper.
func (h *harness) do(method, path, token string, body any) (int, map[string]any) {
	h.t.Helper()
	var payload io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			h.t.Fatalf("marshal: %v", err)
		}
		payload = strings.NewReader(string(raw))
	}
	req, err := http.NewRequest(method, h.ts.URL+path, payload)
	if err != nil {
		h.t.Fatalf("request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := h.ts.Client().Do(req)
	if err != nil {
		h.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		h.t.Fatalf("reading the body of %s %s: %v", method, path, err)
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		var list []any
		if err2 := json.Unmarshal(raw, &list); err2 == nil {
			return resp.StatusCode, map[string]any{"list": list}
		}
		return resp.StatusCode, map[string]any{"raw": string(raw)}
	}
	return resp.StatusCode, out
}

// actor creates a user and acquires an api session for it, which is the pair
// every other action needs and so is worth one helper.
func (h *harness) actor(name string) string {
	h.t.Helper()
	code, _ := h.do(http.MethodPost, "/users", "", map[string]any{"name": name})
	if code != http.StatusCreated {
		h.t.Fatalf("creating the user %s: status %d", name, code)
	}
	code, body := h.do(http.MethodPost, "/sessions", "", map[string]any{"actor": name, "context": "api"})
	if code != http.StatusCreated {
		h.t.Fatalf("session for %s: status %d", name, code)
	}
	tok, _ := body["token"].(string)
	if tok == "" {
		h.t.Fatalf("session for %s carried no token: %v", name, body)
	}
	return tok
}

func (h *harness) createEvent(token, title string, capacity int, visibleTo []string) string {
	h.t.Helper()
	id, _ := h.createEventFull(token, title, capacity, visibleTo)
	return id
}

// createEventFull hands back the created event as well as its id, for an arm
// that needs to read a field of it. The toy has NO `GET /events/<id>` route --
// handleEventPath serves delete, join, leave and count and nothing else -- so
// the create response is the only place a test can read a stored capacity from
// the application rather than from the literal it typed, and adding a route to
// the system under test to suit a test would be adding production surface for a
// test's convenience.
func (h *harness) createEventFull(token, title string, capacity int, visibleTo []string) (string, map[string]any) {
	h.t.Helper()
	code, body := h.do(http.MethodPost, "/events", token, map[string]any{
		"title": title, "capacity": capacity, "visible_to": visibleTo,
	})
	if code != http.StatusCreated {
		h.t.Fatalf("creating %q: status %d (%v)", title, code, body)
	}
	id, _ := body["id"].(string)
	if id == "" {
		h.t.Fatalf("creating %q returned no id: %v", title, body)
	}
	return id, body
}

func TestAUserIsCreatedOnceAndANameIsNotReused(t *testing.T) {
	h := start(t)
	code, body := h.do(http.MethodPost, "/users", "", map[string]any{"name": "alice"})
	if code != http.StatusCreated {
		t.Fatalf("first alice: status %d (%v)", code, body)
	}
	code, _ = h.do(http.MethodPost, "/users", "", map[string]any{"name": "alice"})
	if code != http.StatusConflict {
		t.Errorf("a second alice: status %d, want %d", code, http.StatusConflict)
	}
	code, _ = h.do(http.MethodPost, "/users", "", map[string]any{"name": "  "})
	if code != http.StatusBadRequest {
		t.Errorf("a blank name: status %d, want %d", code, http.StatusBadRequest)
	}
}

func TestASessionBelongsToAUserThatExists(t *testing.T) {
	h := start(t)
	code, _ := h.do(http.MethodPost, "/sessions", "", map[string]any{"actor": "nobody", "context": "api"})
	if code != http.StatusNotFound {
		t.Errorf("a session for an absent user: status %d, want %d", code, http.StatusNotFound)
	}
	_, _ = h.do(http.MethodPost, "/users", "", map[string]any{"name": "alice"})
	code, _ = h.do(http.MethodPost, "/sessions", "", map[string]any{"actor": "alice", "context": "telepathy"})
	if code != http.StatusBadRequest {
		t.Errorf("a session in an unknown context: status %d, want %d", code, http.StatusBadRequest)
	}
}

func TestAnEventNeedsASessionATitleAndACapacity(t *testing.T) {
	h := start(t)
	code, _ := h.do(http.MethodPost, "/events", "", map[string]any{"title": "x", "capacity": 1})
	if code != http.StatusUnauthorized {
		t.Errorf("no session: status %d, want %d", code, http.StatusUnauthorized)
	}
	tok := h.actor("alice")
	code, _ = h.do(http.MethodPost, "/events", tok, map[string]any{"title": "", "capacity": 1})
	if code != http.StatusBadRequest {
		t.Errorf("no title: status %d, want %d", code, http.StatusBadRequest)
	}
	code, _ = h.do(http.MethodPost, "/events", tok, map[string]any{"title": "x", "capacity": 0})
	if code != http.StatusBadRequest {
		t.Errorf("capacity 0: status %d, want %d", code, http.StatusBadRequest)
	}
	code, _ = h.do(http.MethodPost, "/events", tok, map[string]any{"title": "x", "capacity": 1, "visible_to": []string{"wizard"}})
	if code != http.StatusBadRequest {
		t.Errorf("an unknown role in visible_to: status %d, want %d", code, http.StatusBadRequest)
	}
}

// THE VISIBILITY RULE, which the ledger's "API visibility leak" row is about.
// The honest version asks visible_to for the role the CALLER holds.
func TestTheListShowsOnlyWhatTheCallersRoleMaySee(t *testing.T) {
	h := start(t)
	alice := h.actor("alice")
	bob := h.actor("bob")
	private := h.createEvent(alice, "members only", 2, []string{roleAdmin, roleMember})
	open := h.createEvent(alice, "open to all", 2, []string{roleAdmin, roleMember, roleGuest})

	code, body := h.do(http.MethodGet, "/events", bob, nil)
	if code != http.StatusOK {
		t.Fatalf("bob's list: status %d", code)
	}
	seen := idsOf(t, body)
	if seen[private] {
		t.Errorf("bob is a guest of %s and saw it: visible_to is %v", private, []string{roleAdmin, roleMember})
	}
	if !seen[open] {
		t.Errorf("bob is a guest of %s, which is visible to guests, and did not see it", open)
	}

	// JOIN AGREES WITH LIST. bob cannot see the member-only event, so he cannot
	// join it either: one answer about visibility, from both endpoints.
	if code, _ := h.do(http.MethodPost, "/events/"+private+"/join", bob, nil); code != http.StatusNotFound {
		t.Errorf("bob joining an event he cannot see: status %d, want %d", code, http.StatusNotFound)
	}
	if code, _ := h.do(http.MethodPost, "/events/"+open+"/join", bob, nil); code != http.StatusOK {
		t.Fatalf("bob joining %s: status %d", open, code)
	}
	code, body = h.do(http.MethodGet, "/events", alice, nil)
	if code != http.StatusOK {
		t.Fatalf("alice's list: status %d", code)
	}
	if seen := idsOf(t, body); !seen[private] || !seen[open] {
		t.Errorf("alice is admin of both and saw %v", seen)
	}
}

func idsOf(t *testing.T, body map[string]any) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	list, ok := body["list"].([]any)
	if !ok {
		t.Fatalf("the list endpoint did not return a list: %v", body)
	}
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("a list item is not an object: %v", item)
		}
		id, _ := m["id"].(string)
		out[id] = true
	}
	return out
}

// THE DEFAULT visible_to, WHICH NOTHING OVERRIDES AND SO NOTHING WAS PINNING.
// Every other arm here passes an explicit `visible_to`, which left
// createEvent's default branch exercised by no test at all: driven at 2187e7a,
// changing the default from {admin, member} to {admin, member, guest}, to
// {admin} or to {} left the WHOLE cmd/toy package silent. The first of those is
// the epic's visibility-leak row arriving by accident, on the one axis this SUT
// exists to be wrong about -- so the default needs an arm of its own, with no
// override set.
//
// AND ITS CONSEQUENCE IS PART OF THE RULE rather than a surprise: a defaulted
// event takes no new members ever, because joinEvent agrees with listEvents
// about visibility and a caller who has not joined holds the guest role. That
// is what "not public" means here, as joinEvent's own comment says -- there is
// no invitation in this domain.
func TestAnEventWithNoVisibleToIsAdminAndMemberOnly(t *testing.T) {
	h := start(t)
	alice := h.actor("alice")
	bob := h.actor("bob")

	// NO visible_to KEY AT ALL, which is why this does not use h.createEvent:
	// the helper always sends one, and the production path under test is the
	// one where the field is absent.
	code, body := h.do(http.MethodPost, "/events", alice, map[string]any{
		"title": "defaulted", "capacity": 2,
	})
	if code != http.StatusCreated {
		t.Fatalf("creating an event with no visible_to: status %d (%v)", code, body)
	}
	id, _ := body["id"].(string)
	if id == "" {
		t.Fatalf("the created event carried no id: %v", body)
	}
	raw, ok := body["visible_to"].([]any)
	if !ok {
		t.Fatalf("the created event carried no visible_to: %v", body)
	}
	var got []string
	for _, r := range raw {
		s, _ := r.(string)
		got = append(got, s)
	}
	// ERRORF AND NOT FATALF, so the endpoint half below runs too: an arm that
	// stopped here would rest entirely on the echoed field, and a field echo is
	// a string -- the thing rook's r5/10 finding was about.
	want := []string{roleAdmin, roleMember}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("the default visible_to is %v, want %v", got, want)
	}

	// AND THE DEFAULT IS READ THE SAME WAY BY THE ENDPOINTS, which is what
	// makes the list above more than a field echo.
	_, lst := h.do(http.MethodGet, "/events", bob, nil)
	if idsOf(t, lst)[id] {
		t.Errorf("bob is a guest of the defaulted event and saw it: the default is not public")
	}
	if code, _ := h.do(http.MethodPost, "/events/"+id+"/join", bob, nil); code != http.StatusNotFound {
		t.Errorf("bob joining the defaulted event: status %d, want %d", code, http.StatusNotFound)
	}
	_, lst = h.do(http.MethodGet, "/events", alice, nil)
	if !idsOf(t, lst)[id] {
		t.Errorf("alice is the admin of the defaulted event and did not see it")
	}
}

// THE CAPACITY RULE, which the ledger's "capacity race" row is about. Sequential
// joins at the boundary are refused by the honest version AND by the buggy one:
// the race needs two joins in flight at once, which is R5.3's drill rather than
// this test's, so what is pinned here is the rule the race violates.
func TestJoiningStopsAtCapacity(t *testing.T) {
	h := start(t)
	alice := h.actor("alice")
	bob := h.actor("bob")
	carol := h.actor("carol")
	id := h.createEvent(alice, "one seat", 1, []string{roleAdmin, roleMember, roleGuest})

	if code, body := h.do(http.MethodPost, "/events/"+id+"/join", bob, nil); code != http.StatusOK {
		t.Fatalf("bob joining: status %d (%v)", code, body)
	}
	if code, _ := h.do(http.MethodPost, "/events/"+id+"/join", bob, nil); code != http.StatusConflict {
		t.Errorf("bob joining twice: status %d, want %d", code, http.StatusConflict)
	}
	if code, _ := h.do(http.MethodPost, "/events/"+id+"/join", carol, nil); code != http.StatusConflict {
		t.Errorf("carol joining a full event: status %d, want %d", code, http.StatusConflict)
	}
	h.settle(t)
	if got, want := h.count(id, alice), 1; got != want {
		t.Errorf("the API count is %d, want %d", got, want)
	}
	if got, want := h.storeCount(id), 1; got != want {
		t.Errorf("the store's count is %d, want %d", got, want)
	}
}

// settle waits for the recount queue to empty, which is what every arm below
// that reads the API count now has to do.
//
// WHY THE ARMS MOVED AT R6.3 AND WHAT THEY ASSERT INSTEAD. The count became
// EVENTUALLY consistent: a join enqueues a recount and the worker applies it,
// so an arm reading the API count in the next breath reads the old one. That is
// the phase's whole point -- a lagging count is a `pending` on an eventual
// claim type and not a defect -- so the arms wait for the system to settle
// rather than asserting a weaker thing about it. Every value they required
// before, they require after.
//
// AND THE WAIT IS THE SYSTEM'S OWN SIGNAL, not a sleep: it polls the depth the
// `direct` source reports, which is the same number the engine's quiesce probe
// reads. A fixed sleep would be a guess about the worker's tick and would pass
// for the wrong reason on a loaded box.
//
// IT FAILS THE TEST RATHER THAN HANGING. A queue that never empties is this
// phase's `stale` row, and an arm that waited for ever would report it as a
// test that hung.
func (h *harness) settle(t *testing.T) {
	t.Helper()
	// CLOCK-FREE, AND THE BOUND IS A COUNT. A deadline in seconds was the right
	// instrument for a goroutine on a timer and is the wrong one now: the queue
	// advances on the request sequence, so there is nothing to wait FOR, and a
	// wall-clock bound in a suite that pins determinism is the shape fable's
	// ruling 1 (coop r6/78) removed from the toy itself.
	//
	// IT DRAINS EXACTLY THE DEPTH IT READ, then asserts empty. The assertion is
	// not a formality: a drain that enqueued more work would leave the queue
	// non-empty here, and that is a finding about the toy rather than a settle
	// that needs another pass.
	depth := h.srv.queueDepth()
	for i := 0; i < depth; i++ {
		h.srv.mu.Lock()
		h.srv.drainOne()
		h.srv.mu.Unlock()
	}
	if left := h.srv.queueDepth(); left != 0 {
		t.Fatalf("after draining the %d job(s) the queue held, %d remain: a drain enqueued "+
			"more work, which is a FINDING about the toy and not a settle that needs "+
			"another pass", depth, left)
	}
}

func (h *harness) count(id, token string) int {
	h.t.Helper()
	code, body := h.do(http.MethodGet, "/events/"+id+"/count", token, nil)
	if code != http.StatusOK {
		h.t.Fatalf("the count of %s: status %d (%v)", id, code, body)
	}
	n, ok := body["count"].(float64)
	if !ok {
		h.t.Fatalf("the count of %s is not a number: %v", id, body)
	}
	return int(n)
}

func (h *harness) storeCount(id string) int {
	h.t.Helper()
	code, body := h.do(http.MethodGet, "/store/events/"+id+"/count", "", nil)
	if code != http.StatusOK {
		h.t.Fatalf("the store count of %s: status %d (%v)", id, code, body)
	}
	n, ok := body["count"].(float64)
	if !ok {
		h.t.Fatalf("the store count of %s is not a number: %v", id, body)
	}
	return int(n)
}

func TestLeavingGivesTheSeatBack(t *testing.T) {
	h := start(t)
	alice := h.actor("alice")
	bob := h.actor("bob")
	id := h.createEvent(alice, "one seat", 1, []string{roleAdmin, roleMember, roleGuest})
	if code, _ := h.do(http.MethodPost, "/events/"+id+"/join", bob, nil); code != http.StatusOK {
		t.Fatalf("bob joining")
	}
	if code, _ := h.do(http.MethodDelete, "/events/"+id+"/join", bob, nil); code != http.StatusOK {
		t.Fatalf("bob leaving")
	}
	if code, _ := h.do(http.MethodDelete, "/events/"+id+"/join", bob, nil); code != http.StatusConflict {
		t.Errorf("leaving twice should conflict")
	}
	h.settle(t)
	if got := h.count(id, alice); got != 0 {
		t.Errorf("the API count after leaving is %d, want 0", got)
	}
	if got := h.storeCount(id); got != 0 {
		t.Errorf("the store count after leaving is %d, want 0", got)
	}
}

// THE DELETE RULE, AND THE STALE COUNT THE LEDGER ASKS FOR. The admin-only
// check and the list exclusion are honest; the COUNT is the epic's "stale count
// after delete" row, so this test asserts the defect and names it. The two
// oracles are the point: the API reports the old attendance and the store,
// computing from the attendee list, reports none. R5.3 finds that disagreement
// from a seed and shrinks it.
func TestOnlyTheAdminDeletesAndTheCountIsLeftStale(t *testing.T) {
	h := start(t)
	alice := h.actor("alice")
	bob := h.actor("bob")
	id := h.createEvent(alice, "doomed", 2, []string{roleAdmin, roleMember, roleGuest})
	if code, _ := h.do(http.MethodPost, "/events/"+id+"/join", bob, nil); code != http.StatusOK {
		t.Fatalf("bob joining")
	}
	if code, _ := h.do(http.MethodDelete, "/events/"+id, bob, nil); code != http.StatusForbidden {
		t.Errorf("bob is a member, not the admin: status %d, want %d", code, http.StatusForbidden)
	}
	// THE VALUE BEFORE, so "stale" is about a number that STAYED rather than
	// about a coincidence: an arm that only read the count after the delete
	// would pass if the count had been 1 for the wrong reason, or 0 all along.
	h.settle(t)
	if got := h.count(id, alice); got != 1 {
		t.Fatalf("the count before the delete is %d, want 1: this arm is about a count the delete "+
			"LEAVES, which needs a count to leave", got)
	}
	if code, _ := h.do(http.MethodDelete, "/events/"+id, alice, nil); code != http.StatusOK {
		t.Fatalf("alice deleting her own event")
	}
	// THE ROW: the API still reports the attendee the deleted event used to have.
	if got := h.count(id, alice); got != 1 {
		t.Errorf("the API count after delete is %d, want 1 -- the epic's stale-count row requires "+
			"the denormalized counter to be LEFT, and R5.3 shrinks a seed that finds it", got)
	}
	// AND THE SECOND ORACLE DISAGREES, which is what makes it findable: the
	// store computes from the attendee list, which the delete did empty.
	if got := h.storeCount(id); got != 0 {
		t.Errorf("the store count after delete is %d, want 0: without the disagreement there is "+
			"nothing for a run to see", got)
	}
	code, body := h.do(http.MethodGet, "/events", alice, nil)
	if code != http.StatusOK {
		t.Fatalf("the list after delete: status %d", code)
	}
	if idsOf(t, body)[id] {
		t.Errorf("the deleted event %s is still listed", id)
	}
}

// THE LEAK IS PINNED, not merely present. The epic's "API visibility leak" row
// lives in eventCount, which answers any session without asking whether its
// role may see the event -- so a guest reads the attendance of an event the
// list correctly hides from them. This test asserts the DEFECT, naming the row,
// because a deliberately buggy application with nothing holding its bugs in
// place is one somebody repairs by accident, taking the shrinker's fixtures
// with it.
//
// AND IT PINS THE INCONSISTENCY, which is the part a reader needs: the same
// actor, the same event, two endpoints, two answers. Fixing eventCount would
// make this fail and say why.
func TestTheCountLeaksToARoleTheListHidesItFrom(t *testing.T) {
	h := start(t)
	alice := h.actor("alice")
	bob := h.actor("bob")
	id := h.createEvent(alice, "members only", 2, []string{roleAdmin, roleMember})

	// The list is correct: bob is a guest of this event and cannot see it.
	code, body := h.do(http.MethodGet, "/events", bob, nil)
	if code != http.StatusOK {
		t.Fatalf("bob's list: status %d", code)
	}
	if idsOf(t, body)[id] {
		t.Fatalf("the list leaked %s to a guest; this test is about the COUNT leaking "+
			"where the list does not", id)
	}

	// The count is not: it answers him anyway. That is the ledger row.
	code, body = h.do(http.MethodGet, "/events/"+id+"/count", bob, nil)
	if code != http.StatusOK {
		t.Fatalf("the count refused a guest with status %d -- the epic's API visibility leak row "+
			"requires it to ANSWER, and R5.3 shrinks a seed that finds it", code)
	}
	if _, ok := body["count"].(float64); !ok {
		t.Errorf("the count answered a guest without a count: %v", body)
	}
}

func TestThePageRendersTheEventAndItsJoinFormWorks(t *testing.T) {
	h := start(t)
	alice := h.actor("alice")
	id := h.createEvent(alice, "a <scripted> title", 3, []string{roleAdmin, roleMember, roleGuest})

	code, body := h.do(http.MethodGet, "/ui/events/"+id, alice, nil)
	if code != http.StatusOK {
		t.Fatalf("the page: status %d (%v)", code, body)
	}
	page, _ := body["raw"].(string)
	for _, want := range []string{
		`id="title"`, `id="role"`, `id="count"`, `id="join"`,
		"a &lt;scripted&gt; title", // escaped, so the page cannot be a script
		">" + roleAdmin + "<",
		"0 of 3",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the page does not carry %q:\n%s", want, page)
		}
	}
	if strings.Contains(page, "<script") {
		t.Errorf("the page carries a script, and R9 drives it with a real browser:\n%s", page)
	}

	bob := h.actor("bob")
	if code, _ := h.do(http.MethodPost, "/ui/events/"+id+"/join?token="+bob, "", nil); code != http.StatusOK {
		t.Fatalf("joining through the form: status %d", code)
	}
	h.settle(t)
	if got := h.count(id, alice); got != 1 {
		t.Errorf("after the form join the count is %d, want 1", got)
	}
}

func TestThePageIsNotShownToARoleItIsNotVisibleTo(t *testing.T) {
	h := start(t)
	alice := h.actor("alice")
	bob := h.actor("bob")
	id := h.createEvent(alice, "members only", 2, []string{roleAdmin, roleMember})
	if code, _ := h.do(http.MethodGet, "/ui/events/"+id, bob, nil); code != http.StatusNotFound {
		t.Errorf("the page for a guest: status %d, want %d", code, http.StatusNotFound)
	}
}

// THE WORLD IS THE FILE, which is what makes the rollback provider possible: a
// second server over the same path sees exactly what the first one wrote, and
// nothing the first held in memory.
func TestTheWorldSurvivesAReopenBecauseItIsAllInTheFile(t *testing.T) {
	h := start(t)
	alice := h.actor("alice")
	id := h.createEvent(alice, "persisted", 2, []string{roleAdmin, roleMember, roleGuest})

	path := filepath.Join(h.dir, "state.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the state file was never written: %v", err)
	}
	if !strings.Contains(string(raw), "persisted") {
		t.Errorf("the state file does not carry the event:\n%s", raw)
	}

	again, err := newServer(path)
	if err != nil {
		t.Fatalf("reopening: %v", err)
	}
	ts2 := httptest.NewServer(again.routes())
	defer ts2.Close()
	h2 := &harness{t: t, ts: ts2, dir: h.dir}
	code, body := h2.do(http.MethodGet, "/events", alice, nil)
	if code != http.StatusOK {
		t.Fatalf("the reopened list: status %d (%v)", code, body)
	}
	if !idsOf(t, body)[id] {
		t.Errorf("the reopened server does not know event %s: %v", id, body)
	}
}

func TestAnUnreadableWorldIsRefusedRatherThanStartedEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := newServer(path); err == nil {
		t.Error("an undecodable state file started empty, which would discard the world silently")
	}
	// A FIELD NOBODY WROTE IS REFUSED TOO, for the reason docs/coverage.toml's
	// parser gives: a lenient reader turns a typo into silence.
	if err := os.WriteFile(path, []byte(`{"users":{},"events":{},"sessions":{},"seq":0,"sorce":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := newServer(path); err == nil {
		t.Error("an unknown field in the state file was accepted")
	}
}

func TestAnAbsentWorldIsAnEmptyOneAndNotAnError(t *testing.T) {
	dir := t.TempDir()
	srv, err := newServer(filepath.Join(dir, "nothing-here.json"))
	if err != nil {
		t.Fatalf("an absent state file: %v", err)
	}
	if n := len(srv.st.Users); n != 0 {
		t.Errorf("a fresh world holds %d user(s)", n)
	}
}

// THE SAVE IS ATOMIC AND LEAVES NOTHING BEHIND, because a temp file left in the
// directory would be a path docs --check refuses and, worse, a file the rollback
// provider's verify would have to learn to ignore.
func TestSavingLeavesNoTempFilesBehind(t *testing.T) {
	h := start(t)
	alice := h.actor("alice")
	for i := 0; i < 3; i++ {
		h.createEvent(alice, fmt.Sprintf("e%d", i), 2, []string{roleAdmin})
	}
	entries, err := os.ReadDir(h.dir)
	if err != nil {
		t.Fatal(err)
	}
	var leftovers []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".toy-state-") {
			leftovers = append(leftovers, e.Name())
		}
	}
	if len(leftovers) > 0 {
		t.Errorf("the save left %d temp file(s) behind: %v", len(leftovers), leftovers)
	}
}

// TestTheCapacityRaceIsPresentWhenDrivenConcurrently is the by-hand test fable
// required of C1 (coop breakout-r5/1, ruling 3): R5 PLANTS the race and cannot
// catch it, because one action at a time cannot race and the ledger row's
// catcher is `concurrency`, which is R8's. So the defect is demonstrated here
// rather than promised, and R8 inherits a bug with a witness.
//
// WHAT IT ASSERTS IS THE DEFECT, NOT THE RULE. Two joins on a one-seat event,
// in flight together, BOTH land -- so the attendee list passes capacity. The
// sequential rule is pinned separately by TestJoiningStopsAtCapacity, and the
// pair is the point: the same endpoint is correct in sequence and wrong under
// overlap, which is what makes `concurrency` the stage that can see it.
//
// NO t.Fatalf FROM A GOROUTINE: results come back over a channel and every
// assertion runs on the test's own goroutine, because testing's Fatal family is
// documented as the test goroutine's alone.
func TestTheCapacityRaceIsPresentWhenDrivenConcurrently(t *testing.T) {
	h := start(t)
	alice := h.actor("alice")
	bob := h.actor("bob")
	carol := h.actor("carol")
	id, created := h.createEventFull(alice, "one seat, two claimants", 1,
		[]string{roleAdmin, roleMember, roleGuest})
	joiners := []struct{ name, token string }{{"bob", bob}, {"carol", carol}}

	// THE ARM'S OWN REACH, ASSERTED RATHER THAN ASSUMED. This arm can see the
	// race only while more joins are in flight than there are seats. Driven at
	// 2187e7a with the fixture's capacity raised from 1 to 2: every assertion
	// below passed with the window CLOSED -- the defect gone, the arm silent,
	// and the arm's own message still claiming a one-seat event. That is rook's
	// r5/11 shape, and fable's r5/16 reading of it is the rule the gate now
	// applies to every R5 battery: an arm whose reach depends on a fixture
	// property asserts that property.
	seats, ok := created["capacity"].(float64)
	if !ok {
		t.Fatalf("the created event carries no numeric capacity: %v", created)
	}
	if int(seats) >= len(joiners) {
		t.Fatalf("this arm needs fewer seats than claimants to reach the window: capacity is %d "+
			"with %d join(s) in flight, so both could land honestly and a CLOSED race would pass "+
			"here", int(seats), len(joiners))
	}

	type result struct {
		who  string
		code int
		err  error
	}
	results := make(chan result, len(joiners))
	for _, who := range joiners {
		go func(name, token string) {
			req, err := http.NewRequest(http.MethodPost, h.ts.URL+"/events/"+id+"/join", nil)
			if err != nil {
				results <- result{name, 0, err}
				return
			}
			req.Header.Set("Authorization", "Bearer "+token)
			resp, err := h.ts.Client().Do(req)
			if err != nil {
				results <- result{name, 0, err}
				return
			}
			defer func() { _ = resp.Body.Close() }()
			_, _ = io.Copy(io.Discard, resp.Body)
			results <- result{name, resp.StatusCode, nil}
		}(who.name, who.token)
	}

	landed := 0
	for i := 0; i < len(joiners); i++ {
		r := <-results
		if r.err != nil {
			t.Fatalf("%s joining: %v", r.who, r.err)
		}
		if r.code == http.StatusOK {
			landed++
		}
	}

	if landed != len(joiners) {
		t.Fatalf("%d of %d concurrent joins landed, want all of them: the capacity race is the "+
			"epic's R5 ledger row and must be PRESENT in the toy, so joins in flight past the "+
			"seat count have to overrun it. If this fails, the window in joinEvent was closed and "+
			"R8 has nothing to catch", landed, len(joiners))
	}
	h.settle(t)
	if got := h.storeCount(id); got != len(joiners) {
		t.Errorf("the store holds %d attendee(s) on a %d-seat event, want %d: every join landed "+
			"but the list did not grow by that many", got, int(seats), len(joiners))
	} else if got <= int(seats) {
		t.Errorf("the store holds %d attendee(s) within the %d seat(s) declared: the OVERRUN is "+
			"the defect, so an arm whose list stays inside capacity has not seen it", got, int(seats))
	}
	if got := h.count(id, alice); got != len(joiners) {
		t.Errorf("the API reports %d on a %d-seat event, want %d", got, int(seats), len(joiners))
	}
}

// recount asks for an event's attendance to be recomputed and returns the
// RECOUNT's own id, which is what the worker's number is then read about.
func (h *harness) recount(id, token string) string {
	h.t.Helper()
	code, body := h.do(http.MethodPost, "/events/"+id+"/recount", token, nil)
	if code != http.StatusOK {
		h.t.Fatalf("asking for a recount of %s: status %d (%v)", id, code, body)
	}
	rid, ok := body["id"].(string)
	if !ok || rid == "" {
		h.t.Fatalf("the recount of %s has no id: %v", id, body)
	}
	return rid
}

// tally reads the worker's own count through the recount that asked for it.
// This is an ordinary actor read: it is served the number the worker has, and
// it advances the queue by one AFTER answering.
func (h *harness) tally(rid, token string) int {
	h.t.Helper()
	code, body := h.do(http.MethodGet, "/recounts/"+rid+"/tally", token, nil)
	if code != http.StatusOK {
		h.t.Fatalf("the tally of recount %s: status %d (%v)", rid, code, body)
	}
	n, ok := body["tally"].(float64)
	if !ok {
		h.t.Fatalf("the tally of recount %s is not a number: %v", rid, body)
	}
	return int(n)
}

// TestTheWorkersTallyLagsAndTheApiCountDoesNot is the arm for R6.3's whole
// point, and without it every other arm in this file passes against a
// synchronous worker.
//
// TWO COUNTS WITH TWO TIMINGS, which is fable's ruling at coop r6/82. The API's
// count is R5's number and moves with the write; the worker's tally is R6's and
// is maintained by the recount queue, so it is the number as of the last
// recount APPLIED. This arm asserts both halves in one place, because the cut
// that made one field carry both roles is what turned seed 40's R5 finding into
// an R6 one.
//
// THERE IS NOTHING TO STOP AND NOTHING TO RACE: the queue advances on the
// request sequence (fable's ruling 1, coop r6/78), so the lag is a fact of the
// request ORDER and not a timing window.
func TestTheWorkersTallyLagsAndTheApiCountDoesNot(t *testing.T) {
	h := start(t)

	alice := h.actor("alice")
	// VISIBLE TO EVERY ROLE, because an event with no `visible_to` is admin and
	// member only and bob would be hidden from it -- a 404 about visibility
	// would look like a worker fault here.
	id := h.createEvent(alice, "lag", 4, []string{roleAdmin, roleMember, roleGuest})
	bob := h.actor("bob")
	if code, body := h.do(http.MethodPost, "/events/"+id+"/join", bob, nil); code != http.StatusOK {
		t.Fatalf("bob joining: status %d (%v)", code, body)
	}

	// R5's NUMBER IS CURRENT IMMEDIATELY, both the API's and the store's, and a
	// join enqueues NOTHING: the worker's number is the recount's business.
	if got := h.storeCount(id); got != 1 {
		t.Errorf("the store's computed count is %d, want 1", got)
	}
	if got := h.count(id, alice); got != 1 {
		t.Errorf("the API count is %d, want 1: it is maintained by the write that changes it, "+
			"and a lag here would turn R5's rows into R6 findings", got)
	}
	if got := h.srv.queueDepth(); got != 0 {
		t.Fatalf("a join left %d job(s) queued: the tally is maintained by recounts alone, and "+
			"a job per join could never leave the queue deeper than one in a walk", got)
	}

	// ASKING FOR A RECOUNT QUEUES ONE JOB AND APPLIES NOTHING YET.
	rid := h.recount(id, alice)
	code, body := h.do(http.MethodGet, "/store/queue", "", nil)
	if code != http.StatusOK {
		t.Fatalf("reading the queue: status %d (%v)", code, body)
	}
	if d, _ := body["depth"].(float64); d != 1 {
		t.Errorf("the queue depth is %v, want 1: a depth that under-reports is a probe that "+
			"reads quiet while work remains", body["depth"])
	}
	if idle, _ := body["idle"].(bool); idle {
		t.Error("the queue reports idle with a job outstanding")
	}

	// AND THE PROBE READS DID NOT MOVE IT, which is handleQueue's own rule: "a
	// probe that advanced the system it measures would make quiet a thing the
	// asking caused".
	if got := h.srv.queueDepth(); got != 1 {
		t.Fatalf("the depth is %d after a `direct` read, want 1: a probe endpoint drained", got)
	}

	// THE WORKER'S NUMBER IS BEHIND, read off the server so the assertion
	// itself moves nothing -- an HTTP read advances the queue after answering,
	// which is the next thing this arm checks.
	h.srv.mu.Lock()
	lagged := h.srv.st.Events[id].Tally
	h.srv.mu.Unlock()
	if lagged != 0 {
		t.Errorf("the worker's tally is %d before any read, want 0: the recount is still "+
			"queued, and if it has been applied then the ask is synchronous", lagged)
	}

	// THE FIRST READ IS SERVED THE OLD NUMBER AND THEN ADVANCES THE QUEUE,
	// which is the whole mechanism: a drain in front of the read would serve
	// the freshly-applied tally and no observation could ever see the lag.
	if got := h.tally(rid, alice); got != 0 {
		t.Errorf("the first tally read is %d, want 0: it must be served the number the worker "+
			"has, not the one it is about to compute", got)
	}
	if got := h.srv.queueDepth(); got != 0 {
		t.Fatalf("the depth is %d after a read, want 0: a read must advance the queue by one, "+
			"or a run whose last step asks for a recount can never go quiet", got)
	}
	if got := h.tally(rid, alice); got != 1 {
		t.Errorf("the second tally read is %d, want 1: the first read advanced the queue, so "+
			"the recount has been applied", got)
	}

	// SETTLED AND IDLE, the end state this arm has always pinned.
	code, body = h.do(http.MethodGet, "/store/queue", "", nil)
	if code != http.StatusOK {
		t.Fatalf("reading the queue at the end: status %d (%v)", code, body)
	}
	if d, _ := body["depth"].(float64); d != 0 {
		t.Errorf("the queue depth at the end is %v, want 0", body["depth"])
	}
	if idle, _ := body["idle"].(bool); !idle {
		t.Error("the queue does not report idle with nothing outstanding")
	}
}

// TestASecondRecountIsDroppedAndTheTallyNeverSettles is the R6 ledger row: a
// retry dropped, a count that never settles.
//
// WHAT IT PINS: the worker applies the FIRST recount for an event and treats
// every later one as redundant, so once the attendee list moves after that
// first recount the tally is wrong for good -- while R5's numbers, the API's
// count and the store's, stay right. That last half is what makes this an R6
// row and not a second reading of an R5 one.
//
// IT IS THE HALF A WALK CANNOT SHOW ON ITS OWN. A walk reaches this through the
// statute and reports a finding; this arm reads the toy's own fields, so a
// failure here says the PLANT is gone where a walk's failure could equally mean
// the engine, the bindings or the adapter moved.
func TestASecondRecountIsDroppedAndTheTallyNeverSettles(t *testing.T) {
	h := start(t)
	alice := h.actor("alice")
	id := h.createEvent(alice, "dropped", 4, []string{roleAdmin, roleMember, roleGuest})
	bob := h.actor("bob")
	carol := h.actor("carol")

	if code, body := h.do(http.MethodPost, "/events/"+id+"/join", bob, nil); code != http.StatusOK {
		t.Fatalf("bob joining: status %d (%v)", code, body)
	}

	// THE FIRST RECOUNT IS APPLIED and the tally is right.
	first := h.recount(id, alice)
	h.settle(t)
	if got := h.tally(first, alice); got != 1 {
		t.Fatalf("after the first recount the tally is %d, want 1: the row is about the SECOND "+
			"ask, so a first that does not work would make this arm prove nothing", got)
	}

	// THE WORLD MOVES.
	if code, body := h.do(http.MethodPost, "/events/"+id+"/join", carol, nil); code != http.StatusOK {
		t.Fatalf("carol joining: status %d (%v)", code, body)
	}

	// AND THE SECOND RECOUNT IS DROPPED. It is queued -- the depth says so, so
	// a probe reads busy and the ask is not refused -- and draining it applies
	// nothing.
	second := h.recount(id, alice)
	if got := h.srv.queueDepth(); got != 1 {
		t.Fatalf("the second recount left %d job(s) queued, want 1: the row is that the job is "+
			"DROPPED BY THE WORKER, not that it was never queued", got)
	}
	h.settle(t)
	if got := h.srv.queueDepth(); got != 0 {
		t.Fatalf("the queue is %d deep after settling, want 0", got)
	}
	if got := h.tally(second, alice); got != 1 {
		t.Errorf("the tally is %d after the second recount, want 1: the second ask must be "+
			"dropped, leaving the number the first recount computed", got)
	}

	// R5's NUMBERS ARE RIGHT, which is what makes this an R6 row.
	if got := h.count(id, alice); got != 2 {
		t.Errorf("the API count is %d, want 2: the dropped retry must not touch R5's number", got)
	}
	if got := h.storeCount(id); got != 2 {
		t.Errorf("the store's computed count is %d, want 2", got)
	}

	// AND NO LATER ASK REPAIRS IT, which is the "never settles" half: every
	// recount after the first is dropped, so the gap survives any number of
	// them.
	third := h.recount(id, alice)
	h.settle(t)
	if got := h.tally(third, alice); got != 1 {
		t.Errorf("a third recount moved the tally to %d: the row is that it never settles", got)
	}
}

// TestTheWorldsClockMovesONLYWhenToldAndAnswersWhatItHolds is R7.4's first toy
// arm: the application gets a notion of time, and it is the world's.
//
// THE LADDER IS WHAT MAKES `advance` MEAN ANYTHING, and it ran against the
// ADAPTER until this commit: a server that echoed the request would pass a
// `set` and the FIRST `advance` and be wrong from the second on, so the cells
// run in one order against one world. They moved here because the number
// moved here.
func TestTheWorldsClockMovesONLYWhenToldAndAnswersWhatItHolds(t *testing.T) {
	h := start(t)
	if h.srv.st.Now != 0 {
		t.Errorf("a fresh world reads %d, want 0: the epoch is zero at the first seal", h.srv.st.Now)
	}
	for _, c := range []struct {
		op   string
		t    int64
		want int64
	}{
		{"set", 5000000000, 5000000000},
		{"advance", 30000000000, 35000000000},
		{"advance", 30000000000, 65000000000},
		{"set", 1000000000, 1000000000},
	} {
		code, body := h.do(http.MethodPost, "/clock", "", map[string]any{"op": c.op, "t": c.t})
		if code != http.StatusOK {
			t.Fatalf("%s %d: status %d (%v)", c.op, c.t, code, body)
		}
		// JSON numbers decode as float64; the comparison is on the int64 the
		// protocol declares, not on whatever the round trip produced.
		got, ok := body["t"].(float64)
		if !ok {
			t.Fatalf("%s %d: the answer carries no `t`: %v", c.op, c.t, body)
		}
		if int64(got) != c.want {
			t.Errorf("%s %d: answered %d, want %d", c.op, c.t, int64(got), c.want)
		}
		if h.srv.st.Now != c.want {
			t.Errorf("%s %d: the world holds %d, want %d", c.op, c.t, h.srv.st.Now, c.want)
		}
	}
}

// TestTheClockIsInTheStateFileAndSurvivesAReload is the rollback property, and
// it is an ARM rather than a comment because the provider copies the file and
// reads nothing: a clock held outside it would survive a restore, so the world
// after one would be the checkpoint PLUS however far time had been moved.
//
// RELOADED THROUGH newServer AND NOT BY UNMARSHALLING HERE: a test that
// decoded the bytes itself would assert the field is written and say nothing
// about whether the server reads it back.
func TestTheClockIsInTheStateFileAndSurvivesAReload(t *testing.T) {
	h := start(t)
	if code, body := h.do(http.MethodPost, "/clock", "",
		map[string]any{"op": "set", "t": 42000000000}); code != http.StatusOK {
		t.Fatalf("setting the clock: status %d (%v)", code, body)
	}
	path := filepath.Join(h.dir, "state.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the state file: %v", err)
	}
	var onDisk struct {
		Now int64 `json:"now"`
	}
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatalf("decoding the state file: %v", err)
	}
	if onDisk.Now != 42000000000 {
		t.Errorf("the state file holds now=%d, want 42000000000: a checkpoint is a copy of "+
			"this file, so a clock it does not carry is a clock a restore cannot put back", onDisk.Now)
	}
	again, err := newServer(path)
	if err != nil {
		t.Fatalf("reloading the world: %v", err)
	}
	if again.st.Now != 42000000000 {
		t.Errorf("a reloaded world reads %d, want 42000000000", again.st.Now)
	}
}

// TestAClockMoveIsRefusedOrInertWhereItShouldBe holds the three things around
// the move: a word that is not an op, a method that is not POST, and the
// queue, which a clock move must NOT advance.
//
// THE QUEUE CELL IS THE ONE WORTH THE LINES. `advancing` steps the recount
// worker after a GET, so if a clock MOVE drained one, moving time would also
// move the worker and R6's lag and R7's deadline could not be exercised
// independently. The depth is read either side of the move, so the cell fails
// whichever way such a coupling arrived.
//
// AND THE 405 PROBE BELOW DOES DRAIN ONE, WHICH THIS CELL MEASURED THE HARD
// WAY: `advancing` reads the METHOD and not the outcome, so a GET to a
// POST-only route steps the worker after being refused. That is true of every
// route in this file and not of this one -- `GET /users` is a 405 that drains
// too -- and it costs the design nothing, because the queue advances on the
// REQUEST SEQUENCE and a refused request is one. It is written down because
// my first draft of this cell read the depth before the probe and blamed the
// move for the probe's drain. The depth is therefore taken AFTER the probe.
func TestAClockMoveIsRefusedOrInertWhereItShouldBe(t *testing.T) {
	h := start(t)
	tok := h.actor("alice")
	id := h.createEvent(tok, "launch", 10, nil)
	h.recount(id, tok)

	code, body := h.do(http.MethodPost, "/clock", "", map[string]any{"op": "rewind", "t": 1})
	if code != http.StatusBadRequest {
		t.Errorf("an unknown op is status %d, want 400 (%v)", code, body)
	}
	msg, _ := body["error"].(string)
	for _, want := range []string{"rewind", "set", "advance"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the refusal does not name %q: %q", want, msg)
		}
	}
	if h.srv.st.Now != 0 {
		t.Errorf("a refused op moved the world to %d", h.srv.st.Now)
	}

	if code, _ := h.do(http.MethodGet, "/clock", "", nil); code != http.StatusMethodNotAllowed {
		t.Errorf("GET /clock is status %d, want 405", code)
	}

	h.recount(id, tok)
	before := h.srv.queueDepth()
	if before == 0 {
		t.Fatalf("the queue is empty before the clock move, so this cell cannot see a drain")
	}
	if code, body := h.do(http.MethodPost, "/clock", "",
		map[string]any{"op": "advance", "t": 60000000000}); code != http.StatusOK {
		t.Fatalf("advancing: status %d (%v)", code, body)
	}
	if after := h.srv.queueDepth(); after != before {
		t.Errorf("the queue was %d before the clock moved and %d after: moving time must not "+
			"move the worker, or R6's lag and R7's deadline cannot be exercised apart", before, after)
	}
}
