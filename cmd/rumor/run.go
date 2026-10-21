package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/decl"
	"forge.axonibyte.com/axonibyte/rumor/internal/platform"
	"forge.axonibyte.com/axonibyte/rumor/internal/rollback"
	"forge.axonibyte.com/axonibyte/rumor/internal/schedule"
	"forge.axonibyte.com/axonibyte/rumor/internal/wire"

	// THE ONE SHIPPED ROLLBACK PROVIDER, imported for its registration.
	//
	// fable made this R5.3's first item at coop r5/58, from ibis's measurement
	// at r4/128: NOTHING in the binary imported internal/rollback or this
	// package, so rollback.Registered() was empty everywhere except those
	// packages' own test binaries -- and R5.3's shrink step cannot Open() a
	// provider that was never registered. toyrestart registers itself in
	// init(), so the import is the whole fix and it belongs where the run
	// lives: a run is what needs a clean world to shrink on.
	//
	// IT IS BLANK BECAUSE THE SIDE EFFECT IS THE POINT. A named import that
	// went unused would not compile, and routing a call through it to satisfy
	// the compiler would be a worse lie than the underscore.
	_ "forge.axonibyte.com/axonibyte/rumor/internal/rollback/toyrestart"
)

// defaultSteps is how many steps a run takes when nobody says.
//
// A NUMBER AND NOT A BUDGET: R15 owns budgets and R22 owns profiles with their
// seed counts. This is enough steps for a walk to reach a second creation and
// a precondition that only becomes true after one, which is what makes a run
// of the default length say anything at all.
const defaultSteps = 20

// runProject walks one project against its declared adapter.
//
// THE SEED IS PRINTED FIRST, ALWAYS, which is issue 72's first bullet: "one
// integer, printed first on every run, accepted back by `--seed`". It is
// printed BEFORE the adapter is spawned, so a run that dies in the handshake
// still tells the reader the one thing they need to replay it. A seed reported
// only on success would be missing from exactly the runs somebody wants to
// reproduce.
func runProject(dir string, seed uint64, haveSeed bool, steps int, stdout, stderr io.Writer) int {
	if steps <= 0 {
		steps = defaultSteps
	}
	if !haveSeed {
		minted, err := schedule.Mint()
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "run: minting a seed: %v\n", err)
			return 2
		}
		seed = minted
	}
	if _, err := fmt.Fprintf(stdout, "run: seed %d\n", seed); err != nil {
		_, _ = fmt.Fprintf(stderr, "run: writing the seed: %v\n", err)
		return 2
	}

	prep, err := platform.Load(filepath.Join(dir, "platform.toml"), decl.TOML)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "run: could not run: reading platform.toml: %v\n", err)
		return 2
	}
	if !prep.OK() {
		for _, ref := range prep.Refusals {
			_, _ = fmt.Fprintf(stdout, "run: REFUSED %s\n", ref)
		}
		return 1
	}
	brep, err := bindings.Load(filepath.Join(dir, "bindings.toml"), decl.TOML)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "run: could not run: reading bindings.toml: %v\n", err)
		return 2
	}
	if !brep.OK() {
		for _, ref := range brep.Refusals {
			_, _ = fmt.Fprintf(stdout, "run: REFUSED %s\n", ref)
		}
		return 1
	}
	statute, err := os.ReadFile(filepath.Join(dir, "statute.rego"))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "run: could not run: reading statute.rego: %v\n", err)
		return 2
	}

	p := schedule.Project{
		Statute: string(statute),
		Actions: brep.Bindings.Actions,
		Weights: prep.Platform.Weights,
		Venire:  prep.Platform.Venire,
		Roles:   brep.Bindings.Roles,
	}

	ctx := context.Background()
	sess, err := wire.Open(ctx, wire.Spawn{
		Command: prep.Platform.Adapter.Command,
		Cwd:     dir,
		Env:     prep.Platform.Adapter.Env,
	}, 0)
	if err != nil {
		// COULD-NOT-RUN AND NEVER A REFUSAL: "the project is not wrong; its
		// adapter is" (docs/contract.md:57).
		_, _ = fmt.Fprintf(stderr, "run: could not run: %v\n", err)
		return 2
	}
	defer func() { _ = sess.Close() }()

	rec, werr := schedule.Walk(ctx, p, sess, seed, steps, 1)
	// THE RECORD IS PRINTED EVEN WHEN THE WALK STOPPED EARLY, because the
	// steps it did take are what a reader needs to see where it stopped.
	for _, s := range rec.Steps {
		kind := s.Kind
		if kind == "" {
			kind = "(nothing applicable)"
		}
		mark := ""
		if s.Planted {
			mark = " PLANTED"
		}
		// WHAT THE STEP WAS ABOUT, where it consumed anything. A trace showing
		// only what a step CREATED cannot tell two joins of different events
		// apart, which is exactly the difference a reader is looking for when a
		// walk goes wrong. Omitted where nothing was consumed rather than
		// printed empty, so the common line stays short.
		used := ""
		if len(s.Used) > 0 {
			used = fmt.Sprintf(" <- %v", s.Used)
		}
		if _, err := fmt.Fprintf(stdout, "run: %3d %s %s%s %v%s\n",
			s.N, s.Actor, kind, mark, s.Handles, used); err != nil {
			_, _ = fmt.Fprintf(stderr, "run: writing the trace: %v\n", err)
			return 2
		}
	}
	if werr != nil {
		_, _ = fmt.Fprintf(stderr, "run: could not run: %v\n", werr)
		return 2
	}
	// THE SESSION COUNT IS IN THE VERDICT for the provider count's reason: a
	// token is acquired at first use and reused after, so a holder that
	// re-acquired every step would send identical requests and the difference
	// would be invisible to a reader. The number makes it readable.
	if _, err := fmt.Fprintf(stdout,
		"run: %s -- seed %d, %d step(s), %d handle(s), %d session(s); "+
			"%d rollback provider(s) registered\n",
		filepath.Base(dir), rec.Seed, len(rec.Steps), len(rec.HandleSequence()),
		rec.Sessions, len(rollback.Registered())); err != nil {
		_, _ = fmt.Fprintf(stderr, "run: writing the verdict: %v\n", err)
		return 2
	}
	return 0
}

// parseRunArgs reads `run PROJECT [--seed N] [--steps N]`.
func parseRunArgs(args []string) (dir string, seed uint64, haveSeed bool, steps int, bad string) {
	rest := args
	for len(rest) > 0 {
		switch {
		case rest[0] == "--seed" || rest[0] == "--steps":
			if len(rest) < 2 {
				return "", 0, false, 0, rest[0] + " wants a value"
			}
			n, err := strconv.ParseUint(rest[1], 10, 64)
			if err != nil {
				return "", 0, false, 0, fmt.Sprintf("%s %s: wanted a non-negative integer", rest[0], rest[1])
			}
			if rest[0] == "--seed" {
				seed, haveSeed = n, true
			} else {
				if n == 0 {
					return "", 0, false, 0, "--steps 0: a walk of no steps is not a walk"
				}
				steps = int(n)
			}
			rest = rest[2:]
		case len(rest[0]) > 2 && rest[0][:2] == "--":
			return "", 0, false, 0, fmt.Sprintf("%s is not a flag of run", rest[0])
		default:
			if dir != "" {
				return "", 0, false, 0, "run takes one project directory"
			}
			dir = rest[0]
			rest = rest[1:]
		}
	}
	if dir == "" {
		return "", 0, false, 0, "run wants a project directory"
	}
	return dir, seed, haveSeed, steps, ""
}
