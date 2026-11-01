package main

import (
	"fmt"
	"io"
	"path/filepath"

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
			stageFacts(name, prep.Platform.Stages[name]))
	}
	return 0
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
// choice but the subject's shape: cmd/rumor/run.go reads
// `Stages[stage.API].ReconcileEvery` and says why -- the walk IS the api stage,
// so a period under another stage's table is a period for a stage that does not
// walk.
//
// SO A PERIOD DECLARED ELSEWHERE IS NAMED AS INERT RATHER THAN PRINTED AS IF IT
// WORKED. That is the one reading this line can give that no other output can:
// the value parses, the platform accepts it, every arm passes, and it changes
// nothing about any run -- a knob that reads as set and does nothing, which is
// the exact shape `reconcile_every`'s own negative guard was added to refuse.
// Printing it the same way for every stage would make this report the thing
// that CONFIRMS the mistake.
func stageFacts(name string, st platform.Stage) string {
	if name != stage.API {
		if st.ReconcileEvery > 0 {
			return fmt.Sprintf("declares reconcile_every = %d, which only the %s stage reads, "+
				"so no run reconciles on it", st.ReconcileEvery, stage.API)
		}
		return "enabled, and declares nothing a run reads beyond that"
	}
	if st.ReconcileEvery > 0 {
		return fmt.Sprintf("reconciles every %d step(s) while the walk runs, and after the "+
			"last step", st.ReconcileEvery)
	}
	return "reconciles only after the last step, which is every run before R8/110"
}
