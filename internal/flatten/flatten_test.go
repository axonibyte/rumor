package flatten

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

// THE UNTOLD CLAIM IS THE FIRST TEST, as issue 43 asks, and it is driven from
// the REAL FIXTURE rather than from a hand-made ledger: this package's job is
// to produce the `known` array that examples/transitive-count's cases already
// carry, so the fixture is the oracle and a ledger I invented would only agree
// with me.
//
// The case is the vocabulary's own illustration. Alice and Bob each create a
// user; Alice tells Charlie about hers and nobody tells him about Bob's.
// Charlie must know exactly one claim at his observation's position.
func TestTheUntoldClaimIsNotKnown(t *testing.T) {
	const caseFile = "../../examples/transitive-count/cases/charlie-explained.json"
	body, err := os.ReadFile(caseFile)
	if err != nil {
		t.Fatalf("the fixture that pins this package's output is missing: %v", err)
	}
	var fixture struct {
		Input struct {
			Observation struct {
				Actor    string `json:"actor"`
				Position int    `json:"position"`
			} `json:"observation"`
			Known  []string `json:"known"`
			Claims []struct {
				Ref      string `json:"ref"`
				Actor    string `json:"actor"`
				Position int    `json:"position"`
			} `json:"claims"`
		} `json:"input"`
	}
	if err := json.Unmarshal(body, &fixture); err != nil {
		t.Fatalf("%s: %v", filepath.Base(caseFile), err)
	}
	if len(fixture.Input.Claims) != 2 || len(fixture.Input.Known) != 1 {
		t.Fatalf("the fixture changed shape: %d claim(s) and %d known, wanted 2 and 1. This test reads the fixture as its oracle, so a shape change must be seen rather than absorbed",
			len(fixture.Input.Claims), len(fixture.Input.Known))
	}

	l := Ledger{}
	for _, c := range fixture.Input.Claims {
		l.Claims = append(l.Claims, Claim{Ref: c.Ref, Actor: c.Actor, Position: c.Position})
	}
	// The tell the fixture implies: Alice told Charlie of her claim, before his
	// observation. The fixture carries `known` and not the tell, because the
	// statute is handed the flattened result -- producing it is this package.
	l.Tells = []Told{{Actor: "alice", Recipient: "charlie", ClaimRef: "alice.user#1", Position: 3}}

	got, err := Known(l, fixture.Input.Observation.Actor, fixture.Input.Observation.Position, nil)
	if err != nil {
		t.Fatalf("Known: %v", err)
	}
	want := append([]string(nil), fixture.Input.Known...)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Known = %v, and the fixture says the observer knows %v", got, want)
	}
	for _, c := range fixture.Input.Claims {
		if c.Actor == "bob" {
			for _, k := range got {
				if k == c.Ref {
					t.Errorf("%s is Bob's untold claim and it is in Charlie's knowledge: the gap that makes this case `explained` has been flattened away", c.Ref)
				}
			}
		}
	}
	// GUARDED, not unconditional: a PROOF line printed under a failure
	// asserts what the failure just denied. Driven by the transitivity
	// mutant, which made the line below contradict two errors above it.
	if !t.Failed() {
		t.Logf("PROOF Charlie knows %v at position %d, and Bob's claim is not in it", got, fixture.Input.Observation.Position)
	}
}

// A TELL IS ONE HOP AND NOTHING IS INFERRED FROM IT. Alice tells Bob; Bob
// tells nobody. Charlie knows nothing, however short the chain looks.
func TestKnowledgeDoesNotTravelThroughAnActor(t *testing.T) {
	l := Ledger{
		Claims: []Claim{{Ref: "alice.user#1", Actor: "alice", Position: 1}},
		Tells:  []Told{{Actor: "alice", Recipient: "bob", ClaimRef: "alice.user#1", Position: 2}},
	}
	bob, err := Known(l, "bob", 9, nil)
	if err != nil {
		t.Fatalf("Known(bob): %v", err)
	}
	if !reflect.DeepEqual(bob, []string{"alice.user#1"}) {
		t.Errorf("Bob was told and knows %v", bob)
	}
	charlie, err := Known(l, "charlie", 9, nil)
	if err != nil {
		t.Fatalf("Known(charlie): %v", err)
	}
	if len(charlie) != 0 {
		t.Errorf("Charlie knows %v with no tell to him: knowledge followed a tell", charlie)
	}
	// AND THE SECOND HOP IS ITS OWN FACT: with Bob's tell added, Charlie knows.
	l.Tells = append(l.Tells, Told{Actor: "bob", Recipient: "charlie", ClaimRef: "alice.user#1", Position: 3})
	charlie, err = Known(l, "charlie", 9, nil)
	if err != nil {
		t.Fatalf("Known(charlie) after the second tell: %v", err)
	}
	if !reflect.DeepEqual(charlie, []string{"alice.user#1"}) {
		t.Errorf("Charlie was told by Bob and knows %v", charlie)
	}
}

// A TELL CARRIES ONE REFERENCE, NOT THE TELLER'S KNOWLEDGE. This is the
// property this package exists for, and it was MISSING from this suite until a
// mutation found the hole: with Alice holding two claims and telling Bob about
// one, every test I had passed against a flattening that handed Bob both.
//
// The distinction is invisible in a one-claim ledger, which is what the other
// tells tests use, and it is invisible to a transitive closure over tell
// EDGES, because every tell names a ref and closing over refs changes
// nothing. It only shows when a teller knows MORE than it told.
func TestATellCarriesTheRefAndNotTheTellersKnowledge(t *testing.T) {
	l := Ledger{
		Claims: []Claim{
			{Ref: "alice.user#1", Actor: "alice", Position: 1},
			{Ref: "alice.user#2", Actor: "alice", Position: 2},
		},
		Tells: []Told{{Actor: "alice", Recipient: "bob", ClaimRef: "alice.user#1", Position: 3}},
	}

	alice, err := Known(l, "alice", 9, nil)
	if err != nil {
		t.Fatalf("Known(alice): %v", err)
	}
	if len(alice) != 2 {
		t.Fatalf("Alice made two claims and knows %v: the premise of this test is broken", alice)
	}

	bob, err := Known(l, "bob", 9, nil)
	if err != nil {
		t.Fatalf("Known(bob): %v", err)
	}
	if !reflect.DeepEqual(bob, []string{"alice.user#1"}) {
		t.Errorf("Bob was told ONE of Alice's two claims and knows %v: a tell moved the teller's knowledge instead of the reference it names", bob)
	}
	// Stated as an absence as well, with the thing that leaked named: a
	// missing-signal failure reads worse than "this ref arrived".
	for _, k := range bob {
		if k == "alice.user#2" {
			t.Error("alice.user#2 was never told to Bob and he knows it")
		}
	}
	if !t.Failed() {
		t.Logf("PROOF Alice knows %d, Bob knows %v: the tell carried its reference and nothing else", len(alice), bob)
	}
}

// THE POSITION BOUND, both directions: knowledge is held AT a position, so a
// tell after it does not count and one at it does.
func TestKnowledgeIsHeldAtAPosition(t *testing.T) {
	l := Ledger{
		Claims: []Claim{{Ref: "alice.user#1", Actor: "alice", Position: 1}},
		Tells:  []Told{{Actor: "alice", Recipient: "bob", ClaimRef: "alice.user#1", Position: 5}},
	}
	for _, tc := range []struct {
		at   int
		want int
	}{{4, 0}, {5, 1}, {6, 1}} {
		got, err := Known(l, "bob", tc.at, nil)
		if err != nil {
			t.Fatalf("at %d: %v", tc.at, err)
		}
		if len(got) != tc.want {
			t.Errorf("at position %d Bob knows %v, wanted %d ref(s): the tell is at 5", tc.at, got, tc.want)
		}
	}
	// An actor's OWN claim is known at its own position, not after it.
	own, err := Known(l, "alice", 1, nil)
	if err != nil {
		t.Fatalf("alice at 1: %v", err)
	}
	if !reflect.DeepEqual(own, []string{"alice.user#1"}) {
		t.Errorf("Alice at the position of her own claim knows %v", own)
	}
}

// fakeAncestry EXERCISES THE EXTENSION POINT. Issue 43 asks for it to be
// exercised by a test rather than left as a comment, so here it is with a
// chain and a cycle.
type fakeAncestry struct {
	parents map[string][]string
	calls   int
}

func (f *fakeAncestry) Parents(ref string) []string {
	f.calls++
	return f.parents[ref]
}

func TestTheAncestryExtensionPointIsExercised(t *testing.T) {
	l := Ledger{
		Claims: []Claim{{Ref: "alice.user#1", Actor: "alice", Position: 1}},
		Tells:  []Told{{Actor: "alice", Recipient: "bob", ClaimRef: "alice.user#1", Position: 2}},
	}

	// WITHOUT it, v1's behaviour: one hop, one ref.
	plain, err := Known(l, "bob", 9, nil)
	if err != nil {
		t.Fatalf("one-hop: %v", err)
	}
	if !reflect.DeepEqual(plain, []string{"alice.user#1"}) {
		t.Fatalf("one-hop Bob knows %v", plain)
	}

	// WITH it, a chain: the told claim derives from two others, one of which
	// derives from a third. Knowing the child means knowing the line.
	anc := &fakeAncestry{parents: map[string][]string{
		"alice.user#1":    {"alice.account#1", "alice.session#1"},
		"alice.account#1": {"alice.org#1"},
	}}
	got, err := Known(l, "bob", 9, anc)
	if err != nil {
		t.Fatalf("with ancestry: %v", err)
	}
	want := []string{"alice.account#1", "alice.org#1", "alice.session#1", "alice.user#1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("with ancestry Bob knows %v, wanted %v", got, want)
	}
	if anc.calls == 0 {
		t.Error("the provider was never called, so this test would pass against a Known that ignores its Ancestry argument")
	}
	if !t.Failed() {
		t.Logf("PROOF the extension point ran: %d call(s), knowledge closed from 1 ref to %d", anc.calls, len(got))
	}

	// A CYCLE TERMINATES. A fixture can hand a provider a loop, and a walk
	// that did not track visits would not return -- a hang, not a failure.
	loop := &fakeAncestry{parents: map[string][]string{
		"alice.user#1": {"alice.loop#1"},
		"alice.loop#1": {"alice.user#1"},
	}}
	cyc, err := Known(l, "bob", 9, loop)
	if err != nil {
		t.Fatalf("with a cyclic ancestry: %v", err)
	}
	if !reflect.DeepEqual(cyc, []string{"alice.loop#1", "alice.user#1"}) {
		t.Errorf("a cycle gave %v", cyc)
	}
}

// THE REFUSALS, each with its permitted twin, because a guard that never sees
// the accepted case cannot tell "refuses correctly" from "refuses always".
func TestMalformedFactsAreRefused(t *testing.T) {
	good := Ledger{
		Claims: []Claim{{Ref: "alice.user#1", Actor: "alice", Position: 1}},
		Tells:  []Told{{Actor: "alice", Recipient: "bob", ClaimRef: "alice.user#1", Position: 2}},
	}
	if _, err := Known(good, "bob", 9, nil); err != nil {
		t.Fatalf("the permitted twin must pass, and it did not: %v", err)
	}

	for _, tc := range []struct {
		name string
		l    Ledger
		who  string
		at   int
	}{
		{"no actor asked about", good, "", 9},
		{"position zero", good, "bob", 0},
		{"a claim with no ref", Ledger{Claims: []Claim{{Actor: "alice", Position: 1}}}, "alice", 9},
		{"a claim with no actor", Ledger{Claims: []Claim{{Ref: "x", Position: 1}}}, "alice", 9},
		{"a claim at position zero", Ledger{Claims: []Claim{{Ref: "x", Actor: "alice"}}}, "alice", 9},
		{"a tell with no recipient", Ledger{Tells: []Told{{Actor: "alice", ClaimRef: "x", Position: 1}}}, "bob", 9},
		{"a tell with no claim ref", Ledger{Tells: []Told{{Actor: "alice", Recipient: "bob", Position: 1}}}, "bob", 9},
		{"a tell to its own actor", Ledger{Tells: []Told{{Actor: "alice", Recipient: "alice", ClaimRef: "x", Position: 1}}}, "alice", 9},
	} {
		if _, err := Known(tc.l, tc.who, tc.at, nil); err == nil {
			t.Errorf("%s: accepted, and it must refuse", tc.name)
		}
	}
}

// THE OUTPUT IS SORTED, so a comparison against a fixture's array cannot
// depend on map iteration. Driven with refs whose insertion order is not
// their sorted order.
func TestOutputIsSorted(t *testing.T) {
	l := Ledger{Claims: []Claim{
		{Ref: "alice.user#3", Actor: "alice", Position: 1},
		{Ref: "alice.user#1", Actor: "alice", Position: 2},
		{Ref: "alice.user#2", Actor: "alice", Position: 3},
	}}
	got, err := Known(l, "alice", 9, nil)
	if err != nil {
		t.Fatalf("Known: %v", err)
	}
	want := []string{"alice.user#1", "alice.user#2", "alice.user#3"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Known = %v, wanted %v sorted", got, want)
	}
}
