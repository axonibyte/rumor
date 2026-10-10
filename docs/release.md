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

Not yet written.
