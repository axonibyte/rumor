package browser

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const snapTok = "tok-s3cret-91"

// fromURL is page script that reads the actor's token off the page's own
// address, as an application echoing ?token= would -- so the token reaches the
// page WITHOUT appearing in the served source.
const fromURL = `new URLSearchParams(location.search).get("token")`

// snapPages are the rows measured at coop r9/187-190, one page per way a token
// was seen reaching the screen, plus the clean page and the canvas residue.
var snapPages = map[string]string{
	"/attr":   `<input value="` + snapTok + `">`,
	"/prop":   `<input id="i"><script>document.getElementById("i").value = ` + fromURL + `</script>`,
	"/typed":  `<input id="i">`,
	"/after":  `<style>#c::after { content: "` + snapTok + `" }</style><p id="c"></p>`,
	"/iframe": `<iframe src="/inner"></iframe>`, // the frame's src carries no token
	"/inner":  `<p>` + snapTok + `</p>`,
	"/shadow": `<div id="h"></div><script>document.getElementById("h").attachShadow({mode: "open"}).innerHTML = "<p>" + ` + fromURL + ` + "</p>"</script>`,
	"/closed": `<div id="h"></div><script>document.getElementById("h").attachShadow({mode: "closed"}).innerHTML = "<p>" + ` + fromURL + ` + "</p>"</script>`,
	"/canvas": `<canvas id="k" width="300" height="40"></canvas><script>document.getElementById("k").getContext("2d").fillText(` + fromURL + `, 10, 20)</script>`,
	"/clean":  `<p>nothing to hide</p>`,
}

// realSession starts the real pair and opens alice's session, skipping by name
// where the pair is absent, as every browser-side arm here does.
func realSession(t *testing.T) (context.Context, *Driver, string, string) {
	t.Helper()
	driver, found := realDriver(t)
	if !found {
		t.Skip("no chromedriver on PATH: a snapshot needs a page")
	}
	browserBin, bfound := realBrowser(t)
	if !bfound {
		t.Skip("no chromium on PATH")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := snapPages[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		page(w, body)
	}))
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	t.Cleanup(cancel)
	d, err := Start(ctx, driver, browserBin, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close(context.Background()) })
	session, err := d.SessionFor(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	return ctx, d, session, srv.URL
}

// EVERY ROW IS AN ARM (fable, coop r9/191). A page that puts the token where a
// check can reach it has its screenshot WITHHELD, naming where; the clean page
// and the canvas keep theirs, and the canvas's note names it as the residue.
func TestAScreenshotIsWithheldWhereverTheTokenCanBeFound(t *testing.T) {
	ctx, d, session, base := realSession(t)
	for _, c := range []struct {
		page, want string // want: where, for a withheld PNG; "" for a kept one
	}{
		{"/attr", "the page source"},
		{"/prop", "a form control's live value"},
		{"/typed", "a form control's live value"},
		{"/after", "the page source"},
		{"/iframe", "the page source of a frame 1 deep"},
		{"/shadow", "the text inside a shadow root"},
		{"/closed", "the text inside a shadow root"}, // measured: chromedriver hands back a closed root
		{"/canvas", ""},
		{"/clean", ""},
	} {
		t.Run(c.page, func(t *testing.T) {
			if err := d.navigate(ctx, session, base, c.page, snapTok); err != nil {
				t.Fatal(err)
			}
			if c.page == "/typed" {
				el, ok, err := d.element(ctx, session, "#i")
				if err != nil || !ok {
					t.Fatalf("finding the input: %v %v", ok, err)
				}
				if err := d.sendKeys(ctx, session, el, snapTok); err != nil {
					t.Fatal(err)
				}
			}
			dir := t.TempDir()
			snap, err := d.Capture(ctx, "alice", snapTok, dir, "7-3-alice", true)
			if err != nil {
				t.Fatal(err)
			}
			// THE CHECK LEAVES THE SESSION WHERE THE WALK ACTS: the top
			// document. A walk into a frame that stayed there would move
			// the actor's next step onto the frame's page.
			if c.page == "/iframe" {
				if _, top, err := d.element(ctx, session, "iframe"); err != nil || !top {
					t.Fatalf("after the frame walk the session is not at the top document: %v", err)
				}
			}
			png := filepath.Join(dir, "7-3-alice.png")
			_, statErr := os.Stat(png)
			if c.want != "" {
				if snap.PNG != "" || statErr == nil {
					t.Fatalf("%s: a screenshot was kept (%q) though the token is on the page", c.page, snap.PNG)
				}
				if !strings.Contains(snap.Note, "withheld") || !strings.Contains(snap.Note, c.want) {
					t.Fatalf("%s: note %q, want it withheld naming %q", c.page, snap.Note, c.want)
				}
				return
			}
			raw, err := os.ReadFile(png)
			if err != nil || !bytes.HasPrefix(raw, []byte("\x89PNG\r\n\x1a\n")) {
				t.Fatalf("%s: no PNG written (%v), note %q", c.page, err, snap.Note)
			}
			if !strings.Contains(snap.Note, ScreenshotResidue) {
				t.Fatalf("%s: a kept screenshot's note %q does not name the residue", c.page, snap.Note)
			}
		})
	}
}

// THE DOM IS REDACTED, AND THE GREP IS THE ARM (124 d2): the snapshot file's
// bytes carry no token. The presence partner reads the same page's source
// first and finds it there, so a page that never held the token cannot pass.
func TestTheDOMSnapshotCarriesNoToken(t *testing.T) {
	ctx, d, session, base := realSession(t)
	if err := d.navigate(ctx, session, base, "/attr", snapTok); err != nil {
		t.Fatal(err)
	}
	var src struct {
		Value string `json:"value"`
	}
	if err := d.call(ctx, http.MethodGet, "/session/"+session+"/source", nil, &src); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(src.Value, snapTok) {
		t.Fatal("the page under test never held the token, so this arm would pass on nothing")
	}
	dir := t.TempDir()
	snap, err := d.Capture(ctx, "alice", snapTok, dir, "7-3-alice", false)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, snap.HTML))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(snapTok)) {
		t.Fatalf("the DOM snapshot carries the token: %q", raw)
	}
	if !bytes.Contains(raw, []byte(Redacted)) {
		t.Fatalf("the DOM snapshot shows no redaction where the token was: %q", raw)
	}
	if snap.PNG != "" || snap.Note != "screenshot not asked for" {
		t.Fatalf("not asked for a screenshot, got PNG %q note %q", snap.PNG, snap.Note)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Fatalf("not asked for a screenshot, the directory holds %d files", len(entries))
	}
}

// A name is a plain file name: a snapshot cannot be steered outside its
// directory by a name the caller built.
func TestASnapshotNameThatIsNotAPlainFileIsRefused(t *testing.T) {
	d := &Driver{sessions: map[string]string{"alice": "s1"}}
	for _, name := range []string{"", "../x", "a/b", ".hidden"} {
		if _, err := d.Capture(context.Background(), "alice", "", t.TempDir(), name, false); err == nil ||
			!strings.Contains(err.Error(), "plain file name") {
			t.Errorf("name %q: %v, want a refusal naming a plain file name", name, err)
		}
	}
	if _, err := d.Capture(context.Background(), "bob", "", t.TempDir(), "x", false); err == nil ||
		!strings.Contains(err.Error(), fmt.Sprintf("no session for %q", "bob")) {
		t.Errorf("an actor with no session: %v", err)
	}
}
