// Package browser drives a real browser over W3C WebDriver, which is the half
// R9.1b adds behind the `flow` seam v0.1.48 shipped with a fake.
//
// WHY WebDriver AND NOT CDP (fable's ruling at coop r9/73, on the measurement at
// r9/71): the protocol is spoken by a DRIVER PROCESS beside the browser --
// chromedriver for chromium and its kin, geckodriver for firefox -- so one code
// path reaches both families, where CDP reaches chromium alone and would make
// `firefox` a name the scan selects and cannot speak to.
//
// WHAT THIS PACKAGE DOES AND WHAT IT DOES NOT. It starts the driver, opens one
// SESSION PER ACTOR, and stops both. It executes no flow step: the four step
// kinds are issue 122's deliverable 2 and a head of their own, so nothing here
// knows what `navigate` or `click` mean. A package that did both would make the
// lifecycle untestable without a page to drive.
//
// ONE SESSION PER ACTOR IS THE ISOLATION, and it is the protocol's rather than
// ours: a W3C session carries its own cookie jar and storage, so two actors in
// two sessions cannot see each other's logins without us arranging anything.
// The arm proves that against a real browser rather than trusting the sentence.
package browser

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"syscall"
	"time"
)

// StopGrace is how long a driver gets to exit after its signal before it is
// killed, matching internal/wire's own grace for an adapter: a process that
// cleans up after itself is given the chance to, and one that will not cannot
// hold a run open.
const StopGrace = 3 * time.Second

// ReadyTimeout bounds the wait for the driver's own `/status`, and it is a
// BOUND rather than a sleep: the loop asks and stops asking, so a driver that
// never answers is an error naming the wait and not a hung run.
//
// SET FROM TWO MEASURED DISTRIBUTIONS, uncensored, under fable's method at coop
// r9/103. Every run's answer-time was recorded with a 120s ceiling so nothing
// was cut off at a threshold:
//
//	sll-driver-01 (rook, r9/104)  N=20  hw.ncpu 4  load 1.21
//	    min 25.8   median 63.9   p90 65.1   MAX 68.3 ms
//	matrix        (ibis, r9/105)  N=20  hw.ncpu 8  load 2.31
//	    min 21.1   median 21.5   p95 31.8   MAX 32.7 ms
//
// 0 censored in 40 runs, and the combined observed tail is 68.3ms. 10s is ~146x
// that, which leaves room for a cold start and for CI contention that NEITHER
// distribution sampled.
//
// THE BOUND WENT DOWN, NOT UP, AND MY FIRST TWO READINGS WERE BOTH WRONG. I set
// 20s against a fake that answers in microseconds. rook's 1-in-6 failure then
// made me raise it to 60s, which was a margin I chose and called measured
// (r9/101, corrected at r9/102). Their distribution settles it: nothing takes
// between 68ms and 20s, so that failure was never a SLOW driver and no amount of
// waiting could have turned it into a pass -- 60s would only have delayed the
// same message by 40 seconds.
//
// WHICH MAKES THE BOUND A DISCRIMINATOR AND NOT A BUDGET. A driver that has
// exited is answered at once below. A driver that is alive and answering
// `ready:false` cannot be told from a slow one by waiting -- but healthy
// readiness is TENS OF MILLISECONDS on both boxes, so one that is still not
// ready after 10s is stuck, and saying so quickly is the whole value of the
// number.
const ReadyTimeout = 10 * time.Second

// RequestBound is how long ONE WebDriver call may take, and it is named because
// a failure has to be able to say which budget it was.
//
// THREE BOUNDS CAN END A CALL and they are not the same fault: the FLOW's budget
// (the caller's ctx deadline, which waitBudget reads as the step's), this one,
// and ReadyTimeout at start. A reader told only "context deadline exceeded"
// cannot tell which to raise, or whether the run was simply cancelled -- which
// is the gap r9/154's contended red left: an arm's own 2s expired on a POST to
// /element and the error named no budget at all.
//
// IT IS LONGER THAN ANY STEP SHOULD TAKE AND SHORTER THAN THE WIRE'S OWN
// REQUEST BOUND (wire.DefaultRequestTimeout, 60s), so a driver that stops
// answering is this package's finding rather than the engine's timeout.
const RequestBound = 30 * time.Second

// Driver is a running WebDriver process and the sessions opened through it.
type Driver struct {
	base     string
	path     string
	browser  string
	cmd      *exec.Cmd
	http     *http.Client
	headless bool

	// exited is closed when the driver process has been reaped, and it is the
	// ONLY place cmd.Wait is called. Two Waits on one Cmd is a bug -- the
	// second answers an error and the first's result is lost -- so the reaper
	// is started once beside the process and both the readiness wait and the
	// teardown read this channel instead of waiting again.
	exited chan struct{}

	// sessions maps an ACTOR to its session id, which is the whole of "one
	// context per actor": the key is the actor, so asking twice answers the
	// same session and two actors can never be handed one.
	sessions map[string]string

	// worlds remembers, per system address, whether anything answers there --
	// nil for proved, an error for the refusal to repeat.
	//
	// IT EXISTS BECAUSE A NAVIGATION TO NOTHING SUCCEEDS. Measured: with the
	// adapter reporting `http://127.0.0.1:1`, chromedriver answered the
	// navigate 200 and loaded CHROME'S OWN ERROR PAGE, so the click that
	// followed found no element and the walk recorded "the page would not
	// perform it" -- a REFUSAL attributed to a system that was never reached.
	// W3C WebDriver carries no HTTP status for a navigation, so the browser
	// cannot be asked; the address can.
	//
	// CACHED BECAUSE IT IS ONE FACT PER RUN, not one per step: a walk drives
	// many flows against one base and a dial per navigate would be a cost with
	// no second answer. A world that dies mid-run fails at the browser instead,
	// which is a true report of a different thing.
	worlds map[string]error
}

// whyCallEnded names WHICH budget ended a call, because the transport's own
// error does not.
//
// MEASURED, AND IT COST A RED NOBODY COULD READ: r9/154's contended red was
// `TestTheActorsTokenReachesNoOutput` failing at load 6.8 to 9.2 with a POST to
// .../element that "hit its deadline" -- and the arm could not tell, nor could
// its reader, whether the FLOW's budget had run out, the driver had stopped
// answering, or the run had been cancelled. Those three want different answers:
// raise the flow's budget, look at the driver, or nothing at all.
//
// THE CALLER'S DEADLINE IS THE FLOW'S BUDGET, which is waitBudget's reading of
// fable's ruling (coop r9/110) rather than a second meaning invented here: the
// ctx a flow is driven with IS the step's budget, so a call that ends on it has
// spent exactly that.
//
// A CANCELLED RUN IS NOT A BUDGET AT ALL and says so: a reader chasing a
// timeout when the run was stopped is looking for a fault that never happened.
func (d *Driver) whyCallEnded(ctx context.Context, method, path string, err error) error {
	// THE ERROR AND THE CONTEXT ARE READ TOGETHER, AND READING THE CONTEXT ALONE
	// WAS THIS FUNCTION'S OWN DEFECT. rook, coop rumor/1973, drove a
	// "connection reset by peer" that arrived while the flow's budget happened to
	// have expired, and got back "ran out of THE FLOW'S BUDGET ... the driver's
	// own bound of 30s and this run's readiness bound of 10s are untouched" -- a
	// diagnosis that clears precisely the thing that had failed. The `%w` tail
	// kept the real error, so nothing was LOST; the sentence in front of it was
	// wrong, which is worse than saying nothing.
	//
	// AND KEYING ON THE ERROR ALONE CANNOT WORK EITHER, which is rook's
	// refutation of their own first fix (coop r9/160, go1.26) and re-measured
	// here on go1.27.2 against a wedged server --
	// TestTheTOOLCHAINCannotTellTheBoundsApartByErrorAlone pins it:
	//
	//	caller ctx deadline       errors.Is(DeadlineExceeded) TRUE   os.IsTimeout TRUE
	//	client Timeout, ctx live  errors.Is(DeadlineExceeded) TRUE   os.IsTimeout TRUE
	//	caller cancelled          errors.Is(Canceled)         TRUE   os.IsTimeout FALSE
	//
	// Go wraps the client's OWN timeout as context.DeadlineExceeded as well, and
	// os.IsTimeout holds for both, so NO predicate over the error can say whose
	// bound ended the call. `ctx.Err()` is the only thing that can.
	//
	// SO THE TEST IS CONJUNCTIVE: the ERROR says whether a bound ended the call
	// at all, and the CONTEXT says WHOSE. Neither of them alone is a diagnosis,
	// and the grid in budget_test.go drives all three context states against all
	// four error shapes rather than moving one input at a time -- which is how
	// four cells passed over this.
	//
	// THE CALLER'S DEADLINE IS THE FLOW'S BUDGET, which is waitBudget's reading of
	// fable's ruling (coop r9/110) rather than a second meaning invented here: the
	// ctx a flow is driven with IS the step's budget, so a call that ends on it has
	// spent exactly that.
	bound := errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err)
	switch {
	case bound && errors.Is(ctx.Err(), context.DeadlineExceeded):
		over := ""
		if dl, ok := ctx.Deadline(); ok {
			over = fmt.Sprintf(", which was %s ago", time.Since(dl).Round(time.Millisecond))
		}
		return fmt.Errorf("%s %s ran out of THE FLOW'S BUDGET%s: the budget is the deadline this "+
			"flow was driven with, so this step had that long for every call it made -- the "+
			"driver's own bound of %s and this run's readiness bound of %s are untouched: %w",
			method, path, over, RequestBound, ReadyTimeout, err)
	case errors.Is(err, context.Canceled) && errors.Is(ctx.Err(), context.Canceled):
		return fmt.Errorf("%s %s stopped because THIS RUN WAS CANCELLED, which is no budget "+
			"expiring and names no fault in the driver: %w", method, path, err)
	case bound:
		return fmt.Errorf("%s %s ran out of THE DRIVER'S OWN REQUEST BOUND of %s while the flow's "+
			"budget still had time: the driver stopped answering one call, which is this "+
			"package's fault to report and not the step's: %w", method, path, RequestBound, err)
	default:
		// A FAILURE NO BOUND EXPLAINS, AND BOTH FACTS ARE NAMED WHEN BOTH ARE
		// TRUE. A transport failure alongside a context that is already done is
		// two true things, and neither is the other's cause: reporting only the
		// context clears the driver, and reporting only the error hides that the
		// step had no time left to try again either.
		if cerr := ctx.Err(); cerr != nil {
			return fmt.Errorf("%s %s failed on its own and NO BOUND EXPLAINS IT, though this run's "+
				"context was ALSO already done (%v): the two are both true and neither caused "+
				"the other, so the call is the thing to look at: %w", method, path, cerr, err)
		}
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
}

// freePort asks the kernel for a port and gives it back.
//
// THE RACE IS NAMED RATHER THAN HIDDEN: between the close here and the driver's
// own bind, another process could take it. Nothing in this package pretends
// otherwise -- what makes that safe is that readiness is PROVED below, so a
// stolen port is a named start error and never a driver we think we have.
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	if cerr := l.Close(); cerr != nil {
		return 0, cerr
	}
	return port, nil
}

// Start launches the driver and waits for it to answer, or says why not.
//
// THE BROWSER IS PASSED AND NOT GUESSED. The scan resolved a specific binary
// (cmd/rumor's pairs), and a driver left to find its own would be free to pick
// a different install than the one the skip line named -- so the path travels
// into the session's capabilities and the two cannot disagree.
// THERE IS NO BOUND PARAMETER, and that is deliberate after one measured
// mistake: I first made the readiness bound an argument so a cell could hold it
// cheaply, and the mutant that passed 1s from the stage SURVIVED -- nothing held
// the value production sends. A constant cannot be passed wrongly. The cells
// reach the bound through waitReady directly and through a driver that is SLOW
// to come up, which is the case that matters anyway.
func Start(ctx context.Context, driverPath, browserPath string, headless bool) (*Driver, error) {
	port, err := freePort()
	if err != nil {
		return nil, fmt.Errorf("no local port for the webdriver: %w", err)
	}
	cmd := exec.Command(driverPath, fmt.Sprintf("--port=%d", port))
	// ITS OWN PROCESS GROUP, SO THE WHOLE TREE CAN BE STOPPED. A webdriver
	// SPAWNS THE BROWSER, and signalling the driver alone does not take the
	// browser with it -- MEASURED 2026-11-08 while driving the flow arms: with
	// chromedriver reaped and gone, six `chrome:` processes were still alive,
	// reparented onto the test binary, and `go test` waited on them until it was
	// killed at 110s having passed in 0.8s. The same orphans explain 19 stray
	// processes found in this jail earlier the same night.
	//
	// SO "NOTHING IS LEFT RUNNING" NEEDED THIS TO BE TRUE. Closing the sessions
	// asks the browser to go and the driver acknowledges before it has gone, so
	// a signal to the driver can outrun the browser's own exit. A group makes
	// the guarantee structural instead of a race: one signal reaches the driver
	// and everything it started.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting %s: %w", driverPath, err)
	}
	d := &Driver{
		base:     fmt.Sprintf("http://127.0.0.1:%d", port),
		path:     driverPath,
		browser:  browserPath,
		cmd:      cmd,
		http:     &http.Client{Timeout: RequestBound},
		headless: headless,
		sessions: map[string]string{}, worlds: map[string]error{},
		exited: make(chan struct{}),
	}
	go func() {
		_ = cmd.Wait()
		close(d.exited)
	}()
	if err := d.waitReady(ctx, ReadyTimeout); err != nil {
		d.stopProcess()
		return nil, err
	}
	return d, nil
}

// waitReady polls the driver's own readiness endpoint until it says ready.
//
// A DRIVER THAT HAS EXITED IS ANSWERED AT ONCE AND NOT AT THE BOUND. The bound
// is for a driver that is slow to come up; a process that is already gone will
// never answer, and spending ReadyTimeout finding that out would turn the
// commonest failure on a misconfigured box -- a `driver` on PATH that is not a
// webdriver at all -- into a 20-second stall per run. The reason is also more
// precise: "it exited" and "it never answered" are different faults.
func (d *Driver) waitReady(ctx context.Context, bound time.Duration) error {
	deadline := time.Now().Add(bound)
	var last error
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		select {
		case <-d.exited:
			return fmt.Errorf("%s exited before it answered /status, so it is not a webdriver "+
				"this build can drive (last: %v)", d.path, last)
		default:
		}
		var out struct {
			Value struct {
				Ready bool `json:"ready"`
			} `json:"value"`
		}
		if err := d.call(ctx, http.MethodGet, "/status", nil, &out); err != nil {
			last = err
		} else if out.Value.Ready {
			return nil
		} else {
			last = fmt.Errorf("the driver answered /status and is not ready")
		}
		time.Sleep(50 * time.Millisecond)
	}
	// THE MESSAGE NAMES STUCK, NOT SLOW. Measured readiness is tens of
	// milliseconds on both boxes this phase has (see ReadyTimeout), so a driver
	// that answered and never readied inside the bound is not behind -- it is
	// not coming. "within 10s" alone invites the reader to grant it more time,
	// which is the reading that cost rook a 1-in-6 failure misfiled as slowness.
	return fmt.Errorf("%s answered /status but never became ready within %s, and measured "+
		"readiness is tens of milliseconds on this phase's boxes -- so it is stuck rather than "+
		"slow, and waiting longer would not help: %w (a port taken between being chosen and "+
		"being bound reads the same way)", d.path, bound, last)
}

// capabilities is what a new session asks for.
//
// `--no-sandbox` IS ABSENT AND THAT IS DELIBERATE (issue 122's deliverable 1,
// on the measurement at coop rumor/1920): headless chromium runs with its
// sandbox ON as an ordinary user given a writable HOME, and the flag is needed
// only when running as root. A CI step must never carry it, so the driver does
// not put it there for them.
func (d *Driver) capabilities() map[string]any {
	opts := map[string]any{}
	if d.browser != "" {
		opts["binary"] = d.browser
	}
	args := []string{}
	if d.headless {
		args = append(args, "--headless=new")
	}
	opts["args"] = args
	return map[string]any{
		"capabilities": map[string]any{
			"alwaysMatch": map[string]any{
				"goog:chromeOptions":  opts,
				"moz:firefoxOptions":  map[string]any{"args": args},
				"acceptInsecureCerts": false,
			},
		},
	}
}

// SessionFor is the actor's own session, opened on first ask.
//
// ASKED TWICE IT ANSWERS ONCE. An actor acts many times in a walk, and a
// session per ACT rather than per ACTOR would throw away the cookies that make
// the actor one person.
func (d *Driver) SessionFor(ctx context.Context, actor string) (string, error) {
	if actor == "" {
		return "", fmt.Errorf("a browser session belongs to an actor and none was named")
	}
	if id, ok := d.sessions[actor]; ok {
		return id, nil
	}
	var out struct {
		Value struct {
			SessionID string `json:"sessionId"`
			Error     string `json:"error"`
			Message   string `json:"message"`
		} `json:"value"`
	}
	if err := d.call(ctx, http.MethodPost, "/session", d.capabilities(), &out); err != nil {
		return "", fmt.Errorf("opening a session for %q%s: %w", actor, d.whyNoSession(), err)
	}
	if out.Value.Error != "" {
		return "", fmt.Errorf("the driver refused a session for %q: %s: %s",
			actor, out.Value.Error, out.Value.Message)
	}
	if out.Value.SessionID == "" {
		return "", fmt.Errorf("the driver answered a session for %q with no sessionId", actor)
	}
	d.sessions[actor] = out.Value.SessionID
	return out.Value.SessionID, nil
}

// whyNoSession adds the one thing a driver's own answer cannot say: that this
// run asked for a HEADED browser on a box with no display to draw it on.
//
// MEASURED, which is why the clause exists at all: `--headed` here answers
// `POST /session` with a 500 and "Chrome instance exited. Examine ChromeDriver
// verbose log to determine the cause" -- true, unhelpful, and it sends its
// reader to a log the run did not keep. The run already knows the two facts
// that explain it.
//
// IT IS A CLAUSE AND NOT A REFUSAL BEFORE THE RUN, deliberately: a display can
// be reached in ways this check cannot enumerate -- an Xvfb started around the
// run, a nested compositor, an environment a wrapper sets -- so refusing up
// front on an absent DISPLAY would refuse runs that would have worked. The
// driver's own failure stays the verdict; this only says what it most likely
// means.
//
// AND IT SAYS NOTHING WHEN HEADLESS, because then the display is irrelevant and
// a reader chasing a mention of it would be chasing a red herring.
func (d *Driver) whyNoSession() string {
	if d.headless {
		return ""
	}
	if os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "" {
		return ""
	}
	return " (this run asked for a HEADED browser and neither DISPLAY nor " +
		"WAYLAND_DISPLAY is set, so there is nothing for it to draw on: a headed browser " +
		"needs a display, and headless is the default for exactly this reason)"
}

// Actors is who holds a session, sorted, so a caller reporting them is
// deterministic and a test can read them.
func (d *Driver) Actors() []string {
	out := make([]string, 0, len(d.sessions))
	for a := range d.sessions {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

// Close deletes every session this driver opened and then stops it.
//
// THE SESSIONS GO FIRST, and a failure to delete one does not stop the others
// or the process: a browser left running is the leak this method exists to
// prevent, so the teardown is best-effort per session and unconditional for the
// process. The errors are joined and returned so a caller can SAY a session
// would not close, which is a fact about the browser and not a reason to keep
// it alive.
func (d *Driver) Close(ctx context.Context) error {
	var failed []string
	for _, actor := range d.Actors() {
		id := d.sessions[actor]
		if err := d.call(ctx, http.MethodDelete, "/session/"+id, nil, nil); err != nil {
			failed = append(failed, fmt.Sprintf("%s (%s): %v", actor, id, err))
		}
		delete(d.sessions, actor)
	}
	d.stopProcess()
	if len(failed) > 0 {
		return fmt.Errorf("%d session(s) would not close: %s", len(failed), strings.Join(failed, "; "))
	}
	return nil
}

// stopProcess asks the driver to go, gives it a bounded grace, then kills it --
// internal/wire's stopAdapter shape (reap in a goroutine, select on the reap
// against a deadline, kill then collect), for its reason: a process that will
// not leave cannot hold a run open.
//
// THE ASK IS A SIGNAL AND NOT A CLOSED STDIN, which is where this departs from
// stopAdapter: an adapter reads its protocol on stdin and takes EOF as the word
// to stop, while a webdriver reads HTTP and would never notice. os.Interrupt is
// the portable ask -- chromedriver and geckodriver both stop on it -- and Kill
// is what makes the wait bounded whether or not it was heard.
func (d *Driver) stopProcess() {
	if d.cmd == nil || d.cmd.Process == nil {
		return
	}
	// THE GROUP AND NOT THE PROCESS: a negative pid signals the whole group, so
	// the browser the driver spawned goes with it. The driver's own pid is the
	// group id because Start asked for Setpgid.
	pid := d.cmd.Process.Pid
	signalGroup(pid, syscall.SIGTERM)
	select {
	case <-d.exited:
	case <-time.After(StopGrace):
		signalGroup(pid, syscall.SIGKILL)
		<-d.exited
	}
	// AND AGAIN AFTER THE REAP, because the driver exiting is not the browser
	// exiting: the first signal is the polite one and a browser mid-teardown can
	// outlive it. A signal to a group with no members left is harmless, which is
	// what makes this cheap rather than conditional.
	signalGroup(pid, syscall.SIGKILL)
	d.cmd = nil
}

// signalGroup signals a whole process group and says nothing about the result:
// the group may already be empty, which is the success case and not an error.
func signalGroup(pid int, sig syscall.Signal) {
	_ = syscall.Kill(-pid, sig)
}

// call is the one place a WebDriver request is made, so the base, the timeout
// and the decoding cannot come apart between callers.
func (d *Driver) call(ctx context.Context, method, path string, body any, out any) error {
	var rdr *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(raw)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, d.base+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.http.Do(req)
	if err != nil {
		return d.whyCallEnded(ctx, method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	// THE STATUS IS READ, AND IT WAS NOT. A DELETE is sent with nowhere to
	// decode a body into, so without this a driver answering 500 to "close
	// this session" was indistinguishable from one that closed it -- which is
	// a browser left running and a teardown that reported success. W3C errors
	// also carry a body, but only a request with somewhere to put it could see
	// that, so the status is the check that covers every call here.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%s %s answered %s: %s", method, path, resp.Status, strings.TrimSpace(string(snippet)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
