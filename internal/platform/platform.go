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
	"strconv"
	"strings"
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

	// ZoneEnv is the NAME of the environment variable through which the
	// engine tells the adapter what zone the system under test is configured
	// for (R7.5 D4c; fable's ruling at coop rumor/1780, "the zone rides the
	// PROFILE, not the row").
	//
	// THE PROJECT NAMES IT AND THE PLATFORM SUPPLIES THE VALUE, which is the
	// shape a rollback provider's `AddrEnv` already has: the provider names
	// TOY_ADDR and cmd/rumor sets it to the world it started. Here the project
	// names the variable and the engine sets it from `[clock].zone`, so a
	// promoted seed replays the world its own declarations describe -- without
	// it the row would need a token seeds.lock's grammar does not carry
	// (ibis's finding at rumor/1777, and r7/109 keeps that grammar).
	//
	// WHY A NAME AND NOT A CONVENTION: "TOY_ZONE" is the toy's spelling, and
	// an engine that knew it would hold the kind of system-under-test
	// knowledge this project keeps out of the engine -- the same reason the
	// concurrency stage reads its subject from a declaration rather than
	// deriving it. OPTIONAL, because a project whose system takes no zone
	// declares none.
	ZoneEnv string
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

	// ReconcileEvery is `[stages.<name>].reconcile_every`: how many steps
	// apart the walk re-checks what it is holding, instead of only after the
	// last step.
	//
	// A STAGE DECLARING NONE READS 0 AND IS WALKED EXACTLY AS BEFORE, which
	// is what keeps every R5-R7 measurement's meaning.
	//
	// IT IS A COUNT OF STEPS AND NOT A DURATION, deliberately. A period in
	// time would make one seeded walk replay differently on a slower box,
	// which is precisely what R5's ruling that NOTHING IS DRAWN exists to
	// prevent -- so the period is declared, counted in steps, and the
	// evidence it leaves is a count and an order rather than an elapsed time.
	ReconcileEvery int

	// ShrinkCandidates is `[stages.<name>].shrink_candidates`: how many
	// candidates a shrink of this stage's finding may replay before it stops
	// where it got to (issue 124's deliverable 4, fable's ruling at coop
	// r9/204). Absent, it is DefaultShrinkCandidates.
	//
	// A COUNT AND NOT A DURATION, for ReconcileEvery's reason above: one seed
	// must shrink to the same reproduction on any box, and a clock would let a
	// slower box stop sooner.
	ShrinkCandidates int
}

// DefaultShrinkCandidates is a stage's shrink budget when it declares none.
//
// READ FROM THE TOY'S OWN SHRINKS, measured 2026-11-13 (coop r9/205): the
// promoted seeds needed 6 to 15 candidates (walking 2 and 40: 8 and 7; settling
// 12: 15; expiring 3: 14; dst-spring 1 and dst-fall 2: 6 each), and the viewing
// and clicking bugs 10 to 14. The largest is 15, settling 12 reducing 24 steps
// to 19. 32 is about twice that, so every shrink the ledger's bugs needed
// finishes under the default with room, and a longer trace still stops rather
// than running on.
//
// WHAT IT COSTS UNDER A BROWSER, measured the same day: a candidate is a walk
// plus one session per actor, 1.1-1.4s a session quiet and 1.9-2.6s loaded, so
// about 30s a candidate on a loaded 4-core box and 32 of them about 15 minutes.
// Nothing ends a shrink sooner than its count -- `rumor run` has no clock (R15
// owns budgets, coop r9/206-208) -- so a project that cannot spend that lowers
// the number, and the shrink says how far it got.
const DefaultShrinkCandidates = 32

// Weights are the walk's weights: a complete default table and sparse
// overrides per stage and per profile.
type Weights struct {
	Default map[string]int
	Stage   map[string]map[string]int
	Profile map[string]map[string]int
}

// Profile is R22's: which stages to run, how many seeds, and the budget.
type Profile struct {
	// ReconcileEvery is `[profiles.<name>].reconcile_every`, and it is the
	// level fable ruled at coop rumor/1619: the same period the api stage's
	// table can carry, declarable per profile so a project can reconcile in
	// ONE declared world without moving any measurement taken in the others.
	//
	// WHY THE LEVEL EXISTS AT ALL. R8/111 found the period declared by no
	// project in the tree -- a knob with no declaration beside a field with no
	// reader. Putting it on `[stages.api]` to fix that would have reached every
	// run of examples/toy under every profile, which is the release control's
	// five digest rows, all five promoted seeds (two of them ABOUT
	// reconciliation), and D10's precondition -- rook's finding at r8/125, and
	// the sharpest item on the list, because an arm whose precondition has
	// quietly gone does not announce it.
	ReconcileEvery int

	// ClockStart is `[profiles.<name>].clock_start`, the instant a run of THIS
	// profile starts its clock at -- the level fable ruled at coop r7/109, and
	// the fourth to take the shape the period, the venire and the observations
	// took in 111.
	//
	// WHY A PROFILE AND NOT THE PROJECT ALONE. R7.5 needs two worlds in one
	// project: one starting in the day before the spring transition and one
	// before the fall, because a single start in March cannot reach November in
	// a walk any budget allows. Declared at the project level only, the second
	// world would have to move the first -- which is a committed reading.
	//
	// ZERO IS ABSENT, as it is for the period: a profile declaring 0 declares
	// nothing and takes the project's `[clock].start`, then the epoch. The
	// opposite reading would make 0 the one instant a profile could not inherit.
	ClockStart int64

	// Venire is `[profiles.<name>].venire`: rows this profile ADDS to the
	// platform's venire, and the level fable ruled at coop rumor/1660 on the
	// finding at r8/146.
	//
	// IT APPENDS AND CAN NEVER REPLACE, which is the whole of why a level here
	// is safe to give a project that already has committed evidence. A profile
	// declaring no rows draws the pool it always drew -- BY CONSTRUCTION and
	// not by a check -- so seeds.lock's five rows, the executed samples and
	// D10's precondition keep their meaning without this field being consulted
	// at all. That is the same argument the period's level rests on, and the
	// reason both are per profile rather than on `[[venire]]` itself: a row
	// added there would reach every run of every profile.
	Venire []Member

	// Observations is `[profiles.<name>].observations`: the observation NAMES
	// this profile adds to the project's, and the third declaration to be given
	// a profile level -- fable's ruling at coop rumor/1678 on R8/111's finding.
	//
	// NAMES AND NOT ROWS, which is the one way this declaration's shape differs
	// from the venire's, and the difference is which document the row lives in.
	// A venire row is this file's own vocabulary: `[[venire]]` is a table of
	// platform.toml, so a profile's copy of one is the same two keys in the same
	// document. What an observation IS -- its source, its claim type, its
	// arguments -- is bindings.toml's, which this package does not read and
	// cannot import, because internal/bindings reads THIS one and the import
	// would be a cycle. So a profile names what it takes and internal/bindings
	// keeps what the thing is, rather than this table learning a second
	// document's schema in order to hold a copy.
	//
	// A NAME NO PROFILE CLAIMS IS THE PROJECT'S, and that half of the rule is
	// Observes's to state rather than this field's.
	Observations []string

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

// PeriodLevel names where an effective period came from, because "4" and
// "where 4 came from" are different questions and only the second one tells a
// reader whether their own declaration is the one in force.
type PeriodLevel string

// The three levels, and `PeriodAbsent` is a value rather than an empty string
// for the reason `rumor report` exists: a level omitted when there is nothing
// to say cannot be told from a build that never learned to report one.
const (
	PeriodFromProfile PeriodLevel = "profile"
	PeriodFromStage   PeriodLevel = "stage"
	PeriodAbsent      PeriodLevel = "absent"
)

// ClockStartLevel names where an effective clock start came from, for
// PeriodLevel's reason read once more: an instant and where that instant was
// declared are different questions, and only the second tells a reader whether
// their own line is the one in force.
type ClockStartLevel string

// The three levels, and `ClockStartEpoch` is a value rather than an empty
// string so that "nothing declared one, so the run begins at the epoch" is a
// thing the report SAYS rather than a silence a reader has to interpret.
const (
	ClockStartFromProfile ClockStartLevel = "profile"
	ClockStartFromClock   ClockStartLevel = "clock"
	ClockStartEpoch       ClockStartLevel = "epoch"
)

// ClockStartFor resolves the instant a run begins at, and it is the ONE place
// the precedence is stated -- fable's ruling at coop r7/109: the profile, then
// the project's `[clock].start`, then the epoch.
//
// THE PROJECT'S DEFAULT ARRIVES AS AN ARGUMENT because it lives in the OTHER
// document. `[clock].start` is bindings.toml's and this package does not read
// bindings; passing the declared value in keeps the precedence in one function
// without making internal/platform depend on internal/bindings for one field.
// The caller that has both halves is `rumor run`, which is where the period's
// resolver is reached from too.
//
// ZERO IS ABSENT AT BOTH LEVELS, as it is for the period. A profile declaring
// `clock_start = 0` declares nothing and inherits; so does a project. The
// alternative -- zero as a declared epoch -- would make the epoch the one
// instant that cannot be inherited, and the epoch is exactly what inheriting
// the whole way down already gives.
func (p Platform) ClockStartFor(profile string, declared int64) (int64, ClockStartLevel) {
	if profile != "" {
		if pr, ok := p.Profiles[profile]; ok && pr.ClockStart != 0 {
			return pr.ClockStart, ClockStartFromProfile
		}
	}
	if declared != 0 {
		return declared, ClockStartFromClock
	}
	return 0, ClockStartEpoch
}

// ReconcileEvery resolves the walk's period for one profile, and it is the ONE
// place the precedence is stated -- fable's wording at coop rumor/1619.
//
// THE ORDER IS THE ONE Weight ALREADY HAS, deliberately: profile, then stage,
// then absent. A second precedence spelled differently from the first is how a
// tuned knob silently does nothing, which is the hazard Weight's own comment
// names, and this is the second knob to acquire levels.
//
// IT IS READ FROM THE api STAGE'S TABLE AND NO OTHER at the stage level,
// because the walk IS the api stage; a period under another stage's table is a
// period for a stage that does not walk, and `rumor report` names that as inert
// rather than printing it as though it worked.
//
// ZERO IS NOT A LEVEL. A profile declaring `reconcile_every = 0` is declaring
// no period, which reads as absent here rather than as a profile-level zero
// overriding a stage-level period -- the opposite reading would make `0` the
// one value that cannot be expressed at the stage level once any profile exists.
func (p Platform) ReconcileEvery(profile string) (int, PeriodLevel) {
	if profile != "" {
		if pr, ok := p.Profiles[profile]; ok && pr.ReconcileEvery > 0 {
			return pr.ReconcileEvery, PeriodFromProfile
		}
	}
	if st, ok := p.Stages[stageAPI]; ok && st.ReconcileEvery > 0 {
		return st.ReconcileEvery, PeriodFromStage
	}
	return 0, PeriodAbsent
}

// VenireFor is the actor pool one profile draws, and it is the ONE place the
// appending is stated -- fable's ruling at coop rumor/1660.
//
// IT APPENDS, IN ORDER, AND NEVER REPLACES. A profile's rows come after the
// platform's, so a profile naming a role the platform already names draws MORE
// actors of it rather than a different number. There is no expression in this
// file for "fewer", and that is deliberate rather than unfinished: a level that
// could shrink the committed pool could silently invalidate a committed seed,
// which is the one thing this level was added to be unable to do.
//
// A PROFILE THAT IS NOT DECLARED DRAWS THE COMMITTED POOL, as do one with no
// rows and the empty profile name. The resolver does not refuse an unknown
// profile: `rumor run --profile nonesuch` is stage.Plan's refusal and is made
// before this is reached, and answering again in a different voice would be two
// statements of one rule.
//
// THE RESULT IS ALWAYS A COPY, the no-rows path included. Handing a caller this
// platform's own slice lets one `append` write into the committed pool's spare
// capacity -- a mutation of what every other profile reads, from a call that
// reads like a read.
func (p Platform) VenireFor(profile string) []Member {
	var extra []Member
	if profile != "" {
		if pr, ok := p.Profiles[profile]; ok {
			extra = pr.Venire
		}
	}
	out := make([]Member, 0, len(p.Venire)+len(extra))
	out = append(out, p.Venire...)
	out = append(out, extra...)
	return out
}

// Observes answers whether a run of one profile takes the observation named,
// and it is the ONE place the observations level's rule is stated -- fable's
// ruling at coop rumor/1678.
//
// IT APPENDS, LIKE THE OTHER TWO, and it is a predicate rather than a slice
// because the rows are bindings.toml's: a caller holds the declared
// observations already and needs to know which of them this profile's walk
// makes. internal/schedule's ObservationsFor is that caller, and the only one.
//
// A NAME NO PROFILE CLAIMS REACHES EVERY PROFILE, which is the whole of why
// this level is safe to give a project that already has committed evidence.
// Every observation declared before this ruling is claimed by no profile, so it
// is taken by every run exactly as it was -- BY CONSTRUCTION and not by a
// check, which is the argument the period's level and the venire's both rest
// on.
//
// A NAME SOME PROFILE CLAIMS IS NOT THE PROJECT'S, and that is the half that
// makes the level mean anything. Without it a profile's declaration could only
// ADD a row already reaching every walk, so declaring `page_count` under
// `[profiles.viewing]` would still open a browser session in every committed
// run -- measured at coop rumor/1675 as six sessions where the committed
// reading has four, and tests/clock's control failing with it. A name is the
// project's or it is its claimants', never both.
//
// TWO PROFILES MAY CLAIM ONE NAME, and both take it. There is no expression
// here for "no profile may want what another wants", and inventing one would
// refuse a correct project for a coincidence.
//
// AN UNDECLARED PROFILE AND THE EMPTY NAME TAKE THE PROJECT'S OBSERVATIONS, as
// VenireFor's callers get the committed pool, and for that resolver's reason: a
// profile name no platform declares is stage.Plan's refusal, made before this
// is reached.
func (p Platform) Observes(profile, observation string) bool {
	claimed := false
	for name, pr := range p.Profiles {
		for _, o := range pr.Observations {
			if o != observation {
				continue
			}
			if name == profile && profile != "" {
				return true
			}
			claimed = true
		}
	}
	return !claimed
}

// stageAPI is internal/stage's API constant, spelled here because this package
// cannot import that one: internal/stage reads platform.Platform, so the
// dependency runs the other way and importing back would be a cycle.
//
// THE COPY IS JOINED TO THE ORIGINAL BY A TEST AND BY NOTHING ELSE. A rename of
// the stage would leave this resolver reading a table no project declares --
// every period would resolve to `absent`, every project would report itself as
// not reconciling, and nothing would fail to build. That is the shape of the
// cast tests/contract/source_test.go already arms between wire and ledger, so
// the pin lives beside it, in the one package that may import both.
const stageAPI = "api"

// WalkingStageName exports the copy above for that pin alone. It is a function
// rather than an exported constant so that nothing in the product can read the
// stage's name from here instead of from internal/stage, which would make the
// copy load-bearing where it is meant to be a shadow.
func WalkingStageName() string { return stageAPI }

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

// Load reads a platform.toml and parses it. The decoder is the caller's, for
// internal/decl's reason: the TOML dependency is one decision and this package
// does not need to know which way it went.
//
// THE TWO ERROR SHAPES ARE DIFFERENT ON PURPOSE. A file that cannot be read at
// all comes back as an error -- could-not-run. A file that reads and is not a
// platform comes back as a Report full of refusals, which is a verdict on the
// project. `rumor validate`'s exit statuses turn on exactly that distinction.
func Load(path string, dec decl.Decoder) (Report, error) {
	doc, err := decl.Load(path, dec)
	if err != nil {
		return Report{}, err
	}
	return Parse(doc), nil
}

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
		// AND THE NAME THAT CARRIES THE ZONE, refused when `env` does not
		// pass it through: a variable the engine sets and the adapter never
		// receives is a declaration with no effect, which is the
		// declared-and-read-by-nothing shape this file refuses elsewhere.
		p.Adapter.ZoneEnv = a.Str("zone_env", false)
		if p.Adapter.ZoneEnv != "" {
			var passed bool
			for _, name := range p.Adapter.Env {
				if name == p.Adapter.ZoneEnv {
					passed = true
					break
				}
			}
			if !passed {
				r.Refuse("adapter.zone_env", fmt.Sprintf("%q is not in [adapter].env, so the "+
					"engine would set a variable the adapter never receives: `env` is the list "+
					"of names passed through to it", p.Adapter.ZoneEnv))
			}
		}
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
			st := Stage{
				Enabled:          s.Bool("enabled", true),
				ReconcileEvery:   s.Int("reconcile_every", false),
				ShrinkCandidates: DefaultShrinkCandidates,
			}
			// UNDER ONE IS REFUSED BY NAME: the gate is a candidate, so a
			// budget of 0 would stop every shrink before it learned whether the
			// failure reproduces at all, and say so as if it had tried.
			if s.Has("shrink_candidates") {
				st.ShrinkCandidates = s.Int("shrink_candidates", false)
				if st.ShrinkCandidates < 1 {
					s.Refuse("shrink_candidates", fmt.Sprintf(
						"%d: the replay gate is itself a candidate, so a shrink needs at least 1",
						st.ShrinkCandidates))
				}
			}
			// A NEGATIVE PERIOD IS REFUSED BY NAME rather than clamped: `n %
			// period` with a negative period never equals 0, so the stage
			// would silently reconcile only at the end while its declaration
			// said otherwise -- a knob that reads as set and does nothing.
			if st.ReconcileEvery < 0 {
				s.Refuse("reconcile_every", fmt.Sprintf(
					"%d: a period is a count of steps and cannot be negative", st.ReconcileEvery))
			}
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
			prof.ReconcileEvery = pt.Int("reconcile_every", false)
			// NEGATIVE IS REFUSED BY NAME, the same way the stage table's is
			// and for its reason: `n % period` never reaches 0 for a negative,
			// so the walk would reconcile only at the end while the
			// declaration said otherwise -- a knob that reads as set and does
			// nothing.
			if prof.ReconcileEvery < 0 {
				pt.Refuse("reconcile_every", fmt.Sprintf(
					"%d: a period is a count of steps and cannot be negative", prof.ReconcileEvery))
			}
			// AND A PERIOD ON A PROFILE THAT DOES NOT RUN THE WALK IS REFUSED
			// BY NAME, which is fable's clause at coop rumor/1619 and the one
			// new refusal this level needs.
			//
			// The walk IS the api stage, so a period on a profile whose stage
			// list omits api is a period nothing will read -- exactly the inert
			// declaration `rumor report` names on a non-walking STAGE, except
			// that here it can be refused outright: a stage table is shared by
			// every profile and cannot be judged alone, where a profile's own
			// stage list says whether that profile walks.
			//
			// REFUSED RATHER THAN REPORTED because the two cases differ in who
			// can still be right. `[stages.concurrency].reconcile_every` may be
			// read by a future stage that walks; a profile selecting no api
			// stage will never walk under its own declaration, so there is no
			// reading on which the period means anything.
			if prof.ReconcileEvery > 0 {
				walks := false
				for _, st := range prof.Stages {
					if st == stageAPI {
						walks = true
						break
					}
				}
				if !walks {
					pt.Refuse("reconcile_every", fmt.Sprintf(
						"%d: this profile's stages are %s, which do not include %q -- the walk IS "+
							"that stage, so a period declared here is one no run of this profile "+
							"can read", prof.ReconcileEvery,
						strings.Join(prof.Stages, ", "), stageAPI))
				}
			}
			// AND THE INSTANT A RUN OF THIS PROFILE STARTS ITS CLOCK AT
			// (R7.5, fable's ruling at coop r7/109). Read at 64 bits for
			// `decl.Int64`'s reason -- nanoseconds since the epoch do not fit
			// in 32 -- and optional, so every profile committed before this
			// level keeps the start it had.
			//
			// NEGATIVE IS NOT REFUSED, AND THAT IS NOT AN OMISSION: an instant
			// before 1970 is a world a project may want, and the clock's own
			// refusal is about moving BACKWARD rather than about where it
			// began. The period's negative refusal exists because `n % period`
			// can never reach zero; there is no such arithmetic here.
			prof.ClockStart = pt.Int64("clock_start", false)
			// AND A START ON A PROFILE THAT DOES NOT RUN THE WALK IS REFUSED BY
			// NAME, which is the period's clause read for this level and
			// fable's word at r7/109. The walk IS the api stage, so a start
			// declared by a profile whose stage list omits api is an instant no
			// run of that profile can begin at -- refusable here for the
			// period's reason: a profile's own `stages` list says whether it
			// walks, where a stage table shared by every profile cannot be
			// judged alone.
			if prof.ClockStart != 0 {
				walks := false
				for _, st := range prof.Stages {
					if st == stageAPI {
						walks = true
						break
					}
				}
				if !walks {
					pt.Refuse("clock_start", fmt.Sprintf(
						"%d: this profile's stages are %s, which do not include %q -- the walk IS "+
							"that stage, so a start declared here is an instant no run of this "+
							"profile can begin at", prof.ClockStart,
						strings.Join(prof.Stages, ", "), stageAPI))
				}
			}
			// AND THE OBSERVATIONS THIS PROFILE TAKES, by name (fable, coop
			// rumor/1678). Not required: a profile declaring none takes the
			// project's observations and nothing else, which is every profile
			// committed before this level existed.
			//
			// NO REFUSAL HERE, AND THE ASYMMETRY WITH THE PERIOD IS DELIBERATE.
			// The period's two refusals are both answerable inside this file: a
			// negative count is wrong on its face, and whether this profile walks
			// is its own `stages` list. Every way an observation name can be wrong
			// needs bindings.toml -- the name may be declared by nothing, or
			// declared with a source no stage this profile selects can drive --
			// and a pairing across two documents is what an arm is for. Arm 19
			// carries both, beside the venire cell it already carries for the same
			// reason.
			prof.Observations = pt.Strings("observations", false)
			// THE APPENDED ROWS ARE READ THE WAY THE PLATFORM'S OWN ARE, so a
			// malformed row is refused in the same words, and keyed by the
			// profile -- which is what makes the refusal name the row a reader
			// has to go and edit rather than "a venire entry".
			//
			// THERE IS NO "no entries" REFUSAL HERE, and the asymmetry with the
			// platform's venire is the point: an empty `[[venire]]` at the top
			// level is a run with no actors, while a profile declaring none is
			// the ordinary case and draws the committed pool.
			for _, m := range pt.Tables("venire") {
				var member Member
				member.Role = m.Str("role", true)
				member.Count = m.Int("count", true)
				if member.Count < 1 {
					m.Refuse("count", fmt.Sprintf("%d: a role with no actors can do nothing", member.Count))
				}
				m.Done()
				prof.Venire = append(prof.Venire, member)
			}
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

// SeedLine is one line of a seeds file: the promoted seed, the tokens beside it
// and the comment after it.
//
// THE GRAMMAR IS docs/contract.md's AND THE PARSE IS IN ONE PLACE. FOUR readers
// had their own copy of "split on the first #, trim, the rest is the seed" --
// validate's arm 10b, internal/shrink's promotion check, tests/run's
// promoted-seed arm, and the document -- and they failed in two directions:
//
//	tests/run's arm   LOUD:  ParseUint on the whole remaining text, so
//	                         `12 profile=settling` fails "is not one
//	                         non-negative integer per line"
//	validate's 10b    LOUD:  the same, as a refusal
//	internal/shrink   QUIET: it compares the whole non-comment text to the
//	                         seed's digits, so that line reads as "seed 12 is
//	                         not here" and a re-promotion appends a SECOND
//	                         line for it
//	the document      neither: prose cannot fail, which is why it is the one
//	                         reader allowed its own copy
//
// THIS CENSUS SAID THREE AND OMITTED tests/run's, which rook caught at coop
// rumor/1103 by appending the token and watching that arm fail. The omission
// inverted the interesting part: it counted the reader that cannot fail and
// left out the one that fails loudest. The FIX was complete either way -- rook
// drove it at /1102 by breaking this function and requiring every dependent to
// die, four packages and four deaths with no survivor -- so what was one short
// was the record of what it fixed, which is the half a reader acts on.
type SeedLine struct {
	// Seed is the digits as written, unparsed: the grammar is the document's
	// and a caller that wants a number says so.
	Seed string

	// Profile is the `profile=NAME` token's value, EMPTY meaning the default
	// weights -- which is what every line written before this field existed
	// means, so no committed seed changes meaning by this arriving.
	Profile string

	// Steps is the `steps=N` token's value, ZERO meaning the line does not say
	// and the replaying caller's own default stands.
	//
	// IT IS ON THE LINE BECAUSE THE LENGTH IS A FACT ABOUT THE ROW, which is
	// ratchet's finding at coop r6/106: the replay arm held `24` for the one
	// promoted seed that needed it, so "the length is per-seed" was true of
	// the comment and false of the code -- the next profiled promotion would
	// inherit 24 silently, and a row needing another length would read as a
	// LOST PROMOTION rather than as a seed replayed too short. A fact about
	// one seed, living in the arm that replays all of them.
	Steps int

	// Witness is the `witness=WORD` token's value, EMPTY meaning the row is a
	// promotion of a DEFECT -- which is what every line written before this
	// field existed means, so no committed seed changes meaning by this
	// arriving, exactly as `profile=` was added.
	//
	// IT EXISTS BECAUSE A ROW CAN CARRY SOMETHING THAT IS NOT A BUG. seeds.lock
	// is defined by docs/contract.md as lines ending in "a comment naming the
	// DEFECT it found", and the arm that replays every row requires exit 3 of
	// each. A seed proving that a pending was held and then CLEARED exits 0
	// and finds nothing -- that is the point of it -- so adding one broke the
	// arm on its own terms (ibis, coop rumor/1238) rather than being carried
	// by the format. fable ruled the token at rumor/1241: a row that says what
	// it WITNESSES is replayed against that, and a row that says nothing keeps
	// exit 3 and its defect comment unchanged.
	//
	// ONE WORD TODAY. `cleared` is a pending held and then found consistent.
	// An unknown word is REFUSED by name with the known set printed, for the
	// reason the unknown-token refusal below gives: a word this build does not
	// know is a row asking for a check it will not get.
	Witness string

	// Comment is the text after `#`, trimmed. A promotion carries one naming
	// the defect it caught, which is what separates a regression suite from a
	// list of numbers.
	Comment string
}

// WitnessCleared is the one witness word this build knows: the row proves a
// pending was held and then found consistent, so the run exits 0 and finds
// nothing. docs/vocabulary.md's "`pending` until the declared probes read
// quiet" had no committed run to read it from until seed 66 (coop rumor/1241).
const WitnessCleared = "cleared"

// Witnesses is the known set, in the order a refusal prints it. IT IS A
// FUNCTION AND NOT A LITERAL IN THE MESSAGE so the refusal cannot drift from
// the check: a message listing words the parser no longer accepts is worse
// than no list, because a reader fixes their line to match it.
func Witnesses() []string { return []string{WitnessCleared} }

// KnownWitness reports whether the word is one this build checks a row against.
func KnownWitness(word string) bool {
	for _, w := range Witnesses() {
		if w == word {
			return true
		}
	}
	return false
}

// ParseSeedLine reads one line. The second return is whether the line carries a
// seed at all -- blank and comment-only lines carry none and are not errors --
// and the third is why a line that tried to be a seed is not one.
func ParseSeedLine(line string) (SeedLine, bool, error) {
	var out SeedLine
	t := strings.TrimSpace(line)
	if t == "" || strings.HasPrefix(t, "#") {
		return out, false, nil
	}
	if h := strings.Index(t, "#"); h >= 0 {
		out.Comment = strings.TrimSpace(t[h+1:])
		t = strings.TrimSpace(t[:h])
	}
	fields := strings.Fields(t)
	if len(fields) == 0 {
		// A LINE WHOSE ONLY CONTENT WAS A TRAILING COMMENT MARKER, which is a
		// comment line written confusingly rather than a broken seed.
		return out, false, nil
	}
	out.Seed = fields[0]
	for _, tok := range fields[1:] {
		name, value, ok := strings.Cut(tok, "=")
		if !ok {
			return out, true, fmt.Errorf("%q is not a `name=value` token", tok)
		}
		switch name {
		case "steps":
			n, err := strconv.Atoi(value)
			if err != nil {
				return out, true, fmt.Errorf("`steps=%s` is not a number", value)
			}
			if n < 1 {
				// THE SAME REFUSAL `--steps 0` GIVES, in the same words: a
				// walk of no steps is not a walk, and a line asking for one
				// would replay nothing and report a promotion as checked.
				return out, true, fmt.Errorf("`steps=%d`: a walk of no steps is not a walk", n)
			}
			if out.Steps != 0 {
				return out, true, fmt.Errorf("two `steps=` tokens (%d and %d): a seed replays at one length", out.Steps, n)
			}
			out.Steps = n
		case "profile":
			if value == "" {
				// EMPTY IS REFUSED RATHER THAN READ AS THE DEFAULT, because
				// those are different requests: a line that MEANS the default
				// omits the token, and one that names nothing is a line whose
				// author was interrupted.
				return out, true, fmt.Errorf("`profile=` names no profile; omit the token for the default weights")
			}
			if out.Profile != "" {
				return out, true, fmt.Errorf("two `profile=` tokens (%q and %q): a seed runs under one", out.Profile, value)
			}
			out.Profile = value
		case "witness":
			if value == "" {
				return out, true, fmt.Errorf("`witness=` names nothing; omit the token for a row that promotes a defect")
			}
			if out.Witness != "" {
				return out, true, fmt.Errorf("two `witness=` tokens (%q and %q): a row witnesses one thing", out.Witness, value)
			}
			// THE WORD IS CHECKED HERE AND NOT BY THE CALLER, so every reader
			// of the grammar refuses the same set. A caller left to validate
			// it would be the second copy of the rule, which is how `steps`
			// came to be read two ways (ratchet, coop r6/106).
			if !KnownWitness(value) {
				return out, true, fmt.Errorf("`witness=%s` is not a witness this build knows; it knows %s",
					value, strings.Join(Witnesses(), ", "))
			}
			out.Witness = value
		default:
			// AN UNKNOWN TOKEN IS REFUSED AND NOT IGNORED. A token this build
			// does not know is a line asking for something it will not get,
			// and reading past it would run the seed under the default while
			// its author believed otherwise.
			return out, true, fmt.Errorf("%q is not a token this build knows; it knows `profile=`, `steps=` and `witness=`", name)
		}
	}
	return out, true, nil
}
