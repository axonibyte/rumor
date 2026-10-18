# Contract

This document is the contract: what an adapter speaks, what a project declares
itself with, and the version both are held to. R4.1 wrote the wire protocol and
the `platform.toml` schema; the bindings schema arrives with R4.2, and
`rumor validate` (R4.3) is what refuses a project whose three declarations do
not agree. `docs/coverage.toml` names this file, and `rumor docs --check` fails
if a source path it should describe is not mapped.

**Every field below exists because something consumes it, and the consumer is
named.** That is not a style rule. A field with no consumer is either dead or a
channel for an adapter to answer a question the engine did not ask -- rook
found one in the first draft of this protocol (`observe`'s response echoed the
source it had been handed), and applying the same test to the rest of the draft
took the run-local handles off the wire entirely.


## Adapter wire protocol

### Transport and framing

JSON-lines over stdio: one JSON object per line, requests on the adapter's
stdin, responses on its stdout. **The wire client spawns the adapter** from
`[adapter].command` in `platform.toml`, sends `describe` first, and keeps the
process for the whole run.

**THE CLIENT AND NOT THE ENGINE, as of R4.3** (fable, coop r4/74). This said
"the engine spawns" until `rumor validate` needed a `describe` of its own: the
capability arms and the contract-version arm can only be answered by ASKING the
adapter, so validate owns the first spawn in this project. It owns it as ONE
client in `internal/wire` that R5's engine reuses, rather than a validate-only
spawner -- two spawners would drift, and the second would be written by whoever
needed it next without the reasons the first had. validate asks `describe` and
nothing else, and keeps the process for one exchange rather than a run.

**One request is outstanding at a time per process.** Responses therefore
arrive in order, and `id` is a check rather than a router: a response whose
`id` is not the outstanding request's is a wire refusal naming both. Where a
stage needs concurrency (R8's N-way fan-out), it uses N adapter processes
rather than N outstanding requests on one, because the engine owns the timing
and a pipelined adapter would own part of it.

The adapter's stderr is free text. It is captured into the run's report and
never parsed; a diagnosis reads it (R10, R11) and no decision turns on it.

EOF on stdout, or a process exit, before the run ends is **could-not-run**,
with the exit status in the detail. It is never a refusal and never a finding.

### Fields every message carries

| field | type | consumer |
|---|---|---|
| `kind` | string | the decoder: it selects which required-field set applies, so the message's shape is read from the message itself rather than inferred from what was last sent |
| `contract` | string | the version check below. On **every** message, not only the handshake, so a mid-run change is caught at the next message rather than at startup alone |
| `id` | string | correlation, as above: the response to the outstanding request, and nothing else |

### Strict decode, refused by name

The decoder refuses a message with an **unknown** field or a **missing**
required one, naming the message kind, the field, and which of the two it was.
Unknown-is-an-error rather than unknown-is-ignored follows `docs/coverage.toml`'s
parser and the reason given there: a lenient reader turns a typo into silence.
Here the silence would be worse than an uncovered path -- an adapter sending
`sorce` would observe nothing and the run would read as clean.

A decode refusal is **could-not-run** (`rumor validate` exit 2), never
*refused* (exit 1). The project is not wrong; its adapter is. That boundary is
`rumor attaint`'s (R3) and fable's D7 drill asserts it.

### `describe`

**Request, beyond the envelope:** nothing. There is nothing to ask.

**Response, beyond the envelope:**

| field | type | consumer |
|---|---|---|
| `actions` | array of `{name, context}` | validate's arm on an action with no UI flow needs the action set; the arm on a browser-stage actor reaching an API action needs each action's `context`, because "could reach" is exactly "its context is one that actor holds a session in" |
| `observations` | array of `{name, source}` | the engine's pre-run check: a bindings observation the adapter does not declare is could-not-run before any actor moves, rather than mid-run. This is the engine's check and **not** one of R4.3's thirteen refusals |
| `contexts` | array of strings | `session`'s `context` must name one of these, and the reachability arm above is computed over them |
| `components` | array of `{name, stateful}` | the arm on a stateful component no rollback provider covers compares these against `[[rollback]].covers` in `platform.toml`. Without `components` that arm has nothing to compare and would pass vacuously |
| `capabilities` | object: `{faults, volume, upgrade}`, three arrays of names | the arm on a declared fault, volume target or upgrade with no capability or path behind it. Each array may be empty, and an empty array is the answer that makes the arm fire rather than an omission that makes it vacuous |

`context` on an action is a context name or `"none"` for an action taken
without a session.

**WHY `capabilities` IS HERE AND NOT IN `platform.toml`** (fable, coop
breakout-r4/3, whose D13 drill reads it from `describe`): the arm asks whether
there is anything *behind* a declaration. A second static file saying
`faults = ["disk_full"]` is another declaration, not something behind the first,
so pairing the two would prove only that a project wrote one word twice.
Pairing a declared use against the adapter's live answer is a check. The same
reasoning runs the other way for rollback: a provider is `rumor`'s own
machinery and the project merely selects it, so `[[rollback]]` stays in
`platform.toml` and is paired against `components` here -- live against static
in both cases, with the live side being whichever party has to do the thing.

### `execute`

**Request, beyond the envelope:**

| field | type | consumer |
|---|---|---|
| `action` | string | one of `describe`'s action names |
| `token` | string | present **exactly when** the action's context is not `"none"`: the session the action is taken in, which is also who takes it. There is no `actor` field here -- the engine knows which actor it acquired the token for, and a second statement of the same fact is a channel for the two to disagree |
| `args` | object | the action's declared arguments (R4.2). Values are the system's own ids, never handles: see below |

**Response, beyond the envelope:**

| field | type | consumer |
|---|---|---|
| `ids` | object, declared name to id | the engine binds each to the handle it is assigning. Keys are the names the action declares it creates, not handles |
| `response` | any | the bindings' declared response shape, and the read-back an `api` observation checks |

**No run-local handle crosses the wire, in either direction.** The engine owns
the handle table (`docs/design.md`: it "maps each to whatever id the server
returned on the current attempt"), so the engine resolves a handle to an id
when it builds `args`, and joins an id back to the handle it was assigning when
it reads `ids`. An earlier draft keyed `ids` by handle; keying them by the
action's declared names is the same rule as rook's finding -- the adapter was
being handed an identifier it does nothing with, and could answer with a
different one. An action that creates two entities is why the keys exist at all:
a bare id could not say which was which.

### `observe`

**Request, beyond the envelope:**

| field | type | consumer |
|---|---|---|
| `query` | string | the named observation. When `source` is `direct` its verb shape is an **epic Open item**; the decoder checks that `query` is present and leaves its shape alone until R8 fixes it |
| `source` | string | one of `api`, `ui`, `direct` (`docs/vocabulary.md`, Observation sources) |
| `token` | string | present **exactly when** `source` is not `direct`: `api` and `ui` are an actor's observations through its session, and `direct` is the engine's, never an actor's (R9) |

**Response, beyond the envelope:**

| field | type | consumer |
|---|---|---|
| `value` | any | what was observed. The ledger writes it as an `observed` fact at the position it assigns, with the source the engine asked for |

**No position.** The ledger assigns positions on append (`docs/design.md`:
"Positions are assigned by the store, not passed in"), and
`docs/vocabulary.md` makes an observation with no position a validation
refusal -- which reads as "the adapter must send one". It must not: an adapter
that supplied positions would own append-only ordering, which the ledger owns.

**And no source.** The engine records the source it *asked* for; the request is
the only place a source is chosen. An adapter that cannot observe through the
channel it was handed answers the error envelope with `kind: "source"`, which
is could-not-run. It must not answer through another channel, because a
substituted source would record a `ui` rule's run against an `api` read and the
report would name a stage that never ran.

### `session`

**Request, beyond the envelope:**

| field | type | consumer |
|---|---|---|
| `actor` | string | the adapter acquires the session as this actor. The name is the one the ledger speaks, not a handle |
| `context` | string | one of `describe`'s contexts |

**Response, beyond the envelope:**

| field | type | consumer |
|---|---|---|
| `token` | string | every later message for that actor carries it in place of the actor's name |

`actor` enters the protocol here and only here. Every later message carries the
token instead, which is what makes the browser stage's one-isolated-context-per-actor
rule (R9) a property of this exchange rather than an honor system.

### The error envelope

Any response may be replaced by `kind: "error"`.

**Response, beyond the envelope:**

| field | type | consumer |
|---|---|---|
| `error` | object: `{kind, field?, detail}` | the run's could-not-run report, and the six kinds below |

The six kinds:

| `error.kind` | when | consumer |
|---|---|---|
| `contract` | the version the peer was sent does not match its own | validate's contract-version arm; fable's D6 drill bumps the adapter's alone |
| `field` | the peer could not decode a message: `field` names it, `detail` says unknown or missing | the decoder's own refusals, and D7 |
| `source` | the adapter cannot observe through the source it was handed | `observe` above: the alternative is a substituted source |
| `session` | a session could not be acquired, or a token was rejected | the run cannot proceed as that actor |
| `action` | the adapter does not implement a named action | the engine's assertion against its own `describe` answer: reaching this means the engine asked for something the adapter never declared |
| `internal` | anything else; `detail` is free text | the run's could-not-run report |

**Every envelope is could-not-run.** An adapter cannot refuse a project and
cannot produce a finding: a refusal is `rumor validate`'s verdict over declared
files, and a finding is the statute's over a sealed ledger. The adapter reports
what it saw, or that it could not act.

### The version check

Three places state the contract version: the engine's own constant, the
adapter's `contract` field, and `contract` in `platform.toml`. **All three must
be equal, by exact string comparison.** v1's value is `"1"`, there is no
compatibility window, and a mismatch is refused naming every value seen and
where each came from -- a refusal that names one side leaves the reader to
guess which of the other two it disagreed with.

The version is a string rather than a number so that a later form compares
exactly without numeric coercion deciding whether `1` and `1.0` are the same
version.

### What an adapter never does

Each of these is owned elsewhere, and an adapter that did it would make a run
irreproducible or a report untrue:

- **assign a position.** The ledger assigns, on append (`docs/vocabulary.md`).
- **draw randomness.** The engine owns the seed and the action sequence
  (`docs/design.md`); a driver that drew its own would make replay depend on
  something the seed does not carry.
- **substitute an observation source.** See `observe`.
- **see a handle.** The engine owns the handle table.
- **report a finding, or refuse a project.** Neither is the adapter's verdict
  to give.


## Bindings schema

What a project binds its statute to: one `bindings.toml` per package, beside
the statute it binds. The wire protocol above says what an adapter can be
asked; this says what this project's actions, observations and claims ARE, in
the vocabulary's words (`docs/vocabulary.md`: `of`, `role`, `visible_to`, and
`ref` is a handle).

**EVERY FIELD IS DERIVED FROM THE THREE EXAMPLE PACKAGES' OWN CASES, not
invented.** What they speak: `planted-lie` emits `user.created` and observes
`user.count` with source `api` and `of = user.created`; `role-visibility` emits
`doc.created` carrying `visible_to` and observes `doc.count`;
`transitive-count` emits `user.created`, observes `user.count` and carries
tells. Each case's observation holds an actor, a position, `of`, a query, a
source and a value; each claim holds `ref`, an actor, a type, a position and a
handle, with `ref` and the handle equal.

```toml
[[action]]
name = "create_user"           # one of `describe`'s action names
context = "api"                # a context name, or "none"
emits = ["user.created"]       # claim types this action appends
requires = ["can_create"]      # preconditions AS STATUTE REFERENCES: rule names
creates = ["user"]             # the keys `execute`'s `ids` comes back under
response = { status = 201 }    # the expected response shape

[[observation]]
name = "user.count"            # `observe`'s `query`
source = "api"                 # api | ui | direct
of = "user.created"            # the claim type it is judged against

[claim."user.created"]
timing = "immediate"           # or "eventual", which then needs a bound

[[role]]
name = "host"
contexts = ["api"]             # the session contexts an actor of this role may hold
```

### The keys, and the arm each one answers to

| key | required | consumer |
|---|---|---|
| `[[action]].name` | yes | `execute`'s `action`; the adapter must declare it in `describe` |
| `[[action]].context` | yes | the arm on a browser-stage actor reaching an API action pairs this with `[[role]].contexts` |
| `[[action]].emits` | yes | the arm on a rule referencing a claim type no binding emits, and its mirror on a binding emitting a type no rule consumes |
| `[[action]].requires` | yes | the arm on an action with no precondition. Rule NAMES, because the engine evaluates preconditions against the sealed ledger (`docs/design.md`) and a prose condition could not be evaluated at all |
| `[[action]].creates` | no | the keys of `execute`'s `ids`. Absent for an action that creates nothing |
| `[[action]].touches` | no | the stateful components this action MUTATES, which is the other half of the rollback-coverage arm. ibis found the hole (coop r4/23): a component was nameable only on a `direct` observation, so an action that dirties an uncovered component and is never read directly fell outside an arm whose wording is about bindings that TOUCH one -- and the arm then passed a project it should refuse |
| `[[action]].response` | no | the response shape an `api` observation reads back |
| `[[observation]].name` | yes | `observe`'s `query` |
| `[[observation]].source` | yes | one of the vocabulary's three |
| `[[observation]].of` | yes | the claim type the observation is judged against -- the cases carry `of` in the observation, and `internal/ledger` puts `Of` beside Query, Source and Value on the `observed` fact (ibis, coop r4/20) |
| `[[observation]].component` | only for `source = "direct"` | the arm on a stateful component no rollback provider covers: a `direct` observation reads the project's own store, so it is the binding that NAMES a component, which is then paired with `describe`'s `components` and `platform.toml`'s `[[rollback]].covers`. fable's D11 drill removes the provider and expects the component named |
| `[claim.<type>].timing` | yes, per emitted type | `immediate`, or `eventual` with a bound. The arm on an eventual type with no bound reads this |
| `[claim.<type>].bound` | only when `timing = "eventual"` | a duration (`30s`) or `"quiesce"`. The second half of that arm refuses eventual claims with no quiesce probe declared |
| `[[role]].name`, `.contexts` | yes | the reachability arm above. A role is the vocabulary's `role` |

**WHAT AN ACTION DECLARES IT TOUCHES IS A CLAIM THE ENGINE CANNOT CHECK**, and
saying so is the price of making `touches` optional: an action naming nothing is
read as mutating nothing, exactly as an action with no `requires` is read as
having no precondition -- the difference being that arm 3 refuses the second and
no arm refuses the first. A nineteenth arm is not ours to add (the count is
fable's, ruled at eighteen in coop breakout-r4/6), so the hole is written down
here instead of being closed by invention. `component` on an observation stays
what it is: what a `direct` observation READS.

**THE OBSERVATION DECLARES NO POSITION, and that is settled rather than
omitted** (ibis, coop r4/20): the arm on an observation lacking a position or a
source is static over the **cases'** declared observations, which carry both --
17 of them across the three packages, none missing either field -- and D16
strips one field from one of those. A position in this file would be a fourth
place the same fact is stated, and the adapter must not send one either (see
`observe` above).

### UI actions

An action whose context is a browser context declares a **flow** instead of a
response shape: the steps the driver executes (R9).

```toml
[[action]]
name = "create_doc"
context = "browser"
emits = ["doc.created"]
requires = ["can_create"]

  [[action.flow]]
  step = "navigate"
  to = "/docs/new"

  [[action.flow]]
  step = "fill"
  field = "title"
  value = "quarterly"

  [[action.flow]]
  step = "click"
  target = "submit"

  [[action.flow]]
  step = "wait"
  until = "doc-created"
```

| step | fields | why |
|---|---|---|
| `navigate` | `to` | the path, never a full URL: the base belongs to the platform's adapter, not to a binding |
| `fill` | `field`, `value` | |
| `click` | `target` | |
| `wait` | `until` | **a condition and never a duration.** R9's flake policy is explicit that there are no fixed sleeps, so a `wait` step carrying a duration is refused by name rather than slept through. An action that fails its wait condition is retried once at the same position and then recorded as a `ui_timeout` finding (R9), which is a different outcome from a refusal here |

The arm on a browser-stage action with no UI flow reads `[[action.flow]]`; it is
what makes "the browser stage is enabled" more than a flag.

### Reserved for the phase that owns them

Present in the schema now, **accepted when absent and refused when malformed**,
which is issue 63's own rule:

| table | phase | why it is named now |
|---|---|---|
| `[clock]` | R7 | the vocabulary's Time section puts the test clock's declaration in the bindings, and the arm on a rule reading `clock` with no clock control refuses on this table's absence -- so it has to live here to be absent from here |
| `[quiesce]` | R6 | the second half of the eventual arm: eventual claims with no quiesce probe declared |
| `[deadline]` | R7 | deadline fields arrive with controlled time |
| `[seeding]` | R20 | a declared seeding path, which R20 may also reach through an action |
| `[uses]` (`faults`, `volume`, `upgrade`) | R19, R20, R21 | the arm on a declared fault, volume target or upgrade with no capability behind it. This table is the project's ASK; `describe`'s `capabilities` is what the adapter can actually do, and the arm pairs them -- live against static, which is why neither side is in the other's file |


## `platform.toml` schema

A project's own declaration: which adapter to run, who the run draws its actors
from, which stages exist, how the walk is weighted, and where its seeds live.
The bindings (R4.2) say what the actions *are*; this file says what this
platform *is*.

The parser is strict in both directions, for `docs/coverage.toml`'s reason: an
unknown key is an error naming the key and its table, and a missing required
key is an error naming it. So the parser's key set **is** the schema, and a
later phase adding a key adds it here with a Revisions entry.

```toml
contract = "1"                      # matched against the adapter's and the engine's
seeds_file = "seeds.lock"           # default; R5 promotes a seed into it

[adapter]
command = ["rumor", "fake-adapter"] # argv the engine spawns
cwd = "."                           # optional
env = ["BASE_URL"]                  # optional: variable names passed through

[[venire]]                          # the cast the run draws its actors from
role = "host"                       # a role the statute speaks (docs/vocabulary.md)
count = 1

[[venire]]
role = "guest"
count = 3

[stages.api-sim]
enabled = true

[stages.fuzz]
enabled = false

[stages.concurrency]
enabled = false

[stages.browser]
enabled = false

[weights.default]                   # action kind to weight; nemesis kinds are
create_event = 5                    # action kinds in the same pool
join_event = 8
suborn = 1

[weights.stage.browser]             # sparse override
join_event = 3

[weights.profile.nightly]           # sparse override, wins over the stage's
suborn = 3

[profiles.pre-push]
stages = ["api-sim"]
seeds = 4                           # a count, as `rumor run --seeds N` (R22)
budget = "5m"

[profiles.nightly]
stages = ["api-sim", "concurrency", "browser"]
seeds = 64
budget = "2h"
```

### The seeds file

`seeds_file` names the file R5 promotes a seed into, and this is its **minimal**
format, fixed in R4 so `rumor validate`'s arm on a promoted seed has something
to read (fable, coop r4/74). R5 may EXTEND it and may not contradict it.

```
17                      # a promoted seed, on its own line
42  # overcounted after a late tell
```

- **One seed per line**: a non-negative integer, optionally followed by `#` and
  a comment naming the defect it found.
- **Blank lines are ignored**, and so are lines whose first non-space character
  is `#`. A comment allowed AFTER a seed and refused on its own line would be a
  distinction with nothing behind it, and a lock file with no room for a header
  cannot explain itself -- `examples/twin/seeds.lock` is exactly that case, a
  file whose only content is why it is empty.
- **Empty is a state, not a placeholder**: no seed has found a failure yet. The
  file's presence is what arm 10a reads; its contents are what 10b reads.

A line that is neither ignorable nor a seed is **refused by arm 10b, naming the
line** -- a promoted seed that cannot be read is a promoted seed missing, and a
run that silently skipped it would replay less than it reported.

### The keys

| key | required | consumer |
|---|---|---|
| `contract` | yes | the three-way version check above |
| `seeds_file` | no, defaults to `seeds.lock` | validate's arm on a missing seeds file or promoted seed; R5 writes promotions into it. Named `seeds_file` and not `seeds` because a profile's `seeds` is a **count** -- one word for a path and a count in one file is a misreading waiting to happen |
| `[adapter].command` | yes | the engine spawns it. Nothing else in the project says how to reach the system under test |
| `[adapter].cwd`, `[adapter].env` | no | the spawn; `env` is a list of variable **names** passed through, so a platform's secrets stay out of this file |
| `[[venire]]` | yes, at least one | the walk draws its actors from these roles and counts. An empty venire can run nothing, which is a parse refusal rather than one of validate's thirteen |
| `[stages.<name>].enabled` | yes, at least one enabled | stage selection and skip reasons in the report (R8); `stages.browser.enabled` is what the two browser-stage arms read, and both are vacuous without it |
| `[weights.default]` | yes | the walk draws from the applicable set by weight (`docs/design.md`, "the engine owns the seed") |
| `[weights.stage.<stage>]`, `[weights.profile.<profile>]` | no | the same, per stage and per profile, which is the shape design.md already relies on |
| `[profiles.<name>]` | no | R22's profiles: `stages`, `seeds` (a count), `budget` (a duration) |

**Weight resolution order, stated because an unstated precedence is where a
tuned weight silently does nothing:** for an action kind in stage *s* under
profile *p*, the weight is the first of `[weights.profile.p]`,
`[weights.stage.s]`, `[weights.default]` that names it. Overrides are sparse;
`[weights.default]` must be **complete**, so an override naming an action kind
the default does not is a parse refusal naming the kind. The reason is the
resolution above: a kind only an override names has no weight at all in any
stage or profile that does not override it, which is a silent zero rather than
a declared one. A weight of `0` is legal and means never drawn; a negative one
is refused, because a weighted draw cannot express it and reading it as zero
would make the file say something it does not.

**None of this file's refusals are `rumor validate`'s arms.** A document that
cannot be read as a platform at all is **could-not-run**, and it arrives before
any of the eighteen arms evaluates. Validate's refusals are about a project
whose statute, bindings and platform disagree.

### Reserved for the phase that owns them

Present in the schema now and accepted when absent -- a **malformed** one is
refused even now, which is R4.2's rule for the same situation in the bindings:

| table | phase | why it is here |
|---|---|---|
| `[[rollback]]` (`provider`, `covers`) | R13 | the arm on a stateful component with no rollback provider needs the platform-side half now, or it has nothing to compare `describe`'s `components` against. `docs/rollback.md` defines the provider interface in R13, and the names must agree then |
| not a table: `capabilities` | R19, R20, R21 | faults, volume targets and upgrades are declared by the adapter's `describe`, not here. R4.1's first draft put them in this file; the reason they moved is under `describe` above |
| `[thresholds]` (`blocking = false` by default) | R15 | coverage thresholds are advisory until promoted; R15 names the metrics, and until then an unknown key inside the table is refused like any other |


## Revisions

| version | date | run | change |
|---|---|---|---|
| v1 | 2026-10-17 | R4.1 | first definitions. The wire protocol: JSON-lines over stdio, one outstanding request per process, `kind`/`contract`/`id` on every message, strict decode refused by name as could-not-run, the four request/response pairs, the error envelope's six kinds with every envelope could-not-run, `describe`'s declared surface including `capabilities`, the three-way version check by exact string comparison with `"1"` as v1's value, and the five things an adapter never does. No run-local handle crosses the wire in either direction, and `observe` answers `value` alone -- neither a position nor a source. `platform.toml`: `contract`, `seeds_file`, `[adapter]`, `[[venire]]`, `[stages.<name>]`, `[weights.default]` with sparse per-stage and per-profile overrides resolved profile-then-stage-then-default, and `[profiles.<name>]`; `[[rollback]]` and `[thresholds]` reserved for R13 and R15, accepted when absent and refused when malformed, with faults, volume targets and upgrades declared by `describe`'s `capabilities` rather than by this file (fable breakout-r4/3). The bindings schema arrived with R4.2 in the same run: one `bindings.toml` per package, with actions (name, context, emits, requires as statute references, creates, response), UI flows whose `wait` step takes a condition and never a duration, observations (name, source, `of`, and `component` for `direct`), what each action `touches` for the rollback arm, per-type claim timing with a bound for `eventual`, roles and the session contexts they may hold, and `[clock]`, `[quiesce]`, `[deadline]`, `[seeding]` and `[uses]` reserved for R7, R6, R7, R20 and R19/R20/R21. An observation declares no position: that arm is static over the CASES' observations, which carry one (ibis, coop r4/20) |
