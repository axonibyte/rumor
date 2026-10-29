// Package attaint is the self-test: it judges the STATUTES rather than the
// system under test (R3, issues 52 and 53).
//
// WHAT IT IS FOR. `rumor check` asks whether each example's finding matches the
// verdict its author wrote down. That question is answered by the same statutes
// it is checking, so a statute that had quietly stopped deciding anything would
// still agree with a case file written to match it. attaint asks the other
// question: are these statutes load-bearing at all? The epic's done-when is the
// shape of the answer -- deleting a rule, weakening a rule, or stripping a
// planted tag must make attaint FAIL on the R1 examples.
//
// THREE ARMS, EACH WITH ITS OWN REFUSAL TEXT, because the gate reads the text
// and not the exit status (fable, breakout-r3/0). A single boolean would make a
// drill that trips the wrong arm look like a drill that passed.
//
//  1. CORROBORATION. docs/vocabulary.md `planted`, first use: "`rumor attaint`
//     requires every planted lie in a fixture to produce `bug`". So every case
//     whose sealed prefix carries a planted claim must find `bug`, and that bug
//     must REST ON the lie -- the corroboration rule must hold. A `bug` the
//     statute reached by some other route is the engine reporting a defect, not
//     THE QUOTE IS SHORTER THAN IT WAS, and deliberately. The 2026-10-19
//     amendment (fable breakout-r5/1) inserted a sentence between "the control
//     passing" and "`rumor attaint` requires", so the old quotation spanned
//     text that is no longer contiguous -- tests/vocabulary caught it as a
//     quote the document no longer says. What the amendment scopes is a WALK,
//     which counts what it did not reach; attaint over FIXTURES is unchanged,
//     a fixture's author having arranged the observation, and that is the
//     clause this package rests on.
//     the control passing, which is exactly what examples/planted-lie's
//     `stripped-tag` case says in its own `why`.
//
//  2. THE MUTATION PASS. Every rule a package declares must be killed by at
//     least one of its mutants. This is the DUAL of what tests/engine asserts:
//     that harness asks "is each mutant caught", which is satisfied by a mutant
//     set that batters one rule and never touches the rest. Here the question
//     is per RULE, and a rule no mutant moves is NAMED rather than passed over
//     -- an unmutated rule is a rule nothing proves is load-bearing.
//
//  3. VACUITY. Nothing examined is a refusal and never a pass. This is the arm
//     the other two rest on: "every planted lie produces bug" is satisfied for
//     free by a fixture set with no planted lies in it, which is why stripping
//     the tags has to be caught HERE rather than by arm 1 passing quietly.
//
// IT DOES NOT SHARE A LOADER WITH internal/examples, for the reason that
// package states about tests/engine: a walk the subject chooses is a walk the
// subject can narrow.
package attaint

import (
	"context"
	"encoding/json"
	"fmt"
	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/decl"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/open-policy-agent/opa/v1/rego"

	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
	"forge.axonibyte.com/axonibyte/rumor/internal/ledger"
)

// CorroborationRule is the rule that says a finding rests on a planted claim.
//
// A CONSTANT RATHER THAN A PER-PACKAGE SETTING, on engine.FindingRule's
// reasoning: a corroboration rule each package named for itself would let a
// package opt out of the control by renaming it. A package takes part in arm 1
// exactly when its rules.json declares this rule, and Report.Corroborating says
// how many did -- so "no package took part" is a visible number and not a
// silence.
const CorroborationRule = "rests_on_planted"

// Clause is one verdict attaint can reach. The gate reads these strings.
type Clause string

const (
	ClauseNoPackages Clause = "no-package-examined"
	ClauseNoCases    Clause = "no-case-examined"
	ClauseNoMutants  Clause = "no-mutant-examined"
	ClauseNoRules    Clause = "no-rule-declared"
	// ClauseCheckerNeverFired is the control ARMED AND NEVER EVALUATED: a case
	// declares itself a control, a planted claim IS in the sealed prefix, and
	// the observation never reaches it. fable breakout-r3/2 separates this
	// from the two it is easily confused with -- the control EVALUATED and not
	// passing is ClauseControlDidNotPass, and no control declared anywhere is
	// ClauseNoControlDeclared, which is a vacuity rather than a failed check.
	ClauseCheckerNeverFired  Clause = "checker never fired"
	ClauseNoControlDeclared  Clause = "no-case-declares-a-corroboration-control"
	ClauseLieNotFound        Clause = "planted-lie-did-not-produce-bug"
	ClauseControlDidNotPass  Clause = "bug-does-not-rest-on-the-planted-claim"
	ClauseRuleUnmutated      Clause = "rule-killed-by-no-mutant"
	ClauseFindingUnreachable Clause = "finding-the-package-can-no-longer-produce"
	ClauseMutantDead         Clause = "mutant-the-statute-has-absorbed"
	// ClauseStripDoesNotClear is the strip's SECOND direction, which
	// docs/vocabulary.md states and nothing checked until R3.2: with the
	// planted claims REMOVED ALTOGETHER the run must come back `consistent`.
	// Removing the TAG must leave the same bug (the first direction, driven by
	// the committed stripped-tag case and drill d3); removing the CLAIMS must
	// remove it. A bug that survives the lie's removal was never the lie.
	ClauseStripDoesNotClear Clause = "the-bug-survives-removing-the-planted-claims"
)

// Report is what one pass measured. Every count is reported even on a refusal,
// because "0 refusals" over nothing examined is the failure this tool exists to
// make impossible to print.
type Report struct {
	Packages      int
	Cases         int
	Rules         int
	Mutants       int
	Corroborating int // packages declaring CorroborationRule
	PlantedCases  int // cases whose sealed prefix carries a planted claim
	Controls      int // cases declaring themselves corroboration controls
	Evaluated     int // of those, controls the observation actually reached
	Corroborated  int // of those, bug AND resting on the lie
	Cleared       int // planted cases whose bug disappears when the lie is removed
	// PlantedNotControl counts cases carrying a planted claim that do NOT
	// declare themselves corroboration controls, so the strip clause is not
	// applied to them. Reported because the scoping is a narrowing: a reader
	// has to be able to see how many cases it excluded.
	PlantedNotControl int
	KilledRules       int      // rules killed by at least one mutant
	Findings          int      // distinct finding values the packages reach
	DeadMutants       []string // "pkg/mutant" indistinguishable from its own statute
	Unmutated         []string // "pkg/rule" that no mutant moved -- always named

	// LedgerInert is "pkg/mutant" for a mutant that moves NOTHING from the
	// ledger and something from the cases. That is R2.3's equivalence set,
	// MEASURED rather than read: tests/engine carries the same three as a
	// named list with a written reason, so discovering them independently
	// makes this a second oracle on that list instead of a consumer of it.
	//
	// A MUTANT IN HERE IS ALIVE AND EXEMPTED FROM NOTHING. rook asked at
	// rumor/600 whether this arm exempts those three the way tests/engine's
	// branch does, and carries the second half of the proof if so. It does
	// not exempt them: there is no declaration to grant, they move rules from
	// the cases, and that is the whole reason both paths are measured. The
	// list is reported so a FOURTH going ledger-inert is visible here too.
	LedgerInert []string
	Refusals    []Refusal
}

// Refusal is one failure with the clause that produced it.
type Refusal struct {
	Clause Clause
	Detail string
}

func (r Refusal) String() string { return fmt.Sprintf("REFUSED %s: %s", r.Clause, r.Detail) }

// OK reports whether the pass found nothing to refuse.
func (r Report) OK() bool { return len(r.Refusals) == 0 }

type pkg struct {
	name    string
	dir     string
	statute string
	rules   []string
	cases   []exCase
	mutants []string
}

type exCase struct {
	name    string
	expect  map[string]any
	check   engine.Check
	sealed  *ledger.Ledger
	planted bool
	// caseInput is the case's own `input` block, the BARE path tests/statute
	// drives. Arm 2 reads both paths, so a mutant whose clause the sealed
	// prefix has made unreachable still counts for the rule it attacks.
	caseInput map[string]any
}

type caseDoc struct {
	Expect map[string]any `json:"expect"`
	Input  struct {
		Observation struct {
			Actor  string `json:"actor"`
			Of     string `json:"of"`
			Query  string `json:"query"`
			Source string `json:"source"`
			Value  any    `json:"value"`
			Role   string `json:"role"`
		} `json:"observation"`
	} `json:"input"`
}

type sealDoc struct {
	Seal int `json:"seal"`
}

// Run walks dir and returns what it measured. An error is a tree it could not
// read; a REFUSAL is a verdict and rides in the Report, because the caller's
// exit code has to tell "the statutes are not load-bearing" apart from "the
// directory is missing".
func Run(ctx context.Context, dir string, out io.Writer) (Report, error) {
	var rep Report
	pkgs, err := load(dir)
	if err != nil {
		return rep, err
	}
	rep.Packages = len(pkgs)
	if rep.Packages == 0 {
		rep.Refusals = append(rep.Refusals, Refusal{ClauseNoPackages,
			fmt.Sprintf("%s holds no package with a statute.rego, so nothing was judged", dir)})
		return rep, nil
	}

	for _, p := range pkgs {
		rep.Cases += len(p.cases)
		rep.Rules += len(p.rules)
		rep.Mutants += len(p.mutants)
		if declares(p.rules, CorroborationRule) {
			rep.Corroborating++
		}
	}
	// THE THREE COUNTING GUARDS, before any arm runs. Each one is a way the
	// arms below would otherwise report a clean pass over nothing.
	if rep.Cases == 0 {
		rep.Refusals = append(rep.Refusals, Refusal{ClauseNoCases,
			"every package has a statute and no cases, so no rule was exercised"})
	}
	if rep.Rules == 0 {
		rep.Refusals = append(rep.Refusals, Refusal{ClauseNoRules,
			"no package declares a rule in rules.json, so there is nothing to kill"})
	}
	if rep.Mutants == 0 {
		rep.Refusals = append(rep.Refusals, Refusal{ClauseNoMutants,
			"no package carries a mutant, so no rule can be shown load-bearing"})
	}
	if !rep.OK() {
		return rep, nil
	}

	for _, p := range pkgs {
		if err := corroborate(ctx, p, &rep, out); err != nil {
			return rep, err
		}
		if err := mutate(ctx, p, &rep, out); err != nil {
			return rep, err
		}
		if err := reachable(ctx, p, &rep, out); err != nil {
			return rep, err
		}
		if err := stripClears(ctx, p, &rep, out); err != nil {
			return rep, err
		}
	}

	// ARM 1's VACUITY GUARD, and it is the whole reason the strip is caught.
	// "Every planted lie produces bug" is TRUE of a fixture set holding no
	// planted lie. Without this line, removing the tags turns arm 1 from a
	// control into a tautology and attaint goes green on a tree that proves
	// nothing.
	if rep.Controls == 0 {
		rep.Refusals = append(rep.Refusals, Refusal{ClauseNoControlDeclared,
			fmt.Sprintf("no case in any of the %d package(s) declares %s=true, so the corroboration "+
				"control was never exercised; %d case(s) carry a planted claim and %d package(s) declare the rule",
				rep.Packages, CorroborationRule, rep.PlantedCases, rep.Corroborating)})
	}

	sort.Strings(rep.Unmutated)
	sort.Strings(rep.DeadMutants)
	sort.Strings(rep.LedgerInert)
	return rep, nil
}

// corroborate runs arm 1 over one package.
//
// TWO QUESTIONS, NOT ONE, because the vocabulary asks two and they fail apart:
//
//   - EVERY PLANTED LIE PRODUCES bug. Literally docs/vocabulary.md `planted`.
//     It applies to any case whose sealed prefix carries a planted claim, in
//     any package, whether or not that package models attribution.
//
//   - A CASE THAT DECLARES ITSELF A CONTROL MUST HAVE ITS CONTROL PASS. A case
//     expecting `rests_on_planted: true` is asserting "this bug is the engine
//     catching the lie". That is the claim the strip breaks, and it is read
//     from the case's OWN expect block rather than from a package the tool
//     picked -- so a tree where the tags have been stripped cannot satisfy it
//     by having nothing to attribute.
//
// Attribution is NOT demanded of a package that never claimed it. role-
// visibility and transitive-count carry planted claims and model no
// corroboration rule; requiring one of them would be this tool inventing a
// clause the vocabulary does not have.
func corroborate(ctx context.Context, p pkg, rep *Report, out io.Writer) error {
	for _, c := range p.cases {
		wantsControl := false
		if v, ok := c.expect[CorroborationRule]; ok && sameJSON(v, true) {
			wantsControl = true
		}
		if !c.planted && !wantsControl {
			continue
		}
		if c.planted {
			rep.PlantedCases++
		}
		if wantsControl {
			rep.Controls++
		}

		f, err := engine.Evaluate(ctx, p.statute, p.rules, c.sealed, c.check)
		if err != nil {
			return fmt.Errorf("%s/%s: %w", p.name, c.name, err)
		}
		if c.planted && !sameJSON(f.Value, "bug") {
			rep.Refusals = append(rep.Refusals, Refusal{ClauseLieNotFound,
				fmt.Sprintf("%s/%s carries a planted claim in its sealed prefix and found %v, not bug: "+
					"a lie the statute does not notice", p.name, c.name, f.Value)})
			continue
		}
		if !wantsControl {
			continue
		}
		// THE STRIP IS CAUGHT HERE. The case says this bug rests on a lie; if
		// the ledger carries no lie, the claim cannot be true however the
		// statute evaluates, and saying so names the tree rather than the rule.
		if !c.planted {
			rep.Refusals = append(rep.Refusals, Refusal{ClauseControlDidNotPass,
				fmt.Sprintf("%s/%s expects %s=true but its sealed prefix carries no planted claim, "+
					"so there is no lie for the finding to rest on", p.name, c.name, CorroborationRule)})
			continue
		}
		// ARMED BUT NEVER FIRED. The lie is in the prefix and the OBSERVATION
		// never reaches it -- the observer does not hold it, or it is not of
		// the type being counted -- so the control was never evaluated at all.
		// That is a different event from a control that ran and did not pass,
		// and reporting them with one clause would make a fixture nobody
		// observes indistinguishable from a statute that stopped attributing.
		//
		// REACHABILITY IS READ FROM WHAT THE ENGINE HANDED THE STATUTE, not
		// recomputed here: Finding.Input is exactly the document the rules saw,
		// so the knowledge rule lives in internal/flatten and has no second
		// implementation in this file.
		reached, err := reachesAPlantedClaim(f.Input)
		if err != nil {
			return fmt.Errorf("%s/%s: %w", p.name, c.name, err)
		}
		if !reached {
			rep.Refusals = append(rep.Refusals, Refusal{ClauseCheckerNeverFired,
				fmt.Sprintf("%s/%s declares itself a corroboration control and its sealed prefix carries a "+
					"planted claim, but the observation reaches none: the observer does not hold a planted "+
					"claim of the observed type, so the control was armed and never evaluated",
					p.name, c.name)})
			continue
		}
		rep.Evaluated++
		if !sameJSON(f.Value, "bug") {
			rep.Refusals = append(rep.Refusals, Refusal{ClauseLieNotFound,
				fmt.Sprintf("%s/%s declares itself a corroboration control and found %v, not bug",
					p.name, c.name, f.Value)})
			continue
		}
		v, ok := f.Rules[CorroborationRule]
		if !ok || !sameJSON(v, true) {
			rep.Refusals = append(rep.Refusals, Refusal{ClauseControlDidNotPass,
				fmt.Sprintf("%s/%s found bug with %s=%v: the finding does not rest on the planted claim, "+
					"so this is a bug the engine reports rather than the control passing",
					p.name, c.name, CorroborationRule, v)})
			continue
		}
		rep.Corroborated++
		// A FAILED WRITE IS A FAILED PASS, not a cosmetic loss: these lines ARE
		// the evidence the verdict rests on, which is internal/examples.go:123's
		// reasoning applied here. Caught by golangci-lint on the runner (task
		// 231) and not locally -- errcheck runs nowhere else, rook rumor/509.
		if _, werr := fmt.Fprintf(out, "%-16s %-26s planted lie corroborated   bug rests on the lie\n",
			p.name, c.name); werr != nil {
			return fmt.Errorf("writing the corroboration line for %s/%s: %w", p.name, c.name, werr)
		}
	}
	return nil
}

// mutate runs arm 2 over one package: which rules does some mutant move?
//
// BOTH PATHS COUNT, and that is not a weakening. R2.3 established that three
// mutants are EQUIVALENT on the ledger path because the sealed prefix leaves
// their position guard nothing to exclude (tests/engine, declared by name).
// Measuring only the ledger would therefore report the rules those mutants
// attack as unmutated -- blaming the mutant set for a property of the bound.
// A rule is load-bearing if SOME mutant moves it on SOME path; the path is
// named in the output so a rule killed only from the cases is visible as such.
func mutate(ctx context.Context, p pkg, rep *Report, out io.Writer) error {
	cleanLedger := make([]map[string]any, len(p.cases))
	cleanCase := make([]map[string]any, len(p.cases))
	for i, c := range p.cases {
		f, err := engine.Evaluate(ctx, p.statute, p.rules, c.sealed, c.check)
		if err != nil {
			return fmt.Errorf("%s/%s: the clean statute did not evaluate from the ledger: %w", p.name, c.name, err)
		}
		cleanLedger[i] = f.Rules
		g, err := evalCasePath(ctx, p.statute, p.rules, c.caseInput)
		if err != nil {
			return fmt.Errorf("%s/%s: the clean statute did not evaluate from the case: %w", p.name, c.name, err)
		}
		cleanCase[i] = g
	}

	// EVERY KILLER PER RULE AND EVERY RULE PER MUTANT, which is fable's ruling
	// at coop rumor/1057 on rook's /1055. This pass used to keep the FIRST
	// killer of each rule and nothing else, over a relation that is
	// many-to-many: m1-ignores-position moves both `believed` and `finding`, so
	// m2-finding-ignores-delete -- which moves `finding` too -- appeared
	// NOWHERE in the output, not as a kill and not as alive. rook drove it:
	// hold m1 aside and `finding` goes to m2, `believed` to m5. A mutant
	// shadowed that way prints exactly what a no-op prints, so editing m2 to do
	// nothing would leave the output identical and nothing in the suite could
	// see it regress. Both directions are recorded now, so a mutant that stops
	// attacking loses a line.
	killers := map[string]map[string]string{}  // rule -> mutant -> path
	movesRules := map[string]map[string]bool{} // mutant -> rules it moves
	movedBy := map[string]bool{}
	movedLedger := map[string]bool{}
	movedCases := map[string]bool{}
	for _, mp := range p.mutants {
		src, err := os.ReadFile(mp)
		if err != nil {
			return err
		}
		mid := strings.TrimSuffix(filepath.Base(mp), ".rego")
		for i, c := range p.cases {
			f, err := engine.Evaluate(ctx, string(src), p.rules, c.sealed, c.check)
			if err != nil {
				// A mutant that will not evaluate is red for the wrong reason
				// and proves nothing about the rule it was cut to attack.
				return fmt.Errorf("%s/%s on %s: the mutant did not evaluate from the ledger, "+
					"which is not the same as moving a rule: %w", p.name, mid, c.name, err)
			}
			g, err := evalCasePath(ctx, string(src), p.rules, c.caseInput)
			if err != nil {
				return fmt.Errorf("%s/%s on %s: the mutant did not evaluate from the case: %w",
					p.name, mid, c.name, err)
			}
			for _, rule := range p.rules {
				// MEASURED FOR EVERY RULE AND EVERY MUTANT, with no short
				// circuit once a rule is already killed: the per-MUTANT
				// question below needs this mutant's own movement, and
				// skipping the comparison would make a mutant look dead
				// because some earlier mutant got to the rule first.
				onLedger := moved(cleanLedger[i], f.Rules, rule)
				onCases := moved(cleanCase[i], g, rule)
				if onLedger {
					movedLedger[mid] = true
				}
				if onCases {
					movedCases[mid] = true
				}
				if onLedger || onCases {
					movedBy[mid] = true
				}
				if !onLedger && !onCases {
					continue
				}
				if movesRules[mid] == nil {
					movesRules[mid] = map[string]bool{}
				}
				movesRules[mid][rule] = true
				if killers[rule] == nil {
					killers[rule] = map[string]string{}
				}
				// THE LEDGER PATH WINS THE LABEL where a mutant moves the rule
				// on both, because the ledger is the path the product takes: a
				// killer reported "from the cases" when it also moves the rule
				// from a sealed prefix would send a reader to the weaker
				// evidence. The pair is recorded once per (rule, mutant)
				// however many cases move it, the question being whether this
				// mutant attacks this rule at all.
				if killers[rule][mid] != "ledger" {
					if onLedger {
						killers[rule][mid] = "ledger"
					} else {
						killers[rule][mid] = "cases"
					}
				}
			}
		}
	}

	for _, rule := range p.rules {
		if ks, ok := killers[rule]; ok && len(ks) > 0 {
			rep.KilledRules++
			mids := make([]string, 0, len(ks))
			for mid := range ks {
				mids = append(mids, mid)
			}
			// SORTED, so the line is a fact about the package and not about
			// map order: two runs over one tree must print the same bytes.
			sort.Strings(mids)
			named := make([]string, 0, len(mids))
			for _, mid := range mids {
				named = append(named, fmt.Sprintf("%s (from the %s)", mid, ks[mid]))
			}
			if _, werr := fmt.Fprintf(out, "%-16s %-26s killed by %s\n",
				p.name, rule, strings.Join(named, ", ")); werr != nil {
				return fmt.Errorf("writing the kill line for %s/%s: %w", p.name, rule, werr)
			}
			continue
		}
		// NAMED, never counted silently. issue 52: "every rule killed by at
		// least one mutant, unmutated rules named".
		rep.Unmutated = append(rep.Unmutated, p.name+"/"+rule)
		rep.Refusals = append(rep.Refusals, Refusal{ClauseRuleUnmutated,
			fmt.Sprintf("%s/%s is moved by none of the %d mutant(s) over %d case(s) on either path, "+
				"so nothing shows it is load-bearing", p.name, rule, len(p.mutants), len(p.cases))})
	}

	// THE MUTATION PASS'S OTHER HALF, and it is what catches a WEAKENED
	// statute. The loop above asks whether every rule is attacked. This asks
	// whether every mutant still attacks: a mutant that moves no rule on any
	// case on either path is indistinguishable from the statute it was cut
	// from, which means the STATUTE has drifted into the mutant rather than
	// the mutant having become harmless. Loosening planted-lie's honest count
	// makes the statute identical to m5-honest-counts-planted, and nothing
	// else in this tool would notice -- the finding is still bug, the control
	// still passes, every rule is still moved by some other mutant.
	//
	// It is NOT the declared-equivalence case from tests/engine: those three
	// are equivalent on the LEDGER path only and still move their rules from
	// the cases, so they move something and are not dead here.
	for _, mp := range p.mutants {
		mid := strings.TrimSuffix(filepath.Base(mp), ".rego")
		if movedBy[mid] {
			// WHAT THIS MUTANT MOVES, BY NAME, which is the half that was
			// missing. A mutant whose rules were all credited to an earlier
			// mutant printed nothing at all, so being shadowed and being a
			// no-op were the same observation (rook, coop rumor/1055). Every
			// mutant that attacks anything now names what it attacks, and one
			// that stops attacking loses a rule from this line or the line
			// altogether.
			rules := make([]string, 0, len(movesRules[mid]))
			for rule := range movesRules[mid] {
				rules = append(rules, rule)
			}
			sort.Strings(rules)
			// AND HOW MANY OF THEM IT IS THE ONLY KILLER OF, WHICH IS A FIGURE
			// AND NOT A GATE. fable ruled it at coop rumor/1064 on rook's
			// /1061: "sole killer of no rule" is a coverage SHAPE of the corpus
			// and not a fault in one mutant, and a refusal that fires on eight
			// of ten blocks every release while naming nothing a commit can
			// fix. What stays enforced is the mutant that moves NO rule, which
			// claims to break something and breaks nothing.
			//
			// IT IS WORTH PRINTING BECAUSE IT IS THE THING A READER WANTED
			// when they read the old one-killer-per-rule column as exclusive
			// attribution. Which rules are pinned by one mutant alone was
			// unknowable from that column: it kept whichever killer sorted
			// first and said nothing about the rest, so a rule with one killer
			// and a rule with nine printed the same line.
			//
			// AND NO FIGURE FROM ANY CORPUS IS QUOTED HERE, deliberately. This
			// comment carried "of the toy's ten rules, exactly two have a sole
			// killer" and that number was WRONG -- measured on a branch whose
			// examples/toy still held the pre-regeneration mutants, where one
			// mutant kills eight rules by absence (rook, coop rumor/1077). A
			// count of someone else's corpus, written into this package's
			// source, rots the first time that corpus changes and cannot be
			// checked from here. The report prints the number; this says what
			// it means.
			sole := 0
			for _, rule := range rules {
				if len(killers[rule]) == 1 {
					sole++
				}
			}
			if _, werr := fmt.Fprintf(out, "%-16s %-26s moves %s; sole killer of %d rule(s)\n",
				p.name, mid, strings.Join(rules, ", "), sole); werr != nil {
				return fmt.Errorf("writing the moves line for %s/%s: %w", p.name, mid, werr)
			}
			// ALIVE. Report the ones alive ONLY from the cases: the sealed
			// prefix has made their clause unreachable, which is a fact about
			// the bound and not about the mutant.
			if !movedLedger[mid] && movedCases[mid] {
				rep.LedgerInert = append(rep.LedgerInert, p.name+"/"+mid)
				if _, werr := fmt.Fprintf(out,
					"%-16s %-26s alive from the cases only (the seal reaches its clause)\n",
					p.name, mid); werr != nil {
					return fmt.Errorf("writing the inert line for %s/%s: %w", p.name, mid, werr)
				}
			}
			continue
		}
		rep.DeadMutants = append(rep.DeadMutants, p.name+"/"+mid)
		rep.Refusals = append(rep.Refusals, Refusal{ClauseMutantDead,
			fmt.Sprintf("%s/%s moves no rule on any of the %d case(s) on either path: the statute now "+
				"computes what this mutant computes, so the statute has been weakened into it",
				p.name, mid, len(p.cases))})
	}
	return nil
}

// reachable runs arm 4: every finding value the package's own cases expect
// must actually be produced by the clean statute on at least one of them.
//
// WHY THIS IS NOT `rumor check` AGAIN. check asks "does THIS case produce what
// its author wrote". That question is per case, and deleting a clause makes one
// case disagree -- which check reports and attaint does not need to repeat.
// This asks whether the package can still REACH the finding at all. A statute
// whose `explained` clause is gone has lost a capability the fixtures say it
// has, and the loss is a property of the statute rather than of any one case.
// It is the arm that answers fable's D1: a deleted rule named as no longer
// producing its finding.
func reachable(ctx context.Context, p pkg, rep *Report, out io.Writer) error {
	want := map[string]bool{}
	for _, c := range p.cases {
		if v, ok := c.expect[engine.FindingRule]; ok {
			if sv, ok := v.(string); ok {
				want[sv] = true
			}
		}
	}
	got := map[string]string{} // finding -> the case that produced it
	for _, c := range p.cases {
		f, err := engine.Evaluate(ctx, p.statute, p.rules, c.sealed, c.check)
		if err != nil {
			return fmt.Errorf("%s/%s: %w", p.name, c.name, err)
		}
		if sv, ok := f.Value.(string); ok {
			if _, already := got[sv]; !already {
				got[sv] = c.name
			}
		}
	}
	names := make([]string, 0, len(want))
	for v := range want {
		names = append(names, v)
	}
	sort.Strings(names)
	for _, v := range names {
		if by, ok := got[v]; ok {
			rep.Findings++
			// THE FOURTH SITE, AND THE RUNNER NAMED ONLY THREE (345, 431, 463).
			// Fixed anyway: a write whose failure is discarded is the same
			// defect whether or not the linter reached it, and a lint that
			// catches three of four unchecked writes in one file is itself
			// worth reporting rather than trusting.
			if _, werr := fmt.Fprintf(out, "%-16s %-26s reached by %s\n", p.name, v, by); werr != nil {
				return fmt.Errorf("writing the reachability line for %s/%s: %w", p.name, v, werr)
			}
			continue
		}
		rep.Refusals = append(rep.Refusals, Refusal{ClauseFindingUnreachable,
			fmt.Sprintf("%s: a case expects the finding %q and no case in the package produces it from its "+
				"ledger, so the clause that yields it is gone or unreachable", p.name, v)})
	}
	return nil
}

// reachesAPlantedClaim reports whether the observation in this input document
// reaches a planted claim: one the observer HOLDS and of the type being
// counted. Both conditions, because either alone is reached by a claim the
// control could never have attributed the finding to.
func reachesAPlantedClaim(input map[string]any) (bool, error) {
	// NORMALISED THROUGH JSON RATHER THAN TYPE-ASSERTED. The engine builds
	// this document with concrete Go types -- []map[string]any for claims,
	// []string for known -- and a `.([]any)` assertion on those silently
	// yields the zero value. That is how the first cut of this function
	// reported "reaches none" for every case including the healthy one: a
	// guard whose failure mode is a quiet false is the shape this whole tool
	// exists to refuse, and it took the d0 control going red to show it.
	var doc struct {
		Known  []string `json:"known"`
		Claims []struct {
			Ref     string `json:"ref"`
			Type    string `json:"type"`
			Planted bool   `json:"planted"`
		} `json:"claims"`
		Observation struct {
			Of string `json:"of"`
		} `json:"observation"`
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return false, fmt.Errorf("re-reading the statute's input: %w", err)
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return false, fmt.Errorf("re-reading the statute's input: %w", err)
	}
	known := make(map[string]bool, len(doc.Known))
	for _, k := range doc.Known {
		known[k] = true
	}
	for _, c := range doc.Claims {
		// BOTH CONDITIONS: a planted claim of another type, or one the
		// observer does not hold, is a claim the control could never have
		// attributed this finding to.
		if c.Planted && c.Type == doc.Observation.Of && known[c.Ref] {
			return true, nil
		}
	}
	return false, nil
}

// stripClears runs the strip's SECOND direction over one package.
//
// docs/vocabulary.md, `planted`, states two directions and the tree checked
// one: "with `planted` removed from every planted claim **in a case that
// declares the lie its control** ... the same run must
// still produce the same `bug` findings, and with the planted claims removed
// altogether the run must be `consistent`". The first is the committed
// stripped-tag case and drill d3. This is the second, and it is the direction
// that proves the bug WAS the lie: a bug that survives the lie's removal was
// something else all along, and a statute could pass every other arm here
// while reporting one.
//
// THE COUNTERFACTUAL IS DERIVED, NOT COMMITTED, and that is a decision with a
// reason: an eighteenth case under examples/planted-lie would move R1's gated
// 3/17/19, which is precisely why rook declined to add a `clock` example
// (quill rumor/577). Deriving it also makes the check general -- every planted
// case in every package, rather than one fixture that could be written to
// pass.
//
// TELLS GO WITH THEIR CLAIMS. R2.1 refuses a tell that passes a claim not yet
// made, so a prefix keeping a tell whose claim was removed would not load at
// all -- and a refusal there would read as this arm's finding rather than as
// the counterfactual being unbuildable.
func stripClears(ctx context.Context, p pkg, rep *Report, out io.Writer) error {
	for _, c := range p.cases {
		if !c.planted {
			continue
		}
		// SCOPED TO CASES THAT DECLARE THE CONTROL, and the narrowing is
		// stated because it is exactly the shape that must not be done
		// quietly: the arm FIRES on three of the four planted cases in
		// examples/ and this skips them.
		//
		// THE REASON, and it covers these three and nothing wider: the
		// vocabulary's `planted` section opens "its two uses are both checks
		// on the engine", and the strip is the second use -- a statement about
		// a fixture using the lie AS ITS CONTROL. role-visibility's
		// viewer-own-lie-exceeds and transitive-count's two carry a planted
		// claim INCIDENTALLY: they are testing visibility and knowledge gaps,
		// their bug does not rest on the lie (the corroboration arm already
		// reports they declare no attribution), and removing it leaves a bug
		// or an explained that was never about the lie.
		//
		// WHAT I AM NOT DOING IS DECIDING IT. Read without that scoping the
		// clause says those three violate the contract, which is a claim about
		// R1's GATED tree and fable's to rule, not mine at a checkpoint. So
		// the skipped cases are COUNTED AND PRINTED rather than passed over,
		// and the question is on the record. If fable rules the unqualified
		// reading, deleting four lines here restores it.
		if v, ok := c.expect[CorroborationRule]; !ok || !sameJSON(v, true) {
			rep.PlantedNotControl++
			if _, werr := fmt.Fprintf(out,
				"%-16s %-26s planted but not a control: the strip clause is not applied\n",
				p.name, c.name); werr != nil {
				return fmt.Errorf("writing the skip line for %s/%s: %w", p.name, c.name, werr)
			}
			continue
		}
		// AND THE CONTROL MUST HAVE BEEN EVALUATED, which d4 forced into the
		// open rather than a review finding: a control that never FIRES is by
		// construction a case whose bug does not rest on the lie, so removing
		// the lie cannot clear it and this arm fires too. The two overlap by
		// necessity, not by a fixture's accident.
		//
		// THE SPECIFIC DIAGNOSIS WINS. "The observation reaches no planted
		// claim" already names the defect exactly; adding "and the bug
		// survives removing it" is the same fact restated, and it would make
		// every armed-never-fired case refuse twice -- which fable's "must not
		// refuse anything else" is precisely about. The strip clause
		// presupposes the lie is in play; where it is not, the question is not
		// yet askable.
		f0, err := engine.Evaluate(ctx, p.statute, p.rules, c.sealed, c.check)
		if err != nil {
			return fmt.Errorf("%s/%s: %w", p.name, c.name, err)
		}
		reached, err := reachesAPlantedClaim(f0.Input)
		if err != nil {
			return fmt.Errorf("%s/%s: %w", p.name, c.name, err)
		}
		if !reached {
			continue
		}
		stripped, removed, err := withoutPlantedClaims(c.sealed)
		if err != nil {
			return fmt.Errorf("%s/%s: building the claims-removed counterfactual: %w", p.name, c.name, err)
		}
		if removed == 0 {
			// The case is marked planted and the rebuild found nothing to
			// remove: the two readings of "planted" have diverged and the
			// silent version of this is an arm that passes over everything.
			return fmt.Errorf("%s/%s is a planted case and the counterfactual removed no claim", p.name, c.name)
		}
		f, err := engine.Evaluate(ctx, p.statute, p.rules, stripped, c.check)
		if err != nil {
			return fmt.Errorf("%s/%s: the claims-removed counterfactual did not evaluate: %w", p.name, c.name, err)
		}
		// UNDEFINED IS NOT SURVIVAL, and the distinction is this project's own:
		// a finding is a rule that HOLDS, and absence is key-absence (quill
		// r3/10). Without this branch an undefined finding printed "the
		// finding is <nil> ... the bug survives the lie's removal", which is
		// false -- nothing survived, the statute answered nothing. Unreachable
		// while all three statutes carry `default finding`; nothing requires
		// one, so it is one default-less statute away.
		if !f.Defined {
			rep.Refusals = append(rep.Refusals, Refusal{ClauseStripDoesNotClear,
				fmt.Sprintf("%s/%s: with its %d planted claim(s) removed the statute yields NO finding at "+
					"all, so nothing says the run came back consistent -- an absent finding is not a verdict "+
					"and must not be read as one",
					p.name, c.name, removed)})
			continue
		}
		if !sameJSON(f.Value, "consistent") {
			rep.Refusals = append(rep.Refusals, Refusal{ClauseStripDoesNotClear,
				fmt.Sprintf("%s/%s: with its %d planted claim(s) removed the finding is %v, not consistent -- "+
					"the bug survives the lie's removal, so it was not the lie",
					p.name, c.name, removed, f.Value)})
			continue
		}
		rep.Cleared++
		if _, werr := fmt.Fprintf(out, "%-16s %-26s cleared by removing %d planted claim(s)\n",
			p.name, c.name, removed); werr != nil {
			return fmt.Errorf("writing the clearance line for %s/%s: %w", p.name, c.name, werr)
		}
	}
	return nil
}

// withoutPlantedClaims rebuilds a sealed ledger with every planted claim gone,
// and with any tell that passed one of them gone with it. It returns the new
// ledger and how many claims it dropped.
func withoutPlantedClaims(l *ledger.Ledger) (*ledger.Ledger, int, error) {
	// REFS ARE NOT UNIQUE IN THE STORE, so the tell filter below cannot assume
	// a ref identifies one claim (quill r3/10). If a ref is carried by BOTH a
	// planted and an honest claim, dropping tells by ref would drop tells that
	// pass the honest one -- silently changing what the observer knows in the
	// counterfactual, which is the thing this arm is measuring. Nothing in the
	// tree produces such a ledger and nothing forbids one, so it is refused by
	// name rather than resolved by a guess about which claim a tell meant.
	dropped := map[string]bool{}
	honest := map[string]bool{}
	for _, f := range l.SealedPrefix() {
		if f.Kind != ledger.KindClaim {
			continue
		}
		if f.Planted {
			dropped[f.Ref] = true
		} else {
			honest[f.Ref] = true
		}
	}
	for ref := range dropped {
		if honest[ref] {
			return nil, 0, fmt.Errorf("the ref %q is carried by both a planted and an honest claim, so "+
				"removing the planted one cannot be told from removing the honest one and a tell passing "+
				"it cannot be attributed", ref)
		}
	}
	out := ledger.New(false)
	highest, removed := 0, 0
	for _, f := range l.SealedPrefix() {
		if f.Kind == ledger.KindClaim && f.Planted {
			// COUNTED PER CLAIM, not per distinct ref: the count appears in
			// the operator's line and a ref-keyed count would understate a
			// ledger carrying two planted claims under one ref.
			removed++
			continue
		}
		if f.Kind == ledger.KindTold && dropped[f.ClaimRef] {
			continue
		}
		if err := out.AppendAt(f); err != nil {
			return nil, 0, err
		}
		if f.Position > highest {
			highest = f.Position
		}
	}
	if highest == 0 {
		return nil, 0, fmt.Errorf("removing the planted claims empties the prefix, so there is nothing to judge")
	}
	// THE SEAL MOVES DOWN WITH THE LAST SURVIVING FACT WHEN IT HAS TO, and the
	// store is what forced the question: SealAt refuses a position the ledger
	// no longer holds ("a seal names a prefix that exists"), which is right --
	// removing the claim AT the seal leaves that position empty.
	//
	// SEALING AT THE HIGHEST REMAINING LOSES NOTHING, which is why this is safe
	// rather than a weakening: every surviving fact is at or below it, so the
	// prefix still holds all of them, and the observation at seal+1 still sits
	// after every one. What changes is only the integer, and v1's own rule is
	// that a replay assigns its own positions and compares findings, never
	// positions.
	seal := l.Seal()
	if highest < seal {
		seal = highest
	}
	if err := out.SealAt(seal); err != nil {
		return nil, 0, err
	}
	return out, removed, nil
}

// moved reports whether a rule's value differs between the clean statute and a
// mutant on one case. A rule that becomes UNDEFINED has moved.
func moved(clean, mutant map[string]any, rule string) bool {
	cv, cok := clean[rule]
	mv, mok := mutant[rule]
	if cok != mok {
		return true
	}
	return cok && !sameJSON(cv, mv)
}

// evalCasePath evaluates the statute against the case's own input block --
// the bare path, with no ledger and no seal.
func evalCasePath(ctx context.Context, module string, rules []string, input any) (map[string]any, error) {
	got := map[string]any{}
	for _, rule := range rules {
		rs, err := rego.New(
			rego.Query(engine.StatutePackage+"."+rule),
			rego.Module("statute.rego", module),
			rego.Input(input),
		).Eval(ctx)
		if err != nil {
			return nil, fmt.Errorf("rule %s: %w", rule, err)
		}
		if len(rs) == 0 || len(rs[0].Expressions) == 0 {
			continue
		}
		got[rule] = rs[0].Expressions[0].Value
	}
	return got, nil
}

func declares(rules []string, name string) bool {
	for _, r := range rules {
		if r == name {
			return true
		}
	}
	return false
}

// load reads every package under dir: statute, rules, cases with their sealed
// ledgers, and mutants.
func load(dir string) ([]pkg, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("%s is not readable, so nothing would be judged: %w", dir, err)
	}
	var out []pkg
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pdir := filepath.Join(dir, e.Name())
		sb, err := os.ReadFile(filepath.Join(pdir, "statute.rego"))
		if err != nil {
			continue
		}
		p := pkg{name: e.Name(), dir: pdir, statute: string(sb)}

		raw, err := os.ReadFile(filepath.Join(pdir, "rules.json"))
		if err != nil {
			return nil, fmt.Errorf("%s has a statute but no rules.json: %w", p.name, err)
		}
		if err := json.Unmarshal(raw, &p.rules); err != nil {
			return nil, fmt.Errorf("%s/rules.json: %w", p.name, err)
		}

		if p.mutants, err = filepath.Glob(filepath.Join(pdir, "mutants", "*.rego")); err != nil {
			return nil, err
		}
		sort.Strings(p.mutants)

		files, err := filepath.Glob(filepath.Join(pdir, "cases", "*.json"))
		if err != nil {
			return nil, err
		}
		sort.Strings(files)
		for _, cf := range files {
			c, err := loadCase(pdir, p.name, cf)
			if err != nil {
				return nil, err
			}
			p.cases = append(p.cases, c)
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out, nil
}

func loadCase(pdir, pkgName, cf string) (exCase, error) {
	name := strings.TrimSuffix(filepath.Base(cf), ".json")
	raw, err := os.ReadFile(cf)
	if err != nil {
		return exCase{}, err
	}
	var doc caseDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return exCase{}, fmt.Errorf("%s: %w", cf, err)
	}
	var flat map[string]any
	if err := json.Unmarshal(raw, &struct {
		Input *map[string]any `json:"input"`
	}{Input: &flat}); err != nil {
		return exCase{}, fmt.Errorf("%s: %w", cf, err)
	}
	if len(doc.Expect) == 0 {
		return exCase{}, fmt.Errorf("%s/%s has no expect block, so it asserts nothing", pkgName, name)
	}
	var sd sealDoc
	if err := readJSON(filepath.Join(pdir, "ledger", name+".seal.json"), &sd); err != nil {
		return exCase{}, fmt.Errorf("%s/%s: %w", pkgName, name, err)
	}
	lraw, rerr := os.ReadFile(filepath.Join(pdir, "ledger", name+".jsonl"))
	if err := rerr; err != nil {
		return exCase{}, fmt.Errorf("%s/%s has no ledger file: %w", pkgName, name, err)
	}
	l, err := ledger.Load(strings.NewReader(string(lraw)), bindings.ClockControlledAt(pdir, decl.TOML))
	if err != nil {
		return exCase{}, fmt.Errorf("%s/%s: the committed ledger does not load: %w", pkgName, name, err)
	}
	if err := l.SealAt(sd.Seal); err != nil {
		return exCase{}, fmt.Errorf("%s/%s: sealing at the recorded %d: %w", pkgName, name, sd.Seal, err)
	}
	o := doc.Input.Observation
	// PLANTED IS READ FROM THE SEALED PREFIX, not from the case file's `input`
	// block. The ledger is what the engine judges, so a tag present in the case
	// JSON and absent from the ledger must read as absent -- which is precisely
	// the drill that strips the ledger files.
	planted := false
	for _, f := range l.SealedPrefix() {
		if f.Kind == ledger.KindClaim && f.Planted {
			planted = true
			break
		}
	}
	return exCase{
		name:      name,
		expect:    doc.Expect,
		sealed:    l,
		planted:   planted,
		caseInput: flat,
		check: engine.Check{
			Actor:  o.Actor,
			Query:  o.Query,
			Of:     o.Of,
			Source: ledger.Source(o.Source),
			Value:  o.Value,
			Role:   o.Role,
		},
	}, nil
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// sameJSON compares through encoding/json in both directions so an integer from
// a file and a float from the evaluator do not read as a difference.
func sameJSON(a, b any) bool {
	ja, err1 := json.Marshal(a)
	jb, err2 := json.Marshal(b)
	if err1 != nil || err2 != nil {
		return false
	}
	var ua, ub any
	if json.Unmarshal(ja, &ua) != nil || json.Unmarshal(jb, &ub) != nil {
		return false
	}
	return reflect.DeepEqual(ua, ub)
}
