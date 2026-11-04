package fake

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

// drive sends one request line and returns the response line.
func drive(t *testing.T, cfg Config, request map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshalling the request: %v", err)
	}
	var out bytes.Buffer
	if err := Run(cfg, bytes.NewReader(append(b, '\n')), &out); err != nil {
		t.Fatalf("Run: %v", err)
	}
	line := strings.TrimSuffix(out.String(), "\n")
	if line == "" {
		t.Fatal("no response")
	}
	return []byte(line)
}

func request(kind wire.Kind, extra map[string]any) map[string]any {
	m := map[string]any{"kind": string(kind), "contract": wire.ContractVersion, "id": "7"}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func dflt() Config { return Config{Surface: Default(), Omit: map[wire.Kind]string{}} }

// TestEveryAnswerSATISFIESTheRealDecODER is the two-instrument check over this
// package: the fake builds responses as maps, so nothing but package wire's own
// decoder can say whether what it writes is on the protocol.
func TestEveryAnswerSatisfiesTheRealDecoder(t *testing.T) {
	cases := []map[string]any{
		request(wire.KindDescribe, nil),
		request(wire.KindExecute, map[string]any{"action": "create_event", "args": map[string]any{}, "token": "t"}),
		request(wire.KindObserve, map[string]any{"query": "event", "source": "api", "token": "t"}),
		request(wire.KindClock, map[string]any{"op": "advance", "t": 30000000000}),
		request(wire.KindSession, map[string]any{"actor": "alice", "context": "api"}),
		request(wire.KindFlow, map[string]any{"action": "create_event", "token": "t", "steps": []any{
			map[string]any{"step": "navigate", "to": "/events/new"},
		}}),
	}
	if len(cases) != len(wire.Kinds("request")) {
		t.Fatalf("%d requests driven, %d request kinds exist", len(cases), len(wire.Kinds("request")))
	}
	for _, req := range cases {
		line := drive(t, dflt(), req)
		msg, err := wire.DecodeResponse(line)
		if err != nil {
			t.Errorf("%s: the fake's answer is not on the protocol: %v\n%s", req["kind"], err, line)
			continue
		}
		env := msg.Env()
		if env.ID != "7" {
			t.Errorf("%s: answered id %q, want the request's", req["kind"], env.ID)
		}
		if env.Contract != wire.ContractVersion {
			t.Errorf("%s: answered contract %q", req["kind"], env.Contract)
		}
	}
}

func TestRecordedAnswers(t *testing.T) {
	line := drive(t, dflt(), request(wire.KindExecute,
		map[string]any{"action": "create_event", "args": map[string]any{}, "token": "t"}))
	msg, err := wire.DecodeResponse(line)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	exec, ok := msg.(wire.ExecuteResponse)
	if !ok {
		t.Fatalf("answered %T", msg)
	}
	if exec.IDs["event"] != "e-1" {
		t.Errorf("ids %v, want the recorded id", exec.IDs)
	}

	line = drive(t, dflt(), request(wire.KindObserve,
		map[string]any{"query": "event", "source": "api", "token": "t"}))
	msg, err = wire.DecodeResponse(line)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	obs, ok := msg.(wire.ObserveResponse)
	if !ok {
		t.Fatalf("answered %T", msg)
	}
	if obs.Value != "e-1" {
		t.Errorf("value %v, want the recorded observation", obs.Value)
	}

	line = drive(t, dflt(), request(wire.KindSession, map[string]any{"actor": "alice", "context": "api"}))
	msg, _ = wire.DecodeResponse(line)
	sess, ok := msg.(wire.SessionResponse)
	if !ok || sess.Token != Default().Token {
		t.Errorf("session answered %v", msg)
	}
}

// TestAnUnrecordedActionOrQueryIsAnEnvelopeNamingWhatIsRecorded: a fake that
// answered anything would make a drill pass for the wrong reason, and a fake
// that said only "no" would send its user to read this source.
func TestAnUnrecordedActionOrQueryIsAnEnvelopeNamingWhatIsRecorded(t *testing.T) {
	line := drive(t, dflt(), request(wire.KindExecute,
		map[string]any{"action": "nonesuch", "args": map[string]any{}, "token": "t"}))
	msg, err := wire.DecodeResponse(line)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	env, ok := msg.(wire.ErrorResponse)
	if !ok {
		t.Fatalf("answered %T, want an error envelope", msg)
	}
	if env.Error.Kind != wire.FaultAction {
		t.Errorf("kind %q, want action", env.Error.Kind)
	}
	for _, want := range []string{"nonesuch", "create_event", "join_event"} {
		if !strings.Contains(env.Error.Detail, want) {
			t.Errorf("detail %q omits %q", env.Error.Detail, want)
		}
	}

	line = drive(t, dflt(), request(wire.KindObserve,
		map[string]any{"query": "nonesuch", "source": "api", "token": "t"}))
	msg, _ = wire.DecodeResponse(line)
	env, ok = msg.(wire.ErrorResponse)
	if !ok || !strings.Contains(env.Error.Detail, "nonesuch") {
		t.Errorf("an unrecorded query answered %v", msg)
	}
}

// TestFailAnswersEveryRequestWithThatKind is issue 62's "speaks the error
// envelope when asked to".
func TestFailAnswersEveryRequestWithThatKind(t *testing.T) {
	cfg := dflt()
	cfg.Fail = wire.FaultSession
	for _, req := range []map[string]any{
		request(wire.KindDescribe, nil),
		request(wire.KindSession, map[string]any{"actor": "alice", "context": "api"}),
	} {
		msg, err := wire.DecodeResponse(drive(t, cfg, req))
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		env, ok := msg.(wire.ErrorResponse)
		if !ok || env.Error.Kind != wire.FaultSession {
			t.Errorf("%s answered %v", req["kind"], msg)
		}
	}
}

// TestOmitIsD7, end to end: the fake drops one required field and the REAL
// decoder refuses it by name. That pairing is the drill -- a test that only
// checked the JSON lacked a key would prove the deletion and not the refusal.
func TestOmitIsD7EndToEnd(t *testing.T) {
	cfg := dflt()
	cfg.Omit = map[wire.Kind]string{wire.KindExecute: "ids"}
	line := drive(t, cfg, request(wire.KindExecute,
		map[string]any{"action": "create_event", "args": map[string]any{}, "token": "t"}))
	if bytes.Contains(line, []byte(`"ids"`)) {
		t.Fatalf("ids was not omitted: %s", line)
	}
	_, err := wire.DecodeResponse(line)
	if err == nil {
		t.Fatal("the decoder accepted a response missing a required field")
	}
	ref, ok := err.(wire.Refusal)
	if !ok {
		t.Fatalf("%T, want a wire.Refusal", err)
	}
	if ref.Field != "ids" || !strings.Contains(ref.Reason, "missing field") {
		t.Errorf("refusal is %+v, want ids named as missing", ref)
	}
	// THE CONTROL: without --omit the same request decodes, so the refusal is
	// the deletion's and not the request's.
	if _, err := wire.DecodeResponse(drive(t, dflt(), request(wire.KindExecute,
		map[string]any{"action": "create_event", "args": map[string]any{}, "token": "t"}))); err != nil {
		t.Errorf("the unomitted answer is refused: %v", err)
	}
}

// TestContractIsD6: the adapter claims another version, every answer carries it,
// and the engine's own check names all three values.
func TestContractIsD6(t *testing.T) {
	// THE BUMPED VERSION IS DERIVED, NOT TYPED. This read "2", which stopped
	// being a bump the day the engine reached 2 (2026-10-19) -- the test then
	// failed saying the check had accepted a bumped adapter, when what it had
	// accepted was agreement. A bump is "not what the engine says", so it is
	// spelled that way.
	bumped := wire.ContractVersion + "-bumped"
	cfg := dflt()
	cfg.Contract = bumped
	msg, err := wire.DecodeResponse(drive(t, cfg, request(wire.KindDescribe, nil)))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := msg.Env().Contract; got != bumped {
		t.Fatalf("contract %q, want %q", got, bumped)
	}
	err = wire.CheckVersion(msg.Env().Contract, wire.ContractVersion)
	if err == nil {
		t.Fatal("the version check accepted a bumped adapter")
	}
	for _, want := range []string{`"` + bumped + `"`, `"` + wire.ContractVersion + `"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%q omits %s", err, want)
		}
	}
}

// TestAMalformedRequestIsAnsweredRatherThanFatal, and the answer carries the
// decoder's own field: an adapter that exited here would make the engine read a
// transport failure where the fault was one message.
func TestAMalformedRequestIsAnsweredRatherThanFatal(t *testing.T) {
	var out bytes.Buffer
	in := strings.NewReader(`{"kind":"observe","contract":"1","id":"9","query":"event","sorce":"api"}` + "\n")
	if err := Run(dflt(), in, &out); err != nil {
		t.Fatalf("Run: %v", err)
	}
	msg, err := wire.DecodeResponse([]byte(strings.TrimSpace(out.String())))
	if err != nil {
		t.Fatalf("the answer is not on the protocol: %v", err)
	}
	env, ok := msg.(wire.ErrorResponse)
	if !ok {
		t.Fatalf("answered %T", msg)
	}
	if env.Error.Kind != wire.FaultField || env.Error.Field != "sorce" {
		t.Errorf("error %+v, want kind field naming sorce", env.Error)
	}
	if env.Env().ID != "9" {
		t.Errorf("id %q: a readable id is carried even through a refusal", env.Env().ID)
	}
}

func TestOneResponsePerRequestInOrder(t *testing.T) {
	reqs := []map[string]any{
		request(wire.KindSession, map[string]any{"actor": "alice", "context": "api"}),
		request(wire.KindDescribe, nil),
		request(wire.KindObserve, map[string]any{"query": "attendance", "source": "direct"}),
	}
	var in bytes.Buffer
	for _, r := range reqs {
		b, _ := json.Marshal(r)
		in.Write(append(b, '\n'))
	}
	in.WriteString("\n") // a blank line is not a message and gets no answer
	var out bytes.Buffer
	if err := Run(dflt(), &in, &out); err != nil {
		t.Fatalf("Run: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != len(reqs) {
		t.Fatalf("%d request(s) got %d answer(s)", len(reqs), len(lines))
	}
	for i, line := range lines {
		msg, err := wire.DecodeResponse([]byte(line))
		if err != nil {
			t.Fatalf("answer %d: %v", i, err)
		}
		if string(msg.Env().Kind) != reqs[i]["kind"] {
			t.Errorf("answer %d is a %s, want a %s", i, msg.Env().Kind, reqs[i]["kind"])
		}
	}
}

func TestLoadSurfaceIsStrict(t *testing.T) {
	good := `{"actions":[],"observations":[],"contexts":[],"components":[],` +
		`"capabilities":{"faults":[],"volume":[],"upgrade":[]},` +
		`"execute":{},"observe":{},"token":"t"}`
	if _, err := LoadSurface(strings.NewReader(good)); err != nil {
		t.Fatalf("a well-formed surface is refused: %v", err)
	}
	bad := strings.Replace(good, `"token"`, `"tokne"`, 1)
	_, err := LoadSurface(strings.NewReader(bad))
	if err == nil {
		t.Fatal("a mistyped key was accepted, so a drill would get the default's answer silently")
	}
	if !strings.Contains(err.Error(), "tokne") {
		t.Errorf("%q does not name the key", err)
	}
}

// TestTheDefaultSurfaceIsSelfConsistent: a drill copies the default and changes
// ONE thing, so a default that declared an action it could not execute would
// make every drill start from an incoherent world.
func TestTheDefaultSurfaceIsSelfConsistent(t *testing.T) {
	s := Default()
	if len(s.Actions) == 0 || len(s.Observations) == 0 {
		t.Fatal("the default declares nothing, so every drill from it is vacuous")
	}
	for _, a := range s.Actions {
		if _, ok := s.Execute[a.Name]; !ok {
			t.Errorf("action %q is declared and not recorded", a.Name)
		}
		if a.Context == "" {
			t.Errorf("action %q declares no context", a.Name)
		}
		found := false
		for _, c := range s.Contexts {
			if c == a.Context || a.Context == "none" {
				found = true
			}
		}
		if !found {
			t.Errorf("action %q needs context %q, which is not declared", a.Name, a.Context)
		}
	}
	for _, o := range s.Observations {
		if _, ok := s.Observe[o.Name]; !ok {
			t.Errorf("observation %q is declared and not recorded", o.Name)
		}
	}
	for name := range s.Execute {
		declared := false
		for _, a := range s.Actions {
			if a.Name == name {
				declared = true
			}
		}
		if !declared {
			t.Errorf("%q is recorded and not declared", name)
		}
	}
	if s.Token == "" {
		t.Error("no recorded token, so session cannot be answered")
	}
}

func TestAFailedWriteIsReported(t *testing.T) {
	b, _ := json.Marshal(request(wire.KindDescribe, nil))
	err := Run(dflt(), bytes.NewReader(append(b, '\n')), failingWriter{})
	if err == nil {
		t.Fatal("a failed write reported success")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errNoPipe{} }

type errNoPipe struct{}

func (errNoPipe) Error() string { return "no pipe" }

// --id-salt MOVES EVERY ID AND NOTHING ELSE.
//
// The salted sibling of TestRecordedAnswers's id assertion, ruled at coop
// r5/58. Two salts stand for two attempts: the ids must differ between them and
// from the unsalted answer, which is what makes issue 72's done-when
// ("the same handle sequence with different ids") reachable at all.
func TestTheIDSaltMovesEveryIDAndOnlyIDs(t *testing.T) {
	exec := func(cfg Config) wire.ExecuteResponse {
		t.Helper()
		line := drive(t, cfg, request(wire.KindExecute,
			map[string]any{"action": "create_event", "args": map[string]any{}, "token": "t"}))
		msg, err := wire.DecodeResponse(line)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		resp, ok := msg.(wire.ExecuteResponse)
		if !ok {
			t.Fatalf("answered %T: %s", msg, line)
		}
		return resp
	}

	plain := exec(dflt())
	if plain.IDs["event"] != "e-1" {
		t.Fatalf("the unsalted answer must be unchanged, so every existing fixture stays "+
			"byte-identical; got %v", plain.IDs)
	}

	a := dflt()
	a.IDSalt = "att1"
	b := dflt()
	b.IDSalt = "att2"
	one, two := exec(a), exec(b)

	if one.IDs["event"] == plain.IDs["event"] {
		t.Errorf("a salted id must differ from the unsalted one; both are %v", one.IDs["event"])
	}
	if one.IDs["event"] == two.IDs["event"] {
		t.Errorf("two salts must answer two ids, or an attempt comparison cannot see ids move; "+
			"both are %v", one.IDs["event"])
	}
	if one.IDs["event"] != "att1-e-1" || two.IDs["event"] != "att2-e-1" {
		t.Errorf("the salt is a prefix on the recorded id: got %v and %v", one.IDs["event"], two.IDs["event"])
	}

	// DETERMINISTIC: the same salt twice is the same id, or a failing attempt
	// could not replay and the seed would not carry the run.
	if again := exec(a); again.IDs["event"] != one.IDs["event"] {
		t.Errorf("the same salt answered %v then %v, so it is not deterministic",
			one.IDs["event"], again.IDs["event"])
	}

	// AND ONLY IDS. An observation's value is an arbitrary answer -- this
	// surface's `attendance` is the number 1 -- so the salt must not reach it.
	line := drive(t, a, request(wire.KindObserve,
		map[string]any{"query": "event", "source": "api", "token": "t"}))
	msg, err := wire.DecodeResponse(line)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	obs, ok := msg.(wire.ObserveResponse)
	if !ok {
		t.Fatalf("answered %T", msg)
	}
	if obs.Value != "e-1" {
		t.Errorf("the salt reached an observation value, which was never an id: got %v", obs.Value)
	}
}

// A NON-STRING ID IS REFUSED WHEN A SALT IS SET, rather than passed through
// unmoved -- one id silently staying put is exactly the state the attempt
// comparison cannot detect.
func TestANonStringIDIsRefusedUnderASalt(t *testing.T) {
	cfg := dflt()
	cfg.IDSalt = "att1"
	rec := cfg.Surface.Execute["create_event"]
	rec.IDs = map[string]any{"event": 7}
	cfg.Surface.Execute["create_event"] = rec

	line := drive(t, cfg, request(wire.KindExecute,
		map[string]any{"action": "create_event", "args": map[string]any{}, "token": "t"}))
	msg, err := wire.DecodeResponse(line)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	errResp, ok := msg.(wire.ErrorResponse)
	if !ok {
		t.Fatalf("a numeric id under a salt must be an error envelope, not %T: %s", msg, line)
	}
	if !strings.Contains(errResp.Error.Detail, "unmoved") {
		t.Errorf("the fault must say why passing it through is not an option; got %q", errResp.Error.Detail)
	}

	// WITHOUT A SALT the same surface is answered, since nothing is claimed
	// about ids moving.
	cfg.IDSalt = ""
	line = drive(t, cfg, request(wire.KindExecute,
		map[string]any{"action": "create_event", "args": map[string]any{}, "token": "t"}))
	msg, err = wire.DecodeResponse(line)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := msg.(wire.ExecuteResponse); !ok {
		t.Errorf("a numeric id with NO salt must still be answered: %s", line)
	}
}

// A REPEAT EXECUTION ANSWERS A DIFFERENT ID.
//
// Found by driving the walk: two actors each created an event, this adapter
// answered "e-1" both times, and internal/handle REFUSED the second binding --
// "id e-1 is already bound to member_1.event#1, so binding member_2.event#1 to
// it would make two handles name one entity ... a comparison by handle would
// then agree about different things." A server that created two things gave
// them two ids.
//
// THE FIRST CALL IS UNCHANGED, which is the half that keeps every existing
// fixture byte-identical: docs/cli.md's console blocks, the twin's live arms
// and TestRecordedAnswers above all execute each action once.
func TestARepeatExecutionAnswersADifferentID(t *testing.T) {
	ids := func(cfg Config, calls int) []string {
		t.Helper()
		var out []string
		// ONE PROCESS, MANY REQUESTS, because the counter is per-process state:
		// driving each call through a fresh Run would reset it and the arm
		// would read "e-1" every time and pass with the defect present.
		var in strings.Builder
		for i := 0; i < calls; i++ {
			raw, err := json.Marshal(request(wire.KindExecute, map[string]any{
				"action": "create_event", "args": map[string]any{}, "token": "t",
			}))
			if err != nil {
				t.Fatalf("encoding an execute: %v", err)
			}
			in.Write(raw)
			in.WriteString("\n")
		}
		var buf strings.Builder
		if err := Run(cfg, strings.NewReader(in.String()), &buf); err != nil {
			t.Fatalf("Run: %v", err)
		}
		for _, line := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
			msg, err := wire.DecodeResponse([]byte(line))
			if err != nil {
				t.Fatalf("decode %q: %v", line, err)
			}
			resp, ok := msg.(wire.ExecuteResponse)
			if !ok {
				t.Fatalf("answered %T: %s", msg, line)
			}
			id, _ := resp.IDs["event"].(string)
			out = append(out, id)
		}
		return out
	}

	got := ids(dflt(), 3)
	if len(got) != 3 {
		t.Fatalf("three requests answered %d time(s): %v", len(got), got)
	}
	if got[0] != "e-1" {
		t.Errorf("the FIRST call must answer the recorded id unchanged, or every existing fixture "+
			"moves; got %q", got[0])
	}
	seen := map[string]bool{}
	for i, id := range got {
		if seen[id] {
			t.Errorf("call %d answered %q again: two creations with one id cannot both be bound to "+
				"a handle", i+1, id)
		}
		seen[id] = true
	}

	// DETERMINISTIC: the same sequence of requests gives the same ids, so a
	// seed still carries the whole run.
	if again := ids(dflt(), 3); strings.Join(again, ",") != strings.Join(got, ",") {
		t.Errorf("two identical request sequences answered %v then %v", got, again)
	}

	// AND THE SALT COMPOSES WITH IT, since an attempt comparison needs both:
	// different ids within an attempt AND different ids between attempts.
	salted := dflt()
	salted.IDSalt = "att2"
	for i, id := range ids(salted, 3) {
		if id == got[i] {
			t.Errorf("call %d answered %q under a salt and without one: the two mechanisms must "+
				"compose", i+1, id)
		}
	}
}

// TestARecordedMissingIsTheOutcomeAndNotAValue holds the one-word branch this
// package gained for fable's ruling (coop rumor/1669), and its PAIR is the
// point: a surface records `missing` to mean the OUTCOME, and any other string
// is still an ordinary value.
//
// WITHOUT THE SECOND CASE THE BRANCH IS UNHELD. Widening it to every string
// would make a surface recording a word -- an enum, a status, a name -- answer
// as though the subject were unservable, and the walk would write `missing`
// over a value the surface meant literally. The cell therefore carries the
// string the branch must NOT take.
func TestARecordedMissingIsTheOutcomeAndNotAValue(t *testing.T) {
	for _, c := range []struct {
		what, recorded string
		missing        bool
	}{
		{"the word is the outcome", wire.ValueMissing, true},
		{"another string is a value", "pending-elsewhere", false},
	} {
		t.Run(c.what, func(t *testing.T) {
			cfg := dflt()
			cfg.Surface.Observe["subject"] = c.recorded
			raw := drive(t, cfg, request(wire.KindObserve, map[string]any{
				"query": "subject", "source": string(wire.SourceAPI), "token": "fake-token",
			}))
			msg, err := wire.DecodeResponse(raw)
			if err != nil {
				t.Fatalf("the real decoder refused this answer: %v\n%s", err, raw)
			}
			resp, ok := msg.(wire.ObserveResponse)
			if !ok {
				t.Fatalf("not an observe response: %#v", msg)
			}
			if resp.Missing != c.missing {
				t.Errorf("missing is %v, want %v (recorded %q)", resp.Missing, c.missing, c.recorded)
			}
			if c.missing {
				// AND NO VALUE BESIDE IT: the spelling is the walk's to write.
				if resp.Value != nil {
					t.Errorf("a value was written beside missing: %#v", resp.Value)
				}
				t.Logf("PROOF %s, with no value beside it", c.what)
				return
			}
			if resp.Value != c.recorded {
				t.Errorf("the value is %#v, want %q", resp.Value, c.recorded)
			}
			t.Logf("PROOF %s: %q came back as the value", c.what, c.recorded)
		})
	}
}

// R9.1's FAKE DRIVER, BOTH WAYS. It is what makes every head of this child
// drivable with no browser on the box, so what it answers has to be exact.
func TestTheFakeDriverReachesEveryStepItWasGiven(t *testing.T) {
	line := drive(t, dflt(), request(wire.KindFlow, map[string]any{
		"action": "create_event", "token": "t", "steps": []any{
			map[string]any{"step": "navigate", "to": "/events/new"},
			map[string]any{"step": "fill", "field": "title", "value": "x"},
			map[string]any{"step": "click", "target": "submit"},
		}}))
	msg, err := wire.DecodeResponse(line)
	if err != nil {
		t.Fatalf("the fake's flow answer is not on the protocol: %v", err)
	}
	resp, ok := msg.(wire.FlowResponse)
	if !ok {
		t.Fatalf("the fake answered %T where a flow response was asked", msg)
	}
	if resp.Reached != 3 {
		t.Errorf("reached %d of 3 steps: a fake that cannot count its own steps hands the retry "+
			"policy a position it must not trust", resp.Reached)
	}
}

// driveMany feeds SEVERAL requests to ONE adapter, which `drive` cannot: it
// starts a fresh Run per call and the per-action call counter dies with it. The
// retry this head exists for is two calls to the same adapter, so a helper that
// could not do that would make the "fails once, then succeeds" case untestable
// -- and that case is the whole difference between a retry that worked and a
// `ui_timeout`.
func driveMany(t *testing.T, cfg Config, requests ...map[string]any) [][]byte {
	t.Helper()
	var in bytes.Buffer
	for _, r := range requests {
		b, err := json.Marshal(r)
		if err != nil {
			t.Fatalf("marshalling a request: %v", err)
		}
		in.Write(append(b, '\n'))
	}
	var out bytes.Buffer
	if err := Run(cfg, bytes.NewReader(in.Bytes()), &out); err != nil {
		t.Fatalf("Run: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != len(requests) {
		t.Fatalf("%d answer(s) for %d request(s): one request, one line", len(lines), len(requests))
	}
	got := make([][]byte, 0, len(lines))
	for _, l := range lines {
		got = append(got, []byte(l))
	}
	return got
}

// flowOf is the three-step flow these cases drive, built fresh each time so no
// case can mutate another's.
func flowOf(action string, from int) map[string]any {
	m := map[string]any{"action": action, "token": "t", "steps": []any{
		map[string]any{"step": "navigate", "to": "/events/new"},
		map[string]any{"step": "click", "target": "join"},
		map[string]any{"step": "wait", "until": "count-changed"},
	}}
	if from != 0 {
		m["from"] = from
	}
	return request(wire.KindFlow, m)
}

func flowAnswer(t *testing.T, line []byte) wire.FlowResponse {
	t.Helper()
	msg, err := wire.DecodeResponse(line)
	if err != nil {
		t.Fatalf("the fake's flow answer is not on the protocol: %v", err)
	}
	resp, ok := msg.(wire.FlowResponse)
	if !ok {
		t.Fatalf("the fake answered %T where a flow response was asked: %s", msg, line)
	}
	return resp
}

func faultOf(t *testing.T, line []byte) wire.Fault {
	t.Helper()
	msg, err := wire.DecodeResponse(line)
	if err != nil {
		t.Fatalf("the refusal is not on the protocol: %v", err)
	}
	resp, ok := msg.(wire.ErrorResponse)
	if !ok {
		t.Fatalf("answered %T, want an error: %s", msg, line)
	}
	return resp.Error
}

// A CONFIGURED STOP IS WHAT MAKES THE ENGINE'S RETRY DRIVABLE, and these are
// its two shapes. `Times: 1` fails the first call and completes the second,
// which is the retry WORKING; `Times: 0` fails every call, which is the second
// failure of the same wait that fable's r9/1 item 2 makes a `ui_timeout`.
//
// THE PRECONDITION IS ASSERTED FIRST IN BOTH: a case whose first answer was
// already complete would pass its "then it completes" assertion for a reason
// that has nothing to do with the retry.
func TestAStoppedFlowNamesWhereAndWhyItStopped(t *testing.T) {
	cfg := dflt()
	cfg.StopFlow = map[string]StopFlow{"create_event": {At: 2, Reason: wire.StopWait, Times: 1}}

	lines := driveMany(t, cfg, flowOf("create_event", 0), flowOf("create_event", 2))

	first := flowAnswer(t, lines[0])
	if first.Reached != 2 || first.Stopped != wire.StopWait {
		t.Fatalf("first answer reached %d stopped %q, want 2 and %q: without the stop there is no "+
			"retry to drive and every assertion below is vacuous", first.Reached, first.Stopped, wire.StopWait)
	}
	if first.Response != nil {
		t.Errorf("a flow that stopped at %d carried a response %v: the page never reached the "+
			"state that answer describes", first.Reached, first.Response)
	}

	second := flowAnswer(t, lines[1])
	if second.Stopped != wire.StopNone {
		t.Errorf("the resumed flow stopped %q, want empty: Times 1 stops once", second.Stopped)
	}
	if second.Reached != 3 {
		t.Errorf("the resumed flow reached %d of 3: a retry that worked reaches the end", second.Reached)
	}
	if second.Response == nil {
		t.Error("the completed flow carried no response, where the stopped one is the answer-less case")
	}
}

// AND `Times: 0` IS EVERY CALL, which is the case the finding rests on. Driven
// as THREE calls rather than two, because a knob that stopped exactly twice
// would satisfy a two-call case and lie about "every".
func TestAFlowStoppedEveryTimeNeverCompletes(t *testing.T) {
	cfg := dflt()
	cfg.StopFlow = map[string]StopFlow{"create_event": {At: 2, Reason: wire.StopWait}}

	lines := driveMany(t, cfg,
		flowOf("create_event", 0), flowOf("create_event", 2), flowOf("create_event", 2))
	for i, line := range lines {
		resp := flowAnswer(t, line)
		if resp.Stopped != wire.StopWait || resp.Reached != 2 {
			t.Errorf("call %d reached %d stopped %q, want 2 and %q on every call",
				i+1, resp.Reached, resp.Stopped, wire.StopWait)
		}
	}
}

// A STEP THE PAGE WOULD NOT PERFORM IS THE OTHER WORD, and it is here because
// `reached` cannot tell it from a failed wait: both stop at the same index and
// only one is a `ui_timeout`.
func TestAStoppedFlowCanSayTheStepWouldNotGo(t *testing.T) {
	cfg := dflt()
	cfg.StopFlow = map[string]StopFlow{"create_event": {At: 1, Reason: wire.StopStep}}
	resp := flowAnswer(t, drive(t, cfg, flowOf("create_event", 0)))
	if resp.Stopped != wire.StopStep {
		t.Errorf("stopped %q, want %q", resp.Stopped, wire.StopStep)
	}
	if resp.Reached != 1 {
		t.Errorf("reached %d, want 1", resp.Reached)
	}
}

// A STOP CONFIGURED PAST THE FLOW IS THIS ADAPTER'S OWN FAULT, not the
// caller's, and the kind says so: a fake answering `reached` equal to or above
// the step count would be a flow that both completed and stopped, and the
// engine would read a reason with nothing left to retry.
func TestAStopConfiguredPastTheFlowIsAnInternalFault(t *testing.T) {
	for _, at := range []int{3, 4, -1} {
		cfg := dflt()
		cfg.StopFlow = map[string]StopFlow{"create_event": {At: at, Reason: wire.StopWait}}
		fault := faultOf(t, drive(t, cfg, flowOf("create_event", 0)))
		if fault.Kind != wire.FaultInternal {
			t.Errorf("at %d: kind %q, want %q -- a misconfigured fake is not the engine's mistake",
				at, fault.Kind, wire.FaultInternal)
		}
		if fault.Field != "stopFlow" {
			t.Errorf("at %d: the fault names %q, want stopFlow", at, fault.Field)
		}
	}
}

// AND A RESUME PAST THE END REACHES THE CALLER AS AN ENVELOPE NAMING `from`,
// which is the end-to-end half: the READER is wire's decoder and not this
// package, so what this case pins is that the refusal survives the adapter
// rather than being swallowed into a `reached` the flow never drove. It is a
// `field` fault for that reason -- I first wrote it expecting the `action` fault
// this package would have raised, drove it, and found this package's guard
// unreachable.
func TestAFlowResumingPastItsEndIsRefusedByName(t *testing.T) {
	for _, from := range []int{3, 9} {
		fault := faultOf(t, drive(t, dflt(), flowOf("create_event", from)))
		if fault.Kind != wire.FaultField || fault.Field != "from" {
			t.Errorf("from %d: %+v, want a field fault naming from", from, fault)
		}
	}
}

// A FLOW NOT NAMED IN StopFlow IS UNTOUCHED, which is the control that keeps
// every case above from passing because the knob stops everything.
func TestAnUnnamedFlowIsNotStopped(t *testing.T) {
	cfg := dflt()
	cfg.StopFlow = map[string]StopFlow{"create_event": {At: 1, Reason: wire.StopWait}}
	resp := flowAnswer(t, drive(t, cfg, flowOf("join_event", 0)))
	if resp.Stopped != wire.StopNone || resp.Reached != 3 {
		t.Errorf("join_event reached %d stopped %q, want 3 and empty: the knob is keyed by action",
			resp.Reached, resp.Stopped)
	}
}

// A FLOW OF NO STEPS IS A REFUSAL AND NOT `reached: 0`, because zero is also
// what a driver that stopped at the first step would report, and the two are
// different events.
func TestAFlowWithNoStepsIsRefusedRatherThanAnsweredZero(t *testing.T) {
	line := drive(t, dflt(), request(wire.KindFlow, map[string]any{
		"action": "create_event", "token": "t", "steps": []any{}}))
	msg, err := wire.DecodeResponse(line)
	if err != nil {
		t.Fatalf("the refusal is not on the protocol: %v", err)
	}
	resp, ok := msg.(wire.ErrorResponse)
	if !ok {
		t.Fatalf("a flow of no steps answered %T, want an error", msg)
	}
	if resp.Error.Field != "steps" {
		t.Errorf("the refusal names %q, want steps: %+v", resp.Error.Field, resp.Error)
	}
}

// TestAKindThisAdapterDoesNotServeIsAFaultNamingTheKind IS THE CONTRACT
// VERSION'S OWN WITNESS, moved here from cmd/toy-adapter by fable's ruling at
// coop r9/40 part three.
//
// WHY THE VERSION NEEDS A WITNESS AT ALL. `wire.ContractVersion` stays "2"
// through R9.1's `flow`, and the note above that constant gives the reason: the
// version is one string compared exactly with no compatibility window, so a bump
// would refuse an adapter built before the request AT THE HANDSHAKE -- and then
// an adapter that merely LACKS `flow` would be refused for its VERSION and never
// for the thing it lacks. An unmoved version is what keeps the by-name refusal
// reachable, and an argument resting on a refusal nobody has seen fire is prose.
//
// WHY IT IS HERE AND NOT WHERE IT WAS. It drove `flow` at cmd/toy-adapter, whose
// fall-through refused it -- the live case, because that adapter genuinely
// lacked the kind. H2 ended that: examples/toy declares an action with a flow
// and the walk now routes it there, so the toy must serve it. With it served, no
// adapter in this tree lacks the kind by accident, and the live case has to be
// one that lacks it ON PURPOSE.
//
// AND A KNOB IS NOT A FABRICATED SUBJECT, which is rook's objection at r9/37 and
// the reason it does not hold here: rook's point was that a type written for the
// arm would make the kind fabricated, and `Lacks` is in the --fail/--omit
// family, whose whole purpose is an adapter misbehaving in a NAMED way. The kind
// is real, the refusal is this package's own, and the wording comes from
// wire.UnservedKind, which both adapters share.
func TestAKindThisAdapterDoesNotServeIsAFaultNamingTheKind(t *testing.T) {
	flow := request(wire.KindFlow, map[string]any{
		"action": "create_event", "token": "t",
		"steps": []any{map[string]any{"step": "click", "target": "join"}},
	})

	// THE CONTROL FIRST: without the knob this adapter SERVES `flow`, so the
	// refusal below is the knob's doing and not the kind being unsupported.
	if resp := flowAnswer(t, drive(t, dflt(), flow)); resp.Reached != 1 {
		t.Fatalf("the control did not drive the flow (reached %d), so the refusal below says "+
			"nothing about `Lacks`", resp.Reached)
	}

	cfg := dflt()
	cfg.Lacks = map[wire.Kind]bool{wire.KindFlow: true}
	fault := faultOf(t, drive(t, cfg, flow))
	if fault.Kind != wire.FaultInternal {
		t.Errorf("an unserved kind is fault %q, want %q", fault.Kind, wire.FaultInternal)
	}
	if fault.Field != "kind" {
		t.Errorf("the fault names field %q, want kind: a reader has to be told WHICH field of "+
			"the message the adapter could not act on", fault.Field)
	}
	if !strings.Contains(fault.Detail, "flow") {
		t.Errorf("the fault does not name the kind it could not serve: %q", fault.Detail)
	}
	if !strings.Contains(fault.Detail, "does not serve") {
		t.Errorf("the fault does not say it is unserved, so a reader cannot tell it from a "+
			"malformed request: %q", fault.Detail)
	}
}

// AND LACKING ONE KIND LEAVES THE OTHERS SERVED, which is the narrowing: a knob
// that silently refused everything would satisfy the arm above and make the
// adapter useless for the drills it exists for.
func TestLackingOneKindLeavesTheRestServed(t *testing.T) {
	cfg := dflt()
	cfg.Lacks = map[wire.Kind]bool{wire.KindFlow: true}
	msg, err := wire.DecodeResponse(drive(t, cfg, request(wire.KindDescribe, nil)))
	if err != nil {
		t.Fatalf("describe is not on the protocol with --lacks flow set: %v", err)
	}
	if _, ok := msg.(wire.DescribeResponse); !ok {
		t.Errorf("describe answered %T with only `flow` lacking", msg)
	}
}
