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

**A mutant that cannot apply is not a surviving mutant.** Driving the
flattening module, the first attempt at a transitivity mutant never fired: it
read a map that was still empty at the point it was inserted, so the suite
stayed green and the report said SURVIVED. A dead mutant counted as a survivor
is a reporting error, not a result, and it sends the next hour at the wrong
question. The second attempt — the recipient learning everything the teller
knows — applied, and *that* one survived for real.

**And what it found is the shape worth generalising: the property a module
exists for can be the one its tests cannot see.** `internal/flatten`'s whole
job is that a tell carries a *reference* and not the teller's knowledge, and
every test written for it passed against a flattening that handed the recipient
both. The distinction is invisible in a one-claim ledger, which is what the
obvious tests use, and invisible to a transitive closure over tell *edges*,
because every tell names a ref. It shows only when a teller knows more than it
told. The case now exists, states the leak as an absence with the leaked
reference named, and the mutant dies on it.

**Where a fixture is the oracle, read the fixture.** That module produces the
`known` arrays the example cases already carry, so its first test loads
`examples/transitive-count/cases/charlie-explained.json` and compares against
the array in the file rather than against a ledger written to match the
implementation. The fixture also asserts its own shape — two claims, one known
— so a corpus change is seen rather than absorbed.

**A pinned hash catches an unnoticed edit; it does not forbid a deliberate
one.** Each statute and rules file is pinned by git blob id, computed in the
test rather than asked of git. Five of the six pins were measured at the branch
the examples were carried from; the sixth moved in v0.1.8, when three sentences
in `planted-lie/statute.rego` were found to have gone false — an ADR named by
its retired thread, a harness described in the previous engine's API, and a
phase said to be implemented in the previous language. The test **fired on that
edit before the pin was moved**, which is the property working: the edit was
deliberate, recorded in the same commit, and the alternative was leaving three
false sentences in a live fixture to preserve identity with a branch that has
since been swept.

Moving a pin made three neighbouring claims false — a package comment, a test
comment and a proof line all said the files matched the branch they came from.
They were corrected in the same pass. A changed fact has siblings in the prose
around it, and the prose is where the next reader looks first.

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

### A validator must not rebuild its subject

`tests/ledger` checks the ledger files under `examples/<name>/ledger/` against
the cases they were derived from. The tempting shape is to rebuild each ledger
the way the generator did and compare -- and it is worthless, because it agrees
with itself whatever the generator did. So every arm reads the **committed
file** and checks it against the **case**, which was authored independently and
before the ledger form existed: the file must load through the store's own
guards, the sealed prefix's claims must equal the case's claims inside its
bound, the prefix's tells must flatten to exactly the knowledge that bound
allows, and the committed bytes must be what the store re-dumps so no file has
drifted. Two arms exist only to stop the others passing vacuously: the cases and
the ledgers must be in bijection, so a dropped case cannot go unnoticed, and the
arm about bounds refuses if no case is found whose bound actually excludes
something.

Mutation-checked rather than asserted: changing a claim reference, falsifying a
seal file's recorded bound, reordering a ledger's lines, deleting one ledger
file, and emptying the recorded knowledge each turn the suite red.

## Self-test the oracle

An oracle is checked before it is trusted, in the same run that trusts it.
`rumor validate` declares **twenty-three** arms, and the count is pinned in FIVE
places that move together -- `internal/validate.TotalArms`, that package's own
`allArmIDs` (which `unreached` walks), `tests/validate`'s `allArms`, `ci.yml`'s
literal and this sentence -- the way the release gate's own case count is. It
was three until R6.2 found the other two the hard way: `allArmIDs` is
unexported, so no external suite could read it and nothing did, and `ci.yml`'s
is a grep in YAML that no Go test could see. Both have oracles now
(`TestAllArmIDsIsTotalArms`, in-package, and `TestTheWorkflowsArmCountIsTotalArms`),
because a copy with no oracle is found by whoever bumps next. The invariant
is an ACCOUNTING one rather than a per-project number (fable, coop r4/77): every
run accounts for all twenty-three as evaluated or skipped-with-its-reason, never
silently and never twice, and across `tests/validate/fixtures/` every arm is
evaluated at least once. The permitted twin evaluates all twenty-three, which
is what fable's D9 drill requires of it; a project declaring no browser stage
honestly reports **twenty-one**, and one whose adapter cannot be reached
reports **nineteen** -- the skips named, and could-not-run rather than refused.
Each of those three figures is measured here rather than computed, at R7.2 as at
R6.2 as at R4.3: arm 19 evaluates on an unreachable adapter because eight of its
nine cells read documents alone, so that reading moved by one and not by two;
the three arms R6.2 added read the bindings alone, so they evaluate in all three
conditions and every figure moved by exactly three; and R7.2's arm 20 reads the
bindings alone for the same reason -- both `[deadline]` and `[clock]` are
presence in the same file -- so every figure moved by exactly one again,
measured at each count rather than carried forward: 23 of 23 on the twin and on
the toy, 21 of 23 with the browser stage off, 19 of 23 with no adapter on
PATH.

`tools/release-gate.py --self-test` drives twenty cases, fifteen trailer checks
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

## A structural bound can make a mutant equivalent

R2.3 drives the three example packages twice: once from the flattened cases,
once from R2.1's ledger files through `internal/engine`. Sixteen of the
nineteen mutants die on both paths and to the *same* cases. The three
`*-ignores-position` mutants die only on the case path, and that is not a
weaker harness — it is what the structural bound means.

Those mutants remove `c.position <= input.observation.position`. A case carries
claims its own bound excludes, so dropping the guard admits them and the counts
move. A **sealed prefix** holds in-bound facts only, because the bound *is* the
seal: there is nothing left for the guard to exclude, so the mutant computes
exactly what the clean statute computes. A statute's position guard is dead in
production, and the mutant that exists to prove it matters cannot die there.

The rule this gives, which generalises past Rego: **when a defect is designed
out of the data, the mutant for the check against it goes equivalent, and
equivalence has to be proven rather than assumed.** `tests/engine` requires,
for each declared mutant, identical values for every rule on every case against
the clean statute *on the same path*, and requires the other path to still kill
it — so the clause is shown to matter and the bound, not the harness, is shown
to be what put it out of reach. A mutant neither killed nor provably equivalent
fails; one declared equivalent that starts dying fails; and the declared set
must equal the measured set in both directions, so a fourth going equivalent is
a finding and a stale declaration cannot hide one.

## A mutant that did not compile is not a kill

Disabling a guard by replacing its condition with `false` can leave the
variable it bound unused, and Go then refuses to build. A mutation harness that
reads "the suite did not report ok" as a kill counts that as a dead mutant,
which is the same error as treating a red build as a caught defect. R2.3's
mutation runs check the build separately and report `DID NOT COMPILE, so it
proves nothing`, then re-express the mutant so it compiles — `false && cond`
rather than `false`.

## A refusal that can never be the clause that answered is not a check

Two refusals in R2.3's `pending` path looked independent: no clock fact at the
seal, and no clock fact at or before the eventual type's last claim. Disabling
the first killed no test. The reason is structural rather than a missing case:
the clock that supplies the claim's time sits at or before the claim, the claim
is inside the prefix, so the seal sees that clock too. The first refusal could
fire first but could never be the only answer, and the test could not tell them
apart because both messages contained the same words.

So they became **one clause naming both facts** — `present=true/false` for each
— with the test reaching both halves: no clock at all, and a clock sitting only
*after* the claim. The lesson is the cron's rule stated from the other side:
read a control's verdict off the clause that answered, and when two clauses can
never answer separately, they are one clause and should say so.

## An unreachable guard is not a cautious one

`internal/engine` carried `if known == nil { known = []string{} }` with a
comment explaining why an empty knowledge list must not reach a statute as
`null`. No mutation could kill it: `flatten.Known` builds its result with
`make()`, so the empty case already arrives as `[]`. The guard was unreachable,
not merely unexercised, and deleting it is the honest outcome — a guard nothing
can reach is a claim about the code that the code does not support. The
property it was protecting is asserted where it can be: the harness compares
the engine's input against the case's field by field.

The same pass kept a redundancy that is *not* dead: the seal is still passed to
`flatten.Known` even though the prefix handed to it is already bounded by the
seal. Raising that argument kills nothing, so the mutant is equivalent. Rather
than leave that to a passing suite, the premise it rests on — that nothing
handed to the flattener sits past the seal — is asserted directly over all 17
fixtures, and the arm fails if none of them holds a fact past its seal.

## Where a fixture is the oracle, read the fixture first

R2.3's first attempt asserted that the engine's flattened knowledge equalled
the case's `known` list outright. It does not, for two of the three
`position-bounded` cases: a case deliberately carries knowledge its own bound
excludes, because the statute narrows it with a position guard, where a ledger
narrows it structurally. Asserting the unfiltered equality was asserting that
the bound does not work.

The comparison is now against the case's own claims filtered to the case's
bound — computed from the case, not read from the `.seal.json`, which records
the same relation and would make the arm agree with a file instead of with a
case.

## A comment that quotes a document is a claim about that document

R2.3 shipped two quotations of `docs/vocabulary.md` that were accurate when
written and false thirty minutes later — **in the same tag**. The amendment
commit reworded both entries, four commits after the file that quoted them, and
one of those comments also reported a discrepancy as open and sent the reader to
a thread for a decision the same tag had already made. Nothing noticed, because
no test read those strings. quill found it by eye (rumor/577), as the pivot's
`"in Rust"` and the stale ADR citation were found by eye before it.

`tests/vocabulary` is the check. Three things about building it are the
transferable part:

**Attribution has to be narrow, and the width is a measurement not a taste.**
The first version checked every quoted span in every comment. Measured against
the tree, that produced **seven false positives for one real finding** —
comments quote the epic, issue 44 and their own emphasis in the same breath. An
oracle that cries wolf seven times is one nobody reads, which is a worse outcome
than no oracle. Narrowing to quotes adjacent to a vocabulary citation found the
real defect with nothing spurious; widening it slightly to the prose citation
forms (`the vocabulary`, `v1`) caught the second real defect too.

**Normalise what the medium changes, not what the author wrote.** Three
normalisations each earned their place from a real quote here: whitespace,
because quotations and the document both wrap and a line-oriented matcher
reports ABSENT for text that is present; markdown emphasis, because the document
writes a position as `*p*` and the quoting comment writes `p`; and case, because
a sentence quoted mid-prose loses its capital and one file emphasises a clause
in capitals. Without these the oracle reports three defects that are not there.

**Give the oracle no exception for historical quotes.** The first fix for the
stale quote *quoted the superseded sentence* while explaining it — which the
oracle must flag, since that is exactly the shape it hunts. The resolution is
not an exemption but a rule for comments: **a comment that needs to discuss an
old wording describes it rather than quoting it.** An exception is the hole a
stale quote hides in.

And the line-break trap caught three readers in a row — quill's first sweep, the
first version of this oracle, and a `grep -o` I ran to check one of its findings.
A quotation that wraps is one quotation; any instrument that reads lines will
disagree.

## A pin catches an edit; it cannot catch a birth defect

`tests/statute` pins the three statutes and their `rules.json` by blob id, and
that pin has done its job — it fired on the one deliberate statute correction
before the hash was moved. It would have caught **none** of quill's Q2
(rumor/584): three stale sentences sitting in all six `planted-lie` mutants.

Two reasons, and both generalise:

- **The mutants are not pinned**, and pinning them would be the wrong fix. A
  mutant is edited deliberately whenever a clause changes; a pin there would be
  a hash to update rather than a property to hold.
- **The sentences were false the day the mutants were cut.** They were copied
  from the statute's header before the statute was corrected. There was never a
  moment when the file changed, so there is no drift for a pin to see.

The guard that fits the defect is a **sweep with a denominator and a positive
control**: no fixture under `examples/` names a retired mechanism, over all 77
files, with a word every package must contain asserted so that an unreadable
path cannot report a clean corpus by reading nothing. That is ratchet's rule at
`r1.vocab/8` — *found by sweeping rather than fixing the instance* — pointed at
the fixtures instead of the vocabulary.

The shape to notice is that **the incomplete sweep was recorded as reassurance**.
wren's R2 gate verified per blob that exactly one file of 43 had changed, which
is simultaneously the proof the statutes' semantics held and the trace of the six
files nobody touched. A number can be correct and still be the fingerprint of
what was missed.

## A mutant set proves rules are attacked; it does not prove each rule is

`tests/engine` asks, of every mutant, whether some case catches it. That is the
right question for a mutant and it is satisfied by a mutant set that batters one
rule and never touches the rest — every mutant dies, every rule but one is
unproven, and the suite is green.

`rumor attaint` (R3.1) asks the dual, per RULE: is this rule moved by at least
one mutant, on at least one case? A rule nothing moves is NAMED rather than
counted, because an unmutated rule is a rule nothing shows is load-bearing.

Both paths count. R2.3 established that three mutants are equivalent on the
ledger path — the sealed prefix leaves their position guard nothing to exclude.
Measuring only the ledger would report the rules those three attack as
unmutated, which blames the mutant set for a property of the bound. The output
names the path, so a rule killed only from the cases is visible as one.

## A statute can be weakened INTO one of its own mutants

The per-rule question above has a blind spot, and a drill found it rather than a
review: loosen `planted-lie`'s honest count so a planted claim counts as honest,
and nothing notices. The finding is still `bug`, the corroboration control still
passes, every rule is still moved by some other mutant, no finding became
unreachable. The statute is weaker and every arm is green.

What catches it is the other half of the mutation pass — not "is every rule
attacked by some mutant" but **does every mutant still attack**. The weakening
is exactly the shape of the committed mutant `m5-honest-counts-planted`, so
after it the mutant computes what the statute computes and moves nothing. A
mutant that moves no rule on any case on either path has not become harmless:
the statute has drifted into it.

This is distinct from a declared equivalence. The three position mutants are
equivalent on the ledger path only and still move their rules from the cases, so
they move something and are not dead.

## A fixture that damages a tree must replace, never add

The drills under `tests/attaint/drills/` are each the pristine `examples/` tree
with one file replaced, and each drill directory holds only the file it changes
— so the directory is the diff, readable without diffing two copies of a
77-file tree.

The hazard is a path that drifts. An overlay written to
`examples/planted-lie/ledger/with-planted.json` when the file is `.jsonl` lands
*beside* the original, leaves the tree undamaged, and the drill then passes
while testing nothing — a green that means the opposite of what it appears to.
So the harness counts the files before and after laying the overlay down and
refuses if the count moved.

And each drill asserts the refusal COUNT as well as the clause. A drill that
trips a second arm is not the drill that was designed, even when it still goes
red.

## Prove a finding was its cause by removing the cause

A fixture that plants a lie and observes a `bug` has shown the two coincide. It
has not shown the bug came from the lie: a statute that reports `bug` for an
unrelated reason passes that fixture exactly as a correct one does, and every
other arm with it — the finding is right, the control attributes it, the rules
are all attacked.

The missing arm is the counterfactual. Remove the cause and the finding must
go. `rumor attaint` rebuilds each sealed prefix without its planted claims and
requires `consistent`; a bug that outlives the lie was never the lie.

Two details the store settled rather than the design:

- **Dependents go with what they depend on.** A tell passing a removed claim
  cannot stay — the ledger refuses it, and a refusal there would read as this
  arm's finding rather than as the counterfactual being unbuildable.
- **The bound travels with the facts.** `SealAt` refuses a position the ledger
  no longer holds, so removing the claim *at* the seal cannot keep the old
  seal. Sealing at the highest survivor loses nothing: every survivor is at or
  below it. v1 already says a replay assigns its own positions and compares
  findings, never positions.

**And the counterfactual is derived, not committed.** A new fixture would have
moved R1's gated counts, and a derived one checks every qualifying case rather
than one example that could be written to pass.

## Every refusing arm needs a permitted twin, and the pairing should be derived

ratchet's rule from R2.1: *a refusal arm that never sees the accepted case
cannot tell "refuses correctly" from "refuses always".*

Coverage and discrimination are separate claims and fail separately. "Every arm
fires somewhere" is satisfied by an arm hard-wired to refuse; "every arm can
decline" is what catches it. Two tests, not two columns of one.

**Derive the pairing.** Most arms refuse something *absent* from a healthy
subject, so the healthy subject is their twin and no list can drift out of date;
only the arms whose refusal is an *addition* need a twin of their own, named
with its reason. A hand-maintained pairing is the defect one layer up from the
thing it is checking.

## Ask what a check cannot see, before calling it done

Five instruments in this project failed the same way in one evening, each
correct about its own contents and blind to the next arrival:

| instrument | what it could not see |
|---|---|
| a lint reporting "3 issues" | a fourth, capped by `max-same-issues` |
| a test asserting a list of clauses | a clause added after the list was written |
| a doc block grepping the lines it quotes | a new *kind* of line |
| that block's own total, a literal | the prose figure it was protecting going stale |
| a floor of `6` on a 7-line list | the deletion it was added to refuse |

Each fix was correct and each was the next one's blind spot, because that is
what a fix *is* when the defect is "this check cannot see X" — closing one view
opens the next. Adding a denominator three times did not stop it; asking the
question once would have.

**The remedy is a source that moves.** A remembered number goes stale silently;
a number counted from the artifact moves when the artifact does. Where two
artifacts can disagree, read one from the other so they convict each other —
the sample's floor counted from the page's own table fails both when a kind is
added without a sample and when a sample is deleted without the table.

## A documented sample that cannot fail is decoration

`release-gate.py` executes fenced blocks marked `run` and fails on a nonzero
exit — but it compares nothing to the page, so *marking* a sample `run` proves
only that the command exits 0. The gap is output comparison, and a sample can
rot for a whole release while every path-coverage check stays green, because a
coverage check cannot see a quoted figure.

A sample that asserts itself closes it with no new checker. Drive both
directions before trusting one: it must pass as committed **and** fail on a
drifted figure, a shrunken quote list, and a reworded sentence it reads from.

## An assertion pair both halves of a mutant satisfy is one assertion

A test that asserts a wrong rendering is ABSENT and a right one is PRESENT
looks two-sided and is not. `internal/examples`' first absence arm required the
output to contain no `<nil>` and to contain `no finding` — and a renderer
answering `no finding` for EVERY case satisfies both, which is a worse defect
than the one the arm was added for. It was found by a reviewer running that
mutant rather than by reading the diff: the engine's twin arm failed it and the
walk's passed.

The repair is an assertion on a real value in the same run, and the fixture
usually already supplies one: removing `default finding` from a copied package
leaves its other rules standing, so defined findings are printed beside the
absent one and no second fixture is needed. The general form — if a mutant
exists that satisfies every assertion in the arm, the arm has one side, however
many `if`s it contains.

## A tightening needs a control from what the system already accepts

An added refusal is easy to drive in the direction it was written for and easy
to leave undriven in the other. Both of release-gate arm 9's first two fixes
were measured only against the run that prompted them — a head already refused
for other reasons, where nothing legitimate could be lost — and both refused
the project's own history: keying the `Docs-Unchanged` exemption by commit made
the gate reject v0.1.12 on seven paths, v0.1.13 on three and the then-queued
v0.1.14 on one, because three of the four trailers in `v0.1.10..main` sit on a
bump that touched only `CHANGELOG.md` and `VERSION`.

The control is cheap and it is not the same as the fixture: run the tightened
check against **inputs the system has already accepted** — each release with
its own tool as the baseline — and require them to stay admitted. A fixture
proves the new refusal fires; only an accepted input proves nothing else does.
`--self-test` therefore carries the ordinary shape of a run in this project —
a feature commit with the reason on the bump — beside the pair that pins the
new boundary, because the pair alone passed every version of the rule that was
wrong.

The general form: a check that only ever gained refusals has been measured in
one direction. Ask what it now rejects that it used to accept, and measure that
against history rather than against a fixture written by the same hand.

## An executable doc block is code, and inherits none of the code's discipline

A fence marked `run` is executed by the release gate, which makes it the only
code in the tree that no suite reads. `docs/cli.md`'s sample invoked the
toolchain as a bare `go` and behaved three ways on three boxes: absent in one
jail, go1.27.2 in another, go1.26.9 on the runner. The workflows had already
settled this — resolve `go go126 go125`, then `export GOTOOLCHAIN=local`, at
eight sites in `ci.yml` and two in `tag-pipeline.yml` — and the doc block
simply never inherited it, because nothing checked that it should.

**Two distinct defects, and the first is the one that costs an hour.** A
missing interpreter surfaced on the `doc-run-block-failed` arm, so the gate
reported a DOCS defect for a box with no Go on it. A block that resolves its
own tool can say `PROVISIONING finding and not a code one` instead, and the
gate quotes a failing block's last line, so the right words reach the reader of
the finding rather than the author of the block. The second defect is silent
substitution: under `GOTOOLCHAIN=auto` a `go` directive above the installed
toolchain is satisfied by *downloading* another one, so a green run is not
evidence about the toolchain that ran it.

**Pin the extraction to the executor, or the suite checks a different set.** A
test over `run` blocks must find exactly the blocks the gate runs, so read the
gate's fence pattern and refuse to run if it has moved. Pair it with a
known-positive count: every check over an empty set passes, and a fence form
that quietly stops matching is how that set empties.

## Two copies of one schema need a check that reads both

R4.1 wrote the adapter's field tables twice: in `docs/contract.md`, where a
reader meets them, and in `internal/wire`'s schema, where the decoder refuses an
unknown or missing field by name. Both were written from one draft in one hour,
and nothing but that coincidence required them to stay equal. This tree has
already paid for the same shape twice — a comment quoting a vocabulary sentence
that was reworded four commits later in the same tag, and the release gate's
contract-version arm matching a Rust filename for three weeks after the Go port,
green in its own self-test throughout because the fixture was Rust too.

So `tests/contract` reads the document's tables and the package's schema and
requires one answer. Three things made it a check rather than a gesture:

- **The envelope is read from the document too**, not assumed. It is the set
  every per-message comparison subtracts, so a wrong value there would shift all
  nine comparisons at once in the same direction — and a check whose baseline
  moves with its subject agrees with itself.
- **The vacuity guard is derived from the protocol**, not remembered: the number
  of comparisons must equal the kinds in both directions. A parser that found no
  sections would otherwise report a clean agreement, which is the failure mode
  of every document scraper.
- **It was driven against a mutated document**, five ways: a field row deleted,
  a row added for a field nothing speaks, the quoted version value reworded, a
  heading renamed, and one envelope field misspelled. All five fire. The first
  run of the test itself failed two of those for its own reasons — it read the
  error envelope's second table as fields, and read the response's table as the
  request's where the request says "nothing" — which is the argument for driving
  the parser against a broken document before trusting a green one.

What it does not prove, stated because the shape invites over-reading: it
compares field **sets**, not types, and not the required/optional split.
`observe`'s rule that a token is present exactly when the source is not `direct`
is prose in the document and an arm in the decoder, driven in the package's own
tests; the agreement check would pass if the document called that field
required.

The general form: when a document and a program state the same schema, pick one
to be the source a test reads and have it convict the other. Prose that merely
says "keep these in step" is not a mechanism, and the copy that drifts is always
the one nobody runs.

## A parser must not refuse what a validator is built to refuse

`rumor validate`'s nineteen refusals (R4.3) and the bindings parser (R4.2) read
the same file, and eight of those arms are about bindings that are **well formed
and wrong**: an action with no precondition, a binding emitting a claim type no
rule consumes, an eventual claim type with no bound, a browser-stage action with
no UI flow, a declared fault with no capability behind it. Every one of those is
a file a strict parser would be tempted to refuse — and a parser that refused it
would make the arm **unreachable**. The run would stop at could-not-run, the arm
would never evaluate, and its fixture would be measuring the parser.

Nothing about that failure is visible in a green suite. The parser's tests pass,
because refusing a malformed file is what they ask for; the arm's fixture passes
too, because it trips *something*. Only the exit status tells them apart, and a
fixture asserting "this file is refused" does not read it.

So the division is written down in the package comment and **tested as its
own property**, with the battery inverted: one case per arm, each mutating the
file the way that arm's fixture will, asserting that it still **parses**. The
parser then requires only what makes the file readable as bindings at all — a
name, a context, a source, a timing — and `emits`, `requires`, `flow`, `bound`
and `[uses]` are optional here and refused there.

Two refusals in the same file show the boundary rather than blurring it: a
`direct` observation must name a component, because that field is the only place
a binding names one and validate's rollback-coverage arm has nothing to compare
without it — while a *named component no provider covers* is the arm. The parse
refusal is about the file's readability; the arm is about the project's
consistency.

The general form: when two checks read one input, the stricter one upstream can
silently disarm the one downstream. Name which check owns each refusal, and test
the upstream check for **acceptance** of exactly what the downstream one
refuses.

## An existence check is not an accuracy check

Until v0.1.19 this project asserted that `go.mod` **existed** and never that it
was **right**. The whole of it was one line in `ci.yml`'s fetch step —

    test -f go.mod || { echo "no go.mod: wrong tree"; exit 1; }

— which is a wrong-tree guard: it reads none of the file's content, and
`tag-pipeline.yml` names the file only in a comment. So an unused `require`, a
missing one, or a `go.sum` short an entry could travel all the way to a tag with
nothing in the repository to say so, through every release including v0.1.18.

**The two are easy to confuse because the cheap check is the one that passes.**
A path that exists, a file that parses, a binary that runs: each is a real
precondition, and each reads like a verdict on the thing it is a precondition
for. The question to ask of any guard is which of the two it answers, and
whether anything answers the other.

`ci.yml`'s manifest arm answers accuracy with `go mod tidy -diff`, and three
properties of it are the methodology rather than the plumbing.

**The diff is on stdout and the excuse is on stderr.** `go mod tidy -diff`
exits non-zero BOTH for "these files are not what tidying would write" and for
"I could not read the module graph" — and tidying needs the network, because it
resolves the test-only dependencies of dependencies: 32 modules on a cold cache,
measured. A step reading only the exit code would blame the manifest for an
unreachable proxy, which is the misattribution `v0.1.18` fixed in the doc blocks
(a jail with no Go read as documentation drift) and the one run 192 made visible
in the vulnerability step (nine CVEs reported as a database outage). So the two
streams are captured into two files and the answer is read from which one has
content.

**The reading is a script with its own battery, because a step cannot be
driven.** `tools/gomod-accuracy-verdict.sh` classifies into four answers — tidy,
inaccurate, step defect, blocked — over nine named clauses, and `ci.yml`'s
verdict-scripts step runs its self-test on the runner and greps its declared
count: all 9 clauses over 10 cases. That is the same arrangement, and the same
reason, as `tools/govulncheck-verdict.sh` and `tools/verify-tag-verdict.sh`.
Two of the nine clauses exist only to refuse a contradiction rather than to act
on one: a clean exit beside a diff means the tool's documented contract and our
reading of it disagree, and acting on either would be acting on a contract we
have just been shown we do not understand.

**A pass over an empty graph is not a pass.** `go mod tidy` exits 0 on a module
with no requirements at all, so the arm's known positive is that there was
something to be accurate about — an empty or absent `go.sum` is `BLOCKED`, not
green. This is the vacuity guard `rumor validate` carries for a project that
declares no rule, applied at the one place in this arm that can be vacuous.

**And the wiring is driven separately from the reading.** The battery covers
what the clauses say; nothing in it covers whether the step captures the two
streams into the right files, passes them in the right order with the real exit
status, and propagates the verdict's code.
`tools/drive-gomod-accuracy.py` extracts the step from `ci.yml` — by its
`- name:` line, not from a copy — and runs it over four trees: the tree as
committed, one with an unused require, one with `GOPROXY=off` over an empty
module cache, and one with no requirements at all. The third is the condition
the arm is shaped around, and it is the one a reading of the exit code alone
gets wrong.

## A check that proves one half can pass over a corpse

R5.2's rollback provider has to answer whether the world is the checkpoint, and
the obvious check is the one that fails this test: compare the state file,
byte for byte, against the saved copy. It is exact, it is cheap, and it is
**most perfectly satisfied when the application is dead** — a process that is
not running writes nothing, so the file stays flawless. A shrink step would then
be told it had a verified clean world, and would run its candidate against
nothing at all.

So `Verify` asks two questions: the file **is** the checkpoint, and the server
**answers**. The second is not a liveness nicety; it is what makes the first
mean a world a candidate can run in. The test that pins this kills the process
and leaves the file untouched — the one state where a one-sided check passes and
the world is useless — and requires `ErrDirty`.

**The general form.** When a check is a conjunction, ask which conjunct is
satisfied by the subject's ABSENCE. A path-coverage check passes over an empty
tree; a count of refusals passes over nothing examined; a byte comparison passes
over a corpse. Those are the halves that need a second question beside them, and
the test for them is the state where the cheap half is at its best and the thing
being checked does not exist.

It is the same fault as a vacuous arm, seen from the other side: a vacuous arm
has nothing to judge, and a one-sided verify has something to judge and judges
the wrong half of it.

## Two verdicts that cannot be told apart need both halves in one arm

`rumor validate /a/mistyped/path` used to answer `REFUSED no-bindings`, exit 1,
the project is wrong. An existing project genuinely missing its `bindings.toml`
answered the same clause, with the same exit, and the two runs differed only in
the basename printed on the tally line. Neither output was self-evidently
wrong — each is a sentence the tool is entitled to say — and that is what kept
the defect alive: the finding is not in either verdict but in the fact that
**one input's answer is also the other's**.

An arm written for the first half alone does not hold the fix. "An absent path
is could-not-run" is satisfied by making *both* could-not-run, which erases a
distinction the project needs: a directory that was read and found wanting is
the project's verdict, exit 1, and fable ruled that boundary at coop r4/79. So
the arm asserts the pair — absent path and a path that is a file are 2 and name
the path; an empty directory is still 1 and names the clause — and a fix that
collapses them fails the third case.

**The general form.** When two inputs that deserve different verdicts produce
the same one, the test is one arm over all of them, not one arm each. One arm
each can be satisfied by moving the collision rather than removing it, and the
move looks like progress: each half, read alone, now says the right thing about
its own input.

**The sibling this found.** The same question asked one level in — which
verdicts here are claims about something nothing read? — turned up a vacuity
refusal beside a decode failure: an unparsable `rules.json` reported
`could not run: rules.json: invalid character` AND `REFUSED no-rule-declared:
rules.json declares no rule`. The status was right, because could-not-run
outranks refused, so a test reading only the exit code would have passed over
it. A refusal naming a declaration nobody read is wrong whatever the status
above it says.

## R5

The lessons from R5's children, under one heading per phase rather than
appended at the end of the file (fable, coop r4/137). Three sections were
already on their branches when that was ruled and landed as they were; this
child is the first to use the structure, so it also moves those three here, in
the order they landed. A child inserting a lesson adds a `###` under its
phase's heading in landing order -- never at EOF, where two branches collide
by construction.

### A removal finds a missing arm; only a boundary mutant finds a disarmed one

Two sweeps over the same battery (`cmd/toy`, `internal/rollback/toyrestart`)
answered different questions, and the cheap one answered the easy question.

**Removing a mechanism** — the planted capacity race, the planted count leak,
the planted stale count, the admin-only check, the capacity check, both
visibility checks, `Verify`'s server half, the empty-checkpoint branch — killed
its named arm in all nine cases. That is worth running, and it proves only that
every mechanism has *an* arm somewhere near it: a removal changes behavior at
every input that reaches the code at all, so almost any arm on the path reports
it.

**Making a mechanism subtly wrong** found three holes the removals could not
see:

- `createEvent` defaults an absent `visible_to` to `{admin, member}`. Every arm
  passed the field explicitly, so the default branch was exercised by nothing:
  changing it to `{admin, member, guest}` left the whole package green — the
  epic's visibility-leak row arriving by accident, on the one axis this system
  exists to be wrong about.
- `Verify` comparing LENGTHS instead of bytes left every test in its package
  passing, because the only moved world any arm built had an extra user in it.
  The arm that pins the comparison was true for a reason unrelated to what it
  asserts.
- `Verify` reading an absent state file as the checkpoint itself — rather than
  as an empty world — was also invisible.

**The general form, which is the same law twice.** An arm can only see a defect
its INPUT reaches. A removal is reached by every input; a boundary is reached
only by an input sitting on it, and a default is reached only by an input that
does not override it. So: for every value a test may override, one arm must
assert the default with nothing set; and for every comparison, one arm must
differ in the way the cheap implementation would miss.

**A fixture property an arm's reach depends on is the arm's own
precondition.** The concurrency arm drives two joins at a one-seat event and
asserts that both land. Nothing asserted the *one seat*: with the fixture's
capacity raised to 2, both joins land honestly, every assertion passes, and the
arm reports a race that had been closed — while its own message still says
"one-seat event". The repair is not a better assertion about the result but an
assertion about the input: more joins in flight than seats, read from the create
response because the toy has no route that returns an event. The four cells then
read `seats < claimants` with the race open → pass; with it closed → fail in the
draw; and either capacity-2 cell → fail at the precondition, which is correct
because the arm is disarmed. fable made this the gate's reading of every R5
battery at coop r5/16.

**And the ordering inside an arm decides what it can report.** The default arm
above checks the echoed field and then the two endpoints that read it. Written
with `t.Fatalf` on the field, the arm would rest entirely on a string — the
failure rook found one layer down, where renaming a fixture key disarmed a
boundary arm and nothing screamed. With `t.Errorf` it reports all three, and
the mutant that only the field catches (dropping `member` from the default,
which no endpoint can distinguish, because a defaulted event can never gain a
member) is visibly the one with a single witness.

### A list-shaped reference needs its defect past element one

Four of arm 19's eight cells walk a list — an action's `requires`, an
observation's `args`, a flow step's `{handle}` placeholders, and the bindings'
observations — and each fixture broke the *only* entry there was. Replacing each
loop with one that reads `x[0]` alone then passed every fixture built for it:
the cheapest wrong implementation of those four cells was invisible to the
battery written to hold them. Each fixture now carries a declared name first and
the dangling one after it, and all four mutants fail naming the fixture that
stopped being refused.

**Position matters more than it looks.** With the defect LAST, a cell that reads
only the first element and a cell that stops before the last are both caught;
with it FIRST, only a cell that skips the first is. One fixture cannot kill all
three shortcuts, so the defect goes last and the surviving one is written down
here rather than imagined away.

**And a mutant can be killed for the wrong reason.** The first version of this
sweep used `x[:1]`, which PANICS on an empty slice — the twin's observation
declares no `args` at all — so three mutants came back "killed" by a crash with
nothing to do with reading past element one. A panic-safe mutant
(`x[:min(1, len(x))]`) reported the truth: all three had survived. Read HOW a
mutant died, not only that it did.

It is the same law as the concurrency arm's one seat, one layer down: an arm can
only see a defect its INPUT reaches, and a list of length one reaches no
iteration bug at all.

### A forbid-only assertion cannot see a disappearance

R5.1's applicability arm asked one question of the action set it was handed:
does every action offered have its preconditions among the verdicts the case
records as true. rook renamed `may_leave` in `statute.rego` and `rules.json`,
left `bindings.toml`'s `requires = ["may_leave"]` alone, and `leave_event`
dropped out of every walk — the rule was no longer declared, its verdict came
back absent, and absent is not applicable. The arm passed, and its own `t.Logf`
printed the shrunken set as the proof.

Nothing the assertion claimed was false; every action offered really was
permitted. The fault is its direction. "Every element of the answer satisfies
P" is preserved by **deletion**: it holds for the right answer, for every
subset of it, and most comfortably for the empty one. An arm of that shape can
catch an answer that grew and nothing else, which is the wrong half for any
check whose subject is a set something downstream will draw from.

**The non-emptiness guard is the tell, not the fix.** The arm already carried
`if len(got) == 0`, written because the empty case was the visible one. That
turns "passes over nothing" into "passes over anything non-empty" and reads as
though the hole were closed. Zero is the only case it excluded, and one element
short of the truth is the defect that matters.

**The general form.** Where what a check protects is *which* elements are
present, the assertion must be an equality against a set derived from a source
the subject cannot influence — here `honest-count`'s own `expect` block, read
as a document rather than evaluated a second time, because a second evaluation
would agree with the code by construction. Compare it as a sequence, which pins
the declared order the function promises in the same breath. And the equality
needs its own reach: with fewer than two permitted elements it cannot see a
drop either, so the arm asserts that count before trusting itself.

Three layers were innocent alone — a static validator with no arm pairing
`requires` against the declared rules, a walk layer that cannot tell a missing
name from a declining rule because it is never given the rule set, and this
arm. The hole was between them, which is the usual address.

### A survivor can indict the oracle rather than the battery

Two mutants survived R5.4's first sweep, and neither wanted a new arm. "The
token is never attached to the execute" survived because `internal/fake` ignores
an execute's token entirely, so nothing in the tree could observe the engine's
central job in that child. "The handle choice takes the first instead of
drawing" survived because a handle a step CONSUMED appeared nowhere in the
record -- only the handles it created did. Both properties were implemented,
both were untestable, and the suite was green either way.

The diagnosis to reach for first is "my arms are too weak". These were not: no
arm over the existing surface could have killed either mutant, because the
observable did not exist. The fix was two observables -- an adapter that records
each request before answering it, and a field on the step for what it consumed
-- after which both mutants died at named assertions. So when a survivor looks
impossible, ask what in the system could have SEEN the difference, and if the
answer is nothing, that is the finding.

The recording adapter is worth keeping as a worked example, because three
separate measurements were needed before it recorded anything, and each one made
the arms fail on their own no-subject guard rather than on the property:

- `cat > file` block-buffers when its output is a regular file, so the requests
  sat in the buffer and were lost when the adapter was killed.
- POSIX has a non-interactive shell redirect an ASYNCHRONOUS list's standard
  input to `/dev/null`, so a BACKGROUNDED reader saw EOF at once and never
  created the capture file at all.
- Emitting the canned responses up front leaves the last request on the wrong
  side of the kill: the engine wrote the execute, the walk returned, and the
  process was killed before the reader was scheduled.

Answering per request fixes the third by construction rather than by timing: the
engine cannot have read a response without the loop having already appended the
request that earned it. A capture that is complete *by construction* is worth
more than one that is complete *in practice*, and the difference only shows up
under a kill.

The guard is what made all three findable. Every arm here asserts it has a
subject -- the capture holds a request, some step consumed a handle, two
candidates existed at some consuming step -- so an instrument that recorded
nothing failed loudly instead of passing on an empty set.

### A promoted fixture needs a runner, or it is a comment

`examples/toy/seeds.lock` carries this project's first promoted seed: "a seed
that produced a defect is promoted into `seeds.lock` with a comment naming the
defect" (`docs/design.md`), which is how a project's found bugs become its
regression suite. Nothing runs that file. R22 is the phase that teaches a run to
take promoted seeds FIRST, so between the promotion and R22 the file is prose
with a number in it -- and a promotion that quietly stopped reproducing would
read exactly like one that still does.

**The arm is in `tests/run` and not in the phase that will consume the file**,
because the gap is now and R22 is later. It reads every seed in `seeds.lock`,
runs each twice through the toy's own adapter, and requires three things: the
run exits on a finding, the two runs agree byte for byte, and the finding is
attributed to honest claims.

**Each of the three fails differently, and that is the point.** A seed that
walks clean means the promotion has lost its defect -- the system under test
repaired by accident, which `cmd/toy`'s own suite exists to prevent and which
this catches from the outside, where a reader would otherwise see only a shorter
trace. Two runs disagreeing means the seed is not the reproducer it was promoted
as. And an attribution that moved to `rests_on_planted true` means the finding
became the engine catching a planted lie rather than the application being
wrong: a different finding under the same name, since "two findings are the same
when the rule that fired, the finding's value, its attribution and its kind all
agree" -- four fields, corrected here with `docs/design.md` after coop
rumor/1225 found this quotation and the renderer disagreeing at three.

**The file is also the arm's own vacuity guard.** A `seeds.lock` with no seed
makes the arm fail rather than pass over an empty set, because "there are no
promotions" and "every promotion still reproduces" are otherwise the same green.
A deliberate decision to carry no promotion has to change the arm, not satisfy
it by subtraction.

**What makes the promotion worth holding, measured 2026-10-24:** 11 of 24 toy
seeds find a bug and 13 walk clean. A promoted seed is therefore selecting
something. Had every seed found one, the file would have recorded a property of
the toy rather than a reproducer, and the arm would have been true for a reason
unrelated to what it asserts.

### A fake wider than the real thing passes forever

R5.3's shrinker was reviewed, cleared and green, and the first time its six
steps ran against the real application they failed at the fourth. Three defects
came out of that one run, and no unit arm in the package could have seen any of
them.

- **The project declared no `[adapter].env`.** `[adapter].env` is a list of
  variable NAMES the engine passes through, so `examples/toy` not naming
  `TOY_ADDR` meant the value could never reach the adapter -- and the
  composition the rollback provider's own package documents ("the provider owns
  the application, the adapter drives that one through `TOY_ADDR`") was
  unreachable for the one project that ships it.
- **The provider's address is not stable.** `Restore` stops the toy and starts
  it again on `127.0.0.1:0`, deliberately, so two worlds on one box cannot
  collide. An adapter spawned once is therefore talking to a dead port from the
  second candidate onward: the driver has to re-open it against the provider's
  current address for every candidate, and nothing said so because nothing had
  ever run a second candidate against a real world.
- **The final replay skipped the restore.** The one candidate whose trace is
  REPORTED and PROMOTED ran on whatever the previous candidate had dirtied.

**The common cause is the fixture, and it is the mirror of the narrow-stub
lesson.** This package's world fixture answers every request, so a dirty world
and a clean one produce the same verdict and a missing restore changes nothing
it can observe. `cmd/toy` answers `409 "that name is taken"` to a repeated
`create_user`, which is what made the third defect arrive as a failure instead
of a silence. A stub NARROWER than the real thing fails loudly and gets fixed;
a stub WIDER than it passes forever.

**And an arm was holding the defect in place.** It read `restored ==
probes - 1` and explained it in words: "the final replay is the one that does
not rebuild a world." `docs/design.md` and `docs/rollback.md` both say the
opposite and say it is not optional -- so the documents were right, the code was
wrong, and the test asserted the code. A test can pin wrong behavior as a
property, and the thing that dislodged this one was not a reading of the
assertion but a real application refusing the request.

**How to apply.** When a suite's subject is a COMPOSITION -- an engine, a
provider and a system under test -- the unit arms cannot be the only arms, no
matter how many of them are green. Run the whole thing against the real
dependency once before calling the composition built, and treat every
permissive answer in a fixture as a question: what would the real one refuse
here, and which of my arms would notice?

### A cleanup that cannot run reads exactly like a cleanup

Four of this tree's `TestMain` functions did

```go
dir, _ := os.MkdirTemp("", "rumor-run-bin")
defer func() { _ = os.RemoveAll(dir) }()
...
os.Exit(m.Run())
```

and a deferred call does not run when the goroutine leaves through `os.Exit`.
So the cleanup was dead code that reads as live code, and every run of those
packages left about 30 MB in `/tmp` -- 300 MB after one gate's three passes,
and 3.1 GB on a reviewer's box after a day of mutation runs.

**Nothing in the toolchain says so.** `go vet ./...` is clean on the tree that
contains all four, and the lint gate was green on the same commit. The checks
that exist for this pattern are in third-party analysers that this project's
pinned set does not run, so the arm has to be the project's own.

**The arm is shaped like the defect, not like the four instances.** A test per
`TestMain` would have covered the four that were known; `tests/hygiene` parses
every `.go` file in the tree and reports any function that both defers a call
and exits the process. That found a fifth site in shipping code on its first
run -- `cmd/toy-adapter`'s `main`, where every exit path closes by hand and
says so in a comment, which is why the arm carries a declared exemption with
its reason. An exemption whose named function no longer exists fails the arm
too: a declaration that covers nothing must not pass as a declaration.

**And the resource a package creates is the package's to remove.** The same
issue's other half was not a stranded defer at all: `toyrestart`'s provider made
a world directory in `open`, `Discard` removed only the saved world *file* of
one checkpoint, and nothing removed the directory -- so every caller leaked one
by construction, 374 of them in a day. The fix that suggests itself, removing it
in `Close`, breaks a documented contract that an existing arm asserts (`Close`
stops the toy and leaves the files, which is how the arm for "the file is
perfect and the server is dead" kills the server). The removal belongs in its
own method, and the callers that open a world call it.

**How to apply.** When a cleanup is a `defer` in a function that can call
`os.Exit`, it is not a cleanup. Check the class across the tree rather than the
instances you were told about, declare the deliberate exceptions where a reader
will find them, and make the declaration itself fail when its subject moves.

## R6

The lessons from R6's children, under one heading per phase (fable, coop
r4/137). A child inserting a lesson adds a `###` here in landing order.

### An arm whose reason expired is not an arm you weakened

R6.1 changed an existing arm, and the change reads from a distance exactly like
the thing this project forbids: an assertion that used to demand one answer now
demands a different one, in the same commit as the code that produces it.

**The two cases are not alike and the difference is checkable.** Two days apart
this tree has seen both:

- **R5.3**: an arm asserted `restored == probes - 1` and explained it in words —
  "the final replay is the one that does not rebuild a world". `docs/design.md`
  and `docs/rollback.md` both said the opposite and said it was not optional. The
  arm had pinned a DEFECT. The documents were the authority and the arm was
  wrong when it was written.
- **R6.1**: an arm asserted that past an eventual type's bound the statute's
  finding stands with no overlay, because "v1 RESERVES `stale` for R6 and defines
  nothing". That was RIGHT when written. Issue 89 is where R6 defines the word,
  so the arm's own stated reason expired — and leaving it would have pinned the
  reservation after the thing reserved had arrived.

**The test is whether the arm's stated reason still holds, not whether the arm
still passes.** An arm that pinned a defect was always wrong and the documents
show it. An arm whose reason expired was right, and what changed is the world it
described. An arm with NO stated reason gives a reviewer nothing to check in
either direction, which is the argument for writing the reason beside the
assertion rather than only in a commit message.

**Two things make the expired kind auditable**, and both were done here rather
than asserted:

- **The new arm asserts strictly MORE than the old one.** Value, kind, rule,
  that the statute's verdict survives underneath, and that the reason locates
  what was late. A change that asserts strictly less is a weakening whatever
  charter it cites.
- **The name moves with the behaviour.** The arm read
  `...IsPendingAndOutsideItIsNot`, which after the change would have described
  the opposite of what its body checks. A stale name on a passing arm is how the
  next reader is told the wrong thing by something green.

**How to apply.** When a change needs an existing assertion to flip, say which
case it is and show the evidence for it: the document that contradicted the old
arm, or the charter line that spends what the old arm was waiting for. Then show
the new arm asserts more, and rename it if its name described the old answer.

### A hand-cut copy of a moving file is stale from its second edit

`examples/*/mutants/` holds one mutant per rule, each carrying the sentence
**"One change against the statute and nothing else, which is what makes `rumor
attaint`'s kill attributable to the rule it names"**. Nothing checked it, and on
2026-10-26 nine of `examples/toy`'s ten were false: each had been cut from the
statute as it stood and never re-cut, so all nine lacked a rule the statute had
since gained and five lacked a whole block of them.

**A mutant missing a rule kills it by ABSENCE**, which is not the deviation the
mutant exists to assert — and `attaint` said so in its own output, crediting
one mutant with eight kills, seven of them for rules that have mutants of their
own. The tally read `10 rule(s) killed, 0 unmutated` throughout. After the
re-cut every rule is killed by its own mutant and **the tally is unchanged**:
the number was never going to move, only its meaning, which is why no figure
could have found this.

Two things close it, and the second is the transferable one:

- **The mutants are generated, not maintained** (`tools/make-toy-mutants.py`),
  the same answer `tools/make-validate-fixtures.py` gives for the validate
  fixtures. Each entry declares the mutant's own change and asserts its anchor
  is UNIQUE in the statute, so a rename cannot silently mutate a different rule.
- **The one-change claim has an oracle** that reads the files rather than a
  table: `tests/statute`'s `TestEveryMutantIsItsStatutePlusOneChange` strips
  comments and counts the contiguous regions in which a mutant differs from its
  statute, requiring exactly one. It is a region count and not a line count,
  because replacing a four-line rule body with one line is one change; what is
  not one change is a second region elsewhere in the file.

**The four mutants that are not one hunk are DECLARED with their counts, not
excluded.** They are hand-cut mutants in R1–R3 packages, none shown to need
more than one region, and the declaration is pinned in both directions — repair
one and the test fails because its count moved, which is the signal to delete
its row. A filter would have made them permanently invisible; a declaration
makes them a list someone can finish.

**How to apply.** Any artifact that is a COPY of something under active edit
needs a generator and an oracle over the copy's own claim about itself. Look for
the claim first: here it was written in every mutant's header, in prose, for
months, and was the only statement of the invariant anywhere.

### A reader holds the total, and a pattern sees only its own vocabulary

R6.3 added one five-line arm to `tests/vocabulary` and broke it twice, in two
ways that look unrelated and are the same shape: a check whose own report is
assembled from the thing it is checking. The enumeration below is ratchet's at
coop r6/132; three of us hit it in one afternoon.

**ONE: a denominator is only evidence if the reader holds the total
independently.** The arm's first form printed `PROOF 5 warrant header(s)` and
passed. Five of HOW MANY was the question it could not answer, and the file held
eleven — the regex read `^// (\d+) -> (\d+) AT ` while the older headers read
`" in R3.1"`, so six were invisible to the arm and to its own log line. That
form is in no tree this entry ships on; `vocabulary_test.go:607` is the FIX,
`(?:AT|in)`, which is why the broken one is quoted here and not cited by
line. The rule this project already had, **name the count in the
positive line**, was FOLLOWED here and did not save it: the writer names what it
found, and a count without its total cannot distinguish 5 of 5 from 5 of 11.
Fixed in `abc400a` by accepting both markers a real header carries.

**TWO: a pattern keyed on the subject's own vocabulary excludes whatever is
spelled differently.** Four instances in one day, across three agents:

- quill's `engine.Judge` sweep, a qualified name, which cannot see a call from
  inside the same package.
- quill's `-run Stale|Pending`, which silently dropped an arm named neither.
- this arm's `" AT "` above, five of eleven.
- ratchet's verification script, written to CHECK the fix above, whose loose
  pattern stopped at the space — so every capture was a prefix the strict
  pattern could never match, and it printed `11 of 12` beside all twelve listed
  as excluded. Twelve was the total on the tree that script ran on -- R6.3's
  first cut, off v0.1.34 and without the gate bundle's own `34 -> 43` prose
  line -- a superseded lineage whose shas resolve today and stop the moment it
  is swept, so it is named here by what it lacked rather than by one of them.
  This base holds THIRTEEN and the arm prints `11 of 13`.

**THE CURE IS A PARTITION, ASSERTED AND NOT PRINTED.** Two patterns read the
same lines, so the warrants and the exclusions must sum to the total:
`vocabulary_test.go:652` refuses `len(found)+len(skipped) != len(all)`, which is
an invariant rather than a taxonomy — it needs no opinion about which lines
ought to be headers. Printing both numbers made two of us look; it is what
caught instance four. But rook drove that state at coop rumor/1153 and the arm
PASSED, because the coverage was a `t.Logf` and the only assertion was the
constant: eleven matched beside twelve excluded, an impossibility, green. **A
diagnostic catches this when somebody reads the line; a check catches it when
nobody does.**

**AND A FIGURE QUOTED IN PROSE NEEDS THE BASE IT WAS MEASURED ON** (fable, coop
rumor/1133, on D4). All three corrections above are that one miss: a sha from a
superseded lineage, a line number that moved under the fix, and a denominator
true of the tree it was taken on. Prose is the one artifact in the repository
with no instrument comparing its figures against the tree it ships in, so the
base travels with the figure or the figure rots quietly.

**How to apply.** Two tests, in this order. Does the report carry its own
denominator, measured by something other than the pattern under test? And is
the relationship between the numbers ASSERTED — not merely visible — so that a
set of figures which cannot all be true fails the run? Any arm that reports
coverage of a corpus it also selects needs both; the second is one line, and
without it the first is a courtesy to whoever happens to read the log.

### A wrapper that hides a nil, and a rule that assumed the count only rises

R6.4 widened one signature and changed one arm's rule, and each change was
caught by a suite rather than by a reviewer — both for the same reason: the
thing the change quietly assumed was never written down anywhere an instrument
could read it.

**A LIFT THAT PRESERVES EVERYTHING BUT NIL-NESS CHANGES WHAT THE CALLEE CAN
REFUSE.** `shrink.Request.Replay` grew a field, which meant thirty-two existing
fixtures no longer type-checked. Rewriting thirty-two closure bodies would have
made the diff about punctuation, so a test-only lift carried them across and its
name carried the assumption: `settledN(needs(3))` says *this fixture's runs
settle*, which is what makes those arms about length rather than about waiting.
The lift returned a non-nil closure over whatever it was handed — including
`nil`. `TestBisectRefusals` passes `nil` to assert the "no run function"
refusal, and that refusal became unreachable: the arm panicked inside a
candidate instead of reading the message it was written to read. The lift passes
nil through now. **A wrapper is transparent only in the dimensions you checked;
a callee that refuses on a property of its argument can be blinded by a wrapper
that preserves every other one.**

**AN ARM THAT REMOVES AN ORDERING ASSUMPTION CAN INTRODUCE A MONOTONICITY ONE.**
`declaredQuotes` carries warrant headers — `// N -> M AT <phase>` — and the arm
tying them to the constant took the greatest TARGET, deliberately, so that no
reading depended on file order (coop r6/120, with a planted out-of-order header
driving it at rumor/1143). That reason was right and the rule still broke: R6.4
removed a dead function whose doc comment carried an attributed quotation, so
the count went DOWN, and with a `47 -> 46` header the
greatest target is 47 against a constant of 46 — a correct tree, refused.
Switching to the greatest `from` survived the fall and failed the next rise, and
the two headers together are a 2-cycle: **no order-free rule can name the last
transition of a sequence that can move in both directions.** The final form
asserts file order as a property instead of assuming it — each `from` equals the
previous `to`, the last `to` equals the constant, a gap or a repeated start
refuses by name — which also catches a break no maximum ever could.

**AND A DELETION CAN CARRY A QUOTATION OUT WITH IT.** The count moved because
code was removed, not because a quotation was edited: the sentence lost its
subject rather than its truth. A sweep that counts attributed quotes across a
tree will therefore fire on a pure deletion, and the warrant header is what
tells the next reader which of those two things happened.

**AND SAY WHAT WENT, NOT WHAT IS LEFT.** The first draft of this entry said the
removed comment held the tree's ONLY quotation of that sentence. It did not:
`internal/engine/pending.go` still quotes the vocabulary's "at every full
reconciliation" inside a longer span, and the phrase is in two documents
besides. What was true is narrower and is the only part the count rests on —
that file carried one attributed quote and what replaced it carries none. A
claim about a whole tree needs the whole tree searched; a claim about one file
needs one file, and the two are a `grep` apart.

**How to apply.** When a signature grows, ask what every existing caller was
assuming that it had no way to state — then make the carrier say it, and check
the carrier preserves the properties the callee refuses on. When an arm's rule
is chosen to remove one assumption, name the assumption it adds: a maximum
assumes monotonicity, a last-in-file assumes order, and a chain assumes only
what it asserts.


## R7

The lessons from R7's children, one `###` per child in landing order.

### A harness that builds the project differently reads a world the product does not have

R7.1 added an end-of-run refusal and `tests/clock`'s arms disagreed with the
CLI about the same seed: `rumor run` exited 0 where the suite saw a refusal.
Two instruments, one subject, and the one that was wrong was the instrument.

**The harness was missing one line the product has.** `cmd/rumor` builds a
quiesce prober from the project's own probes; this suite's helper assembled a
`schedule.Project` by hand and left `Prober` nil. The walk gates its
end-of-run reconciliation on `p.Prober != nil`, so with no prober NO pending a
run holds is ever re-asked, and every one of them survives to the end. The
suite was not testing a stricter world; it was testing a different one.

**What makes it worse than a wrong answer is which arm it flattered.** Three
seeds were in the suite as the CONTROL for the new refusal — they hold a
pending and CLOSE it, so they must not refuse. Without the prober they could
not close anything, so they refused too, and the control became a second copy
of the arm it existed to contrast with. A control that agrees with its arm for
the arm's own reason is worse than no control: it reports a contrast that was
never drawn.

**It is the fake-wider-than-the-real-thing lesson with the sign flipped.** That
entry is about a stub that accepts more than the real collaborator and so
passes forever. This is a harness that provides LESS than the real caller, and
it fails loudly — but only once something in the product can tell the
difference. Here that was a refusal keyed on a state only the reconciliation
can clear; before it existed, the suite had been green for a world the product
never runs.

**AND THE SAME PHASE PAID FOR IT AGAIN IN A CAPTURE.** Three of five control
digests in this child's first bump matched two other actors to the character
and two did not, because the two were captured differently: the pair needed an
EXIT STATUS as well as the bytes, so they went through `out=$(run)` instead of
a pipe, and `$(...)` strips the trailing newline that the piped three kept.
Needing a second fact had silently changed the instrument. **A digest and a
status come from one run through a FILE** — `run > f; rc=$?; sha256sum < f` —
or they come from two different measurements wearing one number. The partial
match is what localized it, which is the same tell vocabulary v3 records from
a control re-run with a flag missing: a full mismatch would have been better
evidence and arrived sooner.

**How to apply.** When a suite assembles the product's own request object by
hand, list the fields the product's caller sets and say, field by field, why
each omission is safe — a nil slot is a DECISION about the world under test,
not a default. Where two instruments answer differently about one subject, the
first hypothesis is neither "the code" nor "the other instrument" but that the
two are driving different worlds. And where an arm has a control, the control's
own premise needs an assertion: here, that the control's pendings are held at
all and reach zero open, so a run that cannot hold one fails as a control
rather than passing as a contrast.

### An arm belongs at the clause, when the wiring can only fail by not happening

The same commit armed a decision rather than its call site, which is the shape
`internal/shrink`'s R6 finding warns about — a tested decision beside an
untripped wiring. ratchet gave the test for telling the two apart (coop r7/21),
and it is about what the SITE can get wrong.

That R6 site contributed three observables of its own — the words it printed,
the stream it printed them on, and the status it returned — and nothing read
any of them, so it could be wrong three ways while the function it called
stayed green. This one contributes nothing but the call: one unbranched
`if err != nil { return rec, err }`. **A site that can only fail by NOT
HAPPENING is armed when its happening is asserted**, and two end-to-end arms
die when the error is made never to propagate.

**Why the clauses needed it.** Two mutants survived the suite arms — keying
the refusal on "a pending is open" instead of on the bound's FORM, and dropping
the guard that reads whether the clock moved. Neither state is reachable from a
fixture: a quiesce-form pending open at the end needs a re-ask that cannot be
made, which no committed run produces, and nothing moves the clock until a
later child draws `advance`. The decision takes exactly the facts it judges, so
the cells a fixture cannot build are built against the function, and the
narrowing stops being a sentence in a comment.

**How to apply.** Count what the call site itself can get wrong. If the answer
is "nothing but its own absence", assert the absence end to end and put the
cells where the clauses are. If the answer is "the words, the stream, the
status", the site has observables and they need arms of their own wherever the
function's own tests cannot see them.

### A control re-derived from its subject agrees with whatever the subject became

`tools/d4control.py` is the D4 release control: five seeds, each with its base,
its invocation and its expected reading, driven against a tag to answer whether
the behaviour the phase froze is the behaviour the tag has. The method it
carries is the separation of its two halves, and that separation exists because
the first version did not have it.

**THE FIRST VERSION READ ITS EXPECTATIONS OFF THE TREE IT WAS JUDGING.** A
control whose figures come from the live `examples/toy` is a control that moves
when the toy moves, so a change to the project under test changes both the
measurement and the thing it is measured against, and the two agree for exactly
the wrong reason. The control now has one subject per half:

* **The BYTES half** runs against `examples/toy` **frozen at a commit named in
  the tool** — materialised per run with `git archive <SUBJECT> examples/toy`
  into a temporary directory, so the clone's own working copy cannot perturb it.
  Five 16-hex digests, and they are not allowed to move for the rest of the
  phase. The frozen subject is a COMMIT, which is why a later change to the live
  toy — a weight added for a new engine kind, say — leaves this half untouched
  by construction rather than by anybody remembering to check.
* **The FIELDS half** runs against the **live** `examples/toy` at whatever head
  is being judged, and compares four named fields off the finding line — kind,
  step, rule, planted — rather than the whole of stdout. What it answers is
  "what happened did not move", which survives a renderer change that the bytes
  half is entitled to catch.

**THE ARCHIVE INTO A TEMPORARY DIRECTORY CARRIES A PRECONDITION THE PROSE KEPT
LOSING: the project directory's BASENAME must still be `toy`,** because the run
echoes the project directory's name into its summary line, so a frozen subject
unpacked as `subject/` or `t/` changes the output it was frozen to reproduce.
`git archive <commit> examples/toy` preserves the path, so the basename is
right by construction — and that is the point worth taking: a precondition an
instrument satisfies structurally is one its documentation can forget, which is
the second-best place for it and the only place it rots.

**A FIGURE WITHOUT ITS BASE AND ITS INVOCATION IS NOT A CONTROL.** Two of the
five seeds are driven at a different base and a different step count from the
other three, and an earlier table held the digests alone. A reader reproducing
one of those rows with the wrong `--steps` gets a different digest and no way to
tell a regression from a mis-drive, so each row now carries `base`, `steps`,
`profile`, the digest, the exit code, the line count and the fields — a row is
the whole invocation or it is an anecdote.

**AND THE CONTROL'S OWN ARMS READ CAPTURED OUTPUT, NOT STUBS.**
`tools/test_d4control.sh` drives the control against real output from v0.1.31,
v0.1.36 and d7ce321 with exactly one line changed per cell, and asserts it
REFUSES. A fixture of invented shape tests the fixture: a stub emitting a shape
the real run never had will satisfy every arm written against it while the arms
stay blind to the place a defect actually lives. One cell was **deleted** rather
than kept when its subject turned out unable to fail — clock facts go to the
ledger and stdout prints none, so an arm asserting a clock token's absence on
stdout passed on every build ever made — and the reason is left in place where
the cell was, because an arm that cannot fail is worth less than a note saying
why it was removed.

### A refusal that pins two values together hides which of them the code read

R7.2's clock request carries two rulings that interact (fable, coop r7/51). The
ledger's clock fact **is written from the RESPONSE's `t`** -- an adapter that
rounds forward has answered for the world, and the ledger records the world.
And a `set` whose answer **is not the time that was asked for** is refused, "by
more than the adapter's declared granularity if it declares one, else at all"
-- and nothing in the protocol lets an adapter declare a granularity today, so
"at all" is the whole of it.

**TOGETHER THEY MAKE THE FIRST RULING UNOBSERVABLE.** In every run that is not
a refusal, the answer equals the ask, so an arm over the fact's time cannot
tell which of the two the producer read. I measured it rather than reasoned
about it: with `t = got` changed to `_ = got` in the seal -- the fact written
from the ask instead of the answer -- `internal/schedule` stays GREEN, while
three other mutants in the same sitting each killed an arm.

**THE TEMPTATION IS THE CELL THAT PASSES EITHER WAY.** An arm asserting the
fact equals the response reads like coverage of the ruling and is satisfied by
the ask, because the two are equal wherever it runs. Written, it would be a
check that cannot fail, in a file whose other cells can -- and the next reader
has no way to tell the difference from the outside.

**SO THE ARM SAYS WHAT IT DOES NOT PROVE, IN ITS OWN BODY.** Not in the commit
message, which the reader of a failing test is not holding, and not in a
sibling document: in the test that a reader would otherwise credit with the
coverage, naming the condition under which the arm becomes possible -- the day
an adapter can declare a granularity and the equality refusal relaxes to it.
A gap recorded where the credit would be claimed is the only place it stays
visible.

**THE GENERAL SHAPE, which is worth more than the instance:** when a guard
makes two expressions equal on every path that survives it, a test over either
one measures both, and a change of SOURCE between them is invisible to the
whole suite. Finding that out costs one mutant. Not looking costs a believed
arm, and a believed arm is worse than a missing one.

### A walk shows the product can REACH a state; a cell says what the state MEANS

R7.3 gave the walk an `advance` that lands exactly on a pending's bound, which
is the one input separating `now-claimTime > bound` from `>=`. I built the
end-to-end drill for it first -- two seeds of one project, one landing on the
bound and finding nothing, one landing a tick past and composing `stale` -- and
wrote in the landings arm that the middle landing "is the one input that
separates a strict comparison from a loose one".

**IT WAS NOT, YET.** ratchet drove the mutant at that head (coop r7/56):
`> e.Bound` to `>=` SURVIVED the whole tree -- 37 packages, rc 0 -- because the
cell where the comparison lives had INSIDE and PAST and nothing at the bound's
own instant. The landings arm asserted the landing SET, not the verdict at it.
A comment claiming a hinge is not a hinge.

**THE TWO TIERS ANSWER DIFFERENT QUESTIONS AND BOTH ARE NEEDED:**

* The WALK answers *can the product reach this state at all*. Before R7.3 no
  walk could put the clock on a bound, so a duration-bounded pending had one
  possible end — the run's unmeasured refusal — and the comparison was
  unreachable from any drill. That is worth a drill precisely because it is a
  claim about reachability, which a unit test cannot make.
* The CELL answers *what the state means*. It runs in 0.01s against the
  comparison itself, in every suite, on every change — where the walk costs
  3.3s, a real adapter, a profile and a pinned seed.

**SO THE ORDER IS THE CELL FIRST.** The expensive drill is what you add once
the cheap one exists, and I had it backwards: I reached for the impressive
measurement and let a comment stand in for the arm. The rule that falls out is
narrow enough to apply: **when a drill's value rests on one comparison, that
comparison gets a cell at its own tier, and the drill proves reachability
rather than meaning.**

**AND THE BOUNDARY CELL IS THE ONE A SUITE IS MOST LIKELY TO LACK.** INSIDE and
PAST read identically under either operator; only the boundary tells them
apart, and a suite assembled from "a case that works and a case that fails"
will have both and not it. Where a comparison has a boundary, the boundary is
the cell — and after R7.3 it is also a state a drawn step reaches by
construction rather than a value only a test could invent.

### A pass-through's fixture must answer what neither input could produce

R7.4 moved the toy's clock out of the adapter and into the application: the
adapter now forwards `set` and `advance` to `POST /clock` and answers what came
back, so `advance`'s relativity belongs to the world rather than to the process
relaying the request.

**THE ARM THAT WOULD HAVE PASSED ANYWAY.** The obvious cell hands the stub the
value it expects -- ask for `set 5e9`, have the stub answer `5e9`, assert the
adapter returns `5e9`. That cell is green against the forwarding adapter, and
also against the one that keeps its own number and never calls the toy at all,
and also against one that echoes the request without calling anything: all
three produce the ask. A fixture whose answer equals its input cannot see which
of its inputs an implementation read.

**SO THE STUB ANSWERS A THIRD NUMBER.** It returns `777000000000` -- not the
ask, not the sum of the asks, not anything either arithmetic could reach -- and
the cell asserts the adapter returns THAT. Driven at the head: answering `m.T`
instead of the toy's `out.T` fails all three cells; forwarding every op as
`advance` fails the two `set` cells; sending `m.T + 1` fails the request
assertion in all three.

**AND THE REQUEST IS ASSERTED, NOT ONLY THE ANSWER.** A relay has two halves
and only one of them is observable in the return value: an adapter that
forwarded a `set` as an `advance` would agree with the stub about every number
it got back. So the cell reads what the stub SAW -- method, path, op and `t` --
which is the same argument as the general one above, run on the other
direction.

**THE TRANSFERABLE RULE:** where the subject's job is to pass a value through,
the fixture's answer must be a value the subject could not have computed, and
both directions get an assertion. It is `A control can be the case it controls
for` applied to a relay: here the control's value and the subject's own
arithmetic coincide unless the fixture deliberately makes them differ.

### A reader of a fact nothing writes passes every cell that builds its input

R7.4 extended `advance`'s targets to the deadlines a project declares, and the
first mechanism I wrote for it could not work at all: it harvested the SEALED
PREFIX for `observed` facts whose query `[deadline].instants` names.
`ledger.KindObserved` has validation and has **never had a producer** -- "the
observation is the check's INPUT while the sealed prefix is the DATA" (fable,
coop breakout-r2/13, ratified) -- and `internal/schedule`'s own comment says
NOTHING IS APPENDED eight lines above where I was working.

**THE UNIT CELLS PASSED, AND THEY PASSED BECAUSE THEY BUILT THE INPUT.** Each
one appended `observed` facts to a ledger by hand and then asserted the
harvest found them: the matching query, the non-matching one, two at different
instants, a non-numeric value refused. Four cells, four green, over a shape no
run produces.

**THE END-TO-END ARM SAID "no step advanced where the deadline is declared".**
It is the only arm that could, because it asks the question through the
product: a walk, a real adapter answering an instant, and a landing drawn from
what the run had actually read.

**THE RULE THAT FALLS OUT IS NARROW AND NOT "WRITE MORE INTEGRATION TESTS":
where a reader's subject is a FACT KIND, one arm has to obtain that fact the
way production obtains it.** A cell that constructs its own input measures the
reader's arithmetic, which is worth measuring -- the fractional-nanosecond
refusal is still armed there -- and says nothing about whether the thing it
reads is ever written. tests/quiesce's header carries this class from the other
side: an arm proving a mechanism the live path never REACHES. This is its
mirror, a reader of a fact the live path never WRITES, and the same arm answers
both: drive it through the product once.

**AND THE SAME TICK SUPPLIED THE SMALLER VERSION.** A mutant appending to a
recorded reason survived cells that pinned that reason's exact text -- because
those cells called the function that BUILDS it, and the mutation was at the
CALL SITE. A cell has to sit where the value it pins is READ.

### An input-fidelity arm is bounded by the vocabulary it was written against

**The case corpus is checked against the engine's own input, field by field, by
`TestTheEngineInputMatchesTheCaseInput` -- and that arm listed v1's vocabulary:
the observation's six fields, `known` filtered to the bound, and each claim as
`ref|type|planted|visible_to`.** R7 then added two fields to that input, the
`clock` table and each claim's `t`, and extended neither side of the arm. So
every case block in the corpus could omit both and the arm stayed green over
the omission, which is exactly what happened: the three clocked cases carried
`t` on their claims and no `clock` at all.

**WHAT THAT COSTS IS NOT A MISSING ASSERTION, IT IS A KILL FOR THE WRONG
REASON.** rook specified a control for the row -- swap the statute's `c.t` for
`input.clock.now` and require the corpus to notice -- and measured it INERT
(coop rumor/1426). The cure was a case where the two readings differ: a join
honestly inside its expiry, then the clock advanced past that expiry before the
expiry was observed. On the ledger path that case killed the control at once.
On the case path the control died too, and ratchet measured WHY (/1428): the
case block carried no `clock`, so `input.clock.now` was UNDEFINED and the rule
went false. **The mutant died of absence, through a different case, and said
nothing about the two readings differing.** A control that dies of absence
reports the same colour as a control that discriminates.

**THE TWO PATHS DISAGREEING ABOUT WHICH CASE KILLED WHAT IS THE SYMPTOM TO
WATCH FOR**, because it is visible without knowing the defect: one corpus, two
builders, and a mutant that each kills through a different case means one
builder is feeding a rule something the other does not have.

**THE RULE: WHEN A FIELD IS ADDED TO AN INPUT, THE ARM THAT PROVES THE INPUT
FAITHFUL IS PART OF THE CHANGE.** Not a later sweep -- the same change, because
until it is extended the arm's green is a statement about the old vocabulary
and reads exactly like a statement about the new one. Presence is compared
before value, since absent and zero are different answers where a project
without clock control is MEANT to have no key.

**AND THE CONTROL IS NOW A MUTANT RATHER THAN A DRIVE.** m15 swaps the time the
clause reads, committed beside the always-rules it does not resemble: it
disables nothing, it changes which fact answers the question. A drive proves
the corpus discriminated once, on the tree it was run against; a committed
mutant is what re-measures it.

## R8

The lessons from R8's children, one `###` per child in landing order.

### A clause reachable only through a live system is armed by nothing

108's oracle carries two judgements for D4 — a refusal that names neither the
field nor a rule, and a monoculture of one sentence over every hostile value.
Both were implemented, both had been driven by hand in a worktree against a
mutated `cmd/toy`, and both drives were recorded in commit messages. rook then
turned each branch off with `if false && ...`: the build was clean and
`internal/fuzz` AND `tests/run` stayed GREEN. Every finding D4 could emit was a
refusal nobody could show failing.

**THE ONLY ARM OVER THEM ASSERTED THE SUBJECT's SPREAD, WHICH IS THE OTHER
HALF.** It pinned the toy's own answers — 8 distinct messages over 21 system
refusals, none naming nothing — so it proved that a specific well-behaved set
PASSES. The branches that emit a finding were never reached by it. A positive
arm and a negative arm are two different arms, and a suite holding only the
first reads exactly like a suite holding both.

**THE CURE WAS DELETING THE WRITER, NOT ADDING A FIXTURE.** `Findings` was a
field that three statements in `Run` appended to, so reaching the clauses meant
running the whole stage against a live adapter — and a deleted append left
every consumer reading an empty list, which is indistinguishable from a clean
verdict. It became a method composing three clause methods, each a pure
function of the results. There is then no call site to omit: a fixture drives
the same path a consumer takes, so a clause DROPPED FROM THE COMPOSITION dies
in the arms as well as one whose condition broke. That second class had no
cover at all before, and it is the class that cannot be fixed by writing more
fixtures against the old shape.

**A SURVIVING MUTANT USUALLY INDICTS THE FIXTURE, NOT THE CODE.** Twice in one
day a mutant lived, and both times the arm was at fault because its fixture
passed under either reading. Removing `Field != ""` killed nothing because
`strings.Contains(msg, "")` is true for any message, so the guard is redundant
and no arm can ever kill it — it is kept, and documented as unarmable, so the
next reader does not delete it as dead. The channel guard's layer restriction
survived because the fixture beside the accepted case held a SPEAKING refusal;
the separating fixture is one with no system refusal at all. When a mutant
lives, ask first whether the fixture distinguishes the two readings.

**AND THE ORACLE's CHANNEL IS ITS OWN PRECONDITION.** D4 reads a refusal's
text at `body.error`, which nothing declares — `[[action]].response` is an
equality test for what acceptance IS, not a schema. Renaming the toy's tag to
`json:"message"`, against a system that still names every field it refuses,
made the stage emit 27 findings, every one false and all of them the loudest
output it has. An empty message is ambiguous between a wrong key and a genuinely
silent refusal, and only the second is a finding. The run supplies the control:
text on ANY system refusal proves the channel readable, text on NONE is a
could-not-run naming the key. A silent refusal beside a speaking one stays a
finding, which is the half that stops the guard swallowing the defect next to
it.

**A GUARD NO ARM CAN REACH IS THE SAME MISTAKE ONE LEVEL UP.** `stage.Select`
returns a list and `cmd/rumor/run.go` consumes one element of it, so the first
stage to become runnable beside `api` would be selected and silently dropped.
That cannot be driven today — only one stage is runnable, so the list cannot
hold two — and a `len(stages) > 1` refusal would therefore be armed by nothing.
The arm is a tripwire instead: it builds the strongest declaration a project
can make, every known stage enabled, and fails in the SAME CHANGE that makes a
second one runnable, naming which. Driven by flipping that flag: one failure
over the whole tree, and its message says to extend the dispatch rather than
widen the arm.

### A fixture whose plan is the identity is the no-plan case

109's fan-out carries a seeded plan: which session issues each request, in
issue order. A width check refuses a plan the dispatch does not match, and two
live arms drove the fan-out end to end. The plan could still have been
decoration, and was: turning the dispatch off — `sessions[sess]` to
`sessions[k]`, and the recorded session with it — left both suites green.

**THE REASON WAS THE FIXTURE AND NOT THE ARM's SHAPE.** At width 2 there are
two permutations, and SIX OF TWELVE seeds draw the identity `[0 1]` — seeds 1,
4, 6, 8, 9 and 11. Both arms pinned seeds from that half. A fixture whose plan
is the identity cannot tell a dispatch that follows the plan from one that
ignores it, because the two produce the same assignment: the fixture *is* the
no-plan case, and every assertion over it passes under either reading.

**SO THE CURE HAS TWO HALVES AND ONLY ONE IS THE OBVIOUS ONE.** Asserting that
the recorded session is the one the plan chose catches the mutant today.
Asserting that the FIXTURE IS A REAL PERMUTATION is what keeps it caught: the
gap came from a seed, so a seed changed next month would restore it silently
while the first assertion went on passing against nothing. An arm that refuses
its own vacuous setup is the durable half.

**AND THE SAME SHAPE APPEARED THREE TIMES IN ONE CHILD**, which is why it is
written as a shape rather than an incident. A monoculture fixture built from
`"bad request"` tripped a *different* clause and so proved nothing about the
one it named. An accepted-case fixture held a speaking refusal beside the
silent one, so a mutant that widened the refusal set to every result survived.
And the identity plan above. In all three the code was right and the arm
established nothing, so the question to ask of a new fixture is not "does this
pass" but **what would this look like if the thing I am testing were absent** —
and if the answer is "the same", the fixture is the subject's twin and not its
test.

### A verdict's author owns the comparison, and a moved reading needs its own evidence

Every reducer in the shrinker took `want engine.Finding`, so the only verdict a
shrink could keep was the statute's. Widening it to an interface moved the
statute's six-clause comparison out of the gate and behind
`StatuteVerdict.Recurs`, unchanged.

**"MOVED AND NOT REWRITTEN" IS A CLAIM THAT NEEDS ITS OWN CHECK, AND THE ARMS
PASSING IS THE WEAKER ONE.** A suite going green over a refactor says the
suite's assertions still hold; it does not say every clause is still reachable
at the new site. The stronger pair is textual and reachability: the clause
messages extracted at the new site and diffed byte for byte against the old,
and then each clause disabled one at a time to see which arm dies. Six of six
clauses died, with the per-clause arm counts recorded — and five of the six
rest on a SINGLE arm each, so one deleted or re-scoped test silently uncovers a
clause. That margin is worth knowing before it is spent.

**A ONE-IMPLEMENTATION INTERFACE SAYS NOTHING**, so the widening's own arm is a
want that is not the statute's, driven through the real shrinker with the
statute's finding held CONSTANT across every candidate: a reducer still reading
the old type sees them all reproduce and reduces to nothing, so the reduction
that comes back is the new author's doing and not the old reader's.

### A harness must tell four outcomes apart, not two

Mutation runs in this child reported a kill or a survival, and both readings
were wrong at least once.

**A HARNESS THAT READS ONE STREAM CAN REPORT A WORKING ORACLE AS A DEAD ONE.**
Two genuine kills came back as SURVIVED because the classifier grepped `--- FAIL`
out of stdout alone: both arms fired correctly and the nil receiver then
panicked, and the FAIL line did not reach the stream being read. The return
code was 1 throughout. Reading one stream of a two-stream process is the same
defect as reading a figure without its command, and it costs more than the
reverse — a survivor sends you to look at real code, while a false survivor
sends you to "fix" an arm that was already right.

**AND rc ALONE CANNOT TELL A KILL FROM A MUTANT THAT DIED OF ITS OWN
MALFORMITY.** A `goto` into a loop did not compile, which is not a kill. A
format verb applied to the wrong type panicked, which gave rc 1 for the wrong
reason and would have been counted. A mutant whose anchor equals its
replacement changes nothing and silently joins the skips. So the four outcomes
a harness owes its reader are: killed, survived, no-op, and malformed — and
only the first two are evidence about the arms.

### An oracle is only as sound as where its inputs come from

109 found this three times, in three different places, and each time the clause
itself was correctly written.

**A VERDICT MAY REST ON THE ANSWERS INSTEAD OF THE WORLD.** The overfill clause
counted the acceptances a fan-out received and never read what the system
HELD — so it was a claim about responses wearing the words of a claim about
state, and a system that answered 200 to two requests while persisting nothing
read as having overfilled. The race's signature is the system's own quantity,
and the fix was to read it back through a declared query. A clause whose
subject is the system's state must take that state from the system, not infer
it from what the system said about it.

**AND TWO INPUTS DRAWN FROM ONE SOURCE CANNOT DISAGREE.** A limit read from an
observation, beside a held count read from the same system, makes the clause
"the system disagrees with itself" rather than "the system exceeded its
DECLARED limit" — one reader, two readings, no independent fact between them.
So the limit is the walk's own drawn number, independent of the system by
construction, and the bindings already said why: "an oracle that read the same
field could not disagree with it". Where a clause compares two quantities, ask
of each where it came from, and if the answer is the same place, the comparison
is a tautology waiting to pass.

**AND A GUARD NO HONEST RUN CAN FIRE NEEDS ITS BROKEN INPUT MANUFACTURED.** The
clause separating "the answers and the world disagree" from "the limit was
exceeded" cannot fire on a correct system, by design — so it was driven by
mutating the system under test to answer acceptance and persist nothing. A
guard like that is otherwise armed by nothing while looking armed: the suite is
green, the clause is reachable in principle, and no run has ever reached it.
The input the guard exists to catch has to be built on purpose, and if it
cannot be, the guard is a comment.

**A STAGE THAT DRAWS ITS OWN SETUP MAKES ITS REACHABILITY A LOTTERY.** The
concurrency stage builds the world it contends in, and the obvious way to build
it was the walk's: draw each setup argument from the project's declared list
through the run's own generator. Measured on the shipped toy, that would have
failed two times in three. `create_event.visible_to` draws from three subsets of
roles and a would-be contender is a GUEST relative to the event it has not
joined, so the two subsets that omit `guest` answer every request 404 before the
capacity check is reached — the stage would have reported "the system was never
asked to choose between them" over a system whose race is one line away. The
drawn setup was replaced by an enumeration: the first declared value of every
argument, then one coordinate moved at a time, stopping at the first world the
clause can be answered in. WHAT A SEED CHOOSES IS THE PART A REPLAY HAS TO
REPRODUCE — here the issue order of the fan-out — and not the part that decides
whether the question can be asked at all. Where a harness has to construct the
conditions of its own measurement, drawing them is how an oracle becomes
intermittently vacuous, and the vacuity looks exactly like a pass.

**AND I DID NOT PREDICT THE SECOND UNREACHABLE WORLD.** I expected one -- the
subset naming only `admin` -- and wrote that into the code's comment before
running it. The run showed two, with both members answered "no such event",
because the role in `visible_to` is relative to the event and not the actor's
own. The comment was corrected from the output. A count asserted from reading
the declaration, where the mechanism is in the system under test rather than in
the declaration, is a prediction wearing a measurement's clothes.
