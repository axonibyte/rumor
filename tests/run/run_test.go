// `rumor run`: the seed printed first and accepted back, and a walk driven end
// to end through the wire client against the fake adapter.
package run_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"forge.axonibyte.com/axonibyte/rumor/internal/platform"
)

var rumorBin string

// TestMain builds rumor once and puts it on PATH, because the FIXTURE's
// [adapter].command is `rumor fake-adapter` -- what a real project writes. A
// failed build fails the suite rather than skipping it.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "rumor-run-bin")
	if err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: %v\n", err)
		os.Exit(1)
	}
	// EVERY EXIT PATH CLEANS UP, not just the last one. The `defer` that stood
	// here never ran: a deferred call does not execute when the goroutine
	// leaves through os.Exit, so this package left its built binary in /tmp on
	// every run (issue 88, measured at the R5 gate and again on two boxes).
	//
	// AND THE OBVIOUS FIX COVERS ONE EXIT OF THREE. Issue 88 proposes
	// `code := m.Run(); RemoveAll(dir); os.Exit(code)`, which leaves the two
	// failure paths below still leaking -- a build that does not compile and a
	// PATH that cannot be set both exit without cleaning. One closure covers
	// every exit, including whichever one is added next.
	die := func(code int) {
		_ = os.RemoveAll(dir)
		os.Exit(code)
	}

	rumorBin = filepath.Join(dir, "rumor")
	// THE TOY PAIR IS BUILT TOO, because `examples/toy` against
	// `cmd/toy-adapter` "is the pairing that exercises the toy"
	// (tests/wire/walk_test.go) and the promoted-seed arm below runs exactly
	// that pairing through this verb. toy-adapter spawns `toy` itself when
	// TOY_ADDR is unset, so both have to be on the PATH this TestMain sets --
	// one missing is a could-not-run that looks like a walk finding nothing.
	for _, pkg := range []struct{ out, path string }{
		{"rumor", "./cmd/rumor"},
		{"toy", "./cmd/toy"},
		{"toy-adapter", "./cmd/toy-adapter"},
	} {
		build := exec.Command("go", "build", "-o", filepath.Join(dir, pkg.out), pkg.path)
		build.Dir = filepath.Join("..", "..")
		build.Env = append(os.Environ(), "GOTOOLCHAIN=local")
		if out, berr := build.CombinedOutput(); berr != nil {
			fmt.Fprintf(os.Stderr, "TestMain: building %s: %v\n%s\n", pkg.path, berr, out)
			die(1)
		}
	}
	if err := os.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH")); err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: %v\n", err)
		die(1)
	}
	die(m.Run())
}

const fixture = "fixtures/walkable"

func run(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(rumorBin, args...)
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("running %v: %v", args, err)
	}
	return out.String(), errb.String(), code
}

// THE SEED IS THE FIRST LINE, which is issue 72's first bullet: one integer,
// printed first on every run, accepted back by --seed.
func TestTheSeedIsThePrintedFirstLine(t *testing.T) {
	out, errb, code := run(t, "run", fixture, "--seed", "42", "--steps", "4")
	if code != 0 {
		t.Fatalf("exit %d\nstdout:\n%s\nstderr:\n%s", code, out, errb)
	}
	first := strings.SplitN(out, "\n", 2)[0]
	if first != "run: seed 42" {
		t.Errorf("the FIRST line must carry the seed; got %q", first)
	}
}

// AND IT IS PRINTED EVEN WHEN THE RUN CANNOT PROCEED. A seed reported only on
// success would be missing from exactly the runs somebody wants to reproduce,
// so this drives a project whose adapter cannot be spawned and still requires
// the seed on stdout.
func TestTheSeedIsPrintedBeforeTheAdapterIsSpawned(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"platform.toml", "bindings.toml", "statute.rego"} {
		b, err := os.ReadFile(filepath.Join(fixture, f))
		if err != nil {
			t.Fatalf("reading the fixture's %s: %v", f, err)
		}
		if f == "platform.toml" {
			b = []byte(strings.Replace(string(b), `command = ["rumor", "fake-adapter"]`,
				`command = ["rumor-no-such-adapter-xyz"]`, 1))
			if !strings.Contains(string(b), "rumor-no-such-adapter-xyz") {
				t.Fatal("could not point the fixture at a missing adapter, so this arm would be " +
					"testing the working path instead of the failing one")
			}
		}
		if err := os.WriteFile(filepath.Join(dir, f), b, 0o600); err != nil {
			t.Fatalf("writing %s: %v", f, err)
		}
	}

	out, errb, code := run(t, "run", dir, "--seed", "7")
	if code != 2 {
		t.Errorf("an adapter that cannot be spawned is could-not-run, exit 2; got %d", code)
	}
	if !strings.HasPrefix(out, "run: seed 7\n") {
		t.Errorf("the seed must reach stdout before the spawn is attempted; stdout was %q", out)
	}
	if !strings.Contains(errb, "rumor-no-such-adapter-xyz") {
		t.Errorf("the could-not-run must name the binary; stderr was %q", errb)
	}
}

// --seed REPRODUCES THE WHOLE TRACE, which is the done-when's second half.
func TestTheSameSeedReproducesTheTrace(t *testing.T) {
	a, _, ca := run(t, "run", fixture, "--seed", "99", "--steps", "6")
	b, _, cb := run(t, "run", fixture, "--seed", "99", "--steps", "6")
	if ca != 0 || cb != 0 {
		t.Fatalf("exits %d and %d", ca, cb)
	}
	if a != b {
		t.Errorf("one seed gave two traces:\n--- first\n%s\n--- second\n%s", a, b)
	}

	// THE ARM ASSERTS ITS OWN REACH: two identical runs of a trace with no
	// steps in it would satisfy the comparison above and prove nothing.
	steps := strings.Count(a, "run:   ") + strings.Count(a, "run:  1")
	if !strings.Contains(a, "member_") || steps == 0 {
		t.Fatalf("the trace records no steps, so comparing two of them says nothing:\n%s", a)
	}
}

// A DIFFERENT SEED MUST WALK DIFFERENTLY, the control that keeps the arm above
// from being true and empty.
func TestADifferentSeedGivesADifferentTrace(t *testing.T) {
	base, _, _ := run(t, "run", fixture, "--seed", "1", "--steps", "8")
	baseBody := strings.SplitN(base, "\n", 2)[1]
	for _, seed := range []string{"2", "3", "4", "5", "99", "12345"} {
		out, _, _ := run(t, "run", fixture, "--seed", seed, "--steps", "8")
		if strings.SplitN(out, "\n", 2)[1] != baseBody {
			return
		}
	}
	t.Errorf("seven seeds all walked identically, so the trace does not depend on the seed:\n%s", base)
}

// WITH NO --seed THE RUN MINTS ONE, prints it, and two runs differ. A minted
// seed that was always the same would make every unreproduced run the same run.
func TestWithoutASeedOneIsMintedAndPrinted(t *testing.T) {
	seedOf := func(out string) uint64 {
		t.Helper()
		first := strings.SplitN(out, "\n", 2)[0]
		n, err := strconv.ParseUint(strings.TrimPrefix(first, "run: seed "), 10, 64)
		if err != nil {
			t.Fatalf("the first line %q does not carry a parseable seed: %v", first, err)
		}
		return n
	}
	a, _, ca := run(t, "run", fixture, "--steps", "3")
	b, _, cb := run(t, "run", fixture, "--steps", "3")
	if ca != 0 || cb != 0 {
		t.Fatalf("exits %d and %d", ca, cb)
	}
	if seedOf(a) == seedOf(b) {
		t.Errorf("two unseeded runs minted the same seed %d, so an unreproduced run is always "+
			"the same run", seedOf(a))
	}

	// AND THE PRINTED SEED REPRODUCES IT, which is what makes printing it worth
	// anything: the minted number has to be the one --seed accepts.
	replay, _, _ := run(t, "run", fixture, "--steps", "3", "--seed", strconv.FormatUint(seedOf(a), 10))
	if replay != a {
		t.Errorf("replaying the printed seed gave a different run:\n--- minted\n%s\n--- replayed\n%s", a, replay)
	}
}

// THE REGISTERED PROVIDER COUNT IS IN THE VERDICT, because it was ZERO in this
// binary until the run verb imported the one shipped provider (fable coop
// r5/58, from ibis r4/128): R5.3's shrink step cannot Open() a provider nobody
// registered, and a count in the verdict is how a reader sees it is not zero.
func TestTheVerdictNamesTheRegisteredProviders(t *testing.T) {
	out, _, code := run(t, "run", fixture, "--seed", "5", "--steps", "2")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, out)
	}
	if !strings.Contains(out, "1 rollback provider(s) registered") {
		t.Errorf("the verdict must report a NON-ZERO provider count; got:\n%s", out)
	}
}

// THE USAGE ERRORS, each exit 2 and each naming what was wrong rather than
// printing the usage line alone.
func TestRunsUsageErrors(t *testing.T) {
	for _, c := range []struct {
		args []string
		says string
	}{
		{[]string{"run"}, "wants a project directory"},
		{[]string{"run", fixture, "--steps", "0"}, "not a walk"},
		{[]string{"run", fixture, "--seed"}, "wants a value"},
		{[]string{"run", fixture, "--seed", "not-a-number"}, "wanted a non-negative integer"},
		{[]string{"run", fixture, "--nonesuch"}, "not a flag of run"},
		{[]string{"run", fixture, "another"}, "one project directory"},
	} {
		_, errb, code := run(t, c.args...)
		if code != 2 {
			t.Errorf("%v: exit %d, want 2 for a usage error", c.args, code)
		}
		if !strings.Contains(errb, c.says) {
			t.Errorf("%v: stderr must say %q; got %q", c.args, c.says, errb)
		}
	}
}

// A PROJECT WHOSE WEIGHT TABLE IS INCOMPLETE IS REFUSED BY NAME, which is the
// rule fable ruled at coop r5/49 arriving through a run. Measured the same day:
// examples/twin weights publish_doc and not publish_doc_ui, so this is the
// state our own examples were in until arm 19's cell closed it.
func TestAnActionWithNoWeightStopsTheRun(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"platform.toml", "bindings.toml", "statute.rego"} {
		b, err := os.ReadFile(filepath.Join(fixture, f))
		if err != nil {
			t.Fatalf("reading %s: %v", f, err)
		}
		if f == "platform.toml" {
			b = []byte(strings.Replace(string(b), "create_event = 4\n", "", 1))
			if strings.Contains(string(b), "create_event = 4") {
				t.Fatal("could not remove create_event's weight, so this arm cannot reach the refusal")
			}
		}
		if err := os.WriteFile(filepath.Join(dir, f), b, 0o600); err != nil {
			t.Fatalf("writing %s: %v", f, err)
		}
	}

	out, errb, code := run(t, "run", dir, "--seed", "3")
	if code != 2 {
		t.Errorf("exit %d, want 2: an undeclared weight is could-not-run\nstdout %s\nstderr %s", code, out, errb)
	}
	if !strings.Contains(errb, "create_event") || !strings.Contains(errb, "not a zero") {
		t.Errorf("the refusal must name the action and say why absent is not zero; got %q", errb)
	}
}

// THE DOCUMENTED OUTPUT IS THE BINARY'S OUTPUT.
//
// docs/cli.md's `rumor run` console block is a sample nothing would otherwise
// check -- "a documented sample that cannot fail is decoration"
// (docs/testing-methodology.md) -- and the same shape tests/validate holds its
// two blocks to. The run it documents is seeded, so the comparison is exact
// rather than approximate.
func TestTheDocumentedRunIsTheBinarysRun(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "cli.md"))
	if err != nil {
		t.Fatalf("reading docs/cli.md: %v", err)
	}
	const marker = "$ rumor run tests/run/fixtures/walkable --seed 42 --steps 5\n"
	i := strings.Index(string(raw), marker)
	if i < 0 {
		t.Fatalf("docs/cli.md no longer shows that command, so this arm is checking nothing: " +
			"either the block moved or its invocation changed")
	}
	rest := string(raw)[i+len(marker):]
	end := strings.Index(rest, "```")
	if end < 0 {
		t.Fatal("the console block after that command is unterminated")
	}
	want := strings.TrimRight(rest[:end], "\n")

	cmd := exec.Command(rumorBin, "run", fixture, "--seed", "42", "--steps", "5")
	cmd.Dir = "."
	out, rerr := cmd.Output()
	if rerr != nil {
		t.Fatalf("running the documented command: %v", rerr)
	}
	got := strings.TrimRight(string(out), "\n")
	if got != want {
		t.Errorf("docs/cli.md claims:\n%s\n\nthe binary prints:\n%s", want, got)
	}
}

// THE TRACE NAMES WHAT A STEP CONSUMED, AND IT IS A HANDLE THE RUN ALREADY
// MINTED.
//
// This is the only place a `handle` argument is visible from outside the walk,
// and the fixture now declares all four sources -- so a run reaching exit 0 at
// all is also the end-to-end check that the engine composes `args` from the
// bindings and the adapter accepts the request. The arrow alone would be
// decoration; what makes it an assertion is that the consumed handle must
// appear as a MINTED handle on an earlier line, which an invented or
// misresolved name could not.
func TestTheTraceNamesTheConsumedHandle(t *testing.T) {
	out, errb, code := run(t, "run", fixture, "--seed", "42", "--steps", "8")
	if code != 0 {
		t.Fatalf("exit %d\nstdout:\n%s\nstderr:\n%s", code, out, errb)
	}

	minted := map[string]int{}
	consumed := 0
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "run: ") {
			continue
		}
		n := 0
		if _, err := fmt.Sscanf(strings.TrimPrefix(line, "run: "), "%d", &n); err != nil {
			continue // the seed line and the verdict, which carry no step number
		}
		left, right, hasArrow := strings.Cut(line, " <- ")
		if hasArrow {
			for _, h := range strings.Fields(strings.Trim(right, "[]")) {
				consumed++
				at, ok := minted[h]
				if !ok {
					t.Errorf("step %d consumed %q, which no earlier step minted: the handle was "+
						"invented or resolved from somewhere the walk did not bind\n%s", n, h, out)
					continue
				}
				if at >= n {
					t.Errorf("step %d consumed %q, minted at step %d: a step cannot consume a "+
						"handle from its own step or later", n, h, at)
				}
			}
		}
		if i := strings.Index(left, " ["); i >= 0 {
			for _, h := range strings.Fields(strings.Trim(left[i+1:], "[]")) {
				if _, seen := minted[h]; !seen {
					minted[h] = n
				}
			}
		}
	}
	if consumed == 0 {
		t.Fatalf("no step in eight consumed a handle, so this arm is reading nothing -- the "+
			"fixture's join_event declares a `handle` argument and should have been drawn:\n%s", out)
	}
}

// A FOUND BUG IS THE RUN'S PRODUCT: IT IS REPORTED, AND THE STATUS SAYS SO.
//
// `walkable`'s statute declares no `finding` rule deliberately, so that every
// other arm here is about the VERB and no walk stops early. This arm is that
// fixture with ONE change -- a finding rule -- built in a temp directory the
// way the no-weight arm builds its own, so the committed fixture keeps its
// property.
//
// IT EXISTS BECAUSE THE CARRIER WAS NOT THE CONSUMER. schedule.Record grew
// Finding and FoundAt at R5.5 and `cmd/` read NEITHER -- zero hits (ibis, coop
// main/149) -- so a found bug printed as a slightly short trace with a clean
// verdict and exit 0. Every unit arm over the walk passed throughout: the walk
// really did carry the finding. Nothing downstream looked, which is a gap only
// a test that runs the BINARY can see.
//
// THE CONTROL IS THE OTHER HALF. A walk that stopped at step 5 for any reason
// would satisfy the assertions below on their own, so the unmodified fixture is
// run at the same seed and the same length and must reach step 10 and exit 0.
// Without it this arm cannot tell "the finding ended the walk" from "the walk
// ends at 5".
func TestAFoundBugIsReportedAndTheStatusSaysSo(t *testing.T) {
	// TWO CASES OVER ONE BASE, so the attribution the line prints is
	// demonstrably the STATUTE'S and not a constant: the same fixture with the
	// attribution rule declared true and with it absent must print different
	// words in that position. A single case would pass just as well on a
	// hard-coded one.
	for _, tc := range []struct {
		name  string
		extra string
		want  string
	}{
		{"attribution undeclared", "", "undeclared"},
		{"attribution declared", "\ndefault rests_on_planted := true\n", "true"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range []string{"platform.toml", "bindings.toml", "statute.rego"} {
				b, err := os.ReadFile(filepath.Join(fixture, f))
				if err != nil {
					t.Fatalf("reading %s: %v", f, err)
				}
				if f == "statute.rego" {
					if strings.Contains(string(b), "finding") {
						t.Fatal("the fixture's statute already names a finding, so appending " +
							"one proves nothing about this arm's change")
					}
					b = append(b, []byte("\n"+
						"default finding := \"consistent\"\n\n"+
						"finding := \"bug\" if {\n"+
						"\tcount([c | some c in input.claims; c.type == \"event.created\"]) >= 2\n"+
						"}\n"+tc.extra)...)
				}
				if err := os.WriteFile(filepath.Join(dir, f), b, 0o600); err != nil {
					t.Fatalf("writing %s: %v", f, err)
				}
			}

			const steps = 10
			out, errb, code := run(t, "run", dir, "--seed", "42", "--steps", strconv.Itoa(steps))
			if code != 3 {
				t.Fatalf("exit %d, want 3: a finding is the run's product and not a refusal of "+
					"the project (1) or a could-not-run (2)\nstdout:\n%s\nstderr:\n%s",
					code, out, errb)
			}

			// THE LINE NAMES THE VALUE, THE STEP, THE RULE AND THE
			// ATTRIBUTION -- the three parts of a finding's identity plus where
			// it happened, all read off the record rather than recomposed.
			var value, rule, attrName, attr string
			var at int
			for _, line := range strings.Split(out, "\n") {
				if !strings.HasPrefix(line, "run: FOUND ") {
					continue
				}
				if _, err := fmt.Sscanf(line, "run: FOUND %s after step %d -- %s %s %s",
					&value, &at, &rule, &attrName, &attr); err != nil {
					t.Fatalf("the finding line is not readable: %q: %v", line, err)
				}
			}
			if value != "bug" {
				t.Fatalf("no finding line naming a bug in:\n%s", out)
			}
			if strings.TrimSuffix(rule, ",") != "data.rumor.statute.finding" {
				t.Errorf("the finding line names rule %q, and the statute's rule is "+
					"data.rumor.statute.finding", rule)
			}
			if attrName != "rests_on_planted" {
				t.Errorf("the finding line names the attribution %q, and a finding's identity "+
					"is the rule, the value and rests_on_planted", attrName)
			}
			if attr != tc.want {
				t.Errorf("the attribution reads %q, want %q: the printed attribution is not "+
					"the statute's answer", attr, tc.want)
			}

			// AND THE STEP IT NAMES IS THE LAST STEP THE WALK TOOK, which is
			// the first half of "the first bug ends the walk": the trace a
			// shrink reads must include the step that produced the finding, and
			// nothing may follow it.
			last := 0
			for _, line := range strings.Split(out, "\n") {
				n := 0
				if _, err := fmt.Sscanf(strings.TrimPrefix(line, "run: "), "%d", &n); err == nil && n > last {
					last = n
				}
			}
			if at != last {
				t.Errorf("the finding is reported after step %d and the trace's last step is "+
					"%d: either a step ran after the bug or the reported step is not the one "+
					"that produced it\n%s", at, last, out)
			}
			if last >= steps {
				t.Errorf("the walk took all %d steps, so it did not stop on the finding and "+
					"this arm would pass on a run that never stopped at all\n%s", steps, out)
			}

			cout, cerr, ccode := run(t, "run", fixture, "--seed", "42", "--steps", strconv.Itoa(steps))
			if ccode != 0 {
				t.Fatalf("the control exited %d, want 0: the unmodified fixture declares no "+
					"finding rule\nstdout:\n%s\nstderr:\n%s", ccode, cout, cerr)
			}
			if strings.Contains(cout, "FOUND") {
				t.Errorf("the control reports a finding, so the line above is not the appended "+
					"rule's doing\n%s", cout)
			}
			if !strings.Contains(cout, fmt.Sprintf("%d step(s)", steps)) {
				t.Errorf("the control did not take all %d steps, so the short walk above is "+
					"not the finding's doing\n%s", steps, cout)
			}
		})
	}
}

// EVERY PROMOTED SEED STILL REPRODUCES ITS FINDING, TWICE IDENTICALLY.
//
// `examples/toy/seeds.lock` is this project's first promotion (D16): "a seed
// that produced a defect is promoted into seeds.lock with a comment naming the
// defect" (docs/design.md). Until R22 teaches a run to take promoted seeds
// FIRST, nothing re-runs them -- so without this arm the file is a comment, and
// a promotion that stopped reproducing would say nothing. Prose is not a
// mechanism.
//
// TWICE, AND COMPARED BYTE FOR BYTE, which is the property D16 asks for and
// not a restatement of the determinism arm above: that one walks the fake
// through a fixture, and this one walks the TOY through its own adapter, where
// every id behind the trace is a real one the application minted. The trace
// prints handles and never ids, so two runs agreeing byte for byte is the
// walk's determinism surviving a real system under test.
//
// EXIT 3 IS PART OF THE ASSERTION. A promoted seed that walks cleanly is a
// promotion that has lost its defect -- the toy repaired by accident, which
// cmd/toy's own suite exists to prevent and which this arm catches from the
// outside.
func TestEveryPromotedToySeedStillFindsItsBug(t *testing.T) {
	const lock = "../../examples/toy/seeds.lock"
	raw, err := os.ReadFile(lock)
	if err != nil {
		t.Fatalf("reading %s: %v", lock, err)
	}
	// THE PARSE IS platform's, AND THE PROFILE COMES WITH IT. This arm had its
	// own reading of the grammar -- the fourth copy in the tree -- and a seed
	// line carrying a `profile=NAME` token broke it in the loud direction,
	// which is the lucky one: the quiet direction was internal/shrink reading
	// the same line as "that seed is not here" (R6.3).
	//
	// EACH SEED RUNS UNDER ITS OWN PROFILE, which is what makes a promotion
	// replayable at all: R6's bug ledger says a row is caught by the feature
	// listed for it, and seed 12's row is caught under `settling`. Replaying it
	// under the default mix would walk a different trace entirely and this arm
	// would report a lost promotion about a seed nobody had asked for.
	type promoted struct {
		seed, profile string
		steps         int
	}
	var seeds []promoted
	for _, line := range strings.Split(string(raw), "\n") {
		parsed, isSeed, perr := platform.ParseSeedLine(line)
		if !isSeed {
			continue
		}
		if perr != nil {
			t.Fatalf("%s: %q is not a promoted seed in the format docs/contract.md declares: %v",
				lock, strings.TrimSpace(line), perr)
		}
		if _, cerr := strconv.ParseUint(parsed.Seed, 10, 64); cerr != nil {
			t.Fatalf("%s: %q is not one non-negative integer per line, which is the format "+
				"docs/contract.md declares: %v", lock, parsed.Seed, cerr)
		}
		seeds = append(seeds, promoted{seed: parsed.Seed, profile: parsed.Profile,
			steps: parsed.Steps})
	}
	if len(seeds) == 0 {
		// THE MESSAGE NAMES WHAT THE LOCK SHOULD HOLD TODAY, not what it held
		// when this arm was written. It said "D16 promoted seed 7" until
		// 2026-10-24, and seed 7 retired from the lock that day -- so the one
		// line a reader sees when the file is empty was describing a seed the
		// file no longer carries (ratchet, coop r5/229). It prints ONLY when
		// the lock is empty, which is exactly when somebody is working out what
		// the file should hold, so stale prose here costs most.
		t.Fatalf("%s holds no seed, so this arm is checking nothing. D12 wants each of the "+
			"toy's two api-sim bugs found from a COMMITTED seed: seed 2 for the stale count "+
			"in four actions and seed 40 for the row reached with no delete, in three. A "+
			"deliberate decision to carry no promotion here has to change this arm, not "+
			"pass it silently", lock)
	}

	for _, p := range seeds {
		seed := p.seed
		t.Run("seed"+seed, func(t *testing.T) {
			// ONE CONSTANT FOR THE LENGTH, because the exit-0 message below
			// quotes it: two literals saying 10, one inside a sentence about
			// the other, is a drift waiting to happen.
			//
			// AND THE LENGTH COMES OFF THE LINE, which is ratchet's finding
			// at coop r6/106. This read `steps := 10` and 24 when a PROFILE
			// was present -- so 24, a fact about seed 12's row, lived in the
			// arm that replays every seed: the next profiled promotion would
			// inherit it silently, and a row needing another length would read
			// as a LOST PROMOTION rather than as a seed replayed too short.
			//
			// TEN IS STILL THE DEFAULT, and it is the arm's and not the
			// document's: a line that says nothing replays at the control's
			// own invocation, which is what seeds 2 and 40 have always meant.
			steps := 10
			if p.steps != 0 {
				steps = p.steps
			}
			args := []string{"run", "../../examples/toy", "--seed", seed}
			if p.profile != "" {
				args = append(args, "--profile", p.profile)
			}
			args = append(args, "--steps", strconv.Itoa(steps))
			first, errb, code := run(t, args...)
			// THE STATUS IS READ AS THE FOUR THINGS IT MEANS, not as "3 or not
			// 3". ratchet found this message asserting the wrong one of them
			// (coop r5/198): the adapter's own request budget expired, the run
			// reported could-not-run, and this arm said "a clean walk means the
			// promotion has lost its defect" -- about a run that never walked.
			// The verb has four statuses and docs/cli.md documents them because
			// collapsing outcomes into one number loses which happened; an arm
			// over that verb which collapses three of them back again is the
			// same defect one level up.
			//
			// NONE OF THEM IS A SKIP. A could-not-run names nothing about the
			// promotion, and that is precisely why it must still fail: a seed
			// whose defect could not be checked has not been shown to
			// reproduce, and passing here would report an unmeasured promotion
			// as a measured one.
			switch code {
			case 3:
				// The promotion holds. The assertions below say what it holds.
			case 0:
				t.Fatalf("exit 0: seed %s walked CLEAN over %d step(s) asked for, so the "+
					"promotion has lost its defect -- either the system under test was "+
					"repaired or the draw moved off it\nstdout:\n%s", seed, steps, first)
			case 1:
				t.Fatalf("exit 1: the project was REFUSED, so examples/toy's own "+
					"declarations are wrong and this says nothing about whether seed %s "+
					"still reproduces\nstdout:\n%s\nstderr:\n%s", seed, first, errb)
			case 2:
				t.Fatalf("exit 2: COULD-NOT-RUN. Nothing was observed, so this names "+
					"nothing about seed %s's defect -- it is not a clean walk and not a "+
					"lost promotion. The adapter carries its own per-request budget and "+
					"an expiry arrives here\nstdout:\n%s\nstderr:\n%s", seed, first, errb)
			default:
				t.Fatalf("exit %d is not a status `rumor run` documents (0 clean, 1 "+
					"refused, 2 could-not-run, 3 a finding)\nstdout:\n%s\nstderr:\n%s",
					code, first, errb)
			}
			if !strings.Contains(first, "run: FOUND ") {
				t.Errorf("exit 3 with no finding line, so the status and the report disagree:\n%s",
					first)
			}
			// AND IT IS THE HONEST FINDING, which is ONE OF THE TWO things
			// seeds.lock claims for every seed it carries: that each is the
			// honest finding, and that each is byte-identical across two runs.
			// This assertion checks the first; the two-run comparison below
			// checks the second.
			//
			// TWO DENOMINATORS SIT CLOSE TOGETHER HERE, AND SAYING "HALF" HID
			// THAT (rook, coop r5/236). The attribution is a THIRD of a
			// FINDING'S IDENTITY -- rule, value, attribution -- and separately
			// it is ONE OF TWO CLAIMS the LOCK makes. This comment said "half"
			// beside a quotation cut before the lock's own "a third of a
			// finding's identity", so the two fractions read as a contradiction
			// while counting different things. Neither was wrong; nothing said
			// which was which.
			//
			// AND THE CITATION WAS STRANDED BEFORE THAT: it said "what
			// seeds.lock's comment claims for seed 7" until 2026-10-24, and
			// seed 7 retired from the lock that day -- so it landed on the
			// retirement note, the one paragraph in that file saying the
			// opposite. ratchet found it at r5/235 by grepping for the seed I
			// had removed, which is the search I owed and did not run: a
			// cleanup cannot see the citations it strands.
			//
			// A promotion whose bug came to rest on
			// a planted claim is the engine catching a lie rather than the toy
			// being wrong, and it would be the wrong fixture for a shrinker:
			// `rests_on_planted` is part of the finding's identity, so a change
			// in it is a change of finding.
			if !strings.Contains(first, "rests_on_planted false") {
				t.Errorf("the promoted seed's finding is not attributed to honest claims, so it "+
					"is not the finding seeds.lock names:\n%s", first)
			}
			second, _, again := run(t, args...)
			if again != code {
				t.Errorf("the same seed exited %d and then %d", code, again)
			}
			if first != second {
				t.Errorf("seed %s does not reproduce: the two runs differ\nfirst:\n%s\n"+
					"second:\n%s", seed, first, second)
			}
			// AND THE PASS SAYS WHAT IT REPLAYED, which is ratchet's second
			// line at coop r6/107 and rook's at /1106: the length appeared in
			// ONE `t.Fatalf` on the exit-0 path and nowhere else, so a PASSING
			// replay recorded no length, no profile and no finding at all. A
			// green then says "the promotions hold" without saying what was
			// run -- and the invocation is the thing two of us have now got
			// wrong twice in one day by reading a figure without one beside it.
			//
			// THE POSITIVE LINE CARRIES THE DENOMINATOR, which is ratchet's own
			// note in its own words: name it where the check PASSED, not only
			// in the refusal.
			profile := p.profile
			if profile == "" {
				profile = "default"
			}
			t.Logf("PROOF seed %s replayed under profile %s at %d step(s), twice and "+
				"byte-identical: %s", seed, profile, steps, strings.TrimSpace(findingLine(first)))
		})
	}
}

// findingLine is the run's FOUND line, for a PROOF line that says what the
// replay found rather than only that it found something. Empty when there is
// none, which a caller prints as such rather than hiding.
func findingLine(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "run: FOUND ") {
			return line
		}
	}
	return "(no finding line)"
}

// A QUIESCE BOUND NOTHING CAN JUDGE IS REFUSED, NOT RUN QUIETLY.
//
// ratchet drove the alternative at coop r6/8: a claim type bounded `quiesce`
// with no prober wired holds a real disagreement open on every pass -- the
// register keeps it, StillOpen cannot see it because the query WAS re-asked,
// and the walk never ends on a `pending` -- so the run exits 0 on a
// disagreement. fable ruled the shutter at r6/10 and keyed it structurally at
// r6/12: the condition is the PROBER SLOT being nil, which R6.2 falsifies by
// construction when it wires one, so the refusal removes itself rather than
// waiting to be remembered.
//
// THE FIXTURE IS `walkable` WITH ONE CHANGE, built in a temp directory the way
// the no-weight arm builds its own, and the arm asserts the change landed: a
// bound on an IMMEDIATE claim is refused by the decoder, so the timing moves
// with it and a reader can see why both lines are edited.
// A DURATION BOUND IS REFUSED NAMING THE DURATION, which is the arm ratchet
// asked for at coop rumor/1192 and fable placed in 92's branch at r6/153.
//
// IT IS IN tests/run AND NOT internal/engine FOR THE REASON THE DEFECT HAD.
// Every `Bound:` in the tree was a test literal inside internal/engine --
// pending_test.go and reconcile_test.go hand the field in -- while the only
// producer the product has, schedule.EventualOf, never set it. An arm that
// passes a value in cannot see a writer that omitted it, so this one declares
// the bound in a PROJECT and drives the binary: the walk's own constructor
// builds the Eventual and the refusal has to name what the project said.
//
// AND IT DRIVES examples/toy RATHER THAN fixtures/walkable, because the
// refusal is judged LAZILY. It fires where a disagreement on that claim type
// is judged, so a project that walks cleanly never reaches it -- the walkable
// fixture exits 0 with the bound declared, which is what this arm looked like
// before it was driven. Seed 12 under `settling` is the committed row that
// disagrees on `event.tallied` (examples/toy/seeds.lock), so it is the drive
// that reaches the code.
func TestADurationBoundIsRefusedNamingTheDuration(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"platform.toml", "bindings.toml", "statute.rego", "seeds.lock"} {
		b, err := os.ReadFile(filepath.Join("../../examples/toy", f))
		if err != nil {
			t.Fatalf("reading %s: %v", f, err)
		}
		if f == "bindings.toml" {
			const was = "[claim.\"event.tallied\"]\ntiming = \"eventual\"\nbound = \"quiesce\""
			if !strings.Contains(string(b), was) {
				t.Fatal("examples/toy's `event.tallied` claim is not the quiesce-bounded block " +
					"this arm edits, so the duration bound below would not be declared and " +
					"the refusal could not fire")
			}
			b = []byte(strings.Replace(string(b), was,
				"[claim.\"event.tallied\"]\ntiming = \"eventual\"\nbound = \"30s\"", 1))
		}
		if err := os.WriteFile(filepath.Join(dir, f), b, 0o600); err != nil {
			t.Fatalf("writing %s: %v", f, err)
		}
	}

	out, errb, code := run(t, "run", dir, "--profile", "settling", "--seed", "12", "--steps", "24")
	if code != 2 {
		t.Fatalf("exit %d, want 2: a duration bound with no clock fact is COULD-NOT-RUN -- the "+
			"bound cannot be measured, which is not a refusal of the project (1) and not a "+
			"finding (3)\nstdout:\n%s\nstderr:\n%s", code, out, errb)
	}
	// ON STDERR AND NOT STDOUT, which this arm had wrong until it was driven:
	// a REFUSAL of the project prints to stdout with the run's own lines (the
	// quiesce arm below reads it there), while a COULD-NOT-RUN is the error
	// stream's. Asserting the wrong stream would have passed on an empty
	// string once the message moved anywhere.
	//
	// THE DURATION THE PROJECT DECLARED. This is the whole arm: the figure
	// comes from the field the only live writer sets, so "30s" can appear only
	// if the parse reached it.
	if !strings.Contains(errb, "bound of 30s") {
		t.Errorf("the refusal does not name the declared bound, so a project is told its "+
			"declaration cannot be measured without being told which declaration:\n%s", errb)
	}
	// AND NEVER THE ZERO VALUE, which is the live symptom rook found at coop
	// rumor/1191: the parse was validated and discarded, so 30s, 5m and 250ms
	// all printed the same figure and a reader could not tell a parsed
	// declaration from an unparsed one. Driven on v0.1.36's binary this exact
	// project prints "a bound of 0".
	if strings.Contains(errb, "bound of 0") {
		t.Errorf("the refusal reports a bound of 0 for a project declaring 30s, so the parse "+
			"is being discarded again:\n%s", errb)
	}
	if !strings.Contains(errb, "event.tallied") {
		t.Errorf("the refusal names no claim type, so a project with several eventual types "+
			"cannot tell which bound is unmeasurable:\n%s", errb)
	}
}

func TestAQuiesceBoundWithNoProberIsRefused(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"platform.toml", "bindings.toml", "statute.rego"} {
		b, err := os.ReadFile(filepath.Join(fixture, f))
		if err != nil {
			t.Fatalf("reading %s: %v", f, err)
		}
		if f == "bindings.toml" {
			const was = "[claim.event_created]\ntiming = \"immediate\""
			if !strings.Contains(string(b), was) {
				t.Fatal("the fixture's claim block is not what this arm edits, so the " +
					"quiesce bound below would not be declared and the refusal could not fire")
			}
			b = []byte(strings.Replace(string(b), was,
				"[claim.event_created]\ntiming = \"eventual\"\nbound = \"quiesce\"", 1))
		}
		if err := os.WriteFile(filepath.Join(dir, f), b, 0o600); err != nil {
			t.Fatalf("writing %s: %v", f, err)
		}
	}

	out, errb, code := run(t, "run", dir, "--seed", "42", "--steps", "5")
	if code != 1 {
		t.Fatalf("exit %d, want 1: a project declaring a bound this build cannot judge is "+
			"REFUSED, not could-not-run (2) and not a finding (3)\nstdout:\n%s\nstderr:\n%s",
			code, out, errb)
	}
	// IT NAMES THE ABSENT PROBE TABLE, and this assertion MOVED at R6.2 with
	// the behaviour it is about. It used to require the words "no quiesce
	// prober", which was right while the Prober slot was nil in every run;
	// internal/quiesce fills that slot now, built from the project's own
	// probes, so "no prober is wired" is no longer a thing this build can be.
	// What is still unjudgeable is a quiesce bound with NO PROBE DECLARED --
	// nothing to ask, and "quiet" over an empty set is a claim about nothing.
	// That is the condition fable's r6/12 ruling named: structural, and
	// falsified by construction the moment a prober exists, so the shutter
	// removes itself rather than being remembered and taken out.
	if !strings.Contains(out, "REFUSED") || !strings.Contains(out, "no [[quiesce.probe]]") {
		t.Errorf("the refusal does not name the absent probe table, so a reader cannot tell "+
			"it from a malformed project:\n%s", out)
	}
	if strings.Contains(out, "no quiesce prober") {
		t.Errorf("the refusal still blames a missing prober, which R6.2 wired:\n%s", out)
	}
	// AND IT NAMES THE TYPES, because a project with several eventual types
	// needs to know which bound is unjudgeable.
	if !strings.Contains(out, "event_created") {
		t.Errorf("the refusal names no claim type:\n%s", out)
	}
	// NOTHING IS WALKED AFTER IT. A refusal that printed its line and then ran
	// anyway would be the silent exit-0 case with a warning attached.
	if strings.Contains(out, "run:   1 ") {
		t.Errorf("the run walked a step after refusing:\n%s", out)
	}

	// THE CONTROL: the unmodified fixture declares no quiesce bound and still
	// walks to exit 0, so the refusal is about the BOUND and not about this
	// arm's fixture or the slot being nil in every run.
	cout, cerr, ccode := run(t, "run", fixture, "--seed", "42", "--steps", "5")
	if ccode != 0 {
		t.Fatalf("the control exited %d, want 0: walkable declares no quiesce bound\n"+
			"stdout:\n%s\nstderr:\n%s", ccode, cout, cerr)
	}
	if strings.Contains(cout, "REFUSED") {
		t.Errorf("the control was refused, so the refusal fires without a quiesce bound:\n%s", cout)
	}
}

// TestAQuiesceBoundWithAProbeIsNotRefusedForWantOfAProber is the other half of
// the arm above, and it is what makes the move at R6.2 an ADDITION rather than
// a relaxation: the same fixture, the same bound, PLUS a well-formed probe, must
// no longer be refused for having nothing to ask.
//
// WITHOUT THIS CELL the change above would be indistinguishable from deleting a
// refusal: a build that refused every quiesce bound and a build that refuses
// only the unaskable one both satisfy the arm above.
func TestAQuiesceBoundWithAProbeIsNotRefusedForWantOfAProber(t *testing.T) {
	for _, c := range []struct {
		what  string
		probe string
		// refused is whether the PROJECT is wrong, and says which clause.
		refused string
	}{
		{"a well-formed direct probe",
			"[quiesce]\n\n  [[quiesce.probe]]\n  kind = \"direct\"\n  timeout = \"30s\"\n  query = \"event\"\n  quiet = 0\n",
			""},
		{"a probe with no timeout",
			"[quiesce]\n\n  [[quiesce.probe]]\n  kind = \"direct\"\n  query = \"event\"\n  quiet = 0\n",
			"no timeout"},
		{"a probe of an unknown kind",
			"[quiesce]\n\n  [[quiesce.probe]]\n  kind = \"stares_at_it\"\n  timeout = \"30s\"\n  quiet = 0\n",
			"not one of"},
		{"a query on a kind that does not read one",
			"[quiesce]\n\n  [[quiesce.probe]]\n  kind = \"queue_depth\"\n  timeout = \"30s\"\n  query = \"event\"\n  quiet = 0\n",
			"only the `direct` kind"},
	} {
		t.Run(c.what, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range []string{"platform.toml", "bindings.toml", "statute.rego"} {
				b, err := os.ReadFile(filepath.Join(fixture, f))
				if err != nil {
					t.Fatalf("reading %s: %v", f, err)
				}
				if f == "bindings.toml" {
					const was = "[claim.event_created]\ntiming = \"immediate\""
					if !strings.Contains(string(b), was) {
						t.Fatal("the fixture's claim block is not what this arm edits")
					}
					b = []byte(strings.Replace(string(b), was,
						"[claim.event_created]\ntiming = \"eventual\"\nbound = \"quiesce\"", 1))
					b = append(b, []byte("\n"+c.probe)...)
				}
				if err := os.WriteFile(filepath.Join(dir, f), b, 0o600); err != nil {
					t.Fatalf("writing %s: %v", f, err)
				}
			}
			out, errb, code := run(t, "run", dir, "--seed", "42", "--steps", "5")
			if c.refused == "" {
				if strings.Contains(out, "no [[quiesce.probe]]") {
					t.Errorf("a project WITH a probe was refused for having none:\n%s", out)
				}
				if strings.Contains(out, "no quiesce prober") {
					t.Errorf("a project with a probe was refused for want of a prober:\n%s", out)
				}
				return
			}
			if code != 1 {
				t.Fatalf("exit %d, want 1: %s is the PROJECT being wrong\nstdout:\n%s\nstderr:\n%s",
					code, c.what, out, errb)
			}
			if !strings.Contains(out, c.refused) {
				t.Errorf("the refusal for %s does not say %q:\n%s", c.what, c.refused, out)
			}
		})
	}
}

// TestANeverQuietProbeEndsTheRunAsAFinding is fable's D5 end to end, and the
// half of it that only a whole run can show: "the never-quiet one ends in the
// timeout finding inside its declared timeout plus grace, measured, hang report
// written".
//
// NO PENDING IS OPEN HERE, DELIBERATELY. The fixture declares probes and NO
// quiesce bound, so nothing in the walk is eventual and no observation
// disagrees -- which is exactly fable's ruling 2 at coop r6/46: "a probe that
// gives up while every observation agrees is the fifth finding all the same".
// internal/engine's pendingOverlay cannot reach that case, since it returns
// before it looks when the statute found agreement, so a run is the only place
// this can be driven.
//
// THE TIMEOUT IS 100ms AND THAT IS NOT A SHORTCUT. A real 30s budget would make
// this arm time the BOX, which the room measured as unavailable to any of us --
// three jails, four CPUs, one kern.boottime (ratchet and rook, coop r5/192 and
// /193). The declared timeout is the project's, so an arm may declare a small
// one; what it must not do is assert a wall-clock figure it does not control,
// which is why the bound below is generous in one direction only.
func TestANeverQuietProbeEndsTheRunAsAFinding(t *testing.T) {
	for _, c := range []struct {
		what  string
		quiet string
		query string
		// found is whether the run should end on the quiesce_timeout finding,
		// and code the exit status when it should not.
		found bool
		code  int
	}{
		// The fake adapter's default surface answers `attendance` with 1, so a
		// probe wanting 0 never reads quiet and one wanting 1 reads quiet at once.
		{"a probe that never reads quiet", "0", "attendance", true, 3},
		{"a probe that reads quiet immediately", "1", "attendance", false, 0},
		// AND THE THIRD STATE, which is the one worth most: a query the adapter
		// does not record is COULD-NOT-RUN (2) and never the finding. The
		// adapter did not answer the question, so "the system will not settle"
		// would be a verdict about a system nobody asked -- the
		// two-states-one-code shape the room spent a night correcting.
		{"a probe the adapter cannot answer", "0", "no_such_observation", false, 2},
	} {
		t.Run(c.what, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range []string{"platform.toml", "bindings.toml", "statute.rego"} {
				b, err := os.ReadFile(filepath.Join(fixture, f))
				if err != nil {
					t.Fatalf("reading %s: %v", f, err)
				}
				if f == "bindings.toml" {
					b = append(b, []byte("\n[quiesce]\n\n  [[quiesce.probe]]\n  kind = \"direct\"\n"+
						"  timeout = \"100ms\"\n  every = \"10ms\"\n  query = \""+c.query+"\"\n"+
						"  quiet = "+c.quiet+"\n")...)
				}
				if err := os.WriteFile(filepath.Join(dir, f), b, 0o600); err != nil {
					t.Fatalf("writing %s: %v", f, err)
				}
			}
			start := time.Now()
			out, errb, code := run(t, "run", dir, "--seed", "42", "--steps", "3")
			elapsed := time.Since(start)

			if !c.found {
				// THE CONTROLS, and they carry the whole weight of the cell
				// above: the same fixture, the same probe kind, the same wiring,
				// one field different. Without them a build that reported
				// quiesce_timeout unconditionally would pass.
				if code != c.code {
					t.Fatalf("exit %d, want %d for %s\nstdout:\n%s\nstderr:\n%s",
						code, c.code, c.what, out, errb)
				}
				if strings.Contains(out, "quiesce_timeout") {
					t.Errorf("%s produced the timeout finding:\n%s", c.what, out)
				}
				if c.code == 2 && !strings.Contains(out+errb, "could not run") {
					t.Errorf("an unanswerable probe did not say could-not-run:\nstdout:\n%s\n"+
						"stderr:\n%s", out, errb)
				}
				return
			}

			if code != 3 {
				t.Fatalf("exit %d, want 3: a probe that gave up is a FINDING, not a refusal (1) "+
					"and not could-not-run (2)\nstdout:\n%s\nstderr:\n%s", code, out, errb)
			}
			if !strings.Contains(out, "FOUND quiesce_timeout") {
				t.Errorf("the run does not report the fifth finding by name:\n%s", out)
			}
			// IT IS NOT A BUG AND CARRIES NO KIND, which is what fable's D7
			// ruled and what a reader keying on `bug` must not see here.
			if strings.Contains(out, "FOUND bug") {
				t.Errorf("the timeout was reported as a bug:\n%s", out)
			}
			// THE HANG REPORT IS WRITTEN AND NAMES THE PROBE AND THE TIMEOUT.
			for _, want := range []string{"did not read quiet", "100ms", "probe 1"} {
				if !strings.Contains(out+errb, want) {
					t.Errorf("the hang report does not say %q:\nstdout:\n%s\nstderr:\n%s",
						want, out, errb)
				}
			}
			// NO PENDING WAS OPEN: nothing here is eventual, so the finding
			// cannot have come from the overlay.
			//
			// READ OFF THE RUN'S OWN QUIESCE LINE, not from the ABSENCE of the
			// word. This searched stdout for "pending" and passed because the
			// string did not appear -- then R6.3 gave every run a line that
			// says how many are open, which contains the word and broke the
			// arm while its subject was unchanged. An absence that any new
			// output can end was never the assertion; the number is.
			if !strings.Contains(out, "0 pending(s) open at the end") {
				t.Errorf("the run does not report zero pendings open, and nothing here is "+
					"eventual so the finding cannot have come from the overlay:\n%s", out)
			}
			if strings.Contains(out, "FOUND pending") || strings.Contains(out, "kind stale") {
				t.Errorf("a pending or a stale appears in a run with no eventual type:\n%s", out)
			}
			// INSIDE THE DECLARED TIMEOUT PLUS GRACE. One direction only: the
			// floor would be timing the box.
			if elapsed > 30*time.Second {
				t.Errorf("the run took %s against a declared 100ms: the budget is not bounding it",
					elapsed)
			}
		})
	}
}

// THE SHRINK SAYS WHAT IT DID, ON THE LIVE PATH -- fable's ruling 1 at coop
// r6/153, and rook's gate blocker at rumor/1195: D11 reads "with every
// candidate quiesced" and the output had ZERO occurrences of `candidate`, so the
// condition could not be gated from the product at all. The two quiesce lines
// that existed are the RUN's own pending, not a candidate's.
//
// IT DRIVES THE BINARY AND NOT shrink.Result.Render, which is the whole point:
// the words have to reach a reader through `rumor run`, and a unit arm over the
// renderer would pass while nothing carried its output to the product -- the
// defect R6.1 shipped and r6/78 ruling 3 forbids.
//
// SEED 2 AT TEN STEPS IS THE CHEAP CELL and it exercises the shared clause: its
// bug is at step 4 of a 4-step trace, so nothing shortens and the NOT REDUCED
// branch prints, with the candidate count, the verified clean world and the
// settling. Measured on this box at 3d5d5d7 plus this commit: 1.31s, against the 57.82s
// seed 12 costs in the promoted-seed arm.
func TestTheShrinkSaysWhatItDidAndOnWhatKindOfWorld(t *testing.T) {
	out, errb, code := run(t, "run", "../../examples/toy", "--seed", "2", "--steps", "10")
	if code != findingStatusWanted {
		t.Fatalf("exit %d, want %d\nstdout:\n%s\nstderr:\n%s", code, findingStatusWanted, out, errb)
	}
	// THE WORD D11 ASKS FOR. rook could not gate the condition because this
	// token did not appear anywhere in a run's output.
	if !strings.Contains(out, "candidate(s)") {
		t.Fatalf("the output never says how many candidates ran, so D11's \"every candidate "+
			"quiesced\" cannot be read from the product:\n%s", out)
	}
	for _, want := range []string{
		"shrink: NOT REDUCED",
		"4 step(s) is the shortest trace that reproduces",
		"verified clean world",
		"settled with no pending left open",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the shrink's report does not say %q\n%s", want, out)
		}
	}
	// AND THE FIGURES AGREE WITH THE TRACE PRINTED BELOW THEM, which the first
	// cut of this line got wrong in the product: it read the bisection's
	// shortest PROBED PREFIX, so a reduction that then dropped a kind reported
	// "REDUCED 0 step(s) to 14" beside "trace: seed 1, 12 step(s)".
	if !strings.Contains(out, "trace: seed 2, 4 step(s)") {
		t.Errorf("the trace's own length is not 4, so the figures above it describe "+
			"a different trace\n%s", out)
	}
}

// AND A DECLARED PROVIDER THIS BUILD DOES NOT REGISTER IS A CARRIED NAME --
// fable's ruling at coop rumor/1202 on rook's /1197. examples/twin declares
// `doc-store-truncate` and says in its own comment that the name is carried AS
// DECLARED until R13 defines the interface, so the contract permits a name
// nothing can open yet. The wiring's first cut sent it through the
// could-not-run path and printed "run: could not shrink: no provider named ..."
// on STDERR for a shipped R4 example that was behaving exactly as documented.
//
// THE ASSERTION IS TWO-SIDED: the honest line is present AND the misleading one
// is absent, because a fix that added the first without removing the second
// would read as correct in a grep for the new words.
func TestADeclaredProviderThisBuildDoesNotRegisterIsNotAFailure(t *testing.T) {
	out, errb, code := run(t, "run", "../../examples/twin", "--seed", "1", "--steps", "10")
	if code != findingStatusWanted {
		t.Fatalf("exit %d, want %d\nstdout:\n%s\nstderr:\n%s", code, findingStatusWanted, out, errb)
	}
	if !strings.Contains(out, "shrink: NOT SHRUNK") ||
		!strings.Contains(out, `"doc-store-truncate"`) ||
		!strings.Contains(out, "carried as declared") {
		t.Errorf("twin's carried provider name is not reported as carried:\n%s", out)
	}
	if strings.Contains(errb, "could not shrink") || strings.Contains(out, "could not shrink") {
		t.Errorf("a carried provider name is still reported as a failure to shrink:\n"+
			"stdout:\n%s\nstderr:\n%s", out, errb)
	}
	// THE REGISTERED SET IS NAMED, so a reader is told what this build could
	// have opened rather than only what it could not.
	if !strings.Contains(out, "toy-process-restart") {
		t.Errorf("the line does not name the providers this build does register:\n%s", out)
	}
}

// findingStatusWanted is the exit status a found bug reports (cmd/rumor's
// findingStatus, which is unexported). Named here rather than written as 3 at
// four call sites: a literal would not move if the statute's exit codes ever
// did, and the four exit statuses are one named constant in the product for
// exactly that reason (fable, coop r5/176).
const findingStatusWanted = 3
