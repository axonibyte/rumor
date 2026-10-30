package concurrency

import (
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/shrink"
)

// THE VERDICT IS JUDGED BY A MAJORITY OF MORE THAN ONE PROBE, and the arm
// asserts the PROPERTY rather than the pair.
//
// WHY NOT THE LITERALS: `2, 3` written here and `2, 3` written there is one
// fact in two places, and an arm that compares them cannot fail for the reason
// the numbers exist. What the reasoning rests on is that a single probe cannot
// decide a verdict that recurs only sometimes -- which collapses a race into
// unstable-and-never-shrunk, so the clean-world check over it is satisfied
// vacuously (rook, coop r8/38; fable's ruling 3 at rumor/1521) -- and that the
// threshold is a MAJORITY, because accepting a candidate that overfilled once
// by luck is the direction that puts a flake in a regression suite while
// refusing a real race only costs the minimal trace. Both are checked, so
// 1-of-1, 1-of-3 and 2-of-4 all fail here.
//
// AND NOT FROM A MEASURED RATE, which is rook's bound at r8/71: "a reproduction
// rate measured at one load is not a rate". This arm cites no rate either.
func TestTheOverfillVerdictIsJudgedByAMajorityOfMoreThanOneProbe(t *testing.T) {
	var w shrink.Want = Overfill{Action: "join_event"}
	p, ok := w.(shrink.Probabilistic)
	if !ok {
		t.Fatal("the overfill verdict is not Probabilistic, so internal/shrink probes it ONCE -- " +
			"and a race that recurs only sometimes is then marked unstable and never shrunk, " +
			"which satisfies the clean-world check over it vacuously")
	}
	k, n := p.Attempts()
	if n < 2 {
		t.Errorf("the verdict asks for %d probe(s): one probe cannot tell a race that never "+
			"recurs from one that recurs sometimes", n)
	}
	if k < 2 {
		t.Errorf("the threshold is %d of %d: a candidate that recurred once by luck would be "+
			"accepted, and a reduction kept on it reports a trace whose own replay would "+
			"usually find nothing", k, n)
	}
	if k*2 <= n {
		t.Errorf("the threshold is %d of %d, which is not a majority: more probes may fail than "+
			"recur and the candidate is still accepted", k, n)
	}
	t.Logf("PROOF %d of %d, a majority of more than one probe", k, n)
}

// EVERY CLAUSE OF Recurs IS DRIVEN, each by the state it exists for. The four
// are the identity of a stage verdict: whose oracle, about what, and what it
// found -- plus the absence of one at all, which is a fact and not a false.
func TestTheOverfillVerdictComparesWhoseOracleAboutWhatAndWhatItFound(t *testing.T) {
	want := Overfill{Action: "join_event"}
	found := func(rule, subject string, found bool) shrink.Candidate {
		return shrink.Candidate{Stage: &shrink.StageVerdict{
			Rule: rule, Subject: subject, Found: found, Says: "a sentence"}}
	}
	for _, c := range []struct {
		what string
		cand shrink.Candidate
		ok   bool
		says string
	}{
		{"the verdict recurs", found(OverfillRule, "join_event", true), true, ""},
		{"no stage verdict at all", shrink.Candidate{}, false, "no stage verdict"},
		{"another oracle's verdict", found("rumor.browser.overfill", "join_event", true),
			false, "two oracles"},
		{"the same oracle on another subject", found(OverfillRule, "borrow_book", true),
			false, "two verdicts"},
		{"contended and nothing found", found(OverfillRule, "join_event", false),
			false, "no overfill was found"},
	} {
		t.Run(c.what, func(t *testing.T) {
			ok, why := want.Recurs(c.cand)
			if ok != c.ok {
				t.Fatalf("Recurs said %v, wanted %v: %s", ok, c.ok, why)
			}
			if c.ok {
				if why != "" {
					t.Errorf("a recurrence carries a reason: %q", why)
				}
				return
			}
			if !strings.Contains(why, c.says) {
				t.Errorf("the reason does not say %q: %q", c.says, why)
			}
			t.Logf("PROOF %s", why)
		})
	}
}

// A TRY BECOMES A VERDICT IN Contention's OWN WORDS, and a try that found
// nothing says why rather than going quiet.
//
// THE SENTENCE IS NOT COMPOSED TWICE, which is engine.Finding.Says's rule: only
// the author of a verdict knows what its numbers mean. So the overfill's
// sentence is Contention.Finding's and the silent cases carry the vacuity
// reason, which is the one thing that tells a reader of an unstable candidate
// what happened instead.
func TestATryBecomesAVerdictInTheOraclesWords(t *testing.T) {
	over := Try{Contention: Contention{Action: "join_event", Limit: 1, Before: 0,
		Accepted: 2, Observed: 2, Width: 2}}
	v := over.Verdict("join_event")
	if !v.Found {
		t.Fatal("an overfill did not become a found verdict")
	}
	if v.Rule != OverfillRule || v.Subject != "join_event" {
		t.Errorf("the verdict is %q on %q", v.Rule, v.Subject)
	}
	if v.Says != over.Contention.Finding() {
		t.Errorf("the verdict's sentence is composed a second time:\n%q\n%q",
			v.Says, over.Contention.Finding())
	}

	// A VACUOUS TRY CARRIES ITS VACUITY. The clause could not be answered, so
	// "no overfill" would be this stage's setup answering and not the system.
	vac := Try{Contention: Contention{Action: "join_event", Limit: 1, Before: 0,
		Accepted: 0, Observed: 0, Width: 2}}
	vv := vac.Verdict("join_event")
	if vv.Found {
		t.Error("a try that answered nothing became a found verdict")
	}
	if vv.Says != vac.Contention.Vacuous() {
		t.Errorf("a vacuous try's verdict does not carry its reason: %q", vv.Says)
	}

	// AND A CLEAN ONE SAYS SO, because a verdict with an empty sentence reads
	// as a verdict nobody wrote.
	clean := Try{Contention: Contention{Action: "accept_invitation", Limit: 1, Before: 0,
		Accepted: 1, Observed: 1, Width: 2}}
	cv := clean.Verdict("accept_invitation")
	if cv.Found || cv.Says == "" {
		t.Errorf("a clean try's verdict is %+v", cv)
	}
	t.Logf("PROOF over %q; vacuous %q; clean %q", v.Says, vv.Says, cv.Says)
}
