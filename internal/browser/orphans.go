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
	// A NAME AND NOT A PATH OR A COMMAND LINE. Without this, the prefix and the
	// first `-` were the whole test, so every one of these read as ours with
	// pid 87357 -- driven, 2026-11-17, at quill's finding (coop r10/11):
	//
	//	rumor-browser-87357-3668960404/org.chromium.Chromium.scoped_dir.ug4m8j
	//	rumor-browser-87357-3668960404 --enable-logging
	//	rumor-browser-87357-3668960404<TAB>x
	//
	// treeInArgv's truncation was the ONLY thing keeping a path out, and this
	// function's own doc said it applied a strict test of its own. It does now:
	// a caller that forgets to cut the subdirectory gets a refusal rather than
	// a tree name that matches nothing.
	if strings.ContainsAny(rest, "/ \t") {
		return 0, false
	}
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
func sweepStaleTmpdirs() sweepTally {
	return sweepAndReapIn(os.TempDir())
}

// sweepAndReapIn runs both passes over a named directory, in the order that
// keeps each counter meaningful, so an arm can drive the PAIR rather than only
// its halves.
//
// THE RECORDED HANDLE GOES FIRST, AND THE FIELD HAD TO TEACH ME THAT. I ran
// argv first, reasoning that a reaped group's tree could then be collected in
// the same pass. The consequence, measured in run #787: a stranded group is
// ALWAYS claimed by argv before the tree pass can read its handle, because
// chrome puts the owned tree in its own --user-data-dir -- 32 of 33 members
// did. So `reaped` became unreachable for exactly the population the handle
// was built for, and #787 reported "RECOVERED 1 group(s) from argv" for a
// group whose driver.pgid I had measured intact minutes earlier.
//
// THAT HID THE QUESTION quill ASKED AT r10/3. Whether the handle path works in
// the field is answerable only if the handle is tried first; with argv ahead of
// it, `reaped 0` means "argv got there first" and not "the handle failed", and
// those are the two readings this whole head exists to separate.
func sweepAndReapIn(dir string) sweepTally {
	t := sweepStaleTmpdirsIn(dir)
	// AND ARGV SECOND, for what no tree's handle claimed: a tree an earlier
	// sweep destroyed, or one that recorded nothing. The removal gate has
	// already held any tree whose group is still alive, so a group reaped here
	// leaves its tree for the next sweep to collect.
	t.absorb(reapOrphansByArgv())
	return t
}

// sweepTally is what one sweep did, by outcome and with every outcome named.
//
// A STRUCT AND NOT MORE RETURN VALUES, because the thing that went wrong in
// the field was an outcome NOBODY COUNTED. reapNoHandle and reapEmpty both
// landed in the gap between `reaped` and `refused`, so a sweep that dropped
// the handle on a live group and a sweep with nothing to do printed the same
// sentence. A named field per outcome makes the next addition say its name in
// the report or not compile.
type sweepTally struct {
	removed    int // trees deleted
	reaped     int // groups signalled from the handle their own tree recorded
	recovered  int // groups signalled from argv, no tree's handle claiming them
	refused    int // groups LEFT ALIVE because they are not wholly ours
	nohandle   int // stale trees that recorded no pgid at all
	held       int // trees something running still names, so they were not removed
	kept       int // trees whose maker is still alive
	unreadable int // times the process table could not be read at all

	// AND THE IDENTITIES FOR THE OUTCOMES A READER HAS TO CHASE. The counts
	// alone cost this head a whole night: #787 reported "RECOVERED 1 group(s)"
	// and nothing in the line said WHICH, so attributing it to the group a
	// cancelled run had stranded rested on my own wall-clock notes and could
	// not be re-derived by anyone else from the log (ibis, coop r10/15). A
	// number says work happened; a name says what it happened to.
	//
	// ONLY THE RARE OUTCOMES. removed, nohandle and kept are bulk -- 47 in one
	// field sweep -- and naming them would bury the two that mean something.
	reapedGroups    []int    // pgids taken by the handle their own tree recorded
	recoveredGroups []int    // pgids argv reached that no handle claimed
	refusedGroups   []int    // pgids left ALIVE, members not wholly ours
	heldTrees       []string // trees kept because something still names them
}

func (t *sweepTally) absorb(o sweepTally) {
	t.removed += o.removed
	t.reaped += o.reaped
	t.recovered += o.recovered
	t.refused += o.refused
	t.nohandle += o.nohandle
	t.held += o.held
	t.kept += o.kept
	t.unreadable += o.unreadable
	t.reapedGroups = append(t.reapedGroups, o.reapedGroups...)
	t.recoveredGroups = append(t.recoveredGroups, o.recoveredGroups...)
	t.refusedGroups = append(t.refusedGroups, o.refusedGroups...)
	t.heldTrees = append(t.heldTrees, o.heldTrees...)
}

// nameSome renders identities for a report line, bounded so one pathological
// sweep cannot produce a line nothing will read.
//
// THE ELIDED COUNT IS DERIVED AND NOT WRITTEN DOWN, which is the difference
// between this and the figures quill retired at coop r10/13: a hand-maintained
// total goes stale silently, while len() cannot.
func nameSome[T any](xs []T, limit int) string {
	if len(xs) == 0 {
		return ""
	}
	shown := xs
	if len(shown) > limit {
		shown = shown[:limit]
	}
	parts := make([]string, 0, len(shown))
	for _, x := range shown {
		parts = append(parts, fmt.Sprint(x))
	}
	out := strings.Join(parts, ", ")
	if rest := len(xs) - len(shown); rest > 0 {
		out += fmt.Sprintf(" and %d more", rest)
	}
	return out
}

// sweepStaleTmpdirsIn is the sweep over a named directory, so an arm can drive
// it against a tree it made rather than against the box's real /tmp -- where
// the honest answer depends on what else is running and a passing arm would
// prove nothing.
func sweepStaleTmpdirsIn(dir string) sweepTally {
	var t sweepTally
	entries, err := os.ReadDir(dir)
	if err != nil {
		return t
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
			t.kept++
			continue
		}
		tree := filepath.Join(dir, e.Name())
		// REAP BEFORE REMOVE, and in that order for the reason stopProcess
		// gives: removing the tree while chrome still held files there would
		// be deleting a live process's state.
		pgid := readPGID(tree)
		switch reapGroup(pgid) {
		case reapSignalled:
			t.reaped++
			t.reapedGroups = append(t.reapedGroups, pgid)
		case reapRefused:
			// COUNTED SEPARATELY, because this is the outcome a reader must
			// not mistake for "nothing needed doing": the group is RUNNING.
			t.refused++
			t.refusedGroups = append(t.refusedGroups, pgid)
		case reapNoHandle:
			// COUNTED, because a tree with no handle must be legible as that
			// and not as a reap. The argv pass is what reaches its group.
			t.nohandle++
		case reapUnreadable:
			// NOTHING IS KNOWN about this group, which is neither a reap nor a
			// refusal nor an empty group. The gate below holds the tree,
			// because treeStillClaimed answers the same unreadable table.
			t.unreadable++
		}
		// A TREE IS NEVER REMOVED WHILE ANYTHING STILL NAMES IT, by pgid or by
		// argv (fable, coop r10/5). Removing it deletes driver.pgid, the only
		// handle, while the group is still running -- which is what task 1254
		// did to group 88126 and is how a leak stops being reclaimable.
		//
		// AND THE TREE COMES BACK IF YOU DO IT ANYWAY. Measured on forge-01:
		// 88126's tree was absent at 03:49:0x and BORN AGAIN at 03:49:08, with
		// a fresh profile dir and no driver.pgid, re-created by the survivors
		// themselves. So the removal does not merely lose the handle, it
		// manufactures a tree indistinguishable from one made before the
		// record existed -- which the next sweep removes again, forever.
		if treeStillClaimed(e.Name(), pgid) {
			t.held++
			t.heldTrees = append(t.heldTrees, e.Name())
			continue
		}
		if err := os.RemoveAll(tree); err == nil {
			t.removed++
		}
	}
	return t
}

// treeStillClaimed reports whether anything running still refers to this tree:
// the group its own record names, or a command line naming the directory.
//
// THE TWO QUESTIONS ARE NOT THE SAME ONE. A group can hold the tree with no
// handle on disk (the re-created tree above), and a handle can name a group
// whose members carry no data-dir in argv -- a chromedriver's own command line
// is just `chromedriver --port=N`. Either claim is enough to hold the tree.
//
// AND THE PRICE OF THIS RULE IS A TREE THAT IS NEVER COLLECTED, which is
// DELIBERATE and is not a defect to fix later (quill, coop r10/6). If a
// recorded pgid is recycled onto a foreign process, the pgid check matches a
// stranger and the directory stays forever. That is the safe direction by a
// wide margin: an uncollectable 600 KB directory costs nothing, and deleting a
// live group's only handle cost 4.70 GiB of resident chrome in task 1253. A
// reader who finds an ancient tree here is looking at the price, not the bug.
func treeStillClaimed(name string, pgid int) bool {
	if pgid > 1 {
		members, err := listGroupMembers(pgid)
		// CANNOT LOOK COUNTS AS CLAIMED. The alternative is what shipped
		// before: a failed ps answered "nothing claims this tree" and the
		// sweep deleted it while the group ran.
		if err != nil || len(members) > 0 {
			return true
		}
	}
	procs, err := listAllProcesses()
	if err != nil {
		return true
	}
	for _, proc := range procs {
		if _, named, ok := treeInArgv(proc.command); ok && named == name {
			return true
		}
	}
	return false
}

// reapOrphansByArgv stops browser groups whose own tree can no longer speak
// for them, finding them by the owned tree their command line still names.
//
// IT IS NOT A BACKUP TO THE RECORDED PGID, IT IS THE OTHER HALF. For every
// tree an earlier sweep already destroyed, this is the ONLY handle: the pgid
// lived in the tree, and the tree is gone (quill, coop r10/8, correcting their
// own r9/413). argv is carried by the PROCESSES, so it survives both the
// tree's destruction and its rebirth. It runs AFTER the recorded handle, for
// the reason sweepAndReapIn gives: ahead of it, argv claims every group first
// and `reaped` stops being able to move.
//
// THE CYCLE IT BREAKS, which is why no field sweep ever reaped anything. A
// sweep deletes a live group's tree, taking the handle with it. The survivors
// RECREATE that path, because it is their TMPDIR and the next scoped_dir needs
// it. The reborn tree carries no driver.pgid BY CONSTRUCTION, since only Start
// writes one. So the next sweep reads no handle, removes the tree, counts
// reaped 0 -- and the tree is born again. Every "reaped 0" in the field was
// taken over these zombie trees and not over the originals, which is also why
// the reap fired in quill's arm and never once in CI: an arm has no earlier
// sweep to destroy the handle first.
//
// AND IT TERMINATES. argv claims the zombie tree, so the removal gate holds it;
// argv finds the group, so this pass reaps it; once the processes are gone
// nothing claims the tree and the next sweep collects it. The end state is
// reachable rather than a steady state of removals.
//
// OURS BY CONSTRUCTION, WHICH IS THE WHOLE LICENSE FOR IT: the path it matches
// on is a directory os.MkdirTemp made under this package's own prefix, with
// the maker's pid in the name. A neighbour's browser in the same jail, under
// the same unix user, running the same binaries, cannot have one in its argv
// unless rumor gave it one. That is the distinction the name test could not
// make, and refusing a group whose members are not all browsers still applies
// on top of it.
func reapOrphansByArgv() sweepTally {
	var t sweepTally
	groups, err := argvOrphanGroups()
	if err != nil {
		t.unreadable++
		return t
	}
	for pgid := range groups {
		switch reapGroup(pgid) {
		case reapSignalled:
			t.recovered++
			t.recoveredGroups = append(t.recoveredGroups, pgid)
		case reapRefused:
			t.refused++
			t.refusedGroups = append(t.refusedGroups, pgid)
		}
	}
	return t
}

// argvOrphanGroups answers the groups that name an owned tree whose maker is
// gone, one entry per group.
//
// THE MAKER'S PID IS THE LIVENESS TEST, exactly as it is for a tree on disk: a
// CONCURRENT run's browsers name ITS tree, whose pid is alive, so they are
// skipped. This is the same guard that keeps the tree sweep from deleting a
// live run's directory, applied to the same name from the other side.
func argvOrphanGroups() (map[int]string, error) {
	self := os.Getpid()
	found := map[int]string{}
	procs, err := listAllProcesses()
	if err != nil {
		// NOTHING IS RECOVERED WHEN NOTHING CAN BE READ, which is the safe
		// direction here: this pass only ever SIGNALS, so an unreadable table
		// must yield no targets rather than a guess -- and the error goes UP,
		// so the report can say the pass was blind instead of saying it found
		// nothing.
		return found, err
	}
	for _, proc := range procs {
		if proc.pgid <= 1 {
			continue
		}
		owner, name, ok := treeInArgv(proc.command)
		if !ok || owner == self || aliveElsewhere(owner) {
			continue
		}
		found[proc.pgid] = name
	}
	return found, nil
}

// treeInArgv answers the owned tree a command line names, and false for a
// command line that names none.
//
// THE MATCH IS ON THE TREE, NOT ON THE PATH, which is the hazard quill named
// at coop r10/8: what argv carries is a path UNDER the tree, so a check that
// compared the whole string would claim nothing and read as "no process names
// it" -- the same silence as a missing handle. This extracts the directory
// name and the callers compare that, so the subdirectory is irrelevant.
//
// AND THE TRUNCATION IS NO LONGER THE ONLY GUARD. It was, which this comment
// used to deny by pointing at ownerOfTmpdir's "strict test" (quill, coop
// r10/11): that function took the prefix and the first `-` and accepted a
// slash, a space or a tab in between. It refuses them now, so a caller that
// skips the cut is told, instead of being handed a name that matches nothing.
//
// IT TAKES THE DIRECTORY NAME AND NOT THE WHOLE PATH, because what chrome
// carries is a path INSIDE the tree -- `--user-data-dir=/tmp/rumor-browser-
// 87357-3668960404/org.chromium.Chromium.scoped_dir.I7L6um` -- so the name
// ends at the next separator, and ownerOfTmpdir then applies the same strict
// test the sweep uses on a directory entry.
func treeInArgv(command string) (owner int, name string, ok bool) {
	i := strings.Index(command, tmpdirPrefix)
	if i < 0 {
		return 0, "", false
	}
	rest := command[i:]
	end := strings.IndexAny(rest, "/ \t")
	if end < 0 {
		end = len(rest)
	}
	name = rest[:end]
	if owner, ok = ownerOfTmpdir(name); !ok {
		return 0, "", false
	}
	return owner, name, true
}

// sweepReport is the line the sweep says about itself, or "" when it found
// nothing to do.
//
// IT EXISTS BECAUSE THE SWEEP WAS SILENT AND THAT COST AN ATTRIBUTION. Task
// 1247 was cancelled with browsers up and its groups were gone afterwards,
// but nothing in any log said who took them: "the run left none" and "the
// next run's sweep reaped them" were separable only by argument (quill, coop
// r9/422) rather than by evidence. A sweep that says what it did makes the
// next one a reading instead of a deduction.
//
// QUIET WHEN THERE IS NOTHING TO SAY, because a line on every start would be
// noise on the ordinary path and would train a reader to skip it.
func sweepReport(t sweepTally) string {
	if t.removed == 0 && t.reaped == 0 && t.recovered == 0 && t.refused == 0 &&
		t.nohandle == 0 && t.held == 0 && t.unreadable == 0 {
		return ""
	}
	// THE SWEEPER NAMES ITSELF. `go test` runs a binary per package, several at
	// once, and each sweeps on its first Start -- so a job log can hold several
	// of these lines with no way to tell which process wrote which, or in what
	// order, because the runner flushes the whole step at once and every line
	// carries the same stamp. That is why run #790's two lines could not be
	// ordered, and why one of the two explanations for them (a race between
	// binaries) could not be separated from the other (ibis, coop r10/21).
	line := fmt.Sprintf("browser: pid %d swept %d stale tree(s), reaped %d browser "+
		"group(s), left %d in use", os.Getpid(), t.removed, t.reaped, t.kept)
	if t.reaped > 0 {
		// NAMED, AND THIS IS THE ONE A CLEAR IS ASKED FOR. fable's done-when at
		// coop r10/10 is "the next run's sweep saying reaped 1", so the figure
		// that settles whether the recorded handle works in the field was the
		// one outcome my first naming pass left anonymous -- it named recovered,
		// refused and held and skipped the one being demanded as evidence.
		line += fmt.Sprintf(" (pgid %s)", nameSome(t.reapedGroups, 4))
	}
	if t.recovered > 0 {
		// THE ONE A READER MUST SEE NEXT, because it means the first pass
		// missed a live group: the tree that should have named it was gone or
		// recorded nothing, and argv is what found it instead. NAMED, so the
		// next reader does not have to take the attribution on trust the way
		// #787's "RECOVERED 1 group(s)" had to be taken on mine.
		line += fmt.Sprintf("; RECOVERED %d group(s) from argv that no tree's "+
			"handle claimed: pgid %s", t.recovered, nameSome(t.recoveredGroups, 4))
	}
	if t.nohandle > 0 {
		line += fmt.Sprintf("; %d stale tree(s) recorded no handle", t.nohandle)
	}
	if t.held > 0 {
		line += fmt.Sprintf("; HELD %d tree(s) that something running still names: %s",
			t.held, nameSome(t.heldTrees, 4))
	}
	if t.unreadable > 0 {
		// LOUD, AND FIRST AMONG THE ALARMS, because every other number in this
		// line was computed from a table this sweep could not read: nothing was
		// removed, nothing was reaped, and nothing should be believed about
		// what is running.
		line += fmt.Sprintf("; COULD NOT READ the process table %d time(s), so "+
			"every tree was held and no group was examined", t.unreadable)
	}
	if t.refused > 0 {
		// NAMED LOUDLY AND LAST, where a reader's eye ends: a refusal means a
		// group of ours was left RUNNING, which is the leak still happening
		// rather than the sweep working.
		line += fmt.Sprintf("; REFUSED %d group(s) that are still alive and not "+
			"wholly ours -- recycled pgid, or our orphans with a stranger in the "+
			"group: pgid %s", t.refused, nameSome(t.refusedGroups, 4))
	}
	return line
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
// THE SAFE FORM IS ARGV AND NOT A NAME, AND IT IS BUILT NOW -- see
// reapOrphansByArgv. With TMPDIR owned, chrome's --user-data-dir sits INSIDE a
// tree we made, so a stray proves its own provenance with no name test and
// after its tree is gone.
//
// THE FIELD ASKED FOR IT, and the reason recorded here for deferring it had
// expired. Task 1254 (cf559ba) swept 47 stale trees and reaped 0 while
// cancelled task 1253's group 88126 was still running -- 1 chromedriver and 32
// chrome, ppid 1, 4.70 GiB RSS summed -- because the sweep REMOVED that
// group's tree, which is where its only handle lived. Nothing about reaching
// it was hard: one SIGTERM to -88126 took 33 members to 0. The old reason was
// that argv could not reach the strays that already existed -- of the 234
// drained on 2026-11-16, 0 named a tree of ours. That holds for orphans made
// BEFORE the owned TMPDIR and fails for every one made after it: 32 of 88126's
// 33 named theirs.
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
// It answers WHICH of four things happened, and the distinction is not
// cosmetic. A bool conflated the benign outcome with the alarming one (quill,
// coop r9/426): an EMPTY group and a REFUSED group both read "reaped 0" in
// identical words, and refused is the only one that means our own orphans are
// still running. The sweep's line could not show the case a reader needs.
func reapGroup(pgid int) reapOutcome {
	if pgid <= 1 {
		return reapNoHandle
	}
	members, err := listGroupMembers(pgid)
	if err != nil {
		// ITS OWN OUTCOME, so "I could not look" never again renders as the
		// benign "the group is gone" (ibis, coop r10/21).
		return reapUnreadable
	}
	if len(members) == 0 {
		return reapEmpty
	}
	if !membersAreOurs(members) {
		return reapRefused
	}
	_ = signalGroupOf(pgid, syscall.SIGTERM)
	for i := 0; i < int(StopGrace/(100*time.Millisecond)); i++ {
		if !groupHasMembers(pgid) {
			return reapSignalled
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = signalGroupOf(pgid, syscall.SIGKILL)
	return reapSignalled
}

// reapOutcome names what a stale tree's recorded group turned out to be.
type reapOutcome int

const (
	// reapNoHandle: the tree recorded no pgid. Every tree made before the
	// record existed is this, and nothing can be done for it here.
	reapNoHandle reapOutcome = iota
	// reapEmpty: the group is gone. Benign, and the ordinary case.
	reapEmpty
	// reapRefused: the group has members and they are not all ours, so it was
	// LEFT ALIVE. This is the one worth a line: either a pid was recycled onto
	// a stranger, or something joined a group of ours and our orphans are
	// still there.
	reapRefused
	// reapSignalled: the group was ours and was stopped.
	reapSignalled
	// reapUnreadable: the process table could not be read, so NOTHING is known
	// about this group. It is not empty, it is unexamined, and the two read
	// identically in the shape that shipped before r10/21.
	reapUnreadable
)

// THE TWO SYSCALLS ARE REACHED THROUGH VARIABLES so that the decisions above
// can be driven. What this code decides -- is this group ours, does it get a
// signal, which one, and in what order -- is the whole of it, and none of that
// is observable from a test that can neither conjure a real chromedriver nor
// be allowed to signal a real group to find out. Substituting these lets an
// arm assert the DECISION; whether a live browser then dies is the real pair's
// question and quill drives it in a jail that has one.
var (
	listGroupMembers = psGroupMembers
	listAllProcesses = psAllProcesses
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
func psGroupMembers(pgid int) ([]string, error) {
	procs, err := psAllProcesses()
	if err != nil {
		return nil, err
	}
	var members []string
	for _, proc := range procs {
		if proc.pgid == pgid {
			members = append(members, proc.command)
		}
	}
	return members, nil
}

// procLine is one process as ps answers it: the group it belongs to, and the
// command line that may name the tree this package gave it.
type procLine struct {
	pgid    int
	command string
}

// psAllProcesses answers every process this jail can see, with its group.
//
// THE SAME READING SERVES BOTH QUESTIONS -- which processes are in one group,
// and which groups name a tree of ours -- so there is one ps form to get wrong
// and one place to fix it.
func psAllProcesses() ([]procLine, error) {
	out, err := exec.Command("ps", "-axww", "-o", "pgid=", "-o", "command=").Output()
	if err != nil {
		// THE ERROR IS CARRIED AND NOT SWALLOWED, because the previous shape
		// returned nil here and nil is indistinguishable from "this box is
		// running nothing". Every caller then read a FAILED ps as an EMPTY
		// process table: reapGroup answered reapEmpty, whose own comment calls
		// that benign and ordinary, and treeStillClaimed looped over nothing
		// and said no process claims this tree. So one failed exec could make
		// the sweep delete a live group's tree and report `reaped 0` -- which
		// is exactly the pair of lines run #790 printed, and the reason I
		// cannot say from that log whether it was this or two binaries racing
		// (ibis, coop r10/20-21).
		return nil, fmt.Errorf("reading the process table: %w", err)
	}
	return parsePSTable(out), nil
}

// parsePSTable takes the two-column reading apart. It is separate from the
// command so an arm can drive the PARSING over a captured table, including the
// one-column output the comma form produces, without a box that has the
// processes in it.
func parsePSTable(out []byte) []procLine {
	var procs []procLine
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		pgid, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		procs = append(procs, procLine{pgid: pgid, command: strings.TrimSpace(line[len(f[0]):])})
	}
	return procs
}

// groupHasMembers answers whether a group still holds anything, and treats a
// reading it could not take as "yes": every caller is deciding whether to stop
// waiting or to delete something, and both are safer held than hurried.
func groupHasMembers(pgid int) bool {
	members, err := listGroupMembers(pgid)
	return err != nil || len(members) > 0
}

// membersAreOurs holds when a group is non-empty and everything in it is a
// browser or its driver. A single foreign member is enough to refuse: the
// cost of refusing is leaked processes a human can still see, and the cost of
// being wrong is signalling someone else's work.
func membersAreOurs(members []string) bool {
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
	sweepOnce.Do(func() {
		// ON STDERR, where this package already puts a reading of THIS BOX
		// rather than of the run: the job log keeps it, and nothing parses it.
		if line := sweepReport(sweepStaleTmpdirs()); line != "" {
			fmt.Fprintln(os.Stderr, line)
		}
	})
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
