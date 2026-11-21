package explain

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
)

// renderOf renders one example case and returns the text.
func renderOf(t *testing.T, pkg, name string) (string, loaded, *Derivation) {
	t.Helper()
	lc := load(t, pkg, name)
	d := lc.build(t)
	var b bytes.Buffer
	if err := Render(&b, d); err != nil {
		t.Fatalf("%s/%s: Render: %v", pkg, name, err)
	}
	return b.String(), lc, d
}

// --- d2's named arm: the text names the statute line --------------------

func TestTheTextNamesTheStatuteLine(t *testing.T) {
	checked := 0
	for _, pkg := range packages(t) {
		for _, name := range cases(t, pkg) {
			text, lc, _ := renderOf(t, pkg, name)
			if len(lc.finding.Fired) == 0 {
				t.Fatalf("%s/%s fired no definition, so this arm has nothing to look for", pkg, name)
			}
			for _, def := range lc.finding.Fired {
				want := fmt.Sprintf("%s:%d", def.Module, def.Row)
				if !strings.Contains(text, want) {
					t.Errorf("%s/%s: the text does not name %s:\n%s", pkg, name, want, text)
				}
				checked++
			}

			// AND IT SAYS WHAT THE LINE IS, which is the half a file-and-line
			// alone does not give a reader: a default row means no clause
			// matched, and that sentence is why the reader need not open the
			// statute.
			if def := lc.finding.Fired[0]; def.Default {
				if !strings.Contains(text, "the DEFAULT row: no clause matched") {
					t.Errorf("%s/%s fired a default row and the text does not say so:\n%s", pkg, name, text)
				}
			} else if !strings.Contains(text, "a clause matched") {
				t.Errorf("%s/%s fired a clause and the text does not say so:\n%s", pkg, name, text)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no statute line was checked, so this arm passed over nothing")
	}

	// THE CONTROL: the same reader against a derivation whose rule lines were
	// dropped must NOT find a line. Without it, a `Contains` arm would pass on
	// a renderer that printed every row in the file.
	_, _, d := renderOf(t, "toy", "stale-count")
	row := d.Verdict.Rules[0].Row
	d.Verdict.Rules = nil
	var b bytes.Buffer
	if err := Render(&b, d); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), fmt.Sprintf("statute.rego:%d", row)) {
		t.Errorf("the text names a statute line the derivation does not carry, so the arm above proves nothing:\n%s", b.String())
	}
}

// --- the done-when: enough to decide server or rule ----------------------

// TestEveryToyBugsTextCarriesWhatTheDecisionNeeds is the mechanical half of
// d2's done-when. The reader's half is rook's cold read, which is the arm the
// issue names and which no assertion here can stand in for; this one holds the
// text to the five things that decision cannot be made without.
func TestEveryToyBugsTextCarriesWhatTheDecisionNeeds(t *testing.T) {
	bugs := 0
	for _, name := range cases(t, "toy") {
		text, lc, d := renderOf(t, "toy", name)
		if d.Verdict.Value != "bug" {
			continue
		}
		bugs++

		// 1. the finding, in the vocabulary's own word
		if !strings.Contains(text, "FINDING    bug") {
			t.Errorf("toy/%s: the text does not state the finding:\n%s", name, text)
		}
		// 2. the statute line, asserted by the arm above and re-read here
		// because this arm is about the DECISION and not about the line
		if !strings.Contains(text, fmt.Sprintf("statute.rego:%d", lc.finding.Fired[0].Row)) {
			t.Errorf("toy/%s: no statute line in the text", name)
		}
		// 3. what came back, which is the other half of the contradiction
		if !strings.Contains(text, "OBSERVED   "+num(d.Observation.Value)) {
			t.Errorf("toy/%s: the observed value %v is not in the text:\n%s", name, d.Observation.Value, text)
		}
		// 4. the actor, named rather than implied
		if !strings.Contains(text, d.Selector.Actor) {
			t.Errorf("toy/%s: the text does not name %s", name, d.Selector.Actor)
		}
		// 5. every fact the actor knew, with how it was known
		for _, kf := range d.Expectation.Known {
			if !strings.Contains(text, kf.Ref) {
				t.Errorf("toy/%s: %s is in the chain and not in the text", name, kf.Ref)
			}
		}
		for _, kf := range d.Expectation.Known {
			for _, o := range kf.Via {
				if o.Kind == "tell" && !strings.Contains(text, fmt.Sprintf("told to %s by %s at %d", o.To, o.Actor, o.Position)) {
					t.Errorf("toy/%s: the tell of %s is not rendered", name, kf.Ref)
				}
				if o.Kind == "claim" && !strings.Contains(text, fmt.Sprintf("%s's own claim at %d", o.Actor, o.Position)) {
					t.Errorf("toy/%s: the claim of %s is not rendered", name, kf.Ref)
				}
			}
		}
	}
	if bugs != 10 {
		t.Fatalf("%d bug(s) rendered, and examples/toy carried ten when this arm was written: the set has moved and the arm's claim moved with it", bugs)
	}
}

// --- shortest path first -------------------------------------------------

func TestTheChainIsShortestPathFirst(t *testing.T) {
	// The metric, driven on a hand-built set so every case it must order is
	// present at once: two own claims, two tells, and one fact known both
	// ways, which is as short as its shortest origin.
	in := []KnownFact{
		{Ref: "tell-late", Via: []Origin{{Kind: "tell", Actor: "x", To: "a", Position: 9}}},
		{Ref: "claim-late", Via: []Origin{{Kind: "claim", Actor: "a", Position: 8}}},
		{Ref: "tell-early", Via: []Origin{{Kind: "tell", Actor: "x", To: "a", Position: 2}}},
		{Ref: "claim-early", Via: []Origin{{Kind: "claim", Actor: "a", Position: 1}}},
		{Ref: "both", Via: []Origin{{Kind: "tell", Actor: "x", To: "a", Position: 3}, {Kind: "claim", Actor: "a", Position: 7}}},
	}
	want := []string{"claim-early", "both", "claim-late", "tell-early", "tell-late"}
	var got []string
	for _, kf := range chainOrder(in) {
		got = append(got, kf.Ref)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("order %v, wanted %v: own claims are no hop and come first, then by position", got, want)
	}

	// `both` sorts at depth 0 on its CLAIM's position (7) and not on its
	// tell's (3), which is the half a min-over-positions reader would get
	// wrong: it is a claim, so it is no hop, and among claims 7 follows 1.
	if got[1] != "both" {
		t.Errorf("a fact known both ways sorts at %d, and it is no hop: %v", 1, got)
	}

	// AND THE ORDER REACHES THE TEXT, which the unit above does not prove:
	// stale-count has two own claims and one tell, so the tell must be last.
	text, _, _ := renderOf(t, "toy", "stale-count")
	iClaim := strings.Index(text, "alice.del#1")
	iTell := strings.Index(text, "bob.join#1")
	if iClaim < 0 || iTell < 0 {
		t.Fatalf("the chain is not in the text:\n%s", text)
	}
	if iClaim > iTell {
		t.Errorf("a told fact is rendered before an own claim:\n%s", text)
	}
}

// --- what the actor cannot account for -----------------------------------

func TestTheTextSeparatesWhatHappenedFromWhatTheActorKnows(t *testing.T) {
	// untold-join: dana joined and carol was never told, which IS the gap
	// between the API's 1 and carol's 0.
	text, _, _ := renderOf(t, "toy", "untold-join")
	if !strings.Contains(text, "IN THE PREFIX, NOT KNOWN TO CAROL") {
		t.Errorf("the block that explains the gap is absent:\n%s", text)
	}
	if !strings.Contains(text, "dana.join#1") || !strings.Contains(text, "dana claimed event.joined at 4") {
		t.Errorf("the unknown claim is not named with its type and position:\n%s", text)
	}

	// AND THE OTHER DIRECTION: stale-count's alice knows every prefix claim,
	// so the block must be absent rather than present and empty.
	text, _, d := renderOf(t, "toy", "stale-count")
	if strings.Contains(text, "NOT KNOWN TO") {
		t.Errorf("the block appears for an actor who knows every claim:\n%s", text)
	}
	if len(unknownToActor(d)) != 0 {
		t.Errorf("stale-count reports %d unknown claim(s), so the arm above is not about an empty set", len(unknownToActor(d)))
	}
}

// --- the shape of the output ---------------------------------------------

func TestReproductionIsTheLastBlock(t *testing.T) {
	// fable's condition at r11/18: the Reproduction render is written last, so
	// a re-cut after rook's clear touches one block and nothing above it.
	// Asserted for every case, because "last" is a property of the render and
	// not of the one case somebody checked.
	for _, pkg := range packages(t) {
		for _, name := range cases(t, pkg) {
			text, _, _ := renderOf(t, pkg, name)
			lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
			last := lines[len(lines)-1]
			if !strings.HasPrefix(last, "REPRODUCTION") {
				t.Errorf("%s/%s ends with %q and not the reproduction", pkg, name, last)
			}
		}
	}
}

func TestTheRenderIsByteStable(t *testing.T) {
	// The statute's other answers come out of a map, so an unsorted render
	// would differ between runs in the same process -- which is also d3's
	// contract, and cheaper to hold here than to discover there.
	for _, pkg := range packages(t) {
		for _, name := range cases(t, pkg) {
			lc := load(t, pkg, name)
			d := lc.build(t)
			var first, second bytes.Buffer
			if err := Render(&first, d); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 8; i++ {
				second.Reset()
				if err := Render(&second, d); err != nil {
					t.Fatal(err)
				}
				if first.String() != second.String() {
					t.Fatalf("%s/%s renders differently on pass %d:\n%s\n----\n%s",
						pkg, name, i, first.String(), second.String())
				}
			}
			if len(d.Expectation.Holds) < 2 {
				t.Errorf("%s/%s holds %d rule(s), so this arm cannot see a map's order",
					pkg, name, len(d.Expectation.Holds))
			}
		}
	}
}

// TestTheStatutesOtherAnswersAreInOrder is the DETERMINISTIC half of the arm
// above. Byte-stability kills an unordered render only because Go varies a
// map's iteration order between passes -- a real kill, measured, but a
// probabilistic one, and an arm that relies on a coin coming up differently
// eight times is not the arm to leave guarding a contract d3 inherits.
//
// THE ORDER IS TWO GROUPS: rules the finding's evaluation reached, then the
// rest, each by name. The group is the ranking the render has a basis for --
// a rule nothing touched cannot be the error -- and the name breaks every tie,
// so the order is total.
func TestTheStatutesOtherAnswersAreInOrder(t *testing.T) {
	text, _, d := renderOf(t, "toy", "visibility-leak")
	if len(d.Expectation.Holds) < 2 {
		t.Fatalf("visibility-leak holds %d rule(s): an order needs two", len(d.Expectation.Holds))
	}
	block, ok := blockAfter(text, "THE STATUTE ALSO HELD -- reached rules first; a rule nothing touched cannot be the error")
	if !ok {
		t.Fatalf("the block is absent:\n%s", text)
	}
	if len(block) != len(d.Expectation.Holds) {
		t.Errorf("%d line(s) for %d rule(s)", len(block), len(d.Expectation.Holds))
	}

	var names []string
	seenUnreached := false
	for _, line := range block {
		name := strings.Fields(line)[0]
		names = append(names, name)
		h, ok := d.Expectation.Holds[name]
		if !ok {
			t.Errorf("the block names %q, which is not a rule the statute answered", name)
			continue
		}
		// THE GROUPS DO NOT INTERLEAVE: once an unreached rule has been
		// printed, no reached one may follow it.
		if !h.Reached {
			seenUnreached = true
		} else if seenUnreached {
			t.Errorf("%q is reached and follows an unreached rule: %v", name, names)
		}
		// AND EACH ROW SAYS WHERE ITS ANSWER CAME FROM, which is the whole
		// point of the block: a line, or why there is none.
		if h.Reached {
			for _, rl := range h.Rules {
				if !strings.Contains(line, fmt.Sprintf("%s:%d", rl.Module, rl.Row)) {
					t.Errorf("%q carries %s:%d and the row does not name it: %q", name, rl.Module, rl.Row, line)
				}
			}
		} else if !strings.Contains(line, "(not reached)") {
			t.Errorf("%q was not reached and the row does not say so: %q", name, line)
		}
	}
	if !seenUnreached {
		t.Fatal("every rule was reached, so the grouping arm above saw one group")
	}

	// Within each group, ascending by name.
	var reached, rest []string
	for _, n := range names {
		if d.Expectation.Holds[n].Reached {
			reached = append(reached, n)
		} else {
			rest = append(rest, n)
		}
	}
	for _, group := range [][]string{reached, rest} {
		for i := 1; i < len(group); i++ {
			if group[i-1] >= group[i] {
				t.Errorf("%q follows %q, which is not ascending: %v", group[i], group[i-1], group)
			}
		}
	}

	// THE MEASURED PIN: visibility-leak reaches exactly two rules, and the one
	// that explains it -- carol's role cannot see the subject, so her
	// expectation falls to its default -- is the second line of the block
	// rather than buried among fourteen alphabetically.
	if len(reached) != 2 || reached[0] != "believed" || reached[1] != "role_sees_subject" {
		t.Errorf("the reached group is %v, wanted believed then role_sees_subject", reached)
	}
	if !strings.Contains(block[1], "role_sees_subject") || !strings.Contains(block[1], "statute.rego:102 (default)") {
		t.Errorf("the second row is %q", block[1])
	}
}

// blockAfter returns the indented lines following a heading, which is how the
// render groups: a heading at column 0, its rows indented, ended by a blank
// line.
func blockAfter(text, heading string) ([]string, bool) {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if line != heading {
			continue
		}
		var out []string
		for _, row := range lines[i+1:] {
			if strings.TrimSpace(row) == "" {
				break
			}
			out = append(out, strings.TrimSpace(row))
		}
		return out, true
	}
	return nil, false
}

// --- the cases d1 refuses, and the ones it explains ----------------------

func TestAnUndefinedFindingRendersAsNoLineHeld(t *testing.T) {
	base := load(t, "toy", "stale-count")
	f := base.finding
	f.Value, f.Defined, f.Fired = nil, false, nil
	d, err := Build(f, base.ledger, base.check, base.run())
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := Render(&b, d); err != nil {
		t.Fatal(err)
	}
	text := b.String()
	if !strings.Contains(text, "FINDING    no finding") {
		t.Errorf("an undefined finding does not render as an absence:\n%s", text)
	}
	if !strings.Contains(text, "no statute line held: the statute defined nothing for this check") {
		t.Errorf("the render does not say why there is no line:\n%s", text)
	}
	// The chain survives, because it is what the reader has left to look at.
	if !strings.Contains(text, "alice.event#1") {
		t.Errorf("the chain is gone from an undefined finding's render:\n%s", text)
	}
}

func TestAnOverlayRendersTheStatutesOwnVerdictAndItsBound(t *testing.T) {
	base := load(t, "toy", "stale-count")
	for _, tc := range []struct {
		name  string
		until int64
		says  string
	}{
		{"a clock bound", 30000000000, "until 30000000000"},
		// A QUIESCE BOUND NAMES NO INSTANT IN THE VERDICT LINE: it used to say
		// "until the system settles", which is true of a bound still waiting
		// and FALSE of a probe that gave up -- both carry Until 0. The bound
		// block says which way it went instead.
		{"a quiesce bound", 0, "engine.pending overrode it"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := base.finding
			f.Value = "pending"
			f.StatuteValue = "bug"
			f.Overlay = overlayFor(tc.until)
			d, err := Build(f, base.ledger, base.check, base.run())
			if err != nil {
				t.Fatal(err)
			}
			var b bytes.Buffer
			if err := Render(&b, d); err != nil {
				t.Fatal(err)
			}
			text := b.String()
			for _, want := range []string{"FINDING    pending", "the statute said bug", "engine.pending", tc.says, "held open until the bound"} {
				if !strings.Contains(text, want) {
					t.Errorf("the render omits %q:\n%s", want, text)
				}
			}
		})
	}
}

func TestRenderRefusesNothingToRender(t *testing.T) {
	var b bytes.Buffer
	err := Render(&b, nil)
	if err == nil {
		t.Fatal("Render accepted a nil derivation")
	}
	if !strings.Contains(err.Error(), "no derivation to render") {
		t.Errorf("the refusal does not say what was missing: %v", err)
	}
	if b.Len() != 0 {
		t.Errorf("it wrote %d byte(s) before refusing: %q", b.Len(), b.String())
	}
}

// overlayFor builds the engine overlay the arm above imposes. It is a helper
// rather than a literal in the table so the two cells differ in exactly the
// field under test.
func overlayFor(until int64) *engine.Overlay {
	return &engine.Overlay{Rule: "engine.pending", Reason: "held open until the bound",
		Value: "pending", Until: until}
}

// failingWriter fails on every write, which is the case a renderer that
// discarded its write error would report as a successful explanation.
type failingWriter struct{ err error }

func (f failingWriter) Write([]byte) (int, error) { return 0, f.err }

func TestAFailedWriteIsReportedAndNotSwallowed(t *testing.T) {
	_, _, d := renderOf(t, "toy", "stale-count")
	boom := errors.New("the pipe closed")
	err := Render(failingWriter{err: boom}, d)
	if err == nil {
		t.Fatal("Render reported success over a writer that accepted nothing")
	}
	if !errors.Is(err, boom) {
		t.Errorf("the writer's own error is not in the chain: %v", err)
	}
	if !strings.Contains(err.Error(), "could not be written") {
		t.Errorf("the error does not say what failed: %v", err)
	}
}

// TestALargeNumberIsNotRenderedAsAnExponent pins the formatting independently
// of the render's own helper, so the sweep arm above cannot pass by agreeing
// with whatever num() does.
//
// A JSON NUMBER IS A float64, and %v prints 30000000000 as 3e+10 -- which is
// what toy/invitation-honored-late's expiry rendered as until d4 reached it.
// An instant a reader cannot grep, beside a clock line that prints all
// eleven digits.
func TestALargeNumberIsNotRenderedAsAnExponent(t *testing.T) {
	text, _, d := renderOf(t, "toy", "invitation-honored-late")
	if _, ok := d.Observation.Value.(float64); !ok {
		t.Fatalf("the observed value is %T, so this arm is not about a JSON number", d.Observation.Value)
	}
	if !strings.Contains(text, "OBSERVED   30000000000") {
		t.Errorf("the observation is not rendered in full:\n%s", text)
	}
	if strings.Contains(text, "e+") {
		t.Errorf("an exponent reached the render:\n%s", text)
	}
	// AND THE SAME FOR A RULE'S VALUE, which goes through the same helper and
	// is where an expiry sits in this very case.
	block, ok := blockAfter(text, "THE STATUTE ALSO HELD -- reached rules first; a rule nothing touched cannot be the error")
	if !ok {
		t.Fatal("the held block is absent")
	}
	for _, line := range block {
		if strings.Contains(line, "e+") {
			t.Errorf("a rule's value carries an exponent: %q", line)
		}
	}
}
