# Design

> Stub from R0.1. The headings below are the ones this document owes; the
> content arrives with the phase that owns the subject. `docs/coverage.toml`
> names this file, and `rumor docs --check` fails if a source path it should
> describe is not mapped.


## Architecture

Not yet written.

## The decided list

This section records decisions as they are settled, with the run that settled
each. It is **not** complete: the epic holds the full list, and entries arrive
here as the phases that own them land.

### Names, settled in R0.4

- **The module is `forge.axonibyte.com/axonibyte/rumor`. The binary is
  `rumor`.**

  > **The crates.io problem does not survive the pivot to Go**, and this entry
  > is rewritten rather than annotated because its premise is gone. A Go module
  > path is a URL the project controls, not a name allocated from a shared
  > registry, so there is nothing to be squatted and no fallback name to settle
  > on. The decision this entry used to record — ship as `rumor-cli` because the
  > bare `rumor` crate name is held — asked a question Go does not pose.

  Until v0.1.3 the crate was `rumor-cli`, because the bare `rumor` name on
  crates.io is held, with `rumor-client` and `rumor-server`, by one owner as a
  reservation published 2023-11-20 and never updated — its own description reads
  `reserved; library for rumor file synchronization protocol`. That constraint
  shaped the published name for four tags and is kept here as the record of why,
  not as a live decision.

  A transfer has been asked for and the ask's status is recorded in
  `docs/prior-art.md`. If it is ever granted, publishing under `rumor` is an
  improvement landed by a later patch run; nothing in the project depends on
  it. The binary is `rumor` either way, which is the name users type.

- **The licence is BSD-2-Clause**, and the allowlist settled in R0.2 gates every
  pull request. What *applies* that allowlist changed with the port: until the Go
  module landed it was `cargo deny check licenses`; it is now `tests/licences`,
  a Go test over the surface `go list -deps -test` reports.

  **The eight ids, enumerated here because this document is their home.**
  `deny.toml` held them and was the second reader the test compared itself
  against; G4 removed it with the Rust tree, which left the list in exactly one
  place — `tests/licences` — and nothing able to notice it drifting from R0.2's
  decision (ratchet, thread:go-port/54). A decision's home is a document, not a
  tool's config, so:

      ALLOWLIST BEGIN
      Apache-2.0
      BSD-2-Clause
      BSD-3-Clause
      ISC
      MIT
      MPL-2.0
      UPL-1.0
      Unicode-3.0
      ALLOWLIST END

  The test parses that block and compares it against its own list, so the two
  have to agree — the project's rule about testing against the source rather
  than a re-export, applied to itself. The markers are deliberate: a reader
  needs the list and a parser needs an unambiguous edge, and a heading would
  give it neither.

  **The allowlist now has one home, which is a deliberate loss.** It lived in
  `deny.toml` and was duplicated in the test so the two had to agree, and that
  cross-check is gone with the Rust tree: `deny.toml` had no other consumer, and
  keeping a file no tool reads to preserve a comparison would be keeping an
  oracle that cannot disagree about anything a reader would act on. The test
  reports the comparison as SKIPPED naming this change, rather than passing
  silently, so the weakening is visible in the run and not only here.

- **The policy engine is OPA, pinned, and it is the project's first
  dependency** (G3, issue 34).

  The statutes are Rego, so the engine is whichever library evaluates Rego on
  the chosen runtime. Under Rust that was `regorus`, pinned with `=` so a patch
  release could not change a statute's verdict without a commit saying so; the
  Go equivalent is `github.com/open-policy-agent/opa`, which is Rego's reference
  implementation rather than a reimplementation of it. The same pinning
  INTENTION applies, and the mechanism is different in a way worth stating
  rather than glossing: Cargo's `=` refuses a raise, while a Go `require` is a
  minimum that minimal version selection may raise. Nothing here raises it, and
  `go.sum` pins the content; see "The engine" for what that does and does not
  guarantee.

  It also takes the `go` directive up from 1.25 to **1.26.0**, because OPA
  declares 1.26.0 and a module cannot depend on one that requires a newer
  language version than it declares. The lower bound was deliberately 1.25 at
  the port — a newer toolchain builds an older module and not the reverse — so
  raising it is a cost, paid here with the reason recorded: the runner measures
  at go1.26.8, so no runner is stranded by the raise today.

See `docs/prior-art.md` for the registry, GitHub and cross-language sweep
behind the first entry, including the one collision that matters inside Rust:
in the Nostr protocol *rumor* is a defined term for an unsigned event.

## The engine

Rules are Rego, evaluated by **OPA embedded as a library**, at
`github.com/open-policy-agent/opa v1.21.2`.

**What that line does and does not guarantee** (wren, `thread:rumor` record
537, sharpened at 542). A `go.mod` `require` is a **minimum**, not an exact
pin: minimal version selection takes the highest version any module in the
graph asks for, so a transitive requirement could raise the one that builds.
Nothing in this tree does — OPA is the only direct dependency and nothing
requires a newer OPA — so the selected version *is* v1.21.2, and `go.sum` pins
the content of what is fetched, which is why the verdicts are reproducible. The
previous engine was pinned with Cargo's `=`, which refuses the raise outright;
Go has no such operator, and an earlier version of this paragraph described
Cargo's guarantee in Go's syntax. The property to rely on is: one direct
dependency, a `go.sum` over its content, and a release gate that refuses a run
whose `go.mod` changed without the documents that describe it.

The decision and what it rules out are recorded in
`docs/adr/0001-engine.md`. This section is the inventory behind it: what the
engine was measured to do, and from which runs.

### The six capabilities, measured

Measured against OPA v1.21.2 from a Go test suite, every arm driven in both
directions, with the run output quoted in the G2 checkpoint
(`thread:go-port`). The six capabilities, and the Regorus verdicts they are
compared against, are **ibis's** — measured at `thread:r1` record 1 and
`thread:adr-regorus-tracing` record 0, and credited to ibis throughout.

| # | Capability | OPA v1.21.2 | Regorus 0.12.0 (ibis) |
|---|---|---|---|
| 1 | aggregation | **pass** — `{"biggest":3,"observed":2,"total":6}` | pass, same figures |
| 2 | negation | **pass** — both directions | pass |
| 3 | comprehensions | **pass** — array, set, object | pass |
| 4 | input/data split | **pass** — both directions | pass |
| 5 | test files | **pass** — `tester.Runner` | **gap**, workaround |
| 6 | evaluation tracing | **pass** — retrievable | **gap** (R10's) |

Capabilities 1 through 4 reproduce ibis's figures exactly, which is what makes
the two inventories comparable rather than merely adjacent.

**Capability 5 is the gap that closes.** `tester.Runner` enumerates the `test_`
rules in a module and reports each one's verdict, including a *failing* one. The
engine this replaces exposes no test entrypoint, so the workaround was a
host-side runner that evaluates the package and lists the rules that held —
which can show that a test rule is absent and can never show that one *failed*.

**Capability 6 is the gap the decision turns on.** A trace is retrievable:
`topdown.NewBufferTracer()` passed as `rego.QueryTracer`, rendered with
`topdown.PrettyTrace`, produced 28 events and 648 bytes for a three-rule policy,
naming each rule indexed, entered, unified and exited. The engine this replaces
has a `trace()` builtin that returns its argument and no retrieval API at all.

In-policy `print()` also reaches the host, carrying `file:line`
(`policy/pr.rego:4: seen`) via `rego.EnablePrintStatements` and
`rego.PrintHook`. Statutes do not use it — ADR 0001 keeps `print()` out of a
statute — and what makes that sentence true is `tests/statute`, which checks
every statute and every mutant two ways: a static scan of the parsed module,
and a hook on the real evaluator. Both are needed, because OPA **erases**
`print()` calls unless `rego.EnablePrintStatements(true)` is passed: measured
with the flag off, a `PrintHook` alone captured nothing from a statute full of
them, so a reader who installs only the hook gets a silence that reads like
compliance. `print()` remains available to diagnostics outside statutes.

### Coverage is range-based, and that is the useful part

The coverage report addresses **rows and columns**, not whole lines, so one
report answers two different questions about the same line: the rule *head*'s
range says whether the rule **held**, and the body's ranges say whether it was
**evaluated**. For a rule whose body ran and failed:

```
row 3 head (col 1-2)    not_covered     the rule produced no value
row 3 body (col 6-30)   covered         the body was evaluated
aggregate               covered_lines 3, not_covered_lines 1, coverage 75
```

Two consequences worth writing down. The aggregate counts such a line in
**both** tallies, so reading `covered_lines` as a line count is wrong. And a
rule the **indexer** skips before evaluation (`Index data.cov.b (matched 0
rules, early exit)`) is absent from `covered` because it was never evaluated,
not because it failed — so coverage alone cannot distinguish "did not hold" from
"was never reached". The trace distinguishes them, which is why the explanation
behind a finding is built from the trace and not from coverage.

### The language version the engine forces

OPA v1.21.2 declares `go 1.26.0`. At the port this module declared a lower
version deliberately, under a comment permitting a raise once the runner's
version was known and something here needed the newer language. Both
conditions held by G3, which first took OPA as a real dependency: the CI jail
had been measured at go1.26.8, and the engine was the thing that needed it. The
raise landed with that run, in v0.1.4, and `go.mod` has declared `go 1.26.0`
since; this paragraph keeps the reasoning as a record of why the bound moved.

One trap, because it defeats a version assertion rather than failing it: with
the default `GOTOOLCHAIN=auto`, invoking an older toolchain against a module
whose directive requires a newer one makes Go **download and hand off to the
newer toolchain silently**. Measured: `go125` ran a suite under go1.26.8 and
reported `ok`, while the same command under `GOTOOLCHAIN=local` refused with
`running go 1.25.14; GOTOOLCHAIN=local`. Naming a versioned binary therefore
does not pin the toolchain that builds the code; the `go` directive does.

## The engine owns the seed

Not yet written.

## Knowledge is flattened outside Rego

`known(actor, claim_ref)` — what an actor should believe at a position — is
computed by `internal/flatten` and handed to a statute as data. It is not
derived inside Rego, for three reasons in the order that decides them.

**A rule that followed tells would be a different statute.** The vocabulary is
explicit that rules never follow tells. If each statute derived knowledge, each
would carry that derivation, and one that got it subtly wrong would produce a
finding that reads as a verdict about the system under test while actually
being a verdict about its own bookkeeping.

**It is data to the question, not part of it.** Every case under `examples/`
already hands the statute a `known` array. This package produces those arrays
from a ledger, so its output shape was pinned by fixtures that existed before
it did — the epic's "the examples are the acceptance fixtures and they exist
before the engine does", read literally.

**The extension point needs somewhere to live.** Ancestry facts will let
knowledge travel along a derivation chain rather than only along a tell. That
changes how knowledge is computed and nothing about what a statute means, so it
belongs where it can be swapped without touching a statute or re-pinning a
fixture hash. It is an interface, and a test supplies one — including a cyclic
provider, because a walk without a visited set would hang rather than fail.

**One hop means one hop.** A tell moves a *reference* from one actor to one
other actor and nothing is inferred from it. If Alice tells Bob and Bob tells
Charlie, Charlie knows the claim because of Bob's tell, which is its own fact —
not because knowledge was chased through Bob. The property that pins this is
that a teller who knows more than it told must not leak the rest: Alice holding
two claims and telling Bob one leaves Bob knowing exactly one. That case was
missing from the first suite and a mutation found it; a one-claim ledger cannot
tell the two behaviours apart.

## The handle table is per attempt

`internal/handle` maps `actor.type#n` to the id the server returned, for **one
attempt**. The handle is the stable name and the id is the disposable one,
which is the opposite of the usual arrangement and the reason raw server ids
never enter the ledger's logical content, a statute, or any comparison: a
replay re-creates the entities, the server hands back new ids, and the findings
still compare.

A table that outlived its attempt would resolve a handle to an id from a
previous run, so `NextAttempt` returns an **empty** table rather than a copy,
and a test asserts that the old id no longer resolves while the handle takes
the new one.

**Three collisions, and v1 names one of them.** The draft named none when this
package was written, which was raised and amended: v1 now says the collision
that matters is **between attempts** — a retried attempt produces the same
handle with a different server id, the table keeps the current attempt's id
only, and a claim naming a handle absent from it "is refused at the ledger as a
validation error (R4), never judged as a finding". `NextAttempt` implements the
first half and `Require` is the seam for the second: it returns an error rather
than a bool, because a caller holding `(string, bool)` can read the false as a
disagreement and emit `bug`, which reports bookkeeping as a defect in the system
under test — a failure that looks like the product working.

The other two are **within** an attempt and remain this package's decision,
supported by v1's new line that a handle names at most one entity per run: a
handle rebound to a *different* id is refused, because the handle is the
identity and a second binding would redirect every claim already made about it;
and two handles bound to the *same* id are refused, because the run then created
fewer entities than it believes and a comparison by handle would agree about
different things. Rebinding to the **identical** id is permitted — a retried
create that actually succeeded is not a conflict, and refusing it would make the
table refuse its own idempotence.

## The ledger is append-only

`internal/ledger` holds the four fact kinds `docs/vocabulary.md` allows and
nothing else, with a position and an actor on every fact. Nothing is ever
rewritten or removed: a claim that something is gone is a claim like any other.

**Positions are assigned by the store, not passed in.** The vocabulary calls a
position "a logical, monotone integer assigned to every fact as it is
appended". If callers chose positions, monotonicity would be the caller's
invariant and this package could not promise it, so `Append` assigns and
returns. `AppendAt` exists for `Load`, which has to reproduce positions it is
handed, and it is therefore the only door the seal has to guard.

**Sealing is what makes a check reproducible.** A ledger is sealed at a
position before any check runs, and writing at or before a sealed position is
an error, never a reorder. `SealedPrefix` returns nothing for an unsealed
ledger, deliberately: a check reading an unsealed ledger is a sequencing
error, and returning everything would hide it behind a plausible answer.

**The dump is deterministic and diffable, which are two properties and both are
tested.** One field order, ascending positions and no map iteration anywhere,
so the same logical content gives byte-identical output and re-dumping a loaded
ledger does not churn a committed fixture. One fact per line, asserted by
changing a single fact and requiring exactly one line to move -- a format that
reflowed would pass a round-trip test and fail that one. `Load` goes through
the same guards as `Append` and rejects unknown fields, so a dump no writer
could have produced is refused on the way in rather than trusted because it
came from a file.

### The example cases are not ledgers, and their positions do not carry over

The three packages' cases were authored as flattened input: each carries a
`known` list and a claim set, with positions chosen to express a bound. A
ledger needs the knowledge to arrive as `told` facts, and a `told` fact needs a
position of its own. Measured across all 17 cases, only two leave room for it;
the rest would need a position that is already taken, and the three
`position-bounded` cases put the observation at the same position as a claim,
which this store refuses as non-monotone.

So the ledger files under `examples/<name>/ledger/` **re-assign positions**.
They preserve the case's order and its bound, not its integers, which is what
the vocabulary allows when it says positions "are not shared across runs, and a
replay assigns its own", and that replay "compares findings, never positions or
raw ids, so two runs with different positions can agree". The equivalence that
matters is therefore asserted rather than assumed: for every case, the claims
inside the sealed prefix must match the case's expected `truth`, and the
prefix's `told` facts must flatten to exactly the case's `known` list.

One reading had to be chosen and is recorded here rather than left to inference:
**the observation is the check's input, not a fact in the sealed prefix.** v1
gives an observation a position and also says the check reads the sealed
prefix, which leaves open whether the observation sits inside that prefix; R2.3
takes "a sealed ledger and flattened facts as data, one check as input", and
these files follow it. If the vocabulary settles it the other way, the ledger
files change and the cases do not.
