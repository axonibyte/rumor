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
// TWO BINARIES AS OF R7.5 D4, and the import means a DIFFERENT thing in each.
// `cmd/rumor` had the database linked in before its import was written, through
// OPA's `v1/topdown`, so there the explicit line is belt-and-braces and this
// arm guards the guarantee rather than the line. `cmd/toy` depends on no such
// package -- 190 dependencies, all stdlib and first-party -- so there the
// import IS the whole guarantee, and dropping it would leave the system under
// test reading the HOST's zoneinfo while the engine read the binary's. That
// disagreement would surface as a finding about the box.
//
// WHAT THIS DOES NOT PROVE: which VERSION of the database is linked, or that
// either binary reads a zone at all. The version belongs to a promoted seed's
// own row, which is this child's later work.
package hygiene_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/proof"
)

// TestTheEngineCarriesAZoneDatabase asks the TOOLCHAIN what the binary is built
// from, which is the instrument a grep over the tree cannot be: a transitive
// import is invisible to every search of first-party source, and that is how
// two of us answered this question wrongly before driving it (coop r7/96, /97).
func TestTheEngineCarriesAZoneDatabase(t *testing.T) {
	// EVERY MESSAGE NAMES ITS OWN SUBJECT, which is the trap this arm already
	// paid for once: driving the single-subject version at ./cmd/toy printed
	// "./cmd/rumor's 189 dependencies", the subject and the claim disagreeing
	// in the one line a reader acts on. The subject is now a loop variable and
	// every string below reads it.
	//
	// THE FLOOR IS PER-SUBJECT for the same reason the denominator exists at
	// all: cmd/rumor's graph is 402 packages and the toy's is 190, so one
	// shared floor would be either useless for the first or wrong for the
	// second. Each is set well under its measured size -- the guard is against
	// a listing that FAILED SHORT, not an assertion about the graph.
	for _, subject := range []struct {
		pkg   string
		floor int
		why   string
	}{
		{pkg: "./cmd/rumor", floor: 200,
			why: "the engine resolves `[clock].zone` and derives a walk's transition " +
				"landings from the BINARY's database rather than the host's"},
		{pkg: "./cmd/toy", floor: 100,
			why: "the system under test reads its own instants as local time (R7.5 D4) " +
				"from the BINARY's database, and no other package in its graph carries one"},
	} {
		t.Run(subject.pkg, func(t *testing.T) {
			cmd := exec.Command(zoneGoBin(t), "list", "-deps", subject.pkg)
			cmd.Dir = "../.."
			cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local")
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("`go list -deps %s` failed, so the build's surface is UNMEASURED "+
					"rather than clean: %v", subject.pkg, err)
			}
			deps := strings.Split(strings.TrimSpace(string(out)), "\n")
			// A DENOMINATOR FIRST: a failed walk and a walk that found nothing
			// both print no tzdata, and only one is a finding about the build.
			if len(deps) < subject.floor {
				t.Fatalf("`go list -deps %s` named %d package(s), short of the %d this "+
					"binary's graph holds: a short listing reports no zone database just as "+
					"a stripped build would", subject.pkg, len(deps), subject.floor)
			}
			for _, d := range deps {
				if d == "time/tzdata" {
					proof.Logf(t, "PROOF `go list -deps %s` carries time/tzdata among %d "+
						"package(s): %s", subject.pkg, len(deps), subject.why)
					return
				}
			}
			t.Errorf("time/tzdata is in none of %s's %d dependencies. The claim it carried "+
				"was: %s -- so without a database linked in, that reading becomes a "+
				"statement about the BOX, and a promoted seed's transition instants become "+
				"a property of whatever host ran it", subject.pkg, len(deps), subject.why)
		})
	}
}

// zoneGoBin resolves the toolchain the way tests/licences and docs/cli.md's run
// blocks do -- by name and in order, never a bare `go` assumed present, with
// the absence reported as UNMEASURED.
func zoneGoBin(t *testing.T) string {
	t.Helper()
	for _, c := range []string{"go127", "go126", "go125", "go"} {
		if p, err := exec.LookPath(c); err == nil {
			return p
		}
	}
	t.Fatal("no go127, go126, go125 or go on PATH: this arm cannot enumerate a build, which is " +
		"UNMEASURED and not a pass")
	return ""
}
