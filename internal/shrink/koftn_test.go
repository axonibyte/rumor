package shrink

import (
	"context"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
)

// flaky is a Want whose verdict recurs only on the attempts it names, and which
// declares itself probabilistic.
//
// THIS IS THE SHAPE A RACE HAS. A statute evaluating rules against a sealed
// ledger answers the same way every time; a check-then-act defect does not, and
// one probe collapses it into "unstable" -- so the finding is never shrunk and
// the clean-world check over it is satisfied VACUOUSLY because the shrink never
// ran (rook, coop r8/38; fable's cure at rumor/1521 item 3).
type flaky struct {
	k, n     int
	recurOn  map[int]bool
	attempts int
}

func (f *flaky) Describe() string     { return "a flaky verdict" }
func (f *flaky) Attempts() (int, int) { return f.k, f.n }
func (f *flaky) Recurs(Candidate) (bool, string) {
	f.attempts++
	if f.recurOn[f.attempts] {
		return true, ""
	}
	return false, "it did not recur on this probe"
}

func TestAProbabilisticVerdictIsProbedKOfN(t *testing.T) {
	f := &fake{}
	// Recurs on the 3rd and 4th probes, so k=2 is met at the 4th and the 5th
	// is never run.
	w := &flaky{k: 2, n: 5, recurOn: map[int]bool{3: true, 4: true}}
	v, err := Once(context.Background(), f, cp, w,
		settled(func(context.Context) (engine.Finding, error) { return bug(), nil }))
	if err != nil {
		t.Fatalf("Once: %v", err)
	}
	if !v.Reproduced {
		t.Fatalf("a verdict that recurred twice in four probes with k=2 was not reproduced: %s", v.Why)
	}
	if v.Recurred != 2 || v.Attempts != 4 {
		t.Errorf("Recurred/Attempts = %d/%d, wanted 2/4: k of n is a THRESHOLD and not a quota, "+
			"so probing past the kth spends replays to learn nothing", v.Recurred, v.Attempts)
	}
	// THE WORLD IS RESTORED BEFORE EVERY PROBE, not once before the first: a
	// probe against the world the previous one left would attribute its
	// finding to both.
	if f.restored != 4 || f.verified != 4 {
		t.Errorf("the provider restored %d time(s) and verified %d over 4 probes; each probe "+
			"needs a world proven clean or the finding is not the candidate's",
			f.restored, f.verified)
	}
}

// A VERDICT THAT RECURS TOO SELDOM IS UNSTABLE AND SAYS SO IN FIGURES, which
// is the half rook and fable both asked for by name: a reader told only
// "unstable" cannot tell a trace that NEVER causes the finding from one that
// causes it one time in five, and those are acted on differently.
func TestAVerdictRecurringTooSeldomSaysRecurredKOfNAndNotUnstable(t *testing.T) {
	f := &fake{}
	w := &flaky{k: 3, n: 5, recurOn: map[int]bool{2: true}}
	v, err := Once(context.Background(), f, cp, w,
		settled(func(context.Context) (engine.Finding, error) { return bug(), nil }))
	if err != nil {
		t.Fatalf("Once: %v", err)
	}
	if v.Reproduced {
		t.Fatal("one recurrence in five met a threshold of three")
	}
	for _, want := range []string{"recurred 1 of 5", "3 was wanted"} {
		if !strings.Contains(v.Why, want) {
			t.Errorf("the reason does not carry %q, so a reader cannot tell sometimes from "+
				"never: %q", want, v.Why)
		}
	}
	if v.Attempts != 5 || v.Recurred != 1 {
		t.Errorf("Attempts/Recurred = %d/%d, wanted 5/1", v.Attempts, v.Recurred)
	}
	if f.restored != 5 {
		t.Errorf("the provider restored %d time(s) over 5 probes", f.restored)
	}
}

// A DETERMINISTIC WANT IS PROBED ONCE, which is the statute's path and must not
// change: probing a verdict twice that cannot differ would double every replay
// in a shrink for nothing.
func TestADeterministicWantIsProbedExactlyOnce(t *testing.T) {
	f := &fake{}
	v, err := Once(context.Background(), f, cp, wantBug(),
		settled(func(context.Context) (engine.Finding, error) { return bug(), nil }))
	if err != nil {
		t.Fatalf("Once: %v", err)
	}
	if !v.Reproduced {
		t.Fatalf("the statute's own finding did not reproduce: %s", v.Why)
	}
	if v.Attempts != 1 || v.Recurred != 1 {
		t.Errorf("Attempts/Recurred = %d/%d, wanted 1/1: a want that does not declare itself "+
			"probabilistic is probed once", v.Attempts, v.Recurred)
	}
	if f.restored != 1 {
		t.Errorf("the provider restored %d time(s) for one probe", f.restored)
	}
}

// A THRESHOLD THAT IS NOT ONE IS REFUSED BY NAME, rather than silently read as
// 1-of-1 or as never: k below one would accept a candidate that recurred not at
// all, and n below k could never be met.
func TestAThresholdThatIsNotOneIsRefused(t *testing.T) {
	for _, c := range []struct{ k, n int }{{0, 3}, {-1, 3}, {4, 2}, {1, 0}} {
		w := &flaky{k: c.k, n: c.n, recurOn: map[int]bool{1: true}}
		_, err := Once(context.Background(), &fake{}, cp, w,
			settled(func(context.Context) (engine.Finding, error) { return bug(), nil }))
		if err == nil {
			t.Errorf("recur %d of %d was accepted as a threshold", c.k, c.n)
			continue
		}
		if !strings.Contains(err.Error(), "which is not a threshold") {
			t.Errorf("%d of %d: the refusal does not say what is wrong: %v", c.k, c.n, err)
		}
	}
}
