package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
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
	if rc := run([]string{"docs", "--check"}, &out, &errb); rc != 0 {
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
	if rc := run([]string{"docs", "--check"}, &out, &errb); rc != 1 {
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
	rc := run([]string{"docs", "--check"}, w, &errb)
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
		if rc := run(args, &out, &errb); rc != 2 {
			t.Errorf("args %q must exit 2, got %d", args, rc)
		}
		if !strings.Contains(errb.String(), usage) {
			t.Errorf("args %q must print the usage line to stderr; got %q", args, errb.String())
		}
	}
}
