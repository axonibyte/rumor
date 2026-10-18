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

### The second direct dependency, added in R4.1

`github.com/BurntSushi/toml v1.6.0`, MIT, pinned. OPA was the first; this is the
second, and the whole dependency surface is still those two plus what they pull.

**It is a DIRECT require in `go.mod`, and it took a review to get there.** `go
get` had left it in the indirect block while `internal/decl/toml.go` imported it
directly — so this page called it a direct dependency and the module graph
called it indirect, and the next `go mod tidy` would have produced a `go.mod`
diff over a pure label correction. rook found it by reading the file rather than
the prose (coop r4/48), and nothing under `.forgejo/` or `tools/` runs `go mod
tidy -diff`, which is why a green CI run said nothing about it.

**Why a library and not a parser of our own.** `platform.toml` and a package's
`bindings.toml` use nested tables and arrays of tables, which
`internal/coverage`'s hand-written line reader does not reach — that one reads
two keys in one flat shape and was ported as-is from the Rust tree. The
alternative was a first TOML parser whose failure mode is a SILENT mis-parse of
a valid file, against a library whose failure mode is a refusal with a line
number.

**It was ruled rather than taken quietly** (quill coop r4/16, fable r4/30),
because a direct dependency is a standing cost to everyone who audits this tree.
rook measured it against the gate's own discriminators rather than the
allowlist's reputation (r4/17): the licence file is `COPYING`, which
`tests/licences` does match; `classify()` fires exactly one family, so no
`dualApproved` entry is owed; and `ci.yml` pins no absolute module count, so
adding a module moves no literal.

**What it is NOT allowed to decide.** Strictness belongs to `internal/decl`, not
to the decoder: the library answers "is this TOML" and nothing else, so a
mistyped key arrives as a document and is refused by name with its key path
rather than as a parse error naming a line. That split is also why the library
is swappable — `decl.Decoder` is a function parameter — and why
`internal/platform` and `internal/bindings` do not import it at all.

**One measured surprise, recorded because the comment version of it was wrong
for an hour:** this library hands an array of tables over as
`[]map[string]any`, not as `[]any` of maps. `decl.Reader.Tables` accepts both,
and the test that found it reads the library's actual output rather than our
expectation of it.

## The engine

Rules are Rego, evaluated by **OPA embedded as a library**, at
`github.com/open-policy-agent/opa v1.21.2`.

**What that line does and does not guarantee** (wren, `thread:rumor` record
537, sharpened at 542). A `go.mod` `require` is a **minimum**, not an exact
pin: minimal version selection takes the highest version any module in the
graph asks for, so a transitive requirement could raise the one that builds.
Nothing in this tree does, and the reason moved with R4.1: there are now TWO
direct dependencies, and the second one's own `go.mod` declares **no requires at
all** (`github.com/BurntSushi/toml v1.6.0`: a module line and `go 1.18`,
nothing else), so it cannot raise OPA or anything under it. The selected version
therefore *is* v1.21.2, and `go.sum` pins the content of what is fetched, which
is why the verdicts are reproducible. The earlier wording here — "OPA is the only
direct dependency" — was still on the page after R4.1 added the second, and rook
caught it (coop r4/48): the conclusion survived, the stated reason did not, which
is the worse of the two to leave standing. The
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

Written ahead of R5, which builds it: this section is the design R5 is built
to, and R5's checklist in the epic is the acceptance. When R5 lands, what it
measured amends this text rather than the text excusing the code.

**A run is a seeded walk.** Every run begins from one integer, the seed,
printed first and accepted back by `--seed`. Everything the run decides by
chance descends from that integer through one pseudo-random generator the
engine holds: which actor moves next, which action it takes, which lie is
planted and where. Nothing else in the system draws randomness. Drivers and
adapters are handed one action at a time, execute it against the system under
test, and report what they saw; they are never given the generator. That is
what "the engine owns the seed and the action sequence" means in the decided
list, and the reason is replay: a run must be reproducible from the seed, the
weight file, the bindings and the project's commit alone. A driver that drew
its own randomness would make the replay depend on something the seed does not
carry, and the shrinker below would be shrinking a trace it could not re-run.

**How the walk chooses.** At each step the engine computes the actions that
are applicable, by evaluating each action's preconditions against the sealed
ledger and the handle table (an event can be joined only by an actor who knows
of it; a capacity can be exceeded only if someone is at it). From the
applicable set it draws one by weight. The weights come from the project's
`platform.toml`, per stage and profile, so a project tunes what its crowd tends
to do without touching the engine. The nemesis kinds are action kinds in the
same pool with their own weights: `suborn` feeds an actor a false claim,
`canard` is a false tell, `palter` a half-true claim, `essoin` an abandoned
flow. A lie is therefore scheduled exactly the way an honest action is, which is
why the control in the vocabulary has no trust model to lean on: the engine
knows which claims it planted because it planted them, and the statute does
not.

**The sequence is over handles, not ids.** The walk names entities by run-local
handles (`alice.event#1`), and the handle table maps each to whatever id the
server returned on the current attempt. So the same seed yields the same
sequence of actions on every attempt, even though every attempt's ids differ,
and two attempts are compared by their findings over handles, never by raw ids
or by positions. Without that, a replay would diverge at the first id the
server chose differently, and nothing about the walk could be called
deterministic.

**Shrinking needs a clean world.** When a run fails, the engine first replays
the whole failure once on a verified clean rollback. If it does not reproduce,
the seed is marked unstable and is not shrunk; R14 turns that single replay
into a repeated check. If it does, the shrinker looks for a shorter trace with
the same finding: length bisection first, then removal of whole action kinds.
Every candidate trace is run against a verified clean rollback, through the
rollback trait (`describe`, `checkpoint`, `restore`, `verify`, `discard`; a
process-restart provider for the toy system under test, the small deliberately
buggy application in `examples/toy/` whose design is R5's own work in the
`r5.toy` thread and whose bug ledger is in the epic). The verify step is not
optional: a shrink step run on a world the previous step dirtied would
attribute to the trace what the leftover state did, and the "minimal" trace
would be minimal only in that world. Shrinking respects bounds, so a trace is
never shrunk to one that could not have produced its finding under an eventual
type's bound. The result is written as a minimal trace, which names the
output's shape (the actions, their positions, their handles and the rules that
fired) and not a property of it; the property R5 is held to is that each of its
three toy bugs shrinks to under five actions with every step on a verified
clean rollback.

**What "the same finding" means.** The shrinker keeps a candidate only if it
reproduces the same finding, and that identity is stated here because R14 later
builds its deterministic-or-unstable classification on it and the first
shrinker would otherwise choose silently. Two findings are the same when the
rule that fired, the finding's value, and its attribution (`rests_on_planted`)
all agree. The attribution is part of the identity because `bug` resting on a
planted claim and `bug` resting on none are different findings sharing one
value, which is the difference the corroboration arm exists to see; a shrinker
that let a trace drift from one to the other would be shrinking toward a
different defect.

**A candidate that cannot run is not one that did not reproduce.** Dropping an
action can leave a later one with a precondition that is no longer true, so the
candidate is inapplicable: it cannot be executed at all. The shrinker records
that as its own outcome and discards the candidate, never as evidence that the
failure is unstable. R14 separates the two by name, and R15's toy bug (an action
unreachable because its precondition is never true) is what consumes the
distinction.

**An unstable seed is not promoted.** In R5 a failure that does not reproduce
on the first clean replay is marked unstable, is not shrunk, and is not promoted
into `seeds.lock`; R14 turns the single replay into a repeated check and keeps
the same rule.

**Seeds are promoted, and the promoted ones run first.** A seed that found a
defect is promoted into `seeds.lock` with a comment naming the defect, and
committed seeds always run before new ones fill the remaining budget (R22). So
a project's history of found bugs becomes its regression suite without anyone
writing a test, and the walk's own output is what grows it.

Two things are open and recorded as such in the epic: whether guided coverage
weights are committed alongside `seeds.lock`, and the shrinker's name.

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

## Evaluation: a finding, its rule and its trace

`internal/engine` is the seam between the store, the flattening and a statute
(R2.3, issue 44). `Input` reads a **sealed** ledger and one check and produces
the statute's input; `Evaluate` returns the finding with the rule definition
that produced it, every named sub-rule, and the OPA trace retained whole for
`rumor explain` (R10). `rumor check` is the same walk over the three worked
examples, so the claim can be re-run by anyone holding the binary.

**The bound is the seal, and the observation sits at `seal+1`.** The statutes
bound their claim sets with `c.position <= input.observation.position`. Every
fact in a sealed prefix is at or before the seal, so `seal+1` admits exactly
the prefix and nothing else — the structural bound the ledger files were built
around, carried into the evaluation rather than restated by each statute.

A consequence worth stating, because it changes what the mutants prove: once
the bound is structural, **a statute's own position guard is dead in
production**. The three `*-ignores-position` mutants die against the flattened
cases and cannot die against the ledgers, since the prefix holds nothing for
the guard to exclude. `tests/engine` requires each of them to be *provably*
equivalent — identical values for every rule on every case — and requires the
case path to still kill it, so the clause is shown to matter and the bound, not
the harness, is what put it out of reach.

**A finding names a definition, not a rule name.**
`data.rumor.statute.finding` is the same string for every finding in every
case, which is a constant dressed as evidence. The engine reports the module
and row of the definition whose body succeeded, read off the trace's `Exit`
events — the statute's own account of itself — and marks the `default` as such.

**Why the facts arrive in `input` and not in OPA's `data`.** Issue 44 words the
architecture as "data = sealed ledger + flattened facts; input = one check", and
OPA has two namespaces of those names. The three v1 statutes read
`input.claims` and `input.known`; they are the R1 examples, pinned by content
hash, and R1 closed gated over them. Moving the facts to `data` would edit all
three. So the engine assembles one input document and the sentence is honoured
in the sense that matters: every fact in it comes from the sealed prefix and
nothing else, and the check is the only thing supplied from outside. The
namespace split belongs to whichever phase next edits the statutes.

**ADR 0001 is enforced at the layer that runs statutes.** `Evaluate` passes no
`rego.EnablePrintStatements`, so OPA *erases* print calls: a statute full of
them evaluates clean and silent. That is measured, and it is why the engine
refuses such a module statically instead of running it. `tests/statute` still
asks the other question — whether the committed statutes and mutants call
`print` — with two oracles.

### An absent finding is not a finding of nil

`Finding.Defined` is false when the statute yielded no finding at all, and
`Finding.Says()` is how a reader is told so: the value when one was defined, an
explicit absence when none was. The method exists because the field is easy to
forget and the forgetting is silent — a consumer formatting `Value` directly
prints `<nil>`, which reads as a verdict OF nil rather than as no verdict, and
the difference is this document's own: a finding is a rule that **holds**, while
absence is key-absence. Both consumers of `Finding` ignored the field until it
was measured, so the sentence lives beside it rather than in each caller.

Nothing in the committed corpus reaches the state: every statute carries
`default finding`, nothing requires one, and a `pending` verdict is `Judge`'s
and cannot arrive through `Evaluate`. It is a latent defect closed at the
source, and the tests reach the state with a default-less statute written in
the test rather than added to `examples/`, whose case and package counts
`rumor attaint` asserts.

## `pending` is an overlay, and the clock is a ledger fact

`pending` needs two things no v1 statute has: the bound declared for an
`eventual` claim type, which lives in the bindings artefact (R4), and the run's
time, which is a `clock` fact. So the engine applies it **over** the statute's
finding and says so: the statute's own verdict is kept beside it with the
definition that produced it, and the overlay names `engine.pending` with the
time the bound expires. If a later phase hands the bound to statutes, this
overlay is the single thing that goes.

Four ways it declines, each a decision: agreement has nothing to hold open; an
undeclared type is not eventual; no claim of the type means nothing is late;
and **past the bound the statute's finding stands**, because v1 reserves
`stale` for R6 and defines nothing, so the engine does not invent a verdict it
has not been given. It *refuses* when the bound cannot be measured — an
eventual type with no clock fact at or before its last claim — rather than
reading missing data as expired, which would turn a `pending` into a defect.

**The re-check is keyed on the pair.** v1 re-checks a `pending` "at the actor's
next observation of the same query and at every full reconciliation", so
another query from the same actor does not clear it, and a re-check clears the
old pending whatever the new verdict is: clearing only on agreement would leave
one that had become a defect sitting in the register as though it were merely
late.

**A reconciliation that is not full is refused.** It observes every actor the
sealed prefix names and needs a value for each; a pass that skipped one would be
indistinguishable from a full one. Two readings are recorded rather than
inferred. First, "all honest actors" is read as *every* actor, because v1 tags
claims and not actors and says outright that there is no trust model — the
`with-planted` fixture is one actor holding an honest claim and a planted one,
and excluding her would observe nobody in the very example the done-when names;
which actors hold a planted claim is reported instead. Second, **the run is not
an actor**: v1 gives a `clock` fact's actor as "the run", which controls time
rather than holding beliefs, so clock facts contribute no one to reconcile
against.
