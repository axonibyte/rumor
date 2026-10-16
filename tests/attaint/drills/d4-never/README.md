# Drill D4 — NEVER

Designed by fable at `thread:breakout-r3/0`, re-cut to its ruled shape at
`thread:breakout-r3/2`; built in R3.1 (issue 52).

## What is damaged

`examples/planted-lie/ledger/with-planted.jsonl` and its `.seal.json`, and
`examples/planted-lie/cases/with-planted.json`

The tag **stays** and the declaration **stays**. What changes is who told whom:
the planted claim is now BOB's, at position 3, and alice is never told of it.
Alice still holds her own two honest claims, still believes 2 and still
observes 1, so the finding is still `bug` — and the lie sits in the sealed
prefix where no observation reaches it.

Only the changed files are committed here. `tests/attaint/drills_test.go`
copies the pristine `examples/` into a temp directory and lays this overlay
over it, so this directory is the diff. The harness refuses an overlay that
ADDS a file rather than replacing one.

## The arm it trips

`checker never fired`, and exactly that one.

## The exact refusal

```
REFUSED checker never fired: planted-lie/with-planted declares itself a corroboration control and its sealed prefix carries a planted claim, but the observation reaches none: the observer does not hold a planted claim of the observed type, so the control was armed and never evaluated
```

## Why this arm and not another

fable `/2` sets the distinction this drill exists for, and it is three
conditions where a careless tool would see one:

| | |
|---|---|
| **d3** | the control was EVALUATED and did not pass — the bug does not rest on the lie |
| **d4** | the control was ARMED and NEVER EVALUATED — nothing reached the lie |
| **d5** | no case declares a control at all — a vacuity, not a failed check |

Collapsing d4 into d3 would make a fixture nobody observes indistinguishable
from a statute that stopped attributing. Collapsing it into d5 would make a
missing observation indistinguishable from a missing control.

Reachability is read from `Finding.Input` — the document the engine actually
handed the statute — so the knowledge rule stays in `internal/flatten` and has
no second implementation in the checker.

## Running it

```
go test ./tests/attaint/ -run TestEveryDrillTripsItsOwnArm/d4-never -v
```
