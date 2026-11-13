package run_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// A SHRINK UNDER --browser-check REPLAYS THROUGH THE BROWSER (issue 124's
// deliverable 4). The reduction's lines read the same either way, so the
// witness is the driver's own count of sessions opened during the shrink: every
// candidate gets fresh sessions, so there are at least as many as candidates.
// Seed 40 reduces 16 steps to 9 in 13 candidates through the adapter, and does
// the same through the browser (measured, about 100s here) -- which is the
// claim that the browser reproduces what the adapter did, not a weaker one.
//
// THE PRESENCE PARTNER is the same seed with no browser: no such line, since a
// shrink through the adapter opens no session and must not say it did.
func TestAShrinkUnderTheBrowserReplaysThroughIt(t *testing.T) {
	out, errb := snapRunIn(t, "../../examples/toy", "40")
	reduced := regexp.MustCompile(`(?m)^shrink: REDUCED 16 step\(s\) to 9 -- (\d+) candidate\(s\)`).FindStringSubmatch(out)
	if reduced == nil {
		t.Fatalf("the browser shrink did not reduce 16 steps to 9 as the adapter's does:\n%s\nstderr:\n%s", out, errb)
	}
	candidates, _ := strconv.Atoi(reduced[1])
	m := regexp.MustCompile(`(?m)^shrink: replayed through the browser, (\d+) session\(s\) opened`).FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("a shrink under --browser-check does not say it replayed through the browser:\n%s", out)
	}
	if opened, _ := strconv.Atoi(m[1]); opened < candidates {
		t.Fatalf("%d session(s) opened over %d candidate(s): fresh sessions per candidate means at least one each", opened, candidates)
	}

	plain, errb, code := run(t, "run", "../../examples/toy", "--profile", snapProfile,
		"--seed", "40", "--steps", snapSteps)
	if code != 3 {
		t.Fatalf("without a browser: exit %d\n%s\n%s", code, plain, errb)
	}
	if strings.Contains(plain, "replayed through the browser") {
		t.Fatalf("a shrink with no browser says it replayed through one:\n%s", plain)
	}
}

// A SHRINK THAT REACHES ITS DECLARED COUNT STOPS AND SAYS HOW FAR IT GOT, from
// a project's own platform.toml (coop r9/204): the toy with its api stage
// allowed 3 candidates, where seed 2's shrink needs 8. Run twice, the stanza is
// byte-identical, because the budget is a count and not a clock (rook, r9/203).
func TestADeclaredShrinkBudgetStopsTheShrinkWhereItGot(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "toy")
	if err := os.CopyFS(dir, os.DirFS("../../examples/toy")); err != nil {
		t.Fatal(err)
	}
	pt := filepath.Join(dir, "platform.toml")
	raw, err := os.ReadFile(pt)
	if err != nil {
		t.Fatal(err)
	}
	const was = "api = { enabled = true }\n"
	if strings.Count(string(raw), was) != 1 {
		t.Fatalf("the toy's [stages] api line moved, so this arm cannot declare its budget")
	}
	if err := os.WriteFile(pt, []byte(strings.Replace(string(raw), was,
		"api = { enabled = true, shrink_candidates = 3 }\n", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	stanza := func() string {
		out, errb, code := run(t, "run", dir, "--profile", "walking", "--seed", "2")
		if code != 3 {
			t.Fatalf("exit %d, want the walking bug\n%s\n%s", code, out, errb)
		}
		var lines []string
		for _, l := range strings.Split(out, "\n") {
			if strings.HasPrefix(l, "shrink:") {
				lines = append(lines, l)
			}
		}
		return strings.Join(lines, "\n")
	}
	first, second := stanza(), stanza()
	if !strings.Contains(first, "shrink: STOPPED AT ITS BUDGET -- 3 of 3 candidate(s) tried") {
		t.Fatalf("a shrink allowed 3 candidates did not stop at them:\n%s", first)
	}
	if strings.Contains(first, "UNSTABLE") || strings.Contains(first, "not reproduc") {
		t.Fatalf("a stopped shrink reads as a failure to reproduce:\n%s", first)
	}
	if first != second {
		t.Fatalf("one seed stopped in two different places:\n--- first\n%s\n--- second\n%s", first, second)
	}
}

// THE BUDGET IS THE STAGE'S THAT DROVE THE WALK (coop r9/204): under
// --browser-check it is the `browser` stage's, and without, the `api` stage's.
// One project declares both -- browser 2, api left at the default -- and the
// same seed stops at 2 with a browser and runs its full shrink without one, so
// a build reading either stage's number for both would fail one half.
func TestTheBrowserStagesShrinkBudgetIsTheOneUnderABrowser(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "toy")
	if err := os.CopyFS(dir, os.DirFS("../../examples/toy")); err != nil {
		t.Fatal(err)
	}
	pt := filepath.Join(dir, "platform.toml")
	raw, err := os.ReadFile(pt)
	if err != nil {
		t.Fatal(err)
	}
	const was = "browser = { enabled = true }\n"
	if strings.Count(string(raw), was) != 1 {
		t.Fatalf("the toy's [stages] browser line moved, so this arm cannot declare its budget")
	}
	if err := os.WriteFile(pt, []byte(strings.Replace(string(raw), was,
		"browser = { enabled = true, shrink_candidates = 2 }\n", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"run", dir, "--profile", snapProfile, "--seed", "40", "--steps", snapSteps}
	withBrowser, errb, code := run(t, append(args, "--browser-check")...)
	skipWithoutAPair(t, withBrowser, errb)
	if code != 3 || !strings.Contains(withBrowser, "shrink: STOPPED AT ITS BUDGET -- 2 of 2 candidate(s) tried") {
		t.Fatalf("under a browser the shrink did not take the browser stage's 2 (exit %d):\n%s", code, withBrowser)
	}
	plain, perrb, pcode := run(t, args...)
	if pcode != 3 || !strings.Contains(plain, "shrink: REDUCED 16 step(s) to 9") {
		t.Fatalf("without a browser the shrink did not take the api stage's default (exit %d):\n%s\n%s", pcode, plain, perrb)
	}
}
