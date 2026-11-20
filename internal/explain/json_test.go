package explain

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/examples"
)

func jsonOf(t *testing.T, pkg, name string) ([]byte, loaded, *Derivation) {
	t.Helper()
	lc := load(t, pkg, name)
	d := lc.build(t)
	var b bytes.Buffer
	if err := RenderJSON(&b, d); err != nil {
		t.Fatalf("%s/%s: RenderJSON: %v", pkg, name, err)
	}
	return b.Bytes(), lc, d
}

// jsonAsTheCommandWrites builds the derivation the way `rumor explain` does --
// through examples.Load, so the statute digest is the real one -- and renders
// it.
//
// THE HARNESS'S OWN LOADER CANNOT PIN THE SCHEMA. load() invents a digest
// ("sha-of-toy") because d1's and d2's arms do not care what it is; the
// fixture and the reproduction tuple do, and a pinned file built from an
// invented digest would pin the harness rather than the contract. Found by
// these two arms failing against a fixture generated from the command.
func jsonAsTheCommandWrites(t *testing.T, pkg, name string) []byte {
	t.Helper()
	c, err := examples.Load(context.Background(), filepath.Join("..", "..", "examples", pkg), name)
	if err != nil {
		t.Fatalf("%s/%s: %v", pkg, name, err)
	}
	d, err := Build(c.Finding, c.Ledger, c.Check, Run{Package: c.Package, Case: c.Name, StatuteSHA: c.StatuteSHA})
	if err != nil {
		t.Fatalf("%s/%s: Build: %v", pkg, name, err)
	}
	var b bytes.Buffer
	if err := RenderJSON(&b, d); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// --- d3's named arms -----------------------------------------------------

// TestTheJSONRoundTripsBackToTheValue is d3's round-trip arm.
//
// FULL DeepEqual CANNOT HOLD AND THE REASON IS THE SHAPE'S, not a defect: a
// derivation carries the statute's own answers in `any` fields -- an observed
// value, a rule's verdict -- and JSON has one number type, so an int arrives
// back as a float64. So the comparison is in two halves: the typed sub-values,
// which must survive exactly, and the whole value compared THROUGH JSON in
// both directions, which is what "the same derivation" can mean across a
// format that does not carry Go's types.
func TestTheJSONRoundTripsBackToTheValue(t *testing.T) {
	for _, pkg := range packages(t) {
		for _, name := range cases(t, pkg) {
			raw, _, d := jsonOf(t, pkg, name)

			var back Derivation
			if err := json.Unmarshal(raw, &back); err != nil {
				t.Fatalf("%s/%s: the JSON this package wrote does not parse: %v", pkg, name, err)
			}

			// The halves with no `any` in them survive exactly.
			if !reflect.DeepEqual(d.Selector, back.Selector) {
				t.Errorf("%s/%s: selector %+v became %+v", pkg, name, d.Selector, back.Selector)
			}
			if !reflect.DeepEqual(d.Verdict.Rules, back.Verdict.Rules) {
				t.Errorf("%s/%s: rules %+v became %+v", pkg, name, d.Verdict.Rules, back.Verdict.Rules)
			}
			if !reflect.DeepEqual(d.Expectation.Known, back.Expectation.Known) {
				t.Errorf("%s/%s: the chain changed across the round trip", pkg, name)
			}
			if !reflect.DeepEqual(d.Expectation.Claims, back.Expectation.Claims) {
				t.Errorf("%s/%s: the claims changed across the round trip", pkg, name)
			}
			if !reflect.DeepEqual(d.Reproduction, back.Reproduction) {
				t.Errorf("%s/%s: reproduction %+v became %+v", pkg, name, d.Reproduction, back.Reproduction)
			}
			if (d.Clock == nil) != (back.Clock == nil) {
				t.Errorf("%s/%s: the clock's presence changed across the round trip", pkg, name)
			}
			if d.Clock != nil && back.Clock != nil && *d.Clock != *back.Clock {
				t.Errorf("%s/%s: clock %v became %v", pkg, name, *d.Clock, *back.Clock)
			}

			// And the whole value, through the format in both directions.
			var again bytes.Buffer
			if err := RenderJSON(&again, &back); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(raw, again.Bytes()) {
				t.Errorf("%s/%s: re-rendering the parsed derivation differs:\n%s\n----\n%s",
					pkg, name, raw, again.Bytes())
			}

			// The rule names survive with their values and their reach.
			for rule, h := range d.Expectation.Holds {
				got, ok := back.Expectation.Holds[rule]
				if !ok {
					t.Errorf("%s/%s: %s is missing after the round trip", pkg, name, rule)
					continue
				}
				if got.Reached != h.Reached || !reflect.DeepEqual(got.Rules, h.Rules) {
					t.Errorf("%s/%s: %s came back reached=%v %v, was reached=%v %v",
						pkg, name, rule, got.Reached, got.Rules, h.Reached, h.Rules)
				}
			}
		}
	}
}

// TestTheSchemaIsPinnedByAFixture is d3's "a schema change fails a pinned
// fixture by name". The fixture is the command's own output, committed, so
// the file in testdata and the schema in docs/findings.md describe one thing.
func TestTheSchemaIsPinnedByAFixture(t *testing.T) {
	path := filepath.Join("testdata", "derivation-stale-count.json")
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the pinned fixture is unreadable, so nothing pins the schema: %v", err)
	}
	got := jsonAsTheCommandWrites(t, "toy", "stale-count")
	if !bytes.Equal(want, got) {
		t.Errorf("%s no longer matches the derivation. If the shape changed on purpose, "+
			"regenerate it with `rumor explain --json examples/toy stale-count > %s` and move "+
			"docs/findings.md's schema with it.\n--- fixture\n%s\n--- got\n%s", path, path, want, got)
	}

	// THE FIXTURE IS NOT EMPTY AND NOT A STUB, so a truncated or absent file
	// cannot pass this arm by matching nothing.
	var probe map[string]any
	if err := json.Unmarshal(want, &probe); err != nil {
		t.Fatalf("the fixture is not JSON: %v", err)
	}
	for _, key := range []string{"selector", "verdict", "expectation", "observation", "reproduction"} {
		if _, ok := probe[key]; !ok {
			t.Errorf("the fixture carries no %q, so it pins only part of the schema", key)
		}
	}
}

// TestNoTraceKeyAtAnyDepth is the structural half of the trace decision, and
// the half r11/4 settled as d3's: engine.Finding's Trace is exported and
// untagged, so one embedded engine type would ship OPA's pretty text under a
// "Trace" key with nothing failing. d1 asserts the TYPES carry no trace
// field; this asserts the rendered JSON carries no such KEY at any depth.
func TestNoTraceKeyAtAnyDepth(t *testing.T) {
	for _, pkg := range packages(t) {
		for _, name := range cases(t, pkg) {
			raw, _, _ := jsonOf(t, pkg, name)
			var any_ any
			if err := json.Unmarshal(raw, &any_); err != nil {
				t.Fatal(err)
			}
			if hits := traceKeys(any_, "$"); len(hits) > 0 {
				t.Errorf("%s/%s carries %v", pkg, name, hits)
			}
		}
	}

	// THE CONTROL: the walker must find a trace key when one is there, at
	// depth, or the arm above would pass over any document at all.
	var planted any
	if err := json.Unmarshal([]byte(`{"a":{"b":[{"trace":"..."}]}}`), &planted); err != nil {
		t.Fatal(err)
	}
	if hits := traceKeys(planted, "$"); len(hits) != 1 {
		t.Errorf("the walker found %v in a document that carries one trace key", hits)
	}
	var cased any
	if err := json.Unmarshal([]byte(`{"Trace":"..."}`), &cased); err != nil {
		t.Fatal(err)
	}
	if hits := traceKeys(cased, "$"); len(hits) != 1 {
		t.Errorf("the walker missed a capitalised Trace, which is the spelling an embedded engine type would ship: %v", hits)
	}
}

// traceKeys reports every key at any depth whose name mentions a trace.
func traceKeys(v any, path string) []string {
	var out []string
	switch t := v.(type) {
	case map[string]any:
		for k, sub := range t {
			if strings.Contains(strings.ToLower(k), "trace") {
				out = append(out, path+"."+k)
			}
			out = append(out, traceKeys(sub, path+"."+k)...)
		}
	case []any:
		for i, sub := range t {
			out = append(out, traceKeys(sub, fmt.Sprintf("%s[%d]", path, i))...)
		}
	}
	return out
}

// TestTheJSONIsByteStableForOneFinding is d3's "stable across runs for the
// same finding": the only map in the shape is Holds, which encoding/json
// writes in sorted key order, and every slice has a total order.
func TestTheJSONIsByteStableForOneFinding(t *testing.T) {
	for _, pkg := range packages(t) {
		for _, name := range cases(t, pkg) {
			first, _, d := jsonOf(t, pkg, name)
			for i := 0; i < 8; i++ {
				var b bytes.Buffer
				if err := RenderJSON(&b, d); err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(first, b.Bytes()) {
					t.Fatalf("%s/%s differs on pass %d", pkg, name, i)
				}
			}
			if len(d.Expectation.Holds) < 2 {
				t.Errorf("%s/%s holds %d rule(s), so this arm cannot see a map's order", pkg, name, len(d.Expectation.Holds))
			}
		}
	}
}

// TestFromTheJSONAloneTheRawTraceIsReachable is rook's condition from coop
// r11/2, driven rather than promised: the reproduction tuple is the pointer
// back to a trace nothing retains, so a reader with the JSON and nothing else
// must be able to re-run the finding and get the trace.
func TestFromTheJSONAloneTheRawTraceIsReachable(t *testing.T) {
	raw := jsonAsTheCommandWrites(t, "toy", "stale-count")

	// Everything below comes out of the DOCUMENT, not out of the test's own
	// knowledge: that is what "from the JSON alone" means.
	var doc struct {
		Reproduction struct {
			Package    string `json:"package"`
			Case       string `json:"case"`
			Seal       int    `json:"seal"`
			StatuteSHA string `json:"statute_sha"`
			Events     int    `json:"events"`
		} `json:"reproduction"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	r := doc.Reproduction
	if r.Package == "" || r.Case == "" || r.Seal == 0 || r.StatuteSHA == "" || r.Events == 0 {
		t.Fatalf("the reproduction is not usable on its own: %+v", r)
	}

	c, err := examples.Load(context.Background(), filepath.Join("..", "..", "examples", r.Package), r.Case)
	if err != nil {
		t.Fatalf("re-running %s/%s from the JSON alone: %v", r.Package, r.Case, err)
	}
	if c.Finding.Trace == "" || c.Finding.TraceEvents == 0 {
		t.Fatal("the re-run produced no trace, so the tuple does not reach it")
	}
	if c.Finding.TraceEvents != r.Events {
		t.Errorf("the re-run traced %d event(s) and the document says %d", c.Finding.TraceEvents, r.Events)
	}
	if c.StatuteSHA != r.StatuteSHA {
		t.Errorf("the statute is now %s and the document names %s: the finding cannot be reproduced "+
			"from this document, which is what the digest is for", c.StatuteSHA, r.StatuteSHA)
	}
	if int(c.Ledger.Seal()) != r.Seal {
		t.Errorf("the re-run sealed at %d and the document says %d", c.Ledger.Seal(), r.Seal)
	}
}

// TestAHostileRefIsNotEscapedAway pins the one encoder option this file sets:
// without SetEscapeHTML(false) a `<` in a ref arrives as <, which is
// hostile to a reader and to a diff, and nothing else in the suite would
// notice.
func TestAHostileRefIsNotEscapedAway(t *testing.T) {
	d := &Derivation{
		Selector:    Selector{Actor: "a<b", Query: "q&r", Position: 1},
		Verdict:     Verdict{Value: "bug", Defined: true, Says: "bug"},
		Expectation: Expectation{Holds: map[string]Hold{}},
		Observation: Observation{Actor: "a<b", Value: "x>y", Position: 1},
	}
	var b bytes.Buffer
	if err := RenderJSON(&b, d); err != nil {
		t.Fatal(err)
	}
	text := b.String()
	for _, want := range []string{`"a<b"`, `"q&r"`, `"x>y"`} {
		if !strings.Contains(text, want) {
			t.Errorf("%s is not in the output:\n%s", want, text)
		}
	}
	// THE NEEDLES ARE ASSEMBLED AT RUNTIME. Written literally, `\u003c` in
	// this file is liable to be unescaped into `<` by a tool between the
	// author and the file -- which happened, and inverted this very check
	// into "fail if the output contains a `<`". A needle built from parts
	// cannot be rewritten that way.
	esc := func(hex string) string { return string([]byte{92}) + "u" + hex }
	for _, bad := range []string{esc("003c"), esc("003e"), esc("0026")} {
		if strings.Contains(text, bad) {
			t.Errorf("the encoder escaped HTML after all (%s is in the output):\n%s", bad, text)
		}
	}
}

func TestRenderJSONRefusesNothingToRender(t *testing.T) {
	var b bytes.Buffer
	err := RenderJSON(&b, nil)
	if err == nil {
		t.Fatal("RenderJSON accepted a nil derivation")
	}
	if !strings.Contains(err.Error(), "no derivation to render") {
		t.Errorf("the refusal does not say what was missing: %v", err)
	}
	if b.Len() != 0 {
		t.Errorf("it wrote %d byte(s) before refusing", b.Len())
	}
}

func TestAFailedJSONWriteIsReported(t *testing.T) {
	_, _, d := jsonOf(t, "toy", "stale-count")
	boom := errors.New("the pipe closed")
	err := RenderJSON(failingWriter{err: boom}, d)
	if err == nil {
		t.Fatal("RenderJSON reported success over a writer that accepted nothing")
	}
	if !errors.Is(err, boom) {
		t.Errorf("the writer's own error is not in the chain: %v", err)
	}
}

// TestEveryFixtureKeyIsInTheSchemaTable holds docs/findings.md's table to the
// fixtures: a field that enters the data and not the document fails here.
//
// THE DOCUMENT IS THE CONTRACT THE NEXT PHASE CONSUMES, so "it is written
// somewhere" is not enough -- it has to be written in the table a reader of
// that phase will look at. Keys absent from both fixtures (the overlay's, the
// statute's own verdict, the finding's kind) cannot be checked this way because
// no committed example produces a `pending` finding; the overlay arms pin those
// instead, and the document says so.
func TestEveryFixtureKeyIsInTheSchemaTable(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "findings.md"))
	if err != nil {
		t.Fatal(err)
	}
	table := string(doc)
	if !strings.Contains(table, "### The JSON form") {
		t.Fatal("docs/findings.md has no JSON form section, so nothing documents the schema")
	}

	checked := 0
	for _, name := range []string{"derivation-stale-count.json", "derivation-invitation-honored-late.json"} {
		raw, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatal(err)
		}
		for _, path := range schemaPaths(v, "") {
			checked++
			if !strings.Contains(table, "`"+path+"`") {
				t.Errorf("%s carries %s and the table in docs/findings.md does not mention it", name, path)
			}
		}
	}
	if checked < 25 {
		t.Fatalf("only %d key path(s) checked, and the shape has more than that: the walker is not reaching into the document", checked)
	}
	t.Logf("%d key path(s) checked against the table", checked)
}

// schemaPaths lists the dotted key paths of a decoded document, with array
// indices collapsed to `[]` and the one map keyed by rule name collapsed to
// `<rule>`, which is how the table spells them.
func schemaPaths(v any, prefix string) []string {
	var out []string
	switch t := v.(type) {
	case map[string]any:
		for k, sub := range t {
			path := k
			if prefix != "" {
				path = prefix + "." + k
			}
			if prefix == "expectation.holds" {
				path = "expectation.holds.<rule>"
			}
			out = append(out, path)
			out = append(out, schemaPaths(sub, path)...)
		}
	case []any:
		for _, sub := range t {
			out = append(out, schemaPaths(sub, prefix+"[]")...)
		}
	}
	return unique(out)
}

func unique(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
