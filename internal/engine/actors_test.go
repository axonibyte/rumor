package engine

import (
	"sort"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/ledger"
)

// AN ACTOR THAT MAKES NO CLAIM IS STILL AN ACTOR. Most of the examples' own
// observers make none -- charlie and dana are told of claims and never make
// one -- so an actor list derived from claims would leave the observer out of
// its own reconciliation.
func TestActorsNamesTheToldAsWellAsTheClaimants(t *testing.T) {
	l, _, _ := fixture(t, "transitive-count", "charlie-explained")
	got := Actors(l)
	sort.Strings(got)
	if strings.Join(got, ",") != "alice,bob,charlie" {
		t.Fatalf("actors %v, wanted alice, bob and charlie: charlie makes no claim and is told of one", got)
	}
	claimants := map[string]bool{}
	for _, f := range l.SealedPrefix() {
		if f.Kind == ledger.KindClaim {
			claimants[f.Actor] = true
		}
	}
	if len(claimants) == len(got) {
		t.Error("this fixture's claimants and actors are the same set, so it cannot show the difference")
	}

	// AND THE RUN IS NOT AN ACTOR. A clock fact's actor is "the run" (v1, Fact
	// kinds), which controls time rather than holding beliefs; reconciling
	// against it opens a pending for an actor that knows nothing by definition.
	timed := timedLedger(t, 100, 105)
	for _, a := range Actors(timed) {
		if a == "run" {
			t.Error("the clock's writer is listed as an actor to reconcile against")
		}
	}
	writes := 0
	for _, f := range timed.SealedPrefix() {
		if f.Kind == ledger.KindClock && f.Actor == "run" {
			writes++
		}
	}
	if writes == 0 {
		t.Error("the timed fixture holds no clock fact written by the run, so that arm proved nothing")
	}
	t.Logf("PROOF %d actor(s) from %d claimant(s) plus the tells: %v; and %d clock fact(s) by the run add none",
		len(got), len(claimants), got, writes)
}
