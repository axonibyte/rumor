package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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
	// THE PACKAGE AUDITS ITS OWN LEAVINGS, by a before-and-after diff rather
	// than a count. MEASURED 2026-11-08: with the driver signalled instead of
	// its process group, these arms left 8 `chrome:` processes and a
	// chromedriver alive -- orphans that reparented onto the test binary and
	// held `go test` open until it was killed at 110s having passed in 2s.
	//
	// WHY HERE AND NOT IN AN ARM. The leak shows in whichever arm started the
	// browser, so a per-driver assertion sees only its own group and the arm
	// that leaks is the one without the check. A diff over the whole account
	// catches any arm, including ones not written yet, and the BEFORE set is
	// what keeps it honest in a jail where something else may be running: only
	// what this run added can fail it.
	//
	// AND IT IS SCOPED TO THIS PROCESS'S OWN DESCENDANTS, which the gate forced
	// on 2026-11-09: tests/run launches browsers too now, `go test ./...` runs
	// packages in parallel, and this check failed naming 22 processes that were
	// that package's. MEASURED both ways -- this package ALONE is clean and the
	// suite was not -- so the claim "this package left no browser behind" was
	// resting on a measurement of "no browser is running anywhere".
	//
	// THE PROCESS GROUP AND NOT DESCENT, and descent was tried first and
	// MEASURED WRONG. The paragraph above says those orphans "reparented onto
	// the test binary"; they do not. Measured in this jail 2026-11-09: a child
	// whose parent exits gets PPID 1, so a browser outliving its driver is no
	// longer below this process and a descent filter would miss the ONE case
	// this check exists for -- it would have read clean while failing.
	//
	// A pgid SURVIVES ITS LEADER'S DEATH, which is what makes it the right
	// handle: Start sets Setpgid so each driver leads its own group and its
	// browsers inherit it, so an orphan is still in the group whose leader is
	// gone. The groups are recorded as the arms start them, which is why every
	// arm goes through startReal or startFake -- and TestEveryArmStartsThrough
	// TheHelpers keeps an arm not written yet from bypassing the register.
	before := browserProcesses()
	code := m.Run()
	// A LEAK IS REPORTED EVEN WHEN AN ARM ALREADY FAILED, and only the EXIT
	// CODE is withheld in that case. The report used to be skipped entirely on
	// a failure so as not to pile a second complaint onto the first, and that
	// silence has a cost I measured 2026-11-09: a mutant that signalled the
	// driver instead of its group left ELEVEN groups -- 83 processes -- in this
	// jail across one sweep, and every run that produced them FAILED, so
	// nothing said so. That is a sufficient mechanism for the stale processes
	// head 2 recorded as unexplained: any failing run leaks invisibly.
	//
	// THE CODE IS STILL NOT OVERWRITTEN, which is the half worth keeping: a
	// failing arm is the finding a reader should chase first, and a leak report
	// that changed 1 into 1 tells them nothing new.
	if leaked := newSince(before); len(leaked) > 0 {
		fmt.Fprintf(os.Stderr, "\ninternal/browser left %d browser process(es) running: %s\n"+
			"a driver that outlives its arm holds the harness open and accumulates in the jail\n",
			len(leaked), strings.Join(leaked, ", "))
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

// browserProcesses is the pids of every chromedriver and chrome IN A GROUP THIS
// PACKAGE STARTED, which is what "this package left one behind" means. A browser
// another package started is in another group and none of this check's business
// -- reporting one would be a false red against the package that cleaned up.
func browserProcesses() map[string]bool {
	out := map[string]bool{}
	// ONE COLUMN PER KEYWORD, because `-o a=,b=` is ONE column whose header is
	// everything after the first `=`, and a multi-column ps truncates at the
	// terminal width without -ww.
	ps, err := exec.Command("ps", "-axww", "-o", "pid=", "-o", "pgid=", "-o", "command=").Output()
	if err != nil {
		return out
	}
	ours := startedGroups()
	for _, line := range strings.Split(string(ps), "\n") {
		if !strings.Contains(line, "chromedriver") && !strings.Contains(line, "chrome:") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		// f[1] is the PGID here, not the parent: a group's leader is the driver
		// this package started, and its browsers inherit the group.
		if ours[f[1]] {
			out[f[0]] = true
		}
	}
	return out
}

// groups is every process group this package has started, as pgid strings. A
// driver leads its own group (Start sets Setpgid), so the group is the pid.
//
// NO MUTEX AND THE REASON IS STATED: the arms in this package do not call
// t.Parallel, so starts are sequential, and TestMain reads this only after
// m.Run has returned. An arm that added t.Parallel would need a lock here, and
// that is what this sentence is for.
var groups []int

// noteGroup records a driver's group so the leak check can tell this package's
// browsers from another package's.
func noteGroup(d *Driver) {
	if d != nil && d.cmd != nil && d.cmd.Process != nil {
		groups = append(groups, d.cmd.Process.Pid)
	}
}

func startedGroups() map[string]bool {
	out := make(map[string]bool, len(groups))
	for _, g := range groups {
		out[strconv.Itoa(g)] = true
	}
	return out
}

// newSince answers the browser processes that were not there before, waiting a
// little first: a teardown is allowed to be slower than the last arm's return.
func newSince(before map[string]bool) []string {
	var leaked []string
	for i := 0; i < 40; i++ {
		leaked = nil
		for pid := range browserProcesses() {
			if !before[pid] {
				leaked = append(leaked, pid)
			}
		}
		if len(leaked) == 0 {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	sort.Strings(leaked)
	return leaked
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
	noteGroup(d)
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
	noteGroup(d)
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

// shortReady is the bound the bounded-wait cell hands waitReady directly.
// ReadyTimeout is 60s as of rook's measurement at coop r9/99, and a cell that
// sat through it would cost a minute of every suite run to prove a property a
// short bound proves identically -- that SOMETHING other than the caller's
// deadline ends the wait.
const shortReady = 2 * time.Second

// TestTheReadinessWaitIsBoundedWithoutADeadlineFromTheCaller holds the bound
// ITSELF, which the ctx-deadline arm cannot: that one hands Start a 2s context,
// so it would pass if the only bound were the caller's. A caller passing
// context.Background() -- which the stage reasonably might -- would hang forever
// on a driver that half-starts.
//
// IT DRIVES waitReady AND NOT Start, because Start deliberately takes no bound
// argument: a passed bound left the value production sends held by nothing, and
// the mutant that sent 1s from the stage SURVIVED. So the cheap bound is reached
// where it cannot also be production's.
func TestTheReadinessWaitIsBoundedWithoutADeadlineFromTheCaller(t *testing.T) {
	path, _ := fakeDriver(t, "never")
	port, err := freePort()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(path, fmt.Sprintf("--port=%d", port))
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	d := &Driver{
		base:     fmt.Sprintf("http://127.0.0.1:%d", port),
		path:     path,
		http:     &http.Client{Timeout: 5 * time.Second},
		exited:   make(chan struct{}),
		sessions: map[string]string{},
	}
	go func() { _ = cmd.Wait(); close(d.exited) }()
	t.Cleanup(func() {
		_ = cmd.Process.Signal(os.Interrupt)
		<-d.exited
	})

	began := time.Now()
	err = d.waitReady(context.Background(), shortReady)
	took := time.Since(began)
	if err == nil {
		t.Fatalf("waitReady returned nil against a driver that never readies")
	}
	if took > shortReady+5*time.Second {
		t.Fatalf("waitReady took %s against a %s bound and no caller deadline", took, shortReady)
	}
	if !strings.Contains(err.Error(), "/status") {
		t.Fatalf("the error does not name the wait that failed: %v", err)
	}
	// AND IT NAMES STUCK RATHER THAN SLOW, which is rook's diagnosis at coop
	// r9/104 made readable: 40 uncensored runs put healthy readiness in the tens
	// of milliseconds, so a driver that answered and never readied is not behind.
	// A message that only reported the elapsed bound invited the next reader to
	// grant it more time, which is how a stuck driver got filed as a slow one.
	if !strings.Contains(err.Error(), "stuck rather than slow") {
		t.Errorf("the error lets a stuck driver read as a slow one: %v", err)
	}
}

// TestAStartingDriverIsGivenTimeToComeUp is rook's measurement as a cell. A real
// chromedriver on their box exceeded my old 20s bound while STARTING, and the
// stage reported FOUND, NOT DRIVABLE for a driver that was merely slow -- the
// worst reading this package can produce. This drives the PRODUCTION path, bound
// and all, against a fake that withholds readiness and then grants it.
func TestAStartingDriverIsGivenTimeToComeUp(t *testing.T) {
	path, _ := fakeDriver(t, "slow")
	began := time.Now()
	d, err := Start(context.Background(), path, "", true)
	noteGroup(d)
	if err != nil {
		t.Fatalf("a driver that readies after %s was refused: %v", fakedriver.SlowFor, err)
	}
	defer func() { _ = d.Close(context.Background()) }()
	// THE VACUITY GUARD, AND ITS FLOOR IS WRITTEN HERE RATHER THAN IMPORTED.
	// My first version compared the elapsed time against fakedriver.SlowFor --
	// the same constant the fake withholds readiness for -- so a mutant setting
	// SlowFor to 0 moved BOTH sides and the arm went vacuous with its own guard
	// agreeing. That is precisely the fault rook found in my pair-table
	// assertion at coop r9/98, committed again one file away. An independent
	// literal cannot be moved by a cut to the fake.
	const floor = 2 * time.Second
	if fakedriver.SlowFor < floor {
		t.Fatalf("the fake withholds readiness for only %s, under this arm's %s floor: a bound "+
			"too tight to matter would pass", fakedriver.SlowFor, floor)
	}
	if took := time.Since(began); took < floor {
		t.Fatalf("Start returned in %s, inside the %s the fake should have withheld: the arm is vacuous",
			took, floor)
	}
}

// TestTheReadinessBoundIsSizedFromTheMeasuredTail pins the FIGURE, which no
// behavioural cell can: a fake slow by 3s passes a 10s bound as happily as a
// 60s one, so only an assertion on the constant holds its derivation.
//
// BOTH ENDS ARE THE CLAIM. The floor keeps the bound far above the 68.3ms tail
// of forty uncensored runs (rook r9/104, ibis r9/105), so a cold start or CI
// contention neither of us sampled cannot be failed as stuck. The CEILING is the
// half my own two mistakes needed: 20s and then 60s were numbers I chose, and at
// 878x the observed maximum a bound stops discriminating and merely delays the
// message. Either end moving is a figure that has left its measurement.
func TestTheReadinessBoundIsSizedFromTheMeasuredTail(t *testing.T) {
	// the combined observed maximum, written here rather than imported: a
	// derivation that reads its own inputs agrees with itself (coop r9/98).
	const observedMax = 68300 * time.Microsecond
	if ReadyTimeout < 100*observedMax {
		t.Errorf("ReadyTimeout is %s, under 100x the %s tail measured across 40 uncensored runs: "+
			"a cold start nobody sampled would read as STUCK", ReadyTimeout, observedMax)
	}
	if ReadyTimeout > 20*time.Second {
		t.Errorf("ReadyTimeout is %s, over 290x the %s tail: past that it no longer tells a stuck "+
			"driver from a slow one, it only delays the message (rook, coop r9/104)",
			ReadyTimeout, observedMax)
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
	noteGroup(d)
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

// TestEveryStartRegistersItsProcessGroup is the guard the leak check needs, and
// it exists because scoping that check to this package's own groups traded away
// a property the box-wide diff had for free: a diff catches an arm not written
// yet, and a register only catches arms that remember to register.
//
// SO THE REGISTER IS NOT LEFT TO MEMORY. This arm reads this package's own test
// sources and requires every `Start(` that takes a driver to be followed by
// noteGroup within two lines. An arm added later either registers or fails
// here, which is the same bargain the diff made -- paid once, in source.
//
// A SOURCE SCAN AND NOT A RUNTIME COUNT, because the thing being guarded is
// what a FUTURE arm does: a count of registrations at runtime says nothing
// about the arm that forgot, since the arm that forgot is the one not running.
func TestEveryStartRegistersItsProcessGroup(t *testing.T) {
	sources, err := filepath.Glob("*_test.go")
	if err != nil || len(sources) == 0 {
		t.Fatalf("no test sources to read: %v", err)
	}
	// THE NEEDLE IS BUILT FROM PIECES so that no line of this file contains it
	// whole. The first draft spelled it as one literal and the scan reported
	// ITSELF -- twice, since the fix spelled it again -- which is the shape a
	// grep over its own report always takes.
	needle := ":= " + "Start(" + "context.Background()"
	unregistered := []string{}
	for _, name := range sources {
		raw, rerr := os.ReadFile(name)
		if rerr != nil {
			t.Fatalf("reading %s: %v", name, rerr)
		}
		lines := strings.Split(string(raw), "\n")
		for i, line := range lines {
			// A CODE LINE THAT BINDS A DRIVER, not the bare call: the `:=` is
			// what tells a start whose driver is kept from one whose is not.
			if !strings.Contains(line, needle) {
				continue
			}
			// A Start WHOSE DRIVER IS DISCARDED CANNOT LEAK ONE. The arm for a
			// missing driver asserts the error and binds no *Driver, so there
			// is no group to record -- `_, err :=` is that shape.
			if strings.Contains(line, "_, err :=") || strings.Contains(line, "if _, err") {
				continue
			}
			registered := false
			for j := i + 1; j < len(lines) && j <= i+2; j++ {
				if strings.Contains(lines[j], "noteGroup(") {
					registered = true
				}
			}
			if !registered {
				unregistered = append(unregistered,
					fmt.Sprintf("%s:%d", name, i+1))
			}
		}
	}
	if len(unregistered) > 0 {
		t.Errorf("these arms start a driver and do not register its process group, so a browser "+
			"they leak is invisible to TestMain's check: %s", strings.Join(unregistered, ", "))
	}
}

// TestABudgetNeverCoversTheSessionOpen is a guard bought by three failures in
// one day, two of them on the forge and none on this box.
//
// THE SHAPE EVERY TIME: an arm sets a deadline to bound a WAIT it expects to go
// unmet, Flow opens the actor's session inside that deadline, and on a loaded
// box the session POST spends the budget -- so the arm fails with "context
// deadline exceeded" on `/session`, which is a timing fact about the BOX
// reported as a verdict about a wait. It cannot be reproduced here on demand,
// which is exactly why it must not be left to whoever writes the next arm: I
// wrote the lesson into one cell's comment and then made the same mistake in a
// new arm two cells away, in the same sitting.
//
// SO THE RULE IS IN SOURCE: an arm that bounds a flow must acquire the session
// first, outside the budget. Sessions are cached per actor, so the Flow then
// reuses it and the deadline bounds only its subject.
func TestABudgetNeverCoversTheSessionOpen(t *testing.T) {
	sources, err := filepath.Glob("*_test.go")
	if err != nil || len(sources) == 0 {
		t.Fatalf("no test sources to read: %v", err)
	}
	// BUILT FROM PIECES so this arm's own source does not match itself, which
	// is the trap its sibling fell into twice.
	bound := "context." + "WithTimeout("
	drives := []string{"d." + "Flow(", "d." + "Read("}
	pre := "SessionFor(" + "context.Background()"

	var offenders []string
	for _, name := range sources {
		raw, rerr := os.ReadFile(name)
		if rerr != nil {
			t.Fatalf("reading %s: %v", name, rerr)
		}
		for _, chunk := range strings.Split(string(raw), "\nfunc ") {
			if !strings.Contains(chunk, bound) {
				continue
			}
			drivesAFlow := false
			for _, d := range drives {
				if strings.Contains(chunk, d) {
					drivesAFlow = true
				}
			}
			if !drivesAFlow || strings.Contains(chunk, pre) {
				continue
			}
			offenders = append(offenders,
				fmt.Sprintf("%s:%s", name, strings.SplitN(chunk, "(", 2)[0]))
		}
	}
	if len(offenders) > 0 {
		t.Errorf("these arms bound a flow with a deadline and do not acquire the session first, "+
			"so the budget covers the session open and the arm fails on a loaded box about "+
			"something it is not testing: %s", strings.Join(offenders, ", "))
	}
}
