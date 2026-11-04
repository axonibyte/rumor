package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
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
	if _, how := browserDriver(); !strings.Contains(how, "none of") {
		t.Fatalf("PATH was emptied and a driver was still found (%s), so no arm below is about the stage", how)
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
	if rc := browserStage(flowBindings(2, 1), &out, &errb); rc != 0 {
		t.Errorf("a missing driver exited %d: deliverable 5 makes it a SKIP, and a refusal would "+
			"fail every run on every box without a browser", rc)
	}
	got := out.String()
	for _, want := range []string{"SKIPPED", "no driver", stage.Browser, "chromium", browserDriverEnv, "2 action(s)"} {
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
	_ = browserStage(flowBindings(1, 0), &out, &errb)
	if strings.TrimSpace(out.String()) == "" {
		t.Error("the browser stage skipped and printed nothing: a silent skip reads exactly like a " +
			"stage that ran and found nothing")
	}
}

func TestTheBrowserStageNamesTheDriverItFound(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake driver is a shell script")
	}
	dir := emptyPath(t)
	fake := filepath.Join(dir, "zz-fake-browser")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(browserDriverEnv, "zz-fake-browser")
	var out, errb bytes.Buffer
	if rc := browserStage(flowBindings(3, 2), &out, &errb); rc != 0 {
		t.Errorf("a driver was present and the stage exited %d", rc)
	}
	got := out.String()
	// "THIS STAGE drove none" REPLACED "none was driven yet", and the reason is
	// in browser.go: as of R9.1's H2 the WALK drives these flows through the
	// adapter, so a line saying none was driven reads as a contradiction beside
	// a summary that counts the ones that were. The arm asserts the new words
	// because the old ones would now be a false reading of a true count.
	for _, want := range []string{"driver", fake, browserDriverEnv, "3 action(s)", "THIS STAGE drove none"} {
		if !strings.Contains(got, want) {
			t.Errorf("the present-driver line does not name %q: %q", want, got)
		}
	}
	if strings.Contains(got, "SKIPPED") {
		t.Errorf("a driver was found and the stage still reported a skip: %q", got)
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
	if len(browserCandidates) == 0 {
		t.Fatal("browserCandidates is empty, so this arm asserts nothing and the scan can never " +
			"find a driver")
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
	want := []string{"chrome", "chromium", "chromium-browser", "firefox", "google-chrome", "headless_shell"}
	if !slices.Equal(slices.Sorted(slices.Values(browserCandidates)), slices.Sorted(slices.Values(want))) {
		t.Errorf("browserCandidates is %v and this arm expects %v: a name added here needs a line "+
			"in the other list, and a name that differs by a letter is a browser this project "+
			"cannot find on a box that has it", browserCandidates, want)
	}

	for _, name := range browserCandidates {
		t.Run(name, func(t *testing.T) {
			dir := emptyPath(t)
			planted := filepath.Join(dir, name)
			if err := os.WriteFile(planted, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			// RUMOR_BROWSER STAYS EMPTY, which emptyPath has already set: this
			// arm is about the scan and not about the named-command branch.
			path, how := browserDriver()
			if path != planted {
				t.Errorf("a %q on PATH resolved to %q, want %q: the scan did not find a binary "+
					"under a name it carries", name, path, planted)
			}
			if !strings.Contains(how, name) || !strings.Contains(how, "on PATH") {
				t.Errorf("the how is %q and does not say that %q was found on PATH, which is what "+
					"an operator reads when the answer is a surprise", how, name)
			}
		})
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
	candidate := filepath.Join(dir, browserCandidates[0])
	named := filepath.Join(dir, "zz-named-browser")
	for _, f := range []string{candidate, named} {
		if err := os.WriteFile(f, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// THE PRECONDITION FIRST: with the variable unset the candidate is what the
	// scan finds, so the switch below is about precedence and not about the
	// candidate being unfindable.
	if path, _ := browserDriver(); path != candidate {
		t.Fatalf("with %s unset the scan found %q, want the candidate %q", browserDriverEnv, path, candidate)
	}

	t.Setenv(browserDriverEnv, "zz-named-browser")
	path, how := browserDriver()
	if path != named {
		t.Errorf("%s names a driver and the scan's candidate won anyway (%q): an operator naming a "+
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
	if rc := browserStage(flowBindings(1, 0), &out, &errb); rc != 0 {
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
	if rc := browserStage(flowBindings(2, 1), &out, &errb); rc != 0 {
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
