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
func browserStage(b bindings.Bindings, stdout io.Writer) int {
	driver, how := browserDriver()
	flows := flowsDeclared(b)
	if driver == "" {
		_, _ = fmt.Fprintf(stdout, "run:   %s -- SKIPPED, no driver: %s. %d action(s) declare a flow and none was driven\n",
			stage.Browser, how, flows)
		return 0
	}
	_, _ = fmt.Fprintf(stdout, "run:   %s -- driver %s (%s). %d action(s) declare a flow and none was driven yet: the flows are R9.1's next head\n",
		stage.Browser, driver, how, flows)
	return 0
}
