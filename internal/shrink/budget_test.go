package shrink

import (
	"context"
	"strings"
	"testing"
	"time"

	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
	"forge.axonibyte.com/axonibyte/rumor/internal/schedule"
)

// budgetRequest is the reduction TestShrinkRunsTheWholeReduction drives -- 12
// steps, the failure needing 4 and create_event -- with a budget, and a world
// that records which lengths it saw reproduce.
// seen is what the world saw reproduce: the lengths, and the last set of kinds
// a candidate ran without and still reproduced -- which is what a stopped
// shrink must report, since each was RUN.
type seen struct {
	lengths     map[int]bool
	lastRemoved []string
}

func budgetRequest(max int, delay time.Duration) (Request, *world, *seen) {
	w := &world{needSteps: 4, needKinds: []string{"create_event"}, all: steps(12)}
	sn := &seen{lengths: map[int]bool{}}
	run := func(c context.Context, n int, without []string) (engine.Finding, []schedule.Step, error) {
		time.Sleep(delay)
		f, ran, err := w.replay(c, n, without)
		if f.Value == "bug" {
			if len(without) == 0 {
				sn.lengths[n] = true
			} else {
				sn.lastRemoved = append([]string(nil), without...)
			}
		}
		return f, ran, err
	}
	return Request{
		Seed: 42, Steps: steps(12), Kinds: []string{"create_event", "join_event", "leave_event"},
		Want: wantBug(), Provider: &fake{}, Checkpoint: cp, Replay: settledReplay(run),
		MaxCandidates: max,
	}, w, sn
}

// smallest is the shortest length the world saw reproduce, or the whole trace.
func (sn *seen) smallest() int {
	out := 12
	for n := range sn.lengths {
		if n < out {
			out = n
		}
	}
	return out
}

// A SHRINK THAT REACHES ITS COUNT STOPS WHERE IT GOT TO (issue 124's
// deliverable 4, coop r9/204), and at every count short of finishing it says
// so: Stopped, exactly that many candidates spent, the smallest reproduction a
// length that was RUN and reproduced -- never an error, never UNSTABLE, never
// "not reproducible", since the gate reproduced the failure.
func TestAShrinkStoppedByItsBudgetSaysHowFarItGot(t *testing.T) {
	whole, _, _ := budgetRequest(0, 0)
	full, err := Shrink(context.Background(), whole)
	if err != nil || full.Stopped {
		t.Fatalf("the unbounded shrink: %+v, %v", full, err)
	}
	n := full.Probes
	if n < 4 {
		t.Fatalf("the unbounded shrink spent %d candidate(s), too few for this arm to step through", n)
	}
	for max := 1; max < n; max++ {
		req, w, sn := budgetRequest(max, 0)
		got, err := Shrink(context.Background(), req)
		if err != nil {
			t.Fatalf("budget %d: Shrink returned an error, where a spent budget is a result: %v", max, err)
		}
		if !got.Stopped || !got.Stable {
			t.Fatalf("budget %d of %d: Stopped=%v Stable=%v, want a stopped stable shrink", max, n, got.Stopped, got.Stable)
		}
		if got.Probes != max || w.calls != max {
			t.Fatalf("budget %d: reported %d candidate(s) and ran %d", max, got.Probes, w.calls)
		}
		// THE SMALLEST SO FAR, EXACTLY: not merely a length that reproduced
		// but the shortest one that did. A stop that fell back to the whole
		// trace would still name a length that reproduced, and would have
		// thrown away what the bisection established.
		if got.Reduction.Steps != sn.smallest() {
			t.Fatalf("budget %d: reported %d step(s), but the shortest the world saw reproduce was %d",
				max, got.Reduction.Steps, sn.smallest())
		}
		// AND THE KINDS ALREADY REMOVED STAY REMOVED: each was run without
		// and the failure held, which a stop must not forget.
		if strings.Join(got.Kinds.Removed, ",") != strings.Join(sn.lastRemoved, ",") {
			t.Fatalf("budget %d: reported removed %v, but the last reproducing candidate ran without %v",
				max, got.Kinds.Removed, sn.lastRemoved)
		}
		out := got.Render()
		if !strings.Contains(out, "STOPPED AT ITS BUDGET") || !strings.Contains(out, "smallest reproduction so far") {
			t.Fatalf("budget %d: the report does not say it stopped and how far it got:\n%s", max, out)
		}
		if strings.Contains(out, "UNSTABLE") || strings.Contains(out, "not reproduc") {
			t.Fatalf("budget %d: a stopped shrink reads as a failure to reproduce:\n%s", max, out)
		}
	}
	// AND A BUDGET IT DOES NOT REACH CHANGES NOTHING: the presence partner, so
	// a build that stopped every shrink could not pass above.
	req, _, _ := budgetRequest(n, 0)
	got, err := Shrink(context.Background(), req)
	if err != nil || got.Stopped || got.Reduction.Steps != full.Reduction.Steps ||
		strings.Join(got.Kinds.Removed, ",") != strings.Join(full.Kinds.Removed, ",") {
		t.Fatalf("a budget equal to the shrink's own count changed it: %+v, %v", got, err)
	}
}

// THE SAME COUNT GIVES THE SAME REPRODUCTION HOWEVER SLOW THE BOX (rook, coop
// r9/203): candidates are counted, not timed, so a replay ten times slower
// stops at the same place. This is "two loads" made deterministic -- the delay
// stands in for the load, and only a budget read off a clock could tell them
// apart.
func TestABudgetStopsAtTheSamePlaceWhateverTheReplayCosts(t *testing.T) {
	for _, max := range []int{2, 4} {
		fast, _, _ := budgetRequest(max, 0)
		slow, _, _ := budgetRequest(max, 15*time.Millisecond)
		a, aerr := Shrink(context.Background(), fast)
		b, berr := Shrink(context.Background(), slow)
		if aerr != nil || berr != nil {
			t.Fatalf("budget %d: %v / %v", max, aerr, berr)
		}
		if a.Render() != b.Render() {
			t.Fatalf("budget %d: a slower replay stopped somewhere else:\n--- fast\n%s--- slow\n%s", max, a.Render(), b.Render())
		}
	}
}
