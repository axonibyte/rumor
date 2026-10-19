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
	build := exec.Command("go", "build", "-o", rumorBin, "./cmd/rumor")
	build.Dir = filepath.Join("..", "..")
	build.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	if out, berr := build.CombinedOutput(); berr != nil {
		fmt.Fprintf(os.Stderr, "TestMain: building rumor: %v\n%s\n", berr, out)
		os.Exit(1)
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
