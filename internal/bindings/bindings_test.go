package bindings

import (
	"forge.axonibyte.com/axonibyte/rumor/internal/decl"
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
// probes wraps one probe table as a `[quiesce]` carrying it, which is the
// shape the cells below mutate.
func probes(p map[string]any) map[string]any {
	return map[string]any{"probe": []any{p}}
}

// TestAProbesDurationsAreReadAtTheParseAndTheirTextIsKept is the CONTROL for
// the four refusal cells above, and the half that would otherwise be silent:
// a declaration that is fine must still PARSE, and the raw spelling must
// survive, because `internal/quiesce` quotes it back ("its declared timeout of
// -5s") and `30s` and `30000ms` are not one sentence to a reader.
func TestAProbesDurationsAreReadAtTheParseAndTheirTextIsKept(t *testing.T) {
	doc := wellFormed()
	// THE SPELLINGS ARE CHOSEN SO A NORMALIZATION WOULD SHOW. `2000ms` and
	// `0.25s` are what the declaration says; `time.Duration.String()` renders
	// the same two values as `2s` and `250ms`, so a producer that handed back
	// the PARSED duration instead of the text fails here. Written with `2s`
	// and `250ms` first, this cell passed against exactly that mutant --
	// String() returns those two unchanged, so the arm could not tell the text
	// from the number it was guarding against.
	doc["quiesce"] = probes(map[string]any{
		"kind": "queue_depth", "timeout": "2000ms", "every": "0.25s", "quiet": int64(0)})
	rep := Parse(doc)
	if !rep.OK() {
		t.Fatalf("a probe declaring 2000ms and 0.25s is refused: %v", rep.Refusals)
	}
	if len(rep.Bindings.Probes) != 1 {
		t.Fatalf("%d probe(s) parsed, want 1", len(rep.Bindings.Probes))
	}
	if got := rep.Bindings.Probes[0]; got.Timeout != "2000ms" || got.Every != "0.25s" {
		t.Errorf("timeout %q every %q, want \"2000ms\" and \"0.25s\": the TEXT is what a later "+
			"tier quotes, so a validated duration handed back as a number would change "+
			"every message that names it", got.Timeout, got.Every)
	}
	// AND AN ABSENT `every` IS STILL ABSENT: `Duration` returns zero for a
	// missing key and refuses nothing, so the optional half must not acquire
	// a value here -- internal/quiesce reads "" as "use the default".
	doc = wellFormed()
	doc["quiesce"] = probes(map[string]any{"kind": "queue_depth", "timeout": "30s"})
	rep = Parse(doc)
	if !rep.OK() {
		t.Fatalf("a probe with no `every` is refused: %v", rep.Refusals)
	}
	if got := rep.Bindings.Probes[0].Every; got != "" {
		t.Errorf("an absent `every` parsed to %q, want empty", got)
	}
}

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
		// THE PROBE DURATIONS, fable's ruling at coop rumor/1359 item 2 on
		// ratchet's /1356: each of these passed `rumor validate` at 23 of 23
		// arms with 0 refusals, and internal/quiesce then re-parsed the value
		// with the error dropped. The refusal is decl's own sentence at the
		// key's own path.
		{"a probe timeout of zero", "quiesce.probe[0].timeout", "covers nothing", func(doc map[string]any) {
			doc["quiesce"] = probes(map[string]any{"kind": "queue_depth", "timeout": "0s"})
		}},
		{"a negative probe timeout", "quiesce.probe[0].timeout", "covers nothing", func(doc map[string]any) {
			doc["quiesce"] = probes(map[string]any{"kind": "queue_depth", "timeout": "-5s"})
		}},
		{"a probe `every` of zero", "quiesce.probe[0].every", "covers nothing", func(doc map[string]any) {
			doc["quiesce"] = probes(map[string]any{
				"kind": "queue_depth", "timeout": "30s", "every": "0s"})
		}},
		{"a probe timeout that is not a duration", "quiesce.probe[0].timeout", "wanted a duration", func(doc map[string]any) {
			doc["quiesce"] = probes(map[string]any{"kind": "queue_depth", "timeout": "soonish"})
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
	// THE TYPED FORM, R7.4, BESIDE A SIBLING KEY THAT IS STILL CARRIED --
	// which is the half of the [quiesce] split worth repeating here: typing
	// `instants` must not turn a key a later phase reserves into a refusal.
	doc["deadline"] = map[string]any{
		"instants": []any{"invitation_expiry"},
		"grace":    "2s",
	}
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
	// READ INDIVIDUALLY, like the probe above and for its reason: a later
	// field on the typed half must not make this arm vacuous.
	if len(b.DeadlineInstants) != 1 || b.DeadlineInstants[0] != "invitation_expiry" {
		t.Errorf("deadline instants %#v", b.DeadlineInstants)
	}
	// AND THE TYPED KEY IS CARRIED TOO, which is what `Any` over `Keys` does
	// for every reserved table: a consumer reading the map sees `instants`
	// beside `grace`. Asserted so that moving the typed read OUT of the
	// carried loop would fail here rather than silently narrow what a later
	// phase can find in the map.
	if _, ok := b.Deadline["instants"]; !ok {
		t.Errorf("the typed key is not carried: %#v", b.Deadline)
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
	doc["deadline"] = map[string]any{"instants": "invitation_expiry"}
	if rep := Parse(doc); rep.OK() || !names(rep.Refusals, "deadline.instants", "wanted an array of strings") {
		t.Errorf("a scalar instant list: %v", rep.Refusals)
	}
	doc = wellFormed()
	doc["deadline"] = map[string]any{"instants": []any{"invitation_expiry", int64(3)}}
	if rep := Parse(doc); rep.OK() || !names(rep.Refusals, "deadline.instants[1]", "wanted a string") {
		t.Errorf("a non-string instant: %v", rep.Refusals)
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
		"refusal":  rep.Bindings.Refusal,
		"seeding":  rep.Bindings.Seeding,
	} {
		if got != nil {
			t.Errorf("[%s] is absent from the document and parsed to %#v, want nil: a "+
				"consumer asking whether the project DECLARES it would read yes", name, got)
		}
	}

	doc := wellFormed()
	for _, k := range []string{"clock", "quiesce", "deadline", "refusal", "seeding"} {
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
		"refusal":  rep.Bindings.Refusal,
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
	// AND THE TYPED HALVES ARE EMPTY FOR A BARE TABLE, which is the same
	// distinction one level in: a bare [deadline] is PRESENT and declares no
	// instant, so a consumer asking for targets gets none while arm 20 still
	// sees a table. The probes carry the same property and have since R6.2.
	if rep.Bindings.DeadlineInstants != nil {
		t.Errorf("a bare [deadline] declares instants %#v", rep.Bindings.DeadlineInstants)
	}
	if rep.Bindings.Probes != nil {
		t.Errorf("a bare [quiesce] declares probes %#v", rep.Bindings.Probes)
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

// TestClockControlledAtReadsTheProjectAndNotAGuess is the question `rumor
// check` and `rumor attaint` ask before loading a committed ledger (R7.4):
// both passed FALSE unconditionally until this landed, so a clocked project's
// own cases could not load at all.
//
// THREE CELLS BECAUSE THERE ARE THREE ANSWERS, and the third is the one a
// default would get wrong: a project that declares `[clock]`, one that does
// not, and a DIRECTORY WITH NO bindings.toml -- which three of the five
// committed examples are, being statute fixtures. A missing file means no
// clock control rather than an error, because the verb that refuses a project
// for its declarations is `rumor validate` and these readers are not it.
func TestClockControlledAtReadsTheProjectAndNotAGuess(t *testing.T) {
	write := func(dir, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "bindings.toml"), []byte(body), 0o600); err != nil {
			t.Fatalf("writing bindings.toml: %v", err)
		}
	}
	// THE MINIMUM A PROJECT MUST SAY to parse at all: one action with a
	// precondition and the claim type it emits. Anything less is refused by
	// the parser and `ClockControlledAt` answers false for a project it could
	// not read -- which is a fourth answer and not this cell's subject.
	const minimum = `[[action]]
name = "do"
context = "none"
emits = ["thing.done"]
requires = ["may_do"]

[claim."thing.done"]
timing = "immediate"
`
	clocked := t.TempDir()
	write(clocked, minimum+"\n[clock]\nform = \"faked\"\n")
	if !ClockControlledAt(clocked, decl.TOML) {
		rep, err := Load(filepath.Join(clocked, "bindings.toml"), decl.TOML)
		t.Errorf("a project declaring [clock] reads as unclocked: parse ok=%v err=%v refusals=%v",
			rep.OK(), err, rep.Refusals)
	}

	plain := t.TempDir()
	write(plain, minimum)
	if ClockControlledAt(plain, decl.TOML) {
		t.Error("a project declaring no [clock] reads as clocked, which would let a committed " +
			"ledger carry a clock fact the vocabulary forbids there")
	}

	empty := t.TempDir()
	if ClockControlledAt(empty, decl.TOML) {
		t.Error("a directory with no bindings.toml reads as clocked")
	}

	// AND THE TABLE'S PRESENCE IS THE TEST, NOT ITS CONTENTS: a bare `[clock]`
	// is clock control to a RUN, which is the asymmetry
	// TestABareReservedTableIsPRESENTAndAnAbsentOneIsNIL exists to make
	// visible. A reader that required a form here would disagree with
	// cmd/rumor/run.go about the same project.
	bare := t.TempDir()
	write(bare, minimum+"\n[clock]\n")
	if !ClockControlledAt(bare, decl.TOML) {
		t.Error("a bare [clock] reads as unclocked here, where cmd/rumor/run.go reads it as control")
	}
}

// `[refusal].message` IS THE TYPED HALF, and these are the three answers a
// parser owes about it: the path is carried as declared, a bare table declares
// no path and is not a fault, and a non-string is refused at the key's own
// path.
//
// WHAT IS NOT HERE, DELIBERATELY: whether the path RESOLVES against a live
// answer. That is a fact about a system's reply and belongs to the reader that
// has one -- internal/fuzz, which refuses by name when nothing was found at
// the declared path. A parser that tried would be checking a declaration
// against nothing.
func TestTheRefusalMessagePathIsCarriedAndATypeFaultIsRefused(t *testing.T) {
	doc := wellFormed()
	doc["refusal"] = map[string]any{"message": "body.error"}
	rep := Parse(doc)
	if !rep.OK() {
		t.Fatalf("a declared refusal path is refused: %v", rep.Refusals)
	}
	if rep.Bindings.RefusalMessage != "body.error" {
		t.Errorf("the declared path did not survive the parse: %q", rep.Bindings.RefusalMessage)
	}

	// A BARE TABLE DECLARES NO PATH AND IS NOT A FAULT, which is the
	// convention [quiesce] set and [clock] and [deadline] kept: a key a later
	// phase reserves here must not be refused by this one.
	doc = wellFormed()
	doc["refusal"] = map[string]any{}
	rep = Parse(doc)
	if !rep.OK() {
		t.Fatalf("a bare [refusal] is refused: %v", rep.Refusals)
	}
	if rep.Bindings.RefusalMessage != "" {
		t.Errorf("a bare table yielded a path: %q", rep.Bindings.RefusalMessage)
	}
	if rep.Bindings.Refusal == nil {
		t.Error("a bare [refusal] parsed to nil, so a consumer asking whether the project " +
			"DECLARES the table would read no")
	}

	// AND THE SIBLING KEYS ARE CARRIED, so a project is not refused for a key
	// a later phase reserves in this table.
	doc = wellFormed()
	doc["refusal"] = map[string]any{"message": "body.error", "code": "body.code"}
	rep = Parse(doc)
	if !rep.OK() {
		t.Fatalf("a [refusal] with a sibling key is refused: %v", rep.Refusals)
	}
	if rep.Bindings.Refusal["code"] != "body.code" {
		t.Errorf("the sibling key was dropped: %#v", rep.Bindings.Refusal)
	}

	// A NON-STRING IS REFUSED AT THE KEY'S OWN PATH. A path is a string; a
	// table or a number there is this phase's own field spelled wrong, which
	// issue 63's rule refuses rather than carries.
	doc = wellFormed()
	doc["refusal"] = map[string]any{"message": 7}
	rep = Parse(doc)
	if rep.OK() {
		t.Fatal("a numeric refusal path parsed, so a project can declare a path that is not one")
	}
	var named bool
	for _, ref := range rep.Refusals {
		if strings.Contains(ref.Error(), "refusal.message") {
			named = true
		}
	}
	if !named {
		t.Errorf("the refusal does not name refusal.message, so the author is sent looking: %v",
			rep.Refusals)
	}
}

// `[[action]].contended` IS THE CONCURRENCY STAGE's SUBJECT, DECLARED BY NAME.
//
// fable ruled it into 109 at coop rumor/1525 after rook measured why the
// tempting derivation is the dangerous one: `store_event_count` declares
// `of = "event.joined"` and THREE actions emit that claim type, of which
// accept_invitation holds its lock across the check and the insert. A stage
// deriving its subject from the claim type can therefore select the CONTROL and
// report no overfill -- which is exactly what a correct system reports.
func TestWhatAnActionContendsOnIsCarried(t *testing.T) {
	doc := wellFormed()
	acts, ok := doc["action"].([]any)
	if !ok || len(acts) == 0 {
		t.Fatalf("the fixture has no actions to declare contention on: %T", doc["action"])
	}
	first, ok := acts[0].(map[string]any)
	if !ok {
		t.Fatalf("the fixture's first action is %T", acts[0])
	}
	first["contended"] = map[string]any{
		"limit": "create_event.capacity", "held": "store_event_count",
	}
	rep := Parse(doc)
	if !rep.OK() {
		t.Fatalf("a declared contention is refused: %v", rep.Refusals)
	}
	got := rep.Bindings.Actions[0].Contended
	if got.Limit != "create_event.capacity" || got.Held != "store_event_count" {
		t.Errorf("contended %+v did not survive the parse", got)
	}

	// AN ACTION THAT DECLARES NONE CONTENDS ON NOTHING, which is every action
	// in a project the concurrency stage has nothing to do with.
	rep = Parse(wellFormed())
	if !rep.OK() {
		t.Fatalf("the well-formed bindings are refused: %v", rep.Refusals)
	}
	if c := rep.Bindings.Actions[0].Contended; c.Limit != "" || c.Held != "" {
		t.Errorf("an action declaring no contention parsed to %+v, so a stage asking which "+
			"actions contend would be given one that does not", c)
	}
}

// HALF A SUBJECT IS REFUSED AT THE KEY's OWN PATH. An action saying it contends
// without saying where its limit is read has declared something a stage cannot
// judge: an overfill is a comparison against a limit, and a limit it cannot
// read is not a limit.
func TestAContentionMissingEitherHalfIsRefusedByName(t *testing.T) {
	for _, c := range []struct {
		name  string
		table map[string]any
		want  string
	}{
		{"no limit", map[string]any{"held": "store_event_count"}, "limit"},
		{"no held", map[string]any{"limit": "create_event.capacity"}, "held"},
		{"neither", map[string]any{}, "limit"},
	} {
		doc := wellFormed()
		acts := doc["action"].([]any)
		acts[0].(map[string]any)["contended"] = c.table
		rep := Parse(doc)
		if rep.OK() {
			t.Errorf("%s: a half-declared contention parsed, so a stage would contend against a "+
				"limit it cannot read", c.name)
			continue
		}
		var named bool
		for _, ref := range rep.Refusals {
			if strings.Contains(ref.Error(), "contended."+c.want) {
				named = true
			}
		}
		if !named {
			t.Errorf("%s: the refusal does not name contended.%s: %v", c.name, c.want, rep.Refusals)
		}
	}
}
