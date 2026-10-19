package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/examples"
	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

// failWriter refuses every write. It is the whole reason run() takes io.Writer
// rather than *os.File: golangci-lint flagged three unchecked Fprintf returns
// in this file on run 143, and "checked" is only meaningful if something
// exercises the failure. errcheck can see that a return is ignored; it cannot
// see whether the handling is right.
type failWriter struct{ n int }

func (w *failWriter) Write(p []byte) (int, error) {
	w.n++
	return 0, errors.New("device full")
}

func TestCleanCheckPrintsTheCountAndExitsZero(t *testing.T) {
	orig := coverageCheck
	t.Cleanup(func() { coverageCheck = orig })
	coverageCheck = func() (int, error) { return 40, nil }

	var out, errb bytes.Buffer
	if rc := run([]string{"docs", "--check"}, bytes.NewReader(nil), &out, &errb); rc != 0 {
		t.Fatalf("a clean check must exit 0, got %d (stderr %q)", rc, errb.String())
	}
	if !strings.Contains(out.String(), "40 path(s), every one mapped") {
		t.Fatalf("the count line is what CI greps for; got %q", out.String())
	}
	if errb.Len() != 0 {
		t.Fatalf("a clean check must say nothing on stderr, got %q", errb.String())
	}
}

func TestAFailedCheckExitsOneAndNamesItOnStderr(t *testing.T) {
	orig := coverageCheck
	t.Cleanup(func() { coverageCheck = orig })
	coverageCheck = func() (int, error) { return 0, errors.New("stowaway.rs matches no entry") }

	var out, errb bytes.Buffer
	if rc := run([]string{"docs", "--check"}, bytes.NewReader(nil), &out, &errb); rc != 1 {
		t.Fatalf("a failed check must exit 1, got %d", rc)
	}
	if !strings.Contains(errb.String(), "stowaway.rs") {
		t.Fatalf("the failure must reach stderr by name; got %q", errb.String())
	}
	if out.Len() != 0 {
		t.Fatalf("a failed check must print nothing on stdout, got %q", out.String())
	}
}

// THE ARM THAT WOULD HAVE CAUGHT RUN 143'S FINDING. With the write unchecked
// this returned 0 -- reporting a clean check whose result never arrived, which
// is the same shape as a gate reading its formatter's exit status instead of
// its own.
func TestAFailedWriteDoesNotReportSuccess(t *testing.T) {
	orig := coverageCheck
	t.Cleanup(func() { coverageCheck = orig })
	coverageCheck = func() (int, error) { return 40, nil }

	w := &failWriter{}
	var errb bytes.Buffer
	rc := run([]string{"docs", "--check"}, bytes.NewReader(nil), w, &errb)
	if rc == 0 {
		t.Fatal("a check whose result could not be written must not exit 0")
	}
	if rc != 1 {
		t.Fatalf("expected exit 1, got %d", rc)
	}
	if w.n == 0 {
		t.Fatal("the write was never attempted, so this proves nothing about handling it")
	}
	if !strings.Contains(errb.String(), "could not be written") {
		t.Fatalf("the reason must reach stderr; got %q", errb.String())
	}
}

func TestUsageErrorExitsTwo(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"docs"},
		{"bogus"},
		{"docs", "--check", "extra"},
		{"--check", "docs"},
	} {
		var out, errb bytes.Buffer
		if rc := run(args, bytes.NewReader(nil), &out, &errb); rc != 2 {
			t.Errorf("args %q must exit 2, got %d", args, rc)
		}
		if !strings.Contains(errb.String(), usage) {
			t.Errorf("args %q must print the usage line to stderr; got %q", args, errb.String())
		}
	}
}

// `rumor check` (R2.3). The seam is examplesCheck, for the same reason
// coverageCheck is one: the argument surface, the three exit codes and the
// failed-write path are driven here without a tree of examples on disk, and the
// walk itself is tested in internal/examples against the real one.
func TestCheckReportsTheTallyAndExitsZero(t *testing.T) {
	saved := examplesCheck
	defer func() { examplesCheck = saved }()
	var gotDir string
	examplesCheck = func(dir string, out io.Writer) (examples.Result, error) {
		gotDir = dir
		_, _ = fmt.Fprintln(out, "planted-lie  honest-only  consistent  agrees")
		return examples.Result{Packages: 3, Cases: 17, Agreed: 17}, nil
	}
	var stdout, stderr bytes.Buffer
	if rc := run([]string{"check"}, bytes.NewReader(nil), &stdout, &stderr); rc != 0 {
		t.Fatalf("exit %d, wanted 0; stderr %q", rc, stderr.String())
	}
	if gotDir != defaultExamplesDir {
		t.Errorf("walked %q, wanted the default %q", gotDir, defaultExamplesDir)
	}
	if !strings.Contains(stdout.String(), "17 case(s) over 3 package(s) from ledger files, 17 agreed, 0 disagreed") {
		t.Errorf("the tally is not on stdout: %q", stdout.String())
	}
	// --examples replaces the directory, which is what lets this command be
	// pointed at another tree.
	gotDir = ""
	stdout.Reset()
	if rc := run([]string{"check", "--examples", "/elsewhere"}, bytes.NewReader(nil), &stdout, &stderr); rc != 0 {
		t.Fatalf("exit %d with --examples", rc)
	}
	if gotDir != "/elsewhere" {
		t.Errorf("walked %q, wanted /elsewhere", gotDir)
	}
}

// A DISAGREEMENT IS EXIT 1, AND SO IS EXAMINING NOTHING. Zero disagreements
// over zero cases is not a pass: it reads identically to agreement everywhere,
// which is the defect ibis main/352 named.
func TestCheckExitsOneOnADisagreementAndOnAnEmptyWalk(t *testing.T) {
	saved := examplesCheck
	defer func() { examplesCheck = saved }()
	for _, tc := range []struct {
		name string
		res  examples.Result
		err  error
		want int
	}{
		{"a case disagrees", examples.Result{Packages: 3, Cases: 17, Agreed: 16, Disagreed: 1}, nil, 1},
		{"nothing examined", examples.Result{}, nil, 1},
		{"the tree cannot be read", examples.Result{}, errors.New("no such tree"), 1},
		{"everything agrees", examples.Result{Packages: 3, Cases: 17, Agreed: 17}, nil, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			examplesCheck = func(string, io.Writer) (examples.Result, error) { return tc.res, tc.err }
			var stdout, stderr bytes.Buffer
			if rc := run([]string{"check"}, bytes.NewReader(nil), &stdout, &stderr); rc != tc.want {
				t.Fatalf("exit %d, wanted %d; stdout %q stderr %q", rc, tc.want, stdout.String(), stderr.String())
			}
			if tc.err != nil && !strings.Contains(stderr.String(), "no such tree") {
				t.Errorf("the failure is not named on stderr: %q", stderr.String())
			}
		})
	}
}

// The same arm as run 143's, for this subcommand: a tally that never arrived
// must not be reported as a pass.
func TestCheckDoesNotReportSuccessWhenItsTallyCannotBeWritten(t *testing.T) {
	saved := examplesCheck
	defer func() { examplesCheck = saved }()
	examplesCheck = func(string, io.Writer) (examples.Result, error) {
		return examples.Result{Packages: 3, Cases: 17, Agreed: 17}, nil
	}
	var stderr bytes.Buffer
	if rc := run([]string{"check"}, bytes.NewReader(nil), &failWriter{}, &stderr); rc != 1 {
		t.Fatalf("exit %d, wanted 1 when the tally could not be written", rc)
	}
	if !strings.Contains(stderr.String(), "could not be written") {
		t.Errorf("stderr does not say the result was lost: %q", stderr.String())
	}
}

func TestCheckRefusesAnUnknownFlag(t *testing.T) {
	for _, args := range [][]string{
		{"check", "--examples"},
		{"check", "--nonsense", "x"},
		{"check", "examples"},
		{"check", "--examples", "a", "b"},
	} {
		var stdout, stderr bytes.Buffer
		if rc := run(args, bytes.NewReader(nil), &stdout, &stderr); rc != 2 {
			t.Errorf("%v: exit %d, wanted 2 for a usage error", args, rc)
		}
		if !strings.Contains(stderr.String(), "rumor check") {
			t.Errorf("%v: the usage line does not mention check: %q", args, stderr.String())
		}
	}
}

// --- `rumor fake-adapter` (R4.1) ------------------------------------------

// TestFakeAdapterSpeaksTheProtocolOnStdout, and writes NOTHING else there: this
// subcommand's stdout is the wire, so the tally line every other subcommand
// prints would be a message the engine's decoder has to refuse.
func TestFakeAdapterSpeaksTheProtocolOnStdout(t *testing.T) {
	var stdout, stderr bytes.Buffer
	in := strings.NewReader(`{"kind":"describe","contract":"` + wire.ContractVersion + `","id":"1"}` + "\n")
	if rc := run([]string{"fake-adapter"}, in, &stdout, &stderr); rc != 0 {
		t.Fatalf("rc %d, stderr %q", rc, stderr.String())
	}
	lines := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("one request produced %d line(s) on stdout: %q", len(lines), stdout.String())
	}
	msg, err := wire.DecodeResponse([]byte(lines[0]))
	if err != nil {
		t.Fatalf("stdout is not on the protocol: %v", err)
	}
	if _, ok := msg.(wire.DescribeResponse); !ok {
		t.Errorf("answered %T", msg)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr %q: nothing to report on a clean run", stderr.String())
	}
}

// TestFakeAdapterFlagsAreRefusedByName, each with the reason on stderr and rc 2
// -- the usage code this binary has carried since R0.1.
func TestFakeAdapterFlagsAreRefusedByName(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"fake-adapter", "--nonesuch", "x"}, "unknown flag --nonesuch"},
		{[]string{"fake-adapter", "--omit", "ids"}, "wanted KIND.FIELD"},
		{[]string{"fake-adapter", "--omit", "execute."}, "wanted KIND.FIELD"},
		{[]string{"fake-adapter", "--surface"}, "takes a value"},
		{[]string{"fake-adapter", "--surface", "/nonesuch/surface.json"}, "--surface /nonesuch/surface.json"},
	} {
		var stdout, stderr bytes.Buffer
		rc := run(tc.args, bytes.NewReader(nil), &stdout, &stderr)
		if rc != 2 {
			t.Errorf("%v: rc %d, want 2", tc.args, rc)
		}
		if !strings.Contains(stderr.String(), tc.want) {
			t.Errorf("%v: stderr %q omits %q", tc.args, stderr.String(), tc.want)
		}
		if stdout.Len() != 0 {
			t.Errorf("%v: wrote %q to the wire while refusing its arguments", tc.args, stdout.String())
		}
	}
}

func TestFakeAdapterOmitAndContractReachTheConfig(t *testing.T) {
	var stdout, stderr bytes.Buffer
	in := strings.NewReader(`{"kind":"execute","contract":"` + wire.ContractVersion +
		`","id":"1","action":"create_event","args":{},"token":"t"}` + "\n")
	if rc := run([]string{"fake-adapter", "--omit", "execute.ids", "--contract", "2"}, in, &stdout, &stderr); rc != 0 {
		t.Fatalf("rc %d, stderr %q", rc, stderr.String())
	}
	line := strings.TrimSpace(stdout.String())
	if strings.Contains(line, `"ids"`) {
		t.Errorf("--omit did not reach the adapter: %s", line)
	}
	if !strings.Contains(line, `"contract":"2"`) {
		t.Errorf("--contract did not reach the adapter: %s", line)
	}
}

// TestFakeAdapterReportsAFailedWriteOnTheExitCode: the engine reads this
// process's status, so a write that never arrived cannot be a clean exit.
func TestFakeAdapterReportsAFailedWrite(t *testing.T) {
	var stderr bytes.Buffer
	in := strings.NewReader(`{"kind":"describe","contract":"` + wire.ContractVersion + `","id":"1"}` + "\n")
	if rc := run([]string{"fake-adapter"}, in, &failWriter{}, &stderr); rc != 1 {
		t.Errorf("rc %d, want 1", rc)
	}
	if !strings.Contains(stderr.String(), "fake-adapter:") {
		t.Errorf("stderr %q does not name the subcommand", stderr.String())
	}
}

// TestFakeAdapterUsageIsInTheUsageLine, because a subcommand nobody can
// discover is a subcommand the agent guide has to carry instead.
func TestFakeAdapterUsageIsInTheUsageLine(t *testing.T) {
	for _, want := range []string{"fake-adapter", "--surface", "--contract", "--fail", "--omit", "--id-salt"} {
		if !strings.Contains(usage, want) {
			t.Errorf("the usage line omits %q", want)
		}
	}
}
