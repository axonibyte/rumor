package browser

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
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

// TestTheThreeBoundsAreTOLDApart holds the classification itself, including the
// two an arm cannot drive cheaply: the driver's own 30s bound and a cancelled
// run. Driving the first would cost 30s per cell and the second is not a fault
// at all, so both are held here against the exact error shapes they produce.
func TestTheThreeBoundsAreTOLDApart(t *testing.T) {
	d := &Driver{}

	t.Run("the flow's budget", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
		defer cancel()
		<-ctx.Done()
		err := d.whyCallEnded(ctx, "POST", "/session/x/element", context.DeadlineExceeded)
		if !strings.Contains(err.Error(), "THE FLOW'S BUDGET") {
			t.Errorf("not named as the flow's: %v", err)
		}
	})

	t.Run("a cancelled run is no budget", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		<-ctx.Done()
		err := d.whyCallEnded(ctx, "POST", "/session/x/element", context.Canceled)
		if !strings.Contains(err.Error(), "CANCELLED") {
			t.Errorf("a cancelled run was not named as one: %v", err)
		}
		if strings.Contains(err.Error(), "BUDGET") {
			t.Errorf("a cancelled run was reported as a budget expiring: %v", err)
		}
	})

	t.Run("the driver's own request bound", func(t *testing.T) {
		// A REAL TIMEOUT VALUE AND NOT A STRING: os.IsTimeout keys on the
		// error's Timeout() method, so a fabricated text would pass a check the
		// transport's own error would fail.
		timeout := &net.OpError{Op: "dial", Err: &timeoutErr{}}
		err := d.whyCallEnded(context.Background(), "POST", "/session/x/element", timeout)
		if !strings.Contains(err.Error(), "THE DRIVER'S OWN REQUEST BOUND") {
			t.Errorf("a transport timeout was not named as the driver's: %v", err)
		}
		if !strings.Contains(err.Error(), RequestBound.String()) {
			t.Errorf("the bound's own value is absent, so a reader cannot tell what to raise: %v", err)
		}
	})

	t.Run("anything else is passed through", func(t *testing.T) {
		err := d.whyCallEnded(context.Background(), "POST", "/session/x/element",
			errors.New("connection reset by peer"))
		if strings.Contains(err.Error(), "BUDGET") || strings.Contains(err.Error(), "BOUND") {
			t.Errorf("an ordinary failure was dressed as a budget: %v", err)
		}
		if !strings.Contains(err.Error(), "connection reset by peer") {
			t.Errorf("the underlying failure was lost: %v", err)
		}
	})
}

// timeoutErr is an error that IS a timeout by the interface os.IsTimeout reads,
// so the cell above drives the real predicate rather than a message.
type timeoutErr struct{}

func (*timeoutErr) Error() string { return "i/o timeout" }
func (*timeoutErr) Timeout() bool { return true }
