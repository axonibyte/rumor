// Package hygiene_test holds tree-wide checks that no single package can make
// about itself. Its first arm is issue 88's class: a cleanup that cannot run.
package hygiene_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// root is this package's distance from the tree's top. The walk starts there
// rather than at "." so the check covers cmd/, internal/ and tests/ alike --
// issue 88's own prose said "four TestMains" and missed a fifth MkdirTemp under
// internal/ (ratchet, coop rumor/946). A check scoped to one directory would
// have reproduced that blind spot exactly.
const root = "../.."

// exempt names the sites where a defer sits beside an os.Exit ON PURPOSE,
// each with the reason it is safe. An undeclared site fails this arm; a
// declared one that no longer exists fails it too, because an exemption whose
// subject has been renamed stops covering anything and says nothing when it
// does (the shape of every stale declaration this room found this week).
var exempt = map[string]string{
	"cmd/toy-adapter/main.go:main": "every os.Exit path here calls a.close() by " +
		"hand and the code says why; quill fixed that leak at coop r5/102 and " +
		"cmd/toy-adapter/main_test.go drives the failing-open path",
}

// TestNoCleanupIsStrandedByOsExit finds the defect of issue 88 by its SHAPE.
//
// A deferred call does not run when the goroutine leaves through os.Exit. So a
// function that does both -- defers something and exits the process -- has a
// cleanup that never happens, and the compiler, go vet and golangci-lint v2.14
// all say nothing about it: vet was clean on this tree at 7dfa641 with the four
// leaking TestMains in it, and CI's lint gate was green on the same commit.
// That is why this arm is hand-written rather than a linter's config line.
//
// WHAT IT DOES NOT PROVE: that temp directories are removed. It proves no
// cleanup is stranded by os.Exit. The world directory in toyrestart leaked
// with no defer and no os.Exit anywhere near it, and TestCloseRemovesTheWorld
// is what covers that half.
func TestNoCleanupIsStrandedByOsExit(t *testing.T) {
	var findings []string
	files := 0
	seen := map[string]bool{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// vendor and the module cache are other people's code; .git is not go.
			switch d.Name() {
			case ".git", "vendor", "node_modules":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if perr != nil {
			return perr
		}
		files++
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			var defers []*ast.DeferStmt
			var exits []*ast.CallExpr
			// THE SCOPE IS THE FUNCTION'S OWN BODY, not its closures: `die :=
			// func(code int) { ...; os.Exit(code) }` is the FIX, and counting
			// its os.Exit as the enclosing function's would flag every repair.
			for _, stmt := range fn.Body.List {
				ast.Inspect(stmt, func(n ast.Node) bool {
					if _, isFunc := n.(*ast.FuncLit); isFunc {
						return false
					}
					switch v := n.(type) {
					case *ast.DeferStmt:
						defers = append(defers, v)
					case *ast.CallExpr:
						if sel, isSel := v.Fun.(*ast.SelectorExpr); isSel {
							if id, isID := sel.X.(*ast.Ident); isID &&
								id.Name == "os" && sel.Sel.Name == "Exit" {
								exits = append(exits, v)
							}
						}
					}
					return true
				})
			}
			if len(defers) > 0 && len(exits) > 0 {
				rel := filepath.ToSlash(strings.TrimPrefix(
					filepath.Clean(path), filepath.Clean(root)+string(filepath.Separator)))
				key := rel + ":" + fn.Name.Name
				if _, ok := exempt[key]; ok {
					seen[key] = true
					continue
				}
				findings = append(findings, fset.Position(fn.Pos()).String()+
					" "+fn.Name.Name+": defers "+itoa(len(defers))+
					" call(s) and exits the process "+itoa(len(exits))+" time(s)")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	for _, f := range findings {
		t.Errorf("a deferred cleanup cannot run here: %s", f)
	}
	// THE CONTROL: the walk must have READ something. An empty corpus would
	// report zero findings and pass, which is the shape of every vacuous
	// check in this room's week.
	if files == 0 {
		t.Fatalf("the walk parsed NO .go files under %s, so zero findings means "+
			"nothing -- fix the walk before reading this arm's verdict", root)
	}
	t.Logf("%d .go file(s) parsed under %s", files, root)
	// AND EVERY EXEMPTION MUST STILL HAVE A SUBJECT.
	for key, why := range exempt {
		if !seen[key] {
			t.Errorf("exempt site %s was not found in the tree, so the exemption "+
				"covers nothing: %s", key, why)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
