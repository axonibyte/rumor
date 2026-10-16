# Changelog

Every tag has one section. A section is written in the bump commit for its
tag and is never edited once that tag exists.

## 0.1.16 — 2026-10-16

**A `Docs-Unchanged` exemption belongs to its own release run.** Arm 9 of the
release gate unioned every trailer in the range and matched by document, so a
reason written for one release answered for a later one: with v0.1.12's bump in
range, its `docs/release.md` reason covered an unexplained `VERSION` change two
releases on — same head, only the base moved.

- **The boundary is the release, not the commit.** The union starts after the
  last bump in range that is not this run's own, since an earlier release's bump
  is in range only because the base was stale or the branch was cut from another
  release's branch. The convention stands untouched: a reason is written about
  the run, on the bump, last, when the range is known.
- **The commit boundary was tried first and refused the project's own
  history** — v0.1.12 on seven paths, v0.1.13 on three, the then-queued v0.1.14
  on one — because three of the four trailers in `v0.1.10..main` sit on a bump
  that touched only `CHANGELOG.md` and `VERSION`.
- **The accepted-release control is the arm a tightening needs here**, and
  neither earlier attempt ran it: `--self-test` now carries the ordinary shape
  of every run in this history beside the pair that pins the boundary both ways.
- **Citing an object a reader can reach** is written down beside the
  fixture-branch rules: a swept branch leaves its commits fetchable by full sha
  and no longer discoverable, and an object never pushed is unreachable whatever
  anyone sweeps.

## 0.1.15 — 2026-10-16

Three fixes from one night's reading of this project's own instruments, and the
thread they share: a check reports what it knows about, and what it does not
know about is invisible in the same shape as nothing being wrong.

- **The Runner requirements table gains its converse.** The table said what the
  runner must have; nothing said that two of those tools are absent from every
  agent jail, so a green local gate is `ci.yml` minus the lint and the
  vulnerability step and an errcheck or vulnerability finding appears for the
  first time on the push. They are a `go install` away rather than unavailable,
  and the distinction decides what a reader does next. Two runs had already been
  spent on this one cause, two weeks and two actors apart.
- **`golangci-lint`'s issue counts are uncapped in CI**, with both flags pinned
  by `tools/check-workflows.py`. The defaults cap at 3 same-text and 50
  per-linter issues and cap the SUMMARY with the list, so a step can print
  `3 issues: errcheck: 3` over a tree with four and nothing in its output
  disagrees with anything. Measured with the step's own pinned binary: the same
  tree prints 3 by default and 4 with the flags.
- **An undefined finding says so rather than rendering as nil.**
  `engine.Finding.Says()` answers the value when the statute defined one and an
  explicit absence when it did not, so the distinction between a finding that
  HOLDS and key-absence lives beside the field rather than in each consumer --
  both of which ignored it. Latent rather than live: every committed statute
  carries `default finding`. Its first arm was one-sided and a reviewer's mutant
  said so; the twin that distinguishes always-absence from correct is in the
  same commit as the arm it repairs.

## 0.1.14 — 2026-10-16

R3.2 (issue 53): the strip's second direction, and every arm given a permitted
twin. Both halves of what `rumor attaint` was missing, found by building the
checks and then pointing them at themselves.

- **The contract stated two directions for the strip and the tree checked
  one.** `docs/vocabulary.md` under `planted`: with the *tag* removed the same
  run must still produce the same `bug`, and with the planted claims *removed
  altogether* it must come back `consistent`. The second is what proves a bug
  WAS the lie rather than an artifact of the statute — a statute could pass
  every other arm while reporting one that was never about the lie. The
  counterfactual is **derived**, not committed: an eighteenth case under
  `examples/planted-lie` would move R1's gated 3/17/19, and deriving it checks
  every qualifying case rather than one fixture that could be written to pass.
- **The sentence was amended, not the fixtures.** The arm fires on three of the
  four planted cases under the unqualified reading, and those three carry a
  planted claim *incidentally*. The warrant for scoping it was not in the text
  — "its two uses are both checks on the engine" says what the checks are about,
  not which fixtures each covers — so the scope is now **in** v1's strip bullet,
  with a Revisions row recording both readings. Changing a fixture R1's gate
  passed to fit a sentence is the wrong direction of repair.
- **Three conditions the control arm tells apart**, which collapsing any two
  would lose: the control *evaluated and not passing*, *armed and never
  evaluated*, and *no control declared*. A fixture nobody observes must not read
  as a statute that stopped attributing.
- **Every arm has a permitted twin, derived rather than listed.** Nine clauses
  refuse something *absent* from a healthy tree, so the healthy package is their
  twin and no list can drift; only the two whose refusal is an *addition* carry a
  named twin. Coverage and discrimination are separate tests because they fail
  separately: an arm hard-wired to refuse passes the first and fails this one.
- **`attaint` measures R2.3's equivalence set instead of reading it**, and a
  test compares it to `tests/engine`'s declaration in both directions through
  `go/ast` — a *partial* pattern read would report "the declaration is stale",
  which is a finding about mutants that is really a finding about a regex.
- **Two latent defects**: an undefined finding reported as a surviving bug
  (absence is not a verdict), and a ref carried by both a planted and an honest
  claim, now refused by name rather than resolved by guessing which claim a tell
  meant.
- **`cli.md`'s sample is now its own check.** `release-gate.py` already executes
  blocks marked `run` and fails on a nonzero exit, but compares nothing to the
  page — so marking the block `run` would not have caught the stale tally this
  release started with; the gap was output comparison. The block greps each line
  it quotes, counts the lines against the figure **read from the prose**, and
  takes its floor from the page's own table, so the table and the sample convict
  each other. Four failure arms driven, including the sentence being reworded.

## 0.1.13 — 2026-10-16

R3.1 (issue 52): `rumor attaint`, the self-test, with fable's drills as
fixtures. `rumor check` asks whether each example's finding matches the verdict
its author wrote down — a question the statutes themselves answer, so a statute
that had quietly stopped deciding anything would still agree with a case file
written to match it. `attaint` asks whether the statutes are load-bearing at
all, and the epic's done-when is the shape of the answer: deleting a rule,
weakening a rule or stripping a planted tag must make it fail on the R1
examples.

- **Seven arms, each with its own refusal text**, because the gate reads the
  text and not the exit status. A single boolean would make a drill that trips
  the wrong arm look like a drill that passed. Four counting guards sit under
  them — no package, no case, no rule, no mutant — since every arm above is
  satisfied for free by an empty walk.
- **The mutation pass is two questions, and the second is the one a drill
  earned.** "Is every rule attacked by some mutant" is the dual of what
  `tests/engine` asserts, and it catches a deleted rule. It does *not* catch a
  weakened one: loosening `planted-lie`'s honest count passed every arm in the
  first build — the finding was still `bug`, the control still passed, every
  rule was still moved by some other mutant. What catches it is **does every
  mutant still attack**: the weakening is `m5-honest-counts-planted`'s exact
  shape, so afterwards m5 moves nothing, and a mutant that moves nothing means
  the statute has drifted into it rather than the mutant having gone harmless.
- **Both paths count, which reads like a loosening and is not.** Measured on the
  ledger alone, three rules come back unmutated: they are attacked only by the
  mutants R2.3 declared equivalent under the structural bound, so reporting them
  would blame the mutant set for a property of the seal. The output names the
  path, so a rule killed only from the cases is visible as one.
- **The equivalence set is measured here, not read.** `attaint` holds no copy of
  `tests/engine`'s declaration and exempts nothing; those three move rules from
  the cases, so they are alive and the dead-mutant arm never considers them.
  What it reports is which mutants are *inert on the ledger*, discovered
  independently, and a test compares that to `equivalentUnderTheBound` in both
  directions — so a stale declaration and a fourth mutant going equivalent each
  fail in two files for two reasons. The declaration is read through `go/ast`
  rather than a pattern: a **partial** pattern match reports "the declaration is
  stale", which is a finding about mutants that is really a finding about a
  regex, and a partial failure wears a result's clothes where a total one does
  not.
- **Three conditions the control arm tells apart**, which collapsing would lose:
  the control *evaluated and not passing* (the strip), *armed and never
  evaluated* (nothing reaches the lie), and *no control declared at all* (a
  vacuity). A fixture nobody observes must not read as a statute that stopped
  attributing. Reachability is taken from the document the engine handed the
  statute, so the knowledge rule stays in `internal/flatten` with no second
  implementation.
- **Six drills under `tests/attaint/drills/`**, each the pristine `examples/`
  tree with one file replaced, so each directory is its own diff. An overlay
  must REPLACE and never ADD: a path that drifted would land beside the original,
  leave the tree undamaged, and pass while testing nothing — so the harness
  counts files before and after. Each drill asserts the refusal **count** as
  well as the clause, because a drill that trips a second arm is not the drill
  that was designed.
- **Review fixes, in their own commit before this one.** Four unchecked
  `fmt.Fprintf` in `internal/attaint`: these lines are the evidence the verdict
  rests on, so a line nobody received is not a line that was reported. The
  runner named three of the four — `issues.max-same-issues` defaults to 3 and
  caps on the issue *text*, identical across all four, so a count read as
  completeness. And `TestEveryClauseIsExercised` did not do what it was named:
  a hand list checked only by its length, asserting one clause of eleven. It now
  reads the `Clause` constants through `go/ast` and requires the driven set to
  equal the declared set in both directions, one scenario per clause.
- `tests/vocabulary`'s declared quote count moves 13 → 14: `internal/attaint`
  quotes the `planted` entry's first use, which is the clause the corroboration
  arm implements.

## 0.1.12 — 2026-10-16

quill's second review, collected: the stale quotations and the fixtures that
outlived their mechanism, with a permanent instrument for the class.

- **Two quotations of `docs/vocabulary.md` in `internal/engine/pending.go` were
  accurate when written and false thirty minutes later — in the same tag.** The
  amendment commit reworded both entries four commits after the file quoted
  them, and both are in v0.1.11. One of them also reported a discrepancy as open
  and sent the reader to `thread:breakout-r2` for a call that had already been
  made and carried into the document above it (quill rumor/577, Q1).
- **A third misquote the oracle found and the review did not**:
  `internal/flatten/doc.go` quoted "Rules never follow tells." with a full stop
  where the document has a semicolon. Punctuation altered inside quotation marks
  is still a misquote.
- **`tests/vocabulary` holds every attributed quote in the tree to the
  document's current text** — 13 of them, the count declared so a sweep that
  finds none fails rather than passes, over a document whose normalised length
  is checked for the same reason. The attribution width is a *measurement*:
  checking every quoted span produced seven false positives for one real
  finding, because comments quote the epic and issue 44 in the same breath.
  Three normalisations — whitespace, markdown emphasis, case — each earned by a
  real quote here; without them the oracle reports three defects that are not
  there. **No exception for historical quotes**: a comment discussing an old
  wording describes it rather than quoting it, because an exception is the hole
  a stale quote hides in.
- **The three sentences corrected in `planted-lie/statute.rego` at v0.1.8 were
  left in all six of its mutants** (quill rumor/584, Q2): ADR 0001 cited by its
  drafting thread, the print ban described in Regorus's API, and the flattening
  said to be in Rust. 18 hits over 22 `.rego` files, re-swept to 0 after the
  fix. A mutant is what a reader opens when a mutant fails, so a fixture naming
  a harness that does not exist sends them looking for a call that is not there.
- **A pin catches an edit; it cannot catch a birth defect.** The drift pins hold
  three statutes and three `rules.json` and no mutant, and these sentences were
  false the day the mutants were cut — copied from the statute's header before
  the statute was corrected. So the guard is a sweep:
  `TestNoFixtureNamesARetiredMechanism` forbids five retired names across all 77
  files under `examples/`, with a positive control so an unreadable path cannot
  report a clean corpus by reading nothing, and a self-test driving a stale
  header against a corrected one.
- Not in this release: **quill's Q3**, four Rust-era nouns still in the epic's
  Decided list, whose fourth bullet is the authority the three already-fixed
  "in Rust" sentences were derived from. The epic's body is cal's, not a commit.

## 0.1.11 — 2026-10-15

R2.3: evaluation — a finding with its rule and its trace, the examples driven
from their ledger files, reconciliation, and `pending` on a clock.

- **`internal/engine` is the seam** between R2.1's store, R2.2's flattening and
  a statute. `Input` reads a sealed ledger and one check; `Evaluate` returns the
  finding with the rule definition that produced it, every named sub-rule, and
  the OPA trace retained whole for R10. The observation carries no position —
  it is the check's input, not a fact in the prefix — and sits at `seal+1`, so
  the statutes' `c.position <= ...` bound admits exactly the prefix.
- **A finding names a definition, not a rule name.**
  `data.rumor.statute.finding` is the same string for every finding in every
  case. The module and row of the definition whose body succeeded are read off
  the trace's `Exit` events, with the `default` marked, and the test requires
  two different findings in one package to come from two different rows.
- **`rumor check`** runs all 17 cases from their ledger files and prints the
  finding, its definition and its trace size per case with a tally — issue 44's
  "under one command", re-runnable from the binary. Examining nothing exits 1:
  zero disagreements over zero cases reads identically to agreement everywhere.
- **`tests/engine` requires the two paths to agree** rule by rule, case by
  case, with no shared evaluator — the ledger path through `internal/engine`,
  the case path through `rego` directly.
- **Three mutants go equivalent once the bound is structural**, and are proven
  so rather than excused: identical values for every rule on every case, and
  the case path must still kill them. A fourth going equivalent is a failure,
  and so is a stale declaration.
- **ADR 0001 at the layer that runs statutes.** `Evaluate` passes no
  `rego.EnablePrintStatements`, so OPA erases print calls and a statute full of
  them evaluates clean and silent — measured, and the reason the engine now
  refuses such a module outright. All 3 statutes and all 19 mutants are
  examined, the count reported, and a print planted into each module's own
  source as the control.
- **Reconciliation across the actors a sealed prefix names**, refused when it is
  not full: no pass, no query, an actor with no observed value, a value for
  someone the prefix does not name, or a prefix naming nobody. The control the
  done-when asks for passes — over planted-lie the honest-only set is
  `consistent`, the planted set is `bug` resting on the lie, and the
  stripped-tag set is `bug` resting on nothing.
- **`pending` is an overlay, and the clock is driven.** It needs the bound from
  the bindings (R4) and a `clock` fact, which no v1 statute reads, so the engine
  applies it over the statute's finding and keeps that finding beside it. Past
  the bound the statute's verdict stands, because v1 reserves `stale` for R6.
  Re-checked at the actor's next observation of the same query and at every full
  reconciliation; a reconciliation leaving one uncovered is refused.
- **Two findings against v1, reported rather than patched here:** the Fact kinds
  entry says `clock` facts appear in the examples' ledger files, and none of the
  17 holds one; and that entry gives a `clock` fact's actor as "the run", which
  a first reconciliation duly treated as an actor to judge.
- `docs/vocabulary.md`'s v1 row records the two readings R2.1 and R2.3 adopted,
  ratified as v1's meaning rather than changed, and three wordings fable amended
  in the same pass (breakout-r2/19): which document the flattened facts are
  fields of, that a `clock` fact belongs to no actor, and a Fact kinds note that
  now counts what is in the ledger files — `told` in eleven of seventeen,
  `clock` in none.
- **`govulncheck`'s exit 3 is a verdict, not a route problem.** `ci.yml` read
  it as "could not load the vulnerability database", which is the opposite of
  what the tool's own source says; run 192 printed "affected by 9
  vulnerabilities" and then announced a network finding. The classification
  moved to `tools/govulncheck-verdict.sh` — nine named clauses, ten cases over
  the real strings, mutation-checked — and both verdict scripts' batteries now
  run in CI, since neither was exercised by any workflow.
- **The vulnerability gate is red on this run and the cause is named.** This is
  the first commit to call `rego.Rego.Eval` from library code rather than from
  a test, so `govulncheck` now finds call paths into `net/http`,
  `net/textproto` and `crypto/tls`. All nine entries are standard library,
  found in go1.26.8 and fixed in go1.26.9, which the runner does not have.
  Measured both ways: 5de30c8 exits 0. There is no repo-side fix and none was
  invented.

## 0.1.10 — 2026-10-15

R2.1: the append-only ledger store, and the example cases as ledger files.

- **`internal/ledger` holds the four fact kinds and nothing else**, with a
  position and an actor on every fact. Positions are **assigned by the store**
  rather than passed in, because the vocabulary calls a position "assigned to
  every fact as it is appended" — if callers chose them, monotonicity would be
  the caller's invariant. `AppendAt` exists for `Load` and is the only door the
  seal has to guard.
- **Sealing, driven both ways.** Writing at or before a sealed position is an
  error and never a reorder; a position past the seal is accepted, and `Append`
  keeps working after a seal. `SealedPrefix` returns nothing for an unsealed
  ledger deliberately: a check reading an unsealed ledger is a sequencing error,
  and returning everything would hide it behind a plausible answer.
- **The dump is deterministic and diffable**, which are two properties and both
  are tested: byte-identical output for the same content, and one fact per line
  asserted by changing a single fact and requiring exactly one line to move — a
  format that reflowed would pass a round-trip test and fail that one. `Load`
  applies the same guards as `Append` and rejects unknown fields.
- **A tell cannot pass a claim that has not been made.** v1 does not state the
  rule and it follows from the entry, since "the receiver now knows that claim"
  cannot hold for a claim nobody has made. It lives in `AppendAt` rather than
  `Validate` because referential integrity is about the ledger's state, not the
  fact's shape.
- **The 17 example cases are now ledgers**, under `examples/<name>/ledger/`, and
  their positions do **not** carry over. The cases were authored as flattened
  input; a ledger needs the knowledge to arrive as `told` facts, and a tell needs
  a position of its own. Only two of the 17 leave room for one, and the three
  position-bounded cases put the observation at a claim's position, which the
  store refuses as non-monotone. The ledgers re-assign positions, preserving
  order and the bound rather than the integers — which is what the vocabulary
  allows when it says a replay assigns its own and compares findings, never
  positions.
- **The bound is now structural.** Two cases hand the statute knowledge their
  own bound excludes; placing each tell after its claim makes the seal exclude
  that knowledge, instead of leaving every statute to re-apply the bound.
- **`tests/ledger` checks the committed files against the cases and does not
  rebuild them**, because a validator that reconstructs its subject agrees with
  itself. Cases and ledgers must be in bijection, and the bound arm refuses if
  no case is found whose bound excludes anything.

## 0.1.9 — 2026-10-14

R2.2: what an actor knows, and the handle table. Plus the pin sentence wren
found.

- **`internal/flatten` computes `known(actor, claim_ref)`** from an actor's own
  claims and the one-hop `told` facts at or before a position, with an
  **ancestry extension point** that is an interface a test supplies — including
  a cyclic provider, since a walk without a visited set would hang rather than
  fail. The module doc says why this logic is outside Rego: a rule that followed
  tells would be a different statute, the fixtures pinned the output shape
  before the package existed, and an extension point needs somewhere to live.
- **The untold claim is the first test and it reads the real fixture.** The
  package produces the `known` arrays `examples/` already carries, so the
  fixture is the oracle: Charlie, told only by Alice, knows one ref, and Bob's
  claim is absent — the gap that makes that case `explained` rather than `bug`.
- **A mutation found a hole the first suite walked past.** A tell carries a
  *reference*, not the teller's knowledge, and that distinction is invisible in
  a one-claim ledger. Alice holding two claims and telling Bob one must leave
  Bob knowing exactly one; the case was added and the mutant now dies.
- **`internal/handle` keeps the run-local table for one attempt.** The handle is
  the stable name and the server id the disposable one, so `NextAttempt` returns
  an empty table rather than a copy. Three collisions: the between-attempts one
  v1 names, answered by `Require` returning an error rather than a bool — a
  caller holding `(string, bool)` can read the false as a disagreement and emit
  `bug`, reporting bookkeeping as a defect in the system under test — and two
  within-attempt conflicts that remain this project's decision, with the reasons
  recorded.
- **A `go.mod` `require` is a minimum, and `docs/design.md` said it was exact.**
  Cargo's `=` refuses a raise; minimal version selection may take a higher
  version if anything in the graph asks for one. Nothing here does — one direct
  require, 27 indirect, `go.sum` over the content — so the fact held while the
  explanation did not.

## 0.1.8 — 2026-10-14

ratchet's review of the port, acted on. Three findings, one of which changed
shape when it met the second.

- **The licence allowlist has two sources again.** R0.2's eight ids are
  enumerated in `docs/design.md` and `tests/licences` reads them from there,
  comparing against its own copy. `deny.toml` was the second reader until G4
  removed it with the Rust tree, which left the list in one place with nothing
  able to notice it drifting from the decision. A decision's home is a document,
  not a tool's config, so the pair is restored somewhere a sweep cannot take it.
- **`ci.yml` asserts the allowlist agreement and refuses any skip.** The review
  asked for the SKIP to be asserted by name, since it was printed and
  unasserted; with the arm reading the document it no longer skips, so that
  assertion would pin an unreachable state. The agreement is asserted instead,
  and a skip anywhere in the licence suite is now a finding — which is what both
  versions were protecting.
- **Three sentences in `examples/planted-lie/statute.rego` had gone false** and
  are corrected, with the drift hash re-pinned in the same commit: ADR 0001
  named by its retired thread, a harness described in Regorus's API, and R2's
  flattening said to happen in Rust. The drift test fired on the edit before the
  pin moved, which is the property — it catches an unnoticed edit and does not
  forbid a deliberate one. Byte-identity with `r1.2-examples` was built to prove
  the port changed no statute; it did, that branch is swept, and the identity is
  recorded in 0.1.4's section where it belongs.

## 0.1.7 — 2026-10-14

The vocabulary v1 as reviewed, and the engine docs brought current.

- `docs/vocabulary.md` carried onto the Go tree and amended from ratchet's
  review against the three example packages and rook's findings from building
  R2.2 against it: observations carry `of` and `role`, claims may carry
  `visible_to`, a claim's ref is its handle in v1, the between-attempts handle
  collision is named, the expectation on a claim is optional, `pending` and
  `clock` are named as gaps with the runs that exercise them, reserved words
  may appear in prose, "the same `bug` findings" compares findings alone, and
  the last Rust nouns are gone. R1.3 is this section.
- `docs/design.md`: the language-version paragraph in the past tense, since
  `go.mod` has declared 1.26.0 since v0.1.4; the print paragraph says what
  makes "statutes do not use it" true and records that OPA erases `print()`
  calls without `rego.EnablePrintStatements(true)`.
- `docs/adr/README.md`: a record is dated and frozen at acceptance.
- `docs/world.md`: the Trespass heading reserves the word and points at the
  vocabulary; it defines nothing.
- `README.md`: a workflow change is proved on its own event before it carries
  a release.

## 0.1.6 — 2026-10-14

The pipeline signs, verifies its own signature, and publishes. This is the
first tag with an artefact attached to it.

- **SHA256SUMS is signed with the release machine key**, `ssh-keygen -Y sign -n
  file` against `anvil@forge-01.corp.axonibyte.com` in the ci jail. For five
  tags the step looked for `gpg` and reported a "named wait" for a maintainer
  key; cal's amendment had decided ssh signing two days earlier, and nothing of
  the maintainer's belongs on a runner. A missing key is now a refusal naming
  the path it could not read.
- **The run verifies what it just signed**, with `ssh-keygen -Y verify` against
  the committed principal list, asserting the `Good "file" signature` line and
  not only the exit status.
- **The release is created and carries three files** — the binary,
  `SHA256SUMS`, `SHA256SUMS.sig` — with the tag's changelog section as its
  body. `GET /releases` was empty for v0.1.0 through v0.1.5; the epic had asked
  for this since R0. The upload is read back and the asset list compared,
  because a 201 and a release that carries the file are different claims. The
  artefact's name is read off `SHA256SUMS` rather than recomposed, since that
  file records what was actually built.
- **`fixture-v*` is in the trigger**, so the pipeline's three refusals can be
  driven by any agent instead of asking cal to push deliberately bad `v*` tags.
  A fixture runs every check and publishes nothing, refused by the tag's shape.
- `tools/forge-release.py` carries a battery over the changelog-section reader
  and the multipart body. It does **not** cover the create-and-attach half —
  nothing in it talks to the forge — and both the tool and the methodology doc
  say so, because a battery that implied otherwise would read as coverage of
  the only part that can fail in production.
- **The tag is checked out, which it never was.** `git clone` leaves the default
  branch checked out, so the version agreement, the build and `tools/` were all
  reading `main` while reporting on the tag — invisible for five releases
  because a fast-forward release makes those the same commit. The run now checks
  out the tag's tree, takes `tools/` from the workflow's own commit, and asserts
  both; an unasserted checkout is the defect being repaired.
- **`workflow_dispatch` takes a `tag` input**, so this file is proved against an
  existing tag before it carries a release. Every stale step in it was found one
  tag at a time, each costing a release, because the only trigger was a `v*`
  push that only the maintainer can make. The tag is resolved once and read back
  by the six steps that need it, rather than each repeating its own fallback.

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
