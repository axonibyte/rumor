package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/stage"
)

// browserCandidates are the commands a driver is looked for under, in order.
//
// NAMED AND NOT GUESSED: a stage that searched for "a browser" could not tell a
// reader what it failed to find, and the skip below has to name it or the
// operator cannot act on it.
var browserCandidates = []string{"chrome", "chromium", "chromium-browser", "firefox", "google-chrome", "headless_shell"}

// browserDriverEnv names a command outright, for a box that holds one under
// another name and for the arms, which must drive BOTH paths without a browser.
const browserDriverEnv = "RUMOR_BROWSER"

// browserDriver resolves the driver and says HOW it was looked for, because the
// how is what an operator needs when the answer is "none".
func browserDriver() (path, how string) {
	if named := os.Getenv(browserDriverEnv); named != "" {
		// A NAMED COMMAND IS STILL CHECKED. Trusting the variable would turn a
		// typo into "the driver is present" and move the refusal to a later
		// head, where it would read as the stage failing.
		found, err := exec.LookPath(named)
		if err != nil {
			return "", fmt.Sprintf("%s names %q, which is not executable here", browserDriverEnv, named)
		}
		return found, fmt.Sprintf("%s=%s", browserDriverEnv, named)
	}
	for _, name := range browserCandidates {
		if found, err := exec.LookPath(name); err == nil {
			return found, fmt.Sprintf("%s on PATH", name)
		}
	}
	return "", fmt.Sprintf("none of %s on PATH, and %s unset",
		strings.Join(browserCandidates, ", "), browserDriverEnv)
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
	driver, how := browserDriver()
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
	if driver == "" {
		_, _ = fmt.Fprintf(stdout, "run:   %s -- SKIPPED, no driver: %s. %d action(s) declare a flow and THIS STAGE drove none; the walk drives them through the adapter\n",
			stage.Browser, how, flows)
		return conductVerdict(tokens, stderr)
	}
	_, _ = fmt.Fprintf(stdout, "run:   %s -- driver %s (%s). %d action(s) declare a flow and THIS STAGE drove none; the walk drives them through the adapter, and driving them through a browser here is R9.1b's\n",
		stage.Browser, driver, how, flows)
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
