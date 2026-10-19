// The example packages' bindings, held to the packages' own cases.
//
// WHY THIS FILE EXISTS AND WHAT IT IS NOT. R4.2 owes "one bindings file per R1
// example package", and the easy version of that is three files nobody reads.
// The check that makes them worth having is the one rook's review implies (coop
// r4/24): a bindings file's claim types, observation name, source and `of` are
// DERIVED from that package's cases, so the two can be required to agree. What
// cannot be derived -- the action's name and response shape -- is not checked
// here, because a case carries the ledger and never the act that produced it.
package contract_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/decl"
	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

const examplesDir = "../../examples"

// caseFile is the part of an example case this test reads.
type caseFile struct {
	Input struct {
		Observation struct {
			Of     string `json:"of"`
			Query  string `json:"query"`
			Source string `json:"source"`
			Role   string `json:"role"`
		} `json:"observation"`
		Claims []struct {
			Type      string `json:"type"`
			VisibleTo any    `json:"visible_to"`
		} `json:"claims"`
	} `json:"input"`
}

func TestEveryExampleCarriesBindingsThatAgreeWithItsCases(t *testing.T) {
	pkgs, err := filepath.Glob(filepath.Join(examplesDir, "*", "bindings.toml"))
	if err != nil {
		t.Fatalf("globbing: %v", err)
	}
	// R1 SHIPPED THREE PACKAGES, R4.2 OWES ONE FILE EACH, R4.3 ADDED THE TWIN AND
	// R5.2 THE TOY. The literal is the claim: a package without bindings, or a
	// bindings file for a package that no longer exists, fails here rather than
	// passing quietly.
	if len(pkgs) != 5 {
		t.Fatalf("%d bindings file(s) under examples/, want 5: %v", len(pkgs), pkgs)
	}

	for _, path := range pkgs {
		dir := filepath.Dir(path)
		name := filepath.Base(dir)

		rep, err := bindings.Load(path, decl.TOML)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if !rep.OK() {
			t.Errorf("%s: the bindings do not parse: %v", name, rep.Refusals)
			continue
		}
		b := rep.Bindings

		// what the CASES say
		caseTypes, caseRoles := map[string]bool{}, map[string]bool{}
		// THE CASES' OBSERVATIONS AS A SET. This held one (query, source, of)
		// and refused a package whose cases observed differently, saying so in
		// its own message: "this test assumes one observation per package and
		// that is no longer true". examples/toy (R5.2) made it untrue -- it
		// observes a count through `api` AND the store through `direct`, the
		// second oracle being the whole point of a `direct` observation and the
		// only thing validate's rollback-coverage arm has to compare. So the
		// assertion is now a set equality, which is what it always meant: every
		// observation the cases carry is declared, and every declared
		// observation is carried.
		caseObs := map[[3]string]bool{}
		cases, err := filepath.Glob(filepath.Join(dir, "cases", "*.json"))
		if err != nil || len(cases) == 0 {
			t.Errorf("%s: no cases to derive from (%v)", name, err)
			continue
		}
		for _, c := range cases {
			raw, err := os.ReadFile(c)
			if err != nil {
				t.Fatalf("%s: %v", c, err)
			}
			var cf caseFile
			if err := json.Unmarshal(raw, &cf); err != nil {
				t.Fatalf("%s: %v", c, err)
			}
			o := cf.Input.Observation
			caseObs[[3]string{o.Query, o.Source, o.Of}] = true
			if o.Role != "" {
				caseRoles[o.Role] = true
			}
			for _, cl := range cf.Input.Claims {
				caseTypes[cl.Type] = true
				switch v := cl.VisibleTo.(type) {
				case string:
					caseRoles[v] = true
				case []any:
					for _, item := range v {
						if s, ok := item.(string); ok {
							caseRoles[s] = true
						}
					}
				}
			}
		}

		// THE OBSERVATIONS, all three fields, BOTH DIRECTIONS. A declared
		// observation no case carries is a binding for another package; an
		// observation the cases carry and the bindings do not declare is a
		// ledger the bindings cannot produce.
		declaredObs := map[[3]string]bool{}
		for _, o := range b.Observations {
			key := [3]string{o.Name, string(o.Source), o.Of}
			declaredObs[key] = true
			if o.Source != wire.SourceAPI && o.Source != wire.SourceUI && o.Source != wire.SourceDirect {
				t.Errorf("%s: source %q is not one of the vocabulary's three", name, o.Source)
			}
		}
		if len(b.Observations) == 0 {
			t.Errorf("%s: no observation declaredObs, and the cases carry %d", name, len(caseObs))
		}
		for key := range caseObs {
			if !declaredObs[key] {
				t.Errorf("%s: the cases observe (%s, %s, %s) and the bindings declare no such observation",
					name, key[0], key[1], key[2])
			}
		}
		for key := range declaredObs {
			if !caseObs[key] {
				t.Errorf("%s: the bindings declare (%s, %s, %s) and no case carries it",
					name, key[0], key[1], key[2])
			}
		}

		// THE CLAIM TYPES, both directions: a type the cases carry and the
		// bindings do not emit is a binding that cannot produce the ledger, and
		// a type the bindings emit and no case carries is a binding for another
		// package.
		emitted := map[string]bool{}
		for _, a := range b.Actions {
			for _, e := range a.Emits {
				emitted[e] = true
			}
		}
		for ct := range caseTypes {
			if !emitted[ct] {
				t.Errorf("%s: the cases carry claim type %q and no action emits it", name, ct)
			}
			if _, ok := b.Claims[ct]; !ok {
				t.Errorf("%s: claim type %q has no [claim.%q] timing", name, ct, ct)
			}
		}
		for e := range emitted {
			if !caseTypes[e] {
				t.Errorf("%s: an action emits %q and no case carries it", name, e)
			}
		}

		// THE ROLES, one direction only and the reason is stated: a role the
		// cases carry must be declared, while a package whose cases carry NO
		// role declares none -- planted-lie and transitive-count -- because a
		// declared role with no case behind it would be the bindings inventing
		// the package's subject.
		declared := map[string]bool{}
		for _, r := range b.Roles {
			declared[r.Name] = true
		}
		for r := range caseRoles {
			if !declared[r] {
				t.Errorf("%s: the cases carry role %q and the bindings do not declare it", name, r)
			}
		}
		for r := range declared {
			if !caseRoles[r] {
				t.Errorf("%s: the bindings declare role %q and no case carries it", name, r)
			}
		}
		if len(caseRoles) == 0 && len(b.Roles) != 0 {
			t.Errorf("%s: %d role(s) declared where the cases carry none", name, len(b.Roles))
		}
	}
}

// TestOnlyTheTwinDeclaresAPrecondition PINS A RULING rather than flagging an
// open question, which is what it was for about forty minutes.
//
// IT IS TWO-DIRECTIONAL SINCE R4.3 (ibis), and that is the point rather than a
// convenience: narrowing it to "skip the twin" would have let the twin LOSE its
// precondition silently, and arm 3's only case material with it. So the
// assertion is the exact set -- the twin declares one, the R1 three do not --
// and either half failing is a defect.
//
// The collision: validate's third arm refuses an action with no precondition,
// and no R1 statute HAS a precondition rule -- all three are counts, sets, a
// finding and the planted control. So these three files can parse or validate,
// not both. fable ruled it (coop r4/45, on quill r4/37): the R1 packages are
// LEDGER fixtures, their bindings are held to the strict reader and to their own
// cases, and issue 63's done-when reads "parse and agree with the cases" for
// them. Validation lives in a purpose-built twin, which fable moved to
// examples/twin/ at coop r4/63 -- flat, beside the three, so every reader that
// walks examples/* sees it without seven call sites learning to recurse.
//
// SO A `requires` APPEARING HERE IS A DEFECT, not progress: it would make an R1
// fixture pretend to be the twin. The assertion therefore stays, where before it
// was written to be deleted.
func TestOnlyTheTwinDeclaresAPrecondition(t *testing.T) {
	pkgs, _ := filepath.Glob(filepath.Join(examplesDir, "*", "bindings.toml"))
	seen := map[string]bool{}
	var withRequires []string
	for _, path := range pkgs {
		rep, err := bindings.Load(path, decl.TOML)
		if err != nil || !rep.OK() {
			t.Fatalf("%s: %v %v", path, err, rep.Refusals)
		}
		pkg := filepath.Base(filepath.Dir(path))
		for _, a := range rep.Bindings.Actions {
			if len(a.Requires) > 0 && !seen[pkg] {
				seen[pkg] = true
				withRequires = append(withRequires, pkg)
			}
		}
	}
	sort.Strings(withRequires)
	// THE SET GREW BY ONE AND THE RULE DID NOT. examples/toy (R5.2) is a real
	// application rather than a ledger fixture, and validate's arm 3 refuses
	// ANY action with no precondition -- so every one of its six actions names
	// a rule, and the package could not exist otherwise. What this assertion is
	// about is unchanged and is the other half: an R1 package has no
	// precondition rule for a `requires` to point at, so a `requires` there
	// would still be an R1 fixture pretending to be the twin.
	if got := strings.Join(withRequires, ","); got != "toy,twin" {
		t.Errorf("packages declaring a precondition: %v, want exactly [toy twin]. An R1 package is a "+
			"LEDGER fixture whose statute has no precondition rule for a `requires` to point at "+
			"(fable, coop r4/45); the twin is the one FIXTURE package that must have one, or arm 3 "+
			"has no case material (fable, coop r4/63); and examples/toy is an application, whose "+
			"every action must declare one", withRequires)
	}
}
