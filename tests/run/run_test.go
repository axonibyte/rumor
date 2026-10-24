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
	defer func() { _ = os.RemoveAll(dir) }()

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
			os.Exit(1)
		}
	}
	if err := os.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH")); err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: %v\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
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
	var seeds []string
	for _, line := range strings.Split(string(raw), "\n") {
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		if _, perr := strconv.ParseUint(line, 10, 64); perr != nil {
			t.Fatalf("%s: %q is not one non-negative integer per line, which is the format "+
				"docs/contract.md declares: %v", lock, line, perr)
		}
		seeds = append(seeds, line)
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

	for _, seed := range seeds {
		t.Run("seed"+seed, func(t *testing.T) {
			// ONE CONSTANT FOR THE LENGTH, because the exit-0 message below
			// quotes it: two literals saying 10, one inside a sentence about
			// the other, is a drift waiting to happen.
			const steps = 10
			first, errb, code := run(t, "run", "../../examples/toy", "--seed", seed,
				"--steps", strconv.Itoa(steps))
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
			// AND IT IS THE HONEST FINDING, which is half of what seeds.lock
			// claims for EVERY seed it carries: "Both are the HONEST finding --
			// `rests_on_planted false`". This cited "seed 7" until 2026-10-24,
			// and seed 7 retired from the lock that day -- so the citation
			// landed on the one paragraph in that file which says the opposite,
			// the retirement note calling it five actions and superseded.
			// ratchet found it at coop r5/235 by grepping the file for the seed
			// I had removed, which is the search I owed and did not run: a
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
			second, _, again := run(t, "run", "../../examples/toy", "--seed", seed, "--steps", "10")
			if again != code {
				t.Errorf("the same seed exited %d and then %d", code, again)
			}
			if first != second {
				t.Errorf("seed %s does not reproduce: the two runs differ\nfirst:\n%s\n"+
					"second:\n%s", seed, first, second)
			}
		})
	}
}
