package hygiene_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
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

// TestTheTimeoutDiagnosticCountsTheSetItLists EXECUTES the rc-124 diagnostic
// against a log whose answer is known, and it exists because the arithmetic has
// been wrong three times in one day and nothing held it any of the three times
// (ratchet, coop r8/166):
//
//	r8/162   the list said `^(ok|FAIL)` and the count said `^ok`      nothing held it
//	/1675    the denominator counted every package and the numerator
//	         only those that run tests                                nothing held it
//	003a3a2  `?` is a report and was outside both                     nothing holds it
//
// THE RECURRING SHAPE IS A NUMERATOR AND A DENOMINATOR OVER DIFFERENT SETS, so
// this arm holds the shape and not the three instances: the two halves of the
// fraction must be the SAME pattern, and that pattern must count every kind of
// line `go test` uses to report a package.
//
// IT RUNS THE FILE'S OWN TEXT rather than a copy of the pattern. A second
// spelling here would be a third place to keep in step, and the defect this
// arm is for is two places already disagreeing -- so the pattern is extracted
// from ci.yml and handed to `grep`, which is what the step does with it.
//
// WHY IT IS TESTABLE AT ALL, which is the clause the untestable-configuration
// exemption would otherwise cover: this diagnostic is a pure function of a log
// file. Give it bytes, it prints figures. No runner, no toy, no forge.
func TestTheTimeoutDiagnosticCountsTheSetItLists(t *testing.T) {
	path := filepath.Join("..", "..", ".forgejo", "workflows", "ci.yml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	test, ok := splitSteps(string(raw))["test"]
	if !ok {
		t.Fatalf("%s has no step named `test`", path)
	}

	// THE TWO HALVES, as the step spells them: the list is `grep -aE` and the
	// count is `grep -acE`, both over ../test.log.
	rx := regexp.MustCompile(`grep -a(c?)E '([^']*)' \.\./test\.log`)
	var list, count []string
	for _, m := range rx.FindAllStringSubmatch(test, -1) {
		pat := m[2]
		// THE REPORTED-SET PAIR AND NOT EVERY GREP IN THE STEP. The step also
		// greps for an ok-line and for a cached verdict, which are different
		// claims; the pair this arm is about is the one naming a FAILED
		// package as reported, which is what makes it the fraction's set.
		if !strings.Contains(pat, "ok") || !strings.Contains(pat, "FAIL") {
			continue
		}
		if m[1] == "c" {
			count = append(count, pat)
		} else {
			list = append(list, pat)
		}
	}
	if len(list) != 1 || len(count) != 1 {
		t.Fatalf("%s spells %d list grep(s) and %d count grep(s) over the reported set, want one "+
			"of each: this arm reads the pair, and two of either would leave one unread",
			path, len(list), len(count))
	}
	if list[0] != count[0] {
		t.Errorf("the list counts %q and the figure counts %q: a numerator and a denominator over "+
			"different sets is the defect this arm exists for, and it has shipped twice",
			list[0], count[0])
	}

	// AND THE DENOMINATOR IS EVERY PACKAGE, which is the other side of the same
	// fraction: `go list ./...` counts a package with no test files, so the
	// numerator must count its `?` line or the figure reads short on a run
	// where nothing was running at all.
	//
	// ON THE FIGURE'S OWN LINE AND NOT ANYWHERE IN THE STEP, which is this
	// clause's second draft and the reason is a measurement. The first spelled
	// `strings.Contains(test, "list ./...")`, and the step uses `go list`
	// elsewhere -- so replacing the denominator with the literal `echo 42` left
	// this arm green. A clause that passes while the thing it names is gone is
	// the defect this file is about.
	var figure string
	for _, line := range strings.Split(test, "\n") {
		if strings.Contains(line, "package(s) reported;") {
			figure = line
			break
		}
	}
	if figure == "" {
		t.Fatalf("%s prints no `package(s) reported;` figure, so there is no fraction to read "+
			"and every clause below would pass vacuously", path)
	}
	if !strings.Contains(figure, "list ./...") {
		t.Errorf("the figure takes its denominator from something other than `go list ./...`: %q. "+
			"A literal or a narrower list cannot be said to count the same packages as the "+
			"numerator", strings.TrimSpace(figure))
	}
	if !strings.Contains(figure, count[0]) {
		t.Errorf("the figure's numerator is not the pattern this arm read (%q): %q",
			count[0], strings.TrimSpace(figure))
	}

	// THE FIXTURE: one line of each kind `go test` reports, one package absent
	// entirely, and three lines that must NOT count -- a verbose FAIL header,
	// an indented line carrying the word, and a package of another module.
	log := strings.Join([]string{
		"ok  \tforge.axonibyte.com/axonibyte/rumor/internal/wire\t0.201s",
		"FAIL\tforge.axonibyte.com/axonibyte/rumor/tests/clock\t7.134s",
		"?   \tforge.axonibyte.com/axonibyte/rumor/internal/vocabulary\t[no test files]",
		"--- FAIL: TestSomething (0.01s)",
		"    walk_test.go:189: ok  forge.axonibyte.com/axonibyte/rumor/internal/quoted",
		"ok  \tsome.other.host/not/ours\t1.000s",
		"",
	}, "\n")
	logPath := filepath.Join(t.TempDir(), "test.log")
	if err := os.WriteFile(logPath, []byte(log), 0o600); err != nil {
		t.Fatal(err)
	}

	const wantReported = 3
	out, _ := exec.Command("sh", "-c",
		"grep -acE '"+count[0]+"' "+logPath).Output()
	got := strings.TrimSpace(string(out))
	if got != strconv.Itoa(wantReported) {
		t.Errorf("the figure reads %q over a log carrying ok, FAIL and `?` for this module and "+
			"three lines that are not package reports; want %d", got, wantReported)
	}
	lines, _ := exec.Command("sh", "-c",
		"grep -aE '"+list[0]+"' "+logPath).Output()
	gotLines := strings.Split(strings.TrimSpace(string(lines)), "\n")
	if len(gotLines) != wantReported {
		t.Fatalf("the list printed %d line(s) over that log, want %d: %q",
			len(gotLines), wantReported, gotLines)
	}
	for _, want := range []string{"internal/wire", "tests/clock", "internal/vocabulary"} {
		if !strings.Contains(strings.Join(gotLines, "\n"), want) {
			t.Errorf("the list omits %s, which reported", want)
		}
	}
	t.Logf("PROOF one pattern on both sides of the fraction, driven over a %d-line log: %d reported, "+
		"%d listed, and the three non-reports left out", strings.Count(log, "\n"), wantReported, len(gotLines))
}
