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

// TestAnAdvanceRecordsTHEWORLDANDNotTheAsk is the arm ratchet asked for at coop
// r7/53, and it is the first one that can exist.
//
// WHY IT COULD NOT EXIST IN 101. `ask` sends `set` and refuses any answer that
// differs from the instant it named, so in every accepted run the ask and the
// answer are ONE number and `t = got` is observationally equal to `t = c.t`. I
// drove that and said so in the commit: the mutant survives, and it has to.
// `advance` names a DELTA instead, and where the clock lands is the world's to
// say -- a coarse tick, a clamp -- so here the two numbers can differ and the
// ledger's choice becomes falsifiable.
//
// THE ADAPTER HERE ROUNDS UP TO THE NEXT WHOLE SECOND, which is the honest
// behavior of a clock whose tick is a second: asked for 250ms it answers 1s.
// Nothing about that is a fault, and an engine that refused it would be
// refusing the world for not being as fine as the engine's arithmetic.
func TestAnAdvanceRecordsTHEWORLDANDNotTheAsk(t *testing.T) {
	const second = int64(1_000_000_000)
	coarse := &stubClock{answer: func(req wire.ClockRequest) (int64, error) {
		switch req.Op {
		case wire.ClockAdvance:
			// THE WORLD'S OWN ARITHMETIC, and not the engine's: the adapter
			// holds the clock, so it answers where ITS clock now reads.
			return second, nil
		default:
			return req.T, nil
		}
	}}

	l := ledger.New(true)
	clk := &runClock{control: true, form: bindings.ClockAdapter}

	got, err := clk.advance(coarse, 250_000_000)
	if err != nil {
		t.Fatalf("a coarse clock's answer was refused: %v", err)
	}
	if got != second {
		t.Fatalf("advance answers %d, want %d: the adapter said where its clock landed", got, second)
	}
	if clk.t != second {
		t.Errorf("the run's clock holds %d, want %d: anything else leaves the engine believing a "+
			"time the system under test does not hold, and the next seal would `set` it backward",
			clk.t, second)
	}

	// AND THE LEDGER CARRIES THE ANSWER. This is the assertion the whole arm
	// exists for: 250ms was asked for and the fact reads one second.
	if err := clk.seal(l, coarse); err != nil {
		t.Fatalf("sealing after the advance: %v", err)
	}
	facts := l.Facts()
	if len(facts) != 1 {
		t.Fatalf("%d fact(s) after one seal, want 1", len(facts))
	}
	if facts[0].T == nil || *facts[0].T != second {
		t.Fatalf("the clock fact reads %v, want %d: the ledger records the time the WORLD "+
			"answered, not the delta the engine asked for", facts[0].T, second)
	}
	t.Logf("PROOF asked 250000000ns, world answered %d, ledger reads %d", got, *facts[0].T)
}

// TestAnAdvanceTheRunCannotAcceptIsRefusedByName: the two answers an advance
// must not take, and the two it must.
func TestAnAdvanceTheRunCannotAcceptIsRefusedByName(t *testing.T) {
	const held = int64(7_000_000_000)
	for _, tc := range []struct {
		name   string
		delta  int64
		answer func(wire.ClockRequest) (int64, error)
		want   string
	}{
		{"a clock that went backward", 1_000_000_000, func(wire.ClockRequest) (int64, error) {
			return held - 1, nil
		}, "moved the world backward"},
		{"an adapter that does not know the kind", 1_000_000_000, func(wire.ClockRequest) (int64, error) {
			return 0, fmt.Errorf("the adapter answered a could-not-run fault to the clock " +
				"request: unknown kind \"clock\"")
		}, "did not answer"},
		// AND THE ENGINE'S OWN ARITHMETIC IS CHECKED BEFORE THE WIRE: a
		// non-positive delta is this side's fault, so it must not reach the
		// adapter at all.
		{"an advance of zero", 0, echo, "is not an advance"},
		{"an advance of a negative", -5, echo, "is not an advance"},
	} {
		clk := &runClock{control: true, form: bindings.ClockAdapter, t: held}
		st := &stubClock{answer: tc.answer}
		_, err := clk.advance(st, tc.delta)
		if err == nil {
			t.Errorf("%s: accepted", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: refused as %q, which does not name %q", tc.name, err, tc.want)
		}
		if clk.t != held {
			t.Errorf("%s: the run's clock moved to %d on a refused advance", tc.name, clk.t)
		}
		if tc.delta <= 0 && len(st.asks) != 0 {
			t.Errorf("%s: %d request(s) sent for a delta the engine should refuse itself",
				tc.name, len(st.asks))
		}
	}
}

// TestAFakedAdvanceMovesTheRunsOwnClockAndAsksNothing: the other form.
func TestAFakedAdvanceMovesTheRunsOwnClockAndAsksNothing(t *testing.T) {
	clk := &runClock{control: true, form: bindings.ClockFaked, t: 100}
	st := &stubClock{answer: echo}
	got, err := clk.advance(st, 400)
	if err != nil {
		t.Fatalf("a faked advance: %v", err)
	}
	if got != 500 || clk.t != 500 {
		t.Errorf("advance answers %d with the clock at %d, want 500 and 500", got, clk.t)
	}
	if len(st.asks) != 0 {
		t.Errorf("%d request(s) sent under faked time, want 0: the stack is already under a "+
			"clock the engine owns, so there is no second holder to agree with", len(st.asks))
	}
}

// TestLandingsAreTheThreeAndOnlyWhatIsAHEAD drives fable's three landings at
// coop r7/42 and the filter that makes a drawn advance executable.
//
// THE MIDDLE LANDING IS THE HINGE and the reason the set is three: the overlay
// holds a disagreement open while `now-claimTime > bound` is FALSE, so landing
// exactly ON the bound is the one input that separates a strict comparison
// from a loose one. At bound-1ns both readings hold it open; at bound+1ns both
// let it past.
//
// AND THE FILTER IS NOT TIDINESS. `advance` refuses a non-positive delta as
// this side's own arithmetic error, so a landing at or behind the clock is one
// the mover would reject -- and the pool's gate reads this same list, so an
// unfiltered landing would make the gate say "live" where the draw cannot act.
func TestLandingsAreTheThreeAndOnlyWhatIsAHEAD(t *testing.T) {
	for _, tc := range []struct {
		name   string
		untils []int64
		now    int64
		want   []int64
	}{
		{"one bound, well ahead", []int64{100}, 0, []int64{99, 100, 101}},
		{"nothing to land on", nil, 0, nil},
		// THE EDGE THE FILTER EXISTS FOR: a bound ONE TICK ahead offers two
		// usable landings and one -- bound-1ns, which is the clock itself --
		// that would be an advance of zero.
		{"a bound one tick ahead", []int64{1}, 0, []int64{1, 2}},
		// AND A BOUND ALREADY BEHIND THE CLOCK offers nothing: a pending sits
		// in the register until its re-check, so this is reachable rather than
		// hypothetical.
		{"a bound behind the clock", []int64{50}, 100, nil},
		// ONE TICK PAST A BEHIND-BOUND IS STILL BEHIND; only its +1ns can be
		// ahead, and here it is not.
		{"a bound at the clock", []int64{100}, 100, []int64{101}},
		// ADJACENT BOUNDS SHARE LANDINGS: 100+1 and 101 and 102-1 are one
		// instant, and a shared landing is one landing.
		{"adjacent bounds", []int64{100, 101}, 0, []int64{99, 100, 101, 102}},
		{"two bounds apart", []int64{100, 500}, 0, []int64{99, 100, 101, 499, 500, 501}},
	} {
		got := Landings(tc.untils, tc.now)
		if len(got) != len(tc.want) {
			t.Errorf("%s: Landings(%v, %d) is %v, want %v", tc.name, tc.untils, tc.now, got, tc.want)
			continue
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Errorf("%s: Landings(%v, %d) is %v, want %v", tc.name, tc.untils, tc.now, got, tc.want)
				break
			}
		}
	}

	// AND EVERY LANDING IS ONE THE MOVER ACCEPTS, which is the property the
	// filter exists for, asserted by DRIVING the mover rather than by reading
	// the numbers: for each landing, an advance of `landing - now` must not be
	// refused for its own arithmetic.
	const now = int64(1_000)
	for _, l := range Landings([]int64{1_001, 1_500}, now) {
		clk := &runClock{control: true, form: bindings.ClockFaked, t: now}
		if _, err := clk.advance(&stubClock{answer: echo}, l-now); err != nil {
			t.Errorf("the mover refuses a landing this function offered (%d from %d): %v", l, now, err)
		}
	}
	t.Logf("PROOF %v from bounds 1001 and 1500 at a clock of 1000, every one drivable",
		Landings([]int64{1_001, 1_500}, now))
}
