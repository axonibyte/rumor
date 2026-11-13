package wire_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
	snaps []string // the names asked for
}

func (b *waitingBrowser) Flow(_ context.Context, _, _, _ string, req wire.FlowRequest) (
	wire.FlowResponse, string, error) {
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
