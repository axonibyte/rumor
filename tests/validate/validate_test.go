// Package validate_test holds `rumor validate` to its arms (R4.3, issue 64).
//
// THE FIXTURES LIVE HERE AND NOT UNDER examples/, and that is a ruling rather
// than a convenience (fable, coop r4/63): they are deliberately BROKEN
// projects, so `rumor check` and `rumor attaint` must never walk them. Only
// this suite and the drills read them.
package validate_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/validate"
)

// THE DECLARED SIZE, which is the claim: C2 ships six arms and C3 brings the
// count to eighteen. A fixture directory that vanished would otherwise leave
// every remaining arm passing and the suite quiet about it.
const wantFixtures = 18

// armOf is the fixture naming convention: <arm>-<slug>. The arm is the part
// before the first dash, so the directory name IS the expectation and a
// renamed directory cannot quietly assert something else.
func armOf(dir string) string { return strings.SplitN(dir, "-", 2)[0] }

// TestMain BUILDS THE BINARY ONCE AND PUTS IT ON PATH, which is how the live
// arms are reachable without anything in the twin being test-shaped (fable,
// coop r4/74). The twin's [adapter].command is `rumor fake-adapter --surface
// surface.json` -- what a real project writes -- so the only question is
// whether a `rumor` exists to spawn. The same `go build -o` ci.yml runs answers
// it.
//
// A FAILED BUILD FAILS THE SUITE rather than skipping it: the four live arms
// would otherwise report could-not-run and every test here would pass over
// them, which is the vacuity this project refuses everywhere else.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "rumor-validate-bin")
	if err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	bin := filepath.Join(dir, "rumor")
	build := exec.Command("go", "build", "-o", bin, "./cmd/rumor")
	build.Dir = filepath.Join("..", "..")
	build.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	if out, berr := build.CombinedOutput(); berr != nil {
		fmt.Fprintf(os.Stderr, "TestMain: building rumor for the live arms: %v\n%s\n", berr, out)
		os.Exit(1)
	}
	if err := os.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH")); err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: %v\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func fixturesDir(t *testing.T) string {
	t.Helper()
	return filepath.Join("fixtures")
}

func fixtures(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(fixturesDir(t))
	if err != nil {
		t.Fatalf("reading the fixture directory: %v", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

// TestTheFixturesAreAllPresent refuses a suite that has drifted from the
// directory, the same arm tests/attaint has over its drills: deleting a fixture
// would otherwise shrink the coverage silently.
func TestTheFixturesAreAllPresent(t *testing.T) {
	got := fixtures(t)
	if len(got) != wantFixtures {
		t.Fatalf("%d fixture(s) under tests/validate/fixtures, want %d: %v", len(got), wantFixtures, got)
	}
	seen := map[string]bool{}
	for _, f := range got {
		arm := armOf(f)
		if arm == f {
			t.Errorf("%q does not name its arm: the convention is <arm>-<slug>", f)
		}
		if seen[arm] {
			t.Errorf("two fixtures claim arm %s: one fixture per arm is the invariant", arm)
		}
		seen[arm] = true
	}
}

// TestThePermittedTwinPasses is the known positive this whole suite rests on.
// Without it, an arm that fired on everything would look identical to an arm
// that fired on exactly the wrong thing.
func TestThePermittedTwinPasses(t *testing.T) {
	rep := validate.Run(filepath.Join("..", "..", "examples", "twin"))
	if !rep.OK() {
		for _, r := range rep.Refusals {
			t.Errorf("  refused: %s", r)
		}
		for _, c := range rep.CouldNotRun {
			t.Errorf("  could not run: %s", c)
		}
		t.Fatalf("the permitted twin does not validate, so every refusal below is unreadable")
	}
	if rep.Exit() != 0 {
		t.Errorf("the twin exits %d, want 0", rep.Exit())
	}
	// AND IT EXAMINED SOMETHING. A twin that validated because nothing was read
	// would pass this test and mean nothing.
	if rep.Actions == 0 || rep.Cases == 0 || rep.Rules == 0 {
		t.Errorf("the twin validated over %d action(s), %d case(s), %d rule(s): a pass over an empty set is vacuous",
			rep.Actions, rep.Cases, rep.Rules)
	}
	t.Logf("PROOF twin passes over %d action(s), %d observation(s), %d case(s), %d rule(s), %d mutant(s)",
		rep.Actions, rep.Observations, rep.Cases, rep.Rules, rep.Mutants)
}

// TestEveryFixtureTripsExactlyItsOwnArm is fable's invariant over the eighteen
// (coop breakout-r4/6): a fixture tripping two arms shows neither.
func TestEveryFixtureTripsExactlyItsOwnArm(t *testing.T) {
	for _, f := range fixtures(t) {
		t.Run(f, func(t *testing.T) {
			rep := validate.Run(filepath.Join(fixturesDir(t), f))
			if len(rep.CouldNotRun) > 0 {
				for _, c := range rep.CouldNotRun {
					t.Errorf("  could not run: %s", c)
				}
				t.Fatalf("%s could not be read, which is not the same as being refused", f)
			}
			var arms []string
			for _, r := range rep.Refusals {
				arms = append(arms, r.Arm)
			}
			sort.Strings(arms)
			uniq := arms[:0]
			for i, a := range arms {
				if i == 0 || arms[i-1] != a {
					uniq = append(uniq, a)
				}
			}
			want := armOf(f)
			if strings.Join(uniq, ",") != want {
				t.Fatalf("%s tripped %v, want exactly [%s]: a fixture that trips a second arm is not the fixture that was designed",
					f, uniq, want)
			}
			if rep.Exit() != 1 {
				t.Errorf("%s exits %d, want 1 (refused)", f, rep.Exit())
			}
			t.Logf("PROOF %s -> %s", f, rep.Refusals[0])
		})
	}
}

// TestADecoderFailureIsCouldNotRunAndNotRefused pins docs/contract.md:57's
// boundary where it still lives after fable's ruling at coop r4/79.
//
// THE BOUNDARY MOVED AND THIS TEST MOVED WITH IT. It pointed at a missing
// project, which is now a REFUSAL -- a project that has not said what it is is
// the project being wrong. could-not-run belongs to the ADAPTER and the
// DECODER, so this aims at the decoder: a bindings.toml that is not TOML.
func TestADecoderFailureIsCouldNotRunAndNotRefused(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bindings.toml"), []byte("this is not = = toml\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep := validate.Run(dir)
	if len(rep.Refusals) != 0 {
		t.Errorf("a malformed bindings.toml produced %d refusal(s): %v -- a file the reader cannot read "+
			"is not a project that is wrong", len(rep.Refusals), rep.Refusals)
	}
	if len(rep.CouldNotRun) == 0 {
		t.Fatal("no could-not-run reason, so the run reported nothing at all")
	}
	if rep.Exit() != 2 {
		t.Errorf("exit %d, want 2", rep.Exit())
	}
	t.Logf("PROOF could-not-run: %s", rep.CouldNotRun[0])
}

// TestAVacuousProjectIsRefusedAndAccountsForEveryArm is fable's D8, ruled at
// coop r4/79: zero rules, no bindings and zero actions are each a REFUSAL at
// exit 1 and never a pass -- "there is nothing to validate because the project
// has not said what it is" -- and `rumor attaint` draws the same line.
//
// IT PINS THE ACCOUNTING THROUGH THE VACUITY PATH too: the arms did not run, so
// all eighteen are SKIPPED with the refusal named, rather than left unaccounted
// while the verdict claims eighteen.
func TestAVacuousProjectIsRefusedAndAccountsForEveryArm(t *testing.T) {
	twin := filepath.Join("..", "..", "examples", "twin")
	cases := []struct {
		what, clause string
		set          func(t *testing.T, dir string)
	}{
		{"no bindings at all", "no-bindings", func(t *testing.T, dir string) {
			if err := os.Remove(filepath.Join(dir, "bindings.toml")); err != nil {
				t.Fatal(err)
			}
		}},
		{"zero actions", "bindings-refused", func(t *testing.T, dir string) {
			if err := os.WriteFile(filepath.Join(dir, "bindings.toml"), []byte("# nothing\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"zero rules", "no-rule-declared", func(t *testing.T, dir string) {
			if err := os.WriteFile(filepath.Join(dir, "rules.json"), []byte("[]"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.what, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "twin")
			if out, err := exec.Command("cp", "-r", twin, dir).CombinedOutput(); err != nil {
				t.Fatalf("copying the twin: %v %s", err, out)
			}
			c.set(t, dir)
			rep := validate.Run(dir)
			if rep.Exit() != 1 {
				t.Errorf("%s exits %d, want 1 (refused)", c.what, rep.Exit())
			}
			if len(rep.CouldNotRun) != 0 {
				t.Errorf("%s also reported could-not-run %v: that is the decoder's verdict, not the project's",
					c.what, rep.CouldNotRun)
			}
			var clauses []string
			for _, r := range rep.Refusals {
				clauses = append(clauses, r.Arm)
			}
			if len(clauses) != 1 || clauses[0] != c.clause {
				t.Fatalf("%s refused %v, want exactly [%s]", c.what, clauses, c.clause)
			}
			if n := len(rep.Evaluated) + len(rep.Skipped); n != validate.TotalArms {
				t.Errorf("%s accounts for %d arm(s), want %d: a vacuity must not leave arms unaccounted",
					c.what, n, validate.TotalArms)
			}
			t.Logf("PROOF %s -> %s, %d arm(s) skipped", c.what, rep.Refusals[0], len(rep.Skipped))
		})
	}
}

// TestTheCasesVacuityGuardFires is the live half of C2's vacuity guard: a
// project whose bindings parse and whose cases/ is absent would otherwise let
// arms 1, 12a and 12b pass in silence.
func TestTheCasesVacuityGuardFires(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"bindings.toml", "rules.json", "statute.rego"} {
		b, err := os.ReadFile(filepath.Join("..", "..", "examples", "twin", f))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, f), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rep := validate.Run(dir)
	if rep.Exit() != 2 {
		t.Fatalf("a project with no cases/ exits %d, want 2", rep.Exit())
	}
	if len(rep.Refusals) != 0 {
		t.Errorf("it also produced %d refusal(s); an unexamined arm is not a refused one", len(rep.Refusals))
	}
	t.Logf("PROOF %v", rep.CouldNotRun)
}

// TestBindingsStillRefusesAZeroActionFile PINS A COUPLING rather than trusting
// it. internal/validate has no actions-vacuity check of its own because
// internal/bindings refuses a zero-action file first, which would make such a
// check unreachable -- and a check that cannot fire reads exactly like one that
// does. If that refusal moves, this fails here instead of opening a silent hole.
//
// IT READS THE REFUSAL'S TEXT AND NOT THE EXIT STATUS NOW: fable's r4/79 made a
// project's emptiness a refusal rather than could-not-run, so the coupling
// shows up in what the refusal says.
func TestBindingsStillRefusesAZeroActionFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bindings.toml"), []byte("# nothing declared\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep := validate.Run(dir)
	if rep.Exit() != 1 {
		t.Fatalf("a zero-action bindings file exits %d, want 1", rep.Exit())
	}
	var joined string
	for _, r := range rep.Refusals {
		joined += r.Detail + " "
	}
	if !strings.Contains(joined, "no actions") {
		t.Fatalf("the refusal reads %q; validate relies on bindings refusing \"no actions\", so validate "+
			"now has an unguarded vacuity", joined)
	}
	t.Logf("PROOF the coupling holds: %s", strings.TrimSpace(joined))
}

// changedBy declares, per fixture, the ONLY paths that may differ from the
// permitted twin. Everything else the fixture shares with the twin must be
// byte-identical.
//
// THIS TABLE IS THE "ONE CHANGE" INVARIANT MADE MECHANICAL, and it exists
// because the invariant broke silently once: when the twin earned its
// seeds.lock, six fixtures predating that file began tripping arm 10a as well
// as their own, and no test noticed. A fixture is generated from the twin by
// tools/make-validate-fixtures.py; this is the oracle over the result.
var changedBy = map[string][]string{
	"1-type-not-emitted":   {"cases/fresh-publisher.json"},
	"2-type-not-described": {"bindings.toml"},
	"3-no-precondition":    {"bindings.toml"},
	"7a-eventual-no-bound": {"bindings.toml"},
	"8-clock-no-control":   {"statute.rego"},
	"10a-no-seeds-file":    {"seeds.lock", "WHY-no-seeds-file.md"},
	"11-no-mutants":        {"mutants", "WHY-no-mutants.md"},
	"12a-no-position":      {"cases/miscount.json"},
	"12b-no-source":        {"cases/untold-claim.json"},

	// FIXTURES WHOSE CONDITION HAS A PRECONDITION declare BOTH edits, and fable's
	// rule (coop r4/74) is that this is still ONE change: one CONDITION, not one
	// byte. The setup edit prepares the ground -- the browser stage on, a claim
	// type made eventual -- and each fixture's README names which edit is which.
	// A setup edit that tripped an arm by itself would make the fixture two
	// fixtures, which is why 4's added action carries `requires`.
	"4-no-ui-flow":          {"bindings.toml", "WHY-no-ui-flow.md"},
	"5-browser-reaches-api": {"bindings.toml", "WHY-browser-reaches-api.md"},
	"6-component-uncovered": {"platform.toml"},
	"7b-quiesce-no-probe":   {"bindings.toml"},
	"10b-unreadable-seed":   {"seeds.lock"},

	// THE LIVE ARMS change the project's ASK and never the adapter's surface:
	// changing what the adapter can do would test the fake adapter, and these
	// arms are about what the project asks for.
	"9a-fault-unavailable":   {"bindings.toml"},
	"9b-volume-unavailable":  {"bindings.toml"},
	"9c-upgrade-unavailable": {"bindings.toml"},
	"13-contract-mismatch":   {"platform.toml"},
}

// TestEveryFixtureIsTheTwinPlusItsOwnChange is the drift oracle. Without it a
// change to the twin silently invalidates every fixture that predates it, and
// the arm-isolation test above starts failing for a reason that has nothing to
// do with the arms.
func TestEveryFixtureIsTheTwinPlusItsOwnChange(t *testing.T) {
	twin := filepath.Join("..", "..", "examples", "twin")
	for _, f := range fixtures(t) {
		t.Run(f, func(t *testing.T) {
			allowed, ok := changedBy[f]
			if !ok {
				t.Fatalf("%s is in fixtures/ and not in changedBy: declare what it changes, "+
					"or the one-change invariant says nothing about it", f)
			}
			allow := map[string]bool{}
			for _, a := range allowed {
				allow[a] = true
			}
			var drifted []string
			err := filepath.Walk(twin, func(path string, fi os.FileInfo, err error) error {
				if err != nil || fi.IsDir() {
					return err
				}
				rel, rerr := filepath.Rel(twin, path)
				if rerr != nil {
					return rerr
				}
				// an allowed path, or anything under an allowed directory
				for a := range allow {
					if rel == a || strings.HasPrefix(rel, a+string(filepath.Separator)) {
						return nil
					}
				}
				want, rerr := os.ReadFile(path)
				if rerr != nil {
					return rerr
				}
				got, gerr := os.ReadFile(filepath.Join("fixtures", f, rel))
				if gerr != nil {
					drifted = append(drifted, rel+" (missing from the fixture)")
					return nil
				}
				if string(got) != string(want) {
					drifted = append(drifted, rel)
				}
				return nil
			})
			if err != nil {
				t.Fatalf("walking the twin: %v", err)
			}
			if len(drifted) > 0 {
				sort.Strings(drifted)
				t.Fatalf("%s differs from the twin outside what it declares to change: %v -- "+
					"regenerate with tools/make-validate-fixtures.py", f, drifted)
			}
		})
	}
}

// allArms is the eighteen, named. fable ruled the count at coop breakout-r4/6
// and it moves only with a count posted in r4 first.
var allArms = []string{
	"1", "2", "3", "4", "5", "6", "7a", "7b", "8",
	"9a", "9b", "9c", "10a", "10b", "11", "12a", "12b", "13",
}

// TestEveryArmAccountsForItself is the "18 of 18" invariant, and it is an
// accounting one rather than a single project's number.
//
// WHY NOT SIMPLY THE TWIN: the twin reports SIXTEEN of eighteen, because arms 4
// and 5 are vacuous unless `stages.browser.enabled` is true and the twin cannot
// enable them. Making it able to would mean declaring a browser action and a
// role, and tests/contract's agreement test refuses an examples/* package that
// declares either with no case carrying it. So the eighteenth arm is reached
// across the SUITE -- each fixture that enables the browser stage evaluates
// those two -- and what this test pins is that every arm is evaluated SOMEWHERE
// and that no run ever both evaluates and skips one.
func TestEveryArmAccountsForItself(t *testing.T) {
	seen := map[string]bool{}

	check := func(name string, rep validate.Report) {
		for _, a := range rep.Evaluated {
			if _, also := rep.Skipped[a]; also {
				t.Errorf("%s reports arm %s as BOTH evaluated and skipped", name, a)
			}
			seen[a] = true
		}
		if n := len(rep.Evaluated) + len(rep.Skipped); n != validate.TotalArms {
			t.Errorf("%s accounts for %d arm(s), want %d: every arm either evaluates or says why not",
				name, n, validate.TotalArms)
		}
	}

	check("the twin", validate.Run(filepath.Join("..", "..", "examples", "twin")))
	for _, f := range fixtures(t) {
		check(f, validate.Run(filepath.Join("fixtures", f)))
	}

	var missing []string
	for _, a := range allArms {
		if !seen[a] {
			missing = append(missing, a)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("arm(s) %v are evaluated by nothing in this suite: an arm no run reaches is an arm "+
			"whose fixture proves only that the fixture was read", missing)
	}
	if len(allArms) != validate.TotalArms {
		t.Fatalf("this test names %d arms and validate.TotalArms is %d", len(allArms), validate.TotalArms)
	}
	t.Logf("PROOF %d of %d arms evaluated across the twin and %d fixtures",
		len(seen), validate.TotalArms, len(fixtures(t)))
}

// TestTheDocumentedOutputIsTheBinarysOutput holds docs/cli.md's two console
// blocks to what `rumor validate` actually prints.
//
// IT EXISTS BECAUSE THEY WENT STALE ONCE, inside an hour: the blocks were
// byte-checked when written, then the twin gained a browser stage to satisfy
// fable's D9 drill and both became wrong -- 16 of 18 where the binary now says
// 18, and 12 where it says 14. A quoted output is a claim about the program,
// and this project checks quoted documents elsewhere (tests/vocabulary) for the
// same reason. The fixtures got a drift oracle in C3; this is the docs' one.
func TestTheDocumentedOutputIsTheBinarysOutput(t *testing.T) {
	bin, err := exec.LookPath("rumor")
	if err != nil {
		t.Fatalf("TestMain should have put rumor on PATH: %v", err)
	}
	root := filepath.Join("..", "..")
	raw, err := os.ReadFile(filepath.Join(root, "docs", "cli.md"))
	if err != nil {
		t.Fatal(err)
	}
	blocks := regexp.MustCompile("(?s)```console\n\\$ rumor validate examples/twin[^\n]*\n(.*?)```").
		FindAllStringSubmatch(string(raw), -1)
	if len(blocks) != 2 {
		t.Fatalf("docs/cli.md holds %d `rumor validate examples/twin` console block(s), want 2: "+
			"this oracle names what it checks, so a third would be unchecked", len(blocks))
	}

	run := func(path string) string {
		cmd := exec.Command(bin, "validate", "examples/twin")
		cmd.Dir = root
		cmd.Env = []string{"PATH=" + path, "GOTOOLCHAIN=local"}
		out, _ := cmd.CombinedOutput() // a non-zero status is a verdict, not a failure
		return strings.TrimRight(string(out), "\n")
	}

	// THE TWO ENVIRONMENTS ARE THE POINT of the two blocks: whether a `rumor`
	// exists to spawn decides four arms, and the documented figure depends on
	// it. The second uses an empty temp dir rather than an empty PATH so the
	// difference is "no rumor here" and not "no PATH at all".
	cases := []struct {
		what, path, want string
	}{
		{"with the adapter on PATH", filepath.Dir(bin), strings.TrimRight(blocks[0][1], "\n")},
		{"no rumor on PATH", t.TempDir(), strings.TrimRight(blocks[1][1], "\n")},
	}
	for _, c := range cases {
		t.Run(c.what, func(t *testing.T) {
			got := run(c.path)
			if got != c.want {
				t.Errorf("docs/cli.md claims:\n%s\n\nthe binary prints:\n%s", c.want, got)
			}
		})
	}
}

// wantTallies is how many command tallies docs/cli.md quotes outside the
// `rumor validate` blocks above: `check`'s and `attaint`'s. It is DECLARED so
// that an empty scan fails -- a regexp that matched nothing would assert the
// happy path twice and say nothing about either tally.
const wantTallies = 2

// TestTheDocumentedTalliesAreTheBinarys holds those two quoted tallies to what
// the commands actually print over examples/.
//
// IT EXISTS BECAUSE THE TWIN MOVED BOTH AND NO SUITE NOTICED. A fourth example
// took the corpus from 3 packages and 17 cases to 4 and 22, and docs/cli.md
// went on quoting the old figures: `check`'s tally sits in a plain fence that
// nothing executes, and `attaint`'s sits inside a `run` block that only
// release-gate.py executes -- on a pull request to main, which is after every
// push a branch makes. The branch tip was green in CI while the gate refused it
// with doc-run-block-failed. `go test ./...` runs on every push, so a claim
// about what the program prints is checked here rather than at the release.
//
// IT CHECKS PRESENCE, NOT THE WHOLE STREAM, which is what the validate oracle
// above does instead: these two samples quote ONE line of a many-line run on
// purpose, so the assertion the document makes is that this line appears, and
// holding it to the entire output would be asserting something it never said.
func TestTheDocumentedTalliesAreTheBinarys(t *testing.T) {
	bin, err := exec.LookPath("rumor")
	if err != nil {
		t.Fatalf("TestMain should have put rumor on PATH: %v", err)
	}
	root := filepath.Join("..", "..")
	raw, err := os.ReadFile(filepath.Join(root, "docs", "cli.md"))
	if err != nil {
		t.Fatal(err)
	}
	quoted := regexp.MustCompile(`(?m)^(check|attaint): .*$`).FindAllString(string(raw), -1)
	if len(quoted) != wantTallies {
		t.Fatalf("docs/cli.md quotes %d `check`/`attaint` tally line(s), want %d: this oracle "+
			"names what it checks, so an unquoted or a third tally is unchecked", len(quoted), wantTallies)
	}
	for _, line := range quoted {
		verb := strings.SplitN(line, ":", 2)[0]
		t.Run(verb, func(t *testing.T) {
			cmd := exec.Command(bin, verb, "--examples", "examples")
			cmd.Dir = root
			cmd.Env = []string{"PATH=" + filepath.Dir(bin), "GOTOOLCHAIN=local"}
			out, cmdErr := cmd.CombinedOutput()
			if cmdErr != nil {
				t.Fatalf("rumor %s --examples examples: %v\n%s", verb, cmdErr, out)
			}
			for _, got := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
				if got == line {
					return
				}
			}
			t.Errorf("docs/cli.md quotes:\n%s\n\nrumor %s printed:\n%s", line, verb, out)
		})
	}
}
