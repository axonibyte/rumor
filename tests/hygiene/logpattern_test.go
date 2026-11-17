package hygiene_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/proof"
)

// TestEveryTestLogPatternCIMinesIsEmittedBySomeCode is the gate that was
// missing when I broke one of these views myself.
//
// ci.yml mines ../test.log for lines this repo's own code writes to stderr --
// the browser sweep's report and the group-left-alive warning. Nothing tied
// the grep pattern to the emitting format string, so the two could drift
// apart silently: the view would exit 1, "no match", and print the REASSURING
// sentence. That is indistinguishable from a run where the subsystem had
// nothing to report.
//
// IT HAPPENED. The sweep line gained the sweeping process's pid, becoming
// `browser: pid 19595 swept ...`, while ci.yml still grepped 'browser: swept'
// -- a pattern that no longer matches anything the code can emit. The head
// that made the line MORE useful would have made the view blind, and a green
// run would have said "the browser sweep collected nothing this run" while the
// sweep collected 47 trees.
//
// THE ARM READS ci.yml's OWN TEXT, as ciproof_test.go does, because ci.yml is
// configuration: its text is the thing under test, not a copy of it.
func TestEveryTestLogPatternCIMinesIsEmittedBySomeCode(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", ".forgejo", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("reading ci.yml: %v", err)
	}
	// The single-quoted fixed-string greps over ../test.log. The -aE forms are
	// regexes and belong to ciproof_test.go, which drives them as regexes.
	// EITHER ARTIFACT, because the property is about the PATTERN and not the
	// file: the views moved from ../test.log to ../test.json when condition (5)
	// at coop r10/22 put every instrument on the record, and an arm pinned to
	// the old name would have read "no views found" and said so as a Fatal --
	// a gate failing because the thing it guards improved.
	rx := regexp.MustCompile(`grep -a '([^']+)' \.\./test\.(?:log|json)`)
	found := rx.FindAllStringSubmatch(string(raw), -1)
	if len(found) == 0 {
		t.Fatal("no fixed-string greps over the suite's artifact found in ci.yml: either the " +
			"views were removed or this arm's regex no longer matches the step's spelling")
	}

	sources, err := filepath.Glob(filepath.Join("..", "..", "internal", "browser", "*.go"))
	if err != nil || len(sources) == 0 {
		t.Fatalf("globbing internal/browser: %v (%d file(s))", err, len(sources))
	}
	var body strings.Builder
	for _, f := range sources {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("reading %s: %v", f, err)
		}
		body.Write(b)
	}
	code := body.String()

	for _, m := range found {
		pattern := m[1]
		// THE PATTERN MUST BE A LITERAL PIECE OF AN EMITTED STRING. A format
		// string can carry a %d between words, so a pattern that spans one
		// cannot be found here -- which is itself the finding: such a pattern
		// is one the code may stop matching without anything noticing, and the
		// fix is to grep a prefix the format owns.
		if !strings.Contains(code, pattern) {
			t.Errorf("ci.yml mines ../test.log for %q and no literal in "+
				"internal/browser/*.go contains it, so that view can only ever "+
				"report \"no match\" -- which prints the view's REASSURING "+
				"sentence and reads exactly like a quiet subsystem", pattern)
		}
	}
	if !t.Failed() {
		proof.Logf(t, "PROOF %d fixed-string test.log pattern(s) in ci.yml, every one "+
			"of them a literal in internal/browser's own emitted strings", len(found))
	}
}
