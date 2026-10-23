package schedule

import (
	"fmt"
	"regexp"
	"sort"
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

// HandleKindsNeeded is every handle KIND an action needs before it can be
// taken: the kinds its `handle` arguments declare, then the kinds its UI flow
// refers to, first appearance first and duplicates removed.
//
// THE ARGUMENTS COME FIRST because their failure is earlier: an unresolvable
// argument means the execute request cannot be composed at all, while an
// unresolvable flow placeholder means it cannot be navigated. A refusal naming
// the first missing kind should name the one that stops the step soonest.
func HandleKindsNeeded(a bindings.Action) []string {
	var out []string
	seen := map[string]bool{}
	add := func(kind string) {
		if kind == "" || seen[kind] {
			return
		}
		seen[kind] = true
		out = append(out, kind)
	}
	for _, arg := range a.Args {
		if arg.Source == bindings.ArgHandle {
			add(arg.Of)
		}
	}
	for _, kind := range FlowPlaceholders(a) {
		add(kind)
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
// THERE ARE TWO WAYS AN ACTION NAMES A HANDLE AND THIS COUNTS BOTH, which is
// R5.4's correction to this function. Until the argument schema landed, a flow
// placeholder was the only one -- the comment here said so, and said it as the
// reason the check was complete -- and `[[action.args]]` with `source =
// "handle"` is now the second and the more direct: the request cannot be
// WRITTEN without it, while a flow placeholder cannot be NAVIGATED without it.
// Both are counted by HandleKindsNeeded, and an action needing neither needs
// nothing.
//
// AND IT ASKS WHOSE HANDLE IT IS, which it did not until R5.4's close. The
// comment here used to state the gap and promise a filter "one layer up":
//
//	the table is global to the attempt, so a kind bound by anybody satisfies
//	this, while `[[action.args]]`'s `of` is documented as resolving to "an id
//	the ACTOR knows of" ... an action whose argument names an event only
//	somebody else can see is applicable here and would be filtered one layer up
//
// That filter was never built, and the walk reached the gap: seed 6 step 8 had
// `guest_1` join `member_1.event#1`, an event nothing had told it of, because
// `knows_of_subject` passed on guest_1's OWN event while the draw took
// member_1's. ibis drove it (coop r5/111), ratchet found this comment and read
// it correctly as a declared narrowing rather than an oversight (r5/109), and
// fable ruled the scoping into the engine (r5/114): "the precondition and the
// argument quantify over one set".
//
// `known` IS THE ACTOR'S, from engine.KnownHandlesAt, and it is REQUIRED rather
// than optional. A nil set means the actor knows nothing, so every
// handle-needing action is inapplicable to it -- which is the correct answer
// for a fresh actor and, more to the point, fails CLOSED. A nil meaning
// "unscoped" would have made the one caller that forgot to pass it silently
// restore the defect.
//
// IT IS STILL ONLY THE TABLE'S QUESTION PLUS THE ACTOR'S: whether the project
// PERMITS the action is the statute's, and whether the role holds the context
// is HoldsContext's. Three separate halves, each refusing in its own terms.
func Resolvable(a bindings.Action, t *handle.Table, known []string) (bool, string) {
	needs := HandleKindsNeeded(a)
	if len(needs) == 0 {
		return true, ""
	}
	for _, kind := range needs {
		if len(UsableHandles(t, known, kind)) == 0 {
			return false, kind
		}
	}
	return true, ""
}

// UsableHandles is the handles of one kind this actor may name: bound in the
// attempt's table AND named by a claim the actor knows.
//
// IT IS ONE FUNCTION BECAUSE IT HAS TWO CALLERS AND THEY MUST NOT DRIFT.
// Resolvable asks whether the set is non-empty and chooseHandles draws from it;
// two intersections computed apart would let the pool offer an action whose
// draw then refuses -- the drawn-and-then-refused shape Resolvable exists to
// prevent, which is exactly how this defect arrived the first time
// (chooseHandles' own error says "Resolvable should have held this action out
// of the pool, so applicability and composition disagree").
//
// SORTED, so a caller drawing from it draws from a set and not from a map's
// order, which is D1's rule and the reason chooseHandles sorted before.
func UsableHandles(t *handle.Table, known []string, kind string) []string {
	if t == nil {
		return nil
	}
	is := make(map[string]bool, len(known))
	for _, h := range known {
		is[h] = true
	}
	var out []string
	for _, h := range t.Handles() {
		if k, ok := handleKind(h); !ok || k != kind {
			continue
		}
		if !is[h] {
			continue
		}
		out = append(out, h)
	}
	sort.Strings(out)
	return out
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
