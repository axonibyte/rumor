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
	// R1 SHIPPED THREE PACKAGES AND R4.2 OWES ONE FILE EACH. The literal is the
	// claim: a fourth package without bindings, or a bindings file for a package
	// that no longer exists, fails here rather than passing quietly.
	if len(pkgs) != 3 {
		t.Fatalf("%d bindings file(s) under examples/, want 3: %v", len(pkgs), pkgs)
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
		var query, source, of string
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
			if query == "" {
				query, source, of = o.Query, o.Source, o.Of
			}
			if o.Query != query || o.Source != source || o.Of != of {
				t.Errorf("%s: %s observes (%s, %s, %s) where another case observes (%s, %s, %s); "+
					"this test assumes one observation per package and that is no longer true",
					name, filepath.Base(c), o.Query, o.Source, o.Of, query, source, of)
			}
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

		// THE OBSERVATION, all three fields.
		if len(b.Observations) != 1 {
			t.Errorf("%s: %d observation(s) declared, and the cases carry one", name, len(b.Observations))
		} else {
			o := b.Observations[0]
			if o.Name != query || string(o.Source) != source || o.Of != of {
				t.Errorf("%s: bindings observe (%s, %s, %s), cases observe (%s, %s, %s)",
					name, o.Name, o.Source, o.Of, query, source, of)
			}
			if o.Source != wire.SourceAPI && o.Source != wire.SourceUI && o.Source != wire.SourceDirect {
				t.Errorf("%s: source %q is not one of the vocabulary's three", name, o.Source)
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

// TestTheR1BindingsDeclareNoPreconditions PINS A RULING rather than flagging an
// open question, which is what it was for about forty minutes.
//
// The collision: validate's third arm refuses an action with no precondition,
// and no R1 statute HAS a precondition rule -- all three are counts, sets, a
// finding and the planted control. So these three files can parse or validate,
// not both. fable ruled it (coop r4/45, on quill r4/37): the R1 packages are
// LEDGER fixtures, their bindings are held to the strict reader and to their own
// cases, and issue 63's done-when reads "parse and agree with the cases" for
// them. Validation lives in a purpose-built twin under examples/validate/twin/,
// where a statute with a real precondition rule gives arm 3 case material that
// is derived rather than invented.
//
// SO A `requires` APPEARING HERE IS A DEFECT, not progress: it would make an R1
// fixture pretend to be the twin. The assertion therefore stays, where before it
// was written to be deleted.
func TestTheR1BindingsDeclareNoPreconditions(t *testing.T) {
	pkgs, _ := filepath.Glob(filepath.Join(examplesDir, "*", "bindings.toml"))
	var withRequires []string
	for _, path := range pkgs {
		rep, err := bindings.Load(path, decl.TOML)
		if err != nil || !rep.OK() {
			t.Fatalf("%s: %v %v", path, err, rep.Refusals)
		}
		for _, a := range rep.Bindings.Actions {
			if len(a.Requires) > 0 {
				withRequires = append(withRequires, filepath.Base(filepath.Dir(path)))
			}
		}
	}
	sort.Strings(withRequires)
	if len(withRequires) != 0 {
		t.Errorf("%v declare preconditions, which fable's ruling (coop r4/45) reserves for the twin "+
			"under examples/validate/twin/: an R1 package is a LEDGER fixture and its statute has no "+
			"precondition rule for a `requires` to point at", withRequires)
	}
}
