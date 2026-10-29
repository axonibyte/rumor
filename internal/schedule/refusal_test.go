package schedule

import (
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

// TestARefusedActionIsReadOffTheDeclaredResponse is R7.4's second half at the
// cell tier: what counts as the system DECLINING, and what does not.
//
// IT IS THE UNIT UNDER fable's RULING (coop rumor/1405 item 2) AND MY OWN
// MEASUREMENT (r7/64). The walk appended a claim on `emits` alone until this
// landed, so a 409 and a 200 produced the same ledger; the cells below are the
// discriminator, and the two ACCEPTED cases are what stop them passing
// vacuously -- an always-refuse reader satisfies every refusal cell and
// nothing else.
func TestARefusedActionIsReadOffTheDeclaredResponse(t *testing.T) {
	declared := bindings.Action{
		Name:     "join_event",
		Emits:    []string{"event.joined"},
		Response: map[string]any{"status": int64(200)},
	}
	undeclared := bindings.Action{Name: "tell_only", Emits: []string{"x"}}

	for _, c := range []struct {
		what    string
		action  bindings.Action
		answer  any
		refused bool
		names   []string
	}{
		{
			// THE NUMERIC BRANCH IS NOT COSMETIC: `status = 200` is an int64
			// from TOML and the answer is a float64 from JSON, so a plain
			// `==` makes every action in every project refused.
			what:    "the declared status, as JSON decodes it",
			action:  declared,
			answer:  map[string]any{"status": float64(200), "body": map[string]any{"id": "e1"}},
			refused: false,
		},
		{
			what:    "a 409 where 200 was declared",
			action:  declared,
			answer:  map[string]any{"status": float64(409), "body": map[string]any{"error": "at capacity"}},
			refused: true,
			names:   []string{"join_event", "status", "200", "409"},
		},
		{
			what:    "an answer with no status at all",
			action:  declared,
			answer:  map[string]any{"body": map[string]any{}},
			refused: true,
			names:   []string{"join_event", "carries no status"},
		},
		{
			what:    "an answer that is not a table",
			action:  declared,
			answer:  "OK",
			refused: true,
			names:   []string{"not a table", "OK"},
		},
		{
			// A PROJECT THAT DECLARES NOTHING CANNOT HAVE ITS REFUSALS SEEN,
			// which is the behavior every project had before R7.4 and is
			// stated rather than hidden: there is nothing to compare against.
			what:    "an action declaring no response, answered anything",
			action:  undeclared,
			answer:  map[string]any{"status": float64(500)},
			refused: false,
		},
	} {
		why, refused := refusal(c.action, wire.ExecuteResponse{Response: c.answer})
		if refused != c.refused {
			t.Errorf("%s: refused=%v, want %v (%q)", c.what, refused, c.refused, why)
			continue
		}
		if !refused && why != "" {
			t.Errorf("%s: accepted but carries a reason %q", c.what, why)
		}
		for _, want := range c.names {
			if !contains(why, want) {
				t.Errorf("%s: the reason does not name %q: %q", c.what, want, why)
			}
		}
	}
}

// TestTheRefusalReasonIsStableAcrossAttempts pins the ORDER of the keys the
// reason is composed from. A map's own order would make two attempts of one
// seed record different words for one disagreement, and the record comparison
// would fail for a reason that is not a difference.
//
// TWO BROKEN KEYS AND NOT ONE, because with a single key every order agrees:
// the cell can only see the hazard where the declaration has more than one.
func TestTheRefusalReasonIsStableAcrossAttempts(t *testing.T) {
	a := bindings.Action{
		Name:     "create_event",
		Emits:    []string{"event.created"},
		Response: map[string]any{"status": int64(201), "created": true},
	}
	answer := map[string]any{"status": float64(500), "created": false}
	first, refused := refusal(a, wire.ExecuteResponse{Response: answer})
	if !refused {
		t.Fatalf("two broken keys read as accepted: %q", first)
	}
	for i := 0; i < 50; i++ {
		again, _ := refusal(a, wire.ExecuteResponse{Response: answer})
		if again != first {
			t.Fatalf("the reason moved between reads: %q then %q -- a map's order reached "+
				"the record, and two attempts of one seed would disagree", first, again)
		}
	}
	// AND IT IS THE FIRST KEY IN SORTED ORDER, asserted rather than left to
	// whatever the implementation happens to pick: `created` sorts before
	// `status`, so a reason naming the status is the map's order leaking.
	if !contains(first, "created") {
		t.Errorf("the reason names %q, want the first key in sorted order (`created`)", first)
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
