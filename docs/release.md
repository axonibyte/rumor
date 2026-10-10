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
