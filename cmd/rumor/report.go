package main

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"forge.axonibyte.com/axonibyte/rumor/internal/decl"
	"forge.axonibyte.com/axonibyte/rumor/internal/platform"
	"forge.axonibyte.com/axonibyte/rumor/internal/stage"
)

// reportProject is where a reader SEES the run's stage facts, and it exists in
// this shape because the first attempt got the channel wrong (fable, coop
// rumor/1597).
//
// THE FACTS ARE THE RECORD's AND NOT A STREAM's, and the reason was already
// written on schedule.Record.Stages before this child began: stdout is the
// subject of every recorded digest and of R7's measurement row, and STDERR's
// EMPTINESS is what lets a control read a build failure as a build failure --
// rook measured stderr as part of D7 at coop rumor/1480. The first head printed
// two UNCONDITIONAL stderr lines, which retires that reading; a run that is not
// asked now prints neither, and both streams are as v0.1.45 left them byte for
// byte.
//
// SO A READER ASKS. `rumor report PROJECT` prints the selection, every skip, and
// each selected stage's DECLARED FACTS -- and nothing else, which is what makes
// it safe to print on stdout: the digests are taken from `rumor run`, and this
// is a different invocation.
//
// THE THIRD CLAUSE ARRIVED AT R8/111 and the sentence said "and nothing else"
// without it for one head. fable ruled it at coop rumor/1614 on my question at
// r8/121: a stage's DECLARATION is decided by the platform and the profile
// alone, which is the same property that makes the first two clauses printable
// without a walk, so it belongs here -- while the settling VERDICT belongs to
// `rumor run`, which is the invocation that has a record.
//
// IT RUNS NOTHING, which is not a shortcut but the subject's own shape: the
// stages are chosen BEFORE anything runs (Select's comment and fable's ruling at
// rumor/1477), so selection and skips are decided by the platform and the
// profile alone. A report that ran the stages in order to describe them would
// produce a second set of findings a reader would then have to tell from the
// first.
func reportProject(dir, profile string, stdout, stderr io.Writer) int {
	prep, err := platform.Load(filepath.Join(dir, "platform.toml"), decl.TOML)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "report: could not run: reading platform.toml: %v\n", err)
		return 2
	}
	// THE REFUSALS GO WHERE run PUTS THEM, on stdout, because a refusal is this
	// tool's answer and not a diagnostic about its own execution -- the same
	// split `rumor run` keeps between a REFUSED line and a could-not-run.
	if !prep.OK() {
		for _, ref := range prep.Refusals {
			_, _ = fmt.Fprintf(stdout, "report: REFUSED %s\n", ref)
		}
		return 1
	}
	selected, skipped, serr := stage.Plan(prep.Platform, profile)
	if serr != nil {
		_, _ = fmt.Fprintf(stdout, "report: REFUSED %v\n", serr)
		return 1
	}
	// BOTH LINES ALWAYS, `none` INCLUDED. A skip list omitted when there is
	// nothing to say cannot be told from a build that never learned to report
	// one -- the shape that let `fuzz` ship known, unrunnable and declared by
	// nobody through 108's gate, and the reason this child exists.
	_, _ = fmt.Fprintf(stdout, "report: stages selected -- %s\n", nameList(selected))
	_, _ = fmt.Fprintf(stdout, "report: stages skipped -- %s\n", skipList(skipped))
	// AND EACH SELECTED STAGE'S DECLARED FACTS, which is fable's ruling at coop
	// rumor/1614 on the question at r8/121: the settling VERDICT belongs to
	// `rumor run`, which has a record, and the DECLARATION belongs here,
	// because a declaration is decided by the platform and the profile alone --
	// the same thing that makes the two lines above safe to print without a
	// walk.
	//
	// IT ANSWERS THE OTHER HALF OF THE DEFECT THIS CHILD FOUND. `rec.Settled`
	// had no reader and `reconcile_every` had no DECLARATION anywhere in the
	// tree, and the second was invisible because nothing ever printed what a
	// stage declares. A reader who runs this on a project that means to
	// reconcile periodically now sees at once whether it does.
	//
	// ONE LINE PER SELECTED STAGE, `nothing` INCLUDED, for the reason the skip
	// list gives above: a stage omitted when it has nothing to say cannot be
	// told from a build that never learned to report that stage at all.
	for _, name := range selected {
		_, _ = fmt.Fprintf(stdout, "report: stage %s -- %s\n", name,
			stageFacts(name, prep.Platform, profile))
	}
	// AND THE POOL THIS PROFILE DRAWS, which is fable's clause at coop
	// rumor/1660 and the same reading the period's line gives: a reader asking
	// which actors their profile runs is usually asking whether their OWN rows
	// are in force, and the platform's `[[venire]]` alone cannot say.
	//
	// ALWAYS, AND `appends none` INCLUDED, for the reason the two lines above
	// give: a line omitted when there is nothing to say cannot be told from a
	// build that never learned to print it.
	_, _ = fmt.Fprintf(stdout, "report: venire -- %s\n", venireFacts(prep.Platform, profile))
	// AND WHAT THIS PROFILE CLAIMS TO OBSERVE, which is fable's third line at
	// coop rumor/1678: "three declarations, one level, one resolver, one report
	// line each". The reader's question is the venire line's exactly -- whether
	// their own claims are in force -- and it has a second half the pool's does
	// not, because a claim takes a name OUT of every other profile's set.
	//
	// FROM THE PLATFORM ALONE, like every line above it. This command reads one
	// file on purpose: a declaration is decided by the platform and the profile
	// together, and loading bindings.toml here would make a report of what a
	// profile DECLARES fail on a project whose bindings do not parse. So this
	// line names the claims and not the resolved set -- the resolved set is a
	// fact about two documents, and `rumor run`'s observation count is where a
	// reader sees it.
	_, _ = fmt.Fprintf(stdout, "report: observations -- %s\n",
		observationFacts(prep.Platform, profile))
	// AND WHERE A RUN OF THIS PROFILE STARTS ITS CLOCK (R7.5; fable's ruling at
	// coop r7/111). The fourth level, and the first whose OTHER half this
	// command cannot read -- which is why it names what it can see rather than
	// resolving.
	_, _ = fmt.Fprintf(stdout, "report: clock start -- %s\n",
		clockStartFacts(prep.Platform, profile))
	return 0
}

// clockStartFacts is where a run of one profile starts its clock, in the
// project's own words and WITHOUT RESOLVING -- which is the difference between
// this line and the period's, and the whole of fable's correction at coop
// r7/111.
//
// THE PERIOD CAN BE RESOLVED HERE AND THIS CANNOT. Both of the period's levels
// are the platform's, so `ReconcileEvery` answers an effective value from the
// one file this command reads. The clock start's other level is
// `[clock].start` in bindings.toml, and this command reads platform.toml ALONE
// on purpose -- the reason is the comment above the observations line: loading
// bindings here would make a report of what a profile DECLARES fail on a
// project whose bindings do not parse. So an "effective start" is not
// available to this verb, and claiming one would be a sentence about a file
// that was never opened.
//
// SO IT NAMES THE LEVEL IT CANNOT SEE, in observationFacts's voice: the reader
// asking "is MY line in force" is answered for the profile's own line, and
// told exactly which other declaration could outrank nothing or supply the
// start instead. The effective instant with its level belongs where the run
// resolves it -- the record line at the first seal, beside the `set` it issues
// (fable, r7/111).
//
// ZERO IS ABSENT, as it is at every level of this one: a profile declaring
// `clock_start = 0` has declared nothing, and this line says so in the same
// words as a profile that omitted the key.
func clockStartFacts(p platform.Platform, profile string) string {
	const other = "[clock].start, which this command does not read"
	if profile == "" {
		return "no profile selected, so a run starts at " + other + ", or the epoch"
	}
	pr, ok := p.Profiles[profile]
	if !ok || pr.ClockStart == 0 {
		return fmt.Sprintf("[profiles.%s] declares none, so a run of it starts at %s, or the "+
			"epoch", profile, other)
	}
	return fmt.Sprintf("[profiles.%s].clock_start declares %dns, which outranks %s",
		profile, pr.ClockStart, other)
}

// observationFacts is what one profile claims to observe, in the project's own
// words.
//
// IT NAMES THE CLAIMS AND THE CONSEQUENCE, which is where it differs from
// venireFacts and is the point of printing it at all. An appended venire row
// can only add actors to the profile that declares it; a CLAIM also removes the
// name from every other profile, so a reader of this line needs both halves --
// and the profile that reads "claims none" is told what that means rather than
// left to infer it from an absence.
func observationFacts(p platform.Platform, profile string) string {
	if profile == "" {
		return "no profile selected, so a run takes the observations no profile claims"
	}
	pr, ok := p.Profiles[profile]
	if !ok || len(pr.Observations) == 0 {
		return fmt.Sprintf("[profiles.%s] claims none, so a run of it takes the observations "+
			"no profile claims", profile)
	}
	return fmt.Sprintf("[profiles.%s].observations claims %s, which this profile takes in "+
		"addition to the observations no profile claims -- and which no other profile takes",
		profile, strings.Join(pr.Observations, ", "))
}

// venireFacts is the actor pool one profile draws, in the project's own words.
//
// IT NAMES THE APPENDED ROWS SEPARATELY rather than printing the resolved pool
// as one list, and that is the whole reason this helper exists: a resolved list
// cannot be told from a platform that declared those rows at the top level,
// where the entire safety argument for the level is that the COMMITTED rows are
// untouched. A reader has to be able to see which rows are the profile's.
func venireFacts(p platform.Platform, profile string) string {
	committed := memberList(p.Venire)
	if profile == "" {
		return committed + " -- from [[venire]], with no profile selected"
	}
	pr, ok := p.Profiles[profile]
	if !ok || len(pr.Venire) == 0 {
		return fmt.Sprintf("%s -- from [[venire]]; [profiles.%s] appends none", committed, profile)
	}
	return fmt.Sprintf("%s -- from [[venire]]; [profiles.%s].venire appends %s",
		committed, profile, memberList(pr.Venire))
}

// memberList renders venire rows in the order the file declares them, because
// the draw reads them in that order and a report that sorted them would answer
// a question about a pool nobody declared.
func memberList(ms []platform.Member) string {
	if len(ms) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(ms))
	for _, m := range ms {
		parts = append(parts, fmt.Sprintf("%d %s", m.Count, m.Role))
	}
	return strings.Join(parts, ", ")
}

// parseReportArgs takes PROJECT and `--profile NAME` and REFUSES everything
// else by name.
//
// `--seed` AND `--steps` ARE REFUSED SPECIFICALLY rather than swept into the
// unknown-flag message, because they are the two a reader will try: this verb
// runs no walk, so a seed it silently accepted would promise a determinism it
// never exercised.
func parseReportArgs(args []string) (dir, profile, bad string) {
	rest := args
	for len(rest) > 0 {
		switch {
		case rest[0] == "--profile":
			if len(rest) < 2 {
				return "", "", "--profile wants a value"
			}
			profile = rest[1]
			if profile == "" {
				return "", "", "--profile wants a name"
			}
			rest = rest[2:]
		case rest[0] == "--seed" || rest[0] == "--steps":
			return "", "", fmt.Sprintf("%s is not a report flag: a report takes no walk, so a "+
				"seed or a step count would name a run this verb does not make", rest[0])
		case len(rest[0]) > 2 && rest[0][:2] == "--":
			return "", "", fmt.Sprintf("unknown flag %s", rest[0])
		default:
			if dir != "" {
				return "", "", "report takes one PROJECT"
			}
			dir = rest[0]
			rest = rest[1:]
		}
	}
	if dir == "" {
		return "", "", "report wants a PROJECT"
	}
	return dir, profile, ""
}

// stageFacts is what one selected stage DECLARES, in its own words.
//
// THE PERIOD IS THE api STAGE'S AND NO OTHER'S, which is not a formatting
// choice but the subject's shape: the walk IS the api stage, so a period under
// another stage's table is a period for a stage that does not walk.
//
// THIS COMMENT SAID "cmd/rumor/run.go reads `Stages[stage.API].ReconcileEvery`"
// AND THAT WAS TRUE FOR ONE HEAD. The profile level (fable, coop rumor/1619)
// moved the read into platform.Platform.ReconcileEvery, the one place the
// precedence is stated, so a sentence citing run.go's old expression now points
// at code that is not there. A claim about a sibling file is re-read on the
// tree being committed, not inherited from the head it was written on.
//
// SO A PERIOD DECLARED ELSEWHERE IS NAMED AS INERT RATHER THAN PRINTED AS IF IT
// WORKED. That is the one reading this line can give that no other output can:
// the value parses, the platform accepts it, every arm passes, and it changes
// nothing about any run -- a knob that reads as set and does nothing, which is
// the exact shape `reconcile_every`'s own negative guard was added to refuse.
// Printing it the same way for every stage would make this report the thing
// that CONFIRMS the mistake.
func stageFacts(name string, p platform.Platform, profile string) string {
	st := p.Stages[name]
	if name != stage.API {
		if st.ReconcileEvery > 0 {
			return fmt.Sprintf("declares reconcile_every = %d, which only the %s stage reads, "+
				"so no run reconciles on it", st.ReconcileEvery, stage.API)
		}
		return "enabled, and declares nothing a run reads beyond that"
	}
	// THE EFFECTIVE PERIOD AND THE LEVEL IT CAME FROM, which is fable's clause
	// at coop rumor/1619 and the reason this verb is where the declaration
	// lives: a reader asking "does my project reconcile" is usually asking
	// "is MY declaration the one in force", and the number alone cannot say.
	//
	// IT IS THE RESOLVER'S ANSWER AND NOT THIS FUNCTION'S READING. Spelling
	// the precedence a second time here is the drift the single resolver
	// exists to prevent, and it would be invisible: both spellings agree until
	// a project declares the period at both levels.
	every, level := p.ReconcileEvery(profile)
	if every == 0 {
		return fmt.Sprintf("reconciles only after the last step, which is every run before "+
			"R8/110 -- no period at any level (%s)", level)
	}
	// THE LEVEL IS NAMED IN THE PROJECT'S OWN WORDS, a table a reader can go
	// and edit, rather than as the bare word the type carries.
	where := "[stages." + stage.API + "].reconcile_every"
	if level == platform.PeriodFromProfile {
		where = "[profiles." + profile + "].reconcile_every"
	}
	return fmt.Sprintf("reconciles every %d step(s) while the walk runs, and after the "+
		"last step -- from %s", every, where)
}
