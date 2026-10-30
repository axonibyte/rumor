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
	// Steps are the trace this try ran, in the shape a shrink reduces: the
	// bootstrap, the world, and the fan-out as one step. It is what the
	// stage's own verdict is handed to internal/shrink with.
	Steps []schedule.Step

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
	// THE LAST ANSWERABLE TRY AND NOT THE LAST TRY, which is what changed when
	// an absence stopped ending the sweep: a clean world may be followed by a
	// vacuous one -- the toy's `visible_to` subsets are not ordered by
	// contendability -- so reading the final element would report "no try could
	// answer" over a run that answered twice. A FOUND overfill still ends the
	// sweep, so where there is a finding the last try is also the answering one.
	for i := len(v.Tries) - 1; i >= 0; i-- {
		if v.Tries[i].Contention.Vacuous() == "" {
			return v.Tries[i], true
		}
	}
	return Try{}, false
}

// Answerable is how many of this subject's fan-outs the overfill clause could
// judge.
//
// IT IS ON THE VERDICT'S LINE BECAUSE AN ABSENCE IS A SAMPLE. "No overfill"
// over one fan-out and over four are different claims, and rook's drive at coop
// r8/74 is why the difference is printed rather than implied: an intermittent
// race that missed its window answered exactly as a system with no race does.
// How many fan-outs an absence NEEDS is not derivable from any declaration, so
// the number is reported instead of chosen.
func (v Verdict) Answerable() int {
	n := 0
	for _, t := range v.Tries {
		if t.Contention.Vacuous() == "" {
			n++
		}
	}
	return n
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
	} else if t, ok := v.Answered(); ok {
		// THE SAMPLE IS ON THE LINE. A clean verdict is an absence claim, and
		// an absence over one overlapping fan-out is not the same claim as one
		// over four -- so the count of fan-outs the clause could judge is
		// printed beside it, and a reader can tell a system with no race from
		// one whose race did not fire this time.
		fmt.Fprintf(&b, "concurrency:   no overfill in %d answerable fan-out(s): %s held its own "+
			"limit under %d overlapping request(s) each time\n",
			v.Answerable(), v.Subject, t.Contention.Width)
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

	w, err := newWorld(b, sub, host, open, actors, seed)
	if err != nil {
		return v, err
	}
	defer w.close()
	v.Plan = w.plan
	hostActor := HostActor(actors)

	// THE BOOTSTRAP RUNS ONCE AND THE SWEEP DOES NOT REPEAT IT, which is a fact
	// about what it makes rather than a convenience: an actor exists after its
	// own `create_user`, and running it again is answered with the refusal
	// cmd/toy gives a name already taken. So it is executed here, through the
	// SAME runner as everything else, with only the bootstrap's steps in the
	// list -- the step list is data and the runner has no branch for this.
	//
	// AND IT IS STILL IN THE TRACE THE SHRINK IS HANDED, because a replay runs
	// against a world RESTORED to clean, where nothing has created anything. A
	// trace that omitted the bootstrap would be a trace that cannot be replayed
	// -- which is the one thing the record exists to be.
	boot := w.Trace(Chain{PerActor: chain.PerActor}, nil, hostActor)
	boot = boot[:len(boot)-1] // the builder always ends with the fan-out; not yet
	switch _, fanned, refused, rerr := w.run(boot); {
	case rerr != nil:
		return v, fmt.Errorf("bootstrapping %s: %w", sub.Action.Name, rerr)
	case refused != "":
		return v, fmt.Errorf("bootstrapping %s: %s", sub.Action.Name, refused)
	case fanned:
		return v, fmt.Errorf("the bootstrap list held a fan-out of %s, which would spend the "+
			"room before the sweep released anything", sub.Action.Name)
	}

	for _, c := range sweep(chain.Once, sub, w.limit) {
		// THE WORLD IS BUILT FRESH FOR EVERY TRY, which is what makes the
		// sweep sound rather than a retry: each try creates its own subject to
		// contend, so a vacuous one leaves nothing behind for the next -- the
		// room is per-thing, and the precondition that bars a second request
		// from one actor is per-thing too.
		//
		// THE TRACE IS BUILT WHOLE AND RUN FROM ITS WORLD STEPS ON, so the
		// steps the record carries are the steps that ran: the bootstrap's
		// prefix is in the trace and was run above, and what this try executes
		// is the rest of the same list.
		full := w.Trace(chain, c, hostActor)
		t, fanned, refused, rerr := w.run(full[len(boot):])
		if rerr != nil {
			return v, rerr
		}
		if refused != "" {
			return v, fmt.Errorf("building the world for %s: %s", sub.Action.Name, refused)
		}
		if !fanned {
			return v, fmt.Errorf("the trace built for %s holds no fan-out step, so this try "+
				"contended nothing", sub.Action.Name)
		}
		t.Choices = c.render()
		t.Steps = full
		v.Tries = append(v.Tries, t)
		// A FINDING STOPS THE SWEEP AND AN ABSENCE DOES NOT, which is rook's
		// finding at coop r8/74 taken, and it is the asymmetry between the two
		// claims rather than a preference.
		//
		// A FOUND OVERFILL NEEDS ONE WITNESS: the system held more than its own
		// limit allows, once, and no further sampling makes that less true.
		//
		// AN ABSENCE NEEDS AS MANY AS THERE ARE. "No overfill" was reported off
		// ONE answerable fan-out, and rook drove what that costs: with the
		// toy's re-check restored four times in five -- an INTERMITTENT race --
		// the stage answered `no overfill: join_event held its own limit under
		// 2 overlapping request(s)`, rc 0, with try 3 overlapping by 28.8ms so
		// togetherness held. A race that missed its window read exactly like a
		// system that has none. The k-of-n gate does not help: it guards the
		// SHRINK of a race already found, not the finding of one.
		//
		// SO EVERY DECLARED WORLD IS CONTENDED WHEN NOTHING IS FOUND, and the
		// count of answerable fan-outs is carried into the verdict's line. What
		// this does NOT do is choose a number: how many fan-outs an absence
		// needs is not derivable from anything a project declares, so the stage
		// samples what the declarations give it and SAYS how many that was. One
		// is still one, and the line says so instead of implying a verdict.
		if t.Contention.Finding() != "" {
			return v, nil
		}
	}

	// AN ANSWERABLE TRY MEANS THE SWEEP RAN OUT OF WORLDS WITH NO OVERFILL,
	// which is a verdict and not a could-not-run: the clause was answered, as
	// many times as the project declares worlds, and the answer was no. The
	// line carries the count.
	if _, ok := v.Answered(); ok {
		return v, nil
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
