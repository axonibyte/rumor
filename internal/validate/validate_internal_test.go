package validate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	for _, ok := range []string{"12 profile=settling\n", "12\n", "12  # a comment\n"} {
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
