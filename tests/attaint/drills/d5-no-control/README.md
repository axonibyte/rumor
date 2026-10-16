# Drill D5 — NO CONTROL (the vacuity)

Split out of D4 by fable at `thread:breakout-r3/2`; built in R3.1 (issue 52).

This was the first cut of D4. fable ruled that D4 proper is the control ARMED
AND NEVER FIRED, and that "no case declares a control at all" is a third and
weaker condition — a vacuity rather than a failed check — so it keeps its own
drill and its own clause.

## What is damaged

`examples/planted-lie/ledger/with-planted.jsonl and examples/planted-lie/cases/with-planted.json`

The tag is removed from the ledger AND the case stops declaring itself a corroboration control, so no case anywhere in the corpus expects `rests_on_planted: true`.

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
