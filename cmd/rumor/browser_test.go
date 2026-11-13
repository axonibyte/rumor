package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/browser/fakedriver"
	"forge.axonibyte.com/axonibyte/rumor/internal/proof"
	"forge.axonibyte.com/axonibyte/rumor/internal/stage"
)

// THE SEAM IS DENIED, NOT TRUSTED. Every arm here points PATH at an empty
// directory first, so a box that HAS a browser cannot make the absent-driver
// arms pass for the wrong reason -- and a box that lacks one (this jail today)
// cannot make the present-driver arm vacuous. Without that, each arm would be
// measuring the box rather than the stage.
func emptyPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	t.Setenv(browserDriverEnv, "")
	t.Setenv(webDriverEnv, "")
	if _, _, how := browserDriver(); !strings.Contains(how, "no pair of") {
		t.Fatalf("PATH was emptied and a pair was still found (%s), so no arm below is about the stage", how)
	}
	return dir
}

func flowBindings(withFlow, without int) bindings.Bindings {
	var b bindings.Bindings
	for i := 0; i < withFlow; i++ {
		b.Actions = append(b.Actions, bindings.Action{
			Name: "flows",
			Flow: []bindings.Step{{Step: bindings.StepNavigate, To: "/ui/events/1"}, {Step: bindings.StepClick, Target: "leave"}},
		})
	}
	for i := 0; i < without; i++ {
		b.Actions = append(b.Actions, bindings.Action{Name: "api-only"})
	}
	return b
}

func TestTheBrowserStageSkipsByNameWhenNoDriverIsPresent(t *testing.T) {
	emptyPath(t)
	var out, errb bytes.Buffer
	if rc := browserStage(context.Background(), flowBindings(2, 1), runOpts{}, &out, &errb); rc != 0 {
		t.Errorf("a missing driver exited %d: deliverable 5 makes it a SKIP, and a refusal would "+
			"fail every run on every box without a browser", rc)
	}
	got := out.String()
	// THE NEEDLE MOVED WITH THE RULING, not around a failure: "no driver" became
	// "no drivable browser" because a pair is what the stage now looks for, and
	// the names it reports are pairs (fable, coop r9/73). The assertion is no
	// weaker -- it still requires the state, the stage's name, a pair by name
	// and the flow count.
	for _, want := range []string{"SKIPPED", "no drivable browser", stage.Browser,
		"chromium+chromedriver", browserDriverEnv, "2 action(s)"} {
		if !strings.Contains(got, want) {
			t.Errorf("the skip does not name %q, so a reader cannot act on it: %q", want, got)
		}
	}
}

// A SKIP THAT SAYS NOTHING IS INDISTINGUISHABLE FROM A STAGE THAT RAN. This is
// the second oracle on the arm above: the line exists, and it is not empty.
func TestTheSkipIsNotSilent(t *testing.T) {
	emptyPath(t)
	var out, errb bytes.Buffer
	_ = browserStage(context.Background(), flowBindings(1, 0), runOpts{}, &out, &errb)
	if strings.TrimSpace(out.String()) == "" {
		t.Error("the browser stage skipped and printed nothing: a silent skip reads exactly like a " +
			"stage that ran and found nothing")
	}
}

// TestTheBrowserStageNamesTheDriverItFound: RETARGETED BY R9.1b's HEAD 2 and
// not weakened. What `plant` writes is `#!/bin/sh exit 0`, which is a file on
// PATH and not a webdriver -- and as of head 2 the stage STARTS what it found,
// so this fixture now measures the state an operator with the wrong package
// installed is actually in. The naming property this arm was written for is
// unchanged and still asserted: both halves of the pair and both env vars reach
// the line. The property it can no longer hold -- that a complete pair prints
// the drivable line and no skip -- moved to the arm below, which plants a
// driver that answers the protocol. A fixture that cannot reach a claim must
// not be the thing asserting it.
func TestTheBrowserStageNamesTheDriverItFound(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake driver is a shell script")
	}
	dir := emptyPath(t)
	fake := plant(t, dir, "zz-fake-browser")
	// AND ITS DRIVER, because a browser alone is no longer a presence: under
	// fable's r9/73 ruling this box would read FOUND, NOT DRIVABLE, which the
	// arm below drives on purpose as its own case. `zz-fake-browser` pairs with
	// nothing this build knows, so the driver is named outright.
	fakeDriver := plant(t, dir, "zz-fake-webdriver")
	t.Setenv(browserDriverEnv, "zz-fake-browser")
	t.Setenv(webDriverEnv, "zz-fake-webdriver")
	var out, errb bytes.Buffer
	if rc := browserStage(context.Background(), flowBindings(3, 2), runOpts{}, &out, &errb); rc != 0 {
		t.Errorf("a driver was present and the stage exited %d", rc)
	}
	got := out.String()
	// "THIS STAGE drove none" REPLACED "none was driven yet", and the reason is
	// in browser.go: as of R9.1's H2 the WALK drives these flows through the
	// adapter, so a line saying none was driven reads as a contradiction beside
	// a summary that counts the ones that were. The arm asserts the new words
	// because the old ones would now be a false reading of a true count.
	for _, want := range []string{fake, fakeDriver, browserDriverEnv,
		webDriverEnv, "3 action(s)", "THIS STAGE drove none"} {
		if !strings.Contains(got, want) {
			t.Errorf("the line does not name %q: %q", want, got)
		}
	}
	// AND IT CLAIMS NO MORE THAN PRESENCE SUPPORTS. Without --browser-check
	// nothing was started, so "COMPLETE" and "UNKNOWN" are the whole of what
	// this stage knows; a line that said DRIVABLE here would be head 1's error
	// restated, and one that said NOT DRIVABLE would be a verdict on a launch
	// nobody attempted.
	for _, want := range []string{"the pair is COMPLETE", "UNKNOWN", "--browser-check"} {
		if !strings.Contains(got, want) {
			t.Errorf("the default line does not carry %q: %q", want, got)
		}
	}
	if strings.Contains(got, "DRIVABLE,") || strings.Contains(got, "NOT DRIVABLE") {
		t.Errorf("the default line reaches a drivability verdict with nothing started: %q", got)
	}
}

// TestWithTheCheckAPairThatIsNotAWebdriverIsFoundNotDrivable is the other half
// of the arm above: `plant` writes a file on PATH that is not a webdriver, and
// WHEN ASKED the stage starts it, fails, and names the fault. This is the state
// an operator with the wrong package installed is in, and the reason is what
// tells them so.
func TestWithTheCheckAPairThatIsNotAWebdriverIsFoundNotDrivable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture plants a shell script")
	}
	dir := emptyPath(t)
	plant(t, dir, "zz-fake-browser")
	plant(t, dir, "zz-fake-webdriver")
	t.Setenv(browserDriverEnv, "zz-fake-browser")
	t.Setenv(webDriverEnv, "zz-fake-webdriver")
	var out, errb bytes.Buffer
	if rc := browserStage(context.Background(), flowBindings(3, 2), runOpts{browserCheck: true}, &out, &errb); rc != 0 {
		t.Fatalf("an undrivable pair exited %d, wanted the skip; stderr %q", rc, errb.String())
	}
	// THE OUTCOME IS READ FROM STDERR, because a probe's verdict is a reading of
	// THIS BOX and browser.go keeps it off the hashed stream (plumb's
	// measurement, coop r9/260). The stdout side is asserted too, in the arm
	// below, so moving the line back would redden something.
	got := errb.String()
	for _, want := range []string{"FOUND, NOT DRIVABLE", "starting the driver failed", "exited"} {
		if !strings.Contains(got, want) {
			t.Errorf("a pair that is not a webdriver does not report %q on stderr: %q", want, got)
		}
	}
}

// TestTheDefaultRunStartsNoBrowserAtAll IS THE COST CLAIM, and it is the arm
// this head's first cut did not have. With the probe unconditional, CI run 1109
// died at the step's 1800s budget with no test failing, where the same suite one
// commit earlier finished the whole job in 21m17s: `tests/run` makes 55
// `rumor run` invocations and ci.yml names it the critical path, so a real
// browser start and teardown per run cost AT LEAST 9m34s -- a lower bound,
// because 1109 was killed at its cap rather than finishing (see run.go, where
// the per-launch quotient this once reported is retracted).
//
// A LINE SAYING "the pair is COMPLETE" CANNOT PROVE NOTHING WAS LAUNCHED. The
// driver's own journal can: the fake writes a line the moment it is executed, so
// an absent journal is proof that no driver process ever ran.
func TestTheDefaultRunStartsNoBrowserAtAll(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake driver is this test binary")
	}
	dir := emptyPath(t)
	plant(t, dir, "zz-fake-browser")
	journal := filepath.Join(t.TempDir(), "journal")
	t.Setenv(browserDriverEnv, "zz-fake-browser")
	t.Setenv(webDriverEnv, os.Args[0])
	t.Setenv(fakedriver.ModeEnv, "ready")
	t.Setenv(fakedriver.JournalEnv, journal)

	var out, errb bytes.Buffer
	if rc := browserStage(context.Background(), flowBindings(2, 1), runOpts{}, &out, &errb); rc != 0 {
		t.Fatalf("the default stage exited %d, stderr %q", rc, errb.String())
	}
	// THE PRECONDITION: a COMPLETE pair was found, so this arm is about the
	// stage declining to launch and not about there being nothing to launch.
	if !strings.Contains(out.String(), "the pair is COMPLETE") {
		t.Fatalf("no complete pair was found, so this arm has no subject: %q", out.String())
	}
	if _, err := os.Stat(journal); err == nil {
		raw, _ := os.ReadFile(journal)
		t.Fatalf("the default run executed the driver; its journal holds %q", string(raw))
	} else if !os.IsNotExist(err) {
		t.Fatalf("could not tell whether the driver ran: %v", err)
	}
}

// TestEveryCandidateNameIsFoundOnPath IS THE GAP ROOK NAMED IN ITS OWN CLEAR
// (coop rumor/1884): "the PRESENT driver path is exercised only by PATH-denying
// fixtures". Every present-driver arm above goes through `RUMOR_BROWSER`, so the
// CANDIDATE SCAN -- the loop over `browserCandidates` -- has only ever been
// driven in its failing direction, where it reports "none of ... on PATH". An
// empty candidate list, a misspelled entry, or a loop that never returned would
// pass every arm in this file.
//
// ONE CASE PER MEMBER, derived from `browserCandidates` rather than from a list
// here: a name added to the slice and not to a test is the thing a hand-written
// list cannot see, and a name MISSPELLED in the slice is what this catches --
// the operator's box has the binary, the scan does not find it, and the stage
// reports a skip that is false.
func TestEveryCandidateNameIsFoundOnPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake driver is a shell script")
	}
	if len(browserPairs) == 0 {
		t.Fatal("browserPairs is empty, so this arm asserts nothing and the scan can never " +
			"find a drivable browser")
	}

	// THE LIST IS DUPLICATED HERE ON PURPOSE, and a MUTANT is why. Deriving the
	// planted filenames from `browserCandidates` makes the scan's half of this
	// arm agree with the code NO MATTER WHAT: misspelling one entry to
	// "chromiumm" SURVIVED, because the arm then planted a "chromiumm" and the
	// scan duly found it. A typo there is the defect that matters most -- the
	// operator's box holds the binary, the scan misses it, and the stage reports
	// a skip that is FALSE -- and only a second, independently written list can
	// see it. The derivation above is still what catches a name added to the
	// slice and not tested; the duplication below is what catches a name
	// MISSPELLED in it. Neither alone is enough, which is the point of both.
	// BOTH HALVES OF EVERY PAIR, independently written. The misspelling that
	// survived the derived list is now TWO defects, not one: a mistyped browser
	// is a browser the scan cannot find on a box that has it, and a mistyped
	// DRIVER is a drivable box reported as "found, not drivable" -- which after
	// fable's r9/73 ruling is the state an operator acts on.
	want := []string{
		"chrome+chromedriver", "chromium+chromedriver", "chromium-browser+chromedriver",
		"firefox+geckodriver", "google-chrome+chromedriver", "headless_shell+chromedriver",
	}
	if !slices.Equal(slices.Sorted(slices.Values(pairNames())), slices.Sorted(slices.Values(want))) {
		t.Errorf("browserPairs is %v and this arm expects %v: a pair added here needs a line "+
			"in the other list, and a half that differs by a letter is either a browser this "+
			"project cannot find or a driver it reports absent on a box that holds it",
			pairNames(), want)
	}

	for _, pair := range browserPairs {
		t.Run(pair.browser, func(t *testing.T) {
			dir := emptyPath(t)
			plantedBrowser := plant(t, dir, pair.browser)
			plantedDriver := plant(t, dir, pair.driver)
			// RUMOR_BROWSER STAYS EMPTY, which emptyPath has already set: this
			// arm is about the scan and not about the named-command branch.
			browser, driver, how := browserDriver()
			if browser != plantedBrowser {
				t.Errorf("a %q on PATH resolved to %q, want %q: the scan did not find a binary "+
					"under a name it carries", pair.browser, browser, plantedBrowser)
			}
			if driver != plantedDriver {
				t.Errorf("%q paired with driver %q, want %q: the pair's other half is what makes "+
					"it drivable", pair.browser, driver, plantedDriver)
			}
			if !strings.Contains(how, pair.browser) || !strings.Contains(how, pair.driver) ||
				!strings.Contains(how, "on PATH") {
				t.Errorf("the how is %q and does not name both halves found on PATH, which is what "+
					"an operator reads when the answer is a surprise", how)
			}
		})
	}
}

// plant writes an executable under `dir` and answers its path, so an arm says
// what it planted once rather than three times.
func plant(t *testing.T, dir, name string) string {
	t.Helper()
	at := filepath.Join(dir, name)
	if err := os.WriteFile(at, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return at
}

// A BROWSER WITHOUT ITS DRIVER IS FOUND, NOT DRIVABLE -- the state fable's
// ruling at coop r9/73 creates and the one rook named at r9/72 as the dangerous
// one, because a browser on PATH reads like a met precondition and is not.
//
// THE BOX THIS IS ABOUT IS REAL AND IS MINE: firefox 158 sits on PATH in the
// jail leading R9.1b with neither geckodriver nor chromedriver beside it, and
// the scan that shipped in v0.1.48 would have selected it -- position four of
// six, the first name present -- and handed the driver's head a browser nothing
// in this tree can speak to. The arm plants the same shape rather than reading
// that box, because an arm that measured this jail would pass for the wrong
// reason anywhere else (the file's own emptyPath rule).
func TestABrowserWithoutItsDriverIsFoundNotDrivable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the planted binaries are shell scripts")
	}
	dir := emptyPath(t)
	plant(t, dir, "firefox")

	browser, driver, how := browserDriver()
	if browser != "" || driver != "" {
		t.Errorf("firefox alone resolved to browser %q driver %q: a browser whose driver is absent "+
			"is not a resolution, and reporting one hands a later head something it cannot drive",
			browser, driver)
	}
	// THE CLAUSE, NOT THE TWO WORDS -- and this is rook's finding at coop r9/98
	// against my own head 1. The verdict also embeds pairNames(), which is
	// "chrome+chromedriver, ... firefox+geckodriver, ...", so Contains(how,
	// "firefox") and Contains(how, "geckodriver") are BOTH satisfied by the pair
	// table whether the missing half is named or not. I reported the mutant that
	// drops the naming as dead; rook cut it and the suite passed, and I
	// reproduced that. The assertion could not fail for the defect it exists
	// for. "firefox without geckodriver" appears only when the undrivable entry
	// carries both names, so it discriminates and the two words do not.
	for _, want := range []string{"FOUND, NOT DRIVABLE", "firefox without geckodriver"} {
		if !strings.Contains(how, want) {
			t.Errorf("the verdict does not name %q: %q -- an operator one package away from a "+
				"drivable box has to be told which package", want, how)
		}
	}

	// AND THE STAGE SAYS SO, not just the resolver: the skip is what a reader
	// meets, and "no pair at all" and "a browser without its driver" are
	// different states that must not print the same sentence.
	var out, errb bytes.Buffer
	if rc := browserStage(context.Background(), flowBindings(1, 0), runOpts{}, &out, &errb); rc != 0 {
		t.Errorf("an undrivable browser exited %d, wanted the skip", rc)
	}
	// THE SAME CLAUSE HERE, for the same reason: the stage line carries the
	// resolver's `how` verbatim, pair table and all, so "geckodriver" alone was
	// satisfied by the table.
	if got := out.String(); !strings.Contains(got, "FOUND, NOT DRIVABLE") || !strings.Contains(got, "firefox without geckodriver") {
		t.Errorf("the skip does not carry the found-not-drivable state and the missing half: %q", got)
	}
}

// AN INCOMPLETE PAIR DOES NOT BLOCK A COMPLETE ONE, which is the precedence the
// resolver is written in and the reason is a box this room holds: a jail with
// firefox and no geckodriver beside a chromium with its chromedriver should run
// on chromium rather than refuse because an earlier name matched. Driven with
// BOTH present, since with only one either order passes.
func TestAnIncompletePairDoesNotBlockACompleteOne(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the planted binaries are shell scripts")
	}
	dir := emptyPath(t)
	// THE INCOMPLETE PAIR MUST COME FIRST IN SCAN ORDER OR THIS ARM CANNOT FAIL.
	// Written with chromium as the complete one it passed while a mutant turned
	// the loop's `continue` into a `break`: chromium is position two and firefox
	// position four, so the scan answered before it ever met the incomplete
	// pair. firefox is the incomplete one here and google-chrome the complete
	// one BEHIND it, which is the only arrangement where stopping early and
	// carrying on give different answers. Found by driving the mutant, not by
	// reading the loop.
	plant(t, dir, "firefox")
	wantBrowser := plant(t, dir, "google-chrome")
	wantDriver := plant(t, dir, "chromedriver")

	browser, driver, how := browserDriver()
	if browser != wantBrowser || driver != wantDriver {
		t.Errorf("resolved %q/%q, want the complete pair %q/%q: an undrivable earlier name must not "+
			"take the scan down with it", browser, driver, wantBrowser, wantDriver)
	}
	if !strings.Contains(how, "google-chrome") || !strings.Contains(how, "chromedriver") {
		t.Errorf("the how is %q and does not name the pair that answered", how)
	}
	// AND IT DOES NOT CLAIM THE ONE IT PASSED: naming firefox here would tell an
	// operator with a working browser about a package they do not need.
	if strings.Contains(how, "firefox") {
		t.Errorf("the how names firefox on a box with a drivable chromium: %q", how)
	}
}

// A NAMED BROWSER IS A PAIR TOO: `RUMOR_BROWSER` overrides the list and does not
// escape the rule, since W3C WebDriver is spoken by the driver either way.
func TestANamedBrowserWithoutItsDriverIsNotAPresence(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the planted binaries are shell scripts")
	}
	dir := emptyPath(t)
	plant(t, dir, "chromium")
	t.Setenv(browserDriverEnv, "chromium")

	browser, _, how := browserDriver()
	if browser != "" {
		t.Errorf("%s named a browser whose driver is absent and it resolved to %q", browserDriverEnv, browser)
	}
	for _, want := range []string{"FOUND, NOT DRIVABLE", "chromedriver"} {
		if !strings.Contains(how, want) {
			t.Errorf("the verdict does not name %q: %q", want, how)
		}
	}
}

// THE PAIR IS MATCHED ON A BASENAME, so `RUMOR_BROWSER=/usr/local/bin/chromium`
// pairs as `chromium` does. Without this an operator naming a full path -- the
// ordinary way to point at one of two installs -- would get "its driver cannot
// be inferred" for a browser this build knows by name.
func TestTheDriverIsPairedByBasename(t *testing.T) {
	for _, in := range []string{"chromium", "/usr/local/bin/chromium", "./chromium"} {
		got, known := driverFor(in)
		if !known || got != "chromedriver" {
			t.Errorf("driverFor(%q) = (%q, %v), want chromedriver: a path names the browser its "+
				"last element names", in, got, known)
		}
	}
	if _, known := driverFor("zz-not-a-browser"); known {
		t.Error("driverFor claimed a pair for a name this build does not carry, which would make an " +
			"inferred driver a guess presented as a resolution")
	}
}

// AND THE NAMED COMMAND WINS OVER A CANDIDATE ON PATH, which is the precedence
// `browserDriver` is written in and nothing asserted. Driven with BOTH present,
// because with only one of them either order passes.
func TestANamedDriverBeatsACandidateOnPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake driver is a shell script")
	}
	dir := emptyPath(t)
	// A COMPLETE PAIR FOR THE CANDIDATE, since an incomplete one is no longer a
	// presence at all and the precondition below would be measuring that
	// instead of the precedence this arm is about.
	candidate := plant(t, dir, browserPairs[0].browser)
	plant(t, dir, browserPairs[0].driver)
	named := plant(t, dir, "zz-named-browser")

	// THE PRECONDITION FIRST: with the variable unset the candidate is what the
	// scan finds, so the switch below is about precedence and not about the
	// candidate being unfindable.
	if path, _, _ := browserDriver(); path != candidate {
		t.Fatalf("with %s unset the scan found %q, want the candidate %q", browserDriverEnv, path, candidate)
	}

	// THE NAMED BROWSER CARRIES ITS DRIVER TOO: `zz-named-browser` is no pair
	// this build knows, so its driver cannot be inferred and the variable's
	// other half names it. Without this the named branch is FOUND, NOT DRIVABLE
	// and the precedence question never arises.
	t.Setenv(browserDriverEnv, "zz-named-browser")
	t.Setenv(webDriverEnv, browserPairs[0].driver)
	path, _, how := browserDriver()
	if path != named {
		t.Errorf("%s names a browser and the scan's candidate won anyway (%q): an operator naming a "+
			"command is overriding the list, not adding to it", browserDriverEnv, path)
	}
	if !strings.Contains(how, browserDriverEnv) {
		t.Errorf("the how is %q and does not name %s, so a reader cannot tell which branch answered",
			how, browserDriverEnv)
	}
}

// A TYPO IS NOT A PRESENCE, which is the same shape as pushsafe's branch check
// in my own toolset: trusting the variable would move the failure to a later
// head, where it reads as the stage breaking rather than as the name being wrong.
func TestANamedDriverThatIsNotExecutableIsNotAPresence(t *testing.T) {
	emptyPath(t)
	t.Setenv(browserDriverEnv, "zz-no-such-browser-binary")
	var out, errb bytes.Buffer
	if rc := browserStage(context.Background(), flowBindings(1, 0), runOpts{}, &out, &errb); rc != 0 {
		t.Errorf("an unresolvable %s exited %d, wanted the skip", browserDriverEnv, rc)
	}
	got := out.String()
	if !strings.Contains(got, "not executable here") || !strings.Contains(got, "zz-no-such-browser-binary") {
		t.Errorf("the skip does not say the NAMED command could not be resolved: %q", got)
	}
}

func TestFlowsDeclaredCountsActionsAndNotSteps(t *testing.T) {
	// Two actions with two steps each: an implementation counting steps says 4.
	if got := flowsDeclared(flowBindings(2, 3)); got != 2 {
		t.Errorf("flowsDeclared = %d over 2 flow-carrying actions (2 steps each) and 3 without, want 2", got)
	}
	if got := flowsDeclared(flowBindings(0, 4)); got != 0 {
		t.Errorf("flowsDeclared = %d where no action carries a flow, want 0", got)
	}
}

// DELIVERABLE 3's CONDUCT HALF, both ways (fable, coop r9/5).
//
// THE DECISION, DRIVEN DIRECTLY, because no run can reach it while the stage asks
// for nothing: a refuser that WAS asked must produce the refusal, its count and
// the actors that asked. Inline, these words would be armed by nothing.
func TestTheConductRefusalNamesWhoAskedAndHowMany(t *testing.T) {
	r := &refusedTokens{}
	if _, asked := conductRefusal(r); asked {
		t.Error("an unasked token source reported a refusal, so the verdict does not depend on conduct")
	}
	// BOTH HALVES OF THE REFUSAL, because a mutant returning a usable token
	// BESIDE the error survived a cell that read only the error: a caller that
	// ignored err would then hold a token the rule forbids. The value carries
	// nothing AND the error names the rule.
	tok, err := r.Token("member_1", "api")
	if err == nil {
		t.Error("the stage's token source handed out a token: the capability must be DENIED in the " +
			"product and not only counted in a test")
	} else if !strings.Contains(err.Error(), "may not hold an API session") {
		t.Errorf("the refusal does not name the rule: %v", err)
	}
	if tok != "" {
		t.Errorf("the refusal returned the token %q beside its error, so a caller ignoring the "+
			"error holds one anyway", tok)
	}
	_, _ = r.Token("guest_2", "api")
	msg, asked := conductRefusal(r)
	if !asked {
		t.Fatal("a token source asked twice reported no refusal")
	}
	for _, want := range []string{"2 API session token(s)", "member_1/api", "guest_2/api", "must hold none"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the refusal does not name %q: %q", want, msg)
		}
	}
}

// AND THE WIRING: the status moves with the verdict, which is the half the
// decision above cannot show.
func TestTheConductVerdictCarriesTheStatusAndTheStream(t *testing.T) {
	var errb bytes.Buffer
	if rc := conductVerdict(&refusedTokens{}, &errb); rc != 0 {
		t.Errorf("a stage that asked for nothing exited %d, want 0", rc)
	}
	if errb.String() != "" {
		t.Errorf("a clean conduct wrote to stderr: %q", errb.String())
	}
	errb.Reset()
	r := &refusedTokens{}
	_, _ = r.Token("member_1", "api")
	if rc := conductVerdict(r, &errb); rc != 2 {
		t.Errorf("a stage that took a token exited %d, want 2 (could not run)", rc)
	}
	if !strings.Contains(errb.String(), "could not run") {
		t.Errorf("the refusal did not reach stderr as a could-not-run: %q", errb.String())
	}
}

// AND THE LIVE STAGE ASKS FOR NOTHING, which is the property itself -- asserted
// after the stage has run and had the chance.
func TestTheLiveBrowserStageTakesNoAPISessionToken(t *testing.T) {
	emptyPath(t)
	var out, errb bytes.Buffer
	if rc := browserStage(context.Background(), flowBindings(2, 1), runOpts{}, &out, &errb); rc != 0 {
		t.Fatalf("the stage exited %d with stderr %q", rc, errb.String())
	}
	// PRECONDITION FIRST: the stage ran and said so, so a silent no-op cannot
	// pass this arm by having done nothing at all.
	if !strings.Contains(out.String(), stage.Browser) {
		t.Fatalf("the stage printed no line, so there is no run for this arm to be about: %q", out.String())
	}
	if errb.String() != "" {
		t.Errorf("the stage reported a conduct refusal on a clean run: %q", errb.String())
	}
}

// TestMain lets this test binary be the fake webdriver the arms below point the
// stage at, which is how a box with no chromium measures a stage whose whole
// job is starting one.
func TestMain(m *testing.M) {
	if fakedriver.MainIfAsked() {
		return
	}
	os.Exit(m.Run())
}

// TestTheStageStartsAndStopsADriverThatAnswers is head 2's deliverable as a
// reader meets it: a complete pair that WORKS prints the drivable line, and the
// stage puts back everything it took.
//
// THE JOURNAL IS THE PROOF AND THE LINE IS NOT. A stage that opened a session
// and never deleted it would print the same sentence and leak a browser per
// run, so the arm reads the driver's own record of what it was asked.
func TestTheStageStartsAndStopsADriverThatAnswers(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture plants a shell script")
	}
	for _, c := range []struct {
		name   string
		headed bool
		want   string
		unwant string
	}{
		{"headless", false, "headless", "--headless=new is absent"},
		{"headed", true, "headed", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := emptyPath(t)
			browserBin := plant(t, dir, "zz-fake-browser")
			journal := filepath.Join(t.TempDir(), "journal")
			t.Setenv(browserDriverEnv, "zz-fake-browser")
			// THE DRIVER IS THIS TEST BINARY, named outright rather than put on
			// PATH: the stage execs it with `--port=N` and the environment below
			// makes it answer as a webdriver.
			t.Setenv(webDriverEnv, os.Args[0])
			t.Setenv(fakedriver.ModeEnv, "ready")
			t.Setenv(fakedriver.JournalEnv, journal)

			var out, errb bytes.Buffer
			if rc := browserStage(context.Background(), flowBindings(2, 1), runOpts{headed: c.headed, browserCheck: true}, &out, &errb); rc != 0 {
				t.Fatalf("a working pair exited %d, stderr %q", rc, errb.String())
			}
			got := errb.String()
			for _, want := range []string{"DRIVABLE", "one session opened and closed", browserBin, c.want} {
				if !strings.Contains(got, want) {
					t.Errorf("the drivable line does not carry %q on stderr: %q", want, got)
				}
			}
			if strings.Contains(got, "SKIPPED") || strings.Contains(got, "NOT DRIVABLE") {
				t.Errorf("a pair that answers the protocol was still reported as a skip: %q", got)
			}

			raw, err := os.ReadFile(journal)
			if err != nil {
				t.Fatalf("the driver recorded nothing, so the stage drove nothing: %v", err)
			}
			record := string(raw)
			if n := strings.Count(record, "\nopen ") + boolToInt(strings.HasPrefix(record, "open ")); n != 1 {
				t.Errorf("want exactly 1 session opened, the driver was asked %d time(s): %q", n, record)
			}
			if !strings.Contains(record, "close sess-1") {
				t.Errorf("the stage opened a session and did not delete it: %q", record)
			}
			// AND THE FLAG REACHED THE BROWSER, not merely the line: the
			// capabilities the driver was handed are where `--headed` has its
			// effect, and a line that said "headed" over a headless session
			// would be the silent-skip class wearing honest words.
			if c.headed && strings.Contains(record, "--headless=new") {
				t.Errorf("--headed printed `headed` and still asked for a headless browser: %q", record)
			}
			if !c.headed && !strings.Contains(record, "--headless=new") {
				t.Errorf("a headless run did not ask for a headless browser: %q", record)
			}
		})
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// TestBrowserCheckIsParsedAndDefaultsToOff holds the flag that governs whether
// a run launches a browser at all. ITS DEFAULT IS THE ONE CI PAYS FOR: with the
// check unconditional, run 1109 died at the 1800s step budget.
func TestBrowserCheckIsParsedAndDefaultsToOff(t *testing.T) {
	for _, c := range []struct {
		args  []string
		check bool
		bad   bool
	}{
		{[]string{"proj"}, false, false},
		{[]string{"proj", "--browser-check"}, true, false},
		{[]string{"--browser-check", "proj"}, true, false},
		{[]string{"proj", "--headed", "--browser-check"}, true, false},
		{[]string{"proj", "--browser-check=yes"}, false, true},
	} {
		_, _, _, _, _, opts, bad := parseRunArgs(c.args)
		if c.bad {
			if bad == "" {
				t.Errorf("%v was accepted; --browser-check takes no value", c.args)
			}
			continue
		}
		if bad != "" {
			t.Fatalf("%v was refused: %s", c.args, bad)
		}
		if opts.browserCheck != c.check {
			t.Errorf("%v parsed browserCheck=%t, want %t", c.args, opts.browserCheck, c.check)
		}
	}
}

// TestTheHeadedFlagIsParsedAndDefaultsToHeadless holds the surface: `--headed`
// takes no value, and its ABSENCE is what every unattended run gets.
func TestTheHeadedFlagIsParsedAndDefaultsToHeadless(t *testing.T) {
	for _, c := range []struct {
		args   []string
		headed bool
		bad    bool
	}{
		{[]string{"proj"}, false, false},
		{[]string{"proj", "--headed"}, true, false},
		{[]string{"--headed", "proj"}, true, false},
		{[]string{"proj", "--headed", "--seed", "7"}, true, false},
		{[]string{"proj", "--headed=yes"}, false, true},
	} {
		dir, _, _, _, _, opts, bad := parseRunArgs(c.args)
		if c.bad {
			if bad == "" {
				t.Errorf("%v was accepted; --headed takes no value", c.args)
			}
			continue
		}
		if bad != "" {
			t.Fatalf("%v was refused: %s", c.args, bad)
		}
		if dir != "proj" {
			t.Errorf("%v read the project as %q", c.args, dir)
		}
		if opts.headed != c.headed {
			t.Errorf("%v parsed headed=%t, want %t", c.args, opts.headed, c.headed)
		}
	}
}

// TestTheHeadedFlagReachesTheBrowserFromTheCommandLine closes the gap the two
// arms above leave open between them. One measures parseRunArgs, the other
// measures browserStage; NEITHER measures main.go, where the value is handed
// across two positional call sites -- `parseRunArgs` answers seven values and
// `runProject` takes seven -- and a transposition there would pass both.
//
// THE GAP IS NOT HYPOTHETICAL IN THIS TREE. Issue 108 shipped internal/fuzz
// with zero production callers and its arms all green; fable ruled the wiring
// outstanding at coop rumor/1580. A capability nothing calls is the failure
// this project keeps finding, so the flag is measured from argv to the
// capabilities the browser was actually handed.
func TestTheHeadedFlagReachesTheBrowserFromTheCommandLine(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture plants a shell script")
	}
	for _, c := range []struct {
		name     string
		args     []string
		headless bool
	}{
		{"default", []string{"run", "", "--seed", "1", "--steps", "1", "--browser-check"}, true},
		{"headed", []string{"run", "", "--seed", "1", "--steps", "1", "--headed", "--browser-check"}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := browserOnlyProject(t)
			journal := filepath.Join(t.TempDir(), "journal")
			pathDir := emptyPath(t)
			plant(t, pathDir, "zz-fake-browser")
			t.Setenv(browserDriverEnv, "zz-fake-browser")
			t.Setenv(webDriverEnv, os.Args[0])
			t.Setenv(fakedriver.ModeEnv, "ready")
			t.Setenv(fakedriver.JournalEnv, journal)

			args := append([]string{}, c.args...)
			args[1] = dir
			var out, errb bytes.Buffer
			if rc := run(args, bytes.NewReader(nil), &out, &errb); rc != 0 {
				t.Fatalf("`rumor %v` exited %d\n%s\n%s", args, rc, out.String(), errb.String())
			}
			// STDERR, for browser.go's reason: a `rumor run` hashes its stdout.
			if !strings.Contains(errb.String(), "DRIVABLE") {
				t.Fatalf("the run did not drive the pair, so this arm has no subject: %q", errb.String())
			}
			raw, err := os.ReadFile(journal)
			if err != nil {
				t.Fatalf("the driver recorded nothing: %v", err)
			}
			asked := strings.Contains(string(raw), "--headless=new")
			if asked != c.headless {
				t.Errorf("`rumor %v` asked for a headless browser = %t, want %t: %q",
					args, asked, c.headless, string(raw))
			}
		})
	}
}

// browserOnlyProject is the smallest project that selects the browser stage: one
// action with a flow, and no `api` stage, so the stage under test is the only
// one that runs and the walk needs no system.
func browserOnlyProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("platform.toml", `contract = "2"

[adapter]
command = ["sh", "-c", "exit 1"]

[[venire]]
role = "member"
count = 1

[stages]
browser = { enabled = true }

[weights.default]
publish_doc_ui = 1
`)
	write("bindings.toml", `[[action]]
name = "publish_doc_ui"
context = "browser"
requires = ["may_publish"]

  [[action.flow]]
  step = "navigate"
  to = "/docs/new"

  [[action.flow]]
  step = "click"
  target = "publish"
`)
	write("statute.rego", "package rumor.statute\n\ndefault may_publish := true\n")
	return dir
}

// TestABrowserThatComesUpAndHangsIsBoundedByTheStage holds probeBound. A
// readiness check cannot see this failure: the driver answers /status, so it is
// up, and then never answers a session -- which is what a browser missing a
// library or denied a writable HOME does. Without the stage's own bound the run
// would wait on the HTTP client's timeout, and with neither it would wait
// forever on a stage that exists to report in one line.
func TestABrowserThatComesUpAndHangsIsBoundedByTheStage(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake driver is this test binary")
	}
	journal := filepath.Join(t.TempDir(), "journal")
	t.Setenv(fakedriver.ModeEnv, "wedge")
	t.Setenv(fakedriver.JournalEnv, journal)

	began := time.Now()
	why, _ := driveOnce(context.Background(), os.Args[0], "zz-fake-browser", true, time.Second)
	took := time.Since(began)

	if why == "" {
		t.Fatalf("a driver that never answers a session was reported as drivable")
	}
	// THE PRECONDITION: the fake really did wedge, so this arm is about the
	// bound and not about a driver that failed to start for some other reason.
	raw, err := os.ReadFile(journal)
	if err != nil || !strings.Contains(string(raw), "wedged") {
		t.Fatalf("the fake never reached its wedge, so the bound was not what ended this: %v %q", err, string(raw))
	}
	// WELL INSIDE THE HTTP CLIENT'S OWN 30s TIMEOUT, which is the next bound
	// out: an arm that allowed 30s would pass with the stage's bound removed.
	if took > 10*time.Second {
		t.Fatalf("driveOnce took %s against a 1s bound, so the bound was not applied", took)
	}
}

// TestASessionTheDriverWillNotCloseIsSaidOutLoud: the pair drove, so it IS
// drivable -- and the delete was refused, so a browser is still running. Those
// are two verdicts and the line has to carry both, because this is the only
// place a reader could learn the second.
//
// IT IS ALSO WHY `call` READS THE HTTP STATUS. A DELETE is sent with nowhere to
// decode a body into, so before the status check a driver answering 500 to
// "close this session" was indistinguishable from one that closed it.
func TestASessionTheDriverWillNotCloseIsSaidOutLoud(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake driver is this test binary")
	}
	dir := emptyPath(t)
	plant(t, dir, "zz-fake-browser")
	journal := filepath.Join(t.TempDir(), "journal")
	t.Setenv(browserDriverEnv, "zz-fake-browser")
	t.Setenv(webDriverEnv, os.Args[0])
	t.Setenv(fakedriver.ModeEnv, "refuse-close")
	t.Setenv(fakedriver.JournalEnv, journal)

	var out, errb bytes.Buffer
	if rc := browserStage(context.Background(), flowBindings(1, 0), runOpts{browserCheck: true}, &out, &errb); rc != 0 {
		t.Fatalf("a drivable pair with a refused close exited %d, stderr %q", rc, errb.String())
	}
	got := errb.String()
	// STILL DRIVABLE: the pair started and took a session, and reporting it as
	// a skip would send an operator to install a package they already have.
	if !strings.Contains(got, "DRIVABLE") || strings.Contains(got, "NOT DRIVABLE") {
		t.Errorf("a pair that drove and then leaked was reported undrivable: %q", got)
	}
	if !strings.Contains(got, "NOT CLOSED") {
		t.Errorf("the refused delete is not in the line, so nothing tells a reader a browser is still running: %q", got)
	}
	if !strings.Contains(got, "would not close") {
		t.Errorf("the line does not carry the driver's own reason: %q", got)
	}
	// AND THE DELETE WAS REALLY ATTEMPTED, so this arm is about noticing a
	// refusal and not about a teardown that never ran.
	raw, err := os.ReadFile(journal)
	if err != nil || !strings.Contains(string(raw), "close sess-1") {
		t.Fatalf("the stage never asked for the delete: %v %q", err, string(raw))
	}
}

// TestAPlainRunLaunchesNoBrowser is the cost claim where CI actually pays it:
// at the command line, through the same dispatch `tests/run` drives 55 times.
// The stage-level arm above measures browserStage; this one measures what the
// BINARY does when nobody passes a flag, which is the number that turned a
// 21m17s job into a run killed at the 1800s budget.
func TestAPlainRunLaunchesNoBrowser(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake driver is this test binary")
	}
	dir := browserOnlyProject(t)
	journal := filepath.Join(t.TempDir(), "journal")
	pathDir := emptyPath(t)
	plant(t, pathDir, "zz-fake-browser")
	t.Setenv(browserDriverEnv, "zz-fake-browser")
	t.Setenv(webDriverEnv, os.Args[0])
	t.Setenv(fakedriver.ModeEnv, "ready")
	t.Setenv(fakedriver.JournalEnv, journal)

	var out, errb bytes.Buffer
	if rc := run([]string{"run", dir, "--seed", "1", "--steps", "1"}, bytes.NewReader(nil), &out, &errb); rc != 0 {
		t.Fatalf("a plain run exited %d\n%s\n%s", rc, out.String(), errb.String())
	}
	if !strings.Contains(out.String(), "the pair is COMPLETE") {
		t.Fatalf("no complete pair was found, so this arm has no subject: %q", out.String())
	}
	if _, err := os.Stat(journal); err == nil {
		raw, _ := os.ReadFile(journal)
		t.Fatalf("`rumor run` with no flag executed a driver; journal: %q", string(raw))
	} else if !os.IsNotExist(err) {
		t.Fatalf("could not tell whether the driver ran: %v", err)
	}
}

// TestTheProbesOutcomeNeverReachesTheHashedStream is the cell the channel move
// of r9/263 needed, and it drives D1 itself rather than a proxy for it.
//
// WHAT WENT WRONG WITHOUT IT (plumb, coop r9/260): one seed run into two
// directories differed in exactly one line, the browser stage's own probe --
// "FOUND, NOT DRIVABLE ... would not open a session" on a loaded box against
// "DRIVABLE" on a free one -- with the FOUND stanza and the snapshot line
// byte-identical. `rumor run` hashes its STDOUT, so D1's "two attempts of one
// seed are byte-identical" held for NO `--browser-check` run on a busy box.
// The repair first offered was an exclusion in the arm that noticed; that
// weakens the only check reading this stream to route around a line which, by
// design.md:1813-1823, does not belong in it.
//
// SO THE ARM ASSERTS THE PROPERTY AND NOT THE IMPLEMENTATION. Two runs of the
// same stage differing ONLY in an outcome the box decides -- a session the
// driver closes, and one it refuses to -- must leave stdout byte-identical and
// must say different things on stderr. An arm that merely grepped stdout for
// "DRIVABLE" would pass the day someone wrote the verdict to both streams.
//
// AND THE STDERR HALF IS WHAT STOPS IT PASSING VACUOUSLY: a stage that printed
// NOTHING anywhere would satisfy byte-identity perfectly.
func TestTheProbesOutcomeNeverReachesTheHashedStream(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture plants a shell script")
	}
	dir := emptyPath(t)
	plant(t, dir, "zz-fake-browser")
	t.Setenv(browserDriverEnv, "zz-fake-browser")
	t.Setenv(webDriverEnv, os.Args[0])

	probe := func(mode string) (string, string) {
		t.Helper()
		t.Setenv(fakedriver.ModeEnv, mode)
		t.Setenv(fakedriver.JournalEnv, filepath.Join(t.TempDir(), "journal"))
		var out, errb bytes.Buffer
		if rc := browserStage(context.Background(), flowBindings(1, 0), runOpts{browserCheck: true}, &out, &errb); rc != 0 {
			t.Fatalf("mode %q exited %d, stderr %q", mode, rc, errb.String())
		}
		return out.String(), errb.String()
	}

	closedOut, closedErr := probe("ready")
	leakedOut, leakedErr := probe("refuse-close")

	// THE TWO RUNS REALLY DID DIFFER, checked before the identity is believed:
	// without this the arm would pass against a fake whose modes do the same
	// thing, which is a control that is the case it controls for.
	if closedErr == leakedErr {
		t.Fatalf("both modes said the same thing on stderr, so this arm has no subject: %q", closedErr)
	}
	if !strings.Contains(leakedErr, "NOT CLOSED") {
		t.Fatalf("the refused close is not on stderr, so the outcome under test never happened: %q", leakedErr)
	}

	if closedOut != leakedOut {
		t.Errorf("stdout moved with an outcome the BOX decided, which is D1 broken for every "+
			"--browser-check run on a loaded box:\n closed: %q\n leaked: %q", closedOut, leakedOut)
	}
	// AND THE VERDICT ITSELF IS NOT THERE, which byte-identity alone would not
	// catch: two runs that both leaked would agree while both carrying it.
	for _, word := range []string{"DRIVABLE", "NOT CLOSED", "one session opened"} {
		if strings.Contains(closedOut, word) {
			t.Errorf("stdout carries the probe's outcome word %q: %q", word, closedOut)
		}
	}
	// THE DETERMINABLE HALF IS STILL SAID, so this is a channel move and not a
	// silent drop: a reader of the record alone is told the probe ran and where
	// its answer is.
	for _, want := range []string{"WAS PROBED", "on stderr"} {
		if !strings.Contains(closedOut, want) {
			t.Errorf("stdout does not say %q, so the record points nowhere: %q", want, closedOut)
		}
	}
	proof.Logf(t, "PROOF 2 --browser-check runs differing only in the box's own outcome, stdout byte-identical at %d bytes, stderr differing", len(closedOut))
}
