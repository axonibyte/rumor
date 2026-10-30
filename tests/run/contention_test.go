package run_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"forge.axonibyte.com/axonibyte/rumor/internal/concurrency"
	"forge.axonibyte.com/axonibyte/rumor/internal/decl"
	"forge.axonibyte.com/axonibyte/rumor/internal/platform"
	"forge.axonibyte.com/axonibyte/rumor/internal/rollback"
	"forge.axonibyte.com/axonibyte/rumor/internal/rollback/toyrestart"
	"forge.axonibyte.com/axonibyte/rumor/internal/schedule"
	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

// THE OVERFILL CLAUSE AND THE CONTROL THAT HOLDS TOGETHERNESS AND VARIES THE
// DEFECT -- which is rook's correction at coop r8/37, taken.
//
// My drive on the mechanism held the defect and varied TOGETHERNESS: it serialized
// the fan and watched the overlap clause fire. That proves the clause is
// sensitive to togetherness, and rook's objection is exact -- "a clause firing
// on ANY overlapping fan-out would pass it identically". The complement is to
// hold togetherness and vary the defect, and rook drove it by restoring
// joinEvent's capacity re-check inside the window: 1 of 2 landed instead of 2
// of 2, still overlapping by 29.450ms against 30.448ms. Nothing committed,
// because it was a mutation of the toy.
//
// THIS IS THAT CONTROL, COMMITTED, AND IT NEEDS NO MUTATION. cmd/toy already
// contains both readings of one rule:
//
//	joinEvent        checks capacity, DROPS THE LOCK, then inserts
//	acceptInvitation checks capacity and inserts under a HELD lock
//
// Same oracle, same check-then-act shape, same structure, differing in exactly
// the one variable. A fan-out on each at capacity 1 therefore holds
// togetherness constant and varies the defect -- and cmd/toy's own comment
// forbids the alternative: "A REAL WINDOW RATHER THAN A TEST HOOK. A hook the
// test flips would be a fixture override standing in for the production path,
// and this project has been bitten by one."
func TestTheOverfillClauseFiresOnTheRacedActionAndNotOnTheLockedOne(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join("..", "..", "examples", "toy")

	prep, err := platform.Load(filepath.Join(dir, "platform.toml"), decl.TOML)
	if err != nil || !prep.OK() {
		t.Fatalf("platform: %v %v", err, prep.Refusals)
	}
	actors, err := schedule.Actors(prep.Platform.Venire)
	if err != nil {
		t.Fatalf("Actors: %v", err)
	}
	if len(actors) < 3 {
		t.Fatalf("the toy declares %d actor(s); this arm needs a creator and two contenders",
			len(actors))
	}
	creator, first, second := actors[0], actors[1], actors[2]

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

	type party struct {
		sess *wire.Session
		toks *schedule.Sessions
	}
	open := func() party {
		s, oerr := wire.Open(ctx, wire.Spawn{
			Command: prep.Platform.Adapter.Command,
			Cwd:     dir,
			Env:     prep.Platform.Adapter.Env,
		}, 30*time.Second)
		if oerr != nil {
			t.Fatalf("Open: %v", oerr)
		}
		t.Cleanup(func() { _ = s.Close() })
		return party{sess: s, toks: schedule.NewSessions(s)}
	}
	// THREE SESSIONS, THREE PROCESSES: the creator and the two contenders. The
	// contenders need their own, because the wire allows one outstanding
	// request per process and two requests on one session would serialize
	// against each other instead of contending.
	host, a, b := open(), open(), open()

	exec := func(p party, actor, action string, args map[string]any) wire.ExecuteResponse {
		t.Helper()
		tok, terr := p.toks.Token(actor, "api")
		if terr != nil {
			t.Fatalf("token for %s: %v", actor, terr)
		}
		resp, rerr := p.sess.Execute(wire.ExecuteRequest{Action: action, Token: tok, Args: args})
		if rerr != nil {
			t.Fatalf("%s as %s: %v", action, actor, rerr)
		}
		return resp
	}
	register := func(p party, actor string) string {
		t.Helper()
		tok, terr := p.toks.Token(actor, "none")
		if terr != nil {
			t.Fatalf("token for %s: %v", actor, terr)
		}
		resp, rerr := p.sess.Execute(wire.ExecuteRequest{Action: "create_user", Token: tok,
			Args: map[string]any{"name": actor}})
		if rerr != nil {
			t.Fatalf("create_user as %s: %v", actor, rerr)
		}
		id, has := resp.IDs["user"]
		if !has {
			t.Fatalf("create_user as %s carried no user id: %v", actor, resp.Response)
		}
		return fmt.Sprint(id)
	}
	newEvent := func(capacity int) string {
		t.Helper()
		resp := exec(host, creator, "create_event", map[string]any{
			"title": "Standup", "capacity": capacity,
			"visible_to": []any{"admin", "member", "guest"},
		})
		id, has := resp.IDs["event"]
		if !has {
			t.Fatalf("create_event carried no event id: %v", resp.Response)
		}
		return fmt.Sprint(id)
	}
	// ACCEPTED IS THE CALLER's TO DETERMINE, which is why internal/concurrency
	// takes a count: this reads the adapter's `{status, body}` shape, the one
	// cmd/toy-adapter writes on both paths.
	countAccepted := func(as []concurrency.Attempt) int {
		t.Helper()
		n := 0
		for _, att := range as {
			if att.Fault != "" {
				t.Fatalf("session %d faulted, so the fan-out never reached the system: %s",
					att.Session, att.Fault)
			}
			if status, _ := statusOf(t, att.Response); status >= 200 && status < 300 {
				n++
			}
		}
		return n
	}

	_ = register(host, creator)
	firstID := register(a, first)
	secondID := register(b, second)

	plan, perr := concurrency.PlanFor(0, 2)
	if perr != nil {
		t.Fatalf("PlanFor: %v", perr)
	}

	// ---- THE SUBJECT: join_event, which drops the lock inside the window.
	raced := newEvent(1)
	reqA, rerr := joinReq(a.toks, first, raced)
	if rerr != nil {
		t.Fatalf("join request: %v", rerr)
	}
	reqB, rerr := joinReq(b.toks, second, raced)
	if rerr != nil {
		t.Fatalf("join request: %v", rerr)
	}
	joins, ferr := concurrency.Fan(plan, []*wire.Session{a.sess, b.sess},
		[]wire.ExecuteRequest{reqA, reqB})
	if ferr != nil {
		t.Fatalf("Fan: %v", ferr)
	}
	assertPlanDispatched(t, plan, joins)
	if !concurrency.Overlapped(joins) {
		t.Fatalf("the two joins did not overlap, so this run exercised the sequential path: "+
			"%v and %v", joins[0].InFlight(), joins[1].InFlight())
	}
	subject := concurrency.Contention{Action: "join_event", Room: 1,
		Accepted: countAccepted(joins), Width: 2}
	if why := subject.Vacuous(); why != "" {
		t.Fatalf("the subject fan-out cannot answer the clause: %s", why)
	}
	finding := subject.Finding()
	if finding == "" {
		t.Fatalf("join_event accepted %d of 2 into room for 1 and no overfill was reported: the "+
			"clause is silent over the defect it exists for", subject.Accepted)
	}
	t.Logf("PROOF subject: %s", finding)
	t.Logf("PROOF subject %s", concurrency.OverlapMeasured(joins))

	// ---- THE CONTROL: accept_invitation, which holds the lock across the
	// same check-then-act. Same width, same room, same togetherness.
	locked := newEvent(1)
	invA := exec(host, creator, "invite_user", map[string]any{"event": locked, "user": firstID})
	invB := exec(host, creator, "invite_user", map[string]any{"event": locked, "user": secondID})
	idOf := func(resp wire.ExecuteResponse) string {
		t.Helper()
		id, has := resp.IDs["invitation"]
		if !has {
			t.Fatalf("invite_user carried no invitation id: %v", resp.Response)
		}
		return fmt.Sprint(id)
	}
	tokA, terr := a.toks.Token(first, "api")
	if terr != nil {
		t.Fatalf("token: %v", terr)
	}
	tokB, terr := b.toks.Token(second, "api")
	if terr != nil {
		t.Fatalf("token: %v", terr)
	}
	accepts, ferr := concurrency.Fan(plan, []*wire.Session{a.sess, b.sess},
		[]wire.ExecuteRequest{
			{Action: "accept_invitation", Token: tokA, Args: map[string]any{"invitation": idOf(invA)}},
			{Action: "accept_invitation", Token: tokB, Args: map[string]any{"invitation": idOf(invB)}},
		})
	if ferr != nil {
		t.Fatalf("Fan: %v", ferr)
	}
	// TOGETHERNESS IS HELD, and the arm asserts it rather than assuming it:
	// without this the control could pass by having serialized, which is the
	// one way it could be silent for a reason other than the held lock.
	assertPlanDispatched(t, plan, accepts)
	if !concurrency.Overlapped(accepts) {
		t.Fatalf("the two accepts did not overlap, so the control is silent because it was "+
			"sequential and not because the lock is held: %v and %v",
			accepts[0].InFlight(), accepts[1].InFlight())
	}
	control := concurrency.Contention{Action: "accept_invitation", Room: 1,
		Accepted: countAccepted(accepts), Width: 2}
	if why := control.Vacuous(); why != "" {
		t.Fatalf("the control fan-out cannot answer the clause either, so it controls for "+
			"nothing: %s", why)
	}
	if f := control.Finding(); f != "" {
		t.Errorf("accept_invitation holds the lock across its own check-then-act, so an overfill "+
			"here is a SECOND defect and not this clause firing correctly: %s", f)
	}
	t.Logf("PROOF control: accept_invitation accepted %d of 2 into room for 1, no finding, %s",
		control.Accepted, concurrency.OverlapMeasured(accepts))
}

// assertPlanDispatched is rook's cure at coop r8/40, shared by every live
// fan-out arm.
//
// THE MUTANT THAT SURVIVED: `sessions[sess]` -> `sessions[k]` together with
// `Session: sess` -> `Session: k`. The width check does not see it -- a plan of
// the right width is still a plan nothing OBEYS -- and both suites stayed
// green, so the plan could have been decoration. The reason the arms could not
// see it is that they pinned seeds whose plan was the IDENTITY: at width 2
// there are two permutations and SIX OF TWELVE seeds draw [0 1], seeds 1 and 4
// among them, which were the two I had used. A fixture whose plan is the
// identity IS the no-plan case.
//
// SO IT CHECKS BOTH HALVES: that the fixture is a real permutation, and that
// the recorded session is the one the plan chose.
func assertPlanDispatched(t *testing.T, plan concurrency.Plan, attempts []concurrency.Attempt) {
	t.Helper()
	identity := true
	for k, sess := range plan.Issue {
		if sess != k {
			identity = false
			break
		}
	}
	if identity {
		t.Fatalf("the plan is the identity order %v, so this fixture cannot tell a dispatch that "+
			"follows the plan from one that ignores it -- the mutant that survived at r8/40. "+
			"Pick a seed whose plan is a real permutation.", plan.Issue)
	}
	if len(attempts) != plan.Width() {
		t.Fatalf("%d attempt(s) for a plan %d wide", len(attempts), plan.Width())
	}
	for k, att := range attempts {
		if att.Session != plan.Issue[k] {
			t.Errorf("attempt %d was carried by session %d where the plan chose %d: the plan is "+
				"decoration if the dispatch does not follow it", k, att.Session, plan.Issue[k])
		}
	}
}
