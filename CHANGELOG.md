# Changelog

Every tag has one section. A section is written in the bump commit for its
tag and is never edited once that tag exists.

## 0.1.3 — 2026-10-12

A release gate that refuses, a tag pipeline that publishes nothing when it
refuses, and twelve fixture branches that have each been seen to trip one arm.

- `tools/release-gate.py`: the epic's nine Release gate bullets as twelve arms
  with stable ids, because bullet 1 is a disjunction and bullet 4 has three
  failure modes. It does **not** short-circuit: issue 4's test is that a fixture
  shows "the refusal reason it names, and no other", and a gate that exits at
  its first finding cannot answer the second half. Three exit statuses, and the
  third is not a verdict — 0 clean, 1 findings, 2 could not run, which the
  workflow treats as a failed run.
- `--self-test` drives every arm and asserts each fires **alone**, plus a clean
  run that must pass and a nine-shape battery over the `Docs-Unchanged` reader.
  It found five arm/fixture defects before a branch existed, including two pairs
  of arms that could not be told apart.
- Driving the gate against the accepted `6c5583d..5e8a639` found two more that
  no fixture could reach, because a fixture repo has no tags: `git rev-parse` on
  an annotated tag returns the tag object, and the tag for the version being
  gated counts as "the latest".
- `.forgejo/workflows/release-gate.yml` measures `python3` and fails loudly when
  it is absent rather than skipping, which is the standing argument against
  python in CI answered rather than ignored. It runs the gate's own self-test
  before the gate judges anything, and asserts the finding **set** from a file
  the gate writes, not a grep of its prose.
- `.forgejo/workflows/tag-pipeline.yml`: FreeBSD amd64 only, since an artifact
  nobody ran is not a release artifact. Every check sits before the build, so a
  refusal publishes nothing. A missing trust store is reported separately from a
  failed signature — announcing the first as the second would accuse a correct
  tag of forgery — and with no maintainer key on the runner the signature is a
  named wait, not a silent skip.
- Arm `docs-stale` reads the working tree, so it now verifies that the checkout
  **is** the run being gated and reports UNMEASURED when it is not. It had taken
  a ref and ignored it, which made it answer about whatever branch happened to be
  out; the fixture verification could not see that, because it checked each
  branch out first and so agreed by accident.
- The `Docs-Unchanged` trailer folds like the git trailer it is. A reason wrapped
  to column 0 does not exempt: git reads only the first line there, and
  truncating it published a reversed sentence into the PR summary.

## 0.1.2 — 2026-10-12

CI exists on the FreeBSD runner, and the licence gate has been seen to refuse.

- `.forgejo/workflows/ci.yml`: eight steps on the `freebsd-15-jail` runner, each
  asserting a value only execution could have produced rather than printing a
  status. Every external call carries a `timeout` and a hung call is a named
  finding against the runner, not a red build.
- The licence gate pins `cargo-deny 0.20.2` and asserts the pin rather than
  printing it: the literal is duplicated from the install line on purpose, so
  moving one without the other fails the step.
- **Test 3, the gate's negative control.** `tests/fixtures/deny-gpl/` is a
  separate workspace — excluded by the root manifest — holding one crate
  licensed `GPL-3.0-only` under a `BSD-2-Clause` root, so the only violation in
  that graph is the GPL crate. The step drives it with the real `deny.toml`
  rather than a copy, and asserts the exact status 4 plus the log naming both
  `GPL-3.0-only` and `gpl-fixture`. Before this, the gate had passed from the
  first run having examined no dependency licence at all; a gate never seen to
  fail is indistinguishable from one that is not wired up.
- **A timeout is a bound, not a diagnosis.** Four `rc 124` messages asserted
  causes the bound cannot support — a clone finishing at 181 s, a test taking
  901 s, and two that named the registry. Each now states what the status
  proves and says which log decides the rest. The cargo-deny install gains a
  60-second index touch ahead of its 900-second wrapper, so no-route fails in
  its own words and the wrapper can only mean slow.
- `tools/check-workflows.py` reads every workflow and refuses an unbounded job
  or invocation.
- `docs/release.md` gains **Test 3: the licence gate's negative control**, and
  `docs/coverage.toml` maps the fixture to it.

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
