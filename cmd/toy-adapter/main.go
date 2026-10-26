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
		{Name: "recount_event", Context: "api"},
		{Name: "join_event_ui", Context: "browser"},
	}
	observations = []wire.Observation{
		{Name: "event_count", Source: wire.SourceAPI},
		// THE WORKER'S OWN COUNT, asked about a RECOUNT and not about an event.
		// It is an `api` observation: an actor reads it through its own
		// session, where `direct` is the engine's look into the store.
		{Name: "event_tally", Source: wire.SourceAPI},
		{Name: "store_event_count", Source: wire.SourceDirect},
	}
	contexts = []string{"api", "browser"}
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
	// stateDir is the temp directory this adapter MADE, and only that. A
	// --state path the caller supplied is the caller's to remove, so this
	// stays empty there and close() has nothing to delete.
	stateDir string

	// THE REQUEST BUDGET AND WHAT IT HAS ACTUALLY COST, which is the half
	// nobody could see. ratchet measured at coop r5/200 that the headroom of
	// this timeout "is not measurable from outside the adapter": the test's
	// whole `go test` invocation is 6-8s warm, the budget governs ONE request
	// inside it, and NEITHER figure gives the margin between them. So the
	// constant was chosen without a measurement and could not be validated
	// where it sat -- and raising it on the same reasoning that set it would
	// just move an unmeasured guess, however much less often it failed.
	//
	// These three fields are that measurement, taken from inside.
	budget  time.Duration
	calls   int
	slowest time.Duration
	// slowestWhat is the route that cost `slowest`, because a headroom figure
	// with no route attached cannot be acted on.
	slowestWhat string
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
		// CLOSED BY HAND because os.Exit runs no deferred function: the defer
		// above never fires on this path, so without this the toy AND its
		// directory leak on exactly the runs that went wrong.
		a.close()
		os.Exit(1)
	}
}

// open starts a toy unless TOY_ADDR names a running one.
//
// A DEFERRED CLEANUP ON THE FAILURE PATHS, so the temp directory cannot
// outlive a failed open. Every return below MkdirTemp is an error path and each
// one used to leave a directory behind; one deferred close covers them together
// and covers whatever path is added next, which naming them one at a time would
// not. On the happy path the caller's own close() does it (quill, coop r5/102:
// the leak was on every run, clean ones included -- 11 of 11).
//
// THE FLAG RATHER THAN A NAMED RETURN, and my first version got this wrong.
// With `(a *adapter, err error)` the deferred function reads `a` AFTER
// `return nil, fmt.Errorf(...)` has already assigned nil to it, so the cleanup
// called close() on a nil adapter and panicked -- caught by the failing-open
// cell in the test beside this, which is the cell that exists for this path.
// A local flag reads nothing the returns have touched.
func open(toyBin, state string) (*adapter, error) {
	a := &adapter{budget: requestBudget, client: &http.Client{Timeout: requestBudget}}
	opened := false
	defer func() {
		if !opened {
			a.close()
		}
	}()
	if addr := os.Getenv("TOY_ADDR"); addr != "" {
		a.base = "http://" + strings.TrimPrefix(strings.TrimPrefix(addr, "http://"), "https://")
		opened = true
		return a, nil
	}
	if state == "" {
		dir, err := os.MkdirTemp("", "toy-adapter-*")
		if err != nil {
			return nil, fmt.Errorf("making a state directory: %w", err)
		}
		a.stateDir = dir
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
	opened = true
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

// close stops the toy this adapter started and removes the directory it made.
//
// THE CHILD DIES FIRST, which is the order and not an accident: the toy holds
// state.json open and writes it, so removing the directory under a live toy
// would race its last write. Kill then Wait then RemoveAll.
// requestBudget is how long ONE request to the toy may take before this adapter
// gives up.
//
// THE NUMBER IS CHOSEN FROM THE MEASUREMENT BELOW IT, which is the whole point
// of instrumenting first (ratchet, coop r5/200): "raising 10s to 60s on the
// same reasoning that set it at 10s just moves an unmeasured guess, even though
// it will certainly fail less often."
//
// THE MEASUREMENT, taken 2026-10-26 at vm.loadavg 7.09 on a FOUR-CPU box that
// three jails share -- so a busy one, which is the only kind available here:
//
//	one request to the toy        median   2.0ms   worst  24.2ms  (30 requests)
//	process start to first answer median  10.4ms   worst  27.2ms  (12 starts)
//
// AND THAT CHANGES WHAT THE NUMBER IS FOR. The old 10s already stood at roughly
// 370 TIMES the worst work observed, so the budget was never below the work it
// measures -- which was the premise fable's r5/196 placement rested on and
// which rook and ratchet had already withdrawn as unmeasured at r5/199 and
// /200, load having failed to predict which run paid. An expiry here therefore
// means a STALL of ten seconds or more, not a request that ran slightly long.
//
// SO 60s IS STALL TOLERANCE AND NOT A HEADROOM FIGURE, and saying which is the
// point: it buys survival of a 10-to-60s stall and nothing else, and it fixes
// no cause. What makes it safe rather than merely larger is the half beside it
// -- an expiry now names the budget and the elapsed and says the instrument
// gave up, so a 60s wait reads as this adapter's patience running out rather
// than as a verdict on the toy. The cause of the original stalls remains
// UNMEASURED and is recorded as such in the R5 gate record.
const requestBudget = 60 * time.Second

func (a *adapter) close() {
	// THE HEADROOM IS PUBLISHED ONCE, AT CLOSE, AND ON STDERR. Stdout is the
	// wire -- one JSON response per line -- so a figure printed there would be
	// a protocol violation; stderr is where this adapter already says things to
	// a human.
	//
	// ONCE AND NOT PER REQUEST, which is ratchet's flood at coop main 12/259: a
	// per-poll line for a persistent condition drowns the thing it reports.
	// One summary names the count, the worst route and the margin, which is
	// everything needed to choose the number.
	if a.calls > 0 {
		head := a.budget - a.slowest
		_, _ = fmt.Fprintf(os.Stderr, "toy-adapter: %d request(s), slowest %s (%s), budget %s, "+
			"headroom %s\n", a.calls, a.slowest.Round(time.Millisecond), a.slowestWhat,
			a.budget, head.Round(time.Millisecond))
	}
	if a.child != nil && a.child.Process != nil {
		_ = a.child.Process.Kill()
		_ = a.child.Wait()
	}
	if a.stateDir != "" {
		_ = os.RemoveAll(a.stateDir)
		a.stateDir = ""
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
		"error": map[string]any{
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
	case "recount_event":
		// IT CREATES A `recount`, which is what gives the observation of the
		// worker's tally a subject of its own: the handle the walk mints for
		// this step resolves to the recount's id, and nothing else in a run
		// mints that kind. An observation declaring the EVENT handle instead
		// is makeable on every step that touches an event (ibis, coop r6/100).
		p = plan{http.MethodPost, "/events/" + subject + "/recount", nil, "recount"}
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
	// THE SUBJECT COMES OUT OF `args`, which is the shape the execute path
	// above already has at its own m.Args["event"].
	//
	// THIS PATH SPLIT THE QUERY ON A SLASH until now -- contract 1's form --
	// while contract 2 moved the observation's arguments into `args` on
	// 2026-10-19 (docs/contract.md's v2 row, "observe gains args") and
	// examples/toy has declared `args = ["event"]` on both observations ever
	// since. So the two halves disagreed in the one direction nothing could
	// see: the project was right, the wire type carried Args, and this
	// adapter -- the tree's only query-splitter -- was the half left behind.
	// A real walk died at its first observation with "event_count names one
	// event" (quill, coop r5/163), which is this function faulting correctly
	// on a request it would not read.
	//
	// The query is the observation's NAME ALONE, which bindings.toml says in
	// its own words beside the args row it declares.
	name := m.Query
	subject, _ := m.Args["event"].(string)
	// THE SOURCE IS CHECKED AGAINST THE DECLARED ONE and a mismatch is
	// FaultSource, never an answer through another channel: docs/contract.md
	// says an adapter that cannot observe through the source it was handed
	// says so rather than substituting one.
	var want wire.Source
	switch name {
	case "event_count", "event_tally":
		want = wire.SourceAPI
	case "store_event_count":
		want = wire.SourceDirect
	// THE PROBE QUERIES, which R6.2 reserved as the two reported kinds' own
	// names. internal/quiesce asks for `queue_depth` and `worker_idle` through
	// source `direct` and nothing else: the wire does not move, because
	// `direct` is the engine's own observation and its Query is opaque to the
	// decoder (docs/contract.md). An adapter that serves them answers from
	// whatever its system exposes; this one asks the toy.
	case "queue_depth", "worker_idle":
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
	case "event_count":
		if subject == "" {
			return fault(id, wire.FaultField, "args",
				"event_count names one event: send it as args.event")
		}
		path = "/events/" + subject + "/count"
	case "event_tally":
		// THE SUBJECT IS A RECOUNT, so the arg is `recount` and not `event`.
		rid, _ := m.Args["recount"].(string)
		if rid == "" {
			return fault(id, wire.FaultField, "args",
				"event_tally names one recount: send it as args.recount")
		}
		path = "/recounts/" + rid + "/tally"
	case "store_event_count":
		if subject == "" {
			return fault(id, wire.FaultField, "args",
				"store_event_count names one event: send it as args.event")
		}
		path = "/store/events/" + subject + "/count"
	// NO SUBJECT, AND THAT IS NOT AN OMISSION: the queue is the toy's and not
	// one event's, so a probe naming an event would be asking a question this
	// system does not have. A quiesce probe asks whether the SYSTEM settled.
	case "queue_depth", "worker_idle":
		path = "/store/queue"
	}

	// THE VALUE IS THE COUNT, NOT THE BODY THAT CARRIES IT, and this is the
	// second half of the same contract drift as the args read above.
	//
	// Both count routes answer {"event":..,"count":N,"deleted":..}, and this
	// function used to pass that whole object through as the observation's
	// value. The reviewed definition of this observation's value is a SCALAR:
	// examples/toy/cases/honest-count.json carries `"value": 1` and the
	// statute compares it to `believed`, which is
	// `count(joined_by_observer) - count(left_by_observer)` -- a number. An
	// object never equals a number, so once the args fix let a real walk's
	// observation through, EVERY observation fell to `default finding :=
	// "bug"` and every seed ended at its first observable step. Measured on
	// seeds 34, 35, 6, 7 and 4242 before this hunk: 4, 5, 4, 4 and 2 steps of
	// ten, each with a finding and no disagreement to justify it.
	//
	// `deleted` is deliberately dropped rather than carried alongside: the
	// statute reads the ledger for that -- `deleted` is a fact it derives from
	// the delete_event action -- and an observation that also reported it
	// would be giving the statute a second, unreviewed source for something it
	// already decides.
	var body struct {
		Count int  `json:"count"`
		Tally int  `json:"tally"`
		Depth int  `json:"depth"`
		Idle  bool `json:"idle"`
	}
	code, err := a.call(http.MethodGet, path, m.Token, nil, &body)
	if err != nil {
		return fault(id, wire.FaultInternal, "", err.Error())
	}
	if code >= 400 {
		return fault(id, wire.FaultInternal, "query",
			fmt.Sprintf("the toy answered %s with status %d", path, code))
	}
	var value any = body.Count
	// EACH PROBE'S VALUE IS THE ONE FIELD ITS KIND IS ABOUT, and a scalar for
	// the reason the count above is one: internal/quiesce compares the answer
	// with the project's declared `quiet`, which is a TOML scalar. Handing back
	// the whole body would never equal it, and every probe would read busy for
	// ever -- the silent-and-total failure that package's own arms exist for.
	switch name {
	case "event_tally":
		value = body.Tally
	case "queue_depth":
		value = body.Depth
	case "worker_idle":
		value = body.Idle
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
	start := time.Now()
	resp, err := a.client.Do(req)
	lat := time.Since(start)
	a.calls++
	if lat > a.slowest {
		a.slowest, a.slowestWhat = lat, method+" "+path
	}
	if err != nil {
		// AN EXPIRY PRESENTS AS THE ADAPTER'S OWN FAULT, NAMING THE BUDGET AND
		// THE ELAPSED (fable, coop r5/206 item 3). Before this the error was
		// http's own "context deadline exceeded", which says nothing about
		// whose budget was exceeded or by how much -- so a reader could not
		// tell a wedged toy from a budget set below the work. `run` reports it
		// as could-not-run, which is CORRECT and unchanged: the system did not
		// answer, so what an observation would have said is unknown.
		if os.IsTimeout(err) {
			return 0, fmt.Errorf("%s %s: the toy did not answer within this adapter's %s "+
				"request budget (elapsed %s); the budget is the ADAPTER's and not the toy's, "+
				"so this is the instrument giving up rather than a verdict on the system: %w",
				method, path, a.budget, lat.Round(time.Millisecond), err)
		}
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
