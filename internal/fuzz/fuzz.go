// Package fuzz derives a hostile corpus from a project's own declarations.
//
// DERIVED AND NOT HAND-WRITTEN, which is D1 (rook, coop r8/2). A hand-written
// corpus that silently omits an action tests nothing about it while the case
// count looks healthy, so the corpus is a function of the bindings and the
// only way to add a case is to declare an action.
//
// WHAT COUNTS AS A WRITE: an action that emits a claim. Measured over
// examples/toy at 24ea97e, all nine declared actions emit one (ibis, coop
// r8/0), so there is no read-only action to exclude -- but the rule is written
// as the predicate rather than as the number, because a later project will
// have both.
//
// THE CLASSES ARE BOUNDED BY ROOK's D1 PARTNER, and that is the whole reason
// this file does not generate unknown fields or oversize bodies. cmd/toy's
// decodeBody sets DisallowUnknownFields and an io.LimitReader at 1MB, so those
// two classes are 400 BY CONSTRUCTION -- the refusal comes from encoding/json
// before a handler sees the request, and a 400 proves nothing about the system
// under test (ibis r8/1, rook r8/3: "Each corpus class must produce a refusal
// naming a FIELD OR RULE OF THE TOY"). A corpus class that cannot be refused
// BY NAME is not evidence, so it is not generated.
//
// WHAT A DECLARATION KNOWS ABOUT AN ARGUMENT'S TYPE, and where it stops. A
// `literal` carries its value, so its type is known. A `handle` names a handle
// KIND and crosses the wire as an id, so a string. An `actor` is a name, so a
// string. A `draw` is whatever the pool holds and the declaration says
// NOTHING about it -- so a draw argument gets the type-agnostic classes only,
// and that limit is recorded here rather than papered over with a guess.
package fuzz

import (
	"fmt"
	"sort"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
)

// Class is one hostile shape. The name is part of the report, so it is a
// stable string rather than an integer nobody can read in a finding.
type Class string

const (
	// ClassMissing omits a declared argument. The toy answers "<field> is
	// required" and names the field, which is what makes this class evidence.
	ClassMissing Class = "missing"
	// ClassEmpty sends the empty string for an argument the declaration says
	// is a string. Distinct from missing: a handler that checks presence but
	// not emptiness passes the first and fails the second.
	ClassEmpty Class = "empty"
	// ClassWrongType sends a number where the declaration implies a string,
	// and a string where it carries a number.
	ClassWrongType Class = "wrong-type"
	// ClassUnknownID sends a well-formed id that names nothing, which only a
	// `handle` argument can have. The toy answers 404 "no such <kind>".
	ClassUnknownID Class = "unknown-id"
	// ClassNonPositive sends 0 and -1 where the declaration gives a numeric
	// domain, because "at least 1" is a RULE the toy states and a rule is the
	// other thing a refusal may name.
	ClassNonPositive Class = "non-positive"
	// ClassNotARole sends a string that is not a declared role, where the
	// declaration's own domain is made of role names. DERIVED FROM TWO
	// DECLARATIONS AND NOT FROM KNOWLEDGE OF THE TOY: the draw's `from` holds
	// the roles, `[[role]]` names them, and a string in neither is a value the
	// system has a rule about. Without that pairing this class would be a
	// guess about what the system validates.
	ClassNotARole Class = "not-a-role"
)

// Case is one request the stage will send: an action, the class being
// exercised, the field it perturbs, and the arguments to send.
//
// FIELD IS PART OF THE CASE because D1's partner is about the refusal naming
// a field, and a case that perturbs an argument without recording WHICH one
// cannot check that claim.
type Case struct {
	Action string
	Class  Class
	Field  string
	Args   map[string]any
}

// String renders a case for a report line.
func (c Case) String() string {
	if c.Field == "" {
		return fmt.Sprintf("%s/%s", c.Action, c.Class)
	}
	return fmt.Sprintf("%s/%s/%s", c.Action, c.Class, c.Field)
}

// Writes are the declared actions that emit a claim, in declaration order.
//
// ORDER IS THE DECLARATION'S, not sorted: the corpus is reported beside the
// bindings and a reader comparing the two should not have to re-sort one.
func Writes(b bindings.Bindings) []bindings.Action {
	var out []bindings.Action
	for _, a := range b.Actions {
		if len(a.Emits) > 0 {
			out = append(out, a)
		}
	}
	return out
}

// Corpus derives the hostile corpus. It refuses rather than returning an
// empty slice when a project declares no write action at all, because an empty
// corpus and a corpus nobody generated read identically in a report -- the
// shape D1's partner exists to catch.
func Corpus(b bindings.Bindings) ([]Case, error) {
	writes := Writes(b)
	roles := map[string]bool{}
	for _, r := range b.Roles {
		roles[r.Name] = true
	}
	if len(writes) == 0 {
		return nil, fmt.Errorf("no declared action emits a claim, so there is no write to fuzz: " +
			"an empty corpus would report as a clean run")
	}
	var out []Case
	for _, a := range writes {
		for _, arg := range a.Args {
			out = append(out, casesFor(a, arg, roles)...)
		}
	}
	if len(out) == 0 {
		names := make([]string, 0, len(writes))
		for _, a := range writes {
			names = append(names, a.Name)
		}
		sort.Strings(names)
		return nil, fmt.Errorf("%d write action(s) declare no arguments between them (%v), "+
			"so no argument can be perturbed and every case would be the honest request",
			len(writes), names)
	}
	return out, nil
}

// casesFor is the classes one argument earns, decided by what its declaration
// knows about its type and nothing more.
func casesFor(a bindings.Action, arg bindings.Arg, roles map[string]bool) []Case {
	base := func(cl Class) Case {
		return Case{Action: a.Name, Class: cl, Field: arg.Name, Args: honest(a, arg.Name)}
	}
	// MISSING IS EARNED BY EVERY ARGUMENT, whatever its source: a declared
	// argument absent from the request is a shape the system must refuse by
	// naming it, and no type knowledge is needed to omit something.
	out := []Case{base(ClassMissing)}

	switch arg.Source {
	case "handle":
		// A handle crosses the wire as an id, so a string: empty, the wrong
		// type, and an id that names nothing are all reachable.
		out = append(out, with(base(ClassEmpty), arg.Name, ""))
		out = append(out, with(base(ClassWrongType), arg.Name, 0))
		out = append(out, with(base(ClassUnknownID), arg.Name, unknownID(arg.Of)))
	case "actor":
		out = append(out, with(base(ClassEmpty), arg.Name, ""))
		out = append(out, with(base(ClassWrongType), arg.Name, 0))
	case "literal":
		switch arg.Value.(type) {
		case string:
			out = append(out, with(base(ClassEmpty), arg.Name, ""))
			out = append(out, with(base(ClassWrongType), arg.Name, 0))
		case int64, int, float64:
			out = append(out, with(base(ClassWrongType), arg.Name, "not-a-number"))
			out = append(out, with(base(ClassNonPositive), arg.Name, 0))
			out = append(out, with(base(ClassNonPositive), arg.Name, -1))
		}
	case "draw":
		// A DRAW's TYPE IS IN ITS DOMAIN, and reading it is the difference
		// between a typed hostile value and a guess. `from` is carried as
		// decoded, so its first element's type is the argument's type; an
		// empty domain is refused by the bindings schema and cannot reach
		// here, but it is handled rather than indexed blindly.
		if len(arg.From) == 0 {
			out = append(out, with(base(ClassWrongType), arg.Name, 0))
			break
		}
		switch arg.From[0].(type) {
		case int64, int, float64:
			out = append(out, with(base(ClassWrongType), arg.Name, "not-a-number"))
			out = append(out, with(base(ClassNonPositive), arg.Name, 0))
			out = append(out, with(base(ClassNonPositive), arg.Name, -1))
		case string:
			out = append(out, with(base(ClassEmpty), arg.Name, ""))
			out = append(out, with(base(ClassWrongType), arg.Name, 0))
		case []any:
			out = append(out, with(base(ClassWrongType), arg.Name, "not-a-list"))
			if r, ok := notARole(roles, arg.From); ok {
				out = append(out, with(base(ClassNotARole), arg.Name, []any{r}))
			}
		}
	}
	return out
}

// notARole answers whether this draw's domain is made of ROLE NAMES, and if so
// gives a string that is not one. It returns false rather than inventing a
// value when the domain holds anything the role list does not, because then
// the system's rule about it is unknown from the declarations and a refusal
// could not be predicted.
func notARole(roles map[string]bool, from []any) (string, bool) {
	if len(roles) == 0 {
		return "", false
	}
	saw := false
	for _, elem := range from {
		list, ok := elem.([]any)
		if !ok {
			return "", false
		}
		for _, v := range list {
			s, ok := v.(string)
			if !ok || !roles[s] {
				return "", false
			}
			saw = true
		}
	}
	if !saw {
		return "", false
	}
	// DERIVED SO IT CANNOT COLLIDE with a role a later project declares: the
	// name says what it is for, and the arm asserts it is absent from the set.
	return "not-a-declared-role", true
}

// honest is the argument map with every declared argument present and
// plausible, which is what a perturbation is a perturbation OF. The perturbed
// field is left absent; a class that sends a value puts it back.
func honest(a bindings.Action, omit string) map[string]any {
	m := map[string]any{}
	for _, arg := range a.Args {
		if arg.Name == omit {
			continue
		}
		switch arg.Source {
		case "literal":
			m[arg.Name] = arg.Value
		case "handle":
			m[arg.Name] = "{" + arg.Of + "}"
		case "actor":
			m[arg.Name] = "{actor}"
		case "draw":
			m[arg.Name] = "{draw}"
		}
	}
	return m
}

// with sets one field on a case's arguments and returns it.
func with(c Case, field string, v any) Case {
	m := make(map[string]any, len(c.Args)+1)
	for k, val := range c.Args {
		m[k] = val
	}
	m[field] = v
	c.Args = m
	return c
}

// unknownID is a well-formed id of the right KIND that names nothing. It is
// derived from the kind so a finding says which handle kind went unresolved,
// and it is deliberately not a random string: a corpus that changes between
// runs cannot be replayed from a seed.
func unknownID(kind string) string {
	if kind == "" {
		return "nonesuch"
	}
	return kind + "-nonesuch"
}
