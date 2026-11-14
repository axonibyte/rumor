package main

import (
	"bytes"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
	"forge.axonibyte.com/axonibyte/rumor/internal/schedule"
)

// fault is one non-stopping finding as the walk builds it, so these arms drive
// the SAME constructor the walk uses rather than a hand-made Finding that
// could drift from it.
func fault(actor, action, kind, text string, status, at int) schedule.Walked {
	return schedule.Walked{
		Finding: engine.PageFault(actor, action, kind, text, status),
		At:      at,
		Actor:   actor,
	}
}

// TestEitherFieldIsAFindingAndBothExitThree is fable's ruling driven (coop
// r9/316): the stop is a mechanism of the walk, the exit status is the verdict
// on the run.
//
// THE FOURTH CASE IS THE ONE THAT CAN FAIL QUIETLY. A record with neither
// field must exit 0, and an implementation that returned 3 whenever `Walked`
// was non-nil-but-empty would satisfy every other case here.
func TestEitherFieldIsAFindingAndBothExitThree(t *testing.T) {
	stopper := engine.UITimedOut("join_event", "member_1", 1, 2, "count-changed")
	for _, c := range []struct {
		name string
		rec  schedule.Record
		want int
	}{
		{"neither", schedule.Record{}, 0},
		{"an empty Walked is still neither", schedule.Record{Walked: []schedule.Walked{}}, 0},
		{"a stopper alone", schedule.Record{Finding: &stopper, FoundAt: 2}, findingStatus},
		{"non-stopping alone", schedule.Record{
			Walked: []schedule.Walked{fault("member_1", "join_event", "console", "boom", 0, 4)},
		}, findingStatus},
		{"both", schedule.Record{
			Finding: &stopper, FoundAt: 2,
			Walked: []schedule.Walked{fault("member_1", "join_event", "response", "/api/x", 503, 1)},
		}, findingStatus},
	} {
		if got := findingExit(c.rec); got != c.want {
			t.Errorf("%s: exit %d, want %d: cli.md's row for 3 is \"a bug was found\" and for 0 "+
				"is \"the walk completed and no observation found a bug\" -- neither says "+
				"\"stopped\"", c.name, got, c.want)
		}
	}
}

// TestTheNonStoppingLineNamesTheKindTheActorAndTheStep is v9's requirement
// driven: the row says the finding "names the actor, the step it is attributed
// to, and which of the two fired", and a line missing any one of the three is
// a finding a reader cannot act on.
func TestTheNonStoppingLineNamesTheKindTheActorAndTheStep(t *testing.T) {
	rec := schedule.Record{Walked: []schedule.Walked{
		fault("member_1", "join_event", "response", "/api/events/1", 503, 4),
	}}
	var out bytes.Buffer
	if err := writeWalked(&out, rec); err != nil {
		t.Fatalf("writing the line: %v", err)
	}
	line := out.String()
	for _, want := range []string{
		"page_fault",    // the finding
		"kind response", // WHICH of the two fired
		"member_1",      // the actor
		"step 4",        // the step it is attributed to
		"the walk continued",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("the line %q does not name %q", line, want)
		}
	}
	if n := strings.Count(line, "\n"); n != 1 {
		t.Errorf("one finding produced %d line(s), want exactly 1", n)
	}
}

// TestAQuietRunPrintsNoFindingLine is the vacuity guard for the arm above and
// fable's third case (coop r9/319, as corrected): a run with none exits 0 and
// prints no finding line.
//
// EXACTLY ZERO BYTES, not "no FOUND line". A renderer that printed a header,
// or an empty line per run, would pass a contains-check and still put a line
// on the stream a reader has to account for.
func TestAQuietRunPrintsNoFindingLine(t *testing.T) {
	var out bytes.Buffer
	if err := writeWalked(&out, schedule.Record{}); err != nil {
		t.Fatalf("writing nothing: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("a run with no non-stopping finding wrote %d byte(s): %q", out.Len(), out.String())
	}
	if got := findingExit(schedule.Record{}); got != 0 {
		t.Errorf("a run with no finding exits %d, want 0", got)
	}
}

// TestEveryNonStoppingFindingGetsItsOwnLine is fable's "ONE VERDICT LINE PER
// non-stopping finding": two faults are two lines, in the order found, each
// with its own step.
func TestEveryNonStoppingFindingGetsItsOwnLine(t *testing.T) {
	rec := schedule.Record{Walked: []schedule.Walked{
		fault("member_1", "join_event", "console", "first", 0, 2),
		fault("member_2", "leave_event", "response", "/api/y", 500, 7),
	}}
	var out bytes.Buffer
	if err := writeWalked(&out, rec); err != nil {
		t.Fatalf("writing two: %v", err)
	}
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("%d line(s) for two findings, want 2: %q", len(lines), out.String())
	}
	if !strings.Contains(lines[0], "step 2") || !strings.Contains(lines[0], "member_1") {
		t.Errorf("the first line is not the first finding: %q", lines[0])
	}
	if !strings.Contains(lines[1], "step 7") || !strings.Contains(lines[1], "member_2") {
		t.Errorf("the second line is not the second finding: %q", lines[1])
	}
}
