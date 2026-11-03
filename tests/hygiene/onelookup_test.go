// Package hygiene's ONE-LOOKUP-SITE arm: in the ENGINE's own packages,
// `internal/bindings.ZoneNamed` is the only place that resolves an IANA zone
// name, and until this file existed that was a comment.
//
// WHY A SOURCE ARM AND NOT A CELL, which is ibis's finding at coop rumor/1774
// against 9f43356. A SECOND lookup site is EQUIVALENT, not merely unkilled: a
// duplicate `time.LoadLocation` answers exactly what ZoneNamed answers for
// every name, so no input distinguishes the two and no test can reach the
// difference. ibis drove it -- "a SECOND site in zoneOf, guard kept, import
// dropped: SURVIVED" -- and named the instrument this file is.
//
// WHAT THE CLAIM IS FOR. `LoadLocation("")` SUCCEEDS with a real UTC and a
// FAILED lookup answers a nil *Location whose `.String()` is also "UTC", so
// three different projects -- one declaring UTC, one declaring nothing, and one
// whose name no database holds -- read identically through a raw lookup. One
// site is what keeps the empty-name decision and the error path in a single
// place, where they are tested; a second site is where one of them goes
// missing.
//
// cmd/toy IS EXCLUDED AND THAT IS THE SCOPE, not an exemption. The toy is the
// SYSTEM UNDER TEST: it reads its own instants in the zone its `-zone` flag
// names, which is its configuration and deliberately NOT the engine's
// declaration (fable's ruling at coop r7/114) -- the two disagreeing is a
// thing a run can find. A system under test importing the engine's bindings
// package to resolve its own zone would make that disagreement impossible,
// which is the opposite of what this project wants. My first writing of this
// arm claimed the whole repository and the arm itself said otherwise: three
// sites, two of them prose and one of them the toy's.
//
// IT PARSES RATHER THAN GREPS, for the other half of that: a text scan counts
// the needle inside COMMENTS, and the comments above and in internal/bindings
// quote `time.LoadLocation` to explain why one site exists. go/parser sees
// calls.
//
// AND IT RESOLVES THE IMPORT'S LOCAL NAME rather than matching the word
// `time`, which is ibis's limit at coop rumor/1782: this tree aliases stdlib
// imports (`cryptorand "crypto/rand"`), so a second site written `t "time"`
// ... `t.LoadLocation(name)` would have survived an arm keyed on the literal.
//
// WHAT THIS DOES NOT PROVE: that the site BEHAVES, which
// internal/bindings.TestZoneNamedIsNilForNoneAndErrorsForUnknown drives, nor
// that a caller cannot read a zone some other way -- `time.Time.In` is not a
// lookup. It proves the engine holds exactly one resolver, which is the claim
// the comments make.
package hygiene_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheEnginesPackagesHoldOneZoneLookupSite(t *testing.T) {
	const root = "../.."
	const home = "internal/bindings/bindings.go"

	var scanned int
	found := map[string]int{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, werr error) error {
		if werr != nil {
			return werr
		}
		if info.IsDir() {
			switch info.Name() {
			// scratchpad HOLDS EXTRACTIONS of other revisions, whose copies of
			// this module would each count as sites.
			case ".git", "scratchpad", "target":
				return filepath.SkipDir
			// AND THE SYSTEM UNDER TEST, for the reason this file's header
			// gives: its zone is its own configuration.
			case "toy":
				if filepath.Base(filepath.Dir(path)) == "cmd" {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return perr
		}
		scanned++
		rel, _ := filepath.Rel(root, path)
		// THE LOCAL NAME "time" IS BOUND TO, not the literal word (ibis's
		// limit at coop rumor/1782). A file may alias a stdlib import, and
		// this tree does -- `cryptorand "crypto/rand"` in internal/schedule --
		// so an arm matching `time.LoadLocation` by its text would miss a
		// second site written as `t "time"` ... `t.LoadLocation(name)`. The
		// import specs say which identifier names the package in THIS file,
		// and a dot-import binds no identifier at all, which is reported
		// rather than skipped: it would make every bare `LoadLocation(` a
		// candidate and this arm cannot see those.
		local := ""
		for _, spec := range f.Imports {
			if spec.Path == nil || spec.Path.Value != `"time"` {
				continue
			}
			switch {
			case spec.Name == nil:
				local = "time"
			case spec.Name.Name == ".":
				t.Errorf("%s dot-imports \"time\", so a call to LoadLocation there carries no "+
					"package name and this arm cannot see it", rel)
			case spec.Name.Name == "_":
				// A BLANK IMPORT CALLS NOTHING, which is how cmd/rumor and
				// cmd/toy carry `time/tzdata`.
			default:
				local = spec.Name.Name
			}
		}
		if local == "" {
			// A FILE THAT DOES NOT IMPORT `time` CANNOT CALL INTO IT, so there
			// is nothing here to find and the walk moves on.
			return nil
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "LoadLocation" {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != local {
				return true
			}
			found[filepath.ToSlash(rel)]++
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v -- the engine's surface is UNMEASURED rather than clean", root, err)
	}
	// A DENOMINATOR FIRST: a walk that found nothing and a walk that failed
	// short both report one lookup site, and only one of them is a measurement.
	if scanned < 40 {
		t.Fatalf("parsed %d non-test .go file(s), far short of this module's source: a short "+
			"walk reports one lookup site exactly as a correct tree would", scanned)
	}
	if len(found) != 1 || found[home] != 1 {
		t.Errorf("`time.LoadLocation` is CALLED in %v across %d parsed file(s), want exactly "+
			"once in %s: a second resolver answers what the first does for every name, so no "+
			"cell can tell them apart and the empty-name decision can go missing from one",
			found, scanned, home)
		return
	}
	t.Logf("PROOF `time.LoadLocation` is called once in the engine's %d non-test .go file(s), "+
		"in %s; cmd/toy is out of scope as the system under test", scanned, home)
}
