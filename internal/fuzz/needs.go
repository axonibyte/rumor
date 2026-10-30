package fuzz

import (
	"sort"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
)

// Needs are the handle kinds a corpus needs honest ids for, sorted.
//
// IT READS THE CORPUS AND NOT THE BINDINGS, because the corpus is what will
// be sent: an action whose every case is `missing` on its only handle would
// need no id at all, and a setup that ran anyway would be doing work no case
// asked for.
//
// THIS IS THE HALF OF 108's setup.go THAT DID NOT MOVE. internal/setup now
// holds the ORDER, which 109's second stage needed for its own reason; what a
// stage wants ids for is the stage's own question, and this one's answer is a
// property of the hostile corpus rather than of the declarations.
func Needs(b bindings.Bindings, cases []Case) []string {
	byAction := map[string]bindings.Action{}
	for _, a := range b.Actions {
		byAction[a.Name] = a
	}
	set := map[string]bool{}
	for _, c := range cases {
		a := byAction[c.Action]
		for _, arg := range a.Args {
			if arg.Source != "handle" || arg.Of == "" {
				continue
			}
			// THE PERTURBED FIELD STILL NEEDS ITS KIND unless the case omits
			// it: `unknown-id` sends a derived id and needs none, but
			// `wrong-type` on one handle leaves the OTHER handle honest.
			if arg.Name == c.Field && (c.Class == ClassMissing || c.Class == ClassUnknownID) {
				continue
			}
			set[arg.Of] = true
		}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
