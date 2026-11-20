// Package explain is a finding's derivation as data (#132, R11 d1): what the
// statute held and the statute lines that held it, the claims and tells the
// actor's knowledge rests on, the clock, and the observation that contradicted
// it -- as one Go value with a stable shape.
//
// IT IS A VIEW AND NOT A SECOND SOURCE. Everything here is derived from what a
// run already retains -- the finding, the sealed ledger and the check -- so
// nothing is stored twice and a derivation cannot disagree with the run it
// explains. Where a field cannot be derived, this package refuses by name
// rather than reporting an absence as a value (quill, coop r11/1, taken as
// settled at r11/5). Same rule as cairn's SPEC 10.3 in another project: a
// derived thing may answer nothing its source cannot.
//
// THREE INPUTS, BECAUSE A FINDING ALONE CANNOT SAY WHY. engine.Finding carries
// the statute's input, whose `known` is a list of REFS: what the actor knew and
// not how. The claim and tell that made each ref known are in the ledger, at a
// position, so the derivation joins them back. For a knowledge bug that join IS
// the question of whether the server or the rule is wrong.
//
// THREE PARTS AND NOT TWO HALVES, which is the one thing the first toy bug
// changed about the settled shape. r11/1 asked for the expectation's chain and
// the contradicting observation kept apart, and they are; what rendering
// examples/toy/cases/stale-count showed is that the statute's VERDICT is a
// third thing. Carried on the expectation it reads `expectation.value: "bug"`
// directly above `observation.value: 1`, two fields a reader is invited to
// compare and which are not comparable -- one is a word from the findings
// vocabulary, the other is what an API answered. So Verdict holds what the
// statute said, Expectation holds what the actor knew and how, Observation
// holds what was seen, and the distinction the phase exists to make -- a rule
// you disagree with, or facts you agree with -- survives the render.
//
// THE EXPECTED VALUE IS NOT A FIELD, because no rule name is the vocabulary's.
// `believed` is the convention of all five packages under examples/ and of the
// engine's own tests, and docs/vocabulary.md reserves the WORD expectation
// without reserving a rule to carry it. A field named for a convention would
// read as a contract and come up empty on the first statute that spells it
// otherwise, so Expectation.Holds carries every rule the caller named with the
// statute's value for it, and the reader keeps the naming.
//
// THE ORDER IS POSITION ORDER, and it is given rather than left to d2. Claims
// are in the order the statute was given them; each known fact's origins are in
// position order; the statute lines are in module and row order. d2 renders
// "shortest path first" and can read the hop depth off Via -- an actor's own
// claim is no hop, a tell is one -- which is d2's metric to choose and not this
// package's to impose.
//
// THE RAW TRACE IS NOT HERE, DELIBERATELY. OPA's pretty text is the evaluator's
// narration, not the reader's question, and embedding it would make d3's JSON
// schema hostage to OPA's formatting. Nothing retains a finding past the
// process either (measured: no json.Marshal of a Finding anywhere in the tree),
// so the pointer cannot be a retrieval -- it is a REPRODUCTION: the package,
// case, seal and statute digest that re-run the finding, plus the event count
// as a size. ibis, coop r11/3; rook's condition at r11/2.
package explain

import (
	"errors"
	"fmt"
	"sort"

	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
	"forge.axonibyte.com/axonibyte/rumor/internal/ledger"
)

// ErrNoTrace is returned for a finding the engine kept no trace for: there is
// nothing to derive an expectation from, and an empty explanation would read as
// "the statute said nothing" when the truth is "nobody looked".
var ErrNoTrace = errors.New("explain: the finding carries no evaluation trace")

// ErrUnsealed is returned for a ledger with no sealed prefix: the world at the
// finding's position is not knowable, so neither are its facts.
var ErrUnsealed = errors.New("explain: the ledger is unsealed")

// ErrNoInput is returned for a finding that kept no statute input. The input is
// the derivation's spine -- the observation, the claims and the knowledge the
// statute actually read -- and without it there is no chain to show.
var ErrNoInput = errors.New("explain: the finding carries no statute input")

// ErrDisagrees is returned where the finding's input and the run's own facts do
// not describe the same thing: a ref the actor knows that the sealed prefix
// cannot explain, or an input observation that is not the check the caller
// named.
//
// IT IS THE FOURTH REFUSAL AND NOT A RENDERED ROW, which is a decision r11/1
// did not have: those three refuse an ABSENCE, and this one refuses a
// CONTRADICTION. A derivation is only worth reading if it explains the finding
// the reader asked about, from facts that produced it; a disagreement between
// its sources is a defect to report, and printing it as a bare ref or as
// somebody else's check would hide the defect behind a plausible answer.
var ErrDisagrees = errors.New("explain: the finding's input and the run's facts disagree")

// Origin is one ledger entry that made a ref known: the actor's own claim, or a
// tell that passed it to them.
type Origin struct {
	Kind     string `json:"kind"`           // "claim" or "tell"
	Actor    string `json:"actor"`          // the claimant, or the teller
	To       string `json:"to,omitempty"`   // the recipient, tells only
	Position int    `json:"position"`       // where it sits in the ledger
	Type     string `json:"type,omitempty"` // the claim's type, claims only
}

// KnownFact is one flattened fact with the entries that made it known. Via is
// never empty in a derivation this package returns: a ref the actor knows for
// no reason in the sealed prefix is a defect in the flattener or the ledger,
// and it is refused through ErrDisagrees rather than rendered as a bare ref.
type KnownFact struct {
	Ref string   `json:"ref"`
	Via []Origin `json:"via"`
}

// RuleLine is one statute definition that produced the verdict.
//
// Default IS LOAD-BEARING AND NOT DECORATION: a finding off a default row means
// NO clause matched, which reads nothing like a clause firing, and it is the
// first thing a reader deciding "server or rule" needs to know. Measured over
// examples/toy: all ten of its bugs fire the default row and all fifteen of its
// consistent and explained cases fire a clause, so for the toy this flag is the
// whole difference between the two outcomes. Counted by
// TestToyBugsFireTheDefaultRow rather than by hand, which is why it says ten --
// my own eye said nine.
type RuleLine struct {
	Module  string `json:"module"`
	Row     int    `json:"row"`
	Default bool   `json:"default"`
}

// Claim is a claim the statute was given, as the statute saw it.
type Claim struct {
	Ref      string `json:"ref"`
	Actor    string `json:"actor"`
	Type     string `json:"type,omitempty"`
	Position int    `json:"position"`
	At       *int64 `json:"at,omitempty"` // the run's time when it was made, where the run has a clock
}

// Overlay is the engine rule that overrode the statute's own verdict, where one
// did: a disagreement held open as `pending`, or a pending past its bound
// imposed as a `bug` of kind `stale`.
//
// IT IS HERE AT d1 AND NOT HELD FOR d4, because a reader of an overridden
// finding who is shown only the statute's lines is reading the wrong rule
// entirely -- the verdict came from the engine, and the statute's own answer is
// beside it in StatuteValue. d4 adds what a PENDING finding needs on top of
// this; it does not need a second shape.
type Overlay struct {
	Rule   string `json:"rule"`
	Reason string `json:"reason,omitempty"`
	Value  any    `json:"value"`
	Kind   string `json:"kind,omitempty"`
	Detail string `json:"detail,omitempty"`

	// Until is the clock time the bound expires at, and 0 means a quiesce
	// bound -- the engine's own meaning for the field, kept rather than
	// re-spelled, because a second meaning for one number is how two readers
	// come to disagree about it.
	Until int64 `json:"until"`
}

// Verdict is what the statute held, and the statute lines that held it.
type Verdict struct {
	Value   any    `json:"value"`
	Defined bool   `json:"defined"`
	Says    string `json:"says"` // the engine's own rendering, so an absence reads as one
	Kind    string `json:"kind,omitempty"`

	Rules []RuleLine `json:"rules"`

	// StatuteValue is the statute's own verdict where the engine overrode it,
	// and Overridden says so in a word so the absence of an overlay is a
	// statement in the JSON rather than a missing key.
	StatuteValue any      `json:"statute_value,omitempty"`
	Overridden   bool     `json:"overridden"`
	Overlay      *Overlay `json:"overlay,omitempty"`
}

// Hold is one rule the caller named: the value the statute gave it, and the
// statute lines it fired while THIS finding was evaluated.
//
// Reached IS THE FIELD THAT MAKES AN ABSENCE A FACT. A rule's value comes from
// its own evaluation, so a rule can be defined -- and so carry a value here --
// without the finding's evaluation ever touching it. No lines then, and that is
// not a gap: a rule that did not participate cannot be the error. Said in a
// word rather than left to be inferred from an empty list, so the JSON states
// it (fable's ruling, coop r11/26).
type Hold struct {
	Value   any        `json:"value"`
	Rules   []RuleLine `json:"rules,omitempty"`
	Reached bool       `json:"reached"`
}

// Expectation is what the actor knew and how it came to know it, plus the
// statute's other answers at this position.
type Expectation struct {
	// Holds is every rule the caller named, with the value the statute gave
	// it and the lines that produced it -- the statute's intermediate
	// conclusions, under the names the statute uses. The finding itself is not
	// in here; it is the Verdict.
	//
	// THE LINES ARE WHY THE WRONG RULE CAN BE NAMED. A finding off a `default`
	// row is innocent by construction when the wrong rule is upstream of it
	// (ibis, coop r11/24): the default fired exactly right given a wrong
	// input, so the row a reader is handed is not the row to go and read. The
	// rule that IS wrong is in here, with its own line.
	Holds map[string]Hold `json:"holds"`

	Known  []KnownFact `json:"known"`
	Claims []Claim     `json:"claims"`
}

// Observation is what was seen, read from the input the statute was given
// rather than recomposed from the check, so the derivation shows what the
// statute actually read.
type Observation struct {
	Actor    string `json:"actor"`
	Of       string `json:"of,omitempty"`
	Query    string `json:"query,omitempty"`
	Source   string `json:"source,omitempty"`
	Role     string `json:"role,omitempty"`
	Value    any    `json:"value"`
	Position int    `json:"position"`
}

// Clock is the run's time at the seal, present only where the run has one.
//
// IT IS CARRIED WHENEVER THE RUN HAS A CLOCK and not only for a pending
// finding: d4 needs the bound and the clock together, and a second shape for
// pending would be two schemas for one question.
type Clock struct {
	Now int64 `json:"now"`
}

// Reproduction is how to get the raw trace back: not a retrieval, because
// nothing retains a finding past the process, but the inputs that re-run it.
type Reproduction struct {
	Package    string `json:"package"`
	Case       string `json:"case"`
	Seal       int    `json:"seal"`
	StatuteSHA string `json:"statute_sha,omitempty"`
	Events     int    `json:"events"` // a size, and never an identity
}

// Selector is what identifies the finding, which nothing in engine.Finding
// does: the check at its position. Anything looser and two findings in one case
// answer to one name, which makes `rumor explain` ambiguous on the first real
// statute.
type Selector struct {
	Actor    string `json:"actor"`
	Of       string `json:"of,omitempty"`
	Query    string `json:"query,omitempty"`
	Source   string `json:"source,omitempty"`
	Position int    `json:"position"`
}

// Derivation is one finding explained as data.
type Derivation struct {
	Selector     Selector     `json:"selector"`
	Verdict      Verdict      `json:"verdict"`
	Expectation  Expectation  `json:"expectation"`
	Observation  Observation  `json:"observation"`
	Clock        *Clock       `json:"clock,omitempty"`
	Reproduction Reproduction `json:"reproduction"`
}

// Run names the run the finding came from, which the finding itself does not
// carry: the caller knows its package, its case and the statute it evaluated.
type Run struct {
	Package    string
	Case       string
	StatuteSHA string
}

// Build derives the explanation of f, which was produced by checking c against
// the statute over l.
//
// It refuses by name rather than explaining an absence: a finding with no trace
// (ErrNoTrace), a ledger with no sealed prefix (ErrUnsealed), a finding with no
// statute input (ErrNoInput), and sources that contradict each other
// (ErrDisagrees).
//
// AN UNDEFINED FINDING IS EXPLAINED AND NOT REFUSED, which is the second thing
// the cut settled against r11/1: I listed "a trace with no fired definitions"
// there as a third emptiness to refuse. It is not one. A traced finding off no
// definition is the statute saying nothing where the reader expected a verdict
// -- the most useful thing a derivation can report, since it sends the reader
// to the statute's clauses and not to the server -- so it is stated, through
// the engine's own Says so that it cannot be read as a verdict of nil, with no
// rule lines because none held.
func Build(f engine.Finding, l *ledger.Ledger, c engine.Check, r Run) (*Derivation, error) {
	if f.TraceEvents == 0 || f.Trace == "" {
		return nil, fmt.Errorf("%w: %d event(s), %d byte(s); the engine retains it per finding, so this is a finding nobody traced rather than a statute that said nothing",
			ErrNoTrace, f.TraceEvents, len(f.Trace))
	}
	if l == nil {
		return nil, fmt.Errorf("%w: no ledger was given", ErrUnsealed)
	}
	seal := l.Seal()
	if seal <= 0 {
		return nil, fmt.Errorf("%w: the world at the finding's position is not knowable, so neither are its facts", ErrUnsealed)
	}
	if len(f.Input) == 0 {
		return nil, fmt.Errorf("%w: the statute's input is the derivation's spine", ErrNoInput)
	}

	obs, err := observationOf(f.Input, c)
	if err != nil {
		return nil, err
	}
	prefix := l.SealedPrefix()
	known, err := knownOf(f.Input, prefix, c.Actor, seal)
	if err != nil {
		return nil, err
	}

	d := &Derivation{
		Selector: Selector{
			Actor:    c.Actor,
			Of:       c.Of,
			Query:    c.Query,
			Source:   string(c.Source),
			Position: obs.Position,
		},
		Verdict: Verdict{
			Value:        f.Value,
			Defined:      f.Defined,
			Says:         f.Says(),
			Kind:         f.Kind,
			Rules:        rulesOf(f),
			StatuteValue: f.StatuteValue,
			Overridden:   f.Overlay != nil,
			Overlay:      overlayOf(f),
		},
		Expectation: Expectation{
			Holds:  holdsOf(f),
			Known:  known,
			Claims: claimsOf(prefix),
		},
		Observation: obs,
		Reproduction: Reproduction{
			Package:    r.Package,
			Case:       r.Case,
			Seal:       seal,
			StatuteSHA: r.StatuteSHA,
			Events:     f.TraceEvents,
		},
	}
	if now, ok := clockOf(f.Input); ok {
		d.Clock = &Clock{Now: now}
	}
	return d, nil
}

// rulesOf reads the statute lines off the finding's fired definitions. An
// undefined finding has none, and the empty slice is the honest answer: no
// clause held.
func rulesOf(f engine.Finding) []RuleLine { return linesOf(f.Fired) }

// linesOf renders statute definitions in module and row order. ONE READER for
// the finding's own lines and for every other rule's, so the two orders cannot
// drift apart.
func linesOf(defs []engine.Definition) []RuleLine {
	out := make([]RuleLine, 0, len(defs))
	for _, def := range defs {
		out = append(out, RuleLine{Module: def.Module, Row: def.Row, Default: def.Default})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Module != out[j].Module {
			return out[i].Module < out[j].Module
		}
		return out[i].Row < out[j].Row
	})
	return out
}

// overlayOf renders the engine rule that overrode the statute, and nil where
// none did.
func overlayOf(f engine.Finding) *Overlay {
	if f.Overlay == nil {
		return nil
	}
	return &Overlay{
		Rule:   f.Overlay.Rule,
		Reason: f.Overlay.Reason,
		Value:  f.Overlay.Value,
		Kind:   f.Overlay.Kind,
		Detail: f.Overlay.Detail,
		Until:  f.Overlay.Until,
	}
}

// holdsOf copies the statute's other answers, without the finding itself.
//
// IT IS A COPY AND NOT THE ENGINE'S MAP: a derivation that handed out
// f.Rules would let its reader mutate the finding, and a view that can change
// its source is not a view.
func holdsOf(f engine.Finding) map[string]Hold {
	out := make(map[string]Hold, len(f.Rules))
	for rule, v := range f.Rules {
		if rule == engine.FindingRule {
			continue
		}
		h := Hold{Value: v}
		if defs, ok := f.Definitions[rule]; ok && len(defs) > 0 {
			h.Rules, h.Reached = linesOf(defs), true
		}
		out[rule] = h
	}
	return out
}

// claimsOf renders the claims of the sealed prefix in position order, which is
// the order the statute was given them.
func claimsOf(prefix []ledger.Fact) []Claim {
	out := make([]Claim, 0, len(prefix))
	var t int64
	clocked := false
	for _, fact := range prefix {
		if fact.Kind == ledger.KindClock && fact.T != nil {
			t, clocked = *fact.T, true
		}
		if fact.Kind != ledger.KindClaim {
			continue
		}
		cl := Claim{Ref: fact.Ref, Actor: fact.Actor, Type: fact.Type, Position: fact.Position}
		if clocked {
			at := t
			cl.At = &at
		}
		out = append(out, cl)
	}
	return out
}

// observationOf reads the observation out of the input the statute was given
// and checks it against the check the caller named.
//
// THE CHECK IS NOT THE OBSERVATION'S SOURCE HERE, on purpose. The statute
// judged the input; a derivation recomposed from the check would agree with the
// caller even where the engine handed the statute something else, which is
// exactly the case a reader of `rumor explain` is trying to see. So the input
// is read, and a disagreement with the check is refused: a derivation of
// somebody else's check, rendered under this selector, is worse than no answer.
func observationOf(input map[string]any, c engine.Check) (Observation, error) {
	raw, ok := input["observation"].(map[string]any)
	if !ok {
		return Observation{}, fmt.Errorf("%w: it carries no `observation` map, which every check's input has", ErrNoInput)
	}
	pos, ok := asInt(raw["position"])
	if !ok {
		return Observation{}, fmt.Errorf("%w: the observation's position is %v, and a finding sits at a position or nowhere", ErrNoInput, raw["position"])
	}
	obs := Observation{
		Actor:    str(raw["actor"]),
		Of:       str(raw["of"]),
		Query:    str(raw["query"]),
		Source:   str(raw["source"]),
		Role:     str(raw["role"]),
		Value:    raw["value"],
		Position: pos,
	}
	for _, m := range []struct {
		field    string
		got, way string
	}{
		{"actor", obs.Actor, c.Actor},
		{"of", obs.Of, c.Of},
		{"query", obs.Query, c.Query},
		{"source", obs.Source, string(c.Source)},
		{"role", obs.Role, c.Role},
	} {
		if m.got != m.way {
			return Observation{}, fmt.Errorf("%w: the statute was given %s %q and the check names %q, so this finding is not the one the selector describes",
				ErrDisagrees, m.field, m.got, m.way)
		}
	}
	return obs, nil
}

// knownOf joins every ref the statute was told the actor knows back to the
// ledger entries that made it known: the actor's own claim at or before the
// seal, and any tell of that ref to the actor at or before it.
//
// A REF WITH NO ORIGIN IS A DEFECT AND NOT A ROW. The flattener builds `known`
// from exactly these entries, so a ref it reports that the prefix cannot
// explain means the two disagree -- and a derivation that printed the ref bare
// would hide the disagreement behind a fact with no reason.
func knownOf(input map[string]any, prefix []ledger.Fact, actor string, seal int) ([]KnownFact, error) {
	raw, ok := input["known"]
	if !ok {
		return nil, fmt.Errorf("%w: it carries no `known`, which every check's input has", ErrNoInput)
	}
	refs, err := refStrings(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: `known` is %v", ErrNoInput, err)
	}
	out := make([]KnownFact, 0, len(refs))
	for _, ref := range refs {
		kf := KnownFact{Ref: ref}
		for _, fact := range prefix {
			if fact.Position > seal {
				continue
			}
			switch fact.Kind {
			case ledger.KindClaim:
				if fact.Ref == ref && fact.Actor == actor {
					kf.Via = append(kf.Via, Origin{Kind: "claim", Actor: fact.Actor, Position: fact.Position, Type: fact.Type})
				}
			case ledger.KindTold:
				if fact.ClaimRef == ref && fact.To == actor {
					kf.Via = append(kf.Via, Origin{Kind: "tell", Actor: fact.Actor, To: fact.To, Position: fact.Position})
				}
			}
		}
		if len(kf.Via) == 0 {
			return nil, fmt.Errorf("%w: %s knows %q and the sealed prefix holds no claim of theirs and no tell to them for it: the flattener and the ledger disagree, which is a defect to report and not a fact to render",
				ErrDisagrees, actor, ref)
		}
		out = append(out, kf)
	}
	return out, nil
}

// clockOf reads the run's time out of the statute's input, where the run has
// one. The key and its shape are the vocabulary's, and absence is absence: a
// run with no clock control has no clock fact, so the key is never written.
func clockOf(input map[string]any) (int64, bool) {
	c, ok := input["clock"].(map[string]any)
	if !ok {
		return 0, false
	}
	return asInt64(c["now"])
}

// refStrings reads a []string out of the input's `known`, which arrives as
// []string from engine.Input and as []any from an input that made a round trip
// through JSON. Both are the same list, and a derivation that handled only the
// first would refuse every finding read back from a file.
func refStrings(v any) ([]string, error) {
	switch refs := v.(type) {
	case []string:
		return refs, nil
	case []any:
		out := make([]string, 0, len(refs))
		for i, r := range refs {
			s, ok := r.(string)
			if !ok {
				return nil, fmt.Errorf("a list whose element %d is %T and not a ref", i, r)
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("%T and not a list of refs", v)
	}
}

// str reads a string field, and answers "" for a key the input does not carry
// -- which is how engine.Input writes an absent role.
func str(v any) string {
	s, _ := v.(string)
	return s
}

// asInt and asInt64 take a number as the engine wrote it or as JSON read it
// back. A position is an int in process and a float64 after a round trip, and
// the two must not read as different worlds -- which the round-trip arm in
// explain_test.go drives rather than assumes.
//
// THERE IS NO json.Number CASE, and the absence is a decision: nothing in this
// tree decodes with UseNumber, so the case could not be reached and a case no
// mutation can kill is scaffolding, not a guard (engine.Input makes the same
// argument about a nil check it deleted).
func asInt(v any) (int, bool) {
	n, ok := asInt64(v)
	return int(n), ok
}

func asInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int64:
		return n, true
	case float64:
		if n != float64(int64(n)) {
			return 0, false
		}
		return int64(n), true
	default:
		return 0, false
	}
}
