// Command rumor is the command line entry point.
//
// R0.1 created exactly one subcommand, `docs --check`, which is the CLI
// surface that tag's minor bump paid for. Everything else arrives with the
// phase that owns it.
//
// Ported from the Rust original at 20675b3 (crates/rumor-cli/src/main.rs) for
// the pivot to Go, G1/issue 32. The three exit codes are carried unchanged,
// because .forgejo/workflows and tools/ read them: 0 on a clean check, 1 on a
// failed check, 2 on a usage error.
package main

import (
	"fmt"
	"io"
	"os"

	"forge.axonibyte.com/axonibyte/rumor/internal/coverage"
)

const usage = "usage: rumor docs --check"

// coverageCheck is a seam, not indirection for its own sake: it lets a test
// drive the WRITE handling below -- including the failed-write path -- without
// needing a workspace on disk to be checked first.
var coverageCheck = func() (int, error) { return coverage.Check(".") }

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
	_, _ = fmt.Fprintln(stderr, usage)
	return 2
}
