// The promoter's output, read by the ARM that reads seeds files.
//
// WHY THIS SUITE EXISTS SEPARATELY. internal/shrink holds its own reading of
// docs/contract.md's seed grammar, used to avoid promoting a seed twice, and a
// second reading of one format is exactly the pair that drifts. So the
// authority is internal/validate's arm 10b and this drives it: a file
// shrink.Promote wrote, handed to validate.Run, must produce no refusal. A test
// that re-checked the format by eye would be a third reading.
package shrink_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/shrink"
	"forge.axonibyte.com/axonibyte/rumor/internal/validate"
)

// twinCopy is a complete, valid project to promote into: examples/twin, whose
// own seeds.lock is a header and nothing else.
func twinCopy(t *testing.T) string {
	t.Helper()
	src := filepath.Join("..", "..", "examples", "twin")
	dst := t.TempDir()
	// THE COPY IS RECURSIVE, AND MY FIRST ONE WAS NOT. Skipping directories
	// left out cases/, so validate refused with "no-case" and SKIPPED every
	// arm -- including 10b. The promotion arm then read zero refusals and
	// passed, on an oracle that had never run. The vacuity control below is
	// what caught it.
	copied := 0
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(src, path)
		if rerr != nil {
			return rerr
		}
		if d.IsDir() {
			if rel == "." {
				return nil
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		copied++
		return os.WriteFile(filepath.Join(dst, rel), b, 0o644)
	})
	if err != nil {
		t.Fatalf("copying the twin: %v", err)
	}
	if copied == 0 {
		t.Fatal("copied no files from examples/twin, so there is no project to promote into")
	}
	if _, err := os.Stat(filepath.Join(dst, "seeds.lock")); err != nil {
		t.Fatalf("the copy has no seeds.lock, which is the file under test: %v", err)
	}
	return dst
}

// refusalsFor returns arm 10b's refusals, and REFUSES TO ANSWER unless 10b
// actually ran.
//
// "NO REFUSAL" IS NOT "PASSED" UNLESS THE ARM RAN, which this suite learned the
// expensive way: a non-recursive copy left out cases/, validate skipped all
// nineteen arms behind a no-case refusal, and the promotion arm read an empty
// refusal list as success. Every later call is therefore gated on 10b being in
// Evaluated, so the oracle cannot go quiet without saying so.
func refusalsFor(t *testing.T, dir string) []string {
	t.Helper()
	rep := validate.Run(dir)
	ran := false
	for _, a := range rep.Evaluated {
		if a == "10b" {
			ran = true
		}
	}
	if !ran {
		t.Fatalf("arm 10b did not run, so its silence is not a verdict: evaluated %v, skipped %v, "+
			"refusals %v", rep.Evaluated, rep.Skipped, rep.Refusals)
	}
	var out []string
	for _, r := range rep.Refusals {
		// THE ARM IS A FIELD, not a substring of the detail: matching the text
		// would also catch a refusal that merely MENTIONS the seeds file, and
		// this arm is about 10b alone.
		if r.Arm == "10b" {
			out = append(out, r.String())
		}
	}
	return out
}

func TestAPromotedSeedIsReadableByTheArmThatReadsSeeds(t *testing.T) {
	dir := twinCopy(t)
	path := filepath.Join(dir, "seeds.lock")

	// THE CONTROL FIRST: the untouched twin must not already be refusing on
	// 10b, or a clean result after the promotion would prove nothing.
	if bad := refusalsFor(t, dir); len(bad) != 0 {
		t.Fatalf("the twin refuses on its seeds file before any promotion: %v", bad)
	}

	for _, c := range []struct {
		seed uint64
		why  string
	}{
		{42, "overcounted after a late tell"},
		{0, "zero is a seed"},
		{18446744073709551615, "the largest uint64, which must still be a non-negative integer"},
		{7, ""},
		// A COMMENT HOLDING A `#` IS STILL ONE COMMENT, because the arm takes
		// the text before the FIRST hash.
		{99, "the rule # fired twice"},
	} {
		added, err := shrink.Promote(path, c.seed, c.why)
		if err != nil {
			t.Fatalf("promoting %d: %v", c.seed, err)
		}
		if !added {
			t.Errorf("%d was not added", c.seed)
		}
		if bad := refusalsFor(t, dir); len(bad) != 0 {
			t.Fatalf("after promoting %d the arm refuses: %v\nfile:\n%s", c.seed, bad,
				mustRead(t, path))
		}
	}

	t.Logf("PROOF arm 10b read %d promoted seed(s) with no refusal; file:\n%s", 5, mustRead(t, path))
}

// AND THE ARM REFUSES WHAT THE PROMOTER WOULD NEVER WRITE, which is the
// vacuity control: without it, a 10b that refused nothing at all would make
// every arm above pass.
func TestTheArmStillRefusesALineThatIsNotASeed(t *testing.T) {
	dir := twinCopy(t)
	path := filepath.Join(dir, "seeds.lock")
	if _, err := shrink.Promote(path, 5, "a real promotion"); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("not-a-seed\n"); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	bad := refusalsFor(t, dir)
	if len(bad) == 0 {
		t.Fatal("arm 10b accepted a line that is not a seed, so it cannot be the oracle for the " +
			"promoter's output")
	}
	if !strings.Contains(strings.Join(bad, " "), "not-a-seed") {
		t.Errorf("the refusal should name the line; got %v", bad)
	}
}

func mustRead(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("reading %s: %v", p, err)
	}
	return string(b)
}
