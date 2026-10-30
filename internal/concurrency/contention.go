package concurrency

import "fmt"

// Contention is what one fan-out observed: how many requests the system
// ACCEPTED against how much room its own declared limit left.
//
// ROOM AND NOT CAPACITY, because the two differ whenever anything is already
// there: a capacity of 3 with two attendees leaves room for one, and a clause
// written against the capacity would call a correct third acceptance an
// overfill. The caller computes the room from the state it set up, which is the
// only place that knows it.
//
// WHY ACCEPTANCE IS THE CALLER's TO DETERMINE: what counts as the system taking
// an action is the project's own declaration -- `[[action]].response` is an
// equality contract for exactly that (docs/contract.md) -- so this package
// takes the COUNT and asserts nothing about the transport. A status comparison
// in here would be this package inventing a second answer to a question the
// bindings already answer.
type Contention struct {
	// Action is the action the fan-out contended on, for the finding's words.
	Action string
	// Limit is the system's own declared limit on the thing being contended.
	Limit int
	// Before is how much of that limit was already taken when the fan-out was
	// released, so the ROOM is derived rather than passed: a caller that
	// computed the room itself could get it wrong in the direction that hides
	// a finding, and the two numbers it is computed from are the ones the
	// setup actually knows.
	Before int
	// Accepted is how many of the fan-out the system ANSWERED acceptance to.
	Accepted int
	// Observed is what the system HOLDS afterwards, read back through a
	// declared observation.
	//
	// THE VERDICT RESTS ON THIS AND NOT ON Accepted, which is rook's finding
	// at coop r8/45 against the head before this one: counting 2xx answers
	// makes the clause "a claim about RESPONSES and not about the world", so a
	// system answering 200 and persisting nothing read as an overfill. The
	// race's signature is the system's own quantity -- len(Attendees) >
	// Capacity -- and examples/toy already declares the observation that reads
	// it, store_event_count, whose own comment says why it is the right one:
	// "computed from the attendee list ... an oracle that read the same field
	// could not disagree with it".
	Observed int
	// Width is how many requests were issued.
	Width int
}

// Room is how many of the fan-out the system could accept without exceeding
// its own limit.
//
// DERIVED AND NOT DECLARED, because the limit and what was already taken are
// facts the setup holds while the room is arithmetic over them -- and a clause
// written against the LIMIT alone would call a correct acceptance an overfill
// whenever anything was already there.
func (c Contention) Room() int { return c.Limit - c.Before }

// Vacuous is why this fan-out cannot answer the overfill clause, or "" when it
// can.
//
// THE CLAUSE IS AN ABSENCE CLAIM, SO IT NEEDS ITS PRECONDITIONS STATED. "No
// overfill" is what a correct system and an untested one both report, and three
// setups make it meaningless:
//
// NO ROOM AT ALL. With room below one every request must refuse whatever the
// system does, so "no overfill" is guaranteed by the setup rather than by the
// system -- the shape of a control that is the case it controls for.
//
// MORE ROOM THAN CONTENDERS. With room at or above the width every request can
// be accepted legitimately, so the limit is never under pressure and a system
// with no check at all would pass. This is the one the mechanism arm sits in
// deliberately -- it runs capacity 3 with two joins because nothing there is
// meant to rest on the race -- which is exactly why the oracle must refuse it
// rather than read it as a clean run.
//
// NOTHING ACCEPTED. If the system took none of them, it was never asked to
// decide between them: every request refused for some earlier reason, and the
// limit was not exercised.
func (c Contention) Vacuous() string {
	if c.Width < 2 {
		return fmt.Sprintf("%s was contended by %d request(s): one request in flight cannot "+
			"overfill anything, so the clause has nothing to judge", c.Action, c.Width)
	}
	if c.Room() < 1 {
		return fmt.Sprintf("%s had room for %d: every request must refuse whatever the system "+
			"does, so \"no overfill\" is the setup's answer and not the system's",
			c.Action, c.Room())
	}
	if c.Room() >= c.Width {
		return fmt.Sprintf("%s had room for %d against %d request(s): the limit was never under "+
			"pressure, so a system with no check at all would pass this fan-out",
			c.Action, c.Room(), c.Width)
	}
	if c.Accepted == 0 {
		return fmt.Sprintf("%s accepted none of %d request(s): the system was never asked to "+
			"choose between them, so its limit was not exercised", c.Action, c.Width)
	}
	return ""
}

// Disagreement is the responses and the world telling different stories, or ""
// when they agree.
//
// IT IS ITS OWN FINDING AND NOT AN OVERFILL. A system that answered acceptance
// to two requests and holds one has a defect, but not THIS defect -- and
// reporting it as an overfill would send a reader to the locking of a
// check-then-act when the fault is that an answer was not the truth. rook
// measured the agreeing case at coop r8/45: after the race at capacity 1, the
// declared observation reads 2 against two acceptances, so the pair agrees on
// the shipped toy and this clause is a guard rather than a live finding.
func (c Contention) Disagreement() string {
	if c.Observed == c.Before+c.Accepted {
		return ""
	}
	return fmt.Sprintf("%s answered acceptance to %d of %d request(s) over %d already held, so "+
		"the system should hold %d -- and it holds %d. The answers and the world disagree, which "+
		"is a defect about what an answer MEANS and not about this limit",
		c.Action, c.Accepted, c.Width, c.Before, c.Before+c.Accepted, c.Observed)
}

// Finding is the overfill this fan-out found, or "" when it found none.
//
// THE QUANTITY IS THE SYSTEM's OWN, read back rather than inferred from its
// answers: the race's signature is that it HOLDS more than its limit allows.
// It names the arithmetic and not the verdict, because "a race was found"
// cannot be acted on while the numbers can be checked against the rule.
func (c Contention) Finding() string {
	if c.Observed <= c.Limit {
		return ""
	}
	return fmt.Sprintf("%s left the system holding %d against its own limit of %d: %d more than "+
		"it allows, after %d request(s) were answered acceptance over %d already held. The check "+
		"it makes and the insert it does are not one step",
		c.Action, c.Observed, c.Limit, c.Observed-c.Limit, c.Accepted, c.Before)
}
