package schedule

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
	"forge.axonibyte.com/axonibyte/rumor/internal/ledger"
	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

// stubClock is an adapter's clock answers, written by the arm.
//
// EVERY CELL BELOW IS AN ANSWER A REAL ADAPTER COULD GIVE and the toy does
// not: it is honest, so driving "a `set` came back changed" against it is
// impossible. The engine's check is what is under test here, and the toy
// drives the same path end to end in tests/clock.
type stubClock struct {
	asks []wire.ClockRequest
	// answer takes the request and gives the `t` to answer with, or an error
	// standing in for the fault an adapter that does not know the kind sends.
	answer func(wire.ClockRequest) (int64, error)
}

func (s *stubClock) Clock(req wire.ClockRequest) (wire.ClockResponse, error) {
	s.asks = append(s.asks, req)
	t, err := s.answer(req)
	if err != nil {
		return wire.ClockResponse{}, err
	}
	return wire.ClockResponse{T: t}, nil
}

// echo is the honest adapter: a `set` leaves the clock reading what was asked.
func echo(req wire.ClockRequest) (int64, error) { return req.T, nil }

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
	clk := &runClock{control: true, form: bindings.ClockFaked}
	st := &stubClock{answer: echo}

	for i := 0; i < 3; i++ {
		if err := clk.seal(l, st); err != nil {
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
	// AND A `faked` RUN ASKED THE ADAPTER NOTHING, which is the whole of that
	// form: the stack is already under faked time, so the engine's own `t` is
	// the world's and a request would be the engine asking a clock it holds.
	if len(st.asks) != 0 {
		t.Errorf("%d clock request(s) sent by a `faked` run, want 0: %v", len(st.asks), st.asks)
	}
}

func TestAFactAlreadyWrittenDoesNotMoveWhenTheClockDOES(t *testing.T) {
	l := ledger.New(true)
	clk := &runClock{control: true, form: bindings.ClockFaked}
	st := &stubClock{answer: echo}

	if err := clk.seal(l, st); err != nil {
		t.Fatalf("the first seal: %v", err)
	}
	// R7.3's MOVER, STOOD IN FOR HERE. `advance` is an action drawn by weight
	// and it does not exist yet, so the arm moves the field the way it will:
	// what is under test is whether the fact written a moment ago kept its own
	// time.
	clk.t = 30_000_000_000
	if err := clk.seal(l, st); err != nil {
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

// THE `adapter` FORM ASKS AT EVERY SEAL AND RECORDS THE ANSWER (fable, coop
// r7/51). The request is a `set` of the clock the run holds, so the system
// under test's own clock and the ledger agree about the time at the position
// the fact is written for.
func TestAnAdapterClockIsSetAtEverySealAndTheLedgerTakesTheAnswer(t *testing.T) {
	l := ledger.New(true)
	clk := &runClock{control: true, form: bindings.ClockAdapter}
	st := &stubClock{answer: echo}

	for i := 0; i < 3; i++ {
		if err := clk.seal(l, st); err != nil {
			t.Fatalf("seal %d: %v", i+1, err)
		}
		// R7.3's MOVER AGAIN, STOOD IN FOR: the field moves the way `advance`
		// will, so the arm sees three DIFFERENT asks rather than three copies
		// of zero, and a producer that sent a constant would be caught.
		clk.t = int64(i+1) * 5_000_000_000
	}

	if len(st.asks) != 3 {
		t.Fatalf("%d clock request(s) for three seals, want 3: %v", len(st.asks), st.asks)
	}
	for i, got := range st.asks {
		if got.Op != wire.ClockSet {
			t.Errorf("request %d is a %q, want %q", i+1, got.Op, wire.ClockSet)
		}
		if want := int64(i) * 5_000_000_000; got.T != want {
			t.Errorf("request %d sets %dns, want %dns: the ask is the clock the run holds",
				i+1, got.T, want)
		}
	}
	facts := l.Facts()
	if len(facts) != 3 {
		t.Fatalf("%d fact(s), want 3", len(facts))
	}
	for i, f := range facts {
		if want := int64(i) * 5_000_000_000; f.T == nil || *f.T != want {
			t.Errorf("clock fact %d reads %v, want %d", i+1, f.T, want)
		}
	}
	// WHAT THIS ARM DOES NOT PROVE, said plainly rather than left for a
	// reader to assume: that the fact's time comes from the RESPONSE and not
	// from the ask. It cannot, here or anywhere today -- the third refusal
	// below makes a `set` whose answer differs a refusal, so no accepted run
	// exists in which the two numbers are different. The source is the
	// ruling's and is written down in docs/contract.md; it becomes observable
	// the day an adapter can declare a granularity, and the arm for it belongs
	// to that day.
}

// THE CLOCKED PROJECT THAT DOES NOT SAY HOW IS REFUSED BEFORE A STEP, and it
// is refused HERE rather than at the parse. internal/bindings carries a
// `[clock]` with no `form` without complaint on purpose: this is the reader
// that knows which request it was about to send, and a parse refusal would
// have made this one unreachable -- which is what R6.2 measured beside the
// probes, where a fixture written to trip an arm tripped the parser first.
func TestAClockedProjectWithNoFormIsRefusedBeforeAnyStep(t *testing.T) {
	_, err := Walk(context.Background(), Project{ClockControl: true}, &wire.Session{}, 1, 1, 1)
	if err == nil {
		t.Fatal("a project declaring `[clock]` with no form was walked")
	}
	if !strings.Contains(err.Error(), "no `form`") {
		t.Errorf("refused as %q, which does not name the missing form", err)
	}
	// THE CONTROL IS THE SAME CALL WITH A FORM: it must get PAST this refusal
	// and fail on something else, or the arm above would also pass against a
	// walk that refuses every project.
	_, err = Walk(context.Background(), Project{ClockControl: true, ClockForm: bindings.ClockFaked},
		&wire.Session{}, 1, 1, 1)
	if err != nil && strings.Contains(err.Error(), "no `form`") {
		t.Errorf("a project declaring `faked` was refused for a missing form: %v", err)
	}
}

// THE THREE REFUSALS, each by its own name. An adapter can answer three ways
// that the run cannot accept, and a single "bad clock answer" would make a
// reader of the output guess which.
func TestAClockAnswerTheRunCannotAcceptIsRefusedByName(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer func(wire.ClockRequest) (int64, error)
		want   string
	}{
		{"an adapter that does not know the kind", func(wire.ClockRequest) (int64, error) {
			// WHAT THE WIRE HANDS BACK for a fault envelope: an ordinary
			// error, the session still usable. This is D10's refusal, and it
			// is reachable only because the contract version did not move.
			return 0, fmt.Errorf("the adapter answered a could-not-run fault to the clock " +
				"request: unknown kind \"clock\"")
		}, "did not answer"},
		{"a clock that runs backward", func(req wire.ClockRequest) (int64, error) {
			return req.T - 1, nil
		}, "never runs backward"},
		{"a `set` that comes back changed", func(req wire.ClockRequest) (int64, error) {
			return req.T + 1, nil
		}, "leaves the clock reading what was asked for"},
	} {
		l := ledger.New(true)
		clk := &runClock{control: true, form: bindings.ClockAdapter, t: 7_000_000_000}
		err := clk.seal(l, &stubClock{answer: tc.answer})
		if err == nil {
			t.Errorf("%s: accepted", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: refused as %q, which does not name %q", tc.name, err, tc.want)
		}
		// AND NOTHING WAS WRITTEN. A refusal that left a clock fact behind
		// would put the engine's ask in the ledger under the name of a time
		// the run refused to accept.
		if n := len(l.Facts()); n != 0 {
			t.Errorf("%s: %d fact(s) written by a refused seal, want 0", tc.name, n)
		}
	}
	// THE BACKWARD CASE IS REACHED BEFORE THE EQUALITY CASE, which is why it
	// is ordered that way in the producer: `req.T - 1` differs from the ask as
	// well, so a producer checking equality first would refuse it under the
	// wrong name and the reason that matters -- replay rests on a ledger whose
	// times never decrease -- would never be printed.
}

func TestAProjectWithoutClockControlWritesNoClockFactAndItsPositionsDoNotShift(t *testing.T) {
	l := ledger.New(false)
	clk := &runClock{control: false, form: bindings.ClockFaked}
	st := &stubClock{answer: echo}

	for i := 0; i < 3; i++ {
		if err := clk.seal(l, st); err != nil {
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
