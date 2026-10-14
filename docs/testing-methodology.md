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

Not yet written.

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

Firing *alone* is the property, not merely firing. Two arms that always fire
together cannot be told apart by any fixture, and the first draft of this gate
had two such pairs: an unchanged version also fails "not greater than the latest
tag", and a commit after the bump also leaves the head subject wrong. Both were
found by the self-test before a branch was cut, and both were repaired — one in
the arm, one in the fixture.

## Two oracles for any claim that matters

Not yet written.

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
