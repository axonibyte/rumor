// Arms for the run's only source of chance. These are the LIBRARY halves of
// fable's D1 and D2 (breakout-r5/0): the drills drive a whole run, and a drill
// that fails because the generator is not deterministic would be reported
// against the walk. Each arm here states what it would catch.
package schedule

import (
	"sort"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
)

// draws is enough to make a coincidence absurd rather than merely unlikely:
// two different PCG streams agreeing on 1,000 consecutive bounded draws has
// probability about 10^-300 at n=8. A SMALLER number is the thing to question
// if this ever fails -- a 10-draw arm would pass on a generator that had
// collapsed to a short cycle.
const draws = 1000

func sequence(t *testing.T, s *Source, n int) []int {
	t.Helper()
	out := make([]int, n)
	for i := range out {
		v, err := s.IntN(8)
		if err != nil {
			t.Fatalf("draw %d of %d refused: %v", i, n, err)
		}
		out[i] = v
	}
	return out
}

// D1's library half: ONE seed, two Sources, the same sequence. What it would
// catch: a generator seeded from the clock, from a package-level default, or
// from anything the seed does not carry -- which is the defect that makes a
// shrink step unre-runnable.
func TestOneSeedDrawsOneSequence(t *testing.T) {
	a := sequence(t, New(42), draws)
	b := sequence(t, New(42), draws)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("draw %d differs between two Sources of seed 42: %d vs %d", i, a[i], b[i])
		}
	}
	t.Logf("PROOF %d draws identical from two Sources of one seed", draws)
}

// THE OTHER SIDE, and without it the arm above passes on a generator that
// returns a constant. A different seed must produce a different sequence.
func TestADifferentSeedDrawsADifferentSequence(t *testing.T) {
	a := sequence(t, New(42), draws)
	b := sequence(t, New(43), draws)
	same := 0
	for i := range a {
		if a[i] == b[i] {
			same++
		}
	}
	if same == len(a) {
		t.Fatalf("seeds 42 and 43 drew the SAME %d values: the seed is not reaching the generator", len(a))
	}
	// at n=8 over 1000 draws, matching positions cluster near 125; the arm is
	// about "not identical", and the figure is logged so a reader can see the
	// generator is not merely offset by one.
	t.Logf("PROOF %d of %d positions coincide between seeds 42 and 43", same, draws)
}

// REPLAY IS THE WHOLE POINT: a Source re-made from a run's printed seed draws
// that run's sequence. D2 and D11 (a promoted seed, months later) both rest on
// this, and it is the same property as D1 read from the operator's side.
func TestThePrintedSeedReproducesTheRun(t *testing.T) {
	first := New(20261019)
	printed := first.Seed()
	want := sequence(t, first, draws)
	got := sequence(t, New(printed), draws)
	for i := range want {
		if want[i] != got[i] {
			t.Fatalf("draw %d of the replay differs: %d vs %d", i, want[i], got[i])
		}
	}
}

// Seed() is what a run prints FIRST, so it must be the integer handed in and
// not a transformation of it: an operator retyping it must land on this run.
func TestSeedIsTheIntegerGivenAndNotADerivation(t *testing.T) {
	for _, seed := range []uint64{0, 1, 42, 1 << 63, ^uint64(0)} {
		if got := New(seed).Seed(); got != seed {
			t.Errorf("New(%d).Seed() = %d: a printed seed that is not the seed cannot be retyped", seed, got)
		}
	}
}

// AN EMPTY APPLICABLE SET IS A FINDING, NOT A PANIC. The stdlib panics on
// IntN(0); a walk reaching a step where nothing is applicable has something to
// say about the project, and a panic here would read as a defect in this
// package instead.
func TestADrawFromNothingIsRefusedRatherThanFatal(t *testing.T) {
	for _, n := range []int{0, -1, -1000} {
		if _, err := New(1).IntN(n); err == nil {
			t.Errorf("IntN(%d) returned no error: the stdlib would panic and the walk would lose its finding", n)
		}
	}
	if _, err := New(1).IntN(1); err != nil {
		t.Errorf("IntN(1) refused, and a single candidate IS a choice: %v", err)
	}
}

// Mint is the ONE entry of entropy, and New must not share it. Two mints
// differing proves Mint is live; New(seed) being unaffected proves the run's
// determinism does not depend on Mint having been called.
func TestMintIsTheOnlyEntropyAndNewDoesNotUseIt(t *testing.T) {
	a, err := Mint()
	if err != nil {
		t.Fatalf("Mint refused: %v", err)
	}
	b, err := Mint()
	if err != nil {
		t.Fatalf("Mint refused on the second call: %v", err)
	}
	if a == b {
		t.Errorf("two mints gave %d twice: a run with no --seed would reuse one corpus", a)
	}
	before := sequence(t, New(7), 50)
	if _, err := Mint(); err != nil {
		t.Fatalf("Mint refused between runs: %v", err)
	}
	after := sequence(t, New(7), 50)
	for i := range before {
		if before[i] != after[i] {
			t.Fatalf("New(7)'s sequence changed across a Mint at draw %d: the seed is not the whole of the entropy", i)
		}
	}
}

// D3's LIBRARY HALF: a kind weighted zero is never drawn. What it would catch:
// a cumulative walk using `<=` instead of `<`, which gives a zero-weighted
// kind the first slot and makes "never" mean "whenever the draw lands on 0".
func TestAZeroWeightIsNeverDrawn(t *testing.T) {
	// THE ZERO KIND MUST SORT FIRST, and my first version had it sorting LAST
	// ({suborn 0, create_user 3, join_event 1} orders as create_user,
	// join_event, suborn) so the arm passed with `<=` in place -- rook found it
	// by mutation at coop r5/6-7. The cumulative walk compares n against the
	// first candidate's weight before any subtraction, so FIRST POSITION is the
	// only slot a `<=` boundary can wrongly select, and an arm whose zero sits
	// anywhere else cannot see it.
	s := New(11)
	cands := []Candidate{{"aaa_zero", 0}, {"create_user", 3}, {"join_event", 1}}

	// THE ARM ASSERTS THAT IT IS ARMED, which is rook's repair at coop r5/11
	// after they broke the fix above by RENAMING the fixture: aaa_zero ->
	// zzz_zero with `<=` still in place passed silently, the defect present and
	// the arm quiet. The arm's power rests on sort order, so the order is now
	// checked rather than assumed -- a rename disarms the arm loudly instead of
	// turning it into a tautology.
	order := make([]string, len(cands))
	for i, c := range cands {
		order[i] = c.Kind
	}
	sort.Strings(order)
	var zero string
	for _, c := range cands {
		if c.Weight == 0 {
			zero = c.Kind
		}
	}
	if order[0] != zero {
		t.Fatalf("this arm needs %q to sort FIRST to reach the boundary; sorted order is %v, "+
			"so the arm cannot see a `<=` defect and would pass with one present", zero, order)
	}
	for i := 0; i < draws; i++ {
		got, err := s.Pick(cands)
		if err != nil {
			t.Fatalf("draw %d refused: %v", i, err)
		}
		if got == "aaa_zero" {
			t.Fatalf("draw %d chose a kind weighted 0: a project that writes 0 has said never", i)
		}
	}
	t.Logf("PROOF %d draws, none of them the zero-weighted kind", draws)
}

// THE CALLER'S ORDER MUST NOT CHANGE THE RUN, which is fable's D1 reasoning
// made testable: a draw that trusted slice order (or walked a map) would pass a
// set comparison and fail D1's sequence comparison, because the applicable set
// is assembled fresh at every step.
func TestTheCandidateOrderDoesNotChangeTheDraw(t *testing.T) {
	forward := []Candidate{{"a", 1}, {"b", 2}, {"c", 3}}
	reversed := []Candidate{{"c", 3}, {"b", 2}, {"a", 1}}
	one, two := New(99), New(99)
	for i := 0; i < draws; i++ {
		x, err := one.Pick(forward)
		if err != nil {
			t.Fatalf("draw %d refused: %v", i, err)
		}
		y, err := two.Pick(reversed)
		if err != nil {
			t.Fatalf("draw %d refused on the reversed set: %v", i, err)
		}
		if x != y {
			t.Fatalf("draw %d: %q from the forward set, %q from the reversed one -- "+
				"the caller's assembly order is reaching the run", i, x, y)
		}
	}
	t.Logf("PROOF %d draws identical across two orderings of one set", draws)
}

// THE WEIGHTS MUST ACTUALLY BIAS, or the arm above passes on a draw that
// ignores weight entirely. The bound is wide on purpose: this asserts the
// SHAPE (3:1 lands nearer 3:1 than 1:1), not a distribution.
func TestTheWeightsBiasTheDraw(t *testing.T) {
	s := New(2026)
	cands := []Candidate{{"heavy", 3}, {"light", 1}}
	heavy := 0
	for i := 0; i < draws; i++ {
		got, err := s.Pick(cands)
		if err != nil {
			t.Fatalf("draw %d refused: %v", i, err)
		}
		if got == "heavy" {
			heavy++
		}
	}
	// 3:1 over 1,000 draws expects 750; anything under 650 or over 850 means
	// the weights are not reaching the draw in the shape the project wrote.
	if heavy < 650 || heavy > 850 {
		t.Errorf("a 3:1 weighting drew the heavy kind %d of %d times, outside [650,850]", heavy, draws)
	}
	t.Logf("PROOF a 3:1 weighting drew heavy %d of %d (expected about 750)", heavy, draws)
}

// A NEGATIVE WEIGHT REFUSES AND A LONE CANDIDATE IS ALWAYS DRAWN -- the two
// clauses of the old all-zero arm that the R5.4 ruling left standing.
//
// THIS WAS TestAllZeroWeightsRefuseRatherThanFallBack and it asserted the
// OPPOSITE of what Pick now does, so it is narrowed rather than deleted: what
// changed is a RULING, not a bug. fable ruled at coop r5/121 and r5/124 that an
// all-zero pool is an EMPTY pool and a no-op step, because D3's "never chosen"
// applies candidate by candidate and a step with nothing choosable is the step
// the walk already knew how to record. The old sentence -- "a fallback would run
// a walk the project asked not to have" -- is still true of a UNIFORM fallback,
// which is not what replaced it.
//
// AND THE ZERO CLAUSE IS NOT RE-ASSERTED HERE, because it is already covered
// better: TestAZeroWeightIsNeverDrawn above puts the zero-weighted candidate in
// FIRST sort position, which rook established at coop r5/6-7 is the only slot a
// `<=` boundary can wrongly select. My first attempt at this replacement
// re-asserted the clause with the zero in an arbitrary slot, which would have
// been a weaker duplicate of an arm that already carries that history. The
// no-op half has its own arm, TestAnAllZeroPoolIsANoOpAndNotARefusal.
func TestANegativeWeightRefusesAndALoneCandidateIsDrawn(t *testing.T) {
	if _, err := New(1).Pick([]Candidate{{"a", -1}}); err == nil {
		t.Error("a negative weight was drawn from rather than refused: internal/platform rejects " +
			"one at parse, so reaching Pick means a caller built it")
	}
	got, err := New(1).Pick([]Candidate{{"only", 7}})
	if err != nil || got != "only" {
		t.Errorf("a single candidate must always be drawn: got %q, %v", got, err)
	}
}

// AN ALL-ZERO POOL IS AN EMPTY POOL AND NOT A REFUSAL, ruled by fable at coop
// r5/121 and placed as a UNIT DRIVE OF Pick at r5/124 rather than as a toy walk:
// ratchet was right that the toy cannot reach the state, so a project would
// prove nothing here and the three cells need no project at all.
//
// WHAT MADE IT REACHABLE. R5.4's honest `tell` is applicable to any actor
// holding any claim, so a step whose only candidate was `tell` gave a pool that
// was non-empty and entirely zero. A project weighting the kind at 0 then got an
// ABORTED walk where before it got a skipped step -- the old sentence had never
// been wrong in practice because no project could reach the state.
//
// THREE CELLS, which is fable's own list: an all-zero pool answers the no-op, a
// pool with one positive weight draws THAT candidate and not the zero beside it,
// and the refusing mutant fails the first.
func TestAnAllZeroPoolIsANoOpAndNotARefusal(t *testing.T) {
	// CELL 1: every candidate weighted zero.
	src := New(99)
	kind, err := src.Pick([]Candidate{{Kind: "tell", Weight: 0}, {Kind: "canard", Weight: 0}})
	if err != nil {
		t.Fatalf("an all-zero pool refused: %v -- a zero weight is the project saying never, "+
			"candidate by candidate, so a step with nothing choosable is the no-op the walk "+
			"already records", err)
	}
	if kind != "" {
		t.Errorf("an all-zero pool drew %q: nothing was choosable, so the answer is the empty "+
			"kind and the caller records a no-op step", kind)
	}

	// CELL 2: an EMPTY pool answers identically. The walk tests len(pool) == 0
	// before calling, so this was unreachable from the only caller and its old
	// refusal said what the no-op step now says.
	if kind, err := New(99).Pick(nil); err != nil || kind != "" {
		t.Errorf("an empty pool gave (%q, %v); it is the same answer as an all-zero one", kind, err)
	}

	// CELL 3: THE CONTROL. One positive weight beside a zero must draw the
	// positive one -- without this, a Pick that answered "" for every input
	// would pass both cells above.
	for i := 0; i < 8; i++ {
		got, err := New(uint64(i)).Pick([]Candidate{
			{Kind: "tell", Weight: 0},
			{Kind: "create_event", Weight: 3},
		})
		if err != nil {
			t.Fatalf("seed %d: a pool with a positive weight refused: %v", i, err)
		}
		if got != "create_event" {
			t.Errorf("seed %d: drew %q beside a zero-weighted candidate; a zero is never chosen",
				i, got)
		}
	}

	// AND A NEGATIVE WEIGHT IS STILL A DEFECT, which the ruling did not touch:
	// internal/platform refuses one at parse, so reaching Pick means a caller
	// built it.
	if _, err := New(1).Pick([]Candidate{{Kind: "x", Weight: -1}}); err == nil {
		t.Error("a negative weight must still refuse: it cannot come from a parsed project")
	}
	t.Log("PROOF all-zero -> (\"\", nil); empty -> (\"\", nil); zero beside 3 -> create_event " +
		"over 8 seeds; negative -> refusal")
}

// TestOnlyTheFindingsThatStopAWalkStopIt pins the stopping set to exactly two
// values, from inside the package, because the set grew and had no oracle.
//
// WHAT IT CATCHES. The check was inline as `v == findingBug`, and R6.2 added a
// second finding that must also end a walk -- `quiesce_timeout`, the system
// refusing to settle. A value added to the vocabulary with no single place to
// decide this is a value some paths stop on and others walk past, and the
// walk-past direction is the quiet one: the run would keep stepping after it
// had something to report, and a shrink would reduce against a world one step
// too far.
//
// THE CONTINUING VALUES ARE ASSERTED TOO, which is the half that would catch
// the opposite error. Making `pending` stop a walk would end a run on a
// disagreement that is INSIDE its bound -- the thing `pending` exists to hold
// open -- and a test naming only the stoppers would pass.
func TestOnlyTheFindingsThatStopAWalkStopIt(t *testing.T) {
	for _, v := range []any{"bug", engine.FindingQuiesceTimedOut} {
		if !stopsTheWalk(v) {
			t.Errorf("%v does not stop the walk: the run would step past something it found", v)
		}
	}
	for _, v := range []any{"consistent", "explained", "pending"} {
		if stopsTheWalk(v) {
			t.Errorf("%v stops the walk: a walk ending here reports a verdict it did not reach", v)
		}
	}
	// A FINDING THE STATUTE LEFT UNDEFINED IS NOT A STOP, and it is not a
	// string either -- engine.Finding.Value is `any` and nil when the statute
	// yielded nothing, so a type assertion is the only safe read.
	if stopsTheWalk(nil) {
		t.Error("an undefined finding stops the walk: nothing was found, so nothing ends it")
	}
	if stopsTheWalk(3) {
		t.Error("a non-string finding value stops the walk")
	}
}

// THE SENTENCE FOR A PENDING THE RUN ENDS HOLDING, driven. ratchet measured
// that nothing asserted it (coop r6/185): mutating all three occurrences left
// four packages green, so the line added so that silence would not pass was
// itself unwitnessed -- the defect it exists to prevent, one layer out.
//
// AT THE UNIT TIER BECAUSE THE PRODUCT CANNOT REACH IT TODAY, which is fable's
// shape for quill's PendingOpen refusal at coop rumor/1235: the guard is
// correct, it sits on a path the product takes, and the walk that leaves a
// pending open waits on R7's clock or R14's repeated check. stillOpen is a
// function so that this can drive it at all.
func TestTheSentenceForAPendingLeftOpen(t *testing.T) {
	for _, c := range []struct {
		what    string
		open    map[string][]string
		held    map[string]ask
		reAsked map[string]bool
		want    []string
	}{
		{
			what: "the walk recorded where it began",
			open: map[string][]string{"member_1": {"event_tally"}},
			held: map[string]ask{askKey("member_1", "event_tally"): {at: 7}},
			want: []string{"member_1/event_tally held at step 7, never re-checked: " +
				"still open at the end"},
		},
		{
			// THE HALF THAT WOULD OTHERWISE VANISH. A pending nothing
			// remembered is still one the run ended holding, and omitting it
			// would make the register and the report disagree on the count.
			what: "nothing remembered where it began",
			open: map[string][]string{"guest_1": {"event_tally"}},
			held: map[string]ask{},
			want: []string{"guest_1/event_tally still open at the end, and the walk did " +
				"not record where it began"},
		},
		{
			// THE CONTROL, and it carries the weight: a build appending a
			// sentence unconditionally passes both cells above and fails this.
			what: "nothing open",
			open: map[string][]string{},
			held: map[string]ask{askKey("member_1", "event_tally"): {at: 3}},
			want: nil,
		},
		{
			// THE CELL THE TABLE COULD NOT EXPRESS BEFORE, and the one this
			// commit exists for: a key that is BOTH open AND re-checked. The
			// reconciliation already worded it -- "held at step N, re-checked
			// after the last step: pending" -- so this function must word
			// NOTHING, or one pending takes two sentences that contradict
			// each other (rook, coop rumor/1264; ratchet asked for this cell
			// at rumor/1269).
			//
			// IT IS THE ONLY CELL WHERE `open` AND `held` AGREE AND THE
			// ANSWER IS STILL SILENCE, which is why the exclusivity had to
			// become a parameter: from those two maps alone this case is
			// indistinguishable from the first cell above.
			what:    "open and re-checked is the reconciliation's to word, not this",
			open:    map[string][]string{"guest_1": {"event_tally"}},
			held:    map[string]ask{askKey("guest_1", "event_tally"): {at: 24}},
			reAsked: map[string]bool{askKey("guest_1", "event_tally"): true},
			want:    nil,
		},
		{
			// AND THE MIXED CASE, so the guard is a filter and not a switch:
			// one key re-asked beside one that was not, the second still
			// worded.
			what: "one re-asked, one not",
			open: map[string][]string{"member_1": {"event_tally", "attendance"}},
			held: map[string]ask{
				askKey("member_1", "event_tally"): {at: 4},
				askKey("member_1", "attendance"):  {at: 9},
			},
			reAsked: map[string]bool{askKey("member_1", "event_tally"): true},
			want: []string{
				"member_1/attendance held at step 9, never re-checked: still open at the end",
			},
		},
		{
			what: "two queries for one actor are two sentences",
			open: map[string][]string{"member_1": {"event_tally", "attendance"}},
			held: map[string]ask{
				askKey("member_1", "event_tally"): {at: 4},
				askKey("member_1", "attendance"):  {at: 9},
			},
			want: []string{
				"member_1/attendance held at step 9, never re-checked: still open at the end",
				"member_1/event_tally held at step 4, never re-checked: still open at the end",
			},
		},
	} {
		t.Run(c.what, func(t *testing.T) {
			got := stillOpen(c.open, c.held, c.reAsked)
			sort.Strings(got)
			if len(got) != len(c.want) {
				t.Fatalf("got %d sentence(s) %q, want %d %q", len(got), got, len(c.want), c.want)
			}
			for i := range c.want {
				if got[i] != c.want[i] {
					t.Errorf("sentence %d:\n got %q\nwant %q", i, got[i], c.want[i])
				}
			}
		})
	}
}
