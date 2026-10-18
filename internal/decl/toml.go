package decl

import (
	"fmt"

	"github.com/BurntSushi/toml"
)

// TOML decodes a declaration file as TOML. It is the Decoder every caller in
// this tree passes, and it is a function rather than the default so that the
// dependency stays visible at the call site.
//
// THE READER IS PINNED AND THE CHOICE WAS RULED, not taken quietly:
// github.com/BurntSushi/toml v1.6.0, MIT, passes the licence gate's own
// discriminators -- its licence file is COPYING, which tests/licences matches,
// and classify() fires exactly one family, so no dualApproved entry is owed
// (rook, coop r4/17; fable's ruling, r4/30). `platform.toml` and a package's
// `bindings.toml` share this one reader, which is why it lives beside the
// schema-independent half rather than in either schema's package.
//
// WHAT IT DOES NOT DO is the point of the split: strictness is the Reader's.
// This function answers "is this TOML" and nothing else, so a mistyped key
// arrives as a document and is refused by name with its path, rather than
// arriving as a parse error that names a line and not a meaning.
func TOML(data []byte) (map[string]any, error) {
	var doc map[string]any
	if err := toml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("not TOML: %w", err)
	}
	// NO nil GUARD HERE, AND THAT IS MEASURED RATHER THAN ASSUMED. The first
	// version returned an empty map when `doc` came back nil, on the theory
	// that a keyless document might decode to nil -- and the mutant that
	// deleted the guard SURVIVED, which is the only reason it got measured.
	// v1.6.0 always allocates: an empty file, a comments-only file and a
	// blank-line file each decode to a non-nil map of length 0. An unreachable
	// guard is not a cautious one (docs/testing-methodology.md), so it is gone,
	// and the reachable form of the same worry lives in Load, where a decoder
	// answering (nil, nil) is refused by name and driven by a test.
	return doc, nil
}
