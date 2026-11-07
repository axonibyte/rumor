package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/stage"
)

// browserPair is a browser and the driver binary that speaks W3C WebDriver to
// it. BOTH halves are needed and neither is sufficient.
//
// WHY PAIRS AND NOT NAMES (fable, coop r9/73, on the measurement at r9/71):
// R9.1b drives a browser over W3C WebDriver, which is spoken by a DRIVER
// process and not by the browser itself. The six names this scan grew with are
// two protocol families -- chromium's kin answer chromedriver, firefox answers
// geckodriver -- so a list of browsers cannot say whether any of them is
// drivable. Measured: firefox 158's own `--help` says its
// `--remote-debugging-port` serves WebDriver BiDi and it has no CDP, and the
// jail holding it has neither geckodriver nor chromedriver, while forge-01's ci
// jail holds chromium 156 AND chromedriver from the same package.
//
// A BROWSER WITHOUT ITS DRIVER IS FOUND, NOT DRIVABLE, and that is rook's point
// at coop r9/72: it must never read as a met precondition. The old scan would
// have selected firefox here -- position four of six, the first name present --
// and handed a later head a browser nothing in this tree can speak to.
type browserPair struct{ browser, driver string }

// browserPairs are the pairs a driver is looked for under, in order.
//
// NAMED AND NOT GUESSED: a stage that searched for "a browser" could not tell a
// reader what it failed to find, and the skip below has to name it or the
// operator cannot act on it.
var browserPairs = []browserPair{
	{"chrome", "chromedriver"},
	{"chromium", "chromedriver"},
	{"chromium-browser", "chromedriver"},
	{"firefox", "geckodriver"},
	{"google-chrome", "chromedriver"},
	{"headless_shell", "chromedriver"},
}

// browserDriverEnv names a BROWSER outright, for a box that holds one under
// another name and for the arms, which must drive every path without one.
const browserDriverEnv = "RUMOR_BROWSER"

// webDriverEnv names the DRIVER outright, and it exists because the pair's
// other half cannot be guessed from an arbitrary path: `RUMOR_BROWSER` may name
// a build under any filename, and a driver inferred from one would be a guess
// presented as a resolution. Where the named browser's BASENAME is a pair this
// build knows, its driver is taken from the table and the `how` says so.
const webDriverEnv = "RUMOR_WEBDRIVER"

// driverFor is the driver a named browser pairs with, by BASENAME, and whether
// this build knows one. A path is matched on its last element so
// `/usr/local/bin/chromium` pairs as `chromium` does.
func driverFor(browser string) (string, bool) {
	base := filepath.Base(browser)
	for _, p := range browserPairs {
		if p.browser == base {
			return p.driver, true
		}
	}
	return "", false
}

// pairNames is the pairs as a reader needs them in a refusal: both halves of
// each, so the operator is told what to install and not merely what is absent.
func pairNames() []string {
	out := make([]string, 0, len(browserPairs))
	for _, p := range browserPairs {
		out = append(out, p.browser+"+"+p.driver)
	}
	return out
}

// browserDriver resolves a browser AND its driver and says HOW the pair was
// looked for, because the how is what an operator needs when the answer is
// "none" -- and WHICH HALF is missing is what they need when the answer is
// "found, not drivable" (fable, coop r9/73).
//
// A FOUND-BUT-UNDRIVABLE BROWSER DOES NOT STOP THE SCAN, and the reason is a
// box this room holds: a jail with firefox and no geckodriver beside a chromium
// with its chromedriver should run on chromium, not refuse because an earlier
// name matched. So the scan takes the FIRST COMPLETE PAIR, and every incomplete
// one it passed is NAMED in the verdict -- an operator one package away from a
// drivable box is told which package.
func browserDriver() (browser, driver, how string) {
	if named := os.Getenv(browserDriverEnv); named != "" {
		// A NAMED COMMAND IS STILL CHECKED. Trusting the variable would turn a
		// typo into "the browser is present" and move the refusal to a later
		// head, where it would read as the stage failing.
		foundBrowser, err := exec.LookPath(named)
		if err != nil {
			return "", "", fmt.Sprintf("%s names %q, which is not executable here", browserDriverEnv, named)
		}
		wantDriver := os.Getenv(webDriverEnv)
		fromTable := false
		if wantDriver == "" {
			paired, known := driverFor(named)
			if !known {
				return "", "", fmt.Sprintf("%s names %q, which is executable, and %s is unset: "+
					"this build pairs %s, and %q is none of them, so its driver cannot be inferred -- "+
					"FOUND, NOT DRIVABLE", browserDriverEnv, named, webDriverEnv,
					strings.Join(pairNames(), ", "), filepath.Base(named))
			}
			wantDriver, fromTable = paired, true
		}
		foundDriver, derr := exec.LookPath(wantDriver)
		if derr != nil {
			return "", "", fmt.Sprintf("%s names %q, which is executable, and its driver %q is NOT on PATH "+
				"-- FOUND, NOT DRIVABLE: W3C WebDriver is spoken by the driver and not by the browser",
				browserDriverEnv, named, wantDriver)
		}
		if fromTable {
			return foundBrowser, foundDriver, fmt.Sprintf("%s=%s with its paired driver %s on PATH",
				browserDriverEnv, named, wantDriver)
		}
		return foundBrowser, foundDriver, fmt.Sprintf("%s=%s and %s=%s", browserDriverEnv, named, webDriverEnv, wantDriver)
	}
	var undrivable []string
	for _, p := range browserPairs {
		foundBrowser, err := exec.LookPath(p.browser)
		if err != nil {
			continue
		}
		foundDriver, derr := exec.LookPath(p.driver)
		if derr != nil {
			undrivable = append(undrivable, fmt.Sprintf("%s without %s", p.browser, p.driver))
			continue
		}
		return foundBrowser, foundDriver, fmt.Sprintf("%s with %s on PATH", p.browser, p.driver)
	}
	if len(undrivable) > 0 {
		return "", "", fmt.Sprintf("FOUND, NOT DRIVABLE: %s on PATH, so no pair is complete; "+
			"this build pairs %s, and %s is unset", strings.Join(undrivable, " and "),
			strings.Join(pairNames(), ", "), browserDriverEnv)
	}
	return "", "", fmt.Sprintf("no pair of %s on PATH, and %s unset",
		strings.Join(pairNames(), ", "), browserDriverEnv)
}

// flowsDeclared is how many actions carry a UI flow, which is what this stage
// exists to drive.
//
// IT COUNTS ACTIONS AND NOT STEPS: an action is what a walk selects, so the
// figure a reader compares against the run is the number of drivable actions.
func flowsDeclared(b bindings.Bindings) int {
	n := 0
	for _, a := range b.Actions {
		if len(a.Flow) > 0 {
			n++
		}
	}
	return n
}

// refusedTokens is the only way anything inside this stage could obtain an API
// session token, and it refuses every ask.
//
// THERE IS NO INTERFACE HERE, DELIBERATELY. An `apiTokens` interface stood here
// and nothing consumed it: the stage holds the concrete refuser, so the
// abstraction had exactly one implementation and no caller that varied. It was
// an unused unexported type, which `go build` and `go vet` accept and
// golangci-lint's `unused` does not -- so it passed every local suite and
// failed CI in the lint step, which is how 9884fad went red with its parent
// green (rook's sort at coop rumor/1863, the duration argument at /1866).
// Re-introduce an interface when a second implementation exists, not before.
//
// THIS IS DELIVERABLE 3's CONDUCT HALF (fable, coop r9/5, amending 120): the
// property is that the browser stage acquires no API session OVER ITS RUN,
// asserted after it has had the chance -- not at startup, which has no subject,
// because every stage builds its own bookkeeping and `Sessions` acquires at
// first use, so a map read before the stage acts is empty on a working build and
// a broken one alike (measured at r9/4).
//
// THE CAPABILITY IS DENIED IN THE PRODUCT, NOT ONLY IN AN ARM. Handing the stage
// a working Sessions and checking a counter in a test would leave the production
// path and the armed path different, which is the fake-wider-than-the-real trap
// docs/testing-methodology.md names. Handed this, any future code in this stage
// that asks for a token gets an error naming the rule IN A REAL RUN, and the arm
// reads that same refusal.
//
// THE DECLARATION HALF IS NOT HERE AND IS NOT MISSING. The other side of this
// rule -- that a project may not DECLARE a role holding a browser context which
// also reaches non-browser actions -- is refused by internal/validate's arm 5
// ("browser-stage actor(s) reaching a non-browser action"), driven in-tree by
// tests/validate/fixtures/5-browser-reaches-api, kept reachable by
// internal/bindings' own cell requiring such a role to PARSE, and accounted for
// in tests/validate's arm list. Four tiers, none of them here, and this pointer
// is what says which guard this stage rests on rather than a fifth assertion.
//
// 9884fad's MESSAGE SAYS THAT HALF "DOES NOT EXIST", AND IT IS WRONG. I searched
// for it through a `head -8` and arms_platform.go sorts last of validate's arms
// files, so the decisive line was in the tail I discarded; I then published the
// absence. Corrected at coop r9/7 and r9/8, and recorded here because the commit
// is pushed and its message cannot be.
//
// WHAT IT DOES NOT PROVE: a driver that opened its own `wire.Session` directly
// would bypass this seam entirely. This enforces the rule against the stage's
// own token source; it is not a proof that no API traffic can leave the stage,
// and the driver's head owes that separately.
// refusedTokens answers every ask with the rule and remembers it was asked.
type refusedTokens struct {
	asked []string
}

func (r *refusedTokens) Token(actor, ctxName string) (string, error) {
	r.asked = append(r.asked, actor+"/"+ctxName)
	return "", fmt.Errorf("the browser stage may not hold an API session, and %q asked for a %q "+
		"token: actors in this stage act through the browser and speak only through tells (issue 120)",
		actor, ctxName)
}

// conductRefusal is the verdict on the stage's own conduct once it has had the
// chance to ask.
//
// IT IS A FUNCTION SO THE DECISION HAS AN ARM. While the stage asks for nothing
// no run can reach the non-empty branch, so the refusal's words and its status
// would be armed by nothing if they lived inline -- the same reasoning as
// missingRunners in run.go. The arm drives this directly with a refuser that WAS
// asked, and separately drives the live stage to show it asks nothing.
func conductRefusal(r *refusedTokens) (string, bool) {
	if len(r.asked) == 0 {
		return "", false
	}
	return fmt.Sprintf("the browser stage asked for %d API session token(s) -- %s -- and a stage "+
		"whose actors act through the browser must hold none", len(r.asked), strings.Join(r.asked, ", ")), true
}

// browserStage is R9's `browser` stage, dispatched by `rumor run` under the one
// name (issue 120, deliverable 5).
//
// WHAT THIS HEAD DOES AND WHAT IT DOES NOT. It makes the stage DISPATCHED and
// reports what it met: a driver, or none. It drives no flow yet -- that is
// R9.1's next head and the line says so, because a stage that printed nothing
// would be indistinguishable from one that ran and found nothing.
//
// A MISSING DRIVER IS A SKIP AND NOT A REFUSAL, by issue 120's deliverable 5:
// the project is not wrong and the build is not broken, so exit 0 with the
// reason named. A refusal here would fail every run on every box without a
// browser, which today is this one.
func browserStage(b bindings.Bindings, stdout, stderr io.Writer) int {
	// THE STAGE'S ONLY TOKEN SOURCE, AND IT REFUSES. Nothing below asks it for
	// anything; the point is that asking is impossible to do quietly.
	tokens := &refusedTokens{}
	browser, driver, how := browserDriver()
	flows := flowsDeclared(b)
	// "THIS STAGE DROVE NONE" AND NOT "NONE WAS DRIVEN", and H2 is why: the WALK
	// now drives a browser-context action's flow through the adapter, so a run
	// can print "1 step(s) through `flow`" in its summary and this line three
	// lines later. Measured on examples/toy under the `clicking` profile with
	// ZERO browser binaries on PATH -- both sentences true, one a count of the
	// stage's work and the other of the walk's, and a reader seeing "none was
	// driven" beside the summary reads a contradiction. A true sentence that
	// reads as "no flow was driven at all" is the silent-skip class wearing
	// honest words.
	if browser == "" {
		// THE SKIP NAMES WHICH HALF IS MISSING, which is the whole of the pair
		// ruling as a reader meets it: "no pair" and "a browser without its
		// driver" are different states and only the second tells an operator
		// they are one package away. `how` carries the distinction.
		_, _ = fmt.Fprintf(stdout, "run:   %s -- SKIPPED, no drivable browser: %s. %d action(s) declare a flow and THIS STAGE drove none; the walk drives them through the adapter\n",
			stage.Browser, how, flows)
		return conductVerdict(tokens, stderr)
	}
	_, _ = fmt.Fprintf(stdout, "run:   %s -- browser %s driven by %s (%s). %d action(s) declare a flow and THIS STAGE drove none; the walk drives them through the adapter, and driving them through the browser is R9.1b's next head\n",
		stage.Browser, browser, driver, how, flows)
	return conductVerdict(tokens, stderr)
}

// conductVerdict reports the conduct refusal if the stage asked for a token, and
// is the one place the stage's exit status depends on it.
//
// AFTER THE BODY AND NOT BEFORE, which is the ruling's "after it has had the
// chance": a check in front of the work would be the startup form that has no
// subject.
func conductVerdict(tokens *refusedTokens, stderr io.Writer) int {
	if msg, asked := conductRefusal(tokens); asked {
		_, _ = fmt.Fprintf(stderr, "run: could not run: %s\n", msg)
		return 2
	}
	return 0
}
