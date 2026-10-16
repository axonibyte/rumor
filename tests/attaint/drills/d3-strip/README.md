# Drill D3 — STRIP

Designed by fable at `thread:breakout-r3/0`; built in R3.1 (issue 52).

## What is damaged

`examples/planted-lie/ledger/with-planted.jsonl`

The `"planted":true` tag is removed from the one planted claim. The claim itself stays, and `cases/with-planted.json` is untouched -- it still expects `rests_on_planted: true`.

Only the changed file is committed here. `tests/attaint/drills_test.go` copies
the pristine `examples/` into a temp directory and lays this overlay over it, so
this directory is the diff. The harness refuses an overlay that ADDS a file
rather than replacing one, which is how a drifted path is caught rather than
silently testing nothing.

## The arm it trips

`bug-does-not-rest-on-the-planted-claim`, and exactly that one. The harness asserts the refusal COUNT is 1
as well as the clause: a drill that trips a second arm is not the drill that was
designed, even when it still goes red.

## The exact refusal

```
REFUSED bug-does-not-rest-on-the-planted-claim: planted-lie/with-planted expects rests_on_planted=true but its sealed prefix carries no planted claim, so there is no lie for the finding to rest on
```

## Why this arm and not another

The finding does NOT change: `bug` is driven by what the observer believes, which
the tag does not affect. docs/vocabulary.md says so under `planted` -- "with the
tag removed from every planted claim but the CLAIM still present, the same run
must still produce the same `bug`" -- and examples/planted-lie/cases/stripped-tag
is the committed case that asserts it.

So the strip is not caught by the finding moving. It is caught by the control
being unable to pass: the case declares this bug rests on a lie, and the sealed
prefix holds no lie for it to rest on.

THE UN-STRIPPED TWIN is the pristine tree, which d0-control runs: there
`with-planted` corroborates and attaint passes. Same case, same statute, one tag.

## Running it

```
go test ./tests/attaint/ -run TestEveryDrillTripsItsOwnArm/d3-strip -v
```

Or against a tree by hand, after laying the overlay over a copy of `examples/`:

```
rumor attaint --examples <that tree>
```
