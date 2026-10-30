package fuzz

import (
	"path/filepath"
	"sort"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/decl"
)

// toyBindings loads the SHIPPED toy rather than a fixture of its own. A
// fixture here would be a copy of the declarations the corpus is derived
// from, so an arm over it would prove the derivation agrees with a copy --
// the override shape this project has been bitten by before.
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

// D1: EVERY DECLARED WRITE ACTION APPEARS. The count is asserted too, so a
// derivation that silently stopped after one action cannot pass by having
// "every action it looked at" present.
func TestEveryDeclaredWriteActionAppears(t *testing.T) {
	b := toyBindings(t)
	writes := Writes(b)
	if len(writes) == 0 {
		t.Fatal("the toy declares no write action, which would make this arm vacuous")
	}
	cases, err := Corpus(b)
	if err != nil {
		t.Fatalf("Corpus: %v", err)
	}
	seen := map[string]int{}
	for _, c := range cases {
		seen[c.Action]++
	}
	for _, a := range writes {
		if seen[a.Name] == 0 {
			t.Errorf("%s emits %v and earns no case: the corpus is not derived from the declarations",
				a.Name, a.Emits)
		}
	}
	if len(seen) != len(writes) {
		t.Errorf("%d write action(s) declared, %d represented in the corpus", len(writes), len(seen))
	}
	t.Logf("PROOF %d write action(s), %d case(s), every action represented", len(writes), len(cases))
}

// D1's PARTNER: the corpus is a FUNCTION of the declarations, so an action
// added to them appears without anyone editing this package. Driven by adding
// one rather than by reading the code, because "derived" is a claim about
// behaviour under a changed input.
func TestAnAddedWriteActionAppearsWithoutEditingTheCorpus(t *testing.T) {
	b := toyBindings(t)
	before, err := Corpus(b)
	if err != nil {
		t.Fatalf("Corpus: %v", err)
	}
	b.Actions = append(b.Actions, bindings.Action{
		Name:    "shred_event",
		Context: "api",
		Emits:   []string{"event.shredded"},
		Args:    []bindings.Arg{{Name: "event", Source: "handle", Of: "event"}},
	})
	after, err := Corpus(b)
	if err != nil {
		t.Fatalf("Corpus after the addition: %v", err)
	}
	if len(after) <= len(before) {
		t.Fatalf("adding a write action moved the corpus from %d to %d case(s): "+
			"a hand-written corpus would not have grown", len(before), len(after))
	}
	found := false
	for _, c := range after {
		if c.Action == "shred_event" {
			found = true
			break
		}
	}
	if !found {
		t.Error("shred_event emits a claim and earned no case")
	}
	t.Logf("PROOF %d -> %d case(s) on one added action", len(before), len(after))
}

// AN ACTION THAT EMITS NOTHING IS NOT A WRITE, so it earns no case. The other
// half of the predicate, since the toy happens to have no read-only action and
// an arm over the toy alone could not tell "all nine are writes" from "every
// action is fuzzed".
func TestAnActionThatEmitsNothingIsNotFuzzed(t *testing.T) {
	b := toyBindings(t)
	b.Actions = append(b.Actions, bindings.Action{
		Name:    "peek_event",
		Context: "api",
		Args:    []bindings.Arg{{Name: "event", Source: "handle", Of: "event"}},
	})
	cases, err := Corpus(b)
	if err != nil {
		t.Fatalf("Corpus: %v", err)
	}
	for _, c := range cases {
		if c.Action == "peek_event" {
			t.Fatalf("peek_event emits no claim and earned %s", c)
		}
	}
	if got := len(Writes(b)); got != 9 {
		t.Errorf("Writes counted %d over the toy plus one read-only action, expected 9", got)
	}
}

// NO WRITE ACTION AT ALL IS A REFUSAL AND NOT AN EMPTY CORPUS, because the
// two read identically in a report.
func TestNoWriteActionRefusesByName(t *testing.T) {
	var b bindings.Bindings
	b.Actions = []bindings.Action{{Name: "peek", Context: "api"}}
	if _, err := Corpus(b); err == nil {
		t.Fatal("a project with no write action returned a corpus instead of refusing")
	}
	b.Actions = []bindings.Action{{Name: "poke", Context: "api", Emits: []string{"poked"}}}
	_, err := Corpus(b)
	if err == nil {
		t.Fatal("a write action with no arguments returned a corpus instead of refusing")
	}
	t.Logf("PROOF both empty shapes refuse: %v", err)
}

// THE CLASSES ARE EARNED BY THE DECLARATION AND NOT HANDED OUT, which is what
// keeps the corpus honest about what it knows: only a `handle` can have an id
// that names nothing, and only a numeric literal can be non-positive.
func TestAClassIsOnlyGeneratedWhereTheDeclarationEarnsIt(t *testing.T) {
	b := toyBindings(t)
	cases, err := Corpus(b)
	if err != nil {
		t.Fatalf("Corpus: %v", err)
	}
	src := map[string]string{}
	byArg := map[string]bindings.Arg{}
	for _, a := range b.Actions {
		for _, arg := range a.Args {
			src[a.Name+"/"+arg.Name] = arg.Source
			byArg[a.Name+"/"+arg.Name] = arg
		}
	}
	roleSet := map[string]bool{}
	for _, r := range b.Roles {
		roleSet[r.Name] = true
	}
	if len(roleSet) == 0 {
		t.Fatal("the toy declares no role, which would make the not-a-role check vacuous")
	}
	classes := map[Class]int{}
	for _, c := range cases {
		classes[c.Class]++
		key := c.Action + "/" + c.Field
		switch c.Class {
		case ClassUnknownID:
			if src[key] != "handle" {
				t.Errorf("%s: unknown-id on a %q argument, which has no id to be unknown", c, src[key])
			}
		case ClassNonPositive:
			// EARNED BY A NUMERIC DOMAIN, whether the number is a literal's
			// value or a draw's `from`. The toy's `capacity` is a DRAW, so
			// restricting this to literals made the class dead against the
			// shipped project -- found by driving the class-coverage arm, not
			// by reading this function.
			if s := src[key]; s != "literal" && s != "draw" {
				t.Errorf("%s: non-positive on a %q argument", c, s)
			}
			if !numericDomain(byArg[key]) {
				t.Errorf("%s: non-positive on an argument with no numeric domain", c)
			}
		case ClassNotARole:
			if !roleDomain(roleSet, byArg[key]) {
				t.Errorf("%s: not-a-role on an argument whose domain is not role names", c)
			}
		case ClassEmpty:
			if s := src[key]; s == "draw" {
				t.Errorf("%s: empty on a draw, whose type the declaration does not give", c)
			}
		}
		if c.Field == "" {
			t.Errorf("%s records no field, so no refusal can be checked against it", c)
		}
	}
	for _, cl := range []Class{ClassMissing, ClassEmpty, ClassWrongType, ClassUnknownID,
		ClassNonPositive, ClassNotARole} {
		if classes[cl] == 0 {
			t.Errorf("class %q appears in no case over the toy, so its branch is unmeasured", cl)
		}
	}
	keys := make([]string, 0, len(classes))
	for cl := range classes {
		keys = append(keys, string(cl))
	}
	sort.Strings(keys)
	t.Logf("PROOF classes %v over %d case(s)", keys, len(cases))
}

// THE PERTURBED FIELD IS THE ONLY ONE THAT MOVES. A class that quietly
// dropped a second argument would refuse for the wrong reason and the
// refusal would still name a field, so this is checked rather than assumed.
func TestOnlyThePerturbedFieldDiffersFromTheHonestRequest(t *testing.T) {
	b := toyBindings(t)
	cases, err := Corpus(b)
	if err != nil {
		t.Fatalf("Corpus: %v", err)
	}
	byName := map[string]bindings.Action{}
	for _, a := range b.Actions {
		byName[a.Name] = a
	}
	for _, c := range cases {
		a := byName[c.Action]
		for _, arg := range a.Args {
			if arg.Name == c.Field {
				continue
			}
			if _, ok := c.Args[arg.Name]; !ok {
				t.Errorf("%s also omits %q, so a refusal could name either", c, arg.Name)
			}
		}
		if c.Class == ClassMissing {
			if _, ok := c.Args[c.Field]; ok {
				t.Errorf("%s is the missing class and still sends %q", c, c.Field)
			}
		}
	}
}

// numericDomain is whether an argument's declaration gives numbers: a
// literal's value or a draw's domain.
func numericDomain(arg bindings.Arg) bool {
	var v any
	switch arg.Source {
	case "literal":
		v = arg.Value
	case "draw":
		if len(arg.From) == 0 {
			return false
		}
		v = arg.From[0]
	default:
		return false
	}
	switch v.(type) {
	case int64, int, float64:
		return true
	}
	return false
}

// roleDomain is whether every member of a draw's domain is a declared role.
func roleDomain(roles map[string]bool, arg bindings.Arg) bool {
	if arg.Source != "draw" || len(arg.From) == 0 {
		return false
	}
	for _, elem := range arg.From {
		list, ok := elem.([]any)
		if !ok {
			return false
		}
		for _, x := range list {
			s, ok := x.(string)
			if !ok || !roles[s] {
				return false
			}
		}
	}
	return true
}

// THE NOT-A-ROLE VALUE IS NOT A ROLE, which the class's whole claim rests on
// and which a later project could break by declaring that name.
func TestTheNotARoleValueIsNotADeclaredRole(t *testing.T) {
	b := toyBindings(t)
	roles := map[string]bool{}
	for _, r := range b.Roles {
		roles[r.Name] = true
	}
	v, ok := notARole(roles, []any{[]any{"admin"}})
	if !ok {
		t.Fatal("a role-only domain earned no not-a-role value")
	}
	if roles[v] {
		t.Errorf("the not-a-role value %q IS a declared role, so the class sends a legal value", v)
	}
	if _, ok := notARole(roles, []any{[]any{"admin", "nonesuch"}}); ok {
		t.Error("a domain holding a non-role earned a not-a-role value, so the rule was assumed")
	}
	if _, ok := notARole(map[string]bool{}, []any{[]any{"admin"}}); ok {
		t.Error("a project with no declared role earned a not-a-role value")
	}
}
