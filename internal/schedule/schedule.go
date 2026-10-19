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
