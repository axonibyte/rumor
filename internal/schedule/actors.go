package schedule

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/handle"
	"forge.axonibyte.com/axonibyte/rumor/internal/platform"
)

// Actors names the run's actors from platform.toml's [[venire]].
//
// <role>_<n>, n FROM 1 PER ROLE, IN VENIRE ORDER, ruled at coop r5/58.
// [[venire]] carries a role and a count and nothing else, so a run has to name
// its own actors -- and HANDLES EMBED THE ACTOR (`member_1.event#1`), which
// makes actor naming decide the handle sequence that issue 72's done-when
// compares between two attempts.
//
// WHY NOT A HYPHEN: internal/handle's shape is ^[a-z][a-z0-9_]*\., so
// `member-1` is not a well-formed actor segment. The vocabulary ruled the
// hyphen out before this function existed, which is why the recommendation was
// made with that measured rather than guessed.
//
// WHY VENIRE ORDER AND NOT THE DRAW'S: the names have to be a function of the
// PROJECT, not of the seed. Two attempts from one seed must agree on who
// `member_2` is before either of them draws anything, or the handle sequence
// they are compared by would move with the draw.
func Actors(venire []platform.Member) ([]string, error) {
	if len(venire) == 0 {
		return nil, fmt.Errorf("schedule: the platform declares no [[venire]], so the run has no " +
			"actors: a walk is a sequence of actions by actors and there is nobody to take one")
	}
	var out []string
	seen := map[string]bool{}
	for _, m := range venire {
		if m.Role == "" {
			return nil, fmt.Errorf("schedule: a [[venire]] entry declares no role, and the actor's " +
				"name is derived from it")
		}
		if m.Count <= 0 {
			return nil, fmt.Errorf("schedule: [[venire]] gives role %q a count of %d: a role nobody "+
				"holds cannot take an action, and a run drawing from an empty role would stall "+
				"rather than say so", m.Role, m.Count)
		}
		if seen[m.Role] {
			return nil, fmt.Errorf("schedule: role %q appears twice in [[venire]]: the second "+
				"numbering would restart at 1 and collide with the first's actors", m.Role)
		}
		seen[m.Role] = true
		for i := 1; i <= m.Count; i++ {
			name := m.Role + "_" + strconv.Itoa(i)
			// THE NAME IS CHECKED AGAINST THE VOCABULARY'S SHAPE rather than
			// trusted: a role spelled with a character the handle form refuses
			// would produce actors whose handles cannot be formed, and the
			// failure would surface later as an unparseable handle instead of
			// here as a bad role.
			if !handle.Valid(name + ".thing#1") {
				return nil, fmt.Errorf("schedule: role %q gives the actor name %q, which cannot "+
					"carry a run-local handle: the vocabulary's handle shape is actor.type#n with "+
					"the actor lowercase alphanumeric and underscores", m.Role, name)
			}
			out = append(out, name)
		}
	}
	return out, nil
}

// placeholder is a flow step's `{name}` reference.
var placeholder = regexp.MustCompile(`\{([a-z][a-z0-9_]*)\}`)

// FlowPlaceholders is the handle KINDS an action's UI flow refers to, in first
// appearance order with duplicates removed.
//
// The toy's join_event_ui navigates to `/ui/events/{event}`, so the kind is
// `event` and the step cannot be taken until some event handle resolves to an
// id the browser can be pointed at.
func FlowPlaceholders(a bindings.Action) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range a.Flow {
		for _, field := range []string{s.To, s.Value, s.Target, s.Until} {
			for _, m := range placeholder.FindAllStringSubmatch(field, -1) {
				if !seen[m[1]] {
					seen[m[1]] = true
					out = append(out, m[1])
				}
			}
		}
	}
	return out
}

// Resolvable reports whether every handle kind this action needs has a handle
// that resolves to an id, and names the first kind that does not.
//
// RESOLVABILITY IS PART OF APPLICABILITY, ruled at coop r5/58. It is the C1
// refusal arriving one layer earlier: ObserveArgs refuses a handle the table
// cannot resolve, and by then the walk has already chosen the step. A suborn
// mints a handle for a claim about an action that NEVER RAN, so that handle
// resolves to no id at all -- and an action needing it must be inapplicable
// rather than drawn and then refused, because the walk must never ask the wire
// for a thing that does not exist.
//
// THE TABLE IS THE ORACLE AND THAT IS THE WHOLE MECHANISM: handle.Table holds
// only handles something BOUND to an id, and a suborn never binds. So
// "resolves" and "is in the table" are the same question, and there is no
// second bookkeeping of unresolvable handles to drift from the first.
//
// An action with no flow placeholders needs nothing, which is every action in
// the toy but join_event_ui: the bindings schema gives actions `creates` and a
// `flow`, never declared args, so a flow reference is the only way an action
// names a handle today. An observation's `args` is the other half and
// ObserveArgs holds it.
func Resolvable(a bindings.Action, t *handle.Table) (bool, string) {
	needs := FlowPlaceholders(a)
	if len(needs) == 0 {
		return true, ""
	}
	if t == nil {
		return false, needs[0]
	}
	have := map[string]bool{}
	for _, h := range t.Handles() {
		if kind, ok := handleKind(h); ok {
			have[kind] = true
		}
	}
	for _, kind := range needs {
		if !have[kind] {
			return false, kind
		}
	}
	return true, ""
}

// handleKind is the type segment of a run-local handle: the `event` of
// `member_1.event#1`.
func handleKind(h string) (string, bool) {
	dot := strings.IndexByte(h, '.')
	hash := strings.LastIndexByte(h, '#')
	if dot < 0 || hash < 0 || hash <= dot+1 {
		return "", false
	}
	return h[dot+1 : hash], true
}
