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

	"forge.axonibyte.com/axonibyte/rumor/internal/decl"
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

// Refusal is one reason a document is not a platform: internal/decl's, because
// the rules for reading a declaration are shared with the bindings schema and a
// second copy of them would drift.
type Refusal = decl.Refusal

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

// THE FILE'S NAME IS THE LOADER'S TO ADD. A Refusal carries the key path and
// the reason and not "platform.toml:", because this package is handed a decoded
// document and does not know what it was read from -- the same refusal over a
// document built by a test has no filename at all.

// OK reports whether the document is a platform.
func (r Report) OK() bool { return len(r.Refusals) == 0 }

// Parse reads a decoded document. The argument is what a TOML reader produced:
// tables as map[string]any, arrays as []any, integers as int64 or int.
func Parse(doc map[string]any) Report {
	r := decl.New(doc)
	var p Platform

	p.Contract = r.Str("contract", true)
	p.SeedsFile = r.Str("seeds_file", false)
	if p.SeedsFile == "" {
		p.SeedsFile = DefaultSeedsFile
	}

	if a, ok := r.Table("adapter", true); ok {
		p.Adapter.Command = a.Strings("command", true)
		if len(p.Adapter.Command) == 0 {
			r.Refuse("adapter.command", "an empty argv: the engine would have nothing to spawn")
		}
		p.Adapter.Cwd = a.Str("cwd", false)
		p.Adapter.Env = a.Strings("env", false)
		a.Done()
	}

	for _, m := range r.Tables("venire") {
		var member Member
		member.Role = m.Str("role", true)
		member.Count = m.Int("count", true)
		if member.Count < 1 {
			m.Refuse("count", fmt.Sprintf("%d: a role with no actors can do nothing", member.Count))
		}
		m.Done()
		p.Venire = append(p.Venire, member)
	}
	if len(p.Venire) == 0 {
		r.Refuse("venire", "no entries: a run with no actors can do nothing. This is a parse refusal, not one of validate's arms")
	}

	p.Stages = map[string]Stage{}
	if stages, ok := r.Table("stages", true); ok {
		enabled := 0
		for _, name := range stages.Keys() {
			s, _ := stages.Table(name, true)
			st := Stage{Enabled: s.Bool("enabled", true)}
			if st.Enabled {
				enabled++
			}
			s.Done()
			p.Stages[name] = st
		}
		if len(p.Stages) == 0 {
			stages.Refuse("", "no stages declared")
		} else if enabled == 0 {
			stages.Refuse("", "every stage is disabled: the run would do nothing")
		}
		stages.Done()
	}

	p.Weights = Weights{Stage: map[string]map[string]int{}, Profile: map[string]map[string]int{}}
	if w, ok := r.Table("weights", true); ok {
		p.Weights.Default = weightTable(w, "default", true)
		if len(p.Weights.Default) == 0 {
			w.Refuse("default", "no action kinds: the walk would have nothing to draw from")
		}
		for _, side := range []string{"stage", "profile"} {
			if t, ok := w.Table(side, false); ok {
				for _, name := range t.Keys() {
					into := p.Weights.Stage
					if side == "profile" {
						into = p.Weights.Profile
					}
					into[name] = weightTable(t, name, true)
				}
				t.Done()
			}
		}
		w.Done()
	}

	p.Profiles = map[string]Profile{}
	if profs, ok := r.Table("profiles", false); ok {
		for _, name := range profs.Keys() {
			pt, _ := profs.Table(name, true)
			var prof Profile
			prof.Stages = pt.Strings("stages", true)
			prof.Seeds = pt.Int("seeds", true)
			if prof.Seeds < 1 {
				pt.Refuse("seeds", fmt.Sprintf("%d: a profile that runs no seed runs nothing", prof.Seeds))
			}
			prof.Budget = pt.Duration("budget", true)
			pt.Done()
			p.Profiles[name] = prof
		}
		profs.Done()
	}

	for _, rb := range r.Tables("rollback") {
		entry := Rollback{Provider: rb.Str("provider", true), Covers: rb.Strings("covers", true)}
		if len(entry.Covers) == 0 {
			rb.Refuse("covers", "a provider that covers nothing: name the components it restores")
		}
		rb.Done()
		p.Rollback = append(p.Rollback, entry)
	}

	if th, ok := r.Table("thresholds", false); ok {
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
		for _, name := range th.Keys() {
			p.Thresholds[name] = th.Any(name)
		}
		if b, ok := p.Thresholds["blocking"]; ok {
			if _, isBool := b.(bool); !isBool {
				th.Refuse("blocking", fmt.Sprintf("%T: blocking is true or false", b))
			}
		}
		th.Done()
	}

	r.Done()

	// CROSS-KEY REFUSALS, after every key has been read: each one needs two
	// parts of the file and so cannot be checked while reading either.
	for name, t := range p.Weights.Stage {
		if _, ok := p.Stages[name]; !ok {
			r.Refuse("weights.stage."+name, "no stage of that name is declared")
		}
		weightsAgainstDefault(r, "weights.stage."+name, t, p.Weights.Default)
	}
	for name, t := range p.Weights.Profile {
		if _, ok := p.Profiles[name]; !ok {
			r.Refuse("weights.profile."+name, "no profile of that name is declared")
		}
		weightsAgainstDefault(r, "weights.profile."+name, t, p.Weights.Default)
	}
	for name, prof := range p.Profiles {
		for _, s := range prof.Stages {
			if _, ok := p.Stages[s]; !ok {
				r.Refuse("profiles."+name+".stages", "names stage "+s+", which is not declared")
			}
		}
	}

	if found := r.Refusals(); len(found) > 0 {
		return Report{Refusals: found}
	}
	return Report{Platform: p}
}

// weightTable reads one weights table: action kind to a non-negative integer.
// Zero is legal and means never drawn; a negative weight is refused, because a
// weighted draw cannot express it and silently treating it as zero would make
// the file say something it does not mean.
func weightTable(r *decl.Reader, key string, required bool) map[string]int {
	t, ok := r.Table(key, required)
	if !ok {
		return nil
	}
	out := map[string]int{}
	for _, kind := range t.Keys() {
		n := t.Int(kind, true)
		if n < 0 {
			t.Refuse(kind, fmt.Sprintf("%d: a weight cannot be negative", n))
			continue
		}
		out[kind] = n
	}
	t.Done()
	return out
}

// weightsAgainstDefault is the completeness rule: `[weights.default]` must name
// every action kind, so an override naming one it does not is a refusal. The
// reason is resolution -- Weight falls through to the default, so a kind only
// an override names has no weight in any stage or profile that does not
// override it, which is a silent zero rather than a declared one.
func weightsAgainstDefault(r *decl.Reader, path string, table, def map[string]int) {
	var missing []string
	for kind := range table {
		if _, ok := def[kind]; !ok {
			missing = append(missing, kind)
		}
	}
	sort.Strings(missing)
	for _, kind := range missing {
		r.Refuse(path, "names action kind "+kind+", which [weights.default] does not: the default table must be complete")
	}
}
