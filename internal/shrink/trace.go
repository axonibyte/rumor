package shrink

import (
	"fmt"
	"strings"

	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
	"forge.axonibyte.com/axonibyte/rumor/internal/schedule"
)

// Trace is a shrunk failure, written in the shape docs/design.md fixes: "The
// result is written as a minimal trace, which names the output's shape (the
// actions, their positions, their handles and the rules that fired) and not a
// property of it".
//
// SO THE NAME IS A FORMAT AND NEVER A CLAIM. "Minimal" is what the document
// calls this output, and the document says in the same sentence that it names a
// shape rather than a property -- the bisection returns a local minimum
// (Bisect's own comment says why), so a renderer that printed "minimal" as a
// verdict would promise what no step established. The word therefore does not
// appear in what Render writes; the property R5 is held to is stated in the
// epic as three toy bugs under five actions, and that is measured elsewhere.
type Trace struct {
	// Seed is the integer the run descends from, so the trace can be replayed.
	Seed uint64

	// Steps are the surviving steps, in order, carrying their own positions.
	Steps []schedule.Step

	// Finding is the verdict the shrunk trace still produces. Its Fired
	// definitions are the "rules that fired" half of the shape.
	Finding engine.Finding
}

// Render writes the trace as the lines a reader and a diff both need.
//
// STABLE AND LINE-ORIENTED, because two attempts from one seed must produce
// BYTE-IDENTICAL output: the comparison the vocabulary asks for is over
// handles, and a renderer that sorted a map or printed an address would make
// two equal traces look different. Every field below comes from the record in
// the order the record holds it.
//
// POSITIONS ARE THE STEP'S OWN, not the slice index. A shrunk trace has gaps --
// kind removal takes whole kinds out of the middle -- and renumbering would
// produce a trace that reads as contiguous and replays as something else.
// internal/schedule's Step.N is the position the walk assigned, and that is
// what a reader needs to match against the run it came from.
func (t Trace) Render() string {
	var b strings.Builder
	fmt.Fprintf(&b, "trace: seed %d, %d step(s)\n", t.Seed, len(t.Steps))
	for _, s := range t.Steps {
		kind := s.Kind
		if kind == "" {
			// NOTHING APPLICABLE IS A STEP THE WALK RECORDED, and a shrunk
			// trace keeps it for the reason the walk did: a record that
			// silently dropped a stalled step would let two traces agree while
			// their draws differed.
			kind = "(nothing applicable)"
		}
		planted := ""
		if s.Planted {
			planted = " PLANTED"
		}
		fmt.Fprintf(&b, "trace: %3d %s %s%s %v\n", s.N, s.Actor, kind, planted, s.Handles)
	}

	// THE FINDING AND THE RULES THAT FIRED, which is the half of the shape a
	// reader uses to tell two failures apart. An undefined finding says so
	// rather than printing a bare nil, which Finding.Says exists for.
	fmt.Fprintf(&b, "trace: finding %s", t.Finding.Says())
	if t.Finding.Rule != "" {
		fmt.Fprintf(&b, " by %s", t.Finding.Rule)
	}
	b.WriteString("\n")
	for _, d := range t.Finding.Fired {
		fmt.Fprintf(&b, "trace: fired %s\n", d.String())
	}
	// A FINDING WITH NO FIRED DEFINITION IS SAID OUT LOUD. The engine reads
	// them off a trace, so an empty list means the rules that fired are
	// UNKNOWN rather than that none did -- and a trace that printed nothing
	// there would read as the latter.
	if len(t.Finding.Fired) == 0 {
		b.WriteString("trace: fired (none recorded)\n")
	}
	return b.String()
}

// Handles is every handle the trace's steps minted, in order, with duplicates
// removed. It is what two attempts from one seed are compared by -- "two
// attempts are compared by their findings over handles, never by raw ids or by
// positions" (docs/design.md).
func (t Trace) Handles() []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range t.Steps {
		for _, h := range s.Handles {
			if !seen[h] {
				seen[h] = true
				out = append(out, h)
			}
		}
	}
	return out
}
