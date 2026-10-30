package shrink

import (
	"context"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
)

// THE PRECONDITION FOR FABLE'S PLACEMENT AT coop r5/206 (5), handed over rather
// than described: a reproduction is reachable having compared THREE parts of four.
//
// WHAT THIS ARM IS FOR. `Verdict.AttributionCompared` is written at replay.go:127
// and read nowhere in production -- rook enumerated the sites at coop r5/204: one
// write, two reads, both in tests. fable placed the fix as "Shrink prints one
// line saying the reproduction was partial and on what; the operator is told, not
// refused". This arm pins the state that makes that line necessary, so the line's
// own arm has something to stand on.
//
// WHAT IT DOES NOT CLAIM, and I measured before claiming it: the COMPARISON is
// not at fault. With no attribution, two bugs from `default finding := "bug"`
// differ in NO field engine.Finding carries -- same Defined, same Value, same
// Rule, empty Rules -- so there is nothing to distinguish and nothing failed to.
// My first reading at coop r5/202 said otherwise and r5/203 corrected it from a
// run. The defect is the unread field, not the equality.
//
// AFTER THE FIX this arm should still pass unchanged: a partial reproduction stays
// reachable and stays a reproduction. What gets added beside it is the assertion
// that Shrink SAYS SO.
func TestAPartialReproductionIsReachable(t *testing.T) {
	// Exactly what a statute declaring no rests_on_planted produces twice.
	v, err := Once(context.Background(), &fake{}, cp, wantBug(),
		settled(func(context.Context) (engine.Finding, error) { return bug(), nil }))
	if err != nil {
		t.Fatalf("Once: %v", err)
	}
	if !v.Reproduced {
		t.Fatalf("the gate did not reproduce, so this says nothing about partiality: %s", v.Why)
	}
	if v.AttributionCompared {
		t.Fatal("AttributionCompared is true though neither finding carries the rule, " +
			"so this tree no longer has the state fable's line is for")
	}
	// AND THE CONTRAST, so "partial" is a property of THIS pair and not of every
	// reproduction: two findings that DO carry the attribution reproduce with the
	// third part actually compared.
	planted := engine.Finding{Defined: true, Value: "bug", Rule: "data.rumor.statute.finding",
		Rules: map[string]any{"rests_on_planted": true}}
	full, err := Once(context.Background(), &fake{}, cp, StatuteVerdict{Finding: planted},
		settled(func(context.Context) (engine.Finding, error) { return planted, nil }))
	if err != nil {
		t.Fatalf("Once: %v", err)
	}
	if !full.Reproduced || !full.AttributionCompared {
		t.Fatalf("the full comparison must reproduce with all four parts: reproduced=%v compared=%v %s",
			full.Reproduced, full.AttributionCompared, full.Why)
	}
}

// AND SHRINK SAYS SO, which is the assertion ratchet's arm above was written to
// stand on and fable's placement at coop r5/206 (5): "Shrink prints one line
// saying the reproduction was partial and on what; the operator is told, not
// refused."
//
// IT DRIVES Shrink AND NOT A HAND-BUILT Result. A Result composed here would
// prove the renderer reads its own field and nothing about whether the gate's
// answer ever reaches it -- which is exactly the defect being fixed, one test
// removed. The field's whole history is that it was written and never carried,
// so the arm has to start where it is written.
//
// THE CONTROL IS THE ATTRIBUTION-CARRYING PAIR, so the line is a property of a
// partial comparison and not something Render says about every result. Without
// it a renderer that always printed the warning would pass.
func TestShrinkSaysWhenTheReproductionWasPartial(t *testing.T) {
	carried := engine.Finding{Defined: true, Value: "bug", Rule: "data.rumor.statute.finding",
		Rules: map[string]any{"rests_on_planted": false}}
	// A KIND THAT IS PRESENT ON BOTH SIDES, because ratchet measured at coop
	// r6/193 that the cell above drives the sentence with `bug()`, which
	// carries NO kind either side: "the kind agreed" was then two absences
	// agreeing, and the line's own attribution half draws the distinction its
	// kind half does not -- "was NOT compared" says the comparison did not
	// happen, while an agreeing kind and a missing one read alike. This cell
	// is the same partial comparison with the kind actually agreeing, so the
	// clause is witnessed as an agreement rather than as a shared silence.
	kinded := engine.Finding{Defined: true, Value: "bug", Kind: "stale",
		Rule: "data.rumor.statute.finding"}

	for _, tc := range []struct {
		name    string
		finding engine.Finding
		partial bool
	}{
		{"neither side carries the attribution", bug(), true},
		{"neither carries it and both carry a kind", kinded, true},
		{"both sides carry it", carried, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := tc.finding
			// THE WORLD IS THE PACKAGE'S OWN, so the reduction behaves as it
			// does everywhere else here -- a trace that still needs
			// create_event, which is what keeps Shrink from refusing a shrink
			// to zero actions. Only the FINDING is substituted: the world
			// decides whether a candidate reproduces, and this decides which
			// bug it reproduces as.
			w := &world{needSteps: 4, needKinds: []string{"create_event"}, all: steps(12)}
			got, err := Shrink(context.Background(), Request{
				Seed: 42, Steps: steps(12),
				Kinds: []string{"create_event", "join_event", "leave_event"},
				Want:  StatuteVerdict{Finding: f}, Provider: &fake{}, Checkpoint: cp,
				Replay: func(c context.Context, n int, without []string) (Candidate, error) {
					was, ran, rerr := w.replay(c, n, without)
					if rerr != nil || was.Value != "bug" {
						return Candidate{Finding: was, Steps: ran}, rerr
					}
					return Candidate{Finding: f, Steps: ran}, nil
				},
			})
			if err != nil {
				t.Fatalf("Shrink: %v", err)
			}
			if !got.Stable {
				t.Fatalf("the gate called a reproducing failure unstable, so this says "+
					"nothing about partiality: %s", got.Why)
			}

			// THE GATE'S ANSWER REACHED THE RESULT. This is the half that was
			// missing: the Verdict carried it and the Result did not.
			if got.AttributionCompared == tc.partial {
				t.Fatalf("Result.AttributionCompared is %v for a pair that %s carry the "+
					"attribution, so the gate's answer did not reach the result",
					got.AttributionCompared, map[bool]string{true: "does not", false: "does"}[tc.partial])
			}

			out := got.Render()
			said := strings.Contains(out, "PARTIAL REPRODUCTION")
			if said != tc.partial {
				t.Errorf("Render says partial = %v, want %v -- the operator is told exactly "+
					"when the comparison ran on three parts of four\n%s", said, tc.partial, out)
			}
			if tc.partial && !strings.Contains(out, engine.AttributionRule) {
				t.Errorf("the line does not name %s, so it says the reproduction was partial "+
					"without saying on what\n%s", engine.AttributionRule, out)
			}
			// AND THE ARITHMETIC IS ASSERTED, NOT ONLY THE HEADING. ratchet
			// swept the tree at coop r6/185 and found this line's sentence
			// stale -- it said "three parts ... established on two" after the
			// kind became the fourth part at R6.1 -- and nothing failed,
			// because the only assertion here was on "PARTIAL REPRODUCTION".
			// A heading an operator cannot act on is not what the line is for:
			// the figures and the three that DID agree are the content, so
			// both are pinned.
			if tc.partial {
				for _, want := range []string{
					"the rule, the value and the kind agreed",
					`"The same finding" is four parts and this one is established on three`,
				} {
					if !strings.Contains(out, want) {
						t.Errorf("the partial line does not say %q, so it heads a fact it "+
							"never states and a sentence left at the old count would "+
							"pass\n%s", want, out)
					}
				}
			}
			// AND THE TRACE IS STILL THERE EITHER WAY: the operator is told, not
			// refused, so nothing is withheld because the comparison was partial.
			if !strings.Contains(out, "trace: seed 42") {
				t.Errorf("the trace is missing from the rendered result, so a partial "+
					"reproduction cost the operator the output it came for\n%s", out)
			}
		})
	}
}
