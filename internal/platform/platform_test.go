package platform

import (
	"strings"
	"testing"
	"time"
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
		{"a profile with no seeds", "profiles.nightly.seeds", "runs nothing", func(doc map[string]any) {
			doc["profiles"].(map[string]any)["nightly"].(map[string]any)["seeds"] = int64(0)
		}},
		{"a zero budget", "profiles.nightly.budget", "runs nothing", func(doc map[string]any) {
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
