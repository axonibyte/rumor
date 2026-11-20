package explain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/decl"
	"forge.axonibyte.com/axonibyte/rumor/internal/engine"
	"forge.axonibyte.com/axonibyte/rumor/internal/ledger"
)

// --- the harness ----------------------------------------------------------
//
// IT LOADS THE EXAMPLES ITSELF AND DOES NOT CALL internal/examples, for the
// reason that package states about tests/engine: a harness that asks the code
// under test which cases to examine lets a walk that quietly skipped a case
// make the harness agree with it. The loader here is twenty lines and the
// glob is its own.

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

type loaded struct {
	pkg     string
	name    string
	statute string
	rules   []string
	ledger  *ledger.Ledger
	check   engine.Check
	finding engine.Finding
}

func examplesDir() string { return filepath.Join("..", "..", "examples") }

// packages lists every example package, which is every directory under
// examples/ with a statute.rego -- not a list written here, so a sixth package
// is examined the day it lands rather than the day someone remembers it.
func packages(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(examplesDir())
	if err != nil {
		t.Fatalf("examples/ is not readable, so nothing would be examined: %v", err)
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(examplesDir(), e.Name(), "statute.rego")); err == nil {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	if len(out) == 0 {
		t.Fatal("examples/ holds no package with a statute.rego: an empty walk is not a pass")
	}

	// AND THE SET IS CHECKED AGAINST THE PACKAGES THAT EXISTED AT THE CUT,
	// which auto-discovery alone cannot do. A package DIRECTORY that vanishes
	// is simply not listed, and the case-count floor in the arm below cannot
	// see it: MEASURED with examples/twin moved aside, the walk reported
	// "built 42 derivation(s) across 4 example package(s)" and PASSED, because
	// 42 is over the floor of 25. ibis drew the distinction this closes (coop
	// r11/17): an absent examples/ catches a vacuous walk, and one absent
	// package catches a walk that skips a package it should have seen. They are
	// two checks and the phase wants both.
	//
	// A NEW PACKAGE IS STILL WELCOME AND WALKED THE DAY IT LANDS -- the list
	// below is a floor and not the walk's input, so nothing here has to be
	// edited to examine a sixth. Only a LOST one is red.
	for _, want := range []string{"planted-lie", "role-visibility", "toy", "transitive-count", "twin"} {
		found := false
		for _, got := range out {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("examples/%s has a statute.rego at R11 d1's cut and is not in this walk (%v): a package that disappears is a deletion nobody meant or a rename nobody finished, and it cannot be left to the case count, which passes at 42 of 47",
				want, out)
		}
	}
	return out
}

func cases(t *testing.T, pkg string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(examplesDir(), pkg, "cases", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	if len(files) == 0 {
		t.Fatalf("%s has a statute and no cases", pkg)
	}
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, strings.TrimSuffix(filepath.Base(f), ".json"))
	}
	return out
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

// load evaluates one committed case through the engine, which is the only way
// to get a finding whose trace and input are the ones a run would produce.
func load(t *testing.T, pkg, name string) loaded {
	t.Helper()
	pdir := filepath.Join(examplesDir(), pkg)
	statute, err := os.ReadFile(filepath.Join(pdir, "statute.rego"))
	if err != nil {
		t.Fatal(err)
	}
	var rules []string
	readJSON(t, filepath.Join(pdir, "rules.json"), &rules)
	var doc caseDoc
	readJSON(t, filepath.Join(pdir, "cases", name+".json"), &doc)
	var sd struct {
		Seal int `json:"seal"`
	}
	readJSON(t, filepath.Join(pdir, "ledger", name+".seal.json"), &sd)
	lraw, err := os.ReadFile(filepath.Join(pdir, "ledger", name+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	l, err := ledger.Load(strings.NewReader(string(lraw)),
		bindings.ClockControlledAt(pdir, decl.TOML), ledger.InZone(bindings.ClockZoneAt(pdir, decl.TOML)))
	if err != nil {
		t.Fatalf("%s/%s: the committed ledger does not load: %v", pkg, name, err)
	}
	if err := l.SealAt(sd.Seal); err != nil {
		t.Fatalf("%s/%s: sealing at the recorded %d: %v", pkg, name, sd.Seal, err)
	}
	o := doc.Input.Observation
	c := engine.Check{Actor: o.Actor, Query: o.Query, Of: o.Of,
		Source: ledger.Source(o.Source), Value: o.Value, Role: o.Role}
	f, err := engine.Evaluate(context.Background(), string(statute), rules, l, c)
	if err != nil {
		t.Fatalf("%s/%s: %v", pkg, name, err)
	}
	return loaded{pkg: pkg, name: name, statute: string(statute), rules: rules, ledger: l, check: c, finding: f}
}

func (lc loaded) run() Run {
	return Run{Package: lc.pkg, Case: lc.name, StatuteSHA: "sha-of-" + lc.pkg}
}

func (lc loaded) build(t *testing.T) *Derivation {
	t.Helper()
	d, err := Build(lc.finding, lc.ledger, lc.check, lc.run())
	if err != nil {
		t.Fatalf("%s/%s: Build: %v", lc.pkg, lc.name, err)
	}
	return d
}

// --- the statute reader, which is the second instrument -------------------
//
// A derivation's rule lines are checked against the STATUTE TEXT and not only
// against the finding they came from. Comparing them to f.Fired alone asks the
// engine whether it agrees with itself; reading the row out of the file asks
// whether the row names a definition of the rule the finding is of, which is
// the claim `rumor explain` makes to its reader.

// definesFinding answers whether row (1-based) of statute is a definition of
// the finding rule, and whether that definition is a default.
func definesFinding(statute string, row int) (isDef, isDefault bool) {
	lines := strings.Split(statute, "\n")
	if row < 1 || row > len(lines) {
		return false, false
	}
	line := lines[row-1]
	if strings.HasPrefix(line, "default "+engine.FindingRule+" ") {
		return true, true
	}
	// A clause's head is `finding := ...` or `finding if ...` at column 0;
	// anything indented is a body line and anything else is another rule.
	for _, head := range []string{engine.FindingRule + " :=", engine.FindingRule + " if", engine.FindingRule + " ="} {
		if strings.HasPrefix(line, head) {
			return true, false
		}
	}
	return false, false
}

// otherRuleRow finds a row that defines some rule OTHER than the finding,
// which is the control for definesFinding: a reader that cannot tell those
// apart would pass this suite while pointing `rumor explain` at any line in
// the file.
//
// IT DOES NOT LOOK FOR A `default` ROW. Two of the five example statutes --
// role-visibility and transitive-count -- carry exactly one default and it is
// the finding's, so a control keyed on `default` found nothing there and the
// arm fataled with the package unnamed. The reader is the one being
// controlled, so the control has to cover every head shape the reader sees.
func otherRuleRow(t *testing.T, pkg, statute string) int {
	t.Helper()
	for i, line := range strings.Split(statute, "\n") {
		name, _, ok := ruleHead(line)
		if !ok || name == engine.FindingRule {
			continue
		}
		return i + 1
	}
	t.Fatalf("%s: no statute row defines a rule other than the finding, so definesFinding has no control", pkg)
	return 0
}

// ruleHead reads the rule a column-0 line defines, if it defines one: the name
// and what follows it. `package` and `import` are statements and not rules.
func ruleHead(line string) (name, rest string, ok bool) {
	rest = strings.TrimPrefix(line, "default ")
	i := 0
	for i < len(rest) && (rest[i] == '_' || (rest[i] >= 'a' && rest[i] <= 'z') ||
		(rest[i] >= '0' && rest[i] <= '9')) {
		i++
	}
	if i == 0 || i == len(rest) {
		return "", "", false
	}
	name, rest = rest[:i], rest[i:]
	if name == "package" || name == "import" {
		return "", "", false
	}
	for _, head := range []string{" :=", " =", " if", " contains", "["} {
		if strings.HasPrefix(rest, head) {
			return name, rest, true
		}
	}
	return "", "", false
}

// --- arm 1: every example case's rule lines are its statute's own ---------

func TestEveryExampleCaseNamesTheStatuteLinesItFired(t *testing.T) {
	built := 0
	for _, pkg := range packages(t) {
		for _, name := range cases(t, pkg) {
			lc := load(t, pkg, name)
			d := lc.build(t)
			built++

			// The rule lines ARE the finding's fired definitions: same
			// count, same rows, same default flags, nothing invented and
			// nothing dropped.
			if got, want := len(d.Verdict.Rules), len(lc.finding.Fired); got != want {
				t.Fatalf("%s/%s: %d rule line(s) for %d fired definition(s)", pkg, name, got, want)
			}
			for i, def := range lc.finding.Fired {
				got := d.Verdict.Rules[i]
				if got.Module != def.Module || got.Row != def.Row || got.Default != def.Default {
					t.Errorf("%s/%s: rule line %d is %s:%d (default %v), fired was %s",
						pkg, name, i, got.Module, got.Row, got.Default, def)
				}
			}

			// And each one names a row of the statute that really defines
			// the finding, with the default flag the text carries.
			for _, rl := range d.Verdict.Rules {
				if rl.Module != "statute.rego" {
					t.Errorf("%s/%s: rule line module is %q and the only module evaluated is statute.rego",
						pkg, name, rl.Module)
					continue
				}
				isDef, isDefault := definesFinding(lc.statute, rl.Row)
				if !isDef {
					t.Errorf("%s/%s: rule line %s:%d is not a definition of %q in the committed statute",
						pkg, name, rl.Module, rl.Row, engine.FindingRule)
					continue
				}
				if isDefault != rl.Default {
					t.Errorf("%s/%s: %s:%d carries default=%v and the statute text says %v",
						pkg, name, rl.Module, rl.Row, rl.Default, isDefault)
				}
			}

			// THE CONTROL, in the same loop so it cannot be skipped
			// separately: the reader above must say NO to a row that
			// defines another rule.
			other := otherRuleRow(t, pkg, lc.statute)
			if isDef, _ := definesFinding(lc.statute, other); isDef {
				t.Fatalf("%s: definesFinding accepts row %d, which defines another rule: the arm above proves nothing",
					pkg, other)
			}
			if isDef, _ := definesFinding(lc.statute, 0); isDef {
				t.Fatalf("%s: definesFinding accepts row 0, which no file has", pkg)
			}
		}
	}
	if built < 25 {
		t.Fatalf("only %d derivation(s) built: examples/toy alone carried 25 cases when this arm was written, so the walk has lost cases rather than passed", built)
	}
	t.Logf("built %d derivation(s) across %d example package(s)", built, len(packages(t)))
}

// TestToyBugsFireTheDefaultRow pins the measurement RuleLine's doc comment
// rests on: in examples/toy every bug fires the default row -- no clause
// matched -- and every other outcome fires a clause. If the toy gains a bug
// clause this arm goes red, which is the point: the comment has to change with
// it rather than quietly become false.
func TestToyBugsFireTheDefaultRow(t *testing.T) {
	bugs, others := 0, 0
	for _, name := range cases(t, "toy") {
		lc := load(t, "toy", name)
		d := lc.build(t)
		if len(d.Verdict.Rules) != 1 {
			t.Fatalf("toy/%s fires %d definitions, and this arm reads one", name, len(d.Verdict.Rules))
		}
		isBug := d.Verdict.Value == "bug"
		if isBug != d.Verdict.Rules[0].Default {
			t.Errorf("toy/%s says %q off %s:%d (default %v)", name, d.Verdict.Says,
				d.Verdict.Rules[0].Module, d.Verdict.Rules[0].Row, d.Verdict.Rules[0].Default)
		}
		if isBug {
			bugs++
		} else {
			others++
		}
	}
	if bugs == 0 || others == 0 {
		t.Fatalf("%d bug(s) and %d other outcome(s): an arm about the difference needs both", bugs, others)
	}
	t.Logf("%d bug(s) off the default row, %d outcome(s) off a clause", bugs, others)
}

// --- arm 2: the refusals, each by name ------------------------------------

func TestBuildRefusesByName(t *testing.T) {
	base := load(t, "toy", "stale-count")

	noOrigin := func() map[string]any {
		in := copyInput(base.finding.Input)
		in["known"] = []string{"alice.event#1", "nobody.claimed#1"}
		return in
	}

	for _, tc := range []struct {
		name   string
		want   error
		says   string
		mutate func(f *engine.Finding, l **ledger.Ledger, c *engine.Check)
	}{
		{"no trace at all", ErrNoTrace, "nobody traced",
			func(f *engine.Finding, _ **ledger.Ledger, _ *engine.Check) { f.Trace, f.TraceEvents = "", 0 }},
		{"events without text", ErrNoTrace, "nobody traced",
			func(f *engine.Finding, _ **ledger.Ledger, _ *engine.Check) { f.Trace = "" }},
		{"text without events", ErrNoTrace, "nobody traced",
			func(f *engine.Finding, _ **ledger.Ledger, _ *engine.Check) { f.TraceEvents = 0 }},
		{"no ledger", ErrUnsealed, "no ledger was given",
			func(_ *engine.Finding, l **ledger.Ledger, _ *engine.Check) { *l = nil }},
		{"unsealed ledger", ErrUnsealed, "not knowable",
			func(_ *engine.Finding, l **ledger.Ledger, _ *engine.Check) { *l = unsealed(t, "toy", "stale-count") }},
		{"no input", ErrNoInput, "spine",
			func(f *engine.Finding, _ **ledger.Ledger, _ *engine.Check) { f.Input = nil }},
		{"input without observation", ErrNoInput, "no `observation` map",
			func(f *engine.Finding, _ **ledger.Ledger, _ *engine.Check) {
				in := copyInput(f.Input)
				delete(in, "observation")
				f.Input = in
			}},
		{"observation without a position", ErrNoInput, "at a position or nowhere",
			func(f *engine.Finding, _ **ledger.Ledger, _ *engine.Check) {
				in := copyInput(f.Input)
				obs := copyInput(in["observation"].(map[string]any))
				delete(obs, "position")
				in["observation"] = obs
				f.Input = in
			}},
		{"input without known", ErrNoInput, "no `known`",
			func(f *engine.Finding, _ **ledger.Ledger, _ *engine.Check) {
				in := copyInput(f.Input)
				delete(in, "known")
				f.Input = in
			}},
		{"known is not a list of refs", ErrNoInput, "not a list of refs",
			func(f *engine.Finding, _ **ledger.Ledger, _ *engine.Check) {
				in := copyInput(f.Input)
				in["known"] = map[string]any{"alice.event#1": true}
				f.Input = in
			}},
		{"a known ref with no origin", ErrDisagrees, "flattener and the ledger disagree",
			func(f *engine.Finding, _ **ledger.Ledger, _ *engine.Check) { f.Input = noOrigin() }},
		{"a check naming another actor", ErrDisagrees, "not the one the selector describes",
			func(_ *engine.Finding, _ **ledger.Ledger, c *engine.Check) { c.Actor = "carol" }},
		{"a check naming another query", ErrDisagrees, "not the one the selector describes",
			func(_ *engine.Finding, _ **ledger.Ledger, c *engine.Check) { c.Query = "page_count" }},
		{"a check naming another source", ErrDisagrees, "not the one the selector describes",
			func(_ *engine.Finding, _ **ledger.Ledger, c *engine.Check) { c.Source = ledger.Source("browser") }},
		{"a check naming another role", ErrDisagrees, "not the one the selector describes",
			func(_ *engine.Finding, _ **ledger.Ledger, c *engine.Check) { c.Role = "guest" }},
		{"a check naming another claim type", ErrDisagrees, "not the one the selector describes",
			func(_ *engine.Finding, _ **ledger.Ledger, c *engine.Check) { c.Of = "page.created" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := base.finding
			l := base.ledger
			c := base.check
			tc.mutate(&f, &l, &c)
			d, err := Build(f, l, c, base.run())
			if err == nil {
				t.Fatalf("Build returned a derivation for %s: %+v", tc.name, d.Verdict)
			}
			if d != nil {
				t.Errorf("Build returned both a derivation and an error, and a caller reading the value first would render it")
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, wanted %v", err, tc.want)
			}
			if !strings.Contains(err.Error(), tc.says) {
				t.Errorf("the refusal does not say why: %v", err)
			}
		})
	}

	// AND THE CONTROL: the unmutated case builds. Without it every arm above
	// would pass against a Build that refused everything.
	if _, err := Build(base.finding, base.ledger, base.check, base.run()); err != nil {
		t.Fatalf("the unmutated case refuses too, so the arms above are vacuous: %v", err)
	}
}

// unsealed loads a case's ledger and does NOT seal it, which is the state
// engine.Input refuses as a sequencing error.
func unsealed(t *testing.T, pkg, name string) *ledger.Ledger {
	t.Helper()
	pdir := filepath.Join(examplesDir(), pkg)
	raw, err := os.ReadFile(filepath.Join(pdir, "ledger", name+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	l, err := ledger.Load(strings.NewReader(string(raw)),
		bindings.ClockControlledAt(pdir, decl.TOML), ledger.InZone(bindings.ClockZoneAt(pdir, decl.TOML)))
	if err != nil {
		t.Fatal(err)
	}
	if l.Seal() != 0 {
		t.Fatalf("a freshly loaded ledger reports seal %d, so this is not the unsealed state", l.Seal())
	}
	return l
}

func copyInput(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// --- arm 3: an undefined finding is explained, not refused ----------------

func TestUndefinedFindingIsExplained(t *testing.T) {
	base := load(t, "toy", "stale-count")
	f := base.finding
	// A traced finding off no definition, which is what a statute with no
	// clause for this observation and no default produces.
	f.Value, f.Defined, f.Fired = nil, false, nil

	d, err := Build(f, base.ledger, base.check, base.run())
	if err != nil {
		t.Fatalf("an undefined finding was refused, and "+
			"\"the statute said nothing\" is the answer a reader most needs: %v", err)
	}
	if d.Verdict.Defined {
		t.Error("the derivation reports the finding as defined")
	}
	if d.Verdict.Says != "no finding" {
		t.Errorf("says is %q, and a reader of a nil value cannot tell no verdict from a verdict of nil", d.Verdict.Says)
	}
	if len(d.Verdict.Rules) != 0 {
		t.Errorf("an undefined finding names %d statute line(s), and none held", len(d.Verdict.Rules))
	}
	// The chain is still there: the refusal would have cost the reader the
	// facts, which are the whole of what is left to look at.
	if len(d.Expectation.Known) == 0 || len(d.Expectation.Claims) == 0 {
		t.Errorf("the chain is empty: %d known, %d claim(s)", len(d.Expectation.Known), len(d.Expectation.Claims))
	}
	b, err := json.Marshal(d.Verdict)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"defined":false`) {
		t.Errorf("the JSON does not state the absence: %s", b)
	}
}

// --- arm 4: the knowledge join --------------------------------------------

func TestKnownFactsCarryTheEntryThatMadeThemKnown(t *testing.T) {
	// stale-count's alice holds two claims of her own and was TOLD of bob's
	// join, so one case carries both origins.
	d := load(t, "toy", "stale-count").build(t)

	want := map[string]Origin{
		"alice.event#1": {Kind: "claim", Actor: "alice", Position: 1, Type: "event.created"},
		"alice.del#1":   {Kind: "claim", Actor: "alice", Position: 5, Type: "event.deleted"},
		"bob.join#1":    {Kind: "tell", Actor: "bob", To: "alice", Position: 4},
	}
	if len(d.Expectation.Known) != len(want) {
		t.Fatalf("%d known fact(s), wanted %d", len(d.Expectation.Known), len(want))
	}
	for _, kf := range d.Expectation.Known {
		w, ok := want[kf.Ref]
		if !ok {
			t.Errorf("unexpected ref %q", kf.Ref)
			continue
		}
		if len(kf.Via) != 1 {
			t.Errorf("%s has %d origin(s), wanted 1: %+v", kf.Ref, len(kf.Via), kf.Via)
			continue
		}
		if kf.Via[0] != w {
			t.Errorf("%s via %+v, wanted %+v", kf.Ref, kf.Via[0], w)
		}
		if kf.Via[0].Position > d.Reproduction.Seal {
			t.Errorf("%s via position %d, which is past the seal %d", kf.Ref, kf.Via[0].Position, d.Reproduction.Seal)
		}
	}

	// EVERY CASE, not only this one: a known fact with no origin is the
	// defect knownOf refuses, so no derivation this package returns may
	// carry one.
	for _, pkg := range packages(t) {
		for _, name := range cases(t, pkg) {
			for _, kf := range load(t, pkg, name).build(t).Expectation.Known {
				if len(kf.Via) == 0 {
					t.Errorf("%s/%s: %q is known for no reason", pkg, name, kf.Ref)
				}
			}
		}
	}
}

// TestKnownOfIsBoundedByTheSealEvenWhenItsCallerIsNot drives knownOf with facts
// PAST the seal, which no committed case can produce: every example ledger ends
// at its own seal, so the bound inside knownOf is redundant against
// ledger.SealedPrefix for every fixture in the tree.
//
// THIS ARM IS WHY THE BOUND IS NOT SCAFFOLDING. A mutation battery removing it
// survived the rest of this suite, and the engine makes the same argument about
// the seal it passes to the flattener: the premise -- that nothing handed in
// sits past the seal -- holds today and is the caller's, while the function's
// contract is "at or before the seal". So the premise is asserted here instead
// of inferred from a passing suite, and the bound now has a test that kills its
// removal.
func TestKnownOfIsBoundedByTheSealEvenWhenItsCallerIsNot(t *testing.T) {
	lc := load(t, "toy", "stale-count")
	seal := lc.ledger.Seal()
	prefix := lc.ledger.SealedPrefix()
	for _, f := range prefix {
		if f.Position > seal {
			t.Fatalf("the sealed prefix already holds position %d past the seal %d, which is a ledger defect and not this package's", f.Position, seal)
		}
	}
	// Two entries a careless caller would hand in: a later tell of a ref the
	// actor already knows, and a later claim of her own.
	unbounded := append(append([]ledger.Fact{}, prefix...),
		ledger.Fact{Kind: ledger.KindTold, Position: seal + 1, Actor: "carol", To: "alice", ClaimRef: "bob.join#1"},
		ledger.Fact{Kind: ledger.KindClaim, Position: seal + 2, Actor: "alice", Type: "event.recreated", Ref: "alice.event#1"},
	)

	got, err := knownOf(lc.finding.Input, unbounded, "alice", seal)
	if err != nil {
		t.Fatalf("knownOf: %v", err)
	}
	for _, kf := range got {
		if len(kf.Via) != 1 {
			t.Errorf("%s has %d origin(s) at the seal, wanted 1: %+v", kf.Ref, len(kf.Via), kf.Via)
		}
		for _, o := range kf.Via {
			if o.Position > seal {
				t.Errorf("%s via position %d, which is past the seal %d", kf.Ref, o.Position, seal)
			}
		}
	}

	// THE CONTROL: the same facts under a looser seal DO reach the join, so
	// the arm above is about the bound and not about facts it could never
	// have found.
	loose, err := knownOf(lc.finding.Input, unbounded, "alice", seal+2)
	if err != nil {
		t.Fatalf("knownOf at the looser seal: %v", err)
	}
	extra := 0
	for _, kf := range loose {
		for _, o := range kf.Via {
			if o.Position > seal {
				extra++
			}
		}
	}
	if extra != 2 {
		t.Fatalf("the looser seal reaches %d entry past %d, wanted 2: the planted facts are unreachable and the arm proves nothing", extra, seal)
	}
}

// --- arm 5: the clock, where the run has one ------------------------------

func TestClockIsCarriedExactlyWhereTheRunHasOne(t *testing.T) {
	clocked, bare := 0, 0
	for _, pkg := range packages(t) {
		for _, name := range cases(t, pkg) {
			lc := load(t, pkg, name)
			d := lc.build(t)
			in, hasClock := lc.finding.Input["clock"].(map[string]any)
			switch {
			case hasClock && d.Clock == nil:
				t.Errorf("%s/%s: the statute was given a clock and the derivation has none", pkg, name)
			case !hasClock && d.Clock != nil:
				t.Errorf("%s/%s: the derivation invents a clock at %d", pkg, name, d.Clock.Now)
			case hasClock:
				clocked++
				now, ok := in["now"].(int64)
				if !ok {
					t.Fatalf("%s/%s: the input's clock.now is %T, which this arm cannot compare", pkg, name, in["now"])
				}
				if d.Clock.Now != now {
					t.Errorf("%s/%s: clock %d, the statute was given %d", pkg, name, d.Clock.Now, now)
				}
				// A claim's At is the clock the run was at, so a clocked
				// run's claims carry one.
				for _, cl := range d.Expectation.Claims {
					if cl.At == nil {
						t.Errorf("%s/%s: claim %s has no time in a clocked run", pkg, name, cl.Ref)
					}
				}
			default:
				bare++
				for _, cl := range d.Expectation.Claims {
					if cl.At != nil {
						t.Errorf("%s/%s: claim %s carries time %d in a run with no clock", pkg, name, cl.Ref, *cl.At)
					}
				}
			}
		}
	}
	if clocked == 0 || bare == 0 {
		t.Fatalf("%d clocked case(s) and %d without: an arm about the difference needs both", clocked, bare)
	}
}

// --- arm 6: the verdict's other halves ------------------------------------

func TestHoldsCarriesTheStatutesOtherAnswersAndNotTheFinding(t *testing.T) {
	lc := load(t, "toy", "stale-count")
	d := lc.build(t)

	if _, ok := d.Expectation.Holds[engine.FindingRule]; ok {
		t.Errorf("Holds carries the finding itself, which is the Verdict: %v", d.Expectation.Holds[engine.FindingRule])
	}
	if len(d.Expectation.Holds) != len(lc.finding.Rules)-1 {
		t.Errorf("Holds has %d rule(s) and the finding answered %d", len(d.Expectation.Holds), len(lc.finding.Rules))
	}
	for rule, v := range lc.finding.Rules {
		if rule == engine.FindingRule {
			continue
		}
		if got, ok := d.Expectation.Holds[rule]; !ok || !reflect.DeepEqual(got, v) {
			t.Errorf("Holds[%q] is %v (present %v), the statute answered %v", rule, got, ok, v)
		}
	}

	// IT IS A COPY: a view that can change its source is not a view.
	d.Expectation.Holds["believed"] = "mutated"
	delete(d.Expectation.Holds, "may_join")
	if lc.finding.Rules["believed"] == "mutated" {
		t.Error("writing through Holds changed the finding's own answer")
	}
	if _, ok := lc.finding.Rules["may_join"]; !ok {
		t.Error("deleting through Holds removed one of the finding's own answers")
	}
}

func TestOverlayIsCarriedWithTheStatutesOwnVerdict(t *testing.T) {
	base := load(t, "toy", "stale-count")

	// Without an overlay the absence is stated and no overlay is invented.
	d := base.build(t)
	if d.Verdict.Overridden || d.Verdict.Overlay != nil {
		t.Errorf("an un-overridden finding reports overridden=%v overlay=%+v", d.Verdict.Overridden, d.Verdict.Overlay)
	}
	b, err := json.Marshal(d.Verdict)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"overridden":false`) {
		t.Errorf("the JSON leaves the override unstated: %s", b)
	}
	if strings.Contains(string(b), "statute_value") {
		t.Errorf("the JSON carries a statute_value for a finding nothing overrode: %s", b)
	}

	// With one, the engine's rule is named and the statute's own answer is
	// kept beside it -- a reader shown only the statute's lines for an
	// overridden finding is reading the wrong rule entirely.
	f := base.finding
	f.Value, f.Kind = "pending", ""
	f.StatuteValue = "bug"
	f.Overlay = &engine.Overlay{Rule: "engine.pending", Reason: "held open until the bound",
		Value: "pending", Until: 30000000000, Detail: ""}

	d, err = Build(f, base.ledger, base.check, base.run())
	if err != nil {
		t.Fatal(err)
	}
	if !d.Verdict.Overridden {
		t.Error("an overridden finding does not say so")
	}
	if d.Verdict.StatuteValue != "bug" {
		t.Errorf("the statute's own verdict is %v, wanted bug", d.Verdict.StatuteValue)
	}
	if d.Verdict.Overlay == nil {
		t.Fatal("the overlay is absent")
	}
	if got := *d.Verdict.Overlay; got.Rule != "engine.pending" || got.Value != "pending" || got.Until != 30000000000 {
		t.Errorf("the overlay reads %+v", got)
	}
	// The statute's lines are still the statute's: the overlay does not
	// rewrite where the finding came from.
	if len(d.Verdict.Rules) != len(base.finding.Fired) {
		t.Errorf("%d rule line(s) under an overlay, the statute fired %d", len(d.Verdict.Rules), len(base.finding.Fired))
	}
}

// --- arm 7: the selector and the observation ------------------------------

func TestSelectorIsTheCheckAtItsPositionAndTheObservationIsTheInputs(t *testing.T) {
	for _, pkg := range packages(t) {
		for _, name := range cases(t, pkg) {
			lc := load(t, pkg, name)
			d := lc.build(t)
			in := lc.finding.Input["observation"].(map[string]any)

			if d.Selector.Actor != lc.check.Actor || d.Selector.Query != lc.check.Query ||
				d.Selector.Of != lc.check.Of || d.Selector.Source != string(lc.check.Source) {
				t.Errorf("%s/%s: the selector %+v is not the check %+v", pkg, name, d.Selector, lc.check)
			}
			if want := in["position"].(int); d.Selector.Position != want || d.Observation.Position != want {
				t.Errorf("%s/%s: selector at %d and observation at %d, the statute was given %d",
					pkg, name, d.Selector.Position, d.Observation.Position, want)
			}
			if want := lc.ledger.Seal() + 1; d.Observation.Position != want {
				t.Errorf("%s/%s: the observation sits at %d and the seal is %d: a check is the position after the sealed prefix",
					pkg, name, d.Observation.Position, lc.ledger.Seal())
			}
			if !reflect.DeepEqual(d.Observation.Value, in["value"]) {
				t.Errorf("%s/%s: observation value %v, the statute read %v", pkg, name, d.Observation.Value, in["value"])
			}
			if d.Observation.Role != fmt.Sprint(orEmpty(in["role"])) {
				t.Errorf("%s/%s: observation role %q, the statute read %v", pkg, name, d.Observation.Role, in["role"])
			}
			if d.Reproduction.Seal != lc.ledger.Seal() || d.Reproduction.Events != lc.finding.TraceEvents {
				t.Errorf("%s/%s: reproduction %+v against seal %d and %d event(s)",
					pkg, name, d.Reproduction, lc.ledger.Seal(), lc.finding.TraceEvents)
			}
			if d.Reproduction.Package != pkg || d.Reproduction.Case != name {
				t.Errorf("%s/%s: reproduction names %s/%s", pkg, name, d.Reproduction.Package, d.Reproduction.Case)
			}
		}
	}
}

func orEmpty(v any) any {
	if v == nil {
		return ""
	}
	return v
}

// --- arm 8: an input that made a round trip through JSON ------------------
//
// d3 renders this derivation as JSON, and a finding read back from a file
// carries float64 where the engine wrote int and []any where it wrote
// []string. Both must build the SAME derivation, or the shape means one thing
// in process and another on disk.

func TestInputThroughJSONBuildsTheSameDerivation(t *testing.T) {
	for _, name := range []string{"stale-count", "invitation-honored-late"} {
		t.Run(name, func(t *testing.T) {
			lc := load(t, "toy", name)
			want := lc.build(t)

			raw, err := json.Marshal(lc.finding.Input)
			if err != nil {
				t.Fatal(err)
			}
			var back map[string]any
			if err := json.Unmarshal(raw, &back); err != nil {
				t.Fatal(err)
			}
			// The round trip is real: this is the shape change the arm is about.
			if _, ok := back["observation"].(map[string]any)["position"].(float64); !ok {
				t.Fatalf("the round trip did not produce a float64 position, so this arm tests nothing: %T",
					back["observation"].(map[string]any)["position"])
			}
			if _, ok := back["known"].([]any); !ok {
				t.Fatalf("the round trip did not produce a []any known, so this arm tests nothing: %T", back["known"])
			}

			f := lc.finding
			f.Input = back
			got, err := Build(f, lc.ledger, lc.check, lc.run())
			if err != nil {
				t.Fatalf("a finding whose input made a round trip through JSON is refused: %v", err)
			}

			// Compared through JSON in both directions, since the value a
			// round trip carries is a float64 by construction and the
			// derivation is what must agree, not the Go types inside it.
			jw, err := json.Marshal(want)
			if err != nil {
				t.Fatal(err)
			}
			jg, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			if string(jw) != string(jg) {
				t.Errorf("the derivations differ:\n in process: %s\nafter JSON: %s", jw, jg)
			}
		})
	}
}

// --- arm 9: d1 owns its types ---------------------------------------------

// TestNoEngineTypeIsEmbedded is the structural half of the trace decision:
// engine.Finding's fields are untagged and Trace is exported, so one embedded
// engine type would ship OPA's pretty text as "Trace" with nothing failing.
// d3's fixture asserts no Trace key at any depth of the rendered JSON; this
// asserts the types themselves, which is what makes that absence structural
// rather than a habit.
// foreignFields reports every field of ty, walked to any depth, that is a
// struct from another package or that carries the raw trace. It is a function
// rather than assertions in a loop so the arm below can run it over a shape
// that DOES embed an engine type and check that it objects.
func foreignFields(ty reflect.Type, path string, seen map[reflect.Type]bool) []string {
	for ty.Kind() == reflect.Pointer || ty.Kind() == reflect.Slice || ty.Kind() == reflect.Array ||
		ty.Kind() == reflect.Map {
		ty = ty.Elem()
	}
	if ty.Kind() != reflect.Struct || seen[ty] {
		return nil
	}
	seen[ty] = true
	if pkg := ty.PkgPath(); pkg != "" && !strings.HasSuffix(pkg, "/internal/explain") {
		return []string{fmt.Sprintf("%s is %s.%s, a type from another package: its fields change under this shape without a line of this package changing",
			path, pkg, ty.Name())}
	}
	var out []string
	for i := 0; i < ty.NumField(); i++ {
		fld := ty.Field(i)
		if fld.Name == "Trace" || strings.Contains(strings.ToLower(fld.Tag.Get("json")), "trace") {
			out = append(out, fmt.Sprintf("%s.%s is a trace field, and the raw trace is deliberately not in this shape",
				path, fld.Name))
		}
		out = append(out, foreignFields(fld.Type, path+"."+fld.Name, seen)...)
	}
	return out
}

func TestNoEngineTypeIsEmbedded(t *testing.T) {
	for _, bad := range foreignFields(reflect.TypeOf(Derivation{}), "Derivation", map[reflect.Type]bool{}) {
		t.Error(bad)
	}

	// THE CONTROL, both ways the shape could lose this property: a field of
	// an engine type, and a trace field of its own. A reader that reported
	// neither would pass the arm above over any shape at all.
	type withEngine struct {
		Selector Selector
		Finding  engine.Finding
	}
	type withTrace struct {
		Selector Selector
		Trace    string
	}
	for _, control := range []struct {
		name string
		ty   reflect.Type
	}{
		{"a field of an engine type", reflect.TypeOf(withEngine{})},
		{"a trace field of its own", reflect.TypeOf(withTrace{})},
	} {
		if got := foreignFields(control.ty, "control", map[reflect.Type]bool{}); len(got) == 0 {
			t.Errorf("the reader accepts %s, so the arm above proves nothing", control.name)
		}
	}
}
