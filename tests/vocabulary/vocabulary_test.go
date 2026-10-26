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
	"strconv"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/vocabulary"
)

const root = "../.."

// declaredQuotes is what this tree is expected to quote. A count, not a list of
// the quotes themselves: the quotes are DERIVED below, and the number is here so
// that a sweep which suddenly finds none fails instead of passing. ibis main/352
// -- zero absences reads identically whether every quote was checked or the walk
// never ran.
// 13 -> 14 in R3.1: internal/attaint quotes the `planted` entry's first use,
// which is the clause the corroboration arm implements. The quote is PRESENT --
// the oracle reported 0 absent and refused only on the count, which is the
// count doing its job.
// 14 -> 15 in R3.2: internal/attaint quotes the `planted` entry's SECOND use,
// the strip, which is the clause the new arm implements. Same shape as the
// 13 -> 14 move: the oracle reported 0 absent and refused on the count alone.
// 26 -> 28 in R5.1: internal/schedule's pool and internal/engine's
// CanardSubjects each quote the v1 row's sentence about canard -- "a weight and
// a precondition over the kinds above, not a fact kind and not a tag" -- which
// is the clause that decides canard is NOT a fact kind and so has no ledger
// representation to implement. Same shape as both moves above: the oracle
// reported 0 absent and refused on the count alone, so the count is what caught
// the addition and the quotes were already true.
// 28 -> 29 in R5.1, the nemesis preconditions: internal/schedule quotes the
// planted-claim entry's "a lie an actor tells about its own action", which is
// the clause fable's ruling at coop r5/49 rests on -- suborn and palter inherit
// an honest action's precondition BECAUSE the lie is about the actor's own
// action, so an action the actor could not take is a lie of a kind this
// document does not define. Third move of the same shape: 0 absent, refused on
// the count.
// 29 -> 31 in R5.4, and NOT a new quotation: examples/toy/mutants gained
// m10-has-user-always, and a mutant is a COPY of its statute, so the two quotes
// examples/toy/statute.rego already carried are now counted twice. The count is
// of quotes this tree makes, not of distinct sentences, because the sweep's job
// is to notice a document that stopped saying something -- and it reported 0
// absent here, as at every move above, refusing on the count alone.
// 31 -> 32 in R5.5: internal/schedule's observe step quotes the sealing rule,
// "A ledger is sealed at a position before any check runs; the check reads the
// sealed prefix", because the walk must seal before it evaluates an observation
// as a check. Fifth move of the same shape: 0 absent, refused on the count.
//
// EVERY MOVE KEEPS ITS REASON, AND NOTHING BELOW ENFORCES THAT. rook named it
// at coop r5/160: this arm refuses on the COUNT and tests MEMBERSHIP of each
// quote, so it catches a number that moved and a quote that was never the
// document's -- and it cannot see a reason that fell out. The reasons are prose
// at the point of use, which is why they are here and not in a commit message
// nobody re-reads.
//
// SO IF YOU ARE RESOLVING THIS NUMBER IN A RE-CUT, keep every reason, not the
// count. R5.3's third and R5.5 each take this to 32 from v0.1.27's 31, for
// DIFFERENT quotations -- internal/shrink's Findings entry and the observe
// step's sealed-prefix rule -- so onto v0.1.28 the answer is 33 with THREE
// reasons, and the one most likely to be dropped is the oldest, because by then
// it is two bases back and nothing refuses on its absence.
//
// AND THE MOVE CAME WITH A CORRECTED ATTRIBUTION. The first version of that
// comment credited this document with engine.Input's REFUSAL TEXT -- "seals a
// prefix before any check runs, so this is a sequencing error and not an empty
// result" -- and the sweep reported 32 against 31 with that line named and 1
// absent. The count and the membership test catch different halves of one
// mistake: the count says a quote moved, the membership says it was never the
// document's.
//
// AND 32 -> 33 IN R5.3's RE-CUT ONTO v0.1.28, which is the resolution the
// paragraph above asked for: the third reason, kept rather than folded into a
// number. R5.3's own reason stands unchanged -- internal/shrink quotes the
// Findings entry's "names the rule that produced it", which is why the
// clean-world replay gate compares a replayed finding by RULE as well as by
// value: one value reached by a different clause is a different finding, so two
// distinct failures would otherwise read as one reproduction and the seed would
// be shrunk against the wrong one.
//
// AND THAT CLAUSE'S REACH IS NAMED, because ratchet read it the wide way (coop
// r5/210) and a later reader would too: it is true of two NAMED clauses, and it
// does NOT cover the `bug` DEFAULT. A finding from `default finding := "bug"`
// fires no clause, so its Rule is the default's own path and constant across
// two such findings -- the rule comparison cannot separate them, and nothing
// else in engine.Finding can either. That case is answered by the attribution
// instead, and by Shrink saying so when the attribution was not compared.
//
// The moves are ADDITIVE and none cancels another: R5.4's came from a mutant
// copying a statute, R5.5's from the observe step quoting the sealing rule,
// R5.3's from a new quotation in a new package.
// The number is the sweep's and not my arithmetic -- it refuses on the count and
// names the actual, which is how this value was settled.
// 33 -> 34 AT R6.1: the v2 Revisions row quotes v1's own "later phases add
// kinds under `bug`" sentence, to show that `stale` lands as the kind v1
// reserved it as rather than as a fifth finding. A quote inside the document
// about the document, which this sweep counts like any other -- and the quote
// is the warrant for the shape, so it is worth the count moving.
// 35 -> 44 AT R6's GATE BUNDLE, AND NOT ONE NEW QUOTATION: examples/toy's ten
// mutants are COPIES of its statute, and NINE had been cut from older revisions
// of it and never re-cut, so each was missing the comment blocks that carry the
// statute's quotes. Regenerating all ten from the current statute
// (tools/make-toy-mutants.py) gives each of those nine the quote its statute has
// carried all along: +1 per mutant, +9. Same mechanism as 29 -> 31 in R5.4, when
// m10 was added -- this sweep counts the quotes the TREE MAKES and not the
// distinct sentences, because its job is to notice a document that stopped
// saying something.
//
// THE NUMBER IS THE SWEEP'S AND THE BASE IS NAMED. This commit's first cut said
// 34 -> 43 on a v0.1.33 base; re-cut onto v0.1.34 (21fd1a0) the base is 35,
// because R6.2 added the `quiesce_timeout` overlay's quotation, so the same +9
// lands on 44. 0 absent, as at every move above, so the count alone is what
// refused.
// 44 -> 45 AT R6.3: tests/quiesce quotes the `stale` row's own rule -- "a run
// that ends on one leaves no pending open" -- because that clause is the
// warrant for the arm asserting Record.PendingOpen at all. The rule was
// unmeasurable before that field existed, so the quote is what says the
// assertion is the document's and not the arm's invention.
const declaredQuotes = 45

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

// retired names a mechanism this project does not use any more, with the reason
// it is forbidden in a fixture. A FIXTURE IS READ BY A HUMAN WHEN IT FAILS, so a
// fixture naming a tool that does not exist sends that reader looking for a call
// they will never find -- which is what rook said at go-port/58 about the
// statute and is why the same three sentences in six mutants are a defect and
// not untidiness.
//
// WHY A SWEEP AND NOT A HASH PIN, which is the part worth carrying. The drift
// test in tests/statute pins the three statutes and their rules.json, and it
// would have caught none of this: the mutants are not pinned, and more to the
// point these sentences were false THE DAY THE MUTANTS WERE CUT. They were
// copied from the statute's header before the statute was corrected. A pin
// catches an edit; it cannot catch a birth defect. Only a sweep over the whole
// corpus can, which is ratchet's rule at r1.vocab/8 -- found by sweeping rather
// than fixing the instance -- applied to examples/ instead of the vocabulary.
var retired = map[string]string{
	"adr-regorus-tracing/1": "ADR 0001 lives at docs/adr/0001-engine.md; the thread it was drafted in is not its citation",
	"set_gather_prints":     "Regorus's print API. The engine is OPA: rego.EnablePrintStatements, a print.Hook and a static scan",
	"take_prints":           "Regorus's print API, as above",
	"flattening in Rust":    "the flattening is Go, in internal/flatten, since the G1 pivot",
	"crates/rumor":          "the Rust tree was removed in G4; nothing under examples/ should name a crate path",
}

// NO FIXTURE NAMES A RETIRED MECHANISM. Counts the corpus it examined and
// proves it saw one, because zero hits over zero files is the same number as
// zero hits over twenty-two.
func TestNoFixtureNamesARetiredMechanism(t *testing.T) {
	var files []string
	err := filepath.Walk(filepath.Join(root, "examples"), func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking examples/: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no files under examples/: an empty walk is not a pass")
	}

	// THE POSITIVE CONTROL, and it is a word every one of the three packages
	// must contain: without it a mis-built path or an unreadable file would
	// make every fixture clean by not being read.
	saw := 0
	hits := 0
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		body := string(b)
		if strings.Contains(body, "rumor") || strings.Contains(body, "claim") {
			saw++
		}
		for name, why := range retired {
			if strings.Contains(body, name) {
				hits++
				t.Errorf("%s names the retired %q: %s", f, name, why)
			}
		}
	}
	if saw == 0 {
		t.Fatalf("examined %d file(s) under examples/ and none mentioned the project at all, so the "+
			"sweep did not read the corpus it reports on", len(files))
	}
	t.Logf("PROOF %d file(s) under examples/ examined, %d of them read as this project's, %d retired "+
		"name(s) found, %d name(s) forbidden", len(files), saw, hits, len(retired))
}

// THE SWEEP'S OWN SELF-TEST: it must refuse a fixture that names a retired
// mechanism, and admit the corrected wording. Driven on constructed text rather
// than by editing the corpus, so the suite cannot be left dirty by a failure.
func TestTheRetiredNameSweepFires(t *testing.T) {
	stale := "# NO print() ANYWHERE, per ADR 0001 (adr-regorus-tracing/1): `set_gather_prints(true)`\n" +
		"# and then `take_prints()` must come back empty. (R2 does the flattening in Rust.)"
	fixed := "# NO print() ANYWHERE, per ADR 0001 (docs/adr/0001-engine.md): a static scan and a hook\n" +
		"# under rego.EnablePrintStatements. (R2 does the flattening in Go, internal/flatten.)"

	var found []string
	for name := range retired {
		if strings.Contains(stale, name) {
			found = append(found, name)
		}
	}
	if len(found) != 4 {
		t.Fatalf("the stale fixture trips %d retired name(s), wanted 4: %v", len(found), found)
	}
	for name := range retired {
		if strings.Contains(fixed, name) {
			t.Errorf("the corrected wording still trips %q", name)
		}
	}
	t.Logf("PROOF the stale header trips %d of %d retired names and the corrected header trips none",
		len(found), len(retired))
}

// --- the version constant (R4.1) ------------------------------------------

// TestTheVersionConstantIsTheDocumentsHighestRevision.
//
// docs/vocabulary.md's preamble promised this constant to R4's contract, and a
// constant nothing reads would be a token for the release gate to match rather
// than a claim. So the document is the source and the constant is held to it.
//
// THE HIGHEST ROW, NOT THE LAST ONE. The first version of this check read the
// table's final row, which assumes an ordering the document never states --
// driven against a `v9` row inserted at the TOP of the table, it passed. Taking
// the maximum is order-agnostic, so a newer row anywhere in the table is seen.
//
// AND NOT "v1 APPEARS SOMEWHERE", which stays true forever once v2 is added --
// exactly the bump this check exists to catch.
func TestTheVersionConstantIsTheDocumentsHighestRevision(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(root, "docs", "vocabulary.md"))
	if err != nil {
		t.Fatalf("docs/vocabulary.md: %v", err)
	}
	rows := revisionVersions(string(b))
	if len(rows) == 0 {
		t.Fatal("no version rows under \"Revisions\": a parser that found none would otherwise agree with anything")
	}
	highest := rows[0]
	for _, v := range rows {
		if versionNumber(v) > versionNumber(highest) {
			highest = v
		}
	}
	if highest != vocabulary.VocabularyVersion {
		t.Errorf("the Revisions table's highest version is %q and internal/vocabulary says %q: "+
			"one of the two moved without the other (rows seen: %v)", highest, vocabulary.VocabularyVersion, rows)
	}
}

// revisionVersions reads the first column of the Revisions table's rows. It
// skips the header and the separator by requiring the cell to look like a
// version, which is also what stops it reading the tables above.
func revisionVersions(text string) []string {
	start := strings.Index(text, "## Revisions")
	if start < 0 {
		return nil
	}
	var out []string
	for _, line := range strings.Split(text[start:], "\n") {
		m := revisionRow.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		out = append(out, m[1])
	}
	return out
}

// versionNumber reads the integer out of a `vN` cell. A cell the row pattern
// matched always has one, so a parse failure is impossible rather than ignored.
func versionNumber(v string) int {
	n, err := strconv.Atoi(strings.TrimPrefix(v, "v"))
	if err != nil {
		return -1
	}
	return n
}

var revisionRow = regexp.MustCompile(`^\|\s*(v[0-9]+)\s*\|`)
