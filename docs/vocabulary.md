# Vocabulary

This document is the versioned vocabulary of `rumor`: the words the ledger,
the statute, the findings and the bindings share. It is **v1**, written in
R1.3 from the epic's Decided list, and every later phase that changes a term
here bumps the version and adds a Revisions entry. The vocabulary version
constant promised to R4's contract now exists as
`internal/vocabulary.VocabularyVersion`, and `tests/vocabulary` holds it to the
HIGHEST version in the Revisions table at the end of this file -- the highest
and not the last, because this document states no ordering for those rows -- so
the two cannot disagree about which vocabulary the module implements.

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
| `clock` | the run; a clock fact belongs to no actor, and reconciliation walks actors, never facts | the run's time at a position (see Time) |

A **claim** carries exactly one of two tags: none (an honest claim) or
`planted` (see `planted`). A claim type is declared **`immediate`** or
**`eventual`** in the bindings; an eventual type carries a **bound**, a
duration or until-quiesce, inside which disagreement is `pending` rather than
`bug` (R6).

A **tell** (`told`) is one hop and never reaches the system under test: it
moves knowledge between actors, not state into the server. Rules never
follow tells; the engine's flattening module computes `known(actor,
claim_ref)` from the one-hop `told` facts in Go, outside Rego, with an
extension point for ancestry facts, and hands the result to Rego as data (as
fields of the check's input document, which is where R2.3's engine puts the
sealed prefix and the flattened facts; OPA's own `data` namespace is not used
in v1) (R2). The example cases therefore carry `known` pre-flattened, which is this
architecture and not a shortcut; `told` facts appear in the examples'
ledger files (eleven of seventeen AT R2.1, 2026-10-15); no example carries a `clock` fact
until R2.3 drives one, which is a named gap, not an oversight. Whether a tell
carries the claim's reference or its payload is an epic Open item leaning
reference; v1 adopts the reference form and the ADR that decides it revises
this entry if it decides otherwise.

**Deletion is a new fact.** Nothing in the ledger is ever rewritten or
removed; a claim that something is gone is a claim like any other.

## Findings

A **finding** is the engine's verdict on one observation, judged against what
the observing actor should believe at that position: the claims it made and
the claims it was told, flattened. There are four findings in v1, and v3 adds
the fifth this document reserved:

| finding | meaning |
|---|---|
| `consistent` | the observation agrees with what the observer knows |
| `explained` | the observation agrees with the whole truth, disagrees with what the observer knows, and the gap is accounted for by claims the observer was never told: the untold claims, added to what the observer believes, balance to the observation. Not a defect. Charlie, told only by Alice of the two users Alice and Bob created, observes 2 and the finding is `explained` |
| `pending` | the observation disagrees on an `eventual` claim type and the disagreement is inside the type's declared bound. `pending` exists only for eventual types and only inside their bound; it is re-checked at the actor's next observation of the same query and at every full reconciliation (R6) |
| `bug` | the observation disagrees with what the observer knows, no account of untold claims balances the gap, the disagreement is not inside a bound, and the claims involved are honest |
| `quiesce_timeout` | DEFINED AT v3 (R6.2), the fifth finding this document reserved below: the quiesce probes a project declared did not read quiet within their declared timeouts. THE FINDING IS THE TIMEOUT and not a verdict on any claim -- a system that would not settle is a thing the run FOUND, whether or not a disagreement was open at the moment it gave up. It names the probe and the timeout it passed, and the claim types holding a `pending` then, if any: a pending is what this finding usually CARRIES, not what it requires. Neither inside a bound nor past one, because the bound was never reached. It is not a `bug` and not a refusal: the project's declarations are right and its adapter answered, so there is nothing for either reader to fix but the system under test. Exits 3, where a finding does |

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
each by its own Revisions entry: `stale` (DEFINED BELOW at v2, R6.1), a
quiesce timeout (R6), `ui_timeout` (R9), and `trespass` (R18: an action the
statute forbids that succeeds). v1 named them so the examples could avoid the
words; it did not define them, and TWO of them are now spent: `stale` as a kind
at v2, and the quiesce timeout as `quiesce_timeout`, the one finding beside the
four, at v3.

**THE SLOT BESIDE THE FOUR IS SPENT, and `ui_timeout` and `trespass` stay
reserved AS KINDS.** The sentence above reserved exactly one finding beside the
four and `quiesce_timeout` is it, so a later phase reading the same sentence
cannot claim a sixth: what v1 named and this version has not spent arrives as a
kind under `bug`, or by amending this sentence in its own Revisions row and
saying why the shape needs a finding. R9 holds `ui_timeout` and R18 holds
`trespass`; neither has a slot here to fill.

### Kinds under `bug`

A kind narrows a `bug` by saying WHY it is one. It is not a fifth finding: a
kind carries `bug`'s own meaning and its own status, and a reader keying on the
finding sees every bug whatever its kind.

| kind | meaning |
|---|---|
| `stale` | a `pending` past its bound. The observation disagreed on an `eventual` claim type, the disagreement was inside the bound and so was held open, and the bound has since passed with the disagreement unresolved. It names the claim type, the position of the claim it was late about, and the bound it passed |

**A `stale` is terminal and a `pending` is not.** `pending` is re-checked at the
actor's next observation of the same query and at every full reconciliation; a
`stale` has had that chance and is a finding, so it is not held open for
re-check and a run that ends on one leaves no pending open.

**The bound has two forms and R6 judges one of them.** A duration bound is
measured against the run's own clock, and an eventual type with a duration
bound in a project with no clock control is REFUSED rather than read as past
its bound -- time the run does not control never decides a verdict. The
quiesce form asks instead whether the system has settled: the disagreement is
`pending` until the declared probes read quiet and `stale` if the
reconciliation that follows still disagrees. R6 builds the quiesce form end to
end; the duration form keeps its refusal until a later phase gives the run the
clock. Avoiding a word means no statute rule,
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
  passing. A planted claim that an observation tests must produce `bug`; a
  planted claim no observation tests is counted and printed as unobserved,
  neither a pass nor a failure. `rumor attaint` requires every planted lie
  in a fixture to produce `bug` (R3), a fixture's author having arranged the
  observation; a walk (R5) counts what it did not reach. For a half-true
  claim the engine records, under the tag, which part is false, and an
  observation tests the claim when it reads that part.
- **The strip.** With `planted` removed from every planted claim **in a case
  that declares the lie its control**, the same run must still produce the same
  `bug` findings, and with the planted claims removed altogether the run must be
  `consistent`; a `bug` that appears or vanishes for any other reason is a
  defect in the statute or the engine (R3). A case that carries a planted claim
  **without** declaring a control is counted and printed, not stripped: the
  strip proves a `bug` was the lie rather than the statute, and that proof is
  only meaningful where the lie is the finding's subject.

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
| v3 | 2026-10-26 | R6.2 | `quiesce_timeout` DEFINED, the second of the four words v1 named and did not define, and the ONE FINDING BESIDE THESE FOUR that v1's own sentence reserved -- not a kind under `bug`, which is what `stale` spent at v2. A disagreement on an `eventual` claim type bounded `quiesce` whose declared probes did not read quiet within their declared timeouts: neither inside the bound nor past it, because the system would not settle. It is a finding and not a refusal for the reason fable's D7 gives (coop breakout-r6/0): the project's declarations are right and its adapter answered, so a refusal would tell a reader to fix something that is not broken; the exit is 3, where a finding's is. THE SPELLING IS v1'S, not invented here: v1 tokenized `ui_timeout` and left this one as the prose "a quiesce timeout", and the token follows the sibling it was listed beside. A TERM IS ADDED ABOVE AND NONE IS CHANGED OR REMOVED, which is this version's only addition, and `ui_timeout` and `trespass` stay reserved. The engine refused this state until now rather than naming it -- its own comment said the kind was R6.2's to define and that the build must not invent a word -- so no verdict changes meaning; what was a refusal is now a finding. NOT YET PRODUCED IN FULL, said here rather than discovered later: the engine reaches this finding through `pendingOverlay`, which returns before it looks when the statute found agreement -- so a probe that gives up while every observation agrees is today reported by nothing. The definition above is the word as ruled (coop r6/42); the half that reports a timeout with no pending open belongs to whatever POLLS the probes, which is R6.2's remaining piece, and internal/engine/pending.go names the gap at the site that produces it. MEASURED on R6's control for every child, AND THE RECORD OF IT NOW CARRIES ITS INVOCATION, which is the only thing that was ever wrong with it: `rumor run examples/toy --seed N --steps 10`, stdout only, sha256 truncated to 16, gives 786416e6925acf7c, dff917a0f6c74983 and ba42d0333035a0aa for seeds 2, 40 and 1 -- v2's triple exactly -- at v0.1.31, at this branch's base and with this version applied, so the three findings the engine already produced are unmoved. THE FLAG IS THE WHOLE OF IT (ratchet, coop r6/4-/5): without `--steps 10` seeds 2 and 40 are byte-identical and seed 1 is 80d212c53f606e6d, so a reader re-running the control from the record alone matches two of three and reads the third as a regression they caused. I did, and spent an hour of this phase on it before ratchet named the flag; the partial match is what bought the wrong conclusion, a full mismatch having been better evidence. The numbers were right and nothing is restated here, only the command beside them |
| v2 | 2026-10-25 | R6.1 | `stale` DEFINED, the first of the four words v1 named and did not define: a kind under `bug` rather than a fifth finding, which is what v1's own "later phases add kinds under `bug`" sentence reserved it as. A `pending` past its bound, naming the claim type, the claim's position and the bound; terminal, so it is not held open for re-check where a `pending` is. The two bound forms are stated with the one R6 judges: the quiesce form, pending until the declared probes read quiet and `stale` if the reconciliation after still disagrees, which needs no clock; and the duration form, which keeps the refusal R2.3 gave it -- an eventual type with a duration bound and no clock control is refused rather than read as past its bound -- until a later phase gives the run the clock. No term above is changed or removed, and the other three reserved words stay reserved. MEASURED rather than assumed: the engine overlay declined past the bound until this version because the word was undefined (its own comment said so), and the three findings it does produce are unmoved -- toy seeds 2, 40 and 1 are byte-identical to v0.1.31's output, digests 786416e6925acf7c, dff917a0f6c74983 and ba42d0333035a0aa, which is R6's control for every child |
| v1 | 2026-10-13, amended 2026-10-14, 2026-10-15 and 2026-10-16 | R1.3 | first definitions: four fact kinds, four findings and the control, positions and sealing, run-local handles, `planted`, `clock` facts, the three observation sources. Named for later phases without defining them: `stale`, the quiesce timeout, `ui_timeout`, `trespass`; the `told` payload-or-reference and the `direct` verb shape are Open items, adopted here in their leaning form. Amended 2026-10-14 from the review against the three packages: `explained`/`bug` clauses corrected and the two inputs stated; `of`, `role` and `visible_to` named; ref = handle in v1 and the between-attempts collision; the expectation on a claim optional; `pending` and `clock` named as gaps with their examples' arrival; reserved words may appear in prose; "the same `bug` findings" compares findings alone; Go, not Rust. Amended 2026-10-15 from R2.1 and R2.3, two readings v1 left open now RATIFIED as its meaning rather than changed (ratchet breakout-r2/12, fable breakout-r2/13): a `told` fact cannot pass a claim no earlier position holds, which follows from "the receiver now knows that claim" without v1 stating it as a rule, and the store refuses it at append because referential integrity is about the ledger's state and not the fact's shape; and the observation is the check's INPUT while the sealed prefix is the DATA, so "judged against the knowledge the observer holds at p" and "the check reads the sealed prefix" say the same thing once the observation is not counted as part of the prefix. Neither adds or removes a definition above. Three wordings amended in the same pass, fable's to word and rook's to carry (breakout-r2/19): "hands the result to Rego as data" now says which document those fields are in, since the epic's "data" and "input" named the two things and not OPA's two namespaces; the `clock` row's "written by" now reads "the run; a clock fact belongs to no actor, and reconciliation walks actors, never facts", which is what an engine that reconciled against an actor called "run" found the hard way; and the Fact kinds note now counts what is actually there -- `told` facts in eleven of the seventeen ledger files, and no `clock` fact in any of them until R2.3 drives one. Amended 2026-10-16 from R3.2 (fable breakout-r3/7, on ibis r3/9 and /11 and quill r3/10): the STRIP bullet gains the scope it was read as already having. `rumor attaint` implements it and fires on three of the four planted cases in `examples/` under the unqualified reading -- role-visibility's viewer-own-lie-exceeds and transitive-count's charlie-own-lie and charlie-knows-only-his-lie carry a planted claim INCIDENTALLY, testing visibility and knowledge gaps, and removing the lie leaves a finding that was never about it. quill's reading settled that the warrant was not in the text: "its two uses are both checks on the engine" says what the checks are about and not which fixtures each covers, and the sibling bullet is explicit where it means universality. So the sentence is amended rather than the three fixtures, which R1's gate passed: changing a gated fixture to fit a sentence is the wrong direction of repair. The CORROBORATION bullet stays universal -- every planted lie produces `bug`, which all three incidental cases already meet -- and a planted case that declares no control is counted and printed rather than silently skipped. Amended 2026-10-17 from R4.1 (quill): the version constant this document promised to R4's contract now exists as `internal/vocabulary.VocabularyVersion`, carrying the value this table's highest row names; no term above is added, removed or changed by that, and the preamble's sentence about the Revisions section BEING the version is replaced by the pairing. Amended 2026-10-19 from R5's breakout (ratchet rumor/853, fable breakout-r5/1): the CORROBORATION bullet gains the scope the strip gained on 10-16 -- it binds where an observation tests the lie, and a planted claim no observation tests is counted and printed rather than read as a pass or a failure; a fixture arranges its observations and `rumor attaint` is unchanged over fixtures. A half-true claim (`palter`) is planted like any lie, the engine recording under the tag which part is false; there is still no third state. `canard` is named here only to say what it is not: a tell that passes on a planted claim by an actor that is not its author, a weight and a precondition over the kinds above, not a fact kind and not a tag |
