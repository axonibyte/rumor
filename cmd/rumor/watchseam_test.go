package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	webdriver "forge.axonibyte.com/axonibyte/rumor/internal/browser"
	"forge.axonibyte.com/axonibyte/rumor/internal/browser/fakedriver"
	"forge.axonibyte.com/axonibyte/rumor/internal/schedule"
)

// THE SEAM BETWEEN THE BROWSER'S WATCH AND THE WALK, which nothing else drives
// (issue 125's deliverable 1; the cells on plumb's fixtures, coop r9/301-302).
// internal/browser's own arms hold what a watch HEARS, and internal/schedule's
// hold what the walk does with what routedFlow hands it. Between them sits
// watchingDriver, the only code that turns a browser.Fault into a
// schedule.PageFault, and every real-browser run in tests/run walks the toy,
// whose pages raise no fault -- so the conversion has never been handed one.
// A Kind and a Text transposed, or a Status dropped, passed every package.
//
// WHAT THIS DOES NOT PROVE: that the walk attributes a converted fault to the
// right step (a47c88a's own arms, through routedFlow), or that a run records
// it (deliverable 2).

// realPairOrSkip finds chromedriver and a chromium on PATH, skipping by name
// where either is absent, as every real-browser arm in this tree does.
func realPairOrSkip(t *testing.T) (string, string) {
	t.Helper()
	driver, ok := pathLookup("chromedriver")
	if !ok {
		t.Skip("no chromedriver on PATH: a fault needs a real page")
	}
	for _, n := range []string{"chrome", "chromium", "chromium-browser", "google-chrome"} {
		if b, ok := pathLookup(n); ok {
			return driver, b
		}
	}
	t.Skip("no chromium on PATH")
	return "", ""
}

func pathLookup(name string) (string, bool) {
	for _, dir := range strings.Split(os.Getenv("PATH"), ":") {
		if dir == "" {
			continue
		}
		p := dir + "/" + name
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0 {
			return p, true
		}
	}
	return "", false
}

func seamPage(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprint(w, "<!doctype html><title>t</title>"+body)
}

// A FAULT CROSSES THE SEAM FIELD FOR FIELD. Two steps through the stage's own
// wrapper against a real pair: a console error, whose Status must be 0 and
// whose Text is the entry, and a 503 document, whose Status must be 503 and
// whose Text is its URL. The two kinds put DIFFERENT values in each field, so
// a conversion that crossed any two of them cannot pass both. And a clean step
// between them hears nothing, so the arm is not satisfied by a watch that
// replays every fault it ever heard.
func TestTheStageHandsTheWalkEachHeardFaultFieldForField(t *testing.T) {
	driver, browserBin := realPairOrSkip(t)
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/console":
			seamPage(rw, `<p id="x">c</p><script>console.error("seam planted error")</script>`)
		case "/down":
			rw.WriteHeader(http.StatusServiceUnavailable)
			seamPage(rw, `<p id="x">down</p>`)
		default:
			seamPage(rw, `<p id="x">up</p>`)
		}
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	d, err := webdriver.Start(ctx, driver, browserBin, true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close(context.Background()) }()

	var fd schedule.FlowDriver = watchingDriver{d}
	w, err := fd.WatchFor(ctx, "alice", "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w.Close(context.Background()) }()

	stepOnto := func(page string) []schedule.PageFault {
		t.Helper()
		if _, err := w.Since(ctx); err != nil {
			t.Fatal(err)
		}
		if _, _, _, err := fd.Read(ctx, "alice", srv.URL, page, "x", ""); err != nil {
			t.Fatalf("reading %s: %v", page, err)
		}
		got, err := w.Since(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	got := stepOnto("/console")
	if len(got) != 1 || got[0].Kind != "console" || got[0].Status != 0 ||
		!strings.Contains(got[0].Text, "seam planted error") {
		t.Fatalf("the console step reached the walk as %+v, want exactly {console, the entry, 0}", got)
	}
	if got := stepOnto("/clean"); len(got) != 0 {
		t.Fatalf("a clean step reached the walk as %+v: the seam replayed an earlier step's fault", got)
	}
	got = stepOnto("/down")
	if len(got) != 1 || got[0].Kind != "response" || got[0].Status != 503 ||
		!strings.Contains(got[0].Text, "/down") {
		t.Fatalf("the 503 step reached the walk as %+v, want exactly {response, its URL, 503}", got)
	}
}

// AND A DRIVER THAT CANNOT LISTEN REACHES THE WALK AS A REFUSAL. The fake
// driver answers `"capabilities":{}`, so internal/browser refuses the watch by
// name; the stage must pass that refusal on and hand the walk NO watch, because
// an empty one would read, step after step, as a page that misbehaved in no
// way -- the heard and the unheard are the same bytes (a47c88a).
func TestTheStageHandsTheWalkARefusalAndNoWatchWhenTheDriverCannotListen(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake driver is this test binary")
	}
	t.Setenv(fakedriver.ModeEnv, "ready")
	t.Setenv(fakedriver.JournalEnv, t.TempDir()+"/journal")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	d, err := webdriver.Start(ctx, os.Args[0], "", true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close(context.Background()) }()

	w, err := watchingDriver{d}.WatchFor(ctx, "alice", "")
	if err == nil {
		t.Fatalf("the stage handed the walk a watch (%T) on a driver offering no BiDi", w)
	}
	if !strings.Contains(err.Error(), "no webSocketUrl") {
		t.Fatalf("the refusal reached the walk as %q, not naming the missing webSocketUrl", err)
	}
	if w != nil {
		t.Fatalf("the refusal came with a watch (%T): the walk would read it as a quiet page", w)
	}
}
