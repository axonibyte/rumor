package validate

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/platform"
)

// browserEnabled reports whether the browser stage is on.
//
// BOTH BROWSER ARMS ARE VACUOUS WITHOUT IT and docs/contract.md:419 says so:
// `stages.browser.enabled` "is what the two browser-stage arms read, and both
// are vacuous without it". So the flag is not a convenience here -- it is what
// makes the two arms mean anything, and a fixture for either has to ENABLE the
// stage rather than merely add or remove a flow.
func browserEnabled(p platform.Platform) bool {
	st, ok := p.Stages["browser"]
	return ok && st.Enabled
}

// isBrowser reports whether a context is the browser one. One name today, and
// the test is a helper rather than a literal at three call sites so a second
// browser context has one place to arrive.
func isBrowser(ctx string) bool { return ctx == "browser" }

// arm4: A BROWSER-STAGE ACTION WITH NO UI FLOW.
//
// docs/contract.md:295: "An action whose context is a browser context declares
// a flow instead of a response", and :330: this arm "reads [[action.flow]]; it
// is what makes 'the browser stage is enabled' more than a flag".
//
// THE NUMBERING OF 4 AND 5 IS MINE AND FABLE'S TO SWAP: their /60 names "4 and
// 5 (browser)" without saying which is which, and the epic's bullet order is
// not in this tree. The conditions are the documented pair; if the labels are
// backwards, the fixture directories rename and nothing else moves.
func arm4(rep *Report, b bindings.Bindings, p platform.Platform) {
	if !browserEnabled(p) {
		rep.skip("4", "stages.browser.enabled is not true, and this arm is vacuous without it")
		return
	}
	rep.ran("4")
	var bad []string
	for _, a := range b.Actions {
		if isBrowser(a.Context) && len(a.Flow) == 0 {
			bad = append(bad, a.Name)
		}
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		rep.refuse("4", "no UI flow on browser-stage action(s): %s -- a browser action declares a flow "+
			"instead of a response, so the stage has nothing to drive", strings.Join(bad, ", "))
	}
}

// arm5: A BROWSER-STAGE ACTOR THAT COULD REACH AN API ACTION.
//
// docs/contract.md:69 defines reachability exactly: "could reach" is "its
// context is one that actor holds a session in". So a role holding the browser
// context AND the context of a non-browser action could take that action from a
// browser session, which is what the browser stage's one-isolated-context-per-
// actor property exists to prevent (:161).
func arm5(rep *Report, b bindings.Bindings, p platform.Platform) {
	if !browserEnabled(p) {
		rep.skip("5", "stages.browser.enabled is not true, and this arm is vacuous without it")
		return
	}
	rep.ran("5")
	byContext := map[string][]string{}
	for _, a := range b.Actions {
		if !isBrowser(a.Context) {
			byContext[a.Context] = append(byContext[a.Context], a.Name)
		}
	}
	var bad []string
	for _, r := range b.Roles {
		holdsBrowser := false
		for _, c := range r.Contexts {
			if isBrowser(c) {
				holdsBrowser = true
			}
		}
		if !holdsBrowser {
			continue
		}
		for _, c := range r.Contexts {
			if names := byContext[c]; len(names) > 0 {
				sort.Strings(names)
				bad = append(bad, r.Name+" holds "+c+" and could reach "+strings.Join(names, "/"))
			}
		}
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		rep.refuse("5", "browser-stage actor(s) reaching a non-browser action: %s -- one isolated "+
			"context per actor is what the browser stage rests on", strings.Join(bad, "; "))
	}
}

// probeKinds is the set [[quiesce.probe]].kind may name. It lives here and not
// in bindings because the PARSER requires the key and validate owns its VALUE:
// bindings.go reads `kind` with required=true so an absent key is a malformed
// file, and an unknown one is a thing to say about the project, by name, with a
// fixture that trips it.
var probeKinds = map[string]bool{"queue_depth": true, "worker_idle": true, "direct": true}

// arm7: AN EVENTUAL CLAIM TYPE WITH NO BOUND (7a), ONE BOUNDED ON QUIESCE WITH
// NO QUIESCE PROBE DECLARED (7b), A PROBE WITH NO TIMEOUT (7c), A PROBE OF AN
// UNKNOWN KIND (7d), AND `query` ON A PROBE THAT IS NOT `direct` (7e).
//
// internal/bindings leaves `bound` optional on purpose -- its comment at the
// timing key says THE BOUND IS NOT REQUIRED HERE -- precisely so this arm can
// fire instead of the parser refusing the file. 7b is docs/contract.md:341's
// "second half of the eventual arm".
//
// 7c AND 7d ARE RULED IDS AND NOT A FOLD INTO 7b (fable, coop r6/27): "every
// refusal gets its own id and its own fixture that trips it". Both are the same
// SUBJECT as 7b reached from the probe's end, so folding looked right until
// TestEveryFixtureTripsExactlyItsOwnArm refused it -- "2 fixtures claim arm 7b:
// one fixture per arm is the invariant" -- and under 7b WITHOUT a fixture they
// would reproduce rook's arm-19 finding exactly: a refusal nothing trips, which
// is deletable in silence. The count moved 19 -> 21 with the accounting posted
// at coop r6/28 first, which is breakout-r4/6's rule.
//
// 7e WAS A SENTENCE AND NOT A MECHANISM, which is why it is an arm. bindings.go
// described Query as "the `direct` kind's observation name, and is refused on
// the other two -- a queue depth is not read through an observation", and
// nothing refused it: a declaration could carry a query on a queue_depth probe
// and validate passed it. That is arm 19's shape written in prose, and fable
// ruled it an id of its own at coop r6/33 with the count posted at r6/36.
//
// A PROBE IS IDENTIFIED BY ITS POSITION because nothing names it. `[[quiesce.probe]]`
// entries carry no key, so the refusal says "probe 1" in declaration order and
// the kind beside it, which is the most a reader can be given to find the line.
func arm7(rep *Report, b bindings.Bindings) {
	rep.ran("7a")
	rep.ran("7b")
	rep.ran("7c")
	rep.ran("7d")
	rep.ran("7e")
	var noBound, noProbe []string
	for t, c := range b.Claims {
		if c.Timing != "eventual" {
			continue
		}
		switch {
		case c.Bound == "":
			noBound = append(noBound, t)
		// THE TYPED PROBES, NOT THE RESERVED MAP, and the difference is a
		// hole the typing opened. Quiesce carries every sibling key, so
		// `[quiesce]` holding ANY of them while declaring no probe made this
		// check pass with ZERO probes the engine can ask. Probes is what "a
		// condition nothing can evaluate" is actually about.
		//
		// THE LIVE SHAPE IS A SIBLING KEY AND NOT THE OLD SPELLING. This
		// comment cited `probe = "/test/quiet"` as the example, and rook
		// measured at coop rumor/1026 that it cannot occur at this head: the
		// typed array-of-tables took that key at 4045c16, so the old form is a
		// PARSE refusal -- "quiesce.probe: string: wanted an array of tables"
		// -- before any arm runs. An illustration of a state the parser refuses
		// reads as a live hazard, which is worse than no example. Driven, the
		// shape that does occur: the twin plus `[quiesce]` with `note = "..."`
		// and an eventual quiesce bound refuses 7b by name.
		case c.Bound == "quiesce" && len(b.Probes) == 0:
			noProbe = append(noProbe, t)
		}
	}
	if len(noBound) > 0 {
		sort.Strings(noBound)
		rep.refuse("7a", "eventual claim type(s) with no bound: %s -- nothing says how long the walk "+
			"waits before the claim has not settled", strings.Join(noBound, ", "))
	}
	if len(noProbe) > 0 {
		sort.Strings(noProbe)
		rep.refuse("7b", "claim type(s) bounded on quiesce with no [quiesce] probe declared: %s -- "+
			"the bound names a condition nothing can evaluate", strings.Join(noProbe, ", "))
	}

	var noTimeout, badKind, strayQuery []string
	for i, p := range b.Probes {
		where := fmt.Sprintf("probe %d (kind %q)", i+1, p.Kind)
		if p.Timeout == "" {
			noTimeout = append(noTimeout, where)
		}
		if !probeKinds[p.Kind] {
			badKind = append(badKind, where)
		}
		// ONLY ON A KIND THIS ARM KNOWS. A probe of an UNKNOWN kind carrying a
		// query is 7d's to refuse and not also this one: whether a query
		// belongs to a kind nobody has defined is not a question, and refusing
		// both would make one declaration trip two arms -- which the fixture
		// invariant forbids and which would tell its reader two things when
		// one of them is the cause.
		if p.Query != "" && probeKinds[p.Kind] && p.Kind != "direct" {
			strayQuery = append(strayQuery, where)
		}
	}
	if len(noTimeout) > 0 {
		rep.refuse("7c", "[[quiesce.probe]] with no timeout: %s -- a probe without one is a wait "+
			"with no end, and a system that never goes quiet has to be a finding rather than a hang",
			strings.Join(noTimeout, ", "))
	}
	if len(strayQuery) > 0 {
		rep.refuse("7e", "[[quiesce.probe]] declaring `query` on a kind that does not read one: %s "+
			"-- only the `direct` kind answers through an observation, and a queue depth or an "+
			"idle worker is a property the adapter reports", strings.Join(strayQuery, ", "))
	}
	if len(badKind) > 0 {
		kinds := make([]string, 0, len(probeKinds))
		for k := range probeKinds {
			kinds = append(kinds, k)
		}
		sort.Strings(kinds)
		rep.refuse("7d", "[[quiesce.probe]] of an unknown kind: %s -- the kinds are %s, and nothing "+
			"can answer a probe whose kind no adapter reports", strings.Join(badKind, ", "),
			strings.Join(kinds, ", "))
	}
}

// arm8: A RULE READING `clock` WITH NO CLOCK CONTROL.
//
// docs/contract.md:340: the arm "refuses on this table's absence -- so it has
// to live here to be absent from here". The rule side is read from the statute
// text, because that is where a rule reaching for the clock is visible at all:
// `input.clock` is the engine's channel for a controlled time, and a statute
// using it when no [clock] table declares one reads whatever the host clock
// said.
func arm8(rep *Report, dir string, b bindings.Bindings) {
	src, err := os.ReadFile(filepath.Join(dir, "statute.rego"))
	if err != nil {
		rep.skip("8", "no statute.rego to read, so whether a rule reaches for the clock is unasked")
		return
	}
	// READ AND PASSED IS NOT SKIPPED: a statute that never touches input.clock
	// is one this arm EVALUATED and found nothing wrong with.
	rep.ran("8")
	if !strings.Contains(string(src), "input.clock") {
		return
	}
	// DECLARED, NOT KEYED. `b.Clock` is nil when the table is ABSENT and an
	// empty non-nil map when it is present and bare, because internal/bindings
	// assigns the carried map only inside `if t, ok := r.Table(...); ok`. A
	// length test reads a bare `[clock]` as no clock at all and refuses a
	// project that declares control -- see arm20 below, where rook measured the
	// same expression failing the other way.
	if b.Clock == nil {
		rep.refuse("8", "the statute reads input.clock and bindings.toml declares no [clock] table: "+
			"the rule would read whatever the host clock said, which no seed can reproduce")
	}
}

// arm20: A `[deadline]` TABLE WITH NO CLOCK CONTROL.
//
// R7's THIRD RULING, and docs/contract.md carried the row for it with
// "nothing yet" in the refused-by column from R7.1 until this arm: "a project
// with no `[clock]` has no clock, and that is R7's third ruling rather than a
// silence: nothing is read as `time did not pass`". Two of the three refusals
// that ruling names already existed -- a rule reading `clock` (arm 8, above)
// and an eventual type bounded by a duration (the engine, at run time) -- and
// this is the third.
//
// WHY IT IS AN ARM AND NOT A PARSE REFUSAL, which is the line arm 8 sits on
// too: `[deadline]` is well-formed on its own and means nothing wrong until you
// know what else the project declares. A parser refusing it could not tell a
// clocked project from an unclocked one, because the two tables are read in the
// same pass; the pairing is what is wrong, and a pairing is what an arm is for.
//
// BOTH SIDES ARE PRESENCE AND THE CODE SAID LENGTH UNTIL ROOK DROVE IT
// (coop rumor/1354, measured at 9ab53f2 on one toy with only the table
// differing): `[deadline]` WITH A KEY and no `[clock]` refused at rc 1, and
// `[deadline]` BARE and no `[clock]` passed at rc 0, 23 of 23 evaluated. A bare
// table is presence without content -- the exact pairing this arm exists to
// refuse -- and `len(b.Deadline) == 0` could not see it while two comments here
// said presence. The distinction the fix rests on is real and now pinned in
// internal/bindings: an ABSENT reserved table leaves its field NIL, a bare one
// assigns an empty non-nil map.
//
// AND THE SAME EXPRESSION FAILED THE OTHER WAY ON THE CLOCK SIDE, which rook's
// drive did not reach: a keyed `[deadline]` beside a BARE `[clock]` would have
// been refused for declaring no clock, a false refusal of a project that
// declares control. One wrong idiom, two directions, and only the first had a
// witness. cmd/rumor/run.go had it right all along -- `brep.Bindings.Clock !=
// nil` is what gives the run its clock -- so the arms now ask the question the
// run answers.
//
// THIS ARM READS THE BINDINGS ALONE and evaluates
// wherever they can be read -- no statute, no adapter, no platform file. That is
// why it moves all three of the documented figures by one rather than only the
// first (docs/testing-methodology.md's twenty-three / twenty-one / nineteen),
// the way R6.2's three arms did and arm 19 did not.
//
// fable ruled the id at coop r7/48: a fresh number, because a cited identifier
// does not move for a sibling and the lettered families are the ones born
// together, which 8 and 20 are not.
func arm20(rep *Report, b bindings.Bindings) {
	// READ AND PASSED IS NOT SKIPPED, as arm 8 says of itself: a project
	// declaring no `[deadline]` is one this arm evaluated and found nothing
	// wrong with. There is nothing here that can fail to be read, so this arm
	// never skips.
	rep.ran("20")
	if b.Deadline == nil {
		return
	}
	if b.Clock == nil {
		rep.refuse("20", "bindings.toml declares a [deadline] table and no [clock] table: "+
			"a deadline is an instant the run has to be able to reach, and a project with no "+
			"clock control cannot move its own time, so every such field would be judged "+
			"against whatever the host clock said -- which no seed can reproduce")
	}
}

// arm10a: A SEEDS FILE THAT IS NOT THERE.
//
// IT FIRES ON THE DEFAULT TOO, and that decision is the interesting one.
// internal/platform:164 fills `seeds_file` in with seeds.lock when the key is
// absent, so the parsed struct CANNOT tell a declared path from a defaulted
// one -- a default erases the distinction the way any fallback does.
//
// The tempting fix was to fire only on a path that differs from the default,
// which would have let the permitted twin pass while carrying no seeds file.
// That is narrowing an arm so a fixture can pass it, and fable refused exactly
// that shape at coop r4/63 over attaint's corroboration control: the twin EARNS
// a guard rather than being waved through it. So the twin carries a seeds.lock
// -- empty, because no seed has been promoted and R5 owns the entry format --
// and this arm asks the question of every project.
func arm10a(rep *Report, dir string, p platform.Platform) {
	rep.ran("10a")
	path := p.SeedsFile
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	if _, err := os.Stat(path); err != nil {
		rep.refuse("10a", "seeds_file names %q and it is not there: the promoted seeds a run would "+
			"replay first cannot be read", p.SeedsFile)
	}
}

// arm10b: A PROMOTED SEED THAT CANNOT BE READ.
//
// THE FORMAT IS docs/contract.md's "The seeds file", fixed in R4 precisely so
// this arm has something to read (fable, coop r4/74): one seed per line, a
// non-negative integer, optionally followed by `#` and a comment. Blank lines
// and comment-only lines are ignored -- a comment allowed after a seed and
// refused on its own line would be a distinction with nothing behind it, and
// the twin's own seeds.lock is a file whose only content is why it is empty.
//
// "A promoted seed that cannot be read is a promoted seed missing": a run that
// skipped the line would replay less than it reported, so the line is NAMED
// rather than counted.
func arm10b(rep *Report, dir string, p platform.Platform) {
	path := p.SeedsFile
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		rep.skip("10b", "there is no seeds file to read; its absence is arm 10a's finding")
		return
	}
	rep.ran("10b")
	var bad []string
	for i, line := range strings.Split(string(raw), "\n") {
		// THE PARSE IS platform's AND NOT A SECOND COPY. This arm had its own
		// reading of the grammar, and so did internal/shrink, which is how a
		// TOKEN on the line could have been read three ways (R6.3).
		parsed, isSeed, perr := platform.ParseSeedLine(line)
		if !isSeed {
			continue
		}
		if perr != nil {
			bad = append(bad, fmt.Sprintf("line %d: %q -- %v", i+1, strings.TrimSpace(line), perr))
			continue
		}
		if !isNonNegativeInt(parsed.Seed) {
			bad = append(bad, fmt.Sprintf("line %d: %q", i+1, strings.TrimSpace(line)))
			continue
		}
		// AND A PROFILE A SEED NAMES MUST BE DECLARED. A seed line asking for
		// weights the platform does not have would REPLAY UNDER THE DEFAULT
		// MIX while its author believed otherwise -- a promoted seed that
		// silently stops being about what it caught, which is the same shape
		// as a seed that cannot be read at all and the reason this arm exists.
		if parsed.Profile != "" {
			if _, ok := p.Profiles[parsed.Profile]; !ok {
				declared := make([]string, 0, len(p.Profiles))
				for name := range p.Profiles {
					declared = append(declared, name)
				}
				sort.Strings(declared)
				named := "none"
				if len(declared) > 0 {
					named = strings.Join(declared, ", ")
				}
				bad = append(bad, fmt.Sprintf("line %d: seed %s names profile %q and the "+
					"platform declares %s", i+1, parsed.Seed, parsed.Profile, named))
			}
		}
	}
	if len(bad) > 0 {
		rep.refuse("10b", "seeds_file holds line(s) that are not a promoted seed: %s -- a seed that "+
			"cannot be read is a seed a replay would skip", strings.Join(bad, "; "))
	}
}

// isNonNegativeInt is the seed grammar, written out rather than delegated to
// strconv so a leading `+`, a sign, whitespace or an empty string are all
// refused by the same reading the document states.
func isNonNegativeInt(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
