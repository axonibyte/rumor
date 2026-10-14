# Architecture decision records

One record per decided Open item from the epic, named `NNNN-<slug>.md` and
written when its `adr-<slug>` thread closes. A record states the question, the
options, the decision, and what it rules out.

**A record is dated and frozen at acceptance**, like a `CHANGELOG.md` section:
its body describes the world on the day it was decided and is never edited to
track what happened afterwards. A later change is a new record, or a one-line
"Superseded by NNNN" added at the top of the old one, never a rewrite of its
body. So 0001's Consequences describing the Rust tree as still present is
correct as written; v0.1.4 removed it two commits later.


> Stub from R0.1. The headings below are the ones this document owes; the
> content arrives with the phase that owns the subject. `docs/coverage.toml`
> names this file, and `rumor docs --check` fails if a source path it should
> describe is not mapped.

## Records

- [0001 — The engine: OPA embedded as a library, and Go](0001-engine.md).
  Accepted by cal 2026-10-14. Replaces the epic's original Rust-with-Regorus
  line; the inventory behind it is `docs/design.md`, "The engine".
