package browser

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

// TestABudgetExpiryNamesWHICHBudget is R9.2's first deliverable: the instrument
// for the contended red of coop r9/154.
//
// WHAT THAT RED LEFT UNREADABLE. `TestTheActorsTokenReachesNoOutput` went red at
// load 6.8 to 9.2 with a POST to .../element that "hit its deadline", and
// NOTHING in the error said which of three bounds had ended it: the FLOW's
// budget, the driver's own request bound, or a cancelled run. Those want
// different answers -- raise the budget, look at the driver, or nothing -- so a
// reader who cannot tell them apart cannot act on any of them.
//
// DETERMINISTIC, NOT A RACE. The fake holds the page call until the caller hangs
// up, so the expiry fires exactly when the budget does rather than after a sleep
// chosen to be longer than it. rook's 0-of-8 and my 1-of-24 on the last race are
// why this is built this way: a fault that reproduces every time is the
// acceptance, and a rate neither box can bound is not.
func TestABudgetExpiryNamesWHICHBudget(t *testing.T) {
	d, _ := startFake(t, "wedge-element", true)
	defer func() { _ = d.Close(context.Background()) }()

	// A LISTENER, because a flow navigates before it clicks and the address is
	// dialed first; nothing is read from this page.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<!doctype html><title>x</title><p id="count">0</p>`)
	}))
	defer srv.Close()

	// THE SESSION FIRST, outside the budget, as the guard in browser_test.go
	// requires: a budget that covered the open would measure the box.
	if _, serr := d.SessionFor(context.Background(), "alice"); serr != nil {
		t.Fatalf("opening a session for alice: %v", serr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	began := time.Now()
	_, _, ferr := d.Flow(ctx, "alice", srv.URL, "tok", wire.FlowRequest{
		Action: "join_event_ui", Steps: flowSteps(
			map[string]any{"step": "navigate", "to": "/ui/events/e1"},
			map[string]any{"step": "click", "target": "join"},
		)})
	took := time.Since(began)

	if ferr == nil {
		t.Fatal("a call held past the flow's budget was reported as a drive")
	}
	for _, says := range []string{"THE FLOW'S BUDGET", "/element", "the deadline this flow was driven with"} {
		if !strings.Contains(ferr.Error(), says) {
			t.Errorf("the failure does not say %q, so its reader cannot tell which bound "+
				"ended it: %v", says, ferr)
		}
	}
	// AND IT SAYS THE OTHER TWO ARE UNTOUCHED, which is the half that stops a
	// reader raising the wrong bound: the driver answered every other call.
	if !strings.Contains(ferr.Error(), "untouched") {
		t.Errorf("the failure does not clear the driver's bound and the readiness bound, so a "+
			"reader may raise either: %v", ferr)
	}
	t.Logf("PROOF the flow's budget of 300ms ended a wedged /element in %s, named as the flow's",
		took.Round(time.Millisecond))
}

// AND WITH THE DELAY REMOVED THE SAME FLOW NAMES NO BUDGET, which is the
// instrument's own control (fable, coop r9/156: "its mutant, the delay removed,
// passing"). Against a fake that answers, the click finds no element and the
// flow STOPS at that step -- a stop is the page's answer and not a bound.
func TestWithoutTheDelayNoBudgetIsNamed(t *testing.T) {
	d, _ := startFake(t, "ready", true)
	defer func() { _ = d.Close(context.Background()) }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<!doctype html><title>x</title><p id="count">0</p>`)
	}))
	defer srv.Close()

	if _, serr := d.SessionFor(context.Background(), "alice"); serr != nil {
		t.Fatalf("opening a session for alice: %v", serr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	got, _, ferr := d.Flow(ctx, "alice", srv.URL, "tok", wire.FlowRequest{
		Action: "join_event_ui", Steps: flowSteps(
			map[string]any{"step": "navigate", "to": "/ui/events/e1"},
			map[string]any{"step": "click", "target": "join"},
		)})
	if ferr != nil {
		if strings.Contains(ferr.Error(), "BUDGET") {
			t.Fatalf("a budget was named with no delay in play, so the arm above would pass "+
				"on a driver that was merely slow: %v", ferr)
		}
		t.Fatalf("driving the flow: %v", ferr)
	}
	if got.Stopped != wire.StopStep {
		t.Errorf("stopped %q, want %q: the element is absent, which is the page's answer",
			got.Stopped, wire.StopStep)
	}
}

// TestEachBOUNDxERRORCellIsDiagnosedApart drives the classifier as the GRID it
// is, and the grid is rook's finding at coop rumor/1973 made structural.
//
// WHAT THE OLD TABLE MISSED AND WHY. It held four cells, each moving ONE input:
// a deadline with the deadline error, a cancellation with the cancellation
// error, a transport timeout, and "anything else" against a LIVE context. The
// classifier takes TWO inputs, so a one-dimensional table leaves the whole
// off-diagonal undriven -- and the cell that mattered was exactly there: a
// transport failure arriving while the context had expired, which the old code
// reported as the flow's budget while explicitly clearing the driver. Four green
// cells over a wrong diagnosis.
//
// SO EVERY CONTEXT STATE MEETS EVERY ERROR SHAPE, and each cell says what the
// sentence must say AND what it must not: a classifier is only as good as the
// readings it refuses, and "must not name a budget" is the half the defect
// walked through.
func TestEachBOUNDxERRORCellIsDiagnosedApart(t *testing.T) {
	const (
		flow      = "THE FLOW'S BUDGET"
		driver    = "THE DRIVER'S OWN REQUEST BOUND"
		cancelled = "THIS RUN WAS CANCELLED"
		neither   = "NO BOUND EXPLAINS IT"
		clears    = "untouched"
	)
	live := func() context.Context { return context.Background() }
	expired := func() context.Context {
		// A DEADLINE ALREADY PAST, so the context is born expired and ctx.Err()
		// is DeadlineExceeded; cancel() afterwards releases the timer without
		// overwriting that cause, which a later Canceled would.
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		cancel()
		return ctx
	}
	stopped := func() context.Context {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return ctx
	}
	transport := errors.New("connection reset by peer")

	for _, cell := range []struct {
		ctxName, errName string
		ctx              func() context.Context
		err              error
		wants            []string
		notWants         []string
	}{
		// A LIVE CONTEXT CANNOT BE THE FLOW'S BUDGET, whatever the error says:
		// Go reports the client's own Timeout as a deadline too.
		{"live", "deadline", live, context.DeadlineExceeded,
			[]string{driver}, []string{flow, cancelled, neither}},
		{"live", "cancellation", live, context.Canceled,
			nil, []string{flow, driver, cancelled, neither}},
		{"live", "transport timeout", live, &timeoutErr{},
			[]string{driver}, []string{flow, cancelled, neither}},
		{"live", "transport failure", live, transport,
			[]string{"connection reset by peer"}, []string{flow, driver, cancelled, neither}},

		// AN EXPIRED CONTEXT IS THE FLOW'S BUDGET ONLY WHEN A BOUND ACTUALLY
		// ENDED THE CALL. The last cell here is the defect rook drove.
		{"expired", "deadline", expired, context.DeadlineExceeded,
			[]string{flow, clears}, []string{driver, cancelled, neither}},
		{"expired", "cancellation", expired, context.Canceled,
			[]string{neither, "ALSO already done"}, []string{flow, clears, cancelled}},
		{"expired", "transport timeout", expired, &timeoutErr{},
			[]string{flow, clears}, []string{driver, cancelled, neither}},
		{"expired", "transport failure", expired, transport,
			[]string{neither, "ALSO already done", "connection reset by peer"},
			[]string{flow, clears, cancelled}},

		// A CANCELLED RUN IS NOT A BUDGET, and a deadline error under a
		// cancelled context is the DRIVER's -- the deadline that fired was not
		// this caller's, since this caller was cancelled rather than timed out.
		{"cancelled", "deadline", stopped, context.DeadlineExceeded,
			[]string{driver}, []string{flow, cancelled, neither}},
		{"cancelled", "cancellation", stopped, context.Canceled,
			[]string{cancelled, "no fault in the driver"}, []string{flow, driver, neither}},
		{"cancelled", "transport timeout", stopped, &timeoutErr{},
			[]string{driver}, []string{flow, cancelled, neither}},
		{"cancelled", "transport failure", stopped, transport,
			[]string{neither, "ALSO already done", "connection reset by peer"},
			[]string{flow, clears, cancelled}},
	} {
		t.Run(cell.ctxName+" context, "+cell.errName, func(t *testing.T) {
			d := &Driver{}
			got := d.whyCallEnded(cell.ctx(), "POST", "/session/x/element", cell.err).Error()
			for _, says := range cell.wants {
				if !strings.Contains(got, says) {
					t.Errorf("does not say %q: %s", says, got)
				}
			}
			for _, never := range cell.notWants {
				if strings.Contains(got, never) {
					t.Errorf("says %q, which is a reading this cell rules out: %s", never, got)
				}
			}
			// THE UNDERLYING FAILURE SURVIVES EVERY CELL, so no diagnosis is
			// bought by discarding what the transport said.
			if !errors.Is(d.whyCallEnded(cell.ctx(), "POST", "/p", cell.err), cell.err) {
				t.Errorf("the underlying error is no longer unwrappable from the diagnosis")
			}
		})
	}
}

// TestTheTOOLCHAINCannotTellTheBoundsApartByErrorAlone pins the premise the
// conjunctive shape rests on, because a comment is an untested claim whoever
// wrote it.
//
// ROOK MEASURED IT ON go1.26 AND REFUTED THEIR OWN FIRST FIX WITH IT (coop
// r9/160); this re-measures it here, and it is the reason `whyCallEnded` reads
// ctx.Err() at all. If a future toolchain stops reporting the client's own
// Timeout as a context deadline, THIS arm goes red -- which is the signal that
// the error alone has become sufficient and the conjunction could simplify.
// Nothing else in the tree would notice.
func TestTheTOOLCHAINCannotTellTheBoundsApartByErrorAlone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	_, callers := (&http.Client{}).Do(req)

	req2, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	_, ours := (&http.Client{Timeout: 150 * time.Millisecond}).Do(req2)

	if callers == nil || ours == nil {
		t.Fatalf("a wedged server answered: caller %v, client %v", callers, ours)
	}
	// THE AMBIGUITY ITSELF IS THE ASSERTION: both read as a context deadline.
	for label, err := range map[string]error{"the caller's ctx": callers, "the client's own Timeout": ours} {
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("%s no longer wraps context.DeadlineExceeded, so the error alone may now be "+
				"enough to say whose bound ended a call: %v", label, err)
		}
		if !os.IsTimeout(err) {
			t.Errorf("%s is no longer an os.IsTimeout, which `bound` reads: %v", label, err)
		}
	}
	t.Logf("PROOF both read as a context deadline: caller %q, client %q", callers, ours)
}

// timeoutErr is an error that IS a timeout by the interface os.IsTimeout reads,
// so the cell above drives the real predicate rather than a message.
type timeoutErr struct{}

func (*timeoutErr) Error() string { return "i/o timeout" }
func (*timeoutErr) Timeout() bool { return true }
