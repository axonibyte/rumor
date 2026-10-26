package quiesce

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

// stub answers probes from a table, and records what was asked.
//
// IT ANSWERS BY QUERY because that is what the adapter sees: the prober's whole
// convention is which query name each kind asks for, so a stub keyed on the
// query is the one that can catch a wrong name.
type stub struct {
	answers map[string]any
	asked   []string
	err     error
	// after is an answer that replaces `answers` once `flip` rounds have been
	// asked, so a system can be driven from busy to quiet.
	after map[string]any
	flip  int
	calls int
}

func (s *stub) Observe(req wire.ObserveRequest) (wire.ObserveResponse, error) {
	s.asked = append(s.asked, req.Query)
	s.calls++
	if req.Source != wire.SourceDirect {
		return wire.ObserveResponse{}, errors.New("a probe must ask with source direct")
	}
	if req.Token != "" {
		return wire.ObserveResponse{}, errors.New("a direct observation carries no token")
	}
	if s.err != nil {
		return wire.ObserveResponse{}, s.err
	}
	table := s.answers
	if s.after != nil && s.calls >= s.flip {
		table = s.after
	}
	v, ok := table[req.Query]
	if !ok {
		return wire.ObserveResponse{}, errors.New("not recorded: " + req.Query)
	}
	return wire.ObserveResponse{Value: v}, nil
}

// virtual is a clock the arms drive, so no arm waits on the wall.
type virtual struct {
	at    time.Time
	slept time.Duration
}

func (v *virtual) now() time.Time { return v.at }
func (v *virtual) sleep(_ context.Context, d time.Duration) error {
	v.at = v.at.Add(d)
	v.slept += d
	return nil
}

func newVirtual() *virtual {
	return &virtual{at: time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)}
}

// TestEachKindDrivenQuietAndNeverQuiet is fable's D5, which asks for exactly
// this: "each kind (queue depth, worker idle, direct) driven quiet and driven
// never-quiet; the never-quiet one ends in the timeout finding inside its
// declared timeout plus grace, measured, hang report written".
//
// BOTH DIRECTIONS FOR EACH KIND, which is the half that makes either reading
// worth anything: a prober that always answered quiet would pass the first
// three cells and a prober that always timed out would pass the other three.
func TestEachKindDrivenQuietAndNeverQuiet(t *testing.T) {
	for _, c := range []struct {
		kind string
		// query is what the ADAPTER is asked. For the two reported kinds that is
		// the kind's own name and the probe DECLARES no query -- arm 7e refuses
		// one -- so `declares` is empty for them and the convention under test
		// is that the prober asks for the kind by name anyway.
		query    string
		declares string
		// quiet is what the project declares, busy what a working system reads
		// while it is still going.
		quiet any
		busy  any
	}{
		{"queue_depth", "queue_depth", "", int64(0), float64(3)},
		{"worker_idle", "worker_idle", "", true, false},
		{"direct", "store_event_count", "store_event_count", int64(0), float64(7)},
	} {
		t.Run(c.kind+" reads quiet", func(t *testing.T) {
			v := newVirtual()
			s := &stub{answers: map[string]any{c.query: c.quiet}}
			p := build(t, s, v, probe(c.kind, c.declares, "30s", "", c.quiet))
			got, err := p.Quiesce(context.Background())
			if err != nil {
				t.Fatalf("refused: %v", err)
			}
			if got != engine.QuiesceQuiet {
				t.Errorf("state is %s, want quiet", got)
			}
			if p.Hang() != "" {
				t.Errorf("a quiet answer wrote a hang report: %s", p.Hang())
			}
			if len(s.asked) != 1 || s.asked[0] != c.query {
				t.Errorf("asked %v, want exactly [%s]", s.asked, c.query)
			}
			if v.slept != 0 {
				t.Errorf("slept %s before answering quiet: a system already quiet is not waited on", v.slept)
			}
		})

		t.Run(c.kind+" never reads quiet", func(t *testing.T) {
			v := newVirtual()
			s := &stub{answers: map[string]any{c.query: c.busy}}
			p := build(t, s, v, probe(c.kind, c.declares, "30s", "", c.quiet))
			got, err := p.Quiesce(context.Background())
			if err != nil {
				t.Fatalf("a probe that gave up must be a FINDING and not an error: %v", err)
			}
			if got != engine.QuiesceTimedOut {
				t.Fatalf("state is %s, want timed out", got)
			}
			// THE REPORT IS WRITTEN AND IT NAMES THE PROBE AND THE TIMEOUT,
			// which is what fable amended the definition to at coop r6/42: the
			// finding is the timeout, and it names the probe and the timeout it
			// passed.
			for _, want := range []string{"did not read quiet", c.kind, "30s", "probe 1"} {
				if !strings.Contains(p.Hang(), want) {
					t.Errorf("the hang report does not say %q: %s", want, p.Hang())
				}
			}
			// INSIDE THE DECLARED TIMEOUT PLUS GRACE, MEASURED on the virtual
			// clock: it gives up at the deadline and not one cadence past it.
			if v.slept < 30*time.Second || v.slept > 31*time.Second {
				t.Errorf("gave up after %s, want the declared 30s plus at most one cadence", v.slept)
			}
		})
	}
}

// TestABusySystemThatSettlesIsQuiet is the control for the six cells above: the
// prober WAITS rather than reporting the first busy reading, so a system that
// settles on the third round is quiet and not timed out.
func TestABusySystemThatSettlesIsQuiet(t *testing.T) {
	v := newVirtual()
	s := &stub{
		answers: map[string]any{"queue_depth": float64(4)},
		after:   map[string]any{"queue_depth": float64(0)},
		flip:    3,
	}
	p := build(t, s, v, probe("queue_depth", "", "30s", "", int64(0)))
	got, err := p.Quiesce(context.Background())
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	if got != engine.QuiesceQuiet {
		t.Fatalf("state is %s, want quiet: a system that settles is not a hang", got)
	}
	if p.Hang() != "" {
		t.Errorf("a settling system wrote a hang report: %s", p.Hang())
	}
	// THREE, because the stub flips to quiet ON its third answer: busy, busy,
	// quiet. What the arm is about is that the count is greater than ONE -- a
	// prober that reported the first busy reading would have asked once.
	if len(s.asked) != 3 {
		t.Errorf("asked %d time(s), want 3: busy, busy, then quiet", len(s.asked))
	}
	if v.slept == 0 {
		t.Error("settled without ever waiting, so it did not re-ask between rounds")
	}
}

// TestTheFirstProbeToPassItsOwnTimeoutEndsIt pins "each with its own timeout".
//
// WHY IT MATTERS: a 5s queue probe beside a 60s worker probe must give up at
// 5s, and the report must name the FIVE-second one. A prober taking the longest
// deadline would wait a minute and then blame whichever probe it noticed first.
func TestTheFirstProbeToPassItsOwnTimeoutEndsIt(t *testing.T) {
	v := newVirtual()
	s := &stub{answers: map[string]any{"queue_depth": float64(9), "worker_idle": false}}
	p := build(t, s, v,
		probe("queue_depth", "", "5s", "1s", int64(0)),
		probe("worker_idle", "", "60s", "1s", true))
	got, err := p.Quiesce(context.Background())
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	if got != engine.QuiesceTimedOut {
		t.Fatalf("state is %s, want timed out", got)
	}
	if !strings.Contains(p.Hang(), "5s") {
		t.Errorf("the report does not name the 5s timeout: %s", p.Hang())
	}
	if strings.Contains(p.Hang(), "within its declared timeout of 60s") {
		t.Errorf("the report blames the 60s probe, which had not passed its own: %s", p.Hang())
	}
	if v.slept > 6*time.Second {
		t.Errorf("waited %s: the shortest timeout ends it, not the longest", v.slept)
	}
}

// TestQuietIsTheConjunction: one probe quiet and one busy is not quiet.
func TestQuietIsTheConjunction(t *testing.T) {
	v := newVirtual()
	s := &stub{answers: map[string]any{"queue_depth": float64(0), "worker_idle": false}}
	p := build(t, s, v,
		probe("queue_depth", "", "10s", "1s", int64(0)),
		probe("worker_idle", "", "10s", "1s", true))
	got, _ := p.Quiesce(context.Background())
	if got == engine.QuiesceQuiet {
		t.Fatal("an empty queue with a busy worker read as quiet: quiet is the conjunction")
	}
	if !strings.Contains(p.Hang(), "worker_idle") {
		t.Errorf("the report does not name the busy probe: %s", p.Hang())
	}
	if strings.Contains(p.Hang(), `probe 1 (kind "queue_depth") reads`) {
		t.Errorf("the report lists the QUIET probe as still busy: %s", p.Hang())
	}
}

// TestAnIntegerQuietMatchesAJSONNumber is the type arm, and it is the one
// defect in this package that would have been silent and total.
//
// A TOML `quiet = 0` IS int64 AND THE SAME 0 ARRIVES FROM JSON AS float64, so a
// bare == over `any` is false for two values that are the same number. Every
// probe would read busy, every run would time out, and the hang report would
// say the system never settled when it was quiet from the first round.
func TestAnIntegerQuietMatchesAJSONNumber(t *testing.T) {
	for _, c := range []struct {
		name string
		got  any
		want any
		same bool
	}{
		{"json float against a toml int", float64(0), int64(0), true},
		{"json float against a toml float", float64(2), float64(2), true},
		{"int against int", 3, int64(3), true},
		{"different numbers", float64(1), int64(0), false},
		{"bool against bool", true, true, true},
		{"bool against its opposite", false, true, false},
		{"string against string", "idle", "idle", true},
		{"a number against a bool is not a match", float64(0), false, false},
	} {
		if got := quiet(c.got, c.want); got != c.same {
			t.Errorf("%s: quiet(%#v, %#v) = %v, want %v", c.name, c.got, c.want, got, c.same)
		}
	}
}

// TestAnAdapterThatCannotAnswerIsNotAHang separates could-not-run from the
// finding. Folding them would report "the system will not settle" when the
// truth is that nobody asked it -- the two-states-one-verdict shape.
func TestAnAdapterThatCannotAnswerIsNotAHang(t *testing.T) {
	v := newVirtual()
	s := &stub{answers: map[string]any{"queue_depth": float64(0)}, err: errors.New("pipe closed")}
	p := build(t, s, v, probe("queue_depth", "", "30s", "", int64(0)))
	got, err := p.Quiesce(context.Background())
	if err == nil {
		t.Fatal("an adapter that could not answer was reported as a state")
	}
	if got == engine.QuiesceTimedOut {
		t.Error("an adapter error read as a probe that gave up")
	}
	if got != engine.QuiesceAbsent {
		t.Errorf("state is %s, want absent: no prober answered", got)
	}
	if p.Hang() != "" {
		t.Errorf("an adapter error wrote a hang report: %s", p.Hang())
	}
}

// TestEveryProbeIsAskedAtLeastOnce: a timeout of 0 must still put the question.
// Checking the deadline before the round would report a hang having asked
// nothing, which is a verdict about a system nobody spoke to.
func TestEveryProbeIsAskedAtLeastOnce(t *testing.T) {
	v := newVirtual()
	s := &stub{answers: map[string]any{"queue_depth": float64(0)}}
	p := build(t, s, v, probe("queue_depth", "", "0s", "", int64(0)))
	got, err := p.Quiesce(context.Background())
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	if len(s.asked) == 0 {
		t.Fatal("a timeout of 0 asked nothing and still answered")
	}
	if got != engine.QuiesceQuiet {
		t.Errorf("state is %s: the system WAS quiet on the first ask", got)
	}
}

// TestNewRefusesWhatValidateWouldHave, because a run is not obliged to have
// been validated first and the alternative is a wait with no end.
func TestNewRefusesWhatValidateWouldHave(t *testing.T) {
	v := newVirtual()
	s := &stub{answers: map[string]any{}}
	for _, c := range []struct {
		what string
		p    []bindings.Probe
		says string
	}{
		{"no probes at all", nil, "nothing to ask"},
		{"a probe with no timeout",
			[]bindings.Probe{probe("queue_depth", "", "", "", int64(0))}, "no timeout"},
		{"a timeout that is not a duration",
			[]bindings.Probe{probe("queue_depth", "", "soon", "", int64(0))}, "not a duration"},
		{"an every that is not a duration",
			[]bindings.Probe{probe("queue_depth", "", "30s", "often", int64(0))}, "not a duration"},
		{"an unknown kind",
			[]bindings.Probe{probe("stares_at_it", "", "30s", "", int64(0))}, "not one of"},
		{"query on a kind that does not read one",
			[]bindings.Probe{probe("queue_depth", "item.count", "30s", "", int64(0))}, "only the `direct` kind"},
		{"direct with no query",
			[]bindings.Probe{probe("direct", "", "30s", "", int64(0))}, "no `query`"},
	} {
		t.Run(c.what, func(t *testing.T) {
			_, err := New(c.p, s, WithClock(v.now, v.sleep))
			if err == nil {
				t.Fatalf("%s was accepted", c.what)
			}
			if !strings.Contains(err.Error(), c.says) {
				t.Errorf("refused, but not by the clause for %s: %v", c.what, err)
			}
		})
	}
	if _, err := New([]bindings.Probe{probe("queue_depth", "", "30s", "1s", int64(0))}, nil); err == nil {
		t.Error("a nil observer was accepted, so nothing could be asked")
	}
	// THE CONTROL: a well-formed probe is ACCEPTED, so the refusals above are
	// about their own clauses and not about New refusing everything.
	if _, err := New([]bindings.Probe{probe("direct", "store_event_count", "30s", "1s", int64(0))},
		s, WithClock(v.now, v.sleep)); err != nil {
		t.Errorf("a well-formed direct probe was refused: %v", err)
	}
}

// TestADeclaredCadenceIsHonoured: `every` decides the asking rate, and the
// shortest among the probes wins so a fast probe is not slowed by a slow one.
func TestADeclaredCadenceIsHonoured(t *testing.T) {
	v := newVirtual()
	s := &stub{answers: map[string]any{"queue_depth": float64(1), "worker_idle": false}}
	p := build(t, s, v,
		probe("queue_depth", "", "10s", "500ms", int64(0)),
		probe("worker_idle", "", "10s", "5s", true))
	if _, err := p.Quiesce(context.Background()); err != nil {
		t.Fatalf("refused: %v", err)
	}
	// 10s at 500ms is about twenty rounds, two probes each: a 5s cadence would
	// give four. The assertion is a floor rather than an equality because the
	// round count depends on where the deadline check falls.
	if len(s.asked) < 30 {
		t.Errorf("asked %d time(s) over 10s: the 500ms cadence was not honoured", len(s.asked))
	}
}

// --- helpers ---------------------------------------------------------------

func probe(kind, query, timeout, every string, quiet any) bindings.Probe {
	return bindings.Probe{Kind: kind, Query: query, Timeout: timeout, Every: every, Quiet: quiet}
}

func build(t *testing.T, s *stub, v *virtual, probes ...bindings.Probe) *Prober {
	t.Helper()
	p, err := New(probes, s, WithClock(v.now, v.sleep))
	if err != nil {
		t.Fatalf("New refused a well-formed prober: %v", err)
	}
	return p
}
