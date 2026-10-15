package ledger

import (
	"encoding/json"
	"strings"
	"testing"
)

// Every guard is driven in BOTH directions. A refusal arm that never sees the
// permitted case cannot tell "it refuses correctly" from "it refuses always".

func claim(actor, ref string) Fact {
	return Fact{Kind: KindClaim, Actor: actor, Type: "user.created", Ref: ref}
}
func obs(actor string, v int) Fact {
	return Fact{Kind: KindObserved, Actor: actor, Query: "user.count", Source: SourceAPI, Value: v}
}

func TestAppendAssignsMonotonePositions(t *testing.T) {
	l := New(false)
	for i, want := range []int{1, 2, 3} {
		got, err := l.Append(claim("alice", "alice.user#1"))
		if err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
		if got != want {
			t.Errorf("append %d returned position %d, want %d", i, got, want)
		}
	}
	if l.Len() != 3 {
		t.Errorf("Len %d, want 3", l.Len())
	}
}

func TestASealedPositionRefusesAWriteAndLaterPositionsStillAppend(t *testing.T) {
	l := New(false)
	for i := 0; i < 3; i++ {
		if _, err := l.Append(claim("alice", "alice.user#1")); err != nil {
			t.Fatal(err)
		}
	}
	if err := l.SealAt(2); err != nil {
		t.Fatalf("seal: %v", err)
	}

	// REFUSED: at the seal, and before it.
	for _, p := range []int{1, 2} {
		f := claim("bob", "bob.user#1")
		f.Position = p
		err := l.AppendAt(f)
		if err == nil {
			t.Errorf("write at position %d (seal %d) was accepted", p, l.Seal())
			continue
		}
		if !strings.Contains(err.Error(), "never a reorder") {
			t.Errorf("position %d refused with the wrong reason: %v", p, err)
		}
	}
	// PERMITTED: after the seal. Without this arm the refusal above could be
	// "it refuses everything".
	f := claim("bob", "bob.user#1")
	f.Position = 4
	if err := l.AppendAt(f); err != nil {
		t.Errorf("write at position 4 past the seal was refused: %v", err)
	}
	// And Append, which assigns, keeps working after a seal.
	if p, err := l.Append(obs("charlie", 2)); err != nil || p != 5 {
		t.Errorf("Append after seal gave (%d, %v), want (5, nil)", p, err)
	}
}

func TestPositionsAreMonotoneEvenWithoutASeal(t *testing.T) {
	l := New(false)
	if _, err := l.Append(claim("alice", "alice.user#1")); err != nil {
		t.Fatal(err)
	}
	f := claim("bob", "bob.user#1")
	f.Position = 1
	if err := l.AppendAt(f); err == nil {
		t.Error("a repeated position was accepted with no seal in play")
	} else if !strings.Contains(err.Error(), "monotone") {
		t.Errorf("refused for the wrong reason: %v", err)
	}
}

func TestSealBounds(t *testing.T) {
	l := New(false)
	if _, err := l.Append(claim("alice", "alice.user#1")); err != nil {
		t.Fatal(err)
	}
	if err := l.SealAt(2); err == nil {
		t.Error("sealing past the highest held position was accepted")
	}
	if err := l.SealAt(1); err != nil {
		t.Errorf("sealing at a held position was refused: %v", err)
	}
	if err := l.SealAt(0); err == nil {
		t.Error("moving the seal back was accepted")
	}
}

func TestSealedPrefixIsWhatACheckReads(t *testing.T) {
	l := New(false)
	for i := 0; i < 3; i++ {
		if _, err := l.Append(claim("alice", "alice.user#1")); err != nil {
			t.Fatal(err)
		}
	}
	if got := l.SealedPrefix(); len(got) != 0 {
		t.Errorf("unsealed ledger yielded %d fact(s), want 0", len(got))
	}
	if err := l.SealAt(2); err != nil {
		t.Fatal(err)
	}
	got := l.SealedPrefix()
	if len(got) != 2 {
		t.Fatalf("sealed prefix holds %d, want 2", len(got))
	}
	for i, f := range got {
		if f.Position != i+1 {
			t.Errorf("prefix[%d] position %d, want %d", i, f.Position, i+1)
		}
	}
}

func TestValidateRefusalsAndTheirPermittedTwins(t *testing.T) {
	ptrI := func(i int64) *int64 { return &i }
	cases := []struct {
		name         string
		clockControl bool
		f            Fact
		wantErr      string // "" means it must be ACCEPTED
	}{
		{"claim ok", false, claim("alice", "alice.user#1"), ""},
		{"claim without type", false, Fact{Kind: KindClaim, Actor: "a", Ref: "a.user#1"}, "no type"},
		{"claim without ref", false, Fact{Kind: KindClaim, Actor: "a", Type: "user.created"}, "no ref"},
		{"claim may be planted", false, func() Fact { f := claim("a", "a.user#1"); f.Planted = true; return f }(), ""},

		{"observed api ok", false, obs("a", 1), ""},
		{"observed ui ok", false, Fact{Kind: KindObserved, Actor: "a", Query: "q", Source: SourceUI, Value: 1}, ""},
		{"observed direct ok", false, Fact{Kind: KindObserved, Actor: "a", Query: "q", Source: SourceDirect, Value: 1}, ""},
		{"observed without source", false, Fact{Kind: KindObserved, Actor: "a", Query: "q", Value: 1}, "no source"},
		{"observed bad source", false, Fact{Kind: KindObserved, Actor: "a", Query: "q", Source: "guess", Value: 1}, "not one of"},
		{"observed without query", false, Fact{Kind: KindObserved, Actor: "a", Source: SourceAPI, Value: 1}, "no query"},
		{"observed cannot be planted", false, Fact{Kind: KindObserved, Actor: "a", Query: "q", Source: SourceAPI, Planted: true}, "tag on a claim"},

		{"told ok", false, Fact{Kind: KindTold, Actor: "alice", To: "charlie", ClaimRef: "alice.user#1"}, ""},
		{"told without recipient", false, Fact{Kind: KindTold, Actor: "alice", ClaimRef: "alice.user#1"}, "no recipient"},
		{"told without claim ref", false, Fact{Kind: KindTold, Actor: "alice", To: "charlie"}, "reference form"},
		{"told to itself", false, Fact{Kind: KindTold, Actor: "alice", To: "alice", ClaimRef: "alice.user#1"}, "to itself"},

		{"clock without control", false, Fact{Kind: KindClock, Actor: "run", T: ptrI(5)}, "no clock control"},
		{"clock with control", true, Fact{Kind: KindClock, Actor: "run", T: ptrI(5)}, ""},
		{"clock with control but no time", true, Fact{Kind: KindClock, Actor: "run"}, "no time"},

		{"no kind", false, Fact{Actor: "a"}, "no kind"},
		{"unknown kind", false, Fact{Kind: "rumour", Actor: "a"}, "and nothing else"},
		{"no actor", false, Fact{Kind: KindClaim, Type: "t", Ref: "r"}, "no actor"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l := New(c.clockControl)
			f := c.f
			if f.Position == 0 {
				f.Position = 1
			}
			err := l.Validate(f)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("accepted case was refused: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("refusal case was accepted")
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("refused for the wrong reason: got %q, want it to mention %q", err, c.wantErr)
			}
		})
	}
}

func TestValidateRefusesAFactWithNoPosition(t *testing.T) {
	l := New(false)
	if err := l.Validate(claim("alice", "alice.user#1")); err == nil {
		t.Error("a fact with position 0 was accepted")
	} else if !strings.Contains(err.Error(), "no position") {
		t.Errorf("wrong reason: %v", err)
	}
}

func build(t *testing.T) *Ledger {
	t.Helper()
	l := New(true)
	for _, f := range []Fact{
		claim("alice", "alice.user#1"),
		claim("bob", "bob.user#1"),
		{Kind: KindTold, Actor: "alice", To: "charlie", ClaimRef: "alice.user#1"},
		obs("charlie", 2),
	} {
		if _, err := l.Append(f); err != nil {
			t.Fatalf("building: %v", err)
		}
	}
	if err := l.SealAt(4); err != nil {
		t.Fatalf("sealing: %v", err)
	}
	return l
}

func dumpOf(t *testing.T, l *Ledger) string {
	t.Helper()
	var sb strings.Builder
	if err := l.Dump(&sb); err != nil {
		t.Fatalf("dump: %v", err)
	}
	return sb.String()
}

func TestDumpIsDeterministic(t *testing.T) {
	l := build(t)
	a, b := dumpOf(t, l), dumpOf(t, l)
	if a != b {
		t.Error("two dumps of one ledger differ")
	}
	// And stable across a round trip, which is the property a committed fixture
	// depends on: re-dumping a loaded ledger must not churn the file.
	l2, err := Load(strings.NewReader(a), true)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := dumpOf(t, l2); got != a {
		t.Errorf("round-trip changed the bytes:\n%s\nvs\n%s", a, got)
	}
}

func TestDumpIsDiffableOneFactPerLine(t *testing.T) {
	l := build(t)
	a := dumpOf(t, l)
	lines := strings.Split(strings.TrimRight(a, "\n"), "\n")
	if len(lines) != l.Len() {
		t.Fatalf("%d line(s) for %d fact(s): the dump is not one fact per line", len(lines), l.Len())
	}

	// Change ONE fact and assert exactly ONE line moves. A format that reflowed
	// would fail this even though it round-tripped.
	l2 := New(true)
	for i, f := range l.Facts() {
		if i == 3 {
			f.Value = 99
		}
		if err := l2.AppendAt(f); err != nil {
			t.Fatalf("rebuilding: %v", err)
		}
	}
	b := strings.Split(strings.TrimRight(dumpOf(t, l2), "\n"), "\n")
	if len(b) != len(lines) {
		t.Fatalf("line counts differ: %d vs %d", len(lines), len(b))
	}
	differing := 0
	for i := range lines {
		if lines[i] != b[i] {
			differing++
		}
	}
	if differing != 1 {
		t.Errorf("changing one fact moved %d line(s), want exactly 1", differing)
	}
}

func TestRoundTripPreservesEveryFact(t *testing.T) {
	l := build(t)
	l2, err := Load(strings.NewReader(dumpOf(t, l)), true)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	before, after := l.Facts(), l2.Facts()
	if len(before) != len(after) {
		t.Fatalf("loaded %d fact(s), dumped %d", len(after), len(before))
	}
	for i := range before {
		if dumpOne(t, before[i]) != dumpOne(t, after[i]) {
			t.Errorf("fact %d changed:\n  %s\n  %s", i, dumpOne(t, before[i]), dumpOne(t, after[i]))
		}
	}
}

// dumpOne marshals ONE fact directly. It used to build a one-fact ledger, which
// stopped working when tells gained referential integrity -- a lone tell cannot
// enter a ledger, and that is correct. The comparison never needed a ledger.
func dumpOne(t *testing.T, f Fact) string {
	t.Helper()
	b, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("dumpOne: %v", err)
	}
	return string(b)
}

// LOAD MUST NOT TRUST THE FILE. Each of these is a dump a writer could never
// have produced, and Load goes through the same guards Append does.
func TestLoadRefusesWhatAWriterWouldHaveRefused(t *testing.T) {
	for _, c := range []struct{ name, body, want string }{
		{"non-monotone positions",
			`{"kind":"claim","position":2,"actor":"a","type":"t","ref":"r"}` + "\n" +
				`{"kind":"claim","position":1,"actor":"b","type":"t","ref":"r"}` + "\n", "monotone"},
		{"an unknown field",
			`{"kind":"claim","position":1,"actor":"a","type":"t","ref":"r","mood":"smug"}` + "\n", "unknown field"},
		{"a bad source",
			`{"kind":"observed","position":1,"actor":"a","query":"q","source":"guess"}` + "\n", "not one of"},
		{"an unknown kind",
			`{"kind":"rumour","position":1,"actor":"a"}` + "\n", "and nothing else"},
		{"no position",
			`{"kind":"claim","position":0,"actor":"a","type":"t","ref":"r"}` + "\n", "no position"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, err := Load(strings.NewReader(c.body), true); err == nil {
				t.Fatal("accepted")
			} else if !strings.Contains(err.Error(), c.want) {
				t.Errorf("refused for the wrong reason: got %q, want it to mention %q", err, c.want)
			}
		})
	}
	// The permitted twin, so the five refusals above are not "Load refuses
	// everything".
	ok := `{"kind":"claim","position":1,"actor":"a","type":"t","ref":"r"}` + "\n"
	if _, err := Load(strings.NewReader(ok), true); err != nil {
		t.Errorf("a valid dump was refused: %v", err)
	}
}

func TestLoadRefusesAClockFactWithoutClockControl(t *testing.T) {
	body := `{"kind":"clock","position":1,"actor":"run","t":5}` + "\n"
	if _, err := Load(strings.NewReader(body), false); err == nil {
		t.Error("a clock fact loaded with no clock control declared")
	}
	if _, err := Load(strings.NewReader(body), true); err != nil {
		t.Errorf("the same fact was refused WITH clock control: %v", err)
	}
}

// A TELL CANNOT PASS A CLAIM THAT DOES NOT EXIST YET, and the permitted twin is
// the same tell once its claim is in the ledger.
func TestATellMustReferenceAClaimAlreadyHeld(t *testing.T) {
	l := New(false)
	tell := Fact{Kind: KindTold, Actor: "alice", To: "charlie", ClaimRef: "alice.user#1"}
	if _, err := l.Append(tell); err == nil {
		t.Error("a tell referencing no held claim was accepted")
	} else if !strings.Contains(err.Error(), "has not been made") {
		t.Errorf("refused for the wrong reason: %v", err)
	}
	if _, err := l.Append(claim("alice", "alice.user#1")); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Append(tell); err != nil {
		t.Errorf("the same tell was refused after its claim landed: %v", err)
	}
	// And a tell of a DIFFERENT claim still refuses, so the check reads the ref
	// rather than merely counting claims.
	other := Fact{Kind: KindTold, Actor: "bob", To: "charlie", ClaimRef: "bob.user#1"}
	if _, err := l.Append(other); err == nil {
		t.Error("a tell of an unheld ref was accepted once any claim existed")
	}
}
