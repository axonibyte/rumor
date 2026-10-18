// Package bindings reads a package's `bindings.toml`: what a project binds its
// statute to -- its actions, the claim types they emit, their preconditions as
// statute references, its observations, when each claim type settles, and which
// session contexts each role may hold.
//
// docs/contract.md's "Bindings schema" section is this package's prose, and
// internal/decl is the strict reading both declaration files share.
//
// THE RULE THAT DECIDES WHAT IS REQUIRED HERE, and it is the whole boundary
// between this package and `rumor validate` (R4.3): **a parser must not refuse
// what a validator is built to refuse.** Eight of validate's eighteen arms are
// about bindings that are well formed and wrong -- an action with no
// precondition, a rule referencing a claim type no binding emits, an eventual
// type with no bound, a browser-stage action with no UI flow. If this package
// demanded `requires`, arm 3 could never fire: the file would not parse, the
// run would report could-not-run, and the arm's fixture would be testing the
// parser. So `emits`, `requires`, `flow`, `bound` and `[uses]` are all OPTIONAL
// here and refused there, and what is required here is only what makes the file
// readable as bindings at all.
package bindings

import (
	"fmt"
	"time"

	"forge.axonibyte.com/axonibyte/rumor/internal/decl"
	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

// Refusal is internal/decl's, for the reason given there: the rules for reading
// a declaration are shared with platform.toml and a second copy would drift.
// The file's name is the loader's to add -- this package is handed a decoded
// document and does not know what it was read from.
type Refusal = decl.Refusal

// Timing is when a claim type settles.
const (
	Immediate = "immediate"
	Eventual  = "eventual"
)

// Quiesce is the bound meaning "until the system settles", as opposed to a
// duration. R6 builds the probe; the word is fixed here because the arm on an
// eventual type with no bound has to tell a missing bound from this one.
const Quiesce = "quiesce"

// Step kinds in a UI flow (R9).
const (
	StepNavigate = "navigate"
	StepFill     = "fill"
	StepClick    = "click"
	StepWait     = "wait"
)

// Bindings is one package's declaration.
type Bindings struct {
	Actions      []Action
	Observations []Observation
	Claims       map[string]Claim
	Roles        []Role

	// Reserved for the phase that owns each, and carried as read: the keys
	// inside them belong to R6, R7 and R20, and checking them against a key set
	// this phase would be inventing is how a schema acquires fields nothing
	// consumes. A malformed TABLE is still refused, which is issue 63's rule.
	Clock    map[string]any
	Quiesce  map[string]any
	Deadline map[string]any
	Seeding  map[string]any

	// Uses is the project's ASK: the faults, volume targets and upgrades it
	// wants. `describe`'s capabilities is what the adapter can actually do, and
	// validate's arm pairs them -- live against static, which is why neither
	// side is written in the other's file.
	Uses Uses
}

// Action is one thing an actor can do.
type Action struct {
	Name     string
	Context  string
	Emits    []string
	Requires []string
	Creates  []string
	Response map[string]any
	Flow     []Step

	// Touches names the stateful components this action MUTATES, which is the
	// other half of the rollback-coverage arm: a component is otherwise
	// nameable only on a `direct` observation, so an action that dirties an
	// uncovered component and is never read directly fell outside an arm whose
	// wording is about bindings that touch one (ibis, coop r4/23).
	//
	// OPTIONAL, AND THE COST IS STATED RATHER THAN DESIGNED AWAY: an action
	// naming nothing is read as mutating nothing, exactly as an action with no
	// `requires` is read as having no precondition. Arm 3 refuses the second;
	// no arm refuses the first, and a nineteenth arm is not this package's to
	// invent -- the count is fable's.
	Touches []string
}

// Step is one step of a UI flow.
type Step struct {
	Step   string
	To     string
	Field  string
	Value  string
	Target string
	Until  string
}

// Observation is one named observation and how it is taken.
//
// IT CARRIES NO POSITION, and that is settled rather than omitted: positions
// are the ledger's, assigned on append, and the validate arm about an
// observation lacking a position or a source is static over the EXAMPLES' cases,
// whose observations carry both (ibis, coop r4/20).
type Observation struct {
	Name   string
	Source wire.Source
	Of     string

	// Component is the part of the system a `direct` observation reads. It is
	// how a binding names a stateful component at all, which is what validate's
	// rollback-coverage arm compares against `describe`'s components and
	// platform.toml's [[rollback]].covers (fable's D11).
	Component string
}

// Claim is when one claim type settles.
type Claim struct {
	Timing string
	Bound  string
}

// Role is a role from the statute and the session contexts an actor of it may
// hold. The reachability arm -- a browser-stage actor that could reach an API
// action -- is computed over these and the actions' contexts.
type Role struct {
	Name     string
	Contexts []string
}

// Uses is the project's declared asks (R19, R20, R21).
type Uses struct {
	Faults  []string
	Volume  []string
	Upgrade []string
}

// Report is what reading a document produced: the bindings, and every reason it
// is not one. All of them rather than the first, as internal/platform and
// `rumor attaint` both do: a file that trips three refusals is a different
// event from one that trips the refusal it was written to trip.
type Report struct {
	Bindings Bindings
	Refusals []Refusal
}

// OK reports whether the document is a set of bindings.
func (r Report) OK() bool { return len(r.Refusals) == 0 }

// Load reads a package's bindings.toml and parses it. The decoder is the
// caller's, and the two error shapes are internal/platform's: an unreadable
// file is could-not-run, a readable file that is not bindings is a verdict.
func Load(path string, dec decl.Decoder) (Report, error) {
	doc, err := decl.Load(path, dec)
	if err != nil {
		return Report{}, err
	}
	return Parse(doc), nil
}

// Parse reads a decoded document.
func Parse(doc map[string]any) Report {
	r := decl.New(doc)
	var b Bindings

	for _, a := range r.Tables("action") {
		action := Action{
			Name:     a.Str("name", true),
			Context:  a.Str("context", true),
			Emits:    a.Strings("emits", false),
			Requires: a.Strings("requires", false),
			Creates:  a.Strings("creates", false),
			Touches:  a.Strings("touches", false),
		}
		if t, ok := a.Table("response", false); ok {
			action.Response = map[string]any{}
			for _, k := range t.Keys() {
				action.Response[k] = t.Any(k)
			}
			t.Done()
		}
		for _, s := range a.Tables("flow") {
			action.Flow = append(action.Flow, step(s))
		}
		a.Done()
		b.Actions = append(b.Actions, action)
	}
	if len(b.Actions) == 0 {
		r.Refuse("action", "no actions: a binding with nothing to do binds nothing. "+
			"This is a parse refusal; validate's vacuity guard is the one that reports it as a refusal rather than a pass")
	}

	for _, o := range r.Tables("observation") {
		obs := Observation{
			Name:   o.Str("name", true),
			Source: wire.Source(o.Str("source", true)),
			Of:     o.Str("of", true),
		}
		switch obs.Source {
		case wire.SourceAPI, wire.SourceUI:
			if o.Has("component") {
				o.Refuse("component", fmt.Sprintf(
					"a %s observation reads what an actor can see, so it names no component of the project's own store", obs.Source))
				o.Str("component", false)
			}
		case wire.SourceDirect:
			// REQUIRED FOR direct AND FOR NOTHING ELSE. A direct observation is
			// the engine reading the project's own store, and this field is the
			// only place a binding names a component -- without it validate's
			// rollback-coverage arm has nothing to compare and passes
			// vacuously. It is a PARSE refusal and not that arm: the arm is
			// about a named component no provider covers.
			obs.Component = o.Str("component", true)
		default:
			o.Refuse("source", fmt.Sprintf("%q: not one of api, ui, direct", obs.Source))
		}
		o.Done()
		b.Observations = append(b.Observations, obs)
	}

	b.Claims = map[string]Claim{}
	if claims, ok := r.Table("claim", false); ok {
		for _, name := range claims.Keys() {
			c, _ := claims.Table(name, true)
			claim := Claim{Timing: c.Str("timing", true), Bound: c.Str("bound", false)}
			switch claim.Timing {
			case Immediate:
				if claim.Bound != "" {
					c.Refuse("bound", "an immediate claim type settles at its action, so a bound says nothing")
				}
			case Eventual:
				// THE BOUND IS NOT REQUIRED HERE. "An eventual claim type with
				// no bound" is one of validate's arms, and a parser that
				// refused it would make that arm unreachable.
				if claim.Bound != "" && claim.Bound != Quiesce {
					if _, err := time.ParseDuration(claim.Bound); err != nil {
						c.Refuse("bound", fmt.Sprintf(
							"%q: wanted a duration such as 30s, or %q", claim.Bound, Quiesce))
					}
				}
			case "":
				// already refused as a missing key
			default:
				c.Refuse("timing", fmt.Sprintf("%q: wanted %q or %q", claim.Timing, Immediate, Eventual))
			}
			c.Done()
			b.Claims[name] = claim
		}
		claims.Done()
	}

	for _, role := range r.Tables("role") {
		entry := Role{Name: role.Str("name", true), Contexts: role.Strings("contexts", true)}
		role.Done()
		b.Roles = append(b.Roles, entry)
	}

	for _, reserved := range []struct {
		key  string
		into *map[string]any
	}{
		{"clock", &b.Clock},
		{"quiesce", &b.Quiesce},
		{"deadline", &b.Deadline},
		{"seeding", &b.Seeding},
	} {
		if t, ok := r.Table(reserved.key, false); ok {
			carried := map[string]any{}
			for _, k := range t.Keys() {
				carried[k] = t.Any(k)
			}
			t.Done()
			*reserved.into = carried
		}
	}

	if uses, ok := r.Table("uses", false); ok {
		b.Uses = Uses{
			Faults:  uses.Strings("faults", false),
			Volume:  uses.Strings("volume", false),
			Upgrade: uses.Strings("upgrade", false),
		}
		uses.Done()
	}

	r.Done()

	// CROSS-KEY REFUSALS, after every key has been read. Each needs two parts of
	// the file, so none can be checked while reading either.
	//
	// WHAT IS NOT HERE IS THE POINT: an action's `emits` naming a claim type the
	// [claim] table does not describe is NOT refused, because "a rule references
	// a claim type no binding emits" and its mirror are validate's arms over the
	// statute and the bindings TOGETHER, and this package sees only one of the
	// two.
	seen := map[string]bool{}
	for _, a := range b.Actions {
		if seen[a.Name] {
			r.Refuse("action", fmt.Sprintf("%q is declared twice: an action name is how `execute` asks for it", a.Name))
		}
		seen[a.Name] = true
	}
	seenObs := map[string]bool{}
	for _, o := range b.Observations {
		if seenObs[o.Name] {
			r.Refuse("observation", fmt.Sprintf("%q is declared twice: an observation name is `observe`'s query", o.Name))
		}
		seenObs[o.Name] = true
	}
	seenRole := map[string]bool{}
	for _, role := range b.Roles {
		if seenRole[role.Name] {
			r.Refuse("role", fmt.Sprintf("%q is declared twice", role.Name))
		}
		seenRole[role.Name] = true
	}

	if found := r.Refusals(); len(found) > 0 {
		return Report{Refusals: found}
	}
	return Report{Bindings: b}
}

// step reads one UI flow step.
//
// EACH KIND'S OWN FIELDS AND NO OTHERS, because a `click` carrying a `value` is
// either a step written for the wrong kind or a field the driver will ignore,
// and an ignored field in a flow is a flow that does not do what it says.
func step(s *decl.Reader) Step {
	out := Step{Step: s.Str("step", true)}
	switch out.Step {
	case StepNavigate:
		out.To = s.Str("to", true)
		if len(out.To) > 0 && out.To[0] != '/' {
			// THE PATH AND NOT A URL: the base belongs to the platform's
			// adapter, so a flow carrying a host would bind this statute to one
			// deployment.
			s.Refuse("to", fmt.Sprintf("%q: wanted a path beginning with /, not a URL -- the base is the adapter's", out.To))
		}
	case StepFill:
		out.Field = s.Str("field", true)
		out.Value = s.Str("value", true)
	case StepClick:
		out.Target = s.Str("target", true)
	case StepWait:
		out.Until = s.Str("until", true)
		// A CONDITION AND NEVER A DURATION. R9's flake policy says there are no
		// fixed sleeps, so a wait that parses as a duration is refused by name
		// rather than slept through.
		if out.Until != "" {
			if _, err := time.ParseDuration(out.Until); err == nil {
				s.Refuse("until", fmt.Sprintf(
					"%q is a duration: a wait step takes a CONDITION, because the browser stage has no fixed sleeps (R9)", out.Until))
			}
		}
	case "":
		// already refused as a missing key
	default:
		s.Refuse("step", fmt.Sprintf("%q: wanted one of %s, %s, %s, %s",
			out.Step, StepNavigate, StepFill, StepClick, StepWait))
	}
	s.Done()
	return out
}
