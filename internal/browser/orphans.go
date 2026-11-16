package browser

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// THIS FILE IS THE OTHER HALF OF THE OWNED TMPDIR, and the half that was
// missing. Start's comment explains why the driver gets a TMPDIR this process
// made; removing it lives in stopProcess, which is reached from Close and from
// one Start failure. NOTHING REACHES IT WHEN THE PROCESS IS SIGNALLED, because
// Go's default disposition for SIGTERM and SIGINT is to die without running
// anything -- and there was no signal.Notify in cmd/rumor or internal/browser,
// so the polite kill was the unhandled path.
//
// THAT IS THE COMMON PATH AND NOT THE EXOTIC ONE: CI runs the suite under
// `timeout 1800`, which sends SIGTERM first, and a cancelled forge run is the
// same shape. quill drove it (coop r9/401) with a two-process probe, because a
// defer has to be tested from outside the process that would run it:
//
//	owned dir made and handed over   /tmp/rumor-browser-1434395202
//	SIGKILL the rumor-side process   DIR SURVIVED
//	and it orphaned                  1 chromedriver + 1 chrome, still running
//
// MEASURED ON forge-01 2026-11-16, which is what the leak costs when it runs
// for a day: 227 `chrome:` processes and 7 chromedrivers alive in the ci jail,
// every one of them between 1.7 and 5.3 hours old and none belonging to the
// job that was running. The host is 16 GiB and its swap was 96% IN USE with
// 5 GiB in Laundry. A chromedriver on that box cannot answer POST /session
// inside RequestBound, so tests/run's browser arm failed on two consecutive
// heads (tasks 1240 and 1242) with "NOT DRIVABLE" -- a red that is about the
// orphans and not about the code under test.
//
// SO THE DIRECTORY LEAK AND THE PROCESS LEAK ARE TWO FAULTS, and the owned
// TMPDIR only ever fixed the first. Task 1242 is the proof: it ran with the
// owned TMPDIR in place and failed identically, because the box was already
// full of the previous runs' browsers.
//
// TWO MECHANISMS HERE, AND THEY COVER DIFFERENT SIGNALS:
//
//	signal.Notify   SIGTERM/SIGINT/SIGHUP -- runs the real teardown, so the
//	                browsers GO. This is the only one that reclaims processes.
//	a startup sweep SIGKILL cannot be caught by anything, so the dirs it
//	                strands are reclaimed by the next run instead.
//
// AND THE SWEEP REAPS, IT DOES NOT ONLY REMOVE, which is the difference
// between the clause being met and looking met (quill, coop r9/405). A sweep
// that deleted directories would fix the harmless half twice over: a SIGKILLed
// run's ~33 chrome processes would still be alive, the next run would inherit
// them, and the loop above would not be broken. So each owned tree carries the
// driver's PGID, written at Start, and the sweep signals that group before it
// removes the tree.
//
// THE PGID AND NOT THE PID IN THE NAME. The name's pid is RUMOR'S, and a dead
// rumor pid says nothing about where its orphans went -- the group is the one
// handle that still reaches them once their maker is gone. It is also what
// keeps the reaping safe: the pgid came from a process we started, so it
// cannot name a neighbour's, and a group that no longer exists signals
// nothing. A pid can be RECYCLED, though, so the group is checked to be ours
// by what is running in it before it is signalled -- see reapGroup.

// ownedTmpdir makes the tree this process hands to a driver as TMPDIR.
//
// THE PID IS IN THE NAME SO THE SWEEP CAN BE SAFE BY CONSTRUCTION. Two rumor
// runs share the ci jail, so a sweep that matched `rumor-browser-*` and removed
// what it found would delete a CONCURRENT run's live tree -- quill's condition
// on the second fix, and the reason the name carries the maker rather than the
// sweep guessing from a timestamp.
func ownedTmpdir() (string, error) {
	return os.MkdirTemp("", fmt.Sprintf("%s%d-", tmpdirPrefix, os.Getpid()))
}

// tmpdirPrefix is the one spelling of the name, so the maker and the sweep
// cannot drift apart. It is deliberately ours and specific: the removal must
// never be able to reach another tenant's files, and isolex had 9,041 entries
// beside ours in the ci jail.
const tmpdirPrefix = "rumor-browser-"

// ownerOfTmpdir reads the pid back out of a name ownedTmpdir made, and reports
// false for anything that is not one of ours.
//
// IT IS STRICT ON PURPOSE. `rumor-browser-probe` is an ACTOR name in
// cmd/rumor, not a directory, but a loose match is how a sweep starts deleting
// things it was never meant to see: the name must be the prefix, then digits,
// then the separator MkdirTemp's random suffix follows.
func ownerOfTmpdir(name string) (int, bool) {
	if !strings.HasPrefix(name, tmpdirPrefix) {
		return 0, false
	}
	rest := strings.TrimPrefix(name, tmpdirPrefix)
	dash := strings.IndexByte(rest, '-')
	if dash <= 0 {
		return 0, false
	}
	pid, err := strconv.Atoi(rest[:dash])
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}

// aliveElsewhere reports whether a pid belongs to a process that still exists.
//
// SIGNAL 0 AND NOT A NAME MATCH. quill's instrument note from the same probe:
// `pgrep chrome` matches chromedriver too, so a reaper keyed on the name kills
// the driver while reporting it found a browser -- one pid, printed twice.
// This asks the kernel about a pid we wrote down ourselves instead.
//
// EPERM COUNTS AS ALIVE: the pid exists and belongs to someone else, which is
// the case where removing its tree would be the worst thing to do.
func aliveElsewhere(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// sweepStaleTmpdirs removes owned trees whose maker is gone, and answers what
// it did so an arm can assert on it rather than on the filesystem's mood.
//
// A MISSING OR UNREADABLE TempDir IS NOT AN ERROR HERE. This runs for its side
// effect on the way into a run that has real work to do; failing the run
// because a sweep could not read a directory would trade a reclaimed
// directory for a lost suite.
func sweepStaleTmpdirs() (removed, kept int) {
	return sweepStaleTmpdirsIn(os.TempDir())
}

// sweepStaleTmpdirsIn is the sweep over a named directory, so an arm can drive
// it against a tree it made rather than against the box's real /tmp -- where
// the honest answer depends on what else is running and a passing arm would
// prove nothing.
func sweepStaleTmpdirsIn(dir string) (removed, kept int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0
	}
	self := os.Getpid()
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, ok := ownerOfTmpdir(e.Name())
		if !ok {
			continue
		}
		if pid == self || aliveElsewhere(pid) {
			kept++
			continue
		}
		tree := filepath.Join(dir, e.Name())
		// REAP BEFORE REMOVE, and in that order for the reason stopProcess
		// gives: removing the tree while chrome still held files there would
		// be deleting a live process's state.
		reapGroup(readPGID(tree))
		if err := os.RemoveAll(tree); err == nil {
			removed++
		}
	}
	return removed, kept
}

// pgidFile is where an owned tree records the group its driver leads, so a
// later run can reach that group after the maker is gone.
const pgidFile = "driver.pgid"

// recordPGID writes the driver's group into its own tree. A failure here is
// not fatal to the run: it costs a FUTURE sweep its handle on these processes,
// which is worth a weaker guarantee rather than refusing to drive a browser.
func recordPGID(tree string, pgid int) {
	_ = os.WriteFile(filepath.Join(tree, pgidFile), []byte(strconv.Itoa(pgid)), 0o644)
}

// readPGID answers the group an owned tree recorded, or 0 for a tree that
// recorded none -- which is every tree made before this file existed, and is
// why the reaper treats 0 as "nothing to do" rather than as an error.
func readPGID(tree string) int {
	raw, err := os.ReadFile(filepath.Join(tree, pgidFile))
	if err != nil {
		return 0
	}
	pgid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || pgid <= 1 {
		return 0
	}
	return pgid
}

// reapGroup stops the browsers a dead run left behind.
//
// IT CHECKS THE GROUP IS STILL OURS FIRST, because a pid can be recycled: the
// group we wrote down may since have been handed to something unrelated, and
// signalling that would be this package reaching a stranger. The test is what
// is RUNNING in the group -- every member must be a chromedriver or a chrome,
// and there must be at least one. A recycled group fails that and is left
// alone; an empty group needs nothing.
//
// POLITE THEN FINAL, the same bargain stopProcess makes, and against the same
// hazard: a browser mid-teardown can outlive the first signal.
//
// REFUSING IS A MISS AND NOT A HAZARD, which is the property worth being clear
// about: if the check is wrong the group is LEFT, never wrongly signalled, so
// the guard bounds completeness rather than safety. The two worlds a stale
// tree can be in:
//
//	orphans alive   the group exists, so its id cannot be handed to anyone
//	                else, the members are all ours, and the reap fires
//	group emptied   the id is reusable and may now read foreign -- and there
//	                is nothing left to reap, so collecting the tree is right
//
// The harmful case needs live orphans AND a reused group id at once. That is
// what the kernel forbids, and the second world needs no measurement at all
// (quill, coop r9/409, withdrawing their own residual).
//
// WHAT IS DELIBERATELY NOT HERE: a reap of groups this process never
// recorded. It was proposed and refused on a measurement (quill r9/411, ibis
// r9/412, withdrawn r9/413). The ci jail has ONE unix user for every repo's
// jobs -- 236 of 243 processes in jid 2 are act_runner -- and a neighbour with
// 9,043 /tmp entries beside ours. So "a browser group no tree of ours claims"
// does not mean stray; it also means that neighbour's LIVE browsers, same
// user and same binaries. That substitutes ours-BY-NAME for ours-BY-
// CONSTRUCTION, which is the instrument that misread this leak twice.
//
// THE SAFE FORM, IF IT IS EVER WANTED, IS ARGV AND NOT A NAME: with TMPDIR
// owned, chrome's --user-data-dir sits INSIDE a tree we made, so a stray can
// prove its own provenance with no name test and even after its tree is gone.
// It is not built here because widening what this tool may kill is a decision
// about the tool and not a refactor. It also cannot reach the strays that
// already exist: of the 234 in the jail, 0 named a tree of ours, 226 named
// /tmp/org.chromium.*, and the 7 drivers carried no data-dir at all. Code
// cannot own what was made before the provenance did, so draining those is a
// one-time act by a human rather than anything this file should attempt.
//
// RECORDED AS AN ASSUMPTION WITH ITS SCOPE, because "closed" would quietly
// inherit a box it was never measured on. MEASURED on FreeBSD 15.1-STABLE at
// kern.randompid=0: a group whose leader was reaped but which still held a
// member kept its id across 163,095 forks and TWO FULL WRAPS of the 99,999 pid
// space -- the allocator passed it twice and skipped it both times. RELIED
// UPON, UNMEASURED, on 15.1-RELEASE-p2 at kern.randompid=313, which is CI's
// box: randompid=0 is what made the skip legible as a sequential gap, so the
// reading cannot be taken there the same way, and no kernel source is on
// either box to cite the allocator from instead.
func reapGroup(pgid int) {
	if pgid <= 1 || !groupIsOurs(pgid) {
		return
	}
	_ = signalGroupOf(pgid, syscall.SIGTERM)
	for i := 0; i < int(StopGrace/(100*time.Millisecond)); i++ {
		if !groupHasMembers(pgid) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = signalGroupOf(pgid, syscall.SIGKILL)
}

// THE TWO SYSCALLS ARE REACHED THROUGH VARIABLES so that the decisions above
// can be driven. What this code decides -- is this group ours, does it get a
// signal, which one, and in what order -- is the whole of it, and none of that
// is observable from a test that can neither conjure a real chromedriver nor
// be allowed to signal a real group to find out. Substituting these lets an
// arm assert the DECISION; whether a live browser then dies is the real pair's
// question and quill drives it in a jail that has one.
var (
	listGroupMembers = psGroupMembers
	signalGroupOf    = func(pgid int, sig syscall.Signal) error {
		return syscall.Kill(-pgid, sig)
	}
)

// groupMembers answers the command of every process in a group.
//
// ONE -o PER KEYWORD: `-o a=,b=` is ONE column on this ps, whose header is
// everything after the first `=`, so the comma form silently answers a list
// that cannot contain what this is looking for. browser_test.go's own leak
// check carries the same note, and it cost six wrong readings to relearn.
func psGroupMembers(pgid int) []string {
	out, err := exec.Command("ps", "-axww", "-o", "pgid=", "-o", "command=").Output()
	if err != nil {
		return nil
	}
	var members []string
	want := strconv.Itoa(pgid)
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 || f[0] != want {
			continue
		}
		members = append(members, strings.TrimSpace(line[len(f[0]):]))
	}
	return members
}

func groupHasMembers(pgid int) bool { return len(listGroupMembers(pgid)) > 0 }

// groupIsOurs holds when a group is non-empty and everything in it is a
// browser or its driver. A single foreign member is enough to refuse: the
// cost of refusing is leaked processes a human can still see, and the cost of
// being wrong is signalling someone else's work.
func groupIsOurs(pgid int) bool {
	members := listGroupMembers(pgid)
	if len(members) == 0 {
		return false
	}
	for _, m := range members {
		if !strings.HasPrefix(m, "chrome:") && !strings.Contains(m, "chromedriver") &&
			!strings.Contains(m, "geckodriver") && !strings.Contains(m, "firefox") {
			return false
		}
	}
	return true
}

var (
	liveMu      sync.Mutex
	liveDrivers = map[*Driver]struct{}{}

	handlerOnce sync.Once
	sweepOnce   sync.Once
)

// rememberDriver puts a started driver where the signal handler can find it,
// and arms that handler the first time there is anything to lose.
func rememberDriver(d *Driver) {
	liveMu.Lock()
	liveDrivers[d] = struct{}{}
	liveMu.Unlock()
	handlerOnce.Do(installTerminationHandler)
}

// forgetDriver drops a driver that has already been stopped, so the handler
// does not stop it twice and does not hold it alive in the map.
func forgetDriver(d *Driver) {
	liveMu.Lock()
	delete(liveDrivers, d)
	liveMu.Unlock()
}

// sweepOnFirstStart runs the startup sweep exactly once per process, on the way
// into the first Start, which is the only moment that is both early enough to
// matter and late enough to know this process drives browsers at all.
func sweepOnFirstStart() {
	sweepOnce.Do(func() { _, _ = sweepStaleTmpdirs() })
}

// installTerminationHandler makes the polite kill run the real teardown.
//
// A LIBRARY TAKING SIGNALS IS INTRUSIVE AND IS THE RIGHT CALL HERE, narrowly:
// this package starts processes that outlive their parent by design -- a
// webdriver spawns a browser into its own group -- so the knowledge of what
// must be stopped lives here and nowhere else. Nothing is being taken from a
// caller that wanted it: there was no signal.Notify anywhere in this repo.
//
// AND IT RE-RAISES RATHER THAN EXITING. Resetting the disposition and sending
// the same signal to ourselves means the process still dies the way the sender
// asked -- killed by SIGTERM, not exited 0 or 1 -- so `timeout`, a shell's job
// control and the forge runner all read the outcome they would have read
// before. An os.Exit here would turn every cancelled CI job into a clean exit
// and hide the cancellation.
func installTerminationHandler() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		s, ok := <-ch
		if !ok {
			return
		}
		stopEveryLiveDriver()
		signal.Stop(ch)
		sig, ok := s.(syscall.Signal)
		if !ok {
			os.Exit(1)
		}
		signal.Reset(sig)
		_ = syscall.Kill(os.Getpid(), sig)
	}()
}

// stopEveryLiveDriver tears down whatever is still running, taking the list
// under the lock and releasing it before the slow part: stopProcess waits on a
// reap, and holding the registry lock across that would deadlock any driver
// trying to deregister itself at the same moment.
func stopEveryLiveDriver() {
	liveMu.Lock()
	ds := make([]*Driver, 0, len(liveDrivers))
	for d := range liveDrivers {
		ds = append(ds, d)
	}
	liveDrivers = map[*Driver]struct{}{}
	liveMu.Unlock()
	for _, d := range ds {
		d.stopProcess()
	}
}
