package concurrency

import (
	"fmt"
	"sort"
	"strings"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
)

// Subject is one action the stage will contend, with the two facts the project
// declared about it.
type Subject struct {
	Action bindings.Action
	// LimitFrom is the action and argument `contended.limit` names, split.
	LimitAction string
	LimitArg    string
	// Held is the declared query that reads what the system holds.
	Held string
}

// Subjects is every action a project declares as contended, in a fixed order,
// and the refusal for a project that declares none.
//
// EVERY ACTION THAT DECLARES IT AND NO OTHER, which is fable's ruling at coop
// rumor/1525: the stage's subject is DECLARED and never derived. The derivation
// that looks available is the dangerous one -- examples/toy's
// `store_event_count` declares `of = "event.joined"` and three actions emit
// that claim type, of which `accept_invitation` holds its lock across its own
// check-then-act, so a stage choosing by claim type can select the CONTROL and
// report no overfill, which is what a correct system reports.
//
// A PROJECT DECLARING NONE IS REFUSED BY NAME, which is 108's shape: a stage
// that ran over an empty subject list would exit having contended nothing and
// read exactly like a stage that contended everything and found nothing. The
// refusal names the table so the author knows what to write rather than what
// went wrong.
//
// ORDER IS BY ACTION NAME so two runs of one project contend in one order, and
// a record comparing them is comparing the same thing twice.
func Subjects(b bindings.Bindings) ([]Subject, error) {
	var out []Subject
	for _, a := range b.Actions {
		if a.Contended.Limit == "" && a.Contended.Held == "" {
			continue
		}
		action, arg, ok := strings.Cut(a.Contended.Limit, ".")
		if !ok || action == "" || arg == "" {
			// `rumor validate` refuses this before a run, by name, as a cell
			// of arm 19. Reaching it here means the stage was handed a project
			// validate would have refused, so it says which action and stops
			// rather than contending against a limit it cannot read.
			return nil, fmt.Errorf("action %q declares contended.limit %q, which does not name "+
				"an action's argument as `<action>.<arg>`: the limit is read from the argument "+
				"that set it, and `rumor validate` refuses this before a run",
				a.Name, a.Contended.Limit)
		}
		out = append(out, Subject{Action: a, LimitAction: action, LimitArg: arg, Held: a.Contended.Held})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no action declares `contended`, so this stage has nothing to " +
			"contend: an action that two callers can race says so with " +
			"`contended = { limit = \"<action>.<arg>\", held = \"<query>\" }`, and a run over no " +
			"subject would report finding nothing exactly as a run over a sound system does")
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Action.Name < out[j].Action.Name })
	return out, nil
}

// Width is how many requests a subject's fan-out issues: its limit plus one.
//
// LIMIT PLUS ONE AND NOT MORE, which is fable's wording and has a reason beyond
// economy: the smallest fan-out that can exceed a limit is the one that proves
// the check and the insert are not one step, and every request past it adds a
// contender whose refusal says nothing new.
//
// AND IT DOES NOT KEEP THE TRACE UNDER FIVE ACTIONS, which this comment claimed
// -- "two users, the event, and the fan-out as one" -- until the trace existed
// to be counted. The creator needs its own user to create anything, and it must
// not be one of the contenders: at the toy's `visible_to = ["admin"]` the
// creator can see the event and a member cannot, so a fan-out including it is
// answerable-and-CLEAN at the first world and the sweep stops there, reporting
// no overfill over a system whose race is two tries away. So the shipped toy's
// trace is five steps -- three create_users, the create_event, the fan-out --
// and issue 1's figure is about the WALK's traces rather than a stage's. The
// count is in world.Trace's own comment, beside the thing that builds it.
func Width(limit int) int { return limit + 1 }
