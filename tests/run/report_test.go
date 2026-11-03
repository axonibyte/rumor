// 111's REPORT, AS RULED: A VERB THAT ASKS, NOT A STREAM THAT TELLS.
package run_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

// AND THE REPORT NAMES THE LEVEL THE PERIOD CAME FROM, which is fable's clause
// at coop rumor/1619: "4" and "where 4 came from" are different questions, and
// only the second tells a reader whether THEIR declaration is the one in force.
//
// DRIVEN ON THE COMMITTED TOY AT BOTH LEVELS, which is what makes it a test of
// the PRECEDENCE: the same project answers differently under a profile that
// declares a period, so an identical pair of lines would mean the profile level
// was not read while both halves still looked asserted.
func TestTheReportNamesTheLevelThePeriodCameFrom(t *testing.T) {
	const toy = "../../examples/toy"

	// THE PROFILE LEVEL, from the committed `reconciling` profile.
	out, errb, code := run(t, "report", toy, "--profile", "reconciling")
	if code != 0 {
		t.Fatalf("report exited %d\nstdout:\n%s\nstderr:\n%s", code, out, errb)
	}
	if !strings.Contains(out, "reconciles every 4 step(s)") {
		t.Errorf("the profile's period does not reach the report:\n%s", out)
	}
	// THE TABLE IS NAMED IN THE PROJECT'S OWN WORDS, a line a reader can go and
	// edit, rather than the bare level word.
	if !strings.Contains(out, "from [profiles.reconciling].reconcile_every") {
		t.Errorf("the report gives the period without the table it came from:\n%s", out)
	}

	// AND WITH NO PROFILE THE SAME PROJECT REPORTS NO PERIOD AT ANY LEVEL, so
	// the profile's declaration does not leak into an unprofiled read.
	uout, _, ucode := run(t, "report", toy)
	if ucode != 0 {
		t.Fatalf("the unprofiled report exited %d:\n%s", ucode, uout)
	}
	if !strings.Contains(uout, "no period at any level") {
		t.Errorf("an unprofiled read of the toy reports a period, so the profile level leaks "+
			"into every run:\n%s", uout)
	}
	// THE PAIR MUST DIFFER. Identical output would satisfy both halves above on
	// a build that ignored the profile entirely, because the first assertion
	// would then be reading a stage-level period that does not exist.
	if out == uout {
		t.Errorf("the profiled and unprofiled reports are byte-identical, so the level is "+
			"not read:\n%s", out)
	}
}

// AND WHERE A RUN OF THE SELECTED PROFILE STARTS ITS CLOCK, which is R7.5's
// fourth level and the first whose OTHER half this verb cannot read (fable,
// coop r7/111).
//
// THE RULING THIS ARM GUARDS IS A SUBTRACTION. The line was first asked for as
// an effective start with its level, like the period's above; fable struck
// that, because both of the period's levels are the platform's while this
// one's other level is `[clock].start` in bindings.toml -- and this command
// opens platform.toml ALONE, so that a report of what a profile DECLARES
// still answers on a project whose bindings do not parse. So each sentence
// names the level it cannot see, and the last arm here drives the reason:
// broken bindings, and the line still comes.
//
// ALL THREE SENTENCES ARE DRIVEN. An arm on the declared case alone would pass
// on a build that resolved a start from bindings and printed it -- the exact
// thing the ruling removed.
func TestTheReportNamesWhereAProfilesClockStarts(t *testing.T) {
	const toy = "../../examples/toy"
	const unread = "[clock].start, which this command does not read"

	// NO PROFILE SELECTED. The epoch is named as the fallback and `[clock]`
	// as the level that could supply one instead -- not as what a run will do,
	// which this verb has not opened the file to find out.
	uout, uerrb, ucode := run(t, "report", toy)
	if ucode != 0 {
		t.Fatalf("report exited %d\nstdout:\n%s\nstderr:\n%s", ucode, uout, uerrb)
	}
	uline := clockStartLine(t, uout)
	if !strings.Contains(uline, "no profile selected") {
		t.Errorf("an unprofiled read does not say no profile is selected: %q", uline)
	}

	// A PROFILE THAT DECLARES NONE IS A DIFFERENT SENTENCE, so a reader who
	// selected one is answered about THEIR table rather than about selection.
	pout, _, pcode := run(t, "report", toy, "--profile", "walking")
	if pcode != 0 {
		t.Fatalf("report --profile walking exited %d:\n%s", pcode, pout)
	}
	pline := clockStartLine(t, pout)
	if !strings.Contains(pline, "[profiles.walking] declares none") {
		t.Errorf("a profile declaring no start is not reported in its own words: %q", pline)
	}
	if pline == uline {
		t.Errorf("selecting a profile changed nothing in the line, so the profile is not "+
			"read: %q", pline)
	}

	// AND THE DECLARED CASE, ON A COPY DECLARING TWO, which is ibis's finding
	// at coop rumor/1773 and the reason this is not one profile.
	//
	// ONE WOULD NOT SEE THE DEFECT. With a start declared in `walking` alone,
	// "the start walking declares" and "the only start in the file" are the
	// same number, so a reader that took ANY profile's nonzero start answered
	// correctly here -- and the mutant that made `clockStartFacts` do exactly
	// that SURVIVED. The second profile's value is what the arm below asserts
	// is NOT printed.
	const start int64 = 1772000000000000000
	const other int64 = 1793000000000000000
	dir := toyWithClockStarts(t, map[string]int64{"walking": start, "settling": other})
	dout, _, dcode := run(t, "report", dir, "--profile", "walking")
	if dcode != 0 {
		t.Fatalf("report on the declared copy exited %d:\n%s", dcode, dout)
	}
	dline := clockStartLine(t, dout)
	if !strings.Contains(dline, "[profiles.walking].clock_start declares "+
		strconv.FormatInt(start, 10)+"ns") {
		t.Errorf("the declared start does not reach the report with its value: %q", dline)
	}
	if !strings.Contains(dline, "outranks "+unread) {
		t.Errorf("the declared start is printed without saying what it beats, which is the "+
			"whole of a reader's question: %q", dline)
	}
	// TWO STATES, TWO SENTENCES: the undeclared wording is gone rather than
	// standing beside a number.
	if strings.Contains(dline, "declares none") {
		t.Errorf("a declared start still reads as undeclared: %q", dline)
	}
	// AND NOT THE OTHER PROFILE'S, which is the half that kills the survivor:
	// `[profiles.settling]` declares a start too, and a line that reported it
	// under `--profile walking` would be answering "is SOME line in force"
	// where a reader asked about their own.
	if strings.Contains(dline, strconv.FormatInt(other, 10)) {
		t.Errorf("the report names [profiles.settling]'s start under --profile walking, so "+
			"the line answers about any declaration rather than the selected one: %q", dline)
	}
	// AND THE SAME PROJECT ANSWERS THE OTHER PROFILE WITH ITS OWN, which makes
	// this a measurement of the SELECTION rather than of one string: identical
	// lines for two profiles would satisfy the halves above on a build that
	// read neither.
	oout, _, ocode := run(t, "report", dir, "--profile", "settling")
	if ocode != 0 {
		t.Fatalf("report --profile settling exited %d:\n%s", ocode, oout)
	}
	oline := clockStartLine(t, oout)
	if !strings.Contains(oline, strconv.FormatInt(other, 10)) {
		t.Errorf("[profiles.settling] declares %d and its own report does not name it: %q",
			other, oline)
	}
	if oline == dline {
		t.Errorf("two profiles with two different starts report the same line, so the "+
			"selection is not read: %q", oline)
	}

	// EVERY SENTENCE NAMES THE LEVEL IT CANNOT SEE. This is the clause fable
	// kept when the effective value was struck.
	for _, l := range []string{uline, pline, dline} {
		if !strings.Contains(l, unread) {
			t.Errorf("the line does not name the level this command cannot read: %q", l)
		}
	}

	// AND THE REASON IT CANNOT: bindings.toml is not opened here. A project
	// whose bindings are unparseable still reports what its platform declares,
	// which is the second oracle on the subtraction above -- a build that
	// resolved the start would have to read this file, and would fail.
	//
	// THE FIXTURE IS REFUSED BY THE PROJECT'S OWN PARSER, measured rather than
	// assumed: these same bytes make `rumor validate` exit 2 with "cannot be
	// read as a declaration: not TOML: toml: line 1", 0 of 24 arms evaluated.
	// Without that the arm would be vacuous in the one way it cannot afford --
	// a bindings.toml that happened to PARSE would be read by a resolving
	// build without complaint, and this would still pass.
	broken := toyWithClockStart(t, "walking", start)
	if err := os.WriteFile(filepath.Join(broken, "bindings.toml"),
		[]byte("this is not toml = = [\n"), 0o644); err != nil {
		t.Fatalf("breaking the copy's bindings: %v", err)
	}
	bout, berrb, bcode := run(t, "report", broken, "--profile", "walking")
	if bcode != 0 {
		t.Fatalf("a report of what a profile declares failed on unparseable bindings, exit "+
			"%d -- which is the failure the single-file read exists to prevent\nstdout:\n%s"+
			"\nstderr:\n%s", bcode, bout, berrb)
	}
	if got := clockStartLine(t, bout); got != dline {
		t.Errorf("breaking bindings.toml changed the clock start line, so this verb reads "+
			"it after all:\n with: %q\nbroken: %q", dline, got)
	}
}

// clockStartLine is the one clock start line, and it FATALS when the count is
// not exactly one -- for settlingLine's reason: a helper answering "" for
// absent turns a missing line into a mismatch whose message blames the words.
func clockStartLine(t *testing.T, out string) string {
	t.Helper()
	var found []string
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "report: clock start --") {
			found = append(found, strings.TrimSpace(l))
		}
	}
	if len(found) != 1 {
		t.Fatalf("want exactly one clock start line, got %d:\n%s", len(found), out)
	}
	return found[0]
}

// toyWithClockStart copies examples/toy and declares `clock_start` in a NAMED
// profile's table, checking its own anchor landed.
//
// THE KEY GOES DIRECTLY UNDER THE HEADER, which is what puts it in that
// profile's table and not in the one after it -- and the copy is then driven
// through the real verb, so a key that landed in the wrong table would be read
// by platform.toml's own parser rather than by this helper's hope.
func toyWithClockStart(t *testing.T, profile string, start int64) string {
	t.Helper()
	return toyWithClockStarts(t, map[string]int64{profile: start})
}

// toyWithClockStarts declares `clock_start` in EVERY named profile, each with
// its own value.
//
// ONE PROFILE COULD NOT SEE THE DEFECT, which is ibis's finding at coop
// rumor/1773 against ba5c720. With a start declared in a single profile and
// nowhere else, "the start the NAMED profile declares" and "the only start in
// the file" are the same number -- so a reader that took ANY profile's nonzero
// start answered correctly on every input this fixture could build, and the
// mutant that made it do exactly that SURVIVED. Two profiles with two values
// make the selection observable, which is what ibis asked for in those words:
// "two profiles, two values, assert the named one's and not the other's".
//
// THE SIBLING WAS ALREADY ARMED and that is the tell: the identical mutant on
// `observationFacts` dies, because its own fixture carries two profiles that
// claim different observations. The gap was this fixture and not the pattern.
func toyWithClockStarts(t *testing.T, starts map[string]int64) string {
	t.Helper()
	// THE VALUES MUST DIFFER, or two profiles are one number again and this
	// helper's whole reason is gone.
	seen := map[int64]string{}
	for profile, start := range starts {
		if start == 0 {
			t.Fatalf("[profiles.%s] would declare a start of 0, which is the ABSENT case and "+
				"not a value this fixture can select on", profile)
		}
		if other, dup := seen[start]; dup {
			t.Fatalf("[profiles.%s] and [profiles.%s] would declare the same start %d, so the "+
				"copy cannot tell a reader of the NAMED profile from a reader of any profile",
				profile, other, start)
		}
		seen[start] = profile
	}
	return toyCopy(t, func(t *testing.T, s string) string {
		t.Helper()
		for profile, start := range starts {
			s = declareClockStart(t, s, profile, start)
		}
		return s
	})
}

// declareClockStart puts one `clock_start` under one profile's header.
func declareClockStart(t *testing.T, s, profile string, start int64) string {
	t.Helper()
	header := "[profiles." + profile + "]\n"
	decl := "clock_start = " + strconv.FormatInt(start, 10) + "\n"
	if strings.Count(s, header) != 1 {
		t.Fatalf("examples/toy does not declare %q exactly once, so this copy would "+
			"carry no start and every arm would pass vacuously", header)
	}
	// THE GUARD ASKS ABOUT THE TARGET PROFILE'S OWN TABLE, not the file. It
	// asked about the file until R7.5 D4 declared `clock_start` on the two
	// `dst-*` profiles, and then fatal'd on a project exactly as valid as
	// before: the state this copy must build is "the NAMED profile declares
	// none", and a declaration elsewhere is somebody else's.
	if strings.Contains(tableAfter(s, header), "clock_start") {
		t.Fatalf("[profiles.%s] already declares a clock_start, so this copy no longer "+
			"builds the state it was written for", profile)
	}
	s = strings.Replace(s, header, header+decl, 1)
	if !strings.Contains(s, header+decl) {
		t.Fatal("the clock start did not land in the copy")
	}
	return s
}

// AND THE DOCUMENTED REPORTS ARE THE BINARY'S REPORTS, which is the shape
// TestTheDocumentedRunIsTheBinarysRun and tests/validate's two blocks already
// hold their samples to -- and the one cli.md's `rumor report` blocks did not
// have.
//
// FOUND BY EDITING THEM. This child adds a line to both blocks, and nothing in
// the tree would have noticed if it had reached only one of them, or carried a
// typo, or gone stale the next time a level is added -- "a documented sample
// that cannot fail is decoration" (docs/testing-methodology.md), and these two
// could not. D4 edits the same blocks again.
//
// THE WHOLE BLOCK, NOT THE NEW LINE. An arm asserting only the clock start
// line would leave every other line of the sample exactly as unchecked as it
// was, and the gap would be the same size with one more test in the count.
//
// RUN FROM THE REPOSITORY ROOT, so the path in the sample is the path the
// binary is given: `rumor report examples/toy` as printed, rather than a
// package-relative spelling that happens to produce the same output.
func TestTheDocumentedReportsAreTheBinarysReports(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "cli.md"))
	if err != nil {
		t.Fatalf("reading docs/cli.md: %v", err)
	}
	for _, c := range []struct {
		marker string
		args   []string
	}{
		{"$ rumor report examples/toy\n", []string{"report", "examples/toy"}},
		{"$ rumor report examples/toy --profile reconciling\n",
			[]string{"report", "examples/toy", "--profile", "reconciling"}},
	} {
		// EXACTLY ONE, counted rather than found: Index takes the first of
		// several silently, and a second copy of a block is how a sample gets
		// edited in one place and read in the other. The unprofiled marker
		// ends in a newline, so it cannot match the profiled line's prefix.
		if n := strings.Count(string(raw), c.marker); n != 1 {
			t.Fatalf("docs/cli.md holds %d copies of %q, want 1: either the block moved or "+
				"this arm is checking a block nobody reads", n, c.marker)
		}
		i := strings.Index(string(raw), c.marker)
		rest := string(raw)[i+len(c.marker):]
		end := strings.Index(rest, "```")
		if end < 0 {
			t.Fatalf("the console block after %q is unterminated", c.marker)
		}
		want := strings.TrimRight(rest[:end], "\n")
		// AND IT IS NOT EMPTY. A fence that moved to immediately after the
		// command would leave want == "" and compare nothing, which is the
		// fence-extraction trap R8.4 paid for: a loose fence splits a block
		// and both halves still parse.
		if want == "" {
			t.Fatalf("the block after %q holds no output lines", c.marker)
		}

		cmd := exec.Command(rumorBin, c.args...)
		cmd.Dir = filepath.Join("..", "..")
		out, rerr := cmd.Output()
		if rerr != nil {
			t.Fatalf("running the documented command %v: %v", c.args, rerr)
		}
		if got := strings.TrimRight(string(out), "\n"); got != want {
			t.Errorf("docs/cli.md claims:\n%s\n\nthe binary prints:\n%s", want, got)
		}
	}
}

// tableAfter is one TOML table's body: everything from `header` to the next
// table header at column 0.
//
// IT EXISTS SO A GUARD CAN BE SCOPED. A whole-file `strings.Contains` cannot
// tell "this profile declares a key" from "some profile does", and the
// difference is the premise of every copy helper in this file.
func tableAfter(doc, header string) string {
	i := strings.Index(doc, header)
	if i < 0 {
		return ""
	}
	rest := doc[i+len(header):]
	for j := 1; j < len(rest); j++ {
		if rest[j] == '[' && rest[j-1] == '\n' {
			return rest[:j]
		}
	}
	return rest
}
