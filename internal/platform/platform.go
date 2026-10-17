// Package platform reads a project's `platform.toml`: which adapter to run,
// who the run draws its actors from, which stages exist, how the walk is
// weighted, and where its seeds live.
//
// docs/contract.md's `platform.toml` section is this package's prose.
//
// WHERE THE STRICTNESS LIVES, and why it is here rather than in a decoder's
// metadata: Parse takes the DECODED document -- whatever produced it -- and
// refuses an unknown key by its full path, a missing required key by its path,
// and a wrong type with the type it wanted. Writing it this way has two
// consequences worth stating. The refusals are the same whichever TOML reader
// is wired in (that choice is a dependency decision, held in coop r4/16), and
// every refusal is driven from a value a test built, with no file on disk and no
// text to get right -- the same seam cmd/rumor uses so its argument surface can
// be driven without a workspace.
//
// NONE OF THESE ARE `rumor validate`'s ARMS. Validate's eighteen refusals are
// about a project whose statute, bindings and platform do not agree (R4.3).
// These are about a file that cannot be read as a platform at all, which is
// could-not-run and arrives before any arm evaluates.
package platform

import (
	"fmt"
	"sort"
	"time"
)

// DefaultSeedsFile is where R5 promotes a seed when the file is not named.
const DefaultSeedsFile = "seeds.lock"

// Platform is one project's declaration.
type Platform struct {
	Contract   string
	SeedsFile  string
	Adapter    Adapter
	Venire     []Member
	Stages     map[string]Stage
	Weights    Weights
	Profiles   map[string]Profile
	Rollback   []Rollback
	Thresholds map[string]any
}

// Adapter is the argv the engine spawns, and the two optional parts of that
// spawn. Env holds variable NAMES passed through, so a platform's secrets stay
// out of this file.
type Adapter struct {
	Command []string
	Cwd     string
	Env     []string
}

// Member is one entry in the venire: a role the statute speaks and how many
// actors of it the run draws.
type Member struct {
	Role  string
	Count int
}

// Stage is one stage's selection. Enabled is required because the two
// browser-stage arms read it, and an absent value would make them vacuous.
type Stage struct {
	Enabled bool
}

// Weights are the walk's weights: a complete default table and sparse
// overrides per stage and per profile.
type Weights struct {
	Default map[string]int
	Stage   map[string]map[string]int
	Profile map[string]map[string]int
}

// Profile is R22's: which stages to run, how many seeds, and the budget.
type Profile struct {
	Stages []string
	Seeds  int
	Budget time.Duration
}

// Rollback is the platform-side half of the arm on a stateful component no
// rollback provider covers. docs/rollback.md defines the provider interface in
// R13; the names have to agree then.
type Rollback struct {
	Provider string
	Covers   []string
}

// Refusal is one reason a document is not a platform. Path is the key's full
// dotted path, which is the only form a reader can act on without counting
// braces.
type Refusal struct {
	Path   string
	Reason string
}

func (r Refusal) Error() string {
	if r.Path == "" {
		return "platform.toml: " + r.Reason
	}
	return fmt.Sprintf("platform.toml: %s: %s", r.Path, r.Reason)
}

// Weight resolves one action kind's weight for a stage and a profile.
//
// THE ORDER IS STATED BECAUSE AN UNSTATED PRECEDENCE IS WHERE A TUNED WEIGHT
// SILENTLY DOES NOTHING: profile, then stage, then default. Either name may be
// empty, which means "no override from that side".
func (w Weights) Weight(stage, profile, kind string) (int, bool) {
	if profile != "" {
		if t, ok := w.Profile[profile]; ok {
			if v, ok := t[kind]; ok {
				return v, true
			}
		}
	}
	if stage != "" {
		if t, ok := w.Stage[stage]; ok {
			if v, ok := t[kind]; ok {
				return v, true
			}
		}
	}
	v, ok := w.Default[kind]
	return v, ok
}

// --- parsing --------------------------------------------------------------

// Report is what reading a document produced: the platform, and EVERY reason
// it is not one. All of them rather than the first, for `rumor attaint`'s
// reason one phase over -- a file that trips three refusals is a different
// event from one that trips the refusal it was written to trip, and a reader
// who gets one at a time cannot tell those apart.
type Report struct {
	Platform Platform
	Refusals []Refusal
}

// OK reports whether the document is a platform.
func (r Report) OK() bool { return len(r.Refusals) == 0 }

// Parse reads a decoded document. The argument is what a TOML reader produced:
// tables as map[string]any, arrays as []any, integers as int64 or int.
func Parse(doc map[string]any) Report {
	r := &reader{doc: doc}
	var p Platform

	p.Contract = r.str("contract", true)
	p.SeedsFile = r.str("seeds_file", false)
	if p.SeedsFile == "" {
		p.SeedsFile = DefaultSeedsFile
	}

	if a, ok := r.table("adapter", true); ok {
		p.Adapter.Command = a.strings("command", true)
		if len(p.Adapter.Command) == 0 {
			r.refuse("adapter.command", "an empty argv: the engine would have nothing to spawn")
		}
		p.Adapter.Cwd = a.str("cwd", false)
		p.Adapter.Env = a.strings("env", false)
		a.done()
	}

	for _, m := range r.tables("venire") {
		var member Member
		member.Role = m.str("role", true)
		member.Count = m.integer("count", true)
		if member.Count < 1 {
			m.refuse("count", fmt.Sprintf("%d: a role with no actors can do nothing", member.Count))
		}
		m.done()
		p.Venire = append(p.Venire, member)
	}
	if len(p.Venire) == 0 {
		r.refuse("venire", "no entries: a run with no actors can do nothing. This is a parse refusal, not one of validate's arms")
	}

	p.Stages = map[string]Stage{}
	if stages, ok := r.table("stages", true); ok {
		enabled := 0
		for _, name := range stages.keys() {
			s, _ := stages.table(name, true)
			st := Stage{Enabled: s.boolean("enabled", true)}
			if st.Enabled {
				enabled++
			}
			s.done()
			p.Stages[name] = st
		}
		if len(p.Stages) == 0 {
			stages.refuse("", "no stages declared")
		} else if enabled == 0 {
			stages.refuse("", "every stage is disabled: the run would do nothing")
		}
		stages.done()
	}

	p.Weights = Weights{Stage: map[string]map[string]int{}, Profile: map[string]map[string]int{}}
	if w, ok := r.table("weights", true); ok {
		p.Weights.Default = w.weightTable("default", true)
		if len(p.Weights.Default) == 0 {
			w.refuse("default", "no action kinds: the walk would have nothing to draw from")
		}
		for _, side := range []string{"stage", "profile"} {
			if t, ok := w.table(side, false); ok {
				for _, name := range t.keys() {
					into := p.Weights.Stage
					if side == "profile" {
						into = p.Weights.Profile
					}
					into[name] = t.weightTable(name, true)
				}
				t.done()
			}
		}
		w.done()
	}

	p.Profiles = map[string]Profile{}
	if profs, ok := r.table("profiles", false); ok {
		for _, name := range profs.keys() {
			pt, _ := profs.table(name, true)
			var prof Profile
			prof.Stages = pt.strings("stages", true)
			prof.Seeds = pt.integer("seeds", true)
			if prof.Seeds < 1 {
				pt.refuse("seeds", fmt.Sprintf("%d: a profile that runs no seed runs nothing", prof.Seeds))
			}
			prof.Budget = pt.duration("budget", true)
			pt.done()
			p.Profiles[name] = prof
		}
		profs.done()
	}

	for _, rb := range r.tables("rollback") {
		entry := Rollback{Provider: rb.str("provider", true), Covers: rb.strings("covers", true)}
		if len(entry.Covers) == 0 {
			rb.refuse("covers", "a provider that covers nothing: name the components it restores")
		}
		rb.done()
		p.Rollback = append(p.Rollback, entry)
	}

	if th, ok := r.table("thresholds", false); ok {
		// R15 NAMES THE METRICS, so this table is carried as read rather than
		// checked against a key set this phase would be inventing. `blocking`
		// is the one key fixed now, because the epic makes a coverage threshold
		// advisory until it is promoted and a project must be able to promote
		// it.
		//
		// THE WORDING HERE IS DELIBERATELY UNQUOTED. The first version put the
		// epic's phrase in quotation marks three words from a `v1`, and
		// tests/vocabulary read that as a quote attributed to the vocabulary
		// and refused -- correctly: the phrase is the epic's, and the quote
		// oracle cannot tell which v1 a nearby mention means.
		p.Thresholds = map[string]any{}
		for _, name := range th.keys() {
			p.Thresholds[name] = th.any(name)
		}
		if b, ok := p.Thresholds["blocking"]; ok {
			if _, isBool := b.(bool); !isBool {
				th.refuse("blocking", fmt.Sprintf("%T: blocking is true or false", b))
			}
		}
		th.done()
	}

	r.done()

	// CROSS-KEY REFUSALS, after every key has been read: each one needs two
	// parts of the file and so cannot be checked while reading either.
	for name, t := range p.Weights.Stage {
		if _, ok := p.Stages[name]; !ok {
			r.refuse("weights.stage."+name, "no stage of that name is declared")
		}
		r.weightsAgainstDefault("weights.stage."+name, t, p.Weights.Default)
	}
	for name, t := range p.Weights.Profile {
		if _, ok := p.Profiles[name]; !ok {
			r.refuse("weights.profile."+name, "no profile of that name is declared")
		}
		r.weightsAgainstDefault("weights.profile."+name, t, p.Weights.Default)
	}
	for name, prof := range p.Profiles {
		for _, s := range prof.Stages {
			if _, ok := p.Stages[s]; !ok {
				r.refuse("profiles."+name+".stages", "names stage "+s+", which is not declared")
			}
		}
	}

	sort.Slice(r.refusals, func(i, j int) bool {
		if r.refusals[i].Path != r.refusals[j].Path {
			return r.refusals[i].Path < r.refusals[j].Path
		}
		return r.refusals[i].Reason < r.refusals[j].Reason
	})
	if len(r.refusals) > 0 {
		return Report{Refusals: r.refusals}
	}
	return Report{Platform: p}
}

// weightsAgainstDefault is the completeness rule: `[weights.default]` must name
// every action kind, so an override naming one it does not is a refusal. The
// reason is resolution -- Weight falls through to the default, so a kind only
// an override names has no weight in any stage or profile that does not
// override it, which is a silent zero rather than a declared one.
func (r *reader) weightsAgainstDefault(path string, table, def map[string]int) {
	var missing []string
	for kind := range table {
		if _, ok := def[kind]; !ok {
			missing = append(missing, kind)
		}
	}
	sort.Strings(missing)
	for _, kind := range missing {
		r.refuse(path, "names action kind "+kind+", which [weights.default] does not: the default table must be complete")
	}
}

// --- the reader -----------------------------------------------------------

// reader walks one table, remembers which keys it read, and refuses the rest.
// It is the Undecoded() of a hand-written decoder, kept here so the strictness
// does not depend on which TOML library is wired in.
type reader struct {
	doc      map[string]any
	path     string
	read     map[string]bool
	refusals []Refusal
	parent   *reader
}

func (r *reader) refuse(key, reason string) {
	path := r.join(key)
	ref := Refusal{Path: path, Reason: reason}
	top := r
	for top.parent != nil {
		top = top.parent
	}
	top.refusals = append(top.refusals, ref)
}

func (r *reader) join(key string) string {
	switch {
	case r.path == "":
		return key
	case key == "":
		return r.path
	default:
		return r.path + "." + key
	}
}

func (r *reader) mark(key string) {
	if r.read == nil {
		r.read = map[string]bool{}
	}
	r.read[key] = true
}

// keys reports this table's keys, sorted, and marks none of them: a caller
// iterating keys is responsible for reading each one.
func (r *reader) keys() []string {
	out := make([]string, 0, len(r.doc))
	for k := range r.doc {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// done refuses every key nothing read. UNKNOWN IS AN ERROR, not a skip, for
// docs/coverage.toml's reason: a lenient reader turns one mistyped key into a
// platform that runs with a weight, a stage or a provider the project believes
// it declared.
func (r *reader) done() {
	for _, k := range r.keys() {
		if !r.read[k] {
			r.refuse(k, "unknown key")
		}
	}
}

func (r *reader) any(key string) any {
	r.mark(key)
	return r.doc[key]
}

func (r *reader) str(key string, required bool) string {
	r.mark(key)
	v, ok := r.doc[key]
	if !ok {
		if required {
			r.refuse(key, "missing key")
		}
		return ""
	}
	s, ok := v.(string)
	if !ok {
		r.refuse(key, fmt.Sprintf("%T: wanted a string", v))
		return ""
	}
	return s
}

func (r *reader) boolean(key string, required bool) bool {
	r.mark(key)
	v, ok := r.doc[key]
	if !ok {
		if required {
			r.refuse(key, "missing key")
		}
		return false
	}
	b, ok := v.(bool)
	if !ok {
		r.refuse(key, fmt.Sprintf("%T: wanted true or false", v))
		return false
	}
	return b
}

func (r *reader) integer(key string, required bool) int {
	r.mark(key)
	v, ok := r.doc[key]
	if !ok {
		if required {
			r.refuse(key, "missing key")
		}
		return 0
	}
	n, ok := asInt(v)
	if !ok {
		r.refuse(key, fmt.Sprintf("%T: wanted an integer", v))
		return 0
	}
	return n
}

// asInt accepts the integer types a TOML reader may hand over. A float is
// REFUSED rather than truncated: 2.5 actors is a mistake in the file and
// rounding it would carry the mistake into the run.
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

func (r *reader) duration(key string, required bool) time.Duration {
	s := r.str(key, required)
	if s == "" {
		return 0
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		r.refuse(key, fmt.Sprintf("%q: wanted a duration such as 5m or 2h", s))
		return 0
	}
	if d <= 0 {
		r.refuse(key, fmt.Sprintf("%q: a budget of zero or less runs nothing", s))
		return 0
	}
	return d
}

func (r *reader) strings(key string, required bool) []string {
	r.mark(key)
	v, ok := r.doc[key]
	if !ok {
		if required {
			r.refuse(key, "missing key")
		}
		return nil
	}
	items, ok := v.([]any)
	if !ok {
		if ss, ok := v.([]string); ok {
			return ss
		}
		r.refuse(key, fmt.Sprintf("%T: wanted an array of strings", v))
		return nil
	}
	out := make([]string, 0, len(items))
	for i, item := range items {
		s, ok := item.(string)
		if !ok {
			r.refuse(fmt.Sprintf("%s[%d]", key, i), fmt.Sprintf("%T: wanted a string", item))
			continue
		}
		out = append(out, s)
	}
	return out
}

func (r *reader) table(key string, required bool) (*reader, bool) {
	r.mark(key)
	v, ok := r.doc[key]
	if !ok {
		if required {
			r.refuse(key, "missing table")
		}
		return &reader{doc: map[string]any{}, path: r.join(key), parent: r}, false
	}
	t, ok := v.(map[string]any)
	if !ok {
		r.refuse(key, fmt.Sprintf("%T: wanted a table", v))
		return &reader{doc: map[string]any{}, path: r.join(key), parent: r}, false
	}
	return &reader{doc: t, path: r.join(key), parent: r}, true
}

// tables reads an array of tables -- `[[venire]]`, `[[rollback]]`. An absent
// key is an empty list and not a refusal; the CALLER decides whether empty is
// allowed, because venire's empty is fatal and rollback's is not.
func (r *reader) tables(key string) []*reader {
	r.mark(key)
	v, ok := r.doc[key]
	if !ok {
		return nil
	}
	items, ok := v.([]any)
	if !ok {
		r.refuse(key, fmt.Sprintf("%T: wanted an array of tables", v))
		return nil
	}
	var out []*reader
	for i, item := range items {
		t, ok := item.(map[string]any)
		if !ok {
			r.refuse(fmt.Sprintf("%s[%d]", key, i), fmt.Sprintf("%T: wanted a table", item))
			continue
		}
		out = append(out, &reader{doc: t, path: fmt.Sprintf("%s[%d]", r.join(key), i), parent: r})
	}
	return out
}

// weightTable reads one weights table: action kind to a non-negative integer.
// Zero is legal and means never drawn; a negative weight is refused, because a
// weighted draw cannot express it and silently treating it as zero would make
// the file say something it does not mean.
func (r *reader) weightTable(key string, required bool) map[string]int {
	t, ok := r.table(key, required)
	if !ok {
		return nil
	}
	out := map[string]int{}
	for _, kind := range t.keys() {
		n := t.integer(kind, true)
		if n < 0 {
			t.refuse(kind, fmt.Sprintf("%d: a weight cannot be negative", n))
			continue
		}
		out[kind] = n
	}
	t.done()
	return out
}
