package engine

import (
	"context"
	"fmt"
	"time"

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
	Of string // the claim type, e.g. "user.created"

	// Bound is how long after the claim a disagreement is tolerated, in
	// NANOSECONDS -- time.Duration's own unit, which is what the bindings
	// parser produces from `bound = "30s"`.
	//
	// THE UNIT IS NAMED HERE BECAUSE NOTHING NAMED IT, and the field was
	// silently zero in every Eventual the product built until coop
	// rumor/1191. The comparison below is against a CLOCK FACT's `t`, whose
	// unit is R7's to settle when the run controls the clock; until then the
	// duration form is refused before the comparison is reached, so this
	// field's job is to be NAMED in that refusal rather than to decide a
	// verdict. R7: make a clock fact's `t` nanoseconds too, or convert here.
	Bound int64

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
func pendingOverlay(l *ledger.Ledger, c Check, b Bindings, statuteValue any) (*Overlay, *pastBound, error) {
	if statuteValue == "consistent" {
		return nil, nil, nil
	}
	e, ok := b.eventual(c.Of)
	if !ok {
		return nil, nil, nil
	}
	prefix := l.SealedPrefix()
	claimPos, ok := latestClaimOf(prefix, c.Of)
	if !ok {
		return nil, nil, nil
	}
	// THE QUIESCE BOUND IS JUDGED WITHOUT A CLOCK, which is why R6 can judge
	// it at all: the question is whether the system has gone quiet, not how
	// long ago the claim was made. R6.2's probes answer it and hand the answer
	// in on the check; this package only reads it.
	//
	// AND A QUIESCED DISAGREEMENT IS NOT `stale` BY ITSELF. This said the
	// opposite -- "a quiesced disagreement IS `stale`, not `bug` plain: the
	// system was given its chance to settle, it said it had settled, and it
	// still disagrees" -- and fable overturned it at coop r6/87: being given
	// the chance to settle is what a HELD pending records, and a first look at
	// a quiet system was never waiting for anything. So this function answers
	// `pending` or hands the facts back, and Judge decides the word where the
	// re-check is in hand.
	//
	// THE SENTENCE OUTLIVED THE ARM BELOW IT BY ONE COMMIT, which is the
	// finding ratchet reported at coop r6/91: it sat ten lines above code
	// saying the opposite, in the summary position a reader meets first. A
	// comment that states an overturned rule is worse than none, and the
	// overturned text is kept here deliberately -- quoted as what was wrong --
	// because a reader who learned the old rule from this file needs to find
	// it refuted where they first read it.
	if e.Quiesce {
		switch c.Quiesce {
		case QuiesceBusy:
			return &Overlay{
				Rule:  "engine.pending",
				Value: "pending",
				Reason: fmt.Sprintf("%q is eventual bounded %q and the system is still busy, "+
					"so the disagreement on the claim at position %d is inside the bound",
					c.Of, "quiesce", claimPos),
			}, nil, nil
		case QuiesceQuiet:
			// QUIET IS NOT A VERDICT HERE, which is the whole of fable's
			// ruling (coop r6/87). The system says it has settled and the
			// observation still disagrees -- but whether that is `stale`
			// depends on something this function cannot see: whether the
			// disagreement was ever HELD. On a FIRST look there was no lag to
			// excuse, so it reads as an immediate type would, a plain `bug`
			// with the statute's own rule; after a pending was held and
			// re-checked, it is the word. Judge has the register and decides.
			//
			// THREE ARMS PINNED THE OTHER ANSWER and they were rewritten with
			// this change: each passed a register that could hold no pending
			// for the query -- two passing nil, one building a fresh one -- so
			// `stale` on first sight was the expectation BY CONSTRUCTION
			// (ratchet, coop r6/85). They sat where nothing could contradict
			// them: no production path called Judge, so the wrong word was
			// pinned in the one region a walk never reaches.
			return nil, &pastBound{of: c.Of, claimPos: claimPos, byQuiesce: true}, nil
		case QuiesceTimedOut:
			// AND IT IS REACHED HERE ONLY WITH A DISAGREEMENT OPEN, WHICH IS
			// NARROWER THAN THE WORD. fable amended the definition at coop
			// r6/42: the finding IS the timeout -- the declared probes did not
			// read quiet within their timeouts -- and a system that would not
			// settle is a thing the run found "whether or not a disagreement
			// was open at the moment it gave up". A pending is what it usually
			// CARRIES, not what it requires.
			//
			// pendingOverlay CANNOT SAY THAT, and the other half now exists.
			// This returns at the top when the statute found agreement, so with
			// every observation consistent a probe that gave up is invisible
			// here -- which is not a defect of this function, an overlay being
			// by construction a verdict ON a finding. internal/schedule's walk
			// asks the prober after its last step and produces the finding for
			// that case; this arm stays the path for a timeout with a pending
			// ALSO open. Two sites, one word, and the division is which of them
			// can see a run where nothing disagreed.
			//
			// THE FIFTH FINDING, AND NOT A KIND UNDER `bug`. fable's D7 (coop
			// breakout-r6/0) rules it "neither `bug` nor a refusal, and the
			// exit is 3", and the reason is whose fault it is: the project is
			// not wrong and its adapter is not wrong, the SYSTEM would not go
			// quiet, which is a thing the run FOUND. A refusal would tell the
			// project's reader to fix their declarations, and there is nothing
			// there to fix.
			//
			// THE WORD IS v1'S OWN RESERVATION, not one invented here.
			// docs/vocabulary.md reserved "kinds under `bug` and one finding
			// beside these four" and listed `stale`, a quiesce timeout,
			// `ui_timeout` and `trespass`; `stale` was spent as a KIND at v2
			// and `trespass` is already "not a finding here", so the one
			// finding beside the four is this. The spelling follows
			// `ui_timeout`, which v1 tokenized where it left this as prose
			// (ibis, coop r6/31).
			//
			// IT CARRIES NO KIND, deliberately. Kind narrows a `bug` by saying
			// why it is one; a finding of its own needs no narrowing, and
			// setting both would make a reader keying on Value see a bug that
			// this is not.
			return &Overlay{
				Rule:  "engine.quiesce_timeout",
				Value: FindingQuiesceTimedOut,
				Reason: fmt.Sprintf("%q is eventual bounded %q and the quiesce probe gave up "+
					"before the system went quiet, so the disagreement on the claim at "+
					"position %d is neither inside the bound nor past it: the run found that "+
					"the system would not settle", c.Of, "quiesce", claimPos),
			}, nil, nil
		case QuiesceAbsent:
			// ABSENT, which is not an answer. A caller with no prober wired
			// must not have its silence read as "not quiet": that would hold a
			// real disagreement open to the end of a run and report it as
			// clean, which is the outcome ratchet drove at r6/8.
			return nil, nil, fmt.Errorf("engine: %q is eventual bounded %q and no quiesce answer "+
				"was supplied (%s): the bound cannot be judged, and treating silence as "+
				"\"not quiet\" would hold this disagreement open for the whole run and "+
				"report it as clean", c.Of, "quiesce", c.Quiesce)
		default:
			// A STATE THIS SWITCH HAS NOT MET, named rather than folded into
			// the absent clause (ratchet, coop r6/19). Go checks no switch for
			// exhaustiveness, and this is the enum most likely to GROW: R6.2
			// owns the probes and the timed-out kind is theirs to define. A
			// new state falling into "no quiesce answer was supplied" would be
			// a correct refusal naming the wrong cause, which is a branch
			// written for one state standing in for every state that reaches
			// it.
			return nil, nil, fmt.Errorf("engine: %q is eventual bounded %q and the quiesce answer "+
				"is %d, a state this build does not know: it refuses rather than reading an "+
				"unknown answer as any of quiet, busy or timed out",
				c.Of, "quiesce", int(c.Quiesce))
		}
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
		return nil, nil, fmt.Errorf("engine: %q is declared eventual with a bound of %s, but its bound "+
			"cannot be measured: a clock fact at the seal is present=%v, and one at or before the "+
			"last %q claim (position %d) is present=%v. Treating that as past the bound would turn "+
			"a pending into a defect on missing data, which is the R4 shape -- time the run does "+
			"not control never decides a verdict",
			c.Of, time.Duration(e.Bound), hasNow, c.Of, claimPos, hasClaimTime)
	}
	// PAST THE BOUND IS FACTS AND NOT THE WORD, for the quiesce arm's reason
	// above: `stale` is a pending that WAS HELD and still disagrees, and this
	// function cannot see the register. The duration form keeps R2.3's refusal
	// until a later phase gives the run the clock, so nothing reaches here in
	// R6 with a duration bound -- but the arm is written the same way as its
	// sibling rather than left as the one place the word could still be
	// composed from an unheld disagreement.
	if now-claimTime > e.Bound {
		return nil, &pastBound{
			of: c.Of, claimPos: claimPos, bound: e.Bound, claimTime: claimTime, now: now,
		}, nil
	}
	return &Overlay{
		Rule:  "engine.pending",
		Value: "pending",
		Reason: fmt.Sprintf("%q is eventual with a bound of %s; the last claim of it was made at %d "+
			"and the run's time at the seal is %d, so the disagreement is inside the bound",
			c.Of, time.Duration(e.Bound), claimTime, now),
		Until: claimTime + e.Bound,
	}, nil, nil
}

// pastBound is what pendingOverlay found when a disagreement is NOT inside the
// bound, handed back for Judge to decide on.
//
// IT CARRIES FACTS AND NOT A VERDICT, which is fable's ruling at coop r6/87:
// "Judge decides `stale` ... and pendingOverlay yields `pending` or nothing and
// never the word". The overlay can see that the bound is past; it cannot see
// whether a pending was ever HELD, because the register is not among its
// arguments -- and docs/vocabulary.md :118 makes being held open part of what
// the word means. An overlay that named it anyway was answering a question it
// could not ask.
type pastBound struct {
	of       string
	claimPos int
	// byQuiesce says the probes answered, so the message speaks of the system
	// going quiet rather than of a duration; the clock fields are then unset.
	byQuiesce bool
	bound     int64
	claimTime int64
	now       int64
}

// stale is the ONE PLACE the word is composed, and only Judge calls it.
func stale(p pastBound, prev Pending) *Overlay {
	if p.byQuiesce {
		return &Overlay{
			Rule:  "engine.stale",
			Value: "bug",
			Kind:  "stale",
			Reason: fmt.Sprintf("%q is eventual bounded %q; the disagreement on the claim at "+
				"position %d was held open as `pending` at seal %d, the system has since read "+
				"quiet, and it still disagrees: the bound is past",
				p.of, "quiesce", p.claimPos, prev.At),
		}
	}
	return &Overlay{
		Rule:  "engine.stale",
		Value: "bug",
		Kind:  "stale",
		Reason: fmt.Sprintf("%q is eventual with a bound of %s; the disagreement on the claim at "+
			"position %d was held open as `pending` at seal %d, the claim was made at %d and the "+
			"run's time at the seal is %d, so it is %d past the bound",
			p.of, time.Duration(p.bound), p.claimPos, prev.At, p.claimTime, p.now,
			(p.now-p.claimTime)-p.bound),
		Until: p.claimTime + p.bound,
	}
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

	ov, past, err := pendingOverlay(l, c, b, f.Value)
	if err != nil {
		return f, err
	}
	// THE WORD IS DECIDED HERE AND NOWHERE ELSE, which is fable's placement at
	// coop r6/87. `stale` is "a `pending` past its bound" -- docs/vocabulary.md
	// :118 requires that the disagreement "was inside the bound and so was held
	// open", and :130 gives it only "if the reconciliation that FOLLOWS still
	// disagrees". Both halves are in hand at this point and in no other: `past`
	// says the bound is behind us, and f.ReChecked says this observation IS the
	// re-check of a pending this run held.
	//
	// A FIRST LOOK PAST THE BOUND IS A PLAIN `bug`, deliberately, and it is not
	// a weaker answer. There was no lag to excuse: the system reads quiet and
	// the observation disagrees, so the type behaves exactly as an immediate one
	// would and the statute's own finding and rule stand. Reporting `stale`
	// there spent the word on a disagreement nothing had ever been waiting for,
	// and it moved two of ruling 3's identity fields on an R5 row that had not
	// changed (ibis, coop r6/84; seed 40 read engine.stale where the baseline
	// reads the statute's rule).
	//
	// ReChecked STOPS BEING REPORT-ONLY. It was set here and read only into a
	// report (ratchet, coop r6/86: "no non-test site reads it into a Value or a
	// Kind"), which is why the register could not reach the decision.
	if past != nil {
		if f.ReChecked != nil {
			ov = stale(*past, *f.ReChecked)
		}
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
