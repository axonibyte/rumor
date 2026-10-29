// Package engine_test drives the three example packages THROUGH THE ENGINE,
// from the ledger files, and requires the result to equal what the bare
// statute tests get from the flattened cases (R2.3, issue 44).
//
// WHY THIS IS NOT tests/statute AGAIN. That harness hands each statute the
// case's own `input` block: a `known` array and a claim list somebody wrote by
// hand. This one hands the statute nothing. It loads examples/<pkg>/ledger/
// <case>.jsonl through internal/ledger, seals it at the position the .seal.json
// records, and lets internal/engine derive the claim list, the flattened
// knowledge and the observation's position from that prefix alone. The only
// thing supplied from outside is the check -- the observed value, its query,
// its type, the source and the role -- which is what an observation IS.
//
// So the question this answers and the layer below cannot: does the path a real
// run takes (facts -> store -> seal -> flatten -> statute) reach the same
// verdicts as the path the examples were authored against? A difference here is
// an adapter defect, and nothing in tests/statute or tests/ledger would see it.
//
// THE TWO PATHS DO NOT SHARE AN EVALUATOR. The ledger path goes through
// internal/engine; the case path calls rego directly in this file. A harness
// that ran both through the same helper would agree with itself about anything
// that helper got wrong.
//
// POSITIONS ARE DELIBERATELY NOT COMPARED. R2.1 re-assigned them -- a tell
// needs a position of its own, so the integers differ by construction and the
// vocabulary permits it ("a replay assigns its own"). What must agree is the
// findings and every named sub-rule, which is what "the same findings" means.
package engine_test

import (
	"context"
	"encoding/json"
	"fmt"
	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/decl"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/open-policy-agent/opa/v1/rego"

	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
	"forge.axonibyte.com/axonibyte/rumor/internal/ledger"
)

const examplesDir = "../../examples"

// statutePackage is duplicated from the statutes rather than parsed out of
// them, for the reason tests/statute gives: a harness that read the package
// name from the module under test would agree with it whatever it said.
const statutePackage = "data.rumor.statute"

// Declared sizes. ibis main/352's rule: a count of zero disagreements reads
// identically whether every case agreed or none ran, so the sizes are asserted
// before any verdict is reported. They match tests/statute's own declarations.
const (
	wantPackages = 5
	wantCases    = 33
	wantMutants  = 37
)

type observation struct {
	Actor    string `json:"actor"`
	Position int    `json:"position"`
	Of       string `json:"of"`
	Query    string `json:"query"`
	Source   string `json:"source"`
	Value    any    `json:"value"`
	Role     string `json:"role"`
}

type sealRecord struct {
	Bound         int      `json:"bound"`
	Observer      string   `json:"observer"`
	Seal          int      `json:"seal"`
	KnownInPrefix []string `json:"known_in_prefix"`
}

type kase struct {
	name       string
	expect     map[string]any
	caseInput  map[string]any // the flattened input the case carries
	obs        observation
	ledgerPath string
	seal       sealRecord
	raw        string // the committed ledger bytes
}

type pkg struct {
	name    string
	statute string
	rules   []string
	cases   []kase
	mutants []string
}

func load(t *testing.T) []pkg {
	t.Helper()
	entries, err := os.ReadDir(examplesDir)
	if err != nil {
		t.Fatalf("%s is not readable, so this harness would examine nothing: %v", examplesDir, err)
	}
	var out []pkg
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(examplesDir, e.Name())
		src, err := os.ReadFile(filepath.Join(dir, "statute.rego"))
		if err != nil {
			continue
		}
		p := pkg{name: e.Name(), statute: string(src)}
		rulesRaw, err := os.ReadFile(filepath.Join(dir, "rules.json"))
		if err != nil {
			t.Fatalf("%s has a statute but no rules.json", p.name)
		}
		if err := json.Unmarshal(rulesRaw, &p.rules); err != nil {
			t.Fatalf("%s/rules.json: %v", p.name, err)
		}
		if len(p.rules) == 0 {
			t.Fatalf("%s/rules.json names no rules, so no case in it can assert anything", p.name)
		}

		caseFiles, _ := filepath.Glob(filepath.Join(dir, "cases", "*.json"))
		sort.Strings(caseFiles)
		for _, cf := range caseFiles {
			raw, err := os.ReadFile(cf)
			if err != nil {
				t.Fatalf("%s: %v", cf, err)
			}
			var doc struct {
				Expect map[string]any `json:"expect"`
				Input  struct {
					Observation observation      `json:"observation"`
					Known       []string         `json:"known"`
					Claims      []map[string]any `json:"claims"`
				} `json:"input"`
			}
			if err := json.Unmarshal(raw, &doc); err != nil {
				t.Fatalf("%s: %v", cf, err)
			}
			if len(doc.Expect) == 0 {
				t.Fatalf("%s has no expect block, so it asserts nothing", cf)
			}
			name := strings.TrimSuffix(filepath.Base(cf), ".json")

			// The case's own input, re-assembled as the bare harness hands it
			// over: the same three fields, from the same file.
			var flat map[string]any
			if err := json.Unmarshal(raw, &struct {
				Input *map[string]any `json:"input"`
			}{Input: &flat}); err != nil {
				t.Fatalf("%s: %v", cf, err)
			}

			lp := filepath.Join(dir, "ledger", name+".jsonl")
			lraw, err := os.ReadFile(lp)
			if err != nil {
				t.Fatalf("case %s has no ledger at %s: R2.1 put one beside every case", name, lp)
			}
			var s sealRecord
			sraw, err := os.ReadFile(filepath.Join(dir, "ledger", name+".seal.json"))
			if err != nil {
				t.Fatalf("ledger %s has no seal record: the seal is not derivable from the file", name)
			}
			if err := json.Unmarshal(sraw, &s); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			p.cases = append(p.cases, kase{
				name: name, expect: doc.Expect, caseInput: flat,
				obs: doc.Input.Observation, ledgerPath: lp, seal: s, raw: string(lraw),
			})
		}
		p.mutants, _ = filepath.Glob(filepath.Join(dir, "mutants", "*.rego"))
		sort.Strings(p.mutants)
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// sealed loads the case's committed ledger and seals it at the RECORDED
// position. Sealing at the end of the file instead would erase the three
// position-bounded cases, which are the ones whose bound this path must honour.
func (c kase) sealed(t *testing.T) *ledger.Ledger {
	t.Helper()
	// UNDER THE PROJECT'S OWN CONTROL (R7.4): `false` refused every clock
	// fact, so a clocked project's cases could not be read here at all, and
	// R7.4's ledger row is one. Asked through bindings.ClockControlledAt, the
	// one expression cmd/rumor/run.go uses.
	l, err := ledger.Load(strings.NewReader(c.raw),
		bindings.ClockControlledAt(filepath.Dir(filepath.Dir(c.ledgerPath)), decl.TOML))
	if err != nil {
		t.Fatalf("%s: the committed ledger does not load: %v", c.name, err)
	}
	if err := l.SealAt(c.seal.Seal); err != nil {
		t.Fatalf("%s: sealing at the recorded %d: %v", c.name, c.seal.Seal, err)
	}
	return l
}

// check is the observation, taken from the case. Its POSITION is left behind
// deliberately: the engine derives that from the seal, and a caller cannot
// place its observation inside the data that judges it.
func (c kase) check() engine.Check {
	return engine.Check{
		Actor:  c.obs.Actor,
		Query:  c.obs.Query,
		Of:     c.obs.Of,
		Source: ledger.Source(c.obs.Source),
		Value:  c.obs.Value,
		Role:   c.obs.Role,
	}
}

// evalCasePath evaluates a module against the case's own flattened input, the
// way tests/statute does, through rego directly.
func evalCasePath(ctx context.Context, module string, rules []string, input any) (map[string]any, error) {
	got := map[string]any{}
	for _, rule := range rules {
		rs, err := rego.New(
			rego.Query(statutePackage+"."+rule),
			rego.Module("statute.rego", module),
			rego.Input(input),
		).Eval(ctx)
		if err != nil {
			return nil, fmt.Errorf("rule %s: %w", rule, err)
		}
		if len(rs) == 0 || len(rs[0].Expressions) == 0 {
			continue
		}
		got[rule] = rs[0].Expressions[0].Value
	}
	return got, nil
}

func sameJSON(a, b any) bool {
	ja, err1 := json.Marshal(a)
	jb, err2 := json.Marshal(b)
	if err1 != nil || err2 != nil {
		return false
	}
	var ua, ub any
	if json.Unmarshal(ja, &ua) != nil || json.Unmarshal(jb, &ub) != nil {
		return false
	}
	return reflect.DeepEqual(ua, ub)
}

func disagreements(expect, got map[string]any) []string {
	var bad []string
	keys := make([]string, 0, len(expect))
	for k := range expect {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, rule := range keys {
		have, present := got[rule]
		if !present {
			bad = append(bad, fmt.Sprintf("%s: undefined, wanted %v", rule, expect[rule]))
			continue
		}
		if !sameJSON(expect[rule], have) {
			bad = append(bad, fmt.Sprintf("%s: got %v, wanted %v", rule, have, expect[rule]))
		}
	}
	return bad
}

func TestTheHarnessExaminesSomething(t *testing.T) {
	pkgs := load(t)
	if len(pkgs) != wantPackages {
		t.Fatalf("expected %d statute packages, found %d", wantPackages, len(pkgs))
	}
	cases, mutants, bounded := 0, 0, 0
	for _, p := range pkgs {
		if len(p.cases) == 0 {
			t.Errorf("%s has no cases", p.name)
		}
		if len(p.mutants) == 0 {
			t.Errorf("%s has no mutants, so nothing proves its clauses matter", p.name)
		}
		cases += len(p.cases)
		mutants += len(p.mutants)
		for _, c := range p.cases {
			l := c.sealed(t)
			if l.Len() > c.seal.Seal {
				bounded++
			}
		}
	}
	if cases != wantCases {
		t.Errorf("expected %d cases, counted %d", wantCases, cases)
	}
	if mutants != wantMutants {
		t.Errorf("expected %d mutants, counted %d", wantMutants, mutants)
	}
	// AND THE ARM THAT STOPS THIS HARNESS PASSING VACUOUSLY: if no ledger held
	// a fact past its seal, every case would be a prefix of its whole file and
	// the bound this path exists to honour would never be exercised.
	if bounded == 0 {
		t.Error("no ledger holds a fact past its seal, so no case here exercises the bound")
	}
	t.Logf("PROOF %d package(s), %d case(s), %d mutant(s), %d ledger(s) with facts past the seal",
		len(pkgs), cases, mutants, bounded)
}

// THE CASES ARE GREEN FROM THE LEDGER FILES, against the `expect` the examples
// authored -- not against whatever the other path happens to produce.
func TestEveryCaseIsGreenFromItsLedger(t *testing.T) {
	ctx := context.Background()
	for _, p := range load(t) {
		for _, c := range p.cases {
			t.Run(p.name+"/"+c.name, func(t *testing.T) {
				f, err := engine.Evaluate(ctx, p.statute, p.rules, c.sealed(t), c.check())
				if err != nil {
					t.Fatalf("evaluating from the ledger: %v", err)
				}
				if bad := disagreements(c.expect, f.Rules); len(bad) > 0 {
					t.Fatalf("the ledger path disagrees with this case:\n  %s", strings.Join(bad, "\n  "))
				}
				if !f.Defined {
					t.Fatal("no finding at all")
				}
				if len(f.Fired) != 1 {
					t.Errorf("%d definition(s) fired, so the finding names no single rule: %v", len(f.Fired), f.Fired)
				}
				if f.TraceEvents == 0 || f.Trace == "" {
					t.Error("the finding carries no trace, which R10 needs")
				}
				t.Logf("PROOF %v from %s, %d trace event(s), %d rule(s) agreed",
					f.Value, f.Fired[0], f.TraceEvents, len(c.expect))
			})
		}
	}
}

// THE TWO PATHS AGREE, rule by rule, case by case. This is the done-when
// sentence of issue 44, and it is a stronger claim than both paths being green
// against `expect`: `expect` names only the rules the example chose to pin,
// and this compares every rule rules.json asks for.
func TestTheLedgerPathAndTheCasePathAgree(t *testing.T) {
	ctx := context.Background()
	compared := 0
	for _, p := range load(t) {
		for _, c := range p.cases {
			t.Run(p.name+"/"+c.name, func(t *testing.T) {
				fromLedger, err := engine.Evaluate(ctx, p.statute, p.rules, c.sealed(t), c.check())
				if err != nil {
					t.Fatalf("the ledger path: %v", err)
				}
				fromCase, err := evalCasePath(ctx, p.statute, p.rules, c.caseInput)
				if err != nil {
					t.Fatalf("the case path: %v", err)
				}
				if len(fromCase) == 0 {
					t.Fatal("the case path returned no rule at all, so there is nothing to agree with")
				}
				for _, rule := range p.rules {
					lv, lok := fromLedger.Rules[rule]
					cv, cok := fromCase[rule]
					if lok != cok {
						t.Errorf("%s: defined in the ledger path %v, in the case path %v", rule, lok, cok)
						continue
					}
					if lok && !sameJSON(lv, cv) {
						t.Errorf("%s: ledger path %v, case path %v", rule, lv, cv)
					}
				}
				compared += len(p.rules)
				t.Logf("PROOF %d rule(s) equal; finding %v both ways", len(p.rules), fromLedger.Value)
			})
		}
	}
	if compared == 0 {
		t.Error("no rule was compared, so this harness proved nothing")
	}
}

// A MUTANT'S VERDICT IS A NAMED CLAUSE, not a boolean. A composite condition
// is satisfied by its weakest part, so the loop below reports WHICH clause
// answered and the self-test drives every clause with a synthetic outcome --
// including the ones no committed mutant currently reaches. Without that, an
// arm could sit permanently unreachable while the suite stayed green.
type clause string

const (
	clauseKilled             clause = "killed-on-both-paths"
	clauseEquivalent         clause = "equivalent-under-the-bound"
	clauseSurvived           clause = "survived-and-not-declared"
	clauseNotIdentical       clause = "declared-but-values-differ"
	clauseCasesDoNotKill     clause = "declared-but-no-path-kills-it"
	clauseDeclarationIsStale clause = "declared-but-killed-from-the-ledger"
	clausePathsDisagree      clause = "paths-disagree-about-which-case-kills-it"
)

// outcome is what one mutant did, measured; judge turns it into a clause.
type outcome struct {
	killedFromLedger []string
	killedFromCases  []string
	identicalToClean bool
	declared         bool
}

// judge is the decision, separated from the measurement so it can be driven
// with outcomes the committed mutants do not produce. Only clauseKilled and
// clauseEquivalent are passes.
func judge(o outcome) clause {
	if len(o.killedFromLedger) == 0 {
		switch {
		case !o.declared:
			return clauseSurvived
		case !o.identicalToClean:
			return clauseNotIdentical
		case len(o.killedFromCases) == 0:
			return clauseCasesDoNotKill
		default:
			return clauseEquivalent
		}
	}
	if o.declared {
		return clauseDeclarationIsStale
	}
	if strings.Join(o.killedFromLedger, ",") != strings.Join(o.killedFromCases, ",") {
		return clausePathsDisagree
	}
	return clauseKilled
}

func (c clause) pass() bool { return c == clauseKilled || c == clauseEquivalent }

// THE ORACLE'S SELF-TEST. Every clause is reached by a constructed outcome, and
// the verdict is read off the clause rather than off pass(): two different
// refusals both make pass() false, so a boolean cannot tell them apart and a
// clause that stopped firing would be invisible.
func TestTheMutantVerdictFires(t *testing.T) {
	one := []string{"some-case"}
	two := []string{"other-case"}
	for _, tc := range []struct {
		name string
		o    outcome
		want clause
	}{
		{"killed by the same case both ways", outcome{killedFromLedger: one, killedFromCases: one}, clauseKilled},
		{"killed from the ledger, a different case from the cases", outcome{killedFromLedger: one, killedFromCases: two}, clausePathsDisagree},
		{"killed nowhere and not declared", outcome{}, clauseSurvived},
		{"declared, identical, still killed from the cases", outcome{declared: true, identicalToClean: true, killedFromCases: one}, clauseEquivalent},
		{"declared but its values differ", outcome{declared: true, identicalToClean: false, killedFromCases: one}, clauseNotIdentical},
		{"declared, identical, and the cases do not kill it either", outcome{declared: true, identicalToClean: true}, clauseCasesDoNotKill},
		{"declared but killed from the ledger", outcome{declared: true, killedFromLedger: one, killedFromCases: one}, clauseDeclarationIsStale},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := judge(tc.o); got != tc.want {
				t.Fatalf("clause %q, wanted %q", got, tc.want)
			}
		})
	}
	// And the two passing clauses are exactly two: a clause added to the pass
	// set without being thought about would widen what this harness permits.
	passes := 0
	for _, c := range []clause{clauseKilled, clauseEquivalent, clauseSurvived,
		clauseNotIdentical, clauseCasesDoNotKill, clauseDeclarationIsStale, clausePathsDisagree} {
		if c.pass() {
			passes++
		}
	}
	if passes != 2 {
		t.Errorf("%d of 7 clauses pass, expected 2", passes)
	}
	t.Logf("PROOF 7 clause(s) each reached by a constructed outcome, 2 of them passes")
}

// EQUIVALENT UNDER THE STRUCTURAL BOUND, declared by name with its reason.
//
// THIS IS R2.3's FINDING AND IT IS NOT A SOFTENED ASSERTION. Three mutants
// remove the `c.position <= input.observation.position` guard from their
// statute. On the case path they die: a case carries claims its own bound
// excludes, so dropping the guard admits them and the counts move. On the
// ledger path there is nothing for the guard to exclude -- the sealed prefix
// holds in-bound facts only, because R2.1 made the bound the SEAL -- so the
// mutant computes exactly what the clean statute computes.
//
// docs/testing-methodology.md: a mutant that cannot apply is not a surviving
// mutant. So the arm below does not merely excuse these three; for each one it
// requires PROOF of equivalence -- identical values for every rule on every
// case -- and requires the case path to still kill it, which is what shows the
// clause does matter and that the structural bound, not the harness, is what
// put it out of reach. A mutant that is neither killed nor provably equivalent
// is a failure, and a mutant declared here that starts dying is a failure too.
var equivalentUnderTheBound = map[string]string{
	"planted-lie/m1-ignores-position":      "the prefix holds only in-bound claims, so the position guard has nothing to exclude",
	"role-visibility/m3-ignores-position":  "same: the seal excludes the late claim and its tell structurally",
	"transitive-count/m1-ignores-position": "same: bob's claim sits past the seal rather than inside the input",
}

func TestEveryMutantIsRedFromItsLedgerOrProvablyEquivalent(t *testing.T) {
	ctx := context.Background()
	equivalent := map[string]bool{}
	for _, p := range load(t) {
		// The clean statute's values from the LEDGER path, per case: the
		// baseline an equivalence claim is measured against.
		clean := map[string]map[string]any{}
		for _, c := range p.cases {
			f, err := engine.Evaluate(ctx, p.statute, p.rules, c.sealed(t), c.check())
			if err != nil {
				t.Fatalf("%s/%s: the clean statute did not evaluate from the ledger: %v", p.name, c.name, err)
			}
			clean[c.name] = f.Rules
		}

		for _, mp := range p.mutants {
			id := p.name + "/" + strings.TrimSuffix(filepath.Base(mp), ".rego")
			t.Run(id, func(t *testing.T) {
				src, err := os.ReadFile(mp)
				if err != nil {
					t.Fatalf("%s: %v", mp, err)
				}
				var byLedger, byCase []string
				identical := true
				for _, c := range p.cases {
					f, err := engine.Evaluate(ctx, string(src), p.rules, c.sealed(t), c.check())
					if err != nil {
						// A mutant that does not evaluate is red for the wrong
						// reason and proves nothing about the clause it removes.
						t.Fatalf("the mutant did not evaluate from the ledger, which is not the same as being caught: %v", err)
					}
					if len(disagreements(c.expect, f.Rules)) > 0 {
						byLedger = append(byLedger, c.name)
					}
					for _, rule := range p.rules {
						cv, cok := clean[c.name][rule]
						mv, mok := f.Rules[rule]
						if cok != mok || (cok && !sameJSON(cv, mv)) {
							identical = false
						}
					}
					got, err := evalCasePath(ctx, string(src), p.rules, c.caseInput)
					if err != nil {
						t.Fatalf("the mutant did not evaluate from the case: %v", err)
					}
					if len(disagreements(c.expect, got)) > 0 {
						byCase = append(byCase, c.name)
					}
				}

				reason, declared := equivalentUnderTheBound[id]
				o := outcome{
					killedFromLedger: byLedger,
					killedFromCases:  byCase,
					identicalToClean: identical,
					declared:         declared,
				}
				switch c := judge(o); c {
				case clauseKilled:
					t.Logf("PROOF %s: killed by %d of %d case(s) on both paths: %s",
						c, len(byLedger), len(p.cases), strings.Join(byLedger, ", "))
				case clauseEquivalent:
					equivalent[id] = true
					t.Logf("PROOF %s (%s); identical on all %d case(s) from the ledger, still killed by %d case(s) from the cases: %s",
						c, reason, len(p.cases), len(byCase), strings.Join(byCase, ", "))
				default:
					t.Fatalf("REFUSED by %s: killed from the ledgers by %v, from the cases by %v, identical to the clean statute %v, declared equivalent %v",
						c, byLedger, byCase, identical, declared)
				}
			})
		}
	}

	// The declared set must be exactly the measured set, in both directions: a
	// fourth mutant going equivalent is a finding, and a declaration that no
	// longer applies is dead weight that would hide one.
	if len(equivalent) != len(equivalentUnderTheBound) {
		t.Errorf("%d mutant(s) took the equivalence branch, %d declared", len(equivalent), len(equivalentUnderTheBound))
	}
	for id := range equivalentUnderTheBound {
		if !equivalent[id] {
			t.Errorf("%s is declared equivalent under the bound but did not take that branch", id)
		}
	}
}

// THE ENGINE'S INPUT IS THE CASE'S INPUT, field by field -- except the
// positions, which R2.1 re-assigned on purpose. Without this arm the two paths
// could agree on every finding while the engine fed the statute something else
// that happened to score the same.
func TestTheEngineInputMatchesTheCaseInput(t *testing.T) {
	for _, p := range load(t) {
		for _, c := range p.cases {
			t.Run(p.name+"/"+c.name, func(t *testing.T) {
				in, err := engine.Input(c.sealed(t), c.check())
				if err != nil {
					t.Fatal(err)
				}
				obs := in["observation"].(map[string]any)
				caseObs := c.caseInput["observation"].(map[string]any)
				for _, field := range []string{"actor", "of", "query", "source", "value", "role"} {
					want, inCase := caseObs[field]
					got, inEngine := obs[field]
					if inCase != inEngine {
						t.Errorf("observation.%s: present in the case %v, in the engine's input %v", field, inCase, inEngine)
						continue
					}
					if inCase && !sameJSON(want, got) {
						t.Errorf("observation.%s: engine %v, case %v", field, got, want)
					}
				}
				// `known` IS THE CASE'S known FILTERED TO THE BOUND, and the
				// filtering is the point rather than a convenience. A case
				// carries knowledge its observation's position excludes, and
				// the statute narrows it with `c.position <= ...`; the ledger
				// narrows it STRUCTURALLY by placing the tell past the seal, so
				// the engine's `known` is already the narrow set. Two of the
				// three position-bounded cases differ here, and asserting the
				// unfiltered equality would have been asserting the bound does
				// not work. Computed from the case's own claims and bound --
				// not read from the .seal.json, which records the same relation
				// and would make this agree with a file instead of with a case.
				//
				// It is also compared as a VALUE, which is why the engine
				// normalises an empty result to [] rather than leaving it null:
				// the cases carry [].
				wantKnown := caseKnownInBound(c.caseInput, c.obs.Position)
				if !sameJSON(wantKnown, in["known"]) {
					t.Errorf("known: engine %v, case filtered to bound %d %v", in["known"], c.obs.Position, wantKnown)
				}
				// Claims are compared by the fields a statute reads, as sets.
				// Positions are left out: R2.1 re-assigned them and the
				// vocabulary permits it.
				if got, want := claimKeys(in["claims"]), caseClaimKeys(c.caseInput["claims"], c.obs.Position); !reflect.DeepEqual(got, want) {
					t.Errorf("claims inside the bound: engine %v, case %v", got, want)
				}
				t.Logf("PROOF observation, %d known ref(s) and %d claim(s) agree field for field",
					len(wantKnown), len(claimKeys(in["claims"])))
			})
		}
	}
}

// claimKeys renders the engine's claims as sorted "ref|type|planted|visible_to"
// keys -- every field a v1 statute reads, and no position.
func claimKeys(v any) []string {
	var out []string
	for _, c := range v.([]map[string]any) {
		out = append(out, claimKey(c))
	}
	sort.Strings(out)
	return out
}

// caseClaimKeys renders the case's claims the same way, filtered to the case's
// own bound, since the case carries claims the observation's position excludes
// and the ledger excludes them structurally instead.
func caseClaimKeys(v any, bound int) []string {
	var out []string
	for _, raw := range v.([]any) {
		c := raw.(map[string]any)
		pos, ok := c["position"].(float64)
		if !ok || int(pos) > bound {
			continue
		}
		out = append(out, claimKey(c))
	}
	sort.Strings(out)
	return out
}

// caseKnownInBound is the case's `known` list restricted to the claims its
// observation's position admits -- v1's rule, applied to the case's own data.
// An entry naming no claim in the case is kept out and counted nowhere: the
// statute could not hold it either, since held_claims draws from input.claims.
func caseKnownInBound(caseInput map[string]any, bound int) []string {
	pos := map[string]int{}
	for _, raw := range caseInput["claims"].([]any) {
		c := raw.(map[string]any)
		if p, ok := c["position"].(float64); ok {
			pos[fmt.Sprint(c["ref"])] = int(p)
		}
	}
	out := []string{}
	for _, raw := range caseInput["known"].([]any) {
		ref := fmt.Sprint(raw)
		if p, ok := pos[ref]; ok && p <= bound {
			out = append(out, ref)
		}
	}
	sort.Strings(out)
	return out
}

func claimKey(c map[string]any) string {
	planted := false
	if p, ok := c["planted"].(bool); ok {
		planted = p
	}
	var vis []string
	switch v := c["visible_to"].(type) {
	case []string:
		vis = append(vis, v...)
	case []any:
		for _, x := range v {
			vis = append(vis, fmt.Sprint(x))
		}
	}
	sort.Strings(vis)
	return fmt.Sprintf("%v|%v|%v|%s", c["ref"], c["type"], planted, strings.Join(vis, "+"))
}

// ADR 0001 AT THE ENGINE, over every committed statute AND every mutant,
// because a mutant is a statute too.
//
// COUNTS WHAT IT EXAMINED, not only what it refused: zero refusals reads
// identically whether all 22 modules were clean or the loop never ran. And the
// control inverts -- a planted print in each module's own source must be
// refused, so a pass here is not the check failing to fire.
func TestTheEngineAcceptsEveryCommittedStatuteAndRefusesAPlantedPrint(t *testing.T) {
	examined, refused, controls := 0, 0, 0
	for _, p := range load(t) {
		modules := map[string]string{p.name + "/statute.rego": p.statute}
		for _, mp := range p.mutants {
			src, err := os.ReadFile(mp)
			if err != nil {
				t.Fatalf("%s: %v", mp, err)
			}
			modules[p.name+"/mutants/"+filepath.Base(mp)] = string(src)
		}
		names := make([]string, 0, len(modules))
		for n := range modules {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			examined++
			if err := engine.Accepts(modules[n]); err != nil {
				refused++
				t.Errorf("%s: %v", n, err)
			}
			// THE CONTROL, built from this module's own source so it is the
			// same statute in every respect but the call.
			planted := plantPrint(modules[n])
			if planted == "" {
				t.Errorf("%s: no rule body to plant a print in, so this module's control did not run", n)
				continue
			}
			controls++
			if err := engine.Accepts(planted); err == nil {
				t.Errorf("%s: a planted print() was accepted", n)
			} else if !strings.Contains(err.Error(), "print()") {
				t.Errorf("%s: the planted print was refused by the wrong clause: %v", n, err)
			}
		}
	}
	if examined != wantPackages+wantMutants {
		t.Errorf("examined %d module(s), expected %d statute(s) plus %d mutant(s)",
			examined, wantPackages, wantMutants)
	}
	if controls != examined {
		t.Errorf("%d control(s) over %d module(s): every module must have one", controls, examined)
	}
	t.Logf("PROOF %d module(s) examined, %d refused, %d planted-print control(s) all refused",
		examined, refused, controls)
}

// plantPrint inserts a print call into the first rule body in the source, and
// returns "" if it finds nowhere to put one -- which the caller reports rather
// than passing over, since a control that was never constructed proves nothing.
func plantPrint(src string) string {
	lines := strings.Split(src, "\n")
	for i, ln := range lines {
		if strings.HasSuffix(strings.TrimSpace(ln), "{") && !strings.HasPrefix(strings.TrimSpace(ln), "#") {
			out := append([]string{}, lines[:i+1]...)
			out = append(out, "\tprint(\"planted by the harness\")")
			out = append(out, lines[i+1:]...)
			return strings.Join(out, "\n")
		}
	}
	return ""
}
