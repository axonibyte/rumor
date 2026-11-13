package browser

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// RESET GIVES THE NEXT CANDIDATE AN EMPTY COOKIE JAR (issue 124's deliverable
// 4). A shrink candidate replays on a world restored to the checkpoint, and a
// session that survived from the last candidate would carry cookies a vanished
// world issued. Held against a real browser, since the claim is about what
// chromium keeps: the cookie is SEEN before the reset -- the presence partner,
// without which "no cookie after" proves nothing -- and is gone after, in a
// session that is not the old one, the old one no longer answering at all.
func TestResetLeavesTheNextSessionWithNoCookies(t *testing.T) {
	driver, found := realDriver(t)
	if !found {
		t.Skip("no chromedriver on PATH: a cookie jar needs a browser")
	}
	browserBin, bfound := realBrowser(t)
	if !bfound {
		t.Skip("no chromium on PATH")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" {
			http.SetCookie(w, &http.Cookie{Name: "who", Value: "alice-was-here", Path: "/"})
		}
		cookie := "none"
		if c, err := r.Cookie("who"); err == nil {
			cookie = c.Value
		}
		page(w, `<p id="jar">`+cookie+`</p>`)
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	d, err := Start(ctx, driver, browserBin, true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close(context.Background()) }()

	jar := func(session string) string {
		t.Helper()
		if err := d.navigate(ctx, session, srv.URL, "/look", ""); err != nil {
			t.Fatal(err)
		}
		got, ok, err := d.text(ctx, session, "#jar")
		if err != nil || !ok {
			t.Fatalf("reading the jar: %v %v", ok, err)
		}
		return strings.TrimSpace(got)
	}

	before, err := d.SessionFor(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if err := d.navigate(ctx, before, srv.URL, "/login", ""); err != nil {
		t.Fatal(err)
	}
	if got := jar(before); got != "alice-was-here" {
		t.Fatalf("the cookie was never set (%q), so its absence after a reset would prove nothing", got)
	}

	if err := d.Reset(ctx); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	after, err := d.SessionFor(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if after == before {
		t.Fatalf("Reset handed alice the same session %q", before)
	}
	if got := jar(after); got != "none" {
		t.Fatalf("after Reset alice's browser still sends a cookie: %q", got)
	}
	var out struct {
		Value any `json:"value"`
	}
	if err := d.call(ctx, http.MethodGet, "/session/"+before+"/url", nil, &out); err == nil {
		t.Fatalf("the session Reset should have deleted still answers: %v", out.Value)
	}
}
