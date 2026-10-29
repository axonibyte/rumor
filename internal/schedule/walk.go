package schedule

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

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

	// ClockControl says the project declares clock control, which is what
	// lets the run hold a clock at all. It is the bindings' `[clock]` table
	// being PRESENT and nothing about its contents: R7.2 types that table,
	// and a phase that keyed on a field inside it would have to be changed
	// again the moment the shape moved.
	//
	// WITHOUT IT THERE IS NO CLOCK AND NOT A CLOCK OF ZERO. internal/ledger
	// refuses a clock fact outright when control is not declared -- "wall-clock
	// time never enters the ledger" -- so this field decides whether the run
	// may write one, and the vocabulary's refusal stays the thing that enforces
	// it. A project declaring nothing is walked exactly as before: no fact is
	// appended, so no position shifts and every pre-R7 fixture's ledger is
	// byte-identical.
	ClockControl bool

	// ClockForm is HOW the run controls that clock, and it is read only when
	// ClockControl is set. `adapter` means the engine sets the system under
	// test's own test clock through the wire's `clock` request and writes the
	// ledger fact from what comes BACK; `faked` means the stack is already
	// running under faked time, so no request is sent and the engine's own
	// `t` is the world's (fable, coop r7/51).
	//
	// EMPTY IS REFUSED HERE AND NOT AT THE PARSE. internal/bindings carries
	// `[clock]` with no `form` without complaint, because a parser that
	// refused it would make this refusal unreachable and R6.2 measured what
	// that costs. This is the reader that knows which request it was about to
	// send, so this is where a clocked project with no form is named.
	ClockForm bindings.ClockForm
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

	// AdvancedTo is the instant an `advance` step DREW, in nanoseconds from the
	// run's epoch; zero for every other kind.
	//
	// IT IS RECORDED FOR Used's REASON: a choice that appears nowhere in the
	// record cannot be compared between two attempts, and a mutant replacing
	// the draw with "take the first landing" would survive every arm that
	// reads only the kind. The shrink needs it too -- R7.4's done-when is a
	// shrunk trace CONTAINING the advance that crossed a deadline, which is a
	// claim about which landing was taken and not merely that one was.
	//
	// THE DRAW AND THE OUTCOME ARE TWO FACTS AND THIS IS THE DRAW. Where the
	// clock actually ended up is the ledger's clock fact, because the world
	// answers that (fable, coop r7/51): the pair says whether the adapter put
	// the clock where the walk asked.
	AdvancedTo int64

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

	// Refused is why the SYSTEM declined this step, empty where it did not.
	//
	// THE WORDS AND NOT A FLAG, because the disagreement is what a reader of a
	// trace needs: "join_event declares response.status = 200 and the system
	// answered 409" says which action, which key and what came back, where a
	// bare `true` would send the reader to the adapter's log. It is composed
	// from the DECLARATION in sorted key order, so two attempts of one seed
	// name the same disagreement and the record still compares.
	//
	// A REFUSED STEP IS RECORDED AND NOT DROPPED, which is the same rule an
	// inapplicable drawn kind follows: the record is an account of the DRAWS,
	// and one that silently omitted a declined action would let two attempts'
	// records agree while their walks diverged.
	Refused string

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

	// Quiesce records WHETHER THE QUESTION WAS ASKED and what it answered, so
	// a reader can tell "did not run" from "ran, found nothing".
	//
	// THOSE TWO WERE ONE STRING. rook drove D6's first half against v0.1.37
	// (coop rumor/1223) and it asks exactly which of them happened: with
	// probes that never read quiet the run printed `reconciliation: nothing
	// to reconcile`, byte-identical to a quiet run with nothing pending, and
	// identical again to a project declaring no probes at all. Three
	// different states, one sentence -- so the line could not answer the
	// condition it was the only evidence for.
	//
	// IT IS TWO BOOLS AND A NAMING RATHER THAN A VERDICT STRING, because the
	// walk should not compose the operator's prose: cmd/rumor owns the
	// wording and this owns the facts. QuiesceProbes is clock-free (it is
	// Prober.Hang()'s report, which the type documents as safe where the
	// control reads) so putting it in the stream cannot break D1.
	QuiesceAsked  bool
	QuiesceQuiet  bool
	QuiesceProbes string

	// Pendings is one sentence per pending the run held, in its own words:
	// where it was held, whether it was re-checked, and what the re-check
	// decided. Sorted, and carrying no clock, so two attempts of one seed
	// produce byte-identical lines.
	//
	// IT EXISTS BECAUSE "HELD THEN CLEARED" HAD NO WITNESS OUTSIDE THE
	// PROCESS. rook's gate verdict (coop rumor/1223) drove D1 and both of
	// D3's clearing paths and could reach neither: walk.go computes a
	// per-step verdict that can read "pending", the run never printed it and
	// persisted nothing -- no WriteFile, no --out -- so a pending held and
	// then cleared left exactly the same output as a run that never held one.
	// Reconciled came close and is not enough: it records what the re-check
	// SAID without saying which pending it was about or where that pending
	// began, so a reader cannot pair a clearing with the thing it cleared.
	//
	// A COUNT IS NOT A LIFE, which is why this is beside PendingOpen rather
	// than replacing it. PendingOpen answers "did the run end holding one",
	// the question R2.3 forbids a yes to; this answers "what happened to each
	// one", which is the question D3 asks and the count cannot.
	Pendings []string

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
// runClock is the run's own time, and whether the project lets the run hold it.
//
// THE UNIT IS NANOSECONDS FROM THE RUN'S OWN EPOCH, which is zero at the first
// seal -- declared in docs/contract.md at R7.1 before this writer existed, on
// fable's ruling 1 at coop breakout-r7/0. `engine.Eventual.Bound` has carried
// nanoseconds since R6 because that is what time.ParseDuration yields, so the
// comparison R2.3 declared is one subtraction in one unit.
//
// IT MOVES ONLY WHEN THE WALK DRAWS `advance` (R7.3), and nothing here reads
// the host's clock: that is what makes a clocked run exactly as repeatable as
// an unclocked one. Until R7.3 lands the mover, every clock fact a run writes
// carries zero, which is the honest state of a run whose time has not moved
// rather than a placeholder.
type runClock struct {
	control bool
	form    bindings.ClockForm
	t       int64
}

// clockAsker is the half of the adapter session a clocked seal uses.
//
// IT IS AN INTERFACE SO THE SEAL CAN BE DRIVEN WITHOUT A PROCESS. The refusals
// below are about what an adapter ANSWERS -- a fault, a time that runs
// backward, a `set` that comes back changed -- and an arm that had to spawn a
// real adapter to produce each of those answers would be driving the toy's
// honesty rather than the engine's check. *wire.Session satisfies it.
type clockAsker interface {
	Clock(wire.ClockRequest) (wire.ClockResponse, error)
}

// seal writes the run's time at this position and then moves the seal.
//
// THE FACT GOES IN BEFORE THE SEAL MOVES, which is not an ordering detail:
// engine.Now reads the newest clock fact at or BEFORE the seal, so a fact
// appended after sealing is invisible to the very check the seal was moved
// for, and a duration bound would read as unmeasurable at the one position it
// is asked about.
//
// THE TIME IS COPIED, NOT POINTED AT. ledger.Fact carries `T *int64`, so
// handing it the address of this struct's field would give every clock fact in
// the run one shared pointer: the moment R7.3 moves the clock, every fact
// already written would read the new time, and a ledger whose past changes is
// the one thing the vocabulary forbids outright.
func (c *runClock) seal(l *ledger.Ledger, a clockAsker) error {
	if c.control {
		t := c.t
		if c.form == bindings.ClockAdapter {
			got, err := c.ask(a)
			if err != nil {
				return err
			}
			t = got
		}
		if _, err := l.Append(ledger.Fact{Kind: ledger.KindClock, Actor: "run", T: &t}); err != nil {
			return fmt.Errorf("writing the run's clock at position %d: %w", l.Len()+1, err)
		}
	}
	return l.SealAt(l.Len())
}

// ask sets the system under test's clock to the run's time and answers what
// came back, which is what the ledger then records.
//
// THE LEDGER WRITES THE WORLD, NOT THE ASK (fable, coop r7/51). An adapter
// that answered for the world is the authority on what time it is there, and
// a ledger holding the engine's intention instead would be a record of what
// was requested rather than of what happened.
//
// THE THREE REFUSALS ARE BY NAME, and the order matters for the second:
//
//   - NO SUCH REQUEST. An adapter that does not know the kind answers a fault,
//     which wire returns as an error with the session still usable. This is
//     reachable only because the contract version did not move for the new
//     request: behind a bump the handshake would refuse this adapter first,
//     one phase earlier and under the wrong name.
//   - TIME RAN BACKWARD. A `t` less than the clock the run holds is refused
//     before the equality below, so an adapter that answers a smaller number
//     is told THAT rather than "it differs from the ask": replay and the D5
//     drill rest on a ledger whose times never decrease, and the reason is the
//     refusal's whole value.
//   - A `set` CAME BACK CHANGED. `set` means the clock now reads what was
//     asked for; an adapter that rounds has a granularity, and nothing in the
//     protocol lets one DECLARE a granularity today -- `describe`'s
//     capabilities carries faults, volume and upgrade and no such field. So
//     the ruling's "by more than its declared granularity, else at all" is
//     "at all" here, and the conditional half is documented in
//     docs/contract.md rather than coded as a branch no input can reach.
func (c *runClock) ask(a clockAsker) (int64, error) {
	resp, err := a.Clock(wire.ClockRequest{Op: wire.ClockSet, T: c.t})
	if err != nil {
		return 0, fmt.Errorf("the project declares a clock the engine drives through the adapter, "+
			"and setting it to %dns did not answer: %w", c.t, err)
	}
	if resp.T < c.t {
		return 0, fmt.Errorf("the adapter answered %dns where the run's clock holds %dns: time "+
			"never runs backward in a ledger, and both replay and the overage a duration bound is "+
			"judged by rest on that", resp.T, c.t)
	}
	if resp.T != c.t {
		return 0, fmt.Errorf("the adapter answered %dns to a `set` of %dns: a `set` leaves the "+
			"clock reading what was asked for, and an adapter with a coarser clock has no way to "+
			"declare one, so a difference is a disagreement about the time and not a rounding "+
			"this run can account for", resp.T, c.t)
	}
	return resp.T, nil
}

// Tick is the unit a landing is offset by, and it is the contract's own: one
// nanosecond, which fable fixed at coop r7/42 -- "a tick is 1ns, the contract's
// unit, and nothing else".
const Tick = int64(1)

// Landings is where an `advance` may put the run's clock: one tick before each
// live bound, AT it, and one tick past it, keeping only what is ahead of the
// clock now.
//
// THE THREE ARE FABLE's AND THE MIDDLE ONE IS THE HINGE (coop r7/42): "the
// three landings are bound-1ns, at it, +1ns, the middle one the hinge that
// makes :316's strictness falsifiable". `pendingOverlay` holds a disagreement
// open while `now-claimTime > bound` is FALSE, so landing exactly ON the bound
// is the one input that tells a strict comparison from a loose one -- at
// bound-1ns both readings hold it open and at bound+1ns both let it past.
//
// IT RETURNS LANDINGS AND NOT BOUNDS, WHICH IS THE WHOLE POINT OF THE
// FUNCTION. The pool's gate and the step's draw must quantify over ONE set,
// and the set the step draws from is the set of places the clock may go: a
// bound one tick ahead offers two usable landings and one -- bound-1ns -- that
// would be an advance of zero, which `advance` refuses as this side's own
// arithmetic error. Handing the pool BOUNDS would make the gate say "live"
// while the draw picked a landing the mover rejects, which is the
// drawn-and-then-refused shape `Resolvable` exists to prevent, arriving by a
// third door.
//
// SORTED AND DEDUPLICATED for `Register.Untils`'s reasons: adjacent bounds
// share landings, and a map's order is not a run's.
func Landings(untils []int64, now int64) []int64 {
	seen := map[int64]struct{}{}
	out := make([]int64, 0, 3*len(untils))
	for _, u := range untils {
		for _, l := range [3]int64{u - Tick, u, u + Tick} {
			if l <= now {
				// NOT AHEAD IS NOT A LANDING: the clock moves forward or not
				// at all, and a step that moved it nowhere would record a time
				// nothing happened at.
				continue
			}
			if _, dup := seen[l]; dup {
				continue
			}
			seen[l] = struct{}{}
			out = append(out, l)
		}
	}
	slices.Sort(out)
	return out
}

// advanceStep is the `advance` kind's effect: draw one of the landings this
// step was offered and move the clock to it.
//
// THE LANDING IS DRAWN FROM THE GENERATOR, like every other choice a walk
// makes, so one seed replays to the same instant. The list is the same one the
// pool's gate counted -- handed in rather than rebuilt -- so the kind cannot
// be drawn from a set the step then disagrees with.
//
// THE DELTA IS COMPUTED HERE AND THE INSTANT IS RECORDED. The wire carries a
// delta for `advance` (the contract's own shape), while what a reader of a
// trace wants is WHERE the clock was sent; the step records the instant and
// the ledger records where the world put it.
func advanceStep(clk *runClock, a clockAsker, src *Source, landings []int64, step *Step) error {
	if len(landings) == 0 {
		// DRAWN WITH NOTHING TO LAND ON IS A REFUSAL AND NOT A SKIP, which is
		// fable's rule for a drawn kind whose effect cannot run (coop r5/58):
		// a skip drops a draw silently. Reaching here means the pool's gate
		// and this step disagreed about the same list, which is the defect the
		// one-set rule exists to prevent -- so it is named rather than
		// absorbed.
		return fmt.Errorf("advance was drawn with no landing offered: the pool's gate and the " +
			"step read one list, so this is the two disagreeing rather than an empty world")
	}
	i, err := src.IntN(len(landings))
	if err != nil {
		return fmt.Errorf("drawing a landing from %d: %w", len(landings), err)
	}
	to := landings[i]
	if _, err := clk.advance(a, to-clk.t); err != nil {
		return fmt.Errorf("advancing to %dns: %w", to, err)
	}
	step.AdvancedTo = to
	return nil
}

// advance moves the run's clock FORWARD and answers where it ended up.
//
// IT IS R7.3's MOVER and the other half of `ask`. The two exchanges differ in
// what the engine may conclude from the answer, which is fable's ruling at coop
// r7/51 read closely:
//
//   - A `set` MUST COME BACK EXACTLY. The engine named an instant and the clock
//     either reads it afterwards or the adapter and the run disagree about what
//     time it is, so `ask` refuses any difference.
//   - AN `advance` NEED NOT. The engine names a DELTA, and where that lands is
//     the world's answer: an adapter whose clock has a coarse tick, or that
//     clamps, lands somewhere else and has still answered honestly. So this
//     refuses only a clock that went BACKWARD, and the ledger records what came
//     back.
//
// SO THIS IS THE FIRST PLACE "THE LEDGER WRITES THE WORLD" IS FALSIFIABLE,
// which is ratchet's finding at coop r7/53: under `set` alone the ask and the
// answer are one number, so writing either gave the same ledger and the arm
// could not exist. Here they can differ, and an arm where the adapter rounds
// forward is what tells the two apart.
//
// THE RUN'S OWN CLOCK BECOMES THE ANSWER. Anything else would leave the engine
// believing a time the system under test does not hold: the next seal would
// `set` the clock backward to the engine's belief, and `ask` would refuse its
// own request -- a run poisoned by the one exchange that was supposed to move
// it. 101 could not write this line and say what it was for, because under
// `set` the two were always equal.
func (c *runClock) advance(a clockAsker, delta int64) (int64, error) {
	if delta <= 0 {
		return 0, fmt.Errorf("an advance of %dns is not an advance: the run's clock moves "+
			"forward or not at all, and a step that moved it nowhere would record a time "+
			"nothing happened at", delta)
	}
	if c.form != bindings.ClockAdapter {
		// FAKED TIME MOVES WITHOUT ASKING, which is that form's whole meaning:
		// the stack is already under a clock the engine owns, so there is no
		// second holder to agree with.
		c.t += delta
		return c.t, nil
	}
	resp, err := a.Clock(wire.ClockRequest{Op: wire.ClockAdvance, T: delta})
	if err != nil {
		return 0, fmt.Errorf("the project declares a clock the engine drives through the adapter, "+
			"and advancing it by %dns did not answer: %w", delta, err)
	}
	if resp.T < c.t {
		return 0, fmt.Errorf("the adapter answered %dns where the run's clock holds %dns: an "+
			"advance that answers an EARLIER time has moved the world backward, and both replay "+
			"and the overage a duration bound is judged by rest on a ledger whose times never "+
			"decrease", resp.T, c.t)
	}
	// AND NOT `resp.T != c.t+delta`, DELIBERATELY: see the note above. A
	// coarser clock answering 1s where 250ms was asked for has answered for the
	// world, and the world is what the ledger records.
	c.t = resp.T
	return c.t, nil
}

func Walk(ctx context.Context, p Project, sess *wire.Session, seed uint64, steps, attempt int) (Record, error) {
	rec := Record{Seed: seed, Attempt: attempt}
	if steps <= 0 {
		return rec, fmt.Errorf("schedule: a walk of %d steps is not a walk", steps)
	}
	if sess == nil {
		return rec, fmt.Errorf("schedule: no adapter session: a walk executes actions against the " +
			"system under test and cannot be taken against nothing")
	}
	// THE CLOCKED PROJECT THAT DOES NOT SAY HOW is refused before a step is
	// taken, rather than at the first seal: every walk writes a clock fact at
	// every seal, so this would refuse on the first one anyway, and a project
	// that cannot be clocked should not execute an action first.
	if p.ClockControl && p.ClockForm == "" {
		return rec, fmt.Errorf("schedule: the project declares a `[clock]` table and no `form`: "+
			"the run cannot hold a clock without knowing whether to set the adapter's own "+
			"(%q) or to read its time as the world's (%q)", bindings.ClockAdapter, bindings.ClockFaked)
	}
	actors, err := Actors(p.Venire)
	if err != nil {
		return rec, err
	}
	src := New(seed)
	l := ledger.New(p.ClockControl)
	clk := &runClock{control: p.ClockControl, form: p.ClockForm}
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
		//
		// AND THE RUN'S CLOCK IS WRITTEN AT IT when the project declares clock
		// control, which is why this is clk.seal and not l.SealAt. The first
		// pass through here happens before any action, so a clocked run holds
		// a clock fact from before its first step -- fable's D1 asks for it at
		// step 0, and a bound asked about at step 1 needs a time at or before
		// its claim.
		if err := clk.seal(l, sess); err != nil {
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

		// THE LANDINGS ARE READ ONCE, HERE, AND HANDED TO BOTH THE POOL AND
		// THE STEP. `advance` is live iff there is an instant to land on
		// (fable, coop r7/42), and the pool's gate and the step's draw have to
		// quantify over ONE set -- the Tellable rule. Reading the register
		// twice would be two answers to one question, which is the shape the
		// pool's own comment calls the defect being closed.
		//
		// LANDINGS AND NOT BOUNDS, for the reason `Landings` states: every
		// element here is an instant the mover will accept, so the gate cannot
		// say live where the draw would be refused.
		//
		// AN UNCLOCKED RUN'S LIST IS EMPTY BY CONSTRUCTION, not by a second
		// check of `[clock]`: a duration-bounded disagreement in a run with no
		// clock is REFUSED by engine.Judge rather than held, so no pending
		// with an instant can reach the register, and a quiesce-bounded one
		// carries no instant at all.
		landings := Landings(reg.Untils(), clk.t)

		pool, err := Pool(ctx, p.Statute, p.Actions, l, actor, p.Weights, p.Stage, p.Profile, table,
			p.Roles, known, actors, landings)
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
		if err := take(ctx, p, sess, sessions, l, table, mint, src, known, clk, landings, &step); err != nil {
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
		seen, oerr := observe(ctx, p.Statute, sess, sessions, table, p.Observations, l, clk, &step,
			engine.Bindings{Eventual: p.Eventual}, reg, qs, held, &rec.Pendings)
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
	//
	// reAsked IS DECLARED OUT HERE because stillOpen below needs it and the
	// reconciliation that fills it is two blocks in. It holds exactly the keys
	// that got a "re-checked after the last step" sentence, so the two
	// producers of a pending's life describe disjoint sets by construction
	// rather than by a reader trusting that one implies the other.
	reAsked := map[string]bool{}
	if p.Prober != nil {
		state, qerr := p.Prober.Quiesce(ctx)
		if qerr != nil {
			return rec, fmt.Errorf("schedule: asking whether the system settled: %w", qerr)
		}
		// RECORDED WHATEVER THE ANSWER, including the timeout: the question
		// having been ASKED is the fact D6's first half turns on, and a field
		// set only on success would make "asked and gave up" indistinguishable
		// from "never asked" -- the defect one layer in from the one this
		// fixes.
		rec.QuiesceAsked = true
		rec.QuiesceQuiet = state != engine.QuiesceTimedOut
		if h, ok := p.Prober.(interface{ Hang() string }); ok {
			rec.QuiesceProbes = h.Hang()
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
					// AND THE PENDING'S OWN SENTENCE, pairing the clearing
					// with the thing it cleared. Reconciled says only what
					// the re-check decided; this says which pending, where it
					// began, and that it was re-checked here.
					rec.Pendings = append(rec.Pendings, fmt.Sprintf(
						"%s/%s held at step %d, re-checked after the last step: %s",
						actor, query, a.at, f.Says()))
					// MARKED HERE AND NOT AT THE TOP OF THE LOOP, which is the
					// whole distinction: a re-ask that could not be MADE
					// returned above through `if !made { continue }`, and that
					// pending genuinely was never re-checked. Only a re-ask
					// that produced a verdict claims the key.
					reAsked[askKey(actor, query)] = true
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

	// A PENDING THAT WAS NEVER RE-CHECKED STILL GETS A SENTENCE, and this is
	// the half a reader would otherwise have to infer from a count. The
	// reconciliation above runs when a prober is wired AND something was held
	// AND the register is non-empty; a pending outside that gate -- a run
	// with no probes at all -- reaches the end unexamined and said nothing
	// about itself.
	//
	// AND THE GATE IS NOT THE QUIESCE ANSWER, which this comment claimed
	// until it was driven. It read "or one whose probes never read quiet"
	// among the unexamined, and the state is PASSED to the re-ask rather than
	// gating it: on a toy copy whose `queue_depth` probe wants 999999, every
	// held pending is re-checked and answers `quiesce_timeout`, which is
	// terminal, so the register ends EMPTY and this function words nothing --
	// "0 pending(s) open at the end" beside "re-checked after the last step:
	// quiesce_timeout" (seeds 1, 8, 12, 21 at `--steps 24 --profile
	// settling`).
	//
	// AND reAsked IS WHY THIS CANNOT CONTRADICT THE LOOP ABOVE. The register
	// is not emptied by a re-ask: one that answers `pending` again leaves the
	// key open, so passing `reg.Open()` alone made the same (actor, query)
	// take both sentences -- "re-checked after the last step: pending" from
	// the reconciliation and "never re-checked" from here (rook, coop
	// rumor/1264). The set names which keys the loop spoke for.
	//
	// NAMED AS UNEXAMINED RATHER THAN AS CLEAN. "still open at the end" and
	// "re-checked and cleared" are the two outcomes R2.3 distinguishes, and a
	// line that omitted the first would make a run that never asked look
	// exactly like a run that asked and got agreement -- the same collapse
	// rook found in the reconciliation string itself.
	rec.Pendings = append(rec.Pendings, stillOpen(reg.Open(), held, reAsked)...)
	// SORTED FOR THE SAME REASON Reconciled IS: reg.Open() is a map, so
	// without this the line order differs between two attempts of one seed
	// and the control's own comparison fails on an artifact of iteration.
	sort.Strings(rec.Pendings)

	rec.Sessions = sessions.Acquired()
	rec.Facts = l.Facts()
	rec.PendingOpen = reg.Len()
	rec.IDs = map[string]string{}
	for _, h := range table.Handles() {
		if id, ok := table.ID(h); ok {
			rec.IDs[h] = id
		}
	}
	// A RUN DOES NOT END HOLDING A PENDING IT COULD NOT MEASURE, which is
	// R2.3's invariant and fable's ruling at coop r7/15. The record is
	// complete before this fires, so a caller holding the error still has the
	// pendings and the finding it describes.
	if err := unmeasuredAtTheEnd(reg.Open(), held, p.Eventual, clk, rec.Finding, rec.FoundAt); err != nil {
		return rec, err
	}
	return rec, nil
}

// unmeasuredAtTheEnd refuses a run that ends holding a pending under a
// DURATION bound whose clock never moved.
//
// WHY IT IS A REFUSAL AND NOT A VERDICT (fable, coop r7/15). Such a pending is
// not inside its bound; it is UNMEASURED. R7's third ruling is that nothing is
// read as "time did not pass", and the engine already refuses a duration bound
// it cannot measure at the moment of judging -- this is the same refusal moved
// to the one place the walk can see that the measurement never became
// possible. The cause is the RUN's rather than the project's, whose
// declarations are all correct, and the sentence says so.
//
// IT IS KEYED ON THE BOUND'S FORM AND THE CLOCK, NOT ON "A PENDING IS OPEN",
// and that narrowing is the whole of why R6's behavior is untouched. A pending
// left open under a QUIESCE bound is R6's state and R6 judges it: the probes
// read quiet or they did not, and neither answer needs a clock. Refusing on an
// open pending as such would change what rook's D1 and D3 read and move the
// control seeds, which is a different ruling from the one that was made.
//
// A PENDING THE WALK COULD NOT PLACE IS NOT REFUSED ON HERE, deliberately:
// without its `ask` there is no claim type, so there is no bound form to key
// on, and guessing would turn an unattributable pending into a refusal about a
// bound nobody declared. stillOpen names those in the report; this says
// nothing about them.
//
// WHAT FALSIFIES IT IS R7.3. `advance` moves the clock, so `clk.t` stops being
// zero and these same runs resolve, go `stale`, or end as the quiesce_timeout
// shape -- a finding, since the run did move time and the system did not
// settle. The condition removes itself when the mover lands.
func unmeasuredAtTheEnd(open map[string][]string, held map[string]ask,
	eventual []engine.Eventual, clk *runClock, found *engine.Finding, foundAt int) error {

	if clk == nil || !clk.control || clk.t != 0 {
		return nil
	}
	bound := map[string]int64{}
	for _, e := range eventual {
		// `Bound > 0` READS AS "A DURATION BOUND WAS DECLARED" and no longer as
		// "declared and happens to be positive": bindings refuses a
		// non-positive bound at the parse (bindings.go, the Eventual case), so
		// a declared `0s` cannot arrive here as the same zero an ABSENT bound
		// leaves behind. Issue 101's added bullet; before it, those two states
		// were one number.
		if !e.Quiesce && e.Bound > 0 {
			bound[e.Of] = e.Bound
		}
	}
	if len(bound) == 0 {
		return nil
	}

	var unmeasured []string
	for actor, queries := range open {
		for _, query := range queries {
			a, ok := held[askKey(actor, query)]
			if !ok {
				continue
			}
			b, isDuration := bound[a.decl.Of]
			if !isDuration {
				continue
			}
			unmeasured = append(unmeasured, fmt.Sprintf("%s/%s held at step %d, %q bounded %s",
				actor, query, a.at, a.decl.Of, time.Duration(b)))
		}
	}
	if len(unmeasured) == 0 {
		return nil
	}
	// SORTED, because reg.Open() is a map and a refusal whose text differs
	// between two attempts of one seed is a refusal the control cannot hash.
	sort.Strings(unmeasured)

	withheld := ""
	if found != nil {
		// THE FINDING IS NAMED AND NOT ASSERTED (fable, coop r7/15): a finding
		// with a trailing unresolved pending is not one the run can stand
		// behind -- rule 3's silence clause -- so nothing is lost by saying
		// what it was and nothing is claimed by saying it here.
		withheld = fmt.Sprintf("; the run also found %s after step %d, which it does not report "+
			"as a finding: a finding with a pending still unmeasured beside it is where the walk "+
			"stopped asking rather than what the run causes", found.Says(), foundAt)
	}
	return fmt.Errorf("schedule: the run ended holding %d pending(s) under a duration bound and "+
		"moved no time, so %s cannot be measured: %s. The run's clock is still at its epoch -- "+
		"no `advance` was drawn -- and a pending inside a bound the run never approached is "+
		"unmeasured rather than tolerated%s",
		len(unmeasured), plural(len(unmeasured), "it", "they"), strings.Join(unmeasured, "; "), withheld)
}

// plural picks between two words by a count, so a refusal reads as a sentence
// whether it names one pending or six.
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
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
	table *handle.Table, obs []bindings.Observation, l *ledger.Ledger, clk *runClock,
	step *Step, eb engine.Bindings, reg *engine.Register,
	qs engine.QuiesceState, held map[string]ask, lives *[]string) ([]engine.Finding, error) {

	if len(obs) == 0 {
		return nil, nil
	}
	// AND NOTHING HAS HAPPENED IS NOTHING TO JUDGE (R7.4). The engine refuses
	// a check over an unsealed ledger, and a seal of ZERO *is* unsealed
	// (engine.Input's `seal <= 0`) -- so a step that appended NEITHER a claim
	// NOR a tell leaves the ledger empty and the next line would take the walk
	// to a sequencing refusal instead of a verdict.
	//
	// R7.4 IS WHAT MADE THAT REACHABLE and an arm is what found it: before a
	// refused action appended nothing, every first step appended something, so
	// the case could not arise. Measured on the walkable fixture with every
	// create_event answered 500 -- "step 1: judging attendance (direct) after
	// step 1: engine: the ledger is unsealed", that project's first
	// observation being a `direct` one which needs no handle and is therefore
	// tried on every step.
	//
	// SKIPPED AND NOT REFUSED, because there is no question to put: an
	// observation is a comparison against what the ledger says happened, and
	// the ledger says nothing has. The step is still recorded, carrying its
	// refusal.
	if l.Len() == 0 {
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
	if err := clk.seal(l, sess); err != nil {
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
		// A PENDING CLEARED MID-WALK IS D3's FIRST PATH, and it left no
		// sentence at all until rook drove it (coop rumor/1232). All three
		// clearing seeds cleared at the RECONCILIATION -- "after the last
		// step" -- because that was the only place this record was written,
		// so "cleared at the actor's next observation of the same query" was
		// unwitnessed not for want of a run but for want of a line.
		//
		// engine.Judge ALREADY HAD THE FACT: it is the re-check, and it sets
		// Finding.ReChecked before deleting the pending from the register.
		// Nothing outside the engine read it, so a pending that resolved at a
		// later step vanished between the two states this record did cover.
		//
		// READ BEFORE THE OVERWRITE on the next line: `held` still carries the
		// ask this re-check answers, and with it the step the pending began
		// at. One line later that entry is replaced by this step's.
		if found.ReChecked != nil && lives != nil {
			at := -1
			if a, ok := held[askKey(step.Actor, decl.Name)]; ok {
				at = a.at
			}
			where := fmt.Sprintf("step %d", at)
			if at < 0 {
				where = "a step the walk did not record"
			}
			*lives = append(*lives, fmt.Sprintf(
				"%s/%s held at %s, re-checked at step %d: %s",
				step.Actor, decl.Name, where, step.N, found.Says()))
		}
		if found.Value == "pending" && held != nil {
			// WHAT PRODUCED THIS PENDING, REMEMBERED SO IT CAN BE RE-ASKED.
			// The register keys a pending by (actor, query) alone and
			// engine.Check carries no subject, so nothing in the engine can
			// re-issue the observation this came from -- and "a `pending` is
			// re-checked at the actor's next observation of the same query"
			// (docs/vocabulary.md) needs that observation to be MAKEABLE at
			// the end of a run, when no step is supplying handles.
			held[askKey(step.Actor, decl.Name)] = ask{decl: decl, chosen: chosen,
				actor: step.Actor, at: step.N}
		}
		made = append(made, found)
	}
	return made, nil
}

// stillOpen is the sentence for every pending the run ends holding, one per
// (actor, query), naming where it began when the walk recorded that.
//
// IT IS A FUNCTION SO IT CAN BE DRIVEN. Inline in Walk it was reachable only
// through a walk that leaves a pending open, and ratchet measured that nothing
// asserted it: mutating all three occurrences left internal/schedule,
// tests/quiesce, tests/run and internal/engine green (coop r6/185). The
// sentence added so that silence would not pass was itself unwitnessed, which
// is the defect it exists to prevent, one layer out.
//
// THREE WAYS A PENDING REACHES THE END, and this comment said ONE until R7.1
// made the correction legible. It read: "the reconciliation re-asks every open
// pending, so one survives only where the re-ask CANNOT BE MADE". That `only`
// was true while every re-ask resolved, and it is the premise rook's
// rumor/1264 falsified:
//
//   - THE RE-ASK CANNOT BE MADE. observeOne returns not-made for a handle that
//     will not resolve and for an actor with no session in the observation's
//     context, and `if !made { continue }` leaves the pending in the register
//     with nothing said about it. This function words it.
//   - THE RECONCILIATION DID NOT RUN AT ALL -- no prober, nothing held, or an
//     empty register. Same sentence, same reason.
//   - THE RE-ASK WAS MADE AND ANSWERED `pending` AGAIN, which R7.1's clock
//     opened: a duration bound with the run's clock still at zero is INSIDE
//     its bound, so the re-check neither clears the pending nor makes it
//     `stale`, and the key stays open having been spoken for. `reAsked`
//     excludes it here, because the reconciliation already gave it a sentence
//     naming where it was held AND that it was re-checked.
//
// IT IS A DRIVEN PATH NOW, not a reserved one: on a toy copy with
// `bound = "30s"` and a `[clock]`, seeds 1, 8 and 21 at `--steps 24 --profile
// settling` hold a pending at the LAST step, which no later seal sees, so the
// run reaches this reconciliation (ibis, coop rumor/1270; third vantage quill
// rumor/1271). Seed 8 printed both sentences for one pending before this
// change.
func stillOpen(open map[string][]string, held map[string]ask, reAsked map[string]bool) []string {
	var out []string
	for actor, queries := range open {
		for _, query := range queries {
			key := askKey(actor, query)
			// A PENDING THE RECONCILIATION RE-ASKED IS NOT THIS FUNCTION'S TO
			// WORD, and the exclusivity is a parameter rather than an
			// assumption because it is the thing that broke: the caller's
			// `reg.Open()` still holds a key whose re-ask answered `pending`,
			// so "left in the register" and "never re-asked" are two sets and
			// not one.
			if reAsked[key] {
				continue
			}
			a, ok := held[key]
			if !ok {
				// NAMED RATHER THAN OMITTED. A pending the walk cannot place
				// is still a pending the run ended holding, and dropping it
				// here would make the register and the report disagree about
				// how many there were.
				out = append(out, fmt.Sprintf(
					"%s/%s still open at the end, and the walk did not record where it began",
					actor, query))
				continue
			}
			out = append(out, fmt.Sprintf(
				"%s/%s held at step %d, never re-checked: still open at the end",
				actor, query, a.at))
		}
	}
	return out
}

// ask is one observation the walk can re-issue: the declaration, the handles it
// was given, the actor who made it, and the step it was held at.
//
// AT IS HERE SO A PENDING'S LIFE HAS A BEGINNING. D1 and both of D3's clearing
// paths ask for "held then cleared" to be readable from the run's output, and
// a re-check that cannot say WHEN the pending was opened reports half an
// event: `Reconciled` carried the verdict and nothing carried the step, so two
// runs holding a pending at step 2 and at step 9 printed the same line. It is
// Step.N, which the walk already assigns, and it is clock-free.
type ask struct {
	decl   bindings.Observation
	chosen map[string]string
	actor  string
	at     int
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
	clk *runClock, landings []int64, step *Step) error {

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
	case Advance:
		return advanceStep(clk, sess, src, landings, step)
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

	// Refused says the SYSTEM DID NOT DO IT: the adapter's response disagreed
	// with the one the action declares. Nothing is appended for such a step
	// and nothing is minted -- see refusal below.
	Refused bool
}

// refusal reports whether the system refused the action, and in what words.
//
// THE LEDGER RECORDS THE WORLD (fable, coop r7/51 and v0.1.41's own bullet),
// and an action the system refused did not happen in it. Until R7.4 this
// function did not exist: the walk appended a claim on the action's `emits`
// alone and never read the answer, so a refused step recorded a claim the
// system never made and every finding downstream of it was the walk's
// bookkeeping. Measured two-sided (ibis, coop r7/64): the honest toy at seed 1
// runs clean, and with `join_event` wedged at 409 the same seed FOUND a bug
// whose shrunk trace ends at the refused step. ratchet reproduced it from a
// second clone and measured the half I had left open (r7/66): seed 12 refuses
// one join at 409 inside the shipped control, so the path is reached today.
//
// THE TEST IS THE PROJECT'S DECLARED RESPONSE, AND THAT IS A DEVIATION FROM
// THE LETTER OF THE RULING WORTH NAMING. fable's words at coop rumor/1405 are
// "by the status the adapter answered", and this reads `[[action]].response`
// instead, for three reasons the ruling's own "the builder picks the
// mechanics" leaves open:
//
//   - THE WIRE HAS NO STATUS. `execute`'s response carries `ids` and
//     `response any` (docs/contract.md), and `{"status": 200}` is
//     cmd/toy-adapter's own convention. Reading a status here would either
//     invent a wire field -- a contract-version question fable told me to
//     raise rather than take -- or hard-code one adapter's shape into the
//     engine.
//   - HTTP DOES NOT BELONG IN THE WALK. "400 or more is a refusal" is a fact
//     about one transport, and this protocol is deliberately not HTTP.
//   - `response` WAS DECLARED AND READ BY NOTHING. The contract has called it
//     "the bindings' declared response shape" since R4.2 and no code read it:
//     a field with no consumer, which is the defect this document's own rule
//     exists to catch. Now it has one.
//
// SO ACCEPTANCE IS WHAT THE PROJECT SAYS IT IS: every key the action declares
// must be present in the answer and equal to the declared value. An adapter
// that answers 409 where the project declared 200 is refusing; one that
// answers 200 is doing it.
//
// A PROJECT THAT DECLARES NO RESPONSE CANNOT HAVE ITS REFUSALS SEEN, and that
// is stated rather than hidden: `response` is optional in the schema, so an
// action without it is accepted unconditionally -- the behavior every project
// had before this commit. The arm that would close it is `rumor validate`'s
// and it is not taken here.
func refusal(a bindings.Action, resp wire.ExecuteResponse) (string, bool) {
	if len(a.Response) == 0 {
		return "", false
	}
	got, ok := resp.Response.(map[string]any)
	if !ok {
		// A DECLARED SHAPE AND AN ANSWER THAT IS NOT ONE. The project said
		// what acceptance looks like and the adapter answered something else
		// entirely, which cannot be the declared shape -- so it is a refusal
		// and the answer is quoted rather than guessed at.
		return fmt.Sprintf("%s declares a response of %v and the adapter answered %v, "+
			"which is not a table", a.Name, a.Response, resp.Response), true
	}
	for _, k := range sortedKeys(a.Response) {
		want := a.Response[k]
		have, present := got[k]
		if !present {
			return fmt.Sprintf("%s declares response.%s = %v and the adapter's answer "+
				"carries no %s: %v", a.Name, k, want, k, got), true
		}
		if !sameValue(want, have) {
			return fmt.Sprintf("%s declares response.%s = %v and the system answered %v",
				a.Name, k, want, have), true
		}
	}
	return "", false
}

// sortedKeys is the declaration's keys in a fixed order, so two attempts of one
// seed name the SAME disagreement when an answer breaks more than one key. A
// map's own order would make the recorded reason differ between attempts and
// the comparison fail for a reason that is not a difference.
func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// sameValue compares a value a TOML declaration carries against one that came
// back as JSON, without caring which numeric type each arrived as.
//
// IT EXISTS BECAUSE `status = 200` IS AN int64 AND THE ANSWER IS A float64.
// decl reads TOML integers as int64 and the wire decodes JSON numbers as
// float64, so `want == have` is false for two values that are the same number
// -- and every action in every project would read as refused. Driven: without
// the numeric branch, examples/toy's first create_user is a refusal.
func sameValue(want, have any) bool {
	wf, wok := asFloat(want)
	hf, hok := asFloat(have)
	if wok && hok {
		return wf == hf
	}
	return reflect.DeepEqual(want, have)
}

func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	}
	return 0, false
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

	// WHAT THE SYSTEM ANSWERED, READ BEFORE ANYTHING IS RECORDED (R7.4,
	// fable's ruling at coop rumor/1405 item 2). A refused action appends no
	// claim and MINTS NO HANDLE: the system created nothing, so a handle here
	// would resolve to no id and sit in the table as a suborn's does -- and a
	// suborn's unbound handle is for a LIE the engine told, not for a step the
	// world declined.
	//
	// IT ALSO ENDS A HARD ERROR THAT WAS NOT ONE. A refused action declaring
	// `creates` used to reach the `ids` check below with an empty map and kill
	// the run ("the handle would resolve to nothing"), so a project whose
	// system can refuse a creating action could not be walked at all; the
	// refusal is now a recorded step.
	// EMITS IS SET BEFORE THE REFUSAL IS READ, AND THE ORDER IS LOAD-BEARING.
	// It was set after, and the first mutant I drove against this fix SURVIVED
	// because of it: with `out.Emits` still false for a refused action,
	// honest's `if done.Refused` was unreachable -- `!done.Emits` stopped the
	// append first -- so removing the refusal guard changed nothing and both
	// suites stayed green. A guard that cannot fail is not evidence. Setting
	// it here makes the refusal the one reason an emitting action records no
	// claim, and the mutant dies in tests/run.
	out.Emits = len(a.Emits) > 0

	if why, refused := refusal(a, resp); refused {
		out.Refused = true
		step.Refused = why
		return out, nil
	}

	// AN ACTION THAT EMITS NOTHING MINTS NOTHING, and this is checked BEFORE
	// the handle kind is asked for. fable ruled at coop r5/62 that such a step
	// "appends no claim and mints no handle, recorded as a step" -- so the
	// handle-kind question never arises for it, and mintKind's refusal of an
	// action declaring two `creates` must not reach a step that was never
	// going to mint one. My first version asked in the other order and would
	// have refused a recorded-and-harmless step.
	if !out.Emits {
		return out, nil
	}

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
	// A REFUSED ACTION APPENDS NO CLAIM, which is the whole of the fix: the
	// ledger records the world and this did not happen in it. The STEP is
	// recorded, carrying the refusal, so replay and the shrink see a draw that
	// was made and an action the system declined.
	if done.Refused || !done.Emits {
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
	// A PALTER OF A REFUSED ACTION LIES ABOUT NOTHING. design.md's palter is
	// "an action that happened described wrongly"; where the system declined,
	// there is no happening to describe, and appending the claim anyway would
	// plant a lie the control could never clear -- the strip would remove a
	// claim about an action that never ran, which is a suborn's shape under
	// palter's name.
	if done.Refused {
		return nil
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
		// THE BOUND TRAVELS, and leaving it out is what made every duration
		// read as zero: this is the ONLY producer of engine.Eventual in
		// non-test code, so a field it omits is a field the product never has
		// (rook, coop rumor/1191; ratchet /1192).
		out = append(out, engine.Eventual{
			Of:      name,
			Quiesce: c.Bound == bindings.Quiesce,
			Bound:   int64(c.BoundFor),
		})
	}
	// SORTED, because a map's order is not a fact about the project and two
	// runs of one seed must not differ by it.
	sort.Slice(out, func(i, j int) bool { return out[i].Of < out[j].Of })
	return out
}
