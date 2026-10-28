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
	"forge.axonibyte.com/axonibyte/rumor/internal/ledger"
	"forge.axonibyte.com/axonibyte/rumor/internal/platform"
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
// THE PREMISE IS ASSERTED, NOT ASSUMED: the committed toy must NOT already
// declare a clock, or the control arm below is comparing a project with itself.
func clocked(t *testing.T) string {
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
			if strings.Contains(string(raw), "[clock]") {
				t.Fatalf("%s already declares `[clock]`, so the unclocked control arm would be "+
					"driving a clocked project", filepath.Join(project, e.Name()))
			}
			raw = append(raw, []byte("\n[clock]\n")...)
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

	rec, werr := schedule.Walk(ctx, schedule.Project{
		Statute:      string(statute),
		Actions:      brep.Bindings.Actions,
		Weights:      prep.Platform.Weights,
		Venire:       prep.Platform.Venire,
		Roles:        brep.Bindings.Roles,
		Observations: brep.Bindings.Observations,
		Eventual:     schedule.EventualOf(brep.Bindings),
		ClockControl: brep.Bindings.Clock != nil,
	}, sess, seed, steps, 1)
	if werr != nil {
		t.Fatalf("the walk could not run: %v", werr)
	}
	return rec
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
	without := walk(t, project, 1, steps)

	if n := len(clockFacts(without.Facts)); n != 0 {
		t.Fatalf("%d clock fact(s) in a project declaring no `[clock]`, want 0: without the "+
			"declaration the ledger refuses one, so this is either the guard gone or the "+
			"fixture clocked", n)
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
