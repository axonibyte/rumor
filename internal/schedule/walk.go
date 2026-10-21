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

	// Planted says the step appended a planted claim -- a lie the engine knows
	// it told, because it told it.
	Planted bool
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

		pool, err := Pool(ctx, p.Statute, p.Actions, l, actor, p.Weights, p.Stage, p.Profile, table,
			p.Roles)
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

		step := Step{N: n, Actor: actor, Kind: kind}
		if err := take(ctx, p, sess, sessions, l, table, mint, src, &step); err != nil {
			return rec, fmt.Errorf("schedule: step %d: %s taking %s: %w", n, actor, kind, err)
		}
		rec.Steps = append(rec.Steps, step)
	}

	rec.Sessions = sessions.Acquired()
	rec.Facts = l.Facts()
	rec.IDs = map[string]string{}
	for _, h := range table.Handles() {
		if id, ok := table.ID(h); ok {
			rec.IDs[h] = id
		}
	}
	return rec, nil
}

// take performs one drawn kind: the honest action, or the lie.
func take(ctx context.Context, p Project, sess *wire.Session, sessions *Sessions,
	l *ledger.Ledger, table *handle.Table, mint *minter, src *Source, step *Step) error {

	switch step.Kind {
	case Canard:
		return canard(l, src, p, step)
	case Suborn:
		return suborn(ctx, p, l, mint, src, step)
	case Palter:
		// IMPLEMENTED IN R5.4, because that is when the field arrived. The
		// refusal that stood here said palter "needs a claim field to falsify
		// and nothing in R5.1 populates one" -- true then, and R5.4's
		// `visible_to` argument is that field. ibis hit the refusal from the
		// other end driving the toy (coop r5/88): seed 4242 reaches step 3 and
		// step 3 draws palter.
		return palter(ctx, p, sess, sessions, l, table, mint, src, step)
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
	return honest(ctx, sess, sessions, l, table, mint, src, a, step)
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
	table *handle.Table, mint *minter, src *Source, a bindings.Action, step *Step) (performed, error) {
	_ = ctx
	var out performed

	// THE HANDLE CHOICES ARE DRAWN FIRST AND THE REST COMPOSED AFTER, and that
	// order is fixed rather than incidental: both consume the generator, so a
	// run that composed in the other order would walk differently from the same
	// seed. Stated here because it is the kind of thing a later edit reorders
	// without noticing.
	chosen, err := chooseHandles(a, src, table)
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
	table *handle.Table, mint *minter, src *Source, a bindings.Action, step *Step) error {
	done, err := perform(ctx, sess, sessions, table, mint, src, a, step)
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
		Handles:   []string{done.Handle},
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
func suborn(ctx context.Context, p Project, l *ledger.Ledger, mint *minter, src *Source, step *Step) error {
	a, err := lieAbout(ctx, p, l, src, step.Actor, nil)
	if err != nil {
		return err
	}
	kind, err := mintKind(a)
	if err != nil {
		return err
	}
	h := mint.next(step.Actor, kind)
	if _, err := l.Append(ledger.Fact{
		Kind:    ledger.KindClaim,
		Actor:   step.Actor,
		Ref:     h,
		Type:    a.Emits[0],
		Planted: true,
		Handles: []string{h},
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
func canard(l *ledger.Ledger, src *Source, p Project, step *Step) error {
	subjects, err := engine.CanardSubjects(l, step.Actor)
	if err != nil {
		return err
	}
	if len(subjects) == 0 {
		return fmt.Errorf("canard was drawn for %s with no planted claim it knows and did not "+
			"author: its precondition held when the pool was built and does not now", step.Actor)
	}
	si, err := src.IntN(len(subjects))
	if err != nil {
		return fmt.Errorf("drawing which planted claim travels: %w", err)
	}
	ref := subjects[si]

	actors, err := Actors(p.Venire)
	if err != nil {
		return err
	}
	var receivers []string
	for _, cand := range actors {
		if cand == step.Actor {
			continue
		}
		known, err := engine.KnownAt(l, cand)
		if err != nil {
			return fmt.Errorf("what %s knows: %w", cand, err)
		}
		holds := false
		for _, k := range known {
			if k == ref {
				holds = true
				break
			}
		}
		if !holds {
			receivers = append(receivers, cand)
		}
	}
	// SORTED, for D1's reason one noun over: the receiver must be a function of
	// the seed and the SET, not of the order a loop happened to build it in.
	sort.Strings(receivers)
	if len(receivers) == 0 {
		return fmt.Errorf("every other actor already holds %s, so there is nobody for %s to pass "+
			"it on to: canard's precondition is about the teller and this is the receiver half, "+
			"which the pool cannot see", ref, step.Actor)
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
	step.Planted = true
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
	extra func(bindings.Action) bool) (bindings.Action, error) {
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
func chooseHandles(a bindings.Action, src *Source, table *handle.Table) (map[string]string, error) {
	var chosen map[string]string
	for _, arg := range a.Args {
		if arg.Source != bindings.ArgHandle {
			continue
		}
		var candidates []string
		if table != nil {
			for _, h := range table.Handles() {
				if kind, ok := handleKind(h); ok && kind == arg.Of {
					candidates = append(candidates, h)
				}
			}
		}
		if len(candidates) == 0 {
			return nil, fmt.Errorf("schedule: %s declares argument %q as a handle of kind %q and "+
				"no handle of that kind is bound: Resolvable should have held this action out of "+
				"the pool, so applicability and composition disagree", a.Name, arg.Name, arg.Of)
		}
		sort.Strings(candidates)
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
	l *ledger.Ledger, table *handle.Table, mint *minter, src *Source, step *Step) error {

	a, err := lieAbout(ctx, p, l, src, step.Actor, palterable)
	if err != nil {
		return err
	}
	done, err := perform(ctx, sess, sessions, table, mint, src, a, step)
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
		Handles:   []string{done.Handle},
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
