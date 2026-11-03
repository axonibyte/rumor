package coverage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scratch builds an empty workspace with a docs/ directory, cleaned up by the
// test framework rather than by hand.
func scratch(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	if err := os.MkdirAll(filepath.Join(d, "docs"), 0o755); err != nil {
		t.Fatalf("scratch: %v", err)
	}
	return d
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// repoRoot finds the workspace root from this package's directory.
//
// It ASSERTS the marker it is looking for rather than returning a path that
// merely exists. A wrong root would make committedTreeIsFullyMapped pass or
// fail for a reason that has nothing to do with the tree, which is the exact
// shape of a test agreeing with itself.
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("repoRoot: %v", err)
	}
	marker := filepath.Join(root, "docs", "coverage.toml")
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("repoRoot resolved to %s, which has no docs/coverage.toml: %v", root, err)
	}
	return root
}

// The arm issue 2 asks for: the committed tree passes.
func TestCommittedTreeIsFullyMapped(t *testing.T) {
	n, err := Check(repoRoot(t))
	if err != nil {
		t.Fatalf("the committed tree must be fully mapped: %v", err)
	}
	if n <= 20 {
		t.Fatalf("expected the whole workspace, counted %d", n)
	}
}

// ISSUE 6: `.git` IS A FILE IN A WORKTREE, AND THE WALK MUST SKIP IT ANYWAY.
//
// THE FIXTURE IS THE SHAPE AND NOT A MOCK OF IT. `skipped` is consulted on a
// NAME, so an arm that passed a directory called `.git` would exercise the
// branch the defect was never in. Here `.git` is a regular FILE holding the
// `gitdir:` pointer a worktree carries, which is the only spelling that reached
// the entry branch and demanded a docs/coverage.toml line.
//
// WHY A HAND-BUILT TREE RATHER THAN `git worktree add`: this package's tests
// take no dependency on a git binary, and the defect needs nothing from git
// except the one fact that `.git` can be a file. Asserting that fact directly
// keeps the arm honest on a box with no git at all -- the PROVISIONING finding
// ci.yml names at :137 -- and the live reproduction is recorded in the commit
// message rather than demanded of every run.
//
// MUTATION-CHECKED: with `skipped(name)` back inside the isDir branch this
// fails with ".git matches no entry", which is the message the real worktree
// produced at 0da475f.
func TestAGitFileIsSkippedAsAGitDirectoryIs(t *testing.T) {
	d := scratch(t)
	write(t, filepath.Join(d, "docs", "coverage.toml"),
		"[[entry]]\npath = \"docs/*\"\ndocs = [\"docs/release.md\"]\n")
	// A WORKTREE'S .git, byte for byte in shape: a file, not a directory.
	write(t, filepath.Join(d, ".git"), "gitdir: /somewhere/.git/worktrees/w\n")

	n, err := Check(d)
	if err != nil {
		t.Fatalf("a worktree's .git FILE must be skipped exactly as a clone's .git "+
			"DIRECTORY is -- the exemption is the name, not the type: %v", err)
	}
	if n < 1 {
		t.Fatalf("checked %d paths, so the walk found nothing and the arm above "+
			"could have passed on an empty tree", n)
	}
	// THE CONTROL, WITHOUT WHICH THE ARM PROVES ONLY THAT Check RETURNED NIL:
	// the same tree with an unmapped file of another name must still FAIL, so
	// the skip is specific to `.git` and not a walk that stopped early.
	write(t, filepath.Join(d, "stray.txt"), "s\n")
	if _, err := Check(d); err == nil {
		t.Fatal("an unmapped stray.txt passed, so the walk is not checking entries at all")
	} else if !strings.Contains(err.Error(), "stray.txt") {
		t.Fatalf("the refusal names %v, not stray.txt", err)
	}
}

// ROOK'S OBSERVATION at coop r5/180, taken rather than left: hoisting `skipped(name)`
// out of the isDir branch exempts by NAME, so a FILE called `target` is now skipped
// exactly as a DIRECTORY called `target` was. The hoist's comment states that intent
// ("the walk skips only .git and target, with no condition on the type"), which is why
// rook ruled the reason covers what it widens -- but NO CELL EXERCISED IT, and a
// comment is an untested claim whoever wrote it.
//
// SO THIS ASSERTS THE SENTENCE. If the exemption should ever narrow to `.git` alone,
// this arm is what fails and says so, rather than the widening going out silently.
//
// MUTATION-CHECKED: narrow the hoist to `name == ".git"` and this fails with
// "target matches no entry in docs/coverage.toml".
func TestATargetFileIsSkippedAsATargetDirectoryIs(t *testing.T) {
	d := scratch(t)
	write(t, filepath.Join(d, "docs", "coverage.toml"),
		"[[entry]]\npath = \"docs/*\"\ndocs = [\"docs/release.md\"]\n")
	// A FILE, not a directory, and not a plausible build output -- the point is the
	// TYPE, so the content is irrelevant and saying so keeps the fixture honest.
	write(t, filepath.Join(d, "target"), "not a build directory\n")

	n, err := Check(d)
	if err != nil {
		t.Fatalf("the exemption is the NAME and not the type, which is what the hoist's "+
			"own comment claims: %v", err)
	}
	if n < 1 {
		t.Fatalf("checked %d paths, so the walk found nothing and the arm above could "+
			"have passed on an empty tree", n)
	}
	// THE CONTROL, so this proves an exemption and not a walk that stops at any file:
	// a differently-named file in the same tree must still be demanded of the map.
	write(t, filepath.Join(d, "targets"), "one letter off\n")
	if _, err := Check(d); err == nil {
		t.Fatal("a file named `targets` passed, so the skip is not matching whole names")
	} else if !strings.Contains(err.Error(), "targets") {
		t.Fatalf("the refusal names %v, not targets", err)
	}
}

// The other arm issue 2 asks for: an unmapped path fails, by name.
func TestAnUnmappedPathFailsAndIsNamed(t *testing.T) {
	d := scratch(t)
	write(t, filepath.Join(d, "docs", "coverage.toml"),
		"[[entry]]\npath = \"docs/*.toml\"\ndocs = [\"docs/release.md\"]\n")
	write(t, filepath.Join(d, "stowaway.rs"), "// mapped by nothing\n")
	_, err := Check(d)
	if err == nil {
		t.Fatal("an unmapped path must fail the check")
	}
	if !strings.Contains(err.Error(), "stowaway.rs") {
		t.Fatalf("the failure must name it: %v", err)
	}
}

// A check that cannot fail is not a check: prove the positive arm moves.
func TestTheSameTreePassesOnceThePathIsMapped(t *testing.T) {
	d := scratch(t)
	write(t, filepath.Join(d, "docs", "coverage.toml"),
		"[[entry]]\npath = \"docs/*.toml\"\ndocs = [\"docs/release.md\"]\n"+
			"\n[[entry]]\npath = \"*.rs\"\ndocs = [\"docs/cli.md\"]\n")
	write(t, filepath.Join(d, "stowaway.rs"), "// now mapped\n")
	n, err := Check(d)
	if err != nil {
		t.Fatalf("the mapped tree must pass: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 paths, counted %d", n)
	}
}

// quill's finding in r0.1/4: without this the check is one-directional and a
// mistyped glob is invisible as soon as two entries overlap.
func TestADeadEntryFailsAndIsNamed(t *testing.T) {
	d := scratch(t)
	write(t, filepath.Join(d, "docs", "coverage.toml"),
		"[[entry]]\npath = \"docs/*.toml\"\ndocs = [\"docs/release.md\"]\n"+
			"\n[[entry]]\npath = \"docs/*.tml\"\ndocs = [\"docs/release.md\"]\n")
	_, err := Check(d)
	if err == nil {
		t.Fatal("a dead entry must fail the check")
	}
	if !strings.Contains(err.Error(), "docs/*.tml") || !strings.Contains(err.Error(), "dead") {
		t.Fatalf("the failure must name the dead glob: %v", err)
	}
}

// And the pair that stops the arm above from passing for the wrong reason:
// the same tree with the typo corrected must pass, so "fails" is about the
// dead entry and not about the tree.
func TestTheSameTreePassesOnceTheTypoIsCorrected(t *testing.T) {
	d := scratch(t)
	write(t, filepath.Join(d, "docs", "coverage.toml"),
		"[[entry]]\npath = \"docs/*.toml\"\ndocs = [\"docs/release.md\"]\n")
	n, err := Check(d)
	if err != nil {
		t.Fatalf("the corrected tree must pass: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 path, counted %d", n)
	}
}

func TestParserIsStrict(t *testing.T) {
	if _, err := Parse("[[entry]]\npath = \"a\"\ndocs = [\"d\"]\n"); err != nil {
		t.Fatalf("a well-formed entry must parse: %v", err)
	}
	for _, c := range []struct{ bad, why string }{
		{"path = \"a\"\n", "a key outside any entry"},
		{"[[entry]]\ndocs = [\"d\"]\n", "no path key"},
		{"[[entry]]\npath = \"a\"\n", "no docs key"},
		{"[[entry]]\npath = \"a\"\ndocs = []\n", "an empty docs list"},
		{"[[entry]]\npath = a\ndocs = [\"d\"]\n", "an unquoted path"},
		{"[[entry]]\npath = \"a\"\npth = \"b\"\ndocs = [\"d\"]\n", "a typo'd key"},
		{"[[entry]]\npath = \"a\"\npath = \"b\"\ndocs = [\"d\"]\n", "a duplicate key"},
		{"# only a comment\n", "no entries at all"},
	} {
		if _, err := Parse(c.bad); err == nil {
			t.Errorf("must reject %s: %q", c.why, c.bad)
		}
	}
}

func TestGlobsMatchBySegment(t *testing.T) {
	for _, c := range []struct{ pat, path string }{
		{"Cargo.toml", "Cargo.toml"},
		{"docs/*.md", "docs/design.md"},
		{"crates/*/Cargo.toml", "crates/rumor-cli/Cargo.toml"},
		{"crates/rumor-cli/src/**/*.rs", "crates/rumor-cli/src/main.rs"},
		{"crates/rumor-cli/src/**/*.rs", "crates/rumor-cli/src/a/b/c.rs"},
		{"**/*.md", "README.md"},
		{"*.rs", "stowaway.rs"},
		// quill r0.1/4: oneSegment's middle branch needs two * in one segment,
		// which no live entry has, so it was unreachable and untested. These
		// two reach it.
		{"docs/*cover*.toml", "docs/coverage.toml"},
		{"*a*e.md", "agent-guide.md"},
	} {
		if !GlobMatch(c.pat, c.path) {
			t.Errorf("%s should match %s", c.pat, c.path)
		}
	}
	for _, c := range []struct{ pat, path string }{
		{"docs/*.md", "docs/adr/README.md"}, // * is one segment
		{"docs/*.md", "docs/coverage.toml"},
		{"Cargo.toml", "crates/rumor-cli/Cargo.toml"},
		{"crates/*/Cargo.toml", "crates/a/b/Cargo.toml"},
		{"*.rs", "src/main.rs"},
		{"docs/*cover*.toml", "docs/design.md"},
		{"*a*e.md", "README.md"},
	} {
		if GlobMatch(c.pat, c.path) {
			t.Errorf("%s should NOT match %s", c.pat, c.path)
		}
	}
}

// walkSkips is skipDirs, DUPLICATED ON PURPOSE AND NOT READ FROM IT. A test
// that imported the production list would agree with it however that list
// changed, which is the re-export shape: the duplication is the check. So this
// is not asserted equal to skipDirs anywhere -- the BEHAVIOUR is what decides,
// and if the two disagree the subtests below fail rather than this map.
var walkSkips = map[string]bool{".git": true, "target": true}

// artefactFor turns a .gitignore entry into a concrete path to create.
//
// It REFUSES a shape it cannot realise rather than skipping it. A silent skip
// would let a new .gitignore entry go uncovered while this test still reported
// a pass -- the defect being that a checker which decides for itself what
// counts as "not applicable" hides exactly the cases nobody has thought about.
func artefactFor(entry string) (string, bool) {
	e := strings.TrimPrefix(entry, "/")
	switch {
	case e == "":
		return "", false
	case strings.HasSuffix(e, "/"):
		// A directory entry: the walk descends into it, so the path it can
		// name is a FILE inside. An empty directory contributes nothing and
		// would make this cell pass for the wrong reason.
		return strings.TrimSuffix(e, "/") + "/stowaway.bin", true
	case strings.Count(e, "*") == 1:
		return strings.Replace(e, "*", "stowaway", 1), true
	case !strings.ContainsAny(e, "*?[]!"):
		return e, true
	}
	return "", false
}

// THE DECISION, not the walk: being listed in .gitignore is NOT an exemption
// from `docs --check`, and only the walk's own skip list is.
//
// fable placed this at r7/105 and the reason it had no cell is that the walk
// DID have one. .gitignore's own comment carries the ruling, written as a
// correction after run 142 went red on `rumor-bin`: the entry "stops the
// artefact being COMMITTED. It does NOT stop `docs --check` failing on it,
// because the walk in internal/coverage never reads .gitignore". The
// protection is building out-of-tree with -o, which ci.yml and the release do.
//
// WHAT THIS SEES THAT THE CELLS ABOVE CANNOT, measured rather than asserted --
// and the measurement corrected this paragraph's first draft, which claimed the
// wrong cure left every other cell green. It does not, and the reason is luck:
//
//	skipDirs += "rumor", "__pycache__"   kills this cell AND
//	                                     TestCommittedTreeIsFullyMapped
//	skipDirs += "__pycache__" alone      kills THIS CELL ONLY
//	skipped() += a ".pyc" suffix test    kills THIS CELL ONLY
//
// The first is caught elsewhere only because `rumor` collides with the real
// directory cmd/rumor: skipping that name makes the map's `cmd/rumor/*.go`
// entry DEAD, and the dead-entry direction refuses. Run 142's artefact was
// actually named `rumor-bin`, which collides with nothing -- so the accident
// that catches one entry would not have caught the artefact that started this.
// `__pycache__` and the `*.pyc` shape collide with nothing committed, and no
// other cell in this package can see either.
//
// The cure was proposed TWICE: derive the walk's skips from .gitignore. ratchet
// put it at r7/103 after reading the file with `grep -vE '^#|^$'`, which strips
// the comments from the one file whose comment is the ruling.
//
// AND IT IS DRIVEN FROM THE REAL .gitignore, not a copy of its contents, so an
// entry added later is covered without anyone remembering this test. Three of
// us have now hit the refusal on an artefact we had created seconds earlier --
// rook, ibis, and ratchet at rumor/1763 ("the first run's coverage FAIL was MY
// root binary, built with no -o") -- and all three removed the artefact, which
// is the INTENDED response. Self-blame reaching the right act is still an
// unarmed decision.
func TestAGitignoredArtefactIsStillWalkedAndNamed(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), ".gitignore"))
	if err != nil {
		t.Fatalf("the ruling lives in .gitignore and it could not be read: %v", err)
	}
	var entries []string
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		entries = append(entries, line)
	}
	if len(entries) == 0 {
		t.Fatal(".gitignore yielded no entries, so this test would pass over an " +
			"empty corpus -- a vacuous positive, not a clean one")
	}

	refused, skipped := 0, 0
	for _, entry := range entries {
		rel, ok := artefactFor(entry)
		if !ok {
			t.Fatalf("this test cannot realise the .gitignore entry %q as a path. "+
				"Teach artefactFor the shape rather than leaving the entry "+
				"uncovered", entry)
		}
		t.Run(entry, func(t *testing.T) {
			d := scratch(t)
			write(t, filepath.Join(d, "docs", "coverage.toml"),
				"[[entry]]\npath = \"docs/*.toml\"\ndocs = [\"docs/release.md\"]\n"+
					"\n[[entry]]\npath = \"src/**\"\ndocs = [\"docs/design.md\"]\n")
			if err := os.MkdirAll(filepath.Join(d, "src"), 0o755); err != nil {
				t.Fatalf("fixture: %v", err)
			}
			write(t, filepath.Join(d, "src", "one.go"), "package one\n")
			// THE CLEAN TREE PASSES FIRST. Without this the refusal below
			// could be the fixture's own fault and the cell would read as
			// proving the decision.
			if _, err := Check(d); err != nil {
				t.Fatalf("the fixture must pass before the artefact is added: %v", err)
			}

			path := filepath.Join(d, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatalf("fixture: %v", err)
			}
			write(t, path, "an artefact nobody mapped\n")

			_, err := Check(d)
			if walkSkips[strings.Split(rel, "/")[0]] {
				if err != nil {
					t.Fatalf("%s is under the walk's own skip list, so it must NOT "+
						"be demanded of the map: %v", rel, err)
				}
				skipped++
				return
			}
			if err == nil {
				t.Fatalf("%q is in .gitignore but that is not an exemption: %s must "+
					"be refused by the check", entry, rel)
			}
			if !strings.Contains(err.Error(), rel) {
				t.Fatalf("the refusal must NAME the artefact %s, or a reader cannot "+
					"act on it: %v", rel, err)
			}
			refused++
		})
	}
	// BOTH HALVES OR NEITHER MEANS ANYTHING. A corpus that happened to be all
	// skips would report a pass having proved nothing about the decision, and
	// one that was all refusals would not have exercised the skip list at all.
	if refused == 0 || skipped == 0 {
		t.Fatalf("the corpus exercised %d refusal(s) and %d skip(s); both halves "+
			"are needed or this says nothing about which list exempts a path",
			refused, skipped)
	}
}
