# Changelog

Every tag has one section. A section is written in the bump commit for its
tag and is never edited once that tag exists.

## 0.1.5 — 2026-10-14

The tag pipeline verifies a tag instead of guessing about it, and this tag is
the first the pipeline runs green end to end.

- **`gpg.ssh.allowedSignersFile` is configured before `git verify-tag`.**
  Without it git does not check an ssh signature at all — it errors with "needs
  to be configured and exist" — and v0.1.4's run reported that as a failed
  verification, accusing a correctly signed tag. The path is the file
  provisioned in the ci jail (issue 25); the step asserts git reads the value
  back, and an unreadable file is a provisioning finding rather than a verdict.
- **The verdict logic moved out of the YAML** into
  `tools/verify-tag-verdict.sh`, because a workflow step is only exercised by
  pushing a tag and two defects had been sitting in four lines nothing could
  drive. It answers verified, refused or provisioning, and names the clause
  that answered; `--self-test` drives eight real strings and asserts the clause
  rather than the exit status, with three mutants each killing a case.
- **A good signature by an untrusted signer is now a refusal.** With the store
  configured, git prints a `Good ... signature` line *and* `No principal
  matched.`; the old test matched "no principal" and called it provisioning,
  granting what it should refuse. The finding is quill's (rumor/448), and it
  was unreachable until the first defect was fixed — which is why both land in
  one run.
- The v0.1.4 artefact was never published, since its pipeline refused before
  the build. This tag's run is the first with a `freebsd-amd64` artefact
  attached.

## 0.1.4 — 2026-10-14

The pivot to Go with embedded OPA, landed whole: the module, the CLI, the
engine decision, the statute suite, the licence surface, and the removal of the
Rust tree. `main` goes from Rust to Go in one fast-forward, because a release
that carried half of each is the thing the README's own line forbids.

- `go.mod`, `cmd/rumor` and `internal/coverage`: the 412-line coverage module
  carried over behaviour for behaviour, including every message a test asserts
  on, and all seven of its arms with it. The three exit codes are unchanged, 0
  clean, 1 failed check, 2 usage, because the workflows and `tools/` read them.
  `cmd/rumor` gained its first tests: four, including one that drives a writer
  which refuses every write and asserts the exit code is **not** 0 — a check
  whose result never arrived must not report success.
- **The `go` directive is 1.26.0, and it did not start there.** The port chose
  1.25 deliberately, since a newer toolchain builds an older module and not the
  reverse, so the lower bound is the one that cannot strand a runner. OPA
  declares 1.26.0, and a module cannot require less than what it depends on, so
  the statute suite raised it. The runner measures at go1.26.8, which is what
  makes the raise safe rather than merely necessary.
- `GOTOOLCHAIN=local` in every step that resolves a Go binary. Under the default
  `auto`, a `go` directive above the installed toolchain is satisfied by
  silently downloading another, so naming a versioned binary never pinned what
  compiles the code; the build's proof now reads the toolchain recorded
  **inside** the binary.
- A root `VERSION` file is the committed version source. `go.mod` has no
  version field, so the manifest cannot be the second of the three sources the
  gate makes agree, and dropping to two is weaker than three. Both readers are
  strict, and their strictness was driven against each other over 17 inputs
  after they disagreed about a trailing space and a CRLF.
- **The engine is OPA, pinned at `v1.21.2`, with the decision recorded in
  `docs/adr/0001-engine.md`** and the inventory behind it in `docs/design.md`:
  six capabilities measured in both directions, four of them reproducing ibis's
  Regorus figures exactly, and two — a test runner and retrievable evaluation
  traces — where the reference engine answers and the reimplementation does not.
  The pin is exact so a patch release cannot change a statute's verdict without
  a commit saying so.
- **The three R1.2 example packages carry over byte for byte**, proven by tree
  object and by each file's blob id pinned in a test, with a Go harness over
  them: 3 packages, 17 cases, 19 mutants, every case green and every mutant red
  with its killing case named, coverage and a trace asserted per case. A mutant
  that cannot be killed is removed with the gap written down rather than left to
  survive. ADR 0001's prohibition on `print()` inside a statute is enforced by
  two oracles — a static scan for calls no case reaches, and a hook on the real
  evaluator — because OPA **erases** print calls unless they are enabled, so a
  hook alone reports zero for a statute full of them. That flag's necessity is
  measured with it switched off, not asserted.
- **The licence gate is `tests/licences`**, over the surface `go list -deps
  -test` reports rather than the module graph, which over-counts: 24 built
  modules, all inside R0.2's allowlist. Multiplicity is refused rather than
  resolved, since a first-match classifier admits a module under "MIT OR
  GPL-3.0" on whichever marker it checked first; BSD-3-Clause's text contains
  BSD-2-Clause's, so the containment is encoded instead of being right by
  ordering. It has a negative control, which is the one thing the Rust gate
  taught at cost: seven synthetic modules through the same verdict function,
  five refused and two passed, each asserting the clause that answered. Adding
  it exposed that "outside the allowlist" had been **unreachable** — every id
  the classifier could produce was allowed, so a GPL dependency would have
  refused as "unclassified".
- `ci.yml` is Go: build, test, gofmt, vet, `golangci-lint` 2.14.0 and
  `govulncheck` v1.9.0 on measured pins with the version **asserted** rather
  than printed, `docs --check`, and the licence suite run by name so the surface
  has its own row. The trigger is every branch: a workflow's trigger is read
  from the file on the pushed branch, which is why every row until the port came
  from a manual dispatch and a pushed branch produced nothing at all.
- `tag-pipeline.yml` builds with `go build -trimpath` out of tree, and asserts
  the platform from **two** sources — `uname` for the host, `go env` for the
  target — because one read cannot tell a wrong runner from a cross-compile. The
  artefact suffix changes with it: `freebsd-amd64` rather than the LLVM triple
  `x86_64-unknown-freebsd`, which no longer describes how the binary is built.
- `tools/check-workflows.py` was blind to every Go invocation: it matched an
  allowlist of literal tool names, so a command spelled `"$GO"` vanished
  entirely and an unbounded one reported as bounded. Any `$VAR` in command
  position is now bounded-class, with two arms, and the suite is 23.
- `docs-stale` could never fire. Its filter decided "a finding" against "a tool
  that failed to run" with words `rumor docs --check` does not emit, so a real
  refusal read as a broken tool — for any tree, in the Rust tree too. It was
  invisible because the arm had never been measured: no toolchain on the runner,
  and no project in the self-test's synthetic repos. Fixed, with a battery of
  seven real strings, and then driven by a fixture branch that tripped it and
  nothing else.
- **THE RUST TREE IS REMOVED.** `Cargo.toml`, `Cargo.lock`, `crates/`,
  `deny.toml` and `tests/fixtures/deny-gpl/` are gone, with the cargo-deny-era
  `probe-mirror-egress.yml` whose own header said it comes out in the commit
  that reads it — it had been read, green on runs 100, 113 and 118, and that
  answer is now in `docs/release.md` instead of the instrument. **Its record
  lives at the tags v0.1.0, v0.1.1, v0.1.2 and v0.1.3**, which are where that
  tree, its gate and the GPL fixture still exist; `docs/release.md` points at
  them wherever a section describes something only those tags carry.
- Two losses from the removal, stated rather than quietly taken. The licence
  allowlist had two homes, `deny.toml` and the test, which had to agree; it now
  has one, and the test reports that comparison SKIPPED naming the cause instead
  of passing. And the justification for keeping the workflow checker in python —
  that the runner could compile nothing — is retired, because the runner builds
  and tests Go at 1.26.8; what stands in the way of porting it is the self-test
  that would have to be re-earned, which is a cost rather than an obstacle.

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
