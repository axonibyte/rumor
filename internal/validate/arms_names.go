package validate

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/platform"
	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

// arm19: EVERY NAME A PROJECT USES IS A NAME THE PROJECT DECLARES.
//
// ONE ARM WITH SEVERAL CELLS, ruled by fable at coop r4/113. Each cell pairs one
// reference against the set it is looked up in, and the refusal names the file,
// the reference and that set -- so a cell cannot hide behind another's name,
// which is the objection to widening arm 3 (arm 3 is "an action with no
// precondition"; a wrong NAME is a different defect).
//
// WHY IT EXISTS, and the finding is rook's at coop r5/22: `bindings.toml` could
// name a precondition rule no statute declared and every one of the eighteen
// arms passed -- 18 of 18 evaluated, 0 refusals, exit 0, with toy-adapter on
// PATH so nothing was skipped. The action's precondition had vanished from
// every walk silently. quill's /107 is the deciding half: a CONSUMER already
// assumed this pairing existed (internal/schedule.Applicable's absent-verdict
// branch), so the hole was load-bearing before anyone noticed it was missing.
//
// WHAT IS NOT HERE, each for a measured reason:
//
//   - An observation whose `of` names an unemitted claim type. ARM 1 holds it
//     where a case carries the observation, and tests/contract's agreement arm
//     holds it where none does -- driven both ways (ibis, coop r4/119).
//   - An action whose `context` no `[[role]]` holds. The referent set is
//     unsound rather than unmet: examples/twin declares ZERO roles and
//     examples/toy's three all hold `["api"]` while its actions use `api`,
//     `browser` and `none`, so the cell would refuse both of our own examples
//     (ibis r4/115, rook r4/117). "Could reach" is arm 5's question.
//   - `touches` naming nothing. docs/contract.md says the engine cannot check
//     what an action declares it touches, and that stays true: it is a claim
//     about the world, not a reference to a declaration.
//   - CELL 2'S MIRROR: a declared action with NO weight. quill measured it on
//     examples/toy while building R5.1's pool (coop r4/121) -- six action
//     weights, none for the four nemesis kinds, and no refusal from here. They
//     did not ask for a cell and I did not add one: the mirror's referent set
//     is "every declared action plus the nemesis kinds", which is a vocabulary
//     question waiting on R5's weight policy, and their Pool refuses it at the
//     walk. Written down rather than left to be rediscovered.
//
// A DANGLING REFERENCE IS A REFUSAL AND NOT COULD-NOT-RUN, by fable's r4/79
// line: both documents were READ, and what they say disagrees. The one cell
// that needs the adapter is the observations cell, and when no adapter answers
// it reports could-not-run naming itself rather than passing.
func arm19(rep *Report, b bindings.Bindings, p platform.Platform, rules []string) {
	rep.ran("19")

	declared := func(names []string) map[string]bool {
		set := make(map[string]bool, len(names))
		for _, n := range names {
			set[n] = true
		}
		return set
	}

	rule := declared(rules)
	var action []string
	emitted := map[string]bool{}
	created := map[string]bool{}
	for _, a := range b.Actions {
		action = append(action, a.Name)
		for _, t := range a.Emits {
			emitted[t] = true
		}
		for _, h := range a.Creates {
			created[h] = true
		}
	}
	act := declared(action)
	nemesis := declared(NemesisKinds)
	role := map[string]bool{}
	for _, r := range b.Roles {
		role[r.Name] = true
	}

	var bad []string
	note := func(f string, a ...any) { bad = append(bad, fmt.Sprintf(f, a...)) }

	// CELL 1: a precondition naming a rule rules.json does not declare.
	for _, a := range b.Actions {
		for _, r := range a.Requires {
			if !rule[r] {
				note("bindings.toml: requires %s on action %s: no rule of that name in rules.json", r, a.Name)
			}
		}
	}

	// CELL 2: a weight for an action bindings.toml does not declare. The
	// PARSER already refuses `weights.stage.<stage>` for an undeclared stage
	// and cannot refuse this one, because it is handed platform.toml alone --
	// one file, two dangling references of the same kind, opposite answers
	// (ibis r4/105, rook r4/109). This arm reads both documents, which is why
	// the pairing lives here and the parser's own refusal stays where it is.
	//
	// THE REFERENT SET IS ACTIONS PLUS THE NEMESIS KINDS, and the first cut of
	// this cell got it wrong: it refused a weight for `suborn`, which
	// docs/contract.md's own `[weights.default]` sample carries beside a
	// comment saying "nemesis kinds are action kinds in the same pool". Driven
	// against the twin with `suborn = 1` added, it refused the documented shape
	// -- so the cell would have shipped refusing a project this project's own
	// documentation tells people to write.
	//
	// THE DEFAULT TABLE ONLY, because the per-stage and per-profile tables
	// cannot carry a name this cell would catch: internal/platform refuses an
	// override naming a kind `[weights.default]` does not, so every override
	// key is a default key before this arm sees it. Driven, on the twin:
	// `[weights.stage.api] publish_nothing = 2` is could-not-run from the
	// PARSER, not a refusal from here. Looping over them anyway would be two
	// checks that cannot fire, which read exactly like checks that can. The
	// coupling is pinned where it lives -- internal/platform's own test case
	// "an override naming a kind the default does not" -- so the hole cannot
	// open quietly if that refusal ever moves.
	for kind := range p.Weights.Default {
		if !act[kind] && !nemesis[kind] {
			note("platform.toml: [weights.default] names %s: no action of that name in bindings.toml, "+
				"and it is not a nemesis kind", kind)
		}
	}

	// CELL 2's REVERSE, ruled in at coop r4/125 after quill measured the gap
	// from the other side (r4/121, r5/47): every action the bindings declare
	// and every nemesis kind in the pool must HAVE a weight. ABSENT IS NOT
	// ZERO -- zero means never drawn and the project decided it, while absent
	// means the walk reaches a kind the project never weighted, which is the
	// silent-drop class under another name. quill's pool refuses it at the
	// walk; a project should hear it from here first.
	//
	// THE DEFAULT TABLE IS THE COMPLETE ONE, so that is where this looks: the
	// parser already requires every override key to be a default key, which is
	// what makes "resolved from platform.toml (profile, stage, default)" the
	// same question as "is it in the default table".
	for _, name := range action {
		if _, ok := p.Weights.Default[name]; !ok {
			note("bindings.toml: action %s: no weight for it in [weights.default], and absent is not zero", name)
		}
	}
	for _, kind := range NemesisKinds {
		if _, ok := p.Weights.Default[kind]; !ok {
			note("platform.toml: the nemesis kind %s has no weight in [weights.default], and absent is not zero", kind)
		}
	}

	// CELL 3: a venire drawing a role bindings.toml does not declare, AND ONLY
	// WHERE ROLES ARE DECLARED AT ALL. internal/bindings makes `[[role]]`
	// optional, so an absent role table is a project that does not model roles
	// -- examples/twin is one, and refusing it would refuse an example fable's
	// D9 requires to pass. Once a project declares one role, a venire drawing
	// another is a typo, and examples/toy declares exactly its three.
	if len(role) > 0 {
		for _, m := range p.Venire {
			if !role[m.Role] {
				note("platform.toml: [[venire]] draws role %s: no [[role]] of that name in bindings.toml", m.Role)
			}
		}
	}

	// CELL 4: an observation argument naming a handle no action creates. The
	// engine resolves these against the run's handles before a message is
	// written (docs/contract.md's `observe.args`), so a name nothing creates
	// resolves to nothing at run time.
	for _, o := range b.Observations {
		for _, arg := range o.Args {
			if !created[arg] {
				note("bindings.toml: observation %s takes arg %s: no action creates a handle of that name",
					o.Name, arg)
			}
		}
	}

	// CELL 5: a UI flow placeholder naming a handle no action creates. EVERY
	// STRING FIELD OF A STEP IS SCANNED, which is a superset of what a driver
	// would substitute and is stated rather than narrowed: `{...}` is not legal
	// in any of these fields for any other purpose, so a brace-wrapped name is
	// a reference wherever it appears.
	for _, a := range b.Actions {
		for i, s := range a.Flow {
			for _, field := range []struct{ what, text string }{
				{"to", s.To}, {"field", s.Field}, {"value", s.Value},
				{"target", s.Target}, {"until", s.Until},
			} {
				for _, h := range placeholders(field.text) {
					if !created[h] {
						note("bindings.toml: action %s flow step %d %s names {%s}: no action creates a handle of that name",
							a.Name, i+1, field.what, h)
					}
				}
			}
		}
	}

	// CELL 6: a [claim.<type>] table describing a type no action emits. Arm 2
	// is this pairing's other direction -- an emitted type with no table -- and
	// fable moved this one here rather than widening arm 2, so each arm's name
	// keeps describing what it refuses.
	for t := range b.Claims {
		if !emitted[t] {
			note("bindings.toml: [claim.%q] describes a type no action emits", t)
		}
	}

	if len(bad) > 0 {
		sort.Strings(bad)
		rep.refuse("19", "a name the project uses that the project does not declare: %s",
			strings.Join(bad, "; "))
	}
}

// arm19Observations is the one cell that needs the adapter: a bindings
// observation the adapter's `describe` does not declare.
//
// IT IS THE SAME ARM, not a second one -- so it adds to arm 19's refusals and
// does not call ran() again, which would make one arm account for itself twice.
// With no adapter the cell reports COULD-NOT-RUN naming itself, exactly as
// fable's r4/113 reading requires: the question is unanswerable, the run is
// already exit 2 for the describe that failed, and nothing reads the arm's
// silence on this cell as a pass.
//
// docs/contract.md's `observations` row calls this the engine's pre-run check.
// Nothing implemented it -- every use of bindings' Observations in the tree is
// arm 6, a count, wire's own validation or fake's echo (ibis r4/105) -- so the
// check arrives here, before any actor moves, which is what that row promised.
func arm19Observations(rep *Report, b bindings.Bindings, d wire.DescribeResponse) {
	offered := map[string]bool{}
	for _, o := range d.Observations {
		offered[o.Name] = true
	}
	var bad []string
	for _, o := range b.Observations {
		if !offered[o.Name] {
			bad = append(bad, fmt.Sprintf("bindings.toml: observation %s: the adapter's describe declares %d and not that one",
				o.Name, len(d.Observations)))
		}
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		rep.refuse("19", "a name the project uses that the adapter does not declare: %s", strings.Join(bad, "; "))
	}
}

// NemesisKinds are the four the walk draws from the same pool as a project's own
// actions: `suborn` feeds an actor a false claim, `canard` is a false tell,
// `palter` a half-true claim, `essoin` an abandoned flow (docs/design.md).
//
// THIS IS A COPY AND IT SAYS SO. The list belongs to the scheduler that draws
// them -- R5.1's pool, which is not on main yet -- and it is declared here only
// because arm 19's weight cell cannot otherwise tell a nemesis kind from a
// typo. tests/validate pins it against docs/design.md's own sentence, so the
// copy cannot drift from the document while both exist; when the scheduler
// lands, this reads its list and the oracle moves with it.
var NemesisKinds = []string{"canard", "essoin", "palter", "suborn"}

// placeholder is `{name}`: the brace form a flow step uses for a handle.
var placeholder = regexp.MustCompile(`\{([^{}]+)\}`)

func placeholders(s string) []string {
	var out []string
	for _, m := range placeholder.FindAllStringSubmatch(s, -1) {
		out = append(out, m[1])
	}
	return out
}
