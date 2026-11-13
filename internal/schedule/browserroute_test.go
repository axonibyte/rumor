package schedule

import (
	"context"
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
