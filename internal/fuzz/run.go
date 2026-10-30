// Running the corpus against a live system, and the one oracle clause this
// head carries: no 5xx.
//
// THE SETUP MUST SUCCEED AND IS NOT A FINDING. Every class but `missing` sends
// a value, and a handle's honest value is an id the SYSTEM minted, so the
// stage first runs the derived setup (Order, from `creates`) honestly. A setup
// step the system refuses means the stage could not obtain an id -- a
// COULD-NOT-RUN, because what the hostile case would have answered is unknown.
// Reporting it as a finding would blame the corpus for a world it never got.
//
// WHAT THE ORACLE CAN SEE, measured before this was written: cmd/toy-adapter
// answers `Response: {"status": code, "body": out}` on BOTH the refusal and
// the success paths (main.go:499-516), so the status AND the body's `error`
// text cross the wire. "No 5xx" and D4 are both readable through the adapter
// without a new channel.
//
// D4's WORDING IS THE AMENDED ONE: "a 4xx FROM THE SYSTEM's OWN RULES names a
// field or a rule", over the 21 such refusals and not over all 26. The
// unqualified form it replaced was satisfiable on the decoder's accident --
// encoding/json names the struct field in its own error text -- so five of the
// thirteen distinct messages a reader sees passed a clause about rules cmd/toy
// never wrote (rook, rumor/1486, amending the gate after this was measured).
package fuzz

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/schedule"
	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

// Result is one case and what the system answered.
type Result struct {
	Case   Case
	Status int
	Error  string // the body's `error` text, where the system gave one
	Fault  string // an adapter fault kind, where the adapter could not run it
}

// Report is the stage's whole account: the setup it ran, every case's answer,
// and the findings.
//
// IT COUNTS WHAT IT EXAMINED, not only what it refused: zero findings over
// zero cases reads exactly like zero findings over forty-five, which is the
// shape internal/examples' own Result exists to prevent.
type Report struct {
	Setup   []string
	Results []Result

	// RefusalPath is `[refusal].message` as the project declared it -- where
	// in the system's answer the text of a refusal lives. It is carried on the
	// report rather than looked up by the clauses so that the clauses stay pure
	// functions of what a run observed, which is what lets a fixture drive
	// every one of them without an adapter.
	RefusalPath string
}

// Origin is which layer answered a case, because three of them can and only
// one of them is the system under test.
//
// MEASURED OVER examples/toy, 45 cases: EIGHTEEN never left the adapter, FIVE
// reached the toy and were refused by `encoding/json` inside its own
// decodeBody before a handler ran, TWENTY-ONE reached a rule the toy wrote,
// and ONE was accepted. A clause reading "the refusal names a field" passes on
// all twenty-six, because Go's unmarshal error happens to carry the struct
// field -- "json: cannot unmarshal number into Go struct field .title of type
// string" names `title` without the toy having a rule about it.
//
// SO "NAMES A FIELD OR A RULE" IS NOT ENOUGH ON ITS OWN. The question D4 asks
// is whether the SYSTEM discriminates, and a decoder's type error is the same
// non-evidence as the adapter's argument check one layer out.
type Origin int

const (
	// OriginAdapter is a fault: the adapter refused before the system saw a
	// request.
	OriginAdapter Origin = iota
	// OriginDecoder is the system's process refusing the SHAPE of the request
	// before any rule of its own ran.
	OriginDecoder
	// OriginSystem is a rule the system's author wrote.
	OriginSystem
	// OriginAccepted is a request the system took.
	OriginAccepted
	// OriginUnread is an answer this stage could not classify, which is kept
	// distinct from every other value so it cannot be counted as evidence.
	OriginUnread
)

// namesARule is whether a refusal states a rule rather than a field. The toy's
// "capacity must be at least 1" and "only the event's admin invites" are rules;
// a bare "no such event" names an object. The test is deliberately WEAK -- any
// of a few rule words -- because a strong one would be a grammar for English
// error messages, and the clause it serves only needs to tell "this says
// something about my request" from "this says nothing".
func namesARule(msg string) bool {
	for _, w := range []string{" must ", " cannot ", " only ", " is not ", " already ", " at capacity"} {
		if strings.Contains(msg, w) {
			return true
		}
	}
	return false
}

// decoderRefusal is how a Go service's own JSON decode failure reads. It is
// matched on the prefix `encoding/json` itself produces, and the cost is
// stated: a system that WRAPPED that error, or one not written in Go, would be
// classified as OriginSystem and the clause would over-credit it. The test for
// that is the figures below moving, which the arm pins.
const decoderRefusal = "json: cannot unmarshal"

// Origin classifies one result.
func (r Result) Origin() Origin {
	switch {
	case r.Fault != "":
		return OriginAdapter
	case r.Status == 0:
		return OriginUnread
	case r.Status >= 400 && r.Status < 500:
		if strings.Contains(r.Error, decoderRefusal) {
			return OriginDecoder
		}
		return OriginSystem
	case r.Status >= 200 && r.Status < 300:
		return OriginAccepted
	}
	return OriginUnread
}

// SystemRefusals are the results a rule of the system's own refused.
func (r Report) SystemRefusals() []Result {
	var out []Result
	for _, res := range r.Results {
		if res.Origin() == OriginSystem {
			out = append(out, res)
		}
	}
	return out
}

// Reached is how many cases the SYSTEM answered, as against how many were
// sent.
//
// THE DENOMINATOR OF AN ABSENCE CLAIM (rook, coop r8/20, on my own figures at
// r8/19). "No 5xx over 45 cases" is not a claim about 45: 18 of them were
// refused by the ADAPTER's own argument check before the toy saw a request, so
// the clause speaks for the 27 that reached. An absence is only as strong as
// the number that reached, which is D10's shape appearing inside D2 -- "did
// not look" and "looked and did not find" are different claims, and a count is
// the only thing that tells them apart.
//
// SO A REACHED COUNT THAT FALLS WHILE THE CORPUS GROWS IS ITSELF A FINDING:
// more cases and fewer arriving means the new ones are testing the adapter.
// DERIVED IS NOT EFFECTIVE, and those are two readings of one corpus.
func (r Report) Reached() int {
	n := 0
	for _, res := range r.Results {
		if res.Fault == "" && res.Status != 0 {
			n++
		}
	}
	return n
}

// Run executes the setup and then the corpus.
func Run(ctx context.Context, b bindings.Bindings, sess *wire.Session, actor string) (Report, error) {
	var rep Report
	// CARRIED BEFORE ANYTHING CAN FAIL, so a report returned alongside an error
	// still says what the run was reading -- the same reason Walk fills
	// Record.Stages at the top.
	rep.RefusalPath = b.RefusalMessage
	cases, err := Corpus(b)
	if err != nil {
		return rep, err
	}
	order, err := Order(b, Needs(b, cases))
	if err != nil {
		return rep, err
	}

	sessions := schedule.NewSessions(sess)
	byName := map[string]bindings.Action{}
	for _, a := range b.Actions {
		byName[a.Name] = a
	}

	// ids is the handle kind to the id the system minted for it.
	ids := map[string]string{}
	for _, a := range order {
		args, err := honestArgs(a, actor, ids)
		if err != nil {
			return rep, fmt.Errorf("setting up with %s: %w", a.Name, err)
		}
		token, err := sessions.Token(actor, a.Context)
		if err != nil {
			return rep, fmt.Errorf("setting up with %s: %w", a.Name, err)
		}
		resp, err := sess.Execute(wire.ExecuteRequest{Action: a.Name, Token: token, Args: args})
		if err != nil {
			return rep, fmt.Errorf("setting up with %s: %w", a.Name, err)
		}
		// THE SETUP WANTS THE STATUS AND NOT THE TEXT, so it passes no path:
		// a setup step's refusal is a could-not-run named by its own message
		// below, and reading a refusal's text here would make the declaration
		// load-bearing for a step that does not need it.
		status, _, _ := answer(resp, "")
		if status < 200 || status > 299 {
			return rep, fmt.Errorf("the setup step %s answered %d, so no id for %v was obtained: "+
				"a hostile case sent without one would be refused for the wrong reason",
				a.Name, status, a.Creates)
		}
		for kind, v := range resp.IDs {
			ids[kind] = fmt.Sprint(v)
		}
		for _, kind := range a.Creates {
			if _, ok := ids[kind]; !ok {
				return rep, fmt.Errorf("%s declares it creates %q and the system's answer carried no "+
					"such id: the stage would send an id it never obtained", a.Name, kind)
			}
		}
		rep.Setup = append(rep.Setup, a.Name)
	}

	for _, c := range cases {
		a := byName[c.Action]
		args, err := caseArgs(a, c, actor, ids)
		if err != nil {
			return rep, fmt.Errorf("%s: %w", c, err)
		}
		token, err := sessions.Token(actor, a.Context)
		if err != nil {
			return rep, fmt.Errorf("%s: %w", c, err)
		}
		resp, rerr := sess.Execute(wire.ExecuteRequest{Action: a.Name, Token: token, Args: args})
		r := Result{Case: c}
		if rerr != nil {
			// A FAULT IS THE ADAPTER's ANSWER AND NOT A CRASH OF THE STAGE.
			// The adapter saying "this action is not in my surface" is a
			// result about the request, so it is recorded and the corpus goes
			// on; a stage that stopped here would report the cases it never
			// sent as having passed.
			r.Fault = rerr.Error()
			rep.Results = append(rep.Results, r)
			continue
		}
		r.Status, r.Error, _ = answer(resp, b.RefusalMessage)
		rep.Results = append(rep.Results, r)
	}

	// NOTHING REACHED IS A COULD-NOT-RUN, and it is checked BEFORE the oracle
	// clauses so that a run with an empty corpus cannot return their silence as
	// a clean verdict. The report travels with the error: the faults are the
	// evidence for why it could not run.
	if why := rep.Vacuous(); why != "" {
		return rep, errors.New(why)
	}

	return rep, nil
}

// answer reads the status and the error text out of an execute response.
//
// THE SHAPE IS THE ADAPTER's AND IT IS READ DEFENSIVELY: an adapter that
// answered something else entirely would otherwise give a zero status, which
// would read as a pass. A response this cannot read reports status 0 and says
// so through `ok`, and the caller treats that as a fault rather than a 2xx.
func answer(resp wire.ExecuteResponse, path string) (status int, errText string, ok bool) {
	m, isMap := resp.Response.(map[string]any)
	if !isMap {
		return 0, fmt.Sprintf("the adapter answered %v, which is not a table", resp.Response), false
	}
	switch v := m["status"].(type) {
	case float64:
		status = int(v)
	case int:
		status = v
	case int64:
		status = int(v)
	default:
		return 0, fmt.Sprintf("the adapter's answer carries no numeric status: %v", m), false
	}
	return status, textAt(m, path), true
}

// textAt resolves a declared dotted path against the system's answer, which is
// `[refusal].message` -- `body.error` for the toy.
//
// AN ABSENT PATH IS NOT AN ERROR HERE, and that is deliberate rather than
// lax. A 2xx carries no refusal text by design, and a system may refuse in
// silence; both land here as "". What separates those from a path that names
// nothing at all is a question about the WHOLE RUN -- whether any refusal
// anywhere carried text -- and Report.Vacuous asks it, because one answer
// cannot tell them apart and a corpus can.
//
// SEGMENTS ALL THE WAY DOWN, AND THE LAST ONE IS A VALUE. A non-table in the
// middle of the path ends the walk with "", for the reason above: a shape
// mismatch is indistinguishable here from a missing message, and the run-level
// clause is what reports it.
func textAt(m map[string]any, path string) string {
	if path == "" {
		return ""
	}
	seg := strings.Split(path, ".")
	var cur any = m
	for _, name := range seg {
		tbl, isTable := cur.(map[string]any)
		if !isTable {
			return ""
		}
		next, has := tbl[name]
		if !has {
			return ""
		}
		cur = next
	}
	if cur == nil {
		return ""
	}
	if s, isStr := cur.(string); isStr {
		return s
	}
	return fmt.Sprint(cur)
}

// honestArgs is every declared argument with a plausible value: a literal's
// own value, a draw's FIRST domain element, the actor's name, and the id the
// setup obtained for a handle's kind.
//
// THE DRAW TAKES ITS FIRST ELEMENT AND NOT A RANDOM ONE. The setup is not a
// walk and has no seed; a random choice would make two runs of one corpus
// differ in the world the cases were sent against, and a replay could not
// reproduce a finding.
func honestArgs(a bindings.Action, actor string, ids map[string]string) (map[string]any, error) {
	out := map[string]any{}
	for _, arg := range a.Args {
		v, err := honestValue(a, arg, actor, ids)
		if err != nil {
			return nil, err
		}
		out[arg.Name] = v
	}
	return out, nil
}

func honestValue(a bindings.Action, arg bindings.Arg, actor string, ids map[string]string) (any, error) {
	switch arg.Source {
	case "literal":
		return arg.Value, nil
	case "actor":
		return actor, nil
	case "draw":
		if len(arg.From) == 0 {
			return nil, fmt.Errorf("%s's argument %q draws from an empty domain", a.Name, arg.Name)
		}
		return arg.From[0], nil
	case "handle":
		id, ok := ids[arg.Of]
		if !ok {
			return nil, fmt.Errorf("%s's argument %q needs a %q and the setup obtained none",
				a.Name, arg.Name, arg.Of)
		}
		return id, nil
	}
	return nil, fmt.Errorf("%s's argument %q has source %q, which this stage cannot resolve",
		a.Name, arg.Name, arg.Source)
}

// caseArgs is the honest request with exactly one field perturbed.
//
// THE OTHER ARGUMENTS ARE RESOLVED FROM THE DECLARATION AND NOT COPIED FROM
// THE CASE. Corpus fills them with placeholders for the report's sake, and
// sending those would mean every case was refused for the placeholder rather
// than for its own class -- the refusal would name a field, pass a careless
// oracle, and say nothing about the value under test.
func caseArgs(a bindings.Action, c Case, actor string, ids map[string]string) (map[string]any, error) {
	out := map[string]any{}
	for _, arg := range a.Args {
		if arg.Name == c.Field {
			if c.Class == ClassMissing {
				continue
			}
			out[arg.Name] = c.Args[arg.Name]
			continue
		}
		v, err := honestValue(a, arg, actor, ids)
		if err != nil {
			return nil, err
		}
		out[arg.Name] = v
	}
	return out, nil
}

// Findings is every defect the oracle clauses see in this report's results.
//
// IT IS COMPUTED AND NOT STORED, and that is the whole point. A stored slice
// needs a writer to append to it at the right moment, and a writer that is
// deleted leaves every consumer reading an empty list -- which reads as a clean
// verdict. Measured: with the clauses inline in Run, both D4 branches could be
// turned off (`if false && ...`) and internal/fuzz AND tests/run stayed green,
// because the only arm over them asserted the TOY's spread and so never reached
// the branches at all. Both findings D4 can emit were refusals nobody could
// show failing (rook, rumor/1488). There is now no call site to omit: a
// consumer that reads findings runs the clauses, and an arm over a fixture
// report drives the same path a run does -- so a clause dropped from this
// composition dies here too.
func (r Report) Findings() []string {
	var out []string
	out = append(out, r.no5xx()...)
	out = append(out, r.monoculture()...)
	out = append(out, r.namesSomething()...)
	return out
}

// no5xx is D2. The finding names the action, the class and the field, because
// "a 5xx happened" cannot be acted on and "delete_event refused a missing event
// with 500" can.
func (r Report) no5xx() []string {
	var out []string
	for _, res := range r.Results {
		if res.Status >= 500 {
			out = append(out, fmt.Sprintf(
				"%s answered %d: a hostile value for a declared field must be refused, not fail the server (%s)",
				res.Case, res.Status, res.Error))
		}
	}
	return out
}

// monoculture is D4's first half: ONE SENTENCE OVER EVERYTHING FAILS.
//
// "Every refusal names a field or a rule" is satisfied by a system that answers
// every hostile value with one sentence, and such a system discriminates
// nothing -- the clause would pass over an oracle with no information in it. So
// the refusals the SYSTEM's own rules produced are counted by distinct message,
// and one message over many refusals is the finding.
//
// THE SYSTEM's OWN, which is what makes the count mean anything: a decoder's
// type error and an adapter's argument check are not the system
// discriminating, and including them would let a system with one rule inherit
// the decoder's variety.
func (r Report) monoculture() []string {
	sys := r.SystemRefusals()
	if len(sys) < 2 {
		return nil
	}
	seen := map[string]bool{}
	for _, res := range sys {
		seen[res.Error] = true
	}
	if len(seen) != 1 {
		return nil
	}
	var out []string
	for msg := range seen {
		out = append(out, fmt.Sprintf(
			"all %d refusal(s) the system's own rules gave answered %q: one sentence over "+
				"every hostile value discriminates nothing, so a refusal naming a field "+
				"proves nothing about this system", len(sys), msg))
	}
	return out
}

// namesSomething is D4's other half: A REFUSAL THAT NAMES NOTHING OF THE
// REQUEST. The perturbed field, or a rule the system states. Checked only over
// the system's own refusals, for the reason monoculture gives.
//
// THE `Field != ""` GUARD IS REDUNDANT TWICE OVER AND IS KEPT ANYWAY. No
// mutant of it can die, and that was measured rather than assumed: deleting it
// leaves every arm green. Both reasons are accidents rather than design.
// First, `base` sets Field to the argument's own name for every case the
// corpus generates (fuzz.go:141), so a fieldless case is unreachable from
// Corpus. Second, even if one arrived, strings.Contains(msg, "") is TRUE for
// any message, so the clause would already fall through without judging it.
// The guard is therefore documentation of whose business this case is not --
// and it becomes load-bearing the moment either accident ends: a class that
// perturbs the body rather than one field would reach it (D3's shapes are
// exactly that), and a field test stricter than Contains would stop absorbing
// the empty string. Do not delete it as dead on the strength of a surviving
// mutant; the survival is the point above, not a gap in the arms.
func (r Report) namesSomething() []string {
	var out []string
	for _, res := range r.SystemRefusals() {
		if res.Case.Field != "" && !strings.Contains(res.Error, res.Case.Field) && !namesARule(res.Error) {
			out = append(out, fmt.Sprintf(
				"%s was refused with %q, which names neither the field nor a rule: a reader cannot "+
					"tell which of the request's parts the system objected to", res.Case, res.Error))
		}
	}
	return out
}

// Vacuous is why this run cannot answer D2, or "" when it can.
//
// D2's clause is "no 5xx", and it is read off the results. A run in which NO
// case reached the system produces an empty Findings for the same reason a
// clean one does, so the two are indistinguishable to a reader: a server that
// died after the setup, or an adapter whose surface answers nothing, would
// report the corpus as having found no 5xx when the system never saw it.
// This is a COULD-NOT-RUN and not a finding, for the same reason a refused
// setup step is: the defect is in the run, and blaming the system under test
// for a request it never received is the false finding this stage exists to
// avoid.
//
// WHY THE LINE IS AT ZERO AND NOT A FRACTION: a nonzero line needs a NUMBER,
// and nothing in the declarations supplies one. 18 of the honest run's 45 cases
// fault at the adapter, because cmd/toy-adapter's surface genuinely does not
// carry every action the bindings declare -- and that rate is a property of one
// adapter's surface, not of the question, so a constant chosen from it would
// refuse correct runs against a thinner surface and clear vacuous ones against
// a fuller. Zero is the only line this stage can derive rather than guess.
func (r Report) Vacuous() string {
	if len(r.Results) == 0 {
		return "the corpus sent no case at all, so \"no 5xx\" is a statement about nothing"
	}
	// NOTHING DECLARES WHERE A REFUSAL's TEXT LIVES, which is knowable before a
	// single case is sent and is therefore reported before anything about what
	// came back. fable ruled the table into 108 (coop rumor/1498) after rook
	// and I measured that there was nowhere to declare it: `[[action]].response`
	// is an equality contract, so an error key there would make every accepted
	// action a refusal. With no `[refusal]` the D3 and D4 clauses have no
	// subject, and this says so BY NAME rather than reporting findings grounded
	// in a key nobody declared.
	if r.RefusalPath == "" {
		return "no refusal text declared: `[refusal].message` -- the D3 and D4 clauses read a " +
			"refusal's text at the path a project declares there, and with none declared they " +
			"would be grounded in a key nobody named"
	}

	if r.Reached() == 0 {
		return fmt.Sprintf("none of the %d case(s) reached the system -- every one faulted at the "+
			"adapter or went unanswered -- so \"no 5xx\" is a statement about nothing", len(r.Results))
	}

	// AND D4 CANNOT BE ANSWERED WITHOUT HAVING READ ONE REFUSAL's TEXT.
	//
	// The stage reads a refusal's message at `body.error`, and NOTHING
	// DECLARES THAT KEY -- `[[action]].response` is an equality contract for
	// what acceptance is, not a schema, and this protocol deliberately knows
	// nothing about HTTP. So an adapter spelling it `message` or `detail`
	// leaves every message empty, and an empty message is ambiguous: either
	// the stage is reading the wrong key, or the system really did refuse
	// without saying anything. The second IS a finding and the first is not.
	//
	// MEASURED, not reasoned: with cmd/toy's key changed to `message`, the
	// clauses emit 27 findings against a system that names every field --
	// one monoculture over "" and 26 refusals "naming nothing". Every one is
	// false, and they are the loudest output this stage can produce.
	//
	// THE STAGE CALIBRATES ON ITS OWN DATA. Text on ANY system refusal proves
	// the channel is readable, and a silent refusal beside it is then the
	// system's own choice and judged as one. Text on NONE of them leaves the
	// reading unproven, and the honest answer is a could-not-run rather than a
	// finding for every case. This is the positive control the oracle needs and
	// it costs no declaration: the run supplies it or it does not.
	if sys := r.SystemRefusals(); len(sys) > 0 {
		spoke := false
		for _, res := range sys {
			if res.Error != "" {
				spoke = true
				break
			}
		}
		if !spoke {
			return fmt.Sprintf("not one of the %d refusal(s) the system's own rules gave carried any "+
				"message at %q, which is this project's `[refusal].message`: so D4 cannot be told "+
				"apart from a system that puts its text somewhere else, and the likeliest cause is "+
				"that declaration rather than a silent system. The oracle's subject is missing, not "+
				"uninformative", len(sys), r.RefusalPath)
		}
	}
	return ""
}

// Summary is the one line a reader needs: what reached, out of what was sent,
// and the answers. The two numbers travel TOGETHER because either alone
// misleads -- 45 cases says nothing about coverage, and 27 reached says
// nothing about how much was attempted.
func (r Report) Summary() string {
	return fmt.Sprintf("%d of %d case(s) reached the system; %s",
		r.Reached(), len(r.Results), r.Statuses())
}

// Statuses is the answers the report saw, counted, for the one-line summary.
func (r Report) Statuses() string {
	n := map[int]int{}
	for _, res := range r.Results {
		n[res.Status]++
	}
	codes := make([]int, 0, len(n))
	for c := range n {
		codes = append(codes, c)
	}
	sort.Ints(codes)
	parts := make([]string, 0, len(codes))
	for _, c := range codes {
		parts = append(parts, fmt.Sprintf("%d x%d", c, n[c]))
	}
	return strings.Join(parts, ", ")
}
