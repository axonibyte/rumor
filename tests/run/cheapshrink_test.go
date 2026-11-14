package run_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// toyWithAGateOnlyBrowserShrink is examples/toy with the `browser` stage's
// shrink_candidates set to 1, so a browser run that finds something runs its
// shrink's replay gate and stops there.
//
// A NARROWING, AND THIS IS ITS REASON (coop r9/221). Since issue 124's
// deliverable 4 a browser run that finds something shrinks THROUGH the browser,
// and a browser candidate costs a walk plus a session per actor -- measured,
// seed 40's 13 candidates take about 100s where the adapter took 18. tests/run
// has six such runs whose SUBJECT is not the shrink: the snapshot arms, the
// flow walk and the direct-observation arm. Each would spend minutes shrinking
// a finding it never asserts on, and the first gate with all of them went past
// a 40-minute bound where it had taken 929s.
//
// WHAT IT TAKES OUT, EXACTLY: the shrink's REDUCTION in those runs, and nothing
// they assert. The walk, the finding, the snapshot and the page reads are
// untouched, because shrink_candidates is read by nothing but the shrink.
//
// SO THE BROWSER SHRINK IS PROVEN IN THESE THREE ARMS AND NOWHERE ELSE, listed
// so the set can be counted rather than inferred (ibis, coop r9/224). Each runs
// the toy as shipped, or a copy that changes only its own budget:
//
//  1. TestAShrinkUnderTheBrowserReplaysThroughIt -- the full default; the
//     reduction matches the adapter's and the driver's session count shows it
//     went through the browser.
//  2. TestTheBrowserStagesShrinkBudgetIsTheOneUnderABrowser -- the browser
//     stage's own declared budget is the one taken under --browser-check.
//  3. TestADeclaredShrinkBudgetStopsTheShrinkWhereItGot -- a declared count
//     stops the shrink where it got to, the same way twice (adapter path).
//
// A test that takes this helper DOES NOT PROVE THE BROWSER SHRINK, and says so
// at its own call (rook, coop r9/222).
func toyWithAGateOnlyBrowserShrink(t *testing.T) string {
	t.Helper()
	cheapToyOnce.Do(func() {
		// INSIDE TestMain's DIRECTORY, which every exit removes: a copy
		// made under the system temp dir would outlive the suite (issue 88).
		dir, err := os.MkdirTemp(suiteDir, "toy-gate-only-")
		if err != nil {
			cheapToyErr = err
			return
		}
		dir = filepath.Join(dir, "toy")
		if err := os.CopyFS(dir, os.DirFS("../../examples/toy")); err != nil {
			cheapToyErr = err
			return
		}
		pt := filepath.Join(dir, "platform.toml")
		raw, err := os.ReadFile(pt)
		if err != nil {
			cheapToyErr = err
			return
		}
		const was = "browser = { enabled = true }\n"
		if strings.Count(string(raw), was) != 1 {
			cheapToyErr = os.ErrNotExist
			return
		}
		cheapToyErr = os.WriteFile(pt, []byte(strings.Replace(string(raw), was,
			"browser = { enabled = true, shrink_candidates = 1 }\n", 1)), 0o600)
		cheapToy = dir
	})
	if cheapToyErr != nil {
		t.Fatalf("making the gate-only toy (the [stages] browser line may have moved): %v", cheapToyErr)
	}
	return cheapToy
}

var (
	cheapToyOnce sync.Once
	cheapToy     string
	cheapToyErr  error
)
