package bindings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

// wellFormed is the decoded form of bindings for the planted-lie package, which
// is the smallest of the three: one action, one observation, one claim type, one
// role. Every case below starts from a copy.
//
// AND IT IS THE PACKAGE'S OWN VOCABULARY, read off its cases rather than
// invented: the claim type is `user.created`, the observation is `user.count`
// with source api and `of = user.created`, and the actor is alice.
func wellFormed() map[string]any {
	return map[string]any{
		"action": []any{
			map[string]any{
				"name":     "create_user",
				"context":  "api",
				"emits":    []any{"user.created"},
				"requires": []any{"can_create"},
				"creates":  []any{"user"},
				"response": map[string]any{"status": int64(201)},
			},
		},
		"observation": []any{
			map[string]any{"name": "user.count", "source": "api", "of": "user.created"},
		},
		"claim": map[string]any{
			"user.created": map[string]any{"timing": "immediate"},
		},
		"role": []any{
			map[string]any{"name": "host", "contexts": []any{"api"}},
		},
	}
}

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

func TestTheWellFormedBindingsAreAccepted(t *testing.T) {
	rep := Parse(wellFormed())
	if !rep.OK() {
		t.Fatalf("refused: %v", rep.Refusals)
	}
	b := rep.Bindings
	if len(b.Actions) != 1 || b.Actions[0].Name != "create_user" || b.Actions[0].Context != "api" {
		t.Errorf("actions %+v", b.Actions)
	}
	if len(b.Actions[0].Emits) != 1 || b.Actions[0].Emits[0] != "user.created" {
		t.Errorf("emits %v", b.Actions[0].Emits)
	}
	if len(b.Actions[0].Touches) != 0 {
		t.Errorf("touches %v: an action that names none mutates none", b.Actions[0].Touches)
	}
	if b.Actions[0].Response["status"] != int64(201) {
		t.Errorf("response %v", b.Actions[0].Response)
	}
	if len(b.Observations) != 1 || b.Observations[0].Source != wire.SourceAPI || b.Observations[0].Of != "user.created" {
		t.Errorf("observations %+v", b.Observations)
	}
	if b.Claims["user.created"].Timing != Immediate {
		t.Errorf("claims %+v", b.Claims)
	}
	if len(b.Roles) != 1 || b.Roles[0].Contexts[0] != "api" {
		t.Errorf("roles %+v", b.Roles)
	}
}

// TestWhatValidateMustRefuseSTILLPARSES is the most important test in this
// file. Eight of validate's eighteen arms are about bindings that are well
// formed and wrong; a parser that refused any of them would make that arm
// unreachable and its fixture would be testing the parser instead.
func TestWhatValidateMustRefuseStillParses(t *testing.T) {
	for _, tc := range []struct {
		arm    string
		mutate func(map[string]any)
	}{
		{"an action with no precondition", func(doc map[string]any) {
			delete(doc["action"].([]any)[0].(map[string]any), "requires")
		}},
		{"an action with an empty precondition list", func(doc map[string]any) {
			doc["action"].([]any)[0].(map[string]any)["requires"] = []any{}
		}},
		{"a binding emitting a claim type no rule consumes", func(doc map[string]any) {
			doc["action"].([]any)[0].(map[string]any)["emits"] = []any{"nobody.consumes.this"}
		}},
		{"an action emitting nothing at all", func(doc map[string]any) {
			delete(doc["action"].([]any)[0].(map[string]any), "emits")
		}},
		{"an eventual claim type with no bound", func(doc map[string]any) {
			doc["claim"].(map[string]any)["user.created"] = map[string]any{"timing": "eventual"}
		}},
		{"a browser-stage action with no UI flow", func(doc map[string]any) {
			doc["action"].([]any)[0].(map[string]any)["context"] = "browser"
		}},
		{"a declared fault with nothing behind it", func(doc map[string]any) {
			doc["uses"] = map[string]any{"faults": []any{"disk_full"}}
		}},
		{"an action touching a component no rollback provider covers", func(doc map[string]any) {
			doc["action"].([]any)[0].(map[string]any)["touches"] = []any{"uncovered-store"}
		}},
		{"a role that could reach an API action from the browser", func(doc map[string]any) {
			doc["role"].([]any)[0].(map[string]any)["contexts"] = []any{"api", "browser"}
		}},
	} {
		doc := wellFormed()
		tc.mutate(doc)
		if rep := Parse(doc); !rep.OK() {
			t.Errorf("%s: PARSED AS A REFUSAL (%v), which would make validate's arm unreachable",
				tc.arm, rep.Refusals)
		}
	}
}

// TestWhatCannotBeREADAsBindingsIsRefused is the other side: the parse refusals
// are only about a file that cannot be read as bindings at all.
func TestWhatCannotBeReadAsBindingsIsRefused(t *testing.T) {
	for _, tc := range []struct {
		name, path, want string
		mutate           func(map[string]any)
	}{
		{"no actions", "action", "no actions", func(doc map[string]any) { delete(doc, "action") }},
		{"an action with no name", "action[0].name", "missing key", func(doc map[string]any) {
			delete(doc["action"].([]any)[0].(map[string]any), "name")
		}},
		{"an action with no context", "action[0].context", "missing key", func(doc map[string]any) {
			delete(doc["action"].([]any)[0].(map[string]any), "context")
		}},
		{"an unknown key on an action", "action[0].nonesuch", "unknown key", func(doc map[string]any) {
			doc["action"].([]any)[0].(map[string]any)["nonesuch"] = true
		}},
		{"an observation with no source", "observation[0].source", "missing key", func(doc map[string]any) {
			delete(doc["observation"].([]any)[0].(map[string]any), "source")
		}},
		{"an observation with no of", "observation[0].of", "missing key", func(doc map[string]any) {
			delete(doc["observation"].([]any)[0].(map[string]any), "of")
		}},
		{"a fourth observation source", "observation[0].source", "not one of api, ui, direct", func(doc map[string]any) {
			doc["observation"].([]any)[0].(map[string]any)["source"] = "db"
		}},
		{"a direct observation naming no component", "observation[0].component", "missing key", func(doc map[string]any) {
			doc["observation"].([]any)[0].(map[string]any)["source"] = "direct"
		}},
		{"an api observation naming a component", "observation[0].component", "names no component", func(doc map[string]any) {
			doc["observation"].([]any)[0].(map[string]any)["component"] = "store"
		}},
		{"a claim with no timing", "claim.user.created.timing", "missing key", func(doc map[string]any) {
			doc["claim"].(map[string]any)["user.created"] = map[string]any{}
		}},
		{"a third timing", "claim.user.created.timing", "wanted", func(doc map[string]any) {
			doc["claim"].(map[string]any)["user.created"] = map[string]any{"timing": "eventually-ish"}
		}},
		{"an immediate claim with a bound", "claim.user.created.bound", "says nothing", func(doc map[string]any) {
			doc["claim"].(map[string]any)["user.created"] = map[string]any{"timing": "immediate", "bound": "30s"}
		}},
		{"an eventual bound that is neither a duration nor quiesce", "claim.user.created.bound", "wanted a duration", func(doc map[string]any) {
			doc["claim"].(map[string]any)["user.created"] = map[string]any{"timing": "eventual", "bound": "soonish"}
		}},
		{"a role with no contexts", "role[0].contexts", "missing key", func(doc map[string]any) {
			delete(doc["role"].([]any)[0].(map[string]any), "contexts")
		}},
		{"an action declared twice", "action", "declared twice", func(doc map[string]any) {
			first := doc["action"].([]any)[0]
			doc["action"] = []any{first, first}
		}},
		{"an observation declared twice", "observation", "declared twice", func(doc map[string]any) {
			first := doc["observation"].([]any)[0]
			doc["observation"] = []any{first, first}
		}},
		{"a role declared twice", "role", "declared twice", func(doc map[string]any) {
			first := doc["role"].([]any)[0]
			doc["role"] = []any{first, first}
		}},
		{"a clock form that is neither", "clock.form", "wanted", func(doc map[string]any) {
			doc["clock"] = map[string]any{"form": "wristwatch"}
		}},
		{"an unknown top-level key", "nonesuch", "unknown key", func(doc map[string]any) {
			doc["nonesuch"] = true
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

// TestWhatAnActionTouchesIsCarried, which is the half of the rollback-coverage
// arm an observation cannot express: an action may MUTATE a component nothing
// reads directly.
func TestWhatAnActionTouchesIsCarried(t *testing.T) {
	doc := wellFormed()
	doc["action"].([]any)[0].(map[string]any)["touches"] = []any{"store", "queue"}
	rep := Parse(doc)
	if !rep.OK() {
		t.Fatalf("refused: %v", rep.Refusals)
	}
	if got := rep.Bindings.Actions[0].Touches; len(got) != 2 || got[0] != "store" || got[1] != "queue" {
		t.Errorf("touches %v", got)
	}
	doc = wellFormed()
	doc["action"].([]any)[0].(map[string]any)["touches"] = "store"
	if rep := Parse(doc); rep.OK() || !names(rep.Refusals, "action[0].touches", "wanted an array of strings") {
		t.Errorf("a scalar touches: %v", rep.Refusals)
	}
}

// TestAUIFlowIsCheckedStepByStep, including R9's rule that a wait takes a
// condition and never a duration.
func TestAUIFlowIsCheckedStepByStep(t *testing.T) {
	flow := func(steps ...any) map[string]any {
		doc := wellFormed()
		a := doc["action"].([]any)[0].(map[string]any)
		a["context"] = "browser"
		a["flow"] = steps
		return doc
	}
	good := flow(
		map[string]any{"step": "navigate", "to": "/docs/new"},
		map[string]any{"step": "fill", "field": "title", "value": "quarterly"},
		map[string]any{"step": "click", "target": "submit"},
		map[string]any{"step": "wait", "until": "doc-created"},
	)
	rep := Parse(good)
	if !rep.OK() {
		t.Fatalf("a well-formed flow is refused: %v", rep.Refusals)
	}
	if len(rep.Bindings.Actions[0].Flow) != 4 {
		t.Fatalf("flow has %d step(s)", len(rep.Bindings.Actions[0].Flow))
	}

	for _, tc := range []struct {
		name, path, want string
		step             map[string]any
	}{
		{"a wait with a duration", "action[0].flow[0].until", "is a duration",
			map[string]any{"step": "wait", "until": "5s"}},
		{"a wait with no condition", "action[0].flow[0].until", "missing key",
			map[string]any{"step": "wait"}},
		{"a navigate to a URL", "action[0].flow[0].to", "wanted a path",
			map[string]any{"step": "navigate", "to": "https://example.test/docs/new"}},
		{"a navigate with no path", "action[0].flow[0].to", "missing key",
			map[string]any{"step": "navigate"}},
		{"a fill with no value", "action[0].flow[0].value", "missing key",
			map[string]any{"step": "fill", "field": "title"}},
		{"a click with no target", "action[0].flow[0].target", "missing key",
			map[string]any{"step": "click"}},
		{"a fifth step kind", "action[0].flow[0].step", "wanted one of",
			map[string]any{"step": "scroll"}},
		{"a click carrying a value", "action[0].flow[0].value", "unknown key",
			map[string]any{"step": "click", "target": "submit", "value": "x"}},
		{"a step with no kind", "action[0].flow[0].step", "missing key",
			map[string]any{"to": "/docs/new"}},
	} {
		rep := Parse(flow(tc.step))
		if rep.OK() {
			t.Errorf("%s: accepted", tc.name)
			continue
		}
		if !names(rep.Refusals, tc.path, tc.want) {
			t.Errorf("%s: refusals are %v", tc.name, rep.Refusals)
		}
	}
}

// TestTheReservedTablesAreCarriedAndNotInvented: present now, accepted when
// absent, and their inner keys belong to the phase that owns them -- so they are
// carried as read rather than checked against a key set this phase would be
// making up.
func TestTheReservedTablesAreCarriedAndNotInvented(t *testing.T) {
	if rep := Parse(wellFormed()); !rep.OK() {
		t.Fatalf("absent reserved tables are refused: %v", rep.Refusals)
	}
	doc := wellFormed()
	doc["clock"] = map[string]any{"endpoint": "/test/clock"}
	// THE TYPED FORM, R6.2. This read `{"probe": "/test/quiet"}` until the
	// probes were typed: the table was a reserved map, so any key passed and
	// nothing was required, and that string was a valid declaration no code
	// read. It is refused now by name -- "wanted an array of tables" -- which
	// is the narrowing fable ruled at coop r6/1 as a validate matter rather
	// than a contract-version move: nothing read the old shape at run time, so
	// no project can break at run time, and one carrying it is refused loudly
	// before any run.
	doc["quiesce"] = map[string]any{
		"probe": []any{
			map[string]any{"kind": "queue_depth", "timeout": "30s", "quiet": int64(0)},
		},
	}
	doc["deadline"] = map[string]any{"grace": "2s"}
	doc["seeding"] = map[string]any{"path": "./seed.sh"}
	doc["uses"] = map[string]any{"faults": []any{"restart"}, "volume": []any{"users"}}
	rep := Parse(doc)
	if !rep.OK() {
		t.Fatalf("the reserved tables are refused: %v", rep.Refusals)
	}
	b := rep.Bindings
	if b.Clock["endpoint"] != "/test/clock" {
		t.Errorf("clock %v", b.Clock)
	}
	// THE PROBES ARE TYPED NOW and this asserted the untyped string until
	// R6.2. The fields are read individually rather than compared as a struct
	// so a later field added to Probe does not silently make this arm vacuous.
	if len(b.Probes) != 1 {
		t.Fatalf("%d probe(s) parsed, wanted 1: %v", len(b.Probes), b.Probes)
	}
	if got := b.Probes[0]; got.Kind != "queue_depth" || got.Timeout != "30s" {
		t.Errorf("probe kind %q timeout %q", got.Kind, got.Timeout)
	}
	if b.Deadline["grace"] != "2s" || b.Seeding["path"] != "./seed.sh" {
		t.Errorf("deadline %v seeding %v", b.Deadline, b.Seeding)
	}
	if len(b.Uses.Faults) != 1 || b.Uses.Faults[0] != "restart" || len(b.Uses.Upgrade) != 0 {
		t.Errorf("uses %+v", b.Uses)
	}

	// MALFORMED IS STILL REFUSED, which is the half that makes "accepted when
	// absent" safe: a table where a table is wanted, and a list where a list is.
	doc = wellFormed()
	doc["clock"] = "on"
	if rep := Parse(doc); rep.OK() || !names(rep.Refusals, "clock", "wanted a table") {
		t.Errorf("a scalar clock: %v", rep.Refusals)
	}
	doc = wellFormed()
	doc["uses"] = map[string]any{"faults": "disk_full"}
	if rep := Parse(doc); rep.OK() || !names(rep.Refusals, "uses.faults", "wanted an array of strings") {
		t.Errorf("a scalar fault list: %v", rep.Refusals)
	}
	doc = wellFormed()
	doc["uses"] = map[string]any{"nonesuch": []any{}}
	if rep := Parse(doc); rep.OK() || !names(rep.Refusals, "uses.nonesuch", "unknown key") {
		t.Errorf("an unknown key in uses: %v", rep.Refusals)
	}
}

// TestABareReservedTableIsPRESENTAndAnAbsentOneIsNIL pins a distinction the
// parse makes and the tree reads two ways on purpose.
//
// THE FACT: the parse assigns the carried map only inside
// `if t, ok := r.Table(key, false); ok`, so an ABSENT reserved table leaves
// its field nil and a BARE one assigns an empty non-nil map. A later pass
// initializing these fields up front, or assigning unconditionally, would
// erase the difference everywhere at once with nothing else failing.
//
// WHO READS WHICH, AND WHY THAT IS NOT A CONTRADICTION: `rumor validate`'s
// arms 8 and 20 read CONTENTS -- a bare table declares nothing, which fable
// ruled at coop rumor/1359 -- while cmd/rumor/run.go reads PRESENCE for clock
// control (`brep.Bindings.Clock != nil`, R7.1). So a bare `[clock]` is "no
// clock" to the arms and "clock control" to the run. That asymmetry is
// REPORTED and not silently harmonized here: the ruling was about arm 20's
// comments, and changing what gives a run its clock is not a comment fix.
// This arm is what makes either reading observable at all.
func TestABareReservedTableIsPRESENTAndAnAbsentOneIsNIL(t *testing.T) {
	rep := Parse(wellFormed())
	if !rep.OK() {
		t.Fatalf("the well-formed bindings are refused: %v", rep.Refusals)
	}
	for name, got := range map[string]map[string]any{
		"clock":    rep.Bindings.Clock,
		"quiesce":  rep.Bindings.Quiesce,
		"deadline": rep.Bindings.Deadline,
		"seeding":  rep.Bindings.Seeding,
	} {
		if got != nil {
			t.Errorf("[%s] is absent from the document and parsed to %#v, want nil: a "+
				"consumer asking whether the project DECLARES it would read yes", name, got)
		}
	}

	doc := wellFormed()
	for _, k := range []string{"clock", "quiesce", "deadline", "seeding"} {
		doc[k] = map[string]any{}
	}
	rep = Parse(doc)
	if !rep.OK() {
		t.Fatalf("bare reserved tables are refused: %v", rep.Refusals)
	}
	for name, got := range map[string]map[string]any{
		"clock":    rep.Bindings.Clock,
		"quiesce":  rep.Bindings.Quiesce,
		"deadline": rep.Bindings.Deadline,
		"seeding":  rep.Bindings.Seeding,
	} {
		if got == nil {
			t.Errorf("a bare [%s] parsed to nil: a table with no keys is still a "+
				"declaration, and the arms that refuse on its absence would not see it", name)
			continue
		}
		if len(got) != 0 {
			t.Errorf("a bare [%s] carries %d key(s): %#v", name, len(got), got)
		}
	}
}

// TestTheClockFormIsCarriedTypedAndAbsentIsNotAFault, R7.2. The sibling of the
// arm above: [clock] is still a reserved map, and `form` is the one key in it
// this phase reads. The three accepted cells are the two forms the epic names
// and the table that says neither -- ABSENT IS NOT A PARSE FAULT, because the
// engine is the one that cannot act on a clock it has no form for, and a
// refusal here would take that reachable refusal away (fable, coop r7/51).
// The fourth cell, a form that is neither, is in the refusal table above.
func TestTheClockFormIsCarriedTypedAndAbsentIsNotAFault(t *testing.T) {
	for _, tc := range []struct {
		name  string
		table map[string]any
		want  ClockForm
	}{
		{"adapter", map[string]any{"form": "adapter"}, ClockAdapter},
		{"faked", map[string]any{"form": "faked"}, ClockFaked},
		{"no form at all", map[string]any{"endpoint": "/test/clock"}, ""},
	} {
		doc := wellFormed()
		doc["clock"] = tc.table
		rep := Parse(doc)
		if !rep.OK() {
			t.Errorf("%s: refused: %v", tc.name, rep.Refusals)
			continue
		}
		if got := rep.Bindings.ClockForm; got != tc.want {
			t.Errorf("%s: form %q, wanted %q", tc.name, got, tc.want)
		}
		// THE REST OF THE TABLE IS STILL CARRIED, which is what makes `form`
		// a typed half rather than a replacement for the reserved map.
		for k, v := range tc.table {
			if rep.Bindings.Clock[k] != v {
				t.Errorf("%s: clock[%q] is %v, wanted %v", tc.name, k, rep.Bindings.Clock[k], v)
			}
		}
	}
}

func TestEveryRefusalIsReportedNotJustTheFirst(t *testing.T) {
	doc := wellFormed()
	delete(doc["action"].([]any)[0].(map[string]any), "name")
	doc["nonesuch"] = true
	delete(doc["observation"].([]any)[0].(map[string]any), "of")
	rep := Parse(doc)
	if len(rep.Refusals) < 3 {
		t.Fatalf("three faults produced %d refusal(s): %v", len(rep.Refusals), rep.Refusals)
	}
	for _, path := range []string{"action[0].name", "nonesuch", "observation[0].of"} {
		if !names(rep.Refusals, path, "") {
			t.Errorf("no refusal at %q: %v", path, rep.Refusals)
		}
	}
}

// TestTheThreePackagesVocabularyFits is a shape check over bindings written for
// each of the three example packages, so the schema is not only consistent with
// the one package the fixture above is drawn from. role-visibility is the one
// that matters: its claims carry `visible_to`, which is a CLAIM field and not a
// binding field, so if the schema needed a place for it that would show here.
func TestTheThreePackagesVocabularyFits(t *testing.T) {
	for _, tc := range []struct {
		pkg, claimType, observation string
		roles                       []any
	}{
		{"planted-lie", "user.created", "user.count", []any{
			map[string]any{"name": "host", "contexts": []any{"api"}}}},
		{"transitive-count", "user.created", "user.count", []any{
			map[string]any{"name": "host", "contexts": []any{"api"}},
			map[string]any{"name": "guest", "contexts": []any{"api"}}}},
		{"role-visibility", "doc.created", "doc.count", []any{
			map[string]any{"name": "author", "contexts": []any{"api"}},
			map[string]any{"name": "viewer", "contexts": []any{"api"}}}},
	} {
		doc := wellFormed()
		doc["action"].([]any)[0].(map[string]any)["emits"] = []any{tc.claimType}
		doc["observation"].([]any)[0].(map[string]any)["of"] = tc.claimType
		doc["observation"].([]any)[0].(map[string]any)["name"] = tc.observation
		doc["claim"] = map[string]any{tc.claimType: map[string]any{"timing": "immediate"}}
		doc["role"] = tc.roles
		rep := Parse(doc)
		if !rep.OK() {
			t.Errorf("%s: refused: %v", tc.pkg, rep.Refusals)
		}
	}
}

// TestLoadKeepsTheTwoErrorShapesApart is internal/platform's test over this
// package's loader: an unreadable file is could-not-run, a readable file that is
// not bindings is a verdict.
func TestLoadKeepsTheTwoErrorShapesApart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bindings.toml")
	if err := os.WriteFile(path, []byte("# the decoder below ignores these bytes\n"), 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}
	rep, err := Load(path, func([]byte) (map[string]any, error) { return wellFormed(), nil })
	if err != nil || !rep.OK() {
		t.Fatalf("well-formed bindings: err %v, report %+v", err, rep)
	}
	rep, err = Load(path, func([]byte) (map[string]any, error) {
		doc := wellFormed()
		delete(doc, "action")
		return doc, nil
	})
	if err != nil {
		t.Fatalf("refused bindings came back as an error: %v", err)
	}
	if rep.OK() || !names(rep.Refusals, "action", "no actions") {
		t.Errorf("report %+v", rep)
	}
	if _, err := Load(filepath.Join(dir, "nonesuch.toml"),
		func([]byte) (map[string]any, error) { return wellFormed(), nil }); err == nil {
		t.Fatal("an absent file was accepted")
	}
}

// TestAnActionsArgumentsAreReadBySource: the four sources fable ruled at coop
// r5/68 and r5/72, each reading exactly one field, and every refusal the
// schema promises ibis at r5/78 driven here rather than discovered by their
// declarations.
func TestAnActionsArgumentsAreReadBySource(t *testing.T) {
	withArgs := func(args ...any) map[string]any {
		doc := wellFormed()
		doc["action"].([]any)[0].(map[string]any)["args"] = args
		return doc
	}

	// ALL FOUR AT ONCE, because a per-source arm cannot see one source reading
	// another's field: the four are only separable together.
	rep := Parse(withArgs(
		map[string]any{"name": "event", "source": "handle", "of": "event"},
		map[string]any{"name": "title", "source": "literal", "value": "Standup"},
		map[string]any{"name": "capacity", "source": "draw", "from": []any{int64(1), int64(5)}},
		map[string]any{"name": "who", "source": "actor"},
	))
	if !rep.OK() {
		t.Fatalf("the four sources refused: %v", rep.Refusals)
	}
	got := rep.Bindings.Actions[0].Args
	if len(got) != 4 {
		t.Fatalf("read %d argument(s), want 4: %+v", len(got), got)
	}
	if got[0].Source != ArgHandle || got[0].Of != "event" {
		t.Errorf("handle: %+v", got[0])
	}
	if got[1].Source != ArgLiteral || got[1].Value != "Standup" {
		t.Errorf("literal: %+v", got[1])
	}
	if got[2].Source != ArgDraw || len(got[2].From) != 2 {
		t.Errorf("draw: %+v", got[2])
	}
	// AN ACTOR ARGUMENT CARRIES NOTHING, and that is the assertion: if it
	// picked up a value from somewhere the project could name who acts.
	if got[3].Source != ArgActor || got[3].Of != "" || got[3].Value != nil || got[3].From != nil {
		t.Errorf("actor must carry no value of its own: %+v", got[3])
	}

	for _, c := range []struct {
		label string
		arg   map[string]any
		key   string
		says  string
	}{
		{"an unknown source", map[string]any{"name": "x", "source": "guess"},
			"source", "not an argument source"},
		{"a handle with no kind", map[string]any{"name": "x", "source": "handle"},
			"of", "missing"},
		{"a literal with no value", map[string]any{"name": "x", "source": "literal"},
			"value", "needs a `value`"},
		{"a draw with no list", map[string]any{"name": "x", "source": "draw"},
			"from", "needs a `from` list"},
		{"a draw of one value", map[string]any{"name": "x", "source": "draw", "from": "only"},
			"from", "needs a LIST"},
		{"a draw of nothing", map[string]any{"name": "x", "source": "draw", "from": []any{}},
			"from", "is empty"},
		// THE STRICT DECODER IS THE FOURTH REFUSAL I PROMISED: a field the
		// source does not use is never read, so Done() names it. Driven
		// because "the decoder will catch it" is a claim about the decoder.
		{"a literal carrying a draw's list", map[string]any{
			"name": "x", "source": "literal", "value": "v", "from": []any{"a"}},
			"from", "unknown key"},
		{"a handle carrying a value", map[string]any{
			"name": "x", "source": "handle", "of": "event", "value": "v"},
			"value", "unknown key"},
		{"an actor carrying anything", map[string]any{
			"name": "x", "source": "actor", "value": "v"},
			"value", "unknown key"},
	} {
		rep := Parse(withArgs(c.arg))
		if rep.OK() {
			t.Errorf("%s: accepted", c.label)
			continue
		}
		if !names(rep.Refusals, "action[0].args[0]."+c.key, c.says) {
			t.Errorf("%s: refusals %v", c.label, rep.Refusals)
		}
	}

	// AN ARGUMENT DECLARED TWICE IS REFUSED, with its control beside it: two
	// arguments with DIFFERENT names are the ordinary case, so a check that
	// fired on both would be refusing the plural rather than the collision.
	// The composer builds a map keyed by name, so the second entry would
	// silently win and the request would carry whichever the file put last.
	if dup := Parse(withArgs(
		map[string]any{"name": "title", "source": "literal", "value": "a"},
		map[string]any{"name": "title", "source": "literal", "value": "b"},
	)); dup.OK() {
		t.Error("`title` declared twice was accepted: the second entry would overwrite the first")
	} else if !names(dup.Refusals, "action", `declares argument "title" twice`) {
		t.Errorf("the duplicate must be named: %v", dup.Refusals)
	}
	if two := Parse(withArgs(
		map[string]any{"name": "title", "source": "literal", "value": "a"},
		map[string]any{"name": "other", "source": "literal", "value": "b"},
	)); !two.OK() {
		t.Errorf("two differently named arguments must parse: %v", two.Refusals)
	}

	// AND NO ARGS AT ALL IS STILL A BINDING. An action the system takes with
	// no arguments declares none, so the optional half needs its own cell --
	// otherwise every existing example would have to gain an empty table.
	if rep := Parse(wellFormed()); !rep.OK() {
		t.Errorf("an action with no args must still parse: %v", rep.Refusals)
	} else if n := len(rep.Bindings.Actions[0].Args); n != 0 {
		t.Errorf("an action declaring no args has %d", n)
	}
}
