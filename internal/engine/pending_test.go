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

	// PAST THE BOUND IS `stale`, AND THIS ARM CHANGED AT R6.1 BY CHARTER.
	//
	// It read: "the engine does not invent a verdict. v1 reserves `stale` for
	// R6 and defines nothing, so what stands is what the statute said" -- and
	// asserted `bug` with NO overlay. That was right while the word was
	// reserved. Issue 89 is where R6 spends it, so the engine has now been
	// given the verdict it was declining to invent, and the decline is the
	// thing that would be wrong.
	//
	// IT IS NOT A WEAKENED ARM: it asserts strictly more than before -- the
	// value, the KIND, the rule, and that the statute's own verdict survives
	// underneath. The old assertion's own stated reason is what expired.
	outside, err := Judge(ctx, countStatute, countRules, timedLedger(t, 100, 120), chk, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	if outside.Value != "bug" || outside.Kind != "stale" {
		t.Fatalf("past the bound the finding is %v kind %q, wanted bug of kind stale",
			outside.Value, outside.Kind)
	}
	if outside.Overlay == nil || outside.Overlay.Rule != "engine.stale" {
		t.Fatalf("a stale does not name the rule that produced it: %+v", outside.Overlay)
	}
	if outside.StatuteValue != "bug" {
		t.Errorf("the statute's own verdict was lost under the overlay: %v", outside.StatuteValue)
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

	// QUIET AND STILL DISAGREEING: the system was given its chance to settle
	// and says it has, so the disagreement is past the bound.
	settled := base
	settled.Quiesce = QuiesceQuiet
	stale, err := Judge(ctx, countStatute, countRules, noClock(), settled, b, nil)
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
	for _, want := range []string{`"item.created"`, "quiesce", "position 1"} {
		if !strings.Contains(stale.Overlay.Reason, want) {
			t.Errorf("the reason does not say %q: %s", want, stale.Overlay.Reason)
		}
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

	regStale := NewRegister()
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
		{"a probe that gave up", QuiesceTimedOut, "TIMED OUT"},
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

	// THE CONTROL, so the two refusals above are about the ANSWER and not about
	// quiesce bounds being unjudgeable: the two real answers still reach a
	// verdict on the same fixture.
	for _, tc := range []struct {
		state QuiesceState
		value any
	}{{QuiesceBusy, "pending"}, {QuiesceQuiet, "bug"}} {
		c := base
		c.Quiesce = tc.state
		f, err := Judge(ctx, countStatute, countRules, l(), c, b, nil)
		if err != nil {
			t.Fatalf("%s was refused: %v", tc.state, err)
		}
		if f.Value != tc.value {
			t.Errorf("%s gave %v, wanted %v", tc.state, f.Value, tc.value)
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
