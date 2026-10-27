package shrink

import (
	"context"

	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
	"forge.axonibyte.com/axonibyte/rumor/internal/schedule"
)

// settled, settledN and settledK lift a finding-returning replay to the
// Candidate shape, declaring that the run behind it SETTLED.
//
// WHY A LIFT AND NOT THIRTY REWRITTEN CLOSURES. R6.4 widened the replay
// signature so a judge can see whether the candidate's run ended with anything
// still held open as `pending` (Candidate.PendingOpen, R2.3's figure). Every arm
// written before that change assumed a settled run -- it had no way not to --
// and rewriting each body would have put the same literal in thirty places and
// made the diff about punctuation rather than about the judgment.
//
// THE NAME IS THE ASSERTION. A reader of `Bisect(..., settledN(needs(3)))` is
// told that this fixture's runs settle, which is what makes those arms about
// LENGTH rather than about waiting. The arms for a candidate that did NOT
// settle pass a Candidate directly and are named for it, so no fixture carries
// both meanings.
//
// AND A NIL REPLAY LIFTS TO NIL, which the suite caught within a minute of the
// lift existing: TestBisectRefusals passes `nil` to assert the "no run
// function" refusal, and a wrapper that returned a non-nil closure over it made
// that refusal unreachable and panicked inside the candidate instead. A lift
// that hides its argument's nil-ness changes what the callee can refuse.
//
// IT IS TEST-ONLY AND MUST STAY SO: production builds a Candidate from the
// walk's own record, and a lift in the product would be exactly the "assume it
// settled" the refusal exists to prevent.
func settled(f func(context.Context) (engine.Finding, error)) func(context.Context) (Candidate, error) {
	if f == nil {
		return nil
	}
	return func(c context.Context) (Candidate, error) {
		got, err := f(c)
		return Candidate{Finding: got}, err
	}
}

func settledN(f func(context.Context, int) (engine.Finding, error)) func(context.Context, int) (Candidate, error) {
	if f == nil {
		return nil
	}
	return func(c context.Context, steps int) (Candidate, error) {
		got, err := f(c, steps)
		return Candidate{Finding: got}, err
	}
}

func settledK(f func(context.Context, []string) (engine.Finding, error)) func(context.Context, []string) (Candidate, error) {
	if f == nil {
		return nil
	}
	return func(c context.Context, without []string) (Candidate, error) {
		got, err := f(c, without)
		return Candidate{Finding: got}, err
	}
}

// settledReplay is the same lift for a whole Request.Replay, whose shape
// carries the step count and the dropped kinds as well.
//
// THE OLD THREE-VALUE FORM IS WHAT IT LIFTS FROM: (finding, steps, error).
// Every fixture written against it had no settling to report, so the lift says
// the run settled -- the one thing those arms were always assuming and could
// not state.
func settledReplay(f func(context.Context, int, []string) (engine.Finding, []schedule.Step, error)) func(context.Context, int, []string) (Candidate, error) {
	if f == nil {
		return nil
	}
	return func(c context.Context, steps int, without []string) (Candidate, error) {
		got, ran, err := f(c, steps, without)
		return Candidate{Finding: got, Steps: ran}, err
	}
}
