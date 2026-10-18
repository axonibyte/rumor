package decl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// doc is a document exercising every reader below, built rather than parsed --
// which is the seam this package exists to offer.
func doc() map[string]any {
	return map[string]any{
		"name":    "a platform",
		"on":      true,
		"count":   int64(3),
		"budget":  "5m",
		"paths":   []any{"one", "two"},
		"native":  []string{"three"},
		"table":   map[string]any{"inner": "value"},
		"entries": []any{map[string]any{"k": "v"}, map[string]any{"k": "w"}},
	}
}

func has(t *testing.T, refusals []Refusal, path, fragment string) bool {
	t.Helper()
	for _, r := range refusals {
		if r.Path == path && (fragment == "" || strings.Contains(r.Reason, fragment)) {
			return true
		}
	}
	return false
}

// TestEveryReaderReadsItsOwnType is the control: the battery below is all
// refusals, and a reader that refused everything would pass every one of them.
func TestEveryReaderReadsItsOwnType(t *testing.T) {
	r := New(doc())
	if got := r.Str("name", true); got != "a platform" {
		t.Errorf("Str = %q", got)
	}
	if !r.Bool("on", true) {
		t.Error("Bool = false")
	}
	if got := r.Int("count", true); got != 3 {
		t.Errorf("Int = %d", got)
	}
	if got := r.Duration("budget", true); got != 5*time.Minute {
		t.Errorf("Duration = %v", got)
	}
	if got := r.Strings("paths", true); len(got) != 2 || got[0] != "one" {
		t.Errorf("Strings = %v", got)
	}
	// A READER MAY HAND OVER []string RATHER THAN []any, which some decoders do
	// for a homogeneous array; refusing that would refuse a legal file.
	if got := r.Strings("native", true); len(got) != 1 || got[0] != "three" {
		t.Errorf("Strings over []string = %v", got)
	}
	inner, ok := r.Table("table", true)
	if !ok || inner.Str("inner", true) != "value" {
		t.Error("Table")
	}
	inner.Done()
	entries := r.Tables("entries")
	if len(entries) != 2 {
		t.Fatalf("Tables = %d", len(entries))
	}
	for _, e := range entries {
		e.Str("k", true)
		e.Done()
	}
	r.Done()
	if found := r.Refusals(); len(found) != 0 {
		t.Errorf("a well-formed document was refused: %v", found)
	}
}

func TestAMissingRequiredKeyIsNamedAndAnOptionalOneIsNot(t *testing.T) {
	r := New(map[string]any{})
	r.Str("name", true)
	r.Bool("on", true)
	r.Int("count", true)
	r.Strings("paths", true)
	r.Duration("budget", true)
	_, ok := r.Table("table", true)
	if ok {
		t.Error("Table reported a table that is not there")
	}
	found := r.Refusals()
	for _, key := range []string{"name", "on", "count", "paths", "budget"} {
		if !has(t, found, key, "missing key") {
			t.Errorf("no missing-key refusal for %q: %v", key, found)
		}
	}
	if !has(t, found, "table", "missing table") {
		t.Errorf("no missing-table refusal: %v", found)
	}
	// THE OTHER SIDE, because a reader that refused every absence would pass
	// the loop above: optional keys are silent.
	r2 := New(map[string]any{})
	r2.Str("name", false)
	r2.Bool("on", false)
	r2.Int("count", false)
	r2.Strings("paths", false)
	r2.Duration("budget", false)
	r2.Table("table", false)
	r2.Tables("entries")
	r2.Done()
	if found := r2.Refusals(); len(found) != 0 {
		t.Errorf("absent optional keys were refused: %v", found)
	}
}

func TestAWrongTypeSaysWhatItWanted(t *testing.T) {
	// EACH CASE CARRIES ITS OWN WRONG VALUE. The first draft of this table used
	// one value -- int64(7) -- for every reader, and Int ACCEPTED it, so the
	// row asserting "wanted an integer" failed for the right reason: a reader
	// cannot be wrong about a value of its own type.
	for _, tc := range []struct {
		want  string
		value any
		read  func(*Reader)
	}{
		{"wanted a string", int64(7), func(r *Reader) { r.Str("name", true) }},
		{"wanted true or false", "yes", func(r *Reader) { r.Bool("name", true) }},
		{"wanted an integer", "many", func(r *Reader) { r.Int("name", true) }},
		{"wanted an array of strings", "one", func(r *Reader) { r.Strings("name", true) }},
		{"wanted a table", "flat", func(r *Reader) { r.Table("name", true) }},
		{"wanted an array of tables", map[string]any{}, func(r *Reader) { r.Tables("name") }},
	} {
		r := New(map[string]any{"name": tc.value})
		tc.read(r)
		if !has(t, r.Refusals(), "name", tc.want) {
			t.Errorf("%v: refusals are %v", tc.value, r.Refusals())
		}
	}

	// A FLOAT WHERE AN INTEGER IS WANTED IS REFUSED, NOT TRUNCATED: 2.5 actors
	// is a mistake in the file, and rounding it carries the mistake into the run.
	r := New(map[string]any{"count": 2.5})
	r.Int("count", true)
	if !has(t, r.Refusals(), "count", "wanted an integer") {
		t.Errorf("2.5 was accepted as an integer: %v", r.Refusals())
	}

	// AN ITEM's type is named with its INDEX, because "paths wanted strings"
	// does not say which one.
	r = New(map[string]any{"paths": []any{"one", int64(2)}})
	r.Strings("paths", true)
	if !has(t, r.Refusals(), "paths[1]", "wanted a string") {
		t.Errorf("the bad item is not named by index: %v", r.Refusals())
	}
	r = New(map[string]any{"entries": []any{map[string]any{}, "not a table"}})
	r.Tables("entries")
	if !has(t, r.Refusals(), "entries[1]", "wanted a table") {
		t.Errorf("the bad entry is not named by index: %v", r.Refusals())
	}
}

func TestADurationOfZeroOrLessIsRefused(t *testing.T) {
	for _, bad := range []string{"0s", "-1m"} {
		r := New(map[string]any{"budget": bad})
		r.Duration("budget", true)
		if !has(t, r.Refusals(), "budget", "zero or less") {
			t.Errorf("%q was accepted: %v", bad, r.Refusals())
		}
	}
	r := New(map[string]any{"budget": "a fortnight"})
	r.Duration("budget", true)
	if !has(t, r.Refusals(), "budget", "wanted a duration") {
		t.Errorf("an unparseable duration: %v", r.Refusals())
	}
}

// TestDoneRefusesWhatNothingRead is the whole unknown-key rule, at every depth:
// a level that forgot to call Done would be the only level that passes.
func TestDoneRefusesWhatNothingRead(t *testing.T) {
	r := New(map[string]any{
		"name":    "x",
		"extra":   true,
		"table":   map[string]any{"inner": "v", "surplus": 1},
		"entries": []any{map[string]any{"k": "v", "spare": 2}},
	})
	r.Str("name", true)
	inner, _ := r.Table("table", true)
	inner.Str("inner", true)
	inner.Done()
	for _, e := range r.Tables("entries") {
		e.Str("k", true)
		e.Done()
	}
	r.Done()
	found := r.Refusals()
	for _, path := range []string{"extra", "table.surplus", "entries[0].spare"} {
		if !has(t, found, path, "unknown key") {
			t.Errorf("no unknown-key refusal at %q: %v", path, found)
		}
	}
	if len(found) != 3 {
		t.Errorf("%d refusal(s), want 3: %v", len(found), found)
	}
}

// TestHasDoesNotMarkAKeyRead, for the rule "this key is required only when that
// one says so": a Has that marked would make the key exempt from Done.
func TestHasDoesNotMarkAKeyRead(t *testing.T) {
	r := New(map[string]any{"conditional": "x"})
	if !r.Has("conditional") {
		t.Fatal("Has reported a present key absent")
	}
	if r.Has("nonesuch") {
		t.Error("Has reported an absent key present")
	}
	r.Done()
	if !has(t, r.Refusals(), "conditional", "unknown key") {
		t.Errorf("Has exempted the key from Done: %v", r.Refusals())
	}
}

func TestAnyCarriesAValueAndMarksItRead(t *testing.T) {
	r := New(map[string]any{"thresholds": 0.9})
	if v := r.Any("thresholds"); v != 0.9 {
		t.Errorf("Any = %v", v)
	}
	r.Done()
	if found := r.Refusals(); len(found) != 0 {
		t.Errorf("Any did not mark the key read: %v", found)
	}
}

// TestRefusalsAreSortedAndCarryTheirPath: a reader comparing two runs over one
// document reads the same order, so a diff of two reports is about the
// document.
func TestRefusalsAreSortedAndCarryTheirPath(t *testing.T) {
	r := New(map[string]any{"z": 1, "a": 1, "m": 1})
	r.Done()
	found := r.Refusals()
	if len(found) != 3 {
		t.Fatalf("%d refusal(s): %v", len(found), found)
	}
	for i := 1; i < len(found); i++ {
		if found[i-1].Path > found[i].Path {
			t.Fatalf("not sorted: %v", found)
		}
	}
	if found[0].Error() != "a: unknown key" {
		t.Errorf("Error() = %q, want the path and the reason", found[0].Error())
	}
	// THE FILE'S NAME IS NOT HERE, deliberately: this package is handed a
	// decoded document and a refusal over one a test built has no filename.
	if strings.Contains(found[0].Error(), ".toml") {
		t.Errorf("Error() names a file: %q", found[0].Error())
	}
}

// TestAChildsRefusalReachesTheRoot, which is what makes Refusals() the whole
// report rather than the top table's share of it.
func TestAChildsRefusalReachesTheRoot(t *testing.T) {
	r := New(map[string]any{"table": map[string]any{"deeper": map[string]any{}}})
	outer, _ := r.Table("table", true)
	inner, _ := outer.Table("deeper", true)
	inner.Refuse("leaf", "a reason")
	if !has(t, r.Refusals(), "table.deeper.leaf", "a reason") {
		t.Errorf("a grandchild's refusal did not reach the root: %v", r.Refusals())
	}
	if got := inner.Path(); got != "table.deeper" {
		t.Errorf("Path() = %q", got)
	}
}

// TestANilDocumentIsAnEmptyOneAndNotAPanic: a decoder handed an empty file can
// return nil, and the refusals should then be the missing keys rather than a
// crash in the engine.
func TestANilDocumentIsAnEmptyOneAndNotAPanic(t *testing.T) {
	r := New(nil)
	r.Str("name", true)
	r.Done()
	if !has(t, r.Refusals(), "name", "missing key") {
		t.Errorf("a nil document: %v", r.Refusals())
	}
}

// --- Load, the file half --------------------------------------------------

// TestLoadRefusesTheFileAndNotTheProject: every refusal here is about the FILE,
// which is could-not-run. A file that cannot be read says nothing about what it
// would have declared, and reporting it as a verdict on the project would send
// the reader to fix the wrong thing.
func TestLoadRefusesTheFileAndNotTheProject(t *testing.T) {
	dir := t.TempDir()
	good := func(b []byte) (map[string]any, error) { return map[string]any{"name": string(b)}, nil }

	write := func(name, text string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
		return p
	}

	// THE CONTROL FIRST, so the refusals below are not a reader that refuses
	// everything.
	doc, err := Load(write("ok.toml", "name = x"), good)
	if err != nil {
		t.Fatalf("a readable file is refused: %v", err)
	}
	if doc["name"] != "name = x" {
		t.Errorf("document %v", doc)
	}

	for _, tc := range []struct {
		name, want string
		path       string
		dec        Decoder
	}{
		{"no decoder wired", "no decoder is wired", write("a.toml", "x"), nil},
		{"absent file", "no such file", filepath.Join(dir, "nonesuch.toml"), good},
		{"empty file", "the file is empty", write("empty.toml", ""), good},
		{"whitespace only", "the file is empty", write("blank.toml", "\n\t \n"), good},
		{"the decoder refuses", "cannot be read as a declaration", write("bad.toml", "x"),
			func([]byte) (map[string]any, error) { return nil, errDecode{} }},
		{"the decoder answers nothing at all", "no document and no error", write("nil.toml", "x"),
			func([]byte) (map[string]any, error) { return nil, nil }},
	} {
		_, err := Load(tc.path, tc.dec)
		if err == nil {
			t.Errorf("%s: accepted", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %q does not say %q", tc.name, err, tc.want)
		}
		// THE PATH IS IN EVERY ONE OF THEM: a run reads several declaration
		// files, and "the file is empty" without a name is a refusal the reader
		// has to go looking for.
		if !strings.Contains(err.Error(), filepath.Base(tc.path)) {
			t.Errorf("%s: %q does not name the file", tc.name, err)
		}
	}
}

type errDecode struct{}

func (errDecode) Error() string { return "line 1: unexpected character" }

// --- the TOML reader ------------------------------------------------------

// TestTOMLProducesWhatTheReadersExpect: every Reader method above is written
// against the shapes a TOML decoder hands over, and until this test the claim
// "integers arrive as int64, tables as map[string]any, arrays as []any" was a
// comment. It is the two-instrument rule applied to a dependency -- the schema's
// expectations on one side, the library's actual output on the other.
func TestTOMLProducesWhatTheReadersExpect(t *testing.T) {
	doc, err := TOML([]byte(`
name = "a platform"
on = true
count = 3
budget = "5m"
paths = ["one", "two"]

[table]
inner = "value"

[[entries]]
k = "v"

[[entries]]
k = "w"
`))
	if err != nil {
		t.Fatalf("TOML: %v", err)
	}
	r := New(doc)
	if got := r.Str("name", true); got != "a platform" {
		t.Errorf("Str = %q", got)
	}
	if !r.Bool("on", true) {
		t.Error("Bool = false")
	}
	if got := r.Int("count", true); got != 3 {
		t.Errorf("Int = %d: a TOML integer must reach Int, whatever width the library chose", got)
	}
	if got := r.Duration("budget", true); got != 5*time.Minute {
		t.Errorf("Duration = %v", got)
	}
	if got := r.Strings("paths", true); len(got) != 2 || got[1] != "two" {
		t.Errorf("Strings = %v", got)
	}
	inner, ok := r.Table("table", true)
	if !ok || inner.Str("inner", true) != "value" {
		t.Error("Table")
	}
	inner.Done()
	entries := r.Tables("entries")
	if len(entries) != 2 {
		t.Fatalf("Tables = %d, want 2: an array of tables must arrive as one", len(entries))
	}
	for _, e := range entries {
		e.Str("k", true)
		e.Done()
	}
	r.Done()
	if found := r.Refusals(); len(found) != 0 {
		t.Errorf("a well-formed TOML document was refused: %v", found)
	}
}

func TestTOMLRefusesWhatIsNotTOMLAndAcceptsAnEmptyDocument(t *testing.T) {
	if _, err := TOML([]byte("name = \nthis is not toml")); err == nil {
		t.Error("malformed TOML was accepted")
	} else if !strings.Contains(err.Error(), "not TOML") {
		t.Errorf("%q does not say what it is not", err)
	}
	// AN ALL-COMMENTS FILE IS A DOCUMENT WITH NO KEYS, not a nil one -- and this
	// is a claim about BurntSushi/toml v1.6.0, measured: an empty file, a
	// comments-only file and a blank-line file each decode to a non-nil map of
	// length 0. The assertion is here so that a library change which starts
	// answering nil is caught by a test rather than by Load refusing a file
	// that was readable.
	doc, err := TOML([]byte("# nothing but a comment\n"))
	if err != nil {
		t.Fatalf("an all-comments file: %v", err)
	}
	if doc == nil {
		t.Error("an all-comments file decoded to nil, which Load reads as a broken decoder")
	}
	if len(doc) != 0 {
		t.Errorf("document %v", doc)
	}
}
