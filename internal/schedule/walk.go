package schedule

import (
	"context"
	"fmt"
	"sort"
	"strconv"

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

		pool, err := Pool(ctx, p.Statute, p.Actions, l, actor, p.Weights, p.Stage, p.Profile, table)
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
		if err := take(ctx, p, sess, l, table, mint, src, &step); err != nil {
			return rec, fmt.Errorf("schedule: step %d: %s taking %s: %w", n, actor, kind, err)
		}
		rec.Steps = append(rec.Steps, step)
	}

	rec.IDs = map[string]string{}
	for _, h := range table.Handles() {
		if id, ok := table.ID(h); ok {
			rec.IDs[h] = id
		}
	}
	return rec, nil
}

// take performs one drawn kind: the honest action, or the lie.
func take(ctx context.Context, p Project, sess *wire.Session, l *ledger.Ledger,
	table *handle.Table, mint *minter, src *Source, step *Step) error {

	switch step.Kind {
	case Canard:
		return canard(l, src, p, step)
	case Suborn:
		return suborn(ctx, p, l, mint, src, step)
	case Palter, Essoin:
		// UNREACHABLE WHILE THEIR PRECONDITIONS HOLD THEM OUT, and a refusal
		// rather than a skip: fable ruled at coop r5/58 that a drawn kind whose
		// effect is unimplemented is refused BY NAME, because a skip is the
		// silent-drop class. palter needs a claim field to falsify and nothing
		// in R5.1 populates one; essoin needs R9's flow state.
		return fmt.Errorf("the %s effect is not implemented in R5.1, and a drawn kind is refused "+
			"rather than skipped: a skip would drop a draw silently", step.Kind)
	}

	a, ok := actionNamed(p.Actions, step.Kind)
	if !ok {
		return fmt.Errorf("%q is neither a declared action nor a nemesis kind, and the pool it "+
			"came from is built from those two sets alone", step.Kind)
	}
	return honest(ctx, sess, l, table, mint, a, step)
}

// honest executes the action against the system under test and records what it
// emitted.
func honest(ctx context.Context, sess *wire.Session, l *ledger.Ledger, table *handle.Table,
	mint *minter, a bindings.Action, step *Step) error {
	_ = ctx

	resp, err := sess.Execute(wire.ExecuteRequest{Action: a.Name, Args: map[string]any{}})
	if err != nil {
		return fmt.Errorf("executing %s: %w", a.Name, err)
	}

	// AN ACTION THAT EMITS NOTHING MINTS NOTHING, and this is checked BEFORE
	// the handle kind is asked for. fable ruled at coop r5/62 that such a step
	// "appends no claim and mints no handle, recorded as a step" -- so the
	// handle-kind question never arises for it, and mintKind's refusal of an
	// action declaring two `creates` must not reach a step that was never
	// going to mint one. My first version asked in the other order and would
	// have refused a recorded-and-harmless step.
	if len(a.Emits) == 0 {
		return nil
	}

	kind, err := mintKind(a)
	if err != nil {
		return err
	}
	h := mint.next(step.Actor, kind)
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
			return fmt.Errorf("%s declares it creates %q and the adapter's execute answered ids %v, "+
				"which carries no id for it -- the handle would resolve to nothing and a later step "+
				"needing it could not say why. The adapter's response was %v; an empty ids map has "+
				"more than one cause, so read that rather than guessing (401 is no session, 400 is "+
				"no args, and a walk sends neither today)", a.Name, a.Creates[0], resp.IDs, resp.Response)
		}
		str, ok := id.(string)
		if !ok {
			return fmt.Errorf("%s's %q id came back as %T (%v) and a handle resolves to a string id",
				a.Name, a.Creates[0], id, id)
		}
		if err := table.Bind(h, str); err != nil {
			return fmt.Errorf("binding %s to the id the adapter gave: %w", h, err)
		}
	}

	if _, err := l.Append(ledger.Fact{
		Kind:    ledger.KindClaim,
		Actor:   step.Actor,
		Ref:     h,
		Type:    a.Emits[0],
		Handles: []string{h},
	}); err != nil {
		return fmt.Errorf("recording %s's %s claim: %w", step.Actor, a.Emits[0], err)
	}
	step.Handles = append(step.Handles, h)
	return nil
}

// suborn plants a claim about an action that never ran.
//
// DO NOT EXECUTE, ruled at coop r5/58: a suborn is a description with no action
// under it. The handles are minted as if the action had run and are NEVER
// BOUND, so they resolve to no id and any later step needing one is
// inapplicable -- the walk never asks the wire for a thing that does not exist.
func suborn(ctx context.Context, p Project, l *ledger.Ledger, mint *minter, src *Source, step *Step) error {
	a, err := lieAbout(ctx, p, l, src, step.Actor)
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
func lieAbout(ctx context.Context, p Project, l *ledger.Ledger, src *Source, actor string) (bindings.Action, error) {
	var zero bindings.Action
	app, err := Applicable(ctx, p.Statute, p.Actions, l, actor)
	if err != nil {
		return zero, err
	}
	var emitting []bindings.Action
	for _, a := range app {
		if len(a.Emits) > 0 {
			emitting = append(emitting, a)
		}
	}
	if len(emitting) == 0 {
		return zero, fmt.Errorf("no applicable action of %s's emits anything, so there is nothing "+
			"to lie about: the precondition held when the pool was built and does not now", actor)
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
