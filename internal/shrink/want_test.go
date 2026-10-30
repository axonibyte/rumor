package shrink

import (
	"context"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
)

// A VERDICT A STAGE AUTHORED IS A SUBJECT A SHRINK CAN KEEP, which is the whole
// of the widening and is armed with a want that is NOT the statute's.
//
// WHY A SECOND IMPLEMENTATION AND NOT JUST THE STATUTE's: a one-implementation
// interface says nothing. Every reducer took `want engine.Finding`, so a stage
// finding had no `want` to hand the shrinker at all; this arm is what shows the
// type no longer decides who may author one. The concurrency stage's own want
// lands with its dispatch -- a want nothing calls would be the built-and-unread
// shape this package was caught by once at Verdict.AttributionCompared.
//
// THE FAKE JUDGES ON WHAT A STAGE VERDICT WOULD: a property of the candidate
// that is not the statute's finding at all. Here it is the trace still being
// long enough to hold the setup the verdict needs, which is the shape fable's
// ruling describes -- "what shrinks is the SETUP ... the fan-out is one step of
// width N, and N is not reduced".
type needsSetup struct{ atLeast int }

func (n needsSetup) Describe() string { return "a setup of at least some length" }

func (n needsSetup) Recurs(cand Candidate) (bool, string) {
	if len(cand.Steps) >= n.atLeast {
		return true, ""
	}
	return false, "the setup no longer reaches the state the verdict needs"
}

func TestAVerdictAStageAuthoredCanBeTheSubjectOfAShrink(t *testing.T) {
	// THE STATUTE's FINDING IS DELIBERATELY CONSTANT HERE: every candidate
	// answers the same `bug`, so a shrinker still reading engine.Finding would
	// see every candidate reproduce and reduce to nothing. The reduction that
	// comes back is therefore the stage want's doing and not the statute's.
	w := &world{needSteps: 1, all: steps(12)}
	got, err := Shrink(context.Background(), Request{
		Seed: 7, Steps: steps(12), Kinds: []string{"create_event"},
		Want: needsSetup{atLeast: 5}, Provider: &fake{}, Checkpoint: cp,
		Replay: func(c context.Context, n int, without []string) (Candidate, error) {
			was, ran, rerr := w.replay(c, n, without)
			if rerr != nil {
				return Candidate{}, rerr
			}
			_ = was
			return Candidate{Finding: bug(), Steps: ran}, nil
		},
	})
	if err != nil {
		t.Fatalf("Shrink with a stage-authored want: %v", err)
	}
	if !got.Stable {
		t.Fatalf("a stage-authored verdict did not clear the gate: %s", got.Why)
	}
	if got.Reduction.Steps != 5 {
		t.Errorf("the shrink reduced to %d step(s); the want needs 5, so the reduction is not "+
			"following the stage's verdict", got.Reduction.Steps)
	}
}

// AND AN UNSTABLE STAGE VERDICT IS REFUSED THE SAME WAY, with the want's own
// reason carried rather than a sentence the shrinker invented.
func TestAStageVerdictThatDoesNotRecurIsUnstableWithItsOwnReason(t *testing.T) {
	w := &world{needSteps: 1, all: steps(4)}
	got, err := Shrink(context.Background(), Request{
		Seed: 7, Steps: steps(4), Kinds: []string{"create_event"},
		// The gate replays all four steps and the want needs ten, so it
		// cannot recur at full length.
		Want: needsSetup{atLeast: 10}, Provider: &fake{}, Checkpoint: cp,
		Replay: func(c context.Context, n int, without []string) (Candidate, error) {
			_, ran, rerr := w.replay(c, n, without)
			if rerr != nil {
				return Candidate{}, rerr
			}
			return Candidate{Finding: bug(), Steps: ran}, nil
		},
	})
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	if got.Stable {
		t.Fatal("a verdict that cannot recur at full length was called stable")
	}
	if !strings.Contains(got.Why, "no longer reaches the state") {
		t.Errorf("the unstable reason is not the want's own words: %q", got.Why)
	}
}

// A NIL WANT IS REFUSED BY NAME, because a reduction with nothing to reproduce
// would accept every candidate -- the widening's own new way to be wrong.
func TestANilWantIsRefused(t *testing.T) {
	_, err := Once(context.Background(), &fake{}, cp, nil,
		settled(func(context.Context) (engine.Finding, error) { return bug(), nil }))
	if err == nil {
		t.Fatal("Once accepted a nil want, so a shrink with nothing to reproduce would accept " +
			"every candidate it ran")
	}
	if !strings.Contains(err.Error(), "no verdict to keep") {
		t.Errorf("the refusal does not say what is missing: %v", err)
	}
}
