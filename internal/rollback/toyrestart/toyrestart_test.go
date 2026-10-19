// Package toyrestart's test drives the provider against the real toy.
//
// IT BUILDS THE BINARY IT DRIVES, as tests/validate's TestMain does: a provider
// that restarts a process cannot be tested against a stub, and a hard-coded
// path to a binary nobody rebuilt is how this session's drill driver spent an
// afternoon reporting a staleness in its harness as eighteen failing arms.
package toyrestart

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"forge.axonibyte.com/axonibyte/rumor/internal/rollback"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "toyrestart-bin-*")
	if err != nil {
		panic(err)
	}
	bin := filepath.Join(dir, "toy")
	cmd := exec.Command("go", "build", "-o", bin, "../../../cmd/toy")
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	out, err := cmd.CombinedOutput()
	if err != nil {
		// A FAILED BUILD FAILS THE SUITE rather than skipping it: every test
		// here would otherwise pass over a provider that could start nothing.
		panic("building cmd/toy: " + err.Error() + "\n" + string(out))
	}
	if err := os.Setenv("TOY_BIN", bin); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func openProvider(t *testing.T) *provider {
	t.Helper()
	p, err := rollback.Open(Name, rollback.Options{Dir: t.TempDir()})
	if err != nil {
		t.Fatalf("opening %s: %v", Name, err)
	}
	tp, ok := p.(*provider)
	if !ok {
		t.Fatalf("Open returned %T, want *provider", p)
	}
	t.Cleanup(tp.Close)
	return tp
}

// touch makes the world move: it creates a user through the toy's own API, so
// the change is one the application made rather than one the test wrote into
// the file behind its back.
func touch(t *testing.T, p *provider, name string) {
	t.Helper()
	body := strings.NewReader(`{"name":"` + name + `"}`)
	req, err := http.NewRequest(http.MethodPost, "http://"+p.Addr()+"/users", body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("creating the user %s: %v", name, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("creating the user %s: status %d", name, resp.StatusCode)
	}
}

func TestTheProviderIsRegisteredUnderTheNameThePlatformAsks(t *testing.T) {
	found := false
	for _, n := range rollback.Registered() {
		if n == Name {
			found = true
		}
	}
	if !found {
		t.Fatalf("%q is not registered; registered: %v", Name, rollback.Registered())
	}
	// examples/toy/platform.toml names this provider, and a mismatch between
	// that file and this constant would surface only as a run that could not
	// open its rollback -- long after the project parsed.
	raw, err := os.ReadFile("../../../examples/toy/platform.toml")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(`provider = "`+Name+`"`)) {
		t.Errorf("examples/toy/platform.toml does not name %q", Name)
	}
}

func TestAnUnknownProviderNamesWhatIsRegistered(t *testing.T) {
	_, err := rollback.Open("no-such-provider", rollback.Options{Dir: t.TempDir()})
	if err == nil {
		t.Fatal("an unknown provider opened")
	}
	if !strings.Contains(err.Error(), Name) {
		t.Errorf("the error does not name what IS registered, which is where the typo shows: %v", err)
	}
}

func TestOpeningWithNoDirectoryIsRefused(t *testing.T) {
	if _, err := rollback.Open(Name, rollback.Options{}); err == nil {
		t.Error("the provider opened with no directory to work in")
	}
}

func TestDescribeNamesTheComponentThePlatformCovers(t *testing.T) {
	p := openProvider(t)
	d := p.Describe()
	if d.Provider != Name {
		t.Errorf("Describe says %q", d.Provider)
	}
	if len(d.Covers) != 1 || d.Covers[0] != component {
		t.Errorf("Covers is %v, want [%s]: validate's arm 6 compares this against "+
			"platform.toml's [[rollback]].covers and the adapter's components", d.Covers, component)
	}
	if d.How == "" {
		t.Error("How is empty: a verdict that says restored without saying by what mechanism is hard to disbelieve")
	}
}

// THE WHOLE LOOP, and the address is part of the proof: a restore that did not
// restart would hand back the same port.
func TestCheckpointRestoreVerifyPutsTheWorldBack(t *testing.T) {
	p := openProvider(t)
	ctx := context.Background()
	before := p.Addr()
	if !Reachable(before) {
		t.Fatalf("the toy is not listening on %s after open", before)
	}

	touch(t, p, "alice")
	cp, err := p.Checkpoint(ctx)
	if err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	if err := p.Verify(ctx, cp); err != nil {
		t.Fatalf("verify straight after checkpoint: %v", err)
	}
	saved, err := os.ReadFile(p.StateFile())
	if err != nil {
		t.Fatal(err)
	}

	touch(t, p, "bob")
	if err := p.Verify(ctx, cp); !errors.Is(err, rollback.ErrDirty) {
		t.Fatalf("verify over a moved world: %v, want ErrDirty", err)
	}

	if err := p.Restore(ctx, cp); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if err := p.Verify(ctx, cp); err != nil {
		t.Fatalf("verify after restore: %v", err)
	}
	now, err := os.ReadFile(p.StateFile())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(now, saved) {
		t.Errorf("the world after restore is not the world saved")
	}
	if after := p.Addr(); after == before {
		t.Errorf("the address is still %s, so nothing restarted: this provider's whole "+
			"mechanism is that the process goes down and the file becomes authoritative", after)
	}
	if !Reachable(p.Addr()) {
		t.Errorf("nothing is listening on %s after the restore", p.Addr())
	}
	if Reachable(before) {
		t.Errorf("the old toy is still listening on %s", before)
	}

	// AND THE RESTORED WORLD IS THE APPLICATION'S, not just the file's: bob is
	// gone as far as the running server is concerned, which is what a shrink
	// step needs and what a file copy under a live process would not give.
	req, _ := http.NewRequest(http.MethodPost, "http://"+p.Addr()+"/users", strings.NewReader(`{"name":"bob"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("re-creating bob: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("re-creating bob after the restore: status %d, want 201 -- the restored server "+
			"still believes bob exists, so the restore did not reach the application", resp.StatusCode)
	}
}

// THE CORPSE CASE, which is the reason Verify asks two questions. The file is
// byte-perfect precisely because nothing is running, and a verify that compared
// only bytes would call that clean.
func TestVerifyRefusesAWorldWhoseFileIsPerfectAndWhoseServerIsDead(t *testing.T) {
	p := openProvider(t)
	ctx := context.Background()
	touch(t, p, "alice")
	cp, err := p.Checkpoint(ctx)
	if err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	if err := p.Verify(ctx, cp); err != nil {
		t.Fatalf("verify on a live clean world: %v", err)
	}

	addr := p.Addr()
	p.Close() // the file is untouched; only the process is gone
	if Reachable(addr) {
		t.Fatalf("the toy is still listening on %s after Close", addr)
	}
	err = p.Verify(ctx, cp)
	if !errors.Is(err, rollback.ErrDirty) {
		t.Fatalf("verify over a dead toy: %v, want ErrDirty", err)
	}
	if !strings.Contains(err.Error(), "the file matches") {
		t.Errorf("the refusal does not say which half held: %v", err)
	}
}

// A DIFFERENCE OF THE SAME LENGTH IS STILL A DIFFERENCE, and this arm exists
// because the battery could not tell a byte comparison from a length one:
// driven at 2187e7a, replacing `string(got) != string(want)` with
// `len(got) != len(want)` left every test in this package passing. The reason
// is that the only moved world any arm built was one with an EXTRA user, which
// changes the length as a side effect -- so the arm that pins Verify's
// comparison was true for a reason unrelated to what it asserts.
//
// THE EDIT GOES BEHIND THE APPLICATION'S BACK, deliberately, which is the one
// place in this file that is right: `touch` moves the world the way the
// application would because the subject there is the world. The subject here is
// Verify's COMPARISON, and an equal-length change is not something the toy's
// API can be asked for.
func TestVerifyIsDirtyOnADifferenceOfTheSameLength(t *testing.T) {
	p := openProvider(t)
	ctx := context.Background()
	touch(t, p, "alice")
	cp, err := p.Checkpoint(ctx)
	if err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	if err := p.Verify(ctx, cp); err != nil {
		t.Fatalf("verify on a clean world: %v", err)
	}

	before, err := os.ReadFile(p.StateFile())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(before, []byte("alice")) {
		t.Fatalf("the world does not hold the name this arm edits: %s", before)
	}
	after := bytes.Replace(before, []byte("alice"), []byte("alicz"), 1)
	if len(after) != len(before) {
		t.Fatalf("the edit changed the length, %d to %d, which is the case the "+
			"other arms already cover", len(before), len(after))
	}
	if bytes.Equal(after, before) {
		t.Fatal("the edit changed nothing")
	}
	if err := os.WriteFile(p.StateFile(), after, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := p.Verify(ctx, cp); !errors.Is(err, rollback.ErrDirty) {
		t.Fatalf("verify over a world that differs by one byte: %v, want ErrDirty", err)
	}
}

// A WORLD THAT IS GONE IS DIRTY, not clean, and nothing was asserting it: the
// absent-file branch reads as an empty world, and with `got = want` substituted
// there -- driven at 2187e7a -- the whole package stayed green. The branch is
// reachable in the one case a shrink step cares about most, a restore that
// removed the file and a checkpoint that was not empty.
func TestVerifyIsDirtyWhenTheWorldIsGoneAndTheCheckpointIsNot(t *testing.T) {
	p := openProvider(t)
	ctx := context.Background()
	touch(t, p, "alice")
	cp, err := p.Checkpoint(ctx)
	if err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	saved, err := os.ReadFile(p.StateFile())
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) == 0 {
		t.Fatal("the checkpoint is empty, so this arm is not the case it names")
	}
	if err := os.Remove(p.StateFile()); err != nil {
		t.Fatal(err)
	}
	if err := p.Verify(ctx, cp); !errors.Is(err, rollback.ErrDirty) {
		t.Fatalf("verify with the world removed: %v, want ErrDirty", err)
	}
}

// AN EMPTY WORLD IS A WORLD. A checkpoint taken before anything happened has to
// restore to an ABSENT file, not an empty one: the toy reads an empty file as a
// fresh start but a zero-byte file written where JSON is expected is the kind of
// thing a strict decoder is right to refuse, so the provider removes it instead.
func TestAnEmptyWorldRestoresToNoFileAtAll(t *testing.T) {
	p := openProvider(t)
	ctx := context.Background()
	cp, err := p.Checkpoint(ctx)
	if err != nil {
		t.Fatalf("checkpoint of an empty world: %v", err)
	}
	if _, err := os.Stat(p.StateFile()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the world already exists before anything happened: %v", err)
	}

	touch(t, p, "alice")
	if _, err := os.Stat(p.StateFile()); err != nil {
		t.Fatalf("the world was not written: %v", err)
	}
	if err := p.Verify(ctx, cp); !errors.Is(err, rollback.ErrDirty) {
		t.Fatalf("verify after the world moved: %v, want ErrDirty", err)
	}

	if err := p.Restore(ctx, cp); err != nil {
		t.Fatalf("restore to the empty world: %v", err)
	}
	if _, err := os.Stat(p.StateFile()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the state file exists after restoring an empty checkpoint, and the toy would "+
			"have to decode it: %v", err)
	}
	if err := p.Verify(ctx, cp); err != nil {
		t.Errorf("verify after restoring the empty world: %v", err)
	}
	// And the restored toy accepts alice again, which it would refuse if the
	// old world had survived.
	touch(t, p, "alice")
}

func TestDiscardForgetsTheCheckpointAndSaysSoAfterwards(t *testing.T) {
	p := openProvider(t)
	ctx := context.Background()
	touch(t, p, "alice")
	cp, err := p.Checkpoint(ctx)
	if err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	if err := p.Discard(ctx, cp); err != nil {
		t.Fatalf("discard: %v", err)
	}
	for _, c := range []struct {
		what string
		err  error
	}{
		{"verify", p.Verify(ctx, cp)},
		{"restore", p.Restore(ctx, cp)},
		{"discard twice", p.Discard(ctx, cp)},
	} {
		if !errors.Is(c.err, rollback.ErrUnknownCheckpoint) {
			t.Errorf("%s after discard: %v, want ErrUnknownCheckpoint", c.what, c.err)
		}
	}
}

func TestAnUnknownCheckpointIsNotDirty(t *testing.T) {
	p := openProvider(t)
	ctx := context.Background()
	err := p.Verify(ctx, rollback.Checkpoint{ID: "cp-nobody-issued"})
	if errors.Is(err, rollback.ErrDirty) {
		t.Error("a handle this provider never issued came back as a DIRTY world, which is a " +
			"statement about the world rather than about the caller")
	}
	if !errors.Is(err, rollback.ErrUnknownCheckpoint) {
		t.Errorf("verify on an unknown checkpoint: %v", err)
	}
}

// A CANCELLED CONTEXT STOPS EVERY VERB, because a shrink that cannot be bounded
// is a shrink that stops the run (docs/design.md: every candidate runs against a
// verified clean rollback, and a hung restore would hold the whole bisection).
func TestACancelledContextRefusesEveryVerb(t *testing.T) {
	p := openProvider(t)
	live := context.Background()
	touch(t, p, "alice")
	cp, err := p.Checkpoint(live)
	if err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.Checkpoint(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("checkpoint under a cancelled context: %v", err)
	}
	for _, c := range []struct {
		what string
		err  error
	}{
		{"restore", p.Restore(ctx, cp)},
		{"verify", p.Verify(ctx, cp)},
		{"discard", p.Discard(ctx, cp)},
	} {
		if !errors.Is(c.err, context.Canceled) {
			t.Errorf("%s under a cancelled context: %v", c.what, c.err)
		}
	}
}

func TestTwoCheckpointsAreIndependent(t *testing.T) {
	p := openProvider(t)
	ctx := context.Background()
	touch(t, p, "alice")
	first, err := p.Checkpoint(ctx)
	if err != nil {
		t.Fatal(err)
	}
	touch(t, p, "bob")
	second, err := p.Checkpoint(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID {
		t.Fatalf("both checkpoints are %q", first.ID)
	}
	if err := p.Verify(ctx, second); err != nil {
		t.Errorf("verify the newer checkpoint: %v", err)
	}
	if err := p.Verify(ctx, first); !errors.Is(err, rollback.ErrDirty) {
		t.Errorf("verify the older checkpoint over the newer world: %v, want ErrDirty", err)
	}
	if err := p.Restore(ctx, first); err != nil {
		t.Fatalf("restore the older: %v", err)
	}
	if err := p.Verify(ctx, first); err != nil {
		t.Errorf("verify the older after restoring it: %v", err)
	}
	// And the newer is now the dirty one, which is what makes these two
	// independent rather than a stack.
	if err := p.Verify(ctx, second); !errors.Is(err, rollback.ErrDirty) {
		t.Errorf("verify the newer after restoring the older: %v, want ErrDirty", err)
	}
}
