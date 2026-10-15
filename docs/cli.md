# CLI

> Stub from R0.1. The headings below are the ones this document owes; the
> content arrives with the phase that owns the subject. `docs/coverage.toml`
> names this file, and `rumor docs --check` fails if a source path it should
> describe is not mapped.


## `rumor docs --check`

Not yet written.

## `rumor check`

```
rumor check [--examples DIR]
```

Runs every worked example in `DIR` (default `examples/`) **from its ledger
file**: the committed `.jsonl` is loaded through the store, sealed at the
position its `.seal.json` records, and the observation in the case file is
offered as the check. One line per case, then a tally:

```
planted-lie      honest-only                consistent  statute.rego:55            72 trace event(s)  agrees
transitive-count charlie-explained          explained   statute.rego:61           284 trace event(s)  agrees
check: 17 case(s) over 3 package(s) from ledger files, 17 agreed, 0 disagreed
```

Each line carries the finding, **the definition that produced it** — module and
row, with the `default` marked — and the size of the retained evaluation trace.
A case whose finding disagrees with the verdict the example authored is named
on its own line with the rule and both values.

This is issue 44's "under one command": the equality `tests/engine` asserts in
the suite, re-runnable by anyone holding the binary and readable without a Go
toolchain.

Exit codes, the same three the rest of the CLI uses:

| code | meaning |
|---|---|
| 0 | every case agreed |
| 1 | a case disagreed, the tree could not be read, **or nothing was examined** |
| 2 | usage error |

Examining nothing is a failure and not a pass: zero disagreements over zero
cases reads identically to agreement everywhere.

## Subcommands

`docs --check` (R0.1) and `check` (R2.3). Everything else arrives with the
phase that owns it.

## Flags

Not yet written.
