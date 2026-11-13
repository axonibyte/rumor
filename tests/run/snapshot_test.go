package run_test

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// THE TOY'S PAGE AND STORE PART, AND THE PAGE IS KEPT (issue 124's deliverable
// 2, coop r9/191 and r9/197). Measured: `viewing` at seed 40 over 20 steps ends
// in a `bug` at step 16, where guest_1's page reads "no such event" while the
// engine's `direct` read of the same claim holds it -- a browser reading and a
// direct reading of ONE claim disagreeing, which is what a snapshot is for.
// Seed 29 is the control: a `bug` at step 19 on a step that read through the
// browser where page and store AGREE, which is the statute's finding and keeps
// no page.
const snapProfile, snapSteps = "viewing", "20"

var snapLine = regexp.MustCompile(`(?m)^run:   snapshot -- (.*)$`)

func snapRun(t *testing.T, seed string, extra ...string) (string, string) {
	t.Helper()
	return snapRunIn(t, toyWithAGateOnlyBrowserShrink(t), seed, extra...)
}

// snapRunIn is snapRun against a named project. The snapshot arms take the
// gate-only toy (cheapshrink_test.go), since what they assert is the page and
// not the shrink, so THE SNAPSHOT ARMS DO NOT PROVE THE BROWSER SHRINK. The
// browser-shrink arm takes the toy as shipped.
func snapRunIn(t *testing.T, project, seed string, extra ...string) (string, string) {
	t.Helper()
	args := append([]string{"run", project, "--profile", snapProfile,
		"--seed", seed, "--steps", snapSteps, "--browser-check"}, extra...)
	out, errb, code := run(t, args...)
	skipWithoutAPair(t, out, errb)
	if code != 3 {
		t.Fatalf("seed %s: exit %d, want 3 (the toy's bug)\nstdout:\n%s\nstderr:\n%s", seed, code, out, errb)
	}
	return out, errb
}

// TWO DIRECTORIES, ONE STANZA (rook, coop r9/196): the files are named relative
// to --snapshots, so the same seed written to two places prints byte-identical
// stdout. An absolute path or an echo of the directory would break the replay
// hash the first time someone ran from somewhere else. And the page kept is
// the one that DISAGREED: guest_1's "no such event", reopened at the reading's
// own page rather than whichever page the step's last observation left open.
func TestAPageThatDisagreesWithTheStoreIsKeptTheSameWayInAnyDirectory(t *testing.T) {
	dirA, dirB := t.TempDir(), t.TempDir()
	outA, _ := snapRun(t, "40", "--snapshots", dirA)
	outB, _ := snapRun(t, "40", "--snapshots", dirB)
	if outA != outB {
		t.Fatalf("one seed into two directories printed different stdout:\n--- A\n%s\n--- B\n%s", outA, outB)
	}
	m := snapLine.FindStringSubmatch(outA)
	if m == nil {
		t.Fatalf("a page/store disagreement printed no snapshot line:\n%s", outA)
	}
	if strings.Contains(m[1], dirA) || strings.Contains(m[1], "/") {
		t.Fatalf("the snapshot line names a path, not a file relative to --snapshots: %q", m[1])
	}
	name := strings.SplitN(m[1], ";", 2)[0]
	if name != "40-16-guest_1.html" {
		t.Fatalf("the snapshot is %q, want seed-step-actor 40-16-guest_1.html", name)
	}
	for _, dir := range []string{dirA, dirB} {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("the snapshot the line names is not in %s: %v", dir, err)
		}
		if !bytes.Contains(raw, []byte("no such event")) {
			t.Fatalf("the kept page is not the one that disagreed:\n%s", raw)
		}
	}
}

// A STATUTE FINDING ON A PAGE THAT AGREES WITH THE STORE KEEPS NOTHING (fable,
// coop r9/197): seed 29 read through the browser at its finding step, and page
// and store agreed there, so a snapshot would be a snapshot of a clean page.
// Its presence partner is seed 40 above, on the same flags.
func TestAFindingWherePageAndStoreAgreeKeepsNoPage(t *testing.T) {
	dir := t.TempDir()
	out, _ := snapRun(t, "29", "--snapshots", dir)
	if m := snapLine.FindStringSubmatch(out); m != nil {
		t.Fatalf("a finding where page and store agree kept a page: %q\n%s", m[1], out)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("%d file(s) written for a finding with no disagreement", len(entries))
	}
}

// NO --snapshots, AND THE FINDING SAYS SO rather than staying silent.
func TestWithoutSnapshotsTheFindingSaysThePageWasNotKept(t *testing.T) {
	out, _ := snapRun(t, "40")
	m := snapLine.FindStringSubmatch(out)
	if m == nil || m[1] != "snapshot not kept: no --snapshots" {
		t.Fatalf("snapshot line %v, want it to say the page was not kept:\n%s", m, out)
	}
}

// ASKED FOR A SCREENSHOT, THE TOY'S DISAGREEING PAGE GETS ONE: it carries no
// token, so the check finds nothing, a PNG is written, and its note names what
// the check cannot see -- as does the stderr line, said to whoever asked. The
// withholding rows are held against real pages in internal/browser, since
// none of the toy's disagreeing pages carries the token (measured).
func TestATokenFreeDisagreeingPageGetsItsScreenshotWithTheResidueNamed(t *testing.T) {
	dir := t.TempDir()
	out, errb := snapRun(t, "40", "--snapshots", dir, "--snapshot-screenshots")
	m := snapLine.FindStringSubmatch(out)
	if m == nil || !strings.Contains(m[1], "40-16-guest_1.png") || !strings.Contains(m[1], "canvas and image pixels") {
		t.Fatalf("snapshot line %v, want the PNG kept with the residue named:\n%s", m, out)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "40-16-guest_1.png"))
	if err != nil || !bytes.HasPrefix(raw, []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatalf("no PNG written: %v", err)
	}
	if !strings.Contains(errb, "canvas and image pixels") {
		t.Fatalf("asked for screenshots, the run did not say what the check cannot see:\n%s", errb)
	}
}
