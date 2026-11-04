package schedule

import (
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
)

// TestFlowStepsCarriesWhAT WAS DECLARED AND NOTHING ELSE is the composer's arm,
// and the thing it pins is the ABSENCE of empty keys: a `navigate` that went out
// carrying `"until": ""` would be a step the adapter refuses by name for a field
// the project never wrote, and the refusal would send its author looking at a
// declaration that is correct.
//
// IT DOES NOT ASSERT WHICH KIND TAKES WHICH FIELD, because the composer does not
// decide that either -- internal/bindings has parsed and refused the steps
// already, and a second reader here would be the drift the wire carries them
// opaque to avoid.
func TestFlowStepsCarriesWhatWasDeclaredAndNothingElse(t *testing.T) {
	a := bindings.Action{Flow: []bindings.Step{
		{Step: bindings.StepNavigate, To: "/ui/events/1"},
		{Step: bindings.StepFill, Field: "title", Value: "x"},
		{Step: bindings.StepClick, Target: "join"},
		{Step: bindings.StepWait, Until: "count-changed"},
	}}
	got, err := flowSteps(a, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("%d step(s) composed for 4 declared: %v", len(got), got)
	}
	want := []map[string]any{
		{"step": "navigate", "to": "/ui/events/1"},
		{"step": "fill", "field": "title", "value": "x"},
		{"step": "click", "target": "join"},
		{"step": "wait", "until": "count-changed"},
	}
	for i := range want {
		if len(got[i]) != len(want[i]) {
			t.Errorf("step %d composed %v, want exactly %v: an empty key the project never "+
				"declared is a field the adapter refuses by name", i, got[i], want[i])
			continue
		}
		for k, v := range want[i] {
			if got[i][k] != v {
				t.Errorf("step %d's %s is %v, want %v", i, k, got[i][k], v)
			}
		}
	}
}

// AND A FLOW OF NO STEPS COMPOSES AN EMPTY LIST AND NOT NIL, because the wire's
// `steps` is a REQUIRED field: nil marshals to `null`, which the decoder refuses
// for a reason that is this composer's fault and reads as the project's.
func TestFlowStepsOfNothingIsAnEmptyListAndNotNil(t *testing.T) {
	got, err := flowSteps(bindings.Action{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Error("an action with no flow composed nil, which marshals to `null` where the wire " +
			"requires a list")
	}
	if len(got) != 0 {
		t.Errorf("composed %v for an action with no flow", got)
	}
}

// TestTheUITimeoutStopsTheWalk is the explicit half of a decision, and it is
// here because `stopsTheWalk`'s own comment demands it: "a value added to the
// vocabulary with no single place to add it is a value some paths stop on and
// others walk past".
//
// WHY IT STOPS: after a wait that never came true, the page the actor acts
// through is in a state the record cannot describe, so every later step in that
// context would act on a world no reader can reconstruct.
//
// THE CONTROL IS THE WORD THAT MUST NOT STOP IT, asserted beside: `pending` is
// a finding the walk carries on with, so a test that only checked the stoppers
// would pass against a function that stopped on everything.
func TestTheUITimeoutStopsTheWalkAndPendingDoesNot(t *testing.T) {
	for _, c := range []struct {
		v    any
		stop bool
	}{
		{engine.FindingUITimedOut, true},
		{engine.FindingQuiesceTimedOut, true},
		{"bug", true},
		{"pending", false},
		{"consistent", false},
		{nil, false},
		{42, false},
	} {
		if got := stopsTheWalk(c.v); got != c.stop {
			t.Errorf("stopsTheWalk(%v) = %v, want %v", c.v, got, c.stop)
		}
	}
}

// TestTheUITimeoutFindingNamesWhatAReaderNeeds, which is four things: the
// action, the actor whose page it was, where in the flow it stopped and the
// condition it waited for. A finding that named only the action would have a
// reader opening the bindings to guess which wait.
func TestTheUITimeoutFindingNamesWhatAReaderNeeds(t *testing.T) {
	f := engine.UITimedOut("join_event_ui", "member_2", 2, 3, "count-changed")
	if f.Value != engine.FindingUITimedOut || !f.Defined {
		t.Fatalf("value %v defined %v, want %q and true", f.Value, f.Defined, engine.FindingUITimedOut)
	}
	if f.Kind != "" {
		t.Errorf("the finding carries kind %q: a kind narrows a `bug` and this is not one", f.Kind)
	}
	if f.Overlay == nil {
		t.Fatal("no overlay, so `rumor explain` has no rule to name")
	}
	if f.Overlay.Rule != "engine.ui_timeout" {
		t.Errorf("the rule is %q", f.Overlay.Rule)
	}
	// THE CONDITION IS ASSERTED AS THE QUOTED PHRASE and not as a bare
	// substring, and a MUTANT is why: dropping `waiting for %q` from the format
	// while leaving `until` in the argument list SURVIVED a plain Contains,
	// because Sprintf appends `%!(EXTRA string=count-changed)` and the word was
	// in the output after all. A needle Go's own error marker can satisfy is not
	// a needle.
	for _, want := range []string{"join_event_ui", "member_2", `waiting for "count-changed"`, "step 3 of 3"} {
		if !strings.Contains(f.Overlay.Reason, want) {
			t.Errorf("the reason does not name %q: %q", want, f.Overlay.Reason)
		}
	}
	// AND NO FORMATTING FAULT AT ALL, which is general rather than about this
	// finding: a reason carrying `%!` is a Sprintf whose verbs and arguments
	// disagree, and it reaches a reader as the finding's own words.
	if strings.Contains(f.Overlay.Reason, "%!") {
		t.Errorf("the reason carries a formatting fault: %q", f.Overlay.Reason)
	}
}
