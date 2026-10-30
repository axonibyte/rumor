package concurrency

import (
	"strings"
	"testing"
)

// THE OVERFILL CLAUSE AND ITS FOUR VACUITIES, all pure, so each branch is
// driveable without a server.
func TestOverfillIsAFindingAndNamesTheArithmetic(t *testing.T) {
	c := Contention{Action: "join_event", Limit: 1, Before: 0, Accepted: 2, Observed: 2, Width: 2}
	if why := c.Vacuous(); why != "" {
		t.Fatalf("a two-request fan-out into room for one was called unjudgeable: %s", why)
	}
	got := c.Finding()
	if got == "" {
		t.Fatal("two acceptances into room for one is not reported as an overfill")
	}
	for _, want := range []string{"join_event", "holding 2", "limit of 1", "1 more"} {
		if !strings.Contains(got, want) {
			t.Errorf("the finding does not carry %q, so a reader cannot check it against the "+
				"system's own rule: %s", want, got)
		}
	}

	// THE CONTROL IS THE SAME SETUP THE SYSTEM GOT RIGHT, which is what keeps
	// the clause from firing on any contended fan-out.
	ok := Contention{Action: "accept_invitation", Limit: 1, Before: 0, Accepted: 1, Observed: 1, Width: 2}
	if why := ok.Vacuous(); why != "" {
		t.Fatalf("the control setup was called unjudgeable: %s", why)
	}
	if f := ok.Finding(); f != "" {
		t.Errorf("a system that accepted exactly its room was reported as overfilling: %s", f)
	}
}

// EACH VACUITY IS NAMED AND THEY ARE ORDERED, so the reader is told the
// earliest thing wrong with the setup rather than a later consequence.
func TestEachVacuousSetupIsRefusedByName(t *testing.T) {
	for _, c := range []struct {
		name string
		c    Contention
		want string
	}{
		{"one request", Contention{Action: "join_event", Limit: 1, Before: 0, Accepted: 1, Observed: 1, Width: 1},
			"cannot overfill anything"},
		{"no room", Contention{Action: "join_event", Limit: 1, Before: 1, Accepted: 0, Observed: 1, Width: 2},
			"the setup's answer and not the system's"},
		{"room for everyone", Contention{Action: "join_event", Limit: 3, Before: 0, Accepted: 2, Observed: 2, Width: 2},
			"never under pressure"},
		{"nothing accepted", Contention{Action: "join_event", Limit: 1, Before: 0, Accepted: 0, Observed: 0, Width: 2},
			"never asked to choose"},
	} {
		why := c.c.Vacuous()
		if why == "" {
			t.Errorf("%s: read as judgeable, and the clause then says %q", c.name, c.c.Finding())
			continue
		}
		if !strings.Contains(why, c.want) {
			t.Errorf("%s: the refusal does not say %q: %s", c.name, c.want, why)
		}
	}
}

// THE MECHANISM ARM's OWN SETUP IS VACUOUS FOR THIS CLAUSE, and that is worth
// an arm of its own: tests/run's one-world arm runs capacity 3 against two
// joins and says "nothing here rests on the race". Read as a clean run it would
// be a fan-out that proved the limit holds while never pressing it -- rook's
// hazard at coop r8/37, that a clause firing on ANY overlapping fan-out passes
// identically. The oracle refuses that setup instead.
func TestTheMechanismArmsSetupIsRefusedRatherThanReadAsClean(t *testing.T) {
	asMechanismArm := Contention{Action: "join_event", Limit: 3, Before: 0, Accepted: 2, Observed: 2, Width: 2}
	why := asMechanismArm.Vacuous()
	if why == "" {
		t.Fatal("capacity 3 against two joins was read as a judgeable fan-out, so the arm that " +
			"deliberately rests on nothing would read as evidence that the limit holds")
	}
	if !strings.Contains(why, "never under pressure") {
		t.Errorf("the refusal does not say why that setup proves nothing: %s", why)
	}
	// AND IT IS NOT THE FINDING THAT SAVES IT: the clause itself is silent
	// there, which is precisely why the vacuity check has to speak.
	if f := asMechanismArm.Finding(); f != "" {
		t.Errorf("this arm's premise is gone: the clause now fires on that setup anyway: %s", f)
	}
}

// THE VERDICT IS ABOUT THE WORLD AND NOT ABOUT THE ANSWERS, which is rook's
// finding at coop r8/45. Counting acceptances made the clause "a claim about
// RESPONSES and not about the world": a system that answered 200 to two
// requests and persisted nothing read as an overfill.
func TestTheOverfillVerdictReadsTheWorldAndNotTheAnswers(t *testing.T) {
	// TWO ACCEPTANCES, NOTHING PERSISTED. The old clause called this an
	// overfill. It is a defect, but not this one.
	lying := Contention{Action: "join_event", Limit: 1, Before: 0, Accepted: 2, Observed: 0, Width: 2}
	if f := lying.Finding(); f != "" {
		t.Errorf("a system holding nothing was reported as overfilling its limit, so the clause "+
			"is reading its own answers rather than the world: %s", f)
	}
	// AND IT IS NOT SILENTLY DROPPED EITHER: the disagreement is its own
	// finding, named so a reader is sent to what an answer MEANS rather than
	// to the locking of a check-then-act.
	d := lying.Disagreement()
	if d == "" {
		t.Fatal("two acceptances over a world holding nothing produced no finding at all")
	}
	for _, want := range []string{"should hold 2", "holds 0", "not about this limit"} {
		if !strings.Contains(d, want) {
			t.Errorf("the disagreement does not carry %q: %s", want, d)
		}
	}

	// THE AGREEING RACE, which is what the shipped toy does: two acceptances
	// and the world holds two against a limit of one.
	raced := Contention{Action: "join_event", Limit: 1, Before: 0, Accepted: 2, Observed: 2, Width: 2}
	if d := raced.Disagreement(); d != "" {
		t.Errorf("the answers and the world agree here, so nothing should be reported: %s", d)
	}
	if raced.Finding() == "" {
		t.Error("a world holding two against a limit of one is not reported as an overfill")
	}

	// AND THE ROOM IS DERIVED, so a limit already partly taken is judged
	// correctly rather than as an overfill.
	partly := Contention{Action: "join_event", Limit: 3, Before: 2, Accepted: 1, Observed: 3, Width: 2}
	if partly.Room() != 1 {
		t.Errorf("room over limit 3 with 2 held is %d, wanted 1", partly.Room())
	}
	if why := partly.Vacuous(); why != "" {
		t.Fatalf("a limit with room for one against two requests was called unjudgeable: %s", why)
	}
	if f := partly.Finding(); f != "" {
		t.Errorf("a system that filled its last place correctly was reported as overfilling: %s", f)
	}
}
