// 111's REPORT: THE STAGE SELECTION AND EVERY SKIP, BY NAME.
package run_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// THE REPORT IS ON STDERR AND THE DIGESTS DO NOT MOVE, which is the whole of
// fable's ruling at coop rumor/1472 item 1: the walk's stdout is the subject of
// every recorded digest and of R7's measurement row, so a stage line there
// "moves thirty-five figures in five files for no finding".
//
// THE STDOUT HALF IS THE LOAD-BEARING ONE. A report that is correct and on the
// wrong stream is worse than none: it would pass every arm about its CONTENT
// while silently invalidating five frozen digests, seeds.lock and the docs'
// figures. So this asserts the absence as well as the presence.
//
// BOTH LINES ALWAYS PRINT, `none` INCLUDED. An omitted skip line cannot be told
// from a build that never learned to report one -- which is the shape that let
// `fuzz` ship known, unrunnable and declared by nobody straight through 108's
// gate, and the reason this child exists.
func TestTheRunReportsItsStageSelectionAndSkipsOnStderr(t *testing.T) {
	const toy = "../../examples/toy"

	out, errb, code := run(t, "run", toy, "--seed", "2", "--steps", "10")
	if code != 3 {
		t.Fatalf("the shipped toy exited %d, want 3 (its planted defects): a run that did "+
			"something else is not the subject\nstdout:\n%s\nstderr:\n%s", code, out, errb)
	}

	// PRESENT, ON STDERR, BOTH LINES.
	for _, want := range []string{
		"run: stages selected -- api, concurrency, fuzz",
		"run: stages skipped -- browser (enabled and this build cannot run it yet)",
	} {
		if !strings.Contains(errb, want) {
			t.Errorf("stderr does not carry %q:\n%s", want, errb)
		}
	}

	// AND ABSENT FROM STDOUT. Checked by the PREFIX rather than the whole line,
	// so a reworded report still cannot leak onto the hashed stream.
	if strings.Contains(out, "run: stages") {
		t.Errorf("the report reached STDOUT, which is the subject of every recorded digest: "+
			"five frozen digests, seeds.lock and the docs' figures all move\nstdout:\n%s", out)
	}

	// THE SKIP NAMES THE STATE rook's D13 PARTNER DISTINGUISHES: `browser` is
	// false in Known AND declared, so it is DECLARED-BUT-UNRUNNABLE and is
	// reported as a skip with its reason -- not as a stage nobody declared,
	// which is a different reading and 111's phase gate's to name.
	if strings.Contains(errb, "run: stages skipped -- none") {
		t.Errorf("the toy declares `browser`, which this build cannot run, so the skip list "+
			"must not be empty:\n%s", errb)
	}
}

// A PROJECT THAT SKIPS NOTHING SAYS SO, which is the other direction and the one
// that keeps the line above from being a fixed string that happens to be true.
func TestAProjectWithNothingSkippedReportsNoneRatherThanNothing(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"platform.toml", "bindings.toml", "statute.rego", "seeds.lock"} {
		b, err := os.ReadFile(filepath.Join("../../examples/toy", f))
		if err != nil {
			t.Fatalf("reading the toy's %s: %v", f, err)
		}
		body := string(b)
		if f == "platform.toml" {
			// ONLY `browser` IS DROPPED, and the reason is measured rather
			// than tidy: it is the one stage this build cannot run, so its
			// absence is what empties the skip list. Dropping `concurrency` or
			// `fuzz` as well is REFUSED before the report ever prints --
			// platform cross-checks that a profile names only declared stages,
			// and 110's `racing` profile names `concurrency`, so the run exits
			// 1 on "profiles.racing.stages: names stage concurrency, which is
			// not declared". That refusal is a different subject.
			//
			// AND NO PROFILE NAMES `browser`, which is what makes this edit
			// safe: walking, settling and expiring select api, racing selects
			// concurrency.
			const drop = `browser = { enabled = true }`
			if !strings.Contains(body, drop) {
				t.Fatalf("the toy no longer declares %q, so this cell would build a world it "+
					"did not mean to", drop)
			}
			body = strings.Replace(body, drop, "", 1)
		}
		if err := os.WriteFile(filepath.Join(dir, f), []byte(body), 0o644); err != nil {
			t.Fatalf("writing %s: %v", f, err)
		}
	}

	out, errb, code := run(t, "run", dir, "--seed", "2", "--steps", "10")
	if !strings.Contains(errb, "run: stages selected -- api") {
		t.Errorf("exit %d: the api-only project did not report api selected\nstderr:\n%s", code, errb)
	}
	if !strings.Contains(errb, "run: stages skipped -- none") {
		t.Errorf("a project asking for nothing it cannot run reported its skips as something "+
			"other than `none`, so the empty case is indistinguishable from an unimplemented "+
			"one\nstderr:\n%s", errb)
	}
	if strings.Contains(out, "run: stages") {
		t.Errorf("the report reached STDOUT:\n%s", out)
	}
}
