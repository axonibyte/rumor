package schedule

import (
	"fmt"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
)

// THE RECORD CARRIES WHAT THE SEED CHOSE, which is fable's ruling at coop
// rumor/1530: `[[action]].contended.limit` names the creating action's DRAWN
// argument, and that scalar was in no field of Step -- Used carries handles and
// AdvancedTo is the one recorded draw. Without it a replay cannot read the same
// limit the walk handed the system.
//
// AND THE ORDER IS THE DECLARATION's, NOT A MAP's (rook, coop r8/57): a map
// would print byte-identically today only because fmt sorts its keys, which
// rests byte-identity on fmt's behaviour rather than on an order this tree
// claims.
func TestTheDrawnScalarsAreRecordedInDeclaredOrder(t *testing.T) {
	a := bindings.Action{
		Name: "create_event",
		Args: []bindings.Arg{
			{Name: "title", Source: bindings.ArgLiteral, Value: "Standup"},
			{Name: "capacity", Source: bindings.ArgDraw, From: []any{int64(1), int64(2), int64(3)}},
			{Name: "visible_to", Source: bindings.ArgDraw, From: []any{[]any{"admin"}}},
			{Name: "owner", Source: bindings.ArgActor},
		},
	}
	args, err := ExecuteArgs(a, "member_1", nil, New(7), nil)
	if err != nil {
		t.Fatalf("ExecuteArgs: %v", err)
	}
	got := Drawn(a, args)
	if len(got) != 2 {
		t.Fatalf("Drawn recorded %d of 2 drawn argument(s): %+v", len(got), got)
	}
	// DECLARED ORDER: capacity is declared before visible_to, and the literal
	// and the actor are not draws at all.
	if got[0].Arg != "capacity" || got[1].Arg != "visible_to" {
		t.Errorf("Drawn is %q then %q; the action declares capacity then visible_to",
			got[0].Arg, got[1].Arg)
	}
	// AND THE VALUE IS THE ONE SENT, not a second draw: it must equal what is
	// under the argument's own name in the arguments the walk handed over.
	for _, d := range got {
		// COMPARED THROUGH THE FORMATTER AND NEVER WITH ==, which panicked
		// here first: visible_to draws a []any, and Go's != on an interface
		// holding an uncomparable type is a runtime panic rather than false.
		if fmtOf(d.Value) != fmtOf(args[d.Arg]) {
			t.Errorf("%s recorded %v and sent %v: the record is a second draw and not the "+
				"number the system got", d.Arg, d.Value, args[d.Arg])
		}
	}

	// ONE SEED, ONE RECORD -- the property every replay rests on.
	again, err := ExecuteArgs(a, "member_1", nil, New(7), nil)
	if err != nil {
		t.Fatalf("ExecuteArgs: %v", err)
	}
	if fmtOf(Drawn(a, again)) != fmtOf(got) {
		t.Errorf("two walks of one seed recorded different draws:\n  %v\n  %v",
			got, Drawn(a, again))
	}
}

// AN ACTION THAT DRAWS NOTHING RECORDS NOTHING, so a project with no drawn
// argument does not gain an empty field in its record.
func TestAnActionThatDrawsNothingRecordsNoDraw(t *testing.T) {
	a := bindings.Action{
		Name: "create_user",
		Args: []bindings.Arg{{Name: "name", Source: bindings.ArgActor}},
	}
	args, err := ExecuteArgs(a, "member_1", nil, New(1), nil)
	if err != nil {
		t.Fatalf("ExecuteArgs: %v", err)
	}
	if got := Drawn(a, args); len(got) != 0 {
		t.Errorf("an action with no drawn argument recorded %+v", got)
	}
}

// AND A DIFFERENT SEED MOVES IT, which is what makes the record worth
// comparing: a field that could not differ between seeds would be a constant
// dressed as evidence -- the shape AdvancedTo was added against, where a
// chooseHandles mutant "survived the entire suite until this field existed".
func TestADifferentSeedMovesTheRecordedDraw(t *testing.T) {
	a := bindings.Action{
		Name: "create_event",
		Args: []bindings.Arg{
			{Name: "capacity", Source: bindings.ArgDraw,
				From: []any{int64(1), int64(2), int64(3), int64(4), int64(5), int64(6)}},
		},
	}
	seen := map[string]int{}
	for seed := uint64(1); seed <= 20; seed++ {
		args, err := ExecuteArgs(a, "member_1", nil, New(seed), nil)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		seen[fmtOf(Drawn(a, args))]++
	}
	if len(seen) < 2 {
		t.Fatalf("twenty seeds recorded %d distinct draw(s): a record that cannot differ between "+
			"seeds is a constant and not evidence", len(seen))
	}
	t.Logf("PROOF %d distinct recorded draw(s) over 20 seeds", len(seen))
}

// fmtOf renders a value the way the record does, so a comparison here and a
// comparison there cannot disagree about what "the same draw" means.
func fmtOf(v any) string { return fmt.Sprintf("%v", v) }
