package fuzz

import (
	"strings"
	"testing"
)

// THE ORACLE's OWN BRANCHES, DRIVEN BY FIXTURE.
//
// WHY THIS FILE EXISTS: the live arm in tests/run asserts the TOY's spread --
// 8 distinct messages over 21 system refusals, and no refusal naming nothing.
// That is the "a specific set passes" half, and it never reaches the branches
// that emit a finding. Measured at d291b10: turning BOTH D4 branches off with
// `if false && ...` built clean and left internal/fuzz and tests/run green, so
// every finding D4 can emit was a refusal nobody could show failing (rook,
// rumor/1488). A clause with no arm is armed by nothing, however many times it
// has been driven by hand in a worktree.
//
// EVERY ARM HERE GOES THROUGH Report.Findings, which is the path a consumer
// takes, so a clause dropped from that composition fails these too -- not only
// a clause whose own condition was broken.

// sys is a refusal from a rule the system wrote: 4xx, no fault, and an error
// text that is not encoding/json's.
func sys(action, field, msg string) Result {
	return Result{
		Case:   Case{Action: action, Class: ClassWrongType, Field: field},
		Status: 400,
		Error:  msg,
	}
}

func findingAbout(t *testing.T, rep Report, want string) string {
	t.Helper()
	var hit []string
	for _, f := range rep.Findings() {
		if strings.Contains(f, want) {
			hit = append(hit, f)
		}
	}
	if len(hit) != 1 {
		t.Fatalf("wanted exactly one finding containing %q, got %d of %v", want, len(hit), rep.Findings())
	}
	return hit[0]
}

func TestOneSentenceOverEveryRefusalIsAFinding(t *testing.T) {
	rep := Report{Results: []Result{
		sys("create_user", "name", "bad request"),
		sys("create_event", "title", "bad request"),
		sys("create_event", "capacity", "bad request"),
	}}
	got := findingAbout(t, rep, "discriminates nothing")
	for _, want := range []string{"all 3 refusal(s)", `"bad request"`} {
		if !strings.Contains(got, want) {
			t.Errorf("the finding does not carry %q, so a reader cannot act on it: %s", want, got)
		}
	}

	// THE CONTROL IS THE SAME SHAPE WITH THE MESSAGES SPREAD, because a clause
	// that fired on any set of refusals would also "catch" a system that
	// discriminates perfectly.
	spread := Report{Results: []Result{
		sys("create_user", "name", "name is required"),
		sys("create_event", "title", "title is required"),
		sys("create_event", "capacity", "capacity must be at least 1"),
	}}
	if f := spread.Findings(); len(f) != 0 {
		t.Errorf("three distinct messages were reported as a monoculture: %v", f)
	}
}

// A MONOCULTURE OF ONE IS NOT A MONOCULTURE. One refusal cannot be shown to be
// uninformative by its own repetition, and a clause that counted it would
// report a finding against every system with a single reachable rule.
func TestASingleRefusalIsNotAMonoculture(t *testing.T) {
	// THE MESSAGE NAMES ITS FIELD, so the other clause is silent and this arm
	// is about the count alone. The first draft used "bad request" here and
	// failed -- correctly, on namesSomething -- which is the arm asserting
	// silence over a finding that was really there.
	rep := Report{Results: []Result{sys("create_user", "name", "name is required")}}
	if f := rep.Findings(); len(f) != 0 {
		t.Errorf("one refusal was reported as a monoculture: %v", f)
	}
}

// THE DECODER's UNIFORMITY IS NOT THE SYSTEM's, which is the restriction that
// makes the count mean anything. Five identical decoder messages beside a
// discriminating system must be silent -- and the same five beside a system
// whose own refusals are uniform must not rescue it.
func TestTheDecodersOwnUniformityIsNotTheSystems(t *testing.T) {
	decoder := func(field string) Result {
		return Result{
			Case:   Case{Action: "create_event", Class: ClassWrongType, Field: field},
			Status: 400,
			Error:  decoderRefusal + " number into Go struct field ." + field + " of type string",
		}
	}
	honest := Report{Results: []Result{
		decoder("title"), decoder("name"), decoder("user"),
		sys("create_user", "name", "name is required"),
		sys("create_event", "capacity", "capacity must be at least 1"),
	}}
	if f := honest.Findings(); len(f) != 0 {
		t.Errorf("a discriminating system was faulted for the decoder's answers: %v", f)
	}

	// The decoder's variety cannot cover for the system.
	covered := Report{Results: []Result{
		decoder("title"), decoder("name"), decoder("user"),
		sys("create_user", "name", "bad request"),
		sys("create_event", "capacity", "bad request"),
	}}
	got := findingAbout(t, covered, "discriminates nothing")
	if !strings.Contains(got, "all 2 refusal(s)") {
		t.Errorf("the count includes answers the system did not give: %s", got)
	}
}

func TestARefusalNamingNeitherTheFieldNorARuleIsAFinding(t *testing.T) {
	rep := Report{Results: []Result{
		sys("create_event", "capacity", "error 400"),
		sys("create_user", "name", "name is required"),
	}}
	got := findingAbout(t, rep, "names neither the field nor a rule")
	for _, want := range []string{"create_event", "capacity", `"error 400"`} {
		if !strings.Contains(got, want) {
			t.Errorf("the finding does not say which case it is about (%q missing): %s", want, got)
		}
	}
}

// BOTH WAYS OF NAMING SOMETHING SATISFY IT, and the arm asserts each
// separately: a clause that only accepted the field name would fault a system
// that states its rule instead, and one that only accepted a rule word would
// fault a system that names the field.
func TestNamingTheFieldAndNamingARuleBothSatisfyTheClause(t *testing.T) {
	byField := Report{Results: []Result{
		sys("create_user", "name", "name is required"),
		sys("create_event", "title", "title is required"),
	}}
	if f := byField.Findings(); len(f) != 0 {
		t.Errorf("a refusal naming the field was faulted: %v", f)
	}

	// `capacity` is absent from this text; " must " is the rule word.
	byRule := Report{Results: []Result{
		sys("create_event", "capacity", "the number of seats must be at least 1"),
		sys("create_user", "name", "name is required"),
	}}
	if f := byRule.Findings(); len(f) != 0 {
		t.Errorf("a refusal stating a rule without the field's name was faulted: %v", f)
	}
	if strings.Contains("the number of seats must be at least 1", "capacity") {
		t.Fatal("this arm's premise is gone: the rule text now contains the field name, so it " +
			"no longer separates the rule branch from the field branch")
	}
}

// A CASE WITH NO PERTURBED FIELD YIELDS NO NAMING FINDING.
//
// WHAT THIS PINS IS THE BEHAVIOR, NOT THE GUARD, and the difference was found
// by driving the over-strict direction: removing `Field != ""` from the clause
// leaves this arm GREEN, because strings.Contains(msg, "") is true for any
// message and the clause falls through regardless. The state is also
// unreachable from Corpus today, since base() fields every case. So this arm
// is defensive, held against the two changes that would make the input real --
// a class perturbing the body rather than one field, or a field test stricter
// than Contains -- and it is NOT evidence that the guard does anything.
func TestAFieldlessCaseYieldsNoNamingFinding(t *testing.T) {
	rep := Report{Results: []Result{
		{Case: Case{Action: "join_event", Class: ClassMissing}, Status: 400, Error: "error 400"},
		sys("create_user", "name", "name is required"),
	}}
	if f := rep.Findings(); len(f) != 0 {
		t.Errorf("a case with no perturbed field was judged for not naming one: %v", f)
	}
}

// D2 IS ARMED THE SAME WAY, and it had the same gap: the only arm over it was
// the toy answering no 5xx.
func TestAFiveHundredIsAFindingAndNamesTheCase(t *testing.T) {
	rep := Report{Results: []Result{
		{Case: Case{Action: "delete_event", Class: ClassUnknownID, Field: "event"}, Status: 500, Error: "boom"},
		sys("create_user", "name", "name is required"),
	}}
	got := findingAbout(t, rep, "not fail the server")
	for _, want := range []string{"delete_event", "unknown-id", "500", "boom"} {
		if !strings.Contains(got, want) {
			t.Errorf("the finding does not carry %q, so it cannot be acted on: %s", want, got)
		}
	}

	// A 4xx IS NOT A 5xx, which is the boundary the clause turns on.
	ok := Report{Results: []Result{sys("delete_event", "event", "no such event")}}
	if f := ok.Findings(); len(f) != 0 {
		t.Errorf("a refusal was reported as a server failure: %v", f)
	}
}

// THE ORACLE's CHANNEL IS ITS OWN PRECONDITION.
//
// `body.error` is declared by nothing -- `[[action]].response` says what
// ACCEPTANCE is and the protocol knows nothing of HTTP -- so an adapter
// spelling the key `message` leaves every refusal's text empty. Measured
// against the live toy with exactly that change: 27 findings, one monoculture
// over "" and 26 refusals "naming nothing", every one false against a system
// that names each field. The ambiguity is real and the stage cannot resolve it
// from a declaration, so it resolves it from the run.
func TestARefusalTextTheStageNeverReadIsACouldNotRunAndNotAFinding(t *testing.T) {
	blind := Report{Results: []Result{
		sys("create_user", "name", ""),
		sys("create_event", "title", ""),
		sys("create_event", "capacity", ""),
	}}
	why := blind.Vacuous()
	if why == "" {
		t.Fatalf("three refusals with no text read as judgeable, and the clauses then say: %v",
			blind.Findings())
	}
	for _, want := range []string{"3 refusal(s)", "body.error", "subject is missing"} {
		if !strings.Contains(why, want) {
			t.Errorf("the refusal does not carry %q, so a reader cannot tell it from a real "+
				"finding: %s", want, why)
		}
	}
}

// ONE SYSTEM REFUSAL THAT SPOKE IS THE POSITIVE CONTROL, and it turns the
// silent ones back into findings -- which is the half that keeps the guard from
// swallowing the defect it sits next to. A system that answers some refusals
// with a message and others with nothing IS uninformative about the others, and
// that is the system's choice rather than the stage's blindness.
func TestOneRefusalThatSpokeProvesTheChannelAndTheSilentOnesAreJudged(t *testing.T) {
	mixed := Report{Results: []Result{
		sys("create_user", "name", "name is required"),
		sys("create_event", "capacity", ""),
	}}
	if why := mixed.Vacuous(); why != "" {
		t.Fatalf("a run that read one refusal's text was called unreadable: %s", why)
	}
	got := findingAbout(t, mixed, "names neither the field nor a rule")
	if !strings.Contains(got, "capacity") {
		t.Errorf("the finding does not name the silent case: %s", got)
	}

	// AND THE ACCEPTED CASE's EMPTY TEXT IS NOT A SILENT REFUSAL. A 2xx
	// carries no error by design, so counting it as one would make every
	// corpus with an accepted case look unreadable.
	accepted := Report{Results: []Result{
		{Case: Case{Action: "create_user", Class: ClassMissing, Field: "name"}, Status: 201},
		sys("create_event", "capacity", "capacity must be at least 1"),
	}}
	if why := accepted.Vacuous(); why != "" {
		t.Fatalf("an accepted case's absent error text was read as a silent refusal: %s", why)
	}

	// AND THE FIXTURE ABOVE DOES NOT ISOLATE IT, which a mutant showed: with
	// the guard looping over every Result instead of the system's refusals,
	// that report still passes, because the refusal beside the accepted case
	// speaks. The separating case is a report with NO system refusal at all --
	// correct code has nothing to judge and says so by staying silent, while a
	// guard that counted every result would call it unreadable.
	onlyAccepted := Report{Results: []Result{
		{Case: Case{Action: "create_user", Class: ClassMissing, Field: "name"}, Status: 201},
		{Case: Case{Action: "create_event", Class: ClassEmpty, Field: "title"}, Status: 200},
	}}
	if len(onlyAccepted.SystemRefusals()) != 0 {
		t.Fatal("this arm's premise is gone: the fixture now holds a system refusal, so it no " +
			"longer separates the refusal set from the result set")
	}
	if why := onlyAccepted.Vacuous(); why != "" {
		t.Errorf("a corpus the system accepted throughout was called unreadable, which is a claim "+
			"about the oracle's channel and not about the corpus: %s", why)
	}
}
