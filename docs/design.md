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
`canard` passes one on, `palter` tells a half-truth, `essoin` abandons a flow.
A lie is therefore scheduled exactly the way an honest action is, which is why
the control in the vocabulary has no trust model to lean on: the engine knows
which claims it planted because it planted them, and the statute does not.

That sentence names all four in one breath deliberately: `tests/validate` reads
it as the oracle for `internal/validate`'s declared set, and R5.1 split it in
two for a while, which left the arm reading `suborn` and `canard` and calling
the document two short.

**A canard is a lie travelling rather than a lie told.** It is a tell that
PASSES ON a planted claim -- by an actor that is not the claim's author, of a
claim the run planted -- so what it buys is a weight and a precondition and not
a third state: how often lies travel is tuned apart from how often they are
made (`suborn`) and how often honest claims travel (`tell`), and the engine
records the hop so R10 can say who passed the lie and not only who made it.

**Each kind's precondition, ruled at coop r5/49.** `canard`'s is that the
teller knows a planted claim it did not make, and where none exists the kind is
inapplicable like any action whose precondition is false. `suborn` and `palter`
are an honest action taken dishonestly -- the vocabulary's planted claim is a
lie an actor tells about its own action -- so their precondition is THE HONEST
ACTION'S OWN: an applicable action that emits something to lie about. An
unconditioned `suborn` would lie about an action the actor could not take,
which is a lie of a kind the vocabulary does not have. `essoin` is
INAPPLICABLE until R9: abandoning mid-flow needs flow state the engine does not
carry yet, so it keeps a declared weight and is never drawn.

**What a drawn kind DOES, ruled at coop r5/58.** A lie is about acting, and the
three differ in whether the system moved. `canard` appends a `told` from the
drawn actor to a receiver the seed picks among those who do not yet know the
claim, and the system never moves. `suborn` does NOT execute: it appends a
planted claim of the type the chosen honest action would have emitted, with the
handles minted as if it had -- and a handle a suborn minted resolves to no id,
so any later step needing it is inapplicable and the walk never asks the wire
for a thing that does not exist *by that route*. `palter` EXECUTES the honest
action and then appends its claim with exactly one recorded field false, the
field named under the tag; in v1 that field is one a statute READS --
`visible_to` where the action's claim carries it, else `expect` -- and a kind
whose action's claim carries neither is inapplicable for that action. So a
palter is an action that happened described wrongly, and a suborn is a
description with no action under it.

> That clause said "never asks the wire for a thing that does not exist", full
> stop, and R5.4 measured it false: the guard is `Resolvable` reading the handle
> table, so it covers a dependency a handle CARRIES and nothing else. Seed 4242
> at 990b3ac plants `member_1.user#1` with a suborn of `create_user`, and
> `create_event` names no user handle -- its dependency on that user is the
> SESSION, which the toy keys by user name. So the handle guard saw nothing, the
> statute's `has_user` was satisfied by the planted claim, and step 7 asked the
> toy for a session it answered 404 to. The walk died where the design says it
> cannot. Two layers can carry such a dependency and only one of them is the
> engine's: a handle is the engine's own bookkeeping, and a session's key is the
> project's. I wrote that the engine's guard therefore stays what it is and a
> precondition modeling the STORE must discount planted claims itself. fable
> ruled otherwise at coop r5/97 and placed it in the engine: a precondition is
> shown the honest claims only, so no statute writes `not c.planted` and the
> latent class above is unreachable rather than avoidable. See "**`claims` is
> NOT as a check sees them**" below. The measurement stands; the remedy was
> mine and wrong.

**The honest `tell` is a pool kind too, and it is not a nemesis kind.** R5.4
added it (fable, coop r5/114) as `canard`'s mirror: the drawn actor passes a
claim it HOLDS — one the run did not plant — to an actor who does not yet know
it, appended as a `told`, and the system never moves. It differs from `canard`
in two clauses and nothing else. The subject must be UNPLANTED, which is the
whole difference between a claim travelling and a lie travelling and the reason
the two are weighted apart. And AUTHORSHIP IS NOT EXCLUDED, where canard
excludes it: passing on your own lie is just lying, while telling of your own
event is the ordinary case and the one the corpus carries by hand —
honest-count's position 4 is alice telling bob of alice's own event, which is
what makes bob able to join it.

It is weighted in the HONEST band because it is honest, and it is listed in
`PoolKinds` rather than `NemesisKinds`: readers asking "does this name belong to
the engine" want the first, and `rumor validate` pairs weights against it, while
readers asking "is this a lie" still want the second, which stays the four
`docs/design.md` names in one breath.

**Why it had to exist.** Before it, the walk's only `told` was canard's, so a
lie could travel and an honest claim could not, and an actor could act only on
what it had made itself. With the handle draw now scoped, that is the difference
between a join by a second actor and a self-join: seed 6's step 8 was the first
and became the second the moment the draw was scoped. Without the kind a walk
can also produce no `explained` finding at all, since `explained` is what an
observer's knowledge not having reached it looks like.

**Both telling kinds carry the receiver half in the pool**, and that was
measured rather than reasoned. The first cut left it to the step and said so in
a comment — a subject nobody still needs telling of is not a tell anybody can
make, but the pool "cannot see" the other actors. Driven over eighty seeds it
aborted the walk in THREE of seventy-seven completing runs: a telling step drawn
for an actor whose only claim everybody already held. D15 asks for ten steps and
no refusal, so a 4% abort is not a named gap but a flaky drill. `Tellable` now
gates both kinds and filters the draw through one function, which also closes
`canard`'s identical hole — its own error message had admitted it.

**A weight of zero on `tell` is not the same as the kind not existing.** The
kind is applicable to any actor holding any claim, so a step whose only
candidate is `tell` yields a pool that is non-empty and entirely ZERO — which
`Pick` refuses rather than skips, where before such a step had an empty pool and
the walk recorded a no-op and carried on. So a project declaring `tell = 0`
turns a skipped step into an aborted walk. Reported rather than worked around.

**An undeclared weight is refused, not read as zero.** Zero means never chosen
and the project decided it; absent means nobody did. Defaulting either way
would stop the walk's composition being a function of the weight file, and
`validate`'s nineteenth arm pairs every declared action and every nemesis kind
against the table so a project hears it before a run.

**The run names its own actors, `<role>_<n>`.** `[[venire]]` carries a role
and a count and nothing else, so the engine mints `admin_1`, `member_1`,
`member_2` in venire order, n from 1 per role (coop r5/58). Venire order rather
than the draw's, because HANDLES EMBED THE ACTOR and two attempts from one seed
must agree on who `member_2` is before either draws anything. The hyphen is not
available: the vocabulary's handle shape is `actor.type#n` with the actor
lowercase alphanumeric and underscores, so `member-1` could not carry a handle
at all.

**Resolvability is the second half of applicability.** A precondition says the
world permits the action; a resolvable handle says the walk can name what the
action acts on. An action that names a handle no bound handle satisfies is
INAPPLICABLE rather than drawn and then refused -- which is the same refusal
`observe`'s args already make, arriving one layer earlier so the walk never asks
the wire for a thing that does not exist. The handle table is the whole
mechanism: it holds only handles something bound to an id, so "resolves" and "is
in the table" are one question. There are TWO ways an action names a handle and
both count: an argument declared with `source = "handle"`, whose kind the
request cannot be written without, and a `{kind}` in a UI flow, which the flow
cannot be navigated without. The argument is reported first where both are
missing, because an unwritable request stops the step sooner than an
unnavigable page.

**An action's arguments come from four declared sources.** `[[action.args]]`
gives each argument a name, a source, and that source's one field: `handle`
with `of` (a handle KIND, which the engine resolves to an id), `literal` with
`value`, `draw` with `from`, and `actor` with nothing else. Exactly one field
beside `name` and `source` is accepted, so a project saying two things about one
argument is refused rather than guessed at. The fourth source exists because the
wire carries no actor field -- `token` is who acts -- and a system that keys its
sessions by user name still needs the name in the request; it consumes NO draw,
because a source drawing from the generator would make the sequence depend on
how many arguments an action declares, and two projects with one seed would walk
apart for a schema reason. A `handle` argument DOES draw, among the bound
handles of its kind: its value is not determined, several can be bound, and
something has to pick -- the same position a `draw` argument is in. The handle
choices are drawn before the other arguments are composed, and a step records
what it consumed as well as what it created, so a drawn choice can be compared
between two attempts.

**A session per actor per context, acquired at first use.** `token` is present
exactly when the action's context is not `"none"`, and the engine acquires it
for that context -- which is how the engine holds a pairing the wire decoder
deliberately leaves to it. First use rather than up front is what the system
under test allows, not an optimization: where sessions are keyed by user name, a
session for `member_1` can exist only once some action has made a user called
`member_1`, so acquiring every session before the first step would ask for all
of them before the walk had created anybody. One per actor per CONTEXT and not
one per actor: the context is what the token is for, and reusing an actor's
`api` token for its `ui` actions would send a token the adapter issued for
something else. An empty token is refused rather than held, because an omitted
token and an empty one are the same bytes, so the execute would be
indistinguishable from one taken outside a session.

**An action in a context the actor's role may not hold is inapplicable.** The
third half of applicability, and inapplicable rather than attempted for the
reason the other two are: the engine would otherwise acquire a session in a
context the project says this role never has, and an adapter that answered
successfully would make the project's role model unfalsifiable from a walk.
Context `"none"` is held by everybody, because it is not a context -- there is
no session for a role to be entitled to. An absent role and a role holding
nothing are different statements, as with weights: a declaration that exists and
omits the actor's role is REFUSED, since reading it as holding nothing would
make every context-bearing action inapplicable and a walk stalling for a missing
`[[role]]` would look exactly like a walk whose world permits nothing. A project
declaring no roles at all is not gated here; pairing a context against the roles
that hold it needs the bindings alone, so it belongs in `validate`.

**And it is the FIRST half rather than the third, over the project's actions
rather than the applicable ones** -- ibis found the ordering at coop r5/93 and
fable placed it in the engine at r5/95. The refusal above is a fault in the
PROJECT, so it must not depend on what the statute says this actor may do right
now: reached only for an already-applicable action, it could never fire for an
actor nothing is applicable to, and R5.4's own `has_user` makes that actor
ordinary rather than exotic. A minted actor holding no user has no `api` action
applicable, and `create_user`'s context is `"none"`, which returns before the
role is looked up -- so a missing `[[role]]` and a world permitting nothing were
the same answer again, which is the fault the refusal exists to prevent. Running
the gate first also narrows honestly: a precondition is no longer evaluated for
an action this role could never take.

**And a `handle` argument resolves only to a handle the ACTOR knows of.** The
fourth half, closed in R5.4 after the walk reached the gap. `Arg.Of` has always
been documented as resolving to "an id the ACTOR knows of" and the engine did
not do it: the handle table is global to the attempt, so any bound handle of the
right kind satisfied the argument. `Resolvable`'s own comment stated the
narrowing and promised a filter "one layer up" that was never built — ratchet
read it correctly as a declared narrowing rather than an oversight (coop
r5/109), which is what it was, and mine. The walk then reached it: seed 6's
step 8 had `guest_1` join `member_1.event#1`, an event nothing had told it of,
because `knows_of_subject` passed on guest_1's OWN event while the draw took
member_1's (ibis, r5/111). fable ruled the scoping into the engine at r5/114 —
"the precondition and the argument quantify over one set" — so a walk can never
pass `may_join` on event A and send event B.

The instrument is `engine.KnownHandlesAt`, which is `KnownAt` read at the level
of handles rather than refs: a claim names more than its own ref since r5/105,
so an actor told a join knows OF the event, and scoping on refs alone would
leave a told actor unable to act on what it was told. One function,
`UsableHandles`, answers it for both readers — the pool asks whether the set is
non-empty and the draw draws from it, and two intersections computed apart would
put back the drawn-and-then-refused shape this closes.


**A claim's handle is the single `creates` entry, else the action's own name**
(coop r5/62): `member_1.event#1` for `create_event`, `member_1.join_event#1`
for `join_event`. An action declaring more than one `creates` or more than one
`emits` is REFUSED by name until a project needs it, at which point the primary
is declared and not guessed. A step whose action emits nothing appends no claim
and mints no handle, and is recorded as a step -- examples/twin's
`publish_doc_ui` is the live case. The hand-written refs in `examples/`
(`bob.join#1`, `alice.del#1`) are a case's own names and no precedent for what
a run mints; a run compares findings over its own handles.

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
buggy application in `examples/toy/` -- a JSON API AND a server-rendered UI
over users, events, roles and capacity, which the pointer dropped and both the
epic's item 9 and issue 73 carry -- whose design is R5's own work in the
`r5.toy` thread and whose bug ledger is in the epic). The verify step is not
optional: a shrink step run on a world the previous step dirtied would
attribute to the trace what the leftover state did, and the "minimal" trace
would be minimal only in that world. Shrinking respects bounds, so a trace is
never shrunk to one that could not have produced its finding under an eventual
type's bound -- R6 later, since the bound is R6's machinery and the first
eventual toy bug is R6's too. The result is written as a minimal trace, which names the
output's shape (the actions, their positions, their handles and the rules that
fired) and not a property of it; the property R5 is held to is that each of its
three toy bugs shrinks to under five actions with every step on a verified
clean rollback.

**What "the same finding" means.** The shrinker keeps a candidate only if it
reproduces the same finding, and that identity is stated here because R14 later
builds its deterministic-or-unstable classification on it and the first
shrinker would otherwise choose silently. Two findings are the same when the
rule that fired, the finding's value, its attribution (`rests_on_planted`) and
its kind all agree.

**The kind was added to this sentence after R2 wrote it, and the warrant is
recorded rather than assumed.** This paragraph named three parts until fable
ruled the fourth in at coop r6/15 -- "the renderers follow the comparison's
basis" -- and `engine.Finding.Says` was written to carry it, because a
renderer showing the value alone makes a `stale` and a plain `bug` look
identical; `internal/engine/engine.go:230` states the four in those terms. A
sentence left at three while the code compared four is the kind of drift that
reads as agreement: rook drove the question at coop rumor/1225 precisely
because the document and the renderer disagreed and neither said which was
current.

**Identity is four; the CONTROL is five.** A control run is a different
question -- a run against its own recorded self -- and there the step the
finding was found after is the walk's own fact, so the comparison is the four
identity fields plus the step (fable, coop rumor/1039). Ruling 3 asks for one
thing more than either: no trailing pending, which no field on the FOUND line
carries at all. Do not read this paragraph as the control's basis.

The attribution is part of the identity because `bug` resting on a
planted claim and `bug` resting on none are different findings sharing one
value, which is the difference the corroboration arm exists to see; a shrinker
that let a trace drift from one to the other would be shrinking toward a
different defect.

**And when the attribution cannot be compared, the operator is told rather
than refused.** A project with no planted claims need not define `rests_on_planted`
at all -- `rumor attaint` requires it only of a project carrying planted cases
-- and then neither the original finding nor the replayed one carries it, so
"the same finding" is established on the rule and the value alone. That is not a
failure of the comparison: with no attribution, two `bug` findings reached
through `default finding := "bug"` differ in no field a finding carries, so
there is nothing to distinguish. It is a narrower claim than the full one, and
the shrinker says so in a line beside the trace rather than refusing a
legitimate project or letting a narrower agreement read as a whole one.
Measured 2026-10-24: the gate recorded this and nothing downstream read it, so
an operator could shrink on a partial reproduction and never learn it was
partial (ratchet, coop r5/203; the remedy placed at r5/206).

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
`rumor explain` (R10). `rumor check` is the same walk over **every** worked
example under `examples/`, so the claim can be re-run by anyone holding the
binary.

> This said "the three worked examples" until R5.2, and the corpus had been four
> since the permitted twin landed in v0.1.20 and is five with the toy. A count in
> prose that nothing checks is a count that rots: the sentence now names the set
> rather than its size, and the sizes that ARE asserted live in the suites that
> can fail when they move.

**What a statute receives, and what it does not.** `Input` projects each claim
in the sealed prefix as `{ref, actor, type, position}`, plus `planted` and
`visible_to` when present, and the observation as `{actor, position, of, query,
source, value}` plus `role` when the project has roles, and **`handles` when the
claim carries any** — the run-local names, so a rule can ask which entity a
claim is about.

Every package under `examples/` is therefore **single-subject**: one event, one
document, one count per case, and the counts in a statute need no filter because
there is nothing to filter against. That is a limit on RUNS rather than on cases
and R5.2 measured it by hitting it — a walk that creates two events writes joins
for both into one ledger, and a statute that cannot tell them apart would count
across both. R5.3 needs `handles` in this projection, or an observation that
names its subject in a field of its own.

**That fork is closed and the projection now carries `handles`.** fable ruled it
at coop r5/16 — the run-local names, never ids, which is already how the
vocabulary compares findings (rule, value, attribution and kind, over
handles), so
projecting them follows the discipline and projecting ids would breach it. A
statute may then group claims by entity, and "an actor can join only an event it
knows of" means something with two events in one ledger.

**A claim's `handles` are its own name plus what it is ABOUT**, and that is the
corpus's convention rather than a new rule: measured over `examples/*/cases`,
ten claim objects carry more than one handle with the extra one naming the
subject (`bob.join#1` carries `["bob.join#1", "alice.event#1"]`), nine carry
exactly `[ref]`, and forty-three carry none. The key is therefore present only
when the claim has one, which is also `ledger.Fact`'s own `omitempty` — a claim
with no handle has nothing to say about its subject, and giving it an empty list
would make the engine's input differ from the committed file for no gain.

The arm is R5.3's and it is driven: two events in one ledger, one finding each.
Event A has two joins and B has one, so the three counts 2, 1 and 3 separate a
per-entity statute from a spanning one — and with the projection removed the
statute knows of no events at all, so every count reads as spanning and BOTH
clean cases turn into bugs. **The WALK FOLLOWS THE CONVENTION, since R5.4.** This paragraph said it did
not, and that it was "one line in R5.4's files" still to land -- which is why
the done-when's toy third was recorded as waiting on it. It landed in that
release: `internal/schedule`'s honest() writes `claimHandles(done.Handle,
step.Used)`, the minted handle first and then what the step consumed, and
v0.1.27's changelog carries it as "a claim carries the handles it names". So a
walked join does name its event, and the toy third waits on nothing here.
Measured 2026-10-24, reading the call rather than the note.

**What a PRECONDITION is shown is a partial observation** (coop r5/16).
`input.observation` carries `actor` and `position` -- `seal+1`, the position the
action would occupy -- and nothing else: `of`, `query`, `source`, `value` and
`role` are ABSENT, never empty, so a rule reaching for a field a candidate step
cannot have goes undefined rather than reading `""`. `known` is exactly as a
check sees it. The key stays `observation` because live rules
read `input.observation.position`, and a new key would rewrite every
precondition in every project for nothing. Omitting the key entirely was
considered and died on measurement: with no `observation` the held claims are
empty and the rules' DEFAULTS answer, which is silently wrong in both
directions rather than undefined.

**`claims` is NOT as a check sees them: a precondition is shown the HONEST
claims only** -- fable's amendment to r5/16 at coop r5/97, after R5.4 drove the
hole. The two inputs ask different questions. A precondition asks what the
system can be asked to do next, and a suborn never moved the system: the lie is
the claim (r5/58), so a planted claim is a description with no action under it
and the wire knows nothing of it. A check asks whether an observer's belief
agrees with what the store holds, and it MUST see the planted claim or the
defect the `planted-lie` example exists to carry would be invisible. So the
engine strips what it planted before it asks a precondition, and keeps every
claim when it asks for a finding.

The engine is the only layer that can do it, which is the design's own sentence
read forward: the engine knows which claims it planted because it planted them,
and the statute does not. The projection does carry `planted`, so a statute
COULD write `not c.planted` into a precondition -- and R5.4 measured what that
costs. `has_user` satisfied by a planted `user.created` took seed 4242's step 7
to a session the toy answered 404 to; the one-line statute repair worked, and
`made_event`, `already_joined` and `deleted` were three more of the toy's own
rules with the same defect latent in them, none exercised by that seed. A trap
with four instances in the first project to meet it belongs in the engine, not
in a lesson every project learns once each.

`known` is deliberately untouched on both paths, because it is BELIEF: an actor
fed a lie does know the claim. So `knows_of_subject` reads as it always did, and
an actor told of a planted event is stopped one layer down by resolvability --
the handle a suborn minted resolves to no id. A planted ref may therefore appear
in `known` with no claim beside it in `claims`, which is the intent: every
statute in the corpus intersects the two, so the claim simply is not held.

One evaluator serves both, by construction rather than by care: the walk's
applicability and a case's finding go through the same `EvaluateWith`, because
two code paths running statutes would be free to diverge and an explanation of
a verdict the product did not give is worse than none. D14 is the drill that
reads it -- the toy's preconditions agree rule by rule across both inputs, with
the ruled-out shape driven in the same suite and required to DISAGREE, so the
agreement cannot pass on an input unable to tell the two apart. The agreement
has exactly one exception and D14 carries it as a cell: with one planted
`user.created` in the world, `has_user` is false to a precondition and true to a
check at the same position, and `may_create_user` is the reverse. The honest
claim is the control beside it, so the difference is attributable to `planted`
and not to the fixture.

**The empty world is a sealed prefix.** A run's first step is evaluated against
nothing having happened: an empty ledger sealed at 0 is a prefix, and a
precondition asked there sees no claims, no knowledge, and an observation at
position 1. A guard reading only the number cannot tell that from a ledger
holding facts with none sealed, which is a sequencing error; the count
separates them.

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

Three ways it declines, each a decision: agreement has nothing to hold open; an
undeclared type is not eventual; and no claim of the type means nothing is late.
It *refuses* when a DURATION bound cannot be measured — an eventual type with no
clock fact at or before its last claim — rather than reading missing data as
expired, which would turn a `pending` into a defect.

**There were four, and R6.1 spent the fourth.** The fourth decline was "past the
bound the statute's finding stands", because v1 reserved `stale` for R6 and
defined nothing, so the engine would not invent a verdict it had not been given.
v2 defines it, so past the bound the overlay now returns `bug` of kind `stale`,
naming the claim type, the position of the claim it was late about, and the
bound it passed. The word arriving is what changed; the shape did not.

**And the bound has two forms, both judged as of R7.1** — fable's ruling at
breakout-r6/0 gave R6 one of them. A duration bound needs the run to control
the clock, and R7.1 is that arrival: the walk writes `clock(position, t)` at
every seal where the project's `[clock]` table declares control, `t` in
nanoseconds from the run's own epoch, which is zero at the first seal and
moves only when the walk draws `advance` (R7.3). So the comparison this
paragraph described as waiting now runs, and the refusal above fires only
where a project declares no clock.

**A SECOND REFUSAL ARRIVED WITH IT, and it is the run's rather than the
project's** (fable, coop r7/15). A pending still unresolved at the END of a
run, under a duration bound, with the clock where it started, is not inside
its bound: it is UNMEASURED, and R7's third ruling is that nothing is read as
"time did not pass". The run exits could-not-run, naming each pending, its
bound, and that no `advance` was drawn — because R2.3's invariant is that no
run ends with a pending open and exits 0, and a stuck clock does not bend it.
It is keyed on the bound's FORM and the clock, not on a pending being open, so
the quiesce state below is untouched. Where the run also found a bug the
refusal NAMES it without reporting it: a finding with a pending still
unmeasured beside it is where the walk stopped asking rather than what the run
causes. The QUIESCE form needs no clock at all:
it asks whether the system has settled, so a disagreement is `pending` until the
declared probes read quiet and `stale` if the reconciliation that follows still
disagrees. The engine takes the prober's answer as an INPUT on the check and never
measures it, which is what let this land before the probes existed.

**That answer has FOUR values and is not a bool**, which ratchet's finding
forced and fable ruled: a probe that ran and GAVE UP and a system still BUSY
would otherwise hand in the same byte, two causes with one encoding, and only
one of them is safe to read as "not quiet". So `busy` holds the disagreement
open, `quiet` makes it `stale`, `timed out` is the slot R6.2's own finding kind
fills — refused here rather than named, exactly as `stale` was refused before
this phase defined it — and `absent`, the zero value, is a refusal and never an
answer.

**And a bound nothing can judge is refused at the run, not carried quietly.**
ratchet drove the alternative: a claim type bounded `quiesce` with no prober
wired holds a real disagreement open on every pass — the register keeps it,
`StillOpen` cannot see it because the query WAS re-asked, and the walk never
ends on a `pending` — so the run exits 0 on a disagreement, which is worse than
a missed bug. `run` therefore REFUSES such a project by name. The condition is
structural rather than a sentence about this build: it is the prober slot being
nil, which R6.2 falsifies by construction when it wires one in, so the shutter
removes itself instead of waiting to be remembered. `StillOpen`'s proxy — "was
the query re-asked" standing in for "was the pending resolved" — is sound only
while every bound expires on its own.

**THAT PROXY EXPIRED AT R7.1, FOR THE REASON THIS PARAGRAPH GAVE AND NOT IN
THE PHASE IT NAMED.** R6.2 was expected to close it; what closed it was the
duration bound becoming judgeable. A re-ask does not RESOLVE a pending, it
re-judges it, and a duration bound whose run has not moved its clock re-judges
to `pending` — so the reconciliation ran, the prober was present, the query
WAS re-asked, and the pending was still held. One register read twice then
gave a single pending two contradictory sentences, "never re-checked" from the
still-open enumerator and "re-checked after the last step: pending" from the
reconciliation (rook, coop rumor/1264). `stillOpen` takes the re-asked keys
now and words nothing for a pending that was asked and held, so the proxy is
replaced by the fact it stood in for.

**MEASURED, NOT ASSUMED, which is what this section is amended with.** Past a
quiesce bound the engine judges with a ledger carrying NO clock fact at all —
the same ledger that refuses outright for a duration-bounded type, which is the
discriminator between the two forms rather than an accident of a fixture. A
`stale` is terminal: it is not registered for re-check where a `pending` is, so
no run ends with a decided finding sitting in the register as though it were
still late. And the three findings the engine already produced are unmoved —
toy seeds 2, 40 and 1 are byte-identical to v0.1.31's output (786416e6925acf7c,
dff917a0f6c74983, ba42d0333035a0aa), which is R6's control for every child.

**The control carries its invocation, because without it the control bites the
reader it was written for.** It is `rumor run examples/toy --seed N --steps 10`,
stdout only, `sha256` truncated to sixteen. The flag is load-bearing and the
figures above are the flag's: with default steps, seeds 2 and 40 are
byte-identical to the triple above and seed 1 is `80d212c53f606e6d`. So a reader
re-running this from the numbers alone matches two of three and reads the third
as a regression of their own — which happened in R6.2, cost an hour, and was
settled by ratchet naming the flag at coop r6/4–/5. A partial match is worse
evidence than none: a full mismatch would have been read as a method error at
once, where two-of-three bought a confident wrong conclusion about the record.

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
