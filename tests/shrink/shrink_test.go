// R5.3's done-when, third item: the toy's bugs are FOUND by a walk and SHRUNK,
// each to under five actions, every candidate on a verified clean world.
//
// IT IS AN INTEGRATION SUITE AND NOT internal/shrink's, because the thing under
// test is the composition: internal/shrink's own arms drive the six steps
// against a fake provider and a replay closure, which is right for the steps
// and cannot show that a walk's finding reaches the shrinker at all. Here the
// adapter is cmd/toy-adapter, the world is cmd/toy restarted through the
// shipped rollback provider, and the trace comes from internal/schedule's own
// walk -- so a break anywhere along that path fails here and nowhere else.
//
// WHY THIS WAITED. A walk could not produce a finding until R5.5 (issue 83):
// the engine could act on a real application but never ask it anything, so
// there was nothing for a shrinker to shrink. docs/design.md also recorded the
// third as waiting on the walk putting consumed handles on a claim, which
// landed in R5.4 -- that note was stale and is corrected in the same change as
// this file.
package shrink_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/decl"
	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
	"forge.axonibyte.com/axonibyte/rumor/internal/platform"
	"forge.axonibyte.com/axonibyte/rumor/internal/quiesce"
	"forge.axonibyte.com/axonibyte/rumor/internal/rollback"
	"forge.axonibyte.com/axonibyte/rumor/internal/rollback/toyrestart"
	"forge.axonibyte.com/axonibyte/rumor/internal/schedule"
	"forge.axonibyte.com/axonibyte/rumor/internal/shrink"
	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

const project = "../../examples/toy"

// TestMain builds the three binaries this suite drives and puts them on PATH.
// toy-adapter spawns `toy` itself when TOY_ADDR is unset, and the rollback
// provider starts its own, so both have to be findable by name.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "rumor-shrink-bin")
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

// load reads the project the way `rumor run` reads it. It is duplicated rather
// than shared because there is no loader to share yet: the verb composes these
// three reads inline, and a helper invented here would be a second definition
// of what a project is.
// IT RETURNS THE PROBES BESIDE THE PROJECT because the Prober needs the
// adapter session, which is opened per walk: the declarations are read here and
// wired there, which is the same two-step cmd/rumor/run.go performs.
func load(t *testing.T) (schedule.Project, []bindings.Probe) {
	t.Helper()
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
	return schedule.Project{
		Statute:      string(statute),
		Actions:      brep.Bindings.Actions,
		Weights:      prep.Platform.Weights,
		Venire:       prep.Platform.Venire,
		Roles:        brep.Bindings.Roles,
		Observations: brep.Bindings.Observations,
		// THE EVENTUAL TYPES, BECAUSE THE PRODUCT PASSES THEM. This harness
		// built its Project without them and drove a walk cmd/rumor never
		// runs: with no Eventual the overlay cannot engage, so every cell here
		// measured a toy whose declared quiesce bound had no effect. That is
		// fable's ruling 3 (coop r6/78) one layer out, and ratchet's sixth
		// check at /79 in its own words -- ask whether the live path reaches
		// the code an arm drives.
		//
		// THE PROBER IS NOT SET HERE because it needs the adapter session,
		// which is opened per walk below; the caller wires it.
		Eventual: schedule.EventualOf(brep.Bindings),
	}, brep.Bindings.Probes
}

// kindsDrawn is the action kinds the trace actually used, which is what kind
// removal is offered. It is read off the record rather than taken from the
// project so the reduction is over what the failure CONTAINS: offering a kind
// the trace never drew would make the removal pass report a removal that
// removed nothing.
func kindsDrawn(steps []schedule.Step) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range steps {
		if s.Kind == "" || seen[s.Kind] {
			continue
		}
		seen[s.Kind] = true
		out = append(out, s.Kind)
	}
	return out
}

// THE DONE-WHEN: a walk FINDS a toy bug and the shrinker reduces it to under
// five actions, every candidate on a verified clean world.
//
// TWO ROWS, AND THE ROW IS READ OFF THE TRACE RATHER THAN THE FINDING. Both of
// the toy's R5-catchable rows arrive as `default finding := "bug"` with no
// clause fired, so they carry the same value, the same rule and no attribution
// difference -- there is no field of engine.Finding that separates them
// (ratchet carried this for the re-cut, coop r5/178). What separates them is
// the WORLD: once an `event.deleted` claim is in the sealed prefix the statute
// can reach neither `consistent` nor `explained`, so a trace containing a
// delete is the stale count and one without it is the other row. Asserting the
// delete's presence per cell is what keeps these two cells from being one bug
// measured twice.
//
// THE PROVIDER OWNS THE APPLICATION AND THE ADAPTER FOLLOWS IT. toyrestart
// starts cmd/toy, holds the handle and hands out the address it bound; the
// adapter drives that one through TOY_ADDR instead of starting a toy of its
// own. Two things about that had never been driven before this arm, and both
// were wrong:
//
//   - examples/toy declared no `[adapter].env`, so the NAME was not passed
//     through and the value could not reach the adapter at all. Fixed in the
//     project, where the declaration belongs.
//   - the address is NOT STABLE across a restore. Restore stops the toy and
//     starts it again on `127.0.0.1:0`, so every candidate world binds a fresh
//     ephemeral port -- deliberately, so two worlds on one box cannot collide.
//     An adapter spawned once would therefore be talking to a dead port from
//     the second candidate onward, and a shrink would restore one world while
//     asking another. So the DRIVER re-opens the adapter against the provider's
//     current address for every candidate, which is what this closure does.
func TestTheToyBugsAreFoundByAWalkAndShrinkToUnderFiveActions(t *testing.T) {
	const under = 5

	// THE SEEDS ARE CHOSEN FOR LENGTH AND THE CHOICE IS THE ARM'S SUBJECT.
	// Length bisection takes PREFIXES, so the shortest trace a reduction can
	// reach is the step at which the bug first appears; kind removal then takes
	// whole kinds out, and neither strategy can delete a step from the middle.
	// Seed 8 reaches the same row at step 6 and therefore shrinks 6 -> 6, which
	// is not a defect in the reduction but a seed whose bug is simply not
	// reachable sooner. Measured over seeds 1..60: eleven reach a bug by step
	// 6, and these two are the shortest of each row.
	for _, tc := range []struct {
		seed    uint64
		row     string
		deletes bool
	}{
		// THE LABELS SAY WHAT IS PROVEN. "the stale count" is identified by
		// the delete; seed 40's label says the absence and names the
		// interpretation as D12's rather than as something measured here.
		{2, "a bug reached WITH a delete, which is the stale count", true},
		{40, "a bug reached with NO delete, which D12 enumerates as the leak", false},
	} {
		t.Run(fmt.Sprintf("seed%d", tc.seed), func(t *testing.T) {
			ctx := context.Background()
			p, declared := load(t)

			prov, err := rollback.Open(toyrestart.Name, rollback.Options{Dir: project})
			if err != nil {
				t.Fatalf("opening %s: %v", toyrestart.Name, err)
			}
			// REMOVE THE WORLD, NOT ONLY THE PROCESS. `Close()` stops the
			// toy and leaves its directory by contract, so this arm used to
			// leave one toy-world-* per seed per run (issue 88). RemoveWorld
			// stops the toy as well, so it replaces the Close rather than
			// following it; the Close fallback stays for any provider that
			// has not grown the method.
			if r, ok := prov.(interface{ RemoveWorld() }); ok {
				defer r.RemoveWorld()
			} else if c, ok := prov.(interface{ Close() }); ok {
				defer c.Close()
			}
			at, ok := prov.(interface{ Addr() string })
			if !ok {
				t.Fatalf("%s does not expose the address it bound, so the adapter cannot be "+
					"pointed at the world the provider owns", toyrestart.Name)
			}

			// THE ADAPTER IS POINTED AT THE PROVIDER'S TOY, re-read every time
			// because a restore rebinds the port.
			sessionNow := func() (*wire.Session, error) {
				if err := os.Setenv("TOY_ADDR", at.Addr()); err != nil {
					return nil, err
				}
				prep, perr := platform.Load(filepath.Join(project, "platform.toml"), decl.TOML)
				if perr != nil {
					return nil, perr
				}
				return wire.Open(ctx, wire.Spawn{
					Command: prep.Platform.Adapter.Command,
					Cwd:     project,
					Env:     prep.Platform.Adapter.Env,
				}, 0)
			}

			cp, err := prov.Checkpoint(ctx)
			if err != nil {
				t.Fatalf("checkpointing the clean world: %v", err)
			}

			sess, err := sessionNow()
			if err != nil {
				t.Fatalf("opening the adapter on the provider's world: %v", err)
			}
			// THE PROBER, for Eventual's reason: a project declaring probes and
			// walked without one has its bound judged QuiesceAbsent, which
			// engine.Judge refuses rather than reading as "not quiet". The
			// product builds it from the same declarations (cmd/rumor/run.go).
			if len(declared) > 0 {
				pr, qerr := quiesce.New(declared, sess)
				if qerr != nil {
					t.Fatalf("building the prober the project declares: %v", qerr)
				}
				p.Prober = pr
			}
			rec, werr := schedule.Walk(ctx, p, sess, tc.seed, 10, 1)
			_ = sess.Close()
			if werr != nil {
				t.Fatalf("the walk could not run: %v", werr)
			}
			if rec.Finding == nil {
				t.Fatalf("seed %d walked %d step(s) and found no bug, so there is nothing to "+
					"shrink and this cell cannot speak for %s",
					tc.seed, len(rec.Steps), tc.row)
			}

			deleted := false
			for _, s := range rec.Steps {
				if s.Kind == "delete_event" {
					deleted = true
				}
			}
			// WHAT THIS ASSERTION PROVES AND WHAT IT DOES NOT, said here because
			// the `row` strings above read as more than it establishes (fable,
			// coop r5/219 (A), on ratchet's observation at /216).
			//
			// IT PROVES: the delete's presence or absence, and therefore that
			// one of these two bugs was reached WITHOUT one. A delete is
			// sufficient for a bug -- the statute can reach neither
			// `consistent` nor `explained` once `event.deleted` is in the
			// prefix -- so its absence rules the stale count out for seed 40
			// and its presence identifies the row for seed 2.
			//
			// IT DOES NOT PROVE WHICH ROW SEED 40 IS. Calling it the visibility
			// leak rests on D12 as amended enumerating exactly two
			// R5-catchable rows and putting the capacity race in R8 -- not on
			// anything the trace or the finding carries, since both rows arrive
			// as `default finding := "bug"` with no clause fired and no field
			// of engine.Finding separates them. If a fourth catchable row is
			// ever planted, this cell would relabel in silence: it would still
			// be a bug with no delete, and the name in `row` would be wrong
			// with nothing here to notice. The mechanism is given for the
			// delete direction only.
			if deleted != tc.deletes {
				t.Fatalf("seed %d's trace carries delete_event = %v, want %v -- this seed is "+
					"not %s, so the two cells may be one row measured twice",
					tc.seed, deleted, tc.deletes, tc.row)
			}

			probes := 0
			got, serr := shrink.Shrink(ctx, shrink.Request{
				Seed:       tc.seed,
				Steps:      rec.Steps,
				Kinds:      kindsDrawn(rec.Steps),
				Want:       *rec.Finding,
				Provider:   prov,
				Checkpoint: cp,
				SeedsFile:  filepath.Join(t.TempDir(), "seeds.lock"),
				Replay: func(c context.Context, steps int, without []string) (engine.Finding, []schedule.Step, error) {
					probes++
					s, oerr := sessionNow()
					if oerr != nil {
						return engine.Finding{}, nil, fmt.Errorf("re-opening the adapter on the "+
							"restored world: %w", oerr)
					}
					defer func() { _ = s.Close() }()
					// THE PROBER IS REBUILT ON THIS SESSION, and that is not
					// bookkeeping: a Prober holds the session it asks through,
					// so the one the WALK used is closed by the time a
					// candidate replays. Carrying it over gave "write |1: file
					// already closed" from the first probe read of the first
					// replay -- a dead instrument reported as a failure to
					// reproduce, which would have made every candidate look
					// too small.
					rp := p
					if len(declared) > 0 {
						pr, qerr := quiesce.New(declared, s)
						if qerr != nil {
							return engine.Finding{}, nil, fmt.Errorf("building the prober for "+
								"the replay: %w", qerr)
						}
						rp.Prober = pr
					}
					f, ran, rerr := schedule.Replay(c, rp, s, tc.seed, 1, steps, without)
					if rerr != nil {
						return engine.Finding{}, nil, rerr
					}
					if f == nil {
						// NO FINDING IS NOT AN ERROR: it is the candidate
						// failing to reproduce, which is what the reduction
						// reads to decide the candidate is too small.
						return engine.Finding{}, ran, nil
					}
					return *f, ran, nil
				},
			})
			if serr != nil {
				t.Fatalf("Shrink: %v", serr)
			}
			if !got.Stable {
				t.Fatalf("the gate did not reproduce seed %d's finding on a restored world, so "+
					"the failure is unstable rather than shrinkable: %s", tc.seed, got.Why)
			}
			if n := len(got.Trace.Steps); n >= under {
				t.Errorf("shrank to %d action(s), and the done-when is under %d\n%s",
					n, under, got.Render())
			}
			// WHAT THE REDUCTION ACTUALLY REDUCED, SAID OUT LOUD: nothing, for
			// either seed, and that is not a defect. Measured 2026-10-24 --
			// seed 2 bisects 4 -> 4 and seed 40 3 -> 3, with no kind removed in
			// either. The cause is R5.5 meeting R5.3: "the first bug ends the
			// walk", so a walked trace ALREADY stops at the step that produced
			// the finding. No shorter prefix can contain that step, and every
			// kind in so short a trace is one the bug needs -- so both
			// strategies correctly keep everything, and the shrinker CONFIRMS
			// minimality rather than achieving it.
			//
			// THE DONE-WHEN IS THEREFORE SATISFIED WITHOUT A REDUCTION, which a
			// reader of its words ("shrinks to under five actions") would not
			// expect, and which the PROOF line below records so nobody has to
			// take it from prose. internal/shrink's own arms drive traces that
			// DO reduce -- twelve steps to four -- so the strategies are
			// exercised; what no walk can now hand them is a trace that
			// continued past its own first bug. Raised for fable at coop r5.
			if got.Probes < 2 {
				t.Errorf("%d probe(s): the gate alone is one, so a reduction that rebuilt no "+
					"other world did not reduce anything", got.Probes)
			}
			if !got.Promoted {
				t.Error("the shrunk seed was not promoted, so the regression suite would not " +
					"carry the bug this run found")
			}
			t.Logf("PROOF seed %d: %d step(s) walked -> bisected to %d, kinds removed %v, "+
				"trace %d action(s), %d probe(s), %d replay call(s)\n%s",
				tc.seed, len(rec.Steps), got.Reduction.Steps, got.Kinds.Removed,
				len(got.Trace.Steps), got.Probes, probes, got.Render())
		})
	}
}
