package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/decl"
	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
	"forge.axonibyte.com/axonibyte/rumor/internal/platform"
	"forge.axonibyte.com/axonibyte/rumor/internal/quiesce"
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

// findingStatus is the exit status a found bug reports.
//
// THREE, RULED BY fable AT coop r5/176, and the ground is a product choice
// rather than a claim about whose fault a bug is: this verb has four statuses,
// and the two outcomes the tool exists to tell apart never share a number.
//
// fable's earlier /170 ruled 1 because "the project is wrong, which is what a
// bug is", and withdrew that reason in its own words. Three things answer it.
// docs/contract.md draws the line the other way -- "the project is not wrong;
// its adapter is" -- so a fault elsewhere is not the project's fault.
// docs/vocabulary.md defines `bug` as a disagreement over claims that are
// HONEST, which makes it the system under test's. And this function already
// spends 1 on REFUSED two returns above where a finding lands (rook, r5/172):
// `return 1` appears exactly twice in this file, both immediately after
// `run: REFUSED`, so a finding at 1 could not be told from "I refused your
// project" by anything reading the status.
//
// IT IS AN ADDITION AND NOT A MOVE, which is why v0.1.28 stays a patch: no
// existing status changes meaning, and docs/release.md's rule is that
// additions are patches until 1.0 (fable, r5/176).
const findingStatus = 3

// runProject walks one project against its declared adapter.
//
// THE SEED IS PRINTED FIRST, ALWAYS, which is issue 72's first bullet: "one
// integer, printed first on every run, accepted back by `--seed`". It is
// printed BEFORE the adapter is spawned, so a run that dies in the handshake
// still tells the reader the one thing they need to replay it. A seed reported
// only on success would be missing from exactly the runs somebody wants to
// reproduce.
func runProject(dir string, seed uint64, haveSeed bool, steps int, profile string,
	stdout, stderr io.Writer) int {
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
		// THE OBSERVATIONS THE WALK MAKES (R5.5). A project declaring none is
		// walked as before, which is what keeps every pre-R5.5 fixture's
		// meaning.
		Observations: brep.Bindings.Observations,
		// THE EVENTUAL TYPES, so the walk can judge a lagging observation as
		// `pending` instead of `bug` (R6.3). BOTH FORMS TRAVEL, the duration
		// one with its parsed nanoseconds: this comment said "only the quiesce
		// form is passed through with a zero bound" until R7.1 read it against
		// the code, and that stopped being true when rook fixed EventualOf to
		// carry the bound (coop rumor/1191, ratchet /1192). What keeps a
		// duration bound from being read as expired is engine.Judge's refusal,
		// not a truncation here -- which is the arrangement the old sentence
		// said it was avoiding.
		Eventual: schedule.EventualOf(brep.Bindings),
		// AND WHERE THIS PROJECT'S DEADLINES ARE READ (R7.4). `advance` lands
		// on these beside the live pending bounds; a project declaring none is
		// walked exactly as R7.3 left it.
		DeadlineInstants: brep.Bindings.DeadlineInstants,
		// CLOCK CONTROL IS THE `[clock]` TABLE BEING THERE (R7.1). The run may
		// hold a clock only where the project declares one, and internal/ledger
		// refuses a clock fact without it; the table's presence is the control.
		ClockControl: bindings.ClockControlled(brep.Bindings),
		// AND R7.2's TYPED `form` SAYS HOW. It is carried straight through:
		// the parser has already refused a word that is neither form, and a
		// table that names NO form is refused by the walk, which is the reader
		// that knows which request it was about to send.
		ClockForm: brep.Bindings.ClockForm,
		// THE PROFILE THE READER ASKED FOR, which selects the weights table the
		// pool draws from. Empty is the default table, which is every run that
		// does not pass the flag -- so the control seeds are unaffected by the
		// flag existing.
		Profile: profile,
	}

	// A QUIESCE BOUND NOTHING CAN JUDGE IS A REFUSAL, NOT A QUIET RUN.
	//
	// ratchet drove the alternative at coop r6/8: a claim type bounded
	// `quiesce` with nothing supplying a quiesce answer holds a real
	// disagreement open for every pass -- the register keeps it, StillOpen
	// cannot see it because the query WAS re-asked, the walk never ends on a
	// `pending` -- and the run exits 0 on a disagreement. That is the one
	// outcome worse than a missed bug, and it is reachable today because the
	// untyped `[quiesce]` table parses.
	//
	// THE CONDITION IS STRUCTURAL AND NOT A SENTENCE ABOUT THIS BUILD (fable,
	// coop r6/12): it refuses when the project declares such a bound and the
	// PROBER SLOT IS NIL. R6.2 falsifies that by construction when it wires a
	// prober in, so the shutter removes itself; nothing here has to be
	// remembered and removed by hand, and a later reader checking whether the
	// window is shut reads a fact rather than a promise.
	//
	// IT IS A REFUSAL AND NOT A COULD-NOT-RUN: the PROJECT declares a bound
	// this build cannot judge, which is the project's own declaration being
	// unsatisfiable here, and docs/cli.md's 1 is "the project's own documents
	// are wrong".
	// R6.2 WIRED ONE, SO THE SHUTTER HAS REMOVED ITSELF -- and what is left is
	// narrower rather than gone. internal/quiesce implements the Prober slot,
	// built below from the probes the project declared, so "no prober is wired"
	// is no longer a thing this build can be. What remains unjudgeable is a
	// quiesce bound with NO PROBE DECLARED: there is nothing to ask, and
	// "quiet" over an empty set is a claim about nothing.
	//
	// IT DOES NOT TRUST validate TO HAVE RUN. That is arm 7b's refusal too, and
	// a run is not obliged to have been validated first -- which is the whole
	// reason this check is structural and here.
	var bounded []string
	for name, c := range brep.Bindings.Claims {
		if c.Bound == bindings.Quiesce {
			bounded = append(bounded, name)
		}
	}
	sort.Strings(bounded)
	if len(bounded) > 0 && len(brep.Bindings.Probes) == 0 {
		_, _ = fmt.Fprintf(stdout, "run: REFUSED the bounds on %s are `quiesce` and the project "+
			"declares no [[quiesce.probe]]; there is nothing to ask, so the bounds cannot be "+
			"judged\n", strings.Join(bounded, ", "))
		return 1
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

	// THE PROBER IS BUILT HERE AND NOT ABOVE because it asks the ADAPTER: a
	// probe is an `observe` with source `direct`, so it needs the session, and
	// the session is what the lines above just opened.
	//
	// A MALFORMED PROBE IS A REFUSAL AND NOT A COULD-NOT-RUN: the project
	// declared a probe with no timeout, an unknown kind or a `query` on a kind
	// that does not read one, which is the project's own document being wrong
	// -- validate's 7c, 7d and 7e by name, re-checked here because a run need
	// not have been validated.
	if len(brep.Bindings.Probes) > 0 {
		pr, qerr := quiesce.New(brep.Bindings.Probes, sess)
		if qerr != nil {
			_, _ = fmt.Fprintf(stdout, "run: REFUSED %v\n", qerr)
			return 1
		}
		p.Prober = pr
	}

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
		// AND WHERE THE SYSTEM DECLINED (R7.4). A refused step appends no
		// claim, so without this line the trace shows a draw with no handle
		// and no effect and a reader cannot tell it from a step that minted
		// nothing -- a tell, or an inapplicable kind. The REASON rides along
		// because it names the action, the declared key and what came back,
		// which is what the reader would otherwise go to the adapter's log
		// for.
		if s.Refused != "" {
			mark = " REFUSED: " + s.Refused
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
		// AND WHO WAS TOLD, for a step that told somebody. A tell mints no
		// handle and consumes none, so without this the line reads
		// `member_1 tell []` and the drill cannot see whether the actor that
		// joined was the actor that was told.
		if s.Hop != "" {
			used = fmt.Sprintf(" -> %s", s.Hop)
		}
		// AN EMPTY HANDLE LIST IS OMITTED rather than printed as `[]`: a step
		// that minted nothing is a telling step, and `tell [] -> guest_1` reads
		// as a list that failed to fill rather than one there was never
		// anything to put in.
		minted := ""
		if len(s.Handles) > 0 {
			minted = fmt.Sprintf(" %v", s.Handles)
		}
		if _, err := fmt.Fprintf(stdout, "run: %3d %s %s%s%s%s\n",
			s.N, s.Actor, kind, mark, minted, used); err != nil {
			_, _ = fmt.Fprintf(stderr, "run: writing the trace: %v\n", err)
			return 2
		}
	}
	if werr != nil {
		_, _ = fmt.Fprintf(stderr, "run: could not run: %v\n", werr)
		return 2
	}
	// A FINDING IS THE RUN'S PRODUCT, AND THE VERB HAS TO SAY SO. The walk
	// carries the first `bug` and the step it came after (fable, coop r5/152
	// ruling 2), and until now `cmd/` read NEITHER field -- zero hits, ibis at
	// coop main/149. A found bug therefore printed as a slightly short trace
	// with a clean verdict and exit 0: the one outcome this tool exists to
	// produce, reported as nothing having happened. Measured before the fix on
	// examples/toy, where seeds 6, 7 and 4242 each stop early on a finding and
	// all three exited 0.
	//
	// IT IS PRINTED BEFORE THE VERDICT because the verdict is the last line of
	// every run and the finding belongs beside the step that produced it, which
	// the trace shows directly above.
	if rec.Finding != nil {
		// THE ATTRIBUTION IS PART OF THE IDENTITY, which is why it is on this
		// line and not inferred from the trace: "two findings are the same when
		// the rule that fired, the finding's value, its attribution
		// (`rests_on_planted`) and its kind all agree" (docs/design.md), and
		// this line carries those four plus the step, which is the FIVE a
		// control compares (fable, coop rumor/1039). `bug` resting
		// on a planted claim is the engine catching a LIE rather than the
		// system under test being wrong. A statute that declares no such rule
		// says so in that word: the attribution is then UNKNOWN, and printing
		// `false` for it would assert the one thing nothing measured.
		rests := "undeclared"
		if v, ok := rec.Finding.Rules[engine.AttributionRule]; ok {
			rests = fmt.Sprintf("%v", v)
		}
		// AND THE KIND, WHERE THE VOCABULARY GIVES THE FINDING ONE. `stale` is
		// a `bug` whose kind says why it is one -- a pending past its bound --
		// so it is named at the point the value is named and the status stays
		// 3: a stale is a finding, not a refusal (fable, coop breakout-r6/0
		// ruling 2). A finding the statute produced on its own carries no kind
		// and this adds nothing to its line, which is why examples/toy's output
		// is unchanged: nothing in the toy is eventual yet.
		// THE FINDING SAYS ITSELF, kind included. This composed the kind by
		// hand at R6.1, which is the defect engine.Finding.Says's own comment
		// warns about -- "each consumer fixing its own print leaves the next
		// one to rediscover it" -- with this as the third consumer. fable ruled
		// it into Says at coop r6/15 so the shrink trace and the promoted
		// comment carry what the comparator compares; the wording here is
		// unchanged because Says took this line's own.
		if _, err := fmt.Fprintf(stdout, "run: FOUND %s after step %d -- %s, %s %s\n",
			rec.Finding.Says(), rec.FoundAt, rec.Finding.Rule,
			engine.AttributionRule, rests); err != nil {
			_, _ = fmt.Fprintf(stderr, "run: writing the finding: %v\n", err)
			return 2
		}
		// AND THE OVERLAY'S REASON, ON ITS OWN LINE, WHERE THE ENGINE WROTE ONE.
		//
		// R6.2's `quiesce_timeout` needs this and D5 asks for it by name: "the
		// never-quiet one ends in the timeout finding ... hang report WRITTEN".
		// The report says which probe did not read quiet, its declared timeout,
		// how many times it was asked and over what span -- none of which fits
		// the FOUND line's shape, and all of which is what a reader does next.
		//
		// IT IS NOT SPECIAL-CASED TO THAT FINDING, deliberately: `stale` has
		// carried a Reason since R6.1 locating what was late, and nothing
		// printed it either. A finding whose reason the engine wrote and the
		// command swallowed is the struct-holds-what-the-caller-needs shape
		// rook named at coop r5/204, and it was live here for two findings.
		if rec.Finding.Overlay != nil && rec.Finding.Overlay.Reason != "" {
			if _, err := fmt.Fprintf(stdout, "run:   %s\n", rec.Finding.Overlay.Reason); err != nil {
				_, _ = fmt.Fprintf(stderr, "run: writing the finding's reason: %v\n", err)
				return 2
			}
		}
		// AND THE CLOCK-DERIVED HALF ON STDERR, NEVER BESIDE THE VERDICT. The
		// comment above still describes the report, but one sentence of it
		// moved: "how many times it was asked and over what span" is written by
		// the CLOCK, and two attempts of one seed gave "343 time(s) over
		// 2.002s" and "340 time(s) over 2.005s". fable's ruling 2 (coop r6/78)
		// requires those two attempts to be byte-identical, so a measurement on
		// stdout would make D1 unreachable for every run that ends on a
		// timeout. It is not dropped -- the ask count against the declared
		// cadence is what shows a probe was really being asked -- it is printed
		// where the adapter's own timings already go.
		if hm, ok := p.Prober.(interface{ HangMeasured() string }); ok && hm.HangMeasured() != "" {
			_, _ = fmt.Fprintf(stderr, "run: quiesce probes %s\n", hm.HangMeasured())
		}

		// AND THE FAILURE IS SHRUNK, which is the line internal/shrink was
		// built for and never had. R6.4's first commit (fable, coop r6/146):
		// the shrinker was in no binary's dependency closure, so the whole
		// package, the replay written to feed it and the rollback trait were
		// built and unreached. A reader of a found bug wants the smallest trace
		// that still finds it, and a gate cannot drive a reduction the product
		// cannot perform.
		//
		// THE SESSION IS CLOSED FIRST, by the defer above this function's walk,
		// and the shrink opens its own against the PROVIDER's world -- which is
		// why this sits after the walk's output rather than beside it.
		//
		// A SHRINK REFUSAL DOES NOT OVERTURN THE FINDING, and that is a
		// decision with a reason: the run exits with the finding's status
		// either way, because a project's rollback declarations being wrong
		// does not make a found bug unfound, and a gate reading the refusal's 1
		// would lose a finding it exists to catch. The refusal prints, so the
		// reader is told why there is no minimal trace.
		if st, serr := shrinkFound(ctx, dir, prep, p, brep.Bindings.Probes, seed, rec, stdout); serr != nil {
			_, _ = fmt.Fprintf(stderr, "run: could not shrink: %v\n", serr)
			_ = st
		}
	}
	// THE SESSION COUNT IS IN THE VERDICT for the provider count's reason: a
	// token is acquired at first use and reused after, so a holder that
	// re-acquired every step would send identical requests and the difference
	// would be invisible to a reader. The number makes it readable.
	// AND THE OBSERVATION COUNT, for the same reason one layer on (R5.5): a
	// walk that observed NOTHING and a walk whose observations all agreed are
	// the same five lines of trace, and the first has asked the system no
	// question at all. A reader cannot tell them apart without this number.
	if _, err := fmt.Fprintf(stdout,
		"run: %s -- seed %d, %d step(s), %d handle(s), %d session(s), "+
			"%d observation(s); %d rollback provider(s) registered\n",
		filepath.Base(dir), rec.Seed, len(rec.Steps), len(rec.HandleSequence()),
		rec.Sessions, rec.Observations, len(rollback.Registered())); err != nil {
		_, _ = fmt.Fprintf(stderr, "run: writing the verdict: %v\n", err)
		return 2
	}
	// AND WHAT THE RUN DID WITH ITS PENDINGS, which fable ruled a line of the
	// product at coop r6/92 on rook's rumor/1071: clause 3 of the control is
	// "no trailing pending", and it had no reader outside this repository's own
	// tests. rook measured that exactly: the record carried PendingOpen and the
	// run never printed it, so "a trailing pending is computed and never leaves
	// the process" -- the gate had no instrument and only the author could see
	// it. A control clause whose only reader is a test is the dead-region shape
	// one layer out.
	//
	// IT IS PRINTED EVEN WHEN NOTHING WAS RECONCILED, because zero is the
	// answer the gate is checking for. A line that appeared only on a non-empty
	// reconciliation would make "clean" and "not printed" the same observation,
	// which is the absence-is-not-evidence shape this project keeps finding.
	//
	// THE VERDICTS ARE SORTED BY THE WALK and carry no clock, so this line is
	// byte-identical across two attempts of a seed as D1 requires.
	// "DID NOT RUN" AND "RAN, FOUND NOTHING" ARE NOW TWO SENTENCES, which is
	// D6's first half. rook drove it against v0.1.37 (coop rumor/1223) and
	// could not answer it: this read `nothing to reconcile` in THREE
	// different states -- no probes declared at all, probes that never read
	// quiet, and probes that read quiet over nothing pending -- so the only
	// evidence for the condition could not distinguish the condition.
	//
	// THE UNASKED CASE IS NAMED FIRST because it is the one a reader is most
	// likely to mistake for a clean result: a project with no [quiesce]
	// block reconciles nothing, and said so in the same words as a project
	// whose probes went quiet with nothing outstanding.
	//
	// AND THE NOT-QUIET CASE NAMES THE PROBES, from Prober.Hang(), which the
	// type documents as the clock-free half of its report and therefore safe
	// where the control reads; HangMeasured() is the half that must not come
	// here, and does not.
	reconciled := "every probe read quiet and nothing was pending"
	switch {
	case !rec.QuiesceAsked:
		reconciled = "no quiesce probes are declared, so nothing was asked and nothing reconciled"
	case !rec.QuiesceQuiet:
		reconciled = "did NOT reconcile: no probe read quiet"
		if rec.QuiesceProbes != "" {
			reconciled += " -- " + rec.QuiesceProbes
		}
	case len(rec.Reconciled) > 0:
		reconciled = strings.Join(rec.Reconciled, ", ")
	}
	if _, err := fmt.Fprintf(stdout, "run:   quiesce -- %d pending(s) open at the end; "+
		"reconciliation: %s\n", rec.PendingOpen, reconciled); err != nil {
		_, _ = fmt.Fprintf(stderr, "run: writing the quiesce line: %v\n", err)
		return 2
	}
	// AND EACH PENDING'S LIFE, which is what D1 and both of D3's clearing
	// paths read. rook's gate verdict (coop rumor/1223) drove them against
	// v0.1.37 and reached none: the walk computed a per-step verdict that can
	// say "pending", and the run printed a COUNT of what was left open and
	// nothing about what happened to the rest. "Held at step 3 and cleared"
	// and "never held one" were the same output, so the clearing paths had no
	// witness outside a debugger.
	//
	// ON STDOUT AND NOT A FILE, deliberately. A --out would put the evidence
	// somewhere the control does not hash, which is how PendingOpen came to
	// be computed and never read; these lines go in the stream the control
	// compares, so a change to them moves a digest and a reader is told.
	//
	// EACH LINE IS CLOCK-FREE AND THE SET IS SORTED, so D1's "two attempts of
	// one seed are byte-identical" holds over them.
	for _, pending := range rec.Pendings {
		if _, err := fmt.Fprintf(stdout, "run:   pending -- %s\n", pending); err != nil {
			_, _ = fmt.Fprintf(stderr, "run: writing a pending's line: %v\n", err)
			return 2
		}
	}
	// EXIT 3 AND NOT 1. A refusal (1) means the PROJECT is wrong and its
	// reader fixes their declarations; a finding means the SYSTEM UNDER TEST is
	// wrong and its reader fixes their application. Folding the product into
	// the refusal status would make those two indistinguishable to anything
	// reading the exit code, which is every CI step that will ever run this
	// verb -- and "your project is malformed" is the reading a green-seeking
	// pipeline would act on. could-not-run (2) still outranks both: when the
	// run could not finish, what an observation would have said is unknown.
	if rec.Finding != nil {
		return findingStatus
	}
	return 0
}

// parseRunArgs reads `run PROJECT [--seed N] [--steps N] [--profile NAME]`.
//
// THE PROFILE IS A NAME AND NOT A NUMBER, which is why it is parsed before the
// numeric pair rather than folded into it: platform.toml declares profiles and
// a weights table per profile, and the walk already takes one -- what was
// missing was any way for a reader to SELECT it. A row of R6's bug ledger says
// it is caught by `settling`, and until this flag existed that profile could be
// declared, validated and never run (fable, coop r6/92).
func parseRunArgs(args []string) (dir string, seed uint64, haveSeed bool, steps int,
	profile string, bad string) {

	rest := args
	for len(rest) > 0 {
		switch {
		case rest[0] == "--profile":
			if len(rest) < 2 {
				return "", 0, false, 0, "", "--profile wants a value"
			}
			// AN EMPTY NAME IS REFUSED rather than read as "the default",
			// because those are different requests and silence between them is
			// how a reader ends up measuring the wrong mix.
			if rest[1] == "" {
				return "", 0, false, 0, "", "--profile wants a name"
			}
			profile = rest[1]
			rest = rest[2:]
		case rest[0] == "--seed" || rest[0] == "--steps":
			if len(rest) < 2 {
				return "", 0, false, 0, "", rest[0] + " wants a value"
			}
			n, err := strconv.ParseUint(rest[1], 10, 64)
			if err != nil {
				return "", 0, false, 0, "", fmt.Sprintf("%s %s: wanted a non-negative integer", rest[0], rest[1])
			}
			if rest[0] == "--seed" {
				seed, haveSeed = n, true
			} else {
				if n == 0 {
					return "", 0, false, 0, "", "--steps 0: a walk of no steps is not a walk"
				}
				steps = int(n)
			}
			rest = rest[2:]
		case len(rest[0]) > 2 && rest[0][:2] == "--":
			return "", 0, false, 0, "", fmt.Sprintf("%s is not a flag of run", rest[0])
		default:
			if dir != "" {
				return "", 0, false, 0, "", "run takes one project directory"
			}
			dir = rest[0]
			rest = rest[1:]
		}
	}
	if dir == "" {
		return "", 0, false, 0, "", "run wants a project directory"
	}
	return dir, seed, haveSeed, steps, profile, ""
}
