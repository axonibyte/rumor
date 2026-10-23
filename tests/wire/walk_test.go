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
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/ledger"
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
				// THE HONEST TELL AT ZERO, declared and never drawn. R5.4 put
				// it in the pool (fable, coop r5/114), and an undeclared weight
				// is REFUSED rather than read as zero -- so a fixture omitting
				// it fails on the weight file and not on its subject. Zero
				// rather than a number because these arms are about handles,
				// visibility and the role gate: drawing a kind they do not
				// measure would move every walk they assert on, for nothing.
				// A ONE AND NOT A ZERO, which these arms measured for me. At zero
				// the kind is still APPLICABLE -- any actor holding any claim
				// can tell -- so a step whose only candidate was `tell` gave a
				// pool that was non-empty and entirely zero, and Pick refuses
				// that rather than skipping it. Before this kind existed such a
				// step had an EMPTY pool and the walk recorded a no-op and
				// carried on, so `tell = 0` turns a skipped step into an
				// aborted walk. Reported to fable rather than worked around.
				schedule.Tell: 1,
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

// AN ACTION IN AN UNHELD CONTEXT IS NEVER DRAWN, driven through a whole walk
// rather than through Pool alone: the gate exists so the engine never acquires
// a session in a context the project says this role never has, and "never
// drawn" is the observable form of that.
//
// THE CONTROL IS THE SAME WALK WITH THE CONTEXT HELD. Without it the arm would
// pass on a walk that never drew join_event for any reason at all -- a weight
// of zero, an unsatisfied precondition, or a gate that removed everything.
func TestAnActionInAnUnheldContextIsNeverDrawn(t *testing.T) {
	ctx := context.Background()
	sess, err := wire.Open(ctx, fakeSpawn(), 20*time.Second)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = sess.Close() }()

	drew := func(roles []bindings.Role) (bool, bool) {
		t.Helper()
		p := walkProject(t)
		p.Actions[1].Context = "ui"
		p.Roles = roles
		sawUI, sawAPI := false, false
		for _, seed := range []uint64{1, 2, 3, 4, 5} {
			rec, werr := schedule.Walk(ctx, p, sess, seed, 12, 1)
			if werr != nil {
				t.Fatalf("seed %d: %v", seed, werr)
			}
			for _, st := range rec.Steps {
				switch st.Kind {
				case "join_event":
					sawUI = true
				case "create_event":
					sawAPI = true
				}
			}
		}
		return sawUI, sawAPI
	}

	held, api := drew([]bindings.Role{{Name: "member", Contexts: []string{"api", "ui"}}})
	if !held {
		t.Fatalf("the control never drew the ui action even with ui held, so this arm cannot tell "+
			"a gate from an absence (api drawn: %v)", api)
	}
	gated, apiStillDrawn := drew([]bindings.Role{{Name: "member", Contexts: []string{"api"}}})
	if gated {
		t.Error("an action needing ui was drawn by a role holding only api")
	}
	if !apiStillDrawn {
		t.Error("gating ui also stopped the api action being drawn, so the gate is not gating on " +
			"the context")
	}
}

// THE CLAIM IN THE LEDGER CARRIES THE DRAWN VISIBILITY, read off the record
// rather than off the function that computes it. A mutant dropping VisibleTo
// from the appended fact survives every unit arm over that function, because
// the fact is what the statute reads and the function is not.
func TestTheClaimCarriesTheDrawnVisibility(t *testing.T) {
	ctx := context.Background()
	p := walkProject(t)
	p.Actions[0].Args = []bindings.Arg{
		// TWO SUBSETS, NOT ONE, so the arm also shows the value is DRAWN: with
		// a single subset any chooser would produce it.
		{Name: "visible_to", Source: bindings.ArgDraw, From: []any{
			[]any{"member"},
			[]any{"member", "admin"},
		}},
	}

	sess, err := wire.Open(ctx, fakeSpawn(), 20*time.Second)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = sess.Close() }()

	seen := map[string]int{}
	claims := 0
	for _, seed := range []uint64{1, 2, 3, 4, 5, 6} {
		rec, werr := schedule.Walk(ctx, p, sess, seed, 10, 1)
		if werr != nil {
			t.Fatalf("seed %d: %v", seed, werr)
		}
		for _, f := range rec.Facts {
			if f.Type != "event.created" {
				continue
			}
			claims++
			if len(f.VisibleTo) == 0 {
				t.Fatalf("seed %d: claim %s carries no visible_to, so the drawn subset never "+
					"reached the fact a statute reads", seed, f.Ref)
			}
			seen[strings.Join(f.VisibleTo, ",")]++
		}
	}
	if claims == 0 {
		t.Fatal("no create_event claim was appended across six seeds, so this arm is reading nothing")
	}
	// BOTH SUBSETS MUST APPEAR, which is what separates a drawn value from a
	// constant one the arm would accept either way.
	if len(seen) < 2 {
		t.Errorf("six seeds produced %d distinct visibility subset(s) over %d claim(s) (%v): the "+
			"value is not being drawn", len(seen), claims, seen)
	}
}

// A WALKED CLAIM CARRIES THE HANDLES IT NAMES, which is the convention the
// LEDGER already had and the walk was the one writer not following it. fable
// ruled it into R5.4 at coop r5/105 after I measured the corpus: 62 committed
// claims, 52 carrying `handles == [ref]` and 10 carrying a subject beside it --
// `event.joined` with ref `bob.join#1` carries `["bob.join#1","alice.event#1"]`
// -- and none carrying no key at all. Every claim the walk wrote carried the
// minted handle alone, so the first WALKED join was a claim of a shape the
// corpus does not contain, and R5.3's grouping reads `handles` to say which
// entity a claim is about.
//
// THE FIXTURE DECLARES join_event's HANDLE ARGUMENT, as examples/toy does. That
// is what makes this arm about the convention rather than about the fixture: an
// action with no `handle` argument consumes nothing, so it would carry the
// minted handle alone and the arm would pass against the defect.
//
// TWO SHAPES IN ONE WALK, which is why both are asserted off one record rather
// than from two runs: `create_event` consumes no handle and must carry exactly
// its own, `join_event` consumes the event and must carry both in that order.
// An arm reading only the join would pass against a walk that appended every
// bound handle to every claim.
func TestAWalkedClaimCarriesTheHandlesItNames(t *testing.T) {
	ctx := context.Background()
	p := walkProject(t)
	for i := range p.Actions {
		if p.Actions[i].Name == "join_event" {
			p.Actions[i].Args = []bindings.Arg{{Name: "event", Source: bindings.ArgHandle, Of: "event"}}
		}
	}

	sess, err := wire.Open(ctx, fakeSpawn("--id-salt", "handles"), 20*time.Second)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = sess.Close() }()

	rec, err := schedule.Walk(ctx, p, sess, 20261019, 8, 1)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}

	// THE REACH IS ASSERTED FIRST. With no join drawn there is no two-handle
	// claim in the ledger and every assertion below is true of the remaining
	// ones, which is exactly the state this arm must not pass in.
	var joins, creates []ledger.Fact
	for _, f := range rec.Facts {
		if f.Kind != ledger.KindClaim {
			continue
		}
		switch f.Type {
		case "event.joined":
			joins = append(joins, f)
		case "event.created":
			creates = append(creates, f)
		}
	}
	if len(joins) == 0 {
		t.Fatalf("the walk drew no join in 8 steps (kinds %v), so there is no consuming claim to "+
			"read and this arm proves nothing", rec.KindSequence())
	}
	if len(creates) == 0 {
		t.Fatalf("the walk created no event (kinds %v), so the no-argument shape is unmeasured",
			rec.KindSequence())
	}

	for _, f := range creates {
		if len(f.Handles) != 1 || f.Handles[0] != f.Ref {
			t.Errorf("event.created %q carries handles %v: an action consuming no handle names "+
				"only what it minted, which is the 52-claim shape in the corpus", f.Ref, f.Handles)
		}
	}
	for _, f := range joins {
		if len(f.Handles) != 2 {
			t.Errorf("event.joined %q carries handles %v: a join names the join it minted AND the "+
				"event it consumed, so R5.3's grouping can say which entity it is about",
				f.Ref, f.Handles)
			continue
		}
		if f.Handles[0] != f.Ref {
			t.Errorf("event.joined %q carries %v: the MINTED handle comes first, which is the "+
				"corpus's order (`bob.join#1` before `alice.event#1`)", f.Ref, f.Handles)
		}
		// AND THE SECOND IS AN EVENT THE WALK ACTUALLY MINTED, not any string:
		// a claim naming a handle no step produced would be unreadable in a
		// different way, and `rec.IDs` is the walk's own record of what it bound.
		subject := f.Handles[1]
		if _, bound := rec.IDs[subject]; !bound {
			t.Errorf("event.joined %q names subject %q, which this walk never bound (%v)",
				f.Ref, subject, rec.IDs)
		}
		if !strings.Contains(subject, ".event#") {
			t.Errorf("event.joined %q names subject %q, which is not an event handle", f.Ref, subject)
		}
	}
	// THE PROOF LINE REPORTS THE SHAPES IT READ and does not name the shape it
	// wanted: I wrote "carrying [join, event]" here first, and the mutation
	// control printed that sentence under two failing assertions. The second
	// time in one session, so the rule is now mechanical -- a PROOF line
	// contains measurements and no adjectives.
	shapes := func(fs []ledger.Fact) []string {
		var out []string
		for _, f := range fs {
			out = append(out, fmt.Sprint(f.Handles))
		}
		return out
	}
	t.Logf("PROOF %d join(s) %v; %d create(s) %v",
		len(joins), shapes(joins), len(creates), shapes(creates))
}

// A SUBORNED CLAIM CARRIES THE HANDLES THE HONEST ONE WOULD, ruled by fable at
// coop r5/119: "a statute must be able to group a lie by entity exactly as it
// groups the truth, and a lie distinguishable from the truth by its shape is not
// a lie the control can test".
//
// IT IS THE SAME ARM SHAPE AS THE JOIN-HANDLES ONE and the same fixture, driven
// on a SUBORNED join: `join_event` declares an `event` handle argument, so the
// honest claim carries [join, event] and the planted one must too. The handles
// are DRAWN for a suborn even though nothing ran, which is why this was a
// ruling and not a line -- it spends the generator and moves every walk.
//
// THE SHAPES ARE COMPARED AGAINST EACH OTHER, not against a literal: an arm
// asserting [join, event] twice would pass if both shapes were wrong in the
// same way, while "the lie looks like the truth" is the property ruled.
func TestASubornedClaimLooksLikeTheHonestOne(t *testing.T) {
	ctx := context.Background()
	p := walkProject(t)
	for i := range p.Actions {
		if p.Actions[i].Name == "join_event" {
			p.Actions[i].Args = []bindings.Arg{{Name: "event", Source: bindings.ArgHandle, Of: "event"}}
		}
	}
	// SUBORN HEAVY AND tell OFF, so the walk plants rather than tells: this arm
	// is about the planted claim's shape and nothing else.
	p.Weights = platform.Weights{Default: map[string]int{
		"create_event": 4, "join_event": 3,
		schedule.Suborn: 6, schedule.Canard: 0,
		schedule.Palter: 0, schedule.Essoin: 0, schedule.Tell: 0,
	}}

	sess, err := wire.Open(ctx, fakeSpawn("--id-salt", "suborn"), 20*time.Second)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = sess.Close() }()

	rec, err := schedule.Walk(ctx, p, sess, 20261023, 12, 1)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}

	var planted, honest []ledger.Fact
	for _, f := range rec.Facts {
		if f.Kind != ledger.KindClaim || f.Type != "event.joined" {
			continue
		}
		if f.Planted {
			planted = append(planted, f)
		} else {
			honest = append(honest, f)
		}
	}
	// THE REACH IS ASSERTED FIRST, both halves: without a planted join there is
	// no subject, and without an honest one there is nothing to compare it to.
	if len(planted) == 0 {
		t.Fatalf("no SUBORNED join in 12 steps (kinds %v), so this arm has no subject",
			rec.KindSequence())
	}
	if len(honest) == 0 {
		t.Fatalf("no honest join in 12 steps (kinds %v), so there is no truth for the lie to "+
			"resemble", rec.KindSequence())
	}

	shape := func(f ledger.Fact) string {
		if len(f.Handles) == 0 {
			return "none"
		}
		kinds := make([]string, 0, len(f.Handles))
		for _, h := range f.Handles {
			switch {
			case strings.Contains(h, ".join_event#"):
				kinds = append(kinds, "join")
			case strings.Contains(h, ".event#"):
				kinds = append(kinds, "event")
			default:
				kinds = append(kinds, "other")
			}
		}
		return strings.Join(kinds, ",")
	}

	want := shape(honest[0])
	if want != "join,event" {
		t.Fatalf("the HONEST join's shape is %q and not join,event, so the comparison below is "+
			"against the wrong reference: %v", want, honest[0].Handles)
	}
	for _, f := range planted {
		if got := shape(f); got != want {
			t.Errorf("suborned join %q carries %v (%s) where the honest one carries %v (%s): a "+
				"lie distinguishable from the truth by its SHAPE is not a lie the control can "+
				"test", f.Ref, f.Handles, got, honest[0].Handles, want)
		}
		if f.Handles[0] != f.Ref {
			t.Errorf("suborned join %q carries %v: the MINTED handle comes first, as it does for "+
				"the honest claim", f.Ref, f.Handles)
		}
	}
	t.Logf("PROOF %d planted join(s) %v; %d honest %v", len(planted), planted[0].Handles,
		len(honest), honest[0].Handles)
}

// THE WALK OBSERVES AFTER EVERY STEP, AND OBSERVING MOVES NOTHING. R5.5's two
// halves (fable, coop r5/152): "after EVERY step, every declared observation
// whose actor can make it", and "nothing drawn for it, so the handle sequence
// and D1 are untouched".
//
// THE SECOND HALF IS THE ONE WORTH THE ARM. A walk that observed by DRAWING --
// a handle, a receiver, anything off the Source -- would replay differently
// from the same seed the moment observations were declared, and every seeded
// arm written across R5.1 to R5.4 would quietly mean something else. So this
// drives the same seed twice over two projects differing ONLY in whether
// observations are declared, and requires the handle and kind sequences to be
// identical.
//
// `event` AND `direct` BECAUSE THE SURFACE RECORDS IT: the fake's default
// surface answers `event`, and a direct observation is the engine's own so it
// needs no session -- which keeps this arm about the walk rather than about
// token routing. An observation declaring no args is supportable at every step,
// so the count is the step count exactly.
func TestTheWalkObservesAfterEveryStepAndMovesNothing(t *testing.T) {
	ctx := context.Background()
	const seed = 20261023
	const steps = 6

	// A STATUTE THAT JUDGES THE OBSERVATION CONSISTENT, because this arm is
	// about DRIFT and not about findings. The toy's statute defaults `finding`
	// to `bug` for a query it does not recognise -- which is the statute working
	// -- and a bug ENDS a walk (r5/152 ruling 2), so against it the observing
	// run stops at step 1 and there are no two full sequences to compare. The
	// preconditions are the ones walkProject's two actions name.
	const quiet = `package rumor.statute

default finding := "consistent"

default may_create_event := true

default may_join := true
`
	run := func(obs []bindings.Observation, salt string) schedule.Record {
		t.Helper()
		p := walkProject(t)
		p.Statute = quiet
		p.Observations = obs
		sess, err := wire.Open(ctx, fakeSpawn("--id-salt", salt), 20*time.Second)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer func() { _ = sess.Close() }()
		rec, err := schedule.Walk(ctx, p, sess, seed, steps, 1)
		if err != nil {
			t.Fatalf("Walk (observations=%d): %v", len(obs), err)
		}
		return rec
	}

	without := run(nil, "noobs")
	with := run([]bindings.Observation{{Name: "event", Source: wire.SourceDirect}}, "withobs")

	// THE REACH FIRST: the observing run must actually have observed, or the
	// equality below is two identical walks that both did nothing.
	// THE FINDING GUARD COMES FIRST, because a stopped walk EXPLAINS a short
	// count and the count alone reads as the observe step being broken. My
	// first ordering reported "1 observation over 6 steps" when what had
	// happened was a bug at step 1 ending the run exactly as ruled.
	if with.Finding != nil {
		t.Fatalf("the observing run found %v at step %d, so it stopped early: this arm needs a "+
			"statute that judges the observation consistent", with.Finding.Says(), with.FoundAt)
	}
	if len(with.Steps) != steps {
		t.Fatalf("the observing run took %d of %d step(s)", len(with.Steps), steps)
	}
	if with.Observations != steps {
		t.Fatalf("the walk made %d observation(s) over %d step(s); one declared observation with "+
			"no arguments is supportable at every step, so this arm has no subject",
			with.Observations, steps)
	}
	if without.Observations != 0 {
		t.Fatalf("a project declaring no observations made %d: the control is not a control",
			without.Observations)
	}

	// AND NOTHING WAS APPENDED, which is r5/155's amendment driven rather than
	// trusted: an observation is the check's INPUT and not a fact in the sealed
	// prefix (vocabulary v1, ratified breakout-r2/13), so a walk that observed
	// six times must leave ZERO `observed` facts behind. My first cut appended
	// them and the ledger refused it; this arm is what stops that returning.
	for _, f := range with.Facts {
		if f.Kind == ledger.KindObserved {
			t.Fatalf("the walk appended an `observed` fact at position %d: an observation is the "+
				"check's input and not a fact in the sealed prefix", f.Position)
		}
	}

	// THE PROPERTY: one seed, two projects, identical walks.
	if strings.Join(with.HandleSequence(), ",") != strings.Join(without.HandleSequence(), ",") {
		t.Errorf("declaring an observation moved the HANDLE sequence:\n  without %v\n  with    %v",
			without.HandleSequence(), with.HandleSequence())
	}
	if strings.Join(with.KindSequence(), ",") != strings.Join(without.KindSequence(), ",") {
		t.Errorf("declaring an observation moved the KIND sequence:\n  without %v\n  with    %v",
			without.KindSequence(), with.KindSequence())
	}
	t.Logf("PROOF %d step(s), %d observation(s) with and %d without, one kind sequence %v",
		steps, with.Observations, without.Observations, with.KindSequence())
}

// THE FIRST BUG ENDS THE WALK AND IS CARRIED, which is R5.5's ruling (2)
// (fable, coop r5/152): "the first bug ends the run as a failure and is Shrink's
// Want; consistent and explained walk on".
//
// I MET THIS BY ACCIDENT BEFORE I ARMED IT. The no-drift arm above first ran
// against the toy's statute, which defaults `finding` to `bug` for a query it
// does not recognise, and the observing run stopped at step 1 -- reported to me
// as "1 observation over 6 steps", because that arm checked the COUNT before
// the FINDING. The behaviour was right and the reading was wrong, which is why
// it now has an arm of its own rather than a comment.
//
// THE BUG IS THE PRODUCT, NOT A COULD-NOT-RUN: Walk returns no error, the
// Record carries the Finding whole -- because shrink.Request.Want takes a
// Finding and not a value -- and FoundAt names the step after which it arrived.
// A trace a shrink reads must INCLUDE that step, so the check happens after the
// step is appended to the record.
func TestTheFirstBugEndsTheWalkAndIsCarried(t *testing.T) {
	ctx := context.Background()
	p := walkProject(t)
	// The toy's statute with an observation it does not recognise: `finding`
	// falls to its default, which is `bug`.
	p.Observations = []bindings.Observation{{Name: "event", Source: wire.SourceDirect}}

	sess, err := wire.Open(ctx, fakeSpawn("--id-salt", "firstbug"), 20*time.Second)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = sess.Close() }()

	rec, err := schedule.Walk(ctx, p, sess, 20261023, 6, 1)
	if err != nil {
		t.Fatalf("a bug is the tool's product and must not be an error from Walk: %v", err)
	}

	if rec.Finding == nil {
		t.Fatalf("no finding over %d step(s) and %d observation(s): this arm needs a statute whose "+
			"finding is `bug`, and the toy's default is one", len(rec.Steps), rec.Observations)
	}
	if got := rec.Finding.Says(); got != "bug" {
		t.Errorf("the walk stopped on %q; only `bug` ends a walk, and consistent and explained "+
			"walk on", got)
	}
	if rec.FoundAt != len(rec.Steps) {
		t.Errorf("FoundAt is %d and the trace holds %d step(s): the step that produced the finding "+
			"must be IN the trace a shrink reads", rec.FoundAt, len(rec.Steps))
	}
	if len(rec.Steps) >= 6 {
		t.Errorf("the walk took all %d step(s) despite a bug at step %d: the first bug ends it",
			len(rec.Steps), rec.FoundAt)
	}
	// AND THE RECORD IS COMPLETE AT THE STOP, not half-built: a shrink reads
	// Facts and IDs off it, and a walk that returned early without filling them
	// would hand the shrinker an empty world.
	if len(rec.Facts) == 0 {
		t.Error("the record carries no facts, so a shrink has no world to replay")
	}
	if len(rec.IDs) == 0 {
		t.Error("the record carries no ids, so nothing the walk bound can be resolved")
	}
	t.Logf("PROOF %q at step %d of a 6-step walk; %d step(s) in the trace, %d fact(s), %d id(s)",
		rec.Finding.Says(), rec.FoundAt, len(rec.Steps), len(rec.Facts), len(rec.IDs))
}

// REPLAY RE-RUNS A SEEDED ATTEMPT TO A STEP COUNT WITH KINDS DROPPED, which is
// R5.5's ruling (3) (fable, coop r5/152) and what shrink.Request.Replay will be
// handed by R5.3's third.
//
// THREE CELLS, and the third is the one that makes the other two mean anything.
//
//  1. REPLAYING THE WHOLE TRACE REPRODUCES IT. Same seed, same step count,
//     nothing dropped: the kind sequence is identical to the original walk's.
//     Without this the shrinker would be searching a world that does not match
//     the one that failed.
//
//  2. A SHORTER COUNT IS A PREFIX. Replaying to fewer steps gives the first n
//     kinds of the full trace, which is what a length bisection rests on: if a
//     shorter replay were a DIFFERENT walk, halving the trace would be testing
//     an unrelated world and calling it a reduction.
//
//  3. DROPPING A KIND REMOVES IT AND THE STEPS THAT RAN ARE RETURNED. The kind
//     must be absent from the replay, and the walk must still reach its step
//     count -- a step whose every candidate was dropped records as a no-op
//     (R5.5's sibling ruling) rather than ending the run.
//
// AND A KIND THE PROJECT NEVER WEIGHTED IS REFUSED, because a reduction that
// silently dropped a name that was never there would report "removing it
// changed nothing" about nothing at all.
func TestReplayReproducesShortensAndDropsKinds(t *testing.T) {
	ctx := context.Background()
	const seed = 20261023
	const steps = 6

	// The quiet statute again: this arm is about replay and not about findings,
	// and the toy's `finding` defaults to `bug`, which would end every walk at
	// step 1 and leave nothing to compare.
	const quiet = `package rumor.statute

default finding := "consistent"

default may_create_event := true

default may_join := true
`
	project := func() schedule.Project {
		p := walkProject(t)
		p.Statute = quiet
		return p
	}

	withSession := func(salt string, fn func(*wire.Session)) {
		t.Helper()
		sess, err := wire.Open(ctx, fakeSpawn("--id-salt", salt), 20*time.Second)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer func() { _ = sess.Close() }()
		fn(sess)
	}

	var original []string
	withSession("orig", func(sess *wire.Session) {
		rec, err := schedule.Walk(ctx, project(), sess, seed, steps, 1)
		if err != nil {
			t.Fatalf("the original walk: %v", err)
		}
		original = rec.KindSequence()
	})
	if len(original) != steps {
		t.Fatalf("the original walk took %d of %d step(s), so there is no full trace to replay",
			len(original), steps)
	}

	// CELL 1: the whole trace, nothing dropped.
	withSession("whole", func(sess *wire.Session) {
		_, got, err := schedule.Replay(ctx, project(), sess, seed, 1, steps, nil)
		if err != nil {
			t.Fatalf("replaying the whole trace: %v", err)
		}
		if strings.Join(kindsOf(got), ",") != strings.Join(original, ",") {
			t.Errorf("a full replay walked differently from the original:\n  original %v\n  replay   %v",
				original, kindsOf(got))
		}
	})

	// CELL 2: a shorter count is a prefix.
	withSession("short", func(sess *wire.Session) {
		_, got, err := schedule.Replay(ctx, project(), sess, seed, 1, steps-2, nil)
		if err != nil {
			t.Fatalf("replaying a prefix: %v", err)
		}
		want := original[:steps-2]
		if strings.Join(kindsOf(got), ",") != strings.Join(want, ",") {
			t.Errorf("a %d-step replay is not the first %d of the trace:\n  want %v\n  got  %v",
				steps-2, steps-2, want, kindsOf(got))
		}
	})

	// CELL 3: dropping a kind removes it. join_event is in the original or this
	// cell has no subject, which is asserted rather than assumed.
	if !slices.Contains(original, "join_event") {
		t.Fatalf("the original trace %v draws no join_event, so dropping it proves nothing", original)
	}
	withSession("dropped", func(sess *wire.Session) {
		_, got, err := schedule.Replay(ctx, project(), sess, seed, 1, steps, []string{"join_event"})
		if err != nil {
			t.Fatalf("replaying without join_event: %v", err)
		}
		if slices.Contains(kindsOf(got), "join_event") {
			t.Errorf("join_event was dropped and still drawn: %v", kindsOf(got))
		}
		if len(got) != steps {
			t.Errorf("the replay took %d of %d step(s); a step whose candidates were all dropped "+
				"is a no-op and not the end of the run", len(got), steps)
		}
	})

	// AND A KIND NOTHING WEIGHTS IS REFUSED.
	withSession("unknown", func(sess *wire.Session) {
		if _, _, err := schedule.Replay(ctx, project(), sess, seed, 1, steps,
			[]string{"no_such_kind"}); err == nil {
			t.Error("dropping a kind the project never weighted must refuse: silently succeeding " +
				"would report that removing it changed nothing")
		}
	})
	t.Logf("PROOF original %v; full replay identical; %d-step replay is its prefix; join_event "+
		"dropped leaves %d step(s)", original, steps-2, steps)
}

// kindsOf is a step slice's kind sequence, which Record.KindSequence gives for
// a record and Replay returns steps rather than a record.
func kindsOf(steps []schedule.Step) []string {
	out := make([]string, 0, len(steps))
	for _, s := range steps {
		out = append(out, s.Kind)
	}
	return out
}
