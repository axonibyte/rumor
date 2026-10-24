package shrink

import (
	"context"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
)

// THE PRECONDITION FOR FABLE'S PLACEMENT AT coop r5/206 (5), handed over rather
// than described: a reproduction is reachable having compared TWO parts of three.
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
	v, err := Once(context.Background(), &fake{}, cp, bug(),
		func(context.Context) (engine.Finding, error) { return bug(), nil })
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
	full, err := Once(context.Background(), &fake{}, cp, planted,
		func(context.Context) (engine.Finding, error) { return planted, nil })
	if err != nil {
		t.Fatalf("Once: %v", err)
	}
	if !full.Reproduced || !full.AttributionCompared {
		t.Fatalf("the full comparison must reproduce with all three parts: reproduced=%v compared=%v %s",
			full.Reproduced, full.AttributionCompared, full.Why)
	}
}
