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
// WHAT "REPRODUCED" MEANS IS THE VOCABULARY'S, AND IT HAS THREE PARTS: rule,
// value AND ATTRIBUTION. The CHANGELOG fixed that at 0.1.17 with the reason --
// "'the same finding' is rule, value and attribution agreeing, so a shrinker
// cannot drift from a `bug` resting on a planted claim to one resting on none"
// -- and my first version of this gate compared two of the three, with a
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
	want engine.Finding, replay func(context.Context) (Candidate, error)) (Verdict, error) {

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
	got := cand.Finding

	// EVERY CANDIDATE QUIESCES BEFORE IT IS JUDGED, which is R6.4's first
	// done-when and the reason Candidate carries the figure at all. A run that
	// ended holding a disagreement open as `pending` has not finished settling,
	// so its finding is not the finding that trace produces -- it is whatever
	// the trace had reached when the walk stopped asking. Comparing it would
	// make a candidate look like a reproduction or a failure to reproduce on
	// the strength of where the clock happened to be.
	//
	// IT REFUSES RATHER THAN REPORTING "did not reproduce", because the two are
	// different facts and a reader acts on them differently: one says the trace
	// no longer causes the finding, the other says nobody waited to find out.
	// R2.3's rule is that a run ends with no pending open, so a non-zero figure
	// here is a defect in the candidate's run and not a verdict about the
	// trace.
	if cand.PendingOpen != 0 {
		return Verdict{}, fmt.Errorf("shrink: refusing to judge a candidate that did not settle: "+
			"its run ended with %d finding(s) still held open as `pending`, so what it found is "+
			"where the walk stopped asking rather than what the trace causes", cand.PendingOpen)
	}

	wantAttr, wantHas := want.Rules[attributionRule]
	gotAttr, gotHas := got.Rules[attributionRule]

	v := Verdict{Got: got, PendingOpen: cand.PendingOpen, AttributionCompared: wantHas || gotHas}
	switch {
	case want.Defined != got.Defined:
		v.Why = fmt.Sprintf("the original %s and the replay %s", defined(want.Defined), defined(got.Defined))
	case want.Defined && want.Value != got.Value:
		v.Why = fmt.Sprintf("the original found %v and the replay found %v", want.Value, got.Value)
	case want.Defined && want.Kind != got.Kind:
		// A KIND JOINS THE RULE, THE VALUE AND THE ATTRIBUTION in "the same
		// finding" (fable, coop breakout-r6/0, R6.4's clause). `stale` is a
		// `bug` whose kind says WHY it is one -- a pending past its bound -- so
		// two findings that agree on the value and differ on the kind are two
		// different defects wearing one word, and a shrink that traded one for
		// the other would reduce against a failure it did not find.
		//
		// IT LANDS WITH R6.1 RATHER THAN R6.4 BECAUSE R6.1 CREATED THE GAP:
		// engine.Finding gained Kind there, and until this clause read it the
		// comparison treated a stale and a plain bug as one reproduction.
		// Leaving a field written and unread is the shape this package was
		// already caught by once, at Verdict.AttributionCompared.
		v.Why = fmt.Sprintf("both found %v, but the original is of kind %s and the replay %s",
			want.Value, kindName(want.Kind), kindName(got.Kind))
	case wantHas != gotHas:
		// THE RULE BECOMING UNDEFINED IS A DIFFERENCE, not an absence to pass
		// over: engine.Finding's Rules is documented there as carrying every
		// rule the caller named and being absent when undefined, so one side
		// having an attribution and the other not is a finding that no longer
		// rests on what it rested on.
		//
		// THE ATTRIBUTION OF THAT PHRASE IS internal/engine's AND NOT THE
		// VOCABULARY'S. My first version credited it to the vocabulary, which
		// does not say it, and tests/vocabulary's sweep refused by name: "1
		// absent". Citing the document that happens to be nearby is the same
		// defect as citing a guarantee by line number.
		v.Why = fmt.Sprintf("the original %s an attribution and the replay %s one",
			had(wantHas), had(gotHas))
	case wantHas && gotHas && wantAttr != gotAttr:
		v.Why = fmt.Sprintf("both found %v, but the original rests_on_planted %v and the replay %v",
			want.Value, wantAttr, gotAttr)
	case want.Rule != got.Rule:
		// A FINDING NAMES THE RULE THAT PRODUCED IT, so one value reached by a
		// different rule is a different finding. Without this clause a statute
		// whose clauses both yield "bug" would make two distinct failures look
		// like one reproduction.
		v.Why = fmt.Sprintf("both found %v, but by %s and then %s", want.Value, want.Rule, got.Rule)
	default:
		v.Reproduced = true
	}
	return v, nil
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
