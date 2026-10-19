// Package validate refuses a project whose statute, bindings and platform
// disagree. It is `rumor validate` (R4.3, issue 64).
//
// THE BOUNDARY WITH THE PARSERS IS THE WHOLE DESIGN, and internal/bindings
// states the other half of it: **a parser must not refuse what a validator is
// built to refuse.** Eight of the eighteen arms are about declarations that are
// WELL FORMED AND WRONG -- an action with no precondition, a rule referencing a
// claim type no binding emits -- so `emits`, `requires`, `flow`, `bound` and
// `[uses]` are optional over there and refused here. If the parser demanded
// them, the fixture for arm 3 would be testing the parser and the arm could
// never fire.
//
// THE BOUNDARY WITH attaint IS THE SAME SHAPE, ruled at coop r4/69: a mutant
// file does not declare which rule it cuts, so "does a killing mutant exist for
// THIS rule" is answerable only by RUNNING the mutants, which is attaint's arm.
// Arm 11 here is the STRUCTURAL half and nothing more -- rules declared and no
// mutant present at all. Duplicating attaint's question statically would mean
// answering it with a weaker oracle.
//
// EXIT STATUSES ARE docs/contract.md:57's, not this package's choice: 0 passed,
// 1 refused, 2 could-not-run. A document that cannot be READ is could-not-run
// and arrives before any arm evaluates, so a project that is not a project does
// not get a refusal naming an arm. THE PATH ITSELF IS THE FIRST OF THOSE
// DOCUMENTS: a directory that is not there is an unanswerable question, and
// until quill measured it (coop r4/91) a mistyped path came back as the project
// being wrong -- `no-bindings`, exit 1 -- which is a verdict about declarations
// nothing opened.
package validate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/decl"
	"forge.axonibyte.com/axonibyte/rumor/internal/platform"
)

// TotalArms is how many arms this package has, pinned here because a run that
// evaluated sixteen of them reads identically to one that evaluated eighteen
// and found nothing -- the same reason every count in this project is declared
// (fable ruled the number at coop breakout-r4/6, and it moves only with a count
// posted there first).
const TotalArms = 18

// Refusal is one arm firing, named by the arm so a fixture can assert that it
// tripped ITS arm and no other -- the invariant fable set over the eighteen.
type Refusal struct {
	Arm    string
	Detail string
}

// String names an ARM by number and a VACUITY by its clause.
//
// A CLAUSE IS NOT AN ARM, and printing "arm no-rule-declared" said it was. The
// eighteen are numbered; a vacuity refusal is a clause the way `rumor attaint`'s
// are ("no-mutant-examined"), and a reader greping for an arm should not match
// one.
func (r Refusal) String() string {
	for _, a := range allArmIDs {
		if r.Arm == a {
			return fmt.Sprintf("arm %s: %s", r.Arm, r.Detail)
		}
	}
	return fmt.Sprintf("%s: %s", r.Arm, r.Detail)
}

// Report is what validating one project produced: every refusal rather than the
// first, as internal/platform and `rumor attaint` both do, because a project
// tripping three arms is a different event from one tripping the arm it was
// written to trip.
type Report struct {
	Project string

	Refusals    []Refusal
	CouldNotRun []string

	// EXAMINED, so a pass cannot be vacuous. An arm that ran over an empty set
	// answers identically to one that ran over a sound project, which is the
	// shape this project refuses everywhere else.
	Actions      int
	Observations int
	Cases        int
	Rules        int
	Mutants      int
	Stages       int
	Described    bool

	// EVALUATED AND SKIPPED, SEPARATELY. An arm whose precondition is absent --
	// the browser stage off, no rule declared, no seeds file -- did not pass;
	// it did not run. Reporting the two as one would make a project that
	// exercises four arms look like one that satisfies eighteen.
	Evaluated []string
	Skipped   map[string]string
	Rollbacks int
}

// OK reports whether the project is one its own declarations agree on.
func (r Report) OK() bool { return len(r.Refusals) == 0 && len(r.CouldNotRun) == 0 }

// Exit is docs/contract.md:57's status for this report. could-not-run outranks
// refused: if the project could not be read, what an arm would have said about
// it is not known.
func (r Report) Exit() int {
	switch {
	case len(r.CouldNotRun) > 0:
		return 2
	case len(r.Refusals) > 0:
		return 1
	default:
		return 0
	}
}

// ran records that an arm evaluated. Every arm calls this or skip, exactly
// once, and TestEveryArmAccountsForItself asserts the two cover TotalArms.
func (r *Report) ran(arm string) {
	r.Evaluated = append(r.Evaluated, arm)
}

// skip records that an arm did not evaluate, and why. The reason is the useful
// half: "the browser stage is disabled" tells a reader that two arms are
// waiting on a flag rather than that the project satisfied them.
func (r *Report) skip(arm, f string, a ...any) {
	if r.Skipped == nil {
		r.Skipped = map[string]string{}
	}
	r.Skipped[arm] = fmt.Sprintf(f, a...)
}

// vacuous refuses the project for saying nothing, and accounts for every arm.
//
// EXIT 1 AND NOT 2, ruled by fable at coop r4/79: could-not-run belongs to the
// adapter and the decoder, where the PROJECT is not wrong. A statute declaring
// zero rules, a missing bindings file, bindings with zero actions -- those are
// the project being wrong, because there is nothing to validate until it has
// said what it is. `rumor attaint` already draws the line there: zero packages,
// zero rules or zero mutants is a refusal and not a pass.
//
// IT SKIPS ALL EIGHTEEN WITH THE SAME REASON rather than leaving them
// unaccounted. The arms did not run, and why they did not run is the finding.
func (r *Report) vacuous(clause, f string, a ...any) {
	r.refuse(clause, f, a...)
	r.unreached("the project declares too little to validate: see the %s refusal", clause)
}

// unreached accounts for the arms that never ran. It is vacuous's half on the
// could-not-run side: the same accounting, a different verdict.
//
// IT EXISTS BECAUSE THE TALLY LINE IS READ AS A MEASUREMENT, which is the
// reason TotalArms is declared at all. Every could-not-run return used to leave
// all eighteen unaccounted and print "0 of 18 arm(s) evaluated" with no reason
// beside any arm, so the invariant the suite asserts over the vacuity and the
// live paths -- evaluated plus skipped equals TotalArms -- held everywhere
// except where a document failed to parse, which is the one place a reader most
// needs to know that nothing was measured.
func (r *Report) unreached(f string, a ...any) {
	reason := fmt.Sprintf(f, a...)
	for _, arm := range allArmIDs {
		if _, already := r.Skipped[arm]; !already {
			r.skip(arm, "%s", reason)
		}
	}
}

// allArmIDs is the eighteen, for the accounting above. tests/validate asserts
// this list against TotalArms so the two cannot drift.
var allArmIDs = []string{
	"1", "2", "3", "4", "5", "6", "7a", "7b", "8",
	"9a", "9b", "9c", "10a", "10b", "11", "12a", "12b", "13",
}

func (r *Report) refuse(arm, f string, a ...any) {
	r.Refusals = append(r.Refusals, Refusal{Arm: arm, Detail: fmt.Sprintf(f, a...)})
}

func (r *Report) cannot(f string, a ...any) {
	r.CouldNotRun = append(r.CouldNotRun, fmt.Sprintf(f, a...))
}

// caseDoc is the half of a case file this package reads. It is NOT
// internal/examples' struct: that one reads `expect` to compare verdicts, and
// this one reads the observation's declared fields because arms 12a and 12b are
// about a declaration that is missing, which a struct with defaults cannot
// tell from a zero. Hence the pointers.
type caseDoc struct {
	Input struct {
		Observation struct {
			Of       string  `json:"of"`
			Position *int    `json:"position"`
			Source   *string `json:"source"`
		} `json:"observation"`
	} `json:"input"`
}

// Run validates the project rooted at dir.
func Run(dir string) Report {
	rep := Report{Project: filepath.Base(dir)}

	// THE PATH IS CHECKED BEFORE ANYTHING IN IT, and the verdict is
	// could-not-run NAMING THE PATH. A mistyped path and a real project missing
	// its bindings.toml used to produce the same refusal, differing only in the
	// project name on the tally line (quill, coop r4/91) -- so the reader was
	// told their project was wrong when the directory they named was not there
	// to be wrong. A typo is visible only beside the path.
	if info, serr := os.Stat(dir); serr != nil {
		rep.cannot("there is no project to validate: %v", serr)
		rep.unreached("the path given could not be opened as a project directory")
		return rep
	} else if !info.IsDir() {
		rep.cannot("there is no project to validate: %s is not a directory", dir)
		rep.unreached("the path given is not a project directory")
		return rep
	}

	// A MISSING OR UNREADABLE bindings.toml IS THE PROJECT SAYING NOTHING, and
	// fable ruled that a refusal rather than could-not-run (r4/79). The
	// DECODER's own failures stay could-not-run, which is the line fable drew:
	// malformed syntax is the reader's finding, an absent declaration is the
	// project's.
	bpath := filepath.Join(dir, "bindings.toml")
	if _, serr := os.Stat(bpath); serr != nil {
		rep.vacuous("no-bindings", "there is no bindings.toml: a project that does not say what it binds "+
			"has nothing for any arm to read")
		return rep
	}
	brep, err := bindings.Load(bpath, decl.TOML)
	if err != nil {
		rep.cannot("bindings.toml: %v", err)
		rep.unreached("bindings.toml could not be read, so no arm had declarations to read")
		return rep
	}
	if !brep.OK() {
		// THE PARSER'S REFUSALS ARE THE PROJECT'S, not the reader's: it refuses
		// a file it READ and found wanting -- "no actions: a binding with
		// nothing to do binds nothing" among them.
		var why []string
		for _, ref := range brep.Refusals {
			why = append(why, ref.Error())
		}
		rep.vacuous("bindings-refused", "bindings.toml does not describe a project: %s", strings.Join(why, "; "))
		return rep
	}
	b := brep.Bindings
	rep.Actions = len(b.Actions)
	rep.Observations = len(b.Observations)

	prep, perr := platform.Load(filepath.Join(dir, "platform.toml"), decl.TOML)
	if perr != nil {
		rep.cannot("platform.toml: %v", perr)
		rep.unreached("platform.toml could not be read, so no arm had a platform to read")
		return rep
	}
	if !prep.OK() {
		for _, ref := range prep.Refusals {
			rep.cannot("platform.toml: %s", ref.Error())
		}
		rep.unreached("platform.toml does not describe a platform: see the could-not-run line(s)")
		return rep
	}
	plat := prep.Platform
	rep.Stages = len(plat.Stages)
	rep.Rollbacks = len(plat.Rollback)

	var rules []string
	if raw, err := os.ReadFile(filepath.Join(dir, "rules.json")); err != nil {
		rep.cannot("rules.json: %v", err)
	} else if err := json.Unmarshal(raw, &rules); err != nil {
		rep.cannot("rules.json: %v", err)
	}
	rep.Rules = len(rules)

	caseFiles, _ := filepath.Glob(filepath.Join(dir, "cases", "*.json"))
	sort.Strings(caseFiles)
	cases := make(map[string]caseDoc, len(caseFiles))
	for _, cf := range caseFiles {
		raw, err := os.ReadFile(cf)
		if err != nil {
			rep.cannot("%s: %v", filepath.Base(cf), err)
			continue
		}
		var c caseDoc
		if err := json.Unmarshal(raw, &c); err != nil {
			rep.cannot("%s: %v", filepath.Base(cf), err)
			continue
		}
		cases[filepath.Base(cf)] = c
	}
	rep.Cases = len(cases)

	// A PROJECT WITH NOTHING TO EXAMINE IS could-not-run, NOT A PASS. Every arm
	// below quantifies over one of these sets, and all six would be silent over
	// empty ones.
	// THE ACTIONS HALF OF THIS GUARD WOULD BE DEAD CODE, and saying so beats
	// carrying it: internal/bindings/bindings.go:202 refuses a file with no
	// actions outright, so a parse that SUCCEEDED has at least one and
	// `rep.Actions == 0` is unreachable here. A check that cannot fire reads
	// exactly like one that does. The coupling is pinned by an oracle instead --
	// tests/validate asserts that bindings still refuses a zero-action file --
	// so this hole cannot open quietly if that refusal ever moves.
	// ZERO RULES IS A VACUITY AND NOT A PASS, found by driving fable's D8 drill
	// (coop breakout-r4/0: "zero rules; zero bindings; zero actions -- each a
	// refusal, never a pass"). It passed until that drill ran: arm 11 SKIPS when
	// rules.json declares nothing, sixteen arms then evaluated over a statute
	// whose rules nothing asserts, and the verdict was exit 0. A project with no
	// rule has no statute worth validating.
	// COULD-NOT-RUN IS ANSWERED BEFORE VACUITY, because a vacuity refusal is a
	// claim about what the project DECLARES and both counts below are zero when
	// the file holding them would not parse. An unreadable rules.json used to
	// report "rules.json declares no rule" beside the decode failure -- a
	// verdict about a declaration nobody read, and the same mistake quill found
	// at the project path one level up. A malformed case file did it too, via
	// the no-case clause.
	if len(rep.CouldNotRun) > 0 {
		rep.unreached("a declaration could not be read, so what it says is unknown: " +
			"see the could-not-run line(s)")
		return rep
	}
	if rep.Rules == 0 {
		rep.vacuous("no-rule-declared", "rules.json declares no rule, so there is no statute to hold "+
			"the bindings to")
		return rep
	}
	if rep.Cases == 0 {
		rep.vacuous("no-case", "there is no case under cases/, so no observation exists for an arm to read")
		return rep
	}

	arm1(&rep, b, cases)
	arm2(&rep, b)
	arm3(&rep, b)
	arm4(&rep, b, plat)
	arm5(&rep, b, plat)
	arm6(&rep, b, plat)
	arm7(&rep, b)
	arm8(&rep, dir, b)
	arm10a(&rep, dir, plat)
	arm10b(&rep, dir, plat)
	arm11(&rep, dir, rules)
	arm12(&rep, cases)

	// THE LIVE ARMS LAST, because they are the only ones that spawn anything and
	// the static verdict should not depend on whether an adapter started.
	if d, ok := live(&rep, dir, plat); ok {
		arm9(&rep, b, d)
		arm13(&rep, plat, d)
	}
	return rep
}
