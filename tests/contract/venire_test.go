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

	"forge.axonibyte.com/axonibyte/rumor/internal/decl"
	"forge.axonibyte.com/axonibyte/rumor/internal/platform"
)

// appendingProfiles is every committed profile, in every example package, that
// appends venire rows -- `package/profile`, sorted.
//
// IT IS EMPTY, AND THAT IS THE CLAIM rather than a placeholder waiting to be
// filled: no committed project appends a row yet. The browser-context role
// fable ruled at rumor/1660 lands under examples/toy's `[profiles.viewing]`,
// and the commit that adds it moves this literal in the same change. Stating
// the SET rather than a count is what makes that commit's diff say which
// profile gained a pool, and this list is why a reviewer does not have to take
// the commit message's word for it.
var appendingProfiles = []string{}

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
