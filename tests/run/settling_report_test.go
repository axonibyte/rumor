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
	// AND THE SENTENCE THIS WORLD ACTUALLY REACHES IS PINNED HERE, because I
	// mislabeled it in public: r8/126's table offered this world as "due and
	// did not run", and ibis measured it at r8/132 as `no pass held work to
	// re-ask` -- the clause above it in the switch. The counts DO show the
	// shortfall, which is what this arm is about and what it still asserts;
	// they do not select the sentence. Pinning it on the arm whose world I got
	// wrong is what keeps the correction, since the table is a post and a post
	// enforces nothing.
	if !strings.Contains(line, "no pass held work to re-ask") {
		t.Errorf("this world's sentence moved; r8/132 measured it as the held-nothing "+
			"clause and the shortfall is in the counts alone: %q", line)
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
	return toyWithPeriodOn(t, "api", every)
}

// toyWithPeriodOn is the same copy with the period on a NAMED stage, which is
// what lets report_test.go build the one case the walk cannot have: a period
// declared where nothing reads it.
func toyWithPeriodOn(t *testing.T, onStage string, every int) string {
	t.Helper()
	anchor := onStage + " = { enabled = true }"
	return toyCopy(t, func(t *testing.T, s string) string {
		t.Helper()
		if strings.Count(s, anchor) != 1 {
			t.Fatalf("examples/toy does not declare %q exactly once, so this copy would "+
				"carry no period and every arm would pass vacuously", anchor)
		}
		s = strings.Replace(s, anchor,
			onStage+" = { enabled = true, reconcile_every = "+strconv.Itoa(every)+" }", 1)
		if !strings.Contains(s, "reconcile_every = "+strconv.Itoa(every)) {
			t.Fatal("the period did not land in the copy")
		}
		return s
	})
}

// toyCopy is that copy with ONE edit to platform.toml, extracted because a
// second child needed the same project with a different line changed and the
// alternative was a second transcription of the file list -- where a copy that
// quietly stopped carrying `cases/` would read as a refusal about roles.
//
// THE EDIT CHECKS ITS OWN ANCHOR, which is why it is a function and not a
// pair of strings: an edit that silently does not apply is this child's most
// expensive repeated mistake (three R8 mutation controls compared a tree
// against an unmodified copy of itself and read the agreement as a result), so
// each caller proves its own line landed.
func toyCopy(t *testing.T, editPlatform func(*testing.T, string) string) string {
	t.Helper()
	dir := t.TempDir()
	for _, f := range []string{"platform.toml", "bindings.toml", "statute.rego", "seeds.lock"} {
		b, err := os.ReadFile(filepath.Join("../../examples/toy", f))
		if err != nil {
			t.Fatalf("reading %s: %v", f, err)
		}
		s := string(b)
		if f == "platform.toml" {
			s = editPlatform(t, s)
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

// THE TWO SENTENCES NO ARM REACHED, found by ibis clearing ad8b227 (coop
// r8/132) against the world table I posted at r8/126.
//
// MY TABLE'S THIRD ROW WAS WRONG AND THE ARM BUILT ON IT WAS NOT. I offered
// seed 1234 at period 2 as "due and did not run", and it prints `due at 3, ran
// 2: no pass held work to re-ask` -- the SAME clause as the row above it. The
// counts do show a shortfall, which is what
// TestADuePassThatDidNotRunIsCountedSeparately asserts and still correctly
// asserts; what they do not do is select the sentence. `every pass was skipped`
// needs due > 0 AND ran == 0, and no world I had named reached it.
//
// SO FIVE SENTENCES HAD THREE ARMS. The gap is the shape of this child's own
// defect one level up: I counted the STATES I had enumerated rather than the
// branches the code can take, which is a denominator taken from my notes
// instead of from the subject.
func TestEverySettlingSentenceHasAWorld(t *testing.T) {
	// NEVER CAME DUE: a period longer than the walk. due 0 and ran 0, which is
	// the one state whose two zeros are not a shortfall.
	out, _, code := run(t, "run", toyWithPeriod(t, 99), "--seed", "42",
		"--steps", "20", "--profile", "settling")
	if code != 0 && code != 3 {
		t.Fatalf("the never-due world exited %d, want 0 or a finding:\n%s", code, out)
	}
	line := settlingLine(t, out)
	if !strings.Contains(line, "the period never came due in this walk") {
		t.Errorf("a period of 99 over 20 steps does not reach the never-due sentence: %q", line)
	}
	if !strings.Contains(line, "due at 0, ran 0") {
		t.Errorf("the never-due sentence does not carry its two zeros: %q", line)
	}

	// EVERY PASS SKIPPED, CAUSE ONE: a stopped walk. ibis's world -- the walk
	// finds its bug ON the step the only due pass sat at.
	sout, _, scode := run(t, "run", toyWithPeriod(t, 6), "--seed", "1234",
		"--steps", "40", "--profile", "settling")
	if scode != 0 && scode != 3 {
		t.Fatalf("the stopped-walk world exited %d, want 0 or a finding:\n%s", scode, sout)
	}
	sline := settlingLine(t, sout)
	if !strings.Contains(sline, "every pass was skipped") {
		t.Errorf("a walk stopping on its only due step does not reach the skipped sentence: %q",
			sline)
	}
	if !strings.Contains(sline, "due at 1, ran 0") {
		t.Errorf("the skipped sentence does not carry the shortfall it is about: %q", sline)
	}
	// AND THE DISJUNCTION IS RESOLVABLE BY THE READER, which is the reason the
	// line itself does not resolve it: the finding is reported above.
	if !strings.Contains(sout, "FOUND") {
		t.Errorf("the skipped sentence names a stopped walk or a missing probe and the run "+
			"shows neither, so a reader cannot tell which:\n%s", sout)
	}
}

// EVERY PASS SKIPPED, CAUSE TWO: a project with NO PROBE, which is the other
// half of the sentence's disjunction and a different world entirely.
//
// IT IS THE walkable FIXTURE AND NOT THE TOY, because the toy declares two
// probes and cannot be made to have none without removing the thing the other
// arms need. walkable declares no `[[quiesce.probe]]` at all -- run_test.go's
// control arm says so in those words -- so the walk's periodic guard never
// passes, however many steps come due.
//
// WITHOUT THIS ARM the sentence would be covered only by the stopped-walk
// cause, and a build that skipped passes for the WRONG reason would read as
// correct: the words would be right and the world they describe would not be
// the one that produced them.
func TestAProjectWithNoProbeSkipsEveryDuePass(t *testing.T) {
	out, errb, code := fixtureWithPeriod(t, 2)
	if code != 0 {
		t.Fatalf("walkable with a period exited %d, want 0\nstdout:\n%s\nstderr:\n%s",
			code, out, errb)
	}
	line := settlingLine(t, out)
	if !strings.Contains(line, "every pass was skipped") {
		t.Errorf("a project with no probe does not reach the skipped sentence: %q", line)
	}
	// THE PERIOD CAME DUE, which is what makes this the skipped sentence rather
	// than the never-due one -- five steps at a period of 2.
	if !strings.Contains(line, "ran 0") {
		t.Errorf("a project with no probe ran a pass: %q", line)
	}
	if strings.Contains(line, "due at 0") {
		t.Errorf("no step came due, so this world is the never-due case and not this one: %q",
			line)
	}
	// AND NO FINDING, which separates the two causes: the stopped-walk arm
	// above requires one, and this world must reach the same sentence without.
	if strings.Contains(out, "FOUND") {
		t.Errorf("walkable found a bug, so this world no longer isolates the missing probe "+
			"as the cause:\n%s", out)
	}
}

// fixtureWithPeriod copies fixtures/walkable with a period on its api stage and
// runs it, returning what the run printed.
func fixtureWithPeriod(t *testing.T, every int) (string, string, int) {
	t.Helper()
	dir := t.TempDir()
	const anchor = "api = { enabled = true }"
	for _, f := range []string{"platform.toml", "bindings.toml", "statute.rego"} {
		b, err := os.ReadFile(filepath.Join(fixture, f))
		if err != nil {
			t.Fatalf("reading %s: %v", f, err)
		}
		s := string(b)
		if f == "platform.toml" {
			if strings.Count(s, anchor) != 1 {
				t.Fatalf("the walkable fixture does not declare %q exactly once, so this copy "+
					"would carry no period and the arm would pass vacuously", anchor)
			}
			s = strings.Replace(s, anchor,
				"api = { enabled = true, reconcile_every = "+strconv.Itoa(every)+" }", 1)
		}
		if err := os.WriteFile(filepath.Join(dir, f), []byte(s), 0o644); err != nil {
			t.Fatalf("writing %s: %v", f, err)
		}
	}
	return run(t, "run", dir, "--seed", "42", "--steps", "5")
}

// THE PRODUCT'S OWN EXAMPLE RECONCILES, from the COMMITTED project with no copy
// and no edit -- which is the whole reason fable gave the period a profile level
// at coop rumor/1619.
//
// EVERY OTHER ARM IN THIS FILE DECLARES THE PERIOD IN A TEMP COPY, and that is
// the gap this one closes: a knob exercised only by fixtures the tests build is
// a knob the product's own corpus does not use, which is the permission pattern
// R8/111 found twice (a field with no reader, a knob with no declaration). This
// arm would fail if `[profiles.reconciling]` were deleted from examples/toy.
//
// AND IT REACHES A VERDICT rather than merely running passes. Under the default
// mix the passes run and hold nothing, so `Settled` stays empty -- the profile's
// weights block exists to reach the held pending, justified line by line in
// platform.toml's own comment.
func TestTheToysOwnProfileReconcilesAndReachesAVerdict(t *testing.T) {
	const toy = "../../examples/toy"
	// SEED 101 AT 30 STEPS AND NOT 13 AT 40, chosen by cost: both reach a
	// periodic verdict under this profile, 101 at step 24 and 13 at step 32, so
	// the shorter walk is the cheaper witness to the same property. Measured on
	// this tree.
	out, errb, code := run(t, "run", toy, "--seed", "101", "--steps", "30",
		"--profile", "reconciling")
	if code != 0 && code != 3 {
		t.Fatalf("exited %d, want 0 or a finding\nstdout:\n%s\nstderr:\n%s", code, out, errb)
	}
	line := settlingLine(t, out)
	if !strings.Contains(line, "declared every 4 step(s)") {
		t.Errorf("the committed profile's period does not reach the walk, so platform.toml's "+
			"profile level is not read: %q", line)
	}
	if !strings.Contains(line, "bug of kind stale") {
		t.Errorf("the product's own example runs passes but reaches no verdict, so `Settled` "+
			"is empty in the one project that ships with the tool: %q", line)
	}

	// AND THE SAME SEED UNDER `settling` HAS NO PERIOD AT ALL, which is the
	// measurement the ruling turns on and the reason the level exists.
	//
	// seeds.lock's rows 12 and 66 NAME `settling`, and both were measured
	// without a period: row 12's stale is found by the END-OF-RUN
	// reconciliation and row 66 witnesses two pendings held and cleared. A
	// period on that profile -- or on the shared `[stages.api]` table, which
	// every profile reads -- would move the live bounds and so the draws, and
	// those two rows would need re-deriving rather than re-measuring. This arm
	// is what makes "declaring it moved nothing" a check instead of a claim.
	// FIVE STEPS, AND THE SHORT WALK IS NOT A WEAKENING: this control asserts
	// what `settling` DECLARES, which no number of steps can change. A 40-step
	// run of it shrinks a real finding and cost 50 seconds to prove a fact
	// about a TOML table.
	sout, _, scode := run(t, "run", toy, "--seed", "101", "--steps", "5",
		"--profile", "settling")
	if scode != 0 && scode != 3 {
		t.Fatalf("the settling control exited %d\n%s", scode, sout)
	}
	sline := settlingLine(t, sout)
	if !strings.Contains(sline, "no period is declared") {
		t.Errorf("`settling` carries a period, so seeds.lock's rows 12 and 66 are no longer "+
			"the worlds they were measured in: %q", sline)
	}
}
