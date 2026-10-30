package concurrency

import (
	"path/filepath"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/decl"
)

func toyBindings(t *testing.T) bindings.Bindings {
	t.Helper()
	p := filepath.Join("..", "..", "examples", "toy", "bindings.toml")
	rep, err := bindings.Load(p, decl.TOML)
	if err != nil {
		t.Fatalf("loading %s: %v", p, err)
	}
	if !rep.OK() {
		t.Fatalf("%s is not loadable: %v", p, rep.Refusals)
	}
	return rep.Bindings
}

// THE CHAIN OVER THE SHIPPED TOY, AND THE BOOTSTRAP IS IN IT.
//
// This is the arm that would have caught the first cut: a chain derived from
// `creates` alone holds create_event and NOT create_user, because join_event
// takes an `event` and nothing else -- so the setup's first request would be
// refused with "a session belongs to a user that exists" and the fan-out would
// report a could-not-run about a world it never built. The two halves are
// asserted separately because they come from different readings of the
// declaration, and a single "contains both" check would pass if the bootstrap
// landed in the once-only list.
func TestTheToysChainBootstrapsEveryActorAndRunsTheLimitActionOnce(t *testing.T) {
	b := toyBindings(t)
	subs, err := Subjects(b)
	if err != nil {
		t.Fatalf("Subjects: %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("the toy declares %d subject(s); this arm reads the one", len(subs))
	}
	chain, err := SetupFor(b, subs[0])
	if err != nil {
		t.Fatalf("SetupFor(%s): %v", subs[0].Action.Name, err)
	}
	if len(chain.PerActor) != 1 || chain.PerActor[0].Name != "create_user" {
		t.Errorf("the per-actor bootstrap is %v; the toy's is create_user, whose `name` is the "+
			"ACTOR's own and whose context is `none`", names(chain.PerActor))
	}
	if len(chain.Once) != 1 || chain.Once[0].Name != "create_event" {
		t.Errorf("the once-only chain is %v; join_event takes an `event` and nothing else, so "+
			"create_event is the whole of it", names(chain.Once))
	}
	// AND THE LIMIT IS READ FROM A REQUEST THIS CHAIN SENDS, which is the
	// property SetupFor exists to hold rather than the two lists' contents.
	found := false
	for _, a := range chain.Once {
		if a.Name == subs[0].LimitAction {
			found = true
		}
	}
	if !found {
		t.Errorf("%s reads its limit from %s and the chain runs %v", subs[0].Action.Name,
			subs[0].Action.Contended.Limit, chain.Names())
	}
	t.Logf("PROOF chain for %s: %v", subs[0].Action.Name, chain.Names())
}

// A LIMIT ON AN ACTION THE CHAIN DOES NOT RUN IS REFUSED, and the shape is the
// one that can really happen: two actions create the same kind, the project
// names the SECOND as where its limit is read, and the chain -- which takes the
// first provider in declaration order -- runs the other one. The stage would
// then fan out against a capacity nothing sent.
func TestALimitOnAnActionTheChainDoesNotRunIsRefused(t *testing.T) {
	b := bindings.Bindings{Actions: []bindings.Action{
		{Name: "open_room", Creates: []string{"room"}, Args: []bindings.Arg{
			{Name: "seats", Source: bindings.ArgLiteral, Value: 2}}},
		{Name: "open_big_room", Creates: []string{"room"}, Args: []bindings.Arg{
			{Name: "seats", Source: bindings.ArgLiteral, Value: 9}}},
		{Name: "take_seat", Contended: bindings.Contended{
			Limit: "open_big_room.seats", Held: "room_count"},
			Args: []bindings.Arg{{Name: "room", Source: bindings.ArgHandle, Of: "room"}}},
	}}
	subs, err := Subjects(b)
	if err != nil {
		t.Fatalf("Subjects: %v", err)
	}
	_, err = SetupFor(b, subs[0])
	if err == nil {
		t.Fatal("a limit read from an action the chain never runs returned a chain")
	}
	for _, want := range []string{"open_big_room", "open_room", "take_seat"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not name %q: %v", want, err)
		}
	}
	t.Logf("PROOF %v", err)
}

// A LIMIT THAT IS NOT A VALUE THE SETUP SENDS IS REFUSED. `handle` and `actor`
// carry an id and a name; neither has a number in it to be the room, so a
// stage reading one would fan out against a limit it invented.
func TestALimitOnAHandleArgumentIsRefused(t *testing.T) {
	b := bindings.Bindings{Actions: []bindings.Action{
		{Name: "open_room", Creates: []string{"room"}, Args: []bindings.Arg{
			{Name: "owner", Source: bindings.ArgHandle, Of: "user"}}},
		{Name: "create_user", Creates: []string{"user"}},
		{Name: "take_seat", Contended: bindings.Contended{
			Limit: "open_room.owner", Held: "room_count"},
			Args: []bindings.Arg{{Name: "room", Source: bindings.ArgHandle, Of: "room"}}},
	}}
	subs, err := Subjects(b)
	if err != nil {
		t.Fatalf("Subjects: %v", err)
	}
	_, err = SetupFor(b, subs[0])
	if err == nil {
		t.Fatal("a limit read from a handle argument returned a chain")
	}
	if !strings.Contains(err.Error(), "handle") {
		t.Errorf("the refusal does not name the source: %v", err)
	}
	t.Logf("PROOF %v", err)
}

// AN ARGUMENT THE LIMIT ACTION DOES NOT DECLARE IS REFUSED, AND THE REFUSAL
// LISTS THE ONES IT DOES. `rumor validate` refuses this before a run as a cell
// of arm 19, and this is the re-check for a run that was not validated first.
func TestALimitArgumentTheActionDoesNotDeclareIsRefused(t *testing.T) {
	b := bindings.Bindings{Actions: []bindings.Action{
		{Name: "open_room", Creates: []string{"room"}, Args: []bindings.Arg{
			{Name: "seats", Source: bindings.ArgLiteral, Value: 2},
			{Name: "title", Source: bindings.ArgLiteral, Value: "Standup"}}},
		{Name: "take_seat", Contended: bindings.Contended{
			Limit: "open_room.chairs", Held: "room_count"},
			Args: []bindings.Arg{{Name: "room", Source: bindings.ArgHandle, Of: "room"}}},
	}}
	subs, err := Subjects(b)
	if err != nil {
		t.Fatalf("Subjects: %v", err)
	}
	_, err = SetupFor(b, subs[0])
	if err == nil {
		t.Fatal("a limit naming an undeclared argument returned a chain")
	}
	if !strings.Contains(err.Error(), "chairs") || !strings.Contains(err.Error(), "seats") {
		t.Errorf("the refusal names neither the missing argument nor the declared ones: %v", err)
	}
	t.Logf("PROOF %v", err)
}

// AN UNDECLARED LIMIT ACTION IS REFUSED BY NAME rather than resolved to
// nothing: the chain would otherwise be built, run, and contend against a
// limit that was never sent.
func TestAnUndeclaredLimitActionIsRefusedByName(t *testing.T) {
	b := bindings.Bindings{Actions: []bindings.Action{
		{Name: "open_room", Creates: []string{"room"}},
		{Name: "take_seat", Contended: bindings.Contended{
			Limit: "book_room.seats", Held: "room_count"},
			Args: []bindings.Arg{{Name: "room", Source: bindings.ArgHandle, Of: "room"}}},
	}}
	subs, err := Subjects(b)
	if err != nil {
		t.Fatalf("Subjects: %v", err)
	}
	_, err = SetupFor(b, subs[0])
	if err == nil {
		t.Fatal("a limit on an undeclared action returned a chain")
	}
	if !strings.Contains(err.Error(), "book_room") {
		t.Errorf("the refusal does not name the action: %v", err)
	}
	t.Logf("PROOF %v", err)
}

// A BOOTSTRAP WITHOUT AN ACTOR ARGUMENT IS NOT RUN PER ACTOR, which is the
// condition's second half and the one that is easy to drop. An action whose
// request is identical for every actor would be sent N times, and a creating
// action answers the second with a refusal -- so running it per actor would
// manufacture a setup failure rather than build a world.
func TestABootstrapWithNoActorArgumentRunsOnceAndNotPerActor(t *testing.T) {
	b := bindings.Bindings{Actions: []bindings.Action{
		{Name: "open_season", Context: "none", Creates: []string{"season"}},
		{Name: "open_room", Context: "api", Creates: []string{"room"},
			Args: []bindings.Arg{
				{Name: "season", Source: bindings.ArgHandle, Of: "season"},
				{Name: "seats", Source: bindings.ArgLiteral, Value: 2}}},
		{Name: "take_seat", Contended: bindings.Contended{
			Limit: "open_room.seats", Held: "room_count"},
			Args: []bindings.Arg{{Name: "room", Source: bindings.ArgHandle, Of: "room"}}},
	}}
	subs, err := Subjects(b)
	if err != nil {
		t.Fatalf("Subjects: %v", err)
	}
	chain, err := SetupFor(b, subs[0])
	if err != nil {
		t.Fatalf("SetupFor: %v", err)
	}
	if len(chain.PerActor) != 0 {
		t.Errorf("open_season declares no `actor` argument and was put in the per-actor list: %v",
			names(chain.PerActor))
	}
	if got := names(chain.Once); len(got) != 2 || got[0] != "open_season" || got[1] != "open_room" {
		t.Errorf("the once-only chain is %v; it is topological, so the season precedes the room", got)
	}
	t.Logf("PROOF once-only %v, per actor %v", names(chain.Once), names(chain.PerActor))
}

func names(as []bindings.Action) []string {
	out := make([]string, 0, len(as))
	for _, a := range as {
		out = append(out, a.Name)
	}
	return out
}

// A HANDLE THAT IS NOT THE CONTENDED THING IS REFUSED, because the fan-out
// sends ONE set of arguments to every contender.
//
// THE HAZARD IS A FALSE CLEAN AND NOT A CRASH, which is why it is worth a
// refusal. `accept_invitation` is the shape: its handle is an `invitation`, its
// limit would be the event's capacity, and one invitation sent to two
// contenders has the second answered "already accepted" -- so accepted is 1 of
// 2 into room for 1, which is neither a finding nor vacuous by any of
// Contention's four conditions. The stage would report `no overfill` over a
// world in which only one request could ever have succeeded: a control that is
// the case it controls for, arriving through a door the vacuity clause does not
// watch.
//
// BOTH SIDES ARE DRIVEN, because a refusal that fired on the shipped toy would
// take the stage's only subject with it.
func TestAHandleThatIsNotTheContendedThingIsRefused(t *testing.T) {
	// THE SHIPPED TOY PASSES: join_event takes an `event` and create_event --
	// the action its limit is read from -- creates one.
	b := toyBindings(t)
	subs, err := Subjects(b)
	if err != nil {
		t.Fatalf("Subjects: %v", err)
	}
	if _, err := SetupFor(b, subs[0]); err != nil {
		t.Fatalf("the toy's own subject was refused by this check: %v", err)
	}

	// AND THE PER-CONTENDER SHAPE IS REFUSED, naming the kind and what the
	// limit's action does create.
	two := bindings.Bindings{Actions: []bindings.Action{
		{Name: "open_room", Creates: []string{"room"}, Args: []bindings.Arg{
			{Name: "seats", Source: bindings.ArgDraw, From: []any{1, 2}}}},
		{Name: "issue_pass", Creates: []string{"pass"}, Args: []bindings.Arg{
			{Name: "room", Source: bindings.ArgHandle, Of: "room"}}},
		{Name: "take_seat", Contended: bindings.Contended{
			Limit: "open_room.seats", Held: "room_count"},
			Args: []bindings.Arg{
				{Name: "room", Source: bindings.ArgHandle, Of: "room"},
				{Name: "pass", Source: bindings.ArgHandle, Of: "pass"}}},
	}}
	tsubs, err := Subjects(two)
	if err != nil {
		t.Fatalf("Subjects: %v", err)
	}
	_, err = SetupFor(two, tsubs[0])
	if err == nil {
		t.Fatal("a subject taking a per-contender handle returned a chain, so the stage would " +
			"send one pass to every contender and read the refusals as a clean limit")
	}
	for _, want := range []string{"take_seat", `"pass"`, "open_room", "[room]"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not name %q: %v", want, err)
		}
	}
	t.Logf("PROOF %v", err)
}
