# Vocabulary

This document is the versioned vocabulary of `rumor`: the words the ledger,
the statute, the findings and the bindings share. It is **v1**, written in
R1.3 from the epic's Decided list, and every later phase that changes a term
here bumps the version and adds a Revisions entry. The vocabulary version
constant lands in the module with R4's contract; until then the Revisions
section at the end of this file is the version.

Code identifiers use the plain names below (`claim`, `told`, `planted`).
The themed words (`attaint`, `exhume`) belong to subcommands and prose, never
to the ledger.

## Fact kinds

The ledger holds four kinds of fact and nothing else. Every fact carries a
**position** (see Positions) and the **actor** it belongs to; the engine never
writes a fact without both.

| kind | written by | says |
|---|---|---|
| `claim` | an actor, after one of its own actions | "I did this, so the world should now contain that": the claim type from the bindings (`event.created`), its **ref** (the identifier a `told` points at; in v1 a claim's ref is its handle, see Handles), the handles it names, and optionally `visible_to`, the roles the server should show the object to (see Findings on visibility). A claim **may** carry the actor's expectation of what a later observation should show; when it does not, the statute derives the expectation from the claims, which is what the three examples do |
| `observed` | an actor, or the engine for `direct` | what was seen at a position: a named query, its source (see Observation sources), the value returned, **`of`**, the claim type the observation tests, and the **`role`** the observing principal acted as, when the project has roles |
| `told` | the engine, on an actor's behalf | one actor passed a **reference to a claim** to another actor at a position; the receiver now knows that claim |
| `clock` | the run, when it controls time | the run's time at a position (see Time) |

A **claim** carries exactly one of two tags: none (an honest claim) or
`planted` (see `planted`). A claim type is declared **`immediate`** or
**`eventual`** in the bindings; an eventual type carries a **bound**, a
duration or until-quiesce, inside which disagreement is `pending` rather than
`bug` (R6).

A **tell** (`told`) is one hop and never reaches the system under test: it
moves knowledge between actors, not state into the server. Rules never
follow tells; the engine's flattening module computes `known(actor,
claim_ref)` from the one-hop `told` facts in Go, outside Rego, with an
extension point for ancestry facts, and hands the result to Rego as data
(R2). The example cases therefore carry `known` pre-flattened, which is this
architecture and not a shortcut; `told` and `clock` facts appear in the
examples' ledger files (R2.1), and no example exercises `clock` until R2.3
drives one, which is a named gap, not an oversight. Whether a tell
carries the claim's reference or its payload is an epic Open item leaning
reference; v1 adopts the reference form and the ADR that decides it revises
this entry if it decides otherwise.

**Deletion is a new fact.** Nothing in the ledger is ever rewritten or
removed; a claim that something is gone is a claim like any other.

## Findings

A **finding** is the engine's verdict on one observation, judged against what
the observing actor should believe at that position: the claims it made and
the claims it was told, flattened. There are four findings in v1:

| finding | meaning |
|---|---|
| `consistent` | the observation agrees with what the observer knows |
| `explained` | the observation agrees with the whole truth, disagrees with what the observer knows, and the gap is accounted for by claims the observer was never told: the untold claims, added to what the observer believes, balance to the observation. Not a defect. Charlie, told only by Alice of the two users Alice and Bob created, observes 2 and the finding is `explained` |
| `pending` | the observation disagrees on an `eventual` claim type and the disagreement is inside the type's declared bound. `pending` exists only for eventual types and only inside their bound; it is re-checked at the actor's next observation of the same query and at every full reconciliation (R6) |
| `bug` | the observation disagrees with what the observer knows, no account of untold claims balances the gap, the disagreement is not inside a bound, and the claims involved are honest |

Separating `explained` from `bug` needs two inputs, not one: the observer's
flattened knowledge and the whole ledger. Against knowledge alone Charlie
observing 2 and Charlie observing 3 disagree identically; only the ledger
says that one gap is Bob, untold, and the other is nothing. A statute that
has only the observer's knowledge cannot produce `explained` and must not
try. And the two clauses of `explained` are one assertion whenever every
held claim is honest, since then the truth is exactly the believed plus the
untold; they come apart only when the observer holds a claim the ledger does
not count, which is why both are stated and both are tested.

A finding names the rule that produced it, and the engine retains the
evaluation trace behind every finding for `rumor explain` (R2, R10).

**Visibility.** Every v1 finding is a verdict on value agreement. Where a
project has roles, an observer's expectation is what it knows restricted to
what its `role` may see (`visible_to` on the claim), so a correctly hidden
record is not a missing one: Dana, told of both documents and permitted one,
observes one and the finding is `consistent`. A leak is a `bug` only when it
moves the observed value; a leak that preserves the value is invisible to v1
and is the `trespass` of R18, not a finding here.

**`pending` has no example yet.** It needs an `eventual` claim type with a
bound declared in the bindings, and the bindings artefact arrives with R4; no
example exercises `pending` until then, which is a named gap.

Later phases add **kinds** under `bug` and one finding beside these four,
each by its own Revisions entry: `stale` (a `pending` past its bound, R6), a
quiesce timeout (R6), `ui_timeout` (R9), and `trespass` (R18: an action the
statute forbids that succeeds). v1 names them here so the examples can avoid
the words; it does not define them. Avoiding a word means no statute rule,
finding value or case expectation uses it; prose and comments may name one
to say it is deliberately unused, and the R1 gate reads the criterion that
way. `docs/world.md` reserves `trespass` under a heading of its own for the
same reason and defines nothing there either.

**The control.** There is no trust model. A `bug` whose derivation rests on a
`planted` claim is the control passing, not a defect: the engine found the
lie. In the planted-lie example the honest-only set is `consistent` and the
set with the planted claim is `bug`, and both outcomes are required. "The
same `bug` findings" compares findings alone: the attribution beside a
finding (`rests_on_planted`) flips by construction when the tag is stripped,
and that flip is the strip working, not a difference.

## Positions

A **position** is a logical, monotone integer assigned to every fact as it
is appended to a ledger. Positions order facts within one run and nothing
else: they are not timestamps, they are not shared across runs, and a replay
assigns its own.

- An observation at position *p* is judged against the knowledge the
  observer holds at *p*: every `claim` the observer made and every `told` it
  received at positions at or before *p*.
- A ledger is **sealed** at a position before any check runs; the check reads
  the sealed prefix. Writing a fact at or before a sealed position is an
  error, never a reorder (R2).
- Replay and shrink compare **findings**, never positions or raw ids, so two
  runs with different positions can agree.

## Handles

Actors refer to entities by **run-local handles**: `actor.type#n`, as in
`alice.event#1`, the first event Alice created in this run. The engine keeps a
handle table mapping each handle to the id the server returned on the current
attempt (R2). Claims, tells, rules and findings speak handles; raw server ids
never enter the ledger's logical content, the statute, or any comparison.
A replay re-creates the entities, the server hands back new ids, the handles
stay the same, and the findings are compared by handle.

A handle names at most one entity per run, and in v1 a claim's **ref** is
its handle, so a `told` points at the claim by the handle it created; the
examples carry both fields with the same value, which v1 makes a rule
rather than a coincidence. The collision that matters is between attempts:
a retried attempt produces the same handle with a different server id, and
the table keeps the current attempt's id only. A claim that names a handle
absent from the current attempt's table is refused at the ledger as a
validation error (R4), never judged as a finding.

## `planted`

`planted` is a tag on a `claim`. A planted claim is one the run knows to be
false by construction: a lie an actor tells about its own action. Honest
claims are the claims without the tag; there is no third state and no trust
model.

Its two uses are both checks on the engine, not on the system under test:

- **Corroboration.** A `bug` attributable to a planted claim is the control
  passing. `rumor attaint` requires every planted lie in a fixture to
  produce `bug` (R3).
- **The strip.** With `planted` removed from every planted claim, the same
  run must still produce the same `bug` findings, and with the planted claims
  removed altogether the run must be `consistent`; a `bug` that appears or
  vanishes for any other reason is a defect in the statute or the engine (R3).

## Time

Time is a ledger fact: `clock(position, t)`, the run's time *t* at a
position, written only when the run **controls the clock** through an
app-exposed test clock declared in the bindings or a stack run under faked
time (R7). Rules may read `clock` facts. When a project's behavior depends
on time, the run controls the clock; a rule that reads `clock` with no clock
control declared is a validation refusal (R4). Wall-clock time never enters
the ledger. Replays restore the clock with everything else.

## Observation sources

Every `observed` fact carries a **source**, one of three; an observation with
no position or no source is a validation refusal (R4).

| source | who observes | what it reads |
|---|---|---|
| `api` | an actor, through the API | the server's read-back of a resource or query |
| `ui` | an actor, through a real browser in the browser stage | the rendered page, positioned in the ledger like any other observation (R9) |
| `direct` | the engine, never an actor | a named query against the project's own store: the second oracle behind what the API or the page shows (R4, R8, R9). Its exact verb shape is an epic Open item; v1 fixes only who may use it and what it is for |

In the browser stage no actor holds an API session and actors speak to each
other only through tells; `direct` remains available to the engine there and
to no actor (R9).

## Revisions

| version | date | run | change |
|---|---|---|---|
| v1 | 2026-10-13, amended 2026-10-14 | R1.3 | first definitions: four fact kinds, four findings and the control, positions and sealing, run-local handles, `planted`, `clock` facts, the three observation sources. Named for later phases without defining them: `stale`, the quiesce timeout, `ui_timeout`, `trespass`; the `told` payload-or-reference and the `direct` verb shape are Open items, adopted here in their leaning form. Amended 2026-10-14 from the review against the three packages: `explained`/`bug` clauses corrected and the two inputs stated; `of`, `role` and `visible_to` named; ref = handle in v1 and the between-attempts collision; the expectation on a claim optional; `pending` and `clock` named as gaps with their examples' arrival; reserved words may appear in prose; "the same `bug` findings" compares findings alone; Go, not Rust |
