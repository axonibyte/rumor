package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
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
	for _, want := range []string{"driver", fake, browserDriverEnv, "3 action(s)", "none was driven yet"} {
		if !strings.Contains(got, want) {
			t.Errorf("the present-driver line does not name %q: %q", want, got)
		}
	}
	if strings.Contains(got, "SKIPPED") {
		t.Errorf("a driver was found and the stage still reported a skip: %q", got)
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
