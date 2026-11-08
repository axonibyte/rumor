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
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
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
const ReadyTimeout = 20 * time.Second

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
func Start(ctx context.Context, driverPath, browserPath string, headless bool) (*Driver, error) {
	port, err := freePort()
	if err != nil {
		return nil, fmt.Errorf("no local port for the webdriver: %w", err)
	}
	cmd := exec.Command(driverPath, fmt.Sprintf("--port=%d", port))
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting %s: %w", driverPath, err)
	}
	d := &Driver{
		base:     fmt.Sprintf("http://127.0.0.1:%d", port),
		path:     driverPath,
		browser:  browserPath,
		cmd:      cmd,
		http:     &http.Client{Timeout: 30 * time.Second},
		headless: headless,
		sessions: map[string]string{},
		exited:   make(chan struct{}),
	}
	go func() {
		_ = cmd.Wait()
		close(d.exited)
	}()
	if err := d.waitReady(ctx); err != nil {
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
func (d *Driver) waitReady(ctx context.Context) error {
	deadline := time.Now().Add(ReadyTimeout)
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
	return fmt.Errorf("%s did not answer /status as ready within %s: %w -- the port may have been "+
		"taken between being chosen and being bound", d.path, ReadyTimeout, last)
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
		return "", fmt.Errorf("opening a session for %q: %w", actor, err)
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
	_ = d.cmd.Process.Signal(os.Interrupt)
	select {
	case <-d.exited:
	case <-time.After(StopGrace):
		_ = d.cmd.Process.Kill()
		<-d.exited
	}
	d.cmd = nil
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
		return err
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
