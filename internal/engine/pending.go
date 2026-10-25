package engine

import (
	"context"
	"fmt"

	"forge.axonibyte.com/axonibyte/rumor/internal/ledger"
)

// THE CLOCK AND `pending`, which R2.3 is where the vocabulary says they get
// driven (docs/vocabulary.md, Fact kinds: "no example carries a `clock` fact
// until R2.3 drives one"). Both are driven here, in this package's tests, and
// not by a fourth example package: v1 says plainly that `pending` "has no
// example yet" because it needs an `eventual` type with a bound declared in the
// bindings and the bindings artefact arrives with R4. Adding an example would
// also move tests/statute's declared 3/17/19, which are R1's gated numbers.
//
// A MEASUREMENT AGAINST THE SAME ENTRY, AND IT IS NOW SETTLED. This comment
// used to report the Fact kinds note's told/clock claim as an open discrepancy
// -- `told` in 11 of the 17 ledger files, `clock` in none -- and leave the call
// to fable and ratchet on thread:breakout-r2. They made it: fable worded the
// amendment at breakout-r2/19 and it landed in the same tag as this file, so the
// entry now counts what is there and names the `clock` gap outright. Reporting
// rather than quietly fixing was right; the report outlived its question by half
// an hour, which quill found at rumor/577.
//
// THE SUPERSEDED WORDING IS DESCRIBED HERE AND NOT QUOTED, deliberately.
// tests/vocabulary holds every attributed quote in this tree to the document's
// CURRENT text with no exception for historical ones, because an exception is
// exactly the hole a stale quote hides in. A comment that needs to talk about
// an old wording says what it said.

// Eventual declares a claim type whose agreement is allowed to lag, with the
// bound inside which a disagreement is `pending` rather than a defect.
//
// WHERE THESE COME FROM IS R4's. The bindings artefact declares them; this is
// the shape the engine needs to apply one, and nothing here reads a file.
type Eventual struct {
	Of    string // the claim type, e.g. "user.created"
	Bound int64  // how long after the claim a disagreement is tolerated

	// Quiesce says the bound is "until the system settles" rather than a
	// duration, which is `bindings.Quiesce` as the artefact spells it.
	//
	// IT IS THE ONLY BOUND A WALK CAN JUDGE IN R6 (fable, coop
	// breakout-r6/0 ruling 1): a duration needs the run to control the
	// clock, and the clock arrives in R7, so a duration bound stays exactly
	// where R2.3 left it -- declared, validated, and refused at run time
	// until a clock fact exists. The quiesce form needs no clock: it asks
	// whether the system has gone quiet, which R6.2's probes answer and
	// this package only judges.
	Quiesce bool
}

// Bindings is what a run declares about the project under test. v1 names the
// artefact and R4 builds it; only the part the engine needs is here.
type Bindings struct {
	Eventual []Eventual
}

func (b Bindings) eventual(of string) (Eventual, bool) {
	for _, e := range b.Eventual {
		if e.Of == of {
			return e, true
		}
	}
	return Eventual{}, false
}

// Overlay records a finding the ENGINE produced rather than the statute.
//
// WHY `pending` IS NOT A STATUTE RULE IN v1. It depends on two things no
// statute has: the declared bound for an eventual type, which lives in the
// bindings, and the run's time, which is a clock fact. The three v1 statutes
// read neither, and they are pinned. So the engine applies `pending` over the
// statute's own finding and says so -- the statute's verdict is kept in
// StatuteValue and the definition that produced it in Fired, so nothing is
// hidden. If a later phase hands the bound to statutes instead, this overlay is
// the single thing that goes.
type Overlay struct {
	Rule   string // the engine rule that produced the finding, e.g. "engine.pending"
	Reason string
	Until  int64 // the clock time the bound expires at; 0 for a quiesce bound

	// Value is the finding this overlay imposes, and Kind narrows it.
	//
	// THEY ARE HERE BECAUSE THE OVERLAY NOW HAS TWO OUTCOMES. Until R6.1 it
	// had one -- hold the disagreement open as `pending` -- so Judge could
	// write the word itself. Past the bound it now imposes `bug` of kind
	// `stale`, which v1 reserved and R6 spends: a caller that read only
	// Rule could not tell the two apart, and a caller that read the word
	// out of Reason would be parsing prose.
	Value any
	Kind  string
}

// Now is the run's time at the seal: the latest clock fact at or before it.
// Absent when the sealed prefix holds none, which is not an error here -- a
// project whose behaviour does not depend on time writes no clock facts.
func Now(l *ledger.Ledger) (int64, bool) {
	return timeAt(l.SealedPrefix(), l.Seal())
}

// timeAt is the run's time at position p: the latest clock fact at or before
// it. The prefix is ordered by position, so the last match wins.
func timeAt(prefix []ledger.Fact, p int) (int64, bool) {
	var t int64
	found := false
	for _, f := range prefix {
		if f.Kind == ledger.KindClock && f.Position <= p && f.T != nil {
			t, found = *f.T, true
		}
	}
	return t, found
}

// latestClaimOf is the position of the last claim of this type in the prefix.
func latestClaimOf(prefix []ledger.Fact, of string) (int, bool) {
	pos, found := 0, false
	for _, f := range prefix {
		if f.Kind == ledger.KindClaim && f.Type == of && f.Position > pos {
			pos, found = f.Position, true
		}
	}
	return pos, found
}

// pendingOverlay decides whether a statute's finding is held open as `pending`.
//
// THE FOUR WAYS IT DECLINES ARE EACH A DECISION, not a fallthrough:
//
//   - the statute found agreement: there is nothing to hold open.
//   - the type is not declared eventual: v1 confines `pending` to those, so a
//     disagreement on an immediate type stays whatever the statute called it.
//   - no claim of the type sits in the prefix: nothing to be late about.
//   - the bound has passed: v1 RESERVES `stale` for R6 and defines nothing, and
//     tells the examples to avoid the word. So past the bound the statute's
//     finding stands and the engine does not invent a verdict it has not been
//     given.
//
// It REFUSES rather than declining when the bound cannot be judged: an eventual
// type with no clock fact to measure against would otherwise silently read as
// "past the bound", turning a `pending` into a `bug` on missing data. That is
// the R4 shape -- time the run does not control never enters a verdict.
func pendingOverlay(l *ledger.Ledger, c Check, b Bindings, statuteValue any) (*Overlay, error) {
	if statuteValue == "consistent" {
		return nil, nil
	}
	e, ok := b.eventual(c.Of)
	if !ok {
		return nil, nil
	}
	prefix := l.SealedPrefix()
	claimPos, ok := latestClaimOf(prefix, c.Of)
	if !ok {
		return nil, nil
	}
	// THE QUIESCE BOUND IS JUDGED WITHOUT A CLOCK, which is why R6 can judge
	// it at all: the question is whether the system has gone quiet, not how
	// long ago the claim was made. R6.2's probes answer it and hand the answer
	// in on the check; this package only reads it.
	//
	// AND A QUIESCED DISAGREEMENT IS `stale`, not `bug` plain: the system was
	// given its chance to settle, it said it had settled, and it still
	// disagrees. That is the word v1 reserved and R6.1 spends.
	if e.Quiesce {
		if !c.Quiesced {
			return &Overlay{
				Rule:  "engine.pending",
				Value: "pending",
				Reason: fmt.Sprintf("%q is eventual bounded %q and the system has not read quiet, "+
					"so the disagreement on the claim at position %d is inside the bound",
					c.Of, "quiesce", claimPos),
			}, nil
		}
		return &Overlay{
			Rule:  "engine.stale",
			Value: "bug",
			Kind:  "stale",
			Reason: fmt.Sprintf("%q is eventual bounded %q, the system read quiet, and the "+
				"observation still disagrees with the claim at position %d: the bound is past",
				c.Of, "quiesce", claimPos),
		}, nil
	}
	// ONE REFUSAL, NOT TWO, and mutation is what settled it. A separate check
	// for a missing `now` could never be the clause that answered: the clock
	// that gives claimTime sits at or before claimPos, and claimPos is inside
	// the prefix, so the seal sees it too. Disabling that check killed no test,
	// because the other one always fired first with a message containing the
	// same words. So the two are one clause that names both facts, and a reader
	// is told which half was missing rather than inferring it.
	now, hasNow := Now(l)
	claimTime, hasClaimTime := timeAt(prefix, claimPos)
	if !hasNow || !hasClaimTime {
		return nil, fmt.Errorf("engine: %q is declared eventual with a bound of %d, but its bound "+
			"cannot be measured: a clock fact at the seal is present=%v, and one at or before the "+
			"last %q claim (position %d) is present=%v. Treating that as past the bound would turn "+
			"a pending into a defect on missing data, which is the R4 shape -- time the run does "+
			"not control never decides a verdict",
			c.Of, e.Bound, hasNow, c.Of, claimPos, hasClaimTime)
	}
	// PAST THE BOUND IS `stale` NOW, where it used to decline. The comment
	// above this function said why it declined: "v1 RESERVES `stale` for R6 and
	// defines nothing, and tells the examples to avoid the word. So past the
	// bound the statute's finding stands and the engine does not invent a
	// verdict it has not been given." R6.1 is where the word is defined, so the
	// engine has been given it and the decline becomes a finding.
	//
	// IT NAMES THE TYPE, THE POSITION AND THE BOUND, because a `stale` a reader
	// cannot locate is a `bug` with a longer name: the three together say which
	// claim was late and by what measure.
	if now-claimTime > e.Bound {
		return &Overlay{
			Rule:  "engine.stale",
			Value: "bug",
			Kind:  "stale",
			Reason: fmt.Sprintf("%q is eventual with a bound of %d; the last claim of it was made "+
				"at %d and the run's time at the seal is %d, so the disagreement is %d past the "+
				"bound (claim at position %d)",
				c.Of, e.Bound, claimTime, now, (now-claimTime)-e.Bound, claimPos),
			Until: claimTime + e.Bound,
		}, nil
	}
	return &Overlay{
		Rule:  "engine.pending",
		Value: "pending",
		Reason: fmt.Sprintf("%q is eventual with a bound of %d; the last claim of it was made at %d "+
			"and the run's time at the seal is %d, so the disagreement is inside the bound",
			c.Of, e.Bound, claimTime, now),
		Until: claimTime + e.Bound,
	}, nil
}

// Pending is a finding held open for re-check.
type Pending struct {
	Check   Check
	At      int   // the seal it was judged at
	Until   int64 // the clock time its bound expires at
	Statute any   // what the statute itself called it
}

// Register holds the pending findings a run has open.
//
// v1: a `pending` "is re-checked at the actor's next observation of the same
// query and at every full reconciliation (R6)". So the key is the pair (actor,
// query) and nothing else: a different query from the same actor does not
// re-check it, and the same query from a different actor is a different
// observer's verdict.
type Register struct {
	open map[registerKey]Pending
}

type registerKey struct {
	actor, query string
}

// NewRegister returns an empty register.
func NewRegister() *Register {
	return &Register{open: map[registerKey]Pending{}}
}

// Open lists what is held open, for a caller that needs to know a
// reconciliation has not covered everything.
func (r *Register) Open() map[string][]string {
	out := map[string][]string{}
	for k := range r.open {
		out[k.actor] = append(out[k.actor], k.query)
	}
	return out
}

// Len is how many findings are held open.
func (r *Register) Len() int { return len(r.open) }

// Judge evaluates one check and applies the run's bindings, holding a finding
// open as `pending` where an eventual type's bound covers the disagreement.
//
// IT IS ALSO THE RE-CHECK. If the register holds a pending for this actor and
// query, this observation IS v1's "next observation of the same query", so the
// old pending is cleared before the new verdict is recorded -- whatever that
// verdict turns out to be. Clearing it only when the new finding is consistent
// would leave a pending that had become a `bug` sitting in the register as
// though it were still merely late.
func Judge(ctx context.Context, statute string, rules []string, l *ledger.Ledger, c Check,
	b Bindings, reg *Register) (Finding, error) {
	f, err := Evaluate(ctx, statute, rules, l, c)
	if err != nil {
		return f, err
	}
	f.StatuteValue = f.Value

	if reg != nil {
		k := registerKey{actor: c.Actor, query: c.Query}
		if prev, held := reg.open[k]; held {
			f.ReChecked = &prev
			delete(reg.open, k)
		}
	}

	ov, err := pendingOverlay(l, c, b, f.Value)
	if err != nil {
		return f, err
	}
	if ov != nil {
		f.Value = ov.Value
		f.Kind = ov.Kind
		f.Rule = ov.Rule
		f.Overlay = ov
		// ONLY A PENDING IS REGISTERED FOR RE-CHECK. A `stale` is terminal:
		// the bound is past, so there is nothing left to re-check and holding
		// it open would leave the register claiming a decided finding is still
		// waiting -- and R2.3's rule that no pending is left open at the end of
		// a run would then be false of a run that ended on one.
		if reg != nil && ov.Value == "pending" {
			reg.open[registerKey{actor: c.Actor, query: c.Query}] = Pending{
				Check: c, At: l.Seal(), Until: ov.Until, Statute: f.StatuteValue,
			}
		}
	}
	return f, nil
}
