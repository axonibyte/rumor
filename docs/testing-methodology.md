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
