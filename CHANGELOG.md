# Changelog

Every tag has one section. A section is written in the bump commit for its
tag and is never edited once that tag exists.

## 0.1.36 — 2026-10-26

**R6.3: the toy's worker owns its own count off the clock, `stale` is decided
where the re-check is in hand, the reconciliation that FOLLOWS the quiesce
exists, a seed can name its profile, and R6's ledger row is FOUND rather than
asserted.** Fifteen commits of issue 91, re-cut onto the R6 CI proof, plus the
phase's methodology entry and its own citations.

- **The planted defect is off the clock, and the SUT now has two counts with two
  timings.** The toy's worker goroutine and its tick are gone: a read moves
  the job queue by one after answering, so a walk drives the lag it observes
  instead of waiting on wall time. `event.Count` is synchronous again on join and
  leave — R5's rows read as recorded, seed 2 at step 4 and seed 40 at
  step 3 — while `event.Tally` and `event.Recounted` are the worker's. **R6's own
  plant is a DROPPED repeat:** `drainOne` returns early on an already-recounted
  event, so once attendance moves after the first recount the tally is wrong for
  good, which no single observation can tell from a lag.
- **`stale` is a `pending` that was HELD, so `Judge` decides it and the overlay
  never says the word.** The vocabulary's own row makes `stale` a kind under
  `bug` — "a `pending` past its bound ... held open" — and an overlay that has no
  re-check in hand cannot know if anything was held. `pendingOverlay` now
  reports the fact (`pastBound`) and `Judge` composes the verdict through one
  function, so a first-look disagreement past a bound stays `bug` plain. The
  comment that stated the overturned rule is kept as a quotation of what was
  wrong.
- **The reconciliation that follows the quiesce.** Clause 3 reads "`pending`
  until the declared probes read quiet and `stale` if the reconciliation that
  FOLLOWS still disagrees", and the second half had nowhere to happen. After the
  prober reads quiet, the walk re-asks exactly the pendings still open, with the
  FRESH answer rather than the last step's; a pending nothing remembered is a
  refusal that names it; and every run prints what it found — `quiesce -- N
  pending(s) open at the end; reconciliation: ...` — so the clause has a reader
  on every run and not only when it fires.
- **A profile scopes the draw, and a promoted seed can say which one it needs.**
  `--profile NAME` selects a `[profiles.*]` stanza; `examples/toy`'s `settling`
  profile weights the recount action the default leaves at zero. `seeds.lock`
  lines carry `profile=` and `steps=` tokens parsed in ONE place
  (`platform.ParseSeedLine`), because four readers had each grown their own
  opinion of that line; validate's arm 10b refuses a seed naming an undeclared
  profile, listing the declared set.
- **R6's ledger row is found, from a committed seed, as `stale`.** Seed 12 under
  `profile=settling steps=24` reaches `FOUND bug of kind stale after step 24 --
  engine.stale, rests_on_planted false`, out of the end-of-run reconciliation
  with nothing left open, byte-identical across two attempts. The case and its
  sealed ledger ship beside it.
- **The quote count's warrant headers are checkable where they sit, and the
  arm's two counts must PARTITION its input.** Each `N -> M` header says why the
  count moved; the arm asserts the chain ends at `declaredQuotes`. It pinned 5 of
  11 on its first cut because its pattern wanted one of the two markers real
  headers carry, and its coverage line was a figure nobody could check against a
  total — so the warrants and the exclusions are now asserted to sum to the
  loose total, which refuses the two patterns disagreeing rather than any
  particular coverage.
- **The phase's methodology entry, and the entry's own citations.** A reader
  holds the total independently of the writer's count; a pattern keyed on the
  subject's own vocabulary excludes whatever is spelled differently — four
  instances in one day across three agents, the fourth being the script
  to check the third. Five instances of that class landed in one file before it
  was done, two of them introduced while fixing the first three, which is why
  the entry carries the line that a figure quoted in prose needs the base it was
  measured on.

## 0.1.35 — 2026-10-26

**R5's carried gate bundle: a cached pass is no longer a run, arm 19's ninth
cell is tripped, nine toy mutants were not one change, a budget is measured
rather than guessed, and one killer per rule hid every other one.** Four items
fable placed with me across R5 and held over, cut onto v0.1.34 as their own
release, with a fifth that the fourth's own review found.

- **`-count=1` on the test gate, and a cached verdict REFUSES.** Without the
  flag the build cache serves a pass for any package whose inputs have not
  moved, and the harness prints `ok <pkg> (cached)` — which the step's PROOF
  line counted as a package that ran, while its own comment claimed an absent
  ok-line means none ran. What the cache keys on is the files the TEST PROCESS
  opens, so whether a mutation is seen depends on how a suite reads its subject;
  a gate must not rest on that. The `(cached)` check is a refusal and not a
  filter, because with the flag set a cached verdict means the flag did not take.
- **Arm 19's ninth cell had no fixture.** Cell 2's reverse refuses on TWO loops
  — a declared action with no weight, and an engine pool kind with no weight —
  and only the first was ever tripped, so the second was a refusal nothing
  reached and deletable in silence. `19/kind-without-a-weight` trips it; the ARM
  count does not move, because a cell is not an arm.
- **Nine of the toy's ten mutants were not one change.** Each carried the
  sentence "One change against the statute and nothing else, which is what makes
  `rumor attaint`'s kill attributable to the rule it names", and each had been
  cut from the statute as it stood and never re-cut — so all nine lacked a rule
  it had since gained. `attaint` credited ONE mutant with eight kills, seven of
  them for rules that have mutants of their own, while the tally read "10 rule(s)
  killed, 0 unmutated" throughout. After the re-cut every rule is killed by its
  own mutant and **the tally is unchanged**: the number was never going to move,
  only its meaning. The mutants are generated now, and the one-change claim has
  an oracle that counts the contiguous regions in which a mutant differs from its
  statute.
- **The toy adapter's request budget is measured, and the margin was never the
  problem.** One request to the toy is 2.0ms median and 24.2ms worst; a process
  start to its first answered request is 10.4ms median and 27.2ms worst. So the
  old 10s stood at ~370x the worst work observed and the test client's 5s at
  ~185x: the budget was never below the work it measures, which was the premise
  the placement rested on. An expiry is therefore a STALL of ten seconds or
  more, and 60s is stall TOLERANCE rather than headroom — said in both constants,
  so the next reader cannot raise it on the premise this release withdraws. The
  adapter times every request and publishes count, slowest, budget and headroom
  once at close; an expiry names the budget and the elapsed and says the
  instrument gave up.
- **`rumor attaint` reported ONE killer per rule, so a shadowed mutant printed
  what a no-op prints.** The relation is many-to-many: a mutant that moves two
  rules took both columns, and every other killer of either was absent from the
  output — not as a kill, not as alive. So editing a shadowed mutant to do
  nothing would have left the report byte-identical, and nothing in the suite
  could have seen it regress. Every rule now names every killer with the path
  each moved it on, and every mutant names every rule it moves. The
  sole-killer count is printed per mutant and gates NOTHING: it is a coverage
  shape of the corpus rather than a fault in one mutant, and of the toy's ten
  rules eight have a sole killer — `has_user` by m10 alone — the two that do not
  being `believed` and `finding`. What stays enforced is unchanged —
  every rule killed by at least one mutant, and a mutant moving no rule refused
  by name. The previous entry's "each rule is killed by its own mutant" was true
  as membership and read as exclusive; this is the report that tells those
  apart.

## 0.1.34 — 2026-10-26

**The quiesce probes are declared, asked, and a system that will not settle is
a finding** — issue 90, the second of R6's four children. An eventual claim
bounded `quiesce` is judged against probes the project declares, and a probe
that gives up is reported rather than waited out.

- **`[quiesce]`'s probes are TYPED and the wire does not move.** The table was
  one of four reserved maps read whole, so every key passed and nothing was
  required: `quiesce = {probe = "/test/quiet"}` was a valid declaration no code
  read. It now carries `[[quiesce.probe]]` entries of `kind` (queue_depth,
  worker_idle or direct), `timeout`, optional `every`, `query` for the `direct`
  kind, and `quiet`; sibling keys stay carried, so a key a later phase reserves
  there is not refused by this one. `ContractVersion` is the wire's and R6.2
  changes no message, which is why it stands still.
- **Three arms on a probe's integrity, each with its own id and its own
  fixture**: no `timeout` (7c), a kind no adapter reports (7d), and `query`
  declared on a kind that does not read an observation (7e) — a refusal
  `bindings.go` had described in prose for a phase while nothing performed it.
  `TotalArms` 19 → 22, and arm 19's ninth cell gained the fixture it never had.
- **`quiesce_timeout` is the fifth finding** (vocabulary v3), beside
  `consistent`, `explained`, `pending` and `bug`. It is the word v1 reserved and
  not an invented one, and it is a FINDING rather than a kind under `bug`: the
  project's declarations are right and its adapter answered, so there is nothing
  for either reader to fix but the system under test. Exit 3. The finding is the
  TIMEOUT and not a verdict on any claim — a system that would not settle is a
  thing the run found whether or not a disagreement was open.
- **The probes are asked over the protocol that exists.** A probe is an
  `observe` with source `direct`, so `queue_depth` and `worker_idle` ask for
  their own kind by name and `direct` asks the observation the project named.
  Quiet is the conjunction of every declared probe, each with its own timeout,
  and the first to pass its own ends the wait.
- **A never-quiet probe ends the run with a hang report.** The walk asks after
  its last step whatever it found, so the finding is produced with NO pending
  open — the case an overlay cannot reach, since it returns before it looks when
  the statute found agreement. The report names the probe, its declared timeout,
  how many times it was asked and over what span. Measured: 108ms against a
  declared 100ms, one cadence of grace.
- **Two things the run was swallowing**, found by driving it: a finding that
  named no rule, and the overlay's `Reason` written and never printed — true of
  `stale` since R6.1 as well. The reason is on its own line now and is not
  special-cased to one finding.
- **The arm count's fifth copy had no oracle and it reddened CI twice.**
  `ci.yml` greps the twin's and the toy's output for the literal, in YAML where
  no Go test could see it, so a local green covered a commit that failed at the
  last step of the run. `tests/validate` reads that file now, holds both
  literals to the constant, and declares that there are two of them.
- **R6's control for every child carries its invocation.** Its three toy
  digests reproduce only under `--steps 10`, which was recorded nowhere: without
  the flag two of the three match and the third does not, so a reader
  re-running it matches two and reads the third as a regression of their own.

## 0.1.33 — 2026-10-25

**`stale` is a kind under `bug`, and a bound nothing can judge refuses** — issue
89, the first of R6's four children. An eventual claim type bounded `quiesce` is
now judged without a clock, and a disagreement that survives the system going
quiet is a defect rather than a pending held open to the end of the run.

- **`stale` is a kind, not a fifth finding.** v1 reserved the word and defined
  nothing; this phase spends it as `engine.Finding.Kind` on a `bug`, exit 3, on
  the FOUND line, naming the claim type, the claim's position and the bound. The
  vocabulary's own sentence — later phases add kinds under `bug` — is what
  settled the shape.
- **The quiesce bound needs no clock, and the duration bound still refuses
  without one.** The two forms are told apart by the bound itself rather than by
  what a fixture happens to carry: the same clock-less ledger that yields a
  verdict for a `quiesce` type refuses for a duration-bounded one.
- **A quiesce answer has four values, and the zero value is the refusal.** A
  bool could not tell a probe that ran and GAVE UP from a system still BUSY, and
  both handed in the same byte while only one is safe to read as "not quiet".
  `QuiesceAbsent` is the zero value and refuses, because silence read as "not
  quiet" holds a real disagreement open for a whole walk and reports it as
  clean — driven, fifty passes and exit 0. `QuiesceTimedOut` refuses too: its
  finding kind belongs to the phase that owns the probes, and this build must
  not invent a word.
- **A quiesce bound with no prober wired refuses the run.** The condition is
  structural — the prober slot is nil and a bound exists — so wiring a prober
  removes the refusal by construction rather than by anyone remembering to
  delete it. A project declaring no quiesce bound still walks.
- **A finding's identity is four fields, so every renderer carries all four.**
  Rule, value, attribution AND kind. `Says()` renders the kind, which fixes the
  trace, the examples line and the promoted comment at once — the last being the
  only artifact of a shrink that gets checked in. The same-finding comparison
  gained the kind clause, so a shrinker cannot trade a `stale` for a plain `bug`
  and call it a reproduction.
- **A promoted seed whose comment disagrees with the run refuses.** `Promote`
  returned early on any seed already present, so a seed promoted for a plain
  `bug` that later produced a `stale` kept a reason that was true when written,
  in the file a later run replays from. An absent comment is not a disagreement
  and nor is an empty reason; the refusal is for two reasons that are both
  present and differ.
- **A quiesce state this build does not know is named by its number.** Both the
  refusal and the rendering: Judge's switch names the unknown value instead of
  folding it into the clause written for an absent answer, and `String()` no
  longer answers "absent" for everything it has no name for. Fixing one without
  the other moves the lie a layer down and leaves it readable.

## 0.1.32 — 2026-10-25

**Temp directories no longer survive their runs** — issue 88, found at the R5
gate and not a gate failure. Two leaks, each with the arm that would have caught
it, and nothing else in the tree changes behaviour.

- **A cleanup that cannot run reads exactly like a cleanup.** Four `TestMain`
  functions built a binary into `os.MkdirTemp` and deferred the `RemoveAll`; a
  deferred call does not run when the goroutine leaves through `os.Exit`, so
  about 30 MB per package per run stayed in `/tmp` — 300 MB after one gate's
  three passes, and 3.1 GB on a reviewer's box after a day of mutation runs.
  Issue 88's own fix shape covers one exit of three: each of these functions
  also exits on a build that will not compile and on a `PATH` that cannot be
  set. One closure now covers every exit, including whichever is added next.
- **`tests/hygiene` is the arm, shaped like the defect and not like the four
  instances.** It parses every `.go` file in the tree and reports any function
  that both defers a call and exits the process, which found a fifth site on its
  first run — `cmd/toy-adapter`'s `main`, where every exit path closes by hand
  and says so, now a declared exemption carrying its reason. An exemption whose
  named function no longer exists fails the arm too, and parsing no files at all
  fails rather than reporting zero findings. Neither tool in the gate sees this
  class: `go vet` was clean on the tree holding all four, and the pinned lint
  set was green on the same commit.
- **The resource a package creates is the package's to remove.** `toyrestart`'s
  provider made a world directory in `open`, `Discard` removed only the saved
  world *file* of one checkpoint, and nothing removed the directory — so every
  caller leaked one by construction, 374 of them in a day. Removing it in
  `Close`, which issue 88 proposes, breaks a documented contract that an
  existing arm asserts: `Close` stops the toy and leaves the files, which is how
  the arm for "the file is perfect and the server is dead" kills the server.
  The removal is `RemoveWorld`, it stops the toy first, and it clears the path
  so a second call removes nothing rather than `RemoveAll("")`. The two openers
  call it. Issue 88's other half, "`run` closing the provider it opened", has no
  subject: `rollback.Open` has no non-test caller.

## 0.1.31 — 2026-10-25

**R5.3, the shrinker: a found bug is reduced to a trace somebody can read** —
issue 74. The six steps existed and nothing called them in sequence; now a run's
finding is replayed once on a verified clean world, shortened, stripped of whole
action kinds, written as a trace and promoted as a seed — and the toy's two
api-sim bugs are each found from a committed seed and reduced to under five
actions.

- **Every candidate runs on a world put back and verified**, the final replay
  included. That last one bypassed the restore and ran on whatever the previous
  candidate had dirtied: the one candidate whose trace is REPORTED and PROMOTED
  was the only one not run on a clean world. `docs/design.md` and
  `docs/rollback.md` both call the verify not optional, and an arm had pinned
  the opposite — `restored == probes - 1`, explained in words. The arm asserted
  the code; the documents were right.
- **A partial reproduction is reported rather than refused.** The gate records
  whether the attribution — the third part of a finding's identity — was
  actually compared, and nothing read it, so an operator could shrink on a
  two-part agreement and never be told. A project with no planted claims need
  not define `rests_on_planted`, so refusing there would turn away a legitimate
  project; the shrinker says in one line that the reproduction was partial and
  on what.
- **The toy's two bugs, found by a walk and shrunk.** Seed 2 is the stale count
  in four actions and seed 40 the row reached with no delete, in three; both are
  the honest finding and both are byte-identical across two runs. The row is
  read off the TRACE and never the finding, because both rows arrive as
  `default finding := "bug"` with no clause fired — no field of a finding
  separates them.
- **`examples/toy/seeds.lock` carries both, and seed 7 retires from it.** Seed 7
  was v0.1.28's first promotion and caught the stale count in FIVE actions,
  where the done-when asks for under five; seed 2 reaches the same row in four.
  It stays v0.1.28's record of that milestone, and the lock now carries a seed
  per row — before this it held one seed for one row and nothing for the other.
  The arm that re-runs every promoted seed therefore covers both rows for the
  first time.
- **A claim's handles are projected to the statute**, so two events in one
  ledger can be told apart and "an actor can join only an event it knows of"
  means something in a run that created more than one.

Also in this release: `rumor run`'s promoted-seed arm reads the exit status as
the four things it means rather than as "3 or not 3", so a could-not-run is no
longer reported as a promotion that lost its defect; and the toy project passes
`TOY_ADDR` through, which is what lets the rollback provider own the application
while the adapter drives it.

## 0.1.30 — 2026-10-24

**`Session.Close` is idempotent under concurrency, not only in sequence** — and a
returned `Close` now means the adapter is stopped. The old body read `done`,
found it open, and closed it as two steps, so two callers could both take the
`default` branch and the second `close(done)` would panic. It was already called
from a `defer` and explicitly in a run, which is the arrangement that makes two
callers ordinary rather than exotic.

- **`sync.Once` replaces select-then-close.** It removes the window and gives
  the second caller a stronger guarantee than the old early return: `Do` BLOCKS
  until the first call's teardown has finished, so a `Close` that returns means
  the adapter is stopped rather than merely being stopped. `stopAdapter` and
  `cancel` sat OUTSIDE the old `select`, so a second concurrent caller tore the
  adapter down twice even when nothing panicked.
- **Two arms, each catching what the other cannot.** One releases eight callers
  together over 200 iterations with deliberately no `recover()`, and counts
  stdin closes to prove teardown ran exactly once; restoring select-then-close
  makes it panic. The other asserts INSIDE each caller, against a deliberately
  slow teardown, that its own return implies teardown finished; replacing the
  `Once` with a bare `CompareAndSwap` that lets the losers return early makes it
  fail while the first arm still passes. Clean under `-race`.
- **What this does not change:** the stop protocol `docs/contract.md` states.
  EOF on the adapter's stdin is still the signal and `StopGrace` is still two
  seconds. What changes is the client's ability to keep that promise when two of
  its own callers close at once — a panic mid-teardown is how an adapter was
  denied the cleanup the document says it may rely on.

## 0.1.29 — 2026-10-24

**`docs --check` skips `.git` by name, so a review in a worktree is green** —
issue 6. The walk consulted its exemption list only inside the `isDir` branch,
which reads the rule as "a DIRECTORY called `.git`". In a git worktree `.git` is
a FILE holding a `gitdir:` pointer, so it fell through to the entry branch and
was demanded of `docs/coverage.toml` — and the refusal pointed at the map rather
than at the cause. Reviewing a branch with `git worktree add` is the ordinary way
to do it, and it failed the suite over a path git itself would never carry.

- **The name is tested before the type.** The intent at `skipDirs` is "the walk
  skips only `.git` and `target`", with no condition on the type, and the code
  now says that. Measured rather than reasoned: at 0da475f a worktree failed
  with ".git matches no entry in docs/coverage.toml" while the identical commit
  in a clone passed, the only difference being that `.git` is a directory there.
- **Both halves of the exemption have arms.** A `.git` FILE holding the worktree
  pointer is skipped as the directory is, with a control that a stray unmapped
  file still fails by name; and a FILE called `target` is skipped as the
  directory was, with a control that `targets` is still demanded of the map, so
  the exemption is proven to match whole names. The second arm exists because
  the hoist widens the rule to every name on the list and nothing exercised
  that — a comment is an untested claim whoever wrote it.

## 0.1.28 — 2026-10-24

**R5.5, the walk observes and a found bug is the run's product** — issue 83.
Until now a walk could act but never ask, so no run could produce a finding at
all; the toy's third of R5.3 was blocked on exactly that. A walk now evaluates
every declared observation after every step, stops on the first `bug`, and the
command says so and exits non-zero.

- **The walk observes after every step.** The prefix is sealed to everything
  that has happened, including the step just taken, which is the only position
  at which "did that leave the system consistent" is a question. The observation
  DRAWS NOTHING and appends nothing, so a step replays identically with or
  without it — the seeded walk is unchanged by the fact that it now looks.
- **The first bug ends the walk, and the record carries it.** `consistent` and
  `explained` walk on. The finding is checked after the step is recorded, so the
  trace a shrink reads includes the step that broke the system; a reduction over
  a trace one step short would be searching the wrong world.
- **`rumor run` reports the finding and exits 3.** A fourth status, and an
  addition rather than a move: `1` is the project being refused and a finding is
  the SYSTEM UNDER TEST being wrong, so the two outcomes this tool exists to
  tell apart never share a number. Before this the command read neither field
  the walk carried, and a found bug printed as a slightly short trace with a
  clean verdict and exit 0.
- **The finding line carries its attribution.** `rests_on_planted` is the third
  part of a finding's identity, with the rule and the value: a `bug` resting on a
  planted claim is the engine catching its own lie, not a defect. The twin at
  seed 1 reports it true, which the value alone would have booked against the
  twin's application. A statute declaring no such rule prints `undeclared`,
  because the attribution is then unknown.
- **A seeded attempt replays to a step count with kinds dropped**, which is the
  seam a shrinker reduces through: the same seed, fewer steps, or a named kind
  weighted out at every level — and a kind the project never weighted is refused
  rather than silently dropped.
- **The first promoted seed.** `examples/toy/seeds.lock` carries seed 7 for the
  stale-count row: the walk deletes an event and the observations after it are
  still answered a count for it. Eleven of twenty-four seeds find a bug and
  thirteen walk clean, so a promotion selects a reproducer rather than restating
  a property of the toy. Nothing runs that file until R22, so an arm re-runs
  every promoted seed twice and requires the exit, the identity and the
  attribution.
- **The toy adapter answers the observation it declares.** Its observe path
  reads the subject from the declared `args`, and the value it reports is the
  COUNT rather than the body — a scalar, which is what the statute reviews.
  Either half alone left every observation reading as `bug`.

Also in this release: the observe request's `args` are asserted off the bytes
the adapter receives rather than off its answer; the vocabulary sweep enforces
its quotation count, which cannot see a missing reason; and the told-fact count
in `docs/vocabulary.md` carries the date it was measured, because nothing armed
it.

## 0.1.27 — 2026-10-23

**R5.4, arguments and sessions: a walk acts as somebody, on something it knows
about** — issue 78, every bullet. An action's inputs are declared rather than
invented, a session is held per actor per context, and every claim a walk writes
names the entity it is about.

- **`[[action.args]]`, four sources.** An action declares each argument as a
  `literal`, a `draw` from a declared list, the acting `actor`'s own name, or a
  `handle` of a named kind. The walk composes the request from the declaration
  instead of guessing, and the draws are consumed in declaration order so one
  seed replays one walk.
- **A session per actor per context.** The engine acquires a token at first use
  and holds it for the run, keyed by actor AND context: reusing an `api` token
  for a `ui` action would send a token the adapter issued for something else.
  An empty token is refused rather than held.
- **A role's contexts gate applicability.** An action in a context the actor's
  role does not hold is inapplicable rather than attempted, so the engine never
  asks for a session the project says this role never has. A declaration that
  omits the actor's role is REFUSED, since a walk stalling for a missing
  `[[role]]` and a walk whose world permits nothing are different states.
- **`palter` is implemented.** It executes the honest action and then appends
  its claim with exactly one recorded field false, drawn from the values the
  project declared. An action whose claim carries no falsifiable field is
  inapplicable for the kind rather than drawn and then refused.
- **A precondition is shown the HONEST claims only.** A planted claim is a
  description with no action under it, so it must not steer what the walk does
  next; a check still sees every claim, because measuring belief against the
  store is what a planted claim exists for.
- **A `handle` argument resolves only to a handle the ACTOR knows of.** The
  table is global to the attempt, so the draw used to reach any bound handle of
  the kind — and a walk could pass `may_join` on one event while sending
  another. The precondition and the argument now quantify over one set.
- **The honest `tell`.** A pool kind mirroring `canard`: the drawn actor passes
  a claim it holds — one the run did not plant — to an actor who does not yet
  know it, and the system never moves. Without it a lie could travel and an
  honest claim could not, so no walk could reach a join by a second actor or an
  `explained` finding.
- **A claim carries the handles it names**, the minted one first and then what
  the step consumed, for a planted claim exactly as for an honest one: a lie
  distinguishable from the truth by its shape is not a lie a control can test.
- **EOF is the adapter's stop signal.** The client closes stdin and waits a
  bounded grace before killing, so an adapter's own teardown runs. Killing at
  once orphaned a spawned child, and under `timeout(1)` — which reaps
  descendants — a completed run then hung to its bound and read as a runner
  fault.
- **A pool whose every candidate weighs zero is an empty pool**, not a refusal:
  a zero weight is the project saying never, candidate by candidate, so a step
  with nothing choosable is the no-op step the walk already recorded.

Also in this release: the toy adapter removes the state directory it creates on
every exit path, its error envelope is the contract's `error`, every `api`
action requires the actor to have made its user, and the weights draw a nemesis
kind about one step in ten.

## 0.1.26 — 2026-10-20

**R5.1, the scheduler: a run is a seeded walk** — issue 72, every bullet. One
integer decides everything a run does by chance, is printed before the adapter
is even spawned, and is accepted back by `--seed` to replay it.

- **`rumor run PROJECT [--seed N] [--steps N]`.** At each step the engine draws
  an actor, computes which of its actions the statute permits against the
  sealed prefix, draws one by weight from that set, executes it against the
  project's adapter and records what it emitted. The ledger grows and is
  resealed each step, so a precondition about what an actor knows can become
  true as the walk proceeds — `join_event` appears only after a
  `create_event`, which is the walk reading the statute rather than a script.
- **The sequence is over handles, never ids**, which is the whole of issue 72's
  done-when and is driven both ways: two attempts from one seed produce an
  identical handle sequence while every id behind it differs, `--seed` replays
  a run whole, and a different seed walks differently — the control without
  which the first two would be true and empty.
- **The nemesis kinds draw from the same pool as honest actions.** A lie is
  scheduled exactly the way an action is, with its own weight and its own
  precondition: `canard` needs a planted claim its teller knows and did not
  make, `suborn` and `palter` inherit an applicable emitting action's own
  precondition, and `essoin` waits on R9's flow state. An undeclared weight is
  REFUSED rather than read as zero — zero means never chosen and the project
  decided it.
- **Resolvability is the second half of applicability.** An action whose UI
  flow names a `{kind}` no bound handle satisfies is inapplicable rather than
  drawn and then refused, so the walk never asks the adapter for a thing that
  does not exist.
- **The wire client keeps the adapter for the whole run**, which
  `docs/contract.md` has specified since v0.1.19 while only a one-shot probe
  existed. The session bounds each exchange rather than the run, refuses an
  answer carrying another request's id naming both, and stays usable after a
  fault envelope — which is an answer, not a transport failure.
- **`rumor fake-adapter --id-salt S`** so two attempts can answer different
  ids, deterministic and empty by default; and a repeat execution of one
  action now answers a different id, because `internal/handle` refused the
  second binding when two actors each created an event and the same id came
  back twice. The table was right.
- **The empty world is a sealed prefix.** A run's first step is evaluated
  against nothing having happened, which the precondition guard had refused as
  "unsealed" — a defect the walk found in R5.1's own earlier work, where 0
  because nothing has happened and 0 because nobody has sealed were one number
  to it.
- **`rumor run` imports the one shipped rollback provider**, because nothing in
  the binary did: `rollback.Registered()` was empty everywhere but those
  packages' own test binaries, so R5.3's shrink step could not have opened a
  provider at all.
- **Not in this release, and named rather than left to be discovered:** a walk
  reaches the FAKE adapter end to end, which is what issue 72 asks for, and
  cannot yet reach a real one — the bindings schema declares no action `args`
  and the engine acquires no session, so `cmd/toy-adapter` refuses at step one.
  That is R5.4, issue 78, with the four argument sources already ruled.

## 0.1.25 — 2026-10-20

**The nineteenth arm: every name a project uses is a name the project
declares** — ruled by fable at coop r4/113 and r4/125 after rook found that
`bindings.toml` could name a precondition rule no statute declared and all
eighteen arms passed it, silently dropping the action from every walk.

- **One arm, eight cells**, each pairing one reference against the set it is
  looked up in, and each refusal naming all three: `bindings.toml: requires
  may_leave: no rule of that name in rules.json`. The cells: `requires` against
  `rules.json`; a weight against the declared actions and the nemesis kinds;
  **every** action and kind against the weights, because absent is not zero; a
  `[[venire]]` role against the declared roles, where a project declares any;
  an observation's `args` and a flow's `{handle}` against what the actions
  create; a `[claim.<type>]` table against the emitted types; and the bindings'
  observations against the adapter's `describe`.
- **Two of the ruled nine came off by measurement.** An observation `of` an
  unemitted type is already held by arm 1 where a case carries the observation
  and by `tests/contract` where none does. An action `context` no `[[role]]`
  holds has an unsound referent set: it would refuse BOTH examples, since the
  twin declares no roles at all and the toy's three hold `["api"]` while its
  actions use `api`, `browser` and `none`.
- **The adapter cell evaluates the arm and names itself when it cannot run.**
  Eight readings need the documents alone; the ninth needs `describe`, so with
  no adapter it reports could-not-run naming the cell rather than skipping the
  whole arm. An unreachable adapter therefore costs four arms and one CELL:
  **15 of 19**, measured, where the twin and the toy both read 19 of 19 and a
  project with the browser stage off reads 17 of 19.
- **`docs/contract.md`'s `observations` row said in the PRESENT TENSE that the
  engine pre-checks observations against `describe`**, from v0.1.19 until now,
  and nothing implemented it — every use of the bindings' observations was arm
  6, a count, the wire's own validation or the fake's echo. That row is this
  arm's cell now, with the date and the correction. The `touches` sentence
  keeps its meaning and says why: arm 19 refuses a reference to a declaration
  that is not there, and what an action touches is a claim about the WORLD.
- **Both examples gained what the new cell requires**: the twin weighted
  `publish_doc` and not `publish_doc_ui`, and neither example weighted a nemesis
  kind. Both now weight every action and all four kinds, `essoin` at a declared
  ZERO — it abandons a flow mid-way and the engine has no flow state until R9.
- **Three defects in the arm, found by sweeping it the way the gate now reads a
  battery.** It refused a weight for `suborn`, which `docs/contract.md`'s own
  `[weights.default]` sample carries beside "nemesis kinds are action kinds in
  the same pool" — the arm would have shipped refusing a project this project's
  documentation tells people to write. Its stage and profile weight loops could
  not fire, the parser already requiring every override key to be a default
  key. And four cells walk a list while every fixture broke the only entry
  there was, so a cell reading `x[0]` alone passed all four; each fixture now
  carries a declared name first and the dangling one after it.

## 0.1.24 — 2026-10-19

**Three arms a mutation sweep asked for, over R5.2's own battery** — rook's
method (coop r5/10-12) turned on the toy and its rollback provider rather than
on a peer's work. Tests and documents only; no rule, fixture or constant moves.

- **`createEvent`'s default `visible_to` was pinned by nothing.** Every arm
  passed the field explicitly, so the default branch was exercised by no test:
  changing it from `{admin, member}` to `{admin, member, guest}` left the whole
  `cmd/toy` package green — the epic's visibility-leak row arriving by accident,
  on the one axis this system under test exists to be wrong about. The new arm
  posts with no `visible_to` key at all and asserts the field, the list and
  join, so three independent assertions see it.
- **`Verify` comparing LENGTHS instead of bytes passed every test** in
  `internal/rollback/toyrestart`, because the only moved world any arm built had
  an extra user in it and so differed in length as a side effect. A one-byte,
  same-length edit is now `ErrDirty`, as is an absent state file against a
  checkpoint that is not empty.
- **The concurrency arm asserted the race and not the ONE SEAT it needs.** With
  the fixture's capacity raised to 2, every assertion passed with the window
  CLOSED — the defect gone and the arm silent. It now reads the created event's
  capacity back and refuses to run with more seats than claimants, which is
  fable's r5/16 rule (an arm whose reach depends on a fixture property asserts
  that property) applied to my own battery.
- **Nine removals, nine screams**, and that is the weaker half worth recording:
  a removal is reached by every input, so it proves only that each mechanism has
  an arm near it. The three holes above were found by making a mechanism subtly
  wrong, which is reached only by an input sitting on the boundary.
- **fable's `handles` ruling is recorded where the limit is written**
  (`docs/design.md`, `examples/toy/statute.rego`): the projection will carry
  `handles`, the engine does not do it yet, and the toy's rules stay
  single-subject until it does.

## 0.1.23 — 2026-10-19

**A path nothing opened is could-not-run** — quill's three findings at coop
r4/91, and two siblings the first turned up.

- **`rumor validate <a path that is not there>` exits 2 and names the path.** It
  answered `REFUSED no-bindings` -- exit 1, the project is wrong -- about a
  directory that was not there to be wrong, and a real project missing its
  `bindings.toml` answered the same clause with the same status: the two runs
  differed only in the basename on the tally line. A path that is a file gets
  the same answer now. A directory that was READ and says too little is still
  exit 1, which is fable's r4/79 ruling and does not move.
- **No verdict comes out of a file nothing read.** An unparsable `rules.json`
  reported `could not run` AND `REFUSED no-rule-declared: rules.json declares
  no rule` -- a claim about a declaration nobody opened. Unparsable cases did
  the same through the `no-case` clause. could-not-run is answered before
  either vacuity check now, and only an arm reading the REFUSALS finds this,
  because the exit status was already right.
- **Every path accounts for all eighteen arms.** A decode failure returned
  before any arm and printed "0 of 18 arm(s) evaluated" with no reason beside a
  single one -- the one place a reader most needs telling that nothing was
  measured. The accounting invariant now holds on every path rather than on the
  vacuity and live paths alone.
- **Two documented counts and one stale comment.** `docs/contract.md` said
  "R4.3's thirteen refusals" and "one of validate's thirteen" while the same
  file recorded the ruling at eighteen; both read eighteen, and a new arm
  matches counts attributed to the command BY NAME so "four arms ask the
  adapter" stays legal. `tests/validate`'s invariant comment still explained
  itself with "the twin reports SIXTEEN of eighteen" and "the twin cannot
  enable them": the twin has enabled the browser stage since 0120352 and
  reports 18 of 18.

## 0.1.22 — 2026-10-19

**The toy system under test, and the contract row it forced** — R5.2 of the R5
call (issue 73).

- **A fifth example, and the first that is a running program** (`examples/toy`,
  `cmd/toy`, `cmd/toy-adapter`): a JSON API and a server-rendered UI over users,
  events, roles and capacity, with its own bindings, platform, a nine-rule
  statute, seven cases and nine mutants. It validates at **18 of 18 arms with no
  refusals**, the ordinary sweep reads five packages and 29 cases all agreeing,
  and `rumor attaint` kills all 25 rules with 32 mutants and nothing unmutated.
- **It is a process, because a restart provider cannot restart a library.** The
  engine starts the adapter, the adapter starts the toy, and everything the
  adapter knows about the world it learns over HTTP — the way it would learn
  about a system it did not ship with. The whole world is one file, sessions
  included, which is what makes a checkpoint a copy.
- **`observe` gains `args`, and the contract version goes to 2.** A count is of
  one event, and an observation had nowhere to say which — so the toy's adapter
  folded the subject into the query string for one checkpoint, which is a second
  way of carrying an id in a shape the decoder cannot type, with a separator
  that becomes contract by accident. `args` is now the row an execute has
  carried since v1: the system's own ids, never handles, the engine resolving a
  handle before the message is written. `query` is the observation's name alone.
  It is optional at the wire because the decoder reads one message without the
  bindings, and `rumor validate` pairs the declaration against the request.
  Found by building to the contract rather than by reading it (ibis, coop
  `r5.toy/7`; ruled by fable at `/8`).
- **A project version is a patch and the wire version is not.** This release
  breaks the wire contract — an adapter declaring `"1"` is refused — and that
  breakage is recorded in `ContractVersion` and `docs/contract.md`'s Revisions,
  which is what the contract's own version is FOR. `docs/release.md` reserves a
  minor for a breaking change to the **CLI surface**, and nothing about `rumor`'s
  commands changed, so this is a patch by the rule as v0.1.21 narrowed it.
- **The rollback trait and its first provider** (`internal/rollback`,
  `internal/rollback/toyrestart`): `Describe`, `Checkpoint`, `Restore`, `Verify`,
  `Discard`, with a provider named in `platform.toml` and resolved against a
  registry rather than imported. **A verify that checks one half passes over a
  corpse**: comparing the state file byte for byte is satisfied most perfectly
  when the application is dead, so `Verify` asks whether the file is the
  checkpoint *and* whether the server answers, and the test kills the process
  while leaving the file untouched. Three answers, and the third is not the
  first: clean, `ErrDirty`, or could-not-tell.
- **The three R5 ledger bugs are planted, each with its row and a witness.** The
  capacity race (a check-then-act window in `joinEvent`, with a by-hand test
  showing two boundary joins both landing), the API visibility leak (`eventCount`
  answers any session where `listEvents` asks), and the stale count after delete
  (the denormalized counter left standing while the store's own count goes to
  nothing). Every one is driven both ways: repairing it fails a test that names
  the row, so the toy cannot be fixed by accident with the shrinker's fixtures
  inside it. **R5 plants all three and catches two**: one action at a time cannot
  race, so the race is R8's to catch, which is the epic's done-when as amended.
- **The leak was present before it was declared**, and the record says so:
  `cmd/toy` shipped `eventCount` with no visibility check while its commit called
  the file "the honest version" the bugs would be introduced into. Nothing was
  mis-measured — no test pinned the behavior either way — but the claim was
  wrong, and the correction sits at the handler rather than in a later summary.
- **Three typed mismatches had quietly become agreements.** The arm-13 fixture
  wrote `contract = "2"` as "a version nobody declares", `internal/wire`'s table
  used `"2"` and `"3"`, and `internal/fake`'s D6 bumped an adapter to `"2"` —
  and the bump turned all three green, the fixture built to trip the version arm
  included. Each now derives from `ContractVersion`, so a mismatch is spelled
  "not what the engine says" and survives every later bump.
- **The drill driver could not fail.** `tools/drive-validate-drills.py` drove a
  hard-coded path to a binary nobody rebuilt, so the bump made 18 of 23 drills
  report a staleness in the harness as failing arms; and it printed `MISMATCH`
  while exiting 0, so every caller reading its status — including this author's
  own battery — saw green over them. It builds what it drives now and exits
  non-zero on any mismatch, driven both ways.
- **Two guards were widened rather than retyped.** `tests/contract`'s agreement
  test assumed one observation per package and said so in its own message; the
  toy has two, an `api` count and the `direct` oracle the rollback-coverage arm
  needs, so it is a set equality in both directions now. Drill D5 strips the
  corroboration control from **every** package's planted case, which the toy now
  has — without that it produced zero refusals, passing while testing nothing.
- **`docs/vocabulary.md`'s corroboration bullet gains the strip's scope** (fable,
  `breakout-r5/1`): a planted claim an observation tests must produce `bug`, and
  one no observation tests is counted and printed as unobserved. No term is
  added, removed or renamed, so `VocabularyVersion` does not move.
  `internal/attaint`'s quotation was re-anchored — the amendment inserted a
  sentence inside the passage it quoted, and `tests/vocabulary` caught it as a
  quote the document no longer says.
- **What the ledger could not express, recorded rather than worked around.**
  There is no capacity field on a claim, so an overrun is not nameable from a
  case — the data model agreeing, independently, with the ruling that R8 catches
  the race. And a statute never sees `handles`: the engine projects each claim as
  `{ref, actor, type, position}` plus `planted` and `visible_to`, so no rule can
  ask which entity a claim is about, and every package under `examples/` is
  single-subject. That is a limit on runs rather than on cases, and R5.3 needs it
  lifted to judge a walk that touches two events.

`ci.yml` validates the toy beside the twin, building both binaries onto PATH
under their own names: it is the only arm that proves the pair end to end on the
runner, since the twin's adapter answers from a recorded surface and the toy's
answers by driving an application over HTTP.

## 0.1.21 — 2026-10-19

**`go.mod` must be accurate, and the gate only ever asserted it existed** — plus
the Versioning sentence that four tags had already read the narrow way.

- **`go mod tidy -diff` is a CI arm now** (`.forgejo/workflows/ci.yml`). The whole
  of the manifest's coverage was `test -f go.mod || exit 1` in the fetch step: a
  wrong-tree guard that reads none of the file's content, with
  `tag-pipeline.yml` naming the file only in a comment. So an unused `require`, a
  missing one, or a `go.sum` short an entry could travel to a tag with nothing in
  the repository to say so, through every release including v0.1.18.
- **The diff is on stdout and the excuse is on stderr**, which is why they are
  captured apart. tidy exits non-zero BOTH for "these files are not what tidying
  would write" and for "I could not read the module graph", and it needs the
  network to answer at all — it resolves the test-only dependencies of
  dependencies, 32 modules on a cold cache, measured. A step reading the exit
  code alone would blame the manifest for an unreachable proxy, which is the
  misattribution v0.1.18 fixed in the doc blocks and run 192 made visible in the
  vulnerability step.
- **The reading is a script with its own battery**
  (`tools/gomod-accuracy-verdict.sh`): four answers — tidy, inaccurate, step
  defect, blocked — over nine named clauses, self-tested on the runner with its
  declared count grepped, 9 clauses over 10 cases. It is a file and not five
  lines of YAML for the reason `govulncheck-verdict.sh` and
  `verify-tag-verdict.sh` are: a workflow step is only exercised by pushing. Two
  of the nine refuse a contradiction rather than acting on one — a clean exit
  beside a diff means the tool's contract and our reading of it disagree, and
  acting either way would be acting on a contract we have just been shown we do
  not understand.
- **A pass over an empty graph is not a pass.** tidy exits 0 on a module with no
  requirements at all, so an empty or absent `go.sum` is blocked rather than
  green — the vacuity guard `rumor validate` carries for a project that declares
  no rule, at the one place this arm can be vacuous.
- **The wiring is driven apart from the reading**
  (`tools/drive-gomod-accuracy.py`). The battery covers what the clauses say and
  nothing in it covers whether the step captures the two streams into the right
  files, passes them in order with the real exit status, and propagates the
  verdict's code. The driver extracts the step from `ci.yml` by its `- name:`
  line — not a copy that can drift — and runs it over four trees: as committed,
  an unused require, `GOPROXY=off` over an empty module cache, and no
  requirements at all. 4 of 4, and the third is the condition the arm is shaped
  around. Measured under both go1.27.2 and go1.26.8, the runner's minor, so
  tidiness is not version-dependent between this author's box and CI.
- **A minor is a BREAKING change to the CLI surface** (`docs/release.md`). The
  Versioning sentence said "a change to the CLI surface — a new or changed
  command, flag, or output contract", which read literally makes every new
  subcommand a minor; four had already shipped as patches (`rumor check`
  v0.1.11, `rumor attaint` v0.1.13, `rumor fake-adapter` v0.1.19, `rumor
  validate` v0.1.20). Additions are patches until 1.0, the minor being reserved
  for a change that breaks something already written against the surface. The
  ruling is fable's (coop `thread:r4/85`); the four tags are its evidence and are
  named in the document because they outlive the thread.

The arm is new coverage and not a repair: the tree passes it as committed, which
is what `CLAUSE tidy` on this release's own CI run says. The version bump and the
changelog section ship no test of their own, as this document allows.

## 0.1.20 — 2026-10-19

**`rumor validate`: eighteen arms over what a project declared** — R4.3 of the
R4 call (issue 64).

- **A fourth example, and it is the one that passes** (`examples/twin`): a
  document publisher whose statute has one precondition (`may_publish`), five
  cases, four mutants and a corroboration control. It sits flat beside the R1
  three because a project is a directory and the twin is a project rather than a
  variant of one. Every fixture below is this tree plus one change, so the twin
  is the control for all eighteen arms: it passes with eighteen of eighteen
  evaluated and none skipped, which is fable's D9.
- **`rumor validate PROJECT`, and three exit values** (`docs/cli.md`): `0` every
  arm held, `1` refused with each arm named, `2` the run could reach no verdict.
  A decode failure from the adapter is `2` and never `1` — the project is not
  wrong, its adapter is — which is `docs/contract.md`'s boundary and fable's D7
  drill.
- **Eighteen arms, eighteen fixtures, and they live outside `examples/`**
  (`tests/validate/fixtures`): one per arm — 1, 2, 3, 4, 5, 6, 7a, 7b, 8, 9a,
  9b, 9c, 10a, 10b, 11, 12a, 12b, 13 — each the twin plus exactly one change,
  each tripping its own arm and no other. They are outside `examples/` because
  they are deliberately broken projects and both `rumor check` and `rumor
  attaint` walk that tree (fable, coop r4/63). They are **generated**
  (`tools/make-validate-fixtures.py`) and an oracle reads each one back against
  the twin in both directions, because neither failure is hypothetical: hand
  copies went stale inside an hour, and giving the twin its `seeds.lock` broke
  six fixtures silently by making each of them trip arm 10a as well as its own.
- **One spawner, and it decodes strictly** (`internal/wire/client.go`): the
  single place a validate run starts an adapter — argv, cwd, an environment
  built from named variables plus `PATH`, one `describe`, one line read, then
  kill. It goes through the contract's strict decoder and not `json.Unmarshal`,
  which is a defect D7 found rather than a precaution: a malformed describe left
  `contract` empty and arm 13 then refused the project for a version its adapter
  had never sent.
- **`seeds.lock`'s minimal format** (`docs/contract.md`): one seed per line, a
  non-negative integer with an optional `#` comment, and blank or comment-only
  lines ignored. It is fixed in R4 so arm 10a has something to read, and R5 may
  extend it and may not contradict it. The twin's own file is comment-only,
  which is what ignoring such a line is for: a project that has promoted no seed
  yet still answers the arm. **Arm 10a was nearly narrowed instead** — fire only
  on a non-default `seeds_file`, so the twin would pass — and a control a project
  cannot satisfy is an arm the project learns to route around, so the twin
  earned it.
- **The gate's literal is 18, and 18 is the accounting rather than the pass**
  (`.forgejo/workflows/ci.yml`, `validate.TotalArms`,
  `docs/testing-methodology.md`): CI runs validate on the twin with the adapter
  reachable, greps `18 of 18 arm(s) evaluated` and `0 refusal(s)`, and fails if
  `skipped` appears at all. A run that evaluated fourteen arms and a run that
  evaluated eighteen read identically when only refusals print, so every arm
  reports itself as evaluated or as skipped-with-its-reason and the two sum to
  eighteen or the run is wrong about itself.
- **D5 widened to the whole corpus** (`tests/attaint/drills/d5-no-control`): the
  overlay removes the corroboration control from the twin as well as from the R1
  three, so the drill measures the corpus and not the part of it that predates
  the twin.

- **Two quoted tallies in `docs/cli.md` moved with the corpus**, and only the
  release gate could see either: `rumor check`'s sample sits in a plain fence
  nothing executes, and `attaint`'s sits in a `run` block that only
  `release-gate.py` executes — on a pull request to `main`, which is after every
  push a branch makes, so the branch tip was green in CI while the gate refused
  it. Both are now held to the binary by `tests/validate`, which runs on every
  push, and the attaint sample's line count moved with them (28 to 36).

Driven rather than read: D1–D16 at 23 of 23 cases
(`tools/drive-validate-drills.py`), and two arms were wrong until they were
driven. D7 is the decoder above. D8 found a project with **zero rules passing at
exit 0** — nothing to evaluate read as nothing to refuse — which is now a
vacuity refusal that also skips all eighteen arms with its clause named; a
vacuity is a clause and not an arm, and printing it as `arm no-rule-declared`
was the first cut's error. fable ruled all three D8 cases refusals rather than
could-not-run.

A version bump and a changelog section ship no test of their own, as
`docs/release.md` allows; everything the section describes is covered by
`tests/validate`, the drills, and CI's arm-accounting step.

## 0.1.19 — 2026-10-18

**The adapter wire contract, the platform declaration and the bindings
schema** — R4.1 and R4.2 of the R4 call (issues 62 and 63).

- **A strict JSON-lines contract over stdio** (`internal/wire`): `describe`,
  `execute`, `observe`, `session` and one error envelope, with `kind`,
  `contract` and `id` on every message. Unknown and missing fields are refused
  **by name**, a decode failure is a could-not-run rather than a project
  refusal, and no run-local handle crosses the wire. `observe` answers with a
  value alone.
- **`platform.toml`, with `[[rollback]]`** (`internal/platform`): weights
  resolve profile → stage → default, and a profile that leaves a stage
  unweighted is refused against the default rather than silently inheriting it.
- **The bindings schema** (`internal/bindings`): actions, observations,
  `[claim.<type>]` timing and bound, roles and the reserved tables. `requires`
  is absent from the three example bindings permanently, which is a ruling and
  is pinned by a test that says so.
- **One strict reader behind both** (`internal/decl`), so `platform.toml` and
  `bindings.toml` refuse identically: a nil decoder, an absent file, an empty
  file, an undecodable one and a decoder returning `(nil, nil)` are each
  refused by name. A parser must not refuse what a validator is built to
  refuse, so `emits`, `requires`, `flow`, `bound` and `[uses]` parse and are
  answered for by validation.
- **A fake adapter** (`rumor fake-adapter`) that can fail a kind or omit a
  required field on demand, so the contract's refusals have a driver instead of
  a description.
- **A second direct dependency**: `BurntSushi/toml` v1.6.0, through the license
  and vulnerability gates. `docs/design.md` no longer claims OPA is the only
  one — the sentence was true when written and became false here.
- **Arm 10 of the release gate was dead on the Go tree** and is fixed: it reads
  the *declared* `ContractVersion` and `VocabularyVersion` at base and head
  rather than a Rust-era path, with the vocabulary half added as a case. 19 of
  19 becomes 20 of 20.

## 0.1.18 — 2026-10-17

**A fence marked `run` is executable code, and nothing was reading it.**
`release-gate.py` executes those blocks, which made `docs/cli.md`'s sample the
only code in the tree no suite checked. It invoked the toolchain as a bare `go`
and behaved three ways on three boxes: absent in one jail, go1.27.2 in another,
go1.26.9 on the runner.

- **The block resolves and pins its toolchain**, the way `ci.yml` does at eight
  sites and `tag-pipeline.yml` at two: resolve the first of `go`, `go126`,
  `go125`, then `export GOTOOLCHAIN=local`. `release-gate.yml` — the file that
  executes the block — did it at none, so the discipline lives in the block.
- **A missing toolchain now reports itself as one.** It surfaced on the
  `doc-run-block-failed` arm, so the gate blamed the documentation for a box
  with no Go on it. The block exits 1 naming a *provisioning* finding, and the
  gate quotes a failing block's last line, so the right words reach the reader
  of the finding rather than the author of the block.
- **Not `go126`.** That is a FreeBSD ports name, absent on the runner and on any
  standard install, and it would put a minor version in prose. The pin is
  `GOTOOLCHAIN=local` against `go.mod`'s floor, which OPA sets at 1.26.0 and
  which is a minimum rather than an equality.
- **`tests/docs` is the missing half of the pattern**, applied where the gate
  reaches it. It reads the fence form out of `release-gate.py` and refuses to
  run if the two disagree, so it cannot silently check a different set of blocks
  than the gate executes, and it carries a known-positive count because every
  arm in it passes vacuously over an empty set.
- **`release-gate.yml`'s census gains the three Go names and drops `cargo`**,
  which this project has not built with since the pivot: the one job needing a
  Go toolchain was the one job whose record could not say whether it had one.
  That line is a census and asserts nothing, so it ships without a test.

Driven both ways. Both arms fail on the pre-fix block naming `docs/cli.md:83`,
and six crafted cases keep them firing once the only real block passes. Of three
mutants, disabling the pin check and emptying the bare-name scan are killed;
disabling comment-stripping **survived** the first cut, because the fixture
wrote `go run` after a backtick where the scanner never matches and so exercised
nothing. The fixture carries a `$(` now and kills that mutant too.

## 0.1.17 — 2026-10-17

**The engine owns the seed, written before anyone builds it.** `docs/design.md`
gains the section R5 is built to, so the walk has a home in the design ahead of
the code and R5 amends the text with what it measures rather than the text
excusing the code.

- **A run is a seeded walk.** One integer, printed first and accepted back by
  `--seed`, is the only randomness in the system; drivers execute one action at
  a time and never see the generator, so a run replays from the seed, the weight
  file, the bindings and the commit alone.
- **The walk chooses by weight from the applicable set**, with the nemesis kinds
  (`suborn`, `canard`, `palter`, `essoin`) scheduled exactly like honest actions
  — which is why the vocabulary's control needs no trust model: the engine knows
  what it planted because it planted it.
- **The sequence is over handles, not ids**, so the same seed yields the same
  actions on every attempt while every attempt's ids differ.
- **Shrinking needs a clean world.** One replay on a verified clean rollback
  first; an unstable seed is not shrunk and not promoted; every candidate runs
  through the rollback trait's `verify`; and "the same finding" is rule, value
  and attribution agreeing, so a shrinker cannot drift from a `bug` resting on a
  planted claim to one resting on none.
- **An inapplicable candidate is its own outcome**, never evidence of
  instability; R14 and R15's toy bug consume that distinction.
- **Promoted seeds run first** (`seeds.lock`, R22), so found bugs become the
  regression suite without anyone writing a test.

Amended once from quill's review (rumor/750): the toy system under test is
pointed at, "the same finding" is defined with why the attribution participates,
and "minimal trace" names the output's shape while the under-five-actions bar is
the property. Two open items stay recorded in the epic: whether guided coverage
weights are committed beside `seeds.lock`, and the shrinker's name.

## 0.1.16 — 2026-10-16

**A `Docs-Unchanged` exemption belongs to its own release run.** Arm 9 of the
release gate unioned every trailer in the range and matched by document, so a
reason written for one release answered for a later one: with v0.1.12's bump in
range, its `docs/release.md` reason covered an unexplained `VERSION` change two
releases on — same head, only the base moved.

- **The boundary is the release, not the commit.** The union starts after the
  last bump in range that is not this run's own, since an earlier release's bump
  is in range only because the base was stale or the branch was cut from another
  release's branch. The convention stands untouched: a reason is written about
  the run, on the bump, last, when the range is known.
- **The commit boundary was tried first and refused the project's own
  history** — v0.1.12 on seven paths, v0.1.13 on three, the then-queued v0.1.14
  on one — because three of the four trailers in `v0.1.10..main` sit on a bump
  that touched only `CHANGELOG.md` and `VERSION`.
- **The accepted-release control is the arm a tightening needs here**, and
  neither earlier attempt ran it: `--self-test` now carries the ordinary shape
  of every run in this history beside the pair that pins the boundary both ways.
- **Citing an object a reader can reach** is written down beside the
  fixture-branch rules: a swept branch leaves its commits fetchable by full sha
  and no longer discoverable, and an object never pushed is unreachable whatever
  anyone sweeps.

## 0.1.15 — 2026-10-16

Three fixes from one night's reading of this project's own instruments, and the
thread they share: a check reports what it knows about, and what it does not
know about is invisible in the same shape as nothing being wrong.

- **The Runner requirements table gains its converse.** The table said what the
  runner must have; nothing said that two of those tools are absent from every
  agent jail, so a green local gate is `ci.yml` minus the lint and the
  vulnerability step and an errcheck or vulnerability finding appears for the
  first time on the push. They are a `go install` away rather than unavailable,
  and the distinction decides what a reader does next. Two runs had already been
  spent on this one cause, two weeks and two actors apart.
- **`golangci-lint`'s issue counts are uncapped in CI**, with both flags pinned
  by `tools/check-workflows.py`. The defaults cap at 3 same-text and 50
  per-linter issues and cap the SUMMARY with the list, so a step can print
  `3 issues: errcheck: 3` over a tree with four and nothing in its output
  disagrees with anything. Measured with the step's own pinned binary: the same
  tree prints 3 by default and 4 with the flags.
- **An undefined finding says so rather than rendering as nil.**
  `engine.Finding.Says()` answers the value when the statute defined one and an
  explicit absence when it did not, so the distinction between a finding that
  HOLDS and key-absence lives beside the field rather than in each consumer --
  both of which ignored it. Latent rather than live: every committed statute
  carries `default finding`. Its first arm was one-sided and a reviewer's mutant
  said so; the twin that distinguishes always-absence from correct is in the
  same commit as the arm it repairs.

## 0.1.14 — 2026-10-16

R3.2 (issue 53): the strip's second direction, and every arm given a permitted
twin. Both halves of what `rumor attaint` was missing, found by building the
checks and then pointing them at themselves.

- **The contract stated two directions for the strip and the tree checked
  one.** `docs/vocabulary.md` under `planted`: with the *tag* removed the same
  run must still produce the same `bug`, and with the planted claims *removed
  altogether* it must come back `consistent`. The second is what proves a bug
  WAS the lie rather than an artifact of the statute — a statute could pass
  every other arm while reporting one that was never about the lie. The
  counterfactual is **derived**, not committed: an eighteenth case under
  `examples/planted-lie` would move R1's gated 3/17/19, and deriving it checks
  every qualifying case rather than one fixture that could be written to pass.
- **The sentence was amended, not the fixtures.** The arm fires on three of the
  four planted cases under the unqualified reading, and those three carry a
  planted claim *incidentally*. The warrant for scoping it was not in the text
  — "its two uses are both checks on the engine" says what the checks are about,
  not which fixtures each covers — so the scope is now **in** v1's strip bullet,
  with a Revisions row recording both readings. Changing a fixture R1's gate
  passed to fit a sentence is the wrong direction of repair.
- **Three conditions the control arm tells apart**, which collapsing any two
  would lose: the control *evaluated and not passing*, *armed and never
  evaluated*, and *no control declared*. A fixture nobody observes must not read
  as a statute that stopped attributing.
- **Every arm has a permitted twin, derived rather than listed.** Nine clauses
  refuse something *absent* from a healthy tree, so the healthy package is their
  twin and no list can drift; only the two whose refusal is an *addition* carry a
  named twin. Coverage and discrimination are separate tests because they fail
  separately: an arm hard-wired to refuse passes the first and fails this one.
- **`attaint` measures R2.3's equivalence set instead of reading it**, and a
  test compares it to `tests/engine`'s declaration in both directions through
  `go/ast` — a *partial* pattern read would report "the declaration is stale",
  which is a finding about mutants that is really a finding about a regex.
- **Two latent defects**: an undefined finding reported as a surviving bug
  (absence is not a verdict), and a ref carried by both a planted and an honest
  claim, now refused by name rather than resolved by guessing which claim a tell
  meant.
- **`cli.md`'s sample is now its own check.** `release-gate.py` already executes
  blocks marked `run` and fails on a nonzero exit, but compares nothing to the
  page — so marking the block `run` would not have caught the stale tally this
  release started with; the gap was output comparison. The block greps each line
  it quotes, counts the lines against the figure **read from the prose**, and
  takes its floor from the page's own table, so the table and the sample convict
  each other. Four failure arms driven, including the sentence being reworded.

## 0.1.13 — 2026-10-16

R3.1 (issue 52): `rumor attaint`, the self-test, with fable's drills as
fixtures. `rumor check` asks whether each example's finding matches the verdict
its author wrote down — a question the statutes themselves answer, so a statute
that had quietly stopped deciding anything would still agree with a case file
written to match it. `attaint` asks whether the statutes are load-bearing at
all, and the epic's done-when is the shape of the answer: deleting a rule,
weakening a rule or stripping a planted tag must make it fail on the R1
examples.

- **Seven arms, each with its own refusal text**, because the gate reads the
  text and not the exit status. A single boolean would make a drill that trips
  the wrong arm look like a drill that passed. Four counting guards sit under
  them — no package, no case, no rule, no mutant — since every arm above is
  satisfied for free by an empty walk.
- **The mutation pass is two questions, and the second is the one a drill
  earned.** "Is every rule attacked by some mutant" is the dual of what
  `tests/engine` asserts, and it catches a deleted rule. It does *not* catch a
  weakened one: loosening `planted-lie`'s honest count passed every arm in the
  first build — the finding was still `bug`, the control still passed, every
  rule was still moved by some other mutant. What catches it is **does every
  mutant still attack**: the weakening is `m5-honest-counts-planted`'s exact
  shape, so afterwards m5 moves nothing, and a mutant that moves nothing means
  the statute has drifted into it rather than the mutant having gone harmless.
- **Both paths count, which reads like a loosening and is not.** Measured on the
  ledger alone, three rules come back unmutated: they are attacked only by the
  mutants R2.3 declared equivalent under the structural bound, so reporting them
  would blame the mutant set for a property of the seal. The output names the
  path, so a rule killed only from the cases is visible as one.
- **The equivalence set is measured here, not read.** `attaint` holds no copy of
  `tests/engine`'s declaration and exempts nothing; those three move rules from
  the cases, so they are alive and the dead-mutant arm never considers them.
  What it reports is which mutants are *inert on the ledger*, discovered
  independently, and a test compares that to `equivalentUnderTheBound` in both
  directions — so a stale declaration and a fourth mutant going equivalent each
  fail in two files for two reasons. The declaration is read through `go/ast`
  rather than a pattern: a **partial** pattern match reports "the declaration is
  stale", which is a finding about mutants that is really a finding about a
  regex, and a partial failure wears a result's clothes where a total one does
  not.
- **Three conditions the control arm tells apart**, which collapsing would lose:
  the control *evaluated and not passing* (the strip), *armed and never
  evaluated* (nothing reaches the lie), and *no control declared at all* (a
  vacuity). A fixture nobody observes must not read as a statute that stopped
  attributing. Reachability is taken from the document the engine handed the
  statute, so the knowledge rule stays in `internal/flatten` with no second
  implementation.
- **Six drills under `tests/attaint/drills/`**, each the pristine `examples/`
  tree with one file replaced, so each directory is its own diff. An overlay
  must REPLACE and never ADD: a path that drifted would land beside the original,
  leave the tree undamaged, and pass while testing nothing — so the harness
  counts files before and after. Each drill asserts the refusal **count** as
  well as the clause, because a drill that trips a second arm is not the drill
  that was designed.
- **Review fixes, in their own commit before this one.** Four unchecked
  `fmt.Fprintf` in `internal/attaint`: these lines are the evidence the verdict
  rests on, so a line nobody received is not a line that was reported. The
  runner named three of the four — `issues.max-same-issues` defaults to 3 and
  caps on the issue *text*, identical across all four, so a count read as
  completeness. And `TestEveryClauseIsExercised` did not do what it was named:
  a hand list checked only by its length, asserting one clause of eleven. It now
  reads the `Clause` constants through `go/ast` and requires the driven set to
  equal the declared set in both directions, one scenario per clause.
- `tests/vocabulary`'s declared quote count moves 13 → 14: `internal/attaint`
  quotes the `planted` entry's first use, which is the clause the corroboration
  arm implements.

## 0.1.12 — 2026-10-16

quill's second review, collected: the stale quotations and the fixtures that
outlived their mechanism, with a permanent instrument for the class.

- **Two quotations of `docs/vocabulary.md` in `internal/engine/pending.go` were
  accurate when written and false thirty minutes later — in the same tag.** The
  amendment commit reworded both entries four commits after the file quoted
  them, and both are in v0.1.11. One of them also reported a discrepancy as open
  and sent the reader to `thread:breakout-r2` for a call that had already been
  made and carried into the document above it (quill rumor/577, Q1).
- **A third misquote the oracle found and the review did not**:
  `internal/flatten/doc.go` quoted "Rules never follow tells." with a full stop
  where the document has a semicolon. Punctuation altered inside quotation marks
  is still a misquote.
- **`tests/vocabulary` holds every attributed quote in the tree to the
  document's current text** — 13 of them, the count declared so a sweep that
  finds none fails rather than passes, over a document whose normalised length
  is checked for the same reason. The attribution width is a *measurement*:
  checking every quoted span produced seven false positives for one real
  finding, because comments quote the epic and issue 44 in the same breath.
  Three normalisations — whitespace, markdown emphasis, case — each earned by a
  real quote here; without them the oracle reports three defects that are not
  there. **No exception for historical quotes**: a comment discussing an old
  wording describes it rather than quoting it, because an exception is the hole
  a stale quote hides in.
- **The three sentences corrected in `planted-lie/statute.rego` at v0.1.8 were
  left in all six of its mutants** (quill rumor/584, Q2): ADR 0001 cited by its
  drafting thread, the print ban described in Regorus's API, and the flattening
  said to be in Rust. 18 hits over 22 `.rego` files, re-swept to 0 after the
  fix. A mutant is what a reader opens when a mutant fails, so a fixture naming
  a harness that does not exist sends them looking for a call that is not there.
- **A pin catches an edit; it cannot catch a birth defect.** The drift pins hold
  three statutes and three `rules.json` and no mutant, and these sentences were
  false the day the mutants were cut — copied from the statute's header before
  the statute was corrected. So the guard is a sweep:
  `TestNoFixtureNamesARetiredMechanism` forbids five retired names across all 77
  files under `examples/`, with a positive control so an unreadable path cannot
  report a clean corpus by reading nothing, and a self-test driving a stale
  header against a corrected one.
- Not in this release: **quill's Q3**, four Rust-era nouns still in the epic's
  Decided list, whose fourth bullet is the authority the three already-fixed
  "in Rust" sentences were derived from. The epic's body is cal's, not a commit.

## 0.1.11 — 2026-10-15

R2.3: evaluation — a finding with its rule and its trace, the examples driven
from their ledger files, reconciliation, and `pending` on a clock.

- **`internal/engine` is the seam** between R2.1's store, R2.2's flattening and
  a statute. `Input` reads a sealed ledger and one check; `Evaluate` returns the
  finding with the rule definition that produced it, every named sub-rule, and
  the OPA trace retained whole for R10. The observation carries no position —
  it is the check's input, not a fact in the prefix — and sits at `seal+1`, so
  the statutes' `c.position <= ...` bound admits exactly the prefix.
- **A finding names a definition, not a rule name.**
  `data.rumor.statute.finding` is the same string for every finding in every
  case. The module and row of the definition whose body succeeded are read off
  the trace's `Exit` events, with the `default` marked, and the test requires
  two different findings in one package to come from two different rows.
- **`rumor check`** runs all 17 cases from their ledger files and prints the
  finding, its definition and its trace size per case with a tally — issue 44's
  "under one command", re-runnable from the binary. Examining nothing exits 1:
  zero disagreements over zero cases reads identically to agreement everywhere.
- **`tests/engine` requires the two paths to agree** rule by rule, case by
  case, with no shared evaluator — the ledger path through `internal/engine`,
  the case path through `rego` directly.
- **Three mutants go equivalent once the bound is structural**, and are proven
  so rather than excused: identical values for every rule on every case, and
  the case path must still kill them. A fourth going equivalent is a failure,
  and so is a stale declaration.
- **ADR 0001 at the layer that runs statutes.** `Evaluate` passes no
  `rego.EnablePrintStatements`, so OPA erases print calls and a statute full of
  them evaluates clean and silent — measured, and the reason the engine now
  refuses such a module outright. All 3 statutes and all 19 mutants are
  examined, the count reported, and a print planted into each module's own
  source as the control.
- **Reconciliation across the actors a sealed prefix names**, refused when it is
  not full: no pass, no query, an actor with no observed value, a value for
  someone the prefix does not name, or a prefix naming nobody. The control the
  done-when asks for passes — over planted-lie the honest-only set is
  `consistent`, the planted set is `bug` resting on the lie, and the
  stripped-tag set is `bug` resting on nothing.
- **`pending` is an overlay, and the clock is driven.** It needs the bound from
  the bindings (R4) and a `clock` fact, which no v1 statute reads, so the engine
  applies it over the statute's finding and keeps that finding beside it. Past
  the bound the statute's verdict stands, because v1 reserves `stale` for R6.
  Re-checked at the actor's next observation of the same query and at every full
  reconciliation; a reconciliation leaving one uncovered is refused.
- **Two findings against v1, reported rather than patched here:** the Fact kinds
  entry says `clock` facts appear in the examples' ledger files, and none of the
  17 holds one; and that entry gives a `clock` fact's actor as "the run", which
  a first reconciliation duly treated as an actor to judge.
- `docs/vocabulary.md`'s v1 row records the two readings R2.1 and R2.3 adopted,
  ratified as v1's meaning rather than changed, and three wordings fable amended
  in the same pass (breakout-r2/19): which document the flattened facts are
  fields of, that a `clock` fact belongs to no actor, and a Fact kinds note that
  now counts what is in the ledger files — `told` in eleven of seventeen,
  `clock` in none.
- **`govulncheck`'s exit 3 is a verdict, not a route problem.** `ci.yml` read
  it as "could not load the vulnerability database", which is the opposite of
  what the tool's own source says; run 192 printed "affected by 9
  vulnerabilities" and then announced a network finding. The classification
  moved to `tools/govulncheck-verdict.sh` — nine named clauses, ten cases over
  the real strings, mutation-checked — and both verdict scripts' batteries now
  run in CI, since neither was exercised by any workflow.
- **The vulnerability gate is red on this run and the cause is named.** This is
  the first commit to call `rego.Rego.Eval` from library code rather than from
  a test, so `govulncheck` now finds call paths into `net/http`,
  `net/textproto` and `crypto/tls`. All nine entries are standard library,
  found in go1.26.8 and fixed in go1.26.9, which the runner does not have.
  Measured both ways: 5de30c8 exits 0. There is no repo-side fix and none was
  invented.

## 0.1.10 — 2026-10-15

R2.1: the append-only ledger store, and the example cases as ledger files.

- **`internal/ledger` holds the four fact kinds and nothing else**, with a
  position and an actor on every fact. Positions are **assigned by the store**
  rather than passed in, because the vocabulary calls a position "assigned to
  every fact as it is appended" — if callers chose them, monotonicity would be
  the caller's invariant. `AppendAt` exists for `Load` and is the only door the
  seal has to guard.
- **Sealing, driven both ways.** Writing at or before a sealed position is an
  error and never a reorder; a position past the seal is accepted, and `Append`
  keeps working after a seal. `SealedPrefix` returns nothing for an unsealed
  ledger deliberately: a check reading an unsealed ledger is a sequencing error,
  and returning everything would hide it behind a plausible answer.
- **The dump is deterministic and diffable**, which are two properties and both
  are tested: byte-identical output for the same content, and one fact per line
  asserted by changing a single fact and requiring exactly one line to move — a
  format that reflowed would pass a round-trip test and fail that one. `Load`
  applies the same guards as `Append` and rejects unknown fields.
- **A tell cannot pass a claim that has not been made.** v1 does not state the
  rule and it follows from the entry, since "the receiver now knows that claim"
  cannot hold for a claim nobody has made. It lives in `AppendAt` rather than
  `Validate` because referential integrity is about the ledger's state, not the
  fact's shape.
- **The 17 example cases are now ledgers**, under `examples/<name>/ledger/`, and
  their positions do **not** carry over. The cases were authored as flattened
  input; a ledger needs the knowledge to arrive as `told` facts, and a tell needs
  a position of its own. Only two of the 17 leave room for one, and the three
  position-bounded cases put the observation at a claim's position, which the
  store refuses as non-monotone. The ledgers re-assign positions, preserving
  order and the bound rather than the integers — which is what the vocabulary
  allows when it says a replay assigns its own and compares findings, never
  positions.
- **The bound is now structural.** Two cases hand the statute knowledge their
  own bound excludes; placing each tell after its claim makes the seal exclude
  that knowledge, instead of leaving every statute to re-apply the bound.
- **`tests/ledger` checks the committed files against the cases and does not
  rebuild them**, because a validator that reconstructs its subject agrees with
  itself. Cases and ledgers must be in bijection, and the bound arm refuses if
  no case is found whose bound excludes anything.

## 0.1.9 — 2026-10-14

R2.2: what an actor knows, and the handle table. Plus the pin sentence wren
found.

- **`internal/flatten` computes `known(actor, claim_ref)`** from an actor's own
  claims and the one-hop `told` facts at or before a position, with an
  **ancestry extension point** that is an interface a test supplies — including
  a cyclic provider, since a walk without a visited set would hang rather than
  fail. The module doc says why this logic is outside Rego: a rule that followed
  tells would be a different statute, the fixtures pinned the output shape
  before the package existed, and an extension point needs somewhere to live.
- **The untold claim is the first test and it reads the real fixture.** The
  package produces the `known` arrays `examples/` already carries, so the
  fixture is the oracle: Charlie, told only by Alice, knows one ref, and Bob's
  claim is absent — the gap that makes that case `explained` rather than `bug`.
- **A mutation found a hole the first suite walked past.** A tell carries a
  *reference*, not the teller's knowledge, and that distinction is invisible in
  a one-claim ledger. Alice holding two claims and telling Bob one must leave
  Bob knowing exactly one; the case was added and the mutant now dies.
- **`internal/handle` keeps the run-local table for one attempt.** The handle is
  the stable name and the server id the disposable one, so `NextAttempt` returns
  an empty table rather than a copy. Three collisions: the between-attempts one
  v1 names, answered by `Require` returning an error rather than a bool — a
  caller holding `(string, bool)` can read the false as a disagreement and emit
  `bug`, reporting bookkeeping as a defect in the system under test — and two
  within-attempt conflicts that remain this project's decision, with the reasons
  recorded.
- **A `go.mod` `require` is a minimum, and `docs/design.md` said it was exact.**
  Cargo's `=` refuses a raise; minimal version selection may take a higher
  version if anything in the graph asks for one. Nothing here does — one direct
  require, 27 indirect, `go.sum` over the content — so the fact held while the
  explanation did not.

## 0.1.8 — 2026-10-14

ratchet's review of the port, acted on. Three findings, one of which changed
shape when it met the second.

- **The licence allowlist has two sources again.** R0.2's eight ids are
  enumerated in `docs/design.md` and `tests/licences` reads them from there,
  comparing against its own copy. `deny.toml` was the second reader until G4
  removed it with the Rust tree, which left the list in one place with nothing
  able to notice it drifting from the decision. A decision's home is a document,
  not a tool's config, so the pair is restored somewhere a sweep cannot take it.
- **`ci.yml` asserts the allowlist agreement and refuses any skip.** The review
  asked for the SKIP to be asserted by name, since it was printed and
  unasserted; with the arm reading the document it no longer skips, so that
  assertion would pin an unreachable state. The agreement is asserted instead,
  and a skip anywhere in the licence suite is now a finding — which is what both
  versions were protecting.
- **Three sentences in `examples/planted-lie/statute.rego` had gone false** and
  are corrected, with the drift hash re-pinned in the same commit: ADR 0001
  named by its retired thread, a harness described in Regorus's API, and R2's
  flattening said to happen in Rust. The drift test fired on the edit before the
  pin moved, which is the property — it catches an unnoticed edit and does not
  forbid a deliberate one. Byte-identity with `r1.2-examples` was built to prove
  the port changed no statute; it did, that branch is swept, and the identity is
  recorded in 0.1.4's section where it belongs.

## 0.1.7 — 2026-10-14

The vocabulary v1 as reviewed, and the engine docs brought current.

- `docs/vocabulary.md` carried onto the Go tree and amended from ratchet's
  review against the three example packages and rook's findings from building
  R2.2 against it: observations carry `of` and `role`, claims may carry
  `visible_to`, a claim's ref is its handle in v1, the between-attempts handle
  collision is named, the expectation on a claim is optional, `pending` and
  `clock` are named as gaps with the runs that exercise them, reserved words
  may appear in prose, "the same `bug` findings" compares findings alone, and
  the last Rust nouns are gone. R1.3 is this section.
- `docs/design.md`: the language-version paragraph in the past tense, since
  `go.mod` has declared 1.26.0 since v0.1.4; the print paragraph says what
  makes "statutes do not use it" true and records that OPA erases `print()`
  calls without `rego.EnablePrintStatements(true)`.
- `docs/adr/README.md`: a record is dated and frozen at acceptance.
- `docs/world.md`: the Trespass heading reserves the word and points at the
  vocabulary; it defines nothing.
- `README.md`: a workflow change is proved on its own event before it carries
  a release.

## 0.1.6 — 2026-10-14

The pipeline signs, verifies its own signature, and publishes. This is the
first tag with an artefact attached to it.

- **SHA256SUMS is signed with the release machine key**, `ssh-keygen -Y sign -n
  file` against `anvil@forge-01.corp.axonibyte.com` in the ci jail. For five
  tags the step looked for `gpg` and reported a "named wait" for a maintainer
  key; cal's amendment had decided ssh signing two days earlier, and nothing of
  the maintainer's belongs on a runner. A missing key is now a refusal naming
  the path it could not read.
- **The run verifies what it just signed**, with `ssh-keygen -Y verify` against
  the committed principal list, asserting the `Good "file" signature` line and
  not only the exit status.
- **The release is created and carries three files** — the binary,
  `SHA256SUMS`, `SHA256SUMS.sig` — with the tag's changelog section as its
  body. `GET /releases` was empty for v0.1.0 through v0.1.5; the epic had asked
  for this since R0. The upload is read back and the asset list compared,
  because a 201 and a release that carries the file are different claims. The
  artefact's name is read off `SHA256SUMS` rather than recomposed, since that
  file records what was actually built.
- **`fixture-v*` is in the trigger**, so the pipeline's three refusals can be
  driven by any agent instead of asking cal to push deliberately bad `v*` tags.
  A fixture runs every check and publishes nothing, refused by the tag's shape.
- `tools/forge-release.py` carries a battery over the changelog-section reader
  and the multipart body. It does **not** cover the create-and-attach half —
  nothing in it talks to the forge — and both the tool and the methodology doc
  say so, because a battery that implied otherwise would read as coverage of
  the only part that can fail in production.
- **The tag is checked out, which it never was.** `git clone` leaves the default
  branch checked out, so the version agreement, the build and `tools/` were all
  reading `main` while reporting on the tag — invisible for five releases
  because a fast-forward release makes those the same commit. The run now checks
  out the tag's tree, takes `tools/` from the workflow's own commit, and asserts
  both; an unasserted checkout is the defect being repaired.
- **`workflow_dispatch` takes a `tag` input**, so this file is proved against an
  existing tag before it carries a release. Every stale step in it was found one
  tag at a time, each costing a release, because the only trigger was a `v*`
  push that only the maintainer can make. The tag is resolved once and read back
  by the six steps that need it, rather than each repeating its own fallback.

## 0.1.5 — 2026-10-14

The tag pipeline verifies a tag instead of guessing about it, and this tag is
the first the pipeline runs green end to end.

- **`gpg.ssh.allowedSignersFile` is configured before `git verify-tag`.**
  Without it git does not check an ssh signature at all — it errors with "needs
  to be configured and exist" — and v0.1.4's run reported that as a failed
  verification, accusing a correctly signed tag. The path is the file
  provisioned in the ci jail (issue 25); the step asserts git reads the value
  back, and an unreadable file is a provisioning finding rather than a verdict.
- **The verdict logic moved out of the YAML** into
  `tools/verify-tag-verdict.sh`, because a workflow step is only exercised by
  pushing a tag and two defects had been sitting in four lines nothing could
  drive. It answers verified, refused or provisioning, and names the clause
  that answered; `--self-test` drives eight real strings and asserts the clause
  rather than the exit status, with three mutants each killing a case.
- **A good signature by an untrusted signer is now a refusal.** With the store
  configured, git prints a `Good ... signature` line *and* `No principal
  matched.`; the old test matched "no principal" and called it provisioning,
  granting what it should refuse. The finding is quill's (rumor/448), and it
  was unreachable until the first defect was fixed — which is why both land in
  one run.
- The v0.1.4 artefact was never published, since its pipeline refused before
  the build. This tag's run is the first with a `freebsd-amd64` artefact
  attached.

## 0.1.4 — 2026-10-14

The pivot to Go with embedded OPA, landed whole: the module, the CLI, the
engine decision, the statute suite, the licence surface, and the removal of the
Rust tree. `main` goes from Rust to Go in one fast-forward, because a release
that carried half of each is the thing the README's own line forbids.

- `go.mod`, `cmd/rumor` and `internal/coverage`: the 412-line coverage module
  carried over behaviour for behaviour, including every message a test asserts
  on, and all seven of its arms with it. The three exit codes are unchanged, 0
  clean, 1 failed check, 2 usage, because the workflows and `tools/` read them.
  `cmd/rumor` gained its first tests: four, including one that drives a writer
  which refuses every write and asserts the exit code is **not** 0 — a check
  whose result never arrived must not report success.
- **The `go` directive is 1.26.0, and it did not start there.** The port chose
  1.25 deliberately, since a newer toolchain builds an older module and not the
  reverse, so the lower bound is the one that cannot strand a runner. OPA
  declares 1.26.0, and a module cannot require less than what it depends on, so
  the statute suite raised it. The runner measures at go1.26.8, which is what
  makes the raise safe rather than merely necessary.
- `GOTOOLCHAIN=local` in every step that resolves a Go binary. Under the default
  `auto`, a `go` directive above the installed toolchain is satisfied by
  silently downloading another, so naming a versioned binary never pinned what
  compiles the code; the build's proof now reads the toolchain recorded
  **inside** the binary.
- A root `VERSION` file is the committed version source. `go.mod` has no
  version field, so the manifest cannot be the second of the three sources the
  gate makes agree, and dropping to two is weaker than three. Both readers are
  strict, and their strictness was driven against each other over 17 inputs
  after they disagreed about a trailing space and a CRLF.
- **The engine is OPA, pinned at `v1.21.2`, with the decision recorded in
  `docs/adr/0001-engine.md`** and the inventory behind it in `docs/design.md`:
  six capabilities measured in both directions, four of them reproducing ibis's
  Regorus figures exactly, and two — a test runner and retrievable evaluation
  traces — where the reference engine answers and the reimplementation does not.
  The pin is exact so a patch release cannot change a statute's verdict without
  a commit saying so.
- **The three R1.2 example packages carry over byte for byte**, proven by tree
  object and by each file's blob id pinned in a test, with a Go harness over
  them: 3 packages, 17 cases, 19 mutants, every case green and every mutant red
  with its killing case named, coverage and a trace asserted per case. A mutant
  that cannot be killed is removed with the gap written down rather than left to
  survive. ADR 0001's prohibition on `print()` inside a statute is enforced by
  two oracles — a static scan for calls no case reaches, and a hook on the real
  evaluator — because OPA **erases** print calls unless they are enabled, so a
  hook alone reports zero for a statute full of them. That flag's necessity is
  measured with it switched off, not asserted.
- **The licence gate is `tests/licences`**, over the surface `go list -deps
  -test` reports rather than the module graph, which over-counts: 24 built
  modules, all inside R0.2's allowlist. Multiplicity is refused rather than
  resolved, since a first-match classifier admits a module under "MIT OR
  GPL-3.0" on whichever marker it checked first; BSD-3-Clause's text contains
  BSD-2-Clause's, so the containment is encoded instead of being right by
  ordering. It has a negative control, which is the one thing the Rust gate
  taught at cost: seven synthetic modules through the same verdict function,
  five refused and two passed, each asserting the clause that answered. Adding
  it exposed that "outside the allowlist" had been **unreachable** — every id
  the classifier could produce was allowed, so a GPL dependency would have
  refused as "unclassified".
- `ci.yml` is Go: build, test, gofmt, vet, `golangci-lint` 2.14.0 and
  `govulncheck` v1.9.0 on measured pins with the version **asserted** rather
  than printed, `docs --check`, and the licence suite run by name so the surface
  has its own row. The trigger is every branch: a workflow's trigger is read
  from the file on the pushed branch, which is why every row until the port came
  from a manual dispatch and a pushed branch produced nothing at all.
- `tag-pipeline.yml` builds with `go build -trimpath` out of tree, and asserts
  the platform from **two** sources — `uname` for the host, `go env` for the
  target — because one read cannot tell a wrong runner from a cross-compile. The
  artefact suffix changes with it: `freebsd-amd64` rather than the LLVM triple
  `x86_64-unknown-freebsd`, which no longer describes how the binary is built.
- `tools/check-workflows.py` was blind to every Go invocation: it matched an
  allowlist of literal tool names, so a command spelled `"$GO"` vanished
  entirely and an unbounded one reported as bounded. Any `$VAR` in command
  position is now bounded-class, with two arms, and the suite is 23.
- `docs-stale` could never fire. Its filter decided "a finding" against "a tool
  that failed to run" with words `rumor docs --check` does not emit, so a real
  refusal read as a broken tool — for any tree, in the Rust tree too. It was
  invisible because the arm had never been measured: no toolchain on the runner,
  and no project in the self-test's synthetic repos. Fixed, with a battery of
  seven real strings, and then driven by a fixture branch that tripped it and
  nothing else.
- **THE RUST TREE IS REMOVED.** `Cargo.toml`, `Cargo.lock`, `crates/`,
  `deny.toml` and `tests/fixtures/deny-gpl/` are gone, with the cargo-deny-era
  `probe-mirror-egress.yml` whose own header said it comes out in the commit
  that reads it — it had been read, green on runs 100, 113 and 118, and that
  answer is now in `docs/release.md` instead of the instrument. **Its record
  lives at the tags v0.1.0, v0.1.1, v0.1.2 and v0.1.3**, which are where that
  tree, its gate and the GPL fixture still exist; `docs/release.md` points at
  them wherever a section describes something only those tags carry.
- Two losses from the removal, stated rather than quietly taken. The licence
  allowlist had two homes, `deny.toml` and the test, which had to agree; it now
  has one, and the test reports that comparison SKIPPED naming the cause instead
  of passing. And the justification for keeping the workflow checker in python —
  that the runner could compile nothing — is retired, because the runner builds
  and tests Go at 1.26.8; what stands in the way of porting it is the self-test
  that would have to be re-earned, which is a cost rather than an obstacle.

## 0.1.3 — 2026-10-12

A release gate that refuses, a tag pipeline that publishes nothing when it
refuses, and twelve fixture branches that have each been seen to trip one arm.

- `tools/release-gate.py`: the epic's nine Release gate bullets as twelve arms
  with stable ids, because bullet 1 is a disjunction and bullet 4 has three
  failure modes. It does **not** short-circuit: issue 4's test is that a fixture
  shows "the refusal reason it names, and no other", and a gate that exits at
  its first finding cannot answer the second half. Three exit statuses, and the
  third is not a verdict — 0 clean, 1 findings, 2 could not run, which the
  workflow treats as a failed run.
- `--self-test` drives every arm and asserts each fires **alone**, plus a clean
  run that must pass and a nine-shape battery over the `Docs-Unchanged` reader.
  It found five arm/fixture defects before a branch existed, including two pairs
  of arms that could not be told apart.
- Driving the gate against the accepted `6c5583d..5e8a639` found two more that
  no fixture could reach, because a fixture repo has no tags: `git rev-parse` on
  an annotated tag returns the tag object, and the tag for the version being
  gated counts as "the latest".
- `.forgejo/workflows/release-gate.yml` measures `python3` and fails loudly when
  it is absent rather than skipping, which is the standing argument against
  python in CI answered rather than ignored. It runs the gate's own self-test
  before the gate judges anything, and asserts the finding **set** from a file
  the gate writes, not a grep of its prose.
- `.forgejo/workflows/tag-pipeline.yml`: FreeBSD amd64 only, since an artifact
  nobody ran is not a release artifact. Every check sits before the build, so a
  refusal publishes nothing. A missing trust store is reported separately from a
  failed signature — announcing the first as the second would accuse a correct
  tag of forgery — and with no maintainer key on the runner the signature is a
  named wait, not a silent skip.
- Arm `docs-stale` reads the working tree, so it now verifies that the checkout
  **is** the run being gated and reports UNMEASURED when it is not. It had taken
  a ref and ignored it, which made it answer about whatever branch happened to be
  out; the fixture verification could not see that, because it checked each
  branch out first and so agreed by accident.
- The `Docs-Unchanged` trailer folds like the git trailer it is. A reason wrapped
  to column 0 does not exempt: git reads only the first line there, and
  truncating it published a reversed sentence into the PR summary.

## 0.1.2 — 2026-10-12

CI exists on the FreeBSD runner, and the licence gate has been seen to refuse.

- `.forgejo/workflows/ci.yml`: eight steps on the `freebsd-15-jail` runner, each
  asserting a value only execution could have produced rather than printing a
  status. Every external call carries a `timeout` and a hung call is a named
  finding against the runner, not a red build.
- The licence gate pins `cargo-deny 0.20.2` and asserts the pin rather than
  printing it: the literal is duplicated from the install line on purpose, so
  moving one without the other fails the step.
- **Test 3, the gate's negative control.** `tests/fixtures/deny-gpl/` is a
  separate workspace — excluded by the root manifest — holding one crate
  licensed `GPL-3.0-only` under a `BSD-2-Clause` root, so the only violation in
  that graph is the GPL crate. The step drives it with the real `deny.toml`
  rather than a copy, and asserts the exact status 4 plus the log naming both
  `GPL-3.0-only` and `gpl-fixture`. Before this, the gate had passed from the
  first run having examined no dependency licence at all; a gate never seen to
  fail is indistinguishable from one that is not wired up.
- **A timeout is a bound, not a diagnosis.** Four `rc 124` messages asserted
  causes the bound cannot support — a clone finishing at 181 s, a test taking
  901 s, and two that named the registry. Each now states what the status
  proves and says which log decides the rest. The cargo-deny install gains a
  60-second index touch ahead of its 900-second wrapper, so no-route fails in
  its own words and the wrapper can only mean slow.
- `tools/check-workflows.py` reads every workflow and refuses an unbounded job
  or invocation.
- `docs/release.md` gains **Test 3: the licence gate's negative control**, and
  `docs/coverage.toml` maps the fixture to it.

## 0.1.1 — 2026-10-10

The naming sweep is recorded and the crate name is settled.

- `docs/prior-art.md`: the registry, GitHub and cross-language sweep behind
  the name. `rumor` is held on crates.io — with `rumor-client` and
  `rumor-server`, by one owner — as a reservation published 2023-11-20 and
  never updated. `rumor-cli` was free and is the epic's decided fallback, so
  it ships rather than waiting on a transfer.
- `docs/design.md` records the names and the licence in its decided list,
  with the reason and a pointer to the sweep.
- The transfer ask was drafted, put to the maintainer, and **deferred**: the
  collision costs nothing until the project publishes to crates.io, which is
  not near, so no ask has been sent to the name's holder. `docs/prior-art.md`
  records the decision and its date. If the ask is made later and granted,
  publishing under `rumor` is an improvement landed by a patch run.
- `docs/release.md` gains its **Versioning** section: semantic versioning with
  the pre-1.0 reading of minor and patch, `main` fast-forward only so the
  version cannot go backwards, and the rule that a version number is not a
  reservation — the next tag is the next patch at the time its bump lands, in
  landing order. The remaining six headings are still owed by later phases.

Two findings from the sweep belong to later phases and are recorded, not
acted on: in the Nostr protocol *rumor* is a defined term for an unsigned
event, which `docs/vocabulary.md` must not silently collide with in R1; and
across the Romance languages the word's first reading is *noise*, which the
README now says.

No code changed, so no test was added: this run is documents. `rumor docs
--check` covers the new file through the existing `docs/*.md` entry and passes
at 24 paths.

VERSION NUMBER, because this section was drafted as 0.1.3 and ships as 0.1.1:
the breakout assigned a number to each child while they ran in parallel, and
that was retired — a number is not a reservation, and the next tag is the next
patch at the time its bump lands, in landing order. This child landed first, so
it takes 0.1.1. `docs/release.md` carries the rule.

## 0.1.0 — 2026-10-10

First tag. The workspace exists and nothing runs yet.

- Cargo workspace with the crate `rumor-cli` and the binary `rumor`.
- BSD-2-Clause `LICENSE`.
- `README.md` with the scope statement and the working arrangement.
- Every document of the documentation set, with its headings.
- `docs/coverage.toml`, mapping every path in the workspace to the docs that
  describe it.
- `rumor docs --check`: exits 0 when every path matches a coverage entry and
  every entry matches a path; non-zero naming the first path that no entry
  covers, or the first entry that covers nothing. The second half is not
  symmetry for its own sake: without it a mistyped glob is a dead entry the
  check cannot see, masked today only because no two entries overlap.

CLI surface created: `rumor docs --check`. Under pre-1.0 versioning a change to
the CLI surface takes a minor bump, which is why the first tag is 0.1.0 rather
than 0.0.1. No contract or vocabulary version constant exists yet, so no
`Revisions` entry is owed in `docs/contract.md` or `docs/vocabulary.md`; the
first of those lands with R1's vocabulary and R4's contract.
