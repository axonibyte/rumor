package schedule

import (
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
	"forge.axonibyte.com/axonibyte/rumor/internal/ledger"
)

// THE RUN'S CLOCK IS WRITTEN AT THE SEAL, AND WHAT IT WROTE STAYS WRITTEN.
//
// These are the library halves of fable's D1 (coop breakout-r7/0): the drill
// drives a whole clocked run, and an arm here says which of the two ways it
// could fail was the cause. The pointer arm is the one worth having twice
// over: `ledger.Fact` carries `T *int64`, so a seal that handed the ledger the
// address of runClock's own field would give every clock fact in a run ONE
// shared pointer -- invisible today, because the clock cannot move until R7.3
// draws `advance`, and then wrong everywhere at once. A ledger whose past
// changes is the thing the vocabulary forbids outright, so the guard is armed
// before the mover exists rather than after.

func TestASealWritesTheRunsTimeWhenTheProjectDeclaresAClock(t *testing.T) {
	l := ledger.New(true)
	clk := &runClock{control: true}

	for i := 0; i < 3; i++ {
		if err := clk.seal(l); err != nil {
			t.Fatalf("seal %d: %v", i+1, err)
		}
	}

	facts := l.Facts()
	if len(facts) != 3 {
		t.Fatalf("%d fact(s) after three seals, want 3: a clocked run holds its time at every "+
			"seal, so a missing one is a position at which no bound can be measured", len(facts))
	}
	for i, f := range facts {
		if f.Kind != ledger.KindClock {
			t.Errorf("fact %d is a %q, want a clock fact", i+1, f.Kind)
		}
		if f.T == nil {
			t.Fatalf("clock fact %d carries no time", i+1)
		}
		if *f.T != 0 {
			t.Errorf("clock fact %d reads %d, want 0: the run's epoch is zero at the first seal "+
				"and nothing moves it until R7.3 draws `advance`", i+1, *f.T)
		}
	}
	// AND THE SEAL COVERS WHAT WAS WRITTEN, which is the half engine.Now
	// depends on: it reads the newest clock fact at or BEFORE the seal, so a
	// fact appended after the seal moved would be invisible at the position it
	// was written for.
	if l.Seal() != 3 {
		t.Errorf("the seal is at %d after three seals, want 3: the fact goes in before the seal "+
			"moves, or engine.Now cannot see the time it was written for", l.Seal())
	}
	if now, ok := engine.Now(l); !ok || now != 0 {
		t.Errorf("engine.Now reads (%d, %v), want (0, true): the clock the walk wrote is the "+
			"clock the overlay measures a duration bound against", now, ok)
	}
}

func TestAFactAlreadyWrittenDoesNotMoveWhenTheClockDOES(t *testing.T) {
	l := ledger.New(true)
	clk := &runClock{control: true}

	if err := clk.seal(l); err != nil {
		t.Fatalf("the first seal: %v", err)
	}
	// R7.3's MOVER, STOOD IN FOR HERE. `advance` is an action drawn by weight
	// and it does not exist yet, so the arm moves the field the way it will:
	// what is under test is whether the fact written a moment ago kept its own
	// time.
	clk.t = 30_000_000_000
	if err := clk.seal(l); err != nil {
		t.Fatalf("the second seal: %v", err)
	}

	facts := l.Facts()
	if len(facts) != 2 {
		t.Fatalf("%d fact(s), want 2", len(facts))
	}
	if *facts[0].T != 0 {
		t.Errorf("the first clock fact now reads %d, want 0: the ledger's past changed when the "+
			"clock moved, which means the fact carries a POINTER to the run's clock rather than "+
			"a copy of what the time was", *facts[0].T)
	}
	if *facts[1].T != 30_000_000_000 {
		t.Errorf("the second clock fact reads %d, want 30000000000: a seal writes the time as it "+
			"stands, so a clock that moved and a fact that did not is the other failure", *facts[1].T)
	}
}

func TestAProjectWithoutClockControlWritesNoClockFactAndItsPositionsDoNotShift(t *testing.T) {
	l := ledger.New(false)
	clk := &runClock{control: false}

	for i := 0; i < 3; i++ {
		if err := clk.seal(l); err != nil {
			t.Fatalf("seal %d: %v", i+1, err)
		}
	}

	if n := len(l.Facts()); n != 0 {
		t.Fatalf("%d fact(s) in an unclocked run, want 0: a project declaring no `[clock]` is "+
			"walked exactly as before, and a fact appended here would shift every position after "+
			"it -- which is every pre-R7 fixture's ledger", n)
	}
	if _, ok := engine.Now(l); ok {
		t.Error("engine.Now answers in an unclocked run, so a duration bound would be measured " +
			"against a time the project never gave the run")
	}
	// AND THE LEDGER IS WHAT ENFORCES IT, not this type's `if`: the refusal
	// belongs to the vocabulary's "wall-clock time never enters the ledger",
	// so a writer that forgot the check is still refused.
	zero := int64(0)
	if _, err := l.Append(ledger.Fact{Kind: ledger.KindClock, Actor: "run", T: &zero}); err == nil {
		t.Error("the ledger accepted a clock fact with no clock control declared, so the guard " +
			"in runClock.seal is the only thing standing between a project that declared nothing " +
			"and a clock")
	}
}
