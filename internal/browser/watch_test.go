package browser

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// THE FILTER, WITHOUT A BROWSER. What counts as a fault is decided by level and
// status and never by presence (coop r9/163), and this table is that decision
// held where every box can run it. The real-pair arms below hold it against
// what chromedriver actually sends.
func TestAFaultIsCountedByLevelAndStatusNotByPresence(t *testing.T) {
	const tok = "tok-s3cret-91"
	for _, c := range []struct {
		name, method, params string
		want                 string // the Fault's String(), or "" for not counted
	}{
		{"a console error", "log.entryAdded", `{"level":"error","text":"boom"}`, "console error: boom"},
		{"a console warning", "log.entryAdded", `{"level":"warn","text":"careful"}`, ""},
		{"chromedriver's own mapper line", "log.entryAdded",
			`{"level":"info","text":"Launching Mapper instance with selfTargetId:"}`, ""},
		{"a 500", "network.responseCompleted", `{"response":{"url":"http://h/a","status":500}}`, "response 500 from http://h/a"},
		{"a 503", "network.responseCompleted", `{"response":{"url":"http://h/b","status":503}}`, "response 503 from http://h/b"},
		{"a 404, the page's own business", "network.responseCompleted", `{"response":{"url":"http://h/c","status":404}}`, ""},
		{"a 200", "network.responseCompleted", `{"response":{"url":"http://h/d","status":200}}`, ""},
		{"an event this watch does not read", "browsingContext.load", `{"url":"http://h/"}`, ""},
		{"the token in a URL", "network.responseCompleted",
			`{"response":{"url":"http://h/e?token=` + tok + `","status":502}}`, "response 502 from http://h/e?token=" + Redacted},
		{"the token in a console line", "log.entryAdded", `{"level":"error","text":"bad ` + tok + `"}`, "console error: bad " + Redacted},
	} {
		t.Run(c.name, func(t *testing.T) {
			f, ok := counts(c.method, json.RawMessage(c.params), tok)
			got := ""
			if ok {
				got = f.String()
			}
			if got != c.want {
				t.Fatalf("counts(%s, %s) = %q, want %q", c.method, c.params, got, c.want)
			}
		})
	}
}

// A driver that answers a session with no webSocketUrl cannot hear a console,
// and a watch returning no faults from it would read as a clean page. The fake
// is such a driver: it answers `"capabilities":{}`.
func TestADriverThatOffersNoBiDiIsRefusedByName(t *testing.T) {
	path, _ := fakeDriver(t, "ready")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	d, err := Start(ctx, path, "", true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close(context.Background()) }()
	w, err := d.WatchFor(ctx, "alice", "")
	if err == nil {
		_ = w.Close(ctx)
		t.Fatal("WatchFor succeeded against a driver offering no BiDi")
	}
	if !strings.Contains(err.Error(), "no webSocketUrl") {
		t.Fatalf("WatchFor: %v, want a refusal naming the missing webSocketUrl", err)
	}
}

// realWatch starts the real pair, opens alice's session and her watch, and
// serves pages from h. It skips by name where the pair is absent, as every
// browser-side arm in this package does.
func realWatch(t *testing.T, token string, h http.HandlerFunc) (context.Context, *Driver, *Watch, string, string) {
	t.Helper()
	driver, found := realDriver(t)
	if !found {
		t.Skip("no chromedriver on PATH: a console needs a page")
	}
	browserBin, bfound := realBrowser(t)
	if !bfound {
		t.Skip("no chromium on PATH")
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	// 180s: the rate arm dwells 15s on each of five pages (rate_test.go).
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	t.Cleanup(cancel)
	d, err := Start(ctx, driver, browserBin, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close(context.Background()) })
	w, err := d.WatchFor(ctx, "alice", token)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close(context.Background()) })
	return ctx, d, w, d.sessions["alice"], srv.URL
}

// step navigates alice to a page between two boundaries and returns what the
// watch heard inside them -- the shape a flow step will have.
func step(t *testing.T, ctx context.Context, d *Driver, w *Watch, session, base, page, token string) []Fault {
	t.Helper()
	if _, err := w.Since(ctx); err != nil {
		t.Fatal(err)
	}
	if err := d.navigate(ctx, session, base, page, token); err != nil {
		t.Fatal(err)
	}
	got, err := w.Since(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func page(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprint(w, "<!doctype html><title>t</title>"+body)
}

// THE CONSOLE ARM, with its quiet partner on a clean page in the same session.
//
// AND THE REPLAY TRAP (coop r9/163): the faulty page is visited once BEFORE a
// boundary, so a watch that attributed by arrival rather than by boundary would
// hand that error to the clean step after it.
func TestAConsoleErrorBeneathAStepIsHeard(t *testing.T) {
	ctx, d, w, session, base := realWatch(t, "", func(rw http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/faulty":
			page(rw, `<script>console.error("planted console error")</script>`)
		default:
			page(rw, `<script>console.log("an info line is not a fault")</script>`)
		}
	})
	if err := d.navigate(ctx, session, base, "/faulty", ""); err != nil {
		t.Fatal(err)
	}
	if got := step(t, ctx, d, w, session, base, "/clean", ""); len(got) != 0 {
		t.Fatalf("a clean step heard %v: a fault from before its boundary, or an info line, was counted", got)
	}
	got := step(t, ctx, d, w, session, base, "/faulty", "")
	if len(got) != 1 || got[0].Kind != "console" || !strings.Contains(got[0].Text, "planted console error") {
		t.Fatalf("the faulty step heard %v, want exactly the planted console error", got)
	}
}

// THE 5xx ARM, with a 404 beside it that must not count, and its quiet partner.
// The 503 is the DOCUMENT, navigated with the actor's token in its URL, so the
// arm also holds that a fault never carries the credential.
func TestA5xxBeneathAStepIsHeardAndCarriesNoToken(t *testing.T) {
	const tok = "tok-s3cret-91"
	ctx, d, w, session, base := realWatch(t, tok, func(rw http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/down":
			rw.WriteHeader(http.StatusServiceUnavailable)
			page(rw, `<p>down</p>`)
		case "/missing.png":
			http.NotFound(rw, r)
		default:
			page(rw, `<p>up</p><img src="/missing.png">`)
		}
	})
	if got := step(t, ctx, d, w, session, base, "/clean", tok); len(got) != 0 {
		t.Fatalf("a clean step with a 404 image heard %v: a 404 is the page's own business", got)
	}
	got := step(t, ctx, d, w, session, base, "/down", tok)
	if len(got) != 1 || got[0].Kind != "response" || got[0].Status != 503 || !strings.Contains(got[0].Text, "/down") {
		t.Fatalf("the step onto a 503 heard %v, want exactly that response", got)
	}
	if strings.Contains(got[0].Text, tok) {
		t.Fatalf("the fault carries the actor's token: %q", got[0].Text)
	}
}

// THE BOUNDARY IS A BARRIER, AND THIS ARM HOLDS WHAT THAT BUYS. On a real
// chromium the events land before navigate returns, so the real-pair arms pass
// with the barrier removed -- measured, three runs each. What the barrier
// promises is ORDER: an event the browser sends before answering it counts in
// the step it closes. A fake BiDi peer makes that deterministic by sending its
// fault only on receiving the barrier, then answering. Without the round trip
// Since returns before the fault exists, and this arm fails.
func TestAFaultSentBeforeTheBarrierIsAnsweredCountsInItsStep(t *testing.T) {
	url := fakeBiDi(t, func(method string) []string {
		if method != "session.status" {
			return nil
		}
		return []string{`{"type":"event","method":"log.entryAdded","params":{"level":"error","text":"late but inside"}}`}
	}, true)
	d := &Driver{sessions: map[string]string{"alice": "s1"}, bidi: map[string]string{"alice": url}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	w, err := d.WatchFor(ctx, "alice", "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w.Close(ctx) }()
	got, err := w.Since(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Text != "late but inside" {
		t.Fatalf("the step heard %v, want the fault sent before the barrier was answered", got)
	}
}

// fakeBiDi is a WebDriver BiDi peer over a websocket, written from RFC 6455
// directly rather than through internal/websocket so it cannot share a framing
// mistake with the client. It answers every command with success, sending
// first whatever before(method) returns.
func fakeBiDi(t *testing.T, before func(method string) []string, answerClose bool) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	t.Cleanup(func() { _ = ln.Close(); <-done })
	go func() {
		defer close(done)
		nc, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = nc.Close() }()
		_ = nc.SetDeadline(time.Now().Add(10 * time.Second))
		br := bufio.NewReader(nc)
		req, err := http.ReadRequest(br)
		if err != nil {
			return
		}
		sum := sha1.Sum([]byte(req.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
		_, _ = fmt.Fprintf(nc, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n"+
			"Sec-WebSocket-Accept: %s\r\n\r\n", base64.StdEncoding.EncodeToString(sum[:]))
		send := func(op byte, p []byte) { // unmasked, under 126 bytes, as every message here is
			_, _ = nc.Write(append([]byte{0x80 | op, byte(len(p))}, p...))
		}
		for {
			var h [2]byte
			if _, err := io.ReadFull(br, h[:]); err != nil {
				return
			}
			var key [4]byte
			_, _ = io.ReadFull(br, key[:]) // a client frame here is always short and masked
			p := make([]byte, h[1]&0x7F)
			_, _ = io.ReadFull(br, p)
			for i := range p {
				p[i] ^= key[i%4]
			}
			if h[0]&0x0F == 0x8 {
				if !answerClose {
					_, _ = io.Copy(io.Discard, br) // heard the close, never answers; ends when the client hangs up
					return
				}
				send(0x8, p)
				return
			}
			var cmd struct {
				ID     int    `json:"id"`
				Method string `json:"method"`
			}
			_ = json.Unmarshal(p, &cmd)
			for _, ev := range before(cmd.Method) {
				send(0x1, []byte(ev))
			}
			send(0x1, []byte(fmt.Sprintf(`{"type":"success","id":%d,"result":{}}`, cmd.ID)))
		}
	}()
	return "ws://" + ln.Addr().String() + "/session/s1"
}

// CLOSE IS BOUNDED EITHER WAY: a driver that never answers the BiDi close has
// the socket closed under it when the caller's time runs out, so tearing down a
// watch cannot hang a run.
func TestAWatchWhoseDriverNeverAnswersTheCloseStillCloses(t *testing.T) {
	url := fakeBiDi(t, func(string) []string { return nil }, false)
	d := &Driver{sessions: map[string]string{"alice": "s1"}, bidi: map[string]string{"alice": url}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	w, err := d.WatchFor(ctx, "alice", "")
	if err != nil {
		t.Fatal(err)
	}
	short, cancelShort := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancelShort()
	got := make(chan error, 1)
	go func() { got <- w.Close(short) }()
	select {
	case err := <-got:
		if err == nil || !strings.Contains(err.Error(), "did not answer the BiDi close") {
			t.Fatalf("Close: %v, want an error naming the unanswered close", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Close was still waiting 3s after a 300ms bound")
	}
}
