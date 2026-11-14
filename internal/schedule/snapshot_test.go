package schedule

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// NO BROWSER, NO SENTENCE: a run no browser drove has no page, and a line
// about one would move every such run's output for nothing.
func TestARunNoBrowserDroveSaysNothingAboutAPage(t *testing.T) {
	s := NewSessions(nil)
	got, err := s.snapshot(context.Background(), "alice", 7, 3, nil)
	if err != nil || got != "" {
		t.Fatalf("snapshot with no browser = %q, %v; want nothing", got, err)
	}
}

// NO --snapshots, AND THE FINDING SAYS SO rather than staying silent
// (coop r9/191) -- and nothing is written, since there is nowhere to put it.
func TestWithoutADirectoryTheSnapshotSaysItWasNotKept(t *testing.T) {
	stub := &routedFlow{}
	s := NewSessions(nil)
	s.UseBrowser(stub, "http://127.0.0.1:1")
	got, err := s.snapshot(context.Background(), "alice", 7, 3, nil)
	if err != nil || got != "snapshot not kept: no --snapshots" {
		t.Fatalf("snapshot with no directory = %q, %v", got, err)
	}
	if len(stub.snaps) != 0 {
		t.Fatalf("the driver was asked for %d snapshot(s) with nowhere to write them", len(stub.snaps))
	}
}

// THE DRIVER IS HANDED WHAT THE RUN ASKED FOR: the directory, the screenshot
// request, the name seed-step-actor, and the token the actor's page last
// carried -- which is what the DOM is redacted of. A walk that passed no token
// would write the credential into the snapshot verbatim.
func TestTheSnapshotCarriesTheNameAndTheTokenThePageWasDrivenWith(t *testing.T) {
	stub := &routedFlow{}
	s := NewSessions(nil)
	s.UseBrowser(stub, "http://127.0.0.1:1")
	dir := t.TempDir()
	s.UseSnapshots(dir, true)
	s.noteBrowserToken("alice", "tok-alice")
	s.noteBrowserToken("bob", "tok-bob")
	got, err := s.snapshot(context.Background(), "alice", 7, 3, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(stub.snaps) != 1 {
		t.Fatalf("the driver was asked %d time(s), want once", len(stub.snaps))
	}
	c := stub.snaps[0]
	if c.actor != "alice" || c.token != "tok-alice" || c.dir != dir || c.name != "7-3-alice" || !c.screenshot {
		t.Fatalf("the driver was asked %+v", c)
	}
	if got != "7-3-alice.html; screenshot not asked for" {
		t.Fatalf("the sentence is %q", got)
	}
}

// THE ACTOR IS PROJECT TEXT IN A FILE NAME (rook, coop r9/196): a hostile name
// is escaped into one plain file inside the directory, and nothing is written
// outside it.
func TestAHostileActorNameStaysInsideTheSnapshotDirectory(t *testing.T) {
	for _, actor := range []string{"../escape", "a/b", "..", "/abs"} {
		t.Run(actor, func(t *testing.T) {
			stub := &routedFlow{}
			s := NewSessions(nil)
			s.UseBrowser(stub, "http://127.0.0.1:1")
			root := t.TempDir()
			dir := filepath.Join(root, "snaps")
			if err := os.Mkdir(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			s.UseSnapshots(dir, false)
			if _, err := s.snapshot(context.Background(), actor, 7, 3, nil); err != nil {
				t.Fatal(err)
			}
			name := stub.snaps[0].name
			if name != filepath.Base(name) || strings.Contains(name, "/") {
				t.Fatalf("actor %q became the name %q, which is not one plain file", actor, name)
			}
			var outside []string
			_ = filepath.WalkDir(root, func(p string, e os.DirEntry, _ error) error {
				if !e.IsDir() && filepath.Dir(p) != dir {
					outside = append(outside, p)
				}
				return nil
			})
			if len(outside) != 0 {
				t.Fatalf("actor %q wrote outside the directory: %v", actor, outside)
			}
		})
	}
}

// THE DISAGREEMENT IS ONE CLAIM'S, ON ONE SUBJECT (fable, coop r9/197): a
// browser reading and a direct reading of the same claim type about DIFFERENT
// subjects are two claims, and the first cut compared them as one -- measured
// on seed 11, where one event's page and another event's store differed.
func TestPageAndStoreDisagreeOnlyAboutTheSameClaimOfTheSameSubject(t *testing.T) {
	at := &pageRead{page: "/ui/events/e1", element: "count"}
	b := func(subject string, v any) reading {
		return reading{source: "browser", of: "event.joined", subject: subject, value: v, at: at}
	}
	d := func(subject string, v any) reading {
		return reading{source: "direct", of: "event.joined", subject: subject, value: v}
	}
	for _, c := range []struct {
		name string
		rs   []reading
		want bool
	}{
		{"same subject, different answers", []reading{d("e1", 2), b("e1", 0)}, true},
		{"the browser reading first", []reading{b("e1", 0), d("e1", 2)}, true},
		{"same subject, same answer", []reading{d("e1", 2), b("e1", 2)}, false},
		{"one as JSON, one as converted text", []reading{d("e1", 2.0), b("e1", 2)}, false},
		{"different subjects", []reading{d("e2", 2), b("e1", 0)}, false},
		{"different claim types", []reading{{source: "direct", of: "user.created", subject: "e1", value: 2}, b("e1", 0)}, false},
		{"an api reading is not the store", []reading{{source: "api", of: "event.joined", subject: "e1", value: 2}, b("e1", 0)}, false},
		{"no browser reading at all", []reading{d("e1", 2), d("e1", 3)}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := pageDisagrees(c.rs)
			if (got != nil) != c.want {
				t.Fatalf("pageDisagrees = %v, want disagreement=%v", got, c.want)
			}
			if got != nil && got.at != at {
				t.Fatalf("the disagreeing reading did not carry its page")
			}
		})
	}
}

// THE DISAGREEING PAGE IS REOPENED BEFORE IT IS KEPT. The actor's browser shows
// whichever page its LAST observation left, which need not be the one that
// disagreed; on seed 40 they happen to coincide, which is why the end-to-end
// arm cannot see this and a mutant skipping the reopen survived it. So the
// walk's own call is held here: the reading's page is read again, with the
// actor's token, and only then is the snapshot asked for.
func TestTheDisagreeingPageIsReopenedBeforeItIsKept(t *testing.T) {
	stub := &routedFlow{}
	s := NewSessions(nil)
	s.UseBrowser(stub, "http://127.0.0.1:1")
	s.UseSnapshots(t.TempDir(), false)
	s.noteBrowserToken("alice", "tok-alice")
	if _, err := s.snapshot(context.Background(), "alice", 7, 3, &pageRead{page: "/ui/events/e1", element: "count"}); err != nil {
		t.Fatal(err)
	}
	if len(stub.reads) != 1 {
		t.Fatalf("the disagreeing page was read %d time(s) before the snapshot, want once", len(stub.reads))
	}
	r := stub.reads[0]
	if r.actor != "alice" || r.page != "/ui/events/e1" || r.element != "count" || r.token != "tok-alice" {
		t.Fatalf("the page reopened was %+v", r)
	}
	if len(stub.snaps) != 1 || stub.snaps[0].readsBefore != 1 {
		t.Fatalf("the snapshot was not taken AFTER the reopen: %+v", stub.snaps)
	}
	// And with no disagreeing page (a ui_timeout), nothing is reopened.
	stub2 := &routedFlow{}
	s2 := NewSessions(nil)
	s2.UseBrowser(stub2, "http://127.0.0.1:1")
	s2.UseSnapshots(t.TempDir(), false)
	if _, err := s2.snapshot(context.Background(), "alice", 7, 3, nil); err != nil {
		t.Fatal(err)
	}
	if len(stub2.reads) != 0 {
		t.Fatalf("a snapshot with no page to reopen read %d page(s)", len(stub2.reads))
	}
}
