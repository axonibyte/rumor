# Findings

> Stub from R0.1. The headings below are the ones this document owes; the
> content arrives with the phase that owns the subject. `docs/coverage.toml`
> names this file, and `rumor docs --check` fails if a source path it should
> describe is not mapped.


## Explaining a finding

A finding's DERIVATION is the answer to "why did this run say that", as data:
`internal/explain.Build(finding, ledger, check, run)` returns a `*Derivation`,
and `rumor explain` renders it. The shape is R11 d1's; the `--json` rendering
and its field-by-field schema arrive with d3.

**It is a view and not a second source.** Every field is derived from what a
run already retains -- the finding, the sealed ledger and the check -- so
nothing about a finding is stored twice and a derivation cannot disagree with
the run it explains. Where a field is not derivable, `Build` refuses by name
rather than reporting an absence as a value.

**It takes three inputs, because a finding alone cannot say why.** The
statute's input carries `known` as a list of REFS: what the actor knew, and not
how it came to know it. The claim or tell that made each ref known is in the
ledger, at a position, so the derivation joins them back -- and for a knowledge
bug that join is exactly the question of whether the server or the rule is
wrong.

**It has three parts, kept apart on purpose.**

  * `verdict` -- what the statute held: the value, the engine's own `says`
    rendering so an absence cannot read as a verdict of nil, and the statute
    lines (`module:row`) whose bodies succeeded. Each line carries whether it
    is a `default` row, because a finding off a default means NO clause matched,
    which reads nothing like a clause firing. Where an engine rule overrode the
    statute -- a disagreement held open as `pending`, or a pending past its
    bound imposed as a `bug` of kind `stale` -- the overlay is named here and
    the statute's own verdict is kept beside it.
  * `expectation` -- what the actor knew and how: every flattened fact with the
    claim or tell and position that made it known, the claims the statute was
    given in the order it was given them, and `holds`, every other rule the
    caller named with the value the statute gave it. There is no field for "the
    expected value": no rule name is the vocabulary's -- `believed` is a
    convention of the packages under `examples/` -- and a field named for a
    convention would read as a contract and come up empty on the first statute
    that spells it otherwise.
  * `observation` -- what was seen, read out of the input the statute was given
    rather than recomposed from the check, so the derivation shows what the
    statute actually read.

A reader deciding whether to go to the server or to the statute is deciding
whether the chain rests on a rule they disagree with or on facts they agree
with, and one merged list of "evidence" would destroy that distinction.

Beside the three: the run's `clock`, whenever the run has one and not only for
a pending finding, and a `selector` -- the check at its position, which is the
only thing that identifies a finding today, since nothing in the finding itself
does.

**The raw evaluation trace is deliberately absent.** OPA's pretty text is the
evaluator's narration rather than the reader's question, and embedding it would
make the JSON schema hostage to OPA's formatting. Nothing retains a finding
past the process either, so the pointer back to the trace cannot be a
retrieval: `reproduction` names the package, the case, the seal and the
statute's digest -- the inputs that RE-RUN the finding -- with the trace's event
count beside them as a size and never as an identity.

**Four refusals, each by name.** A finding with no trace (`ErrNoTrace`): there
is nothing to derive from, and an empty explanation would read as "the statute
said nothing" when the truth is "nobody looked". A ledger with no sealed prefix
(`ErrUnsealed`): the world at the finding's position is not knowable, so
neither are its facts. A finding with no statute input (`ErrNoInput`): the
input is the derivation's spine. And sources that contradict each other
(`ErrDisagrees`): a ref the actor knows that the sealed prefix cannot explain,
or an input observation that is not the check the caller named. The first three
refuse an ABSENCE; the fourth refuses a CONTRADICTION, which is a defect to
report rather than a plausible answer to render.

**An UNDEFINED finding is explained and not refused.** A traced finding off no
definition is the statute saying nothing where the reader expected a verdict,
which is the most useful thing a derivation can report -- it sends the reader to
the statute's clauses and not to the server. It is stated through the engine's
own `says`, with no rule lines, because none held.

### A pending finding

Where an engine rule held a disagreement open, or ended it, the explanation
carries a `THE BOUND` block: which bound applies, how much of it is left, and
what would settle it.

**There are two bound sources, not three.** A **declared duration** -- the
statute's own `bound` for an eventual claim type -- and **quiesce**, which the
stage's prober decides. The engine distinguishes them in the field it already
carries: `verdict.overlay.until` is the instant a duration expires at, and **0
means quiesce**. The overlay's `rule` names the OUTCOME rather than the bound:
`engine.pending` (held open), `engine.quiesce_timeout` (the prober gave up),
`engine.stale` (a held pending re-checked past its bound, which is a `bug` of
kind `stale`).

**The run's clock is not a third bound; it is the condition under which a
duration is judgeable at all.** So a duration bound's explanation names the
instant that made it judgeable. The unjudgeable case cannot appear in an
explanation: `engine.Judge` REFUSES a duration bound with no clock fact at or
before the claim -- "time the run does not control never decides a verdict" --
so no finding is produced and there is nothing to explain.

**What would settle it is v1's rule, and it is said only where something is
waiting.** A pending "is re-checked at the actor's next observation of the same
query and at every full reconciliation", so the explanation names that actor and
query. A `quiesce_timeout` and a `stale` are verdicts rather than waits -- the
register holds neither -- and their block says that nothing is waiting instead
of inviting a re-ask that has already been answered.

**No case under `examples/` is pending as `rumor check` reports it**, because
that walk evaluates a statute and declares no bindings. A pending is reachable
over the same committed ledgers by declaring the eventual type the check already
counts, which is what a run with bindings does, so the cells in
`internal/explain` are the only witness to this block and they drive
`engine.Judge` rather than building an overlay by hand. One cell builds its own
two-clock ledger: `engine.stale` needs one actor and query to disagree at two
different clocks, and no committed pair does -- of the toy's four invitation
cases only `invitation-honored-late` disagrees at all.

### The JSON form

`rumor explain --json PROJECT CASE` writes the same derivation as JSON. It is
the contract the visual-diagnosis phase consumes, and it is **the same Go value**
-- there is no JSON-only struct and no second set of field names, so the schema
below is a description of `explain.Derivation` and cannot drift from it without
failing the pinned fixtures in `internal/explain/testdata`.

**Stable for one finding.** Two runs over one finding are byte-identical: every
order in the value is total (the chain by hops then position then ref, the
statute lines by module then row, the claims in the order the statute was given
them), and the one map in the shape, `holds`, is written in sorted key order. A
diff of two documents is a diff of the findings. HTML escaping is off, so a `<`
in a ref stays a `<`.

| field | type | what it is |
|---|---|---|
| `selector.actor` | string | the observing actor; with the three below, what identifies this finding |
| `selector.of` | string | the claim type the query counts, absent when the check names none |
| `selector.query` | string | what was asked |
| `selector.source` | string | `api`, `browser` or `direct` |
| `selector.position` | number | the ledger position the check sits at, which is the seal plus one |
| `verdict.value` | any | the finding: `bug`, `consistent`, `explained`, `pending`; `null` when the statute defined none |
| `verdict.defined` | bool | false means the statute yielded no finding at all |
| `verdict.says` | string | the engine's own rendering, so an absence reads as `no finding` rather than as a verdict of null |
| `verdict.kind` | string | narrows the finding where the vocabulary gives it a kind (`stale`); absent otherwise |
| `verdict.rules` | array | the statute definitions whose bodies succeeded for the finding itself |
| `verdict.rules[].module` | string | the statute file the definition is in |
| `verdict.rules[].row` | number | its line |
| `verdict.rules[].default` | bool | **load-bearing**: true means the statute reached its fallback and NO clause matched |
| `verdict.statute_value` | any | the statute's own verdict where an engine rule overrode it; absent where none did |
| `verdict.overridden` | bool | whether an engine rule overrode the statute, stated as a word so its absence is a statement |
| `verdict.overlay.rule` | string | the engine rule that overrode it, e.g. `engine.pending` |
| `verdict.overlay.reason` | string | the engine's sentence about it |
| `verdict.overlay.value` | any | the finding the overlay imposes |
| `verdict.overlay.kind` | string | the kind it imposes, where it imposes one |
| `verdict.overlay.detail` | string | the subject's own words, where the engine rule has them |
| `verdict.overlay.until` | number | the clock time the bound expires at; **0 means a quiesce bound** |
| `expectation.holds` | object | every rule the caller named, keyed by the statute's own name for it; the finding itself is not in here, it is the verdict |
| `expectation.holds.<rule>` | object | one rule the caller named: its value, its lines, and whether the finding's evaluation reached it |
| `expectation.holds.<rule>.value` | any | the value the statute gave that rule |
| `expectation.holds.<rule>.rules` | array | the statute lines that rule fired while THIS finding was evaluated; absent when it was not reached |
| `expectation.holds.<rule>.rules[].module` | string | the statute file |
| `expectation.holds.<rule>.rules[].row` | number | its line -- **this is the line that names a wrong rule**, where the verdict's own row is the innocent default |
| `expectation.holds.<rule>.rules[].default` | bool | true means that rule fell back to its own default |
| `expectation.holds.<rule>.reached` | bool | false means the finding's own evaluation never touched that rule, so it cannot be the error |
| `expectation.known` | array | the flattened facts the actor knew, shortest path first: own claims before tells, then by position |
| `expectation.known[].ref` | string | one fact the actor knew at this position |
| `expectation.known[].via` | array | the ledger entries that made that fact known; never empty in a derivation this package returns |
| `expectation.known[].via[].kind` | string | `claim` or `tell` -- how it came to be known |
| `expectation.known[].via[].actor` | string | the claimant, or the teller |
| `expectation.known[].via[].to` | string | the recipient, tells only |
| `expectation.known[].via[].position` | number | where that entry sits in the ledger |
| `expectation.known[].via[].type` | string | the claim's type, claims only |
| `expectation.claims` | array | the claims of the sealed prefix, in the order the statute was given them |
| `expectation.claims[].ref` | string | a claim in the sealed prefix |
| `expectation.claims[].actor` | string | who claimed it |
| `expectation.claims[].type` | string | the claim's type |
| `expectation.claims[].position` | number | its ledger position |
| `expectation.claims[].at` | number | the run's time when it was made, where the run has a clock |
| `observation.actor` | string | the observing actor, as the STATUTE was given it and not as the caller composed it |
| `observation.of` | string | the claim type the query counts |
| `observation.query` | string | what was asked |
| `observation.source` | string | what answered: `api`, `browser` or `direct` |
| `observation.role` | string | the observer's role, where the project has roles |
| `observation.value` | any | what came back |
| `observation.position` | number | where the observation sits |
| `clock.now` | number | the run's time at the seal; the whole object is absent for a run with no clock |
| `reproduction.package` | string | the example package |
| `reproduction.case` | string | the case |
| `reproduction.seal` | number | the ledger position the prefix was sealed at |
| `reproduction.statute_sha` | string | sha256 of the statute that judged it, in full |
| `reproduction.events` | number | the trace's event count: **a size, never an identity** |

**There is no trace field, at any depth.** OPA's pretty trace is the
evaluator's narration rather than the reader's question, and embedding it would
make this schema hostage to OPA's formatting. Nothing in the tree retains a
finding past the process either, so the pointer back cannot be a retrieval:
`reproduction` is what RE-RUNS the finding. With the four fields above,
`rumor explain` re-derives it, and the digest says whether the statute is still
the one that produced it. An arm drives exactly that -- it reads the tuple out
of the document and reaches the trace with nothing else in hand -- and another
asserts no key at any depth of the document mentions a trace.

**The fixtures pin the schema.** `internal/explain/testdata/` holds the
command's own output for `toy/stale-count` and for `toy/invitation-honored-late`
(the clocked one). A change to the shape fails them by name, with the command
that regenerates them in the failure message, and an arm checks that every key
in both fixtures appears in the table above -- so a field cannot be added to the
data and left out of this document. `verdict.overlay`, `verdict.statute_value`
and `verdict.kind` have no fixture, because no committed example produces a
`pending` finding; they are pinned by the overlay arms in `internal/explain`
instead.

## Deduplication

Not yet written.

## Determinism

Not yet written.

## Coverage reports

Not yet written.
