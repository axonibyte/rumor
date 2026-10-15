package engine

import (
	"context"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/ledger"
)

// THE CLOCK IS DRIVEN HERE, which is what docs/vocabulary.md says R2.3 does.
// The ledgers below are BUILT rather than committed, and that is deliberate:
// `pending` needs an `eventual` type with a bound from the bindings artefact,
// which arrives with R4, and v1 says in as many words that no example
// exercises `pending` until then. Building one here drives the clock without
// adding a fourth example package and without moving tests/statute's declared
// 3/17/19, which are R1's gated numbers.

// countStatute is the smallest statute that can disagree: what the observer
// knows of a type, against what it says it saw. Not one of the three committed
// statutes -- those are pinned, and none declares an eventual type.
const countStatute = `package rumor.statute

held_claims contains c if {
	some c in input.claims
	c.ref in input.known
	c.position <= input.observation.position
}

believed := count({c.ref |
	some c in held_claims
	c.type == input.observation.of
})

default finding := "bug"

finding := "consistent" if input.observation.value == believed
`

var countRules = []string{"believed", "finding"}

// timedLedger builds: a clock at t0, one claim by alice, then a clock at t1.
// Sealed over all three, so the run's time at the seal is t1 and the claim's
// time is t0 -- which is how late the observation is.
func timedLedger(t *testing.T, t0, t1 int64) *ledger.Ledger {
	t.Helper()
	l := ledger.New(true) // the run controls the clock: v1 refuses a clock fact otherwise
	must := func(f ledger.Fact) {
		t.Helper()
		if _, err := l.Append(f); err != nil {
			t.Fatalf("building the ledger: %v", err)
		}
	}
	must(ledger.Fact{Kind: ledger.KindClock, Actor: "run", T: &t0})
	must(ledger.Fact{Kind: ledger.KindClaim, Actor: "alice", Type: "item.created", Ref: "alice.item#1"})
	must(ledger.Fact{Kind: ledger.KindClock, Actor: "run", T: &t1})
	if err := l.SealAt(l.Len()); err != nil {
		t.Fatalf("sealing: %v", err)
	}
	return l
}

func TestAClockFactNeedsClockControl(t *testing.T) {
	var at int64 = 7
	l := ledger.New(false)
	_, err := l.Append(ledger.Fact{Kind: ledger.KindClock, Actor: "run", T: &at})
	if err == nil {
		t.Fatal("a clock fact was accepted with no clock control declared (R4, R7)")
	}
	if !strings.Contains(err.Error(), "clock control") {
		t.Fatalf("refused, but not by the clock-control clause: %v", err)
	}
	// The permitted twin: the same fact, with control declared.
	if _, err := ledger.New(true).Append(ledger.Fact{Kind: ledger.KindClock, Actor: "run", T: &at}); err != nil {
		t.Fatalf("the same fact was refused with control declared: %v", err)
	}
}

// Now IS THE LATEST CLOCK AT OR BEFORE THE SEAL, which matters because a clock
// fact past the seal is a time the check must not see.
func TestNowIsTheLatestClockAtOrBeforeTheSeal(t *testing.T) {
	l := timedLedger(t, 100, 105)
	var later int64 = 999
	if _, err := l.Append(ledger.Fact{Kind: ledger.KindClock, Actor: "run", T: &later}); err != nil {
		t.Fatal(err)
	}
	now, ok := Now(l)
	if !ok {
		t.Fatal("no time at the seal, though the prefix holds two clock facts")
	}
	if now != 105 {
		t.Fatalf("the run's time at the seal is %d, wanted 105: the clock past the seal was read", now)
	}
	// Both sides: move the seal and the later clock becomes visible.
	if err := l.SealAt(l.Len()); err != nil {
		t.Fatal(err)
	}
	if now, _ := Now(l); now != 999 {
		t.Fatalf("after sealing over it the time is %d, wanted 999", now)
	}
	// And a ledger with no clock fact reports absence rather than zero: zero is
	// a time.
	plain := ledger.New(false)
	if _, err := plain.Append(ledger.Fact{Kind: ledger.KindClaim, Actor: "alice",
		Type: "item.created", Ref: "alice.item#1"}); err != nil {
		t.Fatal(err)
	}
	if err := plain.SealAt(1); err != nil {
		t.Fatal(err)
	}
	if v, ok := Now(plain); ok {
		t.Errorf("a ledger with no clock fact reported the time %d as present", v)
	}
	t.Logf("PROOF time at the seal 105 with a 999 clock past it; absent, not 0, with no clock fact")
}

// A DISAGREEMENT INSIDE AN EVENTUAL TYPE'S BOUND IS `pending`, AND OUTSIDE IT
// THE STATUTE'S FINDING STANDS. Both halves, because either alone would pass
// for an overlay that always fired or never did.
func TestADisagreementInsideTheBoundIsPendingAndOutsideItIsNot(t *testing.T) {
	ctx := context.Background()
	b := Bindings{Eventual: []Eventual{{Of: "item.created", Bound: 10}}}
	chk := Check{Actor: "alice", Query: "item.count", Of: "item.created", Source: ledger.SourceAPI, Value: 0}

	inside, err := Judge(ctx, countStatute, countRules, timedLedger(t, 100, 105), chk, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	if inside.Value != "pending" {
		t.Fatalf("inside the bound the finding is %v, wanted pending", inside.Value)
	}
	if inside.StatuteValue != "bug" {
		t.Errorf("the statute's own verdict was lost: %v", inside.StatuteValue)
	}
	if inside.Overlay == nil || inside.Overlay.Rule != "engine.pending" {
		t.Fatalf("the finding does not name the rule that produced it: %+v", inside.Overlay)
	}
	if inside.Overlay.Until != 110 {
		t.Errorf("the bound expires at %d, wanted 110", inside.Overlay.Until)
	}
	if len(inside.Fired) != 1 {
		t.Errorf("the statute's own definition was not kept: %v", inside.Fired)
	}

	// PAST THE BOUND the engine does not invent a verdict. v1 reserves `stale`
	// for R6 and defines nothing, so what stands is what the statute said.
	outside, err := Judge(ctx, countStatute, countRules, timedLedger(t, 100, 120), chk, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	if outside.Value != "bug" || outside.Overlay != nil {
		t.Fatalf("past the bound the finding is %v with overlay %+v, wanted the statute's bug and none",
			outside.Value, outside.Overlay)
	}

	// AND AGREEMENT IS NEVER HELD OPEN: a consistent observation has nothing
	// to be late about, however eventual its type.
	agree := chk
	agree.Value = 1
	ok, err := Judge(ctx, countStatute, countRules, timedLedger(t, 100, 105), agree, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ok.Value != "consistent" || ok.Overlay != nil {
		t.Fatalf("an agreeing observation was overlaid: %v %+v", ok.Value, ok.Overlay)
	}

	// AND A TYPE THAT IS NOT DECLARED EVENTUAL IS NOT HELD OPEN EITHER, which
	// is the control on the bindings themselves: the same ledger and the same
	// disagreement, with the declaration removed.
	none, err := Judge(ctx, countStatute, countRules, timedLedger(t, 100, 105), chk, Bindings{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if none.Value != "bug" || none.Overlay != nil {
		t.Fatalf("with no eventual declaration the finding is %v, wanted the statute's bug", none.Value)
	}
	t.Logf("PROOF inside the bound pending until %d over the statute's %v; past it %v; agreeing %v; undeclared %v",
		inside.Overlay.Until, inside.StatuteValue, outside.Value, ok.Value, none.Value)
}

// AN EVENTUAL TYPE WITH NO CLOCK TO MEASURE AGAINST IS REFUSED, not quietly
// read as expired. Reading it as expired would turn a `pending` into a defect
// on missing data, which is the R4 shape: time the run does not control never
// decides a verdict.
func TestAnEventualTypeWithNoClockIsRefused(t *testing.T) {
	ctx := context.Background()
	b := Bindings{Eventual: []Eventual{{Of: "item.created", Bound: 10}}}
	chk := Check{Actor: "alice", Query: "item.count", Of: "item.created", Source: ledger.SourceAPI, Value: 0}

	plain := ledger.New(false)
	if _, err := plain.Append(ledger.Fact{Kind: ledger.KindClaim, Actor: "alice",
		Type: "item.created", Ref: "alice.item#1"}); err != nil {
		t.Fatal(err)
	}
	if err := plain.SealAt(1); err != nil {
		t.Fatal(err)
	}
	_, err := Judge(ctx, countStatute, countRules, plain, chk, b, nil)
	if err == nil {
		t.Fatal("an eventual type was judged with no clock fact, so its bound was guessed")
	}
	if !strings.Contains(err.Error(), "bound cannot be measured") {
		t.Fatalf("refused, but not by the unmeasurable-bound clause: %v", err)
	}
	// BOTH HALVES OF THAT CLAUSE ARE REACHED. With no clock at all, neither is
	// present; with a clock only AFTER the claim, the seal has one and the
	// claim does not -- and the message must say so, since a reader cannot
	// otherwise tell which half was missing.
	if !strings.Contains(err.Error(), "at the seal is present=false") {
		t.Errorf("the clause does not say the seal had no clock: %v", err)
	}
	late := ledger.New(true)
	var t0 int64 = 50
	if _, err := late.Append(ledger.Fact{Kind: ledger.KindClaim, Actor: "alice",
		Type: "item.created", Ref: "alice.item#1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := late.Append(ledger.Fact{Kind: ledger.KindClock, Actor: "run", T: &t0}); err != nil {
		t.Fatal(err)
	}
	if err := late.SealAt(late.Len()); err != nil {
		t.Fatal(err)
	}
	_, err = Judge(ctx, countStatute, countRules, late, chk, b, nil)
	if err == nil {
		t.Fatal("a claim with no clock before it was judged against a bound")
	}
	if !strings.Contains(err.Error(), "at the seal is present=true") ||
		!strings.Contains(err.Error(), "position 1) is present=false") {
		t.Errorf("the clause does not name which half was missing: %v", err)
	}
	// The permitted twin: the same statute, check and bindings, with a clock.
	f, err := Judge(ctx, countStatute, countRules, timedLedger(t, 100, 105), chk, b, nil)
	if err != nil {
		t.Fatalf("the same judgement with a clock was refused: %v", err)
	}
	if f.Value != "pending" {
		t.Fatalf("the twin yielded %v", f.Value)
	}
}

// A PENDING IS RE-CHECKED AT THE ACTOR'S NEXT OBSERVATION OF THE SAME QUERY,
// which is v1's rule word for word, and the key is the pair: another query from
// the same actor does not re-check it.
func TestAPendingIsReCheckedAtTheNextObservationOfTheSameQuery(t *testing.T) {
	ctx := context.Background()
	b := Bindings{Eventual: []Eventual{{Of: "item.created", Bound: 10}}}
	reg := NewRegister()
	l := timedLedger(t, 100, 105)
	chk := Check{Actor: "alice", Query: "item.count", Of: "item.created", Source: ledger.SourceAPI, Value: 0}

	first, err := Judge(ctx, countStatute, countRules, l, chk, b, reg)
	if err != nil {
		t.Fatal(err)
	}
	if first.Value != "pending" || reg.Len() != 1 {
		t.Fatalf("the first observation left %d pending with finding %v", reg.Len(), first.Value)
	}
	if first.ReChecked != nil {
		t.Error("the first observation claims to have re-checked something")
	}

	// A DIFFERENT QUERY DOES NOT RE-CHECK IT. The other query DISAGREES too,
	// so it opens a pending of its own: the register must then hold two, and
	// the re-check below must clear exactly one. A first attempt gave this
	// query an agreeing value, which registered nothing and so could not tell
	// "the key is the pair" from "the key is the actor".
	other := chk
	other.Query = "item.total"
	if _, err := Judge(ctx, countStatute, countRules, l, other, b, reg); err != nil {
		t.Fatal(err)
	}
	if reg.Len() != 2 {
		t.Fatalf("after a second disagreeing query the register holds %d, wanted 2", reg.Len())
	}

	// THE SAME QUERY DOES, and the pending is cleared whatever the new verdict
	// is -- clearing it only on agreement would leave one that had become a
	// defect sitting in the register as though it were merely late.
	again := chk
	again.Value = 1
	second, err := Judge(ctx, countStatute, countRules, l, again, b, reg)
	if err != nil {
		t.Fatal(err)
	}
	if second.ReChecked == nil {
		t.Fatal("the next observation of the same query did not re-check the pending")
	}
	if second.ReChecked.Statute != "bug" || second.ReChecked.Until != 110 {
		t.Errorf("the re-checked pending is not the one recorded: %+v", second.ReChecked)
	}
	if second.Value != "consistent" {
		t.Errorf("the re-check produced %v", second.Value)
	}
	if reg.Len() != 1 {
		t.Fatalf("after the re-check the register holds %d, wanted the 1 the other query opened", reg.Len())
	}
	if qs := reg.Open()["alice"]; len(qs) != 1 || qs[0] != "item.total" {
		t.Errorf("the surviving pending is %v, wanted item.total alone: the re-check cleared the wrong pair", qs)
	}

	// AND A RE-CHECK THAT STILL DISAGREES IS HELD OPEN AGAIN, not dropped.
	reg2 := NewRegister()
	if _, err := Judge(ctx, countStatute, countRules, l, chk, b, reg2); err != nil {
		t.Fatal(err)
	}
	third, err := Judge(ctx, countStatute, countRules, l, chk, b, reg2)
	if err != nil {
		t.Fatal(err)
	}
	if third.ReChecked == nil || third.Value != "pending" || reg2.Len() != 1 {
		t.Fatalf("a still-disagreeing re-check: rechecked %v, finding %v, register %d",
			third.ReChecked != nil, third.Value, reg2.Len())
	}
	t.Logf("PROOF held open, untouched by another query, cleared by the same one, and re-held when it still disagrees")
}
