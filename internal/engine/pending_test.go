package engine

import (
	"context"
	"strconv"
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

// A DISAGREEMENT INSIDE AN EVENTUAL TYPE'S BOUND IS `pending`, AND PAST IT IT
// IS `stale`. Both halves, because either alone would pass for an overlay that
// always fired or never did.
//
// THE NAME MOVED WITH THE BEHAVIOUR AT R6.1. It read
// ...IsPendingAndOutsideItIsNot, which asserted the old decline; past the bound
// is now a finding of its own, so the old name would have described the
// opposite of what the body checks.
func TestADisagreementInsideTheBoundIsPendingAndPastItIsStale(t *testing.T) {
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

	// EXACTLY AT THE BOUND IS STILL INSIDE IT, which is the cell this arm did
	// not have and the one the comparison lives or dies by. ratchet drove the
	// gap at coop r7/56: `now-claimTime > e.Bound` loosened to `>=` SURVIVED
	// the whole tree at 102's first head -- 37 packages, rc 0 -- because INSIDE
	// (105) and PAST (120) both read the same under either operator and no cell
	// stood at 110.
	//
	// AND R7.3 IS WHY IT MATTERS NOW. The walk's middle landing puts the clock
	// exactly on a pending's bound by construction, so this state stopped being
	// a boundary nobody reaches and became one a drawn `advance` reaches on
	// purpose. The behaviour asserted is the one the overlay documents: `>`,
	// held open at exactly the bound. Nothing here argues for it -- it records
	// which reading the product has, so a change of mind has to be a change of
	// this cell.
	at, err := Judge(ctx, countStatute, countRules, timedLedger(t, 100, 110), chk, b, NewRegister())
	if err != nil {
		t.Fatal(err)
	}
	if at.Value != "pending" {
		t.Fatalf("at exactly the bound (now-claim == 10) the finding is %v kind %q, wanted "+
			"pending: the comparison is STRICT, so the bound's own instant is inside it",
			at.Value, at.Kind)
	}
	// AND THE SAME INSTANT AFTER A PENDING WAS HELD IS STILL NOT `stale`, which
	// is the half a loose comparison would turn into the word: held, re-checked
	// at the bound, and held again.
	atReg := NewRegister()
	if _, err := Judge(ctx, countStatute, countRules, timedLedger(t, 100, 105), chk, b, atReg); err != nil {
		t.Fatal(err)
	}
	if atReg.Len() != 1 {
		t.Fatalf("no pending was held before the at-the-bound re-check: %d open", atReg.Len())
	}
	again, err := Judge(ctx, countStatute, countRules, timedLedger(t, 100, 110), chk, b, atReg)
	if err != nil {
		t.Fatal(err)
	}
	if again.Value != "pending" || again.Kind != "" {
		t.Fatalf("a pending re-checked AT its bound is %v kind %q, wanted pending with no kind: "+
			"a loose comparison would compose `stale` here", again.Value, again.Kind)
	}

	// PAST THE BOUND IS `stale` ONLY AFTER A PENDING WAS HELD, and this arm has
	// changed twice -- the reason each time being that the charter moved under
	// it, never that it was inconvenient.
	//
	// IT READ, BEFORE R6.1: "the engine does not invent a verdict. v1 reserves
	// `stale` for R6 and defines nothing, so what stands is what the statute
	// said" -- asserting `bug` with NO overlay. Right while the word was
	// reserved; issue 89 spent it, so the decline became the wrong answer.
	//
	// IT THEN ASSERTED `stale` ON A FIRST LOOK, with `nil` for the register,
	// and fable ruled that a defect at coop r6/87: `stale` is "a `pending` past
	// its bound", and docs/vocabulary.md :118 requires the disagreement to have
	// been "held open". With no register there is no pending to be past, so the
	// arm pinned the word to a disagreement nothing had waited for.
	//
	// SO IT DRIVES THE SEQUENCE NOW: one register across two Judges, the first
	// INSIDE the bound so a pending is held, the second PAST it so the held
	// pending is re-checked. That asserts strictly more than either earlier
	// cut -- the value, the kind, the rule, the statute's surviving verdict AND
	// that the word rests on a re-check.
	reg := NewRegister()
	if _, err := Judge(ctx, countStatute, countRules, timedLedger(t, 100, 105), chk, b, reg); err != nil {
		t.Fatal(err)
	}
	if reg.Len() != 1 {
		t.Fatalf("no pending was held, so there is nothing for a stale to be past: %d open",
			reg.Len())
	}
	outside, err := Judge(ctx, countStatute, countRules, timedLedger(t, 100, 120), chk, b, reg)
	if err != nil {
		t.Fatal(err)
	}
	if outside.Value != "bug" || outside.Kind != "stale" {
		t.Fatalf("past the bound the finding is %v kind %q, wanted bug of kind stale",
			outside.Value, outside.Kind)
	}
	if outside.ReChecked == nil {
		t.Fatalf("the re-check that makes it a stale is not recorded on the finding")
	}
	if outside.Overlay == nil || outside.Overlay.Rule != "engine.stale" {
		t.Fatalf("a stale does not name the rule that produced it: %+v", outside.Overlay)
	}
	if outside.StatuteValue != "bug" {
		t.Errorf("the statute's own verdict was lost under the overlay: %v", outside.StatuteValue)
	}

	// AND THE SAME OBSERVATION ON A FIRST LOOK IS A PLAIN `bug`, which is the
	// other half of the ruling and the half no arm asserted: past the bound
	// with nothing ever held, the type reads as an immediate one would and the
	// statute's own rule stands. Without this, moving the word back into
	// pendingOverlay would pass every assertion above.
	first, err := Judge(ctx, countStatute, countRules, timedLedger(t, 100, 120), chk, b, NewRegister())
	if err != nil {
		t.Fatal(err)
	}
	if first.Value != "bug" || first.Kind != "" {
		t.Fatalf("a first look past the bound is %v kind %q, wanted a plain bug: `stale` is a "+
			"pending past its bound and no pending was ever held", first.Value, first.Kind)
	}
	if first.Overlay != nil {
		t.Errorf("a first look past the bound was overlaid: %+v", first.Overlay)
	}

	// AND IT LOCATES THE LATENESS: a stale a reader cannot place is a bug with
	// a longer name, so the reason names the type, the bound and the position.
	for _, want := range []string{`"item.created"`, "bound of 10", "past the bound", "position 2"} {
		if !strings.Contains(outside.Overlay.Reason, want) {
			t.Errorf("the stale's reason does not say %q: %s", want, outside.Overlay.Reason)
		}
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

// A QUIESCE BOUND IS JUDGED WITH NO CLOCK AT ALL, which is what makes it the
// bound R6 can judge (fable, coop breakout-r6/0 ruling 1). The duration form
// needs the run to control the clock and R7 brings it; this one asks only
// whether the system said it had settled.
//
// THE CONTROL IS THE REFUSAL NOT FIRING. The same ledger -- a claim and no
// clock fact -- refuses outright for a duration-bounded type
// (TestAnEventualTypeWithNoClockIsRefused drives exactly that), so a quiesce
// bound reaching a verdict here is the discriminator between the two forms and
// not an accident of the fixture.
func TestAQuiesceBoundIsJudgedWithoutAClock(t *testing.T) {
	ctx := context.Background()
	b := Bindings{Eventual: []Eventual{{Of: "item.created", Quiesce: true}}}

	noClock := func() *ledger.Ledger {
		l := ledger.New(false)
		if _, err := l.Append(ledger.Fact{Kind: ledger.KindClaim, Actor: "alice",
			Type: "item.created", Ref: "alice.item#1"}); err != nil {
			t.Fatal(err)
		}
		if err := l.SealAt(1); err != nil {
			t.Fatal(err)
		}
		return l
	}
	base := Check{Actor: "alice", Query: "item.count", Of: "item.created",
		Source: ledger.SourceAPI, Value: 0}

	// STILL BUSY: held open, and NOT refused for want of a clock.
	waiting := base
	waiting.Quiesce = QuiesceBusy
	held, err := Judge(ctx, countStatute, countRules, noClock(), waiting, b, nil)
	if err != nil {
		t.Fatalf("a quiesce bound was refused though it needs no clock: %v", err)
	}
	if held.Value != "pending" || held.Kind != "" {
		t.Fatalf("before quiet the finding is %v kind %q, wanted pending with no kind",
			held.Value, held.Kind)
	}
	if held.StatuteValue != "bug" {
		t.Errorf("the statute's own verdict was lost: %v", held.StatuteValue)
	}

	// QUIET AFTER A PENDING WAS HELD IS `stale`, and the two Judges share one
	// register on purpose. This arm passed `nil` and asserted `stale` from a
	// single quiet call, which fable ruled a defect at coop r6/87: with no
	// register there is no pending for the word to be past, so it pinned
	// `stale` to a disagreement nothing had ever waited for. The busy call
	// above is what makes the quiet one a RE-CHECK.
	reg := NewRegister()
	if _, err := Judge(ctx, countStatute, countRules, noClock(), waiting, b, reg); err != nil {
		t.Fatalf("the busy check was refused: %v", err)
	}
	if reg.Len() != 1 {
		t.Fatalf("the busy check held no pending, so the quiet one is not a re-check: %d open",
			reg.Len())
	}
	settled := base
	settled.Quiesce = QuiesceQuiet
	stale, err := Judge(ctx, countStatute, countRules, noClock(), settled, b, reg)
	if err != nil {
		t.Fatalf("a quiesced check was refused: %v", err)
	}
	if stale.Value != "bug" || stale.Kind != "stale" {
		t.Fatalf("after quiet the finding is %v kind %q, wanted bug of kind stale",
			stale.Value, stale.Kind)
	}
	if stale.Overlay == nil || stale.Overlay.Rule != "engine.stale" {
		t.Fatalf("the stale does not name the rule that produced it: %+v", stale.Overlay)
	}
	for _, want := range []string{`"item.created"`, "quiesce", "position 1", "held open"} {
		if !strings.Contains(stale.Overlay.Reason, want) {
			t.Errorf("the reason does not say %q: %s", want, stale.Overlay.Reason)
		}
	}

	// AND QUIET ON A FIRST LOOK IS A PLAIN `bug` WITH THE STATUTE'S RULE: no
	// lag was ever excused, so the type reads as an immediate one would. This
	// is the assertion that fails if the word moves back into pendingOverlay.
	firstLook, err := Judge(ctx, countStatute, countRules, noClock(), settled, b, NewRegister())
	if err != nil {
		t.Fatalf("a quiesced first look was refused: %v", err)
	}
	if firstLook.Value != "bug" || firstLook.Kind != "" || firstLook.Overlay != nil {
		t.Fatalf("a first look at a quiet system is %v kind %q overlay %+v, wanted a plain bug",
			firstLook.Value, firstLook.Kind, firstLook.Overlay)
	}

	t.Logf("PROOF quiesce bound, no clock fact: not quiet -> %v; quiet -> %v kind %q",
		held.Value, stale.Value, stale.Kind)
}

// A `stale` IS TERMINAL AND IS NOT HELD OPEN FOR RE-CHECK, where a `pending`
// is. R2.3's rule is that no pending is left open at the end of a run; a stale
// registered as one would make that false of any run that ended on a stale, and
// would have the register claiming a decided finding is still waiting.
func TestAStaleIsNotHeldOpenForReCheck(t *testing.T) {
	ctx := context.Background()
	b := Bindings{Eventual: []Eventual{{Of: "item.created", Quiesce: true}}}
	l := func() *ledger.Ledger {
		x := ledger.New(false)
		if _, err := x.Append(ledger.Fact{Kind: ledger.KindClaim, Actor: "alice",
			Type: "item.created", Ref: "alice.item#1"}); err != nil {
			t.Fatal(err)
		}
		if err := x.SealAt(1); err != nil {
			t.Fatal(err)
		}
		return x
	}
	base := Check{Actor: "alice", Query: "item.count", Of: "item.created",
		Source: ledger.SourceAPI, Value: 0}

	// THE CONTROL FIRST: a pending IS registered, so a register that recorded
	// nothing at all would fail here rather than pass the assertion below.
	regPending := NewRegister()
	waiting := base
	waiting.Quiesce = QuiesceBusy
	if _, err := Judge(ctx, countStatute, countRules, l(), waiting, b, regPending); err != nil {
		t.Fatal(err)
	}
	if regPending.Len() != 1 {
		t.Fatalf("a pending was not registered for re-check: %d open", regPending.Len())
	}

	// AND THE STALE IS THE RE-CHECK OF THAT SAME PENDING, which is what this
	// arm had wrong: it built a FRESH register for the quiet call, so no pending
	// existed for the query and `stale` on first sight was the expectation by
	// construction (ratchet, coop r6/85). The word now requires the re-check,
	// so the sequence IS the fixture -- and regPending is reused rather than
	// replaced, which also makes the emptiness asserted below mean something:
	// it held one a moment ago.
	regStale := regPending
	settled := base
	settled.Quiesce = QuiesceQuiet
	f, err := Judge(ctx, countStatute, countRules, l(), settled, b, regStale)
	if err != nil {
		t.Fatal(err)
	}
	if f.Kind != "stale" {
		t.Fatalf("the fixture did not produce a stale: %v kind %q", f.Value, f.Kind)
	}
	if regStale.Len() != 0 {
		t.Errorf("a stale was held open for re-check: %d open, %v", regStale.Len(), regStale.Open())
	}
	t.Logf("PROOF pending registered 1, stale registered %d", regStale.Len())
}

// THE TWO STATES THAT ARE NOT ANSWERS REFUSE, and that is fable's ruling 1(b)
// at coop r6/10 on ratchet's finding at r6/9: a bool could not tell a probe
// that ran and GAVE UP from a system still BUSY, so both handed in the same
// byte and only one of them is safe to read as "not quiet".
//
// ABSENT IS THE DANGEROUS ONE AND IT IS THE ZERO VALUE. ratchet drove the
// consequence: a quiesce bound with nothing supplying an answer held a real
// disagreement open for fifty passes, the register kept one, StillOpen saw
// nothing because the query was re-asked, the walk never ended -- exit 0 on a
// disagreement. A refusal is the only answer that cannot become that.
func TestAQuiesceAnswerThatIsNotAnAnswerRefuses(t *testing.T) {
	ctx := context.Background()
	b := Bindings{Eventual: []Eventual{{Of: "item.created", Quiesce: true}}}
	l := func() *ledger.Ledger {
		x := ledger.New(false)
		if _, err := x.Append(ledger.Fact{Kind: ledger.KindClaim, Actor: "alice",
			Type: "item.created", Ref: "alice.item#1"}); err != nil {
			t.Fatal(err)
		}
		if err := x.SealAt(1); err != nil {
			t.Fatal(err)
		}
		return x
	}
	base := Check{Actor: "alice", Query: "item.count", Of: "item.created",
		Source: ledger.SourceAPI, Value: 0}

	for _, tc := range []struct {
		name  string
		state QuiesceState
		says  string
	}{
		{"absent, the zero value", QuiesceAbsent, "no quiesce answer"},
		// TIMED OUT LEFT THIS TABLE AT R6.2 AND DID NOT LEAVE THE FILE: it is
		// a verdict now -- the `quiesce_timeout` finding -- and it is asserted
		// as one in the control below and in
		// TestAProbeThatGaveUpIsTheQuiesceTimeoutFinding. It refused here only
		// because the word did not exist, which the clause said at the time.
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := base
			c.Quiesce = tc.state
			_, err := Judge(ctx, countStatute, countRules, l(), c, b, nil)
			if err == nil {
				t.Fatalf("%s was judged as a verdict on the claim, so a non-answer became "+
					"either inside or past the bound", tc.state)
			}
			if !strings.Contains(err.Error(), tc.says) {
				t.Errorf("refused, but not by the clause for %s: %v", tc.state, err)
			}
			// AND IT MUST NOT REFUSE AS THE CLOCK CLAUSE DOES: a quiesce bound
			// needs no clock, so borrowing that message would send a reader to
			// wire a clock that cannot help.
			if strings.Contains(err.Error(), "bound cannot be measured") {
				t.Errorf("a quiesce non-answer refused with the duration clause's words: %v", err)
			}
		})
	}

	// THE CONTROL, so the refusal above is about the ANSWER and not about
	// quiesce bounds being unjudgeable: every real answer still reaches a
	// verdict on the same fixture.
	// THE KIND IS IN THE TABLE, not only the value, because with a nil register
	// every row's value is reachable without the overlay at all: the statute's
	// own finding for this fixture is `bug`, so a `bug` row proves nothing on
	// its own. QUIET's row is the discriminator -- no pending was held, so it
	// must be a PLAIN bug, and the word appearing here would be the defect
	// fable ruled out at coop r6/87.
	for _, tc := range []struct {
		state QuiesceState
		value any
		kind  string
	}{{QuiesceBusy, "pending", ""}, {QuiesceQuiet, "bug", ""},
		{QuiesceTimedOut, FindingQuiesceTimedOut, ""}} {
		c := base
		c.Quiesce = tc.state
		f, err := Judge(ctx, countStatute, countRules, l(), c, b, nil)
		if err != nil {
			t.Fatalf("%s was refused: %v", tc.state, err)
		}
		if f.Value != tc.value || f.Kind != tc.kind {
			t.Errorf("%s gave %v kind %q, wanted %v kind %q", tc.state, f.Value, f.Kind,
				tc.value, tc.kind)
		}
	}
}

// Says RENDERS THE KIND, which is what makes a finding's identity readable
// wherever it is printed rather than only where run.go composes a line.
//
// fable ruled the renderers follow the comparator's basis (coop r6/15): rule,
// value, attribution AND kind. A renderer showing the value alone makes a
// `stale` and a plain `bug` identical in the shrink trace, the examples line
// and the PROMOTED COMMENT -- the one artifact of a shrink that gets checked
// in, and the one ratchet's write-once point is about.
//
// ALL THREE SHAPES, because the first alone would pass for a renderer that
// always appended something and the second for one that never did.
func TestSaysRendersTheKindAndTheAbsenceOfOne(t *testing.T) {
	for _, tc := range []struct {
		name string
		f    Finding
		want string
	}{
		{"a kind", Finding{Defined: true, Value: "bug", Kind: "stale"}, "bug of kind stale"},
		{"no kind", Finding{Defined: true, Value: "bug"}, "bug"},
		{"undefined", Finding{}, "no finding"},
	} {
		if got := tc.f.Says(); got != tc.want {
			t.Errorf("%s: Says() = %q, want %q", tc.name, got, tc.want)
		}
	}
	// AND THE WORDING IS THE FOUND LINE'S, so moving it here changed no output:
	// run.go printed " of kind stale" after the value before this and prints
	// Says() now.
	if s := (Finding{Defined: true, Value: "bug", Kind: "stale"}).Says(); !strings.HasSuffix(s, " of kind stale") {
		t.Errorf("Says() no longer carries the FOUND line's own wording: %q", s)
	}
}

// A STATE THIS BUILD DOES NOT KNOW IS NAMED BY ITS NUMBER, in the refusal and
// in the rendering, which ratchet put as both or neither (coop r6/19 and
// rumor/970, on rook's rumor/969).
//
// THE TWO LAYERS AGREED ON ONE LIE. Judge's switch ended in `default:`, so a
// fifth value was refused by the clause written for an ABSENT answer; and
// String's fallback returned "absent", so the parenthesis that clause carries
// to name the real state named the wrong one too. A reader would have been told
// twice that nothing answered, when something did.
//
// THIS ENUM IS THE ONE MOST LIKELY TO GROW -- R6.2 owns the probes and the
// timed-out finding kind is theirs -- and Go checks no switch for
// exhaustiveness, so nothing but an arm holds this.
//
// THE CONTROL IS THE FOUR KNOWN STATES, because a String that printed the
// number for everything would satisfy the first half and lose every name.
func TestAQuiesceStateThisBuildDoesNotKnowIsNamedByItsNumber(t *testing.T) {
	ctx := context.Background()
	b := Bindings{Eventual: []Eventual{{Of: "item.created", Quiesce: true}}}
	l := func() *ledger.Ledger {
		x := ledger.New(false)
		if _, err := x.Append(ledger.Fact{Kind: ledger.KindClaim, Actor: "alice",
			Type: "item.created", Ref: "alice.item#1"}); err != nil {
			t.Fatal(err)
		}
		if err := x.SealAt(1); err != nil {
			t.Fatal(err)
		}
		return x
	}
	base := Check{Actor: "alice", Query: "item.count", Of: "item.created",
		Source: ledger.SourceAPI, Value: 0}

	// One past the last named state, and one far past it: the first is what a
	// sibling actually adding a state produces, the second rules out a fallback
	// that happens to be right for 4.
	for _, q := range []QuiesceState{QuiesceTimedOut + 1, QuiesceState(99)} {
		name := q.String()
		if !strings.Contains(name, strconv.Itoa(int(q))) {
			t.Errorf("QuiesceState(%d).String() is %q and does not carry the number, so the "+
				"refusal below cannot name the state it refused", int(q), name)
		}
		if strings.Contains(name, "absent") {
			t.Errorf("QuiesceState(%d).String() is %q: an answer that was supplied renders as "+
				"the one that was not", int(q), name)
		}

		c := base
		c.Quiesce = q
		_, err := Judge(ctx, countStatute, countRules, l(), c, b, nil)
		if err == nil {
			t.Fatalf("QuiesceState(%d) reached a verdict on the claim", int(q))
		}
		if !strings.Contains(err.Error(), "does not know") ||
			!strings.Contains(err.Error(), strconv.Itoa(int(q))) {
			t.Errorf("QuiesceState(%d) refused, but not by the clause for an unknown state "+
				"and not naming it: %v", int(q), err)
		}
		if strings.Contains(err.Error(), "no quiesce answer") {
			t.Errorf("QuiesceState(%d) refused as though nothing answered: %v", int(q), err)
		}
	}

	for _, tc := range []struct {
		state QuiesceState
		name  string
	}{
		{QuiesceAbsent, "absent"},
		{QuiesceQuiet, "quiet"},
		{QuiesceBusy, "busy"},
		{QuiesceTimedOut, "timed out"},
	} {
		if got := tc.state.String(); got != tc.name {
			t.Errorf("QuiesceState(%d).String() is %q, want %q: the fallback took a state "+
				"this build does know", int(tc.state), got, tc.name)
		}
	}
}

// TestAProbeThatGaveUpIsTheQuiesceTimeoutFinding drives the fifth finding whole:
// the value, the absence of a kind, the rule, and what the reason says.
//
// IT IS NOT ENOUGH TO ASSERT THE VALUE. The question this phase turned on was
// whether a timeout is a `bug` with a kind or a finding of its own, and only
// one of those is wrong in a way a reader notices: a `bug` with kind
// `quiesce_timeout` would satisfy a value assertion AND reach every reader
// keying on `bug`, which is exactly what fable's D7 forbids -- "neither `bug`
// nor a refusal". So Kind is asserted EMPTY, which is the assertion that tells
// the two shapes apart.
//
// AND THE REASON MUST NOT BORROW `stale`'s WORDS. A quiesced disagreement is
// past its bound; this one is neither inside nor past, because the bound was
// never reached. A reason saying "the bound is past" would send a reader
// looking for a late system when what they have is one that never settled.
func TestAProbeThatGaveUpIsTheQuiesceTimeoutFinding(t *testing.T) {
	ctx := context.Background()
	b := Bindings{Eventual: []Eventual{{Of: "item.created", Quiesce: true}}}
	l := ledger.New(false)
	if _, err := l.Append(ledger.Fact{Kind: ledger.KindClaim, Actor: "alice",
		Type: "item.created", Ref: "alice.item#1"}); err != nil {
		t.Fatal(err)
	}
	if err := l.SealAt(1); err != nil {
		t.Fatal(err)
	}
	c := Check{Actor: "alice", Query: "item.count", Of: "item.created",
		Source: ledger.SourceAPI, Value: 0, Quiesce: QuiesceTimedOut}

	f, err := Judge(ctx, countStatute, countRules, l, c, b, nil)
	if err != nil {
		t.Fatalf("a probe that gave up was refused rather than found: %v", err)
	}
	if f.Value != FindingQuiesceTimedOut {
		t.Errorf("the finding is %v, want %q", f.Value, FindingQuiesceTimedOut)
	}
	if f.Kind != "" {
		t.Errorf("the finding carries kind %q: a finding of its own needs no kind, and one "+
			"here would reach every reader keying on `bug`", f.Kind)
	}
	if f.Value == "bug" {
		t.Error("a quiesce timeout read as a bug: the project and its adapter are both right")
	}
	if f.Overlay == nil {
		t.Fatal("no overlay recorded: the statute's own verdict must stay visible beneath it")
	}
	if f.Overlay.Rule != "engine.quiesce_timeout" {
		t.Errorf("the overlay rule is %q, want engine.quiesce_timeout", f.Overlay.Rule)
	}
	if f.StatuteValue != "bug" {
		t.Errorf("the statute's own verdict is %v, want bug: the overlay must not erase it",
			f.StatuteValue)
	}
	for _, want := range []string{"gave up", "neither inside the bound nor past it", "position 1"} {
		if !strings.Contains(f.Overlay.Reason, want) {
			t.Errorf("the reason does not say %q: %s", want, f.Overlay.Reason)
		}
	}
	if strings.Contains(f.Overlay.Reason, "the bound is past") {
		t.Errorf("the reason borrows `stale`'s words: %s", f.Overlay.Reason)
	}
	t.Logf("PROOF %s", f.Says())
}

// TestUntilsIsSortedDeduplicatedAndNotAMapsOrder drives R7.3's one exported
// accessor (fable, coop r7/42): the instants the live pendings' bounds expire
// at, which is what the walk lands `advance` on.
//
// THE DETERMINISM CELL IS THE POINT OF THE ARM. `open` is a Go map and its
// iteration order is deliberately randomized per range, so an accessor that
// returned it raw would hand the walk a different target order for the same
// seed on the same tree -- and every reproducibility claim in this repository
// rests on that not happening. One call cannot see it: the cell calls Untils
// REPEATEDLY on a register big enough that an unsorted order would differ
// between calls with overwhelming probability, and compares each answer with
// the first.
//
// THE REGISTER IS BUILT DIRECTLY and not through Judge, which is this
// package's own privilege as an internal test: what is under test is the
// accessor's contract over a map, not how a pending came to be in it.
func TestUntilsIsSortedDeduplicatedAndNotAMapsOrder(t *testing.T) {
	// EMPTY FIRST, so "sorted" is not read off a one-element answer.
	if got := NewRegister().Untils(); len(got) != 0 {
		t.Errorf("an empty register answers %v, want nothing", got)
	}

	reg := NewRegister()
	held := []struct {
		actor, query string
		until        int64
	}{
		{"alice", "count", 300},
		{"bob", "count", 100},
		{"carol", "count", 100}, // THE DUPLICATE: one instant, two pendings
		{"dave", "count", 200},
		{"erin", "tally", 250},
		// A QUIESCE-BOUNDED PENDING, which carries no instant: its `Until` is
		// the zero value because `pendingOverlay`'s QuiesceBusy arm sets none,
		// and the field's own declaration says "0 for a quiesce bound". It is
		// NOT a target -- the quiesce form asks whether the system settled,
		// not when a bound expires -- and a walk reading it as one would land
		// `advance` on the run's epoch: a tick before zero is a clock moving
		// backward, which the ledger refuses.
		{"frank", "quiet", 0},
	}
	for _, h := range held {
		reg.open[registerKey{actor: h.actor, query: h.query}] = Pending{Until: h.until}
	}

	want := []int64{100, 200, 250, 300}
	got := reg.Untils()
	if len(got) != len(want) {
		t.Fatalf("Untils answers %v, want %v: SIX pendings over four instants -- two bounds "+
			"expiring together are ONE landing, and a quiesce bound is no landing at all",
			got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Untils answers %v, want %v", got, want)
		}
	}

	// AND THE SAME ANSWER EVERY TIME. Twenty calls over six keys: a raw map
	// range would have to agree with the first answer twenty times in a row.
	for i := 0; i < 20; i++ {
		again := reg.Untils()
		for j := range got {
			if again[j] != got[j] {
				t.Fatalf("call %d answers %v where the first answered %v: a map's order is not "+
					"a run's, and a walk drawing a target from this could not replay",
					i+2, again, got)
			}
		}
	}
	t.Logf("PROOF %v from six pendings (one quiesce-bounded, one duplicate instant), "+
		"identical over twenty-one calls", got)
}
