// Command toy-adapter speaks R4's wire contract on stdio and drives the toy
// over HTTP (R5.2, issue 73). It is the `[adapter].command` examples/toy names.
//
// WHO OWNS THE PROCESS, and why this has two modes. The rollback provider
// restarts the toy and swaps its state file, so the provider has to own the
// process; the adapter only needs an address to talk to, and takes it from
// TOY_ADDR -- an env var, which is exactly what platform.toml's `[adapter].env`
// and wire.Spawn's named environment exist to pass. When TOY_ADDR is absent the
// adapter starts a toy of its own and uses that, because `rumor validate`
// spawns the ADAPTER ALONE: no engine, no provider, and four of the eighteen
// arms need a describe to answer. A mode that only worked under the engine
// would make the toy unvalidatable, which issue 73 requires it to be.
//
// AN OBSERVATION'S SUBJECT RIDES IN THE QUERY, and that is a convention this
// file invents because the contract leaves room for it rather than because it
// is obviously right. R4's ObserveRequest carries Query, Source and Token and
// NO args, while an action carries Args -- so `event_count` of a particular
// event has nowhere to put the event but the query string. The shape here is
// `name/subject`, with the subject being the system's own id (the engine
// resolves a handle before the message is written, as it does for Args). Raised
// for quill and fable in coop r5.toy rather than settled here: if the engine
// cannot carry a subject, a parameterized observation is not expressible and
// that is the contract's to say, not the toy's to work around.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

// The actions and observations this adapter can serve. They are the surface
// `describe` reports, and examples/toy's bindings declare the same set: the two
// convict each other, which is validate's arms 1 and 2.
var (
	actions = []wire.Action{
		{Name: "create_user", Context: "none"},
		{Name: "create_event", Context: "api"},
		{Name: "join_event", Context: "api"},
		{Name: "leave_event", Context: "api"},
		{Name: "list_attendees", Context: "api"},
		{Name: "delete_event", Context: "api"},
		{Name: "recount_event", Context: "api"},
		{Name: "invite_user", Context: "api"},
		{Name: "accept_invitation", Context: "api"},
		{Name: "join_event_ui", Context: "browser"},
	}
	observations = []wire.Observation{
		{Name: "event_count", Source: wire.SourceAPI},
		// THE WORKER'S OWN COUNT, asked about a RECOUNT and not about an event.
		// It is an `api` observation: an actor reads it through its own
		// session, where `direct` is the engine's look into the store.
		{Name: "event_tally", Source: wire.SourceAPI},
		{Name: "store_event_count", Source: wire.SourceDirect},
		// THE PAGE'S OWN READING, source `browser` (R8/111). It is the only
		// observation here that reads a DOCUMENT rather than a JSON route, and
		// the one whose value the application RENDERS rather than computes: the
		// count the page shows a reader is what a stale per-event cache would
		// get wrong while /events/<id>/count stayed right.
		// WHERE A BROWSER READS THESE, reported because this adapter is the one
		// thing that knows its own UI (fable, coop r9/120). The page is
		// relative to `base` with the subject as a placeholder, and the element
		// is an ID on that page -- cmd/toy renders `<p id="count">` and
		// `<ul id="attendees">`, so these are the ids a driver reads.
		//
		// THE SAME PAIR THIS ADAPTER'S OWN READ USES, which is what keeps the
		// two oracles about one page: a.page() fetches /ui/events/<id> and
		// pageCount/pageAttendees parse those two ids out of the served HTML.
		// A driver reading a DIFFERENT element would make the browser's answer
		// and the adapter's answer two subjects rather than two readings.
		//
		// AND WHAT THE TEXT MEANS, declared because the engine converts by this
		// and never by what the text looks like (fable, coop r9/122: "NO
		// SNIFFING"). Both are counts, so both are `number`: the element holds
		// the figure alone and the engine makes it a number in one place,
		// refusing by name any text that is not wholly numeric.
		//
		// `page_attendees` IS A `count` AND NOT THE NAMES, which is the
		// observation's own rule one block down -- the finding is that a
		// non-admin was SHOWN the list at all, so the names add nothing a rule
		// reads and would put other actors' ids in every report -- and it is
		// the reading fable added at coop r9/124 for exactly this element: the
		// <li> children are what pageAttendees has counted from the served HTML
		// all along, so the driver counting them from the live DOM keeps the two
		// oracles comparable and the SUT unchanged.
		{Name: "page_count", Source: wire.SourceBrowser,
			Page: "/ui/events/{subject}", Element: "count", Type: wire.ObservedNumber},
		{Name: "page_attendees", Source: wire.SourceBrowser,
			Page: "/ui/events/{subject}", Element: "attendees", Type: wire.ObservedCount},
		// THE EXPIRY, whose value is an INSTANT and not a count: the only
		// observation in this adapter that answers a time. `api`, because an
		// actor reads it through its own session the way it reads a count.
		{Name: "invitation_expiry", Source: wire.SourceAPI},
		// THE ALLOWANCE, TWO READINGS OF ONE SUBJECT (R7.5 D4). Both are of
		// the observing actor's OWN allowance -- read through its session,
		// because an observation carries no handle for the actor's own id --
		// and both are STRINGS rather than counts: a local clock face and a
		// local date.
		//
		// TWO AND NOT ONE, because they fail differently. `reset` is wrong on
		// BOTH transition days, since a reset computed in wall time lands at
		// 01:00 or 23:00 where a daily reset means 00:00. `day` is wrong only
		// in the spring window, where the late reset leaves the application
		// anchored on YESTERDAY -- and it is the reading a statute compares
		// against `input.clock.local.date`, so the two oracles are the
		// application's own arithmetic and the engine's zone reading.
		{Name: "invite_quota_reset", Source: wire.SourceAPI},
		{Name: "invite_quota_day", Source: wire.SourceAPI},
		// AND THE SAME RESET AS AN INSTANT (R7.5 D4c), which is the reading a
		// WALK needs rather than a rule: `[deadline].instants` names an
		// observation whose value IS an instant, and the engine then lands a
		// step one tick before it, on it, and one tick past it. The clock face
		// above is what a statute can judge; this is what makes the walk
		// arrive at the hour where the judgement differs.
		{Name: "invite_quota_reset_at", Source: wire.SourceAPI},
	}
	contexts = []string{"api", "browser"}
	// THE STORE IS STATEFUL AND THE API IS NOT, which is the distinction
	// validate's rollback-coverage arm compares against platform.toml's
	// [[rollback]] entries: the file is the world, so `store` is the component
	// a provider has to cover and `api` is the process in front of it.
	components = []wire.Component{
		{Name: "api", Stateful: false},
		{Name: "store", Stateful: true},
	}
	// EMPTY IS AN ANSWER (docs/contract.md): the toy offers no fault injection,
	// no volume target and no upgrade in R5, so these are empty rather than
	// absent, and examples/toy's `[uses]` asks for none of them. R19 and R21 are
	// where they arrive.
	capabilities = wire.Capabilities{
		Faults:  []string{},
		Volume:  []string{},
		Upgrade: []string{},
	}
)

type adapter struct {
	base   string // http://host:port of the toy
	client *http.Client
	child  *exec.Cmd
	// stateDir is the temp directory this adapter MADE, and only that. A
	// --state path the caller supplied is the caller's to remove, so this
	// stays empty there and close() has nothing to delete.
	stateDir string

	// THE REQUEST BUDGET AND WHAT IT HAS ACTUALLY COST, which is the half
	// nobody could see. ratchet measured at coop r5/200 that the headroom of
	// this timeout "is not measurable from outside the adapter": the test's
	// whole `go test` invocation is 6-8s warm, the budget governs ONE request
	// inside it, and NEITHER figure gives the margin between them. So the
	// constant was chosen without a measurement and could not be validated
	// where it sat -- and raising it on the same reasoning that set it would
	// just move an unmeasured guess, however much less often it failed.
	//
	// These three fields are that measurement, taken from inside.
	budget  time.Duration
	calls   int
	slowest time.Duration
	// slowestWhat is the route that cost `slowest`, because a headroom figure
	// with no route attached cannot be acted on.
	slowestWhat string

	// THE CLOCK IS NOT A FIELD HERE ANY MORE, and the absence is the R7.4
	// change worth reading: this adapter held an `int64` of its own until
	// 103, which was honest while nothing downstream read it -- its comment
	// said exactly that -- and would have been a lie the moment the toy
	// started expiring invitations by a time the adapter was keeping. The
	// world's clock is the WORLD's: `POST /clock` on the toy, whose state
	// file carries it, so a rollback restores the time with everything else.
	// This adapter forwards and answers what came back.
}

func main() {
	toyBin := flag.String("toy", "toy", "the toy binary to start when TOY_ADDR is not set")
	state := flag.String("state", "", "the state file for a toy this adapter starts; a temp file when empty")
	flag.Parse()

	a, err := open(*toyBin, *state)
	if err != nil {
		// A FAULT ENVELOPE, NOT A CRASH: the engine and validate both read one
		// line per request, and an adapter that dies before the first request
		// reads as "no answer" rather than as the provisioning finding it is.
		// So say it on stderr, where the spawner quotes it, and exit non-zero.
		fmt.Fprintf(os.Stderr, "toy-adapter: %v\n", err)
		os.Exit(1)
	}
	defer a.close()

	if err := a.serve(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "toy-adapter: %v\n", err)
		// CLOSED BY HAND because os.Exit runs no deferred function: the defer
		// above never fires on this path, so without this the toy AND its
		// directory leak on exactly the runs that went wrong.
		a.close()
		os.Exit(1)
	}
}

// open starts a toy unless TOY_ADDR names a running one.
//
// A DEFERRED CLEANUP ON THE FAILURE PATHS, so the temp directory cannot
// outlive a failed open. Every return below MkdirTemp is an error path and each
// one used to leave a directory behind; one deferred close covers them together
// and covers whatever path is added next, which naming them one at a time would
// not. On the happy path the caller's own close() does it (quill, coop r5/102:
// the leak was on every run, clean ones included -- 11 of 11).
//
// THE FLAG RATHER THAN A NAMED RETURN, and my first version got this wrong.
// With `(a *adapter, err error)` the deferred function reads `a` AFTER
// `return nil, fmt.Errorf(...)` has already assigned nil to it, so the cleanup
// called close() on a nil adapter and panicked -- caught by the failing-open
// cell in the test beside this, which is the cell that exists for this path.
// A local flag reads nothing the returns have touched.
// startTimeout bounds how long the toy may take to print its address.
//
// A VAR AND NOT A CONST so the cell beside this can drive the expiry without
// spending twenty seconds of suite time on it. The value matches
// internal/rollback/toyrestart's own startTimeout deliberately: the two drive
// the same binary through the same first line, and two different budgets for
// one readiness would be two different answers to "is the toy slow or stuck".
var startTimeout = 20 * time.Second

func open(toyBin, state string) (*adapter, error) {
	a := &adapter{budget: requestBudget, client: &http.Client{Timeout: requestBudget}}
	opened := false
	defer func() {
		if !opened {
			a.close()
		}
	}()
	if addr := os.Getenv("TOY_ADDR"); addr != "" {
		a.base = "http://" + strings.TrimPrefix(strings.TrimPrefix(addr, "http://"), "https://")
		opened = true
		return a, nil
	}
	if state == "" {
		dir, err := os.MkdirTemp("", "toy-adapter-*")
		if err != nil {
			return nil, fmt.Errorf("making a state directory: %w", err)
		}
		a.stateDir = dir
		state = filepath.Join(dir, "state.json")
	}
	// THE ZONE THE SYSTEM IS CONFIGURED WITH TRAVELS AS AN ENV VAR (fable's
	// ruling (a), coop r7/114), beside TOY_ADDR and for the same reason: this
	// is the SUT's own configuration, not the project's declaration.
	//
	// `[clock].zone` NEVER CROSSES THE WIRE (issue 115), so the adapter cannot
	// learn it from the engine and does not try. An operator sets TOY_ZONE to
	// whatever the system under test is configured with, which is exactly what
	// makes a disagreement with the project's declaration FINDABLE -- and
	// unset means the toy anchors on UTC days, which is its own default and
	// not a guess made here.
	args := []string{"--addr", "127.0.0.1:0", "--state", state}
	if zone := os.Getenv("TOY_ZONE"); zone != "" {
		args = append(args, "--zone", zone)
	}
	cmd := exec.Command(toyBin, args...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("piping %s: %w", toyBin, err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting %s (set TOY_ADDR to use a running one): %w", toyBin, err)
	}
	a.child = cmd
	// THE ADDRESS IS READ, NOT GUESSED. The toy binds :0 and prints what it
	// got, so two runs on one box cannot collide over a port.
	//
	// AND THE READ IS BOUNDED, WHICH IT WAS NOT UNTIL R8.4. A bare ReadString
	// blocks for as long as the child declines to print: a toy that STARTS and
	// then stalls before its first line holds this open with no deadline at
	// all. The engine's per-request budget cannot help here, and that is the
	// part worth knowing -- this runs while the adapter is being OPENED, before
	// any request exists to time out.
	//
	// THE SIBLING HAD IT ALL ALONG. internal/rollback/toyrestart drives the
	// same binary for the same reason, with the same constant and an explicit
	// kill, and its comment says "bounded" in those words. So this is the
	// driver that did not get the timeout, which is fable's standing rule at
	// coop rumor/1673 read onto the one place it was missing.
	//
	// NO EXPLICIT KILL, unlike the sibling, and the reason is above this
	// function: the deferred cleanup runs close() on every path that does not
	// set `opened`, and close() stops the child this adapter started. The
	// sibling kills by hand because it has no such defer.
	type started struct {
		line string
		err  error
	}
	ch := make(chan started, 1)
	go func() {
		l, rerr := bufio.NewReader(out).ReadString('\n')
		ch <- started{l, rerr}
	}()
	var line string
	select {
	case r := <-ch:
		if r.err != nil {
			return nil, fmt.Errorf("reading the address %s printed: %w", toyBin, r.err)
		}
		line = r.line
	case <-time.After(startTimeout):
		return nil, fmt.Errorf("%s printed no address in %s, so the world never opened",
			toyBin, startTimeout)
	}
	addr := addressOf(line)
	if addr == "" {
		return nil, fmt.Errorf("%s printed %q, which names no address", toyBin, strings.TrimSpace(line))
	}
	a.base = "http://" + addr
	opened = true
	return a, nil
}

// addressOf reads the address out of the toy's first line, which is
// "toy: listening on 127.0.0.1:PORT state=PATH".
func addressOf(line string) string {
	for _, field := range strings.Fields(line) {
		if strings.Contains(field, ":") && !strings.HasSuffix(field, ":") && !strings.Contains(field, "=") {
			return field
		}
	}
	return ""
}

// close stops the toy this adapter started and removes the directory it made.
//
// THE CHILD DIES FIRST, which is the order and not an accident: the toy holds
// state.json open and writes it, so removing the directory under a live toy
// would race its last write. Kill then Wait then RemoveAll.
// requestBudget is how long ONE request to the toy may take before this adapter
// gives up.
//
// THE NUMBER IS CHOSEN FROM THE MEASUREMENT BELOW IT, which is the whole point
// of instrumenting first (ratchet, coop r5/200): "raising 10s to 60s on the
// same reasoning that set it at 10s just moves an unmeasured guess, even though
// it will certainly fail less often."
//
// THE MEASUREMENT, taken 2026-10-26 at vm.loadavg 7.09 on a FOUR-CPU box that
// three jails share -- so a busy one, which is the only kind available here:
//
//	one request to the toy        median   2.0ms   worst  24.2ms  (30 requests)
//	process start to first answer median  10.4ms   worst  27.2ms  (12 starts)
//
// AND THAT CHANGES WHAT THE NUMBER IS FOR. The old 10s already stood at roughly
// 370 TIMES the worst work observed, so the budget was never below the work it
// measures -- which was the premise fable's r5/196 placement rested on and
// which rook and ratchet had already withdrawn as unmeasured at r5/199 and
// /200, load having failed to predict which run paid. An expiry here therefore
// means a STALL of ten seconds or more, not a request that ran slightly long.
//
// SO 60s IS STALL TOLERANCE AND NOT A HEADROOM FIGURE, and saying which is the
// point: it buys survival of a 10-to-60s stall and nothing else, and it fixes
// no cause. What makes it safe rather than merely larger is the half beside it
// -- an expiry now names the budget and the elapsed and says the instrument
// gave up, so a 60s wait reads as this adapter's patience running out rather
// than as a verdict on the toy. The cause of the original stalls remains
// UNMEASURED and is recorded as such in the R5 gate record.
const requestBudget = 60 * time.Second

func (a *adapter) close() {
	// THE HEADROOM IS PUBLISHED ONCE, AT CLOSE, AND ON STDERR. Stdout is the
	// wire -- one JSON response per line -- so a figure printed there would be
	// a protocol violation; stderr is where this adapter already says things to
	// a human.
	//
	// ONCE AND NOT PER REQUEST, which is ratchet's flood at coop main 12/259: a
	// per-poll line for a persistent condition drowns the thing it reports.
	// One summary names the count, the worst route and the margin, which is
	// everything needed to choose the number.
	if a.calls > 0 {
		head := a.budget - a.slowest
		_, _ = fmt.Fprintf(os.Stderr, "toy-adapter: %d request(s), slowest %s (%s), budget %s, "+
			"headroom %s\n", a.calls, a.slowest.Round(time.Millisecond), a.slowestWhat,
			a.budget, head.Round(time.Millisecond))
	}
	if a.child != nil && a.child.Process != nil {
		_ = a.child.Process.Kill()
		_ = a.child.Wait()
	}
	if a.stateDir != "" {
		_ = os.RemoveAll(a.stateDir)
		a.stateDir = ""
	}
}

// serve reads one request per line and writes one response per request, which
// is the whole transport. The loop is internal/fake's, deliberately: two
// adapters answering the same contract should not disagree about framing.
func (a *adapter) serve(in io.Reader, out io.Writer) error {
	scan := bufio.NewScanner(in)
	scan.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	enc := json.NewEncoder(out)
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if line == "" {
			continue
		}
		if err := enc.Encode(a.answer([]byte(line))); err != nil {
			return fmt.Errorf("writing a response: %w", err)
		}
	}
	if err := scan.Err(); err != nil {
		return fmt.Errorf("reading requests: %w", err)
	}
	return nil
}

func fault(id string, kind wire.FaultKind, field, detail string) any {
	return map[string]any{
		"kind":     string(wire.KindError),
		"contract": wire.ContractVersion,
		"id":       id,
		"error": map[string]any{
			"kind":   string(kind),
			"field":  field,
			"detail": detail,
		},
	}
}

// recordPath names a file this adapter appends one line to per request it
// decodes: the KIND, and the action where the request carries one.
//
// WHY IT EXISTS, and it is not a debugging aid. R9.1's H2 routes a
// browser-context action through `flow`, and the arm that proves that happened
// read only the run's own summary -- which a two-line change defeats, because
// the field behind that summary can be set from the action's CONTEXT and then
// says "flow" with the routing off (rook's M1+M2, coop rumor/1896). An arm on
// THIS side of the wire cannot be fooled that way: the adapter either decoded a
// `flow` or it did not, and the toy's UI route is posted to identically by both
// operations, so the KIND is the only thing that tells them apart out here
// (fable, coop r9/57 item two).
//
// OFF UNLESS ASKED, append-only, and a write failure is SILENT by design: this
// is an observation surface for an arm, and an adapter that died because a test
// harness could not open a file would turn a missing witness into a failed run.
const recordPath = "TOY_ADAPTER_RECORD"

func (a *adapter) note(env wire.Envelope, action string) {
	path := os.Getenv(recordPath)
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = fmt.Fprintf(f, "%s %s\n", env.Kind, action)
}

func (a *adapter) answer(line []byte) any {
	msg, err := wire.DecodeRequest(line)
	if err != nil {
		return fault(idOf(line), wire.FaultField, "", err.Error())
	}
	env := msg.Env()
	// RECORDED ON ARRIVAL, before anything is served: what the arm asks is what
	// REACHED this side, and a line written only on success would make a
	// refusal look like a request that never came.
	switch m := msg.(type) {
	case wire.ExecuteRequest:
		a.note(env, m.Action)
	case wire.FlowRequest:
		a.note(env, m.Action)
	default:
		a.note(env, "")
	}
	switch m := msg.(type) {
	case wire.DescribeRequest:
		return wire.DescribeResponse{
			Envelope:     wire.Envelope{Kind: wire.KindDescribe, Contract: wire.ContractVersion, ID: env.ID},
			Actions:      actions,
			Observations: observations,
			Contexts:     contexts,
			Components:   components,
			Capabilities: capabilities,
			// THE WORLD'S ADDRESS, REPORTED BECAUSE THIS PROCESS IS THE ONE
			// THAT KNOWS IT (fable, coop r9/111). `a.base` is set either from
			// TOY_ADDR or from the address the toy this adapter started
			// printed, and a browser driving a declared flow has no other way
			// to learn it -- the engine sees a command and an environment, not
			// a port chosen at runtime.
			//
			// IT IS WHATEVER THIS ADAPTER IS ACTUALLY TALKING TO, not a
			// configured intention: both paths above assign `a.base` from the
			// address in hand, so a toy that moved would move this with it.
			Base: a.base,
		}
	case wire.SessionRequest:
		return a.session(env.ID, m)
	case wire.ExecuteRequest:
		return a.execute(env.ID, m)
	case wire.ObserveRequest:
		return a.observe(env.ID, m)
	case wire.ClockRequest:
		return a.clockOp(env.ID, m)
	case wire.FlowRequest:
		return a.flow(env.ID, m)
	}
	// UNREACHABLE WHILE THIS ADAPTER SERVES EVERY REQUEST KIND, which it does as
	// of R9.1's H2 -- and the claim is GUARDED rather than asserted here:
	// TestEveryRequestKindWireCanSendHasACaseHere drives every kind
	// `wire.Kinds("request")` reports and fails if one lands below. The last
	// default in this tree to carry an unreachability comment had a FALSE one
	// (internal/fake, found by rook at coop r9/39), so this one does not get to
	// be a sentence nobody checks. It is kept because a kind added to the
	// protocol without a case above is this adapter's gap, and the run should
	// say so rather than taking the engine down.
	//
	// THE WORDING AND THE FIELD COME FROM package wire, not from here: this
	// refusal is what the contract version's own note rests on, and a sentence
	// each adapter spelled for itself is one that can drift out from under that
	// argument one adapter at a time (fable, coop r9/40 part two).
	f := wire.UnservedKind(env.Kind)
	return fault(env.ID, f.Kind, f.Field, f.Detail)
}

// clockOp moves the WORLD's clock and answers WHAT IT NOW HOLDS, which is the
// field fable ruled the ledger writes from (coop r7/51): "an adapter that
// rounds forward answered for the world and the ledger records the world".
//
// IT FORWARDS AND DOES NOT COMPUTE, which is the R7.4 change. The op and the
// delta go to the toy exactly as they arrived and the toy's own number comes
// back, so `advance`'s relativity is the WORLD's property and not this
// process's: an adapter holding its own copy would answer correctly for a run
// and wrongly for anything that read the toy, and the thing about to read it
// is an invitation's expiry. This adapter neither rounds nor clamps -- and the
// engine refuses a `set` that comes back changed, so one that started to
// would be caught rather than believed.
//
// THE OP IS CHECKED BEFORE THE CALL, so an unknown one never reaches the toy.
// The toy refuses it too (it is an application and anything may call it); this
// refusal is the one the contract owes, by name and naming both words.
func (a *adapter) clockOp(id string, m wire.ClockRequest) any {
	switch m.Op {
	case wire.ClockSet, wire.ClockAdvance:
	default:
		// THE DECODER CHECKS FIELD PRESENCE, NOT A VARIANT'S VALUE, so an
		// unknown op arrives intact and is named rather than guessed at.
		return fault(id, wire.FaultField, "op",
			fmt.Sprintf("%q is not a clock op; this protocol has %q and %q",
				m.Op, wire.ClockSet, wire.ClockAdvance))
	}
	var out struct {
		T int64 `json:"t"`
	}
	// `t` IS SENT EVEN WHEN IT IS ZERO: `prune` drops nil values and not zero
	// ones, so `set 0` -- the run's own epoch, which is a legal instant --
	// reaches the toy as a number rather than as an absent key its strict
	// decoder would read as zero by accident.
	code, err := a.call(http.MethodPost, "/clock", "",
		map[string]any{"op": string(m.Op), "t": m.T}, &out)
	if err != nil {
		return fault(id, wire.FaultInternal, "", err.Error())
	}
	if code != http.StatusOK {
		return fault(id, wire.FaultInternal, "op",
			fmt.Sprintf("the toy answered %d to a %q of %dns", code, m.Op, m.T))
	}
	return wire.ClockResponse{
		Envelope: wire.Envelope{Kind: wire.KindClock, Contract: wire.ContractVersion, ID: id},
		T:        out.T,
	}
}

// idOf recovers the id from a line the decoder refused, so a fault can be
// correlated with the request that caused it. internal/fake does the same.
func idOf(line []byte) string {
	var probe struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(line, &probe); err != nil {
		return ""
	}
	return probe.ID
}

func (a *adapter) session(id string, m wire.SessionRequest) any {
	var body struct {
		Token string `json:"token"`
	}
	code, err := a.call(http.MethodPost, "/sessions", "", map[string]any{
		"actor": m.Actor, "context": m.Context,
	}, &body)
	if err != nil {
		return fault(id, wire.FaultSession, "", err.Error())
	}
	if code != http.StatusCreated {
		return fault(id, wire.FaultSession, "actor",
			fmt.Sprintf("the toy refused a session for %q in context %q with status %d", m.Actor, m.Context, code))
	}
	return wire.SessionResponse{
		Envelope: wire.Envelope{Kind: wire.KindSession, Contract: wire.ContractVersion, ID: id},
		Token:    body.Token,
	}
}

func (a *adapter) execute(id string, m wire.ExecuteRequest) any {
	type plan struct {
		method string
		path   string
		body   map[string]any
		// creates names the entity this action makes, which is the key the
		// engine reads an id back under. Empty when the action creates nothing.
		creates string
	}
	arg := func(k string) any { return m.Args[k] }
	subject, _ := m.Args["event"].(string)

	var p plan
	switch m.Action {
	case "create_user":
		p = plan{http.MethodPost, "/users", map[string]any{"name": arg("name")}, "user"}
	case "create_event":
		p = plan{http.MethodPost, "/events", map[string]any{
			"title": arg("title"), "capacity": arg("capacity"), "visible_to": arg("visible_to"),
		}, "event"}
	case "join_event":
		p = plan{http.MethodPost, "/events/" + subject + "/join", nil, ""}
	case "leave_event":
		p = plan{http.MethodDelete, "/events/" + subject + "/join", nil, ""}
	case "list_attendees":
		// THE HONEST READING OF THE WITHHELD ITEM. It creates nothing and the
		// list itself is not carried back: the walk judges the PAGE's copy
		// through page_attendees, and an action that returned the names would
		// put them in a second place for no reader.
		p = plan{http.MethodGet, "/events/" + subject + "/attendees", nil, ""}
	case "delete_event":
		p = plan{http.MethodDelete, "/events/" + subject, nil, ""}
	case "recount_event":
		// IT CREATES A `recount`, which is what gives the observation of the
		// worker's tally a subject of its own: the handle the walk mints for
		// this step resolves to the recount's id, and nothing else in a run
		// mints that kind. An observation declaring the EVENT handle instead
		// is makeable on every step that touches an event (ibis, coop r6/100).
		p = plan{http.MethodPost, "/events/" + subject + "/recount", nil, "recount"}
	case "invite_user":
		// THE INVITEE RIDES IN THE BODY as the id the engine resolved from the
		// `user` handle: no run-local handle crosses the wire, which is the
		// contract's own rule, so what the toy receives is its own id.
		p = plan{http.MethodPost, "/events/" + subject + "/invitations",
			map[string]any{"user": arg("user")}, "invitation"}
	case "accept_invitation":
		inv, _ := m.Args["invitation"].(string)
		if inv == "" {
			return fault(id, wire.FaultField, "args.invitation",
				"accept_invitation needs an invitation id in args and got none")
		}
		p = plan{http.MethodPost, "/invitations/" + inv + "/accept", nil, ""}
	case "join_event_ui":
		// THE UI ACTION GOES THROUGH THE PAGE'S OWN ROUTE, not the API's, so
		// what the flow declares and what the browser would do in R9 are the
		// same path.
		p = plan{http.MethodPost, "/ui/events/" + subject + "/join", nil, ""}
	default:
		return fault(id, wire.FaultAction, "action",
			fmt.Sprintf("%q is not an action this adapter serves; it serves %s", m.Action, names()))
	}
	if strings.Contains(p.path, "//") {
		return fault(id, wire.FaultField, "args.event",
			fmt.Sprintf("%q needs an event id in args and got none", m.Action))
	}

	var out map[string]any
	code, err := a.call(p.method, p.path, m.Token, p.body, &out)
	if err != nil {
		return fault(id, wire.FaultInternal, "", err.Error())
	}
	if code >= 400 {
		// A REFUSAL BY THE SYSTEM IS NOT A PROTOCOL FAULT. The toy saying "at
		// capacity" is the system under test behaving, and the engine has to
		// see it as a result rather than as an adapter that could not run --
		// which is the same boundary docs/contract.md draws for a decode
		// failure being could-not-run and a project being wrong being a
		// refusal. So the status and the body ride back in the response.
		return wire.ExecuteResponse{
			Envelope: wire.Envelope{Kind: wire.KindExecute, Contract: wire.ContractVersion, ID: id},
			IDs:      map[string]any{},
			Response: map[string]any{"status": code, "body": out},
		}
	}
	ids := map[string]any{}
	if p.creates != "" {
		if made, ok := out["id"]; ok {
			ids[p.creates] = made
		}
	}
	return wire.ExecuteResponse{
		Envelope: wire.Envelope{Kind: wire.KindExecute, Contract: wire.ContractVersion, ID: id},
		IDs:      ids,
		Response: map[string]any{"status": code, "body": out},
	}
}

// flow drives a declared UI flow by walking its steps against the toy's own UI
// ROUTES, which is this adapter standing in for a browser until R9.1b replaces
// it with one (fable, coop r9/40 part one).
//
// WHY IT EXISTS AT ALL, and it is not a convenience: examples/toy declares
// `join_event_ui` with a browser context AND a flow, and as of R9.1's H2 the
// walk routes such an action through `flow` rather than `execute`. A
// declaration the walk now honors is one the toy must honor too -- without this
// case, any walk that DREW that action died with "flow is a kind this adapter
// does not serve", which rook's adapterserves.py reports as a latent trap from
// the pushed tip and `clicking`'s own arm reproduces at step 23.
//
// IT IS NO MORE HONEST THAN THE EXECUTE CASE WAS. That case already posts to
// /ui/events/<id>/join and its comment says so: "THE UI ACTION GOES THROUGH THE
// PAGE'S OWN ROUTE, not the API's, so what the flow declares and what the
// browser would do in R9 are the same path". This walks the same path from the
// steps instead of from the args. What it CANNOT do is render, read the DOM or
// evaluate a page condition -- so a `wait` is a no-op here and the arms that
// care whether a condition was truly observed cannot be satisfied by this
// answer, exactly as internal/fake's own flow case says of itself.
//
// `reached` IS len(steps) OR WHERE IT STOPPED, and nothing between: this
// adapter either walks the whole flow or answers a fault, because a toy route
// that returned 4xx is the SYSTEM refusing and belongs in `response` the way
// the execute case puts it there.
func (a *adapter) flow(id string, m wire.FlowRequest) any {
	if len(m.Steps) == 0 {
		return fault(id, wire.FaultAction, "steps",
			fmt.Sprintf("a flow for %q carried no steps, so there is nothing to drive", m.Action))
	}
	if m.From < 0 || m.From >= len(m.Steps) {
		return fault(id, wire.FaultAction, "from",
			fmt.Sprintf("a flow for %q asked to resume at %d with %d step(s)", m.Action, m.From, len(m.Steps)))
	}

	// THE PATH IS CARRIED BETWEEN STEPS, because a `click` acts on the page a
	// `navigate` opened. A resume starts PAST the navigate, so the path has to
	// be read from the steps before `from` as well -- walking them for their
	// path without PERFORMING them is what "the request before it is never
	// re-sent" means here.
	path := ""
	var out map[string]any
	code := 0
	for i, st := range m.Steps {
		kind, _ := st["step"].(string)
		switch kind {
		case bindings.StepNavigate:
			to, _ := st["to"].(string)
			if to == "" {
				return fault(id, wire.FaultField, "steps",
					fmt.Sprintf("step %d of %q navigates nowhere", i+1, m.Action))
			}
			if strings.Contains(to, "{") {
				// AN UNRESOLVED PLACEHOLDER IS THE ENGINE'S FAULT AND IS SAID
				// SO: the walk substitutes a flow's `{kind}` from the handle
				// its own argument chose, and an adapter handed the literal has
				// no entity to point at. Guessing would make a 404 look like
				// the system's answer.
				return fault(id, wire.FaultField, "steps",
					fmt.Sprintf("step %d of %q navigates to %q, which still carries a placeholder: "+
						"the engine resolves those before sending", i+1, m.Action, to))
			}
			path = to
		case bindings.StepClick:
			if i < m.From {
				continue
			}
			target, _ := st["target"].(string)
			if path == "" {
				return fault(id, wire.FaultField, "steps",
					fmt.Sprintf("step %d of %q clicks before anything navigated", i+1, m.Action))
			}
			var err error
			code, err = a.call(http.MethodPost, path+"/"+target, m.Token, nil, &out)
			if err != nil {
				return fault(id, wire.FaultInternal, "", err.Error())
			}
			if code >= 400 {
				return wire.FlowResponse{
					Envelope: wire.Envelope{Kind: wire.KindFlow, Contract: wire.ContractVersion, ID: id},
					Reached:  i,
					Stopped:  wire.StopStep,
					Response: nil,
				}
			}
		case bindings.StepWait:
			// A NO-OP, AND NAMED AS ONE. The toy answers its UI routes
			// synchronously, so the condition a flow waits for is already true
			// when the click returned -- there is no page to poll. A real
			// driver evaluates it, which is R9.1b's first head.
		case bindings.StepFill:
			// NOTHING TO FILL WITHOUT A FORM. The toy's one UI route takes no
			// body, and an adapter that silently dropped a declared `fill`
			// would answer `reached` for a step it never performed.
			return fault(id, wire.FaultAction, "steps",
				fmt.Sprintf("step %d of %q is a fill and the toy's UI has no form to fill", i+1, m.Action))
		default:
			return fault(id, wire.FaultAction, "steps",
				fmt.Sprintf("step %d of %q is %q, which is not a step this adapter drives", i+1, m.Action, kind))
		}
	}
	if code == 0 {
		// NOTHING WAS ASKED OF THE SYSTEM, so there is no status to report and
		// a `"status": 0` would be a non-status dressed as one. A flow of
		// navigates and waits opens a page and acts on nothing; the toy has one
		// UI route and the CLICK is what acts. Measured: a navigate-only flow
		// answered `{"status":0,"body":null}` before this branch, which reads
		// as a response the system gave.
		return wire.FlowResponse{
			Envelope: wire.Envelope{Kind: wire.KindFlow, Contract: wire.ContractVersion, ID: id},
			Reached:  len(m.Steps),
		}
	}
	return wire.FlowResponse{
		Envelope: wire.Envelope{Kind: wire.KindFlow, Contract: wire.ContractVersion, ID: id},
		Reached:  len(m.Steps),
		Response: map[string]any{"status": code, "body": out},
	}
}

func names() string {
	out := make([]string, 0, len(actions))
	for _, act := range actions {
		out = append(out, act.Name)
	}
	return strings.Join(out, ", ")
}

func (a *adapter) observe(id string, m wire.ObserveRequest) any {
	// THE SUBJECT COMES OUT OF `args`, which is the shape the execute path
	// above already has at its own m.Args["event"].
	//
	// THIS PATH SPLIT THE QUERY ON A SLASH until now -- contract 1's form --
	// while contract 2 moved the observation's arguments into `args` on
	// 2026-10-19 (docs/contract.md's v2 row, "observe gains args") and
	// examples/toy has declared `args = ["event"]` on both observations ever
	// since. So the two halves disagreed in the one direction nothing could
	// see: the project was right, the wire type carried Args, and this
	// adapter -- the tree's only query-splitter -- was the half left behind.
	// A real walk died at its first observation with "event_count names one
	// event" (quill, coop r5/163), which is this function faulting correctly
	// on a request it would not read.
	//
	// The query is the observation's NAME ALONE, which bindings.toml says in
	// its own words beside the args row it declares.
	name := m.Query
	subject, _ := m.Args["event"].(string)
	// THE SOURCE IS CHECKED AGAINST THE DECLARED ONE and a mismatch is
	// FaultSource, never an answer through another channel: docs/contract.md
	// says an adapter that cannot observe through the source it was handed
	// says so rather than substituting one.
	var want wire.Source
	switch name {
	case "event_count", "event_tally", "invitation_expiry",
		// THE ALLOWANCE IS READ THROUGH AN ACTOR'S SESSION, which is what api
		// means here, and the same channel invitation_expiry uses.
		"invite_quota_reset", "invite_quota_day", "invite_quota_reset_at":
		want = wire.SourceAPI
	case "store_event_count":
		want = wire.SourceDirect
	case "page_count", "page_attendees":
		want = wire.SourceBrowser
	// THE PROBE QUERIES, which R6.2 reserved as the two reported kinds' own
	// names. internal/quiesce asks for `queue_depth` and `worker_idle` through
	// source `direct` and nothing else: the wire does not move, because
	// `direct` is the engine's own observation and its Query is opaque to the
	// decoder (docs/contract.md). An adapter that serves them answers from
	// whatever its system exposes; this one asks the toy.
	case "queue_depth", "worker_idle":
		want = wire.SourceDirect
	default:
		return fault(id, wire.FaultInternal, "query",
			fmt.Sprintf("%q is not an observation this adapter serves", m.Query))
	}
	if m.Source != want {
		return fault(id, wire.FaultSource, "source",
			fmt.Sprintf("%q is observed through %s and was asked through %s", name, want, m.Source))
	}

	// THE BROWSER OBSERVATION LEAVES HERE, because the page is a DOCUMENT and
	// every other observation below is a JSON route. a.call decodes the body
	// into a struct, so sending it a page would fail on the first `<` and
	// report a decode error about a document that was served correctly.
	//
	// AND IT IS THE RENDERED NUMBER, not the computed one. /events/<id>/count
	// answers from the store; this reads what a reader is SHOWN, so the two
	// disagree exactly when the page's own number is stale -- which is the
	// whole reason a browser source exists rather than a second api query.
	if name == "page_count" || name == "page_attendees" {
		if subject == "" {
			return fault(id, wire.FaultField, "args",
				name+" names one event: send it as args.event")
		}
		path := "/ui/events/" + subject
		code, doc, err := a.page(path, m.Token)
		if err != nil {
			return fault(id, wire.FaultInternal, "", err.Error())
		}
		if code == http.StatusNotFound {
			// A PAGE THAT IS NOT SERVED IS AN ANSWER, not a fault, and that is
			// fable's ruling at coop rumor/1669 -- which REVERSES what this
			// branch said when it was written an hour earlier. The toy answers
			// 404 both for an event that is gone and for one not visible to
			// this reader's role; in both the browser sees no page, which is
			// what a reader of the ledger should see too. So the observation
			// reports `missing` and the walk records it as a value.
			//
			// AND IT IS STILL NOT ZERO, which was the right half of the
			// original reading: a hidden page read as 0 would look like an
			// empty event, and that is the reading a visibility finding would
			// then rest on. `missing` is neither the number nor silence.
			//
			// 404 ALONE AND NOT EVERY >= 400, which is the narrowing and its
			// reason: a 500 is the application BREAKING rather than a page
			// withheld, and reporting that as `missing` would make a broken
			// server read exactly like a deleted event. Every other status
			// keeps the fault below.
			return wire.ObserveResponse{
				Envelope: wire.Envelope{Kind: wire.KindObserve, Contract: wire.ContractVersion, ID: id},
				Missing:  true,
			}
		}
		if code >= 400 {
			return fault(id, wire.FaultInternal, "query",
				fmt.Sprintf("the toy answered %s with status %d", path, code))
		}
		if name == "page_attendees" {
			n, ok := pageAttendees(doc)
			if !ok {
				return fault(id, wire.FaultInternal, "query",
					fmt.Sprintf("%s carries no #attendees element, so who it shows is unknown",
						path))
			}
			return wire.ObserveResponse{
				Envelope: wire.Envelope{Kind: wire.KindObserve, Contract: wire.ContractVersion, ID: id},
				Value:    n,
			}
		}
		n, ok := pageCount(doc)
		if !ok {
			return fault(id, wire.FaultInternal, "query",
				fmt.Sprintf("%s carries no #count element, so what it renders is unknown", path))
		}
		return wire.ObserveResponse{
			Envelope: wire.Envelope{Kind: wire.KindObserve, Contract: wire.ContractVersion, ID: id},
			Value:    n,
		}
	}

	var path string
	switch name {
	case "event_count":
		if subject == "" {
			return fault(id, wire.FaultField, "args",
				"event_count names one event: send it as args.event")
		}
		path = "/events/" + subject + "/count"
	case "event_tally":
		// THE SUBJECT IS A RECOUNT, so the arg is `recount` and not `event`.
		rid, _ := m.Args["recount"].(string)
		if rid == "" {
			return fault(id, wire.FaultField, "args",
				"event_tally names one recount: send it as args.recount")
		}
		path = "/recounts/" + rid + "/tally"
	case "invitation_expiry":
		// THE SUBJECT IS AN INVITATION, so the arg is `invitation` -- the
		// entity invite_user creates, for the reason event_tally names a
		// recount: an expiry is only a question once an invitation exists.
		inv, _ := m.Args["invitation"].(string)
		if inv == "" {
			return fault(id, wire.FaultField, "args",
				"invitation_expiry names one invitation: send it as args.invitation")
		}
		path = "/invitations/" + inv + "/expiry"
	case "store_event_count":
		if subject == "" {
			return fault(id, wire.FaultField, "args",
				"store_event_count names one event: send it as args.event")
		}
		path = "/store/events/" + subject + "/count"
	// NO SUBJECT, AND THAT IS NOT AN OMISSION: the queue is the toy's and not
	// one event's, so a probe naming an event would be asking a question this
	// system does not have. A quiesce probe asks whether the SYSTEM settled.
	case "queue_depth", "worker_idle":
		path = "/store/queue"
	// NO SUBJECT, AND FOR A DIFFERENT REASON THAN THE QUEUE'S: the subject is
	// the OBSERVING ACTOR, which travels as the session token rather than as
	// an argument. The toy answers this path for whoever asks.
	case "invite_quota_reset", "invite_quota_day", "invite_quota_reset_at":
		path = "/invite-quota"
	}

	// THE VALUE IS THE COUNT, NOT THE BODY THAT CARRIES IT, and this is the
	// second half of the same contract drift as the args read above.
	//
	// Both count routes answer {"event":..,"count":N,"deleted":..}, and this
	// function used to pass that whole object through as the observation's
	// value. The reviewed definition of this observation's value is a SCALAR:
	// examples/toy/cases/honest-count.json carries `"value": 1` and the
	// statute compares it to `believed`, which is
	// `count(joined_by_observer) - count(left_by_observer)` -- a number. An
	// object never equals a number, so once the args fix let a real walk's
	// observation through, EVERY observation fell to `default finding :=
	// "bug"` and every seed ended at its first observable step. Measured on
	// seeds 34, 35, 6, 7 and 4242 before this hunk: 4, 5, 4, 4 and 2 steps of
	// ten, each with a finding and no disagreement to justify it.
	//
	// `deleted` is deliberately dropped rather than carried alongside: the
	// statute reads the ledger for that -- `deleted` is a fact it derives from
	// the delete_event action -- and an observation that also reported it
	// would be giving the statute a second, unreviewed source for something it
	// already decides.
	var body struct {
		Count int  `json:"count"`
		Tally int  `json:"tally"`
		Depth int  `json:"depth"`
		Idle  bool `json:"idle"`
		// ExpiresAt IS AN INSTANT IN NANOSECONDS and an int64 rather than an
		// int: the unit is docs/contract.md's for `clock.t`, and a 32-bit int
		// would overflow at a little over two seconds of run time.
		ExpiresAt int64 `json:"expires_at"`
		// THE ALLOWANCE'S TWO READINGS, both strings: a local clock face
		// ("00:00") and a local date ("2026-03-08"). Strings and not numbers
		// because a statute cannot read an instant in a zone -- there is no
		// database behind a policy -- so the application reports what it
		// believes in the form a rule can compare.
		ResetLocal string `json:"reset_local"`
		Day        string `json:"day"`
		ResetAt    int64  `json:"reset_at"`
	}
	code, err := a.call(http.MethodGet, path, m.Token, nil, &body)
	if err != nil {
		return fault(id, wire.FaultInternal, "", err.Error())
	}
	if code >= 400 {
		return fault(id, wire.FaultInternal, "query",
			fmt.Sprintf("the toy answered %s with status %d", path, code))
	}
	var value any = body.Count
	// EACH PROBE'S VALUE IS THE ONE FIELD ITS KIND IS ABOUT, and a scalar for
	// the reason the count above is one: internal/quiesce compares the answer
	// with the project's declared `quiet`, which is a TOML scalar. Handing back
	// the whole body would never equal it, and every probe would read busy for
	// ever -- the silent-and-total failure that package's own arms exist for.
	switch name {
	case "event_tally":
		value = body.Tally
	case "queue_depth":
		value = body.Depth
	case "worker_idle":
		value = body.Idle
	case "invitation_expiry":
		value = body.ExpiresAt
	case "invite_quota_reset":
		value = body.ResetLocal
	case "invite_quota_day":
		value = body.Day
	case "invite_quota_reset_at":
		// AN int64 FOR invitation_expiry's REASON: the unit is
		// docs/contract.md's for `clock.t` and a 32-bit int overflows a little
		// over two seconds into a run, let alone at a 2026 instant.
		value = body.ResetAt
	}
	return wire.ObserveResponse{
		Envelope: wire.Envelope{Kind: wire.KindObserve, Contract: wire.ContractVersion, ID: id},
		Value:    value,
	}
}

// call makes one HTTP request against the toy and decodes the body into out
// when out is not nil. It returns the status so a caller can tell the system
// refusing an action from the transport failing.
// page GETs a document and hands back its TEXT. It is a.call's sibling for the
// one route that answers HTML, and it exists rather than a flag on a.call
// because every other caller wants the body decoded and would have to opt out.
func (a *adapter) page(path, token string) (int, string, error) {
	req, err := http.NewRequest(http.MethodGet, a.base+path, nil)
	if err != nil {
		return 0, "", fmt.Errorf("building GET %s: %w", path, err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	start := time.Now()
	resp, err := a.client.Do(req)
	lat := time.Since(start)
	// THE CALL IS COUNTED AND TIMED LIKE ANY OTHER, which is not bookkeeping:
	// the adapter reports its slowest call, and a page fetch left out of that
	// would make the report quietly wrong about where a walk spent its time.
	a.calls++
	if lat > a.slowest {
		a.slowest, a.slowestWhat = lat, http.MethodGet+" "+path
	}
	if err != nil {
		return 0, "", fmt.Errorf("GET %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, "", fmt.Errorf("reading %s: %w", path, err)
	}
	return resp.StatusCode, string(raw), nil
}

// pageAttendeesRe reads the attendee list the page renders, out of the element
// the page gives an id for.
//
// IT MATCHES THE ELEMENT AND COUNTS INSIDE IT, in two steps rather than one
// regexp over `<li>`: a bare `<li>` count would read list items from anywhere on
// the page, and a second list added later would be counted into this
// observation's value without anything saying so.
//
// NON-GREEDY AND (?s), because the element spans lines: the list is written one
// `<li>` per line by cmd/toy, and a greedy match would run to the LAST `</ul>`
// on the page if another list is ever added above it.
var pageAttendeesRe = regexp.MustCompile(`(?s)<ul id="attendees">(.*?)</ul>`)

// pageAttendees counts the attendees the page SHOWS. The second return is
// whether the page carried the element at all, for pageCount's reason: a page
// with no list is a page this observation cannot read, and reading that as 0
// would make a removed element look like an event nobody has joined -- which is
// exactly the honest reading the plant's finding has to be told apart from.
func pageAttendees(doc string) (int, bool) {
	m := pageAttendeesRe.FindStringSubmatch(doc)
	if m == nil {
		return 0, false
	}
	return strings.Count(m[1], "<li>"), true
}

// pageCountRe reads the number the event page RENDERS, out of the element the
// page gives an id for.
//
// A REGEXP AND NOT A PARSER, and the reason is the id: cmd/toy writes
// `<p><span id="count">N</span> of C</p>` itself, so the anchor is the
// application's own contract with its reader rather than a guess about HTML. The
// CAPACITY is matched and discarded deliberately -- requiring it is what keeps
// this from matching a `#count` element of some other shape and reading the
// wrong number out of it.
//
// THE ID MOVED ONTO THE NUMBER IN R9.1b, and the reason is the OTHER oracle: a
// `browser` observation reads #count's TEXT from the live DOM and the engine
// converts it by declaration, so an element holding "0 of 2" would be a refusal
// rather than a reading (fable, coop r9/122). The element holds the number
// ALONE now and the sentence a reader sees is unchanged -- the page still says
// "0 of 2", with the id on the figure instead of the phrase.
var pageCountRe = regexp.MustCompile(`<span id="count">([0-9]+)</span> of ([0-9]+)`)

// pageCount extracts the rendered count. The second return is whether the page
// carried one at all, which is NOT the same question as zero: a page with no
// such element is a page this observation cannot read, and reading that as 0
// would make a broken template look like an empty event.
func pageCount(doc string) (int, bool) {
	m := pageCountRe.FindStringSubmatch(doc)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return n, true
}

func (a *adapter) call(method, path, token string, body map[string]any, out any) (int, error) {
	var payload io.Reader
	if body != nil {
		raw, err := json.Marshal(prune(body))
		if err != nil {
			return 0, fmt.Errorf("encoding the body for %s %s: %w", method, path, err)
		}
		payload = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, a.base+path, payload)
	if err != nil {
		return 0, fmt.Errorf("building %s %s: %w", method, path, err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	start := time.Now()
	resp, err := a.client.Do(req)
	lat := time.Since(start)
	a.calls++
	if lat > a.slowest {
		a.slowest, a.slowestWhat = lat, method+" "+path
	}
	if err != nil {
		// AN EXPIRY PRESENTS AS THE ADAPTER'S OWN FAULT, NAMING THE BUDGET AND
		// THE ELAPSED (fable, coop r5/206 item 3). Before this the error was
		// http's own "context deadline exceeded", which says nothing about
		// whose budget was exceeded or by how much -- so a reader could not
		// tell a wedged toy from a budget set below the work. `run` reports it
		// as could-not-run, which is CORRECT and unchanged: the system did not
		// answer, so what an observation would have said is unknown.
		if os.IsTimeout(err) {
			return 0, fmt.Errorf("%s %s: the toy did not answer within this adapter's %s "+
				"request budget (elapsed %s); the budget is the ADAPTER's and not the toy's, "+
				"so this is the instrument giving up rather than a verdict on the system: %w",
				method, path, a.budget, lat.Round(time.Millisecond), err)
		}
		return 0, fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return resp.StatusCode, fmt.Errorf("reading the answer to %s %s: %w", method, path, err)
	}
	if out != nil && len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			// The UI route answers HTML, which is not a decode failure of the
			// protocol; hand the body back as a string so the engine sees what
			// the page said.
			if s, ok := out.(*any); ok {
				*s = string(raw)
				return resp.StatusCode, nil
			}
			return resp.StatusCode, fmt.Errorf("decoding the answer to %s %s: %w", method, path, err)
		}
	}
	return resp.StatusCode, nil
}

// prune drops keys whose value is nil, so an action that omits an optional
// argument does not send a null the toy's strict decoder would have to accept.
func prune(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		if v == nil {
			continue
		}
		out[k] = v
	}
	return out
}
