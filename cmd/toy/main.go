// Command toy is the small application the engine is proven against: a JSON
// API and a server-rendered UI over users, events, roles, capacity and -- as of
// R7.4 -- EXPIRING INVITATIONS, which are the first thing in this domain that
// carries an instant (R5.2, issue 73). Its bugs arrive one commit at a time, each with the epic's ledger
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
	Count int `json:"count"`
	// Tally is THE WORKER'S OWN NUMBER and nothing else reads or writes it.
	// Count above is maintained by the write that changes it, which is what
	// R5's rows rest on; Tally is maintained by the RECOUNT QUEUE and by
	// nothing else, so it is current as of the last recount the worker applied
	// and a reader asking straight after one asked for sees the old number.
	//
	// TWO COUNTS WITH TWO TIMINGS, which is fable's ruling at coop r6/82: "the
	// eventual type is the WORKER'S OWN claim, not the count R5 planted its
	// rows in". One field with both roles made seed 40's R5 finding read as
	// engine.stale at step 4 behind a delete -- the child losing a row rather
	// than finding it late, where the ledger needs both.
	Tally int `json:"tally"`
	// Recounted says the worker has applied a recount for this event already.
	//
	// IT IS THE R6 LEDGER ROW: a retry dropped, a count that never settles.
	// The worker treats a repeat recount as redundant and drops it, so the
	// SECOND ask for an event is never applied -- and once the attendee list
	// has moved since the first, the tally is wrong for good. One defect wide:
	// the job is dropped, not mis-applied, so the number stays one the worker
	// once computed correctly and the finding is about freshness.
	Recounted bool `json:"recounted"`
	Deleted   bool `json:"deleted"`
}

// invitation is a standing offer to join one event, and it EXPIRES.
//
// IT IS R7's SUBJECT AND THE FIRST THING IN THIS DOMAIN THAT CARRIES AN
// INSTANT. `ExpiresAt` is in nanoseconds from the run's epoch, the same unit
// and the same clock as `state.Now`, and the engine reads it through an
// observation rather than computing it from a lifetime declared somewhere else
// -- which is the whole of `[deadline].instants` in docs/contract.md: one
// number, held by the system that enforces it.
//
// WHY AN ENTITY AND NOT A FLAG ON THE EVENT, which is the recount's argument
// run again: the engine resolves an observation's arguments from the handles a
// STEP minted, so the expiry is only a question once an invitation exists. An
// expiry hung off the event would be observable on every step that touched one
// and would have no instant to report.
type invitation struct {
	ID      string `json:"id"`
	EventID string `json:"event_id"`
	// UserID is WHO WAS INVITED, and only they may accept: an invitation that
	// anyone could redeem would make the deadline the only thing it enforced.
	UserID string `json:"user_id"`
	// ExpiresAt is the instant after which this invitation is no longer
	// honored. The comparison is in acceptInvitation and it is STRICT -- at
	// this instant the invitation still works -- which mirrors the engine's
	// own `now - claimTime > bound` and makes the landing exactly ON the
	// deadline the case that separates the two readings.
	ExpiresAt int64 `json:"expires_at"`
	Accepted  bool  `json:"accepted"`
}

// invitationLifetime is how long an invitation stands, from the instant the
// world held when it was created.
//
// IT LIVES HERE AND IN NO DECLARATION. A lifetime in `bindings.toml` would be
// a second copy of this number, and the engine would then target an instant
// computed from its copy rather than the one this server enforces -- so a walk
// could miss the deadline while both files looked right. The project declares
// where the instant can be READ; this is the only place it is decided.
const invitationLifetime = int64(30 * time.Second)

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

	// Invitations is invitation id -> the invitation.
	Invitations map[string]*invitation `json:"invitations"`

	// Recounts is recount id -> event id, and it exists so a RECOUNT has an
	// identity of its own.
	//
	// WHY AN ENTITY AND NOT A BARE POST: the engine resolves an observation's
	// arguments from the handles the STEP minted, and a handle resolves to an
	// id the system gave. An observation of the worker's tally declaring the
	// EVENT handle would be makeable on every step that touches an event --
	// measured at step 2 of seed 2 (ibis, coop r6/100) -- and the tally is only
	// a question once a recount has been asked for. So the recount is the
	// subject, and `GET /recounts/<id>/tally` answers about the event it names.
	Recounts map[string]string `json:"recounts"`

	Seq int `json:"seq"`

	// Now is the world's own time, in NANOSECONDS from the run's epoch and
	// zero in a fresh world. The unit is docs/contract.md's for `clock.t` and
	// this server is HELD to it rather than choosing it: the engine's clock
	// and this one are compared by a run, and a second unit on that path is
	// where an off-by-a-thousand lives.
	//
	// IT IS IN THE STATE FILE, AND THAT IS THE ROLLBACK DESIGN RATHER THAN A
	// PLACEMENT. This struct's own comment says nothing about the application
	// lives outside the file or the provider's verify would be lying; a clock
	// held anywhere else would survive a restore, so the world after one would
	// be the checkpoint PLUS however far time had been moved since. The
	// adapter held exactly such a number until R7.4 and said so in its own
	// comment -- honest while nothing downstream read it, and wrong the moment
	// an invitation expires by it.
	//
	// NOTHING HERE READS `time.Now` FOR IT. The world moves only when it is
	// TOLD to, through `POST /clock`, which is what makes a clocked run
	// exactly as repeatable as an unclocked one -- the same reason the recount
	// queue advances on the request sequence and not on a timer (fable's
	// ruling 1, coop r6/78).
	Now int64 `json:"now"`

	// Jobs is the recount queue, and it is IN THE STATE FILE for the reason
	// this struct's own comment gives: nothing about the application lives
	// outside the file, or the rollback provider's verify would be lying. A
	// queue in memory would also be invisible to the `direct` source, and the
	// engine's quiesce probe reads its depth through exactly that.
	//
	// EACH ENTRY IS AN EVENT ID AND A TRY COUNT. The id is what to recount;
	// Tries is what makes "a retry dropped" a thing that can be planted (R6's
	// ledger row) rather than a thing that has to be described.
	Jobs []*job `json:"jobs"`
}

// job is one unit of the worker's work: recompute an event's reported count
// from its attendee list.
//
// WHY THE COUNT IS NOT WRITTEN WHERE THE ATTENDEE IS ADDED, which is what makes
// this phase's bound mean anything: the API's number becomes EVENTUALLY
// consistent with the store's, so a reader that asks immediately after a join
// sees the old one. docs/vocabulary.md calls that a `pending` on an eventual
// claim type and not a defect, and the quiesce probes are how a run knows the
// difference between "not yet" and "never".
type job struct {
	EventID string `json:"event_id"`
	// Tries is kept for the row's own sake: a worker that retried would count
	// here, and the plant in drainOne is that the retry is DROPPED rather than
	// counted. It stays zero today and is not read, which is said out loud
	// rather than left to look like an oversight.
	Tries int `json:"tries"`
}

func newState() *state {
	return &state{
		Users:       map[string]*user{},
		Events:      map[string]*event{},
		Sessions:    map[string]*session{},
		Recounts:    map[string]string{},
		Invitations: map[string]*invitation{},
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

// newServer loads the world. THERE IS NO WORKER GOROUTINE AND THERE MUST NOT
// BE ONE: fable's ruling 1 at coop r6/78 is that R5's D1 -- two attempts from
// one seed are byte-identical -- is a property of the whole system under a
// seed, worker included, so "a toy whose counts drain on a wall clock makes
// the walk's observations depend on load". It did: five repeats of seed 1 gave
// three digests and an attribution that flipped. The queue advances on the
// REQUEST SEQUENCE instead (see advancing), which is drawn from nothing the
// clock touches.
//
// THE GOROUTINE WAS HERE AND NOT IN main() FIRST, which was the right fix to
// the wrong mechanism: a constructor is where a server's behaviour belongs, and
// every caller that built a server without main() had no worker at all. That
// reasoning still holds -- it is why the drain now rides routes() rather than
// the listener -- but a timer was never a thing a deterministic run could own.
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
	// AND THE OTHER TWO, WHICH WERE MISSING. These guards exist because a
	// state file may carry an explicit `null` for a map -- `DisallowUnknownFields`
	// stops an unknown key and says nothing about a known one being null -- and
	// a write into a nil map panics. `recounts` had no guard before R7.4 and
	// `invitations` would have shipped without one by copying its neighbors.
	//
	// FIVE MAPS AND FIVE GUARDS, which is the number this comment got wrong
	// (ratchet, coop r7/65): it said "all four" with its own sentence adding
	// two to three, and a figure that disagrees with the code beside it sends
	// the next reader hunting for an exempt map. `state` declares users,
	// events, sessions, invitations and recounts; the list below is complete
	// rather than historical, and ratchet swept for a sibling before reporting
	// -- no nested maps anywhere, and `event` carries only slices, where
	// appending to a nil one is safe.
	//
	// ARMED FIVE FOR FIVE by TestAStateFileWhoseMapsAreNullReloadsAndStillWrites:
	// each guard removed on its own kills it (ratchet drove all five, where I
	// had driven the two I added).
	if st.Recounts == nil {
		st.Recounts = map[string]string{}
	}
	if st.Invitations == nil {
		st.Invitations = map[string]*invitation{}
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

// enqueue adds a recount for an event. The caller holds the lock.
//
// ONE JOB PER ASK AND NO COLLAPSING, deliberately: the depth is what the
// `queue_depth` probe reads, and a depth that under-reports is a probe that
// reads quiet while work remains -- the one answer that must never be wrong.
//
// IT COLLAPSED A SECOND JOB ONTO THE FIRST FOR ONE AFTERNOON, as the planted
// defect, and a WALK COULD NEVER REACH IT: every step's observations include a
// read, a read advances the queue by one, and an action enqueues at most one,
// so the depth oscillates between zero and one and two jobs for one event are
// never outstanding together. The plant lives in the worker now (see
// drainOne), where a depth of one is enough to reach it.
func (s *server) enqueue(eventID string) {
	s.st.Jobs = append(s.st.Jobs, &job{EventID: eventID})
}

// drainOne applies the oldest job, if any. The caller holds the lock.
//
// A JOB FOR A VANISHED EVENT IS DONE, NOT FAILED: the event was deleted, so
// there is no count to maintain and dropping it is correct rather than a
// silent loss. That distinction is what leaves room for the R6 ledger row to
// be about a retry that was dropped while its event still existed.
func (s *server) drainOne() {
	if len(s.st.Jobs) == 0 {
		return
	}
	j := s.st.Jobs[0]
	s.st.Jobs = s.st.Jobs[1:]
	ev, ok := s.st.Events[j.EventID]
	if !ok {
		return
	}
	// THE WORKER MAINTAINS ITS OWN COUNT AND NOT THE API'S. It wrote ev.Count
	// until fable's ruling at coop r6/82 separated them: one number cannot be
	// both R5's immediate count and R6's eventual one, and making it both
	// relabelled an R5 row instead of adding an R6 one.
	//
	// THE DEFECT, AND IT IS R6'S LEDGER ROW: a retry dropped, a count that
	// never settles. A recount for an event this worker has already recounted
	// is treated as redundant and DROPPED -- so the second ask is never
	// applied, and once the attendee list has moved since the first the tally
	// is wrong for good. The job is dropped and not mis-applied, which is what
	// makes the finding about freshness: the number stays one the worker
	// computed correctly, for a world that has since changed.
	if ev.Recounted {
		return
	}
	ev.Tally = len(ev.Attendees)
	ev.Recounted = true
	_ = s.save()
}

// queueDepth is the work outstanding, which is what a `queue_depth` probe asks
// for. The caller must not hold the lock.
func (s *server) queueDepth() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.st.Jobs)
}

// handleQueue is the `direct` source's view of the worker: how much work is
// outstanding and whether the worker has anything to do.
//
// IT TAKES NO SESSION, for handleStore's reason: docs/vocabulary.md makes
// `direct` the engine's own observation and never an actor's. And it does NOT
// drain: a probe that advanced the system it measures would make "quiet" a
// thing the asking caused.
func (s *server) handleQueue(w http.ResponseWriter, r *http.Request) {
	d := s.queueDepth()
	writeJSON(w, http.StatusOK, map[string]any{"depth": d, "idle": d == 0})
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/clock", s.handleClock)
	mux.HandleFunc("/users", s.handleUsers)
	mux.HandleFunc("/sessions", s.handleSessions)
	mux.HandleFunc("/events", s.handleEvents)
	mux.HandleFunc("/events/", s.handleEventPath)
	mux.HandleFunc("/store/events/", s.handleStore)
	mux.HandleFunc("/store/queue", s.handleQueue)
	mux.HandleFunc("/invitations/", s.handleInvitationPath)
	mux.HandleFunc("/recounts/", s.handleRecountPath)
	mux.HandleFunc("/ui/events/", s.handleUI)
	return s.advancing(mux)
}

// advancing is the worker: A READ ADVANCES THE QUEUE BY ONE, AFTER IT HAS
// ANSWERED, and a write only adds to it. That is fable's ruling 1 (coop r6/78)
// -- the queue advances on the request sequence, "drawn from nothing the clock
// touches" -- so the lag is a fact of the step ORDER and two attempts of a seed
// are byte-identical as D1 requires. A goroutine on a 2ms tick gave three
// digests for seed 1 and an attribution that flipped.
//
// AFTER AND NOT BEFORE, which is what makes the lag observable at all: a drain
// in front of a read would serve the freshly-applied tally, so the gap would
// exist and nothing could ever see it. The reader is served what the worker
// has, and then the worker takes one step.
//
// A WRITE DOES NOT DRAIN, and this is the half I had wrong first. With writes
// draining too, every write drained one job and enqueued one, so the queue
// never held more than a single job -- and the R6 row needs two changes to one
// event to be outstanding TOGETHER. The depth that `queue_depth` reports was
// structurally incapable of exceeding 1, which made the probe decorative and
// the ledger's row unreachable. My own arm caught it (TestASecondChange...)
// where no reasoning had.
//
// THE PROBE ENDPOINTS ARE EXEMPT BY NAME, which handleQueue states as its own
// rule: "a probe that advanced the system it measures would make quiet a thing
// the asking caused". /store/events/ and /store/queue are the engine's `direct`
// observations, and nothing else may join that set without the same argument.
//
// ONE JOB, NOT THE QUEUE: draining to empty would make the depth a probe reads
// never exceed one read's worth of work, which is the same defect as above
// wearing different clothes.
func (s *server) advancing(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r)
		if r.Method == http.MethodGet && !strings.HasPrefix(r.URL.Path, "/store/") {
			s.mu.Lock()
			s.drainOne()
			s.mu.Unlock()
		}
	})
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

// handleClock moves the world's time and answers what it now holds.
//
// IT ANSWERS THE STATE AND NOT THE ASK, which is the only thing that makes the
// `advance` op meaningful: a server that echoed the request would pass a `set`
// and a single `advance` and be wrong from the second one on. The adapter
// forwards this answer unchanged, and the engine writes the ledger from what
// came back (fable, coop r7/51).
//
// A WRITE, SO IT DRAINS NOTHING. `advancing` steps the recount queue after a
// GET, and a clock move is a POST -- so moving time does not also move the
// worker, and the two phases' mechanisms stay separable. A reader who wants
// both does both.
//
// WHAT IS NOT REFUSED HERE, said rather than left to look like an oversight: a
// `set` to an earlier instant, and an `advance` of zero or less. The engine
// refuses a non-positive delta before it reaches any adapter and refuses an
// answer that moved the world BACKWARD, so a guard here would be a second
// reading of a rule that already has one -- and this is an APPLICATION's test
// clock, which a test is entitled to wind back.
func (s *server) handleClock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, http.StatusMethodNotAllowed, "only POST moves the clock")
		return
	}
	var body struct {
		Op string `json:"op"`
		T  int64  `json:"t"`
	}
	if err := decodeBody(r, &body); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch body.Op {
	case "set":
		s.st.Now = body.T
	case "advance":
		s.st.Now += body.T
	default:
		// NAMED, NOT GUESSED AT: the strict decoder checks that `op` is
		// present and a string, which is not the same as it being one of two
		// words. The adapter refuses an unknown op before this route is
		// reached; this is the second reader, and it exists because the toy is
		// an application that anything may call.
		fail(w, http.StatusBadRequest,
			fmt.Sprintf("%q is not a clock op; this one has \"set\" and \"advance\"", body.Op))
		return
	}
	if err := s.save(); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"t": s.st.Now})
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
	case len(parts) == 2 && parts[1] == "tally" && r.Method == http.MethodGet:
		s.eventTally(w, r, id)
	case len(parts) == 2 && parts[1] == "recount" && r.Method == http.MethodPost:
		s.recountEvent(w, r, id)
	case len(parts) == 2 && parts[1] == "invitations" && r.Method == http.MethodPost:
		s.inviteToEvent(w, r, id)
	default:
		fail(w, http.StatusNotFound, fmt.Sprintf("no route for %s %s", r.Method, r.URL.Path))
	}
}

// handleInvitationPath routes the two things an invitation answers for: being
// accepted, and saying when it expires.
//
// THE EXPIRY IS A GET AND A ROUTE OF ITS OWN, because it is what the engine
// READS. `[deadline].instants` names an observation, the adapter turns that
// name into a path, and the value it hands back is the instant itself -- so
// there has to be somewhere to ask. It answers the instant and nothing else
// derived from it: no "expired" boolean, because that would be a SECOND source
// for a decision acceptInvitation already makes, which is the mistake the
// count routes' own comment records one entity over.
func (s *server) handleInvitationPath(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/invitations/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	id := parts[0]
	if id == "" {
		fail(w, http.StatusNotFound, "no invitation id in the path")
		return
	}
	switch {
	case len(parts) == 2 && parts[1] == "accept" && r.Method == http.MethodPost:
		s.acceptInvitation(w, r, id)
	case len(parts) == 2 && parts[1] == "expiry" && r.Method == http.MethodGet:
		s.invitationExpiry(w, r, id)
	default:
		fail(w, http.StatusNotFound, fmt.Sprintf("no route for %s %s", r.Method, r.URL.Path))
	}
}

// inviteToEvent creates a standing offer for one user to join one event.
//
// THE EVENT'S ADMIN AND NOBODY ELSE. A member who could invite would make the
// role irrelevant to the only thing this entity controls, and the statute's
// `role` is held relative to an event precisely so this kind of rule can be
// written.
func (s *server) inviteToEvent(w http.ResponseWriter, r *http.Request, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.session(r)
	if sess == nil {
		fail(w, http.StatusUnauthorized, "inviting needs a session")
		return
	}
	ev, ok := s.st.Events[id]
	if !ok || ev.Deleted {
		fail(w, http.StatusNotFound, "no such event")
		return
	}
	if roleOf(ev, sess.UserID) != roleAdmin {
		fail(w, http.StatusForbidden, "only the event's admin invites")
		return
	}
	var body struct {
		User string `json:"user"`
	}
	if err := decodeBody(r, &body); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, ok := s.st.Users[body.User]; !ok {
		fail(w, http.StatusNotFound, "no such user")
		return
	}
	if roleOf(ev, body.User) == roleMember {
		fail(w, http.StatusConflict, "that user is already a member")
		return
	}
	inv := &invitation{
		ID:      s.nextID("i"),
		EventID: ev.ID,
		UserID:  body.User,
		// THE DEADLINE IS COMPUTED FROM THE WORLD'S OWN CLOCK, so an
		// invitation created at `now` expires at a known instant and nothing
		// about it depends on when the process happens to be running.
		ExpiresAt: s.st.Now + invitationLifetime,
	}
	s.st.Invitations[inv.ID] = inv
	if err := s.save(); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, inv)
}

// acceptInvitation redeems one, and THIS IS WHERE THE DEADLINE IS ENFORCED.
//
// THE COMPARISON IS STRICT AND THAT IS THE HINGE: `now > ExpiresAt` refuses,
// so AT the expiry the invitation still works. It mirrors the engine's own
// `now - claimTime > bound` deliberately -- one reading of "past a deadline"
// in this repository, not two -- and it is what makes the walk's landing
// exactly ON the instant the case that tells a strict comparison from a loose
// one. A `>=` here and a `>` there would make the two tiers disagree about the
// same nanosecond.
//
// NO VISIBILITY CHECK, AND THAT IS WHAT AN INVITATION IS. joinEvent refuses an
// event the actor cannot see; this route is the other door, and the invitation
// is the key. Its own comment carries the pair.
//
// AND NO CAPACITY RACE. joinEvent drops the lock between the check and the
// insert because the epic's ledger carries that row; this route holds it, and
// the absence is deliberate rather than an oversight -- one defect per row,
// and nobody asked for a second way to overfill an event.
func (s *server) acceptInvitation(w http.ResponseWriter, r *http.Request, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.session(r)
	if sess == nil {
		fail(w, http.StatusUnauthorized, "accepting needs a session")
		return
	}
	inv, ok := s.st.Invitations[id]
	if !ok {
		fail(w, http.StatusNotFound, "no such invitation")
		return
	}
	if inv.UserID != sess.UserID {
		fail(w, http.StatusForbidden, "that invitation is not yours")
		return
	}
	if inv.Accepted {
		fail(w, http.StatusConflict, "that invitation has been accepted already")
		return
	}
	if s.st.Now > inv.ExpiresAt {
		fail(w, http.StatusGone, fmt.Sprintf(
			"invitation %s expired at %dns and it is now %dns", inv.ID, inv.ExpiresAt, s.st.Now))
		return
	}
	ev, ok := s.st.Events[inv.EventID]
	if !ok || ev.Deleted {
		fail(w, http.StatusNotFound, "no such event")
		return
	}
	if len(ev.Attendees) >= ev.Capacity {
		fail(w, http.StatusConflict, fmt.Sprintf("event %s is at capacity %d", ev.ID, ev.Capacity))
		return
	}
	ev.Attendees = append(ev.Attendees, sess.UserID)
	// THE API'S COUNT MOVES WITH THE WRITE, exactly as joinEvent does: this is
	// the number R5's rows are about, and a second way in that left it alone
	// would be a count defect nobody planted.
	ev.Count = len(ev.Attendees)
	inv.Accepted = true
	if err := s.save(); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ev)
}

// invitationExpiry answers WHEN, and nothing about whether.
func (s *server) invitationExpiry(w http.ResponseWriter, r *http.Request, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	inv, ok := s.st.Invitations[id]
	if !ok {
		fail(w, http.StatusNotFound, "no such invitation")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"invitation": inv.ID,
		"expires_at": inv.ExpiresAt,
	})
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
	// call the pair a defect.
	//
	// AND AS OF R7.4 THERE IS AN INVITATION IN THIS DOMAIN, which is what this
	// comment said there was not. An invite-only event -- visible to admin and
	// member alone -- still takes no new members THROUGH THIS ROUTE; the way
	// in is `POST /invitations/<id>/accept`, where the invitation is the
	// authorization and the visibility check is deliberately absent. Two doors
	// with two keys, rather than one door that sometimes ignores its lock.
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
	// THE API'S COUNT MOVES WITH THE WRITE, as it did before the worker: this
	// is the number R5's rows are about, and an eventual bound over it turns
	// those rows into R6 findings (fable, coop r6/82).
	//
	// AND IT ENQUEUES NOTHING. The worker's tally is maintained by recounts
	// alone, which is what makes the tally a question an actor ASKS -- and the
	// queue is then only ever as deep as the recounts outstanding, which a walk
	// can actually reach: every step's observations include a read, and a read
	// advances the queue by one, so a job enqueued by every join could never
	// leave the queue deeper than one.
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
	// SYNCHRONOUS, for the join's reason: this is R5's number, and the worker's
	// is the recount queue's business.
	ev.Count = len(ev.Attendees)
	if err := s.save(); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ev)
}

// deleteEvent carries the ledger's "stale count after delete" row (epic, R5,
// caught by api-sim): it marks the event deleted and empties the attendee list,
// and LEAVES THE DENORMALIZED COUNT where it was. So the API goes on reporting
// an attendance for an event that is gone, while the store's own count --
// computed from the attendee list by /store/events/<id>/count -- says nothing is
// attending.
//
// THE TWO ORACLES ARE WHAT MAKE IT FINDABLE. A count kept in a second place is
// not a defect by itself; a count kept in a second place that nobody updates is,
// and the only way to SEE that is to ask both. The direct observation declared
// in examples/toy/bindings.toml reads the list and the api observation reads the
// field, so the disagreement between them is the finding -- which is why the
// second oracle is not optional decoration.
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
	// THE DEFECT: ev.Count is NOT cleared with the event it counts.
	if err := s.save(); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ev)
}

// eventCount carries the ledger's "API visibility leak" row (epic, R5, caught
// by api-sim): it asks for a session and then answers ANY caller, without
// asking whether that caller's role may see the event at all. listEvents asks;
// this does not, and two endpoints disagreeing about one question is the defect.
//
// IT WAS HERE BEFORE IT WAS DECLARED, and that is recorded rather than tidied
// away: C1 shipped this handler with no visibility check while its commit
// called the file "the honest version" the bugs would be introduced into. The
// omission already WAS the leak. Nothing was mis-measured -- no test pinned the
// behaviour either way -- but the claim was wrong, and the repair is to make the
// defect deliberate, name its row, and pin it in the suite so it cannot be
// fixed by accident.
//
// WHY A LEAK HERE IS VISIBLE TO v1 AT ALL: docs/vocabulary.md:80 makes a leak a
// `bug` only when it MOVES the observed value, and this one does. An observer
// whose role may not see the event should expect nothing of it, and this hands
// back a number -- the shape examples/toy's visibility-leak case judges.
//
// It also reports the DENORMALIZED count, which is what an actor sees and what
// the store's own count is compared against.
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

// recountEvent is an actor asking for the event's attendance to be recomputed.
// It enqueues one job and answers with the RECOUNT's own id, so the walk can
// mint a handle for it and an observation can name it as its subject.
//
// IT ENQUEUES AND DOES NOT RECOMPUTE, which is the whole point: the worker's
// number is maintained by the queue, so what this returns is a promise and the
// tally read straight after it is the old one. That gap is the `pending` an
// eventual claim type exists for.
func (s *server) recountEvent(w http.ResponseWriter, r *http.Request, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.session(r)
	if sess == nil {
		fail(w, http.StatusUnauthorized, "asking for a recount needs a session")
		return
	}
	ev, ok := s.st.Events[id]
	if !ok || ev.Deleted {
		fail(w, http.StatusNotFound, "no such event")
		return
	}
	rid := s.nextID("r-")
	s.st.Recounts[rid] = ev.ID
	s.enqueue(ev.ID)
	if err := s.save(); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	// THE KEY IS `id`, which is this toy's convention for a thing it just made
	// (create_user and create_event both answer under it) and what the adapter
	// reads to fill the execute response's `ids`. Answering under `recount`
	// instead left that map EMPTY, and the walk refused with the right
	// question: a handle that would resolve to nothing.
	writeJSON(w, http.StatusOK, map[string]any{"id": rid, "event": ev.ID})
}

// handleRecountPath routes /recounts/<id>/tally and nothing else.
func (s *server) handleRecountPath(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/recounts/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "tally" || r.Method != http.MethodGet {
		fail(w, http.StatusNotFound, fmt.Sprintf("no route for %s %s", r.Method, r.URL.Path))
		return
	}
	s.recountTally(w, r, parts[0])
}

// recountTally answers about the event a recount names, which is what makes the
// RECOUNT the observation's subject.
func (s *server) recountTally(w http.ResponseWriter, r *http.Request, rid string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.session(r)
	if sess == nil {
		fail(w, http.StatusUnauthorized, "reading a tally needs a session")
		return
	}
	eid, ok := s.st.Recounts[rid]
	if !ok {
		fail(w, http.StatusNotFound, "no such recount")
		return
	}
	ev, ok := s.st.Events[eid]
	if !ok {
		fail(w, http.StatusNotFound, "no such event")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"recount": rid, "event": ev.ID,
		"tally": ev.Tally, "deleted": ev.Deleted})
}

// eventTally serves THE WORKER'S OWN COUNT, which is the number R6's eventual
// claim type is about: it lags a write until the recount queue advances, and a
// reader asking in between is reading a `pending` rather than a defect.
//
// IT IS A SEPARATE ROUTE AND NOT A FIELD ON THE COUNT RESPONSE, deliberately.
// An observation names one value, and two numbers in one body would make the
// adapter choose which the statute judges -- a choice that belongs in the
// project's declarations, where `of` and `query` already say it.
//
// IT TAKES A SESSION, like the count: this is an `api` observation and not the
// engine's `direct` one. The store's own route stays the second oracle.
func (s *server) eventTally(w http.ResponseWriter, r *http.Request, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.session(r)
	if sess == nil {
		fail(w, http.StatusUnauthorized, "reading a tally needs a session")
		return
	}
	ev, ok := s.st.Events[id]
	if !ok {
		fail(w, http.StatusNotFound, "no such event")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"event": ev.ID, "tally": ev.Tally, "deleted": ev.Deleted})
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
