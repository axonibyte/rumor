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

`.forgejo/workflows/release-gate.yml` runs on every pull request to `main` and
calls `tools/release-gate.py`. The epic's list is the source of truth; this
section is its canonical copy and the fixture-branch convention beside it.

**Nine bullets, twelve arms.** Two bullets carry more than one failure, so each
arm has a stable id and a fixture branch names exactly one:

| arm | id | the refusal |
|---|---|---|
| 1 | `version-unchanged` | the bump commit does not change the workspace version |
| 2 | `subject-mismatch` | the subject is not `release: vX.Y.Z` matching that version |
| 3 | `commit-after-bump` | a commit follows the bump commit |
| 4 | `version-not-greater` | the new version is not greater than the latest tag |
| 5 | `changelog-section-absent` | no `CHANGELOG.md` section for the new version |
| 6 | `changelog-section-empty` | the section is present and empty |
| 7 | `changelog-date-wrong` | the section is the right version and not today's date |
| 8 | `docs-stale` | `rumor docs --check` finds generated docs out of date |
| 9 | `docs-unchanged-no-trailer` | a source path changed, a mapped doc did not, and no `Docs-Unchanged` trailer covers it |
| 10 | `revision-entry-missing` | a contract or vocabulary version constant moved with no new entry under "Revisions" |
| 11 | `doc-run-block-failed` | a doc command block marked `run` fails |
| 12 | `not-rebased` | the branch is not rebased on `main` |

Arms 1 and 2 are bullet 1's disjunction; arms 5, 6 and 7 are bullet 4's three
failure modes. Every other bullet is one arm.

### The gate does not short-circuit

Every arm runs on every invocation, each finding prints with its id, and the
exit status says only whether the set is empty. This is what makes the fixture
test answerable: a branch must show **the reason it names and no other**, and a
gate that stopped at its first finding could not tell a branch tripping three
arms from one tripping its own.

### Arm 4 presupposes a new version

Bullet 4 says "the **new** version is not greater than the latest tag". When the
version did not change there is no new version, so arm 4 has no subject and does
not apply. Without this, every arm-1 fixture also fails arm 4 — an unchanged
version equals the latest tag and so is not greater than it — and neither arm
could ever show its own reason alone. The alternatives were worse: a
short-circuit destroys the test for all twelve, and reading bullet 4 as "changed
but not greater" amends the epic from a child issue.

### Three exit statuses, and the third is not a verdict

`0` clean, `1` one or more findings, `2` the gate could not reach a verdict —
a bad invocation, an unreadable file, a git that refused. **A gate that cannot
run is not a gate that passed**, so the workflow treats `2` as a failure of the
run. This is the same separation `ci.yml` makes for `rc 124`: a tool that never
answered is a statement about the runner, and collapsing it into either verdict
is how a broken gate reads as a green one.

### Why the gate is python, against the standing argument

`tools/check-workflows.py` is deliberately local-only, because "adding a python
dependency to CI would repeat the `actions/checkout` mistake of depending on an
unmeasured interpreter". That reasoning holds, so the workflow **measures**
`python3` in a probe and the gate step **fails loudly** when it is absent rather
than skipping. Re-implementing twelve arms in `sh` would have given up the
self-test that found five defects in them before a branch existed, and a `rumor`
subcommand is a Rust change this issue does not ask for and could not be driven
on a box with no toolchain. If a runner turns out to lack `python3`, the honest
fix is the subcommand, not a quietly skipped gate.

### The fixture-branch convention

One branch per arm, named `r0.3-fix-<id>`, each cut from `main` and carrying the
smallest change that trips its own arm. The branch's PR must show the gate red
naming that arm and no other, which `tools/release-gate.py --expect <id>`
asserts directly. Two further rules, both learned the hard way:

- **A fixture must reach its arm.** A fixture that cannot produce the condition
  is green for the wrong reason, and a green fixture proves nothing.
- **Drive all twelve in one window**, bounded by `git ls-remote origin main`
  before and after and asserted equal. If `main` advances while the twelve sit
  as open PRs, all twelve also fail `not-rebased` and no fixture shows its own
  reason — a confounder that has already bitten this repo once, when `r0.2-ci`
  went three commits behind inside a day.
- **A fixture must be red for one reason across the whole PR, not just in the
  gate.** The branch's other checks run too. The first
  `revision-entry-missing` fixture put its constant above `main.rs`'s inner doc
  comments, so the tree did not compile: the arm fired correctly, `docs --check`
  could not build, arm `docs-stale` went UNMEASURED, and `ci.yml` would have
  been red for E0753 — a second reason the fixture does not name. Run
  `cargo test --locked`, `cargo fmt --all --check` and
  `cargo clippy --locked --all-targets -- -D warnings` on each fixture before
  opening its PR.
- **Verify each fixture with its own tree checked out.** Arm `docs-stale` runs
  `rumor docs --check`, which reads the working tree, so it is the one arm whose
  answer depends on the checkout. The gate reports UNMEASURED when the checkout
  is not the run being gated; a verification from another branch therefore
  leaves that arm unmeasured rather than wrong, but it also leaves it unproven.
- **Never rewrite a pushed fixture.** Re-cut it under a `-<n>` suffix, as
  `r0.2-ci-2` was, and leave the original on the forge as the record. The
  asserting step strips a trailing `-<digits>` when it derives the arm from the
  branch name, so a re-cut keeps the arm it declares.

#### The branches, and the window that bounds them

Eleven fixtures plus the arm-9 half of the pair were driven in one window whose
endpoints were read with `git ls-remote origin main` and are identical, so
`main` did not move across the batch and arm `not-rebased` cannot have
confounded the rest:

| window | `main` | |
| --- | --- | --- |
| open | `5e8a6392c4c612b72fc2006b070a7e3513af3158` | |
| close | `5e8a6392c4c612b72fc2006b070a7e3513af3158` | identical |

| arm | branch |
| --- | --- |
| `version-unchanged` | `r0.3-fix-version-unchanged` |
| `subject-mismatch` | `r0.3-fix-subject-mismatch` |
| `commit-after-bump` | `r0.3-fix-commit-after-bump` |
| `version-not-greater` | `r0.3-fix-version-not-greater` |
| `changelog-section-absent` | `r0.3-fix-changelog-section-absent` |
| `changelog-section-empty` | `r0.3-fix-changelog-section-empty` |
| `changelog-date-wrong` | `r0.3-fix-changelog-date-wrong` |
| `docs-stale` | `r0.3-fix-docs-stale` |
| `revision-entry-missing` | `r0.3-fix-revision-entry-missing-2` |
| `doc-run-block-failed` | `r0.3-fix-doc-run-block-failed` |
| `not-rebased` | `r0.3-fix-not-rebased` |
| `docs-unchanged-no-trailer` | `r0.3-fix-docs-unchanged-no-trailer-2` |
| *(none — the control)* | `r0.3-control-docs-unchanged-3` |

Three branches are superseded and stay on the forge unrewritten:
`r0.3-fix-docs-unchanged-no-trailer` and `r0.3-control-docs-unchanged` carried a
changelog section inserted above the file's preamble, and
`r0.3-fix-revision-entry-missing` did not compile. `r0.3-control-docs-unchanged-2`
is kept deliberately: its trailer is hard-wrapped to column 0, so it is now a
**red** control for the wrap rule below.

#### `r0.3-fix-docs-stale` is red twice, from one cause

Its unmapped path also fails `coverage::tests::committed_tree_is_fully_mapped`,
because the arm and that test measure the same property. This is one cause with
two reporters, not the two-unrelated-reasons ambiguity the rule above forbids,
and it is not removable: mapping the path would destroy the fixture. Every other
fixture and the control are green in `ci.yml`.

#### Writing a `Docs-Unchanged` trailer

It is a git trailer and the gate reads it as one.

- The separator is `--` or an em dash. A lone `-` is **not** accepted: it is
  ambiguous against a hyphenated filename, and widening the separator widens the
  exemption, which is the one direction worth being strict in.
- A reason longer than one line **continues on an indented line**, which is
  git's own folding rule. The gate joins the indented continuations.
- A reason wrapped to **column 0 exempts nothing in that commit message** —
  not the wrapped trailer, and not a well-formed trailer beside it. An
  unindented line ends the trailer block, which is why `git interpret-trailers`
  reports no trailers at all for such a message. The first version of this
  reader instead truncated the reason, turning "the edit is a comment and
  changes no command, flag or output" into "…and changes no", which reverses it;
  the second kept the sibling trailer and so was wider than git. The gate prints
  a note on stderr naming the file, and `docs-unchanged-no-trailer` fires for
  every doc the message claimed.

  The gate itself never reads the reason's *value* — arm 9 asks only whether a
  doc has a trailer — and the epic's "the reasons are copied into the PR summary"
  and rook's sign-off on each reason are hand steps worked from the commit
  message. So the truncation had no consumer inside the gate and no arm could go
  red for it. That is why the shape is written down here: it is a defect with no
  symptom, and the refusal is what gives it one.

**Read the reason from the commit message body. Never from a trailer parser.**
The reviewer who signs off on each reason (rook, per the epic) needs the source
named, because both sources nearer to hand are wrong (rook, thread:r0.3/17):

| source | what it gives you |
| --- | --- |
| the commit message body | the reason as written — **this is the one** |
| the release gate's output | no reason at all; it reads only whether a trailer exists, so a reader can wrongly conclude none was given |
| `git interpret-trailers --parse` | the reason, folded, when the block is well formed — and **nothing at all, with exit status 0**, when it is not |

That last row was measured rather than assumed, and it is not what any of us
expected (rook thread:r0.3/17 and /20, ibis /18, measured twice independently).
On a reason wrapped to column 0, `interpret-trailers` does not truncate the
entry: the unindented line **ends the trailer block**, so git reports no trailers
for that message — including any well-formed trailer beside it.

**The exit status is 0.** That is the reason this row matters more than the rule
does. A truncated sentence is something a reviewer might notice; an empty answer
with a success status is indistinguishable from "this commit claimed no
exemption", so a reviewer would conclude the opposite of the truth and have no
signal that anything was wrong. Hence: read the body.

The gate is pinned to that behavior by a case in `--self-test` that runs both
parsers on the same message and requires the same answer. The reader used to
keep the well-formed sibling in a broken block, which made it **wider than git**
on exactly the malformed input; a broken block now exempts nothing.
- An empty reason does not exempt.

### The positive control

One further branch that the gate must **pass**: a source change whose mapped doc
is unchanged, carrying `Docs-Unchanged: <doc> — <reason>`. It differs from arm
9's fixture **only by the trailer** — same source path, same mapped doc, same
edit. `r0.3-control-docs-unchanged-3` is made with `git commit-tree` on arm 9's
fixture's own tree object, so `git diff` between the two branches is empty and
the message is provably the only difference either run can see.

A control is only a control when every arm actually ran. Verify it with its own
tree checked out and **require stderr to be empty**: a control that passes with
`docs-stale` UNMEASURED has twelve arms on paper and eleven in fact. Two branches differing in two things would not show which one the gate
read. Without a passing control, twelve refusals prove the gate is loud rather
than discriminating: a gate that refuses everything satisfies all twelve.

## Tag pipeline

`.forgejo/workflows/tag-pipeline.yml` runs on a `v*` tag push and **publishes
nothing when it refuses**, which is why every check sits before the build rather
than after it.

It refuses, each with its own message:

- the tag is **lightweight** — checked by object type, since a lightweight tag
  is a ref to a commit and can never carry a signature
- the tag **carries no signature**
- the tag has a signature that **fails verification** — reported separately from
  a runner with **no trust material**, because the second is a provisioning
  finding and announcing it as the first would accuse a correct tag of forgery
- the tagged commit is **not `main`'s head**, or is **not a bump commit**
- the tag version, the `Cargo.toml` version and the changelog heading disagree

On success it builds the release binary, writes `SHA256SUMS` over what was
actually built, and signs it.

### FreeBSD amd64 only

One build, no cross-build. An artifact nobody ran is not a release artifact, and
there is no Linux runner; `SHA256SUMS` covers what was built and the release
notes say what was not. Linux arrives by its own patch run the day a runner
exists — and **not** as a job added in advance, because a job whose label no
runner answers queues rather than failing, which would make every green in the
file ambiguous.

### The signature is a named wait, not a silent skip

Signing needs the maintainer's private key and none is on the runner. The step
attempts the signature and names its absence as a **provisioning finding**
rather than treating it as done: a pipeline that quietly shipped an unsigned
checksum file would be the always-pass shape of the very gate this phase builds.
Until a signer exists on the runner, a tag run reports `SHA256SUMS` written and
UNSIGNED, and the release is incomplete by the epic's own list rather than by
the file's silence.

## Runner requirements

Every tool a workflow in this repository depends on, so a second runner is stood
up from this list rather than from archaeology in the YAML. A dependency on the
runner is allowed **when it is declared** (fable, thread:rumor/342); this is that
declaration, and anything a workflow needs which is not here is a defect in this
list.

| tool | used by | pinned? |
| --- | --- | --- |
| `rustc`, `cargo` | every build, test and lint step; the release build | by the repository's toolchain file |
| `cargo-deny` | `ci.yml`'s licence gate | **0.20.2**, asserted against the install line so moving one without the other fails the step |
| `python3` | `release-gate.yml` runs `tools/release-gate.py`; `tools/check-workflows.py` is local-only | no version pin; the gate's probe step measures it and fails loudly when absent |
| `sha256` or `sha256sum` | `tag-pipeline.yml`'s checksum step | either; the step measures which is present and refuses when neither is |
| `gpg` | `tag-pipeline.yml`'s signature step | optional — a missing trust store is reported separately from a failed signature |

**The gate's own probe is the enforcement, not this table.** `release-gate.yml`
measures `python3` and fails the run when it is absent rather than skipping, so a
runner missing it produces a named failure instead of a quiet pass. The table
exists so the failure is anticipated rather than diagnosed.

**`runs-on` binds to a LABEL, not to a platform.** The label says which runner
answers; it does not measure what that runner has. `tag-pipeline.yml` therefore
asserts its host triple against `x86_64-unknown-freebsd` from `rustc -vV` rather
than trusting the label, and refuses to publish from any other host.

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

### Every external call is bounded, and a hang is a named finding

The runner is **single-concurrency**. One job that hangs does not merely fail
slowly: it head-of-line blocks every queued run and every push anyone makes
until someone cancels it. This happened twice. `runner-smoke` run 3 ran 1816 s
and was cancelled; `probe-pkg` run 97 ran past 20 minutes on 2026-10-10 with
three unbounded `pkg` calls, holding three queued runs behind it.

So every workflow carries two bounds, and the duplication is deliberate:

| mechanism | bounds | why not alone |
| --- | --- | --- |
| `timeout-minutes` on the job | anything, including what the file does not anticipate | a runner feature nobody here has observed working — the run logs are 404 to us |
| `timeout N` on each call | the specific call, and names which one | cannot bound a hang somewhere no call was expected |

**`rc` 124 is reported separately from any other failure.** They are different
findings: 124 says the tool never answered inside its bound, which is a
statement about the *runner*; any other non-zero says the tool answered and the
answer was no, which is a statement about the *code*. Collapsing them files a
wedged runner as a broken build.

**Capture status with `cmd ... || rc=$?`, never a bare `rc=$?` on the next
line.** `run:` blocks execute under `bash -e`, where a failing command aborts
the step immediately — so a capture on the following line never executes, and
neither does the `tail` of the log nor the 124 discrimination. This is measured,
not inferred: under `bash -e`, `false; rc=$?; echo "$rc"` prints nothing and
exits 1, and `timeout 1 sleep 5; rc=$?; test "$rc" -eq 124 && echo HANG` exits
124 *without printing HANG*. `-e` is also in force inside a group on the right
of `||`, so `cmd || { cmd2; rc=$?; }` fails the same way. A command on the left
of `||` is exempt from `-e`, which is what makes the guarded form work.

This one is worth stating plainly because the first version of `ci.yml` used the
bare form at every step. That file had just been corrected for four
pipeline-status defects — `cargo build | tail` returning `tail`'s status — and
the correction was "redirect, read the status, then format". The redirect was
right and the read was unreachable, so on a failure the log still never printed.
One bug was replaced by another with the same symptom.

### Why one version is a literal and the other is derived

The deny step holds two versions and treats them oppositely, which looks
inconsistent until you ask what each one is *for* (ratchet, r0.2/16):

| value | treatment | because |
| --- | --- | --- |
| `cargo-deny` 0.20.2 | hard-coded literal, asserted by `grep` | the pin is the **subject** of the check. Deriving it would make the check agree with whatever is installed, which is the drift it exists to refuse. |
| `rumor-cli@<version>` | derived from `cargo metadata` | the crate version is an **input** to the check. A literal here carries the author's context — the first draft of this assertion said `0.1.3`, which was the author's branch, and would have failed on `0.1.0` and again at every bump. |

So: *derived pin, no guard; derived version, no brittleness.* The test is whether
the value is what the check is about or merely something the check needs. Do not
"unify" these two by reading both from one place — the duplication of `0.20.2`
between the install line and the assertion is deliberate, and is itself the
check.

### Test 3: the licence gate's negative control

`cargo deny check licenses` passed from the first run, and that proved nothing.
ibis read task 139's job log and found why: all seven unused allowances raised
`license-not-encountered`, and `deny` resolved `rumor-cli@0.1.0` alone, so the
gate passed having examined **no dependency licence at all**. A gate that
encounters none of its own allowances cannot fail, and a gate never seen to fail
is indistinguishable from one that is not wired up.

`tests/fixtures/deny-gpl/` is the control. It is a separate workspace holding one
crate licensed `GPL-3.0-only` -- a valid SPDX id, absent from `deny.toml`'s
`allow` -- depended on by a `BSD-2-Clause` root, so the only violation in that
graph is the GPL crate and a refusal cannot be about anything else. The root
manifest `exclude`s the path, so the fixture reaches the control and never the
shipped graph.

The CI step drives it with the **real** `deny.toml` (`--config deny.toml`), never
a copy: a control derived from a copy of the thing it checks agrees with itself
whatever the real file says. It then reads two oracles, because an exit status
alone does not say which licence was refused:

| oracle | failure it catches |
| --- | --- |
| `rc` is not 0 | the gate passed a GPL dependency, so it does not gate |
| `rc` is not 1 | `deny.toml` did not deserialize, so the control never ran |
| log names `GPL-3.0-only` | something else refused, so this shows nothing |
| log names `gpl-fixture` | the refusal is not about the fixture crate |

Driven locally against cargo-deny 0.20.2 before the step was written: rc 4,
`rejected: license is not explicitly allowed`, both names present; and the real
workspace still rc 0 with the same seven warnings, which is what proves the
exclusion held.

### tools/check-workflows.py

`tools/check-workflows.py` refuses a workflow that can hang or cannot report
why. Three checks: every job declares `timeout-minutes`; every invocation of a
network or build tool carries a bound (`timeout`, or `curl -m` — *not*
`--connect-timeout`, which bounds only the handshake); and no bare `rc=$?`.

Run it before committing a workflow change, with its own self-test:

    python3 tools/check-workflows.py --self-test      # 10 arms
    python3 tools/check-workflows.py .forgejo/workflows

The self-test's primary fixture is `probe-pkg.yml` **as it actually shipped** —
the file that wedged the runner. A checker that does not refuse that
byte-for-byte would not have prevented what it exists to prevent.

Two bounds on it, stated here rather than only in its header:

- **It is textual, not semantic.** It finds a tool name in command position on a
  logical line. It does not follow `sh -c`, a variable holding a command name, a
  script the workflow writes and then runs, or `eval`. Its silence is not proof
  that nothing can hang.
- **It is local-only, and that is a limitation rather than a design.** It is
  Python because the runner cannot compile anything, so a Rust gate could not run
  there; and adding a Python dependency to CI would repeat the `actions/checkout`
  mistake of depending on an unmeasured interpreter on the runner. It should
  become a `rumor` subcommand beside `docs --check` once that jail has a working
  toolchain. Until then nothing *enforces* it — a reviewer has to run it.
