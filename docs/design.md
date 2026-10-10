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

- **The crate is `rumor-cli`. The binary is `rumor`.**

  The bare `rumor` crate name on crates.io is not available. It is held, with
  `rumor-client` and `rumor-server`, by one owner as a reservation published
  2023-11-20 and never updated — its own description reads `reserved; library
  for rumor file synchronization protocol`. `rumor-cli` was the epic's decided
  fallback and was free, so it ships from v0.1.0 rather than waiting on an ask
  that may never be answered.

  A transfer has been asked for and the ask's status is recorded in
  `docs/prior-art.md`. If it is ever granted, publishing under `rumor` is an
  improvement landed by a later patch run; nothing in the project depends on
  it. The binary is `rumor` either way, which is the name users type.

- **The licence is BSD-2-Clause**, and `cargo deny check licenses` gates every
  pull request against the allowlist in `deny.toml` (R0.2).

See `docs/prior-art.md` for the registry, GitHub and cross-language sweep
behind the first entry, including the one collision that matters inside Rust:
in the Nostr protocol *rumor* is a defined term for an unsigned event.

## Why Rust and one binary

Not yet written.

## The engine owns the seed

Not yet written.

## The ledger is append-only

Not yet written.
