// Package engine evaluates one check against a sealed ledger (R2.3, issue 44).
//
// WHAT IT IS FOR. R2.1 built the fact store and R2.2 built the knowledge
// flattening. Neither produces a verdict. This package is the seam between
// them and a statute: it reads a sealed ledger, flattens the observer's
// knowledge through internal/flatten, hands both to the statute as the
// evaluation's input, and returns the finding together with the rule
// definition that produced it and the evaluation trace behind it.
//
// THE ADAPTER LIVES HERE AND NOT IN internal/flatten, deliberately. R2.2 kept
// internal/ledger out of its types (see flatten.Claim) so the two runs could
// land in parallel, and fable breakout-r2/4 put the adapter in this run. So
// this is the only file in the tree that converts a ledger.Fact into a
// flatten.Claim, and that conversion is this package's main risk surface --
// which is why engine_test.go drives it off the committed fixtures rather than
// off a struct it built itself.
//
// WHERE THE FACTS ARRIVE, and the reason is a constraint rather than a
// preference. Issue 44 words the architecture as "data = sealed ledger +
// flattened facts; input = one check". OPA has two namespaces of those names,
// and the obvious reading would put the prefix in `data` and the observation in
// `input`. The three v1 statutes read `input.claims` and `input.known`: they
// are the R1 examples, pinned by content hash in tests/statute, carried byte
// for byte from r1.2-examples, and R1 closed gated over them. Moving the facts
// to OPA's `data` would edit all three. So the engine assembles ONE input
// document -- the observation under `observation`, the sealed prefix's claims
// under `claims`, the flattened knowledge under `known` -- and the issue's
// sentence is satisfied in the sense that matters: every fact in it comes from
// the sealed prefix and nothing else, and the check is the one thing supplied
// from outside. Whichever phase next edits the statutes is where the namespace
// split belongs; asked on thread:breakout-r2 rather than decided quietly here.
//
// THE OBSERVATION IS THE INPUT AND THE PREFIX IS THE DATA, which is the
// reading ratchet adopted in R2.1 and fable ratified at breakout-r2/13: an
// observation is not a fact in the sealed prefix. That is why Position below
// is derived from the seal instead of being carried on the Check -- a caller
// cannot place its observation inside the data it is judged against.
//
// WHAT THIS PACKAGE DOES NOT DO. It does not decide WHICH checks to run; a
// run or a reconciliation pass does that. It does not write to the ledger. It
// holds no clock of its own: time, where a statute needs it, is a clock fact
// in the prefix (docs/vocabulary.md, Time).
package engine
