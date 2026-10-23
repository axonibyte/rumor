// Package schedule owns the run's only source of chance.
//
// WHY A PACKAGE AND NOT internal/engine, which the name "the engine owns the
// seed" points at. `engine` is the EVALUATOR -- Input, Evaluate, Accepts,
// Finding -- deciding whether a statute's rules fire against a sealed ledger.
// What design.md's section names is the WALK, which shares no state with the
// evaluator and did not exist until R5. Measured at v0.1.21 before choosing
// the name: no PRNG, no seed and no action walk anywhere under internal/, and
// `engine`'s three "walk" mentions are an AST walk and a fact walk. Reversible
// while nothing imports this, and named apart so a reader looking for the walk
// is not sent to the evaluator.
//
// ONE SOURCE, AND THE SEED IS ALL OF IT. docs/design.md, "The engine owns the
// seed": "Everything the run decides by chance descends from that integer
// through one pseudo-random generator the engine holds ... Nothing else in the
// system draws randomness. Drivers and adapters are handed one action at a
// time ... they are never given the generator." The reason is replay -- a run
// must be reproducible from the seed, the weight file, the bindings and the
// project's commit alone -- so a driver that drew its own randomness would
// make the replay depend on something the seed does not carry, and R5.3's
// shrinker would be shrinking a trace it could not re-run.
//
// WHY math/rand/v2's PCG RATHER THAN v1's Source. v1 documents that its
// generator's sequence is not guaranteed across releases; a replay that
// depends on the toolchain's version is not a replay, and fable's D1 compares
// two attempts of ONE engine while D11's promoted seed must still reproduce a
// finding months later. PCG is a fixed algorithm with a two-word seed. Stdlib
// only: R4.1 added this tree's second direct dependency and docs/design.md
// carries the reason, so a third needs its own.
//
// WHERE ENTROPY ENTERS, exactly once and never during a run: Mint, for the
// case where no `--seed` was given. New is pure. A run therefore has one
// integer to print first and accept back, which is what D2 reads.
package schedule

import (
	cryptorand "crypto/rand"
	"encoding/binary"
	"fmt"
	"math/rand/v2"
	"sort"
)

// Source is the run's generator. One per run, held by the walk, never handed
// to a driver or an adapter.
type Source struct {
	seed uint64
	rng  *rand.Rand
}

// New is the run's generator for a seed. Pure: two Sources from one seed draw
// the same sequence, which is the property D1 and D11 both rest on.
func New(seed uint64) *Source {
	// PCG TAKES TWO WORDS AND THE RUN DECLARES ONE. The second is a fixed
	// stream constant rather than a second seed: a run names ONE integer
	// (design.md, and `--seed` takes one), so deriving the high word from the
	// low one keeps the declared seed the whole of the run's entropy. Changing
	// this constant changes every run's sequence, which is why it is written
	// down rather than inlined.
	const stream = 0x9E3779B97F4A7C15 // the 64-bit golden-ratio odd constant
	return &Source{seed: seed, rng: rand.New(rand.NewPCG(seed, seed^stream))}
}

// Seed is the integer this Source descends from, so a run can print it first
// and accept it back (D2).
func (s *Source) Seed() uint64 { return s.seed }

// IntN draws in [0,n). It REFUSES n <= 0 rather than panicking as the stdlib
// does: a walk that has computed an empty applicable set has a finding to
// report about the project ("no action is applicable at this step"), not a
// crash to hand the operator, and a panic here would be indistinguishable
// from a defect in this package.
func (s *Source) IntN(n int) (int, error) {
	if n <= 0 {
		return 0, fmt.Errorf("schedule: a draw from %d candidates is not a choice: "+
			"an empty applicable set is the walk's finding to report, not a draw", n)
	}
	return s.rng.IntN(n), nil
}

// Uint64 draws a full word, for a caller that needs its own derived stream.
func (s *Source) Uint64() uint64 { return s.rng.Uint64() }

// Mint is the ONE place entropy enters, used when no `--seed` was given, and
// it happens before the walk starts rather than during it. crypto/rand rather
// than the clock: two runs started in the same second must not share a seed,
// and a seed derived from time is guessable in a way that makes a "random"
// corpus quietly narrow.
func Mint() (uint64, error) {
	var b [8]byte
	if _, err := cryptorand.Read(b[:]); err != nil {
		return 0, fmt.Errorf("schedule: cannot mint a seed, so no run can be "+
			"reproduced from one: %w", err)
	}
	return binary.LittleEndian.Uint64(b[:]), nil
}

// Candidate is one applicable action kind and the weight resolved for it from
// the project's platform.toml -- profile, then stage, then default, which is
// internal/platform's Weight and not this package's business.
type Candidate struct {
	Kind   string
	Weight int
}

// Pick draws one candidate in proportion to weight.
//
// IT SORTS, AND THAT IS NOT TIDINESS. fable's D1 ruling (coop breakout-r5/5)
// names the one way an identical walk yields reordered findings: "something
// unseeded between the walk and the verdict, and in Go that thing has a name,
// map iteration, which is randomized on purpose". A weighted draw that walked
// a map, or that trusted its caller's slice order, would pass a set comparison
// on every run and fail D1's sequence comparison on most of them. Sorting by
// kind makes the draw a function of the seed and the SET, so the caller cannot
// change the run by changing the order it happens to assemble candidates in.
//
// A ZERO WEIGHT IS NEVER DRAWN, which is D3's first half and the reason the
// cumulative walk uses a strict `<`: a project that writes 0 has said "never",
// not "rarely".
//
// EVERY WEIGHT ZERO IS AN EMPTY POOL, NOT A REFUSAL, ruled by fable at coop
// r5/121 and r5/124. The sentence here used to say refusal, and half of its
// reason still holds: a uniform fallback would run a walk the project asked not
// to have. The other half was wrong. D3's "never chosen" applies CANDIDATE BY
// CANDIDATE, so a zero-weight candidate is simply not a candidate -- and a step
// with nothing choosable is the no-op step the walk already knew how to record,
// not a fault. "A project that weights `tell` at 0 is a project that does not
// want tells, not a broken one."
//
// SO ZERO-WEIGHT CANDIDATES ARE DROPPED BEFORE THE QUESTION IS ASKED, and an
// empty result and an all-zero one are the same answer: `("", nil)`, meaning
// nothing was drawable. The caller records a no-op step, exactly as it does for
// a pool that was empty to begin with.
//
// WHAT MADE THIS REACHABLE. R5.4's honest `tell` is applicable to any actor
// holding any claim, so a step whose only candidate was `tell` gave a pool that
// was non-empty and entirely zero -- a state no project could reach before,
// which is why the old sentence had never been wrong in practice. A project
// weighting the kind at 0 then got an ABORTED walk where it had previously got
// a skipped step.
//
// AN EMPTY `candidates` NO LONGER REFUSES EITHER, and that is the same ruling
// rather than a check being dropped: the walk has always tested `len(pool) == 0`
// before calling, so the old refusal was unreachable from the only caller, and
// its contract -- "an empty applicable set is the walk's finding to report" --
// is what the no-op step IS.
func (s *Source) Pick(candidates []Candidate) (string, error) {
	// A NEGATIVE WEIGHT IS STILL A DEFECT and is still refused: internal/platform
	// rejects one at parse, so reaching here means a caller built it.
	ordered := make([]Candidate, 0, len(candidates))
	for _, c := range candidates {
		if c.Weight < 0 {
			return "", fmt.Errorf("schedule: %q carries weight %d: a negative weight is "+
				"refused by internal/platform at parse, so reaching here is a defect in "+
				"the caller rather than in the project", c.Kind, c.Weight)
		}
		if c.Weight == 0 {
			continue
		}
		ordered = append(ordered, c)
	}
	if len(ordered) == 0 {
		return "", nil
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Kind < ordered[j].Kind })

	total := 0
	for _, c := range ordered {
		total += c.Weight
	}

	n, err := s.IntN(total)
	if err != nil {
		return "", err
	}
	for _, c := range ordered {
		if n < c.Weight {
			return c.Kind, nil
		}
		n -= c.Weight
	}
	// UNREACHABLE WHILE total IS THE SUM, and it is said rather than ignored:
	// a silent fallthrough here would return the empty kind and the walk would
	// execute nothing while reporting a step.
	return "", fmt.Errorf("schedule: the cumulative draw fell past %d candidate(s) "+
		"summing to %d, which is a defect in this function", len(ordered), total)
}
