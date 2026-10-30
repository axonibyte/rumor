// Package stage is the dispatcher's own vocabulary: which stage names it can
// run, and which of a project's declared stages a run will take.
//
// WHY THE SET LIVES HERE AND NOT IN internal/platform. platform READS a
// declaration; it takes `[stages]` as a map keyed by whatever the file says,
// which rook measured at coop r8/16 -- "any name under `[stages]` becomes a
// stage", decl's unknown-key refusal is about keys WITHIN a table, and
// validate's `len(plat.Stages)` is a count and not a membership test. So
// nothing refused `stages.fuzzz = { enabled = true }`: it parsed, counted, and
// would have been recorded. The capability set belongs to whatever can RUN a
// stage, which is the dispatcher, so the refusal is here.
//
// `api` IS THE MAIN LOOP AND `api-sim` WAS A STALE NAME FOR IT (fable, coop
// rumor/1477, on my census at r8/15: all thirty-six platform.toml in the tree
// declare `api` and not one declares `api-sim`, `fuzz` or `concurrency` --
// those three appeared only in contract.md's example). 110 does not add a
// second loop beside the walk; it gives the one loop its settling, its time
// and its periodic full reconciliation, which is what that line described.
package stage

import (
	"fmt"
	"sort"
	"strings"

	"forge.axonibyte.com/axonibyte/rumor/internal/platform"
)

// API is the main loop: the walk `rumor run` has always taken.
const API = "api"

// Concurrency is the fan-out stage: limit-plus-one requests in flight against
// one world, for every action a project declares as contended (R8.2, 109).
//
// IT IS A CONSTANT BECAUSE THE DISPATCH BRANCHES ON IT. `api` has been one
// since the dispatch existed, and the second runnable name wants the same: a
// string literal in cmd/rumor's switch and another in this map is one name in
// two places, which is the drift the tripwire below is about.
const Concurrency = "concurrency"

// Fuzz is 108's hostile-corpus stage: every declared write action driven with
// inputs derived from the declarations, counting only what reached the system.
//
// IT WAS BUILT IN 108 AND WIRED HERE, which is the gap quill found (coop r8/75)
// and fable ruled outstanding rather than deliberate (rumor/1580): 108 shipped
// `internal/fuzz` with ZERO production callers, this map reading
// `"fuzz": false, // runnable when it lands` after it had landed, and no
// project declaring the stage. A child named for a stage ships that stage
// runnable under `rumor run` (rumor/1542), so the wiring is 110's first item
// and precedes anything of 110's own.
const Fuzz = "fuzz"

// Known is every stage name the dispatcher recognises, with whether it can
// run one today.
//
// KNOWN AND RUNNABLE ARE TWO FACTS. `browser` is declared ENABLED by the toy
// and by thirty-four other files today and no dispatcher can run it before R9;
// refusing those projects would be refusing a declaration the contract invites.
// So a known name the dispatcher cannot run yet is not an error and not a run
// either -- it simply does not appear among the stages that ran, and D13's skip
// reasons are 111's to report.
var Known = map[string]bool{
	API:         true,  // the main loop, runnable now
	Concurrency: true,  // 109's fan-out, runnable now
	Fuzz:        true,  // 108's stage, wired by 110 -- see the comment above
	"browser":   false, // R9
}

// Select is the stages a run will take, in a fixed order, and the refusal for
// a declared name the dispatcher does not know.
//
// THE REFUSAL COMES BEFORE THE RUN (fable, coop rumor/1477): a name nobody can
// run is a project error, and discovering it after a walk would mean reporting
// a run whose stage list was already wrong. It names the stage and lists what
// IS known, because "unknown stage" without the set leaves the author guessing
// at a spelling.
//
// ORDER IS Known's CANONICAL ORDER AND NOT THE DECLARATION's: two projects
// declaring the same stages in different orders run them alike, and a record
// whose order followed the file could not be compared between projects.
func Select(p platform.Platform, profile string) ([]string, error) {
	declared := make([]string, 0, len(p.Stages))
	for name := range p.Stages {
		declared = append(declared, name)
	}
	sort.Strings(declared)
	for _, name := range declared {
		if _, ok := Known[name]; !ok {
			return nil, fmt.Errorf("the platform declares a stage %q, which this build cannot run: "+
				"known stages are %s. A stage nobody can run is a declaration error, and it is "+
				"refused here rather than after a walk whose stage list was already wrong",
				name, strings.Join(knownNames(), ", "))
		}
	}

	// A PROFILE NARROWS; ITS ABSENCE DOES NOT WIDEN. Without one the run takes
	// the stages the platform ENABLES, which is what `rumor run` has always
	// done with the walk. With one, platform's own cross-check has already
	// refused a profile naming an undeclared stage.
	want := map[string]bool{}
	if profile == "" {
		for name, st := range p.Stages {
			want[name] = st.Enabled
		}
	} else {
		prof, ok := p.Profiles[profile]
		if !ok {
			return nil, fmt.Errorf("no profile %q is declared, so the stages it would select are unknown", profile)
		}
		for _, name := range prof.Stages {
			want[name] = p.Stages[name].Enabled
		}
	}

	var out []string
	for _, name := range knownNames() {
		if want[name] && Known[name] {
			out = append(out, name)
		}
	}
	if len(out) == 0 {
		// AN EMPTY SELECTION IS A REFUSAL AND NOT A RUN THAT DID NOTHING.
		// internal/examples already holds this rule for a walk -- "an empty
		// walk is not a pass" -- and the same reading applies one level up: a
		// profile naming only stages this build cannot run, or a platform
		// disabling the one it can, would otherwise exit 0 having tested
		// nothing, which reads exactly like a clean run.
		// THE ASKED-FOR SET IS THE ONE THAT ASKED, and driving this refusal is
		// what caught it: with a profile named, the message read "profile
		// \"expiring\" selects api, browser" while that profile selects only
		// `api` -- the platform's DECLARATIONS wearing the profile's name. A
		// refusal that misreports what was asked for sends its reader to the
		// wrong file.
		where, asked := "the platform enables", declared
		if profile != "" {
			where = fmt.Sprintf("profile %q selects", profile)
			asked = append([]string(nil), p.Profiles[profile].Stages...)
			sort.Strings(asked)
		}
		// EACH ASKED-FOR STAGE SAYS WHY IT DID NOT RUN, and the first fix to
		// this message is why: correcting "selects api, browser" to "selects
		// api" left it reading "selects api, and this build runs api" over a
		// run that took no stage -- true in every clause and useless, because
		// the one fact that explains it is that `api` is declared with
		// enabled = false. A refusal that lists the sets without the reason
		// sends its reader back to guess which of them is wrong.
		var why []string
		for _, name := range asked {
			switch {
			case !Known[name] && p.Stages[name].Enabled:
				why = append(why, fmt.Sprintf("%s is enabled and this build cannot run it yet", name))
			case !p.Stages[name].Enabled:
				why = append(why, fmt.Sprintf("%s is declared with enabled = false", name))
			}
		}
		return nil, fmt.Errorf("no stage this build can run: %s %s -- %s. This build runs %s, and a "+
			"run that took no stage at all would exit reporting nothing tested",
			where, declaredList(asked), strings.Join(why, "; "), runnableList())
	}
	return out, nil
}

// declaredList and runnableList render the two sets a refusal has to name: what
// the project asked for and what this build could have given it. Without both
// the author cannot tell a typo from a stage that is simply not built yet.
func declaredList(declared []string) string {
	if len(declared) == 0 {
		return "nothing"
	}
	return strings.Join(declared, ", ")
}

func runnableList() string {
	var out []string
	for _, name := range knownNames() {
		if Known[name] {
			out = append(out, name)
		}
	}
	return strings.Join(out, ", ")
}

// knownNames is Known's names in canonical order: the main loop first, then the
// rest alphabetically, so a refusal and a record read the same way twice.
func knownNames() []string {
	rest := make([]string, 0, len(Known))
	for name := range Known {
		if name != API {
			rest = append(rest, name)
		}
	}
	sort.Strings(rest)
	return append([]string{API}, rest...)
}
