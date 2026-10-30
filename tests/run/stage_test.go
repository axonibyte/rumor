package run_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/concurrency"
	"forge.axonibyte.com/axonibyte/rumor/internal/decl"
	"forge.axonibyte.com/axonibyte/rumor/internal/platform"
	"forge.axonibyte.com/axonibyte/rumor/internal/rollback"
	"forge.axonibyte.com/axonibyte/rumor/internal/rollback/toyrestart"
	"forge.axonibyte.com/axonibyte/rumor/internal/schedule"
	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

// THE STAGE ITSELF, OVER THE SHIPPED TOY: it derives its own setup, builds its
// own world, fans out on the action the project declares as contended, reads
// the held quantity back and reports the overfill.
//
// WHAT THIS ARM IS FOR THAT THE MECHANISM ARM IS NOT. The arm beside this one
// drives Fan, Contention and the plan directly, with the world built by hand --
// the creator, the two contenders, the event at capacity 1, the join requests.
// Everything it hands the fan-out is something I wrote in the test. This one
// hands the stage a PROJECT and nothing else: which action to contend, what its
// limit is, which query reads the world, and which actions must run first are
// all read out of examples/toy. So it is the arm that fails if the derivation
// is wrong while every hand-built one stays green -- and a stage whose subject,
// limit and setup come from a test is a mechanism with a test, not a stage.
//
// THE SWEEP IS ASSERTED AND NOT ONLY THE FINDING, and the toy makes it matter
// twice over: `create_event.visible_to` draws from three subsets and only the
// third can be contended. A would-be joiner is a GUEST relative to the event --
// cmd/toy's roleOf answers `guest` for anyone who is neither its admin nor
// already an attendee -- so the two subsets that omit `guest` answer every join
// 404 before the capacity check is reached. Those tries must be REPORTED as not
// contended and the stage must go on: an arm that checked only the finding
// would pass over a sweep that silently skipped a world, and would pass
// equally over one that never needed to sweep at all.
func TestTheStageContendsTheToysDeclaredSubject(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join("..", "..", "examples", "toy")

	prep, err := platform.Load(filepath.Join(dir, "platform.toml"), decl.TOML)
	if err != nil || !prep.OK() {
		t.Fatalf("platform: %v %v", err, prep.Refusals)
	}
	brep, err := bindings.Load(filepath.Join(dir, "bindings.toml"), decl.TOML)
	if err != nil || !brep.OK() {
		t.Fatalf("bindings: %v %v", err, brep.Refusals)
	}
	actors, err := schedule.Actors(prep.Platform.Venire)
	if err != nil {
		t.Fatalf("Actors: %v", err)
	}

	// ONE WORLD, OWNED BY THE PROVIDER, which is what makes the fan-out a
	// contention: an adapter spawned without TOY_ADDR starts a toy of its own,
	// and N of those would contend on nothing.
	prov, err := rollback.Open(toyrestart.Name, rollback.Options{Dir: dir})
	if err != nil {
		t.Fatalf("opening %s: %v", toyrestart.Name, err)
	}
	if r, ok := prov.(interface{ RemoveWorld() }); ok {
		defer r.RemoveWorld()
	} else if c, ok := prov.(interface{ Close() }); ok {
		defer c.Close()
	}
	at, ok := prov.(interface{ Addr() string })
	if !ok {
		t.Fatalf("%s does not expose the address it bound", toyrestart.Name)
	}
	t.Setenv("TOY_ADDR", at.Addr())

	open := func() (*wire.Session, error) {
		s, oerr := wire.Open(ctx, wire.Spawn{
			Command: prep.Platform.Adapter.Command,
			Cwd:     dir,
			Env:     prep.Platform.Adapter.Env,
		}, 30*time.Second)
		if oerr != nil {
			return nil, oerr
		}
		t.Cleanup(func() { _ = s.Close() })
		return s, nil
	}
	host, err := open()
	if err != nil {
		t.Fatalf("opening the host: %v", err)
	}

	// SEED 0, BECAUSE ITS PLAN IS A REAL PERMUTATION. Six of twelve seeds draw
	// the identity order at width 2, and a fixture whose plan is the identity
	// cannot tell a dispatch that follows the plan from one that ignores it --
	// the mutant that survived at r8/40.
	rep, err := concurrency.Run(brep.Bindings, host, open, actors, 0)
	if err != nil {
		t.Fatalf("Run over examples/toy: %v\n%s", err, rep.Render())
	}
	if len(rep.Verdicts) != 1 {
		t.Fatalf("%d verdict(s); the toy declares one contended action", len(rep.Verdicts))
	}
	v := rep.Verdicts[0]
	if v.Subject != "join_event" {
		t.Errorf("the stage contended %q; the toy declares join_event", v.Subject)
	}
	// THE SETUP IS DERIVED, AND THE BOOTSTRAP IS IN IT. Without create_user the
	// first request would be refused for want of a session's user, which is the
	// half the handle graph cannot see.
	if got := strings.Join(v.Setup, ", "); !strings.Contains(got, "create_user (per actor)") ||
		!strings.Contains(got, "create_event") {
		t.Errorf("the derived setup is %q; it needs the per-actor bootstrap and the action the "+
			"limit is read from", got)
	}
	// THE SWEEP IS LOAD-BEARING: on this project the first two worlds cannot be.
	if len(v.Tries) < 2 {
		t.Fatalf("%d try(s): a contender is a guest relative to an event it has not joined, so "+
			"the toy's `visible_to` subsets without `guest` cannot be contended and the sweep "+
			"must move past them. Tries: %+v", len(v.Tries), v.Tries)
	}
	// EVERY TRY BUT THE LAST IS VACUOUS, which is what makes the sweep a search
	// for an answerable world rather than a retry for a wanted answer: it stops
	// at the first try the clause can judge, so a non-final try that COULD have
	// been judged would mean it went on past an answer.
	for i, tr := range v.Tries[:len(v.Tries)-1] {
		why := tr.Contention.Vacuous()
		if why == "" {
			t.Errorf("try %d (%s) answered the clause and the sweep went on anyway",
				i+1, tr.Choices)
			continue
		}
		t.Logf("PROOF try %d (%s) NOT CONTENDED -- %s", i+1, tr.Choices, why)
	}
	// AND THE ANSWERING TRY IS THE LAST, with the plan it was issued in obeyed.
	answered, ok := v.Answered()
	if !ok {
		t.Fatalf("no try could answer the overfill clause:\n%s", rep.Render())
	}
	assertPlanDispatched(t, v.Plan, answered.Attempts)
	if !answered.Overlapped {
		t.Error("the answering try is recorded as not having overlapped, which Run refuses: a " +
			"non-overlapping fan-out is a could-not-run")
	}
	// THE FINDING IS THE POINT. join_event drops its lock inside the window, so
	// the system holds more than its own limit allows -- and the stage reports
	// it without this test naming the action, the capacity or the query.
	if f := v.Finding(); f == "" {
		t.Fatalf("the stage contended join_event at a limit of %d with %d request(s), %d accepted, "+
			"and found no overfill: the clause is silent over the defect it exists for.\n%s",
			answered.Contention.Limit, answered.Contention.Width, answered.Contention.Accepted,
			rep.Render())
	}
	if d := v.Disagreement(); d != "" {
		t.Errorf("the answers and the world disagree, which is a different defect and makes the "+
			"overfill verdict unreadable: %s", d)
	}
	if got := rep.Findings(); len(got) != 1 {
		t.Errorf("Report.Findings is %v; the toy holds exactly one contended action with one "+
			"overfill", got)
	}
	// THE RENDER IS CLOCK-FREE, which is what lets these lines go on the stream
	// D1 compares. The measured half is asserted present and separate, so a
	// duration cannot migrate into the hashed stream without an arm failing.
	out := rep.Render()
	for _, forbidden := range []string{"ms", "µs", "overlapped by"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("the rendered verdict contains %q, so it carries a clock and two attempts of "+
				"one seed could not be byte-identical:\n%s", forbidden, out)
		}
	}
	measured := rep.Measured()
	if len(measured) == 0 || !strings.Contains(strings.Join(measured, " "), "overlapped by") {
		t.Errorf("the measured half is %v; the fan-out overlapped, so it has a duration to report "+
			"on stderr", measured)
	}
	t.Logf("PROOF\n%s", out)
	t.Logf("PROOF measured (stderr only): %v", measured)
}

// D1 OVER THE STAGE's OWN STREAM: two runs of one seed, on a world put back
// between them, render byte-identically.
//
// IT IS THE CLAIM THE WHOLE CLOCK-FREE SPLIT EXISTS FOR, and it could not be
// made against the mechanism alone: Plan.Render was already armed for
// stability, but a stage prints its setup, its choices, its counts and its
// verdict beside the plan, and any one of those could carry a clock, a port, an
// id or a map's iteration order. rook named that risk at coop r8/36 -- a
// duration on the hashed stream would make D6 satisfied by the change that
// breaks D1 -- so this compares the whole rendering rather than the plan's line.
//
// THE WORLD IS PUT BACK AND NOT REUSED, which is also a measurement about the
// stage: its per-actor bootstrap creates a user, and cmd/toy refuses a name
// already taken, so a second Run against the FIRST run's world is a
// could-not-run by construction. That is the arrangement `rumor run` has
// anyway -- the provider owns the world a stage contends in -- and it is why
// this arm restores rather than simply calling Run twice.
func TestTwoRunsOfOneSeedRenderIdentically(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join("..", "..", "examples", "toy")

	prep, err := platform.Load(filepath.Join(dir, "platform.toml"), decl.TOML)
	if err != nil || !prep.OK() {
		t.Fatalf("platform: %v %v", err, prep.Refusals)
	}
	brep, err := bindings.Load(filepath.Join(dir, "bindings.toml"), decl.TOML)
	if err != nil || !brep.OK() {
		t.Fatalf("bindings: %v %v", err, brep.Refusals)
	}
	actors, err := schedule.Actors(prep.Platform.Venire)
	if err != nil {
		t.Fatalf("Actors: %v", err)
	}
	prov, err := rollback.Open(toyrestart.Name, rollback.Options{Dir: dir})
	if err != nil {
		t.Fatalf("opening %s: %v", toyrestart.Name, err)
	}
	if r, ok := prov.(interface{ RemoveWorld() }); ok {
		defer r.RemoveWorld()
	}
	at, ok := prov.(interface{ Addr() string })
	if !ok {
		t.Fatalf("%s does not expose the address it bound", toyrestart.Name)
	}
	cp, err := prov.Checkpoint(ctx)
	if err != nil {
		t.Fatalf("checkpointing the clean world: %v", err)
	}
	defer func() { _ = prov.Discard(ctx, cp) }()

	// THE ADDRESS IS RE-READ FOR EVERY ATTEMPT, because a restore rebinds the
	// port: sessions opened against the first address would be talking to a
	// world that no longer exists, which is the same reason cmd/rumor's shrink
	// re-opens its adapter per candidate.
	attempt := func() string {
		t.Helper()
		t.Setenv("TOY_ADDR", at.Addr())
		open := func() (*wire.Session, error) {
			s, oerr := wire.Open(ctx, wire.Spawn{
				Command: prep.Platform.Adapter.Command,
				Cwd:     dir,
				Env:     prep.Platform.Adapter.Env,
			}, 30*time.Second)
			if oerr != nil {
				return nil, oerr
			}
			t.Cleanup(func() { _ = s.Close() })
			return s, nil
		}
		host, oerr := open()
		if oerr != nil {
			t.Fatalf("opening the host: %v", oerr)
		}
		rep, rerr := concurrency.Run(brep.Bindings, host, open, actors, 7)
		if rerr != nil {
			t.Fatalf("Run: %v\n%s", rerr, rep.Render())
		}
		if len(rep.Findings()) == 0 {
			t.Fatalf("the attempt found nothing, so an identical pair of renderings would be "+
				"two copies of a run that tested nothing:\n%s", rep.Render())
		}
		return rep.Render()
	}

	first := attempt()
	if err := prov.Restore(ctx, cp); err != nil {
		t.Fatalf("putting the world back between attempts: %v", err)
	}
	second := attempt()
	if first != second {
		t.Errorf("two attempts of seed 7 rendered differently:\n--- first\n%s--- second\n%s",
			first, second)
	}
	t.Logf("PROOF two attempts of seed 7, byte-identical:\n%s", first)
}
