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
// text cross the wire. "No 5xx" and "a 4xx names a field or rule" are both
// readable through the adapter without a new channel.
package fuzz

import (
	"context"
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
	Setup    []string
	Results  []Result
	Findings []string
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
		status, _, _ := answer(resp)
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
		r.Status, r.Error, _ = answer(resp)
		rep.Results = append(rep.Results, r)
	}

	// D2: NO 5xx. The finding names the action, the class and the field,
	// because "a 5xx happened" cannot be acted on and "delete_event refused a
	// missing event with 500" can.
	for _, r := range rep.Results {
		if r.Status >= 500 {
			rep.Findings = append(rep.Findings, fmt.Sprintf(
				"%s answered %d: a hostile value for a declared field must be refused, not fail the server (%s)",
				r.Case, r.Status, r.Error))
		}
	}
	return rep, nil
}

// answer reads the status and the error text out of an execute response.
//
// THE SHAPE IS THE ADAPTER's AND IT IS READ DEFENSIVELY: an adapter that
// answered something else entirely would otherwise give a zero status, which
// would read as a pass. A response this cannot read reports status 0 and says
// so through `ok`, and the caller treats that as a fault rather than a 2xx.
func answer(resp wire.ExecuteResponse) (status int, errText string, ok bool) {
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
	if body, isBody := m["body"].(map[string]any); isBody {
		if e, has := body["error"]; has {
			errText = fmt.Sprint(e)
		}
	}
	return status, errText, true
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
