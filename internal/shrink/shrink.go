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
	Replay func(ctx context.Context, steps int, without []string) (engine.Finding, []schedule.Step, error)
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

	// Probes is every candidate world rebuilt, the gate's included.
	Probes int

	// AttributionCompared is the gate Verdict's own field, carried out so a
	// reader of the RESULT can see it. False with Stable true means "the same
	// finding" was established on two of its three parts: the rule and the
	// value agreed and the attribution was never compared, because neither
	// finding carries the rule.
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
	if !r.AttributionCompared {
		fmt.Fprintf(&b, "shrink: PARTIAL REPRODUCTION -- the rule and the value agreed; the "+
			"attribution was NOT compared, neither finding carrying %s. \"The same finding\" "+
			"is three parts and this one is established on two\n", engine.AttributionRule)
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
		func(c context.Context) (engine.Finding, error) {
			f, _, rerr := req.Replay(c, len(req.Steps), nil)
			return f, rerr
		})
	probes++
	if err != nil {
		return Result{Probes: probes}, err
	}
	if !v.Reproduced {
		return Result{Stable: false, Why: v.Why, Probes: probes,
			AttributionCompared: v.AttributionCompared}, nil
	}

	// 2. LENGTH BISECTION.
	red, err := Bisect(ctx, req.Provider, req.Checkpoint, req.Want, len(req.Steps),
		func(c context.Context, steps int) (engine.Finding, error) {
			f, _, rerr := req.Replay(c, steps, nil)
			return f, rerr
		})
	probes += red.Probes
	if err != nil {
		return Result{Stable: true, Reduction: red, Probes: probes,
			AttributionCompared: v.AttributionCompared}, err
	}

	// 3. KIND REMOVAL, over the SHORTENED trace: the cheap pass went first, so
	// this one has fewer candidates to rebuild a world for.
	kr, err := RemoveKinds(ctx, req.Provider, req.Checkpoint, req.Want, req.Kinds,
		func(c context.Context, without []string) (engine.Finding, error) {
			f, _, rerr := req.Replay(c, red.Steps, without)
			return f, rerr
		})
	probes += kr.Probes
	if err != nil {
		return Result{Stable: true, Reduction: red, Kinds: kr, Probes: probes,
			AttributionCompared: v.AttributionCompared}, err
	}

	// 4. THE TRACE THAT RAN.
	finding, steps, err := req.Replay(ctx, red.Steps, kr.Removed)
	probes++
	if err != nil {
		return Result{Stable: true, Reduction: red, Kinds: kr, Probes: probes,
				AttributionCompared: v.AttributionCompared},
			fmt.Errorf("shrink: the final replay of the shortened trace: %w", err)
	}
	out := Result{
		Stable:    true,
		Reduction: red,
		Kinds:     kr,
		Trace:     Trace{Seed: req.Seed, Steps: steps, Finding: finding},
		Probes:    probes,
		// THE GATE'S, carried so Render can say whether "the same finding"
		// was established on three parts or two.
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
