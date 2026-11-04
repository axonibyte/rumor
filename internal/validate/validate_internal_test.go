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

	// AND THE REFUSAL NAMES THE RULE, NOT THE TABLE'S SHAPE (rook, coop
	// rumor/1364; my sibling at /1365). The bare-clock cells above are exactly
	// where the old wording lied: it said "no [clock] table" while the file
	// held one. A cell over the TEXT is what keeps the clause from drifting
	// back, since behaviour alone cannot tell the two sentences apart.
	for _, tc := range []struct {
		name            string
		clock, deadline map[string]any
		arm             string
	}{
		{"arm 20, a bare clock", bare, keyed, "20"},
		{"arm 20, no clock at all", absent, keyed, "20"},
	} {
		rep := &Report{}
		arm20(rep, bindings.Bindings{Clock: tc.clock, Deadline: tc.deadline})
		if len(rep.Refusals) != 1 {
			t.Fatalf("%s: %d refusal(s)", tc.name, len(rep.Refusals))
		}
		got := rep.Refusals[0].Detail
		if !strings.Contains(got, "no clock control") {
			t.Errorf("%s: %q does not name the rule", tc.name, got)
		}
		if strings.Contains(got, "no [clock] table") {
			t.Errorf("%s: %q names the table's SHAPE, which is false of a bare table", tc.name, got)
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
			continue
		}
		// AND ITS TEXT NAMES THE RULE TOO: the bare-table cell is the one the
		// old wording was false of.
		if tc.want == 1 {
			got := rep.Refusals[0].Detail
			if !strings.Contains(got, "no clock control") || strings.Contains(got, "no [clock] table") {
				t.Errorf("arm 8, %s: %q does not name the rule", tc.name, got)
			}
		}
	}
	// AND `.local` HAS ITS OWN CONTROL (R7.5; fable's ruling at coop r7/117),
	// which is a CELL of this arm and not a new one: `input.clock.local` is
	// the run's instant read in `[clock].zone`, and the engine builds the key
	// only from a declared zone -- so a statute reading it on a project with
	// none is undefined at every seal.
	//
	// THE FOUR DIRECTIONS, and the third is the one that makes this a cell
	// about the ZONE rather than about the clock: a project that declares a
	// clock AND a zone is clean, while the same statute beside a clock with no
	// zone is refused. Without that pair the cell would pass on an arm that
	// refused every `.local` reader.
	const readsLocal = "package rumor\nallow { input.clock.local.date == \"x\" }\n"
	for _, tc := range []struct {
		name  string
		src   string
		clock map[string]any
		zone  string
		want  int
		names string
	}{
		{"`.local` with a clock and a zone", readsLocal, clockd, "America/Chicago", 0, ""},
		{"`.local` with a clock and NO zone", readsLocal, clockd, "", 1, "`[clock].zone`"},
		// AND NO CLOCK AT ALL IS THE CLOCK'S REFUSAL AND NOT THE ZONE'S, which
		// is one refusal rather than two: a project with no table has already
		// been told the thing it must fix, and a second sentence naming the
		// zone would send its reader after a key that cannot help yet.
		{"`.local` with no clock table", readsLocal, absent, "", 1, "no clock control"},
		{"a plain clock reader with a zone declared", reads, clockd, "America/Chicago", 0, ""},
	} {
		if err := os.WriteFile(filepath.Join(dir, "statute.rego"), []byte(tc.src), 0o644); err != nil {
			t.Fatal(err)
		}
		rep := &Report{}
		arm8(rep, dir, bindings.Bindings{Clock: tc.clock, ClockZone: tc.zone})
		if len(rep.Refusals) != tc.want {
			t.Errorf("arm 8, %s: %d refusal(s), want %d: %v",
				tc.name, len(rep.Refusals), tc.want, rep.Refusals)
			continue
		}
		if tc.want == 1 {
			if got := rep.Refusals[0].Detail; !strings.Contains(got, tc.names) {
				t.Errorf("arm 8, %s: %q does not name %s", tc.name, got, tc.names)
			}
			// AND IT IS ARM 8 AND NOT A NEW ARM, which is the ruling itself
			// (fable, coop r7/117: "a cell of arm 8 and not a new arm") and
			// was armed by nothing until ibis measured it at rumor/1779:
			// `rep.refuse("8", ...) -> rep.refuse("25", ...)` SURVIVED TWICE,
			// because this cell asserted a COUNT and a DETAIL substring and
			// never the Arm -- and `refuse` appends whatever string it is
			// handed. So a refusal could cite an arm that does not exist, and
			// the change the ruling DECLINED was one character away with the
			// suite green.
			if got := rep.Refusals[0].Arm; got != "8" {
				t.Errorf("arm 8, %s: the refusal cites arm %q, want 8 -- a cell of arm 8 and "+
					"not a new arm is the ruling, and an arm number nothing holds is how a "+
					"new one arrives unnoticed", tc.name, got)
			}
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

// ARM 22's MESSAGE NAMES THREE THINGS, which fable required at coop r9/17: the
// action, its context and the handle it would have declared, "so the project's
// author is told what to drop or what to wait for".
//
// THE FIXTURE ARM CANNOT SEE THIS. tests/validate asserts WHICH arm a fixture
// trips, by id, so a refusal that named none of the three would pass there --
// the id is not the message. This is the cell for the words.
func TestArm22NamesTheActionItsContextAndTheHandle(t *testing.T) {
	on := platform.Platform{Stages: map[string]platform.Stage{"browser": {Enabled: true}}}
	browserAction := bindings.Action{Name: "publish_doc_ui", Context: "browser", Creates: []string{"published_doc"}}

	// THE CONTROLS FIRST, so the refusal below is about `creates` on a BROWSER
	// action and not about either half alone.
	for _, clean := range []bindings.Bindings{
		{Actions: []bindings.Action{{Name: "publish_doc", Context: "api", Creates: []string{"doc"}}}},
		{Actions: []bindings.Action{{Name: "publish_doc_ui", Context: "browser"}}},
	} {
		var rep Report
		arm22(&rep, clean, on)
		if len(rep.Refusals) != 0 {
			t.Errorf("arm 22 refused a project it should not: %+v", rep.Refusals)
		}
	}

	// AND WITH THE STAGE OFF IT IS VACUOUS, not clean: a skip and a pass are
	// different accounts and arms 4 and 5 make the same distinction.
	var off Report
	arm22(&off, bindings.Bindings{Actions: []bindings.Action{browserAction}}, platform.Platform{})
	if len(off.Refusals) != 0 {
		t.Errorf("arm 22 refused with the browser stage off: %+v", off.Refusals)
	}
	if len(off.Skipped) == 0 {
		t.Error("arm 22 neither ran nor skipped with the stage off, so a run would not account for it")
	}

	var rep Report
	arm22(&rep, bindings.Bindings{Actions: []bindings.Action{browserAction}}, on)
	if len(rep.Refusals) != 1 {
		t.Fatalf("arm 22 over a browser action declaring `creates` gave %d refusal(s), want 1: %+v",
			len(rep.Refusals), rep.Refusals)
	}
	got := rep.Refusals[0].Detail
	for _, want := range []string{"publish_doc_ui", "browser", "published_doc"} {
		if !strings.Contains(got, want) {
			t.Errorf("the refusal does not name %q, so the author is not told what to drop: %q", want, got)
		}
	}
}
