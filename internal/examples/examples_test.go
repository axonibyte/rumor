package examples

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const realTree = "../../examples"

// Declared sizes, matching tests/statute's and tests/engine's. A tally of zero
// disagreements reads identically over zero cases and over twenty-two, so the
// count of what was EXAMINED is asserted first.
const (
	wantPackages = 5
	wantCases    = 29
)

func TestRunOverTheCommittedTree(t *testing.T) {
	var out strings.Builder
	res, err := Run(context.Background(), realTree, &out)
	if err != nil {
		t.Fatalf("running over the committed tree: %v", err)
	}
	if res.Packages != wantPackages || res.Cases != wantCases {
		t.Fatalf("examined %d package(s) and %d case(s), wanted %d and %d",
			res.Packages, res.Cases, wantPackages, wantCases)
	}
	if res.Agreed != wantCases || res.Disagreed != 0 {
		t.Fatalf("%d agreed and %d disagreed, wanted all %d to agree", res.Agreed, res.Disagreed, wantCases)
	}
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != wantCases {
		t.Fatalf("%d line(s) for %d case(s)", len(lines), wantCases)
	}
	// Every line must carry the two things issue 44 asks a finding to keep: the
	// definition that produced it and a trace behind it.
	for _, ln := range lines {
		if !strings.Contains(ln, "statute.rego:") {
			t.Errorf("line names no definition: %q", ln)
		}
		if strings.Contains(ln, "   0 trace event(s)") {
			t.Errorf("line carries no trace: %q", ln)
		}
		if !strings.Contains(ln, "agrees") {
			t.Errorf("line does not report agreement: %q", ln)
		}
	}
	// And the findings are not all the same word, which a walk that evaluated
	// one case and reported it seventeen times would be.
	for _, want := range []string{"consistent", "bug", "explained"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("no case produced %q, so the walk cannot be exercising the statutes", want)
		}
	}
	t.Logf("PROOF %d case(s) over %d package(s), all agreeing, every line with a definition and a trace",
		res.Cases, res.Packages)
}

// copyPackage copies one example package into a temporary tree, so a failure
// can be planted without touching the committed fixtures.
func copyPackage(t *testing.T, name string) string {
	t.Helper()
	root := t.TempDir()
	src := filepath.Join(realTree, name)
	dst := filepath.Join(root, name)
	err := filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
	if err != nil {
		t.Fatalf("copying %s: %v", name, err)
	}
	return root
}

// A DISAGREEMENT IS A RESULT, NOT AN ERROR, because the caller's exit code has
// to tell "this tree cannot be read" from "this tree disagrees". The planted
// change is to the case's EXPECT rather than to the statute, so what moves is
// the verdict the example claims and not the engine's.
func TestADisagreementIsCountedAndNamed(t *testing.T) {
	root := copyPackage(t, "planted-lie")
	cf := filepath.Join(root, "planted-lie", "cases", "honest-only.json")
	b, err := os.ReadFile(cf)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	expect := doc["expect"].(map[string]any)
	if expect["finding"] != "consistent" {
		t.Fatalf("this fixture's finding is %v, so the planted change below is not the one described", expect["finding"])
	}
	expect["finding"] = "bug"
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cf, out, 0o644); err != nil {
		t.Fatal(err)
	}

	var sb strings.Builder
	res, err := Run(context.Background(), root, &sb)
	if err != nil {
		t.Fatalf("a disagreeing tree returned an error rather than a result: %v", err)
	}
	if res.Disagreed != 1 || res.Agreed != res.Cases-1 {
		t.Fatalf("%d disagreed and %d agreed of %d", res.Disagreed, res.Agreed, res.Cases)
	}
	if !strings.Contains(sb.String(), "DISAGREES: finding got consistent, wanted bug") {
		t.Errorf("the line does not name the rule and both values: %q", sb.String())
	}
	t.Logf("PROOF one planted expect: %d of %d disagreed, named on its own line", res.Disagreed, res.Cases)
}

// EVERY WAY THE WALK CAN EXAMINE NOTHING IS AN ERROR, read off the clause that
// answered rather than off err != nil -- four refusals share that return.
func TestTheWalkRefusesRatherThanExaminingNothing(t *testing.T) {
	ctx := context.Background()
	empty := t.TempDir()
	noStatute := t.TempDir()
	if err := os.MkdirAll(filepath.Join(noStatute, "not-a-package", "cases"), 0o755); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct{ name, dir, clause string }{
		{"a tree that is not there", filepath.Join(empty, "absent"), "is not readable"},
		{"a tree with no statute package", noStatute, "no package with a statute.rego"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Run(ctx, tc.dir, io.Discard); err == nil {
				t.Fatal("accepted")
			} else if !strings.Contains(err.Error(), tc.clause) {
				t.Fatalf("refused by the wrong clause: %v", err)
			}
		})
	}

	// A MISSING LEDGER IS AN ERROR AND NOT A SKIP. A walk that passed over the
	// case would report a smaller Cases count, and a smaller count with zero
	// disagreements still reads as a pass.
	t.Run("a case with no ledger file", func(t *testing.T) {
		root := copyPackage(t, "planted-lie")
		if err := os.Remove(filepath.Join(root, "planted-lie", "ledger", "honest-only.jsonl")); err != nil {
			t.Fatal(err)
		}
		_, err := Run(ctx, root, io.Discard)
		if err == nil {
			t.Fatal("a case with no ledger file was passed over")
		}
		if !strings.Contains(err.Error(), "has no ledger file") {
			t.Fatalf("refused by the wrong clause: %v", err)
		}
	})

	// A package whose cases are gone: the statute is there and nothing can be
	// asserted about it.
	t.Run("a package with a statute and no cases", func(t *testing.T) {
		root := copyPackage(t, "planted-lie")
		if err := os.RemoveAll(filepath.Join(root, "planted-lie", "cases")); err != nil {
			t.Fatal(err)
		}
		_, err := Run(ctx, root, io.Discard)
		if err == nil {
			t.Fatal("a package with no cases was accepted")
		}
		if !strings.Contains(err.Error(), "no cases") {
			t.Fatalf("refused by the wrong clause: %v", err)
		}
	})

	// The permitted twin for all four: the committed tree.
	if _, err := Run(ctx, realTree, io.Discard); err != nil {
		t.Fatalf("the committed tree was refused: %v", err)
	}
}

// failWriter refuses writes after n of them, so a result lost on the way out is
// an error rather than a silent success -- run 143's finding, applied here.
type failWriter struct{ left int }

func (w *failWriter) Write(p []byte) (int, error) {
	if w.left > 0 {
		w.left--
		return len(p), nil
	}
	return 0, io.ErrClosedPipe
}

func TestALostLineIsAnError(t *testing.T) {
	_, err := Run(context.Background(), realTree, &failWriter{})
	if err == nil {
		t.Fatal("a result that could not be written was reported as a clean walk")
	}
	if !strings.Contains(err.Error(), "writing the result for") {
		t.Fatalf("the error does not name the write: %v", err)
	}
	// And it names WHICH case was lost, not merely that something was.
	if !strings.Contains(err.Error(), "planted-lie/honest-only") {
		t.Errorf("the error does not name the case whose line was lost: %v", err)
	}
}

// THE WALK'S OWN LINE MUST NOT READ AN UNDEFINED FINDING AS A VERDICT.
//
// quill found this in internal/attaint (thread:r3/10) and the same field was
// ignored here (thread:rumor/623): the finding column printed `%v` of Value,
// so a statute that defines no finding rendered `<nil>` -- a verdict of nil
// rather than no verdict. The planted change is the REMOVAL of `default
// finding` from a copied package, which is the one edit that reaches the
// state; the committed corpus keeps its default and is not touched.
func TestAnUndefinedFindingIsNotPrintedAsNil(t *testing.T) {
	root := copyPackage(t, "planted-lie")
	sp := filepath.Join(root, "planted-lie", "statute.rego")
	b, err := os.ReadFile(sp)
	if err != nil {
		t.Fatalf("reading the copied statute: %v", err)
	}
	const def = "default finding := \"bug\"\n"
	if !strings.Contains(string(b), def) {
		t.Fatalf("the copied statute has no %q line: the planting no longer reaches the state under test", def)
	}
	if err := os.WriteFile(sp, []byte(strings.Replace(string(b), def, "", 1)), 0o644); err != nil {
		t.Fatalf("planting the removal: %v", err)
	}

	var out strings.Builder
	if _, err := Run(context.Background(), root, &out); err != nil {
		t.Fatalf("the walk could not read the planted tree: %v", err)
	}
	got := out.String()
	if strings.Contains(got, "<nil>") {
		t.Errorf("a line rendered an undefined finding as <nil>:\n%s", got)
	}
	if !strings.Contains(got, "no finding") {
		t.Errorf("no line names the absence; one case must now have none:\n%s", got)
	}
	// THE OTHER SIDE, AND THIS ARM WAS ONE-SIDED WITHOUT IT (quill,
	// thread:rumor/631): absent-<nil> plus present-"no finding" are both
	// satisfied by a renderer that answers "no finding" for EVERY case, which
	// is a worse defect than the one being fixed. The same planted tree
	// supplies the control for free -- removing `default finding` leaves the
	// `consistent` rule standing, so defined findings are still printed beside
	// the absent ones and no second fixture is needed.
	if !strings.Contains(got, "consistent") {
		t.Errorf("no line carries a real finding: a renderer answering the absence for every case "+
			"would satisfy the two assertions above, so this arm is what tells them apart:\n%s", got)
	}
	t.Logf("PROOF the walk names an absent finding rather than printing nil, and still prints the defined ones")
}
