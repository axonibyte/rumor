package hygiene

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
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
		t.Logf("PROOF %d resolver(s) in ci.yml, every one trying its bare name last, and "+
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
