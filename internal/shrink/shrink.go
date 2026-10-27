package shrink

import (
	"context"
	"fmt"
	"strings"

	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
	"forge.axonibyte.com/axonibyte/rumor/internal/rollback"
	"forge.axonibyte.com/axonibyte/rumor/internal/schedule"
)

// Request is one failing run offered for shrinking.
type Request struct {
	Seed  uint64
	Steps []schedule.Step // the failing trace, whole
	Kinds []string        // the action kinds it drew
	Want  engine.Finding  // the finding to keep reproducing

	Provider   rollback.Provider
	Checkpoint rollback.Checkpoint

	// SeedsFile is where a promotion goes. Empty promotes nothing, which is
	// what a caller shrinking for a report rather than for a regression suite
	// wants.
	SeedsFile string

	// Replay runs the first `steps` steps with every kind in `without`
	// dropped, and returns the finding AND the steps that actually ran.
	//
	// IT RETURNS THE STEPS BECAUSE THE TRACE MUST BE THE ONE THAT RAN. A
	// composition that assembled the result by slicing Request.Steps would be
	// reporting what it ASKED for rather than what happened -- and the two
	// differ exactly when a step stalls because its precondition no longer
	// holds in the shortened world, which is the case a shrink creates.
	// IT RETURNS A Candidate AND NOT A BARE FINDING, which is R6.4's first
	// done-when: every candidate quiesces before it is judged. The finding
	// alone cannot say whether the run that produced it had settled, so a
	// candidate still holding a `pending` would be compared as though it had
	// finished -- a reduction nobody waited for.
	Replay func(ctx context.Context, steps int, without []string) (Candidate, error)
}

// Result is what a shrink established.
type Result struct {
	// Stable says the gate reproduced the failure. When false nothing else
	// here was attempted: design.md marks the seed unstable and does not
	// shrink it.
	Stable bool

	// Why is the gate's reason when Stable is false.
	Why string

	Reduction Reduction
	Kinds     KindReduction
	Trace     Trace

	// Promoted says a seed was written to SeedsFile. False when no file was
	// named, and false when the seed was already there.
	Promoted bool

	// From is the length of the trace this reduction started with, so the
	// result can say what it reduced FROM. The figure is the caller's input and
	// is carried because a reader of "12 step(s)" cannot tell a reduction from
	// a trace that was always that long -- rook showed D11 cannot be gated from
	// an output that never says a candidate ran (fable, coop r6/153 ruling 1).
	From int

	// Probes is every candidate world rebuilt, the gate's included.
	Probes int

	// AttributionCompared is the gate Verdict's own field, carried out so a
	// reader of the RESULT can see it. False with Stable true means "the same
	// finding" was established on three of its four parts: the rule, the value
	// and the kind agreed and the attribution was never compared, because
	// neither finding carries the rule.
	//
	// IT IS HERE BECAUSE THE VERDICT'S COPY HAD NO READER. ratchet measured it
	// at coop r5/203 -- written at the gate and read by nothing in production,
	// only by two test lines -- and rook named the shape at /204: a struct
	// holding what the caller needs while nothing asks, which is cmd/rumor's
	// own R5.5 defect one layer in.
	AttributionCompared bool
}

// Render writes the result as the lines an operator reads: what the gate
// decided, and then the trace.
//
// THE PARTIAL REPRODUCTION IS TOLD, NOT REFUSED, which is fable's ruling at
// coop r5/206. A project with no planted claims need not define an attribution
// rule, so refusing a shrink there would turn away a legitimate project; but
// silence would let the operator read a two-part agreement as a three-part one.
// This is the same decision `rumor run` makes one layer up, where the finding
// line prints `rests_on_planted undeclared` rather than `false`: an unknown is
// said in that word rather than reported as a match.
//
// AND IT DOES NOT CALL THE REPRODUCTION WRONG. With no attribution on either
// side, two findings from `default finding := "bug"` differ in NO field
// engine.Finding carries -- so there was nothing to distinguish and the
// comparison is not failing to (ratchet's correction of their own reading,
// r5/203). The line reports what was compared, and the operator decides.
func (r Result) Render() string {
	var b strings.Builder
	if !r.Stable {
		// NOTHING ELSE WAS ATTEMPTED, so there is no trace to render: the gate
		// refused and design.md marks the seed unstable rather than shrinking
		// it.
		fmt.Fprintf(&b, "shrink: UNSTABLE -- %s\n", r.Why)
		return b.String()
	}
	// WHAT IT REDUCED, FROM WHAT, AT WHAT COST, AND ON WHAT -- fable's ruling 1
	// at coop r6/153. Before this line the output was the trace alone, so a
	// reader could not tell a reduction from a trace that was always that
	// length, could not see how many worlds it cost, and had nothing saying a
	// candidate had been run on a clean world at all. rook's point is that D11
	// cannot be GATED from an output that never says a candidate ran or
	// quiesced; mine is that 14 and 12 sat fifteen lines apart and the
	// subtraction was the reader's to do.
	//
	// THE CLEAN WORLD AND THE SETTLING ARE TRUE BY CONSTRUCTION AND NOT BY
	// CLAIM, which is what lets one line say them: every candidate goes through
	// Once, which restores and VERIFIES and refuses a world it cannot prove is
	// the checkpoint; and since R6.4 it refuses a candidate whose run ended
	// holding anything open as `pending`. A result that is Stable with no error
	// therefore had every candidate verified and settled -- the sentence is a
	// report of the refusals above it, and if either is ever removed this line
	// becomes false and its arm says so.
	// THE "TO" IS THE TRACE'S OWN LENGTH AND NOT THE BISECTION'S PREFIX, which
	// the first cut got wrong and the product said out loud: "REDUCED 0 step(s)
	// to 14" beside "trace: seed 1, 12 step(s)". Reduction.Steps is the
	// shortest PROBED PREFIX, measured before the kind-removal pass, so a
	// reduction that then dropped a kind reports a longer figure than the trace
	// it printed two lines down. The trace is what a reader replays, so the
	// trace's length is the number that belongs here.
	to := len(r.Trace.Steps)
	if to < r.From {
		fmt.Fprintf(&b, "shrink: REDUCED %d step(s) to %d -- %d candidate(s), each replayed on a "+
			"verified clean world and each settled with no pending left open\n",
			r.From, to, r.Probes)
	} else {
		fmt.Fprintf(&b, "shrink: NOT REDUCED -- %d step(s) is the shortest trace that "+
			"reproduces; %d candidate(s), each replayed on a verified clean world and each "+
			"settled with no pending left open\n", r.From, r.Probes)
	}
	if len(r.Kinds.Removed) > 0 {
		fmt.Fprintf(&b, "shrink: without %s\n", strings.Join(r.Kinds.Removed, ", "))
	}
	if !r.AttributionCompared {
		fmt.Fprintf(&b, "shrink: PARTIAL REPRODUCTION -- the rule, the value and the kind "+
			"agreed; the attribution was NOT compared, neither finding carrying %s. "+
			"\"The same finding\" is four parts and this one is established on three\n",
			engine.AttributionRule)
	}
	b.WriteString(r.Trace.Render())
	return b.String()
}

// Shrink runs the whole reduction in docs/design.md's order: replay the failure
// once on a verified clean world, then length bisection, then removal of whole
// action kinds, then write the trace and promote the seed.
//
// IT EXISTS BECAUSE THE PIECES WERE UNREACHED. Measured 2026-10-21 on this
// branch: nothing outside internal/shrink's own tests called Once, Bisect,
// RemoveKinds, Promote or Trace. Six built steps and no caller is the
// built-is-not-reachable shape, and a shrinker whose parts never run in
// sequence has never been shown to work as one.
//
// AN UNSTABLE SEED STOPS EVERYTHING AND IS NOT A FAILURE OF THIS FUNCTION.
// design.md: "If it does not reproduce, the seed is marked unstable and is not
// shrunk." So Stable is false, Why carries the gate's reason, and nothing is
// bisected, removed, rendered or promoted -- promoting an unstable seed would
// put a flake into the regression suite, which is the one outcome worse than
// not shrinking.
//
// THE FINAL REPLAY IS A REAL RUN AND NOT AN ASSEMBLY. After the two reductions
// the trace is built from one more replay of the surviving shape, so what gets
// reported and promoted is a trace that was observed rather than one computed
// by slicing the input.
func Shrink(ctx context.Context, req Request) (Result, error) {
	if req.Replay == nil {
		return Result{}, fmt.Errorf("shrink: no replay function, so no candidate can be run")
	}
	if len(req.Steps) == 0 {
		return Result{}, fmt.Errorf("shrink: the trace has no steps, so there is no failure to " +
			"shorten and nothing a reduction could remove")
	}

	var probes int

	// 1. THE GATE.
	v, err := Once(ctx, req.Provider, req.Checkpoint, req.Want,
		func(c context.Context) (Candidate, error) {
			return req.Replay(c, len(req.Steps), nil)
		})
	probes++
	if err != nil {
		return Result{From: len(req.Steps), Probes: probes}, err
	}
	if !v.Reproduced {
		return Result{Stable: false, Why: v.Why, From: len(req.Steps), Probes: probes,
			AttributionCompared: v.AttributionCompared}, nil
	}

	// 2. LENGTH BISECTION.
	red, err := Bisect(ctx, req.Provider, req.Checkpoint, req.Want, len(req.Steps),
		func(c context.Context, steps int) (Candidate, error) {
			return req.Replay(c, steps, nil)
		})
	probes += red.Probes
	if err != nil {
		return Result{Stable: true, Reduction: red, From: len(req.Steps), Probes: probes,
			AttributionCompared: v.AttributionCompared}, err
	}

	// 3. KIND REMOVAL, over the SHORTENED trace: the cheap pass went first, so
	// this one has fewer candidates to rebuild a world for.
	kr, err := RemoveKinds(ctx, req.Provider, req.Checkpoint, req.Want, req.Kinds,
		func(c context.Context, without []string) (Candidate, error) {
			return req.Replay(c, red.Steps, without)
		})
	probes += kr.Probes
	if err != nil {
		return Result{Stable: true, Reduction: red, Kinds: kr, From: len(req.Steps), Probes: probes,
			AttributionCompared: v.AttributionCompared}, err
	}

	// 4. THE TRACE THAT RAN, ON A WORLD PUT BACK FIRST.
	//
	// IT GOES THROUGH Once FOR THE RESTORE, which is the defect this fixed:
	// step 4 called req.Replay directly, so the trace that gets REPORTED and
	// PROMOTED was the only candidate in the whole reduction not run on a
	// verified clean world -- it ran on whatever the last kind-removal
	// candidate left behind. design.md is explicit that "the verify step is not
	// optional: a shrink step run on a world the previous step dirtied would
	// attribute to the trace what the leftover state did".
	//
	// WHY NO ARM HERE SAW IT. This package's own world fixture accepts every
	// request, so a dirty world and a clean one produce the same answer and the
	// missing restore is invisible. It surfaced the first time the reduction
	// ran against a real application: cmd/toy answers 409 "that name is taken"
	// when a create_user repeats, so the final replay failed at step 1 while
	// the gate and both reductions had passed. A fake wider than the real thing
	// passes forever.
	//
	// AND Once IS THE RIGHT CALLER, not a second restore written here: it is
	// the only place that restores and VERIFIES, and routing through it also
	// confirms the surviving shape still reproduces -- so the trace reported is
	// one observed to carry the finding rather than one assumed to.
	var steps []schedule.Step
	fv, err := Once(ctx, req.Provider, req.Checkpoint, req.Want,
		func(c context.Context) (Candidate, error) {
			cand, rerr := req.Replay(c, red.Steps, kr.Removed)
			steps = cand.Steps
			return cand, rerr
		})
	probes++
	if err == nil && !fv.Reproduced {
		// THE REDUCTIONS SAID THIS SHAPE REPRODUCES AND NOW IT DOES NOT, which
		// is a contradiction and not a shrink: reporting the trace anyway would
		// promote a seed whose own final run disagreed with the reduction that
		// chose it.
		return Result{Stable: true, Reduction: red, Kinds: kr, From: len(req.Steps), Probes: probes,
				AttributionCompared: v.AttributionCompared},
			fmt.Errorf("shrink: the surviving shape did not reproduce on its final clean "+
				"replay though the reduction kept it: %s", fv.Why)
	}
	finding := fv.Got
	if err != nil {
		return Result{Stable: true, Reduction: red, Kinds: kr, From: len(req.Steps), Probes: probes,
				AttributionCompared: v.AttributionCompared},
			fmt.Errorf("shrink: the final replay of the shortened trace: %w", err)
	}
	out := Result{
		Stable:    true,
		Reduction: red,
		Kinds:     kr,
		Trace:     Trace{Seed: req.Seed, Steps: steps, Finding: finding},
		From:      len(req.Steps),
		Probes:    probes,
		// THE GATE'S, carried so Render can say whether "the same finding"
		// was established on four parts or three.
		AttributionCompared: v.AttributionCompared,
	}

	// 5. PROMOTION, last, because a seed is worth promoting only once the
	// trace behind it has been established.
	if req.SeedsFile != "" {
		added, perr := Promote(req.SeedsFile, req.Seed, finding.Says())
		if perr != nil {
			return out, fmt.Errorf("shrink: promoting the shrunk seed: %w", perr)
		}
		out.Promoted = added
	}
	return out, nil
}
