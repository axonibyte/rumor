# 0001 — The engine: OPA embedded as a library, and Go

- **Status:** accepted by cal, 2026-10-14.
- **Supersedes:** the epic's original decided line, *"Rust, one static binary;
  rules are Rego, evaluated by embedded Regorus"*, and the draft ADR on Regorus
  tracing that was being written against it.
- **Recorded on:** issue 1 (Amendments), `README.md`, and `thread:go-port`.
  The inventory behind it is `docs/design.md`, "The engine".

## The question

The product's value is the **explanation behind a finding** — not the finding.
R2 commits to evaluation traces retained per finding, and R10's explain feature
assumes those traces exist. So: does the chosen engine produce a trace that the
host can retrieve?

The question arrived as a measurement, not as a preference. ibis measured the
six capabilities the statutes need against Regorus 0.12.0 and found that five
held and the sixth did not — recorded at `thread:r1` record 1 and
`thread:adr-regorus-tracing` record 0, under issue 28, and credited to ibis
here and in `docs/design.md`.

## What was measured

Regorus 0.12.0 (**ibis**): aggregation, negation, comprehensions and the
input/data split all hold. Two gaps:

- **Test files.** A `test_` rule evaluates, but the library exposes no test
  entrypoint, so tests run host-side by evaluating the package and enumerating
  the rules that held. That shape can report a test rule as *absent*; it can
  never report one as *failed*.
- **Evaluation tracing.** `trace()` exists as a builtin and returns its
  argument, with the crate's own TODO noting that stateful builtins would be
  needed to capture traces, and there is no retrieval API. Coverage reports
  whole lines with `not_covered` empty, so it answers which lines the evaluator
  touched and cannot answer why a finding held.

The gap is diagnostics, not evaluation: Regorus judges correctly. It does not
narrate, and it does not narrate *by design*.

OPA v1.21.2 (G2, this record): the same six arms, driven in both directions,
**six of six**. Capability 5 holds because `tester.Runner` enumerates each
`test_` rule with its verdict including failures. Capability 6 holds because a
trace is retrievable — 28 events for a three-rule policy, naming each rule
indexed, entered, unified and exited. Coverage is additionally range-based, so
one report separates "held" from "evaluated". Figures and run output are in
`docs/design.md` and the G2 checkpoint.

## The decision

**Go, one static binary. Rules are Rego, evaluated by OPA embedded as a
library, pinned at `v1.21.2`.**

OPA is the reference implementation and a Go library: in a Go binary it embeds
natively and brings the trace, the coverage report, the test runner and the
full builtin set with it. The single static binary — the property the original
line was chosen for — is unchanged, and the language of the statutes is
unchanged, because Rego is Rego. R1.2's statutes and their fixtures carry over
byte for byte; only their harness changes.

The cost is a day or two of porting now against a rewrite after R2, when the
engine's output becomes a user-visible feature rather than an internal detail.

The honest note about how this decision was reached, because the record is
worth more than the appearance: cal asked whether Rust was the right choice
three times before the premise behind it was examined, and the Rust line turned
out to have had no written reason. The engine requirement was discovered by
measuring, and the language followed from the engine rather than the reverse.

## What it rules out

- **Regorus**, in any phase. Not for a defect — it judges correctly — but
  because the diagnostics R2 and R10 depend on are absent by its design, and
  waiting for them is waiting on someone else's roadmap.
- **A sidecar.** Running OPA as a separate process or service would keep the
  engine's diagnostics but lose the single static binary, which is the
  property the original decision existed to protect.
- **Two engines.** Keeping Regorus for evaluation and adding something for
  explanation was considered and is ruled out: two engines mean two verdicts
  that can disagree, and a statute whose finding depends on which one ran is
  worse than either alone.
- **Patching or forking the engine to add trace capture**, which was the
  fallback being drafted when the question was still about Regorus. It is moot:
  the reference engine already has it.

## Consequences

- The Rust tree is replaced, not kept in parallel. It stays in the repository
  only until the port lands, so that `main` is never half of each, and is
  removed in one commit (G4).
- R1.1's Regorus inventory stands as the record of what Regorus was measured to
  do. Nothing of it is discarded; it is simply no longer the engine.
- The deny-based licence gate built for the Rust tree is superseded, and the
  Go licence surface is gated on the packages actually built — tests included —
  rather than on the full module graph, which carries requirements that never
  ship.
- OPA declares `go 1.26.0`, so this module's language version rises with the
  run that first takes OPA as a real dependency. `docs/design.md` records the
  toolchain trap that goes with it.
