package concurrency

import (
	"fmt"
	"sort"
	"strings"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/schedule"
	"forge.axonibyte.com/axonibyte/rumor/internal/shrink"
	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

// world is the sessions, the tokens and the declarations one contention runs
// against: the host that builds the world and one party per contender.
//
// IT EXISTS SO THE ORIGINAL RUN AND A REPLAY TAKE ONE PATH. The shrink asks for
// the same contention again with fewer steps, and fable's ruling 3 at coop
// r6/78 forbids proving a claim on a path the product does not take -- so a
// replay that re-implemented the fan-out would be proving the shrink against a
// second reading of the stage. `run` executes whatever steps it is HANDED, and
// what differs between the original and a replay is the step list, which is
// data rather than a branch.
type world struct {
	b     bindings.Bindings
	sub   Subject
	held  bindings.Observation
	limit int
	plan  Plan

	host    *wire.Session
	hostTok *schedule.Sessions
	parties []party

	// ran is every step this world executed, in order, which is what a replay
	// owes its caller: internal/shrink's Replay contract is that the steps
	// come back because "a composition that assembled the result by slicing
	// Request.Steps would be reporting what it ASKED for rather than what
	// happened" -- and the two differ exactly when a step stalls in the
	// shortened world, which is the case a shrink creates.
	ran []schedule.Step

	// ids is the handle kind to the id the system minted for it, accumulated
	// across every step this world has run. A later creation of the same kind
	// REPLACES the earlier one, which is what a sweep wants: each try contends
	// its own subject and the one in hand is the one the fan-out names.
	ids map[string]string
}

// newWorld opens the sessions one contention needs and reads the facts it is
// judged against out of the declarations.
//
// IT IS SHARED BY THE RUN AND THE REPLAY so neither can disagree with the other
// about the width, the plan or the limit: a replay that sized its own fan-out
// would be contending something else and calling it the same trace.
func newWorld(b bindings.Bindings, sub Subject, host *wire.Session,
	open func() (*wire.Session, error), actors []string, seed uint64) (*world, error) {

	limitAction, _ := actionNamed(b, sub.LimitAction)
	limitArg, _ := argNamed(limitAction, sub.LimitArg)
	limit, err := limitValue(sub, limitArg)
	if err != nil {
		return nil, err
	}
	width := Width(limit)

	// THE VENIRE HAS TO HOLD THE HOST AND EVERY CONTENDER, and a short one is
	// refused by name rather than contended with a repeated actor. Two requests
	// from one actor have the second refused by the precondition, which would
	// lower the acceptances for a reason that is not the limit -- and the
	// verdict would read as a system that held its limit.
	if len(actors) < width+1 {
		return nil, fmt.Errorf("contending %s needs a host and %d contender(s) -- its limit is "+
			"%d and a fan-out is limit plus one -- and the platform's venire declares %d actor(s) "+
			"(%s): two requests from one actor have the second refused by the precondition, which "+
			"would lower the acceptances for a reason that is not the limit",
			sub.Action.Name, width, limit, len(actors), strings.Join(actors, ", "))
	}
	plan, err := PlanFor(seed, width)
	if err != nil {
		return nil, err
	}
	held, err := observationNamed(b, sub.Held)
	if err != nil {
		return nil, fmt.Errorf("contending %s: %w", sub.Action.Name, err)
	}
	w := &world{b: b, sub: sub, held: held, limit: limit, plan: plan,
		host: host, hostTok: schedule.NewSessions(host), ids: map[string]string{}}
	for i, actor := range actors[1 : width+1] {
		sess, oerr := open()
		if oerr != nil {
			w.close()
			return nil, fmt.Errorf("opening contender %d of %d for %s: %w",
				i+1, width, sub.Action.Name, oerr)
		}
		w.parties = append(w.parties,
			party{actor: actor, sess: sess, tokens: schedule.NewSessions(sess)})
	}
	return w, nil
}

// HostActor is the actor that builds the world: the first the venire declares.
//
// IT IS NOT A CONTENDER, and that is load-bearing rather than tidy. On the
// shipped toy the creator's role relative to its own event is `admin` and a
// would-be joiner's is `guest`, so a fan-out including the creator is
// answerable at `visible_to = ["admin"]` -- one request accepted into room for
// one, no overfill, the clause ANSWERED -- and the sweep would stop at the
// first world and report no overfill over a system whose race is two tries
// away.
func HostActor(actors []string) string { return actors[0] }

// close releases the contender sessions this world opened. The host's belongs
// to the caller.
func (w *world) close() {
	for _, p := range w.parties {
		_ = p.sess.Close()
	}
}

// party is one contender: the actor it races as and the session it races from.
type party struct {
	actor  string
	sess   *wire.Session
	tokens *schedule.Sessions
}

// Trace is the steps a contention takes, in the shape a shrink reduces: the
// bootstrap once per actor, the world the limit is set in, and THE FAN-OUT AS
// ONE STEP of width N.
//
// ONE STEP FOR THE FAN-OUT IS fable's RULING 2 (coop rumor/1521): "what shrinks
// is the SETUP: the step prefix and the action kinds in it ... the fan-out is
// one step of width N in the trace, N is the stage's and is not reduced". Fan's
// refusal of width 1 stands, so there is nothing inside it to take out -- and
// issue 1's "under five actions" counts it as one action beside its setup.
//
// THE STEP COUNT IS FIVE ON THE SHIPPED TOY AND I HAD SAID FOUR. Width's own
// comment claimed "two users, the event, and the fan-out as one", which is the
// count for two contenders and no separate creator -- but the creator needs its
// own user to create anything (`has_user`), and it must NOT be a contender: at
// `visible_to = ["admin"]` the creator can see the event and a member cannot,
// so a fan-out including the creator would be answerable-and-CLEAN at try 1 and
// the sweep would stop there, reporting no overfill over a system whose race is
// one try away. So three create_users, the create_event and the fan-out: five,
// one over the done-when's figure, and the figure is the walk's traces rather
// than a stage's.
//
// Drew CARRIES THE SETUP'S OWN CHOICES, which is what that field exists for:
// "so a limit can be read from the WALK's OWN NUMBER ... without it a replay
// could not read the same limit the walk handed the system". A replay composes
// each step's arguments from the step, so it sends what the trace SAYS was
// sent rather than re-deriving it.
//
// Handles IS EMPTY AND THAT IS NOT AN OVERSIGHT. A handle is the walk's own
// run-local naming, resolved through a handle table this stage does not have;
// it holds the system's ids by kind instead. Synthesising `member_1.event#1`
// here would put a name in the record that nothing minted, and Trace.Handles
// compares those names.
func (w *world) Trace(chain Chain, c choice, hostActor string) []schedule.Step {
	var out []schedule.Step
	n := 0
	add := func(actor string, a bindings.Action) {
		n++
		out = append(out, schedule.Step{N: n, Actor: actor, Kind: a.Name, Drew: drawn(a, c)})
	}
	for _, a := range chain.PerActor {
		add(hostActor, a)
		for _, p := range w.parties {
			add(p.actor, a)
		}
	}
	for _, a := range chain.Once {
		add(hostActor, a)
	}
	// THE FAN-OUT's ACTOR FIELD IS ONE ACTOR WIDE AND A FAN-OUT IS N, so the
	// contenders are joined: the record would otherwise name one of the actors
	// that raced and silently drop the rest. Nothing in the shrink's path
	// parses this field -- it is rendered, and the reductions read N and Kind
	// -- and these steps are never handed to the walk, whose RoleOf would read
	// a role out of it.
	who := make([]string, 0, len(w.parties))
	for _, p := range w.parties {
		who = append(who, p.actor)
	}
	n++
	out = append(out, schedule.Step{N: n, Actor: strings.Join(who, "+"), Kind: w.sub.Action.Name})
	return out
}

// drawn is the choices a setup step sends, in the action's declared order.
func drawn(a bindings.Action, c choice) []schedule.Draw {
	var out []schedule.Draw
	for _, arg := range a.Args {
		if arg.Source != bindings.ArgDraw {
			continue
		}
		if v, ok := c.valueFor(a.Name, arg.Name); ok {
			out = append(out, schedule.Draw{Arg: arg.Name, Value: v})
			continue
		}
		if len(arg.From) > 0 {
			out = append(out, schedule.Draw{Arg: arg.Name, Value: arg.From[0]})
		}
	}
	return out
}

// run executes a trace against this world and reports what the fan-out
// observed.
//
// FOUR RETURNS, EACH A DIFFERENT FACT, because the callers read them
// differently and none of them may be inferred from another.
//
// `fanned` SAYS WHETHER THE FAN-OUT STEP RAN AT ALL. A trace cut below it --
// which is what a length bisection asks for -- contends nothing, and that is
// the candidate not reproducing to a REPLAY and a defect in the trace to the
// original run. Reading it off a zero Try would be reading "the system
// accepted nothing" for "the system was never asked".
//
// `refused` IS THE SYSTEM DECLINING SOMETHING THE TRACE ASKED FOR, and it is
// not an error for the same reason: to the original run a refused setup step is
// a could-not-run -- the stage was building its own world and the system said
// no -- while to a replay it is the candidate not reproducing, because a
// reduction that removed a kind the setup needed is SUPPOSED to fail that way.
// Returning an error for both would make every kind-removal probe abort the
// whole reduction; returning a Try for both would read a world that was never
// built as a system that held its limit.
//
// `err` IS THE MACHINE: a transport fault, a response that is not a table, a
// fan-out that did not overlap. Those mean the same thing to both callers.
func (w *world) run(steps []schedule.Step) (Try, bool, string, error) {
	byName := map[string]bindings.Action{}
	for _, a := range w.b.Actions {
		byName[a.Name] = a
	}
	for _, s := range steps {
		if s.Kind == w.sub.Action.Name {
			t, refused, err := w.fanOut(s)
			if refused == "" && err == nil {
				w.ran = append(w.ran, s)
			}
			return t, refused == "" && err == nil, refused, err
		}
		a, ok := byName[s.Kind]
		if !ok {
			return Try{}, false, "", fmt.Errorf("the trace names a step %q and the project "+
				"declares no such action: a replay cannot send what nothing describes", s.Kind)
		}
		sess, tokens := w.host, w.hostTok
		for _, p := range w.parties {
			if p.actor == s.Actor {
				sess, tokens = p.sess, p.tokens
			}
		}
		got, refused, err := w.step(a, s, sess, tokens)
		if err != nil {
			return Try{}, false, "", err
		}
		if refused != "" {
			return Try{}, false, refused, nil
		}
		for kind, id := range got {
			w.ids[kind] = id
		}
		w.ran = append(w.ran, s)
	}
	return Try{}, false, "", nil
}

// fanOut releases the subject's requests together and judges what the system
// held.
//
// THE HELD QUANTITY IS READ BEFORE AND AFTER, so the room is computed from two
// measurements rather than from an assumption about a world this stage built
// itself. A setup that left something behind then shows up as a room the
// verdict was computed from, not as an overfill nobody can explain.
func (w *world) fanOut(s schedule.Step) (Try, string, error) {
	before, refused, err := observe(w.host, w.held, w.ids)
	if err != nil {
		return Try{}, "", fmt.Errorf("reading %s before the fan-out: %w", w.held.Name, err)
	}
	if refused != "" {
		return Try{}, refused, nil
	}
	reqs := make([]wire.ExecuteRequest, 0, len(w.parties))
	sessions := make([]*wire.Session, 0, len(w.parties))
	for _, p := range w.parties {
		args, refused, aerr := argsFor(w.sub.Action, p.actor, w.ids, drawsOf(s))
		if aerr != nil {
			return Try{}, "", fmt.Errorf("composing %s for %s: %w", w.sub.Action.Name, p.actor, aerr)
		}
		if refused != "" {
			return Try{}, refused, nil
		}
		tok, terr := p.tokens.Token(p.actor, w.sub.Action.Context)
		if terr != nil {
			// A SESSION THE SYSTEM WOULD NOT ISSUE IS A REFUSAL AND NOT AN
			// ERROR, because it is how a reduction that removed the bootstrap
			// fails: cmd/toy declines a session for an actor with no user, and
			// that is the candidate not reproducing rather than the machine
			// breaking.
			return Try{}, fmt.Sprintf("the system would not issue %s a session in context %q, so "+
				"the fan-out could not be released: %v", p.actor, w.sub.Action.Context, terr), nil
		}
		reqs = append(reqs, wire.ExecuteRequest{
			Action: w.sub.Action.Name, Token: tok, Args: args})
		sessions = append(sessions, p.sess)
	}

	atts, ferr := Fan(w.plan, sessions, reqs)
	if ferr != nil {
		return Try{}, "", ferr
	}
	// A FAN-OUT THAT DID NOT OVERLAP IS A COULD-NOT-RUN IN BOTH READINGS, and
	// that is why it is an error here and not a refusal. It is not a fact about
	// the project's choices, which is what the sweep varies, and it is not a
	// candidate failing to reproduce either: it is this run having exercised
	// the sequential path, where a correct system and one with no check at all
	// answer identically. A replay that read it as "did not reproduce" would
	// mark a real race unstable on the strength of a run that tested nothing.
	if !Overlapped(atts) {
		return Try{}, "", fmt.Errorf("the %d requests of %s did not overlap, so this run "+
			"exercised the sequential path and a check-then-act defect could not be reached by "+
			"it: a correct system and one with no check at all answer it the same way",
			len(w.parties), w.sub.Action.Name)
	}
	accepted := 0
	for _, att := range atts {
		if att.Fault != "" {
			return Try{}, "", fmt.Errorf("session %d faulted, so the fan-out never reached the "+
				"system and its count is not a measurement: %s", att.Session, att.Fault)
		}
		// ACCEPTANCE IS THE PROJECT's OWN DECLARATION, read through the walk's
		// one reading of it: `[[action]].response` is an equality contract for
		// exactly this question, and a 2xx range here would be a second answer
		// about one transport that could disagree with the first.
		if _, refused := schedule.Refusal(w.sub.Action, att.Response); !refused {
			accepted++
		}
	}
	observed, oref, oerr := observe(w.host, w.held, w.ids)
	if oerr != nil {
		return Try{}, "", fmt.Errorf("reading %s after the fan-out: %w", w.held.Name, oerr)
	}
	if oref != "" {
		return Try{}, oref, nil
	}
	return Try{
		Attempts:   atts,
		Overlapped: true,
		Measured:   OverlapMeasured(atts),
		Contention: Contention{Action: w.sub.Action.Name, Limit: w.limit, Before: before,
			Accepted: accepted, Observed: observed, Width: len(w.parties)},
	}, "", nil
}

// step executes one setup step and returns the ids its answer carried.
//
// A STEP THE SYSTEM REFUSED IS A REFUSAL AND NOT AN ERROR, for run's reason:
// the original run reads it as a could-not-run and a replay reads it as the
// candidate not reproducing, and only the caller knows which it is.
func (w *world) step(a bindings.Action, s schedule.Step, sess *wire.Session,
	tokens *schedule.Sessions) (map[string]string, string, error) {

	args, refused, err := argsFor(a, s.Actor, w.ids, drawsOf(s))
	if err != nil {
		return nil, "", err
	}
	if refused != "" {
		return nil, refused, nil
	}
	tok, err := tokens.Token(s.Actor, a.Context)
	if err != nil {
		return nil, fmt.Sprintf("the system would not issue %s a session in context %q: %v",
			s.Actor, a.Context, err), nil
	}
	resp, err := sess.Execute(wire.ExecuteRequest{Action: a.Name, Token: tok, Args: args})
	if err != nil {
		return nil, "", fmt.Errorf("%s as %s: %w", a.Name, s.Actor, err)
	}
	if why, refused := schedule.Refusal(a, resp); refused {
		return nil, fmt.Sprintf("the setup step %s was refused, so the world the fan-out would "+
			"contend in was not built: %s", a.Name, why), nil
	}
	got := map[string]string{}
	for kind, id := range resp.IDs {
		got[kind] = fmt.Sprint(id)
	}
	for _, kind := range a.Creates {
		if _, ok := got[kind]; !ok {
			return nil, "", fmt.Errorf("%s declares it creates %q and the system's answer carried "+
				"no such id: the fan-out would name a thing it never obtained", a.Name, kind)
		}
	}
	return got, "", nil
}

// drawsOf is a step's own drawn values, keyed by argument name: what the trace
// says this step sent.
func drawsOf(s schedule.Step) map[string]any {
	if len(s.Drew) == 0 {
		return nil
	}
	out := make(map[string]any, len(s.Drew))
	for _, d := range s.Drew {
		out[d.Arg] = d.Value
	}
	return out
}

// arguments composes one request's `args` from the declaration, the ids the
// setup obtained and the try's own choices.
//
// IT IS NOT schedule.ExecuteArgs, AND THE DIFFERENCE IS THE HANDLE. The walk
// resolves a handle argument through the run-local handle TABLE, because a walk
// is a sequence of steps whose handles belong to actors; this stage has no walk
// and no table -- it has the ids its own setup just obtained, keyed by the kind
// the creating action declared. internal/fuzz composes the same way and for the
// same reason.
func argsFor(a bindings.Action, actor string, ids map[string]string,
	drew map[string]any) (map[string]any, string, error) {
	args := make(map[string]any, len(a.Args))
	for _, arg := range a.Args {
		switch arg.Source {
		case bindings.ArgHandle:
			id, ok := ids[arg.Of]
			if !ok {
				// A REFUSAL AND NOT AN ERROR, for observe's reason one field
				// over: a reduction that removed the step which mints this kind
				// leaves a trace that cannot send the request, and that is the
				// candidate not reproducing. Sending the request anyway would
				// carry a name the system never minted and be answered 404 --
				// a refusal the fan-out would then report as its own.
				return nil, fmt.Sprintf("action %q takes a handle of kind %q and the trace holds "+
					"no id for one, so the request cannot name what it is about",
					a.Name, arg.Of), nil
			}
			args[arg.Name] = id
		case bindings.ArgLiteral:
			args[arg.Name] = arg.Value
		case bindings.ArgDraw:
			// THE VALUE THE TRACE SAYS THIS STEP SENT, which is what makes a
			// replay a replay: Step.Drew exists "so a limit can be read from
			// the WALK's OWN NUMBER", and composing from it rather than from
			// the sweep's choice means the request is the one the record
			// describes.
			if v, ok := drew[arg.Name]; ok {
				args[arg.Name] = v
				continue
			}
			if len(arg.From) == 0 {
				return nil, "", fmt.Errorf("action %q's argument %q draws from an empty list",
					a.Name, arg.Name)
			}
			// THE FIRST DECLARED VALUE, which is Run's rule for everything
			// neither the sweep nor the trace names: the subject's own draws
			// included, so every contender sends the same request and the
			// fan-out is a contention on one thing rather than N requests that
			// differ.
			args[arg.Name] = arg.From[0]
		case bindings.ArgActor:
			if actor == "" {
				return nil, "", fmt.Errorf("action %q declares argument %q as the acting actor's "+
					"name and no actor was given", a.Name, arg.Name)
			}
			args[arg.Name] = actor
		default:
			return nil, "", fmt.Errorf("action %q's argument %q declares source %q, which is not "+
				"one of %v", a.Name, arg.Name, arg.Source, bindings.ArgSources)
		}
	}
	return args, "", nil
}

// observationNamed is the declared query `contended.held` names.
//
// REFUSED BY NAME WHEN IT IS NOT DECLARED, and the refusal lists what is: the
// verdict rests on this read-back, so a stage that went on without it would be
// judging the overfill clause on the acceptances alone -- a claim about
// RESPONSES, which is the reading rook corrected at coop r8/45.
func observationNamed(b bindings.Bindings, name string) (bindings.Observation, error) {
	var names []string
	for _, o := range b.Observations {
		if o.Name == name {
			return o, nil
		}
		names = append(names, o.Name)
	}
	sort.Strings(names)
	return bindings.Observation{}, fmt.Errorf("`contended.held` names the query %q and the "+
		"project declares no such observation (it declares %v): the verdict rests on what the "+
		"system HOLDS, so there is nothing to read it with", name, names)
}

// observe reads the held quantity back through the declared observation.
//
// THE ARGUMENT NAMES ARE HANDLE KINDS, which is the walk's own rule rather than
// a convention invented here: observeHandles matches a declared argument name
// against the KIND of a handle the step touched (internal/schedule/walk.go), so
// `store_event_count`'s `event` takes the event. The ids this stage holds are
// keyed by kind for the same reason, so the pairing is direct -- and an
// argument whose kind the setup never minted is refused by name rather than
// sent empty.
func observe(sess *wire.Session, decl bindings.Observation, ids map[string]string) (int, string, error) {
	var args map[string]any
	if len(decl.Args) > 0 {
		args = make(map[string]any, len(decl.Args))
		for _, name := range decl.Args {
			id, ok := ids[name]
			if !ok {
				// A REFUSAL AND NOT AN ERROR, which the kind-removal pass found
				// the hard way: it drops `create_event` and the trace that
				// remains mints no event, so the read-back is about nothing --
				// and that is the candidate not reproducing, exactly as the
				// reduction intends, rather than the machine failing. Returning
				// an error here aborted the whole reduction on its first
				// kind-removal probe.
				return 0, fmt.Sprintf("the observation %q declares an argument %q and the trace "+
					"minted no %q, so there is nothing to read it about",
					decl.Name, name, name), nil
			}
			args[name] = id
		}
	}
	resp, err := sess.Observe(wire.ObserveRequest{
		Query:  decl.Name,
		Source: decl.Source,
		Args:   args,
	})
	if err != nil {
		return 0, "", fmt.Errorf("observing %s: %w", decl.Name, err)
	}
	n, ok := asInt(resp.Value)
	if !ok {
		return 0, "", fmt.Errorf("the observation %q answered %v (%T), which is not a count: the "+
			"overfill clause compares it against a limit", decl.Name, resp.Value, resp.Value)
	}
	return n, "", nil
}

// Replay re-runs one of this stage's traces against a world a caller has just
// restored, and reports it as a candidate internal/shrink can judge.
//
// IT IS THE PRODUCT's OWN PATH AND NOT A SECOND READING. The steps come from
// the record the run produced, every request is composed from the step that
// describes it, and the fan-out goes through the same `run` as the original --
// fable's ruling 3 at coop r6/78 forbids proving a claim on a path the product
// does not take, and a replay that re-implemented the contention would be
// proving the shrink against a stage nobody ships.
//
// THE SESSIONS ARE OPENED FRESH EVERY TIME, which is not bookkeeping: a restore
// rebinds the port, so sessions opened once and reused would be talking to a
// world that no longer exists after the first candidate. cmd/rumor's shrink
// makes the same arrangement for the same reason.
//
// PendingOpen IS ZERO AND THAT IS A FACT ABOUT THIS STAGE, not a default. A
// `pending` is a disagreement the walk holds open on an eventual claim type;
// this stage asks no eventual question and holds no ledger, so there is nothing
// it could leave open. internal/shrink refuses a candidate that did not settle,
// and zero is what says this one did.
func Replay(b bindings.Bindings, sub Subject, host *wire.Session,
	open func() (*wire.Session, error), actors []string, seed uint64,
	steps []schedule.Step) (shrink.Candidate, error) {

	w, err := newWorld(b, sub, host, open, actors, seed)
	if err != nil {
		return shrink.Candidate{}, err
	}
	defer w.close()
	t, fanned, refused, rerr := w.run(steps)
	if rerr != nil {
		return shrink.Candidate{Steps: w.ran}, rerr
	}
	switch {
	case refused != "":
		// THE SYSTEM DECLINED SOMETHING THE TRACE ASKED FOR, which is this
		// candidate not reproducing and is the expected answer for a reduction
		// that removed a step the setup needed. The reason travels so a reader
		// of an unstable seed is told what happened instead.
		return shrink.Candidate{Steps: w.ran, Stage: &shrink.StageVerdict{
			Rule: OverfillRule, Subject: sub.Action.Name, Found: false,
			Says: refused}}, nil
	case !fanned:
		return shrink.Candidate{Steps: w.ran, Stage: &shrink.StageVerdict{
			Rule: OverfillRule, Subject: sub.Action.Name, Found: false,
			Says: fmt.Sprintf("the trace ran %d step(s) and none of them contended %s",
				len(w.ran), sub.Action.Name)}}, nil
	}
	return shrink.Candidate{Steps: w.ran, Stage: t.Verdict(sub.Action.Name)}, nil
}
