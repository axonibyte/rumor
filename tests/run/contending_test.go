// AXIS 1 OF D10's PARTNER: HOLD THE WORLD, VARY THE STAGE.
package run_test

import (
	"os"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/decl"
	"forge.axonibyte.com/axonibyte/rumor/internal/platform"
)

// contention_test.go's TestTheOverfillClauseFiresOnTheRacedActionAndNotOnTheLockedOne
// is the OTHER axis, and this is deliberately not a second copy of it: that one
// holds the stage and varies the DEFECT -- `joinEvent` drops the lock,
// `acceptInvitation` holds it -- which is rook's correction at coop r8/37. This
// holds the defect and varies the STAGE, which is the claim D10 makes: the
// SEQUENTIAL walk reaches the capacity action AT CAPACITY and reports nothing,
// while the fan-out finds the overfill.
//
// THE STATE HAS TO BE MANUFACTURED, which is the whole reason examples/toy gains
// the `contending` and `racing` profiles. ibis drove the default mix at coop
// r8/33 -- 503 steps over seeds 1-12, join_event drawn THIRTY times, ZERO
// at-capacity refusals -- because joins SPREAD: create_event at weight 15 makes
// new events as readily as joins fill old ones, and the handle draw is scoped to
// what an actor KNOWS, so every actor joins the event it made. A seed search over
// 60 seeds at 200 steps under `contending` found three seeds that reach it; seed
// 34 reaches it thirteen times inside 37 steps.
//
// THE TWO PROFILES DIFFER IN THE STAGE LIST ALONE, which is what makes this a
// measurement about the stage. An unprofiled run also finds the race -- the toy's
// own `[stages]` note says so -- but it runs four stages under the DEFAULT
// weights, varying the mix and the stage list at once.
//
// WHY 37 STEPS, RE-MEASURED ON THE FOUR-LINE MIX. The bound excludes a finding
// this seed reaches later, and the numbers are the committed tree's:
//
//	37 steps   rc 0, 14 at-capacity, no finding
//	38 steps   rc 0, 15 at-capacity, no finding
//	47 steps   rc 3, 15 at-capacity, `FOUND bug after step 47 ... rests_on_planted true`
//	200 steps  rc 3, 15 at-capacity, the same step-47 finding
//
// SO THE BOUND IS A DECLARED INPUT AND NOT A SUPPRESSION: 37 < 47 excludes that
// row, and 38 would too. The number is kept at 37 because it is what the arm
// below was driven at.
//
// AND THE REASON PRINTED HERE FIRST WAS WRONG, found by ibis clearing this
// commit (coop r8/92): it said the finding was at step 38, `rests_on_planted
// false`, with 13 at-capacity at 200 steps. Every one of those was measured on
// the EIGHT-override mix, before the ablation cut it to four -- a figure true of
// a tree the commit then changed, which is the same error as quoting a parent's
// path count. A figure in a comment or a message is re-read on the tree being
// committed, not inherited from the version it was measured on.
func TestTheWalkReachesCapacityAndIsSilentWhileTheFanOutFindsIt(t *testing.T) {
	const toy = "../../examples/toy"

	// THE API SIDE. The walk alone, reaching a full event and saying nothing
	// about it.
	out, errb, code := run(t, "run", toy, "--seed", "34", "--steps", "37",
		"--profile", "contending")

	// REACHED, COUNTED FIRST, because every assertion after it is about silence
	// and silence over an event that was never full is the vacuous pass this
	// whole arm exists to avoid. ibis's zero is what it has to beat.
	//
	// THE PROXY IS A 409 ON join_event, AND ITS OTHER READING IS EXCLUDED BY THE
	// BINDINGS: cmd/toy answers 409 for "already joined" as well as for "at
	// capacity", and `join_event` declares `requires = ["may_join", ...]` whose
	// `may_join` is `knows_of_subject AND not already_joined`. So a join the walk
	// DREW is not one the ledger thinks is already joined, and the remaining
	// reading is capacity.
	reached := strings.Count(out, "join_event REFUSED")
	at409 := strings.Count(out, "answered 409")
	if reached == 0 || at409 == 0 {
		t.Fatalf("the walk reached no refused join (%d refusals, %d 409s), so the silence "+
			"asserted below would be silence about an event that was never full -- the "+
			"profile or the seed has stopped manufacturing the state\nstdout:\n%s",
			reached, at409, out)
	}

	// AND SILENT. Both halves, because `code` alone would also be 0 on a run that
	// found nothing because it reached nothing, and that case is excluded above.
	if code != 0 {
		t.Errorf("the api side exited %d, so it FOUND something and the silence this arm "+
			"reports is not the stage's\nstdout:\n%s\nstderr:\n%s", code, out, errb)
	}
	if strings.Contains(out, "run: FOUND") {
		t.Errorf("the walk reported a finding while at capacity:\n%s", out)
	}
	// AND IT IS THE WALK THAT RAN, not the fan-out under another name: a
	// `concurrency:` line here would mean the profile's stage list stopped
	// holding and the "api reported nothing" reading would be about a run that
	// never walked.
	if strings.Contains(out, "concurrency:") {
		t.Errorf("the `contending` profile ran the concurrency stage, so this is not the "+
			"walk alone:\n%s", out)
	}

	// THE CONCURRENCY SIDE. Same world, same weights, same seed; the stage list
	// is the only thing that moved.
	rout, rerr, rcode := run(t, "run", toy, "--seed", "34", "--steps", "37",
		"--profile", "racing")

	if rcode == 0 {
		t.Fatalf("the fan-out exited 0 against the same world the walk found full: the "+
			"comparison has nothing in it\nstdout:\n%s\nstderr:\n%s", rout, rerr)
	}
	// THE SENTENCE AND NOT JUST THE CODE, because rc 3 is satisfied by any
	// finding and this arm is about the OVERFILL. It is quoted loosely enough to
	// survive a reworded limit but tightly enough to name the clause.
	if !strings.Contains(rout, "left the system holding") || !strings.Contains(rout, "limit of") {
		t.Errorf("the fan-out exited %d without naming the overfill clause:\n%s", rcode, rout)
	}
	// AND THE WALK DID NOT RUN ON THIS SIDE, the mirror of the guard above.
	if strings.Contains(rout, "run:   1 ") {
		t.Errorf("the `racing` profile walked, so the two sides differ in more than the "+
			"stage:\n%s", rout)
	}
}

// TestTheApiSidesEffectiveMixIsTheMeasuredOne replaces an arm that guarded the
// wrong invariant, and the replacement is ibis's measurement at coop r8/94.
//
// THE ARM BEFORE THIS ONE compared `contending`'s weights table to `racing`'s,
// because `racing`'s comment claimed they had to match for the two runs to be a
// comparison about the STAGE. ibis measured that claim from both sides:
//
//	NOT NECESSARY -- `internal/concurrency` holds ZERO references to the weights
//	table, so the fan-out never draws. racing's four weights set to 1/99/7/5
//	leaves stdout byte-identical and rc 3 either way.
//	NOT SUFFICIENT -- `[weights.stage.api] create_event = 8` moves the api
//	side's EFFECTIVE mix while both profile tables stay equal, and the old arm
//	passed.
//
// An invariant that is neither necessary nor sufficient is noise, so racing's
// weights table is gone (it was inert) and this pins the thing the measurement
// actually rests on: what the API side RESOLVES, through
// Weights.Weight(stage, profile, kind) and its profile-then-stage-then-default
// precedence.
//
// create_event IS PINNED AT ITS DEFAULT ON PURPOSE. The ablation that chose this
// mix found `create_event = 8` COSTS at-capacity reaches, so the profile
// deliberately does not override it and 15 arrives by fall-through -- which
// means a stage-level override is exactly how it could move without any profile
// being edited. A kind the profile leaves alone is still part of the measured
// mix.
func TestTheApiSidesEffectiveMixIsTheMeasuredOne(t *testing.T) {
	raw, err := os.ReadFile("../../examples/toy/platform.toml")
	if err != nil {
		t.Fatalf("the toy's platform: %v", err)
	}
	doc, err := decl.TOML(raw)
	if err != nil {
		t.Fatalf("the toy's platform is not TOML: %v", err)
	}
	rep := platform.Parse(doc)
	if !rep.OK() {
		t.Fatalf("the toy's platform does not parse: %v", rep.Refusals)
	}

	for _, w := range []struct {
		kind string
		want int
		how  string
	}{
		{"join_event", 40, "the profile"},
		{"tell", 12, "the profile"},
		{"leave_event", 0, "the profile"},
		{"delete_event", 0, "the profile"},
		{"create_event", 15, "FALL-THROUGH to [weights.default]"},
	} {
		got, ok := rep.Platform.Weights.Weight("api", "contending", w.kind)
		if !ok {
			t.Errorf("%s resolves to no weight at all under (api, contending), so the mix "+
				"the arm above was driven under is not the one declared", w.kind)
			continue
		}
		if got != w.want {
			t.Errorf("%s resolves to %d under (api, contending), want %d by %s: the api "+
				"side's effective mix has moved, so the at-capacity reaches above are "+
				"being measured under a different world", w.kind, got, w.want, w.how)
		}
	}

	// AND NO STAGE-LEVEL TABLE FOR api EXISTS AT ALL, which closes the CLASS
	// rather than enumerating its members. ibis's residue at coop r8/97: the
	// loop above pins FIVE kinds where `[weights.default]` carries thirteen, so
	// `[weights.stage.api] create_user = 1` resolves past it. Pinning all
	// thirteen would be brittle and STILL incomplete -- a kind added to the
	// default table tomorrow is unpinned the moment it lands.
	//
	// THE MEASUREMENT'S REAL PRECONDITION IS THAT NOTHING OVERRIDES api AT THE
	// STAGE LEVEL, because precedence is profile, then STAGE, then default: a
	// stage table silently outranks every fall-through the ablation chose. So
	// the absence is the invariant, and it is one assertion instead of thirteen.
	//
	// IF A LATER CHANGE WANTS `[weights.stage.api]`, THIS ARM SHOULD FAIL. It is
	// not in the way of that change; it is the notice that the at-capacity
	// figures above were taken under a different mix and have to be retaken.
	if t2, ok := rep.Platform.Weights.Stage["api"]; ok && len(t2) > 0 {
		t.Errorf("`[weights.stage.api]` declares %d weight(s), which outrank every kind the "+
			"`contending` profile leaves at its default: the api side's effective mix is no "+
			"longer the one the at-capacity figures were measured under, and they must be "+
			"retaken rather than this arm relaxed: %v", len(t2), t2)
	}

	// AND racing DECLARES NO WEIGHTS, which is what makes its side of the
	// comparison unambiguous rather than merely equal. If a table reappears
	// there, the premise in its comment has drifted back.
	if t2, ok := rep.Platform.Weights.Profile["racing"]; ok && len(t2) > 0 {
		t.Errorf("`racing` declares %d weight(s) again: the fan-out does not draw, so a mix "+
			"there is inert and its presence misleads a reader into the premise ibis "+
			"disproved: %v", len(t2), t2)
	}
}
