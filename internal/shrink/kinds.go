package shrink

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"forge.axonibyte.com/axonibyte/rumor/internal/rollback"
)

// KindReduction is what a kind-removal pass established.
type KindReduction struct {
	// Removed are the kinds the trace reproduced without, in the order they
	// were dropped.
	Removed []string

	// Probes counts the candidate traces run, each on its own verified clean
	// world.
	Probes int
}

// RemoveKinds drops whole action kinds from a failing trace, keeping each
// removal that still reproduces.
//
// IT IS THE SECOND HALF OF design.md's ORDER -- "length bisection first, then
// removal of whole action kinds" -- and it runs AFTER a bisection because a
// shorter trace has fewer kinds to try, so the cheap pass goes first.
//
// GREEDY AND CUMULATIVE: a kind that can go stays gone while the next is tried,
// so the result is a trace with several kinds removed rather than the best
// single removal. That is what makes it a reduction and not a survey.
//
// THE ORDER IS SORTED, AND THAT IS A DETERMINISM DECISION rather than a
// preference. Removing A then B can end somewhere other than B then A, so the
// result is a function of the order the kinds are tried in -- and a caller that
// built its slice from a map would hand over a different order on each run,
// which would make one seed shrink two ways. Sorting makes the reduction a
// function of the kind SET, which is what two attempts from one seed actually
// agree on.
//
// AN EMPTY TRACE THAT REPRODUCES IS A REFUSAL, not a triumph. If dropping the
// last kind leaves nothing and the finding still appears, the finding is not a
// property of the trace at all -- the world was dirty, or the statute answers
// the same way over an empty ledger -- and reporting "shrunk to zero actions"
// would be the shrinker's most confident wrong answer. The replay gate
// establishes the failure is real before any of this; this says so again at the
// one point where the trace stops existing.
func RemoveKinds(ctx context.Context, pr rollback.Provider, cp rollback.Checkpoint,
	want Want, kinds []string,
	run func(ctx context.Context, without []string) (Candidate, error)) (KindReduction, error) {

	if run == nil {
		return KindReduction{}, fmt.Errorf("shrink: no run function, so no candidate trace can be " +
			"replayed without a kind")
	}

	// A FIXED ORDER OVER A DEDUPLICATED SET: a kind named twice would be tried
	// twice, and the second attempt would be removing what is already gone.
	seen := map[string]bool{}
	ordered := make([]string, 0, len(kinds))
	for _, k := range kinds {
		if k == "" {
			return KindReduction{}, fmt.Errorf("shrink: the kind list holds an empty name, and a " +
				"trace cannot be replayed without a kind that is not named")
		}
		if !seen[k] {
			seen[k] = true
			ordered = append(ordered, k)
		}
	}
	sort.Strings(ordered)

	out := KindReduction{}
	var removed []string
	for _, k := range ordered {
		candidate := append(append([]string{}, removed...), k)

		// THE LAST KIND LEAVING NOTHING IS WHERE THE GUARD BITES: if this
		// candidate removes every kind there was, the trace it describes is
		// empty.
		empty := len(candidate) == len(ordered)

		out.Probes++
		v, err := Once(ctx, pr, cp, want, func(c context.Context) (Candidate, error) {
			return run(c, candidate)
		})
		if err != nil {
			return out, fmt.Errorf("shrink: the candidate without %s: %w",
				strings.Join(candidate, ", "), err)
		}
		if !v.Reproduced {
			continue
		}
		if empty {
			return out, fmt.Errorf("shrink: the trace reproduced with every kind removed (%s), so "+
				"the finding is not a property of the trace: either the world was not clean or the "+
				"statute answers the same way over an empty ledger. Refusing to report a shrink to "+
				"zero actions", strings.Join(candidate, ", "))
		}
		removed = candidate
	}
	out.Removed = removed
	return out, nil
}
