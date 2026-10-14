// Package handle keeps the run-local handle table: `actor.type#n` to the id
// the server returned, for ONE attempt.
//
// WHY PER ATTEMPT, which is the whole design. The vocabulary's Handles
// heading: "A replay re-creates the entities, the server hands back new ids,
// the handles stay the same, and the findings are compared by handle." So the
// handle is the stable name and the id is the disposable one -- the opposite
// of the usual arrangement, and the reason raw ids never enter the ledger's
// logical content, a statute, or any comparison. A table that outlived its
// attempt would map a handle to an id from a previous run, and every finding
// compared through it would be about a server state that no longer exists.
//
// WHICH COLLISION IS v1's AND WHICH IS MINE, now that the draft answers.
//
// v1 NAMES THE BETWEEN-ATTEMPTS ONE (amended on r1.3-vocabulary-2 after I
// raised its absence at thread:r1.vocab/7): "a retried attempt produces the
// same handle with a different server id, and the table keeps the current
// attempt's id only. A claim that names a handle absent from the current
// attempt's table is refused at the ledger as a validation error (R4), never
// judged as a finding." NextAttempt implements the first half; Require is the
// seam for the second, and the refusal itself is the ledger's to make.
//
// THE TWO WITHIN-ATTEMPT CONFLICTS ARE STILL MINE, and v1's new line supports
// them rather than stating them -- "a handle names at most one entity per run"
// is why both are refused:
//
//   - the same handle bound twice to DIFFERENT ids. The handle is the
//     identity; letting the second win would silently redirect every claim
//     already made about it.
//   - two handles bound to the SAME id. Two logical entities resolving to one
//     server entity means the run created fewer things than it believes, and
//     a comparison by handle would then report agreement between claims about
//     different entities.
//
// A rebinding to the IDENTICAL id is permitted and tested: retrying a create
// that actually succeeded is not a conflict, and refusing it would make the
// table refuse its own idempotence.
package handle

import (
	"fmt"
	"regexp"
	"sort"
)

// shape is the vocabulary's handle form: actor.type#n, n from 1.
//
// DUPLICATED FROM THE VOCABULARY DELIBERATELY. If this read the shape from
// somewhere the vocabulary also generated, the two would agree whatever
// either said; written out, a change to v1's Handles heading has to be made
// here too and the duplication is the check.
var shape = regexp.MustCompile(`^[a-z][a-z0-9_]*\.[a-z][a-z0-9_.]*#[1-9][0-9]*$`)

// Valid reports whether s is a well-formed run-local handle.
func Valid(s string) bool { return shape.MatchString(s) }

// Table is one attempt's bindings. Not safe for concurrent use: an attempt is
// a sequence of actions by construction, and a mutex here would imply the
// run may interleave them, which R2 does not do.
type Table struct {
	attempt  int
	byHandle map[string]string
	byID     map[string]string
}

// New returns the table for an attempt, numbered from 1.
func New(attempt int) (*Table, error) {
	if attempt < 1 {
		return nil, fmt.Errorf("handle: attempt %d is not an attempt: they are numbered from 1", attempt)
	}
	return &Table{attempt: attempt, byHandle: map[string]string{}, byID: map[string]string{}}, nil
}

// Attempt is the attempt this table belongs to.
func (t *Table) Attempt() int { return t.attempt }

// Bind records that `handle` is the entity the server returned `id` for.
func (t *Table) Bind(handle, id string) error {
	if !Valid(handle) {
		return fmt.Errorf("handle: %q is not a run-local handle; v1's form is actor.type#n", handle)
	}
	if id == "" {
		return fmt.Errorf("handle: %s was bound to an empty id, which records nothing", handle)
	}
	if was, ok := t.byHandle[handle]; ok {
		if was == id {
			return nil // idempotent: the same create reported twice
		}
		return fmt.Errorf("handle: %s is already bound to %q and cannot be rebound to %q in attempt %d: the handle is the identity, so a second binding would redirect every claim already made about it", handle, was, id, t.attempt)
	}
	if other, ok := t.byID[id]; ok {
		return fmt.Errorf("handle: id %q is already bound to %s, so binding %s to it would make two handles name one entity in attempt %d: a comparison by handle would then agree about different things", id, other, handle, t.attempt)
	}
	t.byHandle[handle] = id
	t.byID[id] = handle
	return nil
}

// ID is the server id for a handle in this attempt.
func (t *Table) ID(handle string) (string, bool) {
	id, ok := t.byHandle[handle]
	return id, ok
}

// Handle is the reverse: which handle an id belongs to.
//
// IT EXISTS FOR ONE JOB -- turning a server's response back into the logical
// name before anything is written down -- and not for comparisons, which are
// by handle. A finding that needed this would be a finding about an id.
func (t *Table) Handle(id string) (string, bool) {
	h, ok := t.byID[id]
	return h, ok
}

// Require is ID for a caller that must not proceed without the id.
//
// IT EXISTS TO MAKE ONE MISTAKE HARD. v1: "A claim that names a handle absent
// from the current attempt's table is refused at the ledger as a validation
// error (R4), NEVER JUDGED AS A FINDING." A caller holding (string, bool) can
// treat the false as a disagreement and emit `bug`, which would report a
// bookkeeping gap as a defect in the system under test -- the worst available
// failure, because it looks like the product working. The error says what it
// is, and names the attempt, since "absent" is only meaningful against one.
//
// The REFUSAL is still the ledger's to make: this package does not know what
// is being validated. What it owes is an answer a caller cannot mistake.
func (t *Table) Require(handle string) (string, error) {
	if !Valid(handle) {
		return "", fmt.Errorf("handle: %q is not a run-local handle; v1's form is actor.type#n", handle)
	}
	id, ok := t.byHandle[handle]
	if !ok {
		return "", fmt.Errorf("handle: %s is absent from attempt %d's table, so nothing in this attempt names that entity. This is a VALIDATION refusal and never a finding (v1, Handles): a claim naming it is refused at the ledger", handle, t.attempt)
	}
	return id, nil
}

// Handles lists the bound handles, sorted, so a dump or a diff is stable.
func (t *Table) Handles() []string {
	out := make([]string, 0, len(t.byHandle))
	for h := range t.byHandle {
		out = append(out, h)
	}
	sort.Strings(out)
	return out
}

// NextAttempt returns an empty table for the following attempt.
//
// EMPTY, NOT COPIED, and that is the point: the ids from the previous attempt
// are stale the moment the entities are re-created. Returning a copy would be
// the defect this package exists to prevent, and it would be invisible until
// a replay compared a finding against an id that no longer resolves.
func (t *Table) NextAttempt() (*Table, error) { return New(t.attempt + 1) }
