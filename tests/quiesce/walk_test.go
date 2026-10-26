// R6.3's walk-level arms: the bound a project declares has an effect on a WALK,
// which is what fable's ruling 3 (coop r6/78) asks for by name -- "the walk
// judges with the bindings and the prober's answer, next commit, with an arm
// that drives a lagging count through a WALK and reads `pending` then
// `consistent` after the drain".
//
// WHY A SUITE OF ITS OWN. The overlay's arms in R6.1 and R6.2 drove
// engine.Judge on fixtures, and quill measured at coop r6/80 that NO production
// path called Judge at all: "a shutter in front of a room nobody could enter".
// An arm here drives internal/schedule's own walk against the live toy through
// cmd/toy-adapter, so what it reads is what `rumor run` produces. tests/shrink
// drives the same walk and could host these, but its subject is the reduction
// and a walk-judgment arm there would be found by nobody looking for it.
package quiesce_test

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
	"forge.axonibyte.com/axonibyte/rumor/internal/platform"
	"forge.axonibyte.com/axonibyte/rumor/internal/quiesce"
	"forge.axonibyte.com/axonibyte/rumor/internal/schedule"
	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

const project = "../../examples/toy"

// TestMain builds the toy and its adapter and puts them on PATH. toy-adapter
// spawns `toy` itself when TOY_ADDR is unset, so both must be findable by name.
//
// EVERY EXIT PATH CLEANS UP, for the reason tests/shrink and tests/run both
// record: a deferred call does not run when the goroutine leaves through
// os.Exit, so a `defer` here would leave the built binaries in /tmp on every
// run (issue 88).
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "rumor-quiesce-bin")
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

// derived is examples/toy with ONE DECLARATION CHANGED: the count's claim type
// goes eventual, bounded on quiesce. It is written to a temp dir per arm and the
// committed corpus is never touched.
//
// WHY A DERIVED PROJECT AND NOT THE CORPUS ITSELF. fable ruled at coop r6/82
// that examples/toy's `event.joined` must stay immediate: it is the claim R5
// planted its rows in, and a bound over it turns seed 40's R5 finding into an
// R6 one -- the child losing a row rather than finding it late. So the corpus
// keeps the baseline and this arm declares the bound it needs, which is the
// shape issue 64's eighteen validate fixtures already use: each a copy of the
// twin with one change, so what is under test is the change.
//
// THE CHANGE IS ASSERTED, NOT ASSUMED. A copy whose edit silently missed would
// drive the unbounded project and read `consistent` everywhere, which is a
// green arm over nothing -- the fixture-builder failure my own notes name.
func derived(t *testing.T) string {
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
			before := string(raw)
			after := strings.Replace(before, `[claim."event.joined"]`+"\ntiming = \"immediate\"",
				`[claim."event.joined"]`+"\ntiming = \"eventual\"\nbound = \"quiesce\"", 1)
			if after == before {
				t.Fatalf("the one change this fixture is did not apply: %s no longer declares "+
					"`event.joined` as immediate, so the copy would drive an unbounded project "+
					"and every observation would read consistent", filepath.Join(project, e.Name()))
			}
			raw = []byte(after)
		}
		if werr := os.WriteFile(filepath.Join(dir, e.Name()), raw, 0o644); werr != nil {
			t.Fatalf("writing %s: %v", e.Name(), werr)
		}
	}
	return dir
}

// walk runs one attempt of a seed against a fresh toy, the way cmd/rumor does:
// the project's own declarations, its eventual types, and a prober built from
// its own probes.
func walk(t *testing.T, seed uint64, steps int) schedule.Record {
	t.Helper()
	ctx := context.Background()
	project := derived(t)

	prep, err := platform.Load(filepath.Join(project, "platform.toml"), decl.TOML)
	if err != nil || !prep.OK() {
		t.Fatalf("platform.toml: %v %v", err, prep.Refusals)
	}
	brep, err := bindings.Load(filepath.Join(project, "bindings.toml"), decl.TOML)
	if err != nil || !brep.OK() {
		t.Fatalf("bindings.toml: %v %v", err, brep.Refusals)
	}
	statute, err := os.ReadFile(filepath.Join(project, "statute.rego"))
	if err != nil {
		t.Fatalf("statute.rego: %v", err)
	}
	// A FRESH WORLD, so the counts a walk observes are its own: the adapter
	// spawns its own toy when TOY_ADDR is unset, and a state file left by
	// another arm would make the first observation disagree for a reason that
	// is not the lag.
	if err := os.Unsetenv("TOY_ADDR"); err != nil {
		t.Fatal(err)
	}
	sess, err := wire.Open(ctx, wire.Spawn{
		Command: prep.Platform.Adapter.Command,
		Cwd:     project,
		Env:     prep.Platform.Adapter.Env,
	}, 0)
	if err != nil {
		t.Fatalf("opening the adapter: %v", err)
	}
	defer func() { _ = sess.Close() }()

	p := schedule.Project{
		Statute:      string(statute),
		Actions:      brep.Bindings.Actions,
		Weights:      prep.Platform.Weights,
		Venire:       prep.Platform.Venire,
		Roles:        brep.Bindings.Roles,
		Observations: brep.Bindings.Observations,
		Eventual:     schedule.EventualOf(brep.Bindings),
	}
	if len(brep.Bindings.Probes) > 0 {
		pr, qerr := quiesce.New(brep.Bindings.Probes, sess)
		if qerr != nil {
			t.Fatalf("building the prober: %v", qerr)
		}
		p.Prober = pr
	}
	rec, werr := schedule.Walk(ctx, p, sess, seed, steps, 1)
	if werr != nil {
		t.Fatalf("the walk could not run: %v", werr)
	}
	return rec
}

// TestABoundedWalkIsJudgedWithTheBindingsAndLeavesNoPendingOpen drives a walk
// over a project that DECLARES an eventual type bounded on quiesce, with a
// prober built from its own probes, and asserts the two things that are true of
// it today: every observation is judged, and the run ends with nothing held
// open.
//
// IT IS THE FIRST ARM IN THE PROJECT ON THIS PATH AT ALL. Every `pending` arm
// before it drove engine.Judge on a fixture, and quill measured that no
// production path called Judge (coop r6/80): the walk used engine.Evaluate,
// which takes no bindings, so a declared bound had no effect on the one path
// the product runs. A walk that refused or crashed with the bindings wired
// fails here.
//
// WHAT IT DOES NOT YET DRIVE, SAID PLAINLY RATHER THAN LEFT AS A GAP. fable's
// ruling 3 (coop r6/78) asks for an arm reading `pending` then `consistent`
// after the drain, and that needs an observation of a value that LAGS. After
// fable's ruling at /82 the toy's lagging number is the worker's own tally, and
// an observation of it needs a claim type of its own: `event.joined` must stay
// immediate because R5's rows are planted in it. A claim type reaches the
// ledger only as an action's single emitted type -- internal/schedule refuses
// an action declaring two, "which one a claim's handle follows is not this
// package's to choose" -- so the tally needs an action of its own, and adding
// one to the pool moves the draw for every seed, which the epic's done-when
// forbids. Measured and raised at coop r6; this arm gains the sequence when
// that is ruled, and asserts PendingOpen meanwhile so the half that IS true
// cannot rot.
func TestABoundedWalkIsJudgedWithTheBindingsAndLeavesNoPendingOpen(t *testing.T) {
	rec := walk(t, 3, 10)

	// EVERY OBSERVATION REACHED A VERDICT. A walk that judged nothing would
	// leave every Verdicts empty and pass any assertion about pendings, which
	// is the vacuous shape this checks first.
	judged := 0
	for _, st := range rec.Steps {
		judged += len(st.Verdicts)
		for _, v := range st.Verdicts {
			if v == "" {
				t.Errorf("step %d carries an empty verdict, so an observation was recorded "+
					"without being judged:\n%s", st.N, render(rec))
			}
		}
	}
	if judged == 0 {
		t.Fatalf("the walk judged no observation at all, so nothing here is about the "+
			"bindings:\n%s", render(rec))
	}

	// AND NOTHING IS HELD OPEN AT THE END, which is docs/vocabulary.md's rule
	// in its own words -- "a run that ends on one leaves no pending open" --
	// and part of fable's control for 91 (coop r6/78 ruling 2). It was
	// unmeasurable until Record.PendingOpen existed: a `pending` stops no walk
	// and the record kept only the finding that ended one.
	if rec.PendingOpen != 0 {
		t.Errorf("the run ended with %d finding(s) still held open for re-check: a pending is "+
			"re-checked at the actor's next observation of the same query, and nothing "+
			"reconciles after the last step\n%s", rec.PendingOpen, render(rec))
	}
}

func render(rec schedule.Record) string {
	var b strings.Builder
	for _, s := range rec.Steps {
		fmt.Fprintf(&b, "    %2d %-14s %v\n", s.N, s.Kind, s.Verdicts)
	}
	fmt.Fprintf(&b, "    PendingOpen=%d at the end\n", rec.PendingOpen)
	return b.String()
}
