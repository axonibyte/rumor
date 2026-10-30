package concurrency

import (
	"fmt"
	"strings"

	"forge.axonibyte.com/axonibyte/rumor/internal/schedule"
)

// Plan is the fan-out a SEED chose: which session issues each request, in issue
// order. It carries no clock and no address, which is the whole point of its
// existing separately from the attempts it produces.
//
// WHY A PLAN AT ALL, which rook named at coop r8/36 against the head before
// this one. D1's "two attempts of one seed are byte-identical" is not a claim
// that the system is deterministic; it is ENFORCED by keeping the hashed stream
// clock-free and sorted (cmd/rumor/run.go:502). A concurrency stage whose only
// contribution to that stream were a clock-free "did overlap" would put ONE BIT
// there -- and seed+1 cannot move one bit, so byte-identity would hold for
// every seed and D9 would be a frozen oracle: an arm that cannot fail.
//
// THE PLAN IS WHAT SEED+1 MOVES. Issue order over the sessions is drawn from
// schedule.Source, the run's own PCG generator -- "two Sources from one seed
// draw the same sequence, which is the property D1 and D11 both rest on" --
// so the same seed renders the same plan and a different seed renders a
// different one. That gives D9 a subject with more than one bit in it.
type Plan struct {
	// Issue[k] is the session that carries the k-th request issued.
	Issue []int
}

// PlanFor is the fan-out for a seed over `width` sessions: every session
// carries exactly one request, and the SEED chooses the order they are issued
// in.
//
// EVERY SESSION ONCE, which is a property of this stage rather than a general
// truth: the wire allows one outstanding request per process, so a session
// carrying two requests would serialize them against itself and the second
// would not be part of the contention. A permutation is therefore the whole
// space of fan-outs at a given width.
//
// AND THE ORDER MATTERS EVEN THOUGH THE RELEASE IS SIMULTANEOUS. The goroutines
// are released from one closed channel, but which session's request was
// WRITTEN first is still a fact about the attempt, and it is the fact a replay
// has to reproduce; a plan that recorded only the width would make two runs
// with different issue orders byte-identical and hide the difference from D9.
func PlanFor(seed uint64, width int) (Plan, error) {
	if width < 2 {
		return Plan{}, fmt.Errorf("a fan-out of %d is not a fan-out: one request in flight "+
			"exercises the sequential path, so there is no plan to draw", width)
	}
	src := schedule.New(seed)
	issue := make([]int, 0, width)
	for i := 0; i < width; i++ {
		issue = append(issue, i)
	}
	// FISHER-YATES THROUGH THE RUN's OWN GENERATOR, so the draw count is a
	// function of the width alone and the sequence is the seed's.
	for i := width - 1; i > 0; i-- {
		j, err := src.IntN(i + 1)
		if err != nil {
			return Plan{}, fmt.Errorf("drawing the issue order at %d: %w", i, err)
		}
		issue[i], issue[j] = issue[j], issue[i]
	}
	return Plan{Issue: issue}, nil
}

// Width is how many requests this plan puts in flight.
func (p Plan) Width() int { return len(p.Issue) }

// Render is the plan's canonical line for the stream the control hashes.
//
// CLOCK-FREE BY CONSTRUCTION AND NOT BY PROMISE: there is nothing in a Plan to
// print but session indices, which is why the type holds no times and no
// addresses. The arm calls this twice with a sleep between and requires the
// same bytes, so a clock that leaked in later would be caught rather than
// argued about.
func (p Plan) Render() string {
	parts := make([]string, 0, len(p.Issue))
	for _, s := range p.Issue {
		parts = append(parts, fmt.Sprint(s))
	}
	return fmt.Sprintf("fan-out of %d, issued %s", len(p.Issue), strings.Join(parts, " then "))
}
