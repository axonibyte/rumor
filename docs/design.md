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
  intention applies and Go's module graph plus `go.sum` is what carries it.

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

Rules are Rego, evaluated by **OPA embedded as a library**, pinned at
`github.com/open-policy-agent/opa v1.21.2`. The pin is exact so that a patch
release cannot change a statute's verdict without a commit saying so; the same
reasoning held for the engine this replaces.

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

## The ledger is append-only

Not yet written.
