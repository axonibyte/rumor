package run_test

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// theObservationCounts reads the summary's two observation figures: how many the
// walk evaluated, and how many a real browser read from the page.
var theObservationCounts = regexp.MustCompile(`(\d+) observation\(s\) of which (\d+) read from the page`)

func observationCounts(t *testing.T, out string) (taken, read int) {
	t.Helper()
	m := theObservationCounts.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("the summary reports no observation counts:\n%s", out)
	}
	taken, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatalf("the observation count %q is not a number", m[1])
	}
	read, err = strconv.Atoi(m[2])
	if err != nil {
		t.Fatalf("the page-read count %q is not a number", m[2])
	}
	return taken, read
}

// skipWithoutAPair skips by the PRODUCT'S own verdict and never by a fallback,
// which is fable's rule for the browser-side arms (coop r9/118).
func skipWithoutAPair(t *testing.T, out, errb string) {
	t.Helper()
	for _, miss := range []string{"SKIPPED, no drivable browser", "no browser for this run"} {
		if strings.Contains(out, miss) || strings.Contains(errb, miss) {
			t.Skipf("this box has no drivable browser: %s", miss)
		}
	}
}

// TestObservationsAreReadFromThePageWhenThereIsABrowser is R9.1b deliverable 3
// end to end: a `browser` observation is read from the LIVE DOM, at the page and
// element the real adapter reported, converted by the type it declared (fable,
// coop r9/120, r9/122 and r9/124).
//
// IT ASSERTS THE PAIR AND NOT A NUMBER. The same seed is walked twice -- once
// with the flag and once without -- and the claim is that the WALK did not move
// while the ORACLE did: the observation count is identical, and the page-read
// count goes from zero to all of them. Either figure alone proves nothing, since
// a build that observed nothing would report 0 and 0 and a build that read
// everything through the adapter would report the same total.
func TestObservationsAreReadFromThePageWhenThereIsABrowser(t *testing.T) {
	const profile, seed, steps = "viewing", "29", "14"

	withFlag, errb, code := run(t, "run", "../../examples/toy",
		"--profile", profile, "--seed", seed, "--steps", steps, "--browser-check")
	skipWithoutAPair(t, withFlag, errb)
	if code != 0 {
		t.Fatalf("with a browser: exit %d\nstdout:\n%s\nstderr:\n%s", code, withFlag, errb)
	}
	taken, read := observationCounts(t, withFlag)
	if taken < 1 {
		t.Fatalf("the walk evaluated no observations, so this arm is VACUOUS -- the profile's "+
			"claim moved, and 0 read of 0 taken would agree with any oracle\n%s", withFlag)
	}
	if read < 1 {
		t.Fatalf("%d observation(s) were taken and NONE was read from the page: the walk was "+
			"handed a browser and asked the adapter anyway, which is deliverable 3 not "+
			"happening\n%s", taken, withFlag)
	}

	// AND WITHOUT THE FLAG THE SAME WALK READS NO PAGE AT ALL, which is what
	// every run did before this head and what keeps the cost opt-in.
	plain, perrb, pcode := run(t, "run", "../../examples/toy",
		"--profile", profile, "--seed", seed, "--steps", steps)
	if pcode != 0 {
		t.Fatalf("without a browser: exit %d\nstdout:\n%s\nstderr:\n%s", pcode, plain, perrb)
	}
	plainTaken, plainRead := observationCounts(t, plain)
	if plainRead != 0 {
		t.Errorf("%d observation(s) were read from a page on a run that asked for no browser",
			plainRead)
	}
	if plainTaken != taken {
		t.Errorf("the walk took %d observation(s) with a browser and %d without: the ORACLE is "+
			"what the flag changes, and a walk that moved with it would make the two runs "+
			"different worlds rather than two readings of one", taken, plainTaken)
	}
}

// TestADirectObservationIsNeverReadThroughTheBrowser is the other half of
// deliverable 3 as fable wrote it: "the adapter's own read of the served HTML
// stays `direct`, the engine's second oracle, refused to an actor by name".
//
// THE DEFAULT PROFILE CLAIMS NO BROWSER OBSERVATION, so every reading it takes
// is `api` or `direct` -- and this run is given a real browser. If `direct` were
// read through the page the count would be non-zero, so zero is the assertion
// and the observation total is its presence partner: a run that observed nothing
// would also report zero and prove nothing.
//
// IT IS HERE AND NOT IN internal/schedule BECAUSE THE ADAPTER BRANCH NEEDS A
// LIVE SESSION. The in-package arms drive the routing with a nil session, which
// a non-browser source would panic on rather than answer -- so the claim is held
// where a real adapter runs, which is what I owed after retracting it.
func TestADirectObservationIsNeverReadThroughTheBrowser(t *testing.T) {
	out, errb, code := run(t, "run", "../../examples/toy", "--seed", "2", "--steps", "6",
		"--browser-check")
	skipWithoutAPair(t, out, errb)
	// A FINDING IS THIS TOOL'S PRODUCT, so a non-zero exit is not a failure
	// here: seed 2 is a promoted row and reports the stale count, which
	// docs/cli.md's table calls exit 3 -- "a bug was found". 0 and 3 are both
	// runs that happened; 1 and 2 are a refusal and a could-not-run, and
	// neither would have an observation line to read.
	if code != 0 && code != 3 {
		t.Fatalf("exit %d, want 0 (clean) or 3 (a bug found)\nstdout:\n%s\nstderr:\n%s",
			code, out, errb)
	}
	taken, read := observationCounts(t, out)
	if taken < 1 {
		t.Fatalf("the walk evaluated no observations, so zero read from the page says "+
			"nothing\n%s", out)
	}
	if read != 0 {
		t.Errorf("%d of %d observation(s) were read from a page on a profile that claims no "+
			"browser observation: an `api` or `direct` reading went through the browser, and "+
			"`direct` is the engine's view and nobody's page", read, taken)
	}
}
