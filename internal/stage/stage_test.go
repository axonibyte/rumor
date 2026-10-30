package stage

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/decl"
	"forge.axonibyte.com/axonibyte/rumor/internal/platform"
)

// toyPlatform loads the SHIPPED toy, not a fixture: the question is what this
// dispatcher does with the declarations the tree actually carries.
func toyPlatform(t *testing.T) platform.Platform {
	t.Helper()
	p := filepath.Join("..", "..", "examples", "toy", "platform.toml")
	rep, err := platform.Load(p, decl.TOML)
	if err != nil {
		t.Fatalf("loading %s: %v", p, err)
	}
	if !rep.OK() {
		t.Fatalf("%s is not loadable: %v", p, rep.Refusals)
	}
	return rep.Platform
}

// RAN, NOT DECLARED -- rook's partner at coop r8/14. The toy declares `api`,
// `concurrency` and `browser`, all ENABLED, and `browser` is one this build
// cannot run. A selection that named every declared stage would return all
// three, which is the defect the drill is about, so the arm asserts the
// declaration it must NOT follow.
//
// THE EXPECTATION IS THE RUNNABLE SET AND NOT A LITERAL, which is what 109
// moving the toy's declarations taught: this arm read `just [api]` and failed
// the moment `concurrency` became runnable and the toy enabled it -- correctly,
// and for a reason that was not its subject. The discriminator was never the
// COUNT; it is that an enabled stage nobody can run is absent. So the set is
// computed from Known and `browser` is asserted present-and-enabled-and-absent
// by name: the arm now fails when a stage that cannot run is selected, or when
// one that can is dropped, and not when the toy declares one more.
func TestSelectReturnsWhatRunsAndNotWhatIsDeclared(t *testing.T) {
	p := toyPlatform(t)
	if st, ok := p.Stages["browser"]; !ok || !st.Enabled {
		t.Fatal("the toy no longer declares browser as enabled, so this arm cannot tell ran from declared")
	}
	if Known["browser"] {
		t.Fatal("this build now runs browser, so the toy's declarations no longer hold a stage " +
			"that is enabled and unrunnable -- which is the only thing this arm can read")
	}
	var want []string
	for _, name := range knownNames() {
		st, declared := p.Stages[name]
		if Known[name] && declared && st.Enabled {
			want = append(want, name)
		}
	}
	if len(want) == 0 {
		t.Fatal("the toy enables no stage this build runs, so a selection would be a refusal")
	}
	got, err := Select(p, "")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if !slices.Equal(got, want) {
		t.Errorf("Select returned %v and the toy enables %v of the stages this build runs: "+
			"browser is declared and enabled and this build cannot run it, so it must not appear",
			got, want)
	}
	for _, name := range got {
		if name == "browser" {
			t.Errorf("browser was selected and no build can run it: %v", got)
		}
	}
	t.Logf("PROOF declared %d stage(s), ran %v", len(p.Stages), got)
}

// A PROFILE NARROWS, and the toy's own `expiring` names only `api`.
func TestAProfileNarrowsTheSelection(t *testing.T) {
	p := toyPlatform(t)
	got, err := Select(p, "expiring")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if len(got) != 1 || got[0] != API {
		t.Errorf("profile expiring selected %v", got)
	}
}

// AN UNKNOWN DECLARED STAGE IS REFUSED BY NAME, BEFORE THE RUN, and the
// refusal lists what IS known -- rook measured at r8/16 that nothing else in
// the stack objects: `stages.fuzzz = { enabled = true }` parses and counts.
func TestAnUnknownDeclaredStageIsRefusedByName(t *testing.T) {
	p := toyPlatform(t)
	p.Stages["fuzzz"] = platform.Stage{Enabled: true}
	_, err := Select(p, "")
	if err == nil {
		t.Fatal("a stage no build can run returned a selection")
	}
	if !strings.Contains(err.Error(), "fuzzz") {
		t.Errorf("the refusal does not name the stage: %v", err)
	}
	for _, known := range []string{API, "fuzz", "concurrency", "browser"} {
		if !strings.Contains(err.Error(), known) {
			t.Errorf("the refusal does not list the known stage %q: %v", known, err)
		}
	}
	t.Logf("PROOF %v", err)
}

// AN EMPTY SELECTION IS A REFUSAL AND NAMES ITS REASON PER STAGE. Both reasons
// are driven, because the first version of this message listed the sets and
// not the cause -- it read "selects api, and this build runs api" over a run
// that took no stage, true in every clause and useless.
func TestAnEmptySelectionRefusesWithTheReason(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		p := toyPlatform(t)
		// EVERY RUNNABLE STAGE THE PROJECT ENABLES, and not `api` alone: this
		// cell disabled the one name and stopped emptying the selection the
		// moment a second stage became runnable and the toy enabled it. A cell
		// that no longer reaches the state it is named for passes on the
		// refusal it was not testing.
		for name := range Known {
			if _, declared := p.Stages[name]; declared && Known[name] {
				p.Stages[name] = platform.Stage{Enabled: false}
			}
		}
		_, err := Select(p, "")
		if err == nil {
			t.Fatal("a platform with its only runnable stage disabled returned a selection")
		}
		if !strings.Contains(err.Error(), "enabled = false") {
			t.Errorf("the refusal does not say the stage is disabled: %v", err)
		}
		t.Logf("PROOF %v", err)
	})
	t.Run("known but not runnable yet", func(t *testing.T) {
		p := toyPlatform(t)
		prof := p.Profiles["expiring"]
		prof.Stages = []string{"browser"}
		p.Profiles["expiring"] = prof
		_, err := Select(p, "expiring")
		if err == nil {
			t.Fatal("a profile selecting only an unrunnable stage returned a selection")
		}
		if !strings.Contains(err.Error(), "cannot run it yet") {
			t.Errorf("the refusal does not say the stage is not built yet: %v", err)
		}
		t.Logf("PROOF %v", err)
	})
}

// THE REFUSAL NAMES WHAT ASKED, and this is the arm for the bug driving it
// found: with a profile named, the message reported the PLATFORM's
// declarations under the profile's name.
func TestTheRefusalNamesTheProfilesOwnSelectionAndNotTheDeclarations(t *testing.T) {
	p := toyPlatform(t)
	prof := p.Profiles["expiring"]
	prof.Stages = []string{API}
	p.Profiles["expiring"] = prof
	p.Stages[API] = platform.Stage{Enabled: false}
	_, err := Select(p, "expiring")
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if strings.Contains(err.Error(), "browser") {
		t.Errorf("the refusal names browser, which the profile does not select: %v", err)
	}
}

// A PROFILE NOBODY DECLARED IS REFUSED rather than read as selecting nothing,
// which would be indistinguishable from a platform that enables nothing.
func TestAnUndeclaredProfileIsRefused(t *testing.T) {
	p := toyPlatform(t)
	_, err := Select(p, "nonesuch")
	if err == nil {
		t.Fatal("an undeclared profile returned a selection")
	}
	if !strings.Contains(err.Error(), "nonesuch") {
		t.Errorf("the refusal does not name the profile: %v", err)
	}
}

// THE ORDER IS CANONICAL AND NOT A MAP's, so two projects declaring the same
// stages differently run them alike and a record can be compared.
func TestTheOrderIsCanonicalAcrossPasses(t *testing.T) {
	for i := 0; i < 20; i++ {
		got := knownNames()
		if got[0] != API {
			t.Fatalf("pass %d put %q first, not the main loop", i, got[0])
		}
		for j := 1; j < len(got); j++ {
			if got[j-1] > got[j] {
				t.Fatalf("pass %d is not sorted after the main loop: %v", i, got)
			}
		}
	}
}

// EVERY SELECTABLE STAGE MUST HAVE A DISPATCHER THAT CONSUMES IT.
//
// Select returns a LIST, and through 108 cmd/rumor read exactly one element of
// it -- `Stage: stages[0]` -- and walked unconditionally. So the moment a second
// name in Known became runnable, a project enabling both would have had one
// stage run and one SELECTED AND SILENTLY DROPPED: no refusal, no skip reason,
// and a record naming only the first. That is the shape the dispatch was built
// to end (stages declared and never dispatched, my r8/6) reappearing one level
// up.
//
// IT FIRED, AND WHAT IT BOUGHT. 109 flipped `concurrency` to runnable and this
// arm failed in the same change, reading "Select returned [api concurrency] ...
// and cmd/rumor/run.go:185 consumes stages[0] alone -- so 1 of them would be
// dropped". The fix was its own instruction: the dispatch became a loop with a
// case per stage, and this arm now compares the runnable set against the set
// that loop CONSUMES rather than against the single name `api`.
//
// consumed IS A LIST IN THIS FILE AND THAT IS THE POINT. It is not derived from
// Known -- a set derived from the thing it checks could not disagree with it --
// so the next stage to become runnable fails here until somebody adds its case
// to cmd/rumor's switch AND names it below. When this fails, the fix is not to
// append to the list: it is to make the dispatch consume the stage, then say so
// here.
func TestEverySelectableStageHasADispatcherThatConsumesIt(t *testing.T) {
	// The cases in cmd/rumor/run.go's dispatch loop: `stage.API` runs
	// walkStage, `stage.Concurrency` runs concurrencyStage, `stage.Fuzz` runs
	// fuzzStage.
	consumed := []string{API, Concurrency, Fuzz}

	// The strongest declaration a project can make: every name this
	// dispatcher knows, all enabled.
	all := platform.Platform{Stages: map[string]platform.Stage{}}
	for name := range Known {
		all.Stages[name] = platform.Stage{Enabled: true}
	}

	var runnable []string
	for _, name := range knownNames() {
		if Known[name] {
			runnable = append(runnable, name)
		}
	}

	got, err := Select(all, "")
	if err != nil {
		t.Fatalf("a platform enabling every known stage was refused, so this arm cannot see what "+
			"the dispatcher would consume: %v", err)
	}
	if !slices.Equal(got, consumed) {
		t.Fatalf("Select returned %v over every-stage-enabled and cmd/rumor's dispatch consumes "+
			"%v: a selected stage with no case is run by nothing, and it would be dropped without "+
			"a refusal or a skip reason. Extend the dispatch to consume every selected stage in "+
			"the same change that made one runnable, then name it here.", got, consumed)
	}
	if !slices.Equal(runnable, consumed) {
		t.Fatalf("Known marks %v runnable and cmd/rumor's dispatch consumes %v. The selection "+
			"above happens to agree, so the drop is not visible yet -- it will be for the first "+
			"project that enables the odd one out.", runnable, consumed)
	}
}
