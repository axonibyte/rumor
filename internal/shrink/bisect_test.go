package shrink

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
	"forge.axonibyte.com/axonibyte/rumor/internal/rollback"
)

// needs returns a run function whose failure requires at least `k` steps, which
// is the monotonic case: every prefix at or above k reproduces and none below.
func needs(k int) func(context.Context, int) (engine.Finding, error) {
	return func(_ context.Context, steps int) (engine.Finding, error) {
		if steps >= k {
			return bug(), nil
		}
		return engine.Finding{Defined: true, Value: "consistent",
			Rule: "data.rumor.statute.finding"}, nil
	}
}

func TestBisectFindsTheShortestReproducingPrefix(t *testing.T) {
	f := &fake{}
	r, err := Bisect(context.Background(), f, cp, bug(), 10, settledN(needs(3)))
	if err != nil {
		t.Fatalf("Bisect: %v", err)
	}
	if r.Steps != 3 {
		t.Errorf("shortened to %d step(s), want 3", r.Steps)
	}
	if !r.Found {
		t.Error("Found is false though 3 < 10")
	}

	if r.Probes < 2 {
		t.Errorf("%d probe(s) cannot have established both the bound and a shorter length", r.Probes)
	}

	// AND EVERY CANDIDATE GOT ITS OWN VERIFIED CLEAN WORLD: one restore and one
	// verify per probe, which is the property the gate exists to enforce and
	// the reason a shrink is expensive.
	if f.restored != r.Probes || f.verified != r.Probes {
		t.Errorf("%d probe(s) but %d restore(s) and %d verify(s): a candidate ran on a world that "+
			"was not rebuilt for it", r.Probes, f.restored, f.verified)
	}
}

// IT MUST ACTUALLY HALVE, AND THE FIXTURE IS WHAT MAKES THAT MEASURABLE.
//
// My first version of this arm bounded the probes on a 10-step trace whose
// answer was 3, and a `mid := lo` mutant SURVIVED it: a scan starting at 1
// reaches 3 in three probes, comfortably inside any bound that a bisection also
// meets. The fixture made both strategies cheap, so the assertion could not fail
// for the case it was written to catch.
//
// The answer here is 33 of 64, far from both ends: a bisection needs the bound
// probe plus six halvings, a scan from 1 needs thirty-three. No bound satisfies
// both, which is what makes the number evidence about the SEARCH rather than
// about the answer.
func TestBisectHalvesRatherThanScanning(t *testing.T) {
	r, err := Bisect(context.Background(), &fake{}, cp, bug(), 64, settledN(needs(33)))
	if err != nil {
		t.Fatalf("Bisect: %v", err)
	}
	if r.Steps != 33 {
		t.Fatalf("shortened to %d, want 33", r.Steps)
	}
	// 1 + ceil(log2(64)) = 7. A scan would be 33.
	if r.Probes > 8 {
		t.Errorf("%d probes to find 33 in 64: a halving search needs about seven, so this is a "+
			"scan wearing a bisection's name", r.Probes)
	}
}

// THE WHOLE TRACE IS THE ANSWER WHEN NOTHING SHORTER REPRODUCES, and Found says
// so rather than the caller inferring it from Steps == n.
func TestBisectKeepsTheWholeTraceWhenNothingShorterReproduces(t *testing.T) {
	r, err := Bisect(context.Background(), &fake{}, cp, bug(), 8, settledN(needs(8)))
	if err != nil {
		t.Fatalf("Bisect: %v", err)
	}
	if r.Steps != 8 {
		t.Errorf("Steps = %d, want the original 8", r.Steps)
	}
	if r.Found {
		t.Error("Found is true though nothing shorter than 8 reproduced")
	}
}

// NON-MONOTONIC REPRODUCTION: THE RESULT REPRODUCES, AND THAT IS THE WHOLE
// CLAIM. A shorter trace is a different world rather than a subset of a longer
// one's behaviour, so a predicate can hold at 5, fail at 3 and hold again at 1.
// This arm therefore asserts the returned length REPRODUCES -- it deliberately
// does not assert the global minimum, because the search cannot establish one
// and design.md's "minimal trace" names the output's shape rather than a
// property of it. An arm demanding 1 here would be asserting something the
// algorithm never promised.
func TestBisectUnderNonMonotonicReproductionReturnsAReproducingLength(t *testing.T) {
	odd := func(_ context.Context, steps int) (engine.Finding, error) {
		if steps == 1 || steps >= 5 {
			return bug(), nil
		}
		return engine.Finding{Defined: true, Value: "consistent",
			Rule: "data.rumor.statute.finding"}, nil
	}
	r, err := Bisect(context.Background(), &fake{}, cp, bug(), 9, settledN(odd))
	if err != nil {
		t.Fatalf("Bisect: %v", err)
	}
	got, err := odd(context.Background(), r.Steps)
	if err != nil {
		t.Fatalf("re-running the result: %v", err)
	}
	if got.Value != "bug" {
		t.Errorf("Bisect returned %d step(s), which does NOT reproduce: the invariant is that the "+
			"length it reports was run and reproduced", r.Steps)
	}
	if r.Steps >= 9 {
		t.Errorf("Steps = %d, so nothing was shortened at all on a trace that reproduces at 1", r.Steps)
	}
}

// A WORLD THAT CANNOT BE VERIFIED STOPS THE BISECTION, rather than the
// candidate being skipped: lengths measured in different worlds are not
// comparable, so carrying on would produce a "minimal" trace that is minimal in
// no single world.
func TestBisectStopsOnADirtyWorld(t *testing.T) {
	f := &fake{verify: rollback.ErrDirty}
	r, err := Bisect(context.Background(), f, cp, bug(), 10, settledN(needs(3)))
	if err == nil {
		t.Fatal("a dirty world did not stop the bisection")
	}
	if !errors.Is(err, rollback.ErrDirty) {
		t.Errorf("the error does not wrap rollback.ErrDirty: %v", err)
	}
	if !strings.Contains(err.Error(), "candidate") {
		t.Errorf("the error should name which candidate failed; got %v", err)
	}
	// THE PROBE COUNT SURVIVES THE ERROR, so a caller can say how many worlds
	// were spent before it stopped.
	if r.Probes != 1 {
		t.Errorf("Probes = %d, want the one probe that hit the dirty world", r.Probes)
	}
}

// AN UPPER BOUND THAT DOES NOT REPRODUCE IS A SEQUENCING ERROR. Searching for a
// shorter version of a failure that is not there would make every length it
// reported meaningless, so it refuses and names the gate that establishes this.
func TestBisectRefusesATraceThatDoesNotReproduce(t *testing.T) {
	_, err := Bisect(context.Background(), &fake{}, cp, bug(), 6,
		settledN(func(context.Context, int) (engine.Finding, error) {
			return engine.Finding{Defined: true, Value: "consistent",
				Rule: "data.rumor.statute.finding"}, nil
		}))
	if err == nil {
		t.Fatal("a trace that does not reproduce was bisected anyway")
	}
	if !strings.Contains(err.Error(), "no failure to shorten") {
		t.Errorf("the refusal must say there is no failure to shorten; got %v", err)
	}
}

func TestBisectRefusals(t *testing.T) {
	for _, c := range []struct {
		label string
		n     int
		fn    func(context.Context, int) (engine.Finding, error)
		says  string
	}{
		{"zero steps", 0, needs(1), "cannot be bisected"},
		{"a negative length", -3, needs(1), "cannot be bisected"},
		{"no run function", 5, nil, "no run function"},
	} {
		_, err := Bisect(context.Background(), &fake{}, cp, bug(), c.n, settledN(c.fn))
		if err == nil {
			t.Errorf("%s: accepted", c.label)
			continue
		}
		if !strings.Contains(err.Error(), c.says) {
			t.Errorf("%s: refusal must say %q; got %v", c.label, c.says, err)
		}
	}
}

// A run THAT FAILS IS A COULD-NOT-RUN AND NOT A NON-REPRODUCTION, because
// reading an error as "did not reproduce" would shorten a trace on the strength
// of a broken replay.
func TestBisectDoesNotReadARunErrorAsNonReproduction(t *testing.T) {
	_, err := Bisect(context.Background(), &fake{}, cp, bug(), 10,
		settledN(func(_ context.Context, steps int) (engine.Finding, error) {
			if steps == 10 {
				return bug(), nil
			}
			return engine.Finding{}, fmt.Errorf("the adapter died")
		}))
	if err == nil {
		t.Fatal("a replay that errored was read as a non-reproduction")
	}
	if !strings.Contains(err.Error(), "the adapter died") {
		t.Errorf("the cause must survive; got %v", err)
	}
}
