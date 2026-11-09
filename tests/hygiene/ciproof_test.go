package hygiene_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
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
	//
	// IT GAINED `-timeout` AT R8/111 and the flag's VALUE is deliberately not
	// pinned here. The per-binary watchdog's number is held against the step's
	// outer budget by TestTheTwoTestBudgetsAgree, which reads both out of this
	// same file; repeating it here would make one number live in two places and
	// a budget change touch two files. What this arm still pins is the flag SET
	// and its ORDER, so a dropped -v, a reordering, or a watchdog quietly
	// removed all fail -- which is the whole-invocation keying the comment above
	// asks for.
	invocation := regexp.MustCompile(`test\s+-count=1\s+-timeout\s+\S+\s+-v\s+\./\.\.\.`)
	if !invocation.MatchString(test) {
		t.Errorf("%s's test step does not run `test -count=1 -timeout <d> -v ./...`: without -v "+
			"the harness discards every passing arm's PROOF line, so the mining step below "+
			"greps an empty subject and the whole gate reports nothing; without -timeout the "+
			"per-binary watchdog is the toolchain's silent default", path)
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

// TestTheFailureViewActuallySearchesTheLog RUNS the test step's FAIL/panic
// pattern, because for some number of runs it did not run at all.
//
// MEASURED ON RUN 1012 (task 1012, 2026-11-01): the step printed "(no FAIL or
// panic line in test.log)" over a log holding `--- FAIL: TestX`, a bare `FAIL`,
// and `FAIL forge.../tests/run 607.258s`. The pattern was
// `^(--- )?FAIL|^panic:|^\t+--- FAIL`, and BSD grep 2.6.0-FreeBSD refuses `\t`
// in an ERE outright -- "grep: trailing backslash (\)" -- so the whole grep
// exited 2, printed nothing, and under `set -o pipefail` the `|| echo` branch
// fired. A broken search and a clean log printed the same sentence, and the
// sentence is the reassuring one.
//
// SO THE ARM DRIVES THE PATTERN AND DOES NOT READ IT. A text assertion that the
// pattern "contains FAIL" would have passed on the broken one: it did contain
// FAIL. What separates them is whether grep can COMPILE it, which only running
// it answers -- the same reason the diagnostic arm above runs its own pattern
// rather than comparing spellings.
//
// AND THE SECOND HALF IS THE rc, which is the defect that outlives the regexp.
// `rc 1` is "no match" and is the only rc the reassuring sentence may be
// printed for; anything above 1 is the instrument failing. A step that cannot
// tell those apart mis-reads every future break the same way, so this arm
// requires the step to branch on the rc rather than on emptiness.
func TestTheFailureViewActuallySearchesTheLog(t *testing.T) {
	path := filepath.Join("..", "..", ".forgejo", "workflows", "ci.yml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	test, ok := splitSteps(string(raw))["test"]
	if !ok {
		t.Fatalf("%s has no step named `test`", path)
	}

	// THE FLAGS ARE TOLERATED AND CAPTURED, not pinned. This read
	// `grep -nE '<pat>' ../test.log` with nothing allowed between `-nE` and the
	// quote, so adding the context flag the view needs made this arm report
	// "0 FAIL/panic grep(s)" -- an arm whose purpose is to DRIVE the pattern
	// failing on where a flag sits. Group 1 is now those flags, and the second
	// half of this arm drives them (rook, coop r9/150).
	rx := regexp.MustCompile(`grep -nE([^']*)'([^']*)' \.\./test\.log`)
	// THE FAILURE VIEW AND NOT EVERY -nE GREP IN THE STEP: the cached-verdict
	// refusal reads the same log with the same flags, and it is a different
	// claim. `panic:` is the clause only the failure view carries.
	var pats, flagSets []string
	for _, m := range rx.FindAllStringSubmatch(test, -1) {
		if strings.Contains(m[2], "panic") {
			flagSets = append(flagSets, m[1])
			pats = append(pats, m[2])
		}
	}
	if len(pats) != 1 {
		t.Fatalf("%s spells %d FAIL/panic grep(s) over ../test.log, want exactly one: this arm "+
			"drives the pattern, and two would leave one undriven", path, len(pats))
	}
	pattern := pats[0]
	viewFlags := strings.Fields(flagSets[0])

	// THE FIXTURE CARRIES EVERY SHAPE `go test -v` USES TO REPORT A FAILURE,
	// and three that must not match. The indented one is a SUBTEST and it is
	// the clause the broken pattern was written for -- so a fix that merely
	// deleted `\t` would pass the compile and lose the case.
	log := strings.Join([]string{
		"ok  \tforge.axonibyte.com/axonibyte/rumor/internal/wire\t0.201s",
		"--- PASS: TestSomethingFine (0.01s)",
		"=== RUN   TestSomethingElse",
		"--- FAIL: TestTopLevel (0.01s)",
		"\t--- FAIL: TestTopLevel/the_subtest (0.00s)",
		"panic: runtime error: index out of range",
		"FAIL\tforge.axonibyte.com/axonibyte/rumor/tests/run\t607.258s",
		"",
	}, "\n")
	logPath := filepath.Join(t.TempDir(), "test.log")
	if err := os.WriteFile(logPath, []byte(log), 0o600); err != nil {
		t.Fatal(err)
	}

	// rc 0 AND FOUR LINES. The rc is asserted separately from the count
	// because they fail differently: rc 2 is the pattern not compiling (today's
	// defect) and a short count is the pattern compiling and missing a shape.
	cmd := exec.Command("grep", "-nE", pattern, logPath)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("the step's own FAIL/panic pattern %q does not run: %v. A pattern grep cannot "+
			"compile searches nothing, and the step's `rc 1` branch prints `(no FAIL or panic "+
			"line in test.log)` over whatever the log actually holds", pattern, err)
	}
	got := strings.Split(strings.TrimSpace(string(out)), "\n")
	const wantMatches = 4
	if len(got) != wantMatches {
		t.Errorf("the pattern matched %d line(s) of the fixture, want %d: %q",
			len(got), wantMatches, got)
	}
	for _, want := range []string{"--- FAIL: TestTopLevel (", "the_subtest", "panic:", "tests/run"} {
		if !strings.Contains(strings.Join(got, "\n"), want) {
			t.Errorf("the pattern misses %q, which `go test -v` prints for a failure", want)
		}
	}

	// AND THE STEP BRANCHES ON THE rc. Keyed on the sentence: it may appear in
	// the arm for rc 1 and nowhere else, so a future edit that moves it back
	// behind a bare `||` fails here.
	const sentence = "(no FAIL or panic line in test.log)"
	if !strings.Contains(test, "frc") || !strings.Contains(test, `case "$frc" in`) {
		t.Errorf("%s does not capture the FAIL grep's own rc, so it cannot tell a broken search "+
			"from a clean log", path)
	}
	if strings.Contains(test, "|| echo \""+sentence) {
		t.Errorf("%s prints %q on ANY nonzero rc, which is the defect run 1012 hid: a grep that "+
			"exits 2 reads as a clean log", path, sentence)
	}
	// AND THE VIEW MUST CARRY THE ARM'S OWN WORDS, which is the defect this
	// whole head exists for and which no assertion here could see.
	//
	// MEASURED: the view printed the matching lines ALONE, so a CI-only failure
	// arrived with `--- FAIL` and nothing a reader could act on -- twice, and
	// the second time 1117's cause was unrecoverable. `go test -v` prints a
	// test's diagnostic BEFORE its `--- FAIL` line, so the first proposed fix
	// (`-A`) recovered NONE of those words; driven here rather than read,
	// because three of us agreed on the wrong side (coop r9/147).
	//
	// THE FIXTURE PUTS WORDS ON BOTH SIDES: a t.Errorf line before a
	// `--- FAIL`, and a panic trace after a `panic:`. A one-sided flag serves
	// one of them and fails this arm.
	// THE TRACE SITS AT THE END WITH NO MATCH AFTER IT, and that placement is
	// the arm rather than a detail. My first fixture put a `FAIL <pkg>` line
	// AFTER the trace, so `-B` pulled the trace in as BEFORE-context of that
	// line and a one-sided flag passed -- a tautology in an arm written to
	// catch one-sidedness, found by mutating the flag to -B and watching it
	// survive. With nothing matching below it, only AFTER-context reaches the
	// trace. [[an-unfaithful-mutant-that-dies-is-a-false-pass]]
	ctx := strings.Join([]string{
		"=== RUN   TestWithWords",
		"    f_test.go:7: THE ARMS OWN WORDS, which a reader needs",
		"--- FAIL: TestWithWords (0.00s)",
		"FAIL\tforge.axonibyte.com/axonibyte/rumor/tests/x\t1.000s",
		"panic: runtime error: index out of range",
		"\tTRACE LINE AFTER THE PANIC",
		"\tsecond trace frame",
		"\tthird trace frame",
		"",
	}, "\n")
	ctxPath := filepath.Join(t.TempDir(), "ctx.log")
	if err := os.WriteFile(ctxPath, []byte(ctx), 0o600); err != nil {
		t.Fatal(err)
	}
	args := append(append([]string{"-nE"}, viewFlags...), pattern, ctxPath)
	cout, cerr := exec.Command("grep", args...).Output()
	if cerr != nil {
		t.Fatalf("the step's view (grep -nE %v) does not run: %v", viewFlags, cerr)
	}
	for _, want := range []string{"THE ARMS OWN WORDS", "TRACE LINE AFTER THE PANIC"} {
		if !strings.Contains(string(cout), want) {
			t.Errorf("the view's flags %v lose %q: a failure's own words reach no reader, "+
				"which is what this view exists to carry. Output:\n%s",
				viewFlags, want, cout)
		}
	}

	t.Logf("PROOF the step's own pattern %q ran and matched %d of %d failure shapes, and its "+
		"flags %v carry words on BOTH sides of a match", pattern, len(got), wantMatches, viewFlags)
}

// floorSeconds is the per-package watchdog's lower bound, and it is a number
// read off runs rather than chosen.
//
// MEASURED ON tests/run, THE SUITE'S CRITICAL PATH, in the ci jail:
//
//	task 995   598.000s   ok
//	task 1011  607.591s   FAIL
//	task 1012  607.258s   FAIL
//
// 598 under `go test`'s 10m default and 607 over it. So a watchdog at or near
// 600s is known to fire on a package that is not hung, and this floor is that
// figure with room for the growth that crossed it in the first place -- half
// again, not a round number for its own sake.
//
// AND IT STAYS 900 THROUGH THE 2026-11-04 CHANGE, which is worth saying because
// the obvious move was to raise it. Run 987 killed a package at 1200.035s under
// contention and the same package runs 717.898s alone -- but 1200.035 is where
// the watchdog KILLED it, not how long it takes, so that datum says ">1200s" and
// nothing more. A floor derived from it would be a number chosen against a
// truncation (rook, coop thread:rumor/1836 and the correction at /1837). The
// floor keeps the figure it was measured from; what moved is the relation below.
const floorSeconds = 900

// TestTheTwoTestBudgetsAgree holds the test step's two timeouts against each
// other, because for the whole life of the 1800s budget they disagreed.
//
// THERE ARE TWO AND ONLY ONE WAS DECLARED. The step wraps `go test` in
// `timeout 1800`, which fable raised from 900 at coop rumor/1665; `go test` also
// has its own per-binary watchdog, whose default is 10m and which nothing in
// this file mentioned. The smaller bound is the one that fires, so raising the
// outer one moved the cap a reader could see and left the cap that was actually
// in force where it was.
//
// THE RELATION IS floor < inner AND inner == outer, and the equality is the
// half that changed on 2026-11-04. It used to be floor < inner < outer, on the
// argument that an inner bound strictly inside the outer turns a HUNG package
// into goroutine stacks where the outer `timeout` prints nothing about where it
// stopped. RUN 987 IS WHAT RETIRED THAT: 892d0308 passed this workflow on
// r7.5-dst in 1237s and failed it on main in 1220s -- the same sha, the same
// file, no `if:` in it -- because the 1200s inner bound fired on a package that
// contention had slowed, and a released sha went red on main for it.
//
// NO INNER BOUND BELOW THE OUTER IS SAFE, and the reason is in floorSeconds
// above: the only contended figure anyone holds is censored at the very bound
// under discussion, so nothing bounds a contended package from above. A
// watchdog that can fire before the step's own budget will eventually fire on
// contention, whatever number it holds. So the per-binary bound stops being the
// contention guard, the outer budget becomes the only thing that can fire, and
// rc 124 with the step's diagnostic replaces a red that names a test.
//
// WHAT THE EQUALITY COSTS, recorded because it is a capability: a package that
// truly hangs now dies by `timeout` with no stacks. ci.yml says the same beside
// the number. fable ruled it at coop thread:rumor/1835.
//
// WHY EQUALITY AND NOT inner >= outer. Any inner above the outer behaves
// identically -- it can never fire -- so behaviour alone cannot choose between
// them. Equality is what ties the two numbers together: coop rumor/1665 raised
// the outer 900 -> 1800 and left the inner at the toolchain's silent 10m, which
// is the whole reason this arm exists. Under equality that same raise reddens
// here instead of passing.
//
// AND IT IS AN ARM BECAUSE THE DEFAULT IS SILENT. A missing `-timeout` is not a
// visible value to review; it is 10m arriving from the toolchain, which is how
// this survived a budget change made specifically to stop the suite reading as
// broken.
func TestTheTwoTestBudgetsAgree(t *testing.T) {
	path := filepath.Join("..", "..", ".forgejo", "workflows", "ci.yml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	test, ok := splitSteps(string(raw))["test"]
	if !ok {
		t.Fatalf("%s has no step named `test`", path)
	}

	outerRx := regexp.MustCompile(`timeout ([0-9]+) "\$GO" test`)
	om := outerRx.FindAllStringSubmatch(test, -1)
	if len(om) != 1 {
		t.Fatalf("%s wraps `go test` in %d `timeout N` call(s), want exactly one", path, len(om))
	}
	outer, err := strconv.Atoi(om[0][1])
	if err != nil {
		t.Fatalf("the outer budget %q is not a number of seconds: %v", om[0][1], err)
	}

	// THE INNER ONE MUST BE SPELLED. A regexp that tolerated its absence would
	// pass on exactly the configuration this arm exists to refuse.
	innerRx := regexp.MustCompile(`-timeout ([0-9][0-9a-z]*)`)
	im := innerRx.FindAllStringSubmatch(test, -1)
	if len(im) != 1 {
		t.Fatalf("%s passes %d `-timeout` flag(s) to `go test`, want exactly one: with none, the "+
			"per-binary watchdog is the toolchain's silent 10m default, which is smaller than "+
			"the %ds this step budgets and is therefore the bound in force", path, len(im), outer)
	}
	spelled := im[0][1]
	if _, err := strconv.Atoi(spelled); err == nil {
		t.Fatalf("`-timeout %s` has no unit: `go test` reads a bare number as NANOSECONDS, so "+
			"this would kill every package immediately", spelled)
	}
	d, err := time.ParseDuration(spelled)
	if err != nil {
		t.Fatalf("`-timeout %s` is not a duration `go test` accepts: %v", spelled, err)
	}
	inner := int(d.Seconds())

	// THE ARM RUN 987 WOULD HAVE REDDENED. At 892d0308 this read inner 1200,
	// outer 1800, and the old assertion was satisfied by exactly that pair.
	if inner != outer {
		which := "SMALLER than"
		why := "so contention can kill a package the step's own budget would have let finish, " +
			"which is run 987: 892d0308 passed on r7.5-dst in 1237s and failed on main in 1220s"
		if inner > outer {
			which = "LARGER than"
			why = "so it can never fire, and a budget raise that leaves it behind is invisible " +
				"-- the shape coop rumor/1665 left here when the outer went 900 -> 1800"
		}
		t.Errorf("the per-package watchdog is %ds, %s the step's whole budget of %ds: %s",
			inner, which, outer, why)
	}
	if inner <= floorSeconds {
		t.Errorf("the per-package watchdog is %ds, at or below the %ds floor: tests/run has "+
			"already reported 607.591s and 607.258s in the ci jail, so a bound this low kills a "+
			"package that is running and not hung", inner, floorSeconds)
	}
	// THE PROOF LINE DERIVES ITS SYMBOLS instead of spelling the relation it
	// hopes for. Written as `< ... ==` it printed "watchdog 1200s == budget
	// 1800s" under the very mutant that reddened the arm above -- a false
	// sentence in the evidence line, and the one a reader quotes.
	t.Logf("PROOF floor %ds %s watchdog %ds %s budget %ds, all three read out of %s",
		floorSeconds, relSymbol(floorSeconds, inner), inner, relSymbol(inner, outer), outer,
		filepath.Base(path))
}

// relSymbol spells the relation that HOLDS between two figures, so a PROOF line
// cannot assert one that does not.
func relSymbol(a, b int) string {
	switch {
	case a < b:
		return "<"
	case a > b:
		return ">"
	default:
		return "=="
	}
}

// TestTheFailureViewSurvivesTheRunnersErrFlag RUNS the failure-view fragment
// under -e, because reading it cannot show what -e does to it.
//
// MEASURED ON TASK 1014: the step spent 11.5 minutes on a suite that PASSED and
// reported `exit status 1` with an empty log. The forge invokes a step with -e,
// and under -e a BARE command exiting nonzero ends the script right there. The
// grep had just been changed from `grep ... || echo "(no FAIL...)"` to a bare
// `grep ... > ../fails.txt`, so on a green run it found nothing, exited 1, and
// killed the step before any branch could print. The form it replaced was doing
// two jobs at once -- masking grep's error AND keeping -e from firing -- and
// removing the mask took the guard with it.
//
// SO THE GREEN PATH IS THE ONE THAT BREAKS, which is why this arm drives the
// CLEAN log first. Every earlier arm here drove a log with failures in it: the
// mined lines, the four shapes, the rc branches. A log with NOTHING to find is
// the case the rc-1 guard exists for, and no arm had ever run it.
//
// IT SUBSTITUTES THE PATHS AND NOTHING ELSE. The fragment is lifted out of
// ci.yml verbatim between `view_broken=0` and its `esac`, with only the two file
// paths redirected into the temp dir -- so the thing under test is the step's
// own text, not a transcription of it.
func TestTheFailureViewSurvivesTheRunnersErrFlag(t *testing.T) {
	path := filepath.Join("..", "..", ".forgejo", "workflows", "ci.yml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	test, ok := splitSteps(string(raw))["test"]
	if !ok {
		t.Fatalf("%s has no step named `test`", path)
	}

	const open = "view_broken=0"
	i := strings.Index(test, open)
	if i < 0 {
		t.Fatalf("%s's test step has no `%s`, so the failure view's shape has changed and this "+
			"arm is reading for something that is gone", path, open)
	}
	j := strings.Index(test[i:], "esac")
	if j < 0 {
		t.Fatalf("%s's failure view does not reach an `esac`", path)
	}
	fragment := test[i : i+j+len("esac")]

	// THE rc REACHES `frc`, and this is a text clause because the execution
	// below cannot distinguish "captured correctly" from "captured by luck".
	//
	// ITS FIRST DRAFT WAS WORTHLESS AND THE MUTANT SHOWED IT: `Contains("|") &&
	// !Contains("||")` was meant to refuse a pipe, and the grep PATTERN carries
	// `|` as alternation -- so it could only ever fire on the correct text, and
	// it was skipped on the correct text only because `|| frc=$?` happens to
	// contain `||`. A clause that cannot fail where it matters and false-fires
	// where it does not adds a way to be wrong and no way to be right.
	if !strings.Contains(fragment, "|| frc=$?") {
		t.Errorf("the failure view does not capture the grep's rc through an AND-OR list " +
			"(`|| frc=$?`): under the runner's -e a bare grep that finds nothing ends the step " +
			"with no output, which is what task 1014 did to a green suite")
	}

	for _, c := range []struct {
		name, log, want string
	}{
		{"a clean log", "ok  \tforge.axonibyte.com/axonibyte/rumor/internal/wire\t0.201s\n",
			"(no FAIL or panic line in test.log)"},
		{"a log with a failure", "--- FAIL: TestTopLevel (0.01s)\nFAIL\tforge/x\t2.0s\n",
			"--- FAIL: TestTopLevel"},
	} {
		dir := t.TempDir()
		logPath := filepath.Join(dir, "test.log")
		if err := os.WriteFile(logPath, []byte(c.log), 0o600); err != nil {
			t.Fatal(err)
		}
		script := strings.ReplaceAll(fragment, "../test.log", logPath)
		script = strings.ReplaceAll(script, "../fails.txt", filepath.Join(dir, "fails.txt"))
		scriptPath := filepath.Join(dir, "view.sh")
		if err := os.WriteFile(scriptPath, []byte(script+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		out, err := exec.Command("sh", "-e", scriptPath).CombinedOutput()
		if err != nil {
			t.Errorf("%s: the failure view exits nonzero under -e (%v) and printed %q. The forge "+
				"runs a step with -e, so this is the step dying before it reports anything",
				c.name, err, string(out))
			continue
		}
		if !strings.Contains(string(out), c.want) {
			t.Errorf("%s: the failure view printed %q, want it to contain %q",
				c.name, string(out), c.want)
		}
	}
	t.Logf("PROOF the step's own failure view ran under -e over a clean log and a failing one, " +
		"exiting 0 and reporting in both")
}
