package setup

import (
	"path/filepath"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/decl"
)

// toyBindings is the shipped toy, loaded: the arms below read a real project
// rather than a hand-built one wherever the question is about a declaration
// somebody ships.
func toyBindings(t *testing.T) bindings.Bindings {
	t.Helper()
	p := filepath.Join("..", "..", "examples", "toy", "bindings.toml")
	rep, err := bindings.Load(p, decl.TOML)
	if err != nil {
		t.Fatalf("loading %s: %v", p, err)
	}
	if !rep.OK() {
		t.Fatalf("%s is not loadable: %v", p, rep.Refusals)
	}
	return rep.Bindings
}

// A KIND NOTHING CREATES IS REFUSED BY NAME, not skipped: a short setup would
// send ids it never obtained and the 404s would be reported as findings about
// the system.
func TestAnUnprovidedKindRefusesByName(t *testing.T) {
	b := toyBindings(t)
	_, err := Order(b, []string{"spaceship"})
	if err == nil {
		t.Fatal("a kind nothing creates returned an order")
	}
	if !strings.Contains(err.Error(), "spaceship") {
		t.Errorf("the refusal does not name the kind: %v", err)
	}
	if !strings.Contains(err.Error(), "event") {
		t.Errorf("the refusal does not list the kinds that ARE declared: %v", err)
	}
	t.Logf("PROOF %v", err)
}

// A CYCLE IS REFUSED WITH ITS PATH rather than recursed into. Built from two
// actions that each need the other's handle, because the shipped toy has no
// cycle and an arm that could not construct one would be asserting the
// absence of a shape it never tried.

// A CYCLE IS REFUSED WITH ITS PATH rather than recursed into. Built from two
// actions that each need the other's handle, because the shipped toy has no
// cycle and an arm that could not construct one would be asserting the
// absence of a shape it never tried.
func TestACycleRefusesWithItsPath(t *testing.T) {
	var b bindings.Bindings
	b.Actions = []bindings.Action{
		{Name: "make_a", Emits: []string{"a.made"}, Creates: []string{"a"},
			Args: []bindings.Arg{{Name: "b", Source: "handle", Of: "b"}}},
		{Name: "make_b", Emits: []string{"b.made"}, Creates: []string{"b"},
			Args: []bindings.Arg{{Name: "a", Source: "handle", Of: "a"}}},
	}
	_, err := Order(b, []string{"a"})
	if err == nil {
		t.Fatal("a cycle returned an order")
	}
	if !strings.Contains(err.Error(), "->") {
		t.Errorf("the refusal carries no path: %v", err)
	}
	t.Logf("PROOF %v", err)
}

// NEEDS READS THE CORPUS AND NOT THE BINDINGS. A case that omits its only
// handle needs no id, so a setup driven from the bindings alone would do work
// no case asked for -- and on a project whose only handle-taking action is
// fuzzed only by `missing`, that work could not even be done.

// THE PROVIDER CHOICE IS DECLARATION ORDER AND NOT A MAP's, so the sequence a
// reader derives by hand from the file is the one the stage runs.
func TestTheProviderIsTheFirstInDeclarationOrder(t *testing.T) {
	var b bindings.Bindings
	b.Actions = []bindings.Action{
		{Name: "first_maker", Emits: []string{"x.made"}, Creates: []string{"x"}},
		{Name: "second_maker", Emits: []string{"x.made"}, Creates: []string{"x"}},
	}
	for i := 0; i < 20; i++ {
		if got := Providers(b)["x"].Name; got != "first_maker" {
			t.Fatalf("provider for x is %q on pass %d, not the first declared", got, i)
		}
	}
}

// BOTH HALVES OF THE BOOTSTRAP CONDITION ARE REQUIRED, AND THE SHIPPED TOY
// CANNOT SAY SO.
//
// rook drove the mutants at coop r8/65 against this package's own move:
// bootstrapFirst's predicate SURVIVED, and so did dropping only `!handles` --
// though its comment says "BOTH CONDITIONS ARE REQUIRED". The reason is the
// fixture. The one arm over the order runs examples/toy, where create_user is
// the only `none`-context action and it takes ZERO handle arguments, so nothing
// there is both and the condition cannot discriminate. A guard whose fixture
// cannot tell its two clauses apart is armed by nothing while looking armed.
//
// THE FIXTURE IS SYNTHETIC FOR THAT REASON, which is this file's own shape for
// two of its three other arms: a cycle and a second provider are not shapes any
// shipped project has either.
//
// WHAT EACH HALF KILLS:
//
//	!handles      seat_guest is `none`-context AND takes a `season`. Hoisting it
//	              would move it ahead of the action that MAKES the season, which
//	              is the topological order this function must keep.
//	Context=="none" make_z needs nothing and is `api`-context. Hoisting it would
//	              put it ahead of create_user -- and a session for an actor that
//	              does not exist yet is refused, which is the dependency the
//	              handle graph cannot see and the whole reason for the hoist.
func TestTheBootstrapHoistNeedsBothOfItsConditions(t *testing.T) {
	b := bindings.Bindings{Actions: []bindings.Action{
		{Name: "make_z", Context: "api", Creates: []string{"z"}},
		{Name: "create_user", Context: "none", Creates: []string{"user"},
			Args: []bindings.Arg{{Name: "name", Source: bindings.ArgActor}}},
		{Name: "open_season", Context: "api", Creates: []string{"season"}},
		{Name: "seat_guest", Context: "none", Creates: []string{"seat"},
			Args: []bindings.Arg{{Name: "season", Source: bindings.ArgHandle, Of: "season"}}},
	}}

	// THE HANDLE HALF. The kinds are asked for in an order that puts the
	// handle-taking bootstrap first in the topological result, so a hoist that
	// ignored its arguments would move it ahead of its own provider.
	order, err := Order(b, []string{"seat"})
	if err != nil {
		t.Fatalf("Order(seat): %v", err)
	}
	if got := names(order); len(got) != 2 || got[0] != "open_season" || got[1] != "seat_guest" {
		t.Errorf("the order is %v; seat_guest is `none`-context and takes a season, so hoisting "+
			"it would put it ahead of the action that makes one", got)
	}

	// THE CONTEXT HALF. `z` is asked for FIRST, so the topological result has
	// make_z before create_user and only the hoist can fix it -- a hoist
	// reading "no handle arguments" alone would hoist both and keep that order,
	// leaving the session for create_user's actor to be asked for before the
	// actor exists.
	order, err = Order(b, []string{"z", "user"})
	if err != nil {
		t.Fatalf("Order(z, user): %v", err)
	}
	if got := names(order); len(got) != 2 || got[0] != "create_user" {
		t.Errorf("the order is %v; create_user is the only action here that needs no session, "+
			"so it goes first however the kinds were asked for", got)
	}
	t.Logf("PROOF seat -> %v, (z, user) -> %v", names(mustOrder(t, b, "seat")),
		names(mustOrder(t, b, "z", "user")))
}

func mustOrder(t *testing.T, b bindings.Bindings, need ...string) []bindings.Action {
	t.Helper()
	out, err := Order(b, need)
	if err != nil {
		t.Fatalf("Order(%v): %v", need, err)
	}
	return out
}

func names(as []bindings.Action) []string {
	out := make([]string, 0, len(as))
	for _, a := range as {
		out = append(out, a.Name)
	}
	return out
}
