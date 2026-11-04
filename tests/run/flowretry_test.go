package run_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// THE RETRY PATH DRIVEN BOTH WAYS, which is issue 120's Tests section by name:
// "the retry-then-`ui_timeout` path driven both ways, with a mutant that skips
// silently killed".
//
// THE PROJECT IS examples/twin AND NOT examples/toy, and the reason is measured
// rather than preferred: the twin's adapter is `rumor fake-adapter`, which
// serves `flow` and can be told to stop one short with `--stop-flow`, while
// `cmd/toy-adapter` serves the toy's `join_event_ui` as an EXECUTE and refuses
// the `flow` kind by name. fable's reachable done-when for v0.1.48 is "a walk
// under a committed seed reaching the `flow` seam through the fake", and this is
// that walk.
//
// SEED 7 IS COMMITTED HERE because it DRAWS the browser action: measured, it
// takes `publish_doc_ui` at steps 3 and 4 of six. A seed that never drew it
// would make every assertion below vacuous, which is why the control at the top
// of each case asserts the draw happened before reading anything else.
const flowSeed = "7"

// twinWithStopFlow copies the twin and points its adapter at a fake that stops
// one action's flow short. The spec is the adapter's own flag, so nothing here
// re-implements the stop.
func twinWithStopFlow(t *testing.T, spec string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "twin")
	src := filepath.Join("..", "..", "examples", "twin")
	if out, err := exec.Command("cp", "-r", src, dir).CombinedOutput(); err != nil {
		t.Fatalf("copying the twin: %v %s", err, out)
	}
	path := filepath.Join(dir, "platform.toml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// THE EXISTING FLAGS ARE KEPT, not replaced: the twin names its own surface
	// and arms 9a/9b/9c pair the project's `[uses]` against THAT adapter's
	// answer, so a copy that dropped `--surface` would be refused for a reason
	// that has nothing to do with the flow.
	const was = `command = ["rumor", "fake-adapter", "--surface", "surface.json"]`
	if !strings.Contains(string(raw), was) {
		t.Fatalf("%s does not declare %s, so this arm cannot reach the fake's flags", path, was)
	}
	with := `command = ["rumor", "fake-adapter", "--surface", "surface.json", "--stop-flow", "` + spec + `"]`
	if err := os.WriteFile(path, []byte(strings.Replace(string(raw), was, with, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// A FLOW THAT STOPS ONCE AND THEN GOES THROUGH IS NOT A FINDING, which is the
// first of the two directions: the engine seals, advances and sends the flow
// again from where it reached, and a page that was merely slow must not be
// recorded as a broken one.
func TestAFlowThatStopsOnceResumesAndIsNotAFinding(t *testing.T) {
	dir := twinWithStopFlow(t, "publish_doc_ui:1:wait:1")
	out, errb, code := run(t, "run", dir, "--seed", flowSeed, "--steps", "6")

	// THE CONTROL: the browser action was DRAWN. Without it a walk that never
	// reached the flow would satisfy "no ui_timeout" for the wrong reason.
	if !strings.Contains(out, "publish_doc_ui") {
		t.Fatalf("seed %s did not draw the browser action, so this arm is vacuous\n%s", flowSeed, out)
	}
	if strings.Contains(out, "ui_timeout") {
		t.Errorf("a flow that stopped ONCE produced a ui_timeout, which is the second failure's "+
			"finding and not the first's\nstdout:\n%s", out)
	}

	// THE EXIT CODE IS NOT THE ORACLE HERE, and measuring it was my first cut's
	// mistake: this seed on the UNTOUCHED twin already finds a `bug` at step 5
	// and exits 3, so "not 3" would have been a claim about the twin's own
	// planted defect rather than about the retry. The oracle is that the
	// stop-once run reaches the SAME verdict as the untouched one: a retry that
	// worked changes nothing a reader sees but the steps it took.
	plain, _, plainCode := run(t, "run", filepath.Join("..", "..", "examples", "twin"),
		"--seed", flowSeed, "--steps", "6")
	if code != plainCode {
		t.Errorf("the stop-once run exits %d and the untouched twin exits %d on the same seed: a "+
			"retry that worked must not change the verdict\nstop-once:\n%s\nuntouched:\n%s",
			code, plainCode, out, plain)
	}
	if got, want := foundLine(out), foundLine(plain); got != want {
		t.Errorf("the stop-once run found %q and the untouched twin found %q", got, want)
	}
	if errb != "" && strings.Contains(errb, "ui_timeout") {
		t.Errorf("stderr names a ui_timeout on a retry that worked: %s", errb)
	}
}

// foundLine is the run's verdict line, or "" when it found nothing. It exists so
// two runs are compared on the FINDING rather than on their whole output, which
// differs by construction: one of them drove a flow twice.
func foundLine(out string) string {
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "run: FOUND") {
			return l
		}
	}
	return ""
}

// AND A FLOW THAT STOPS EVERY TIME IS THE FINDING, which is the second
// direction. `--stop-flow` with no TIMES stops every call, so the re-check after
// the seal fails at the same position -- the second failure of the same wait.
func TestAFlowThatNeverCompletesIsAUITimeout(t *testing.T) {
	dir := twinWithStopFlow(t, "publish_doc_ui:1:wait")
	out, errb, code := run(t, "run", dir, "--seed", flowSeed, "--steps", "6")

	if !strings.Contains(out, "publish_doc_ui") {
		t.Fatalf("seed %s did not draw the browser action, so this arm is vacuous\n%s", flowSeed, out)
	}
	if !strings.Contains(out, "ui_timeout") {
		t.Errorf("a flow that never completes produced no ui_timeout: a wait that never came "+
			"true must not be silently skipped (issue 120, deliverable 2)\nstdout:\n%s\nstderr:\n%s",
			out, errb)
	}
	if code != 3 {
		t.Errorf("exit %d, want 3 -- where a finding's is (docs/vocabulary.md v7)\nstderr:\n%s", code, errb)
	}
	// AND THE WALK ENDED THERE, which is the decision `stopsTheWalk` carries:
	// after a wait that never came true the page is in a state the record
	// cannot describe. A run that carried on would print steps past the finding.
	if !strings.Contains(out, "FOUND") {
		t.Errorf("the finding is not announced on the FOUND line, so a reader of stdout alone "+
			"cannot see it\n%s", out)
	}
}

// THE TRAP'S DYNAMIC WITNESS (fable, coop r9/40 part four, amended at r9/43).
//
// WHAT IT EXISTS TO CATCH, which neither of the other two instruments can see:
// examples/toy declares `join_event_ui` with a browser context and a UI flow, so
// as of R9.1's H2 the walk routes it through `flow` -- and cmd/toy-adapter
// served that action as an EXECUTE and refused the `flow` kind by name. Any walk
// that DREW it died with "flow is a kind this adapter does not serve". That is a
// latent trap and not a visible break: rook's call-site check passes a kind
// whose dispatch has no case, and the mutants do not send one.
//
// IT RUNS UNDER A PROFILE AND NOT AT A HUNTED SEED, which is the ruling's
// correction and rook's reason: a seed is a fact about today's weights. Thirty
// unprofiled seeds drew the action NONE of the time, because every unprofiled
// walk halts on a finding at step 7 to 12 and the action competes with that halt
// rather than only with its weight -- a bigger --steps cannot help.
// `examples/toy`'s `clicking` profile is committed for this: it appends the one
// role that holds the browser context and weights the three-link chain the draw
// needs, with the four kinds producing the early finding at zero.
//
// AND IT ASSERTS THE DRAW RATHER THAN ASSUMING IT, so an undrawn walk FAILS here
// instead of passing on nothing. The seed is named because the arm needs one,
// not because it does the work: if a weight moves and the draw stops happening,
// this arm says so.
func TestTheToyWalkDrawsItsBrowserActionAndGoesThroughFlow(t *testing.T) {
	// THE ADAPTER'S OWN RECORD, asked for by env so it is off for every other
	// run. `t.Setenv` reaches the child because `run` inherits this process's
	// environment.
	record := filepath.Join(t.TempDir(), "asked")
	t.Setenv("TOY_ADAPTER_RECORD", record)

	out, errb, code := run(t, "run", filepath.Join("..", "..", "examples", "toy"),
		"--profile", "clicking", "--seed", "3", "--steps", "30")

	// COULD-NOT-RUN FIRST, and by its own words: this is the shape the trap took,
	// and reading it before anything else makes the failure say "the adapter
	// refused the kind" rather than "a count was 0" several inferences later.
	if strings.Contains(errb, "does not serve") || strings.Contains(out, "does not serve") {
		t.Fatalf("the adapter refused a kind the walk sent:\nstderr:\n%s\nstdout:\n%s", errb, out)
	}
	if code == 2 {
		t.Fatalf("could-not-run, exit 2:\nstderr:\n%s", errb)
	}

	// THE DRAW, asserted on the step line rather than on the action's name
	// anywhere in the output -- a `suborn` names the action too, and a suborn
	// never executes, so it sends no flow at all. That false positive is exactly
	// what my first sweep for this seed reported.
	drawn := regexp.MustCompile(`(?m)^run: +\d+ \S+ join_event_ui\b`)
	if !drawn.MatchString(out) {
		t.Fatalf("the walk did not DRAW join_event_ui under the clicking profile, so this arm "+
			"asserted nothing: a weight or the action set has moved and the profile needs "+
			"re-measuring\n%s", out)
	}

	// AND THAT IT WENT THROUGH `flow`, which the run now says in its own words.
	// Before this clause existed, a flow that completed first time was
	// indistinguishable in this output from an execute.
	through := regexp.MustCompile(`(\d+) step\(s\) through ` + "`flow`")
	m := through.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("the summary does not report any step through `flow`, so the action was carried "+
			"by `execute` -- which is the routing not happening\n%s", out)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n < 1 {
		t.Errorf("the summary reports %q step(s) through `flow`", m[1])
	}

	// AND THE FAR SIDE OF THE WIRE SAYS SO TOO, which is the half the summary
	// cannot carry (fable, coop r9/57 item two, on rook's M1+M2 at rumor/1896).
	// The summary reads a field, and that field can be set from the action's
	// CONTEXT -- a two-line change that leaves it true with the routing OFF, so
	// an arm reading only this run's output is fooled by it. The ADAPTER either
	// decoded a `flow` or it did not.
	//
	// THE KIND IS THE ONLY DISCRIMINATOR OUT THERE: both operations post to the
	// toy's same UI route by design, so a request log of paths would show the
	// identical line either way.
	kinds, rerr := os.ReadFile(record)
	if rerr != nil {
		t.Fatalf("the adapter wrote no request record at %s: %v", record, rerr)
	}
	got := string(kinds)
	if !strings.Contains(got, "flow join_event_ui") {
		t.Errorf("the adapter never decoded a `flow` for join_event_ui, so nothing crossed the "+
			"wire as one however the summary reads\nrecord:\n%s", got)
	}
	// THE CONTROL, so the assertion above is not satisfied by an empty file or a
	// record of everything: the same walk must also have sent EXECUTEs, which is
	// what every api-context action is.
	if !strings.Contains(got, "execute ") {
		t.Errorf("the record holds no execute at all, so it is not a record of this walk\n%s", got)
	}
}

// A LATER WAIT IS PROGRESS AND NOT THE FINDING, which is the third direction and
// the one `--stop-flow`'s fifth field exists for: the re-check gets FURTHER and
// stops somewhere else, so the wait that failed first DID come true. Recording
// `ui_timeout` for it would name a condition that was met.
//
// THE POSITION IS WHAT THE ENGINE COMPARES, not the word: both answers say
// `wait`, and only the index tells a repeat from a step forward. A build that
// compared the word alone would call this a timeout.
func TestAFlowStoppingAtALaterWaitIsNotATimeout(t *testing.T) {
	dir := twinWithStopFlow(t, "publish_doc_ui:1:wait:1:2")
	out, errb, code := run(t, "run", dir, "--seed", flowSeed, "--steps", "6")

	if !strings.Contains(out, "publish_doc_ui") {
		t.Fatalf("seed %s did not draw the browser action, so this arm is vacuous\n%s", flowSeed, out)
	}
	if strings.Contains(out, "ui_timeout") || strings.Contains(errb, "ui_timeout") {
		t.Errorf("a flow that stopped at a LATER wait produced a ui_timeout: the first wait came "+
			"true, so naming it would name a condition that was met\nstdout:\n%s", out)
	}
	plain, _, plainCode := run(t, "run", filepath.Join("..", "..", "examples", "twin"),
		"--seed", flowSeed, "--steps", "6")
	if code != plainCode {
		t.Errorf("the later-wait run exits %d and the untouched twin exits %d on the same seed",
			code, plainCode)
	}
	if got, want := foundLine(out), foundLine(plain); got != want {
		t.Errorf("the later-wait run found %q and the untouched twin found %q", got, want)
	}
}

// AND A RUN THAT DRIVES NO FLOW SAYS SO, rather than leaving the clause out.
//
// THIS ARM WAS THE OPPOSITE FOR ONE DRAFT. It asserted the clause was ABSENT at
// zero, which I chose to hold the toy's three control digests still -- and rook
// named that as the shape this tree already refuses three files away (coop
// r9/50): `TestTheReportSaysNoneRatherThanNothing` exists because "a skip list
// omitted when there is nothing to say cannot be told from a build that never
// learned to report one, which is the shape that let `fuzz` ship known,
// unrunnable and declared by nobody". `0 step(s) through flow` omitted reads
// identically to a build that never counted them, and the digests nothing pins
// were the only thing bought by hiding it.
//
// THE PROJECT IS examples/toy UNPROFILED, measured: its browser action is
// drawable only under the `clicking` profile, so an unprofiled walk drives none
// and the number it prints has to be a zero rather than a silence.
func TestARunThatDrivesNoFlowSaysSoRatherThanOmittingIt(t *testing.T) {
	out, _, _ := run(t, "run", filepath.Join("..", "..", "examples", "toy"),
		"--seed", "2", "--steps", "4")
	if !strings.Contains(out, "0 step(s) through `flow`") {
		t.Errorf("an unprofiled toy run does not say it drove none, so a reader cannot tell a "+
			"zero from a build that never counted them\n%s", out)
	}
	// AND THE CONTROL: the summary line is there to be read, so an arm that
	// passed because the whole run failed would prove nothing.
	if !strings.Contains(out, "run: toy -- seed 2") {
		t.Errorf("no summary line, so the assertion above says nothing\n%s", out)
	}
}
