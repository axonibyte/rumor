package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/examples"
	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

// failWriter refuses every write. It is the whole reason run() takes io.Writer
// rather than *os.File: golangci-lint flagged three unchecked Fprintf returns
// in this file on run 143, and "checked" is only meaningful if something
// exercises the failure. errcheck can see that a return is ignored; it cannot
// see whether the handling is right.
type failWriter struct{ n int }

func (w *failWriter) Write(p []byte) (int, error) {
	w.n++
	return 0, errors.New("device full")
}

func TestCleanCheckPrintsTheCountAndExitsZero(t *testing.T) {
	orig := coverageCheck
	t.Cleanup(func() { coverageCheck = orig })
	coverageCheck = func() (int, error) { return 40, nil }

	var out, errb bytes.Buffer
	if rc := run([]string{"docs", "--check"}, bytes.NewReader(nil), &out, &errb); rc != 0 {
		t.Fatalf("a clean check must exit 0, got %d (stderr %q)", rc, errb.String())
	}
	if !strings.Contains(out.String(), "40 path(s), every one mapped") {
		t.Fatalf("the count line is what CI greps for; got %q", out.String())
	}
	if errb.Len() != 0 {
		t.Fatalf("a clean check must say nothing on stderr, got %q", errb.String())
	}
}

func TestAFailedCheckExitsOneAndNamesItOnStderr(t *testing.T) {
	orig := coverageCheck
	t.Cleanup(func() { coverageCheck = orig })
	coverageCheck = func() (int, error) { return 0, errors.New("stowaway.rs matches no entry") }

	var out, errb bytes.Buffer
	if rc := run([]string{"docs", "--check"}, bytes.NewReader(nil), &out, &errb); rc != 1 {
		t.Fatalf("a failed check must exit 1, got %d", rc)
	}
	if !strings.Contains(errb.String(), "stowaway.rs") {
		t.Fatalf("the failure must reach stderr by name; got %q", errb.String())
	}
	if out.Len() != 0 {
		t.Fatalf("a failed check must print nothing on stdout, got %q", out.String())
	}
}

// THE ARM THAT WOULD HAVE CAUGHT RUN 143'S FINDING. With the write unchecked
// this returned 0 -- reporting a clean check whose result never arrived, which
// is the same shape as a gate reading its formatter's exit status instead of
// its own.
func TestAFailedWriteDoesNotReportSuccess(t *testing.T) {
	orig := coverageCheck
	t.Cleanup(func() { coverageCheck = orig })
	coverageCheck = func() (int, error) { return 40, nil }

	w := &failWriter{}
	var errb bytes.Buffer
	rc := run([]string{"docs", "--check"}, bytes.NewReader(nil), w, &errb)
	if rc == 0 {
		t.Fatal("a check whose result could not be written must not exit 0")
	}
	if rc != 1 {
		t.Fatalf("expected exit 1, got %d", rc)
	}
	if w.n == 0 {
		t.Fatal("the write was never attempted, so this proves nothing about handling it")
	}
	if !strings.Contains(errb.String(), "could not be written") {
		t.Fatalf("the reason must reach stderr; got %q", errb.String())
	}
}

func TestUsageErrorExitsTwo(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"docs"},
		{"bogus"},
		{"docs", "--check", "extra"},
		{"--check", "docs"},
	} {
		var out, errb bytes.Buffer
		if rc := run(args, bytes.NewReader(nil), &out, &errb); rc != 2 {
			t.Errorf("args %q must exit 2, got %d", args, rc)
		}
		if !strings.Contains(errb.String(), usage) {
			t.Errorf("args %q must print the usage line to stderr; got %q", args, errb.String())
		}
	}
}

// `rumor check` (R2.3). The seam is examplesCheck, for the same reason
// coverageCheck is one: the argument surface, the three exit codes and the
// failed-write path are driven here without a tree of examples on disk, and the
// walk itself is tested in internal/examples against the real one.
func TestCheckReportsTheTallyAndExitsZero(t *testing.T) {
	saved := examplesCheck
	defer func() { examplesCheck = saved }()
	var gotDir string
	examplesCheck = func(dir string, out io.Writer) (examples.Result, error) {
		gotDir = dir
		_, _ = fmt.Fprintln(out, "planted-lie  honest-only  consistent  agrees")
		return examples.Result{Packages: 3, Cases: 17, Agreed: 17}, nil
	}
	var stdout, stderr bytes.Buffer
	if rc := run([]string{"check"}, bytes.NewReader(nil), &stdout, &stderr); rc != 0 {
		t.Fatalf("exit %d, wanted 0; stderr %q", rc, stderr.String())
	}
	if gotDir != defaultExamplesDir {
		t.Errorf("walked %q, wanted the default %q", gotDir, defaultExamplesDir)
	}
	if !strings.Contains(stdout.String(), "17 case(s) over 3 package(s) from ledger files, 17 agreed, 0 disagreed") {
		t.Errorf("the tally is not on stdout: %q", stdout.String())
	}
	// --examples replaces the directory, which is what lets this command be
	// pointed at another tree.
	gotDir = ""
	stdout.Reset()
	if rc := run([]string{"check", "--examples", "/elsewhere"}, bytes.NewReader(nil), &stdout, &stderr); rc != 0 {
		t.Fatalf("exit %d with --examples", rc)
	}
	if gotDir != "/elsewhere" {
		t.Errorf("walked %q, wanted /elsewhere", gotDir)
	}
}

// A DISAGREEMENT IS EXIT 1, AND SO IS EXAMINING NOTHING. Zero disagreements
// over zero cases is not a pass: it reads identically to agreement everywhere,
// which is the defect ibis main/352 named.
func TestCheckExitsOneOnADisagreementAndOnAnEmptyWalk(t *testing.T) {
	saved := examplesCheck
	defer func() { examplesCheck = saved }()
	for _, tc := range []struct {
		name string
		res  examples.Result
		err  error
		want int
	}{
		{"a case disagrees", examples.Result{Packages: 3, Cases: 17, Agreed: 16, Disagreed: 1}, nil, 1},
		{"nothing examined", examples.Result{}, nil, 1},
		{"the tree cannot be read", examples.Result{}, errors.New("no such tree"), 1},
		{"everything agrees", examples.Result{Packages: 3, Cases: 17, Agreed: 17}, nil, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			examplesCheck = func(string, io.Writer) (examples.Result, error) { return tc.res, tc.err }
			var stdout, stderr bytes.Buffer
			if rc := run([]string{"check"}, bytes.NewReader(nil), &stdout, &stderr); rc != tc.want {
				t.Fatalf("exit %d, wanted %d; stdout %q stderr %q", rc, tc.want, stdout.String(), stderr.String())
			}
			if tc.err != nil && !strings.Contains(stderr.String(), "no such tree") {
				t.Errorf("the failure is not named on stderr: %q", stderr.String())
			}
		})
	}
}

// The same arm as run 143's, for this subcommand: a tally that never arrived
// must not be reported as a pass.
func TestCheckDoesNotReportSuccessWhenItsTallyCannotBeWritten(t *testing.T) {
	saved := examplesCheck
	defer func() { examplesCheck = saved }()
	examplesCheck = func(string, io.Writer) (examples.Result, error) {
		return examples.Result{Packages: 3, Cases: 17, Agreed: 17}, nil
	}
	var stderr bytes.Buffer
	if rc := run([]string{"check"}, bytes.NewReader(nil), &failWriter{}, &stderr); rc != 1 {
		t.Fatalf("exit %d, wanted 1 when the tally could not be written", rc)
	}
	if !strings.Contains(stderr.String(), "could not be written") {
		t.Errorf("stderr does not say the result was lost: %q", stderr.String())
	}
}

func TestCheckRefusesAnUnknownFlag(t *testing.T) {
	for _, args := range [][]string{
		{"check", "--examples"},
		{"check", "--nonsense", "x"},
		{"check", "examples"},
		{"check", "--examples", "a", "b"},
	} {
		var stdout, stderr bytes.Buffer
		if rc := run(args, bytes.NewReader(nil), &stdout, &stderr); rc != 2 {
			t.Errorf("%v: exit %d, wanted 2 for a usage error", args, rc)
		}
		if !strings.Contains(stderr.String(), "rumor check") {
			t.Errorf("%v: the usage line does not mention check: %q", args, stderr.String())
		}
	}
}

// --- `rumor fake-adapter` (R4.1) ------------------------------------------

// TestFakeAdapterSpeaksTheProtocolOnStdout, and writes NOTHING else there: this
// subcommand's stdout is the wire, so the tally line every other subcommand
// prints would be a message the engine's decoder has to refuse.
func TestFakeAdapterSpeaksTheProtocolOnStdout(t *testing.T) {
	var stdout, stderr bytes.Buffer
	in := strings.NewReader(`{"kind":"describe","contract":"` + wire.ContractVersion + `","id":"1"}` + "\n")
	if rc := run([]string{"fake-adapter"}, in, &stdout, &stderr); rc != 0 {
		t.Fatalf("rc %d, stderr %q", rc, stderr.String())
	}
	lines := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("one request produced %d line(s) on stdout: %q", len(lines), stdout.String())
	}
	msg, err := wire.DecodeResponse([]byte(lines[0]))
	if err != nil {
		t.Fatalf("stdout is not on the protocol: %v", err)
	}
	if _, ok := msg.(wire.DescribeResponse); !ok {
		t.Errorf("answered %T", msg)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr %q: nothing to report on a clean run", stderr.String())
	}
}

// TestFakeAdapterFlagsAreRefusedByName, each with the reason on stderr and rc 2
// -- the usage code this binary has carried since R0.1.
func TestFakeAdapterFlagsAreRefusedByName(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"fake-adapter", "--nonesuch", "x"}, "unknown flag --nonesuch"},
		{[]string{"fake-adapter", "--omit", "ids"}, "wanted KIND.FIELD"},
		{[]string{"fake-adapter", "--omit", "execute."}, "wanted KIND.FIELD"},
		{[]string{"fake-adapter", "--surface"}, "takes a value"},
		{[]string{"fake-adapter", "--surface", "/nonesuch/surface.json"}, "--surface /nonesuch/surface.json"},
		// THE ARITY CASES ASSERT THE CLAUSE AND NOT THE USAGE TEXT, because
		// EVERY refusal here appends the same `wanted ACTION:AT:REASON[:TIMES]`
		// sentence -- so a case keyed on that phrase is satisfied by whichever
		// clause answered, and a mutant loosening the arity bound passed while
		// a DIFFERENT clause refused the same spec. The arity message is the
		// only one with nothing between the colon and `wanted`.
		{[]string{"fake-adapter", "--stop-flow", "create_event:2"}, ": wanted ACTION:AT:REASON"},
		{[]string{"fake-adapter", "--stop-flow", "create_event:2:wait:1:2:extra"}, ": wanted ACTION:AT:REASON"},
		{[]string{"fake-adapter", "--stop-flow", "create_event:2:wait:1:later"}, `THENAT "later" is not a number`},
		{[]string{"fake-adapter", "--stop-flow", ":2:wait"}, "no action named"},
		{[]string{"fake-adapter", "--stop-flow", "create_event:two:wait"}, `AT "two" is not a number`},
		{[]string{"fake-adapter", "--stop-flow", "create_event:2:wait:once"}, `TIMES "once" is not a number`},
		{[]string{"fake-adapter", "--stop-flow", "create_event:2:"}, "no reason named"},
	} {
		var stdout, stderr bytes.Buffer
		rc := run(tc.args, bytes.NewReader(nil), &stdout, &stderr)
		if rc != 2 {
			t.Errorf("%v: rc %d, want 2", tc.args, rc)
		}
		if !strings.Contains(stderr.String(), tc.want) {
			t.Errorf("%v: stderr %q omits %q", tc.args, stderr.String(), tc.want)
		}
		if stdout.Len() != 0 {
			t.Errorf("%v: wrote %q to the wire while refusing its arguments", tc.args, stdout.String())
		}
	}
}

func TestFakeAdapterOmitAndContractReachTheConfig(t *testing.T) {
	var stdout, stderr bytes.Buffer
	in := strings.NewReader(`{"kind":"execute","contract":"` + wire.ContractVersion +
		`","id":"1","action":"create_event","args":{},"token":"t"}` + "\n")
	if rc := run([]string{"fake-adapter", "--omit", "execute.ids", "--contract", "2"}, in, &stdout, &stderr); rc != 0 {
		t.Fatalf("rc %d, stderr %q", rc, stderr.String())
	}
	line := strings.TrimSpace(stdout.String())
	if strings.Contains(line, `"ids"`) {
		t.Errorf("--omit did not reach the adapter: %s", line)
	}
	if !strings.Contains(line, `"contract":"2"`) {
		t.Errorf("--contract did not reach the adapter: %s", line)
	}
}

// TestFakeAdapterStopFlowReachesTheConfig, which is the wiring half: the parse
// above is refused by name, and this is the one case that proves a WELL-FORMED
// spec arrives at the adapter and changes what it answers. Without it the flag
// could parse perfectly and be dropped on the floor.
//
// THE CONTROL IS THE SAME REQUEST WITHOUT THE FLAG, run first: a `stopped` that
// appears either way would say nothing about the flag.
func TestFakeAdapterStopFlowReachesTheConfig(t *testing.T) {
	flow := `{"kind":"flow","contract":"` + wire.ContractVersion +
		`","id":"1","action":"create_event","token":"t","steps":[` +
		`{"step":"click","target":"join"},{"step":"wait","until":"count-changed"}]}` + "\n"

	var plain, stderr bytes.Buffer
	if rc := run([]string{"fake-adapter"}, strings.NewReader(flow), &plain, &stderr); rc != 0 {
		t.Fatalf("control: rc %d, stderr %q", rc, stderr.String())
	}
	if strings.Contains(plain.String(), `"stopped"`) {
		t.Fatalf("the control already stops: %s", strings.TrimSpace(plain.String()))
	}
	if !strings.Contains(plain.String(), `"reached":2`) {
		t.Fatalf("the control did not complete the flow: %s", strings.TrimSpace(plain.String()))
	}

	var stopped bytes.Buffer
	stderr.Reset()
	rc := run([]string{"fake-adapter", "--stop-flow", "create_event:1:wait"},
		strings.NewReader(flow), &stopped, &stderr)
	if rc != 0 {
		t.Fatalf("rc %d, stderr %q", rc, stderr.String())
	}
	line := strings.TrimSpace(stopped.String())
	if !strings.Contains(line, `"stopped":"wait"`) {
		t.Errorf("--stop-flow did not reach the adapter: %s", line)
	}
	if !strings.Contains(line, `"reached":1`) {
		t.Errorf("--stop-flow reached the adapter and its position did not: %s", line)
	}

	// AND TIMES REACHES IT TOO, which needs TWO requests down one stdin: with
	// `:1` the first flow stops and the second completes, and a flag whose
	// count was dropped would stop both. One request cannot tell the two apart,
	// which is why the case above is not enough on its own.
	var twice bytes.Buffer
	stderr.Reset()
	if rc := run([]string{"fake-adapter", "--stop-flow", "create_event:1:wait:1"},
		strings.NewReader(flow+flow), &twice, &stderr); rc != 0 {
		t.Fatalf("rc %d, stderr %q", rc, stderr.String())
	}
	answers := strings.Split(strings.TrimSpace(twice.String()), "\n")
	if len(answers) != 2 {
		t.Fatalf("%d answer(s) for 2 requests: %q", len(answers), twice.String())
	}
	if !strings.Contains(answers[0], `"stopped":"wait"`) {
		t.Errorf("the first flow did not stop: %s", answers[0])
	}
	if strings.Contains(answers[1], `"stopped"`) {
		t.Errorf("TIMES 1 stopped the second flow too, so the count did not reach the adapter: %s", answers[1])
	}
}

// TestFakeAdapterReportsAFailedWriteOnTheExitCode: the engine reads this
// process's status, so a write that never arrived cannot be a clean exit.
func TestFakeAdapterReportsAFailedWrite(t *testing.T) {
	var stderr bytes.Buffer
	in := strings.NewReader(`{"kind":"describe","contract":"` + wire.ContractVersion + `","id":"1"}` + "\n")
	if rc := run([]string{"fake-adapter"}, in, &failWriter{}, &stderr); rc != 1 {
		t.Errorf("rc %d, want 1", rc)
	}
	if !strings.Contains(stderr.String(), "fake-adapter:") {
		t.Errorf("stderr %q does not name the subcommand", stderr.String())
	}
}

// TestFakeAdapterUsageIsInTheUsageLine, because a subcommand nobody can
// discover is a subcommand the agent guide has to carry instead.
func TestFakeAdapterUsageIsInTheUsageLine(t *testing.T) {
	for _, want := range []string{"fake-adapter", "--surface", "--contract", "--fail", "--omit", "--id-salt", "--stop-flow", "--lacks"} {
		if !strings.Contains(usage, want) {
			t.Errorf("the usage line omits %q", want)
		}
	}
}

// THE DISPATCH'S PASS OVER ITS SELECTION NAMES EVERY STAGE IT CANNOT RUN.
//
// rook's finding at coop r8/67 moved this check ahead of the first stage, so a
// build inconsistency costs nothing instead of a whole walk. The refusal it
// guards cannot be reached by any build -- internal/stage's tripwire fails in
// the same change that would create one -- so what is armed here is the
// selection and not the message: every missing name, in the selection's order,
// and none when every stage has a runner.
//
// A FIRST-ONE-WINS PASS WOULD PASS A ONE-STAGE CELL, which is why the subject
// has two missing stages around one that is present.
func TestTheDispatchNamesEveryStageItCannotRun(t *testing.T) {
	runners := map[string]func() int{"api": func() int { return 0 }}
	got := missingRunners([]string{"fuzz", "api", "browser"}, runners)
	if len(got) != 2 || got[0] != "fuzz" || got[1] != "browser" {
		t.Errorf("the pass named %v; it must name every selected stage with no case, in the "+
			"selection's order", got)
	}
	if got := missingRunners([]string{"api"}, runners); len(got) != 0 {
		t.Errorf("the pass named %v over a selection every runner covers", got)
	}
}

// THE ENGINE SUPPLIES THE SYSTEM'S ZONE AND PUTS THE ENVIRONMENT BACK (R7.5
// D4c; fable's ruling at coop rumor/1780).
//
// IN-PROCESS AND WITH AN ADAPTER THAT REPORTS ITS OWN ENVIRONMENT, which is
// what makes both halves observable. The supply is visible from outside -- the
// toy prints the zone it reads -- but the RESTORE is a property of this
// process: the variable is set before a spawn and put back when the verb
// returns, and its only consumer is the shrink, which spawns many adapters in
// one run. An arm that forked a binary could not see it, and a mutant that
// dropped the restore survived one that tried.
//
// THE ADAPTER IS A SHELL THAT WRITES WHAT IT SAW AND FAILS. `wire.Open` then
// reports could-not-run, which is exit 2 and not a refusal -- so this cell
// asserts the engine's BEHAVIOUR BEFORE the spawn without needing a walkable
// world or a real system under test.
func TestTheEngineSuppliesTheSystemsZoneAndRestoresTheEnvironment(t *testing.T) {
	dir := t.TempDir()
	seen := filepath.Join(dir, "seen")
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("platform.toml", `contract = "2"

[adapter]
command = ["sh", "-c", "printf '%s' \"$RUMOR_TEST_ZONE\" > `+seen+`; exit 1"]
env = ["RUMOR_TEST_ZONE"]
zone_env = "RUMOR_TEST_ZONE"

[[venire]]
role = "member"
count = 1

[stages]
api = { enabled = true }

[weights.default]
do = 1
canard = 0
palter = 0
suborn = 0
essoin = 0
`)
	write("bindings.toml", `[[action]]
name = "do"
context = "api"
emits = ["thing.done"]
requires = ["may_do"]

[claim."thing.done"]
timing = "immediate"

[clock]
form = "faked"
zone = "America/Chicago"
`)
	write("statute.rego", "package rumor.statute\n\ndefault may_do := true\n")

	// THE PRIOR VALUE IS A SENTINEL, so "restored" is distinguishable from
	// "unset" and from "left at the project's zone".
	const sentinel = "a-value-nobody-derived"
	t.Setenv("RUMOR_TEST_ZONE", sentinel)

	var out, errb bytes.Buffer
	rc := run([]string{"run", dir, "--seed", "1", "--steps", "1"}, bytes.NewReader(nil), &out, &errb)
	// COULD-NOT-RUN, because the adapter exits: the project is not wrong and
	// this is not a refusal (docs/contract.md:57).
	if rc != 2 {
		t.Fatalf("exit %d, want 2 (could-not-run: the fixture's adapter fails on purpose)\n%s\n%s",
			rc, out.String(), errb.String())
	}

	// THE ADAPTER SAW THE PROJECT'S DECLARED ZONE, not the sentinel: the
	// engine resolved `[clock].zone` and set the variable the project named.
	raw, rerr := os.ReadFile(seen)
	if rerr != nil {
		t.Fatalf("the adapter recorded nothing, so it never ran and this cell measures "+
			"neither half: %v", rerr)
	}
	if got := string(raw); got != "America/Chicago" {
		t.Errorf("the adapter saw RUMOR_TEST_ZONE=%q, want America/Chicago: the engine did not "+
			"supply the zone the project declares, so a promoted row would walk a world its "+
			"own declarations do not describe", got)
	}

	// AND THE ENVIRONMENT IS BACK, which is the half no out-of-process arm can
	// see. Left set, every later spawn in this process -- every candidate the
	// shrink replays -- would inherit a zone from a verb that has returned.
	if got := os.Getenv("RUMOR_TEST_ZONE"); got != sentinel {
		t.Errorf("RUMOR_TEST_ZONE is %q after the verb returned, want the sentinel %q: a "+
			"process-wide variable the engine set and did not put back reaches every later "+
			"spawn", got, sentinel)
	}
}

// --- `rumor explain` (R11 d2) --------------------------------------------

// TestExplainPrintsTheDerivationForARealCase drives the WHOLE path -- the
// loader, the derivation and the render -- against the committed toy rather
// than a swapped seam, because every other arm in this file proves the
// argument surface and none of them proves the command explains anything.
func TestExplainPrintsTheDerivationForARealCase(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if rc := run([]string{"explain", filepath.Join("..", "..", "examples", "toy"), "stale-count"},
		bytes.NewReader(nil), &stdout, &stderr); rc != 0 {
		t.Fatalf("exit %d, stderr %q", rc, stderr.String())
	}
	text := stdout.String()
	for _, want := range []string{
		"EXPLAIN toy stale-count",
		"FINDING    bug",
		"statute.rego:396 -- the DEFAULT row: no clause matched",
		"OBSERVED   1",
		"WHY THE STATUTE HELD THAT -- alice's knowledge, shortest path first",
		"bob.join#1         told to alice by bob at 4",
		"REPRODUCTION  toy stale-count at seal 5",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the output does not carry %q:\n%s", want, text)
		}
	}
	if stderr.Len() != 0 {
		t.Errorf("a printed derivation wrote to stderr: %q", stderr.String())
	}
}

// TestExplainsSampleInTheDocsIsTheCommandsOwnOutput holds docs/cli.md's sample
// to the real thing.
//
// THE SAMPLE IS PROSE UNTIL SOMETHING RUNS IT. cli.md's convention is a plain
// fence per command -- the gate executes only fences marked `run` -- so a
// sample like this one drifts the day the render changes and nothing says so.
// This arm is the mechanism: it extracts the block and compares it line for
// line.
//
// THE DIGEST LINE IS COMPARED BY ITS PREFIX, deliberately. The reproduction
// names a sha256 over examples/toy/statute.rego, so a document that pinned it
// would make every statute edit a documentation edit; the doc shows a
// placeholder, and this arm checks the real line carries 64 hex characters
// where the placeholder sits.
func TestExplainsSampleInTheDocsIsTheCommandsOwnOutput(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "cli.md"))
	if err != nil {
		t.Fatal(err)
	}
	sample, ok := sampleAfter(string(doc), "## `rumor explain`")
	if !ok {
		t.Fatal("docs/cli.md carries no sample under `rumor explain`, so this arm has nothing to hold")
	}
	if len(sample) < 20 {
		t.Fatalf("the sample is %d line(s), which is too few to be the render: %v", len(sample), sample)
	}

	var stdout, stderr bytes.Buffer
	if rc := run([]string{"explain", filepath.Join("..", "..", "examples", "toy"), "stale-count"},
		bytes.NewReader(nil), &stdout, &stderr); rc != 0 {
		t.Fatalf("exit %d, stderr %q", rc, stderr.String())
	}
	got := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	if len(got) != len(sample) {
		t.Fatalf("the sample has %d line(s) and the command printed %d:\n--- doc\n%s\n--- got\n%s",
			len(sample), len(got), strings.Join(sample, "\n"), strings.Join(got, "\n"))
	}
	for i := range sample {
		if strings.HasPrefix(sample[i], "REPRODUCTION") {
			head := "REPRODUCTION  toy stale-count at seal 5, statute "
			if !strings.HasPrefix(got[i], head) {
				t.Errorf("line %d: %q does not begin %q", i+1, got[i], head)
				continue
			}
			rest := strings.TrimPrefix(got[i], head)
			digest := strings.SplitN(rest, ",", 2)[0]
			if len(digest) != 64 || strings.Trim(digest, "0123456789abcdef") != "" {
				t.Errorf("line %d: %q is not a full sha256 where the doc shows a placeholder", i+1, digest)
			}
			if !strings.Contains(sample[i], "<sha256 of statute.rego>") {
				t.Errorf("line %d: the doc pins a digest instead of marking a placeholder: %q", i+1, sample[i])
			}
			continue
		}
		if got[i] != sample[i] {
			t.Errorf("line %d has drifted:\n  doc %q\n  got %q", i+1, sample[i], got[i])
		}
	}
}

// TestTheDocsHeldBlockFragmentIsTheCommandsOwnOutput pins the SECOND sample in
// `rumor explain`'s section: the one that shows a rule with its line beside a
// rule nothing touched.
//
// WHY A SECOND FRAGMENT EXISTS. The full sample is toy/stale-count, whose
// chain shows both an own claim and a tell -- and which reaches none of its
// fourteen rules, so the block that names the wrong rule shows no line at all
// there. A document whose only sample hides the feature is documentation of
// something else.
func TestTheDocsHeldBlockFragmentIsTheCommandsOwnOutput(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "cli.md"))
	if err != nil {
		t.Fatal(err)
	}
	frag, ok := fenceStartingWith(string(doc), "## `rumor explain`", "THE STATUTE ALSO HELD")
	if !ok {
		t.Fatal("docs/cli.md carries no THE STATUTE ALSO HELD fragment under `rumor explain`")
	}

	var stdout, stderr bytes.Buffer
	if rc := run([]string{"explain", filepath.Join("..", "..", "examples", "toy"), "visibility-leak"},
		bytes.NewReader(nil), &stdout, &stderr); rc != 0 {
		t.Fatalf("exit %d, stderr %q", rc, stderr.String())
	}
	var got []string
	in := false
	for _, line := range strings.Split(stdout.String(), "\n") {
		if strings.HasPrefix(line, "THE STATUTE ALSO HELD") {
			in = true
		} else if in && strings.TrimSpace(line) == "" {
			break
		}
		if in {
			got = append(got, line)
		}
	}
	if len(got) != len(frag) {
		t.Fatalf("the fragment has %d line(s) and the command printed %d:\n--- doc\n%s\n--- got\n%s",
			len(frag), len(got), strings.Join(frag, "\n"), strings.Join(got, "\n"))
	}
	for i := range frag {
		if got[i] != frag[i] {
			t.Errorf("line %d has drifted:\n  doc %q\n  got %q", i+1, frag[i], got[i])
		}
	}
	// The fragment is only worth pinning if it shows BOTH kinds.
	joined := strings.Join(frag, "\n")
	if !strings.Contains(joined, "statute.rego:") || !strings.Contains(joined, "(not reached)") {
		t.Errorf("the fragment does not show a line and an unreached rule together:\n%s", joined)
	}
}

// fenceStartingWith returns the lines of the first fenced block under a heading
// whose first line begins with a prefix.
func fenceStartingWith(doc, heading, prefix string) ([]string, bool) {
	lines := strings.Split(doc, "\n")
	start := -1
	for i, line := range lines {
		if line == heading {
			start = i
			break
		}
	}
	if start < 0 {
		return nil, false
	}
	var cur []string
	in := false
	for _, line := range lines[start+1:] {
		if strings.HasPrefix(line, "## ") {
			break
		}
		if line == "```" {
			if in && len(cur) > 0 && strings.HasPrefix(cur[0], prefix) {
				return cur, true
			}
			cur, in = nil, !in
			continue
		}
		if in {
			cur = append(cur, line)
		}
	}
	return nil, false
}

// sampleAfter returns the lines of the LAST fenced block under a heading,
// which is where each command's section in cli.md puts its sample output: the
// first fence is the usage line.
func sampleAfter(doc, heading string) ([]string, bool) {
	lines := strings.Split(doc, "\n")
	start := -1
	for i, line := range lines {
		if line == heading {
			start = i
			break
		}
	}
	if start < 0 {
		return nil, false
	}
	var best []string
	var cur []string
	in := false
	for _, line := range lines[start+1:] {
		if strings.HasPrefix(line, "## ") {
			break
		}
		if line == "```" {
			if in {
				if len(cur) > len(best) {
					best = cur
				}
				cur = nil
			}
			in = !in
			continue
		}
		if in {
			cur = append(cur, line)
		}
	}
	return best, len(best) > 0
}

func TestExplainRefusesAWrongArgumentCount(t *testing.T) {
	for _, args := range [][]string{
		{"explain"},
		{"explain", "examples/toy"},
		{"explain", "examples/toy", "stale-count", "extra"},
	} {
		var stdout, stderr bytes.Buffer
		if rc := run(args, bytes.NewReader(nil), &stdout, &stderr); rc != 2 {
			t.Errorf("%v exited %d, and a usage error is 2", args, rc)
		}
		if !strings.Contains(stderr.String(), "rumor explain PROJECT CASE") {
			t.Errorf("%v: the usage does not name the command's own arguments: %q", args, stderr.String())
		}
		if stdout.Len() != 0 {
			t.Errorf("%v wrote to stdout before refusing: %q", args, stdout.String())
		}
	}
}

func TestExplainNamesWhyItCouldNotExplain(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		says string
	}{
		{"no such package", []string{"explain", filepath.Join("..", "..", "examples", "nope"), "x"}, "statute.rego"},
		{"no such case", []string{"explain", filepath.Join("..", "..", "examples", "toy"), "not-a-case"}, "not-a-case"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if rc := run(tc.args, bytes.NewReader(nil), &stdout, &stderr); rc != 1 {
				t.Fatalf("exit %d, and a refusal is 1 (stderr %q)", rc, stderr.String())
			}
			if !strings.HasPrefix(stderr.String(), "explain: ") {
				t.Errorf("the refusal is not attributed to the command: %q", stderr.String())
			}
			if !strings.Contains(stderr.String(), tc.says) {
				t.Errorf("the refusal does not name %q: %q", tc.says, stderr.String())
			}
			if stdout.Len() != 0 {
				t.Errorf("it printed %q before refusing", stdout.String())
			}
		})
	}
}

func TestExplainDoesNotReportSuccessWhenItsOutputCannotBeWritten(t *testing.T) {
	var stderr bytes.Buffer
	w := &failWriter{}
	if rc := run([]string{"explain", filepath.Join("..", "..", "examples", "toy"), "stale-count"},
		bytes.NewReader(nil), w, &stderr); rc != 1 {
		t.Fatalf("exit %d over a writer that accepted nothing", rc)
	}
	if w.n == 0 {
		t.Error("nothing was written at all, so the failure is not the writer's")
	}
	if !strings.Contains(stderr.String(), "could not be written") {
		t.Errorf("the reason is not on stderr: %q", stderr.String())
	}
}
