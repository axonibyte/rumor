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
// SO A READER ASKS. `rumor report PROJECT` prints the selection and every skip
// and nothing else, which is what makes it safe to print on stdout: the digests
// are taken from `rumor run`, and this is a different invocation.
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
