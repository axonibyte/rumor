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
	// product gave: the pair is a property of the box, and a skip that did not
	// say which half was missing would be the silent-skip class this stage
	// exists to refuse.
	//
	// AND IT IS NOT THE CI JAIL'S STATE, which this comment claimed when it was
	// written and MEASURED FALSE the same day: forge-01's ci jail carries
	// /usr/local/bin/chrome WITH /usr/local/bin/chromedriver, so the skip paths
	// below are for a box poorer than that one.
	//
	// WHICH MEANS WHETHER THIS ARM RAN ON THE FORGE IS STILL UNKNOWN, and the
	// reason is the harness and not the pair: ci.yml runs the suite without
	// `-v`, so a package line reading `ok` cannot be told from one whose arms
	// all skipped. ci.yml's own PROOF steps are where that would be closed --
	// one of them already asserts no arm of the licence suite skipped -- and
	// this remainder is carried rather than fixed here, because the workflow is
	// every head's and not this one's.
	for _, miss := range []string{"SKIPPED, no drivable browser", "no browser for this run"} {
		if strings.Contains(out, miss) || strings.Contains(errb, miss) {
			t.Skipf("this box has no drivable browser, so the walk fell back to the adapter: %s",
				miss)
		}
	}
	// 0 OR 3, BY docs/cli.md'S OWN TABLE: 0 is a clean walk and 3 is "a bug was
	// found", which is this tool's PRODUCT and not a failure. Since `clicking`
	// began claiming `page_count` this seed reaches the toy's planted stale
	// count at step 12 -- MEASURED through both oracles, so the finding is the
	// plant and not an artifact of reading the page. 1 and 2 are a refusal and a
	// could-not-run, and neither would carry the counts this arm reads.
	if code != 0 && code != 3 {
		t.Fatalf("exit %d, want 0 (clean) or 3 (a bug found)\nstdout:\n%s\nstderr:\n%s",
			code, out, errb)
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

	// AND THE PAGE, which is the other half of deliverable 4's "the DRAW and the
	// page" (fable, coop r9/116). One walk now witnesses both: the flow drawn
	// and driven through a real browser, and an observation READ from the page
	// that flow acted on. Before `clicking` claimed `page_count` this run read
	// no page at all, so the two halves could only be seen in separate walks.
	taken, read := observationCounts(t, out)
	if taken < 1 {
		t.Fatalf("the walk evaluated no observations, so this arm says nothing about the "+
			"page\n%s", out)
	}
	if read < 1 {
		t.Errorf("%d observation(s) were taken and none was read from the page: the walk drove "+
			"the flow through a browser and then asked the adapter what the page said, which is "+
			"deliverable 3 not reaching deliverable 4's walk\n%s", taken, out)
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
	if code != 0 && code != 3 {
		t.Fatalf("exit %d, want 0 (clean) or 3 (a bug found)\nstdout:\n%s\nstderr:\n%s",
			code, out, errb)
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

	// AND NO PAGE WAS READ EITHER, which is the same opt-in rule for the other
	// oracle: without the flag the adapter answers every observation from the
	// served HTML, exactly as it did before this head.
	if _, read := observationCounts(t, out); read != 0 {
		t.Errorf("%d observation(s) were read from a page on a run that asked for no browser",
			read)
	}
}
