package handle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// THE HANDLES IN THE FIXTURES MUST BE VALID HANDLES. The shape is duplicated
// from the vocabulary into this package, so the duplication needs an oracle
// that is neither copy: every handle the example packages actually use.
func TestTheShapeAcceptsEveryHandleTheExamplesUse(t *testing.T) {
	var handles []string
	seen := map[string]bool{}
	err := filepath.Walk("../../examples", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".json") || strings.Contains(path, "rules.json") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var c struct {
			Input struct {
				Known  []string `json:"known"`
				Claims []struct {
					Ref    string `json:"ref"`
					Handle string `json:"handle"`
				} `json:"claims"`
			} `json:"input"`
		}
		if json.Unmarshal(body, &c) != nil {
			return nil // not a case file; the suite below does not depend on it
		}
		for _, k := range c.Input.Known {
			if !seen[k] {
				seen[k] = true
				handles = append(handles, k)
			}
		}
		for _, cl := range c.Input.Claims {
			for _, h := range []string{cl.Ref, cl.Handle} {
				if h != "" && !seen[h] {
					seen[h] = true
					handles = append(handles, h)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the examples: %v", err)
	}
	// THE COUNT IS DECLARED AND ASSERTED BOTH WAYS. Zero refusals over an
	// empty corpus reads exactly like zero refusals over a valid one, so a
	// floor is not enough -- and my first floor was a GUESS of 10, which this
	// assertion caught: the examples carried exactly 8 distinct handles across
	// the three packages. Fewer means the walk stopped seeing files; more
	// means the corpus grew and this oracle should be re-read rather than
	// quietly widened.
	//
	// 14 SINCE examples/toy (R5.2, 2026-10-19), AND IT WAS RE-READ RATHER THAN
	// WIDENED. The toy adds exactly six: alice.event#1, alice.del#1,
	// bob.join#1, bob.left#1, carol.join#1 and dana.join#1. The user handles in
	// the list are not new -- transitive-count already carried them -- and
	// every one of the six is accepted by the shape below, which is the half
	// this oracle exists to assert.
	// 17 SINCE examples/toy/cases/dropped-recount (R6.3, 2026-10-26), RE-READ
	// AND NOT WIDENED. That case adds exactly three: bob.tally#1 and
	// bob.tally#2, which carry the worker's own claim type and are the two asks
	// the row is about, and carol.user#1 -- the one user handle that WAS new,
	// transitive-count having carried alice, bob, charlie and dana but never
	// carol. All three are accepted by the shape below, which is the half this
	// oracle exists to assert.
	// 18 SINCE examples/toy/cases/invitation-expiry (R7.4, 2026-10-29), RE-READ
	// AND NOT WIDENED. That case adds exactly one: alice.invitation#1, the
	// handle of the entity whose EXPIRY the new observation reads -- the first
	// handle in this corpus naming a thing that carries an instant. It is
	// accepted by the shape below, which is the half this oracle exists to
	// assert.
	// 19 SINCE examples/toy/cases/invitation-honored-late (R7.4): one more,
	// `bob.accept#1`, the handle of the join that redeemed an invitation PAST
	// its expiry. It is accepted by the shape below, which is the half this
	// oracle exists to assert.
	const wantHandles = 19
	if len(handles) != wantHandles {
		t.Fatalf("found %d distinct handle(s) in the examples and this test declares %d: %v. A change in the corpus must be seen, not absorbed",
			len(handles), wantHandles, handles)
	}
	for _, h := range handles {
		if !Valid(h) {
			t.Errorf("%q is a handle the examples use and this package's shape refuses it", h)
		}
	}
	if !t.Failed() {
		t.Logf("PROOF the shape accepts all %d distinct handle(s) the example packages use", len(handles))
	}
}

func TestTheShapeRefusesWhatIsNotAHandle(t *testing.T) {
	for _, bad := range []string{
		"", "alice", "alice.user", "alice.user#", "alice.user#0",
		"alice.user#01", "Alice.user#1", "alice..user#1", ".user#1",
		"alice.user#1 ", "alice.user#1#2", "7alice.user#1", "alice.user#x",
	} {
		if Valid(bad) {
			t.Errorf("%q is accepted as a handle and is not one", bad)
		}
	}
	// THE PERMITTED TWINS, so the shape is not merely strict.
	for _, good := range []string{"alice.user#1", "bob.event#12", "a.b#1", "alice.user_group#3", "alice.org.unit#2"} {
		if !Valid(good) {
			t.Errorf("%q is a well-formed handle and the shape refuses it", good)
		}
	}
}

func TestBindResolvesBothWays(t *testing.T) {
	tab, err := New(1)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := tab.Bind("alice.user#1", "srv-7"); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if id, ok := tab.ID("alice.user#1"); !ok || id != "srv-7" {
		t.Errorf("ID = %q, %v", id, ok)
	}
	if h, ok := tab.Handle("srv-7"); !ok || h != "alice.user#1" {
		t.Errorf("Handle = %q, %v", h, ok)
	}
	if _, ok := tab.ID("alice.user#2"); ok {
		t.Error("an unbound handle resolved")
	}
	if got := tab.Handles(); !reflect.DeepEqual(got, []string{"alice.user#1"}) {
		t.Errorf("Handles = %v", got)
	}
}

// THE TWO COLLISIONS, each refused, each with its permitted twin. v1 names
// neither; see this package's doc comment and thread:r1.vocab/7.
func TestTheCollisionsAreRefused(t *testing.T) {
	t.Run("the same handle to a different id", func(t *testing.T) {
		tab, _ := New(1)
		if err := tab.Bind("alice.user#1", "srv-7"); err != nil {
			t.Fatalf("first bind: %v", err)
		}
		err := tab.Bind("alice.user#1", "srv-8")
		if err == nil {
			t.Fatal("rebinding a handle to a different id was accepted")
		}
		if !strings.Contains(err.Error(), "srv-7") || !strings.Contains(err.Error(), "srv-8") {
			t.Errorf("the refusal names neither id: %v", err)
		}
		// AND THE BINDING DID NOT MOVE, which matters more than the error: a
		// refusal that still mutated would be worse than silence.
		if id, _ := tab.ID("alice.user#1"); id != "srv-7" {
			t.Errorf("after the refused rebinding the handle resolves to %q", id)
		}
	})

	t.Run("the same id under a second handle", func(t *testing.T) {
		tab, _ := New(1)
		if err := tab.Bind("alice.user#1", "srv-7"); err != nil {
			t.Fatalf("first bind: %v", err)
		}
		if err := tab.Bind("alice.user#2", "srv-7"); err == nil {
			t.Fatal("two handles were bound to one id")
		}
		if _, ok := tab.ID("alice.user#2"); ok {
			t.Error("the refused handle is bound anyway")
		}
	})

	t.Run("THE PERMITTED TWIN: rebinding to the identical id is idempotent", func(t *testing.T) {
		tab, _ := New(1)
		if err := tab.Bind("alice.user#1", "srv-7"); err != nil {
			t.Fatalf("first bind: %v", err)
		}
		if err := tab.Bind("alice.user#1", "srv-7"); err != nil {
			t.Errorf("the same create reported twice was refused: %v", err)
		}
		if got := tab.Handles(); len(got) != 1 {
			t.Errorf("the idempotent bind changed the table: %v", got)
		}
	})

	t.Run("a malformed handle and an empty id", func(t *testing.T) {
		tab, _ := New(1)
		if err := tab.Bind("not a handle", "srv-7"); err == nil {
			t.Error("a malformed handle was bound")
		}
		if err := tab.Bind("alice.user#1", ""); err == nil {
			t.Error("an empty id was bound")
		}
	})
}

// THE TABLE IS PER ATTEMPT: the handles survive and the ids do not.
func TestTheNextAttemptKeepsHandlesAndDropsIds(t *testing.T) {
	first, err := New(1)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := first.Bind("alice.user#1", "srv-7"); err != nil {
		t.Fatalf("Bind: %v", err)
	}

	second, err := first.NextAttempt()
	if err != nil {
		t.Fatalf("NextAttempt: %v", err)
	}
	if second.Attempt() != 2 {
		t.Errorf("the next attempt is numbered %d", second.Attempt())
	}
	if id, ok := second.ID("alice.user#1"); ok {
		t.Errorf("attempt 2 resolves alice.user#1 to %q, carried from attempt 1: a replay re-creates the entity and the server hands back a NEW id", id)
	}
	// The same handle takes the new id, which is the vocabulary's property.
	if err := second.Bind("alice.user#1", "srv-99"); err != nil {
		t.Fatalf("binding the same handle in attempt 2: %v", err)
	}
	if id, _ := second.ID("alice.user#1"); id != "srv-99" {
		t.Errorf("attempt 2 resolves to %q", id)
	}
	// And attempt 1 is untouched, so the two tables are genuinely separate.
	if id, _ := first.ID("alice.user#1"); id != "srv-7" {
		t.Errorf("attempt 1 now resolves to %q", id)
	}
	if _, err := New(0); err == nil {
		t.Error("attempt 0 was accepted")
	}
}

// REQUIRE IS THE SEAM FOR v1's VALIDATION RULE, and the rule is about the
// between-attempts case: "A claim that names a handle absent from the current
// attempt's table is refused at the ledger as a validation error (R4), never
// judged as a finding." These cases pin the answer a caller gets, because the
// mistake this prevents -- reading an absence as a disagreement and emitting
// `bug` -- would report bookkeeping as a defect in the system under test.
func TestRequireRefusesAnAbsentHandleAsValidationAndNotAFinding(t *testing.T) {
	tab, err := New(1)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := tab.Bind("alice.user#1", "srv-7"); err != nil {
		t.Fatalf("Bind: %v", err)
	}

	// THE PERMITTED TWIN FIRST: a bound handle resolves with no error.
	id, err := tab.Require("alice.user#1")
	if err != nil {
		t.Fatalf("Require on a bound handle: %v", err)
	}
	if id != "srv-7" {
		t.Errorf("Require = %q", id)
	}

	// Absent in this attempt.
	if _, err := tab.Require("alice.user#9"); err == nil {
		t.Error("Require accepted a handle that is not in the table")
	} else {
		for _, want := range []string{"alice.user#9", "attempt 1", "VALIDATION", "never a finding"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("the refusal does not say %q: %v", want, err)
			}
		}
	}

	// Malformed is also a refusal, and a different one.
	if _, err := tab.Require("not a handle"); err == nil {
		t.Error("Require accepted a malformed handle")
	}

	// THE CASE v1 NAMES: the same handle across attempts. In attempt 2 the
	// entity has not been re-created yet, so the handle is absent and a claim
	// naming it is a validation refusal -- not a finding, and not the old id.
	second, err := tab.NextAttempt()
	if err != nil {
		t.Fatalf("NextAttempt: %v", err)
	}
	gotID, err := second.Require("alice.user#1")
	if err == nil {
		t.Errorf("attempt 2 resolved alice.user#1 to %q: the table must keep the CURRENT attempt's id only", gotID)
	}
	if err != nil && !strings.Contains(err.Error(), "attempt 2") {
		t.Errorf("the refusal does not name the attempt it is about: %v", err)
	}
	// And once re-created, it resolves to the new id.
	if err := second.Bind("alice.user#1", "srv-99"); err != nil {
		t.Fatalf("binding in attempt 2: %v", err)
	}
	if id, err := second.Require("alice.user#1"); err != nil || id != "srv-99" {
		t.Errorf("after re-creation Require = %q, %v", id, err)
	}
	if !t.Failed() {
		t.Logf("PROOF an absent handle refuses as validation in both attempts, and resolves to the new id once re-created")
	}
}
