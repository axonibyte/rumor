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

// Describe spawns the adapter, asks it to describe itself, reads one line, and
// stops it.
//
// ONE REQUEST AND ONE LINE, which is the protocol's own rule (one outstanding
// request per process) applied to the only exchange this needs. The process is
// killed rather than waited for: an adapter that keeps the connection open for
// a run it is not going to get would otherwise hold this call to the timeout.
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
		return zero, fmt.Errorf("opening the adapter's stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return zero, fmt.Errorf("opening the adapter's stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return zero, fmt.Errorf("spawning %q: %w", filepath.Base(s.Command[0]), err)
	}
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

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

	var resp DescribeResponse
	if derr := json.Unmarshal(raw, &resp); derr != nil {
		return zero, fmt.Errorf("the adapter's answer is not a describe response: %w", derr)
	}
	if resp.Kind != "describe" {
		return zero, fmt.Errorf("the adapter answered %q where a describe was asked", resp.Kind)
	}
	return resp, nil
}
