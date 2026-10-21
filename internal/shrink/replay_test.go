package shrink

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
	"forge.axonibyte.com/axonibyte/rumor/internal/rollback"
)

// fake is a provider whose three Verify answers are selectable, because the
// trait distinguishes them and a test that could only produce one could not
// show the gate keeps them apart.
type fake struct {
	verify   error // nil, rollback.ErrDirty, or anything else
	restore  error
	restored int
	verified int
	order    []string
}

func (f *fake) Describe() rollback.Description {
	return rollback.Description{Provider: "fake", Covers: []string{"store"}, How: "a test double"}
}
func (f *fake) Checkpoint(context.Context) (rollback.Checkpoint, error) {
	return rollback.Checkpoint{ID: "cp-1"}, nil
}
func (f *fake) Restore(_ context.Context, _ rollback.Checkpoint) error {
	f.restored++
	f.order = append(f.order, "restore")
	return f.restore
}
func (f *fake) Verify(_ context.Context, _ rollback.Checkpoint) error {
	f.verified++
	f.order = append(f.order, "verify")
	return f.verify
}
func (f *fake) Discard(context.Context, rollback.Checkpoint) error { return nil }

var cp = rollback.Checkpoint{ID: "cp-1"}

func bug() engine.Finding {
	return engine.Finding{Defined: true, Value: "bug", Rule: "data.rumor.statute.finding"}
}

// A REPLAY THAT REPRODUCES CLEARS THE GATE, and the ORDER is asserted: restore
// then verify then run. A gate that ran the replay before verifying would pass
// this arm on the value alone, which is why the order is read off the provider
// rather than assumed from the source.
func TestAReproducedFailureClearsTheGate(t *testing.T) {
	f := &fake{}
	ran := false
	v, err := Once(context.Background(), f, cp, bug(), func(context.Context) (engine.Finding, error) {
		ran = true
		f.order = append(f.order, "replay")
		return bug(), nil
	})
	if err != nil {
		t.Fatalf("refused a clean world: %v", err)
	}
	if !v.Reproduced {
		t.Errorf("not reproduced, and the reason given was %q", v.Why)
	}
	if !ran {
		t.Error("the replay never ran")
	}
	if strings.Join(f.order, ",") != "restore,verify,replay" {
		t.Errorf("order was %v, want restore then verify then the replay", f.order)
	}
}

// A WORLD PROVEN DIRTY REFUSES, AND THE REPLAY MUST NOT RUN. design.md: a step
// run on a dirtied world "would attribute to the trace what the leftover state
// did". The arm therefore checks the replay was never called, not only that an
// error came back -- a gate that ran it and then returned an error would have
// already spent the world.
func TestAProvenDirtyWorldRefusesBeforeRunning(t *testing.T) {
	f := &fake{verify: rollback.ErrDirty}
	ran := false
	_, err := Once(context.Background(), f, cp, bug(), func(context.Context) (engine.Finding, error) {
		ran = true
		return bug(), nil
	})
	if err == nil {
		t.Fatal("a dirty world was accepted")
	}
	if !errors.Is(err, rollback.ErrDirty) {
		t.Errorf("the refusal does not wrap rollback.ErrDirty, so a caller cannot tell it from a "+
			"could-not-look: %v", err)
	}
	if ran {
		t.Error("the replay ran on a world proven dirty")
	}
	if f.verified != 1 {
		t.Errorf("Verify called %d time(s)", f.verified)
	}
}

// AND "COULD NOT TELL" IS NOT "CLEAN", which is the distinction the sentinel
// exists for. This is the arm that fails if the two branches are collapsed:
// both produce an error, so an arm checking only `err != nil` would pass on a
// gate that read an unanswerable question as a dirty one -- or worse, as clean.
func TestAProviderThatCannotTellIsNotCleanAndNotDirty(t *testing.T) {
	f := &fake{verify: fmt.Errorf("the checksum file is unreadable")}
	ran := false
	_, err := Once(context.Background(), f, cp, bug(), func(context.Context) (engine.Finding, error) {
		ran = true
		return bug(), nil
	})
	if err == nil {
		t.Fatal("an unverifiable world was accepted as clean")
	}
	if errors.Is(err, rollback.ErrDirty) {
		t.Error("a could-not-look was reported as a proven dirty world: a caller refusing on dirty " +
			"and reporting could-not-run otherwise would take the wrong branch")
	}
	if !strings.Contains(err.Error(), "not the same as clean") {
		t.Errorf("the error must say an unanswerable verify is not a clean world; got %v", err)
	}
	if ran {
		t.Error("the replay ran without a verified world")
	}
}

// A FAILURE THAT DOES NOT COME BACK IS UNSTABLE, NOT AN ERROR. The seed is not
// shrunk, and that is a VERDICT the caller acts on rather than a could-not-run:
// collapsing it into an error would make an unstable seed indistinguishable
// from a broken provider.
func TestAFailureThatDoesNotReturnIsUnstable(t *testing.T) {
	for _, c := range []struct {
		label string
		got   engine.Finding
		says  string
	}{
		{"a different value", engine.Finding{Defined: true, Value: "consistent",
			Rule: "data.rumor.statute.finding"}, "found bug and the replay found consistent"},
		{"no finding at all", engine.Finding{Defined: false}, "found a finding and the replay found none"},
		// ONE VALUE REACHED BY A DIFFERENT RULE IS A DIFFERENT FINDING, because
		// a finding names the rule that produced it. Without this cell a
		// statute whose two clauses both yield "bug" would make two distinct
		// failures read as one reproduction.
		{"the same value by another rule", engine.Finding{Defined: true, Value: "bug",
			Rule: "data.rumor.statute.other"}, "by data.rumor.statute.finding and then"},
	} {
		f := &fake{}
		v, err := Once(context.Background(), f, cp, bug(), func(context.Context) (engine.Finding, error) {
			return c.got, nil
		})
		if err != nil {
			t.Errorf("%s: returned an error rather than an unstable verdict: %v", c.label, err)
			continue
		}
		if v.Reproduced {
			t.Errorf("%s: read as reproduced", c.label)
		}
		if !strings.Contains(v.Why, c.says) {
			t.Errorf("%s: Why = %q, want it to say %q", c.label, v.Why, c.says)
		}
		// THE REPLAY'S OWN FINDING IS KEPT, so a reader of an unstable seed
		// sees what it produced instead of re-running it by hand.
		if v.Got.Defined != c.got.Defined || v.Got.Value != c.got.Value {
			t.Errorf("%s: the verdict dropped the replay's finding: %+v", c.label, v.Got)
		}
	}
}

func TestOnceRefusals(t *testing.T) {
	ok := func(context.Context) (engine.Finding, error) { return bug(), nil }
	for _, c := range []struct {
		label string
		pr    rollback.Provider
		cp    rollback.Checkpoint
		fn    func(context.Context) (engine.Finding, error)
		says  string
	}{
		{"no provider", nil, cp, ok, "no rollback provider"},
		{"no replay", &fake{}, cp, nil, "nothing to run"},
		{"an empty checkpoint", &fake{}, rollback.Checkpoint{}, ok, "names nothing"},
		{"a restore that fails", &fake{restore: fmt.Errorf("the process will not die")}, cp, ok,
			"restoring the world"},
	} {
		_, err := Once(context.Background(), c.pr, c.cp, bug(), c.fn)
		if err == nil {
			t.Errorf("%s: accepted", c.label)
			continue
		}
		if !strings.Contains(err.Error(), c.says) {
			t.Errorf("%s: refusal must say %q; got %v", c.label, c.says, err)
		}
	}
}
