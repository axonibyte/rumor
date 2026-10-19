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
check: 29 case(s) over 5 package(s) from ledger files, 29 agreed, 0 disagreed
```

Each line carries the finding, **the definition that produced it** — module and
row, with the `default` marked — and the size of the retained evaluation trace.
A case whose finding disagrees with the verdict the example authored is named
on its own line with the rule and both values.

A case whose statute defines **no** finding prints `no finding` in that column,
not `<nil>`: the walk renders `engine.Finding.Says()` rather than the value, so
an absent verdict reads as an absence instead of as a verdict of nil. No
committed example reaches it — every statute carries a `default` — and the
tests plant one by removing that line.

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

## `rumor validate`

```
rumor validate PROJECT
```

Refuses a project whose statute, bindings and platform do not agree (R4.3,
issue 64). PROJECT is a directory holding `bindings.toml`, `platform.toml`,
`statute.rego`, `rules.json`, `cases/` and `mutants/` -- `examples/twin` is one.

**The exit statuses are `docs/contract.md`'s and not this command's choice:**

| status | meaning |
|---|---|
| 0 | passed: every arm that evaluated found nothing |
| 1 | refused: an arm fired, and the line NAMES it |
| 2 | could-not-run: a declaration could not be READ, or the adapter did not answer |

**2 outranks 1.** If the declarations could not be read, what an arm would have
said about them is not known -- and a decode refusal is 2 and never 1, because
"the project is not wrong; its adapter is". fable's D7 drill asserts that
boundary for the adapter and this command keeps the same one.

**It reports what it EVALUATED, not only what it refused.** Eighteen arms are
declared, and an arm whose precondition is absent -- the browser stage off, no
rule declared, no seeds file -- did not pass, it did not run. Those are named
with their reason rather than folded into a number:

```console
$ rumor validate examples/twin          # with the adapter on PATH
validate: twin -- 18 of 18 arm(s) evaluated; 2 action(s), 1 observation(s), 5 case(s), 4 rule(s), 4 mutant(s); 0 refusal(s)
```

Eighteen of eighteen is what a complete small project looks like: fable's D9
drill requires the untouched examples to pass with every arm EVALUATED and none
skipped, which is why the twin enables the browser stage and declares a browser
action with its flow. **An adapter it cannot reach costs four arms**, and that
is worth showing because the figure depends on it -- the twin's
`[adapter].command` is `rumor fake-adapter`, so a `rumor` that is not installed
takes the capability arms and the contract-version arm with it:

```console
$ rumor validate examples/twin          # no rumor on PATH
validate: could not run: asking the adapter to describe itself: spawning "rumor": exec: "rumor": executable file not found in $PATH
validate: arm 13 skipped: the adapter did not answer a describe, so what it can do is unknown
validate: arm 9a skipped: the adapter did not answer a describe, so what it can do is unknown
validate: arm 9b skipped: the adapter did not answer a describe, so what it can do is unknown
validate: arm 9c skipped: the adapter did not answer a describe, so what it can do is unknown
validate: twin -- 14 of 18 arm(s) evaluated; 2 action(s), 1 observation(s), 5 case(s), 4 rule(s), 4 mutant(s); 0 refusal(s)
```

The run is could-not-run either way -- exit 2, never a refusal, because the
project is not wrong when its adapter will not start -- and the four arms are
SKIPPED rather than passed.

The twin's browser action **emits nothing and names no role**, and that is
forced rather than minimal: `tests/contract`'s agreement test refuses an
`examples/*` package emitting a claim type no case carries, or declaring a role
no case carries. Each arm's own fixture lives in `tests/validate/fixtures/`, is
the twin with one change, and trips exactly its own arm.

**Four arms ask the adapter.** The capability arms (9a, 9b, 9c) pair `[uses]` in
`bindings.toml` -- the project's ASK -- against `describe`'s `capabilities`,
what the adapter can actually do; and the contract-version arm (13) compares the
engine's constant, the adapter's `contract` and `platform.toml`'s. For those,
this command spawns `[adapter].command` through the one wire client, sends a
single `describe`, reads one line and stops the process. An adapter that will
not start or will not speak is could-not-run, never a refusal.

## `rumor attaint`

```
rumor attaint [--examples DIR]
```

The self-test. `rumor check` asks whether each example's finding matches the
verdict its author wrote down — a question the statutes themselves answer, so a
statute that had quietly stopped deciding anything would still agree with a case
file written to match it. `attaint` asks whether the statutes are load-bearing
at all.

Eight arms, each with its own refusal text. The text is the verdict: a gate reads
it rather than the exit status, because a drill that trips the wrong arm has not
demonstrated what it was cut to demonstrate.

| clause | what it catches |
|---|---|
| `checker never fired` | a case declares itself a control, the lie is in the prefix, and the observation never reaches it — the control was **armed and never evaluated** |
| `no-case-declares-a-corroboration-control` | no case declares one at all, so "every planted lie produces `bug`" holds for free |
| `planted-lie-did-not-produce-bug` | a case whose sealed prefix carries a planted claim and whose finding is not `bug` |
| `bug-does-not-rest-on-the-planted-claim` | a case declaring `rests_on_planted: true` whose control was **evaluated and did not pass** — the strip |
| `rule-killed-by-no-mutant` | a rule no mutant moves on either path: nothing shows it is load-bearing |
| `mutant-the-statute-has-absorbed` | a mutant that moves nothing, which means the statute has been **weakened** into it |
| `finding-the-package-can-no-longer-produce` | a finding the fixtures expect and the statute can no longer reach — **a deleted clause** |
| `the-bug-survives-removing-the-planted-claims` | the strip's **second direction**: with the planted claims removed altogether the run must come back `consistent`, and a bug that outlives the lie was never the lie |

Plus four counting guards — no package, no case, no rule, no mutant — because
every arm above is satisfied for free by an empty walk.

**Six kinds of line**, then a tally. Over `examples/` the run prints 50 of them;
one of each kind is shown here, and the counts in the tally are the whole run's:

```sh run
# THIS BLOCK IS THE SAMPLE AND ITS OWN CHECK. `release-gate.py` executes every
# fenced block marked `run` and fails on a nonzero exit, so the lines quoted
# here cannot drift from the tool without the gate saying so. The previous
# sample did drift -- `docs --check` stayed green throughout, because a
# path-coverage check cannot see a quoted figure (quill r3/12).
# THE TOOLCHAIN IS RESOLVED AND PINNED, NOT ASSUMED. This one block behaved
# three ways on three boxes: no `go` at all in rook's jail, go1.27.2 in
# quill's, go1.26.9 on the runner (rook /749 and /753, quill /752). The first
# of those surfaced as `doc-run-block-failed` -- a DOCS arm -- for a box with
# no Go on it, which is what ci.yml:137 already calls "a PROVISIONING finding
# and not a code one". ci.yml resolves the name at 8 sites and
# tag-pipeline.yml at 2; release-gate.yml, the file that EXECUTES this block,
# does it at none, so the discipline has to live here.
GO=""
for c in go go126 go125; do
  command -v "$c" >/dev/null 2>&1 && { GO="$c"; break; }
done
test -n "$GO" || {
  echo "no go, go126 or go125 on PATH: this box has no Go toolchain, which is a PROVISIONING finding and not a code one"
  exit 1
}

# GOTOOLCHAIN=local IS THE PIN, AND IT IS A GATE NOT A PREFERENCE. Under the
# default `auto`, a go directive above the installed toolchain is SILENTLY
# satisfied by downloading another one, so naming the binary does not pin what
# compiles the code (ratchet go-port/6, reproduced at ci.yml:138-149). It does
# not make one version run everywhere and nothing here asks it to: go.mod's
# 1.26.0 is OPA's floor, not an equality, so a box above the floor is
# conforming and merely not identical to the runner.
export GOTOOLCHAIN=local

out=$("$GO" run ./cmd/rumor attaint --examples examples) || exit 1

# THE TOTAL IS READ FROM THE PROSE ABOVE, NOT RESTATED HERE. A literal here
# would be a third copy of one number -- the sentence, the comparison and its
# message -- and only two of them inside the block a failure points at, so the
# sentence could go stale while the gate stayed green (quill r3/16). The cost
# is named rather than skipped: this now depends on that sentence's wording and
# says so loudly if it is reworded. Two numbers that can disagree is worse.
want=$(sed -n 's/.*the run prints \([0-9]\{1,\}\) of them.*/\1/p' docs/cli.md)
[ -n "$want" ] || {
  echo 'docs/cli.md no longer states how many lines the run prints, so this block cannot check it'
  exit 1
}

quoted=0
while IFS= read -r line; do
  [ -z "$line" ] && continue
  quoted=$((quoted + 1))
  printf '%s\n' "$out" | grep -qxF "$line" || {
    printf 'docs/cli.md has drifted from the tool; this line is not in its output:\n  %s\n' "$line"
    exit 1
  }
done <<'LINES'
planted-lie      with-planted               planted lie corroborated   bug rests on the lie
planted-lie      believed                   killed by m1-ignores-position (from the cases)
planted-lie      m1-ignores-position        alive from the cases only (the seal reaches its clause)
planted-lie      bug                        reached by stripped-tag
planted-lie      with-planted               cleared by removing 1 planted claim(s)
transitive-count charlie-own-lie            planted but not a control: the strip clause is not applied
attaint: 5 package(s), 29 case(s), 25 rule(s), 32 mutant(s); 3 planted lie(s) corroborated of 6, 3 cleared by removal, 3 planted-not-control; 25 rule(s) killed, 0 unmutated; 0 refusal(s)
LINES

# BOTH GUARDS BELOW ARE UNCONDITIONAL, which the first cut got wrong: the total
# sat INSIDE the loop, so emptying the quoted list made the subset check
# vacuous AND stopped the total ever running -- one list absorbing both guards
# (quill r3/16). The total now runs once rather than once per quoted line.
#
# AND THE LIST'S OWN FLOOR IS DERIVED FROM THE TABLE ABOVE, not remembered: a
# literal 6 permitted exactly the deletion it was added to refuse, because the
# list quotes one line per kind PLUS the tally and so holds seven (quill
# r3/18). Counting the table's rows makes the two convict each other -- a kind
# added to the table without a sample, or a sample deleted without the table,
# fails here. The sed range BOUNDS it to that table: a bare grep over the page
# returns 20, because the clause table matches the same shape.
kinds=$(sed -n '/^| line | what it records |/,/^$/p' docs/cli.md | grep -c '^| `')
[ "$kinds" -gt 0 ] || {
  echo 'docs/cli.md no longer tabulates the kinds of line, so this block cannot check the sample'
  exit 1
}
[ "$quoted" -eq $((kinds + 1)) ] || {
  printf 'the quoted list holds %s line(s); the table names %s kind(s), so it should hold %s\n' \
    "$quoted" "$kinds" "$((kinds + 1))"
  exit 1
}

got=$(printf '%s\n' "$out" | grep -vc '^attaint:')
[ "$got" = "$want" ] || {
  printf 'docs/cli.md says the run prints %s line(s) and it printed %s\n' "$want" "$got"
  exit 1
}
```

| line | what it records |
|---|---|
| `planted lie corroborated` | a control fired and the bug rests on the lie |
| `killed by <mutant> (from the <path>)` | a rule some mutant moves, naming which and on which path |
| `alive from the cases only` | a mutant inert on the ledger because the seal reaches its clause — R2.3's equivalence set, measured |
| `reached by <case>` | a finding the package can still produce |
| `cleared by removing N planted claim(s)` | the strip's second direction: the bug went when the lie did |
| `planted but not a control` | a case carrying a planted claim the strip clause is **not** applied to — printed so the narrowing below is visible rather than asserted |

**The strip has two directions and the contract states both.**
`docs/vocabulary.md` under `planted`: with the *tag* removed the same run must
still produce the same `bug` (the committed `stripped-tag` case and drill `d3`),
and with the planted claims *removed altogether* the run must be `consistent`.
The second is what proves the bug was the lie rather than an artifact of the
statute. The counterfactual is **derived**, not committed — a further case
under `examples/planted-lie` would move the gated 5/28/32 — so attaint rebuilds
the sealed prefix without the planted claims (and without any tell that passed
one, since the store refuses a tell whose claim is gone) and re-evaluates.

It applies to cases that **declare themselves corroboration controls**. Three
cases in `examples/` carry a planted claim incidentally — role-visibility is
testing visibility, transitive-count knowledge gaps — and the vocabulary's two
uses of `planted` are both "checks on the engine", so the clause describes a
fixture using the lie as its control. Those three are **counted and printed**,
never silently skipped, so the narrowing is visible to a reader.

**Both paths count for the mutation pass.** R2.3 established that three mutants
are equivalent on the ledger path, because the sealed prefix leaves their
position guard nothing to exclude (`tests/engine`, declared by name). Measuring
only the ledger would report the rules those mutants attack as unmutated —
blaming the mutant set for a property of the bound. A rule is load-bearing if
some mutant moves it on some path, and the path is named so a rule killed only
from the cases is visible as such.

The tally rides on every path, pass or refusal: zero refusals over nothing
examined is the failure this command exists to make impossible to print.

Exit codes, the same three the rest of the CLI uses:

| code | meaning |
|---|---|
| 0 | every arm passed |
| 1 | an arm refused, or the tree could not be read |
| 2 | usage error |

### The drills

`tests/attaint/drills/` holds fable's drill designs (`thread:breakout-r3/0`,
D4 split in two at `/2`) as fixtures. Each is the pristine `examples/` tree with
one file replaced, and each directory holds only the file it changes, so the
directory is the diff. The epic's done-when is three of them — delete, weaken,
strip:

| drill | damage | the arm it trips |
|---|---|---|
| `d0-control` | none | passes |
| `d1-delete` | the clause yielding `explained` is removed | `finding-the-package-can-no-longer-produce` |
| `d2-weaken` | the honest count stops excluding planted claims | `mutant-the-statute-has-absorbed` |
| `d3-strip` | the `planted` tag leaves the ledger | `bug-does-not-rest-on-the-planted-claim` |
| `d4-never` | the lie is tagged and declared, and no observation reaches it | `checker never fired` |
| `d5-no-control` | no case declares a corroboration control at all | `no-case-declares-a-corroboration-control` |

Each trips **exactly one** arm, which the harness asserts alongside the clause.

The last three are one distinction in three parts, and collapsing any two of
them would lose it: d3 is the control **evaluated and not passing**, d4 the
control **armed and never evaluated**, d5 **no control declared**. A fixture
nobody observes must not read as a statute that stopped attributing.

```
go test ./tests/attaint/ -v
```

## `rumor fake-adapter`

```
rumor fake-adapter [--surface FILE] [--contract VERSION] [--fail KIND] [--omit KIND.FIELD]
```

The fake adapter (R4.1, issue 62): it speaks the whole wire protocol
(`docs/contract.md`) against nothing. Requests arrive as JSON-lines on stdin and
answers leave on stdout, one per line; **stderr is the only place this
subcommand writes prose**, because its stdout is the wire and a tally line there
would be a message the engine's decoder has to refuse.

Every answer is recorded rather than computed -- the declared surface, one
response per action, one value per observation, one token. An adapter that
computed its answers would be a second implementation of the system under test,
and a run against it would prove something about the fake.

**A SUBCOMMAND AND NOT A SECOND BINARY.** `ci.yml` builds one artifact
(`go build -o ../rumor-bin ./cmd/rumor`) and the tag pipeline builds one; a
`cmd/fake-adapter` would need both. What it would NOT have missed is coverage:
`go test`, `go vet`, `golangci-lint` and `govulncheck` all run `./...`, so a
second package would already be tested, vetted, linted and scanned (rook, coop
r4/4, correcting the reason this entry first gave).

| flag | what it does | what consumes it |
|---|---|---|
| `--surface FILE` | reads the declared surface and the recorded answers from JSON, strictly: an unknown key is refused by name | R4.2's example bindings and R4.3's permitted twin, which pin the values they validate against |
| `--contract VERSION` | answers with that contract version instead of the engine's | fable's D6 drill: a version bump on one side of three, refused naming all three |
| `--fail KIND` | answers every request with an error envelope of that kind | issue 62's "speaks the error envelope when asked to"; the kinds are in `docs/contract.md` |
| `--omit KIND.FIELD` | deletes one required field from one kind's response | fable's D7 drill: the decoder refuses the field by name, and validate reports could-not-run rather than a refusal |

Without `--surface` the built-in surface answers: two API actions, an `api` and
a `direct` observation, one stateful component, no capabilities. It is complete
by construction -- every declared action has a recorded response and every
declared observation a recorded value -- because a drill copies it and changes
one thing, and an incoherent starting world would make every drill pass for the
wrong reason.

| exit | meaning |
|---|---|
| 0 | stdin reached EOF and every answer was written |
| 1 | an answer could not be written, or stdin could not be read |
| 2 | usage error |

A malformed request is **answered**, not fatal: it becomes an error envelope
whose `field` is the decoder's own, because an adapter that exited would make
the engine read a transport failure where the fault was one message.

```sh run
# THE SECOND `run` BLOCK IN THIS FILE, and it checks the table above against the
# binary rather than describing it. The toolchain preamble is repeated rather
# than shared because the gate executes each block in its own shell
# (release-gate.py's run_doc_blocks); the ARGUMENT for resolving the name and
# pinning GOTOOLCHAIN is in the attaint block above and is not restated here.
GO=""
for c in go go126 go125; do
  command -v "$c" >/dev/null 2>&1 && { GO="$c"; break; }
done
test -n "$GO" || {
  echo "no go, go126 or go125 on PATH: this box has no Go toolchain, which is a PROVISIONING finding and not a code one"
  exit 1
}
export GOTOOLCHAIN=local

# BUILT ONCE, OUT OF TREE, AND RUN FIVE TIMES. `go run` per case would relink
# per case inside this arm's 60s bound, and a build INTO the tree would leave an
# unmapped path that `docs --check` refuses -- which is how run 142 went red on
# `rumor-bin`.
tmp=$(mktemp -d) || exit 1
trap 'rm -rf "$tmp"' EXIT
"$GO" build -o "$tmp/rumor" ./cmd/rumor || exit 1

# THE VERSION IS READ FROM THE CONSTANT, not written here. Three places already
# state the contract version -- the engine, the adapter and platform.toml -- and
# a fourth copy inside a doc block is the copy that goes stale unnoticed.
ver=$(sed -n 's/^const ContractVersion = "\(.*\)"$/\1/p' internal/wire/wire.go)
[ -n "$ver" ] || {
  echo "internal/wire no longer declares ContractVersion as a quoted literal, so this block cannot build a request"
  exit 1
}

describe="{\"kind\":\"describe\",\"contract\":\"$ver\",\"id\":\"d1\"}"
execute="{\"kind\":\"execute\",\"contract\":\"$ver\",\"id\":\"e1\",\"action\":\"create_event\",\"args\":{},\"token\":\"t\"}"

out=$(printf '%s\n' "$describe" | "$tmp/rumor" fake-adapter) || exit 1
for want in '"kind":"describe"' '"capabilities"' '"contexts"'; do
  case "$out" in
    *"$want"*) : ;;
    *) echo "the fake adapter's describe answer does not carry $want: $out"; exit 1 ;;
  esac
done

bumped=$(printf '%s\n' "$describe" | "$tmp/rumor" fake-adapter --contract 99) || exit 1
case "$bumped" in
  *'"contract":"99"'*) : ;;
  *) echo "--contract did not reach the answer: $bumped"; exit 1 ;;
esac

# --omit, BOTH WAYS. The deletion alone would also pass for an adapter that
# never answered `ids` at all, so the control is the same request without it.
omitted=$(printf '%s\n' "$execute" | "$tmp/rumor" fake-adapter --omit execute.ids) || exit 1
control=$(printf '%s\n' "$execute" | "$tmp/rumor" fake-adapter) || exit 1
case "$omitted" in
  *'"ids"'*) echo "--omit execute.ids left ids in the answer: $omitted"; exit 1 ;;
esac
case "$control" in
  *'"ids"'*) : ;;
  *) echo "the unomitted answer carries no ids either, so the check above proves nothing: $control"; exit 1 ;;
esac

# THE FLAG TABLE AND THE BINARY'S USAGE CONVICT EACH OTHER, in both directions:
# a flag documented and not implemented fails here, and so does one implemented
# and not documented. The usage text is the binary's own, read off a refusal.
usage=$("$tmp/rumor" fake-adapter --nonesuch x 2>&1 >/dev/null)
# SCOPED TO THIS SUBCOMMAND'S LINE. The usage text carries every subcommand, so
# the reverse direction below read `--examples` off `rumor check` and refused --
# correctly, for a flag that is not this table's to document.
fakeusage=$(printf '%s\n' "$usage" | sed -n 's/.*rumor fake-adapter //p')
[ -n "$fakeusage" ] || {
  echo "the binary's usage no longer carries a `rumor fake-adapter` line, so neither direction below can be checked: $usage"
  exit 1
}
documented=$(sed -n '/^## `rumor fake-adapter`/,/^## Subcommands/p' docs/cli.md |
  sed -n 's/^| `\(--[a-z-]*\)[ `].*/\1/p')
n=0
for f in $documented; do
  n=$((n + 1))
  case "$fakeusage" in
    *"$f"*) : ;;
    *) echo "docs/cli.md documents $f and the binary's usage does not name it: $fakeusage"; exit 1 ;;
  esac
done
[ "$n" -ge 1 ] || {
  echo "no flag rows found in the fake-adapter table, so the loop above examined nothing"
  exit 1
}
for f in $(printf '%s\n' "$fakeusage" | tr ' ' '\n' | sed -n 's/^\[\(--[a-z-]*\)$/\1/p'); do
  case " $(printf '%s ' $documented)" in
    *" $f "*) : ;;
    *) echo "the binary's usage names $f and docs/cli.md's flag table does not"; exit 1 ;;
  esac
done
```

## Subcommands

`docs --check` (R0.1), `check` (R2.3), `validate` (R4.3), `attaint` (R3.1) and
`fake-adapter` (R4.1). Everything else arrives with the phase that owns it.

## Flags

Not yet written.
