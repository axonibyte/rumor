package schedule

import (
	"strings"
	"testing"
)

// CONTEXT "none" ACQUIRES NOTHING AND CACHES NOTHING, which is the contract's
// own rule arriving in the engine: `token` is present exactly when the action's
// context is not "none", and ExecuteRequest.Token is omitempty, so the empty
// string IS the absent field. A run whose context-free actions quietly acquired
// sessions would report a session count nobody asked for.
func TestContextNoneAcquiresNothing(t *testing.T) {
	// THE HOLDER HAS NO ADAPTER, which is the assertion: if "none" asked for a
	// session this would refuse for want of one rather than answer.
	s := NewSessions(nil)
	tok, err := s.Token("member_1", "none")
	if err != nil {
		t.Fatalf("an action outside a session must not need one: %v", err)
	}
	if tok != "" {
		t.Errorf("token %q for context none, want the empty string so omitempty omits the field", tok)
	}
	if s.Acquired() != 0 {
		t.Errorf("acquired %d session(s) for an action taken outside a session", s.Acquired())
	}
}

func TestSessionRefusals(t *testing.T) {
	for _, c := range []struct {
		label   string
		actor   string
		context string
		says    string
	}{
		// AN EMPTY CONTEXT IS NOT "none". A declaration that was never read
		// and a declaration of "outside a session" are different facts, and
		// treating the first as the second would silently drop the token for
		// an action that needs one.
		{"the empty context", "member_1", "", "never read"},
		{"no actor", "", "api", "no actor"},
		{"no adapter to ask", "member_1", "api", "no adapter session"},
	} {
		s := NewSessions(nil)
		tok, err := s.Token(c.actor, c.context)
		if err == nil {
			t.Errorf("%s: accepted, answering %q", c.label, tok)
			continue
		}
		if !strings.Contains(err.Error(), c.says) {
			t.Errorf("%s: refusal must say %q; got %v", c.label, c.says, err)
		}
		if tok != "" {
			t.Errorf("%s: refused AND answered %q", c.label, tok)
		}
	}
}
