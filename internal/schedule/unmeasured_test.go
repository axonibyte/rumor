package schedule

import (
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
)

// THE END-OF-RUN REFUSAL'S DECISION, DRIVEN DIRECTLY, because two of its
// clauses cannot be reached through a walk and a third cannot be reached
// through the product at all.
//
// WHY THESE ARE HERE AND NOT ONLY IN tests/clock. Two mutants survived the
// suite arms: keying the refusal on "a pending is open" instead of on the
// bound's FORM, and dropping the `clk.t != 0` guard so a run that HAD moved
// its clock still refused. Neither is reachable from a fixture today -- a
// quiesce-form pending open at the end needs a re-ask that cannot be made,
// which no committed run produces (ibis, coop rumor/1266), and nothing moves
// the clock until R7.3 draws `advance`. The decision is a function taking
// exactly the facts it judges, so the cells that a fixture cannot build are
// built here, and the narrowing stops being a sentence in a comment.
//
// THIS IS ratchet's r6/171 SHAPE USED ON PURPOSE rather than fallen into: a
// tested decision beside an untripped wiring is a defect when the wiring is
// what carries the risk. Here the WIRING is one call with no branching and the
// DECISION is where every clause lives, so the arms go where the clauses are
// and the walk-tier arms in tests/clock say the call happens at all.

func openOne(actor, query string) map[string][]string {
	return map[string][]string{actor: {query}}
}

func heldOne(actor, query, of string, at int) map[string]ask {
	return map[string]ask{
		askKey(actor, query): {
			decl:   bindings.Observation{Name: query, Of: of},
			actor:  actor,
			at:     at,
			chosen: map[string]string{},
		},
	}
}

func durationType(of string, bound int64) []engine.Eventual {
	return []engine.Eventual{{Of: of, Bound: bound}}
}

func TestADurationBoundedPendingOpenWithAnUnmovedClockRefuses(t *testing.T) {
	err := unmeasuredAtTheEnd(openOne("member_2", "event_tally"),
		heldOne("member_2", "event_tally", "event.tallied", 19),
		durationType("event.tallied", 30_000_000_000),
		&runClock{control: true}, nil, 0)
	if err == nil {
		t.Fatal("a pending left open under a duration bound whose clock never moved was not " +
			"refused: R2.3's invariant is that a run does not end holding one")
	}
	for _, want := range []string{
		"member_2/event_tally held at step 19",
		`"event.tallied" bounded 30s`,
		"moved no time",
		"still at its epoch",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not say %q\n%v", want, err)
		}
	}
}

func TestAClockThatMOVEDIsNotRefusedOn(t *testing.T) {
	// R7.3's MOVER STOOD IN FOR, as internal/schedule's clock arms do for the
	// same reason: `advance` does not exist, so the field is moved by hand.
	// What is under test is that the refusal reads the clock at all -- a build
	// that dropped this guard would refuse a run whose bound WAS measurable,
	// and would do it on the day `advance` lands and not before.
	err := unmeasuredAtTheEnd(openOne("member_2", "event_tally"),
		heldOne("member_2", "event_tally", "event.tallied", 19),
		durationType("event.tallied", 30_000_000_000),
		&runClock{control: true, t: 90_000_000_000}, nil, 0)
	if err != nil {
		t.Fatalf("a run that moved its clock to 90s was refused for not moving time: the bound "+
			"was measurable and the pending is a verdict the engine owns, not a refusal\n%v", err)
	}
}

func TestAQuiesceBoundedPendingOpenIsNotThisRefusalsBusiness(t *testing.T) {
	// THE NARROWING, ARMED. A pending left open under a QUIESCE bound is R6's
	// state and R6 judges it: the probes read quiet or they did not, and
	// neither answer needs a clock. A refusal keyed on "a pending is open"
	// would change what rook's D1 and D3 read and move the control seeds,
	// which is a different ruling from the one fable made at coop r7/15.
	//
	// THE DURATION TYPE IS DECLARED BESIDE IT ON PURPOSE: without one the
	// function returns early on "this project has no duration bound", and the
	// cell would pass for a reason that says nothing about the quiesce
	// pending.
	eventual := []engine.Eventual{
		{Of: "event.tallied", Quiesce: true},
		{Of: "event.joined", Bound: 30_000_000_000},
	}
	err := unmeasuredAtTheEnd(openOne("member_2", "event_tally"),
		heldOne("member_2", "event_tally", "event.tallied", 19),
		eventual, &runClock{control: true}, nil, 0)
	if err != nil {
		t.Fatalf("a quiesce-bounded pending was refused by the duration bound's refusal, so the "+
			"refusal is keyed on a pending being open rather than on the bound's form\n%v", err)
	}
}

func TestAProjectWithNoClockControlIsNotRefusedOnHere(t *testing.T) {
	// The engine refuses a duration bound it cannot measure at the moment of
	// judging, which is where a project declaring no clock is caught. Firing
	// here as well would report the same defect twice and name the run as the
	// cause when the project is.
	err := unmeasuredAtTheEnd(openOne("member_2", "event_tally"),
		heldOne("member_2", "event_tally", "event.tallied", 19),
		durationType("event.tallied", 30_000_000_000),
		&runClock{control: false}, nil, 0)
	if err != nil {
		t.Fatalf("a project with no clock control was refused by the END-OF-RUN refusal, which "+
			"names the RUN as the cause for a project that never gave it a clock\n%v", err)
	}
}

func TestAPendingTheWalkCouldNotPlaceIsNotRefusedOn(t *testing.T) {
	// WITHOUT ITS `ask` THERE IS NO CLAIM TYPE, so there is no bound form to
	// key on. stillOpen names these in the report; guessing here would turn an
	// unattributable pending into a refusal about a bound nobody declared.
	err := unmeasuredAtTheEnd(openOne("member_2", "event_tally"),
		map[string]ask{}, durationType("event.tallied", 30_000_000_000),
		&runClock{control: true}, nil, 0)
	if err != nil {
		t.Fatalf("a pending the walk could not place was refused on, so the refusal is naming a "+
			"bound it cannot know this pending has\n%v", err)
	}
}

func TestTheRefusalNamesAFindingItWithholds(t *testing.T) {
	found := engine.Finding{Defined: true, Value: "bug", Rule: "data.rumor.statute.finding"}
	err := unmeasuredAtTheEnd(openOne("member_2", "event_tally"),
		heldOne("member_2", "event_tally", "event.tallied", 19),
		durationType("event.tallied", 30_000_000_000),
		&runClock{control: true}, &found, 24)
	if err == nil {
		t.Fatal("not refused")
	}
	for _, want := range []string{"the run also found bug", "after step 24", "where the walk stopped asking"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not say %q, so a finding is dropped in silence\n%v", want, err)
		}
	}
}
