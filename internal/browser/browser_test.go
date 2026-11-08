package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"forge.axonibyte.com/axonibyte/rumor/internal/browser/fakedriver"
)

// THE FAKE DRIVER IS THIS TEST BINARY RE-EXECUTED, which is os/exec's own
// pattern and is here for a reason this package cannot do without: a jail with
// no chromium still has to be able to drive every arm below. A fake that was a
// function call instead would leave exec, the port, the readiness poll and the
// teardown -- four of the five things this package IS -- unmeasured.
//
// What the fake is NOT is evidence that a real chromedriver answers the same
// way. That is the browser-side arm, and it runs in CI where the real pair
// lives; the two are named apart so neither is read as the other.
//
// The server itself lives in internal/browser/fakedriver because cmd/rumor's
// stage arms need the same one, and a protocol fake kept in two places drifts.
func TestMain(m *testing.M) {
	if fakedriver.MainIfAsked() {
		return
	}
	os.Exit(m.Run())
}

// fakeDriver points Start at this test binary and returns the journal to read.
func fakeDriver(t *testing.T, mode string) (driverPath, journal string) {
	t.Helper()
	journal = filepath.Join(t.TempDir(), "journal")
	t.Setenv(fakedriver.ModeEnv, mode)
	t.Setenv(fakedriver.JournalEnv, journal)
	return os.Args[0], journal
}

func journalLines(t *testing.T, journal string) []string {
	t.Helper()
	raw, err := os.ReadFile(journal)
	if err != nil {
		t.Fatalf("reading the fake driver's journal: %v", err)
	}
	var out []string
	for _, l := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

func linesWith(lines []string, prefix string) []string {
	var out []string
	for _, l := range lines {
		if strings.HasPrefix(l, prefix) {
			out = append(out, l)
		}
	}
	return out
}

func startFake(t *testing.T, mode string, headless bool) (*Driver, string) {
	t.Helper()
	path, journal := fakeDriver(t, mode)
	d, err := Start(context.Background(), path, "/nonexistent/chromium", headless)
	if err != nil {
		t.Fatalf("Start against the fake driver: %v", err)
	}
	return d, journal
}

// TestEachActorGetsItsOwnSession is the isolation claim at the only level this
// package can hold it: two actors, two session ids, two opens on the wire.
func TestEachActorGetsItsOwnSession(t *testing.T) {
	d, journal := startFake(t, "ready", true)
	defer func() { _ = d.Close(context.Background()) }()

	alice, err := d.SessionFor(context.Background(), "alice")
	if err != nil {
		t.Fatalf("session for alice: %v", err)
	}
	bob, err := d.SessionFor(context.Background(), "bob")
	if err != nil {
		t.Fatalf("session for bob: %v", err)
	}
	if alice == bob {
		t.Fatalf("alice and bob share session %q, so either could see the other's login", alice)
	}
	if opens := linesWith(journalLines(t, journal), "open "); len(opens) != 2 {
		t.Fatalf("want 2 session opens on the wire, got %d: %v", len(opens), opens)
	}
	if got := d.Actors(); len(got) != 2 || got[0] != "alice" || got[1] != "bob" {
		t.Fatalf("Actors() = %v, want [alice bob]", got)
	}
}

// TestTheSameActorAskedTwiceGetsOneSession is the other half of "per actor": an
// actor acts many times, and a session per ACT would throw away the cookies
// that make them one person.
func TestTheSameActorAskedTwiceGetsOneSession(t *testing.T) {
	d, journal := startFake(t, "ready", true)
	defer func() { _ = d.Close(context.Background()) }()

	first, err := d.SessionFor(context.Background(), "alice")
	if err != nil {
		t.Fatalf("first session for alice: %v", err)
	}
	second, err := d.SessionFor(context.Background(), "alice")
	if err != nil {
		t.Fatalf("second session for alice: %v", err)
	}
	if first != second {
		t.Fatalf("alice was handed two sessions, %q then %q", first, second)
	}
	if opens := linesWith(journalLines(t, journal), "open "); len(opens) != 1 {
		t.Fatalf("want 1 session open on the wire, got %d: %v", len(opens), opens)
	}
}

// TestCloseDeletesEverySessionAndStopsTheDriver is the leak claim, and it
// measures both halves on the wire and in the process table, because a Close
// that deleted the sessions and left the driver running would pass a test that
// only read the journal.
func TestCloseDeletesEverySessionAndStopsTheDriver(t *testing.T) {
	d, journal := startFake(t, "ready", true)
	alice, err := d.SessionFor(context.Background(), "alice")
	if err != nil {
		t.Fatalf("session for alice: %v", err)
	}
	bob, err := d.SessionFor(context.Background(), "bob")
	if err != nil {
		t.Fatalf("session for bob: %v", err)
	}
	pid := d.cmd.Process.Pid

	if err := d.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}

	closes := linesWith(journalLines(t, journal), "close ")
	if len(closes) != 2 {
		t.Fatalf("want 2 session deletes on the wire, got %d: %v", len(closes), closes)
	}
	for _, id := range []string{alice, bob} {
		if len(linesWith(closes, "close "+id)) != 1 {
			t.Fatalf("session %q was not deleted; the journal holds %v", id, closes)
		}
	}
	if d.Actors() != nil && len(d.Actors()) != 0 {
		t.Fatalf("Close left %v holding sessions", d.Actors())
	}
	if alive(pid) {
		t.Fatalf("the driver (pid %d) is still running after Close", pid)
	}
}

// alive asks whether a pid exists, which after a reaped Wait is the whole
// question: signal 0 is delivered to nothing and reports only existence.
func alive(pid int) bool {
	for i := 0; i < 50; i++ {
		if err := syscall.Kill(pid, 0); err != nil {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
	return true
}

// capabilitiesOpened is the capabilities object the driver actually sent, read
// back off the wire rather than from the function that built it.
func capabilitiesOpened(t *testing.T, journal string) map[string]any {
	t.Helper()
	opens := linesWith(journalLines(t, journal), "open ")
	if len(opens) != 1 {
		t.Fatalf("want 1 session open to read capabilities from, got %d", len(opens))
	}
	parts := strings.SplitN(opens[0], " ", 3)
	if len(parts) != 3 {
		t.Fatalf("journal line carries no body: %q", opens[0])
	}
	var body struct {
		Capabilities struct {
			AlwaysMatch map[string]any `json:"alwaysMatch"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal([]byte(parts[2]), &body); err != nil {
		t.Fatalf("decoding the capabilities the driver sent: %v (%q)", err, parts[2])
	}
	return body.Capabilities.AlwaysMatch
}

func chromeArgs(t *testing.T, always map[string]any) []string {
	t.Helper()
	opts, ok := always["goog:chromeOptions"].(map[string]any)
	if !ok {
		t.Fatalf("no goog:chromeOptions in %v", always)
	}
	raw, ok := opts["args"].([]any)
	if !ok {
		t.Fatalf("no args in %v", opts)
	}
	var out []string
	for _, a := range raw {
		out = append(out, fmt.Sprint(a))
	}
	return out
}

func has(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

// TestHeadlessAsksForAHeadlessBrowserAndHeadedDoesNot measures the flag where
// it has an effect -- in the capabilities that reach the driver -- and not in
// the field that holds it.
func TestHeadlessAsksForAHeadlessBrowserAndHeadedDoesNot(t *testing.T) {
	t.Run("headless", func(t *testing.T) {
		d, journal := startFake(t, "ready", true)
		defer func() { _ = d.Close(context.Background()) }()
		if _, err := d.SessionFor(context.Background(), "alice"); err != nil {
			t.Fatalf("session: %v", err)
		}
		if args := chromeArgs(t, capabilitiesOpened(t, journal)); !has(args, "--headless=new") {
			t.Fatalf("a headless driver asked for %v, with no headless flag", args)
		}
	})
	t.Run("headed", func(t *testing.T) {
		d, journal := startFake(t, "ready", false)
		defer func() { _ = d.Close(context.Background()) }()
		if _, err := d.SessionFor(context.Background(), "alice"); err != nil {
			t.Fatalf("session: %v", err)
		}
		if args := chromeArgs(t, capabilitiesOpened(t, journal)); has(args, "--headless=new") {
			t.Fatalf("a headed driver asked for %v, which draws no window", args)
		}
	})
}

// TestTheSandboxFlagIsNeverAskedFor holds issue 122 deliverable 1's measured
// reason (coop rumor/1920): headless chromium keeps its sandbox as an ordinary
// user with a writable HOME, and `--no-sandbox` is a root-only workaround no CI
// step should inherit from us.
func TestTheSandboxFlagIsNeverAskedFor(t *testing.T) {
	for _, headless := range []bool{true, false} {
		t.Run(fmt.Sprintf("headless=%t", headless), func(t *testing.T) {
			d, journal := startFake(t, "ready", headless)
			defer func() { _ = d.Close(context.Background()) }()
			if _, err := d.SessionFor(context.Background(), "alice"); err != nil {
				t.Fatalf("session: %v", err)
			}
			for _, args := range [][]string{chromeArgs(t, capabilitiesOpened(t, journal))} {
				for _, bad := range []string{"--no-sandbox", "--disable-setuid-sandbox"} {
					if has(args, bad) {
						t.Fatalf("the driver asked for %s in %v", bad, args)
					}
				}
			}
		})
	}
}

// TestTheScannedBrowserTravelsIntoTheSession is why Start takes a browser path:
// the scan resolved one binary, and a driver left to find its own could pick a
// different install than the stage line named.
func TestTheScannedBrowserTravelsIntoTheSession(t *testing.T) {
	path, journal := fakeDriver(t, "ready")
	d, err := Start(context.Background(), path, "/opt/odd/place/chromium", true)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = d.Close(context.Background()) }()
	if _, err := d.SessionFor(context.Background(), "alice"); err != nil {
		t.Fatalf("session: %v", err)
	}
	opts, ok := capabilitiesOpened(t, journal)["goog:chromeOptions"].(map[string]any)
	if !ok {
		t.Fatalf("no goog:chromeOptions sent")
	}
	if got := fmt.Sprint(opts["binary"]); got != "/opt/odd/place/chromium" {
		t.Fatalf("the session names browser %q, not the one the scan resolved", got)
	}
}

// TestADriverThatNeverReportsReadyIsABoundedStartError measures the bound. A
// readiness wait that cannot end is this package's worst failure -- it would
// hang every run on a box where the driver half-starts -- so the arm hands
// Start a deadline and requires it back.
func TestADriverThatNeverReportsReadyIsABoundedStartError(t *testing.T) {
	path, journal := fakeDriver(t, "never")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	began := time.Now()
	d, err := Start(ctx, path, "", true)
	took := time.Since(began)
	if err == nil {
		_ = d.Close(context.Background())
		t.Fatalf("Start succeeded against a driver that never reports ready")
	}
	if took > 10*time.Second {
		t.Fatalf("Start took %s to give up on a driver that never reports ready", took)
	}

	// AND IT LEFT NOTHING BEHIND. A failed Start that abandoned its process
	// would leak a driver per attempt, which no later Close can reach.
	port := 0
	for _, l := range linesWith(journalLines(t, journal), "listen ") {
		port, _ = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(l, "listen ")))
	}
	if port == 0 {
		t.Fatalf("the fake driver never recorded its port, so this arm cannot tell a cleanup from a leak")
	}
	if answers(port) {
		t.Fatalf("127.0.0.1:%d still answers, so the failed Start left its driver running", port)
	}
}

// answers asks whether anything is still serving a port, giving a stopping
// process a moment to actually stop.
func answers(port int) bool {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	for i := 0; i < 50; i++ {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/status", port), nil)
		if err != nil {
			return false
		}
		resp, err := client.Do(req)
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		time.Sleep(20 * time.Millisecond)
	}
	return true
}

// TestASessionWithoutAnActorIsRefused: the map is keyed by actor, so an empty
// key would be a session every nameless caller shared.
func TestASessionWithoutAnActorIsRefused(t *testing.T) {
	d, journal := startFake(t, "ready", true)
	defer func() { _ = d.Close(context.Background()) }()
	if _, err := d.SessionFor(context.Background(), ""); err == nil {
		t.Fatalf("an empty actor name opened a session")
	}
	if opens := linesWith(journalLines(t, journal), "open "); len(opens) != 0 {
		t.Fatalf("the refusal still opened %d session(s): %v", len(opens), opens)
	}
}

// TestAStartThatCannotExecIsAnErrorAndNotAPanic: a path that is not a program
// is what a scan on a broken box hands us.
func TestAStartThatCannotExecIsAnErrorAndNotAPanic(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such-driver")
	if _, err := Start(context.Background(), missing, "", true); err == nil {
		t.Fatalf("Start succeeded with no driver at %s", missing)
	}
}

// TestTheReadinessWaitIsBoundedWithoutADeadlineFromTheCaller holds ReadyTimeout
// ITSELF, which the arm above cannot: that one hands Start a 2s context, so it
// would still pass if the only bound were the caller's. A caller passing
// context.Background() -- which the stage reasonably might -- would then hang
// forever on a driver that half-starts, and nothing in this package would
// notice. This arm costs ReadyTimeout of wall clock and buys the bound.
func TestTheReadinessWaitIsBoundedWithoutADeadlineFromTheCaller(t *testing.T) {
	path, _ := fakeDriver(t, "never")
	type outcome struct {
		d   *Driver
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		d, err := Start(context.Background(), path, "", true)
		done <- outcome{d, err}
	}()
	select {
	case got := <-done:
		if got.err == nil {
			_ = got.d.Close(context.Background())
			t.Fatalf("Start succeeded against a driver that never reports ready")
		}
		if !strings.Contains(got.err.Error(), "/status") {
			t.Fatalf("the error does not name the wait that failed: %v", got.err)
		}
	case <-time.After(ReadyTimeout + 10*time.Second):
		t.Fatalf("Start has not returned after %s with no deadline from the caller, so the wait is bounded only by one",
			ReadyTimeout+10*time.Second)
	}
}

// TestADriverThatExitsIsReportedAtOnceAndNotAtTheBound measures the difference
// between "it exited" and "it never answered". A `chromedriver` on PATH that is
// not a webdriver -- a wrapper script, a stub, the wrong package -- exits
// immediately, and waiting ReadyTimeout to say so would stall every run on such
// a box for 20 seconds and then name the wrong fault.
func TestADriverThatExitsIsReportedAtOnceAndNotAtTheBound(t *testing.T) {
	quitter := filepath.Join(t.TempDir(), "zz-quits-at-once")
	if err := os.WriteFile(quitter, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	began := time.Now()
	d, err := Start(context.Background(), quitter, "", true)
	took := time.Since(began)
	if err == nil {
		_ = d.Close(context.Background())
		t.Fatalf("Start succeeded against a driver that exits immediately")
	}
	if !strings.Contains(err.Error(), "exited") {
		t.Errorf("the error does not say the driver exited, so it names the wrong fault: %v", err)
	}
	// WELL INSIDE THE BOUND, which is the claim: half of ReadyTimeout would
	// still be a pass for a wait that learned nothing from the exit.
	if took > ReadyTimeout/4 {
		t.Fatalf("Start took %s to notice a driver that exited, against a bound of %s", took, ReadyTimeout)
	}
}

// TestCloseReportsASessionTheDriverRefusedToDelete is the package half of the
// leak claim. Close is best-effort per session and unconditional for the
// process -- a browser left running is what it exists to prevent -- but
// best-effort must not mean silent: the caller is the only one who can SAY a
// session would not close, and it can only say what Close tells it.
func TestCloseReportsASessionTheDriverRefusedToDelete(t *testing.T) {
	d, journal := startFake(t, "refuse-close", true)
	if _, err := d.SessionFor(context.Background(), "alice"); err != nil {
		t.Fatalf("session for alice: %v", err)
	}
	pid := d.cmd.Process.Pid

	err := d.Close(context.Background())
	if err == nil {
		t.Fatalf("Close reported success over a delete the driver refused with 500")
	}
	for _, want := range []string{"would not close", "alice"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not carry %q, so a caller cannot report it: %v", want, err)
		}
	}
	// THE DELETE WAS ATTEMPTED, so this is about a refusal and not a teardown
	// that skipped the call.
	if closes := linesWith(journalLines(t, journal), "close "); len(closes) != 1 {
		t.Errorf("want 1 delete attempted, got %d: %v", len(closes), closes)
	}
	// AND THE DRIVER IS STILL STOPPED. A session that will not close must not
	// buy the process a reprieve, or a refusing driver would leak itself too.
	if alive(pid) {
		t.Errorf("a refused delete left the driver (pid %d) running", pid)
	}
	// AND THE SESSION IS NOT LEFT IN THE MAP, because a retry against a driver
	// that is gone would be a second failure with no subject.
	if got := d.Actors(); len(got) != 0 {
		t.Errorf("Close left %v holding sessions after a refused delete", got)
	}
}
