package hygiene

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/proof"
)

// THE TIMELINE IS THE ONLY THING THAT CAN ANSWER "WHAT ELSE WAS RUNNING", and
// the job log could not answer it at all before R10 d2. The runner emits a
// step's output in ONE FLUSH, so every line of task 1240's 1400-second suite
// carried the same wall-clock stamp: the diagnosis had to read per-package
// elapsed figures and GUESS at overlap, because the flush had erased it.
//
// WHAT THIS ARM DRIVES IS THE SHAPE CI ACTUALLY FEEDS IT, which is the half I
// got wrong and only found by running it. `go tool test2json` invoked
// standalone has no `-p`, so it attributes NOTHING: every event arrives with a
// Time and no Package field. The first cut of the tool read Package only, so
// against the real tee'd stream it printed "no package events" and the whole
// feature was a no-op that looked implemented. A fixture built from
// `go test -json` would have passed and hidden exactly that.
func TestTheTimelineRecoversOverlapFromAStampedStreamWithNoPackageFields(t *testing.T) {
	py := pythonOrSkip(t)
	tool := filepath.Join("..", "..", "tools", "test-timeline.py")

	// THREE PACKAGES WHOSE OVERLAP IS KNOWN BY CONSTRUCTION. go buffers a
	// package's output and flushes it WHEN THE PACKAGE ENDS, so the stamp on a
	// verdict line is that package's end and its start is end-minus-elapsed:
	//
	//	alpha  ends 10.0s having taken 10.0  -> runs  0.0 .. 10.0
	//	beta   ends 11.0s having taken  6.0  -> runs  5.0 .. 11.0
	//	gamma  ends 20.0s having taken  1.0  -> runs 19.0 .. 20.0
	//
	// alpha and beta therefore overlap and gamma touches neither: the peak is
	// TWO, and a tool that simply counted packages would say three.
	stream := strings.Join([]string{
		event("2026-11-16T00:00:10Z", "ok  \tforge.axonibyte.com/axonibyte/rumor/alpha\t10.000s\n"),
		event("2026-11-16T00:00:11Z", "ok  \tforge.axonibyte.com/axonibyte/rumor/beta\t6.000s\n"),
		event("2026-11-16T00:00:20Z", "FAIL\tforge.axonibyte.com/axonibyte/rumor/gamma\t1.000s\n"),
	}, "")

	path := filepath.Join(t.TempDir(), "test.json")
	if err := os.WriteFile(path, []byte(stream), 0o644); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}

	out, err := exec.Command(py, tool, "--json", path).CombinedOutput()
	if err != nil {
		t.Fatalf("running %s: %v\n%s", tool, err, out)
	}
	got := string(out)

	for _, want := range []string{
		"3 package(s)",
		"peak concurrency 2",
		"alpha",
		"beta",
		"gamma",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the timeline does not report %q:\n%s", want, got)
		}
	}
	// THE VERDICT TRAVELS WITH THE ROW, because a timeline that showed timing
	// and lost which package FAILED would send a reader back to the log it was
	// meant to replace.
	if !strings.Contains(got, "fail") {
		t.Errorf("gamma FAILED and no row says so:\n%s", got)
	}
	// AND THE PEAK NAMES ITS MEMBERS: "concurrency 2" without saying WHICH two
	// cannot support a contention argument about a particular package.
	if !strings.Contains(got, "peak at ") {
		t.Errorf("the peak does not name when it happened or who was in it:\n%s", got)
	}
	if !t.Failed() {
		proof.Logf(t, "PROOF timeline over a Package-less stamped stream: 3 package(s), peak 2, verdicts carried")
	}
}

// TestTheTimelineSurvivesAStreamThatIsNotAllJSON holds the case that turned a
// build failure into an empty report once before, in ci.yml's own record: the
// test step folds stderr into the stream, so a compile error arrives as plain
// text beside the JSON. A reporter that threw on it would take the run's one
// surviving artifact with it.
func TestTheTimelineSurvivesAStreamThatIsNotAllJSON(t *testing.T) {
	py := pythonOrSkip(t)
	tool := filepath.Join("..", "..", "tools", "test-timeline.py")

	stream := "# not json at all\n" +
		event("2026-11-16T00:00:10Z", "ok  \tforge.axonibyte.com/axonibyte/rumor/alpha\t2.000s\n") +
		"FAIL\tforge.axonibyte.com/axonibyte/rumor/beta [build failed]\n"

	path := filepath.Join(t.TempDir(), "test.json")
	if err := os.WriteFile(path, []byte(stream), 0o644); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}

	out, err := exec.Command(py, tool, "--json", path).CombinedOutput()
	if err != nil {
		t.Fatalf("a mixed stream must still report, and this exited %v:\n%s", err, out)
	}
	got := string(out)
	if !strings.Contains(got, "alpha") {
		t.Errorf("the JSON half was lost because the stream was mixed:\n%s", got)
	}
	if !strings.Contains(got, "non-JSON") {
		t.Errorf("the report does not say the stream was mixed, so a reader cannot "+
			"tell a partial timeline from a whole one:\n%s", got)
	}
	if !t.Failed() {
		proof.Logf(t, "PROOF a mixed JSON/plain stream still reports, and says it was mixed")
	}
}

// event spells one test2json record the way the tee'd stream carries it: a
// Time and an Output, and deliberately NO Package.
func event(when, output string) string {
	return `{"Time":"` + when + `","Action":"output","Output":` + quote(output) + "}\n"
}

func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// pythonOrSkip answers the interpreter the test step resolves the same way.
//
// A SKIP HERE IS ABOUT THE BOX AND NOT THE CLAIM, and it is honest only
// because CI's own jail was MEASURED to carry python3.12 before the test step
// was allowed to depend on it. If that stops being true the step prints its
// "no timeline" line and this arm skips together, which is the pair a reader
// should see rather than a green arm over a dead feature.
func pythonOrSkip(t *testing.T) string {
	t.Helper()
	for _, name := range []string{"python3.12", "python3"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	t.Skip("no python3 on this box: the test step prints its no-timeline line here too")
	return ""
}
