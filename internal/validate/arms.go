package validate

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
)

// arm1: A CLAIM TYPE THE STATUTE IS JUDGED ON THAT NO BINDING EMITS.
//
// HOW A STATUTE "REFERENCES" A TYPE, measured rather than assumed: no statute
// in this tree names a claim type literally -- a grep for a dotted string over
// all four returns nothing. The type reaches a statute only as
// `input.observation.of`, which the engine takes from the CASE. So the cases
// are the statute's own declaration of what it judges, and a case judged on a
// type no action emits means the statute judges something the project never
// produces.
//
// This is the first of internal/bindings/bindings.go:304's pair, held back from
// the parser because it needs both files and that package is handed one.
func arm1(rep *Report, b bindings.Bindings, cases map[string]caseDoc) {
	emitted := map[string]bool{}
	for _, a := range b.Actions {
		for _, t := range a.Emits {
			emitted[t] = true
		}
	}
	var bad []string
	for name, c := range cases {
		of := c.Input.Observation.Of
		if of != "" && !emitted[of] {
			bad = append(bad, name+" judges "+of)
		}
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		rep.refuse("1", "no action emits the claim type judged by: %s -- the statute judges what the project never produces",
			strings.Join(bad, "; "))
	}
}

// arm2: AN ACTION EMITS A CLAIM TYPE THE [claim] TABLE DOES NOT DESCRIBE.
//
// The mirror of arm 1, and the other half of bindings.go:304's pair: the
// project produces something whose settling nothing declares, so no later arm
// can ask whether it settles immediately or within a bound.
func arm2(rep *Report, b bindings.Bindings) {
	var bad []string
	for _, a := range b.Actions {
		for _, t := range a.Emits {
			if _, ok := b.Claims[t]; !ok {
				bad = append(bad, a.Name+" emits "+t)
			}
		}
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		rep.refuse("2", "no [claim.<type>] table describes: %s -- nothing declares when it settles",
			strings.Join(bad, "; "))
	}
}

// arm3: AN ACTION WITH NO PRECONDITION.
//
// `requires` names statute RULE names. An action declaring none is read as
// having no precondition, which is what this arm refuses -- and the reason
// internal/bindings leaves `requires` optional: if the parser demanded it, the
// file would not parse, the run would report could-not-run, and this arm's
// fixture would be testing the parser.
//
// THE ASYMMETRY WITH `touches` IS DELIBERATE AND WRITTEN DOWN (docs/contract.md
// and bindings.go): an action naming nothing it touches is read as mutating
// nothing and NO arm refuses it. A nineteenth arm is not this package's to
// invent; the count is fable's.
func arm3(rep *Report, b bindings.Bindings) {
	var bad []string
	for _, a := range b.Actions {
		if len(a.Requires) == 0 {
			bad = append(bad, a.Name)
		}
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		rep.refuse("3", "no `requires` on: %s -- an action with no precondition is one the walk may take from any state",
			strings.Join(bad, ", "))
	}
}

// arm11: RULES DECLARED AND NO MUTATION FIXTURE PRESENT.
//
// THE STRUCTURAL HALF ONLY, ruled at coop r4/69. A mutant file does not declare
// which rule it cuts -- mutants/README.md describes them and the .rego does not
// -- so the per-RULE question, does a KILLING mutant exist for this rule, is
// answerable only by running them. That is `rumor attaint`'s arm and this one
// must not restate it with a weaker oracle.
//
// So: rules.json non-empty and mutants/ holding no mutant at all -- absent, or
// empty of .rego, or every file byte-identical to the statute, since a copy
// cuts nothing and mutants/README.md already refuses one naming the cut.
func arm11(rep *Report, dir string, rules []string) {
	if len(rules) == 0 {
		return // nothing declared, so nothing is owed a fixture
	}
	statute, _ := os.ReadFile(filepath.Join(dir, "statute.rego"))
	files, _ := filepath.Glob(filepath.Join(dir, "mutants", "*.rego"))
	real := 0
	for _, f := range files {
		body, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if len(statute) > 0 && string(body) == string(statute) {
			continue // a copy cuts nothing
		}
		real++
	}
	rep.Mutants = real
	if real == 0 {
		rep.refuse("11", "%d rule(s) declared in rules.json and no mutation fixture under mutants/: "+
			"whether each rule is load-bearing is then unasked, and attaint has nothing to run", len(rules))
	}
}

// arm12: AN OBSERVATION LACKING A POSITION (12a) OR A SOURCE (12b).
//
// STATIC OVER THE CASES, not over bindings.toml, and that is settled rather
// than convenient (docs/contract.md, and ibis at coop r4/20): a binding's
// observation declares no position at all -- positions are the ledger's,
// assigned on append -- so the only place an observation carries both is a
// case. fable's D16 strips one field from one of them.
//
// TWO ARMS, ONE WALK, because they quantify over the same set and a case
// missing both should name both rather than the first.
func arm12(rep *Report, cases map[string]caseDoc) {
	var noPos, noSrc []string
	for name, c := range cases {
		if c.Input.Observation.Position == nil {
			noPos = append(noPos, name)
		}
		if c.Input.Observation.Source == nil || *c.Input.Observation.Source == "" {
			noSrc = append(noSrc, name)
		}
	}
	if len(noPos) > 0 {
		sort.Strings(noPos)
		rep.refuse("12a", "no observation position in: %s -- it cannot be bounded against the seal",
			strings.Join(noPos, ", "))
	}
	if len(noSrc) > 0 {
		sort.Strings(noSrc)
		rep.refuse("12b", "no observation source in: %s -- it cannot be read as api, ui or direct",
			strings.Join(noSrc, ", "))
	}
}
