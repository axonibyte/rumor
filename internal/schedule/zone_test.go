package schedule

import (
	"testing"
	"time"

	"forge.axonibyte.com/axonibyte/rumor/internal/proof"
)

// chicagoTransitions2026 is THE SECOND ORACLE, AND IT IS ARITHMETIC RATHER THAN
// tzdata (ratchet, coop r7/106; fable's acceptance at /107). A check that asked
// the database the code asks would agree with itself whatever the code did, so
// these two instants come from the PUBLISHED RULE -- US daylight time begins on
// the second Sunday in March and ends on the first Sunday in November, the
// change at 02:00 local -- and the cell proves its own dates are those Sundays
// rather than trusting the author's calendar arithmetic.
//
//	spring  2026-03-08 02:00 CST (UTC-6) -> 03:00 CDT, so 08:00Z
//	fall    2026-11-01 02:00 CDT (UTC-5) -> 01:00 CST, so 07:00Z
func chicagoTransitions2026(t *testing.T) (spring, fall int64) {
	t.Helper()
	springDay := time.Date(2026, time.March, 8, 0, 0, 0, 0, time.UTC)
	fallDay := time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC)
	// THE DATES ARE CHECKED, NOT ASSUMED: a wrong Sunday would make this
	// oracle agree with nothing, and it is the half of the arithmetic a reader
	// cannot verify at a glance.
	if d := springDay.Weekday(); d != time.Sunday {
		t.Fatalf("2026-03-08 is a %s: the rule is the SECOND Sunday in March, so this "+
			"oracle's own date is wrong before the code is asked anything", d)
	}
	if springDay.Day() < 8 || springDay.Day() > 14 {
		t.Fatalf("2026-03-08 is day %d of March, which is not the second Sunday's range 8-14",
			springDay.Day())
	}
	if d := fallDay.Weekday(); d != time.Sunday {
		t.Fatalf("2026-11-01 is a %s: the rule is the FIRST Sunday in November", d)
	}
	if fallDay.Day() > 7 {
		t.Fatalf("2026-11-01 is day %d of November, which is not the first Sunday's range 1-7",
			fallDay.Day())
	}
	return time.Date(2026, time.March, 8, 8, 0, 0, 0, time.UTC).UnixNano(),
		time.Date(2026, time.November, 1, 7, 0, 0, 0, time.UTC).UnixNano()
}

// TestNextTransitionFindsTheHourThatMOVESAndNothingElse drives R7.5's reader of
// the zone database: the first instant in (from, to] at which the offset
// changes, which is the hour that repeats and the hour that vanishes.
//
// WHAT IT HOLDS: both of a year's transitions, each to the NANOSECOND, against
// instants computed from the published rule; a zone that never changes offset;
// a window inside one season; and the bound being EXCLUSIVE of `from` and
// INCLUSIVE of `to`, which is what makes "the one after it appears once the
// clock has passed it" true rather than hoped for.
func TestNextTransitionFindsTheHourThatMOVESAndNothingElse(t *testing.T) {
	chicago, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Fatalf("America/Chicago does not resolve, so this box carries no zone database "+
			"and the reading is UNMEASURED rather than clean: %v", err)
	}
	spring, fall := chicagoTransitions2026(t)
	jan := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC).UnixNano()
	dec := time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC).UnixNano()

	if got, ok := NextTransition(chicago, jan, dec); !ok || got != spring {
		t.Errorf("over all of 2026 the first transition is (%d, %v), want (%d, true) -- the "+
			"SECOND Sunday in March at 02:00 local, which is 08:00Z", got, ok, spring)
	}
	// AND THE ONE AFTER IT ONLY ONCE THE CLOCK HAS PASSED THE FIRST, which is
	// the whole of fable's narrowing: the list never holds two.
	if got, ok := NextTransition(chicago, spring, dec); !ok || got != fall {
		t.Errorf("from the spring instant the next transition is (%d, %v), want (%d, true) -- "+
			"the FIRST Sunday in November at 02:00 local, which is 07:00Z", got, ok, fall)
	}
	// EXCLUSIVE OF `from`: standing exactly ON a transition does not find it
	// again, or a walk parked there would be offered the same landing forever.
	if got, ok := NextTransition(chicago, spring, spring+int64(time.Hour)); ok {
		t.Errorf("standing on the spring transition finds %d in the next hour: the search is "+
			"exclusive of `from`, or a clock parked on a transition lands on it repeatedly", got)
	}
	// INCLUSIVE OF `to`: a bound exactly AT a transition still offers it.
	if got, ok := NextTransition(chicago, jan, spring); !ok || got != spring {
		t.Errorf("a window ending exactly at the transition gives (%d, %v), want (%d, true): "+
			"`to` is a live bound and the instant is reachable", got, ok, spring)
	}
	// A TRANSITION INSIDE THE FIRST STEP, which is the reading the `dst`
	// profile will live in: D3 places the clock inside the DAY BEFORE a
	// transition, so the instant is less than one step away. A search that
	// began a step late would pass every assertion above and miss exactly the
	// case this child is built for -- measured, it survived until this line.
	if got, ok := NextTransition(chicago, spring-int64(time.Hour), spring+int64(time.Hour)); !ok || got != spring {
		t.Errorf("an hour either side of the spring instant gives (%d, %v), want (%d, true): "+
			"the walk must find a transition inside its first step, which is where the `dst` "+
			"profile's committed seed starts", got, ok, spring)
	}
	// A ZONE THAT NEVER MOVES, and a window inside one season: both are "no
	// transition in range", which is what makes an unchanged landings list a
	// measured property of this function rather than an intention.
	if got, ok := NextTransition(time.UTC, jan, dec); ok {
		t.Errorf("UTC reports a transition at %d: a zone with no rules has none to find", got)
	}
	jun := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC).UnixNano()
	jul := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC).UnixNano()
	if got, ok := NextTransition(chicago, jun, jul); ok {
		t.Errorf("June to July reports a transition at %d, and Chicago has none between them",
			got)
	}
	if !t.Failed() {
		proof.Logf(t, "PROOF both of 2026's Chicago transitions are found to the nanosecond against "+
			"the published rule (%d and %d), the window is exclusive of `from` and inclusive "+
			"of `to`, and UTC and a single season report none", spring, fall)
	}
}

// TestOnlyTheNextTransitionJoinsTheBounds drives the DECISION at the walk's one
// site: which bounds a step may land on once a zone is declared.
//
// IT IS A FUNCTION FOR THIS REASON. Inline, these four readings were reachable
// only through a whole Walk; here each is one assertion, and the list that
// `Landings` turns into instants is the thing being checked rather than the
// instants themselves, which `Landings`'s own arm already holds.
func TestOnlyTheNextTransitionJoinsTheBounds(t *testing.T) {
	if _, err := time.LoadLocation("America/Chicago"); err != nil {
		t.Fatalf("no zone database on this box: UNMEASURED, not clean: %v", err)
	}
	spring, _ := chicagoTransitions2026(t)
	jan := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC).UnixNano()
	apr := time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC).UnixNano()
	jun := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC).UnixNano()
	jul := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC).UnixNano()

	// THE RESOLVER IS DRIVEN DIRECTLY, because through the bounds an empty zone
	// and UTC are INDISTINGUISHABLE: UTC has no transitions, so "" resolving to
	// it would add nothing and every assertion below would still pass. Measured
	// -- a mutant returning time.UTC for "" survived until this loop existed,
	// which is the deferred cell actually landing rather than being named.
	for name, want := range map[string]bool{"": false, "Not/AZone": false, "America/Chicago": true} {
		if got := zoneOf(name) != nil; got != want {
			t.Errorf("zoneOf(%q) resolves: %v, want %v -- LoadLocation(\"\") SUCCEEDS with a "+
				"real UTC, so an empty name must be refused HERE or a project that declared "+
				"nothing silently reads its clock in UTC", name, got, want)
		}
	}
	// NO ZONE IS TODAY'S WALK, and `zone = ""` is no zone -- the cell the parse
	// could not hold, because internal/bindings stores a NAME and "" and an
	// absent key are one value there (ibis, coop rumor/1732).
	for _, zone := range []string{"", "Not/AZone"} {
		got := untilsWithTransition(zone, []int64{apr}, jan)
		if len(got) != 1 || got[0] != apr {
			t.Errorf("zone %q gave bounds %v, want exactly the declared one: an empty or "+
				"unresolvable zone is NO zone, and LoadLocation(\"\") answering a real UTC is "+
				"what would otherwise make a project that declared nothing gain one", zone, got)
		}
	}
	// A ZONE AND A BOUND PAST A TRANSITION: one extra bound, the transition.
	got := untilsWithTransition("America/Chicago", []int64{apr}, jan)
	if len(got) != 2 || got[0] != apr || got[1] != spring {
		t.Errorf("bounds are %v, want [%d %d]: the next transition joins as a BOUND, which is "+
			"what makes its three landings fall out of Landings unchanged", got, apr, spring)
	}
	// NO TRANSITION IN RANGE ADDS NOTHING: the list is the same as today's.
	if got := untilsWithTransition("America/Chicago", []int64{jul}, jun); len(got) != 1 || got[0] != jul {
		t.Errorf("June to July gave %v, want exactly [%d]: a window with no transition in it "+
			"leaves the walk it had", got, jul)
	}
	// TWO BOUNDS, AND THE WINDOW REACHES THE FURTHEST. Every case above holds
	// ONE bound, where the nearest and the furthest are the same instant -- so
	// a reader taking the NEAREST passed all of them, and fable's /90(3)
	// window was armed only where the two could not differ (ibis, coop
	// rumor/1740). Here the transition lies between them: with the nearest it
	// is out of range and nothing joins.
	feb := time.Date(2026, time.February, 15, 0, 0, 0, 0, time.UTC).UnixNano()
	dec := time.Date(2026, time.December, 15, 0, 0, 0, 0, time.UTC).UnixNano()
	if got := untilsWithTransition("America/Chicago", []int64{feb, dec}, jan); len(got) != 3 ||
		got[2] != spring {
		t.Errorf("bounds {feb, dec} at jan gave %v, want the two plus %d: the window runs to "+
			"the FURTHEST live bound, and a reader taking the nearest finds nothing between "+
			"January and February", got, spring)
	}
	// AND THE OTHER END OF THE SAME CLAUSE: a furthest bound at or BEFORE now
	// is no window at all, which `to <= from` is for.
	if got := untilsWithTransition("America/Chicago", []int64{jan}, apr); len(got) != 1 {
		t.Errorf("a bound BEFORE the clock gave %v, want it alone: there is no window from "+
			"April back to January, and a search that ran anyway would offer a landing the "+
			"mover refuses for moving time backward", got)
	}
	// AND NO LIVE BOUND MEANS NO WINDOW: nothing joins, so `advance` does not
	// become live where it is not today (fable, coop r7/90(3)).
	if got := untilsWithTransition("America/Chicago", nil, jan); len(got) != 0 {
		t.Errorf("an empty bound list gave %v, want none: with no furthest bound there is no "+
			"window to search, and appending here would make `advance` live in a zone-declared "+
			"project that declares no deadline", got)
	}
	// THE CALLER'S SLICE IS NOT GROWN IN PLACE, and the reason is the
	// CONTRACT rather than a live hazard: ownership is not visible from inside
	// a function that is handed a slice, so a future caller sharing one would
	// be written through. ibis measured that today's caller is not that
	// caller -- Register.Untils builds a fresh slice per call -- so what this
	// holds is the contract and not a defect it prevents (coop rumor/1740).
	owned := make([]int64, 1, 4)
	owned[0] = apr
	backing := owned[:cap(owned)]
	_ = untilsWithTransition("America/Chicago", owned, jan)
	// THE HEADER IS THE WRONG ORACLE AND THAT IS THE POINT: `owned` keeps
	// len 1 whatever append does, so a write into its spare capacity is
	// invisible there and a mutant doing exactly that survived this cell until
	// it read the ARRAY. What another holder of the same array would see is
	// backing[1].
	if backing[1] != 0 {
		t.Errorf("the caller's spare capacity now holds %d: appending into a borrowed slice "+
			"writes through to whatever else holds that array, and `reg.Untils()` hands out "+
			"a slice the register owns", backing[1])
	}
	if !t.Failed() {
		proof.Logf(t, "PROOF only the next transition joins, an empty or unresolvable zone joins "+
			"nothing, a window with no transition joins nothing, no bound means no window, "+
			"and the caller's slice is left alone")
	}
}

// AND THE COMPOSITION IS THE CLAIM 115 MAKES: "the landings list holds the
// three transition instants in addition to the three deadline instants".
//
// WHY A CELL OVER BOTH FUNCTIONS AND NOT EACH. `untilsWithTransition` answers
// BOUNDS and `Landings` turns each bound into three, and the cells above and
// R7's own hold each half -- but the sentence the issue asks for is about what
// a step may draw, which is the composition walk.go performs. Nothing asserted
// it, so a change that produced a transition BOUND and lost one of its three
// LANDINGS would have passed both halves.
//
// SIX INSTANTS AND NOT FIVE OR SEVEN: a deadline and a transition, three each,
// and this names all six rather than counting them -- a count passes on three
// of one and three of the other in the wrong places, which is the mutation
// 115's test line asks for ("the three transition landings removed one at a
// time").
func TestTheLandingsHoldTheTransitionsThreeBesideTheDeadlinesThree(t *testing.T) {
	spring, _ := chicagoTransitions2026(t)
	// THE CLOCK SITS BEFORE BOTH and the deadline is past the transition, so
	// every one of the six is ahead of now and none is dropped for being
	// behind it -- the condition the issue states as "a bound past a
	// transition".
	now := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC).UnixNano()
	deadline := time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC).UnixNano()

	got := Landings(untilsWithTransition("America/Chicago", []int64{deadline}, now), now)
	held := map[int64]bool{}
	for _, at := range got {
		held[at] = true
	}
	for _, want := range []struct {
		what string
		at   int64
	}{
		{"one tick before the transition", spring - Tick},
		{"the transition itself", spring},
		{"one tick past the transition", spring + Tick},
		{"one tick before the deadline", deadline - Tick},
		{"the deadline itself", deadline},
		{"one tick past the deadline", deadline + Tick},
	} {
		if !held[want.at] {
			t.Errorf("the landings do not hold %s (%d): %v", want.what, want.at, got)
		}
	}
	// AND NOTHING ELSE, because a landing nobody declared is an instant a step
	// may put the clock on for no stated reason.
	if len(got) != 6 {
		t.Errorf("the landings hold %d instant(s), want exactly the six named above: %v",
			len(got), got)
	}
	// AND WITHOUT THE ZONE THE SAME CALL HOLDS THREE, which is what makes the
	// six a measurement about the transition rather than about Landings.
	if bare := Landings(untilsWithTransition("", []int64{deadline}, now), now); len(bare) != 3 {
		t.Errorf("with no zone the landings hold %d instant(s), want the deadline's three: %v",
			len(bare), bare)
	}
	if !t.Failed() {
		proof.Logf(t, "PROOF a declared zone and one bound past the transition give six landings -- "+
			"%d±1 and %d±1 -- and no zone gives three", spring, deadline)
	}
}
