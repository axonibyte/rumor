package concurrency

import (
	"strings"
	"testing"
)

// THE OVERFILL CLAUSE AND ITS FOUR VACUITIES, all pure, so each branch is
// driveable without a server.
func TestOverfillIsAFindingAndNamesTheArithmetic(t *testing.T) {
	c := Contention{Action: "join_event", Room: 1, Accepted: 2, Width: 2}
	if why := c.Vacuous(); why != "" {
		t.Fatalf("a two-request fan-out into room for one was called unjudgeable: %s", why)
	}
	got := c.Finding()
	if got == "" {
		t.Fatal("two acceptances into room for one is not reported as an overfill")
	}
	for _, want := range []string{"join_event", "accepted 2 of 2", "room for 1", "1 more"} {
		if !strings.Contains(got, want) {
			t.Errorf("the finding does not carry %q, so a reader cannot check it against the "+
				"system's own rule: %s", want, got)
		}
	}

	// THE CONTROL IS THE SAME SETUP THE SYSTEM GOT RIGHT, which is what keeps
	// the clause from firing on any contended fan-out.
	ok := Contention{Action: "accept_invitation", Room: 1, Accepted: 1, Width: 2}
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
		{"one request", Contention{Action: "join_event", Room: 1, Accepted: 1, Width: 1},
			"cannot overfill anything"},
		{"no room", Contention{Action: "join_event", Room: 0, Accepted: 0, Width: 2},
			"the setup's answer and not the system's"},
		{"room for everyone", Contention{Action: "join_event", Room: 3, Accepted: 2, Width: 2},
			"never under pressure"},
		{"nothing accepted", Contention{Action: "join_event", Room: 1, Accepted: 0, Width: 2},
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
	asMechanismArm := Contention{Action: "join_event", Room: 3, Accepted: 2, Width: 2}
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
