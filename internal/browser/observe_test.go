package browser

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestAReadingComesFromTheLiveDOMAndNotTheServedHTML is the whole reason a
// `browser` observation is not a second `api` one, and it is why fable's r9/120
// put the read in the DRIVER: the adapter fetches the served HTML and parses it,
// so a figure the application WRITES INTO THE PAGE is invisible to it and
// visible to a reader.
//
// THE PAGE SERVES AN EMPTY ELEMENT AND FILLS IT BY SCRIPT, and the arm asserts
// BOTH halves: the bytes carry no 7, and the browser reads 7. One without the
// other proves nothing -- a page that served the number would pass this against
// a driver that read the HTML, which is the oracle this arm exists to tell apart.
func TestAReadingComesFromTheLiveDOMAndNotTheServedHTML(t *testing.T) {
	driver, found := realDriver(t)
	if !found {
		t.Skip("no chromedriver on PATH: the browser-side arms run where the pair lives")
	}
	browserBin, bfound := realBrowser(t)
	if !bfound {
		t.Skip("no chromium on PATH")
	}

	const served = `<!doctype html><title>late</title><p id="count"></p>` +
		`<script>document.getElementById("count").textContent = "7";</script>`
	if strings.Contains(served, ">7<") {
		t.Fatal("the fixture serves the number in its bytes, so it cannot tell the two oracles apart")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, served)
	}))
	defer srv.Close()

	d, err := Start(context.Background(), driver, browserBin, true)
	noteGroup(d)
	if err != nil {
		t.Fatalf("starting the real pair: %v", err)
	}
	defer func() { _ = d.Close(context.Background()) }()

	text, _, present, rerr := d.Read(context.Background(), "alice", srv.URL, "/event", "count", "tok")
	if rerr != nil {
		t.Fatalf("reading #count: %v", rerr)
	}
	if !present {
		t.Fatal("the element was not found, so the reading says nothing about the DOM")
	}
	if strings.TrimSpace(text) != "7" {
		t.Errorf("the browser read %q, want 7: the figure is written into the page after it is "+
			"served, so an oracle reading the HTML sees an empty element and a reader sees 7",
			text)
	}
}

// AND AN ELEMENT THE PAGE DOES NOT CARRY IS AN ANSWER, not an error: a page
// that does not show the figure is what a reader SEES, and the walk turns this
// `false` into its one spelling of `missing` -- the same word the adapter's own
// unservable page produces (fable, coop rumor/1669).
func TestAnAbsentElementIsAnAnswerAndNotAnError(t *testing.T) {
	driver, found := realDriver(t)
	if !found {
		t.Skip("no chromedriver on PATH")
	}
	browserBin, bfound := realBrowser(t)
	if !bfound {
		t.Skip("no chromium on PATH")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<!doctype html><title>bare</title><p id="other">x</p>`)
	}))
	defer srv.Close()

	d, err := Start(context.Background(), driver, browserBin, true)
	noteGroup(d)
	if err != nil {
		t.Fatalf("starting the real pair: %v", err)
	}
	defer func() { _ = d.Close(context.Background()) }()

	text, _, present, rerr := d.Read(context.Background(), "alice", srv.URL, "/x", "count", "tok")
	if rerr != nil {
		t.Fatalf("an absent element was reported as an error: %v", rerr)
	}
	if present {
		t.Errorf("the page carries no #count and the read reported one, as %q", text)
	}
}

// TestAReadRefusesWhatItCannotAddress needs no browser: these are settled by
// reading the arguments, and the walk refuses them first with the message a
// project reads. This is the same refusal one layer in, so no later caller can
// reach "#" or an empty path by a route that skips it.
func TestAReadRefusesWhatItCannotAddress(t *testing.T) {
	d, _ := startFake(t, "ready", true)
	defer func() { _ = d.Close(context.Background()) }()

	for _, c := range []struct{ name, page, element, wants string }{
		{"no page", "", "count", "names no page"},
		{"no element", "/x", "", "names no element"},
		{"an element that is a selector", "/x", "#count .n", "selector and not an ID"},
		{"an element with a dot", "/x", "count.n", "selector and not an ID"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, _, _, err := d.Read(context.Background(), "alice", "http://127.0.0.1:1",
				c.page, c.element, "tok")
			if err == nil {
				t.Fatalf("%s was read anyway", c.name)
			}
			if !strings.Contains(err.Error(), c.wants) {
				t.Errorf("the refusal does not say %q: %v", c.wants, err)
			}
		})
	}
}

// TestACountIsTheChildrenAndNotTheDescendants is fable's third reading word
// (coop r9/124) and the narrowing inside it: a `count` is "the number of the
// element's CHILD elements", which is what the adapter's own oracle takes from
// the served HTML.
//
// THE LIST CARRIES A NESTED ELEMENT ON PURPOSE. A bare `*` from an element finds
// every DESCENDANT, so this page reads 4 that way and 3 the right way -- a
// fixture of plain items would make the two indistinguishable and the selector's
// discrimination would sit unheld in a comment.
func TestACountIsTheChildrenAndNotTheDescendants(t *testing.T) {
	driver, found := realDriver(t)
	if !found {
		t.Skip("no chromedriver on PATH")
	}
	browserBin, bfound := realBrowser(t)
	if !bfound {
		t.Skip("no chromium on PATH")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/empty":
			_, _ = fmt.Fprint(w, `<!doctype html><title>none</title><ul id="attendees"></ul>`)
		default:
			_, _ = fmt.Fprint(w, `<!doctype html><title>three</title>`+
				`<ul id="attendees"><li><span>ibis</span></li><li>wren</li><li>rook</li></ul>`)
		}
	}))
	defer srv.Close()

	d, err := Start(context.Background(), driver, browserBin, true)
	noteGroup(d)
	if err != nil {
		t.Fatalf("starting the real pair: %v", err)
	}
	defer func() { _ = d.Close(context.Background()) }()

	_, kids, present, rerr := d.Read(context.Background(), "alice", srv.URL,
		"/event", "attendees", "tok")
	if rerr != nil {
		t.Fatalf("reading #attendees: %v", rerr)
	}
	if !present {
		t.Fatal("the list was not found at all")
	}
	if kids != 3 {
		t.Errorf("the count is %d, want 3: the page has three items and four descendants, so a "+
			"reading of %d is every descendant rather than the children a reader sees", kids, kids)
	}

	// AND AN EMPTY LIST READS ZERO AND IS STILL PRESENT, which is the state a
	// non-admin's page legitimately produces: zero attendees SHOWN is the
	// reading the statute compares against 0, and an absent element would be
	// `missing` instead -- a different answer about a different page.
	_, kids, present, rerr = d.Read(context.Background(), "bob", srv.URL,
		"/empty", "attendees", "tok")
	if rerr != nil {
		t.Fatalf("reading an empty #attendees: %v", rerr)
	}
	if !present || kids != 0 {
		t.Errorf("an empty list read (%d, present=%v), want (0, true): an empty list is an "+
			"answer and not an absence", kids, present)
	}
}
