// The setup sequence a hostile case needs is DERIVED from `creates`, the same
// way the corpus is derived from the arguments.
//
// WHY IT HAS TO BE DERIVED. Every class but `missing` sends a value, and a
// `handle` argument's honest value is an id the system minted -- so the stage
// cannot send one case until something has created an event, a user, an
// invitation. Writing that sequence by hand would put the toy's own shape in
// the stage, and the next project would need a second hand-written sequence;
// worse, it would be a sequence nobody checks against the declarations, so an
// action that stopped creating what it says it creates would leave the stage
// sending ids it believed in.
//
// `creates` IS ALREADY THE DECLARATION FOR IT. An action names the handle
// KINDS it mints, and an argument names the kind it needs, so "what must run
// before this case" is a reachability question over two fields the project
// already writes for other reasons -- the rollback-coverage arm reads
// `touches`, `rumor validate` reads `creates` against the handle table, and
// the pairing here adds no schema.
package fuzz

import (
	"fmt"
	"sort"
	"strings"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
)

// Providers maps a handle kind to the action that mints it.
//
// A KIND WITH TWO PROVIDERS IS NOT AN ERROR HERE: the walk may have several
// ways to make an event, and the setup only needs one. The choice is the
// FIRST in declaration order, so the sequence a reader derives by hand from
// the file is the one the stage runs -- a map's iteration order would make
// the setup differ between runs of the same bindings, which is the
// determinism a replay rests on.
func Providers(b bindings.Bindings) map[string]bindings.Action {
	out := map[string]bindings.Action{}
	for _, a := range b.Actions {
		for _, kind := range a.Creates {
			if _, taken := out[kind]; !taken {
				out[kind] = a
			}
		}
	}
	return out
}

// Needs are the handle kinds a corpus needs honest ids for, sorted.
//
// IT READS THE CORPUS AND NOT THE BINDINGS, because the corpus is what will
// be sent: an action whose every case is `missing` on its only handle would
// need no id at all, and a setup that ran anyway would be doing work no case
// asked for.
func Needs(b bindings.Bindings, cases []Case) []string {
	byAction := map[string]bindings.Action{}
	for _, a := range b.Actions {
		byAction[a.Name] = a
	}
	set := map[string]bool{}
	for _, c := range cases {
		a := byAction[c.Action]
		for _, arg := range a.Args {
			if arg.Source != "handle" || arg.Of == "" {
				continue
			}
			// THE PERTURBED FIELD STILL NEEDS ITS KIND unless the case omits
			// it: `unknown-id` sends a derived id and needs none, but
			// `wrong-type` on one handle leaves the OTHER handle honest.
			if arg.Name == c.Field && (c.Class == ClassMissing || c.Class == ClassUnknownID) {
				continue
			}
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

// Order is the actions to run, in order, so that every kind in need has an
// honest id. It refuses rather than returning a partial sequence, because a
// stage that ran a short setup would send ids it never obtained and report
// the resulting 404s as findings about the system.
func Order(b bindings.Bindings, need []string) ([]bindings.Action, error) {
	prov := Providers(b)
	var out []bindings.Action
	done := map[string]bool{}
	var resolve func(kind string, path []string) error
	resolve = func(kind string, path []string) error {
		if done[kind] {
			return nil
		}
		for _, seen := range path {
			if seen == kind {
				// A CYCLE IS REFUSED BY NAME AND WITH ITS PATH. Two actions
				// each needing the other's handle cannot both go first, and a
				// stage that recursed would hang rather than say so.
				return fmt.Errorf("the handle kinds %s create each other, so no setup order exists",
					strings.Join(append(path, kind), " -> "))
			}
		}
		a, ok := prov[kind]
		if !ok {
			kinds := make([]string, 0, len(prov))
			for k := range prov {
				kinds = append(kinds, k)
			}
			sort.Strings(kinds)
			return fmt.Errorf("a case needs a handle of kind %q and no declared action creates one "+
				"(declared kinds: %v): the stage would send an id it never obtained", kind, kinds)
		}
		for _, arg := range a.Args {
			if arg.Source == "handle" && arg.Of != "" {
				if err := resolve(arg.Of, append(path, kind)); err != nil {
					return err
				}
			}
		}
		done[kind] = true
		out = append(out, a)
		return nil
	}
	for _, kind := range need {
		if err := resolve(kind, nil); err != nil {
			return nil, err
		}
	}
	return bootstrapFirst(out), nil
}

// bootstrapFirst hoists the actions that need NO SESSION and NO HANDLE ahead
// of the rest, keeping the topological order within each group.
//
// THE HANDLE GRAPH IS NOT THE WHOLE DEPENDENCY GRAPH, and running the setup is
// what showed it. `creates` and `of` express what an action MAKES and what one
// NEEDS, so the order they give satisfies every handle -- and the toy's
// create_event still failed, because a session for the actor is refused until
// that actor exists as a user ("no user named %q: a session belongs to a user
// that exists"). That dependency is declared, but in `requires`:
// create_event requires `has_user`, which is a STATUTE RULE and not a handle,
// so no amount of reading `creates` could see it.
//
// RESOLVING A RULE TO THE ACTION THAT SATISFIES IT WOULD MEAN EVALUATING THE
// STATUTE, which is the walk's work and not a setup's. What is derivable from
// the declaration alone is narrower and enough: an action whose context is
// "none" needs no session, and one with no handle arguments needs nothing the
// setup has not got yet, so it can always go first. BOTH CONDITIONS ARE
// REQUIRED -- hoisting a no-session action that took a handle would move it
// ahead of whatever makes that handle, which is the ordering this function
// exists to keep.
func bootstrapFirst(in []bindings.Action) []bindings.Action {
	var first, rest []bindings.Action
	for _, a := range in {
		handles := false
		for _, arg := range a.Args {
			if arg.Source == "handle" {
				handles = true
			}
		}
		if a.Context == "none" && !handles {
			first = append(first, a)
			continue
		}
		rest = append(rest, a)
	}
	return append(first, rest...)
}
