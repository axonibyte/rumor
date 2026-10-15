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

	Rule  string       // the fully qualified rule, e.g. data.rumor.statute.finding
	Fired []Definition // the definition(s) whose body succeeded, in row order

	Rules map[string]any // every rule the caller named, absent when undefined

	Trace       string // OPA's pretty trace, retained whole
	TraceEvents int
	Covered     int     // covered ranges in the statute
	Coverage    float64 // percent, as OPA reports it

	Input map[string]any // exactly what the statute was given
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
	var out Finding
	input, err := Input(l, c)
	if err != nil {
		return out, err
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
