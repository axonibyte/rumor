package hygiene_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// THE SUITE'S OWN EVIDENCE HAS TO REACH A READER, AND FOR 111 SITES IT DID NOT.
//
// This tree calls `t.Logf("PROOF ...` 111 times to say what an arm actually
// drove -- which seed, which profile, how many steps, which finding. None of it
// reached CI. `go test` discards a PASSING test's log output unless -v is set,
// and ci.yml's test step echoed `tail -20`, which under -v is the last package's
// verbose output rather than anybody's evidence. Measured at dd614ba: 0 PROOF
// lines in test.log plain, 344 with -v over ./... (rook, coop rumor/1119), and
// 3 of 13 lines against 0 of 1 on the promoted-seed arm alone (ratchet,
// rumor/1123).
//
// WHY IT WENT UNNOTICED FOR THE WHOLE PROJECT IS THE PART THIS ARM EXISTS FOR:
// ci.yml's own SHELL echoes the word PROOF at several steps, so a CI log is
// thick with it while carrying none from the tests. Two producers of one-looking
// line, the loud one masking the silent one -- which is the same shape as a
// cancelled run reading green, or a mutant that does not mutate.
//
// SO THE GATE HAS THREE CONDITIONS AND THEY ARE AND, NOT OR (fable, coop
// rumor/1124): -v puts the lines in the file, a step mines them out of it, and a
// count of ZERO REFUSES. Drop any one and the other two report nothing: without
// -v the mining greps an empty subject; without the mining the lines sit in a
// file nothing reads, since no artifact step uploads it; without the refusal a
// step that finds nothing prints nothing and reads exactly like a step that
// found everything.
//
// THIS ARM IS TEXT-KEYED ON PURPOSE. ci.yml is configuration, so its text IS
// its structure, and the repository has the precedent beside this one:
// tests/validate declares how many times ci.yml makes the arm-count claim,
// because "a deleted gate passes forever".
func TestCiMinesTheSuitesProofLines(t *testing.T) {
	path := filepath.Join("..", "..", ".forgejo", "workflows", "ci.yml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	steps := splitSteps(string(raw))
	if len(steps) < 2 {
		t.Fatalf("%s parsed into %d step(s), so this arm is reading the wrong shape and "+
			"every assertion below would pass vacuously", path, len(steps))
	}

	test, ok := steps["test"]
	if !ok {
		t.Fatalf("%s has no step named `test`; this arm keys on it, and a renamed step "+
			"would make the -v assertion below unfalsifiable. Steps found: %v",
			path, stepNames(steps))
	}

	// CONDITION ONE: the invocation carries -v, which is what makes the lines
	// exist at all. Keyed on the whole invocation rather than on " -v "
	// anywhere in the step, because a -v in a comment or in another command
	// would satisfy the looser pattern and prove nothing.
	invocation := regexp.MustCompile(`test\s+-count=1\s+-v\s+\./\.\.\.`)
	if !invocation.MatchString(test) {
		t.Errorf("%s's test step does not run `test -count=1 -v ./...`: without -v the "+
			"harness discards every passing arm's PROOF line, so the mining step below "+
			"greps an empty subject and the whole gate reports nothing", path)
	}
	if !strings.Contains(test, "test.log") {
		t.Errorf("%s's test step does not write test.log, which is the file the mining "+
			"step reads", path)
	}

	// CONDITION TWO: exactly one step mines PROOF out of the FILE. Out of the
	// file and not out of the job's output: ci.yml's own shell echoes the word,
	// so a grep over anything but test.log would be satisfied by this file's
	// own lines and would pass with the suite silent.
	var mining []string
	for name, body := range steps {
		if regexp.MustCompile(`grep[^\n]*PROOF[^\n]*test\.log`).MatchString(body) {
			mining = append(mining, name)
		}
	}
	if len(mining) != 1 {
		t.Fatalf("%s has %d step(s) grepping PROOF out of test.log, want exactly 1: %v. "+
			"Zero means the suite's evidence reaches no reader; more than one means two "+
			"gates can disagree about whether it did", path, len(mining), mining)
	}
	mine := steps[mining[0]]

	// CONDITION THREE: a count of zero REFUSES. This is the one whose absence
	// is invisible -- a mining step over an empty subject prints nothing and
	// exits 0, which is the `tail -20` defect one layer out.
	if !regexp.MustCompile(`-eq 0`).MatchString(mine) {
		t.Errorf("%s's mining step (%q) tests no zero case: a step that finds nothing "+
			"must say so, or it reads exactly like a step that found everything",
			path, mining[0])
	}
	if !regexp.MustCompile(`(?s)-eq 0.*?exit 1`).MatchString(mine) {
		t.Errorf("%s's mining step (%q) has a zero case that does not exit non-zero, so "+
			"a run with no PROOF line would be reported as green", path, mining[0])
	}
	if !strings.Contains(mine, "PROOF ") {
		t.Errorf("%s's mining step (%q) prints no PROOF count of its own, so a reader "+
			"cannot tell a mined zero from a step that did not run", path, mining[0])
	}

	t.Logf("PROOF ci.yml step %q mines PROOF from test.log and refuses on zero, and the "+
		"test step runs -v: all three of fable rumor/1124's conditions present",
		mining[0])
}

// splitSteps maps each `- name:` step to its text, for a file whose structure is
// its text. A step's body runs to the next step at the same indent.
func splitSteps(y string) map[string]string {
	head := regexp.MustCompile(`(?m)^      - name: (.+)$`)
	locs := head.FindAllStringSubmatchIndex(y, -1)
	out := make(map[string]string, len(locs))
	for i, loc := range locs {
		name := strings.TrimSpace(y[loc[2]:loc[3]])
		end := len(y)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		out[name] = y[loc[1]:end]
	}
	return out
}

func stepNames(steps map[string]string) []string {
	names := make([]string, 0, len(steps))
	for n := range steps {
		names = append(names, n)
	}
	return names
}
