// The observation-source vocabulary is held in TWO packages and joined by a
// cast, so this file reads both.
package contract_test

import (
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/ledger"
	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

// TestTheWireAndLedgerSourceVocabulariesAgreeByValue arms a hazard that was
// documented and armed by nothing.
//
// internal/schedule converts one to the other with `ledger.Source(decl.Source)`
// -- a BARE CAST -- so the two sets must agree BY VALUE, and the compiler cannot
// say so: both are named string types and the conversion is always legal.
//
// ibis MEASURED THE GAP at coop r8/113: putting internal/ledger's
// `SourceBrowser` back to "ui" while keeping the identifier BUILDS and passes
// all 41 packages. internal/ledger's own cell constructs the fact with
// `Source: SourceBrowser`, so it compares the constant with itself and cannot
// see the value move. R8.4's vocabulary v4 row and docs/testing-methodology.md
// both describe the resulting refusal -- at the ledger, at the first observation
// of that source, at runtime rather than at compile time -- and neither is a
// check. This file is the check.
//
// IT COMPARES THE TWO VALUES AND NOT LITERALS, deliberately. A coordinated
// rename of both is legitimate and R8.4 WAS one, so an arm pinning "browser"
// would fail on the next correct move and teach whoever hit it to delete the
// arm. What must never happen is the two drifting APART.
//
// WHAT IT CANNOT DO, in this file's own tradition: the three pairs below are a
// hand-written enumeration, because neither package offers its constants as a
// set a test could walk. A FOURTH source added to one package alone is
// therefore invisible here -- the same shape as the contract tables this
// package already reads by their first column, and the reason the vocabulary
// names its sources in a table rather than in prose.
func TestTheWireAndLedgerSourceVocabulariesAgreeByValue(t *testing.T) {
	for _, p := range []struct {
		what string
		w    wire.Source
		l    ledger.Source
	}{
		{"api", wire.SourceAPI, ledger.SourceAPI},
		{"browser", wire.SourceBrowser, ledger.SourceBrowser},
		{"direct", wire.SourceDirect, ledger.SourceDirect},
	} {
		// NON-EMPTY FIRST: two empty strings compare equal, so without this the
		// agreement below would pass on a constant someone had emptied.
		//
		// AND THE TWO PACKAGES ARE PROTECTED ASYMMETRICALLY, measured while
		// driving this guard: emptying internal/ledger's constant does not
		// COMPILE -- its validator holds `case "":` beside the three, so an
		// empty value is a duplicate case -- while internal/wire has no such
		// clause and an emptied constant there builds and runs. So this guard
		// is load-bearing for wire and redundant for ledger, which is the
		// opposite of where the value hazard above lives.
		if string(p.w) == "" || string(p.l) == "" {
			t.Errorf("the %s source is empty on one side (wire %q, ledger %q), and two empty "+
				"strings agree -- so the comparison below would pass on a constant that had "+
				"been emptied rather than renamed", p.what, p.w, p.l)
			continue
		}
		if string(p.w) != string(p.l) {
			t.Errorf("wire and ledger disagree on the %s source: wire %q, ledger %q. "+
				"internal/schedule casts one to the other with `ledger.Source(decl.Source)`, "+
				"so a run writes a fact the ledger refuses -- at the first observation of that "+
				"source, at RUNTIME and not at compile time", p.what, p.w, p.l)
		}
	}
}
