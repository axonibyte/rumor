package engine

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"forge.axonibyte.com/axonibyte/rumor/internal/ledger"
)

// PERIODIC FULL RECONCILIATION (R6), the third of issue 44's bullets.
//
// WHAT "ALL HONEST ACTORS" IS READ AS, and it is a reading rather than a
// restatement. v1 tags CLAIMS with `planted`, not actors, and says outright
// that "there is no trust model". An actor can hold one honest claim and one
// planted one -- planted-lie's `with-planted` is exactly that, and it is the
// example the done-when names for this test. Excluding any actor who holds a
// planted claim would therefore observe nobody in that fixture and the control
// could never fire. So a pass observes EVERY actor named in the sealed prefix,
// honesty stays a property of the claims a statute discounts, and PlantedBy
// reports which actors are implicated so a caller can say so. Asked on
// thread:breakout-r2; if fable wants it narrower, Actors is the one function
// that changes.

// Pass is one query put to every actor in a reconciliation.
type Pass struct {
	Query  string
	Of     string
	Source ledger.Source
	Values map[string]any    // actor -> the value that actor observed
	Roles  map[string]string // actor -> role, where the project has roles

	// Quiesce is the probes' answer for this pass, carried through to every
	// Check it judges.
	//
	// WITHOUT IT A QUIESCE-BOUNDED PROJECT COULD NOT RECONCILE AT ALL. The
	// Check this pass builds took the zero value, which 1(b) made
	// QuiesceAbsent, and QuiesceAbsent refuses -- so a full reconciliation of
	// any project declaring a quiesce bound returned "no quiesce answer was
	// supplied (absent)" and never reached a verdict. Measured at v0.1.33
	// before this field existed: one busy pending, one covering pass, and the
	// reconciliation errored rather than re-checking it.
	//
	// IT IS STILL AN INPUT AND NEVER A MEASUREMENT HERE, the same split as
	// Check.Quiesce: R6.2's probes decide it and hand it in, and this package
	// judges with it. Absent remains the zero value and remains a refusal,
	// because a caller that wired no prober must not have silence read as
	// "not quiet".
	Quiesce QuiesceState
}

// Report is what one reconciliation found.
type Report struct {
	Actors    []string            // every actor observed, sorted
	Queries   []string            // every query put, in the order given
	Findings  map[string]Finding  // "actor/query" -> finding
	ReChecked []string            // "actor/query" pairs that re-checked a pending
	StillOpen map[string][]string // pendings no pass covered, actor -> queries
	Planted   []string            // actors holding a planted claim in the prefix
}

// Key is how a report names one actor's verdict on one query.
func Key(actor, query string) string { return actor + "/" + query }

// Actors names every actor in the sealed prefix, sorted: claimants, observers,
// tellers and the actors told. An actor that makes no claim is still an actor --
// charlie and dana make none in the examples and are the observers in most of
// them -- so deriving this from claims alone would leave the observer out of its
// own reconciliation.
//
// A CLOCK FACT'S ACTOR IS NOT ONE OF THEM, and finding that out is why this
// function does not simply walk every fact's Actor field. docs/vocabulary.md's
// Fact kinds table gives a `clock` fact's actor as "the run": the run is what
// controls time, not a participant whose beliefs are judged. A first version
// walked every fact and duly reconciled against an actor called "run", which
// knows no claims, disagreed with every observation, and left a pending open
// for itself at every pass.
func Actors(l *ledger.Ledger) []string {
	seen := map[string]bool{}
	for _, f := range l.SealedPrefix() {
		if f.Kind == ledger.KindClock {
			continue
		}
		if f.Actor != "" {
			seen[f.Actor] = true
		}
		if f.Kind == ledger.KindTold && f.To != "" {
			seen[f.To] = true
		}
	}
	out := make([]string, 0, len(seen))
	for a := range seen {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

// PlantedBy names the actors holding a planted claim in the sealed prefix.
// Reported, never used to skip an actor: see the reading above.
func PlantedBy(l *ledger.Ledger) []string {
	seen := map[string]bool{}
	for _, f := range l.SealedPrefix() {
		if f.Kind == ledger.KindClaim && f.Planted && f.Actor != "" {
			seen[f.Actor] = true
		}
	}
	out := make([]string, 0, len(seen))
	for a := range seen {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

// Reconcile puts every pass to every actor and returns one finding per pair.
//
// IT REFUSES AN INCOMPLETE PASS rather than reporting over whoever happened to
// be covered. Three refusals, each because the silent version is worse:
//
//   - no actors: a reconciliation over an empty ledger would report zero
//     disagreements, which reads identically to agreement everywhere.
//   - an actor with no observed value: skipping it would make a pass that
//     observed one of five actors indistinguishable from a full one.
//   - a value for an actor the prefix does not name: the pass and the ledger
//     disagree about who exists, and guessing which is right is not the
//     engine's call.
//
// AND IT REFUSES WHEN A PENDING SURVIVES IT. v1: a `pending` is re-checked "at
// every full reconciliation". A pass set that leaves one open was not full, so
// the report names it and Reconcile returns an error -- the caller asked for a
// full reconciliation and did not get one.
func Reconcile(ctx context.Context, statute string, rules []string, l *ledger.Ledger,
	passes []Pass, b Bindings, reg *Register) (Report, error) {
	rep := Report{Findings: map[string]Finding{}, StillOpen: map[string][]string{}}
	if l == nil {
		return rep, fmt.Errorf("engine: no ledger to reconcile")
	}
	if len(passes) == 0 {
		return rep, fmt.Errorf("engine: a reconciliation with no pass puts no question to anyone")
	}
	rep.Actors = Actors(l)
	rep.Planted = PlantedBy(l)
	if len(rep.Actors) == 0 {
		return rep, fmt.Errorf("engine: the sealed prefix names no actor, so a reconciliation over it " +
			"would report zero disagreements for the same reason it would report zero of anything")
	}

	for _, p := range passes {
		if p.Query == "" {
			return rep, fmt.Errorf("engine: a pass with no query asks nothing")
		}
		rep.Queries = append(rep.Queries, p.Query)
		known := map[string]bool{}
		for _, a := range rep.Actors {
			known[a] = true
		}
		var strangers []string
		for a := range p.Values {
			if !known[a] {
				strangers = append(strangers, a)
			}
		}
		if len(strangers) > 0 {
			sort.Strings(strangers)
			return rep, fmt.Errorf("engine: pass %q carries a value for %s, whom the sealed prefix does "+
				"not name: the pass and the ledger disagree about who exists",
				p.Query, strings.Join(strangers, ", "))
		}
		for _, a := range rep.Actors {
			v, ok := p.Values[a]
			if !ok {
				return rep, fmt.Errorf("engine: pass %q has no observed value for %s: a full "+
					"reconciliation observes every actor, and skipping one would make this "+
					"indistinguishable from a full pass", p.Query, a)
			}
			f, err := Judge(ctx, statute, rules, l, Check{
				Actor: a, Query: p.Query, Of: p.Of, Source: p.Source, Value: v, Role: p.Roles[a],
				Quiesce: p.Quiesce,
			}, b, reg)
			if err != nil {
				return rep, fmt.Errorf("engine: reconciling %s on %q: %w", a, p.Query, err)
			}
			k := Key(a, p.Query)
			rep.Findings[k] = f
			if f.ReChecked != nil {
				rep.ReChecked = append(rep.ReChecked, k)
			}
		}
	}

	// NOTHING MAY BE LEFT IN THE REGISTER, and that is the invariant rather
	// than "every query was re-asked".
	//
	// THE DENOMINATOR WAS THE DEFECT (ratchet, coop r6/11). This block used to
	// report only pendings whose query was ABSENT from the pass set, which
	// answers "was it re-asked" as a proxy for "was it resolved". The two
	// agree for every bound that expires on its own -- a duration pending
	// re-checked after its bound either clears or becomes a finding -- so the
	// proxy survived until a bound that does not expire on its own arrived.
	// A pending that WAS re-asked and is still open afterwards fell through
	// the `if !asked` and the function returned nil with a live disagreement
	// in the register: an open pending, reported as clean.
	//
	// SO BOTH CAUSES ARE REPORTED AND THE MESSAGE NAMES WHICH, because they
	// ask different things of a reader: an unasked pending means the caller's
	// pass set was not full, and a surviving one means the system did not
	// settle and the run has a finding to carry.
	if reg != nil && reg.Len() > 0 {
		asked := map[string]bool{}
		for _, q := range rep.Queries {
			asked[q] = true
		}
		var unasked, survived []string
		for actor, queries := range reg.Open() {
			for _, q := range queries {
				rep.StillOpen[actor] = append(rep.StillOpen[actor], q)
				if asked[q] {
					survived = append(survived, Key(actor, q))
				} else {
					unasked = append(unasked, Key(actor, q))
				}
			}
		}
		for a := range rep.StillOpen {
			sort.Strings(rep.StillOpen[a])
		}
		sort.Strings(unasked)
		sort.Strings(survived)
		if len(unasked) > 0 {
			return rep, fmt.Errorf("engine: %d actor(s) still hold a pending this reconciliation did "+
				"not re-check (%v): v1 re-checks every pending at every full reconciliation, so this "+
				"pass set was not full", len(rep.StillOpen), rep.StillOpen)
		}
		if len(survived) > 0 {
			return rep, fmt.Errorf("engine: %d pending(s) were re-checked by this reconciliation and "+
				"are STILL OPEN (%v): the system did not settle, so this is a disagreement the run "+
				"carries and not a clean pass", len(survived), survived)
		}
	}
	sort.Strings(rep.ReChecked)
	return rep, nil
}
