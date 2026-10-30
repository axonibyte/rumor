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
