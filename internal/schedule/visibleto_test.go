package schedule

import (
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
)

// THE DRAWN visible_to IS WHAT THE CLAIM CARRIES, which is palter's first live
// field (coop r5/58): a half-true claim falsifies a recorded field a statute
// reads, and until an action declared this one a walk produced no such field.
func TestTheDrawnVisibleToBecomesTheClaimsVisibility(t *testing.T) {
	a := bindings.Action{Name: "create_event", Args: []bindings.Arg{
		{Name: "visible_to", Source: bindings.ArgDraw, From: []any{[]any{"member", "admin"}}},
	}}
	args, err := ExecuteArgs(a, "member_1", nil, New(3), nil)
	if err != nil {
		t.Fatalf("ExecuteArgs: %v", err)
	}
	got, err := visibleTo(a, args)
	if err != nil {
		t.Fatalf("visibleTo: %v", err)
	}
	if strings.Join(got, ",") != "member,admin" {
		t.Errorf("the claim's visibility is %v, want the drawn subset [member admin]", got)
	}
}

// AN ACTION DECLARING NONE CARRIES NONE, and nil rather than an empty slice so
// the ledger's omitempty leaves the field out of a claim that has no
// visibility to state.
func TestAnActionDeclaringNoVisibleToCarriesNone(t *testing.T) {
	got, err := visibleTo(bindings.Action{Name: "create_user"}, map[string]any{})
	if err != nil {
		t.Fatalf("visibleTo: %v", err)
	}
	if got != nil {
		t.Errorf("an action declaring no visible_to produced %v", got)
	}
}

func TestVisibleToRefusals(t *testing.T) {
	declaring := func(from []any) bindings.Action {
		return bindings.Action{Name: "create_event", Args: []bindings.Arg{
			{Name: "visible_to", Source: bindings.ArgDraw, From: from},
		}}
	}
	for _, c := range []struct {
		label string
		a     bindings.Action
		args  map[string]any
		says  string
	}{
		// AN EMPTY LIST IS NOT AN ABSENT ONE, and the ledger cannot tell them
		// apart: omitempty drops an empty visible_to, so the claim would reach
		// the statute looking exactly like one that declares no visibility.
		{"an empty drawn subset", declaring([]any{[]any{}}),
			map[string]any{"visible_to": []any{}}, "different statements to a statute"},
		// A ROLE NAME WHERE A ROLE SUBSET BELONGS is the plausible mistake: it
		// would reach the statute as the wrong shape and fail inside the
		// policy with nothing naming the cause.
		{"a bare string", declaring([]any{"member"}),
			map[string]any{"visible_to": "member"}, "a list of roles"},
		{"a list holding a number", declaring([]any{[]any{int64(1)}}),
			map[string]any{"visible_to": []any{int64(1)}}, "a role is a string"},
		{"declared and not composed", declaring([]any{[]any{"member"}}),
			map[string]any{}, "no visibility at all"},
	} {
		got, err := visibleTo(c.a, c.args)
		if err == nil {
			t.Errorf("%s: accepted, giving %v", c.label, got)
			continue
		}
		if !strings.Contains(err.Error(), c.says) {
			t.Errorf("%s: refusal must say %q; got %v", c.label, c.says, err)
		}
	}
}
