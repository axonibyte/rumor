package concurrency

import (
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
)

// THE SWEEP IS THE BASELINE AND THEN ONE COORDINATE AT A TIME, with the limit
// pinned. Both halves matter: a sweep that only ever sent the baseline would
// report the project's first declared world as the whole of what it tried, and
// a sweep that moved the limit would change how many contenders the fan-out
// needs after the sessions were opened.
func TestTheSweepMovesOneCoordinateAtATimeAndNeverTheLimit(t *testing.T) {
	limitAction := bindings.Action{Name: "open_room", Creates: []string{"room"},
		Args: []bindings.Arg{
			{Name: "seats", Source: bindings.ArgDraw, From: []any{3, 1, 2}},
			{Name: "shown_to", Source: bindings.ArgDraw, From: []any{"staff", "all"}},
			{Name: "title", Source: bindings.ArgLiteral, Value: "Standup"},
		}}
	sub := Subject{Action: bindings.Action{Name: "take_seat"},
		LimitAction: "open_room", LimitArg: "seats"}

	got := sweep([]bindings.Action{limitAction}, sub, 1)
	// One baseline, plus one try per further value of the ONE argument that is
	// not the limit: 1 + (2-1) = 2.
	if len(got) != 2 {
		t.Fatalf("%d world(s): the baseline plus one per further value of each drawn argument "+
			"that is not the limit. Got %v", len(got), rendered(got))
	}
	for i, c := range got {
		v, ok := c.valueFor("open_room", "seats")
		if !ok {
			t.Fatalf("world %d sends no seats at all: the limit must be in every try", i+1)
		}
		if n, _ := asInt(v); n != 1 {
			t.Errorf("world %d sends seats=%v; the limit is pinned to the value the width was "+
				"derived from", i+1, v)
		}
	}
	if v, _ := got[0].valueFor("open_room", "shown_to"); v != "staff" {
		t.Errorf("the baseline sends shown_to=%v; it is the project's FIRST declared value, which "+
			"is the world a reader derives by hand from the file", v)
	}
	if v, _ := got[1].valueFor("open_room", "shown_to"); v != "all" {
		t.Errorf("the second world sends shown_to=%v; the sweep moves the coordinate", v)
	}
	// A LITERAL IS NOT A COORDINATE: the project fixed it, so there is nothing
	// to sweep and no try should carry it as a choice.
	for i, c := range got {
		if _, ok := c.valueFor("open_room", "title"); ok {
			t.Errorf("world %d carries the literal `title` as a swept choice", i+1)
		}
	}
	t.Logf("PROOF %v", rendered(got))
}

// THE LIMIT IS THE SMALLEST THE PROJECT DECLARED, which is what keeps the
// fan-out to the smallest width that can exceed it -- and keeps the actors a
// run needs down to what a venire declares.
func TestTheLimitIsTheSmallestDeclaredValue(t *testing.T) {
	sub := Subject{Action: bindings.Action{Name: "take_seat"},
		LimitAction: "open_room", LimitArg: "seats"}
	got, err := limitValue(sub, bindings.Arg{Name: "seats", Source: bindings.ArgDraw,
		From: []any{3, 1, 2}})
	if err != nil {
		t.Fatalf("limitValue: %v", err)
	}
	if got != 1 {
		t.Errorf("the limit is %d; the smallest declared value is 1, and Width is limit plus one", got)
	}
	if w := Width(got); w != 2 {
		t.Errorf("the width is %d", w)
	}
	lit, err := limitValue(sub, bindings.Arg{Name: "seats", Source: bindings.ArgLiteral, Value: int64(4)})
	if err != nil {
		t.Fatalf("limitValue over a literal: %v", err)
	}
	if lit != 4 {
		t.Errorf("a literal limit read as %d", lit)
	}
}

// A LIMIT THAT IS NOT A COUNT IS REFUSED, naming the value and its type: a
// fan-out cannot be sized from a word, and a stage that fell back to a default
// would be contending against a limit the project never declared.
func TestALimitThatIsNotACountIsRefused(t *testing.T) {
	sub := Subject{Action: bindings.Action{Name: "take_seat"},
		LimitAction: "open_room", LimitArg: "seats"}
	for _, arg := range []bindings.Arg{
		{Name: "seats", Source: bindings.ArgDraw, From: []any{"many", "few"}},
		{Name: "seats", Source: bindings.ArgLiteral, Value: "many"},
		{Name: "seats", Source: bindings.ArgDraw, From: []any{1.5}},
	} {
		if _, err := limitValue(sub, arg); err == nil {
			t.Errorf("%v returned a limit", arg)
		} else {
			t.Logf("PROOF %v", err)
		}
	}
}

// A VENIRE TOO SMALL FOR THE HOST AND EVERY CONTENDER IS REFUSED BY NAME,
// before a single session is opened.
//
// IT IS A REFUSAL AND NOT A NARROWER FAN-OUT. Two requests from one actor have
// the second refused by the precondition -- the toy's `may_join` is "has not
// already joined" -- so the acceptances would fall for a reason that is not the
// limit, and the verdict would read as a system that held it. This arm passes
// no session at all, which is the proof that the check comes first: a Run that
// opened anything before counting the actors would panic here rather than
// refuse.
func TestAVenireTooSmallIsRefusedBeforeAnythingIsOpened(t *testing.T) {
	b := bindings.Bindings{
		Actions: []bindings.Action{
			{Name: "open_room", Creates: []string{"room"}, Args: []bindings.Arg{
				{Name: "seats", Source: bindings.ArgLiteral, Value: 2}}},
			{Name: "take_seat", Contended: bindings.Contended{
				Limit: "open_room.seats", Held: "room_count"},
				Args: []bindings.Arg{{Name: "room", Source: bindings.ArgHandle, Of: "room"}}},
		},
		Observations: []bindings.Observation{{Name: "room_count"}},
	}
	_, err := Run(b, nil, nil, []string{"admin_1", "member_1"}, 0)
	if err == nil {
		t.Fatal("a venire of 2 contended a limit of 2, which needs a host and 3 contenders")
	}
	for _, want := range []string{"take_seat", "3 contender(s)", "2 actor(s)"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not name %q: %v", want, err)
		}
	}
	t.Logf("PROOF %v", err)
}

// `contended.held` NAMING NO DECLARED OBSERVATION IS REFUSED, and the refusal
// lists the ones that are declared. The verdict rests on the read-back, so a
// stage that went on without it would be judging the overfill clause on the
// acceptances alone -- a claim about RESPONSES and not about the world, which
// is the reading rook corrected at coop r8/45.
func TestAHeldQueryNothingDeclaresIsRefused(t *testing.T) {
	b := bindings.Bindings{
		Actions: []bindings.Action{
			{Name: "open_room", Creates: []string{"room"}, Args: []bindings.Arg{
				{Name: "seats", Source: bindings.ArgLiteral, Value: 1}}},
			{Name: "take_seat", Contended: bindings.Contended{
				Limit: "open_room.seats", Held: "seat_count"},
				Args: []bindings.Arg{{Name: "room", Source: bindings.ArgHandle, Of: "room"}}},
		},
		Observations: []bindings.Observation{{Name: "room_count"}, {Name: "admin_count"}},
	}
	_, err := Run(b, nil, nil, []string{"a", "b", "c"}, 0)
	if err == nil {
		t.Fatal("a held query nothing declares was contended")
	}
	for _, want := range []string{"seat_count", "admin_count", "room_count"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not name %q: %v", want, err)
		}
	}
	t.Logf("PROOF %v", err)
}

// A HANDLE THE SETUP NEVER OBTAINED IS REFUSED RATHER THAN SENT EMPTY: a
// request carrying a name the system never minted would be answered 404, and
// the fan-out would report that as a system that refused under contention.
func TestAHandleTheSetupNeverObtainedIsRefused(t *testing.T) {
	a := bindings.Action{Name: "take_seat", Args: []bindings.Arg{
		{Name: "room", Source: bindings.ArgHandle, Of: "room"}}}
	if _, err := arguments(a, "member_1", map[string]string{}, nil); err == nil {
		t.Fatal("a handle argument with no id composed a request")
	} else if !strings.Contains(err.Error(), "room") {
		t.Errorf("the refusal does not name the kind: %v", err)
	}
}

func rendered(cs []choice) [][]string {
	out := make([][]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.render())
	}
	return out
}
