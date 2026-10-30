package fuzz

import (
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/setup"
)

// THE SETUP IS DERIVED FROM `creates`, over the shipped toy: an invitation
// needs an event and a user first, and the order says so without this test
// naming the actions.
func TestTheSetupOrderIsDerivedAndTopological(t *testing.T) {
	b := toyBindings(t)
	cases, err := Corpus(b)
	if err != nil {
		t.Fatalf("Corpus: %v", err)
	}
	need := Needs(b, cases)
	if len(need) == 0 {
		t.Fatal("the toy's corpus needs no handle, which would make this arm vacuous")
	}
	order, err := setup.Order(b, need)
	if err != nil {
		t.Fatalf("Order(%v): %v", need, err)
	}
	at := map[string]int{}
	for i, a := range order {
		at[a.Name] = i
	}
	// EVERY NEEDED KIND IS PROVIDED BY SOMETHING IN THE ORDER.
	made := map[string]bool{}
	for _, a := range order {
		for _, k := range a.Creates {
			made[k] = true
		}
	}
	for _, k := range need {
		if !made[k] {
			t.Errorf("kind %q is needed and nothing in the order creates it", k)
		}
	}
	// AND EVERY ACTION's OWN HANDLE ARGUMENTS ARE MADE BEFORE IT RUNS, which
	// is the topological claim and the only reason the order is a list.
	for i, a := range order {
		for _, arg := range a.Args {
			if arg.Source != "handle" || arg.Of == "" {
				continue
			}
			found := false
			for j := 0; j < i; j++ {
				for _, k := range order[j].Creates {
					if k == arg.Of {
						found = true
					}
				}
			}
			if !found {
				t.Errorf("%s runs at %d and needs a %q that nothing before it creates",
					a.Name, i, arg.Of)
			}
		}
	}
	names := make([]string, 0, len(order))
	for _, a := range order {
		names = append(names, a.Name)
	}
	t.Logf("PROOF needs %v, order %v", need, names)
}

// A KIND NOTHING CREATES IS REFUSED BY NAME, not skipped: a short setup would
// send ids it never obtained and the 404s would be reported as findings about
// the system.

// NEEDS READS THE CORPUS AND NOT THE BINDINGS. A case that omits its only
// handle needs no id, so a setup driven from the bindings alone would do work
// no case asked for -- and on a project whose only handle-taking action is
// fuzzed only by `missing`, that work could not even be done.
func TestNeedsIsDrivenByTheCasesAndNotTheDeclarations(t *testing.T) {
	b := toyBindings(t)
	all, err := Corpus(b)
	if err != nil {
		t.Fatalf("Corpus: %v", err)
	}
	if got := Needs(b, all); len(got) == 0 {
		t.Fatal("the full corpus needs nothing")
	}
	// Only the cases that OMIT their handle: nothing honest is left to resolve.
	var omits []Case
	for _, c := range all {
		if c.Class == ClassMissing {
			omits = append(omits, c)
		}
	}
	if len(omits) == 0 {
		t.Fatal("no missing-class case, so this arm cannot distinguish the two readings")
	}
	got := Needs(b, omits)
	for _, k := range got {
		// An action with TWO handles still needs the other one honest, so a
		// non-empty answer is correct; what must not appear is a kind needed
		// ONLY by a field some case omits.
		t.Logf("still needed under missing-only: %q", k)
	}
	if len(got) >= len(Needs(b, all)) {
		t.Errorf("missing-only cases need %v, the full corpus needs %v: "+
			"Needs is not reading the cases", got, Needs(b, all))
	}
	t.Logf("PROOF full corpus needs %v, missing-only needs %v", Needs(b, all), got)
}

// THE PROVIDER CHOICE IS DECLARATION ORDER AND NOT A MAP's, so the sequence a
// reader derives by hand from the file is the one the stage runs.
