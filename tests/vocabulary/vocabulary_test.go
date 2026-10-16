// Package vocabulary_test checks that every sentence this tree QUOTES from
// docs/vocabulary.md is still in it (quill rumor/577, Q1).
//
// WHY IT EXISTS. R2.3 shipped two quotes that were accurate when written and
// false thirty minutes later, in the same tag: 0b2aa28 quoted the vocabulary's
// `clock` gap sentence and its `told`/`clock` claim, and 2d1cb40 -- the
// amendment, four commits later and in v0.1.11 with it -- reworded both. One of
// them also reported the discrepancy as an open question and routed the reader
// to thread:breakout-r2 for a decision that the same tag had already made. No
// test read those strings, so nothing noticed.
//
// It is the same class as docs/design.md's "in Rust" and statute.rego's stale
// ADR citation, both found by eye during the pivot. The generalisation is that
// a comment quoting a document is a claim about that document, and claims about
// things that move need a check.
//
// WHAT IT CANNOT DO, said plainly because the first three versions of this file
// each over-claimed:
//
//   - It only sees quotes ATTRIBUTED to the vocabulary, by the citation forms
//     this tree actually uses: `vocabulary.md`, `the vocabulary`, `v1`. A quote
//     with no attribution within 60 characters is invisible here. Widening that
//     to every quoted span in every comment was MEASURED and produced seven
//     false positives for one real finding -- blocks that quote the epic or
//     issue 44 in the same breath -- and an oracle that cries wolf seven times
//     is one nobody reads.
//   - It compares text, not meaning. A paraphrase presented as a quotation
//     passes if the words happen to occur.
//
// THREE NORMALISATIONS, each because a real quote in this tree needed it:
//
//	whitespace   the quotes and the document both wrap, so a line-oriented
//	             matcher reports ABSENT for text that is present. quill hit
//	             this first and disclosed it; the first version of THIS file
//	             hit it too and missed both of the real findings.
//	emphasis     the document writes the observation's position as *p* and
//	             internal/flatten quotes it as p. Same claim.
//	case         a sentence quoted mid-prose loses its capital, and
//	             internal/handle emphasises a clause in caps.
package vocabulary_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const root = "../.."

// declaredQuotes is what this tree is expected to quote. A count, not a list of
// the quotes themselves: the quotes are DERIVED below, and the number is here so
// that a sweep which suddenly finds none fails instead of passing. ibis main/352
// -- zero absences reads identically whether every quote was checked or the walk
// never ran.
const declaredQuotes = 13

// citation names the ways this tree attributes a quote to the vocabulary. It is
// written out rather than inferred: a regexp that matched any nearby word would
// collect the epic's and the issue's quotes too, which is the measured failure
// described above.
var attributed = regexp.MustCompile(`(?i)(?:vocabulary\.md|the vocabulary(?:'s)?|v1(?:'s)?)[^"]{0,60}?"([^"]{15,})"`)

var commentLine = regexp.MustCompile(`^(?://+|#+)\s?(.*)$`)

// normalise collapses whitespace, strips markdown emphasis and folds case. See
// the three reasons in the package comment; each came from a real quote here.
func normalise(s string) string {
	s = regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
	s = strings.NewReplacer("*", "", "_", "").Replace(s)
	return strings.ToLower(strings.TrimSpace(s))
}

// commentBlocks returns each contiguous run of comment lines, joined, with the
// line the run starts on. JOINED, because a quotation that wraps across two
// comment lines is one quotation and a line-oriented reader cannot see it.
func commentBlocks(src string) []struct {
	Line int
	Text string
} {
	var out []struct {
		Line int
		Text string
	}
	var cur []string
	start := 0
	flush := func() {
		if len(cur) > 0 {
			out = append(out, struct {
				Line int
				Text string
			}{start, strings.Join(cur, " ")})
			cur = nil
		}
	}
	for n, ln := range strings.Split(src, "\n") {
		if m := commentLine.FindStringSubmatch(strings.TrimSpace(ln)); m != nil {
			if len(cur) == 0 {
				start = n + 1
			}
			cur = append(cur, m[1])
			continue
		}
		flush()
	}
	flush()
	return out
}

// sources walks the tree for the files that can carry a quote. THIS PACKAGE IS
// EXCLUDED, and that is not tidiness: the control below plants a quote that is
// deliberately absent from the document, and a sweep that read its own fixtures
// would report it as a finding every run.
func sources(t *testing.T) []string {
	t.Helper()
	var out []string
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == ".git" || p == filepath.Join(root, "tests", "vocabulary") {
				return filepath.SkipDir
			}
			return nil
		}
		switch filepath.Ext(p) {
		case ".go", ".rego":
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	if len(out) == 0 {
		t.Fatalf("no sources found under %s: an empty walk is not a pass", root)
	}
	return out
}

func doc(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, "docs", "vocabulary.md"))
	if err != nil {
		t.Fatalf("docs/vocabulary.md is not readable, so every quote below would read as absent: %v", err)
	}
	n := normalise(string(b))
	// A DENOMINATOR, so a truncated or empty read cannot pass as agreement.
	// quill's sweep printed one for the same reason.
	if len(n) < 5000 {
		t.Fatalf("the vocabulary normalises to %d chars, far short of the document: a short read would "+
			"make every quote absent and this suite would blame the quotes", len(n))
	}
	return n
}

type quote struct {
	File string
	Line int
	Text string
}

func attributedQuotes(t *testing.T) []quote {
	t.Helper()
	var out []quote
	for _, f := range sources(t) {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		for _, blk := range commentBlocks(string(b)) {
			for _, m := range attributed.FindAllStringSubmatch(blk.Text, -1) {
				out = append(out, quote{File: f, Line: blk.Line, Text: m[1]})
			}
		}
	}
	return out
}

// EVERY QUOTE THIS TREE ATTRIBUTES TO THE VOCABULARY IS STILL IN IT.
func TestEveryAttributedQuoteIsStillInTheDocument(t *testing.T) {
	d := doc(t)
	qs := attributedQuotes(t)
	if len(qs) != declaredQuotes {
		t.Errorf("found %d attributed quote(s), expected %d: a quote was added or removed without "+
			"this count moving, and a sweep that finds none would otherwise pass", len(qs), declaredQuotes)
	}
	absent := 0
	for _, q := range qs {
		if !strings.Contains(d, normalise(q.Text)) {
			absent++
			t.Errorf("%s:%d quotes the vocabulary as %q, which the document no longer says",
				q.File, q.Line, q.Text)
		}
	}
	t.Logf("PROOF %d attributed quote(s) examined over %d char(s) of document, %d absent",
		len(qs), len(d), absent)
}

// THE ORACLE'S SELF-TEST. A check that has never refused anything is
// indistinguishable from a passing suite, so the three ways this one can be
// fooled are driven here with constructed input.
func TestTheQuoteCheckFires(t *testing.T) {
	d := doc(t)
	for _, tc := range []struct {
		name    string
		comment string
		want    int // quotes the attribution rule should collect
		absent  bool
	}{
		{
			// THE REAL DEFECT, reconstructed: R2.3's own stale quote.
			name:    "a stale vocabulary quote is caught",
			comment: "// (docs/vocabulary.md, Fact kinds: \"no example exercises `clock` until R2.3 drives one\")",
			want:    1, absent: true,
		},
		{
			// THE LINE-BREAK TRAP, which defeated quill's first sweep and this
			// file's first version: the same quote, wrapped.
			name: "a stale quote that WRAPS is still caught",
			comment: "// (docs/vocabulary.md, Fact kinds: \"no example exercises `clock` until\n" +
				"// R2.3 drives one\")",
			want: 1, absent: true,
		},
		{
			name:    "the landed wording passes",
			comment: "// docs/vocabulary.md: \"no example carries a `clock` fact until R2.3 drives one\"",
			want:    1, absent: false,
		},
		{
			// EMPHASIS AND CASE, the two normalisations real quotes needed.
			name:    "a quote differing only by emphasis and case passes",
			comment: "// v1: \"EVERY `claim` the observer made and every `told` it received at positions at or before p\"",
			want:    1, absent: false,
		},
		{
			// THE NARROWING, asserted so it cannot widen unnoticed: a quote of
			// something else, with no vocabulary attribution, is not collected.
			name:    "an unattributed quote is not collected at all",
			comment: "// issue 44: \"data = sealed ledger + flattened facts; input = one check\"",
			want:    0, absent: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, blk := range commentBlocks(tc.comment) {
				for _, m := range attributed.FindAllStringSubmatch(blk.Text, -1) {
					got = append(got, m[1])
				}
			}
			if len(got) != tc.want {
				t.Fatalf("collected %d quote(s), wanted %d: %v", len(got), tc.want, got)
			}
			if tc.want == 0 {
				return
			}
			isAbsent := !strings.Contains(d, normalise(got[0]))
			if isAbsent != tc.absent {
				t.Fatalf("quote %q reads absent=%v, wanted %v", got[0], isAbsent, tc.absent)
			}
		})
	}
	t.Logf("PROOF the check catches a stale quote, catches one that wraps, passes the landed wording, " +
		"passes through emphasis and case, and collects nothing from an unattributed quote")
}
