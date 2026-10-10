# rumor

`rumor` drives a system under test as a set of simulated users, keeps an
append-only ledger of what each of them should believe, and reports where the
system and the ledger disagree.

## Scope

**In:** request/response systems with principals, persisted state,
asynchronous work, and optionally a browser UI.

**Out:** component conformance, browser-vs-fake comparison, human evidence,
compilers, embedded targets, and batch pipelines.

## Install

Not yet. The crate is `rumor-cli` and the binary is `rumor`; the bare `rumor`
crate name on crates.io belongs to someone else and arrives by transfer or not
at all. See `docs/prior-art.md` once R0.4 lands.

## Quickstart

Not yet: the toy system under test arrives in R5. Until then:

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

## On the name

In the Romance languages the word's first reading is not *hearsay* but
**noise** — Italian `rumore` is noise outright, and Portuguese carries
"continuous noise" as a listed sense. A tool whose whole job is separating a
real finding from noise meets that reading first, so the tagline always says
what the name means here: a simulated user's belief, held and checked.
