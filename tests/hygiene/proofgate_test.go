package hygiene

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/proof"
)

// A PROOF LINE IS A CLAIM THAT A CHECK PASSED, so a claim that can print from a
// FAILING test is not a proof -- which is 124's deliverable 6 and fable's
// ruling at coop r9/251. internal/proof.Logf is the one writer that will not
// log from a failed test, and this arm is what keeps the 221 sites on it.
//
// WHY AN ARM AND NOT A CONVENTION: the original defect was a convention. The
// batteries step's comment said for months that a battery which never runs can
// rot, and the one battery left out of its list is the one that rotted. A bare
// `t.Logf("PROOF` is the same shape -- correct-looking, uncaught, and silent.
//
// THE SCAN IS OVER TRACKED FILES, not a walk of the working tree: an untracked
// scratch file is not this project's code, and a walk would make a stray copy
// in anyone's checkout fail everybody's suite.
func TestEveryProofLineGoesThroughTheHelper(t *testing.T) {
	root := filepath.Join("..", "..")
	out, err := exec.Command("git", "-C", root, "ls-files", "*_test.go").Output()
	if err != nil {
		t.Fatalf("listing tracked test files: %v", err)
	}
	files := strings.Fields(string(out))
	if len(files) < 50 {
		t.Fatalf("git ls-files returned %d test file(s), too few for this scan to mean "+
			"anything: the arm would pass by looking at nothing", len(files))
	}

	// THE NEEDLE IS BUILT AT RUNTIME SO THE SCAN CANNOT MATCH ITS OWN SOURCE.
	// This file names the bare form in its prose and in its own cells, and a
	// literal here would make the arm red on itself -- the shape ibis already
	// hit once in internal/browser's source guards.
	bare := "t.Logf(" + `"` + "PROOF"
	helper := "proof.Logf(t, " + `"` + "PROOF"

	self := filepath.ToSlash(filepath.Join("tests", "hygiene", "proofgate_test.go"))

	sites, bad, sawSelf := 0, 0, false
	for _, f := range files {
		b, rerr := os.ReadFile(filepath.Join(root, f))
		if rerr != nil {
			t.Fatalf("reading %s: %v", f, rerr)
		}
		body := string(b)
		sites += strings.Count(body, helper)
		if f == self {
			sawSelf = true
			continue
		}
		for n, line := range strings.Split(body, "\n") {
			if strings.Contains(line, bare) {
				bad++
				t.Errorf("%s:%d writes a PROOF line with a bare t.Logf: use "+
					"proof.Logf(t, ...), which does not log from a failing test", f, n+1)
			}
		}
	}

	// AND THE ARM HAS TO SEE ITSELF, because the false green this check now
	// refuses is the one it shipped with. `ls-files` lists TRACKED files, so
	// while this file was still untracked the scan did not include it, counted
	// 221, and PASSED -- and `git add` alone turned the identical source red at
	// 222 (rook, coop r9/253). An arm whose verdict depends on whether it is
	// staged yet is measuring the index rather than the tree, and it passes in
	// precisely the state where it has never read itself.
	if !sawSelf {
		t.Fatalf("%s is not tracked, so this scan did not include itself and its count is "+
			"of a tree that does not contain this arm: a pass here would mean nothing", self)
	}

	// AND A COUNT, because every case above is negative: a tree that lost its
	// PROOF lines entirely would satisfy all of them. The figure is a sum a
	// reader can take apart: 221 sites 124's d6 moved onto the helper, this
	// arm's own closing line, and one PROOF added since -- R9.2's findings-count
	// arm in tests/vocabulary and R9.2's stream arm in cmd/rumor, which moved it
	// from 222 to 223 and then to 224 -- the first two times this check priced a
	// real addition rather than a mutant, both within a day of its landing.
	// 224 -> 225 is R9.3's `witness=heard` arm in tests/run, and it is the first
	// time this check caught a line that had NOT gone through the helper: the
	// arm's PROOF sat under a loop that can Errorf, so it could have printed a
	// claim beside the failure refuting it. It was found only because staging
	// the new file made `git ls-files` able to see it -- the whole suite had
	// just passed green with the file untracked, which is this arm's blind spot
	// and the reason a new test file is staged before the gate is believed.
	// This arm's own line is counted like every other rather than exempted: an
	// exemption would put a hole in the only check that can see this file.
	// Retyped deliberately when it changes, which is the point of it.
	// 225 -> 226 is R10 d1's compile-counter arm in internal/engine, whose
	// PROOF names the count one Evaluate produced. It prices a real
	// addition, not a mutant.
	// 226 -> 227 is R10 d1's case-path compile arm in internal/attaint, which
	// the hoist shipped without: the engine path had an arm and its sibling did
	// not, so one of the two counters was read by nothing.
	// 227 -> 228 is R10 d1's pass-scale compile count in tests/attaint, whose
	// PROOF names the count one whole attaint pass produced -- the figure
	// #128's done-when asks for, which neither unit arm could supply.
	// 228 -> 230 is R10 d2's timeline, two arms in this package: one drives the
	// tool over a stamped stream carrying NO Package fields, which is the shape
	// CI's tee actually produces and the one a `go test -json` fixture would
	// have hidden; the other holds that a stream mixing JSON with a plain build
	// error still reports and says it was mixed.
	// 230 -> 231 is the gate on ci.yml's fixed-string test.log patterns. ci
	// mines ../test.log for lines internal/browser writes to stderr, and
	// nothing tied the grep to the emitting format string: when the sweep's
	// line gained the sweeping pid, 'browser: swept' stopped matching anything
	// the code can emit, and that view would have printed its REASSURING
	// sentence on a run that swept 47 trees. The new arm reads ci.yml's own
	// text and requires each pattern to be a literal in internal/browser.
	// 231 -> 232 is the arm that holds the PROJECT's own license identity in
	// its two copies. v0.1.52 replaced BSD-2-Clause with the Blue Oak Model
	// License; LICENSE changed and docs/design.md -- the decision's home, and a
	// file that same commit edited -- went on saying BSD-2-Clause, four lines
	// under a CHANGELOG section announcing the replacement. tests/licences read
	// DEPENDENCY licenses and `rumor docs --check` maps paths, so nothing in the
	// tree read the one the project ships under. The new arm reads LICENSE's own
	// title and holds that sentence against it.
	const sitesInTrackedTests = 232
	if sites != sitesInTrackedTests {
		t.Errorf("%d call(s) of the helper in tracked test files, want %d: if that is "+
			"deliberate, move the literal with it; if not, PROOF lines have been lost",
			sites, sitesInTrackedTests)
	}
	if bad == 0 {
		proof.Logf(t, "PROOF %d tracked test file(s) scanned, %d helper call(s), 0 bare",
			len(files), sites)
	}
}

// AND THE HELPER ITSELF IS DRIVEN BOTH WAYS, because the arm above says only
// WHERE the lines are written, not that writing them through the helper changes
// anything. rook's mutant is the subject (coop r9/249): a failing arm must
// print NO proof.
//
// THE SUBJECT IS A testing.TB OF MY OWN, not a real test, and that is the whole
// reason this cell can exist: a cell cannot fail itself and still report its
// own verdict. proof.Logf takes the interface, so a recorder answers Failed()
// as the cell chooses and keeps what was written -- which makes the negative a
// MEASUREMENT ("zero lines were written") rather than an absence somebody has
// to go looking for in captured output.
type recorder struct {
	testing.TB
	failed bool
	logs   []string
	fatal  string
}

func (r *recorder) Helper()                   {}
func (r *recorder) Failed() bool              { return r.failed }
func (r *recorder) Logf(f string, a ...any)   { r.logs = append(r.logs, fmt.Sprintf(f, a...)) }
func (r *recorder) Fatalf(f string, a ...any) { r.fatal = fmt.Sprintf(f, a...) }

func TestTheHelperLogsOnlyWhenTheTestHasNotFailed(t *testing.T) {
	t.Run("a passing arm logs, and logs what it was given", func(t *testing.T) {
		r := &recorder{}
		proof.Logf(r, "PROOF %d of %d", 3, 4)
		if len(r.logs) != 1 {
			t.Fatalf("wrote %d line(s), want 1: %v", len(r.logs), r.logs)
		}
		if r.logs[0] != "PROOF 3 of 4" {
			t.Errorf("wrote %q, want %q", r.logs[0], "PROOF 3 of 4")
		}
	})

	t.Run("a FAILED arm writes nothing", func(t *testing.T) {
		r := &recorder{failed: true}
		proof.Logf(r, "PROOF this claim must not reach the log")
		if len(r.logs) != 0 {
			t.Errorf("a failing arm wrote %d PROOF line(s), and that is rook's mutant "+
				"exactly -- a red run carrying a green claim: %v", len(r.logs), r.logs)
		}
	})

	t.Run("a format without the word is refused, not logged", func(t *testing.T) {
		// THE WORD IS WHAT ci.yml MINES, so a line that loses the prefix stops
		// reaching the record silently. The refusal is what makes that loud.
		r := &recorder{}
		proof.Logf(r, "this says nothing the miner can find")
		if r.fatal == "" {
			t.Error("a format not beginning with PROOF was accepted, so the word can drift " +
				"out of the record with nothing saying so")
		}
		if len(r.logs) != 0 {
			t.Errorf("the refused format was logged anyway: %v", r.logs)
		}
	})
}
