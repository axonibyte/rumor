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
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/decl"
	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
	"forge.axonibyte.com/axonibyte/rumor/internal/ledger"
	"forge.axonibyte.com/axonibyte/rumor/internal/schedule"
)

// the five rules every toy action's `requires` names, which ibis measured as
// the only preconditions in the tree (coop r5/3).
var preconditions = []string{
	"may_create_event", "may_create_user", "may_delete", "may_join", "may_leave",
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
	var names []string
	for _, a := range got {
		names = append(names, a.Name)
	}
	sort.Strings(names)

	// every returned action's preconditions must be among the two the case
	// records as true; nothing requiring may_join, may_leave's falsehoods or
	// may_delete may appear.
	permitted := map[string]bool{"may_leave": true, "may_create_event": true}
	for _, a := range got {
		for _, r := range a.Requires {
			if !permitted[r] {
				t.Errorf("Applicable offered %q, which requires %q -- the case records that false",
					a.Name, r)
			}
		}
	}
	if len(got) == 0 {
		t.Error("Applicable offered nothing, and honest-count records two preconditions true")
	}
	t.Logf("PROOF applicable at the seal: %v", names)
}

func str(v any) string {
	s, _ := v.(string)
	return s
}
