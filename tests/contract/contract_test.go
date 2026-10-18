// Package contract_test checks that docs/contract.md and package wire describe
// the same protocol.
//
// WHY IT EXISTS. R4.1 wrote the field tables in the document and the field sets
// in the code from one draft, and nothing but a reader's memory required them
// to stay equal. This tree has already paid for that twice: R2.3 shipped two
// comments quoting a vocabulary sentence that was reworded four commits later
// in the same tag (tests/vocabulary exists for that), and the release gate's
// contract-version arm matched a Rust filename for three weeks after the Go
// port, green in its own self-test the whole time because the fixture was Rust
// too (quill coop r4/5). A document and a table that must agree are two copies
// of one schema; the only thing that keeps two copies honest is a check that
// reads both.
//
// WHAT IT CANNOT DO, said plainly:
//
//   - It compares the field SETS, not the types or the required/optional
//     split. `observe`'s token rule ("present exactly when the source is not
//     direct") is prose in the document and an arm in the decoder, driven in
//     internal/wire's own tests; this file would pass if the document called it
//     required.
//   - It reads the tables by their first column and the labels above them. A
//     field named only in prose is invisible here, which is why every message's
//     fields are in a table and the error envelope's six KINDS are in a table
//     with a different header.
package contract_test

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/decl"
	"forge.axonibyte.com/axonibyte/rumor/internal/platform"
	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

const doc = "../../docs/contract.md"

// section is one `### ` heading's text, up to the next one.
func sections(t *testing.T, text string) map[string]string {
	t.Helper()
	out := map[string]string{}
	heads := regexp.MustCompile(`(?m)^### (.+)$`).FindAllStringSubmatchIndex(text, -1)
	for i, h := range heads {
		title := strings.TrimSpace(text[h[2]:h[3]])
		end := len(text)
		if i+1 < len(heads) {
			end = heads[i+1][0]
		}
		out[title] = text[h[1]:end]
	}
	if len(out) == 0 {
		t.Fatal("no `### ` headings found: the parser below would then report every set as empty")
	}
	return out
}

var rowField = regexp.MustCompile("(?m)^\\| `([^`]+)` \\|")

// fieldsAfter reads the backticked names in the first column of the table that
// follows `label` in `section`, stopping at the next label. "nothing" where a
// table would be is an empty set and not a parse failure -- `describe`'s
// request says exactly that.
func fieldsAfter(section, label string) ([]string, bool) {
	i := strings.Index(section, label)
	if i < 0 {
		return nil, false
	}
	rest := section[i+len(label):]
	// BOUNDED BY THE NEXT LABEL, so a request that says "nothing" does not
	// read the response's table as its own -- which is what the first run of
	// this test reported for `describe`.
	if j := strings.Index(rest, "beyond the envelope:**"); j >= 0 {
		rest = rest[:j]
	}
	// ONLY THE FIRST CONTIGUOUS TABLE. The error envelope's section carries a
	// second table -- its six KINDS -- and a scan that ran to the next label
	// read those as fields, which is how this function was first written and
	// what its first run reported.
	var rows []string
	started := false
	for _, ln := range strings.Split(rest, "\n") {
		if strings.HasPrefix(ln, "|") {
			started = true
			rows = append(rows, ln)
			continue
		}
		if started {
			break
		}
	}
	var out []string
	for _, m := range rowField.FindAllStringSubmatch(strings.Join(rows, "\n"), -1) {
		out = append(out, m[1])
	}
	sort.Strings(out)
	return out, true
}

func TestTheDocumentAndTheDecoderDescribeTheSameMessages(t *testing.T) {
	raw, err := os.ReadFile(doc)
	if err != nil {
		t.Fatalf("reading %s: %v", doc, err)
	}
	text := string(raw)
	secs := sections(t, text)

	// THE ENVELOPE IS READ FROM THE DOCUMENT TOO, not assumed: it is the one
	// field set every message below is measured against, so a wrong one here
	// would shift all nine comparisons at once.
	envelope, ok := fieldsAfter(secs["Fields every message carries"], "| field | type | consumer |")
	if !ok {
		t.Fatal("the envelope table was not found under \"Fields every message carries\"")
	}
	wantEnvelope := []string{"contract", "id", "kind"}
	if !equal(envelope, wantEnvelope) {
		t.Fatalf("the document's envelope is %v, want %v", envelope, wantEnvelope)
	}

	// headings, by kind. The error envelope is response-only, which the
	// decoder enforces and this table records.
	headings := map[wire.Kind]string{
		wire.KindDescribe: "`describe`",
		wire.KindExecute:  "`execute`",
		wire.KindObserve:  "`observe`",
		wire.KindSession:  "`session`",
		wire.KindError:    "The error envelope",
	}

	// The document's own words for each direction, so the label is not
	// reconstructed by capitalising a string.
	labels := map[string]string{
		"request":  "**Request, beyond the envelope:**",
		"response": "**Response, beyond the envelope:**",
	}

	compared := 0
	for _, direction := range []string{"request", "response"} {
		for _, kind := range wire.Kinds(direction) {
			heading, ok := headings[kind]
			if !ok {
				t.Errorf("%s: the protocol carries this kind and this file names no heading for it", kind)
				continue
			}
			section, ok := secs[heading]
			if !ok {
				t.Errorf("%s: no `### %s` section in %s", kind, heading, doc)
				continue
			}
			label := labels[direction]
			documented, found := fieldsAfter(section, label)
			if !found {
				t.Errorf("%s %s: %q is not under `### %s`", kind, direction, label, heading)
				continue
			}
			required, optional, ok := wire.Schema(direction, kind)
			if !ok {
				t.Fatalf("%s %s: package wire has no schema", kind, direction)
			}
			code := append(append([]string(nil), required...), optional...)
			code = without(code, envelope)
			sort.Strings(code)
			if !equal(documented, code) {
				t.Errorf("%s %s: the document lists %v, package wire %v (both beyond the envelope)",
					kind, direction, documented, code)
			}
			compared++
		}
	}

	// VACUITY GUARD, derived rather than remembered: every kind in each
	// direction must have been compared. A parser that found no sections would
	// otherwise report a clean agreement.
	want := len(wire.Kinds("request")) + len(wire.Kinds("response"))
	if compared != want || compared == 0 {
		t.Errorf("compared %d message(s), want %d", compared, want)
	}
}

// TestTheDocumentNamesTheVersionTheCodeHolds: the three statements of the
// contract version are the engine's constant, the adapter's field and
// platform.toml's key, and the document quotes the value. A bump that moved the
// constant and not the prose would leave the document naming a version nothing
// speaks.
func TestTheDocumentNamesTheVersionTheCodeHolds(t *testing.T) {
	raw, err := os.ReadFile(doc)
	if err != nil {
		t.Fatalf("reading %s: %v", doc, err)
	}
	quoted := "v1's value is `\"" + wire.ContractVersion + "\"`"
	if !strings.Contains(string(raw), quoted) {
		t.Errorf("%s does not contain %q, so the document and wire.ContractVersion disagree", doc, quoted)
	}
	// AND THE EXAMPLE platform.toml CARRIES THE SAME VALUE, which is the copy a
	// project would start from.
	sample := "contract = \"" + wire.ContractVersion + "\""
	if !strings.Contains(string(raw), sample) {
		t.Errorf("%s's platform.toml sample does not contain %q", doc, sample)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func without(all, drop []string) []string {
	skip := map[string]bool{}
	for _, d := range drop {
		skip[d] = true
	}
	var out []string
	for _, a := range all {
		if !skip[a] {
			out = append(out, a)
		}
	}
	return out
}

// --- the document's own samples (R4.1, R4.2) ------------------------------

// TestEveryTomlSampleInTheDocumentParses.
//
// THIS IS THE TEST R4.1 LISTED AS OWED and could not write until the reader
// landed: `docs/contract.md` prints a `platform.toml`, a `bindings.toml` and a
// UI-flow fragment, and until now nothing read them. A documented sample that
// cannot fail is decoration -- this tree's own methodology -- and these three
// are the first thing a project copies.
//
// EACH FENCE IS PARSED AS WHAT ITS SECTION SAYS IT IS, which is also how the
// test avoids the shape that bit the first version of the field-table parser:
// the fences are found inside their own `## ` section rather than by position
// in the file.
func TestEveryTomlSampleInTheDocumentParses(t *testing.T) {
	raw, err := os.ReadFile(doc)
	if err != nil {
		t.Fatalf("reading %s: %v", doc, err)
	}
	text := string(raw)

	platformSamples := tomlFences(t, text, "## `platform.toml` schema")
	if len(platformSamples) != 1 {
		t.Fatalf("%d toml sample(s) under the platform.toml schema, want 1", len(platformSamples))
	}
	doc0, err := decl.TOML([]byte(platformSamples[0]))
	if err != nil {
		t.Fatalf("the platform.toml sample is not TOML: %v", err)
	}
	if rep := platform.Parse(doc0); !rep.OK() {
		t.Errorf("the platform.toml sample does not parse as a platform: %v", rep.Refusals)
	}

	bindingSamples := tomlFences(t, text, "## Bindings schema")
	if len(bindingSamples) != 2 {
		t.Fatalf("%d toml sample(s) under the bindings schema, want 2 (the file and the UI flow)",
			len(bindingSamples))
	}
	for i, sample := range bindingSamples {
		decoded, err := decl.TOML([]byte(sample))
		if err != nil {
			t.Errorf("bindings sample %d is not TOML: %v", i, err)
			continue
		}
		if rep := bindings.Parse(decoded); !rep.OK() {
			t.Errorf("bindings sample %d does not parse as bindings: %v", i, rep.Refusals)
		}
	}
}

// tomlFences returns the ```toml blocks inside one `## ` section.
func tomlFences(t *testing.T, text, heading string) []string {
	t.Helper()
	start := strings.Index(text, heading)
	if start < 0 {
		t.Fatalf("no %q section in %s", heading, doc)
	}
	rest := text[start+len(heading):]
	if end := strings.Index(rest, "\n## "); end >= 0 {
		rest = rest[:end]
	}
	var out []string
	for _, m := range tomlFence.FindAllStringSubmatch(rest, -1) {
		out = append(out, m[1])
	}
	return out
}

var tomlFence = regexp.MustCompile("(?s)```toml\n(.*?)```")
