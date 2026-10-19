package schedule

import (
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/handle"
)

func table(t *testing.T, pairs map[string]string) *handle.Table {
	t.Helper()
	tab, err := handle.New(1)
	if err != nil {
		t.Fatalf("handle.New: %v", err)
	}
	for h, id := range pairs {
		if err := tab.Bind(h, id); err != nil {
			t.Fatalf("Bind(%q,%q): %v", h, id, err)
		}
	}
	return tab
}

// THE VALUES ARE IDS, which is contract 2's reason for existing: a handle on
// the wire is a second way of carrying an id that the decoder cannot check.
func TestTheArgsCarryIdsAndNotHandles(t *testing.T) {
	tab := table(t, map[string]string{"alice.event#1": "ev-7"})
	decl := bindings.Observation{Name: "event_count", Args: []string{"event"}}
	got, err := ObserveArgs(decl, map[string]string{"event": "alice.event#1"}, tab)
	if err != nil {
		t.Fatalf("refused a resolvable handle: %v", err)
	}
	if got["event"] != "ev-7" {
		t.Errorf("args[event] = %v, want the id ev-7: a handle reaching the wire is the "+
			"breach contract 2 was raised to prevent", got["event"])
	}
}

// ABSENT, NEVER EMPTY. An observation declaring no arguments must compose to
// nil so the field is omitted; an empty object would read as "declared none"
// and "declared some and sent nothing" at once, and validate pairs both halves.
func TestNoDeclaredArgumentsComposesToAbsentAndNotEmpty(t *testing.T) {
	got, err := ObserveArgs(bindings.Observation{Name: "user_count"}, nil, nil)
	if err != nil {
		t.Fatalf("an observation with no arguments refused: %v", err)
	}
	if got != nil {
		t.Errorf("composed %#v, want nil: `args` is present exactly when the observation "+
			"declares any (contract.md:134)", got)
	}
}

// FOUR REFUSALS, each the thing a silent fallback would hide.
func TestTheRefusals(t *testing.T) {
	tab := table(t, map[string]string{"alice.event#1": "ev-7"})
	decl := bindings.Observation{Name: "event_count", Args: []string{"event"}}

	// 1. a declared argument the walk chose no handle for: a partial args would
	//    be a could-not-run the project did not cause.
	if _, err := ObserveArgs(decl, nil, tab); err == nil {
		t.Error("a missing declared argument composed a request")
	}
	// 2. a handle this attempt cannot resolve: never fall back to the handle.
	if _, err := ObserveArgs(decl, map[string]string{"event": "alice.event#9"}, tab); err == nil {
		t.Error("an unresolvable handle composed a request")
	}
	// 3. an argument the declaration does not name, which the decoder would
	//    accept and nothing would check.
	if _, err := ObserveArgs(decl, map[string]string{"event": "alice.event#1", "extra": "alice.event#1"}, tab); err == nil {
		t.Error("an undeclared argument was carried rather than refused")
	}
	// 4. the same, where NOTHING is declared: the empty-declaration path must
	//    refuse a choice too, or it becomes a silent drop.
	if _, err := ObserveArgs(bindings.Observation{Name: "user_count"}, map[string]string{"event": "alice.event#1"}, tab); err == nil {
		t.Error("a choice against a declaration with no arguments was dropped silently")
	}
	// AND THE CONTROL: declaring arguments with no table is refused rather
	// than composing a request with handles in it.
	if _, err := ObserveArgs(decl, map[string]string{"event": "alice.event#1"}, nil); err == nil {
		t.Error("a declared argument with no handle table composed a request")
	}
}
