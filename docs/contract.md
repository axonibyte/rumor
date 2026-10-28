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

**EOF on the adapter's STDIN is the stop signal, and the client waits a bounded
grace before killing.** An adapter that has work to do on the way out -- a child
process to reap, a temporary directory to remove -- does it when its stdin
closes, and R5.4 is where that became a promise rather than a hope. The client
had closed stdin and killed at once, so whether an adapter's teardown ran was a
race it usually lost: an adapter's cleanup is its own `defer`, and a defer does
not run under `SIGKILL`. Measured 2026-10-22 against `cmd/toy-adapter`, which
spawns the toy and reaps it on EOF -- four of eight runs orphaned the toy, and
with `timeout(1)` in front the run then hung to its bound having already printed
a complete result. The grace is `internal/wire.StopGrace`, two seconds: orders
of magnitude over what an honest teardown needs, and the kill still arrives
after it, so an adapter that ignores EOF cannot hold a run's teardown open.
An adapter may therefore rely on reaching its cleanup, and must not rely on
having longer than the grace to finish it.

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

The same boundary runs through the project PATH: a directory that is not there,
or a path that is not a directory, is **could-not-run** and the line names the
path. Exit 1 there would be a verdict about declarations nothing opened, and it
read identically to a real project missing its `bindings.toml`.

### `describe`

**Request, beyond the envelope:** nothing. There is nothing to ask.

**Response, beyond the envelope:**

| field | type | consumer |
|---|---|---|
| `actions` | array of `{name, context}` | validate's arm on an action with no UI flow needs the action set; the arm on a browser-stage actor reaching an API action needs each action's `context`, because "could reach" is exactly "its context is one that actor holds a session in" |
| `observations` | array of `{name, source}` | **arm 19's observations cell**: a bindings observation the adapter does not declare is refused before any actor moves, rather than mid-run. This row said "the engine's pre-run check" in the present tense from v0.1.19 until 2026-10-19, and nothing implemented it -- every use of the bindings' observations was arm 6, a count, the wire's own validation or the fake's echo (ibis, coop r4/105). It is one of R4.3's twenty-three arms now, and could-not-run when no adapter answers |
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
| `args` | object | the observation's **declared arguments**, present **exactly when** the observation declares any in `bindings.toml`. Values are the system's **own ids, never run-local handles**: the engine owns the handle table and resolves a handle before the message is written, exactly as it does for an execute's `args`. The decoder cannot check the "exactly when" half -- whether an observation declares arguments is in the project's bindings, and the decoder reads one message without them -- so it accepts `args` as optional and `rumor validate`, which holds both halves, pairs the declaration against the request |

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

**When the engine acquires one.** Before an actor's first action in a context,
and not before that: a run with four actors and three contexts would otherwise
open twelve sessions before its first step, most of them never used, and the
adapter's session count would be a function of the project rather than of the
walk. First use is also the only order a system under test need support -- where
sessions are keyed by user name, a session for `member_1` can exist only once
some action has created a user called `member_1`, so acquiring every session up
front would ask for all of them before anybody existed. The token is then held
**per actor per context** for the rest of the attempt, because the context is
what the token is for: reusing an actor's `api` token for its `ui` actions would
send a token the adapter issued for something else. A new attempt acquires its
own, since a second attempt is a second world. An empty `token` in the response
is a could-not-run rather than a held session: an omitted token and an empty one
are the same bytes on an execute, so the request would be indistinguishable from
one taken outside a session. And an action whose context the actor's role may not
hold -- `[[role]].contexts` -- is INAPPLICABLE rather than attempted, so no
session is acquired for it at all.

### `clock`

**Request, beyond the envelope:**

| field | type | consumer |
|---|---|---|
| `op` | string | `set` or `advance`. The variant decides how `t` reads, which is `observe`'s `source` shape rather than a field this protocol already had |
| `t` | integer | NANOSECONDS. For `set`, the instant the clock is to hold; for `advance`, the amount to move it by. One field, because the op already says which and two would let a request carry both and mean neither |

**Response, beyond the envelope:**

| field | type | consumer |
|---|---|---|
| `t` | integer | the time the clock ACTUALLY holds after the request, in the same unit. Not necessarily the time asked for: an adapter may clamp, round, or refuse to go backwards |

**The unit is the one R7.1 declared before any writer existed** -- `clock.t` is
nanoseconds from the run's own epoch, zero at the first seal, in "The run's
clock, and its unit" below. A second unit on this path is where an
off-by-a-thousand lives, and the engine's own `Bound` has carried nanoseconds
since R6 because `time.ParseDuration` yields them.

**THE VERSION DOES NOT MOVE FOR THIS REQUEST, and that is load-bearing rather
than lenient.** The version is one string three places must match by exact
comparison with no compatibility window ("The version check", below), so an
adapter built before this request must still PASS the handshake -- otherwise it
would be refused for its version and never for the thing it actually lacks, and
the by-name refusal R7.2 requires could not fire at all (rook, coop
rumor/1348). An adapter that does not implement `clock` answers the error
envelope and is refused by name for that.

**WHEN THE ENGINE SENDS IT, AND WHAT IT WILL NOT ACCEPT.** A project declaring
`form = "adapter"` has its clock set at EVERY seal -- twice a step, at the top
and again before the step's observations -- so the system under test's clock
and the ledger's fact agree about the time at the position the fact is written
for. Three answers are refused by name, in this order:

| the adapter | the engine | why the name matters |
|---|---|---|
| answers a fault, not knowing the kind | names the missing request | this is the refusal the unmoved version makes reachable; behind a bump the handshake would have refused the same adapter one phase earlier and for the wrong reason |
| answers a `t` LESS than the clock the run holds | names time running backward | replay and the overage a duration bound is judged by both rest on a ledger whose times never decrease, and that reason is the refusal's whole value -- so this is checked BEFORE the one below, which a backward `t` also trips |
| answers a `set` with a `t` that is not the one asked for | names the disagreement about the time | a `set` leaves the clock reading what was asked for. An adapter with a coarser clock would be rounding, and **nothing in this protocol lets an adapter declare a granularity** -- `describe`'s `capabilities` carries faults, volume targets and upgrades and no such field -- so the difference cannot be accounted for and is refused at all rather than within a tolerance nobody declared. WHEN a declaring mechanism exists, this is the row that relaxes to it |

**A refused answer writes no clock fact**: the seal fails and the run stops,
rather than recording the engine's ask under the name of a time the run
refused to accept.

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
be equal, by exact string comparison.** v1's value is `"2"`, there is no
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

  [[action.args]]              # one per argument `execute`'s `args` carries
  name = "name"                #   the key in `args`
  source = "actor"             #   handle | literal | draw | actor
                               #   `actor` takes no further field
  [[action.args]]
  name = "visible_to"
  source = "draw"              #   the seed draws one element of `from`
  from = [["host"], ["host", "guest"]]

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
| `[[action.args]].name` | yes, per argument | the key this argument takes in `execute`'s `args`. Declared twice in one action is refused: the request is built as a map keyed by name, so the second would silently win |
| `[[action.args]].source` | yes, per argument | where the walk gets the value, and the only four it can: `handle` (a handle KIND the engine resolves to an id), `literal` (a fixed value), `draw` (the seed draws from a declared list), `actor` (the step's own actor name). EXACTLY ONE FIELD BESIDE `name` AND `source` IS ACCEPTED, the one this source uses -- a `from` on a literal is a project saying two things about one argument, and the strict decoder refuses the unread key by name |
| `[[action.args]].of` | only for `source = "handle"` | the handle KIND, `event` and never `member_1.event#1`: a declaration naming one run's handle could not survive a second run. It is also the second way an action names a handle, so a kind nothing has bound makes the action INAPPLICABLE, exactly as an unresolvable `{kind}` in a flow does |
| `[[action.args]].value` | only for `source = "literal"` | the value, carried as decoded including `false`, `0` and `""` |
| `[[action.args]].from` | only for `source = "draw"` | the list drawn from. A LIST is required and a single-element list is accepted but a scalar is not -- the refusal is about the TYPE and not about how many values there are, since `["x"]` is itself a draw of one and is accepted: a scalar does not say whether the project means one choice or the first of several. An empty `from` is refused -- there is nothing to draw and a drawn argument cannot be omitted |
| `[[observation]].name` | yes | `observe`'s `query` |
| `[[observation]].source` | yes | one of the vocabulary's three |
| `[[observation]].of` | yes | the claim type the observation is judged against -- the cases carry `of` in the observation, and `internal/ledger` puts `Of` beside Query, Source and Value on the `observed` fact (ibis, coop r4/20) |
| `[[observation]].component` | only for `source = "direct"` | the arm on a stateful component no rollback provider covers: a `direct` observation reads the project's own store, so it is the binding that NAMES a component, which is then paired with `describe`'s `components` and `platform.toml`'s `[[rollback]].covers`. fable's D11 drill removes the provider and expects the component named |
| `[claim.<type>].timing` | yes, per emitted type | `immediate`, or `eventual` with a bound. The arm on an eventual type with no bound reads this |
| `[claim.<type>].bound` | only when `timing = "eventual"` | a duration (`30s`) or `"quiesce"`. **A duration of zero or less is refused AT THE PARSE**, in `internal/decl`'s own words -- "a duration of zero or less covers nothing" -- because the engine compares `now-claimTime > Bound` in nanoseconds, so a declared `0s` puts every disagreement instantly past its bound and a negative one makes the comparison meaningless; `internal/schedule`'s `Bound > 0` therefore reads as "a duration bound was DECLARED" rather than conflating a declared `0s` with an absent one. The second half of that arm refuses eventual claims with no quiesce probe declared |
| `[[role]].name`, `.contexts` | yes | the reachability arm above. A role is the vocabulary's `role` |

**WHAT AN ACTION DECLARES IT TOUCHES IS A CLAIM THE ENGINE CANNOT CHECK**, and
saying so is the price of making `touches` optional: an action naming nothing is
read as mutating nothing, exactly as an action with no `requires` is read as
having no precondition -- the difference being that arm 3 refuses the second and
no arm refuses the first. **This stays true for `touches` after arm 19
landed** (fable, coop r4/113, 2026-10-19): arm 19 refuses a reference to a
declaration that is not there, and what an action touches is a claim about the
WORLD rather than a name looked up in a document. The count is fable's and the
hole is written down here instead of being closed by invention. `component` on an observation stays
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
| `[clock]` | R7 | the vocabulary's Time section puts the test clock's declaration in the bindings, and the arm on a rule reading `clock` with no clock control refuses on this table's absence -- so it has to live here to be absent from here. **The unit of the fact this table enables is declared below**, by R7.1, before anything writes one. **`form` is TYPED as of R7.2** and the rest of the table is still carried, the same split R6.2 made in `[quiesce]`: the two forms are named under the unit below, a value that is neither is refused by name at the parse, and an ABSENT `form` is not a parse fault -- the engine refuses that, because the engine is the reader that cannot act on it |
| `[quiesce]` | R6 | the second half of the eventual arm: eventual claims with no quiesce probe declared (7b), plus the three probe-integrity arms R6.2 typed the probes for -- a probe with no `timeout` (7c), a probe of a kind no adapter reports (7d), and `query` declared on a kind that does not read an observation (7e, which this file's own sibling sentence asserted was refused for a phase before anything refused it). All four refuse before any actor moves, and 7b counts the TYPED probes rather than the table's keys, so a `[quiesce]` carrying only sibling keys is still a project declaring no probe |
| `[deadline]` | R7 | deadline fields arrive with controlled time |
| `[seeding]` | R20 | a declared seeding path, which R20 may also reach through an action |
| `[uses]` (`faults`, `volume`, `upgrade`) | R19, R20, R21 | the arm on a declared fault, volume target or upgrade with no capability behind it. This table is the project's ASK; `describe`'s `capabilities` is what the adapter can actually do, and the arm pairs them -- live against static, which is why neither side is in the other's file |

### The run's clock, and its unit

**`clock.t` is nanoseconds from the run's own epoch, and that epoch is zero at
the first seal.** The unit is declared here before anything writes a clock
fact, which is fable's ruling at coop breakout-r7/0: a unit settled after the
writer exists is a unit that two readers have already had to guess at, and the
engine and the adapter are the two readers.

Three things follow, and each of them is the reason for the choice rather than
a consequence of it:

- **`[claim.<type>].bound`'s duration form needs no conversion.** `30s` is
  parsed by `time.ParseDuration`, which yields nanoseconds, so the comparison
  R2.3 declared and R6 left refused is one subtraction in one unit. A second
  unit anywhere on that path would put a conversion inside every `pending`,
  and a conversion is where an off-by-a-thousand lives.
- **An overage is reported in one denomination.** Both operands being
  nanoseconds, the bound, the claim's time and the overage are the same kind
  of number, and a reader comparing two of them is not also converting.
- **Wall-clock time never enters, and the unit is what keeps that true.** The
  run's clock starts at zero and moves only when the walk draws `advance`
  (R7.3), so a clocked run is exactly as repeatable as an unclocked one. The
  vocabulary's "wall-clock time never enters the ledger" is a property this
  unit has to hold up, not a caution beside it: a clock measured from an
  absolute epoch would make every run's facts differ by when it ran.

**A project with no `[clock]` has no clock**, and that is R7's third ruling
rather than a silence: nothing is read as "time did not pass". Two of the
three refusals that ruling names exist today and the third does not, which is
stated here rather than written as one sentence in the present tense:

| in a project with no `[clock]` | refused by | as of |
|---|---|---|
| a rule reading `clock` | `rumor validate`'s arm 8 | R4, kept |
| an eventual type bounded by a duration | the engine, at run time, naming the bound it cannot measure | R2.3, kept |
| a `[deadline]` table | `rumor validate`'s arm 20 | R7.2 |

**The third row is a declaration and not a description**, and this document
has paid for that distinction once already: 7e, two tables above, is a sibling
sentence that asserted a refusal before anything refused it. `deadline`
appears in no file under `internal/validate` -- zero under every spelling,
against `git grep -c 'quiesce' -- internal/validate/arms_platform.go` at
eleven and the same grep with `-i` at fourteen, which is the figure's
invocation written beside it because the figure alone has three right answers
(ibis, coop r7/7) -- and a `[deadline]` table added to a project with no
`[clock]`
passes `rumor validate` at rc 0 with its stdout byte-identical to the run
without it -- driven by ibis (coop r7/3) and reproduced by ratchet (r7/4) on
the commit that first carried this paragraph as a flat present-tense claim.
The arm is issue 101's third bullet.

**`clock.form` says HOW the run's clock is controlled, and there are two
ways** (fable, coop r7/51). The field is typed by R7.2 and read by the engine
in R7.3; every other key in `[clock]` stays carried and unchecked, for the
reason the reserved-table rule above gives.

| `form` | what the engine does | what the project provides |
|---|---|---|
| `adapter` | sends the `clock` request and writes the ledger fact from the RESPONSE's `t`, not from what it asked for | an app-exposed test clock the adapter can set and advance |
| `faked` | sends nothing: the engine's own `t` is the world's | a stack already running under faked time |

**An absent `form` is not refused here**, and that is a placement rather than
a leniency. Refusing it at the parse would be requiring at the parse what a
later reader is meant to refuse, which R6.2 measured the cost of: a fixture
written to trip one arm tripped the parser first and the arm's refusal became
unreachable. The engine refuses a `[clock]` it cannot read a form from, where
the refusal can name the request it was about to send. A form that is neither
word IS refused at the parse, because that is not a key a later phase might
reserve -- it is this phase's own field, spelled wrong.


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
contract = "2"                      # matched against the adapter's and the engine's
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
12 profile=settling steps=24  # the profile its row needs, and the length
```

- **One seed per line**: a non-negative integer, optionally followed by
  `name=value` tokens and then by `#` and a comment naming the defect it found.
- **`profile=NAME` says which weights a replay must use**, and its ABSENCE means
  the default mix -- which is what every line written before the token existed
  means, so no committed seed changes meaning by the token arriving. The name
  must be a profile the platform declares; arm 10b refuses one that is not,
  because a seed naming weights that do not exist would replay under the
  default while its author believed otherwise, and a promoted seed that
  silently stops being about what it caught is the same loss as one that cannot
  be read at all.
- **`steps=N` says how long a replay must walk**, and its absence leaves the
  length to the replaying caller. The length is a fact about the ROW a seed
  catches -- R6's is a sequence that does not fit in the ten steps the others
  are replayed at -- so a line that cannot say it leaves that fact in whatever
  replays it, where the next promotion inherits it silently and a row needing
  another length reads as a lost promotion rather than as a seed walked too
  short. `steps=0` is refused in the same words `--steps 0` is: a walk of no
  steps is not a walk.
- **`witness=WORD` says what a row proves when it is not a defect**, and its
  ABSENCE means the row promotes a defect -- which is what every line written
  before the token existed means, so no committed seed changes meaning by it
  arriving. One word is known today, `cleared`: a pending held and then found
  consistent, a run that exits 0 and finds nothing. A row carrying it is
  replayed with rc 0 REQUIRED and its output read for a pending re-checked and
  cleared; a row without it keeps exit 3 and the defect comment above. An
  unknown word is refused by arm 10b, by name, with the known set printed.
- **A token this build does not know is refused**, as is `profile=` with no
  name, a `steps=` that is not a number or a `witness=` naming nothing, and so
  is any token given twice:
  a line that means the default omits the token, and one that names nothing is
  a line whose author was interrupted.
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
| `[[venire]]` | yes, at least one | the walk draws its actors from these roles and counts. An empty venire can run nothing, which is a parse refusal rather than one of validate's twenty-three |
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
any of the twenty-three arms evaluates. Validate's refusals are about a project
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
| v2 (decl only) | 2026-10-28 | R7.2 | `[clock]`'s `form` is TYPED and the wire does not move. The table was reserved and read whole into a map, so every key passed and nothing was required: a project could declare `clock = {form = "wristwatch"}` and be accepted by a parser that had no opinion about it. It now carries `form`, one of `adapter` or `faked`, and a third word is refused by name before any actor moves. THIS IS `decl only` for R6.2's reason, which fable ruled at coop r6/1 and this row follows rather than re-argues: nothing read the old shape at RUN time, so no adapter and no project can break at run time, and a project carrying a wrong form is refused loudly by `rumor validate` instead of running under a clock nobody controls. THE KEY IS OPTIONAL AT THE PARSE, which is the half worth writing down: every `[clock]` written before this row still parses, and the project that declares the table and no form is refused by the ENGINE in R7.3 -- placed there because a parse refusal would make the engine's refusal unreachable, and the engine is the reader that knows which request it was about to send. WHAT WOULD FALSIFY IT: a third form arriving without a version move, or an engine that acts on a `[clock]` whose form it never read |
| v2 (decl only) | 2026-10-28 | R7.1 | THE CLOCK FACT'S UNIT IS DECLARED: `clock.t` is nanoseconds from the run's own epoch, zero at the first seal. THE WIRE DOES NOT MOVE and no table changes shape, which is why this is `decl only`: `[clock]` stays reserved for R7.2 to type, and an adapter built against v2 is unaffected -- the unit is an agreement about a LEDGER fact that the engine writes and a later clock request will answer in. DECLARED BEFORE THE WRITER EXISTS, on fable's ruling 1 at coop breakout-r7/0, which is the whole of why this row is its own commit: `Bound` has carried nanoseconds since R6 because that is what `time.ParseDuration` yields (ratchet, coop r6/157), `Now` has read the newest clock fact at or before the seal since R2.3, and the comparison between them has been unreachable for want of a writer (ratchet r6/155, rook rumor/1196) -- so the unit was already implied by two sides of a subtraction that nothing had performed. Settling it after the writer landed would have meant writing the unit down from the code that happened to produce it. NOTHING IS TESTED BY THIS ROW and it ships with no arm, which is the honest statement rather than a gap: the row declares, and the arms arrive with the ledger's control, the writer at every seal, the bound's judging and the replay, each in its own commit of R7.1. WHAT WOULD FALSIFY IT: a clock fact whose `t` is not nanoseconds, or an epoch that is not zero at the first seal, in which case `now - claimTime > bound` is comparing two units and every duration bound is wrong by whatever the ratio is |
| v2 (decl only) | 2026-10-27 | R6.4 | THE SEEDS FILE's line gains `witness=WORD`, one word known: `cleared`. A bare row still promotes a DEFECT, so every committed line keeps its meaning and no reader of the old grammar is wrong about a line it has seen -- the same extension clause `profile=` took, and the same "R5 may EXTEND it and may not contradict it". WHY IT WAS NEEDED: this file is defined as lines ending in a comment naming the defect they found, and the arm that replays every row requires exit 3 of each. R6's gate asked for a committed run proving a pending HELD AND THEN CLEARED -- D1's case, and docs/vocabulary.md's "`pending` until the declared probes read quiet", which rook found had no run to read it from (coop rumor/1230). Such a run exits 0 and finds nothing, so adding seed 66 broke the replay arm on the arm's own correct terms rather than being carried by the format (ibis, coop rumor/1238); fable ruled the token at rumor/1241 over loosening the arm, which would have stopped exit 3 being required of the rows that ARE defects. THE WORD IS CHECKED IN THE PARSER and not by each caller, so every reader of the grammar refuses the same set and the refusal prints the set from the same list it checks -- a message naming words the parser no longer accepts is worse than no message, because a reader edits their line to match it. NO WIRE CHANGE AND NO VERSION MOVE, the `profile=` precedent and fable's ruling at coop r6/1: `ContractVersion` is the wire's, this message is no message, and arm 13 has nothing to read |
| v2 (decl only) | 2026-10-26 | R6.3 | THE SEEDS FILE's line gains `name=value` TOKENS: `profile=NAME` and `steps=N`. R5 fixed the minimal format and said "R5 may EXTEND it and may not contradict it", which this is: a bare seed still means a seed under the default weights, so every committed line keeps its meaning and no reader of the old grammar is wrong about a line it has already seen. WHY A TOKEN AT ALL: R6's bug ledger says each row is caught by the stage or feature listed for it and one row says `settling`, so the seed that catches it must be replayed under THAT profile -- and a promoted seed whose line cannot say so is a seed that quietly replays under the wrong mix (fable, coop r6/92). AN UNDECLARED PROFILE AND AN UNKNOWN TOKEN ARE BOTH REFUSED by arm 10b, which is the arm that already exists for "a promoted seed that cannot be read": the loss is the same, a seed that no longer measures what it was promoted for. NO WIRE CHANGE AND NO VERSION MOVE, which is the `[quiesce]` precedent in the row below and fable's ruling at coop r6/1: `ContractVersion` is the wire's, this message is no message, and arm 13 has nothing to read. AND `steps=N` IS THERE BECAUSE A LENGTH IS A FACT ABOUT A ROW: R6's row is a sequence that does not fit in the ten steps the other promotions replay at, and holding that 24 in the arm that replays every seed made it the next profiled promotion's silent inheritance (ratchet, coop r6/106). THE GRAMMAR IS PARSED IN ONE PLACE NOW -- `platform.ParseSeedLine` -- because three readers each had their own copy and a token would have made two disagree quietly: internal/shrink compared the whole non-comment text to the seed's digits, so `12 profile=settling` read as "that seed is not here" and would have been promoted a SECOND time |
| v2 (decl only) | 2026-10-26 | R6.2 | `[quiesce]`'s probes are TYPED and the wire does not move. The table was reserved and read whole into a map, so every key passed and nothing was required -- `quiesce = {probe = "/test/quiet"}` was a valid declaration no code read. It now carries `[[quiesce.probe]]` entries of `kind` (queue_depth, worker_idle or direct), `timeout`, optional `every`, `query` for the `direct` kind reusing R5's observation shape, and `quiet`; sibling keys stay carried, so a key a later phase reserves here is not refused by this one. NO WIRE CHANGE AND NO VERSION MOVE, which is fable's ruling at coop r6/1 and the reason this row says so rather than bumping the number: `ContractVersion` is the wire's, R6.2 changes no message, and arm 13 has nothing to read. Narrowing a reserved table is a VALIDATE matter -- the untyped shape was never read at run time, so no shipped project can break at run time, and one carrying it is refused BY NAME before any run, which is the R4 way to break. Measured: `bindings_test.go`'s own well-formed fixture was refused the moment the probes were typed ("wanted an array of tables"), and it moved to the typed form in the same commit |
| v2 | 2026-10-19 | R5.2 | `observe` gains `args`: an object of the observation's declared arguments, present exactly when the observation declares any, carrying the system's own ids and never handles -- the same row an execute has carried since v1, for the same reason. `query` is the observation's NAME alone. `bindings.toml`'s `[[observation]]` gains `args`, the declared argument names, so the engine composes a request from the bindings and the handle table and nothing else. WHY IT IS A VERSION AND NOT AN ADDITION: before this an observation of one entity had nowhere to say which, so a subject would have been folded into `query` -- a second way of carrying an id, in a shape the decoder cannot type and with a separator that becomes contract by accident. The toy's adapter did exactly that for one checkpoint, which is how the gap was found (ibis, coop r5.toy/7; ruled by fable at /8). `direct` keeps its open verb shape for R8. The three-way version check now compares against `"2"`, and arm 13 does its job on an adapter still declaring `"1"` |
| v1 | 2026-10-17 | R4.1 | first definitions. The wire protocol: JSON-lines over stdio, one outstanding request per process, `kind`/`contract`/`id` on every message, strict decode refused by name as could-not-run, the four request/response pairs, the error envelope's six kinds with every envelope could-not-run, `describe`'s declared surface including `capabilities`, the three-way version check by exact string comparison with `"1"` as v1's value, and the five things an adapter never does. No run-local handle crosses the wire in either direction, and `observe` answers `value` alone -- neither a position nor a source. `platform.toml`: `contract`, `seeds_file`, `[adapter]`, `[[venire]]`, `[stages.<name>]`, `[weights.default]` with sparse per-stage and per-profile overrides resolved profile-then-stage-then-default, and `[profiles.<name>]`; `[[rollback]]` and `[thresholds]` reserved for R13 and R15, accepted when absent and refused when malformed, with faults, volume targets and upgrades declared by `describe`'s `capabilities` rather than by this file (fable breakout-r4/3). The bindings schema arrived with R4.2 in the same run: one `bindings.toml` per package, with actions (name, context, emits, requires as statute references, creates, response), UI flows whose `wait` step takes a condition and never a duration, observations (name, source, `of`, and `component` for `direct`), what each action `touches` for the rollback arm, per-type claim timing with a bound for `eventual`, roles and the session contexts they may hold, and `[clock]`, `[quiesce]`, `[deadline]`, `[seeding]` and `[uses]` reserved for R7, R6, R7, R20 and R19/R20/R21. An observation declares no position: that arm is static over the CASES' observations, which carry one (ibis, coop r4/20) |
