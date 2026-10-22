// The adapter client.
//
// WHY IT LIVES HERE AND NOT IN internal/validate, ruled at coop r4/74: the
// epic's capability and contract-version bullets can only be answered by ASKING
// the adapter, so validate owns the first spawn in this project -- but as ONE
// client that R5's engine reuses, not a validate-only spawner. Two spawners
// would drift, and the second would be written by whoever needed it next
// without the reasons the first had.
//
// docs/contract.md's "The engine spawns the adapter" is amended to say the WIRE
// CLIENT spawns it, for the engine and for validate, with that reason.
//
// EVERY FAILURE TO ANSWER IS could-not-run AND NEVER A REFUSAL. The project is
// not wrong when its adapter will not start, will not speak, or will not stop
// -- "the project is not wrong; its adapter is" (docs/contract.md:57), which is
// also why the decode refusals this file can produce are exit 2 over there.
package wire

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// DefaultDescribeTimeout bounds one describe. It is a CEILING on a handshake
// and not a performance budget: an adapter that has not answered a describe in
// this long is not slow, it is not answering.
const DefaultDescribeTimeout = 20 * time.Second

// Spawn is how to reach one adapter: the argv, and the two optional parts of
// that spawn. It mirrors platform.toml's [adapter] rather than importing it,
// because internal/platform reads a document and this package speaks a
// protocol -- and a dependency from here to there would make the protocol
// depend on the file format that happens to carry it.
type Spawn struct {
	Command []string
	Cwd     string
	Env     []string // variable NAMES passed through, so secrets stay out of the file
}

// start spawns one adapter and hands back the process and its two pipes.
//
// WHY IT IS SHARED BY Describe AND Session: the comment at the top of this file
// says two spawners would drift and the second would be written by whoever
// needed it next without the reasons the first had -- and R5's session IS that
// second caller. So the cwd rule, the named-variables environment and the PATH
// exception live here once. Everything ABOVE the spawn differs between the two
// (a probe closes stdin and kills; a run keeps both), which is why only this
// much is shared and not the exchange.
func (s Spawn) start(ctx context.Context) (*exec.Cmd, io.WriteCloser, io.ReadCloser, error) {
	cmd := exec.CommandContext(ctx, s.Command[0], s.Command[1:]...)
	if s.Cwd != "" {
		cmd.Dir = s.Cwd
	}
	// ENV IS A PASS-THROUGH LIST OF NAMES, so this builds the child's
	// environment from the named variables only. An adapter needing BASE_URL
	// says so in platform.toml; it does not inherit whatever the operator had.
	env := []string{}
	for _, name := range s.Env {
		if v, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+v)
		}
	}
	// PATH comes too, or `command = ["rumor", ...]` could not be found at all.
	if v, ok := os.LookupEnv("PATH"); ok {
		env = append(env, "PATH="+v)
	}
	cmd.Env = env

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("opening the adapter's stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("opening the adapter's stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, nil, fmt.Errorf("spawning %q: %w", filepath.Base(s.Command[0]), err)
	}
	return cmd, stdin, stdout, nil
}

// StopGrace bounds how long a caller waits for an adapter to exit on its own
// after its stdin closes, before killing it.
//
// SECONDS AND NOT MILLISECONDS, AND NOT A CONFIGURABLE EITHER. EOF on stdin is
// the adapter's documented stop signal, and what a well-behaved one does next
// is its own teardown -- cmd/toy-adapter kills one child and waits for it,
// which is microseconds. So this is orders of magnitude over what the honest
// case needs, which is the point: the bound exists for the DISHONEST case, and
// one that a slow-but-correct adapter could trip would turn a grace period into
// a second race. It stays far under any CI step's own bound.
const StopGrace = 2 * time.Second

// stopAdapter closes the adapter's stdin, gives it StopGrace to exit on its
// own, and kills it only if the grace elapses.
//
// WHY THE GRACE IS NOT OPTIONAL, measured by fable at coop r5/100 and
// reproduced here 2026-10-22. Both call sites closed stdin and killed AT ONCE.
// An adapter's cleanup is its own `defer`, and a defer does not run under
// SIGKILL -- so whether cmd/toy-adapter reaped the toy it spawned was a race
// between its read of EOF and the kill arriving. On this box `rumor run` hung
// to a 15s bound in 4 of 8 runs and `rumor validate` in 2 of 3, each having
// already printed its complete result: the orphaned toy outlives the run, and
// timeout(1) in front -- which is what ci.yml wraps every step in -- waits for
// the orphan it inherited. A run that did its work and then hangs reads as a
// runner fault, which is the worst form of this.
//
// THE KILL IS KEPT, which is what makes this a grace and not a wait. Describe's
// comment gives the reason and it still holds: "an adapter that keeps the
// connection open for a run it is not going to get would otherwise hold this
// call to the timeout". An adapter that ignores EOF meets the kill exactly as
// before, two seconds later.
//
// ONE FUNCTION FOR BOTH CALL SITES. Describe and Session.Close want the same
// three steps in the same order, and a second copy would be free to drift in
// the bound or the order -- which is the whole defect here, in a smaller form.
//
// THE CALLER'S CONTEXT STILL BOUNDS IT. Describe's context carries a deadline,
// so the effective grace there is whatever of StopGrace fits inside what the
// describe exchange left: exec.CommandContext kills on expiry either way, and
// nothing here can outlive the bound the caller already set.
//
// `stdin` MAY ALREADY BE CLOSED and that is not an error worth reporting:
// Describe closes it after writing its one request, so this is a second Close
// on an os.File, which answers ErrClosed and changes nothing.
func stopAdapter(cmd *exec.Cmd, stdin io.Closer) {
	if stdin != nil {
		_ = stdin.Close()
	}
	if cmd == nil || cmd.Process == nil {
		return
	}
	reaped := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(reaped)
	}()
	select {
	case <-reaped:
	case <-time.After(StopGrace):
		_ = cmd.Process.Kill()
		<-reaped
	}
}

// Describe spawns the adapter, asks it to describe itself, reads one line, and
// stops it.
//
// ONE REQUEST AND ONE LINE, which is the protocol's own rule (one outstanding
// request per process) applied to the only exchange this needs. The process is
// stopped through stopAdapter: EOF first, a bounded grace for the adapter's own
// teardown, then the kill -- so an adapter that keeps the connection open for a
// run it is not going to get still cannot hold this call to the timeout, and
// one that cleans up after itself is now given the chance to.
func Describe(ctx context.Context, s Spawn, timeout time.Duration) (DescribeResponse, error) {
	var zero DescribeResponse
	if len(s.Command) == 0 {
		return zero, fmt.Errorf("the platform declares no [adapter].command, so there is nothing to ask")
	}
	if timeout <= 0 {
		timeout = DefaultDescribeTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd, stdin, stdout, err := s.start(ctx)
	if err != nil {
		return zero, err
	}
	defer func() { stopAdapter(cmd, stdin) }()

	// THE EMBEDDED ENVELOPE IS NAMED, not set through promoted fields: a
	// promoted-field struct literal needs go1.27 and go.mod's language floor is
	// 1.26, which v0.1.18 settled deliberately as OPA's minimum rather than an
	// equality. Raising the floor to save three words here would spend that.
	req := DescribeRequest{Envelope: Envelope{
		Kind:     KindDescribe,
		Contract: ContractVersion,
		ID:       "validate-describe",
	}}
	line, err := json.Marshal(req)
	if err != nil {
		return zero, fmt.Errorf("encoding the describe request: %w", err)
	}
	if _, err := stdin.Write(append(line, '\n')); err != nil {
		return zero, fmt.Errorf("writing describe to the adapter: %w", err)
	}
	_ = stdin.Close()

	br := bufio.NewReader(stdout)
	raw, err := br.ReadBytes('\n')
	if err != nil && len(raw) == 0 {
		// EOF WITH NOTHING READ IS THE ADAPTER DECLINING TO SPEAK, and
		// docs/contract.md is explicit that EOF before the run ends is
		// could-not-run with the status in the detail.
		return zero, fmt.Errorf("the adapter answered nothing before closing: %w", err)
	}

	// THE STRICT DECODER, NOT json.Unmarshal, and driving fable's D7 drill is
	// what found the difference. A plain unmarshal accepts a describe with
	// `contract` missing, leaves the field empty, and the contract-version arm
	// then REFUSES the project for a value its adapter never sent -- exit 1
	// blaming the project for its adapter's malformed message. The decoder
	// refuses the message by name instead, and this returns an error, which the
	// caller reports as could-not-run: "the project is not wrong; its adapter
	// is" (docs/contract.md:57). D7 asserts exactly that boundary.
	msg, derr := DecodeResponse(raw)
	if derr != nil {
		return zero, fmt.Errorf("the adapter's describe is not a well-formed response: %w", derr)
	}
	resp, ok := msg.(DescribeResponse)
	if !ok {
		return zero, fmt.Errorf("the adapter answered %q where a describe was asked", msg.Env().Kind)
	}
	return resp, nil
}
