package run_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// THE FUZZ STAGE, DRIVEN THROUGH THE BINARY: a project that enables it with the
// walk gets both, one that enables it alone gets no walk, and one that declares
// no refusal text is told which key is missing.
//
// THE FIRST CELL IS THE ONE THAT WOULD HAVE CAUGHT A LEAK OLDER THAN THIS
// STAGE. Before 110, `concurrency` was the last runnable stage, so nothing ran
// after a stage that names a world in the PROCESS environment -- `[adapter].env`
// passes the variable through by name. shrinkFound set it per candidate and
// tore the world down in a defer without restoring it, and concurrencyStage did
// the same between fan-outs; `Unsetenv` appeared nowhere in cmd/rumor. Wiring
// `fuzz`, which Select orders after both, made it reachable: api+fuzz answered
// "connection refused" from an adapter reading a live-looking address for a
// dead world, while concurrency+fuzz and fuzz alone were clean. Those three
// runs are what placed it in the walk's shrink rather than the fan-out, and
// this cell is the one that fails if either restoration is removed.
//
// IT COPIES examples/toy, the shape dispatch_test.go established, and ASSERTS
// the declaration rather than writing it: adding a key the toy already declares
// is a duplicate TOML key and a refusal, and an edit that silently added
// nothing would leave the arm reading a world it did not build.
func TestTheDispatchRunsTheFuzzStage(t *testing.T) {
	for _, c := range []struct {
		what     string
		platform func(string) string
		bindings func(string) string
		walks    bool
		summary  bool
		vacuous  bool
		code     int
	}{
		{
			// NO EDIT: the shipped toy enables api and fuzz both, and this is
			// the ORDER that exposed the env leak.
			what:     "the walk and then the fuzz stage",
			platform: requireFuzz,
			walks:    true,
			summary:  true,
			code:     3, // the walk and the concurrency stage find the toy's planted defects
		},
		{
			// THE STAGE WITHOUT THE WALK, so the summary is not riding on the
			// walk having opened a session first.
			what:     "the fuzz stage alone",
			platform: func(s string) string { return concurrencyOff(apiOff(requireFuzz(s))) },
			walks:    false,
			summary:  true,
			code:     0, // the hostile corpus finds nothing on this toy
		},
		{
			// NO REFUSAL TEXT DECLARED is a COULD-NOT-RUN naming the key, not a
			// report of findings grounded in a key nobody declared: with no
			// `[refusal].message` the D3 and D4 clauses have no subject (ibis,
			// coop r8/29). 2 is "what an observation would have said is
			// unknown", which is exactly that.
			what:     "the fuzz stage with no refusal text declared",
			platform: func(s string) string { return concurrencyOff(apiOff(requireFuzz(s))) },
			bindings: cutRefusal,
			walks:    false,
			vacuous:  true,
			code:     2,
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
				if f == "platform.toml" && c.platform != nil {
					body = c.platform(body)
				}
				if f == "bindings.toml" && c.bindings != nil {
					body = c.bindings(body)
				}
				if err := os.WriteFile(filepath.Join(dir, f), []byte(body), 0o644); err != nil {
					t.Fatalf("writing %s: %v", f, err)
				}
			}
			out, errb, code := run(t, "run", dir, "--seed", "2", "--steps", "10")
			if code != c.code {
				t.Fatalf("exit %d, wanted %d\nstdout:\n%s\nstderr:\n%s", code, c.code, out, errb)
			}
			walked := strings.Contains(out, "run:   1 ")
			if walked != c.walks {
				t.Errorf("the walk %s and the selection %s it\nstdout:\n%s",
					did(walked), wanted(c.walks), out)
			}
			// THE SUMMARY IS THE STAGE's OWN EVIDENCE IT REACHED THE SYSTEM. A
			// stage that opened nothing prints no summary, which is exactly
			// what the leak produced -- so asserting the line is asserting the
			// world was there to be asked.
			summarised := strings.Contains(out, "fuzz: ") &&
				strings.Contains(out, "case(s) reached the system")
			if summarised != c.summary {
				t.Errorf("the stage %s report a summary and the cell %s one\nstdout:\n%s\nstderr:\n%s",
					did(summarised), wanted(c.summary), out, errb)
			}
			if c.summary {
				// AND NEVER BY FAILING TO OPEN A WORLD: the leak's symptom was
				// this exact string on stderr with no summary on stdout, so the
				// absence of it is asserted beside the presence of the line.
				if strings.Contains(errb, "connection refused") {
					t.Errorf("the stage reached the system and something still could not connect, "+
						"which is the address-environment leak this arm exists for\nstderr:\n%s", errb)
				}
			}
			if c.vacuous {
				// AND IT IS ON STDERR, BY THE TOOL's OWN DIVISION. I asserted
				// stdout first and was wrong: `fuzz.Run` refuses the absent
				// table itself, before any report exists to read Vacuous() on,
				// so this arrives as a could-not-run through the error path.
				// Measured rather than assumed, and the stream is asserted
				// because a refusal that moved to stdout would move every
				// unprofiled control digest.
				if !strings.Contains(errb, "no refusal text declared") ||
					!strings.Contains(errb, "[refusal].message") {
					t.Errorf("a stage whose oracle had no subject did not name the key that was "+
						"missing\nstderr:\n%s", errb)
				}
				if strings.Contains(out, "fuzz: FOUND ") {
					t.Errorf("a stage with no refusal text declared reported findings grounded in "+
						"a key nobody declared\nstdout:\n%s", out)
				}
			}
			t.Logf("PROOF exit %d\nstdout:\n%s\nstderr:\n%s", code, out, errb)
		})
	}
}

// requireFuzz changes nothing and asserts the shipped toy enables the stage,
// which every cell rests on. fable ruled the declaration into the project at
// coop rumor/1580, so writing it here would be a duplicate key.
func requireFuzz(s string) string {
	if !strings.Contains(s, "fuzz = { enabled = true }") {
		panic("examples/toy no longer enables the fuzz stage, so these cells would drive a " +
			"project with no corpus to send: fable ruled it in at coop rumor/1580")
	}
	return s
}

func concurrencyOff(s string) string {
	return strings.Replace(s, "concurrency = { enabled = true }",
		"concurrency = { enabled = false }", 1)
}

// cutRefusal removes the toy's `[refusal]` table, and asserts it found one: a
// silent no-op would leave the vacuity cell driving a project that DOES declare
// its refusal text, reading as a could-not-run arm while testing nothing.
func cutRefusal(s string) string {
	const block = "[refusal]\nmessage = \"body.error\"\n"
	if !strings.Contains(s, block) {
		panic("examples/toy no longer declares [refusal].message as this arm expects, so the " +
			"vacuity cell would drive a project whose oracle has its subject")
	}
	return strings.Replace(s, block, "", 1)
}
