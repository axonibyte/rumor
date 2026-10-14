package flatten

import (
	"fmt"
	"sort"
)

// Claim is the part of a ledger claim this package reads: who made it, what it
// is called, and where it sits.
//
// DELIBERATELY NOT internal/ledger's TYPE. R2.1 and R2.2 run in parallel and
// its store is unmerged, so importing it would make this package unbuildable
// until that lands -- and would couple the knowledge rule to a storage shape.
// These three fields are the whole of what flattening needs; the adapter that
// feeds them from a sealed ledger belongs to R2.3, which has both in hand.
type Claim struct {
	Ref      string // the claim's reference, e.g. "alice.user#1"
	Actor    string // the actor who made it
	Position int    // where it was appended
}

// Told is one hop: Actor passed a reference to Recipient at Position.
type Told struct {
	Actor     string // the teller
	Recipient string // the actor who now knows the claim
	ClaimRef  string // a REFERENCE, not a payload: the vocabulary's adopted form
	Position  int
}

// Ledger is the prefix a check reads. Nothing here is sealed or ordered by
// this package: the caller hands in the facts it wants considered.
type Ledger struct {
	Claims []Claim
	Tells  []Told
}

// Ancestry is THE EXTENSION POINT, and it is an interface rather than a
// comment so that a test can supply one.
//
// R18's ancestry facts let knowledge travel along a derivation chain: knowing
// a claim implies knowing the claims it was derived from. That is a change to
// how knowledge is computed and to nothing else, which is why it enters here
// and not in a statute. A nil Ancestry means one-hop only, which is v1.
type Ancestry interface {
	// Parents returns the refs the given claim derives from. It is called
	// only for refs already known, and its result is closed over -- a parent
	// of a parent is reached, because ancestry is a chain by definition. A
	// cycle is handled by the caller (this package) and not by the provider.
	Parents(ref string) []string
}

// Known returns the claim refs `actor` knows at position `at`, sorted.
//
// The rule, from the vocabulary's Positions heading: "every `claim` the
// observer made and every `told` it received at positions at or before p".
// With an Ancestry, the ancestors of those refs as well.
//
// SORTED, because this output is compared: the fixtures carry `known` as an
// array and a map-iteration order would make a passing check depend on the
// run. The error cases refuse rather than guess, since a malformed fact here
// would silently narrow what an actor is held to know -- which turns a `bug`
// into an `explained` and hides a real finding.
func Known(l Ledger, actor string, at int, anc Ancestry) ([]string, error) {
	if actor == "" {
		return nil, fmt.Errorf("flatten: no actor given, and knowledge is always some actor's")
	}
	if at < 1 {
		return nil, fmt.Errorf("flatten: position %d is not a position: v1 assigns positions from 1 upward", at)
	}

	known := map[string]bool{}
	for i, c := range l.Claims {
		if c.Ref == "" || c.Actor == "" || c.Position < 1 {
			return nil, fmt.Errorf("flatten: claim %d is incomplete (ref %q, actor %q, position %d): every fact carries a position and an actor", i, c.Ref, c.Actor, c.Position)
		}
		if c.Actor == actor && c.Position <= at {
			known[c.Ref] = true
		}
	}
	for i, t := range l.Tells {
		if t.Actor == "" || t.Recipient == "" || t.ClaimRef == "" || t.Position < 1 {
			return nil, fmt.Errorf("flatten: tell %d is incomplete (actor %q, recipient %q, ref %q, position %d)", i, t.Actor, t.Recipient, t.ClaimRef, t.Position)
		}
		// THE STORE REFUSES THIS TOO, AND THE DUPLICATION IS THE CHECK: a
		// self-tell is meaningless (an actor cannot inform itself) and would
		// quietly widen knowledge if it ever reached here from a file that
		// bypassed the store.
		if t.Actor == t.Recipient {
			return nil, fmt.Errorf("flatten: tell %d is addressed to its own actor %q, which tells nobody anything", i, t.Actor)
		}
		if t.Recipient == actor && t.Position <= at {
			known[t.ClaimRef] = true
		}
	}

	if anc != nil {
		if err := closeOverAncestry(known, anc); err != nil {
			return nil, err
		}
	}

	out := make([]string, 0, len(known))
	for ref := range known {
		out = append(out, ref)
	}
	sort.Strings(out)
	return out, nil
}

// closeOverAncestry adds the ancestors of everything known, to a fixed point.
//
// A CYCLE IS THIS PACKAGE'S PROBLEM, NOT THE PROVIDER'S. An ancestry provider
// reading facts from a ledger can be handed a cycle by a malformed fixture,
// and a naive walk would not return. The visited set makes the walk terminate
// on any graph, and the bound is the number of distinct refs.
func closeOverAncestry(known map[string]bool, anc Ancestry) error {
	queue := make([]string, 0, len(known))
	for ref := range known {
		queue = append(queue, ref)
	}
	sort.Strings(queue) // so a provider's call order is reproducible too
	seen := map[string]bool{}
	for len(queue) > 0 {
		ref := queue[0]
		queue = queue[1:]
		if seen[ref] {
			continue
		}
		seen[ref] = true
		for _, parent := range anc.Parents(ref) {
			if parent == "" {
				return fmt.Errorf("flatten: ancestry of %q names an empty parent", ref)
			}
			if !known[parent] {
				known[parent] = true
			}
			if !seen[parent] {
				queue = append(queue, parent)
			}
		}
	}
	return nil
}
