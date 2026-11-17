package browser

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"forge.axonibyte.com/axonibyte/rumor/internal/browser/fakedriver"
)

// killProofEnv asks this test binary to be the SUBJECT of the kill-proof arm
// rather than the one running it: a defer cannot be tested from inside the
// process that would run it, so the arm needs a second process it can signal.
const killProofEnv = "RUMOR_BROWSER_KILLPROOF_CHILD"

// KillProofChildIfAsked is the child half of
// TestATerminationSignalStopsTheBrowsersAndTakesTheTreeWithIt. It starts a
// real Driver against the fake driver, says where its owned tree is, and then
// waits to be signalled -- which is the whole point: nothing here removes the
// tree, so if the tree is gone afterwards it is because the signal handler ran.
func KillProofChildIfAsked() bool {
	if os.Getenv(killProofEnv) == "" {
		return false
	}
	// Set by the child and not inherited from the parent, so that the parent's
	// own TestMain does not mistake ITSELF for the fake driver. The grandchild
	// this Start spawns inherits it and does.
	journal := filepath.Join(os.TempDir(), fmt.Sprintf("killproof-journal-%d", os.Getpid()))
	_ = os.Setenv(fakedriver.ModeEnv, "ready")
	_ = os.Setenv(fakedriver.JournalEnv, journal)
	// AND THIS ONE OFF AGAIN, because Start hands the grandchild os.Environ():
	// a grandchild that still saw it would run this function instead of
	// serving the fake driver's protocol, and the driver would never answer.
	_ = os.Unsetenv(killProofEnv)

	d, err := Start(context.Background(), os.Args[0], "/nonexistent/chromium", true)
	noteGroup(d)
	// REGISTERED LIKE EVERY OTHER START, and honest about what that buys here:
	// this process exits through the signal handler and never reaches
	// TestMain's leavings audit, so the registration audits nothing in the
	// CHILD. What actually stops this arm leaking is the parent killing the
	// child's whole group in its defer. The call stays because the gate's
	// bargain is paid in source by every Start that binds a driver, and an
	// exemption for this one would put the hole in the check rather than here.
	if err != nil {
		fmt.Println("ERR " + err.Error())
		os.Exit(2)
	}
	// THE DRIVER'S PID TRAVELS WITH THE TREE, because quill's prediction
	// (coop r9/403) is about PROCESSES and not directories: after the fix a
	// killed run must leave ZERO, not fewer, and a sweep that only collected
	// directories would look identical from the tree alone.
	//
	// AND THE REACH OF THAT, STATED, because this arm does not settle quill's
	// prediction and should not be read as doing so. The fake driver spawns no
	// browser, so signalling the PROCESS and signalling its GROUP are
	// indistinguishable here -- the distinction that the 227 orphans in CI
	// turned on is exactly the one a fake cannot show. What this pid does
	// answer is whether the driver is reached AT ALL on the kill path. DRIVEN:
	// with signalGroup mutated to a no-op the arm fails, though on the child's
	// own exit rather than here, because the teardown then blocks on the reap.
	// The browser half needs the real pair, which is CI's arm and not a unit's.
	fmt.Printf("READY %s %d\n", d.tmpdir, d.cmd.Process.Pid)
	_ = os.Stdout.Sync()
	select {} // wait to be killed; the handler is the thing under test
}

// TestATerminationSignalStopsTheBrowsersAndTakesTheTreeWithIt is the arm the
// owned TMPDIR shipped without, and the one quill's probe showed was needed
// (coop r9/401): the removal lived only on paths a signal never reaches, so a
// killed run left its tree AND its browsers behind. CI runs the suite under
// `timeout`, which sends SIGTERM, so this is the common path.
//
// IT HAS TO BE TWO PROCESSES. An in-process arm can only assert that Close
// removes the tree, which was already true and was never the defect.
func TestATerminationSignalStopsTheBrowsersAndTakesTheTreeWithIt(t *testing.T) {
	child := exec.Command(os.Args[0])
	child.Env = append(os.Environ(), killProofEnv+"=1")
	// The child must NOT inherit a fake-driver mode, or it would serve the
	// protocol instead of running the arm.
	child.Env = withoutVars(child.Env, fakedriver.ModeEnv, fakedriver.JournalEnv)
	child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe for the child: %v", err)
	}
	if err := child.Start(); err != nil {
		t.Fatalf("starting the child: %v", err)
	}
	defer func() {
		_ = syscall.Kill(-child.Process.Pid, syscall.SIGKILL)
		_ = child.Wait()
	}()

	line := make(chan string, 1)
	go func() {
		r := bufio.NewReader(stdout)
		s, _ := r.ReadString('\n')
		line <- strings.TrimSpace(s)
	}()

	var tmpdir string
	var driverPID int
	select {
	case s := <-line:
		if _, err := fmt.Sscanf(s, "READY %s %d", &tmpdir, &driverPID); err != nil {
			t.Fatalf("the child did not report a tree and a driver pid, it said: %q", s)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the child never reported its tree and driver pid")
	}

	if _, err := os.Stat(tmpdir); err != nil {
		t.Fatalf("the child's owned tree should exist before the signal: %v", err)
	}
	if !aliveElsewhere(driverPID) {
		t.Fatalf("the driver %d was already gone before the signal, so this arm "+
			"would pass without testing anything", driverPID)
	}

	// THE POLITE KILL, to the child alone and not its group: the group would
	// also reach the driver directly, and then the tree could be gone for a
	// reason that is not the handler.
	if err := syscall.Kill(child.Process.Pid, syscall.SIGTERM); err != nil {
		t.Fatalf("signalling the child: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	select {
	case <-done:
	case <-time.After(StopGrace + 30*time.Second):
		t.Fatal("the child did not exit after SIGTERM")
	}

	if _, err := os.Stat(tmpdir); !os.IsNotExist(err) {
		t.Errorf("the owned tree survived SIGTERM at %s (stat err %v): "+
			"the teardown did not run, which is the leak this arm exists for", tmpdir, err)
	}

	// THE PROCESS HALF, which is the one that took CI down. The driver was
	// reparented when the child died, so it is nothing's child now and only
	// the group signal can have reached it. Bounded rather than instant: a
	// reap is allowed to be slower than the parent's exit.
	gone := false
	for i := 0; i < 50; i++ {
		if !aliveElsewhere(driverPID) {
			gone = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !gone {
		t.Errorf("the driver %d is still running after the child took SIGTERM: "+
			"the group signal was skipped on the kill path, so a killed run leaves "+
			"its processes behind and the tree's removal only hid it", driverPID)
	}
}

func withoutVars(env []string, names ...string) []string {
	out := env[:0:0]
	for _, kv := range env {
		drop := false
		for _, n := range names {
			if strings.HasPrefix(kv, n+"=") {
				drop = true
				break
			}
		}
		if !drop {
			out = append(out, kv)
		}
	}
	return out
}

// TestTheSweepTakesADeadRunsTreeAndLeavesALiveOnesAlone is the SIGKILL half.
// Nothing in-process can run when the process is killed outright, so the only
// remedy is the next run reclaiming what the last one stranded -- and the only
// thing making that safe is that two rumor runs share the ci jail, so a sweep
// that could not tell them apart would delete a live run's tree.
func TestTheSweepTakesADeadRunsTreeAndLeavesALiveOnesAlone(t *testing.T) {
	dir := t.TempDir()

	dead := deadPID(t)
	stale := filepath.Join(dir, fmt.Sprintf("%s%d-stale", tmpdirPrefix, dead))
	live := filepath.Join(dir, fmt.Sprintf("%s%d-live", tmpdirPrefix, os.Getpid()))
	foreign := filepath.Join(dir, "org.chromium.Chromium.abc")
	for _, d := range []string{stale, live, foreign} {
		if err := os.MkdirAll(filepath.Join(d, "inside"), 0o755); err != nil {
			t.Fatalf("making %s: %v", d, err)
		}
	}

	removed, kept := sweepStaleTmpdirsIn(dir)

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("the dead run's tree survived the sweep (stat err %v)", err)
	}
	if _, err := os.Stat(live); err != nil {
		t.Errorf("a LIVE run's tree was swept away, which is the bug the pid in "+
			"the name exists to prevent: %v", err)
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Errorf("the sweep reached a directory that is not ours at all: %v", err)
	}
	if removed != 1 || kept != 1 {
		t.Errorf("sweep reported removed=%d kept=%d, want 1 and 1", removed, kept)
	}
}

// deadPID answers a pid that certainly belonged to a process and certainly
// does not now, by starting one and reaping it.
func deadPID(t *testing.T) int {
	t.Helper()
	c := exec.Command("/bin/sh", "-c", "exit 0")
	if err := c.Run(); err != nil {
		t.Fatalf("running a throwaway process: %v", err)
	}
	return c.Process.Pid
}

// TestOnlyOurOwnTmpdirNamesAreReadAsOurs guards the parser the sweep's safety
// rests on: a loose match is how a sweep starts deleting what it was never
// meant to see.
func TestOnlyOurOwnTmpdirNamesAreReadAsOurs(t *testing.T) {
	for _, c := range []struct {
		name string
		pid  int
		ok   bool
	}{
		{"rumor-browser-1234-abcdef", 1234, true},
		{"rumor-browser-1-x", 1, true},
		{"rumor-browser-probe", 0, false},
		{"rumor-browser-abc-def", 0, false},
		{"rumor-browser--x", 0, false},
		{"rumor-browser-0-x", 0, false},
		{"rumor-browser-1234", 0, false},
		{"org.chromium.Chromium.x", 0, false},
		{"notrumor-browser-12-x", 0, false},
	} {
		pid, ok := ownerOfTmpdir(c.name)
		if ok != c.ok || pid != c.pid {
			t.Errorf("%q read as pid=%d ok=%v, want pid=%d ok=%v", c.name, pid, ok, c.pid, c.ok)
		}
	}
}

// TestTheSweepReapsADeadRunsGroupAndRefusesAForeignOne is quill's correction
// (coop r9/405) made into an arm: a sweep that only REMOVED would fix the
// harmless half twice over, leaving a SIGKILLed run's browsers alive for the
// next run to inherit. What this package decides is whether a recorded group
// is still ours and what signal it gets; whether a live browser then dies is
// the real pair's question, which a jail with no chromium cannot ask.
func TestTheSweepReapsADeadRunsGroupAndRefusesAForeignOne(t *testing.T) {
	for _, c := range []struct {
		name    string
		members []string
		want    []syscall.Signal
	}{
		{"a driver and its browsers", []string{
			"/usr/local/bin/chromedriver --port=1", "chrome: --type=renderer", "chrome: --type=gpu",
		}, []syscall.Signal{syscall.SIGTERM}},
		{"browsers whose driver already went", []string{
			"chrome: --type=renderer",
		}, []syscall.Signal{syscall.SIGTERM}},
		{"a RECYCLED group that is not ours", []string{
			"chrome: --type=renderer", "/usr/bin/vi notes.txt",
		}, nil},
		{"a group that is wholly someone else's", []string{
			"/usr/local/bin/postgres",
		}, nil},
		{"a group that no longer exists", nil, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			var sent []syscall.Signal
			// THE GROUP OBEYS THE POLITE SIGNAL HERE, which the lister has to
			// model or every case escalates: members until TERM lands, none
			// after. A group that does NOT obey is its own arm below.
			defer restoreReaper(swapReaper(
				func(int) []string {
					if len(sent) > 0 {
						return nil
					}
					return c.members
				},
				func(_ int, sig syscall.Signal) error { sent = append(sent, sig); return nil },
			))
			reapGroup(4242)
			if len(sent) != len(c.want) {
				t.Fatalf("group %v was sent %v, want %v", c.members, sent, c.want)
			}
			for i := range c.want {
				if sent[i] != c.want[i] {
					t.Errorf("signal %d was %v, want %v", i, sent[i], c.want[i])
				}
			}
		})
	}
}

// TestAGroupThatWillNotGoGetsKilled holds the escalation: the members never
// leave, so the polite signal must be followed by the final one. It is the
// same bargain stopProcess makes with a live driver.
func TestAGroupThatWillNotGoGetsKilled(t *testing.T) {
	var sent []syscall.Signal
	defer restoreReaper(swapReaper(
		func(int) []string { return []string{"chrome: --type=renderer"} },
		func(_ int, sig syscall.Signal) error { sent = append(sent, sig); return nil },
	))
	reapGroup(4242)
	if len(sent) != 2 || sent[0] != syscall.SIGTERM || sent[1] != syscall.SIGKILL {
		t.Errorf("a group that would not go was sent %v, want TERM then KILL", sent)
	}
}

// TestTheRecordedGroupSurvivesTheRunThatWroteIt is the handle itself: without
// it a later sweep has only rumor's own dead pid, which says nothing about
// where the orphans went.
func TestTheRecordedGroupSurvivesTheRunThatWroteIt(t *testing.T) {
	tree := t.TempDir()
	if got := readPGID(tree); got != 0 {
		t.Errorf("a tree with no record answered %d, want 0", got)
	}
	recordPGID(tree, 31337)
	if got := readPGID(tree); got != 31337 {
		t.Errorf("the recorded group read back as %d, want 31337", got)
	}
	// A tree whose record is nonsense must answer 0 rather than a pgid: 0 and
	// 1 are the two values that would signal something that is not ours.
	for _, junk := range []string{"", "   ", "nope", "0", "1", "-9"} {
		if err := os.WriteFile(filepath.Join(tree, pgidFile), []byte(junk), 0o644); err != nil {
			t.Fatalf("writing junk: %v", err)
		}
		if got := readPGID(tree); got != 0 {
			t.Errorf("record %q read as %d, want 0", junk, got)
		}
	}
}

func swapReaper(list func(int) []string, sig func(int, syscall.Signal) error) (func(int) []string, func(int, syscall.Signal) error) {
	oldList, oldSig := listGroupMembers, signalGroupOf
	listGroupMembers, signalGroupOf = list, sig
	return oldList, oldSig
}

func restoreReaper(list func(int) []string, sig func(int, syscall.Signal) error) {
	listGroupMembers, signalGroupOf = list, sig
}
