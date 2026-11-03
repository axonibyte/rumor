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

	// THE DATABASE IS EMBEDDED HERE TOO, as cmd/rumor embeds it (fable, coop
	// r7/95). The reason is this binary's own: as of R7.5 this application
	// reads its own instants as LOCAL time, so a box whose zone database is
	// absent or stale would make the system under test behave differently from
	// the one a seed was promoted against -- and the walk would report a
	// finding about the HOST. One database, in the binary, on every box.
	//
	// IT IS THE TOY'S FIRST AND ONLY TZ DEPENDENCY (ibis, coop r7/113): the
	// instant stays injected through POST /clock and nothing here reads
	// `time.Now`; the location only READS that instant.
	_ "time/tzdata"
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
	// Rendered is THE NUMBER THE PAGE SHOWS, and it is the ledger's "cached
	// page shows a count the API already updated" row (epic, R9's catcher,
	// planted here in R8/111 by fable's ruling at coop rumor/1464 -- the way
	// R5 planted the race and R7 the grace, with R8's stages driven against it
	// and finding nothing because no stage of R8 reads a page).
	//
	// THREE NUMBERS WITH THREE TIMINGS, which is why this is a third field and
	// not a reuse of either: Count is R5's, maintained by the write that
	// changes it; Tally is the worker's, maintained by the recount queue; and
	// Rendered is the PAGE'S, maintained by the writes that render afterwards.
	// Folding the page's number into Count would make the plant invisible --
	// the page and the API would read one field and could never disagree, which
	// is exactly the state examples/toy/cases/viewer-reads-the-page.json is the
	// honest control for.
	//
	// THE DEFECT IS ONE WRITE WIDE: every path that changes the count sets this
	// too EXCEPT leaveEvent, so the API's count drops on a leave and the page
	// keeps the number it last rendered.
	//
	// AND THE CLAIM IS ABOUT CONSUMERS, NOT VISIBILITY, which is ratchet's
	// finding at coop r8/176 against the sentence that stood here: "only a
	// reader can see it" was wider than the code. A leave answers with the
	// event entire, so its body carries `count=1 rendered=2` and the stale
	// number is in the API's own bytes. Nothing READS it there -- `tally`
	// serialises the same way and is read only through its own path -- and the
	// one consumer of a rendered count anywhere in this tree is
	// cmd/toy-adapter's pageCount, which parses the PAGE. So the defect is
	// reachable through a reader because nothing else consumes the number, not
	// because the number is hidden.
	Rendered int `json:"rendered"`
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

// invitationQuota is how many invitations ONE user may create in one LOCAL
// day, and it is R7.5's planted row (issue 115 deliverable 4, class api).
//
// TWO, MEASURED RATHER THAN CHOSEN, and the figure is ibis's (coop r7/116)
// after mine missed a stream. Across all five rows of examples/toy/seeds.lock:
// four draw no invitation AT ALL and that is structural rather than lucky --
// `[weights.default]` sets invite_user = 0 and only `[weights.profile.expiring]`
// raises it, so walking and settling cannot draw one. Seed 3 under `expiring`
// makes THREE calls by THREE inviters, two in the run (member_2, guest_1) and
// one in the SHRUNK trace (admin_1), which replays on a clean world and so
// reaches this server the way the run does.
//
// NO INVITER MAKES MORE THAN ONE in either stream, so a per-inviter limit of 2
// keeps a 2x margin and every promoted seed draws what it drew before this
// constant existed. A limit of 1 would move seed 3's world, which is the thing
// R7.5 is not allowed to do.
//
// MY OWN FIRST FIGURE WAS "TWO BY TWO", and the defect was the instrument and
// not the world: the run prints `run:` and `trace:` lines, my first grep
// showed both, and the per-actor count I published was taken over `^run:`
// alone. A narrowing applied between seeing the data and reporting it.
//
// IT LIVES HERE AND IN NO DECLARATION, for invitationLifetime's reason: a
// limit in bindings.toml would be a second copy of this number and the engine
// would judge against its copy rather than the one this server enforces.
const invitationQuota = 2

// quota is one user's invitation allowance for one local day.
//
// IT IS IN THE STATE FILE for the reason `state`'s own comment gives: nothing
// about the application lives outside it, or the rollback provider's verify
// would be lying.
//
// *** THIS IS WHERE R7.5's DEFECT IS PLANTED. *** `ResetAt` is the start of
// the current LOCAL day plus twenty-four hours of WALL time, which is right on
// every day of the year except the two a daylight-saving transition makes 23
// or 25 hours long. On the spring day the reset lands an hour LATE, so a user
// who has spent their allowance is still refused for an hour into the next
// day; on the fall day an hour EARLY, so an allowance comes back while the day
// it belongs to is still running. What a daily reset MEANS is the zone's own
// next midnight, which is `AddDate(0, 0, 1)` and not `Add(24 * time.Hour)`.
//
// RE-ANCHORED DAILY, which is fable's ruling 2 at coop r7/90: every touch
// recomputes `Day` and `ResetAt` from the CURRENT local date, so the error
// does not accumulate and the plant is exactly ONE DAY wide. Without that one
// transition would shift every reset for the rest of the run, and the finding
// would be about the whole walk rather than about a day.
type quota struct {
	// Day is the local date this allowance was anchored on, as the zone read
	// it at the time -- so a stale Day beside a later local date IS the
	// disagreement a rule reads.
	Day string `json:"day"`

	Used int `json:"used"`

	// ResetAt is the instant this server will next re-anchor, computed the
	// wrong way on purpose. See this type's own comment.
	ResetAt int64 `json:"reset_at"`
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

	// Invitations is invitation id -> the invitation.
	Invitations map[string]*invitation `json:"invitations"`

	// Quotas is user id -> that user's invitation allowance (R7.5). Keyed on
	// the INVITER, which is the user whose allowance an invitation spends.
	Quotas map[string]*quota `json:"quotas"`

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
		Quotas:      map[string]*quota{},
	}
}

type server struct {
	mu   sync.Mutex
	st   *state
	path string

	// zone is the zone this application reads its own instants in, and it is
	// CONFIGURATION and not world state -- it belongs beside `path` rather
	// than in the state file, for the same reason the listen address does.
	//
	// IT IS THE SYSTEM'S OWN DECLARATION AND NOT THE PROJECT'S, which is
	// fable's ruling at coop r7/114: `[clock].zone` is what the ENGINE reads
	// the run's instants in and it never crosses the wire, so this is
	// configured separately -- and the two disagreeing is then something a run
	// can FIND rather than something the harness makes impossible.
	//
	// NEVER nil. An unconfigured toy anchors on UTC days, which is the
	// compatibility property: UTC has no transitions, so the planted reset
	// below is arithmetically correct on every day of the year and every
	// profile committed before R7.5 draws exactly what it drew.
	zone *time.Location
}

func main() {
	addr := flag.String("addr", "127.0.0.1:0", "the address to serve on; :0 picks a free port and the chosen one is printed")
	statePath := flag.String("state", "", "the file holding the whole world (required)")
	zoneName := flag.String("zone", "", "the IANA zone this application reads its own instants in; empty means UTC")
	flag.Parse()
	if *statePath == "" {
		fmt.Fprintln(os.Stderr, "toy: --state is required: the whole world lives in one file and nothing else holds it")
		os.Exit(2)
	}
	// THE ZONE IS REFUSED BY NAME AT STARTUP, not at the first invitation: a
	// system that comes up and then fails one route is harder to diagnose than
	// one that will not come up, and the adapter reads this process's stderr.
	//
	// AND THE EMPTY CASE IS SPELLED OUT rather than passed through. Measured
	// twice in this child: `time.LoadLocation("")` SUCCEEDS with a real UTC, so
	// forwarding it would work by accident and read as an oversight -- and the
	// day someone wants an unconfigured toy to be an ERROR, the line to change
	// is this one.
	loc := time.UTC
	if *zoneName != "" {
		var zerr error
		if loc, zerr = time.LoadLocation(*zoneName); zerr != nil {
			fmt.Fprintf(os.Stderr, "toy: --zone %q: %v -- the name is an IANA zone this "+
				"build's database holds\n", *zoneName, zerr)
			os.Exit(2)
		}
	}
	srv, err := newServerInZone(*statePath, loc)
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
	// AND THE ZONE IT READS ITS OWN INSTANTS IN, on STDERR (R7.5 D4c).
	//
	// WHY IT IS PRINTED AT ALL: the configuration was unobservable from
	// outside. The engine derives this from `[clock].zone` and sets the
	// variable the project names, and whether it actually did so could not be
	// seen in a run -- `rumor run` prints a finding's RULE, and the
	// declared-against-configured disagreement and the allowance's own
	// arithmetic both arrive at the same default clause. Three mutants of the
	// derivation survived an arm that compared whole runs across three
	// environments, because the outputs were identical either way.
	//
	// STDERR AND NOT STDOUT, for the line above's reason inverted: the adapter
	// and the rollback provider READ stdout to learn the port, so a second
	// line there would have to be parsed by both. stderr is forwarded to the
	// engine's own stderr, where a reader and an arm can both see it.
	//
	// IT NAMES THE ZONE AND NOT THE FLAG, so an unconfigured toy says UTC
	// rather than saying nothing: "no line" and "UTC" would otherwise be the
	// same observation, which is the absence-is-not-evidence shape this
	// project keeps finding.
	fmt.Fprintf(os.Stderr, "toy: reading instants in %s\n", srv.zone)

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
func newServer(path string) (*server, error) { return newServerInZone(path, time.UTC) }

// newServerInZone is newServer with the zone this application reads its own
// instants in (R7.5).
//
// newServer KEEPS ITS SIGNATURE deliberately: seven callers construct a server
// and a second positional argument would have edited all of them to say "and
// UTC" -- churn that reads as a change at each site while asserting nothing.
// UTC is the default for the reason the field's own comment gives.
func newServerInZone(path string, loc *time.Location) (*server, error) {
	if loc == nil {
		// nil IS NOT UTC HERE EITHER, and this is the guard rather than a
		// cast: `(*time.Location)(nil).String()` is the string "UTC" while any
		// read THROUGH a nil location panics, so a nil arriving here would be
		// a server that looks UTC-configured in every message and dies on the
		// first invitation.
		loc = time.UTC
	}
	srv := &server{path: path, st: newState(), zone: loc}
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
	// SIX MAPS AND SIX GUARDS, and the figure moves with the code: it said
	// "all four" once over five guards (ratchet, coop r7/65), and a figure
	// that disagrees with the code beside it sends the next reader hunting for
	// an exempt map. `state` declares users, events, sessions, invitations,
	// recounts and -- as of R7.5 -- quotas; the list below is complete rather
	// than historical, and the sweep for a sibling was run again when the
	// sixth landed: no nested maps anywhere, and `event` carries only slices,
	// where appending to a nil one is safe.
	//
	// ARMED SIX FOR SIX by TestAStateFileWhoseMapsAreNullReloadsAndStillWrites:
	// each guard removed on its own kills it (ratchet drove all five, where I
	// had driven the two I added; the sixth came with its own).
	if st.Recounts == nil {
		st.Recounts = map[string]string{}
	}
	if st.Invitations == nil {
		st.Invitations = map[string]*invitation{}
	}
	if st.Quotas == nil {
		st.Quotas = map[string]*quota{}
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

// handleUserPath answers what the world holds about ONE user, which is one
// thing: their invitation allowance (R7.5).
//
// A GET AND IT MUTATES NOTHING, which is what makes it an observation rather
// than a step. Where the world holds no record it reports the one the next
// invitation WOULD be judged against -- computed, not stored -- so a rule
// reading this field does not have to carry a case for "nobody has invited
// yet" and cannot be satisfied by an absence.
//
// `reset_local` IS THE FIELD A STATUTE CAN JUDGE. A daily reset is at 00:00 by
// definition, and rego cannot read an instant in a zone, so the application
// reports the local clock FACE of its own next reset and the rule compares a
// string. `day` is the other half: beside `input.clock.local.date` it says
// whether this allowance is anchored on the day the run is actually in.
func (s *server) handleUserPath(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		fail(w, http.StatusMethodNotAllowed, "only GET reads a user's allowance")
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/users/")
	id, tail, ok := strings.Cut(rest, "/")
	if !ok || tail != "invite-quota" {
		fail(w, http.StatusNotFound, "this path answers only /users/<id>/invite-quota")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, found := s.st.Users[id]; !found {
		fail(w, http.StatusNotFound, "no such user")
		return
	}
	writeJSON(w, http.StatusOK, s.quotaBody(id))
}

// quotaBody is what both allowance routes answer, so the one an ACTOR reads
// through its own session and the one addressed by id cannot drift apart.
//
// IT MUTATES NOTHING. Where the world holds no record it reports the one the
// next invitation WOULD be judged against -- `anchorFor`, computed and not
// stored -- so a rule reading these fields needs no case for "nobody has
// invited yet" and cannot be satisfied by an absence.
func (s *server) quotaBody(id string) map[string]any {
	// WHAT THE NEXT INVITATION WOULD BE JUDGED AGAINST, which is the same
	// freshness test `quotaFor` applies -- not the stored record as stored.
	//
	// THIS IS A CORRECTION FOUND BY DRIVING (R7.5 D4c). Reporting the stored
	// record made the reading wrong one nanosecond past EVERY local midnight:
	// this application re-anchors an allowance when it is next SPENT, so a
	// stored day of "yesterday" an instant after midnight is correct
	// behaviour, and a rule comparing it against the run's local date would
	// have reported a defect at every day boundary in the walk. The subject is
	// the day the application BELIEVES it is in, and that is this expression.
	//
	// AND IT LEAVES THE PLANT VISIBLE, which is the whole point: in the spring
	// window the planted reset has NOT been reached -- it lands an hour late --
	// so the stored record is still what the next invitation would use, its day
	// is genuinely yesterday, and the disagreement is the defect rather than
	// the bookkeeping.
	q, stored := s.st.Quotas[id], true
	if q == nil || s.st.Now >= q.ResetAt {
		fresh := s.anchorFor(s.st.Now)
		q, stored = &fresh, false
	}
	return map[string]any{
		"user":        id,
		"day":         q.Day,
		"used":        q.Used,
		"limit":       invitationQuota,
		"remaining":   invitationQuota - q.Used,
		"reset_at":    q.ResetAt,
		"reset_local": s.localFace(q.ResetAt),
		"zone":        s.zone.String(),
		"anchored":    stored,
	}
}

// handleOwnQuota answers the SESSION's own allowance, which is the shape an
// observation can ask for: an observation carries the observing actor and no
// handle for that actor's own id, so a route addressed by id could only be
// asked about somebody else.
//
// THE ACTOR'S OWN ALLOWANCE IS THE SUBJECT, and that is also what makes the
// reading honest: the allowance an invitation spends belongs to the INVITER,
// so the user who would be refused is the one being read.
func (s *server) handleOwnQuota(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		fail(w, http.StatusMethodNotAllowed, "only GET reads an allowance")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.session(r)
	if sess == nil {
		fail(w, http.StatusUnauthorized, "reading your own allowance needs a session")
		return
	}
	writeJSON(w, http.StatusOK, s.quotaBody(sess.UserID))
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/users/", s.handleUserPath)
	mux.HandleFunc("/invite-quota", s.handleOwnQuota)
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
	case len(parts) == 2 && parts[1] == "attendees" && r.Method == http.MethodGet:
		s.eventAttendees(w, r, id)
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

// anchorFor is the allowance this server WOULD give a user at the instant
// `at`, computed and not stored.
//
// ONE ARITHMETIC SITE. The read route reports this where nothing is stored
// yet, so the planted computation appears in exactly one place and a fix or a
// mutant cannot reach the write path and miss the read.
func (s *server) anchorFor(at int64) quota {
	t := time.Unix(0, at).In(s.zone)
	dayStart := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, s.zone)
	return quota{
		Day: t.Format("2006-01-02"),
		// *** THE PLANT (R7.5, class api). *** Twenty-four hours of WALL time
		// from the start of today, where a daily reset means the zone's own
		// next midnight -- `dayStart.AddDate(0, 0, 1)`. The two agree on every
		// day except the two a transition makes 23 or 25 hours long.
		ResetAt: dayStart.Add(24 * time.Hour).UnixNano(),
	}
}

// quotaFor is the inviter's allowance at the world's current instant,
// RE-ANCHORING it when the world has passed the reset this server computed
// (fable, coop r7/90 ruling 2: the plant is one day wide).
//
// THE STALE `Day` IS THE OBSERVABLE. On the spring day the reset is an hour
// late, so for that hour this returns a record whose `Day` is YESTERDAY while
// the zone has already moved on -- which is the disagreement a rule reads
// against `input.clock.local.date`.
func (s *server) quotaFor(userID string) *quota {
	q := s.st.Quotas[userID]
	if q == nil || s.st.Now >= q.ResetAt {
		fresh := s.anchorFor(s.st.Now)
		q = &fresh
		s.st.Quotas[userID] = q
	}
	return q
}

// localFace is the local clock face of an instant in this server's zone,
// "15:04". It is what makes the planted reset READABLE by a statute: a daily
// reset is at 00:00 by definition, so a rule can test this field without zone
// arithmetic a policy cannot do.
func (s *server) localFace(at int64) string {
	return time.Unix(0, at).In(s.zone).Format("15:04")
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
	// THE ALLOWANCE IS SPENT BY THE INVITER (R7.5), and an exhausted one is a
	// REFUSAL: the answer does not match this action's declared `response`, so
	// the walk records a refused step and the ledger gains no claim -- which
	// is right, because the invitation did not happen in the world.
	q := s.quotaFor(sess.UserID)
	if q.Used >= invitationQuota {
		fail(w, http.StatusTooManyRequests, fmt.Sprintf(
			"%s has used %d of %d invitations for %s; the next reset is %s local",
			sess.UserID, q.Used, invitationQuota, q.Day, s.localFace(q.ResetAt)))
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
	// SPENT BEFORE THE SAVE, so the allowance is in the same file write as the
	// invitation it paid for: a count incremented after a failed save would
	// charge a user for an invitation the world does not hold.
	q.Used++
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
	// THE DEFECT: AN UNDECLARED GRACE PERIOD. The check is here, it reads the
	// world's clock, and it compares against the wrong instant -- `ExpiresAt`
	// plus another whole lifetime -- so an invitation is honored for twice as
	// long as the system told everybody it would be.
	//
	// IT IS THE R7 LEDGER ROW (epic): "an invitation honored past its expiry".
	// `GET /invitations/<id>/expiry` still answers `ExpiresAt`, which is the
	// instant the project DECLARES through `[deadline].instants` -- so the
	// system reports one deadline and enforces another, and the statute's
	// `honored_past_expiry` catches exactly that disagreement by reading the
	// join's own time against the reported expiry.
	//
	// ONE DEFECT WIDE, which is this file's rule for every plant. The check
	// is not removed: an invitation a WHOLE EXTRA LIFETIME past its expiry is
	// still refused, so the route has not become unconditional and the arm
	// below drives both sides. A grace of this shape is the ordinary way real
	// systems acquire it -- somebody adds slack to a deadline and nobody
	// updates what the API reports -- which is why it is this rather than a
	// deleted comparison.
	//
	// AND THE COMPARISON STAYS STRICT. `>` is the repository's one reading of
	// "past a deadline" (cmd/toy, internal/engine, examples/toy's statute);
	// moving it to `>=` would plant a SECOND defect about the boundary, and
	// the boundary is the thing R7.3's three landings measure.
	if s.st.Now > inv.ExpiresAt+invitationLifetime {
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
	// AND THE PAGE'S NUMBER WITH IT. A second way in that left Rendered behind
	// would plant the SAME defect twice, in two places, and a shrink could not
	// say which write it had found.
	ev.Rendered = ev.Count
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
	// AND THE PAGE'S NUMBER, so a join is honest in both readings and the
	// omission in leaveEvent is the only place a reader and the API differ.
	ev.Rendered = ev.Count
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
	// THE DEFECT: ev.Rendered IS NOT UPDATED WITH THE COUNT IT CACHES. The
	// API's number drops here and the page keeps the one it last rendered, so a
	// reader is served a count the API has already updated -- the epic's ledger
	// row, planted in R8/111 under fable's ruling at coop rumor/1464.
	//
	// LEAVING IS THE RIGHT WRITE TO PLANT IT IN, and the choice is not
	// arbitrary: a join raises both numbers, so a world that only joins reads
	// the same figure twice and is the honest control. Only a path that LOWERS
	// the count can leave a cache high, and leaveEvent is the one such path a
	// walk can reach -- delete makes the page 404, which is a different reading
	// (`missing`) this statute already carries a rule for.
	//
	// ONE DEFECT WIDE, this file's rule for every plant: one write is absent
	// and nothing else is touched. Every declared READ path stays correct -- the
	// count path and the store path both answer the new number -- and the only
	// CONSUMER of a wrong one is the page's reader.
	//
	// NOT "a reader of the API here is correct", which is what this comment said
	// until ratchet drove it at coop r8/179: "the phrase went, the claim stayed".
	// THIS HANDLER'S OWN REPLY carries `count=1 rendered=2`, because it answers
	// with the event entire. The bytes are honest about both numbers; what makes
	// the defect a reader's is that nothing consumes the stale one but the page.
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

// eventAttendees serves the attendee list, and it is THE ONE THING IN THIS API
// THAT ONLY THE EVENT'S ADMIN MAY HAVE.
//
// IT EXISTS FOR THE SECOND UI-ONLY PLANT (epic's ledger: "template renders an
// item to the wrong role", R9's catcher; fable's shape at coop rumor/1682). That
// row needs an item the API WITHHOLDS and the template leaks, and before this
// path there was none: listEvents filters whole events by visibility and then
// returns each one entire -- attendees included -- so every item on the page was
// one the API already gave the same reader.
//
// AN ADMIN-ONLY PATH RATHER THAN A TIGHTER listEvents, which is fable's ruling
// and the half that keeps the evidence: narrowing what listEvents returns would
// move what every committed reading of this API gets, and R5's rows rest on
// those. A NEW path is read by nobody until something asks for it.
//
// THE ROLE IS THE EVENT'S AND NOT THE RUN'S: roleOf answers admin only for the
// event's own AdminID, so "admin" here is per event, which is the same notion
// the page's gate and visibleTo use. internal/schedule's RoleOf is the venire's
// role and a different namespace; a statute comparing the two would be comparing
// two things that share a word.
//
// 403 AND NOT 404, deliberately: anyone who can see the event already knows it
// exists -- the page told them -- so withholding the list is not a secret about
// existence, and a 404 here would be indistinguishable from a deleted event,
// which is a reading this project gives `missing` instead.
func (s *server) eventAttendees(w http.ResponseWriter, r *http.Request, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.session(r)
	if sess == nil {
		fail(w, http.StatusUnauthorized, "the attendee list needs a session")
		return
	}
	ev, ok := s.st.Events[id]
	if !ok || ev.Deleted {
		fail(w, http.StatusNotFound, "no such event")
		return
	}
	if roleOf(ev, sess.UserID) != roleAdmin {
		fail(w, http.StatusForbidden, "the attendee list is the event admin's")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"event": ev.ID, "attendees": ev.Attendees})
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
	// THE DEFECT: THE TEMPLATE RENDERS THE ATTENDEE LIST TO EVERY ROLE that can
	// see the event, while the API serves it to the event's ADMIN alone
	// (eventAttendees). That is the epic's ledger row "template renders an item
	// to the wrong role", planted here in R8/111 on fable's shape at coop
	// rumor/1682, with R9's browser stage as its catcher.
	//
	// ONE TEMPLATE WIDE, this file's rule for every plant: the gate above is
	// untouched and correct -- a role the event is not visible to still gets a
	// 404 and no page at all -- and the only wrong thing in the system is which
	// roles this one element reaches. The API withholds it, the store never
	// offers it, and a reader is the only party who can see it leak.
	//
	// AN EMPTY LIST IS NOT THE DEFECT, which is why the element is rendered
	// even when there is nobody in it: a guest looking at an event with no
	// attendees learns nothing, so the finding is a NON-EMPTY list in front of a
	// non-admin (fable's rule at rumor/1682). Rendering the element only when it
	// is non-empty would hide the honest case and make the plant's absence
	// indistinguishable from an empty world.
	attendees := ""
	for _, a := range ev.Attendees {
		attendees += "<li>" + html.EscapeString(a) + "</li>\n"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	page := `<!doctype html>
<title>` + html.EscapeString(ev.Title) + `</title>
<h1 id="title">` + html.EscapeString(ev.Title) + `</h1>
<p id="role">` + role + `</p>
<p id="count">` + strconv.Itoa(ev.Rendered) + ` of ` + strconv.Itoa(ev.Capacity) + `</p>
<ul id="attendees">
` + attendees + `</ul>
<form id="join" method="post" action="/ui/events/` + html.EscapeString(ev.ID) + `/join?token=` + html.EscapeString(sess.Token) + `">
<button type="submit">join</button>
</form>
`
	if _, err := io.WriteString(w, page); err != nil {
		// Nowhere to report it to the client; the status is already sent.
		fmt.Fprintf(os.Stderr, "toy: writing the page for %s: %v\n", ev.ID, err)
	}
}
