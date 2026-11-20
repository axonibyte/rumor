// Render is the derivation written for a person (#132, R11 d2).
//
// THE ORDER IS THE EPIC'S AND NOT THE STRUCT'S: the finding, the observation,
// then the chain that produced it, shortest path first. d1's value is
// ordered for a schema -- selector, verdict, expectation, observation, clock,
// reproduction -- and a reader's question runs the other way: what did the
// statute hold, what contradicted it, and why did it hold that.
//
// IT PROPOSES NO FIELD. Every line below is a function of what d1 already
// carries (fable's condition at coop r11/18): a field this render wants is a
// finding against d1 for rook's read, not a change made here.

package explain

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// Render writes the derivation to w.
//
// WHY A WRITER AND NOT A STRING: the command streams to stdout, and a failed
// write is the command's verdict to report rather than this function's to
// swallow -- the same argument `rumor docs --check` makes about its own one
// line of output.
func Render(w io.Writer, d *Derivation) error {
	if d == nil {
		return fmt.Errorf("explain: no derivation to render, which is a caller's defect and not an empty explanation")
	}
	var b strings.Builder

	// THE HEADING NAMES THE FINDING, because nothing in a finding identifies
	// it and a reader with two explanations open needs to tell them apart.
	fmt.Fprintf(&b, "EXPLAIN %s %s\n", d.Reproduction.Package, d.Reproduction.Case)
	fmt.Fprintf(&b, "  %s\n\n", selectorLine(d.Selector))

	// 1. WHAT THE STATUTE HELD, and the line that held it.
	//
	// THE LABEL IS "FINDING" AND NOT "EXPECTED", which is the same correction
	// d1's shape needed: `bug`, `consistent`, `explained` and `pending` are
	// words from the findings vocabulary, not values anybody expected, and
	// "EXPECTED bug" reads as though a bug were wanted. The epic's order --
	// expectation, observation, chain -- is kept; only the word is right.
	fmt.Fprintf(&b, "FINDING    %s\n", d.Verdict.Says)
	for _, line := range verdictLines(d.Verdict) {
		fmt.Fprintf(&b, "           %s\n", line)
	}

	// 2. THE OBSERVATION: what contradicted it. Kept to its own block for the
	// reason d1 keeps it to its own field -- the reader is deciding whether
	// the chain rests on a rule they disagree with or on facts they agree
	// with, and merging the two destroys the distinction.
	fmt.Fprintf(&b, "\nOBSERVED   %s\n", observedLine(d.Observation))

	// 3. THE CHAIN, shortest path first.
	fmt.Fprintf(&b, "\nWHY THE STATUTE HELD THAT -- %s's knowledge, shortest path first\n", d.Selector.Actor)
	known := chainOrder(d.Expectation.Known)
	if len(known) == 0 {
		fmt.Fprintf(&b, "  (nothing: %s knew no fact at this position)\n", d.Selector.Actor)
	}
	for _, kf := range known {
		fmt.Fprintf(&b, "  %-18s %s\n", kf.Ref, originsLine(kf.Via, d.Selector.Actor))
	}

	// AND WHAT THE ACTOR DOES NOT KNOW, which is the other half of a
	// knowledge bug and is derivable from the same two fields: a claim in the
	// sealed prefix whose ref the actor's `known` does not carry is a thing
	// that HAPPENED and that this actor cannot account for. For a count that
	// reads low, this block is the answer.
	if unknown := unknownToActor(d); len(unknown) > 0 {
		fmt.Fprintf(&b, "\nIN THE PREFIX, NOT KNOWN TO %s\n", strings.ToUpper(d.Selector.Actor))
		for _, cl := range unknown {
			fmt.Fprintf(&b, "  %-18s %s claimed %s at %d\n", cl.Ref, cl.Actor, cl.Type, cl.Position)
		}
	}

	// The statute's other answers, under its own names, each with the line
	// that produced it.
	//
	// THE LINE IS HERE BECAUSE THE DEFAULT ROW ABOVE IS INNOCENT. When the
	// wrong rule is upstream of the finding, the row beside FINDING fired
	// exactly right and the rule to go and read is in this block -- so a
	// reader who gets no line here is sent back to the file the explanation
	// was supposed to replace (ibis, coop r11/24; fable's ruling at r11/26).
	//
	// AND "not reached" IS PRINTED RATHER THAN LEFT BLANK, because a blank
	// says nothing about why: a rule whose lines are absent was never touched
	// while this finding was computed, and that is the one thing it proves --
	// it cannot be the error.
	// REACHED FIRST, THEN ALPHABETICAL WITHIN EACH GROUP. I told the room I
	// had no basis to rank a statute's own rules and would leave the order to
	// rook's cold read (coop r11/23); the basis arrived with the lines. A rule
	// the finding's evaluation touched is a candidate for the error and a rule
	// it never touched cannot be one, so the candidates go first -- which is
	// also what makes a fourteen-rule block readable when twelve of them say
	// "(not reached)". Deterministic either way: the group is a fact and the
	// name breaks every tie.
	if len(d.Expectation.Holds) > 0 {
		fmt.Fprintf(&b, "\nTHE STATUTE ALSO HELD -- reached rules first; a rule nothing touched cannot be the error\n")
		for _, rule := range holdOrder(d.Expectation.Holds) {
			h := d.Expectation.Holds[rule]
			fmt.Fprintf(&b, "  %-22s %-10v %s\n", rule, h.Value, holdLines(h))
		}
	}

	if d.Clock != nil {
		fmt.Fprintf(&b, "\nCLOCK      the run was at %d\n", d.Clock.Now)
	}

	// 4. THE REPRODUCTION, LAST IN THE OUTPUT AND LAST IN THIS FILE. It is the
	// element rook's condition from r11/2 can still move -- "from the JSON
	// alone, reach the raw trace" -- so it is written here, at the end, where
	// a re-cut after the clear touches one block and nothing above it
	// (fable's condition at r11/18).
	fmt.Fprintf(&b, "\nREPRODUCTION  %s\n", reproductionLine(d.Reproduction))

	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("explain: the derivation could not be written: %w", err)
	}
	return nil
}

// selectorLine names the check: the actor, what was asked, through what, where.
func selectorLine(s Selector) string {
	parts := []string{fmt.Sprintf("%s asked %s", s.Actor, orNone(s.Query))}
	if s.Of != "" {
		parts = append(parts, fmt.Sprintf("counting %s", s.Of))
	}
	if s.Source != "" {
		parts = append(parts, fmt.Sprintf("through %s", s.Source))
	}
	parts = append(parts, fmt.Sprintf("at position %d", s.Position))
	return strings.Join(parts, ", ")
}

// verdictLines names the statute lines, and says what a default row MEANS.
//
// "NO CLAUSE MATCHED" IS THE SENTENCE A READER NEEDS and it is not decoration:
// a finding off a `default` row says the statute reached its fallback, which
// reads nothing like a clause firing, and in examples/toy all ten bugs land
// there. A reader who sees only "statute.rego:396" has to open the file to
// learn that.
func verdictLines(v Verdict) []string {
	var out []string
	switch {
	case len(v.Rules) == 0 && !v.Defined:
		out = append(out, "no statute line held: the statute defined nothing for this check")
	case len(v.Rules) == 0:
		out = append(out, "no statute line recorded for a defined finding, which is a defect to report")
	}
	for _, r := range v.Rules {
		if r.Default {
			out = append(out, fmt.Sprintf("%s:%d -- the DEFAULT row: no clause matched", r.Module, r.Row))
			continue
		}
		out = append(out, fmt.Sprintf("%s:%d -- a clause matched", r.Module, r.Row))
	}
	if v.Overridden && v.Overlay != nil {
		out = append(out, fmt.Sprintf("the statute said %v; %s overrode it%s",
			v.StatuteValue, v.Overlay.Rule, untilClause(v.Overlay)))
		if v.Overlay.Reason != "" {
			out = append(out, v.Overlay.Reason)
		}
	}
	return out
}

func untilClause(o *Overlay) string {
	if o.Until == 0 {
		return " until the system settles"
	}
	return fmt.Sprintf(" until %d", o.Until)
}

// observedLine renders what came back, with the role where the project has one.
func observedLine(o Observation) string {
	s := fmt.Sprintf("%v", o.Value)
	var tail []string
	if o.Source != "" {
		tail = append(tail, fmt.Sprintf("%s answered", o.Source))
	}
	if o.Query != "" {
		tail = append(tail, fmt.Sprintf("asked %s", o.Query))
	}
	if o.Of != "" {
		tail = append(tail, fmt.Sprintf("counting %s", o.Of))
	}
	if o.Role != "" {
		tail = append(tail, fmt.Sprintf("as %s", o.Role))
	}
	if len(tail) == 0 {
		return s
	}
	return s + "   (" + strings.Join(tail, ", ") + ")"
}

// chainOrder is "shortest path first", and the metric is named here because d1
// deliberately left it to this phase: an actor's OWN claim is no hop, a tell is
// one hop, and a fact reachable both ways is as short as its shortest origin.
// Within a depth the earliest origin position comes first, and the ref breaks a
// tie, so the order is total and the render is byte-stable across runs.
func chainOrder(known []KnownFact) []KnownFact {
	out := append([]KnownFact{}, known...)
	sort.SliceStable(out, func(i, j int) bool {
		di, dj := hops(out[i]), hops(out[j])
		if di != dj {
			return di < dj
		}
		pi, pj := firstPosition(out[i]), firstPosition(out[j])
		if pi != pj {
			return pi < pj
		}
		return out[i].Ref < out[j].Ref
	})
	return out
}

// hops is 0 for a fact the actor claimed themselves and 1 for one they were
// told. A fact with no origin cannot reach here -- Build refuses it through
// ErrDisagrees -- so the fallback is deliberately large rather than 0: an
// unordered fact must sort last and be visible, not silently lead the chain.
func hops(kf KnownFact) int {
	best := 1 << 30
	for _, o := range kf.Via {
		d := 1
		if o.Kind == "claim" {
			d = 0
		}
		if d < best {
			best = d
		}
	}
	return best
}

func firstPosition(kf KnownFact) int {
	best := 1 << 30
	for _, o := range kf.Via {
		if o.Position < best {
			best = o.Position
		}
	}
	return best
}

// originsLine says HOW the actor knows a fact, in the actor's own terms.
func originsLine(via []Origin, actor string) string {
	parts := make([]string, 0, len(via))
	for _, o := range via {
		switch o.Kind {
		case "claim":
			if o.Type != "" {
				parts = append(parts, fmt.Sprintf("%s's own claim at %d (%s)", o.Actor, o.Position, o.Type))
				continue
			}
			parts = append(parts, fmt.Sprintf("%s's own claim at %d", o.Actor, o.Position))
		case "tell":
			parts = append(parts, fmt.Sprintf("told to %s by %s at %d", o.To, o.Actor, o.Position))
		default:
			// Not reachable from Build, which writes only the two kinds, and
			// printed rather than dropped so a third kind arrives visibly.
			parts = append(parts, fmt.Sprintf("%s at %d", o.Kind, o.Position))
		}
	}
	if len(parts) == 0 {
		return "known for no reason the ledger explains"
	}
	return strings.Join(parts, "; ")
}

// unknownToActor lists the prefix claims the actor's knowledge does not carry.
func unknownToActor(d *Derivation) []Claim {
	known := make(map[string]bool, len(d.Expectation.Known))
	for _, kf := range d.Expectation.Known {
		known[kf.Ref] = true
	}
	var out []Claim
	for _, cl := range d.Expectation.Claims {
		if !known[cl.Ref] {
			out = append(out, cl)
		}
	}
	return out
}

// holdLines names where a rule's answer came from, or says that nothing
// touched it.
func holdLines(h Hold) string {
	if !h.Reached {
		return "(not reached)"
	}
	parts := make([]string, 0, len(h.Rules))
	for _, rl := range h.Rules {
		if rl.Default {
			parts = append(parts, fmt.Sprintf("%s:%d (default)", rl.Module, rl.Row))
			continue
		}
		parts = append(parts, fmt.Sprintf("%s:%d", rl.Module, rl.Row))
	}
	if len(parts) == 0 {
		// Not reachable from Build, which sets Reached only with lines, and
		// said rather than left blank so the disagreement is visible.
		return "(reached, with no line recorded -- a defect to report)"
	}
	return strings.Join(parts, ", ")
}

// holdOrder lists the rules reached by this finding's evaluation first, then
// the rest, each group by name.
func holdOrder(m map[string]Hold) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		ri, rj := m[out[i]].Reached, m[out[j]].Reached
		if ri != rj {
			return ri
		}
		return out[i] < out[j]
	})
	return out
}

func orNone(s string) string {
	if s == "" {
		return "(no query named)"
	}
	return s
}

// reproductionLine is the pointer back to the raw trace: not a retrieval,
// because nothing retains a finding past the process, but the inputs that
// re-run it. LAST, for the reason given at the call site.
func reproductionLine(r Reproduction) string {
	s := fmt.Sprintf("%s %s at seal %d", r.Package, r.Case, r.Seal)
	if r.StatuteSHA != "" {
		s += fmt.Sprintf(", statute %s", r.StatuteSHA)
	}
	return s + fmt.Sprintf(", %d trace event(s)", r.Events)
}
