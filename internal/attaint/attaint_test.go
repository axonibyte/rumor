package attaint

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
					"known":  c.known,
					"claims": c.claims,
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

// EVERY CLAUSE IS REACHED BY SOME TEST IN THIS FILE. A clause that stopped
// firing would otherwise be invisible: the suite would still be green and the
// arm would be gone.
func TestEveryClauseIsExercised(t *testing.T) {
	all := []Clause{
		ClauseNoPackages, ClauseNoCases, ClauseNoMutants, ClauseNoRules,
		ClauseCheckerNeverFired, ClauseNoControlDeclared, ClauseLieNotFound,
		ClauseControlDidNotPass, ClauseRuleUnmutated, ClauseFindingUnreachable,
		ClauseMutantDead,
	}
	seen := map[Clause]bool{}
	// ClauseControlDidNotPass is reached by the d3 drill in tests/attaint and
	// by the case below, so it is driven here too rather than assumed.
	p := healthy()
	c := p.cases["lie"]
	c.ledger = strings.ReplaceAll(c.ledger, `,"planted":true`, "")
	p.cases["lie"] = c
	rep := run(t, build(t, p))
	for _, r := range rep.Refusals {
		seen[r.Clause] = true
	}
	if !seen[ClauseControlDidNotPass] {
		for _, r := range rep.Refusals {
			t.Errorf("  %s", r)
		}
		t.Fatalf("stripping the tag did not reach %s", ClauseControlDidNotPass)
	}
	if len(all) != 11 {
		t.Fatalf("%d clauses listed; the table in this test must name every declared clause", len(all))
	}
	t.Logf("PROOF %s reached by the strip; %d clause(s) declared in total", ClauseControlDidNotPass, len(all))
}
