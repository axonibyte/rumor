package hygiene

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/proof"
)

// THE TIMELINE PUTS PER-PACKAGE TIMING IN THE JOB LOG, which had none: the
// runner emits a step's output in ONE FLUSH, so every line of task 1240's
// 1400-second suite carried the same wall-clock stamp and the diagnosis had
// to read elapsed figures alone. What it does NOT answer, from the shape CI
// feeds it, is what ran beside what -- see below.
//
// WHAT THIS ARM DRIVES IS THE SHAPE CI ACTUALLY FEEDS IT, which is the half I
// got wrong twice. `go tool test2json` invoked standalone has no `-p`, so it
// attributes NOTHING: every event arrives with a Time and no Package field.
// The first cut read Package only and printed "no package events" against the
// real stream -- a no-op that looked implemented.
//
// AND THE SECOND MISTAKE WAS WORSE, because it PUBLISHED A NUMBER. Having
// taught it to read the verdict lines, this arm then asserted a peak
// concurrency from them, and task 1245 duly reported "peak concurrency 30" on
// a box whose -p caps parallelism at GOMAXPROCS=8. `go test` emits package
// blocks in LISTED order, so a slow package holds back the print of
// everything behind it: all 46 packages in 1245 shared FOUR end times, and
// nineteen counted as running together had finished and were queued to print.
// quill refuted it at coop r9/418 and the figure is withdrawn at r9/419.
//
// SO THIS ARM NOW PINS THE REFUSAL. A print time cannot support an overlap
// claim, and the tool must say so rather than compute one -- an arm that
// asserted the figure is what let the artifact reach a job log.
func TestTheTimelineRefusesToClaimOverlapFromPrintTimes(t *testing.T) {
	py := pythonOrSkip(t)
	tool := filepath.Join("..", "..", "tools", "test-timeline.py")

	// THREE PACKAGES WHOSE PRINT TIMES WOULD TEMPT AN OVERLAP CLAIM, which is
	// the point: computed as end-minus-elapsed, alpha spans 0..10 and beta
	// 5..11, so a tool reading these as real times answers "peak 2". They are
	// PRINT times, and nothing here says the two ever ran together -- go may
	// have finished beta long before and held its block behind alpha's.
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

	for _, want := range []string{"3 package(s)", "alpha", "beta", "gamma"} {
		if !strings.Contains(got, want) {
			t.Errorf("the timeline does not report %q:\n%s", want, got)
		}
	}
	// THE REFUSAL IS THE CLAIM HERE. These rows are print times, so any
	// concurrency figure drawn from them is an artifact of go's output order.
	if strings.Contains(got, "peak concurrency") {
		t.Errorf("the timeline claims a concurrency figure from PRINT times, which is "+
			"the artifact that reached task 1245's job log as \"peak concurrency 30\" "+
			"on an 8-way box:\n%s", got)
	}
	if !strings.Contains(got, "PRINT times") {
		t.Errorf("the timeline does not warn that these are print times, so a reader "+
			"cannot tell this shape from one that carries real times:\n%s", got)
	}
	// THE VERDICT TRAVELS WITH THE ROW, because a timeline that showed timing
	// and lost which package FAILED would send a reader back to the log it was
	// meant to replace.
	if !strings.Contains(got, "fail") {
		t.Errorf("gamma FAILED and no row says so:\n%s", got)
	}
	if !t.Failed() {
		proof.Logf(t, "PROOF print-time stream: 3 package(s) and verdicts reported, NO concurrency claimed")
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
