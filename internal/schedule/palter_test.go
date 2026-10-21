package schedule

import (
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
)

// visibleToArg builds an action declaring a drawn visible_to over the subsets
// given, which is the shape examples/toy declares.
func visibleToArg(subsets ...[]string) bindings.Action {
	var from []any
	for _, s := range subsets {
		var list []any
		for _, role := range s {
			list = append(list, role)
		}
		from = append(from, list)
	}
	return bindings.Action{
		Name:  "create_event",
		Emits: []string{"event.created"},
		Args: []bindings.Arg{
			{Name: "visible_to", Source: bindings.ArgDraw, From: from},
		},
	}
}

// PALTER NEEDS A FALSIFIABLE FIELD, which is design.md's own clause: the field
// "is one a statute READS -- visible_to where the action's claim carries it,
// else expect -- and a kind whose action's claim carries neither is
// INAPPLICABLE for that action".
func TestPalterableNeedsAFieldAndAnAlternative(t *testing.T) {
	for _, c := range []struct {
		label string
		a     bindings.Action
		want  bool
	}{
		{"three subsets, as the toy declares",
			visibleToArg([]string{"admin"}, []string{"admin", "member"},
				[]string{"admin", "member", "guest"}), true},
		{"two subsets is enough", visibleToArg([]string{"admin"}, []string{"guest"}), true},

		// A DRAW OF ONE VALUE IS NOT FALSIFIABLE. "Exactly one recorded field
		// false" needs a false value to exist, and the only values this engine
		// may record are ones the PROJECT declared -- so with one subset there
		// is nothing false to say, and the kind is inapplicable rather than
		// lied about with a value nobody named.
		{"one subset leaves nothing false to record",
			visibleToArg([]string{"admin"}), false},

		{"no visible_to at all", bindings.Action{Name: "create_user",
			Emits: []string{"user.created"}}, false},

		// A LITERAL IS NOT FALSIFIABLE EITHER, for the same reason: the engine
		// would have to invent the false value rather than draw it from what
		// the project declared.
		{"visible_to as a literal", bindings.Action{Name: "create_event",
			Emits: []string{"event.created"},
			Args: []bindings.Arg{{Name: "visible_to", Source: bindings.ArgLiteral,
				Value: []any{"admin"}}}}, false},
	} {
		if got := palterable(c.a); got != c.want {
			t.Errorf("%s: palterable = %v, want %v", c.label, got, c.want)
		}
	}
}

// THE LIE IS A VALUE THE PROJECT DECLARED, and it is never the true one. A
// falsehood drawn from the action's own list is something the system could
// plausibly have done, which is what makes it a lie rather than a corruption --
// an invented role would be caught as malformed instead of as a visibility
// defect.
func TestFalsifyVisibleToDrawsADifferentDeclaredSubset(t *testing.T) {
	a := visibleToArg([]string{"admin"}, []string{"admin", "member"},
		[]string{"admin", "member", "guest"})
	truth := []string{"admin", "member"}

	declared := map[string]bool{
		"admin": true, "admin,member": true, "admin,member,guest": true,
	}

	// EVERY SEED MUST PRODUCE A DECLARED SUBSET THAT IS NOT THE TRUE ONE, and
	// the loop is over seeds rather than one draw because a single draw could
	// pass by luck on an implementation that returns the truth sometimes.
	seenOther := map[string]bool{}
	for _, seed := range []uint64{1, 2, 3, 4, 5, 6, 7, 8} {
		got, err := falsifyVisibleTo(a, truth, New(seed))
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		joined := strings.Join(got, ",")
		if joined == strings.Join(truth, ",") {
			t.Errorf("seed %d recorded the TRUE subset %v, so the claim is not false at all",
				seed, got)
		}
		if !declared[joined] {
			t.Errorf("seed %d recorded %v, which the project never declared: an invented value "+
				"reads as malformed rather than as a visibility defect", seed, got)
		}
		seenOther[joined] = true
	}

	// AND BOTH ALTERNATIVES MUST BE REACHABLE, or the draw is a constant: with
	// two other subsets available, eight seeds that all chose one would mean
	// the generator is not being consulted.
	if len(seenOther) < 2 {
		t.Errorf("eight seeds produced %d distinct false subset(s) (%v) out of two available: "+
			"the falsehood is not being drawn", len(seenOther), seenOther)
	}
}

// AND WHERE NOTHING IS FALSIFIABLE IT REFUSES rather than returning the truth
// dressed as a lie -- which would record a claim marked planted whose content
// is correct, the one outcome that makes the engine's own knowledge of its lies
// untrue.
func TestFalsifyVisibleToRefusesWhenEverySubsetIsTheTruth(t *testing.T) {
	a := visibleToArg([]string{"admin"})
	_, err := falsifyVisibleTo(a, []string{"admin"}, New(1))
	if err == nil {
		t.Fatal("a single declared subset equal to the truth produced a falsehood anyway")
	}
	if !strings.Contains(err.Error(), "no false value to record") {
		t.Errorf("the refusal must say there is no false value; got %v", err)
	}

	// AN ACTION DECLARING NO DRAWN visible_to IS THE OTHER REFUSAL, and it
	// names the disagreement rather than the field: reaching here means
	// palterable and this function disagree, which is a defect in the pair and
	// not in the project.
	_, err = falsifyVisibleTo(bindings.Action{Name: "create_user"}, nil, New(1))
	if err == nil {
		t.Fatal("an action with no drawn visible_to produced a falsehood")
	}
	if !strings.Contains(err.Error(), "disagree") {
		t.Errorf("the refusal should name the disagreement; got %v", err)
	}
}
