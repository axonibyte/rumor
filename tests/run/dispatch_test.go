package run_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// THE DISPATCH, DRIVEN THROUGH THE BINARY: a project that enables the
// concurrency stage gets its fan-out, one that enables it alone gets no walk,
// and one that enables it with no world to contend in is told why.
//
// IT COPIES examples/toy, which is this package's own established fixture
// shape -- the hang-report arm drives "a copy of examples/toy whose
// `worker_idle` can never read quiet" the same way. The copy is still the
// subject for the two cells that need declarations the shipped toy does not
// have: `api` disabled, and no `[[rollback]]` provider.
//
// THE STAGE IS NO LONGER ADDED BY THE FIXTURE, AND THAT IS THE RULING. fable
// settled at coop rumor/1553 that examples/toy enables `concurrency` itself --
// a project declaring a contended action and not running the stage that
// contends it is the mechanism-not-stage void one tier down -- with the two
// unprofiled seeds.lock rows taking a stages-only profile so the promotion arm
// keeps reading the status. So the first cell edits NOTHING, and the fixture
// ASSERTS the declaration instead of writing it: an edit that added a key the
// toy already declares is a duplicate TOML key and a refusal, and an edit that
// silently added nothing would leave this arm reading a world it did not build.
func TestTheDispatchRunsTheConcurrencyStage(t *testing.T) {
	for _, c := range []struct {
		what     string
		edit     func(string) string
		walks    bool
		contends bool
		skipped  bool
		code     int
	}{
		{
			// NO EDIT AT ALL: the shipped toy enables both.
			what:     "both stages",
			edit:     requireConcurrency,
			walks:    true,
			contends: true,
			code:     3, // the stage finds the toy's capacity race; so does the walk, often
		},
		{
			// THE WALK RUNS ONLY WHEN `api` IS SELECTED, which through 108 it
			// did unconditionally: a project enabling another stage alone would
			// have walked anyway and read as a run of something nobody asked
			// for.
			what:     "the concurrency stage alone",
			edit:     func(s string) string { return apiOff(requireConcurrency(s)) },
			walks:    false,
			contends: true,
			code:     3,
		},
		{
			// A PROJECT WITH NO WORLD TO CONTEND IN IS TOLD WHY, and the run
			// does not fail: its declarations are correct and simply do not
			// reach a provider, which is shrinkFound's reading of the same
			// absence. D13's skip reasons are 111's to report; this is the line
			// that keeps the absence from being silent in the meantime.
			what:    "the concurrency stage with no rollback provider",
			edit:    func(s string) string { return noProvider(apiOff(requireConcurrency(s))) },
			walks:   false,
			skipped: true,
			code:    0,
		},
	} {
		t.Run(c.what, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range []string{"platform.toml", "bindings.toml", "statute.rego", "seeds.lock"} {
				b, err := os.ReadFile(filepath.Join("../../examples/toy", f))
				if err != nil {
					t.Fatalf("reading the toy's %s: %v", f, err)
				}
				body := string(b)
				if f == "platform.toml" {
					body = c.edit(body)
				}
				if err := os.WriteFile(filepath.Join(dir, f), []byte(body), 0o644); err != nil {
					t.Fatalf("writing %s: %v", f, err)
				}
			}
			out, errb, code := run(t, "run", dir, "--seed", "0", "--steps", "6")
			if code != c.code {
				t.Fatalf("exit %d, wanted %d\nstdout:\n%s\nstderr:\n%s", code, c.code, out, errb)
			}
			// THE WALK'S OWN EVIDENCE IS A TRACE LINE, not the absence of the
			// stage's: a run that walked prints a numbered step, and that is
			// what tells a selection that included `api` from one that did not.
			walked := strings.Contains(out, "run:   1 ")
			if walked != c.walks {
				t.Errorf("the walk %s and the selection %s it\nstdout:\n%s",
					did(walked), wanted(c.walks), out)
			}
			contended := strings.Contains(out, "concurrency: join_event -- setup ")
			if contended != c.contends {
				t.Errorf("the stage %s contend and the selection %s it to\nstdout:\n%s",
					did(contended), wanted(c.contends), out)
			}
			if c.contends {
				if !strings.Contains(out, "concurrency:   FOUND") {
					t.Errorf("the stage contended and reported no finding over a system whose "+
						"race it exists to find\nstdout:\n%s", out)
				}
				// THE MEASURED HALF IS ON STDERR AND NOWHERE ELSE. A duration
				// on stdout would make D1 unreachable for every run that
				// contends, and both halves are asserted so neither can
				// migrate silently.
				if !strings.Contains(errb, "run: concurrency join_event try ") ||
					!strings.Contains(errb, "overlapped by") {
					t.Errorf("the overlap is not reported on stderr, so the run's own evidence "+
						"that it was concurrent is missing\nstderr:\n%s", errb)
				}
				if strings.Contains(out, "overlapped by") {
					t.Errorf("stdout carries the measured overlap, so two attempts of one seed "+
						"could not be byte-identical\nstdout:\n%s", out)
				}
			}
			if c.skipped {
				if !strings.Contains(out, "concurrency: NOT CONTENDED -- ") ||
					!strings.Contains(out, "[[rollback]]") {
					t.Errorf("a stage that could not contend said so without naming what was "+
						"missing\nstdout:\n%s", out)
				}
			}
			t.Logf("PROOF exit %d\nstdout:\n%s\nstderr:\n%s", code, out, errb)
		})
	}
}

// The edits, each one line of intent over the toy's own platform.

// requireConcurrency changes nothing and asserts the shipped toy enables the
// stage, which is what every cell below rests on.
//
// IT IS A CHECK AND NOT AN EDIT because fable's ruling put the declaration in
// the project: adding it here would be a duplicate TOML key -- measured, the
// run refuses with "Key 'stages.concurrency.enabled' has already been
// defined" -- and an edit that found nothing to change would leave these cells
// driving a project with no stage to run while still reading as a dispatch arm.
func requireConcurrency(s string) string {
	if !strings.Contains(s, "concurrency = { enabled = true }") {
		panic("examples/toy no longer enables the concurrency stage, so these cells would drive " +
			"a project with no stage to contend with: fable ruled it in at coop rumor/1553")
	}
	return s
}

func apiOff(s string) string {
	return strings.Replace(s, "api = { enabled = true }", "api = { enabled = false }", 1)
}

// noProvider cuts the [[rollback]] block, which is the last thing in the toy's
// platform. It asserts it found one rather than returning the file unchanged: a
// silent no-op edit would leave the cell driving the provider path under a name
// that says it does not, which is a fixture builder that passes by doing
// nothing.
func noProvider(s string) string {
	i := strings.Index(s, "[[rollback]]")
	if i < 0 {
		panic("the toy declares no [[rollback]] block, so this edit removes nothing")
	}
	return s[:i]
}

func did(b bool) string {
	if b {
		return "did"
	}
	return "did NOT"
}

func wanted(b bool) string {
	if b {
		return "wanted"
	}
	return "did not want"
}
