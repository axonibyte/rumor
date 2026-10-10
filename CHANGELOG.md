# Changelog

Every tag has one section. A section is written in the bump commit for its
tag and is never edited once that tag exists.

## 0.1.1 — 2026-10-10

The naming sweep is recorded and the crate name is settled.

- `docs/prior-art.md`: the registry, GitHub and cross-language sweep behind
  the name. `rumor` is held on crates.io — with `rumor-client` and
  `rumor-server`, by one owner — as a reservation published 2023-11-20 and
  never updated. `rumor-cli` was free and is the epic's decided fallback, so
  it ships rather than waiting on a transfer.
- `docs/design.md` records the names and the licence in its decided list,
  with the reason and a pointer to the sweep.
- The transfer ask was drafted, put to the maintainer, and **deferred**: the
  collision costs nothing until the project publishes to crates.io, which is
  not near, so no ask has been sent to the name's holder. `docs/prior-art.md`
  records the decision and its date. If the ask is made later and granted,
  publishing under `rumor` is an improvement landed by a patch run.
- `docs/release.md` gains its **Versioning** section: semantic versioning with
  the pre-1.0 reading of minor and patch, `main` fast-forward only so the
  version cannot go backwards, and the rule that a version number is not a
  reservation — the next tag is the next patch at the time its bump lands, in
  landing order. The remaining six headings are still owed by later phases.

Two findings from the sweep belong to later phases and are recorded, not
acted on: in the Nostr protocol *rumor* is a defined term for an unsigned
event, which `docs/vocabulary.md` must not silently collide with in R1; and
across the Romance languages the word's first reading is *noise*, which the
README now says.

No code changed, so no test was added: this run is documents. `rumor docs
--check` covers the new file through the existing `docs/*.md` entry and passes
at 24 paths.

VERSION NUMBER, because this section was drafted as 0.1.3 and ships as 0.1.1:
the breakout assigned a number to each child while they ran in parallel, and
that was retired — a number is not a reservation, and the next tag is the next
patch at the time its bump lands, in landing order. This child landed first, so
it takes 0.1.1. `docs/release.md` carries the rule.

## 0.1.0 — 2026-10-10

First tag. The workspace exists and nothing runs yet.

- Cargo workspace with the crate `rumor-cli` and the binary `rumor`.
- BSD-2-Clause `LICENSE`.
- `README.md` with the scope statement and the working arrangement.
- Every document of the documentation set, with its headings.
- `docs/coverage.toml`, mapping every path in the workspace to the docs that
  describe it.
- `rumor docs --check`: exits 0 when every path matches a coverage entry and
  every entry matches a path; non-zero naming the first path that no entry
  covers, or the first entry that covers nothing. The second half is not
  symmetry for its own sake: without it a mistyped glob is a dead entry the
  check cannot see, masked today only because no two entries overlap.

CLI surface created: `rumor docs --check`. Under pre-1.0 versioning a change to
the CLI surface takes a minor bump, which is why the first tag is 0.1.0 rather
than 0.0.1. No contract or vocabulary version constant exists yet, so no
`Revisions` entry is owed in `docs/contract.md` or `docs/vocabulary.md`; the
first of those lands with R1's vocabulary and R4's contract.
