package run_test

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// theFlowCounts reads the summary's two flow figures: how many steps went
// through `flow`, and how many of those a real browser drove.
var theFlowCounts = regexp.MustCompile(`(\d+) step\(s\) through ` + "`flow`" + `, (\d+) through a real browser`)

// TestAWalkDrivesADeclaredFlowThroughARealBrowser is R9.1b deliverable 2's
// end-to-end arm: the WALK drives a declared browser flow through a real
// browser, against the real system, at the address the real adapter reported.
//
// IT RUNS UNDER THE `clicking` PROFILE AND ASSERTS THE DRAW, which is fable's
// ruling (coop r9/40 part four, amended at r9/43): thirty unprofiled seeds drew
// the action NONE of the time because every unprofiled walk stops on a finding
// at step 7 to 12, so a bigger --steps cannot help and "a seed is a fact about
// today's weights". The draw is therefore checked before the browser count --
// an arm that read 0 of 0 as agreement would pass on a walk that never reached
// the action at all.
//
// AND IT MEASURES THE WHOLE CHAIN, which is why it is here and not a unit: the
// adapter reports its base on describe (fable, coop r9/111), the walk reads it
// off that session, and the flow's `navigate` resolves against it. A wrong
// address anywhere in that chain cannot produce this count -- the navigate
// would land nowhere and the flow would stop or refuse.
func TestAWalkDrivesADeclaredFlowThroughARealBrowser(t *testing.T) {
	out, errb, code := run(t, "run", "../../examples/toy",
		"--profile", "clicking", "--seed", "7", "--steps", "20", "--browser-check")

	// A BOX WITHOUT A DRIVABLE PAIR SKIPS BY NAME, carrying the reason the
	// product gave. This is the CI jail's state and it is not a failure: the
	// pair is a property of the box, and a skip that did not say which half was
	// missing would be the silent-skip class this stage exists to refuse.
	for _, miss := range []string{"SKIPPED, no drivable browser", "no browser for this run"} {
		if strings.Contains(out, miss) || strings.Contains(errb, miss) {
			t.Skipf("this box has no drivable browser, so the walk fell back to the adapter: %s",
				miss)
		}
	}
	if code != 0 {
		t.Fatalf("exit %d\nstdout:\n%s\nstderr:\n%s", code, out, errb)
	}

	m := theFlowCounts.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("the summary reports no flow counts at all:\n%s", out)
	}
	flows, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatalf("the flow count %q is not a number", m[1])
	}
	if flows < 1 {
		t.Fatalf("the `clicking` profile drew no browser action at this seed, so this arm is "+
			"VACUOUS -- the weights or the chain moved, and a browser count of 0 here would "+
			"agree with a walk that never reached the action\n%s", out)
	}

	drove, err := strconv.Atoi(m[2])
	if err != nil {
		t.Fatalf("the browser count %q is not a number", m[2])
	}
	if drove != flows {
		t.Errorf("%d step(s) went through `flow` and a real browser drove %d of them: on a run "+
			"handed a browser the walk drives EVERY declared flow through it, and a flow that "+
			"fell back to the adapter is the routing not happening\n%s", flows, drove, out)
	}
}

// TestWithoutTheFlagTheWalkDrivesTheAdapter is the other side, and it is what
// every run before R9.1b did. It is also the arm that keeps the cost opt-in:
// run 1109 died at CI's 1800s budget because a browser started on every run,
// and `tests/run` makes dozens of invocations.
//
// IT NEEDS NO BROWSER ON THE BOX, so it does not skip -- which is what makes it
// the control for the arm above rather than a second copy of it.
func TestWithoutTheFlagTheWalkDrivesTheAdapter(t *testing.T) {
	out, errb, code := run(t, "run", "../../examples/toy",
		"--profile", "clicking", "--seed", "7", "--steps", "20")
	if code != 0 {
		t.Fatalf("exit %d\nstdout:\n%s\nstderr:\n%s", code, out, errb)
	}
	m := theFlowCounts.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("the summary reports no flow counts at all:\n%s", out)
	}
	if m[1] == "0" {
		t.Fatalf("the `clicking` profile drew no browser action at this seed, so this arm is "+
			"vacuous\n%s", out)
	}
	if m[2] != "0" {
		t.Errorf("%s step(s) were driven by a real browser on a run that asked for none: the "+
			"browser is OPT-IN, and a run that starts one unasked is what cost CI 1109 its "+
			"1800s budget\n%s", m[2], out)
	}
}
