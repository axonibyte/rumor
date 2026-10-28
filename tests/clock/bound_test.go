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

// THE SEEDS ARE CHOSEN BY WHAT THEY DO, measured at coop r7/11 over the
// duration-bounded copy below, not borrowed from the quiesce form's records.
//
//	66, 75, 140  hold a pending and CLOSE it: the re-check agrees, which needs
//	             no clock at all, so the run ends with nothing open
//	12           holds mid-walk and one pending survives to the end
//	8            holds at the LAST step, so no later seal exists to meet it
//	             (ibis, coop rumor/1270)
//	1            the same, and the run finds a bug besides
//
// A seed that never disagrees would leave the bound unasked, and an arm over
// an unasked bound is the shape this suite exists to avoid.
const settledSeed, heldSteps, heldProfile = 66, 24, "settling"

// heldSeed is the one the no-clock arm uses: it must reach a judgment of an
// `event.tallied` claim for the engine's own refusal to fire, and 12 does so
// at step 19.
const heldSeed = 12

func TestADurationBoundIsMeasuredWhenTheRunHoldsAClock(t *testing.T) {
	rec := walkProfile(t, durationBounded(t, true), settledSeed, heldSteps, heldProfile)

	if len(rec.Pendings) == 0 {
		t.Fatalf("no pending at all over %d steps of seed %d under %q: the bound was never asked, "+
			"so this says nothing about whether a duration can be measured",
			heldSteps, settledSeed, heldProfile)
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
	// agreement closes a pending, which needs no clock. Seed 66 closes BOTH of
	// its pendings at the end-of-run reconciliation -- which is the path the
	// room measured it on (ibis coop r6/184, rook rumor/1232) -- so that is
	// the sentence asserted here. The mid-walk path is seed 12's and is read
	// in TestTheRefusalKeepsTheRecordItRefusesOn below.
	if !strings.Contains(joined, "re-checked after the last step") {
		t.Errorf("no pending was re-checked at the reconciliation, so the duration form is not "+
			"being re-checked the way the quiesce form is\n%s", joined)
	}
	if !strings.Contains(joined, "consistent") {
		t.Errorf("no re-check closed a pending as consistent, so either the bound is being read "+
			"as expired or nothing settled\n%s", joined)
	}
	if rec.PendingOpen != 0 {
		t.Errorf("%d pending(s) open at the end of a run that settled: this seed is the control "+
			"for the end-of-run refusal and has to be a run that does not trip it",
			rec.PendingOpen)
	}
}

// TestTheRefusalKeepsTheRecordItRefusesOn: the refusal is returned BESIDE a
// complete record, deliberately, so a reader is not made to choose between
// being told the run is unmeasured and being told what it saw.
//
// IT IS ALSO WHERE THE MID-WALK RE-CHECK IS READ under a duration bound. Seed
// 12 holds one pending at step 19 and closes it AT STEP 24 -- D3a's first path,
// the actor's next observation of the same query -- and holds another that
// survives to the end, which is why the run refuses. Both facts are in the one
// record.
func TestTheRefusalKeepsTheRecordItRefusesOn(t *testing.T) {
	rec, err := tryWalkProfile(t, durationBounded(t, true), heldSeed, heldSteps, heldProfile)
	if err == nil {
		t.Fatal("seed 12 did not refuse, so this arm is not reading a refused run")
	}
	joined := strings.Join(rec.Pendings, "\n")
	if !strings.Contains(joined, "re-checked at step") {
		t.Errorf("the refused run's record carries no mid-walk re-check, so either the record "+
			"did not survive the refusal or the duration form is not re-checked at a step\n%s",
			joined)
	}
	if !strings.Contains(joined, "consistent") {
		t.Errorf("nothing in the refused run's record closed as consistent\n%s", joined)
	}
	if rec.PendingOpen == 0 {
		t.Error("the refused run's record says nothing was open, which contradicts the refusal " +
			"it came with")
	}
	if len(rec.Steps) != heldSteps {
		t.Errorf("the refused run's record has %d step(s) of %d: the refusal fires at the END, "+
			"so the walk it describes is a whole walk", len(rec.Steps), heldSteps)
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
	// AND IT IS THE ENGINE'S REFUSAL AND NOT THE WALK'S, which have to be told
	// apart now that both exist and both say "cannot be measured". This one
	// fires at the moment of judging because the project declared no clock;
	// the walk's fires at the END because the run never moved the clock it
	// has. An arm that could pass on either would stop being about this one.
	if strings.Contains(err.Error(), "moved no time") {
		t.Errorf("the refusal is the END-OF-RUN one, which cannot be right for a project that "+
			"declares no clock: there is nothing for the run to have moved\n%v", err)
	}
}

// TestARunEndingWithAnUnmeasuredPendingRefuses is fable's ruling at coop r7/15,
// reading B: a pending still unresolved at the end, under a duration bound,
// with the clock where it started, is UNMEASURED rather than inside its bound.
//
// IT IS THE RUN'S FAULT AND NOT THE PROJECT'S, which is the half the sentence
// has to carry: every declaration here is correct and the engine judged the
// bound all the way to the end. What is missing is an `advance`, and until
// R7.3 draws one no run has moved its clock.
func TestARunEndingWithAnUnmeasuredPendingRefuses(t *testing.T) {
	for _, seed := range []uint64{8, 12} {
		rec, err := tryWalkProfile(t, durationBounded(t, true), seed, heldSteps, heldProfile)
		if err == nil {
			t.Errorf("seed %d ran to the end holding %d pending(s) and did not refuse: a run "+
				"that ends with a pending it could not measure is R2.3's invariant broken",
				seed, rec.PendingOpen)
			continue
		}
		for _, want := range []string{
			"under a duration bound and moved no time",
			`"event.tallied" bounded 30s`,
			"held at step",
			"still at its epoch",
		} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("seed %d's refusal does not say %q, so it names the state without "+
					"naming what a reader can do about it\n%v", seed, want, err)
			}
		}
	}
}

// TestTheRefusalNamesTheFindingItWithholds: seed 1 ends with two unmeasured
// pendings AND a bug. fable ruled the refusal takes the run and names the
// finding beside the pendings it could not measure, so nothing is lost and
// nothing is asserted -- a finding with a pending still unmeasured beside it
// is not one the run can stand behind (rule 3's silence clause).
func TestTheRefusalNamesTheFindingItWithholds(t *testing.T) {
	_, err := tryWalkProfile(t, durationBounded(t, true), 1, heldSteps, heldProfile)
	if err == nil {
		t.Fatal("seed 1 did not refuse, so this says nothing about a withheld finding")
	}
	for _, want := range []string{
		"the run also found",
		"does not report as a finding",
		"where the walk stopped asking",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not say %q: a finding dropped in silence is the one "+
				"outcome this clause exists to prevent\n%v", want, err)
		}
	}
}

// TestAClosedPendingIsNotARefusal is the control, and it carries the weight
// here: a refusal that fired on every clocked duration-bounded run would
// satisfy both arms above. These three seeds hold a pending and close it by
// AGREEMENT at the re-check, which needs no clock -- so the bound is measured,
// nothing is left open, and the run is clean.
func TestAClosedPendingIsNotARefusal(t *testing.T) {
	for _, seed := range []uint64{66, 75, 140} {
		rec, err := tryWalkProfile(t, durationBounded(t, true), seed, heldSteps, heldProfile)
		if err != nil {
			t.Errorf("seed %d refused: %v\n\nTHAT IS THE CONTROL FAILING -- this seed's "+
				"pending closes at its re-check, so the refusal is firing on a run that "+
				"settled", seed, err)
			continue
		}
		if rec.PendingOpen != 0 {
			t.Errorf("seed %d ended with %d pending(s) open and no refusal", seed, rec.PendingOpen)
		}
		if len(rec.Pendings) == 0 {
			t.Errorf("seed %d held no pending at all, so it is not a control for this refusal "+
				"-- it never reached the state the refusal is about", seed)
		}
	}
}
