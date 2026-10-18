package validate

import (
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
		return
	}
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
		return
	}
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

// arm7: AN EVENTUAL CLAIM TYPE WITH NO BOUND (7a), AND ONE BOUNDED ON QUIESCE
// WITH NO QUIESCE PROBE DECLARED (7b).
//
// internal/bindings leaves `bound` optional on purpose -- its comment at the
// timing key says THE BOUND IS NOT REQUIRED HERE -- precisely so this arm can
// fire instead of the parser refusing the file. 7b is docs/contract.md:341's
// "second half of the eventual arm".
func arm7(rep *Report, b bindings.Bindings) {
	var noBound, noProbe []string
	for t, c := range b.Claims {
		if c.Timing != "eventual" {
			continue
		}
		switch {
		case c.Bound == "":
			noBound = append(noBound, t)
		case c.Bound == "quiesce" && len(b.Quiesce) == 0:
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
		return // no statute is not this arm's finding
	}
	if !strings.Contains(string(src), "input.clock") {
		return
	}
	if len(b.Clock) == 0 {
		rep.refuse("8", "the statute reads input.clock and bindings.toml declares no [clock] table: "+
			"the rule would read whatever the host clock said, which no seed can reproduce")
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
	path := p.SeedsFile
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	if _, err := os.Stat(path); err != nil {
		rep.refuse("10a", "seeds_file names %q and it is not there: the promoted seeds a run would "+
			"replay first cannot be read", p.SeedsFile)
	}
}
