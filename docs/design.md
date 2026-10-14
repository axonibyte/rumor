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

- **The licence is BSD-2-Clause**, and `cargo deny check licenses` gates every
  pull request against the allowlist in `deny.toml` (R0.2).

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
statute — but it remains available to diagnostics.

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

OPA v1.21.2 declares `go 1.26.0`. This module declares a lower version
deliberately (see `go.mod`), under a comment permitting a raise once the
runner's version is known and something here needs the newer language. Both
conditions now hold: the CI jail was measured at go1.26.8, and the engine is the
thing that needs it. The raise lands with the run that first takes OPA as a real
dependency, not with this inventory.

One trap, because it defeats a version assertion rather than failing it: with
the default `GOTOOLCHAIN=auto`, invoking an older toolchain against a module
whose directive requires a newer one makes Go **download and hand off to the
newer toolchain silently**. Measured: `go125` ran a suite under go1.26.8 and
reported `ok`, while the same command under `GOTOOLCHAIN=local` refused with
`running go 1.25.14; GOTOOLCHAIN=local`. Naming a versioned binary therefore
does not pin the toolchain that builds the code; the `go` directive does.

## Why Rust and one binary

Not yet written.

## The engine owns the seed

Not yet written.

## The ledger is append-only

Not yet written.
