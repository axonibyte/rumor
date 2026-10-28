// R7.1's walk-level arms: a project that declares `[clock]` has the run's own
// time in its ledger, and a project that declares none is walked exactly as it
// was before R7 existed.
//
// WHY A SUITE OF ITS OWN, and the reason is tests/quiesce's own: the engine's
// clock facts had arms at the ledger tier since R2.3 and NO production path
// wrote one, so what the unit arms proved was that a type behaved, not that a
// run did. These drive internal/schedule's walk against the live toy through
// cmd/toy-adapter, which is the path `rumor run` takes.
//
// fable's D1 (coop breakout-r7/0) is the drill these are the suite half of:
// "with `[clock]` declared, a `clock` fact at every seal, `t` in nanoseconds
// from zero, present at step 0".
package clock_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/decl"
	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
	"forge.axonibyte.com/axonibyte/rumor/internal/ledger"
	"forge.axonibyte.com/axonibyte/rumor/internal/platform"
	"forge.axonibyte.com/axonibyte/rumor/internal/quiesce"
	"forge.axonibyte.com/axonibyte/rumor/internal/schedule"
	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

const project = "../../examples/toy"

// TestMain builds the toy and its adapter and puts them on PATH, for
// tests/quiesce's reason: toy-adapter spawns `toy` itself when TOY_ADDR is
// unset, so both must be findable by name. Every exit path cleans up, because
// a `defer` does not run through os.Exit (issue 88).
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "rumor-clock-bin")
	if err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: %v\n", err)
		os.Exit(1)
	}
	die := func(code int) {
		_ = os.RemoveAll(dir)
		os.Exit(code)
	}
	for _, pkg := range []struct{ out, path string }{
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

// clocked is examples/toy with ONE DECLARATION ADDED: a `[clock]` table. The
// corpus is never touched, which is fable's r6/82 ruling about the toy's
// baseline and the shape issue 64's validate fixtures already use.
//
// THE TABLE IS EMPTY ON PURPOSE. R7.1 reads the table's PRESENCE and nothing
// inside it; R7.2 types its contents. An empty table is therefore the smallest
// thing that declares clock control, and a fixture carrying keys would be
// asserting a shape this phase does not define.
//
// THE PREMISE IS ASSERTED, NOT ASSUMED, AND IT INVERTED AT R7.2: the committed
// toy now DECLARES `[clock]` (issue 101's first bullet, on fable's ruling at
// coop rumor/1315), so what these helpers assert is that the declaration is
// there to be rewritten or taken out. Before, they asserted its absence. An
// unasserted premise would have left this package comparing a project with
// itself the moment the toy changed, which is the failure the original
// assertion was put in for.
//
// `toyClock` IS THE LIVE DECLARATION, by its exact bytes. Matching the whole
// two-line table rather than the word `[clock]` is what makes the removal
// below removal and not a truncation: the file's comments stay, and a comment
// declares nothing.
const toyClock = "[clock]\nform = \"adapter\"\n"

func clocked(t *testing.T) string {
	t.Helper()
	// `faked` IS THE FORM EVERY ARM HERE DECLARED BEFORE R7.2 NAMED IT. The
	// engine's own `t` was the world's and no request was sent, so this
	// spelling leaves every assertion in this package about exactly the run
	// it was written against. The LIVE toy declares `adapter`, which the arm
	// below drives over the wire.
	return clockedAs(t, bindings.ClockFaked)
}

// clockedAs is the toy with its clock form rewritten.
func clockedAs(t *testing.T, form bindings.ClockForm) string {
	t.Helper()
	dir := t.TempDir()
	entries, err := os.ReadDir(project)
	if err != nil {
		t.Fatalf("reading %s: %v", project, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		raw, rerr := os.ReadFile(filepath.Join(project, e.Name()))
		if rerr != nil {
			t.Fatalf("reading %s: %v", e.Name(), rerr)
		}
		if e.Name() == "bindings.toml" {
			raw = []byte(reform(t, string(raw), form))
		}
		if werr := os.WriteFile(filepath.Join(dir, e.Name()), raw, 0o644); werr != nil {
			t.Fatalf("writing %s: %v", e.Name(), werr)
		}
	}
	return dir
}

// reform rewrites the live toy's clock form, and unclock takes the table out.
//
// BOTH ASSERT THEY CHANGED SOMETHING. A copy helper whose edit silently missed
// would hand every arm in this package the live toy under another name, and
// the arms would pass against it -- which is how a control becomes a second
// copy of the thing it contrasts with.
func reform(t *testing.T, raw string, form bindings.ClockForm) string {
	t.Helper()
	out := strings.Replace(raw, toyClock, fmt.Sprintf("[clock]\nform = %q\n", form), 1)
	if out == raw && form != bindings.ClockAdapter {
		t.Fatalf("the committed toy's bindings do not hold %q, so this copy's form was not "+
			"rewritten and the arm would be driving the live project", toyClock)
	}
	return out
}

func unclock(t *testing.T, raw string) string {
	t.Helper()
	out := strings.Replace(raw, toyClock, "", 1)
	if out == raw {
		t.Fatalf("the committed toy's bindings do not hold %q, so nothing was removed and the "+
			"unclocked control would be a clocked run", toyClock)
	}
	if strings.Contains(out, "\n[clock]") {
		t.Fatal("a `[clock]` table survives in the unclocked copy")
	}
	return out
}

// unclocked is the toy with no clock at all: the control for "declaring a
// clock adds facts and changes nothing else", which was the COMMITTED toy
// until R7.2 declared one in it.
func unclocked(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	entries, err := os.ReadDir(project)
	if err != nil {
		t.Fatalf("reading %s: %v", project, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		raw, rerr := os.ReadFile(filepath.Join(project, e.Name()))
		if rerr != nil {
			t.Fatalf("reading %s: %v", e.Name(), rerr)
		}
		if e.Name() == "bindings.toml" {
			raw = []byte(unclock(t, string(raw)))
		}
		if werr := os.WriteFile(filepath.Join(dir, e.Name()), raw, 0o644); werr != nil {
			t.Fatalf("writing %s: %v", e.Name(), werr)
		}
	}
	return dir
}

// walk runs one attempt of a seed the way cmd/rumor does: the project's own
// declarations, its eventual types, and clock control read off the bindings.
func walk(t *testing.T, dir string, seed uint64, steps int) schedule.Record {
	return walkProfile(t, dir, seed, steps, "")
}

func walkProfile(t *testing.T, dir string, seed uint64, steps int, profile string) schedule.Record {
	t.Helper()
	rec, err := tryWalkProfile(t, dir, seed, steps, profile)
	if err != nil {
		t.Fatalf("the walk could not run: %v", err)
	}
	return rec
}

// tryWalkProfile hands the walk's own error back instead of ending the arm,
// because one arm here HAS a refusal as its subject: a duration bound with no
// clock must still be refused by name, and an arm that could only read a
// successful walk could not see it.
func tryWalkProfile(t *testing.T, dir string, seed uint64, steps int, profile string) (schedule.Record, error) {
	t.Helper()
	ctx := context.Background()

	prep, err := platform.Load(filepath.Join(dir, "platform.toml"), decl.TOML)
	if err != nil || !prep.OK() {
		t.Fatalf("platform.toml: %v %v", err, prep.Refusals)
	}
	brep, err := bindings.Load(filepath.Join(dir, "bindings.toml"), decl.TOML)
	if err != nil || !brep.OK() {
		t.Fatalf("bindings.toml: %v %v", err, brep.Refusals)
	}
	statute, err := os.ReadFile(filepath.Join(dir, "statute.rego"))
	if err != nil {
		t.Fatalf("statute.rego: %v", err)
	}
	// A FRESH WORLD per arm, tests/quiesce's reason: the adapter spawns its own
	// toy when TOY_ADDR is unset, and a state file left by another arm would
	// make an observation disagree for a reason that is not this arm's.
	if err := os.Unsetenv("TOY_ADDR"); err != nil {
		t.Fatal(err)
	}
	sess, err := wire.Open(ctx, wire.Spawn{
		Command: prep.Platform.Adapter.Command,
		Cwd:     dir,
		Env:     prep.Platform.Adapter.Env,
	}, 0)
	if err != nil {
		t.Fatalf("opening the adapter: %v", err)
	}
	defer func() { _ = sess.Close() }()

	return schedule.Walk(ctx, prober(t, brep, sess, schedule.Project{
		Statute:      string(statute),
		Actions:      brep.Bindings.Actions,
		Weights:      prep.Platform.Weights,
		Venire:       prep.Platform.Venire,
		Roles:        brep.Bindings.Roles,
		Observations: brep.Bindings.Observations,
		Eventual:     schedule.EventualOf(brep.Bindings),
		ClockControl: brep.Bindings.Clock != nil,
		// AND HOW, which `rumor run` reads the same way (cmd/rumor/run.go).
		// R7.2 refuses a clocked project that names no form, so a harness
		// that dropped this would refuse every fixture in this package.
		ClockForm: brep.Bindings.ClockForm,
		Profile:   profile,
	}), sess, seed, steps, 1)
}

// prober wires the project's own quiesce probes, which is what cmd/rumor does
// and what this harness did NOT until R7.1's end-of-run refusal made the gap
// visible.
//
// WHY IT MATTERS MORE THAN IT LOOKS. Without a prober the end-of-run
// reconciliation never runs -- walk.go gates it on `p.Prober != nil` -- so
// EVERY pending a run holds survives to the end. A suite missing it reads a
// world the product does not have: seeds 66, 75 and 140 close their pendings
// at that reconciliation through `rumor run` and kept them open here, which
// turned a control into a second copy of the arm it was supposed to contrast
// with. Measured both ways at coop r7/16.
func prober(t *testing.T, brep bindings.Report, sess *wire.Session, p schedule.Project) schedule.Project {
	t.Helper()
	if len(brep.Bindings.Probes) == 0 {
		return p
	}
	pr, err := quiesce.New(brep.Bindings.Probes, sess)
	if err != nil {
		t.Fatalf("building the prober from the project's own probes: %v", err)
	}
	p.Prober = pr
	return p
}

// replayed drives schedule.Replay over a fresh world, the way internal/shrink
// drives a candidate: the same project, the same seed, the same attempt
// number. The attempt matters -- a second attempt is a second world with its
// own handle ids -- so a replay compared against a walk has to claim the same
// one or the comparison is between two different worlds.
func replayed(t *testing.T, dir string, seed uint64, steps int, profile string) (*engine.Finding, []schedule.Step, int, error) {
	t.Helper()
	ctx := context.Background()

	prep, err := platform.Load(filepath.Join(dir, "platform.toml"), decl.TOML)
	if err != nil || !prep.OK() {
		t.Fatalf("platform.toml: %v %v", err, prep.Refusals)
	}
	brep, err := bindings.Load(filepath.Join(dir, "bindings.toml"), decl.TOML)
	if err != nil || !brep.OK() {
		t.Fatalf("bindings.toml: %v %v", err, brep.Refusals)
	}
	statute, err := os.ReadFile(filepath.Join(dir, "statute.rego"))
	if err != nil {
		t.Fatalf("statute.rego: %v", err)
	}
	if err := os.Unsetenv("TOY_ADDR"); err != nil {
		t.Fatal(err)
	}
	sess, err := wire.Open(ctx, wire.Spawn{
		Command: prep.Platform.Adapter.Command,
		Cwd:     dir,
		Env:     prep.Platform.Adapter.Env,
	}, 0)
	if err != nil {
		t.Fatalf("opening the adapter: %v", err)
	}
	defer func() { _ = sess.Close() }()

	return schedule.Replay(ctx, prober(t, brep, sess, schedule.Project{
		Statute:      string(statute),
		Actions:      brep.Bindings.Actions,
		Weights:      prep.Platform.Weights,
		Venire:       prep.Platform.Venire,
		Roles:        brep.Bindings.Roles,
		Observations: brep.Bindings.Observations,
		Eventual:     schedule.EventualOf(brep.Bindings),
		ClockControl: brep.Bindings.Clock != nil,
		// AND HOW, which `rumor run` reads the same way (cmd/rumor/run.go).
		// R7.2 refuses a clocked project that names no form, so a harness
		// that dropped this would refuse every fixture in this package.
		ClockForm: brep.Bindings.ClockForm,
		Profile:   profile,
	}), sess, seed, 1, steps, nil)
}

func clockFacts(facts []ledger.Fact) []ledger.Fact {
	var out []ledger.Fact
	for _, f := range facts {
		if f.Kind == ledger.KindClock {
			out = append(out, f)
		}
	}
	return out
}

// TestAClockedRunHoldsTheRunsTimeAtEverySeal is fable's D1 on the walk: the
// fact is there, it is the run's, its time is the unit the contract declares,
// and it is present before the first action.
//
// TWO PER STEP IS THE MEASUREMENT AND NOT AN ESTIMATE: a step seals twice --
// once at its top, which is the prefix a precondition is evaluated against,
// and once before the observations that judge what the step did. A clock fact
// at "every seal" therefore means two, and pinning the number is what would
// catch a seal that stopped writing one: a bound asked about at an unclocked
// seal reads as unmeasurable, which is a refusal rather than a wrong answer,
// so it would not otherwise fail loudly here.
func TestAClockedRunHoldsTheRunsTimeAtEverySeal(t *testing.T) {
	const steps = 4
	rec := walk(t, clocked(t), 1, steps)

	if len(rec.Facts) == 0 {
		t.Fatal("the walk recorded no facts at all")
	}
	// PRESENT BEFORE THE FIRST ACTION, which is D1's "at step 0": the first
	// seal happens at the top of step 1, before an actor is drawn, so a
	// clocked run's ledger opens with the run's time.
	if first := rec.Facts[0]; first.Kind != ledger.KindClock || first.Position != 1 {
		t.Errorf("the run's first fact is a %q at position %d, want a clock fact at 1: a bound "+
			"asked about at step 1 needs a time at or before the claim it is about",
			first.Kind, first.Position)
	}

	cf := clockFacts(rec.Facts)
	if want := 2 * len(rec.Steps); len(cf) != want {
		t.Errorf("%d clock fact(s) over %d step(s), want %d: a step seals at its top and again "+
			"before its observations, and a seal with no clock fact is a position at which no "+
			"duration bound can be measured", len(cf), len(rec.Steps), want)
	}
	for i, f := range cf {
		if f.Actor != "run" {
			t.Errorf("clock fact %d is %q's: a clock fact belongs to no actor, and reconciliation "+
				"walks actors and never facts (vocabulary v1)", i+1, f.Actor)
		}
		if f.T == nil {
			t.Fatalf("clock fact %d carries no time", i+1)
		}
		if *f.T != 0 {
			t.Errorf("clock fact %d reads %d, want 0: the run's epoch is zero at the first seal "+
				"and nothing moves it until R7.3 draws `advance`, so a non-zero time here is a "+
				"clock reading something the run does not own", i+1, *f.T)
		}
	}
}

// THE `adapter` FORM, DRIVEN AGAINST THE TOY'S OWN CLOCK. The unit arms in
// internal/schedule drive the engine's checks against answers a real adapter
// does not give; this one drives the honest path all the way through the
// wire: `rumor` sets the toy's clock at every seal and writes the ledger fact
// from what the toy answered.
//
// THE COMPARISON IS THE POINT. The same seed under `faked` sends no request
// at all, so two runs agreeing fact for fact says the request neither moved
// the world nor disturbed the walk -- and the toy really did echo the ask,
// because a toy answering anything else is a refusal and this run is not one.
func TestTheAdapterFormSetsTheToysClockAndTheLedgerAgrees(t *testing.T) {
	const steps = 4
	viaAdapter := walk(t, clockedAs(t, bindings.ClockAdapter), 1, steps)
	faked := walk(t, clocked(t), 1, steps)

	ca, cf := clockFacts(viaAdapter.Facts), clockFacts(faked.Facts)
	if len(ca) == 0 {
		t.Fatal("no clock fact in a run whose project declares `form = \"adapter\"`")
	}
	if len(ca) != len(cf) {
		t.Fatalf("%d clock fact(s) through the adapter and %d under faked time: the form changed "+
			"how many times the run holds its time", len(ca), len(cf))
	}
	for i := range ca {
		if ca[i].T == nil || cf[i].T == nil {
			t.Fatalf("clock fact %d carries no time", i+1)
		}
		if *ca[i].T != *cf[i].T {
			t.Errorf("clock fact %d reads %d through the adapter and %d under faked time",
				i+1, *ca[i].T, *cf[i].T)
		}
		if *ca[i].T != 0 {
			t.Errorf("clock fact %d reads %d, want 0: the toy's clock is set to the run's, and "+
				"nothing moves the run's until R7.3 draws `advance`", i+1, *ca[i].T)
		}
	}
	if len(viaAdapter.Steps) != len(faked.Steps) {
		t.Errorf("%d step(s) through the adapter and %d under faked time", len(viaAdapter.Steps), len(faked.Steps))
	}
}

// TestDeclaringAClockAddsFactsAndChANGESNothingElse is the half that makes the
// clock safe to add: the same seed over the same steps draws the same actions
// and records the same claims and tells, with the clock facts interleaved.
//
// POSITIONS DO SHIFT, and that is why the comparison is over the SEQUENCE of
// non-clock facts rather than their positions: a clock fact occupies a
// position like any other fact. What must not move is what HAPPENED.
func TestDeclaringAClockAddsFactsAndChangesNothingElse(t *testing.T) {
	const steps = 4
	withClock := walk(t, clocked(t), 1, steps)
	without := walk(t, unclocked(t), 1, steps)

	if n := len(clockFacts(without.Facts)); n != 0 {
		t.Fatalf("%d clock fact(s) in a project declaring no `[clock]`, want 0: without the "+
			"declaration the ledger refuses one, so this is either the guard gone or the "+
			"fixture clocked", n)
	}

	// AND THE OTHER HALF OF THE NAME, which went unchecked until rook mutated
	// the product during the D1/D5 drive (coop rumor/1342): with `if false` on
	// the seal's clock Append, 8 of this package's 10 tests die and THIS ONE
	// PASSED -- because `shape` below SKIPS clock facts, so a run with none
	// compares equal to a run with none, and the `without` assertion above is
	// still true of a build that writes no clock fact at all. "AddsFacts" was
	// the half nothing asserted.
	if n := len(clockFacts(withClock.Facts)); n == 0 {
		t.Fatalf("no clock fact in a project that declares `[clock]`, and the rest of this test " +
			"cannot see that: it compares only NON-clock shapes and the step counts, both of " +
			"which a build writing no clock fact satisfies")
	}

	shape := func(facts []ledger.Fact) []string {
		var out []string
		for _, f := range facts {
			if f.Kind == ledger.KindClock {
				continue
			}
			out = append(out, fmt.Sprintf("%s/%s/%s", f.Kind, f.Actor, f.Ref))
		}
		return out
	}
	a, b := shape(withClock.Facts), shape(without.Facts)
	if len(a) != len(b) {
		t.Fatalf("%d non-clock fact(s) with a clock and %d without: declaring a clock changed "+
			"what the walk did\n%v\n%v", len(a), len(b), a, b)
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("non-clock fact %d is %q with a clock and %q without: the clock is supposed "+
				"to be written beside the walk and not to steer it", i+1, a[i], b[i])
		}
	}
	if len(withClock.Steps) != len(without.Steps) {
		t.Errorf("%d step(s) with a clock and %d without", len(withClock.Steps), len(without.Steps))
	}
}
