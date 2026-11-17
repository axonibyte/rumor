package browser

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
// TestATerminationSignalStopsTheBrowsersAndKeepsTheTree. It starts a
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

// TestATerminationSignalStopsTheBrowsersAndKeepsTheTree is the arm the
// owned TMPDIR shipped without, and the one quill's probe showed was needed
// (coop r9/401): the removal lived only on paths a signal never reaches, so a
// killed run left its tree AND its browsers behind. CI runs the suite under
// `timeout`, which sends SIGTERM, so this is the common path.
//
// IT HAS TO BE TWO PROCESSES. An in-process arm can only assert that Close
// removes the tree, which was already true and was never the defect.
func TestATerminationSignalStopsTheBrowsersAndKeepsTheTree(t *testing.T) {
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

	// THE TREE IS EXPECTED TO SURVIVE NOW, handed to the next run's sweep, so
	// what proves the handler ran is the PROCESS half below and not a deleted
	// directory. Asserting absence here would be the stale verdict quill warns
	// about at coop r9/459, whose easy fix is restoring the race.
	if _, err := os.Stat(tmpdir); err != nil {
		t.Errorf("the owned tree is gone at %s (%v): the handler must hand it on, "+
			"not delete it", tmpdir, err)
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

	got := sweepStaleTmpdirsIn(dir)

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
	if got.removed != 1 || got.kept != 1 || got.refused != 0 {
		t.Errorf("sweep reported removed=%d refused=%d kept=%d, want 1, 0 and 1",
			got.removed, got.refused, got.kept)
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
		// A PATH, A COMMAND LINE AND A TAB, each of which read as ours with
		// pid 87357 until quill drove the exact-path mutant (coop r10/11) and
		// found that this function's documented strictness stopped at the
		// first `-`. treeInArgv's truncation was the only guard; the comment
		// there said otherwise, which is the worse half of the defect.
		{"rumor-browser-87357-3668960404/org.chromium.Chromium.scoped_dir.ug4m8j", 0, false},
		{"rumor-browser-87357-3668960404 --enable-logging", 0, false},
		{"rumor-browser-87357-3668960404\tx", 0, false},
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
				func(int) ([]string, error) {
					if len(sent) > 0 {
						return nil, nil
					}
					return c.members, nil
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
		func(int) ([]string, error) { return []string{"chrome: --type=renderer"}, nil },
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

func swapReaper(list func(int) ([]string, error), sig func(int, syscall.Signal) error) (func(int) ([]string, error), func(int, syscall.Signal) error) {
	oldList, oldSig := listGroupMembers, signalGroupOf
	listGroupMembers, signalGroupOf = list, sig
	return oldList, oldSig
}

func restoreReaper(list func(int) ([]string, error), sig func(int, syscall.Signal) error) {
	listGroupMembers, signalGroupOf = list, sig
}

// TestTheSweepSaysWhatItDidAndIsQuietWhenItDidNothing is the instrument the
// sweep shipped without, and the gap cost a real attribution: task 1247 was
// cancelled with browsers up, its groups were gone afterwards, and nothing in
// any log said who took them -- "the run left none" and "the next run's sweep
// reaped them" were separable only by argument (quill, coop r9/422). A sweep
// that says what it did makes the next one a reading.
func TestTheSweepSaysWhatItDidAndIsQuietWhenItDidNothing(t *testing.T) {
	if got := sweepReport(sweepTally{kept: 3}); got != "" {
		t.Errorf("a sweep that collected nothing said %q, want silence: a line on "+
			"every start is noise that trains a reader to skip it", got)
	}
	// REMOVED WITHOUT REAPING IS STILL WORTH SAYING, because it is the shape a
	// pre-fix tree leaves: no pgid recorded, so nothing to signal.
	if got := sweepReport(sweepTally{removed: 2, nohandle: 2, kept: 1}); !strings.Contains(got, "swept 2") ||
		!strings.Contains(got, "reaped 0") {
		t.Errorf("a sweep that removed trees but reaped nothing said %q", got)
	}
	got := sweepReport(sweepTally{removed: 4, reaped: 3, kept: 2})
	for _, want := range []string{"swept 4", "reaped 3", "left 2"} {
		if !strings.Contains(got, want) {
			t.Errorf("the sweep's line %q omits %q", got, want)
		}
	}

	// THE CASE THE BOOL HID. An EMPTY group and a REFUSED one both left
	// "reaped 0" and were byte-identical, and refused is the only one meaning
	// our orphans are STILL RUNNING (quill, coop r9/426). The two lines must
	// differ, and the refusal must be the one that is loud.
	empty := sweepReport(sweepTally{removed: 1})
	refused := sweepReport(sweepTally{removed: 1, refused: 1})
	if empty == refused {
		t.Errorf("an empty group and a REFUSED one read identically (%q): the "+
			"alarming outcome is invisible, which is the defect this splits", empty)
	}
	if !strings.Contains(refused, "REFUSED 1") || !strings.Contains(refused, "still alive") {
		t.Errorf("a refusal does not say a group was left alive: %q", refused)
	}
	if strings.Contains(empty, "REFUSED") {
		t.Errorf("an empty group raises a refusal alarm: %q", empty)
	}
	// AND A REFUSAL ALONE BREAKS THE SILENCE, even with nothing swept or
	// reaped: it is the one outcome that must never be quiet.
	if sweepReport(sweepTally{refused: 1}) == "" {
		t.Error("a sweep that refused a live group said nothing at all")
	}
}

// TestAReapedGroupIsReportedAndARefusedOneIsNot keeps the count honest: a
// group the guard refuses must not be counted as reaped, or the line would
// claim work the sweep declined to do.
func TestAReapedGroupIsReportedAndARefusedOneIsNot(t *testing.T) {
	defer restoreReaper(swapReaper(
		func(int) ([]string, error) { return []string{"chrome: --type=renderer"}, nil },
		func(int, syscall.Signal) error { return nil },
	))
	if got := reapGroup(4242); got != reapSignalled {
		t.Errorf("a group wholly ours reported outcome %d, want reapSignalled", got)
	}
	// SWAPPED AND NOT RESTORED HERE: restoreReaper(swapReaper(...)) would put
	// the originals straight back, which is how this arm first passed over a
	// stub that was never in effect. The deferred restore above holds the
	// originals and still runs.
	swapReaper(
		func(int) ([]string, error) { return []string{"/usr/bin/vi notes.txt"}, nil },
		func(int, syscall.Signal) error { return nil },
	)
	if got := reapGroup(4242); got != reapRefused {
		t.Errorf("a FOREIGN group reported outcome %d, want reapRefused: counting a "+
			"refusal as empty is the conflation that hid a live leak", got)
	}
}

// TestTeardownNeverRemovesTheTreeItHandsToTheSweep is the contract after
// quill's structural fix (coop r9/457), and it replaces an arm of mine that
// had stopped being able to fail.
//
// THE ARM IT REPLACES asserted the tree is kept WHEN THE GROUP WILL NOT
// EMPTY. That was right while teardown still removed the tree in the ordinary
// case -- and verifying emptiness first turned out to be necessary and NOT
// sufficient: task 1251 went green with the check answering true, the removal
// running, and 46 chrome alive in the group afterwards. killpg reaches the
// members that exist AT THAT INSTANT, chrome forks helpers continuously, and
// no tighter loop closes that because every check has an after.
//
// SO TEARDOWN NO LONGER REMOVES AT ALL and the next run's sweep does it,
// which makes "the tree is kept" true unconditionally -- and made the old arm
// unfireable. Both cases are driven here instead: a clean stop and a stop with
// a survivor planted in the group must each leave the tree AND its pgid, so
// the handle is there for the sweep either way.
func TestTeardownNeverRemovesTheTreeItHandsToTheSweep(t *testing.T) {
	for _, c := range []struct {
		name     string
		survivor bool
	}{
		{"a clean stop", false},
		{"a stop whose group will not empty", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			d, _ := startFake(t, "ready", true)
			tree := d.tmpdir
			pgid := d.cmd.Process.Pid
			if tree == "" {
				t.Fatal("the driver reported no owned tree")
			}

			if c.survivor {
				// Stands in for the helper chrome forks past the signal: a real
				// member of the driver's group that ignores TERM.
				sv := exec.Command("/bin/sh", "-c", "trap '' TERM; sleep 60")
				sv.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pgid: pgid}
				if err := sv.Start(); err != nil {
					t.Skipf("cannot plant a survivor in the driver's group: %v", err)
				}
				defer func() { _ = sv.Process.Kill(); _ = sv.Wait() }()
			}

			_ = d.Close(context.Background())

			if _, err := os.Stat(tree); err != nil {
				t.Errorf("teardown REMOVED %s: the handle is gone, so no later sweep "+
					"can reach that group -- which is how a green run strands browsers "+
					"nobody can find (stat err %v)", tree, err)
			}
			// THE HANDLE AND NOT ONLY THE DIRECTORY: an empty tree is no use to
			// the sweep, which reads the group out of driver.pgid.
			if got := readPGID(tree); got != pgid {
				t.Errorf("the kept tree records pgid %d, want %d: the sweep would have "+
					"a directory and no way to reach the group", got, pgid)
			}

			// This arm's own leavings: the signals are real here, so stop the group.
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
			_ = os.RemoveAll(tree)
		})
	}
}

// TestATeardownWhoseSignalsDoNotLandRETURNS is quill's second finding driven
// (coop r9/437). The StopGrace branch read `signalGroup(KILL); <-d.exited`
// with nothing behind it, so a SIGKILL that does not land left teardown
// blocked FOREVER. quill hit it by neutering the signal and watching their
// probe hang to rc 124 at 120s -- which in CI is a hang inside the suite that
// the outer budget reports as rc 124, indistinguishable from the 1800s kill
// that nine heads were read as. A teardown defect wearing a box problem's
// clothes.
//
// NEUTERING THE SIGNAL IS THE ONLY WAY IN. quill could not force the real
// case -- it needs a driver that survives SIGKILL -- so the seam stands in for
// it, and what is asserted is the BOUND: teardown returns, and the tree is
// kept because the group it could not stop is still alive.
func TestATeardownWhoseSignalsDoNotLandRETURNS(t *testing.T) {
	d, _ := startFake(t, "ready", true)
	tree := d.tmpdir

	defer restoreReaper(swapReaper(
		listGroupMembers,
		// The signal is swallowed: nothing is delivered, so the driver lives.
		func(int, syscall.Signal) error { return nil },
	))

	done := make(chan struct{})
	go func() { _ = d.Close(context.Background()); close(done) }()

	// GENEROUS BUT FINITE. The two grace periods plus room for the emptiness
	// check; the unbounded version never arrives at all, which is the point.
	select {
	case <-done:
	case <-time.After(4*StopGrace + 30*time.Second):
		t.Fatal("teardown did not return when its signals did not land: this is the " +
			"unbounded `<-d.exited`, and in CI it is a hang the budget kill reports " +
			"as rc 124 rather than as a teardown that could not stop its driver")
	}

	if _, err := os.Stat(tree); err != nil {
		t.Errorf("the tree was removed although the group was never stopped (%v): "+
			"the handle must survive a teardown that failed", err)
	}
	// THE DRIVER IS STILL RUNNING -- that is what made the wait unbounded, and
	// it is this arm's own leaving. The signals are restored by the deferred
	// swap above, so stop the group for real rather than handing TestMain's
	// leavings audit a process this test created on purpose.
	if pgid := readPGID(tree); pgid > 1 {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	}
	_ = os.RemoveAll(tree)
}

// swapProcessTable substitutes the whole-table ps reading the argv pass walks,
// and answers the original so a deferred restore can put it back. It is
// separate from swapReaper because the two readings answer different questions
// and an arm that wants one must not silently get the other.
func swapProcessTable(all func() ([]procLine, error)) func() ([]procLine, error) {
	old := listAllProcesses
	listAllProcesses = all
	return old
}

// chromeArgvFromTheField is one process's command line as ps printed it on
// forge-01 at 2026-11-17T03:49Z, trimmed in the middle only. It is a MEMBER OF
// GROUP 88126, the group cancelled task 1253 left running and task 1254's
// sweep removed the tree of: this string is the whole reason the argv pass
// exists, so the parser is driven against the real bytes rather than a
// plausible imitation of them.
const chromeArgvFromTheField = "chrome: --type=utility " +
	"--utility-sub-type=unzip.mojom.Unzipper --lang=en-US " +
	"--user-data-dir=/tmp/rumor-browser-87357-3668960404/" +
	"org.chromium.Chromium.scoped_dir.ug4m8j --enable-logging --log-level=0 (chrome)"

// TestATreeNameIsReadOutOfARealBrowsersArgv drives the provenance test over
// the field's own bytes, and over the shapes that must NOT read as ours.
func TestATreeNameIsReadOutOfARealBrowsersArgv(t *testing.T) {
	owner, name, ok := treeInArgv(chromeArgvFromTheField)
	if !ok || owner != 87357 || name != "rumor-browser-87357-3668960404" {
		t.Fatalf("the field's own chrome argv read as owner=%d name=%q ok=%v, want "+
			"87357 and rumor-browser-87357-3668960404: the group this pass was "+
			"built for would not be found", owner, name, ok)
	}
	for _, c := range []struct {
		why, command string
	}{
		{"a neighbour's browser in the same jail, same user, same binary",
			"chrome: --type=renderer --user-data-dir=/tmp/org.chromium.Chromium.9fQ2"},
		{"a chromedriver with no data dir at all, which 7 of the drained 234 were",
			"/usr/local/bin/chromedriver --port=9515"},
		{"rumor-browser-probe, an ACTOR NAME in cmd/rumor and not a tree",
			"rumor run --actor rumor-browser-probe"},
		{"a tree name with no maker pid in it", "chrome: --user-data-dir=/tmp/rumor-browser-x1/u"},
	} {
		if _, _, ok := treeInArgv(c.command); ok {
			t.Errorf("%s read as ours: %q", c.why, c.command)
		}
	}
}

// TestAGroupWhoseTreeIsGONEIsStillReapedFromItsArgv is the arm for what task
// 1254 actually did: it swept 47 stale trees, reaped 0, and left group 88126
// running with 33 members, because the sweep REMOVED the tree whose
// driver.pgid was the only handle on that group. The recovery has no file to
// read -- the tree is gone and is not planted here -- so the group's own argv
// is the only thing that can find it.
func TestAGroupWhoseTreeIsGONEIsStillReapedFromItsArgv(t *testing.T) {
	dead := deadPID(t)
	// THE GROUP DIES WHEN IT IS SIGNALLED, which is both what a reachable
	// group does and what keeps this arm off StopGrace's clock: the first
	// listing is the membership test reapGroup makes before it signals, and
	// every listing after it is the wait for the group to empty.
	listings := 0
	defer restoreReaper(swapReaper(
		func(int) ([]string, error) {
			listings++
			if listings > 1 {
				return nil, nil
			}
			return []string{chromeArgvFromTheField}, nil
		},
		func(pgid int, sig syscall.Signal) error { return nil },
	))
	defer func(old func() ([]procLine, error)) { listAllProcesses = old }(swapProcessTable(
		func() ([]procLine, error) {
			return []procLine{{pgid: 88126, command: strings.Replace(chromeArgvFromTheField,
				"87357", strconv.Itoa(dead), 1)}}, nil
		},
	))

	var signalled []int
	signalGroupOf = func(pgid int, sig syscall.Signal) error {
		signalled = append(signalled, pgid)
		return nil
	}

	got := reapOrphansByArgv()

	if got.recovered != 1 {
		t.Errorf("the argv pass recovered %d group(s) from a dead run's live "+
			"browser, want 1: nothing else can reach a group whose tree is gone",
			got.recovered)
	}
	if len(signalled) != 1 || signalled[0] != 88126 {
		t.Fatalf("the signals went to %v, want exactly group 88126: a group that "+
			"emptied on the polite signal must not also be SIGKILLed", signalled)
	}
	if len(got.recoveredGroups) != 1 || got.recoveredGroups[0] != 88126 {
		t.Errorf("recoveredGroups=%v, want [88126]: counted without recording, so "+
			"the line could not say WHICH group argv reached", got.recoveredGroups)
	}
}

// TestALiveRunsBrowsersAreNOTReapedFromArgv is the condition the whole pass
// turns on, and the one quill set on the first sweep: two rumor runs share the
// ci jail, so a reaper that cannot tell them apart kills a CONCURRENT run's
// browsers. The maker's pid in the tree name is the same guard the tree sweep
// uses, applied to the same name from the other side.
func TestALiveRunsBrowsersAreNOTReapedFromArgv(t *testing.T) {
	defer restoreReaper(swapReaper(
		func(int) ([]string, error) { return []string{chromeArgvFromTheField}, nil },
		func(int, syscall.Signal) error { return nil },
	))
	defer func(old func() ([]procLine, error)) { listAllProcesses = old }(swapProcessTable(
		func() ([]procLine, error) {
			// OUR OWN PID IN THE NAME: this process is alive by definition, so
			// its browsers are a live run's, which is exactly the case a
			// neighbouring rumor job presents.
			return []procLine{{pgid: 88126, command: strings.Replace(
				chromeArgvFromTheField, "87357", strconv.Itoa(os.Getpid()), 1)}}, nil
		},
	))

	var signalled []int
	signalGroupOf = func(pgid int, sig syscall.Signal) error {
		signalled = append(signalled, pgid)
		return nil
	}

	if got := reapOrphansByArgv(); got.recovered != 0 || got.refused != 0 {
		t.Errorf("a LIVE run's browsers were counted recovered=%d refused=%d, "+
			"want 0 and 0", got.recovered, got.refused)
	}
	if len(signalled) != 0 {
		t.Fatalf("a live run's group was signalled: %v", signalled)
	}
}

// TestTheSweepKeepsTheTreeOfAGroupItWillNotReap is the other half of 1254's
// defect. A refused group is one that is STILL RUNNING, and its tree holds the
// only handle anything has on it: removing that tree while the group lives
// makes the group unreachable by this package's own record, which is how a
// leak becomes permanent instead of being cleared by the next run.
func TestTheSweepKeepsTheTreeOfAGroupItWillNotReap(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, fmt.Sprintf("%s%d-refused", tmpdirPrefix, deadPID(t)))
	if err := os.MkdirAll(stale, 0o755); err != nil {
		t.Fatalf("making %s: %v", stale, err)
	}
	recordPGID(stale, 4242)
	defer restoreReaper(swapReaper(
		// A STRANGER IN THE GROUP, so the reap refuses and the group lives.
		func(int) ([]string, error) { return []string{"/usr/bin/vi notes.txt"}, nil },
		func(int, syscall.Signal) error { return nil },
	))

	got := sweepStaleTmpdirsIn(dir)

	if got.refused != 1 || got.removed != 0 {
		t.Errorf("sweep reported refused=%d removed=%d, want 1 and 0", got.refused, got.removed)
	}
	if len(got.refusedGroups) != 1 || got.refusedGroups[0] != 4242 {
		t.Errorf("refusedGroups=%v, want [4242]: a refusal means our orphans are "+
			"STILL RUNNING, so the one outcome that must never be anonymous is "+
			"exactly this one", got.refusedGroups)
	}
	if _, err := os.Stat(filepath.Join(stale, pgidFile)); err != nil {
		t.Fatalf("the handle on a group that is STILL RUNNING was deleted: %v -- "+
			"this is what task 1254 did to group 88126, and it is why that group "+
			"could not be reclaimed by any later sweep", err)
	}
}

// TestTheSweepsLineTellsAHandleItNeverHadFromAGroupItReaped closes the gap the
// field found in the instrument: reapNoHandle and reapEmpty were both counted
// NOWHERE, so "swept 47 stale tree(s), reaped 0 browser group(s), left 0 in
// use" was the line for a sweep with nothing to do AND for a sweep that had
// just dropped the handle on 33 live processes.
func TestTheSweepsLineTellsAHandleItNeverHadFromAGroupItReaped(t *testing.T) {
	nothing := sweepReport(sweepTally{})
	if nothing != "" {
		t.Errorf("a sweep that did nothing said %q, want silence", nothing)
	}
	// THE 1254 SHAPE: trees removed, no group reaped, and the reason being
	// that none of them recorded a handle.
	blind := sweepReport(sweepTally{removed: 47, nohandle: 47})
	if !strings.Contains(blind, "47 stale tree(s) recorded no handle") {
		t.Errorf("a sweep that found no handle on 47 trees said %q, which is "+
			"indistinguishable from 47 trees whose groups were already gone", blind)
	}
	if blind == sweepReport(sweepTally{removed: 47}) {
		t.Error("a sweep that had no handle and one whose groups were empty read " +
			"identically: that is the silence the field was lost in")
	}
	// AND A RECOVERY IS LOUD, because it means the first pass missed a live
	// group and only argv found it.
	saved := sweepReport(sweepTally{removed: 47, nohandle: 46, recovered: 1})
	if !strings.Contains(saved, "RECOVERED 1 group(s) from argv") {
		t.Errorf("an argv recovery is not named in %q", saved)
	}
	if sweepReport(sweepTally{recovered: 1}) == "" {
		t.Error("a sweep whose only act was recovering a live group said nothing at all")
	}
}

// TestTheOneColumnPsFormIsReadAsNOTHINGRatherThanAsAGroup guards the trap that
// has cost this package six wrong readings: `ps -o a=,b=` is ONE column on
// this ps, whose header is everything after the first `=`. A parser that took
// the single column as a pgid with an empty command would answer that every
// group is empty -- benign-looking, and the reason nothing would ever be
// reaped again.
func TestTheOneColumnPsFormIsReadAsNOTHINGRatherThanAsAGroup(t *testing.T) {
	twoColumn := []byte("88126 chrome: --type=renderer\n 1 /sbin/init --x\n")
	if got := parsePSTable(twoColumn); len(got) != 2 || got[0].pgid != 88126 ||
		!strings.HasPrefix(got[0].command, "chrome:") {
		t.Fatalf("the two-column reading parsed as %+v", got)
	}
	// WHAT THE COMMA FORM ACTUALLY PRINTS: the pgid alone, no command.
	if got := parsePSTable([]byte("88126\n 1\n")); len(got) != 0 {
		t.Errorf("the one-column form parsed as %d process(es): a command-less "+
			"row must not become a group member, or membersAreOurs would be "+
			"asked about an empty string", len(got))
	}
}

// TestATreeWithNoHandleIsHELDWhileAProcessStillNamesIt is fable's ruling at
// coop r10/5 -- a tree is never removed while anything still names it, by pgid
// or by argv -- and it is driven in the shape the field produced, which is the
// one a recorded handle cannot cover.
//
// MEASURED ON forge-01: group 88126's tree was absent at 03:49:0x and BORN
// AGAIN at 03:49:08, carrying a fresh profile dir and NO driver.pgid, because
// the surviving processes re-created it after the sweep removed it. So removal
// while the group lives does not just lose the handle; it manufactures a tree
// that reads exactly like one made before the record existed, and the next
// sweep removes it again. The only thing that still ties that tree to its
// processes is their argv.
func TestATreeWithNoHandleIsHELDWhileAProcessStillNamesIt(t *testing.T) {
	dir := t.TempDir()
	dead := deadPID(t)
	ghost := fmt.Sprintf("%s%d-3668960404", tmpdirPrefix, dead)
	if err := os.MkdirAll(filepath.Join(dir, ghost, "org.chromium.Chromium.scoped_dir.Dc0Fl8"), 0o755); err != nil {
		t.Fatalf("planting the re-created tree: %v", err)
	}
	// NO recordPGID CALL: that is the whole point of the shape.
	defer restoreReaper(swapReaper(
		func(int) ([]string, error) { return nil, nil },
		func(int, syscall.Signal) error { return nil },
	))
	defer func(old func() ([]procLine, error)) { listAllProcesses = old }(swapProcessTable(
		func() ([]procLine, error) {
			return []procLine{{pgid: 88126, command: strings.Replace(
				chromeArgvFromTheField, "87357-3668960404", ghost[len(tmpdirPrefix):], 1)}}, nil
		},
	))

	got := sweepStaleTmpdirsIn(dir)

	if _, err := os.Stat(filepath.Join(dir, ghost)); err != nil {
		t.Fatalf("a tree a RUNNING process still names was removed: %v -- the next "+
			"sweep then finds the same tree re-created with no handle, which is "+
			"the loop that kept group 88126 alive through 47 removals", err)
	}
	if got.held != 1 || got.removed != 0 {
		t.Errorf("sweep reported held=%d removed=%d, want 1 and 0", got.held, got.removed)
	}
	if got.nohandle != 1 {
		t.Errorf("the handle-less tree was counted nohandle=%d, want 1: a reader "+
			"must be able to tell this from a tree whose group was already gone",
			got.nohandle)
	}
	// THE NAME AND NOT ONLY THE COUNT. Asserting "HELD 1 tree(s)" alone passed
	// over a mutant that recorded no name at all, because that prefix renders
	// the COUNT and matches an empty name list just as well.
	line := sweepReport(got)
	if !strings.Contains(line, "HELD 1 tree(s)") || !strings.Contains(line, ghost) {
		t.Errorf("the sweep's line does not name the tree it held (%s): %q", ghost, line)
	}
	if len(got.heldTrees) != 1 || got.heldTrees[0] != ghost {
		t.Errorf("heldTrees=%v, want [%s]", got.heldTrees, ghost)
	}
}

// TestAStrandedGroupWITHAHandleIsReapedNotRecovered drives BOTH passes together
// and asserts which one did the work, which is the thing the two halves cannot
// show on their own.
//
// RUN #787 IS WHY IT EXISTS. I had the argv pass running first, so a group
// stranded by a cancelled run was claimed by argv before the tree pass ever
// read its driver.pgid -- chrome carries the owned tree in its own
// --user-data-dir, 32 of 33 members did, so argv always wins a race it is put
// first in. The field then reported "RECOVERED 1 group(s) from argv that no
// tree's handle claimed" for a group whose handle I had measured intact
// minutes before, and `reaped` could not move for the one population the
// recorded handle was built to serve. A counter that cannot move is not an
// instrument, and quill's r10/3 question -- does the handle path work in the
// field -- was unanswerable while the order hid it.
func TestAStrandedGroupWITHAHandleIsReapedNotRecovered(t *testing.T) {
	dir := t.TempDir()
	dead := deadPID(t)
	tree := fmt.Sprintf("%s%d-176799846", tmpdirPrefix, dead)
	if err := os.MkdirAll(filepath.Join(dir, tree), 0o755); err != nil {
		t.Fatalf("planting the stranded tree: %v", err)
	}
	recordPGID(filepath.Join(dir, tree), 35589)

	// THE GROUP IS REACHABLE BOTH WAYS AT ONCE, which is the whole point: its
	// tree records the pgid AND its member's argv names that tree, exactly as
	// a real stranded chrome does. Only the order decides which counter moves.
	listings := 0
	defer restoreReaper(swapReaper(
		func(int) ([]string, error) {
			listings++
			if listings > 1 {
				return nil, nil
			}
			return []string{chromeArgvFromTheField}, nil
		},
		func(int, syscall.Signal) error { return nil },
	))
	defer func(old func() ([]procLine, error)) { listAllProcesses = old }(swapProcessTable(
		func() ([]procLine, error) {
			return []procLine{{pgid: 35589, command: strings.Replace(
				chromeArgvFromTheField, "rumor-browser-87357-3668960404", tree, 1)}}, nil
		},
	))

	got := sweepAndReapIn(dir)

	if got.reaped != 1 {
		t.Errorf("reaped=%d, want 1: a group whose own tree records its pgid must "+
			"be taken by the HANDLE, or `reaped 0` in a job log means \"argv got "+
			"there first\" and not \"the handle failed\" -- the two readings this "+
			"head exists to separate (quill, coop r10/3)", got.reaped)
	}
	if got.recovered != 0 {
		t.Errorf("recovered=%d, want 0: argv must not preempt a handle that is "+
			"sitting right there in the tree", got.recovered)
	}
	// AND THE IDENTITY IS RECORDED, NOT JUST COUNTED. Asserting the counter
	// alone left a mutant alive: deleting the line that appends the pgid broke
	// no arm, because the report's own arm builds its tally BY HAND and so
	// tests the RENDERING while nothing tested the RECORDING. A count says work
	// happened; this is the cell that says what it happened to.
	if len(got.reapedGroups) != 1 || got.reapedGroups[0] != 35589 {
		t.Errorf("reapedGroups=%v, want [35589]: the sweep counted the reap and "+
			"did not record WHICH group, so its line cannot name one", got.reapedGroups)
	}
}

// TestTheSweepsLineNAMESWhatItRecoveredHeldAndRefused closes the gap the field
// reading left open. #787's line said "RECOVERED 1 group(s) from argv that no
// tree's handle claimed" and named nothing, so tying that recovery to the group
// a cancelled run had stranded rested on my own wall-clock measurements and
// could not be re-derived from the log by anyone else (ibis, coop r10/15). A
// count says work happened; a name says what it happened to.
func TestTheSweepsLineNAMESWhatItRecoveredHeldAndRefused(t *testing.T) {
	line := sweepReport(sweepTally{
		reaped: 1, reapedGroups: []int{96441},
		recovered: 1, recoveredGroups: []int{35589},
		held: 1, heldTrees: []string{"rumor-browser-12502-176799846"},
		refused: 1, refusedGroups: []int{4242},
	})
	// pgid 96441 IS THE ONE A CLEAR IS ASKED FOR: fable's done-when is "the next
	// run's sweep saying reaped 1", and 96441 is the group a superseded run
	// actually stranded at 05:54:23Z with its handle intact. The first naming
	// pass named recovered, refused and held and left reaped anonymous, so the
	// figure the clear turns on could not be tied to a group from the log.
	for _, want := range []string{"pgid 96441", "pgid 35589",
		"rumor-browser-12502-176799846", "pgid 4242"} {
		if !strings.Contains(line, want) {
			t.Errorf("the sweep's line omits %q, so a reader cannot say WHICH: %q", want, line)
		}
	}

	// BOUNDED, AND THE ELIDED COUNT IS DERIVED. One field sweep met 47 stale
	// trees; a line that named every identity would be one nobody reads. The
	// "and N more" comes from len(), so it cannot drift the way a written
	// figure does (quill, coop r10/13).
	many := sweepTally{recovered: 6, recoveredGroups: []int{1, 2, 3, 4, 5, 6}}
	if got := sweepReport(many); !strings.Contains(got, "and 2 more") {
		t.Errorf("a six-group recovery did not elide with a derived count: %q", got)
	}
	if strings.Contains(sweepReport(many), "pgid 5") {
		t.Errorf("the line named past its bound: %q", sweepReport(many))
	}

	// AND A SWEEP WITH NOTHING TO SAY STILL SAYS NOTHING, so the identities
	// cannot turn the quiet path noisy.
	if got := sweepReport(sweepTally{kept: 3}); got != "" {
		t.Errorf("a sweep that did nothing said %q", got)
	}
}

// TestAnUnreadableProcessTableHoldsEverythingAndSaysSo is the arm for the
// defect run #790 could have been: psAllProcesses used to answer nil on error,
// and nil is indistinguishable from a box running nothing. reapGroup read that
// as reapEmpty -- whose own comment calls it benign and ordinary -- and
// treeStillClaimed looped over nothing and said no process claims this tree.
// So ONE failed exec could delete a live group's tree and report `reaped 0`,
// which is precisely the pair of lines #790 printed (ibis, coop r10/20-21).
func TestAnUnreadableProcessTableHoldsEverythingAndSaysSo(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, fmt.Sprintf("%s%d-blind", tmpdirPrefix, deadPID(t)))
	if err := os.MkdirAll(stale, 0o755); err != nil {
		t.Fatalf("planting the tree: %v", err)
	}
	recordPGID(stale, 4242)

	blind := errors.New("ps: cannot fork")
	var signalled []int
	defer restoreReaper(swapReaper(
		func(int) ([]string, error) { return nil, blind },
		func(pgid int, _ syscall.Signal) error { signalled = append(signalled, pgid); return nil },
	))
	defer func(old func() ([]procLine, error)) { listAllProcesses = old }(swapProcessTable(
		func() ([]procLine, error) { return nil, blind },
	))

	got := sweepAndReapIn(dir)

	if _, err := os.Stat(filepath.Join(stale, pgidFile)); err != nil {
		t.Fatalf("a tree was deleted on a reading that COULD NOT BE TAKEN: %v -- the "+
			"old shape answered nil here and the sweep read it as an empty box", err)
	}
	if got.removed != 0 || got.reaped != 0 || got.recovered != 0 {
		t.Errorf("removed=%d reaped=%d recovered=%d, want 0, 0 and 0: nothing may be "+
			"collected or signalled on a table nobody could read",
			got.removed, got.reaped, got.recovered)
	}
	if len(signalled) != 0 {
		t.Errorf("signals went to %v on an unreadable table", signalled)
	}
	if got.unreadable == 0 {
		t.Error("the sweep did not count the unreadable reading, so its line cannot " +
			"distinguish \"I could not look\" from \"there was nothing there\"")
	}
	if line := sweepReport(got); !strings.Contains(line, "COULD NOT READ the process table") {
		t.Errorf("the sweep's line does not say it was blind: %q", line)
	}
	// AND THE LINE NAMES THE PROCESS THAT SWEPT, so several binaries' lines in
	// one batched job log can be told apart -- the thing #790's two lines could
	// not be, which left a race and a ps failure indistinguishable.
	if line := sweepReport(got); !strings.Contains(line, fmt.Sprintf("pid %d ", os.Getpid())) {
		t.Errorf("the sweep's line does not name the sweeper: %q", line)
	}
}

// TestEachBlindReadingIsGUARDEDSEPARATELY exists because one fixture could not
// tell the guards apart. With BOTH readings failing and a tree that records a
// pgid, the tree is held by the per-group branch and `unreadable` is counted by
// the argv pass -- so deleting either of the other two guards changed nothing
// the first arm asserts, and two mutants lived through it. Each case below is
// shaped so exactly one guard can satisfy it.
func TestEachBlindReadingIsGUARDEDSEPARATELY(t *testing.T) {
	blind := errors.New("ps: cannot fork")

	// ONE: the PER-GROUP reading fails while the whole table reads fine. Only
	// reapGroup's own error branch can see this, so reapUnreadable must be
	// counted here or the outcome renders as the benign reapEmpty.
	t.Run("the per-group reading fails", func(t *testing.T) {
		dir := t.TempDir()
		stale := filepath.Join(dir, fmt.Sprintf("%s%d-onlygroup", tmpdirPrefix, deadPID(t)))
		if err := os.MkdirAll(stale, 0o755); err != nil {
			t.Fatalf("planting: %v", err)
		}
		recordPGID(stale, 4242)
		defer restoreReaper(swapReaper(
			func(int) ([]string, error) { return nil, blind },
			func(int, syscall.Signal) error { return nil },
		))
		defer func(old func() ([]procLine, error)) { listAllProcesses = old }(swapProcessTable(
			func() ([]procLine, error) { return nil, nil },
		))
		got := sweepStaleTmpdirsIn(dir)
		if got.unreadable != 1 {
			t.Errorf("unreadable=%d, want 1: the TREE pass read an error from the "+
				"group listing and did not count it, so the outcome renders as "+
				"reapEmpty, which this package documents as benign and ordinary",
				got.unreadable)
		}
		if got.removed != 0 {
			t.Errorf("removed=%d, want 0", got.removed)
		}
	})

	// TWO: the WHOLE-TABLE reading fails and the tree records NO pgid, so the
	// per-group branch cannot fire and only treeStillClaimed's own error branch
	// stands between a live group's tree and deletion.
	t.Run("the whole-table reading fails with no handle to fall back on", func(t *testing.T) {
		dir := t.TempDir()
		ghost := fmt.Sprintf("%s%d-nohandle", tmpdirPrefix, deadPID(t))
		if err := os.MkdirAll(filepath.Join(dir, ghost), 0o755); err != nil {
			t.Fatalf("planting: %v", err)
		}
		defer restoreReaper(swapReaper(
			func(int) ([]string, error) { return nil, nil },
			func(int, syscall.Signal) error { return nil },
		))
		defer func(old func() ([]procLine, error)) { listAllProcesses = old }(swapProcessTable(
			func() ([]procLine, error) { return nil, blind },
		))
		got := sweepStaleTmpdirsIn(dir)
		if _, err := os.Stat(filepath.Join(dir, ghost)); err != nil {
			t.Fatalf("the tree was deleted on an unreadable table: %v -- this is the "+
				"re-created ghost shape, whose only tie to its processes is argv", err)
		}
		if got.held != 1 || got.removed != 0 {
			t.Errorf("held=%d removed=%d, want 1 and 0", got.held, got.removed)
		}
	})
}
