// The draw pool: applicable actions and applicable nemesis kinds in one
// candidate set, each carrying platform.toml's weight for it.
//
// WHY HERE AND NOT IN internal/schedule: these read the committed corpus --
// examples/toy's statute, bindings and a case's ledger -- so they are a suite
// over the tree rather than a unit over a function, the same split the
// applicability arms beside them use.
package schedule_test

import (
	"context"
	"slices"
	"sort"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/decl"
	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
	"forge.axonibyte.com/axonibyte/rumor/internal/handle"
	"forge.axonibyte.com/axonibyte/rumor/internal/ledger"
	"forge.axonibyte.com/axonibyte/rumor/internal/platform"
	"forge.axonibyte.com/axonibyte/rumor/internal/schedule"
)

// WHY EVERY Pool CALL BELOW PASSES NO ROLES. These arms are about weights and
// the nemesis preconditions, and they predate the role-and-context gate; their
// actor is `observer`, which is not a name Actors() could mint (<role>_<n>), so
// a declared role list could not be matched to it anyway. Passing none means the
// gate is not exercised HERE -- it has its own arms, named for it, below.

// knownFor is what the walk hands Pool: the handles this actor knows at the
// seal, through the same engine.KnownHandlesAt the walk calls.
//
// IT IS THE REAL ANSWER AND NOT THE WHOLE TABLE. R5.4 scoped a `handle`
// argument to the handles the ACTOR knows of (fable, coop r5/114), and an arm
// that handed Pool every bound handle would be testing the world before that
// close while claiming to test the one after. Where an arm's own guard then
// reports an empty pool, the arm is telling the truth about the new behaviour.
func knownFor(t *testing.T, l *ledger.Ledger, actor string) []string {
	t.Helper()
	known, err := engine.KnownHandlesAt(l, actor)
	if err != nil {
		t.Fatalf("the handles %s knows: %v", actor, err)
	}
	return known
}

// toyActors is the toy's venire, which is the actor list the walk hands Pool.
//
// WHY THESE ARMS PASS IT RATHER THAN THEIR OWN FIXTURE'S NAMES. R5.4 gave both
// telling kinds a receiver half -- a subject nobody still needs telling of is
// not a tell anybody can make -- and that half is about the OTHER actors. The
// case ledgers these arms read are alice/bob/carol worlds, and the venire is
// admin_1/member_1/member_2/guest_1, none of whom know anything in them: so
// every subject has a receiver and the receiver half is satisfied without
// affecting what these arms measure. It has its OWN arm, named for it, which
// drives the half that matters.
func toyActors(t *testing.T) []string {
	t.Helper()
	rep, err := platform.Load(toy+"/platform.toml", decl.TOML)
	if err != nil || !rep.OK() {
		t.Fatalf("the toy's platform: %v %v", err, rep.Refusals)
	}
	actors, err := schedule.Actors(rep.Platform.Venire)
	if err != nil {
		t.Fatalf("the toy's actors: %v", err)
	}
	return actors
}

// toyActions is the toy's declared actions, loaded the way the walk would.
func toyActions(t *testing.T) []bindings.Action {
	t.Helper()
	rep, err := bindings.Load(toy+"/bindings.toml", decl.TOML)
	if err != nil || !rep.OK() {
		t.Fatalf("the toy's bindings: %v %v", err, rep.Refusals)
	}
	return rep.Bindings.Actions
}

// toyWeights is the toy's platform.toml as committed.
func toyWeights(t *testing.T) platform.Weights {
	t.Helper()
	rep, err := platform.Load(toy+"/platform.toml", decl.TOML)
	if err != nil || !rep.OK() {
		t.Fatalf("the toy's platform: %v %v", err, rep.Refusals)
	}
	return rep.Platform.Weights
}

// withNemesis is the toy's table PLUS a weight for each nemesis kind. It is a
// FIXTURE and not the corpus, which the arm below measures rather than assumes.
func withNemesis(base platform.Weights, w int) platform.Weights {
	d := make(map[string]int, len(base.Default)+len(schedule.NemesisKinds))
	for k, v := range base.Default {
		d[k] = v
	}
	for _, k := range schedule.NemesisKinds {
		d[k] = w
	}
	return platform.Weights{Default: d, Stage: base.Stage, Profile: base.Profile}
}

// withoutWeight is a table with one kind's weight REMOVED from every level.
//
// WHY THE REFUSAL ARM BUILDS ITS OWN GAP rather than leaning on the corpus:
// my first version used examples/toy/platform.toml as committed, which declares
// no nemesis weights today -- and fable has just put the four of them in the
// toy (coop r5/49). An arm resting on what a fixture currently LACKS turns red
// on somebody else's correct change and says nothing about the subject, so the
// absence under test is constructed here instead.
func withoutWeight(base platform.Weights, kind string) platform.Weights {
	d := map[string]int{}
	for k, v := range base.Default {
		if k != kind {
			d[k] = v
		}
	}
	strip := func(in map[string]map[string]int) map[string]map[string]int {
		out := map[string]map[string]int{}
		for name, t := range in {
			cp := map[string]int{}
			for k, v := range t {
				if k != kind {
					cp[k] = v
				}
			}
			out[name] = cp
		}
		return out
	}
	return platform.Weights{Default: d, Stage: strip(base.Stage), Profile: strip(base.Profile)}
}

// tbl is a handle table with nothing bound, which is the state a walk starts
// in. Pool needs one because RESOLVABILITY IS PART OF APPLICABILITY: an action
// whose flow names a `{kind}` no bound handle satisfies cannot be taken.
func tbl(t *testing.T) *handle.Table {
	t.Helper()
	table, err := handle.New(1)
	if err != nil {
		t.Fatalf("a fresh handle table: %v", err)
	}
	return table
}

func kinds(cs []schedule.Candidate) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.Kind)
	}
	sort.Strings(out)
	return out
}

// CANARD'S PRECONDITION, THREE CELLS FROM ONE FIXTURE. planted-join holds
// carol's PLANTED claim carol.join#1, told to alice and never to bob. So each
// clause of "the teller knows a planted claim it did not make" is the only
// thing separating one actor from another:
//
//	alice  knows it, did not author it   -> applicable
//	carol  authored it                   -> inapplicable
//	bob    was never told it             -> inapplicable
//
// Deleting any clause makes one of these three wrong, which is what stops the
// precondition from being satisfiable by accident.
func TestCanardsPreconditionSeparatesKnowledgeFromAuthorship(t *testing.T) {
	l, _, _ := sealedToy(t, "planted-join")

	alice, err := engine.CanardSubjects(l, "alice")
	if err != nil {
		t.Fatalf("alice: %v", err)
	}
	if len(alice) != 1 || alice[0] != "carol.join#1" {
		t.Errorf("alice knows carol's planted claim and did not make it, so canard's subjects are "+
			"[carol.join#1]; got %v", alice)
	}

	carol, err := engine.CanardSubjects(l, "carol")
	if err != nil {
		t.Fatalf("carol: %v", err)
	}
	if len(carol) != 0 {
		t.Errorf("carol AUTHORED the planted claim, so passing it on is not a canard; got %v", carol)
	}

	bob, err := engine.CanardSubjects(l, "bob")
	if err != nil {
		t.Fatalf("bob: %v", err)
	}
	if len(bob) != 0 {
		t.Errorf("bob was never told carol's claim, so there is nothing for bob to pass on; got %v", bob)
	}
}

// AND A LEDGER WITH NO PLANTED CLAIM AT ALL: the kind is inapplicable for
// everyone, which is the state the pool arm below reads.
func TestCanardIsInapplicableWhereNothingWasPlanted(t *testing.T) {
	l, _, observer := sealedToy(t, "honest-count")
	subjects, err := engine.CanardSubjects(l, observer)
	if err != nil {
		t.Fatalf("CanardSubjects: %v", err)
	}
	if len(subjects) != 0 {
		t.Errorf("honest-count plants nothing, so canard has no subject; got %v", subjects)
	}
}

// THE POOL ITSELF: the applicable actions AND the applicable nemesis kinds, in
// one candidate set, each weighted. honest-count plants nothing, so canard is
// the one kind that must be absent -- which makes this arm read the
// applicability of a nemesis kind and not merely the presence of four names.
func TestPoolCarriesActionsAndApplicableNemesisKinds(t *testing.T) {
	ctx := context.Background()
	l, _, observer := sealedToy(t, "honest-count")
	w := withNemesis(toyWeights(t), 2)

	got, err := schedule.Pool(ctx, statute(t), toyActions(t), l, observer, w, "", "", tbl(t), nil, knownFor(t, l, observer), toyActors(t))
	if err != nil {
		t.Fatalf("Pool: %v", err)
	}

	// create_event is what honest-count's verdicts permit with NOTHING BOUND,
	// plus suborn and palter, which inherit an applicable emitting action's
	// precondition (fable coop r5/49). THREE kinds are excluded and for THREE
	// DIFFERENT reasons, which is what makes this a set assertion and not a
	// count: canard has no planted claim to pass on here, essoin waits on
	// R9's flow state -- both weighted 2 in this table, so neither is absent
	// for want of a weight -- and leave_event is out on RESOLVABILITY.
	//
	// leave_event LEFT THIS SET IN R5.4 (issue 78) and the change is the
	// principle this file already states at tbl(): resolvability is part of
	// applicability. It now declares `[[action.args]] of = "event"`, tbl() is
	// a fresh table, and docs/contract.md's own row says a kind nothing has
	// bound makes the action inapplicable, exactly as an unresolvable {kind}
	// in a flow does. The cell below binds one and watches it come back, so
	// this is an assertion about resolvability rather than a shrunken want.
	// AND `tell` JOINED THIS SET IN R5.4, which is the honest kind's whole
	// point: bob knows two claims the run did not plant, and the venire holds
	// four actors none of whom know them, so there is both something to pass on
	// and somebody who still needs it. canard stays OUT of the same pool on the
	// same fixture, which is the pair worth seeing in one want -- the two kinds
	// differ on plantedness and here that difference is the whole difference.
	want := []string{"create_event", "palter", "suborn", "tell"}
	if strings.Join(kinds(got), ",") != strings.Join(want, ",") {
		t.Fatalf("the pool at honest-count's seal is %v; want %v (canard out on its precondition "+
			"and tell IN on its mirror, essoin out until R9, all weighted here; leave_event out "+
			"until an event handle resolves its declared arg)", kinds(got), want)
	}

	// THE CONTROL FOR THAT EXCLUSION, without which the want above would pass
	// over a Pool that dropped leave_event for any reason at all: bind one
	// event handle and the action must return.
	// THE HANDLE IS alice'S AND NOT THE OBSERVER'S, which R5.4's scoping makes
	// the difference between a control and a no-op. A `handle` argument now
	// resolves only to a handle the ACTOR knows of (fable, coop r5/114), and
	// honest-count's observer is bob: bob was TOLD alice.event#1 at position 4
	// and bob's own join claim names it, so bob knows it. `bob.event#1` -- what
	// this bound before -- is a handle bob never created and was never told,
	// so binding it left the action correctly inapplicable and this control
	// failed loudly rather than passing on nothing.
	withEvent := tbl(t)
	if err := withEvent.Bind("alice.event#1", "e-99"); err != nil {
		t.Fatalf("binding an event handle: %v", err)
	}
	back, err := schedule.Pool(ctx, statute(t), toyActions(t), l, observer, w, "", "", withEvent, nil, knownFor(t, l, observer), toyActors(t))
	if err != nil {
		t.Fatalf("Pool with a bound event handle: %v", err)
	}
	if !slices.Contains(kinds(back), "leave_event") {
		t.Errorf("with an event handle bound the pool is %v; leave_event must be in it, or its "+
			"absence above was not about resolvability", kinds(back))
	}
	// EACH CANDIDATE CARRIES THE TABLE'S WEIGHT, asserted as equality against
	// the table. My first version of this read `!= 2 && != w.Default[kind]`,
	// which cannot fail: the fixture's 2 IS the table's value for the nemesis
	// kinds, so the disjunction was satisfied by every possible weight.
	for _, c := range got {
		want, ok := w.Default[c.Kind]
		if !ok {
			t.Errorf("candidate %q is in the pool but the table this pool was built from declares "+
				"no weight for it, which Pool is supposed to refuse", c.Kind)
			continue
		}
		if c.Weight != want {
			t.Errorf("candidate %q carries weight %d and platform.toml resolves %d", c.Kind, c.Weight, want)
		}
	}

	// AND THE POOL IS DRAWABLE: Pick must answer one of exactly these kinds.
	// A pool Pick refuses would be a candidate set in name only.
	src := schedule.New(99)
	drawn, err := src.Pick(got)
	if err != nil {
		t.Fatalf("the pool Pool built is not drawable: %v", err)
	}
	found := false
	for _, k := range kinds(got) {
		if k == drawn {
			found = true
		}
	}
	if !found {
		t.Errorf("Pick answered %q, which is not in the pool %v", drawn, kinds(got))
	}
}

// CANARD IN THE POOL, the other side of the same question: on planted-join its
// precondition holds for the observer, so the kind must APPEAR. Without this
// cell the arm above would pass over a Pool that never offered canard at all.
func TestCanardEntersThePoolWhereItsPreconditionHolds(t *testing.T) {
	ctx := context.Background()
	l, _, observer := sealedToy(t, "planted-join")
	if observer != "alice" {
		t.Fatalf("this arm needs planted-join's observer to be the actor that was TOLD the planted "+
			"claim; the seal names %q, so the cell no longer reaches canard's precondition", observer)
	}
	w := withNemesis(toyWeights(t), 3)

	got, err := schedule.Pool(ctx, statute(t), toyActions(t), l, observer, w, "", "", tbl(t), nil, knownFor(t, l, observer), toyActors(t))
	if err != nil {
		t.Fatalf("Pool: %v", err)
	}
	for _, c := range got {
		if c.Kind == schedule.Canard {
			if c.Weight != 3 {
				t.Errorf("canard is in the pool with weight %d, want the declared 3", c.Weight)
			}
			return
		}
	}
	t.Errorf("alice knows a planted claim she did not make, so canard belongs in the pool; got %v",
		kinds(got))
}

// AN UNDECLARED WEIGHT IS REFUSED AND NOT READ AS ZERO -- fable ruled the
// refusal in at coop r5/49, because "absent means not drawn" is the silent-drop
// class by another name.
func TestAnUndeclaredWeightIsRefusedRatherThanAssumedZero(t *testing.T) {
	ctx := context.Background()
	l, _, observer := sealedToy(t, "honest-count")

	// THE ARM ASSERTS ITS OWN PRECONDITION: the table it passes really must be
	// missing suborn, or Pool has nothing to refuse and this passes vacuously.
	w := withoutWeight(withNemesis(toyWeights(t), 2), schedule.Suborn)
	if _, ok := w.Weight("", "", schedule.Suborn); ok {
		t.Fatal("the fixture table still declares a weight for suborn, so this arm cannot reach " +
			"the refusal it exists for and would pass with the defect present")
	}
	if _, ok := w.Weight("", "", schedule.Palter); !ok {
		t.Fatal("the fixture table lost palter's weight too, so a refusal here would not be " +
			"evidence about suborn in particular")
	}

	_, err := schedule.Pool(ctx, statute(t), toyActions(t), l, observer, w, "", "", tbl(t), nil, knownFor(t, l, observer), toyActors(t))
	if err == nil {
		t.Fatal("a kind the walk will draw with no declared weight must be refused, not invented")
	}
	if !strings.Contains(err.Error(), "not a zero") {
		t.Errorf("the refusal must say WHY absent is not zero, since that is the whole decision; got %v", err)
	}
	if !strings.Contains(err.Error(), schedule.Suborn) {
		t.Errorf("the refusal must name WHICH kind has no weight; got %v", err)
	}
}

// AN ACTION NAMED LIKE A NEMESIS KIND IS REFUSED, because the pool is keyed by
// kind and two candidates spelled alike would draw as one carrying two weights.
func TestAnActionNamedLikeANemesisKindIsRefused(t *testing.T) {
	ctx := context.Background()
	l, _, observer := sealedToy(t, "honest-count")
	acts := append(toyActions(t), bindings.Action{Name: schedule.Suborn, Requires: []string{"may_leave"}})

	_, err := schedule.Pool(ctx, statute(t), acts, l, observer, withNemesis(toyWeights(t), 1), "", "", tbl(t), nil, knownFor(t, l, observer), toyActors(t))
	if err == nil {
		t.Fatal("an action named `suborn` must be refused: the pool is keyed by kind")
	}
	if !strings.Contains(err.Error(), "nemesis kind") {
		t.Errorf("the refusal must name the collision; got %v", err)
	}
}

// SUBORN AND PALTER INHERIT THE HONEST ACTION'S PRECONDITION, so a seal where
// NO applicable action emits anything must offer neither. Without this cell the
// pool arm above would pass over an implementation that still returned true
// unconditionally, which is what I first built.
//
// THE WORLD IS BUILT BY STRIPPING `emits`, not by finding a case with none:
// every toy action emits, so the state this arm needs does not exist in the
// corpus and asserting it did would be the reach failure fable's /16 ruling now
// has the gate read every battery for.
func TestSubornAndPalterNeedAnApplicableEmittingAction(t *testing.T) {
	ctx := context.Background()
	l, _, observer := sealedToy(t, "honest-count")

	acts := toyActions(t)
	emitting := 0
	for i := range acts {
		if len(acts[i].Emits) > 0 {
			emitting++
			acts[i].Emits = nil
		}
	}
	if emitting == 0 {
		t.Fatal("no toy action emits anything, so stripping emits changes nothing and this arm " +
			"cannot reach the precondition it is about")
	}

	got, err := schedule.Pool(ctx, statute(t), acts, l, observer, withNemesis(toyWeights(t), 2), "", "", tbl(t), nil, knownFor(t, l, observer), toyActors(t))
	if err != nil {
		t.Fatalf("Pool: %v", err)
	}
	for _, c := range got {
		if c.Kind == schedule.Suborn || c.Kind == schedule.Palter {
			t.Errorf("%q was offered where no applicable action emits anything: the planted claim "+
				"would have no type to be, and a lie about an action the actor could not take is a "+
				"lie of a kind the vocabulary does not have", c.Kind)
		}
	}
}

// ESSOIN IS NEVER DRAWN IN R5, and this pins it so it cannot creep back in on
// the way to R9. It is excluded by its PRECONDITION and not by its weight: the
// table here gives it a positive one, so an implementation that relied on the
// toy's zero would fail this arm.
func TestEssoinIsNeverOfferedBeforeR9(t *testing.T) {
	ctx := context.Background()
	for _, name := range []string{"honest-count", "planted-join"} {
		l, _, observer := sealedToy(t, name)
		got, err := schedule.Pool(ctx, statute(t), toyActions(t), l, observer,
			withNemesis(toyWeights(t), 7), "", "", tbl(t), nil, knownFor(t, l, observer), toyActors(t))
		if err != nil {
			t.Fatalf("%s: Pool: %v", name, err)
		}
		for _, c := range got {
			if c.Kind == schedule.Essoin {
				t.Errorf("%s offered essoin with weight %d: abandoning mid-flow needs flow state "+
					"the engine gains at R9, so it is inapplicable and not merely cheap", name, c.Weight)
			}
		}
	}
}

// AN ACTION WHOSE FLOW NEEDS AN UNRESOLVABLE HANDLE IS INAPPLICABLE, which is
// the second half of applicability fable ruled in at coop r5/58 -- the C1
// refusal one layer earlier, so the walk never asks the wire for a thing that
// does not exist.
//
// THE WORLD IS CONSTRUCTED, and why: the toy's only flow action is
// join_event_ui, which requires may_join -- false at honest-count, so it is
// already excluded by its PRECONDITION and could never show this. The arm
// gives that action a precondition the case records TRUE, so the only thing
// left that can exclude it is the unresolvable `{event}` in its flow.
func TestAnActionWhoseFlowHandleCannotResolveIsInapplicable(t *testing.T) {
	ctx := context.Background()
	l, _, observer := sealedToy(t, "honest-count")
	w := withNemesis(toyWeights(t), 2)

	acts := toyActions(t)
	var flowAction string
	for i := range acts {
		if len(schedule.FlowPlaceholders(acts[i])) > 0 {
			flowAction = acts[i].Name
			acts[i].Requires = []string{"may_leave"}
			break
		}
	}
	if flowAction == "" {
		t.Fatal("no toy action has a flow placeholder, so this arm cannot reach resolvability at " +
			"all and would pass with the check absent")
	}

	// NOTHING BOUND: the flow's {event} resolves to no id, so the action is out.
	empty := tbl(t)
	got, err := schedule.Pool(ctx, statute(t), acts, l, observer, w, "", "", empty, nil, knownFor(t, l, observer), toyActors(t))
	if err != nil {
		t.Fatalf("Pool: %v", err)
	}
	for _, c := range got {
		if c.Kind == flowAction {
			t.Fatalf("%q was offered with no bound handle for its flow's {event}: the walk would "+
				"ask the wire to navigate to an id that does not exist", flowAction)
		}
	}

	// BOUND: the same action, the same precondition, one event handle that
	// resolves -- and now it must be offered. Without this cell the assertion
	// above would pass over a Pool that excluded the action for any reason at
	// all, including a precondition I had got wrong.
	// THE HANDLE IS alice'S AND NOT THE OBSERVER'S, which R5.4's scoping makes
	// the difference between a control and a no-op. A `handle` argument now
	// resolves only to a handle the ACTOR knows of (fable, coop r5/114), and
	// honest-count's observer is bob: bob was TOLD alice.event#1 at position 4
	// and bob's own join claim names it, so bob knows it. `bob.event#1` -- what
	// this bound before -- is a handle bob never created and was never told,
	// so binding it left the action correctly inapplicable and this control
	// failed loudly rather than passing on nothing.
	bound := tbl(t)
	if err := bound.Bind("alice.event#1", "e-99"); err != nil {
		t.Fatalf("binding an event handle: %v", err)
	}
	got, err = schedule.Pool(ctx, statute(t), acts, l, observer, w, "", "", bound, nil, knownFor(t, l, observer), toyActors(t))
	if err != nil {
		t.Fatalf("Pool with a bound handle: %v", err)
	}
	found := false
	for _, c := range got {
		if c.Kind == flowAction {
			found = true
		}
	}
	if !found {
		t.Errorf("%q was excluded even with a resolving event handle, so the arm above proves "+
			"nothing about resolvability; pool is %v", flowAction, kinds(got))
	}
}

// AN ACTION IN A CONTEXT THE ACTOR'S ROLE DOES NOT HOLD IS INAPPLICABLE, which
// is issue 78's third clause and the one that keeps the engine from acquiring a
// session in a context the project says this role never has.
//
// IT IS A DIFFERENTIAL ARM: the same statute, ledger, actor, weights and table
// are used twice and ONLY the role declaration changes. An arm that asserted a
// fixed expected set would move whenever the toy's statute or weights moved,
// and would not say that the ROLES were what removed the action.
func TestAnActionInAnUnheldContextIsInapplicable(t *testing.T) {
	ctx := context.Background()

	// The actor must be a name Actors() could mint, since the role is read off
	// it. honest-count's own actor is `observer`, which is not one.
	const actor = "member_1"

	// AND IT MUST HOLD A USER, because `has_user` gates every `api` action
	// since R5.4: honest-count's ledger alone leaves a minted actor with an
	// empty pool and this differential nothing to subtract from.
	l := sealedToyWithUserFor(t, actor)
	w := withNemesis(toyWeights(t), 2)

	// ONE ACTION MOVED TO A SECOND CONTEXT, and the corpus is read rather than
	// assumed: if the toy stopped declaring the action this picks, the guard
	// below fails loudly instead of the arm passing on an empty difference.
	acts := toyActions(t)
	moved := ""
	for i := range acts {
		if acts[i].Context == "api" && len(acts[i].Emits) > 0 {
			acts[i].Context = "ui"
			moved = acts[i].Name
			break
		}
	}
	if moved == "" {
		t.Fatal("no toy action declares context api, so there is nothing to move to a second " +
			"context and this arm cannot reach the gate")
	}

	// BASELINE: the role holds BOTH contexts, so nothing is gated out.
	both := []bindings.Role{{Name: "member", Contexts: []string{"api", "ui"}}}
	before, err := schedule.Pool(ctx, statute(t), acts, l, actor, w, "", "", tbl(t), both, knownFor(t, l, actor), toyActors(t))
	if err != nil {
		t.Fatalf("Pool with both contexts held: %v", err)
	}
	if !strings.Contains(strings.Join(kinds(before), ","), moved) {
		t.Fatalf("%s is not in the pool even when its context is held (%v), so removing it proves "+
			"nothing", moved, kinds(before))
	}

	// AND THE SAME CALL WITH ui NOT HELD: the moved action is gone and the
	// others are untouched.
	apiOnly := []bindings.Role{{Name: "member", Contexts: []string{"api"}}}
	after, err := schedule.Pool(ctx, statute(t), acts, l, actor, w, "", "", tbl(t), apiOnly, knownFor(t, l, actor), toyActors(t))
	if err != nil {
		t.Fatalf("Pool with ui unheld: %v", err)
	}
	if strings.Contains(strings.Join(kinds(after), ","), moved) {
		t.Errorf("%s needs context ui and the role holds only api, so it must be inapplicable; "+
			"the pool is %v", moved, kinds(after))
	}
	// NOTHING ELSE GOES -- OVER THE PROJECT'S ACTIONS, AND THAT BOUND IS THE
	// POINT. My first version of this loop asserted it over every KIND in the
	// pool, and R5.4's palter broke it honestly: a nemesis kind RIDES an
	// applicable action's precondition (fable r5/49), so gating the only
	// palterable action out legitimately takes palter with it. The assertion
	// was wrong about the system's structure rather than about the gate --
	// nemesis kinds are derived from the action set, so they are not
	// independent of it, and an arm demanding they survive would be demanding
	// that a lie outlive the action it is about.
	nemesis := map[string]bool{schedule.Suborn: true, schedule.Canard: true,
		schedule.Palter: true, schedule.Essoin: true}
	for _, k := range kinds(before) {
		if k == moved || nemesis[k] {
			continue
		}
		if !strings.Contains(strings.Join(kinds(after), ","), k) {
			t.Errorf("gating ui also removed %q, which needs no ui: before %v, after %v",
				k, kinds(before), kinds(after))
		}
	}
}

// AND A ROLE THE DECLARATION OMITS IS A REFUSAL, not an empty pool: a walk that
// stalled for a missing [[role]] would look exactly like a walk whose world
// permits nothing.
//
// TWO CELLS, AND THE SECOND IS THE ONE THAT MATTERS. This arm first held only
// the first, and ARRANGED a user to reach the gate -- saying so in a comment
// that described the defect as a premise: "the role lookup is reached only for
// an action that is already applicable, so an actor `has_user` gates out would
// produce an empty pool and no refusal". That sentence is the bug ibis reported
// at coop r5/93 and fable placed in the engine at r5/95, and the arm could not
// see it because the arm had removed the condition. A harness that sets up the
// premise cannot test it.
//
// Cell 2 drives the arranged state deliberately wrong: the actor holds NO user,
// so no `api` action is applicable and the only applicable action is
// `create_user`, whose context is `none` and which therefore returns from
// HoldsContext before the role is ever looked up. Under the old ordering the
// pool came back POPULATED and silent. The refusal now fires because the gate
// runs over the project's actions.
func TestAnUndeclaredRoleRefusesRatherThanEmptyingThePool(t *testing.T) {
	ctx := context.Background()

	w := withNemesis(toyWeights(t), 2)
	other := []bindings.Role{{Name: "admin", Contexts: []string{"api"}}}

	// CELL 1: the actor holds a user, so `api` actions are applicable.
	withUser := sealedToyWithUserFor(t, "member_1")
	_, err := schedule.Pool(ctx, statute(t), toyActions(t), withUser, "member_1", w, "", "", tbl(t), other, knownFor(t, withUser, "member_1"), toyActors(t))
	if err == nil {
		t.Fatal("a declaration naming only `admin` was read as `member` holding no context")
	}
	if !strings.Contains(err.Error(), "member") {
		t.Errorf("the refusal must name the role it could not find; got %v", err)
	}

	// CELL 2: the same actor with no user of its own.
	noUser, _, _ := sealedToy(t, "honest-count")

	// THE PREMISE IS MEASURED AND NOT ASSUMED. With a role that IS declared,
	// this world must give a pool that holds `create_user` and no `api` action
	// -- otherwise an applicable `api` action is present and cell 2 would reach
	// the gate the old way, proving nothing about the ordering.
	member := []bindings.Role{{Name: "member", Contexts: []string{"api"}}}
	ok, err := schedule.Pool(ctx, statute(t), toyActions(t), noUser, "member_1", w, "", "", tbl(t), member, knownFor(t, noUser, "member_1"), toyActors(t))
	if err != nil {
		t.Fatalf("Pool for an actor with no user and a declared role: %v", err)
	}
	api := map[string]bool{}
	for _, a := range toyActions(t) {
		if a.Context == "api" {
			api[a.Name] = true
		}
	}
	got := kinds(ok)
	if !strings.Contains(strings.Join(got, ","), "create_user") {
		t.Fatalf("an actor with no user must still be able to create one; pool is %v", got)
	}
	for _, k := range got {
		if api[k] {
			t.Fatalf("%q is an api action and was applicable to an actor with no user, so "+
				"`has_user` is not gating and cell 2 cannot test the ordering; pool is %v", k, got)
		}
	}

	_, err = schedule.Pool(ctx, statute(t), toyActions(t), noUser, "member_1", w, "", "", tbl(t), other, knownFor(t, noUser, "member_1"), toyActors(t))
	if err == nil {
		t.Fatal("an actor nothing `api` is applicable to got a pool and no refusal, so a missing " +
			"[[role]] and a world permitting nothing are still the same answer")
	}
	if !strings.Contains(err.Error(), "member") {
		t.Errorf("the refusal must name the role it could not find; got %v", err)
	}
}

// A HANDLE THE ACTOR DOES NOT KNOW OF DOES NOT RESOLVE ITS ARGUMENT, which is
// R5.4's close of the narrowing `Resolvable`'s own comment had declared and left
// open. fable ruled it at coop r5/114: "the precondition and the argument
// quantify over one set", so a walk can never pass `may_join` on event A and
// send event B.
//
// IT IS A DIFFERENTIAL OVER ONE VARIABLE: the same statute, ledger, actor,
// weights and roles, two handle tables that differ only in WHOSE event is
// bound. honest-count's observer is bob, who was told `alice.event#1` at
// position 4; `carol.event#1` is an event of nobody's that bob was never told.
// An arm asserting a fixed set would move whenever the toy's weights moved and
// would not say that KNOWLEDGE was what removed the action.
//
// WHY THIS IS THE DEFECT AND NOT A TIGHTENING. Seed 6 step 8 had `guest_1` join
// `member_1.event#1` -- an event nothing had told it of -- because
// `knows_of_subject` passed on guest_1's OWN event while the draw took
// member_1's (ibis, coop r5/111). The precondition and the argument were
// quantifying over different sets, so the walk sent the wire an id the actor
// had no business naming, and `Arg.Of`'s own documented promise -- "an id the
// ACTOR knows of" -- was false of the engine.
func TestAHandleTheActorDoesNotKnowDoesNotResolve(t *testing.T) {
	ctx := context.Background()
	l, _, observer := sealedToy(t, "honest-count")
	w := withNemesis(toyWeights(t), 2)
	known := knownFor(t, l, observer)

	// THE PREMISE, MEASURED: bob knows alice's event and not carol's. Without
	// this the two cells below could differ for some other reason entirely.
	if !slices.Contains(known, "alice.event#1") {
		t.Fatalf("%s does not know alice.event#1 (%v), so the KNOWN cell below is not about "+
			"knowledge", observer, known)
	}
	if slices.Contains(known, "carol.event#1") {
		t.Fatalf("%s knows carol.event#1 (%v), so the UNKNOWN cell below has no subject",
			observer, known)
	}

	pool := func(bind string) []string {
		t.Helper()
		table := tbl(t)
		if err := table.Bind(bind, "e-99"); err != nil {
			t.Fatalf("binding %s: %v", bind, err)
		}
		got, err := schedule.Pool(ctx, statute(t), toyActions(t), l, observer, w, "", "", table,
			nil, known, toyActors(t))
		if err != nil {
			t.Fatalf("Pool with %s bound: %v", bind, err)
		}
		return kinds(got)
	}

	withKnown := pool("alice.event#1")
	withUnknown := pool("carol.event#1")

	// WHICH ACTIONS TURN ON THIS IS DERIVED FROM THE BINDINGS, not named here.
	// It said "leave_event ... is the action whose applicability turns on this
	// and nothing else", which was true of the project when it was written and
	// stopped being true the moment R6.3 added `recount_event` -- a second
	// action declaring an `event` handle argument. The arm then failed on its
	// own premise rather than on its subject.
	//
	// DERIVED IS STRICTLY STRONGER: every action declaring a handle argument of
	// kind `event` must move, and every action declaring none must not, so a
	// third such action is covered by arriving rather than by being remembered.
	needsEventHandle := map[string]bool{}
	for _, a := range toyActions(t) {
		for _, arg := range a.Args {
			if arg.Source == bindings.ArgHandle && arg.Of == "event" {
				needsEventHandle[a.Name] = true
			}
		}
	}
	if len(needsEventHandle) == 0 {
		t.Fatal("no action declares an `event` handle argument, so this arm has no subject")
	}
	const needsHandle = "leave_event"
	if !needsEventHandle[needsHandle] {
		t.Fatalf("%q no longer declares an `event` handle argument, so the two cells below are "+
			"not about it", needsHandle)
	}
	if !slices.Contains(withKnown, needsHandle) {
		t.Fatalf("%q is absent with a KNOWN event bound (%v), so this arm cannot show knowledge "+
			"removing it", needsHandle, withKnown)
	}
	for name := range needsEventHandle {
		if slices.Contains(withUnknown, name) {
			t.Errorf("%q is present with only an UNKNOWN event bound (%v): the draw would send "+
				"%s an id it was never told of, which is the promise Arg.Of makes and the engine "+
				"did not keep", name, withUnknown, observer)
		}
	}

	// AND NOTHING ELSE MOVED, over the project's ACTIONS. Nemesis kinds ride an
	// applicable action's precondition (fable r5/49), so they are derived and
	// not independent -- the same bound the role-gate arm carries, for the same
	// reason.
	nemesis := map[string]bool{schedule.Suborn: true, schedule.Canard: true,
		schedule.Palter: true, schedule.Essoin: true}
	for _, k := range withKnown {
		if needsEventHandle[k] || nemesis[k] {
			continue
		}
		if !slices.Contains(withUnknown, k) {
			t.Errorf("binding an unknown event also removed %q, which needs no event handle: "+
				"known %v, unknown %v", k, withKnown, withUnknown)
		}
	}
	t.Logf("PROOF %s knows %v; pool with alice's event %v, with carol's %v",
		observer, known, withKnown, withUnknown)
}

// THE HONEST TELL AND CANARD SEPARATE ON PLANTEDNESS AND ON AUTHORSHIP, and
// planted-join separates all three actors on exactly those two clauses. One
// fixture, both kinds, because the pair is the point: before R5.4 the walk's
// only `told` was canard's, so a lie could travel and an honest claim could not.
//
// THE FIXTURE, READ AND NOT ASSUMED -- my first version of this arm asserted
// that carol had something to tell, which is false, and the arm failed on its
// own premise rather than on the subject:
//
//	alice  owns alice.event#1 (honest); told bob.join#1 (honest) and
//	       carol.join#1 (PLANTED)        -> tells 2, canards 1
//	bob    told alice.event#1 (honest); owns bob.join#1 (honest)
//	                                     -> tells 2, canards 0
//	carol  owns carol.join#1 (PLANTED), told nothing
//	                                     -> tells 0, canards 0
//
// SO EACH CELL IS ONE CLAUSE. alice's tells must EXCLUDE carol.join#1 though it
// knows it: that is plantedness. alice's canard must INCLUDE it: that is the
// same clause read the other way. bob canards nothing though it knows two
// claims: plantedness again, from the other side. carol tells nothing though it
// knows a claim: an actor holding only a lie has nothing honest to pass on,
// which is the clause in its purest form. And carol canards nothing because it
// AUTHORED the lie -- the clause tell deliberately does not have, since an
// actor telling of its own event is the ordinary case and the one honest-count
// carries by hand at position 4.
func TestTheHonestTellAndCanardSeparateOnPlantedness(t *testing.T) {
	l, _, _ := sealedToy(t, "planted-join")

	tells := func(actor string) []string {
		t.Helper()
		got, err := engine.TellSubjects(l, actor)
		if err != nil {
			t.Fatalf("%s's tell subjects: %v", actor, err)
		}
		return got
	}
	canards := func(actor string) []string {
		t.Helper()
		got, err := engine.CanardSubjects(l, actor)
		if err != nil {
			t.Fatalf("%s's canard subjects: %v", actor, err)
		}
		return got
	}

	// THE PREMISE, MEASURED: the fixture holds both a planted claim and honest
	// ones, or no cell below is about plantedness.
	var planted, honest int
	for _, f := range l.SealedPrefix() {
		if f.Kind != ledger.KindClaim {
			continue
		}
		if f.Planted {
			planted++
		} else {
			honest++
		}
	}
	if planted == 0 || honest == 0 {
		t.Fatalf("planted-join holds %d planted and %d honest claim(s); this arm needs both",
			planted, honest)
	}

	aliceTells, aliceCanards := tells("alice"), canards("alice")
	if len(aliceTells) != 2 {
		t.Errorf("alice knows two honest claims (its own event and bob's join), so it has two to "+
			"tell; got %v", aliceTells)
	}
	if slices.Contains(aliceTells, "carol.join#1") {
		t.Errorf("carol.join#1 is PLANTED and alice knows it, but a tell passes on a claim the "+
			"run did NOT plant: %v", aliceTells)
	}
	if !slices.Contains(aliceCanards, "carol.join#1") {
		t.Errorf("alice knows carol's planted claim and did not author it, so it is exactly a "+
			"canard's subject; got %v", aliceCanards)
	}
	if !slices.Contains(aliceTells, "alice.event#1") {
		t.Errorf("alice cannot tell of the event it created itself (%v), which is the ordinary "+
			"case and the clause canard has and tell must not", aliceTells)
	}

	if got := canards("bob"); len(got) != 0 {
		t.Errorf("bob knows no planted claim, so it has no canard to make; got %v", got)
	}
	if got := tells("bob"); len(got) != 2 {
		t.Errorf("bob knows two honest claims, so it has two to tell; got %v", got)
	}

	if got := tells("carol"); len(got) != 0 {
		t.Errorf("carol knows ONLY the lie it authored, so it has nothing honest to pass on; "+
			"got %v", got)
	}
	if got := canards("carol"); len(got) != 0 {
		t.Errorf("carol AUTHORED the planted claim, so passing it on is not a canard; got %v", got)
	}

	t.Logf("PROOF alice tells %v canards %v; bob tells %v canards %v; carol tells %v canards %v",
		aliceTells, aliceCanards, tells("bob"), canards("bob"), tells("carol"), canards("carol"))
}
