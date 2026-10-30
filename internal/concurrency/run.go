package concurrency

import (
	"fmt"
	"sort"
	"strings"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/schedule"
	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

// Report is what the concurrency stage did: one verdict per declared subject,
// in Subjects' order.
type Report struct {
	Verdicts []Verdict
}

// Verdict is one subject contended: the world the stage built, the plan it
// issued, and every fan-out it released.
//
// THE TRIES ARE ALL KEPT AND NOT ONLY THE LAST. A fan-out the clause cannot
// answer is a fact about the setup, and a record that showed only the answering
// one would leave a reader unable to tell a subject contended on the first try
// from one that took three -- which is the difference between a project whose
// first declared choice puts its system under pressure and one whose does not.
type Verdict struct {
	Subject string
	// Setup is the chain's steps as Chain.Names renders them.
	Setup []string
	Plan  Plan
	Tries []Try
}

// Try is one fan-out: the setup choices that built its world and what it
// observed.
type Try struct {
	// Choices is the drawn arguments this try sent, rendered `action.arg=value`
	// in a fixed order, so a reader can rebuild the world by hand.
	Choices    []string
	Contention Contention
	// Attempts is what the fan-out did, request by request, and it is kept for
	// a reason the r8/40 mutant names: a plan the dispatch IGNORES is a plan
	// that was decoration, and the only way to see that is to compare the
	// session each attempt was carried by against the session the plan chose.
	// Render prints none of this -- the attempts carry a clock.
	Attempts []Attempt
	// Measured is the clock-derived half, for stderr only: Overlapped is the
	// clock-free fact and rides on the hashed stream.
	Measured   string
	Overlapped bool
}

// Answered is the try whose fan-out the overfill clause could judge.
//
// IT IS THE LAST ONE AND ONLY WHEN IT IS NOT VACUOUS, because the sweep stops
// at the first world the clause can answer in: a run whose every try was
// vacuous has no answering try at all, and that is a could-not-run rather than
// a verdict.
func (v Verdict) Answered() (Try, bool) {
	if len(v.Tries) == 0 {
		return Try{}, false
	}
	last := v.Tries[len(v.Tries)-1]
	if last.Contention.Vacuous() != "" {
		return Try{}, false
	}
	return last, true
}

// Finding is the overfill this subject's contention found, or "".
func (v Verdict) Finding() string {
	t, ok := v.Answered()
	if !ok {
		return ""
	}
	return t.Contention.Finding()
}

// Disagreement is the answers and the world telling different stories, which
// is its own defect and not an overfill.
func (v Verdict) Disagreement() string {
	t, ok := v.Answered()
	if !ok {
		return ""
	}
	return t.Contention.Disagreement()
}

// Render is the verdict's lines for the stream the control hashes.
//
// CLOCK-FREE, like every other line D1 compares: the plan prints session
// indices, the tries print the project's own choices and the system's own
// quantities, and the one number that comes from a clock is in Measured where
// the adapter's timings already go. A duration here would move a digest every
// run, and D6 would then be satisfied by the change that breaks D1.
func (v Verdict) Render() string {
	var b strings.Builder
	fmt.Fprintf(&b, "concurrency: %s -- setup %s; %s\n",
		v.Subject, strings.Join(v.Setup, ", "), v.Plan.Render())
	for i, t := range v.Tries {
		c := t.Contention
		fmt.Fprintf(&b, "concurrency:   try %d -- %s; room for %d of %d request(s), %d accepted "+
			"over %d held, the system holds %d against a limit of %d\n",
			i+1, strings.Join(t.Choices, " "), c.Room(), c.Width, c.Accepted, c.Before,
			c.Observed, c.Limit)
		if why := c.Vacuous(); why != "" {
			fmt.Fprintf(&b, "concurrency:     NOT CONTENDED -- %s\n", why)
		}
	}
	if d := v.Disagreement(); d != "" {
		fmt.Fprintf(&b, "concurrency:   DISAGREEMENT %s\n", d)
	}
	if f := v.Finding(); f != "" {
		fmt.Fprintf(&b, "concurrency:   FOUND %s\n", f)
	} else if _, ok := v.Answered(); ok {
		fmt.Fprintf(&b, "concurrency:   no overfill: %s held its own limit under %d "+
			"overlapping request(s)\n", v.Subject, v.Tries[len(v.Tries)-1].Contention.Width)
	}
	return b.String()
}

// Render is every verdict's lines, in Subjects' order.
func (r Report) Render() string {
	var b strings.Builder
	for _, v := range r.Verdicts {
		b.WriteString(v.Render())
	}
	return b.String()
}

// Findings is every overfill and every disagreement the stage found.
//
// A COMPUTED METHOD AND NOT A STORED FIELD, which is D4's lesson from 108 taken
// forward: a Report with a Findings field has a writer, and both of that
// child's D4 branches were armed by nothing because the writer could be wrong
// in a direction no arm looked at. There is nothing to write here -- the
// findings ARE the tries' own verdicts, read when asked.
func (r Report) Findings() []string {
	var out []string
	for _, v := range r.Verdicts {
		if d := v.Disagreement(); d != "" {
			out = append(out, d)
		}
		if f := v.Finding(); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// Measured is the clock-derived half of the report, for stderr.
func (r Report) Measured() []string {
	var out []string
	for _, v := range r.Verdicts {
		for i, t := range v.Tries {
			if t.Measured == "" {
				continue
			}
			out = append(out, fmt.Sprintf("%s try %d %s", v.Subject, i+1, t.Measured))
		}
	}
	return out
}

// Run contends every action a project declares as contended, against one
// system, and judges each against its own declared limit.
//
// THE SESSIONS ARE THE CALLER's TO OPEN, and that is the arrangement the
// package's own header demands: N adapter processes that each start a system of
// their own contend on NOTHING, so whoever owns the world -- the rollback
// provider, under `rumor run` -- is who can point an adapter at it. A stage
// that spawned its own would be a fan-out against N worlds, every request
// accepted and every race reported as absent.
//
// THE HOST IS actors[0] AND THE CONTENDERS FOLLOW IT. The host builds the
// world; each contender races as itself, because an action's precondition is
// about the ACTOR -- the toy's `may_join` is "knows of the event and has not
// already joined" -- so two requests from one actor would have the second
// refused for a reason that has nothing to do with the limit.
//
// NOTHING HERE DRAWS. Every argument is the declaration's own: a literal, or a
// draw's first declared value, or the sweep's coordinate. A setup drawn from
// the seed would make the stage's own reachability a lottery, and the shipped
// toy measures how bad a one: `create_event.visible_to` draws from three
// subsets and TWO of them cannot be contended at all. A joiner's role is
// relative to the event -- cmd/toy's roleOf answers `guest` for anyone who is
// neither its admin nor already an attendee -- so a would-be contender is a
// GUEST, and the two subsets without `guest` answer every join 404 before the
// capacity check is reached. A stage that drew once would report "the system
// was never asked to choose between them" two times in three, over a system
// whose race is right there. What the SEED chooses is the plan, which is the
// stream D9 reads and the one place variation belongs.
func Run(b bindings.Bindings, host *wire.Session, open func() (*wire.Session, error),
	actors []string, seed uint64) (Report, error) {

	subs, err := Subjects(b)
	if err != nil {
		return Report{}, err
	}
	var rep Report
	for _, sub := range subs {
		v, cerr := contend(b, sub, host, open, actors, seed)
		rep.Verdicts = append(rep.Verdicts, v)
		if cerr != nil {
			return rep, cerr
		}
	}
	return rep, nil
}

// contend is one subject's whole contention: its world, its fan-out and its
// verdict. The verdict travels with the error so a could-not-run still says
// what the stage had built and observed, which is the arrangement fuzz.Run
// takes for the same reason.
func contend(b bindings.Bindings, sub Subject, host *wire.Session,
	open func() (*wire.Session, error), actors []string, seed uint64) (Verdict, error) {

	v := Verdict{Subject: sub.Action.Name}
	chain, err := SetupFor(b, sub)
	if err != nil {
		return v, err
	}
	v.Setup = chain.Names()

	limitAction, _ := actionNamed(b, sub.LimitAction)
	limitArg, _ := argNamed(limitAction, sub.LimitArg)
	limit, err := limitValue(sub, limitArg)
	if err != nil {
		return v, err
	}
	width := Width(limit)

	// THE VENIRE HAS TO HOLD THE HOST AND EVERY CONTENDER, and a short one is
	// refused by name rather than contended with a repeated actor. Two requests
	// from one actor have the second refused by the precondition, which would
	// lower the acceptances for a reason that is not the limit -- and the
	// verdict would read as a system that held its limit.
	if len(actors) < width+1 {
		return v, fmt.Errorf("contending %s needs a host and %d contender(s) -- its limit is %d "+
			"and a fan-out is limit plus one -- and the platform's venire declares %d actor(s) "+
			"(%s): two requests from one actor have the second refused by the precondition, which "+
			"would lower the acceptances for a reason that is not the limit",
			sub.Action.Name, width, limit, len(actors), strings.Join(actors, ", "))
	}
	hostActor, contenders := actors[0], actors[1:width+1]

	plan, err := PlanFor(seed, width)
	if err != nil {
		return v, err
	}
	v.Plan = plan

	held, err := observationNamed(b, sub.Held)
	if err != nil {
		return v, fmt.Errorf("contending %s: %w", sub.Action.Name, err)
	}

	hostTokens := schedule.NewSessions(host)
	type party struct {
		actor  string
		sess   *wire.Session
		tokens *schedule.Sessions
	}
	parties := make([]party, 0, width)
	for i, actor := range contenders {
		s, oerr := open()
		if oerr != nil {
			return v, fmt.Errorf("opening contender %d of %d for %s: %w",
				i+1, width, sub.Action.Name, oerr)
		}
		defer func() { _ = s.Close() }()
		parties = append(parties, party{actor: actor, sess: s, tokens: schedule.NewSessions(s)})
	}

	// THE BOOTSTRAP RUNS ON THE SESSION ITS ACTOR WILL RACE FROM, once each.
	// It is outside the sweep because what it makes persists: an actor exists
	// after its first `create_user`, and running it again would be answered
	// with the refusal cmd/toy gives a name already taken.
	for _, a := range chain.PerActor {
		if _, serr := step(a, hostActor, host, hostTokens, nil, nil); serr != nil {
			return v, fmt.Errorf("bootstrapping the host %s: %w", hostActor, serr)
		}
	}
	for _, p := range parties {
		for _, a := range chain.PerActor {
			if _, serr := step(a, p.actor, p.sess, p.tokens, nil, nil); serr != nil {
				return v, fmt.Errorf("bootstrapping the contender %s: %w", p.actor, serr)
			}
		}
	}

	for _, choice := range sweep(chain.Once, sub, limit) {
		// THE WORLD IS BUILT FRESH FOR EVERY TRY, which is what makes the
		// sweep sound rather than a retry: each try creates its own subject to
		// contend, so a vacuous one leaves nothing behind for the next -- the
		// room is per-thing, and the precondition that bars a second request
		// from one actor is per-thing too.
		ids := map[string]string{}
		for _, a := range chain.Once {
			got, serr := step(a, hostActor, host, hostTokens, ids, choice)
			if serr != nil {
				return v, fmt.Errorf("building the world for %s: %w", sub.Action.Name, serr)
			}
			for kind, id := range got {
				ids[kind] = id
			}
		}

		before, oerr := observe(host, held, ids)
		if oerr != nil {
			return v, fmt.Errorf("reading %s before the fan-out: %w", sub.Held, oerr)
		}

		reqs := make([]wire.ExecuteRequest, 0, width)
		sessions := make([]*wire.Session, 0, width)
		for _, p := range parties {
			args, aerr := arguments(sub.Action, p.actor, ids, choice)
			if aerr != nil {
				return v, fmt.Errorf("composing %s for %s: %w", sub.Action.Name, p.actor, aerr)
			}
			tok, terr := p.tokens.Token(p.actor, sub.Action.Context)
			if terr != nil {
				return v, fmt.Errorf("a session for %s in context %q: %w",
					p.actor, sub.Action.Context, terr)
			}
			reqs = append(reqs, wire.ExecuteRequest{Action: sub.Action.Name, Token: tok, Args: args})
			sessions = append(sessions, p.sess)
		}

		atts, ferr := Fan(plan, sessions, reqs)
		if ferr != nil {
			return v, ferr
		}
		// A FAN-OUT THAT DID NOT OVERLAP IS A COULD-NOT-RUN AND NOT A TRY TO
		// SWEEP PAST. It is not a fact about the project's choices, which is
		// what the sweep varies; it is this run having exercised the
		// sequential path, where a correct system and one with no check at all
		// answer identically. Sweeping on would send more requests against the
		// same unanswerable question.
		if !Overlapped(atts) {
			return v, fmt.Errorf("the %d requests of %s did not overlap, so this run exercised "+
				"the sequential path and a check-then-act defect could not be reached by it: a "+
				"non-overlapping fan-out is a could-not-run, because a correct system and one "+
				"with no check at all answer it the same way", width, sub.Action.Name)
		}
		accepted := 0
		for _, att := range atts {
			if att.Fault != "" {
				return v, fmt.Errorf("session %d faulted, so the fan-out never reached the "+
					"system and its count is not a measurement: %s", att.Session, att.Fault)
			}
			// ACCEPTANCE IS THE PROJECT's OWN DECLARATION, read through the
			// walk's one reading of it: `[[action]].response` is an equality
			// contract for exactly this question, and a 2xx range here would
			// be a second answer about one transport that could disagree with
			// the first.
			if _, refused := schedule.Refusal(sub.Action, att.Response); !refused {
				accepted++
			}
		}
		observed, oerr := observe(host, held, ids)
		if oerr != nil {
			return v, fmt.Errorf("reading %s after the fan-out: %w", sub.Held, oerr)
		}

		t := Try{
			Choices:    choice.render(),
			Attempts:   atts,
			Overlapped: true,
			Measured:   OverlapMeasured(atts),
			Contention: Contention{Action: sub.Action.Name, Limit: limit, Before: before,
				Accepted: accepted, Observed: observed, Width: width},
		}
		v.Tries = append(v.Tries, t)
		if t.Contention.Vacuous() == "" {
			return v, nil
		}
	}

	// EVERY DECLARED WORLD WAS VACUOUS, AND THAT IS A COULD-NOT-RUN. "No
	// overfill" is what a correct system and an untested one both report, so a
	// stage that returned silence here would be reporting its own setup as a
	// clean result. The reasons are all carried, because which precondition
	// failed is what the project's author fixes.
	var why []string
	for i, t := range v.Tries {
		why = append(why, fmt.Sprintf("try %d (%s): %s", i+1,
			strings.Join(t.Choices, " "), t.Contention.Vacuous()))
	}
	return v, fmt.Errorf("no fan-out of %s could answer the overfill clause over %d declared "+
		"world(s): %s. The clause is an absence claim, so a silent pass here would be this "+
		"stage's setup answering and not the system",
		sub.Action.Name, len(v.Tries), strings.Join(why, "; "))
}

// choice is one try's drawn arguments: action name to argument name to value.
type choice map[string]map[string]any

func (c choice) valueFor(action, arg string) (any, bool) {
	if byArg, ok := c[action]; ok {
		v, has := byArg[arg]
		return v, has
	}
	return nil, false
}

// render is the choice's own line, in a fixed order so two runs name the same
// world the same way.
func (c choice) render() []string {
	var out []string
	for action, byArg := range c {
		for arg, v := range byArg {
			out = append(out, fmt.Sprintf("%s.%s=%v", action, arg, v))
		}
	}
	sort.Strings(out)
	return out
}

// sweep is the worlds the stage will try, in order: the first declared value of
// every drawn argument, then one coordinate moved at a time.
//
// ENUMERATED AND NOT DRAWN, which is Run's own rule applied: the stage's
// reachability must not be a lottery. The first try is every draw's first
// declared value, which is the world a reader derives by hand from the file.
//
// ONE COORDINATE AT A TIME AND NOT THE PRODUCT. A product over the declared
// lists is exponential in the number of drawn arguments and every point of it
// costs a creation and a fan-out; a coordinate sweep is linear and reaches
// every single value a project declared, which is what the vacuity it exists to
// escape turns on -- the toy's two unreachable worlds are the `visible_to`
// subsets that omit `guest`, and moving that one coordinate leaves them both. A project
// whose contention needs two choices moved together is a world this sweep
// cannot build, and it says so by reporting every try's vacuity rather than by
// silently passing.
//
// THE LIMIT IS NOT SWEPT. It is pinned to the value limitValue chose, because
// the width is derived from it: moving it would change how many contenders the
// fan-out needs, and the sessions are opened once before the sweep begins.
func sweep(once []bindings.Action, sub Subject, limit int) []choice {
	base := choice{}
	type coord struct {
		action string
		arg    string
		from   []any
	}
	var coords []coord
	for _, a := range once {
		for _, arg := range a.Args {
			if arg.Source != bindings.ArgDraw || len(arg.From) == 0 {
				continue
			}
			if a.Name == sub.LimitAction && arg.Name == sub.LimitArg {
				if base[a.Name] == nil {
					base[a.Name] = map[string]any{}
				}
				base[a.Name][arg.Name] = limit
				continue
			}
			if base[a.Name] == nil {
				base[a.Name] = map[string]any{}
			}
			base[a.Name][arg.Name] = arg.From[0]
			coords = append(coords, coord{action: a.Name, arg: arg.Name, from: arg.From})
		}
	}
	out := []choice{base}
	for _, c := range coords {
		for i := 1; i < len(c.from); i++ {
			next := choice{}
			for action, byArg := range base {
				next[action] = map[string]any{}
				for arg, v := range byArg {
					next[action][arg] = v
				}
			}
			next[c.action][c.arg] = c.from[i]
			out = append(out, next)
		}
	}
	return out
}

// limitValue is the number the fan-out contends against.
//
// THE SMALLEST THE PROJECT DECLARED, where it declared a list. Width is limit
// plus one, so the smallest limit is the smallest fan-out that can exceed it --
// every contender past that adds a refusal that says nothing new -- and it is
// also the one that keeps the trace inside issue 1's "under five actions" and
// the venire's own size. It is not the SEED's choice for Run's reason: a width
// that moved with the seed would move how many actors the stage needs, and a
// project whose venire holds enough for one draw and not another would contend
// on some seeds and refuse on others.
func limitValue(sub Subject, arg bindings.Arg) (int, error) {
	switch arg.Source {
	case bindings.ArgLiteral:
		n, ok := asInt(arg.Value)
		if !ok {
			return 0, fmt.Errorf("%s reads its limit from %s.%s, whose literal is %v (%T): a "+
				"limit is a count, and a fan-out cannot be sized from a value that is not one",
				sub.Action.Name, sub.LimitAction, sub.LimitArg, arg.Value, arg.Value)
		}
		return n, nil
	case bindings.ArgDraw:
		best, found := 0, false
		for _, v := range arg.From {
			n, ok := asInt(v)
			if !ok {
				return 0, fmt.Errorf("%s reads its limit from %s.%s, which draws %v (%T): a "+
					"limit is a count, and a fan-out cannot be sized from a value that is not one",
					sub.Action.Name, sub.LimitAction, sub.LimitArg, v, v)
			}
			if !found || n < best {
				best, found = n, true
			}
		}
		if !found {
			return 0, fmt.Errorf("%s reads its limit from %s.%s, which draws from an empty list",
				sub.Action.Name, sub.LimitAction, sub.LimitArg)
		}
		return best, nil
	}
	// SetupFor has already refused every other source by name; this is the
	// pairing's other half rather than a second message for the same fault.
	return 0, fmt.Errorf("%s reads its limit from %s.%s, whose source is %q",
		sub.Action.Name, sub.LimitAction, sub.LimitArg, arg.Source)
}

// asInt reads a count out of a value a TOML declaration or a JSON answer
// carried, without caring which numeric type it arrived as -- the reason
// internal/schedule's own comparison gives: a project writes 1 and a wire
// answer comes back 1.0, and the two are the same count.
func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float32:
		if float32(int(n)) != n {
			return 0, false
		}
		return int(n), true
	case float64:
		if float64(int(n)) != n {
			return 0, false
		}
		return int(n), true
	}
	return 0, false
}

// step executes one setup action and returns the ids its answer carried.
//
// A SETUP STEP THE SYSTEM REFUSED IS A COULD-NOT-RUN, named: the stage was
// building a world and the system declined, so whatever the fan-out went on to
// observe would be about a world the stage does not know the shape of. It is
// the reading internal/fuzz's setup takes, for the same reason.
func step(a bindings.Action, actor string, sess *wire.Session, tokens *schedule.Sessions,
	ids map[string]string, c choice) (map[string]string, error) {

	args, err := arguments(a, actor, ids, c)
	if err != nil {
		return nil, err
	}
	tok, err := tokens.Token(actor, a.Context)
	if err != nil {
		return nil, fmt.Errorf("a session for %s in context %q: %w", actor, a.Context, err)
	}
	resp, err := sess.Execute(wire.ExecuteRequest{Action: a.Name, Token: tok, Args: args})
	if err != nil {
		return nil, fmt.Errorf("%s as %s: %w", a.Name, actor, err)
	}
	if why, refused := schedule.Refusal(a, resp); refused {
		return nil, fmt.Errorf("the setup step %s was refused, so the world the fan-out would "+
			"contend in was never built: %s", a.Name, why)
	}
	got := map[string]string{}
	for kind, id := range resp.IDs {
		got[kind] = fmt.Sprint(id)
	}
	for _, kind := range a.Creates {
		if _, ok := got[kind]; !ok {
			return nil, fmt.Errorf("%s declares it creates %q and the system's answer carried no "+
				"such id: the fan-out would name a thing it never obtained", a.Name, kind)
		}
	}
	return got, nil
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
func arguments(a bindings.Action, actor string, ids map[string]string, c choice) (map[string]any, error) {
	args := make(map[string]any, len(a.Args))
	for _, arg := range a.Args {
		switch arg.Source {
		case bindings.ArgHandle:
			id, ok := ids[arg.Of]
			if !ok {
				return nil, fmt.Errorf("action %q takes a handle of kind %q and the setup holds "+
					"no id for one: the request would carry a name the system never minted",
					a.Name, arg.Of)
			}
			args[arg.Name] = id
		case bindings.ArgLiteral:
			args[arg.Name] = arg.Value
		case bindings.ArgDraw:
			if v, ok := c.valueFor(a.Name, arg.Name); ok {
				args[arg.Name] = v
				continue
			}
			if len(arg.From) == 0 {
				return nil, fmt.Errorf("action %q's argument %q draws from an empty list",
					a.Name, arg.Name)
			}
			// THE FIRST DECLARED VALUE, which is Run's rule for everything the
			// sweep is not moving: the subject's own draws included, so every
			// contender sends the same request and the fan-out is a contention
			// on one thing rather than N requests that differ.
			args[arg.Name] = arg.From[0]
		case bindings.ArgActor:
			if actor == "" {
				return nil, fmt.Errorf("action %q declares argument %q as the acting actor's "+
					"name and no actor was given", a.Name, arg.Name)
			}
			args[arg.Name] = actor
		default:
			return nil, fmt.Errorf("action %q's argument %q declares source %q, which is not one "+
				"of %v", a.Name, arg.Name, arg.Source, bindings.ArgSources)
		}
	}
	return args, nil
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
func observe(sess *wire.Session, decl bindings.Observation, ids map[string]string) (int, error) {
	var args map[string]any
	if len(decl.Args) > 0 {
		args = make(map[string]any, len(decl.Args))
		for _, name := range decl.Args {
			id, ok := ids[name]
			if !ok {
				return 0, fmt.Errorf("the observation %q declares an argument %q and the setup "+
					"minted no %q: the read-back would be about nothing", decl.Name, name, name)
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
		return 0, fmt.Errorf("observing %s: %w", decl.Name, err)
	}
	n, ok := asInt(resp.Value)
	if !ok {
		return 0, fmt.Errorf("the observation %q answered %v (%T), which is not a count: the "+
			"overfill clause compares it against a limit", decl.Name, resp.Value, resp.Value)
	}
	return n, nil
}
