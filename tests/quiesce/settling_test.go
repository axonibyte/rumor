// R8/110's periodic reconciliation: the "selective WHEN (settling, periodic
// reconciliation)" that R5/152 deferred to the api-sim BY NAME, with the arms
// that separate the period's MECHANISM from the reconciliation's EFFECT.
package quiesce_test

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
	"forge.axonibyte.com/axonibyte/rumor/internal/schedule"
)

// countingProber answers a fixed state and COUNTS the asks, which is what makes
// the "draws nothing" arm below non-vacuous: without a count, a period that was
// never wired to anything would pass that arm trivially -- the two walks would
// be identical because nothing ran, not because nothing was drawn.
type countingProber struct {
	state engine.QuiesceState
	asks  *int
}

func (c countingProber) Quiesce(context.Context) (engine.QuiesceState, error) {
	*c.asks++
	return c.state, nil
}

func (c countingProber) State(context.Context) (engine.QuiesceState, error) {
	return engine.QuiesceBusy, nil
}

// settlingWalk walks the judged project with a declared period. `observes`
// false strips the OBSERVATIONS, which is how a walk that HOLDS NOTHING is
// built: with nothing observed nothing is judged, so no pending is ever created
// and the walk still runs every step.
//
// STRIPPING THE EVENTUAL TYPE INSTEAD WAS THE FIRST ATTEMPT AND IT WAS WRONG.
// Without it a lagging observation is judged `bug` rather than `pending` --
// which is the entire reason Project.Eventual exists -- so the walk stopped at
// step 3 on a real finding, and every arm built on that fixture was measuring a
// three-step walk it had not asked for.
func settlingWalk(t *testing.T, seed uint64, end engine.QuiesceState, every int,
	observes bool, asks *int) schedule.Record {
	t.Helper()
	p := judgedProject(t, end)
	p.ReconcileEvery = every
	if !observes {
		p.Observations = nil
	}
	if asks != nil {
		p.Prober = countingProber{state: end, asks: asks}
	}
	rec, err := schedule.Walk(context.Background(), p, fakeSpawn(t), seed, 8, 1)
	if err != nil {
		t.Fatalf("seed %d, period %d: %v", seed, every, err)
	}
	return rec
}

// TestThePeriodItselfDrawsNothing is R5/152's "NOTHING IS DRAWN" clause applied
// to the WHEN it deferred, and it is the only arm that can measure it.
//
// WHY THE FIXTURE HOLDS NOTHING. Comparing period 0 against period N on a
// project that DOES hold pendings would compare two different DECLARED WORLDS:
// clearing a pending at step 2 changes the live bounds, which changes what
// `advance` lands on, which moves the draws -- by design, and documented on
// Project.ReconcileEvery. With no OBSERVATIONS nothing is ever judged and so
// nothing is ever held: the periodic pass fires, asks the prober, finds nothing
// open and returns, and the walk still runs all eight steps. What is left is
// the period's mechanism alone, and it must move nothing.
func TestThePeriodItselfDrawsNothing(t *testing.T) {
	offAsks, onAsks := 0, 0
	off := settlingWalk(t, 1, engine.QuiesceQuiet, 0, false, &offAsks)
	on := settlingWalk(t, 1, engine.QuiesceQuiet, 1, false, &onAsks)

	// THE PERIOD ACTUALLY RAN, asserted before anything is concluded from the
	// two walks agreeing. One ask is the end-of-run pass; a period of 1 over
	// eight steps asks at every step as well.
	if onAsks <= offAsks {
		t.Fatalf("a period of 1 asked the prober %d time(s) and no period asked %d: the "+
			"period never fired, so every arm below would pass on a disconnected knob",
			onAsks, offAsks)
	}

	if got, want := strings.Join(on.HandleSequence(), ","), strings.Join(off.HandleSequence(), ","); got != want {
		t.Errorf("a period moved the handle sequence:\n with: %s\n  w/o: %s", got, want)
	}
	if got, want := strings.Join(on.KindSequence(), ","), strings.Join(off.KindSequence(), ","); got != want {
		t.Errorf("a period moved the kind sequence:\n with: %s\n  w/o: %s", got, want)
	}
	if len(on.Settled) != 0 {
		t.Errorf("the fixture held something after all, so this arm measured the "+
			"reconciliation rather than the period: %v", on.Settled)
	}
}

// TestAPeriodicReconciliationStampsItsStep is the other direction: with
// something held, the period reaches it and the record says WHERE.
//
// THE STAMP IS THE WHOLE POINT. R5/152's reason for observing at all is that "a
// walk that observed only at the end could say a run is wrong without saying
// WHEN it became wrong", so a periodic verdict with no position would reproduce
// the defect the cadence exists to prevent.
func TestAPeriodicReconciliationStampsItsStep(t *testing.T) {
	rec := settlingWalk(t, 1, engine.QuiesceQuiet, 2, true, nil)

	if len(rec.Settled) == 0 {
		t.Fatalf("a period of 2 over 8 steps settled nothing:\n%s", render(rec))
	}
	stamp := regexp.MustCompile(`^step (\d+): (.+)$`)
	for _, v := range rec.Settled {
		m := stamp.FindStringSubmatch(v)
		if m == nil {
			t.Errorf("%q does not name the step it ran at", v)
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			t.Errorf("%q: %v", v, err)
			continue
		}
		if n%2 != 0 || n < 1 || n > 8 {
			t.Errorf("%q ran at a step a period of 2 over 8 steps does not reach", v)
		}
		if m[2] == "" {
			t.Errorf("%q carries a step and no verdict", v)
		}
	}
}

// TestNoPeriodSettlesNothingAndStillReconcilesAtTheEnd is the default-off
// control, stated as an arm rather than left to the other suites: a project
// that declares no period must behave exactly as it did before this existed.
func TestNoPeriodSettlesNothingAndStillReconcilesAtTheEnd(t *testing.T) {
	rec := settlingWalk(t, 1, engine.QuiesceQuiet, 0, true, nil)

	if len(rec.Settled) != 0 {
		t.Errorf("no period was declared and %d verdict(s) were settled: %v",
			len(rec.Settled), rec.Settled)
	}
	// AND THE END-OF-RUN PASS STILL RUNS, which is the half that would catch
	// the extraction having moved the reconciliation out of the clean path:
	// `Settled` empty is also what a broken end-of-run pass looks like from
	// here, so the two are asserted together.
	if len(rec.Reconciled) == 0 {
		t.Errorf("the end-of-run reconciliation judged nothing:\n%s", render(rec))
	}
}

// TestOneSeedAndOnePeriodWalkTheSameTwice is D1 at the period's level. The
// documented invariant is per (seed, period), so this is where a new source of
// nondeterminism in the periodic pass would show up.
func TestOneSeedAndOnePeriodWalkTheSameTwice(t *testing.T) {
	first := settlingWalk(t, 1, engine.QuiesceQuiet, 2, true, nil)
	second := settlingWalk(t, 1, engine.QuiesceQuiet, 2, true, nil)

	if a, b := strings.Join(first.Settled, "|"), strings.Join(second.Settled, "|"); a != b {
		t.Errorf("two runs of seed 1 at period 2 settled differently:\n %s\n %s", a, b)
	}
	if a, b := strings.Join(first.Reconciled, "|"), strings.Join(second.Reconciled, "|"); a != b {
		t.Errorf("the end-of-run reconciliation differed:\n %s\n %s", a, b)
	}
	if len(first.Steps) != len(second.Steps) {
		t.Errorf("two runs walked %d and %d step(s)", len(first.Steps), len(second.Steps))
	}
}

// TestAMidRunQuiesceTimeoutIsNotAFindingByItself is the one place the periodic
// pass must NOT copy the end-of-run pass, and the arm exists because copying it
// is the obvious thing to do.
//
// AT THE END, the bare state `quiesce_timeout` means the run FINISHED without
// settling, which R2.3 forbids; the end-of-run block turns it into a finding on
// its own. MID-RUN it means the system has not settled YET at step n -- the
// ordinary state of a system under test, and exactly what an eventual bound
// exists to tolerate.
//
// THE FIXTURE HOLDS NOTHING, which is what isolates the STATE from a VERDICT.
// With a pending held, a re-check against a timed-out prober returns
// `quiesce_timeout` as its own verdict and legitimately ends the walk (the arm
// below) -- so a shortened walk would not tell the two apart. With nothing
// observed the periodic pass can only ever act on the bare state, and must
// not.
func TestAMidRunQuiesceTimeoutIsNotAFindingByItself(t *testing.T) {
	asks := 0
	rec := settlingWalk(t, 1, engine.QuiesceTimedOut, 2, false, &asks)

	if asks <= 1 {
		t.Fatalf("the prober was asked %d time(s), so the period never fired and this arm "+
			"would pass on a disconnected knob", asks)
	}
	if len(rec.Settled) != 0 {
		t.Fatalf("the fixture held something, so this arm cannot tell the bare STATE from a "+
			"re-check VERDICT: %v", rec.Settled)
	}
	if len(rec.Steps) != 8 {
		t.Errorf("the walk ran %d of 8 steps: a mid-run quiesce timeout was treated as a "+
			"finding on its own\n%s", len(rec.Steps), render(rec))
	}
	// AND THE END-OF-RUN PASS STILL CALLS IT, which is the half that keeps this
	// arm from passing on a build that stopped reading the state at all.
	if rec.Finding == nil {
		t.Errorf("the run ended on a quiesce timeout and recorded no finding:\n%s", render(rec))
	}
}

// TestAPeriodicFindingEndsTheWalkLikeAnyOther is the behavior the arm above had
// to be split away from, and it was found by that arm failing: with a pending
// held and a prober that never quiesces, the re-check at the first period it
// reaches answers `quiesce_timeout` ITSELF -- the pending's bound is quiesce and
// quiesce is not coming, so it can never resolve.
//
// THAT IS A FINDING AND IT ENDS THE RUN, which is what the end-of-run pass
// already says of its own re-checks: "a finding the reconciliation produces ends
// the run like any other". The periodic pass inherits the rule rather than
// inventing one.
func TestAPeriodicFindingEndsTheWalkLikeAnyOther(t *testing.T) {
	rec := settlingWalk(t, 1, engine.QuiesceTimedOut, 2, true, nil)

	if rec.Finding == nil {
		t.Fatalf("no finding at all, so there is nothing to attribute:\n%s", render(rec))
	}
	if len(rec.Steps) >= 8 {
		t.Errorf("the walk ran all %d steps: a stopping verdict from the periodic "+
			"reconciliation did not end it\n%s", len(rec.Steps), render(rec))
	}
	// THE STEP IT STOPPED AT IS ONE THE PERIOD REACHES. Without this the arm
	// would also pass if the walk had stopped for an unrelated reason at some
	// other step, which is the attribution the mutation lesson asks for.
	if rec.FoundAt%2 != 0 || rec.FoundAt == 0 {
		t.Errorf("the walk stopped at step %d, which a period of 2 does not reach, so the "+
			"finding is not attributable to the periodic pass\n%s", rec.FoundAt, render(rec))
	}
}
