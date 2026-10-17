// Package fake is the fake adapter: it speaks the whole wire protocol against
// nothing.
//
// WHAT IT IS FOR, which decides every choice below. R4.3's permitted twin is
// "the fake adapter with the example bindings passes `rumor validate`", so this
// has to answer every message kind well enough to be validated against -- and
// fable's drills need it to MISBEHAVE in exactly three named ways: a bumped
// contract version (D6), a response missing a required field (D7), and the
// error envelope (D7's could-not-run half). So each flag below has a drill
// behind it, and there are no others.
//
// IT TOUCHES NO NETWORK AND NO CLOCK. Every answer is recorded: the declared
// surface, one response per action, one value per observation, one token. An
// adapter that computed its answers would be a second implementation of the
// system under test, and a run against it would prove something about the fake
// rather than about the contract.
//
// WHY IT ENCODES FROM MAPS AND NOT FROM wire's STRUCTS: a struct with json tags
// cannot omit a required field, and omitting one is the whole of D7. The typed
// value is built first and turned into a map, so the shape still comes from
// package wire and only the deletion is this package's.
package fake

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

// Recorded is one action's recorded answer.
type Recorded struct {
	IDs      map[string]any `json:"ids"`
	Response any            `json:"response"`
}

// Surface is what the adapter declares and what it answers with: the describe
// response, plus one recorded answer per action and one value per observation.
// It is JSON because the drills copy it and change one field, and a format a
// drill can edit by hand is the point.
type Surface struct {
	Actions      []wire.Action      `json:"actions"`
	Observations []wire.Observation `json:"observations"`
	Contexts     []string           `json:"contexts"`
	Components   []wire.Component   `json:"components"`
	Capabilities wire.Capabilities  `json:"capabilities"`

	Execute map[string]Recorded `json:"execute"`
	Observe map[string]any      `json:"observe"`
	Token   string              `json:"token"`
}

// Default is the built-in surface, used when none is named. It is deliberately
// small and deliberately COMPLETE: every message kind has something to answer
// with, so `rumor fake-adapter` is useful with no arguments at all.
func Default() Surface {
	return Surface{
		Actions: []wire.Action{
			{Name: "create_event", Context: "api"},
			{Name: "join_event", Context: "api"},
		},
		Observations: []wire.Observation{
			{Name: "event", Source: wire.SourceAPI},
			{Name: "attendance", Source: wire.SourceDirect},
		},
		Contexts:     []string{"api"},
		Components:   []wire.Component{{Name: "store", Stateful: true}},
		Capabilities: wire.Capabilities{Faults: []string{}, Volume: []string{}, Upgrade: []string{}},
		Execute: map[string]Recorded{
			"create_event": {IDs: map[string]any{"event": "e-1"}, Response: map[string]any{"status": 201}},
			"join_event":   {IDs: map[string]any{}, Response: map[string]any{"status": 200}},
		},
		Observe: map[string]any{"event": "e-1", "attendance": 1},
		Token:   "fake-token",
	}
}

// LoadSurface reads a surface from JSON. Strict, for package wire's reason: a
// drill that mistypes a key would otherwise get the default's answer and the
// drill would pass for the wrong reason.
func LoadSurface(r io.Reader) (Surface, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	var s Surface
	if err := dec.Decode(&s); err != nil {
		return Surface{}, fmt.Errorf("fake: reading the surface: %w", err)
	}
	return s, nil
}

// Config is the adapter's whole behavior.
type Config struct {
	Surface Surface

	// Contract is the version the adapter claims. Empty means the engine's own,
	// which is the only value that agrees. fable's D6 bumps exactly this.
	Contract string

	// Fail answers every request with this error kind. "Speaks the error
	// envelope when asked to", issue 62.
	Fail wire.FaultKind

	// Omit deletes one required field from one kind's response, which is D7:
	// the decoder must refuse by name and validate must report could-not-run
	// rather than a refusal.
	Omit map[wire.Kind]string
}

// Run reads requests until EOF and answers each one.
//
// IT ANSWERS A MALFORMED REQUEST RATHER THAN DYING ON IT: a decode refusal
// becomes an error envelope with kind `field`, which is what the contract says
// an adapter does. An adapter that exited would make the engine read a transport
// failure where the fault was one message.
func Run(cfg Config, in io.Reader, out io.Writer) error {
	contract := cfg.Contract
	if contract == "" {
		contract = wire.ContractVersion
	}
	scan := bufio.NewScanner(in)
	// A LINE CAN BE LONG: a recorded response is arbitrary JSON, and the
	// default 64 KiB would turn a large one into a scanner error that reads as
	// a protocol fault. 4 MiB, named here rather than left as a default
	// nobody chose.
	scan.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if line == "" {
			continue
		}
		answer := cfg.answer(contract, []byte(line))
		if err := write(out, answer); err != nil {
			return err
		}
	}
	if err := scan.Err(); err != nil {
		return fmt.Errorf("fake: reading requests: %w", err)
	}
	return nil
}

// answer builds one response as a map, so a required field can be deleted.
func (cfg Config) answer(contract string, line []byte) map[string]any {
	msg, err := wire.DecodeRequest(line)
	if err != nil {
		return envelope(contract, idOf(line), wire.Fault{
			Kind:   wire.FaultField,
			Field:  fieldOf(err),
			Detail: err.Error(),
		})
	}
	env := msg.Env()
	if cfg.Fail != "" {
		return envelope(contract, env.ID, wire.Fault{
			Kind:   cfg.Fail,
			Detail: fmt.Sprintf("the fake adapter was asked to answer %s with this kind", env.Kind),
		})
	}
	var typed wire.Message
	switch m := msg.(type) {
	case wire.DescribeRequest:
		typed = wire.DescribeResponse{
			Envelope:     wire.Envelope{Kind: wire.KindDescribe, Contract: contract, ID: env.ID},
			Actions:      cfg.Surface.Actions,
			Observations: cfg.Surface.Observations,
			Contexts:     cfg.Surface.Contexts,
			Components:   cfg.Surface.Components,
			Capabilities: cfg.Surface.Capabilities,
		}
	case wire.ExecuteRequest:
		rec, ok := cfg.Surface.Execute[m.Action]
		if !ok {
			return envelope(contract, env.ID, wire.Fault{
				Kind:   wire.FaultAction,
				Field:  "action",
				Detail: fmt.Sprintf("%q is not recorded in this surface; it records %s", m.Action, list(keys(cfg.Surface.Execute))),
			})
		}
		typed = wire.ExecuteResponse{
			Envelope: wire.Envelope{Kind: wire.KindExecute, Contract: contract, ID: env.ID},
			IDs:      rec.IDs,
			Response: rec.Response,
		}
	case wire.ObserveRequest:
		value, ok := cfg.Surface.Observe[m.Query]
		if !ok {
			return envelope(contract, env.ID, wire.Fault{
				Kind:   wire.FaultInternal,
				Field:  "query",
				Detail: fmt.Sprintf("%q is not recorded in this surface; it records %s", m.Query, list(keysAny(cfg.Surface.Observe))),
			})
		}
		typed = wire.ObserveResponse{
			Envelope: wire.Envelope{Kind: wire.KindObserve, Contract: contract, ID: env.ID},
			Value:    value,
		}
	case wire.SessionRequest:
		typed = wire.SessionResponse{
			Envelope: wire.Envelope{Kind: wire.KindSession, Contract: contract, ID: env.ID},
			Token:    cfg.Surface.Token,
		}
	default:
		// UNREACHABLE WHILE wire's REQUEST KINDS ARE THE FOUR ABOVE, and it
		// answers rather than panicking: a kind added to the protocol without
		// a case here is this adapter's gap, and the run should say so and
		// carry on rather than taking the engine down.
		return envelope(contract, env.ID, wire.Fault{
			Kind:   wire.FaultInternal,
			Detail: fmt.Sprintf("the fake adapter has no answer for a %s request", env.Kind),
		})
	}
	obj := object(typed)
	if field, ok := cfg.Omit[env.Kind]; ok {
		delete(obj, field)
	}
	return obj
}

// object turns a typed message into the map that gets written. The round trip
// through package wire's own tags is deliberate: the shape stays wire's and
// only the deletion above is this package's.
func object(msg wire.Message) map[string]any {
	b, err := json.Marshal(msg)
	if err != nil {
		// A MESSAGE wire ITSELF BUILT cannot fail to marshal unless a recorded
		// value is unmarshalable, which is the surface's fault and is reported
		// as such rather than silently dropped.
		return map[string]any{
			"kind":     string(msg.Env().Kind),
			"contract": msg.Env().Contract,
			"id":       msg.Env().ID,
			"error": map[string]any{
				"kind":   string(wire.FaultInternal),
				"detail": fmt.Sprintf("a recorded value cannot be encoded: %v", err),
			},
		}
	}
	var obj map[string]any
	if err := json.Unmarshal(b, &obj); err != nil {
		return map[string]any{}
	}
	return obj
}

func envelope(contract, id string, f wire.Fault) map[string]any {
	return object(wire.ErrorResponse{
		Envelope: wire.Envelope{Kind: wire.KindError, Contract: contract, ID: id},
		Error:    f,
	})
}

func write(out io.Writer, obj map[string]any) error {
	b, err := json.Marshal(obj)
	if err != nil {
		return fmt.Errorf("fake: encoding a response: %w", err)
	}
	if _, err := out.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("fake: writing a response: %w", err)
	}
	return nil
}

// idOf reads the id out of a line the decoder refused, so even a refusal can be
// correlated where the id itself was readable. An unreadable id answers with an
// empty one rather than inventing a value the engine never sent.
func idOf(line []byte) string {
	var raw struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(line, &raw); err != nil {
		return ""
	}
	return raw.ID
}

// fieldOf pulls the field out of a wire.Refusal, where there is one. The
// envelope's `field` is what D7 asserts on, so it has to carry the decoder's
// own answer rather than a restatement.
func fieldOf(err error) string {
	var ref wire.Refusal
	if ok := asRefusal(err, &ref); ok {
		return ref.Field
	}
	return ""
}

func asRefusal(err error, out *wire.Refusal) bool {
	ref, ok := err.(wire.Refusal)
	if !ok {
		return false
	}
	*out = ref
	return true
}

func keys(m map[string]Recorded) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func keysAny(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func list(items []string) string {
	if len(items) == 0 {
		return "nothing"
	}
	return strings.Join(items, ", ")
}
