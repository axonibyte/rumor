package browser

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

// THE FAKE DRIVER CANNOT STAND IN FOR A PAGE, and these arms are split on that
// line. A fake that answered /element and /click would be a pretend browser,
// and every claim driven against it would be a claim about my own simulation --
// the fixture-override fault this tree has paid for before. So the fake covers
// exactly the paths that never reach a page (a step the driver refuses before
// it opens anything), and everything that touches a document is driven against
// a REAL chromium in TestAFlowDrivesARealPage below.

func flowSteps(steps ...map[string]any) []map[string]any { return steps }

// TestAFlowRefusesWhatItCannotDriveBeforeOpeningAnything: each of these is
// settled by reading the steps, so none of them needs a page and all of them
// are driven here.
func TestAFlowRefusesWhatItCannotDriveBeforeOpeningAnything(t *testing.T) {
	d, journal := startFake(t, "ready", true)
	defer func() { _ = d.Close(context.Background()) }()

	for _, c := range []struct {
		name  string
		req   wire.FlowRequest
		wants string
	}{
		{"no steps at all", wire.FlowRequest{Action: "a"}, "carried no steps"},
		{"a resume past the end", wire.FlowRequest{Action: "a", From: 9,
			Steps: flowSteps(map[string]any{"step": "navigate", "to": "/x"})}, "asked to resume at 9"},
		{"a resume before the start", wire.FlowRequest{Action: "a", From: -1,
			Steps: flowSteps(map[string]any{"step": "navigate", "to": "/x"})}, "asked to resume at -1"},
		{"a navigate to nowhere", wire.FlowRequest{Action: "a",
			Steps: flowSteps(map[string]any{"step": "navigate", "to": ""})}, "navigates nowhere"},
		{"an unresolved placeholder", wire.FlowRequest{Action: "a",
			Steps: flowSteps(map[string]any{"step": "navigate", "to": "/ui/events/{event}"})},
			"still carries a placeholder"},
		{"a click before anything opened", wire.FlowRequest{Action: "a",
			Steps: flowSteps(map[string]any{"step": "click", "target": "join"})},
			"clicks before anything navigated"},
		{"a fill before anything opened", wire.FlowRequest{Action: "a",
			Steps: flowSteps(map[string]any{"step": "fill", "field": "title", "value": "x"})},
			"fills before anything navigated"},
		{"a step kind nobody drives", wire.FlowRequest{Action: "a",
			Steps: flowSteps(map[string]any{"step": "hover", "target": "x"})},
			"not a step this driver drives"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := d.Flow(context.Background(), "alice", "http://127.0.0.1:1", "tok", c.req)
			if err == nil {
				t.Fatalf("%s was accepted", c.name)
			}
			if !strings.Contains(err.Error(), c.wants) {
				t.Errorf("the refusal does not say %q: %v", c.wants, err)
			}
		})
	}

	// AND NOTHING WAS OPENED, which this arm's NAME has claimed since it was
	// written and nothing checked. It was false: the session was acquired
	// before any step was read, so every case above refused with a browser
	// session already open. The gate found the sibling defect -- a flow that
	// navigated before refusing a LATER step -- and this is the same ordering
	// mistake one line further up.
	if opens := linesWith(journalLines(t, journal), "open "); len(opens) != 0 {
		t.Errorf("%d session(s) were opened for flows that could be refused by reading them: %v",
			len(opens), opens)
	}
}

// TestANavigateAgainstNothingIsRefusedRatherThanDriven is a defect this head
// found in itself, by mutation. With the adapter reporting `http://127.0.0.1:1`
// the end-to-end arm PASSED: chromedriver answered the navigate 200, loaded
// CHROME'S OWN ERROR PAGE, the click that followed found no element, and the
// walk recorded "the page would not perform it" -- a refusal attributed to a
// system that was never reached, at exit 0.
//
// W3C WEBDRIVER CARRIES NO HTTP STATUS FOR A NAVIGATION, so the browser cannot
// be asked whether the page loaded. The address can be, once per run.
//
// IT RUNS WITHOUT A BROWSER, which is why it is a fake-driven arm: the check
// happens before the driver is asked for a page, so the box this runs on needs
// no pair -- and that is also the assertion, since the fake WOULD have answered
// the navigate.
func TestANavigateAgainstNothingIsRefusedRatherThanDriven(t *testing.T) {
	d, _ := startFake(t, "ready", true)
	defer func() { _ = d.Close(context.Background()) }()

	// A PORT THAT WAS REAL AND IS NOT, rather than a number chosen for being
	// unlikely: the kernel gave it out, so it is a legal address, and closing
	// the listener is what makes "nothing answers" true of it.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	dead := ln.Addr().String()
	if cerr := ln.Close(); cerr != nil {
		t.Fatal(cerr)
	}

	_, _, ferr := d.Flow(context.Background(), "alice", "http://"+dead, "tok",
		wire.FlowRequest{Action: "join_event_ui",
			Steps: flowSteps(map[string]any{"step": "navigate", "to": "/ui/events/1"})})
	if ferr == nil {
		t.Fatal("a flow navigating at an address nothing serves was DRIVEN: the browser would " +
			"load its own error page and the next step would read as the page refusing")
	}
	for _, want := range []string{"nothing answers at", dead, "adapter"} {
		if !strings.Contains(ferr.Error(), want) {
			t.Errorf("the refusal does not say %q: %v", want, ferr)
		}
	}

	// AND THE ANSWER IS REMEMBERED, which is the other half: a walk drives many
	// flows against one base, and a dial per navigate would be a cost with no
	// second answer.
	if _, seen := d.worlds[dead]; !seen {
		t.Error("the refusal was not cached, so every navigate would dial again")
	}
}

// TestAWaitRefusesWhatNoConditionCanFix is what remains of this file's earlier
// arm. RETARGETED, NOT DELETED: that arm held `wait` refusing because its
// vocabulary was undefined, and fable DEFINED it at coop r9/110 -- so the claim
// it existed for is gone and the refusals that survive the definition are these
// two. A wait with no condition and a wait before anything opened are still
// settled by reading the steps, so neither needs a page.
func TestAWaitRefusesWhatNoConditionCanFix(t *testing.T) {
	d, _ := startFake(t, "ready", true)
	defer func() { _ = d.Close(context.Background()) }()

	for _, c := range []struct {
		name  string
		steps []map[string]any
		wants string
	}{
		{"a wait for nothing", flowSteps(
			map[string]any{"step": "navigate", "to": "/x"},
			map[string]any{"step": "wait", "until": ""}), "waits for nothing"},
		{"a wait before anything opened", flowSteps(
			map[string]any{"step": "wait", "until": "count-changed"}),
			"waits before anything navigated"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := d.Flow(context.Background(), "alice", "http://127.0.0.1:1", "tok",
				wire.FlowRequest{Action: "a", Steps: c.steps})
			if err == nil {
				t.Fatalf("%s was accepted", c.name)
			}
			if !strings.Contains(err.Error(), c.wants) {
				t.Errorf("the refusal does not say %q: %v", c.wants, err)
			}
		})
	}
}

// TestTheWaitVocabularyIsDrivenBothWays is fable's r9/110 as cells, against a
// REAL page, which is the only place a condition can be evaluated.
//
// BOTH FORMS AND THE ABSENT CASE. `<id>-changed` is satisfied when the text
// differs from what it was immediately before the preceding step; `<id>` is
// satisfied when the element is present; and an element ABSENT before counts as
// changed when it appears, which the ruling states and an implementation
// comparing against an empty string would get wrong.
func TestTheWaitVocabularyIsDrivenBothWays(t *testing.T) {
	driver, found := realDriver(t)
	if !found {
		t.Skip("no chromedriver on PATH: a condition needs a page")
	}
	browserBin, bfound := realBrowser(t)
	if !bfound {
		t.Skip("no chromium on PATH")
	}

	// THE PAGE CHANGES ON THE POST, which is what makes every case below
	// reachable: #count's text moves, #joined appears where it was absent, and
	// #never is never there at all.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.Method == http.MethodPost {
			_, _ = fmt.Fprint(w, `<!doctype html><title>after</title>`+
				`<p id="count">1 of 2</p><p id="joined">yes</p>`+
				`<form id="join" method="post" action="?"><button type="submit">join</button></form>`)
			return
		}
		_, _ = fmt.Fprint(w, `<!doctype html><title>before</title>`+
			`<p id="count">0 of 2</p>`+
			`<form id="join" method="post" action="?"><button type="submit">join</button></form>`)
	}))
	defer srv.Close()

	for _, c := range []struct {
		name    string
		until   string
		stopped wire.Stop
		reached int
		detail  string
	}{
		{"the changed form", "count-changed", wire.StopNone, 3, ""},
		{"the present form on an element that appears", "joined", wire.StopNone, 3, ""},
		{"a condition the page never meets", "never", wire.StopWait, 2, "#never read absent"},
	} {
		t.Run(c.name, func(t *testing.T) {
			d, err := Start(context.Background(), driver, browserBin, true)
			noteGroup(d)
			if err != nil {
				t.Fatalf("starting the real pair: %v", err)
			}
			defer func() { _ = d.Close(context.Background()) }()

			// THE SESSION IS OPENED BEFORE THE BUDGET STARTS, and the gate is
			// why. This cell's 2s bounded the WAIT and the browser's own
			// startup together, so when a sibling package was launching
			// chromium at the same time the session POST alone spent the
			// budget and the arm failed with "context deadline exceeded" on
			// `/session` -- a timing fact about the BOX reported as a verdict
			// about a wait. Sessions are cached per actor, so the Flow below
			// reuses this one and the budget now bounds only its subject.
			if _, serr := d.SessionFor(context.Background(), "alice"); serr != nil {
				t.Fatalf("opening a session for alice: %v", serr)
			}

			steps := flowSteps(
				map[string]any{"step": "navigate", "to": "/ui/events/e1"},
				map[string]any{"step": "click", "target": "join"},
				map[string]any{"step": "wait", "until": c.until},
			)

			// THE UNMEETABLE CASE IS DRIVEN IN TWO CALLS, AND THE FORGE IS WHY.
			// The budget is the ctx deadline by waitBudget's reading of fable's
			// ruling, so a deadline set across the WHOLE flow bounds the
			// navigate and the click as well as the wait -- and on the forge's
			// shared box, with 42 packages running at once, those two spent the
			// 2s before the wait was reached. The cell then failed with a
			// driving error about a timing fact of the BOX, reported as a
			// verdict about a wait (task 1116, 5.70s against a 2s budget).
			//
			// SO THE STEPS BEFORE IT RUN UNBOUNDED and the flow is RESUMED into
			// the wait with the budget. `from` is the product's own seam for
			// this -- the engine resumes a stopped flow the same way -- so the
			// cell drives the shipped path rather than a test-only one.
			//
			// AND 10s, NOT 2s: an unmet wait costs its whole budget, so this is
			// the cell's price, and it has to exceed the re-navigate a resume
			// performs on the slowest box this suite runs on. It is sized the
			// way ReadyTimeout was -- a measured figure with room -- and the
			// PROOF line below prints what the pre-work actually took, so the
			// next reader has the number rather than my adjective.
			ctx, from := context.Background(), 0
			if c.stopped == wire.StopWait {
				began := time.Now()
				pre, _, perr := d.Flow(ctx, "alice", srv.URL, "t",
					wire.FlowRequest{Action: "join_event_ui", Steps: steps[:2]})
				if perr != nil {
					t.Fatalf("driving the steps before the wait: %v", perr)
				}
				if pre.Reached != 2 {
					t.Fatalf("the navigate and the click reached %d of 2, so the wait was never "+
						"the subject", pre.Reached)
				}
				t.Logf("PROOF the pre-wait steps took %s, against the wait's 10s budget",
					time.Since(began).Round(time.Millisecond))
				from = 2
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
				defer cancel()
			}

			got, detail, err := d.Flow(ctx, "alice", srv.URL, "t",
				wire.FlowRequest{Action: "join_event_ui", From: from, Steps: steps})
			if err != nil {
				t.Fatalf("driving the flow: %v", err)
			}
			if got.Stopped != c.stopped {
				t.Errorf("stopped %q, want %q (detail %q)", got.Stopped, c.stopped, detail)
			}
			if got.Reached != c.reached {
				t.Errorf("reached %d, want %d", got.Reached, c.reached)
			}
			if c.detail == "" {
				if detail != "" {
					t.Errorf("a satisfied condition carries a detail: %q", detail)
				}
				return
			}
			// THE FAILURE NAMES THE CONDITION AND WHAT IT SAW, which is the
			// half of the ruling the wire shape has no field for.
			for _, want := range []string{c.until, c.detail} {
				if !strings.Contains(detail, want) {
					t.Errorf("the detail does not carry %q: %q", want, detail)
				}
			}
		})
	}
}

// TestAFlowDrivesARealPage is the browser-side arm, and it is the only one here
// that proves the driver drives: a real chromium, a real document served over
// HTTP, a real click, and the page's own state read back afterwards.
//
// THE PAGE IS THIS TEST'S OWN and shaped like cmd/toy's: an id per readable
// thing and a form whose id is what a `click target` names. Driving the toy
// itself would need the toy, its state and a session token, which is the WALK's
// business; what this arm owes is that navigate and click reach a document.
func TestAFlowDrivesARealPage(t *testing.T) {
	driver, found := realDriver(t)
	if !found {
		t.Skip("no chromedriver on PATH: the browser-side arms run where the pair lives")
	}
	browserBin, bfound := realBrowser(t)
	if !bfound {
		t.Skip("no chromium on PATH")
	}

	// A CHANNEL AND NOT A BOOL, for two reasons the first version of this arm
	// had wrong. The handler runs on the server's goroutine and the assertion on
	// the test's, so a plain bool is a data race; and a W3C click returns when
	// the click is DISPATCHED, so the POST it causes may still be in flight.
	// This arm failed in a group run and passed alone until it waited for the
	// effect instead of reading a variable.
	//
	// WAITING HERE IS NOT THE `wait` STEP. The flow's own wait is refused above
	// while its vocabulary is undefined; this is the test synchronising with its
	// own server, which is a different thing and must not be confused for one.
	joined := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/join") {
			select {
			case joined <- struct{}{}:
			default:
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(w, `<!doctype html><title>joined</title><p id="count">1 of 2</p>`)
			return
		}
		// THE TOKEN MUST ARRIVE, and the page says so rather than the arm
		// trusting the URL it built: cmd/toy serves 401 without one.
		if r.URL.Query().Get("token") == "" {
			http.Error(w, "the page needs a session", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<!doctype html><title>event</title>
<p id="count">0 of 2</p>
<form id="join" method="post" action="%s/join?token=t"><button type="submit">join</button></form>`,
			r.URL.Path)
	}))
	defer srv.Close()

	d, err := Start(context.Background(), driver, browserBin, true)
	noteGroup(d)
	if err != nil {
		t.Fatalf("starting the real pair: %v", err)
	}
	defer func() { _ = d.Close(context.Background()) }()

	resp, _, err := d.Flow(context.Background(), "alice", srv.URL, "t",
		wire.FlowRequest{Action: "join_event_ui", Steps: flowSteps(
			map[string]any{"step": "navigate", "to": "/ui/events/e1"},
			map[string]any{"step": "click", "target": "join"},
		)})
	if err != nil {
		t.Fatalf("driving navigate+click against a real page: %v", err)
	}
	if resp.Reached != 2 {
		t.Errorf("reached %d of 2 steps, stopped %q", resp.Reached, resp.Stopped)
	}
	if resp.Stopped != wire.StopNone {
		t.Errorf("a flow that drove both steps reports stopped %q", resp.Stopped)
	}
	// THE CLICK IS PROVED BY THE SERVER, not by the absence of an error: a
	// driver that found the button and never clicked it would answer the same
	// way, which is the shape of every claim this phase has had to re-measure.
	select {
	case <-joined:
	case <-time.After(10 * time.Second):
		t.Errorf("the flow reported 2 steps and the page was never POSTed to: the click did nothing")
	}
	// AND NO RESPONSE IS CLAIMED, because a W3C driver is not told a status --
	// examples/toy/bindings.toml:305-309 leaves what a browser "response" means
	// to this phase, and a status it cannot know would be a non-status dressed
	// as one.
	if resp.Response != nil {
		t.Errorf("the flow invented a response a browser cannot see: %#v", resp.Response)
	}
}

// TestAMissingTargetStopsTheFlowRatherThanFailingIt: a target the page does not
// carry is the SYSTEM's answer, so it stops at that step the way the adapter's
// 4xx does, and is not an error about this build.
func TestAMissingTargetStopsTheFlowRatherThanFailingIt(t *testing.T) {
	driver, found := realDriver(t)
	if !found {
		t.Skip("no chromedriver on PATH")
	}
	browserBin, bfound := realBrowser(t)
	if !bfound {
		t.Skip("no chromium on PATH")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<!doctype html><title>bare</title><p id="count">0 of 2</p>`)
	}))
	defer srv.Close()

	d, err := Start(context.Background(), driver, browserBin, true)
	noteGroup(d)
	if err != nil {
		t.Fatalf("starting the real pair: %v", err)
	}
	defer func() { _ = d.Close(context.Background()) }()

	resp, _, err := d.Flow(context.Background(), "alice", srv.URL, "t",
		wire.FlowRequest{Action: "join_event_ui", Steps: flowSteps(
			map[string]any{"step": "navigate", "to": "/ui/events/e1"},
			map[string]any{"step": "click", "target": "join"},
		)})
	if err != nil {
		t.Fatalf("a missing target was an error rather than a stop: %v", err)
	}
	if resp.Stopped != wire.StopStep {
		t.Errorf("stopped %q, want %q", resp.Stopped, wire.StopStep)
	}
	if resp.Reached != 1 {
		t.Errorf("reached %d, want 1 -- the navigate landed and the click did not", resp.Reached)
	}
}

// TestTwoActorsSessionsDoNotLeakThroughAFlow is ISSUE 122'S FIRST TEST, and
// until this head it existed only as prose: browser.go's "a W3C session carries
// its own cookie jar and storage" was asserted by nobody, and the nearest arm --
// TestEachActorGetsItsOwnSession -- compares session IDS on the wire against a
// FAKE and never opens a page. fable ruled this shape at coop r9/118 after ibis
// reported the gap at r9/117; the wire-level arm stays as itself and is not this.
//
// THE SHAPE, as ruled: the first actor acts so that its session holds something
// the page SHOWS BACK, the second's flow reads the same page and sees NONE of
// it, and THE PRESENCE PARTNER IS THE FIRST ACTOR RE-READING AND SEEING ITS OWN,
// IN THE SAME ARM -- so a driver that sees nothing at all fails on the partner
// rather than passing the absence. An absence assertion without its partner is
// the vacuity this tree refuses by name.
//
// A COOKIE IS THE SUBJECT because it is what the claim is about. The login sets
// one and every later page reads ONLY the cookie, never the query, so what the
// page shows is a fact about whose session asked -- which is exactly the leak a
// shared jar would produce.
//
// DRIVEN ONLY WHERE THE SCAN READS A PAIR, skipped by that verdict and never by
// a fallback: an adapter standing in here would answer about no browser at all.
func TestTwoActorsSessionsDoNotLeakThroughAFlow(t *testing.T) {
	driver, found := realDriver(t)
	if !found {
		t.Skip("no chromedriver on PATH: the browser-side arms run where the pair lives")
	}
	browserBin, bfound := realBrowser(t)
	if !bfound {
		t.Skip("no chromium on PATH")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.URL.Path == "/login" {
			// THE TOKEN IS THE IDENTITY, and navigate is what puts it in the
			// URL: the driver appends `?token=` for the actor it drives, so
			// each actor signs in as itself without this arm building a URL.
			who := r.URL.Query().Get("token")
			http.SetCookie(w, &http.Cookie{Name: "who", Value: who, Path: "/"})
			_, _ = fmt.Fprintf(w, `<!doctype html><title>in</title><p id="who">%s</p>`, who)
			return
		}
		// THE QUERY IS IGNORED HERE, deliberately, and ROOK MEASURED MY FIRST
		// REASON FOR IT WRONG (rumor/1950). I wrote that a page reading the
		// token would let this arm "pass on a shared cookie jar"; driven, it
		// FAILS -- bob reads himself, not nobody.
		//
		// THE GUARD IS STILL NEEDED, for the better reason rook gave: under
		// query-reading the verdict stops depending on the JAR at all, so the
		// arm measures nothing about isolation, and the presence partner goes
		// vacuous with it -- alice would read alice with no cookie anywhere.
		// Only the COOKIE decides, so only the cookie can be shared.
		who := "nobody"
		if c, cerr := r.Cookie("who"); cerr == nil {
			who = c.Value
		}
		_, _ = fmt.Fprintf(w, `<!doctype html><title>page</title><p id="who">%s</p>`, who)
	}))
	defer srv.Close()

	d, err := Start(context.Background(), driver, browserBin, true)
	noteGroup(d)
	if err != nil {
		t.Fatalf("starting the real pair: %v", err)
	}
	defer func() { _ = d.Close(context.Background()) }()

	ctx := context.Background()
	navigate := func(actor, token, to string) {
		t.Helper()
		if _, _, ferr := d.Flow(ctx, actor, srv.URL, token, wire.FlowRequest{
			Action: "visit", Steps: flowSteps(map[string]any{"step": "navigate", "to": to}),
		}); ferr != nil {
			t.Fatalf("%s navigating %s: %v", actor, to, ferr)
		}
	}
	reads := func(actor string) string {
		t.Helper()
		session, serr := d.SessionFor(ctx, actor)
		if serr != nil {
			t.Fatalf("session for %s: %v", actor, serr)
		}
		got, present, terr := d.text(ctx, session, "#who")
		if terr != nil {
			t.Fatalf("reading #who for %s: %v", actor, terr)
		}
		if !present {
			t.Fatalf("%s sees no #who element at all, so this arm can say nothing about "+
				"what leaked: the page did not render for it", actor)
		}
		return got
	}

	// ALICE SIGNS IN, so her session now holds a cookie the page shows back.
	navigate("alice", "alice", "/login")

	// THE PRESENCE PARTNER FIRST, because it is what makes the absence below
	// mean anything: alice re-reads the ordinary page and must see HER OWN.
	navigate("alice", "alice", "/page")
	if got := reads("alice"); got != "alice" {
		t.Fatalf("alice re-read the page as %q, want alice: her own session's cookie did not "+
			"reach it, so the absence asserted below would hold for a driver that carries no "+
			"cookies at all", got)
	}

	// AND BOB SEES NONE OF IT. Same page, a session that never signed in.
	navigate("bob", "bob", "/page")
	if got := reads("bob"); got != "nobody" {
		t.Errorf("bob read the page as %q, want nobody: one actor's cookies reached another's "+
			"flow, so two actors share a jar and either could act as the other", got)
	}

	// AND THE SESSIONS ARE TWO, which is the wire-level claim holding at the
	// same time rather than instead: one session answering both actors would
	// produce the leak above by a different route.
	alice, _ := d.SessionFor(ctx, "alice")
	bob, _ := d.SessionFor(ctx, "bob")
	if alice == bob {
		t.Errorf("alice and bob share session %q", alice)
	}
}

// realDriver and realBrowser answer what is on PATH, so the browser-side arms
// skip by name where the pair is absent rather than failing a browserless box.
func realDriver(t *testing.T) (string, bool) {
	t.Helper()
	return lookPath("chromedriver")
}

func realBrowser(t *testing.T) (string, bool) {
	t.Helper()
	for _, n := range []string{"chrome", "chromium", "chromium-browser", "google-chrome"} {
		if p, ok := lookPath(n); ok {
			return p, true
		}
	}
	return "", false
}

func lookPath(name string) (string, bool) {
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

// TestClosingTheDriverLeavesNoBrowserBehind holds deliverable 1's "nothing is
// left running" against a REAL browser, which is the only place it can be
// tested: a fake driver spawns nothing, so every earlier arm for this claim
// measured a process with no children.
//
// AND IT WAS FALSE UNTIL MEASURED. Driving these flow arms, chromedriver was
// reaped and gone while SIX `chrome:` processes stayed alive, reparented onto
// the test binary -- `go test` then waited on them until it was killed at 110s
// having passed in 0.8s. The same orphans explain 19 strays found in this jail
// earlier the same night. Closing a session ASKS the browser to go and the
// driver answers before it has gone, so a signal to the driver alone can outrun
// the browser's exit.
//
// THE PROCESS GROUP IS THE ASSERTION, not a count of chrome processes: `kill(0)`
// to a GROUP answers ESRCH exactly when no member is left, which is the claim
// stated as a system call rather than as a pattern over `ps` output that a
// browser could rename its way out of.
func TestClosingTheDriverLeavesNoBrowserBehind(t *testing.T) {
	driver, found := realDriver(t)
	if !found {
		t.Skip("no chromedriver on PATH: this claim needs a driver that spawns a browser")
	}
	browserBin, bfound := realBrowser(t)
	if !bfound {
		t.Skip("no chromium on PATH")
	}

	d, err := Start(context.Background(), driver, browserBin, true)
	noteGroup(d)
	if err != nil {
		t.Fatalf("starting the real pair: %v", err)
	}
	// A SESSION IS NOT ENOUGH: IT MUST HAVE DRIVEN A PAGE. The first version of
	// this arm opened a session and closed it, and the mutant restoring the
	// exact code that orphaned six chrome processes SURVIVED -- a blank browser
	// tears down on a SIGINT where one holding a document does not. An arm that
	// cannot reach the state the defect needs is not evidence about it, however
	// directly it asserts the claim.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<!doctype html><title>held</title>
<p id="count">0 of 2</p>
<form id="join" method="post" action="%s/join?token=t"><button type="submit">join</button></form>`,
			r.URL.Path)
	}))
	defer srv.Close()
	if _, _, err := d.Flow(context.Background(), "alice", srv.URL, "t",
		wire.FlowRequest{Action: "held", Steps: flowSteps(
			map[string]any{"step": "navigate", "to": "/ui/events/e1"},
			map[string]any{"step": "click", "target": "join"},
		)}); err != nil {
		_ = d.Close(context.Background())
		t.Fatalf("driving a page before the teardown: %v", err)
	}
	pgid := d.cmd.Process.Pid
	if err := syscall.Kill(-pgid, 0); err != nil {
		t.Fatalf("the driver's group is already empty with a session open, so this arm has no "+
			"subject: %v", err)
	}

	if err := d.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// THE GROUP MUST BE EMPTY. A browser still running is one `kill(0)` away
	// from being detected, and the window is bounded: a group that empties
	// slowly is still a group that empties.
	var last error
	for i := 0; i < 100; i++ {
		last = syscall.Kill(-pgid, 0)
		if last != nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("5s after Close the driver's process group %d still has members (kill(0) = %v): "+
		"the browser outlived the driver", pgid, last)
}

// TestTheActorsTokenReachesNoOutput is fable's own arm (coop r9/110): "a test
// that greps the stage's output for the token and finds it is the arm".
//
// THE TOKEN HAS TO TRAVEL IN THE URL because cmd/toy authenticates by an
// Authorization header or `?token=` and a browser navigation can set no header.
// So it is in a URL by necessity, and the rule is that it appears nowhere a
// human or a log will see. The leak is not in what this package FORMATS -- no
// error here prints the built URL -- it is in what it PASSES THROUGH:
// chromedriver's message for a failed navigation can echo the URL back, and a
// page's own text can carry the token, the toy's form action being exactly that.
//
// SO THE ARM DRIVES BOTH PATHS WITH A TOKEN THAT CANNOT OCCUR BY ACCIDENT, and
// greps everything the driver hands back.
func TestTheActorsTokenReachesNoOutput(t *testing.T) {
	driver, found := realDriver(t)
	if !found {
		t.Skip("no chromedriver on PATH")
	}
	browserBin, bfound := realBrowser(t)
	if !bfound {
		t.Skip("no chromium on PATH")
	}
	const token = "ZZ-SECRET-TOKEN-8f3a1c-ZZ"

	// THE PAGE PRINTS THE TOKEN BACK, which is the toy's own shape: its form
	// action embeds the session token, so a condition reading that element's
	// text would carry the credential into a detail.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<!doctype html><title>t</title><p id="leak">token=%s</p>`,
			r.URL.Query().Get("token"))
	}))
	defer srv.Close()

	d, err := Start(context.Background(), driver, browserBin, true)
	noteGroup(d)
	if err != nil {
		t.Fatalf("starting the real pair: %v", err)
	}
	defer func() { _ = d.Close(context.Background()) }()

	// THE SESSION IS OPENED FIRST, for the reason the wait-vocabulary arm above
	// states: a budget covering the browser's startup as well as the wait makes
	// the arm a measurement of how busy the box is.
	if _, serr := d.SessionFor(context.Background(), "alice"); serr != nil {
		t.Fatalf("opening a session for alice: %v", serr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// A CONDITION THAT CANNOT BE MET, so the detail is produced and carries the
	// text of an element the page filled with the token.
	_, detail, ferr := d.Flow(ctx, "alice", srv.URL, token,
		wire.FlowRequest{Action: "leaky", Steps: flowSteps(
			// TWO NAVIGATES, so the condition has a real BEFORE to compare
			// against: the element exists with the token in it, the second
			// navigate leaves it the same, and the condition therefore cannot
			// be met -- which is what produces a detail carrying the text.
			map[string]any{"step": "navigate", "to": "/page"},
			map[string]any{"step": "navigate", "to": "/page"},
			map[string]any{"step": "wait", "until": "leak-changed"},
		)})

	var text string
	if ferr != nil {
		text += ferr.Error()
	}
	text += detail
	if text == "" {
		t.Fatalf("the flow produced neither an error nor a detail, so this arm greps nothing")
	}
	if strings.Contains(text, token) {
		t.Errorf("the actor's token reached the driver's output: %q", text)
	}
	// AND THE REDACTION IS VISIBLE rather than a silent deletion, so a reader
	// knows a value was removed and does not read the gap as "no token".
	if !strings.Contains(text, Redacted) {
		t.Errorf("nothing was redacted, so this arm may be passing because the token never "+
			"reached the text at all: %q", text)
	}
}

// TestRedactionTakesWholeValuesAndNotSubstrings holds the boundary rule, which
// a measured corruption bought. The wait-vocabulary arm drives with the token
// "t", and a blanket ReplaceAll turned `"never" was not met: #never read absent`
// into `was no<token redacted> me<token redacted>` -- every letter t in the
// sentence redacted, and a detail no reader could use.
//
// NO LENGTH THRESHOLD, because that is a number chosen rather than measured.
// The rule is that a token is a whole value: an occurrence counts only where
// what surrounds it is not itself value-shaped.
func TestRedactionTakesWholeValuesAndNotSubstrings(t *testing.T) {
	for _, c := range []struct {
		name, text, token, want string
	}{
		{"a short token inside words is left alone",
			`"never" was not met: #never read absent`, "t",
			`"never" was not met: #never read absent`},
		{"the same short token as a whole value is taken",
			"token=t&x=1", "t", "token=" + Redacted + "&x=1"},
		{"a realistic token is taken",
			"navigating to /p?token=ZZ-SECRET-8f3a1c failed", "ZZ-SECRET-8f3a1c",
			"navigating to /p?token=" + Redacted + " failed"},
		{"a token that is part of a longer value is not a match",
			"token=ZZ-SECRET-8f3a1c-EXTRA", "ZZ-SECRET-8f3a1c",
			"token=ZZ-SECRET-8f3a1c-EXTRA"},
		{"an empty token rewrites nothing",
			"nothing to hide", "", "nothing to hide"},
		{"every whole occurrence goes, not just the first",
			"a=K1 b=K1", "K1", "a=" + Redacted + " b=" + Redacted},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := redact(c.text, c.token); got != c.want {
				t.Errorf("redact(%q, %q)\n got %q\nwant %q", c.text, c.token, got, c.want)
			}
		})
	}
}

// TestAPageThatMovesMidReadIsNotAFailure is the defect the FORGE found and this
// box hid: task 1116 failed `count-changed` with "stale element reference",
// because a wait reads an element in TWO calls -- find, then text -- and the
// click's POST response landed between them. It reproduced 1 run in 6 here,
// which is exactly why the gate passed and CI did not.
//
// A STALE REFERENCE IS THE PAGE MOVING, and for a wait that is the thing it is
// waiting FOR. So the poll takes no reading this tick and re-finds on the next,
// and the flow reports the wait unmet at its budget rather than a driving error.
//
// THE FAKE PRODUCES THE RACE ON DEMAND, which no real page can be made to do:
// every call it answers here is a FAILURE, so nothing about a page can pass on
// it -- the distinction that keeps it out of the rule this file's other arms
// state.
func TestAPageThatMovesMidReadIsNotAFailure(t *testing.T) {
	d, _ := startFake(t, "stale", true)
	defer func() { _ = d.Close(context.Background()) }()

	// A LISTENER FOR THE NAVIGATE TO REACH, because the flow really does
	// navigate: this arm pointed at a dead port and passed until the look-ahead
	// stopped skipping the step it looks ahead from -- which is the defect
	// above. Nothing is read FROM this server; the fake answers the protocol.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<!doctype html><title>x</title><p id="count">0</p>`)
	}))
	defer srv.Close()

	// THE SESSION FIRST, outside the budget, which the guard in browser_test.go
	// requires of every arm that bounds a flow: against this fake the open is
	// instant, and the rule is uniform so the next arm is not asked to judge
	// whether its own driver is fast enough.
	if _, serr := d.SessionFor(context.Background(), "alice"); serr != nil {
		t.Fatalf("opening a session for alice: %v", serr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	got, detail, err := d.Flow(ctx, "alice", srv.URL, "t",
		wire.FlowRequest{Action: "join_event_ui", From: 1, Steps: flowSteps(
			map[string]any{"step": "navigate", "to": "/ui/events/e1"},
			map[string]any{"step": "wait", "until": "count-changed"},
		)})
	if err != nil {
		t.Fatalf("a page that moved under the read was reported as a failure: %v", err)
	}
	if got.Stopped != wire.StopWait {
		t.Errorf("stopped %q, want %q: a condition that never got a reading is unmet, which the "+
			"engine re-checks -- not an error that ends the run", got.Stopped, wire.StopWait)
	}
	if got.Reached != 1 {
		t.Errorf("reached %d, want 1: the wait is where it stopped", got.Reached)
	}
	if detail == "" {
		t.Error("the unmet wait carries no detail, so a reader cannot tell what it saw")
	}
}

// AND A 404 THAT IS NOT STALENESS STAYS AN ERROR. The protocol answers 404 for
// "no such element" and "no such window" as well, so matching the STATUS would
// swallow a selector that matches nothing into silence -- the matcher keys on
// the W3C error NAME, and this holds the exact message the forge produced.
func TestOnlyAStaleReferenceIsTreatedAsAMovedPage(t *testing.T) {
	for _, c := range []struct {
		name  string
		err   error
		stale bool
	}{
		{"the forge's own message", fmt.Errorf(`GET /session/x/element/y/text answered 404 Not ` +
			`Found: {"value":{"error":"stale element reference","message":"stale element ` +
			`reference: stale element not found\n  (Session info: chrome=156.0.8078.79)"}}`), true},
		{"no such element", fmt.Errorf(`answered 404 Not Found: {"value":{"error":"no such ` +
			`element","message":"no such element: Unable to locate element"}}`), false},
		{"no such window", fmt.Errorf(`answered 404 Not Found: {"value":{"error":"no such ` +
			`window","message":"no such window: target window already closed"}}`), false},
		{"nothing at all", nil, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := isStale(c.err); got != c.stale {
				t.Errorf("isStale is %v, want %v, for %v", got, c.stale, c.err)
			}
		})
	}
}

// TestAWaitRightAfterTheNavigateStillNavigates is the defect the arm above found
// by accident, held on purpose: the look-ahead that captures a `-changed` wait's
// BEFORE text ended in `continue`, which continues the STEP LOOP -- so when the
// wait followed the very first step, the step it looked ahead from was never
// performed.
//
// WHY EVERY OTHER ARM PASSED OVER IT: in the toy's own flow the `-changed` wait
// follows a CLICK, so `path` is already set by the navigate before it and the
// branch with the `continue` was never taken. It fires only where the wait
// follows the FIRST step, which no fixture here had until this one.
//
// THE DISCRIMINATOR IS THE VERDICT, not a log: with the navigate skipped the
// element never appears, so the wait is unmet and the flow stops at it; with the
// navigate performed the element appears, which the ruling makes a CHANGE from
// absent, so the flow runs to the end.
func TestAWaitRightAfterTheNavigateStillNavigates(t *testing.T) {
	driver, found := realDriver(t)
	if !found {
		t.Skip("no chromedriver on PATH: a condition needs a page")
	}
	browserBin, bfound := realBrowser(t)
	if !bfound {
		t.Skip("no chromium on PATH")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<!doctype html><title>late</title><p id="count">3 of 4</p>`)
	}))
	defer srv.Close()

	d, err := Start(context.Background(), driver, browserBin, true)
	noteGroup(d)
	if err != nil {
		t.Fatalf("starting the real pair: %v", err)
	}
	defer func() { _ = d.Close(context.Background()) }()

	// THE SESSION IS OPENED BEFORE THE BUDGET STARTS, and I had to be told
	// twice: the cell two above this one carries the same sentence, and I wrote
	// this one with the deadline around the WHOLE flow anyway. The gate caught
	// it at once -- "opening a session for alice: context deadline exceeded" --
	// which is a timing fact about the box reported as a verdict about a wait.
	if _, serr := d.SessionFor(context.Background(), "alice"); serr != nil {
		t.Fatalf("opening a session for alice: %v", serr)
	}

	// A SHORT BUDGET, because a flow that fails to navigate stops at the wait
	// and spends it: the cell should cost a second, not the whole bound. It
	// bounds the WAIT alone now, which is the only thing it is about.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	got, detail, ferr := d.Flow(ctx, "alice", srv.URL, "t",
		wire.FlowRequest{Action: "join_event_ui", Steps: flowSteps(
			map[string]any{"step": "navigate", "to": "/ui/events/e1"},
			map[string]any{"step": "wait", "until": "count-changed"},
		)})
	if ferr != nil {
		t.Fatalf("driving the flow: %v", ferr)
	}
	if got.Stopped != wire.StopNone {
		t.Errorf("stopped %q with detail %q, want it to run through: #count is on the page the "+
			"navigate opens, and it was ABSENT before -- which the ruling makes a change. A stop "+
			"here means the navigate did not happen", got.Stopped, detail)
	}
	if got.Reached != 2 {
		t.Errorf("reached %d of 2 steps", got.Reached)
	}
}
