package shrink

import (
	"context"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
	"forge.axonibyte.com/axonibyte/rumor/internal/schedule"
)

// ROOK'S OBSERVATION AT coop r5/221, handed over as an arm rather than described:
// the restore fix added a branch -- "the surviving shape did not reproduce on its
// final clean replay though the reduction kept it" -- and NOTHING drove it. Measured
// with rook's own positive control: zero *_test.go hits for "final clean", against
// one for "restore(s) for" and four for "Reproduced", so the absence is a
// measurement and not a failed search.
//
// AND I READ PAST IT. My r5/215 cleared this commit's substance and praised that
// branch for making the promoted trace observed rather than assumed -- without
// asking whether the new oracle had an oracle. That is "self-test the oracle"
// applied to everyone's code today except the one I was reviewing.
//
// THE CALL NUMBER IS LEARNED, NOT HARD-CODED. The final replay is the last probe,
// and which number that is depends on the reduction's own arithmetic; a literal 10
// here would rot the moment bisection or kind removal changes its candidate set. So
// the arm runs the reduction once to LEARN the count, then re-runs an identical
// world with the finding substituted on exactly that call.
func TestAFinalReplayThatDoesNotReproduceIsAnError(t *testing.T) {
	newWorld := func() *world {
		return &world{needSteps: 4, needKinds: []string{"create_event"}, all: steps(12)}
	}
	kinds := []string{"create_event", "join_event", "leave_event"}

	// 1. LEARN the probe count from a clean run, which also proves the fixture
	//    reproduces at all -- without this the arm below could "pass" because the
	//    reduction never got as far as a final replay.
	base, err := Shrink(context.Background(), Request{
		Seed: 42, Steps: steps(12), Kinds: kinds, Want: wantBug(),
		Provider: &fake{}, Checkpoint: cp, Replay: settledReplay(newWorld().replay),
	})
	if err != nil {
		t.Fatalf("the clean run must succeed or this arm proves nothing: %v", err)
	}
	if !base.Stable || base.Probes < 2 {
		t.Fatalf("clean run: stable=%v probes=%d -- need a reduction that reaches a "+
			"final replay", base.Stable, base.Probes)
	}

	// 2. RE-RUN, substituting a DIFFERENT finding on the last probe only.
	w := newWorld()
	last := base.Probes
	var seen int
	flip := func(c context.Context, n int, without []string) (engine.Finding, []schedule.Step, error) {
		f, ran, rerr := w.replay(c, n, without)
		seen++
		if seen == last {
			// Defined, but a different value: the Value clause fires, so the gate
			// says NOT reproduced without any attribution being involved.
			return engine.Finding{Defined: true, Value: "clean",
				Rule: "data.rumor.statute.finding"}, ran, rerr
		}
		return f, ran, rerr
	}

	got, err := Shrink(context.Background(), Request{
		Seed: 42, Steps: steps(12), Kinds: kinds, Want: wantBug(),
		Provider: &fake{}, Checkpoint: cp, Replay: settledReplay(flip),
	})
	if err == nil {
		t.Fatal("a final replay that did not reproduce was reported as a shrink, so the " +
			"promoted trace would be one the reduction chose and the final run denied")
	}
	if !strings.Contains(err.Error(), "final clean replay") {
		t.Errorf("the error must name the branch it came from; got %v", err)
	}
	// AND THE RESULT STILL CARRIES WHAT IT LEARNED, because an error that threw the
	// probe count away would make the contradiction unreportable.
	if !got.Stable {
		t.Error("Stable is false though the GATE reproduced: the contradiction is at the " +
			"final replay, not at the gate, and conflating them loses which one disagreed")
	}
	if got.Probes != last {
		t.Errorf("Probes %d, want %d -- the count the error was raised on", got.Probes, last)
	}
	// THE CONTROL: substituting on a call that is NOT the last must NOT reach this
	// branch, so the arm is about the FINAL replay and not about any disagreement.
	w2 := newWorld()
	var seen2 int
	early := func(c context.Context, n int, without []string) (engine.Finding, []schedule.Step, error) {
		f, ran, rerr := w2.replay(c, n, without)
		seen2++
		if seen2 == 1 {
			return engine.Finding{Defined: true, Value: "clean",
				Rule: "data.rumor.statute.finding"}, ran, rerr
		}
		return f, ran, rerr
	}
	ctl, cerr := Shrink(context.Background(), Request{
		Seed: 42, Steps: steps(12), Kinds: kinds, Want: wantBug(),
		Provider: &fake{}, Checkpoint: cp, Replay: settledReplay(early),
	})
	if cerr != nil && strings.Contains(cerr.Error(), "final clean replay") {
		t.Errorf("a disagreement at the GATE reached the final-replay branch: %v", cerr)
	}
	if ctl.Stable {
		t.Error("CONTROL: the gate's own disagreement must mark the seed UNSTABLE, which " +
			"is design.md's rule and a different outcome from this arm's")
	}
}
