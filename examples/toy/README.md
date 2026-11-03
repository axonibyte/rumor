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

## The daily allowance, and the two profiles that reach it (R7.5)

A fourth planted row: a per-user daily invitation allowance whose next reset
this application computes as **the start of the current local day plus
twenty-four hours of wall time**. That is the zone's own next midnight on every
day of the year except the two a daylight-saving transition makes 23 or 25
hours long — so the reset lands an hour LATE in spring and an hour EARLY in
autumn, and a user is refused for an hour into a new day or allowed one before
the old day ends. It is re-anchored from the current local date each day, so
the plant is exactly one day wide.

| profile | the world it places | what a run finds |
|---|---|---|
| `dst-spring` | midday 2026-03-07, the day before the 23-hour day | the allowance's next reset reads `01:00` local |
| `dst-fall` | midday 2026-10-31, the day before the 25-hour day | it reads `23:00` local |

**Two committed seeds, one per direction**: `1 profile=dst-spring steps=24` and
`2 profile=dst-fall steps=24`, each finding its row in three steps. One start
in the day before March cannot reach November in a walk any budget allows,
which is why the direction is the profile's and not the seed's (fable, coop
`r7/110`).

**THE SYSTEM'S ZONE IS ITS OWN CONFIGURATION.** `toy --zone` names the zone
this application reads its own instants in, passed as `TOY_ZONE` and declared
in `[adapter].env`; `[clock].zone` in `bindings.toml` is what the ENGINE reads
the run's instants in. They are separate on purpose — the two disagreeing is a
thing a run can find — so a walk of either profile wants `TOY_ZONE` set to the
zone the project declares, and with it unset these seeds find that
disagreement instead of the arithmetic. `seeds.lock` says so beside the rows.

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
