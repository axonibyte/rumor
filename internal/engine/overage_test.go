package engine

import (
	"context"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/ledger"
)

// EVERY TIME FIGURE IN THE OVERLAY'S SENTENCES IS A DURATION, which is R7.1's
// last clause and the door R6 named at coop rumor/1202.
//
// WHY IT IS WORTH AN ARM AND NOT ONLY A RENDER. The bound was already printed
// as `30s` while the claim's time, the seal's time and -- the figure a reader
// actually acts on -- the OVERAGE were bare nanosecond counts beside it. A
// sentence that mixes the two asks the reader to convert, and a reader
// converting can be out by a thousand with the sentence still looking right.
// `clock.t` is nanoseconds from the run's own epoch because the contract
// declares that unit, so rendering is the only place the unit becomes legible.
//
// THE POSITIONS STAY INTEGERS, deliberately: a position is not a time, and
// `time.Duration(19)` of a position would read "19ns" and be nonsense.
//
// THE PRODUCT PATH IS R7.3's. `cmd/rumor` prints `Overlay.Reason` for a
// finding that carries one, so the `stale` sentence reaches stdout -- but a
// duration bound can only be PAST once something moves the run's clock, and
// nothing does until `advance` is drawn. So this is armed where it can be
// driven, with the same shape as the clock-fact arms one package over.

func TestTheStaleSentenceReadsInDurationsIncludingTheOverage(t *testing.T) {
	ctx := context.Background()
	b := Bindings{Eventual: []Eventual{{Of: "item.created", Bound: 10}}}
	chk := Check{Actor: "alice", Query: "item.count", Of: "item.created", Source: ledger.SourceAPI, Value: 0}

	// THE SEQUENCE IS pending_test's, because `stale` rests on a re-check:
	// one register, the first look INSIDE the bound so a pending is held, the
	// second PAST it. Claim at 100ns, seal at 120ns, bound 10ns, so the
	// overage is exactly 10ns and the arm can name it.
	reg := NewRegister()
	if _, err := Judge(ctx, countStatute, countRules, timedLedger(t, 100, 105), chk, b, reg); err != nil {
		t.Fatal(err)
	}
	if reg.Len() != 1 {
		t.Fatalf("no pending was held, so there is nothing for a stale to be past: %d open", reg.Len())
	}
	out, err := Judge(ctx, countStatute, countRules, timedLedger(t, 100, 120), chk, b, reg)
	if err != nil {
		t.Fatal(err)
	}
	if out.Kind != "stale" || out.Overlay == nil {
		t.Fatalf("the sequence did not produce a stale with an overlay: %v %+v", out.Kind, out.Overlay)
	}

	for _, want := range []string{
		"a bound of 10ns",
		"made at 100ns into the run",
		"time at the seal is 120ns",
		"it is 10ns past the bound",
	} {
		if !strings.Contains(out.Overlay.Reason, want) {
			t.Errorf("the stale sentence does not say %q:\n%s", want, out.Overlay.Reason)
		}
	}
	// AND NO BARE COUNT IS LEFT WHERE A TIME BELONGS. The overage is the one
	// figure a reader acts on, so "past the bound" followed by an integer
	// with no unit is the specific regression this cell exists to catch.
	if strings.Contains(out.Overlay.Reason, "is 10 past the bound") {
		t.Errorf("the overage is a bare count:\n%s", out.Overlay.Reason)
	}
}

func TestThePendingSentenceReadsInDurationsToo(t *testing.T) {
	ctx := context.Background()
	b := Bindings{Eventual: []Eventual{{Of: "item.created", Bound: 10}}}
	chk := Check{Actor: "alice", Query: "item.count", Of: "item.created", Source: ledger.SourceAPI, Value: 0}

	out, err := Judge(ctx, countStatute, countRules, timedLedger(t, 100, 105), chk, b, NewRegister())
	if err != nil {
		t.Fatal(err)
	}
	if out.Value != "pending" || out.Overlay == nil {
		t.Fatalf("inside the bound the finding is %v with overlay %+v, wanted pending",
			out.Value, out.Overlay)
	}
	for _, want := range []string{
		"a bound of 10ns",
		"made at 100ns into the run",
		"time at the seal is 105ns",
	} {
		if !strings.Contains(out.Overlay.Reason, want) {
			t.Errorf("the pending sentence does not say %q:\n%s", want, out.Overlay.Reason)
		}
	}
}
