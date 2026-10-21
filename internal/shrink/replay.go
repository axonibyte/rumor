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
)

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
// WHAT "REPRODUCED" MEANS IS THE VOCABULARY'S, NOT A STRING COMPARE: findings
// are compared by rule and value (docs/vocabulary.md fixes the four values, and
// a finding "names the rule that produced it"). Attribution over handles is the
// third part and it compares TRACES rather than one finding, so it belongs to
// the reduction steps that follow, not to this gate. Said here so the gate is
// not read as doing the whole comparison.
func Once(ctx context.Context, pr rollback.Provider, cp rollback.Checkpoint,
	want engine.Finding, replay func(context.Context) (engine.Finding, error)) (Verdict, error) {

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

	got, err := replay(ctx)
	if err != nil {
		return Verdict{}, fmt.Errorf("shrink: replaying the failure on the clean world: %w", err)
	}

	v := Verdict{Got: got}
	switch {
	case want.Defined != got.Defined:
		v.Why = fmt.Sprintf("the original %s and the replay %s", defined(want.Defined), defined(got.Defined))
	case want.Defined && want.Value != got.Value:
		v.Why = fmt.Sprintf("the original found %v and the replay found %v", want.Value, got.Value)
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
