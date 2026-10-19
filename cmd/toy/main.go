// Command toy is the small application the engine is proven against: a JSON
// API and a server-rendered UI over users, events, roles and capacity (R5.2,
// issue 73). Its bugs arrive one commit at a time, each with the epic's ledger
// row it answers; this file is the honest version they are introduced into.
//
// WHY IT IS A PROCESS AND NOT A LIBRARY. The rollback provider R5.3 consumes is
// a PROCESS-restart provider, and a restart provider cannot restart a library
// (ibis, coop r5.toy/1). The engine starts `toy-adapter`, the adapter starts
// this, and everything the adapter knows about the world it learns over HTTP --
// the same way it would learn about a system it did not ship with.
//
// ALL STATE IS IN ONE FILE, which is the whole of the rollback design: a
// checkpoint is a copy of that file, a restore is a copy back, and `verify`
// READS it rather than assuming the restore worked. Sessions live in it too,
// so the world after a restore is the world the checkpoint described and not
// the checkpoint plus whoever was still logged in.
//
// THE IDS ARE SEQUENTIAL AND THAT IS NOT A DETERMINISM CLAIM. docs/design.md
// says every attempt's ids differ and the engine compares findings over
// handles, never ids, so this server is free to count from one. What it must
// not do is draw randomness: the engine owns the seed, and a server that rolled
// its own dice would make a replay depend on something the seed does not carry.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The three roles. A role is held RELATIVE TO AN EVENT and is not a property of
// a user: the same actor is admin of the event it created, member of one it
// joined, and guest of one it has only been told about. docs/vocabulary.md's
// `role` on an observation is the role the observing principal acted as, which
// is exactly this.
const (
	roleAdmin  = "admin"
	roleMember = "member"
	roleGuest  = "guest"
)

type user struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type event struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	AdminID   string   `json:"admin_id"`
	Capacity  int      `json:"capacity"`
	VisibleTo []string `json:"visible_to"`
	Attendees []string `json:"attendees"`
	// Count is DENORMALIZED on purpose: it is the number the API reports and
	// the store's own count is computed from Attendees. Two sources for one
	// fact is where the ledger's "stale count after delete" row lives, and the
	// honest version keeps them equal.
	Count   int  `json:"count"`
	Deleted bool `json:"deleted"`
}

type session struct {
	Token   string `json:"token"`
	UserID  string `json:"user_id"`
	Context string `json:"context"`
}

// state is the whole world, and the file is the whole state. Nothing about the
// application lives outside it -- no counter in memory, no cache -- because the
// rollback provider's verify compares the file and would be lying if it did.
type state struct {
	Users    map[string]*user    `json:"users"`
	Events   map[string]*event   `json:"events"`
	Sessions map[string]*session `json:"sessions"`
	Seq      int                 `json:"seq"`
}

func newState() *state {
	return &state{
		Users:    map[string]*user{},
		Events:   map[string]*event{},
		Sessions: map[string]*session{},
	}
}

type server struct {
	mu   sync.Mutex
	st   *state
	path string
}

func main() {
	addr := flag.String("addr", "127.0.0.1:0", "the address to serve on; :0 picks a free port and the chosen one is printed")
	statePath := flag.String("state", "", "the file holding the whole world (required)")
	flag.Parse()
	if *statePath == "" {
		fmt.Fprintln(os.Stderr, "toy: --state is required: the whole world lives in one file and nothing else holds it")
		os.Exit(2)
	}
	srv, err := newServer(*statePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "toy: %v\n", err)
		os.Exit(1)
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "toy: listening on %s: %v\n", *addr, err)
		os.Exit(1)
	}
	// THE ADDRESS IS PRINTED AND THE PORT IS NOT ASSUMED. The adapter starts
	// this process on :0 and reads the line, so two runs on one box never
	// collide over a port -- which would surface as a test failure about the
	// application rather than about the harness.
	// No flush: os.Stdout is a *os.File and Go does not buffer it, so the
	// adapter reading this from a pipe gets the line as it is written. The
	// buffering trap this project has been bitten by is Python's.
	fmt.Printf("toy: listening on %s state=%s\n", ln.Addr().String(), *statePath)
	if err := http.Serve(ln, srv.routes()); err != nil {
		fmt.Fprintf(os.Stderr, "toy: serving: %v\n", err)
		os.Exit(1)
	}
}

func newServer(path string) (*server, error) {
	srv := &server{path: path, st: newState()}
	if err := srv.load(); err != nil {
		return nil, err
	}
	return srv, nil
}

// load reads the world, treating an absent file as an empty one: the first run
// of a fresh checkout has no state and that is not an error. A file that exists
// and cannot be read IS one -- starting empty over an unreadable world would
// silently discard it, which is the shape of defect this project calls making
// the error go away.
func (s *server) load() error {
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading the state file %s: %w", s.path, err)
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil
	}
	st := newState()
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(st); err != nil {
		return fmt.Errorf("decoding the state file %s: %w", s.path, err)
	}
	if st.Users == nil {
		st.Users = map[string]*user{}
	}
	if st.Events == nil {
		st.Events = map[string]*event{}
	}
	if st.Sessions == nil {
		st.Sessions = map[string]*session{}
	}
	s.st = st
	return nil
}

// save writes the world atomically. The temp file is created beside the target
// so the rename cannot cross a device, and its mode is set explicitly because
// an atomic replace makes a NEW inode and inherits nothing from the old one.
func (s *server) save() error {
	raw, err := json.MarshalIndent(s.st, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding the state: %w", err)
	}
	raw = append(raw, '\n')
	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, ".toy-state-*")
	if err != nil {
		return fmt.Errorf("creating a temp file in %s: %w", dir, err)
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing %s: %w", name, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("syncing %s: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", name, err)
	}
	if err := os.Chmod(name, 0o644); err != nil {
		return fmt.Errorf("setting the mode of %s: %w", name, err)
	}
	if err := os.Rename(name, s.path); err != nil {
		return fmt.Errorf("replacing %s: %w", s.path, err)
	}
	return nil
}

func (s *server) nextID(prefix string) string {
	s.st.Seq++
	return fmt.Sprintf("%s%d", prefix, s.st.Seq)
}

// roleOf is the role an actor holds relative to one event.
func roleOf(ev *event, userID string) string {
	if ev.AdminID == userID {
		return roleAdmin
	}
	for _, id := range ev.Attendees {
		if id == userID {
			return roleMember
		}
	}
	return roleGuest
}

func visibleTo(ev *event, role string) bool {
	for _, r := range ev.VisibleTo {
		if r == role {
			return true
		}
	}
	return false
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/users", s.handleUsers)
	mux.HandleFunc("/sessions", s.handleSessions)
	mux.HandleFunc("/events", s.handleEvents)
	mux.HandleFunc("/events/", s.handleEventPath)
	mux.HandleFunc("/store/events/", s.handleStore)
	mux.HandleFunc("/ui/events/", s.handleUI)
	return mux
}

type apiError struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	// A FAILED WRITE IS NOT COSMETIC but there is nowhere left to report it:
	// the status line is already out. It is swallowed HERE and nowhere else,
	// and the reason is that the client's read is what notices.
	_ = json.NewEncoder(w).Encode(body)
}

func fail(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, apiError{Error: msg})
}

func decodeBody(r *http.Request, into any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		return err
	}
	return nil
}

// session resolves the bearer token to a session, or nil.
func (s *server) session(r *http.Request) *session {
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if tok == "" {
		tok = r.URL.Query().Get("token")
	}
	if tok == "" {
		return nil
	}
	return s.st.Sessions[tok]
}

func (s *server) handleUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, http.StatusMethodNotAllowed, "only POST creates a user")
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := decodeBody(r, &body); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		fail(w, http.StatusBadRequest, "name is required")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.st.Users {
		if u.Name == body.Name {
			fail(w, http.StatusConflict, "that name is taken")
			return
		}
	}
	u := &user{ID: s.nextID("u"), Name: body.Name}
	s.st.Users[u.ID] = u
	if err := s.save(); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

func (s *server) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, http.StatusMethodNotAllowed, "only POST acquires a session")
		return
	}
	var body struct {
		Actor   string `json:"actor"`
		Context string `json:"context"`
	}
	if err := decodeBody(r, &body); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var found *user
	for _, u := range s.st.Users {
		if u.Name == body.Actor {
			found = u
			break
		}
	}
	if found == nil {
		fail(w, http.StatusNotFound, fmt.Sprintf("no user named %q: a session belongs to a user that exists", body.Actor))
		return
	}
	// THE CONTEXT NAMES ARE THE PROJECT'S, AND "browser" IS NOT A FREE CHOICE:
	// internal/validate's isBrowser tests for that exact string, so a UI action
	// declared under any other name makes validate's arms 4 and 5 skip instead
	// of evaluating. The page is served under /ui/, which is a path and not a
	// context -- the two are deliberately not the same word.
	if body.Context != "api" && body.Context != "browser" {
		fail(w, http.StatusBadRequest, fmt.Sprintf("context %q is neither api nor browser", body.Context))
		return
	}
	sess := &session{Token: s.nextID("t"), UserID: found.ID, Context: body.Context}
	s.st.Sessions[sess.Token] = sess
	if err := s.save(); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, sess)
}

func (s *server) handleEvents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.createEvent(w, r)
	case http.MethodGet:
		s.listEvents(w, r)
	default:
		fail(w, http.StatusMethodNotAllowed, "events takes GET and POST")
	}
}

func (s *server) createEvent(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title     string   `json:"title"`
		Capacity  int      `json:"capacity"`
		VisibleTo []string `json:"visible_to"`
	}
	if err := decodeBody(r, &body); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.session(r)
	if sess == nil {
		fail(w, http.StatusUnauthorized, "creating an event needs a session")
		return
	}
	if strings.TrimSpace(body.Title) == "" {
		fail(w, http.StatusBadRequest, "title is required")
		return
	}
	if body.Capacity < 1 {
		fail(w, http.StatusBadRequest, "capacity must be at least 1")
		return
	}
	vis := body.VisibleTo
	if len(vis) == 0 {
		// A DEFAULT THAT IS NOT A GUESS: admin and member are the roles that
		// have a relationship to the event, and guest is the role the
		// visibility leak is about. An absent list therefore means "not
		// public" rather than "everyone".
		vis = []string{roleAdmin, roleMember}
	}
	for _, role := range vis {
		if role != roleAdmin && role != roleMember && role != roleGuest {
			fail(w, http.StatusBadRequest, fmt.Sprintf("visible_to names %q, which is not a role", role))
			return
		}
	}
	ev := &event{
		ID:        s.nextID("e"),
		Title:     body.Title,
		AdminID:   sess.UserID,
		Capacity:  body.Capacity,
		VisibleTo: vis,
		Attendees: []string{},
	}
	s.st.Events[ev.ID] = ev
	if err := s.save(); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, ev)
}

// listEvents is the endpoint the ledger's "API visibility leak" row is about.
// The honest version asks visibleTo for the role the CALLER holds on each
// event; the bug drops that question for one role.
func (s *server) listEvents(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.session(r)
	if sess == nil {
		fail(w, http.StatusUnauthorized, "listing events needs a session")
		return
	}
	out := []*event{}
	for _, ev := range s.st.Events {
		if ev.Deleted {
			continue
		}
		if !visibleTo(ev, roleOf(ev, sess.UserID)) {
			continue
		}
		out = append(out, ev)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	writeJSON(w, http.StatusOK, out)
}

// handleEventPath routes /events/<id>, /events/<id>/join and /events/<id>/count.
func (s *server) handleEventPath(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/events/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	id := parts[0]
	if id == "" {
		fail(w, http.StatusNotFound, "no event id in the path")
		return
	}
	switch {
	case len(parts) == 1 && r.Method == http.MethodDelete:
		s.deleteEvent(w, r, id)
	case len(parts) == 2 && parts[1] == "join" && r.Method == http.MethodPost:
		s.joinEvent(w, r, id)
	case len(parts) == 2 && parts[1] == "join" && r.Method == http.MethodDelete:
		s.leaveEvent(w, r, id)
	case len(parts) == 2 && parts[1] == "count" && r.Method == http.MethodGet:
		s.eventCount(w, r, id)
	default:
		fail(w, http.StatusNotFound, fmt.Sprintf("no route for %s %s", r.Method, r.URL.Path))
	}
}

// raceWindow is the gap between the capacity check and the insert. It is the
// ledger's "capacity race" row (epic, R5, caught by concurrency) and it is a
// DEFECT ON PURPOSE: see joinEvent.
//
// A REAL WINDOW RATHER THAN A TEST HOOK. A hook the test flips would be a
// fixture override standing in for the production path, and this project has
// been bitten by one; a check-then-act with a genuine gap is what the defect
// IS, and 25ms makes it reliably reachable by two requests in flight without
// making the honest sequential path slow enough to notice.
const raceWindow = 25 * time.Millisecond

// joinEvent carries the ledger's "capacity race" row: it checks capacity,
// RELEASES THE LOCK, and inserts without checking again, so two joins in
// flight at the boundary both land and the attendee list passes capacity.
//
// R5 PLANTS IT AND CANNOT CATCH IT, which is fable's ruling at coop
// breakout-r5/1 after ratchet found the gap: one action at a time cannot race,
// the row's catcher is `concurrency`, and that stage is R8's. So the bug is
// here from the first commit with a test that drives it by hand, and R8
// inherits a defect rather than a promise.
//
// IT IS NOT A DATA RACE. Every read and write of the shared state happens under
// the mutex; what is unsynchronized is the DECISION, which is why `go test
// -race` is silent on it and a logical test is the only thing that sees it.
func (s *server) joinEvent(w http.ResponseWriter, r *http.Request, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.session(r)
	if sess == nil {
		fail(w, http.StatusUnauthorized, "joining needs a session")
		return
	}
	ev, ok := s.st.Events[id]
	if !ok || ev.Deleted {
		fail(w, http.StatusNotFound, "no such event")
		return
	}
	role := roleOf(ev, sess.UserID)
	if role == roleMember {
		fail(w, http.StatusConflict, "already joined")
		return
	}
	// YOU CAN ONLY JOIN WHAT YOU CAN SEE, and the two endpoints must agree: a
	// list that hides an event while join accepts it is two answers to one
	// question about visibility, and a statute reading both would be right to
	// call the pair a defect. An invite-only event (visible to admin and member
	// alone) therefore takes no new members, which is what invite-only means
	// here -- there is no invitation in this domain.
	if !visibleTo(ev, role) {
		fail(w, http.StatusNotFound, "no such event")
		return
	}
	if len(ev.Attendees) >= ev.Capacity {
		fail(w, http.StatusConflict, fmt.Sprintf("event %s is at capacity %d", ev.ID, ev.Capacity))
		return
	}
	// THE DEFECT: the lock is dropped between the check above and the insert
	// below, so a second request that passed the same check inserts too.
	s.mu.Unlock()
	time.Sleep(raceWindow)
	s.mu.Lock()
	// THE PLANT IS EXACTLY ONE DEFECT WIDE. In the window the event can be
	// deleted, and inserting into a deleted event would be a SECOND bug -- one
	// the epic's ledger does not carry and nobody asked for. So the event is
	// re-read and its existence re-checked; what is deliberately NOT re-checked
	// is capacity, which is the row.
	ev, ok = s.st.Events[id]
	if !ok || ev.Deleted {
		fail(w, http.StatusNotFound, "no such event")
		return
	}
	ev.Attendees = append(ev.Attendees, sess.UserID)
	ev.Count = len(ev.Attendees)
	if err := s.save(); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ev)
}

func (s *server) leaveEvent(w http.ResponseWriter, r *http.Request, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.session(r)
	if sess == nil {
		fail(w, http.StatusUnauthorized, "leaving needs a session")
		return
	}
	ev, ok := s.st.Events[id]
	if !ok || ev.Deleted {
		fail(w, http.StatusNotFound, "no such event")
		return
	}
	kept := ev.Attendees[:0]
	left := false
	for _, a := range ev.Attendees {
		if a == sess.UserID {
			left = true
			continue
		}
		kept = append(kept, a)
	}
	if !left {
		fail(w, http.StatusConflict, "not joined")
		return
	}
	ev.Attendees = kept
	ev.Count = len(ev.Attendees)
	if err := s.save(); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ev)
}

// deleteEvent is the endpoint the ledger's "stale count after delete" row is
// about. The honest version clears the denormalized count with the event.
func (s *server) deleteEvent(w http.ResponseWriter, r *http.Request, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.session(r)
	if sess == nil {
		fail(w, http.StatusUnauthorized, "deleting needs a session")
		return
	}
	ev, ok := s.st.Events[id]
	if !ok || ev.Deleted {
		fail(w, http.StatusNotFound, "no such event")
		return
	}
	if roleOf(ev, sess.UserID) != roleAdmin {
		fail(w, http.StatusForbidden, "only the event's admin may delete it")
		return
	}
	ev.Deleted = true
	ev.Attendees = []string{}
	ev.Count = 0
	if err := s.save(); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ev)
}

// eventCount reports the DENORMALIZED count: this is what an actor sees.
func (s *server) eventCount(w http.ResponseWriter, r *http.Request, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.session(r)
	if sess == nil {
		fail(w, http.StatusUnauthorized, "reading a count needs a session")
		return
	}
	ev, ok := s.st.Events[id]
	if !ok {
		fail(w, http.StatusNotFound, "no such event")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"event": ev.ID, "count": ev.Count, "deleted": ev.Deleted})
}

// handleStore is the `direct` source: the engine's own query against the
// project's store, and the second oracle behind what the API reports. It is
// COMPUTED from the attendee list rather than read from the denormalized
// field, because an oracle that reads the same number cannot disagree with it,
// and the disagreement is the finding.
//
// It takes no session on purpose: docs/vocabulary.md says `direct` is the
// engine's, never an actor's.
func (s *server) handleStore(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/store/events/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) != 2 || parts[1] != "count" || r.Method != http.MethodGet {
		fail(w, http.StatusNotFound, fmt.Sprintf("no route for %s %s", r.Method, r.URL.Path))
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ev, ok := s.st.Events[parts[0]]
	if !ok {
		fail(w, http.StatusNotFound, "no such event")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"event":   ev.ID,
		"count":   len(ev.Attendees),
		"deleted": ev.Deleted,
	})
}

// handleUI is the server-rendered half: one page per event with a join form.
// R9 drives it with a real browser; R5 only has to serve it and declare it, and
// the page is deliberately plain HTML with no script, so what a browser would
// do and what the flow declares cannot drift.
func (s *server) handleUI(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/ui/events/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	id := parts[0]
	if len(parts) == 2 && parts[1] == "join" && r.Method == http.MethodPost {
		s.joinEvent(w, r, id)
		return
	}
	if len(parts) != 1 || r.Method != http.MethodGet {
		fail(w, http.StatusNotFound, fmt.Sprintf("no route for %s %s", r.Method, r.URL.Path))
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.session(r)
	if sess == nil {
		http.Error(w, "the page needs a session", http.StatusUnauthorized)
		return
	}
	ev, ok := s.st.Events[id]
	if !ok || ev.Deleted {
		http.Error(w, "no such event", http.StatusNotFound)
		return
	}
	role := roleOf(ev, sess.UserID)
	if !visibleTo(ev, role) {
		http.Error(w, "not visible to you", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	page := `<!doctype html>
<title>` + html.EscapeString(ev.Title) + `</title>
<h1 id="title">` + html.EscapeString(ev.Title) + `</h1>
<p id="role">` + role + `</p>
<p id="count">` + strconv.Itoa(ev.Count) + ` of ` + strconv.Itoa(ev.Capacity) + `</p>
<form id="join" method="post" action="/ui/events/` + html.EscapeString(ev.ID) + `/join?token=` + html.EscapeString(sess.Token) + `">
<button type="submit">join</button>
</form>
`
	if _, err := io.WriteString(w, page); err != nil {
		// Nowhere to report it to the client; the status is already sent.
		fmt.Fprintf(os.Stderr, "toy: writing the page for %s: %v\n", ev.ID, err)
	}
}
