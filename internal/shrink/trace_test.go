package shrink

import (
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
	"forge.axonibyte.com/axonibyte/rumor/internal/schedule"
)

func sampleTrace() Trace {
	return Trace{
		Seed: 42,
		Steps: []schedule.Step{
			{N: 1, Actor: "member_1", Kind: "create_event", Handles: []string{"member_1.event#1"}},
			// A GAP: step 2 was a kind this pass removed, so the positions are
			// 1 and 4 rather than 1 and 2.
			{N: 4, Actor: "member_2", Kind: "join_event", Handles: []string{"member_2.join_event#1"},
				Planted: true},
		},
		Finding: engine.Finding{Defined: true, Value: "bug", Rule: "data.rumor.statute.finding",
			Fired: []engine.Definition{{Module: "statute.rego", Row: 31}}},
	}
}

// THE SHAPE IS design.md's FOUR PARTS: the actions, their positions, their
// handles and the rules that fired.
func TestRenderCarriesTheShapeDesignFixes(t *testing.T) {
	got := sampleTrace().Render()
	for _, want := range []string{
		"seed 42",          // replayable
		"create_event",     // the action
		"  1 member_1",     // its position
		"  4 member_2",     // and the gap preserved
		"member_1.event#1", // its handles
		"finding bug",      // the verdict
		"by data.rumor.statute.finding",
		"fired statute.rego:31", // the rules that fired
		"PLANTED",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the rendered trace is missing %q:\n%s", want, got)
		}
	}
}

// POSITIONS ARE THE STEP'S OWN AND NOT RENUMBERED. A shrunk trace has gaps,
// and a renderer that printed 1 and 2 would read as contiguous and replay as
// something else.
func TestRenderDoesNotRenumberPositions(t *testing.T) {
	got := sampleTrace().Render()
	if strings.Contains(got, "  2 member_2") {
		t.Errorf("the second step was renumbered to 2, losing the gap:\n%s", got)
	}
}

// "MINIMAL" IS A FORMAT NAME AND NOT A CLAIM, so the word must not appear in
// what a reader is handed: the bisection returns a local minimum, and printing
// "minimal" as a verdict would promise what no step established.
func TestRenderDoesNotClaimMinimality(t *testing.T) {
	got := strings.ToLower(sampleTrace().Render())
	for _, forbidden := range []string{"minimal", "smallest", "shortest possible"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("the rendered trace claims %q, which no step established:\n%s", forbidden, got)
		}
	}
}

// TWO RENDERS OF ONE TRACE ARE BYTE-IDENTICAL, which is what makes a diff
// between two attempts mean something. A renderer that walked a map would fail
// this intermittently rather than never.
func TestRenderIsStable(t *testing.T) {
	a, b := sampleTrace().Render(), sampleTrace().Render()
	if a != b {
		t.Errorf("two renders differ:\n%s\n---\n%s", a, b)
	}
}

// A STALLED STEP IS KEPT AND NAMED, for the reason the walk records it: a trace
// that dropped it would let two traces agree while their draws differed.
func TestRenderNamesAStalledStep(t *testing.T) {
	tr := Trace{Seed: 1, Steps: []schedule.Step{{N: 1, Actor: "member_1"}},
		Finding: engine.Finding{Defined: true, Value: "clean"}}
	got := tr.Render()
	if !strings.Contains(got, "(nothing applicable)") {
		t.Errorf("a step with no kind is not named:\n%s", got)
	}
}

// AN UNDEFINED FINDING SAYS SO rather than printing a bare nil, and a finding
// with no recorded definitions says THAT rather than printing nothing -- an
// empty list means the rules that fired are unknown, not that none did.
func TestRenderIsExplicitAboutWhatItDoesNotKnow(t *testing.T) {
	tr := Trace{Seed: 1, Steps: []schedule.Step{{N: 1, Actor: "a_1", Kind: "k"}},
		Finding: engine.Finding{Defined: false}}
	got := tr.Render()
	if !strings.Contains(got, "no finding") {
		t.Errorf("an undefined finding is not named:\n%s", got)
	}
	if !strings.Contains(got, "(none recorded)") {
		t.Errorf("an empty Fired list must say the rules are unrecorded, not print nothing:\n%s", got)
	}
}

// THE HANDLES ARE WHAT TWO ATTEMPTS ARE COMPARED BY, in order and deduplicated.
func TestHandlesAreOrderedAndDeduplicated(t *testing.T) {
	tr := Trace{Steps: []schedule.Step{
		{N: 1, Handles: []string{"a_1.event#1", "a_1.event#2"}},
		{N: 2, Handles: []string{"a_1.event#1", "a_1.join#1"}},
	}}
	got := strings.Join(tr.Handles(), ",")
	if got != "a_1.event#1,a_1.event#2,a_1.join#1" {
		t.Errorf("Handles() = %q, want first-appearance order with the repeat dropped", got)
	}
}
