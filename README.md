# rumor

`rumor` drives a system under test as a set of simulated users, keeps an
append-only ledger of what each of them should believe, and reports where the
system and the ledger disagree.

## Scope

**In:** request/response systems with principals, persisted state,
asynchronous work, and optionally a browser UI.

**Out:** component conformance, browser-vs-fake comparison, human evidence,
compilers, embedded targets, and batch pipelines.

## Language and engine

Go, one static binary. Rules are Rego, evaluated by OPA embedded as a library.
Decided 2026-10-14, replacing the epic's original line (Rust with embedded
Regorus): the product's value is the explanation behind a finding, and the
reference engine carries evaluation traces, coverage and a test runner that the
Rust reimplementation does not. The Rust tree in this repository is the one
being replaced; it stays until the Go port lands so that nothing on `main` is
ever half of each.

## Install

Not yet. The module is `forge.axonibyte.com/axonibyte/rumor` and the binary is
`rumor`. See `docs/prior-art.md` for the name.

## Quickstart

Not yet: the toy system under test arrives in R5, and the Go port is in
progress. Until the port lands the commands below are the Rust tree's:

    cargo build
    cargo run -- docs --check

## Working on rumor

- **Every commit is authored `Caleb L. Power <cpower@axonibyte.com>`**, set
  with `git config --local` in your own clone and never `--global`: a global
  setting would silently re-author commits in unrelated repositories.
- **`commit.gpgsign` is `false`** for commits. Tags are annotated and signed,
  and signing them is the maintainer's.
- **No generated trailers of any kind.** Not `Co-Authored-By`, not a tool
  banner, not a session identifier. A commit message says what changed and
  why, and nothing about what produced it.
- `main` is fast-forward only. A run ends in a bump commit whose subject is
  `release: vX.Y.Z`, with a dated `CHANGELOG.md` section for that version.
- **Stale remote branches are deleted, by whoever notices.** A branch is stale
  once its work has landed on `main`, or has been superseded on the record (a
  re-cut, a retired gate's fixtures, a pivot). What a branch proved lives in
  the issue, the thread and the workflow run that cite it, never in the branch.
  Standing rule from cal, 2026-10-14.

## On the name

In the Romance languages the word's first reading is not *hearsay* but
**noise** — Italian `rumore` is noise outright, and Portuguese carries
"continuous noise" as a listed sense. A tool whose whole job is separating a
real finding from noise meets that reading first, so the tagline always says
what the name means here: a simulated user's belief, held and checked.
