package concurrency

import (
	"fmt"

	"forge.axonibyte.com/axonibyte/rumor/internal/shrink"
)

// OverfillRule is this oracle's identity, carried on every verdict it writes.
//
// IT IS NOT A REGO PATH, because no rego produced it: a statute's rule is
// `data.rumor.statute.…` and the toy's own statute says of itself that "an
// event's capacity is not a ledger fact, a capacity OVERRUN is not nameable
// from a ledger case, and this statute does not pretend otherwise". So the two
// namespaces cannot collide, and rook's condition at coop rumor/1522 is met by
// a name rather than by hoping: two oracles both finding "an overfill" are two
// different verdicts.
const OverfillRule = "rumor.concurrency.overfill"

// Overfill is the stage's verdict as a shrink Want: this action left the system
// holding more than its own declared limit allows.
//
// WHAT IT KEEPS IS THE DEFECT AND NOT THE DEGREE. A candidate recurs when the
// same oracle finds an overfill of the same ACTION -- not when it finds the
// same numbers. The quantities a race produces move with it: two of two
// requests may land this time and two of three the next, and both are the same
// defect, which is that the check and the insert are not one step. A comparison
// on the counts would read a recurrence as a different finding and mark a real
// race unstable; one on the action and the oracle reads what the verdict is
// about.
type Overfill struct {
	// Action is the contended action: the verdict's subject.
	Action string
}

// Describe names the verdict for a report, which an unstable seed's line needs.
func (o Overfill) Describe() string {
	return fmt.Sprintf("an overfill of %s, by %s", o.Action, OverfillRule)
}

// Recurs reports whether this candidate still gives the verdict.
func (o Overfill) Recurs(cand shrink.Candidate) (bool, string) {
	// THE CANDIDATE'S VERDICT IS THE STAGE'S, NOT THE STATUTE'S, and the
	// absence of one is a fact rather than a false: a candidate carrying no
	// stage verdict at all was produced by something that is not this stage,
	// and saying "no overfill" about it would be this want answering for a run
	// it cannot read.
	if cand.Stage == nil {
		return false, "the candidate carries no stage verdict, so nothing in it is this " +
			"oracle's to compare"
	}
	if cand.Stage.Rule != OverfillRule {
		return false, fmt.Sprintf("the verdict came from %s and this one is %s: two oracles "+
			"finding the same word are not the same finding", cand.Stage.Rule, OverfillRule)
	}
	if cand.Stage.Subject != o.Action {
		return false, fmt.Sprintf("the verdict is about %s and this one is about %s: one oracle "+
			"over two contended actions is two verdicts", cand.Stage.Subject, o.Action)
	}
	if !cand.Stage.Found {
		return false, fmt.Sprintf("%s was contended and no overfill was found: %s",
			o.Action, cand.Stage.Says)
	}
	return true, ""
}

// Attempts is the k-of-n threshold this verdict is judged against: it recurs
// twice in three probes or the candidate is refused.
//
// WHY A THRESHOLD AT ALL (fable's ruling 3, coop rumor/1521, on rook's r8/38):
// one probe collapses a verdict that recurs only sometimes into
// `Reproduced=false`, so the finding is marked unstable and never shrunk -- and
// the clean-world check over it is then satisfied VACUOUSLY, because the shrink
// never ran. A race is the common case of a verdict that recurs only sometimes.
//
// AND THE NUMBERS ARE NOT READ OFF A RATE, which is rook's bound at coop r8/71
// and the reason this comment does not cite my own 20-of-20: "a reproduction
// rate measured at one load is not a rate". The same arm's overlap read
// 30.448ms and then 212.909ms on one box at loadavg 1.42, against a window
// cmd/toy holds for 25ms, so the figure moves with whatever else the machine is
// doing. A threshold chosen from it would be a constant taken from one
// afternoon.
//
// SO THEY COME FROM THE TWO WAYS THIS GATE CAN BE WRONG, which do not depend
// on the rate at all:
//
// k TOO LOW accepts a candidate that overfilled once by luck, and the shrink
// then keeps a reduction on a verdict that mostly does not recur -- reporting,
// and in another caller PROMOTING, a trace whose own replay would usually find
// nothing. That is the failure that puts a flake in a regression suite.
//
// k TOO HIGH refuses a real race that recurs sometimes, so the finding is
// marked unstable and NOT SHRUNK -- design.md's own rule -- while the stage
// still reports the overfill it found. The product loses a minimal trace and
// keeps the finding.
//
// The second is the recoverable direction, so k is a MAJORITY: more probes
// recur than not. n is then the smallest number for which a majority exists
// AND one probe may fail without refusing the candidate, which is 3 -- at n=2
// a single non-recurrence takes k=2 below its threshold, which is the one-probe
// collapse again wearing a second probe. Two of three.
//
// WHAT IT COSTS IS STATED: a candidate is replayed up to three times, each on a
// world restored and verified, so a reduction over this verdict costs up to
// three times a deterministic one. k of n is a threshold and not a quota --
// internal/shrink stops at the kth recurrence -- so a race that reproduces
// reliably pays two probes per candidate and not three.
func (o Overfill) Attempts() (k, n int) { return 2, 3 }

// Verdict is the stage's own finding in the shape internal/shrink keeps, built
// from a try that could answer the clause.
//
// THE SENTENCE IS Contention's AND NOT COMPOSED HERE, which is the rule
// engine.Finding.Says is built on: only the author of a verdict knows what its
// numbers mean, and a second renderer would drift from the first. A try that
// found nothing carries its reason in the same field, so a reader of an
// unstable candidate is told what it produced instead.
func (t Try) Verdict(action string) *shrink.StageVerdict {
	says := t.Contention.Finding()
	found := says != ""
	if !found {
		if why := t.Contention.Vacuous(); why != "" {
			says = why
		} else {
			says = fmt.Sprintf("%s held its own limit of %d under %d overlapping request(s)",
				action, t.Contention.Limit, t.Contention.Width)
		}
	}
	return &shrink.StageVerdict{
		Rule:    OverfillRule,
		Subject: action,
		Found:   found,
		Says:    says,
	}
}
