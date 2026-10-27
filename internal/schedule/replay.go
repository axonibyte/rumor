package schedule

import (
	"context"
	"fmt"

	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
	"forge.axonibyte.com/axonibyte/rumor/internal/platform"
	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

// Replay runs a seeded attempt again to `steps` steps with every kind in
// `without` dropped, and answers the finding it reached and THE STEPS THAT
// ACTUALLY RAN.
//
// R5.5's ruling (3), fable at coop r5/152: "Shrink's real Replay is a seeded run
// to a step count evaluated the same way; the fake stays in the unit tests
// beside it."
//
// IT LIVES HERE AND NOT IN internal/shrink, AND THAT IS A SEQUENCING FACT
// RATHER THAN A PREFERENCE. `internal/shrink` is not on this branch at all:
// R5.5 is cut off v0.1.27 and the shrinker lands with R5.3's third, which
// fable sequenced AFTER R5.5 (r5/152, v0.1.28 before the toy third). So the
// signature shrink.Request.Replay declares cannot be satisfied from here --
// the type does not exist yet -- while the CAPABILITY it needs can be, and
// without importing anything. R5.3's third then hands this to Shrink in one
// line. Either reading of the ruling ends with this function existing; only
// where the wiring sits differs, so it is built and the fact is named rather
// than waiting on a round trip.
//
// THE STEPS COME BACK BECAUSE THE TRACE MUST BE THE ONE THAT RAN, which is
// shrink.Request.Replay's own stated reason: a caller that assembled the result
// by slicing the original trace would report what it ASKED for rather than what
// happened, and the two differ exactly when a step stalls because its
// precondition no longer holds in the shortened world -- which is the case a
// shrink creates on purpose.
//
// DROPPING A KIND IS A ZERO WEIGHT and not a removal from the pool, because
// zero is what the project's own vocabulary has for "never chosen" (D3) and
// because R5.5's sibling ruling made an all-zero pool the no-op step rather
// than a refusal. So a step whose every candidate was dropped records as a
// no-op and the walk carries on, which is what lets a reduction remove a kind
// the middle of a trace needed without the whole replay dying.
//
// IT DOES NOT RESTORE THE WORLD, and that boundary is deliberate: the clean
// world is the Provider's and the Checkpoint's, which shrink.Once owns and
// drives with its own verify. A replay that restored as a side effect would
// make "was this world clean" unanswerable from outside it.
// AND IT CARRIES THE RUN'S SETTLING, which the first version discarded. R6.4's
// first done-when is that every candidate quiesces before it is judged, and the
// walk already computes the figure -- Record.PendingOpen, the findings still
// held open when the run ended, which R2.3 requires to be zero. Returning the
// finding and the steps alone left a judge unable to tell a trace that no
// longer causes the finding from a run nobody waited for, and those are
// different facts a reader acts on differently.
func Replay(ctx context.Context, p Project, sess *wire.Session, seed uint64, attempt, steps int,
	without []string) (*engine.Finding, []Step, int, error) {

	if steps <= 0 {
		return nil, nil, 0, fmt.Errorf("schedule: a replay of %d steps is not a replay", steps)
	}
	if len(without) > 0 {
		dropped, err := withoutKinds(p.Weights, without)
		if err != nil {
			return nil, nil, 0, err
		}
		p.Weights = dropped
	}
	rec, err := Walk(ctx, p, sess, seed, steps, attempt)
	if err != nil {
		return nil, nil, 0, err
	}
	return rec.Finding, rec.Steps, rec.PendingOpen, nil
}

// withoutKinds is the weight table with each named kind set to ZERO at every
// level it appears.
//
// EVERY LEVEL, because internal/platform resolves a weight from profile, then
// stage, then default: zeroing only the default would leave a stage override
// re-arming a kind the caller asked to drop, and the walk would draw what the
// reduction was testing the absence of.
//
// A NAME NOTHING WEIGHTS IS REFUSED rather than ignored. A reduction asking to
// drop a kind this project never had is a caller that has lost track of which
// project it is shrinking, and silently succeeding would report "removing it
// changed nothing" about a kind that was never there -- the strongest possible
// wrong answer.
func withoutKinds(w platform.Weights, without []string) (platform.Weights, error) {
	out := platform.Weights{
		Default: map[string]int{},
		Stage:   map[string]map[string]int{},
		Profile: map[string]map[string]int{},
	}
	for k, v := range w.Default {
		out.Default[k] = v
	}
	copyTable := func(in map[string]map[string]int) map[string]map[string]int {
		cp := map[string]map[string]int{}
		for name, t := range in {
			inner := map[string]int{}
			for k, v := range t {
				inner[k] = v
			}
			cp[name] = inner
		}
		return cp
	}
	out.Stage = copyTable(w.Stage)
	out.Profile = copyTable(w.Profile)

	for _, kind := range without {
		seen := false
		if _, ok := out.Default[kind]; ok {
			out.Default[kind] = 0
			seen = true
		}
		for _, t := range out.Stage {
			if _, ok := t[kind]; ok {
				t[kind] = 0
				seen = true
			}
		}
		for _, t := range out.Profile {
			if _, ok := t[kind]; ok {
				t[kind] = 0
				seen = true
			}
		}
		if !seen {
			return platform.Weights{}, fmt.Errorf("schedule: a replay was asked to drop %q and "+
				"this project weights no such kind: a reduction that silently dropped a name the "+
				"project never had would report that removing it changed nothing, about a kind "+
				"that was never there", kind)
		}
	}
	return out, nil
}
