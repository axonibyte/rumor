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
	"sort"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/decl"
	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
	"forge.axonibyte.com/axonibyte/rumor/internal/handle"
	"forge.axonibyte.com/axonibyte/rumor/internal/platform"
	"forge.axonibyte.com/axonibyte/rumor/internal/schedule"
)

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

	got, err := schedule.Pool(ctx, statute(t), toyActions(t), l, observer, w, "", "", tbl(t))
	if err != nil {
		t.Fatalf("Pool: %v", err)
	}

	// create_event and leave_event are what honest-count's verdicts permit --
	// the pair the applicability arm asserts against the corpus -- plus suborn
	// and palter, which inherit an applicable emitting action's precondition
	// (fable coop r5/49). TWO kinds are excluded and for DIFFERENT reasons,
	// which is what makes this a set assertion and not a count: canard has no
	// planted claim to pass on here, and essoin waits on R9's flow state. Both
	// carry weight 2 in this table, so neither is absent for want of a weight.
	want := []string{"create_event", "leave_event", "palter", "suborn"}
	if strings.Join(kinds(got), ",") != strings.Join(want, ",") {
		t.Fatalf("the pool at honest-count's seal is %v; want %v (canard out on its precondition, "+
			"essoin out until R9, both weighted 2 here)", kinds(got), want)
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

	got, err := schedule.Pool(ctx, statute(t), toyActions(t), l, observer, w, "", "", tbl(t))
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

	_, err := schedule.Pool(ctx, statute(t), toyActions(t), l, observer, w, "", "", tbl(t))
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

	_, err := schedule.Pool(ctx, statute(t), acts, l, observer, withNemesis(toyWeights(t), 1), "", "", tbl(t))
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

	got, err := schedule.Pool(ctx, statute(t), acts, l, observer, withNemesis(toyWeights(t), 2), "", "", tbl(t))
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
			withNemesis(toyWeights(t), 7), "", "", tbl(t))
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
	got, err := schedule.Pool(ctx, statute(t), acts, l, observer, w, "", "", empty)
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
	bound := tbl(t)
	if err := bound.Bind(observer+".event#1", "e-99"); err != nil {
		t.Fatalf("binding an event handle: %v", err)
	}
	got, err = schedule.Pool(ctx, statute(t), acts, l, observer, w, "", "", bound)
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
