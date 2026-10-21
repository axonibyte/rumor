package shrink

import (
	"context"
	"fmt"

	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
	"forge.axonibyte.com/axonibyte/rumor/internal/rollback"
)

// Reduction is what a bisection established.
type Reduction struct {
	// Steps is the length of the shortest PROBED prefix that reproduced. It is
	// n when nothing shorter did.
	Steps int

	// Probes counts the candidate traces actually run, each on its own
	// verified clean world. It is in the result because the cost of a shrink
	// is the number of worlds it rebuilt, not the number of steps it removed,
	// and because a bisection that had degenerated into a linear scan would
	// otherwise be invisible.
	Probes int

	// Found says a prefix strictly shorter than n reproduced.
	Found bool
}

// Bisect looks for a shorter prefix of a failing trace that still reproduces
// the finding.
//
// THE ORDER IS design.md's: "length bisection first, then removal of whole
// action kinds". This is the length half, so the candidates are PREFIXES -- a
// subsequence is the kind-removal step's business and not this one's.
//
// EVERY CANDIDATE RUNS ON A VERIFIED CLEAN WORLD, through Once, which is the
// composition the gate exists for: "a shrink step run on a world the previous
// step dirtied would attribute to the trace what the leftover state did, and
// the 'minimal' trace would be minimal only in that world". So a dirty world
// stops the bisection rather than being skipped past -- a shrink that carried
// on would be reporting lengths measured in different worlds.
//
// IT RETURNS A LOCAL MINIMUM AND THE DOCUMENT ALREADY ALLOWS THAT. Reproduction
// is NOT monotonic in length: a prefix of three steps is a different world from
// the first three steps of a ten-step run, not a subset of its behaviour, so a
// trace can reproduce at 5, fail at 3 and reproduce again at 1. A binary search
// over a non-monotonic predicate therefore returns a length that reproduced
// whose probed neighbour did not, which is a local minimum and not the global
// one. design.md: "The result is written as a minimal trace, which names the
// output's SHAPE (the actions, their positions, their handles and the rules
// that fired) and not a property of it". The honest claim is "this trace is
// short and it reproduces", never "no shorter trace exists".
//
// WHY IT PROBES n ITSELF rather than trusting the caller's gate. Once has
// usually just established that the whole trace reproduces, so this first probe
// is a duplicate of it -- one world out of the log(n) a bisection rebuilds. It
// is spent anyway for Verify's own stated reason: "a caller that skipped it
// would be assuming what it came to check". A bisection whose upper bound does
// not actually reproduce searches for a shorter version of a failure that is
// not there, and every length it reports would be meaningless.
func Bisect(ctx context.Context, pr rollback.Provider, cp rollback.Checkpoint,
	want engine.Finding, n int,
	run func(ctx context.Context, steps int) (engine.Finding, error)) (Reduction, error) {

	if n < 1 {
		return Reduction{}, fmt.Errorf("shrink: a trace of %d step(s) cannot be bisected: there is "+
			"nothing to shorten and no candidate to run", n)
	}
	if run == nil {
		return Reduction{}, fmt.Errorf("shrink: no run function, so no candidate trace can be replayed")
	}

	var probes int
	reproduces := func(k int) (bool, error) {
		probes++
		v, err := Once(ctx, pr, cp, want, func(c context.Context) (engine.Finding, error) {
			return run(c, k)
		})
		if err != nil {
			return false, fmt.Errorf("shrink: the %d-step candidate: %w", k, err)
		}
		return v.Reproduced, nil
	}

	ok, err := reproduces(n)
	if err != nil {
		return Reduction{Probes: probes}, err
	}
	if !ok {
		return Reduction{Probes: probes}, fmt.Errorf("shrink: the whole %d-step trace did not "+
			"reproduce on a clean world, so there is no failure to shorten: the replay gate is what "+
			"establishes this, and a seed it calls unstable is not shrunk", n)
	}

	// hi ALWAYS REPRODUCES and lo is the shortest length not yet ruled out.
	// The invariant is what makes the answer meaningful when the search stops:
	// whatever hi holds has been RUN and reproduced, so the result is never a
	// length that was only inferred.
	lo, hi := 1, n
	for lo < hi {
		mid := lo + (hi-lo)/2
		ok, err := reproduces(mid)
		if err != nil {
			return Reduction{Probes: probes}, err
		}
		if ok {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return Reduction{Steps: hi, Probes: probes, Found: hi < n}, nil
}
