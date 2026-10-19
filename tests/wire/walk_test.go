// Issue 72's done-when, driven end to end: two attempts from one seed produce
// the same handle sequence while every id behind it differs, and --seed
// reproduces a run.
//
// WHY IT LIVES BESIDE THE SESSION ARMS: it needs a really spawned
// `rumor fake-adapter`, and tests/wire is where TestMain already builds rumor
// onto PATH. The walk is internal/schedule's, but the thing under test here is
// the whole path -- scheduler, wire client, handle table, adapter process --
// and a package boundary is not what this arm is about.
package wire_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/platform"
	"forge.axonibyte.com/axonibyte/rumor/internal/schedule"
	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

// walkProject is a project the FAKE can actually serve: its two actions are the
// two the fake's default surface records, so every honest step reaches a real
// adapter and gets a real id back.
//
// IT IS BUILT HERE AND NOT READ FROM examples/toy, and the reason is the
// adapter: the toy's bindings declare six actions, and the fake's surface
// records two. A walk over the toy's bindings against the fake would refuse on
// the first action the surface does not know, which is a fault about the
// FIXTURE PAIRING and not about the walk. examples/toy against cmd/toy-adapter
// is the pairing that exercises the toy, and that is R5.2's suite.
func walkProject(t *testing.T) schedule.Project {
	t.Helper()
	return schedule.Project{
		// The statute is the toy's, so applicability is evaluated by real
		// rules rather than by a permissive stub. may_create_event and
		// may_create_user are the two its defaults permit from an empty world.
		Statute: toyStatute(t),
		Actions: []bindings.Action{
			{Name: "create_event", Context: "api", Emits: []string{"event.created"},
				Requires: []string{"may_create_event"}, Creates: []string{"event"}},
			{Name: "join_event", Context: "api", Emits: []string{"event.joined"},
				Requires: []string{"may_join"}, Touches: []string{"store"}},
		},
		Weights: platform.Weights{
			Default: map[string]int{
				"create_event": 4, "join_event": 1,
				schedule.Suborn: 0, schedule.Canard: 0,
				schedule.Palter: 0, schedule.Essoin: 0,
			},
		},
		Venire: []platform.Member{{Role: "member", Count: 2}},
	}
}

// TWO ATTEMPTS, ONE SEED: the handle sequence must be identical and every id
// behind it must differ. That pair of assertions IS issue 72's done-when, and
// neither half alone says anything -- equal handles with equal ids would be
// satisfied by a walk that never touched the handle table, and differing ids
// with differing handles would be a walk that is not deterministic at all.
func TestTwoAttemptsFromOneSeedAgreeOnHandlesAndDifferOnIDs(t *testing.T) {
	ctx := context.Background()
	p := walkProject(t)
	const seed = 20261019

	attempt := func(n int, salt string) schedule.Record {
		t.Helper()
		sess, err := wire.Open(ctx, fakeSpawn("--id-salt", salt), 20*time.Second)
		if err != nil {
			t.Fatalf("attempt %d: Open: %v", n, err)
		}
		defer func() { _ = sess.Close() }()
		rec, err := schedule.Walk(ctx, p, sess, seed, 6, n)
		if err != nil {
			t.Fatalf("attempt %d: Walk: %v", n, err)
		}
		return rec
	}

	one := attempt(1, "att1")
	two := attempt(2, "att2")

	// THE ARM ASSERTS ITS OWN REACH FIRST. With no handles minted, "the two
	// sequences agree" is true of two empty lists and the id comparison has
	// nothing to compare -- which is the state this arm would pass in while
	// proving nothing.
	seq := one.HandleSequence()
	if len(seq) < 2 {
		t.Fatalf("the walk minted %d handle(s) in 6 steps (%v), and this arm needs at least two "+
			"to show a SEQUENCE rather than a single value; kinds drawn were %v",
			len(seq), seq, one.KindSequence())
	}
	if len(one.IDs) == 0 {
		t.Fatalf("no handle was bound to an id, so there is nothing for the id halves to differ "+
			"on; handles were %v", seq)
	}

	if strings.Join(seq, ",") != strings.Join(two.HandleSequence(), ",") {
		t.Fatalf("one seed gave two handle sequences:\n  attempt 1 %v\n  attempt 2 %v",
			seq, two.HandleSequence())
	}
	if strings.Join(one.KindSequence(), ",") != strings.Join(two.KindSequence(), ",") {
		t.Errorf("one seed drew two kind sequences:\n  attempt 1 %v\n  attempt 2 %v",
			one.KindSequence(), two.KindSequence())
	}

	// AND EVERY ID MOVED. Not "some id differs": one id that stayed put is the
	// state a handle table that quietly reused an earlier binding would
	// produce, and it would pass a weaker check.
	for _, h := range seq {
		a, aok := one.IDs[h]
		b, bok := two.IDs[h]
		if aok != bok {
			t.Errorf("handle %q is bound in one attempt and not the other (%v / %v)", h, aok, bok)
			continue
		}
		if !aok {
			continue // minted and unbound in both, which is consistent
		}
		if a == b {
			t.Errorf("handle %q resolves to %q in BOTH attempts: the ids did not move, so this "+
				"suite cannot tell a handle sequence from an id sequence", h, a)
		}
	}
	t.Logf("PROOF one seed, two attempts: handles %v; ids %v then %v", seq, one.IDs, two.IDs)
}

// --seed REPRODUCES A RUN: the same seed twice against the same adapter gives
// the same record, ids included. This is the half the salt arm cannot show,
// since there the ids are made to differ on purpose.
func TestTheSameSeedReproducesTheWholeRun(t *testing.T) {
	ctx := context.Background()
	p := walkProject(t)
	const seed = 7

	run := func() schedule.Record {
		t.Helper()
		sess, err := wire.Open(ctx, fakeSpawn(), 20*time.Second)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer func() { _ = sess.Close() }()
		rec, err := schedule.Walk(ctx, p, sess, seed, 5, 1)
		if err != nil {
			t.Fatalf("Walk: %v", err)
		}
		return rec
	}

	a, b := run(), run()
	if strings.Join(a.KindSequence(), ",") != strings.Join(b.KindSequence(), ",") {
		t.Errorf("the same seed drew two sequences:\n  %v\n  %v", a.KindSequence(), b.KindSequence())
	}
	if strings.Join(a.HandleSequence(), ",") != strings.Join(b.HandleSequence(), ",") {
		t.Errorf("the same seed minted two handle sequences:\n  %v\n  %v",
			a.HandleSequence(), b.HandleSequence())
	}
	for h, id := range a.IDs {
		if b.IDs[h] != id {
			t.Errorf("handle %q resolved to %q then %q with no salt: the run is not reproducible",
				h, id, b.IDs[h])
		}
	}
}

// A DIFFERENT SEED MUST WALK DIFFERENTLY, which is the control for both arms
// above: if every seed produced the same walk, "the same seed reproduces a run"
// would be true and empty.
func TestADifferentSeedWalksDifferently(t *testing.T) {
	ctx := context.Background()
	p := walkProject(t)

	run := func(seed uint64) schedule.Record {
		t.Helper()
		sess, err := wire.Open(ctx, fakeSpawn(), 20*time.Second)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer func() { _ = sess.Close() }()
		rec, err := schedule.Walk(ctx, p, sess, seed, 8, 1)
		if err != nil {
			t.Fatalf("Walk with seed %d: %v", seed, err)
		}
		return rec
	}

	// SEEDS ARE TRIED UNTIL ONE DIVERGES, and the arm fails if NONE does.
	// Asserting that two particular seeds differ would pin an accident of this
	// generator; asserting that the walk is seed-SENSITIVE is the property.
	base := run(1).KindSequence()
	for _, seed := range []uint64{2, 3, 4, 5, 99, 12345} {
		if strings.Join(run(seed).KindSequence(), ",") != strings.Join(base, ",") {
			return
		}
	}
	t.Errorf("seven seeds all drew %v, so the walk does not depend on the seed and every "+
		"reproducibility arm here is vacuous", base)
}
