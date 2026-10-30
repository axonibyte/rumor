package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/decl"
	"forge.axonibyte.com/axonibyte/rumor/internal/platform"
	"forge.axonibyte.com/axonibyte/rumor/internal/quiesce"
	"forge.axonibyte.com/axonibyte/rumor/internal/rollback"
	"forge.axonibyte.com/axonibyte/rumor/internal/schedule"
	"forge.axonibyte.com/axonibyte/rumor/internal/shrink"
	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

// shrinkFound reduces a failing trace to a shorter one with the same finding,
// and it is the commit that gives the shrinker a path the product takes.
//
// IT EXISTS BECAUSE internal/shrink WAS IN NO BINARY'S DEPENDENCY CLOSURE.
// Measured at v0.1.36 (278b554), three ways with a control each: `go list
// -deps` returns internal/shrink for NONE of cmd/rumor, cmd/toy or
// cmd/toy-adapter while returning internal/engine and internal/schedule for
// all three; `schedule.Replay` renamed leaves every main building, so it had no
// caller; `rollback.Open` renamed does too, while renaming
// `rollback.Registered` -- the control -- breaks run.go:359 by name. So six
// shrink steps, a replay built to feed them and the whole rollback trait were
// built and unreached, and the product's only use of that package was counting
// how many providers had registered. A count is not an integration.
//
// AND THAT IS WHY IT IS 92's FIRST COMMIT RATHER THAN A LATER ONE (fable, coop
// r6/146): "every candidate quiesces before judging" is a claim about the
// SHRINKER's candidate replays, and ruling 3 at r6/78 forbids proving a claim
// on a path the product never takes. R6.1 shipped an engine whose overlay no
// walk reached; an arm written against shrink.Shrink with a fake Replay would
// be that defect again with a different package name.
//
// THE SHAPE IS R5.5's, in tests/shrink, and is not invented here: the shrink's
// world belongs to the PROVIDER, not to the session the walk used. The provider
// starts the system under test and owns its address; each candidate re-opens
// the adapter against that address, and the prober is rebuilt on the candidate's
// own session.
//
// EVERY REFUSAL BELOW IS A PROJECT'S DOCUMENT BEING WRONG, which is why they
// print and return a refusal rather than a could-not-run: the declarations say
// a world can be put back and the pieces to do it are missing or ambiguous.
func shrinkFound(ctx context.Context, dir string, prep platform.Report, p schedule.Project,
	probes []bindings.Probe, seed uint64, rec schedule.Record, stdout io.Writer) (int, error) {

	// NO PROVIDER IS NOT A REFUSAL AND NOT AN ERROR. docs/design.md requires the
	// replay to run "on a verified clean rollback", so a project that declares
	// no provider cannot be shrunk at all -- and most of the examples declare
	// none on purpose. The line is printed because a reader who expected a
	// minimal trace needs to know why there is none, which is the same reason
	// the quiesce line prints when nothing was reconciled.
	if len(prep.Platform.Rollback) == 0 {
		_, err := fmt.Fprintf(stdout, "shrink: NOT SHRUNK -- the project declares no "+
			"[[rollback]] provider, so there is no world to replay the failure on\n")
		return 0, err
	}
	// MORE THAN ONE IS A REFUSAL AND NOT A FIRST-WINS. Which world a shrink
	// replays in is the project's statement, and `covers` names components
	// rather than ranking providers, so nothing in the document says which to
	// use. Picking the first would be this code inventing a precedence the
	// format does not have.
	if len(prep.Platform.Rollback) > 1 {
		names := make([]string, 0, len(prep.Platform.Rollback))
		for _, rb := range prep.Platform.Rollback {
			names = append(names, rb.Provider)
		}
		_, err := fmt.Fprintf(stdout, "shrink: REFUSED the project declares %d rollback "+
			"providers (%s) and nothing says which owns the world a shrink replays in\n",
			len(names), strings.Join(names, ", "))
		return 1, err
	}

	name := prep.Platform.Rollback[0].Provider

	// A DECLARED PROVIDER THIS BUILD DOES NOT REGISTER IS A CARRIED NAME, NOT A
	// FAILURE (fable, coop rumor/1202, on rook's /1197). examples/twin declares
	// `doc-store-truncate` and says in its own comment that the name is carried
	// AS DECLARED until docs/rollback.md defines the interface in R13, so the
	// contract permits a name nothing can open yet. My first cut sent that
	// through the could-not-run path, which printed "run: could not shrink: no
	// provider named ..." on stderr for a shipped R4 example -- a misleading
	// line on a row that is behaving exactly as its document says.
	//
	// IT IS CHECKED BEFORE Open AND NOT FROM Open's ERROR, because an error
	// string is not a state: "not registered" and "registered but would not
	// start" are different facts and only the first is the contract's.
	registered := rollback.Registered()
	known := false
	for _, r := range registered {
		if r == name {
			known = true
		}
	}
	if !known {
		sort.Strings(registered)
		_, werr := fmt.Fprintf(stdout, "shrink: NOT SHRUNK -- the project declares the rollback "+
			"provider %q, which this build does not register (%s): the name is carried as "+
			"declared until the interface is defined, so no shrink was attempted\n",
			name, strings.Join(registered, ", "))
		return 0, werr
	}

	prov, err := rollback.Open(name, rollback.Options{Dir: dir, Env: prep.Platform.Adapter.Env})
	if err != nil {
		// COULD-NOT-RUN: the provider IS registered and declared, and opening
		// it failed, which is the machine and not the document.
		return 2, fmt.Errorf("opening the rollback provider %q: %w", name, err)
	}
	// REMOVE THE WORLD AND NOT ONLY THE PROCESS (issue 88): Close stops the
	// system and leaves its directory by contract, so a run would leave one
	// world behind per shrink. The Close fallback stays for a provider that has
	// not grown the method.
	defer func() {
		if r, ok := prov.(interface{ RemoveWorld() }); ok {
			r.RemoveWorld()
		} else if c, ok := prov.(interface{ Close() }); ok {
			c.Close()
		}
	}()

	at, ae, refusal := providerAddress(prov, name, prep.Platform.Adapter.Env)
	if refusal != "" {
		_, werr := fmt.Fprintf(stdout, "shrink: REFUSED %s\n", refusal)
		return 1, werr
	}

	cp, err := prov.Checkpoint(ctx)
	if err != nil {
		return 2, fmt.Errorf("checkpointing the clean world of %s: %w", name, err)
	}
	defer func() { _ = prov.Discard(ctx, cp) }()

	// THE ADDRESS IS RE-READ EVERY TIME, because a restore rebinds the port: a
	// session opened once and reused would be talking to a world that no longer
	// exists after the first candidate.
	sessionNow := func() (*wire.Session, error) {
		if err := os.Setenv(ae.AddrEnv(), at.Addr()); err != nil {
			return nil, fmt.Errorf("naming the provider's world in %s: %w", ae.AddrEnv(), err)
		}
		fresh, lerr := platform.Load(filepath.Join(dir, "platform.toml"), decl.TOML)
		if lerr != nil {
			return nil, lerr
		}
		return wire.Open(ctx, wire.Spawn{
			Command: fresh.Platform.Adapter.Command,
			Cwd:     dir,
			Env:     fresh.Platform.Adapter.Env,
		}, 0)
	}

	res, serr := shrink.Shrink(ctx, shrink.Request{
		Seed:  seed,
		Steps: rec.Steps,
		Kinds: kindsDrawn(rec.Steps),
		// THE WALK's WANT IS THE STATUTE's VERDICT, named as such now that a
		// stage can author one too (fable, coop rumor/1521). The reading is
		// unchanged -- StatuteVerdict carries the comparison that was in Once.
		Want:       shrink.StatuteVerdict{Finding: *rec.Finding},
		Provider:   prov,
		Checkpoint: cp,
		// NOTHING IS PROMOTED, AND THAT IS NOT AN OVERSIGHT. Request.SeedsFile's
		// own contract says an empty value "promotes nothing, which is what a
		// caller shrinking for a report rather than for a regression suite
		// wants" -- and `rumor run` is the report caller.
		//
		// DRIVEN BEFORE IT WAS DECIDED: passing the project's own path appended
		// `1  # bug` to examples/toy/seeds.lock on the first run of this
		// wiring. A plain run had edited the project's checked-in regression
		// suite as a side effect, with a comment reading `bug` beside entries
		// whose comments name the defect, the row and the invocation. Every
		// subsequent run would add another, and a seed nobody reviewed would be
		// replayed by the arm that re-runs every promotion.
		//
		// A PROMOTION IS A DECISION. The lock is the one artifact of a shrink
		// that gets checked in (ratchet's write-once point, coop r6/15), so
		// what goes in it is for whoever is adding to the suite -- not for
		// whatever happened to run last.
		SeedsFile: "",
		Replay: func(c context.Context, steps int, without []string) (shrink.Candidate, error) {
			s, oerr := sessionNow()
			if oerr != nil {
				return shrink.Candidate{}, fmt.Errorf("re-opening the adapter on the "+
					"restored world: %w", oerr)
			}
			defer func() { _ = s.Close() }()

			// THE PROBER IS REBUILT ON THIS SESSION AND THAT IS NOT
			// BOOKKEEPING. A Prober holds the session it asks through, and the
			// walk's session is closed before the first candidate replays.
			// Carrying it over gave "write |1: file already closed" from the
			// first probe read -- a dead instrument reported as a failure to
			// reproduce, which makes every candidate look too small (R5.5, in
			// tests/shrink's own comment).
			proj := p
			if len(probes) > 0 {
				pr, qerr := quiesce.New(probes, s)
				if qerr != nil {
					return shrink.Candidate{}, fmt.Errorf("rebuilding the project's "+
						"prober on the candidate's session: %w", qerr)
				}
				proj.Prober = pr
			}
			f, ran, open, rerr := schedule.Replay(c, proj, s, seed, 1, steps, without)
			if rerr != nil {
				return shrink.Candidate{Steps: ran, PendingOpen: open}, rerr
			}
			// NO FINDING IS THE ZERO Finding AND NOT AN ERROR: schedule.Replay
			// returns a pointer so that "walked and found nothing" is
			// expressible, and shrink.Once compares `Defined` -- so the zero
			// value is exactly the shape it reads as "found none".
			if f == nil {
				return shrink.Candidate{Steps: ran, PendingOpen: open}, nil
			}
			return shrink.Candidate{Finding: *f, Steps: ran, PendingOpen: open}, nil
		},
	})
	if serr != nil {
		return 2, fmt.Errorf("shrinking seed %d: %w", seed, serr)
	}
	if _, err := io.WriteString(stdout, res.Render()); err != nil {
		return 2, err
	}
	return 0, nil
}

// kindsDrawn is the action kinds a trace actually drew, in the order it drew
// them first.
//
// THE SHRINKER REMOVES KINDS AND CAN ONLY REMOVE WHAT WAS THERE. Handing it the
// project's whole weight table would have it try to drop kinds the trace never
// used, which costs a replay each and can remove nothing -- and a candidate
// that differs from its parent in no way is not a reduction.
//
// tests/shrink HAS THE SAME FUNCTION and that duplication is deliberate for now:
// this one is the product's, and a shared helper would put a test's convenience
// in the engine's path. If a third caller appears it belongs in internal/schedule
// beside the Step it reads.
func kindsDrawn(steps []schedule.Step) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range steps {
		if s.Kind == "" || seen[s.Kind] {
			continue
		}
		seen[s.Kind] = true
		out = append(out, s.Kind)
	}
	return out
}

// providerAddress answers whether the adapter can be pointed at the world this
// provider owns, and names the reason when it cannot.
//
// IT IS A FUNCTION SO THE THREE REFUSALS HAVE A FIXTURE. No committed project
// can reach them: the product registers exactly one provider and it implements
// both methods, and examples/toy passes TOY_ADDR through, so a refusal driven
// only from the CLI would be a refusal nothing trips -- arm 19's shape, which I
// named at coop r6/148 rather than leave for the review. A provider is an
// interface value, so a stub missing a method is expressible in a test and not
// in a project document.
//
// THE ENV CHECK IS HERE WITH THEM BECAUSE IT IS THE SAME QUESTION ONE STEP ON:
// a provider can own an address and name its variable, and the project can
// still fail to pass that variable to the adapter -- docs/testing-methodology.md
// records exactly that defect, where an `env` list omitting TOY_ADDR meant the
// value could never reach the adapter, so it started a system of its own and
// every candidate ran against a world the provider did not control. A
// reproduction that proves nothing, with nothing to notice.
//
// IT RETURNS THE REASON RATHER THAN PRINTING IT, so the caller owns the one
// "shrink: REFUSED" prefix and an arm can read the words without a stdout.
func providerAddress(prov any, name string, env []string) (interface{ Addr() string }, interface{ AddrEnv() string }, string) {
	at, ok := prov.(interface{ Addr() string })
	if !ok {
		return nil, nil, fmt.Sprintf("%s does not expose the address of the world it owns, so "+
			"the adapter cannot be pointed at it and every candidate would run against a "+
			"system this provider cannot put back", name)
	}
	ae, ok := prov.(interface{ AddrEnv() string })
	if !ok {
		return nil, nil, fmt.Sprintf("%s does not name the environment variable its adapter "+
			"reads the address from, and guessing one out of the adapter's env list would be "+
			"this code choosing a convention", name)
	}
	for _, e := range env {
		if e == ae.AddrEnv() {
			return at, ae, ""
		}
	}
	return nil, nil, fmt.Sprintf("%s owns its world's address and reads it from %s, which "+
		"[adapter] env does not pass through (%v): the adapter would start a system of its own "+
		"and the candidates would not be replays", name, ae.AddrEnv(), env)
}
