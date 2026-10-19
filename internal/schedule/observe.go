package schedule

import (
	"fmt"
	"sort"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/handle"
)

// ObserveArgs composes an observe request's `args` from the observation's
// DECLARATION and the handle table, and from nothing else -- which is contract
// 2's own sentence (docs/contract.md:134, the v2 row, fable's ruling at coop
// r5.toy/8). `chosen` names, per declared argument, the handle the walk means:
// the engine knows which entity the step is about, the adapter never does.
//
// THE VALUES ARE IDS AND THE INPUTS ARE HANDLES, and that asymmetry is the
// whole point. A handle is the stable name across attempts and an id is the
// disposable one (docs/vocabulary.md, Handles), so the table is consulted at
// the moment the message is written. An unresolvable handle REFUSES rather
// than falling back to sending the handle itself, because a handle on the wire
// is exactly the breach contract 2 was raised to prevent -- a second way of
// carrying an id, in a shape the decoder cannot check.
//
// ABSENT, NEVER EMPTY. contract.md: `args` is "present exactly when the
// observation declares any". An observation that declares none composes to nil
// so the field is omitted, not to an empty object: `rumor validate` holds both
// halves and pairs the declaration against the request, and an empty object
// would read as "declared none and sent none" and as "declared some and sent
// nothing" at once.
func ObserveArgs(decl bindings.Observation, chosen map[string]string, t *handle.Table) (map[string]any, error) {
	if len(decl.Args) == 0 {
		if len(chosen) > 0 {
			names := make([]string, 0, len(chosen))
			for name := range chosen {
				names = append(names, name)
			}
			sort.Strings(names)
			return nil, fmt.Errorf("schedule: observation %q declares no arguments and the walk "+
				"chose %v: an argument the declaration does not name is a field the decoder "+
				"accepts and nothing checks", decl.Name, names)
		}
		return nil, nil
	}
	if t == nil {
		return nil, fmt.Errorf("schedule: observation %q declares %d argument(s) and there is no "+
			"handle table to resolve them against", decl.Name, len(decl.Args))
	}

	args := make(map[string]any, len(decl.Args))
	declared := make(map[string]bool, len(decl.Args))
	for _, name := range decl.Args {
		declared[name] = true
		h, ok := chosen[name]
		if !ok {
			return nil, fmt.Errorf("schedule: observation %q declares argument %q and the walk "+
				"chose no handle for it: a partial `args` would be refused as a could-not-run "+
				"the project did not cause", decl.Name, name)
		}
		id, err := t.Require(h)
		if err != nil {
			return nil, fmt.Errorf("schedule: observation %q's argument %q names handle %q, which "+
				"this attempt's table cannot resolve, and a handle is never written to the wire: %w",
				decl.Name, name, h, err)
		}
		args[name] = id
	}
	// EXTRA CHOICES ARE REFUSED, not dropped silently: a walk that chose an
	// argument the observation does not declare has a defect the engine can
	// see, and dropping it would send a request that looks well-formed.
	extra := make([]string, 0)
	for name := range chosen {
		if !declared[name] {
			extra = append(extra, name)
		}
	}
	if len(extra) > 0 {
		sort.Strings(extra)
		return nil, fmt.Errorf("schedule: observation %q does not declare %v, which the walk chose: "+
			"the request would carry a field the declaration cannot be paired against", decl.Name, extra)
	}
	return args, nil
}
