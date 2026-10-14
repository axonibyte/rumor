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
	"os"

	"forge.axonibyte.com/axonibyte/rumor/internal/coverage"
)

const usage = "usage: rumor docs --check"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is separated from main so a test can drive the argument surface and read
// both streams. main itself then holds nothing but os.Exit, which is the one
// line a test cannot cover.
func run(args []string, stdout, stderr *os.File) int {
	if len(args) == 2 && args[0] == "docs" && args[1] == "--check" {
		n, err := coverage.Check(".")
		if err != nil {
			fmt.Fprintf(stderr, "docs --check: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "docs --check: %d path(s), every one mapped in docs/coverage.toml\n", n)
		return 0
	}
	fmt.Fprintln(stderr, usage)
	return 2
}
