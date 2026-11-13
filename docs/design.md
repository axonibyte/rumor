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
INAPPLICABLE until the engine carries flow state to abandon, so it keeps a
declared weight and is never drawn. R9.1 gives the walk a PENDING FLOW — a
browser flow stopped at a wait, carried across one step boundary — and that is
not yet the state `essoin` needs: abandoning is a drawn kind with its own
record, and a pending flow is the engine's own retry in progress. The refusal
stays by name rather than being widened to fit the nearest available state.

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

**`advance` is a pool kind too, and it moves the RUN rather than the system.**
R7.3 added it. Every other kind in the pool is something an actor does to the
system under test; this one moves the run's own clock, so no actor in the
system does it and nothing in the ledger reads as an action. It is listed in
`PoolKinds` and not in `NemesisKinds` for `tell`'s reason -- it tells no lie --
and it is weighted like everything else, because the walk's composition is a
function of `platform.toml` and nothing else.

**It is live IFF there is an instant to land on** (fable, coop r7/42): the
project declares `[clock]` AND some live pending's bound gives the clock
somewhere to go. With no target it is INAPPLICABLE rather than a no-op, so it
is never drawn into nothing and a run that cannot move time spends no step
pretending to. Both halves reach the pool as ONE argument, the landings
themselves, because the gate and the draw must quantify over one set -- a gate
over BOUNDS would say live where the draw picked an instant the mover refuses.

**The three landings, and the middle one is why there are three.** A drawn
advance lands one tick before a bound, exactly ON it, or one tick past; a tick
is one nanosecond, the contract's unit. The overlay holds a disagreement open
while `now-claimTime > bound` is FALSE, so the bound's own instant is the one
input that separates a strict comparison from a loose one -- at a tick before,
both readings hold it open, and at a tick past, both let it go. That is the
hinge, and a walk reaches it on purpose rather than by luck.

**What a drawn advance DOES.** It asks the adapter to move the clock by a
delta and writes the ledger's fact from the ANSWER, not the ask: an adapter
whose clock has a coarse tick answers for the world, and the world is what the
ledger records. A `set` must come back exactly, since the engine named an
instant; an `advance` need not, since the engine named only a distance. What is
refused is a clock that went BACKWARD -- replay and every duration bound rest
on a ledger whose times never decrease -- and the step records the instant it
drew, so two attempts of one seed can be compared on where the clock was SENT
as well as where it arrived.

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
cannot be navigated without. AS OF R9.1 THE FLOW'S PLACEHOLDER IS ALSO
SUBSTITUTED before the steps go out, to the id the action's OWN ARGUMENT chose —
so nothing is drawn for it and the page cannot be pointed at one entity while
the request named another. Until a flow was actually sent, nothing filled them
in: the kinds were read for applicability and the literal would have reached the
adapter. The argument is reported first where both are
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
`api` token for its `browser` actions would send a token the adapter issued for
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
over users, events, roles, capacity and -- as of R7.4 -- expiring invitations,
which the pointer dropped and both the epic's item 9 and issue 73 carry -- whose design is R5's own work in the
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

**A REFUSED ACTION APPENDS NO CLAIM, as of R7.4** (fable's ruling, coop
rumor/1405 item 2, on ibis's measurement at r7/64). The walk reads what the
system answered: an accepted action appends its claims, and one the system
declined appends none and is RECORDED AS REFUSED in the step, carrying which
action, which declared key and what came back. Until R7.4 the walk appended on
the action's `emits` alone and never read the answer, so a refused step
recorded a claim the system never made and every finding downstream of it was
the walk's bookkeeping -- measured: with `join_event` wedged at 409 the toy's
clean seed 1 produced a bug whose shrunk trace ended at the refused step, and
it produces none now.

**It changes what the walk DOES next, and that is the mechanism working.** A
refused join leaves the actor able to join again, because `may_join` reads the
ledger and the ledger now reflects the world -- so two runs differing only in
the system's answer diverge from that step on. Before, the scheduler believed
a claim nothing had done.

**A palter of a refused action plants nothing**: a palter is "an action that
happened described wrongly", and where the system declined there is no
happening to describe. Appending the claim anyway would plant a lie the
corroboration control could never clear, since the strip would remove a claim
about an action that never ran -- a suborn's shape under palter's name.

**And a step that appends NOTHING AT ALL makes no observation**, which R7.4 is
what made reachable: the engine refuses a check over an unsealed ledger and a
seal of zero IS unsealed, so a first step whose action was refused would have
taken the walk to a sequencing refusal. There is no question to put -- an
observation compares against what the ledger says happened, and the ledger
says nothing has.

**A statute is handed the run's CLOCK, and that key was missing for three
phases** (R7.4; fable's ruling at coop rumor/1405 item 1). `input.clock` is a
table with `now`, the run's time in nanoseconds at the position being judged --
the latest `clock` fact at or before it, read by the same `timeAt` the
`pending` overlay uses, so the statute and the overlay cannot disagree about
what time it is. It is in BOTH inputs: a check's and a precondition's, because
unlike `of`, `query`, `value` and `role` -- absent from a precondition because
a candidate step cannot have them -- the run's time is a fact of the world at
the moment the question is asked, and each input reads it at its own seal.

**AND EACH CLAIM CARRIES THE RUN'S TIME WHEN IT WAS MADE** -- `c.t`, the
latest `clock` fact at or before that claim's position, present wherever the
run has a clock. It arrived with R7.4's ledger row because `input.clock.now`
cannot compose it: `now` is the time at the SEAL, one number for the whole
evaluation, so a rule reading it says *the run is past the deadline* and never
*this join happened past the deadline* -- and a clock that advanced after an
honest accept satisfies the first while the second is false. The two are the
difference between the application behaving and the application honoring an
expired invitation, which is the row R7 exists to catch. `t` is the
vocabulary's own word for the run's time (`clock(position, t)`), so it has one
name everywhere it appears.

**A HAND-WRITTEN CASE MUST CARRY IT TOO.** A case's `input` block is a mirror
of its committed ledger, and tests/engine drives both paths and requires them
to agree about which case kills each mutant -- so a claim written there without
its `t` makes the ledger path see the row and the case path miss it. That arm
is what caught the omission in R7.4's own case.

**Where the run has no clock the key is ABSENT, not zero.** `internal/ledger`
refuses a `clock` fact outright without declared control, so an unclocked run's
prefix holds none and nothing is added; a rule reading `input.clock.now` there
goes UNDEFINED, which is the state `rumor validate`'s arm 8 refuses the project
for asking about. That is what makes "nothing changes for a project that
declares no control" structural rather than promised.

**IT WAS A HALF CHANNEL AND THAT IS WHY THIS PARAGRAPH IS HERE.** `clock` has
been a ledger fact since R2.3, the vocabulary has said "rules may read `clock`
facts" since v1, and arm 8 has refused a statute for reading `input.clock`
since R4 -- while `Input` built `{observation, known, claims}` and
`prefixFacts` projected claims and tells only. Measured both ways before it was
filled (ibis, coop r7/63): a listed rule over `input.clock` read false on the
case path and never fired on the run path, where the same rule over
`input.known` read true and fired every time. A channel written at every seal
that reaches nothing is the shape of defect this project's own
"every field has a consumer" rule exists to catch, run backwards -- a CONSUMER
with no field.

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

**WHOSE CLOCK IT IS, AS OF R7.2.** The `[clock]` table now says HOW the run
controls time, and the two forms put `t` in different hands. Under
`form = "faked"` the stack is already running under faked time, so the engine
sends nothing and its own `t` is the world's -- which is what R7.1 did
everywhere. Under `form = "adapter"` the engine SETS the system under test's
own test clock through the wire's `clock` request at every seal, and the fact
it writes carries the time that came BACK: an adapter that answered for the
world is the authority on what time it is there, and a ledger holding the
engine's intention instead would record what was asked rather than what
happened. A run whose clock the adapter cannot move, or moves backward, or
sets to something other than the ask, is refused by name rather than recorded
-- the engine never writes a fact it did not get an answer for. A project that
declares the table and no form is refused before a step is taken, because the
engine cannot ask for a clock it was not told how to reach.

**AND THE WORLD'S CLOCK LIVES WHERE THE WORLD DOES, as of R7.4.**
`form = "adapter"` says the system under test holds the time; it does not say
where in it. In the shipped example it is a field of `cmd/toy`'s STATE FILE,
and that is the rollback design rather than a placement: a checkpoint is a copy
of that file and a restore is a copy back, so a clock held anywhere else --
in the adapter process, which is where this one was until R7.4 -- would survive
a restore, and the world after one would be the checkpoint plus however far
time had been moved since. The adapter forwards `set` and `advance` and answers
what came back, so the relativity of an `advance` is a property of the world
and not of the process relaying the request. Moving the clock is a WRITE and
drains no queued work: R6's lag advances on the request SEQUENCE, so the two
phases' mechanisms stay separable and a run can exercise either alone.

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

**R7's done-when carries its own invocation, and BOTH flags are load-bearing in
opposite directions.** The row R7.4 was built to find — an invitation honored
past its expiry — is found from the committed seed 3 under
`[profiles.expiring]`, and the invocation is `rumor run examples/toy --seed 3
--steps 24 --profile expiring`, stdout only, `sha256` truncated to sixteen, and
from a project directory whose basename is `toy`, because the summary line
prints that basename and it is therefore part of what is hashed:
**a2c20f20769767d5**, twice byte-identical, exit 3, FOUND after step 23 and
REDUCED 23 steps to 8 with the crossing advance at step 7. `toy` and
`toy-adapter` must be on PATH beside `rumor`, since the toy's surface is those
two binaries; without them the run exits 1 and stdout is a 12-byte echo of the
seed, which five such echoes will compare equal to each other and to nothing
else. **The basename clause is not pedantry**: the same project copied to a
directory named `mtoy` gives `7231faa04c99e100`, with the exit code, the
finding, the shrink and the trace all identical and ONE line of forty
different. ratchet hit that before reproducing the figure (coop rumor/1451) and
found out only because the row names the path; measured again here, the diff is
line 39 and nothing else. So the row is reproducible from `examples/toy` and
not from a copy under another name — a third way to a complete, self-consistent
wrong answer, after the two flags.

**Drop either flag and the output still looks like a reproduction.** Without
`--profile expiring` the same seed at the same step count finds a DIFFERENT bug
— `4cf91954cbb095eb`, still exit 3, still `rests_on_planted false`, but FOUND
after step 13 and REDUCED 13 to 10 — so the reader gets a plausible find at the
expected exit code and the wrong subject, with nothing in the output to say so.
Without `--steps 24` the default step count ends at `919000a6d96fc090` and
**exit 0**, no finding at all, which reads as the row having regressed. R6.2's
hazard was a PARTIAL match, two digests of three, which a careful reader
notices and investigates. THIS ONE IS COMPLETE AND SELF-CONSISTENT: the wrong
invocation exits non-zero, names a bug, reduces a trace and reports
`rests_on_planted false`, so there is nothing in it for a careful reader to
catch — it reads as confirmation of a different bug. On this row the flags are
therefore not provenance but REQUIRED: a reader who supplies neither is not
reproducing a weaker version of this measurement, they are measuring something
else. That last reading is rook's, at coop rumor/1450, who reproduced all three
digests on a clone built from the tag's own tree.

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

## A zone transition is a landing, and the toy's clock arithmetic is the bug

R7 gave `advance` the three landings of a declared deadline: one tick before
it, on it, and one tick past it. R7.5 gives it the same three at a **zone
transition**, and the reason is that a transition is where a system's own
arithmetic about time stops agreeing with the calendar. The hour that vanishes
in spring and the hour that repeats in autumn are the two instants a day is not
twenty-four hours long, and a walk that never lands there cannot find a system
that assumed it was.

**The project declares the zone and the engine reads the run's instant in it.**
`[clock].zone` is an IANA name; the engine resolves it once, hands the resolved
location to the ledger, and builds `input.clock.local` — a date, an offset and
a DST flag — so a statute can ask what local day the run is in without zone
arithmetic a policy cannot do. The zone never crosses the wire: the adapter
exchange is unchanged and `clock.t` stays nanoseconds.

**Only the NEXT transition joins the landings**, between the run's clock and
the furthest live bound, which keeps the list bounded by the bounds (fable,
issue 115 amendment 3). A project that declares no zone draws exactly the walk
it drew before, which is measured rather than promised: the five rows of
`examples/toy/seeds.lock` that predate this phase walk byte-identically with
the toy's zone declared and undeclared.

### What the toy gets wrong, and why the rule reads the zone's midnight

The toy gains a per-user daily invitation allowance whose next reset it
computes as **the start of the current local day plus twenty-four hours of wall
time**. On an ordinary day that is the zone's own next midnight. On the day a
spring transition shortens to 23 hours it lands an hour LATE, so a user who has
spent an allowance is still refused for an hour into the next day; on the
25-hour autumn day it lands an hour EARLY, so an allowance returns while the
day it belongs to is still running. The error is one expression — `Add(24 *
time.Hour)` where `AddDate(0, 0, 1)` is meant — and it is **re-anchored
daily**, recomputed from the current local date each time the allowance is
touched, so the plant is exactly one day wide and each transition gives its own
finding from its own seed.

**The rule reads the zone's midnight rather than the toy's**, and that is the
whole point of the pairing. The application reports the local clock face of its
own next reset, and a daily reset is at `00:00` by definition — so the statute
compares a string it can read against a constant the vocabulary of a daily
reset fixes, and never recomputes the instant. A second reading, the allowance's
anchor DAY, is compared against `input.clock.local.date`: that one is the two
zones agreeing, and it catches a system configured for a different zone than
the project declares. Two oracles, because they fail differently — the reset
face is wrong on both transition days, and the anchor day only in the window
after a late reset.

**And the reset instant itself is a declared deadline**, named in
`[deadline].instants`, which is what gives a walk a REASON to arrive: without
it the only landings in range are the transition's own and an invitation's
thirty-second expiry, and neither falls in the hour where the arithmetic is
wrong. Spring's discriminating landing is the tick BEFORE the declared reset
and autumn's are ON it and one tick PAST (ibis, coop r7/119) — so a cell
written from one direction alone arms one direction alone.

## The hostile corpus is derived from the declarations

**A corpus that is written by hand tests whatever its author remembered.** The
fuzz stage's cases are a function of the project's own bindings: an action
earns cases because it declares arguments, and the only way to add a case is
to declare something. R8's D1 (rook, coop r8/2) is that drill, and its partner
is the measurement — add a write action to the declarations and the corpus
must grow without this package being edited, because a hand-written corpus
would not have grown. Measured on `examples/toy`: nine write actions, 45
cases, and one added action takes it to 49.

**A WRITE IS AN ACTION THAT EMITS A CLAIM**, written as the predicate rather
than as the count. All nine of the toy's declared actions emit one, so there
is no read-only action to exclude there and an arm over the toy alone could
not tell "all nine are writes" from "every action is fuzzed" — so the arm adds
a claimless action and requires it to earn nothing.

**THE CLASSES ARE BOUNDED BY WHAT A REFUSAL CAN NAME, and two obvious ones are
deliberately absent.** `cmd/toy`'s request decoder sets
`DisallowUnknownFields` and an `io.LimitReader` at 1MB, so an unknown field or
an oversize body is a 400 from `encoding/json` BEFORE any handler sees the
request. A 400 earned that way says nothing about the system under test, which
is why D1's partner requires each class to produce a refusal naming a FIELD OR
RULE OF THE SYSTEM (ibis, coop r8/1; rook r8/3). A class that cannot be
refused by name is not evidence, so it is not generated.

**AN ARGUMENT'S TYPE COMES FROM ITS DECLARATION, AND THE DECLARATION IS
READ RATHER THAN GUESSED AT.** A `literal` carries its value, so its type is
known. A `handle` names a kind and crosses the wire as an id, so a string. An
`actor` is a name. A `draw` carries `from`, and its first element's type is
the argument's type — which is the whole difference between a typed hostile
value and a guess, and it was found by driving the class-coverage arm rather
than by reading the code: `capacity` is a DRAW, not a literal, so restricting
the non-positive class to literals left it DEAD against the shipped project
while the arm still passed. The toy's own rule, "capacity must be at least 1",
was the one rule the corpus could not reach.

**ONE CLASS IS DERIVED FROM TWO DECLARATIONS AT ONCE.** Where a draw's domain
is made entirely of names the project declares as roles, a string in neither
is a value the system has a rule about, and the toy answers by naming the
field: "visible_to names %q, which is not a role". The pairing is what makes
it sound — the domain alone would not say the set is closed, and `[[role]]`
alone would not say this argument draws from it. A domain holding anything the
role list does not earns no such case, because then the system's rule about it
is unknown from the declarations and the refusal could not be predicted.

**THE PERTURBED FIELD IS PART OF THE CASE**, because D1's partner is a claim
about the refusal naming a field, and a case that perturbs an argument without
recording which one cannot check that claim. Every other declared argument is
present and plausible, and an arm asserts it: a class that quietly dropped a
second argument would be refused for the wrong reason while still naming a
field.

**AN EMPTY CORPUS IS A REFUSAL AND NOT A RESULT.** A project declaring no
write action, or write actions with no arguments between them, is refused by
name — because an empty corpus and a corpus nobody generated read identically
in a report, which is the shape D1's partner exists to catch.

**AND A CORPUS NOTHING ANSWERED IS THE SAME REFUSAL ONE LAYER LATER.** The
"no 5xx" clause is read off the results, so a run in which NO case reached the
system produces an empty finding list for the same reason a clean one does: a
server that died after its setup, or an adapter whose surface answers nothing,
would be reported as having found no 5xx over requests the system never
received. Zero reached is a COULD-NOT-RUN and not a finding, for the reason a
refused setup step is — the defect is in the run, and the system under test is
not answerable for a request that never arrived. The line is at zero and not
at a fraction because a nonzero line needs a NUMBER and nothing in the
declarations supplies one: 18 of the toy's 45 cases fault at the adapter in the
HONEST run, its surface genuinely not carrying every declared action, and that
rate is a property of one adapter rather than of the question — a constant taken
from it would refuse correct runs against a thinner surface and clear vacuous
ones against a fuller. Zero is the only line derivable rather than guessed.

## The walk is the `api` stage, and the dispatcher says so

**`[stages]` was declared, cross-checked and never dispatched.** Every
non-test reader of `platform.Stages` was the parser, the profile cross-check
that a named stage exists, one `validate` arm reading `browser.enabled`, and a
count. Nothing in `internal/schedule` or `internal/engine` read it, and none of
`rumor`'s verbs ran a stage: `run` walked, and that was the whole of stage
execution. R8.1 is the first child that must RUN one, so it carries the
dispatch for all of them.

**IT CARRIED THE DISPATCH AND NOT ITS OWN STAGE's CASE, which is the half this
paragraph claimed and R8.3 supplied.** v0.1.43 shipped `fuzz` KNOWN, UNRUNNABLE
and declared by nobody: `internal/fuzz` had no production caller, `Known` read
`"fuzz": false` with a comment saying "runnable when it lands" after it had
landed, and no project selected it — so the stage the child was named for could
not be run by `rumor run`. A child named for a stage ships that stage runnable
or it has shipped a mechanism, which is the rule R8.2's edge was ruled on while
R8.1 had already shipped without it. The wiring is R8.3's first item and
precedes anything of its own; `fuzz` is the third runnable name and the toy
declares it.

**THE WALK IS `api`, AND `api-sim` WAS A STALE NAME FOR IT.** All thirty-six
`platform.toml` in this repository declare `api`; not one declared `api-sim`,
`fuzz` or `concurrency`, which appeared only in `docs/contract.md`'s example.
R8's api-sim child does not add a second loop beside this one — it gives this
one its settling, its time and its periodic full reconciliation — so the
example was renamed rather than a second stage invented (fable, coop
rumor/1477).

**A DECLARED NAME THE DISPATCHER CANNOT RUN IS REFUSED BEFORE THE RUN**, and
the refusal names the stage and lists the set. Nothing else in the stack
objects: `[stages]` is a map keyed by whatever the file says, `decl`'s
unknown-key refusal is about keys WITHIN a table, and `validate` counts the
stages rather than testing membership — so `stages.fuzzz = { enabled = true }`
parsed, counted, and would have been recorded as a stage of the run. The
capability set belongs to whatever can RUN a stage, so the refusal lives with
the dispatcher and happens before a walk whose stage list was already wrong.

**KNOWN AND RUNNABLE ARE TWO FACTS.** `browser` is declared ENABLED by
thirty-five of those files and no build could run it before R9; refusing them
would be refusing a declaration the contract invites. So a known name a build
cannot run yet is neither an error nor a run — it is absent from the
stages that ran, which is what makes "ran" different from "declared". That
difference is the drill's point: a record naming every declared stage
unconditionally satisfies "the record names the stages that ran" and is the
hand-written corpus wearing a different hat (rook, coop r8/14).

**AND THAT DIFFERENCE IS NOW REPORTED RATHER THAN INFERRED** (R8.4, issue 111).
"Absent from the stages that ran" was a fact a reader had to derive by holding
`[stages]` beside a run's output; the report names the selection and every skip
with its reason, so `browser` reads as DECLARED-BUT-UNRUNNABLE rather than as a
stage that quietly did not appear — which is the distinction rook's D13 partner
turns on, and the one a reader cannot make from an absence.

**AND `browser` IS RUNNABLE AS OF R9.1, so the paragraphs above are history and
not the present tense.** The stage is dispatched under its one name, selected
and reported like every other, and where no drivable browser is on the box it
SKIPS BY NAME carrying how it looked — the six browser-and-driver PAIRS,
`RUMOR_BROWSER`, and how many actions declare a flow.

**A BROWSER IS DRIVABLE ONLY WITH ITS DRIVER, and R9.1b's scan looks for PAIRS
because of it** (fable's ruling at coop r9/73, on the measurement at r9/71). The
flows are driven over W3C WebDriver, which is spoken by a DRIVER PROCESS and not
by the browser: chromium and its kin answer `chromedriver`, firefox answers
`geckodriver`, and the six names this scan grew with are therefore two protocol
families rather than six interchangeable commands. Measured on the two boxes
this phase has: firefox 158's own `--help` says its `--remote-debugging-port`
serves WebDriver BiDi and it carries no CDP, and the jail holding it has neither
driver, while forge-01's ci jail holds chromium 156 and `chromedriver` from the
same package.

**SO A BROWSER WITHOUT ITS DRIVER IS FOUND, NOT DRIVABLE — a THIRD state, and
never a met precondition.** The scan that shipped in v0.1.48 took the first
browser name present, which on a firefox-only box would have selected a browser
nothing in this tree can speak to and handed it to the driver's head as a
resolution. The skip now names WHICH HALF is missing, because "no pair at all"
and "a browser one package short" are different things to an operator and only
the second tells them what to install. An incomplete pair does not take the scan
down with it: the first COMPLETE pair answers, so a box with firefox and no
`geckodriver` beside a chromium with its `chromedriver` runs on chromium, and
the pair that was passed over is named only when none completes. A skip is not an absence, which is the whole
point of the two sentences before this one: what changed is that `browser` now
reaches the dispatcher, so "declared but unrunnable" is a reading the report
makes rather than one a reader derives.

**FOUND IS NOT DRIVABLE — AND SAYING SO COSTS NOTHING WHILE PROVING IT COSTS A
BROWSER** (R9.1b's second head, issue 122's deliverable 1). The scan can say a
pair is on `PATH`; only a launch can say it runs, because a complete pair still
fails on a browser short of a shared library, on a user with no writable `HOME`,
and on a `chromedriver` that is a stub or the wrong package. This head's first
cut therefore started the pair on EVERY run, and CI measured the bill: run 1109
died at the step's 1800s budget with no test failing, where the same suite one
commit earlier finished the whole job in 21m17s. `tests/run` makes 55
`rumor run` invocations and ci.yml names it the critical path, so the check put
55 real browser launches into it.

**THE AGGREGATE IS MEASURED AND THE MECHANISM IS NOT.** This paragraph first
divided a 11-to-13-minute figure by the 55 runs and called the quotient a per-launch
cost of about 12s. That was arithmetic, not a measurement, and a direct one
refutes it: chromedriver start, session open, delete and driver stop together
take **0.8 to 0.9s**, three runs on a quiet 8-core box (2026-11-08). 55 of those
is under a minute. The BILL ITSELF is also narrower than first reported: run
1109 was KILLED at the 1800s cap, so its true duration is unknown and only a
LOWER BOUND can be had -- 1800s against 1110's 20m25.6s test step is **at least
9m34s**, unbounded above. "11 to 13 minutes" was a range stated for a quantity
that has no upper measurement, which is the same fault as the quotient. So the
cost is real, its floor is 9m34s, its cause is not per-run
startup latency, and what remains unexplained is how 55 browsers cost that much
on a 4-core jail running a parallel suite — memory and CPU pressure inflating
the whole critical path is the obvious candidate and is NOT measured here. The
cost is reason enough to keep the check off by default; the mechanism is not
claimed.

**SO THE DEFAULT CLAIM IS NARROWED TO WHAT PRESENCE SUPPORTS, AND THE PROOF IS
AVAILABLE ON REQUEST.** The line says the pair is COMPLETE and that whether it
runs here is UNKNOWN; `--browser-check` starts it and reports what happened,
with a failure to start as a skip naming its reason. A diagnostic that every run
pays for is a diagnostic priced wrong, and driving a flow needs a browser
anyway — so deliverable 2 gets drivability as a side effect of real work instead
of as a line of its own.

**ONE SESSION PER ACTOR, AND THE ISOLATION IS THE PROTOCOL'S.** A W3C session
carries its own cookie jar and storage, so two actors in two sessions cannot
see each other's logins without rumor arranging anything — which is why the
driver keys sessions BY ACTOR and answers the same session when an actor asks
twice. An actor acts many times in a walk, and a session per ACT would throw
away the cookies that make the actor one person. The stage's own probe opens a
single session under an obviously-synthetic name, because actor identity
belongs to the WALK: a session per real actor arrives with flow execution,
which is deliverable 2.

**HEADLESS IS THE DEFAULT BECAUSE IT IS THE ONE AN UNATTENDED RUN CAN TAKE.**
`--headed` is a property of the run and not of the project, so the same
bindings run headless in CI and headed on a desk. `--no-sandbox` is NEVER asked
for: measured on forge-01's ci jail, headless chromium keeps its sandbox as an
ordinary user with a writable `HOME`, and the flag is a root-only workaround
that no CI step should inherit from this engine.

**NOTHING IS LEFT RUNNING, AND EVERY WAIT IS BOUNDED.** Sessions are deleted
before the driver is stopped; the driver is signaled, given a grace, and killed
if it does not go. A signal rather than a closed stdin is where this departs
from `internal/wire`'s adapter teardown — an adapter takes EOF on stdin as the
word to stop and a webdriver reads HTTP, so it would never notice. Three bounds
sit in front of a run that could otherwise hang: the readiness wait, which also
answers AT ONCE when the driver has exited rather than spending the full bound
to report the wrong fault; the stage's own probe bound, for a browser that comes
up and then wedges, which no readiness check can see; and the HTTP client's
own timeout behind both.

**AND EVERY REPLY'S STATUS IS READ, WHICH IS WHAT MAKES A REFUSED TEARDOWN
VISIBLE.** A `DELETE /session/{id}` is sent with nowhere to decode a body into,
so a driver answering 500 to "close this session" was indistinguishable from one
that closed it: a browser left running and a teardown reporting success. W3C
errors carry a body too, but only a request with somewhere to put it could ever
see that, so the status is the check that covers every call. Closing is
best-effort per session and unconditional for the process — a refusing session
must not buy the driver a reprieve — and best-effort does not mean silent: the
refusal is carried out to the caller, which is the only thing that can say it.

**THE STAGE AND THE WALK ARE TWO COUNTS, and the report says whose is whose.** A
browser-context action's flow is driven by the WALK — through a real browser
where the run was handed one and through the adapter otherwise, and the adapter
path needs no driver on this box at all; the STAGE is what the browser rides.
So a run can report steps through `flow` in its summary and a skipped browser
stage three lines later, both true — and the stage's line says "THIS STAGE drove
none" for exactly that reason. The earlier wording said "none was driven", which
was true of the stage and read as true of the run.

**THE WALK DRIVES THE FLOW AND THE STAGE OWNS THE BROWSER** (fable, coop
r9/110). The stage starts the pair, stops it, and hands the walk what it drives
through; it does not drive declared flows itself. Which driver a flow reaches is
decided in ONE place — whether this run was handed a browser — and the ROUTE IS
A RECORDED FACT AND NOT AN INFERENCE: a step carries both "a flow was driven"
and "a browser drove it", because a reader who cannot tell those apart cannot
tell a page from a simulation of one. The RESUME GOES TO WHOEVER DROVE IT: a
flow a browser started and the adapter resumed would re-check its condition
against a page the adapter cannot see, and the `StopWait` the engine re-checks
is precisely a condition on that page. One driver for the flow and its resume,
or the re-check means nothing.

**THE ADAPTER REPORTS WHERE ITS WORLD IS, AND THE WALK READS IT THERE** (fable,
coop r9/111). A `navigate` resolves against the system's base URL, and before
R9.1b nothing in the engine had one: `platform.Adapter` carries a command, a
directory and an environment and no address, and the one stage that knew an
address started its own world to get it. The address now rides the ADAPTER'S
DESCRIBE, because the adapter is the one thing that knows where its world is —
whether it started the system itself or was handed `TOY_ADDR` — so the address
lives where the knowledge lives. Declaring it in `platform.toml` was refused as
duplicating what the adapter already holds, where it will drift; making the walk
own the world was refused as moving ownership for a reason that is not the
walk's. `describe` gains an OPTIONAL `base`, absent by `omitempty`, so the
contract stays `"2"` and every adapter that reports none is unchanged.

**AND AN ABSENT BASE IS A NAMED REFUSAL, NEVER A GUESSED HOST OR PORT.** A flow
that navigates against an adapter whose describe carries no `base` is refused at
the walk, by name, saying that the adapter omitted an optional field and that a
host is not guessed here. Nothing between the adapter and the walk carries an
address: `schedule.Browser` has no field for one, so the walk reads it off the
session it already holds and there is nowhere for a default to enter.

**A NAVIGATION TO AN ADDRESS NOTHING SERVES IS REFUSED BEFORE THE BROWSER SEES
IT**, and that rule was bought by a mutant. With the adapter reporting
`http://127.0.0.1:1`, chromedriver answered the navigate `200`, loaded the
BROWSER'S OWN ERROR PAGE, and the click that followed found no element — so the
walk recorded "the page would not perform it" at exit 0, attributing a refusal
to a system that was never reached. W3C WebDriver carries no HTTP status for a
navigation, so the browser cannot be asked; the address is dialed once per run
instead, and the refusal names the adapter as the address's source. WHAT THE
DIAL PROVES IS NARROW AND SAID SO: a listener at that address, not that the
listener is the system under test — an address pointing at the wrong live server
still drives.

**AND THE ADAPTER REPORTS WHERE A BROWSER OBSERVATION IS READ, BESIDE `base`**
(fable, coop r9/120). A `browser` observation names a page and an element on
`describe`: `page` is a path relative to `base` with the subject as a
placeholder — the toy's `/ui/events/{subject}` — and `element` is an element ID
on that page. The driver navigates the ACTOR'S OWN context to the page and reads
the element's text content from the live DOM, and that is the `browser`
observation. The adapter's own read of the served HTML stays `direct`, the
engine's second oracle, and `direct` is refused to an actor by name: a page is
one reader's view and `direct` is nobody's, which is why a direct check carries
no role. Declaring the two in `platform.toml` was refused as coupling the
project to the adapter's UI, where it would drift; reading whatever page the
last flow left open was refused because an observation would then depend on what
happened to be open.

**AND THE ADAPTER DECLARES WHAT A READING MEANS — `number`, `text` OR `count`,
NEVER SNIFFED** (fable, coop r9/122 and r9/124). A browser reads text and a statute compares a
count, so the reading's type is declared beside the page and the element: for
`number` the engine converts a wholly numeric text to a number IN ONE PLACE and
refuses by name any text that is not — `0 of 2` is a refusal, not a bug and not
a zero — and a `text` observation stays text, reaching only judgements written
for a string. The statute is untouched, which is what keeps `a string never
equals a count` load-bearing: what reaches it is a number BY DECLARATION and
never by appearance. A rule that converted whatever looked numeric would make
the type a property of today's data — an attendee list showing one name that
happens to be `7` would become the number seven — so the declaration is what
makes it a fact about the observation. The toy's `#count` holds the number alone
for this reason, with the sentence a reader sees unchanged.

**AND `count` IS THE ELEMENT'S CHILD ELEMENTS**, which is the reading a LIST
has: the toy's `#attendees` carries no figure at all, so its text is neither a
number nor a judgeable string — and counting its `<li>` children is exactly what
the adapter's served-HTML oracle has computed all along. THE SUT DOES NOT CHANGE
TO BE OBSERVED and the statute does not move: a page with an empty list reads 0,
the same figure the other oracle gives, so the two stay comparable. It is the
CHILDREN and not the descendants — a bare `*` from an element finds every
descendant, so a list whose items each carried a span would count twice what a
reader sees as items. An element that is ABSENT is `missing` rather than 0, for
the reason a withheld page is never an empty one: a hidden page read as a zero is
the reading a visibility finding would then rest on.

**AN ELEMENT IS AN ID AND NEVER A CSS SELECTOR**, in the same vocabulary as a
flow's `until`: one word for one thing. A selector would make the adapter's
answer a query language the engine has to implement, and two spellings of "which
element" is how a reader ends up unable to tell a missing element from a
mistyped selector. The wire refuses one by name, and so does the driver. A page
carrying its own scheme is refused for `base`'s reason: one address, in one
place. AND A BROWSER OBSERVATION WHOSE DESCRIBE CARRIES NEITHER IS A NAMED
REFUSAL AT THE WALK — never an invented path or selector — naming the
observation and which half is missing, because that refusal is the project's to
read.

**A BROWSER-CONTEXT ACTION IS CARRIED BY `flow` AND NOT `execute`**, decided by
the action's CONTEXT and nothing else: `docs/contract.md` says such an action
"declares a flow instead of a response", so the two operations are not
alternatives for one action and there is no fallback when a driver is absent. A
fallback chosen by box state would be two paths for one declaration, which is
the silent-substitution class this tree refuses by name.

**A `-changed` WAIT READS ITS BEFORE-TEXT BY LOOKING AHEAD, AND THE STEP IT
LOOKS AHEAD FROM STILL RUNS.** The condition compares against the element's text
"immediately before the preceding step ran", which is a moment already past by
the time the wait is reached — so the driver reads that element before
performing the step in front of it. Where no page is open yet the before-text is
recorded as ABSENT rather than empty, because the ruling makes an element's
APPEARANCE a change and an empty string would reach that verdict by accident.
This branch once ended the loop iteration, which skipped the very step it had
looked ahead from: a flow of `[navigate, wait "x-changed"]` never navigated, and
the wait then compared against a page nothing had opened — a `ui_timeout` for a
page the driver never visited. It fires only where the wait follows the FIRST
step, which is why a fixture whose wait follows a click passes over it.

**AND A PAGE THAT MOVES MID-READ IS NOT A FAILURE.** An element is found and read
in two calls, so a navigation landing between them makes the reference STALE.
For a WAIT that is the thing it is waiting for: the tick takes no reading and the
next one re-finds. For an OBSERVATION the page is re-read once, because a
reading is wanted and the page that is there now is the one to report; a second
staleness is reported rather than looped on. The matcher keys on the W3C error
NAME and not the 404 status, since the same status also carries "no such
element" — and swallowing that would turn a selector matching nothing into
silence. MEASURED: this failed the forge once in six runs while the local gate
passed, which is what a race between two calls looks like from two boxes.

**AND THE ENGINE RETRIES THE WAIT, WHICH IS WHY THE RETRIER IS NOT THE ADAPTER.**
A wait is bounded in the engine's time — checked at each seal and after each
clock advance the engine makes — so an adapter re-checking inside one call has
nothing advance between its two looks and reads the same world twice. The walk
therefore resumes a stopped flow at the TOP of the next step, after the seal and
the clock write and BEFORE any draw: no generator entropy is consumed, no step
slot is taken, and none of the steps before the stopped position are re-sent, so
a seed's world does not move. The SECOND failure of the SAME wait is a
`ui_timeout`; a flow that got further and stopped at a later wait has made
progress, which is why the POSITION decides it and not the word.

**A KIND AN ADAPTER DOES NOT SERVE IS REFUSED FROM ONE SITE**, and that site is
load-bearing for the contract version rather than a tidiness. The version stays
`"2"` through R9.1's `flow` because a version is one string three places compare
EXACTLY with no compatibility window: a bump would refuse an adapter built before
the request AT THE HANDSHAKE, and then an adapter that merely LACKS `flow` would
be refused for its VERSION and never for the thing it lacks. An unmoved version
is what keeps that by-name refusal reachable — so the refusal's own wording is
part of the argument, and a sentence each adapter spelled for itself is one that
can drift out from under it one adapter at a time. It was two sites saying two
things, with only one of them able to satisfy the arm that holds the argument up.

**AND THE WITNESS FOR IT HAS TO BE AN ADAPTER THAT LACKS A KIND ON PURPOSE.** It
used to be `cmd/toy-adapter`'s fall-through, which genuinely lacked `flow` — and
R9.1 ended that, because `examples/toy` declares an action with a flow and a
declaration the walk honors is one the toy must honor too. With it served, no
adapter in this tree lacks the kind by accident, so the live case is the fake
under a lacks-a-kind knob, in the same family as the contract-bump and
missing-field knobs: a NAMED misbehavior and not a mock written for the arm. The
distinction matters because a subject invented to make an arm pass would make the
kind fabricated, which is the objection that shaped this choice rather than one
it ignores. With every request kind served, the toy's own fall-through is
unreachable — and that claim is driven over the table of kinds rather than left
in a comment, because the last default in this tree to carry an unreachability
comment had a false one.

**A READER ASKS FOR IT, AND NEITHER STREAM IS FREE.** The first attempt printed
the selection and the skips on stderr unconditionally, reasoning that stdout was
the constraint; that is half the rule. stdout is the subject of every recorded
digest and of R7's measurement row, and **stderr's EMPTINESS is what lets a
control read a build failure as a build failure** — rook measured it as part of
D7, and a run printing two lines there on every invocation retires that reading
(fable, coop rumor/1597). The reason was already written on
`schedule.Record.Stages` before this child began. So the facts are the record's,
and `rumor report` is where a reader sees them: it prints them and nothing else,
runs no stage, and leaves both of `rumor run`'s streams as v0.1.45 left them
byte for byte.

**AND THE TAXONOMY IS SHARED WITH THE EMPTY-SELECTION REFUSAL BELOW** rather
than copied: two readers deriving "why not" separately would let one state read
two ways depending on whether any stage happened to run. **Both lines print
always, `none` included** — a skip list silent when it has nothing to say cannot
be told from a build that never learned to report one, which is exactly how
`fuzz` reached 108's gate known, unrunnable and declared by nobody.

**AN EMPTY SELECTION IS A REFUSAL, NOT A RUN THAT DID NOTHING**, and it names
the reason per stage. The first version of that message listed the sets and not
the cause: with a profile named it read "selects api, and this build runs api"
over a run that took no stage — true in every clause and useless, because the
fact that explains it is that `api` was declared with `enabled = false`. Before
that it was worse, reporting the PLATFORM's declarations under the profile's
name. Both were found by driving the refusal rather than by reading it.

**AND THE STAGE REACHES THE POOL, which is what `[weights.stage.<name>]` was
for.** `Weights.Weight` skips the stage table entirely when the name is empty,
and nothing ever filled it, so every per-stage override was dead in the same
way the stages were. Setting it moves no recorded figure today because no
project in the tree declares one — measured before it was wired, which is the
control for that line — and the profile's table still wins over it, so a
profile naming a kind keeps its weight.

**AND THE DISPATCH CONSUMES EVERY STAGE THE SELECTION NAMES.** `Select`
returned a list from the day it existed and `cmd/rumor` read one element of it,
walking unconditionally: so the second runnable stage would have had a project
enabling both run one and drop the other with no refusal and no skip reason.
The arm that fails in the same change as the flip is what caught it, and the
fix is the loop it asked for — a case per stage, the walk among them, running
only when `api` is selected. The statuses across stages are ranked for
shrinkFound's reason: a refusal stops the run where it is found, a finding is
the product and a later stage's could-not-run does not unfind it, and a
could-not-run otherwise outranks a clean exit.

**A STAGE THAT NEEDS THE WORLD ASKS THE PROVIDER FOR IT.** The walk drives one
session and an adapter spawned alone starts a system of its own, so a fan-out
cannot be built out of the walk's arrangement: the world has to belong to
something that can hand out its address. In this binary that is the
`[[rollback]]` provider, which is what the shrink already uses for its own
reason. A project declaring none is told so and the run is not failed by it —
its declarations are correct and simply do not reach a world the stage can use,
which is the same reading the shrink takes of the same absence.

**THREE BOUNDS CAN END A WEBDRIVER CALL AND THEY WANT DIFFERENT ANSWERS, so the
failure names WHICH.** A call made while driving a flow can end on the FLOW'S
BUDGET — the deadline the flow was driven with, which is `waitBudget`'s reading
of fable's ruling at coop r9/110 rather than a second meaning — on this
package's own `RequestBound` of 30s, or because the run was CANCELLED. Raise the
step's budget, look at the driver, or do nothing: a reader handed only "context
deadline exceeded" cannot tell which, and that is what r9/154's contended red
left unreadable. `RequestBound` is named rather than inlined so the sentence can
cite it, and it sits below the wire's own 60s request timeout on purpose, so a
driver that stops answering is this package's finding rather than the engine's.

**AND THE ERROR AND THE CONTEXT ARE READ TOGETHER, because neither alone is a
diagnosis.** Reading the CONTEXT alone dresses a dead driver as a budget: a
`connection reset by peer` arriving while the budget happened to have expired
was reported as the budget, in a sentence that then cleared the driver
explicitly (rook, coop rumor/1973). Reading the ERROR alone cannot work either,
measured on go1.26 and again on go1.27.2: Go reports the client's OWN timeout as
`context.DeadlineExceeded` as well, and `os.IsTimeout` holds for both, so no
predicate over the error can say whose bound fired. So the test is conjunctive —
the error says whether a bound ended the call, the context says whose — and a
failure no bound explains passes through naming BOTH facts when the context is
also done, since the two are true together and neither caused the other.

**A BOUND IS DRIVEN BY A FAKE THAT WAITS FOR THE CALLER TO HANG UP.** The
protocol fake's `wedge-element` mode blocks on the request's own context, so an
expiry fires exactly when the budget does rather than after a sleep chosen to
outlast it; `docs/testing-methodology.md` carries why that shape and not a
sleep, and why the classifier's arms are a grid over both inputs rather than a
list.

**THE WATCHDOG HEARS THE PAGE OVER WEBDRIVER BIDI, BECAUSE CLASSIC WEBDRIVER
CANNOT** (R9.3, fable's ruling at coop r9/164, on the measurement at r9/163).
A W3C classic session carries no console and no HTTP status, so issue 124's
5xx-and-console watchdog needs a second channel. Measured with chromedriver
156.0.8078.79 against a page that logs `console.error` and fetches a 503: both
Chromium's `goog:loggingPrefs` log endpoint and BiDi (`webSocketUrl: true`)
report both faults. The log endpoint is Chromium's alone, so it would give the
stage a second path for one family, which r9/73 rules out; BiDi is the
standard and `geckodriver` speaks it too. The two events the watchdog reads are
`log.entryAdded` and `network.responseCompleted`.

**BIDI RIDES A WEBSOCKET, AND THE WEBSOCKET IS IN-TREE RATHER THAN A THIRD
DIRECT DEPENDENCY.** The standard library has no websocket client and neither
direct dependency carries one. Taking a library for it would add a standing
cost to everyone who audits this tree, as the dependency sections above put it,
and BiDi needs only a fixed subset of RFC 6455. So `internal/websocket` is a
client and nothing more: text frames only, masked as section 5.3 requires, with
continuation fragments reassembled, pings answered, and close echoed. It
offers no extension and no compression, and a server that answers with either
is refused by name, as is a `wss://` URL. The ruling bounded it at about three
hundred lines; an extension BiDi turned out to need would have brought the
dependency case back to this page before the cut, not after. Its arms build
and parse the server's frames from the RFC independently of the client's
encoder, so the two cannot agree by sharing a mistake. The accept-key
arithmetic is checked against section 1.3's own worked example.

**THE WATCH HEARS FAULTS AND RECORDS NONE** (`internal/browser/watch.go`).
Every session asks for `webSocketUrl`, and `WatchFor` opens that actor's BiDi
connection and subscribes it to the two events. A driver that answered with no
URL is refused by name, because a watch that heard nothing from it would read
as a clean page. What a fault MEANS is issue 124's deliverable 3: the
`page_fault` finding, its attribution and the vocabulary row are the R9.3
lead's, so this layer decides only what COUNTS, which is a fact about the
protocol. Two rules, both measured at r9/163:

- **By level and status, never by presence.** With BiDi on, chromedriver adds
  an INFO entry of its own ("Launching Mapper instance") to every session, so a
  watch that fired on any entry would fire on every clean page. A console entry
  counts at level `error`; a response counts at 500 and above, so a 404 stays
  the page's own business. A fault's text passes the same token redaction as
  every other line, because a 5xx on a navigated page carries the token in its
  URL.
- **The step boundary is a barrier, not an arrival time.** BiDi delivered
  entries logged before the subscription, so "arrived since I asked" is not
  "caused by this step". `Since` sends a command and waits for its reply. The
  connection is ordered, so every event the browser sent before answering has
  arrived by then. The flow calls it before a step and discards the result,
  then after it and keeps the result. On real chromium the events land before
  `navigate` returns, and the real-pair arms pass with the barrier removed. A
  fake BiDi peer that sends its fault only on receiving the barrier is the arm
  that fails without it.

**WHAT IT CANNOT SEE** is a fault the page raises after the step's closing
barrier, such as a timer or a slow fetch. That fault lands in the next step's
window. The alternative would be waiting for it, which is a fixed sleep, and
this stage has none.

**THE THIRD ARM IS A REQUEST RATE, AND IT READS RESPONSES ALONE** (fable's
ruling at coop r9/173, from the measurement at r9/172; `rate.go`). On chromium
156, two seconds after load, an honest page added about one response a second
(a 1s poll made 3 in the window). A fetch loop made 260 to 374, and a 100ms
poll made 20. The default fires at **at least 20 responses in EVERY whole
second of a 2-second window** (r9/179), and a binding may raise it. It was 10
for one gate, which put the 100ms poll exactly on the line, and the gate
measured what that costs. The poll fired 10 of 10 at load 2 and not at all at
load 12, because chromium's timer slips under load. Load deciding a verdict is
what R9 rules out, so the default moved off the poll interval. The 100ms poll
is now a real page that must not fire at any load. Every second must carry the rate on its own,
rather than the window carrying the total. A page that loads thirty assets in
half a second puts thirty inside two seconds, and a total would fire on every
heavy page load. Log entries are counted beside the rate and reported in the
same sentence. They are never added to it.

The window runs on the BROWSER's clock, the timestamp each BiDi event carries,
so a slow box delays the events without changing what they say. It spans step
boundaries, because a step usually ends when its page has loaded, well under
two seconds, while a loop runs as long as the actor stays on the page. One
finding per episode; the arm re-arms once a second falls short.

**A RENDER LOOP THAT FETCHES NOTHING IS INVISIBLE HERE, BY DESIGN.** Measured:
a `requestAnimationFrame` loop made 1 response and 0 log entries, the same as
a clean page. Seeing it would take a MutationObserver installed in the page,
and the ruling forbids code in the page under test, since a finding read off
our own script is not the system's. If such a loop matters to a project, a
binding's flow can assert what the page shows. A loop on a page the actor
leaves within the window is not seen either.

## A fan-out is N processes against one world

**TWO REQUESTS IN FLIGHT IS THE ONLY WAY A CHECK-THEN-ACT DEFECT CAN BE
REACHED**, and the walk cannot do it: a `wire.Session` is not safe for
concurrent use, deliberately, because "the protocol says `where N sessions are
wanted, N processes` rather than N outstanding requests on one ... the engine
owns the timing". So the concurrency stage is a SET of sessions, each driven by
one goroutine, and no session is ever touched by two.

**AND THE N PROCESSES MUST SHARE ONE WORLD, WHICH IS SILENT WHEN WRONG.**
`cmd/toy-adapter` has two modes: it takes an address from `TOY_ADDR`, and when
that is absent it starts a toy of its own — the mode that makes `rumor
validate` answer with no engine at all. Two adapters spawned carelessly
therefore contend on NOTHING: two requests against two servers, both
succeeding, and a capacity race reported as absent by a run that never put two
callers in one world. The provider owns the process and the adapter is pointed
at it, which is what `[adapter].env` and the provider's own `Addr()` already
exist for — `Addr` being deliberately off the rollback trait, since "the five
verbs are about putting a world back, and where a system under test listens is
not one of them".

**SO THE ARM PROVES ONE WORLD BEFORE IT PROVES ANY TIMING.** The second actor
joins the event the FIRST actor created; against two toys that join is a 404,
because the second adapter's world has no such event. Measured through two
adapters on one provider-owned toy: both joins accepted, overlapping by 28.4ms
with each request in flight about 29ms.

**AND "CONCURRENT" IS A CLAIM THAT CAN BE FALSE, so every attempt carries its
span.** A fan-out whose requests happened to serialize — a slow spawn, a busy
box, a barrier released late — would exercise the sequential path and report
the race as absent, which is indistinguishable from a system that has none. The
spans are what let a run be REFUSED rather than believed, and the barrier is a
closed channel rather than a sleep so the release is one event instead of a
function of the box's load. The spans are taken client-side, around the
exchange, so two requests count as in flight together whether the system
interleaves them or serializes them under a held lock: that keeps the mechanism
arm independent of the defect it exists to reach.

**AND THE FAN-OUT NEEDS A SEEDED PLAN, OR BYTE-IDENTITY IS A FROZEN ORACLE.**
D1's "two attempts of one seed are byte-identical" is not a claim that the
system is deterministic; it is ENFORCED by keeping the hashed stream clock-free
and sorted. A concurrency stage whose only contribution to that stream were a
clock-free "did overlap" would put ONE BIT there — and one bit cannot differ
between seed N and seed N+1, so byte-identity would hold for every seed and the
determinism clause could never fail. The plan is what carries more than a bit:
which session issues each request, in issue order, drawn from the run's own PCG
generator so that one seed renders one plan and a different seed renders a
different one. Measured at width 6: forty seeds give forty distinct plans, and
all thirty-nine adjacent pairs differ.

**THE VERDICT AND ITS DURATION ARE TWO METHODS, AND THE ONE CARRYING A CLOCK
SAYS SO IN ITS NAME.** The shape is the quiesce prober's, not a new one:
`Hang()` is the clock-free half "and therefore safe where the control reads;
`HangMeasured()` is the half that must not come here", and the measured half
goes to stderr because stdout "would make D1 unreachable for every run that
ends on a timeout". A duration on the hashed stream moves a digest every run,
so the concurrency clause would then be satisfied by the very change that
breaks determinism. Returning the verdict and the figure together invited
printing them together, so `Overlapped` answers a bool and `OverlapMeasured`
renders the figure for stderr alone — and is empty when nothing overlapped, so
a caller printing it unconditionally says nothing rather than something false.

**AND THE RULE HAS A SECOND STAGE UNDER IT NOW, which is how a rule stops being
one case.** The browser stage's `--browser-check` probe starts the pair and
reports what happened, and that outcome is a reading of THE BOX: measured on one
seed run into two directories, the two differed in that line alone — `FOUND, NOT
DRIVABLE ... would not open a session` under load against `DRIVABLE` on a free
box, with every other byte including the FOUND stanza identical. So D1 held for
no `--browser-check` run on a loaded box, and the stream was the defect rather
than the arm that noticed. The verdict now goes to stderr as `HangMeasured()`
does; stdout keeps the determinable half, that the pair is COMPLETE and WAS
PROBED, so the record points at the answer instead of losing it. THE REPAIR
OFFERED FIRST WAS AN EXCLUSION in the determinism arm, which would have narrowed
the only check that reads this stream to accommodate a line this paragraph says
does not belong in it — the shape worth naming, because an exclusion is cheap,
local, and looks like diligence.

**THE PLAN IS DISPATCHED AND NOT MERELY DECLARED**, which is the same defect
R8.1 closed one level up: `[stages]` was parsed, cross-checked and never
dispatched. The fan-out issues the k-th request to the session the plan chose,
spawns in that order and records in it, so the attempts a run hashes follow the
seed rather than the goroutine scheduler. A width disagreement between the plan
and the sessions is refused rather than trusted. What the plan fixes is which
session carries what and in what order the attempt is recorded — never who wins
the race, because fixing that would remove the contention the stage exists for.

**AND THE CONTROL HOLDS TOGETHERNESS WHILE VARYING THE DEFECT, which is the
opposite axis from the one a serialized fan-out tests.** Serializing the fan and
watching the overlap clause fire proves the clause is sensitive to
togetherness — but a clause that fired on ANY overlapping fan-out would pass
that test identically. The complement is to hold togetherness constant and
change only whether the defect is present, and cmd/toy already contains both
readings of one rule: `joinEvent` checks capacity, DROPS THE LOCK, then
inserts; `acceptInvitation` checks capacity and inserts under a HELD lock. Same
oracle, same check-then-act, same structure, differing in exactly one variable.
So a fan-out on each at capacity 1 is the control, and it needs no mutation and
no test hook — which the toy's own comment forbids, since "a hook the test
flips would be a fixture override standing in for the production path, and this
project has been bitten by one". Measured: join_event accepted 2 of 2 into room
for 1 and the clause fires; accept_invitation accepted 1 of 2 and it is silent;
both fan-outs overlapped, so the decision moved and togetherness did not.

**THE ABSENCE CLAIM CARRIES ITS PRECONDITIONS, because three setups make "no
overfill" meaningless.** Room below one: every request must refuse whatever the
system does, so the setup answers rather than the system — a control that is
the case it controls for. Room at or above the width: every acceptance is
legitimate, the limit is never under pressure, and a system with no check at
all would pass. Nothing accepted: the system was never asked to choose between
them. The mechanism arm sits in the second of those deliberately, running
capacity 3 against two joins because nothing there is meant to rest on the
race, which is exactly why the oracle refuses that shape rather than reading it
as a clean run.

**AND THE VERDICT READS THE WORLD, NOT THE ANSWERS.** Counting the acceptances
a fan-out received makes the overfill clause a claim about RESPONSES: a system
that answered 200 to two requests and persisted nothing would read as having
overfilled, which is a defect about what an answer MEANS and not about the
locking of a check-then-act. The race's signature is the system's own
quantity — it HOLDS more than its limit allows — and the toy already declares
the observation that reads it, `store_event_count`, whose own comment says why
it is the right one: "computed from the attendee list ... an oracle that read
the same field could not disagree with it". So the verdict rests on the
read-back, the acceptances are kept only so the two can be compared, and a
disagreement between them is its own finding rather than an overfill. Measured
on the shipped toy: after the race at capacity 1 the declared observation reads
2 against two acceptances, so the pair agrees and that clause is a guard rather
than a live finding.

**THE ROOM IS DERIVED FROM THE LIMIT AND WHAT WAS ALREADY TAKEN**, never passed
in: those two are facts the setup holds, the room is arithmetic over them, and a
caller computing it itself could get it wrong in the direction that hides a
finding.

**A STAGE'S SETUP IS DERIVED FROM THE DECLARATIONS, AND TWO STAGES NOW DERIVE
IT.** A `handle` argument's honest value is an id the system minted, so neither
the hostile corpus nor a fan-out can send a request until something has created
the thing it names; `creates` and an argument's `of` are already the
declaration for that, so the order is a reachability question over two fields
the project writes for other reasons. R8.1 derived it for the corpus. R8.2 asks
the same question for its own reason — a fan-out cannot contend an event
nothing created — so the resolver moved out of the fuzz stage rather than being
copied into the concurrency one, and what stayed behind is the part that is not
shared: which kinds a stage wants ids for at all is the stage's own question.

**AND THE LIMIT MUST BE READ FROM A REQUEST THE SETUP SENT.**
`contended.limit` names an argument of the action that created the contended
thing, so a chain that never ran that action would leave the stage fanning out
against a number it chose against a world somebody else built. The chain is
derived and the limit is declared, so the two can disagree — a project naming
its limit on a second creator of the same kind resolves to the first — and that
is a refusal naming both rather than a contention at the wrong capacity.

**THE BOOTSTRAP IS THE HALF THE HANDLE GRAPH CANNOT SEE.** R8.1 measured it:
the toy's `create_event` failed until `create_user` ran, because a session
belongs to a user that exists, and that dependency is declared in `requires` as
a STATUTE RULE which no amount of reading `creates` can resolve. The fuzz stage
got it for free — its corpus needs a `user` handle, so the bootstrap was
already in the chain and only had to be hoisted first. A fan-out does not:
`join_event` takes an `event` and nothing else. So the derivable condition is
named rather than inferred from one project — context `none`, no handle
argument, and an `actor`-sourced argument — and an action meeting it is run
once FOR EVERY ACTOR that will act, because the actor argument is what makes
its request say something different each time. Without the actor argument it
would be N identical requests, and a creating action answers the second of
those with a refusal, which would manufacture a setup failure instead of a
world.

**THE STAGE'S WORLDS ARE ENUMERATED AND NOT DRAWN.** A seed chooses the plan,
and nothing else about a contention. Drawing the setup would make the stage's
own reachability a lottery, and the shipped toy measures how bad a one: two of
the three `visible_to` subsets `create_event` draws from cannot be contended at
all, because a would-be joiner's role is relative to the event — anyone who is
neither its admin nor already an attendee is a `guest` — so the subsets that
omit `guest` answer every join 404 before the capacity check is reached. A
stage that drew once would report "the system was never asked to choose between
them" two times in three over a system whose race is right there. So the sweep
is the first declared value of every drawn argument, then ONE COORDINATE MOVED
AT A TIME, stopping at the first world the clause can be answered in. A product
over the declared lists would be exponential and every point of it costs a
creation and a fan-out; a coordinate sweep is linear and reaches every value a
project declared. A contention needing two choices moved together is a world
this sweep cannot build, and it says so by reporting every try's vacuity rather
than by passing quietly. The limit is not swept: the width is derived from it,
and the sessions are opened before the sweep begins.

**EVERY HANDLE THE SUBJECT TAKES MUST BE THE CONTENDED THING, and where it is
not the stage refuses rather than answers.** A fan-out sends ONE set of
arguments to every contender, which is right for the thing being contended —
the point is that they all name the same event — and wrong for anything
belonging to one caller. `accept_invitation` is the shape: its handle is an
`invitation`, its limit would be the event's capacity, and one invitation sent
to two contenders has the second answered "already accepted". Accepted would be
1 of 2 into room for 1, which is neither a finding nor VACUOUS by any of the
four conditions — so the stage would report no overfill over a world in which
only one request could ever have succeeded. A control that is the case it
controls for, arriving through a door the vacuity clause does not watch. The
condition is derivable, which is why it is a refusal and not a comment: the
contended thing is what the limit's action CREATES, so a handle of any other
kind is per-caller by construction. Sending per-contender arguments is not built
instead, because nothing in the tree declares such a subject and a mechanism
with no project behind it is the void this phase exists to close.

**A FINDING STOPS THE SWEEP AND AN ABSENCE DOES NOT.** The two claims are not
symmetric. A found overfill needs ONE witness — the system held more than its
own limit allows, once, and no further sampling makes that less true — so the
sweep ends there. "No overfill" is an absence, and it was being reported off the
single fan-out that happened to be answerable. rook drove what that costs: with
the toy's capacity re-check restored four times in five, an INTERMITTENT race,
the stage answered `no overfill` at exit 0 with the fan-out overlapping by
28.8ms, so togetherness held and the race simply missed its window. It read
exactly like a system that has none. The k-of-n threshold does not help here: it
guards the SHRINK of a race already found, not the finding of one.

**SO AN ABSENCE CARRIES THE NUMBER OF FAN-OUTS IT RESTS ON**, and the stage
contends every world the project declares before reporting one. What this does
NOT do is choose a number: how many fan-outs an absence needs is not derivable
from anything a project declares, and inventing one would be a constant nobody
measured. The stage samples what the declarations give it and SAYS how many that
was — `no overfill in 1 answerable fan-out(s)` is a weaker sentence than `in 4`,
and a reader can now tell them apart. Separating "this system has no race" from
"its race did not fire this time" needs more than the declarations carry, and
that remains open.

**WHAT WAS ALREADY TAKEN IS READ BACK, NOT ASSUMED.** The stage builds each
try's world itself, so it could have passed zero; instead it reads the held
quantity through the declared query BEFORE releasing the barrier and again
after. A setup that left something behind — a world not as clean as the stage
believed — then shows up as a room the verdict is computed from rather than as
an overfill nobody can explain.

**AND ACCEPTANCE HAS ONE READING IN THIS PROJECT.** `[[action]].response` is
the project's equality contract for what the system taking an action looks
like, and the walk has read it since R8.1. A fan-out counting 2xx answers would
be a second answer, about one transport, to a question the bindings already
answer — and the two could then disagree about the same response. So the stage
calls the walk's reading rather than carrying its own.

## A shrink's subject is the stage's own verdict

**EVERY REDUCER TOOK `want engine.Finding`** — `Once`, `Bisect` and
`RemoveKinds` alike — so the only verdict a shrink could keep was one the
statute produced by evaluating rules against the ledger. A concurrency overfill
is not such a verdict and cannot become one, and the toy's statute says so of
ITSELF for a data-model reason rather than a wording one: "THERE IS NO CAPACITY
FIELD ... an event's capacity is not a ledger fact, a capacity OVERRUN is not
nameable from a ledger case, and this statute does not pretend otherwise." So a
stage finding had no `want` to hand the shrinker at all, whatever the gate did,
and the clean-world check over it was UNREACHABLE rather than merely unreached.

**A VERDICT IS NOT ONLY A STATUTE's.** The type is widened to an interface: a
want answers whether a candidate still gives it, and writes its own reason when
it does not. The reason belongs to the want because only the author of a
verdict knows what separated it from what came back — the statute's reading
names values, kinds, rules and attribution, a stage's names its own quantities,
and a shrinker that composed the sentence itself would have to know every
verdict there will ever be. The statute's own disclaimer stays true, and that
is exactly why the finding is the stage's to name rather than the statute's to
be taught.

**THE STATUTE's READING MOVED AND WAS NOT REWRITTEN.** The walk is the only
caller today, so a widening that altered what a statute verdict means would be
a change to the walk disguised as a refactor: the clause order, the wording of
every reason and the attribution handling are the ones that were in `Once`, and
the package's existing arms passing unchanged is what says so. The attribution
field stays the question of the want that HAS one — only a statute verdict
carries a `rests_on_planted` rule, and asking every want would make the field
read "false" where it means "not applicable".

**WHAT SHRINKS IS THE SETUP.** The step prefix and the action kinds in it, as
the two reducers already do; a fan-out is one step of width N, N belongs to the
stage and is not reduced, and the refusal of width 1 stands because one request
in flight is the sequential path. A candidate at which the verdict does not
recur is refused and the finding marked unstable and not shrunk — never read as
clean.

**AND A VERDICT THAT RECURS ONLY SOMETIMES IS PROBED k OF n.** One probe at the
gate collapses a race into `Reproduced=false`, so the finding is marked unstable
and never shrunk — and the clean-world check over it is then satisfied
VACUOUSLY, because the shrink never ran. A candidate at which the verdict does
not recur k-of-n is refused and the finding marked unstable, which is design's
own rule, and never read as clean.

**ONE OF ONE UNLESS THE WANT ASKS OTHERWISE.** Whether a verdict is
deterministic is a fact about what produced it — a statute evaluating rules
against a sealed ledger gives the same answer every time, a race does not — so
the author declares it and a want that says nothing is probed once, exactly as
before. Probing a verdict that cannot differ would double every replay in a
shrink for nothing.

**THE WORLD IS RESTORED BEFORE EVERY PROBE**, not once before the first: a probe
run against the world the previous one left would attribute its finding to both,
which is the whole of what the clean-world check exists to prevent. And k of n
is a THRESHOLD and not a quota — the probes stop at the kth recurrence, because
running the rest would spend replays to learn nothing the verdict turns on.

**THE REASON READS "RECURRED k OF n" AND NOT "unstable".** A reader told only
that a seed is unstable cannot tell a trace that never causes the finding from
one that causes it two times in five, and those are acted on differently; the
verdict carries both counts so the distinction survives into the report. A
threshold that is not one — k below one, or n below k — is refused by name
rather than read as 1-of-1 or as never.

**AND THE CANDIDATE CARRIES THE VERDICT TOO, which is the other half of the
same widening.** Making `Want` an interface let a stage AUTHOR a verdict and
left the candidate's verdict typed as the statute's `Finding` — so a stage want
had to answer "does this candidate still give that verdict" out of a field its
own oracle never wrote. What that costs is exact: two oracles both finding "an
overfill" compare equal, because there is no field in which they differ. A
stage verdict is therefore its own small type on the candidate, carrying the
ORACLE'S IDENTITY, its SUBJECT, whether it found anything, and its own
sentence; a want compares the first three and a report prints the last.

**IT IS NOT A FIFTH FINDING AND NOT A NEW KIND, and `docs/vocabulary.md` is
why.** `bug` is defined there as an OBSERVATION disagreeing with what its
observer knows, with no account of untold claims and the claims honest; a
contention is none of that — nothing observed, no observer, no ledger. That
document also says the slot beside the four findings is SPENT, so a sixth needs
it amended in its own Revisions row with a reason the shape needs one. A stage
that wrote a new word into a finding's value would be amending the vocabulary by
assignment. The four findings keep their meanings and a stage's verdict stands
beside them.

**AND THE CONCURRENCY STAGE IS THE FIRST AUTHOR OF ONE.** Its oracle is named
`rumor.concurrency.overfill` — not a rego path, because no rego produced it, so
it cannot collide with a statute's `data.rumor.…` — and its verdict's subject is
the contended action. What a candidate must still give is the DEFECT and not the
DEGREE: the same oracle finding an overfill of the same action. The quantities a
race produces move with it, two of two requests landing this time and two of
three the next, and both are the same defect — that the check and the insert are
not one step — so a comparison on the counts would read a recurrence as a
different finding and mark a real race unstable.

**THE STAGE'S TRACE IS THE SETUP PLUS THE FAN-OUT AS ONE STEP**, which is what
the reductions are given: the bootstrap once per actor, the world the limit is
set in, and the contention itself. On the shipped toy that is five steps —
three `create_user`, the `create_event`, the fan-out — and the fan-out's actor
field names every contender joined, because the field is one actor wide and a
fan-out is N. Each setup step carries its own drawn values, so a replay composes
every request from the step that describes it rather than re-deriving it: the
request sent is the one the record says was sent.

**THE WORLD IS CHECKPOINTED BEFORE THE STAGE CONTENDS IT**, and that is the one
ordering difference from the shrink of a walk's finding. The walk drives its own
adapter's system, so the provider's world is still clean when the shrink asks
for a checkpoint afterwards; the concurrency stage contends IN the provider's
world, so a checkpoint taken after it would checkpoint the overfill — and every
candidate restored to that would start with the race already run.

**A STAGE AND A REPLAY TAKE ONE PATH THROUGH THE STAGE.** The replay runs
whatever step list it is handed against a freshly opened set of sessions; what
differs between the original run and a candidate is the LIST, which is data
rather than a branch. So a setup step the system declines means two things to
the two callers and the code says both: to the original run it is a
could-not-run, because the stage was building its own world and the system said
no, and to a replay it is the candidate not reproducing, because a reduction
that removed a step the setup needed is supposed to fail that way. Returning an
error for both aborted the whole reduction on its first kind-removal probe; the
refusal travels separately now and the reason is kept either way.

**AND THE THRESHOLD IS A MAJORITY OF MORE THAN ONE PROBE, chosen from the two
ways the gate can be wrong rather than from a measured rate.** A reproduction
rate measured at one load is not a rate: the same fan-out's overlap read
30.448ms and then 212.909ms on one machine against a window the toy holds for
25ms. What does not depend on the rate is the cost of each error. Too low a
threshold accepts a candidate that overfilled once by luck, and a reduction kept
on it reports — and in another caller promotes — a trace whose own replay would
usually find nothing. Too high refuses a real race that recurs sometimes, so the
finding is marked unstable and not shrunk while the stage still reports the
overfill: the product loses a minimal trace and keeps the finding. The second is
the recoverable direction, so the threshold is a majority, and the smallest n
that admits a majority while letting one probe fail is three. Two of three.

**SO THREE READERS STOP SPEAKING THE STATUTE'S WORDS OVER IT.** A trace prints
the oracle's sentence and names the oracle, instead of "no finding" and "fired
(none recorded)" — which is what a trace that reproduced a real race read
before. The partial-reproduction line, which reports that `rests_on_planted`
was not compared, is the statute's question and is absent for a verdict that has
no such rule. And a promoted seed's comment is the oracle's sentence, because
the lock is the one artifact of a shrink that gets checked in and a row saying
nothing was found is a row nobody can act on.
