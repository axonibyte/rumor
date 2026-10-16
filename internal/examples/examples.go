// Package examples runs the worked example packages through the engine from
// their ledger files (R2.3, issue 44).
//
// WHY THIS IS A PACKAGE AND NOT ONLY A TEST. Issue 44 asks for the equality
// "under one command". tests/engine asserts it in the suite; this is the same
// walk behind `rumor check`, so the claim can be re-run by anyone with the
// binary and read without a Go toolchain. It is also the first consumer of
// internal/engine outside a test, which is what shows the package is usable
// rather than merely tested.
//
// AND IT DOES NOT SHARE ITS LOADER WITH tests/engine, deliberately. That
// harness globs and parses its own fixtures. If it called this package, the
// code under test would be choosing which cases the harness examines, and a
// walk that quietly skipped a case would make the harness agree with it.
package examples

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
	"forge.axonibyte.com/axonibyte/rumor/internal/ledger"
)

// Result is the tally one run produces. Cases is what it EXAMINED, which is
// reported beside the disagreements because zero disagreements over zero cases
// reads exactly like zero disagreements over seventeen.
type Result struct {
	Packages  int
	Cases     int
	Agreed    int
	Disagreed int
}

type caseDoc struct {
	Expect map[string]any `json:"expect"`
	Input  struct {
		Observation struct {
			Actor  string `json:"actor"`
			Of     string `json:"of"`
			Query  string `json:"query"`
			Source string `json:"source"`
			Value  any    `json:"value"`
			Role   string `json:"role"`
		} `json:"observation"`
	} `json:"input"`
}

type sealDoc struct {
	Seal int `json:"seal"`
}

// Run walks dir, evaluates every case from its ledger file and writes one line
// per case to out. It returns an error only for a tree it cannot read; a case
// that DISAGREES is a result and not an error, because the caller's exit code
// has to tell those apart.
func Run(ctx context.Context, dir string, out io.Writer) (Result, error) {
	var res Result
	entries, err := os.ReadDir(dir)
	if err != nil {
		return res, fmt.Errorf("%s is not readable, so nothing would be examined: %w", dir, err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, e.Name(), "statute.rego")); err == nil {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return res, fmt.Errorf("%s holds no package with a statute.rego: an empty walk is not a pass", dir)
	}

	for _, name := range names {
		pdir := filepath.Join(dir, name)
		statute, err := os.ReadFile(filepath.Join(pdir, "statute.rego"))
		if err != nil {
			return res, fmt.Errorf("%s: %w", name, err)
		}
		var rules []string
		raw, err := os.ReadFile(filepath.Join(pdir, "rules.json"))
		if err != nil {
			return res, fmt.Errorf("%s has a statute but no rules.json: %w", name, err)
		}
		if err := json.Unmarshal(raw, &rules); err != nil {
			return res, fmt.Errorf("%s/rules.json: %w", name, err)
		}
		if len(rules) == 0 {
			return res, fmt.Errorf("%s/rules.json names no rules, so no case in it can assert anything", name)
		}
		res.Packages++

		files, err := filepath.Glob(filepath.Join(pdir, "cases", "*.json"))
		if err != nil {
			return res, err
		}
		sort.Strings(files)
		if len(files) == 0 {
			return res, fmt.Errorf("%s has a statute and no cases", name)
		}
		for _, cf := range files {
			caseName := strings.TrimSuffix(filepath.Base(cf), ".json")
			res.Cases++
			line, agreed, err := one(ctx, pdir, name, caseName, string(statute), rules, cf)
			if err != nil {
				return res, err
			}
			if agreed {
				res.Agreed++
			} else {
				res.Disagreed++
			}
			if _, err := fmt.Fprintln(out, line); err != nil {
				return res, fmt.Errorf("writing the result for %s/%s: %w", name, caseName, err)
			}
		}
	}
	return res, nil
}

// one evaluates a single case from its ledger and renders the line for it.
func one(ctx context.Context, pdir, pkgName, caseName, statute string, rules []string, cf string) (string, bool, error) {
	var doc caseDoc
	if err := readJSON(cf, &doc); err != nil {
		return "", false, err
	}
	if len(doc.Expect) == 0 {
		return "", false, fmt.Errorf("%s/%s has no expect block, so it asserts nothing", pkgName, caseName)
	}
	var sd sealDoc
	if err := readJSON(filepath.Join(pdir, "ledger", caseName+".seal.json"), &sd); err != nil {
		return "", false, fmt.Errorf("%s/%s: %w", pkgName, caseName, err)
	}
	lraw, err := os.ReadFile(filepath.Join(pdir, "ledger", caseName+".jsonl"))
	if err != nil {
		return "", false, fmt.Errorf("case %s/%s has no ledger file: %w", pkgName, caseName, err)
	}
	l, err := ledger.Load(strings.NewReader(string(lraw)), false)
	if err != nil {
		return "", false, fmt.Errorf("%s/%s: the committed ledger does not load: %w", pkgName, caseName, err)
	}
	if err := l.SealAt(sd.Seal); err != nil {
		return "", false, fmt.Errorf("%s/%s: sealing at the recorded %d: %w", pkgName, caseName, sd.Seal, err)
	}
	o := doc.Input.Observation
	f, err := engine.Evaluate(ctx, statute, rules, l, engine.Check{
		Actor:  o.Actor,
		Query:  o.Query,
		Of:     o.Of,
		Source: ledger.Source(o.Source),
		Value:  o.Value,
		Role:   o.Role,
	})
	if err != nil {
		return "", false, fmt.Errorf("%s/%s: %w", pkgName, caseName, err)
	}

	bad := disagreements(doc.Expect, f.Rules)
	where := "no definition"
	if len(f.Fired) == 1 {
		where = f.Fired[0].String()
	} else if len(f.Fired) > 1 {
		where = fmt.Sprintf("%d definitions", len(f.Fired))
	}
	head := fmt.Sprintf("%-16s %-26s %-11v %-24s %4d trace event(s)",
		pkgName, caseName, f.Says(), where, f.TraceEvents)
	if len(bad) == 0 {
		return head + "  agrees", true, nil
	}
	return head + "  DISAGREES: " + strings.Join(bad, "; "), false, nil
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// disagreements lists the rules whose value differs from the case's expect.
// Compared through encoding/json in both directions so an integer from a file
// and a float from the evaluator do not read as a disagreement.
func disagreements(expect, got map[string]any) []string {
	keys := make([]string, 0, len(expect))
	for k := range expect {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var bad []string
	for _, rule := range keys {
		have, present := got[rule]
		if !present {
			bad = append(bad, fmt.Sprintf("%s undefined, wanted %v", rule, expect[rule]))
			continue
		}
		if !sameJSON(expect[rule], have) {
			bad = append(bad, fmt.Sprintf("%s got %v, wanted %v", rule, have, expect[rule]))
		}
	}
	return bad
}

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
