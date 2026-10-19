// Arms for the run's only source of chance. These are the LIBRARY halves of
// fable's D1 and D2 (breakout-r5/0): the drills drive a whole run, and a drill
// that fails because the generator is not deterministic would be reported
// against the walk. Each arm here states what it would catch.
package schedule

import "testing"

// draws is enough to make a coincidence absurd rather than merely unlikely:
// two different PCG streams agreeing on 1,000 consecutive bounded draws has
// probability about 10^-300 at n=8. A SMALLER number is the thing to question
// if this ever fails -- a 10-draw arm would pass on a generator that had
// collapsed to a short cycle.
const draws = 1000

func sequence(t *testing.T, s *Source, n int) []int {
	t.Helper()
	out := make([]int, n)
	for i := range out {
		v, err := s.IntN(8)
		if err != nil {
			t.Fatalf("draw %d of %d refused: %v", i, n, err)
		}
		out[i] = v
	}
	return out
}

// D1's library half: ONE seed, two Sources, the same sequence. What it would
// catch: a generator seeded from the clock, from a package-level default, or
// from anything the seed does not carry -- which is the defect that makes a
// shrink step unre-runnable.
func TestOneSeedDrawsOneSequence(t *testing.T) {
	a := sequence(t, New(42), draws)
	b := sequence(t, New(42), draws)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("draw %d differs between two Sources of seed 42: %d vs %d", i, a[i], b[i])
		}
	}
	t.Logf("PROOF %d draws identical from two Sources of one seed", draws)
}

// THE OTHER SIDE, and without it the arm above passes on a generator that
// returns a constant. A different seed must produce a different sequence.
func TestADifferentSeedDrawsADifferentSequence(t *testing.T) {
	a := sequence(t, New(42), draws)
	b := sequence(t, New(43), draws)
	same := 0
	for i := range a {
		if a[i] == b[i] {
			same++
		}
	}
	if same == len(a) {
		t.Fatalf("seeds 42 and 43 drew the SAME %d values: the seed is not reaching the generator", len(a))
	}
	// at n=8 over 1000 draws, matching positions cluster near 125; the arm is
	// about "not identical", and the figure is logged so a reader can see the
	// generator is not merely offset by one.
	t.Logf("PROOF %d of %d positions coincide between seeds 42 and 43", same, draws)
}

// REPLAY IS THE WHOLE POINT: a Source re-made from a run's printed seed draws
// that run's sequence. D2 and D11 (a promoted seed, months later) both rest on
// this, and it is the same property as D1 read from the operator's side.
func TestThePrintedSeedReproducesTheRun(t *testing.T) {
	first := New(20261019)
	printed := first.Seed()
	want := sequence(t, first, draws)
	got := sequence(t, New(printed), draws)
	for i := range want {
		if want[i] != got[i] {
			t.Fatalf("draw %d of the replay differs: %d vs %d", i, want[i], got[i])
		}
	}
}

// Seed() is what a run prints FIRST, so it must be the integer handed in and
// not a transformation of it: an operator retyping it must land on this run.
func TestSeedIsTheIntegerGivenAndNotADerivation(t *testing.T) {
	for _, seed := range []uint64{0, 1, 42, 1 << 63, ^uint64(0)} {
		if got := New(seed).Seed(); got != seed {
			t.Errorf("New(%d).Seed() = %d: a printed seed that is not the seed cannot be retyped", seed, got)
		}
	}
}

// AN EMPTY APPLICABLE SET IS A FINDING, NOT A PANIC. The stdlib panics on
// IntN(0); a walk reaching a step where nothing is applicable has something to
// say about the project, and a panic here would read as a defect in this
// package instead.
func TestADrawFromNothingIsRefusedRatherThanFatal(t *testing.T) {
	for _, n := range []int{0, -1, -1000} {
		if _, err := New(1).IntN(n); err == nil {
			t.Errorf("IntN(%d) returned no error: the stdlib would panic and the walk would lose its finding", n)
		}
	}
	if _, err := New(1).IntN(1); err != nil {
		t.Errorf("IntN(1) refused, and a single candidate IS a choice: %v", err)
	}
}

// Mint is the ONE entry of entropy, and New must not share it. Two mints
// differing proves Mint is live; New(seed) being unaffected proves the run's
// determinism does not depend on Mint having been called.
func TestMintIsTheOnlyEntropyAndNewDoesNotUseIt(t *testing.T) {
	a, err := Mint()
	if err != nil {
		t.Fatalf("Mint refused: %v", err)
	}
	b, err := Mint()
	if err != nil {
		t.Fatalf("Mint refused on the second call: %v", err)
	}
	if a == b {
		t.Errorf("two mints gave %d twice: a run with no --seed would reuse one corpus", a)
	}
	before := sequence(t, New(7), 50)
	if _, err := Mint(); err != nil {
		t.Fatalf("Mint refused between runs: %v", err)
	}
	after := sequence(t, New(7), 50)
	for i := range before {
		if before[i] != after[i] {
			t.Fatalf("New(7)'s sequence changed across a Mint at draw %d: the seed is not the whole of the entropy", i)
		}
	}
}
