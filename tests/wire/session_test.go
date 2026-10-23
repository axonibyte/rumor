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
	"strconv"
	"strings"
	"syscall"
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

// toyStatute is examples/toy's statute, so the walk arms evaluate applicability
// by real rules rather than by a permissive stub of my own.
func toyStatute(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "examples", "toy", "statute.rego"))
	if err != nil {
		t.Fatalf("the toy's statute: %v", err)
	}
	return string(b)
}

// CLOSE LETS THE ADAPTER CLEAN UP AFTER ITSELF, which is R5.4's exit defect:
// fable measured it at coop r5/100 and ruled the fix the session's.
//
// WHAT WAS WRONG. Close closed stdin and killed AT ONCE, and an adapter's
// cleanup is its own `defer` -- which does not run under SIGKILL. So whether
// cmd/toy-adapter reaped the toy it had spawned was a race. Reproduced here
// 2026-10-22 before the fix: `rumor run` hung to a 15s bound in 4 of 8 runs and
// `rumor validate` in 2 of 3, each having already printed a complete result,
// because timeout(1) waits for the orphan it inherits.
//
// THE FAKE SPAWNS A CHILD AND REMOVES ITS DIRECTORY, in that order, which is
// what makes one observable stand for both halves of "no child alive and no
// state dir behind": the `rm -rf` is reachable only after `wait` has reaped the
// child, so a missing directory is proof the adapter's teardown ran to
// completion. The directory is the proxy and the script's ORDER is what closes
// the distance -- stated because a proxy whose distance is not named is the
// defect one layer up.
//
// THE CHILD'S PID IS CHECKED TOO, and it cannot be fooled by a zombie here for
// the same reason: the fake `wait`s, so there is nothing left to be a zombie of.
//
// TWO CELLS. The honest adapter must get its EOF and clean up; the MUTANT
// ignores EOF entirely and must still be stopped, bounded, because the kill is
// what Describe's comment keeps and a grace that became a wait would hand any
// wedged adapter the run's teardown.
//
// NOT BACKGROUNDED, NOT `cat`: this reuses what walk_test.go's fixture comment
// paid for -- a backgrounded reader has its stdin redirected to /dev/null by
// POSIX and sees EOF at once, and `cat > file` block-buffers to a regular file.
// The reader loop is the foreground process and answers per request.
func TestCloseLetsTheAdapterCleanUpBeforeKilling(t *testing.T) {
	describe := mustJSON(t, wire.DescribeResponse{
		Envelope:     wire.Envelope{Kind: wire.KindDescribe, Contract: wire.ContractVersion, ID: "1"},
		Actions:      []wire.Action{},
		Observations: []wire.Observation{},
		Contexts:     []string{},
		Components:   []wire.Component{},
	})

	// `honest` reaps its child and removes its directory when stdin closes;
	// `mutant` answers and then ignores EOF forever.
	adapter := func(t *testing.T, honest bool) (wire.Spawn, string, string) {
		t.Helper()
		dir := filepath.Join(t.TempDir(), "state")
		pidFile := filepath.Join(t.TempDir(), "child.pid")
		// THE KILL IS PART OF THE CLEANUP AND I LEFT IT OUT FIRST: `wait` alone
		// waits for `sleep 300` to finish on its own, so the honest cell failed
		// against a correct fix and said the fix was broken. No `set -e`
		// either, since `wait` on a killed child answers non-zero and would
		// abort the shell before the `rm`.
		tail := "kill $child 2>/dev/null\nwait $child 2>/dev/null\nrm -rf " + dir + "\n"
		if !honest {
			tail = "while :; do sleep 1; done\n"
		}
		script := "mkdir -p " + dir + "\n" +
			"sleep 300 &\n" +
			"child=$!\n" +
			"printf '%s\\n' \"$child\" > " + pidFile + "\n" +
			"while IFS= read -r l; do\n" +
			"  printf '%s\\n' '" + describe + "'\n" +
			"done\n" +
			tail
		return wire.Spawn{Command: []string{"sh", "-c", script}}, dir, pidFile
	}

	// gone reports whether a pid is no longer a process at all. Signal 0 is the
	// existence test and never delivered.
	gone := func(pid int) bool {
		return syscall.Kill(pid, 0) != nil
	}
	childOf := func(t *testing.T, pidFile string) int {
		t.Helper()
		raw, err := os.ReadFile(pidFile)
		if err != nil {
			t.Fatalf("the fake adapter wrote no child pid: %v", err)
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
		if err != nil {
			t.Fatalf("the child pid %q does not parse: %v", raw, err)
		}
		return pid
	}

	t.Run("honest adapter cleans up", func(t *testing.T) {
		spawn, dir, pidFile := adapter(t, true)
		sess, err := wire.Open(context.Background(), spawn, 5*time.Second)
		if err != nil {
			t.Fatalf("opening a session against the spawning fake: %v", err)
		}
		pid := childOf(t, pidFile)
		if gone(pid) {
			t.Fatalf("the fake's child %d was already gone before Close, so this arm has no "+
				"subject", pid)
		}
		if _, err := os.Stat(dir); err != nil {
			t.Fatalf("the fake made no state directory, so its removal proves nothing: %v", err)
		}

		start := time.Now()
		if err := sess.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		took := time.Since(start)

		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("the adapter's state directory survived Close (%v), so its teardown did not "+
				"run: Close killed it before EOF could reach it", err)
		}
		if !gone(pid) {
			t.Errorf("the child %d the adapter spawned is still alive after Close, so the adapter "+
				"never reaped it", pid)
		}
		// AND IT DID NOT SPEND THE GRACE: an honest adapter exits on EOF, so
		// the whole point is that this costs nothing. A Close that took the
		// full bound would mean the EOF never arrived and the kill did the
		// work -- passing the two assertions above by accident is not possible,
		// but passing them slowly would mean something else was fixed.
		if took >= wire.StopGrace {
			t.Errorf("Close took %v, which is the whole grace: the adapter exited on the kill "+
				"rather than on EOF", took)
		}
		// THE PROOF LINE READS THE WORLD rather than restating the intent: a
		// log that says "directory gone" beside an assertion that just failed
		// is a verdict written next to its own test, and this one printed
		// exactly that while the cell was red.
		_, statErr := os.Stat(dir)
		t.Logf("PROOF honest adapter: directory exists=%v, child %d alive=%v, Close took %v of a "+
			"%v grace", statErr == nil, pid, !gone(pid), took, wire.StopGrace)
	})

	t.Run("an adapter ignoring EOF is still stopped", func(t *testing.T) {
		spawn, _, pidFile := adapter(t, false)
		sess, err := wire.Open(context.Background(), spawn, 5*time.Second)
		if err != nil {
			t.Fatalf("opening a session against the EOF-ignoring fake: %v", err)
		}
		pid := childOf(t, pidFile)

		start := time.Now()
		if err := sess.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		took := time.Since(start)

		if took < wire.StopGrace {
			t.Errorf("Close returned in %v against an adapter that never exits, which is under "+
				"the %v grace: it cannot have waited", took, wire.StopGrace)
		}
		if took > 4*wire.StopGrace {
			t.Errorf("Close took %v against an adapter that ignores EOF: the grace is a bound and "+
				"this is not bounded by it", took)
		}
		// The child is NOT asserted dead here, and that is the honest reading:
		// this adapter was killed, so its own cleanup could not run, which is
		// exactly the state the honest cell exists to contrast with.
		t.Logf("PROOF EOF-ignoring adapter: stopped in %v against a %v grace (child %d was never "+
			"reaped by it, which is what being killed means)", took, wire.StopGrace, pid)
	})
}

// SESSION.OBSERVE, DRIVEN AT ALL FOR THE FIRST TIME. rook measured it at
// coop r5/153: the method is defined at internal/wire/session.go:168 and called
// by NOTHING -- zero matches for `.Observe(` across both trees, tests included
// -- so R5.5's ruling (1), an `api` observation through the acting actor's
// session, had no coverage to extend. Their zeros are evidence rather than
// silence because they carried known positives beside them (`.Pick(` in three
// files, ObserveArgs seven calls in its own test).
//
// WHAT THIS DRIVES is the exchange and nothing above it: that an observe goes
// out on a KEPT session, that the answer comes back decoded as an
// ObserveResponse carrying the surface's value, that the id check holds across
// an observe as it does across an execute, and that a fault envelope is an
// ANSWER rather than a poison. Whether the walk composes the right request is
// internal/schedule's, and whether the toy answers it is the toy's.
func TestTheKeptSessionObserves(t *testing.T) {
	sess := open(t, fakeSpawn())

	// THE DEFAULT SURFACE IS THE ORACLE, not a number I chose: it records
	// `event` and `attendance`, so an observation of `event` must come back
	// with whatever that surface says and an unrecorded name must fault.
	resp, err := sess.Observe(wire.ObserveRequest{Query: "event", Source: wire.SourceDirect})
	if err != nil {
		t.Fatalf("observing `event` through direct: %v", err)
	}
	if resp.Value == nil {
		t.Errorf("the surface records `event`, so its value must come back; got nil")
	}
	first := resp.Value

	// AND THE SESSION IS KEPT: a second observe on the same process answers
	// again. Without this the arm above would pass against a client that
	// spawned one adapter per request, which is the property Session exists for.
	again, err := sess.Observe(wire.ObserveRequest{Query: "event", Source: wire.SourceDirect})
	if err != nil {
		t.Fatalf("second observe on the same session: %v", err)
	}
	if fmt.Sprint(again.Value) != fmt.Sprint(first) {
		t.Errorf("one surface answered %v then %v for the same query", first, again.Value)
	}

	// AN UNRECORDED QUERY IS A FAULT AND NOT A POISON, which is the rule at the
	// top of client.go: the session stays usable and the caller decides. Driven
	// because an observation the project declares and the adapter does not is
	// exactly what R5.5's walk will meet in the wild.
	if _, err := sess.Observe(wire.ObserveRequest{Query: "no-such-observation",
		Source: wire.SourceDirect}); err == nil {
		t.Error("an observation this surface does not record must come back a fault")
	}
	after, err := sess.Observe(wire.ObserveRequest{Query: "event", Source: wire.SourceDirect})
	if err != nil {
		t.Fatalf("the session was poisoned by a FAULT, which is an answer and not a poison: %v", err)
	}
	if after.Value == nil {
		t.Error("the session answered nil after a fault")
	}
	t.Logf("PROOF observe on a kept session: `event` -> %v twice, an unrecorded query faults, "+
		"and the session still answers %v after it", first, after.Value)
}
