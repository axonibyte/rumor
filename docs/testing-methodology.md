# Testing methodology

> Stub from R0.1. The headings below are the ones this document owes; the
> content arrives with the phase that owns the subject. `docs/coverage.toml`
> names this file, and `rumor docs --check` fails if a source path it should
> describe is not mapped.


## The tiers

Not yet written.

## The non-negotiables

Not yet written.

## A portfolio of oracles, not a pyramid

Not yet written.

## Mutation-check every new assertion

An assertion never observed failing is an assertion whose value is unmeasured.
The example packages under `examples/` are built to that rule and the harness
enforces it rather than describing it.

Each package holds a `statute.rego`, a `cases/` directory recording the verdict
every named rule must produce for a given ledger, and a `mutants/` directory of
statutes that differ from it by one clause. `tests/statute/run_test.go` walks
them and requires three things:

- **The clean case green.** Every case's recorded expectation is compared
  against every rule the example owes, read from that package's `rules.json`
  rather than inferred from the statute, so an example that stops defining one
  fails instead of quietly narrowing.
- **Every mutant red.** A mutant must make at least one case *disagree*. One
  that changes no verdict is reported as a survivor naming the clause, because
  the clause it cuts is then untested.
- **A mutant that mutates nothing is refused.** A file identical to the statute
  is rejected naming the cut, not the cases — an edit that stops applying is
  otherwise indistinguishable from a guard nobody tested.

Two properties of the walk matter as much as the assertions. The examples are
**discovered, never listed**, because a listing is a second place to forget one;
and an **empty walk is not a pass** — a missing directory fails loudly rather
than reporting zero problems. The sizes are asserted too: three packages,
seventeen cases, nineteen mutants, each as a number in the test, so a file that
stops being examined cannot hide inside a clean total.

**A mutant that cannot be killed is removed, with the gap recorded.** The
planted-lie package is missing `m2-ignores-knowledge` on purpose: killing it
needs a claim the observer was never told, and the finding for that case is the
`explained` reading not yet settled. It belongs to the transitive-count example,
whose subject is the untold claim, and its absence is written down in that
package's `mutants/README.md` rather than left to be inferred from a shorter
directory.

Each case is also evaluated **with coverage and a trace**, and both are asserted
non-empty against the rule under test. A verdict that agrees with its
expectation while executing none of the statute is a tautology, and the coverage
report is what tells the two apart.

**ADR 0001's prohibition on `print()` inside a statute is enforced, not stated**,
on every statute and every mutant — a mutant is a statute too. That check is the
project's worked example of the next section's rule.

## Self-test the oracle

An oracle is checked before it is trusted, in the same run that trusts it.
`tools/release-gate.py --self-test` drives sixteen cases, fifteen trailer checks
and seven docs-verdict checks, and asserts that **each arm fires alone**. Each
count is declared as a constant and asserted in both directions, because a case
that silently stops running is invisible in a self-consistent total. The
release-gate workflow runs that self-test before the gate judges a pull request,
because a gate whose arms have stopped discriminating would otherwise refuse or
admit a run with nobody the wiser.

**It is not one case per arm, and the gap is where a broken arm hid.** This
section used to say "twelve cases — one per refusal arm plus a clean run", and
that was never true of `docs-stale`: that arm runs the real `rumor docs --check`
against a checked-out ref, which the self-test's synthetic repos cannot provide,
so it reports UNMEASURED there by construction. Eleven arms were driven and the
twelfth was described as if it were.

The arm was also broken for the whole of that time. Its filter — which decides
whether a non-zero exit is *a finding* or *a tool that failed to run* — matched
`not mapped|out of date|unmapped|stale`, and `rumor docs --check` emits none of
those words; its refusals say "matches no entry" and "matches no path". So a real
refusal read as a broken tool and the arm reported UNMEASURED for any tree at
all. Nothing caught it because nothing ran it, and **UNMEASURED reads exactly
like a pass to anyone totalling arms** — which is why this file's own rule is
that the count of unmeasured things is reported out loud rather than folded into
a clean total.

The repair has three parts, and the third is the one that matters here: the
filter widened to the tool's real vocabulary; the vocabulary **duplicated by
hand** rather than imported, because a predicate that read its words from the
thing it judges would agree with it whatever either said; and a battery of seven
real strings — both refusals, the real success line, three broken-tool outputs
and the empty string — so the filter is itself an oracle under test. Restoring
the historical filter fails exactly the two real-refusal cases.

**A workflow step is the hardest thing here to drive, so the logic comes out of
it.** `tag-pipeline.yml`'s verdict on `git verify-tag` was four inline lines of
shell, exercised only by pushing a tag, and it was wrong in two directions at
once for as long as it existed: it missed git's own words for an unconfigured
trust store, so a correctly signed tag read as forged (v0.1.4, run 162); and it
read "no principal" beside a *good* signature as a provisioning gap, which
grants an untrusted signer what it should refuse. That second one was invisible
because the first kept verification from ever happening — fixing one made the
other reachable, which is why both had to land together.

It now lives in `tools/verify-tag-verdict.sh` with eight cases over strings this
project has really seen, and each case asserts **the clause that answered**, by
name, rather than the exit status: three of the eight clauses refuse and two
report provisioning, so a control reading only the status could not tell a
discriminating classifier from one that always refuses.

**Name the half you did not drive.** `tools/forge-release.py` publishes the
release and attaches its three files, and its battery covers exactly two
things: pulling the tag's section out of `CHANGELOG.md` — seven cases, both
heading dialects, a section that is not the first, blank lines inside one, a
missing version and an empty section — and the hand-assembled multipart body,
asserted to open, name its file and close on its own boundary, because a
dropped CRLF pair is the classic silent upload corruption.

Nothing in that battery talks to the forge, and the tool's docstring says so in
those words. The create-and-attach half is proven by a tag run and by nothing
else, which is why the issue's done-when is a published tag rather than a green
local suite. A battery that implied otherwise would be worse than none: it
would read as coverage of the part that can actually fail in production.

Firing *alone* is the property, not merely firing. Two arms that always fire
together cannot be told apart by any fixture, and the first draft of this gate
had two such pairs: an unchanged version also fails "not greater than the latest
tag", and a commit after the bump also leaves the head subject wrong. Both were
found by the self-test before a branch was cut, and both were repaired — one in
the arm, one in the fixture.

## Two oracles for any claim that matters

The `print()` prohibition is the shape of it. Two checks run over the same
modules and neither would do alone:

- **Static**, over the parsed module: it finds a `print` on a path no case
  reaches. The runtime hook cannot see those, and an unexercised `print` is
  still a statute writing to a log.
- **Runtime**, through the same evaluator the cases use, with a hook that
  records every call: it proves the hook is wired and the evaluator is the one
  the verdicts came from. The static scan cannot prove that.

The runtime half carries a trap worth naming, because it is the `docs-stale`
defect in another costume: **OPA erases `print` calls unless print statements
are explicitly enabled**, so a hook installed without that flag reports zero
prints for a statute full of them. The flag's necessity is therefore
*measured* — the same printing module is evaluated with the flag off, and the
test records that the hook came back empty. A precaution whose necessity is
asserted rather than measured is a comment.

Both halves are then driven against a statute that does print, and the walk
itself was driven with a `print` planted in one of the carried mutants: it named
the file and the line. The counts it examined — modules scanned, cases
evaluated — are logged beside the verdict, because zero findings reads
identically whether every module was clean or none was opened.

The licence surface is checked the same way, from two sides. The module graph
comes from the toolchain (`go list -deps -test`), never from a hand-written
list, and each module's licence text is classified independently of it; the
allowlist the classifier judges against is parsed from `deny.toml` and compared
against the copy duplicated in the test, so the two readings have to agree.

## Name what a test does not prove

A result carries its bound or it overstates itself. Three forms this project
holds to:

**A refusal set proves nothing about an exemption.** A gate that refuses
everything satisfies every refusal fixture, so a set of reds shows the gate is
loud rather than that it discriminates. The exemption needs a **positive
control** — an input the gate must pass — and the control must differ from the
nearest refusal fixture by exactly the thing under test. Differing in two things
makes two experiments and settles neither.

**An unmeasured arm is not a covered arm, and must say so.** Where
`tools/release-gate.py` cannot reach a verdict — `docs --check` with no
toolchain, say — it reports UNMEASURED on stderr rather than passing, and the
self-test prints which arms it could not drive. A check that quietly returns
clean when its instrument is missing is indistinguishable from one that passed.

**A fixture has no history, so drive the real thing as well.** Synthetic
fixtures are built to exercise an arm and therefore carry only what the arm
needs. Two defects in this gate were invisible to all twelve fixtures and
surfaced the moment it ran against a real accepted release:

- `git rev-parse <annotated tag>` returns the **tag object's** sha, not the
  commit's, so a comparison against a merge-base failed on a correctly rebased
  run. Every tag here is annotated by rule, so the unpeeled form was wrong for
  every base a real run would use. A fixture repo has no tags at all.
- Re-run after tagging, the tag **for the version being gated** counts as "the
  latest", and a correct run reads as going backwards.

Neither is reachable by building a better fixture. The rule is to drive a real
artifact that is already known-good, and to treat a refusal of it as a finding
about the gate until the gate's own reasoning says otherwise.
