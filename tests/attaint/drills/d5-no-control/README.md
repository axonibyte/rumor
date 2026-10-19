# Drill D5 — NO CONTROL (the vacuity)

Split out of D4 by fable at `thread:breakout-r3/2`; built in R3.1 (issue 52).

This was the first cut of D4. fable ruled that D4 proper is the control ARMED
AND NEVER FIRED, and that "no case declares a control at all" is a third and
weaker condition — a vacuity rather than a failed check — so it keeps its own
drill and its own clause.

## What is damaged

`every package's planted ledger and case` — currently
`examples/planted-lie/{ledger,cases}/with-planted.*`,
`examples/twin/{ledger,cases}/planted-publish.*` and
`examples/toy/{ledger,cases}/planted-join.*`

In each, the tag is removed from the ledger AND the case stops declaring itself
a corroboration control, so no case anywhere in the corpus expects
`rests_on_planted: true`.

## Why it strips EVERY package and not one

It stripped planted-lie alone until R4.3, and that was the drill's ASSUMPTION
rather than its meaning (fable, coop r4/65): planted-lie was simply the only
package that had a control when this drill was written. The twin earned one in
R4.3, its control survived a one-package overlay, the corpus still held a
control, and attaint rightly did not refuse — so the drill produced 0 refusals
where it wants exactly 1. The sentence being drilled is about the CORPUS, so the
overlay is corpus-wide and the expected outcome is unchanged at one refusal.

A per-package guard was refused instead: it would make every package carry a lie
to be allowed in.

Only the changed file is committed here. `tests/attaint/drills_test.go` copies
the pristine `examples/` into a temp directory and lays this overlay over it, so
this directory is the diff. The harness refuses an overlay that ADDS a file
rather than replacing one, which is how a drifted path is caught rather than
silently testing nothing.

## The arm it trips

`no-case-declares-a-corroboration-control`, and exactly that one. The harness asserts the refusal COUNT is 1
as well as the clause: a drill that trips a second arm is not the drill that was
designed, even when it still goes red.

## The exact refusal

```
REFUSED no-case-declares-a-corroboration-control: no case in any of the 3 package(s) declares rests_on_planted=true, so the corroboration control was never exercised; 3 case(s) carry a planted claim and 1 package(s) declare the rule
```

## Why this arm and not another

THE VACUITY THIS WHOLE TOOL EXISTS FOR. "Every planted lie in a fixture produces
`bug`" is TRUE of a corpus with no corroboration control in it -- there is no
counterexample because there is no example. Without this arm, removing the
control is the cheapest way to make attaint green.

The refusal reports what it DID find beside what it did not: three cases still
carry a planted claim and one package still declares the rule, so the reader can
tell "the control was removed" from "this corpus never had one".

## Running it

```
go test ./tests/attaint/ -run TestEveryDrillTripsItsOwnArm/d5-no-control -v
```

Or against a tree by hand, after laying the overlay over a copy of `examples/`:

```
rumor attaint --examples <that tree>
```

## Widened again in R5.2, for the reason it was widened in R4.3

`examples/toy` earned a control of its own — `planted-join`, where a join the
engine planted makes the observer believe two where the store holds one — so
this drill had to strip it too. Without that it produced ZERO refusals: every
other package's control was gone and the toy's still stood, so no clause was
reached and the drill passed while testing nothing. That is the same failure
R4.3 fixed for the twin, and it is the reason this file names the packages
rather than saying "the planted ones": a drill whose scope lives only in a
glob cannot tell you it has stopped covering something.

The overlay is generated from the package's own source alongside the cases, so
the stripped copy cannot drift from what it strips.
