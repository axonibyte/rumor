package concurrency

import (
	"fmt"
	"sort"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/setup"
)

// SetupFor is the actions that must run before a subject's fan-out, in order,
// and the refusals for a subject whose world cannot be built from what the
// project declares.
//
// THE ORDER ITSELF IS internal/setup's, which 108 derived for the hostile
// corpus and this stage asks for a different reason: a fan-out cannot contend
// an event nothing created, and the ids its requests carry are the system's
// own. What is asked for here is narrower than the corpus's -- the handle
// kinds THE SUBJECT takes -- because the fan-out sends one action and not a
// surface.
//
// AND THE LIMIT ACTION MUST BE IN IT, which is this stage's own check and the
// reason there is a function here at all. `contended.limit` names the argument
// the limit is READ from, so a setup that never ran that action would leave the
// limit read from a request nobody sent: the stage would fan out against a
// number it chose and a world somebody else built. The declaration is
// `create_event.capacity` and the chain is derived from `creates`, so the two
// agree on the shipped toy and CAN disagree -- a project naming its limit on a
// second creator of the same kind resolves to the first, and that is the
// refusal below rather than a silent contention at the wrong capacity.
//
// THE SUBJECT IS NOT ITS OWN SETUP. An action that both creates the kind it
// contends and contends it would have the setup spend the room the fan-out
// exists to fill -- one acceptance already in before the barrier releases, so
// the room and the width would not be what the verdict says they are. It is
// refused by name rather than filtered out, because removing it would leave the
// handle it was resolving unmade.
func SetupFor(b bindings.Bindings, s Subject) (Chain, error) {
	limit, ok := actionNamed(b, s.LimitAction)
	if !ok {
		return Chain{}, fmt.Errorf("action %q reads its limit from %s.%s and no action named %q is "+
			"declared: the stage would fan out against a limit nothing set",
			s.Action.Name, s.LimitAction, s.LimitArg, s.LimitAction)
	}
	arg, ok := argNamed(limit, s.LimitArg)
	if !ok {
		names := make([]string, 0, len(limit.Args))
		for _, a := range limit.Args {
			names = append(names, a.Name)
		}
		sort.Strings(names)
		return Chain{}, fmt.Errorf("action %q reads its limit from %s.%s and %s declares no argument "+
			"%q (it declares %v): `rumor validate` refuses this before a run",
			s.Action.Name, s.LimitAction, s.LimitArg, s.LimitAction, s.LimitArg, names)
	}
	// THE LIMIT IS A VALUE THE SETUP SENDS, so it has to come from a source the
	// setup chooses. A `handle` or an `actor` argument carries an id or a name:
	// there is no number in either to be the room, and a stage that read one
	// would be fanning out `width` requests against a limit it invented. The
	// two that remain are the two the project can write down -- a literal the
	// author fixed, or a draw the SEED chooses, which is the toy's `capacity`
	// over [1, 2, 3].
	if arg.Source != bindings.ArgDraw && arg.Source != bindings.ArgLiteral {
		return Chain{}, fmt.Errorf("action %q reads its limit from %s.%s, whose source is %q: a limit "+
			"is a value the setup sends, so it can only be a literal the project fixed or a draw "+
			"the seed chooses", s.Action.Name, s.LimitAction, s.LimitArg, arg.Source)
	}

	// EVERY HANDLE THE SUBJECT TAKES MUST BE THE CONTENDED THING, and this
	// refusal is a bound on the stage rather than a defect in a project.
	//
	// THE FAN-OUT SENDS ONE SET OF ARGUMENTS TO N CONTENDERS. That is right for
	// the thing being contended -- the point is that they all name the SAME
	// event -- and wrong for anything that belongs to one caller. Take
	// `accept_invitation`: its handle is an `invitation`, the limit is the
	// event's capacity, and one invitation sent to two contenders has the
	// second answered "already accepted". Accepted would be 1 of 2 into room
	// for 1, which is neither a finding nor VACUOUS by any of Contention's four
	// conditions -- so the stage would report `no overfill` over a world in
	// which only one request could ever have succeeded. A control that is the
	// case it controls for, arriving through a door the vacuity clause does not
	// watch.
	//
	// THE CONDITION IS DERIVABLE AND THAT IS WHY IT IS A REFUSAL AND NOT A
	// COMMENT: the contended thing is what the limit's action CREATES, so a
	// handle argument of any other kind is per-caller by construction. The
	// shipped toy passes it -- `join_event` takes an `event` and `create_event`
	// makes one -- and nothing in the tree trips it, which is also why sending
	// per-contender arguments is not built here: it would be a mechanism with
	// no project behind it. The refusal says what the stage cannot do instead
	// of answering a clause it could not have answered.
	limited := map[string]bool{}
	for _, kind := range limit.Creates {
		limited[kind] = true
	}
	for _, arg := range s.Action.Args {
		if arg.Source != bindings.ArgHandle || limited[arg.Of] {
			continue
		}
		// THE WIDTH IS NOT IN THIS SENTENCE, because it is not known here: the
		// limit is read later, from the argument the setup sends. A figure put
		// in by hand would be a count of contenders this run may not have.
		return Chain{}, fmt.Errorf("action %q takes a handle of kind %q, and %s -- the action its "+
			"limit is read from -- creates %v: a fan-out sends ONE set of arguments to EVERY "+
			"contender, so a thing belonging to one of them would have the others refused for a "+
			"reason that is not the limit, and the clause would be answered on a world where only "+
			"one request could succeed",
			s.Action.Name, arg.Of, s.LimitAction, limit.Creates)
	}

	order, err := setup.Order(b, needs(s.Action))
	if err != nil {
		return Chain{}, fmt.Errorf("the setup for %q: %w", s.Action.Name, err)
	}
	// THERE IS NO CHECK HERE THAT THE SUBJECT IS NOT ITS OWN SETUP, and the
	// reason is that no project can reach one. A chain containing the subject
	// would mean the subject provides a kind reachable from the kinds the
	// subject takes, and internal/setup resolves depth-first from each needed
	// kind with the path it came by: the moment the subject is chosen as a
	// provider, its own needed kind is already on that path, so the CYCLE
	// refusal fires first and names the path. I wrote the check before I tried
	// to build a fixture for it, and every shape I could write came out as a
	// cycle -- which is a refusal armed by nothing, the thing I was corrected
	// on at r8/27. The cycle arm in internal/setup is what guards it.

	// THE BOOTSTRAP IS SPLIT OUT AND RUN PER ACTOR, which is the half the
	// handle graph cannot see -- and 108 has the measurement for it in
	// internal/setup's own words: the toy's create_event failed until
	// create_user ran, because "a session belongs to a user that exists", and
	// that dependency is declared in `requires` as a STATUTE RULE which no
	// amount of reading `creates` could resolve. The fuzz stage got it for
	// free: its corpus needs a `user` handle, so create_user was already in the
	// chain and only had to be hoisted. A fan-out does not -- join_event takes
	// an `event` and nothing else -- so the chain derived from the subject's own
	// arguments contains no bootstrap at all, and the first setup step would be
	// refused for a reason the fan-out would then report as its own.
	//
	// THE CONDITION IS THE ONE internal/setup ALREADY TRUSTS, PLUS THE ACTOR.
	// Context `none` and no handle argument is what makes an action able to go
	// first; an `actor`-sourced argument is what makes it mean something
	// DIFFERENT for each actor, which is why this one is run N times and the
	// rest once. An action with no actor argument would send N identical
	// requests, and a creating action answers the second of those with a
	// refusal -- so running it per actor would manufacture a setup failure.
	var chain Chain
	for _, a := range b.Actions {
		if perActor(a) {
			chain.PerActor = append(chain.PerActor, a)
		}
	}
	for _, a := range order {
		if !perActor(a) {
			chain.Once = append(chain.Once, a)
		}
	}
	for _, a := range chain.Once {
		if a.Name == s.LimitAction {
			return chain, nil
		}
	}
	ran := make([]string, 0, len(order))
	for _, a := range order {
		ran = append(ran, a.Name)
	}
	return Chain{}, fmt.Errorf("action %q reads its limit from %s.%s and the setup derived from "+
		"`creates` runs %v: %s never runs, so the limit would be read from an argument no "+
		"request carried. The chain comes from the handle kinds %q takes, so either the limit "+
		"names an action that does not create what is contended, or another action creates that "+
		"kind first and provides it",
		s.Action.Name, s.LimitAction, s.LimitArg, ran, s.LimitAction, s.Action.Name)
}

// Chain is a subject's setup in the two shapes a fan-out has to run it in.
//
// TWO LISTS AND NOT ONE, because the number of times a step runs is part of
// what it means. PerActor is run once for every actor that will act -- the
// host and every contender -- and Once is run by the host alone, in order. A
// single list would have left the caller deciding that, which is the decision
// this type exists to make in the place that can read the declarations.
type Chain struct {
	// PerActor is the bootstrap: context `none`, no handle argument, and an
	// `actor`-sourced argument, so it says something different for each actor.
	PerActor []bindings.Action
	// Once is the world the contention happens in, in topological order, and
	// it contains the action the limit is read from.
	Once []bindings.Action
}

// Names is the chain's steps in the order a record prints them, the per-actor
// ones marked, so a reader can see the setup a run took without the stage
// having to narrate it twice.
func (c Chain) Names() []string {
	out := make([]string, 0, len(c.PerActor)+len(c.Once))
	for _, a := range c.PerActor {
		out = append(out, a.Name+" (per actor)")
	}
	for _, a := range c.Once {
		out = append(out, a.Name)
	}
	return out
}

// perActor is the declared condition for a step every actor takes for itself.
func perActor(a bindings.Action) bool {
	if a.Context != "none" {
		return false
	}
	actor := false
	for _, arg := range a.Args {
		if arg.Source == bindings.ArgHandle {
			return false
		}
		if arg.Source == bindings.ArgActor {
			actor = true
		}
	}
	return actor
}

// needs is the handle kinds a subject's own request carries.
//
// THE SUBJECT's ARGUMENTS AND NOT THE PROJECT's, which is the narrowing that
// makes this stage's setup shorter than the fuzz stage's: a fan-out sends ONE
// action, so a kind nothing in that action takes is a world the stage would
// build and never use -- and every setup step is a request that can fail for a
// reason the fan-out then reports as its own.
func needs(a bindings.Action) []string {
	set := map[string]bool{}
	for _, arg := range a.Args {
		if arg.Source == bindings.ArgHandle && arg.Of != "" {
			set[arg.Of] = true
		}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func actionNamed(b bindings.Bindings, name string) (bindings.Action, bool) {
	for _, a := range b.Actions {
		if a.Name == name {
			return a, true
		}
	}
	return bindings.Action{}, false
}

func argNamed(a bindings.Action, name string) (bindings.Arg, bool) {
	for _, arg := range a.Args {
		if arg.Name == name {
			return arg, true
		}
	}
	return bindings.Arg{}, false
}
