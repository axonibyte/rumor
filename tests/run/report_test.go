// 111's REPORT, AS RULED: A VERB THAT ASKS, NOT A STREAM THAT TELLS.
package run_test

import (
	"strings"
	"testing"
)

// `rumor report` IS WHERE A READER SEES THE STAGE FACTS, and this file replaces
// arms that measured the wrong channel.
//
// THE FIRST HEAD PRINTED TWO UNCONDITIONAL STDERR LINES. It was right that
// stdout is the subject of every recorded digest, and wrong that stderr was
// therefore free: STDERR's EMPTINESS is itself load-bearing -- it is what lets a
// control read a build failure as a build failure, and rook measured it as part
// of D7 (coop rumor/1480). fable reversed the shape at rumor/1597, and the
// reason was already written on schedule.Record.Stages before this child began.
//
// SO THE ARM THAT MATTERS IS AN ABSENCE: a run nobody asked prints nothing about
// stages on EITHER stream. Everything else here is about the verb.
func TestRunSaysNothingAboutStagesOnEitherStreamUnlessAsked(t *testing.T) {
	out, errb, code := run(t, "run", "../../examples/toy", "--seed", "2", "--steps", "10")
	if code != 3 {
		t.Fatalf("the toy exited %d, want 3 (its planted defects): a run that did something "+
			"else is not the subject\nstdout:\n%s\nstderr:\n%s", code, out, errb)
	}
	// BOTH STREAMS, and by the bare word rather than a whole line, so a
	// reworded report cannot reach either of them unnoticed.
	if strings.Contains(out, "stages") {
		t.Errorf("stdout carries a stage report, which is the subject of every recorded "+
			"digest: five frozen digests, seeds.lock and the docs' figures all move\n%s", out)
	}
	if strings.Contains(errb, "stages") {
		t.Errorf("stderr carries a stage report on an unasked run, which retires the reading "+
			"that lets a control tell a build failure from noise (rook, D7)\n%s", errb)
	}
}

// AND THE VERB PRINTS THEM, with the skip named beside its reason.
func TestTheReportVerbNamesTheSelectionAndEverySkip(t *testing.T) {
	out, errb, code := run(t, "report", "../../examples/toy")
	if code != 0 {
		t.Fatalf("report exited %d, want 0\nstdout:\n%s\nstderr:\n%s", code, out, errb)
	}
	for _, want := range []string{
		"report: stages selected -- api, concurrency, fuzz",
		"report: stages skipped -- browser (enabled and this build cannot run it yet)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the report does not carry %q:\n%s", want, out)
		}
	}
	// NOTHING ELSE, which is the ruling's own word: a verb that also walked
	// would hand a reader two sets of findings to tell apart.
	for _, unwanted := range []string{"run: seed", "fuzz:", "concurrency:", "FOUND"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("the report printed %q, so it is doing more than reporting:\n%s",
				unwanted, out)
		}
	}
	// AND ITS STDERR IS EMPTY, for the same reason the unasked run's must be.
	if errb != "" {
		t.Errorf("report wrote %d byte(s) to stderr:\n%s", len(errb), errb)
	}
}

// `none` IS SAID RATHER THAN OMITTED, and the shipped toy reaches it under a
// profile -- no fixture needed, because `walking` selects api alone and asks for
// nothing it cannot run.
//
// THE EMPTY CASE IS THE ONE THAT MATTERS: a skip list omitted when there is
// nothing to say cannot be told from a build that never learned to report one,
// which is the shape that let `fuzz` ship known, unrunnable and declared by
// nobody through 108's gate.
func TestTheReportSaysNoneRatherThanNothing(t *testing.T) {
	out, errb, code := run(t, "report", "../../examples/toy", "--profile", "walking")
	if code != 0 {
		t.Fatalf("report --profile walking exited %d\nstdout:\n%s\nstderr:\n%s", code, out, errb)
	}
	if !strings.Contains(out, "report: stages selected -- api") {
		t.Errorf("the walking profile selects api alone and the report does not say so:\n%s", out)
	}
	if !strings.Contains(out, "report: stages skipped -- none") {
		t.Errorf("a profile asking for nothing it cannot run reported its skips as something "+
			"other than `none`:\n%s", out)
	}
}

// THE TWO FLAGS A READER WILL TRY ARE REFUSED BY NAME, because this verb makes
// no walk: a seed it accepted silently would promise a determinism it never
// exercised.
func TestTheReportRefusesWalkFlagsByName(t *testing.T) {
	for _, flag := range []string{"--seed", "--steps"} {
		out, errb, code := run(t, "report", "../../examples/toy", flag, "3")
		if code != 2 {
			t.Errorf("%s exited %d, want 2 (a usage error)\nstdout:\n%s\nstderr:\n%s",
				flag, code, out, errb)
			continue
		}
		if !strings.Contains(errb, "is not a report flag") {
			t.Errorf("%s was refused without saying why:\n%s", flag, errb)
		}
	}
	// AND A PROFILE NOBODY DECLARES IS A REFUSAL AND NOT A USAGE ERROR: the
	// project is wrong, which is exit 1 here, and the distinction is the one
	// docs/vocabulary.md draws between refused and could-not-run.
	out, errb, code := run(t, "report", "../../examples/toy", "--profile", "nonesuch")
	if code != 1 {
		t.Errorf("an undeclared profile exited %d, want 1\nstdout:\n%s\nstderr:\n%s",
			code, out, errb)
	}
}

// AND EACH SELECTED STAGE'S DECLARED FACTS, which is fable's third clause at
// coop rumor/1614 -- the answer to r8/121's question about where a settling
// declaration belongs when the verb makes no run.
//
// THE SET OF FACT LINES IS THE SELECTION EXACTLY, asserted both ways. A line
// for a stage that was SKIPPED would be the report describing something no run
// will do, and a selected stage with no line would be the omission the skip
// list's own `none` rule exists to refuse.
func TestTheReportNamesEachSelectedStagesDeclaredFacts(t *testing.T) {
	out, errb, code := run(t, "report", "../../examples/toy")
	if code != 0 {
		t.Fatalf("report exited %d, want 0\nstdout:\n%s\nstderr:\n%s", code, out, errb)
	}
	for _, want := range []string{
		"report: stage api --",
		"report: stage concurrency --",
		"report: stage fuzz --",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the report carries no facts line for %q:\n%s", want, out)
		}
	}
	// browser IS SKIPPED ON THIS BUILD, and the skip line above already says
	// so -- so a facts line for it would describe a stage that will not run.
	if strings.Contains(out, "report: stage browser --") {
		t.Errorf("the report describes a SKIPPED stage's facts:\n%s", out)
	}
	// AND THE COUNT MATCHES, so a fourth line nobody named cannot slip in
	// behind three that were asserted present.
	if n := strings.Count(out, "report: stage "); n != 3 {
		t.Errorf("want one facts line per selected stage, 3 selected, got %d:\n%s", n, out)
	}
	// THE api STAGE'S FACT IS ITS PERIOD, and the committed toy declares none.
	if !strings.Contains(out, "report: stage api -- reconciles only after the last step") {
		t.Errorf("the toy declares no period and the api line does not say so:\n%s", out)
	}
	if errb != "" {
		t.Errorf("report wrote %d byte(s) to stderr:\n%s", len(errb), errb)
	}
}

// THE DECLARED PERIOD IS READ FROM platform.toml AND PRINTED, which is the half
// of R8/111's defect that no test could have caught before: `reconcile_every`
// was declared by NO project in the tree, so the path from the file to any
// reader had never run once.
func TestTheReportPrintsTheDeclaredPeriod(t *testing.T) {
	out, errb, code := run(t, "report", toyWithPeriod(t, 4))
	if code != 0 {
		t.Fatalf("report exited %d, want 0\nstdout:\n%s\nstderr:\n%s", code, out, errb)
	}
	if !strings.Contains(out, "report: stage api -- reconciles every 4 step(s)") {
		t.Errorf("the declared period does not reach the report:\n%s", out)
	}
	// AND THE UNDECLARED SENTENCE IS GONE, so the two states are two sentences
	// rather than one that happens to mention a number.
	if strings.Contains(out, "reconciles only after the last step") {
		t.Errorf("a declared period still reads as undeclared:\n%s", out)
	}
}

// A PERIOD ON A STAGE THAT DOES NOT WALK IS NAMED AS INERT, and this is the arm
// the line exists for.
//
// `[stages.concurrency].reconcile_every` PARSES, is accepted by the platform,
// passes every validate arm, and changes nothing about any run -- the walk reads
// the period from the api stage's table and no other (cmd/rumor/run.go says so
// where it reads it). A report that printed it the same way for every stage
// would be the thing that CONFIRMED the mistake to whoever made it, which is
// why the wording is asserted and not merely the number.
func TestTheReportNamesAPeriodOnANonWalkingStageAsInert(t *testing.T) {
	out, errb, code := run(t, "report", toyWithPeriodOn(t, "concurrency", 4))
	if code != 0 {
		t.Fatalf("report exited %d, want 0\nstdout:\n%s\nstderr:\n%s", code, out, errb)
	}
	line := ""
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "report: stage concurrency --") {
			line = l
		}
	}
	if line == "" {
		t.Fatalf("no facts line for concurrency:\n%s", out)
	}
	if !strings.Contains(line, "only the api stage reads") {
		t.Errorf("the report does not say the period is unread where it is declared: %q", line)
	}
	if !strings.Contains(line, "no run reconciles on it") {
		t.Errorf("the report names the period without saying it does nothing: %q", line)
	}
	// AND THE api STAGE IS UNAFFECTED: this project declares no period where
	// the walk reads one, so the api line must still say so. Without this the
	// arm could pass on a build that read the period from any stage's table.
	if !strings.Contains(out, "report: stage api -- reconciles only after the last step") {
		t.Errorf("a period on concurrency changed what the api stage reports:\n%s", out)
	}
}
