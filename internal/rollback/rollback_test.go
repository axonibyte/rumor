// Package rollback's own test: the registry and the two sentinels.
//
// IT RUNS AGAINST AN EMPTY REGISTRY, which is what makes the "none is
// registered at all" arm reachable: the one shipped provider registers itself
// from internal/rollback/toyrestart, a package this one does not import, so
// nothing has put anything here.
package rollback

import (
	"errors"
	"strings"
	"testing"
)

// clean empties the registry around a test, so one case's registration cannot
// decide another's verdict.
func clean(t *testing.T) {
	t.Helper()
	mu.Lock()
	saved := factories
	factories = map[string]Factory{}
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		factories = saved
		mu.Unlock()
	})
}

func stub(Options) (Provider, error) { return nil, nil }

func TestOpenOnAnEmptyRegistrySaysSoRatherThanNamingNothing(t *testing.T) {
	clean(t)
	_, err := Open("anything", Options{Dir: t.TempDir()})
	if err == nil {
		t.Fatal("a provider opened out of an empty registry")
	}
	// "no provider named x; registered: []" sends a reader to look for a typo
	// in a list that is not there. The empty case says what is true instead.
	if !strings.Contains(err.Error(), "none is registered at all") {
		t.Errorf("the error does not distinguish an empty registry: %v", err)
	}
}

func TestOpenNamesWhatIsRegistered(t *testing.T) {
	clean(t)
	Register("alpha", stub)
	Register("beta", stub)
	_, err := Open("alfa", Options{Dir: t.TempDir()})
	if err == nil {
		t.Fatal("a misspelled provider opened")
	}
	for _, want := range []string{"alpha", "beta"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error omits %q, which is where the typo shows: %v", want, err)
		}
	}
}

func TestRegisteredIsSortedSoAMessageIsStable(t *testing.T) {
	clean(t)
	for _, n := range []string{"gamma", "alpha", "beta"} {
		Register(n, stub)
	}
	got := Registered()
	want := []string{"alpha", "beta", "gamma"}
	if len(got) != len(want) {
		t.Fatalf("Registered() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Registered() = %v, want %v: an unsorted list makes an error message "+
				"depend on map iteration, which Go randomizes on purpose", got, want)
		}
	}
}

// A DUPLICATE PANICS RATHER THAN WINNING. Two providers under one name is
// unresolvable, and a silent last-wins would make which one you got depend on
// import order -- a defect that moves when an unrelated file adds an import.
func TestRegisteringTwiceUnderOneNamePanics(t *testing.T) {
	clean(t)
	Register("alpha", stub)
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("a duplicate registration was accepted")
		}
		if !strings.Contains(stringOf(r), "alpha") {
			t.Errorf("the panic does not name the provider: %v", r)
		}
	}()
	Register("alpha", stub)
}

func TestRegisteringNothingUsefulPanics(t *testing.T) {
	for _, c := range []struct {
		what string
		name string
		f    Factory
	}{
		{"an empty name", "", stub},
		{"a nil factory", "alpha", nil},
	} {
		t.Run(c.what, func(t *testing.T) {
			clean(t)
			defer func() {
				if recover() == nil {
					t.Errorf("%s was accepted", c.what)
				}
			}()
			Register(c.name, c.f)
		})
	}
}

// THE TWO SENTINELS ARE DISTINCT, which is the whole of their job: a caller
// refuses a shrink step on one and reports could-not-run on the other, so a
// wrapper that collapsed them would make a dirty world look like a missing
// handle.
func TestTheSentinelsAreDistinguishable(t *testing.T) {
	if errors.Is(ErrDirty, ErrUnknownCheckpoint) || errors.Is(ErrUnknownCheckpoint, ErrDirty) {
		t.Fatal("ErrDirty and ErrUnknownCheckpoint are not distinguishable")
	}
	wrapped := errors.Join(ErrDirty, errors.New("context"))
	if !errors.Is(wrapped, ErrDirty) {
		t.Error("a wrapped ErrDirty stops being recognisable, so a caller cannot refuse on it")
	}
}

func stringOf(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	if e, ok := v.(error); ok {
		return e.Error()
	}
	return ""
}
