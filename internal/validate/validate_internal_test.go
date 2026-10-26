package validate

import "testing"

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
