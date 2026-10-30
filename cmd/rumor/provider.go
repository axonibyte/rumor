package main

// The rollback provider is the only thing in this binary that OWNS the system
// under test, and two stages now need that: a shrink replays a failure on a
// world it can put back, and a contention points N adapters at one world. The
// readings they share live here so neither call site carries a second copy.

import (
	"fmt"
	"sort"
	"strings"

	"forge.axonibyte.com/axonibyte/rumor/internal/platform"
	"forge.axonibyte.com/axonibyte/rumor/internal/rollback"
)

// soleProvider is the name of the one provider a project declares, or the
// reason there is none to use: `skip` for a project whose declarations are
// correct and simply do not reach one, `refusal` for declarations that cannot
// be acted on.
//
// THE CALLER NAMES WHAT IT WANTED THE WORLD FOR, and that is not decoration.
// The two readings are shared; the sentences are not. A refusal reading "nothing
// says which owns the world a stage uses" would send a reader of a shrink
// refusal looking for a stage, and the arm that holds the shrink's own wording
// said so the moment this was generalized: it asserts "nothing says which owns
// the world a shrink replays in" by name. A message with two consumers has to
// stay specific to each, or the dedup has cost its readers the one clause that
// tells them where to look.
//
// NO PROVIDER IS NEITHER A REFUSAL NOR AN ERROR. docs/design.md requires a
// replay to run "on a verified clean rollback" and a fan-out to contend one
// world, so a project declaring no provider cannot be shrunk or contended at
// all -- and most of the examples declare none on purpose. The caller prints
// the reason, because a reader who expected a minimal trace or a contention
// needs to know why there is none.
//
// MORE THAN ONE IS A REFUSAL AND NOT A FIRST-WINS. Which world a stage uses is
// the project's statement, and `covers` names components rather than ranking
// providers, so nothing in the document says which to take. Picking the first
// would be this code inventing a precedence the format does not have.
//
// A DECLARED PROVIDER THIS BUILD DOES NOT REGISTER IS A CARRIED NAME, NOT A
// FAILURE (fable, coop rumor/1202, on rook's /1197). examples/twin declares
// `doc-store-truncate` and says in its own comment that the name is carried AS
// DECLARED until docs/rollback.md defines the interface in R13, so the contract
// permits a name nothing can open yet. The first cut of the shrink sent that
// through the could-not-run path and printed an error on stderr for a shipped
// R4 example behaving exactly as its document says.
//
// IT IS CHECKED BEFORE Open AND NOT FROM Open's ERROR, because an error string
// is not a state: "not registered" and "registered but would not start" are
// different facts and only the first is the contract's.
func soleProvider(prep platform.Report, purpose string) (name, skip, refusal string) {
	if len(prep.Platform.Rollback) == 0 {
		return "", fmt.Sprintf("the project declares no [[rollback]] provider, so there is no %s",
			purpose), ""
	}
	if len(prep.Platform.Rollback) > 1 {
		names := make([]string, 0, len(prep.Platform.Rollback))
		for _, rb := range prep.Platform.Rollback {
			names = append(names, rb.Provider)
		}
		return "", "", fmt.Sprintf("the project declares %d rollback providers (%s) and nothing "+
			"says which owns %s", len(names), strings.Join(names, ", "), purpose)
	}
	name = prep.Platform.Rollback[0].Provider
	registered := rollback.Registered()
	for _, r := range registered {
		if r == name {
			return name, "", ""
		}
	}
	sort.Strings(registered)
	return "", fmt.Sprintf("the project declares the rollback provider %q, which this build does "+
		"not register (%s): the name is carried as declared until the interface is defined",
		name, strings.Join(registered, ", ")), ""
}

// providerAddress answers whether the adapter can be pointed at the world this
// provider owns, and names the reason when it cannot.
//
// IT IS A FUNCTION SO THE THREE REFUSALS HAVE A FIXTURE. No committed project
// can reach them: the product registers exactly one provider and it implements
// both methods, and examples/toy passes TOY_ADDR through, so a refusal driven
// only from the CLI would be a refusal nothing trips -- arm 19's shape, which I
// named at coop r6/148 rather than leave for the review. A provider is an
// interface value, so a stub missing a method is expressible in a test and not
// in a project document.
//
// THE ENV CHECK IS HERE WITH THEM BECAUSE IT IS THE SAME QUESTION ONE STEP ON:
// a provider can own an address and name its variable, and the project can
// still fail to pass that variable to the adapter -- docs/testing-methodology.md
// records exactly that defect, where an `env` list omitting TOY_ADDR meant the
// value could never reach the adapter, so it started a system of its own and
// every candidate ran against a world the provider did not control. A
// reproduction that proves nothing, with nothing to notice.
//
// IT RETURNS THE REASON RATHER THAN PRINTING IT, so the caller owns the one
// "shrink: REFUSED" prefix and an arm can read the words without a stdout.
func providerAddress(prov any, name string, env []string) (interface{ Addr() string }, interface{ AddrEnv() string }, string) {
	at, ok := prov.(interface{ Addr() string })
	if !ok {
		return nil, nil, fmt.Sprintf("%s does not expose the address of the world it owns, so "+
			"the adapter cannot be pointed at it and every candidate would run against a "+
			"system this provider cannot put back", name)
	}
	ae, ok := prov.(interface{ AddrEnv() string })
	if !ok {
		return nil, nil, fmt.Sprintf("%s does not name the environment variable its adapter "+
			"reads the address from, and guessing one out of the adapter's env list would be "+
			"this code choosing a convention", name)
	}
	for _, e := range env {
		if e == ae.AddrEnv() {
			return at, ae, ""
		}
	}
	return nil, nil, fmt.Sprintf("%s owns its world's address and reads it from %s, which "+
		"[adapter] env does not pass through (%v): the adapter would start a system of its own "+
		"and the candidates would not be replays", name, ae.AddrEnv(), env)
}
