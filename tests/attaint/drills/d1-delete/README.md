# Drill D1 — DELETE

Designed by fable at `thread:breakout-r3/0`; built in R3.1 (issue 52).

## What is damaged

`examples/transitive-count/statute.rego`

The clause `finding := "explained" if { ... }` is deleted.

Only the changed file is committed here. `tests/attaint/drills_test.go` copies
the pristine `examples/` into a temp directory and lays this overlay over it, so
this directory is the diff. The harness refuses an overlay that ADDS a file
rather than replacing one, which is how a drifted path is caught rather than
silently testing nothing.

## The arm it trips

`finding-the-package-can-no-longer-produce`, and exactly that one. The harness asserts the refusal COUNT is 1
as well as the clause: a drill that trips a second arm is not the drill that was
designed, even when it still goes red.

## The exact refusal

```
REFUSED finding-the-package-can-no-longer-produce: transitive-count: a case expects the finding "explained" and no case in the package produces it from its ledger, so the clause that yields it is gone or unreachable
```

## Why this arm and not another

`charlie-explained` still has its ledger and still expects `explained`. With the
clause gone the statute falls through to `default finding := "bug"`, so the
package can no longer REACH a finding its own fixtures say it produces.

Note which arm does NOT catch this, because it is the reason the arm exists:
every rule is still moved by some mutant (`finding` by m2-ignores-knowledge), the
corroboration control still passes, and nothing is vacuous. Only reachability
notices that a capability has been removed.

## Running it

```
go test ./tests/attaint/ -run TestEveryDrillTripsItsOwnArm/d1-delete -v
```

Or against a tree by hand, after laying the overlay over a copy of `examples/`:

```
rumor attaint --examples <that tree>
```
