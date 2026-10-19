// Package rollback is the trait a shrink step runs on: five verbs over a world
// that can be put back (R5.2's C3, issue 73).
//
// WHOSE INTERFACE THIS IS, said plainly rather than assumed. fable's R5
// breakout puts "the rollback trait with verify not optional" in R5.3, which is
// quill's, and internal/platform's comment says docs/rollback.md defines the
// provider interface in R13 -- so two later runs have a claim on this shape.
// What is here is the MINIMUM issue 73's C3 needs to exist at all: a provider
// cannot be written against an interface that does not exist, and R5.3 cannot
// consume one that was never built. The five verb names are the issue's own, so
// a widening by R5.3 or a definition by R13 should find them already right;
// anything beyond the five is deliberately absent. Raised in coop r5.toy.
//
// VERIFY IS NOT OPTIONAL, AND THAT IS THE WHOLE DESIGN. docs/design.md: "a
// shrink step run on a world the previous step dirtied would attribute to the
// trace what the leftover state did, and the 'minimal' trace would be minimal
// only in that world". So Verify answers whether the world IS the checkpoint,
// and a provider that cannot tell says so rather than returning nil.
package rollback

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
)

// Checkpoint names one saved world. It is opaque to a caller: what a provider
// needs to put the world back is the provider's business, and an engine that
// read into it would be coupled to one provider's mechanism.
type Checkpoint struct {
	// ID is unique within a provider and is what a caller passes back.
	ID string
}

// Description is what a provider says about itself. Covers names the components
// it can put back, which is the half validate's rollback-coverage arm compares
// against the project's `[[rollback]].covers` and the adapter's components.
type Description struct {
	Provider string
	Covers   []string
	// How is one sentence for a human reading a run's output: a verdict that
	// says "restored" without saying by what mechanism is hard to disbelieve.
	How string
}

// Provider is the trait. Every method takes a context because a restore can
// hang -- restarting a process that will not die is the ordinary case -- and a
// shrink that cannot be bounded is a shrink that stops the run.
type Provider interface {
	Describe() Description

	// Checkpoint saves the world and returns the handle to put it back.
	Checkpoint(ctx context.Context) (Checkpoint, error)

	// Restore puts the world back to the checkpoint. It does NOT promise the
	// world is clean afterwards -- Verify answers that, and a caller that
	// skipped it would be assuming what it came to check.
	Restore(ctx context.Context, cp Checkpoint) error

	// Verify reports whether the world now IS the checkpoint. A nil error
	// means proven clean. ErrDirty means proven otherwise. Any other error
	// means the provider could not tell, which is not the same as clean and
	// must not be read as it.
	Verify(ctx context.Context, cp Checkpoint) error

	// Discard forgets a checkpoint and releases whatever held it.
	Discard(ctx context.Context, cp Checkpoint) error
}

// ErrDirty is Verify's answer when the world is provably NOT the checkpoint.
// It is a sentinel so a caller can tell it from "I could not look": a shrink
// step refuses on the first and reports could-not-run on the second.
var ErrDirty = errors.New("rollback: the world does not match the checkpoint")

// ErrUnknownCheckpoint is for a handle this provider did not issue, or one
// already discarded. It is a defect in the caller rather than in the world.
var ErrUnknownCheckpoint = errors.New("rollback: no such checkpoint")

// Options are what a provider needs to open. Dir is the project directory --
// the same cwd the wire client spawns an adapter in -- and Env passes variable
// NAMES through the way platform.toml's [adapter].env does, so a platform's
// secrets stay out of a file and out of this struct.
type Options struct {
	Dir string
	Env []string
}

// Factory opens a provider. A provider is named in platform.toml's
// `[[rollback]].provider` and resolved HERE rather than imported by the engine:
// the engine cannot import a project's Go code, and for R5 the one shipped
// provider is the toy's.
type Factory func(Options) (Provider, error)

var (
	mu        sync.RWMutex
	factories = map[string]Factory{}
)

// Register adds a factory under the name a platform.toml may ask for. It
// PANICS on a duplicate, because two providers answering one name is a
// programming error that no run could resolve correctly -- and a silent
// last-wins would make which one you got depend on import order.
func Register(name string, f Factory) {
	if name == "" {
		panic("rollback: a provider cannot be registered under an empty name")
	}
	if f == nil {
		panic("rollback: " + name + " registered with a nil factory")
	}
	mu.Lock()
	defer mu.Unlock()
	if _, dup := factories[name]; dup {
		panic("rollback: two providers registered as " + name)
	}
	factories[name] = f
}

// Open resolves the name a project declared. The error NAMES what is
// registered, because "unknown provider" sends the reader to the wrong file:
// the usual cause is a typo in platform.toml, and the fix is visible only
// beside the list.
func Open(name string, o Options) (Provider, error) {
	mu.RLock()
	f, ok := factories[name]
	names := make([]string, 0, len(factories))
	for n := range factories {
		names = append(names, n)
	}
	mu.RUnlock()
	if !ok {
		sort.Strings(names)
		if len(names) == 0 {
			return nil, fmt.Errorf("rollback: no provider named %q, and none is registered at all", name)
		}
		return nil, fmt.Errorf("rollback: no provider named %q; registered: %v", name, names)
	}
	return f(o)
}

// Registered is the names, sorted. It exists for a test and for a diagnostic
// that wants to print them, not for a caller to choose from: a project names
// its provider and the engine does not pick one.
func Registered() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(factories))
	for n := range factories {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
