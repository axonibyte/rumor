package validate

import (
	"sort"
	"strings"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/platform"
)

// arm6: A STATEFUL COMPONENT NO ROLLBACK PROVIDER COVERS.
//
// STATIC OVER TWO DECLARATIONS, which is fable's correction to me at coop
// r4/74: I had this arm in the group needing a live `describe`, on the strength
// of docs/contract.md:86 pairing components "live against static". It does not
// need one. A rollback provider and the components it covers are BOTH in
// platform.toml, and the bindings name components in two places -- an action's
// `touches` and a `direct` observation's `component` -- so both sides of the
// pairing are declared and the arm is static.
//
// THE TWO PLACES A BINDING NAMES A COMPONENT ARE BOTH READ, and that matters:
// `touches` exists precisely because a component was otherwise nameable only on
// a direct observation, so an action dirtying an uncovered component that
// nothing reads directly fell outside an arm whose wording is about bindings
// that touch one (ibis, coop r4/23, and bindings.go says so at the field).
func arm6(rep *Report, b bindings.Bindings, p platform.Platform) {
	rep.ran("6")
	covered := map[string]bool{}
	for _, rb := range p.Rollback {
		for _, c := range rb.Covers {
			covered[c] = true
		}
	}
	named := map[string][]string{}
	for _, a := range b.Actions {
		for _, c := range a.Touches {
			named[c] = append(named[c], "action "+a.Name+" touches it")
		}
	}
	for _, o := range b.Observations {
		if o.Component != "" {
			named[o.Component] = append(named[o.Component], "observation "+o.Name+" reads it directly")
		}
	}
	var bad []string
	for c, why := range named {
		if !covered[c] {
			sort.Strings(why)
			bad = append(bad, c+" ("+strings.Join(why, ", ")+")")
		}
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		rep.refuse("6", "stateful component(s) no [[rollback]] provider covers: %s -- a run would "+
			"dirty them with nothing declared to put them back", strings.Join(bad, "; "))
	}
}
