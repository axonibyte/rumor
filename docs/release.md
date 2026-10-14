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

`main` is **fast-forward only**, and every bump commit sets the version in the
root `VERSION` file. Those two together mean the version on `main` can never go
backwards, which is the property the release gate relies on rather than checking
separately.

> **The version lives in a file because Go has nowhere else to put it.** Until
> v0.1.3 it was `Cargo.toml`'s `[workspace.package] version`. `go.mod` has no
> version field — a Go module takes its version from the tag — so the manifest
> cannot be the second of the three sources the gate makes agree. Dropping to a
> two-way tag-against-changelog check was the alternative and it is weaker: the
> refusal is about three sources disagreeing, and two cannot express it. So
> `VERSION` holds one line of dot-separated digits and nothing else writes it.
> Both readers are strict and their strictness is duplicated — `project_version`
> in the gate, three shell tests in `tag-pipeline.yml` — because two readers for
> one fact can disagree about what is well-formed and then one publishes what the
> other refused. They were driven against each other over 17 inputs; see the
> commit that moved them.

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
subcommand is a change this issue does not ask for. **The reason it could not
be written has since expired**: the argument was that the runner compiles
nothing, and the runner now builds and tests Go at 1.26.8, so the subcommand is
available as a route and is a decision rather than an impossibility. If a runner
turns out to lack `python3`, the honest fix is the subcommand, not a quietly
skipped gate.

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
  `go test ./...`, `gofmt -l .` (which must print nothing) and `go vet ./...`
  on each fixture before opening its PR. Name the versioned binary if the box
  has no bare `go`, and export `GOTOOLCHAIN=local` so a directive raise refuses
  instead of downloading a toolchain nobody chose.
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
- the tag version, the `VERSION` file and the changelog heading disagree

On success it builds the release binary, writes `SHA256SUMS` over what was
actually built, and signs it.

### The artefact's name

`rumor-<tag>-<platform>`, where `<tag>` is the tag as pushed, `v` included, and
`<platform>` is `GOOS-GOARCH` measured on the runner rather than written in —
`rumor-v0.1.4-freebsd-amd64`. `SHA256SUMS` names that file and nothing else.

> **The suffix changed at v0.1.4 and this is the record of it.** v0.1.0 through
> v0.1.3 published `rumor-<tag>-x86_64-unknown-freebsd`: an LLVM target triple,
> read out of `rustc -vV`. That is a Rust concept and it no longer describes how
> the binary is built, so keeping it would be a filename asserting a toolchain
> the artefact never saw. Anything pinning the old pattern — a download script,
> a mirror, a checksum reader — needs the new one from v0.1.4 forward. This was
> flagged for overrule before it landed (rook thread:go-port/9) and confirmed
> (fable /10).

### FreeBSD amd64 only

One build, no cross-build. An artifact nobody ran is not a release artifact, and
there is no Linux runner; `SHA256SUMS` covers what was built and the release
notes say what was not. Linux arrives by its own patch run the day a runner
exists — and **not** as a job added in advance, because a job whose label no
runner answers queues rather than failing, which would make every green in the
file ambiguous.

### Verifying the tag: three answers, not two

`git verify-tag` exits non-zero for two unrelated reasons, and only one of them
is about the tag. So the step reads the log through
`tools/verify-tag-verdict.sh`, which answers one of three ways: **verified**
(publish), **refused** (a verdict on the tag, publish nothing), or
**provisioning** (the signature could not be checked here — not a verdict, and
not a pass either).

**The runner must be pointed at a trust store, and v0.1.4 is what happens when
it is not.** An ssh-signed tag needs `gpg.ssh.allowedSignersFile`; without it
git does not check the signature at all, it errors with *"needs to be
configured and exist"*. Run 162 reported that as a failed verification and so
accused a correctly signed tag. The step now sets the config to the file
provisioned in the ci jail, `/usr/local/etc/anvil/allowed_signers` (issue 25),
asserts git reads the value back, and refuses as **provisioning** when the file
is unreadable.

**A good signature by an untrusted signer is a verdict, not a gap.** When the
store is configured and the signer is not on the list, git prints a `Good ...
signature` line *and* `No principal matched.` The old inline test matched "no
principal" and called that provisioning — granting what it should refuse, which
is the one direction a release gate must never fail in (quill, rumor/448). The
`Good ... signature` line is the discriminator: trust material was read.

The classification lives in a script rather than in the YAML because **a
workflow step is only exercised by pushing a tag**. `verify-tag-verdict.sh
--self-test` drives eight cases over strings this project has really seen —
run 162's error, ibis's wrong-key output, gpg's "No public key", an empty log,
a missing log — and each case asserts the **clause that answered**, by name,
rather than the exit status. Three mutants were driven against it: restoring the
old vocabulary, dropping the good-signature guard, and reversing the clause
order. Each kills at least one case.

### Signing SHA256SUMS: a machine key, and the run verifies its own output

For three tags this was a named wait, on the premise that signing needs the
maintainer's key and none belongs on a runner. **That premise is retired.** The
release signer is a machine key — `anvil@forge-01.corp.axonibyte.com`,
generated inside the ci jail by its own user, private half never leaving it
(cal's amendment, rumor/435; provisioned under issue 25). Nothing of cal's is
on the runner and nothing needs to be.

    ssh-keygen -Y sign -n file -f /usr/local/etc/anvil/anvil_ed25519 SHA256SUMS

**The run then verifies what it just produced**, with the verifier's tool and
the principal list a consumer would use:

    ssh-keygen -Y verify -f /usr/local/etc/anvil/allowed_signers \
      -I anvil@forge-01.corp.axonibyte.com -n file -s SHA256SUMS.sig < SHA256SUMS

Two oracles on one claim: "the signer exited 0" says a process ran, and only
the verify says the signature checks out against the list. The step asserts the
`Good "file" signature` line as well as the status, because a tool that prints
nothing and exits 0 would otherwise pass.

**A missing key is a refusal that names the file**, not a wait. The distinction
used to be "signed or waiting on a decision"; the decision is made, so a key
that is absent or unreadable is a provisioning defect in a jail that has been
provisioned, and the run says which path it could not read.

### What a release carries, and that it is read back

The epic asks the pipeline to attach "the binaries, checksums, and changelog
section to the release". For v0.1.0 through v0.1.5 it attached nothing —
`GET /releases` was empty for every tag. `tools/forge-release.py` closes that:
it creates the release for the tag, uses **the tag's own changelog section** as
the body, and uploads three files.

| file | what it is |
| --- | --- |
| `rumor-<tag>-freebsd-amd64` | the binary, built `-trimpath` out of tree |
| `SHA256SUMS` | one line, over what was actually built |
| `SHA256SUMS.sig` | the ssh signature, verified in the same run |

Two details that are decisions rather than mechanics. **The artefact's name is
read off `SHA256SUMS`**, not recomposed from the tag and the platform: the
build step owns that naming rule and a step cannot see another step's
variables, so recomposing it would be a second copy that drifts — and
`SHA256SUMS` is the record of what was really built. And **the release is read
back** after the uploads, with its asset list compared to what was sent,
because a 201 from an upload and a release that carries the file are different
claims.

### Fixtures for the tag pipeline

The gate has twelve fixture branches, one per refusal arm. The tag pipeline had
none, because its trigger is `v*` and tags are the maintainer's by protection —
so driving its refusals meant asking cal to push deliberately bad tags.

The trigger now also matches **`fixture-v*`**, which any agent may push, and
the publish step refuses to publish such a tag **by its shape**. So a fixture
runs every check and creates no release, and a fixture that somehow passed
every check still publishes nothing. The three refusals it exists to drive are
an unsigned or lightweight tag, a tagged commit that is not `main`'s head or
not a bump commit, and the three version sources disagreeing.

## Runner requirements

Every tool a workflow in this repository depends on, so a second runner is stood
up from this list rather than from archaeology in the YAML. A dependency on the
runner is allowed **when it is declared** (fable, thread:rumor/342); this is that
declaration, and anything a workflow needs which is not here is a defect in this
list.

| tool | used by | pinned? |
| --- | --- | --- |
| `go` | every build, test and lint step; the release build | not version-pinned, but the `go` directive in `go.mod` is the floor and `GOTOOLCHAIN=local` makes a raise refuse rather than download. **Resolved, not named**: FreeBSD ports installs versioned binaries, so a box may have `go126`/`go125` and no bare `go`; every recipe tries `go`, `go126`, `go125` in that order and fails naming all three |
| `gofmt` | `ci.yml`'s fmt step | same resolution, `gofmt`/`gofmt126`/`gofmt125` |
| `golangci-lint`, `govulncheck` | `ci.yml`'s lint and vulnerability steps | **2.14.0** and **v1.9.0**, installed by pinned `go install` when absent and the version then asserted rather than printed |
| `python3` | `release-gate.yml` runs `tools/release-gate.py`; `tools/check-workflows.py` is local-only | no version pin; the gate's probe step measures it and fails loudly when absent |
| `sha256` or `sha256sum` | `tag-pipeline.yml`'s checksum step | either; the step measures which is present and refuses when neither is |
| `ssh-keygen` | `tag-pipeline.yml` signs `SHA256SUMS` and verifies it back | **required**, from `FreeBSD-ssh`; it replaced `gpg`, which the pipeline no longer calls. The release key and the allowed-signers file live at `/usr/local/etc/anvil/` (issue 25) |
| `gpg` | nothing calls it; it stays in the probe census | **not required.** Signing is `ssh-keygen` and the tags are ssh-signed, so `git verify-tag` never reaches gpg. It is measured anyway because a PGP-signed tag would need it, and `tools/verify-tag-verdict.sh` still classifies gpg's "No public key" as provisioning rather than as a bad signature |
| `git` | every workflow: the clone, the fetch, every rev-parse, merge-base and ls-tree the gate's arms rest on | no version pin; **not at `/usr/bin/git`** on these hosts — it is `/usr/local/bin/git`, so a runner image that provides only the base system lacks it |

**The runner reaches the FreeBSD package mirror.** `probe-mirror-egress.yml`
asked that question — ratchet's design, r0.2/7 — and answered it green on runs
100, 113 and 118. The instrument said it would come out in the commit that read
it, and it did; the answer is recorded here because the fork it settled is
closed either way. `pkg` itself still refuses on this runner for want of
privilege, which is a refusal rather than a hang and is a separate matter.

**Installed from the `axb` pkg repositories** (fable, thread:rumor/342 and /351).
A second runner is provisioned from this table and that source, not from reading
the YAML.

**`git` was missing from the first version of this table** (ibis,
thread:rumor/372), which is the table's own rule catching its author: anything a
workflow needs and this list omits is a defect in the list. It is the most-used
tool here after the Go toolchain, the gate chooses it deliberately over
`actions/checkout`,
and it is **not** in the base system on these hosts — so the omission would have
produced a runner that could not clone, discovered at the first job rather than
at provisioning time.

**The gate's own probe is the enforcement, not this table.** `release-gate.yml`
measures `python3` and fails the run when it is absent rather than skipping, so a
runner missing it produces a named failure instead of a quiet pass. The table
exists so the failure is anticipated rather than diagnosed.

**`runs-on` binds to a LABEL, not to a platform.** The label says which runner
answers; it does not measure what that runner has. `tag-pipeline.yml` therefore
asserts the platform from **two** sources rather than trusting the label, and
refuses to publish when either disagrees: `uname -s`/`uname -m` for the host,
and `go env GOOS GOARCH` for the target. One read is not enough here, and that
is a change from the Rust pipeline rather than a translation of it — `rustc -vV`
reported a *host* triple, so a single read answered both questions, whereas
`go env` reports what the toolchain will *build for* and an environment variable
can set it. Asserting only the target would let a cross-compile publish;
asserting only the host would let a cross-compiled artefact ship from the right
machine.

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
string, a count the step computed, the toolchain recorded *inside* the built
binary by `go version -m`. Never a duration, and never the absence of red.

### The licence surface, after the port

`cargo deny` is gone, with the tree it judged. What survives it is the
allowlist settled in R0.2 and the reasoning in the sections below, which the Go
check inherits; the tool's own mechanics are at the tags v0.1.2 and v0.1.3.

The live gate is `tests/licences`, run by `go test ./...` and then again **by
name** in its own CI step, so the surface has a row with a figure in it rather
than a verdict folded into another step's output.

Three things differ from `cargo deny`, and each is a reason:

- **The oracle is what gets built**, `go list -deps -test ./...`, tests
  included. Not the module graph, which over-counts: a module required for a
  dependency's own tests is in the graph and not in the binary, and refusing a
  licence the project never ships is a false refusal that teaches people to
  widen allowlists.
- **Multiplicity is refused, not resolved.** A licence file naming two families
  is a refusal unless a human has read it and recorded the decision in the test,
  because a first-match classifier admits a module under "MIT OR GPL-3.0" on
  whichever marker it happened to check first. One module is recorded that way
  today, with the reading written out.
- **A forbidden family is named, not merely unmatched.** The classifier knows
  the copyleft titles even though none of them is allowed, so a GPL dependency
  refuses as "GPL-3.0 is outside R0.2's allowlist" rather than as
  "unclassified", which would read like a gap in the classifier instead of a
  licence the project cannot ship.

It carries Test 3's lesson as its own negative control: seven synthetic
modules driven through the same verdict function the real surface uses, five
refused and two passed, each asserting **the clause that answered** rather than
the boolean — four clauses refuse, and a control satisfied by any refusal would
also be satisfied by a gate that refuses everything.

### go.sum, and what it is for here

`go.sum` is in the tree and is not a lockfile. It records the hash of every
module version in the graph, so a changed upstream tarball fails verification
rather than building; the *versions* are chosen by `go.mod` and the module
graph. It is in the doc coverage map against this file because it changes
whenever the dependency set does, and a dependency arriving with no document
saying why is the thing the map exists to catch.

### Licence id spellings, and why they are not the Decided words

`Unicode-3.0`, where the Decided line says "Unicode", which is not an SPDX id.
The allowlist is SPDX ids because that is what a classifier can compare; the
Decided list is prose, and the two are deliberately not the same strings.

The rest of this section went out with `cargo deny`: an exit-code table for a
tool no longer installed, and two entries about crate-level licence
disjunctions (`Apache-2.0 WITH LLVM-exception`, `Unicode-DFS-2016`) that
described a dependency set this repository no longer has. The record is at the
tags v0.1.2 and v0.1.3, where that tool and those crates exist.

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

A step that holds two versions may have to treat them oppositely, which looks
inconsistent until you ask what each one is *for* (ratchet, r0.2/16). The live
pair is in `ci.yml`'s lint and vulnerability steps:

| value | treatment | because |
| --- | --- | --- |
| `golangci-lint` 2.14.0, `govulncheck` v1.9.0 | hard-coded literals, asserted against the install line | the pin is the **subject** of the check. Deriving it would make the check agree with whatever is installed, which is the drift it exists to refuse. |
| the project's own version | read from `VERSION` | the version is an **input** to the check. A literal here carries the author's context — the first draft of the cargo-era assertion said `0.1.3`, which was the author's branch, and would have failed on `0.1.0` and again at every bump. |

So: *derived pin, no guard; derived version, no brittleness.* The test is whether
the value is what the check is about or merely something the check needs. Do not
"unify" these two by reading both from one place — the duplication of `0.20.2`
between the install line and the assertion is deliberate, and is itself the
check.

### The licence gate's negative control

**Where the control now lives: `tests/licences`,** under "The licence surface,
after the port" above. `tests/fixtures/deny-gpl/` was the Rust control -- a
separate workspace holding one `GPL-3.0-only` crate under a `BSD-2-Clause` root,
driven with the real `deny.toml` so that a control derived from a copy could not
agree with itself -- and it went out with the tree it was built against. The
tags v0.1.2 and v0.1.3 carry it.

THE FINDING THAT PUT IT THERE IS NOT HISTORICAL, which is why this heading stays
rather than being deleted. `cargo deny check licenses` passed from its first run
having examined **no dependency licence at all**: all seven unused allowances
raised `license-not-encountered` and `deny` resolved the one local crate. A gate
that encounters none of its own allowances cannot fail, and a gate never seen to
fail is indistinguishable from one that is not wired up. The Go gate was
reviewed against exactly that and had the same hole -- "24 of 24 allowed" is
what a gate that always says yes also prints -- so it now drives seven synthetic
modules through its own verdict function and asserts the clause that answered.

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
  Python because adding a Python dependency to CI would repeat the
  `actions/checkout` mistake of depending on an unmeasured interpreter on the
  runner. The older half of that reason — that the runner could compile nothing,
  so a gate in the project's own language could not run there — **is retired**:
  the runner builds and tests Go at 1.26.8. So it should become a `rumor`
  subcommand beside `docs --check`, and what stands in the way now is that
  rewriting twelve arms costs the self-test that found five defects in them,
  not the toolchain. Until then nothing *enforces* it — a reviewer has to run
  it.
