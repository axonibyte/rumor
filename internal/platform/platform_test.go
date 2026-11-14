package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"forge.axonibyte.com/axonibyte/rumor/internal/proof"
)

// wellFormed is the decoded form of docs/contract.md's sample, as a TOML reader
// would hand it over. Every mutation below starts from a copy of this, so the
// control and the cases cannot drift apart.
//
// IT IS BUILT, NOT PARSED, and that is the seam this package was written for:
// the refusals are driven with no file on disk and no TOML reader wired in. The
// one thing it therefore cannot prove is that the document's SAMPLE TEXT parses
// -- that test is owed, and it arrives with the reader.
func wellFormed() map[string]any {
	return map[string]any{
		"contract":   "1",
		"seeds_file": "seeds.lock",
		"adapter": map[string]any{
			"command": []any{"rumor", "fake-adapter"},
			"cwd":     ".",
			"env":     []any{"BASE_URL"},
		},
		"venire": []any{
			map[string]any{"role": "host", "count": int64(1)},
			map[string]any{"role": "guest", "count": int64(3)},
		},
		"stages": map[string]any{
			"api-sim":     map[string]any{"enabled": true},
			"fuzz":        map[string]any{"enabled": false},
			"concurrency": map[string]any{"enabled": false},
			"browser":     map[string]any{"enabled": false},
		},
		"weights": map[string]any{
			"default": map[string]any{
				"create_event": int64(5),
				"join_event":   int64(8),
				"suborn":       int64(1),
			},
			"stage": map[string]any{
				"browser": map[string]any{"join_event": int64(3)},
			},
			"profile": map[string]any{
				"nightly": map[string]any{"suborn": int64(3)},
			},
		},
		"profiles": map[string]any{
			"pre-push": map[string]any{
				"stages": []any{"api-sim"},
				"seeds":  int64(4),
				"budget": "5m",
			},
			"nightly": map[string]any{
				"stages": []any{"api-sim", "concurrency", "browser"},
				"seeds":  int64(64),
				"budget": "2h",
			},
		},
	}
}

// at walks a dotted path into a copy of the document, so a case can delete or
// replace one key without rebuilding the fixture around it.
func at(doc map[string]any, path string) (map[string]any, string) {
	parts := strings.Split(path, ".")
	cur := doc
	for _, p := range parts[:len(parts)-1] {
		next, ok := cur[p].(map[string]any)
		if !ok {
			panic("no table at " + p + " in " + path)
		}
		cur = next
	}
	return cur, parts[len(parts)-1]
}

// TestTheWellFormedPlatformIsAccepted is the control for every refusal below: a
// parser that refused everything would pass all of them.
func TestTheWellFormedPlatformIsAccepted(t *testing.T) {
	rep := Parse(wellFormed())
	if !rep.OK() {
		t.Fatalf("the sample is refused: %v", rep.Refusals)
	}
	p := rep.Platform
	if p.Contract != "1" || p.SeedsFile != "seeds.lock" {
		t.Errorf("contract %q seeds_file %q", p.Contract, p.SeedsFile)
	}
	if len(p.Venire) != 2 || p.Venire[0].Role != "host" || p.Venire[1].Count != 3 {
		t.Errorf("venire %+v", p.Venire)
	}
	if len(p.Stages) != 4 || !p.Stages["api-sim"].Enabled || p.Stages["browser"].Enabled {
		t.Errorf("stages %+v", p.Stages)
	}
	if p.Profiles["nightly"].Budget != 2*time.Hour || p.Profiles["pre-push"].Seeds != 4 {
		t.Errorf("profiles %+v", p.Profiles)
	}
	if len(p.Adapter.Command) != 2 || p.Adapter.Command[0] != "rumor" {
		t.Errorf("adapter %+v", p.Adapter)
	}
}

func TestSeedsFileDefaultsToTheLock(t *testing.T) {
	doc := wellFormed()
	delete(doc, "seeds_file")
	rep := Parse(doc)
	if !rep.OK() {
		t.Fatalf("an absent seeds_file is refused: %v", rep.Refusals)
	}
	if rep.Platform.SeedsFile != DefaultSeedsFile {
		t.Errorf("seeds_file %q, want the default %q", rep.Platform.SeedsFile, DefaultSeedsFile)
	}
}

// TestEveryRequiredKeyIsRefusedByItsPath deletes one key at a time. The paths
// are listed rather than derived because the list IS the claim -- "these keys
// are required" -- and deriving it from the parser would make the test agree
// with whatever the parser happens to demand.
func TestEveryRequiredKeyIsRefusedByItsPath(t *testing.T) {
	for _, path := range []string{
		"contract",
		"adapter",
		"adapter.command",
		"stages",
		"weights",
		"weights.default",
	} {
		doc := wellFormed()
		table, key := at(doc, path)
		delete(table, key)
		rep := Parse(doc)
		if rep.OK() {
			t.Errorf("%s deleted: accepted", path)
			continue
		}
		if !names(rep.Refusals, path, "missing") {
			t.Errorf("%s deleted: refusals are %v", path, rep.Refusals)
		}
	}
}

// TestAnUnknownKeyIsRefusedAtEveryLevel: the top table, a named table, a table
// inside an array of tables, and a weights table. One of each, because the
// reader is recursive and a level that forgot to call done() would be the only
// level that fails.
func TestAnUnknownKeyIsRefusedAtEveryLevel(t *testing.T) {
	cases := map[string]func(map[string]any){
		"nonesuch": func(doc map[string]any) {
			doc["nonesuch"] = true
		},
		"adapter.nonesuch": func(doc map[string]any) {
			doc["adapter"].(map[string]any)["nonesuch"] = true
		},
		"stages.api-sim.nonesuch": func(doc map[string]any) {
			doc["stages"].(map[string]any)["api-sim"].(map[string]any)["nonesuch"] = true
		},
		"venire[0].nonesuch": func(doc map[string]any) {
			doc["venire"].([]any)[0].(map[string]any)["nonesuch"] = true
		},
		"profiles.nightly.nonesuch": func(doc map[string]any) {
			doc["profiles"].(map[string]any)["nightly"].(map[string]any)["nonesuch"] = true
		},
	}
	for path, mutate := range cases {
		doc := wellFormed()
		mutate(doc)
		rep := Parse(doc)
		if rep.OK() {
			t.Errorf("%s added: accepted", path)
			continue
		}
		if !names(rep.Refusals, path, "unknown key") {
			t.Errorf("%s added: refusals are %v", path, rep.Refusals)
		}
	}
}

func TestAWrongTypeIsRefusedWithTheTypeItWanted(t *testing.T) {
	for _, tc := range []struct {
		path, want string
		value      any
	}{
		{"contract", "wanted a string", int64(1)},
		{"stages.api-sim.enabled", "wanted true or false", "yes"},
		{"stages.api-sim.reconcile_every", "wanted an integer", "soon"},
		{"venire[0].count", "wanted an integer", 2.5},
		{"adapter.command", "wanted an array of strings", "rumor"},
		{"profiles.nightly.budget", "wanted a duration", "fortnight"},
		{"weights.default.suborn", "wanted an integer", "heavy"},
	} {
		doc := wellFormed()
		switch tc.path {
		case "venire[0].count":
			doc["venire"].([]any)[0].(map[string]any)["count"] = tc.value
		default:
			table, key := at(doc, tc.path)
			table[key] = tc.value
		}
		rep := Parse(doc)
		if rep.OK() {
			t.Errorf("%s = %v: accepted", tc.path, tc.value)
			continue
		}
		if !names(rep.Refusals, tc.path, tc.want) {
			t.Errorf("%s = %v: refusals are %v", tc.path, tc.value, rep.Refusals)
		}
	}
}

// TestAPeriodIsRefusedONLYWhenNegative is both directions of the
// `reconcile_every` guard (R8/110).
//
// A NEGATIVE PERIOD IS REFUSED BY NAME RATHER THAN CLAMPED because `n % period`
// never equals 0 for a negative period: the stage would reconcile only at the
// end while its declaration said otherwise, which is a knob that reads as set
// and does nothing.
//
// AND THE OTHER DIRECTION IS WHAT MAKES THE ARM WORTH HAVING: a guard that
// refused every period would satisfy the first half alone, so 0 (the default,
// meaning end-of-run only) and a positive period must both be ACCEPTED, and the
// positive one must survive parsing with its value intact.
func TestAPeriodIsRefusedONLYWhenNegative(t *testing.T) {
	for _, tc := range []struct {
		value   int64
		refused bool
	}{
		{-1, true},
		{-8, true},
		{0, false},
		{1, false},
		{7, false},
	} {
		doc := wellFormed()
		doc["stages"].(map[string]any)["api-sim"].(map[string]any)["reconcile_every"] = tc.value
		rep := Parse(doc)
		if tc.refused {
			if rep.OK() {
				t.Errorf("reconcile_every = %d: accepted", tc.value)
				continue
			}
			if !names(rep.Refusals, "stages.api-sim.reconcile_every", "cannot be negative") {
				t.Errorf("reconcile_every = %d: refused for the wrong reason: %v",
					tc.value, rep.Refusals)
			}
			continue
		}
		if !rep.OK() {
			t.Errorf("reconcile_every = %d: refused: %v", tc.value, rep.Refusals)
			continue
		}
		// THE VALUE SURVIVES, not just the parse. A reader that accepted the
		// key and dropped it would pass every arm above.
		if got := Parse(doc).Platform.Stages["api-sim"].ReconcileEvery; got != int(tc.value) {
			t.Errorf("reconcile_every = %d parsed as %d", tc.value, got)
		}
	}
}

// TestWhatCanRunNothingIsRefused is the vacuity set, and it is not the same as
// the missing-key set: each of these is a well-formed file that declares a run
// which could do nothing at all.
func TestWhatCanRunNothingIsRefused(t *testing.T) {
	for _, tc := range []struct {
		name, path, want string
		mutate           func(map[string]any)
	}{
		{"no venire", "venire", "no entries", func(doc map[string]any) { delete(doc, "venire") }},
		{"empty venire", "venire", "no entries", func(doc map[string]any) { doc["venire"] = []any{} }},
		{"a role with no actors", "venire[0].count", "can do nothing", func(doc map[string]any) {
			doc["venire"].([]any)[0].(map[string]any)["count"] = int64(0)
		}},
		{"every stage disabled", "stages", "every stage is disabled", func(doc map[string]any) {
			for _, name := range []string{"api-sim", "fuzz", "concurrency", "browser"} {
				doc["stages"].(map[string]any)[name].(map[string]any)["enabled"] = false
			}
		}},
		{"no stages", "stages", "no stages declared", func(doc map[string]any) {
			doc["stages"] = map[string]any{}
		}},
		{"empty default weights", "weights.default", "no action kinds", func(doc map[string]any) {
			doc["weights"].(map[string]any)["default"] = map[string]any{}
		}},
		{"an empty argv", "adapter.command", "nothing to spawn", func(doc map[string]any) {
			doc["adapter"].(map[string]any)["command"] = []any{}
		}},
		// AND A ZONE VARIABLE `env` DOES NOT PASS THROUGH (R7.5 D4c): the
		// engine would set a name the adapter never receives, which is a
		// declaration with no effect -- the shape this package refuses
		// elsewhere. `env` is the list of names passed through, so the pairing
		// is what makes `zone_env` reach anything at all.
		{"a zone_env outside env", "adapter.zone_env", "not in [adapter].env",
			func(doc map[string]any) {
				a := doc["adapter"].(map[string]any)
				a["env"] = []any{"OTHER"}
				a["zone_env"] = "TOY_ZONE"
			}},
		{"a profile with no seeds", "profiles.nightly.seeds", "runs nothing", func(doc map[string]any) {
			doc["profiles"].(map[string]any)["nightly"].(map[string]any)["seeds"] = int64(0)
		}},
		// THE WORDING HERE IS internal/decl's, not this package's: a duration
		// of zero is refused by the shared reader, so the fragment asserted is
		// the part that is about the VALUE rather than about what a budget is
		// for.
		{"a zero budget", "profiles.nightly.budget", "zero or less", func(doc map[string]any) {
			doc["profiles"].(map[string]any)["nightly"].(map[string]any)["budget"] = "0s"
		}},
	} {
		doc := wellFormed()
		tc.mutate(doc)
		rep := Parse(doc)
		if rep.OK() {
			t.Errorf("%s: accepted", tc.name)
			continue
		}
		if !names(rep.Refusals, tc.path, tc.want) {
			t.Errorf("%s: refusals are %v", tc.name, rep.Refusals)
		}
	}
}

func TestANegativeWeightIsRefusedAndZeroIsNot(t *testing.T) {
	doc := wellFormed()
	doc["weights"].(map[string]any)["default"].(map[string]any)["suborn"] = int64(-1)
	rep := Parse(doc)
	if rep.OK() || !names(rep.Refusals, "weights.default.suborn", "cannot be negative") {
		t.Errorf("a negative weight: %v", rep.Refusals)
	}
	// THE CONTROL, because the rule is about negatives and not about small
	// numbers: zero is a legal weight meaning never drawn.
	doc = wellFormed()
	doc["weights"].(map[string]any)["default"].(map[string]any)["suborn"] = int64(0)
	rep = Parse(doc)
	if !rep.OK() {
		t.Errorf("a zero weight is refused: %v", rep.Refusals)
	}
	if w, ok := rep.Platform.Weights.Weight("", "", "suborn"); !ok || w != 0 {
		t.Errorf("zero weight resolved to %d, %v", w, ok)
	}
}

// TestTheCrossKeyRefusals: each needs two parts of the file, so none can be
// checked while reading either one.
func TestTheCrossKeyRefusals(t *testing.T) {
	for _, tc := range []struct {
		name, path, want string
		mutate           func(map[string]any)
	}{
		{"an override for an undeclared stage", "weights.stage.nonesuch", "no stage of that name",
			func(doc map[string]any) {
				doc["weights"].(map[string]any)["stage"].(map[string]any)["nonesuch"] = map[string]any{"suborn": int64(2)}
			}},
		{"an override for an undeclared profile", "weights.profile.nonesuch", "no profile of that name",
			func(doc map[string]any) {
				doc["weights"].(map[string]any)["profile"].(map[string]any)["nonesuch"] = map[string]any{"suborn": int64(2)}
			}},
		{"a profile naming an undeclared stage", "profiles.nightly.stages", "which is not declared",
			func(doc map[string]any) {
				doc["profiles"].(map[string]any)["nightly"].(map[string]any)["stages"] = []any{"api-sim", "nonesuch"}
			}},
		{"an override naming a kind the default does not", "weights.stage.browser", "[weights.default] does not",
			func(doc map[string]any) {
				doc["weights"].(map[string]any)["stage"].(map[string]any)["browser"] = map[string]any{"essoin": int64(2)}
			}},
	} {
		doc := wellFormed()
		tc.mutate(doc)
		rep := Parse(doc)
		if rep.OK() {
			t.Errorf("%s: accepted", tc.name)
			continue
		}
		if !names(rep.Refusals, tc.path, tc.want) {
			t.Errorf("%s: refusals are %v", tc.name, rep.Refusals)
		}
	}
}

// TestWeightResolutionOrder drives all four outcomes, which is the only way to
// tell a precedence from a lookup that happens to agree: profile over stage,
// stage over default, default alone, and absent.
func TestWeightResolutionOrder(t *testing.T) {
	rep := Parse(wellFormed())
	if !rep.OK() {
		t.Fatalf("the sample is refused: %v", rep.Refusals)
	}
	w := rep.Platform.Weights
	for _, tc := range []struct {
		stage, profile, kind string
		want                 int
		found                bool
	}{
		{"browser", "nightly", "suborn", 3, true},     // the profile wins
		{"browser", "nightly", "join_event", 3, true}, // the stage, where the profile is silent
		{"browser", "", "join_event", 3, true},        // the stage alone
		{"api-sim", "", "join_event", 8, true},        // the default, where no override names the stage
		{"", "", "create_event", 5, true},             // the default alone
		{"browser", "nightly", "nonesuch", 0, false},  // no table names it
	} {
		got, ok := w.Weight(tc.stage, tc.profile, tc.kind)
		if got != tc.want || ok != tc.found {
			t.Errorf("Weight(%q, %q, %q) = %d, %v; want %d, %v",
				tc.stage, tc.profile, tc.kind, got, ok, tc.want, tc.found)
		}
	}
}

// TestEveryRefusalIsReportedNotJustTheFirst, because a file with three faults
// repaired one at a time is three runs, and attaint's rule -- print every
// refusal, not the first -- is the one this package follows.
func TestEveryRefusalIsReportedNotJustTheFirst(t *testing.T) {
	doc := wellFormed()
	delete(doc, "contract")
	doc["nonesuch"] = true
	doc["venire"] = []any{}
	rep := Parse(doc)
	if len(rep.Refusals) < 3 {
		t.Fatalf("three faults produced %d refusal(s): %v", len(rep.Refusals), rep.Refusals)
	}
	for _, want := range []string{"contract", "nonesuch", "venire"} {
		if !names(rep.Refusals, want, "") {
			t.Errorf("no refusal names %q: %v", want, rep.Refusals)
		}
	}
	// SORTED BY PATH, so a reader comparing two runs of the same file reads the
	// same order and a diff of two reports is about the faults.
	for i := 1; i < len(rep.Refusals); i++ {
		if rep.Refusals[i-1].Path > rep.Refusals[i].Path {
			t.Errorf("refusals are not sorted by path: %v", rep.Refusals)
			break
		}
	}
}

func TestRollbackIsOptionalAndCheckedWhenPresent(t *testing.T) {
	doc := wellFormed()
	if rep := Parse(doc); !rep.OK() {
		t.Fatalf("no rollback table is refused: %v", rep.Refusals)
	}
	doc["rollback"] = []any{map[string]any{"provider": "process-restart", "covers": []any{"store"}}}
	rep := Parse(doc)
	if !rep.OK() {
		t.Fatalf("a well-formed rollback entry is refused: %v", rep.Refusals)
	}
	if len(rep.Platform.Rollback) != 1 || rep.Platform.Rollback[0].Provider != "process-restart" {
		t.Errorf("rollback %+v", rep.Platform.Rollback)
	}
	doc["rollback"] = []any{map[string]any{"provider": "process-restart", "covers": []any{}}}
	if rep := Parse(doc); rep.OK() || !names(rep.Refusals, "rollback[0].covers", "covers nothing") {
		t.Errorf("a provider covering nothing: %v", rep.Refusals)
	}
}

func TestThresholdsAreCarriedAndBlockingIsTyped(t *testing.T) {
	doc := wellFormed()
	doc["thresholds"] = map[string]any{"blocking": false, "actions": 0.9}
	rep := Parse(doc)
	if !rep.OK() {
		t.Fatalf("a thresholds table is refused: %v", rep.Refusals)
	}
	if rep.Platform.Thresholds["actions"] != 0.9 {
		t.Errorf("thresholds %+v: R15 names the metrics, so they are carried as read", rep.Platform.Thresholds)
	}
	doc["thresholds"] = map[string]any{"blocking": "yes"}
	if rep := Parse(doc); rep.OK() || !names(rep.Refusals, "thresholds.blocking", "true or false") {
		t.Errorf("blocking as a string: %v", rep.Refusals)
	}
}

// names reports whether some refusal has that path and whose reason contains
// the fragment. Path and fragment rather than the whole sentence: a test that
// pinned every word would fail on a reworded message that said the same thing,
// and this tree has one of those checks already (the quote oracle) where the
// wording IS the subject.
func names(refusals []Refusal, path, fragment string) bool {
	for _, r := range refusals {
		if r.Path != path {
			continue
		}
		if fragment == "" || strings.Contains(r.Reason, fragment) {
			return true
		}
	}
	return false
}

// TestLoadKeepsTheTwoErrorShapesApart: an unreadable FILE is an error, which is
// could-not-run; a readable file that is not a platform is a Report full of
// refusals, which is a verdict on the project. `rumor validate`'s exit statuses
// turn on exactly that line, so collapsing the two here would move a refusal
// into the could-not-run bucket for every caller at once.
func TestLoadKeepsTheTwoErrorShapesApart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "platform.toml")
	if err := os.WriteFile(path, []byte("# the decoder below ignores these bytes\n"), 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}

	// A READABLE FILE THAT IS A PLATFORM.
	rep, err := Load(path, func([]byte) (map[string]any, error) { return wellFormed(), nil })
	if err != nil {
		t.Fatalf("a well-formed platform is an error: %v", err)
	}
	if !rep.OK() || rep.Platform.Contract != "1" {
		t.Errorf("report %+v", rep)
	}

	// A READABLE FILE THAT IS NOT: refusals, and NO error.
	rep, err = Load(path, func([]byte) (map[string]any, error) {
		doc := wellFormed()
		delete(doc, "contract")
		return doc, nil
	})
	if err != nil {
		t.Fatalf("a refused platform came back as an error: %v", err)
	}
	if rep.OK() || !names(rep.Refusals, "contract", "missing key") {
		t.Errorf("report %+v", rep)
	}

	// AN UNREADABLE FILE: an error, and an empty report rather than a refusal
	// list a caller might print as findings.
	rep, err = Load(filepath.Join(dir, "nonesuch.toml"), func([]byte) (map[string]any, error) {
		return wellFormed(), nil
	})
	if err == nil {
		t.Fatal("an absent file was accepted")
	}
	if len(rep.Refusals) != 0 {
		t.Errorf("an absent file produced refusals: %v", rep.Refusals)
	}
}

// THE WITNESS TOKEN IS REFUSED BY NAME WITH THE KNOWN SET PRINTED, which is
// fable's ruling at coop rumor/1241 and the half a reader needs: a row asking
// for a check this build will not perform must say so in words that let the
// author fix the line, and a refusal listing words the parser no longer takes
// is worse than none because the reader edits toward it.
//
// THE SET COMES FROM THE SAME LIST THE CHECK USES, so this arm cannot pass
// while the message drifts: it asserts every word Witnesses() returns appears
// in the refusal, rather than matching a literal.
func TestAnUnknownWitnessIsRefusedNamingTheKnownSet(t *testing.T) {
	for _, c := range []struct {
		what, line string
		// wantErr is whether the line must be refused.
		wantErr bool
		// witness is what a good line must parse to.
		witness string
	}{
		{"a word this build does not know", "66 witness=recovered", true, ""},
		{"the token naming nothing", "66 witness=", true, ""},
		{"the token twice", "66 witness=cleared witness=cleared", true, ""},
		{"the known word", "66 profile=settling steps=24 witness=cleared", false, "cleared"},
		// THE ROWS WITHOUT IT ARE THE CONTROL, and they carry the weight: the
		// token must not change what a line written before it existed means.
		{"a bare defect row", "2", false, ""},
		{"a profiled defect row", "12 profile=settling steps=24", false, ""},
	} {
		t.Run(c.what, func(t *testing.T) {
			parsed, isSeed, err := ParseSeedLine(c.line)
			if !isSeed {
				t.Fatalf("%q did not read as a seed line at all", c.line)
			}
			if !c.wantErr {
				if err != nil {
					t.Fatalf("%q was refused and should not be: %v", c.line, err)
				}
				if parsed.Witness != c.witness {
					t.Errorf("%q parsed witness %q, want %q", c.line, parsed.Witness, c.witness)
				}
				return
			}
			if err == nil {
				t.Fatalf("%q was accepted, so a row can ask for a check this build will "+
					"not perform", c.line)
			}
			// Only the unknown-WORD refusal must print the set; the empty and
			// duplicate forms are about the token's shape.
			if !strings.Contains(c.line, "witness=recovered") {
				return
			}
			for _, w := range Witnesses() {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("the refusal does not name the known witness %q, so a reader "+
						"cannot tell what to write: %v", w, err)
				}
			}
		})
	}
}

// R8/111's PROFILE LEVEL for the period (fable, coop rumor/1619).
//
// THE FIXTURE'S WALKING STAGE IS `api-sim` AND THE PRODUCT'S IS `api`, which is
// why every cell below declares its own `api` stage rather than reusing the
// fixture's: the resolver reads `Stages["api"]` because the walk IS that stage,
// so a cell built on `api-sim` would exercise the absent branch while looking
// like it exercised the stage branch. Found by writing it the other way first.
func TestThePeriodResolvesProfileThenStageThenAbsent(t *testing.T) {
	// withLevels returns the fixture carrying an `api` stage and a `walking`
	// profile that selects it, each period applied only when non-negative so
	// one helper serves every cell.
	withLevels := func(stageEvery, profileEvery int) map[string]any {
		doc := wellFormed()
		st := map[string]any{"enabled": true}
		if stageEvery >= 0 {
			st["reconcile_every"] = int64(stageEvery)
		}
		doc["stages"].(map[string]any)["api"] = st
		pr := map[string]any{"stages": []any{"api"}, "seeds": int64(1), "budget": "1m"}
		if profileEvery >= 0 {
			pr["reconcile_every"] = int64(profileEvery)
		}
		doc["profiles"].(map[string]any)["walking"] = pr
		return doc
	}
	for _, tc := range []struct {
		what       string
		stageEvery int
		profEvery  int
		profile    string
		wantEvery  int
		wantLevel  PeriodLevel
	}{
		{"neither level declares one", -1, -1, "walking", 0, PeriodAbsent},
		{"the stage alone", 6, -1, "walking", 6, PeriodFromStage},
		{"the profile alone", -1, 3, "walking", 3, PeriodFromProfile},
		// THE CELL THAT MAKES IT A PRECEDENCE and not two independent reads:
		// both declared, and the profile must win.
		{"both, and the profile wins", 6, 3, "walking", 3, PeriodFromProfile},
		// AND THE SAME PLATFORM READ WITHOUT A PROFILE falls to the stage, so
		// the profile's period cannot leak into an unprofiled run -- which is
		// the whole reason this level exists: declaring it must move nothing
		// that does not name the profile.
		{"both declared, asked with no profile", 6, 3, "", 6, PeriodFromStage},
		{"profile only, asked with no profile", -1, 3, "", 0, PeriodAbsent},
		// ZERO IS NOT A LEVEL. A profile declaring 0 declares no period and
		// falls through, rather than overriding the stage with a zero -- the
		// other reading would make 0 the one stage-level value no profile could
		// leave alone.
		{"the profile declares zero over a stage period", 6, 0, "walking", 6, PeriodFromStage},
		{"a profile nobody declared", 6, -1, "absent-profile", 6, PeriodFromStage},
	} {
		t.Run(tc.what, func(t *testing.T) {
			rep := Parse(withLevels(tc.stageEvery, tc.profEvery))
			if !rep.OK() {
				t.Fatalf("refused: %v", rep.Refusals)
			}
			every, level := rep.Platform.ReconcileEvery(tc.profile)
			if every != tc.wantEvery || level != tc.wantLevel {
				t.Errorf("got %d from %q, want %d from %q",
					every, level, tc.wantEvery, tc.wantLevel)
			}
		})
	}
}

// A PERIOD ON A PROFILE THAT DOES NOT RUN THE WALK IS REFUSED BY NAME, which is
// the one new refusal the level needs (fable, coop rumor/1619).
//
// AND THE OTHER DIRECTION IS WHAT MAKES THE ARM WORTH HAVING, as the
// stage-level guard's comment says of itself: a guard refusing every profile
// period would satisfy the first half alone, so a profile that DOES select the
// api stage must be accepted with its value intact.
func TestAProfilePeriodIsRefusedWhenTheProfileDoesNotWalk(t *testing.T) {
	for _, tc := range []struct {
		what    string
		stages  []any
		every   int64
		refused bool
		reason  string
	}{
		{"no api stage at all", []any{"concurrency", "fuzz"}, 4, true, "do not include"},
		{"an empty stage list", []any{}, 4, true, "do not include"},
		// `api-sim` IS NOT `api`, and this cell exists because the fixture uses
		// that name: a resolver matching a prefix or a substring would accept
		// it, and the refusal would then be silent for the one project shape
		// most likely to hit it.
		{"api-sim, which is not the walking stage", []any{"api-sim"}, 4, true, "do not include"},
		{"api among others", []any{"concurrency", "api"}, 4, false, ""},
		{"api alone", []any{"api"}, 4, false, ""},
		// NEGATIVE IS ITS OWN REFUSAL and must not be swallowed by the
		// does-not-walk one: a profile that walks and declares -1 is refused
		// for the sign, naming it.
		{"negative on a walking profile", []any{"api"}, -1, true, "cannot be negative"},
		// AND ZERO IS ACCEPTED ON A NON-WALKING PROFILE, because zero declares
		// no period -- so there is nothing for that profile to fail to read.
		// Without this cell the guard could fire on every profile carrying the
		// key at all.
		{"zero on a non-walking profile", []any{"fuzz"}, 0, false, ""},
	} {
		t.Run(tc.what, func(t *testing.T) {
			doc := wellFormed()
			doc["stages"].(map[string]any)["api"] = map[string]any{"enabled": true}
			doc["profiles"].(map[string]any)["cell"] = map[string]any{
				"stages": tc.stages, "seeds": int64(1), "budget": "1m",
				"reconcile_every": tc.every,
			}
			rep := Parse(doc)
			if tc.refused {
				if rep.OK() {
					t.Fatalf("accepted a period of %d on stages %v", tc.every, tc.stages)
				}
				if !names(rep.Refusals, "profiles.cell.reconcile_every", tc.reason) {
					t.Errorf("refused for the wrong reason: %v", rep.Refusals)
				}
				return
			}
			if !rep.OK() {
				t.Fatalf("refused: %v", rep.Refusals)
			}
			if got := rep.Platform.Profiles["cell"].ReconcileEvery; got != int(tc.every) {
				t.Errorf("the period parsed as %d, want %d", got, tc.every)
			}
		})
	}
}

// renderVenire is this file's own reading of a pool, so a cell below states the
// order as well as the membership. cmd/rumor has a renderer of its own and this
// is deliberately not it: a test sharing the product's formatter cannot tell a
// correct pool from a formatter that agrees with itself.
func renderVenire(ms []Member) string {
	parts := make([]string, 0, len(ms))
	for _, m := range ms {
		parts = append(parts, fmt.Sprintf("%d %s", m.Count, m.Role))
	}
	return strings.Join(parts, ", ")
}

// TestAProfilesVenireAppendsAndNeverReplaces is the appended level's control,
// and its cells are fable's ruling at coop rumor/1660 read back: a profile with
// no rows of its own draws the pool the platform committed, an undeclared name
// draws it, the empty name draws it, and a profile's rows come AFTER the
// committed ones rather than instead of any of them.
//
// THE LAST CELL IS THE ONE THAT CARRIES THE RULING. A row naming a role the
// platform ALREADY names draws MORE actors of it -- 1 host and then 2 host --
// because this file has no expression for "fewer", and a level that could
// shrink the committed pool could invalidate a committed seed. A resolver that
// replaced by role would pass every other cell here.
func TestAProfilesVenireAppendsAndNeverReplaces(t *testing.T) {
	doc := wellFormed()
	doc["stages"].(map[string]any)["api"] = map[string]any{"enabled": true}
	doc["profiles"].(map[string]any)["cell"] = map[string]any{
		"stages": []any{"api"}, "seeds": int64(1), "budget": "1m",
		"venire": []any{
			map[string]any{"role": "viewer", "count": int64(2)},
			map[string]any{"role": "host", "count": int64(2)},
		},
	}
	doc["profiles"].(map[string]any)["bare"] = map[string]any{
		"stages": []any{"api"}, "seeds": int64(1), "budget": "1m",
	}
	rep := Parse(doc)
	if !rep.OK() {
		t.Fatalf("a well-formed appended venire was refused: %v", rep.Refusals)
	}
	p := rep.Platform

	const committed = "1 host, 3 guest"
	for _, c := range []struct{ what, profile, want string }{
		{"the empty name", "", committed},
		{"a profile declaring no rows", "bare", committed},
		{"a profile that is not declared", "nonesuch", committed},
		{"the profile's own rows, after the committed ones", "cell", committed + ", 2 viewer, 2 host"},
	} {
		if got := renderVenire(p.VenireFor(c.profile)); got != c.want {
			t.Errorf("%s: draws %q, want %q", c.what, got, c.want)
		}
	}

	// AND THE ANSWER IS A COPY, driven rather than asserted. The resolver's
	// comment says a caller cannot reach the committed pool through what it
	// hands back; a shared slice would make that false for every profile at
	// once, from a call that reads like a read.
	v := p.VenireFor("")
	if len(v) == 0 {
		t.Fatal("the committed pool is empty, so this cell has no subject")
	}
	v[0].Count = 99
	if p.Venire[0].Count == 99 {
		t.Error("mutating the resolver's answer changed the committed pool: it handed back its own slice")
	}
}

// TestAProfileVenireRowIsRefusedByName holds the appended rows to the same
// reader the platform's own rows go through, which is why the refusal reads the
// same words -- and names the row where it is written rather than as "a venire
// entry", so a reader knows which table to open.
func TestAProfileVenireRowIsRefusedByName(t *testing.T) {
	doc := wellFormed()
	doc["stages"].(map[string]any)["api"] = map[string]any{"enabled": true}
	doc["profiles"].(map[string]any)["cell"] = map[string]any{
		"stages": []any{"api"}, "seeds": int64(1), "budget": "1m",
		"venire": []any{map[string]any{"role": "viewer", "count": int64(0)}},
	}
	rep := Parse(doc)
	if rep.OK() {
		t.Fatal("accepted a profile venire row that draws no actors")
	}
	if !names(rep.Refusals, "profiles.cell.venire[0].count", "can do nothing") {
		t.Errorf("refused under the wrong name or for the wrong reason: %v", rep.Refusals)
	}
}

// TestClockStartResolvesProfileThenClockThenEpoch drives R7.5's fourth level
// (fable, coop r7/109) at the ONE place its precedence is stated.
//
// THE PROJECT'S DEFAULT ARRIVES AS AN ARGUMENT because `[clock].start` lives in
// bindings.toml, which this package does not read -- so the cell passes it in
// the way `rumor run` does, and the precedence is the thing under test rather
// than the plumbing.
//
// ZERO IS ABSENT AT BOTH LEVELS, which is the reading that makes the epoch
// reachable by inheritance: a profile or a project declaring 0 declares
// nothing, and a run with neither begins at the epoch and SAYS so.
func TestClockStartResolvesProfileThenClockThenEpoch(t *testing.T) {
	const spring = int64(1772956800000000000)
	const fall = int64(1793516400000000000)
	const before70 = int64(-86400000000000) // 1969-12-31T00:00:00Z
	p := Platform{Profiles: map[string]Profile{
		"dst-spring": {ClockStart: spring},
		"plain":      {},
		"pre70":      {ClockStart: before70},
	}}
	for _, tc := range []struct {
		what, profile string
		declared      int64
		wantAt        int64
		wantLevel     ClockStartLevel
	}{
		{"a profile's own start wins", "dst-spring", fall, spring, ClockStartFromProfile},
		{"a profile with none takes the project's", "plain", fall, fall, ClockStartFromClock},
		{"an undeclared profile takes the project's", "nonesuch", fall, fall, ClockStartFromClock},
		{"no profile named takes the project's", "", fall, fall, ClockStartFromClock},
		{"neither declares one, so the epoch", "plain", 0, 0, ClockStartEpoch},
		{"a profile's ZERO is absent and inherits", "plain", spring, spring, ClockStartFromClock},
		// A NEGATIVE IS A DECLARATION AND NOT AN ABSENCE, which this file
		// argues three times and armed in none until ibis drove it (coop
		// rumor/1748). The guard is `!= 0` and one character from the period's
		// `> 0`, so a reader writing the period's test twice would reverse
		// this decision with every cell still green: an instant before 1970 is
		// a world a project may want, where a negative PERIOD is arithmetic
		// that can never reach zero.
		{"a NEGATIVE profile start wins, it is not an absence", "pre70", fall, before70,
			ClockStartFromProfile},
		{"a NEGATIVE project start wins too", "plain", before70, before70, ClockStartFromClock},
	} {
		t.Run(tc.what, func(t *testing.T) {
			at, level := p.ClockStartFor(tc.profile, tc.declared)
			if at != tc.wantAt || level != tc.wantLevel {
				t.Errorf("ClockStartFor(%q, %d) = (%d, %s), want (%d, %s)",
					tc.profile, tc.declared, at, level, tc.wantAt, tc.wantLevel)
			}
		})
	}
	if !t.Failed() {
		proof.Logf(t, "PROOF the profile wins, a zero at either level inherits, and a run with "+
			"nothing declared reports the epoch as a LEVEL rather than as a silence")
	}
}

// TestAStartOnAProfileThatCannotWalkIsRefused is the period's clause read for
// R7.5's level, and it is refused at the PARSE for the period's reason: a
// profile's own `stages` list says whether it walks, where a stage table shared
// by every profile cannot be judged alone.
//
// THE WALK IS THE api STAGE, so the walking case names "api" explicitly rather
// than borrowing the fixture's "api-sim" -- which is a stage this project
// declares and the walk does not read, and a cell built on it would be testing
// the refusal in both directions.
func TestAStartOnAProfileThatCannotWalkIsRefused(t *testing.T) {
	const spring = int64(1772956800000000000)
	withProfile := func(stages []any, start any) map[string]any {
		doc := wellFormed()
		// THE api STAGE IS DECLARED HERE because the fixture's table does not
		// carry one: it declares `api-sim`, a stage this project has and the
		// walk does not read. A profile naming an undeclared stage is refused
		// for THAT, which would hide the refusal under test.
		doc["stages"].(map[string]any)["api"] = map[string]any{"enabled": true}
		prof := map[string]any{"stages": stages, "seeds": int64(1), "budget": "1m"}
		if start != nil {
			prof["clock_start"] = start
		}
		// ADDED BESIDE THE FIXTURE'S PROFILES AND NOT INSTEAD OF THEM: a
		// `[weights.profile.*]` table names one of them, and a doc that
		// dropped them would be refused for an orphaned weights row rather
		// than for this level.
		doc["profiles"].(map[string]any)["p"] = prof
		return doc
	}

	// A PROFILE THAT WALKS CARRIES IT.
	rep := Parse(withProfile([]any{"api"}, spring))
	if !rep.OK() {
		t.Fatalf("a walking profile's clock_start is refused: %v", rep.Refusals)
	}
	if got := rep.Platform.Profiles["p"].ClockStart; got != spring {
		t.Errorf("clock_start is %d, want %d: the instant must survive the parse at full "+
			"width, which is what decl.Int64 is for", got, spring)
	}

	// ONE THAT DOES NOT IS REFUSED, BY NAME AND WITH ITS OWN STAGES IN THE
	// MESSAGE, so the author is told which list made the instant unreadable.
	rep = Parse(withProfile([]any{"concurrency"}, spring))
	if rep.OK() {
		t.Fatalf("a profile selecting no api stage carried a clock_start: the walk IS that " +
			"stage, so the instant is one no run of this profile can begin at")
	}
	if !names(rep.Refusals, "profiles.p.clock_start", "concurrency") {
		t.Errorf("the refusal does not name clock_start and this profile's stages: %v",
			rep.Refusals)
	}

	// AND ABSENT IS NOT REFUSED EITHER WAY, which is what keeps every profile
	// committed before this level readable.
	if rep := Parse(withProfile([]any{"concurrency"}, nil)); !rep.OK() {
		t.Errorf("a non-walking profile with NO clock_start is refused: %v", rep.Refusals)
	}
	// A NEGATIVE IS A DECLARATION AND PARSES, which this file argues and ibis
	// found armed by nothing (coop rumor/1748): an instant before 1970 is a
	// world a project may want, so the guard here is `!= 0` and not the
	// period's `> 0`, one character apart and the opposite decision.
	rep = Parse(withProfile([]any{"api"}, int64(-86400000000000)))
	if !rep.OK() {
		t.Fatalf("a pre-1970 clock_start is refused: %v -- the clock's own refusal is about "+
			"moving BACKWARD, not about where it began", rep.Refusals)
	}
	if got := rep.Platform.Profiles["p"].ClockStart; got != -86400000000000 {
		t.Errorf("a negative clock_start parsed as %d", got)
	}
	// AND A NEGATIVE ON A PROFILE THAT CANNOT WALK IS STILL REFUSED, so the
	// non-walking clause reads the DECLARATION and not the sign.
	if rep := Parse(withProfile([]any{"concurrency"}, int64(-86400000000000))); rep.OK() {
		t.Errorf("a negative clock_start on a non-walking profile parsed clean: the clause is " +
			"about whether the profile walks, not about the value")
	}
	// A ZERO IS ABSENT TOO, and must not trip the refusal: the resolver reads 0
	// as "declares nothing", so the parse must not call it a declaration.
	if rep := Parse(withProfile([]any{"concurrency"}, int64(0))); !rep.OK() {
		t.Errorf("a non-walking profile with clock_start = 0 is refused: %v -- zero is absent "+
			"at this level, so it cannot be the one value that makes a profile unreadable",
			rep.Refusals)
	}
	if !t.Failed() {
		proof.Logf(t, "PROOF a start is carried where the profile walks, refused by name with its "+
			"stages where it does not, and absent or zero is refused neither way")
	}
}

// THE NAME THAT CARRIES THE SYSTEM'S ZONE IS READ (R7.5 D4c; fable's ruling at
// coop rumor/1780).
//
// IT IS OPTIONAL AND ITS ABSENCE IS NOT A ZERO: a project whose system under
// test takes no zone declares none, and that is every project written before
// this phase. The pairing with `env` is refused above; this cell is the value
// reaching the struct at all, which nothing else asserts.
func TestAnAdapterMayNameTheVariableThatCarriesTheZone(t *testing.T) {
	doc := wellFormed()
	a := doc["adapter"].(map[string]any)
	a["env"] = []any{"TOY_ADDR", "TOY_ZONE"}
	a["zone_env"] = "TOY_ZONE"
	rep := Parse(doc)
	if !rep.OK() {
		t.Fatalf("a declared zone_env is refused: %v", rep.Refusals)
	}
	if got := rep.Platform.Adapter.ZoneEnv; got != "TOY_ZONE" {
		t.Errorf("ZoneEnv is %q, want TOY_ZONE", got)
	}

	// AND A PROJECT THAT DECLARES NONE CARRIES THE EMPTY STRING, which is what
	// cmd/rumor reads as "this system takes no zone" -- so an engine that set
	// a variable named "" would be setting one nobody declared.
	plain := wellFormed()
	prep := Parse(plain)
	if !prep.OK() {
		t.Fatalf("the well-formed platform is refused: %v", prep.Refusals)
	}
	if got := prep.Platform.Adapter.ZoneEnv; got != "" {
		t.Errorf("a platform declaring no zone_env carries %q", got)
	}
}

// `shrink_candidates` (issue 124's deliverable 4, coop r9/204): absent, the
// default; declared, its value; under 1, refused by name -- the replay gate is a
// candidate, so 0 would stop every shrink before it learned whether the failure
// reproduces. The default is asserted as a LITERAL, deliberately: a check that
// read DefaultShrinkCandidates back would agree with any value it was given.
func TestShrinkCandidatesDefaultsAndRefusesUnderOne(t *testing.T) {
	if got := Parse(wellFormed()).Platform.Stages["api-sim"].ShrinkCandidates; got != 32 {
		t.Errorf("an undeclared shrink_candidates parsed as %d, want the default 32", got)
	}
	for _, tc := range []struct {
		value   int64
		refused bool
	}{
		{-1, true}, {0, true}, {1, false}, {5, false}, {200, false},
	} {
		doc := wellFormed()
		doc["stages"].(map[string]any)["api-sim"].(map[string]any)["shrink_candidates"] = tc.value
		rep := Parse(doc)
		if tc.refused {
			if rep.OK() || !names(rep.Refusals, "stages.api-sim.shrink_candidates", "at least 1") {
				t.Errorf("shrink_candidates = %d: %v, want a refusal naming the minimum", tc.value, rep.Refusals)
			}
			continue
		}
		if !rep.OK() {
			t.Errorf("shrink_candidates = %d: refused: %v", tc.value, rep.Refusals)
			continue
		}
		if got := rep.Platform.Stages["api-sim"].ShrinkCandidates; got != int(tc.value) {
			t.Errorf("shrink_candidates = %d parsed as %d", tc.value, got)
		}
	}
}
