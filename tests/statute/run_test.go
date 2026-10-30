// Package statute_test is the harness that walks examples/ and refuses a
// surviving mutant.
//
// G3, issue 34. It replaces the Rust harness at crates/rumor-cli/tests/. The
// three packages under examples/ were carried from r1.2-examples BYTE FOR BYTE
// -- the tree object was the same on both sides at the port -- and
// TestStatutesHaveNotDrifted below pins each statute and rules file by content
// hash so a later edit cannot pass unnoticed.
//
// ONE OF THOSE PINS HAS SINCE MOVED, deliberately: planted-lie/statute.rego
// carried three sentences that had gone false, and v0.1.8 corrected them and
// re-pinned the hash in the same commit. So the pins are THIS PROJECT's record
// of its own statutes, not a claim of identity with a swept branch. The
// identity at the port is recorded in CHANGELOG.md 0.1.4, where it belongs.
//
// WHAT "RED" MEANS HERE, because it is the whole point of the mutants: a mutant
// is red when at least one case's observed rule values DISAGREE with that case's
// `expect`. A mutant that no case can distinguish is a clause the examples do
// not actually exercise, and this harness fails naming it rather than reporting
// a green suite over an untested clause.
package statute_test

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/cover"
	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/open-policy-agent/opa/v1/topdown"
	"github.com/open-policy-agent/opa/v1/topdown/print"
)

// examplesDir is relative to this test's own directory, which `go test` makes
// the working directory. Asserted rather than assumed: a wrong root would make
// every table below empty, and an empty table is a suite that passes by
// examining nothing.
const examplesDir = "../../examples"

// statutePackage is the Rego package every statute declares. Duplicated here
// deliberately rather than parsed out of the module: a harness that read the
// package name from the file it is testing would agree with that file whatever
// it said.
const statutePackage = "data.rumor.statute"

type pkg struct {
	name    string // the directory name, e.g. "planted-lie"
	dir     string
	statute string   // statute.rego source
	rules   []string // the rule names rules.json asks for, in file order
	cases   []kase
	mutants []string // paths, sorted
}

type kase struct {
	name   string
	input  any
	expect map[string]any
}

func load(t *testing.T) []pkg {
	t.Helper()
	entries, err := os.ReadDir(examplesDir)
	if err != nil {
		t.Fatalf("%s is not readable, so this suite would examine nothing: %v", examplesDir, err)
	}
	var out []pkg
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(examplesDir, e.Name())
		src, err := os.ReadFile(filepath.Join(dir, "statute.rego"))
		if err != nil {
			continue // not a statute package
		}
		p := pkg{name: e.Name(), dir: dir, statute: string(src)}

		rulesRaw, err := os.ReadFile(filepath.Join(dir, "rules.json"))
		if err != nil {
			t.Fatalf("%s has a statute but no rules.json", p.name)
		}
		if err := json.Unmarshal(rulesRaw, &p.rules); err != nil {
			t.Fatalf("%s/rules.json: %v", p.name, err)
		}
		if len(p.rules) == 0 {
			t.Fatalf("%s/rules.json names no rules, so no case in it can assert anything", p.name)
		}

		caseFiles, _ := filepath.Glob(filepath.Join(dir, "cases", "*.json"))
		sort.Strings(caseFiles)
		for _, cf := range caseFiles {
			raw, err := os.ReadFile(cf)
			if err != nil {
				t.Fatalf("%s: %v", cf, err)
			}
			var doc struct {
				Expect map[string]any `json:"expect"`
				Input  any            `json:"input"`
			}
			if err := json.Unmarshal(raw, &doc); err != nil {
				t.Fatalf("%s: %v", cf, err)
			}
			if len(doc.Expect) == 0 {
				t.Fatalf("%s has no expect block, so it asserts nothing", cf)
			}
			p.cases = append(p.cases, kase{
				name:   strings.TrimSuffix(filepath.Base(cf), ".json"),
				input:  doc.Input,
				expect: doc.Expect,
			})
		}

		p.mutants, _ = filepath.Glob(filepath.Join(dir, "mutants", "*.rego"))
		sort.Strings(p.mutants)
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// evalRules returns each named rule's value for one input, with absence
// recorded as a missing key. OPA reports an undefined rule as an EMPTY result
// set -- ratchet go-port/3 measured that, against Regorus's `{}` -- so "the
// rule did not hold" and "the rule holds and its value is empty" are different
// readings and this function keeps them apart.
func evalRules(ctx context.Context, module string, rules []string, input any,
	opts ...func(*rego.Rego)) (map[string]any, error) {
	got := map[string]any{}
	for _, rule := range rules {
		args := append([]func(*rego.Rego){
			rego.Query(statutePackage + "." + rule),
			rego.Module("statute.rego", module),
			rego.Input(input),
		}, opts...)
		rs, err := rego.New(args...).Eval(ctx)
		if err != nil {
			return nil, fmt.Errorf("rule %s: %w", rule, err)
		}
		if len(rs) == 0 || len(rs[0].Expressions) == 0 {
			continue // undefined: absent from the map
		}
		got[rule] = rs[0].Expressions[0].Value
	}
	return got, nil
}

// sameJSON compares through encoding/json in both directions so an int from a
// case file and a float from the evaluator do not read as a disagreement. The
// comparison that matters is the VALUE, not its Go type.
func sameJSON(a, b any) bool {
	ja, err1 := json.Marshal(a)
	jb, err2 := json.Marshal(b)
	if err1 != nil || err2 != nil {
		return false
	}
	var ua, ub any
	if json.Unmarshal(ja, &ua) != nil || json.Unmarshal(jb, &ub) != nil {
		return false
	}
	return reflect.DeepEqual(ua, ub)
}

// disagreements lists the rules whose observed value differs from expect.
func disagreements(expect, got map[string]any) []string {
	var bad []string
	for _, rule := range sortedKeys(expect) {
		want := expect[rule]
		have, present := got[rule]
		if !present {
			bad = append(bad, fmt.Sprintf("%s: undefined, wanted %v", rule, want))
			continue
		}
		if !sameJSON(want, have) {
			bad = append(bad, fmt.Sprintf("%s: got %v, wanted %v", rule, have, want))
		}
	}
	return bad
}

func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestTheSuiteExaminesSomething(t *testing.T) {
	// ibis main/352's rule, applied to this harness: a refusal count of zero
	// reads identically whether every mutant died or none ran. So the sizes are
	// asserted before any verdict is reported.
	pkgs := load(t)
	if len(pkgs) != 5 {
		t.Fatalf("expected 5 statute packages, found %d: %v", len(pkgs), names(pkgs))
	}
	totalCases, totalMutants := 0, 0
	for _, p := range pkgs {
		if len(p.cases) == 0 {
			t.Errorf("%s has no cases", p.name)
		}
		if len(p.mutants) == 0 {
			t.Errorf("%s has no mutants, so nothing proves its clauses matter", p.name)
		}
		totalCases += len(p.cases)
		totalMutants += len(p.mutants)
	}
	if totalCases != 34 {
		t.Errorf("expected 34 cases across the five packages, counted %d", totalCases)
	}
	// 19, NOT 20, and the shortfall is documented rather than accidental: I
	// declared 20 from a guess and this assertion caught it. planted-lie has
	// six because `m2-ignores-knowledge` was REMOVED -- its own
	// mutants/README.md records that no case in that example can kill it, so it
	// was moved to transitive-count (which carries it as m2) rather than left to
	// survive. 6 + 7 + 6. A surviving mutant and a missing one look the same in
	// a total, which is why the number is declared and the gap is named.
	if totalMutants != 38 {
		t.Errorf("expected 38 mutants across the five packages, counted %d", totalMutants)
	}
	t.Logf("PROOF examined %d package(s), %d case(s), %d mutant(s)",
		len(pkgs), totalCases, totalMutants)
}

func names(pkgs []pkg) []string {
	out := make([]string, len(pkgs))
	for i, p := range pkgs {
		out[i] = p.name
	}
	return out
}

func TestEveryCaseIsGreen(t *testing.T) {
	ctx := context.Background()
	for _, p := range load(t) {
		for _, c := range p.cases {
			t.Run(p.name+"/"+c.name, func(t *testing.T) {
				got, err := evalRules(ctx, p.statute, p.rules, c.input)
				if err != nil {
					t.Fatalf("evaluating the statute: %v", err)
				}
				if bad := disagreements(c.expect, got); len(bad) > 0 {
					t.Fatalf("the clean statute disagrees with this case:\n  %s",
						strings.Join(bad, "\n  "))
				}
			})
		}
	}
}

func TestEveryMutantIsRed(t *testing.T) {
	ctx := context.Background()
	for _, p := range load(t) {
		for _, mutantPath := range p.mutants {
			t.Run(p.name+"/"+strings.TrimSuffix(filepath.Base(mutantPath), ".rego"), func(t *testing.T) {
				src, err := os.ReadFile(mutantPath)
				if err != nil {
					t.Fatalf("%s: %v", mutantPath, err)
				}
				var killedBy []string
				for _, c := range p.cases {
					got, err := evalRules(ctx, string(src), p.rules, c.input)
					if err != nil {
						// A mutant that does not COMPILE is red for the wrong
						// reason: it proves nothing about the clause it removes.
						// Named rather than counted as a kill.
						t.Fatalf("the mutant did not evaluate, which is not the same as being caught: %v", err)
					}
					if bad := disagreements(c.expect, got); len(bad) > 0 {
						killedBy = append(killedBy, c.name)
					}
				}
				if len(killedBy) == 0 {
					t.Fatalf("THIS MUTANT SURVIVED: no case distinguishes it from the clean statute, so the clause it breaks is not exercised by any case in %s", p.name)
				}
				t.Logf("PROOF killed by %d of %d case(s): %s",
					len(killedBy), len(p.cases), strings.Join(killedBy, ", "))
			})
		}
	}
}

func TestCoverageAndTracePerCase(t *testing.T) {
	ctx := context.Background()
	for _, p := range load(t) {
		module, err := ast.ParseModule("statute.rego", p.statute)
		if err != nil {
			t.Fatalf("%s/statute.rego does not parse: %v", p.name, err)
		}
		for _, c := range p.cases {
			t.Run(p.name+"/"+c.name, func(t *testing.T) {
				cov := cover.New()
				tracer := topdown.NewBufferTracer()
				// The FINDING rule, because that is the one a report is about:
				// coverage and a trace for a sub-rule would not say how the
				// verdict was reached.
				if _, err := evalRules(ctx, p.statute, []string{"finding"}, c.input,
					rego.QueryTracer(cov), rego.QueryTracer(tracer)); err != nil {
					t.Fatalf("evaluating with instruments attached: %v", err)
				}

				report := cov.Report(map[string]*ast.Module{"statute.rego": module})
				file, ok := report.Files["statute.rego"]
				if !ok {
					t.Fatal("the coverage report names no file, so it says nothing about this statute")
				}
				if len(file.Covered) == 0 {
					t.Fatal("the coverage report shows no covered range, which cannot be true of a statute that just produced a verdict")
				}

				var pretty bytes.Buffer
				topdown.PrettyTrace(&pretty, *tracer)
				if pretty.Len() == 0 {
					t.Fatal("the trace is empty, so nothing re-derives this finding")
				}
				// A value only execution could have produced: the trace must
				// mention the rule it evaluated.
				if !strings.Contains(pretty.String(), "finding") {
					t.Fatalf("the trace never mentions the rule it evaluated, so it is not a trace of this query:\n%s",
						firstLines(pretty.String(), 5))
				}
				t.Logf("PROOF %d trace event(s), %d covered range(s), coverage %.0f%%",
					len(*tracer), len(file.Covered), file.Coverage)
			})
		}
	}
}

func firstLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

// TestStatutesHaveNotDrifted pins the carried files by content hash.
//
// The hashes are git blob ids. Five were measured at r1.2-examples 9d58c61,
// whose examples/ tree was byte-identical to f031c05 -- the commit quill and I
// both grepped for wren's criterion A. The sixth, planted-lie/statute.rego,
// was re-pinned in v0.1.8 and is this project's own. Computing the hash HERE
// rather than asking git keeps the check independent of the repository's own
// view, and pinning it as a test rather than reporting it once means an edit
// to a statute cannot pass UNNOTICED -- which is the property, not that no
// edit may ever happen.
func TestStatutesHaveNotDrifted(t *testing.T) {
	// RE-PINNED FOR planted-lie/statute.rego, DELIBERATELY AND IN THE SAME
	// COMMIT AS THE EDIT (fable, thread:go-port/61). The carried statute
	// carried three sentences that had gone false: ADR 0001 named by its old
	// thread, a harness described in Regorus's API, and R2's flattening said
	// to be in Rust. Byte-identity with r1.2-examples was built to prove the
	// PORT changed no statute; it did, that branch is swept, and three false
	// sentences in a live fixture cost more than a property whose subject is
	// gone. This test still does its job -- it catches an UNNOTICED edit, and
	// it fired on this one before the hash was moved.
	want := map[string]string{
		"planted-lie/rules.json":        "15b99687cd84307297dbc5b6a8e8e1e806421d3d",
		"planted-lie/statute.rego":      "ed76c3023950a9cbf350f7d0280c1560bf6fc030",
		"role-visibility/rules.json":    "30fd254c10dd7704c1bc1d9f80b63dcc1bf740c4",
		"role-visibility/statute.rego":  "3414a549dc1accebdda42b5a691398a4ffa3980e",
		"transitive-count/rules.json":   "36a71c9896065fb64e390c89c807331d5eb83e02",
		"transitive-count/statute.rego": "d5577fc488765ab82245210e3b172dae64321a88",
	}
	checked := 0
	for rel, hash := range want {
		body, err := os.ReadFile(filepath.Join(examplesDir, rel))
		if err != nil {
			t.Errorf("%s: %v", rel, err)
			continue
		}
		// The git blob header, then the bytes. Built as ONE buffer and hashed
		// in one call so there is no write error to discard: `fmt.Fprintf`
		// into a hash returns an error that can never be non-nil, and errcheck
		// is right to refuse the judgement call -- the lint is cheaper to
		// satisfy than to argue with, and sha1.Sum is clearer anyway.
		blob := append([]byte(fmt.Sprintf("blob %d\x00", len(body))), body...)
		got := fmt.Sprintf("%x", sha1.Sum(blob))
		if got != hash {
			t.Errorf("%s has drifted from r1.2-examples: hash %s, carried %s", rel, got, hash)
		}
		checked++
	}
	if checked != len(want) {
		t.Fatalf("checked %d of %d pinned files; a file this test cannot read is not a file it verified", checked, len(want))
	}
	t.Logf("PROOF %d statute and rules file(s) match their pinned blob ids", checked)
}

// ---------------------------------------------------------------------------
// ADR 0001: no `print()` in a statute.
//
// Carried from the Rust harness, which used regorus's `set_gather_prints(true)`
// and a `take_prints()` that had to come back empty. The Go evaluator needs the
// same two halves and one more precaution: OPA ERASES print calls unless
// `rego.EnablePrintStatements(true)` is passed, so a hook installed without
// that flag reports zero prints for a statute that is full of them. That is the
// `docs-stale` defect exactly -- a check whose silence means nothing -- so the
// flag's necessity is measured below rather than trusted.
//
// TWO ORACLES, because neither is sufficient:
//
//   - STATIC, over the parsed module: finds a print on a path no case reaches.
//     The runtime hook cannot see those, and an unexercised print is still a
//     statute writing to a log.
//   - RUNTIME, through the evaluator the cases already use: proves the hook is
//     wired and the flag is on. The static scan cannot prove that.
//
// Both run over every mutant as well as every statute, because a mutant is a
// statute too.

// printRecorder implements print.Hook. Not guarded by a mutex: rego.Eval runs
// the query on the calling goroutine, and the tests below pass one recorder to
// one Eval at a time.
type printRecorder struct {
	seen []string
}

func (p *printRecorder) Print(ctx print.Context, msg string) error {
	where := "unknown location"
	if ctx.Location != nil {
		where = fmt.Sprintf("%s:%d", ctx.Location.File, ctx.Location.Row)
	}
	p.seen = append(p.seen, fmt.Sprintf("%s: %s", where, msg))
	return nil
}

// staticPrints lists the `print(...)` calls in a module, by line.
//
// The builtin's name comes from the library rather than from a literal here:
// ast.Print is the declaration the evaluator dispatches on, so reading its name
// cannot disagree with the evaluator about what a print call is. That the name
// is still the expected word is asserted once, in the self-test below.
func staticPrints(name, src string) ([]string, error) {
	mod, err := ast.ParseModule(name, src)
	if err != nil {
		return nil, err
	}
	var hits []string
	ast.WalkExprs(mod, func(e *ast.Expr) bool {
		if e.IsCall() {
			if op := e.Operator(); op != nil && op.String() == ast.Print.Name {
				row := 0
				if e.Location != nil {
					row = e.Location.Row
				}
				hits = append(hits, fmt.Sprintf("%s:%d", name, row))
			}
		}
		return false
	})
	return hits, nil
}

func TestNoStatuteCallsPrint(t *testing.T) {
	ctx := context.Background()
	pkgs := load(t)
	if len(pkgs) == 0 {
		t.Fatal("no statute packages: an empty walk is not a pass")
	}

	modulesScanned, evaluations, printsFound := 0, 0, 0
	for _, p := range pkgs {
		// every statute AND every mutant, statically
		sources := map[string]string{filepath.Join(p.name, "statute.rego"): p.statute}
		for _, m := range p.mutants {
			body, err := os.ReadFile(m)
			if err != nil {
				t.Errorf("%s: %v", m, err)
				continue
			}
			sources[filepath.Join(p.name, "mutants", filepath.Base(m))] = string(body)
		}
		for rel, src := range sources {
			hits, err := staticPrints(rel, src)
			if err != nil {
				t.Errorf("%s: could not parse: %v", rel, err)
				continue
			}
			modulesScanned++
			if len(hits) != 0 {
				printsFound += len(hits)
				t.Errorf("ADR 0001: %s calls print() at %v", rel, hits)
			}
		}

		// and at runtime, over every case, with print statements ENABLED
		for _, c := range p.cases {
			rec := &printRecorder{}
			_, err := evalRules(ctx, p.statute, p.rules, c.input,
				rego.EnablePrintStatements(true), rego.PrintHook(rec))
			if err != nil {
				t.Errorf("%s/%s: %v", p.name, c.name, err)
				continue
			}
			evaluations++
			if len(rec.seen) != 0 {
				printsFound += len(rec.seen)
				t.Errorf("ADR 0001: %s/%s printed %d time(s): %v",
					p.name, c.name, len(rec.seen), rec.seen)
			}
		}
	}

	// COUNT WHAT WAS EXAMINED, not only what was refused: zero findings reads
	// identically whether every module was clean or none was opened.
	if modulesScanned != 43 {
		t.Errorf("scanned %d modules, expected 43 (5 statutes + 38 mutants); a count that drifts means a file stopped being examined", modulesScanned)
	}
	if evaluations != 34 {
		t.Errorf("evaluated %d cases under the print hook, expected 34", evaluations)
	}
	// The proof line states the count it MEASURED. Printing "0 prints"
	// unconditionally put a contradiction one line under the finding when this
	// was driven with a print planted in a real mutant.
	if printsFound != 0 {
		t.Logf("ADR 0001 REFUSED: %d print call(s) across %d module(s) and %d evaluation(s)",
			printsFound, modulesScanned, evaluations)
	} else {
		t.Logf("PROOF ADR 0001 checked on %d module(s) statically and %d case evaluation(s) at runtime, 0 prints",
			modulesScanned, evaluations)
	}
}

// TestThePrintCheckFires drives both oracles with a statute that DOES print.
//
// Without this, TestNoStatuteCallsPrint is indistinguishable from a test that
// examines nothing -- and the third case below is the one that matters most: it
// measures that the hook is silent when the flag is off, which is why the flag
// is passed explicitly above rather than left to a default.
func TestThePrintCheckFires(t *testing.T) {
	if ast.Print.Name != "print" {
		t.Fatalf("the builtin this check looks for is named %q, not \"print\": the scan is looking for the wrong thing", ast.Print.Name)
	}

	const noisy = `package rumor.statute

finding := "consistent" if {
	print("reached")
}
`
	const quiet = `package rumor.statute

finding := "consistent" if {
	input.claims == []
}
`
	ctx := context.Background()

	hits, err := staticPrints("noisy.rego", noisy)
	if err != nil {
		t.Fatalf("noisy.rego did not parse: %v", err)
	}
	if len(hits) != 1 {
		t.Errorf("static scan found %d print(s) in a module with one: %v", len(hits), hits)
	} else {
		t.Logf("ok   static scan names the call site: %v", hits)
	}

	if clean, err := staticPrints("quiet.rego", quiet); err != nil {
		t.Fatalf("quiet.rego did not parse: %v", err)
	} else if len(clean) != 0 {
		t.Errorf("static scan flagged %v in a module that calls no print: the scan matches something else", clean)
	}

	rec := &printRecorder{}
	if _, err := evalRules(ctx, noisy, []string{"finding"}, map[string]any{"claims": []any{}},
		rego.EnablePrintStatements(true), rego.PrintHook(rec)); err != nil {
		t.Fatalf("evaluating noisy.rego: %v", err)
	}
	if len(rec.seen) == 0 {
		t.Error("the print hook recorded nothing from a statute that prints: the runtime half of the check cannot fail")
	} else {
		t.Logf("ok   runtime hook captured: %v", rec.seen)
	}

	// THE PRECAUTION, MEASURED. Same module, same hook, flag omitted.
	off := &printRecorder{}
	if _, err := evalRules(ctx, noisy, []string{"finding"}, map[string]any{"claims": []any{}},
		rego.PrintHook(off)); err != nil {
		t.Fatalf("evaluating noisy.rego with print statements disabled: %v", err)
	}
	if len(off.seen) != 0 {
		t.Logf("NOTE this OPA build delivers prints without EnablePrintStatements: %v", off.seen)
	} else {
		t.Logf("PROOF the hook alone is silent (%d captured) -- EnablePrintStatements(true) is load-bearing, not decoration", len(off.seen))
	}
}

// knownMultiHunkMutants are the mutants whose code diff against their statute
// is NOT one contiguous hunk, with the count measured 2026-10-26.
//
// THEY ARE DECLARED AND NOT EXCLUDED, which is the difference between a
// narrowing with a reason and a filter. Each is a hand-cut mutant in an R1-R3
// package that is not mine to re-cut, and none has been shown to NEED more than
// one hunk -- so the entry records what is, not what is right. The count is
// pinned in BOTH directions: repair one and this test fails because its count
// moved, which is the signal to delete its row rather than to re-measure it.
var knownMultiHunkMutants = map[string]int{
	"planted-lie/m1-ignores-position.rego":      2,
	"role-visibility/m1-role-ignored.rego":      3,
	"role-visibility/m3-ignores-position.rego":  3,
	"transitive-count/m1-ignores-position.rego": 3,
}

// TestEveryMutantIsItsStatutePlusOneChange is the invariant every mutant's own
// header already claims, and which nothing checked.
//
// WHAT IT CATCHES, measured rather than imagined. Every mutant carries the
// sentence "One change against <statute> and nothing else, which is what makes
// `rumor attaint`'s kill attributable to the rule it names". On 2026-10-26 NINE
// of examples/toy's ten were false: each had been cut from the statute as it
// stood and never re-cut, so they lacked rules the statute had since gained --
// all nine missing `role_sees_subject if not input.observation.role`, five of
// them missing the whole precondition block.
//
// A MUTANT MISSING A RULE KILLS IT BY ABSENCE, which is not the deviation the
// mutant is supposed to assert, and attaint said so in its own output:
//
//	toy  has_user          killed by m1-believed-ignores-visibility
//	toy  may_create_user   killed by m1-believed-ignores-visibility
//	toy  may_leave         killed by m1-believed-ignores-visibility
//	... eight rules in all, seven of which have mutants of their own
//
// "10 rule(s) killed, 0 unmutated" was GREEN on that, so the figure measured a
// corpus that could not say which mutation killed what. After the re-cut each
// rule is killed by its own mutant and the totals are unchanged -- which is the
// point: the number was never going to move, only its meaning.
//
// COMMENTS ARE STRIPPED BEFORE COMPARING, deliberately. A mutant's header is
// its own prose and its body carries the statute's; neither is the mutation,
// and a comment that drifts is a different (and lesser) defect than a rule that
// does. This test is about the rules.
func TestEveryMutantIsItsStatutePlusOneChange(t *testing.T) {
	pkgs, err := filepath.Glob(filepath.Join("..", "..", "examples", "*", "mutants"))
	if err != nil || len(pkgs) == 0 {
		t.Fatalf("no mutant directories found, so this test asserts nothing: %v %v", pkgs, err)
	}
	sort.Strings(pkgs)

	seen, checked := map[string]bool{}, 0
	for _, dir := range pkgs {
		pkg := filepath.Base(filepath.Dir(dir))
		statute := stripComments(t, filepath.Join(filepath.Dir(dir), "statute.rego"))
		files, gerr := filepath.Glob(filepath.Join(dir, "*.rego"))
		if gerr != nil || len(files) == 0 {
			t.Fatalf("%s has no mutants: %v %v", pkg, files, gerr)
		}
		sort.Strings(files)
		for _, f := range files {
			key := pkg + "/" + filepath.Base(f)
			hunks := countHunks(statute, stripComments(t, f))
			checked++
			want, declared := knownMultiHunkMutants[key]
			if declared {
				seen[key] = true
				if hunks != want {
					t.Errorf("%s has %d hunk(s) and is declared at %d: if it was repaired, "+
						"delete its row; if it grew, it is drifting further", key, hunks, want)
				}
				continue
			}
			if hunks != 1 {
				t.Errorf("%s changes %d hunk(s) of its statute, want 1: its own header says "+
					"one change and nothing else, and attaint's kill attribution rests on "+
					"that -- a mutant missing a rule kills it by ABSENCE", key, hunks)
			}
		}
	}
	for key := range knownMultiHunkMutants {
		if !seen[key] {
			t.Errorf("%s is declared multi-hunk and no mutant of that name was found: a "+
				"declared exception for a file that is gone exempts nothing", key)
		}
	}
	if checked < 30 {
		t.Fatalf("only %d mutant(s) compared: the walk is broken, not the corpus", checked)
	}
	t.Logf("PROOF %d mutant(s) compared across %d package(s), %d declared multi-hunk",
		checked, len(pkgs), len(knownMultiHunkMutants))
}

// stripComments returns a rego file's code lines: no comments, no blank lines.
func stripComments(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var out []string
	for _, line := range strings.Split(string(raw), "\n") {
		s := strings.TrimSpace(line)
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

// countHunks is the number of contiguous regions in which a and b differ.
//
// IT IS A LONGEST-COMMON-SUBSEQUENCE COUNT AND NOT A LINE COUNT, because a
// one-line change and a five-line change are both ONE change -- m4 replaces a
// four-line rule body with a one-line one and that is a single deviation. What
// is NOT one change is a second region somewhere else in the file, which is
// exactly the shape nine stale mutants had.
func countHunks(a, b []string) int {
	n, m := len(a), len(b)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	hunks, i, j, inHunk := 0, 0, 0, false
	for i < n && j < m {
		if a[i] == b[j] {
			i, j, inHunk = i+1, j+1, false
			continue
		}
		if !inHunk {
			hunks++
			inHunk = true
		}
		if lcs[i+1][j] >= lcs[i][j+1] {
			i++
		} else {
			j++
		}
	}
	if i < n || j < m {
		if !inHunk {
			hunks++
		}
	}
	return hunks
}
