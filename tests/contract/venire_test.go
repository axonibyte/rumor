// The committed projects' actor pools, held to the pool their evidence was
// recorded against.
//
// WHY THIS FILE EXISTS. `[profiles.<name>].venire` appends rows to a project's
// venire for one profile (fable, coop rumor/1660, on the finding at r8/146),
// and the whole argument that such a level is safe to give a project that
// already has committed evidence is that a profile declaring NO rows draws the
// pool it always drew. internal/platform's resolver makes that true by
// construction; this is the oracle over the committed FILES, which is the half
// construction cannot cover -- a row added to a profile whose seeds were
// recorded without it changes what a replay draws, and changes it silently.
//
// THE SAME SHAPE c2492a2 GAVE THE PERIOD, which is what fable asked for: the
// level is checked against the committed projects rather than only against
// synthetic documents, because the hazard is in the committed files.
package contract_test

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/decl"
	"forge.axonibyte.com/axonibyte/rumor/internal/platform"
	"forge.axonibyte.com/axonibyte/rumor/internal/schedule"
)

// appendingProfiles is every committed profile, in every example package, that
// appends venire rows -- `package/profile`, sorted.
//
// IT HELD ONE ENTRY FROM R8.4, and it was written empty one commit earlier
// precisely so that this line would have to move: examples/toy's
// `[profiles.viewing]` is the browser-context role's world (fable, coop
// rumor/1660), and it is the only profile in the tree that appends anything.
// Stating the SET rather than a count is what makes this diff say WHICH profile
// gained a pool, so a reviewer does not have to take the commit message's word
// for it.
//
// WHAT IT STILL GUARANTEES, with an entry in it: every OTHER committed profile
// draws exactly the pool it always drew, and `viewing`'s own pool still BEGINS
// with the committed rows. A seed recorded against any other profile is
// untouched by construction, and this is the oracle over that claim.
var appendingProfiles = []string{"toy/viewing"}

func TestNoCommittedProfileQuietlyChangesItsActorPool(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(examplesDir, "*", "platform.toml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no platform to read: %v %v", files, err)
	}
	sort.Strings(files)

	var appending []string
	for _, path := range files {
		pkg := filepath.Base(filepath.Dir(path))
		rep, lerr := platform.Load(path, decl.TOML)
		if lerr != nil {
			t.Errorf("%s: %v", pkg, lerr)
			continue
		}
		if !rep.OK() {
			t.Errorf("%s: the platform does not parse: %v", pkg, rep.Refusals)
			continue
		}
		p := rep.Platform
		committed := poolOf(p.Venire)
		if committed == "" {
			t.Errorf("%s: an empty committed venire, which the parser refuses: %v", pkg, p.Venire)
			continue
		}

		profiles := make([]string, 0, len(p.Profiles))
		for name := range p.Profiles {
			profiles = append(profiles, name)
		}
		sort.Strings(profiles)
		for _, name := range profiles {
			drawn := poolOf(p.VenireFor(name))
			if len(p.Profiles[name].Venire) > 0 {
				appending = append(appending, pkg+"/"+name)
				// AN APPENDED POOL STILL BEGINS WITH THE COMMITTED ONE. This is
				// the resolver's ruling read off the committed file rather than
				// off the resolver: rows are added after, so a project cannot
				// express a profile that draws fewer or different actors.
				if !strings.HasPrefix(drawn, committed) {
					t.Errorf("%s/%s draws %q, which does not begin with the committed %q: "+
						"an appended row may only ADD to the pool", pkg, name, drawn, committed)
				}
				continue
			}
			if drawn != committed {
				t.Errorf("%s/%s declares no venire rows and draws %q, want the committed %q",
					pkg, name, drawn, committed)
			}
		}
	}

	sort.Strings(appending)
	if got, want := strings.Join(appending, " "), strings.Join(appendingProfiles, " "); got != want {
		t.Errorf("profiles appending venire rows: [%s], want [%s]: a profile that appends a row "+
			"draws a pool its committed seeds were not recorded against, so which profiles do "+
			"it is stated here and moves in the commit that does it", got, want)
	}
	t.Logf("PROOF %d platform file(s) read, %d profile(s) appending", len(files), len(appending))
}

// poolOf renders a venire in the order the file declares it, which is the order
// the draw reads. It is NOT cmd/rumor's renderer and deliberately so: a test
// sharing the product's formatter cannot tell a correct pool from a formatter
// that agrees with itself.
func poolOf(ms []platform.Member) string {
	parts := make([]string, 0, len(ms))
	for _, m := range ms {
		parts = append(parts, fmt.Sprintf("%d %s", m.Count, m.Role))
	}
	return strings.Join(parts, ", ")
}

// claimingProfiles is every committed profile, in every example package, that
// claims observation names -- `package/profile`, sorted.
//
// THE SIBLING OF appendingProfiles, and fable's ruling at coop rumor/1678 asked
// for it in those words: "the arm that holds the venire's promise -- the
// committed profiles still draw the committed pool, and now still observe the
// committed observations -- holds this too". One entry, examples/toy's
// `[profiles.viewing]`, which is the browser observation's world.
//
// WHY THE SET AND NOT A COUNT, for appendingProfiles' reason: this line moving
// is what makes a commit say WHICH profile took an observation out of the
// project's set, because a claim does both at once.
var claimingProfiles = []string{"toy/viewing"}

// committedObservations is the observations each committed platform's PROJECT
// declares -- the set every profile of that package observes -- stated per
// package and sorted.
//
// WHY A LITERAL AND NOT A DERIVATION, measured and not foreseen. The first cut
// of this test derived this set from the same files it was checking, so a claim
// on a committed name REMOVED that name from the set the check then ran over:
// the clause could not fail, and the comment beside it said it could. Driven at
// `[profiles.walking].observations = ["event_count"]`, only the stated-set
// clause below reported -- and that drive had used a name read off a grep that
// took the line after each `[[observation]]` header, which misses a row whose
// `name` sits under a comment. This literal was wrong in its first draft for
// the same reason and this test is what said so.
//
// WHAT MOVES IT: a project declaring a NEW project-level observation, which is
// a reviewable line. What must never move it quietly is a claim, and that is
// the whole point -- a claim that takes one of these names makes this set and
// the files disagree, which is the comparison below.
var committedObservations = map[string][]string{
	"toy":  {"event_count", "event_tally", "invitation_expiry", "store_event_count"},
	"twin": {"doc.count"},
}

// TestNoCommittedProfileQuietlyChangesWhatItObserves is the observations
// level's half of the promise above.
//
// THE HAZARD IS THE SUBTRACTION AND NOT THE ADDITION, which is what makes this
// a different oracle rather than a copy. An appended venire row can only ADD
// actors, by construction -- there is no expression in platform.toml for
// "fewer". A CLAIM is not like that: platform.Observes gives a claimed name to
// its claimants and takes it from everyone else, so one line under one profile
// can stop every other profile from making an observation its committed seeds
// were recorded with, and nothing in the resolver can refuse that. The
// committed files are the only place it shows.
func TestNoCommittedProfileQuietlyChangesWhatItObserves(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(examplesDir, "*", "platform.toml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no platform to read: %v %v", files, err)
	}
	sort.Strings(files)

	var claiming []string
	for _, path := range files {
		dir := filepath.Dir(path)
		pkg := filepath.Base(dir)
		prep, lerr := platform.Load(path, decl.TOML)
		if lerr != nil || !prep.OK() {
			t.Errorf("%s: the platform does not read: %v %v", pkg, lerr, prep.Refusals)
			continue
		}
		brep, berr := bindings.Load(filepath.Join(dir, "bindings.toml"), decl.TOML)
		if berr != nil || !brep.OK() {
			t.Errorf("%s: the bindings do not read: %v %v", pkg, berr, brep.Refusals)
			continue
		}
		p := prep.Platform

		// THE COMMITTED OBSERVATIONS, DERIVED FROM THE FILES AND NOT FROM THE
		// RESOLVER: a name no profile claims. Asking platform.Observes for this
		// would be asking the product whether it agrees with itself.
		claimed := map[string][]string{}
		for name, prof := range p.Profiles {
			for _, o := range prof.Observations {
				claimed[o] = append(claimed[o], name)
			}
		}
		var committed []string
		for _, o := range brep.Bindings.Observations {
			if len(claimed[o.Name]) == 0 {
				committed = append(committed, o.Name)
			}
		}
		sort.Strings(committed)

		// THE FILES AND THE STATED SET MUST AGREE, and this is the clause that
		// catches the subtraction. A claim on a committed name takes it out of
		// `committed` above, so the disagreement shows here rather than
		// nowhere.
		stated, ok := committedObservations[pkg]
		if !ok {
			t.Errorf("%s declares a platform and committedObservations does not name it: a "+
				"package nothing states is one this oracle passes over", pkg)
		}
		if strings.Join(committed, " ") != strings.Join(stated, " ") {
			t.Errorf("%s's unclaimed observations are [%s], stated as [%s]: a name that leaves "+
				"this set is one some profile has claimed, and every other profile's seeds "+
				"were recorded while it was still theirs", pkg,
				strings.Join(committed, " "), strings.Join(stated, " "))
		}

		profiles := make([]string, 0, len(p.Profiles))
		for name := range p.Profiles {
			profiles = append(profiles, name)
		}
		sort.Strings(profiles)
		for _, name := range profiles {
			var taken []string
			for _, o := range schedule.ObservationsFor(p, name, brep.Bindings) {
				taken = append(taken, o.Name)
			}
			sort.Strings(taken)

			want := append([]string{}, committed...)
			own := append([]string{}, p.Profiles[name].Observations...)
			if len(own) > 0 {
				claiming = append(claiming, pkg+"/"+name)
				want = append(want, own...)
				sort.Strings(want)
			}
			if strings.Join(taken, " ") != strings.Join(want, " ") {
				t.Errorf("%s/%s observes [%s], want [%s]: a profile takes the observations no "+
					"profile claims, plus its own claims, and nothing else",
					pkg, name, strings.Join(taken, " "), strings.Join(want, " "))
			}
			// AND EVERY COMMITTED OBSERVATION IS STILL TAKEN, read off the
			// STATED set and not the derived one. That is the whole difference
			// between this clause and the one it replaced: ranging over
			// `committed` asked whether the profile observes the names no
			// profile claims, which platform.Observes answers by construction
			// and cannot fail.
			for _, o := range stated {
				if !p.Observes(name, o) {
					t.Errorf("%s/%s does not observe %s, which no profile claims: a committed "+
						"observation is every profile's", pkg, name, o)
				}
			}
		}
	}

	sort.Strings(claiming)
	if got, want := strings.Join(claiming, " "), strings.Join(claimingProfiles, " "); got != want {
		t.Errorf("profiles claiming observations: [%s], want [%s]: a claim takes a name out of "+
			"every other profile's set, so which profiles make one is stated here and moves in "+
			"the commit that does it", got, want)
	}
	t.Logf("PROOF %d platform file(s) read, %d profile(s) claiming", len(files), len(claiming))
}
