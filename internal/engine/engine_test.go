package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/open-policy-agent/opa/v1/ast"

	"forge.axonibyte.com/axonibyte/rumor/internal/ledger"
)

// The fixtures are the COMMITTED ledger files from R2.1, not ledgers this test
// builds. A test that assembled its own input would be checking the adapter
// against the same assumptions the adapter holds; these files were written by
// another run, from cases written before either existed.
const examplesDir = "../../examples"

type seal struct {
	Bound         int      `json:"bound"`
	Observer      string   `json:"observer"`
	Seal          int      `json:"seal"`
	KnownInPrefix []string `json:"known_in_prefix"`
	OwnClaims     []string `json:"own_claims_need_no_tell"`
}

// fixture loads one example's ledger and its seal record, sealed at the
// RECORDED position. Sealing at the end of the file instead would quietly
// erase the three position-bounded cases, whose whole point is that facts sit
// past the seal.
func fixture(t *testing.T, pkg, name string) (*ledger.Ledger, seal, string) {
	t.Helper()
	base := filepath.Join(examplesDir, pkg, "ledger", name)
	raw, err := os.ReadFile(base + ".jsonl")
	if err != nil {
		t.Fatalf("%s: %v", base+".jsonl", err)
	}
	l, err := ledger.Load(strings.NewReader(string(raw)), false)
	if err != nil {
		t.Fatalf("the committed ledger does not load: %v", err)
	}
	var s seal
	sraw, err := os.ReadFile(base + ".seal.json")
	if err != nil {
		t.Fatalf("%s: %v", base+".seal.json", err)
	}
	if err := json.Unmarshal(sraw, &s); err != nil {
		t.Fatalf("%s: %v", base+".seal.json", err)
	}
	if err := l.SealAt(s.Seal); err != nil {
		t.Fatalf("sealing at the recorded %d: %v", s.Seal, err)
	}
	st, err := os.ReadFile(filepath.Join(examplesDir, pkg, "statute.rego"))
	if err != nil {
		t.Fatalf("%s: %v", pkg, err)
	}
	return l, s, string(st)
}

func claimRefs(t *testing.T, in map[string]any) []string {
	t.Helper()
	var out []string
	for _, c := range in["claims"].([]map[string]any) {
		out = append(out, c["ref"].(string))
	}
	return out
}

// AN UNSEALED LEDGER IS REFUSED, and the verdict is read off the clause that
// answered rather than off "err != nil": every other refusal in Input also
// returns an error, so a test that only checked for one would pass if the
// actor check fired instead.
func TestInputRefusesAnUnsealedLedger(t *testing.T) {
	l := ledger.New(false)
	if _, err := l.Append(ledger.Fact{Kind: ledger.KindClaim, Actor: "alice",
		Type: "user.created", Ref: "alice.user#1"}); err != nil {
		t.Fatal(err)
	}
	_, err := Input(l, Check{Actor: "alice", Query: "user.count", Of: "user.created", Source: ledger.SourceAPI, Value: 1})
	if err == nil {
		t.Fatal("an unsealed ledger was accepted: a check would then report a verdict over facts still arriving")
	}
	if !strings.Contains(err.Error(), "unsealed") {
		t.Fatalf("refused, but not by the sealing clause: %v", err)
	}
	// THE PERMITTED TWIN, so the refusal is not just a broken fixture: the same
	// ledger, sealed, is accepted.
	if err := l.SealAt(1); err != nil {
		t.Fatal(err)
	}
	if _, err := Input(l, Check{Actor: "alice", Query: "user.count", Of: "user.created", Source: ledger.SourceAPI, Value: 1}); err != nil {
		t.Fatalf("the same ledger, sealed, was refused: %v", err)
	}
}

func TestInputRefusesACheckWithNoActor(t *testing.T) {
	l, _, _ := fixture(t, "planted-lie", "honest-only")
	_, err := Input(l, Check{Query: "user.count", Of: "user.created", Source: ledger.SourceAPI, Value: 1})
	if err == nil {
		t.Fatal("a check with no actor was accepted, and a finding is always some actor's")
	}
	if !strings.Contains(err.Error(), "names no actor") {
		t.Fatalf("refused, but not by the actor clause: %v", err)
	}
}

// THE BOUND IS THE SEAL, asserted from both sides. One side alone proves
// nothing: that a claim is absent could mean the adapter dropped it, and that
// it is present under a later seal could mean the seal is ignored.
func TestInputReadsOnlyTheSealedPrefix(t *testing.T) {
	l, s, _ := fixture(t, "transitive-count", "charlie-position-bounded")
	in, err := Input(l, Check{Actor: s.Observer, Query: "user.count", Of: "user.created", Source: ledger.SourceAPI, Value: 1})
	if err != nil {
		t.Fatal(err)
	}
	refs := claimRefs(t, in)
	if len(refs) != 1 || refs[0] != "alice.user#1" {
		t.Fatalf("at the seal %d the prefix should hold alice.user#1 alone, got %v", s.Seal, refs)
	}
	if l.Len() <= s.Seal {
		t.Fatalf("this fixture has nothing past its seal, so it cannot show a bound: %d fact(s), seal %d", l.Len(), s.Seal)
	}

	// The same ledger sealed past bob's claim admits it. The seal is therefore
	// what excluded it, not the adapter.
	l2, _, _ := fixture(t, "transitive-count", "charlie-position-bounded")
	if err := l2.SealAt(l2.Len()); err != nil {
		t.Fatal(err)
	}
	in2, err := Input(l2, Check{Actor: s.Observer, Query: "user.count", Of: "user.created", Source: ledger.SourceAPI, Value: 1})
	if err != nil {
		t.Fatal(err)
	}
	refs2 := claimRefs(t, in2)
	if len(refs2) != 2 {
		t.Fatalf("sealed at %d the prefix should hold both claims, got %v", l2.Len(), refs2)
	}
	t.Logf("PROOF seal %d admits %v; seal %d admits %v", s.Seal, refs, l2.Len(), refs2)
}

func TestTheObservationSitsJustPastTheSeal(t *testing.T) {
	l, s, _ := fixture(t, "role-visibility", "position-bounded")
	in, err := Input(l, Check{Actor: s.Observer, Role: "admin", Query: "doc.count",
		Of: "doc.created", Source: ledger.SourceAPI, Value: 1})
	if err != nil {
		t.Fatal(err)
	}
	obs := in["observation"].(map[string]any)
	if obs["position"] != s.Seal+1 {
		t.Fatalf("the observation sits at %v, not just past the seal %d", obs["position"], s.Seal)
	}
	// AND NOTHING IN THE PREFIX IS AT OR AFTER IT, which is the property the
	// statutes' `c.position <= input.observation.position` bound relies on.
	for _, c := range in["claims"].([]map[string]any) {
		if c["position"].(int) >= obs["position"].(int) {
			t.Errorf("claim %v sits at %v, at or past the observation at %v",
				c["ref"], c["position"], obs["position"])
		}
	}
	t.Logf("PROOF seal %d, observation at %v, %d claim(s) all before it",
		s.Seal, obs["position"], len(in["claims"].([]map[string]any)))
}

// An observer's OWN claims need no tell -- v1's rule, and the reason the
// with-planted fixture carries no `told` fact at all.
func TestAnObserverKnowsItsOwnClaimsWithNoTell(t *testing.T) {
	l, s, _ := fixture(t, "planted-lie", "with-planted")
	for _, f := range l.SealedPrefix() {
		if f.Kind == ledger.KindTold {
			t.Fatalf("this fixture has a tell at position %d, so it cannot show the own-claims rule", f.Position)
		}
	}
	in, err := Input(l, Check{Actor: s.Observer, Query: "user.count", Of: "user.created", Source: ledger.SourceAPI, Value: 1})
	if err != nil {
		t.Fatal(err)
	}
	got := in["known"].([]string)
	if strings.Join(got, ",") != strings.Join(s.KnownInPrefix, ",") {
		t.Fatalf("knowledge %v is not the recorded %v", got, s.KnownInPrefix)
	}
	t.Logf("PROOF %s knows %v from %d claim(s) and no tell", s.Observer, got, len(in["claims"].([]map[string]any)))
}

// `planted` and `visible_to` reach the statute only where the ledger carries
// them. Both halves are asserted: present where the file has them, absent
// where it does not.
func TestPlantedAndVisibleToTravelOnlyWhenPresent(t *testing.T) {
	l, s, _ := fixture(t, "planted-lie", "with-planted")
	in, err := Input(l, Check{Actor: s.Observer, Query: "user.count", Of: "user.created", Source: ledger.SourceAPI, Value: 1})
	if err != nil {
		t.Fatal(err)
	}
	planted, clean := 0, 0
	for _, c := range in["claims"].([]map[string]any) {
		if _, ok := c["visible_to"]; ok {
			t.Errorf("claim %v carries visible_to, which this example's ledger does not", c["ref"])
		}
		if v, ok := c["planted"]; ok {
			if v != true {
				t.Errorf("claim %v carries planted=%v: the key is emitted only when true", c["ref"], v)
			}
			planted++
		} else {
			clean++
		}
	}
	if planted != 1 || clean != 1 {
		t.Fatalf("expected one planted and one honest claim, got %d and %d", planted, clean)
	}

	rl, rs, _ := fixture(t, "role-visibility", "viewer-untold-explained")
	rin, err := Input(rl, Check{Actor: rs.Observer, Role: "viewer", Query: "doc.count",
		Of: "doc.created", Source: ledger.SourceAPI, Value: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range rin["claims"].([]map[string]any) {
		if _, ok := c["visible_to"]; !ok {
			t.Errorf("claim %v reached the statute with no visible_to, so its role check cannot hold", c["ref"])
		}
	}
	t.Logf("PROOF planted on 1 of 2 claims; visible_to on %d of %d in role-visibility",
		len(rin["claims"].([]map[string]any)), len(rin["claims"].([]map[string]any)))
}

// findingRows reads the rows of every `finding` definition out of the statute,
// so the test below compares against the statute's own account of itself
// rather than against magic numbers that a deliberate edit would silently
// invalidate.
func findingRows(t *testing.T, statute string) map[int]bool {
	t.Helper()
	m, err := ast.ParseModule("statute.rego", statute)
	if err != nil {
		t.Fatalf("statute does not parse: %v", err)
	}
	rows := map[int]bool{}
	for _, r := range m.Rules {
		if r.Head != nil && string(r.Head.Name) == FindingRule && r.Loc() != nil {
			rows[r.Loc().Row] = r.Default
		}
	}
	if len(rows) < 2 {
		t.Fatalf("this statute has %d definition(s) of %s, so naming one proves nothing", len(rows), FindingRule)
	}
	return rows
}

// A FINDING NAMES THE DEFINITION THAT PRODUCED IT. The assertion that matters
// is not that a name comes back -- `data.rumor.statute.finding` always would --
// but that DIFFERENT findings name DIFFERENT definitions, and that a `bug`
// names the default.
func TestAFindingNamesTheDefinitionThatFired(t *testing.T) {
	ctx := context.Background()
	type want struct {
		pkg, name, of, query, role string
		value                      any
		isDefault                  bool
	}
	cases := []want{
		{"planted-lie", "honest-only", "user.created", "user.count", "", "consistent", false},
		{"planted-lie", "with-planted", "user.created", "user.count", "", "bug", true},
		{"transitive-count", "charlie-explained", "user.created", "user.count", "", "explained", false},
		{"transitive-count", "charlie-bug", "user.created", "user.count", "", "bug", true},
		{"role-visibility", "viewer-untold-explained", "doc.created", "doc.count", "viewer", "explained", false},
	}
	byPkg := map[string]map[string][]int{}
	for _, c := range cases {
		t.Run(c.pkg+"/"+c.name, func(t *testing.T) {
			l, s, st := fixture(t, c.pkg, c.name)
			value := caseValue(t, c.pkg, c.name)
			f, err := Evaluate(ctx, st, []string{FindingRule}, l,
				Check{Actor: s.Observer, Role: c.role, Query: c.query, Of: c.of,
					Source: ledger.SourceAPI, Value: value})
			if err != nil {
				t.Fatal(err)
			}
			if !f.Defined || f.Value != c.value {
				t.Fatalf("finding %v (defined %v), wanted %v", f.Value, f.Defined, c.value)
			}
			if len(f.Fired) != 1 {
				t.Fatalf("%d definition(s) fired, so the finding does not name one: %v", len(f.Fired), f.Fired)
			}
			rows := findingRows(t, st)
			isDefault, known := rows[f.Fired[0].Row]
			if !known {
				t.Fatalf("row %d is not a definition of %s in this statute: %v", f.Fired[0].Row, FindingRule, rows)
			}
			if isDefault != c.isDefault || f.Fired[0].Default != c.isDefault {
				t.Errorf("row %d: default is %v in the statute and %v in the finding, wanted %v",
					f.Fired[0].Row, isDefault, f.Fired[0].Default, c.isDefault)
			}
			if byPkg[c.pkg] == nil {
				byPkg[c.pkg] = map[string][]int{}
			}
			byPkg[c.pkg][c.value.(string)] = append(byPkg[c.pkg][c.value.(string)], f.Fired[0].Row)
			t.Logf("PROOF %v from %s", f.Value, f.Fired[0])
		})
	}
	// THE ARM THAT STOPS THE ABOVE PASSING VACUOUSLY: inside one package, two
	// different findings must come from two different rows. A reader that
	// returned the same row every time would satisfy every assertion above.
	for pkg, byValue := range byPkg {
		if len(byValue) < 2 {
			continue
		}
		seen := map[int]string{}
		for value, rows := range byValue {
			for _, r := range rows {
				if other, dup := seen[r]; dup && other != value {
					t.Errorf("%s: %q and %q both come from row %d, so the row distinguishes nothing",
						pkg, other, value, r)
				}
				seen[r] = value
			}
		}
	}
}

// caseValue takes the observed VALUE from the case file rather than from this
// test, so the check the engine judges is the one the example authored.
func caseValue(t *testing.T, pkg, name string) any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(examplesDir, pkg, "cases", name+".json"))
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var doc struct {
		Input struct {
			Observation struct {
				Value any `json:"value"`
			} `json:"observation"`
		} `json:"input"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if doc.Input.Observation.Value == nil {
		t.Fatalf("%s/%s carries no observed value", pkg, name)
	}
	return doc.Input.Observation.Value
}

func TestTheTraceIsRetainedAndNamesTheRuleItEvaluated(t *testing.T) {
	l, s, st := fixture(t, "transitive-count", "charlie-bug")
	f, err := Evaluate(context.Background(), st, []string{"believed", "truth", "untold_accounts", FindingRule}, l,
		Check{Actor: s.Observer, Query: "user.count", Of: "user.created",
			Source: ledger.SourceAPI, Value: caseValue(t, "transitive-count", "charlie-bug")})
	if err != nil {
		t.Fatal(err)
	}
	if f.TraceEvents == 0 || f.Trace == "" {
		t.Fatal("the trace is empty, so nothing re-derives this finding (R10)")
	}
	if !strings.Contains(f.Trace, FindingRule) {
		t.Fatalf("the trace never names %s, so it is not a trace of this query", FindingRule)
	}
	if f.Covered == 0 {
		t.Fatal("no covered range, which cannot be true of a statute that just produced a verdict")
	}
	// Every rule the caller named came back: an absent one would mean the
	// finding is reported beside a sub-rule the statute never evaluated.
	for _, r := range []string{"believed", "truth", "untold_accounts", FindingRule} {
		if _, ok := f.Rules[r]; !ok {
			t.Errorf("rule %s is absent from the result", r)
		}
	}
	t.Logf("PROOF %d trace event(s), %d covered range(s), %.0f%%, %d rule(s) returned",
		f.TraceEvents, f.Covered, f.Coverage, len(f.Rules))
}

// An undefined rule is ABSENT, not zero. The distinction is the one ratchet
// measured in go-port/3, and reporting a zero would make "the rule did not
// hold" indistinguishable from "it held and counted nothing".
func TestAnUndefinedRuleIsAbsentAndNotZero(t *testing.T) {
	l, s, _ := fixture(t, "planted-lie", "honest-only")
	const st = `package rumor.statute

finding := "consistent" if input.observation.value == 1

never_holds := 1 if false
`
	f, err := Evaluate(context.Background(), st, []string{FindingRule, "never_holds"}, l,
		Check{Actor: s.Observer, Query: "user.count", Of: "user.created", Source: ledger.SourceAPI, Value: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := f.Rules["never_holds"]; ok {
		t.Errorf("an undefined rule came back as %v rather than absent", f.Rules["never_holds"])
	}
	if f.Rules[FindingRule] != "consistent" {
		t.Errorf("the defined rule did not come back: %v", f.Rules)
	}
}

// allFixtures names every committed ledger fixture, as (package, case) pairs.
// Derived from the tree rather than listed here: a hand-written list would
// quietly stop covering a case somebody added.
func allFixtures(t *testing.T) [][2]string {
	t.Helper()
	g, err := filepath.Glob(filepath.Join(examplesDir, "*", "ledger", "*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var out [][2]string
	for _, p := range g {
		out = append(out, [2]string{
			filepath.Base(filepath.Dir(filepath.Dir(p))),
			strings.TrimSuffix(filepath.Base(p), ".jsonl"),
		})
	}
	if len(out) != 29 {
		t.Fatalf("expected the 29 committed fixtures, found %d: an empty or short walk is not a pass", len(out))
	}
	return out
}

// THE PREMISE BEHIND THE FLATTENER'S BOUND, asserted rather than inferred.
// Input passes the seal to flatten.Known even though prefixFacts has already
// applied it; mutating that argument upward kills nothing, so the mutant is
// equivalent and the honest thing is to state and check the premise it rests
// on: nothing handed to the flattener sits past the seal. Checked across all
// 29 fixtures, including the three whose files hold facts beyond it.
func TestNothingHandedToTheFlattenerSitsPastTheSeal(t *testing.T) {
	beyond := 0
	for _, f := range allFixtures(t) {
		l, s, _ := fixture(t, f[0], f[1])
		fl, claims := prefixFacts(l)
		for _, c := range fl.Claims {
			if c.Position > s.Seal {
				t.Errorf("%s/%s: claim %s at %d is past the seal %d", f[0], f[1], c.Ref, c.Position, s.Seal)
			}
		}
		for _, tl := range fl.Tells {
			if tl.Position > s.Seal {
				t.Errorf("%s/%s: tell at %d is past the seal %d", f[0], f[1], tl.Position, s.Seal)
			}
		}
		if len(claims) != len(fl.Claims) {
			t.Errorf("%s/%s: %d claim(s) to the statute but %d to the flattener: one walk, two answers",
				f[0], f[1], len(claims), len(fl.Claims))
		}
		if l.Len() > s.Seal {
			beyond++
		}
	}
	// Without a fixture that HAS facts past its seal, the loop above would
	// pass over 22 ledgers none of which could violate it.
	if beyond == 0 {
		t.Error("no fixture holds a fact past its seal, so this arm proved nothing")
	}
	t.Logf("PROOF 22 fixture(s) examined, %d of them holding facts past the seal, 0 leaked to the flattener", beyond)
}

// THE ENGINE REFUSES A STATUTE THAT CALLS print(), and the control inverts:
// the same statute with the call removed is accepted. Without the twin, a
// refusal could be any other defect in the module.
//
// WHY THIS IS NOT A COPY OF tests/statute's CHECK. That one asks whether the
// committed statutes and mutants call print, with a static scan and a runtime
// hook. This one asks a question it cannot: whether the evaluator that runs
// statutes in production lets one through. It would -- Evaluate passes no
// rego.EnablePrintStatements, so OPA erases the call and the statute evaluates
// clean. Measured below, which is why the refusal is static and not a hook.
func TestTheEngineRefusesAStatuteThatPrints(t *testing.T) {
	l, s, _ := fixture(t, "planted-lie", "honest-only")
	chk := Check{Actor: s.Observer, Query: "user.count", Of: "user.created",
		Source: ledger.SourceAPI, Value: 1}

	const withPrint = `package rumor.statute

finding := "consistent" if {
	print("the observer knows", input.known)
	input.observation.value == 1
}
`
	const withoutPrint = `package rumor.statute

finding := "consistent" if input.observation.value == 1
`
	_, err := Evaluate(context.Background(), withPrint, []string{FindingRule}, l, chk)
	if err == nil {
		t.Fatal("a statute calling print() was evaluated: ADR 0001 forbids it and this evaluator would erase the call")
	}
	if !strings.Contains(err.Error(), "print()") {
		t.Fatalf("refused, but not by the ADR 0001 clause: %v", err)
	}
	if err := Accepts(withPrint); err == nil {
		t.Error("Accepts admitted a statute that calls print()")
	}

	f, err := Evaluate(context.Background(), withoutPrint, []string{FindingRule}, l, chk)
	if err != nil {
		t.Fatalf("the same statute without the call was refused: %v", err)
	}
	if f.Value != "consistent" {
		t.Fatalf("the permitted twin yielded %v", f.Value)
	}
	if err := Accepts(withoutPrint); err != nil {
		t.Errorf("Accepts refused a statute with no print: %v", err)
	}

	// AND THE MEASUREMENT THAT MAKES THE REFUSAL NECESSARY rather than
	// decorative: with the refusal bypassed, the printing statute evaluates to
	// the same finding and says nothing, which is exactly the silence ADR 0001
	// exists to prevent.
	v, defined, err := evalOne(context.Background(), withPrint, FindingRule, f.Input)
	if err != nil {
		t.Fatalf("evaluating the printing statute directly: %v", err)
	}
	if !defined || v != "consistent" {
		t.Fatalf("expected the printing statute to evaluate clean and silent, got %v (defined %v)", v, defined)
	}
	t.Logf("PROOF bypassing the refusal, the printing statute yields %v with no diagnostic at all", v)
}

// AN UNDEFINED FINDING IS NOT A FINDING OF NIL, and Says is where that is kept.
//
// Both directions, because a renderer that always said "no finding" would pass
// a one-sided arm: the default-less statute must reach Defined==false and say
// so, and the statute that defines one must say its value. The default-less
// statute is written here rather than added to examples/ -- the committed
// corpus is counted by `rumor attaint` (3 packages, 17 cases) and an extra
// package would move numbers that have nothing to do with this.
func TestAnUndefinedFindingSaysSoRatherThanNil(t *testing.T) {
	l, s, _ := fixture(t, "planted-lie", "honest-only")
	chk := Check{Actor: s.Observer, Query: "user.count", Of: "user.created",
		Source: ledger.SourceAPI, Value: 1}

	// No `default finding`, and a body that cannot hold for this observation.
	const noDefault = `package rumor.statute

finding := "bug" if input.observation.value == 99999
`
	const withDefault = `package rumor.statute

default finding := "bug"

finding := "consistent" if input.observation.value == 1
`
	f, err := Evaluate(context.Background(), noDefault, []string{FindingRule}, l, chk)
	if err != nil {
		t.Fatalf("a statute with no default was refused: %v", err)
	}
	if f.Defined {
		t.Fatalf("the statute defined a finding after all (%v): the fixture no longer reaches the state under test", f.Value)
	}
	if got := f.Says(); got != "no finding" {
		t.Errorf("Says() on an undefined finding is %q; a reader needs an absence, and %%v of Value would have printed <nil>", got)
	}
	if strings.Contains(f.Says(), "nil") {
		t.Errorf("Says() leaked a nil rendering: %q", f.Says())
	}

	g, err := Evaluate(context.Background(), withDefault, []string{FindingRule}, l, chk)
	if err != nil {
		t.Fatalf("the twin with a default was refused: %v", err)
	}
	if !g.Defined {
		t.Fatal("the twin carries a default and still came back undefined")
	}
	if got := g.Says(); got != "consistent" {
		t.Errorf("Says() on a defined finding is %q, wanted the value itself", got)
	}
	t.Logf("PROOF Says() answers %q undefined and %q defined", f.Says(), g.Says())
}
