// Package bindings reads a package's `bindings.toml`: what a project binds its
// statute to -- its actions, the claim types they emit, their preconditions as
// statute references, its observations, when each claim type settles, and which
// session contexts each role may hold.
//
// docs/contract.md's "Bindings schema" section is this package's prose, and
// internal/decl is the strict reading both declaration files share.
//
// THE RULE THAT DECIDES WHAT IS REQUIRED HERE, and it is the whole boundary
// between this package and `rumor validate` (R4.3): **a parser must not refuse
// what a validator is built to refuse.** Eight of validate's eighteen arms are
// about bindings that are well formed and wrong -- an action with no
// precondition, a rule referencing a claim type no binding emits, an eventual
// type with no bound, a browser-stage action with no UI flow. If this package
// demanded `requires`, arm 3 could never fire: the file would not parse, the
// run would report could-not-run, and the arm's fixture would be testing the
// parser. So `emits`, `requires`, `flow`, `bound` and `[uses]` are all OPTIONAL
// here and refused there, and what is required here is only what makes the file
// readable as bindings at all.
package bindings

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"forge.axonibyte.com/axonibyte/rumor/internal/decl"
	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

// Refusal is internal/decl's, for the reason given there: the rules for reading
// a declaration are shared with platform.toml and a second copy would drift.
// The file's name is the loader's to add -- this package is handed a decoded
// document and does not know what it was read from.
type Refusal = decl.Refusal

// Timing is when a claim type settles.
const (
	Immediate = "immediate"
	Eventual  = "eventual"
)

// ClockForm is HOW a project controls the run's clock, and the two values are
// the two the epic names: an app-exposed test clock the engine moves through
// the adapter's `clock` request, or a stack running under faked time where the
// engine's own `t` is the world's and no request is sent at all (fable, coop
// r7/51).
//
// THE FIELD IS OPTIONAL AT THE PARSE AND A WRONG VALUE IS NOT, which is the
// split R6.2 wrote beside the probes and the reserved block's own rule read
// together. Reserved tables are CARRIED, not policed -- "a key a later phase
// reserves here is not refused by this one" -- and a parser that required
// `form` would refuse every `[clock]` written before this phase, which the
// package's own test guards against by declaring one with an `endpoint` and
// nothing else. I wrote it required first and that test caught it.
//
// A VALUE THAT IS NEITHER FORM IS STILL REFUSED HERE, because "a malformed
// TABLE is still refused" is issue 63's rule and an unknown form is not a key
// a later phase might reserve -- it is this phase's own field, spelled wrong.
// A MISSING form is refused by the CONSUMER instead: the engine cannot ask an
// adapter for a clock it was not told how to reach, and that refusal has a
// reachable path, which a parse refusal would have taken away.
type ClockForm string

const (
	// ClockAdapter: the engine moves the clock through the adapter and the
	// LEDGER RECORDS WHAT CAME BACK, not what was asked for (fable r7/51).
	ClockAdapter ClockForm = "adapter"
	// ClockFaked: the stack runs under faked time, so there is no request and
	// the engine's own `t` is the world's.
	ClockFaked ClockForm = "faked"
)

// ClockControlled reports whether these bindings declare clock control, which
// is the `[clock]` TABLE BEING THERE (R7.1) and not any key inside it.
//
// ONE EXPRESSION, THREE CALLERS, which is why it is a function and not three
// copies of `b.Clock != nil`: `rumor run` decides whether a run may hold a
// clock, and `rumor check` and `rumor attaint` decide whether a project's
// COMMITTED ledger may carry a clock fact -- and those two answers must agree
// with the run's or a clocked project's own cases could not be loaded. Three
// sites reading one field is how a mirrored call site stops being a mirrored
// fix (ibis, coop 5/1082's lesson, met again here).
//
// PRESENCE AND NOT CONTENTS, deliberately, and the asymmetry is REPORTED
// rather than harmonized here: `rumor validate`'s arms 8 and 20 read the
// table's CONTENTS -- a bare `[clock]` declares nothing, which fable ruled at
// coop rumor/1359 -- while this is the run's reading. internal/bindings' own
// TestABareReservedTableIsPRESENTAndAnAbsentOneIsNIL is what makes either
// reading observable, and ratchet's /1373 is the open question of whether a
// default form would put the two against each other on a real project.
func ClockControlled(b Bindings) bool { return b.Clock != nil }

// ClockControlledAt is ClockControlled for a project DIRECTORY, which is what
// `rumor check` and `rumor attaint` need (R7.4): they decide whether a
// project's COMMITTED ledgers may carry clock facts, and ledger.Load refuses
// one where control is not declared.
//
// WITHOUT IT A CLOCKED PROJECT COULD HAVE NO CASES AT ALL. Both readers passed
// FALSE unconditionally, so a case carrying the fact R7 writes at every seal
// failed to load -- measured on a copy of examples/toy with one clock fact
// appended: "the committed ledger does not load: clock fact at position 6 with
// no clock control declared". R7.4's own ledger row needs such a case, which is
// how this was found rather than reasoned.
//
// ONE FUNCTION AND NOT A COPY IN EACH READER, for ClockControlled's reason run
// one level out: a project's cases must load the way its runs are walked.
//
// A PROJECT WITH NO READABLE bindings.toml IS NOT CLOCKED, and that is not a
// silent fallback: three of the five committed examples declare no bindings at
// all -- they are statute fixtures -- so a missing file means no clock control
// rather than an error. `rumor validate` is the verb that refuses a project
// for its declarations, and these two readers are not it.
func ClockControlledAt(dir string, dec decl.Decoder) bool {
	rep, err := Load(filepath.Join(dir, "bindings.toml"), dec)
	if err != nil || !rep.OK() {
		return false
	}
	return ClockControlled(rep.Bindings)
}

// Quiesce is the bound meaning "until the system settles", as opposed to a
// duration. R6 builds the probe; the word is fixed here because the arm on an
// eventual type with no bound has to tell a missing bound from this one.
const Quiesce = "quiesce"

// Step kinds in a UI flow (R9).
const (
	StepNavigate = "navigate"
	StepFill     = "fill"
	StepClick    = "click"
	StepWait     = "wait"
)

// Bindings is one package's declaration.
type Bindings struct {
	Actions      []Action
	Observations []Observation
	Claims       map[string]Claim
	Roles        []Role

	// Reserved for the phase that owns each, and carried as read: the keys
	// inside them belong to R6, R7 and R20, and checking them against a key set
	// this phase would be inventing is how a schema acquires fields nothing
	// consumes. A malformed TABLE is still refused, which is issue 63's rule.
	// Clock stays the RESERVED MAP for anything the table carries beyond the
	// form, the way Quiesce does beside Probes. ClockForm is the typed half,
	// which is what R7.2 adds.
	Clock     map[string]any
	ClockForm ClockForm
	// Quiesce stays the RESERVED MAP for anything the table carries beyond the
	// probes, so a project is not refused for a key R7 reserves. Probes is the
	// typed half, which is what R6.2 added.
	Quiesce map[string]any
	Probes  []Probe
	// Deadline stays the RESERVED MAP for anything the table carries beyond
	// the instants, the third table to take this split after [quiesce] and
	// [clock]. DeadlineInstants is the typed half, which is what R7.4 adds.
	Deadline map[string]any
	// Refusal stays the RESERVED MAP for anything the table carries beyond the
	// message path, the fourth table to take this split after [quiesce],
	// [clock] and [deadline]. RefusalMessage is the typed half, which is what
	// R8.1 adds.
	//
	// TWO MEANINGS OF ONE WORD, AND THIS IS THE OTHER ONE. `bindings.Refusal`
	// is a TYPE in this package -- an alias for decl.Refusal, a fault in
	// READING the declaration, which is what Report.Refusals carries. This
	// field is about the system under test REFUSING AN ACTION, which is a fact
	// about a live answer and not about this file being malformed. The name
	// follows the TOML key, as Clock, Quiesce and Deadline do, because a field
	// spelled differently from the table it reads is the drift those three
	// avoid; the collision is in English and not in Go, and it is written down
	// here so a reader does not resolve it the wrong way.
	Refusal map[string]any
	// RefusalMessage is WHERE in the system's answer the text of a refusal
	// lives, as a PATH and not a value -- `body.error` for the toy.
	//
	// A PATH, BECAUSE A VALUE WOULD BE AN EQUALITY TEST. `[[action]].response`
	// is already a per-action equality contract: "every key the project
	// declares must be present in the answer and equal to the declared value.
	// Anything else is the system REFUSING the action" (contract.md:136-143).
	// An error key declared THERE would make every accepted action a refusal,
	// which was measured before this table existed. A path names where to look
	// and asserts nothing about what is found, so the two declarations do not
	// collide and `[[action]].response` keeps the meaning that document gives
	// it (fable, coop rumor/1498).
	//
	// DECLARED ONCE FOR THE PROJECT AND NOT PER ACTION: the shape of a refusal
	// is a property of the system's API, not of one call into it, and a
	// per-action spelling would let a project describe a system no server
	// implements.
	//
	// OPTIONAL, AND ITS ABSENCE IS A COULD-NOT-RUN WHERE IT IS CONSUMED rather
	// than a parse fault. The walk never reads it, so refusing a project that
	// omits it would refuse every project that does not run the fuzz stage;
	// the stage that needs it names the table when it is missing.
	RefusalMessage string
	// DeadlineInstants names the OBSERVATIONS whose value IS an instant, in
	// nanoseconds from the run's epoch -- the fields of the system's own data
	// that hold a deadline (fable r7/42 ruling 2). The walk extends `advance`'s
	// targets to them beside the live pending bounds, which is what makes a
	// deadline something a seeded walk can cross on purpose.
	//
	// AN OBSERVATION AND NOT A DURATION, AND THE SYSTEM OWNS THE NUMBER. A
	// declared lifetime (`invitation = "30s"`) would put the same fact in two
	// files -- this one and the application's -- and the engine would then
	// target an instant computed from its own copy. Two sources for one number
	// is where R5's stale-count row lives, one level up; here it would be
	// WORSE than a wrong report, because the target would miss the real
	// deadline and the walk would find nothing while both files looked right.
	// So the project says where its deadline can be READ and the run asks.
	//
	// A NAME DECLARED TWICE IS NOT REFUSED, AND THAT PERMISSION IS PROVISIONAL
	// ON A CONSUMER THAT DOES NOT EXIST YET (ratchet, coop r7/60). The reason
	// given for it is that the targets are a SET and `schedule.Landings`
	// dedupes, so a repeat changes no draw -- which is true of `Landings` and
	// says nothing about the path from a DECLARED instant to a landing,
	// because that path is not built in this commit. A reader that took this
	// list positionally, or weighted a landing by how often it appears, would
	// make the permission wrong; whoever writes that reader owns this
	// sentence. The names that ARE how something is asked for -- an action's,
	// an observation's, a role's -- are the ones this parse refuses a
	// duplicate of, below, and a name here is not one of them: it is a
	// reference, which `rumor validate`'s arm 19 cell 7 resolves.
	DeadlineInstants []string
	Seeding          map[string]any

	// Uses is the project's ASK: the faults, volume targets and upgrades it
	// wants. `describe`'s capabilities is what the adapter can actually do, and
	// validate's arm pairs them -- live against static, which is why neither
	// side is written in the other's file.
	Uses Uses
}

// Action is one thing an actor can do.
type Action struct {
	Name     string
	Context  string
	Emits    []string
	Requires []string
	Creates  []string
	Response map[string]any

	// Contended is what this action contends on, when it does: where its limit
	// is read and where what the system HOLDS is read. Zero for an action that
	// declares none, which is every action in a project the concurrency stage
	// has nothing to do with.
	Contended Contended
	Flow      []Step

	// Args are the arguments this action needs and where the walk gets each.
	// Optional: an action the system takes with no arguments declares none,
	// and an adapter that needs one the bindings do not declare is a pairing
	// `rumor validate` reads, not something a run should discover.
	Args []Arg

	// Touches names the stateful components this action MUTATES, which is the
	// other half of the rollback-coverage arm: a component is otherwise
	// nameable only on a `direct` observation, so an action that dirties an
	// uncovered component and is never read directly fell outside an arm whose
	// wording is about bindings that touch one (ibis, coop r4/23).
	//
	// OPTIONAL, AND THE COST IS STATED RATHER THAN DESIGNED AWAY: an action
	// naming nothing is read as mutating nothing, exactly as an action with no
	// `requires` is read as having no precondition. Arm 3 refuses the second;
	// no arm refuses the first, and a nineteenth arm is not this package's to
	// invent -- the count is fable's.
	Touches []string
}

// Step is one step of a UI flow.
// The four argument sources, ruled by fable at coop r5/68 and r5/72. They are
// the only things a WALK can supply: it knows the run's handles, the literals
// the project wrote down, the draw it owns, and whose step this is.
//
// WHY `actor` IS THE FOURTH AND WHY IT TAKES NO DRAW: ibis found the need
// (r5/70) -- cmd/toy keys sessions by user NAME, so a session for member_1
// exists only if some create_user made a user called member_1, and
// wire.ExecuteRequest carries no `actor` field at all, which I had hit
// independently as "unknown field actor". A drawn or literal name breaks the
// SESSION half of this same child. It consumes no draw because a source that
// took from the generator would make the sequence depend on how many arguments
// an action declares, and two projects with one seed would walk apart for a
// schema reason.
const (
	ArgHandle  = "handle"
	ArgLiteral = "literal"
	ArgDraw    = "draw"
	ArgActor   = "actor"
)

// ArgSources in declaration order, for a refusal that can name the set.
var ArgSources = []string{ArgHandle, ArgLiteral, ArgDraw, ArgActor}

// Arg is one argument an action needs, and where the walk gets it.
//
// EXACTLY ONE FIELD BESIDE name AND source IS ACCEPTED, the one that source
// uses: `of` for a handle KIND, `value` for a literal, `from` for a draw's
// list, and nothing for `actor`. The others are refused by name rather than
// ignored -- a `from` on a literal is a project saying two things about one
// argument, and guessing which it meant is how a walk sends something nobody
// declared.
type Arg struct {
	Name   string
	Source string

	// Of is the handle KIND a `handle` argument names -- `event`, not
	// `member_1.event#1`. The engine resolves it to an id the ACTOR knows of,
	// because an id is what crosses the wire and a handle never does.
	Of string

	// Value is a `literal` argument's value, carried as decoded.
	Value any

	// From is a `draw` argument's list, carried as decoded so a project can
	// draw numbers, strings or lists of roles.
	From []any
}

// Contended is an action's declaration that two callers can race it, and the
// two facts a concurrency stage needs to judge the race.
//
// DECLARED BY NAME ON THE ACTION, AND NOT DERIVED, which is fable's ruling at
// coop rumor/1525 on ibis r8/53 and rook r8/54. The tempting derivation is the
// dangerous one: examples/toy's `store_event_count` declares
// `of = "event.joined"`, and THREE actions emit that claim type -- join_event,
// accept_invitation and join_event_ui -- of which accept_invitation is the one
// that holds its lock across the check and the insert. So a stage deriving its
// subject from the claim type can select the CONTROL and report no overfill,
// which is exactly what a correct system reports. The claim type cannot stand
// in for the action.
//
// NEITHER FACT WAS DECLARABLE BEFORE: `capacity` appeared in the toy's bindings
// only as a draw argument's NAME and in comments, so a stage could not learn
// which action contends or what its limit is, and hardcoding either would make
// the stage toy-specific -- the same defect `[refusal].message` closed for a
// refusal's text.
type Contended struct {
	// Limit is where the limit this action contends on is READ: the argument
	// of the action that CREATED the thing, as `create_event.capacity`, or an
	// observation that answers it.
	//
	// READ AND NOT DECLARED AS A NUMBER, for the reason R7 gives a deadline: a
	// declared value would put one fact in two files and the stage would
	// contend against ITS copy, so a run could miss the real limit while both
	// files looked right. The toy's capacity is a DRAW, so there is no single
	// number to copy even in principle.
	//
	// THE TWO FORMS ARE NOT EQUIVALENT AND ONLY ONE IS HONEST, which rook
	// measured at coop rumor/1526 against the ruling that admits both. An
	// ARGUMENT of the creating action is drawn independently of the system, so
	// the clause reads "the system holds more than its DECLARED limit allows".
	// An OBSERVATION does not: Observed and Limit would then both come from
	// the system, and the clause collapses into "the system disagrees with
	// itself" -- which is D7 one level up, the single-oracle defect this
	// stage's verdict was already corrected for once, and which
	// examples/toy's own bindings name: "an oracle that read the same field
	// could not disagree with it".
	//
	// SO THE ARGUMENT FORM IS THE ONE TO IMPLEMENT FIRST, and it needs
	// something this package cannot give it: the drawn scalar is in no field
	// of schedule.Step, whose Used carries HANDLES and whose AdvancedTo is the
	// one recorded draw -- a field that exists because a mutant of
	// chooseHandles "survived the entire suite until this field existed". A
	// stage reading a drawn limit therefore waits on Step carrying the draw.
	// Which form survives is not this type's to settle; that it matters is
	// recorded here so a reader does not implement the cheaper one.
	Limit string

	// Held is the declared query that reads what the system HOLDS, which is
	// the quantity the race's signature is about.
	//
	// A DECLARED QUERY AND NOT A PATH, because this one already had a home:
	// `store_event_count` is a `direct` observation whose own comment says why
	// it is the right oracle -- "computed from the attendee list ... an oracle
	// that read the same field could not disagree with it".
	Held string
}

type Step struct {
	Step   string
	To     string
	Field  string
	Value  string
	Target string
	Until  string
}

// Observation is one named observation and how it is taken.
//
// IT CARRIES NO POSITION, and that is settled rather than omitted: positions
// are the ledger's, assigned on append, and the validate arm about an
// observation lacking a position or a source is static over the EXAMPLES' cases,
// whose observations carry both (ibis, coop r4/20).
// Probe is one way a project says its system is known to be quiet, declared
// under [[quiesce.probe]] and answered by the adapter.
//
// SEVERAL, NOT ONE, each with its own timeout: a queue can be empty while a
// worker is mid-job, so "quiet" is the conjunction of the probes a project
// declares and never a single reading (fable, coop breakout-r6/0's R6.2 row).
//
// THE TIMEOUT IS REQUIRED AND THAT IS THE POINT. A probe without one is a
// wait with no end, and R6's rule is that a system which never goes quiet is
// a FINDING rather than a hang -- so the bound has to exist before the engine
// will ask. validate refuses a probe that declares none, by name.
type Probe struct {
	// Kind is queue_depth, worker_idle or direct. The first two name a
	// property the adapter reports; `direct` reuses the observation shape R5
	// shipped, store_event_count being the model, and so carries Query.
	Kind string

	// Timeout is how long this probe may go unanswered before the run carries
	// a finding of the timeout's own kind. A duration, as a claim's bound is.
	Timeout string

	// Every is how often to ask while waiting. Optional: absent means the
	// engine's own cadence, and a project that wants a slower one says so.
	Every string

	// Query is the `direct` kind's observation name, and is refused on the
	// other two -- a queue depth is not read through an observation.
	//
	// AND SOMETHING REFUSES IT NOW, which it did not when this sentence was
	// written: validate's arm 7e. For one phase this comment asserted a
	// refusal that nothing performed, so a project could declare a query on a
	// queue_depth probe and pass -- arm 19's lesson in prose rather than in
	// code (fable, coop r6/33).
	Query string

	// Quiet is what this probe's answer must read for the system to count as
	// settled: for queue_depth a number (usually 0), for worker_idle a bool,
	// for direct the value the observation must return.
	Quiet any
}

type Observation struct {
	Name   string
	Source wire.Source
	Of     string

	// Component is the part of the system a `direct` observation reads. It is
	// how a binding names a stateful component at all, which is what validate's
	// rollback-coverage arm compares against `describe`'s components and
	// platform.toml's [[rollback]].covers (fable's D11).
	Component string

	// Args are the observation's declared argument NAMES, mirroring an
	// action's: the engine composes an observe request from these and the
	// handle table and nothing else, resolving each handle to the system's own
	// id (contract 2; fable, coop r5.toy/8).
	//
	// WHY THE NAMES AND NOT THE VALUES: a count is of ONE event, and before
	// contract 2 an observation had nowhere to say which -- so the subject
	// would have been folded into the query string, a second way of carrying an
	// id in a shape neither the decoder nor the engine could check, with a
	// separator that was contract by accident. `query` is the observation's
	// name alone again.
	Args []string
}

// Claim is when one claim type settles.
type Claim struct {
	Timing string
	Bound  string

	// BoundFor is the DURATION form of Bound, parsed where it is validated and
	// kept rather than discarded. Zero for the quiesce form, for an absent
	// bound and for an immediate claim, none of which measure a duration.
	//
	// IT EXISTS BECAUSE THE PARSE WAS THROWN AWAY AND A READER PAID FOR IT.
	// This parser validated `bound = "30s"` and kept only the string, so the
	// walk's engine.Eventual carried Bound 0 and the run's refusal said "a
	// bound of 0" for every duration a project could declare (rook, coop
	// rumor/1191). One parse, one place, and the value travels with the
	// string it came from.
	BoundFor time.Duration
}

// Role is a role from the statute and the session contexts an actor of it may
// hold. The reachability arm -- a browser-stage actor that could reach an API
// action -- is computed over these and the actions' contexts.
type Role struct {
	Name     string
	Contexts []string
}

// Uses is the project's declared asks (R19, R20, R21).
type Uses struct {
	Faults  []string
	Volume  []string
	Upgrade []string
}

// Report is what reading a document produced: the bindings, and every reason it
// is not one. All of them rather than the first, as internal/platform and
// `rumor attaint` both do: a file that trips three refusals is a different
// event from one that trips the refusal it was written to trip.
type Report struct {
	Bindings Bindings
	Refusals []Refusal
}

// OK reports whether the document is a set of bindings.
func (r Report) OK() bool { return len(r.Refusals) == 0 }

// Load reads a package's bindings.toml and parses it. The decoder is the
// caller's, and the two error shapes are internal/platform's: an unreadable
// file is could-not-run, a readable file that is not bindings is a verdict.
func Load(path string, dec decl.Decoder) (Report, error) {
	doc, err := decl.Load(path, dec)
	if err != nil {
		return Report{}, err
	}
	return Parse(doc), nil
}

// Parse reads a decoded document.
// durationText reads an optional duration key and answers its RAW spelling.
//
// The value is validated by `decl.Duration` -- malformed, and zero or less,
// each refused in decl's own words at the key's own path -- and the string the
// declaration actually carries is what comes back, because that is what a
// refusal at a later tier quotes and `30s` and `30000ms` are not the same
// sentence to a reader even where they are the same number.
func durationText(r *decl.Reader, key string) string {
	r.Duration(key, false)
	s, _ := r.Any(key).(string)
	return s
}

func Parse(doc map[string]any) Report {
	r := decl.New(doc)
	var b Bindings

	for _, a := range r.Tables("action") {
		action := Action{
			Name:     a.Str("name", true),
			Context:  a.Str("context", true),
			Emits:    a.Strings("emits", false),
			Requires: a.Strings("requires", false),
			Creates:  a.Strings("creates", false),
			Touches:  a.Strings("touches", false),
		}
		if t, ok := a.Table("response", false); ok {
			action.Response = map[string]any{}
			for _, k := range t.Keys() {
				action.Response[k] = t.Any(k)
			}
			t.Done()
		}
		// OPTIONAL, AND BOTH KEYS REQUIRED ONCE THE TABLE IS THERE. An action
		// declaring it contends without saying where its limit is read, or
		// without saying what reads the held quantity, has declared half a
		// subject -- and a stage cannot judge an overfill against a limit it
		// cannot read. `decl.Str` with required=true refuses each at the key's
		// own path, which is where an author looks.
		if t, ok := a.Table("contended", false); ok {
			action.Contended = Contended{
				Limit: t.Str("limit", true),
				Held:  t.Str("held", true),
			}
			t.Done()
		}
		for _, g := range a.Tables("args") {
			action.Args = append(action.Args, arg(g))
		}
		for _, s := range a.Tables("flow") {
			action.Flow = append(action.Flow, step(s))
		}
		a.Done()
		b.Actions = append(b.Actions, action)
	}
	if len(b.Actions) == 0 {
		r.Refuse("action", "no actions: a binding with nothing to do binds nothing. "+
			"This is a parse refusal; validate's vacuity guard is the one that reports it as a refusal rather than a pass")
	}

	for _, o := range r.Tables("observation") {
		obs := Observation{
			Name:   o.Str("name", true),
			Source: wire.Source(o.Str("source", true)),
			Of:     o.Str("of", true),
			// OPTIONAL, like an action's `emits`: an observation of the whole
			// system declares none, and one of a particular entity declares the
			// arguments that name it.
			Args: o.Strings("args", false),
		}
		switch obs.Source {
		case wire.SourceAPI, wire.SourceUI:
			if o.Has("component") {
				o.Refuse("component", fmt.Sprintf(
					"a %s observation reads what an actor can see, so it names no component of the project's own store", obs.Source))
				o.Str("component", false)
			}
		case wire.SourceDirect:
			// REQUIRED FOR direct AND FOR NOTHING ELSE. A direct observation is
			// the engine reading the project's own store, and this field is the
			// only place a binding names a component -- without it validate's
			// rollback-coverage arm has nothing to compare and passes
			// vacuously. It is a PARSE refusal and not that arm: the arm is
			// about a named component no provider covers.
			obs.Component = o.Str("component", true)
		default:
			o.Refuse("source", fmt.Sprintf("%q: not one of api, ui, direct", obs.Source))
		}
		o.Done()
		b.Observations = append(b.Observations, obs)
	}

	b.Claims = map[string]Claim{}
	if claims, ok := r.Table("claim", false); ok {
		for _, name := range claims.Keys() {
			c, _ := claims.Table(name, true)
			claim := Claim{Timing: c.Str("timing", true), Bound: c.Str("bound", false)}
			switch claim.Timing {
			case Immediate:
				if claim.Bound != "" {
					c.Refuse("bound", "an immediate claim type settles at its action, so a bound says nothing")
				}
			case Eventual:
				// THE BOUND IS NOT REQUIRED HERE. "An eventual claim type with
				// no bound" is one of validate's arms, and a parser that
				// refused it would make that arm unreachable.
				if claim.Bound != "" && claim.Bound != Quiesce {
					d, err := time.ParseDuration(claim.Bound)
					switch {
					case err != nil:
						c.Refuse("bound", fmt.Sprintf(
							"%q: wanted a duration such as 30s, or %q", claim.Bound, Quiesce))
					case d <= 0:
						// A NON-POSITIVE BOUND IS REFUSED AT THE PARSE, and the
						// sentence is internal/decl's own: `Reader.Duration`
						// already refuses `d <= 0` with these exact words
						// (decl.go:280-281), so until now TWO duration readers in
						// one tree disagreed -- decl refused a zero and this
						// accepted it.
						//
						// WHY THE PARSE AND NOT A VALIDATE ARM: the engine reads
						// `Bound` as nanoseconds and compares `now-claimTime >
						// Bound` (engine/pending.go), so a declared `0s` makes
						// every disagreement instantly past its bound and a
						// negative one makes the comparison meaningless. Measured
						// before this refusal existed: `0s` made the toy's stale
						// bug VANISH at rc 0 and `-5s` found a DIFFERENT bug at a
						// different step, with `rumor validate` accepting both at
						// 22 of 22 arms. A value that changes which defect a run
						// reports must not reach the engine at all.
						//
						// AND IT GIVES schedule/walk.go's `!e.Quiesce &&
						// e.Bound > 0` ITS MEANING BACK: that test now reads
						// "a duration bound was DECLARED", because a declared
						// `0s` can no longer arrive as the same zero an absent
						// bound leaves behind.
						//
						// ratchet coop r7/29, quill r7/30, ibis r7/31, placed by
						// fable r7/32 as issue 101's added bullet.
						c.Refuse("bound", fmt.Sprintf(
							"%q: a duration of zero or less covers nothing", claim.Bound))
					default:
						// ASSIGNED ONLY WHERE IT IS USABLE. The old line sat
						// OUTSIDE the error check, so a refused parse and a
						// declared `0s` both left BoundFor at zero -- the same
						// conflation this bullet is about, one level down.
						claim.BoundFor = d
					}
				}
			case "":
				// already refused as a missing key
			default:
				c.Refuse("timing", fmt.Sprintf("%q: wanted %q or %q", claim.Timing, Immediate, Eventual))
			}
			c.Done()
			b.Claims[name] = claim
		}
		claims.Done()
	}

	for _, role := range r.Tables("role") {
		entry := Role{Name: role.Str("name", true), Contexts: role.Strings("contexts", true)}
		role.Done()
		b.Roles = append(b.Roles, entry)
	}

	for _, reserved := range []struct {
		key  string
		into *map[string]any
	}{
		{"clock", &b.Clock},
		{"quiesce", &b.Quiesce},
		{"deadline", &b.Deadline},
		{"refusal", &b.Refusal},
		{"seeding", &b.Seeding},
	} {
		if t, ok := r.Table(reserved.key, false); ok {
			// THE PROBES ARE TYPED AND THE REST OF THE TABLE IS STILL CARRIED.
			// [quiesce] was a reserved map read whole into Quiesce, so any key
			// passed and nothing was required: `{probe = "/test/quiet"}` was a
			// valid declaration that no code read. R6.2 types the probe array
			// and leaves the sibling keys carried, so a key a later phase
			// reserves here is not refused by this one.
			if reserved.key == "clock" {
				// OPTIONAL, so `decl` reports nothing for an absent key; the
				// value check is ours because decl reads a string, not an enum.
				form := ClockForm(t.Str("form", false))
				switch form {
				case "", ClockAdapter, ClockFaked:
					// EMPTY IS NOT A PARSE FAULT: the engine refuses a clocked
					// project whose form it cannot read, where the refusal is
					// reachable and can name what it was about to do.
					b.ClockForm = form
				default:
					t.Refuse("form", fmt.Sprintf("%q: wanted %q or %q -- a clock is "+
						"either one the engine moves through the adapter's `clock` request "+
						"or a stack under faked time, and nothing else can be seeded",
						form, ClockAdapter, ClockFaked))
				}
			}
			if reserved.key == "deadline" {
				// OPTIONAL AT THE PARSE, AND THE CONSUMERS ARE REAL. A name
				// here is an observation this file also declares, which is
				// `rumor validate`'s pairing to refuse -- it is the reader
				// that has both halves. A table with no `instants` in it
				// declares no deadline and is not a fault, the same way a
				// [quiesce] carrying only sibling keys declares no probe;
				// arm 20 still reads the table's CONTENTS, so a [deadline]
				// in a project with no clock control is refused whichever
				// of its keys is there (fable, coop rumor/1359).
				//
				// THE SHAPE IS REFUSED HERE AND NOWHERE ELSE: `decl.Strings`
				// names a scalar where an array was wanted and a non-string
				// element at its own index, so `instants = "x"` and
				// `instants = ["x", 3]` each refuse at the key's own path.
				b.DeadlineInstants = t.Strings("instants", false)
			}
			if reserved.key == "refusal" {
				// OPTIONAL AT THE PARSE, for the reason on the field: the walk
				// does not read it, so a project that never runs the fuzz
				// stage must not be refused for omitting it. `decl.Str` refuses
				// a non-string at the key's own path, which is the one shape
				// fault there is to find here -- whether the PATH resolves is a
				// fact about a live answer and belongs to the reader that has
				// one, not to this parser.
				b.RefusalMessage = t.Str("message", false)
			}
			if reserved.key == "quiesce" {
				for _, pr := range t.Tables("probe") {
					b.Probes = append(b.Probes, Probe{
						Kind: pr.Str("kind", true),
						// OPTIONAL AT PARSE AND REQUIRED BY validate, which is
						// not a contradiction: a missing timeout is a thing to
						// say about the PROJECT, so arm 7c names it with the
						// probe it belongs to and the fixture that trips it
						// tests the arm. Required here instead, the parser
						// refuses first and validate's refusal is unreachable
						// -- measured: the fixture tripped bindings-refused
						// and not 7c.
						// THROUGH decl.Duration, SO THE REFUSAL IS decl's OWN
						// WORDS AND THE KEY'S OWN PATH (fable's ruling, coop
						// rumor/1359 item 2, on ratchet's /1356 measurement:
						// `every = "0s"` and `timeout = "-5s"` each passed
						// `rumor validate` at 23 of 23 arms with 0 refusals,
						// and `internal/quiesce` re-parsed both with the error
						// DROPPED -- two readers disagreeing about one
						// declaration, which is what 101 closed for a claim's
						// bound).
						//
						// READ ONCE EACH. `Duration` is `Str` plus the parse,
						// so calling both would refuse a non-string TWICE for
						// one fault; the raw spelling comes from `Any`, which
						// refuses nothing and is what the run quotes back
						// ("its declared timeout of -5s").
						Timeout: durationText(pr, "timeout"),
						Every:   durationText(pr, "every"),
						Query:   pr.Str("query", false),
						Quiet:   pr.Any("quiet"),
					})
					pr.Done()
				}
			}
			carried := map[string]any{}
			for _, k := range t.Keys() {
				carried[k] = t.Any(k)
			}
			t.Done()
			*reserved.into = carried
		}
	}

	if uses, ok := r.Table("uses", false); ok {
		b.Uses = Uses{
			Faults:  uses.Strings("faults", false),
			Volume:  uses.Strings("volume", false),
			Upgrade: uses.Strings("upgrade", false),
		}
		uses.Done()
	}

	r.Done()

	// CROSS-KEY REFUSALS, after every key has been read. Each needs two parts of
	// the file, so none can be checked while reading either.
	//
	// WHAT IS NOT HERE IS THE POINT: an action's `emits` naming a claim type the
	// [claim] table does not describe is NOT refused, because "a rule references
	// a claim type no binding emits" and its mirror are validate's arms over the
	// statute and the bindings TOGETHER, and this package sees only one of the
	// two.
	seen := map[string]bool{}
	for _, a := range b.Actions {
		if seen[a.Name] {
			r.Refuse("action", fmt.Sprintf("%q is declared twice: an action name is how `execute` asks for it", a.Name))
		}
		seen[a.Name] = true
	}
	seenObs := map[string]bool{}
	for _, o := range b.Observations {
		if seenObs[o.Name] {
			r.Refuse("observation", fmt.Sprintf("%q is declared twice: an observation name is `observe`'s query", o.Name))
		}
		seenObs[o.Name] = true
	}
	seenRole := map[string]bool{}
	for _, role := range b.Roles {
		if seenRole[role.Name] {
			r.Refuse("role", fmt.Sprintf("%q is declared twice", role.Name))
		}
		seenRole[role.Name] = true
	}

	// AN ARGUMENT DECLARED TWICE, the same rule one level down and in the same
	// place as its three siblings. ExecuteArgs and ObserveArgs both build a map
	// keyed by the argument's NAME, so a second entry would silently overwrite
	// the first and the request would carry whichever the file happened to put
	// last. It is refused here, at load, because it is a defect in the
	// declaration: a walk-time check would make a project error look like a
	// could-not-run halfway through a run.
	for _, act := range b.Actions {
		seenArg := map[string]bool{}
		for _, ar := range act.Args {
			if ar.Name == "" {
				// `name` is required and arg() has already recorded its
				// absence; counting the blanks as duplicates of each other
				// would name one defect twice.
				continue
			}
			if seenArg[ar.Name] {
				r.Refuse("action", fmt.Sprintf("%q declares argument %q twice: the second entry "+
					"would overwrite the first where the request is built", act.Name, ar.Name))
			}
			seenArg[ar.Name] = true
		}
	}

	if found := r.Refusals(); len(found) > 0 {
		return Report{Refusals: found}
	}
	return Report{Bindings: b}
}

// step reads one UI flow step.
//
// EACH KIND'S OWN FIELDS AND NO OTHERS, because a `click` carrying a `value` is
// either a step written for the wrong kind or a field the driver will ignore,
// and an ignored field in a flow is a flow that does not do what it says.
// arg reads one `[[action.args]]` entry.
//
// THE SOURCE DECIDES WHICH FIELD IS READ, and `Done()` refuses every key this
// does not read -- so a `from` on a literal, or an `of` on a draw, is refused
// by name without a check of its own. That is the strict decoder doing the
// work, and it is why this reads exactly one field per source rather than
// reading all four and validating afterwards.
func arg(g *decl.Reader) Arg {
	a := Arg{
		Name:   g.Str("name", true),
		Source: g.Str("source", true),
	}
	switch a.Source {
	case ArgHandle:
		// A KIND, NOT A HANDLE. `of = "event"` and never
		// `of = "member_1.event#1"`: the walk picks WHICH event this actor
		// knows of, and a declaration naming one run's handle could not
		// survive a second run.
		a.Of = g.Str("of", true)
	case ArgLiteral:
		if !g.Has("value") {
			g.Refuse("value", "a literal argument needs a `value`: the source says the project "+
				"supplies it and then nothing does")
		}
		a.Value = g.Any("value")
	case ArgDraw:
		raw := g.Any("from")
		switch typed := raw.(type) {
		case []any:
			a.From = typed
		case nil:
			g.Refuse("from", "a draw argument needs a `from` list to draw out of")
		default:
			// THE REFUSAL IS ABOUT THE TYPE, NOT ABOUT CARDINALITY, and the
			// sentence used to say otherwise -- "a draw of one would spend the
			// generator on a choice that is not one" -- while the next clause
			// down accepts `from = ["x"]`, which IS a draw of one. ratchet
			// named the contradiction at coop r5/118 and fable ruled the
			// sentence narrowed to the code rather than the code widened to the
			// sentence (r5/119). A one-element list is accepted deliberately:
			// the TYPE is what says "this argument is drawn", and a project
			// extending the list later should not have to restate that.
			g.Refuse("from", fmt.Sprintf("`from` is %T and a draw needs a LIST: a scalar does not "+
				"say whether the project means one choice or the first of several, and `[%v]` "+
				"says the first", raw, raw))
		}
		if raw != nil && len(a.From) == 0 {
			g.Refuse("from", "a draw argument's `from` is empty: there is nothing to draw, and a "+
				"walk that drew from it could only refuse mid-step")
		}
	case ArgActor:
		// NOTHING ELSE TO READ, and that is the point: the value is the step's
		// own actor, so the project does not get to say what it is.
	case "":
		// `source` was required above and its refusal is already recorded;
		// adding a second here would name one defect twice.
	default:
		g.Refuse("source", fmt.Sprintf("%q is not an argument source; the four a walk can supply "+
			"are %s", a.Source, strings.Join(ArgSources, ", ")))
	}
	g.Done()
	return a
}

func step(s *decl.Reader) Step {
	out := Step{Step: s.Str("step", true)}
	switch out.Step {
	case StepNavigate:
		out.To = s.Str("to", true)
		if len(out.To) > 0 && out.To[0] != '/' {
			// THE PATH AND NOT A URL: the base belongs to the platform's
			// adapter, so a flow carrying a host would bind this statute to one
			// deployment.
			s.Refuse("to", fmt.Sprintf("%q: wanted a path beginning with /, not a URL -- the base is the adapter's", out.To))
		}
	case StepFill:
		out.Field = s.Str("field", true)
		out.Value = s.Str("value", true)
	case StepClick:
		out.Target = s.Str("target", true)
	case StepWait:
		out.Until = s.Str("until", true)
		// A CONDITION AND NEVER A DURATION. R9's flake policy says there are no
		// fixed sleeps, so a wait that parses as a duration is refused by name
		// rather than slept through.
		if out.Until != "" {
			if _, err := time.ParseDuration(out.Until); err == nil {
				s.Refuse("until", fmt.Sprintf(
					"%q is a duration: a wait step takes a CONDITION, because the browser stage has no fixed sleeps (R9)", out.Until))
			}
		}
	case "":
		// already refused as a missing key
	default:
		s.Refuse("step", fmt.Sprintf("%q: wanted one of %s, %s, %s, %s",
			out.Step, StepNavigate, StepFill, StepClick, StepWait))
	}
	s.Done()
	return out
}
