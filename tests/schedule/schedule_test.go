// D14 and applicability over the only preconditions in the tree.
//
// WHY HERE AND NOT IN internal/schedule: this reads the committed corpus --
// examples/toy's statute, a case's ledger and that case's recorded verdicts --
// so it is a suite over the tree rather than a unit over a function, the same
// split tests/engine and tests/attaint already use.
package schedule_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/decl"
	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
	"forge.axonibyte.com/axonibyte/rumor/internal/ledger"
	"forge.axonibyte.com/axonibyte/rumor/internal/schedule"
)

// the six rules the toy's actions name in `requires` -- five measured as the
// only preconditions in the tree (coop r5/3), and `has_user` added in R5.4 when
// the session ordering became a precondition (fable, coop r5/92). D14's
// agreement arm below is where a new precondition earns its coverage, so the
// list moves with the bindings: a rule in `requires` and absent here would be
// one the walk reads and this drill never holds to the case path.
var preconditions = []string{
	"has_user", "may_create_event", "may_create_user", "may_delete", "may_join", "may_leave",
}

const toy = "../../examples/toy"

func sealedToy(t *testing.T, name string) (*ledger.Ledger, map[string]any, string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(toy, "ledger", name+".jsonl"))
	if err != nil {
		t.Fatalf("the toy's %s ledger: %v", name, err)
	}
	var seal struct {
		Seal     int    `json:"seal"`
		Observer string `json:"observer"`
	}
	sraw, err := os.ReadFile(filepath.Join(toy, "ledger", name+".seal.json"))
	if err != nil {
		t.Fatalf("the toy's %s seal record: %v", name, err)
	}
	if err := json.Unmarshal(sraw, &seal); err != nil {
		t.Fatalf("%s seal: %v", name, err)
	}
	l, err := ledger.Load(strings.NewReader(string(raw)), false)
	if err != nil {
		t.Fatalf("%s does not load: %v", name, err)
	}
	if err := l.SealAt(seal.Seal); err != nil {
		t.Fatalf("%s sealing at %d: %v", name, seal.Seal, err)
	}

	var doc struct {
		Expect map[string]any `json:"expect"`
		Input  struct {
			Observation map[string]any `json:"observation"`
		} `json:"input"`
	}
	craw, err := os.ReadFile(filepath.Join(toy, "cases", name+".json"))
	if err != nil {
		t.Fatalf("the toy's %s case: %v", name, err)
	}
	if err := json.Unmarshal(craw, &doc); err != nil {
		t.Fatalf("%s case: %v", name, err)
	}
	return l, doc.Input.Observation, seal.Observer
}

func statute(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(toy, "statute.rego"))
	if err != nil {
		t.Fatalf("the toy's statute: %v", err)
	}
	return string(b)
}

// sealedToyWithUserFor is honest-count's ledger plus a `user.created` claim for
// a MINTED actor, sealed over it.
//
// WHY IT EXISTS. R5.4 made `has_user` a precondition of every `api` action
// (fable, coop r5/92), so an actor holding no user of its own has nothing
// applicable. That is the rule working, and it is also fatal to any arm whose
// subject is something else: the role-and-context arms mint `member_1` because
// HoldsContext reads a role off the actor's NAME, and `member_1` appears in no
// committed ledger. Without a user for it those arms measure an empty pool --
// which their own guards reported rather than passing, and this is the fixture
// that answers them.
//
// THE CLAIM IS THE MINIMUM THAT CLEARS has_user AND NOTHING MORE: no event, so
// may_create_event stays true and may_join, may_leave and may_delete stay
// false, exactly as for any fresh actor. An arm about roles must not be handed
// a world that has also moved the other five verdicts.
func sealedToyWithUserFor(t *testing.T, actor string) *ledger.Ledger {
	t.Helper()
	return sealedToyPlusUser(t, actor, false)
}

// sealedToyWithPlantedUserFor is the same world with the claim PLANTED, which
// is the only difference the arms below may lean on. One body for both so the
// honest and planted fixtures cannot drift apart in anything else.
func sealedToyWithPlantedUserFor(t *testing.T, actor string) *ledger.Ledger {
	t.Helper()
	return sealedToyPlusUser(t, actor, true)
}

func sealedToyPlusUser(t *testing.T, actor string, planted bool) *ledger.Ledger {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(toy, "ledger", "honest-count.jsonl"))
	if err != nil {
		t.Fatalf("the toy's honest-count ledger: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	pos := len(lines) + 1
	mark := ""
	if planted {
		mark = `,"planted":true`
	}
	lines = append(lines, fmt.Sprintf(
		`{"kind":"claim","position":%d,"actor":%q,"type":"user.created","ref":"%s.user#1","handles":["%s.user#1"]%s}`,
		pos, actor, actor, actor, mark))
	l, err := ledger.Load(strings.NewReader(strings.Join(lines, "\n")+"\n"), false)
	if err != nil {
		t.Fatalf("honest-count plus a user for %s does not load: %v", actor, err)
	}
	if err := l.SealAt(pos); err != nil {
		t.Fatalf("sealing at %d: %v", pos, err)
	}
	return l
}

// D14 (fable, coop r5/16): the preconditions evaluated under the PRECONDITION
// input must agree, rule by rule, with the same rules evaluated inside a REAL
// observation at the same position. A disagreement is a walk and a case
// contradicting each other about one world, which no other drill sees: the
// walk would offer an action the case's statute would not have permitted, or
// withhold one it would.
func TestThePreconditionInputAgreesWithARealObservation(t *testing.T) {
	ctx := context.Background()
	src := statute(t)
	l, obs, observer := sealedToy(t, "honest-count")

	full, err := engine.Evaluate(ctx, src, preconditions, l, engine.Check{
		Actor:  observer,
		Of:     str(obs["of"]),
		Query:  str(obs["query"]),
		Source: ledger.Source(str(obs["source"])),
		Value:  obs["value"],
		Role:   str(obs["role"]),
	})
	if err != nil {
		t.Fatalf("evaluating under a real observation: %v", err)
	}

	pin, err := engine.PreconditionInput(l, observer)
	if err != nil {
		t.Fatalf("building the precondition input: %v", err)
	}
	part, err := engine.EvaluateWith(ctx, src, preconditions, pin)
	if err != nil {
		t.Fatalf("evaluating under the precondition input: %v", err)
	}

	for _, rule := range preconditions {
		want, wok := full.Rules[rule]
		got, gok := part.Rules[rule]
		if wok != gok || want != got {
			t.Errorf("%s: real observation says %v (defined=%v), the precondition input says %v "+
				"(defined=%v) -- a walk and a case disagreeing about one world", rule, want, wok, got, gok)
		}
	}
	t.Logf("PROOF %d precondition(s) agree rule by rule across both inputs", len(preconditions))
}

// AND THE AGREEMENT HAS EXACTLY ONE EXCEPTION, which is fable's amendment to
// r5/16 at coop r5/97 and the cell that makes it visible: a claim the RUN
// PLANTED is in the check's input and not in the precondition's, so the two
// inputs MUST disagree where one exists.
//
// WHY THAT IS NOT A HOLE IN THE DRILL ABOVE. The drill asks whether a walk and
// a case describe one world. A planted claim is not a fact about the world: the
// system never moved (r5/58), so the wire knows nothing of it, while the
// observer does believe it -- and a check whose whole subject is that gap must
// see it. The disagreement is therefore the design, and an arm asserting
// agreement over a planted world would be asserting that the engine acts on its
// own fabrications.
//
// TWO WORLDS, DIFFERING IN ONE FIELD. The honest cell is the control, and it is
// what makes the planted cell attributable: the same actor, the same claim at
// the same position, `planted` the only difference. Without it this arm would
// also pass if `member_1` or the appended claim were what moved the verdicts.
//
// AND IT IS BY CONSTRUCTION. `has_user if made_user` reads the claim set, so
// stripping the claim is the whole mechanism; `may_create_user if not
// made_user` reads the same set the other way, and is asserted here because a
// strip that moved only one of the two would mean something narrower than what
// was ruled.
func TestAPlantedClaimIsInvisibleToAPreconditionAndVisibleToACheck(t *testing.T) {
	ctx := context.Background()
	src := statute(t)
	const actor = "member_1"

	// the observation is this actor's own, at the same position both paths read
	check := engine.Check{
		Actor:  actor,
		Of:     "user.created",
		Query:  "user.count",
		Source: ledger.SourceAPI,
		Value:  1,
		Role:   "member",
	}

	verdicts := func(l *ledger.Ledger, world string) (map[string]any, map[string]any) {
		t.Helper()
		full, err := engine.Evaluate(ctx, src, preconditions, l, check)
		if err != nil {
			t.Fatalf("%s world, real observation: %v", world, err)
		}
		pin, err := engine.PreconditionInput(l, actor)
		if err != nil {
			t.Fatalf("%s world, precondition input: %v", world, err)
		}
		part, err := engine.EvaluateWith(ctx, src, preconditions, pin)
		if err != nil {
			t.Fatalf("%s world, evaluating the precondition input: %v", world, err)
		}
		return full.Rules, part.Rules
	}

	// THE CONTROL: an HONEST user claim, where the two inputs must still agree
	// rule by rule -- the drill above over a world holding this actor's claim.
	honestObs, honestPre := verdicts(sealedToyWithUserFor(t, actor), "honest")
	for _, rule := range preconditions {
		if honestObs[rule] != honestPre[rule] {
			t.Fatalf("%s: an HONEST claim already splits the two inputs (observation %v, "+
				"precondition %v), so the planted cell below cannot attribute its difference "+
				"to `planted`", rule, honestObs[rule], honestPre[rule])
		}
	}
	if honestPre["has_user"] != true || honestPre["may_create_user"] != false {
		t.Fatalf("the control world does not hold the verdicts this arm subtracts from: "+
			"has_user %v, may_create_user %v", honestPre["has_user"], honestPre["may_create_user"])
	}

	// AND THE SAME WORLD WITH THE CLAIM PLANTED.
	plantedObs, plantedPre := verdicts(sealedToyWithPlantedUserFor(t, actor), "planted")

	if plantedObs["has_user"] != true {
		t.Errorf("a check must SEE the planted claim: has_user under a real observation is %v, "+
			"and a statute that cannot see a lie cannot find one", plantedObs["has_user"])
	}
	if plantedPre["has_user"] != false {
		t.Errorf("a precondition must NOT see the planted claim: has_user under the precondition "+
			"input is %v, so the walk would ask the wire for a session no create_user earned",
			plantedPre["has_user"])
	}
	if plantedObs["may_create_user"] != false {
		t.Errorf("may_create_user under a real observation is %v; the check's world holds the "+
			"claim, so the actor has made its user", plantedObs["may_create_user"])
	}
	if plantedPre["may_create_user"] != true {
		t.Errorf("may_create_user under the precondition input is %v; the store holds no user, "+
			"so the actor must still be able to create one -- a strip that left this false "+
			"would brick every actor a suborn touched", plantedPre["may_create_user"])
	}

	t.Logf("PROOF one planted claim: has_user is %v to a check and %v to a precondition, "+
		"may_create_user %v and %v, over %d rules whose honest verdicts agree",
		plantedObs["has_user"], plantedPre["has_user"],
		plantedObs["may_create_user"], plantedPre["may_create_user"], len(preconditions))
}

// AND THE ARM REACHES THE DEFECT IT EXISTS FOR, which fable now reads every R5
// battery for (coop r5/16, on rook's C1 review). The shape ruled OUT was
// omitting `observation` entirely -- my own first answer -- and ibis measured
// that it does not go undefined but answers wrongly in both directions. If this
// arm could not tell that apart it would be a tautology, so the omission is
// driven here and required to DISAGREE.
func TestOmittingTheObservationWouldHaveBeenCaught(t *testing.T) {
	ctx := context.Background()
	src := statute(t)
	l, _, observer := sealedToy(t, "honest-count")

	good, err := engine.PreconditionInput(l, observer)
	if err != nil {
		t.Fatalf("precondition input: %v", err)
	}
	ruled, err := engine.EvaluateWith(ctx, src, preconditions, good)
	if err != nil {
		t.Fatalf("evaluating the ruled shape: %v", err)
	}

	// the refuted shape: everything except the observation
	without := map[string]any{"known": good["known"], "claims": good["claims"]}
	omitted, err := engine.EvaluateWith(ctx, src, preconditions, without)
	if err != nil {
		t.Fatalf("evaluating the refuted shape: %v", err)
	}

	diff := 0
	for _, rule := range preconditions {
		if ruled.Rules[rule] != omitted.Rules[rule] {
			diff++
		}
	}
	if diff == 0 {
		t.Fatal("omitting `observation` changed no precondition's verdict, so the agreement arm " +
			"above cannot see the shape that was ruled out and proves nothing")
	}
	t.Logf("PROOF omitting `observation` changes %d of %d verdicts, so the arm above discriminates",
		diff, len(preconditions))
}

// Applicable reads those verdicts and answers with actions, and the case's own
// recorded expectations are the oracle: honest-count records may_leave and
// may_create_event true, the other three false.
func TestApplicableAnswersWhatThePreconditionsPermit(t *testing.T) {
	ctx := context.Background()
	src := statute(t)
	l, _, observer := sealedToy(t, "honest-count")

	rep, err := bindings.Load(filepath.Join(toy, "bindings.toml"), decl.TOML)
	if err != nil || !rep.OK() {
		t.Fatalf("the toy's bindings: %v %v", err, rep.Refusals)
	}

	got, err := schedule.Applicable(ctx, src, rep.Bindings.Actions, l, observer)
	if err != nil {
		t.Fatalf("Applicable: %v", err)
	}

	// BOTH DIRECTIONS, rook coop r5/25. This arm used to check only that
	// nothing IMPERMISSIBLE was offered, plus len != 0, and to LOG the set as
	// its proof. A false NEGATIVE was therefore invisible: rook renamed
	// `may_leave` in statute.rego and rules.json, leave_event dropped out of
	// every walk, and this arm passed while its own PROOF line printed the
	// shrunken set. For a walk whose job is exercising actions, an action
	// silently removed from the draw is the whole defect, so the set is now
	// ASSERTED -- and asserted as a SEQUENCE in declared order, which also
	// pins the "in declared order" that Applicable's doc comment promises and
	// nothing tested.
	//
	// THE ORACLE IS THE CORPUS, not a second evaluation: `want` is derived
	// from honest-count's recorded `expect` verdicts, which ibis checked
	// against this answer independently (coop r5/19). An evaluation of my own
	// here would make the arm agree with Applicable by construction.
	expect := recordedVerdicts(t, "honest-count")
	var want []string
	for _, a := range rep.Bindings.Actions {
		holds := true
		for _, r := range a.Requires {
			v, ok := expect[r]
			if !ok {
				t.Fatalf("action %q requires %q and honest-count's `expect` records no such "+
					"verdict: the bindings and the corpus disagree about a rule NAME, which is "+
					"the defect rook drove at coop r5/22 and the reason this arm reads both "+
					"documents rather than evaluating the statute twice", a.Name, r)
			}
			b, ok := v.(bool)
			if !ok {
				t.Fatalf("honest-count records %q as %T (%v); a precondition verdict is a boolean", r, v, v)
			}
			if !b {
				holds = false
				break
			}
		}
		if holds {
			want = append(want, a.Name)
		}
	}

	// THE ARM ASSERTS ITS OWN PRECONDITION. With fewer than two permitted
	// actions an equality check cannot see a DROP -- an all-false `expect`
	// would leave both sides empty and this arm would pass with leave_event
	// missing, which is exactly the defect. honest-count yields two.
	if len(want) < 2 {
		t.Fatalf("this arm needs at least two permitted actions to be able to see one go missing; "+
			"honest-count's recorded verdicts yield %v, so the arm cannot reach the defect it "+
			"exists for and would pass with it present", want)
	}

	gotNames := make([]string, 0, len(got))
	for _, a := range got {
		gotNames = append(gotNames, a.Name)
	}
	if !slices.Equal(gotNames, want) {
		t.Fatalf("Applicable answered %v and honest-count's recorded verdicts permit exactly %v, "+
			"in declared order: an action in neither list is a silent drop from every walk",
			gotNames, want)
	}
	t.Logf("PROOF applicable at the seal: %v (asserted against the corpus, both directions)", gotNames)
}

// recordedVerdicts is honest-count's own `expect` block -- the committed
// corpus's record of what the statute answers at that seal. It is read whole
// rather than filtered to `preconditions` so that a `requires` naming a rule
// the case does not record is a LOOKUP MISS the caller can report, instead of
// silently becoming a false verdict.
func recordedVerdicts(t *testing.T, name string) map[string]any {
	t.Helper()
	craw, err := os.ReadFile(filepath.Join(toy, "cases", name+".json"))
	if err != nil {
		t.Fatalf("the toy's %s case: %v", name, err)
	}
	var doc struct {
		Expect map[string]any `json:"expect"`
	}
	if err := json.Unmarshal(craw, &doc); err != nil {
		t.Fatalf("%s case: %v", name, err)
	}
	if len(doc.Expect) == 0 {
		t.Fatalf("%s records no `expect` block, so this arm has no oracle", name)
	}
	return doc.Expect
}

func str(v any) string {
	s, _ := v.(string)
	return s
}
