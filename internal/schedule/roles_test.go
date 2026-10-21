package schedule

import (
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
)

// THE ROLE IS EVERYTHING BEFORE THE TRAILING ORDINAL, not everything before the
// first underscore: Actors() builds `<role>_<n>` from a role that may itself
// contain one, so `super_admin_1` is role `super_admin`. Without the
// multi-underscore cell a split on the FIRST underscore passes every other.
func TestRoleOfStripsTheOrdinalAndNotTheRole(t *testing.T) {
	for _, c := range []struct {
		actor string
		role  string
		ok    bool
	}{
		{"member_1", "member", true},
		{"member_12", "member", true},
		{"super_admin_1", "super_admin", true},
		{"observer", "", false}, // no ordinal at all
		{"member_", "", false},  // an underscore and nothing after it
		{"member_x", "", false}, // a suffix that is not an ordinal
		{"_1", "", false},       // no role before the ordinal
	} {
		role, ok := RoleOf(c.actor)
		if ok != c.ok || role != c.role {
			t.Errorf("RoleOf(%q) = %q,%v; want %q,%v", c.actor, role, ok, c.role, c.ok)
		}
	}
}

func TestHoldsContext(t *testing.T) {
	member := []bindings.Role{{Name: "member", Contexts: []string{"api"}}}
	both := []bindings.Role{{Name: "member", Contexts: []string{"api", "ui"}}}

	// CONTEXT "none" IS HELD BY EVERYBODY, driven against a role list that
	// holds NOTHING: it is not a context, so there is no session for a role to
	// be entitled to, and gating it would make an action nobody needs a
	// session for unreachable.
	nothing := []bindings.Role{{Name: "member"}}
	if held, err := HoldsContext(nothing, "create_user", "member_1", "none"); err != nil || !held {
		t.Errorf(`context "none" against a role holding nothing: %v %v`, held, err)
	}

	if held, err := HoldsContext(member, "create_event", "member_1", "api"); err != nil || !held {
		t.Errorf("a role declaring api must hold api: %v %v", held, err)
	}
	// AND THE OTHER WAY, which is the whole point of the gate: a context the
	// role does not declare is NOT held, and that is an answer rather than an
	// error -- the action is inapplicable, not a could-not-run.
	held, err := HoldsContext(member, "join_event_ui", "member_1", "ui")
	if err != nil {
		t.Fatalf("an unheld context must be an answer, not a refusal: %v", err)
	}
	if held {
		t.Error("a role declaring only api held ui")
	}
	if held, err := HoldsContext(both, "join_event_ui", "member_1", "ui"); err != nil || !held {
		t.Errorf("a role declaring api and ui must hold ui: %v %v", held, err)
	}

	// THE NARROWING, DRIVEN SO IT IS A STATED BEHAVIOR AND NOT AN ACCIDENT: a
	// project declaring NO roles gets no gate here. The static pairing of a
	// context against the roles that hold it needs the bindings alone and
	// belongs in `rumor validate`.
	if held, err := HoldsContext(nil, "join_event_ui", "member_1", "ui"); err != nil || !held {
		t.Errorf("a project declaring no roles must not be gated here: %v %v", held, err)
	}

	// AND A DECLARATION THAT EXISTS AND OMITS THIS ROLE IS REFUSED, for the
	// reason an undeclared weight is: reading it as "holds nothing" would make
	// every context-bearing action inapplicable, and a walk stalling on a
	// missing [[role]] would look exactly like a walk whose world permits
	// nothing.
	if _, err := HoldsContext(member, "create_event", "admin_1", "api"); err == nil {
		t.Error("a role absent from a non-empty declaration was read as holding no context")
	} else if !strings.Contains(err.Error(), "not a role holding no context") {
		t.Errorf("the refusal must say why absent is not empty; got %v", err)
	}

	// AN ACTOR NAME THIS RUN COULD NOT HAVE MINTED cannot be mapped to a role,
	// so the question cannot be answered and is refused rather than guessed.
	if _, err := HoldsContext(member, "create_event", "observer", "api"); err == nil {
		t.Error("an actor with no ordinal was mapped to a role anyway")
	}
}
