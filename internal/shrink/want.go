package shrink

import (
	"fmt"

	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
)

// Want is the verdict a shrink keeps: what a candidate must still produce for a
// reduction to be accepted.
//
// A VERDICT IS NOT ONLY A STATUTE's, which is fable's ruling at coop
// rumor/1521 on ibis r8/49 and rook r8/38. Every reducer here took `want
// engine.Finding` -- Once, Bisect and RemoveKinds alike -- so the only verdict
// a shrink could keep was one the statute produced by evaluating rules against
// the ledger. A concurrency overfill is not such a verdict and cannot become
// one: examples/toy's statute says so of ITSELF, and for a data-model reason
// rather than a wording one -- "THERE IS NO CAPACITY FIELD ... an event's
// capacity is not a ledger fact, a capacity OVERRUN is not nameable from a
// ledger case, and this statute does not pretend otherwise" (statute.rego:12-19).
// So a stage finding had no `want` to hand the shrinker at all, whatever the
// gate did, and D8 over it was unreachable rather than merely unreached.
//
// THE TYPE IS WIDENED AND THE STATUTE's DISCLAIMER STAYS TRUE: the stage names
// its own finding by its own oracle, which is exactly why this is the stage's
// to say and not the statute's to be taught.
//
// WHAT A SHRINK ASKS OF A CANDIDATE IS UNCHANGED IN SHAPE -- "does this
// candidate still give THAT verdict" -- so the interface is that question and
// nothing more.
type Want interface {
	// Recurs reports whether this candidate still gives the verdict, and when
	// it does not, WHY in one sentence.
	//
	// THE REASON IS THE WANT's TO WRITE, because only the author of a verdict
	// knows what separated it from what came back: the statute's reading names
	// values, kinds, rules and attribution, and a stage's names its own
	// quantities. A shrinker that composed the sentence itself would have to
	// know every verdict there will ever be.
	Recurs(Candidate) (bool, string)

	// Describe names the verdict for a report, so an unstable seed's line can
	// say what was being looked for.
	Describe() string
}

// StatuteVerdict is the statute's finding as a Want: the reading every reducer
// had before the type was widened, moved behind the interface UNCHANGED.
//
// IT IS VERBATIM ON PURPOSE. The walk is the only caller today, and a widening
// that altered what a statute verdict means would be a change to the walk
// disguised as a refactor -- so the clause order, the wording of every reason
// and the attribution handling are the ones that were in Once, and the
// package's existing arms are what say so.
type StatuteVerdict struct {
	Finding engine.Finding
}

// Describe names the finding the way a report does.
func (s StatuteVerdict) Describe() string {
	if !s.Finding.Defined {
		return "no finding"
	}
	if s.Finding.Kind != "" {
		return fmt.Sprintf("%v of kind %s", s.Finding.Value, s.Finding.Kind)
	}
	return fmt.Sprint(s.Finding.Value)
}

// Recurs is the comparison that used to sit in Once, moved and not rewritten.
func (s StatuteVerdict) Recurs(cand Candidate) (bool, string) {
	want, got := s.Finding, cand.Finding
	wantAttr, wantHas := want.Rules[attributionRule]
	gotAttr, gotHas := got.Rules[attributionRule]

	switch {
	case want.Defined != got.Defined:
		return false, fmt.Sprintf("the original %s and the replay %s",
			defined(want.Defined), defined(got.Defined))
	case want.Defined && want.Value != got.Value:
		return false, fmt.Sprintf("the original found %v and the replay found %v",
			want.Value, got.Value)
	case want.Defined && want.Kind != got.Kind:
		// A KIND JOINS THE RULE, THE VALUE AND THE ATTRIBUTION in "the same
		// finding" (fable, coop breakout-r6/0, R6.4's clause). `stale` is a
		// `bug` whose kind says WHY it is one -- a pending past its bound -- so
		// two findings that agree on the value and differ on the kind are two
		// different defects wearing one word, and a shrink that traded one for
		// the other would reduce against a failure it did not find.
		return false, fmt.Sprintf("both found %v, but the original is of kind %s and the replay %s",
			want.Value, kindName(want.Kind), kindName(got.Kind))
	case wantHas != gotHas:
		// THE RULE BECOMING UNDEFINED IS A DIFFERENCE, not an absence to pass
		// over: engine.Finding's Rules is documented there as carrying every
		// rule the caller named and being absent when undefined, so one side
		// having an attribution and the other not is a finding that no longer
		// rests on what it rested on.
		return false, fmt.Sprintf("the original %s an attribution and the replay %s one",
			had(wantHas), had(gotHas))
	case wantHas && gotHas && wantAttr != gotAttr:
		return false, fmt.Sprintf("both found %v, but the original rests_on_planted %v and the replay %v",
			want.Value, wantAttr, gotAttr)
	case want.Rule != got.Rule:
		// A FINDING NAMES THE RULE THAT PRODUCED IT, so one value reached by a
		// different rule is a different finding.
		return false, fmt.Sprintf("both found %v, but by %s and then %s",
			want.Value, want.Rule, got.Rule)
	}
	return true, ""
}

// Compares reports whether this want read an attribution on either side, which
// Verdict carries so a reader can tell "all three agreed" from "two were
// available".
//
// IT IS THE STATUTE's QUESTION AND NOT EVERY WANT's, so it is a method here
// rather than a member of the interface: a stage verdict has no
// `rests_on_planted` rule to compare, and asking every Want for one would make
// the field mean "false" where it should mean "not applicable".
func (s StatuteVerdict) Compares(cand Candidate) bool {
	_, wantHas := s.Finding.Rules[attributionRule]
	_, gotHas := cand.Finding.Rules[attributionRule]
	return wantHas || gotHas
}
