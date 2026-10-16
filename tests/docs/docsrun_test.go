// Package docs_test holds the `run` blocks in docs/ to the toolchain
// discipline the workflows already keep (ibis rumor/755, from rook /749 and
// quill /752).
//
// WHY IT EXISTS. docs/cli.md's sample block is executed by the release gate --
// that is the whole point of marking a fence `run` -- and it invoked the
// toolchain as a bare `go`. Measured, one block then behaved three ways:
//
//	rook's jail    no `go` at all; the gate refused with doc-run-block-failed
//	quill's jail   `go` = go1.27.2, four verifications under it
//	the runner     go1.26.9, from run 232's own PROOF line
//
// Two defects, not one. The first is MISATTRIBUTION: a missing interpreter
// surfaced on a DOCS arm, so the gate said the documentation had drifted when
// the truth was that the box had no Go -- ci.yml:137 already words this exactly
// right, "a PROVISIONING finding and not a code one", and the block said
// nothing of the sort. The second is the SILENT SUBSTITUTION that
// GOTOOLCHAIN=local exists to stop: under the default `auto`, a go directive
// above the installed toolchain is satisfied by downloading another one, so a
// green run is not evidence about the toolchain that was invoked.
//
// ci.yml resolves `go go126 go125` and exports GOTOOLCHAIN=local at 8 sites,
// tag-pipeline.yml at 2. release-gate.yml does so at none -- and it is the file
// that runs these blocks. This suite is the missing half of that pattern,
// applied where the gate actually reaches it.
//
// WHAT IT CANNOT DO, said plainly:
//
//   - It reads the fence form from tools/release-gate.py and refuses to run if
//     the two disagree, so it cannot check blocks the gate does not run. It
//     still cannot check that the gate RUNS the blocks it extracts; that is
//     release-gate.py's own test.
//   - It strips FULL-LINE comments only. A trailing `# ... go run ...` on a
//     live command line is read as code and reported. That direction is
//     deliberate: a guard that over-reports is read, and one that under-reports
//     is the hole this closes.
//   - It checks that a block names the discipline, not that the discipline
//     works. Only running the block does that, which the gate does.
package docs_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const root = "../.."

// THE FENCE FORM IS THE GATE'S, TRANSLITERATED, AND PINNED BELOW. Python's
// re.M|re.S are Go's (?m) and (?s); every other byte is the same.
const gateFencePython = "^```(?:sh|bash|console)\\s+run\\s*$(?P<body>.*?)^```\\s*$"

var fenceRe = regexp.MustCompile("(?ms)^```(?:sh|bash|console)\\s+run\\s*$(.*?)^```\\s*$")

// A Go toolchain in COMMAND POSITION: at the start of a line, or after a
// pipe, a separator, or a command substitution.
var bareGoRe = regexp.MustCompile(`(?m)(?:^|[;&|(]|\$\()[[:space:]]*(go|go125|go126)[[:space:]]+[a-z]`)

// Any mention of a Go toolchain at all, resolved or bare, so that a block
// invoking "$GO" is still held to the pin.
var usesGoRe = regexp.MustCompile(`(?m)(?:^|[;&|(=]|\$\()[[:space:]]*"?\$?\{?(GO|go|go125|go126)\}?"?[[:space:]]+[a-z]`)

type block struct {
	file string
	// line is the 1-indexed line in file where the fence opens.
	line int
	body string
}

// docRunBlocks returns every `run` block under docs/, in path order.
func docRunBlocks(t *testing.T) []block {
	t.Helper()
	var out []block
	dir := filepath.Join(root, "docs")
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(p, ".md") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		text := string(b)
		for _, m := range fenceRe.FindAllStringSubmatchIndex(text, -1) {
			rel, err := filepath.Rel(root, p)
			if err != nil {
				rel = p
			}
			out = append(out, block{
				file: filepath.ToSlash(rel),
				line: strings.Count(text[:m[0]], "\n") + 1,
				body: text[m[2]:m[3]],
			})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", dir, err)
	}
	return out
}

// stripFullLineComments removes lines whose first non-space character is `#`.
// Deliberately narrow: see the package comment.
func stripFullLineComments(s string) string {
	var keep []string
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		keep = append(keep, line)
	}
	return strings.Join(keep, "\n")
}

// TestTheFenceFormIsTheGateS pins this suite's extraction to the gate's. If
// release-gate.py learns a new fence spelling, every other test here quietly
// starts checking a different set of blocks than the gate runs -- which is the
// failure mode that makes a green suite meaningless rather than wrong.
func TestTheFenceFormIsTheGateS(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(root, "tools", "release-gate.py"))
	if err != nil {
		t.Fatalf("reading the gate: %v", err)
	}
	if !strings.Contains(string(b), gateFencePython) {
		t.Fatalf("tools/release-gate.py no longer contains the fence pattern this suite models:\n  %s\n"+
			"Re-read the gate's run_doc_blocks and update gateFencePython AND fenceRe together, "+
			"or this suite is checking a different set of blocks than the gate executes.", gateFencePython)
	}
}

// TestThereIsAtLeastOneRunBlock is the known positive. Every other test here
// passes vacuously over an empty set, and a fence form that silently stops
// matching is exactly how that would happen.
func TestThereIsAtLeastOneRunBlock(t *testing.T) {
	if got := len(docRunBlocks(t)); got == 0 {
		t.Fatal("no `run` blocks found under docs/, so every check in this suite is vacuous; " +
			"either the fence form moved or the blocks are gone")
	}
}

// bareGoNames returns the bare toolchain names a block invokes, in order.
func bareGoNames(body string) []string {
	var out []string
	for _, m := range bareGoRe.FindAllStringSubmatch(stripFullLineComments(body), -1) {
		out = append(out, m[1])
	}
	return out
}

// pinsToolchain reports whether a block that touches Go also pins it. A block
// that does not touch Go at all pins nothing and needs nothing.
func pinsToolchain(body string) (touchesGo, pinned bool) {
	code := stripFullLineComments(body)
	return usesGoRe.MatchString(code), strings.Contains(code, "GOTOOLCHAIN=local")
}

// TestNoRunBlockInvokesABareGoToolchain is the defect itself. `go` is absent in
// rook's jail and answers with an unpinned version in quill's.
func TestNoRunBlockInvokesABareGoToolchain(t *testing.T) {
	for _, blk := range docRunBlocks(t) {
		for _, name := range bareGoNames(blk.body) {
			t.Errorf("%s:%d: a `run` block invokes the toolchain as a bare %q.\n"+
				"The gate executes this block, and the name is absent in at least one jail and "+
				"answers with an unpinned version in another.\n"+
				"Resolve it the way ci.yml does, then export GOTOOLCHAIN=local:\n"+
				"  GO=\"\"; for c in go go126 go125; do command -v \"$c\" >/dev/null 2>&1 && { GO=\"$c\"; break; }; done\n"+
				"  test -n \"$GO\" || { echo \"no go, go126 or go125 on PATH: PROVISIONING finding\"; exit 1; }\n"+
				"  export GOTOOLCHAIN=local",
				blk.file, blk.line, name)
		}
	}
}

// TestEveryRunBlockUsingGoPinsTheToolchain catches the half a resolve loop
// alone would leave open: GOTOOLCHAIN=auto satisfies a go directive above the
// installed toolchain by downloading another one, so resolving the name says
// nothing about what compiled the code.
func TestEveryRunBlockUsingGoPinsTheToolchain(t *testing.T) {
	for _, blk := range docRunBlocks(t) {
		touchesGo, pinned := pinsToolchain(blk.body)
		if touchesGo && !pinned {
			t.Errorf("%s:%d: a `run` block invokes a Go toolchain and never exports GOTOOLCHAIN=local.\n"+
				"Without it a box below go.mod's floor downloads a toolchain and prints ok, so the "+
				"block's green is not evidence about the toolchain it ran under (ci.yml:138-149).",
				blk.file, blk.line)
		}
	}
}

// TestBothArmsFireOnACraftedBlock keeps the two arms above from becoming
// decoration. Once docs/cli.md is fixed, the only block in the tree is a
// PASSING one, so nothing committed would notice if either arm stopped firing
// -- which is this repo's own "an unreachable guard is not a cautious one",
// and the reason the vocabulary of permitted twins exists (R3.1).
//
// The last two cases are the twins: the shape the fix actually took, and the
// comment-stripping that keeps prose ABOUT `go run` from being read as code.
func TestBothArmsFireOnACraftedBlock(t *testing.T) {
	const fixed = "GO=\"\"\n" +
		"for c in go go126 go125; do\n" +
		"  command -v \"$c\" >/dev/null 2>&1 && { GO=\"$c\"; break; }\n" +
		"done\n" +
		"test -n \"$GO\" || exit 1\n" +
		"export GOTOOLCHAIN=local\n" +
		"out=$(\"$GO\" run ./cmd/rumor attaint --examples examples) || exit 1\n"

	for _, tc := range []struct {
		name      string
		body      string
		wantBare  []string
		wantFires bool // the pin arm
	}{
		{
			name:      "a bare go run trips both arms",
			body:      "out=$(go run ./cmd/rumor attaint) || exit 1\n",
			wantBare:  []string{"go"},
			wantFires: true,
		},
		{
			name:      "a bare versioned name trips both arms",
			body:      "go126 test ./...\n",
			wantBare:  []string{"go126"},
			wantFires: true,
		},
		{
			name:      "resolved but unpinned trips only the pin arm",
			body:      "GO=go126\n\"$GO\" test ./...\n",
			wantBare:  nil,
			wantFires: true,
		},
		{
			name:      "the shape the fix took trips neither",
			body:      fixed,
			wantBare:  nil,
			wantFires: false,
		},
		{
			// THE SUBSTITUTION IN THIS COMMENT IS THE POINT, and the first
			// version of this case did not have it: `go run` written after a
			// backtick sits in no position the scanner matches, so the case
			// passed with comment-stripping DISABLED and proved nothing. A
			// comment can only reach the scanner through a separator it
			// honors -- `$(`, `;`, `|`, `(` -- so the fixture has to carry
			// one, or the arm it guards is dead.
			name:      "a full-line comment about go run is not code",
			body:      "# the previous sample said $(go run ./cmd/rumor) and drifted\n" + fixed,
			wantBare:  nil,
			wantFires: false,
		},
		{
			name:      "a block that never touches Go is asked for nothing",
			body:      "sed -n 's/x/y/p' docs/cli.md\n",
			wantBare:  nil,
			wantFires: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := bareGoNames(tc.body)
			if strings.Join(got, ",") != strings.Join(tc.wantBare, ",") {
				t.Errorf("bare toolchain names: got %v, want %v", got, tc.wantBare)
			}
			touchesGo, pinned := pinsToolchain(tc.body)
			if fires := touchesGo && !pinned; fires != tc.wantFires {
				t.Errorf("the pin arm fired=%v, want %v (touchesGo=%v pinned=%v)",
					fires, tc.wantFires, touchesGo, pinned)
			}
		})
	}
}
