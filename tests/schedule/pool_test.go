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

func kinds(cs []schedule.Candidate) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.Kind)
	}
	sort.Strings(out)
	return out
}

// THE CORPUS DECLARES NO NEMESIS WEIGHTS, which is why every arm below that
// wants one builds its own table. Asserted rather than assumed: if the toy ever
// gains them, `withNemesis` silently stops being the thing under test and the
// refusal arm stops reaching its refusal.
func TestTheToyDeclaresNoNemesisWeightYet(t *testing.T) {
	base := toyWeights(t)
	for _, k := range schedule.NemesisKinds {
		if _, ok := base.Weight("", "", k); ok {
			t.Fatalf("examples/toy/platform.toml now declares a weight for %q: the fixture tables in "+
				"this file are no longer distinguishable from the corpus, and the undeclared-weight "+
				"arm can no longer reach its refusal", k)
		}
	}
	if _, ok := base.Weight("", "", "join_event"); !ok {
		t.Fatal("the toy declares no weight for join_event either, so this arm is measuring an " +
			"empty table rather than the absence of the nemesis kinds")
	}
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

	got, err := schedule.Pool(ctx, statute(t), toyActions(t), l, observer, w, "", "")
	if err != nil {
		t.Fatalf("Pool: %v", err)
	}

	// create_event and leave_event are what honest-count's verdicts permit --
	// the pair the applicability arm asserts against the corpus -- plus the
	// three unconditioned nemesis kinds. canard is excluded BY ITS
	// PRECONDITION and not by its weight, which carries 2 here like the rest.
	want := []string{"create_event", "essoin", "leave_event", "palter", "suborn"}
	if strings.Join(kinds(got), ",") != strings.Join(want, ",") {
		t.Fatalf("the pool at honest-count's seal is %v; want %v (canard absent on its precondition, "+
			"the other three unconditioned pending fable's ruling)", kinds(got), want)
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

	got, err := schedule.Pool(ctx, statute(t), toyActions(t), l, observer, w, "", "")
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

// AN UNDECLARED WEIGHT IS REFUSED AND NOT READ AS ZERO. The toy declares none
// of the four, so the corpus's own table reaches this.
func TestAnUndeclaredWeightIsRefusedRatherThanAssumedZero(t *testing.T) {
	ctx := context.Background()
	l, _, observer := sealedToy(t, "honest-count")

	_, err := schedule.Pool(ctx, statute(t), toyActions(t), l, observer, toyWeights(t), "", "")
	if err == nil {
		t.Fatal("the toy declares no nemesis weights, so Pool must refuse rather than invent one")
	}
	if !strings.Contains(err.Error(), "not a zero") {
		t.Errorf("the refusal must say WHY absent is not zero, since that is the whole decision; got %v", err)
	}
}

// AN ACTION NAMED LIKE A NEMESIS KIND IS REFUSED, because the pool is keyed by
// kind and two candidates spelled alike would draw as one carrying two weights.
func TestAnActionNamedLikeANemesisKindIsRefused(t *testing.T) {
	ctx := context.Background()
	l, _, observer := sealedToy(t, "honest-count")
	acts := append(toyActions(t), bindings.Action{Name: schedule.Suborn, Requires: []string{"may_leave"}})

	_, err := schedule.Pool(ctx, statute(t), acts, l, observer, withNemesis(toyWeights(t), 1), "", "")
	if err == nil {
		t.Fatal("an action named `suborn` must be refused: the pool is keyed by kind")
	}
	if !strings.Contains(err.Error(), "nemesis kind") {
		t.Errorf("the refusal must name the collision; got %v", err)
	}
}
