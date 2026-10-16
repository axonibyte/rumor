// Command rumor is the command line entry point.
//
// R0.1 created exactly one subcommand, `docs --check`, which is the CLI
// surface that tag's minor bump paid for. Everything else arrives with the
// phase that owns it; R2.3 adds `check`, which is issue 44's "under one
// command" -- the three example packages run through the engine FROM THEIR
// LEDGER FILES, each finding named with the statute definition that produced
// it, against the verdict the example authored.
//
// Ported from the Rust original at 20675b3 (crates/rumor-cli/src/main.rs) for
// the pivot to Go, G1/issue 32. The three exit codes are carried unchanged,
// because .forgejo/workflows and tools/ read them: 0 on a clean check, 1 on a
// failed check, 2 on a usage error.
package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"forge.axonibyte.com/axonibyte/rumor/internal/attaint"
	"forge.axonibyte.com/axonibyte/rumor/internal/coverage"
	"forge.axonibyte.com/axonibyte/rumor/internal/examples"
)

const usage = "usage: rumor docs --check\n       rumor check [--examples DIR]\n       rumor attaint [--examples DIR]"

// coverageCheck is a seam, not indirection for its own sake: it lets a test
// drive the WRITE handling below -- including the failed-write path -- without
// needing a workspace on disk to be checked first.
var coverageCheck = func() (int, error) { return coverage.Check(".") }

// examplesCheck is the same kind of seam as coverageCheck: it lets a test drive
// the argument surface, the exit codes and the failed-write path without a
// tree of examples on disk.
var examplesCheck = func(dir string, out io.Writer) (examples.Result, error) {
	return examples.Run(context.Background(), dir, out)
}

// attaintCheck is the same seam again, for R3's self-test.
var attaintCheck = func(dir string, out io.Writer) (attaint.Report, error) {
	return attaint.Run(context.Background(), dir, out)
}

// defaultExamplesDir is where the three packages live in this repository. A
// flag overrides it, because the command is useful against another tree and
// because a test must be able to point it somewhere that does not exist.
const defaultExamplesDir = "examples"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is separated from main so a test can drive the argument surface and read
// both streams. main itself then holds nothing but os.Exit, which is the one
// line a test cannot cover. The streams are io.Writer rather than *os.File so a
// test can supply one that fails, which is what the write handling below is
// for.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 2 && args[0] == "docs" && args[1] == "--check" {
		n, err := coverageCheck()
		if err != nil {
			// Nowhere left to report a failed write to stderr, so the verdict
			// rides on the exit code alone -- which is why 1 is returned here
			// whatever the write did.
			_, _ = fmt.Fprintf(stderr, "docs --check: %v\n", err)
			return 1
		}
		// A FAILED WRITE HERE IS NOT COSMETIC, which is why it is checked
		// rather than discarded. This line IS the check's result: CI greps it
		// for the path count, and golangci-lint flagged it unchecked (run 143,
		// errcheck). Returning 0 after it failed to arrive would report success
		// for output nobody received -- the same defect as a gate whose
		// pipeline status came from its formatter.
		if _, werr := fmt.Fprintf(stdout,
			"docs --check: %d path(s), every one mapped in docs/coverage.toml\n", n); werr != nil {
			_, _ = fmt.Fprintf(stderr, "docs --check: the check passed over %d path(s) but its result could not be written: %v\n", n, werr)
			return 1
		}
		return 0
	}
	if len(args) >= 1 && args[0] == "check" {
		dir := defaultExamplesDir
		rest := args[1:]
		switch {
		case len(rest) == 0:
		case len(rest) == 2 && rest[0] == "--examples":
			dir = rest[1]
		default:
			_, _ = fmt.Fprintln(stderr, usage)
			return 2
		}
		res, err := examplesCheck(dir, stdout)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "check: %v\n", err)
			return 1
		}
		// THE SAME RULE AS docs --check ABOVE: this line IS the verdict a
		// reader and a workflow grep for, so a failed write is a failed check
		// rather than a cosmetic loss.
		if _, werr := fmt.Fprintf(stdout,
			"check: %d case(s) over %d package(s) from ledger files, %d agreed, %d disagreed\n",
			res.Cases, res.Packages, res.Agreed, res.Disagreed); werr != nil {
			_, _ = fmt.Fprintf(stderr, "check: %d of %d case(s) agreed but the result could not be written: %v\n",
				res.Agreed, res.Cases, werr)
			return 1
		}
		if res.Disagreed > 0 || res.Cases == 0 {
			return 1
		}
		return 0
	}
	if len(args) >= 1 && args[0] == "attaint" {
		dir, ok := examplesDir(args[1:])
		if !ok {
			_, _ = fmt.Fprintln(stderr, usage)
			return 2
		}
		rep, err := attaintCheck(dir, stdout)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "attaint: %v\n", err)
			return 1
		}
		// EVERY REFUSAL IS PRINTED, not just the first. A drill that trips two
		// arms is a different event from one that trips the arm it was cut for,
		// and the gate reads this text rather than the exit status.
		for _, r := range rep.Refusals {
			_, _ = fmt.Fprintln(stdout, r.String())
		}
		// THE TALLY RIDES ON EVERY PATH, pass or refusal, because the counts are
		// what distinguish a clean tree from an empty one.
		if _, werr := fmt.Fprintf(stdout,
			"attaint: %d package(s), %d case(s), %d rule(s), %d mutant(s); "+
				"%d planted lie(s) corroborated of %d, %d cleared by removal, %d planted-not-control; "+
				"%d rule(s) killed, %d unmutated; %d refusal(s)\n",
			rep.Packages, rep.Cases, rep.Rules, rep.Mutants,
			rep.Corroborated, rep.PlantedCases, rep.Cleared, rep.PlantedNotControl,
			rep.KilledRules, len(rep.Unmutated),
			len(rep.Refusals)); werr != nil {
			_, _ = fmt.Fprintf(stderr, "attaint: the verdict could not be written: %v\n", werr)
			return 1
		}
		if !rep.OK() {
			return 1
		}
		return 0
	}
	_, _ = fmt.Fprintln(stderr, usage)
	return 2
}

// examplesDir reads the one flag `check` and `attaint` share.
func examplesDir(rest []string) (string, bool) {
	switch {
	case len(rest) == 0:
		return defaultExamplesDir, true
	case len(rest) == 2 && rest[0] == "--examples":
		return rest[1], true
	}
	return "", false
}
