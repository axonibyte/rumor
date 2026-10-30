package run_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/concurrency"
	"forge.axonibyte.com/axonibyte/rumor/internal/decl"
	"forge.axonibyte.com/axonibyte/rumor/internal/fuzz"
	"forge.axonibyte.com/axonibyte/rumor/internal/platform"
	"forge.axonibyte.com/axonibyte/rumor/internal/rollback"
	"forge.axonibyte.com/axonibyte/rumor/internal/rollback/toyrestart"
	"forge.axonibyte.com/axonibyte/rumor/internal/schedule"
	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

// THE FUZZ STAGE AGAINST THE LIVE TOY, which is D2's subject and D5's "a live
// server". TestMain builds `toy` and `toy-adapter` onto PATH here, and
// toy-adapter spawns `toy` itself -- so this is the real pairing and not a
// fake surface.
//
// WHY THE ARM LIVES IN tests/run AND NOT internal/fuzz: internal/fuzz's own
// arms are over the DERIVATION, which needs no server, and an arm that spawned
// one from a unit package would make every `go test ./internal/...` depend on
// two binaries being built first. This suite already builds them.
func TestTheFuzzStageDrivesTheToyAndFindsNo5xx(t *testing.T) {
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

	sess, err := wire.Open(ctx, wire.Spawn{
		Command: prep.Platform.Adapter.Command,
		Cwd:     dir,
		Env:     prep.Platform.Adapter.Env,
	}, 30*time.Second)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = sess.Close() }()

	rep, err := fuzz.Run(ctx, brep.Bindings, sess, actors[0])
	if err != nil {
		t.Fatalf("fuzz.Run: %v", err)
	}

	// THE SETUP RAN THE DERIVED ORDER, not a hand-written one.
	if len(rep.Setup) == 0 {
		t.Fatal("the stage ran no setup, so every handle case sent an id it never obtained")
	}

	// D2: NO 5xx over the whole corpus. This is the clause, and the toy
	// passing it is the finding's absence rather than the arm's vacuity --
	// which the next assertion is for.
	if found := rep.Findings(); len(found) != 0 {
		for _, f := range found {
			t.Errorf("finding: %s", f)
		}
	}

	// AND THE CORPUS IS ACTUALLY HOSTILE, which is the half that keeps the
	// clause from being satisfied by a stage nothing refused: a corpus every
	// case of which the system ACCEPTED would report no 5xx and have tested
	// nothing about refusals.
	refused, accepted, faulted := 0, 0, 0
	for _, r := range rep.Results {
		switch {
		case r.Fault != "":
			faulted++
		case r.Status >= 400 && r.Status < 500:
			refused++
		case r.Status >= 200 && r.Status < 300:
			accepted++
		}
	}
	if refused == 0 {
		t.Error("not one case was refused with a 4xx: the corpus is not hostile to this system")
	}
	// EVERY ANSWER IS COUNTED IN EXACTLY ONE BAND, so a status outside them --
	// a 3xx, or a zero with no fault -- cannot sit in the report unexamined
	// while the three counts still look plausible.
	if refused+accepted+faulted != len(rep.Results) {
		t.Errorf("%d result(s) but %d refused + %d accepted + %d faulted: an answer went uncounted",
			len(rep.Results), refused, accepted, faulted)
	}
	// THE DENOMINATOR IS PINNED, which is rook's addition to D2 at coop r8/20
	// on this arm's own first figures. "No 5xx over 45" is a claim about the
	// 27 that REACHED: the other 18 were refused by the adapter's argument
	// check before the toy saw a request. The two numbers are asserted
	// together because a reached count that FALLS while the corpus GROWS is
	// itself the finding -- more cases arriving at the adapter and fewer at
	// the system means the new ones test the adapter. Moving either figure is
	// a deliberate act, the way the mutant counts are.
	if got, want := rep.Reached(), 27; got != want {
		t.Errorf("%d of %d case(s) reached the system, expected %d: a reached count that falls "+
			"while the corpus grows means the new cases are testing the adapter, not the toy",
			got, len(rep.Results), want)
	}
	if got, want := len(rep.Results), 45; got != want {
		t.Errorf("the corpus sent %d case(s), expected %d", got, want)
	}
	if rep.Reached() != refused+accepted {
		t.Errorf("%d reached but %d refused + %d accepted: a reached case went uncounted",
			rep.Reached(), refused, accepted)
	}
	// THE FOUR LAYERS, PINNED. "No 5xx" and "every refusal names a field" are
	// both satisfied by answers the SYSTEM never produced: 18 of these cases
	// never left the adapter, and 5 more were refused by encoding/json inside
	// the toy's own decodeBody before a handler ran -- and Go's unmarshal
	// error carries the struct field, so those five "name the field" without
	// the toy having a rule about them. Only the 21 are the system
	// discriminating. Pinning all four is what keeps a clause from being
	// credited to the wrong layer.
	origins := map[fuzz.Origin]int{}
	for _, r := range rep.Results {
		origins[r.Origin()]++
	}
	for _, want := range []struct {
		o fuzz.Origin
		n int
		s string
	}{
		{fuzz.OriginAdapter, 18, "refused by the adapter before the system saw them"},
		{fuzz.OriginDecoder, 5, "refused by the decoder before a rule ran"},
		{fuzz.OriginSystem, 21, "refused by a rule the system wrote"},
		{fuzz.OriginAccepted, 1, "accepted"},
		{fuzz.OriginUnread, 0, "unclassifiable"},
	} {
		if origins[want.o] != want.n {
			t.Errorf("%d case(s) %s, expected %d: a clause credited to the wrong layer "+
				"proves nothing about this system", origins[want.o], want.s, want.n)
		}
	}

	// D4's OTHER HALF: the system's own refusals must be MORE THAN ONE
	// sentence. A monoculture passes "names a field" and discriminates
	// nothing; the count is the only thing that tells them apart.
	msgs := map[string]bool{}
	for _, r := range rep.SystemRefusals() {
		msgs[r.Error] = true
	}
	if len(msgs) < 2 {
		t.Errorf("the system's %d refusal(s) carry %d distinct message(s): a monoculture",
			len(rep.SystemRefusals()), len(msgs))
	}
	if got, want := len(msgs), 8; got != want {
		t.Errorf("%d distinct message(s) from the system's own rules, expected %d", got, want)
	}

	t.Logf("PROOF setup %v; %s", rep.Setup, rep.Summary())
	t.Logf("PROOF by layer: %d adapter, %d decoder, %d system, %d accepted; %d distinct system message(s)",
		origins[fuzz.OriginAdapter], origins[fuzz.OriginDecoder], origins[fuzz.OriginSystem],
		origins[fuzz.OriginAccepted], len(msgs))
	t.Logf("PROOF %d refused, %d accepted, %d faulted of %d sent",
		refused, accepted, faulted, len(rep.Results))

	// EVERY REFUSAL NAMES SOMETHING, which is D4's subject and is measured
	// here rather than asserted: a 4xx with an empty body says the system
	// refused and nothing about what it objected to.
	silent := 0
	for _, r := range rep.Results {
		if r.Status >= 400 && r.Status < 500 && strings.TrimSpace(r.Error) == "" {
			silent++
		}
	}
	t.Logf("PROOF %d of %d refusals carry no error text", silent, refused)
}

// D5's PRESENCE PARTNER: a run in which nothing reached the system must not be
// reported as having found no 5xx.
//
// WHY THIS NEEDS THE LIVE SESSION AND NOT A FAKE: the clause this drives is
// Report.Vacuous, and the half worth proving is that fuzz.Run CONSULTS it.
// Run takes a *wire.Session, so there is no interface to substitute -- and a
// unit arm over Vacuous alone would pass with the call site deleted, which is
// the one failure this arm exists to catch.
//
// HOW ZERO REACHED IS PRODUCED HONESTLY: the corpus is the actions that emit,
// so stripping Emits from every declared action and appending ONE write action
// the adapter has never heard of leaves a corpus whose every case faults at the
// adapter. The replacement needs no handle, so the derived setup is empty and
// the run gets as far as sending -- which is the state a server that died after
// its setup would leave behind.
func TestTheFuzzStageRefusesToClearASystemThatNeverAnsweredIt(t *testing.T) {
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

	b := brep.Bindings
	stripped := make([]bindings.Action, 0, len(b.Actions)+1)
	for _, a := range b.Actions {
		a.Emits = nil
		stripped = append(stripped, a)
	}
	const absent = "an_action_the_adapter_does_not_carry"
	stripped = append(stripped, bindings.Action{
		Name:    absent,
		Context: "none",
		Emits:   []string{"anything"},
		Args:    []bindings.Arg{{Name: "title", Source: "literal", Value: "a title"}},
	})
	b.Actions = stripped

	sess, err := wire.Open(ctx, wire.Spawn{
		Command: prep.Platform.Adapter.Command,
		Cwd:     dir,
		Env:     prep.Platform.Adapter.Env,
	}, 30*time.Second)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = sess.Close() }()

	rep, rerr := fuzz.Run(ctx, b, sess, actors[0])
	if rerr == nil {
		t.Fatalf("the stage returned no error over %d case(s) of which %d reached: a corpus the "+
			"system never saw was reported as a clean run", len(rep.Results), rep.Reached())
	}
	if rep.Reached() != 0 {
		t.Fatalf("this arm's premise is gone: %d case(s) reached the system, so the error above is "+
			"about something other than the vacuity this drives (%v)", rep.Reached(), rerr)
	}
	if len(rep.Results) == 0 {
		t.Fatalf("no case was sent at all, so the corpus -- not the answer -- is what failed: %v", rerr)
	}
	if !strings.Contains(rerr.Error(), "reached the system") {
		t.Errorf("the refusal does not say what was wrong with the run: %v", rerr)
	}

	// AND THE FINDINGS ARE NOT WHERE THIS LANDED, because a request the system
	// never received is not a defect in the system.
	if found := rep.Findings(); len(found) != 0 {
		t.Errorf("a run that reached nothing produced %d finding(s) against the system: %v",
			len(found), found)
	}

	t.Logf("PROOF %d case(s) sent, %d reached, refused with: %v", len(rep.Results), rep.Reached(), rerr)
}

// THE FAN-OUT MECHANISM, AGAINST ONE PROVIDER-OWNED TOY AND TWO ADAPTERS.
//
// 109's whole premise is that two requests can be in flight against ONE system
// at once, and two things have to be true for that which are easy to get wrong
// and silent when wrong. The wire allows one outstanding request per process,
// so a fan-out is N SESSIONS; and cmd/toy-adapter starts a toy of its OWN when
// TOY_ADDR is absent, so two adapters spawned carelessly contend on nothing --
// two requests, two worlds, both succeeding, and a capacity race reported as
// absent.
//
// SO THIS ARM PROVES ONE WORLD BEFORE IT PROVES ANY TIMING. The second actor
// joins the event the FIRST actor created: against two toys that join is a 404,
// because the second adapter's world has no such event.
//
// AND IT DOES NOT DEPEND ON THE DEFECT. The spans are measured around Execute,
// client side, so two requests are "in flight together" whether the server
// interleaves them or serializes them under a held lock -- which is what keeps
// this arm about the mechanism and leaves the race to the oracle.
func TestTheFanOutReachesOneWorldThroughTwoAdapters(t *testing.T) {
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
	if len(actors) < 2 {
		t.Fatalf("the toy declares %d actor(s); a fan-out needs two distinct ones", len(actors))
	}

	// ONE TOY, OWNED BY THE PROVIDER, which is the mode `[adapter].env` and
	// TOY_ADDR exist for. tests/shrink opens it the same way.
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
		t.Fatalf("%s does not expose the address it bound, so the adapters cannot be pointed at "+
			"one world", toyrestart.Name)
	}
	// t.Setenv AND NOT os.Setenv, which cost the whole suite once: a package's
	// tests share one process, so an os.Setenv here leaked TOY_ADDR into every
	// later test in tests/run, and once this arm's RemoveWorld had stopped the
	// toy they all dialled a dead address -- eleven arms failing with
	// "connection refused" against a world this one had removed. t.Setenv
	// restores the previous value when the test ends. Found by running the
	// WHOLE suite; `-run` over this arm alone passed and showed nothing.
	t.Setenv("TOY_ADDR", at.Addr())

	open := func() (*wire.Session, *schedule.Sessions) {
		s, oerr := wire.Open(ctx, wire.Spawn{
			Command: prep.Platform.Adapter.Command,
			Cwd:     dir,
			Env:     prep.Platform.Adapter.Env,
		}, 30*time.Second)
		if oerr != nil {
			t.Fatalf("Open: %v", oerr)
		}
		return s, schedule.NewSessions(s)
	}
	sessA, tokA := open()
	defer func() { _ = sessA.Close() }()
	sessB, tokB := open()
	defer func() { _ = sessB.Close() }()

	do := func(s *wire.Session, toks *schedule.Sessions, actor, action, context string,
		args map[string]any) wire.ExecuteResponse {
		t.Helper()
		tok, terr := toks.Token(actor, context)
		if terr != nil {
			t.Fatalf("token for %s/%s: %v", actor, context, terr)
		}
		resp, rerr := s.Execute(wire.ExecuteRequest{Action: action, Token: tok, Args: args})
		if rerr != nil {
			t.Fatalf("%s as %s: %v", action, actor, rerr)
		}
		return resp
	}

	// Each actor registers on its OWN session, then the first creates an event
	// with room for both joins.
	do(sessA, tokA, actors[0], "create_user", "none", map[string]any{"name": actors[0]})
	do(sessB, tokB, actors[1], "create_user", "none", map[string]any{"name": actors[1]})
	ev := do(sessA, tokA, actors[0], "create_event", "api", map[string]any{
		"title": "Standup", "capacity": 3, "visible_to": []any{"admin", "member", "guest"},
	})
	id, hasID := ev.IDs["event"]
	if !hasID {
		t.Fatalf("create_event carried no event id, so there is nothing for the second adapter "+
			"to find: %v", ev.Response)
	}
	eventID := fmt.Sprint(id)

	// THE FAN-OUT. Both joins are legal against a capacity of 3, so nothing
	// here rests on the race.
	reqA, rerr := joinReq(tokA, actors[0], eventID)
	if rerr != nil {
		t.Fatalf("join request for %s: %v", actors[0], rerr)
	}
	reqB, rerr := joinReq(tokB, actors[1], eventID)
	if rerr != nil {
		t.Fatalf("join request for %s: %v", actors[1], rerr)
	}
	attempts, ferr := concurrency.Fan([]*wire.Session{sessA, sessB},
		[]wire.ExecuteRequest{reqA, reqB})
	if ferr != nil {
		t.Fatalf("Fan: %v", ferr)
	}

	// ONE WORLD: the second adapter found the first's event. Against two toys
	// this is a 404.
	for _, a := range attempts {
		if a.Fault != "" {
			t.Fatalf("session %d faulted, so the fan-out never reached the system: %s",
				a.Session, a.Fault)
		}
		status, errText := statusOf(t, a.Response)
		if status == 404 {
			t.Fatalf("session %d was told %q: the adapters are not sharing a world, so a "+
				"fan-out would contend on nothing and a race would read as absent",
				a.Session, errText)
		}
		if status < 200 || status > 299 {
			t.Fatalf("session %d answered %d (%s); both joins are legal against a capacity of 3, "+
				"so this arm's premise is gone", a.Session, status, errText)
		}
	}

	// AND THEY WERE IN FLIGHT TOGETHER. A serialized fan-out exercises the
	// sequential path and would report a race as absent.
	overlapped, by := concurrency.Overlapped(attempts)
	if !overlapped {
		t.Fatalf("the two requests did not overlap (%v and %v), so this run exercised the "+
			"sequential path and could not have reached a check-then-act defect",
			attempts[0].InFlight(), attempts[1].InFlight())
	}
	t.Logf("PROOF one world through two adapters, both joins accepted; overlapped by %v "+
		"(in flight %v and %v)", by, attempts[0].InFlight(), attempts[1].InFlight())
}

// joinReq builds one honest join_event request for an actor's api session.
func joinReq(toks *schedule.Sessions, actor, eventID string) (wire.ExecuteRequest, error) {
	tok, err := toks.Token(actor, "api")
	if err != nil {
		return wire.ExecuteRequest{}, err
	}
	return wire.ExecuteRequest{Action: "join_event", Token: tok,
		Args: map[string]any{"event": eventID}}, nil
}

// statusOf reads the adapter's `{status, body}` answer the way the fuzz stage
// does, which is the shape cmd/toy-adapter writes on both paths.
func statusOf(t *testing.T, resp wire.ExecuteResponse) (int, string) {
	t.Helper()
	m, isMap := resp.Response.(map[string]any)
	if !isMap {
		t.Fatalf("the adapter answered %v, which is not a table", resp.Response)
	}
	var status int
	switch v := m["status"].(type) {
	case float64:
		status = int(v)
	case int:
		status = v
	case int64:
		status = int(v)
	}
	var msg string
	if body, isBody := m["body"].(map[string]any); isBody {
		if e, has := body["error"]; has {
			msg = fmt.Sprint(e)
		}
	}
	return status, msg
}
