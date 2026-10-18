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
// not get a refusal naming an arm.
package validate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/decl"
	"forge.axonibyte.com/axonibyte/rumor/internal/platform"
)

// Refusal is one arm firing, named by the arm so a fixture can assert that it
// tripped ITS arm and no other -- the invariant fable set over the eighteen.
type Refusal struct {
	Arm    string
	Detail string
}

func (r Refusal) String() string { return fmt.Sprintf("arm %s: %s", r.Arm, r.Detail) }

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
	Rollbacks    int
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

	brep, err := bindings.Load(filepath.Join(dir, "bindings.toml"), decl.TOML)
	if err != nil {
		rep.cannot("bindings.toml: %v", err)
		return rep
	}
	if !brep.OK() {
		for _, ref := range brep.Refusals {
			rep.cannot("bindings.toml: %s", ref.Error())
		}
		return rep
	}
	b := brep.Bindings
	rep.Actions = len(b.Actions)
	rep.Observations = len(b.Observations)

	prep, perr := platform.Load(filepath.Join(dir, "platform.toml"), decl.TOML)
	if perr != nil {
		rep.cannot("platform.toml: %v", perr)
		return rep
	}
	if !prep.OK() {
		for _, ref := range prep.Refusals {
			rep.cannot("platform.toml: %s", ref.Error())
		}
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
	if rep.Cases == 0 {
		rep.cannot("no case under cases/, so every arm over observations is vacuous")
	}
	if len(rep.CouldNotRun) > 0 {
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
