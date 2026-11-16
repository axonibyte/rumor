package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/open-policy-agent/opa/v1/rego"
)

// THE HOIST, R10 d1. `rego.New(...).Eval(ctx)` is OPA's one-shot call: it
// parses and compiles the module every time it is invoked. The evaluator used
// it per rule per case, so an attaint pass compiled the same handful of
// statutes once for every (mutant, case, rule) triple it visited. The compile
// is the expensive half and the module does not change between those visits.
//
// PrepareForEval does the parse and compile once and hands back a query that
// can be evaluated repeatedly with different inputs. Input therefore moves
// from a New option to an Eval option, and so do the instruments: a tracer
// attached at prepare time would be shared by every evaluation that reused the
// query, which is precisely the bug that would make one finding's trace show
// up under another's.
//
// WHAT THE KEY IS AND WHY IT IS A DIGEST. A prepared query is specific to the
// module AND the query, so the key is both. The statute is held as a sha256 of
// its source rather than the source itself: attaint evaluates a few kilobytes
// of Rego against many cases, and keying on the text would put a copy of every
// mutant in this map for the life of the process. The digest is 64 bytes and
// collides only where the sources are identical, which is when sharing the
// query is correct anyway.
//
// WHAT BOUNDS THE MAP. One entry per (distinct statute, rule) pair the process
// evaluates -- for attaint, (mutants + 1) x (rules + 1). It is not an LRU and
// has no eviction, because a statute that is evaluated once is evaluated at
// most a few more times and the entries are small; a long-lived process that
// compiled unbounded distinct statutes would need one, and does not exist in
// this tree. If one appears, this is the comment that is wrong.
//
// THE COUNTER MOVES WITH THE COMPILE, which is the invariant compiles.go
// states: RecordCompile is called here, on the miss path that actually
// prepares, and NOT at the eval. An increment left at the eval would report
// the hoisted tree still compiling per evaluation and make the hoist look
// inert; an increment left at both would double-count the first visit.
type preparedKey struct {
	statute string
	rule    string
}

var (
	preparedMu sync.RWMutex
	preparedQ  = map[preparedKey]rego.PreparedEvalQuery{}
)

// statuteDigest names a statute by its content, so two identical sources share
// one prepared query and two different ones never do.
func statuteDigest(statute string) string {
	sum := sha256.Sum256([]byte(statute))
	return hex.EncodeToString(sum[:])
}

// preparedFor returns the compiled query for one rule of one statute,
// compiling it on first use.
func preparedFor(ctx context.Context, statute, rule string) (rego.PreparedEvalQuery, error) {
	key := preparedKey{statute: statuteDigest(statute), rule: rule}

	preparedMu.RLock()
	pq, ok := preparedQ[key]
	preparedMu.RUnlock()
	if ok {
		return pq, nil
	}

	preparedMu.Lock()
	defer preparedMu.Unlock()
	// Looked up again under the write lock. Two callers can miss together, and
	// without this the loser would compile a second time -- wasting the work
	// and, worse, making the compile count depend on a race, so the figure this
	// head exists to produce would not be reproducible.
	if pq, ok = preparedQ[key]; ok {
		return pq, nil
	}

	RecordCompile()
	fresh, err := rego.New(
		rego.Query(StatutePackage+"."+rule),
		rego.Module(moduleName, statute),
	).PrepareForEval(ctx)
	if err != nil {
		return rego.PreparedEvalQuery{}, fmt.Errorf("engine: preparing rule %s: %w", rule, err)
	}
	preparedQ[key] = fresh
	return fresh, nil
}

// resetPrepared empties the cache. For arms that need a cold cache to count
// compiles: the count is per-process, so a test that did not clear this would
// measure whichever tests ran before it.
func resetPrepared() {
	preparedMu.Lock()
	defer preparedMu.Unlock()
	preparedQ = map[preparedKey]rego.PreparedEvalQuery{}
}
