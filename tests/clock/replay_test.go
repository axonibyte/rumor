package clock_test

import (
	"fmt"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/ledger"
)

// A REPLAY OF A CLOCKED RUN CARRIES THE CLOCK, which is the vocabulary's own
// sentence -- "Replays restore the clock with everything else" -- and fable's
// D5 (coop breakout-r7/0).
//
// IT IS STRUCTURAL, AND THAT IS WHY IT NEEDS AN ARM RATHER THAN A PARAGRAPH.
// schedule.Replay drives the same Walk with the same Project, so clock control
// travels because it is a field of the thing being handed over; nothing
// converts or re-derives it. A later phase that assembled a Project for the
// replay path out of parts -- the shape cmd/rumor already uses for `run` --
// would drop the field with nothing to notice, and internal/shrink judges a
// candidate by comparing its replay's finding against the original's.
//
// WITH A DURATION BOUND THE LOSS IS LOUD RATHER THAN QUIET, which is why this
// arm uses that fixture: a replay that lost the clock cannot measure the bound
// and refuses by name, so the failure arrives as a could-not-run instead of as
// two findings that disagree for an unstated reason. That is the difference
// between a defect a reader can act on and a shrink reducing against the wrong
// failure.

// factLines renders a ledger for comparison, the clock's TIME included. A
// comparison over kinds and positions alone would pass a run whose clock facts
// all read a different time, which is the half this suite is about.
func factLines(facts []ledger.Fact) []string {
	out := make([]string, 0, len(facts))
	for _, f := range facts {
		t := "-"
		if f.T != nil {
			t = fmt.Sprint(*f.T)
		}
		out = append(out, fmt.Sprintf("%d %s %s %s %v t=%s",
			f.Position, f.Kind, f.Actor, f.Ref, f.Value, t))
	}
	return out
}

func TestAClockedRunIsTheSameRunTwice(t *testing.T) {
	const steps = 6
	dir := clocked(t)
	first := walk(t, dir, 1, steps)
	second := walk(t, dir, 1, steps)

	a, b := factLines(first.Facts), factLines(second.Facts)
	if len(a) != len(b) {
		t.Fatalf("%d fact(s) and then %d from one seed: a clocked run is not repeatable, which "+
			"is the property the unit was chosen to keep\n%v\n%v", len(a), len(b), a, b)
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("fact %d is %q and then %q: two runs of one seed differ, so something in "+
				"the clock is reading what the run does not own", i+1, a[i], b[i])
		}
	}
	if n := len(clockFacts(first.Facts)); n == 0 {
		t.Fatal("no clock fact in either run, so this compared two unclocked runs and says " +
			"nothing about the clock")
	}
}

func TestAReplayOfAClockedRunCarriesTheClock(t *testing.T) {
	// settledSeed AND NOT heldSeed: a run that ends with an unmeasured pending
	// now refuses (fable, coop r7/15), and a replay compared against a refused
	// walk would be comparing two refusals. 66 settles, so both sides have a
	// verdict to compare.
	dir := durationBounded(t, true)
	original := walkProfile(t, dir, settledSeed, heldSteps, heldProfile)

	finding, steps, open, err := replayed(t, dir, settledSeed, heldSteps, heldProfile)
	if err != nil {
		t.Fatalf("the replay refused where the walk ran: %v\n\nTHAT IS THE SHAPE THIS ARM IS "+
			"FOR: with a duration bound, a replay that did not carry the clock cannot measure "+
			"the bound, so it refuses rather than returning a finding that disagrees quietly", err)
	}

	if len(steps) != len(original.Steps) {
		t.Errorf("the replay took %d step(s) and the walk %d", len(steps), len(original.Steps))
	}
	if open != original.PendingOpen {
		t.Errorf("the replay ended with %d pending(s) open and the walk with %d: the figure a "+
			"shrink judges a candidate's settling by did not survive the replay",
			open, original.PendingOpen)
	}
	switch {
	case (finding == nil) != (original.Finding == nil):
		t.Errorf("the replay found %v and the walk found %v: one of them is the finding a shrink "+
			"would compare against", finding, original.Finding)
	case finding != nil && finding.Says() != original.Finding.Says():
		t.Errorf("the replay found %q and the walk found %q", finding.Says(), original.Finding.Says())
	}
}
