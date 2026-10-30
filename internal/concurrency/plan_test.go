package concurrency

import (
	"strings"
	"testing"
	"time"
)

// ONE SEED RENDERS ONE PLAN, which is what D1's byte-identity rests on.
func TestOneSeedRendersOnePlan(t *testing.T) {
	for _, width := range []int{2, 3, 7, 32} {
		a, err := PlanFor(99, width)
		if err != nil {
			t.Fatalf("width %d: %v", width, err)
		}
		b, err := PlanFor(99, width)
		if err != nil {
			t.Fatalf("width %d: %v", width, err)
		}
		if a.Render() != b.Render() {
			t.Errorf("width %d: two plans from one seed differ:\n  %s\n  %s",
				width, a.Render(), b.Render())
		}
		if a.Width() != width {
			t.Errorf("width %d: the plan puts %d request(s) in flight", width, a.Width())
		}
	}
}

// AND SEED+1 MOVES IT, WHICH IS THE HALF THAT MATTERS.
//
// This is rook's finding at coop r8/36 made into an arm. A stage whose only
// contribution to the hashed stream were a clock-free "did overlap" would put
// ONE BIT there, and one bit cannot differ between seed N and seed N+1 for
// every N -- so byte-identity would hold always and D9 would be an oracle that
// cannot fail. The plan is what carries more than a bit, so this arm is what
// says the subject is real.
//
// NOT EVERY ADJACENT PAIR NEED DIFFER AT WIDTH 2, where there are only two
// permutations: at that width two seeds agree half the time by arithmetic, and
// an arm demanding otherwise would be wrong rather than strict. So the claim is
// made where it is meaningful -- over a run of seeds, the plans are not all the
// same -- and the WIDTH at which it is checked is stated.
func TestSeedPlusOneMovesThePlanSoD9HasASubject(t *testing.T) {
	const width = 6
	seen := map[string]int{}
	for seed := uint64(1); seed <= 40; seed++ {
		p, err := PlanFor(seed, width)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		seen[p.Render()]++
	}
	if len(seen) < 2 {
		t.Fatalf("forty seeds at width %d produced %d distinct plan(s): the hashed stream would "+
			"be byte-identical across every seed, so D9 could never fail", width, len(seen))
	}
	// AND ADJACENT SEEDS DIFFER AT LEAST SOMETIMES, which is the specific
	// claim rook's wording makes: seed+1 must be able to move it.
	moved := 0
	for seed := uint64(1); seed < 40; seed++ {
		a, _ := PlanFor(seed, width)
		b, _ := PlanFor(seed+1, width)
		if a.Render() != b.Render() {
			moved++
		}
	}
	if moved == 0 {
		t.Errorf("no adjacent seed pair moved the plan at width %d, so seed+1 cannot move the "+
			"hashed stream", width)
	}
	t.Logf("PROOF %d distinct plan(s) over 40 seeds at width %d; %d of 39 adjacent pairs differ",
		len(seen), width, moved)
}

// THE PLAN IS CLOCK-FREE, CHECKED AND NOT PROMISED. A clock that leaked into
// the rendering would move the hashed stream every run, which is the failure
// mode HangMeasured is kept off stdout to avoid.
func TestThePlansRenderingCarriesNoClock(t *testing.T) {
	p, err := PlanFor(7, 4)
	if err != nil {
		t.Fatalf("PlanFor: %v", err)
	}
	first := p.Render()
	time.Sleep(5 * time.Millisecond)
	if again := p.Render(); again != first {
		t.Errorf("the rendering moved between two calls, so something in it reads a clock:\n"+
			"  %s\n  %s", first, again)
	}
	// Re-derived from the same seed after a wait, too: the draw itself must
	// not reach for a clock either.
	q, err := PlanFor(7, 4)
	if err != nil {
		t.Fatalf("PlanFor: %v", err)
	}
	if q.Render() != first {
		t.Errorf("the same seed drew a different plan after a wait, so the DRAW reads a clock:\n"+
			"  %s\n  %s", first, q.Render())
	}
	for _, unit := range []string{"ms", "µs", "ns", "s)"} {
		if strings.Contains(first, unit) {
			t.Errorf("the rendering carries what looks like a duration (%q): %s", unit, first)
		}
	}
}

// A PLAN OF ONE IS REFUSED, for Fan's reason: it is the sequential path.
func TestAPlanNarrowerThanTwoIsRefused(t *testing.T) {
	for _, width := range []int{1, 0, -3} {
		if _, err := PlanFor(1, width); err == nil {
			t.Errorf("a fan-out of %d drew a plan, so the sequential path could be planned as a "+
				"concurrent one", width)
		}
	}
}
