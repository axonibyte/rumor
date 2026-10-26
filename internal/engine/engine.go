package engine

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/cover"
	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/open-policy-agent/opa/v1/topdown"

	"forge.axonibyte.com/axonibyte/rumor/internal/flatten"
	"forge.axonibyte.com/axonibyte/rumor/internal/ledger"
)

// StatutePackage is the Rego package every v1 statute declares. Duplicated
// from tests/statute on purpose: a reader that took the package name from the
// module it is about would agree with that module whatever it said.
const StatutePackage = "data.rumor.statute"

// FindingRule is the rule a finding is read from. docs/vocabulary.md fixes the
// four finding values; this is the rule that yields one of them.
const FindingRule = "finding"

// AttributionRule is the rule a finding's ATTRIBUTION is read from, and it is
// the third part of a finding's identity: "two findings are the same when the
// rule that fired, the finding's value, and its attribution
// (`rests_on_planted`) all agree" (docs/design.md). A `bug` resting on a
// planted claim and a `bug` resting on none are different findings sharing one
// name, so a reader handed the value alone cannot tell the engine catching a
// lie from the system under test being wrong.
//
// A STATUTE NEED NOT DECLARE IT, and then Rules carries no entry -- the
// attribution is UNKNOWN rather than false, which is a distinction a caller
// reporting it has to keep. `rumor attaint` is what requires it of a project
// carrying planted cases.
const AttributionRule = "rests_on_planted"

// moduleName is the filename the statute is parsed under. It appears in every
// Definition string below, so it is a constant rather than a caller's choice.
const moduleName = "statute.rego"

// Check is one observation offered for judgement: the engine's input.
//
// IT CARRIES NO POSITION. An observation is the check's input and not a fact
// in the sealed prefix (ratchet R2.1, ratified by fable breakout-r2/13), so
// the position it is judged at is derived from the seal and a caller cannot
// place its observation inside the data that judges it.
type Check struct {
	Actor  string        // the observing actor
	Query  string        // what was asked, e.g. "user.count"
	Of     string        // the claim type the query counts, e.g. "user.created"
	Source ledger.Source // api, ui or direct (R4 refuses a fourth)
	Value  any           // what came back
	Role   string        // the observer's role, where the project has roles

	// Quiesce is what a prober answered about the system having settled,
	// which is what a `quiesce` bound is measured against.
	//
	// IT IS AN INPUT AND NOT A MEASUREMENT HERE. R6.2's probes decide it --
	// queue depth, worker idle, a `direct` query, each with its own timeout
	// -- and hand the answer in; this package judges with it and never asks.
	// That split is fable's ruling 1 (coop breakout-r6/0): the engine takes
	// the answer, so R6.1 can land before the probes exist.
	Quiesce QuiesceState
}

// FindingQuiesceTimedOut is the FIFTH FINDING of docs/vocabulary.md, beside
// `consistent`, `explained`, `pending` and `bug`.
//
// IT IS A FINDING AND NOT A KIND, which is fable's D7 (coop breakout-r6/0):
// "the quiesce timeout's kind is in the vocabulary and in the run's output; it
// is neither `bug` nor a refusal, and the exit is 3". A probe that ran and gave
// up means the SYSTEM would not go quiet -- the project's declarations are
// right and its adapter answered -- so there is nothing for a refusal to tell
// anyone to fix.
//
// THE CONSTANT EXISTS SO THE WORD HAS ONE HOME. `bug` is spelled literally in
// four packages and `stale` in two, which is how a finding value drifts; a walk
// stopping on this one reads the constant rather than retyping the string.
const FindingQuiesceTimedOut = "quiesce_timeout"

// QuiesceState is a prober's answer about the system having settled.
//
// FOUR VALUES AND NOT A BOOL, which is fable's ruling 1(b) at coop r6/10 on
// ratchet's finding at r6/9. A bool cannot say "I asked and gave up": a probe
// that ran and TIMED OUT and a system that is still BUSY would hand in the same
// byte, two causes with one encoding, and only one of them is safe to read as
// "not quiet". R6.2 owns the probes and would have been stuck with that.
//
// THE ZERO VALUE IS ABSENT AND IT REFUSES. Nothing having asked is not the same
// as "not quiet": a caller with no prober wired gets a refusal that names the
// slot, never a finding held open and never a defect it has not earned. My
// first version made the zero value mean "not quiet" and reasoned that this was
// the conservative choice -- it was not, because it is indistinguishable from
// an answer.
type QuiesceState int

const (
	// QuiesceAbsent is the zero value: no prober answered. A refusal.
	QuiesceAbsent QuiesceState = iota
	// QuiesceQuiet: the system has settled, so a disagreement that remains is
	// past the bound.
	QuiesceQuiet
	// QuiesceBusy: the system is still working, so a disagreement is inside
	// the bound and held open.
	QuiesceBusy
	// QuiesceTimedOut: a prober ran and gave up. The finding kind for this is
	// R6.2's to define, so R6.1 refuses rather than inventing the word --
	// which is how `stale` itself was handled until this phase defined it.
	QuiesceTimedOut
)

// String names the state for a reader, INCLUDING ONE THIS BUILD DOES NOT KNOW.
//
// THE FALLBACK USED TO BE "absent" AND THAT IS A LIE WITH A CONSUMER (rook,
// coop rumor/969, completing ratchet's r6/19). Judge's refusal for an absent
// answer interpolates this method to name the real state -- "no quiesce answer
// was supplied (%s)" -- so a fifth value would have been refused under the
// absent clause's headline AND rendered as "absent" beside it: two layers
// agreeing on one lie, with the parenthesis that exists to name the state being
// the thing that could not. Fixing the switch alone moves the lie one layer
// down and leaves it readable, which is why this lands with it rather than
// after it.
//
// THE NUMBER IS THE ONLY HONEST ANSWER for a state with no name here. A
// sibling's name would be wrong in the same way "absent" was.
func (q QuiesceState) String() string {
	switch q {
	case QuiesceAbsent:
		return "absent"
	case QuiesceQuiet:
		return "quiet"
	case QuiesceBusy:
		return "busy"
	case QuiesceTimedOut:
		return "timed out"
	}
	return fmt.Sprintf("an unknown state (%d)", int(q))
}

// Definition names one rule definition in a statute: the module and the row it
// starts on, and whether it is the `default`.
//
// WHY A ROW AND NOT JUST A NAME. "A finding names the rule that produced it"
// (docs/vocabulary.md, Findings). A complete rule like `finding` has several
// definitions -- the default, plus one per clause -- and reporting the name
// alone would answer `data.rumor.statute.finding` for every finding in every
// case, which is a constant dressed as evidence. The row distinguishes the
// clause that actually answered, which is what a reader of `rumor explain`
// needs and what a case can assert.
type Definition struct {
	Module  string
	Row     int
	Default bool
}

// String renders a definition as module:row, with the default marked.
func (d Definition) String() string {
	if d.Default {
		return fmt.Sprintf("%s:%d (default)", d.Module, d.Row)
	}
	return fmt.Sprintf("%s:%d", d.Module, d.Row)
}

// Finding is the engine's verdict on one check.
//
// THE TRACE IS RETAINED, not summarised: issue 44 requires every finding to
// keep its evaluation trace for `rumor explain` (R10), and a count of events
// would not re-derive anything. TraceEvents is beside it so a caller can
// assert the trace is non-empty without parsing it.
type Finding struct {
	Value   any  // the finding, e.g. "bug"; nil when the rule is undefined
	Defined bool // false means the statute yielded no finding at all

	// Kind narrows the finding where the vocabulary gives it a kind: `stale`
	// is a `bug` whose kind says WHY it is one -- a pending past its bound --
	// and it is empty for a finding the statute produced on its own.
	//
	// IT IS A KIND AND NOT A FOURTH VALUE, which is fable's ruling 2 (coop
	// breakout-r6/0): v1 reserved `stale` under findings as "a `pending` past
	// its bound", so it exits 3 and lands on the FOUND line where a bug does.
	// A reader keying on Value still sees every bug; a reader that wants the
	// reason reads this.
	Kind string

	// THE STATUTE'S OWN VERDICT IS KEPT WHEN THE ENGINE OVERRIDES IT. Judge
	// may hold a finding open as `pending` (see pending.go); Value is then
	// "pending" and these three say so, so an overlay is never a quiet
	// substitution. All three are zero for an Evaluate that applied none.
	StatuteValue any      // what the statute produced, before any overlay
	Overlay      *Overlay // the engine rule that overrode it, when one did
	ReChecked    *Pending // the pending this observation re-checked, when it did

	Rule  string       // the fully qualified rule, e.g. data.rumor.statute.finding
	Fired []Definition // the definition(s) whose body succeeded, in row order

	Rules map[string]any // every rule the caller named, absent when undefined

	Trace       string // OPA's pretty trace, retained whole
	TraceEvents int
	Covered     int     // covered ranges in the statute
	Coverage    float64 // percent, as OPA reports it

	Input map[string]any // exactly what the statute was given
}

// Says renders the finding the way a reader should see it: the value when the
// statute defined one, and an explicit absence when it did not.
//
// IT EXISTS BECAUSE Defined IS EASY TO FORGET AND THE FORGETTING IS SILENT.
// A caller formatting Value directly prints `<nil>` for an undefined finding,
// which reads as a verdict OF nil rather than as no verdict at all -- and the
// distinction is this package's own: a finding is a rule that HOLDS, while
// absence is key-absence (docs/vocabulary.md). Measured when quill found it in
// internal/attaint (thread:r3/10) and internal/examples had it too
// (thread:rumor/623, /624): two production consumers of Finding, both ignoring
// the field, which is the shape where the third forgets as well. Each consumer
// fixing its own print leaves the next one to rediscover it, so the sentence
// lives here, beside the field it reads.
func (f Finding) Says() string {
	if !f.Defined {
		return "no finding"
	}
	// AND THE KIND, FOR THE REASON THE PARAGRAPH ABOVE GIVES. fable ruled the
	// renderers follow the comparison's basis (coop r6/15): a finding's
	// identity is rule, value, attribution AND kind, so a renderer that shows
	// the value alone makes a `stale` and a plain `bug` look identical -- in
	// the shrink trace, the examples line and the promoted comment, which is
	// the one artifact that gets checked in.
	//
	// IT LIVES HERE AND NOT IN EACH CALLER, which is this comment's own
	// argument turned on me: `run` composed " of kind %s" by hand at R6.1, the
	// third consumer printing a field itself after the first two had ignored
	// it. The wording is the FOUND line's so nothing changes there.
	if f.Kind != "" {
		return fmt.Sprintf("%v of kind %s", f.Value, f.Kind)
	}
	return fmt.Sprint(f.Value)
}

// Input builds the statute's input from a sealed ledger and one check.
//
// THIS IS THE ADAPTER, and it is the only place a ledger.Fact becomes a
// statute's view of a claim. Three translations happen here and each is a
// decision rather than plumbing:
//
//  1. ONLY THE SEALED PREFIX IS READ. An unsealed ledger is refused rather
//     than read whole: the vocabulary seals before any check runs, so a check
//     against an unsealed ledger is a sequencing error, and judging one would
//     report a verdict over facts that were still arriving.
//
//  2. THE OBSERVATION'S POSITION IS seal+1. The statutes bound their claim
//     sets with `c.position <= input.observation.position`; every fact in a
//     sealed prefix is at or before the seal, so seal+1 admits exactly the
//     prefix and nothing else. The bound is therefore STRUCTURAL -- it is the
//     seal -- which is what R2.1 made the ledger files express.
//
//  3. KNOWLEDGE COMES FROM internal/flatten AND NOT FROM A LOCAL LOOP. The
//     one-hop rule, the self-tell refusal and the ancestry extension point all
//     live there; a second implementation here would be a second rule.
func Input(l *ledger.Ledger, c Check) (map[string]any, error) {
	if l == nil {
		return nil, fmt.Errorf("engine: no ledger")
	}
	if c.Actor == "" {
		return nil, fmt.Errorf("engine: the check names no actor, and a finding is always some actor's")
	}
	seal := l.Seal()
	if seal <= 0 {
		return nil, fmt.Errorf("engine: the ledger is unsealed: docs/vocabulary.md seals a prefix " +
			"before any check runs, so this is a sequencing error and not an empty result")
	}

	fl, claims := prefixFacts(l)

	// THE SEAL IS PASSED EVEN THOUGH prefixFacts HAS ALREADY APPLIED IT, and
	// the redundancy is deliberate rather than overlooked: mutating this
	// argument upward changes no result for any fixture, which makes that
	// mutant EQUIVALENT and not surviving (docs/testing-methodology.md). The
	// premise it rests on -- that nothing handed to the flattener sits past the
	// seal -- is asserted directly in engine_test.go rather than inferred from
	// a passing suite, and the argument stays so that a caller who one day
	// hands in facts that were not prefix-filtered is still bounded.
	known, err := flatten.Known(fl, c.Actor, seal, nil)
	if err != nil {
		return nil, fmt.Errorf("engine: flattening %s's knowledge at the seal: %w", c.Actor, err)
	}
	// AN OBSERVER WHO KNOWS NOTHING ARRIVES AS [] AND NOT null, which the
	// cases carry and tests/engine compares against. Nothing normalises it
	// here because nothing needs to: flatten.Known builds its result with
	// make(), so an empty knowledge set is already a constructed empty slice.
	// A guard for nil was written here first and no mutation could kill it --
	// it was unreachable, not merely unexercised.

	obs := map[string]any{
		"actor":    c.Actor,
		"position": seal + 1,
		"of":       c.Of,
		"query":    c.Query,
		"source":   string(c.Source),
		"value":    c.Value,
	}
	if c.Role != "" {
		obs["role"] = c.Role
	}

	return map[string]any{
		"observation": obs,
		"known":       known,
		"claims":      claims,
	}, nil
}

// Evaluate judges one check against a sealed ledger under one statute.
//
// rules are the rule names the statute's rules.json asks for; every one is
// evaluated and an undefined rule is recorded as absent rather than as a zero
// value, because "the rule did not hold" and "it holds and its value is empty"
// are different answers.
func Evaluate(ctx context.Context, statute string, rules []string, l *ledger.Ledger, c Check) (Finding, error) {
	input, err := Input(l, c)
	if err != nil {
		return Finding{}, err
	}
	return EvaluateWith(ctx, statute, rules, input)
}

// EvaluateWith is Evaluate over an input the caller already built, which is
// what R5's walk needs: applicability asks a precondition's rules against
// PreconditionInput, where Evaluate's own Check would mean fabricating an
// observation a candidate step does not have.
//
// THE SPLIT IS A SPLIT AND NOT A SECOND EVALUATOR, which matters more than the
// convenience: two code paths that run statutes would diverge, and an
// explanation of a verdict the product did not give is worse than none (fable,
// thread:adr-regorus-tracing/4, ruling out OPA-beside-Regorus for exactly
// this). Evaluate is now this function plus Input, so a walk's applicability
// and a case's finding are computed by one evaluator over two inputs -- which
// is precisely the premise D14 exists to check.
func EvaluateWith(ctx context.Context, statute string, rules []string, input map[string]any) (Finding, error) {
	var out Finding
	if input == nil {
		return out, fmt.Errorf("engine: no input, and a statute over nothing has no verdict to give")
	}
	out.Input = input
	out.Rule = StatutePackage + "." + FindingRule

	module, err := ast.ParseModule(moduleName, statute)
	if err != nil {
		return out, fmt.Errorf("engine: the statute does not parse: %w", err)
	}
	// ADR 0001 AT THE LAYER THAT RUNS STATUTES. tests/statute enforces the ban
	// over every committed statute and mutant with two oracles; what it cannot
	// answer is whether the ENGINE lets one through, and it would: Evaluate
	// does not pass rego.EnablePrintStatements, so OPA erases print calls and a
	// statute full of them evaluates clean and silent. That is a check whose
	// silence means nothing, so the engine refuses the module instead of
	// running it quietly.
	if hits := printCalls(module); len(hits) > 0 {
		return out, fmt.Errorf("engine: the statute calls print() at %s: ADR 0001 forbids "+
			"diagnostic scaffolding in a statute, and this evaluator would erase the call rather "+
			"than report it", strings.Join(hits, ", "))
	}

	out.Rules = map[string]any{}
	for _, rule := range rules {
		v, defined, err := evalOne(ctx, statute, rule, input)
		if err != nil {
			return out, err
		}
		if defined {
			out.Rules[rule] = v
		}
	}

	// The finding is evaluated again with the instruments attached, so the
	// trace and the coverage are of the FINDING and not of whichever rule
	// happened to be last in rules.json.
	cov := cover.New()
	tracer := topdown.NewBufferTracer()
	v, defined, err := evalOne(ctx, statute, FindingRule, input,
		rego.QueryTracer(cov), rego.QueryTracer(tracer))
	if err != nil {
		return out, err
	}
	out.Value, out.Defined = v, defined

	var pretty bytes.Buffer
	topdown.PrettyTrace(&pretty, *tracer)
	out.Trace = pretty.String()
	out.TraceEvents = len(*tracer)
	out.Fired = firedDefinitions(*tracer, FindingRule)

	report := cov.Report(map[string]*ast.Module{moduleName: module})
	if f, ok := report.Files[moduleName]; ok {
		out.Covered = len(f.Covered)
		out.Coverage = f.Coverage
	}
	return out, nil
}

// evalOne returns one rule's value, and whether it was defined at all.
func evalOne(ctx context.Context, statute, rule string, input any,
	opts ...func(*rego.Rego)) (any, bool, error) {
	args := append([]func(*rego.Rego){
		rego.Query(StatutePackage + "." + rule),
		rego.Module(moduleName, statute),
		rego.Input(input),
	}, opts...)
	rs, err := rego.New(args...).Eval(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("engine: rule %s: %w", rule, err)
	}
	if len(rs) == 0 || len(rs[0].Expressions) == 0 {
		return nil, false, nil
	}
	return rs[0].Expressions[0].Value, true, nil
}

// firedDefinitions reads the definitions of `name` whose body succeeded off the
// trace. An Exit event carrying an *ast.Rule is OPA saying that rule's body
// held; the row comes from the rule's own location, so this is the statute's
// account of itself rather than this package's guess about clause order.
func firedDefinitions(events []*topdown.Event, name string) []Definition {
	seen := map[Definition]bool{}
	for _, ev := range events {
		if ev == nil || ev.Op != topdown.ExitOp {
			continue
		}
		r, ok := ev.Node.(*ast.Rule)
		if !ok || r.Head == nil {
			continue
		}
		if string(r.Head.Name) != name {
			continue
		}
		loc := r.Loc()
		if loc == nil {
			continue
		}
		seen[Definition{Module: moduleName, Row: loc.Row, Default: r.Default}] = true
	}
	out := make([]Definition, 0, len(seen))
	for d := range seen {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Row < out[j].Row })
	return out
}

// prefixFacts splits a sealed prefix into the two shapes the evaluation needs:
// the flattener's view, and the statute's. It is one walk because the two must
// agree about which facts are in scope -- two walks could disagree, and the
// claim list and the knowledge would then describe different prefixes.
//
// Unexported and separate so engine_test.go can assert the invariant the
// flattener's bound rests on, which Input alone would not expose.
func prefixFacts(l *ledger.Ledger) (flatten.Ledger, []map[string]any) {
	prefix := l.SealedPrefix()
	var fl flatten.Ledger
	claims := make([]map[string]any, 0, len(prefix))
	for _, f := range prefix {
		switch f.Kind {
		case ledger.KindClaim:
			m := map[string]any{
				"ref":      f.Ref,
				"actor":    f.Actor,
				"type":     f.Type,
				"position": f.Position,
			}
			// `planted` and `visible_to` are emitted only when present, which
			// is how the cases and the ledger dumps carry them. In Rego an
			// absent `planted` and a false one agree (`not c.planted` holds for
			// both); emitting the key unconditionally would still be correct
			// but would make the engine's input differ from the fixtures' for
			// no reason, and the harness compares them.
			if f.Planted {
				m["planted"] = true
			}
			if len(f.VisibleTo) > 0 {
				m["visible_to"] = f.VisibleTo
			}
			// HANDLES, so a rule can ask WHICH ENTITY a claim is about. fable
			// ruled this at coop r5/16 and design.md carried the fork until
			// now: the run-local NAMES and never ids, which is already how the
			// vocabulary compares findings, so projecting them follows the
			// discipline where projecting ids would breach it.
			//
			// WHY THIS CLOSES A REAL LIMIT: every package under examples/ is
			// single-subject, and a walk that creates two events writes joins
			// for both into ONE ledger. A statute with no identifying field but
			// `ref` counts across both and reads two findings as one -- which
			// R5.2 hit (ibis, coop r5.toy/18) rather than predicted.
			//
			// PRESENT ONLY WHEN NON-EMPTY, for the reason the two keys above
			// are: measured 2026-10-21 over the committed corpus, 19 of 62
			// claim objects in examples/*/cases carry `handles` and 43 do not,
			// and ledger.Fact's own tag is `handles,omitempty`. Emitting it
			// unconditionally would give 43 hand-written claims a key their
			// file does not have, for no gain -- a claim with no handle has
			// nothing to say about its subject, and `not c.handles` reads the
			// same as an absent one in Rego.
			if len(f.Handles) > 0 {
				m["handles"] = f.Handles
			}
			claims = append(claims, m)
			fl.Claims = append(fl.Claims, flatten.Claim{
				Ref:      f.Ref,
				Actor:    f.Actor,
				Position: f.Position,
			})
		case ledger.KindTold:
			fl.Tells = append(fl.Tells, flatten.Told{
				Actor:     f.Actor,
				Recipient: f.To,
				ClaimRef:  f.ClaimRef,
				Position:  f.Position,
			})
		}
	}
	return fl, claims
}

// printCalls lists the print() calls in a parsed statute, by row.
//
// The builtin's name comes from the library rather than from a literal here:
// ast.Print is the declaration the evaluator dispatches on, so reading its name
// cannot disagree with the evaluator about what a print call is. tests/statute
// asserts that the name is still the expected word.
func printCalls(mod *ast.Module) []string {
	var hits []string
	ast.WalkExprs(mod, func(e *ast.Expr) bool {
		if e.IsCall() {
			if op := e.Operator(); op != nil && op.String() == ast.Print.Name {
				row := 0
				if e.Location != nil {
					row = e.Location.Row
				}
				hits = append(hits, fmt.Sprintf("%s:%d", moduleName, row))
			}
		}
		return false
	})
	return hits
}

// Accepts reports whether Evaluate would run this statute at all, with the
// reason when it would not. It parses and applies the ADR 0001 check and
// nothing else, so a caller -- or a harness counting what it examined -- can
// ask the question without supplying a ledger and a check.
func Accepts(statute string) error {
	mod, err := ast.ParseModule(moduleName, statute)
	if err != nil {
		return fmt.Errorf("engine: the statute does not parse: %w", err)
	}
	if hits := printCalls(mod); len(hits) > 0 {
		return fmt.Errorf("engine: the statute calls print() at %s: ADR 0001 forbids "+
			"diagnostic scaffolding in a statute", strings.Join(hits, ", "))
	}
	return nil
}

// PreconditionInput is what a candidate STEP is shown, as against what an
// observation is shown by Input above. fable ruled the shape at coop r5/16
// after ibis measured the only preconditions in the tree (r5/3, /5).
//
// `observation` CARRIES ACTOR AND POSITION AND NOTHING ELSE. Every one of the
// toy's five preconditions reaches `held_claims`, which bounds on
// `input.observation.position`, and three read `.actor`; so a precondition is
// not an observation but it needs those two. `of`, `query`, `source`, `value`
// and `role` are ABSENT rather than empty, which is this package's existing
// discipline pointed at a new caller: a rule reaching for a field a candidate
// step cannot have goes UNDEFINED, where "" would be an answer. A role in
// particular is held per event, which an observation has and a step does not.
//
// WHY THE KEY IS STILL `observation` AND NOT `step`: five live rules read
// `input.observation.position` today, and a new key is a rewrite of every
// precondition in every project in exchange for nothing.
//
// MY OWN FIRST ANSWER WAS WRONG AND THE ORDER MATTERS. I argued for omitting
// the key entirely, reasoning from absent-versus-empty without reading what a
// precondition touches. ibis measured: with no `observation`, `held_claims` is
// empty and the five rules' defaults ANSWER -- may_join, may_leave and
// may_delete false, may_create_user and may_create_event true, nothing
// erroring. Omission is not undefined; it is silently wrong in both
// directions, which is the fault I thought omission prevented.
func PreconditionInput(l *ledger.Ledger, actor string) (map[string]any, error) {
	if l == nil {
		return nil, fmt.Errorf("engine: no ledger")
	}
	if actor == "" {
		return nil, fmt.Errorf("engine: no candidate actor, and applicability is always some actor's")
	}
	seal, err := sealForPrecondition(l)
	if err != nil {
		return nil, err
	}

	fl, claims := prefixFacts(l)
	known, err := knownAtSeal(fl, actor, seal)
	if err != nil {
		return nil, fmt.Errorf("engine: flattening %s's knowledge at the seal: %w", actor, err)
	}

	return map[string]any{
		"observation": map[string]any{
			"actor":    actor,
			"position": seal + 1,
		},
		"known":  known,
		"claims": honestOnly(claims),
	}, nil
}

// honestOnly drops the claims the RUN PLANTED, and it is the whole of fable's
// amendment to r5/16 at coop r5/97.
//
// THE TWO INPUTS ASK DIFFERENT QUESTIONS, which is why one strips and the other
// must not. A precondition asks what the system can be asked to do next, and a
// suborn never moved the system -- "the lie is the claim" (r5/58), so a planted
// claim is a description with no action under it and the wire knows nothing of
// it. A check asks whether an observer's belief agrees with what the store
// holds, and it MUST see the planted claim or the bug the planted-lie example
// exists to carry would be invisible. So `Input` keeps every claim and this one
// keeps the honest ones.
//
// THE ENGINE IS THE ONLY LAYER THAT CAN DO IT, which is the design's own
// sentence read the useful way: "the engine knows which claims it planted
// because it planted them, and the statute does not". The projection does carry
// `planted`, so a statute COULD write `not c.planted` into a precondition --
// and R5.4 measured what that costs. `has_user` satisfied by a planted
// `user.created` took seed 4242's step 7 to a session the toy answered 404 to;
// the one-line statute repair would have worked, and `made_event`,
// `already_joined` and `deleted` were three more rules of the toy's own with
// the same defect latent in them, none of them exercised by that seed. A trap
// with four instances in the first project to meet it is a trap in the engine,
// not a lesson for every project to learn once each. Stripping here makes it
// unreachable.
//
// `known` IS DELIBERATELY UNTOUCHED, on both paths: it is belief, and an actor
// fed a lie does know the claim. So `knows_of_subject` reads as it always did,
// and an actor told of a planted event is stopped one layer down by
// resolvability -- the handle a suborn minted resolves to no id (r5/58). A
// planted ref may therefore appear in `known` with no claim beside it in
// `claims`, which is exactly the intent: `held_claims` in every statute of the
// corpus intersects the two, so the claim simply is not held.
//
// A NEW SLICE AND NOT A FILTER IN PLACE: prefixFacts' result is also what
// Input and CanardSubjects read, and CanardSubjects' whole subject is the
// planted claims. Reusing the backing array would make this function's caller
// decide what another's sees.
func honestOnly(claims []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(claims))
	for _, c := range claims {
		if c["planted"] == true {
			continue
		}
		out = append(out, c)
	}
	return out
}

// sealForPrecondition is the seal a precondition is evaluated at, and the one
// place that decides whether a ledger can carry one.
//
// THE EMPTY WORLD IS A SEALED PREFIX, and my first version of this refused it.
// Both callers below had the same guard written out twice -- `seal <= 0` is a
// sequencing error -- which is true of a ledger that HOLDS facts with none
// sealed, and false of an empty one: internal/ledger accepts SealAt(0) on an
// empty ledger and reports Seal() == 0, because the empty prefix is a prefix.
// A RUN BEGINS THERE. The walk's first step is evaluated against nothing having
// happened, so refusing seal 0 made the scheduler unable to take its own first
// step, and `rumor run` found it in the layer underneath.
//
// It is the day's distinction one more time, in my own code: 0 because nothing
// has happened and 0 because nobody has sealed are different states, and a
// guard reading only the number cannot tell them apart. Len() separates them.
//
// ONE DEFINITION BECAUSE IT WAS TWO: the guard was copied into CanardSubjects
// when I wrote it, so the fix would otherwise have had to be made twice and the
// second site is the one that would have been missed.
func sealForPrecondition(l *ledger.Ledger) (int, error) {
	seal := l.Seal()
	if seal < 0 {
		return 0, fmt.Errorf("engine: the ledger reports a seal of %d, which is not a position", seal)
	}
	if seal == 0 && l.Len() > 0 {
		return 0, fmt.Errorf("engine: the ledger holds %d fact(s) and none of them is sealed: a "+
			"precondition is evaluated against a sealed prefix, so this is a sequencing error and "+
			"not an empty result", l.Len())
	}
	return seal, nil
}

// knownAtSeal is what an actor knows at the seal, with the empty prefix
// answered rather than asked about.
//
// WHY IT IS NOT A BARE flatten.Known: internal/flatten refuses position 0,
// since v1 numbers positions from 1 upward, and it is RIGHT to -- 0 is not a
// position. But the seal of an empty ledger IS 0, and what an actor knows
// before anything has happened is a question with an answer: nothing. So the
// engine declines to ask about a position that does not exist instead of
// asking flatten to soften a guard about v1's numbering.
//
// NOT QUOTED, DELIBERATELY. I first wrote flatten's error text here inside
// quotation marks, and tests/vocabulary refused it: the words are flatten's,
// not the vocabulary document's, and a quote mark after the token v1 reads as
// an attribution to that document. Paraphrase is the honest form for another
// package's reason -- the gate caught a false citation of mine for the second
// time today, which is twice that it earned its place.
//
// There is nothing to flatten in an empty prefix, so the empty answer is by
// construction and not a default standing in for a computation.
func knownAtSeal(fl flatten.Ledger, actor string, seal int) ([]string, error) {
	if actor == "" {
		return nil, fmt.Errorf("no actor given, and knowledge is always some actor's")
	}
	if seal == 0 {
		return nil, nil
	}
	return flatten.Known(fl, actor, seal, nil)
}

// KnownAt is the claim refs an actor holds at the seal.
//
// EXPORTED FOR THE WALK'S CANARD, which has to pick a receiver "among those who
// do not yet know it" (fable, coop r5/58) -- a question about somebody OTHER
// than the actor taking the step, which CanardSubjects cannot answer because it
// is about the teller. It goes through the same prefixFacts and knownAtSeal as
// every other knowledge question here, for the reason EvaluateWith exists: a
// second path computing what an actor knows would be free to disagree with the
// first.
func KnownAt(l *ledger.Ledger, actor string) ([]string, error) {
	if l == nil {
		return nil, fmt.Errorf("engine: no ledger")
	}
	seal, err := sealForPrecondition(l)
	if err != nil {
		return nil, err
	}
	fl, _ := prefixFacts(l)
	known, err := knownAtSeal(fl, actor, seal)
	if err != nil {
		return nil, fmt.Errorf("engine: flattening %s's knowledge at the seal: %w", actor, err)
	}
	return known, nil
}

// KnownHandlesAt is every handle NAMED by a claim `actor` knows at the seal.
//
// IT IS KnownAt's HANDLE-LEVEL SIBLING, and fable named KnownAt as the
// instrument for scoping a `handle` argument (coop r5/114). KnownAt answers in
// REFS; a `handle` argument is answered in handles, and the two are not the
// same set once a claim names more than its own ref. A walked `event.joined`
// carries `["member_1.join_event#1", "member_1.event#1"]` since r5/105, so an
// actor told that join knows OF the event -- by ref it would know only the
// join. Scoping on refs alone would make a told actor unable to act on what it
// was told, which is the opposite of the defect being closed.
//
// ONE PATH, not a second reading of the ledger: it goes through the same
// sealForPrecondition and knownAtSeal that KnownAt and PreconditionInput take,
// for the reason stated on CanardSubjects -- a second computation of what an
// actor knows would be free to disagree with the first.
//
// IT READS ledger.Fact.Handles AND NOT THE STATUTE PROJECTION, because the
// projection does not carry handles yet: fable ruled it at r5/16 and
// docs/design.md says plainly that "the engine does not do it yet". The ledger
// has carried the field all along, so this needs no projection change and makes
// no claim about one.
func KnownHandlesAt(l *ledger.Ledger, actor string) ([]string, error) {
	known, err := KnownAt(l, actor)
	if err != nil {
		return nil, err
	}
	is := make(map[string]bool, len(known))
	for _, ref := range known {
		is[ref] = true
	}
	seen := map[string]bool{}
	var out []string
	for _, f := range l.SealedPrefix() {
		if f.Kind != ledger.KindClaim || !is[f.Ref] {
			continue
		}
		for _, h := range f.Handles {
			if h == "" || seen[h] {
				continue
			}
			seen[h] = true
			out = append(out, h)
		}
	}
	sort.Strings(out)
	return out, nil
}

// TellSubjects answers the honest tell's precondition: the claims `actor` knows
// that the run did NOT plant.
//
// IT IS CanardSubjects' MIRROR AND DIFFERS IN TWO CLAUSES, both deliberate.
// fable ruled the kind at coop r5/114 as "the drawn actor passes a claim it
// holds (honest, not planted) to an actor who does not yet know it".
//
//  1. NOT PLANTED, where canard requires planted. That is the whole difference
//     between a lie travelling and a claim travelling, and it is why the two
//     are separately weighted: how often honest claims travel is a different
//     question about a crowd from how often lies do.
//
//  2. AUTHORSHIP IS NOT EXCLUDED, where canard excludes it. A canard is a tell
//     "by an actor that is not the claim's author" because passing on your own
//     lie is just lying. An honest tell by the author is the ordinary case and
//     the one the corpus carries by hand: honest-count's position 4 is alice
//     telling bob of alice's own event, which is what makes bob able to join
//     it. Excluding the author would leave the kind unable to do the one thing
//     the examples use it for.
//
// THE RECEIVER HALF IS NOT HERE, as it is not in CanardSubjects: this answers
// "has this actor anything to pass on", and whether anybody still needs telling
// is about the OTHER actors, which the pool cannot see. See tell() for that
// half and for the gap it leaves.
func TellSubjects(l *ledger.Ledger, actor string) ([]string, error) {
	known, err := KnownAt(l, actor)
	if err != nil {
		return nil, err
	}
	is := make(map[string]bool, len(known))
	for _, ref := range known {
		is[ref] = true
	}
	var out []string
	for _, f := range l.SealedPrefix() {
		if f.Kind != ledger.KindClaim || f.Planted || !is[f.Ref] {
			continue
		}
		out = append(out, f.Ref)
	}
	sort.Strings(out)
	return out, nil
}

// CanardSubjects answers canard's precondition, which docs/design.md states and
// docs/vocabulary.md's v1 row confirms is "a weight and a precondition over the
// kinds above, not a fact kind and not a tag". A canard is a tell that PASSES
// ON a planted claim, "by an actor who is not the claim's author, of a claim the
// run planted" -- so the kind is applicable to `actor` exactly when it knows
// such a claim, and inapplicable like any action whose precondition is false
// where none exists.
//
// IT RETURNS THE REFS AND NOT A BOOLEAN because the engine records the hop: R10
// says who PASSED the lie and not only who made it, which needs the claim that
// travelled. A caller wanting applicability alone reads len() == 0.
//
// AND IT GOES THROUGH prefixFacts AND flatten.Known, the same two steps
// PreconditionInput takes, for the reason EvaluateWith exists one layer up: a
// second path computing what an actor knows would be free to disagree with the
// first, and a walk and a check disagreeing about one world is the fault D14
// exists to catch.
//
// The order is the sealed prefix's, which is position order. It is not sorted:
// a slice already iterates deterministically, so D1's reason -- Go randomizes
// MAP iteration on purpose -- does not reach here, and position order carries
// something ref order would throw away.
func CanardSubjects(l *ledger.Ledger, actor string) ([]string, error) {
	if l == nil {
		return nil, fmt.Errorf("engine: no ledger")
	}
	if actor == "" {
		return nil, fmt.Errorf("engine: no candidate actor, and a canard is always some actor's")
	}
	seal, err := sealForPrecondition(l)
	if err != nil {
		return nil, err
	}

	fl, claims := prefixFacts(l)
	known, err := knownAtSeal(fl, actor, seal)
	if err != nil {
		return nil, fmt.Errorf("engine: flattening %s's knowledge at the seal: %w", actor, err)
	}
	holds := make(map[string]bool, len(known))
	for _, ref := range known {
		holds[ref] = true
	}

	var out []string
	for _, c := range claims {
		if c["planted"] != true {
			continue
		}
		if author, _ := c["actor"].(string); author == actor {
			continue
		}
		ref, _ := c["ref"].(string)
		if ref == "" || !holds[ref] {
			continue
		}
		out = append(out, ref)
	}
	return out, nil
}
