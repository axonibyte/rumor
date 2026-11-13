// Package proof is the one place a PROOF line is written, and the only place
// that decides whether it may be.
//
// WHY IT EXISTS (fable's ruling, coop r9/251; 124's deliverable 6). This tree
// says what an arm actually drove by logging a line beginning PROOF -- which
// seed, which profile, how many steps, which finding -- and ci.yml MINES those
// lines out of test.log so the record carries evidence rather than a bare
// "ok". 221 of those lines were written with a plain t.Logf, which Go prints
// for a FAILING test as readily as a passing one. rook found the shape (coop
// r9/249): on a mutant that deleted a battery's invocation, the arm failed and
// printed "PROOF ... each mentioned by a workflow" beside its own error.
//
// A CLAIM THAT CAN PRINT FROM A FAILING TEST IS NOT A PROOF, whatever guards
// the miner, which is why the fix is here and not on the miner's step
// condition. ibis bounded the exposure at the condition -- ci.yml carries no
// `if:` or `always()`, so the miner never runs after a red step -- and quill
// measured what defends that bound and found nothing: tools/check-workflows.py
// bounds tool TIMEOUTS and has no opinion on step conditions, so an `always()`
// added next month would unbound 198 sites silently, in a diff that reads as an
// improvement (coop r9/250). Fixing the source leaves that condition FREE:
// once a PROOF cannot lie, diagnostics on red are only diagnostics.
package proof

import (
	"strings"
	"testing"
)

// Logf records what an arm drove, and only when the arm has not failed.
//
// IT TAKES testing.TB so a subtest's own *testing.T is what answers Failed():
// a cell that failed stops claiming, while its siblings still report.
//
// AND IT REFUSES A FORMAT THAT DOES NOT BEGIN WITH THE WORD, because the word
// is what ci.yml mines: a line that drifts to "Proof" or loses the prefix stops
// reaching the record silently, and silence is the failure this package exists
// to end. The refusal is Fatalf rather than a log, since a format is written
// once by hand and a wrong one is a defect in the arm rather than in the run.
func Logf(t testing.TB, format string, args ...any) {
	t.Helper()
	if !strings.HasPrefix(format, "PROOF") {
		t.Fatalf("proof.Logf's format must begin with PROOF, got %q", format)
		// AND THE RETURN IS NOT DEAD CODE, which its own cell found. On a real
		// *testing.T, Fatalf never comes back -- it stops the goroutine -- so
		// this line looks redundant to a reader. It is not: the parameter is
		// testing.TB, an INTERFACE, and an implementation whose Fatalf records
		// and returns would fall straight through to the Logf below and write
		// the very line the refusal just rejected. Relying on a control-flow
		// side effect of one implementation is an unstated assumption; this
		// makes the refusal true of every TB.
		return
	}
	if t.Failed() {
		return
	}
	t.Logf(format, args...)
}
