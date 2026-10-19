// Command toy-adapter speaks R4's wire contract on stdio and drives the toy
// over HTTP (R5.2, issue 73). It is the `[adapter].command` examples/toy names.
//
// WHO OWNS THE PROCESS, and why this has two modes. The rollback provider
// restarts the toy and swaps its state file, so the provider has to own the
// process; the adapter only needs an address to talk to, and takes it from
// TOY_ADDR -- an env var, which is exactly what platform.toml's `[adapter].env`
// and wire.Spawn's named environment exist to pass. When TOY_ADDR is absent the
// adapter starts a toy of its own and uses that, because `rumor validate`
// spawns the ADAPTER ALONE: no engine, no provider, and four of the eighteen
// arms need a describe to answer. A mode that only worked under the engine
// would make the toy unvalidatable, which issue 73 requires it to be.
//
// AN OBSERVATION'S SUBJECT RIDES IN THE QUERY, and that is a convention this
// file invents because the contract leaves room for it rather than because it
// is obviously right. R4's ObserveRequest carries Query, Source and Token and
// NO args, while an action carries Args -- so `event_count` of a particular
// event has nowhere to put the event but the query string. The shape here is
// `name/subject`, with the subject being the system's own id (the engine
// resolves a handle before the message is written, as it does for Args). Raised
// for quill and fable in coop r5.toy rather than settled here: if the engine
// cannot carry a subject, a parameterized observation is not expressible and
// that is the contract's to say, not the toy's to work around.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

// The actions and observations this adapter can serve. They are the surface
// `describe` reports, and examples/toy's bindings declare the same set: the two
// convict each other, which is validate's arms 1 and 2.
var (
	actions = []wire.Action{
		{Name: "create_user", Context: "none"},
		{Name: "create_event", Context: "api"},
		{Name: "join_event", Context: "api"},
		{Name: "leave_event", Context: "api"},
		{Name: "delete_event", Context: "api"},
		{Name: "join_event_ui", Context: "ui"},
	}
	observations = []wire.Observation{
		{Name: "list_events", Source: wire.SourceAPI},
		{Name: "event_count", Source: wire.SourceAPI},
		{Name: "store_event_count", Source: wire.SourceDirect},
	}
	contexts = []string{"api", "ui"}
	// THE STORE IS STATEFUL AND THE API IS NOT, which is the distinction
	// validate's rollback-coverage arm compares against platform.toml's
	// [[rollback]] entries: the file is the world, so `store` is the component
	// a provider has to cover and `api` is the process in front of it.
	components = []wire.Component{
		{Name: "api", Stateful: false},
		{Name: "store", Stateful: true},
	}
	// EMPTY IS AN ANSWER (docs/contract.md): the toy offers no fault injection,
	// no volume target and no upgrade in R5, so these are empty rather than
	// absent, and examples/toy's `[uses]` asks for none of them. R19 and R21 are
	// where they arrive.
	capabilities = wire.Capabilities{
		Faults:  []string{},
		Volume:  []string{},
		Upgrade: []string{},
	}
)

type adapter struct {
	base   string // http://host:port of the toy
	client *http.Client
	child  *exec.Cmd
}

func main() {
	toyBin := flag.String("toy", "toy", "the toy binary to start when TOY_ADDR is not set")
	state := flag.String("state", "", "the state file for a toy this adapter starts; a temp file when empty")
	flag.Parse()

	a, err := open(*toyBin, *state)
	if err != nil {
		// A FAULT ENVELOPE, NOT A CRASH: the engine and validate both read one
		// line per request, and an adapter that dies before the first request
		// reads as "no answer" rather than as the provisioning finding it is.
		// So say it on stderr, where the spawner quotes it, and exit non-zero.
		fmt.Fprintf(os.Stderr, "toy-adapter: %v\n", err)
		os.Exit(1)
	}
	defer a.close()

	if err := a.serve(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "toy-adapter: %v\n", err)
		os.Exit(1)
	}
}

func open(toyBin, state string) (*adapter, error) {
	a := &adapter{client: &http.Client{Timeout: 10 * time.Second}}
	if addr := os.Getenv("TOY_ADDR"); addr != "" {
		a.base = "http://" + strings.TrimPrefix(strings.TrimPrefix(addr, "http://"), "https://")
		return a, nil
	}
	if state == "" {
		dir, err := os.MkdirTemp("", "toy-adapter-*")
		if err != nil {
			return nil, fmt.Errorf("making a state directory: %w", err)
		}
		state = filepath.Join(dir, "state.json")
	}
	cmd := exec.Command(toyBin, "--addr", "127.0.0.1:0", "--state", state)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("piping %s: %w", toyBin, err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting %s (set TOY_ADDR to use a running one): %w", toyBin, err)
	}
	a.child = cmd
	// THE ADDRESS IS READ, NOT GUESSED. The toy binds :0 and prints what it
	// got, so two runs on one box cannot collide over a port.
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("reading the address %s printed: %w", toyBin, err)
	}
	addr := addressOf(line)
	if addr == "" {
		return nil, fmt.Errorf("%s printed %q, which names no address", toyBin, strings.TrimSpace(line))
	}
	a.base = "http://" + addr
	return a, nil
}

// addressOf reads the address out of the toy's first line, which is
// "toy: listening on 127.0.0.1:PORT state=PATH".
func addressOf(line string) string {
	for _, field := range strings.Fields(line) {
		if strings.Contains(field, ":") && !strings.HasSuffix(field, ":") && !strings.Contains(field, "=") {
			return field
		}
	}
	return ""
}

func (a *adapter) close() {
	if a.child != nil && a.child.Process != nil {
		_ = a.child.Process.Kill()
		_ = a.child.Wait()
	}
}

// serve reads one request per line and writes one response per request, which
// is the whole transport. The loop is internal/fake's, deliberately: two
// adapters answering the same contract should not disagree about framing.
func (a *adapter) serve(in io.Reader, out io.Writer) error {
	scan := bufio.NewScanner(in)
	scan.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	enc := json.NewEncoder(out)
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if line == "" {
			continue
		}
		if err := enc.Encode(a.answer([]byte(line))); err != nil {
			return fmt.Errorf("writing a response: %w", err)
		}
	}
	if err := scan.Err(); err != nil {
		return fmt.Errorf("reading requests: %w", err)
	}
	return nil
}

func fault(id string, kind wire.FaultKind, field, detail string) any {
	return map[string]any{
		"kind":     string(wire.KindError),
		"contract": wire.ContractVersion,
		"id":       id,
		"fault": map[string]any{
			"kind":   string(kind),
			"field":  field,
			"detail": detail,
		},
	}
}

func (a *adapter) answer(line []byte) any {
	msg, err := wire.DecodeRequest(line)
	if err != nil {
		return fault(idOf(line), wire.FaultField, "", err.Error())
	}
	env := msg.Env()
	switch m := msg.(type) {
	case wire.DescribeRequest:
		return wire.DescribeResponse{
			Envelope:     wire.Envelope{Kind: wire.KindDescribe, Contract: wire.ContractVersion, ID: env.ID},
			Actions:      actions,
			Observations: observations,
			Contexts:     contexts,
			Components:   components,
			Capabilities: capabilities,
		}
	case wire.SessionRequest:
		return a.session(env.ID, m)
	case wire.ExecuteRequest:
		return a.execute(env.ID, m)
	case wire.ObserveRequest:
		return a.observe(env.ID, m)
	}
	return fault(env.ID, wire.FaultInternal, "kind", fmt.Sprintf("%q is a kind this adapter does not serve", env.Kind))
}

// idOf recovers the id from a line the decoder refused, so a fault can be
// correlated with the request that caused it. internal/fake does the same.
func idOf(line []byte) string {
	var probe struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(line, &probe); err != nil {
		return ""
	}
	return probe.ID
}

func (a *adapter) session(id string, m wire.SessionRequest) any {
	var body struct {
		Token string `json:"token"`
	}
	code, err := a.call(http.MethodPost, "/sessions", "", map[string]any{
		"actor": m.Actor, "context": m.Context,
	}, &body)
	if err != nil {
		return fault(id, wire.FaultSession, "", err.Error())
	}
	if code != http.StatusCreated {
		return fault(id, wire.FaultSession, "actor",
			fmt.Sprintf("the toy refused a session for %q in context %q with status %d", m.Actor, m.Context, code))
	}
	return wire.SessionResponse{
		Envelope: wire.Envelope{Kind: wire.KindSession, Contract: wire.ContractVersion, ID: id},
		Token:    body.Token,
	}
}

func (a *adapter) execute(id string, m wire.ExecuteRequest) any {
	type plan struct {
		method string
		path   string
		body   map[string]any
		// creates names the entity this action makes, which is the key the
		// engine reads an id back under. Empty when the action creates nothing.
		creates string
	}
	arg := func(k string) any { return m.Args[k] }
	subject, _ := m.Args["event"].(string)

	var p plan
	switch m.Action {
	case "create_user":
		p = plan{http.MethodPost, "/users", map[string]any{"name": arg("name")}, "user"}
	case "create_event":
		p = plan{http.MethodPost, "/events", map[string]any{
			"title": arg("title"), "capacity": arg("capacity"), "visible_to": arg("visible_to"),
		}, "event"}
	case "join_event":
		p = plan{http.MethodPost, "/events/" + subject + "/join", nil, ""}
	case "leave_event":
		p = plan{http.MethodDelete, "/events/" + subject + "/join", nil, ""}
	case "delete_event":
		p = plan{http.MethodDelete, "/events/" + subject, nil, ""}
	case "join_event_ui":
		// THE UI ACTION GOES THROUGH THE PAGE'S OWN ROUTE, not the API's, so
		// what the flow declares and what the browser would do in R9 are the
		// same path.
		p = plan{http.MethodPost, "/ui/events/" + subject + "/join", nil, ""}
	default:
		return fault(id, wire.FaultAction, "action",
			fmt.Sprintf("%q is not an action this adapter serves; it serves %s", m.Action, names()))
	}
	if strings.Contains(p.path, "//") {
		return fault(id, wire.FaultField, "args.event",
			fmt.Sprintf("%q needs an event id in args and got none", m.Action))
	}

	var out map[string]any
	code, err := a.call(p.method, p.path, m.Token, p.body, &out)
	if err != nil {
		return fault(id, wire.FaultInternal, "", err.Error())
	}
	if code >= 400 {
		// A REFUSAL BY THE SYSTEM IS NOT A PROTOCOL FAULT. The toy saying "at
		// capacity" is the system under test behaving, and the engine has to
		// see it as a result rather than as an adapter that could not run --
		// which is the same boundary docs/contract.md draws for a decode
		// failure being could-not-run and a project being wrong being a
		// refusal. So the status and the body ride back in the response.
		return wire.ExecuteResponse{
			Envelope: wire.Envelope{Kind: wire.KindExecute, Contract: wire.ContractVersion, ID: id},
			IDs:      map[string]any{},
			Response: map[string]any{"status": code, "body": out},
		}
	}
	ids := map[string]any{}
	if p.creates != "" {
		if made, ok := out["id"]; ok {
			ids[p.creates] = made
		}
	}
	return wire.ExecuteResponse{
		Envelope: wire.Envelope{Kind: wire.KindExecute, Contract: wire.ContractVersion, ID: id},
		IDs:      ids,
		Response: map[string]any{"status": code, "body": out},
	}
}

func names() string {
	out := make([]string, 0, len(actions))
	for _, act := range actions {
		out = append(out, act.Name)
	}
	return strings.Join(out, ", ")
}

func (a *adapter) observe(id string, m wire.ObserveRequest) any {
	name, subject := m.Query, ""
	if i := strings.IndexByte(m.Query, '/'); i >= 0 {
		name, subject = m.Query[:i], m.Query[i+1:]
	}
	// THE SOURCE IS CHECKED AGAINST THE DECLARED ONE and a mismatch is
	// FaultSource, never an answer through another channel: docs/contract.md
	// says an adapter that cannot observe through the source it was handed
	// says so rather than substituting one.
	var want wire.Source
	switch name {
	case "list_events", "event_count":
		want = wire.SourceAPI
	case "store_event_count":
		want = wire.SourceDirect
	default:
		return fault(id, wire.FaultInternal, "query",
			fmt.Sprintf("%q is not an observation this adapter serves", m.Query))
	}
	if m.Source != want {
		return fault(id, wire.FaultSource, "source",
			fmt.Sprintf("%q is observed through %s and was asked through %s", name, want, m.Source))
	}

	var path string
	switch name {
	case "list_events":
		path = "/events"
	case "event_count":
		if subject == "" {
			return fault(id, wire.FaultField, "query",
				"event_count names one event: the query is event_count/<id>")
		}
		path = "/events/" + subject + "/count"
	case "store_event_count":
		if subject == "" {
			return fault(id, wire.FaultField, "query",
				"store_event_count names one event: the query is store_event_count/<id>")
		}
		path = "/store/events/" + subject + "/count"
	}

	var value any
	code, err := a.call(http.MethodGet, path, m.Token, nil, &value)
	if err != nil {
		return fault(id, wire.FaultInternal, "", err.Error())
	}
	if code >= 400 {
		return fault(id, wire.FaultInternal, "query",
			fmt.Sprintf("the toy answered %s with status %d", path, code))
	}
	return wire.ObserveResponse{
		Envelope: wire.Envelope{Kind: wire.KindObserve, Contract: wire.ContractVersion, ID: id},
		Value:    value,
	}
}

// call makes one HTTP request against the toy and decodes the body into out
// when out is not nil. It returns the status so a caller can tell the system
// refusing an action from the transport failing.
func (a *adapter) call(method, path, token string, body map[string]any, out any) (int, error) {
	var payload io.Reader
	if body != nil {
		raw, err := json.Marshal(prune(body))
		if err != nil {
			return 0, fmt.Errorf("encoding the body for %s %s: %w", method, path, err)
		}
		payload = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, a.base+path, payload)
	if err != nil {
		return 0, fmt.Errorf("building %s %s: %w", method, path, err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return resp.StatusCode, fmt.Errorf("reading the answer to %s %s: %w", method, path, err)
	}
	if out != nil && len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			// The UI route answers HTML, which is not a decode failure of the
			// protocol; hand the body back as a string so the engine sees what
			// the page said.
			if s, ok := out.(*any); ok {
				*s = string(raw)
				return resp.StatusCode, nil
			}
			return resp.StatusCode, fmt.Errorf("decoding the answer to %s %s: %w", method, path, err)
		}
	}
	return resp.StatusCode, nil
}

// prune drops keys whose value is nil, so an action that omits an optional
// argument does not send a null the toy's strict decoder would have to accept.
func prune(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		if v == nil {
			continue
		}
		out[k] = v
	}
	return out
}
