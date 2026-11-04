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
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/decl"
	"forge.axonibyte.com/axonibyte/rumor/internal/validate"
)

// THE DECLARED SIZE, which is the claim: C2 ships six arms, C3 brings the count
// to eighteen, fable's r4/113 ruling adds the nineteenth, and R6.2's three
// probe-integrity arms (7c, 7d, 7e) bring it to twenty-two. ONE MORE AT R6's
// GATE BUNDLE: arm 19's cell 2 reverse refuses on TWO loops and only one had a
// fixture, so `19/kind-without-a-weight` is the thirty-first. A fixture
// directory that vanished would otherwise leave every remaining arm passing and
// the suite quiet about it.
//
// TWO MORE AT R8.2, both cells of arm 19: `[[action]].contended` holds two
// REFERENCES and the parser resolved neither, so a `limit` or `held` naming
// nothing passed validate -- and the symptom is quiet, because a limit that
// resolves to nothing leaves the room unknown and every fan-out is then refused
// as vacuous, which reads as "no overfill found" rather than as a broken
// declaration.
// AND TWO MORE AT R8.4, both cells of arm 19 again and both about a row a
// PROFILE appends (fable, coop rumor/1662). The first is a hole this suite had
// for one commit: the venire cell read the platform's own rows alone, so an
// appended row naming a role nothing declares was refused by nobody. The second
// is the pairing fable ruled into this arm rather than giving it an id -- an
// actor holding a context no stage the profile selects can drive, which is an
// actor the profile draws and cannot move.
// AND ONE AT R8.4 FOR ARM 21, its action path -- the case docs/contract.md:222
// states in words. ratchet measured at coop r8/149 that the arm's accounting
// was armed and its JUDGEMENT was not: nothing in the tree read a refusal from
// it, so the comparison could be made unconditionally false and stay green.
// AND TWO MORE AT R8/111, the same shape one declaration over: the
// observations level fable ruled at coop rumor/1678 has a profile claiming
// observation NAMES, so its two pairings are a name nothing declares and a name
// whose source no stage the profile selects can drive. The first is the one
// that bites quietly -- a profile's claim is also what takes a name out of the
// project's set, so a misspelled claim leaves the observation taken by every
// profile and reads as a working declaration from the profile that wanted it.
// AND ONE MORE AT R9.1: arm 22, `creates` on a browser-context action, whose
// handle a `flow` answer carries no field to mint from (fable's ruling at coop
// r9/17 over giving `flow` an `ids` nothing reads). The count was posted in
// thread:breakout-r4 before this constant moved, as arm 21's was.
const wantFixtures = 41

// ARM 19 IS ONE ARM WITH SEVERAL CELLS, so it is the one arm with more than one
// fixture -- and the count is declared here for the same reason every count in
// this project is. fable ruled nine cells at r4/113 and the list settled at
// EIGHT by measurement rather than preference (r4/125): the `of` cell came off,
// already held by arm 1 and tests/contract between them; the action-context
// cell came off, its referent set unsound enough to refuse both of our own
// examples; and cell 2's REVERSE went in -- an action or nemesis kind with no
// weight -- after quill measured that gap from the walk's side.
//
// 11 -> 13 AT R8.2, TWO CELLS, AND THE SYMPTOM IS WHY THEY ARE REFUSED HERE.
// `[[action]].contended` holds two REFERENCES as of R8.2 -- an action's
// argument where the limit is read, and the declared query that reads what the
// system holds -- and the parser requires both keys present and nothing more,
// so nothing resolved either (rook, coop rumor/1528). A `limit` resolving to
// nothing leaves the room unknown, so every fan-out is refused as VACUOUS, and
// a vacuous fan-out reads as "no overfill found" rather than as a broken
// declaration: the one output a reader would act on is the one it cannot
// produce. Cells and not arms, for the reason the instants cell gives below.
//
// 10 -> 11 AT R7.4, AND THIS TIME A CELL WAS ADDED: `[deadline].instants`
// holds observation names as of R7.4, and this arm's own sentence -- "a name
// the project uses that the project does not declare" -- covered them while
// nothing checked them. ratchet drove that gap before the cell existed (coop
// r7/60: an undeclared name passed at rc 0), which is why it is a cell here
// rather than a twenty-third arm whose arrival would have left this sentence
// overreaching.
//
// 9 -> 10 AT R6.2, AND NO CELL WAS ADDED. Cell 2's reverse refuses on TWO
// loops -- a declared ACTION with no weight, and an engine POOL KIND with no
// weight -- and only the first had a fixture, so the second was a refusal
// nothing tripped: rook's arm-19 finding inside the arm that taught the room
// the shape. `19/kind-without-a-weight` trips it. The ARM count does not move,
// because a cell is not an arm; this number counts the fixtures that pair with
// cells, and it was short by one for as long as that loop existed.
// 13 -> 15 AT R8.4, TWO CELLS, BOTH ABOUT A ROW A PROFILE APPENDS (fable, coop
// rumor/1662). `[profiles.<name>].venire` appends rows to the platform's venire
// for one profile, so a profile's row is a venire row and both pairings are
// this arm's: the role it names must be declared, which this arm already
// required of the platform's own rows and did not of a profile's for one
// commit; and the contexts that role holds must be drivable by a stage THAT
// PROFILE selects, which nothing paired at all. Cells and not arms, on the
// ruling itself: fable refused a fresh id for the second because a profile's
// appended row is a venire row and the venire pairing is already arm 19's.
// 15 -> 17 AT R8/111, TWO CELLS, BOTH ABOUT A NAME A PROFILE CLAIMS (fable,
// coop rumor/1678). `[profiles.<name>].observations` claims observation names
// for one profile, and a claimed name stops being the project's -- so the
// claims need the pairings the rows got: the name must be declared, and the
// source it is declared with must be drivable by a stage THAT PROFILE selects.
// Cells and not arms, on cell 8's ruling read one declaration over: the claim
// is a name and what it names is in the other document, which is this arm.
const wantCells19 = 17

// armOf is the fixture naming convention: <arm>-<slug>, or <arm>/<slug> for an
// arm with several cells. The arm is the part before the first separator, so
// the directory name IS the expectation and a renamed directory cannot quietly
// assert something else.
func armOf(dir string) string {
	if i := strings.IndexByte(dir, '/'); i >= 0 {
		return dir[:i]
	}
	return strings.SplitN(dir, "-", 2)[0]
}

// TestMain BUILDS THE BINARY ONCE AND PUTS IT ON PATH, which is how the live
// arms are reachable without anything in the twin being test-shaped (fable,
// coop r4/74). The twin's [adapter].command is `rumor fake-adapter --surface
// surface.json` -- what a real project writes -- so the only question is
// whether a `rumor` exists to spawn. The same `go build -o` ci.yml runs answers
// it.
//
// A FAILED BUILD FAILS THE SUITE rather than skipping it: the five live arms
// would otherwise report could-not-run and every test here would pass over
// them, which is the vacuity this project refuses everywhere else.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "rumor-validate-bin")
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

	bin := filepath.Join(dir, "rumor")
	build := exec.Command("go", "build", "-o", bin, "./cmd/rumor")
	build.Dir = filepath.Join("..", "..")
	build.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	if out, berr := build.CombinedOutput(); berr != nil {
		fmt.Fprintf(os.Stderr, "TestMain: building rumor for the live arms: %v\n%s\n", berr, out)
		die(1)
	}
	if err := os.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH")); err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: %v\n", err)
		die(1)
	}
	die(m.Run())
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
		if !e.IsDir() {
			continue
		}
		// A GROUP, NOT A FIXTURE: an arm with several cells keeps them one
		// level down, and a group holds no bindings.toml of its own. Reporting
		// the group as one fixture would make seven cells look like one and
		// would run validate against a directory that is not a project.
		if _, err := os.Stat(filepath.Join(fixturesDir(t), e.Name(), "bindings.toml")); err != nil {
			cells, cerr := os.ReadDir(filepath.Join(fixturesDir(t), e.Name()))
			if cerr != nil {
				t.Fatalf("reading the cells of %s: %v", e.Name(), cerr)
			}
			for _, c := range cells {
				if c.IsDir() {
					names = append(names, e.Name()+"/"+c.Name())
				}
			}
			continue
		}
		names = append(names, e.Name())
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
	seen := map[string]int{}
	for _, f := range got {
		arm := armOf(f)
		if arm == f {
			t.Errorf("%q does not name its arm: the convention is <arm>-<slug>", f)
		}
		seen[arm]++
	}
	for arm, n := range seen {
		// ONE FIXTURE PER ARM, AND ARM 19 IS THE RULED EXCEPTION: it is one arm
		// with several cells (fable, r4/113), each needing its own broken
		// reference, so its count is declared instead of being capped at one.
		if arm == "19" {
			if n != wantCells19 {
				t.Errorf("arm 19 has %d cell fixture(s), want %d: a cell that vanished would leave "+
					"its pairing unchecked and the arm still passing", n, wantCells19)
			}
			continue
		}
		if n > 1 {
			t.Errorf("%d fixtures claim arm %s: one fixture per arm is the invariant", n, arm)
		}
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

// --- arm 22's second oracles (rook, coop rumor/1884) ----------------------
//
// ROOK NAMED THE GAP IN ITS OWN CLEAR: arm 22's mutants M3 and M4 each die by
// ONE cell, because `TestArm22NamesTheActionItsContextAndTheHandle` is the sole
// oracle for the refusal's WORDING and for SKIP-vs-REFUSE at once. Two claims
// resting on one test means deleting or weakening that test takes both, and the
// wording is a RULING (fable, coop r9/17: the message names the action, its
// context and the handle it would have declared) rather than a convenience.
//
// BOTH ARMS BELOW ARE THE OTHER TIER, deliberately: that cell drives `arm22`
// over a hand-built `bindings.Bindings`, and these run `validate.Run` over FILES
// on disk. A fixture that stopped reaching the arm, or a loader that dropped
// `creates` on the way in, is invisible to the first and fails here.

// TestArm22sRefusalNamesTheThreeEndToEnd is the wording's second witness, taken
// off the fixture rather than off a struct literal. TestEveryFixtureTripsExactly
// ItsOwnArm already proves this fixture trips 22 and nothing else; it says
// nothing about what the refusal SAYS, which is the half R9.1 was ruled on.
func TestArm22sRefusalNamesTheThreeEndToEnd(t *testing.T) {
	const fixture = "22-creates-on-a-browser-action"
	dir := filepath.Join(fixturesDir(t), fixture)
	rep := validate.Run(dir)
	if len(rep.CouldNotRun) != 0 {
		t.Fatalf("%s could not be read: %v", fixture, rep.CouldNotRun)
	}
	var detail string
	for _, r := range rep.Refusals {
		if r.Arm == "22" {
			detail = r.Detail
		}
	}
	if detail == "" {
		t.Fatalf("%s did not refuse on arm 22, so there is no wording to check: %+v", fixture, rep.Refusals)
	}

	// THE EXPECTED NAMES COME FROM THE REAL PARSER over the fixture's own
	// bytes, not from literals here and not from a hand-rolled scan of the
	// TOML. My first cut did scan it by hand and was WORSE THAN NOTHING: this
	// fixture declares `creates` TWICE -- `["doc"]` on an api action and
	// `["published_doc"]` on the browser one -- so it asserted that the arm-22
	// refusal names the api action's handle too, which it must not, and the
	// assertion passed anyway because "doc" is a SUBSTRING of "published_doc".
	// A second reader of a declaration and a substring match, in one cell.
	brep, err := bindings.Load(filepath.Join(dir, "bindings.toml"), decl.TOML)
	if err != nil {
		t.Fatalf("loading the fixture's bindings: %v", err)
	}
	var subject bindings.Action
	for _, a := range brep.Bindings.Actions {
		if a.Context == "browser" && len(a.Creates) > 0 {
			if subject.Name != "" {
				t.Fatalf("the fixture declares two browser actions with `creates` (%s and %s); "+
					"this arm reads one", subject.Name, a.Name)
			}
			subject = a
		}
	}
	if subject.Name == "" {
		t.Fatal("the fixture declares no browser-context action with `creates`, so this test " +
			"asserted nothing: a scan that finds nothing agrees with every wording")
	}

	// WHOLE WORDS, for the substring reason above.
	whole := func(hay, word string) bool {
		return regexp.MustCompile(`\b` + regexp.QuoteMeta(word) + `\b`).MatchString(hay)
	}
	for _, want := range append([]string{subject.Name}, subject.Creates...) {
		if !whole(detail, want) {
			t.Errorf("the end-to-end refusal does not name %q, which fable's r9/17 ruling requires: %q",
				want, detail)
		}
	}

	// THE CONTEXT IS ASSERTED INSIDE THE ACTION'S OWN CLAUSE, not anywhere in
	// the detail, and a MUTANT is why. Dropping `a.Context` from the arm's
	// message SURVIVED a whole-detail match, because the arm's own boilerplate
	// opens with "browser-context action(s) declaring `creates`" -- so the word
	// was there whether or not the action's context was reported. Keying on a
	// word the message itself writes is keying on nothing.
	//
	// THE SEGMENT IS BOUNDED from the action's name to its first handle name,
	// which is the clause this arm composes per action. That pins the ORDER
	// name -> context -> handle without pinning the punctuation between them;
	// an arm that reorders them deliberately moves this bound with it.
	start := strings.Index(detail, subject.Name)
	end := strings.Index(detail[start:], subject.Creates[0])
	if start < 0 || end < 0 {
		t.Fatalf("cannot bound %q's clause in %q", subject.Name, detail)
	}
	clause := detail[start : start+end]
	if !whole(clause, subject.Context) {
		t.Errorf("the refusal names %q and %q but not the context %q between them (%q in %q): a "+
			"reader is told which handle is unmintable and not which world the action acts in",
			subject.Name, subject.Creates[0], subject.Context, clause, detail)
	}

	// AND IT NAMES NOTHING OF THE API ACTION'S, which is the arm's SCOPE and
	// the half the positive assertions cannot see. Driven as whole words, since
	// the api handle here is a substring of the browser one.
	for _, a := range brep.Bindings.Actions {
		if a.Context == "browser" {
			continue
		}
		for _, name := range append([]string{a.Name}, a.Creates...) {
			if name == subject.Name || slices.Contains(subject.Creates, name) {
				continue
			}
			if whole(detail, name) {
				t.Errorf("the refusal names %q, which belongs to the %s-context action %q: arm 22 "+
					"is about browser-context actions and a reader would go and change the wrong one: %q",
					name, a.Context, a.Name, detail)
			}
		}
	}
	t.Logf("PROOF refusal names %s, %s and %v, and nothing of the api action's",
		subject.Name, subject.Context, subject.Creates)
}

// TestArm22SkipsRatherThanPassesWithTheStageOff is skip-vs-refuse's second
// witness, and it is the ACCOUNTING claim rather than the arm's: with the stage
// off the twin must report arm 22 SKIPPED with a reason, never evaluated and
// never refused. A project that reported it as evaluated would be counting an
// arm whose precondition is absent, which is the distinction
// internal/validate's own Report comment exists for.
//
// AND THE CONTROL IS THE SAME TWIN UNTOUCHED, asserted first: arm 22 evaluated
// there. Without it, a build where arm 22 never ran at all would satisfy every
// assertion below.
func TestArm22SkipsRatherThanPassesWithTheStageOff(t *testing.T) {
	twin := filepath.Join("..", "..", "examples", "twin")

	on := validate.Run(twin)
	if _, skipped := on.Skipped["22"]; skipped {
		t.Fatalf("arm 22 skipped on the untouched twin, which declares the browser stage: %q",
			on.Skipped["22"])
	}
	if !slices.Contains(on.Evaluated, "22") {
		t.Fatalf("arm 22 did not evaluate on the untouched twin, so the stage-off case below "+
			"would pass for a reason that is not the stage: %v", on.Evaluated)
	}

	dir := filepath.Join(t.TempDir(), "twin")
	if out, err := exec.Command("cp", "-r", twin, dir).CombinedOutput(); err != nil {
		t.Fatalf("copying the twin: %v %s", err, out)
	}
	path := filepath.Join(dir, "platform.toml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const was = "browser = { enabled = true }"
	if !strings.Contains(string(raw), was) {
		t.Fatalf("%s does not declare %q, so this test cannot turn the stage off", path, was)
	}
	off := strings.Replace(string(raw), was, "browser = { enabled = false }", 1)
	if err := os.WriteFile(path, []byte(off), 0o644); err != nil {
		t.Fatal(err)
	}

	rep := validate.Run(dir)
	if len(rep.CouldNotRun) != 0 {
		t.Fatalf("the stage-off twin could not be read: %v", rep.CouldNotRun)
	}
	if why, skipped := rep.Skipped["22"]; !skipped {
		t.Errorf("arm 22 did not skip with the browser stage off: evaluated %v", rep.Evaluated)
	} else if !strings.Contains(why, "browser") {
		t.Errorf("arm 22 skipped without naming the stage: %q", why)
	}
	if slices.Contains(rep.Evaluated, "22") {
		t.Error("arm 22 is reported as EVALUATED with the stage off, so the run counts an arm " +
			"whose precondition is absent")
	}
	for _, r := range rep.Refusals {
		if r.Arm == "22" {
			t.Errorf("arm 22 REFUSED with the stage off, where it is vacuous: %q", r.Detail)
		}
	}
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

// wantArmCountClaims is how many places under docs/ attribute a COUNT of arms
// to this command by name. It is DECLARED for the reason every count in this
// project is: a scan that found nothing would pass, and the sentence it was
// written to check would have been deleted rather than corrected.
//
// IT MOVED FROM 2 TO 7 AT R9.1, and five of the five additions are prose this
// oracle could already have read and was kept away from. docs/cli.md:84 said so
// in its own words -- "it is corrected here and still outside the oracle,
// because the oracle also pins HOW MANY attributions the documents make and a
// third one fails it" -- and that sentence then rotted a SECOND time, from
// nineteen to twenty-three to a stale twenty-three. A count that fails because
// a true sentence was added is a count to move, never a reason to leave the
// sentence unattributed; the attribution is what the oracle reads, so prose kept
// out of the form is prose nothing checks.
const wantArmCountClaims = 7

// armCountClaim is the shape quill found wrong twice at coop r4/91:
// docs/contract.md said "R4.3's thirteen refusals" and "one of validate's
// thirteen" while the SAME FILE recorded the ruling at eighteen, two hundred
// lines apart. The number had been right when written -- C2 shipped thirteen
// arms and C3 brought the count to eighteen -- and prose does not recompute.
//
// IT MATCHES THE POSSESSIVE ONLY, deliberately: "five arms ask the adapter" and
// "the two browser-stage arms" are true statements about SUBSETS, and an oracle
// that refused every number beside the word "arm" would be refusing correct
// prose. What cannot be a subset is a count attributed to the command itself.
// THE HYPHENATED FORMS COME FIRST AND THAT IS NOT COSMETIC. Go's regexp is
// leftmost-FIRST, not leftmost-longest, and `twenty\b` matches INSIDE
// "twenty-one" because the hyphen is a word boundary -- so with "twenty" ahead
// of it in the alternation this oracle reads a correct document as saying
// "twenty" and errors against it. Measured at the 19 -> 21 bump (R6.2).
var armCountClaim = regexp.MustCompile(`(?i)(validate's|validate has|R4\.3's)\s+(?:own\s+)?` +
	`(twenty-one|twenty-two|twenty-three|twenty-four|twenty-five|one|two|three|four|five|six|seven|eight|nine|ten|` +
	`eleven|twelve|` +
	`thirteen|fourteen|fifteen|sixteen|seventeen|eighteen|nineteen|twenty)\b`)

// spelled is how this project writes a count in prose. An arm count past twenty
// fails the test rather than silently stopping being checked.
var spelled = map[int]string{
	13: "thirteen", 14: "fourteen", 15: "fifteen", 16: "sixteen",
	17: "seventeen", 18: "eighteen", 19: "nineteen", 20: "twenty",
	21: "twenty-one", 22: "twenty-two", 23: "twenty-three",
	24: "twenty-four", 25: "twenty-five",
}

// wantArmTotalClaims is how many times the documents state the count as a PAIR,
// and it is 1: docs/cli.md's "<N> of <N> is what a complete small project looks
// like", quoted here without its number so this comment cannot rot the way the
// sentence it describes did. A zero here would mean the oracle below agrees
// with every number, which is the failure its sibling above already names.
const wantArmTotalClaims = 1

// armTotalClaim is THE OTHER SHAPE A DOCUMENT STATES THIS COUNT IN, and the
// possessive oracle above is blind to it by design -- "matches the possessive
// only" is what keeps it from refusing true prose about subsets. So cli.md's
// pair was pinned by nothing: it read "Twenty-three of twenty-three" while
// TotalArms was 23, and it had to be moved BY HAND at this bump, found by
// grepping rather than by a failing test. That is the gap this closes.
//
// IT FLAGS ONLY AN EQUAL PAIR, and the reason covers exactly that narrowing:
// "nineteen of twenty-four" is a TRUE sentence about a project whose adapter
// cannot be reached, and this document writes it. A pair stating the same
// number twice can only be a claim about the total.
//
// THE VOCABULARY IS BUILT FROM `spelled` RATHER THAN TYPED, longest word first.
// The oracle above carries its alternation as a literal and needed a control
// test to keep "twenty" from matching inside "twenty-four" (Go's regexp is
// leftmost-FIRST); constructing it sorted cannot have that defect, and cannot
// drift from the table either.
var armTotalClaim = regexp.MustCompile(`(?i)\b(` + spelledAlternation() + `)\s+of\s+(` +
	spelledAlternation() + `)\b`)

func spelledAlternation() string {
	words := make([]string, 0, len(spelled))
	for _, w := range spelled {
		words = append(words, w)
	}
	sort.Slice(words, func(i, j int) bool {
		if len(words[i]) != len(words[j]) {
			return len(words[i]) > len(words[j])
		}
		return words[i] < words[j]
	})
	return strings.Join(words, "|")
}

// TestTheArmCountOracleReadsAHyphenatedNumberWhole is the control over the
// alternation's ORDER, which is load-bearing and was not asserted by anything.
//
// WHAT IT CATCHES: `twenty\b` matches inside "twenty-one" -- the hyphen is a
// word boundary -- so an alternation listing "twenty" before "twenty-one" reads
// a CORRECT document as claiming twenty and errors against it. Nothing about
// that is visible in the regexp's source; it reads as a list of words.
//
// AND IT IS SPECIFICALLY GO'S SEMANTICS, verified by ratchet at coop r6/29
// under go126: `(twenty|twenty-one)\b` yields "twenty" and this package's order
// yields "twenty-one", while the SAME wrong order under MustCompilePOSIX yields
// "twenty-one" because POSIX mode is leftmost-LONGEST and order-independent. So
// leftmost-first is what makes the order matter, not the alternation itself.
// Their caveat, recorded because it costs the next reader an afternoon: `\b` is
// not a valid escape in POSIX mode and panics, so a POSIX control cannot simply
// reuse this pattern.
func TestTheArmCountOracleReadsAHyphenatedNumberWhole(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"one of validate's twenty-one", "twenty-one"},
		{"one of validate's twenty-two", "twenty-two"},
		{"one of validate's twenty-four", "twenty-four"},
		{"one of validate's twenty-five", "twenty-five"},
		{"one of validate's twenty", "twenty"},
		{"one of validate's nineteen", "nineteen"},
	} {
		m := armCountClaim.FindStringSubmatch(c.in)
		if m == nil {
			t.Errorf("%q matched nothing: the oracle cannot check a claim it does not see", c.in)
			continue
		}
		if m[2] != c.want {
			t.Errorf("%q read as %q, want %q: a hyphenated number read as its first word "+
				"makes this oracle error against a correct document", c.in, m[2], c.want)
		}
	}
	// EVERY COUNT `spelled` KNOWS MUST BE READABLE, so adding a word to one and
	// not the other cannot pass. This is the drift the pair invites.
	for n, word := range spelled {
		in := "validate has " + word
		m := armCountClaim.FindStringSubmatch(in)
		if m == nil || m[2] != word {
			t.Errorf("spelled[%d] is %q and the claim pattern reads %v: a count this project "+
				"can write is a count it must be able to check", n, word, m)
		}
	}
}

// TestTheTotalClaimOracleSeesOnlyAnEqualPair is armTotalClaim's control, and
// the three cases ARE the narrowing: the pair it must flag, the unequal pair it
// must leave alone because the sentence is true, and a hyphenated word read
// whole rather than as "twenty".
func TestTheTotalClaimOracleSeesOnlyAnEqualPair(t *testing.T) {
	for _, c := range []struct {
		in    string
		equal bool
		word  string
	}{
		{"Twenty-four of twenty-four is what a complete small project looks like", true, "twenty-four"},
		{"a project whose adapter cannot be reached reports nineteen of twenty-four", false, ""},
		{"honestly reports twenty-two of twenty-two", true, "twenty-two"},
	} {
		m := armTotalClaim.FindStringSubmatch(c.in)
		if m == nil {
			t.Errorf("%q matched nothing: the oracle cannot check a claim it does not see", c.in)
			continue
		}
		if got := strings.EqualFold(m[1], m[2]); got != c.equal {
			t.Errorf("%q read as an equal pair = %v, want %v (groups %q and %q)",
				c.in, got, c.equal, m[1], m[2])
		}
		if c.equal && !strings.EqualFold(m[1], c.word) {
			t.Errorf("%q read its count as %q, want %q: a hyphenated number read as its "+
				"first word makes this oracle error against a correct document", c.in, m[1], c.word)
		}
	}
}

func TestTheDocumentedArmCountIsTotalArms(t *testing.T) {
	want, ok := spelled[validate.TotalArms]
	if !ok {
		t.Fatalf("TotalArms is %d and this test does not know how the prose spells it; "+
			"add it to `spelled` rather than leaving the documents unchecked", validate.TotalArms)
	}
	docs := docCorpus(t)

	found, pairs := 0, 0
	for _, d := range docs {
		raw, rerr := os.ReadFile(d)
		if rerr != nil {
			t.Fatal(rerr)
		}
		for _, m := range armCountClaim.FindAllStringSubmatch(string(raw), -1) {
			found++
			if !strings.EqualFold(m[2], want) {
				t.Errorf("%s says %q, and validate.TotalArms is %d (%s)",
					filepath.Base(d), m[0], validate.TotalArms, want)
			}
		}
		for _, m := range armTotalClaim.FindAllStringSubmatch(string(raw), -1) {
			if !strings.EqualFold(m[1], m[2]) {
				continue // a subset reading against the total, and true prose
			}
			pairs++
			if !strings.EqualFold(m[1], want) {
				t.Errorf("%s says %q, and validate.TotalArms is %d (%s)",
					filepath.Base(d), m[0], validate.TotalArms, want)
			}
		}
	}
	if pairs != wantArmTotalClaims {
		t.Errorf("the documents state the count as an equal pair %d time(s), want %d: "+
			"a scan that finds nothing agrees with every number", pairs, wantArmTotalClaims)
	}
	if found != wantArmCountClaims {
		t.Errorf("the documents attribute a count to this command %d time(s), want %d: "+
			"a scan that finds nothing agrees with every number", found, wantArmCountClaims)
	}
	t.Logf("PROOF %d documented arm count(s) read, all %s", found, want)
}

// docCorpus is every document the prose oracles read: docs/ AND its
// subdirectories, in path order. It was `docs/*.md` until R9.1, and that glob
// is not recursive, so docs/adr/ was unread by all three families at once.
// Widening it moves no count today -- measured, docs/adr contributes zero to
// each -- which is the whole point: the hole is closed before something lands
// in it rather than after.
func docCorpus(t *testing.T) []string {
	t.Helper()
	var out []string
	err := filepath.Walk(filepath.Join("..", "..", "docs"), func(p string, info os.FileInfo, werr error) error {
		if werr != nil {
			return werr
		}
		if !info.IsDir() && strings.HasSuffix(p, ".md") {
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking docs/: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("no documents to read: a corpus of nothing agrees with every claim")
	}
	sort.Strings(out)
	return out
}

// wantArmFigureClaims is how many times the corpus and ci.yml write the figure
// WITH ITS NOUN -- `N of M arm` -- and it is 9: docs/cli.md's two sample runs,
// docs/testing-methodology.md's three measured readings, and ci.yml's two greps
// with the two error messages beside them. A zero here would mean the oracle
// below agrees with every number, which is the failure its siblings above
// already name.
const wantArmFigureClaims = 9

// armFigureClaim is THE THIRD SHAPE A DOCUMENT STATES THIS COUNT IN, and R9.1
// is why it exists. The 24 -> 25 bump left FOUR stale numbers in
// docs/testing-methodology.md with every suite green, because that paragraph
// writes each figure in DIGITS -- which armTotalClaim, a SPELLED pair, cannot
// read -- and attributed none of them possessively, which is the only subject
// armCountClaim answers to. ci.yml's own comment named that document as a
// co-mover of the count in so many words, and nothing enforced it.
//
// IT KEYS ON THE NOUN, and the reason covers exactly that narrowing. A bare
// equal-digit-pair oracle is the obvious cure and is WRONG, measured in this
// tree before the pattern was chosen: docs/design.md's "2 of 2 into room for
// 1", docs/release.md's "24 of 24 allowed" -- a licence gate, not arms -- and
// docs/testing-methodology.md's own "5 of 5 from 5 of 11" are true `N of N`
// sentences about other things. The noun is also the form the PRODUCT prints,
// so the documents are held to the spelling a reader can copy out of a real
// run: change the observable rather than tune the pattern.
//
// THE DENOMINATOR IS THE CLAIM, not the pair. `20 of 25 arm(s) evaluated` is
// what an unreachable adapter honestly prints, so a numerator below the total
// is true prose and only the total is pinned -- and the numerator is still
// bounded by it, because no run evaluates more arms than exist.
var armFigureClaim = regexp.MustCompile(`(\d+) of (\d+) arm`)

func TestEveryArmFigureHasTotalArmsAsItsDenominator(t *testing.T) {
	paths := append(docCorpus(t), filepath.Join("..", "..", ".forgejo", "workflows", "ci.yml"))
	total := strconv.Itoa(validate.TotalArms)
	found := 0
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s could not be read, so its arm figures are unchecked: %v", path, err)
		}
		for _, m := range armFigureClaim.FindAllStringSubmatch(string(raw), -1) {
			found++
			if m[2] != total {
				t.Errorf("%s says %q and validate.TotalArms is %d: the DENOMINATOR of an arm "+
					"figure is the total, whatever the numerator",
					filepath.Base(path), m[0], validate.TotalArms)
				continue
			}
			n, cerr := strconv.Atoi(m[1])
			if cerr != nil || n > validate.TotalArms {
				t.Errorf("%s says %q: no run evaluates more arms than exist",
					filepath.Base(path), m[0])
			}
		}
	}
	if found != wantArmFigureClaims {
		t.Errorf("the documents and the workflow write an arm figure %d time(s), want %d: "+
			"a scan that finds nothing agrees with every number", found, wantArmFigureClaims)
	}
	t.Logf("PROOF %d arm figure(s) read, every denominator %s", found, total)
}

// TestTheArmFigureOracleReadsOnlyAnArmFigure is armFigureClaim's control, and
// the six cases ARE the narrowing: the two figures it must pin, and the four
// `N of N` sentences this project's own documents really write about other
// things, which a digit-pair oracle would have errored against.
func TestTheArmFigureOracleReadsOnlyAnArmFigure(t *testing.T) {
	for _, c := range []struct {
		in    string
		match bool
	}{
		{"validate: twin -- 20 of 25 arm(s) evaluated; 2 action(s)", true},
		{"22 of 25 arms with the browser stage off", true},
		{"splits 2 of 2 into room for 1 and the clause fires", false},
		{`"24 of 24 allowed" is what a gate that always says yes also prints`, false},
		{"5 of 5 from 5 of 11. Fixed in `abc400a`", false},
		{"11 of 24 toy seeds find a bug and 13 walk clean", false},
	} {
		if got := armFigureClaim.MatchString(c.in); got != c.match {
			t.Errorf("%q matched = %v, want %v: a figure this oracle cannot see is unpinned, "+
				"and a true sentence about something else that it CAN see makes it error "+
				"against correct prose", c.in, got, c.match)
		}
	}
}

// wantArmNounClaims is how many times the corpus writes a spelled count
// straight onto the noun, and it is 4: docs/cli.md's declared total and its R5
// history, and docs/contract.md's two.
const wantArmNounClaims = 4

// THE FOURTH SHAPE IS THE RESIDUAL THE OTHER THREE LEAVE, and rook named it at
// coop r9/20 while the unit above was being cut: a SPELLED, NON-POSSESSIVE,
// NON-PAIR total -- "twenty-five arms are declared" -- is read by none of them.
// BY VALUE it is irreducible, because "nineteen arms" is a true sentence about a
// project whose adapter cannot be reached and no value oracle separates that
// from a stale total. BY FORM it is not: a spelled number written onto the noun
// must be possessive or half of a pair, and both shapes the value oracles
// already read. So this oracle carries NO NUMBER and cannot rot; what it does is
// make the other three total.
//
// IT WAS ALREADY HOLDING A STALE VALUE. docs/cli.md said "Twenty-three arms are
// declared" at TotalArms 25, having been corrected from NINETEEN one release
// earlier -- and the same sentence explained that it was left outside the oracle
// on purpose, because a third attribution would fail wantArmCountClaims. That
// is the defect this closes: the count moves, the sentence gets attributed.
//
// A PHASE POSSESSIVE COUNTS AS ATTRIBUTION AND IS NOT VALUE-CHECKED, and the
// reason covers exactly that: "R5's eighteen arms" DATES the claim, so it is
// true prose about a tree that is gone, where "validate's eighteen arms" would
// be a claim about this one. The value oracles keep their three literal
// subjects; this one only asks that somebody be named.
var (
	armNounSuffix = `\*{0,2}\s+\*{0,2}arms?\b`
	// armNounBare finds EVERY spelled count written onto the noun.
	armNounBare = regexp.MustCompile(`(?i)\b(` + spelledAlternation() + `)` + armNounSuffix)
	// armNounAttributed finds the possessive ones, the value oracles' own two
	// subjects plus a phase.
	armNounAttributed = regexp.MustCompile(`(?i)\b(?:validate's|validate has|R\d+(?:\.\d+)?'s)\s+(?:own\s+)?(` +
		spelledAlternation() + `)` + armNounSuffix)
	// armNounPaired finds the pair form, which is armTotalClaim's.
	armNounPaired = regexp.MustCompile(`(?i)\b(` + spelledAlternation() + `)\s+of\s+(` +
		spelledAlternation() + `)` + armNounSuffix)
)

// attributedEnds is the set of offsets at which an ATTRIBUTED count ends. A
// bare match is covered when an attributed one ends where it ends, which is
// what makes "R4.3's twenty-five arms" one claim rather than two readings of it.
func attributedEnds(text string) map[int]bool {
	ends := map[int]bool{}
	for _, re := range []*regexp.Regexp{armNounAttributed, armNounPaired} {
		for _, loc := range re.FindAllStringIndex(text, -1) {
			ends[loc[1]] = true
		}
	}
	return ends
}

func TestAnArmCountBeforeTheNounIsAttributed(t *testing.T) {
	paths := append(docCorpus(t), filepath.Join("..", "..", ".forgejo", "workflows", "ci.yml"))
	found := 0
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s could not be read, so its arm counts are unchecked: %v", path, err)
		}
		text := string(raw)
		ends := attributedEnds(text)
		for _, loc := range armNounBare.FindAllStringIndex(text, -1) {
			found++
			if ends[loc[1]] {
				continue
			}
			t.Errorf("%s:%d writes %q and names nobody: a spelled count beside the noun is "+
				"possessive or half of a pair, which are the two shapes the value oracles "+
				"read -- this one is checked by neither and rots in place (rook, coop r9/20)",
				filepath.Base(path), 1+strings.Count(text[:loc[0]], "\n"),
				strings.Join(strings.Fields(text[loc[0]:loc[1]]), " "))
		}
	}
	if found != wantArmNounClaims {
		t.Errorf("the corpus writes a spelled count beside the noun %d time(s), want %d: "+
			"a scan that finds nothing accepts every unattributed count", found, wantArmNounClaims)
	}
	t.Logf("PROOF %d spelled arm count(s) read, every one attributed", found)
}

// TestTheAttributionOracleRefusesExactlyTheUnattributed is the control, and the
// first case is the real sentence this oracle was written for -- docs/cli.md as
// it stood at TotalArms 25, saying twenty-three.
func TestTheAttributionOracleRefusesExactlyTheUnattributed(t *testing.T) {
	for _, c := range []struct {
		in string
		ok bool
	}{
		{"Twenty-three arms\nare declared -- the figure said NINETEEN", false},
		{"nineteen arms evaluated", false},
		{"**Validate has\ntwenty-five arms** -- the figure said", true},
		{"It is one of R4.3's twenty-five arms now", true},
		{"with every\none of R5's eighteen arms passing it", true},
		{"Twenty-five of twenty-five arms is what a complete project looks like", true},
	} {
		locs := armNounBare.FindAllStringIndex(c.in, -1)
		if len(locs) == 0 {
			t.Errorf("%q matched nothing: a count this oracle cannot see is one it cannot refuse", c.in)
			continue
		}
		ends := attributedEnds(c.in)
		last := locs[len(locs)-1]
		if got := ends[last[1]]; got != c.ok {
			t.Errorf("%q read as attributed = %v, want %v: the refusal is the whole oracle, so "+
				"both directions are the test", c.in, got, c.ok)
		}
	}
}

// TestANemesisKindMayBeWeighted is arm 19's weight cell's own control, and the
// test that would have caught the cell's first cut.
//
// IT REFUSED THE DOCUMENTED SHAPE. docs/contract.md's `[weights.default]`
// sample carries `suborn = 1` beside the comment "nemesis kinds are action
// kinds in the same pool", and the cell's referent set was the declared actions
// alone -- so the twin with `suborn = 1` added was refused, which is this
// project's documentation telling a reader to write a project this arm rejects.
// A cell that pairs against a set needs a positive control for every KIND of
// member of that set, not only for the one the fixture uses.
func TestANemesisKindMayBeWeighted(t *testing.T) {
	twin := filepath.Join("..", "..", "examples", "twin")
	for _, kind := range validate.NemesisKinds {
		t.Run(kind, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "twin")
			if out, err := exec.Command("cp", "-r", twin, dir).CombinedOutput(); err != nil {
				t.Fatalf("copying the twin: %v %s", err, out)
			}
			p := filepath.Join(dir, "platform.toml")
			raw, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			const anchor = "publish_doc = 1"
			if !strings.Contains(string(raw), anchor) {
				t.Fatalf("the twin's [weights.default] no longer carries %q, so this control weights nothing", anchor)
			}
			with := strings.Replace(string(raw), anchor, anchor+"\n"+kind+" = 1", 1)
			if err := os.WriteFile(p, []byte(with), 0o644); err != nil {
				t.Fatal(err)
			}
			rep := validate.Run(dir)
			for _, r := range rep.Refusals {
				t.Errorf("a weight for the nemesis kind %s was refused: %s", kind, r)
			}
			t.Logf("PROOF %s may be weighted beside the project's own actions", kind)
		})
	}
}

// TestTheNemesisKindsAreTheDocumentsFour pins internal/validate's COPY of a
// vocabulary it does not own against docs/design.md's own sentence. The list
// belongs to R5.1's scheduler, which is not on main; until it is, two places
// name the four and only one of them is executable.
func TestTheNemesisKindsAreTheDocumentsFour(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "design.md"))
	if err != nil {
		t.Fatal(err)
	}
	// the sentence that introduces them, and nothing else: "The nemesis kinds
	// are action kinds in the same pool with their own weights: ..." up to the
	// end of that sentence.
	window := regexp.MustCompile(`(?s)The nemesis kinds are action kinds in the\s+same pool with their own weights:(.*?)\.\s`)
	m := window.FindStringSubmatch(string(raw))
	if m == nil {
		t.Fatal("docs/design.md no longer introduces the nemesis kinds in the sentence this oracle reads: " +
			"move the oracle with the document rather than deleting it")
	}
	named := map[string]bool{}
	for _, q := range regexp.MustCompile("`([a-z]+)`").FindAllStringSubmatch(m[1], -1) {
		named[q[1]] = true
	}
	if len(named) != len(validate.NemesisKinds) {
		t.Fatalf("docs/design.md names %d nemesis kind(s) %v and internal/validate declares %d %v",
			len(named), named, len(validate.NemesisKinds), validate.NemesisKinds)
	}
	for _, k := range validate.NemesisKinds {
		if !named[k] {
			t.Errorf("internal/validate declares %q and docs/design.md's sentence does not name it", k)
		}
	}
	t.Logf("PROOF the document and this package name the same %d nemesis kind(s)", len(named))
}

// TestWhatIsNotAProjectCannotRunRatherThanBeingRefused is quill's first finding
// at coop r4/91, and the arm that would have caught it is the THIRD case below
// rather than either of the first two.
//
// WHAT WAS WRONG: `rumor validate /a/mistyped/path` answered REFUSED
// no-bindings -- exit 1, "the project is wrong" -- about a directory that was
// not there to be wrong. An existing project missing its bindings.toml answered
// the same clause, and the two runs differed only in the basename on the tally
// line, so neither the status nor the message told a typo from a real vacuity.
//
// WHY THE EMPTY DIRECTORY IS IN THE SAME TEST: a fix that made the no-bindings
// refusal could-not-run as well would have traded one indistinguishable pair
// for another and still passed an absent-path arm. The pair is the assertion;
// either half alone is satisfied by collapsing them.
func TestWhatIsNotAProjectCannotRunRatherThanBeingRefused(t *testing.T) {
	aFile := filepath.Join(t.TempDir(), "platform.toml")
	if err := os.WriteFile(aFile, []byte("# a file where a project was named\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		what   string
		path   string
		exit   int
		clause string // the refusal expected, or "" for none
		names  bool   // the path itself appears in a could-not-run line
	}{
		{"a path that is not there", filepath.Join(t.TempDir(), "typo-here"), 2, "", true},
		{"a file named as a project", aFile, 2, "", true},
		{"a directory with no bindings.toml", t.TempDir(), 1, "no-bindings", false},
	}
	for _, c := range cases {
		t.Run(c.what, func(t *testing.T) {
			rep := validate.Run(c.path)
			if rep.Exit() != c.exit {
				t.Errorf("%s exits %d, want %d (refusals %v, could-not-run %v)",
					c.what, rep.Exit(), c.exit, rep.Refusals, rep.CouldNotRun)
			}
			var clauses []string
			for _, r := range rep.Refusals {
				clauses = append(clauses, r.Arm)
			}
			switch {
			case c.clause == "" && len(clauses) != 0:
				t.Errorf("%s refused %v: a path that cannot be opened is not a project being wrong",
					c.what, clauses)
			case c.clause != "" && (len(clauses) != 1 || clauses[0] != c.clause):
				t.Errorf("%s refused %v, want exactly [%s]", c.what, clauses, c.clause)
			}
			if c.names {
				// THE PATH, not just the failure: the usual cause is a typo,
				// and a typo is only visible beside what was typed.
				joined := strings.Join(rep.CouldNotRun, "\n")
				if !strings.Contains(joined, c.path) {
					t.Errorf("%s does not name %q in its could-not-run line(s): %v",
						c.what, c.path, rep.CouldNotRun)
				}
			} else if len(rep.CouldNotRun) != 0 {
				t.Errorf("%s reported could-not-run %v: a directory that was read is the project's own verdict",
					c.what, rep.CouldNotRun)
			}
			if n := len(rep.Evaluated) + len(rep.Skipped); n != validate.TotalArms {
				t.Errorf("%s accounts for %d arm(s), want %d: a run that measured nothing must say so "+
					"beside every arm, not only in the tally", c.what, n, validate.TotalArms)
			}
			t.Logf("PROOF %s -> exit %d, %d refusal(s), %d could-not-run, %d arm(s) accounted",
				c.what, rep.Exit(), len(rep.Refusals), len(rep.CouldNotRun),
				len(rep.Evaluated)+len(rep.Skipped))
		})
	}
}

// TestAnUnreadableDeclarationIsNotAVacuityRefusal is the sibling of the finding
// above, found while fixing it and the same mistake one level in: a vacuity
// clause is a claim about what the project DECLARES, and both counts it reads
// are zero when the file holding them would not parse.
//
// MEASURED BEFORE THE FIX: an unparsable rules.json printed "could not run:
// rules.json: invalid character" AND "REFUSED no-rule-declared: rules.json
// declares no rule" -- a verdict about a declaration nobody read. Unparsable
// cases did it through the no-case clause. The exit status was right either way
// (2 outranks 1), which is why this needs an arm reading the REFUSALS and not
// only the status.
func TestAnUnreadableDeclarationIsNotAVacuityRefusal(t *testing.T) {
	twin := filepath.Join("..", "..", "examples", "twin")
	cases := []struct {
		what string
		set  func(t *testing.T, dir string)
	}{
		// THESE TWO ARE HERE FOR THE ACCOUNTING HALF rather than for a false
		// refusal: a decode failure in either file returned before any arm, so
		// the tally read "0 of 18 arm(s) evaluated" with no reason beside a
		// single arm -- the one place a reader most needs to be told that
		// nothing was measured.
		{"bindings.toml that will not parse", func(t *testing.T, dir string) {
			if err := os.WriteFile(filepath.Join(dir, "bindings.toml"), []byte("this is not toml {{{\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"platform.toml that will not parse", func(t *testing.T, dir string) {
			if err := os.WriteFile(filepath.Join(dir, "platform.toml"), []byte("this is not toml {{{\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"rules.json that will not parse", func(t *testing.T, dir string) {
			if err := os.WriteFile(filepath.Join(dir, "rules.json"), []byte("not json\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"every case file unparsable", func(t *testing.T, dir string) {
			files, err := filepath.Glob(filepath.Join(dir, "cases", "*.json"))
			if err != nil || len(files) == 0 {
				t.Fatalf("the twin has no case files to break: %v %v", files, err)
			}
			for _, f := range files {
				if err := os.WriteFile(f, []byte("{\n"), 0o644); err != nil {
					t.Fatal(err)
				}
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
			if rep.Exit() != 2 {
				t.Errorf("%s exits %d, want 2 (could-not-run)", c.what, rep.Exit())
			}
			if len(rep.CouldNotRun) == 0 {
				t.Fatalf("%s reported no could-not-run at all", c.what)
			}
			if len(rep.Refusals) != 0 {
				t.Errorf("%s ALSO refused %v: what an unread declaration says is unknown, and a vacuity "+
					"clause claims to know it", c.what, rep.Refusals)
			}
			if n := len(rep.Evaluated) + len(rep.Skipped); n != validate.TotalArms {
				t.Errorf("%s accounts for %d arm(s), want %d", c.what, n, validate.TotalArms)
			}
			t.Logf("PROOF %s -> exit 2, %d could-not-run, 0 refusals, %d arm(s) accounted",
				c.what, len(rep.CouldNotRun), len(rep.Evaluated)+len(rep.Skipped))
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

// TestBindingsRefusesANonPositiveBound is issue 101's added bullet, and it is
// here beside the zero-action arm above because both are PARSE refusals that
// validate surfaces rather than arms of its own.
//
// WHY THE SIGN IS REFUSED AT THE PARSE. internal/decl's `Reader.Duration`
// already refuses `d <= 0` with this exact sentence, so until this landed TWO
// duration readers in one tree disagreed: decl refused a zero and the claim
// parser accepted it. The engine then compares `now-claimTime > Bound` in
// nanoseconds, so a declared `0s` makes every disagreement instantly past its
// bound and a negative one makes the comparison meaningless -- measured on the
// toy before the refusal existed, `0s` made its stale bug VANISH at rc 0 and
// `-5s` found a DIFFERENT bug at a different step, with `rumor validate`
// accepting both. A value that changes which defect a run reports must not
// reach the engine.
//
// THE THIRD CELL IS THE ONE THAT MAKES THE OTHER TWO MEAN ANYTHING: a positive
// bound must NOT produce this sentence, or the cells would pass against a
// parser that refused every bound. Each cell also asserts the parser RAN -- one
// refusal, exit 1 -- so a crash cannot satisfy the absence.
func TestBindingsRefusesANonPositiveBound(t *testing.T) {
	const sentence = "a duration of zero or less covers nothing"
	for _, c := range []struct {
		bound  string
		refuse bool
	}{
		{"0s", true},
		{"-5s", true},
		{"30s", false},
	} {
		dir := t.TempDir()
		body := "[claim.\"doc.published\"]\ntiming = \"eventual\"\nbound = \"" + c.bound + "\"\n"
		if err := os.WriteFile(filepath.Join(dir, "bindings.toml"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		rep := validate.Run(dir)
		// THE SUBJECT RAN: a bindings file with no actions is refused whatever
		// the bound is, so this holds in all three cells and a cell cannot pass
		// by the parser having died.
		if rep.Exit() != 1 || len(rep.Refusals) == 0 {
			t.Fatalf("bound %q: exit %d with %d refusal(s); the parser did not run, so neither "+
				"the presence nor the absence below proves anything",
				c.bound, rep.Exit(), len(rep.Refusals))
		}
		var joined string
		for _, r := range rep.Refusals {
			joined += r.Detail + " "
		}
		got := strings.Contains(joined, sentence)
		if got != c.refuse {
			t.Fatalf("bound %q: refused-for-its-sign is %v, want %v; the refusal reads %q",
				c.bound, got, c.refuse, strings.TrimSpace(joined))
		}
		if c.refuse && !strings.Contains(joined, strconv.Quote(c.bound)) {
			t.Errorf("bound %q: the refusal does not quote the value it refused: %q",
				c.bound, strings.TrimSpace(joined))
		}
	}
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
	// R9.1's arm 22 changes the twin's browser action alone, adding `creates`
	// to it; the generator writes its reason as a comment header on that same
	// file rather than a WHY of its own, which is 7b's shape.
	"22-creates-on-a-browser-action": {"bindings.toml"},

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

	// 7c AND 7d EACH ADD A [quiesce] TABLE, which is one edit to one file and
	// still ONE CONDITION under fable's r4/74 rule: the table is the setup --
	// the twin declares no [quiesce] at all -- and the condition is the one
	// field inside it. 7d's probe carries a timeout it does not need for its
	// own refusal, precisely so it cannot also trip 7c.
	"7c-probe-without-a-timeout":     {"bindings.toml"},
	"7d-probe-of-an-unknown-kind":    {"bindings.toml"},
	"7e-probe-query-on-a-wrong-kind": {"bindings.toml"},
	"10b-unreadable-seed":            {"seeds.lock"},

	// THE LIVE ARMS change the project's ASK and never the adapter's surface:
	// changing what the adapter can do would test the fake adapter, and these
	// arms are about what the project asks for.
	// ARM 19's CELLS. Each breaks ONE reference; the venire cell needs the
	// ground prepared (the twin declares no roles at all, and the cell pairs
	// only where a project declares some), so it declares both edits the way
	// fable's r4/74 rule allows and its README names which is which.
	"19/requires-unknown-rule":           {"bindings.toml"},
	"19/action-without-a-weight":         {"platform.toml"},
	"19/kind-without-a-weight":           {"platform.toml"},
	"19/weight-unknown-action":           {"platform.toml"},
	"19/venire-undeclared-role":          {"bindings.toml", "platform.toml", "WHY-venire-undeclared-role.md"},
	"19/action-arg-unknown-handle":       {"bindings.toml"},
	"19/observe-arg-unknown-handle":      {"bindings.toml"},
	"19/flow-placeholder-unknown-handle": {"bindings.toml"},
	"19/claim-table-unemitted-type":      {"bindings.toml"},
	"19/observation-not-in-describe":     {"bindings.toml"},
	// TWO TABLES, ONE CHANGE: the defect is the dangling instant name, and
	// `[clock]` is the precondition that keeps arm 20 quiet. Without it a
	// `[deadline]` with contents trips arm 20 as well, and a fixture tripping
	// two arms shows neither.
	"19/deadline-instant-unknown-observation": {"bindings.toml"},

	// `contended`'s TWO REFERENCES, one cell each. Both repair the twin's own
	// planted `instants` name while they are in bindings.toml -- without that
	// the fixture would trip the instants cell as well, and a fixture tripping
	// two cells shows neither. That is one CONDITION per fable's r4/74 rule and
	// not two changes: the repair is what isolates the condition.
	"19/contended-limit-unknown-argument":   {"bindings.toml"},
	"19/contended-held-unknown-observation": {"bindings.toml"},

	// THE TWO PROFILE-VENIRE CELLS (fable, coop rumor/1662). THREE EDITS EACH
	// AND ONE CONDITION under fable's r4/74 rule: the twin declares neither a
	// role nor a profile, so both are the ground the cell stands on, and the
	// appended row is the condition. Each fixture's README names which is which.
	"19/profile-venire-undeclared-role":  {"bindings.toml", "platform.toml", "WHY-profile-venire-undeclared-role.md"},
	"19/profile-venire-context-no-stage": {"bindings.toml", "platform.toml", "WHY-profile-venire-context-no-stage.md"},

	// THE TWO PROFILE-OBSERVATION CELLS (fable, coop rumor/1678). ONE EDIT AND
	// THREE, and the asymmetry is the pairing's rather than an oversight: a
	// name nothing declares needs no ground in bindings.toml, while a source no
	// stage drives needs a browser-source observation to exist -- and needs the
	// SURFACE to describe it, or the fixture trips 19/observation-not-in-describe's
	// cell as well and shows neither.
	"19/profile-observation-undeclared":      {"platform.toml", "WHY-profile-observation-undeclared.md"},
	"19/profile-observation-source-no-stage": {"bindings.toml", "surface.json", "platform.toml", "WHY-profile-observation-source-no-stage.md"},

	"9a-fault-unavailable":     {"bindings.toml"},
	"9b-volume-unavailable":    {"bindings.toml"},
	"9c-upgrade-unavailable":   {"bindings.toml"},
	"13-contract-mismatch":     {"platform.toml"},
	"21-context-not-described": {"bindings.toml"},
	"20-deadline-no-clock":     {"bindings.toml"},
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

// allArms is every arm, named. fable ruled the count at coop breakout-r4/6 and
// it moves only with a count posted in the live phase thread first: 23 -> 24
// went to coop breakout-r4/8 before this line or TotalArms moved.
var allArms = []string{
	"1", "2", "3", "4", "5", "6", "7a", "7b", "7c", "7d", "7e", "8",
	"9a", "9b", "9c", "10a", "10b", "11", "12a", "12b", "13", "19", "20",
	"21", "22",
}

// TestEveryArmAccountsForItself is the every-arm-accounted invariant, and it is
// an accounting one rather than a single project's number.
//
// IT WAS NAMED "18 of 18" HERE UNTIL R8.4, and the numbers below were 18 twice
// more: three statements of a count that has to track TotalArms, inside the
// comment about a comment that outlived its measurement. They are the property
// now, which is what the assertions were always about.
//
// WHY NOT SIMPLY THE TWIN, which is the question this comment got wrong for
// three releases: it said the twin reports SIXTEEN of eighteen because arms 4
// and 5 are vacuous without `stages.browser.enabled` and "the twin cannot
// enable them". The twin has enabled them since 0120352 -- it declares a
// browser action and its flow to satisfy fable's D9 drill -- and reports EVERY
// arm evaluated when a `rumor` is on PATH for its `fake-adapter` to spawn. The
// comment
// outlived its measurement (quill, coop r4/91).
//
// The answer that does not rot: a NUMBER about one project is a claim about
// that project's declarations and its environment -- five arms need a spawnable
// adapter -- so what this test pins is the ACCOUNTING, that every arm is
// evaluated SOMEWHERE across the twin and the fixtures and that no run ever
// both evaluates and skips one. The twin passing all of them is asserted where
// it belongs, against the documented output in
// TestTheDocumentedOutputIsTheBinarysOutput.
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
	// exists to spawn decides five arms, and the documented figure depends on
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

// TestTheWorkflowsArmCountIsTotalArms holds CI's own literal to the constant.
//
// WHY IT EXISTS, and it is the fourth copy of this number rather than a
// hypothetical: the R6.2 bump moved TotalArms, tests/validate's `allArms`,
// internal/validate's `allArmIDs` and both docs -- and `.forgejo/workflows/ci.yml`
// greps the twin's and the toy's output for `19 of 19 arm(s) evaluated`. Nothing
// in Go could see that string, so `go test ./...`, `go vet`, gofmt and
// golangci-lint were all green on a commit whose CI failed at the last step of
// the run, with a message that reads like an arm was dropped.
//
// IT IS NOT A DUPLICATE OF TestTheDocumentedOutputIsTheBinarysOutput. That one
// compares docs/cli.md's console blocks to what the binary prints; the gate's
// literal is in a workflow, is a grep rather than a block, and is the one copy
// whose failure costs a CI round trip instead of a local second.
//
// AND IT CHECKS THE COUNT OF LITERALS TOO. A gate whose assertion was deleted
// passes forever, so the number of places ci.yml makes this claim is declared:
// two, the twin's and the toy's.
func TestTheWorkflowsArmCountIsTotalArms(t *testing.T) {
	const wantClaims = 2
	path := filepath.Join("..", "..", ".forgejo", "workflows", "ci.yml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the CI workflow could not be read, so its arm-count gate is unchecked: %v", err)
	}
	claim := regexp.MustCompile(`(\d+) of (\d+) arm\(s\) evaluated`)
	found := claim.FindAllStringSubmatch(string(raw), -1)
	if len(found) != wantClaims {
		t.Fatalf("ci.yml makes the arm-count claim %d time(s), want %d: a deleted gate passes "+
			"forever, and a new one needs this number moved deliberately", len(found), wantClaims)
	}
	for _, m := range found {
		for _, n := range m[1:] {
			if n != strconv.Itoa(validate.TotalArms) {
				t.Errorf("ci.yml greps for %q and validate.TotalArms is %d: the gate would refuse "+
					"a correct run, and it refuses it in CI rather than here",
					m[0], validate.TotalArms)
			}
		}
	}
}

// TestEachArm19FixtureTripsExactlyOneCell is the ONE-CELL invariant, and it is
// a separate test because TestEveryFixtureTripsExactlyItsOwnArm cannot see it:
// arm 19 is one arm with seventeen cells, so a fixture tripping two of them trips
// one ARM and passes there. Every `changedBy` comment above that says "a
// fixture tripping two cells shows neither" was enforced by reading until now.
//
// IT CAUGHT ONE IN THE HOUR IT WAS WRITTEN, which is why it exists rather than
// being a tidy-up. 19/profile-venire-context-no-stage declared a role named
// `viewer` and no other, and declaring ANY role turns on the venire cell's
// "where roles are declared at all" guard -- so the twin's own `[[venire]] role
// = "author"` became unresolved and the fixture reported TWO notes, its
// condition being the second. The fixture now declares `author` as well.
//
// THE SPLIT IS ON THE JOIN arm 19 ITSELF USES. Its refusal is the notes joined
// with "; ", so one note is one cell; a note that ever contains that pair would
// fail here loudly rather than quietly, which is the right way round.
func TestEachArm19FixtureTripsExactlyOneCell(t *testing.T) {
	for _, f := range fixtures(t) {
		if !strings.HasPrefix(f, "19/") {
			continue
		}
		t.Run(f, func(t *testing.T) {
			rep := validate.Run(filepath.Join("fixtures", f))
			detail := ""
			for _, r := range rep.Refusals {
				if r.Arm == "19" {
					detail = r.Detail
				}
			}
			if detail == "" {
				t.Fatalf("no arm 19 refusal, and this fixture is one of its cells: %v", rep.Refusals)
			}
			if notes := strings.Split(detail, "; "); len(notes) != 1 {
				t.Errorf("%d cell(s) tripped, want 1: a fixture tripping two cells shows neither:\n  %s",
					len(notes), strings.Join(notes, "\n  "))
			}
		})
	}
}

// TestArm21ReadsItsOwnVerdict is arm 21's JUDGEMENT, and it exists because
// ratchet measured that nothing read it (coop r8/149). The arm was wired,
// accounted for, and skipped BY NAME when no adapter answers -- and three
// mutants went green: its action comparison, its role comparison and its
// `none` exemption, every one of them REACHED rather than unreachable. A drive
// recorded in a commit message is not a check (ratchet, coop r6/173), and the
// drive for this arm was mine, by hand, in a message.
//
// THE ACTION PATH IS THE FIXTURE, 21-context-not-described, and is not repeated
// here. These are the two cases a fixture cannot carry: the ROLE path, whose
// role the twin does not declare, and the `none` EXEMPTION, which is a POSITIVE
// -- a fixture says which arm refused and cannot say that no arm did.
//
// `author` AND NOT A NEW NAME, for the reason my own profile-venire fixture
// taught an hour before this: declaring ANY role turns on arm 19's venire cell,
// and the twin's own `[[venire]]` draws `author`, so any other name trips that
// cell as well and the case stops being about arm 21.
//
// AND EACH CELL ASSERTS THE ARM EVALUATED. Without that the `none` cell passes
// when the project stops parsing at all -- no refusal from 21 because nothing
// from 21 ran, which is the vacuity this suite refuses everywhere else.
func TestArm21ReadsItsOwnVerdict(t *testing.T) {
	twin := filepath.Join("..", "..", "examples", "twin")
	for _, c := range []struct {
		what, anchor, with string
		refused            bool
		says               string
	}{
		{
			what:    "a role holding a context the describe does not offer",
			anchor:  "[[action]]\nname = \"publish_doc\"",
			with:    "[[role]]\nname = \"author\"\ncontexts = [\"kiosk\"]\n\n[[action]]\nname = \"publish_doc\"",
			refused: true,
			says:    "role author holds kiosk",
		},
		{
			what:    "an action whose context is none",
			anchor:  "name = \"publish_doc\"\ncontext = \"api\"",
			with:    "name = \"publish_doc\"\ncontext = \"none\"",
			refused: false,
		},
	} {
		t.Run(c.what, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "twin")
			if out, err := exec.Command("cp", "-r", twin, dir).CombinedOutput(); err != nil {
				t.Fatalf("copying the twin: %v %s", err, out)
			}
			p := filepath.Join(dir, "bindings.toml")
			raw, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			if n := strings.Count(string(raw), c.anchor); n != 1 {
				t.Fatalf("the anchor occurs %d time(s) in the twin's bindings.toml, so this cell "+
					"edits nothing it means to: %q", n, c.anchor)
			}
			edited := strings.Replace(string(raw), c.anchor, c.with, 1)
			if err := os.WriteFile(p, []byte(edited), 0o644); err != nil {
				t.Fatal(err)
			}

			rep := validate.Run(dir)
			ran := false
			for _, a := range rep.Evaluated {
				if a == "21" {
					ran = true
				}
			}
			if !ran {
				t.Fatalf("arm 21 did not evaluate, so this cell measures nothing: skipped %v, "+
					"could not run %v", rep.Skipped, rep.CouldNotRun)
			}
			got := ""
			for _, r := range rep.Refusals {
				if r.Arm == "21" {
					got = r.Detail
				}
			}

			if !c.refused {
				if got != "" {
					t.Errorf("arm 21 refused a context it must not pair: %s", got)
				}
				t.Logf("PROOF %s is not paired against the describe", c.what)
				return
			}
			if got == "" {
				t.Fatalf("arm 21 did not refuse %s; the report carries %v", c.what, rep.Refusals)
			}
			if !strings.Contains(got, c.says) {
				t.Errorf("arm 21's refusal does not name the value: %q, want it to contain %q", got, c.says)
			}
			// AND WHAT THE ADAPTER OFFERS, because a refusal naming only the bad
			// value leaves a reader without the set it was compared against.
			if !strings.Contains(got, "api") {
				t.Errorf("arm 21's refusal does not name what the describe offers: %q", got)
			}
			t.Logf("PROOF %s -> %s", c.what, got)
		})
	}
}

// TestArm19Cell8PairsOnlyTheBrowserContext is cell 8's POSITIVE, and ratchet
// named the gap it fills (coop r8/155). The cell's NEGATIVE is the fixture
// 19/profile-venire-context-no-stage; widening `isBrowser` to accept ANY
// context SURVIVED the whole suite -- and survived UNREACHED, which is weaker
// than a reached survivor and ratchet said so rather than let it read the same.
// Exactly two projects in the tree carry `[[profiles.*.venire]]` rows and both
// are that cell's negatives, while every role in every project declares only
// `api`, `browser`, or both. So there was no third context anywhere to widen
// onto: the narrowing could be reversed and CI would stay green.
//
// MY OWN STATED REASON WAS THE SPECIFICATION FOR THIS TEST -- "a project may
// declare a `kiosk` context whose adapter serves it, and refusing that under
// every profile would refuse a correct project." Nothing asserted it. The
// reason was right and unheld, which is the third time in two heads; a stated
// reason with no cell is decoration, because the next person to widen the
// predicate gets a green suite.
//
// THE PAIR IS IN ONE TEST so the boundary is one thing a reader sees: the same
// profile, the same appended row, and only the ROLE'S CONTEXT moving. `api` is
// the non-browser side rather than `kiosk` because `kiosk` would also trip arm
// 21 -- the adapter does not describe it -- and a cell about arm 19 should not
// need a second arm's refusal read around it.
func TestArm19Cell8PairsOnlyTheBrowserContext(t *testing.T) {
	twin := filepath.Join("..", "..", "examples", "twin")
	for _, c := range []struct {
		context string
		refused bool
	}{
		// THE POSITIVE: a context no selected stage drives and that is not the
		// browser one. The profile selects `api`, the role holds `api`, and the
		// cell must say nothing.
		{"api", false},
		// AND THE NEGATIVE BESIDE IT, so a reader can see that the only thing
		// separating them is the context's name.
		{"browser", true},
	} {
		t.Run(c.context, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "twin")
			if out, err := exec.Command("cp", "-r", twin, dir).CombinedOutput(); err != nil {
				t.Fatalf("copying the twin: %v %s", err, out)
			}
			// `author` IS THE ROLE THE TWIN'S OWN `[[venire]]` DRAWS. Declaring
			// any role turns on arm 19's venire cell, so any other name leaves
			// the twin's committed row unresolved and trips that cell too.
			b := filepath.Join(dir, "bindings.toml")
			raw, err := os.ReadFile(b)
			if err != nil {
				t.Fatal(err)
			}
			role := "\n[[role]]\nname = \"author\"\ncontexts = [\"" + c.context + "\"]\n"
			if err := os.WriteFile(b, append(raw, []byte(role)...), 0o644); err != nil {
				t.Fatal(err)
			}
			// THE ROW GOES AFTER THE PROFILE'S OWN KEYS. An array-of-tables
			// header directly under the profile header takes `stages`, `seeds`
			// and `budget` into the row and the file stops parsing -- measured,
			// as a mutant that was not a kill.
			p := filepath.Join(dir, "platform.toml")
			raw, err = os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			prof := "\n[profiles.cell]\nstages = [\"api\"]\nseeds = 1\nbudget = \"1m\"\n" +
				"\n[[profiles.cell.venire]]\nrole = \"author\"\ncount = 1\n"
			if err := os.WriteFile(p, append(raw, []byte(prof)...), 0o644); err != nil {
				t.Fatal(err)
			}

			rep := validate.Run(dir)
			ran := false
			for _, a := range rep.Evaluated {
				if a == "19" {
					ran = true
				}
			}
			if !ran {
				t.Fatalf("arm 19 did not evaluate, so this cell measures nothing: skipped %v, "+
					"could not run %v", rep.Skipped, rep.CouldNotRun)
			}
			got := ""
			for _, r := range rep.Refusals {
				if r.Arm == "19" {
					got = r.Detail
				}
			}

			if !c.refused {
				if got != "" {
					t.Errorf("a role holding %q under a profile selecting api was refused by arm 19, "+
						"and only a BROWSER context is pairable against a profile's stages: %s",
						c.context, got)
				}
				t.Logf("PROOF a %q context under a profile that selects api is not paired", c.context)
				return
			}
			if got == "" {
				t.Fatalf("arm 19 did not refuse a %q context under a profile selecting api alone; "+
					"the report carries %v", c.context, rep.Refusals)
			}
			for _, want := range []string{"cell", "author", c.context, "api"} {
				if !strings.Contains(got, want) {
					t.Errorf("the refusal does not name %q: %s", want, got)
				}
			}
			t.Logf("PROOF %q -> %s", c.context, got)
		})
	}
}
