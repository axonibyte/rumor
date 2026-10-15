// Package ledger is rumor's append-only fact store (R2.1, issue 42).
//
// The contract is docs/vocabulary.md v1: four fact kinds and nothing else, a
// position and an actor on every fact, sealing before any check runs, and
// nothing in the ledger ever rewritten or removed.
//
// WHY POSITIONS ARE ASSIGNED HERE AND NOT PASSED IN. The vocabulary says a
// position is "a logical, monotone integer assigned to every fact as it is
// appended". If callers chose positions, monotonicity would be their invariant
// to keep and the store could not promise it. Append assigns; AppendAt exists
// only for Load, which must reproduce positions it is given, and it is the only
// door the seal has to guard.
package ledger

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

// Kind is a fact kind. The vocabulary allows exactly these four.
type Kind string

const (
	KindClaim    Kind = "claim"
	KindObserved Kind = "observed"
	KindTold     Kind = "told"
	KindClock    Kind = "clock"
)

// Source is an observation source. The vocabulary allows exactly these three.
type Source string

const (
	SourceAPI    Source = "api"
	SourceUI     Source = "ui"
	SourceDirect Source = "direct"
)

// Fact is one ledger entry.
//
// ONE STRUCT RATHER THAN FOUR, with `omitempty`, because the dump has to be
// DIFFABLE: a single field order for every kind means a reader comparing two
// ledgers sees only the fields that differ, not a reordering. The cost is that
// kind-specific validity is checked in Validate rather than by the type system,
// so Validate is where every refusal in the vocabulary lands.
type Fact struct {
	Kind     Kind   `json:"kind"`
	Position int    `json:"position"`
	Actor    string `json:"actor"`

	// claim
	Type      string   `json:"type,omitempty"`
	Ref       string   `json:"ref,omitempty"`
	Handles   []string `json:"handles,omitempty"`
	Expect    *int     `json:"expect,omitempty"`
	Planted   bool     `json:"planted,omitempty"`
	VisibleTo []string `json:"visible_to,omitempty"`

	// observed
	Query  string      `json:"query,omitempty"`
	Of     string      `json:"of,omitempty"`
	Source Source      `json:"source,omitempty"`
	Value  interface{} `json:"value,omitempty"`
	Role   string      `json:"role,omitempty"`

	// told
	To       string `json:"to,omitempty"`
	ClaimRef string `json:"claim_ref,omitempty"`

	// clock
	T *int64 `json:"t,omitempty"`
}

// Ledger is an append-only sequence of facts with a sealed prefix.
type Ledger struct {
	facts        []Fact
	seal         int  // 0 means unsealed; otherwise the sealed prefix's last position
	clockControl bool // declared by the run; a clock fact without it is a refusal
}

// New returns an empty ledger. clockControl records whether the run controls the
// clock, because the vocabulary makes a clock fact without it a validation
// refusal rather than a silently accepted wall-clock reading.
func New(clockControl bool) *Ledger { return &Ledger{clockControl: clockControl} }

// Len is the number of facts.
func (l *Ledger) Len() int { return len(l.facts) }

// Seal is the sealed prefix's last position, or 0 when unsealed.
func (l *Ledger) Seal() int { return l.seal }

// Facts returns a copy. A caller holding the slice could otherwise rewrite a
// fact in place, which the vocabulary forbids outright.
func (l *Ledger) Facts() []Fact {
	out := make([]Fact, len(l.facts))
	copy(out, l.facts)
	return out
}

// highest is the largest position held, or 0.
func (l *Ledger) highest() int {
	if len(l.facts) == 0 {
		return 0
	}
	return l.facts[len(l.facts)-1].Position
}

// Append assigns the next position and appends. It is the only way a run writes
// a fact, so monotonicity is this package's invariant and not its caller's.
func (l *Ledger) Append(f Fact) (int, error) {
	f.Position = l.highest() + 1
	if err := l.AppendAt(f); err != nil {
		return 0, err
	}
	return f.Position, nil
}

// AppendAt appends a fact at the position it already carries. Load uses it to
// reproduce a dump; a run uses Append.
func (l *Ledger) AppendAt(f Fact) error {
	if err := l.Validate(f); err != nil {
		return err
	}
	if f.Position <= l.seal {
		return fmt.Errorf("ledger: write at position %d is at or before the seal at %d: "+
			"a sealed prefix is what a check read, so this is an error and never a reorder",
			f.Position, l.seal)
	}
	if f.Position <= l.highest() {
		return fmt.Errorf("ledger: position %d is not after the highest held position %d: "+
			"positions are monotone", f.Position, l.highest())
	}
	// REFERENTIAL INTEGRITY IS ABOUT THE LEDGER'S STATE, NOT THE FACT'S SHAPE,
	// which is why it lives here and not in Validate: a tell passes a REFERENCE
	// to a claim, and v1's "the receiver now knows that claim" cannot hold for a
	// claim nobody has made. v1 does not state this as a rule and it follows
	// from the entry; recorded as a reading rather than left to inference. It is
	// also what makes the position bound STRUCTURAL in the example ledgers
	// instead of something each statute has to re-apply.
	if f.Kind == KindTold && !l.hasClaimBefore(f.ClaimRef, f.Position) {
		return fmt.Errorf("ledger: tell at position %d references claim %q, which no earlier "+
			"position holds: a tell cannot pass a claim that has not been made", f.Position, f.ClaimRef)
	}
	l.facts = append(l.facts, f)
	return nil
}

// SealAt seals the prefix ending at position p.
func (l *Ledger) SealAt(p int) error {
	if p < l.seal {
		return fmt.Errorf("ledger: cannot move the seal back from %d to %d", l.seal, p)
	}
	if p > l.highest() {
		return fmt.Errorf("ledger: cannot seal at %d, past the highest held position %d: "+
			"a seal names a prefix that exists", p, l.highest())
	}
	l.seal = p
	return nil
}

// hasClaimBefore reports whether a claim with this ref sits at an earlier position.
func (l *Ledger) hasClaimBefore(ref string, before int) bool {
	for _, f := range l.facts {
		if f.Kind == KindClaim && f.Ref == ref && f.Position < before {
			return true
		}
	}
	return false
}

// Validate applies every refusal docs/vocabulary.md v1 states.
func (l *Ledger) Validate(f Fact) error {
	if f.Position <= 0 {
		return fmt.Errorf("ledger: fact of kind %q has no position: the engine never writes a fact without one", f.Kind)
	}
	if f.Actor == "" {
		return fmt.Errorf("ledger: fact at position %d has no actor: the engine never writes a fact without one", f.Position)
	}
	switch f.Kind {
	case KindClaim:
		if f.Type == "" {
			return fmt.Errorf("ledger: claim at position %d has no type", f.Position)
		}
		if f.Ref == "" {
			return fmt.Errorf("ledger: claim at position %d has no ref: claims are spoken of by handle", f.Position)
		}
	case KindObserved:
		// R4: an observation with no position or no source is a validation refusal.
		if f.Query == "" {
			return fmt.Errorf("ledger: observation at position %d has no query", f.Position)
		}
		switch f.Source {
		case SourceAPI, SourceUI, SourceDirect:
		case "":
			return fmt.Errorf("ledger: observation at position %d has no source (R4)", f.Position)
		default:
			return fmt.Errorf("ledger: observation at position %d has source %q, not one of api, ui, direct", f.Position, f.Source)
		}
		if f.Planted {
			return fmt.Errorf("ledger: observation at position %d carries `planted`, which is a tag on a claim alone", f.Position)
		}
	case KindTold:
		if f.To == "" {
			return fmt.Errorf("ledger: tell at position %d has no recipient", f.Position)
		}
		if f.ClaimRef == "" {
			return fmt.Errorf("ledger: tell at position %d carries no claim reference: v1 adopts the reference form", f.Position)
		}
		if f.To == f.Actor {
			return fmt.Errorf("ledger: tell at position %d is from %q to itself", f.Position, f.Actor)
		}
	case KindClock:
		if !l.clockControl {
			return fmt.Errorf("ledger: clock fact at position %d with no clock control declared (R4, R7): "+
				"wall-clock time never enters the ledger", f.Position)
		}
		if f.T == nil {
			return fmt.Errorf("ledger: clock fact at position %d carries no time", f.Position)
		}
	case "":
		return fmt.Errorf("ledger: fact at position %d has no kind", f.Position)
	default:
		return fmt.Errorf("ledger: fact at position %d has kind %q: the ledger holds claim, observed, told and clock and nothing else", f.Position, f.Kind)
	}
	if f.Planted && f.Kind != KindClaim {
		return fmt.Errorf("ledger: `planted` on a %s at position %d: it is a tag on a claim", f.Kind, f.Position)
	}
	return nil
}

// SealedPrefix returns the facts a check may read: those at or before the seal.
// An unsealed ledger yields nothing, deliberately -- the vocabulary says a
// ledger is sealed BEFORE any check runs, so a check reading an unsealed ledger
// is a sequencing error, and returning everything would hide it.
func (l *Ledger) SealedPrefix() []Fact {
	var out []Fact
	for _, f := range l.facts {
		if f.Position <= l.seal {
			out = append(out, f)
		}
	}
	return out
}

// Dump writes the ledger as one JSON object per line, ascending by position.
//
// DETERMINISTIC AND DIFFABLE, which are two claims and both are tested: the same
// logical content yields byte-identical output (one field order from the struct,
// positions ascending, no map iteration anywhere), and one fact per line means a
// diff names the fact that changed rather than reflowing the file.
func (l *Ledger) Dump(w io.Writer) error {
	facts := l.Facts()
	sort.Slice(facts, func(i, j int) bool { return facts[i].Position < facts[j].Position })
	bw := bufio.NewWriter(w)
	enc := json.NewEncoder(bw)
	enc.SetEscapeHTML(false)
	for _, f := range facts {
		if err := enc.Encode(f); err != nil {
			return fmt.Errorf("ledger: dumping position %d: %w", f.Position, err)
		}
	}
	return bw.Flush()
}

// Load reads a dump. It goes through AppendAt, so a dump that violates any rule
// the writer enforces is refused on the way in rather than trusted because it
// came from a file.
func Load(r io.Reader, clockControl bool) (*Ledger, error) {
	l := New(clockControl)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	line := 0
	for sc.Scan() {
		line++
		raw := sc.Bytes()
		if len(raw) == 0 {
			continue
		}
		var f Fact
		dec := json.NewDecoder(bytesReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&f); err != nil {
			return nil, fmt.Errorf("ledger: line %d: %w", line, err)
		}
		if err := l.AppendAt(f); err != nil {
			return nil, fmt.Errorf("ledger: line %d: %w", line, err)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("ledger: reading: %w", err)
	}
	return l, nil
}

// bytesReader avoids importing bytes just for one reader in Load.
func bytesReader(b []byte) io.Reader { return &sliceReader{b: b} }

type sliceReader struct {
	b []byte
	i int
}

func (r *sliceReader) Read(p []byte) (int, error) {
	if r.i >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.i:])
	r.i += n
	return n, nil
}
