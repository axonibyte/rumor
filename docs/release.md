# Release

> Stub from R0.1. The headings below are the ones this document owes; the
> content arrives with the phase that owns the subject. `docs/coverage.toml`
> names this file, and `rumor docs --check` fails if a source path it should
> describe is not mapped.


## The sequence

Not yet written.

## Versioning

Semantic versioning, and before 1.0 the two parts that move are the minor and
the patch. A **minor** bump is for a change to the CLI surface — a new or
changed command, flag, or output contract. Everything else is a **patch**.
`v0.1.0` is a minor because it introduced the CLI surface; the children that
followed it are patches.

`main` is **fast-forward only**, and every bump commit sets the workspace
version in `Cargo.toml`. Those two together mean the version on `main` can never
go backwards, which is the property the release gate relies on rather than
checking separately.

**A version number is not a reservation.** The next tag is the next patch number
*at the time its bump lands*, in landing order — not a number assigned to a
child of work in advance. Children that run in parallel therefore do not own
numbers: whichever bump lands first takes the next patch, and the others take
the ones after it, in the order they land. Planning documents may name an
expected number; the authority is the bump that landed.

> This rule replaced one that did the opposite. The R0 breakout assigned a fixed
> version to each of its four children while they ran in parallel, and that
> produced a false constraint — that a child holding a higher number must wait
> for the lower ones to be tagged, or the gate would refuse them afterwards.
> Under landing order there is nothing to wait for. The constraint was recorded
> in a changelog section and in two handover documents before it was retired,
> which is why it is written down here as a rule rather than left as a
> convention: the reserved-number reading is the one a reader arrives with.

**A changelog section is immutable once its tag exists.** Until then it is the
bump branch's draft and may be edited there like any other file — including
renumbering its heading, if the landing order turns out differently from what
was expected. Once the tag exists the section is fixed, and the tag is what
makes it so.

One tag has one section, written in the bump commit for that tag. The bump
commit is the last commit of the branch, so the tagged commit is by definition
the one carrying the final section and nobody reading a tag gets a draft.

## Documentation set

Not yet written.

## Doc coverage map

Not yet written.

## Release gate

Not yet written.

## Tag pipeline

Not yet written.

## CI

One workflow, `.forgejo/workflows/ci.yml`, on the FreeBSD runner. Linux waits on
a host and is R0.2b; `runs-on` binds to the runner's **label**,
`freebsd-15-jail`, not to its name, which has already been renamed once.

### Every step proves it ran

A queued job is not red. A job whose label no runner answers sits pending
forever, and pending reads as "no failure" rather than "never ran" — so a green
overall run is not evidence that any step executed. A zero-elapsed success is
also what a *fast* run looks like, so elapsed time cannot settle it either
direction.

So each step asserts **a value only execution could have produced**: a version
string, a count the step computed, a name resolved out of `Cargo.lock`. Never a
duration, and never the absence of red.

### The deny step, and its two exit codes

`cargo deny check licenses` runs with **no `-D` flag**; use `-W` to surface
warnings. This matters more than it looks:

| exit | meaning | fix |
|------|---------|-----|
| 0 | the graph's licences are all on the allowlist | — |
| 4 | a crate's licence is outside the allowlist, named in the output | decide: allow it, or drop the dependency |
| 1 | **the config failed to deserialize and the gate never ran** | fix `deny.toml` |

Exit 1 and exit 4 are opposite problems and the step's message must distinguish
them: 4 is the gate working, 1 is the gate absent. Under `-D` the distinction
collapses further — an unused allowlist entry and a real licence violation both
return 4, so a cosmetic warning becomes indistinguishable from a violation by
exit status alone.

An allowlist entry that matches nothing warns on **every** run. That warning is
the cost of having decided the list in advance, and the fix is never to trim the
Decided list to quiet it.

The tool is pinned: `cargo install cargo-deny --locked --version 0.20.2`. There
is no FreeBSD binary release, so the job builds it (~5 min). An unpinned gate
tool means the gate's verdict can change with no commit.

### Licence id spellings, and why they are not the Decided words

- `Unicode-3.0`, where the Decided line says "Unicode", which is not an SPDX id.
- `Apache-2.0 WITH LLVM-exception` is **absent** on purpose: the crates needing
  it declare a disjunction, so plain `Apache-2.0` satisfies them. Proved with a
  control rather than assumed — allowing only `BSD-2-Clause` plus the exception
  returns 4, so the passing case means something.
- `Unicode-DFS-2016` is absent: nothing in the dependency set carries it.
