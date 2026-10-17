package bindings

import (
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
	doc["quiesce"] = map[string]any{"probe": "/test/quiet"}
	doc["deadline"] = map[string]any{"grace": "2s"}
	doc["seeding"] = map[string]any{"path": "./seed.sh"}
	doc["uses"] = map[string]any{"faults": []any{"restart"}, "volume": []any{"users"}}
	rep := Parse(doc)
	if !rep.OK() {
		t.Fatalf("the reserved tables are refused: %v", rep.Refusals)
	}
	b := rep.Bindings
	if b.Clock["endpoint"] != "/test/clock" || b.Quiesce["probe"] != "/test/quiet" {
		t.Errorf("clock %v quiesce %v", b.Clock, b.Quiesce)
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
