package shrink

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
	"forge.axonibyte.com/axonibyte/rumor/internal/schedule"
)

// world is a fake system under test: the failure needs `needSteps` steps and
// every kind in `needKinds`, and the steps it returns are the ones that ran.
type world struct {
	needSteps int
	needKinds []string

	// neededAbove makes a kind required ONLY when the candidate is at least
	// that long. It exists so a fixture can tell "kind removal ran on the
	// SHORTENED trace" from "it ran on the whole one": without it the
	// removable set is the same at both lengths and the two are
	// indistinguishable, which let a mutant swapping them survive.
	neededAbove map[string]int

	all   []schedule.Step
	calls int
}

func (w *world) replay(_ context.Context, steps int, without []string) (engine.Finding, []schedule.Step, error) {
	w.calls++
	drop := map[string]bool{}
	for _, k := range without {
		drop[k] = true
	}
	var ran []schedule.Step
	for _, s := range w.all {
		if s.N > steps {
			break
		}
		if drop[s.Kind] {
			continue
		}
		ran = append(ran, s)
	}
	reproduces := steps >= w.needSteps
	for _, k := range w.needKinds {
		if drop[k] {
			reproduces = false
		}
	}
	for k, above := range w.neededAbove {
		if steps >= above && drop[k] {
			reproduces = false
		}
	}
	if reproduces {
		return bug(), ran, nil
	}
	return engine.Finding{Defined: true, Value: "consistent",
		Rule: "data.rumor.statute.finding"}, ran, nil
}

func steps(n int) []schedule.Step {
	var out []schedule.Step
	for i := 1; i <= n; i++ {
		kind := "create_event"
		if i%3 == 0 {
			kind = "join_event"
		} else if i%5 == 0 {
			kind = "leave_event"
		}
		out = append(out, schedule.Step{N: i, Actor: "member_1", Kind: kind,
			Handles: []string{fmt.Sprintf("member_1.event#%d", i)}})
	}
	return out
}

// THE WHOLE REDUCTION, IN design.md's ORDER, AND THE PIECES ACTUALLY RUN.
func TestShrinkRunsTheWholeReduction(t *testing.T) {
	w := &world{needSteps: 4, needKinds: []string{"create_event"}, all: steps(12)}
	f := &fake{}
	dir := t.TempDir()
	seeds := filepath.Join(dir, "seeds.lock")

	got, err := Shrink(context.Background(), Request{
		Seed: 42, Steps: steps(12), Kinds: []string{"create_event", "join_event", "leave_event"},
		Want: bug(), Provider: f, Checkpoint: cp, SeedsFile: seeds, Replay: w.replay,
	})
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	if !got.Stable {
		t.Fatalf("the gate called a reproducing failure unstable: %s", got.Why)
	}
	if got.Reduction.Steps != 4 {
		t.Errorf("shortened to %d step(s), want 4", got.Reduction.Steps)
	}
	// join_event and leave_event are not needed; create_event is.
	if strings.Join(got.Kinds.Removed, ",") != "join_event,leave_event" {
		t.Errorf("removed %v, want the two kinds the failure survives without", got.Kinds.Removed)
	}
	if !got.Promoted {
		t.Error("the seed was not promoted")
	}
	if raw, rerr := os.ReadFile(seeds); rerr != nil || !strings.Contains(string(raw), "42") {
		t.Errorf("the seeds file does not hold the seed: %v %q", rerr, raw)
	}

	// EVERY CANDIDATE GOT ITS OWN WORLD, the gate's included, and the count is
	// the sum the Result reports rather than one the test recomputes.
	if f.restored != got.Probes-1 {
		t.Errorf("%d restore(s) for %d probe(s): the final replay is the one that does not "+
			"rebuild a world, so restores should be probes minus one", f.restored, got.Probes)
	}
}

// THE TRACE IS THE ONE THAT RAN, not a slice of the input. The world drops the
// removed kinds, so the reported steps must carry none of them -- an assembly
// that sliced Request.Steps would still hold join_event.
func TestShrinkReportsTheTraceThatRan(t *testing.T) {
	w := &world{needSteps: 4, needKinds: []string{"create_event"}, all: steps(12)}
	got, err := Shrink(context.Background(), Request{
		Seed: 7, Steps: steps(12), Kinds: []string{"create_event", "join_event", "leave_event"},
		Want: bug(), Provider: &fake{}, Checkpoint: cp, Replay: w.replay,
	})
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	if len(got.Trace.Steps) == 0 {
		t.Fatal("the trace has no steps")
	}
	for _, s := range got.Trace.Steps {
		if s.Kind == "join_event" || s.Kind == "leave_event" {
			t.Errorf("the reported trace holds %s, a kind the reduction removed: %+v",
				s.Kind, got.Trace.Steps)
		}
		if s.N > got.Reduction.Steps {
			t.Errorf("the reported trace holds step %d, past the %d it shortened to",
				s.N, got.Reduction.Steps)
		}
	}
	if got.Trace.Seed != 7 {
		t.Errorf("the trace carries seed %d", got.Trace.Seed)
	}
}

// KIND REMOVAL RUNS ON THE SHORTENED TRACE, AND THE FIXTURE IS WHAT MAKES THAT
// VISIBLE.
//
// A mutant passing len(req.Steps) instead of red.Steps SURVIVED the first
// battery: in a world where the removable kinds are the same at both lengths,
// the two produce identical results and no assertion can separate them. Here
// join_event is required only once a candidate is 10 steps or longer -- so at
// the shortened length of 4 it comes out, and against the whole 12-step trace
// it does not. The removed SET is then the evidence.
func TestKindRemovalRunsOnTheShortenedTrace(t *testing.T) {
	w := &world{
		needSteps:   4,
		needKinds:   []string{"create_event"},
		neededAbove: map[string]int{"join_event": 10},
		all:         steps(12),
	}
	got, err := Shrink(context.Background(), Request{
		Seed: 5, Steps: steps(12), Kinds: []string{"create_event", "join_event", "leave_event"},
		Want: bug(), Provider: &fake{}, Checkpoint: cp, Replay: w.replay,
	})
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	if got.Reduction.Steps != 4 {
		t.Fatalf("shortened to %d, want 4", got.Reduction.Steps)
	}
	if strings.Join(got.Kinds.Removed, ",") != "join_event,leave_event" {
		t.Errorf("removed %v: join_event is removable at 4 steps and NOT at 12, so this set says "+
			"the removal pass ran against the wrong length", got.Kinds.Removed)
	}
}

// AN UNSTABLE SEED IS NOT SHRUNK AND NOT PROMOTED, which is the outcome worse
// than not shrinking: a flake in the regression suite.
func TestAnUnstableSeedIsNotShrunkOrPromoted(t *testing.T) {
	w := &world{needSteps: 99, needKinds: nil, all: steps(5)} // never reproduces
	f := &fake{}
	dir := t.TempDir()
	seeds := filepath.Join(dir, "seeds.lock")

	got, err := Shrink(context.Background(), Request{
		Seed: 13, Steps: steps(5), Kinds: []string{"create_event"},
		Want: bug(), Provider: f, Checkpoint: cp, SeedsFile: seeds, Replay: w.replay,
	})
	if err != nil {
		t.Fatalf("an unstable seed is a verdict, not an error: %v", err)
	}
	if got.Stable {
		t.Fatal("a failure that never came back was called stable")
	}
	if got.Why == "" {
		t.Error("Stable is false and Why says nothing")
	}
	if got.Promoted {
		t.Error("an unstable seed was promoted into the regression suite")
	}
	if _, serr := os.Stat(seeds); serr == nil {
		t.Error("a seeds file was written for an unstable seed")
	}
	// AND NOTHING WAS BISECTED: one probe, the gate's.
	if got.Probes != 1 {
		t.Errorf("%d probe(s) for an unstable seed, want the gate's one", got.Probes)
	}
	if got.Reduction.Steps != 0 || len(got.Kinds.Removed) != 0 {
		t.Errorf("an unstable seed was reduced: %+v %+v", got.Reduction, got.Kinds)
	}
}

// NO SEEDS FILE PROMOTES NOTHING, which a caller shrinking for a report rather
// than a regression suite wants.
func TestShrinkPromotesNothingWithoutASeedsFile(t *testing.T) {
	w := &world{needSteps: 2, needKinds: []string{"create_event"}, all: steps(6)}
	got, err := Shrink(context.Background(), Request{
		Seed: 1, Steps: steps(6), Kinds: []string{"create_event", "join_event"},
		Want: bug(), Provider: &fake{}, Checkpoint: cp, Replay: w.replay,
	})
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	if got.Promoted {
		t.Error("something was promoted with no seeds file named")
	}
}

func TestShrinkRefusals(t *testing.T) {
	w := &world{needSteps: 1, all: steps(3)}
	for _, c := range []struct {
		label string
		req   Request
		says  string
	}{
		{"no replay", Request{Steps: steps(3), Provider: &fake{}, Checkpoint: cp}, "no replay function"},
		{"no steps", Request{Provider: &fake{}, Checkpoint: cp, Replay: w.replay}, "no steps"},
	} {
		_, err := Shrink(context.Background(), c.req)
		if err == nil {
			t.Errorf("%s: accepted", c.label)
			continue
		}
		if !strings.Contains(err.Error(), c.says) {
			t.Errorf("%s: refusal must say %q; got %v", c.label, c.says, err)
		}
	}
}
