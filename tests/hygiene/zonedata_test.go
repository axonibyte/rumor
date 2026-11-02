// Package hygiene's zone-database arm: R7.5's `[clock].zone` is refused when no
// zone database holds the name, so the engine's binary must CARRY one.
//
// THE PROPERTY IS ARMED AND NOT THE IMPORT, which is the distinction this arm
// exists for (ratchet and ibis, coop r7/96-/98). `cmd/rumor` imports
// `time/tzdata` explicitly, but the database was already linked in before that
// line was written: `github.com/open-policy-agent/opa/v1/topdown` imports it
// and cmd/rumor depends on OPA. So an arm that read the IMPORT would be
// checking the line rather than the guarantee, and an OPA bump that dropped
// its import would take the guarantee away with the line still there and
// nothing failing. ibis's rule at coop r6/173, read for a dependency: an import
// with no cell is armed by nothing.
//
// WHY IT MATTERS TO A SEED AND NOT ONLY TO A PARSE: the transition instants a
// walk lands on are derived from this data at walk time, and a replay is a
// RE-RUN from the seed rather than a read of a ledger -- `advanceStep` draws an
// INDEX into a derived slice (internal/schedule/walk.go), so a seed's committed
// trace moves with the database it was promoted against (ratchet coop r7/93,
// ibis r7/94, fable's corrected ruling r7/95). A binary that read the host's
// zoneinfo would make that version a property of whatever box ran it.
//
// WHAT THIS DOES NOT PROVE: which VERSION of the database is linked, or that
// the engine reads a zone at all. The version belongs to a promoted seed's own
// row, which is this child's later work, and nothing consumes `[clock].zone`
// yet.
package hygiene_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestTheEngineCarriesAZoneDatabase asks the TOOLCHAIN what the binary is built
// from, which is the instrument a grep over the tree cannot be: a transitive
// import is invisible to every search of first-party source, and that is how
// two of us answered this question wrongly before driving it (coop r7/96, /97).
func TestTheEngineCarriesAZoneDatabase(t *testing.T) {
	// ONE NAME FOR THE SUBJECT, so a message cannot say it read a package the
	// command did not: driving this arm at ./cmd/toy to prove it fires printed
	// "./cmd/rumor's 189 dependencies", which is the subject and the claim
	// disagreeing in the one line a reader acts on.
	const pkg = "./cmd/rumor"
	cmd := exec.Command(zoneGoBin(t), "list", "-deps", pkg)
	cmd.Dir = "../.."
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("`go list -deps %s` failed, so the build's surface is UNMEASURED "+
			"rather than clean: %v", pkg, err)
	}
	deps := strings.Split(strings.TrimSpace(string(out)), "\n")
	// A DENOMINATOR FIRST: a failed walk and a walk that found nothing both
	// print no tzdata, and only one of them is a finding about the build.
	if len(deps) < 50 {
		t.Fatalf("`go list -deps %s` named %d package(s), far short of this binary's "+
			"graph: a short listing reports no zone database just as a stripped build would",
			pkg, len(deps))
	}
	for _, d := range deps {
		if d == "time/tzdata" {
			t.Logf("PROOF `go list -deps %s` carries time/tzdata among %d package(s), so the "+
				"engine resolves an IANA name without reading the host's zoneinfo",
				pkg, len(deps))
			return
		}
	}
	t.Errorf("time/tzdata is in none of %s's %d dependencies: `[clock].zone` is "+
		"refused when no database holds the name, so without one linked in that refusal "+
		"becomes a statement about the BOX and a promoted seed's transition instants become "+
		"a property of whatever host ran it", pkg, len(deps))
}

// zoneGoBin resolves the toolchain the way tests/licences and docs/cli.md's run
// blocks do -- by name and in order, never a bare `go` assumed present, with
// the absence reported as UNMEASURED.
func zoneGoBin(t *testing.T) string {
	t.Helper()
	for _, c := range []string{"go", "go126", "go125"} {
		if p, err := exec.LookPath(c); err == nil {
			return p
		}
	}
	t.Fatal("no go, go126 or go125 on PATH: this arm cannot enumerate a build, which is " +
		"UNMEASURED and not a pass")
	return ""
}
