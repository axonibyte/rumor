package schedule

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
	"forge.axonibyte.com/axonibyte/rumor/internal/handle"
	"forge.axonibyte.com/axonibyte/rumor/internal/ledger"
	"forge.axonibyte.com/axonibyte/rumor/internal/platform"
	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

// Project is what a walk needs from a project's documents, read once by the
// caller so the loop reads no files.
type Project struct {
	Statute string
	Actions []bindings.Action
	Weights platform.Weights
	Venire  []platform.Member
	Stage   string
	Profile string

	// Roles are the bindings' [[role]] entries, which decide whether an
	// actor's role holds the context an action needs. A project declaring none
	// gets no role gate; HoldsContext states what that does and does not cover.
	Roles []bindings.Role

	// Observations are the bindings' [[observation]] entries, which the walk
	// makes AFTER EVERY STEP (R5.5, issue 83; fable, coop r5/152). A project
	// declaring none is walked exactly as before, which is how every suite
	// written against R5.1-R5.4 keeps its meaning.
	Observations []bindings.Observation

	// Eventual is the claim types the project declares eventual, with their
	// bounds, and it is what makes a lagging observation a `pending`.
	//
	// WITHOUT IT THE BOUND WAS DECLARED AND INERT. The walk judged every
	// observation with engine.Evaluate, which takes no bindings and so never
	// reaches pendingOverlay -- so `timing = "eventual"` with `bound =
	// "quiesce"` had NO EFFECT on a run, and a system that had simply not
	// settled yet was reported as a `bug`. Measured at coop r6/77: the toy's
	// seed 1 stopped at a different step one run in five, because which bug the
	// walk met first depended on the worker's timing.
	//
	// A PROJECT DECLARING NONE IS WALKED AS BEFORE, which keeps every pre-R6.3
	// fixture's meaning: with no eventual type the overlay declines and Judge
	// returns exactly what Evaluate did.
	Eventual []engine.Eventual

	// Prober answers whether the system under test has settled, which is what
	// a `quiesce` bound is judged against.
	//
	// IT IS THE SLOT AND IT IS NIL UNTIL R6.2 WIRES ONE. R6.2 declares the
	// probes -- queue depth, worker idle, a `direct` query, each with its own
	// timeout -- and this is where their answer arrives. The engine judges the
	// answer and never asks for it.
	//
	// ITS NILNESS IS WHAT `run` REFUSES ON (fable, coop r6/12). The condition
	// is STRUCTURAL rather than a sentence about this build: a project
	// declaring a quiesce bound with no prober wired cannot have that bound
	// judged, so the run refuses by name instead of holding the disagreement
	// open and exiting 0. R6.2 falsifies the condition by construction the
	// moment it wires one in, so the shutter removes itself and nothing has to
	// remember to take it out.
	Prober Prober
}

// Prober answers whether the system under test has gone quiet.
//
// IT IS AN INTERFACE AND NOT A FUNCTION FIELD because R6.2's probes are
// several -- queue depth, worker idle, a `direct` query -- each with its own
// timeout, and the thing that combines them into one answer is theirs to shape.
type Prober interface {
	// Quiesce reports the system's settling state, or an error when it cannot
	// be determined. A prober that gave up answers QuiesceTimedOut rather than
	// an error: the engine treats a probe that ran and stopped as a finding of
	// its own kind, not as a failure to run.
	//
	// IT MAY WAIT, up to the bound the project declared. A caller that cannot
	// wait uses State.
	Quiesce(ctx context.Context) (engine.QuiesceState, error)

	// State is the reading at this moment, and it NEVER WAITS.
	//
	// THE WALK NEEDS BOTH, AND THEY ANSWER DIFFERENT QUESTIONS (R6.3). A check
	// taken mid-walk asks "is it settled YET?" -- fable's ruling at coop r6/73
	// is that a disagreement seen while the system is still working reads
	// `pending` and not `bug`, which is an answer about the moment. A check
	// that waited for the bound would serialise the run behind the system it
	// observes and turn every ordinary lag into a stall. Quiesce is for the
	// end, where waiting is the whole point.
	State(ctx context.Context) (engine.QuiesceState, error)
}

// Step is one step of a walk, RECORDED OVER HANDLES AND NEVER OVER IDS. That is
// the whole of issue 72's "the sequence is over handles, not ids": two attempts
// from one seed produce the same Steps while every id behind the handles
// differs.
type Step struct {
	N     int
	Actor string

	// Kind is the action name or nemesis kind drawn. EMPTY MEANS NOTHING WAS
	// APPLICABLE to the drawn actor, and the step is recorded rather than
	// dropped: the record is an account of the DRAWS, and one that silently
	// omitted a stuck actor would let two attempts' records agree while their
	// draws diverged.
	Kind string

	// Handles are the run-local handles this step minted, in creation order.
	Handles []string

	// Used are the handles this step CONSUMED as arguments, in the order the
	// action declares them.
	//
	// IT IS RECORDED FOR Handles' REASON RUN THE OTHER WAY: Handles is what the
	// step created, Used is what it was ABOUT. A choice that appears nowhere in
	// the record cannot be compared between two attempts, and cannot be
	// checked for being a choice at all -- a mutant replacing the draw in
	// chooseHandles with "take the first candidate" survived the entire suite
	// until this field existed, because nothing downstream could tell the two
	// apart.
	//
	// HANDLES AND NOT IDS, like everything else in a Step: two attempts from
	// one seed consume the same handles while the ids behind them differ.
	Used []string

	// Verdicts are what this step's observations were judged to be, in order,
	// each as engine.Finding.Says renders it: "consistent", "pending", "bug of
	// kind stale".
	//
	// WITHOUT IT A WALK THAT HELD A PENDING RECORDED NOTHING ABOUT IT. Observed
	// counts the observations and rec.Finding keeps only the one that STOPPED
	// the walk, and a `pending` stops nothing -- so a run that held a
	// disagreement open and a run where everything agreed left identical
	// records. fable's ruling 3 (coop r6/78) asks for an arm that reads
	// `pending` then `consistent` after the drain, and there was no surface to
	// read it from; R2.3's rule that a run ends with no pending open was
	// equally unmeasurable from the record.
	//
	// IT CARRIES NO REASON AND NO CLOCK. Says renders the value and the kind
	// and nothing else, which is three of the five fields fable's ruling 2
	// compares; an Overlay's Reason names positions and bounds, and the
	// duration form's names the run's time, so a record carrying reasons could
	// not be compared between two attempts.
	Verdicts []string

	// Planted says the step appended a planted claim -- a lie the engine knows
	// it told, because it told it.
	Planted bool

	// Observed is how many declared observations this step supported. It is a
	// COUNT and not the facts: the facts are in the ledger at this step's
	// position, and a record carrying them twice would be two answers to one
	// question. Zero is the ordinary case for a step whose subject no
	// observation declares.
	Observed int

	// Hop is the tell this step made, where it made one: the receiver and the
	// claim that travelled, as "guest_1 <- alice.event#1".
	//
	// IT IS HERE BECAUSE A TELLING STEP IS OTHERWISE INVISIBLE. `canard` and
	// `tell` mint no handle and move no system, so the trace printed
	// `member_1 tell []` -- the kind's name and nothing else. D15's reach half
	// asks for "an honest tell and a join by the TOLD actor" (fable, coop
	// r5/114), and that cannot be read off a trace which does not say who was
	// told. The same blindness hid canard's hop for the whole of R5.1.
	Hop string
}

// Record is one attempt's walk.
type Record struct {
	Seed    uint64
	Attempt int
	Steps   []Step

	// IDs is the attempt's handle table as it stood at the end: handle -> the
	// id the system gave it.
	//
	// IT IS HERE SO THE DIFFERENCE CAN BE SEEN. The record's Steps are over
	// handles and never over ids, which is the property issue 72's done-when
	// asserts -- and asserting it needs both halves: that two attempts' handle
	// sequences are EQUAL and that the ids behind them are NOT. A record
	// carrying only handles would make the second half unmeasurable from
	// outside the walk, and the arm would have to take it on trust.
	//
	// A handle a suborn minted is absent from this map, because nothing bound
	// it: the lie has no id under it.
	IDs map[string]string

	// Facts is the attempt's ledger as it stood at the end.
	//
	// IT IS HERE FOR Used's AND Sessions' REASON: what the walk RECORDED is
	// otherwise unobservable from outside, and a claim's `visible_to` -- the
	// field a statute reads and palter falsifies -- had no oracle at all. A
	// mutant dropping it from the appended fact survived every unit arm over
	// the function that computes it, because nothing downstream could see the
	// fact.
	//
	// IT IS A COPY, taken through Facts(), so a reader cannot append to the
	// attempt's ledger through the record it was handed.
	Facts []ledger.Fact

	// Observations is how many declared observations the attempt EVALUATED,
	// across every step. It is here for Sessions' reason: otherwise a walk that
	// observed NOTHING and a walk whose observations all agreed are the same
	// record, and the first is a project the engine never asked a question of.
	Observations int

	// Finding is the first `bug` an observation yielded, and FoundAt is the
	// step after which it did. Both zero means no observation found one.
	//
	// THE FIRST BUG ENDS THE WALK (fable, coop r5/152 ruling 2) and `consistent`
	// or `explained` walk on. It is held here rather than returned as an error
	// because a bug is the tool's PRODUCT and not a failure to run: the run did
	// exactly what it is for, and `rumor run` reports it and exits non-zero
	// while a could-not-run is a different thing entirely. It is also what
	// shrink.Request.Want takes, which is why the whole Finding is kept and not
	// its value.
	Finding *engine.Finding
	FoundAt int

	// Reconciled is what the end-of-run reconciliation judged, one entry per
	// pending it re-asked, each as engine.Finding.Says renders it and sorted so
	// two attempts of a seed cannot differ by map order.
	Reconciled []string

	// PendingOpen is how many findings the run still holds open for re-check
	// when it ends, read off the register after the last step AND after the
	// reconciliation -- so a run that ends with one is a run where re-asking
	// did not close it, which is the only case R2.3 forbids.
	//
	// IT EXISTS SO "NO TRAILING PENDING" CAN BE CHECKED AT ALL. docs/vocabulary
	// says a `stale` is terminal and "a run that ends on one leaves no pending
	// open", and fable's ruling 2 (coop r6/78) makes a trailing pending part of
	// 91's control. Neither was measurable: the register is local to the walk,
	// a `pending` stops nothing, and the record kept only the finding that
	// ENDED the run. A rule nothing can read is a rule the next change breaks
	// quietly.
	PendingOpen int

	// Sessions is how many `session` requests the attempt sent.
	//
	// IT IS HERE BECAUSE THE PROPERTY IS OTHERWISE UNOBSERVABLE. A token is
	// acquired at first use and reused after, and every token the fake adapter
	// issues is the same string -- so "held and reused" and "asked for again"
	// produce identical requests downstream, and a cache that never cached
	// would be invisible from outside the walk. The count is also what a
	// verdict reports, the way the registered-provider count is.
	Sessions int
}

// HandleSequence is the record's handles in order, flattened across steps. It
// is what two attempts are compared by.
func (r Record) HandleSequence() []string {
	var out []string
	for _, s := range r.Steps {
		out = append(out, s.Handles...)
	}
	return out
}

// KindSequence is the record's drawn kinds in order, a stalled step included as
// the empty string so the sequence counts draws and not successes.
func (r Record) KindSequence() []string {
	out := make([]string, 0, len(r.Steps))
	for _, s := range r.Steps {
		out = append(out, s.Kind)
	}
	return out
}

// Walk takes `steps` steps of a seeded walk against one adapter session.
//
// EVERYTHING DECIDED BY CHANCE DESCENDS FROM THE SEED, through the one
// generator this package holds: which actor moves next, which kind it draws,
// which action a lie is about, and which receiver a canard reaches. The session
// is handed one action at a time and never the generator, which is what makes a
// run reproducible from the seed, the weights, the bindings and the project's
// commit alone.
//
// THE LEDGER GROWS AND IS RESEALED EACH STEP, because applicability is
// evaluated against a sealed prefix: step n+1 must see what step n emitted, or
// a precondition about what an actor knows could never become true.
func Walk(ctx context.Context, p Project, sess *wire.Session, seed uint64, steps, attempt int) (Record, error) {
	rec := Record{Seed: seed, Attempt: attempt}
	if steps <= 0 {
		return rec, fmt.Errorf("schedule: a walk of %d steps is not a walk", steps)
	}
	if sess == nil {
		return rec, fmt.Errorf("schedule: no adapter session: a walk executes actions against the " +
			"system under test and cannot be taken against nothing")
	}
	actors, err := Actors(p.Venire)
	if err != nil {
		return rec, err
	}
	src := New(seed)
	l := ledger.New(false)
	table, err := handle.New(attempt)
	if err != nil {
		return rec, fmt.Errorf("schedule: the attempt's handle table: %w", err)
	}
	mint := &minter{counts: map[string]int{}}
	// ONE HOLDER FOR THE WHOLE ATTEMPT, so a token acquired at step 2 is still
	// held at step 9. It is per ATTEMPT and not per run: a second attempt is a
	// second world with its own ids, and reusing the first attempt's tokens
	// would carry one attempt's state into another.
	sessions := NewSessions(sess)
	// ONE REGISTER FOR THE WHOLE ATTEMPT, for the reason the holder is one: a
	// `pending` opened at step 2 is re-checked at the actor's next observation
	// of the same query, which is a fact about the attempt and not about a
	// step. A fresh register per step would re-open every pending and never
	// re-check one.
	reg := engine.NewRegister()
	// WHAT EACH PENDING CAME FROM, so the end-of-run reconciliation can re-ask
	// it. See ask.
	held := map[string]ask{}
	// stopped says a finding ended the walk, so the loop breaks instead of
	// returning and the end-of-run work below still runs.
	stopped := false

	for n := 1; n <= steps; n++ {
		// SEAL EVERYTHING SO FAR. The seal is the prefix a precondition is
		// evaluated against, so it moves to the end of what has happened
		// before the next step is chosen.
		if err := l.SealAt(l.Len()); err != nil {
			return rec, fmt.Errorf("schedule: step %d: sealing the prefix at %d: %w", n, l.Len(), err)
		}

		ai, err := src.IntN(len(actors))
		if err != nil {
			return rec, fmt.Errorf("schedule: step %d: drawing an actor: %w", n, err)
		}
		actor := actors[ai]

		// WHAT THIS ACTOR KNOWS, read ONCE for the step and handed to both the
		// pool and the step itself. Two reads of the same question would be two
		// answers that could differ -- and the pool offering an action whose
		// draw then refuses is precisely the defect this scoping closes, so the
		// one place it must not be reintroduced is here.
		known, err := engine.KnownHandlesAt(l, actor)
		if err != nil {
			return rec, fmt.Errorf("schedule: step %d: the handles %s knows: %w", n, actor, err)
		}

		pool, err := Pool(ctx, p.Statute, p.Actions, l, actor, p.Weights, p.Stage, p.Profile, table,
			p.Roles, known, actors)
		if err != nil {
			return rec, fmt.Errorf("schedule: step %d: the pool for %s: %w", n, actor, err)
		}
		if len(pool) == 0 {
			rec.Steps = append(rec.Steps, Step{N: n, Actor: actor})
			continue
		}
		kind, err := src.Pick(pool)
		if err != nil {
			return rec, fmt.Errorf("schedule: step %d: drawing from %s's pool: %w", n, actor, err)
		}
		// NOTHING DRAWABLE IS A NO-OP STEP, not a fault (fable, coop r5/121).
		// Pick drops zero-weight candidates before asking, so a pool that was
		// empty and a pool whose every candidate the project weighted zero
		// arrive here identically, and both are a step the walk records as
		// having taken nothing.
		if kind == "" {
			rec.Steps = append(rec.Steps, Step{N: n, Actor: actor})
			continue
		}

		step := Step{N: n, Actor: actor, Kind: kind}
		if err := take(ctx, p, sess, sessions, l, table, mint, src, known, &step); err != nil {
			return rec, fmt.Errorf("schedule: step %d: %s taking %s: %w", n, actor, kind, err)
		}
		// AND THE WALK OBSERVES, after every step (fable, coop r5/152). The
		// facts go in the ledger at this step's position; nothing is drawn, so
		// the step above replays identically with or without this call.
		// THE READING IS TAKEN ONCE PER STEP AND NOT ONCE PER OBSERVATION: the
		// step's checks are judged against one answer about the system, and
		// asking the probes between two observations of the same step would let
		// them disagree about a moment they share.
		qs := engine.QuiesceAbsent
		if p.Prober != nil && len(p.Eventual) > 0 {
			st, qerr := p.Prober.State(ctx)
			if qerr != nil {
				return rec, fmt.Errorf("schedule: reading the probes after step %d: %w", n, qerr)
			}
			qs = st
		}
		seen, oerr := observe(ctx, p.Statute, sess, sessions, table, p.Observations, l, &step,
			engine.Bindings{Eventual: p.Eventual}, reg, qs, held)
		if oerr != nil {
			return rec, fmt.Errorf("schedule: step %d: %w", n, oerr)
		}
		step.Observed = len(seen)
		for i := range seen {
			step.Verdicts = append(step.Verdicts, seen[i].Says())
		}
		rec.Observations += len(seen)
		rec.Steps = append(rec.Steps, step)

		// THE FIRST FINDING THAT STOPS A WALK ENDS IT -- a `bug`, or R6.2's
		// `quiesce_timeout`. Checked after the step is recorded, so the
		// trace a shrink reads includes the step that produced the finding --
		// without it the reduction would search a world one step short of the
		// one that broke.
		//
		// AND IT BREAKS RATHER THAN RETURNING, which is what makes clause 3 of
		// R6's control true of EVERY run. It returned here, so a run that
		// stopped on a finding never reached the end-of-run quiesce or the
		// reconciliation -- and seed 43 under the `settling` profile printed
		// the contradiction out loud: "1 pending(s) open at the end;
		// reconciliation: nothing to reconcile". A run that ends holding one is
		// what R2.3 forbids, and a finding earlier in the walk does not excuse
		// it: the run ended, so the register is answered for.
		//
		// THE FINDING IS NOT OVERWRITTEN by anything the reconciliation reads
		// (see below), so the verdict a shrink reduces is still the first one
		// the walk found.
		for i := range seen {
			if stopsTheWalk(seen[i].Value) {
				f := seen[i]
				rec.Finding = &f
				rec.FoundAt = n
				stopped = true
				break
			}
		}
		if stopped {
			break
		}
	}

	// THE WALK ASKS WHETHER THE SYSTEM SETTLED, AND A PROBE THAT GAVE UP IS A
	// FINDING OF THE RUN (R6.2, fable's ruling 2 at coop r6/46).
	//
	// WHY HERE AND NOT BESIDE A PENDING: "a probe that gives up while every
	// observation agrees is the fifth finding ALL THE SAME, and it is produced
	// by the thing that polls the probes, not by the overlay, which judges
	// findings". So this is asked after the last step whatever the walk found,
	// and it fires with NO PENDING OPEN -- which is the half
	// internal/engine.pendingOverlay cannot reach, because it returns before it
	// looks when the statute found agreement.
	//
	// AND ONLY WHEN A PROBER IS WIRED. A project declaring no probes is walked
	// exactly as before, which is what keeps every pre-R6.2 fixture's meaning;
	// cmd/rumor refuses a quiesce BOUND with no probe separately, and that is a
	// statement about the project rather than about this walk.
	//
	// A PROBER THAT COULD NOT BE ASKED IS AN ERROR AND NOT A FINDING, the
	// distinction internal/quiesce draws at its own boundary: the adapter not
	// answering is could-not-run, where a probe that ran and stopped is a thing
	// the run FOUND.
	if p.Prober != nil {
		state, qerr := p.Prober.Quiesce(ctx)
		if qerr != nil {
			return rec, fmt.Errorf("schedule: asking whether the system settled: %w", qerr)
		}
		// A TIMEOUT DOES NOT OVERWRITE A FINDING THE WALK ALREADY HAS, and the
		// guard arrived with the break above: while a finding RETURNED from the
		// loop, this block could only ever run on a clean walk, so the question
		// never came up. Now that every run reaches it, the first finding is
		// still the verdict -- it is the one a shrink reduces and the one whose
		// trace is recorded, and a probe giving up afterwards is a second thing
		// the run found rather than a correction of the first.
		if state == engine.QuiesceTimedOut && rec.Finding == nil {
			reason := "the declared quiesce probes did not read quiet within their timeouts"
			if h, ok := p.Prober.(interface{ Hang() string }); ok && h.Hang() != "" {
				reason = h.Hang()
			}
			rec.Finding = &engine.Finding{
				Value:   engine.FindingQuiesceTimedOut,
				Defined: true,
				// THE RULE IS THE OVERLAY'S AND NOT EMPTY. cmd/rumor's FOUND
				// line prints Finding.Rule, and with it unset the line read
				// "FOUND quiesce_timeout after step 3 -- , rests_on_planted
				// undeclared" -- a finding naming no rule at all, which is the
				// one field docs/design.md makes part of a finding's identity.
				Rule: "engine.quiesce_timeout",
				Overlay: &engine.Overlay{
					Rule:   "engine.quiesce_timeout",
					Value:  engine.FindingQuiesceTimedOut,
					Reason: reason,
				},
			}
			rec.FoundAt = len(rec.Steps)
		}

		// AND THEN THE RECONCILIATION THAT FOLLOWS, which is
		// docs/vocabulary.md's own phrase for this: the quiesce form is
		// "`pending` until the declared probes read quiet and `stale` if the
		// reconciliation that follows still disagrees". Until it existed the
		// second half of that sentence had nowhere to happen -- a `pending` is
		// re-checked "at the actor's next observation of the same query", and
		// for a pending opened at the last step there is no next observation,
		// so seeds 1 and 3 ended with one held open (ibis, coop r6/84) and
		// R2.3's "a run that ends on one leaves no pending open" was false.
		//
		// IT RE-ASKS EXACTLY WHAT IS OPEN, from `held`: the register keys a
		// pending by (actor, query) and engine.Check carries no subject, so
		// the walk is the only thing that knows which observation to re-issue.
		// A full pass over every declared observation would also re-check
		// them, and would ask questions about subjects no pending was about.
		//
		// THE FRESH QUIESCE ANSWER IS WHAT MAKES IT A VERDICT: the probes have
		// just been asked, so a disagreement that survives here is a pending
		// past its bound -- `stale`, decided by engine.Judge where the
		// re-check is in hand (fable, coop r6/87) -- and one that has resolved
		// reads `consistent` and clears.
		if len(held) > 0 && reg.Len() > 0 {
			open := reg.Open()
			for actor, queries := range open {
				for _, query := range queries {
					a, ok := held[askKey(actor, query)]
					if !ok {
						// A PENDING NOTHING REMEMBERED, NAMED RATHER THAN
						// SKIPPED: it would otherwise sit open to the end of
						// the run and be reported as clean, which is the
						// outcome this whole arm exists to prevent.
						return rec, fmt.Errorf("schedule: a %q pending is open for %s and the "+
							"walk did not record what produced it, so it cannot be re-checked",
							query, actor)
					}
					f, made, oerr := observeOne(ctx, p.Statute, sess, sessions, table, a.decl,
						a.chosen, a.actor, l, engine.Bindings{Eventual: p.Eventual}, reg, state,
						"reconciling after the last step")
					if oerr != nil {
						return rec, oerr
					}
					if !made {
						continue
					}
					rec.Reconciled = append(rec.Reconciled, f.Says())
					// A FINDING THE RECONCILIATION PRODUCES ENDS THE RUN LIKE
					// ANY OTHER, and it does not overwrite one the walk
					// already has: the earlier finding is what a shrink
					// reduces, and the trace that reaches it is recorded.
					if stopsTheWalk(f.Value) && rec.Finding == nil {
						found := f
						rec.Finding = &found
						rec.FoundAt = len(rec.Steps)
						rec.Sessions = sessions.Acquired()
						rec.Facts = l.Facts()
					}
				}
			}
			sort.Strings(rec.Reconciled)
		}
	}

	rec.Sessions = sessions.Acquired()
	rec.Facts = l.Facts()
	rec.PendingOpen = reg.Len()
	rec.IDs = map[string]string{}
	for _, h := range table.Handles() {
		if id, ok := table.ID(h); ok {
			rec.IDs[h] = id
		}
	}
	return rec, nil
}

// observe makes every declared observation this step can support, EVALUATES
// each as a check against the sealed prefix, and returns the findings.
//
// NOTHING IS APPENDED, which is the ruling's amendment and not a detail. My
// first cut wrote each observation into the ledger as an `observed` fact at the
// step's position, which is what r5/152 said -- and it contradicts a rule fable
// had RATIFIED themselves at breakout-r2/13 and which both docs state: "the
// observation is the check's INPUT while the sealed prefix is the DATA ... once
// the observation is not counted as part of the prefix" (vocabulary v1,
// amended 2026-10-15; design.md:691). Measured beside the documents: zero
// `observed` facts in any of the seventeen committed ledgers, and
// ledger.KindObserved has validation and has never had a producer. The ledger
// refused the append outright -- "the engine never writes a fact without one"
// -- because a `direct` observation is nobody's view and has no actor to name.
// Raised rather than implemented through; amended at r5/155.
//
// AFTER EVERY STEP, ruled by fable at coop r5/152. Not periodically and not at
// the end: a count that disagrees with the store disagrees at a POSITION, and a
// walk that observed only at the end could say a run is wrong without saying
// when it became wrong -- which is the whole of what a shrink then has to
// search for. A selective WHEN (settling, periodic reconciliation) is R8's
// api-sim and deliberately not this.
//
// NOTHING IS DRAWN, which is the ruling's own clause and the reason D1 still
// holds: the handle sequence and the kind sequence are what they were before
// this existed, so every seeded walk replays identically and R5.1's arms keep
// their meaning. An observation needing a `{kind}` therefore takes it from THE
// STEP'S OWN HANDLES -- the minted one first, then what the step consumed,
// which is `claimHandles`' list read for a second purpose -- and an observation
// whose kind is not among them is simply not made at this step.
//
// THAT CHOICE IS MINE AND NOT THE RULING'S, so it is named here rather than
// buried: fable settled WHEN, EVALUATION and REPLAY (r5/152) and not which
// handle an argument resolves to. The alternatives were a draw (forbidden by
// the ruling, and it would move every seeded walk), every bound handle of the
// kind (an observation count per step that grows with the table, and a walk
// that reads about entities the step never touched), or the newest of the kind
// (the same answer as this one wherever the step minted one, and an arbitrary
// one wherever it did not). The step's own subject is the one that answers "did
// what just happened leave the system consistent".
//
// A SUBORN'S HANDLE IS UNBOUND AND SO ITS STEP OBSERVES NOTHING. ObserveArgs
// resolves through handle.Table.Require, which refuses a handle with no id --
// which is correct rather than unfortunate: the lie named an entity the system
// never created, so there is nothing to ask the wire about, and asking would be
// the "never ask the wire for a thing that does not exist" rule broken by a new
// door.
func observe(ctx context.Context, statute string, sess *wire.Session, sessions *Sessions,
	table *handle.Table, obs []bindings.Observation, l *ledger.Ledger,
	step *Step, eb engine.Bindings, reg *engine.Register,
	qs engine.QuiesceState, held map[string]ask) ([]engine.Finding, error) {

	if len(obs) == 0 {
		return nil, nil
	}
	// THE PREFIX IS SEALED FIRST, which is what "against the sealed prefix"
	// requires and what the vocabulary states: "A ledger is sealed at a
	// position before any check runs; the check reads the sealed prefix."
	//
	// THE ATTRIBUTION HERE WAS WRONG AND tests/vocabulary CAUGHT IT. I first
	// credited the document with "seals a prefix before any check runs, so this
	// is a sequencing error and not an empty result" -- which is engine.Input's
	// own REFUSAL TEXT and not a sentence the vocabulary contains. The sweep
	// reported 32 quotes against a declared 31 and named the file and line. It
	// is the second time I have credited this document with an error message,
	// and the sweep is why neither reached a reader.
	//
	// engine.Input refuses an unsealed ledger outright, and the walk's own seal
	// happens at the TOP of a step, so everything this step just appended is
	// unsealed when we get here. Sealing to Len() makes the check's world
	// exactly "everything that has happened, including this step", which is the
	// only position at which "did what just happened leave the system
	// consistent" is a question.
	if err := l.SealAt(l.Len()); err != nil {
		return nil, fmt.Errorf("sealing the prefix before observing after step %d: %w", step.N, err)
	}

	var made []engine.Finding
	for _, decl := range obs {
		chosen, ok := observeHandles(decl, table, step)
		if !ok {
			continue
		}
		found, made1, oerr := observeOne(ctx, statute, sess, sessions, table, decl, chosen,
			step.Actor, l, eb, reg, qs, fmt.Sprintf("after step %d", step.N))
		if oerr != nil {
			return nil, oerr
		}
		if !made1 {
			continue
		}
		if found.Value == "pending" && held != nil {
			// WHAT PRODUCED THIS PENDING, REMEMBERED SO IT CAN BE RE-ASKED.
			// The register keys a pending by (actor, query) alone and
			// engine.Check carries no subject, so nothing in the engine can
			// re-issue the observation this came from -- and "a `pending` is
			// re-checked at the actor's next observation of the same query"
			// (docs/vocabulary.md) needs that observation to be MAKEABLE at
			// the end of a run, when no step is supplying handles.
			held[askKey(step.Actor, decl.Name)] = ask{decl: decl, chosen: chosen, actor: step.Actor}
		}
		made = append(made, found)
	}
	return made, nil
}

// ask is one observation the walk can re-issue: the declaration, the handles it
// was given, and the actor who made it.
type ask struct {
	decl   bindings.Observation
	chosen map[string]string
	actor  string
}

func askKey(actor, query string) string { return actor + "\x00" + query }

// observeOne makes ONE declared observation and judges it. It is the body the
// step loop and the end-of-run reconciliation share, so neither can judge by a
// route the other does not -- two paths running statutes would be free to
// disagree, which is the reason this package has one evaluator at all.
func observeOne(ctx context.Context, statute string, sess *wire.Session, sessions *Sessions,
	table *handle.Table, decl bindings.Observation, chosen map[string]string, actor string,
	l *ledger.Ledger, eb engine.Bindings, reg *engine.Register, qs engine.QuiesceState,
	when string) (engine.Finding, bool, error) {

	args, err := ObserveArgs(decl, chosen, table)
	if err != nil {
		// A HANDLE THAT WILL NOT RESOLVE IS A SKIP AND NOT A REFUSAL, for
		// the suborn reason above: the step named something unbound, so
		// this observation has no subject at this position.
		return engine.Finding{}, false, nil
	}

	// THE TOKEN IS THE SOURCE'S, which is the ruling's second clause: an
	// `api` observation goes through the acting actor's own session, and a
	// `direct` one is the engine's own and carries none.
	var token string
	if decl.Source == wire.SourceAPI || decl.Source == wire.SourceUI {
		t, terr := sessions.Token(actor, string(decl.Source))
		if terr != nil {
			// THE ACTOR CANNOT MAKE IT, which is "every declared
			// observation whose actor CAN make it". An actor with no
			// session in that context observes nothing through it, and
			// that is the project's role model answering rather than a
			// fault.
			return engine.Finding{}, false, nil
		}
		token = t
	}

	resp, err := sess.Observe(wire.ObserveRequest{
		Query: decl.Name, Source: decl.Source, Token: token, Args: args,
	})
	if err != nil {
		return engine.Finding{}, false, fmt.Errorf("observing %s (%s) %s: %w",
			decl.Name, decl.Source, when, err)
	}

	// THE OBSERVER IS THE STEP'S ACTOR FOR api AND THE ENGINE FOR direct,
	// which is what `role` then reads off: a direct observation bears no
	// role, because it is nobody's view (engine.Input's own rule) and a
	// statute's role_sees_subject reads its ABSENCE rather than a blank.
	check := engine.Check{
		Query:  decl.Name,
		Of:     decl.Of,
		Source: ledger.Source(decl.Source),
		Value:  resp.Value,
	}
	if decl.Source != wire.SourceDirect {
		check.Actor = actor
		if role, rok := RoleOf(actor); rok {
			check.Role = role
		}
	} else {
		// A CHECK ALWAYS NAMES AN ACTOR -- engine.Input refuses one that
		// does not, because "a finding is always some actor's". For a
		// direct observation that actor is the one whose step produced the
		// subject: the engine asks the store about what this actor just
		// did. The ROLE is still omitted, which is the half that decides
		// what a statute may see.
		check.Actor = actor
	}

	// THE ONE EVALUATOR, as a case's check goes through: a second path
	// running statutes would be free to disagree, and a walk and a case
	// contradicting each other about one world is what D14 exists to catch.
	// THE ATTRIBUTION IS ASKED FOR ALONGSIDE THE FINDING, because it is
	// part of the finding's identity and not a second question: `bug`
	// resting on a planted claim is a different finding from `bug` resting
	// on none (docs/design.md), and a caller handed only the value cannot
	// tell them apart. Naming a rule the statute does not declare is not an
	// error -- Rules simply carries no entry for it -- so a project with no
	// planted cases is unaffected.
	// AND THE QUIESCE READING, which is what makes an eventual bound mean
	// anything mid-walk: pendingOverlay refuses QuiesceAbsent rather than
	// reading silence as "not quiet", so a project with a quiesce bound and
	// no prober is a refusal and not a quiet run (ratchet, coop r6/8).
	check.Quiesce = qs

	// JUDGE AND NOT Evaluate, because Evaluate takes no bindings and so can
	// never reach the overlay. That was the defect: `bound = "quiesce"`
	// parsed, validated, and did nothing to a walk.
	found, err := engine.Judge(ctx, statute,
		[]string{engine.FindingRule, engine.AttributionRule}, l, check, eb, reg)
	if err != nil {
		return engine.Finding{}, false, fmt.Errorf("judging %s (%s) %s: %w",
			decl.Name, decl.Source, when, err)
	}
	return found, true, nil
}

// observeHandles resolves an observation's declared argument names against THE
// STEP'S OWN handles, by kind, drawing nothing.
//
// THE SECOND RETURN IS "THIS STEP CAN SUPPORT IT", not an error: an observation
// the step has no subject for is not a fault, it is an observation for another
// step.
func observeHandles(decl bindings.Observation, table *handle.Table,
	step *Step) (map[string]string, bool) {

	if len(decl.Args) == 0 {
		return nil, true
	}
	if table == nil {
		return nil, false
	}
	// MINTED FIRST, THEN CONSUMED, which is claimHandles' order read for a
	// second purpose: a join's own handle before the event it joined, so an
	// observation declaring `event` takes the event and never the join.
	candidates := make([]string, 0, len(step.Handles)+len(step.Used))
	candidates = append(candidates, step.Handles...)
	candidates = append(candidates, step.Used...)
	chosen := map[string]string{}
	for _, name := range decl.Args {
		found := ""
		for _, h := range candidates {
			if kind, ok := handleKind(h); ok && kind == name {
				found = h
				break
			}
		}
		if found == "" {
			return nil, false
		}
		chosen[name] = found
	}
	return chosen, true
}

// findingBug is the first of the two finding values a walk stops on. The values
// are docs/vocabulary.md's: `bug` means the system and the statute disagree,
// `consistent` and `explained` are a walk continuing, and `pending` is R9's.
const findingBug = "bug"

// stopsTheWalk says whether a finding value ends the walk.
//
// TWO VALUES NOW, AND THE SECOND IS NOT A BUG. R6.2's `quiesce_timeout` is a
// finding of its own -- the system would not go quiet -- so it ends a walk for
// the same reason a bug does: the run has something to report and continuing
// would search past it. It is NOT folded into findingBug, because every reader
// keying on `bug` must not see one here (fable's D7, coop breakout-r6/0).
//
// WHY A FUNCTION AND NOT A SECOND COMPARISON AT THE CALL SITE: the check sat
// inline as `v == findingBug`, and a value added to the vocabulary with no
// single place to add it is a value some paths stop on and others walk past.
func stopsTheWalk(v any) bool {
	s, ok := v.(string)
	return ok && (s == findingBug || s == engine.FindingQuiesceTimedOut)
}

// take performs one drawn kind: the honest action, or the lie.
func take(ctx context.Context, p Project, sess *wire.Session, sessions *Sessions,
	l *ledger.Ledger, table *handle.Table, mint *minter, src *Source, known []string,
	step *Step) error {

	switch step.Kind {
	case Canard:
		return canard(l, src, p, step)
	case Tell:
		return tell(l, src, p, step)
	case Suborn:
		return suborn(ctx, p, l, table, mint, src, known, step)
	case Palter:
		// IMPLEMENTED IN R5.4, because that is when the field arrived. The
		// refusal that stood here said palter "needs a claim field to falsify
		// and nothing in R5.1 populates one" -- true then, and R5.4's
		// `visible_to` argument is that field. ibis hit the refusal from the
		// other end driving the toy (coop r5/88): seed 4242 reaches step 3 and
		// step 3 draws palter.
		return palter(ctx, p, sess, sessions, l, table, mint, src, known, step)
	case Essoin:
		// STILL UNREACHABLE AND STILL A REFUSAL rather than a skip: fable ruled
		// at coop r5/58 that a drawn kind whose effect is unimplemented is
		// refused BY NAME, because a skip is the silent-drop class. essoin
		// needs R9's flow state, so it keeps a declared weight and is never
		// drawn until R9 gives it something to abandon.
		return fmt.Errorf("the %s effect is not implemented before R9, and a drawn kind is refused "+
			"rather than skipped: a skip would drop a draw silently", step.Kind)
	}

	a, ok := actionNamed(p.Actions, step.Kind)
	if !ok {
		return fmt.Errorf("%q is neither a declared action nor a nemesis kind, and the pool it "+
			"came from is built from those two sets alone", step.Kind)
	}
	return honest(ctx, sess, sessions, l, table, mint, src, a, known, step)
}

// honest executes the action against the system under test and records what it
// emitted.
// performed is what executing one action left behind, for the two kinds that
// execute: the args that actually went out, the handle minted for the claim,
// and whether a claim is owed at all.
type performed struct {
	Args   map[string]any
	Handle string
	Emits  bool
}

// perform executes an action and does everything that is true of BOTH an honest
// step and a palter: compose the args, acquire the token, send the execute,
// mint the claim's handle and bind it to the id the system gave.
//
// IT EXISTS BECAUSE PALTER EXECUTES THE SAME WAY honest DOES. design.md:
// "`palter` EXECUTES the honest action and then appends its claim with exactly
// one recorded field false". The only difference is the claim, so the execution
// is one function and the two callers differ in what they record -- a second
// copy would be free to drift in the token, the arg order or the binding, which
// are exactly the parts a reader would assume identical.
func perform(ctx context.Context, sess *wire.Session, sessions *Sessions,
	table *handle.Table, mint *minter, src *Source, a bindings.Action, known []string,
	step *Step) (performed, error) {
	_ = ctx
	var out performed

	// THE HANDLE CHOICES ARE DRAWN FIRST AND THE REST COMPOSED AFTER, and that
	// order is fixed rather than incidental: both consume the generator, so a
	// run that composed in the other order would walk differently from the same
	// seed. Stated here because it is the kind of thing a later edit reorders
	// without noticing.
	chosen, err := chooseHandles(a, src, table, known)
	if err != nil {
		return out, err
	}
	args, err := ExecuteArgs(a, step.Actor, chosen, src, table)
	if err != nil {
		return out, err
	}
	out.Args = args
	// RECORDED IN DECLARATION ORDER, read off the declaration rather than off
	// the map: a map's order is its own, and a record whose order moved between
	// two attempts would fail the comparison for a reason that is not a
	// difference.
	for _, arg := range a.Args {
		if arg.Source == bindings.ArgHandle {
			step.Used = append(step.Used, chosen[arg.Name])
		}
	}
	// THE TOKEN IS ACQUIRED FOR THIS ACTION'S CONTEXT, which is the engine-side
	// half of the pairing internal/wire's semantics() declines to check. An
	// action whose context is "none" gets the empty token and omitempty leaves
	// the field out.
	token, err := sessions.Token(step.Actor, a.Context)
	if err != nil {
		return out, err
	}

	resp, err := sess.Execute(wire.ExecuteRequest{Action: a.Name, Token: token, Args: args})
	if err != nil {
		return out, fmt.Errorf("executing %s: %w", a.Name, err)
	}

	// AN ACTION THAT EMITS NOTHING MINTS NOTHING, and this is checked BEFORE
	// the handle kind is asked for. fable ruled at coop r5/62 that such a step
	// "appends no claim and mints no handle, recorded as a step" -- so the
	// handle-kind question never arises for it, and mintKind's refusal of an
	// action declaring two `creates` must not reach a step that was never
	// going to mint one. My first version asked in the other order and would
	// have refused a recorded-and-harmless step.
	if len(a.Emits) == 0 {
		return out, nil
	}
	out.Emits = true

	kind, err := mintKind(a)
	if err != nil {
		return out, err
	}
	h := mint.next(step.Actor, kind)
	out.Handle = h
	// THE HANDLE IS BOUND ONLY WHERE THE SYSTEM GAVE AN ID FOR WHAT IT
	// CREATED. An action with no `creates` has no entity behind its claim --
	// the toy's join_event is a claim about having joined, not a new thing --
	// so its handle is minted and left unbound, and anything needing it to
	// resolve is inapplicable. That is the same mechanism a suborn uses, which
	// is why there is one of it.
	if len(a.Creates) == 1 {
		id, ok := resp.IDs[a.Creates[0]]
		if !ok {
			// THE ADAPTER'S OWN ANSWER GOES IN THE REFUSAL, because `ids {}` has
			// more than one cause and they need different fixes. ibis drove both
			// over the wire (coop r5/64): no session is 401, no args is 400, and
			// both come back with an empty ids map. A refusal naming only the
			// empty map would send a reader looking at the wrong half.
			return out, fmt.Errorf("%s declares it creates %q and the adapter's execute answered ids %v, "+
				"which carries no id for it -- the handle would resolve to nothing and a later step "+
				"needing it could not say why. The adapter's response was %v; an empty ids map has "+
				"more than one cause, so read that rather than guessing (401 is no session, 400 is "+
				"no args, and a walk sends neither today)", a.Name, a.Creates[0], resp.IDs, resp.Response)
		}
		str, ok := id.(string)
		if !ok {
			return out, fmt.Errorf("%s's %q id came back as %T (%v) and a handle resolves to a string id",
				a.Name, a.Creates[0], id, id)
		}
		if err := table.Bind(h, str); err != nil {
			return out, fmt.Errorf("binding %s to the id the adapter gave: %w", h, err)
		}
	}
	return out, nil
}

// honest executes the action against the system under test and records what it
// emitted, truthfully.
func honest(ctx context.Context, sess *wire.Session, sessions *Sessions, l *ledger.Ledger,
	table *handle.Table, mint *minter, src *Source, a bindings.Action, known []string,
	step *Step) error {
	done, err := perform(ctx, sess, sessions, table, mint, src, a, known, step)
	if err != nil {
		return err
	}
	if !done.Emits {
		return nil
	}
	seen, err := visibleTo(a, done.Args)
	if err != nil {
		return err
	}
	if _, err := l.Append(ledger.Fact{
		Kind:      ledger.KindClaim,
		Actor:     step.Actor,
		Ref:       done.Handle,
		Type:      a.Emits[0],
		Handles:   claimHandles(done.Handle, step.Used),
		VisibleTo: seen,
	}); err != nil {
		return fmt.Errorf("recording %s's %s claim: %w", step.Actor, a.Emits[0], err)
	}
	step.Handles = append(step.Handles, done.Handle)
	return nil
}

// suborn plants a claim about an action that never ran.
//
// DO NOT EXECUTE, ruled at coop r5/58: a suborn is a description with no action
// under it. The handles are minted as if the action had run and are NEVER
// BOUND, so they resolve to no id and any later step needing one is
// inapplicable -- the walk never asks the wire for a thing that does not exist.
func suborn(ctx context.Context, p Project, l *ledger.Ledger, table *handle.Table, mint *minter,
	src *Source, known []string, step *Step) error {
	a, err := lieAbout(ctx, p, l, src, step.Actor, table, known, nil)
	if err != nil {
		return err
	}
	kind, err := mintKind(a)
	if err != nil {
		return err
	}
	h := mint.next(step.Actor, kind)

	// THE PLANTED CLAIM CARRIES THE HANDLES THE HONEST ONE WOULD, ruled by
	// fable at coop r5/119 after ratchet named the gap in my own claimHandles
	// comment: "a statute must be able to group a lie by entity exactly as it
	// groups the truth, and a lie distinguishable from the truth by its shape
	// is not a lie the control can test".
	//
	// SO THE HANDLES ARE DRAWN, WHICH SPENDS THE GENERATOR. I raised this as
	// the reason it was a ruling and not a line: a suborn of an action with a
	// `handle` argument now draws that handle, so every walk from every seed
	// moves. That is the cost the ruling accepted, and it is why this was not
	// mine to take quietly.
	//
	// step.Used IS DELIBERATELY NOT SET. The trace prints Used as `<- [...]`,
	// meaning CONSUMED, and a suborn consumed nothing -- the action never ran.
	// The claim carries the handles; the step does not claim to have used them.
	chosen, err := chooseHandles(a, src, table, known)
	if err != nil {
		return fmt.Errorf("the handles %s's lie about %s would have named: %w", step.Actor, a.Name, err)
	}
	var would []string
	for _, arg := range a.Args {
		if arg.Source == bindings.ArgHandle {
			would = append(would, chosen[arg.Name])
		}
	}
	if _, err := l.Append(ledger.Fact{
		Kind:    ledger.KindClaim,
		Actor:   step.Actor,
		Ref:     h,
		Type:    a.Emits[0],
		Planted: true,
		Handles: claimHandles(h, would),
	}); err != nil {
		return fmt.Errorf("planting %s's %s claim: %w", step.Actor, a.Emits[0], err)
	}
	step.Handles = append(step.Handles, h)
	step.Planted = true
	return nil
}

// canard passes a planted claim on to somebody who does not know it.
//
// THE SYSTEM NEVER MOVES: a canard is a tell, so it appends a `told` and
// executes nothing. The claim is one the teller knows and did not author --
// engine.CanardSubjects answers exactly that -- and the receiver is drawn from
// the actors who do not hold it yet, because telling somebody what they already
// know passes nothing on.
// canard passes on a LIE: a planted claim the actor knows and did not author.
func canard(l *ledger.Ledger, src *Source, p Project, step *Step) error {
	subjects, err := engine.CanardSubjects(l, step.Actor)
	if err != nil {
		return err
	}
	if len(subjects) == 0 {
		return fmt.Errorf("canard was drawn for %s with no planted claim it knows and did not "+
			"author: its precondition held when the pool was built and does not now", step.Actor)
	}
	// A CANARD IS TAGGED, because the run planted what travelled and the
	// control in the vocabulary reads that tag.
	return pass(l, src, p, step, subjects, "planted claim", true)
}

// tell passes on a CLAIM: an honest one the actor knows, authored by anybody
// including itself.
//
// IT IS THE MIRROR OF canard AND SHARES ITS APPEND, which is fable's own
// instruction at coop r5/114 -- "canard's append is the code path, with an
// honest subject and no tag". The two differ in the subject they draw from and
// in whether the hop is tagged, and in nothing else; a second copy of the
// receiver draw would be free to disagree about who still needs telling.
//
// WHY THE KIND EXISTS AT ALL, measured. Before it, the walk's ONLY
// ledger.KindTold append was canard's, so a lie could travel and an honest
// claim never could -- an actor could know only what it had made itself. I
// claimed from that that a join by a second actor was unreachable (coop r5/106)
// and was wrong about the count: the walk reached one anyway, through a handle
// draw that was not scoped to knowledge (ratchet r5/107, ibis r5/111). With
// that draw now scoped, the claim is true again by construction -- seed 6's
// step 8 became a self-join -- so the kind is what makes a told actor able to
// act on what it was told, and what lets a walk produce an `explained` finding
// at all.
//
// IT IS HONEST, so it is weighted in the honest band and is NOT a nemesis kind:
// NemesisKinds stays the four docs/design.md names in one breath, which
// tests/validate pins to that sentence.
func tell(l *ledger.Ledger, src *Source, p Project, step *Step) error {
	subjects, err := engine.TellSubjects(l, step.Actor)
	if err != nil {
		return err
	}
	if len(subjects) == 0 {
		return fmt.Errorf("tell was drawn for %s with no honest claim it knows: its precondition "+
			"held when the pool was built and does not now", step.Actor)
	}
	return pass(l, src, p, step, subjects, "claim", false)
}

// Tellable is the subset of `subjects` that somebody still needs telling of.
//
// IT IS THE RECEIVER HALF, AND IT IS A PRECONDITION AND NOT A HOPE. The first
// cut of the honest tell left this to pass() alone and said so in a comment --
// "both kinds can be drawn and then refuse ... named rather than left". Driven
// over 80 seeds it aborted the walk in THREE of 77 completing runs: a telling
// step drawn for an actor whose only honest claim everybody already held. D15
// asks for ten steps and no refusal, so a 4% abort is not a named gap, it is a
// flaky drill. The measurement is what changed the decision.
//
// IT SERVES BOTH KINDS, because canard had the identical hole: its own error
// said the pool "cannot see" the receiver half. Fixing tell and leaving canard
// drawn-and-refusable would be the same defect in one function with two
// answers.
//
// AND IT FEEDS THE DRAW AS WELL AS THE GATE, through the same function: a draw
// over subjects the gate did not vet could still land on one with no receiver.
func Tellable(l *ledger.Ledger, actors []string, actor string, subjects []string) ([]string, error) {
	var out []string
	for _, ref := range subjects {
		who, err := receiversFor(l, actors, actor, ref)
		if err != nil {
			return nil, err
		}
		if len(who) > 0 {
			out = append(out, ref)
		}
	}
	return out, nil
}

// receiversFor is every actor other than `actor` that does not already hold
// `ref`, sorted -- D1's rule, so the set and not a loop's order decides.
func receiversFor(l *ledger.Ledger, actors []string, actor, ref string) ([]string, error) {
	var out []string
	for _, cand := range actors {
		if cand == actor {
			continue
		}
		known, err := engine.KnownAt(l, cand)
		if err != nil {
			return nil, fmt.Errorf("what %s knows: %w", cand, err)
		}
		holds := false
		for _, k := range known {
			if k == ref {
				holds = true
				break
			}
		}
		if !holds {
			out = append(out, cand)
		}
	}
	sort.Strings(out)
	return out, nil
}

// pass is the hop both telling kinds make: draw which claim travels, draw a
// receiver who does not already hold it, and append the `told`. The system
// never moves.
//
// THE RECEIVER HALF IS NOT IN THE POOL and that is a gap this kind INHERITS
// rather than introduces. A kind's precondition here is "has the teller
// anything to pass on"; whether anybody still needs telling is about the other
// actors, which nemesisApplicable cannot see without the venire. So both kinds
// can be drawn and then refuse, which is the shape Resolvable exists to prevent
// arriving by a third door. Named rather than left: closing it means handing
// Pool the actor list, and that is a signature change past what fable ruled
// (r5/114) -- raised in the same post as this commit.
//
// `tagged` IS planted ON THE HOP AND ON THE STEP, not on the claim: the claim
// was already planted or already honest before this ran, and a tell does not
// change what it is.
func pass(l *ledger.Ledger, src *Source, p Project, step *Step, subjects []string,
	what string, tagged bool) error {

	actors, err := Actors(p.Venire)
	if err != nil {
		return err
	}
	// ONLY SUBJECTS SOMEBODY STILL NEEDS, and the same function the pool asked
	// with. Drawing first and discovering the receiver set empty afterwards is
	// the drawn-and-then-refused shape, which cost three aborted walks in
	// eighty before this filter existed.
	live, err := Tellable(l, actors, step.Actor, subjects)
	if err != nil {
		return err
	}
	if len(live) == 0 {
		return fmt.Errorf("every other actor already holds each %s %s knows, so there is nobody "+
			"to pass one to: the pool asked Tellable and got a non-empty answer, so the world "+
			"moved between the pool and this step", what, step.Actor)
	}
	si, err := src.IntN(len(live))
	if err != nil {
		return fmt.Errorf("drawing which %s travels: %w", what, err)
	}
	ref := live[si]

	receivers, err := receiversFor(l, actors, step.Actor, ref)
	if err != nil {
		return err
	}
	if len(receivers) == 0 {
		return fmt.Errorf("Tellable offered %s and it has no receiver: the filter and the draw "+
			"disagree, which they cannot, since both ask receiversFor", ref)
	}
	ri, err := src.IntN(len(receivers))
	if err != nil {
		return fmt.Errorf("drawing the receiver: %w", err)
	}

	if _, err := l.Append(ledger.Fact{
		Kind:     ledger.KindTold,
		Actor:    step.Actor,
		To:       receivers[ri],
		ClaimRef: ref,
	}); err != nil {
		return fmt.Errorf("recording the hop from %s to %s: %w", step.Actor, receivers[ri], err)
	}
	step.Hop = fmt.Sprintf("%s <- %s", receivers[ri], ref)
	if tagged {
		step.Planted = true
	}
	return nil
}

// lieAbout draws which applicable, emitting action a lie is told about.
//
// The precondition for suborn and palter is the honest action's own, so the set
// here is the same set Pool consulted: the actions whose preconditions hold for
// this actor and which emit something to lie about.
// A PREDICATE RATHER THAN A SECOND CHOOSER. suborn needs an applicable emitting
// action and nothing more; palter needs one whose claim carries a field it can
// falsify. The shared half -- applicability, the emits filter, the sorted draw
// -- is one function because two would be free to disagree about which actions
// a lie may be about, which is the precondition fable ruled at coop r5/49.
// `extra` is nil for suborn.
func lieAbout(ctx context.Context, p Project, l *ledger.Ledger, src *Source, actor string,
	table *handle.Table, known []string, extra func(bindings.Action) bool) (bindings.Action, error) {
	var zero bindings.Action
	app, err := Applicable(ctx, p.Statute, p.Actions, l, actor)
	if err != nil {
		return zero, err
	}
	var emitting []bindings.Action
	for _, a := range app {
		if len(a.Emits) == 0 {
			continue
		}
		// AND RESOLVABLE FOR THIS ACTOR, which this did not ask until the
		// suborn-handles ruling (fable, coop r5/119) needed the handles the
		// honest action would have consumed.
		//
		// IT IS A HOLE I OPENED AT c543a0c AND IT REACHED palter FIRST. This
		// function reads the STATUTE's applicability and nothing else, while
		// the pool also filters by Resolvable -- so once Resolvable became
		// knowledge-scoped, a lie could be drawn about an action whose handle
		// this actor cannot name, and palter, which EXECUTES, would then abort
		// in chooseHandles. None of 80 seeds hit it; it was reachable all the
		// same, which is the only part that matters.
		if ok, _ := Resolvable(a, table, known); !ok {
			continue
		}
		if extra != nil && !extra(a) {
			continue
		}
		emitting = append(emitting, a)
	}
	if len(emitting) == 0 {
		return zero, fmt.Errorf("no applicable action of %s's emits anything this kind can lie "+
			"about, so there is nothing to lie about: the precondition held when the pool was "+
			"built and does not now", actor)
	}
	sort.Slice(emitting, func(i, j int) bool { return emitting[i].Name < emitting[j].Name })
	i, err := src.IntN(len(emitting))
	if err != nil {
		return zero, fmt.Errorf("drawing which action the lie is about: %w", err)
	}
	return emitting[i], nil
}

// mintKind is the handle kind a claim of this action's gets.
//
// THE SINGLE `creates` ENTRY, ELSE THE ACTION'S OWN NAME. Recommended to fable
// at coop r5/59 and built behind this one function so a different ruling is a
// one-function change.
//
// WHY THE FIXTURES ARE NO PRECEDENT, measured: the toy's claim refs follow no
// mechanical rule at all -- event.created gives `event` (its `creates`),
// event.left gives `left` (the type's last segment), and event.joined gives
// `join` and event.deleted gives `del`, which are neither. They are
// hand-written nouns, exactly as alice and bob are hand-written actors.
//
// MORE THAN ONE create OR emit IS REFUSED rather than picked between: no action
// in any example declares two of either, so the refusal costs nothing today and
// the choice stays the gate's.
func mintKind(a bindings.Action) (string, error) {
	if len(a.Creates) > 1 {
		return "", fmt.Errorf("%s declares %d things created (%v) and which one a claim's handle "+
			"follows is not this package's to choose", a.Name, len(a.Creates), a.Creates)
	}
	if len(a.Emits) > 1 {
		return "", fmt.Errorf("%s declares %d emitted types (%v) and which one a claim's handle "+
			"follows is not this package's to choose", a.Name, len(a.Emits), a.Emits)
	}
	if len(a.Creates) == 1 {
		return a.Creates[0], nil
	}
	return a.Name, nil
}

func actionNamed(actions []bindings.Action, name string) (bindings.Action, bool) {
	for _, a := range actions {
		if a.Name == name {
			return a, true
		}
	}
	return bindings.Action{}, false
}

// minter numbers run-local handles: actor.kind#n, n from 1 per actor and kind.
//
// THE COUNTER IS WALK STATE AND NOT TABLE STATE, because a suborn's handle is
// minted and never bound -- so the table cannot be the thing that counts, or
// the number would depend on whether the id arrived.
type minter struct {
	counts map[string]int
}

func (m *minter) next(actor, kind string) string {
	key := actor + "." + kind
	m.counts[key]++
	return key + "#" + strconv.Itoa(m.counts[key])
}

// claimHandles is a claim's `handles`: the handle it MINTED first, then the
// handles the step consumed as arguments.
//
// THE LEDGER'S SHAPE IS THE CONVENTION, and the walk was the one writer not
// following it -- ruled by fable at coop r5/105 after I measured the corpus.
// 62 committed claims: 52 carry `handles == [ref]`, 10 carry a subject beside
// it, none carry no key at all. The ten are the joins and leaves, written by
// hand: `event.joined` with ref `bob.join#1` carries
// `["bob.join#1", "alice.event#1"]`. Every claim this walk wrote carried the
// minted handle alone, so the first WALKED join would have been a claim of a
// shape the corpus does not contain.
//
// WHY IT BELONGS HERE AND NOT IN THE SHRINKER. R5.3's grouping reads `handles`
// to say which entity a claim is about -- that is what the projection was
// ruled for at r5/16 -- and a walked join naming no event is a claim that
// grouping cannot read. fable: it belongs with the walk, not with the shrinker
// that would otherwise have to special-case it.
//
// `step.Used` IS ALREADY THE LIST and in declaration order, which perform()
// records off the declaration rather than off the args map for the reason
// stated there: a map's order is its own. So an action with no handle argument
// -- `create_event`, `create_user` -- yields the minted handle alone, which is
// the 52-claim shape, and nothing here special-cases it.
//
// SUBORN IS DELIBERATELY NOT CHANGED, and the reason is not that it was
// overlooked. A suborn does not execute and never calls chooseHandles, so its
// `step.Used` is empty and this would return the minted handle alone anyway --
// but a planted `event.joined` naming no event is unreadable to the same
// grouping. Giving it a subject means DRAWING one, which consumes the
// generator and would change every walk from every seed, so it is a ruling and
// not a line. Raised rather than taken: fable r5/105 ruled the honest and
// palter cases and said nothing of this one.
//
// A NEW SLICE, not an append onto `Used`: the step keeps its own record and a
// caller that aliased it would be deciding what the step reports.
func claimHandles(minted string, used []string) []string {
	out := make([]string, 0, 1+len(used))
	out = append(out, minted)
	out = append(out, used...)
	return out
}

// chooseHandles draws, for each `handle` argument the action declares, one of
// the bound handles of that kind.
//
// IT DRAWS RATHER THAN TAKING THE FIRST, and the distinction is not symmetry
// for its own sake: a walk that always joined the oldest event would never
// reach a second one, and a fuzzer whose choices are fixed explores one path
// however many seeds it is given. The draw is the generator's, so it replays:
// two attempts from one seed choose the same handles.
//
// WHY THIS IS NOT THE EXCEPTION fable NAMED AT r5/72. The ruling there was that
// an `actor` argument consumes NO draw, because its value is DETERMINED -- the
// step's own actor -- so drawing for it would make the generator's consumption
// depend on how many arguments an action declares while changing nothing about
// the result. A `handle` argument's value is not determined: several handles of
// one kind can be bound, and something has to pick. That is the same position
// a `draw` argument is in, and `draw` consumes.
//
// THE CANDIDATES ARE SORTED, because handle.Table's order is its own business
// and a draw over an unordered set is not reproducible.
//
// AN EMPTY CANDIDATE SET IS A DEFECT IN THE POOL AND SAYS SO. Resolvable holds
// an action out of the pool until every kind it names has a bound handle, so
// reaching here with none means applicability and composition disagree -- which
// is worth a refusal naming both halves rather than a step that merely fails.
func chooseHandles(a bindings.Action, src *Source, table *handle.Table,
	known []string) (map[string]string, error) {
	var chosen map[string]string
	for _, arg := range a.Args {
		if arg.Source != bindings.ArgHandle {
			continue
		}
		// THE SAME SET Resolvable ASKED ABOUT, through the same function. Two
		// intersections computed apart would let the pool offer an action whose
		// draw then refuses, which is the shape the refusal below reports and
		// the shape this scoping was added to close.
		candidates := UsableHandles(table, known, arg.Of)
		if len(candidates) == 0 {
			return nil, fmt.Errorf("schedule: %s declares argument %q as a handle of kind %q and "+
				"this actor knows of no bound handle of that kind: Resolvable should have held "+
				"this action out of the pool, so applicability and composition disagree",
				a.Name, arg.Name, arg.Of)
		}
		if src == nil {
			return nil, fmt.Errorf("schedule: choosing a %q handle for %s's argument %q needs the "+
				"run's generator: a choice made some other way would not replay from the seed",
				arg.Of, a.Name, arg.Name)
		}
		i, err := src.IntN(len(candidates))
		if err != nil {
			return nil, fmt.Errorf("schedule: choosing a %q handle for %s's argument %q: %w",
				arg.Of, a.Name, arg.Name, err)
		}
		if chosen == nil {
			chosen = map[string]string{}
		}
		chosen[arg.Name] = candidates[i]
	}
	return chosen, nil
}

// argVisibleTo is the argument name whose value the claim carries.
//
// THE NAME IS THE CONTRACT HERE, not the source: `visible_to` is one of the
// vocabulary's own words (docs/contract.md names `of`, `role` and `visible_to`
// as the schema's names), and a statute READS it off the claim -- which is why
// an action declaring it must put the drawn value on the fact rather than only
// in the request. It is palter's first live field (coop r5/58): a half-true
// claim falsifies a recorded field a statute reads, and until an action
// declared this one there was no such field in a walk.
const argVisibleTo = "visible_to"

// visibleTo is the drawn `visible_to` an action declares, as the claim carries
// it, or nil where the action declares none.
//
// ABSENT AND EMPTY ARE DIFFERENT TO A STATUTE and the ledger's `omitempty`
// cannot tell them apart: an empty list would be dropped from the fact and the
// claim would reach the statute looking exactly like one that never declared
// visibility. So an action that declares the argument and draws an empty list
// is REFUSED rather than quietly recorded as unrestricted -- the same rule as
// an undeclared weight, one field over.
//
// THE VALUE MUST BE A LIST OF STRINGS, checked rather than asserted: a project
// drawing `visible_to` from a list of role NAMES rather than role SUBSETS is a
// plausible mistake, and it would otherwise reach the statute as a string where
// a list belongs and fail inside the policy with nothing naming the cause.
func visibleTo(a bindings.Action, args map[string]any) ([]string, error) {
	declared := false
	for _, arg := range a.Args {
		if arg.Name == argVisibleTo {
			declared = true
			break
		}
	}
	if !declared {
		return nil, nil
	}
	raw, ok := args[argVisibleTo]
	if !ok {
		return nil, fmt.Errorf("schedule: %s declares a %s argument and the composed args carry "+
			"none: the claim would reach the statute with no visibility at all", a.Name, argVisibleTo)
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("schedule: %s's %s drew %T (%v) and a claim's %s is a list of "+
			"roles: a role NAME where a role SUBSET belongs would reach the statute as the wrong "+
			"shape and fail inside the policy", a.Name, argVisibleTo, raw, raw, argVisibleTo)
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("schedule: %s's %s drew an empty list: the ledger omits an empty "+
			"%s, so the claim would be indistinguishable from one that declares no visibility, "+
			"and those are different statements to a statute", a.Name, argVisibleTo, argVisibleTo)
	}
	out := make([]string, 0, len(list))
	for _, v := range list {
		role, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("schedule: %s's %s drew a list holding a %T (%v) and a role is "+
				"a string", a.Name, argVisibleTo, v, v)
		}
		out = append(out, role)
	}
	return out, nil
}

// palterable reports whether a palter can be told about this action: its claim
// must carry a field a statute READS, and the walk must be able to record a
// value for it that is false.
//
// docs/design.md fixes both halves: the field "is one a statute READS --
// `visible_to` where the action's claim carries it, else `expect` -- and a kind
// whose action's claim carries neither is inapplicable for that action".
//
// `expect` IS NOT REACHABLE FROM A WALK TODAY, measured rather than assumed:
// ledger.Fact carries Expect, and nothing in internal/schedule sets it -- a
// claim's expect comes from a hand-written case. So visible_to is the only
// falsifiable field a walk produces, and an action not declaring it leaves
// palter inapplicable, which is the document's own clause rather than a gap.
//
// AND A DRAW OF ONE VALUE IS NOT FALSIFIABLE. "Exactly one recorded field
// false" needs a false value to exist, and the only values this engine may
// invent are ones the PROJECT declared -- a subset of its own `from` list.
// With one element there is no other subset to record, so the kind is
// inapplicable for that action rather than lied about with a value no project
// ever named.
func palterable(a bindings.Action) bool {
	for _, arg := range a.Args {
		if arg.Name == argVisibleTo && arg.Source == bindings.ArgDraw && len(arg.From) > 1 {
			return true
		}
	}
	return false
}

// palter executes the honest action and then records its claim with exactly one
// field false.
//
// THE ACTION HAPPENS. That is the whole difference from suborn, which is "a
// description with no action under it": a palter is "an action that happened
// described wrongly", so the system DOES move, the handle IS bound to a real
// id, and only the claim lies. A later step needing that handle is therefore
// applicable, where a suborn's is not.
//
// THE LIE IS A VALUE THE PROJECT DECLARED. The recorded subset is drawn from
// the action's own `from` list, excluding the one that actually went out -- so
// the falsehood is a thing the system could plausibly have done, which is what
// makes it a lie rather than a corruption. Inventing a role nobody declared
// would be caught by the statute as malformed instead of as a visibility
// defect.
func palter(ctx context.Context, p Project, sess *wire.Session, sessions *Sessions,
	l *ledger.Ledger, table *handle.Table, mint *minter, src *Source, known []string,
	step *Step) error {

	a, err := lieAbout(ctx, p, l, src, step.Actor, table, known, palterable)
	if err != nil {
		return err
	}
	done, err := perform(ctx, sess, sessions, table, mint, src, a, known, step)
	if err != nil {
		return err
	}
	if !done.Emits {
		// UNREACHABLE WHILE lieAbout FILTERS ON emits, and a refusal rather
		// than a silent return: a palter that executed and recorded nothing
		// would be a step the ledger cannot show as planted.
		return fmt.Errorf("palter chose %s, which emits nothing, so there is no claim to make "+
			"false: lieAbout's filter and this check disagree", a.Name)
	}

	truth, err := visibleTo(a, done.Args)
	if err != nil {
		return err
	}
	lie, err := falsifyVisibleTo(a, truth, src)
	if err != nil {
		return err
	}
	if _, err := l.Append(ledger.Fact{
		Kind:      ledger.KindClaim,
		Actor:     step.Actor,
		Ref:       done.Handle,
		Type:      a.Emits[0],
		Handles:   claimHandles(done.Handle, step.Used),
		VisibleTo: lie,
		Planted:   true,
	}); err != nil {
		return fmt.Errorf("recording %s's half-true %s claim: %w", step.Actor, a.Emits[0], err)
	}
	step.Handles = append(step.Handles, done.Handle)
	step.Planted = true
	return nil
}

// falsifyVisibleTo draws a subset the action did NOT use, from its own list.
func falsifyVisibleTo(a bindings.Action, truth []string, src *Source) ([]string, error) {
	var from []any
	for _, arg := range a.Args {
		if arg.Name == argVisibleTo && arg.Source == bindings.ArgDraw {
			from = arg.From
		}
	}
	if len(from) == 0 {
		return nil, fmt.Errorf("%s declares no drawn %s, so a palter has no field to falsify: "+
			"palterable and this function disagree", a.Name, argVisibleTo)
	}
	was := strings.Join(truth, ",")
	var others [][]string
	for _, v := range from {
		list, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("%s's %s draws from a %T and a subset is a list of roles",
				a.Name, argVisibleTo, v)
		}
		subset := make([]string, 0, len(list))
		for _, r := range list {
			role, ok := r.(string)
			if !ok {
				return nil, fmt.Errorf("%s's %s holds a %T where a role name belongs", a.Name,
					argVisibleTo, r)
			}
			subset = append(subset, role)
		}
		if strings.Join(subset, ",") != was {
			others = append(others, subset)
		}
	}
	if len(others) == 0 {
		return nil, fmt.Errorf("every subset %s declares for %s equals the one it used (%v), so "+
			"there is no false value to record", a.Name, argVisibleTo, truth)
	}
	i, err := src.IntN(len(others))
	if err != nil {
		return nil, fmt.Errorf("drawing the false %s for %s: %w", argVisibleTo, a.Name, err)
	}
	return others[i], nil
}

// EventualOf is the project's eventual claim types, as the walk takes them.
//
// IT IS HERE AND EXPORTED BECAUSE TWO CALLERS NEED IT AND ONE OF THEM IS A
// HARNESS. It lived unexported in cmd/rumor, so tests/shrink built its Project
// with no Eventual and no Prober and drove a walk the product never runs --
// fable's ruling 3 (coop r6/78) one layer out: an arm that cannot apply the
// overlay cannot see a bound that does. A derivation the product performs is
// not the command's private business.
//
// ONLY THE QUIESCE FORM CARRIES THROUGH with a zero bound, which is the
// comment cmd/rumor had at the call site: a DURATION bound needs the run to
// control the clock, and engine.Judge refuses one rather than reading it as
// past, so flattening it here would move that refusal into a silent truncation.
func EventualOf(b bindings.Bindings) []engine.Eventual {
	var out []engine.Eventual
	for name, c := range b.Claims {
		if c.Timing != "eventual" {
			continue
		}
		out = append(out, engine.Eventual{Of: name, Quiesce: c.Bound == bindings.Quiesce})
	}
	// SORTED, because a map's order is not a fact about the project and two
	// runs of one seed must not differ by it.
	sort.Slice(out, func(i, j int) bool { return out[i].Of < out[j].Of })
	return out
}
