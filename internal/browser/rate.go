package browser

import (
	"encoding/json"
	"fmt"
	"time"
)

// Rate is the request-rate arm's threshold: AT LEAST PerSecond responses in
// EVERY one of the window's whole seconds (fable's rulings at coop r9/173 and
// r9/179, from the measurement at r9/172).
//
// THE DEFAULT IS 20 PER SECOND OVER 2 SECONDS, and it was 10 for one gate.
// Chromium 156, two seconds after load: an honest page added about one
// response a second (a 1s poll, 3 in the window), a fetch loop 260 to 374, and
// a 100ms poll 20. At 10 the 100ms poll sat exactly on the line, and the gate
// MEASURED what that costs: it fired 10 of 10 at load 2 and not at all at load
// 12, because chromium's timer slips under load and some second fell to 9.
// Load deciding a verdict is what R9 rules out (r9/1), so the default moved off
// the poll interval: a 100ms poll is an honest page that must not fire at any
// load, and a loop at 60 or more still fires at 20. A binding may raise it.
//
// A render loop that fetches nothing makes no responses and
// is INVISIBLE here by design: the ruling forbids installing anything in the
// page under test, so that loop is a binding's flow to assert, not this arm's.
//
// "SUSTAINED" MEANS EVERY SECOND, NOT THE WINDOW'S TOTAL. A page that loads
// thirty assets in half a second puts thirty responses inside two seconds, and
// a total would fire on every heavy page load. Each whole second of the window
// must carry the rate on its own, which a burst cannot.
type Rate struct {
	PerSecond int
	Window    time.Duration
}

// DefaultRate is the threshold a watch starts with.
var DefaultRate = Rate{PerSecond: 20, Window: 2 * time.Second}

// rateArm watches one session's responses on the BROWSER's clock -- each BiDi
// event carries the time the browser stamped it -- so a slow box delays the
// events and does not change what they say (R9's waits are in the engine's
// time for the same reason). It spans step boundaries on purpose: a step
// usually ends when its page has loaded, well under two seconds, and a loop
// runs for as long as the actor stays on the page. What it cannot see is a
// loop on a page the actor leaves inside the window.
type rateArm struct {
	r      Rate
	resp   []int64 // ms, the responses inside the last window
	logs   []int64 // ms, log entries, counted beside and never summed in
	firing bool    // one finding per episode: re-armed only once a second falls short
}

// stamp is a BiDi event's own timestamp in milliseconds, 0 when it carries none.
func stamp(params json.RawMessage) int64 {
	var p struct {
		Timestamp float64 `json:"timestamp"`
	}
	_ = json.Unmarshal(params, &p)
	return int64(p.Timestamp)
}

func (a *rateArm) log(ts int64) {
	a.logs = trim(append(a.logs, ts), ts-a.r.Window.Milliseconds())
}

// response records one response and answers a Fault the first time the window
// ending at it carries the rate in every second.
func (a *rateArm) response(ts int64) (Fault, bool) {
	w := a.r.Window.Milliseconds()
	a.resp = trim(append(a.resp, ts), ts-w)
	a.logs = trim(a.logs, ts-w)
	sustained := true
	for i := int64(0); i*1000 < w; i++ {
		hi, lo := ts-i*1000, ts-(i+1)*1000
		n := 0
		for _, t := range a.resp {
			if t > lo && t <= hi {
				n++
			}
		}
		if n < a.r.PerSecond {
			sustained = false
			break
		}
	}
	if !sustained {
		a.firing = false
		return Fault{}, false
	}
	if a.firing {
		return Fault{}, false
	}
	a.firing = true
	return Fault{Kind: "rate", Text: fmt.Sprintf("%d responses in %s, at least %d in each second; %d log entries in the same window",
		len(a.resp), a.r.Window, a.r.PerSecond, len(a.logs))}, true
}

// trim drops what fell out of the window, keeping order.
func trim(ts []int64, after int64) []int64 {
	i := 0
	for i < len(ts) && ts[i] <= after {
		i++
	}
	return ts[i:]
}
