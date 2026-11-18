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

	"forge.axonibyte.com/axonibyte/rumor/internal/proof"
)

// THE SUITE'S OWN EVIDENCE HAS TO REACH A READER, AND FOR 111 SITES IT DID NOT.
// (111 when this was written; 221 now, and every one of them went through a
// bare `t.Logf` until 124's deliverable 6 moved them onto the one helper that
// will not log from a failing test -- fable, coop r9/251.)
//
// This tree calls `proof.Logf(t, "PROOF ...` 221 times to say what an arm actually
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
	// THE VERBOSE FLAG IS `-v` OR `-json`, WHICH IS A MEASUREMENT AND NOT A
	// LOOSENING. This arm required `-v` because without it the harness discards
	// a PASSING arm's log line and the mining step greps an empty subject. Go's
	// own `go help testflag` says -json presents "the same information as the -v
	// flag in a machine-readable format", and driven on this box with go1.27.2
	// against a control: `go test -json` carries a passing arm's PROOF line and
	// plain `go test` does not. So -json satisfies what this arm is ACTUALLY
	// about, and the requirement now reads as that -- the passing arms' output
	// must survive -- rather than as one spelling of it.
	invocation := regexp.MustCompile(`test\s+-count=1\s+-timeout\s+\S+\s+(-v|-json)\s+\./\.\.\.`)
	if !invocation.MatchString(test) {
		t.Errorf("%s's test step does not run `test -count=1 -timeout <d> (-v|-json) ./...`: with "+
			"neither flag the harness discards every passing arm's PROOF line, so the mining "+
			"step below greps an empty subject and the whole gate reports nothing; without "+
			"-timeout the per-binary watchdog is the toolchain's silent default", path)
	}
	if !strings.Contains(test, "test.log") {
		t.Errorf("%s's test step does not write test.log, which is the file the mining "+
			"step reads", path)
	}

	// CONDITION TWO: exactly one step mines PROOF out of the RECORD. Out of a
	// file and not out of the job's output: ci.yml's own shell echoes the word,
	// so a grep over anything but the suite's own artifact would be satisfied
	// by this file's lines and would pass with the suite silent.
	//
	// THE ARTIFACT IS test.json SINCE coop r10/22's condition (5). This step
	// DECIDES -- it exits 1 when it finds nothing -- so it reads the record and
	// not the rendering. `PROOF ` is a mid-line literal inside an Output
	// string, so the grep keeps its meaning with no anchor to lose; the arm
	// accepts either name because what it is really about is that the mining
	// reads the SUITE'S artifact, and pinning one filename here would make a
	// rename look like a missing gate.
	var mining []string
	for name, body := range steps {
		if regexp.MustCompile(`grep[^\n]*PROOF[^\n]*test\.(log|json)`).MatchString(body) {
			mining = append(mining, name)
		}
	}
	if len(mining) != 1 {
		t.Fatalf("%s has %d step(s) grepping PROOF out of test.json, want exactly 1: %v. "+
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

	proof.Logf(t, "PROOF ci.yml step %q mines PROOF from test.log and refuses on zero, and the "+
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

	// THE NUMERATOR MOVED OFF THE TEXT AND ONTO THE RECORD, so this arm moved
	// with it. It used to extract the step's `grep -aE` / `grep -acE` pair over
	// ../test.log and assert the two patterns were identical -- a real defect
	// twice over (ratchet, coop r8/162; and the `?` omission). The fraction now
	// takes its numerator from `tools/test-timeline.py --reported` over
	// ../test.json, because the old anchors match NOTHING in the record: a
	// `grep -cE '^ok[[:space:]]'` over test.json answers 0, so a repointed
	// pattern would have read zero packages and called it a shortfall.
	//
	// WHAT THE ARM STILL ASSERTS IS THE SAME CLAIM: the numerator counts every
	// kind of package the denominator counts. It is now driven on the thing
	// that computes it rather than on two spellings of a regex, which is
	// strictly more than the old shape could see -- a pattern can be identical
	// on both sides and still miss a KIND of package, which is exactly how the
	// `?` case shipped.
	// THE FIGURE IS FOUND BY ITS OWN NUMERATOR, not by punctuation. This keyed
	// on "package(s) reported;" and the semicolon was doing the discriminating
	// -- the step's PROOF line says "package(s) reported ok, 0 cached" and
	// would match a looser key. Then the figure gained a clause naming where
	// each half comes from, the semicolon moved, and this arm reported that the
	// fraction did not exist. `$reported of` is the fraction and nothing else
	// in the step is.
	var figure string
	for _, line := range strings.Split(test, "\n") {
		if strings.Contains(line, "$reported of") {
			figure = line
			break
		}
	}
	if figure == "" {
		t.Fatalf("%s prints no `$reported of <N> package(s) reported` figure, so there is no "+
			"fraction to read and every clause below would pass vacuously", path)
	}
	if !strings.Contains(figure, "list ./...") {
		t.Errorf("the figure takes its denominator from something other than `go list ./...`: %q. "+
			"A literal or a narrower list cannot be said to count the same packages as the "+
			"numerator", strings.TrimSpace(figure))
	}
	// THE NUMERATOR IS NAMED ON THE FIGURE'S OWN LINE, for the reason the old
	// clause's second draft recorded: `strings.Contains(test, ...)` anywhere in
	// the step left this arm green when the denominator was replaced by a
	// literal. $reported is the shell variable the step fills from the record.
	if !strings.Contains(figure, "$reported") {
		t.Errorf("the figure's numerator is not the record's count: %q", strings.TrimSpace(figure))
	}
	if !strings.Contains(test, "--reported") || !strings.Contains(test, "--json ../test.json --reported") {
		t.Errorf("%s fills the numerator from something other than "+
			"`tools/test-timeline.py --json ../test.json --reported`", path)
	}
	// AND THE STEP MUST ACTUALLY PRINT THE LIST, which a mutant showed this arm
	// could not see: replacing the step's `--verdicts` invocation with `true`
	// left every clause green, because the arm drove the TOOL and never the
	// step's use of it. The count alone tells a reader that 41 of 42 reported
	// and not WHICH, which is the whole reason the list is beside it.
	if !strings.Contains(test, "--json ../test.json --verdicts") {
		t.Errorf("%s prints the count without listing the packages from the record: a reader "+
			"told `41 of 42` and not which one is missing has to go and find the log", path)
	}

	// THE FIXTURE IS A RECORD AND NOT A LOG, carrying one of every kind of
	// package `go list ./...` counts -- a pass, a FAIL, a package with NO TEST
	// FILES (which emits `skip`, measured 2026-11-17), and one that fails to
	// BUILD -- plus three events that must NOT count: a per-TEST pass, a
	// `start`, and a build event with no Package at all. A non-JSON line is in
	// there too, because a cut record must not take the count with it.
	events := strings.Join([]string{
		`{"Action":"start","Package":"m/internal/wire"}`,
		`{"Action":"run","Package":"m/internal/wire","Test":"TestA"}`,
		`{"Action":"pass","Package":"m/internal/wire","Test":"TestA"}`,
		`{"Action":"pass","Package":"m/internal/wire","Elapsed":0.2}`,
		`{"Action":"fail","Package":"m/tests/clock","Elapsed":7.1}`,
		`{"Action":"skip","Package":"m/internal/vocabulary","Elapsed":0}`,
		`{"Action":"build-output","Output":"# m/internal/broken\n"}`,
		`{"Action":"build-fail"}`,
		`{"Action":"fail","Package":"m/internal/broken","Elapsed":0}`,
		`this line is not a record at all`,
		"",
	}, "\n")
	recPath := filepath.Join(t.TempDir(), "test.json")
	if err := os.WriteFile(recPath, []byte(events), 0o600); err != nil {
		t.Fatal(err)
	}

	// RESOLVED HERE AND NOT THROUGH timeline_test.go's pythonOrSkip: that file
	// is `package hygiene` and this one is `package hygiene_test`, two packages
	// in one directory, so the helper is not visible. Six lines beat a second
	// named helper that would have to be kept in step with the first.
	py := ""
	for _, name := range []string{"python3.12", "python3"} {
		if found, err := exec.LookPath(name); err == nil {
			py = found
			break
		}
	}
	if py == "" {
		t.Skip("no python3 on this box, so the numerator's instrument cannot be driven; " +
			"CI's jail was measured to carry python3.12 before this was written")
	}

	const wantReported = 4
	out, err := exec.Command(py, filepath.Join("..", "..", "tools", "test-timeline.py"),
		"--json", recPath, "--reported").Output()
	if err != nil {
		t.Fatalf("counting the fixture record: %v", err)
	}
	got := strings.TrimSpace(string(out))
	if got != strconv.Itoa(wantReported) {
		t.Fatalf("the numerator reads %q over a record carrying a pass, a FAIL, a no-test-files "+
			"skip and a BUILD FAILURE, with a per-test pass, a start, a Package-less build "+
			"event and a non-JSON line beside them; want %d", got, wantReported)
	}
	// AND THE LIST BESIDE IT COUNTS THE SAME SET, which is fable's close at coop
	// r10/30 and quill's finding at /29: the list was a grep over the RENDERING
	// while the count came from the record, so a line the renderer dropped
	// would print "41 of 42" over a list of 40. One predicate serves both now,
	// and this drives it rather than reading the step's text for it: the count
	// must be this list's length over the same record, for every kind of
	// package go list counts.
	listed, err := exec.Command(py, filepath.Join("..", "..", "tools", "test-timeline.py"),
		"--json", recPath, "--verdicts").Output()
	if err != nil {
		t.Fatalf("listing the fixture record: %v", err)
	}
	rows := strings.Split(strings.TrimSpace(string(listed)), "\n")
	if len(rows) != wantReported {
		t.Errorf("the list holds %d row(s) and the count says %d: a fraction whose halves "+
			"disagree is the defect this arm exists for, and it has now shipped from two "+
			"different sources. Rows: %q", len(rows), wantReported, rows)
	}
	// AND EVERY KIND IS IN IT BY NAME, because a count can agree with a list
	// while both miss the same kind -- which is exactly how `?` was lost.
	joined := strings.Join(rows, "\n")
	for _, want := range []string{"m/internal/wire", "m/tests/clock",
		"m/internal/vocabulary", "m/internal/broken"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the list omits %s, which reported: %q", want, joined)
		}
	}
	if !strings.Contains(joined, "skip\tm/internal/vocabulary") {
		t.Errorf("the no-test-files package is not listed as skip: %q", joined)
	}
	if !strings.Contains(joined, "fail\tm/internal/broken") {
		t.Errorf("the build-failed package is not listed as fail: %q", joined)
	}

	proof.Logf(t, "PROOF the fraction's numerator is computed from the record and counts %d of "+
		"%d package kinds -- pass, FAIL, no-test-files and build-failed -- while a per-test "+
		"event, a start, a Package-less build event and a non-JSON line count for none",
		wantReported, wantReported)
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

	// THE VIEW IS A TOOL MODE NOW, AND THIS ARM STILL DRIVES WHAT SHIPS. It
	// used to extract the step's `grep -nE <flags> '<pat>' ../test.log` and run
	// that exact pattern, because run 1012's pattern did not COMPILE under BSD
	// grep and a text assertion that it "contains FAIL" passed over it. The
	// view now reads ../test.json -- condition (5) at coop r10/22, since its rc
	// gates the step -- so the thing to drive is the invocation, and it is
	// taken out of ci.yml rather than copied here.
	if !strings.Contains(test, "--json ../test.json --failures") {
		t.Fatalf("%s does not run `tools/test-timeline.py --json ../test.json --failures`, so "+
			"this arm would drive a view the step does not use", path)
	}
	py := ""
	for _, name := range []string{"python3.12", "python3"} {
		if found, err := exec.LookPath(name); err == nil {
			py = found
			break
		}
	}
	if py == "" {
		t.Skip("no python3 on this box, so the failure view cannot be driven")
	}

	// THE FIXTURE CARRIES EVERY SHAPE A FAILURE TAKES IN THE RECORD, and the
	// properties the old flag-based view had to be driven for:
	//
	//   a test's OWN WORDS, which arrive BEFORE its `--- FAIL` line -- the
	//   defect that reached CI twice, where a reader got `--- FAIL` and
	//   nothing to act on (coop r9/147)
	//   a SUBTEST failure, the clause a careless fix loses
	//   a PANIC TRACE, which arrives AFTER its `panic:` line, so a one-sided
	//   context served one of the two and failed the other
	//   a PASSING package, which must not appear at all
	//
	// AND THE TWO-SIDED PROPERTY IS NOW STRUCTURAL rather than a flag: a
	// failing package's own output IS its context, so there are no sides. That
	// is a claim and so it is asserted, not assumed -- the words sit before the
	// FAIL line and the trace after the panic, and both must come out.
	record := strings.Join([]string{
		`{"Action":"output","Package":"m/x","Test":"TestWithWords","Output":"=== RUN   TestWithWords\n"}`,
		`{"Action":"output","Package":"m/x","Test":"TestWithWords","Output":"    f_test.go:7: THE ARMS OWN WORDS, which a reader needs\n"}`,
		`{"Action":"output","Package":"m/x","Test":"TestWithWords","Output":"--- FAIL: TestWithWords (0.00s)\n"}`,
		`{"Action":"fail","Package":"m/x","Test":"TestWithWords","Elapsed":0}`,
		`{"Action":"output","Package":"m/x","Test":"TestTop/the_subtest","Output":"    --- FAIL: TestTop/the_subtest (0.00s)\n"}`,
		`{"Action":"fail","Package":"m/x","Test":"TestTop/the_subtest","Elapsed":0}`,
		`{"Action":"output","Package":"m/x","Output":"panic: runtime error: index out of range\n"}`,
		`{"Action":"output","Package":"m/x","Output":"\tTRACE LINE AFTER THE PANIC\n"}`,
		`{"Action":"fail","Package":"m/x","Elapsed":1.0}`,
		`{"Action":"output","Package":"m/quiet","Output":"ok  \tm/quiet\t0.100s\n"}`,
		`{"Action":"pass","Package":"m/quiet","Elapsed":0.1}`,
		"",
	}, "\n")
	recPath := filepath.Join(t.TempDir(), "test.json")
	if err := os.WriteFile(recPath, []byte(record), 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command(py, filepath.Join("..", "..", "tools", "test-timeline.py"),
		"--json", recPath, "--failures").Output()
	if err != nil {
		t.Fatalf("the step's own failure view does not run: %v. A view that cannot run searches "+
			"nothing, and the step's empty-file branch then prints its reassuring sentence "+
			"over whatever the record actually holds", err)
	}
	view := string(out)
	for _, want := range []string{
		"THE ARMS OWN WORDS",         // the words before the FAIL line
		"TRACE LINE AFTER THE PANIC", // the trace after the panic
		"the_subtest",                // the subtest clause
		"panic: runtime error",
		"m/x",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("the failure view loses %q, which a reader needs to act on a failure. "+
				"Output:\n%s", want, view)
		}
	}
	if strings.Contains(view, "m/quiet") {
		t.Errorf("the failure view printed a package that PASSED, so the view is not scoped to "+
			"what failed. Output:\n%s", view)
	}

	// AND THE STEP STILL BRANCHES ON THE VIEW'S OWN rc, which is the half of
	// run 1012's defect that outlives any pattern: a broken search and a clean
	// record must not print the same sentence.
	const sentence = "(no failing package or test in the record)"
	if !strings.Contains(test, "frc") || !strings.Contains(test, `case "$frc" in`) {
		t.Errorf("%s does not capture the failure view's own rc, so it cannot tell a broken "+
			"search from a clean record", path)
	}
	if strings.Contains(test, "|| echo \""+sentence) {
		t.Errorf("%s prints %q on ANY nonzero rc, which is the defect run 1012 hid: a view "+
			"that exits nonzero reads as a clean record", path, sentence)
	}
	if !strings.Contains(test, sentence) {
		t.Errorf("%s never prints %q, so a run with no failures says nothing and the view's "+
			"silence cannot be told from its absence", path, sentence)
	}

	proof.Logf(t, "PROOF the step's own failure view ran over a record carrying a test's own "+
		"words, a subtest, a panic trace and a passing package: all four failure shapes "+
		"reached the reader, the passing package did not, and the step branches on the "+
		"view's rc")
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
	proof.Logf(t, "PROOF floor %ds %s watchdog %ds %s budget %ds, all three read out of %s",
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

	// THE STEP RESOLVES ITS OWN INTERPRETER FROM PATH and the fragment inherits
	// $PY from earlier in the step, which a fragment run alone does not have.
	// Without this the view takes its no-python3 branch and this arm asserts
	// nothing about the view at all -- it passed that way for exactly one run
	// of this file, which is why the branch is named here.
	viewPython := ""
	for _, name := range []string{"python3.12", "python3"} {
		if found, lookErr := exec.LookPath(name); lookErr == nil {
			viewPython = found
			break
		}
	}
	if viewPython == "" {
		t.Skip("no python3 on this box, so the step's failure view cannot be driven")
	}

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
		{"a clean record", `{"Action":"pass","Package":"m/wire","Elapsed":0.2}` + "\n",
			"(no failing package or test in the record)"},
		{"a record with a failure",
			`{"Action":"output","Package":"m/x","Output":"--- FAIL: TestTopLevel (0.01s)\n"}` + "\n" +
				`{"Action":"fail","Package":"m/x","Elapsed":2.0}` + "\n",
			"--- FAILED: m/x"},
	} {
		dir := t.TempDir()
		recPath := filepath.Join(dir, "test.json")
		if err := os.WriteFile(recPath, []byte(c.log), 0o600); err != nil {
			t.Fatal(err)
		}
		// THE FRAGMENT IS RUN AS THE STEP SPELLS IT, with only the things a
		// temp directory cannot supply substituted: the record's path, the
		// scratch file, the interpreter the step resolves from PATH, and the
		// tool's path, which is relative to the repo root in the step and to
		// this package's directory here.
		script := strings.ReplaceAll(fragment, "../test.json", recPath)
		script = strings.ReplaceAll(script, "../fails.txt", filepath.Join(dir, "fails.txt"))
		tool, err := filepath.Abs(filepath.Join("..", "..", "tools", "test-timeline.py"))
		if err != nil {
			t.Fatal(err)
		}
		script = strings.ReplaceAll(script, "tools/test-timeline.py", tool)
		script = "PY=" + viewPython + "\n" + script
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
	proof.Logf(t, "PROOF the step's own failure view ran under -e over a clean log and a failing one, "+
		"exiting 0 and reporting in both")
}

// TestTheLicenceStepRefusesTheNameOnlyProofOverAVersionedLicence EXECUTES the
// licences step's version cross-check, because it exists to close an escape
// that a written-but-undriven check reopens in one character.
//
// WHAT IT GUARDS. tests/licences holds docs/design.md's project-license
// sentence against LICENSE's own title, and its version read is CONDITIONAL on
// LICENSE carrying a `Version x.y.z` line -- an unversioned license text has no
// second copy to compare. quill measured the escape that leaves (coop r10/54):
// delete LICENSE's version line AND move the document to 2.0.0, and the two
// copies state different versions with every instrument satisfied. The suite
// now refuses a version the document states and LICENSE does not, and this step
// asserts the branch from OUTSIDE the arm: a versioned LICENSE must produce the
// version-holding PROOF.
//
// AND THE PATH IS THE WHOLE RISK. The step has already cd'd into src-tree, so
// the file is `LICENSE`; `../../LICENSE` is the path from the TEST's directory
// and is what the reviewer's own snippet carried. A wrong path fails the `if`
// quietly, takes the else branch, prints a reassuring line and hands the free
// escape straight back. So the fragment is run with its cwd set to a tree built
// for the case and its paths NOT substituted: case 2 can only fail if the step
// actually reads the file it names.
func TestTheLicenceStepRefusesTheNameOnlyProofOverAVersionedLicence(t *testing.T) {
	path := filepath.Join("..", "..", ".forgejo", "workflows", "ci.yml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var step string
	for name, body := range splitSteps(string(raw)) {
		if strings.HasPrefix(name, "licences") {
			step = body
			break
		}
	}
	if step == "" {
		t.Fatalf("%s has no step whose name begins `licences`, so this arm is reading for "+
			"something that is gone; the steps are %v", path, stepNames(splitSteps(string(raw))))
	}

	// THE ANCHOR CARRIES NO PATH, deliberately. A first draft anchored on
	// `... ' LICENSE; then` and so a path mutant failed HERE, at the anchor,
	// which proves only that the text changed. With the path out of the anchor
	// a wrong path still matches, the fragment runs, and case 2 below is what
	// fails -- which is the property this arm claims.
	const open = "if grep -q '^Version ' "
	i := strings.Index(step, open)
	if i < 0 {
		t.Fatalf("%s's licences step has no %q, so the version cross-check is gone and the "+
			"name-only PROOF over a versioned LICENSE is accepted again", path, open)
	}
	rest := step[i:]
	j := regexp.MustCompile(`(?m)^\s*fi\s*$`).FindStringIndex(rest)
	if j == nil {
		t.Fatalf("%s's version cross-check does not reach an `fi`", path)
	}
	fragment := rest[:j[1]]

	const versioned = "Copyright (c) 2026 Holder\n\n# Some Model License\n\nVersion 1.0.0\n"
	const unversioned = "Copyright (c) 2026 Holder\n\n# Some Model License\n"
	// The two PROOF shapes the arm can emit, as the arm spells them.
	const withVersion = `PROOF ... LICENSE's title "Some Model License" at version "1.0.0" and ...`
	const nameOnly = `PROOF ... LICENSE's title "Some Model License" and ... NEITHER states a version`

	for _, c := range []struct {
		name, license, log string
		wantExit0          bool
		want               string
	}{
		{"a versioned LICENSE and a version-holding PROOF", versioned, withVersion, true,
			"PROOF LICENSE states a version and the suite's PROOF carries it"},
		{"a versioned LICENSE and only the name-only PROOF", versioned, nameOnly, false,
			"the version half of the project-license pair ran on nothing"},
		{"an unversioned LICENSE and the name-only PROOF", unversioned, nameOnly, true,
			"PROOF LICENSE states no version line"},
	} {
		// THE TREE IS SHAPED LIKE THE RUNNER'S, because the paths are not
		// substituted: `LICENSE` sits in the step's cwd and `../lic.log` one
		// level above it, exactly as `cd src-tree` leaves them.
		root := t.TempDir()
		tree := filepath.Join(root, "src-tree")
		if err := os.Mkdir(tree, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(tree, "LICENSE"), []byte(c.license), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "lic.log"), []byte(c.log+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		scriptPath := filepath.Join(root, "check.sh")
		if err := os.WriteFile(scriptPath, []byte(fragment+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		cmd := exec.Command("sh", "-e", scriptPath)
		cmd.Dir = tree
		out, runErr := cmd.CombinedOutput()
		if c.wantExit0 && runErr != nil {
			t.Errorf("%s: the cross-check exits nonzero under -e (%v) and printed %q; the forge "+
				"runs a step with -e, so this is a legitimate tree failing the step",
				c.name, runErr, string(out))
		}
		if !c.wantExit0 && runErr == nil {
			t.Errorf("%s: the cross-check exited 0 and printed %q. This is the escape quill "+
				"measured: either the check is gone, or it is reading a path that does not "+
				"exist in the step's cwd, which fails the `if` and prints reassurance",
				c.name, string(out))
		}
		if !strings.Contains(string(out), c.want) {
			t.Errorf("%s: the cross-check printed %q, want it to contain %q",
				c.name, string(out), c.want)
		}
	}
	proof.Logf(t, "PROOF the licences step's version cross-check ran under -e over %d tree(s): a "+
		"versioned LICENSE with the version-holding PROOF, the same LICENSE with only the "+
		"name-only PROOF (refused), and an unversioned LICENSE (accepted)", 3)
}
