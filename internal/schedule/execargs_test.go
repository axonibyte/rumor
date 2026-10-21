package schedule

import (
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
)

func act(args ...bindings.Arg) bindings.Action {
	return bindings.Action{Name: "create_event", Args: args}
}

// THE FOUR SOURCES COMPOSE TOGETHER, which is the only way to see one source
// reading another's field: a per-source arm cannot.
func TestTheFourSourcesComposeAnExecutesArgs(t *testing.T) {
	tab := table(t, map[string]string{"member_1.event#1": "ev-7"})
	a := act(
		bindings.Arg{Name: "event", Source: bindings.ArgHandle, Of: "event"},
		bindings.Arg{Name: "title", Source: bindings.ArgLiteral, Value: "Standup"},
		bindings.Arg{Name: "capacity", Source: bindings.ArgDraw, From: []any{int64(2)}},
		bindings.Arg{Name: "name", Source: bindings.ArgActor},
	)
	got, err := ExecuteArgs(a, "member_1", map[string]string{"event": "member_1.event#1"},
		New(42), tab)
	if err != nil {
		t.Fatalf("refused a complete declaration: %v", err)
	}
	// THE HANDLE RESOLVED TO AN ID AND NOT TO ITSELF: a handle on the wire is
	// the breach contract 2 exists to prevent.
	if got["event"] != "ev-7" {
		t.Errorf("args[event] = %v, want the id ev-7", got["event"])
	}
	if got["title"] != "Standup" {
		t.Errorf("args[title] = %v", got["title"])
	}
	if got["capacity"] != int64(2) {
		t.Errorf("args[capacity] = %v (%T), want the one value its list holds", got["capacity"], got["capacity"])
	}
	// THE ACTOR IS THE STEP'S, NOT THE PROJECT'S. The wire carries no `actor`
	// field (contract.md's execute row), so this is the only way the toy --
	// which keys sessions by user name -- can be told who is acting.
	if got["name"] != "member_1" {
		t.Errorf("args[name] = %v, want the acting actor member_1", got["name"])
	}
	if len(got) != 4 {
		t.Errorf("composed %d argument(s), want 4: %v", len(got), got)
	}
}

// A LITERAL IS CARRIED AS DECLARED, including the values a truthiness test
// would drop. A project writes `false` and `0` down to be explicit, and an
// argument that vanished for being falsy would send a request missing exactly
// the field the project was most deliberate about.
func TestALiteralKeepsFalseAndZero(t *testing.T) {
	got, err := ExecuteArgs(act(
		bindings.Arg{Name: "public", Source: bindings.ArgLiteral, Value: false},
		bindings.Arg{Name: "seats", Source: bindings.ArgLiteral, Value: int64(0)},
		bindings.Arg{Name: "note", Source: bindings.ArgLiteral, Value: ""},
	), "member_1", nil, nil, nil)
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("composed %d, want 3: %v", len(got), got)
	}
	if got["public"] != false || got["seats"] != int64(0) || got["note"] != "" {
		t.Errorf("a falsy literal was not carried through: %v", got)
	}
}

// AN EXECUTE'S `args` IS ALWAYS PRESENT, which is where this parts company with
// ObserveArgs on purpose. Measured, not assumed: contract.md's observe row
// carries "present exactly when the observation declares any" and the execute
// row does not, and the structs agree -- ObserveRequest.Args is
// `json:"args,omitempty"` and ExecuteRequest.Args is `json:"args"`, so a nil
// map would marshal to `"args":null` rather than to an absent field.
func TestAnActionWithNoArgumentsComposesAnEmptyObject(t *testing.T) {
	got, err := ExecuteArgs(act(), "member_1", nil, nil, nil)
	if err != nil {
		t.Fatalf("refused an action with no arguments: %v", err)
	}
	if got == nil {
		t.Error("composed nil, which marshals to `\"args\":null`: the field has no omitempty and " +
			"null is not the empty object the walk sends today")
	}
	if len(got) != 0 {
		t.Errorf("composed %v for an action declaring nothing", got)
	}
}

// THE DRAWS REPLAY FROM THE SEED AND ARE CONSUMED IN DECLARATION ORDER. Issue
// 72's done-when compares two attempts' sequences, so a draw whose order or
// count moved with the schema would make one seed walk two ways.
func TestTheDrawsReplayFromTheSeed(t *testing.T) {
	a := act(
		bindings.Arg{Name: "first", Source: bindings.ArgDraw, From: []any{"a", "b", "c", "d", "e"}},
		bindings.Arg{Name: "second", Source: bindings.ArgDraw, From: []any{"a", "b", "c", "d", "e"}},
	)
	one, err := ExecuteArgs(a, "member_1", nil, New(99), nil)
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	two, err := ExecuteArgs(a, "member_1", nil, New(99), nil)
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	if one["first"] != two["first"] || one["second"] != two["second"] {
		t.Errorf("one seed drew twice differently: %v then %v", one, two)
	}

	// AND THE ARM ASSERTS ITS OWN REACH: two arguments drawing the same value
	// every time would satisfy the comparison above and prove nothing, so some
	// seed must separate them.
	separated := false
	for _, seed := range []uint64{1, 2, 3, 4, 5, 6, 7, 8} {
		got, err := ExecuteArgs(a, "member_1", nil, New(seed), nil)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		if got["first"] != got["second"] {
			separated = true
			break
		}
	}
	if !separated {
		t.Error("eight seeds drew both arguments identically, so the second draw is not a second " +
			"draw: the generator is being consumed once or not at all")
	}
}

// THE REFUSALS, each naming what was wrong rather than failing a request later.
func TestExecuteArgsRefusals(t *testing.T) {
	tab := table(t, map[string]string{"member_1.event#1": "ev-7", "member_1.user#1": "u-1"})
	handleArg := bindings.Arg{Name: "event", Source: bindings.ArgHandle, Of: "event"}

	cases := []struct {
		label     string
		a         bindings.Action
		actor     string
		chosen    map[string]string
		src       *Source
		withTable bool
		says      string
	}{
		{"a handle argument the walk chose nothing for",
			act(handleArg), "member_1", nil, nil, true, "chose no handle for it"},
		{"a chosen handle of the wrong kind",
			act(handleArg), "member_1", map[string]string{"event": "member_1.user#1"}, nil, true,
			"well-formed id for the wrong entity"},
		{"a chosen handle that is not a handle",
			act(handleArg), "member_1", map[string]string{"event": "ev-7"}, nil, true,
			"not a run-local handle"},
		{"a chosen handle nothing bound",
			act(handleArg), "member_1", map[string]string{"event": "member_1.event#9"}, nil, true,
			"cannot resolve"},
		{"a handle argument with no table",
			act(handleArg), "member_1", map[string]string{"event": "member_1.event#1"}, nil, false,
			"no handle table"},
		{"a draw from an empty list",
			act(bindings.Arg{Name: "x", Source: bindings.ArgDraw, From: []any{}}),
			"member_1", nil, New(1), true, "draws from an empty list"},
		{"a draw with no generator",
			act(bindings.Arg{Name: "x", Source: bindings.ArgDraw, From: []any{"a"}}),
			"member_1", nil, nil, true, "no generator"},
		{"an actor argument with no actor",
			act(bindings.Arg{Name: "name", Source: bindings.ArgActor}),
			"", nil, nil, true, "no actor was given"},
		{"a source that is not one of the four",
			act(bindings.Arg{Name: "x", Source: "guess"}),
			"member_1", nil, nil, true, "not one of"},
		{"a handle chosen for an argument that takes none",
			act(bindings.Arg{Name: "title", Source: bindings.ArgLiteral, Value: "t"}),
			"member_1", map[string]string{"nonesuch": "member_1.event#1"}, nil, true,
			"declares no handle argument named"},
	}
	for _, c := range cases {
		var tb = tab
		if !c.withTable {
			tb = nil
		}
		got, err := ExecuteArgs(c.a, c.actor, c.chosen, c.src, tb)
		if err == nil {
			t.Errorf("%s: accepted, composing %v", c.label, got)
			continue
		}
		if !strings.Contains(err.Error(), c.says) {
			t.Errorf("%s: refusal must say %q; got %v", c.label, c.says, err)
		}
		if got != nil {
			t.Errorf("%s: refused AND composed %v -- a caller reading the map past the error "+
				"would send a partial request", c.label, got)
		}
	}
}
