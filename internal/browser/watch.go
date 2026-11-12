package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"forge.axonibyte.com/axonibyte/rumor/internal/websocket"
)

// A Fault is something the page did beneath an actor's step that the step's
// own result cannot show: a console error, or a response of 5xx.
//
// THIS PACKAGE HEARS FAULTS AND RECORDS NONE. Turning one into a finding --
// its kind, its attribution in the ledger, the vocabulary row -- is issue 124's
// deliverable 3 and the R9.3 lead's, so nothing here decides what a fault
// MEANS. What is decided here is what COUNTS as one, because that is a fact
// about the protocol and not about a verdict.
type Fault struct {
	// Kind is "console" or "response".
	Kind string
	// Text is the console entry's text, or the response's URL, redacted.
	Text string
	// Status is the HTTP status of a "response" fault, 0 for "console".
	Status int
}

func (f Fault) String() string {
	if f.Kind == "response" {
		return fmt.Sprintf("response %d from %s", f.Status, f.Text)
	}
	return "console error: " + f.Text
}

// counts is the filter, and it is BY LEVEL AND BY STATUS, NEVER BY PRESENCE
// (coop r9/163). Measured: with BiDi enabled, chromedriver adds an INFO entry of
// its own ("Launching Mapper instance") to every session, so a watchdog that
// fired on any entry would fire on every clean page. A console entry counts at
// level "error", the highest BiDi defines; a response counts at 500 and above,
// so a 404 is the page's business and not a fault beneath the actor.
func counts(method string, params json.RawMessage, token string) (Fault, bool) {
	switch method {
	case "log.entryAdded":
		var p struct {
			Level string `json:"level"`
			Text  string `json:"text"`
		}
		if json.Unmarshal(params, &p) != nil || p.Level != "error" {
			return Fault{}, false
		}
		return Fault{Kind: "console", Text: redact(p.Text, token)}, true
	case "network.responseCompleted":
		var p struct {
			Response struct {
				URL    string `json:"url"`
				Status int    `json:"status"`
			} `json:"response"`
		}
		if json.Unmarshal(params, &p) != nil || p.Response.Status < 500 {
			return Fault{}, false
		}
		return Fault{Kind: "response", Text: redact(p.Response.URL, token), Status: p.Response.Status}, true
	}
	return Fault{}, false
}

// A Watch is one actor's BiDi connection, listening for faults.
type Watch struct {
	conn  *websocket.Conn
	token string

	mu      sync.Mutex
	faults  []Fault
	nextID  int
	replies map[int]chan json.RawMessage
	readErr error
	done    chan struct{}
}

// WatchFor opens a BiDi connection for the actor's session and subscribes it to
// the two events a fault can arrive as. token is the actor's credential, so a
// fault's text never carries it: a navigation puts it in the URL (flow.go), and
// a 5xx on that page would otherwise report it verbatim.
//
// A DRIVER THAT OFFERS NO BIDI IS REFUSED BY NAME, not watched in silence. A
// session the driver answered without a `webSocketUrl` cannot hear a console,
// and a watchdog that returned no faults from it would read as a clean page.
func (d *Driver) WatchFor(ctx context.Context, actor, token string) (*Watch, error) {
	if _, err := d.SessionFor(ctx, actor); err != nil {
		return nil, err
	}
	u := d.bidi[actor]
	if u == "" {
		return nil, fmt.Errorf("the driver opened %q's session with no webSocketUrl, so it offers no "+
			"WebDriver BiDi and the page's console and statuses cannot be heard", actor)
	}
	conn, err := websocket.Dial(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("opening %q's BiDi connection: %w", actor, err)
	}
	w := &Watch{conn: conn, token: token, replies: map[int]chan json.RawMessage{}, done: make(chan struct{})}
	go w.read()
	if _, err := w.command(ctx, "session.subscribe", map[string]any{
		"events": []string{"log.entryAdded", "network.responseCompleted"},
	}); err != nil {
		_ = w.Close(context.Background())
		return nil, fmt.Errorf("subscribing %q's BiDi connection: %w", actor, err)
	}
	return w, nil
}

// read is the one reader of the connection: an event is filtered into faults,
// a reply is handed to the command waiting for its id.
func (w *Watch) read() {
	defer close(w.done)
	for {
		msg, err := w.conn.ReadText(context.Background())
		if err != nil {
			w.mu.Lock()
			w.readErr = err
			for _, ch := range w.replies {
				close(ch)
			}
			w.replies = map[int]chan json.RawMessage{}
			w.mu.Unlock()
			return
		}
		var m struct {
			ID     *int            `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(msg, &m) != nil {
			continue
		}
		w.mu.Lock()
		if m.ID != nil {
			if ch, ok := w.replies[*m.ID]; ok {
				ch <- msg
				delete(w.replies, *m.ID)
			}
		} else if f, ok := counts(m.Method, m.Params, w.token); ok {
			w.faults = append(w.faults, f)
		}
		w.mu.Unlock()
	}
}

// command sends one BiDi command and waits inside ctx for its reply, refusing
// an error reply by name.
func (w *Watch) command(ctx context.Context, method string, params any) (json.RawMessage, error) {
	w.mu.Lock()
	if w.readErr != nil {
		w.mu.Unlock()
		return nil, fmt.Errorf("the BiDi connection is gone: %w", w.readErr)
	}
	w.nextID++
	id := w.nextID
	ch := make(chan json.RawMessage, 1)
	w.replies[id] = ch
	w.mu.Unlock()
	body, _ := json.Marshal(map[string]any{"id": id, "method": method, "params": params})
	if err := w.conn.WriteText(ctx, body); err != nil {
		return nil, err
	}
	select {
	case reply, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("the BiDi connection closed before %s was answered", method)
		}
		var r struct {
			Type    string `json:"type"`
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(reply, &r)
		if r.Type == "error" {
			return nil, fmt.Errorf("BiDi refused %s: %s: %s", method, r.Error, r.Message)
		}
		return reply, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("BiDi did not answer %s: %w", method, ctx.Err())
	}
}

// Since is the STEP BOUNDARY, and it is a barrier rather than a timestamp.
//
// MEASURED (coop r9/163): BiDi delivered console entries logged BEFORE the
// subscription, so "what arrived since I asked" is not "what this step caused".
// The connection is ordered, so a command's reply arrives after every event
// the browser emitted before answering it: Since makes that round trip and
// then takes what has arrived, which is everything up to the boundary and
// nothing after it.
//
// The flow calls it once BEFORE a step, discarding the result, and once AFTER,
// keeping it. What it cannot see is a fault the page raises later still -- a
// timer, a slow fetch -- which lands in the NEXT step's window. That is named
// rather than waited out: a wait here would be a fixed sleep, which this stage
// has none of (R9).
func (w *Watch) Since(ctx context.Context) ([]Fault, error) {
	if _, err := w.command(ctx, "session.status", map[string]any{}); err != nil {
		return nil, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	out := w.faults
	w.faults = nil
	return out, nil
}

// Close ends the BiDi connection: a close frame, then the reader hears the
// driver's answer and stops. A driver that does not answer inside ctx has the
// socket closed under it, so Close is bounded either way. The session it
// watched is the driver's and outlives it.
func (w *Watch) Close(ctx context.Context) error {
	err := w.conn.Shutdown(ctx)
	select {
	case <-w.done:
	case <-ctx.Done():
		_ = w.conn.Abort()
		<-w.done
		if err == nil {
			err = fmt.Errorf("the driver did not answer the BiDi close: %w", ctx.Err())
		}
	}
	return err
}
