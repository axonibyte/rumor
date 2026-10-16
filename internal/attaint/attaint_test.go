package attaint

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// THE DRILLS DO NOT REACH THESE ARMS, which is why they are here rather than in
// tests/attaint. Every drill is a damaged copy of the REAL tree, so it always
// has three packages, seventeen cases and nineteen mutants -- it can never
// exercise "no package", "no case", "no mutant" or "no rule". Those four are
// the guards that make the other arms mean anything, and an unfired guard is
// indistinguishable from an absent one.

// pkgSpec builds one synthetic example package on disk.
type pkgSpec struct {
	name    string
	statute string
	rules   []string
	cases   map[string]caseSpec
	mutants map[string]string
}

type caseSpec struct {
	expect map[string]any
	ledger string
	seal   int
	actor  string
	query  string
	of     string
	source string
	value  any
	known  []string
	claims []map[string]any
	// outOfBound appears in the CASE input and never in the ledger: a claim
	// past the observation's position. The clean statute excludes it by its
	// position guard, so a mutant that drops the guard moves a rule FROM THE
	// CASES; the sealed prefix never held it, so that same mutant is identical
	// FROM THE LEDGER. It is the structural-bound shape R2.3 found in the real
	// tree, reproduced at this scale so the ledger-inert reporting path is
	// exercised here and not only against examples/.
	outOfBound []map[string]any
}

// A statute small enough to read, with the two rules the corroboration arm
// needs and a clause a mutant can remove.
const miniStatute = `package rumor.statute

held_claims contains c if {
	some c in input.claims
	c.ref in input.known
	c.position <= input.observation.position
}

believed := count({c.ref | some c in held_claims; c.type == input.observation.of})

default finding := "bug"

finding := "consistent" if input.observation.value == believed

default rests_on_planted := false

rests_on_planted if {
	finding == "bug"
	some c in held_claims
	c.planted
	c.type == input.observation.of
}
`

// miniMutant2 forces the finding, which moves `finding` and -- because the
// control requires a bug -- `rests_on_planted` with it. Two mutants are needed
// for three rules, which is the point the per-rule arm makes: one mutant
// battering one rule leaves the rest unproven.
const miniMutant2 = `package rumor.statute

held_claims contains c if {
	some c in input.claims
	c.ref in input.known
	c.position <= input.observation.position
}

believed := count({c.ref | some c in held_claims; c.type == input.observation.of})

default finding := "consistent"  # MUTANT: never a bug

default rests_on_planted := false

rests_on_planted if {
	finding == "bug"
	some c in held_claims
	c.planted
	c.type == input.observation.of
}
`

// miniMutant3 drops the position guard. Against the LEDGER it changes nothing
// -- the sealed prefix holds only in-bound facts -- and against the CASE it
// admits the out-of-bound claim and moves `believed`. So it is alive, inert on
// the ledger, and reported as such.
const miniMutant3 = `package rumor.statute

held_claims contains c if {
	some c in input.claims
	c.ref in input.known
}

believed := count({c.ref | some c in held_claims; c.type == input.observation.of})

default finding := "bug"

finding := "consistent" if input.observation.value == believed

default rests_on_planted := false

rests_on_planted if {
	finding == "bug"
	some c in held_claims
	c.planted
	c.type == input.observation.of
}
`

const miniMutant = `package rumor.statute

held_claims contains c if {
	some c in input.claims
	c.ref in input.known
	c.position <= input.observation.position
}

believed := 99  # MUTANT

default finding := "bug"

finding := "consistent" if input.observation.value == believed

default rests_on_planted := false

rests_on_planted if {
	finding == "bug"
	some c in held_claims
	c.planted
	c.type == input.observation.of
}
`

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func build(t *testing.T, specs ...pkgSpec) string {
	t.Helper()
	root := t.TempDir()
	for _, s := range specs {
		dir := filepath.Join(root, s.name)
		write(t, filepath.Join(dir, "statute.rego"), s.statute)
		rj, err := json.Marshal(s.rules)
		if err != nil {
			t.Fatal(err)
		}
		write(t, filepath.Join(dir, "rules.json"), string(rj))
		for id, body := range s.mutants {
			write(t, filepath.Join(dir, "mutants", id+".rego"), body)
		}
		for name, c := range s.cases {
			doc := map[string]any{
				"expect": c.expect,
				"input": map[string]any{
					"observation": map[string]any{
						"actor": c.actor, "query": c.query, "of": c.of,
						"source": c.source, "value": c.value, "position": c.seal + 1,
					},
					"known":  append(append([]string{}, c.known...), outOfBoundRefs(c.outOfBound)...),
					"claims": append(append([]map[string]any{}, c.claims...), c.outOfBound...),
				},
			}
			b, err := json.MarshalIndent(doc, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			write(t, filepath.Join(dir, "cases", name+".json"), string(b))
			write(t, filepath.Join(dir, "ledger", name+".jsonl"), c.ledger)
			write(t, filepath.Join(dir, "ledger", name+".seal.json"),
				`{"seal": `+itoa(c.seal)+`}`)
		}
	}
	return root
}

func outOfBoundRefs(cs []map[string]any) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		if r, ok := c["ref"].(string); ok {
			out = append(out, r)
		}
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// healthy is a package that passes every arm: a planted lie that produces bug
// and rests on the lie, and a mutant that moves a rule.
func healthy() pkgSpec {
	return pkgSpec{
		name:    "mini",
		statute: miniStatute,
		rules:   []string{"believed", "finding", "rests_on_planted"},
		mutants: map[string]string{
			"m1-believed-constant": miniMutant,
			"m2-never-a-bug":       miniMutant2,
			"m3-ignores-position":  miniMutant3,
		},
		cases: map[string]caseSpec{
			"lie": {
				expect: map[string]any{"believed": 2, "finding": "bug", "rests_on_planted": true},
				ledger: `{"kind":"claim","position":1,"actor":"a","type":"t","ref":"r1","handles":["r1"]}
{"kind":"claim","position":2,"actor":"a","type":"t","ref":"r2","handles":["r2"],"planted":true}
`,
				seal: 2, actor: "a", query: "q", of: "t", source: "api", value: 1,
				known: []string{"r1", "r2"},
				claims: []map[string]any{
					{"ref": "r1", "actor": "a", "type": "t", "position": 1, "handle": "r1"},
					{"ref": "r2", "actor": "a", "type": "t", "position": 2, "handle": "r2", "planted": true},
				},
				outOfBound: []map[string]any{
					{"ref": "r9", "actor": "a", "type": "t", "position": 9, "handle": "r9"},
				},
			},
		},
	}
}

func run(t *testing.T, dir string) Report {
	t.Helper()
	var out bytes.Buffer
	rep, err := Run(context.Background(), dir, &out)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return rep
}

func only(t *testing.T, rep Report, want Clause) Refusal {
	t.Helper()
	if len(rep.Refusals) != 1 {
		for _, r := range rep.Refusals {
			t.Errorf("  %s", r)
		}
		t.Fatalf("%d refusal(s), wanted exactly 1 (%s)", len(rep.Refusals), want)
	}
	if rep.Refusals[0].Clause != want {
		t.Fatalf("refused by %q, wanted %q", rep.Refusals[0].Clause, want)
	}
	return rep.Refusals[0]
}

// THE POSITIVE CONTROL, and every refusal below is measured against it. Without
// it a guard that fires on everything would look like a guard that works.
func TestTheHealthyPackagePasses(t *testing.T) {
	rep := run(t, build(t, healthy()))
	if !rep.OK() {
		for _, r := range rep.Refusals {
			t.Errorf("  %s", r)
		}
		t.Fatalf("the healthy package refused %d time(s)", len(rep.Refusals))
	}
	if rep.Corroborated != 1 || rep.Controls != 1 || rep.PlantedCases != 1 {
		t.Fatalf("corroborated %d, controls %d, planted %d; wanted 1/1/1",
			rep.Corroborated, rep.Controls, rep.PlantedCases)
	}
	if rep.KilledRules != rep.Rules {
		t.Fatalf("killed %d of %d rule(s); the healthy package must have every rule attacked",
			rep.KilledRules, rep.Rules)
	}
	t.Logf("PROOF healthy: %d rule(s) killed, %d finding(s) reached, %d control(s) corroborated",
		rep.KilledRules, rep.Findings, rep.Corroborated)
}

func TestAnEmptyDirectoryIsRefusedAndNotPassed(t *testing.T) {
	rep := run(t, t.TempDir())
	r := only(t, rep, ClauseNoPackages)
	if rep.Packages != 0 {
		t.Fatalf("Packages %d", rep.Packages)
	}
	t.Logf("PROOF %s", r)
}

func TestAPackageWithNoCasesIsRefused(t *testing.T) {
	p := healthy()
	p.cases = nil
	rep := run(t, build(t, p))
	// Rules and mutants are present, so this isolates the case guard.
	if rep.Cases != 0 || rep.Mutants == 0 || rep.Rules == 0 {
		t.Fatalf("cases %d mutants %d rules %d", rep.Cases, rep.Mutants, rep.Rules)
	}
	t.Logf("PROOF %s", only(t, rep, ClauseNoCases))
}

func TestAPackageWithNoMutantsIsRefused(t *testing.T) {
	p := healthy()
	p.mutants = nil
	rep := run(t, build(t, p))
	if rep.Mutants != 0 || rep.Cases == 0 {
		t.Fatalf("mutants %d cases %d", rep.Mutants, rep.Cases)
	}
	t.Logf("PROOF %s", only(t, rep, ClauseNoMutants))
}

func TestAPackageDeclaringNoRulesIsRefused(t *testing.T) {
	p := healthy()
	p.rules = []string{}
	rep := run(t, build(t, p))
	if rep.Rules != 0 {
		t.Fatalf("rules %d", rep.Rules)
	}
	t.Logf("PROOF %s", only(t, rep, ClauseNoRules))
}

// A planted lie the statute does not notice. The ledger carries the tag and the
// observation AGREES with what the actor believes, so the finding is
// `consistent` and the lie passes unseen.
func TestAPlantedLieThatDoesNotProduceBugIsRefused(t *testing.T) {
	p := healthy()
	c := p.cases["lie"]
	c.value = 2 // agrees with `believed`, so the statute says consistent
	c.expect = map[string]any{"believed": 2, "finding": "consistent", "rests_on_planted": false}
	p.cases["lie"] = c
	rep := run(t, build(t, p))
	var found bool
	for _, r := range rep.Refusals {
		if r.Clause == ClauseLieNotFound {
			found = true
			t.Logf("PROOF %s", r)
		}
	}
	if !found {
		for _, r := range rep.Refusals {
			t.Errorf("  %s", r)
		}
		t.Fatalf("a planted claim produced %q and no refusal named it", "consistent")
	}
	if rep.PlantedCases != 1 {
		t.Fatalf("PlantedCases %d, wanted 1: the guard must have SEEN the lie to refuse it", rep.PlantedCases)
	}
}

// A corpus with a planted claim and no case declaring a control: the arm that
// would otherwise pass for free.
func TestACorpusWithNoControlIsRefused(t *testing.T) {
	p := healthy()
	c := p.cases["lie"]
	c.expect = map[string]any{"believed": 2, "finding": "bug", "rests_on_planted": false}
	p.cases["lie"] = c
	rep := run(t, build(t, p))
	r := only(t, rep, ClauseNoControlDeclared)
	if rep.PlantedCases != 1 {
		t.Fatalf("PlantedCases %d: the refusal must report the lie it DID find", rep.PlantedCases)
	}
	if !strings.Contains(r.Detail, "1 case(s) carry a planted claim") {
		t.Fatalf("the refusal does not report what it found: %s", r.Detail)
	}
	t.Logf("PROOF %s", r)
}

// A rule nothing attacks. The mutant moves `believed`, so adding a rule no
// mutant touches isolates the per-rule arm.
func TestARuleNoMutantMovesIsNamed(t *testing.T) {
	p := healthy()
	p.statute += "\nuntouched := 7\n"
	for id := range p.mutants {
		p.mutants[id] += "\nuntouched := 7\n"
	}
	p.rules = append(p.rules, "untouched")
	c := p.cases["lie"]
	c.expect["untouched"] = 7
	p.cases["lie"] = c
	rep := run(t, build(t, p))
	r := only(t, rep, ClauseRuleUnmutated)
	if len(rep.Unmutated) != 1 || rep.Unmutated[0] != "mini/untouched" {
		t.Fatalf("Unmutated %v, wanted [mini/untouched]: the rule must be NAMED and not only counted", rep.Unmutated)
	}
	t.Logf("PROOF %s", r)
}

// A mutant the statute has absorbed: here by making the mutant a byte-for-byte
// copy of the statute, which is what a weakening converges on.
func TestAMutantIdenticalToItsStatuteIsNamed(t *testing.T) {
	p := healthy()
	p.mutants["m2-absorbed"] = miniStatute
	rep := run(t, build(t, p))
	r := only(t, rep, ClauseMutantDead)
	if len(rep.DeadMutants) != 1 || rep.DeadMutants[0] != "mini/m2-absorbed" {
		t.Fatalf("DeadMutants %v", rep.DeadMutants)
	}
	t.Logf("PROOF %s", r)
}

// A finding the package can no longer produce.
func TestAnUnreachableFindingIsNamed(t *testing.T) {
	p := healthy()
	c := p.cases["lie"]
	// The case asks for a finding this statute has no clause for at all.
	c.expect = map[string]any{"believed": 2, "finding": "explained", "rests_on_planted": true}
	p.cases["lie"] = c
	rep := run(t, build(t, p))
	var found bool
	for _, r := range rep.Refusals {
		if r.Clause == ClauseFindingUnreachable {
			found = true
			t.Logf("PROOF %s", r)
		}
	}
	if !found {
		for _, r := range rep.Refusals {
			t.Errorf("  %s", r)
		}
		t.Fatalf("no refusal named the unreachable finding")
	}
}

// EVERY DECLARED CLAUSE IS DRIVEN, AND THE DECLARED SET IS READ FROM THE
// SOURCE RATHER THAN RETYPED.
//
// quill r3/5 found the previous version of this test doing none of that. It
// held a HAND LIST of eleven clauses whose only check was `len(all) != 11`
// against a literal, and its body asserted exactly ONE of them -- `seen` was
// filled from a single run and compared for one clause, while `all` was used
// for nothing but its length. A twelfth clause would have left the list at
// eleven, passed the check, and gone unexercised and invisible. The test's own
// comment named the state it was in: "a clause that stopped firing would
// otherwise be invisible".
//
// THE FIX IS THE INSTRUMENT ALREADY IN THIS COMMIT, applied to the set it was
// not applied to. tests/attaint reads tests/engine's equivalence declaration
// through go/ast for exactly this reason; here the same reader takes the
// `Clause` consts out of attaint.go, and the EXERCISED set must equal the
// DECLARED set in BOTH directions -- a new clause fails as declared-and-not-
// driven, a deleted one as driven-and-not-declared.
func TestEveryClauseIsExercised(t *testing.T) {
	declared := declaredClauses(t)
	if len(declared) == 0 {
		t.Fatal("parsed no Clause constants out of attaint.go: the comparison would be vacuous")
	}

	// One scenario per clause, each a tree built to produce it. The clause
	// named here is what the scenario is FOR; a scenario may raise others and
	// the union is what is compared.
	scenarios := []struct {
		name  string
		want  Clause
		build func(t *testing.T) string
	}{
		{"an empty directory", ClauseNoPackages, func(t *testing.T) string { return t.TempDir() }},
		{"a package with no cases", ClauseNoCases, func(t *testing.T) string {
			p := healthy()
			p.cases = nil
			return build(t, p)
		}},
		{"a package with no mutants", ClauseNoMutants, func(t *testing.T) string {
			p := healthy()
			p.mutants = nil
			return build(t, p)
		}},
		{"a package declaring no rules", ClauseNoRules, func(t *testing.T) string {
			p := healthy()
			p.rules = []string{}
			return build(t, p)
		}},
		{"a planted lie the statute does not notice", ClauseLieNotFound, func(t *testing.T) string {
			p := healthy()
			c := p.cases["lie"]
			c.value = 2
			c.expect = map[string]any{"believed": 2, "finding": "consistent", "rests_on_planted": false}
			p.cases["lie"] = c
			return build(t, p)
		}},
		{"the tag stripped from the ledger", ClauseControlDidNotPass, func(t *testing.T) string {
			p := healthy()
			c := p.cases["lie"]
			c.ledger = strings.ReplaceAll(c.ledger, `,"planted":true`, "")
			p.cases["lie"] = c
			return build(t, p)
		}},
		{"no case declaring a control", ClauseNoControlDeclared, func(t *testing.T) string {
			p := healthy()
			c := p.cases["lie"]
			c.expect = map[string]any{"believed": 2, "finding": "bug", "rests_on_planted": false}
			p.cases["lie"] = c
			return build(t, p)
		}},
		{"a control armed and never evaluated", ClauseCheckerNeverFired, func(t *testing.T) string {
			// The lie is in the prefix, tagged, and belongs to an actor the
			// observer was never told of -- so no observation reaches it.
			p := healthy()
			c := p.cases["lie"]
			c.ledger = `{"kind":"claim","position":1,"actor":"a","type":"t","ref":"r1","handles":["r1"]}
{"kind":"claim","position":2,"actor":"a","type":"t","ref":"r2","handles":["r2"]}
{"kind":"claim","position":3,"actor":"b","type":"t","ref":"r3","handles":["r3"],"planted":true}
`
			c.seal = 3
			c.claims = []map[string]any{
				{"ref": "r1", "actor": "a", "type": "t", "position": 1, "handle": "r1"},
				{"ref": "r2", "actor": "a", "type": "t", "position": 2, "handle": "r2"},
				{"ref": "r3", "actor": "b", "type": "t", "position": 3, "handle": "r3", "planted": true},
			}
			c.known = []string{"r1", "r2"}
			c.expect = map[string]any{"believed": 2, "finding": "bug", "rests_on_planted": true}
			p.cases["lie"] = c
			return build(t, p)
		}},
		{"a rule no mutant moves", ClauseRuleUnmutated, func(t *testing.T) string {
			p := healthy()
			p.statute += "\nuntouched := 7\n"
			for id := range p.mutants {
				p.mutants[id] += "\nuntouched := 7\n"
			}
			p.rules = append(p.rules, "untouched")
			c := p.cases["lie"]
			c.expect["untouched"] = 7
			p.cases["lie"] = c
			return build(t, p)
		}},
		{"a mutant identical to its statute", ClauseMutantDead, func(t *testing.T) string {
			p := healthy()
			p.mutants["m4-absorbed"] = miniStatute
			return build(t, p)
		}},
		{"a finding the package cannot produce", ClauseFindingUnreachable, func(t *testing.T) string {
			p := healthy()
			c := p.cases["lie"]
			c.expect = map[string]any{"believed": 2, "finding": "explained", "rests_on_planted": true}
			p.cases["lie"] = c
			return build(t, p)
		}},
	}

	seen := map[Clause]bool{}
	for _, sc := range scenarios {
		rep := run(t, sc.build(t))
		got := map[Clause]bool{}
		for _, r := range rep.Refusals {
			seen[r.Clause] = true
			got[r.Clause] = true
		}
		// EACH SCENARIO MUST PRODUCE THE CLAUSE IT IS FOR. Without this the
		// union could be satisfied by scenarios raising each other's clauses,
		// and a scenario that stopped working would hide behind its neighbours.
		if !got[sc.want] {
			for _, r := range rep.Refusals {
				t.Errorf("  %s", r)
			}
			t.Errorf("%s did not produce %s", sc.name, sc.want)
		}
	}

	for c := range declared {
		if !seen[c] {
			t.Errorf("%s is declared in attaint.go and no scenario in this test drives it: "+
				"the arm is unexercised and a suite that stayed green would not say so", c)
		}
	}
	for c := range seen {
		if !declared[c] {
			t.Errorf("%s was produced and is not among attaint.go's declared Clause constants: "+
				"the reader is parsing the wrong thing, or a clause is built from a literal", c)
		}
	}
	// THE LOG MUST NOT CLAIM WHAT THE TEST JUST DISPROVED. The first cut
	// printed "sets equal" unconditionally and did so on a FAILING run, beside
	// the error saying they were not -- a summary line contradicting the
	// finding above it, which is the shape that gets quoted into a report.
	if t.Failed() {
		t.Logf("%d clause(s) declared in attaint.go, %d driven by %d scenario(s); the sets DIFFER, see above",
			len(declared), len(seen), len(scenarios))
		return
	}
	t.Logf("PROOF %d clause(s) declared in attaint.go, %d driven by %d scenario(s), sets equal",
		len(declared), len(seen), len(scenarios))
}

// declaredClauses reads the Clause constants out of attaint.go's own source.
//
// THROUGH go/ast, NOT A PATTERN, for the reason quill gave about the other
// reader: a partial pattern read reports "declared and not driven", which is a
// finding about an arm and is really a finding about a regex. With the syntax
// tree, "some of the constants" is not a state this can be in.
func declaredClauses(t *testing.T) map[Clause]bool {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "attaint.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing attaint.go: %v", err)
	}
	out := map[Clause]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		vs, ok := n.(*ast.ValueSpec)
		if !ok || vs.Type == nil {
			return true
		}
		id, ok := vs.Type.(*ast.Ident)
		if !ok || id.Name != "Clause" {
			return true
		}
		for _, v := range vs.Values {
			bl, ok := v.(*ast.BasicLit)
			if !ok || bl.Kind != token.STRING {
				t.Fatalf("a Clause constant is not a string literal: the declaration's shape changed")
			}
			lit, err := strconv.Unquote(bl.Value)
			if err != nil {
				t.Fatalf("unquoting %s: %v", bl.Value, err)
			}
			out[Clause(lit)] = true
		}
		return true
	})
	return out
}

// failAfter is a writer that succeeds n times and then refuses.
type failAfter struct {
	n int
}

func (w *failAfter) Write(p []byte) (int, error) {
	if w.n <= 0 {
		return 0, errors.New("the sink refused this line")
	}
	w.n--
	return len(p), nil
}

// THE TEST THAT WOULD HAVE CAUGHT THE UNCHECKED WRITES. golangci-lint found
// three of the four on the runner (task 231) and my local gate found none,
// because errcheck runs nowhere else -- rook rumor/509 named that gap a day
// before it bit me. A linter catching it is the better instrument; this is the
// one that works where the linter is absent.
//
// IT WALKS THE FAILURE POSITION rather than failing the first write, because
// each of the four sites is reached at a different point in the walk and a
// single "fail immediately" case would only ever exercise the earliest.
func TestAFailedWriteIsAFailedPass(t *testing.T) {
	dir := build(t, healthy())

	// How many lines a clean pass writes, so the walk below is bounded by a
	// measurement rather than a guess.
	var counting bytes.Buffer
	if _, err := Run(context.Background(), dir, &counting); err != nil {
		t.Fatalf("the healthy package did not run: %v", err)
	}
	lines := bytes.Count(counting.Bytes(), []byte("\n"))
	if lines == 0 {
		t.Fatal("the healthy package wrote no lines, so this test would pass vacuously")
	}

	// AND THE ERROR MUST NAME THE SITE THAT FAILED, not merely exist. The first
	// cut of this test asserted `err != nil` and a mutation check showed it
	// SURVIVED one site reverted to an unchecked write: the ignored failure at
	// line i was masked by the next CHECKED write failing too, so the test was
	// proving "some write is checked" while its name promised every one.
	// Attributing the error to the line that failed makes each site
	// individually load-bearing.
	site := func(line string) string {
		switch {
		case strings.Contains(line, "planted lie corroborated"):
			return "writing the corroboration line"
		case strings.Contains(line, "killed by"):
			return "writing the kill line"
		case strings.Contains(line, "alive from the cases only"):
			return "writing the inert line"
		case strings.Contains(line, "reached by"):
			return "writing the reachability line"
		}
		return ""
	}
	clean := strings.Split(strings.TrimRight(counting.String(), "\n"), "\n")
	if len(clean) != lines {
		t.Fatalf("counted %d line(s) and split %d", lines, len(clean))
	}
	seen := map[string]bool{}
	for i := 0; i < lines; i++ {
		want := site(clean[i])
		if want == "" {
			t.Fatalf("line %d is written by no site this test knows: %q", i+1, clean[i])
		}
		seen[want] = true
		_, err := Run(context.Background(), dir, &failAfter{n: i})
		if err == nil {
			t.Errorf("the sink refused write %d of %d and Run reported no error: "+
				"a line nobody received was counted as evidence", i+1, lines)
			continue
		}
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the sink refused write %d of %d (%q) and the error came from elsewhere:\n  got  %v\n  want it to name %q",
				i+1, lines, clean[i], err, want)
		}
	}
	t.Logf("PROOF %d write position(s) over %d distinct site(s), each refused in turn and each naming itself",
		lines, len(seen))
}

// EVERY ARM HAS A PERMITTED TWIN, AND THE PAIRING IS DERIVED RATHER THAN
// MAINTAINED.
//
// ratchet set the rule in R2.1: "a refusal arm that never sees the accepted
// case cannot tell 'refuses correctly' from 'refuses always'". Every clause in
// this package has a refusing case -- TestEveryClauseIsExercised proves that
// against the declared set. This proves the other side: for each clause there
// is a tree differing in the ONE THING the clause is about on which it does
// NOT fire.
//
// WHY A SECOND TEST AND NOT A COLUMN IN THE FIRST: the two make different
// claims and fail for different reasons. "Every clause fires somewhere" is
// about coverage; "every clause can decline to fire" is about discrimination,
// and an arm hard-wired to refuse passes the first and fails this one.
//
// THE TWIN IS DERIVED, NOT LISTED. For every clause whose refusal is a thing
// REMOVED from a healthy tree -- no package, no case, no mutant, no rule, no
// control declared, a lie nobody notices, a tag stripped, a finding made
// unreachable -- the twin is the healthy package itself, which has all of
// them. Only the two clauses whose refusal is a thing ADDED need their own
// twin, and they are named with the reason.
func TestEveryArmHasAPermittedTwin(t *testing.T) {
	declared := declaredClauses(t)
	if len(declared) == 0 {
		t.Fatal("parsed no Clause constants out of attaint.go: the comparison would be vacuous")
	}

	// The healthy package is the universal twin: nothing is missing from it,
	// nothing is absorbed, every rule is attacked and the control fires.
	healthyDir := build(t, healthy())
	var buf bytes.Buffer
	base, err := Run(context.Background(), healthyDir, &buf)
	if err != nil {
		t.Fatalf("the healthy package did not run: %v", err)
	}
	if !base.OK() {
		for _, r := range base.Refusals {
			t.Errorf("  %s", r)
		}
		t.Fatalf("the universal twin refused %d time(s), so no arm below has a clean counterpart",
			len(base.Refusals))
	}

	// The two clauses whose refusal is an ADDITION rather than an absence need
	// a twin of their own: removing the addition is what the twin shows.
	special := map[Clause]struct {
		why   string
		build func(t *testing.T) string
	}{
		ClauseMutantDead: {
			why: "a mutant that DOES move a rule, so the arm declines where the absorbed one fired",
			build: func(t *testing.T) string {
				p := healthy()
				// Not identical to the statute: differs by one rule, so it is
				// alive. The absorbed twin in the other test IS the statute.
				p.mutants["m4-not-absorbed"] = miniMutant
				return build(t, p)
			},
		},
		ClauseRuleUnmutated: {
			why: "a rule ADDED to the statute that a mutant also moves, where the untouched one did not",
			build: func(t *testing.T) string {
				p := healthy()
				p.statute += "\nalsoattacked := 7\n"
				for id := range p.mutants {
					p.mutants[id] += "\nalsoattacked := 8\n" // moved, so attacked
				}
				p.rules = append(p.rules, "alsoattacked")
				c := p.cases["lie"]
				c.expect["alsoattacked"] = 7
				p.cases["lie"] = c
				return build(t, p)
			},
		},
	}

	clauses := make([]string, 0, len(declared))
	for c := range declared {
		clauses = append(clauses, string(c))
	}
	sort.Strings(clauses)

	derived, named := 0, 0
	for _, name := range clauses {
		c := Clause(name)
		dir, why := healthyDir, "the healthy package, which lacks nothing"
		if sp, ok := special[c]; ok {
			dir, why = sp.build(t), sp.why
			named++
		} else {
			derived++
		}
		var out bytes.Buffer
		rep, err := Run(context.Background(), dir, &out)
		if err != nil {
			t.Errorf("%s: the twin tree did not run: %v", c, err)
			continue
		}
		fired := false
		for _, r := range rep.Refusals {
			if r.Clause == c {
				fired = true
				t.Errorf("%s fired on its permitted twin (%s): the arm refuses where it should accept, "+
					"so a passing run proves nothing about it -- %s", c, why, r.Detail)
			}
		}
		if !fired {
			t.Logf("  %-42s declines on %s", c, why)
		}
	}

	if derived+named != len(declared) {
		t.Fatalf("%d twin(s) over %d declared clause(s)", derived+named, len(declared))
	}
	t.Logf("PROOF %d clause(s), each with a permitted twin it declines on: %d derived from the healthy package, %d named",
		len(declared), derived, named)
}
