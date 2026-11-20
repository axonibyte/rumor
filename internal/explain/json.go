// The derivation as JSON (#132, R11 d3): the same value d1 builds and d2
// renders for a reader, in the form the visual-diagnosis phase consumes.
//
// IT IS THE SAME VALUE AND NOT A SECOND SHAPE. There is no JSON-only struct
// here, no second set of tags, and no field this form carries that the Go
// value does not: the schema in docs/findings.md is a description of
// Derivation, so a change to the data cannot leave the schema behind without
// failing the pinned fixture by name.

package explain

import (
	"encoding/json"
	"fmt"
	"io"
)

// RenderJSON writes the derivation as JSON.
//
// STABLE FOR ONE FINDING, which is d3's own requirement. Every ordering in
// the value is total -- the chain by hops then position then ref, the statute
// lines by module then row, the claims by the position the statute was given
// them -- and the one map in the shape, Holds, is written by encoding/json in
// sorted key order. So two runs over one finding are byte-identical, and a
// diff of two JSON files is a diff of the findings and not of a map's
// iteration.
//
// HTML ESCAPING IS OFF, as internal/ledger's Dump already decided for the
// same reason: a `<` in a ref or a query would otherwise arrive as < and
// make the file hostile to a reader and to a diff.
func RenderJSON(w io.Writer, d *Derivation) error {
	if d == nil {
		return fmt.Errorf("explain: no derivation to render, which is a caller's defect and not an empty explanation")
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(d); err != nil {
		return fmt.Errorf("explain: the derivation could not be written as JSON: %w", err)
	}
	return nil
}
