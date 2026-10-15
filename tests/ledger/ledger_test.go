// The example cases as ledger files (R2.1, issue 42).
//
// WHAT THIS ASSERTS AND WHY IT IS NOT A TAUTOLOGY. It would be easy to write a
// test that rebuilds each ledger the way the generator did and compares -- which
// would agree with itself no matter what. Instead every arm here reads the
// COMMITTED file and checks it against the CASE, which was authored
// independently and before this package existed:
//
//	the file loads through internal/ledger, so every guard applies to it
//	the sealed prefix's claims equal the case's claims inside its bound
//	the prefix's tells flatten to exactly the knowledge the bound allows
//	the committed bytes are what the store re-dumps, so no file has drifted
//	every case has a ledger and every ledger has a case
package ledger_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"forge.axonibyte.com/axonibyte/rumor/internal/ledger"
)

type claimJSON struct {
	Ref       string   `json:"ref"`
	Actor     string   `json:"actor"`
	Type      string   `json:"type"`
	Position  int      `json:"position"`
	Handle    string   `json:"handle"`
	Planted   bool     `json:"planted"`
	VisibleTo []string `json:"visible_to"`
}

type caseJSON struct {
	Input struct {
		Observation struct {
			Actor    string `json:"actor"`
			Position int    `json:"position"`
		} `json:"observation"`
		Known  []string    `json:"known"`
		Claims []claimJSON `json:"claims"`
	} `json:"input"`
}

type sealJSON struct {
	Bound           int      `json:"bound"`
	Observer        string   `json:"observer"`
	Seal            int      `json:"seal"`
	KnownInPrefix   []string `json:"known_in_prefix"`
	OwnClaimsNoTell []string `json:"own_claims_need_no_tell"`
}

func readJSON(t *testing.T, path string, v interface{}) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
}

func root(t *testing.T) string {
	t.Helper()
	// tests/ledger -> repository root
	return filepath.Join("..", "..")
}

func ledgerFiles(t *testing.T) []string {
	t.Helper()
	g, err := filepath.Glob(filepath.Join(root(t), "examples", "*", "ledger", "*.jsonl"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	sort.Strings(g)
	if len(g) == 0 {
		t.Fatal("no ledger files found: an empty walk is not a pass")
	}
	return g
}

// EVERY CASE HAS A LEDGER AND EVERY LEDGER HAS A CASE. Without this, a case
// could be dropped and the remaining arms would still all pass.
func TestTheLedgersAndTheCasesAreInBijection(t *testing.T) {
	cases, err := filepath.Glob(filepath.Join(root(t), "examples", "*", "cases", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	led := ledgerFiles(t)
	if len(cases) != len(led) {
		t.Fatalf("%d case(s) but %d ledger(s): they must be in bijection", len(cases), len(led))
	}
	for _, c := range cases {
		want := strings.Replace(strings.Replace(c, "/cases/", "/ledger/", 1), ".json", ".jsonl", 1)
		if _, err := os.Stat(want); err != nil {
			t.Errorf("case %s has no ledger at %s", filepath.Base(c), want)
		}
	}
	t.Logf("PROOF %d case(s) and %d ledger(s), one to one", len(cases), len(led))
}

func TestEveryLedgerAgreesWithItsCase(t *testing.T) {
	for _, lf := range ledgerFiles(t) {
		name := filepath.Base(lf)
		t.Run(strings.TrimSuffix(name, ".jsonl"), func(t *testing.T) {
			cf := strings.Replace(strings.Replace(lf, "/ledger/", "/cases/", 1), ".jsonl", ".json", 1)
			var c caseJSON
			var s sealJSON
			readJSON(t, cf, &c)
			readJSON(t, strings.TrimSuffix(lf, ".jsonl")+".seal.json", &s)

			raw, err := os.ReadFile(lf)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			// 1. THE GUARDS APPLY. Load refuses anything a writer would have.
			l, err := ledger.Load(strings.NewReader(string(raw)), false)
			if err != nil {
				t.Fatalf("the committed ledger does not load: %v", err)
			}
			// 2. THE BYTES HAVE NOT DRIFTED from what the store writes.
			var sb strings.Builder
			if err := l.Dump(&sb); err != nil {
				t.Fatalf("dump: %v", err)
			}
			if sb.String() != string(raw) {
				t.Errorf("the committed file is not what the store re-dumps")
			}
			// 3. THE SEAL IS THE BOUND. Sealing must be accepted at the recorded
			// position, which also proves the position exists.
			if err := l.SealAt(s.Seal); err != nil {
				t.Fatalf("sealing at the recorded position %d: %v", s.Seal, err)
			}
			prefix := l.SealedPrefix()

			// 4. THE PREFIX'S CLAIMS ARE THE CASE'S CLAIMS INSIDE THE BOUND.
			var wantRefs []string
			for _, cc := range c.Input.Claims {
				if cc.Position <= c.Input.Observation.Position {
					wantRefs = append(wantRefs, cc.Ref)
				}
			}
			var gotRefs []string
			for _, f := range prefix {
				if f.Kind == ledger.KindClaim {
					gotRefs = append(gotRefs, f.Ref)
				}
			}
			sort.Strings(wantRefs)
			sort.Strings(gotRefs)
			if strings.Join(wantRefs, ",") != strings.Join(gotRefs, ",") {
				t.Errorf("claims inside the bound: ledger has %v, case has %v", gotRefs, wantRefs)
			}

			// 5. THE TELLS FLATTEN TO THE KNOWLEDGE THE BOUND ALLOWS -- the
			// observer's OWN claims need no tell, which is v1's rule and which
			// the store's refusal of a self-addressed tell enforces.
			known := map[string]bool{}
			for _, f := range prefix {
				if f.Kind == ledger.KindTold && f.To == s.Observer {
					known[f.ClaimRef] = true
				}
				if f.Kind == ledger.KindClaim && f.Actor == s.Observer {
					known[f.Ref] = true
				}
			}
			var gotKnown []string
			for k := range known {
				gotKnown = append(gotKnown, k)
			}
			sort.Strings(gotKnown)
			want := append([]string{}, s.KnownInPrefix...)
			sort.Strings(want)
			if strings.Join(gotKnown, ",") != strings.Join(want, ",") {
				t.Errorf("flattened knowledge in the prefix: got %v, recorded %v", gotKnown, want)
			}
			// 6. AND THE RECORDED KNOWLEDGE IS THE CASE'S, FILTERED TO THE BOUND.
			// This is the arm that would catch a seal file quietly disagreeing
			// with the case it claims to describe.
			byRef := map[string]claimJSON{}
			for _, cc := range c.Input.Claims {
				byRef[cc.Ref] = cc
			}
			var caseKnownInBound []string
			for _, k := range c.Input.Known {
				if cc, ok := byRef[k]; ok && cc.Position <= c.Input.Observation.Position {
					caseKnownInBound = append(caseKnownInBound, k)
				}
			}
			sort.Strings(caseKnownInBound)
			if strings.Join(want, ",") != strings.Join(caseKnownInBound, ",") {
				t.Errorf("recorded knowledge %v is not the case's %v filtered to bound %d",
					want, caseKnownInBound, c.Input.Observation.Position)
			}
			if s.Observer != c.Input.Observation.Actor {
				t.Errorf("observer %q is not the case's %q", s.Observer, c.Input.Observation.Actor)
			}
			if s.Bound != c.Input.Observation.Position {
				t.Errorf("bound %d is not the case's observation position %d", s.Bound, c.Input.Observation.Position)
			}
		})
	}
}

// THE TWO POSITION-BOUNDED CASES ARE THE REASON THE LEDGER FORM DIFFERS, so
// they get an arm of their own rather than only riding the loop: the case hands
// the statute knowledge its bound excludes, and the ledger excludes it
// STRUCTURALLY by placing the tell after the seal.
func TestABoundExcludesALateTellStructurally(t *testing.T) {
	found := 0
	for _, lf := range ledgerFiles(t) {
		var s sealJSON
		readJSON(t, strings.TrimSuffix(lf, ".jsonl")+".seal.json", &s)
		cf := strings.Replace(strings.Replace(lf, "/ledger/", "/cases/", 1), ".jsonl", ".json", 1)
		var c caseJSON
		readJSON(t, cf, &c)
		if len(c.Input.Known) == len(s.KnownInPrefix) {
			continue // the bound excludes nothing here
		}
		found++
		raw, err := os.ReadFile(lf)
		if err != nil {
			t.Fatal(err)
		}
		l, err := ledger.Load(strings.NewReader(string(raw)), false)
		if err != nil {
			t.Fatal(err)
		}
		if err := l.SealAt(s.Seal); err != nil {
			t.Fatal(err)
		}
		if len(l.SealedPrefix()) >= l.Len() {
			t.Errorf("%s: the bound excludes knowledge but nothing sits past the seal", filepath.Base(lf))
		}
		t.Logf("PROOF %s: case knows %d, bound allows %d, %d fact(s) past the seal",
			filepath.Base(lf), len(c.Input.Known), len(s.KnownInPrefix), l.Len()-len(l.SealedPrefix()))
	}
	if found == 0 {
		t.Error("no case was found whose bound excludes knowledge, so this arm proved nothing")
	}
}
