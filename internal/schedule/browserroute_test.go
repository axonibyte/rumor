package schedule

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

// THE ADAPTER SESSION IN EVERY ARM HERE IS NIL, AND THAT IS THE OTHER HALF OF
// THE ASSERTION. These arms ask which driver a declared browser flow reaches,
// and "it did not reach the adapter" is the harder half to see: a stub adapter
// that answered plausibly would let a route to the wrong driver PASS. A nil
// *wire.Session cannot answer at all, so a flow that went there fails the arm
// loudly instead.
//
// AND THE ADAPTER'S OWN HALF IS NOT TESTED HERE, it is tested by every walk arm
// in tests/wire, tests/clock, tests/quiesce and tests/shrink -- all of which now
// pass the zero Browser explicitly. Those suites drive real flows through a real
// adapter process and would fail if the zero value had started driving a browser
// instead, which is the pair to TestTheZeroBrowserDrivesNothing below.
//
// THE STUB IS NOT A PRETEND BROWSER. It answers in wire's shape and records what
// it was asked, which is all a ROUTING question needs; nothing here asserts
// anything about a page. Whether a real browser performs the steps is measured
// against real chromium in internal/browser/flow_test.go, and a fake is kept out
// of those paths for the reason that file states.
type routedFlow struct {
	calls  []flowCall
	resp   wire.FlowResponse
	detail string
	err    error

	// reads records browser OBSERVATIONS, which is the other half of one
	// actor's context (fable, coop r9/120). It is recorded rather than counted
	// so an arm can place WHICH read happened and not merely how many.
	//
	// NO ARM ASSERTS YET THAT A `direct` OBSERVATION REACHES THE DRIVER NEVER,
	// and this comment said one did. The routing for it cannot be driven from
	// this package: a non-browser source takes the adapter branch, which needs
	// a live *wire.Session, and the nil session these arms rely on would panic
	// rather than answer. It belongs in tests/run beside the end-to-end read,
	// which waits on the value's type (ibis r9/121) -- carried, not claimed.
	reads        []readCall
	readText     string
	readChildren int
	readPresent  bool
	readErr      error

	snaps []snapCall

	// watches records every watch the walk OPENED, in order, so an arm can
	// place one per browser actor rather than count them. watchErr makes the
	// driver refuse to watch, which is the other way this is driven: a
	// refusal the walk must report rather than read as a quiet page.
	watches  []watchCall
	opened   []*stubWatch
	watchErr error
	// rounds is what each opened watch answers, one slice per Since in order,
	// so a cell can plant a quiet boundary and a noisy one in the same run.
	rounds [][]PageFault
}

type readCall struct {
	actor   string
	base    string
	page    string
	element string
	token   string
}

func (r *routedFlow) Read(_ context.Context, actor, base, page, element, token string) (
	string, int, bool, error) {
	r.reads = append(r.reads, readCall{
		actor: actor, base: base, page: page, element: element, token: token,
	})
	return r.readText, r.readChildren, r.readPresent, r.readErr
}

// snapCall records a Snapshot the walk asked for, and the stub writes the DOM
// file the real driver would, so an arm can check WHERE a name puts it.
type snapCall struct {
	actor, token, dir, name string
	screenshot              bool
	readsBefore             int // how many browser reads had happened when it was asked
}

func (r *routedFlow) Snapshot(_ context.Context, actor, token, dir, name string, screenshot bool) (string, error) {
	r.snaps = append(r.snaps, snapCall{actor: actor, token: token, dir: dir, name: name,
		screenshot: screenshot, readsBefore: len(r.reads)})
	if err := os.WriteFile(filepath.Join(dir, name+".html"), []byte("<html></html>"), 0o600); err != nil {
		return "", err
	}
	return name + ".html; screenshot not asked for", nil
}

// watchCall records a watch the walk asked for. The TOKEN is recorded because
// it is what the driver redacts a fault's text of, so an arm can see that the
// walk handed the actor's own credential and not another actor's.
type watchCall struct{ actor, token string }

// stubWatch is one opened watch. It answers `rounds` in order -- one slice per
// Since -- and counts its closes, so an arm can assert that the walk closed
// what it opened rather than leaking a connection per step.
type stubWatch struct {
	rounds [][]PageFault
	n      int
	closed int
}

func (w *stubWatch) Since(context.Context) ([]PageFault, error) {
	if w.n >= len(w.rounds) {
		w.n++
		return nil, nil
	}
	out := w.rounds[w.n]
	w.n++
	return out, nil
}

func (w *stubWatch) Close(context.Context) error { w.closed++; return nil }

func (r *routedFlow) WatchFor(_ context.Context, actor, token string) (ActorWatch, error) {
	r.watches = append(r.watches, watchCall{actor: actor, token: token})
	if r.watchErr != nil {
		return nil, r.watchErr
	}
	w := &stubWatch{rounds: r.rounds}
	r.opened = append(r.opened, w)
	return w, nil
}

type flowCall struct {
	actor string
	base  string
	token string
	req   wire.FlowRequest
}

func (r *routedFlow) Flow(_ context.Context, actor, base, token string, req wire.FlowRequest) (
	wire.FlowResponse, string, error) {
	r.calls = append(r.calls, flowCall{actor: actor, base: base, token: token, req: req})
	return r.resp, r.detail, r.err
}

// browserAction is a declared browser-context action with a two-step flow, which
// is the shape `drive` routes on: ContextBrowser AND a non-empty Flow.
func browserAction() bindings.Action {
	return bindings.Action{
		Name:    "join_event",
		Context: bindings.ContextBrowser,
		Flow: []bindings.Step{
			{Step: bindings.StepNavigate, To: "/ui/events/1"},
			{Step: bindings.StepWait, Until: "count-changed"},
		},
	}
}

func composedSteps() []map[string]any {
	return []map[string]any{
		{"step": "navigate", "to": "/ui/events/1"},
		{"step": "wait", "until": "count-changed"},
	}
}

// TestADeclaredBrowserFlowIsDrivenByTheBrowserAndNotTheAdapter is deliverable
// 2's routing arm: fable's ruling (coop r9/110) is that the WALK drives, so the
// walk's own driver choice is a claim that needs measuring and not a line in a
// commit message.
//
// IT ASSERTS WHAT CROSSED THE SEAM, not just that something did: the actor, the
// base, the token, the resume position and the composed steps all reach the
// driver, and a mutant that dropped any one of them would otherwise pass. The
// actor in particular is what deliverable 1's "one context per actor" keys on.
func TestADeclaredBrowserFlowIsDrivenByTheBrowserAndNotTheAdapter(t *testing.T) {
	stub := &routedFlow{
		resp: wire.FlowResponse{
			Reached: 2, Stopped: wire.StopNone, Response: map[string]any{"count": 1},
		},
		detail: "the count read 1",
	}
	ss := NewSessions(nil)
	ss.UseBrowser(stub, "http://127.0.0.1:8431")

	got, err := drive(context.Background(), nil, ss, "member_1",
		browserAction(), "tok-7", nil, 0, composedSteps())
	if err != nil {
		t.Fatalf("driving a declared flow through the browser: %v", err)
	}

	if len(stub.calls) != 1 {
		t.Fatalf("the browser was asked %d time(s), want exactly 1", len(stub.calls))
	}
	c := stub.calls[0]
	if c.actor != "member_1" {
		t.Errorf("the driver was handed actor %q, want member_1: the actor is what a browser "+
			"session is keyed by, so a flow driven under the wrong one acts as someone else",
			c.actor)
	}
	if c.base != "http://127.0.0.1:8431" {
		t.Errorf("the driver was handed base %q, want the address the stage gave: a `navigate` "+
			"resolves against it", c.base)
	}
	if c.token != "tok-7" {
		t.Errorf("the driver was handed token %q, want tok-7", c.token)
	}
	if c.req.Action != "join_event" || c.req.From != 0 {
		t.Errorf("the driver was asked for %q from %d, want join_event from 0",
			c.req.Action, c.req.From)
	}
	if len(c.req.Steps) != 2 {
		t.Fatalf("%d step(s) reached the driver, want the 2 that were composed", len(c.req.Steps))
	}
	if c.req.Steps[0]["to"] != "/ui/events/1" || c.req.Steps[1]["until"] != "count-changed" {
		t.Errorf("the composed steps reached the driver altered: %v", c.req.Steps)
	}

	if !got.Flowed {
		t.Error("a driven flow was not recorded as flowed")
	}
	if !got.Browser {
		t.Error("a flow a BROWSER drove was not recorded as browser-driven: `Flowed` and " +
			"`Browser` are two facts, and a reader who cannot tell them apart cannot tell a " +
			"page from a simulation of one")
	}
	if got.Detail != "the count read 1" {
		t.Errorf("the detail is %q, want the driver's: the wire has no field for it, so losing "+
			"it here loses what a failing wait saw", got.Detail)
	}
	if got.Reached != 2 || got.Stopped != string(wire.StopNone) {
		t.Errorf("recorded reached=%d stopped=%q, want 2 and %q",
			got.Reached, got.Stopped, wire.StopNone)
	}
	answer, ok := got.Answer.(map[string]any)
	if !ok || answer["count"] != 1 {
		t.Errorf("the system's response did not survive the route: %v", got.Answer)
	}
}

// TestTheZeroBrowserDrivesNothing is the arm the OTHER suites rest on. `Walk`
// takes a Browser by value and forty-odd call sites pass the zero one, so if a
// zero Browser produced a non-nil driver every one of those walks would route a
// declared flow at a browser nobody started.
//
// IT CHECKS THE SEAM AND NOT A DRIVE, deliberately: the zero value's whole
// behavior is that `Browser()` answers nil, and the adapter path it then takes
// is what those suites already measure end to end.
func TestTheZeroBrowserDrivesNothing(t *testing.T) {
	var zero Browser
	ss := NewSessions(nil)
	// AS `Walk` CALLS IT: the driver off the Browser it was handed, and the
	// base off the adapter's describe -- which for an adapter reporting none is
	// the empty string.
	ss.UseBrowser(zero.Driver, "")

	br, base := ss.Browser()
	if br != nil {
		t.Errorf("the zero Browser yielded driver %v, want nil: a walk handed the zero value "+
			"must drive the adapter, which is what every run did before this head", br)
	}
	if base != "" {
		t.Errorf("the zero Browser yielded base %q, want empty", base)
	}
}

// TestANavigateWithNoBaseIsRefusedByName is fable's r9/111 ruling held as a
// measurement: "a flow that navigates against an adapter whose describe carries
// no base is a NAMED refusal at the walk, never a guessed host or port".
//
// THE DRIVER IS LIVE AND WILLING, which is the point: the stub would answer
// this flow happily, so a pass here would mean a REAL browser had been pointed
// at whatever is listening on the box. The refusal has to come before the drive,
// and the arm proves it did by the driver seeing nothing.
func TestANavigateWithNoBaseIsRefusedByName(t *testing.T) {
	stub := &routedFlow{resp: wire.FlowResponse{Reached: 2, Stopped: wire.StopNone}}
	ss := NewSessions(nil)
	ss.UseBrowser(stub, "")

	_, err := drive(context.Background(), nil, ss, "member_1",
		browserAction(), "tok-7", nil, 0, composedSteps())
	if err == nil {
		t.Fatal("a `navigate` with no address drove a real browser instead of refusing")
	}
	if len(stub.calls) != 0 {
		t.Errorf("the driver was asked %d time(s) with no address to navigate against: the "+
			"refusal must come BEFORE the drive", len(stub.calls))
	}
	for _, want := range []string{"join_event", "`navigate`", "`base`", "not guessed"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not say %s: %v", want, err)
		}
	}
}

// AND A FLOW THAT NAVIGATES NOWHERE IS NOT REFUSED. fill, click and wait act on
// the page the driver already has, so refusing them for a missing base would
// refuse a flow with no use for one -- a narrowing whose reason has to cover
// exactly what it narrows.
func TestAFlowThatDoesNotNavigateNeedsNoBase(t *testing.T) {
	stub := &routedFlow{resp: wire.FlowResponse{Reached: 2, Stopped: wire.StopNone}}
	ss := NewSessions(nil)
	ss.UseBrowser(stub, "")

	a := bindings.Action{
		Name:    "type_title",
		Context: bindings.ContextBrowser,
		Flow: []bindings.Step{
			{Step: bindings.StepFill, Field: "title", Value: "x"},
			{Step: bindings.StepClick, Target: "save"},
		},
	}
	steps := []map[string]any{
		{"step": "fill", "field": "title", "value": "x"},
		{"step": "click", "target": "save"},
	}

	got, err := drive(context.Background(), nil, ss, "member_1", a, "tok-7", nil, 0, steps)
	if err != nil {
		t.Fatalf("a flow with no `navigate` was refused for a missing base: %v", err)
	}
	if len(stub.calls) != 1 {
		t.Fatalf("the driver was asked %d time(s), want 1", len(stub.calls))
	}
	if !got.Browser {
		t.Error("the flow was not recorded as browser-driven")
	}
}

// TestTheResumeGoesToTheDriverThatDroveTheFlow is fable's r9/110 wording held as
// a measurement: "one driver for the flow and its resume, or the re-check means
// nothing". A flow a browser started and the adapter resumed would re-check a
// condition against a page the adapter cannot see.
//
// IT DRIVES AND THEN RESUMES ON ONE STUB so "the same driver" is observable:
// two calls on one instance, the second at the position the first stopped at.
// Two stubs could not tell "the same" from "another just like it".
func TestTheResumeGoesToTheDriverThatDroveTheFlow(t *testing.T) {
	a := browserAction()
	stub := &routedFlow{
		resp: wire.FlowResponse{Reached: 1, Stopped: wire.StopWait},
	}
	ss := NewSessions(nil)
	ss.UseBrowser(stub, "http://127.0.0.1:8431")
	// THE TOKEN IS SEEDED RATHER THAN ACQUIRED. There is no adapter session
	// here to ask one of, and acquiring is not what this arm is about -- the
	// resume's own token lookup is sessions_test.go's subject.
	ss.held[sessionKey{actor: "member_1", context: bindings.ContextBrowser}] = "tok-7"

	if _, err := drive(context.Background(), nil, ss, "member_1",
		a, "tok-7", nil, 0, composedSteps()); err != nil {
		t.Fatalf("the first drive: %v", err)
	}

	pend := &pendingFlow{Action: a, Actor: "member_1", Steps: composedSteps(), Reached: 1}
	var step Step
	if _, _, err := resumeFlow(context.Background(), nil, ss, nil, pend, &step); err != nil {
		t.Fatalf("resuming through the browser: %v", err)
	}

	if len(stub.calls) != 2 {
		t.Fatalf("the driver saw %d call(s), want the drive and its resume on the SAME one",
			len(stub.calls))
	}
	if stub.calls[0].req.From != 0 || stub.calls[1].req.From != 1 {
		t.Errorf("the calls resumed from %d then %d, want 0 then 1: the resume sends `from` so "+
			"the steps before it are not re-sent",
			stub.calls[0].req.From, stub.calls[1].req.From)
	}
	if stub.calls[1].actor != "member_1" || stub.calls[1].token != "tok-7" {
		t.Errorf("the resume went out as actor %q with token %q, want member_1 and tok-7",
			stub.calls[1].actor, stub.calls[1].token)
	}
}

// TestABrowserDrivingFailureSaysItWasTheBrowser keeps the two drivers'
// failures apart. The box this phase runs on has a drivable pair and the CI
// jail does not, so "driving join_event's flow from 0" without the driver named
// sends its reader to the adapter's logs for a browser's fault.
func TestABrowserDrivingFailureSaysItWasTheBrowser(t *testing.T) {
	stub := &routedFlow{err: context.DeadlineExceeded}
	ss := NewSessions(nil)
	ss.UseBrowser(stub, "http://127.0.0.1:8431")

	_, err := drive(context.Background(), nil, ss, "member_1",
		browserAction(), "tok-7", nil, 0, composedSteps())
	if err == nil {
		t.Fatal("a driver that refused the flow was reported as a drive")
	}
	if !strings.Contains(err.Error(), "through the browser") {
		t.Errorf("the failure reads %q, and does not say which driver failed", err)
	}
	if !strings.Contains(err.Error(), "join_event") {
		t.Errorf("the failure reads %q, and does not name the action", err)
	}
}


// TestTheWalkOpensOneWatchPerActorAndDiscardsTheOpeningBoundary is issue 125's
// deliverable 1 driven the way it can fail.
//
// THE PLANTED PAIR IS THE WHOLE ARM. `rounds[0]` is answered to the boundary
// taken BEFORE the step and `rounds[1]` to the one taken after, so a walk that
// kept the opening answer -- the obvious way to write this wrong -- would
// report a fault that belongs to whatever ran previously, and this arm sees it
// because the two faults are distinguishable. A walk that took only one
// boundary would report nothing.
func TestTheWalkOpensOneWatchPerActorAndDiscardsTheOpeningBoundary(t *testing.T) {
	before := PageFault{Kind: "console", Text: "this belongs to an earlier step"}
	under := PageFault{Kind: "response", Text: "/api/events/1", Status: 503}
	stub := &routedFlow{
		resp:   wire.FlowResponse{Reached: 2, Stopped: wire.StopNone},
		rounds: [][]PageFault{{before}, {under}},
	}
	ss := NewSessions(nil)
	ss.UseBrowser(stub, "http://127.0.0.1:8431")

	for i := 0; i < 2; i++ {
		if _, err := drive(context.Background(), nil, ss, "member_1",
			browserAction(), "tok-7", nil, 0, composedSteps()); err != nil {
			t.Fatalf("drive %d: %v", i+1, err)
		}
	}

	if len(stub.watches) != 1 {
		t.Fatalf("the driver was asked for %d watch(es) over two steps, want exactly 1: a watch "+
			"is per ACTOR and reopening it per step would lose the barrier's meaning and leak "+
			"a connection a step", len(stub.watches))
	}
	if stub.watches[0].actor != "member_1" || stub.watches[0].token != "tok-7" {
		t.Errorf("the watch was opened for actor %q with token %q, want member_1 and tok-7: the "+
			"token is what the driver redacts a fault's text of, so the wrong one publishes "+
			"another actor's credential", stub.watches[0].actor, stub.watches[0].token)
	}
	if len(stub.opened) != 1 {
		t.Fatalf("%d watch(es) were actually opened, want 1", len(stub.opened))
	}
	if n := stub.opened[0].n; n != 4 {
		t.Errorf("the watch was barriered %d time(s) over two steps, want 4: one before each "+
			"step and one after, and a walk taking fewer cannot bound a step at both ends", n)
	}

	heard := ss.Heard()
	if len(heard) != 1 {
		t.Fatalf("the walk recorded %d fault(s), want exactly the 1 heard UNDER a step: %v",
			len(heard), heard)
	}
	if heard[0].Fault != under {
		t.Errorf("the walk recorded %v, want %v: the opening boundary's answer is discarded, so "+
			"a fault from before the step must not be attributed to it", heard[0].Fault, under)
	}
	if heard[0].Actor != "member_1" || heard[0].Action != "join_event" {
		t.Errorf("the fault is attributed to %q/%q, want member_1/join_event: a fault without "+
			"the step it was heard under is a property of the page and not a finding",
			heard[0].Actor, heard[0].Action)
	}
	if len(ss.Unheard()) != 0 {
		t.Errorf("a step whose watch answered was recorded as unheard: %v", ss.Unheard())
	}
}

// TestARefusedWatchIsRecordedPerStepAndStopsNothing is the other way deliverable
// 1 is driven: the driver cannot watch.
//
// TWO FACTS AT ONCE, and they pull in opposite directions. The driver is asked
// ONCE -- a driver with no BiDi will not acquire it between two steps, and
// retrying would bury the run's output in one fact -- but EACH step records
// that its own page went unheard, because each of them separately was not
// heard. A refusal remembered once and reported once would lose the second
// step; a refusal retried per step would ask a failing driver forever.
func TestARefusedWatchIsRecordedPerStepAndStopsNothing(t *testing.T) {
	stub := &routedFlow{
		resp:     wire.FlowResponse{Reached: 2, Stopped: wire.StopNone},
		watchErr: errors.New("the driver opened this session with no webSocketUrl"),
	}
	ss := NewSessions(nil)
	ss.UseBrowser(stub, "http://127.0.0.1:8431")

	for i := 0; i < 2; i++ {
		got, err := drive(context.Background(), nil, ss, "member_1",
			browserAction(), "tok-7", nil, 0, composedSteps())
		if err != nil {
			t.Fatalf("a watch the driver refused stopped the walk at step %d: %v. A page that "+
				"cannot be HEARD is still a page that can be driven", i+1, err)
		}
		if !got.Flowed || !got.Browser {
			t.Errorf("step %d was not recorded as browser-driven after a refused watch", i+1)
		}
	}

	if len(stub.watches) != 1 {
		t.Errorf("the driver was asked to watch %d time(s), want 1: the refusal is remembered, "+
			"and asking a driver that cannot watch once per step buries the run's output in "+
			"one fact", len(stub.watches))
	}
	un := ss.Unheard()
	if len(un) != 2 {
		t.Fatalf("%d step(s) recorded as unheard, want 2: the open is not retried, but each "+
			"step's page separately went unheard and a single entry would lose the second",
			len(un))
	}
	for i, u := range un {
		if u.Actor != "member_1" || u.Action != "join_event" {
			t.Errorf("unheard %d is attributed to %q/%q, want member_1/join_event",
				i, u.Actor, u.Action)
		}
		if !strings.Contains(u.Why, "webSocketUrl") {
			t.Errorf("unheard %d says %q, want the driver's own refusal: a reason the driver "+
				"gave is the only thing that tells a box with no BiDi from a broken one",
				i, u.Why)
		}
	}
	if len(ss.Heard()) != 0 {
		t.Errorf("a run that heard nothing recorded faults: %v", ss.Heard())
	}
}

// TestTheWatchesCloseAndTheQuietPageRecordsNothing holds the vacuity guard for
// the pair above and the close for the connection.
//
// THE QUIET HALF IS NOT DECORATION: every assertion above is about a PLANTED
// fault, and a walk that recorded every boundary as a fault would satisfy them
// all. A clean page has to come back empty, or "two faults were heard" is not
// evidence that anything was heard.
func TestTheWatchesCloseAndTheQuietPageRecordsNothing(t *testing.T) {
	stub := &routedFlow{resp: wire.FlowResponse{Reached: 2, Stopped: wire.StopNone}}
	ss := NewSessions(nil)
	ss.UseBrowser(stub, "http://127.0.0.1:8431")

	if _, err := drive(context.Background(), nil, ss, "member_1",
		browserAction(), "tok-7", nil, 0, composedSteps()); err != nil {
		t.Fatalf("driving a clean page: %v", err)
	}
	if len(ss.Heard()) != 0 || len(ss.Unheard()) != 0 {
		t.Fatalf("a watched, quiet page recorded heard=%v unheard=%v, want neither: a boundary "+
			"that answers nothing is the ordinary case", ss.Heard(), ss.Unheard())
	}
	if len(stub.opened) != 1 {
		t.Fatalf("%d watch(es) opened, want 1", len(stub.opened))
	}
	if c := stub.opened[0].closed; c != 0 {
		t.Fatalf("the watch was closed %d time(s) during the walk, want 0: it spans the "+
			"attempt, and closing it per step would reopen a connection a step", c)
	}
	if err := ss.CloseWatches(context.Background()); err != nil {
		t.Fatalf("closing the walk's watches: %v", err)
	}
	if c := stub.opened[0].closed; c != 1 {
		t.Errorf("the watch was closed %d time(s) at the attempt's end, want exactly 1: a "+
			"connection the walk opened and did not give back outlives the walk", c)
	}
	if len(ss.watches) != 0 {
		t.Errorf("a closed watch is still held, so a second close would close it twice")
	}
}
