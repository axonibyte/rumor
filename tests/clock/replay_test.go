package clock_test

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/decl"
	"forge.axonibyte.com/axonibyte/rumor/internal/ledger"
	"forge.axonibyte.com/axonibyte/rumor/internal/platform"
)

// A REPLAY OF A CLOCKED RUN CARRIES THE CLOCK, which is the vocabulary's own
// sentence -- "Replays restore the clock with everything else" -- and fable's
// D5 (coop breakout-r7/0).
//
// IT IS STRUCTURAL, AND THAT IS WHY IT NEEDS AN ARM RATHER THAN A PARAGRAPH.
// schedule.Replay drives the same Walk with the same Project, so clock control
// travels because it is a field of the thing being handed over; nothing
// converts or re-derives it. A later phase that assembled a Project for the
// replay path out of parts -- the shape cmd/rumor already uses for `run` --
// would drop the field with nothing to notice, and internal/shrink judges a
// candidate by comparing its replay's finding against the original's.
//
// WITH A DURATION BOUND THE LOSS IS LOUD RATHER THAN QUIET, which is why this
// arm uses that fixture: a replay that lost the clock cannot measure the bound
// and refuses by name, so the failure arrives as a could-not-run instead of as
// two findings that disagree for an unstated reason. That is the difference
// between a defect a reader can act on and a shrink reducing against the wrong
// failure.

// factLines renders a ledger for comparison, the clock's TIME included. A
// comparison over kinds and positions alone would pass a run whose clock facts
// all read a different time, which is the half this suite is about.
func factLines(facts []ledger.Fact) []string {
	out := make([]string, 0, len(facts))
	for _, f := range facts {
		t := "-"
		if f.T != nil {
			t = fmt.Sprint(*f.T)
		}
		out = append(out, fmt.Sprintf("%d %s %s %s %v t=%s",
			f.Position, f.Kind, f.Actor, f.Ref, f.Value, t))
	}
	return out
}

func TestAClockedRunIsTheSameRunTwice(t *testing.T) {
	const steps = 6
	dir := clocked(t)
	first := walk(t, dir, 1, steps)
	second := walk(t, dir, 1, steps)

	a, b := factLines(first.Facts), factLines(second.Facts)
	if len(a) != len(b) {
		t.Fatalf("%d fact(s) and then %d from one seed: a clocked run is not repeatable, which "+
			"is the property the unit was chosen to keep\n%v\n%v", len(a), len(b), a, b)
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("fact %d is %q and then %q: two runs of one seed differ, so something in "+
				"the clock is reading what the run does not own", i+1, a[i], b[i])
		}
	}
	if n := len(clockFacts(first.Facts)); n == 0 {
		t.Fatal("no clock fact in either run, so this compared two unclocked runs and says " +
			"nothing about the clock")
	}
}

func TestAReplayOfAClockedRunCarriesTheClock(t *testing.T) {
	// settledSeed AND NOT heldSeed: a run that ends with an unmeasured pending
	// now refuses (fable, coop r7/15), and a replay compared against a refused
	// walk would be comparing two refusals. 66 settles, so both sides have a
	// verdict to compare.
	dir := durationBounded(t, true)
	original := walkProfile(t, dir, settledSeed, heldSteps, heldProfile)

	finding, steps, open, err := replayed(t, dir, settledSeed, heldSteps, heldProfile)
	if err != nil {
		t.Fatalf("the replay refused where the walk ran: %v\n\nTHAT IS THE SHAPE THIS ARM IS "+
			"FOR: with a duration bound, a replay that did not carry the clock cannot measure "+
			"the bound, so it refuses rather than returning a finding that disagrees quietly", err)
	}

	if len(steps) != len(original.Steps) {
		t.Errorf("the replay took %d step(s) and the walk %d", len(steps), len(original.Steps))
	}
	if open != original.PendingOpen {
		t.Errorf("the replay ended with %d pending(s) open and the walk with %d: the figure a "+
			"shrink judges a candidate's settling by did not survive the replay",
			open, original.PendingOpen)
	}
	switch {
	case (finding == nil) != (original.Finding == nil):
		t.Errorf("the replay found %v and the walk found %v: one of them is the finding a shrink "+
			"would compare against", finding, original.Finding)
	case finding != nil && finding.Says() != original.Finding.Says():
		t.Errorf("the replay found %q and the walk found %q", finding.Says(), original.Finding.Says())
	}
}

// THE FIRST SEAL'S `set` IS IN THE LEDGER, which is issue 115's own test line
// for deliverable 3: "a `set` to `[clock].start` at the first seal; the
// ledger's first `clock` fact holds it".
//
// IT IS THE HALF THREE HEADS OF DECLARATION DID NOT REACH. `clock_start`
// parsed, `platform.ClockStartFor` resolved it and `rumor report` printed it
// while every walk still began at the epoch -- found by a seed search that
// should have succeeded and did not, over twelve seeds at forty steps. So the
// claim is not "the key is read" but "the run's FIRST recorded instant is the
// declared one", and this reads it off the ledger.
//
// THE PROFILE AND THE VALUE ARE THE COMMITTED PROJECT'S, so the arm is about
// the project rather than about a fixture of its own.
func TestTheFirstClockFactHoldsTheProfilesDeclaredStart(t *testing.T) {
	const profile = "dst-spring"
	prep, err := platform.Load(filepath.Join(project, "platform.toml"), decl.TOML)
	if err != nil || !prep.OK() {
		t.Fatalf("platform.toml: %v %v", err, prep.Refusals)
	}
	declared := prep.Platform.Profiles[profile].ClockStart
	if declared == 0 {
		t.Fatalf("[profiles.%s] declares no clock_start, so there is no instant for the first "+
			"seal to set and this arm is checking nothing", profile)
	}

	rec := walkProfile(t, clockedAs(t, bindings.ClockAdapter), 1, 4, profile)
	facts := clockFacts(rec.Facts)
	if len(facts) == 0 {
		t.Fatal("the walk recorded no clock fact at all, so the first one cannot hold anything")
	}
	first := facts[0]
	if first.T == nil {
		t.Fatal("the first clock fact carries no time")
	}
	if *first.T != declared {
		t.Errorf("the first clock fact reads %dns and [profiles.%s].clock_start declares %dns: "+
			"the walk began somewhere the project did not ask for, which is what twelve seeds "+
			"of this profile measured as finding nothing at all", *first.T, profile, declared)
	}
	// AND IT IS THE RUN'S FIRST FACT, not merely present: a clock fact
	// appended after a step would mean the first SEAL carried no instant, and
	// a statute judging that seal would read the epoch.
	if first.Position != 1 {
		t.Errorf("the first clock fact sits at position %d: the start belongs to the FIRST "+
			"seal, and a seal that carried no instant judges at the epoch", first.Position)
	}
	if !t.Failed() {
		t.Logf("PROOF the first clock fact at position 1 reads %dns, which is "+
			"[profiles.%s].clock_start", declared, profile)
	}
}

// AND A REPLAY CARRIES THE CLOCK INTO THE DAY A TRANSITION RESHAPES, which is
// issue 115's replay line -- "R7's replay tests extended by one case each" --
// with one case per direction.
//
// WHY IT IS A DIFFERENT CASE from the duration-bounded replay above: a landing
// is an INDEX into a slice derived at walk time, and the instants these walks
// land on exist only because a zone is declared, a start puts the clock in
// range, and the application's own reset is a declared deadline. A replay that
// lost any of those draws a different landing for the same seed and the trace
// diverges with neither side refusing -- the bound's arm catches a lost clock
// loudly, this one catches a lost WORLD.
//
// WHAT IT ASSERTS AND WHAT IT DOES NOT. The walks land on the reset's
// landings, which sit at the LOCAL DAY BOUNDARY of the day a transition
// reshapes -- one tick past 2026-03-08 00:00 CST for spring, either side of
// 2026-11-01 00:00 CDT for fall. They do not land on the transition INSTANT
// itself (02:00 CST), and this says so rather than claiming it: what makes
// those days 23 and 25 hours long is the transition, and that is why the
// application's wall-clock arithmetic is wrong there.
func TestAReplayCarriesTheClockThroughATransition(t *testing.T) {
	ch, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Fatalf("the zone database does not hold America/Chicago: %v", err)
	}
	for _, tc := range []struct {
		profile string
		seed    uint64
		day     string
	}{
		{profile: "dst-spring", seed: 1, day: "2026-03-08"},
		{profile: "dst-fall", seed: 2, day: "2026-11-01"},
	} {
		t.Run(tc.profile, func(t *testing.T) {
			const steps = 24
			dir := clockedAs(t, bindings.ClockAdapter)
			original := walkProfile(t, dir, tc.seed, steps, tc.profile)

			// THE CLOCK MOVED, counted as DISTINCT instants and not as facts.
			// A run seals twice a step, so "two clock facts" is satisfied by a
			// clock that never advanced -- measured: before this harness
			// carried the project's deadline instants, 24 steps recorded 48
			// facts all reading the start, and a cell keyed on the count
			// passed over it.
			want := factLines(clockFacts(original.Facts))
			instants := map[int64]bool{}
			for _, f := range clockFacts(original.Facts) {
				if f.T != nil {
					instants[*f.T] = true
				}
			}
			if len(instants) < 2 {
				t.Fatalf("the walk recorded %d distinct instant(s): the clock did not advance, "+
					"so this case is not about a transition", len(instants))
			}
			// AND IT REACHED THE DAY THE TRANSITION RESHAPES, read in the
			// project's own zone.
			var reached bool
			for at := range instants {
				if time.Unix(0, at).In(ch).Format("2006-01-02") == tc.day {
					reached = true
					break
				}
			}
			if !reached {
				t.Fatalf("no instant of this walk falls on %s, the day the transition "+
					"reshapes: the landings are elsewhere and the case proves nothing about "+
					"the hour the application's arithmetic is wrong in", tc.day)
			}

			finding, replaySteps, open, rerr := replayed(t, dir, tc.seed, steps, tc.profile)
			if rerr != nil {
				t.Fatalf("the replay refused where the walk ran: %v", rerr)
			}
			if len(replaySteps) != len(original.Steps) {
				t.Errorf("the replay took %d step(s) and the walk %d: a replay that lost the "+
					"zone, the start or the declared deadline draws a different landing",
					len(replaySteps), len(original.Steps))
			}
			if open != original.PendingOpen {
				t.Errorf("the replay ended with %d pending(s) open and the walk with %d",
					open, original.PendingOpen)
			}
			switch {
			case (finding == nil) != (original.Finding == nil):
				t.Errorf("the replay found %v and the walk found %v", finding, original.Finding)
			case finding != nil && finding.Says() != original.Finding.Says():
				t.Errorf("the replay found %q and the walk found %q", finding.Says(),
					original.Finding.Says())
			}

			// AND THE INSTANTS AGREE, which a step count cannot see: two runs
			// can take the same number of steps and land on different
			// instants, and the instants are what this case is about. The walk
			// is driven again because `replayed` answers a finding and steps
			// where this needs the ledger.
			got := factLines(clockFacts(walkProfile(t, dir, tc.seed, steps, tc.profile).Facts))
			if len(got) != len(want) {
				t.Fatalf("%d clock fact(s) and then %d from one seed", len(want), len(got))
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("clock fact %d is %q and then %q: one seed landed on two "+
						"different instants, so a landing is drawn from something the run "+
						"does not own", i+1, want[i], got[i])
				}
			}
			if !t.Failed() {
				t.Logf("PROOF %s/%d reached %s, replays its finding, its %d step(s) and all "+
					"%d of its clock facts over %d distinct instant(s)",
					tc.profile, tc.seed, tc.day, len(original.Steps), len(want), len(instants))
			}
		})
	}
}
