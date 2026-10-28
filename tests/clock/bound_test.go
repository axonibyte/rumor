package clock_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// THE DURATION BOUND IS JUDGED WHERE THE RUN HOLDS A CLOCK, AND REFUSED WHERE
// IT DOES NOT. R2.3 declared the form, R6 left it refused for want of a writer
// (ratchet, coop r6/155; rook, rumor/1196), and R7.1's writer is what makes
// the comparison at pending.go:316 run at all.
//
// WHAT THESE ARMS DELIBERATELY DO NOT PIN. A clocked run whose disagreement is
// held ends with that pending OPEN, because the clock cannot move until R7.3
// draws `advance` and so the re-check always reads "still inside the bound".
// Whether that is correct-and-temporary, a finding, or a run-time refusal is
// fable's to rule (quill, coop r7/8), and it is product output in two of the
// three readings. So these assert what no reading changes: that the bound was
// MEASURED rather than refused, and that the refusal is untouched where no
// clock is declared.

// durationBounded is examples/toy with `event.tallied`'s bound changed from
// the quiesce form to a duration, and `[clock]` added or not. Both edits are
// asserted: a copy whose substitution missed would drive the quiesce form and
// read exactly as R6 did, which is a green arm over nothing.
func durationBounded(t *testing.T, withClock bool) string {
	t.Helper()
	dir := t.TempDir()
	entries, err := os.ReadDir(project)
	if err != nil {
		t.Fatalf("reading %s: %v", project, err)
	}
	const quiesceForm = "[claim.\"event.tallied\"]\ntiming = \"eventual\"\nbound = \"quiesce\""
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		raw, rerr := os.ReadFile(filepath.Join(project, e.Name()))
		if rerr != nil {
			t.Fatalf("reading %s: %v", e.Name(), rerr)
		}
		if e.Name() == "bindings.toml" {
			before := string(raw)
			after := strings.Replace(before, quiesceForm,
				"[claim.\"event.tallied\"]\ntiming = \"eventual\"\nbound = \"30s\"", 1)
			if after == before {
				t.Fatalf("%s no longer declares `event.tallied` bounded on quiesce, so this copy "+
					"would drive the form R6 already judged and say nothing about a duration",
					filepath.Join(project, e.Name()))
			}
			if withClock {
				after += "\n[clock]\n"
			}
			raw = []byte(after)
		}
		if werr := os.WriteFile(filepath.Join(dir, e.Name()), raw, 0o644); werr != nil {
			t.Fatalf("writing %s: %v", e.Name(), werr)
		}
	}
	return dir
}

// seed 12 under `settling` is the seed the room measured a held pending from
// (ibis, coop r6/184; rook, rumor/1232; ratchet r6/192): one disagreement is
// held mid-walk and re-checked at a later step. It is used here because a seed
// that never disagrees would leave the bound unasked, and an arm over an
// unasked bound is the shape this suite exists to avoid.
const heldSeed, heldSteps, heldProfile = 12, 24, "settling"

func TestADurationBoundIsMeasuredWhenTheRunHoldsAClock(t *testing.T) {
	rec := walkProfile(t, durationBounded(t, true), heldSeed, heldSteps, heldProfile)

	if len(rec.Pendings) == 0 {
		t.Fatalf("no pending at all over %d steps of seed %d under %q: the bound was never asked, "+
			"so this says nothing about whether a duration can be measured",
			heldSteps, heldSeed, heldProfile)
	}
	// MEASURED AND NOT REFUSED is the whole claim: before the writer, this very
	// walk could not run -- the engine refused by name at the first judgment of
	// an `event.tallied` claim. A pending line existing at all is the proof the
	// comparison ran.
	joined := strings.Join(rec.Pendings, "\n")
	if !strings.Contains(joined, "held at step") {
		t.Errorf("no pending names where it was held:\n%s", joined)
	}
	// AND THE RE-CHECK IS R6's FORM, unchanged by the bound being a duration:
	// a pending is re-checked at the actor's next observation of the same query
	// and agreement closes it.
	if !strings.Contains(joined, "re-checked at step") {
		t.Errorf("no pending was re-checked AT A STEP, so the duration form is not being "+
			"re-checked the way the quiesce form is (R6.3's second path)\n%s", joined)
	}
	if !strings.Contains(joined, "consistent") {
		t.Errorf("no re-check closed a pending as consistent, so either the bound is being read "+
			"as expired or nothing settled\n%s", joined)
	}
}

func TestADurationBoundWithNoClockIsStillRefusedByName(t *testing.T) {
	// THE SAME FIXTURE, ONE DECLARATION SHORT, which is what makes this a
	// control rather than a second subject: the only difference from the arm
	// above is the `[clock]` table.
	dir := durationBounded(t, false)

	rec, err := tryWalkProfile(t, dir, heldSeed, heldSteps, heldProfile)
	if err == nil {
		t.Fatalf("the walk ran to completion with no clock declared and a duration bound: "+
			"%d pending(s), %d open -- a bound nothing can measure must not be read as a verdict",
			len(rec.Pendings), rec.PendingOpen)
	}
	for _, want := range []string{
		"bound cannot be measured",
		"time the run does not control never decides a verdict",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not say %q, so a reader is not told which half is "+
				"missing\n%v", want, err)
		}
	}
}
