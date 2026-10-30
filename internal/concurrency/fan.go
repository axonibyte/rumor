// Package concurrency puts two or more requests in flight against ONE system at
// once, which is the only way a check-then-act defect can be reached.
//
// WHY N PROCESSES AND NOT N GOROUTINES ON ONE SESSION. The wire protocol says
// "where N sessions are wanted, N processes" rather than N outstanding requests
// on one, and wire.Session says so of itself: "A Session IS NOT SAFE FOR
// CONCURRENT USE, deliberately ... because the engine owns the timing"
// (internal/wire/session.go:61). So a fan-out is a set of sessions, each driven
// by one goroutine, and this package never touches a session from two.
//
// AND THE N PROCESSES MUST SHARE ONE WORLD, which is the half that is easy to
// get wrong and silent when wrong. cmd/toy-adapter has two modes: it takes an
// address from TOY_ADDR, and "When TOY_ADDR is absent the adapter starts a toy
// of its own" (cmd/toy-adapter/main.go:9). Two adapters spawned without it
// would each start a server, so a fan-out would contend on NOTHING -- two
// requests against two worlds, both succeeding, and a capacity race reported as
// absent. The caller owns that: it starts one system, and passes its address
// through the named environment `[adapter].env` declares. The arm for this
// package proves the sessions reached one world before it proves anything about
// timing, because overlap against two worlds is not a fan-out.
package concurrency

import (
	"fmt"
	"sync"
	"time"

	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

// Attempt is one request of a fan-out: which session carried it, the span it
// was in flight, and what came back.
//
// THE SPAN IS RECORDED BECAUSE "CONCURRENT" IS A CLAIM THAT CAN BE FALSE. A
// fan-out whose requests happened to serialize -- a slow spawn, a busy box, a
// barrier that released late -- would exercise the sequential path and report
// the race as absent, which is indistinguishable from a system that has none.
// The spans are what let the caller refuse that run instead of believing it.
type Attempt struct {
	Session  int
	Started  time.Time
	Finished time.Time
	Response wire.ExecuteResponse
	// Fault is a transport failure: the adapter could not carry the request.
	// It is NOT the system refusing, which rides in Response.
	Fault string
}

// InFlight is how long this attempt was outstanding.
func (a Attempt) InFlight() time.Duration { return a.Finished.Sub(a.Started) }

// Fan releases one request per session at the same moment and records each
// attempt's span.
//
// ONE SESSION PER REQUEST, positionally, and a mismatch is refused rather than
// truncated: a caller that passed three requests and two sessions would
// otherwise silently fan out two, and the missing third would look like a
// system that answered fewer times than it was asked.
//
// A FAN-OUT OF ONE IS NOT A FAN-OUT and is refused by name. It would exercise
// the sequential path and report every concurrency clause as satisfied.
func Fan(sessions []*wire.Session, reqs []wire.ExecuteRequest) ([]Attempt, error) {
	if len(sessions) != len(reqs) {
		return nil, fmt.Errorf("%d session(s) for %d request(s): a fan-out carries one request "+
			"per session, and a mismatch would send fewer requests than the caller asked for",
			len(sessions), len(reqs))
	}
	if len(sessions) < 2 {
		return nil, fmt.Errorf("a fan-out of %d is not a fan-out: one request in flight exercises "+
			"the sequential path, and a check-then-act defect cannot be reached by it",
			len(sessions))
	}

	out := make([]Attempt, len(sessions))
	// THE BARRIER IS A CLOSED CHANNEL AND NOT A SLEEP. Every goroutine is
	// spawned, reaches the barrier and blocks; closing it wakes them all from
	// one event, so the release is not staggered by however long the spawns
	// took. A sleep would make the stagger a function of the box's load.
	release := make(chan struct{})
	var wg sync.WaitGroup
	for i := range sessions {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-release
			started := time.Now()
			resp, err := sessions[i].Execute(reqs[i])
			finished := time.Now()
			a := Attempt{Session: i, Started: started, Finished: finished, Response: resp}
			if err != nil {
				a.Fault = err.Error()
			}
			out[i] = a
		}(i)
	}
	close(release)
	wg.Wait()
	return out, nil
}

// Overlapped is whether any two attempts were in flight at the same moment, and
// the longest such overlap.
//
// THIS IS THE RUN's OWN PROOF THAT IT WAS CONCURRENT, and a concurrency stage
// that reports "no race found" without it is making a claim it did not test:
// two requests that serialized would answer exactly as a correct system does.
// The caller treats a non-overlapping fan-out as a COULD-NOT-RUN, not a pass.
func Overlapped(as []Attempt) (bool, time.Duration) {
	var best time.Duration
	found := false
	for i := range as {
		for j := i + 1; j < len(as); j++ {
			a, b := as[i], as[j]
			if a.Fault != "" || b.Fault != "" {
				continue
			}
			// The overlap of two spans is the later start to the earlier end.
			start, end := a.Started, a.Finished
			if b.Started.After(start) {
				start = b.Started
			}
			if b.Finished.Before(end) {
				end = b.Finished
			}
			if d := end.Sub(start); d > 0 {
				found = true
				if d > best {
					best = d
				}
			}
		}
	}
	return found, best
}
