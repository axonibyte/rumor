package run_test

import (
	"os"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/platform"
	"forge.axonibyte.com/axonibyte/rumor/internal/proof"
)

// heardWitnessArm is the arm that witnesses a `witness=heard` row, held as a
// VALUE so the link is the COMPILER'S and not a grep's: delete the arm and this
// tree stops building. That is fable's "a `heard` row with no browser arm in the
// tree is a refusal, not a pass" (coop r9/338) made structural, which is
// stronger than asserting it -- an arm that searched the sources for a name
// would pass over a rename, and a rename is the likely way the witness goes.
var heardWitnessArm = TestEveryHeardRowIsHeardThroughARealBrowser

// heardWitnessArmName is that arm's name, DERIVED and not written, so the
// sentence the replay arm prints cannot drift from the function it names. A
// hard-coded name here would be the second copy of one fact, which is how the
// three control digests in tests/contract went five days stale.
func heardWitnessArmName() string {
	full := runtime.FuncForPC(reflect.ValueOf(heardWitnessArm).Pointer()).Name()
	if i := strings.LastIndex(full, "."); i >= 0 {
		return full[i+1:]
	}
	return full
}

// promotedRow is one line of a seeds.lock, as the product's own parser reads it.
type promotedRow struct {
	seed, profile string
	steps         int
	// witness is the row's `witness=` word, EMPTY for a row that promotes a
	// defect -- which is every row written before the token existed.
	witness string
}

// promotedRows reads the toy's seeds.lock through platform.ParseSeedLine.
//
// IT IS ONE FUNCTION BECAUSE THE GRAMMAR MUST BE READ ONCE. The replay arm's
// own comment records that its inline reading was "the fourth copy in the tree"
// and that a `profile=` token broke it -- loudly there, and quietly in
// internal/shrink, which read the same line as "that seed is not here". A
// second arm keying on `witness=` is exactly how a fifth copy arrives, so both
// callers take their rows from here and the parse stays the product's.
func promotedRows(t *testing.T, lock string) []promotedRow {
	t.Helper()
	raw, err := os.ReadFile(lock)
	if err != nil {
		t.Fatalf("reading %s: %v", lock, err)
	}
	var rows []promotedRow
	for _, line := range strings.Split(string(raw), "\n") {
		parsed, isSeed, perr := platform.ParseSeedLine(line)
		if !isSeed {
			continue
		}
		if perr != nil {
			t.Fatalf("%s: %q is not a promoted seed in the format docs/contract.md declares: %v",
				lock, strings.TrimSpace(line), perr)
		}
		if _, cerr := strconv.ParseUint(parsed.Seed, 10, 64); cerr != nil {
			t.Fatalf("%s: %q is not one non-negative integer per line, which is the format "+
				"docs/contract.md declares: %v", lock, parsed.Seed, cerr)
		}
		rows = append(rows, promotedRow{seed: parsed.Seed, profile: parsed.Profile,
			steps: parsed.Steps, witness: parsed.Witness})
	}
	return rows
}

// theHeardLine reads a non-stopping finding's own line: the kind, the step it is
// attributed to, and the actor. docs/cli.md fixes the form, and v9 requires all
// three -- "a line missing any one of the three is a finding a reader cannot act
// on" -- so this arm reads all three rather than searching for `page_fault`.
var theHeardLine = regexp.MustCompile(
	`FOUND page_fault of kind (\w+) after step (\d+) as (\S+) --`)

// TestEveryHeardRowIsHeardThroughARealBrowser is the witness fable's ruling
// names for a `witness=heard` row (coop r9/338).
//
// WHY THE ROW NEEDED AN ARM OF ITS OWN, measured rather than argued: the arm
// that replays every promoted row drives no browser, and R9.3's two rows exit 3
// there with ZERO `page_fault` lines -- they pass on a statute bug found at the
// same step (ibis, coop r9/337). A row that passes on a defect other than the
// one it names is what seeds.lock exists to forbid, so the word declares its
// witness and this is it.
//
// IT ASSERTS THE LINE AND NOT MERELY exit 3, which is the whole difference: 3
// is also what the statute bug alone produces, so a code-only check here would
// be the same pass for the same wrong reason one arm further on.
//
// IT DOES NOT ASSERT WHICH kind, actor OR step, and that is deliberate. The row
// declares that a fault is HEARD, not which one -- seed 13 hears `response`
// alone and seed 38 hears `console` and `response` -- so an arm pinning the
// values would turn a row about the stage's hearing into a row about today's
// weights, which is the thing `profile=` exists to keep out of a seed.
func TestEveryHeardRowIsHeardThroughARealBrowser(t *testing.T) {
	const lock = "../../examples/toy/seeds.lock"
	var heard []promotedRow
	for _, r := range promotedRows(t, lock) {
		if r.witness == platform.WitnessHeard {
			heard = append(heard, r)
		}
	}
	if len(heard) == 0 {
		// THE VACUITY GUARD, in the replay arm's shape: a lock holding no
		// `heard` row leaves this arm checking nothing, and a deliberate
		// decision to carry none has to change this arm rather than pass it
		// silently. R9.3 promoted seeds 13 and 38 under `profile=planted`.
		t.Fatalf("%s holds no `witness=%s` row, so this arm is checking nothing",
			lock, platform.WitnessHeard)
	}

	for _, r := range heard {
		row := r
		t.Run("seed"+row.seed, func(t *testing.T) {
			steps := 10
			if row.steps != 0 {
				steps = row.steps
			}
			args := []string{"run", "../../examples/toy", "--seed", row.seed}
			if row.profile != "" {
				args = append(args, "--profile", row.profile)
			}
			args = append(args, "--steps", strconv.Itoa(steps), "--browser-check")
			out, errb, code := run(t, args...)

			// A BOX WITHOUT A DRIVABLE PAIR SKIPS BY NAME, carrying the reason
			// the product gave, which is this suite's standing shape for the
			// browser arms. The pair is a property of the box; a skip that did
			// not say which half was missing would be the silent-skip class the
			// stage exists to refuse.
			for _, miss := range []string{"SKIPPED, no drivable browser", "no browser for this run"} {
				if strings.Contains(out, miss) || strings.Contains(errb, miss) {
					t.Skipf("this box has no drivable browser, so nothing could be heard: %s", miss)
				}
			}

			if code != 3 {
				t.Fatalf("exit %d, want 3: a `witness=%s` row's finding is a page_fault, and "+
					"docs/cli.md's row for 3 covers a finding that did not stop the walk\n"+
					"stdout:\n%s\nstderr:\n%s", code, platform.WitnessHeard, out, errb)
			}

			found := theHeardLine.FindAllStringSubmatch(out, -1)
			if len(found) == 0 {
				t.Fatalf("seed %s is promoted as `witness=%s` and the run reports no page_fault "+
					"line at all, so the row witnesses nothing through this arm either\n"+
					"stdout:\n%s", row.seed, platform.WitnessHeard, out)
			}
			for _, m := range found {
				kind, step, actor := m[1], m[2], m[3]
				at, cerr := strconv.Atoi(step)
				if cerr != nil || at < 1 {
					t.Errorf("the line attributes a %s fault to step %q, which is no step: v9 "+
						"wants the step it is attributed to", kind, step)
				}
				if actor == "" || actor == "--" {
					t.Errorf("the line names no actor for the %s fault at step %s", kind, step)
				}
				if !strings.Contains(out, "the walk continued") {
					t.Errorf("the %s fault's line does not say the walk continued, which is the "+
						"clause that separates it from the stopper", kind)
				}
			}
			// THROUGH proof.Logf AND NOT t.Logf, which tests/hygiene enforces and
			// which this arm needed: the loop above can Errorf, and a PROOF line
			// printed beside the failure that refutes it is a claim the run just
			// disproved (ratchet, coop r8/184).
			proof.Logf(t, "PROOF seed %s heard %d page_fault line(s) through a real browser",
				row.seed, len(found))
		})
	}
}
