package hygiene

// A CHOSEN RED. This arm FAILS ON PURPOSE and is removed in the very next
// commit. It exists to prove, on the runner and not on anyone's box, that the
// CI failure view carries a failing arm's own words -- the thing e9f637b
// changed and that no green run can show, because a green run never enters the
// failure path (fable, coop r9/142: "it is SEEN FIRING ONCE on purpose").
//
// IT PROVES THE TWO HALVES SEPARATELY, which is why there are two sentinels
// and filler between them:
//
//   CHOSEN-RED-NEAR sits immediately before the `--- FAIL` line, inside the
//   view's -C 5 window, so it must appear in the MINED VIEW.
//
//   CHOSEN-RED-FAR sits more than five log lines earlier, OUTSIDE that window,
//   so it must NOT appear in the view and MUST appear in the whole test.log
//   printed beneath it. Printing and surviving are different halves and this
//   tells them apart in one run.
//
// If either sentinel is missing from the job log, the head did not do what it
// claims and the next commit restores this file anyway -- the proof is the
// READING, not the restore.

import "testing"

func TestCHOSENREDProvesTheFailureViewCarriesTheWords(t *testing.T) {
	t.Logf("CHOSEN-RED-FAR: only the whole-log half can carry this line")
	for i := 1; i <= 6; i++ {
		t.Logf("chosen-red filler line %d, pushing FAR outside the view's window", i)
	}
	t.Errorf("CHOSEN-RED-NEAR: the mined view must carry this line")
}
