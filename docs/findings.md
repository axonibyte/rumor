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

## Deduplication

Not yet written.

## Determinism

Not yet written.

## Coverage reports

Not yet written.
