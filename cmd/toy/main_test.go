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
	return &harness{t: t, ts: ts, dir: dir}
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
	return id
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
	if got, want := h.count(id, alice), 1; got != want {
		t.Errorf("the API count is %d, want %d", got, want)
	}
	if got, want := h.storeCount(id), 1; got != want {
		t.Errorf("the store's count is %d, want %d", got, want)
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
	id := h.createEvent(alice, "one seat, two claimants", 1, []string{roleAdmin, roleMember, roleGuest})

	type result struct {
		who  string
		code int
		err  error
	}
	results := make(chan result, 2)
	for _, who := range []struct{ name, token string }{{"bob", bob}, {"carol", carol}} {
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
	for i := 0; i < 2; i++ {
		r := <-results
		if r.err != nil {
			t.Fatalf("%s joining: %v", r.who, r.err)
		}
		if r.code == http.StatusOK {
			landed++
		}
	}

	if landed != 2 {
		t.Fatalf("%d of 2 concurrent joins landed, want 2: the capacity race is the epic's R5 "+
			"ledger row and must be PRESENT in the toy, so a run of two in flight has to overrun "+
			"the seat. If this fails, the window in joinEvent was closed and R8 has nothing to catch", landed)
	}
	if got := h.storeCount(id); got != 2 {
		t.Errorf("the store holds %d attendee(s) on a one-seat event, want 2: both joins landed but "+
			"the list did not grow by two", got)
	}
	if got := h.count(id, alice); got != 2 {
		t.Errorf("the API reports %d on a one-seat event, want 2", got)
	}
}
