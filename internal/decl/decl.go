// Package decl reads a decoded declaration file strictly.
//
// Two files in this project are declarations a project writes and this engine
// must refuse rather than interpret generously: `platform.toml` (R4.1) and a
// package's `bindings.toml` (R4.2). They share every rule about HOW a key is
// read -- unknown keys are errors, a missing required key is named, a wrong
// type says what it wanted, and every refusal carries the key's full dotted
// path -- and differ only in WHICH keys they have.
//
// So the rules live here once. The first cut had them in internal/platform and
// internal/bindings would have been a copy; a hand-built copy is correct until
// the original learns something.
//
// WHY THE STRICTNESS IS HERE AND NOT IN A DECODER'S METADATA: Reader takes the
// already-decoded document, so the refusals are the same whichever TOML reader
// produced it, and every one of them can be driven from a value a test built
// with no file on disk. That is the seam `cmd/rumor` uses for its own argument
// surface, one level down.
//
// UNKNOWN IS AN ERROR, NOT A SKIP, which is docs/coverage.toml's argument for
// its own parser: a lenient reader turns one mistyped key into a declaration
// that runs with a weight, a stage, a provider or a precondition the project
// believes it wrote.
package decl

import (
	"fmt"
	"sort"
	"time"
)

// Refusal is one reason a document is not the declaration it claims to be.
// Path is the key's full dotted path, which is the only form a reader can act
// on without counting braces.
type Refusal struct {
	Path   string
	Reason string
}

// Error renders the refusal. The caller supplies the file's name through
// Reader's Name, because "platform.toml: stages.api-sim: unknown key" and the
// same sentence about a bindings file are different findings.
func (r Refusal) Error() string {
	if r.Path == "" {
		return r.Reason
	}
	return r.Path + ": " + r.Reason
}

// Reader walks one table, remembers which keys it read, and refuses the rest.
// It is the Undecoded() of a hand-written decoder.
type Reader struct {
	doc    map[string]any
	path   string
	read   map[string]bool
	parent *Reader

	refusals []Refusal
}

// New starts a reader over a whole document.
func New(doc map[string]any) *Reader {
	if doc == nil {
		doc = map[string]any{}
	}
	return &Reader{doc: doc}
}

// Refusals reports every refusal raised through this reader or any child of
// it, sorted by path and then by reason so two runs over one document read the
// same way and a diff of two reports is about the document.
func (r *Reader) Refusals() []Refusal {
	top := r.root()
	out := append([]Refusal(nil), top.refusals...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Reason < out[j].Reason
	})
	return out
}

func (r *Reader) root() *Reader {
	top := r
	for top.parent != nil {
		top = top.parent
	}
	return top
}

// Refuse records a refusal against a key of this table, or against the table
// itself when key is empty.
func (r *Reader) Refuse(key, reason string) {
	top := r.root()
	top.refusals = append(top.refusals, Refusal{Path: r.join(key), Reason: reason})
}

// Path is this table's own dotted path, for a caller raising a cross-key
// refusal after the walk.
func (r *Reader) Path() string { return r.path }

func (r *Reader) join(key string) string {
	switch {
	case r.path == "":
		return key
	case key == "":
		return r.path
	default:
		return r.path + "." + key
	}
}

func (r *Reader) mark(key string) {
	if r.read == nil {
		r.read = map[string]bool{}
	}
	r.read[key] = true
}

// Keys reports this table's keys, sorted, and marks none of them: a caller
// iterating keys is responsible for reading each one.
func (r *Reader) Keys() []string {
	out := make([]string, 0, len(r.doc))
	for k := range r.doc {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Has reports whether a key is present, without marking it read. For a caller
// whose rule is "this key is required only when that one says so".
func (r *Reader) Has(key string) bool {
	_, ok := r.doc[key]
	return ok
}

// Done refuses every key nothing read.
func (r *Reader) Done() {
	for _, k := range r.Keys() {
		if !r.read[k] {
			r.Refuse(k, "unknown key")
		}
	}
}

// Any returns a key's value as decoded, marking it read. For a table whose
// inner keys belong to a later phase and are carried rather than checked.
func (r *Reader) Any(key string) any {
	r.mark(key)
	return r.doc[key]
}

// Str reads a string.
func (r *Reader) Str(key string, required bool) string {
	r.mark(key)
	v, ok := r.doc[key]
	if !ok {
		if required {
			r.Refuse(key, "missing key")
		}
		return ""
	}
	s, ok := v.(string)
	if !ok {
		r.Refuse(key, fmt.Sprintf("%T: wanted a string", v))
		return ""
	}
	return s
}

// Bool reads a boolean.
func (r *Reader) Bool(key string, required bool) bool {
	r.mark(key)
	v, ok := r.doc[key]
	if !ok {
		if required {
			r.Refuse(key, "missing key")
		}
		return false
	}
	b, ok := v.(bool)
	if !ok {
		r.Refuse(key, fmt.Sprintf("%T: wanted true or false", v))
		return false
	}
	return b
}

// Int reads an integer. A FLOAT IS REFUSED RATHER THAN TRUNCATED: 2.5 actors is
// a mistake in the file, and rounding it would carry the mistake into the run.
func (r *Reader) Int(key string, required bool) int {
	r.mark(key)
	v, ok := r.doc[key]
	if !ok {
		if required {
			r.Refuse(key, "missing key")
		}
		return 0
	}
	n, ok := asInt(v)
	if !ok {
		r.Refuse(key, fmt.Sprintf("%T: wanted an integer", v))
		return 0
	}
	return n
}

// asInt accepts the integer types a TOML reader may hand over.
func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case int32:
		return int(n), true
	}
	return 0, false
}

// Duration reads a duration such as 5m or 2h. Zero or less is refused: a
// budget that runs nothing is a mistake and not a configuration.
func (r *Reader) Duration(key string, required bool) time.Duration {
	s := r.Str(key, required)
	if s == "" {
		return 0
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		r.Refuse(key, fmt.Sprintf("%q: wanted a duration such as 5m or 2h", s))
		return 0
	}
	if d <= 0 {
		r.Refuse(key, fmt.Sprintf("%q: a duration of zero or less covers nothing", s))
		return 0
	}
	return d
}

// Strings reads an array of strings.
func (r *Reader) Strings(key string, required bool) []string {
	r.mark(key)
	v, ok := r.doc[key]
	if !ok {
		if required {
			r.Refuse(key, "missing key")
		}
		return nil
	}
	items, ok := v.([]any)
	if !ok {
		if ss, ok := v.([]string); ok {
			return ss
		}
		r.Refuse(key, fmt.Sprintf("%T: wanted an array of strings", v))
		return nil
	}
	out := make([]string, 0, len(items))
	for i, item := range items {
		s, ok := item.(string)
		if !ok {
			r.Refuse(fmt.Sprintf("%s[%d]", key, i), fmt.Sprintf("%T: wanted a string", item))
			continue
		}
		out = append(out, s)
	}
	return out
}

// Table reads a sub-table. The second return says whether it was there; the
// reader is never nil, so a caller can read from it either way and the
// refusals land on the right paths.
func (r *Reader) Table(key string, required bool) (*Reader, bool) {
	r.mark(key)
	v, ok := r.doc[key]
	if !ok {
		if required {
			r.Refuse(key, "missing table")
		}
		return r.child(key, map[string]any{}), false
	}
	t, ok := v.(map[string]any)
	if !ok {
		r.Refuse(key, fmt.Sprintf("%T: wanted a table", v))
		return r.child(key, map[string]any{}), false
	}
	return r.child(key, t), true
}

// Tables reads an array of tables. AN ABSENT KEY IS AN EMPTY LIST AND NOT A
// REFUSAL: the caller decides whether empty is allowed, because one file's
// empty array is fatal and another's is ordinary.
func (r *Reader) Tables(key string) []*Reader {
	r.mark(key)
	v, ok := r.doc[key]
	if !ok {
		return nil
	}
	items, ok := v.([]any)
	if !ok {
		r.Refuse(key, fmt.Sprintf("%T: wanted an array of tables", v))
		return nil
	}
	var out []*Reader
	for i, item := range items {
		t, ok := item.(map[string]any)
		if !ok {
			r.Refuse(fmt.Sprintf("%s[%d]", key, i), fmt.Sprintf("%T: wanted a table", item))
			continue
		}
		out = append(out, &Reader{
			doc:    t,
			path:   fmt.Sprintf("%s[%d]", r.join(key), i),
			parent: r,
		})
	}
	return out
}

func (r *Reader) child(key string, doc map[string]any) *Reader {
	return &Reader{doc: doc, path: r.join(key), parent: r}
}
