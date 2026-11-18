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

// TestEveryResolverLeadsWithTheToolchainGoModDeclares is the SEMANTIC half,
// and it exists because the structural one above cannot see this.
//
// WHAT THE OTHER ARM MISSES. It holds the bare name LAST, which stops a stock
// `go` shadowing the numbered ones. It says nothing about the order AMONG the
// numbered names -- so for nineteen days ci.yml resolved `go127` first and
// every run compiled under go1.27.4 while go.mod declared `go 1.26.0`, with
// this file green throughout. That was a deliberate remedy (GO-2026-6859 was
// unfixed in the go126 the jail had, ibis coop main/801), not an accident; but
// a remedy and a drift look identical to a test that only checks the tail.
//
// SO THE RULE IS DERIVED FROM go.mod RATHER THAN WRITTEN HERE. The arm reads
// the directive, turns `1.26.0` into the package name `go126`, and requires
// every resolver to try that name before any other numbered one. Nobody has to
// retype a literal when the directive moves: raise go.mod to 1.27 and this arm
// demands go127 leads, which is the other half of the choice cal was given
// (coop main/1016) and the shape that cannot go stale.
//
// IT IS NOT A PIN. A bare `go` still wins when the jail has one -- the
// operator's call, as ci.yml says -- and names for OTHER versions may follow in
// any order; what is fixed is that the declared toolchain is tried first.
func TestEveryResolverLeadsWithTheToolchainGoModDeclares(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "go.mod"))
	if err != nil {
		t.Fatalf("reading go.mod: %v", err)
	}
	m := regexp.MustCompile(`(?m)^go (\d+)\.(\d+)`).FindStringSubmatch(string(raw))
	if m == nil {
		t.Fatalf("go.mod states no `go <major>.<minor>` directive, so this arm has no rule to " +
			"derive and cannot say which toolchain CI should lead with")
	}
	want := "go" + m[1] + m[2] // 1.26 -> go126, the package name the jail installs
	numbered := regexp.MustCompile(`^go[0-9]+$|^gofmt[0-9]+$`)

	lines := ciLines(t)
	checked := 0
	for i := 0; i+1 < len(lines); i++ {
		loop := resolverLoop.FindStringSubmatch(lines[i])
		if loop == nil {
			continue
		}
		if resolverPick.FindStringSubmatch(lines[i+1]) == nil {
			continue // the version probe again: it picks nothing
		}
		names := strings.Fields(loop[1])
		// The gofmt resolvers carry gofmt126/gofmt127; the rule is the same one
		// keyed to the same directive, so the expected leader is prefixed to match.
		lead := want
		if strings.HasPrefix(names[0], "gofmt") {
			lead = "gofmt" + m[1] + m[2]
		}
		var first string
		for _, n := range names {
			if numbered.MatchString(n) {
				first = n
				break
			}
		}
		if first == "" {
			t.Errorf("ci.yml:%d resolves %v with no numbered name at all: the bare name is the "+
				"jail's choice of version, so this loop cannot honour go.mod's directive", i+1, names)
			continue
		}
		checked++
		if first != lead {
			t.Errorf("ci.yml:%d resolves %v, trying %q before %q. go.mod declares %s, so CI would "+
				"compile with a toolchain the module never asked for -- which is exactly what ran "+
				"for nineteen days with the bare-name arm green (coop main/1016)",
				i+1, names, first, lead, m[0])
		}
	}
	if checked == 0 {
		t.Fatal("no resolver with a numbered name was checked, so this arm proved nothing: " +
			"either ci.yml's loops changed shape or the regexes no longer read them")
	}
	proof.Logf(t, "PROOF go.mod declares %s and all %d resolver(s) in ci.yml lead with the "+
		"matching numbered toolchain", m[0], checked)
}

// TestEveryResolvedToolchainIsAlsoTheBareNameInItsStep is the third arm, and it
// exists because the other two check WHICH Go a step chooses and neither checks
// what the step's own `go` resolves to AFTERWARDS.
//
// RUN 1287 IS THE CASE. With go126 leading, the vulnerability step's $GO was
// go1.26.11 and govulncheck was rebuilt under it -- and govulncheck SHELLS OUT
// to the bare `go`, which on that runner is a symlink to go127. Its go1.26
// loader then met go1.27's `go list` and refused go1.27-tagged files in two
// dependencies. The gate BLOCKED rather than passing, which is the gate working;
// the defect was that one step saw two Gos. Both existing arms were green
// through it, because a chosen toolchain and an inherited PATH are different
// facts.
//
// SO THE RULE IS STRUCTURAL: wherever a step resolves the toolchain, it must
// also put that toolchain's own GOROOT/bin first on PATH, so the bare name in
// the step -- and in anything the step spawns -- is the Go the loop picked.
// fable ruled it for every step rather than for the one that failed (coop
// main/1019), so this arm reads every step and not just that one.
//
// GOROOT/bin IS THE ONLY DIRECTORY THAT WORKS, which is why the arm requires
// that spelling and not merely "some PATH export": /usr/local/bin carries
// go126, go127 AND the bare `go` symlink, so prepending the directory $GO lives
// in changes nothing at all.
func TestEveryResolvedToolchainIsAlsoTheBareNameInItsStep(t *testing.T) {
	lines := ciLines(t)
	// The resolution sites are the loops that pick, which the arms above
	// already identify; each must be followed -- before the step can run
	// anything -- by the export derived from that same $GO.
	// GO RESOLUTIONS ONLY, AND THE NARROWING IS EXACTLY ONE SITE WIDE. The
	// file also resolves gofmt into $FMT, and that site is excluded for a
	// stated reason rather than for convenience: GOROOT_BIN is derived from
	// `$GO env GOROOT`, gofmt has no `env` subcommand and no $GO exists in
	// that step, so there is nothing to derive from. It is also not the
	// hazard -- the gofmt step invokes "$FMT" explicitly and spawns nothing
	// that calls a Go, which is what run 1287's defect required. The first
	// arm in this file still holds that step's own order.
	resolutions, exports := 0, 0
	for i := 0; i+1 < len(lines); i++ {
		if resolverLoop.FindStringSubmatch(lines[i]) == nil {
			continue
		}
		pick := resolverPick.FindStringSubmatch(lines[i+1])
		if pick == nil {
			continue // the version probe: it picks nothing and runs nothing
		}
		if pick[1] != "GO" {
			continue // the gofmt resolver, excluded for the reason above
		}
		resolutions++
	}
	for _, l := range lines {
		if strings.Contains(l, `export PATH="$GOROOT_BIN:$PATH"`) {
			exports++
		}
	}
	if resolutions == 0 {
		t.Fatal("no resolver that picks a toolchain was found, so this arm proved nothing: " +
			"ci.yml's loops changed shape or the regexes no longer read them")
	}
	if exports != resolutions {
		t.Errorf("ci.yml resolves a toolchain at %d site(s) but exports its GOROOT/bin onto PATH "+
			"%d time(s). A step that resolves $GO and leaves PATH alone still has the jail's bare "+
			"`go` -- a symlink to another version on this runner -- so anything it SPAWNS compiles "+
			"with a Go the loop did not choose. That is run 1287: govulncheck rebuilt under "+
			"go1.26.11 shelling out to go1.27.4's `go list` (coop main/1018, ruled main/1019)",
			resolutions, exports)
	}
	// AND THE DERIVATION IS CHECKED, NOT JUST THE COUNT. An export of a
	// hard-coded directory would satisfy the count and pin the file to one
	// jail's layout; it has to come from the chosen toolchain itself.
	const derive = `GOROOT_BIN="$(timeout 30 "$GO" env GOROOT)/bin"`
	if got := strings.Count(strings.Join(lines, "\n"), derive); got != resolutions {
		t.Errorf("%d site(s) derive GOROOT_BIN from the chosen $GO, want %d: a literal path would "+
			"satisfy the export count while pinning this file to one jail's layout", got, resolutions)
	}
	proof.Logf(t, "PROOF all %d toolchain resolution(s) in ci.yml put the chosen toolchain's own "+
		"GOROOT/bin first on PATH, derived from that $GO, so no step sees two Gos", resolutions)
}
