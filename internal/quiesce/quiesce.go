// Package quiesce asks a project's declared probes whether the system under
// test has gone quiet, and reports a probe that gave up as a FINDING rather
// than as a failure to run.
//
// IT FILLS A SLOT THAT HAS BEEN EMPTY SINCE R6.1. internal/schedule declared
// `Prober` with one method and nothing implemented it, so a project with a
// `quiesce` bound was REFUSED by cmd/rumor -- "no quiesce prober is wired" --
// and that refusal's own comment says it "removes itself" the moment one is
// wired, which is what this package is.
//
// NO WIRE CHANGE, which is fable's ruling at coop r6/1 and the reason the
// probes ride the protocol that exists: a probe is an `observe` with source
// `direct`, which docs/contract.md already calls "the engine's observation and
// never an actor's" and whose Query the decoder treats as OPAQUE. So
//
//	kind `direct`       asks the observation the project named in `query`
//	kind `queue_depth`  asks the query "queue_depth"
//	kind `worker_idle`  asks the query "worker_idle"
//
// and an adapter answers all three from the one message kind it already
// handles. The two reserved query names are the whole of the convention; they
// are spelled once, here, as the kinds themselves.
//
// THE CLOCK IS INJECTED, and that is not a convenience. The room measured that
// no actor on this fleet can have an isolated run -- three jails, four CPUs,
// one `kern.boottime` to the microsecond (ratchet and rook, coop r5/192-/195)
// -- and that a wall-clock budget below the work it measures is a flake whose
// cause nobody has found. An arm that drove a real 30s timeout would be timing
// the box. So `now` and `sleep` are fields, the arms drive a virtual clock, and
// the only thing a real run gets from this file is the real functions.
package quiesce

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

// DefaultEvery is the cadence for a probe that declares no `every`.
//
// THE DECLARED CADENCE WINS AND THIS IS THE FLOOR UNDER NOTHING: a probe's
// `every` is optional precisely so a project need not tune one, and
// docs/contract.md calls this "the engine's own cadence". It is deliberately
// well under the shortest timeout a project is likely to declare, so a default
// cadence cannot be the reason a probe never gets asked twice.
const DefaultEvery = time.Second

// Observer is the half of a wire session this package uses.
//
// AN INTERFACE AND NOT *wire.Session, so an arm can answer probes without
// spawning an adapter -- and so this package cannot reach for a session's other
// methods. It is the smallest surface that can ask the question.
type Observer interface {
	Observe(wire.ObserveRequest) (wire.ObserveResponse, error)
}

// Prober combines a project's declared probes into one answer.
type Prober struct {
	probes []bindings.Probe
	obs    Observer
	now    func() time.Time
	sleep  func(context.Context, time.Duration) error

	// timeout and every are the probes' durations, PARSED ONCE IN New and
	// positional with `probes`.
	//
	// THEY EXIST SO NO ERROR IS DROPPED WHERE IT CANNOT BE REPORTED. Quiesce's
	// round loop read `d, _ := time.ParseDuration(pr.Timeout)` per round, in a
	// function whose answer is a state and not an error -- so a value New had
	// already approved was re-derived in the one place that could not complain
	// about it, and a non-positive one silently produced a deadline in the past
	// or a poll interval of zero (ratchet, coop rumor/1356: `every = "0s"` ran
	// 3000 asks and was still going, against 6001 asks and 30s slept for the
	// declared control). Parsed here, the loop reads numbers that are known
	// positive and nothing is discarded.
	timeout []time.Duration
	every   []time.Duration

	// hang is the report for the probe that gave up, written when Quiesce
	// answers QuiesceTimedOut and empty otherwise.
	//
	// IT IS NOT RETURNED FROM Quiesce because the Prober interface is
	// internal/schedule's and returns a state, and widening a published
	// interface to carry one implementation's detail is the wrong direction.
	// The caller that produced the finding reads it here.
	hang         string
	hangMeasured string
}

// Option configures a Prober. The clock options exist for the arms.
type Option func(*Prober)

// WithClock replaces the real clock. The sleeper must honour the context.
func WithClock(now func() time.Time, sleep func(context.Context, time.Duration) error) Option {
	return func(p *Prober) { p.now, p.sleep = now, sleep }
}

// New builds a Prober over the probes a project declared.
//
// IT REFUSES A PROBE VALIDATE WOULD HAVE REFUSED, rather than trusting that
// validate ran. `rumor validate` owns arms 7c, 7d and 7e -- no timeout, an
// unknown kind, a `query` on a kind that does not read one -- and a run is not
// obliged to have been validated first. Refusing here is cheap and the
// alternative is a wait with no end, which is the exact thing R6 rules out.
func New(probes []bindings.Probe, obs Observer, opts ...Option) (*Prober, error) {
	if obs == nil {
		return nil, errors.New("quiesce: no observer, so nothing can be asked")
	}
	if len(probes) == 0 {
		return nil, errors.New("quiesce: no probes declared, so there is nothing to " +
			"ask and 'quiet' would be a claim about an empty set")
	}
	p := &Prober{probes: probes, obs: obs, now: time.Now, sleep: realSleep}
	for _, o := range opts {
		o(p)
	}
	for i, pr := range p.probes {
		if _, err := query(pr); err != nil {
			return nil, fmt.Errorf("quiesce: probe %d: %w", i+1, err)
		}
		if pr.Timeout == "" {
			return nil, fmt.Errorf("quiesce: probe %d (kind %q) declares no timeout, and a "+
				"probe without one is a wait with no end", i+1, pr.Kind)
		}
		to, err := time.ParseDuration(pr.Timeout)
		if err != nil {
			return nil, fmt.Errorf("quiesce: probe %d (kind %q) timeout %q is not a duration: %w",
				i+1, pr.Kind, pr.Timeout, err)
		}
		// AND NON-POSITIVE IS NOT REFUSED HERE, WHICH I TRIED AND AN ARM
		// STOPPED. "A timeout of 0 must still put the question" is this
		// package's own rule, armed by TestEveryProbeIsAskedAtLeastOnce with
		// its reason beside it -- checking a deadline before the round would
		// report a hang having asked nothing. fable put the non-positive
		// refusal at the PARSE (coop rumor/1359 item 2) and internal/bindings
		// now reads both durations through `decl.Duration`, so no PROJECT can
		// declare one; a hand-built Probe still can, and a zero `every` still
		// spins for one. That is said here rather than fixed by a refusal this
		// tier's own arm contradicts.
		ev := DefaultEvery
		if pr.Every != "" {
			ev, err = time.ParseDuration(pr.Every)
			if err != nil {
				return nil, fmt.Errorf("quiesce: probe %d (kind %q) every %q is not a duration: %w",
					i+1, pr.Kind, pr.Every, err)
			}
		}
		p.timeout = append(p.timeout, to)
		p.every = append(p.every, ev)
	}
	return p, nil
}

func realSleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// query is the observation name a probe asks for.
func query(pr bindings.Probe) (string, error) {
	switch pr.Kind {
	case "queue_depth", "worker_idle":
		if pr.Query != "" {
			return "", fmt.Errorf("kind %q declares `query` %q, and only the `direct` kind "+
				"reads an observation", pr.Kind, pr.Query)
		}
		return pr.Kind, nil
	case "direct":
		if pr.Query == "" {
			return "", errors.New("kind `direct` declares no `query`, so there is no " +
				"observation to ask for")
		}
		return pr.Query, nil
	default:
		return "", fmt.Errorf("kind %q is not one of direct, queue_depth, worker_idle", pr.Kind)
	}
}

// Hang is the report for the probe that gave up, or empty. IT IS CLOCK-FREE, so
// a caller may print it where the control reads: see the split at Quiesce.
func (p *Prober) Hang() string { return p.hang }

// HangMeasured is the clock-derived half of the same report -- how many times
// the probes were asked and over what span -- or empty. A CALLER MUST NOT PRINT
// IT WHERE THE CONTROL READS: it differs between two attempts of one seed,
// which is what D1 forbids.
func (p *Prober) HangMeasured() string { return p.hangMeasured }

// State is ONE round of the probes, with no waiting: quiet if every declared
// probe reads its quiet value right now, busy otherwise.
//
// WHY A SECOND METHOD AND NOT A FLAG ON Quiesce. The two answer different
// questions and only one of them may block. A RECONCILIATION asks "has it
// settled?" and is entitled to wait out the bound the project declared --
// that is Quiesce. A CHECK MID-WALK asks "is it settled YET?", and must not
// wait: fable's ruling at coop r6/73 is that such a check reads `pending`
// rather than `bug`, which is an answer about the moment and not a reason to
// stop the walk. A per-step check that waited would serialise the whole run
// behind the worker and turn every lag into a stall.
//
// IT NEVER ANSWERS QuiesceTimedOut, deliberately: nothing here gave up, because
// nothing here was waiting. A timeout is Quiesce's verdict alone.
func (p *Prober) State(ctx context.Context) (engine.QuiesceState, error) {
	if err := ctx.Err(); err != nil {
		return engine.QuiesceAbsent, fmt.Errorf("quiesce: reading the probes: %w", err)
	}
	for i, pr := range p.probes {
		q, err := query(pr)
		if err != nil {
			return engine.QuiesceAbsent, fmt.Errorf("quiesce: probe %d: %w", i+1, err)
		}
		resp, oerr := p.obs.Observe(wire.ObserveRequest{
			Envelope: wire.Envelope{Kind: wire.KindObserve},
			Query:    q,
			Source:   wire.SourceDirect,
		})
		if oerr != nil {
			return engine.QuiesceAbsent, fmt.Errorf(
				"quiesce: probe %d (kind %q, query %q): %w", i+1, pr.Kind, q, oerr)
		}
		if !quiet(resp.Value, pr.Quiet) {
			return engine.QuiesceBusy, nil
		}
	}
	return engine.QuiesceQuiet, nil
}

// Quiesce asks every probe until all read quiet or one passes its timeout.
//
// QUIET IS THE CONJUNCTION AND BUSY IS NOT AN ANSWER THIS RETURNS. A queue can
// be empty while a worker is mid-job, so "quiet" is every declared probe
// reading its own quiet value (fable, coop breakout-r6/0) -- and this method
// WAITS rather than reporting busy, because its caller asked whether the system
// has settled and "not yet" is something to wait out until the bound the
// project declared says to stop. QuiesceBusy is what a CHECK carries when the
// answer is handed in; it is not a verdict from the thing doing the waiting.
//
// THE TIMEOUT IS PER PROBE AND THE FIRST TO PASS ITS OWN ENDS IT, which is what
// "each with its own timeout" means: a 5s queue probe beside a 60s worker probe
// gives up at 5s, and the report names that probe rather than the slowest one.
func (p *Prober) Quiesce(ctx context.Context) (engine.QuiesceState, error) {
	p.hang = ""
	p.hangMeasured = ""
	start := p.now()
	// deadline per probe, computed once from one `start` so a slow round cannot
	// extend a budget.
	deadline := make([]time.Time, len(p.probes))
	every := make([]time.Duration, len(p.probes))
	for i := range p.probes {
		// FROM WHAT New PARSED. Nothing is re-derived and no error is dropped
		// here: see the fields' own note.
		deadline[i] = start.Add(p.timeout[i])
		every[i] = p.every[i]
	}

	rounds := 0
	for {
		rounds++
		var busy []string
		notQuiet := make([]bool, len(p.probes))
		for i, pr := range p.probes {
			q, err := query(pr)
			if err != nil {
				return engine.QuiesceAbsent, fmt.Errorf("quiesce: probe %d: %w", i+1, err)
			}
			resp, oerr := p.obs.Observe(wire.ObserveRequest{
				Envelope: wire.Envelope{Kind: wire.KindObserve},
				Query:    q,
				Source:   wire.SourceDirect,
			})
			if oerr != nil {
				// A PROBE THAT COULD NOT BE ASKED IS NOT A PROBE THAT GAVE UP.
				// The first is could-not-run -- the adapter did not answer the
				// question -- and the second is a finding about the system. An
				// error here would read as "the system will not settle" if it
				// were folded into QuiesceTimedOut, which is the two-states-
				// one-verdict shape the room spent a night correcting.
				return engine.QuiesceAbsent, fmt.Errorf(
					"quiesce: probe %d (kind %q, query %q): %w", i+1, pr.Kind, q, oerr)
			}
			if !quiet(resp.Value, pr.Quiet) {
				notQuiet[i] = true
				busy = append(busy, fmt.Sprintf("probe %d (kind %q) reads %v, wants %v",
					i+1, pr.Kind, resp.Value, pr.Quiet))
			}
		}
		if len(busy) == 0 {
			return engine.QuiesceQuiet, nil
		}

		// WHICH PROBE GAVE UP: ITS OWN DEADLINE *AND* ITS OWN ANSWER. Checked
		// AFTER a round so a probe is always asked at least once: a timeout of
		// 0 would otherwise report a hang without ever having put the question.
		//
		// THE INTERSECTION IS THE WHOLE OF rook's coop rumor/1205. A deadline
		// is a fact about TIME and quietness is a fact about the SYSTEM, and
		// this loop read only the first: every probe past its deadline was
		// named as one that "did not read quiet", including probes that read
		// quiet every round. examples/toy declares both probes at 2s, so one
		// never-quiet probe named BOTH; and with the never-quiet probe's own
		// timeout raised to 30s the report named ONLY the probe that had gone
		// quiet -- the exactly-wrong answer, which rook measured across three
		// invocations where nothing but the timeouts moved.
		//
		// `busy` already holds the not-quiet of THIS round, so the report's two
		// halves now come from one reading of the system rather than two
		// different questions that happened to be printed in one sentence.
		now := p.now()
		var expired []string
		for i := range p.probes {
			if notQuiet[i] && !now.Before(deadline[i]) {
				expired = append(expired, fmt.Sprintf("probe %d (kind %q) did not read quiet "+
					"within its declared timeout of %s", i+1, p.probes[i].Kind, p.probes[i].Timeout))
			}
		}
		if len(expired) > 0 {
			sort.Strings(expired)
			sort.Strings(busy)
			// TWO REPORTS, AND THE SPLIT IS D1. The ask count and the elapsed
			// span are CLOCK-DERIVED: "Asked 343 time(s) over 2.002s" and
			// "Asked 340 time(s) over 2.005s" are the same run twice, and
			// fable's ruling 2 (coop r6/78) requires two attempts of a seed to
			// be byte-identical. A hang report on stdout carrying either number
			// makes D1 UNREACHABLE for every run that ends on a timeout, however
			// deterministic the system under test is -- the control cannot
			// compare what the clock wrote.
			//
			// SO THE VERDICT IS CLOCK-FREE AND THE MEASUREMENT KEEPS ITS PLACE.
			// Hang() names the probes, their kinds, their DECLARED timeouts and
			// what each still reads -- every byte of it a declaration or a
			// reading -- and HangMeasured() carries the two numbers for the
			// caller that prints to stderr, where the adapter's own timings
			// already go. Dropping them was the other option and it is worse:
			// the ask count against the cadence is what says a probe was
			// actually being asked, which is how R6.2 measured 108ms against a
			// declared 100ms.
			p.hang = fmt.Sprintf("the system did not go quiet: %s. Still busy at the end: %s",
				strings.Join(expired, "; "), strings.Join(busy, "; "))
			p.hangMeasured = fmt.Sprintf("asked %d time(s) over %s",
				rounds, now.Sub(start).Round(time.Millisecond))
			return engine.QuiesceTimedOut, nil
		}

		// SLEEP THE SHORTEST CADENCE AMONG THE PROBES STILL BEING ASKED, so a
		// probe declaring `every = "100ms"` is honoured even beside one that
		// declares a minute. Asking them all on the fast cadence is the
		// conservative direction: the alternative is per-probe timers, which
		// buys nothing for a conjunction that has to be read whole anyway.
		wait := every[0]
		for _, e := range every[1:] {
			if e < wait {
				wait = e
			}
		}
		if err := p.sleep(ctx, wait); err != nil {
			return engine.QuiesceAbsent, fmt.Errorf("quiesce: waiting between rounds: %w", err)
		}
	}
}

// quiet compares an adapter's answer with the project's declared quiet value.
//
// IT NORMALISES NUMBERS BECAUSE THE TWO SIDES CANNOT AGREE ON A TYPE. A TOML
// `quiet = 0` parses to int64 and the same 0 arrives from JSON as float64, so a
// == over `any` is FALSE for two values that are the same number -- and the
// failure is silent and total: every probe reads busy, every run times out, and
// the report says the system never settled when it was quiet the whole time.
// Driven in this package's arms for all three kinds.
func quiet(got, want any) bool {
	if g, ok := asFloat(got); ok {
		if w, ok := asFloat(want); ok {
			return g == w
		}
		return false
	}
	return got == want
}

func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint64:
		return float64(n), true
	default:
		return 0, false
	}
}
