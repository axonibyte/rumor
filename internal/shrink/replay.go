// Package shrink reduces a failing trace to a shorter one with the same
// finding. This file is the gate that runs BEFORE any reduction.
//
// WHY A GATE AT ALL, in design.md's own words: "When a run fails, the engine
// first replays the whole failure once on a verified clean rollback. If it does
// not reproduce, the seed is marked unstable and is not shrunk." A shrinker
// that skipped it would spend its whole budget bisecting a failure that was
// never a property of the trace, and would then report a "minimal" trace for a
// finding the trace does not cause.
//
// THIS IS THE ROLLBACK TRAIT'S FIRST CONSUMER. Measured 2026-10-21 on this
// tree: nothing outside internal/rollback called Checkpoint, Restore, Verify or
// Discard -- cmd/rumor read Registered() for a count and that was all. So the
// trait was built and unreached, and the shapes below are the first use of it
// rather than a second.
package shrink

import (
	"context"
	"errors"
	"fmt"

	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
	"forge.axonibyte.com/axonibyte/rumor/internal/rollback"
	"forge.axonibyte.com/axonibyte/rumor/internal/schedule"
)

// Candidate is one replayed trace and what is known about the run that
// produced it.
//
// IT EXISTS BECAUSE A FINDING ALONE CANNOT SAY WHETHER THE RUN SETTLED. R6.4's
// first done-when is that every candidate quiesces BEFORE it is judged, and the
// old replay signature returned the finding and the steps and nothing else --
// so a candidate whose walk ended with a disagreement still held open as
// `pending` was compared against the original as though it had finished. That
// is not a reduction that reproduces; it is a reduction nobody waited for.
//
// PendingOpen IS THE WALK'S OWN FIGURE, schedule.Record.PendingOpen, which
// R2.3 requires to be zero at the end of a run and which 91 made readable from
// the product. Zero is "it settled"; anything else is a candidate still
// waiting, and the judge refuses rather than reading it as a verdict.
type Candidate struct {
	Finding engine.Finding
	Steps   []schedule.Step

	// PendingOpen is how many findings the candidate's run still held open
	// when it ended. Zero is the only value a judgment may rest on.
	PendingOpen int
}

// Verdict is what the one replay establishes.
type Verdict struct {
	// Reproduced says the failure came back, so the trace may be shrunk.
	Reproduced bool

	// Got is the finding the replay produced, kept whether or not it matched:
	// a reader of an unstable seed wants to see what it produced INSTEAD, and
	// "did not reproduce" alone sends them to re-run it by hand.
	Got engine.Finding

	// Why names, in one sentence, what separated Got from the original. Empty
	// when they matched.
	Why string

	// Attempts and Recurred are how many times the candidate was probed and
	// how many of those gave the verdict.
	//
	// THEY ARE CARRIED SO "UNSTABLE" CAN BE TOLD FROM "SOMETIMES", which is
	// the distinction a one-probe gate could not express: a reader told only
	// that a seed is unstable cannot tell a trace that never causes the
	// finding from one that causes it two times in five, and those are acted
	// on differently. Both are 1 for a deterministic want, which is every
	// want that does not implement Probabilistic.
	Attempts int
	Recurred int

	// PendingOpen is what the replay's own run left open. It is carried so a
	// reader of the verdict can see that the comparison rested on a settled
	// run, and so Shrink can say so in the product's output rather than assert
	// it (fable, coop r6/153 ruling 1).
	PendingOpen int

	// AttributionCompared says the third part of the vocabulary's comparison
	// actually happened. It is false when neither finding carries the
	// attribution rule -- a project with no planted claims need not define one
	// -- and a reader should know that "the same finding" was then established
	// on rule and value alone rather than assume all three agreed.
	AttributionCompared bool
}

// attributionRule is the attribution the vocabulary names: "the attribution
// beside a finding (`rests_on_planted`)". It is a rule the project's statute
// defines, so it reaches a Finding through Rules when the caller asks for it.
const attributionRule = "rests_on_planted"

// kindName names a finding's kind for a reader, including its absence. An empty
// kind is the ordinary case -- a finding the statute produced on its own -- and
// printing it as nothing would leave the sentence with a hole where the reader
// needs the contrast.
func kindName(k string) string {
	if k == "" {
		return "(none)"
	}
	return k
}

// ErrDirty is returned when the world is provably not the checkpoint, so no
// replay run in it could be attributed. It wraps rollback.ErrDirty so a caller
// can tell this refusal from a could-not-look with errors.Is.
var ErrDirty = fmt.Errorf("shrink: refusing to replay: %w", rollback.ErrDirty)

// Once replays the whole failure one time on a verified clean world.
//
// THE ORDER IS RESTORE, VERIFY, THEN RUN, and the middle step is not optional
// (docs/design.md: "The verify step is not optional: a shrink step run on a
// world the previous step dirtied would attribute to the trace what the
// leftover state did"). Restore's own contract says it does not promise
// cleanliness -- "a caller that skipped it would be assuming what it came to
// check" -- so this asks.
//
// THE THREE ANSWERS TO Verify ARE KEPT APART, because the trait went to the
// trouble of distinguishing them: nil is proven clean, rollback.ErrDirty is
// proven dirty and REFUSES, and any other error is "the provider could not
// tell", which is a could-not-run about the provider and never a clean world.
// Collapsing the last two into one is the failure the sentinel exists to
// prevent -- it would read an unanswerable question as a passing one.
//
// WHAT "REPRODUCED" MEANS IS THE VOCABULARY'S, AND IT HAS FOUR PARTS: rule,
// value, ATTRIBUTION and KIND. The CHANGELOG fixed the first three at 0.1.17
// with the reason --
// "'the same finding' is rule, value and attribution agreeing, so a shrinker
// cannot drift from a `bug` resting on a planted claim to one resting on none"
// -- and the kind joined them at R6.1, fable ruling it in at coop r6/15 so a
// `stale` and a plain `bug` are not traded for one another; the paragraph
// docs/design.md heads on what the same finding means carries that warrant
// rather than assuming it, and the clause below compares the kind.
//
// THIS SENTENCE SAID THREE UNTIL ratchet SWEPT IT at coop r6/185, which is the
// drift that document warns about: a comment left at three while the code
// compared four reads as agreement with the code instead of as a claim about
// it. My first version of this gate compared two of the four, with a
// comment claiming attribution "compares TRACES rather than one finding". That
// was wrong on its own terms: the attribution the vocabulary names sits BESIDE
// A FINDING (`rests_on_planted`) and reaches this function through Rules, so it
// is comparable here and the hazard it guards is precisely a one-finding drift.
//
// A FINDING THAT DOES NOT CARRY THE RULE IS NOT SILENTLY AGREED WITH: when
// neither side has it the verdict says AttributionCompared is false, because a
// project with no planted claims need not define the rule and a reader must be
// able to tell "all three agreed" from "two were available".
func Once(ctx context.Context, pr rollback.Provider, cp rollback.Checkpoint,
	want Want, replay func(context.Context) (Candidate, error)) (Verdict, error) {

	if want == nil {
		return Verdict{}, fmt.Errorf("shrink: no verdict to keep, so no candidate could be " +
			"judged: a reduction with nothing to reproduce would accept every candidate")
	}

	if pr == nil {
		return Verdict{}, fmt.Errorf("shrink: no rollback provider: a trace cannot be replayed on " +
			"a world nothing can put back, and shrinking without one would attribute every step's " +
			"finding to whatever the previous step left behind")
	}
	if replay == nil {
		return Verdict{}, fmt.Errorf("shrink: no replay function, so there is nothing to run " +
			"against the clean world")
	}
	if cp.ID == "" {
		return Verdict{}, fmt.Errorf("shrink: the checkpoint names nothing: Restore takes the " +
			"handle a provider issued, and an empty one is a caller defect rather than a dirty world")
	}

	// HOW MANY TIMES THE CANDIDATE IS PROBED, which is the want's to say.
	//
	// A VERDICT THAT RECURS ONLY SOMETIMES IS THE COMMON CASE FOR A RACE, and
	// one probe collapses it into Reproduced=false -> Stable=false: the
	// finding is marked unstable and never shrunk, so the clean-world check
	// over it is satisfied VACUOUSLY because the shrink never ran (rook, coop
	// r8/38). fable ruled the cure at rumor/1521 item 3: a candidate at which
	// the verdict does not recur k-of-n is REFUSED and the finding marked
	// unstable, design.md's own words, and never read as clean.
	//
	// ONE OF ONE UNLESS THE WANT ASKS OTHERWISE, so the statute's path is the
	// path it was: a deterministic verdict probed twice would double every
	// replay in a shrink for nothing, and the walk is the only caller today.
	k, n := 1, 1
	if pw, ok := want.(Probabilistic); ok {
		k, n = pw.Attempts()
		if k < 1 || n < k {
			return Verdict{}, fmt.Errorf("shrink: the verdict asks to recur %d of %d, which is "+
				"not a threshold: a candidate cannot be judged against it", k, n)
		}
	}

	var v Verdict
	recurred, reasons := 0, make([]string, 0, n)
	for attempt := 0; attempt < n; attempt++ {
		// RESTORED BEFORE EVERY PROBE, not once before the first. Each probe
		// is a replay against a world proven clean, which is the whole of
		// what makes a candidate's finding attributable to the candidate; a
		// second probe on the world the first left would attribute its
		// finding to both.
		if err := pr.Restore(ctx, cp); err != nil {
			return Verdict{}, fmt.Errorf("shrink: restoring the world to checkpoint %s: %w", cp.ID, err)
		}
		switch err := pr.Verify(ctx, cp); {
		case err == nil:
			// Proven clean. This is the only branch that may run the replay.
		case errors.Is(err, rollback.ErrDirty):
			return Verdict{}, ErrDirty
		default:
			return Verdict{}, fmt.Errorf("shrink: the provider could not tell whether the world is "+
				"checkpoint %s, which is not the same as clean: %w", cp.ID, err)
		}

		cand, err := replay(ctx)
		if err != nil {
			return Verdict{}, fmt.Errorf("shrink: replaying the failure on the clean world: %w", err)
		}

		// EVERY CANDIDATE QUIESCES BEFORE IT IS JUDGED, which is R6.4's first
		// done-when and the reason Candidate carries the figure at all. A run
		// that ended holding a disagreement open as `pending` has not finished
		// settling, so its finding is not the finding that trace produces -- it
		// is whatever the trace had reached when the walk stopped asking.
		//
		// IT REFUSES RATHER THAN REPORTING "did not reproduce", because the two
		// are different facts and a reader acts on them differently: one says
		// the trace no longer causes the finding, the other says nobody waited
		// to find out.
		if cand.PendingOpen != 0 {
			return Verdict{}, fmt.Errorf("shrink: refusing to judge a candidate that did not settle: "+
				"its run ended with %d finding(s) still held open as `pending`, so what it found is "+
				"where the walk stopped asking rather than what the trace causes", cand.PendingOpen)
		}

		// THE LAST PROBE's ANSWER IS THE ONE REPORTED, because a reader of an
		// unstable candidate wants to see what it produced and the most recent
		// is the one they can go and re-run.
		v.Got = cand.Finding
		v.PendingOpen = cand.PendingOpen
		if sv, ok := want.(interface{ Compares(Candidate) bool }); ok {
			v.AttributionCompared = sv.Compares(cand)
		}

		ok, why := want.Recurs(cand)
		if ok {
			recurred++
			if recurred >= k {
				// ENOUGH, AND THE REST ARE NOT RUN. k of n is a threshold and
				// not a quota: probing past it would spend replays to learn
				// nothing the verdict turns on.
				v.Reproduced = true
				v.Attempts, v.Recurred = attempt+1, recurred
				if n > 1 {
					v.Why = ""
				}
				return v, nil
			}
			continue
		}
		reasons = append(reasons, why)
	}

	v.Attempts, v.Recurred = n, recurred
	// THE REASON SAYS "RECURRED k OF n" AND NOT "unstable", which rook and
	// fable both asked for by name: a reader told "unstable" cannot tell a
	// trace that never causes the finding from one that causes it sometimes,
	// and those are acted on differently.
	if n > 1 {
		v.Why = fmt.Sprintf("the verdict recurred %d of %d attempt(s) and %d was wanted; last: %s",
			recurred, n, k, lastReason(reasons))
		return v, nil
	}
	v.Why = lastReason(reasons)
	return v, nil
}

// Probabilistic is a Want whose verdict does not recur every time, and says how
// many attempts it wants and how many must recur.
//
// IT IS THE WANT's TO DECLARE AND NOT THE SHRINKER's TO GUESS: whether a
// verdict is deterministic is a fact about what produced it -- a statute
// evaluating rules against a sealed ledger gives the same answer every time,
// and a race does not -- so the author says so and a want that does not
// implement this is probed once, exactly as before.
type Probabilistic interface {
	// Attempts is how many recurrences are wanted (k) out of how many probes
	// (n). A candidate recurring fewer than k times in n is refused and the
	// finding marked unstable, never read as clean.
	Attempts() (k, n int)
}

// lastReason is the most recent probe's reason, or a stand-in when every probe
// recurred and none was recorded.
func lastReason(reasons []string) string {
	if len(reasons) == 0 {
		return "no probe reported a reason"
	}
	return reasons[len(reasons)-1]
}

func defined(ok bool) string {
	if ok {
		return "found a finding"
	}
	return "found none"
}

func had(ok bool) string {
	if ok {
		return "carried"
	}
	return "carried no"
}
