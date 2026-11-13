package hygiene

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/coverage"
)

// A CHECK NOBODY RUNS IS THE WORST MEMBER OF ITS CLASS, because it is silently
// REASSURING: it sits in docs/coverage.toml and in the testing methodology, so
// a reader looking for coverage finds it and stops.
//
// MEASURED, AND IT COST A RED ON A RELEASED main: tools/test_d4control.sh had
// NO CALLER -- no workflow step, no Go suite -- and its `add-all-declared` cell
// was red from be91186 (2026-11-01) through v0.1.49, seen by nobody. Its three
// sibling verdict batteries were in ci.yml's batteries step and it was not, and
// that step's own comment has said "A BATTERY THAT NEVER RUNS IN CI IS A
// BATTERY THAT CAN ROT" the whole time. The comment was prose: it told four
// actors a battery should be there and refused nothing when one was missing.
//
// THE ARM READS THE DECLARATION AND CHECKS THE WORKFLOWS, NEVER THE REVERSE.
// quill's first proposal (coop rumor/2010) was to compare coverage.toml's
// tools/ list against the workflows and flag what is unmentioned; quill then
// measured it and refuted it themselves (rumor/2013): seven of thirteen are
// unmentioned and SIX are generators, drivers or a directory. An arm deriving
// which scripts MATTER from the only list that mentions them agrees with that
// list by construction -- the same trap as deriving the d4control cell's
// expected set from ADDED_BY, and as certifying a same-host test with the pair
// whose sameness was the question. Three instances of it in one night.
//
// SO EACH tools/ ENTRY DECLARES ONE BIT and this arm enforces it. The bit does
// NOT make the decision correct -- an author can write "hand" on a check and
// this arm will believe it. What it does is make the decision VISIBLE: before,
// a missing battery was an absence nobody could see in a diff; now it is a word
// somebody had to type, and a reviewer can disagree with it.
func TestEveryToolsEntryDeclaresWhetherCIRunsIt(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "coverage.toml"))
	if err != nil {
		t.Fatalf("reading docs/coverage.toml: %v", err)
	}
	entries, err := coverage.Parse(string(raw))
	if err != nil {
		t.Fatalf("parsing docs/coverage.toml: %v", err)
	}

	// THE WORKFLOWS, WHOLE, read once. A path is "invoked" when a workflow
	// mentions it; this is deliberately coarse, because a shell line can
	// invoke a script in more ways than a matcher can enumerate, and the
	// failure this arm exists for is a path mentioned NOWHERE.
	wfDir := filepath.Join("..", "..", ".forgejo", "workflows")
	wfNames, err := os.ReadDir(wfDir)
	if err != nil {
		t.Fatalf("reading %s: %v", wfDir, err)
	}
	workflows := map[string]string{}
	for _, f := range wfNames {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".yml") {
			continue
		}
		b, rerr := os.ReadFile(filepath.Join(wfDir, f.Name()))
		if rerr != nil {
			t.Fatalf("reading %s: %v", f.Name(), rerr)
		}
		workflows[f.Name()] = string(b)
	}
	if len(workflows) == 0 {
		t.Fatal("no workflow files read, so every `check` below would fail for the wrong reason")
	}
	// COMMENTS DO NOT COUNT, AND THE MUTANT IS WHY. The first version of this
	// helper matched the whole file, and the mutant that deletes the battery's
	// INVOCATION from ci.yml survived it -- because the step's own comment
	// names tools/test_d4control.sh while explaining why it is there. A grep
	// over a document counts its prose, and an arm that reads a comment as a
	// caller would have declared the original defect fixed by the paragraph
	// describing it.
	//
	// A line whose first non-space character is `#` is a comment in YAML and in
	// the shell inside a `run:` block alike, which is the whole of the rule.
	mentionedInAWorkflow := func(path string) bool {
		for _, body := range workflows {
			for _, line := range strings.Split(body, "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "#") {
					continue
				}
				if strings.Contains(line, path) {
					return true
				}
			}
		}
		return false
	}

	byPath := map[string]coverage.Entry{}
	var tools []coverage.Entry
	for _, e := range entries {
		byPath[e.Path] = e
		if strings.HasPrefix(e.Path, "tools/") {
			tools = append(tools, e)
		}
	}
	if len(tools) == 0 {
		t.Fatal("no tools/ entries found, so this arm checked nothing")
	}

	checks := 0
	for _, e := range tools {
		switch {
		case e.CI == "":
			// THE REFUSAL THAT WOULD HAVE CAUGHT IT: a tools/ entry with no
			// bit is the shape test_d4control.sh had for twelve days.
			t.Errorf("%s declares no `ci` key: say whether a workflow must run it "+
				"(\"check\"), whether another tool drives it (\"driven-by:<path>\"), or "+
				"whether it is run by hand (\"hand\")", e.Path)
		case e.CI == "check":
			checks++
			if !mentionedInAWorkflow(e.Path) {
				t.Errorf("%s is declared `check` and NO workflow mentions it: either a "+
					"workflow step is missing, or the declaration is wrong", e.Path)
			}
		case e.CI == "hand":
			// Nothing is required. A generator a workflow ran would overwrite
			// what the suite reads, which is why this value exists at all.
		case strings.HasPrefix(e.CI, "driven-by:"):
			driver := strings.TrimPrefix(e.CI, "driven-by:")
			d, ok := byPath[driver]
			if !ok {
				t.Errorf("%s says it is driven by %s, which docs/coverage.toml does not "+
					"name: a driver outside the map cannot be held to anything", e.Path, driver)
				continue
			}
			// THE CHAIN MUST REACH A CHECK, or "driven-by" is a way to say
			// "hand" without typing it: a tool driven by a tool that nobody
			// runs is a tool nobody runs.
			if d.CI != "check" {
				t.Errorf("%s says it is driven by %s, whose own `ci` is %q and not "+
					"\"check\": the chain does not reach anything a workflow runs",
					e.Path, driver, d.CI)
			}
			// AND THE DRIVER MUST ACTUALLY NAME IT, so the claim is checked
			// against the driver's text and not taken on the entry's word.
			b, rerr := os.ReadFile(filepath.Join("..", "..", driver))
			if rerr != nil {
				t.Errorf("%s names driver %s, which cannot be read: %v", e.Path, driver, rerr)
				continue
			}
			base := filepath.Base(e.Path)
			if !strings.Contains(string(b), base) {
				t.Errorf("%s says %s drives it, and %s does not mention %q",
					e.Path, driver, driver, base)
			}
		default:
			t.Errorf("%s declares ci = %q, which is not one of \"check\", \"hand\" or "+
				"\"driven-by:<path>\"", e.Path, e.CI)
		}
	}

	// AN EXACT COUNT, SO A SILENTLY EMPTIED MAP CANNOT PASS: every case above
	// is negative, and a tools/ list that lost its `check` entries would
	// satisfy all of them. This is the shape ci.yml's battery step already uses
	// on its verdict scripts -- "all 9 clauses over 10 cases" -- where a
	// literal must be retyped for the count to move.
	//
	// SIX, AND THE NUMBER IS MEASURED RATHER THAN ASSUMED. I first wrote seven
	// and this arm refused it: tools/check-workflows.py is LOCAL-ONLY on
	// purpose, said in .forgejo/workflows/release-gate.yml:16, and its three
	// appearances in the workflows are all comments. quill's census at coop
	// rumor/2013 counted those same three as invocations, so the arm corrected
	// a declaration of mine and a row of theirs on its first real run.
	const declaredChecks = 6
	if checks != declaredChecks {
		t.Errorf("%d tools/ entries are declared `check`, and this arm pins %d: if that is "+
			"a deliberate change, move the literal with it; if it is not, a check has "+
			"quietly stopped being one", checks, declaredChecks)
	}
	t.Logf("PROOF %d tools/ entries, %d declared `check` and each mentioned by a workflow",
		len(tools), checks)
}
