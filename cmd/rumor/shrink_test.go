package main

import (
	"strings"
	"testing"
)

// THE THREE REFUSALS NO PROJECT CAN REACH, each with a fixture that trips it.
//
// I SHIPPED THEM UNTRIPPED AND SAID SO (coop r6/148): the product registers one
// provider, it implements both methods, and examples/toy passes TOY_ADDR
// through, so nothing a project can declare reaches any of these. A refusal
// nothing trips is deletable in silence -- arm 19's shape, which this room has
// found in four tools this week -- and the fixture is a stub provider rather
// than a document because a provider is an interface value and a missing method
// is not something a platform.toml can express.
//
// THE CELLS ARE THE THREE ANSWERS AND A CONTROL, because the first three would
// all pass for a function that refused everything: the full surface with its
// variable passed through returns no reason at all.
func TestTheAdapterMustBePointableAtTheProvidersWorld(t *testing.T) {
	for _, tc := range []struct {
		name  string
		prov  any
		env   []string
		says  string
		wants bool // wants a refusal
	}{
		{
			name:  "no Addr at all",
			prov:  struct{}{},
			env:   []string{"TOY_ADDR"},
			says:  "does not expose the address of the world it owns",
			wants: true,
		},
		{
			name:  "an address with no variable named for it",
			prov:  addrOnly{},
			env:   []string{"TOY_ADDR"},
			says:  "does not name the environment variable",
			wants: true,
		},
		{
			name:  "the variable is not passed through",
			prov:  addrAndEnv{},
			env:   []string{"SOMETHING_ELSE"},
			says:  "[adapter] env does not pass through",
			wants: true,
		},
		{
			// THE CONTROL. Without it a providerAddress that refused every
			// provider would satisfy all three cells above.
			name:  "the whole surface, variable passed through",
			prov:  addrAndEnv{},
			env:   []string{"PATH", "TOY_ADDR"},
			wants: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			at, ae, refusal := providerAddress(tc.prov, "p", tc.env)
			if tc.wants {
				if refusal == "" {
					t.Fatalf("accepted: the adapter would be pointed at nothing and every " +
						"candidate would run against a world this provider cannot put back")
				}
				if !strings.Contains(refusal, tc.says) {
					t.Errorf("refused, but not by the clause for %s: %s", tc.name, refusal)
				}
				// AND IT HANDS BACK NOTHING USABLE, so a caller that ignored
				// the reason could not proceed on a half-answer.
				if at != nil || ae != nil {
					t.Errorf("a refusal returned a usable address or variable: %v %v", at, ae)
				}
				return
			}
			if refusal != "" {
				t.Fatalf("the full surface was refused: %s", refusal)
			}
			if at == nil || ae == nil {
				t.Fatalf("accepted and handed back nothing: %v %v", at, ae)
			}
			if ae.AddrEnv() != "TOY_ADDR" || at.Addr() != "127.0.0.1:1" {
				t.Errorf("the accepted pair does not carry the stub's own values: %q %q",
					at.Addr(), ae.AddrEnv())
			}
		})
	}
}

type addrOnly struct{}

func (addrOnly) Addr() string { return "127.0.0.1:1" }

type addrAndEnv struct{}

func (addrAndEnv) Addr() string    { return "127.0.0.1:1" }
func (addrAndEnv) AddrEnv() string { return "TOY_ADDR" }
