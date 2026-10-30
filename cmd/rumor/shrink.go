package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

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

	// WHICH PROVIDER OWNS THE WORLD IS ONE QUESTION WITH TWO ASKERS NOW, so
	// the reading lives in soleProvider and this call site owns only its own
	// prefix and its own statuses. A shrink and a contention need the same
	// thing for different reasons -- one replays a failure on a world it can
	// put back, the other points N adapters at one world -- and two copies of
	// "more than one is a refusal" could have drifted apart.
	name, skip, refusal := soleProvider(prep, "the world a shrink replays in")
	if skip != "" {
		_, err := fmt.Fprintf(stdout, "shrink: NOT SHRUNK -- %s\n", skip)
		return 0, err
	}
	if refusal != "" {
		_, err := fmt.Fprintf(stdout, "shrink: REFUSED %s\n", refusal)
		return 1, err
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
