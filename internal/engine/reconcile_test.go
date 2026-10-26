package engine

import (
	"context"
	"sort"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/ledger"
)

// THE DONE-WHEN'S OWN CONTROL (issue 44, and docs/vocabulary.md's "The
// control"): reconciled over the planted-lie package, the honest-only set is
// `consistent` and the set carrying the planted claim is `bug`. Both outcomes
// are required of one example, and the third fixture -- the same two claims
// with the tag stripped -- is what shows the strip working: still `bug`, but
// no longer attributable to a lie.
//
// The observed values come from the CASE files, so this test does not invent
// the observation it then judges.
func TestReconciliationOverThePlantedLieSets(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name          string
		finding       string
		restsOnLie    bool
		plantedActors []string
	}{
		{"honest-only", "consistent", false, nil},
		{"with-planted", "bug", true, []string{"alice"}},
		{"stripped-tag", "bug", false, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l, s, st := fixture(t, "planted-lie", tc.name)
			value := caseValue(t, "planted-lie", tc.name)
			rep, err := Reconcile(ctx, st, []string{"believed", "honest_total", "rests_on_planted", "finding"}, l,
				[]Pass{{
					Query:  "user.count",
					Of:     "user.created",
					Source: ledger.SourceAPI,
					Values: map[string]any{s.Observer: value},
				}}, Bindings{}, NewRegister())
			if err != nil {
				t.Fatalf("reconciling: %v", err)
			}
			if len(rep.Actors) == 0 {
				t.Fatal("the pass observed nobody")
			}
			f, ok := rep.Findings[Key(s.Observer, "user.count")]
			if !ok {
				t.Fatalf("no finding for %s: %v", s.Observer, rep.Findings)
			}
			if f.Value != tc.finding {
				t.Errorf("finding %v, wanted %v", f.Value, tc.finding)
			}
			if f.Rules["rests_on_planted"] != tc.restsOnLie {
				t.Errorf("rests_on_planted %v, wanted %v", f.Rules["rests_on_planted"], tc.restsOnLie)
			}
			if strings.Join(rep.Planted, ",") != strings.Join(tc.plantedActors, ",") {
				t.Errorf("planted actors %v, wanted %v", rep.Planted, tc.plantedActors)
			}
			t.Logf("PROOF %d actor(s), finding %v, rests_on_planted %v, planted actors %v",
				len(rep.Actors), f.Value, f.Rules["rests_on_planted"], rep.Planted)
		})
	}
}

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

// A FULL RECONCILIATION IS REFUSED WHEN IT IS NOT FULL, and each refusal is
// read off the clause that answered. A composite "err != nil" would pass if
// any one of these fired for every case.
func TestReconciliationRefusesAnIncompletePass(t *testing.T) {
	ctx := context.Background()
	l, _, st := fixture(t, "transitive-count", "charlie-explained")
	rules := []string{"finding"}
	full := Pass{Query: "user.count", Of: "user.created", Source: ledger.SourceAPI,
		Values: map[string]any{"alice": 2, "bob": 2, "charlie": 2}}

	// The permitted twin FIRST, so a refusal below cannot be the fixture.
	if _, err := Reconcile(ctx, st, rules, l, []Pass{full}, Bindings{}, NewRegister()); err != nil {
		t.Fatalf("the complete pass was refused: %v", err)
	}

	for _, tc := range []struct {
		name, clause string
		passes       []Pass
	}{
		{
			"an actor with no observed value", "a full reconciliation observes every actor",
			[]Pass{{Query: "user.count", Of: "user.created", Source: ledger.SourceAPI,
				Values: map[string]any{"alice": 2}}},
		},
		{
			"a value for someone the prefix does not name", "disagree about who exists",
			[]Pass{{Query: "user.count", Of: "user.created", Source: ledger.SourceAPI,
				Values: map[string]any{"alice": 2, "bob": 2, "charlie": 2, "nobody": 9}}},
		},
		{
			"a pass with no query", "asks nothing",
			[]Pass{{Of: "user.created", Source: ledger.SourceAPI,
				Values: map[string]any{"alice": 2, "bob": 2, "charlie": 2}}},
		},
		{"no pass at all", "puts no question to anyone", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Reconcile(ctx, st, rules, l, tc.passes, Bindings{}, NewRegister())
			if err == nil {
				t.Fatal("accepted")
			}
			if !strings.Contains(err.Error(), tc.clause) {
				t.Fatalf("refused by the wrong clause: %v", err)
			}
		})
	}
}

// A SEALED PREFIX CAN BE EMPTY, which is why the no-actors clause is not dead
// code: positions come from the file, so a ledger whose first fact sits above
// the seal has a seal, passes Input, and names nobody. Reconciling over it
// would report zero disagreements, which reads exactly like agreement
// everywhere.
func TestReconciliationRefusesAPrefixThatNamesNobody(t *testing.T) {
	l, err := ledger.Load(strings.NewReader(
		`{"kind":"claim","position":5,"actor":"alice","type":"user.created","ref":"alice.user#1"}`+"\n"), false)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.SealAt(3); err != nil {
		t.Fatalf("sealing below the first fact: %v", err)
	}
	if len(l.SealedPrefix()) != 0 {
		t.Fatalf("this fixture's prefix holds %d fact(s), so it cannot show an empty one", len(l.SealedPrefix()))
	}
	_, err = Reconcile(context.Background(), countStatute, countRules, l,
		[]Pass{{Query: "user.count", Of: "user.created", Source: ledger.SourceAPI,
			Values: map[string]any{}}}, Bindings{}, NewRegister())
	if err == nil {
		t.Fatal("a reconciliation over a prefix naming nobody was accepted")
	}
	if !strings.Contains(err.Error(), "names no actor") {
		t.Fatalf("refused by the wrong clause: %v", err)
	}
	t.Logf("PROOF seal 3 over a first fact at position 5: prefix empty, reconciliation refused")
}

// EVERY PENDING IS RE-CHECKED AT EVERY FULL RECONCILIATION, and one that no
// pass covers makes the reconciliation not full -- which is reported rather
// than passed over, because an unexamined pending looks identical to a cleared
// one in a count of open findings.
func TestReconciliationReChecksEveryPendingAndRefusesWhenOneSurvives(t *testing.T) {
	ctx := context.Background()
	b := Bindings{Eventual: []Eventual{{Of: "item.created", Bound: 10}}}
	l := timedLedger(t, 100, 105)

	// Two queries from alice, both disagreeing, so two pendings are open.
	reg := NewRegister()
	for _, q := range []string{"item.count", "item.total"} {
		if _, err := Judge(ctx, countStatute, countRules, l,
			Check{Actor: "alice", Query: q, Of: "item.created", Source: ledger.SourceAPI, Value: 0},
			b, reg); err != nil {
			t.Fatal(err)
		}
	}
	if reg.Len() != 2 {
		t.Fatalf("%d pending(s) open, wanted 2", reg.Len())
	}

	// A reconciliation covering ONE of the two queries is not full.
	partial, err := Reconcile(ctx, countStatute, countRules, l,
		[]Pass{{Query: "item.count", Of: "item.created", Source: ledger.SourceAPI,
			Values: map[string]any{"alice": 1}}}, b, reg)
	if err == nil {
		t.Fatal("a reconciliation that left a pending unexamined was reported as full")
	}
	if !strings.Contains(err.Error(), "did not re-check") {
		t.Fatalf("refused by the wrong clause: %v", err)
	}
	if got := partial.StillOpen["alice"]; len(got) != 1 || got[0] != "item.total" {
		t.Fatalf("the report names %v as still open, wanted item.total", got)
	}
	// It still DID re-check the one it covered: the refusal is about fullness,
	// not about the work it did.
	if len(partial.ReChecked) != 1 || partial.ReChecked[0] != Key("alice", "item.count") {
		t.Errorf("re-checked %v, wanted alice/item.count", partial.ReChecked)
	}

	// Covering both is full, and clears both.
	reg2 := NewRegister()
	for _, q := range []string{"item.count", "item.total"} {
		if _, err := Judge(ctx, countStatute, countRules, l,
			Check{Actor: "alice", Query: q, Of: "item.created", Source: ledger.SourceAPI, Value: 0},
			b, reg2); err != nil {
			t.Fatal(err)
		}
	}
	both := []Pass{
		{Query: "item.count", Of: "item.created", Source: ledger.SourceAPI,
			Values: map[string]any{"alice": 1}},
		{Query: "item.total", Of: "item.created", Source: ledger.SourceAPI,
			Values: map[string]any{"alice": 1}},
	}
	rep, err := Reconcile(ctx, countStatute, countRules, l, both, b, reg2)
	if err != nil {
		t.Fatalf("the full reconciliation was refused: %v", err)
	}
	if len(rep.ReChecked) != 2 {
		t.Errorf("re-checked %v, wanted both of alice's pendings", rep.ReChecked)
	}
	if reg2.Len() != 0 {
		t.Errorf("%d pending(s) survived a full reconciliation: %v", reg2.Len(), reg2.Open())
	}
	t.Logf("PROOF one pass left item.total open and was refused; two passes re-checked %d and left 0 open",
		len(rep.ReChecked))
}

// A QUIESCE PENDING RE-CHECKED AND STILL OPEN IS NOT SILENT, which is the
// uncovered half of the register invariant (ratchet, coop r6/11).
//
// THE OLD GUARD ASKED "WAS THE QUERY RE-ASKED" as a proxy for "was the pending
// resolved". For a duration bound the two agree, because a re-checked duration
// pending either clears or becomes a finding; a quiesce bound does not expire
// on its own, so a busy pass re-registers it and the proxy reported nothing.
//
// IT WAS UNREACHABLE BEFORE Pass.Quiesce EXISTED, which is why this arm comes
// with that field: the Check a pass built took the zero value, QuiesceAbsent
// refuses, and a quiesce-bounded project could not reconcile at all. Measured
// at v0.1.33: one busy pending plus one covering pass errored with "no quiesce
// answer was supplied (absent)".
func TestAQuiescePendingReCheckedAndStillOpenIsReported(t *testing.T) {
	ctx := context.Background()
	b := Bindings{Eventual: []Eventual{{Of: "item.created", Quiesce: true}}}
	build := func() *ledger.Ledger {
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

	// STILL BUSY: the pass re-checks it, it does not resolve, and the
	// reconciliation must say so rather than return nil.
	l := build()
	reg := NewRegister()
	busy := Check{Actor: "alice", Query: "item.count", Of: "item.created",
		Source: ledger.SourceAPI, Value: 0, Quiesce: QuiesceBusy}
	if _, err := Judge(ctx, countStatute, countRules, l, busy, b, reg); err != nil {
		t.Fatalf("judging a busy quiesce check: %v", err)
	}
	if reg.Len() != 1 {
		t.Fatalf("%d pending(s) open before the reconciliation, wanted 1", reg.Len())
	}
	rep, err := Reconcile(ctx, countStatute, countRules, l,
		[]Pass{{Query: "item.count", Of: "item.created", Source: ledger.SourceAPI,
			Values: map[string]any{"alice": 0}, Quiesce: QuiesceBusy}}, b, reg)
	if err == nil {
		t.Fatal("a pending that survived a full reconciliation was reported as clean")
	}
	if !strings.Contains(err.Error(), "STILL OPEN") {
		t.Fatalf("refused by the wrong clause: %v", err)
	}
	// NOT the pass-set clause: this pass set WAS full.
	if strings.Contains(err.Error(), "pass set was not full") {
		t.Fatalf("a full pass set was blamed for fullness: %v", err)
	}
	if got := rep.StillOpen["alice"]; len(got) != 1 || got[0] != "item.count" {
		t.Fatalf("the report names %v as still open, wanted item.count", got)
	}

	// THE CONTROL, and it is the one that makes the assertion above mean
	// something: the same pass with the probes reading QUIET resolves the
	// pending, leaves the register empty and is not refused. Without it this
	// arm would pass against a Reconcile that refused every quiesce pass.
	l2 := build()
	reg2 := NewRegister()
	busy2 := busy
	if _, err := Judge(ctx, countStatute, countRules, l2, busy2, b, reg2); err != nil {
		t.Fatal(err)
	}
	quiet, err := Reconcile(ctx, countStatute, countRules, l2,
		[]Pass{{Query: "item.count", Of: "item.created", Source: ledger.SourceAPI,
			Values: map[string]any{"alice": 1}, Quiesce: QuiesceQuiet}}, b, reg2)
	if err != nil {
		t.Fatalf("a quiet reconciliation was refused: %v", err)
	}
	if reg2.Len() != 0 {
		t.Errorf("%d pending(s) survived a quiet reconciliation: %v", reg2.Len(), reg2.Open())
	}
	if len(quiet.ReChecked) != 1 {
		t.Errorf("re-checked %v, wanted alice/item.count", quiet.ReChecked)
	}
	t.Logf("PROOF busy left 1 open and was refused by the STILL OPEN clause; quiet re-checked %d and left 0",
		len(quiet.ReChecked))
}

// BOTH CAUSES AT ONCE, EACH MESSAGE TRUE OF WHAT IT LISTS. ratchet drove this
// at 53818e3 (coop r6/22) and the unasked clause named BOTH pendings as "did
// not re-check" -- it interpolated StillOpen, the union -- so item.count,
// which had been re-checked and survived, was reported as unexamined and the
// words STILL OPEN never appeared. A reader would fix the pass set, re-run,
// and only then learn the system had not settled.
func TestTheUnaskedClauseNamesOnlyTheUnasked(t *testing.T) {
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

	// Two pendings. The pass covers item.count only, and it comes back BUSY --
	// so item.count is re-checked and survives, item.total is never asked.
	reg := NewRegister()
	for _, q := range []string{"item.count", "item.total"} {
		if _, err := Judge(ctx, countStatute, countRules, l,
			Check{Actor: "alice", Query: q, Of: "item.created", Source: ledger.SourceAPI,
				Value: 0, Quiesce: QuiesceBusy}, b, reg); err != nil {
			t.Fatal(err)
		}
	}
	rep, err := Reconcile(ctx, countStatute, countRules, l,
		[]Pass{{Query: "item.count", Of: "item.created", Source: ledger.SourceAPI,
			Values: map[string]any{"alice": 0}, Quiesce: QuiesceBusy}}, b, reg)
	if err == nil {
		t.Fatal("a reconciliation leaving two pendings open was reported as clean")
	}
	// The unasked clause fires first and must name item.total ALONE.
	if !strings.Contains(err.Error(), "item.total") {
		t.Fatalf("the refusal does not name the unasked pending: %v", err)
	}
	if strings.Contains(err.Error(), "item.count") {
		t.Fatalf("the unasked clause names a pending that WAS re-checked: %v", err)
	}
	// StillOpen is the union on purpose: it is the register's contents, and a
	// reader needs both. The MESSAGE is what had to stop conflating them.
	if got := rep.StillOpen["alice"]; len(got) != 2 {
		t.Fatalf("the report names %v, wanted both pendings", got)
	}
	t.Logf("PROOF the unasked clause named item.total alone while StillOpen carried both")
}
