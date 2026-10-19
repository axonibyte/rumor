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
		request(wire.KindSession, map[string]any{"actor": "alice", "context": "api"}),
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
