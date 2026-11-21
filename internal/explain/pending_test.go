package explain

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
	"forge.axonibyte.com/axonibyte/rumor/internal/ledger"
)

// --- d4: pending findings ------------------------------------------------
//
// EVERY CELL HERE DRIVES engine.Judge OVER COMMITTED DATA. `rumor check`'s own
// walk calls Evaluate and declares no bindings, so no case in examples/ is
// pending as the walk reports it -- but a pending is reachable over the same
// ledgers by declaring the eventual type the check already counts, which is
// what a run with bindings does. So these cells need no fixture of their own,
// and the lifecycle below is the toy's own 1ns boundary put to a second use:
// `invitation.sent` is claimed with the clock at 0, so a bound of 30000000000
// expires exactly at invitation-honored-at-the-expiry's clock and sits 1ns
// behind invitation-honored-late's.

// THE BOUND IS LARGER THAN THE CASE'S CLOCK ON PURPOSE. invitation.sent is
// claimed with the clock at 0, so Until is the bound itself, and
// invitation-honored-late's clock reads 30000000001 -- the only committed toy
// case that DISAGREES. A bound of 30000000000 would already be past at that
// instant, which is a bug and not a pending (measured: past the bound with
// nothing held open carries no overlay at all).
const invitationBound = 1_000_000_000_000

func judged(t *testing.T, name string, b engine.Bindings, q engine.QuiesceState, reg *engine.Register) (loaded, engine.Finding) {
	t.Helper()
	lc := load(t, "toy", name)
	c := lc.check
	c.Quiesce = q
	f, err := engine.Judge(context.Background(), lc.statute, lc.rules, lc.ledger, c, b, reg)
	if err != nil {
		t.Fatalf("judging toy/%s: %v", name, err)
	}
	return lc, f
}

func renderJudged(t *testing.T, lc loaded, f engine.Finding) string {
	t.Helper()
	d, err := Build(f, lc.ledger, lc.check, lc.run())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	var b bytes.Buffer
	if err := Render(&b, d); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return b.String()
}

func durationBinding(of string, bound int64) engine.Bindings {
	return engine.Bindings{Eventual: []engine.Eventual{{Of: of, Bound: bound}}}
}

func quiesceBinding(of string) engine.Bindings {
	return engine.Bindings{Eventual: []engine.Eventual{{Of: of, Quiesce: true}}}
}

// TestADurationBoundIsExplainedWithItsClock is the first of the bound kinds:
// the statute's own declared duration, judgeable because the run has a clock.
func TestADurationBoundIsExplainedWithItsClock(t *testing.T) {
	lc, f := judged(t, "invitation-honored-late",
		durationBinding("invitation.sent", invitationBound), engine.QuiesceAbsent, engine.NewRegister())
	if f.Value != "pending" {
		t.Fatalf("the finding is %v, and this cell is about a pending", f.Value)
	}
	text := renderJudged(t, lc, f)
	for _, want := range []string{
		"FINDING    pending",
		"the statute said bug; engine.pending overrode it until 1000000000000",
		"THE BOUND  a declared duration, expiring at 1000000000000",
		"the run's clock made it judgeable: now is 30000000001",
		"969999999999ns remain",
		"settled by bob asking invitation_expiry again, or by the next full reconciliation",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the render omits %q:\n%s", want, text)
		}
	}
}

// TestAQuiesceBoundNamesTheProberAndNoClock is the second: the stage's prober
// decides, so no clock time applies and none is invented.
func TestAQuiesceBoundNamesTheProberAndNoClock(t *testing.T) {
	lc, f := judged(t, "invitation-honored-late",
		quiesceBinding("invitation.sent"), engine.QuiesceBusy, engine.NewRegister())
	if f.Value != "pending" {
		t.Fatalf("the finding is %v, and a busy system holds a disagreement open", f.Value)
	}
	text := renderJudged(t, lc, f)
	if !strings.Contains(text, "THE BOUND  quiesce -- the stage's prober decides, so no clock time applies") {
		t.Errorf("the quiesce bound is not named:\n%s", text)
	}
	// AND THE VERDICT LINE CLAIMS NO INSTANT. A mutation restoring
	// "until the system settles" for every Until of 0 survived this suite
	// until this assertion existed: every cell checked a PREFIX that stays
	// true when the clause is appended, which is a check that cannot fail
	// the other way.
	if strings.Contains(text, "until the system settles") {
		t.Errorf("the verdict line names a bound instant for a quiesce bound:\n%s", text)
	}
	// NO CLOCK TIME IS INVENTED for a bound that has none, which is the half
	// a renderer sharing one code path with the duration form would get
	// wrong: Until is 0 there, and "expiring at 0" would be a lie.
	for _, never := range []string{"expiring at", "ns remain", "past it"} {
		if strings.Contains(text, never) {
			t.Errorf("the quiesce bound render says %q:\n%s", never, text)
		}
	}
	if !strings.Contains(text, "settled by bob asking invitation_expiry again") {
		t.Errorf("what would settle it is absent:\n%s", text)
	}
}

// TestTheProbeGivingUpIsItsOwnFinding is the third overlay: a prober ran and
// gave up, which v1 reserves a finding for rather than calling it a bug.
func TestTheProbeGivingUpIsItsOwnFinding(t *testing.T) {
	lc, f := judged(t, "invitation-honored-late",
		quiesceBinding("invitation.sent"), engine.QuiesceTimedOut, engine.NewRegister())
	if f.Value != engine.FindingQuiesceTimedOut {
		t.Fatalf("the finding is %v, wanted %v", f.Value, engine.FindingQuiesceTimedOut)
	}
	text := renderJudged(t, lc, f)
	for _, want := range []string{
		"FINDING    quiesce_timeout",
		"engine.quiesce_timeout overrode it",
		"THE BOUND  quiesce -- the stage's prober decides",
		"nothing is waiting on this bound: the run has its answer",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the render omits %q:\n%s", want, text)
		}
	}
	// A PROBE THAT GAVE UP IS NOT WAITING FOR THE SYSTEM TO SETTLE, which is
	// what the verdict line said about it until d4 rendered one: the quiesce
	// pending and the quiesce timeout share Until 0, so a clause keyed on
	// that field alone cannot tell them apart and must say nothing.
	if strings.Contains(text, "until the system settles") {
		t.Errorf("the render tells a reader to wait for a probe that gave up:\n%s", text)
	}
}

// TestAPendingPastItsBoundIsExplainedAsStale drives the LIFECYCLE, which is
// the only way engine.stale is reachable: a pending must be HELD and then
// re-checked past its bound.
//
// THIS ONE CELL BUILDS ITS OWN LEDGER, and the reason is measured rather than
// assumed. The lifecycle needs one (actor, query) to DISAGREE at two different
// clocks -- open inside the bound, re-check past it -- and no committed case
// provides that: of the toy's four invitation cases only
// invitation-honored-late disagrees at all, and re-sealing its ledger earlier
// makes the finding `consistent` (driven: seals 6 and 7 answer consistent,
// seal 8 answers bug). So the pair is constructed here, in the plainest form
// that states it: a statute that always finds a bug, and a ledger whose clock
// moves past a bound between two seals. internal/engine builds its own the
// same way.
func TestAPendingPastItsBoundIsExplainedAsStale(t *testing.T) {
	const statute = `package rumor.statute

default finding := "bug"
`
	const facts = `{"kind":"clock","position":1,"actor":"run","t":100}
{"kind":"claim","position":2,"actor":"alice","type":"item.created","ref":"alice.item#1","handles":["alice.item#1"]}
{"kind":"clock","position":3,"actor":"run","t":200}
`
	rules := []string{"finding"}
	b := engine.Bindings{Eventual: []engine.Eventual{{Of: "item.created", Bound: 50}}}
	chk := engine.Check{Actor: "alice", Query: "item.count", Of: "item.created", Source: ledger.SourceAPI, Value: 0}
	reg := engine.NewRegister()

	// Inside: the clock at the seal is 100 and the bound expires at 150.
	inside := twoClockLedger(t, facts, 2)
	first, err := engine.Judge(context.Background(), statute, rules, inside, chk, b, reg)
	if err != nil {
		t.Fatal(err)
	}
	if first.Value != "pending" || first.Overlay == nil || first.Overlay.Until != 150 {
		t.Fatalf("the first judgement did not hold it open at 150: %v %+v", first.Value, first.Overlay)
	}
	if reg.Len() != 1 {
		t.Fatalf("the register holds %d pending(s) after one was opened", reg.Len())
	}

	// Past it: the clock has moved to 200.
	past := twoClockLedger(t, facts, 3)
	second, err := engine.Judge(context.Background(), statute, rules, past, chk, b, reg)
	if err != nil {
		t.Fatal(err)
	}
	if second.Overlay == nil || second.Overlay.Rule != "engine.stale" {
		t.Fatalf("the re-check did not go stale: %+v", second.Overlay)
	}
	if second.Kind != "stale" {
		t.Errorf("the finding's kind is %q, and v1 reserves `stale` for this", second.Kind)
	}

	d, err := Build(second, past, chk, Run{Package: "constructed", Case: "stale", StatuteSHA: "x"})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := Render(&buf, d); err != nil {
		t.Fatal(err)
	}
	text := buf.String()
	for _, want := range []string{
		"FINDING    bug of kind stale",
		"engine.stale overrode it",
		"THE BOUND  a declared duration, expiring at 150",
		"the run's clock made it judgeable: now is 200",
		"50ns past it",
		"nothing is waiting on this bound: the run has its answer",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the render omits %q:\n%s", want, text)
		}
	}
	// AND THE REGISTER NO LONGER HOLDS IT: a finding that has been answered
	// is not still open, which is the other half of "a settled finding is not
	// explained as pending".
	if reg.Len() != 0 {
		t.Errorf("the register still holds %d pending(s) after the re-check", reg.Len())
	}
}

// twoClockLedger loads the constructed facts and seals them at `seal`.
func twoClockLedger(t *testing.T, facts string, seal int) *ledger.Ledger {
	t.Helper()
	l, err := ledger.Load(strings.NewReader(facts), true)
	if err != nil {
		t.Fatalf("the constructed ledger does not load: %v", err)
	}
	if err := l.SealAt(seal); err != nil {
		t.Fatalf("sealing at %d: %v", seal, err)
	}
	return l
}

// TestASettledFindingIsNotExplainedAsPending is d4's second named arm, in both
// directions: a finding no engine rule touched gets no bound block at all, and
// the same case with a binding does.
func TestASettledFindingIsNotExplainedAsPending(t *testing.T) {
	// Settled: no eventual declared, so the statute's own verdict stands.
	lc, f := judged(t, "invitation-honored-late", engine.Bindings{}, engine.QuiesceAbsent, engine.NewRegister())
	if f.Overlay != nil {
		t.Fatalf("an unbounded judgement carries an overlay: %+v", f.Overlay)
	}
	text := renderJudged(t, lc, f)
	for _, never := range []string{"THE BOUND", "pending", "settled by", "engine."} {
		if strings.Contains(text, never) {
			t.Errorf("a settled finding's render says %q:\n%s", never, text)
		}
	}

	// THE CONTROL: the same case, same ledger, with a bound declared, DOES
	// carry the block -- so the absence above is the overlay's and not the
	// render's indifference.
	lc2, f2 := judged(t, "invitation-honored-late",
		durationBinding("invitation.sent", invitationBound), engine.QuiesceAbsent, engine.NewRegister())
	if !strings.Contains(renderJudged(t, lc2, f2), "THE BOUND") {
		t.Error("the same case with a bound declared carries no bound block, so the arm above proves nothing")
	}
}

// TestTheBoundIsInTheJSONToo: d3's form carries the same facts, since d4 adds
// no field -- the overlay was already in the schema. A reader of the document
// must be able to compute what the text states.
func TestTheBoundIsInTheJSONToo(t *testing.T) {
	lc, f := judged(t, "invitation-honored-late",
		durationBinding("invitation.sent", invitationBound), engine.QuiesceAbsent, engine.NewRegister())
	d, err := Build(f, lc.ledger, lc.check, lc.run())
	if err != nil {
		t.Fatal(err)
	}
	if d.Verdict.Overlay == nil {
		t.Fatal("the derivation carries no overlay")
	}
	if d.Verdict.Overlay.Until != invitationBound {
		t.Errorf("until is %d, wanted %d", d.Verdict.Overlay.Until, invitationBound)
	}
	if d.Clock == nil {
		t.Fatal("a duration bound was judged with no clock in the derivation")
	}
	// The three answers the text gives, each computable from the document.
	if got := d.Verdict.Overlay.Until - d.Clock.Now; got != 969999999999 {
		t.Errorf("remaining is %d, wanted 969999999999", got)
	}
	if d.Verdict.StatuteValue != "bug" {
		t.Errorf("the statute's own verdict is %v", d.Verdict.StatuteValue)
	}
	if d.Selector.Actor != "bob" || d.Selector.Query != "invitation_expiry" {
		t.Errorf("the selector does not name what would settle it: %+v", d.Selector)
	}
}
