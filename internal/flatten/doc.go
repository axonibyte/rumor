// Package flatten computes what an actor KNOWS at a position, from the
// one-hop `told` facts and the actor's own claims.
//
// WHY THIS LIVES OUTSIDE REGO, which the epic asks this module to say.
//
// The vocabulary's finding set turns on a distinction that needs TWO inputs:
// `explained` and `bug` differ by whether claims the observer was never told
// account for the gap. Against the observer's knowledge alone, Charlie
// observing 2 and Charlie observing 3 disagree identically; only the whole
// ledger separates them. So a statute is handed two things -- the observer's
// flattened knowledge, and the ledger -- and this package computes the first.
//
// It is not in Rego for three reasons, in the order they decide it:
//
//  1. REACHABILITY IS A GRAPH WALK, AND A RULE THAT FOLLOWED TELLS WOULD BE A
//     DIFFERENT STATUTE. The vocabulary is explicit: "Rules never follow
//     tells". If knowledge were derived inside the statute, every statute
//     would carry that derivation, and a statute that got it subtly wrong
//     would produce a finding that looks like a verdict about the system
//     under test while actually being a verdict about its own bookkeeping.
//     Keeping it out means a statute can be read as a claim about the world.
//  2. IT IS DATA TO THE QUESTION, NOT PART OF IT. `known` is an input the
//     cases already carry: every fixture under examples/ hands the statute a
//     `known` array. This package is the thing that will produce those arrays
//     from a ledger, so its output shape is pinned by fixtures that existed
//     before it did -- which is the epic's "the examples are the acceptance
//     fixtures and they exist before the engine does".
//  3. IT HAS AN EXTENSION POINT, AND REGO IS THE WRONG PLACE FOR ONE. Ancestry
//     facts (R18's) will let knowledge travel along a derivation chain rather
//     than only along a tell. That is a change to HOW KNOWLEDGE IS COMPUTED,
//     not to what any statute means, so it belongs where it can be swapped
//     without touching a statute or re-pinning a fixture hash.
//
// WHAT ONE HOP MEANS HERE, because it is the whole semantics: a tell moves
// knowledge from one actor to one other actor, and nothing is inferred from
// it. If Alice tells Bob about a claim and Bob tells Charlie about the same
// claim, Charlie knows it -- because of Bob's tell, which is its own fact, not
// because knowledge was chased through Bob. With no tell to Charlie, Charlie
// does not know it however many hops away it sits. That is why the untold
// claim is this package's first test.
package flatten
