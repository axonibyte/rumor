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
	nemesis := declared(PoolKinds)
	role := map[string]bool{}
	roleContexts := map[string][]string{}
	for _, r := range b.Roles {
		role[r.Name] = true
		roleContexts[r.Name] = r.Contexts
	}
	observedNames := make([]string, 0, len(b.Observations))
	obsSource := map[string]wire.Source{}
	for _, o := range b.Observations {
		observedNames = append(observedNames, o.Name)
		obsSource[o.Name] = o.Source
	}
	observed := declared(observedNames)

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
				"and it is not a kind the engine draws", kind)
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
	for _, kind := range PoolKinds {
		if _, ok := p.Weights.Default[kind]; !ok {
			note("platform.toml: the engine kind %s has no weight in [weights.default], and absent is not zero", kind)
		}
	}

	// CELL 3: a venire drawing a role bindings.toml does not declare, AND ONLY
	// WHERE ROLES ARE DECLARED AT ALL. internal/bindings makes `[[role]]`
	// optional, so an absent role table is a project that does not model roles
	// -- examples/twin is one, and refusing it would refuse an example fable's
	// D9 requires to pass. Once a project declares one role, a venire drawing
	// another is a typo, and examples/toy declares exactly its three.
	//
	// A PROFILE'S APPENDED ROWS ARE VENIRE ROWS, which is fable's ruling at
	// coop rumor/1662 -- and this cell had the hole for exactly one commit. The
	// level landed at fc85f87 reading `p.Venire` alone, so a row under
	// `[profiles.<name>].venire` naming a role nothing declares resolved to
	// nothing at the draw and was refused by nobody. The rows are walked with
	// the profile NAMED, because "draws role wizard" with no table sends a
	// reader to the platform's own venire, which is not where it is written.
	if len(role) > 0 {
		for _, m := range p.Venire {
			if !role[m.Role] {
				note("platform.toml: [[venire]] draws role %s: no [[role]] of that name in bindings.toml", m.Role)
			}
		}
		for _, name := range profileNames(p) {
			for _, m := range p.Profiles[name].Venire {
				if !role[m.Role] {
					note("platform.toml: [profiles.%s].venire draws role %s: no [[role]] of "+
						"that name in bindings.toml", name, m.Role)
				}
			}
		}
	}

	// CELL 8: A VENIRE ROW A PROFILE'S OWN STAGES CANNOT DRAW. fable ruled this
	// into arm 19 rather than giving it an id of its own (coop rumor/1662, on
	// the ASK at r8/148): a profile's appended row is a venire row, so the
	// pairing is this arm's and the count does not move.
	//
	// WHY IT IS AN ARM AND NOT A PARSE REFUSAL, which is the asymmetry with the
	// period's level and was asked before it was built. The period is refused
	// inside internal/platform because a profile's own stage list says whether
	// that profile walks -- one file, one table. A venire row names a ROLE, and
	// a role's contexts live in bindings.toml, which internal/platform does not
	// read and cannot import: internal/bindings reads platform, so the
	// dependency runs the other way and the import would be a cycle. A pairing
	// needing two files is what an arm is for.
	//
	// THE BROWSER CONTEXT AND NO OTHER, and the reason covers exactly that
	// narrowing: a context is an ADAPTER-DECLARED SESSION KIND, which is what
	// arm 21 pairs against `describe`'s contexts, and not a stage name. A
	// project may declare a `kiosk` context whose adapter serves it, and
	// refusing that under every profile would refuse a correct project.
	// `browser` is the one context with a STAGE that drives it
	// (`stages.browser.enabled`, docs/contract.md:419, and isBrowser's one
	// name), so it is the one a profile's stage list can be said to be unable
	// to draw. `none` is not a context and is skipped here as it is in arm 21.
	//
	// THE PROFILE'S OWN ROWS AND NOT THE COMMITTED ONES, which is fable's "a
	// venire row ON a profile". A committed `[[venire]]` row is drawn by every
	// profile and is the platform's claim rather than any profile's, so
	// refusing it here would refuse a project for a pairing it never declared
	// per profile. WHAT THAT LEAVES: a COMMITTED row whose role holds `browser`
	// under a profile selecting no browser stage is still an actor that cannot
	// act, refused by nothing -- older than this level, not ruled, and reported
	// rather than decided here.
	for _, name := range profileNames(p) {
		prof := p.Profiles[name]
		if selectsBrowserStage(prof.Stages) {
			continue
		}
		for _, m := range prof.Venire {
			var held []string
			for _, c := range roleContexts[m.Role] {
				if isBrowser(c) {
					held = append(held, c)
				}
			}
			if len(held) == 0 {
				continue
			}
			note("platform.toml: [profiles.%s].venire draws role %s, which holds %s, and this "+
				"profile's stages are %s: an actor holding a context no selected stage drives "+
				"is one the profile cannot move", name, m.Role,
				strings.Join(held, ", "), stageList(prof.Stages))
		}
	}

	// CELL 9: A PROFILE TAKING AN OBSERVATION bindings.toml DOES NOT DECLARE.
	// The observations level is fable's ruling at coop rumor/1678, and this is
	// the first of the two pairings it needs -- cells of this arm rather than
	// new ids for cell 8's reason: the claim is a NAME, and what it names lives
	// in the other document.
	//
	// IT IS THE DANGEROUS HALF OF THE TWO, which is why it is spelled before
	// the source cell. A profile's claim is also what takes a name OUT of the
	// project's set (platform.Observes), so a misspelled claim does not merely
	// name nothing: it leaves the real observation claimed by no profile and
	// therefore taken by every one of them -- silently restoring the reading
	// the level was ruled in to prevent. A typo here reads as a working
	// declaration from inside the profile that wanted it, because that profile
	// observes the name either way.
	for _, name := range profileNames(p) {
		for _, o := range p.Profiles[name].Observations {
			if observed[o] {
				continue
			}
			note("platform.toml: [profiles.%s].observations names %s: no observation of that "+
				"name, and a profile's claim is also what takes a name out of the project's "+
				"set -- so a name nothing declares removes nothing and every profile still "+
				"observes what this one meant to keep", name, o)
		}
	}

	// CELL 10: AN OBSERVATION A PROFILE'S OWN STAGES CANNOT DRIVE, which is
	// cell 8 one declaration over and is stated in the ruling itself: "a
	// profile observation whose source the profile's stages cannot reach is
	// refused by name under arm 19's cell, as its venire row is".
	//
	// THE BROWSER SOURCE AND NO OTHER, and the narrowing is cell 8's exactly
	// and covers the same ground. A source is adapter-served: `direct` reads a
	// declared component, `api` IS the walk, and `browser` is the one source
	// with a stage that drives it (`stages.browser.enabled`, isBrowser's one
	// name). A project declaring some further source its adapter serves is not
	// one this cell can judge, and refusing it under every profile would refuse
	// a correct project.
	//
	// THE PROFILE'S OWN CLAIMS AND NOT THE PROJECT'S ROWS, which is cell 8's
	// third clause and leaves the same remainder: a project-level
	// `[[observation]]` with a browser source is taken by every profile,
	// including those selecting no browser stage, and is refused by nothing
	// here. That is older than this level and is reported rather than decided
	// -- except that the toy no longer has one, because this ruling moved its
	// only browser observation onto the profile that drives it.
	for _, name := range profileNames(p) {
		prof := p.Profiles[name]
		if selectsBrowserStage(prof.Stages) {
			continue
		}
		for _, o := range prof.Observations {
			if obsSource[o] != wire.SourceBrowser {
				continue
			}
			note("platform.toml: [profiles.%s].observations takes %s, whose source is %s, and "+
				"this profile's stages are %s: an observation no selected stage drives is one "+
				"the profile cannot make", name, o, wire.SourceBrowser, stageList(prof.Stages))
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

	// CELL 4's SIBLING, for an ACTION's declared arguments (R5.4, issue 78):
	// a `handle` argument whose `of` names a kind no action creates. The engine
	// resolves `of` against the run's handle table before the request is
	// written, exactly as it does an observation's args, so a kind nothing
	// binds resolves to nothing.
	//
	// THE PARSER CANNOT HOLD THIS ONE, which is why it is here and not a
	// refusal in internal/bindings: `arg()` reads ONE [[action.args]] table and
	// the created set spans every action in the document, so the pairing needs
	// both halves at once -- the same reason cell 2 lives here rather than in
	// internal/platform. Measured before writing it: nothing else in the tree
	// pairs Arg.Of against Creates; internal/schedule/actors.go reads it only
	// to decide APPLICABILITY, which makes a dangling kind a step that never
	// runs rather than a declaration anyone refuses.
	//
	// AND INAPPLICABLE IS NOT A REFUSAL, which is the whole reason this cell
	// earns its place: docs/contract.md's own row says a kind nothing has bound
	// makes the action inapplicable. A walk would therefore skip the action in
	// silence for the life of the project, and "my action never fires" is the
	// quietest defect this validator can be asked to catch.
	for _, a := range b.Actions {
		for _, arg := range a.Args {
			if arg.Source != bindings.ArgHandle {
				continue
			}
			if !created[arg.Of] {
				note("bindings.toml: action %s takes arg %s of kind %s: no action creates a handle of that name",
					a.Name, arg.Name, arg.Of)
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

	// CELL 7: a `[deadline].instants` name that no `[[observation]]` declares.
	//
	// IT IS A CELL OF THIS ARM AND NOT A NEW NUMBER, on ratchet's measurement
	// at coop r7/60: this arm's own sentence is "a name the project uses that
	// the project does not declare", `instants` holds exactly such names as of
	// R7.4, and they were NOT seen here -- an undeclared name passed at rc 0,
	// driven. A twenty-third arm would have left that sentence claiming a
	// class it did not cover, which is the shape of defect arm 19 itself was
	// created by (rook, coop r5/22: a precondition naming no rule, eighteen
	// arms green).
	//
	// THE REFERENT SET IS THIS FILE'S OBSERVATIONS, not the adapter's. An
	// instant is read through an observation the project DECLARES; whether the
	// adapter serves that observation is cell 19-observations' pairing, which
	// runs against `describe` and reports could-not-run when nothing answers.
	// One name, two pairings, each named where its referent lives -- so a
	// project whose adapter is absent is still refused for a name its own
	// bindings cannot resolve.
	for _, name := range b.DeadlineInstants {
		if !observed[name] {
			note("bindings.toml: [deadline] instants names %s: no observation of that name, "+
				"and an instant is read through one", name)
		}
	}

	// AND `contended`'s TWO VALUES ARE REFERENCES TOO, which is rook's finding
	// at coop rumor/1528: the parser requires both keys present "and nothing
	// more ... `limit` and `held` are free-form strings ... nothing resolves
	// either". A name a project uses that the project does not declare is this
	// arm's own sentence, so these are cells of it rather than new numbers --
	// the reading `[deadline].instants` established at R7.4.
	//
	// AND THE SYMPTOM IS QUIET, which is why it is refused before the run
	// rather than left to the stage: a `limit` resolving to nothing leaves the
	// room unknown, so every fan-out is refused as VACUOUS -- and a vacuous
	// fan-out reads as "no overfill found" and not as a broken declaration.
	// The one output a reader would act on is the one it cannot produce.
	//
	// THE REFERENT OF `limit` IS AN ACTION's ARGUMENT, spelled `<action>.<arg>`
	// because the limit is set by the action that CREATED the thing and not by
	// the one that contends on it. The referent of `held` is an observation
	// this file declares, the same set `instants` resolves against -- and
	// whether the adapter serves it is cell 19-observations' pairing, exactly
	// as an instant's is.
	for _, a := range b.Actions {
		if a.Contended.Limit == "" && a.Contended.Held == "" {
			continue
		}
		if !argExists(b, a.Contended.Limit) {
			note("bindings.toml: action %s contends on limit %q: no action declares that "+
				"argument, and a limit is read from the argument that set it",
				a.Name, a.Contended.Limit)
		}
		if !observed[a.Contended.Held] {
			note("bindings.toml: action %s contends with held %q: no observation of that name, "+
				"and what the system holds is read through one", a.Name, a.Contended.Held)
		}
	}

	if len(bad) > 0 {
		sort.Strings(bad)
		rep.refuse("19", "a name the project uses that the project does not declare: %s",
			strings.Join(bad, "; "))
	}
}

// profileNames is the declared profiles in a fixed order. arm 19 sorts its
// refusals before printing, so this is not about the output: it is so that a
// project with two bad profiles reports the same pair of notes on every run,
// whatever the map hands back.
func profileNames(p platform.Platform) []string {
	out := make([]string, 0, len(p.Profiles))
	for name := range p.Profiles {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// selectsBrowserStage reports whether a profile's stage list selects the
// browser stage.
//
// THE NAME IS SPELLED HERE RATHER THAN TAKEN FROM isBrowser, which tests a
// CONTEXT. The two strings are equal today and the namespaces are not -- a
// context is an adapter-declared session kind, a stage is something the engine
// runs -- and one helper answering for both would make a rename of either
// silently answer for the other. This child has already met that shape twice in
// the toy's two role namespaces.
func selectsBrowserStage(stages []string) bool {
	for _, s := range stages {
		if s == "browser" {
			return true
		}
	}
	return false
}

// stageList renders a profile's stages for a refusal, `none` included, because
// a refusal reading "this profile's stages are " has lost the half that tells a
// reader what to change.
func stageList(stages []string) string {
	if len(stages) == 0 {
		return "none"
	}
	return strings.Join(stages, ", ")
}

// argExists reports whether `<action>.<arg>` names an argument some action in
// this file declares.
//
// THE SPELLING IS PART OF THE REFERENCE: a bare argument name would be
// ambiguous across actions -- examples/toy declares `name` on two of them --
// and the limit belongs to one action's argument and not to whichever action
// happens to share the word. So a value without exactly one dot resolves to
// nothing and is refused with the rest.
func argExists(b bindings.Bindings, ref string) bool {
	action, arg, ok := strings.Cut(ref, ".")
	if !ok || action == "" || arg == "" {
		return false
	}
	for _, a := range b.Actions {
		if a.Name != action {
			continue
		}
		for _, g := range a.Args {
			if g.Name == arg {
				return true
			}
		}
	}
	return false
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

// PoolKinds is every kind the ENGINE draws beside a project's own actions: the
// four above, the honest `tell`, and R7.3's `advance`.
//
// WHY A SECOND LIST RATHER THAN A FIFTH ENTRY ABOVE. R5.4 added the honest tell
// (fable, coop r5/114) -- it passes on a claim the run did NOT plant, so it is
// not a nemesis kind, and NemesisKinds is pinned by tests/validate to
// docs/design.md's sentence naming those four in one breath. Arm 19's two cells
// are about which names the ENGINE owns rather than about which are lies, so
// they read this list: a weight must EXIST for each, and a weight naming one is
// not a dangling reference.
//
// THE SCHEDULER OWNS THE REAL LIST, as the note above says of the other one:
// internal/schedule.PoolKinds is the set the pool actually draws, and this copy
// exists because arm 19 cannot otherwise tell an engine kind from a typo.
//
// `advance` JOINED AT R7.3 AND IS NOT A NEMESIS KIND: it moves the run's own
// clock and tells no lie, so it belongs in this list and not in the four
// above, which tests/validate pins to docs/design.md's own sentence. Its
// weight is required like every other kind's -- cell 2 below asks only that
// one EXIST, and `essoin`'s zero in the toy is the precedent for a kind the
// table declares and the walk may never draw.
var PoolKinds = []string{"canard", "essoin", "palter", "suborn", "tell", "advance"}

// placeholder is `{name}`: the brace form a flow step uses for a handle.
var placeholder = regexp.MustCompile(`\{([^{}]+)\}`)

func placeholders(s string) []string {
	var out []string
	for _, m := range placeholder.FindAllStringSubmatch(s, -1) {
		out = append(out, m[1])
	}
	return out
}
