// Actor naming and flow-handle resolvability, both ruled at coop r5/58.
package schedule_test

import (
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/decl"
	"forge.axonibyte.com/axonibyte/rumor/internal/handle"
	"forge.axonibyte.com/axonibyte/rumor/internal/platform"
	"forge.axonibyte.com/axonibyte/rumor/internal/schedule"
)

// <role>_<n>, n FROM 1 PER ROLE, IN VENIRE ORDER. The order is part of the
// ruling and not incidental: handles embed the actor, so two attempts from one
// seed must agree on who member_2 is before either draws anything.
func TestActorsAreNamedFromTheVenireInOrder(t *testing.T) {
	got, err := schedule.Actors([]platform.Member{
		{Role: "admin", Count: 1},
		{Role: "member", Count: 2},
		{Role: "guest", Count: 1},
	})
	if err != nil {
		t.Fatalf("Actors: %v", err)
	}
	want := []string{"admin_1", "member_1", "member_2", "guest_1"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("actors %v, want %v in venire order", got, want)
	}

	// EVERY NAME MUST CARRY A HANDLE, which is why the hyphen was ruled out:
	// the vocabulary's shape refuses it, and an actor whose handles cannot be
	// formed would fail later as an unparseable handle rather than here.
	for _, a := range got {
		if !handle.Valid(a + ".event#1") {
			t.Errorf("actor %q cannot carry a run-local handle", a)
		}
	}
}

// THE TOY'S OWN VENIRE, so the naming is exercised against the corpus and not
// only against a literal I wrote.
func TestTheToysVenireNamesFourActors(t *testing.T) {
	rep, err := platform.Load(toy+"/platform.toml", decl.TOML)
	if err != nil || !rep.OK() {
		t.Fatalf("the toy's platform: %v %v", err, rep.Refusals)
	}
	got, err := schedule.Actors(rep.Platform.Venire)
	if err != nil {
		t.Fatalf("Actors on the toy's venire: %v", err)
	}
	want := []string{"admin_1", "member_1", "member_2", "guest_1"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("the toy's venire gives %v, want %v", got, want)
	}
}

// THE REFUSALS, each the world where naming cannot be done rather than done
// badly. A run with no actors, a role nobody holds, and a role whose second
// numbering would collide with the first's.
func TestActorsRefusesWhatCannotBeNamed(t *testing.T) {
	for _, c := range []struct {
		label  string
		venire []platform.Member
		says   string
	}{
		{"no venire at all", nil, "no [[venire]]"},
		{"a role with no name", []platform.Member{{Count: 2}}, "declares no role"},
		{"a role nobody holds", []platform.Member{{Role: "member", Count: 0}}, "count of 0"},
		{"a negative count", []platform.Member{{Role: "member", Count: -1}}, "count of -1"},
		{"the same role twice", []platform.Member{
			{Role: "member", Count: 1}, {Role: "member", Count: 1},
		}, "appears twice"},
		{"a role the handle shape refuses", []platform.Member{{Role: "Member", Count: 1}}, "handle shape"},
	} {
		_, err := schedule.Actors(c.venire)
		if err == nil {
			t.Errorf("%s: must be refused", c.label)
			continue
		}
		if !strings.Contains(err.Error(), c.says) {
			t.Errorf("%s: the refusal must say %q; got %v", c.label, c.says, err)
		}
	}
}

// FLOW PLACEHOLDERS ARE READ FROM THE CORPUS, and the toy's join_event_ui is
// the one action that has any -- `/ui/events/{event}`.
func TestFlowPlaceholdersComeOffTheCorpus(t *testing.T) {
	var found string
	for _, a := range toyActions(t) {
		ph := schedule.FlowPlaceholders(a)
		if len(ph) == 0 {
			continue
		}
		if found != "" {
			t.Fatalf("two toy actions declare flow placeholders (%s and %s); this arm assumes one "+
				"and its expectation below is no longer the whole truth", found, a.Name)
		}
		found = a.Name
		if strings.Join(ph, ",") != "event" {
			t.Errorf("%s's flow needs %v, want [event] from /ui/events/{event}", a.Name, ph)
		}
	}
	if found == "" {
		t.Fatal("no toy action declares a flow placeholder, so every resolvability arm in this " +
			"package is measuring an empty set")
	}
}

// AN ACTION THAT NAMES NO HANDLE NEEDS NOTHING. This arm's comment used to say
// a flow reference was the ONLY way an action names a handle, which R5.4's
// `[[action.args]]` made false: an argument with `source = "handle"` is the
// other way, and the arm below drives it.
func TestResolvableIsTrueWhereNothingIsNeeded(t *testing.T) {
	ok, kind := schedule.Resolvable(bindings.Action{Name: "create_event"}, nil)
	if !ok {
		t.Errorf("an action with no flow and no arguments needs no handle; got a complaint "+
			"about %q", kind)
	}

	// AND AN ARGUMENT THAT IS NOT A HANDLE STILL NEEDS NOTHING: a literal, a
	// draw and the acting actor's name are all available at every step, so an
	// action declaring only those must not be held out of the pool.
	only := bindings.Action{Name: "create_event", Args: []bindings.Arg{
		{Name: "title", Source: bindings.ArgLiteral, Value: "Standup"},
		{Name: "capacity", Source: bindings.ArgDraw, From: []any{int64(1)}},
		{Name: "name", Source: bindings.ArgActor},
	}}
	if ok, kind := schedule.Resolvable(only, nil); !ok {
		t.Errorf("literal, draw and actor arguments need no handle; got a complaint about %q", kind)
	}
}

// A `handle` ARGUMENT IS THE SECOND WAY AN ACTION NAMES A HANDLE, and it is
// counted. Before R5.4 an action declaring one and no flow read as needing
// nothing, so the walk would draw it and ExecuteArgs would then refuse the
// step -- the very "drawn and then refused" shape Resolvable exists to prevent
// (coop r5/58).
func TestResolvableCountsAHandleArgument(t *testing.T) {
	a := bindings.Action{Name: "join_event", Args: []bindings.Arg{
		{Name: "event", Source: bindings.ArgHandle, Of: "event"},
	}}

	if ok, kind := schedule.Resolvable(a, nil); ok || kind != "event" {
		t.Errorf("with no table: ok=%v kind=%q, want false and event", ok, kind)
	}

	tab, err := handle.New(1)
	if err != nil {
		t.Fatalf("handle.New: %v", err)
	}
	if err := tab.Bind("member_1.user#1", "u-1"); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	// A BOUND HANDLE OF ANOTHER KIND DOES NOT SATISFY IT, the control that
	// keeps this from passing on any non-empty table.
	if ok, kind := schedule.Resolvable(a, tab); ok || kind != "event" {
		t.Errorf("a bound USER handle must not satisfy an argument needing an event: ok=%v kind=%q",
			ok, kind)
	}
	if err := tab.Bind("member_2.event#1", "e-1"); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if ok, kind := schedule.Resolvable(a, tab); !ok {
		t.Errorf("a bound event handle must satisfy it; got a complaint about %q", kind)
	}
}

// THE ARGUMENT IS NAMED BEFORE THE FLOW when both are unresolved, because an
// unwritable request stops the step sooner than an unnavigable page.
func TestAnArgumentsKindIsNamedBeforeAFlows(t *testing.T) {
	a := bindings.Action{
		Name: "join_event_ui",
		Args: []bindings.Arg{{Name: "user", Source: bindings.ArgHandle, Of: "user"}},
		Flow: []bindings.Step{{Step: "navigate", To: "/ui/events/{event}"}},
	}
	if got := schedule.HandleKindsNeeded(a); len(got) != 2 || got[0] != "user" || got[1] != "event" {
		t.Errorf("HandleKindsNeeded = %v, want [user event]", got)
	}
	if ok, kind := schedule.Resolvable(a, nil); ok || kind != "user" {
		t.Errorf("ok=%v kind=%q, want false and the argument's kind user", ok, kind)
	}
}

// AND A KIND NAMED TWICE IS NEEDED ONCE: an action whose argument and whose
// flow both refer to `event` must not report it twice, or a caller listing what
// is missing would say the same thing twice.
func TestAKindNeededTwiceIsListedOnce(t *testing.T) {
	a := bindings.Action{
		Name: "join_event_ui",
		Args: []bindings.Arg{{Name: "event", Source: bindings.ArgHandle, Of: "event"}},
		Flow: []bindings.Step{{Step: "navigate", To: "/ui/events/{event}"}},
	}
	if got := schedule.HandleKindsNeeded(a); len(got) != 1 || got[0] != "event" {
		t.Errorf("HandleKindsNeeded = %v, want [event] exactly once", got)
	}
}

// AND NAMES THE KIND IT CANNOT RESOLVE, so a walk's refusal can say what is
// missing rather than that something is.
func TestResolvableNamesTheUnresolvedKind(t *testing.T) {
	a := bindings.Action{Name: "join_event_ui", Flow: []bindings.Step{{Step: "navigate", To: "/ui/events/{event}"}}}

	ok, kind := schedule.Resolvable(a, nil)
	if ok || kind != "event" {
		t.Errorf("with no table at all: ok=%v kind=%q, want false and event", ok, kind)
	}

	empty, err := handle.New(1)
	if err != nil {
		t.Fatalf("handle.New: %v", err)
	}
	if ok, kind := schedule.Resolvable(a, empty); ok || kind != "event" {
		t.Errorf("with nothing bound: ok=%v kind=%q, want false and event", ok, kind)
	}

	// A HANDLE OF ANOTHER KIND IS NOT THE ONE NEEDED. Without this cell a
	// Resolvable that merely counted bound handles would pass.
	other, err := handle.New(1)
	if err != nil {
		t.Fatalf("handle.New: %v", err)
	}
	if err := other.Bind("member_1.user#1", "u-1"); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if ok, kind := schedule.Resolvable(a, other); ok || kind != "event" {
		t.Errorf("a bound USER handle must not satisfy a flow needing an event: ok=%v kind=%q", ok, kind)
	}

	if err := other.Bind("member_1.event#1", "e-1"); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if ok, kind := schedule.Resolvable(a, other); !ok {
		t.Errorf("a bound event handle must satisfy it; got a complaint about %q", kind)
	}
}
