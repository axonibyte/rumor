# Drill D2 — WEAKEN

Designed by fable at `thread:breakout-r3/0`; built in R3.1 (issue 52).

## What is damaged

`examples/planted-lie/statute.rego`

The line `not c.planted` is removed from `honest_total`, so a planted claim is counted as honest. That is exactly the shape of the committed mutant `m5-honest-counts-planted`.

Only the changed file is committed here. `tests/attaint/drills_test.go` copies
the pristine `examples/` into a temp directory and lays this overlay over it, so
this directory is the diff. The harness refuses an overlay that ADDS a file
rather than replacing one, which is how a drifted path is caught rather than
silently testing nothing.

## The arm it trips

`mutant-the-statute-has-absorbed`, and exactly that one. The harness asserts the refusal COUNT is 1
as well as the clause: a drill that trips a second arm is not the drill that was
designed, even when it still goes red.

## The exact refusal

```
REFUSED mutant-the-statute-has-absorbed: planted-lie/m5-honest-counts-planted moves no rule on any of the 4 case(s) on either path: the statute now computes what this mutant computes, so the statute has been weakened into it
```

## Why this arm and not another

THIS IS THE DRILL THAT FOUND A HOLE. The first build of attaint passed it: the
finding is still `bug`, the control still passes, every rule is still moved by
some other mutant, and no finding became unreachable. A weakened statute looked
exactly like a healthy one.

What catches it is the mutation pass's other half -- not "is every rule attacked
by some mutant" but "does every mutant still attack". When the statute is
loosened into m5, m5 stops being able to tell itself apart from the statute, and
a mutant that moves nothing is the statute having drifted into it.

It is NOT the declared-equivalence case from tests/engine. Those three position
mutants are equivalent on the LEDGER path only and still move their rules from
the cases, so they move something and are not dead.

## Running it

```
go test ./tests/attaint/ -run TestEveryDrillTripsItsOwnArm/d2-weaken -v
```

Or against a tree by hand, after laying the overlay over a copy of `examples/`:

```
rumor attaint --examples <that tree>
```
