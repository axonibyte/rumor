// Package attaint_test drives fable's drills (breakout-r3/0) against the real
// example packages.
//
// WHAT A DRILL IS. A drill is a damaged copy of the tree: the pristine
// examples/ with one file replaced. Each drill directory holds ONLY the files
// it changes, so the directory itself is the diff -- a reader opens
// d2-weaken/examples/planted-lie/statute.rego and sees the weakening, rather
// than diffing two copies of a 77-file tree.
//
// THE COMPOSITION IS HERE AND NOT IN attaint. The tool under test never learns
// that drills exist; this harness builds the damaged tree in a temp directory
// and points the tool at it. A tool that assembled its own damaged input would
// be choosing the evidence against itself.
//
// AND EACH DRILL ASSERTS EXACTLY ONE REFUSAL, by clause. fable's D1 says
// "and must not refuse anything else" -- a drill that trips a second arm is
// not the drill that was designed, even when it still goes red, so the count
// is asserted beside the clause.
package attaint_test

import (
	"bytes"
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/attaint"
)

const (
	examplesDir = "../../examples"
	drillsDir   = "drills"
)

type drill struct {
	id string
	// clause is the one refusal this drill is cut to produce. Empty means the
	// drill must PASS, which is what makes d0 a control rather than a label.
	clause attaint.Clause
	// contains is a fragment of the refusal's detail, so a clause that starts
	// firing for a different reason is caught rather than counted.
	contains string
	why      string
}

var drills = []drill{
	{
		id:  "d0-control",
		why: "the three packages unmodified: attaint passes and names what it examined",
	},
	{
		id:       "d1-delete",
		clause:   attaint.ClauseFindingUnreachable,
		contains: `transitive-count: a case expects the finding "explained"`,
		why:      "DELETE: the clause yielding `explained` is removed from the statute",
	},
	{
		id:       "d2-weaken",
		clause:   attaint.ClauseMutantDead,
		contains: "planted-lie/m5-honest-counts-planted moves no rule",
		why:      "WEAKEN: the honest count stops excluding planted claims, which is m5's shape",
	},
	{
		id:       "d3-strip",
		clause:   attaint.ClauseControlDidNotPass,
		contains: "its sealed prefix carries no planted claim",
		why:      "STRIP: the `planted` tag leaves the ledger while the case still declares the control",
	},
	{
		id:       "d4-never",
		clause:   attaint.ClauseCheckerNeverFired,
		contains: "the control was armed and never evaluated",
		why:      "NEVER: the lie is in the prefix, tagged and declared, and the observation never reaches it",
	},
	{
		id:       "d5-no-control",
		clause:   attaint.ClauseNoControlDeclared,
		contains: "the corroboration control was never exercised",
		why:      "VACUITY: no case in the corpus declares itself a control at all",
	},
}

// TestTheDrillsAreAllPresent refuses a drill table that has drifted from the
// directory. Without it, deleting a drill directory would quietly shrink the
// gate's coverage and every remaining drill would still pass.
func TestTheDrillsAreAllPresent(t *testing.T) {
	entries, err := os.ReadDir(drillsDir)
	if err != nil {
		t.Fatalf("%s: %v", drillsDir, err)
	}
	onDisk := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() {
			onDisk[e.Name()] = true
		}
	}
	// d0 is the control and carries no overlay, so it has no directory.
	declared := map[string]bool{}
	for _, d := range drills {
		declared[d.id] = true
		if d.id == "d0-control" {
			continue
		}
		if !onDisk[d.id] {
			t.Errorf("drill %s is declared and has no directory", d.id)
		}
	}
	for name := range onDisk {
		if !declared[name] {
			t.Errorf("%s/%s exists and no drill declares it, so nothing runs it", drillsDir, name)
		}
	}
	// breakout-r3/0 designs five; fable /2 split D4 into the ARMED-AND-NEVER-
	// FIRED case and the vacuity that no case declares a control at all, which
	// are different conditions and must not share a clause.
	if len(drills) < 6 {
		t.Fatalf("%d drill(s) declared; breakout-r3/0 designs five and fable /2 split D4 into two", len(drills))
	}
	t.Logf("PROOF %d drill(s), %d with an overlay directory, all accounted for", len(drills), len(onDisk))
}

func TestEveryDrillTripsItsOwnArm(t *testing.T) {
	for _, d := range drills {
		t.Run(d.id, func(t *testing.T) {
			dir := compose(t, d.id)
			var out bytes.Buffer
			rep, err := attaint.Run(context.Background(), filepath.Join(dir, "examples"), &out)
			if err != nil {
				t.Fatalf("%s: the drill tree did not load, which is not the same as being caught: %v", d.id, err)
			}

			// THE COUNTS ARE ASSERTED ON EVERY DRILL, pass or fail. A damaged
			// tree that stopped being read at all would refuse for the right
			// clause and the wrong reason.
			if rep.Packages != 5 || rep.Cases != 32 || rep.Mutants != 37 || rep.Rules != 30 {
				t.Fatalf("%s examined %d package(s), %d case(s), %d rule(s), %d mutant(s); every drill is a damaged copy of the whole tree",
					d.id, rep.Packages, rep.Cases, rep.Rules, rep.Mutants)
			}

			if d.clause == "" {
				if !rep.OK() {
					for _, r := range rep.Refusals {
						t.Errorf("  %s", r)
					}
					t.Fatalf("%s is the control and it refused %d time(s)", d.id, len(rep.Refusals))
				}
				t.Logf("PROOF %s passes: %d rule(s) killed, %d finding(s) reached, %d control(s) corroborated",
					d.id, rep.KilledRules, rep.Findings, rep.Corroborated)
				return
			}

			if len(rep.Refusals) != 1 {
				for _, r := range rep.Refusals {
					t.Errorf("  %s", r)
				}
				t.Fatalf("%s produced %d refusal(s), wanted exactly 1: a drill that trips a second arm is not the drill that was designed",
					d.id, len(rep.Refusals))
			}
			got := rep.Refusals[0]
			if got.Clause != d.clause {
				t.Fatalf("%s refused by %q, wanted %q", d.id, got.Clause, d.clause)
			}
			if !strings.Contains(got.Detail, d.contains) {
				t.Fatalf("%s refused by the right clause with the wrong detail:\n  got  %s\n  want it to contain %q",
					d.id, got.Detail, d.contains)
			}
			t.Logf("PROOF %s -> %s", d.id, got)
		})
	}
}

// TestTheReadmeNamesTheRefusal keeps each drill's README honest: fable reads
// the text at the gate, so a README describing an arm the drill no longer
// trips is worse than no README.
func TestTheReadmeNamesTheRefusal(t *testing.T) {
	checked := 0
	for _, d := range drills {
		if d.clause == "" {
			continue
		}
		path := filepath.Join(drillsDir, d.id, "README.md")
		b, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		body := string(b)
		if !strings.Contains(body, string(d.clause)) {
			t.Errorf("%s does not name the clause %q it trips", path, d.clause)
		}
		if !strings.Contains(body, d.contains) {
			t.Errorf("%s does not carry the refusal text the drill produces", path)
		}
		checked++
	}
	if checked != len(drills)-1 {
		t.Fatalf("checked %d README(s) of %d drills with a clause", checked, len(drills)-1)
	}
	t.Logf("PROOF %d README(s) each name their clause and their refusal text", checked)
}

// compose builds the damaged tree: pristine examples/, then the drill's files
// over it. It returns the temp root.
func compose(t *testing.T, id string) string {
	t.Helper()
	root := t.TempDir()
	copyTree(t, examplesDir, filepath.Join(root, "examples"))
	overlay := filepath.Join(drillsDir, id, "examples")
	if _, err := os.Stat(overlay); err == nil {
		before := countFiles(t, filepath.Join(root, "examples"))
		copyTree(t, overlay, filepath.Join(root, "examples"))
		after := countFiles(t, filepath.Join(root, "examples"))
		// AN OVERLAY MUST REPLACE, NEVER ADD. A drill whose path drifted would
		// otherwise drop a new file beside the original and leave the tree
		// undamaged -- a drill that silently tests nothing.
		if before != after {
			t.Fatalf("drill %s changed the file count %d -> %d: its overlay added files instead of replacing them",
				id, before, after)
		}
	}
	return root
}

func countFiles(t *testing.T, dir string) int {
	t.Helper()
	n := 0
	if err := filepath.Walk(dir, func(_ string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !fi.IsDir() {
			n++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return n
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	if err := filepath.Walk(src, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if fi.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	}); err != nil {
		t.Fatalf("copying %s: %v", src, err)
	}
}

// TestTheInertSetAgreesWithTheDeclaredOne answers rook rumor/600: does the
// dead-mutant arm exempt the three mutants R2.3 declared equivalent, and if so
// does it carry the second half of that proof?
//
// IT EXEMPTS NOTHING, and this is the measurement rather than the assurance.
// attaint holds no declaration list. It measures both paths, finds those three
// move rules from the CASES, and therefore counts them alive -- the dead-mutant
// arm never considers them. What it reports is the set that is INERT ON THE
// LEDGER, discovered independently.
//
// So the two files are two oracles on one list: tests/engine DECLARES three by
// name with a written reason and proves each identical-and-still-killed;
// attaint MEASURES which are ledger-inert and never reads that declaration.
// This test compares them, by parsing the declaration out of its own source.
// A fourth mutant going equivalent, or a stale declaration, now fails in two
// places instead of one.
func TestTheInertSetAgreesWithTheDeclaredOne(t *testing.T) {
	// READ AS GO, NOT AS TEXT. quill r3/3: a pattern that catches two of three
	// keys reports "declared and not measured -- the declaration is stale",
	// which reads as a finding about mutants and is a finding about a regex.
	// A total failure is catchable with a zero-count control; a PARTIAL one
	// wears a result's clothes. So the declaration is read from the parsed
	// syntax tree, where "some keys" is not a state the reader can be in: it
	// either finds the composite literal and sees every element, or it does
	// not find it and says so.
	path := filepath.Join("..", "engine", "engine_test.go")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	declared := map[string]bool{}
	var lit *ast.CompositeLit
	ast.Inspect(file, func(n ast.Node) bool {
		vs, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for i, name := range vs.Names {
			if name.Name != "equivalentUnderTheBound" || i >= len(vs.Values) {
				continue
			}
			if cl, ok := vs.Values[i].(*ast.CompositeLit); ok {
				lit = cl
			}
		}
		return true
	})
	if lit == nil {
		t.Fatalf("%s declares no equivalentUnderTheBound composite literal; this cross-check is reading a file that moved", path)
	}
	for _, el := range lit.Elts {
		kv, ok := el.(*ast.KeyValueExpr)
		if !ok {
			t.Fatalf("an element of equivalentUnderTheBound is not a key/value pair: the declaration's shape changed")
		}
		k, ok := kv.Key.(*ast.BasicLit)
		if !ok || k.Kind != token.STRING {
			t.Fatalf("a key of equivalentUnderTheBound is not a string literal: the declaration's shape changed")
		}
		id, err := strconv.Unquote(k.Value)
		if err != nil {
			t.Fatalf("unquoting %s: %v", k.Value, err)
		}
		declared[id] = true
	}
	// THE ELEMENT COUNT IS THE PARSE'S OWN DENOMINATOR: every element of the
	// literal became a key, so a silently dropped one is impossible rather
	// than merely unlikely.
	if len(declared) != len(lit.Elts) {
		t.Fatalf("parsed %d key(s) from %d element(s): keys were lost or duplicated", len(declared), len(lit.Elts))
	}
	if len(declared) == 0 {
		t.Fatal("parsed no declarations out of tests/engine: the comparison would be vacuous")
	}

	var out bytes.Buffer
	rep, err := attaint.Run(context.Background(), examplesDir, &out)
	if err != nil {
		t.Fatalf("attaint on the pristine tree: %v", err)
	}
	if !rep.OK() {
		t.Fatalf("the pristine tree refused; this test's premise is d0 passing")
	}
	measured := map[string]bool{}
	for _, id := range rep.LedgerInert {
		measured[id] = true
	}

	for id := range declared {
		if !measured[id] {
			t.Errorf("tests/engine declares %s equivalent under the bound and attaint finds it moves a rule from the ledger: the declaration is stale", id)
		}
	}
	for id := range measured {
		if !declared[id] {
			t.Errorf("attaint finds %s inert on the ledger and tests/engine declares no equivalence for it: a fourth mutant has gone equivalent", id)
		}
	}
	// AND NONE OF THEM IS DEAD, which is the half rook asked about.
	for _, id := range rep.DeadMutants {
		if declared[id] {
			t.Errorf("%s is declared equivalent AND counted dead: the arm is exempting it instead of measuring it", id)
		}
	}
	if len(rep.DeadMutants) != 0 {
		t.Errorf("the pristine tree has %d dead mutant(s): %v", len(rep.DeadMutants), rep.DeadMutants)
	}
	t.Logf("PROOF %d declared by tests/engine, %d measured ledger-inert by attaint, sets equal, 0 dead: %v",
		len(declared), len(measured), rep.LedgerInert)
}
