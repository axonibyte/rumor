// Package validate_test holds `rumor validate` to its arms (R4.3, issue 64).
//
// THE FIXTURES LIVE HERE AND NOT UNDER examples/, and that is a ruling rather
// than a convenience (fable, coop r4/63): they are deliberately BROKEN
// projects, so `rumor check` and `rumor attaint` must never walk them. Only
// this suite and the drills read them.
package validate_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/validate"
)

// THE DECLARED SIZE, which is the claim: C2 ships six arms and C3 brings the
// count to eighteen. A fixture directory that vanished would otherwise leave
// every remaining arm passing and the suite quiet about it.
const wantFixtures = 13

// armOf is the fixture naming convention: <arm>-<slug>. The arm is the part
// before the first dash, so the directory name IS the expectation and a
// renamed directory cannot quietly assert something else.
func armOf(dir string) string { return strings.SplitN(dir, "-", 2)[0] }

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

// TestAProjectThatCannotBeReadIsCouldNotRunAndNotRefused pins
// docs/contract.md:57's boundary: exit 2 is "the project is not wrong, its
// declarations could not be read", and it must never arrive as a refusal naming
// an arm -- fable's D7 asserts the same boundary for the adapter.
func TestAProjectThatCannotBeReadIsCouldNotRunAndNotRefused(t *testing.T) {
	rep := validate.Run(filepath.Join(t.TempDir(), "nothing-here"))
	if len(rep.Refusals) != 0 {
		t.Errorf("an unreadable project produced %d refusal(s): %v", len(rep.Refusals), rep.Refusals)
	}
	if len(rep.CouldNotRun) == 0 {
		t.Fatal("an unreadable project produced no could-not-run reason, so the run reported nothing at all")
	}
	if rep.Exit() != 2 {
		t.Errorf("exit %d, want 2", rep.Exit())
	}
	t.Logf("PROOF could-not-run: %s", rep.CouldNotRun[0])
}

// TestAnEmptyProjectIsVacuousRatherThanValid is the vacuity guard C2 owes: a
// bindings file that parses and declares no action would let every arm over
// actions pass in silence.
func TestAnEmptyProjectIsVacuousRatherThanValid(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bindings.toml"), []byte("# nothing declared\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rules.json"), []byte(`["finding"]`), 0o644); err != nil {
		t.Fatal(err)
	}
	rep := validate.Run(dir)
	if rep.Exit() != 2 {
		t.Errorf("a project declaring no action exits %d, want 2: a pass over an empty set is vacuous", rep.Exit())
	}
	if len(rep.CouldNotRun) == 0 {
		t.Error("no could-not-run reason names the empty set")
	}
	t.Logf("PROOF %v", rep.CouldNotRun)
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
// it. internal/validate drops its own actions-vacuity check because
// internal/bindings refuses a zero-action file first, which makes the check
// unreachable -- and a check that cannot fire reads exactly like one that does.
// If that refusal ever moves, this fails here instead of opening a silent hole
// in validate.
func TestBindingsStillRefusesAZeroActionFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bindings.toml")
	if err := os.WriteFile(path, []byte("# nothing declared\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep := validate.Run(dir)
	if rep.Exit() != 2 {
		t.Fatalf("a zero-action bindings file exits %d, want 2", rep.Exit())
	}
	joined := strings.Join(rep.CouldNotRun, " ")
	if !strings.Contains(joined, "no actions") {
		t.Fatalf("the could-not-run reason is %q; validate relies on bindings refusing "+
			"\"no actions\", so validate now has an unguarded vacuity", joined)
	}
	t.Logf("PROOF the coupling holds: %s", joined)
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
	"4-no-ui-flow":          {"platform.toml", "bindings.toml", "WHY-no-ui-flow.md"},
	"5-browser-reaches-api": {"platform.toml", "bindings.toml", "WHY-browser-reaches-api.md"},
	"6-component-uncovered": {"platform.toml"},
	"7b-quiesce-no-probe":   {"bindings.toml"},
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
