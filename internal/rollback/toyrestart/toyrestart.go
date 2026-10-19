// Package toyrestart is the toy's rollback provider: the one shipped provider
// of R5 (issue 73's C3), registered as "toy-process-restart" because that is
// the name examples/toy/platform.toml asks for.
//
// WHY IT OWNS THE PROCESS. A restart provider cannot restart a library, and it
// cannot restart a process somebody else started either -- so this package
// starts cmd/toy, holds the handle, and hands out the address it bound. The
// adapter then drives that one through TOY_ADDR, which is why cmd/toy-adapter
// reads that variable before starting a toy of its own: under the engine the
// provider owns the application, and `rumor validate` is the case where nobody
// does.
//
// WHY A RESTART AND NOT A TRUNCATE. The whole world is one file, so putting it
// back is a copy -- but the running process holds that world in memory as well,
// and a copy under a live server would leave the file and the server
// disagreeing. Restarting is what makes the file authoritative: the toy loads
// it at startup and holds nothing else.
//
// WHAT VERIFY PROVES, and it is two things rather than one:
//
//  1. THE FILE IS THE CHECKPOINT, byte for byte. Not "a restore was attempted"
//     and not "no error was returned" -- the bytes, because that is the only
//     statement that distinguishes a clean world from a hopeful one.
//  2. THE SERVER ANSWERS. A world nobody can reach is not a world a shrink step
//     can run in, and an unreachable toy would otherwise look clean: the file
//     on disk is perfect precisely when the process is dead.
//
// A verify that proved only the first would pass over a corpse.
package toyrestart

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"forge.axonibyte.com/axonibyte/rumor/internal/rollback"
)

// Name is the provider name examples/toy/platform.toml declares.
const Name = "toy-process-restart"

// The component this provider covers. examples/toy's `direct` observation names
// `store` and platform.toml's [[rollback]].covers has to cover it; validate's
// arm 6 is the pairing.
const component = "store"

// startTimeout bounds how long the toy may take to print its address. It is a
// BOUND and not a guess about speed: a toy that never prints is a provider that
// must say so rather than block a shrink forever.
const startTimeout = 20 * time.Second

func init() { rollback.Register(Name, open) }

type provider struct {
	mu    sync.Mutex
	dir   string // where the state file and the checkpoints live
	bin   string // the toy binary
	state string // the live world
	cmd   *exec.Cmd
	addr  string
	saved map[string]string // checkpoint id -> the file holding it
	seq   int
}

func open(o rollback.Options) (rollback.Provider, error) {
	dir := o.Dir
	if dir == "" {
		return nil, errors.New("toyrestart: no directory to work in: Options.Dir is the project's own")
	}
	work, err := os.MkdirTemp("", "toy-world-*")
	if err != nil {
		return nil, fmt.Errorf("toyrestart: making a world directory: %w", err)
	}
	// THE BINARY IS NAMED, NOT SEARCHED FOR BLINDLY. $TOY_BIN overrides for a
	// caller that built one somewhere; otherwise `toy` on PATH, which is how
	// ci.yml makes it available and how a project would ship it.
	bin := os.Getenv("TOY_BIN")
	if bin == "" {
		bin = "toy"
	}
	p := &provider{
		dir:   work,
		bin:   bin,
		state: filepath.Join(work, "state.json"),
		saved: map[string]string{},
	}
	if err := p.start(); err != nil {
		return nil, err
	}
	return p, nil
}

// Addr is the address the toy is listening on, for a caller that must tell an
// adapter where to find it (TOY_ADDR).
//
// NOT PART OF THE TRAIT, and deliberately: the five verbs are about putting a
// world back, and where a system under test listens is not one of them. R5.3
// needs it to spawn the adapter, so it is here and named rather than smuggled
// into Describe's prose.
func (p *provider) Addr() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.addr
}

// StateFile is the world, for a test that wants to dirty it. Exported for the
// same reason Addr is: a verify that cannot be shown failing has not been
// tested, and dirtying the world is how you show it.
func (p *provider) StateFile() string { return p.state }

func (p *provider) Describe() rollback.Description {
	return rollback.Description{
		Provider: Name,
		Covers:   []string{component},
		How: "stops cmd/toy, copies the saved state file back over the live one, and starts it " +
			"again, so the file is authoritative and nothing survives in memory",
	}
}

// start launches the toy and reads the address it bound. Called on open and on
// every restore.
func (p *provider) start() error {
	cmd := exec.Command(p.bin, "--addr", "127.0.0.1:0", "--state", p.state)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("toyrestart: piping %s: %w", p.bin, err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("toyrestart: starting %s (set TOY_BIN or put it on PATH): %w", p.bin, err)
	}
	// THE ADDRESS IS READ FROM THE PROCESS, bounded. The toy binds :0 and
	// prints what it got, so nothing here guesses a port -- two worlds on one
	// box would otherwise collide and the collision would read as a defect in
	// the application.
	type result struct {
		line string
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		line, err := bufio.NewReader(out).ReadString('\n')
		ch <- result{line, err}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return fmt.Errorf("toyrestart: reading the address %s printed: %w", p.bin, r.err)
		}
		addr := addressOf(r.line)
		if addr == "" {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return fmt.Errorf("toyrestart: %s printed %q, which names no address", p.bin, strings.TrimSpace(r.line))
		}
		p.cmd, p.addr = cmd, addr
		return nil
	case <-time.After(startTimeout):
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("toyrestart: %s printed no address in %s, so the world never opened", p.bin, startTimeout)
	}
}

func addressOf(line string) string {
	for _, field := range strings.Fields(line) {
		if strings.Contains(field, ":") && !strings.HasSuffix(field, ":") && !strings.Contains(field, "=") {
			return field
		}
	}
	return ""
}

func (p *provider) stop() {
	if p.cmd == nil || p.cmd.Process == nil {
		return
	}
	_ = p.cmd.Process.Kill()
	_ = p.cmd.Wait()
	p.cmd, p.addr = nil, ""
}

// Close stops the toy. Not part of the trait -- R13 may put a teardown in it --
// but a package that starts a process owes its caller a way to end it.
func (p *provider) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stop()
}

func (p *provider) Checkpoint(ctx context.Context) (rollback.Checkpoint, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return rollback.Checkpoint{}, err
	}
	p.seq++
	id := fmt.Sprintf("cp%d", p.seq)
	dst := filepath.Join(p.dir, id+".json")
	// AN ABSENT WORLD IS A WORLD, and the empty one: the toy treats a missing
	// state file as a fresh start, so a checkpoint taken before anything
	// happened has to be restorable to exactly that. Saving nothing and
	// recording it is how restore knows to remove the file rather than write
	// an empty one, which the toy would read as a decode failure.
	raw, err := os.ReadFile(p.state)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if err := os.WriteFile(dst, nil, 0o644); err != nil {
			return rollback.Checkpoint{}, fmt.Errorf("toyrestart: saving the empty world: %w", err)
		}
	case err != nil:
		return rollback.Checkpoint{}, fmt.Errorf("toyrestart: reading the world: %w", err)
	default:
		if err := os.WriteFile(dst, raw, 0o644); err != nil {
			return rollback.Checkpoint{}, fmt.Errorf("toyrestart: saving the world: %w", err)
		}
	}
	p.saved[id] = dst
	return rollback.Checkpoint{ID: id}, nil
}

func (p *provider) Restore(ctx context.Context, cp rollback.Checkpoint) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	src, ok := p.saved[cp.ID]
	if !ok {
		return fmt.Errorf("%w: %q", rollback.ErrUnknownCheckpoint, cp.ID)
	}
	raw, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("toyrestart: reading the saved world %s: %w", cp.ID, err)
	}
	// THE PROCESS GOES DOWN FIRST. Writing the file under a live server leaves
	// the two disagreeing: the toy holds the world in memory and would
	// overwrite this on its next save.
	p.stop()
	if len(raw) == 0 {
		if err := os.Remove(p.state); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("toyrestart: removing the world for an empty checkpoint: %w", err)
		}
	} else if err := os.WriteFile(p.state, raw, 0o644); err != nil {
		return fmt.Errorf("toyrestart: writing the world back: %w", err)
	}
	return p.start()
}

func (p *provider) Verify(ctx context.Context, cp rollback.Checkpoint) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	src, ok := p.saved[cp.ID]
	if !ok {
		return fmt.Errorf("%w: %q", rollback.ErrUnknownCheckpoint, cp.ID)
	}
	want, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("toyrestart: reading the saved world %s: %w", cp.ID, err)
	}
	got, err := os.ReadFile(p.state)
	if errors.Is(err, os.ErrNotExist) {
		got = nil
	} else if err != nil {
		return fmt.Errorf("toyrestart: reading the live world: %w", err)
	}
	if string(got) != string(want) {
		return fmt.Errorf("%w: the state file is %d byte(s) and the checkpoint %d",
			rollback.ErrDirty, len(got), len(want))
	}
	// AND THE SERVER ANSWERS. The bytes alone would pass over a dead toy --
	// the file is perfect precisely when nothing is running.
	if p.addr == "" {
		return fmt.Errorf("%w: the file matches and no toy is listening", rollback.ErrDirty)
	}
	if err := p.answers(ctx); err != nil {
		return fmt.Errorf("%w: the file matches and the toy did not answer: %v", rollback.ErrDirty, err)
	}
	return nil
}

// answers asks the toy one question whose STATUS does not matter. An
// unauthenticated list is refused with 401, which is still an answer -- what is
// being proven is that something is listening and speaking HTTP, not what it
// said.
func (p *provider) answers(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+p.addr+"/events", nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	return nil
}

func (p *provider) Discard(ctx context.Context, cp rollback.Checkpoint) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	src, ok := p.saved[cp.ID]
	if !ok {
		return fmt.Errorf("%w: %q", rollback.ErrUnknownCheckpoint, cp.ID)
	}
	delete(p.saved, cp.ID)
	if err := os.Remove(src); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("toyrestart: removing the saved world %s: %w", cp.ID, err)
	}
	return nil
}

// Reachable reports whether an address accepts a connection, for a test that
// wants to assert the toy really went down across a restore.
func Reachable(addr string) bool {
	if addr == "" {
		return false
	}
	c, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}
