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
	"os"
	"path/filepath"
	"sort"
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

// AN ACTION THAT EMITS NOTHING IS RECORDED AND MINTS NOTHING, ruled at coop
// r5/62. examples/twin's publish_doc_ui is the live case -- it declares neither
// `emits` nor `creates`.
//
// THE ACTION IS ONE THE FAKE'S SURFACE RECORDS, with its `emits` stripped in
// this project copy: `emits` is the PROJECT's declaration and the surface is
// the ADAPTER's, so stripping it makes a recorded action that claims nothing.
// My first version invented a new action name, and the fake faulted on an
// action it does not record -- an arm failing for a fixture-pairing reason and
// saying nothing about the behavior.
//
// IT ALSO DECLARES TWO `creates`, which mintKind refuses: ordered the other way
// round, that refusal would reach a step that was never going to mint a handle
// at all, which is the defect this arm exists for.
func TestAnActionThatEmitsNothingIsRecordedAndMintsNothing(t *testing.T) {
	ctx := context.Background()
	p := walkProject(t)
	p.Actions = []bindings.Action{{
		Name: "create_event", Context: "api",
		Requires: []string{"may_create_event"},
		Creates:  []string{"event", "user"},
	}}

	sess, err := wire.Open(ctx, fakeSpawn(), 20*time.Second)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = sess.Close() }()

	rec, err := schedule.Walk(ctx, p, sess, 11, 4, 1)
	if err != nil {
		t.Fatalf("a step whose action emits nothing must be recorded, not refused: %v", err)
	}

	drawn := 0
	for _, s := range rec.Steps {
		if s.Kind != "create_event" {
			continue
		}
		drawn++
		if len(s.Handles) != 0 {
			t.Errorf("step %d emitted nothing and minted %v", s.N, s.Handles)
		}
	}
	// THE ARM ASSERTS ITS OWN REACH: with the action never drawn, "no step
	// minted a handle" is true of a walk that did nothing.
	if drawn == 0 {
		t.Fatalf("the action was never drawn, so this arm never reached the behavior it is "+
			"about; kinds were %v", rec.KindSequence())
	}
	if len(rec.HandleSequence()) != 0 {
		t.Errorf("the walk minted %v where no action emits anything", rec.HandleSequence())
	}
}

// AN EMPTY `ids` REFUSAL CARRIES THE ADAPTER'S OWN ANSWER.
//
// ibis drove both causes over the wire (coop r5/64): no session is 401, no args
// is 400, and BOTH come back with an empty ids map. A refusal naming only the
// empty map sends a reader at the wrong half -- and it did: ibis read my walk's
// step-1 failure as the args case and the response says 401, because a walk
// sends no session token either. The fix for that is to print what the adapter
// said, and this arm is what keeps it printed.
func TestAnEmptyIDsRefusalCarriesTheAdaptersAnswer(t *testing.T) {
	ctx := context.Background()
	p := walkProject(t)
	p.Actions = []bindings.Action{{
		Name: "create_event", Context: "api", Emits: []string{"event.created"},
		Requires: []string{"may_create_event"}, Creates: []string{"event"},
	}}

	// A CANNED SESSION COMES FIRST AS OF R5.4, and the ids say why: the canned
	// lines are answered in request order, and create_event's context is "api",
	// so the walk now acquires a token before it executes anything. Before
	// this, describe was 1 and the execute was 2; now the session is 2 and the
	// execute is 3. The arm failed on exactly that when sessions landed, which
	// is the fixture's premise moving and not the walk's refusal changing.
	token := mustJSON(t, wire.SessionResponse{
		Envelope: wire.Envelope{Kind: wire.KindSession, Contract: wire.ContractVersion, ID: "2"},
		Token:    "canned-token",
	})

	// A canned execute answering NO ids and a recognisable body, which is the
	// shape both real causes produce.
	empty := mustJSON(t, wire.ExecuteResponse{
		Envelope: wire.Envelope{Kind: wire.KindExecute, Contract: wire.ContractVersion, ID: "3"},
		IDs:      map[string]any{},
		Response: map[string]any{"status": 401, "body": map[string]any{"error": "needs a session"}},
	})
	sess, err := wire.Open(ctx, cannedAdapter(t, realDescribeLine(t), token, empty), 20*time.Second)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = sess.Close() }()

	_, err = schedule.Walk(ctx, p, sess, 1, 1, 1)
	if err == nil {
		t.Fatal("a declared `creates` with no id in the answer must refuse: the handle would " +
			"resolve to nothing")
	}
	for _, want := range []string{"401", "needs a session", "more than one cause"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal must carry %q so a reader can tell the causes apart; got %v", want, err)
		}
	}
}

// ONE SESSION PER ACTOR PER CONTEXT, ACQUIRED AT FIRST USE AND REUSED AFTER.
//
// THE COUNT IS THE ONLY OBSERVABLE, which is why Record carries it: every token
// the fake issues is the same string, so a holder that re-acquired on every
// step would send requests indistinguishable from one that cached. The arm
// therefore compares the count against the number of steps that actually
// executed an action -- reuse is the count being SMALLER, and an arm that only
// checked "at least one" would pass on a holder with no cache at all.
func TestASessionIsAcquiredOncePerActorPerContext(t *testing.T) {
	ctx := context.Background()
	p := walkProject(t)

	sess, err := wire.Open(ctx, fakeSpawn(), 20*time.Second)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = sess.Close() }()

	rec, err := schedule.Walk(ctx, p, sess, 7, 10, 1)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}

	// THE DENOMINATOR IS MEASURED FROM THE RECORD, not assumed from the seed:
	// a stalled step draws no action and acquires nothing.
	executed := 0
	actors := map[string]bool{}
	for _, st := range rec.Steps {
		switch st.Kind {
		case "", schedule.Suborn, schedule.Canard, schedule.Palter, schedule.Essoin:
			continue
		}
		executed++
		actors[st.Actor] = true
	}
	if executed < 2 {
		t.Fatalf("only %d step(s) executed an action, so this arm cannot tell a reused session "+
			"from a re-acquired one:\n%v", executed, rec.Steps)
	}
	if rec.Sessions == 0 {
		t.Errorf("%d action(s) in context api executed and no session was acquired", executed)
	}
	if rec.Sessions > len(actors) {
		t.Errorf("acquired %d session(s) for %d actor(s) in ONE context: a token is held per actor "+
			"per context, so this is a cache that is not caching", rec.Sessions, len(actors))
	}
	if rec.Sessions >= executed {
		t.Errorf("acquired %d session(s) across %d executed step(s): first use means the count is "+
			"smaller than the number of steps, or nothing is being reused", rec.Sessions, executed)
	}

	// AND THE COUNT REPLAYS. A session acquired at first use is acquired at a
	// step the seed chose, so two attempts from one seed must agree on how many
	// there were.
	again, err := schedule.Walk(ctx, p, sess, 7, 10, 2)
	if err != nil {
		t.Fatalf("Walk again: %v", err)
	}
	if again.Sessions != rec.Sessions {
		t.Errorf("one seed acquired %d session(s) then %d", rec.Sessions, again.Sessions)
	}
}

// AN ACTION TAKEN OUTSIDE A SESSION ACQUIRES NONE, driven through a real walk
// and not only against the holder: the project declares context "none" and the
// run must send no `session` request at all.
func TestAWalkOfContextFreeActionsAcquiresNoSession(t *testing.T) {
	ctx := context.Background()
	p := walkProject(t)
	for i := range p.Actions {
		p.Actions[i].Context = "none"
	}

	sess, err := wire.Open(ctx, fakeSpawn(), 20*time.Second)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = sess.Close() }()

	rec, err := schedule.Walk(ctx, p, sess, 7, 10, 1)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	executed := 0
	for _, st := range rec.Steps {
		if st.Kind == "create_event" || st.Kind == "join_event" {
			executed++
		}
	}
	if executed == 0 {
		t.Fatalf("no action executed, so an acquisition count of zero says nothing:\n%v", rec.Steps)
	}
	if rec.Sessions != 0 {
		t.Errorf("acquired %d session(s) for %d action(s) whose context is none", rec.Sessions, executed)
	}
}

// AN EMPTY TOKEN IS REFUSED RATHER THAN HELD. omitempty would omit it, so an
// action whose context is NOT "none" would go out looking exactly like one
// taken outside a session, and the adapter's 401 would send a reader looking at
// the wrong half.
func TestAnEmptyTokenIsRefused(t *testing.T) {
	ctx := context.Background()
	p := walkProject(t)
	p.Actions = []bindings.Action{{
		Name: "create_event", Context: "api", Emits: []string{"event.created"},
		Requires: []string{"may_create_event"}, Creates: []string{"event"},
	}}

	blank := mustJSON(t, wire.SessionResponse{
		Envelope: wire.Envelope{Kind: wire.KindSession, Contract: wire.ContractVersion, ID: "2"},
		Token:    "",
	})
	sess, err := wire.Open(ctx, cannedAdapter(t, realDescribeLine(t), blank), 20*time.Second)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = sess.Close() }()

	_, err = schedule.Walk(ctx, p, sess, 1, 1, 1)
	if err == nil {
		t.Fatal("a session answered with an empty token was accepted")
	}
	if !strings.Contains(err.Error(), "empty token") {
		t.Errorf("the refusal must name the empty token; got %v", err)
	}
}

// TWO CONTEXTS, ONE ACTOR, TWO SESSIONS. Without this arm the per-CONTEXT half
// of "one per actor per context" is unmeasured: every other arm here uses a
// project with a single context, so a holder keyed on the actor alone would
// pass all of them. An actor's api token reused for its ui actions would be a
// token the adapter issued for something else, and the two contexts would be
// indistinguishable from the engine's side.
func TestOneActorInTwoContextsHoldsTwoSessions(t *testing.T) {
	ctx := context.Background()
	p := walkProject(t)
	p.Venire = []platform.Member{{Role: "member", Count: 1}}
	p.Actions[0].Context = "api"
	p.Actions[1].Context = "ui"

	sess, err := wire.Open(ctx, fakeSpawn(), 20*time.Second)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = sess.Close() }()

	rec, err := schedule.Walk(ctx, p, sess, 7, 12, 1)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}

	// BOTH CONTEXTS MUST HAVE BEEN REACHED or the count proves nothing: a walk
	// that only ever took create_event would hold one session correctly.
	kinds := map[string]bool{}
	for _, st := range rec.Steps {
		kinds[st.Kind] = true
	}
	if !kinds["create_event"] || !kinds["join_event"] {
		t.Fatalf("the walk did not reach both contexts (%v), so the session count says nothing "+
			"about per-context holding:\n%v", kinds, rec.Steps)
	}
	if rec.Sessions != 2 {
		t.Errorf("one actor acting in two contexts acquired %d session(s), want 2: the token is "+
			"held per actor per CONTEXT, and one token for both would be a token the adapter "+
			"issued for something else", rec.Sessions)
	}
}

// capturingAdapter answers from canned lines AND keeps every request it was
// sent, so an arm can read what the engine actually wrote.
//
// WHY IT EXISTS: cannedAdapter discards its stdin, and internal/fake ignores an
// execute's token entirely -- so "the engine attaches the token" had no oracle
// in this tree at all, and a mutant that dropped the token from every execute
// survived the whole suite. The real enforcement is cmd/toy answering 401
// without a session (R5.4's done-when drives that), but a done-when against
// another process is not a check on this one, and the engine's own half
// deserves an arm here.
//
// The background `cat` is what consumes stdin; the foreground one emits the
// canned responses. Only one reader touches stdin, so there is no race over it.
func capturingAdapter(t *testing.T, lines ...string) (wire.Spawn, string) {
	t.Helper()
	dir := t.TempDir()
	canned := filepath.Join(dir, "canned.jsonl")
	seen := filepath.Join(dir, "seen.jsonl")
	body := ""
	for _, l := range lines {
		body += l + "\n"
	}
	if err := os.WriteFile(canned, []byte(body), 0o600); err != nil {
		t.Fatalf("writing the canned responses: %v", err)
	}
	// THE CAPTURE IS WRITTEN BEFORE THE RESPONSE IS SENT, which is what makes
	// this race-free rather than nearly so. Three measured facts shaped it,
	// each found by the arms failing on their own no-subject guard rather than
	// on the property they are about:
	//
	// `cat > file` block-buffers when its output is a regular file, so the
	// requests sat in cat's buffer and were lost when the adapter was killed.
	//
	// POSIX has a non-interactive shell redirect an ASYNCHRONOUS list's
	// standard input to /dev/null, so a BACKGROUNDED reader saw EOF at once
	// and never created the file at all.
	//
	// And emitting the canned lines up front (which is what cannedAdapter
	// does) leaves the last request on the wrong side of the kill: the engine
	// wrote the execute, Walk returned, and Close killed the shell before the
	// reader was scheduled. Answering per request inverts that -- the engine
	// cannot have read a response without this loop having already appended
	// the request that earned it, so when Walk returns the capture is complete
	// by construction.
	script := "exec 3< " + canned + "\n" +
		"while IFS= read -r l; do\n" +
		"  printf '%s\\n' \"$l\" >> " + seen + "\n" +
		"  IFS= read -r r <&3 && printf '%s\\n' \"$r\"\n" +
		"done\n"
	return wire.Spawn{Command: []string{"sh", "-c", script}}, seen
}

// THE TOKEN REACHES THE EXECUTE, and the engine is what put it there. Driven by
// reading the bytes the walk wrote rather than by trusting that a field set in
// one function arrives in another.
func TestTheAcquiredTokenIsOnTheExecute(t *testing.T) {
	ctx := context.Background()
	p := walkProject(t)
	p.Actions = []bindings.Action{{
		Name: "create_event", Context: "api", Emits: []string{"event.created"},
		Requires: []string{"may_create_event"}, Creates: []string{"event"},
	}}

	token := mustJSON(t, wire.SessionResponse{
		Envelope: wire.Envelope{Kind: wire.KindSession, Contract: wire.ContractVersion, ID: "2"},
		Token:    "the-one-token",
	})
	done := mustJSON(t, wire.ExecuteResponse{
		Envelope: wire.Envelope{Kind: wire.KindExecute, Contract: wire.ContractVersion, ID: "3"},
		IDs:      map[string]any{"event": "e-1"},
		Response: map[string]any{"status": 201},
	})
	spawn, seen := capturingAdapter(t, realDescribeLine(t), token, done)
	sess, err := wire.Open(ctx, spawn, 20*time.Second)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := schedule.Walk(ctx, p, sess, 1, 1, 1); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	// THE PROCESS IS STOPPED BEFORE THE CAPTURE IS READ, so the background cat
	// has flushed: reading a file another process still holds open is how a
	// capture reads short and the arm passes on nothing.
	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	raw, err := os.ReadFile(seen)
	if err != nil {
		t.Fatalf("reading what the engine sent: %v", err)
	}
	var sessionLine, executeLine string
	for _, l := range strings.Split(string(raw), "\n") {
		switch {
		case strings.Contains(l, `"kind":"session"`):
			sessionLine = l
		case strings.Contains(l, `"kind":"execute"`):
			executeLine = l
		}
	}
	if sessionLine == "" || executeLine == "" {
		t.Fatalf("the capture holds no session and/or no execute, so this arm is reading "+
			"nothing:\n%s", raw)
	}
	if !strings.Contains(sessionLine, `"actor":"member_1"`) && !strings.Contains(sessionLine, `"actor":"member_2"`) {
		t.Errorf("the session request names no actor: %s", sessionLine)
	}
	if !strings.Contains(executeLine, `"token":"the-one-token"`) {
		t.Errorf("the execute carries no token, or not the acquired one: %s", executeLine)
	}
}

// AND AN ACTION OUTSIDE A SESSION CARRIES NO TOKEN FIELD AT ALL, the other side
// of the contract row: "present exactly when the action's context is not none".
// An arm asserting only the presence would pass on an engine that always sent
// one.
func TestAnActionOutsideASessionCarriesNoToken(t *testing.T) {
	ctx := context.Background()
	p := walkProject(t)
	p.Actions = []bindings.Action{{
		Name: "create_event", Context: "none", Emits: []string{"event.created"},
		Requires: []string{"may_create_event"}, Creates: []string{"event"},
	}}

	done := mustJSON(t, wire.ExecuteResponse{
		Envelope: wire.Envelope{Kind: wire.KindExecute, Contract: wire.ContractVersion, ID: "2"},
		IDs:      map[string]any{"event": "e-1"},
		Response: map[string]any{"status": 201},
	})
	spawn, seen := capturingAdapter(t, realDescribeLine(t), done)
	sess, err := wire.Open(ctx, spawn, 20*time.Second)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := schedule.Walk(ctx, p, sess, 1, 1, 1); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	raw, err := os.ReadFile(seen)
	if err != nil {
		t.Fatalf("reading what the engine sent: %v", err)
	}
	if !strings.Contains(string(raw), `"kind":"execute"`) {
		t.Fatalf("the capture holds no execute, so this arm is reading nothing:\n%s", raw)
	}
	if strings.Contains(string(raw), `"kind":"session"`) {
		t.Errorf("a session was acquired for an action whose context is none:\n%s", raw)
	}
	if strings.Contains(string(raw), `"token"`) {
		t.Errorf("the execute carries a token field for an action taken outside a session:\n%s", raw)
	}
}

// A HANDLE ARGUMENT IS DRAWN FROM THE BOUND HANDLES OF ITS KIND, AND THE DRAW
// IS A DRAW.
//
// THE ORACLE IS RECONSTRUCTED FROM THE RECORD rather than asserted loosely: for
// each consuming step the candidate set is the event handles bound by earlier
// steps, so "it chose other than the minimum at least once" is checkable. A
// weaker arm -- that two runs used different handles -- would pass on a
// chooser that always takes the first, because which handle sorts first depends
// on who created one first.
//
// IT ALSO DRIVES AN ORDERING I SAID I WOULD DRIVE AND NOT ASSUME (coop r5/71):
// an action needing a handle cannot precede the action that binds one. Nothing
// here arranges that; Resolvable holds join_event out of the pool until an
// event exists, and the walk below would refuse if it did not.
func TestAHandleArgumentIsDrawnFromTheBoundHandles(t *testing.T) {
	ctx := context.Background()
	p := walkProject(t)
	// join_event now NEEDS an event, declared the way a project declares it.
	p.Actions[1].Args = []bindings.Arg{
		{Name: "event", Source: bindings.ArgHandle, Of: "event"},
	}

	sess, err := wire.Open(ctx, fakeSpawn(), 20*time.Second)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = sess.Close() }()

	nonMinimum := false
	sawTwoCandidates := false
	consuming := 0
	for _, seed := range []uint64{1, 2, 3, 4, 5, 6, 7, 8} {
		rec, err := schedule.Walk(ctx, p, sess, seed, 14, 1)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		var bound []string
		for _, st := range rec.Steps {
			if len(st.Used) > 0 {
				consuming++
				var cands []string
				for _, h := range bound {
					if strings.Contains(h, ".event#") {
						cands = append(cands, h)
					}
				}
				sort.Strings(cands)
				if len(cands) > 1 {
					sawTwoCandidates = true
				}
				if len(cands) == 0 {
					t.Errorf("seed %d: a step consumed %v with no event bound before it: the "+
						"action should have been inapplicable", seed, st.Used)
				} else if st.Used[0] != cands[0] {
					nonMinimum = true
				}
			}
			for _, h := range st.Handles {
				if _, ok := rec.IDs[h]; ok {
					bound = append(bound, h)
				}
			}
		}
	}
	if consuming == 0 {
		t.Fatal("no step consumed a handle across eight seeds, so this arm is measuring nothing")
	}
	if !sawTwoCandidates {
		t.Fatalf("no consuming step ever had two candidates (%d consuming step(s)), so a chooser "+
			"that always takes the first could not be told from one that draws", consuming)
	}
	if !nonMinimum {
		t.Errorf("across eight seeds and %d consuming step(s), every chosen handle was the "+
			"first of its candidate set: the choice is not a draw", consuming)
	}
}

// AND THE CHOICE REPLAYS: two attempts from one seed consume the same handles.
// Issue 72's done-when compares what the steps CREATED; this is the same
// property over what they were ABOUT, which a drawn choice makes a separate
// question.
func TestTwoAttemptsConsumeTheSameHandles(t *testing.T) {
	ctx := context.Background()
	p := walkProject(t)
	p.Actions[1].Args = []bindings.Arg{
		{Name: "event", Source: bindings.ArgHandle, Of: "event"},
	}

	sess, err := wire.Open(ctx, fakeSpawn(), 20*time.Second)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = sess.Close() }()

	used := func(r schedule.Record) []string {
		var out []string
		for _, st := range r.Steps {
			out = append(out, st.Used...)
		}
		return out
	}
	a, err := schedule.Walk(ctx, p, sess, 11, 14, 1)
	if err != nil {
		t.Fatalf("attempt 1: %v", err)
	}
	b, err := schedule.Walk(ctx, p, sess, 11, 14, 2)
	if err != nil {
		t.Fatalf("attempt 2: %v", err)
	}
	ua, ub := used(a), used(b)
	if len(ua) == 0 {
		t.Fatal("neither attempt consumed a handle, so comparing them says nothing")
	}
	if strings.Join(ua, " ") != strings.Join(ub, " ") {
		t.Errorf("one seed consumed two different handle sequences:\n%v\n%v", ua, ub)
	}
}
