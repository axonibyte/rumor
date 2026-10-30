package shrink

import (
	"context"
	"strings"
	"testing"
)

// A CANDIDATE THAT DID NOT SETTLE IS REFUSED, AND NOTHING NOTICED WHEN THAT
// GUARD WAS REMOVED.
//
// rook's gate verdict at coop rumor/1222, D11 condition 2: mutate
// `if cand.PendingOpen != 0` to `&& false` at v0.1.37 and the whole suite stays
// green -- 36 ok, rc 0, clean and mutated alike. The refusal's own words appear
// in `Once`'s unsettled-candidate refusal and in NO test, and `PendingOpen:
// [1-9]` matched nowhere in
// the tree. The guard enforces 92's first done-when and was the one thing in 92
// with no arm: my four-refusals shape, on the sentence the phase exists for.
//
// THE ARM IS A UNIT ARM AND THAT IS A MEASURED LIMIT, NOT A PREFERENCE. fable
// placed this as an arm "driven through the product" (coop rumor/1224 item 3),
// and the product cannot reach the state. Measured four ways at 94c413b, each
// with the binaries built from that tree:
//
//   - a probe that never reads quiet, 2s deadline: FOUND quiesce_timeout,
//     "0 pending(s) open at the end"
//   - the same at a 120s deadline: the run waits out the deadline and still
//     ends quiesce_timeout
//   - seed 12 under `settling` with a never-quiet probe: the pending IS held at
//     step 22 and the end-of-run reconciliation re-checks it, "0 open"
//   - the probes removed and the eventual type bounded `30s` instead: rc 2,
//     refused, "its bound cannot be measured" -- no clock fact writer exists
//
// THE DOOR OPENED AT R7.1, AND THE REASON THIS COMMENT GAVE WAS WRONG BEFORE
// IT DID. It read: "walk.go's end-of-run reconciliation sits inside `if
// p.Prober != nil`, so a project WITH a prober always resolves what it held,
// and a project WITHOUT one cannot hold a pending at all". The second clause
// expired as predicted -- R7.1 gives the run a clock, so a duration bound is
// no longer refused -- but THE FIRST CLAUSE WAS NEVER TRUE, and that is the
// part worth keeping. A re-ask does not resolve a pending; it re-judges it. A
// duration bound whose run has not moved its clock re-judges to `pending`, so
// the reconciliation runs, the prober is present, and the pending is still
// held at the end (quill, coop r7/8; rook /1264 found the pair of
// contradictory report lines it produces).
//
// MEASURED THROUGH THE PRODUCT, not reasoned: `rumor run` over examples/toy
// with `event.tallied` bounded `30s` and one `[clock]`, seed 12 at 24 steps
// under `settling`, prints "1 pending(s) open at the end". fable has ruled
// that state a refusal (coop r7/9), which is a change to the RUN's status and
// not to this guard: a candidate that did not settle is still a candidate
// whose finding is where the walk stopped asking.
//
// SO THE ARM IS LIVE AND ITS REASON IS NOW ITS OWN. rook re-drove the mutant
// at 5e89313 and it still dies here (coop r7 rumor/1263), which is why this is
// prose corrected in place rather than coverage to restore -- a comment found
// wrong is how the next reader talks themselves into deleting the arm beside
// it. The two refusals ratchet named at coop r6/188,
// `TestAQuiesceBoundWithNoProberIsRefused` and
// `TestADurationBoundIsRefusedNamingTheDuration` in tests/run, still assert
// what they assert; what they no longer add up to is "unreachable".
//
// THE CONTROL IS THE SETTLED CANDIDATE, because a refusal that fired on every
// candidate would satisfy the assertion above and break every other arm in a
// way this one could not see.
func TestACandidateThatDidNotSettleIsRefusedRatherThanJudged(t *testing.T) {
	unsettled := func(open int) func(context.Context) (Candidate, error) {
		return func(context.Context) (Candidate, error) {
			return Candidate{Finding: bug(), PendingOpen: open}, nil
		}
	}

	for _, open := range []int{1, 7} {
		_, err := Once(context.Background(), &fake{}, cp, wantBug(), unsettled(open))
		if err == nil {
			t.Fatalf("a candidate holding %d pending(s) was JUDGED: its finding is where the "+
				"walk stopped asking rather than what the trace causes, and comparing it makes "+
				"a reduction look like a reproduction on the strength of the clock", open)
		}
		for _, want := range []string{"did not settle", "where the walk stopped asking"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%d pending(s): the refusal does not say %q: %v", open, want, err)
			}
		}
		// THE FIGURE IS NAMED, so a reader is told how many were open rather
		// than only that something was.
		if !strings.Contains(err.Error(), "still held open") {
			t.Errorf("%d pending(s): the refusal does not say what was held: %v", open, err)
		}
	}

	// THE CONTROL: the same finding on a settled candidate reproduces.
	v, err := Once(context.Background(), &fake{}, cp, wantBug(), unsettled(0))
	if err != nil {
		t.Fatalf("a settled candidate was refused, so the refusal is not about the pending: %v", err)
	}
	if !v.Reproduced {
		t.Fatalf("the settled control did not reproduce, so the refusals above say nothing "+
			"about settling: %s", v.Why)
	}
	if v.PendingOpen != 0 {
		t.Errorf("the verdict carries PendingOpen %d for a settled candidate", v.PendingOpen)
	}
}
