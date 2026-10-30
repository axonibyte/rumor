package concurrency

import (
	"strings"
	"testing"
	"time"

	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

// Overlapped IS THE RUN's PROOF THAT IT WAS CONCURRENT, so it is armed over
// spans rather than over a server: the question is arithmetic about two
// intervals, and a live arm could only ever show the one case the box happened
// to produce.
func TestOverlappedSeesTwoRequestsInFlightAtOnce(t *testing.T) {
	base := time.Date(2026, 10, 30, 14, 0, 0, 0, time.UTC)
	at := func(ms, dur int) Attempt {
		return Attempt{Started: base.Add(time.Duration(ms) * time.Millisecond),
			Finished: base.Add(time.Duration(ms+dur) * time.Millisecond)}
	}
	for _, c := range []struct {
		name string
		as   []Attempt
		want bool
		best time.Duration
	}{
		{"two spans crossing", []Attempt{at(0, 30), at(10, 30)}, true, 20 * time.Millisecond},
		{"one inside the other", []Attempt{at(0, 50), at(10, 10)}, true, 10 * time.Millisecond},
		{"identical spans", []Attempt{at(0, 25), at(0, 25)}, true, 25 * time.Millisecond},
		// SERIALIZED IS THE FAILURE THIS EXISTS TO CATCH: back-to-back
		// requests answer exactly as a correct system does.
		{"back to back", []Attempt{at(0, 10), at(10, 10)}, false, 0},
		{"a clear gap", []Attempt{at(0, 10), at(40, 10)}, false, 0},
		{"three, only two crossing", []Attempt{at(0, 5), at(20, 30), at(40, 30)}, true, 10 * time.Millisecond},
	} {
		got := Overlapped(c.as)
		if got != c.want {
			t.Errorf("%s: Overlapped = %v, wanted %v", c.name, got, c.want)
		}
		// THE MEASURED HALF CARRIES THE FIGURE AND IS CHECKED SEPARATELY,
		// because the split is the point: a caller can print the verdict
		// where the control hashes and the duration only where it does not.
		measured := OverlapMeasured(c.as)
		if c.want {
			if want := "overlapped by " + c.best.String(); measured != want {
				t.Errorf("%s: OverlapMeasured = %q, wanted %q", c.name, measured, want)
			}
		} else if measured != "" {
			t.Errorf("%s: nothing overlapped but the measured half said %q, so a caller "+
				"printing it unconditionally would say something false", c.name, measured)
		}
	}
}

// A FAULTED ATTEMPT CANNOT EVIDENCE AN OVERLAP. Its span is the time the
// adapter took to fail, which says nothing about the system having two requests
// in flight -- and counting it would let a run where ONE request reached the
// system claim it was concurrent.
func TestAFaultedAttemptIsNotEvidenceOfOverlap(t *testing.T) {
	base := time.Date(2026, 10, 30, 14, 0, 0, 0, time.UTC)
	good := Attempt{Started: base, Finished: base.Add(30 * time.Millisecond)}
	faulted := Attempt{Started: base.Add(5 * time.Millisecond),
		Finished: base.Add(25 * time.Millisecond), Fault: "the adapter does not carry join_event"}
	if Overlapped([]Attempt{good, faulted}) {
		t.Error("a faulted attempt was counted as a second request in flight, so a fan-out in " +
			"which only one request reached the system could claim it was concurrent")
	}
	// And the same pair without the fault DOES overlap, so the arm above is
	// about the fault and not about the spans.
	faulted.Fault = ""
	if !Overlapped([]Attempt{good, faulted}) {
		t.Error("this arm's premise is gone: the two spans no longer overlap even without the " +
			"fault, so it no longer isolates the fault")
	}
}

// FAN's TWO REFUSALS, both reached before a session is touched -- which is why
// nil sessions are safe here and why the arm can exist at all without a server.
func TestFanRefusesWhatCannotBeAFanOut(t *testing.T) {
	two, err := PlanFor(1, 2)
	if err != nil {
		t.Fatalf("PlanFor: %v", err)
	}

	// A PLAN NOTHING DISPATCHES is the defect R8.1 closed one level up, so a
	// width disagreement is refused rather than trusted.
	_, err = Fan(two, make([]*wire.Session, 3), make([]wire.ExecuteRequest, 3))
	if err == nil {
		t.Fatal("a 2-wide plan over three sessions was accepted, so the plan would govern nothing")
	}
	if !strings.Contains(err.Error(), "plan is 2 wide") {
		t.Errorf("the refusal does not name the width disagreement: %v", err)
	}

	// A MISMATCH WOULD SEND FEWER REQUESTS THAN ASKED, and the missing one
	// would read as a system that answered fewer times than it was called.
	_, err = Fan(two, make([]*wire.Session, 2), make([]wire.ExecuteRequest, 3))
	if err == nil {
		t.Fatal("two sessions for three requests was accepted, so a fan-out can silently drop one")
	}
	for _, want := range []string{"2 session(s)", "3 request(s)"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not carry %q, so the caller cannot see which side is "+
				"short: %v", want, err)
		}
	}

	// A FAN-OUT OF ONE EXERCISES THE SEQUENTIAL PATH and would report every
	// concurrency clause as satisfied. PlanFor refuses to draw one at all, so
	// the only way to present one here is a hand-built Plan -- which is the
	// shape a future caller could reach for.
	_, err = Fan(Plan{Issue: []int{0}}, make([]*wire.Session, 1), make([]wire.ExecuteRequest, 1))
	if err == nil {
		t.Fatal("a single-request fan-out was accepted, so the sequential path could be reported " +
			"as a concurrent one")
	}
	if !strings.Contains(err.Error(), "is not a fan-out") {
		t.Errorf("the refusal does not say what is wrong with one request: %v", err)
	}

	// AND EMPTY IS THE SAME REFUSAL, not a quiet success over nothing.
	if _, err := Fan(Plan{}, nil, nil); err == nil {
		t.Error("an empty fan-out was accepted, so a stage that built no requests would report " +
			"its clauses satisfied")
	}
}
