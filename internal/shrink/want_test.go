package shrink

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
	"forge.axonibyte.com/axonibyte/rumor/internal/schedule"
)

// A VERDICT A STAGE AUTHORED IS A SUBJECT A SHRINK CAN KEEP, which is the whole
// of the widening and is armed with a want that is NOT the statute's.
//
// WHY A SECOND IMPLEMENTATION AND NOT JUST THE STATUTE's: a one-implementation
// interface says nothing. Every reducer took `want engine.Finding`, so a stage
// finding had no `want` to hand the shrinker at all; this arm is what shows the
// type no longer decides who may author one. The concurrency stage's own want
// lands with its dispatch -- a want nothing calls would be the built-and-unread
// shape this package was caught by once at Verdict.AttributionCompared.
//
// THE FAKE JUDGES ON WHAT A STAGE VERDICT WOULD: a property of the candidate
// that is not the statute's finding at all. Here it is the trace still being
// long enough to hold the setup the verdict needs, which is the shape fable's
// ruling describes -- "what shrinks is the SETUP ... the fan-out is one step of
// width N, and N is not reduced".
type needsSetup struct{ atLeast int }

func (n needsSetup) Describe() string { return "a setup of at least some length" }

func (n needsSetup) Recurs(cand Candidate) (bool, string) {
	if len(cand.Steps) >= n.atLeast {
		return true, ""
	}
	return false, "the setup no longer reaches the state the verdict needs"
}

func TestAVerdictAStageAuthoredCanBeTheSubjectOfAShrink(t *testing.T) {
	// THE STATUTE's FINDING IS DELIBERATELY CONSTANT HERE: every candidate
	// answers the same `bug`, so a shrinker still reading engine.Finding would
	// see every candidate reproduce and reduce to nothing. The reduction that
	// comes back is therefore the stage want's doing and not the statute's.
	w := &world{needSteps: 1, all: steps(12)}
	got, err := Shrink(context.Background(), Request{
		Seed: 7, Steps: steps(12), Kinds: []string{"create_event"},
		Want: needsSetup{atLeast: 5}, Provider: &fake{}, Checkpoint: cp,
		Replay: func(c context.Context, n int, without []string) (Candidate, error) {
			was, ran, rerr := w.replay(c, n, without)
			if rerr != nil {
				return Candidate{}, rerr
			}
			_ = was
			return Candidate{Finding: bug(), Steps: ran}, nil
		},
	})
	if err != nil {
		t.Fatalf("Shrink with a stage-authored want: %v", err)
	}
	if !got.Stable {
		t.Fatalf("a stage-authored verdict did not clear the gate: %s", got.Why)
	}
	if got.Reduction.Steps != 5 {
		t.Errorf("the shrink reduced to %d step(s); the want needs 5, so the reduction is not "+
			"following the stage's verdict", got.Reduction.Steps)
	}
}

// AND AN UNSTABLE STAGE VERDICT IS REFUSED THE SAME WAY, with the want's own
// reason carried rather than a sentence the shrinker invented.
func TestAStageVerdictThatDoesNotRecurIsUnstableWithItsOwnReason(t *testing.T) {
	w := &world{needSteps: 1, all: steps(4)}
	got, err := Shrink(context.Background(), Request{
		Seed: 7, Steps: steps(4), Kinds: []string{"create_event"},
		// The gate replays all four steps and the want needs ten, so it
		// cannot recur at full length.
		Want: needsSetup{atLeast: 10}, Provider: &fake{}, Checkpoint: cp,
		Replay: func(c context.Context, n int, without []string) (Candidate, error) {
			_, ran, rerr := w.replay(c, n, without)
			if rerr != nil {
				return Candidate{}, rerr
			}
			return Candidate{Finding: bug(), Steps: ran}, nil
		},
	})
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	if got.Stable {
		t.Fatal("a verdict that cannot recur at full length was called stable")
	}
	if !strings.Contains(got.Why, "no longer reaches the state") {
		t.Errorf("the unstable reason is not the want's own words: %q", got.Why)
	}
}

// A NIL WANT IS REFUSED BY NAME, because a reduction with nothing to reproduce
// would accept every candidate -- the widening's own new way to be wrong.
func TestANilWantIsRefused(t *testing.T) {
	_, err := Once(context.Background(), &fake{}, cp, nil,
		settled(func(context.Context) (engine.Finding, error) { return bug(), nil }))
	if err == nil {
		t.Fatal("Once accepted a nil want, so a shrink with nothing to reproduce would accept " +
			"every candidate it ran")
	}
	if !strings.Contains(err.Error(), "no verdict to keep") {
		t.Errorf("the refusal does not say what is missing: %v", err)
	}
}

// A STAGE VERDICT IS IDENTIFIABLE AS ITS OWN, AND THE REPORT SAYS IT IN THE
// ORACLE'S WORDS.
//
// THIS IS THE OTHER HALF OF THE WIDENING. fable's ruling at coop rumor/1521
// made Want an interface so a stage could author a verdict and left the
// CANDIDATE's verdict typed as the statute's -- so a stage want answered Recurs
// out of a field its own oracle never wrote. rook's condition at /1522 is what
// that costs: "two oracles both saying 'overfill' would compare equal". I
// answered at /1523 that the substance bites on the candidate rather than on
// the type and said I would carry it; this is that, with the three readers that
// would otherwise print the statute's words over a verdict it never authored.
//
// THE FAKE ORACLE IS A SECOND ONE ON PURPOSE. Two stage verdicts that differ
// ONLY in their Rule must not compare equal, which is the whole of rook's
// condition, so the want below is driven against a candidate carrying the other
// oracle's name and must refuse it.
type overfillWant struct {
	rule    string
	subject string
}

func (o overfillWant) Describe() string {
	return fmt.Sprintf("an overfill of %s by %s", o.subject, o.rule)
}

func (o overfillWant) Recurs(cand Candidate) (bool, string) {
	if cand.Stage == nil {
		return false, "the candidate carries no stage verdict, so this oracle's finding is not " +
			"among what it produced"
	}
	if cand.Stage.Rule != o.rule {
		return false, fmt.Sprintf("the verdict came from %s and this one is %s: two oracles "+
			"finding the same WORD are not the same finding", cand.Stage.Rule, o.rule)
	}
	if cand.Stage.Subject != o.subject {
		return false, fmt.Sprintf("the verdict is about %s and this one is about %s",
			cand.Stage.Subject, o.subject)
	}
	if !cand.Stage.Found {
		return false, "the oracle ran and found nothing"
	}
	return true, ""
}

func stageCand(rule, subject string, found bool, ran []schedule.Step) Candidate {
	return Candidate{Steps: ran, Stage: &StageVerdict{
		Rule: rule, Subject: subject, Found: found,
		Says: subject + " held 2 against its own limit of 1"}}
}

func TestAStageVerdictIsKeptAndRenderedAsItsOwn(t *testing.T) {
	const rule, subject = "rumor.concurrency.overfill", "join_event"
	w := &world{needSteps: 1, all: steps(5)}
	got, err := Shrink(context.Background(), Request{
		Seed: 7, Steps: steps(5), Kinds: []string{"create_event"},
		Want: overfillWant{rule: rule, subject: subject}, Provider: &fake{}, Checkpoint: cp,
		Replay: func(c context.Context, n int, without []string) (Candidate, error) {
			_, ran, rerr := w.replay(c, n, without)
			if rerr != nil {
				return Candidate{}, rerr
			}
			// THE VERDICT RECURS ONLY WHILE THE SETUP IS THERE, which is
			// fable's ruling 2 -- what shrinks is the setup -- so the fixture
			// has something for the reduction to find rather than a verdict
			// that is true at every length.
			return stageCand(rule, subject, len(ran) >= 3, ran), nil
		},
	})
	if err != nil {
		t.Fatalf("Shrink over a stage verdict: %v", err)
	}
	if !got.Stable {
		t.Fatalf("the stage verdict did not clear the gate: %s", got.Why)
	}
	out := got.Render()
	// THE TRACE SPEAKS THE ORACLE'S SENTENCE, names the oracle and names the
	// subject. Without the carrier this line read `trace: finding no finding`
	// over a trace that reproduced a real defect.
	for _, want := range []string{
		"trace: verdict " + subject + " held 2 against its own limit of 1",
		"by " + rule, "on " + subject,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the rendered trace does not carry %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "no finding") || strings.Contains(out, "fired (none recorded)") {
		t.Errorf("the trace prints the statute's shape over a verdict the statute did not "+
			"author:\n%s", out)
	}
	// AND THE ATTRIBUTION LINE IS ABSENT: `rests_on_planted` is a statute rule
	// and a stage verdict has none, so "the rule, the value and the kind
	// agreed" would be a partial reproduction of something that was never four
	// parts.
	if strings.Contains(out, "PARTIAL REPRODUCTION") {
		t.Errorf("the report claims a partial reproduction over a stage verdict:\n%s", out)
	}
	t.Logf("PROOF\n%s", out)
}

// TWO ORACLES FINDING THE SAME WORD ARE NOT THE SAME FINDING, which is rook's
// /1522 condition driven rather than asserted: the candidate carries a verdict
// whose Rule is another oracle's, and the gate refuses it with the want's own
// sentence. Before the carrier existed there was no field in which they could
// differ.
func TestTwoOraclesFindingTheSameWordDoNotCompareEqual(t *testing.T) {
	w := &world{needSteps: 1, all: steps(4)}
	got, err := Shrink(context.Background(), Request{
		Seed: 7, Steps: steps(4), Kinds: []string{"create_event"},
		Want:     overfillWant{rule: "rumor.concurrency.overfill", subject: "join_event"},
		Provider: &fake{}, Checkpoint: cp,
		Replay: func(c context.Context, n int, without []string) (Candidate, error) {
			_, ran, rerr := w.replay(c, n, without)
			if rerr != nil {
				return Candidate{}, rerr
			}
			return stageCand("rumor.browser.overfill", "join_event", true, ran), nil
		},
	})
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	if got.Stable {
		t.Fatal("a verdict from another oracle cleared the gate, so the two compare equal")
	}
	if !strings.Contains(got.Why, "two oracles") {
		t.Errorf("the refusal is not the want's own words: %q", got.Why)
	}
	t.Logf("PROOF %s", got.Why)
}

// AND A VERDICT ABOUT ANOTHER SUBJECT IS REFUSED TOO, which is the half rook's
// condition does not name and a project declaring two contended actions needs:
// one oracle, two subjects, and a shrink that reduced against the wrong one
// would report a trace for a defect it did not find.
func TestOneOracleTwoSubjectsDoNotCompareEqual(t *testing.T) {
	w := &world{needSteps: 1, all: steps(4)}
	got, err := Shrink(context.Background(), Request{
		Seed: 7, Steps: steps(4), Kinds: []string{"create_event"},
		Want:     overfillWant{rule: "rumor.concurrency.overfill", subject: "join_event"},
		Provider: &fake{}, Checkpoint: cp,
		Replay: func(c context.Context, n int, without []string) (Candidate, error) {
			_, ran, rerr := w.replay(c, n, without)
			if rerr != nil {
				return Candidate{}, rerr
			}
			return stageCand("rumor.concurrency.overfill", "borrow_book", true, ran), nil
		},
	})
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	if got.Stable {
		t.Fatal("a verdict about another subject cleared the gate")
	}
	if !strings.Contains(got.Why, "borrow_book") || !strings.Contains(got.Why, "join_event") {
		t.Errorf("the refusal does not name both subjects: %q", got.Why)
	}
	t.Logf("PROOF %s", got.Why)
}

// THE STATUTE'S PATH IS UNCHANGED BY ALL OF THIS, and it is asserted rather
// than assumed: a candidate with no stage verdict still renders the finding,
// the rules-fired line and the attribution clause. The package's other arms are
// the control for the reduction itself; this one is the control for the
// RENDERER, which is where the two shapes meet.
func TestTheStatutesTraceStillRendersItsOwnShape(t *testing.T) {
	// THE KIND IS REQUIRED SO THE REDUCTION HAS SOMETHING TO REFUSE. My first
	// cut left needKinds empty, so every kind could be removed and the whole
	// trace still reproduced -- and Shrink refuses that by name ("the finding
	// is not a property of the trace ... refusing to report a shrink to zero
	// actions"), which is a sound refusal about a fixture rather than a reading
	// of the renderer. A control that cannot reach the state it is named for
	// reports nothing about it.
	w := &world{needSteps: 3, needKinds: []string{"create_event"}, all: steps(5)}
	got, err := Shrink(context.Background(), Request{
		Seed: 7, Steps: steps(5), Kinds: []string{"create_event"},
		Want: StatuteVerdict{Finding: bug()}, Provider: &fake{}, Checkpoint: cp,
		Replay: func(c context.Context, n int, without []string) (Candidate, error) {
			f, ran, rerr := w.replay(c, n, without)
			if rerr != nil {
				return Candidate{}, rerr
			}
			return Candidate{Finding: f, Steps: ran}, nil
		},
	})
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	out := got.Render()
	if !strings.Contains(out, "trace: finding bug") {
		t.Errorf("the statute's trace no longer prints its finding:\n%s", out)
	}
	if strings.Contains(out, "trace: verdict") {
		t.Errorf("the statute's trace prints a stage verdict's line:\n%s", out)
	}
	t.Logf("PROOF\n%s", out)
}

// AND A PROMOTED STAGE VERDICT WRITES THE ORACLE'S SENTENCE INTO THE LOCK.
//
// THE LOCK IS THE ONE ARTIFACT OF A SHRINK THAT GETS CHECKED IN (ratchet's
// write-once point, coop r6/15), and its comment names the defect the seed
// caught. `finding.Says()` reads "no finding" for a candidate whose verdict the
// statute never authored -- so without this the row for a real race would be
// committed saying nothing was found, and the next reader would have a seed
// nobody can act on. The branch is here rather than in Promote because only the
// shrink knows which verdict it kept.
func TestAPromotedStageVerdictCarriesTheOraclesSentence(t *testing.T) {
	const rule, subject = "rumor.concurrency.overfill", "join_event"
	lock := filepath.Join(t.TempDir(), "seeds.lock")
	w := &world{needSteps: 1, all: steps(5)}
	got, err := Shrink(context.Background(), Request{
		Seed: 11, Steps: steps(5), Kinds: []string{"create_event"},
		Want: overfillWant{rule: rule, subject: subject}, Provider: &fake{}, Checkpoint: cp,
		SeedsFile: lock,
		Replay: func(c context.Context, n int, without []string) (Candidate, error) {
			_, ran, rerr := w.replay(c, n, without)
			if rerr != nil {
				return Candidate{}, rerr
			}
			return stageCand(rule, subject, len(ran) >= 3, ran), nil
		},
	})
	if err != nil {
		t.Fatalf("Shrink: %v", err)
	}
	if !got.Promoted {
		t.Fatal("the stage verdict's seed was not promoted, so there is no row to read")
	}
	raw, rerr := os.ReadFile(lock)
	if rerr != nil {
		t.Fatalf("reading the lock: %v", rerr)
	}
	line := string(raw)
	if !strings.Contains(line, "join_event held 2 against its own limit of 1") {
		t.Errorf("the promoted row does not carry the oracle's sentence: %q", line)
	}
	if strings.Contains(line, "no finding") {
		t.Errorf("the promoted row says nothing was found over a verdict that was: %q", line)
	}
	t.Logf("PROOF %s", strings.TrimSpace(line))
}
