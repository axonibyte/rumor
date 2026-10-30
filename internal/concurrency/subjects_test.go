package concurrency

import (
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
)

func contends(name, limit, held string) bindings.Action {
	return bindings.Action{Name: name, Contended: bindings.Contended{Limit: limit, Held: held}}
}

// EVERY ACTION THAT DECLARES IT AND NO OTHER, which is the whole of why the
// subject is declared rather than derived: the toy's `store_event_count`
// declares `of = "event.joined"` and three actions emit that type, one of them
// the control that holds its lock.
func TestSubjectsAreTheDeclaredActionsAndNoOthers(t *testing.T) {
	b := bindings.Bindings{Actions: []bindings.Action{
		contends("join_event", "create_event.capacity", "store_event_count"),
		{Name: "accept_invitation"}, // emits the same claim type; declares nothing
		{Name: "join_event_ui"},     // likewise
		contends("borrow_book", "shelve_book.copies", "store_copy_count"),
	}}
	got, err := Subjects(b)
	if err != nil {
		t.Fatalf("Subjects: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("%d subject(s), wanted 2: %+v", len(got), got)
	}
	// ORDER IS BY ACTION NAME, so two runs of one project contend alike.
	if got[0].Action.Name != "borrow_book" || got[1].Action.Name != "join_event" {
		t.Errorf("subjects are %q then %q; wanted them sorted by action name",
			got[0].Action.Name, got[1].Action.Name)
	}
	j := got[1]
	if j.LimitAction != "create_event" || j.LimitArg != "capacity" {
		t.Errorf("join_event's limit split to %q/%q", j.LimitAction, j.LimitArg)
	}
	if j.Held != "store_event_count" {
		t.Errorf("join_event's held is %q", j.Held)
	}
}

// A PROJECT DECLARING NONE IS REFUSED BY NAME, which is 108's shape: a stage
// over an empty subject list exits having contended nothing and reads exactly
// like one that contended everything and found nothing.
func TestAProjectDeclaringNoContentionIsRefusedByName(t *testing.T) {
	_, err := Subjects(bindings.Bindings{Actions: []bindings.Action{
		{Name: "create_user"}, {Name: "create_event"},
	}})
	if err == nil {
		t.Fatal("a project declaring no contention was accepted, so the stage would report " +
			"finding nothing exactly as a run over a sound system does")
	}
	for _, want := range []string{"no action declares `contended`", "contended = {"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not carry %q, so the author is told what went wrong "+
				"rather than what to write: %v", want, err)
		}
	}
}

// AND A MALFORMED limit IS REFUSED RATHER THAN SPLIT WRONG. `rumor validate`
// catches this before a run as a cell of arm 19; reaching it here means the
// stage was handed a project validate would have refused, so it names the
// action and stops instead of contending against a limit it cannot read.
func TestAMalformedLimitIsRefusedNamingTheAction(t *testing.T) {
	for _, bad := range []string{"capacity", "create_event.", ".capacity", ""} {
		b := bindings.Bindings{Actions: []bindings.Action{
			contends("join_event", bad, "store_event_count"),
		}}
		_, err := Subjects(b)
		if err == nil {
			t.Errorf("contended.limit %q was accepted", bad)
			continue
		}
		// The empty case is the no-subject refusal, since a wholly empty
		// contended table is an action that declares nothing.
		if bad == "" {
			continue
		}
		if !strings.Contains(err.Error(), "join_event") {
			t.Errorf("limit %q: the refusal does not name the action: %v", bad, err)
		}
	}
}

// LIMIT PLUS ONE, which is the smallest fan-out that can exceed a limit: every
// request past it adds a contender whose refusal says nothing new.
func TestTheWidthIsTheLimitPlusOne(t *testing.T) {
	for limit, want := range map[int]int{1: 2, 2: 3, 3: 4} {
		if got := Width(limit); got != want {
			t.Errorf("Width(%d) = %d, wanted %d", limit, got, want)
		}
	}
}
