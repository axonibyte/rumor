package wire_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
	"forge.axonibyte.com/axonibyte/rumor/internal/schedule"
	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

// waitingBrowser is a browser whose page never meets a wait: every flow stops
// at the same wait position. The walk retries a failed wait once, at the next
// step, and a second failure of the SAME wait is a `ui_timeout` -- so this is
// the shortest honest way to the ui_timeout path, and it needs no real page.
// It records each snapshot asked of it and writes the DOM file a real driver
// would, so the arm can check what the walk handed it.
type waitingBrowser struct {
	snaps  []string // the names asked for
	opened int      // watches handed out
	closed int      // watches the walk closed

	// plant has the watch hear one console error under EVERY flow, numbered by
	// the flow it was heard under, so an arm can tell which step a recorded
	// fault came from. refuse has the browser offer no watch at all.
	plant, refuse bool
	flows         int // flows driven
	faulted       int // the last flow a planted fault was heard under
}

func (b *waitingBrowser) Flow(_ context.Context, _, _, _ string, req wire.FlowRequest) (
	wire.FlowResponse, string, error) {
	b.flows++
	return wire.FlowResponse{Reached: 1, Stopped: wire.StopWait}, "#done read absent", nil
}

func (b *waitingBrowser) Read(context.Context, string, string, string, string, string) (string, int, bool, error) {
	return "", 0, false, nil
}

func (b *waitingBrowser) Snapshot(_ context.Context, _, _, dir, name string, _ bool) (string, error) {
	b.snaps = append(b.snaps, name)
	if err := os.WriteFile(filepath.Join(dir, name+".html"), []byte("<html></html>"), 0o600); err != nil {
		return "", err
	}
	return name + ".html; screenshot not asked for", nil
}

// WatchFor is the stub half of issue 125's deliverable 1: the interface cannot
// compile without it on every implementation. This fixture is about the
// ui_timeout path and raises nothing on its page, so the watch it answers is
// QUIET AND NOT REFUSING -- the two are different facts and the walk reports
// them differently.
//
// THE CELLS ON THIS FIXTURE ARE plumb'S (coop r9/301): they built it at
// b4531f6, and this head takes only the lines that make it compile.
func (b *waitingBrowser) WatchFor(context.Context, string, string) (schedule.ActorWatch, error) {
	if b.refuse {
		return nil, errors.New("this fixture offers no BiDi")
	}
	b.opened++
	return quietWatch{b: b}, nil
}

// quietWatch hears nothing and closes clean, and tells its browser it was
// closed, so an arm can hold the walk to giving back what it opened.
type quietWatch struct{ b *waitingBrowser }

// Since hears nothing unless the browser plants. With plant, the first
// barrier after a flow hears that flow's numbered fault. The walk's opening
// barrier precedes the flow and so hears nothing new, which keeps the arm
// about the CLOSING answer, the one the walk records.
func (w quietWatch) Since(context.Context) ([]schedule.PageFault, error) {
	if w.b.plant && w.b.flows > w.b.faulted {
		w.b.faulted = w.b.flows
		return []schedule.PageFault{{Kind: "console",
			Text: fmt.Sprintf("planted under flow %d", w.b.flows)}}, nil
	}
	return nil, nil
}

func (w quietWatch) Close(context.Context) error {
	w.b.closed++
	return nil
}

// waitingWalk walks the fixture project with join_event as a browser-context
// flow whose wait never comes true, until a seed reaches the ui_timeout.
func waitingWalk(t *testing.T, br *waitingBrowser, dir string) (schedule.Record, uint64) {
	t.Helper()
	ctx := context.Background()
	sess, err := wire.Open(ctx, fakeSpawn(), 20*time.Second)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = sess.Close() }()
	p := walkProject(t)
	p.Actions[1].Context = "browser"
	p.Actions[1].Flow = []bindings.Step{{Step: "click", Target: "join"}, {Step: "wait", Until: "done"}}
	p.Roles = []bindings.Role{{Name: "member", Contexts: []string{"api", "browser"}}}
	for seed := uint64(1); seed <= 10; seed++ {
		// THE FLOW COUNT IS PER WALK, so an arm can compare it with the one
		// record it is handed; the watch tallies stay whole-test for S1's arm.
		br.flows, br.faulted = 0, 0
		rec, werr := schedule.Walk(ctx, p, sess, schedule.Browser{Driver: br, SnapshotDir: dir}, seed, 12, 1)
		if werr != nil {
			t.Fatalf("seed %d: %v", seed, werr)
		}
		if rec.Finding != nil && rec.Finding.Value == engine.FindingUITimedOut {
			return rec, seed
		}
	}
	t.Fatal("no seed in 1..10 reached a ui_timeout, so this arm never reached the call it is about")
	return schedule.Record{}, 0
}

// THE ui_timeout KEEPS ITS PAGE (issue 124's deliverable 2; the cell ibis owed
// me at coop rumor/2044). Deleting the call block in the walk's ui_timeout path
// survived all 43 packages while its sibling, the disagreement path, is killed
// by three arms; this is the arm that reaches it. The walk must ask the browser
// for exactly one snapshot, named seed-step-actor for the step the finding is
// recorded at, and that step must carry the sentence the browser answered.
func TestAUITimeoutKeepsThePageItTimedOutOn(t *testing.T) {
	br := &waitingBrowser{}
	dir := t.TempDir()
	rec, seed := waitingWalk(t, br, dir)

	var found *schedule.Step
	for i := range rec.Steps {
		if rec.Steps[i].Snapshot != "" {
			if found != nil {
				t.Fatalf("two steps carry a snapshot, %d and %d", found.N, rec.Steps[i].N)
			}
			found = &rec.Steps[i]
		}
	}
	if found == nil {
		t.Fatalf("a ui_timeout at step %d left no snapshot on any step", rec.FoundAt)
	}
	if found.N != rec.FoundAt {
		t.Fatalf("the snapshot is on step %d, the finding at step %d", found.N, rec.FoundAt)
	}
	want := fmt.Sprintf("%d-%d-%s", seed, rec.FoundAt, found.Actor)
	if len(br.snaps) != 1 || br.snaps[0] != want {
		t.Fatalf("the browser was asked for %v, want exactly [%s]", br.snaps, want)
	}
	if found.Snapshot != want+".html; screenshot not asked for" {
		t.Fatalf("the step carries %q, not the browser's sentence", found.Snapshot)
	}
	if _, err := os.Stat(filepath.Join(dir, want+".html")); err != nil {
		t.Fatalf("the page the step names is not in the directory: %v", err)
	}
}

// AND WITHOUT --snapshots THE SAME ui_timeout SAYS SO, asking the browser for
// nothing: the presence partner, so an arm that passed on a walk which never
// reached the call could not pass here too.
func TestAUITimeoutWithoutSnapshotsSaysThePageWasNotKept(t *testing.T) {
	br := &waitingBrowser{}
	rec, _ := waitingWalk(t, br, "")
	for _, st := range rec.Steps {
		if st.N == rec.FoundAt {
			if st.Snapshot != "snapshot not kept: no --snapshots" {
				t.Fatalf("the ui_timeout step carries %q, want it to say the page was not kept", st.Snapshot)
			}
			if len(br.snaps) != 0 {
				t.Fatalf("the browser was asked for %v with nowhere to write", br.snaps)
			}
			return
		}
	}
	t.Fatalf("no step is numbered %d, the finding's", rec.FoundAt)
}

// THE WALK GIVES BACK EVERY WATCH IT OPENED (issue 125's deliverable 1;
// quill's survivor S1 at coop r9/306). a47c88a's arms call
// Sessions.CloseWatches directly, so deleting the walk's own deferred call --
// its only production call site -- left every package green, and a real
// driver would have held one BiDi socket per actor per attempt open until the
// run exited. This arm goes through schedule.Walk itself. The PRECONDITION is
// asserted first, so a walk that opened no watch cannot pass by having
// nothing to close.
func TestTheWalkClosesEveryWatchItOpened(t *testing.T) {
	br := &waitingBrowser{}
	waitingWalk(t, br, "")
	if br.opened == 0 {
		t.Fatal("the walk opened no watch, so this arm has no subject: a browser actor walked and was never watched")
	}
	if br.closed != br.opened {
		t.Fatalf("the walk opened %d watch(es) and closed %d", br.opened, br.closed)
	}
}

// flowSteps is the record's browser-flow steps, in order: the steps whose
// action began a flow through the browser (walk.go's browser branch of drive).
func flowSteps(rec schedule.Record) []schedule.Step {
	var out []schedule.Step
	for _, st := range rec.Steps {
		if st.FlowBrowser {
			out = append(out, st)
		}
	}
	return out
}

// EVERY FLOW THE WALK DRIVES THROUGH THE BROWSER IS HEARD (issue 125's
// deliverables 1 and 2; the remainder ibis named at coop rumor/2106).
// 07ee2f6's arms build schedule.Walked directly, so `TakeHeard -> rec.Walked`
// was asserted by nothing. This walks schedule.Walk through a browser that
// hears one fault NUMBERED BY ITS FLOW under every flow, and the ui_timeout
// path drives TWO: the flow that stops at its wait, and its RESUME on the next
// draw, which is the one that times out. So the record must walk past one
// fault per flow driven, in flow order, the first at the flow's own step.
//
// WHAT IT FOUND WHEN FIRST RUN (coop r9/320): resumeFlow drives the browser
// with no barriers, so the resume's page was neither heard nor unheard --
// one entry for two flows, and the fault under the wait that timed out was
// the one lost. The resume's At is not pinned here, since whether a resume is
// stamped at its pending step or at the draw that ran it is the lead's call.
func TestEveryFlowTheWalkDrivesIsHeardInFlowOrder(t *testing.T) {
	br := &waitingBrowser{plant: true}
	rec, _ := waitingWalk(t, br, "")
	fs := flowSteps(rec)
	if len(fs) == 0 || br.flows < 2 {
		t.Fatalf("the precondition failed: %d flow step(s) and %d flow(s) driven; the ui_timeout "+
			"path drives a flow and its resume", len(fs), br.flows)
	}
	if rec.Finding == nil || rec.Finding.Value != engine.FindingUITimedOut {
		t.Fatalf("the stopper is %v, want the ui_timeout: a page_fault must not displace it", rec.Finding)
	}
	if len(rec.Walked) != br.flows {
		t.Fatalf("%d flow(s) were driven through the browser, each hearing one fault, and the record "+
			"walked past %d: a flow's page went unheard and unrecorded", br.flows, len(rec.Walked))
	}
	for i, w := range rec.Walked {
		want := fmt.Sprintf("planted under flow %d", i+1)
		if w.Finding.Value != engine.FindingPageFault || w.Finding.Overlay == nil ||
			!strings.Contains(w.Finding.Overlay.Reason, want) {
			t.Fatalf("entry %d is %+v, want a page_fault naming %q", i, w.Finding, want)
		}
		if i > 0 && w.At < rec.Walked[i-1].At {
			t.Fatalf("entry %d is at step %d, before entry %d at %d", i, w.At, i-1, rec.Walked[i-1].At)
		}
	}
	if first := rec.Walked[0]; first.At != fs[0].N || first.Actor != fs[0].Actor {
		t.Fatalf("the first flow's fault is at step %d as %q, want step %d as %q",
			first.At, first.Actor, fs[0].N, fs[0].Actor)
	}
	if len(rec.Unheard) != 0 {
		t.Fatalf("a watch that heard every flow left %v unheard", rec.Unheard)
	}
}

// AND A BROWSER THAT CANNOT LISTEN LEAVES EVERY FLOW UNHEARD, the drain's
// other half (`TakeUnheard`, `u.At = n`): one entry per flow driven, with a
// reason, the first at the flow's own step, and nothing walked past.
func TestAWatchThatCannotListenLeavesEveryFlowUnheard(t *testing.T) {
	br := &waitingBrowser{refuse: true}
	rec, _ := waitingWalk(t, br, "")
	fs := flowSteps(rec)
	if len(fs) == 0 || br.flows < 2 {
		t.Fatalf("the precondition failed: %d flow step(s) and %d flow(s) driven", len(fs), br.flows)
	}
	if len(rec.Unheard) != br.flows {
		t.Fatalf("%d flow(s) could not be heard and the record says %d", br.flows, len(rec.Unheard))
	}
	for i, u := range rec.Unheard {
		if u.Why == "" {
			t.Fatalf("unheard entry %d gives no reason: %+v", i, u)
		}
	}
	if u := rec.Unheard[0]; u.At != fs[0].N || u.Actor != fs[0].Actor {
		t.Fatalf("the first unheard entry is %+v, want step %d as %q", u, fs[0].N, fs[0].Actor)
	}
	if len(rec.Walked) != 0 {
		t.Fatalf("a browser that heard nothing walked past %v", rec.Walked)
	}
}

// AND A QUIET PAGE LEAVES BOTH LISTS EMPTY: the presence partner, so neither
// arm above passes on a walk that fills them regardless.
func TestAQuietPageLeavesNothingWalkedPastAndNothingUnheard(t *testing.T) {
	br := &waitingBrowser{}
	rec, _ := waitingWalk(t, br, "")
	if len(flowSteps(rec)) == 0 {
		t.Fatal("the precondition failed: no browser-flow step, so the arm has no subject")
	}
	if len(rec.Walked) != 0 || len(rec.Unheard) != 0 {
		t.Fatalf("a quiet page recorded walked %v and unheard %v", rec.Walked, rec.Unheard)
	}
}
