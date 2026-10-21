package schedule

import (
	"fmt"
	"sort"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/handle"
)

// ExecuteArgs composes an execute request's `args` from the action's
// DECLARATION, the handle table, the walk's handle choices and the run's
// generator -- the four sources fable ruled at coop r5/72 and nothing else.
//
// IT IS ObserveArgs' SIBLING AND DELIBERATELY SHAPED LIKE IT: the values are
// the system's own ids and the inputs are handles, resolved at the moment the
// message is written, because "no run-local handle crosses the wire, in either
// direction" (docs/contract.md, the execute section). An unresolvable handle
// REFUSES rather than falling back to sending the handle itself.
//
// WHERE IT DIFFERS, AND WHY IT IS NOT AN OVERSIGHT: an execute's `args` is
// ALWAYS PRESENT, so this returns a non-nil map even for an action that
// declares none. Measured rather than assumed: contract.md's observe row
// carries "present exactly when the observation declares any" and its execute
// row does not, and the structs agree -- ObserveRequest.Args is
// `json:"args,omitempty"` and ExecuteRequest.Args is `json:"args"`. So an
// absent-when-empty execute would need the tag and the contract row to move
// together, which is a wire version and not this function's business. Today's
// walk sends `{}` for an action with no arguments and that stays true.
//
// THE DRAWS ARE CONSUMED IN DECLARATION ORDER, which is why this takes the
// Source rather than a map of already-drawn values: a map has no order, and two
// attempts from one seed must consume the generator identically or the handle
// sequence issue 72's done-when compares would move for a schema reason. The
// same reasoning is why an `actor` argument consumes NO draw (fable r5/72): a
// source that drew would make the sequence depend on how many arguments an
// action happens to declare.
//
// AN ARGUMENT DECLARED TWICE IS REFUSED AT PARSE, not here: it is a defect in
// the project's declaration, so it belongs at load with the rest of them
// (internal/bindings, `arg`), and catching it again at walk time would be a
// second backstop with the same exit code rather than a check.
func ExecuteArgs(a bindings.Action, actor string, chosen map[string]string,
	src *Source, t *handle.Table) (map[string]any, error) {

	args := make(map[string]any, len(a.Args))
	// THE HANDLE ARGUMENTS ARE THE DENOMINATOR for the extra-choice check
	// below: `chosen` names handles, and only a `handle` argument takes one.
	takesHandle := make(map[string]bool, len(a.Args))

	for _, arg := range a.Args {
		switch arg.Source {
		case bindings.ArgHandle:
			takesHandle[arg.Name] = true
			h, ok := chosen[arg.Name]
			if !ok {
				return nil, fmt.Errorf("schedule: action %q declares argument %q as a handle of "+
					"kind %q and the walk chose no handle for it: a partial `args` would be "+
					"refused as a could-not-run the project did not cause", a.Name, arg.Name, arg.Of)
			}
			// THE CHOSEN HANDLE'S KIND IS CHECKED AGAINST THE DECLARED ONE. A
			// walk that handed `member_1.user#1` to an argument declared
			// `of = "event"` would send a well-formed id for the wrong thing,
			// and the adapter would answer about an entity nobody asked about
			// -- a wrong answer rather than a refusal, which is the worst of
			// the available failures.
			kind, ok := handleKind(h)
			if !ok {
				return nil, fmt.Errorf("schedule: action %q's argument %q was given %q, which is "+
					"not a run-local handle (actor.kind#n)", a.Name, arg.Name, h)
			}
			if kind != arg.Of {
				return nil, fmt.Errorf("schedule: action %q declares argument %q as a handle of "+
					"kind %q and the walk chose %q, which is a %q: the request would carry a "+
					"well-formed id for the wrong entity", a.Name, arg.Name, arg.Of, h, kind)
			}
			if t == nil {
				return nil, fmt.Errorf("schedule: action %q's argument %q names handle %q and "+
					"there is no handle table to resolve it against", a.Name, arg.Name, h)
			}
			id, err := t.Require(h)
			if err != nil {
				return nil, fmt.Errorf("schedule: action %q's argument %q names handle %q, which "+
					"this attempt's table cannot resolve, and a handle is never written to the "+
					"wire: %w", a.Name, arg.Name, h, err)
			}
			args[arg.Name] = id

		case bindings.ArgLiteral:
			// CARRIED AS DECLARED, including a false or a zero. A literal that
			// read its value through a truthiness test would silently drop
			// exactly the values a project writes down to be explicit about.
			args[arg.Name] = arg.Value

		case bindings.ArgDraw:
			if len(arg.From) == 0 {
				return nil, fmt.Errorf("schedule: action %q's argument %q draws from an empty "+
					"list: there is nothing to draw and a drawn argument cannot be omitted",
					a.Name, arg.Name)
			}
			if src == nil {
				return nil, fmt.Errorf("schedule: action %q's argument %q draws from %d value(s) "+
					"and there is no generator: a drawn argument chosen some other way would not "+
					"replay from the seed", a.Name, arg.Name, len(arg.From))
			}
			i, err := src.IntN(len(arg.From))
			if err != nil {
				return nil, fmt.Errorf("schedule: drawing action %q's argument %q: %w",
					a.Name, arg.Name, err)
			}
			args[arg.Name] = arg.From[i]

		case bindings.ArgActor:
			// THE STEP'S OWN ACTOR, which exists because the wire carries no
			// `actor` field -- the contract's execute row says so outright
			// ("There is no `actor` field here") and ibis measured the need
			// from the other end at coop r5/70: cmd/toy keys its sessions by
			// user NAME, so a session for member_1 exists only once some
			// create_user has made a user called member_1.
			if actor == "" {
				return nil, fmt.Errorf("schedule: action %q declares argument %q as the acting "+
					"actor's name and no actor was given: the step would send an empty name for "+
					"whoever is taking it", a.Name, arg.Name)
			}
			args[arg.Name] = actor

		default:
			return nil, fmt.Errorf("schedule: action %q's argument %q declares source %q, which "+
				"is not one of %v", a.Name, arg.Name, arg.Source, bindings.ArgSources)
		}
	}

	// EXTRA CHOICES ARE REFUSED, not dropped, for ObserveArgs' reason: a walk
	// that chose a handle for an argument the action does not declare as a
	// handle has a defect the engine can see, and dropping it would send a
	// request that looks well-formed.
	extra := make([]string, 0)
	for name := range chosen {
		if !takesHandle[name] {
			extra = append(extra, name)
		}
	}
	if len(extra) > 0 {
		sort.Strings(extra)
		return nil, fmt.Errorf("schedule: action %q declares no handle argument named %v, which "+
			"the walk chose a handle for: either the declaration moved or the walk is choosing "+
			"for an argument it does not supply", a.Name, extra)
	}
	return args, nil
}
