package hygiene

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/proof"
)

// THE TOOLCHAIN RESOLVERS IN ci.yml PICK THE FIRST NAME THAT IS ON PATH, so the
// ORDER of the names is the whole decision. 7e2d6a0 taught them go127 and wrote
// `for c in go go127 go126 go125`, and run 719 still compiled with go1.26.10:
// the runner's bare `go` IS go1.26.10, it is found first, and go127 is never
// reached. Nothing caught it, because nothing in the tree read the resolvers at
// all (rook, coop rumor/1979). These two arms are that oracle.
//
// A RESOLVER IS KEYED ON ITS GRAMMAR, NOT ON THE WORD "go". ci.yml also loops
// over every Go name to PRINT each one's version -- a probe that assigns
// nothing and breaks on nothing, where order changes only the print order. A
// search for `for c in go` counts that probe as a resolver, which is how "ten
// places" was counted for nine. The pick is the loop whose body assigns GO or
// FMT and breaks.
var (
	resolverLoop = regexp.MustCompile(`^\s*for c in ([a-z0-9 ]+); do$`)
	resolverPick = regexp.MustCompile(`\{ (GO|FMT)="\$c"; break; \}`)
	anyPick      = regexp.MustCompile(`(GO|FMT)="\$c"`)
)

func ciLines(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", ".forgejo", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("reading ci.yml: %v", err)
	}
	return strings.Split(string(raw), "\n")
}

// TestEveryToolchainResolverTriesTheBareNameLast is the structural half: over
// EVERY resolver in the file, the unnumbered name comes last. Before any
// numbered name, it shadows every newer toolchain the runner carries.
func TestEveryToolchainResolverTriesTheBareNameLast(t *testing.T) {
	lines := ciLines(t)
	sites, assignments := 0, 0
	for _, l := range lines {
		assignments += len(anyPick.FindAllString(l, -1))
	}
	for i := 0; i+1 < len(lines); i++ {
		m := resolverLoop.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		p := resolverPick.FindStringSubmatch(lines[i+1])
		if p == nil {
			continue // the version probe: it picks nothing, so its order decides nothing
		}
		sites++
		bare := map[string]string{"GO": "go", "FMT": "gofmt"}[p[1]]
		names := strings.Fields(m[1])
		for j, n := range names {
			if n == bare && j != len(names)-1 {
				t.Errorf("ci.yml:%d resolves %s as %v, with bare %q at position %d of %d: "+
					"the first name on PATH wins, so a bare name ahead of the numbered ones "+
					"shadows every newer toolchain the runner has -- run 719 compiled with "+
					"go1.26.10 while go1.27.4 was installed", i+1, p[1], names, bare, j+1, len(names))
			}
		}
	}
	// THE VACUITY GUARD IS A SELF-CONSISTENCY CHECK, NOT A COUNT TO KEEP. An
	// exact "9 and 1" would rot the first time anyone added a resolver, and the
	// usual answer to a rotted number is to edit it. Instead every assignment of
	// GO or FMT from $c in the file must sit in a loop this arm recognised: two
	// counts derived from the same file, so a resolver written in a shape the
	// regexes miss -- which would let this arm pass on nothing -- is a failure.
	if sites == 0 || sites != assignments {
		t.Errorf("recognised %d resolver loop(s) but the file assigns GO or FMT from $c %d "+
			"time(s): a resolver this arm cannot read is one whose order it cannot check",
			sites, assignments)
	}
	// THE PROOF LINE IS GATED ON THE VERDICT. Printed unconditionally it said
	// "every one of the file's 10 GO/FMT picks recognised" on the very run where
	// the guard above had just found 9 -- and ci.yml MINES PROOF lines out of
	// test.log, so a failing arm would have published a passing claim.
	if !t.Failed() {
		proof.Logf(t, "PROOF %d resolver(s) in ci.yml, every one trying its bare name last, and "+
			"every one of the file's %d GO/FMT picks recognised", sites, assignments)
	}
}

// TestAResolverPicksTheNumberedToolchainOverABareOldOne is the behavioural half,
// so the structural rule above is shown to MEAN what it claims: the first GO
// resolver is cut out of ci.yml and RUN, under a PATH holding stub toolchains.
func TestAResolverPicksTheNumberedToolchainOverABareOldOne(t *testing.T) {
	lines := ciLines(t)
	var loop []string
	for i := 0; i+1 < len(lines) && loop == nil; i++ {
		if resolverLoop.MatchString(lines[i]) && strings.Contains(lines[i+1], `{ GO="$c"; break; }`) {
			for j := i; j < len(lines); j++ {
				loop = append(loop, lines[j])
				if strings.TrimSpace(lines[j]) == "done" {
					break
				}
			}
		}
	}
	if loop == nil {
		t.Fatal("no GO resolver found in ci.yml, so there is nothing to drive")
	}

	run := func(t *testing.T, stubs map[string]string) string {
		t.Helper()
		dir := t.TempDir()
		for name, version := range stubs {
			body := "#!/bin/sh\necho \"go version " + version + " freebsd/amd64\"\n"
			if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		script := strings.Join(loop, "\n") + "\necho \"PICKED=$GO\"\n"
		cmd := exec.Command("/bin/sh", "-c", script)
		cmd.Env = []string{"PATH=" + dir}
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("running the extracted resolver: %v\n%s", err, out)
		}
		return strings.TrimSpace(string(out))
	}

	// run 719's runner exactly: an old bare `go` beside a newer go127.
	t.Run("an old bare go beside go127", func(t *testing.T) {
		if got := run(t, map[string]string{"go": "go1.26.10", "go127": "go1.27.4"}); got != "PICKED=go127" {
			t.Errorf("got %q, want PICKED=go127: the resolver chose the bare go1.26.10 "+
				"over an installed go1.27.4", got)
		}
	})
	// AND THE BARE NAME IS STILL THE FALLBACK. Moving it last must not lose it: a
	// box carrying only an unnumbered `go` still gets a toolchain.
	t.Run("a bare go alone", func(t *testing.T) {
		if got := run(t, map[string]string{"go": "go1.27.4"}); got != "PICKED=go" {
			t.Errorf("got %q, want PICKED=go: a box with only a bare go was left with "+
				"no toolchain", got)
		}
	})
}

// TestEveryListOfGoToolchainNamesInTheTreePutsTheBareNameLast is the arm the
// two above should have been from the start. They read ci.yml ALONE, and that
// was a narrowing nobody stated: the same bare-first order survived in
// tag-pipeline.yml (the RELEASE build, which did not even know go127), in three
// docs/cli.md run blocks the release gate executes, in release-gate.py's
// docs-stale arm, in d4control.py, in two Go suites that enumerate the build,
// and in docsrun_test.go's hint, which PRESCRIBED the old order to whoever
// read the failure (rook, coop rumor/2002). Every one of them was a resolver
// whose first name wins, in four languages.
//
// SO THIS ARM KEYS ON THE NAMES, NOT ON A LANGUAGE'S GRAMMAR. Any run of two or
// more Go toolchain names -- `go`, or `go` and three digits -- joined only by
// spaces, commas, quotes and brackets is a list of toolchains, whatever syntax
// holds it; and in every such list the bare name comes last or not at all. A
// presence probe, whose order decides nothing, is held to the same rule rather
// than exempted: an exemption is a judgement the arm would have to make, and
// reordering a probe costs nothing.
var toolchainRun = regexp.MustCompile(`\bgo(?:1[0-9]{2})?\b(?:[\s,"'()\[\]{}` + "`" + `]+\bgo(?:1[0-9]{2})?\b)+`)
var toolchainName = regexp.MustCompile(`\bgo(?:1[0-9]{2})?\b`)

// treeListExempt is EXACTLY what this arm does not read, each with its reason.
//   - this file: it quotes the defective order in prose, and its fixtures must.
//   - docs/testing-methodology.md's one line: a case study recording what the
//     workflows resolved AT THE TIME. Rewriting it to today's order would make
//     the history false; it is exempt by its exact text, so a NEW occurrence
//     anywhere in that document is still read.
//   - CHANGELOG.md's one line: a RELEASED entry, which describes what that
//     version shipped. Same reason, same exactness.
var treeListExempt = map[string]string{
	"tests/hygiene/toolchain_resolver_test.go": "",
	"docs/testing-methodology.md":              "settled this — resolve `go go126 go125`, then",
	"CHANGELOG.md":                             "sites and `tag-pipeline.yml` at two: resolve the first of `go`, `go126`,",
}

func TestEveryListOfGoToolchainNamesInTheTreePutsTheBareNameLast(t *testing.T) {
	root := filepath.Join("..", "..")
	out, err := exec.Command("git", "-C", root, "ls-files").Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	lists, files := 0, 0
	for _, rel := range strings.Fields(string(out)) {
		exemptLine, exempt := treeListExempt[rel]
		if exempt && exemptLine == "" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil || strings.ContainsRune(string(raw[:min(len(raw), 8000)]), 0) {
			continue // unreadable or binary: no list of names lives there
		}
		files++
		for n, line := range strings.Split(string(raw), "\n") {
			if exempt && strings.Contains(line, exemptLine) {
				continue
			}
			for _, run := range toolchainRun.FindAllString(line, -1) {
				names := toolchainName.FindAllString(run, -1)
				numbered := 0
				for _, nm := range names {
					if nm != "go" {
						numbered++
					}
				}
				if numbered == 0 {
					continue // "go go" in prose is not a list of toolchains
				}
				lists++
				for j, nm := range names {
					if nm == "go" && j != len(names)-1 {
						t.Errorf("%s:%d lists Go toolchains as %v with the bare name at position %d of %d: "+
							"wherever such a list is a resolver the first name on PATH wins, so a bare `go` "+
							"ahead of the numbered ones shadows every newer toolchain the box carries",
							rel, n+1, names, j+1, len(names))
					}
				}
			}
		}
	}
	if lists == 0 {
		t.Fatalf("read %d file(s) and found no list of Go toolchain names at all: the arm's pattern "+
			"has stopped matching the tree, and a pass here would be a pass on nothing", files)
	}
	if !t.Failed() {
		proof.Logf(t, "PROOF %d list(s) of Go toolchain names across %d tracked file(s), every one with the bare name last",
			lists, files)
	}
}
