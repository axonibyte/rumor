package engine

import (
	"context"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/ledger"
)

// TWO EVENTS IN ONE LEDGER, ONE FINDING EACH -- R5.3's first arm, assigned at
// coop r5/16 and cleared to cut at r5/82.
//
// THE ORACLE IS design.md's OWN FAILURE MODE, not an invented one: "a walk that
// creates two events writes joins for both into one ledger, and a statute that
// cannot tell them apart would count ACROSS BOTH". So the arm needs no subject
// field on the observation -- it reads the counts that a spanning statute and a
// per-entity statute disagree about. Event A has two joins, B has one, and the
// span is three; those three numbers are what separate the two worlds.
//
// THE CONVENTION FOR `handles` IS THE CORPUS'S, MEASURED AND NOT INVENTED. Ten
// claims in examples/*/cases carry more than one handle and the extra one is the
// SUBJECT -- `bob.join#1` carries `["bob.join#1", "alice.event#1"]`, its own
// handle plus the event it joined. Nine carry exactly `[ref]`. So a claim's
// handles are its own name plus what it is about, which is why the field is a
// list and what makes "group claims by entity" possible at all.
//
// WHAT THIS DOES NOT PROVE, stated so it is not read as covered: the WALK does
// not yet follow that convention. internal/schedule's honest() appends only the
// minted handle, so a walked join names no event -- R5.4 computes the consumed
// handles as Step.Used but does not put them on the fact. That is one line in a
// file this child must not touch (fable r5/82: the two children touch different
// files), so it lands after the re-cut. This arm proves the ENGINE half: a
// statute given the corpus's convention can tell two subjects apart.
const twoEventStatute = `package rumor.statute

import rego.v1

# The joins recorded against one event, by handle. This is the rule that
# cannot be written at all without the handles projection.
joins_for(ev) := count([c |
	some c in input.claims
	c.type == "event.joined"
	c.position <= input.observation.position
	ev in c.handles
])

# Every event the prefix knows about.
events contains ev if {
	some c in input.claims
	c.type == "event.created"
	c.position <= input.observation.position
	some ev in c.handles
}

# A count that matches no single event's joins is a count that spans events.
#
# THIS STATUTE IS A FIXTURE AND NOT A MODEL. An "every" over an EMPTY set is
# vacuously true in Rego, so with no events in scope "spans" holds and every
# count reads as a bug. That is deliberate here -- it is what makes the dropped
# projection visible, and the mutant kills both clean cases rather than only the
# spanning one -- but a shipped statute wants an explicit count(events) > 0
# guard. Measured: with the projection removed, 2 and 1 both read "bug".
spans if {
	every ev in events {
		joins_for(ev) != input.observation.value
	}
}

default finding := "clean"

finding := "bug" if spans
`

// twoEventLedger: event A with two joins, event B with one, written the way the
// committed cases write a subject -- the join's own handle plus the event's.
func twoEventLedger(t *testing.T) *ledger.Ledger {
	t.Helper()
	l := ledger.New(false)
	facts := []ledger.Fact{
		{Kind: ledger.KindClaim, Actor: "alice", Type: "event.created",
			Ref: "alice.event#1", Handles: []string{"alice.event#1"}},
		{Kind: ledger.KindClaim, Actor: "bob", Type: "event.created",
			Ref: "bob.event#1", Handles: []string{"bob.event#1"}},
		{Kind: ledger.KindClaim, Actor: "carol", Type: "event.joined",
			Ref: "carol.join#1", Handles: []string{"carol.join#1", "alice.event#1"}},
		{Kind: ledger.KindClaim, Actor: "dave", Type: "event.joined",
			Ref: "dave.join#1", Handles: []string{"dave.join#1", "alice.event#1"}},
		{Kind: ledger.KindClaim, Actor: "erin", Type: "event.joined",
			Ref: "erin.join#1", Handles: []string{"erin.join#1", "bob.event#1"}},
	}
	for _, f := range facts {
		if _, err := l.Append(f); err != nil {
			t.Fatalf("appending %s: %v", f.Ref, err)
		}
	}
	if err := l.SealAt(l.Len()); err != nil {
		t.Fatalf("sealing at %d: %v", l.Len(), err)
	}
	return l
}

func TestTwoEventsInOneLedgerAreToldApart(t *testing.T) {
	ctx := context.Background()
	l := twoEventLedger(t)

	for _, c := range []struct {
		label string
		value int
		want  string
	}{
		// EACH EVENT'S OWN COUNT IS CLEAN, which is the "one finding each"
		// half: two is right for A and one is right for B, and a statute that
		// could only total would call both of them wrong.
		{"event A's own two joins", 2, "clean"},
		{"event B's own one join", 1, "clean"},
		// AND THE SPANNING COUNT IS THE BUG. Three is the total across both
		// events and no single event has three, which is exactly the finding a
		// handle-less projection cannot reach.
		{"the count that spans both events", 3, "bug"},
	} {
		f, err := Evaluate(ctx, twoEventStatute, []string{FindingRule}, l,
			Check{Actor: "alice", Query: "event.join_count", Of: "event.joined",
				Source: ledger.SourceAPI, Value: c.value})
		if err != nil {
			t.Fatalf("%s: %v", c.label, err)
		}
		if !f.Defined {
			t.Errorf("%s: no finding at all", c.label)
			continue
		}
		if f.Value != c.want {
			t.Errorf("%s: finding %v, want %v", c.label, f.Value, c.want)
		}
	}
}

// AND THE PROJECTION CARRIES WHAT THE LEDGER HOLDS, verbatim: the subject
// handle reaches the statute's input, and a claim with no handles omits the key
// rather than sending an empty list -- 43 of 62 claim objects in the committed
// cases have no `handles`, and giving them one would make the engine's input
// differ from the file for no gain.
func TestTheProjectionCarriesHandlesVerbatim(t *testing.T) {
	l := twoEventLedger(t)
	_, claims := prefixFacts(l)

	var join map[string]any
	for _, c := range claims {
		if c["ref"] == "carol.join#1" {
			join = c
		}
	}
	if join == nil {
		t.Fatal("the join claim is not in the projection, so this arm reads nothing")
	}
	got, ok := join["handles"].([]string)
	if !ok {
		t.Fatalf("handles projected as %T, want []string", join["handles"])
	}
	if len(got) != 2 || got[0] != "carol.join#1" || got[1] != "alice.event#1" {
		t.Errorf("handles = %v, want the claim's own handle then its subject", got)
	}

	// THE ABSENT CASE, built rather than borrowed: a claim with no handles must
	// not acquire the key.
	bare := ledger.New(false)
	if _, err := bare.Append(ledger.Fact{Kind: ledger.KindClaim, Actor: "alice",
		Type: "user.created", Ref: "alice.user#1"}); err != nil {
		t.Fatalf("appending: %v", err)
	}
	if err := bare.SealAt(1); err != nil {
		t.Fatalf("sealing: %v", err)
	}
	_, bareClaims := prefixFacts(bare)
	if len(bareClaims) != 1 {
		t.Fatalf("%d claim(s) projected", len(bareClaims))
	}
	if _, present := bareClaims[0]["handles"]; present {
		t.Errorf("a claim with no handles was given the key: %v", bareClaims[0])
	}
}
