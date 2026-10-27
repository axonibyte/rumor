package engine

import (
	"sort"

	"forge.axonibyte.com/axonibyte/rumor/internal/ledger"
)

// WHO A PASS WOULD OBSERVE, and that is all this file now holds.
//
// IT WAS reconcile.go UNTIL R6.4. `Reconcile` put every pass to every actor and
// returned a finding per pair, and `Pass`, `Report`, `Key` and `PlantedBy` were
// its input, its output, its map keys and its report -- six symbols that no
// binary ever reached. fable ruled the removal at coop r6/153: R10 explains
// findings and does not inherit dead engine code, and the R6 gate does not
// carry a dead production symbol into R7. The walk's own reconciliation goes
// through Judge (91, walk.go), so wiring the candidate path through Reconcile
// would have been a second reconciliation path built to make a symbol live.
//
// Actors SURVIVES BECAUSE IT HAS ITS OWN ARM AND ITS OWN MEANING, measured by
// the rename probe rather than assumed: renaming the other four broke nothing,
// renaming this one broke the arm below.
//
// WHAT "ALL HONEST ACTORS" IS READ AS, and it is a reading rather than a
// restatement. v1 tags CLAIMS with `planted`, not actors, and says outright
// that "there is no trust model". An actor can hold one honest claim and one
// planted one, so a pass observes EVERY actor named in the sealed prefix and
// honesty stays a property of the claims a statute discounts.
func Actors(l *ledger.Ledger) []string {
	seen := map[string]bool{}
	for _, f := range l.SealedPrefix() {
		if f.Kind == ledger.KindClock {
			continue
		}
		if f.Actor != "" {
			seen[f.Actor] = true
		}
		if f.Kind == ledger.KindTold && f.To != "" {
			seen[f.To] = true
		}
	}
	out := make([]string, 0, len(seen))
	for a := range seen {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}
