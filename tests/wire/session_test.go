// The kept session, driven against a REAL SPAWNED ADAPTER.
//
// WHY HERE AND NOT internal/wire: the subject spawns a process, and the process
// it spawns is `rumor fake-adapter`. A unit test inside internal/wire would
// have to depend on cmd/rumor being built, which inverts the dependency; this
// is the same split tests/validate's live arms use, and the same TestMain.
//
// WHY A REAL PROCESS AND NOT A PAIR OF PIPES: every behavior under test here is
// about the process -- stdin staying OPEN across exchanges, the adapter
// surviving between them, Close actually stopping it. A pipe pair satisfies the
// request/response shape while answering none of those, so it would be a
// harness that sets up the premise it is meant to test.
package wire_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

// TestMain builds rumor once and puts it on PATH, so `rumor fake-adapter` is
// spawnable by name -- exactly what a project's [adapter].command says.
//
// A FAILED BUILD FAILS THE SUITE rather than skipping it: every arm below
// would otherwise report could-not-run and pass over the subject, which is the
// vacuity this project refuses everywhere else.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "rumor-wire-bin")
	if err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	bin := filepath.Join(dir, "rumor")
	build := exec.Command("go", "build", "-o", bin, "./cmd/rumor")
	build.Dir = filepath.Join("..", "..")
	build.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	if out, berr := build.CombinedOutput(); berr != nil {
		fmt.Fprintf(os.Stderr, "TestMain: building rumor for the session arms: %v\n%s\n", berr, out)
		os.Exit(1)
	}
	if err := os.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH")); err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: %v\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func fakeSpawn(args ...string) wire.Spawn {
	return wire.Spawn{Command: append([]string{"rumor", "fake-adapter"}, args...)}
}

func open(t *testing.T, s wire.Spawn) *wire.Session {
	t.Helper()
	sess, err := wire.Open(context.Background(), s, 20*time.Second)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	return sess
}

// THE HANDSHAKE, and that the adapter is still there afterward. This is the
// one property Describe cannot have: it kills the process it probed.
func TestOpenDescribesAndKeepsTheProcess(t *testing.T) {
	sess := open(t, fakeSpawn())

	d := sess.Described()
	if len(d.Actions) == 0 {
		t.Error("the fake's describe declares no actions, so the handshake carried nothing")
	}

	// THE SECOND EXCHANGE IS THE ASSERTION. Describe closes stdin after
	// writing; if this session did the same, the adapter would have had its EOF
	// and this call could not be answered at all.
	name := d.Actions[0].Name
	if _, err := sess.Execute(wire.ExecuteRequest{Action: name}); err != nil {
		t.Fatalf("a second exchange on a kept session failed, so the process did not survive the "+
			"handshake: %v", err)
	}
}

// MANY EXCHANGES ON ONE PROCESS, each answered in step. A session that lost
// the stream's alignment after the first would show up here and nowhere else.
func TestManyExchangesStayInStepOnOneProcess(t *testing.T) {
	sess := open(t, fakeSpawn())
	name := sess.Described().Actions[0].Name
	for i := 0; i < 5; i++ {
		if _, err := sess.Execute(wire.ExecuteRequest{Action: name}); err != nil {
			t.Fatalf("exchange %d of 5 failed: %v", i+1, err)
		}
	}
}

// THE ID IS A CHECK AND NOT A ROUTER (contract.md:36), and the session stamps
// it rather than the caller. An envelope a caller filled in is OVERWRITTEN --
// otherwise a caller could set the id that the id check exists to make
// meaningful, and the check would be testing the caller's bookkeeping.
func TestTheSessionStampsTheEnvelopeAndACallerCannotSetIt(t *testing.T) {
	sess := open(t, fakeSpawn())
	name := sess.Described().Actions[0].Name

	// A deliberately wrong envelope: someone else's id, a bogus kind and a
	// contract this build does not speak. If any of the three reached the
	// wire, the fake would answer a fault or the id check would fire.
	_, err := sess.Execute(wire.ExecuteRequest{
		Envelope: wire.Envelope{Kind: wire.KindObserve, Contract: "not-a-contract", ID: "999"},
		Action:   name,
	})
	if err != nil {
		t.Fatalf("the session must overwrite a caller's envelope, so this had to succeed: %v", err)
	}
}

// A CANNED-RESPONSE ADAPTER, for the three behaviors the fake cannot produce.
//
// WHY THE FAKE IS NOT ENOUGH: `--fail` is global, so a fake told to fault
// faults the DESCRIBE too and Open never returns -- there is no way to ask it
// for a healthy handshake followed by a fault. The fake also cannot answer with
// somebody else's id, and cannot go silent on one request only.
//
// WHY ITS FIRST LINE IS THE FAKE'S OWN DESCRIBE: a describe I hand-wrote would
// be a fixture under test rather than an input, and the strict decoder would
// then be judging my JSON. So the real `rumor fake-adapter` is asked one
// describe and its answer is replayed verbatim.
//
// WHY `cat` AND NOT A READER: the session writes a request and reads a line, so
// an adapter that prints every line it will ever send and then waits is
// indistinguishable from one answering in turn -- the reader goroutine buffers
// them and each exchange takes the next. `sleep` keeps the process alive so
// that a dead adapter cannot be mistaken for a quiet one; Close kills it.
func cannedAdapter(t *testing.T, lines ...string) wire.Spawn {
	t.Helper()
	f := filepath.Join(t.TempDir(), "canned.jsonl")
	body := ""
	for _, l := range lines {
		body += l + "\n"
	}
	if err := os.WriteFile(f, []byte(body), 0o600); err != nil {
		t.Fatalf("writing the canned responses: %v", err)
	}
	return wire.Spawn{Command: []string{"sh", "-c", "cat " + f + "; sleep 30"}}
}

// realDescribeLine asks the actual fake for one describe and returns its answer
// verbatim, stamped with the id a session's FIRST request carries.
func realDescribeLine(t *testing.T) string {
	t.Helper()
	req := wire.DescribeRequest{Envelope: wire.Envelope{
		Kind: wire.KindDescribe, Contract: wire.ContractVersion, ID: "1",
	}}
	line, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("encoding a describe: %v", err)
	}
	cmd := exec.Command("rumor", "fake-adapter")
	cmd.Stdin = strings.NewReader(string(line) + "\n")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("asking the fake for a describe: %v", err)
	}
	first := strings.SplitN(strings.TrimRight(string(out), "\n"), "\n", 2)[0]
	if first == "" {
		t.Fatal("the fake answered no describe line, so every canned arm would be testing nothing")
	}
	// THE REPLAY IS ONLY FAITHFUL IF IT STILL DECODES. Asserted here so a
	// change in the fake's output cannot quietly turn these arms into tests of
	// a malformed first line.
	if _, derr := wire.DecodeResponse([]byte(first)); derr != nil {
		t.Fatalf("the fake's describe does not decode, so it cannot stand in for a handshake: %v", derr)
	}
	return first
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("encoding a canned response: %v", err)
	}
	return string(b)
}

// AN ERROR ENVELOPE IS AN ANSWER, NOT A POISON. contract.md is explicit that
// every envelope is could-not-run and that an adapter cannot refuse a project,
// so the session stays usable and the caller decides what the fault means.
//
// THE TWO FAULTS CARRY DIFFERENT DETAILS, and that is the whole arm. My first
// version asserted that the second error said "fault" and did not say "cannot
// be used again" -- which a POISONED session satisfies perfectly, because the
// poison it replays IS the first fault's error. Driving the mutant that poisons
// on a fault showed the arm passing with the defect present. A live session
// reads the NEXT canned line and reports the second detail; a poisoned one can
// only repeat the first.
func TestAFaultEnvelopeLeavesTheSessionUsable(t *testing.T) {
	const firstDetail = "canned-fault-one"
	const secondDetail = "canned-fault-two"
	fault := func(id, detail string) string {
		return mustJSON(t, wire.ErrorResponse{
			Envelope: wire.Envelope{Kind: wire.KindError, Contract: wire.ContractVersion, ID: id},
			Error:    wire.Fault{Kind: wire.FaultInternal, Detail: detail},
		})
	}
	sess := open(t, cannedAdapter(t, realDescribeLine(t), fault("2", firstDetail), fault("3", secondDetail)))

	_, err := sess.Execute(wire.ExecuteRequest{Action: "anything"})
	if err == nil {
		t.Fatal("a fault envelope must not read as a success")
	}
	if !strings.Contains(err.Error(), "fault") || !strings.Contains(err.Error(), string(wire.FaultInternal)) {
		t.Errorf("the error must say it was a fault envelope and name the kind; got %v", err)
	}
	if !strings.Contains(err.Error(), firstDetail) {
		t.Errorf("the error must carry the adapter's own detail; got %v", err)
	}

	_, err = sess.Execute(wire.ExecuteRequest{Action: "anything"})
	if err == nil {
		t.Fatal("the second execute read as a success, which the canned adapter cannot give")
	}
	if strings.Contains(err.Error(), firstDetail) {
		t.Errorf("the second exchange replayed the FIRST fault, so the session was poisoned by an "+
			"error envelope and must not have been: %v", err)
	}
	if !strings.Contains(err.Error(), secondDetail) {
		t.Errorf("the second exchange did not reach the adapter: want the second canned detail, got %v", err)
	}
}

// AN ANSWER CARRYING SOMEBODY ELSE'S ID IS REFUSED NAMING BOTH, which is
// contract.md:36's own wording -- "id is a check rather than a router". It is
// also what an unsolicited line produces, since the next exchange reads it and
// finds a stale id.
func TestAnAnswerWithTheWrongIDIsRefusedNamingBoth(t *testing.T) {
	stale := mustJSON(t, wire.ExecuteResponse{
		Envelope: wire.Envelope{Kind: wire.KindExecute, Contract: wire.ContractVersion, ID: "99"},
		IDs:      map[string]any{},
	})
	sess := open(t, cannedAdapter(t, realDescribeLine(t), stale))

	_, err := sess.Execute(wire.ExecuteRequest{Action: "anything"})
	if err == nil {
		t.Fatal("a response whose id is not the outstanding request's must be refused")
	}
	if !strings.Contains(err.Error(), "99") || !strings.Contains(err.Error(), `"2"`) {
		t.Errorf("the refusal must name BOTH ids -- the one answered and the one outstanding; got %v", err)
	}
}

// A SILENT ADAPTER IS BOUNDED PER EXCHANGE, not per run: the handshake
// succeeds, the next request is never answered, and the timeout refuses that
// one exchange. A run-long ceiling would have made this indistinguishable from
// a slow action.
func TestASilentExchangeTimesOutAndPoisons(t *testing.T) {
	sess, err := wire.Open(context.Background(), cannedAdapter(t, realDescribeLine(t)), 300*time.Millisecond)
	if err != nil {
		t.Fatalf("the handshake must still succeed, since its line was answered: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })

	start := time.Now()
	if _, err := sess.Execute(wire.ExecuteRequest{Action: "anything"}); err == nil {
		t.Fatal("an unanswered exchange must not read as a success")
	} else if !strings.Contains(err.Error(), "did not answer") {
		t.Errorf("the refusal must say the adapter did not answer; got %v", err)
	}
	if waited := time.Since(start); waited > 10*time.Second {
		t.Errorf("the exchange waited %s, so it was bounded by something other than the "+
			"per-request timeout", waited)
	}
	if _, err := sess.Execute(wire.ExecuteRequest{Action: "anything"}); err == nil ||
		!strings.Contains(err.Error(), "did not answer") {
		t.Errorf("a timed-out session is mid-exchange and must stay poisoned with the same "+
			"reason; got %v", err)
	}
}

// A MALFORMED ANSWER POISONS, and the poison is STICKY.
//
// THE THIRD CANNED LINE IS A PERFECTLY GOOD RESPONSE, and that is what makes
// stickiness testable. My first version used the fake's `--omit execute.id`,
// which answers EVERY execute the same way -- so a session that did not stay
// poisoned would produce the identical error on its second call, and the arm
// passed with the non-sticky mutant in place. Here a session that carried on
// would read a valid line and SUCCEED, so only the sticky one refuses.
func TestAMalformedAnswerPoisonsTheSessionStickily(t *testing.T) {
	// An execute response with no `id`: the strict decoder refuses it by name,
	// which is the state that leaves the stream of unknown alignment.
	malformed := mustJSON(t, map[string]any{
		"kind": string(wire.KindExecute), "contract": wire.ContractVersion, "ids": map[string]any{},
	})
	good := mustJSON(t, wire.ExecuteResponse{
		Envelope: wire.Envelope{Kind: wire.KindExecute, Contract: wire.ContractVersion, ID: "3"},
		IDs:      map[string]any{},
	})
	sess := open(t, cannedAdapter(t, realDescribeLine(t), malformed, good))

	_, err := sess.Execute(wire.ExecuteRequest{Action: "anything"})
	if err == nil {
		t.Fatal("an answer with no id must not read as a success")
	}
	if !strings.Contains(err.Error(), "well-formed") {
		t.Errorf("the first refusal must say the answer was not a well-formed response; got %v", err)
	}

	_, err = sess.Execute(wire.ExecuteRequest{Action: "anything"})
	if err == nil {
		t.Fatal("a poisoned session carried on and read the next line, which it cannot be trusted " +
			"to do: the stream's alignment is unknown after a refused decode")
	}
	// THE SAME REASON, not a fresh one: a caller that gets a different error on
	// the second call learns nothing about the first.
	if !strings.Contains(err.Error(), "well-formed") {
		t.Errorf("the second refusal must carry the ORIGINAL reason; got %v", err)
	}
}

// CLOSE STOPS THE PROCESS, and is idempotent. Without the second call this arm
// could pass over a Close that panics on a closed channel the second time --
// which a run's deferred Close plus an explicit one would hit immediately.
func TestCloseStopsTheAdapterAndIsIdempotent(t *testing.T) {
	sess, err := wire.Open(context.Background(), fakeSpawn(), 20*time.Second)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	name := sess.Described().Actions[0].Name

	if err := sess.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if err := sess.Close(); err != nil {
		t.Errorf("Close is called from a defer AND explicitly in a run, so it must be idempotent: %v", err)
	}
	if _, err := sess.Execute(wire.ExecuteRequest{Action: name}); err == nil {
		t.Error("an exchange on a closed session answered success, so the adapter was not stopped")
	}
}

// NO COMMAND IS REFUSED BEFORE ANYTHING IS SPAWNED, with the platform named
// rather than a bare exec error.
func TestOpenWithNoCommandIsRefusedByName(t *testing.T) {
	_, err := wire.Open(context.Background(), wire.Spawn{}, time.Second)
	if err == nil {
		t.Fatal("a Spawn with no command must be refused")
	}
	if !strings.Contains(err.Error(), "[adapter].command") {
		t.Errorf("the refusal must name what is missing and where it is declared; got %v", err)
	}
}

// A COMMAND THAT CANNOT BE SPAWNED IS could-not-run AND NAMES THE BINARY, not
// a refusal of the project: "the project is not wrong; its adapter is".
func TestOpenNamesABinaryItCannotSpawn(t *testing.T) {
	_, err := wire.Open(context.Background(), wire.Spawn{Command: []string{"rumor-no-such-adapter-xyz"}},
		time.Second)
	if err == nil {
		t.Fatal("spawning a binary that does not exist must fail")
	}
	if !strings.Contains(err.Error(), "rumor-no-such-adapter-xyz") {
		t.Errorf("the error must name the binary it could not spawn; got %v", err)
	}
}
