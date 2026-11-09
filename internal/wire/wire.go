// Package wire is the adapter contract: the messages an adapter speaks, the
// decoder that refuses a malformed one by name, and the version all three
// statements of the contract are held to.
//
// docs/contract.md is this package's prose and the two are meant to be read
// together; tests/contract checks that they agree rather than trusting that
// they do.
//
// WHY THE DECODER IS STRICT IN BOTH DIRECTIONS. An unknown field is an error
// here, not something ignored, for docs/coverage.toml's reason one level up: a
// lenient reader turns a typo into silence. The silence would be worse than an
// uncovered path -- an adapter sending `sorce` would observe nothing and the
// run would read as clean, because an absent finding is not a verdict and must
// not be read as one (R3.2).
//
// AND A DECODE REFUSAL IS COULD-NOT-RUN, NEVER A PROJECT REFUSAL. The project
// is not wrong; its adapter is. `rumor validate` carries that distinction in
// its exit status (R4.3), and this package carries it in its error type: a
// Refusal is about a message, and nothing here ever produces a finding.
package wire

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
)

// ContractVersion is this engine's statement of the contract version. The
// adapter's `contract` field and `contract` in platform.toml are the other two;
// CheckVersion compares all three.
//
// A STRING, NOT A NUMBER, so that a later form compares exactly without numeric
// coercion deciding whether 1 and 1.0 are the same version. v1's value is "1"
// and there is no compatibility window: see CheckVersion.
// 2 since 2026-10-19: `observe` gained `args`, so an observation can name its
// subject in a typed field instead of folding it into the query string (fable,
// coop r5.toy/8). docs/contract.md's Revisions carries the entry.
//
// IT STAYS "2" THROUGH R9.1's `flow`, which is R7.2's shape and not an
// oversight: the version is one string compared exactly with no compatibility
// window, so a bump refuses an adapter built before the request AT THE
// HANDSHAKE -- and then an adapter that merely lacks `flow` is refused for its
// VERSION and never for the thing it lacks. An unmoved version is what keeps
// that by-name refusal reachable, and `UnservedKind` below is the ONE site that
// gives it -- field `kind`, "%q is a kind this adapter does not serve" -- called
// by both adapters in this tree.
//
// WHAT THE WITNESS IS, AS OF R9.1's H2, because it MOVED and the citation here
// is where the version lives (rook, coop r9/39): this note used to name
// cmd/toy-adapter's fall-through, and that adapter now SERVES `flow`, because
// examples/toy declares an action with a flow and a declaration the walk honors
// is one the toy must honor too (fable, r9/40 part one). With it served, no
// adapter in this tree LACKS the kind by accident -- so the witness is
// `internal/fake` under its lacks-a-kind knob, an adapter at version 2 that
// refuses `flow` by name through the site above, armed by
// TestAKindThisAdapterDoesNotServeIsAFaultNamingTheKind. A knob whose purpose is
// a NAMED misbehavior is not a subject fabricated for the arm (rook r9/37,
// fable r9/40 part three). rook's reason at coop rumor/1348, their condition at
// r9/13, fable's ruling at r9/15 correcting r9/11, and r9/40 for the move.
// docs/contract.md's Revisions carries the row.
const ContractVersion = "2"

// Kind is a message's kind, which the decoder reads off the message rather than
// inferring from what was last sent.
type Kind string

const (
	KindDescribe Kind = "describe"
	KindExecute  Kind = "execute"
	KindObserve  Kind = "observe"
	KindClock    Kind = "clock"
	KindSession  Kind = "session"

	// KindFlow is R9's UI flow: the steps a browser driver executes for one
	// action, sent because the STEPS ARE THE PROJECT'S and the driver is the
	// adapter's. bindings declares them, the engine carries them, the adapter
	// drives them -- so neither side keeps a second copy of the other's.
	//
	// A SEPARATE OPERATION AND NOT A FIELD ON `execute` (fable, coop r9/11):
	// one message meaning either "perform this action" or "replay these steps"
	// would make every reader of `execute` ask which it was holding.
	KindFlow  Kind = "flow"
	KindError Kind = "error"
)

// Source is an observation source. The three are docs/vocabulary.md's, and this
// type exists so a fourth cannot arrive here without arriving there.
type Source string

const (
	SourceAPI     Source = "api"
	SourceBrowser Source = "browser"
	SourceDirect  Source = "direct"
)

// Stop is why a flow got no further, and the set is CLOSED for exactly the
// reason Source's is: the ENGINE branches on this value to decide whether what
// it is looking at becomes a `ui_timeout`, so an adapter free to invent a third
// word would be deciding a finding. A fourth cannot arrive here without
// arriving in docs/vocabulary.md first.
type Stop string

const (
	// StopNone is the zero value, and it means the flow did not stop short --
	// the state of every flow an adapter drove to its last step, which is why
	// the field is omitted for it and an adapter that never stops is unchanged.
	StopNone Stop = ""
	// StopWait is a wait whose condition was not met. This is the one the
	// engine re-checks at the same position, never re-sending the step before
	// it, and records as a `ui_timeout` on the second failure of the SAME wait
	// (fable, coop r9/1 item 2 and r9/21).
	StopWait Stop = "wait"
	// StopStep is a step the page would not perform, and it is NOT a
	// `ui_timeout`: `reached` alone cannot tell the two apart, and retrying
	// this one would re-send the request r9/1 item 2 forbids re-sending. What
	// the engine does with it is R9.1's next head and not this one's to decide.
	StopStep Stop = "step"
)

// ClockOp is what a clock request asks for, and the SHAPE is `observe`'s
// `source` rather than a field this protocol already has: one kind, one variant
// field, the variant deciding how the rest of the message reads. `op` itself is
// NEW -- no message has carried that name before, which ratchet measured at
// v0.1.39 (coop r7/50) when I had written "observe is the precedent" loosely
// enough to be read as "op already exists".
type ClockOp string

const (
	// ClockSet names an instant; ClockAdvance names an amount. Both carry it in
	// the request's single `t`, because the op already says which and two
	// fields would let one message carry both and mean neither.
	ClockSet     ClockOp = "set"
	ClockAdvance ClockOp = "advance"
)

// FaultKind is the error envelope's kind: what separates could-not-run from a
// refusal, which fable's D7 drill asserts by name.
type FaultKind string

const (
	FaultContract FaultKind = "contract"
	FaultField    FaultKind = "field"
	FaultSource   FaultKind = "source"
	FaultSession  FaultKind = "session"
	FaultAction   FaultKind = "action"
	FaultInternal FaultKind = "internal"
)

// Envelope is the three fields every message carries in both directions.
type Envelope struct {
	Kind     Kind   `json:"kind"`
	Contract string `json:"contract"`
	ID       string `json:"id"`
}

// Env lets every message answer for its envelope, which is all the transport
// needs of a message it is about to write or has just read.
func (e Envelope) Env() Envelope { return e }

// Message is anything with an envelope: the four request kinds, the four
// response kinds, and the error envelope.
type Message interface{ Env() Envelope }

// --- requests -------------------------------------------------------------

// DescribeRequest asks the adapter what it can do. It carries the envelope and
// nothing else: there is nothing to ask.
type DescribeRequest struct {
	Envelope
}

// ExecuteRequest takes one action. Token is present exactly when the action's
// context is not "none", and is also WHO acts -- there is no actor field,
// because the engine knows which actor it acquired the token for and a second
// statement of one fact is a channel for the two to disagree.
//
// Args carries the system's own ids, never run-local handles: the engine owns
// the handle table (docs/design.md) and resolves a handle to an id before the
// message is written.
type ExecuteRequest struct {
	Envelope
	Action string         `json:"action"`
	Token  string         `json:"token,omitempty"`
	Args   map[string]any `json:"args"`
}

// FlowRequest asks the adapter to drive one action's declared UI flow (R9.1,
// issue 120 deliverable 2; fable's ruling at coop r9/11).
//
// Token IS THE BROWSER'S AND NEVER THE API's, which is the same rule
// ObserveResponse's row states for a `browser` source: reusing an actor's api
// token here would put one actor in two worlds and retire the one-isolated-
// context-per-actor property the stage rests on.
//
// Steps ARE CARRIED OPAQUE at this layer. internal/bindings has already parsed
// and refused them -- a `wait` whose condition parses as a duration never
// reaches here -- so re-typing them in the transport would be a second reader
// of one declaration, which is the drift docs/contract.md's own sibling rule
// warns about. The adapter refuses a step it does not know, by name.
// From IS WHERE THE FLOW RESUMES, and it exists because fable ruled at coop
// r9/21 that THE RETRIER IS THE ENGINE. A wait is bounded in the engine's time
// -- checked at each seal and after each clock advance the engine makes (r9/1
// item 3) -- so an adapter re-checking inside one call has nothing advance
// between its two looks and reads the same world twice. The engine therefore
// seals, advances, and sends the flow again FROM the unmet wait; r9/1 item 2
// forbids re-sending the steps before it, and `from` is how they are not sent.
//
// ZERO IS THE WHOLE FLOW and the field is omitted for it, so an adapter that
// has never heard of resuming is unchanged -- which is why ContractVersion
// stays at 2, the same reason R7.2 gave for an additive request field: a
// by-name refusal of an unknown field has to stay reachable.
type FlowRequest struct {
	Envelope
	Action string           `json:"action"`
	Token  string           `json:"token,omitempty"`
	From   int              `json:"from,omitempty"`
	Steps  []map[string]any `json:"steps"`
}

// FlowResponse is how far the flow got and what the page then answered.
//
// Reached IS AN INDEX AND NOT A BOOLEAN, because the retry policy needs to know
// WHERE a flow stopped: fable ruled at coop r9/1 that a failed wait re-checks
// its condition at the same position and never re-sends the step before it, so
// "it failed" is not enough to act on and "it failed at step k" is.
// Stopped IS WHY IT GOT NO FURTHER, and `reached` alone cannot say: a wait
// whose condition was not met and a step the page would not perform both stop
// at the same index, and only the first becomes a `ui_timeout`. Without it the
// engine would have to guess which finding it is looking at.
//
// IT IS OPTIONAL AND EMPTY MEANS "it did not stop", which is the state of every
// flow that reached its last step -- so an adapter that completes each flow in
// one call never sends it and is unchanged by this field's existence.
type FlowResponse struct {
	Envelope
	Reached  int  `json:"reached"`
	Stopped  Stop `json:"stopped,omitempty"`
	Response any  `json:"response"`
}

// ObserveRequest reads one named observation. Token is present exactly when
// Source is not "direct": api and browser are an actor's observations through its
// session, and direct is the engine's, never an actor's (R9).
//
// Query is opaque here when Source is "direct": that verb's shape is an epic
// Open item, so the decoder checks presence and leaves the shape alone.
type ObserveRequest struct {
	Envelope
	Query  string `json:"query"`
	Source Source `json:"source"`
	Token  string `json:"token,omitempty"`

	// Args carries the observation's declared arguments, the same row as an
	// execute's: the system's own ids, never run-local handles, because the
	// engine owns the handle table and resolves a handle before the message is
	// written (fable, coop r5.toy/8).
	//
	// THE CONTRACT SAYS "PRESENT EXACTLY WHEN THE OBSERVATION DECLARES ANY" AND
	// THE DECODER CANNOT CHECK THAT, which is why it is optional here rather
	// than required. Whether an observation declares arguments is in the
	// project's bindings, and this decoder reads ONE message without the
	// bindings and without the adapter's describe -- the same boundary that
	// leaves an execute's token-against-context to the engine. `rumor
	// validate` has both halves and is where the pairing belongs.
	Args map[string]any `json:"args,omitempty"`
}

// SessionRequest acquires a session. Actor enters the protocol here and only
// here; every later message carries the token instead.
type SessionRequest struct {
	Envelope
	Actor   string `json:"actor"`
	Context string `json:"context"`
}

// --- responses ------------------------------------------------------------

// Action is one action the adapter can execute. Context names the session
// context it needs, or "none".
type Action struct {
	Name    string `json:"name"`
	Context string `json:"context"`
}

// Observation is one observation the adapter can make, and through which source.
type Observation struct {
	Name   string `json:"name"`
	Source Source `json:"source"`

	// Page and Element are how a BROWSER observation is read, and the adapter
	// reports them because the adapter is the one thing that knows its own UI
	// (fable, coop r9/120, ruling on ibis r9/119).
	//
	// Page is a path relative to `base` with the subject as a placeholder --
	// the toy's "/ui/events/{subject}" -- and Element is an element ID on that
	// page. The driver navigates the ACTOR'S OWN context to the page and reads
	// the element's text content from the LIVE DOM, and that is the observation.
	//
	// AN ID AND NOT A CSS SELECTOR, in the same vocabulary as a flow's `until`:
	// one word for one thing. A selector would make the adapter's answer a
	// query language the engine has to implement, and two spellings of "which
	// element" is how a reader ends up unable to tell a missing element from a
	// mistyped selector.
	//
	// ADDITIVE, SO THE CONTRACT STAYS "2": an adapter reporting neither is
	// unchanged, and a browser observation whose describe carries no page or
	// element is a NAMED REFUSAL at the walk -- never an invented path or
	// selector, for `base`'s own reason.
	Page    string `json:"page,omitempty"`
	Element string `json:"element,omitempty"`

	// Type is what a reading MEANS -- `number`, `text` or `count` -- and it is
	// declared rather than guessed (fable, coop r9/122: "NO SNIFFING", and
	// r9/124 for the third word).
	//
	// WHY DECLARED. A browser reads TEXT, and the statute compares a count: the
	// toy's statute relies on "a string never equals a count", so a reading
	// that arrived as text would reach `consistent` never and `bug` always. The
	// engine converts a `number` observation's text in ONE place and refuses by
	// name any text that is not wholly numeric -- "0 of 2" is a refusal, not a
	// bug and not a zero -- and a `text` observation stays text, reaching only
	// judgements written for a string.
	//
	// AND SNIFFING WOULD BE WORSE THAN EITHER. A rule that converted whatever
	// LOOKED numeric would make the reading's type a property of the page's
	// content: an attendee list rendering a single name that happens to be "7"
	// would become the number seven, and the statute's protection would hold or
	// fail by accident. The declaration is what makes "a number" a fact about
	// the observation rather than about today's data.
	Type string `json:"type,omitempty"`
}

// ObservedNumber and ObservedText are the two readings an element's text can
// carry, and they are spelled ONCE here: the adapter writes one of them, the
// engine branches on it, and a third spelling would be a type nobody converts.
const (
	ObservedNumber = "number"
	ObservedText   = "text"

	// ObservedCount is the number of the element's CHILD ELEMENTS, which is
	// what the adapter has computed from the served HTML all along -- the <li>
	// children of the toy's #attendees (fable, coop r9/124).
	//
	// IT EXISTS SO THE SUT DOES NOT CHANGE TO BE OBSERVED. A list has no figure
	// in it, so reading its text as a number is a refusal and reading it as
	// text reaches no judgement written for a count; the alternatives were to
	// make the application RENDER a number it never showed, or to move the
	// statute. Counting the children is the reading the other oracle already
	// takes, so the two stay comparable and the statute holds unchanged.
	ObservedCount = "count"
)

// Component is one part of the system under test. Stateful is what the validate
// arm on rollback coverage compares against platform.toml's [[rollback]]
// entries; without it that arm has nothing to compare and passes vacuously.
type Component struct {
	Name     string `json:"name"`
	Stateful bool   `json:"stateful"`
}

// Capabilities is what the adapter can actually DO, as opposed to what the
// project declares it wants done. Each list may be empty, and empty is an
// ANSWER: the validate arm on a declared fault, volume target or upgrade with
// nothing behind it fires on an empty list and would be vacuous on an absent
// field, which is why all three are required rather than omitempty.
//
// WHY HERE AND NOT IN platform.toml (fable, coop breakout-r4/3): the arm asks
// whether there is anything behind a declaration, and a second static file is
// another declaration rather than something behind the first.
type Capabilities struct {
	Faults  []string `json:"faults"`
	Volume  []string `json:"volume"`
	Upgrade []string `json:"upgrade"`
}

// DescribeResponse is the adapter's declared surface.
type DescribeResponse struct {
	Envelope
	Actions      []Action      `json:"actions"`
	Observations []Observation `json:"observations"`
	Contexts     []string      `json:"contexts"`
	Components   []Component   `json:"components"`
	Capabilities Capabilities  `json:"capabilities"`

	// Base is where the adapter's world is reachable over HTTP, and it is here
	// because THE ADAPTER IS THE ONE THING THAT KNOWS: it either started the
	// system itself or was handed an address for one, and nothing else in a run
	// sees both cases (fable, coop r9/111, ruling on ibis rumor/1943).
	//
	// IT EXISTS FOR A BROWSER TO NAVIGATE AGAINST. A declared flow's `navigate`
	// step resolves against it, and before this field the engine had no address
	// at all -- `platform.Adapter` carries a command, a directory and an
	// environment, and the one stage that knows an address starts its own world
	// to get it. The alternatives were refused by name: declaring it in
	// platform.toml duplicates what the adapter already holds and will drift,
	// and making the walk own the world moves ownership for a reason that is
	// not the walk's.
	//
	// ADDITIVE, SO THE CONTRACT STAYS "2" (same ruling, and the same shape as
	// ObserveResponse.Missing below): an adapter that never sets it is
	// unchanged, `omitempty` keeps it off the wire for every existing answer,
	// and no persisted artifact gains a field. The strict decoder is what makes
	// the addition visible rather than silent -- before it, an adapter sending
	// `base` was REFUSED as an unknown field.
	//
	// AN ABSENT BASE IS NOT A DEFAULT. There is no localhost to fall back to: a
	// flow that navigates against an adapter whose describe carries no base is
	// a NAMED REFUSAL at the walk, never a guessed host or port.
	Base string `json:"base,omitempty"`
}

// ExecuteResponse reports what the action did. IDs is keyed by the names the
// action declares it creates -- not by handle, which never crosses the wire --
// and the keys exist at all because an action that creates two entities could
// not say which id was which without them.
type ExecuteResponse struct {
	Envelope
	IDs      map[string]any `json:"ids"`
	Response any            `json:"response"`
}

// ObserveResponse carries the value and nothing else: not a position, which the
// ledger assigns on append, and not a source, which the engine asked for. An
// adapter that cannot observe through the source it was handed answers an
// error envelope with FaultSource instead of answering through another channel.
type ObserveResponse struct {
	Envelope
	Value any `json:"value"`

	// Missing says the adapter could not SERVE the subject, and fable ruled it
	// an ANSWER rather than a fault (coop rumor/1669): a page that is not
	// served is what a browser sees, so it is what a reader of the ledger
	// should see too.
	//
	// WHY A FIELD AND NOT THE VALUE ITSELF. The walk turns this into the
	// observation's value, spelled ValueMissing, and the adapter never writes
	// that word -- the SPELLING belongs to docs/vocabulary.md and the engine
	// that compares it, not to each adapter. An adapter reporting the string
	// would make every adapter a second place the word could be misspelled,
	// and a misspelling would read as an ordinary value a statute simply does
	// not match.
	//
	// ADDITIVE, SO THE CONTRACT STAYS "2" (fable, same ruling): an adapter that
	// never sets it is unchanged, `omitempty` keeps it off the wire for every
	// existing answer, and no persisted artifact gains a field.
	//
	// IT IS NOT AN ERROR AND NOT A ZERO. A fault would refuse the run -- the
	// walk has no skip for one -- and a zero would make a page the observer
	// cannot see read as an empty one, which is the reading a visibility
	// finding would then rest on.
	Missing bool `json:"missing,omitempty"`
}

// ValueMissing is the observation value the walk records for a subject the
// adapter could not serve. It is A VALUE A STATUTE MAY COMPARE AND NOT THE
// ABSENCE OF ONE, which is the half of fable's ruling that decides how a
// statute is written against it: a rule may ask whether the reading is
// `missing` and be right about a deleted subject, where a rule reading silence
// has nothing to ask.
const ValueMissing = "missing"

// ClockRequest asks the app-exposed test clock to move. It is the envelope plus
// two fields and nothing else.
//
// `T` IS NANOSECONDS, which docs/contract.md declared at R7.1 before any writer
// existed: "clock.t is nanoseconds from the run's own epoch, zero at the first
// seal". A second unit on this path is where an off-by-a-thousand lives, and
// the engine's own `Bound` has carried nanoseconds since R6 for the same
// reason -- `time.ParseDuration` yields them.
type ClockRequest struct {
	Envelope
	Op ClockOp `json:"op"`
	T  int64   `json:"t"`
}

// ClockResponse carries the time the clock ACTUALLY holds after the request,
// which is not necessarily the time that was asked for: an adapter may clamp,
// round, or refuse to go backwards. Who writes it to the ledger is open at the
// time of writing (ibis, coop r7/49) -- today's writer uses the run's own copy
// (internal/schedule/walk.go's `t := c.t`), and ratchet r7/50's observation is
// that making the response authoritative moves where that value comes from
// rather than adding a field.
type ClockResponse struct {
	Envelope
	T int64 `json:"t"`
}

// SessionResponse carries the token the later messages speak through.
type SessionResponse struct {
	Envelope
	Token string `json:"token"`
}

// Fault is the error envelope's body.
type Fault struct {
	Kind   FaultKind `json:"kind"`
	Field  string    `json:"field,omitempty"`
	Detail string    `json:"detail"`
}

// ErrorResponse may replace any response. EVERY envelope is could-not-run: an
// adapter can neither refuse a project nor produce a finding, because a refusal
// is `rumor validate`'s verdict over declared files and a finding is the
// statute's over a sealed ledger.
type ErrorResponse struct {
	Envelope
	Error Fault `json:"error"`
}

// --- refusals -------------------------------------------------------------

// Refusal is one decode refusal, which names the message kind, the field and
// which fault it was. The three are separate fields rather than one sentence
// because a caller reports them (R4.3 names the arm that fired) and a test
// asserts them.
type Refusal struct {
	Kind   Kind   // empty when the message's kind could not be read
	Field  string // empty where the refusal is not about one field
	Reason string
}

func (r Refusal) Error() string {
	where := "message"
	if r.Kind != "" {
		where = string(r.Kind) + " message"
	}
	if r.Field != "" {
		return fmt.Sprintf("wire: %s: %s %q", where, r.Reason, r.Field)
	}
	return fmt.Sprintf("wire: %s: %s", where, r.Reason)
}

// --- the schema -----------------------------------------------------------

// shape is one message's field set. The decoder reads unknown and missing
// fields off these, and tests/contract checks them against docs/contract.md's
// tables, so the table in the document and the table here convict each other.
type shape struct {
	required []string
	optional []string
}

// requests and responses are the two directions. They are separate maps because
// the same kind has different fields each way -- an execute request carries
// `action`, its response carries `ids` -- so a decoder that did not know its
// direction could not tell a missing field from a well-formed one going the
// other way.
var requests = map[Kind]shape{
	KindDescribe: {required: []string{"kind", "contract", "id"}},
	KindExecute: {
		required: []string{"kind", "contract", "id", "action", "args"},
		optional: []string{"token"},
	},
	KindObserve: {
		required: []string{"kind", "contract", "id", "query", "source"},
		optional: []string{"token", "args"},
	},
	KindClock:   {required: []string{"kind", "contract", "id", "op", "t"}},
	KindSession: {required: []string{"kind", "contract", "id", "actor", "context"}},
	KindFlow: {
		required: []string{"kind", "contract", "id", "action", "steps"},
		optional: []string{"token", "from"},
	},
}

var responses = map[Kind]shape{
	// `base` IS OPTIONAL AND ADDITIVE, so contract "2" stands (fable, coop
	// r9/111): an adapter that reports no address is unchanged and every
	// existing describe still decodes. THE TABLE IS THE SECOND INSTRUMENT over
	// the struct tag, and it caught this addition exactly as it caught
	// `missing` below -- the field went on the struct first and a real
	// toy-adapter's describe was refused for a field the struct could hold.
	KindDescribe: {
		required: []string{"kind", "contract", "id", "actions", "observations", "contexts", "components", "capabilities"},
		optional: []string{"base"},
	},
	KindExecute: {required: []string{"kind", "contract", "id", "ids", "response"}},
	KindFlow: {
		required: []string{"kind", "contract", "id", "reached", "response"},
		optional: []string{"stopped"},
	},
	// `missing` IS OPTIONAL AND `value` STAYS REQUIRED, which is what keeps this
	// additive: an adapter that never answers `missing` is unchanged, and one
	// that does still sends `value` (null). THE TABLE IS THE SECOND INSTRUMENT
	// over the same line -- `into` reads the struct tags -- and this package
	// says in its own words that a disagreement between them is a defect HERE.
	// It was: the field went on the struct first and the strict decoder refused
	// a message the struct could hold, which is the pairing working.
	KindObserve: {required: []string{"kind", "contract", "id", "value"}, optional: []string{"missing"}},
	KindClock:   {required: []string{"kind", "contract", "id", "t"}},
	KindSession: {required: []string{"kind", "contract", "id", "token"}},
	KindError:   {required: []string{"kind", "contract", "id", "error"}},
}

// --- decoding -------------------------------------------------------------

// DecodeRequest reads one request line, as an adapter does. DecodeResponse
// reads one response line, as the engine does.
func DecodeRequest(line []byte) (Message, error) { return decode(line, requests, false) }

// DecodeResponse reads one response line. The error envelope is a response and
// never a request: an adapter reports that it could not act, and the engine has
// nothing to report to an adapter.
func DecodeResponse(line []byte) (Message, error) { return decode(line, responses, true) }

func decode(line []byte, table map[Kind]shape, isResponse bool) (Message, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(line, &raw); err != nil {
		return nil, Refusal{Reason: fmt.Sprintf("not a JSON object: %v", err)}
	}
	kindRaw, ok := raw["kind"]
	if !ok {
		return nil, Refusal{Field: "kind", Reason: "missing field"}
	}
	var kind Kind
	if err := json.Unmarshal(kindRaw, &kind); err != nil {
		return nil, Refusal{Field: "kind", Reason: "not a string"}
	}
	want, ok := table[kind]
	if !ok {
		return nil, Refusal{Kind: kind, Reason: "unknown kind for this direction"}
	}
	if err := want.check(kind, raw); err != nil {
		return nil, err
	}
	msg, err := into(kind, isResponse, line, want.allowed())
	if err != nil {
		return nil, err
	}
	if err := semantics(msg); err != nil {
		return nil, err
	}
	return msg, nil
}

// check is the whole unknown-and-missing test, and it names ONE field at a
// time, lowest first by sorted order: a refusal listing three fields reads as
// one fault with three parts, and the caller then has to parse the sentence to
// act on any of them.
func (s shape) check(kind Kind, raw map[string]json.RawMessage) error {
	allowed := s.allowed()
	var unknown []string
	for f := range raw {
		if !allowed[f] {
			unknown = append(unknown, f)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return Refusal{Kind: kind, Field: unknown[0], Reason: "unknown field"}
	}
	for _, f := range s.required {
		if _, ok := raw[f]; !ok {
			return Refusal{Kind: kind, Field: f, Reason: "missing field"}
		}
	}
	return nil
}

// allowed is the field set, required and optional together. It is one function
// so that the decoder's two readers of it -- the unknown-field test and the
// whose-fault test below -- cannot drift apart.
func (s shape) allowed() map[string]bool {
	out := make(map[string]bool, len(s.required)+len(s.optional))
	for _, f := range s.required {
		out[f] = true
	}
	for _, f := range s.optional {
		out[f] = true
	}
	return out
}

// into unmarshals into the concrete type. DisallowUnknownFields is set here as
// a SECOND INSTRUMENT over the same line: check above reads the schema table,
// this reads the struct tags, and a disagreement between them is a defect in
// this package rather than in the message. It is reported as such.
func into(kind Kind, isResponse bool, line []byte, allowed map[string]bool) (Message, error) {
	var msg Message
	switch {
	case isResponse && kind == KindDescribe:
		msg = &DescribeResponse{}
	case isResponse && kind == KindExecute:
		msg = &ExecuteResponse{}
	case isResponse && kind == KindObserve:
		msg = &ObserveResponse{}
	case isResponse && kind == KindClock:
		msg = &ClockResponse{}
	case isResponse && kind == KindSession:
		msg = &SessionResponse{}
	case isResponse && kind == KindFlow:
		msg = &FlowResponse{}
	case isResponse && kind == KindError:
		msg = &ErrorResponse{}
	case kind == KindDescribe:
		msg = &DescribeRequest{}
	case kind == KindExecute:
		msg = &ExecuteRequest{}
	case kind == KindObserve:
		msg = &ObserveRequest{}
	case kind == KindClock:
		msg = &ClockRequest{}
	case kind == KindSession:
		msg = &SessionRequest{}
	case kind == KindFlow:
		msg = &FlowRequest{}
	default:
		return nil, Refusal{Kind: kind, Reason: "unknown kind for this direction"}
	}
	dec := json.NewDecoder(bytes.NewReader(line))
	dec.DisallowUnknownFields()
	if err := dec.Decode(msg); err != nil {
		return nil, unmarshalRefusal(kind, allowed, err)
	}
	return deref(msg), nil
}

// unmarshalRefusal reads the unmarshal's complaint and says WHOSE fault it is.
//
// THE DISTINCTION IS NOT COSMETIC. check above has already approved the
// top-level key set, so an unknown field reaching here is one of two different
// events: a field the TABLE allows and no struct carries, which is a defect in
// this package, or a field nested inside one -- `capabilities.fault` for
// `capabilities.faults` -- which is the adapter's. The first cut reported both
// as this package's defect, which would have told an adapter author to go and
// read our source. The one-field-at-a-time rule from check holds here too.
func unmarshalRefusal(kind Kind, allowed map[string]bool, err error) error {
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		field := typeErr.Field
		if field == "" {
			field = typeErr.Struct
		}
		return Refusal{Kind: kind, Field: field,
			Reason: fmt.Sprintf("wrong type: %s is not a %s for", typeErr.Value, typeErr.Type)}
	}
	if name, ok := unknownFieldName(err.Error()); ok {
		if _, nested := splitNested(name); !nested && allowed[name] {
			return Refusal{Kind: kind, Field: name,
				Reason: "the schema allows this field and no struct carries it, which is a defect in package wire:"}
		}
		return Refusal{Kind: kind, Field: name, Reason: "unknown field"}
	}
	return Refusal{Kind: kind, Reason: fmt.Sprintf("malformed: %v", err)}
}

// unknownFieldName pulls the field out of encoding/json's own sentence, which
// is `json: unknown field "sorce"`. Matching the text is the only route: the
// package returns a bare error for this case and has since Go 1.10.
func unknownFieldName(text string) (string, bool) {
	const prefix = `json: unknown field "`
	i := strings.Index(text, prefix)
	if i < 0 {
		return "", false
	}
	rest := text[i+len(prefix):]
	j := strings.Index(rest, `"`)
	if j < 0 {
		return "", false
	}
	return rest[:j], true
}

// splitNested reports whether a field name encoding/json gave us is nested. The
// decoder reports nested unknown fields by their LEAF name, so the only
// available test is whether the name is one of this message's own top-level
// fields -- which the caller does with `allowed`. This helper exists to keep
// that reasoning in one place rather than inlining a comment-free condition.
func splitNested(name string) (string, bool) {
	if i := strings.Index(name, "."); i >= 0 {
		return name[:i], true
	}
	return name, false
}

// deref returns the value rather than the pointer, so a caller's type switch
// reads `case ObserveRequest` and not `case *ObserveRequest`. The decode needs
// the pointer; nothing after it does.
func deref(msg Message) Message {
	switch m := msg.(type) {
	case *DescribeRequest:
		return *m
	case *ExecuteRequest:
		return *m
	case *ObserveRequest:
		return *m
	case *ClockRequest:
		return *m
	case *SessionRequest:
		return *m
	case *FlowRequest:
		return *m
	case *DescribeResponse:
		return *m
	case *FlowResponse:
		return *m
	case *ExecuteResponse:
		return *m
	case *ObserveResponse:
		return *m
	case *ClockResponse:
		return *m
	case *SessionResponse:
		return *m
	case *ErrorResponse:
		return *m
	}
	return msg
}

// semantics is the part of the contract the decoder can check on ONE message,
// without the bindings and without the adapter's describe answer.
//
// What it deliberately does NOT check: that an execute's token matches its
// action's context, which needs the action's declared context and is therefore
// the engine's check and not the decoder's. Stating that here is the narrowing;
// the token stays optional at the wire.
func semantics(msg Message) error {
	switch m := msg.(type) {
	case ObserveRequest:
		switch m.Source {
		case SourceAPI, SourceBrowser:
			if m.Token == "" {
				return Refusal{Kind: KindObserve, Field: "token",
					Reason: fmt.Sprintf("missing field for source %q, which is an actor's observation through its session", m.Source)}
			}
		case SourceDirect:
			if m.Token != "" {
				return Refusal{Kind: KindObserve, Field: "token",
					Reason: `present for source "direct", which is the engine's observation and never an actor's`}
			}
		default:
			return Refusal{Kind: KindObserve, Field: "source",
				Reason: fmt.Sprintf("not one of api, browser, direct: %q", m.Source)}
		}
	case DescribeResponse:
		for _, o := range m.Observations {
			switch o.Source {
			case SourceAPI, SourceBrowser, SourceDirect:
			default:
				return Refusal{Kind: KindDescribe, Field: "observations",
					Reason: fmt.Sprintf("observation %q declares a source that is not one of api, browser, direct: %q", o.Name, o.Source)}
			}
			// PAGE AND ELEMENT BELONG TO A BROWSER OBSERVATION AND NOTHING
			// ELSE. They say how a BROWSER reads a page; an `api` or `direct`
			// observation has no page, so carrying them there is the adapter
			// being inconsistent with itself rather than a message the engine
			// should act on.
			if o.Source != SourceBrowser && (o.Page != "" || o.Element != "" || o.Type != "") {
				return Refusal{Kind: KindDescribe, Field: "observations",
					Reason: fmt.Sprintf("observation %q is source %q and carries page, element or "+
						"type, which say how a BROWSER reads a page", o.Name, o.Source)}
			}
			// THE TWO READINGS AND NOTHING ELSE. A third word would be a type
			// the engine does not convert, and an adapter that invented one
			// would have its readings pass through as text while its describe
			// looked deliberate.
			switch o.Type {
			case "", ObservedNumber, ObservedText, ObservedCount:
			default:
				return Refusal{Kind: KindDescribe, Field: "observations",
					Reason: fmt.Sprintf("observation %q declares type %q: a reading is %q, %q "+
						"or %q, and the engine converts by that declaration rather than by what "+
						"the text looks like", o.Name, o.Type,
						ObservedNumber, ObservedText, ObservedCount)}
			}
			// AN ELEMENT IS AN ID, NOT A SELECTOR (fable, coop r9/120). The
			// driver builds "#"+element, so a selector here would be a second
			// query language arriving through the adapter -- and `#count .n`
			// would read as an element nobody can find.
			if strings.ContainsAny(o.Element, "#.[]()>+~*,:\"' \t") {
				return Refusal{Kind: KindDescribe, Field: "observations",
					Reason: fmt.Sprintf("observation %q names element %q: an element is an ID in "+
						"the same vocabulary as a flow's `until`, one word for one thing, and "+
						"never a CSS selector", o.Name, o.Element)}
			}
			// AND THE PAGE IS RELATIVE TO `base`, which is what keeps the
			// address in one place: a page carrying its own scheme would be a
			// second answer to "where is the system", and the walk refuses a
			// guessed host for exactly that reason.
			if strings.Contains(o.Page, "://") {
				return Refusal{Kind: KindDescribe, Field: "observations",
					Reason: fmt.Sprintf("observation %q names page %q: a page is a path relative "+
						"to `base`, and an absolute URL would be a second address", o.Name, o.Page)}
			}
		}
	case FlowResponse:
		switch m.Stopped {
		case StopNone, StopWait, StopStep:
		default:
			return Refusal{Kind: KindFlow, Field: "stopped",
				Reason: fmt.Sprintf("not one of wait, step: %q -- the engine branches on this "+
					"to decide whether the finding is a ui_timeout, so an adapter cannot widen it", m.Stopped)}
		}
		if m.Reached < 0 {
			return Refusal{Kind: KindFlow, Field: "reached",
				Reason: fmt.Sprintf("negative: %d -- it is an index into the steps that were sent", m.Reached)}
		}
	case FlowRequest:
		if m.From < 0 {
			return Refusal{Kind: KindFlow, Field: "from",
				Reason: fmt.Sprintf("negative: %d -- it is the position the flow resumes at, and 0 is the whole flow", m.From)}
		}
		// ONLY WHEN THERE ARE STEPS, and the reason covers exactly that
		// narrowing: `from: 0` on a flow of NO steps satisfies `0 >= 0` and this
		// clause refused it by naming `from`, where the fault is the absent
		// steps and the adapter already names them. Measured -- it broke
		// internal/fake's own zero-step case the moment the clause was added,
		// which is the boundary this project keeps finding under `>=`.
		if len(m.Steps) > 0 && m.From >= len(m.Steps) {
			return Refusal{Kind: KindFlow, Field: "from",
				Reason: fmt.Sprintf("%d with %d step(s): a flow cannot resume past its own end, and the "+
					"engine only resumes at a position an earlier answer reached", m.From, len(m.Steps))}
		}
	case ErrorResponse:
		switch m.Error.Kind {
		case FaultContract, FaultField, FaultSource, FaultSession, FaultAction, FaultInternal:
		default:
			return Refusal{Kind: KindError, Field: "error.kind",
				Reason: fmt.Sprintf("not one of contract, field, source, session, action, internal: %q", m.Error.Kind)}
		}
	}
	return nil
}

// UnservedKind is the fault an adapter answers for a request kind it does not
// serve, and it is HERE because it is load-bearing for the contract version:
// wire.go's note above ContractVersion rests on an adapter that LACKS a request
// being refused by NAME rather than by its version, and a refusal whose wording
// lives in each adapter separately is a refusal that can drift out from under
// that argument one adapter at a time.
//
// IT WAS TWO SITES SAYING TWO THINGS (rook, coop r9/39, measured at fdfc6e3).
// `cmd/toy-adapter` said "%q is a kind this adapter does not serve" with field
// `kind`, which is the wording the arm for this asserts; `internal/fake` said
// "the fake adapter has no answer for a %s request" with NO field, under a
// comment calling its own default unreachable. One of the two satisfied the arm
// and the other could not have, and nothing paired them.
//
// THE FIELD IS `kind` AND THE CLASS IS INTERNAL: the message was well formed and
// the engine was entitled to send it, so this is not the caller's fault -- it is
// the adapter saying what it cannot do, which is a could-not-run for the run and
// never a refusal of the project (fable, coop r9/40 part two).
func UnservedKind(kind Kind) Fault {
	return Fault{
		Kind:   FaultInternal,
		Field:  "kind",
		Detail: fmt.Sprintf("%q is a kind this adapter does not serve", kind),
	}
}

// --- the version check ----------------------------------------------------

// CheckVersion compares the three statements of the contract version: this
// engine's constant, the adapter's `contract` field, and `contract` in
// platform.toml.
//
// IT NAMES EVERY VALUE IT SAW, not the pair that differed. A refusal naming one
// side leaves the reader to guess which of the other two it disagreed with,
// and fable's D6 drill bumps exactly one of the three.
func CheckVersion(adapter, platform string) error {
	if adapter == ContractVersion && platform == ContractVersion {
		return nil
	}
	return Refusal{Reason: fmt.Sprintf(
		"contract version mismatch: engine %q, adapter %q, platform.toml %q",
		ContractVersion, adapter, platform)}
}

// --- writing --------------------------------------------------------------

// Encode writes one message and its newline. JSON-lines is the framing, so the
// newline is part of the message rather than the caller's to remember.
func Encode(w io.Writer, msg Message) error {
	b, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("wire: encoding a %s message: %w", msg.Env().Kind, err)
	}
	if _, err := w.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("wire: writing a %s message: %w", msg.Env().Kind, err)
	}
	return nil
}

// --- the schema, for the tests that check it against the document ---------

// Schema reports the field sets, so tests/contract can compare them with
// docs/contract.md's tables and this package need not export its maps.
// Direction is "request" or "response".
func Schema(direction string, kind Kind) (required, optional []string, ok bool) {
	table := requests
	if direction == "response" {
		table = responses
	}
	s, ok := table[kind]
	if !ok {
		return nil, nil, false
	}
	return append([]string(nil), s.required...), append([]string(nil), s.optional...), true
}

// Kinds reports the kinds a direction carries, sorted, so a sweep over the
// protocol cannot silently miss one.
func Kinds(direction string) []Kind {
	table := requests
	if direction == "response" {
		table = responses
	}
	out := make([]Kind, 0, len(table))
	for k := range table {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// JSONFields reports the json tag names on a message type, which is the second
// reading of the same schema: tests/contract requires it to equal Schema's.
func JSONFields(msg Message) []string {
	t := reflect.TypeOf(msg)
	var out []string
	var walk func(reflect.Type)
	walk = func(t reflect.Type) {
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.Anonymous && f.Type.Kind() == reflect.Struct {
				walk(f.Type)
				continue
			}
			tag := f.Tag.Get("json")
			name := strings.Split(tag, ",")[0]
			if name == "" || name == "-" {
				continue
			}
			out = append(out, name)
		}
	}
	walk(t)
	sort.Strings(out)
	return out
}
