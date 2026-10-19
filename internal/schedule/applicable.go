package schedule

import (
	"context"
	"fmt"
	"sort"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
	"forge.axonibyte.com/axonibyte/rumor/internal/ledger"
)

// Applicable returns the actions every one of whose preconditions holds for
// `actor` at the sealed prefix, in declared order.
//
// WHAT A PRECONDITION IS SHOWN is engine.PreconditionInput, ruled by fable at
// coop r5/16: `observation` with `actor` and `position` and nothing else, and
// `known`/`claims` as a check sees them. It is evaluated by engine.EvaluateWith
// -- the SAME evaluator a case's finding goes through -- because a second
// evaluator over one statute would diverge, and a walk and a case disagreeing
// about one world is the fault D14 exists to catch.
//
// ONE EVALUATION FOR ALL THE RULES, not one per action: the rules are collected
// across every candidate action and asked once, so an action's applicability
// costs no OPA call of its own and two actions naming one rule cannot get two
// answers. Sorted before the call so the request is a function of the SET and
// not of the bindings' order, for D1's reason.
//
// A PRECONDITION THAT IS DEFINED AND FALSE IS NOT APPLICABLE, and one that is
// ABSENT is not applicable either -- those are different states and neither is
// a draw.
//
// THE REASON I FIRST GAVE FOR NOT SEPARATING THEM WAS FALSE, and it is
// recorded here because the code still reads the same way for a different
// reason. I wrote that `rumor validate`'s arms 1 and 2 "already hold that
// every `requires` names a rule the statute declares (contract.md:273)".
// Neither half is true. Arms 1 and 2 pair claim TYPES against `[claim.<type>]`
// tables; arm 3 is an action with NO requires; arm 11 pairs rules against
// mutants. And contract.md's `[[action]].requires` row names exactly one arm --
// the one on an action with no precondition -- so the contract never declared
// the property I attributed to it. rook drove the hole (coop r5/22): rename
// `may_leave` in statute.rego and rules.json, leave bindings.toml alone, and
// leave_event drops silently out of every walk. ibis measured the static layer
// with the adapter on path (coop r4/100): 18 of 18 arms evaluated, 0 refusals,
// exit 0. Nothing anywhere caught it.
//
// WHY THIS LAYER STILL DOES NOT REFUSE AN ABSENT VERDICT: absence is also how
// a rule legitimately declines, so refusing it would break the honest case --
// and the ambiguity is not a judgment call here, it is structural. Telling a
// MISSING NAME from a declining rule needs the statute's DECLARED rule set,
// and this function is not given one: it receives the statute source and the
// actions, never rules.json. The layer that reads both documents is
// `rumor validate`, which is where the static catch belongs (rook r5/26
// home 1); a nineteenth arm is fable's to rule in, not ours to add
// (contract.md:295), and ibis has it either way. Until then the only thing
// standing between a renamed rule and a silent drop is the corpus-level arm in
// tests/schedule, which now asserts the applicable SET in both directions
// against honest-count's recorded verdicts (rook r5/25, r5/27 home 3).
//
// A precondition answering something that is not a boolean IS refused: a
// project whose gate returns 3 has said something the walk cannot read, and
// guessing truthiness would invent a verdict.
func Applicable(ctx context.Context, statute string, actions []bindings.Action,
	l *ledger.Ledger, actor string) ([]bindings.Action, error) {
	if len(actions) == 0 {
		return nil, nil
	}
	input, err := engine.PreconditionInput(l, actor)
	if err != nil {
		return nil, fmt.Errorf("schedule: applicability for %s: %w", actor, err)
	}

	seen := map[string]bool{}
	var rules []string
	for _, a := range actions {
		for _, r := range a.Requires {
			if !seen[r] {
				seen[r] = true
				rules = append(rules, r)
			}
		}
	}
	sort.Strings(rules)

	verdicts := map[string]any{}
	if len(rules) > 0 {
		found, err := engine.EvaluateWith(ctx, statute, rules, input)
		if err != nil {
			return nil, fmt.Errorf("schedule: evaluating %d precondition rule(s) for %s: %w",
				len(rules), actor, err)
		}
		verdicts = found.Rules
	}

	var out []bindings.Action
	for _, a := range actions {
		holds := true
		for _, r := range a.Requires {
			v, defined := verdicts[r]
			if !defined {
				holds = false
				break
			}
			b, ok := v.(bool)
			if !ok {
				return nil, fmt.Errorf("schedule: precondition %q of action %q answered %T (%v) "+
					"and not a boolean: a walk cannot read that as permission, and guessing "+
					"truthiness would invent a verdict", r, a.Name, v, v)
			}
			if !b {
				holds = false
				break
			}
		}
		if holds {
			out = append(out, a)
		}
	}
	return out, nil
}
