package shrink

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("reading %s: %v", p, err)
	}
	return string(b)
}

// THE HEADER SURVIVES, which is the property an append has and a rewrite does
// not: a seeds file's whole content may be the explanation of why it is empty.
func TestPromoteAppendsAndKeepsTheHeader(t *testing.T) {
	p := filepath.Join(t.TempDir(), "seeds.lock")
	const header = "# no seed has found a failure yet\n"
	if err := os.WriteFile(p, []byte(header), 0o644); err != nil {
		t.Fatal(err)
	}
	added, err := Promote(p, 42, "overcounted after a late tell")
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if !added {
		t.Error("Promote reported the seed was not added")
	}
	got := readFile(t, p)
	if !strings.HasPrefix(got, header) {
		t.Errorf("the header was lost:\n%s", got)
	}
	if !strings.Contains(got, "42  # overcounted after a late tell\n") {
		t.Errorf("the promoted line is not in the format contract.md fixes:\n%s", got)
	}
}

// A FILE WITH NO TRAILING NEWLINE DOES NOT GET ITS LAST LINE CORRUPTED. Without
// the guard this writes `17  # first42`, which is one unreadable line where
// there should be two readable ones -- and arm 10b would refuse a file this
// function wrote.
func TestPromoteDoesNotJoinOntoALineWithNoNewline(t *testing.T) {
	p := filepath.Join(t.TempDir(), "seeds.lock")
	if err := os.WriteFile(p, []byte("17  # first"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Promote(p, 42, "second"); err != nil {
		t.Fatalf("Promote: %v", err)
	}
	got := readFile(t, p)
	if strings.Contains(got, "first42") {
		t.Errorf("the append joined onto the previous line:\n%q", got)
	}
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 2 {
		t.Errorf("%d line(s), want 2:\n%q", len(lines), got)
	}
}

// PROMOTING THE SAME SEED TWICE ADDS ONE LINE. A duplicate is readable, so this
// is tidiness rather than correctness -- but a run that promoted on every
// replay would grow the file without bound.
func TestPromoteIsIdempotent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "seeds.lock")
	if _, err := Promote(p, 7, "the first"); err != nil {
		t.Fatal(err)
	}
	added, err := Promote(p, 7, "a different reason")
	if err != nil {
		t.Fatalf("second Promote: %v", err)
	}
	if added {
		t.Error("the same seed was added twice")
	}
	if n := strings.Count(readFile(t, p), "\n"); n != 1 {
		t.Errorf("%d line(s) after promoting one seed twice", n)
	}

	// AND A SEED THAT ONLY LOOKS LIKE ONE ALREADY THERE IS STILL ADDED: 7 and
	// 70 are different seeds, so a prefix match would lose one.
	added, err = Promote(p, 70, "a different seed")
	if err != nil {
		t.Fatal(err)
	}
	if !added {
		t.Error("70 was treated as already present because 7 is")
	}
}

// A SEEDS FILE THAT IS NOT THERE IS CREATED, because arm 10a is what tells a
// project its file is missing and a real failure must not be lost to that.
func TestPromoteCreatesAMissingFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "seeds.lock")
	added, err := Promote(p, 3, "")
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if !added {
		t.Error("not added")
	}
	if got := readFile(t, p); got != "3\n" {
		t.Errorf("wrote %q, want a bare seed and a newline", got)
	}
}

func TestPromoteRefusals(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct {
		label string
		path  string
		why   string
		says  string
	}{
		{"no path", "", "x", "no seeds file named"},
		{"a newline in the comment", filepath.Join(dir, "a.lock"), "two\nlines", "holds a newline"},
		{"a carriage return", filepath.Join(dir, "b.lock"), "two\rlines", "holds a newline"},
	} {
		added, err := Promote(c.path, 1, c.why)
		if err == nil {
			t.Errorf("%s: accepted", c.label)
			continue
		}
		if !strings.Contains(err.Error(), c.says) {
			t.Errorf("%s: refusal must say %q; got %v", c.label, c.says, err)
		}
		if added {
			t.Errorf("%s: refused AND reported added", c.label)
		}
	}
}
