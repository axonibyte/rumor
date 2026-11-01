// R8/111: THE PERIODIC RECONCILIATION'S VERDICT, READ FROM THE PRODUCT.
package run_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// THIS ARM EXISTS BECAUSE SIX PASSING ARMS DID NOT CATCH THE DEFECT, and the
// difference between them and this one is the only thing it is about.
//
// rook drove D11 at coop r8/116 and found `rec.Settled` read ZERO times in
// cmd/rumor: every production reference sat inside internal/schedule, and the
// run's quiesce line joined `rec.Reconciled` alone. fable ruled at rumor/1610
// that a field no product path emits is a PERMISSION and not an occurrence.
// tests/quiesce/settling_test.go asserts on the FIELD and passes either way --
// so the suite made the field look reached while nothing read it. A test that
// would have caught this has to read the STREAM, which is why this arm lives in
// tests/run and drives the binary.
//
// AND `reconcile_every` WAS DECLARED BY NO PROJECT IN THE TREE. Searched as a
// word rather than a sentence (rook's method, coop rumor/1606): the only
// setters were settling_test.go, which assigns the Go field directly, and
// platform_test.go, which parses it -- so platform.toml -> Stages[API] ->
// Project.ReconcileEvery -> the walk had never run end to end in any
// invocation. The field having no reader and the knob having no declaration are
// one defect from two sides, and a fixture that declares it is half the fix.
//
// THE PROFILE IS `settling`, WHICH IS A NARROWING WHOSE REASON COVERS EXACTLY
// IT: the periodic reconciliation re-asks what the walk HOLDS, only an
// `eventual` claim is ever held, and `settling` is the profile examples/toy
// declares for `event.tallied` -- the one eventual type with a `quiesce` bound.
// It also runs the api stage alone, which is the stage the walk IS.
//
// THE PERIOD IS DECLARED IN A COPY AND NEVER IN examples/toy ITSELF, because
// the contract's own entry says absence "is why no existing measurement moves"
// and determinism is per (seed, period): a period in the committed toy would
// move every seed figure in this suite, D10's included.
func TestTheSettlingVerdictReachesTheProductsOutput(t *testing.T) {
	// SEED 23 AT 40 STEPS IS THE WORLD, and it was chosen by measuring eight
	// seeds on this tree rather than by argument. It is the CHEAPEST seed that
	// holds a pending at a step a period can reach: the walk holds
	// member_2/event_tally at step 21 and stops at 22.
	//
	// THE FIRST SWEEP FOUND NOTHING AND THE BOUND WAS WHY. At 20 steps, eight
	// seeds and 33 passes between them held no work at all -- the pending is
	// not held until step 21 -- so a 20-step walk ends before one exists. That
	// is also the shape of rook's null at r8/116: 8 seeds, 20 steps, no verdict
	// reached, and no denominator printed to say the passes had run empty.
	const (
		seed  = "23"
		steps = "40"
	)

	// THE CONTROL IS THE SAME SEED WITH NO PERIOD, which is what makes this a
	// measurement about the period rather than about the world. The pending is
	// still held and still judged -- by the END-OF-RUN reconciliation, whose
	// line says "re-checked after the last step" -- so if the periodic verdict
	// below came from anywhere but the period, this arm would carry it too.
	cout, cerrb, ccode := run(t, "run", "../../examples/toy", "--seed", seed,
		"--steps", steps, "--profile", "settling")
	if ccode != 0 && ccode != 3 {
		t.Fatalf("the control exited %d, want 0 or a finding\nstdout:\n%s\nstderr:\n%s",
			ccode, cout, cerrb)
	}
	cline := settlingLine(t, cout)
	if !strings.Contains(cline, "no period is declared") {
		t.Errorf("the committed toy declares no period and the line does not say so: %q", cline)
	}
	// AND IT NAMES NO COUNT, because a control printing `ran 0` would be
	// indistinguishable from a declared period that never ran -- the exact
	// conflation the counts exist to break.
	if strings.Contains(cline, "ran ") || strings.Contains(cline, "due at") {
		t.Errorf("the undeclared case reports counts, so it reads as a period that ran "+
			"zero times: %q", cline)
	}
	// THE CONTROL IS NON-VACUOUS: the same pending IS reached, by the other
	// path. Without this the arm below could pass on a world where nothing was
	// ever held and the period had nothing to find either way.
	if !strings.Contains(cout, "re-checked after the last step") {
		t.Fatalf("the control holds no pending judged after the last step, so this seed no "+
			"longer exercises the end-of-run path and the arm below proves nothing:\n%s", cout)
	}

	// THE DECLARED CASE: the same seed, a period of 2, and the verdict the
	// periodic pass reached printed in the line.
	dir := toyWithPeriod(t, 2)
	out, errb, code := run(t, "run", dir, "--seed", seed, "--steps", steps,
		"--profile", "settling")
	if code != 0 && code != 3 {
		t.Fatalf("period 2 exited %d, want 0 or a finding\nstdout:\n%s\nstderr:\n%s",
			code, out, errb)
	}
	line := settlingLine(t, out)
	if strings.Contains(line, "no period is declared") {
		t.Fatalf("the period reads as undeclared, so platform.toml never reached the walk "+
			"-- which is the end-to-end path no project in the tree had ever driven: %q", line)
	}
	// THE VERDICT ITSELF, WHICH IS THE WHOLE POINT. `step N: <verdict>` is the
	// stamping internal/schedule documents on Record.Settled, and until this
	// line existed no invocation of this tool could print it.
	if !strings.Contains(line, "step ") || !strings.Contains(line, "bug of kind stale") {
		t.Errorf("the periodic pass reached a verdict and the line does not carry it: %q", line)
	}
	// AND THE DENOMINATOR BESIDE IT. rook's null could not say whether a pass
	// had run at all, so the counts are asserted present even on the run that
	// has a verdict to show.
	if !strings.Contains(line, "due at") || !strings.Contains(line, "ran ") {
		t.Errorf("the line carries a verdict without the counts it was reached over: %q", line)
	}
	// THE PERIODIC PATH IS THE ONE THAT JUDGED IT, read from the pending's own
	// life: with a period the re-check lands ON a step, and the control's
	// "after the last step" wording is gone.
	if strings.Contains(out, "re-checked after the last step") {
		t.Errorf("the pending was still judged after the last step, so the period changed "+
			"nothing about when it was re-asked:\n%s", out)
	}

	// STDOUT AND NOT STDERR, which is the stream claim this arm can make.
	//
	// IT WAS WRITTEN AS "stderr is empty on a clean run" AND THAT IS FALSE OF
	// THIS SUBJECT, found by driving it: the toy's concurrency stage reports
	// each try's overlap in microseconds on stderr, so an unprofiled run has
	// stderr lines that are nothing to do with this child. The emptiness
	// reading in cmd/rumor/report.go is about the REPORT verb, which runs no
	// stage and so has an emptiness to protect; a run that walks does not.
	// What still holds is the placement: stdout is the stream every recorded
	// digest is taken over, so a fact a reader is meant to see goes there.
	if strings.Contains(errb, "settling") {
		t.Errorf("the settling verdict reached stderr, where no digest reads it:\n%s", errb)
	}
}

// AND THE STATE WHERE A DUE PASS DID NOT RUN, which is the one the counts were
// added for: due and ran are separate numbers so that their DIFFERENCE is
// readable, and a single bool could not say it.
//
// SEED 1234 IS THE WORLD AND IT WAS MEASURED, not reasoned: period 2 comes due
// three times and runs twice, because the walk stops on a finding at the step
// the third was due at. That is the same collision the first drive of this
// child hit on seed 42 -- a finding at step 4 with a period of 4 -- so it is
// the ordinary case and not a contrivance.
func TestADuePassThatDidNotRunIsCountedSeparately(t *testing.T) {
	dir := toyWithPeriod(t, 2)
	out, errb, code := run(t, "run", dir, "--seed", "1234", "--steps", "40",
		"--profile", "settling")
	if code != 0 && code != 3 {
		t.Fatalf("exited %d, want 0 or a finding\nstdout:\n%s\nstderr:\n%s", code, out, errb)
	}
	line := settlingLine(t, out)
	due, ran := countIn(t, line, "due at "), countIn(t, line, "ran ")
	if due <= ran {
		t.Fatalf("this seed was chosen because a due pass does not run; due %d, ran %d: %q\n"+
			"the arm is vacuous on a world where every due pass ran", due, ran, line)
	}
	// THE LINE SAYS WHY THERE IS A SHORTFALL rather than leaving the reader to
	// subtract two numbers and guess.
	if !strings.Contains(out, "FOUND") {
		t.Errorf("a pass was skipped and the run reports no finding, so the shortfall has "+
			"neither of the two causes the walk's guard allows:\n%s", out)
	}
}

// countIn reads the integer right after `after` in the line, and FAILS rather
// than returning zero when it is absent -- a helper that returned 0 would make
// a missing count read as a measured zero, which is this child's own defect
// turned on its test.
func countIn(t *testing.T, line, after string) int {
	t.Helper()
	i := strings.Index(line, after)
	if i < 0 {
		t.Fatalf("the line carries no %q: %q", after, line)
	}
	rest := line[i+len(after):]
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	if end == 0 {
		t.Fatalf("no number follows %q: %q", after, line)
	}
	n, err := strconv.Atoi(rest[:end])
	if err != nil {
		t.Fatalf("parsing the count after %q: %v", after, err)
	}
	return n
}

// settlingLine returns the one `run:   settling --` line, and FAILS when there
// is not exactly one. A helper returning "" for absent would turn a missing
// line into a Contains() mismatch whose message blames the text.
func settlingLine(t *testing.T, out string) string {
	t.Helper()
	var found []string
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "run:   settling --") {
			found = append(found, strings.TrimSpace(l))
		}
	}
	if len(found) != 1 {
		t.Fatalf("want exactly one settling line, got %d:\n%s", len(found), out)
	}
	return found[0]
}

// toyWithPeriod copies examples/toy into a temp dir and declares the period on
// the `api` stage, which is the only table the walk reads it from.
//
// THE ANCHOR IS CHECKED AND THE FAILURE IS LOUD, because an edit that silently
// does not apply is this child's most expensive repeated mistake: three
// mutation controls in R8 compared a tree against an unmodified copy of itself
// and read the agreement as a result.
func toyWithPeriod(t *testing.T, every int) string {
	t.Helper()
	dir := t.TempDir()
	const anchor = "api = { enabled = true }"
	for _, f := range []string{"platform.toml", "bindings.toml", "statute.rego", "seeds.lock"} {
		b, err := os.ReadFile(filepath.Join("../../examples/toy", f))
		if err != nil {
			t.Fatalf("reading %s: %v", f, err)
		}
		s := string(b)
		if f == "platform.toml" {
			if strings.Count(s, anchor) != 1 {
				t.Fatalf("examples/toy does not declare %q exactly once, so this copy would "+
					"carry no period and every arm would pass vacuously", anchor)
			}
			s = strings.Replace(s, anchor,
				"api = { enabled = true, reconcile_every = "+strconv.Itoa(every)+" }", 1)
			if !strings.Contains(s, "reconcile_every = "+strconv.Itoa(every)) {
				t.Fatal("the period did not land in the copy")
			}
		}
		if err := os.WriteFile(filepath.Join(dir, f), []byte(s), 0o644); err != nil {
			t.Fatalf("writing %s: %v", f, err)
		}
	}
	// THE CASES COME TOO. tests/contract refuses a declared role no case
	// carries, and a copy missing them is a different project from the one the
	// control runs -- which would put the world back among the variables.
	ents, err := os.ReadDir("../../examples/toy/cases")
	if err != nil {
		t.Fatalf("reading the toy's cases: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "cases"), 0o755); err != nil {
		t.Fatalf("making the cases dir: %v", err)
	}
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		b, rerr := os.ReadFile(filepath.Join("../../examples/toy/cases", e.Name()))
		if rerr != nil {
			t.Fatalf("reading case %s: %v", e.Name(), rerr)
		}
		if werr := os.WriteFile(filepath.Join(dir, "cases", e.Name()), b, 0o644); werr != nil {
			t.Fatalf("writing case %s: %v", e.Name(), werr)
		}
	}
	return dir
}
