package schedule

import (
	"time"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
)

// transitionStep is how far NextTransition walks before bisecting. A zone's
// offset changes at most twice a year, so a day is coarse enough to find the
// interval cheaply and fine enough that no transition hides inside one step:
// nothing in the database moves a clock twice within 24 hours.
const transitionStep = 24 * time.Hour

// NextTransition is the FIRST instant in (from, to] at which `loc`'s offset
// from UTC changes -- the hour that repeats and the hour that vanishes, which
// R7.5 exists to land on (issue 115, deliverable 2).
//
// ONE TRANSITION AND NOT EVERY ONE IN RANGE, which is fable's narrowing at coop
// r7/90(3) and the reason this returns a single instant: the landings list is
// handed to the pool's gate and the step's draw, so a list that grew with how
// far out a bound sits would move the draw's distribution for a project whose
// deadline is years away. The one after this appears once the clock has passed
// it, by the same call on the next step.
//
// THE OFFSET IS THE OBSERVABLE AND NOT A ZONE'S NAME. `time.Time.Zone()`
// answers the offset in effect at an instant, so a transition is where two
// adjacent instants disagree about it -- which is what a local clock
// misbehaving IS, and it needs no API for enumerating a zone's rules (Go
// exports none).
//
// WALK THEN BISECT, and the step is bounded rather than guessed: a day at a
// time until the offset differs from the one at `from`, then a binary search
// inside that day for the first nanosecond that carries the new offset. The
// search is exact -- it ends when the interval is one nanosecond wide -- so the
// instant returned is the transition itself and not a rounding of it, which is
// the distance the three landings are measured in.
//
// WHAT IT DOES NOT DO: answer for a zone with no transitions in range. `UTC`
// never changes offset and a window inside a single season holds none, and both
// come back `false` with nothing for the caller to append -- which is how "no
// transition in range adds nothing" is true of the landings list rather than
// merely intended for it.
func NextTransition(loc *time.Location, from, to int64) (int64, bool) {
	if loc == nil || to <= from {
		return 0, false
	}
	_, base := time.Unix(0, from).In(loc).Zone()
	lo := from
	for {
		hi := lo + int64(transitionStep)
		if hi > to {
			hi = to
		}
		_, off := time.Unix(0, hi).In(loc).Zone()
		if off != base {
			// THE FIRST INSTANT CARRYING THE NEW OFFSET, found exactly: the
			// invariant is that `lo` still reads the old offset and `hi` reads
			// a different one, so the answer is `hi` once they are adjacent.
			for hi-lo > 1 {
				mid := lo + (hi-lo)/2
				if _, m := time.Unix(0, mid).In(loc).Zone(); m == base {
					lo = mid
				} else {
					hi = mid
				}
			}
			return hi, true
		}
		if hi == to {
			return 0, false
		}
		lo = hi
	}
}

// zoneOf resolves a declared zone name, and EMPTY IS NO ZONE rather than UTC.
//
// THIS IS THE CELL THE PARSE COULD NOT HOLD (ratchet and ibis, coop
// rumor/1732). internal/bindings stores the NAME, so `zone = ""` and an absent
// key are one value there and no cell could tell them apart; here the name is
// RESOLVED, and `time.LoadLocation("")` SUCCEEDS with a real UTC -- so passing
// "" through is what would make a project that declared nothing gain UTC and
// every transition landing that follows from one.
//
// A NAME THAT DOES NOT RESOLVE IS nil AND NOT A PANIC: internal/bindings has
// already refused one at the parse, so reaching here means a Project was built
// by something other than `rumor run` -- a test, or a later caller -- and the
// walk it gets is today's rather than a crash.
// ONE LOOKUP SITE IN THE REPOSITORY, as of D5: the body is
// bindings.ZoneNamed, which is also what the parse refuses an unknown name
// with and what the ledger is handed. Two spellings of the empty-name trap is
// how one of them eventually loses the `if name == ""`; this keeps the
// nil-for-none DECISION here, where the cells for it live, and the LOOKUP
// there, where the name is declared.
func zoneOf(name string) *time.Location {
	loc, err := bindings.ZoneNamed(name)
	if err != nil {
		return nil
	}
	return loc
}

// untilsWithTransition is the bounds a step may land on, with the next zone
// transition among them when the project declares a zone (R7.5).
//
// IT IS A FUNCTION SO IT CAN BE DRIVEN. Inline at the call site the decision
// was reachable only through a whole Walk, and R7.5's three readings -- no
// zone, no window, no transition in range -- each want one assertion, which is
// ratchet's rule at coop r6/173 read for a branch rather than a refusal.
//
// NO LIVE BOUND MEANS NO WINDOW AND NOTHING JOINS, fable's /90(3) read
// strictly: with no bound there is no furthest one to search to, and a project
// declaring a zone but no deadline keeps the walk it already had. Appending to
// an empty list would make `advance` live where it is not today and move a
// committed reading the compatibility clause promises is unmoved.
//
// THE INPUT IS NOT GROWN IN PLACE, AND THE REASON IS THE CONTRACT RATHER THAN
// A LIVE HAZARD. A function handed a slice cannot see who else holds its
// array, so appending into spare capacity is a write through to a caller that
// may share one. ibis measured that today's caller does not: Register.Untils
// builds `make([]int64, 0, len(r.open))` fresh per call, so the hazard this
// cites is not live at the one production site (coop rumor/1740). The copy
// stays because the contract is the thing a later caller will rely on, and
// `zone_test.go` reads the BACKING ARRAY to hold it -- the slice header cannot
// see such a write, which is how a mutant doing it survived the first cell.
func untilsWithTransition(zone string, untils []int64, now int64) []int64 {
	loc := zoneOf(zone)
	if loc == nil || len(untils) == 0 {
		return untils
	}
	furthest := untils[0]
	for _, u := range untils {
		if u > furthest {
			furthest = u
		}
	}
	at, ok := NextTransition(loc, now, furthest)
	if !ok {
		return untils
	}
	out := make([]int64, len(untils), len(untils)+1)
	copy(out, untils)
	return append(out, at)
}
