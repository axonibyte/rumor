# The toy system under test

R5.2, issue 73. The small deliberately buggy application the engine is proven
against — and the only package under `examples/` that is a **running program**
rather than a ledger fixture.

    cmd/toy            a JSON API and a server-rendered UI over users, events,
                       roles and capacity; the whole world in one file
    cmd/toy-adapter    R4's wire contract on stdio, driving the toy over HTTP
    examples/toy       this project: bindings, platform, statute, cases, mutants

## Why it is a process

The rollback provider R5.3 consumes restarts the system under test, and a
restart provider cannot restart a library. So the engine starts `toy-adapter`,
the adapter starts `toy`, and everything the adapter knows about the world it
learns over HTTP — the way it would learn about a system it did not ship with.

All state is in ONE file, sessions included, which is the whole of the rollback
design: a checkpoint is a copy of that file, a restore is a copy back, and
`verify` READS it rather than assuming the restore worked.

## The three bugs, and which of them R5 can catch

The epic's R5 ledger rows, each a defect a seed can reach:

| bug | caught by | where it lives |
|---|---|---|
| API visibility leak | api-sim | the count endpoint, planted in C4 |
| capacity race | concurrency (**R8**) | `joinEvent`, planted in C1 |
| stale count after delete | api-sim | the denormalized counter, planted in C4 |

**R5 plants the race and R8 catches it.** One action at a time cannot race, and
the row's catcher is the `concurrency` stage, which is R8's — so the defect
ships from C1 with a by-hand concurrent test that shows two boundary joins both
landing, and R8 inherits a bug rather than a promise (fable, coop
`breakout-r5/1`, after ratchet found the gap in `rumor/853`).

## What the ledger could not express, and what that cost

Two rules are NOT in `statute.rego`, and the reasons are recorded there rather
than left as gaps:

- **There is no capacity field.** A claim carries `type`, `ref`, `handles`,
  `expect`, `planted` and `visible_to`; `expect` is an expectation and not a
  declared capacity. So a capacity overrun is not nameable from a ledger case —
  the data model agreeing, independently, with the ruling above.
- **A statute never sees `handles`.** The engine projects each claim as
  `{ref, actor, type, position}` plus `planted` and `visible_to`
  (`internal/engine`'s `prefixFacts`), so no rule can ask which entity a claim
  is about. Every package under `examples/` is therefore single-subject, this
  one included: one event per case. It is a limit on RUNS rather than on cases,
  and R5.3 needs `handles` in the projection to judge a walk that touches two
  events.

## A leak is a bug only when it moves the value

`docs/vocabulary.md` fixes v1's reading: an observer's expectation is what it
knows **restricted to what its `role` may see**, and a leak is a `bug` only when
it moves the observed value — one that preserves the value is R18's `trespass`.
So `role_sees_subject` restricts the expectation and never produces a verdict,
and the visibility-leak case is one where the restriction is what the API
violated. A `direct` observation bears no role, and nothing restricts it: that
is the conditional "where a project has roles" read as the conditional it is.
