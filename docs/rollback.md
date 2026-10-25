# Rollback

> Stub from R0.1. The headings below are the ones this document owes; the
> content arrives with the phase that owns the subject. `docs/coverage.toml`
> names this file, and `rumor docs --check` fails if a source path it should
> describe is not mapped.

R5.2 filled the four headings it can answer, because the shrinker (R5.3) cannot
consume a provider that does not exist and a provider cannot be written against
an interface nobody wrote. **R13 owns this document**, and the shape below is
the minimum R5 needed rather than the definition R13 will give: the verb names
are issue 73's own, so a widening should find them already right.

## Provider interface

Five verbs over a world that can be put back (`internal/rollback`):

| verb | answers |
|---|---|
| `Describe` | the provider's name, the components it **covers**, and one sentence on its mechanism |
| `Checkpoint` | save the world; return the handle that puts it back |
| `Restore` | put the world back to a handle. It does **not** promise the world is clean afterwards |
| `Verify` | **is** the world this checkpoint? `nil` proven clean, `ErrDirty` proven otherwise, any other error means the provider could not tell |
| `Discard` | forget a handle and release what held it |

**Beyond the five verbs, a provider that takes a resource owes a way to give it
back.** These are not part of the trait -- an engine drives the five above and
nothing else -- but a caller that opened a provider needs them, and
`toy-process-restart` has both:

| method | answers |
|---|---|
| `Close` | stop the process the provider started. It leaves the provider's files alone, which is what lets a caller ask `Verify` about a world whose server is gone |
| `RemoveWorld` | stop the process **and** remove the working directory `open` made. The world outlives the process on purpose, so this is the separate act of being finished with the world itself |

`Discard` releases what one checkpoint held; `RemoveWorld` releases what the
provider held. Neither implies the other, and a caller that opens a world is
the one that knows which it wants: `tests/shrink` and the provider's own test
helper call `RemoveWorld`, because a test that leaves one directory per run
leaves hundreds in a day (issue 88).

A `Checkpoint` is opaque to its caller: what a provider needs to put a world
back is the provider's business, and an engine that read into it would be
coupled to one mechanism.

Every verb takes a `context.Context`. A restore can hang — restarting a process
that will not die is the ordinary case — and a shrink that cannot be bounded is
one that stops the run.

**A provider is named, not imported.** `platform.toml`'s
`[[rollback]].provider` carries a name, and `rollback.Open` resolves it against
a registry a provider adds itself to. The engine cannot import a project's own
Go code, so the registry is where a shipped provider arrives; registering two
under one name panics rather than letting import order decide.

## Coverage

`[[rollback]].covers` names components, and the components are the adapter's:
`describe` reports each with `stateful`, and a `direct` observation in
`bindings.toml` names the one it reads. `rumor validate`'s rollback-coverage arm
pairs the three — a stateful component no provider covers is refused, which is
what makes `covers` more than a label.

## Verify

**Verify is not optional, and that is the design rather than a caution.**
`docs/design.md`: "a shrink step run on a world the previous step dirtied would
attribute to the trace what the leftover state did, and the 'minimal' trace
would be minimal only in that world." So the shrinker runs every candidate
against a **verified** clean rollback, and `Verify` is the thing that earns the
word.

Three answers, and the third is not the first:

- `nil` — proven clean.
- `ErrDirty` — proven **not** clean. A shrink step refuses by name on this.
- anything else — the provider could not tell. **Not** clean, and reported as
  could-not-run rather than as a verdict, the same separation `rumor validate`
  draws between a refusal and a run that never happened.

**A verify that checks one thing can pass over a corpse.** The shipped provider
asks two questions: the state file is byte-for-byte the checkpoint, *and* the
server answers. The first alone is satisfied most perfectly when nothing is
running — a dead application leaves a flawless file — so the second is what
makes the first mean a world a shrink step can run in.

**Byte-for-byte, and an absent world is dirty.** Two parts of that sentence had
no arm until a mutation sweep asked for one: a comparison of LENGTHS passed
every test in the package, because the only moved world any arm built had an
extra record in it and so differed in length as a side effect; and a missing
state file reads as an empty world, which is `ErrDirty` against any checkpoint
that is not itself empty. Both are pinned now —
`TestVerifyIsDirtyOnADifferenceOfTheSameLength` and
`TestVerifyIsDirtyWhenTheWorldIsGoneAndTheCheckpointIsNot`.

## Clone

Not yet written. R13's: a provider that clones a world rather than restoring
one in place, for a shrink that wants candidates in parallel.

## Shipped providers

| name | covers | mechanism |
|---|---|---|
| `toy-process-restart` | `store` | stops `cmd/toy`, copies the saved state file back over the live one, starts it again |

**Why a restart and not a truncate.** The toy's whole world is one file, so
putting it back is a copy — but the running process holds that world in memory
too, and a copy under a live server leaves the two disagreeing. Restarting is
what makes the file authoritative: the toy loads it at startup and holds nothing
else, sessions included.

**It owns the process.** A restart provider cannot restart a library, and it
cannot restart a process somebody else started — so the provider starts the toy
and hands out the address it bound, which is what `cmd/toy-adapter` reads from
`TOY_ADDR`. Under the engine the provider owns the application; `rumor validate`
is the case where nobody does, and there the adapter starts one of its own.

**An empty world is a world.** A checkpoint taken before anything happened
restores to an **absent** file rather than an empty one: the toy treats a
missing file as a fresh start, and a zero-byte file where JSON is expected is
something a strict decoder is right to refuse.
