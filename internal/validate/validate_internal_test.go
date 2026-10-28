package validate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/platform"
)

// TestAllArmIDsIsTotalArms pins the accounting list to the count, from INSIDE
// the package, because nothing could.
//
// WHY IT IS HERE AND NOT IN tests/validate. allArmIDs is unexported, so the
// external suite cannot read it -- and validate.go's own comment claimed
// "tests/validate asserts this list against TotalArms so the two cannot
// drift", which was not true of any test in the tree. The claim was checked by
// nothing for as long as it stood.
//
// WHAT IT WOULD HAVE CAUGHT, measured at the R6.2 bump: tests/validate keeps a
// SECOND copy of the list, and adding 7c and 7d there while TotalArms moved to
// twenty-one left this copy at nineteen. Everything compiled and every suite
// but one passed; the failure surfaced indirectly, as "accounts for 19 arm(s),
// want 21" on the could-not-run path, because `unreached` walks this list. That
// is the worst path to learn it on -- it is the one whose whole job is to say
// that nothing was measured.
func TestAllArmIDsIsTotalArms(t *testing.T) {
	if len(allArmIDs) != TotalArms {
		t.Fatalf("allArmIDs names %d arm(s) and TotalArms is %d: `unreached` walks this "+
			"list, so a short one under-accounts on exactly the path that reports nothing "+
			"was measured", len(allArmIDs), TotalArms)
	}
	seen := map[string]bool{}
	for _, a := range allArmIDs {
		if seen[a] {
			t.Errorf("allArmIDs names %q twice: `unreached` would skip it once and the "+
				"count would still add up", a)
		}
		seen[a] = true
	}
}

// TestArms8And20ReadTheTablesCONTENTSAndABareOneDeclaresNothing arms fable's
// ruling at coop rumor/1359, which settled a three-way disagreement rook
// opened at /1354 and corrected at /1357: a BARE RESERVED TABLE DECLARES
// NOTHING, so the arms' `len(b.X) == 0` was right and the comments beside them
// were the outliers.
//
// THE RULING ASKED FOR A CELL, NOT A SENTENCE -- "a cell driving the bare
// table to rc 0 so the convention is armed rather than stated" -- because the
// tree had held the convention in three places and only the code obeyed it.
// Both arms are driven here, in both directions, since the same expression
// reads the same way in each.
//
// WHY IT IS A UNIT ARM. tests/validate keeps one committed fixture per arm and
// arm 20's trips it with a KEYED `[deadline]`, which is the refusal. The
// ACCEPTANCES cannot live there -- a fixture that does not trip its arm is not
// that suite's shape -- so the bare table, the bare clock and the empty case
// are cells.
//
// AND THE ACCEPTANCE CELLS ARE WHAT MAKE THE REFUSALS MEAN SOMETHING: an arm
// that refused every input would satisfy the refusal cells alone.
func TestArms8And20ReadTheTablesCONTENTSAndABareOneDeclaresNothing(t *testing.T) {
	var (
		absent = map[string]any(nil)
		bare   = map[string]any{}
		keyed  = map[string]any{"invitation_expires_at": "invitation.expires_at"}
		clockd = map[string]any{"form": "adapter"}
	)
	for _, tc := range []struct {
		name            string
		clock, deadline map[string]any
		want            int
	}{
		{"a keyed deadline and no clock", absent, keyed, 1},
		// THE RULED CELL: a bare table declares nothing, so there is no
		// deadline here to pair with a missing clock and nothing to refuse.
		{"a BARE deadline and no clock", absent, bare, 0},
		// AND THE SAME READING ON THE OTHER SIDE: a bare `[clock]` names no
		// form and no endpoint, so a keyed deadline beside it is the pairing
		// this arm refuses.
		{"a keyed deadline and a BARE clock", bare, keyed, 1},
		{"a keyed deadline and a keyed clock", clockd, keyed, 0},
		{"no deadline at all", absent, absent, 0},
		{"a clock and no deadline", clockd, absent, 0},
	} {
		rep := &Report{}
		arm20(rep, bindings.Bindings{Clock: tc.clock, Deadline: tc.deadline})
		if len(rep.Refusals) != tc.want {
			t.Errorf("arm 20, %s: %d refusal(s), want %d: %v",
				tc.name, len(rep.Refusals), tc.want, rep.Refusals)
		}
	}

	// ARM 8's HALF, same idiom and the same two directions. It reads the rule
	// side off statute.rego in the project DIRECTORY, so each cell writes one:
	// a statute that reaches for the clock, and one that does not.
	const reads = "package rumor\nallow { input.clock > 0 }\n"
	const silent = "package rumor\nallow { true }\n"
	dir := t.TempDir()
	for _, tc := range []struct {
		name  string
		src   string
		clock map[string]any
		want  int
	}{
		{"a rule reading the clock and no table", reads, absent, 1},
		{"a rule reading the clock and a BARE table", reads, bare, 1},
		{"a rule reading the clock and a keyed table", reads, clockd, 0},
		{"no rule reading the clock and no table", silent, absent, 0},
	} {
		if err := os.WriteFile(filepath.Join(dir, "statute.rego"), []byte(tc.src), 0o644); err != nil {
			t.Fatal(err)
		}
		rep := &Report{}
		arm8(rep, dir, bindings.Bindings{Clock: tc.clock})
		if len(rep.Refusals) != tc.want {
			t.Errorf("arm 8, %s: %d refusal(s), want %d: %v",
				tc.name, len(rep.Refusals), tc.want, rep.Refusals)
		}
	}
	// AND THE SKIP IS NOT A PASS: with no statute at all the arm reports that
	// the question was unasked, which must not be confused with "evaluated and
	// clean" -- a cell above would read the same either way.
	rep := &Report{}
	arm8(rep, t.TempDir(), bindings.Bindings{Clock: absent})
	if len(rep.Refusals) != 0 || len(rep.Skipped) != 1 {
		t.Errorf("arm 8 with no statute.rego: %d refusal(s) and %d skip(s), want 0 and 1",
			len(rep.Refusals), len(rep.Skipped))
	}
}

// TestArm10bRefusesASeedNamingAnUndeclaredProfile drives arm 10b's SECOND
// clause (R6.3): a line that is a perfectly readable seed, asking for weights
// the platform declares nowhere.
//
// WHY IT IS A UNIT ARM AND NOT A FIXTURE. tests/validate's invariant is one
// committed fixture per arm, with arm 19's cells the ruled exception (fable,
// r4/113), and 10b already has one for its first clause -- a line that is not a
// seed at all. The two refusals are different sentences, so folding both into
// that fixture would let either stand in for the arm, and widening the
// one-per-arm rule is not mine to do. So the clause is driven here, where the
// arm's own function is reachable and the input can be built.
//
// WHAT IT PINS, AND IT IS THE QUIET OUTCOME: a seed naming an undeclared
// profile would REPLAY UNDER THE DEFAULT MIX while its author believed it ran
// under another, so a promoted seed silently stops being about what it caught.
// That is the same loss as a seed that cannot be read, which is why it belongs
// to this arm rather than to a new one.
func TestArm10bRefusesASeedNamingAnUndeclaredProfile(t *testing.T) {
	dir := t.TempDir()
	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "seeds.lock"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p := platform.Platform{
		SeedsFile: "seeds.lock",
		Profiles:  map[string]platform.Profile{"settling": {Stages: []string{"api"}, Seeds: 1}},
	}

	// THE CONTROL FIRST: the same seed, naming the profile that IS declared,
	// and one naming none at all. Without these the refusal below could be
	// about the token existing rather than about the name it carries.
	for _, ok := range []string{"12 profile=settling\n", "12\n", "12  # a comment\n",
		"12 profile=settling steps=24\n", "12 steps=24\n"} {
		write(ok)
		rep := &Report{}
		arm10b(rep, dir, p)
		if len(rep.Refusals) != 0 {
			t.Fatalf("%q was refused: %v", ok, rep.Refusals)
		}
	}

	for _, tc := range []struct{ body, says string }{
		{"12 profile=nosuchprofile\n", "the platform declares settling"},
		{"12 profile=\n", "names no profile"},
		{"12 bogus=1\n", "is not a token this build knows"},
		{"12 profile=settling profile=settling\n", "a seed runs under one"},
		{"12 justaword\n", "not a `name=value` token"},
		{"12 steps=nine\n", "is not a number"},
		{"12 steps=0\n", "a walk of no steps is not a walk"},
		{"12 steps=4 steps=5\n", "a seed replays at one length"},
	} {
		write(tc.body)
		rep := &Report{}
		arm10b(rep, dir, p)
		if len(rep.Refusals) != 1 {
			t.Fatalf("%q gave %d refusal(s), want 1: %v", tc.body, len(rep.Refusals), rep.Refusals)
		}
		if got := rep.Refusals[0].Detail; !strings.Contains(got, tc.says) {
			t.Errorf("%q refused without saying %q: %s", tc.body, tc.says, got)
		}
	}
}
