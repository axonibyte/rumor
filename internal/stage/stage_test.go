package stage

import (
	"path/filepath"
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

// RAN, NOT DECLARED -- rook's partner at coop r8/14. The toy declares `api`
// AND `browser`, both ENABLED, and only `api` is one this build runs. A
// selection that named every declared stage would return both, which is the
// defect the drill is about, so the arm asserts the declaration it must NOT
// follow.
func TestSelectReturnsWhatRunsAndNotWhatIsDeclared(t *testing.T) {
	p := toyPlatform(t)
	if st, ok := p.Stages["browser"]; !ok || !st.Enabled {
		t.Fatal("the toy no longer declares browser as enabled, so this arm cannot tell ran from declared")
	}
	if st, ok := p.Stages["api"]; !ok || !st.Enabled {
		t.Fatal("the toy no longer declares api as enabled")
	}
	got, err := Select(p, "")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if len(got) != 1 || got[0] != API {
		t.Errorf("Select returned %v, expected just [%s]: browser is declared and enabled and "+
			"this build cannot run it, so it did not run", got, API)
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
		p.Stages[API] = platform.Stage{Enabled: false}
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
// Select returns a LIST and cmd/rumor/run.go reads exactly one element of it:
// `Stage: stages[0]` at run.go:185, which is the Project's single stage for
// `[weights.stage.<name>]`. Nothing reads stages[1:]. So the moment a second
// name in Known becomes runnable, a project enabling both gets one stage run
// and one stage SELECTED AND SILENTLY DROPPED -- no refusal, no skip reason,
// and a record naming only the first. That is the shape the dispatch was built
// to end (stages declared and never dispatched, my r8/6), reappearing one
// level up.
//
// IT IS UNREACHABLE TODAY AND THAT IS WHY IT NEEDS A TRIPWIRE. Only `api` is
// runnable, so the list cannot hold two and no behavior can be driven; a guard
// in run.go would be a refusal no arm could reach, which is a refusal armed by
// nothing. This arm instead fails in the SAME CHANGE that breaks the
// invariant, which is the only moment the fix is cheap.
//
// WHEN THIS FAILS, the fix is not to loosen it: extend the dispatch to run
// every selected stage, then widen this arm to the set it consumes.
func TestEverySelectableStageHasADispatcherThatConsumesIt(t *testing.T) {
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
	if len(got) != 1 || got[0] != API {
		t.Fatalf("Select returned %v over every-stage-enabled, and cmd/rumor/run.go:185 consumes "+
			"stages[0] alone -- so %d of them would be dropped without a refusal or a skip "+
			"reason. Runnable set is now %s. Extend the dispatch to consume every selected "+
			"stage in the same change that made one runnable, then widen this arm to match.",
			got, len(got)-1, strings.Join(runnable, ", "))
	}
	if len(runnable) != 1 {
		t.Fatalf("Known now marks %s as runnable, and the single-stage dispatch at "+
			"cmd/rumor/run.go:185 can consume one. The selection above happens to be short, so "+
			"the drop is not visible yet -- it will be for the first project that enables two.",
			strings.Join(runnable, ", "))
	}
}
