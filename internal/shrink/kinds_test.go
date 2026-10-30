package shrink

import (
	"context"
	"errors"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
	"forge.axonibyte.com/axonibyte/rumor/internal/rollback"
)

func clean() engine.Finding {
	return engine.Finding{Defined: true, Value: "consistent", Rule: "data.rumor.statute.finding"}
}

// needsKinds returns a run function whose failure survives only while every
// kind in `required` is still present.
func needsKinds(required ...string) func(context.Context, []string) (engine.Finding, error) {
	need := map[string]bool{}
	for _, k := range required {
		need[k] = true
	}
	return func(_ context.Context, without []string) (engine.Finding, error) {
		for _, k := range without {
			if need[k] {
				return clean(), nil
			}
		}
		return bug(), nil
	}
}

// THE KINDS THE FAILURE DOES NOT NEED ARE DROPPED, CUMULATIVELY: two removals
// in one pass, not the best single one.
func TestRemoveKindsDropsEveryKindTheFailureSurvivesWithout(t *testing.T) {
	f := &fake{}
	r, err := RemoveKinds(context.Background(), f, cp, wantBug(),
		[]string{"join_event", "create_event", "leave_event", "suborn"},
		settledK(needsKinds("create_event")))
	if err != nil {
		t.Fatalf("RemoveKinds: %v", err)
	}
	got := strings.Join(r.Removed, ",")
	if got != "join_event,leave_event,suborn" {
		t.Errorf("removed %q, want the three kinds the failure survives without, sorted", got)
	}
	if r.Probes != 4 {
		t.Errorf("%d probe(s) for four kinds, want one each", r.Probes)
	}
	// ONE VERIFIED CLEAN WORLD PER CANDIDATE, read off the provider.
	if f.restored != r.Probes || f.verified != r.Probes {
		t.Errorf("%d probes but %d restore(s) and %d verify(s)", r.Probes, f.restored, f.verified)
	}
}

// THE ORDER IS SORTED AND NOT THE CALLER'S, so a slice built from a map cannot
// make one seed shrink two ways. Two call orders of the same SET must agree.
func TestRemoveKindsIsAFunctionOfTheSetNotTheSliceOrder(t *testing.T) {
	a, err := RemoveKinds(context.Background(), &fake{}, cp, wantBug(),
		[]string{"a_kind", "b_kind", "c_kind"}, settledK(needsKinds("b_kind")))
	if err != nil {
		t.Fatalf("first order: %v", err)
	}
	b, err := RemoveKinds(context.Background(), &fake{}, cp, wantBug(),
		[]string{"c_kind", "a_kind", "b_kind"}, settledK(needsKinds("b_kind")))
	if err != nil {
		t.Fatalf("second order: %v", err)
	}
	if strings.Join(a.Removed, ",") != strings.Join(b.Removed, ",") {
		t.Errorf("two orders of one set gave %v and %v", a.Removed, b.Removed)
	}
}

// A KIND NAMED TWICE IS TRIED ONCE: the second attempt would be removing what
// is already gone, and the probe count is where that shows.
func TestRemoveKindsDeduplicates(t *testing.T) {
	r, err := RemoveKinds(context.Background(), &fake{}, cp, wantBug(),
		[]string{"a_kind", "a_kind", "b_kind"}, settledK(needsKinds("b_kind")))
	if err != nil {
		t.Fatalf("RemoveKinds: %v", err)
	}
	if r.Probes != 2 {
		t.Errorf("%d probes for two distinct kinds", r.Probes)
	}
	if strings.Join(r.Removed, ",") != "a_kind" {
		t.Errorf("removed %v", r.Removed)
	}
}

// A FAILURE THAT NEEDS EVERY KIND LOSES NONE.
func TestRemoveKindsKeepsEveryNeededKind(t *testing.T) {
	r, err := RemoveKinds(context.Background(), &fake{}, cp, wantBug(),
		[]string{"x_kind", "y_kind"}, settledK(needsKinds("x_kind", "y_kind")))
	if err != nil {
		t.Fatalf("RemoveKinds: %v", err)
	}
	if len(r.Removed) != 0 {
		t.Errorf("removed %v from a failure that needs both", r.Removed)
	}
}

// AN EMPTY TRACE THAT REPRODUCES IS REFUSED, which is the shrinker's most
// confident wrong answer if it is not. A finding that survives removing EVERY
// kind is not a property of the trace -- the world was dirty, or the statute
// answers the same way over an empty ledger -- and "shrunk to zero actions"
// would be reported with full confidence.
func TestRemoveKindsRefusesAnEmptyTraceThatReproduces(t *testing.T) {
	always := func(context.Context, []string) (engine.Finding, error) { return bug(), nil }
	_, err := RemoveKinds(context.Background(), &fake{}, cp, wantBug(),
		[]string{"only_kind"}, settledK(always))
	if err == nil {
		t.Fatal("a trace that reproduced with every kind removed was accepted")
	}
	if !strings.Contains(err.Error(), "not a property of the trace") {
		t.Errorf("the refusal must say the finding is not the trace's; got %v", err)
	}
	if !strings.Contains(err.Error(), "zero actions") {
		t.Errorf("the refusal should name what it declines to report; got %v", err)
	}
}

// AND THE GUARD MUST NOT FIRE WHERE THE TRACE STILL HAS A KIND LEFT -- the
// world where it should NOT fire, without which the arm above would pass on a
// guard that refused every full pass.
func TestRemoveKindsDoesNotRefuseWhenAKindRemains(t *testing.T) {
	r, err := RemoveKinds(context.Background(), &fake{}, cp, wantBug(),
		[]string{"a_kind", "b_kind"}, settledK(needsKinds("b_kind")))
	if err != nil {
		t.Fatalf("refused a reduction that leaves b_kind in place: %v", err)
	}
	if strings.Join(r.Removed, ",") != "a_kind" {
		t.Errorf("removed %v, want a_kind alone", r.Removed)
	}
}

// A DIRTY WORLD STOPS THE PASS and names the candidate, for bisection's reason:
// removals judged in different worlds are not comparable.
func TestRemoveKindsStopsOnADirtyWorld(t *testing.T) {
	f := &fake{verify: rollback.ErrDirty}
	_, err := RemoveKinds(context.Background(), f, cp, wantBug(),
		[]string{"a_kind", "b_kind"}, settledK(needsKinds("b_kind")))
	if err == nil {
		t.Fatal("a dirty world did not stop the pass")
	}
	if !errors.Is(err, rollback.ErrDirty) {
		t.Errorf("the error does not wrap rollback.ErrDirty: %v", err)
	}
	if !strings.Contains(err.Error(), "without a_kind") {
		t.Errorf("the error should name the candidate; got %v", err)
	}
}

func TestRemoveKindsRefusals(t *testing.T) {
	ok := needsKinds("a_kind")
	for _, c := range []struct {
		label string
		kinds []string
		fn    func(context.Context, []string) (engine.Finding, error)
		says  string
	}{
		{"no run function", []string{"a_kind"}, nil, "no run function"},
		{"an unnamed kind", []string{"a_kind", ""}, ok, "empty name"},
	} {
		_, err := RemoveKinds(context.Background(), &fake{}, cp, wantBug(), c.kinds, settledK(c.fn))
		if err == nil {
			t.Errorf("%s: accepted", c.label)
			continue
		}
		if !strings.Contains(err.Error(), c.says) {
			t.Errorf("%s: refusal must say %q; got %v", c.label, c.says, err)
		}
	}

	// NO KINDS AT ALL IS A NO-OP AND NOT A REFUSAL: a trace with nothing to
	// remove has been reduced as far as this pass can take it.
	r, err := RemoveKinds(context.Background(), &fake{}, cp, wantBug(), nil, settledK(ok))
	if err != nil {
		t.Errorf("an empty kind list is a no-op, not an error: %v", err)
	}
	if len(r.Removed) != 0 || r.Probes != 0 {
		t.Errorf("an empty kind list probed %d time(s) and removed %v", r.Probes, r.Removed)
	}
}
