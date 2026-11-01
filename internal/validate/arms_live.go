package validate

import (
	"context"
	"sort"
	"strings"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/platform"
	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

// live asks the adapter to describe itself, once, for the five arms that cannot
// be answered any other way.
//
// A FAILURE TO ANSWER IS could-not-run AND NEVER A REFUSAL (fable, coop r4/74,
// and docs/contract.md:57): the project is not wrong when its adapter will not
// start or will not speak. So the arms below simply do not run, and the report
// carries the reason instead of a verdict about the project.
func live(rep *Report, dir string, p platform.Platform) (wire.DescribeResponse, bool) {
	cwd := p.Adapter.Cwd
	if cwd == "" {
		cwd = dir
	}
	d, err := wire.Describe(context.Background(), wire.Spawn{
		Command: p.Adapter.Command,
		Cwd:     cwd,
		Env:     p.Adapter.Env,
	}, 0)
	if err != nil {
		rep.cannot("asking the adapter to describe itself: %v", err)
		// THE FIVE LIVE ARMS ARE SKIPPED AND NOT PASSED. The run is
		// could-not-run either way, but a report claiming eighteen of eighteen
		// when five never ran would be the vacuity this accounting exists for.
		//
		// ARM 21 JOINED THIS LIST IN THE SAME CHANGE THAT ADDED IT, because an
		// arm that is neither ran nor skipped is counted by nothing -- which is
		// the exact failure the sentence above names, and the reason the word
		// "four" above is now "five" rather than left to drift.
		for _, arm := range []string{"9a", "9b", "9c", "13", "21"} {
			rep.skip(arm, "the adapter did not answer a describe, so what it can do is unknown")
		}
		return d, false
	}
	rep.Described = true
	return d, true
}

// arm9: A DECLARED FAULT (9a), VOLUME TARGET (9b) OR UPGRADE (9c) WITH NO
// CAPABILITY BEHIND IT.
//
// LIVE AGAINST STATIC, which docs/contract.md:344 says is why neither side is
// written in the other's file: `[uses]` in bindings.toml is the project's ASK,
// and `describe`'s `capabilities` is what the adapter can actually do.
//
// AN EMPTY ARRAY IS AN ANSWER AND NOT AN OMISSION (contract.md:73): an adapter
// offering no faults makes a declared fault fire this arm, rather than making
// it vacuous. That is the opposite of the usual default and it is the point --
// the ask is refused because nothing can serve it.
func arm9(rep *Report, b bindings.Bindings, d wire.DescribeResponse) {
	rep.ran("9a")
	rep.ran("9b")
	rep.ran("9c")
	check := func(arm string, asked, offered []string, what string) {
		have := map[string]bool{}
		for _, o := range offered {
			have[o] = true
		}
		var bad []string
		for _, a := range asked {
			if !have[a] {
				bad = append(bad, a)
			}
		}
		if len(bad) > 0 {
			sort.Strings(bad)
			rep.refuse(arm, "[uses] asks for %s the adapter does not offer: %s -- its describe offers %d",
				what, strings.Join(bad, ", "), len(offered))
		}
	}
	check("9a", b.Uses.Faults, d.Capabilities.Faults, "fault(s)")
	check("9b", b.Uses.Volume, d.Capabilities.Volume, "volume target(s)")
	check("9c", b.Uses.Upgrade, d.Capabilities.Upgrade, "upgrade(s)")
}

// arm13: THE THREE STATEMENTS OF THE CONTRACT VERSION DISAGREE.
//
// internal/wire.CheckVersion ALREADY DOES THIS and names every value it saw
// rather than the pair that differed, because fable's D6 drill bumps exactly
// one of the three. Restating the comparison here would be a second copy to
// drift, so this arm calls it and reports what it said.
func arm13(rep *Report, p platform.Platform, d wire.DescribeResponse) {
	rep.ran("13")
	if err := wire.CheckVersion(d.Contract, p.Contract); err != nil {
		rep.refuse("13", "%v", err)
	}
}

// arm21: A ROLE OR ACTION CONTEXT THE ADAPTER DOES NOT DESCRIBE.
//
// docs/contract.md:222 says an `[[action]].context` is "one of `describe`'s
// contexts", and until this arm NOTHING COMPARED THE TWO. `describe`'s
// Contexts field was written by every adapter in the tree -- cmd/toy-adapter
// declares `{"api", "browser"}` and internal/fake carries its own -- and read
// by no production path at all: every `.Contexts` reader was
// bindings.Role.Contexts, the PROJECT's own declaration (quill, coop r8/140;
// the census verified at r8/144, where not even a test read the describe's).
// So the third declared-and-read-by-nothing of this child, and the one the
// CONTRACT states, gains its reader here.
//
// WHAT IT COSTS WITHOUT THE ARM, measured rather than argued: a project may
// declare any context string, and arms 4 and 5 both gate on `isBrowser`, so a
// role or action carrying a context no adapter serves makes those arms SKIP
// that row while looking correct -- docs/vocabulary.md's v4 row in its own
// words about `ui`. The rename needed four sweeps and a fifth for exactly that
// reason; with this pairing it would have been one refusal naming the file and
// the value.
//
// `none` IS EXEMPT AND THE REASON COVERS ONLY IT: the contract calls a context
// "a context name, or `none`" (:104, :372) and internal/schedule/pool.go:344
// says it in one line -- "CONTEXT \"none\" IS HELD BY EVERYBODY, because it is
// not a context". An adapter cannot offer the absence of a session, so
// comparing it against the describe would refuse every project with an action
// that needs none -- create_user in examples/toy, the first step of every walk.
//
// BOTH SIDES ARE CHECKED, roles and actions, because the contract's sentence is
// about actions and arm5's reachability is about roles: a context that reaches
// the adapter through either is one the adapter has to serve.
func arm21(rep *Report, b bindings.Bindings, d wire.DescribeResponse) {
	rep.ran("21")
	have := map[string]bool{}
	for _, c := range d.Contexts {
		have[c] = true
	}
	var bad []string
	for _, a := range b.Actions {
		if a.Context == "" || a.Context == "none" {
			continue
		}
		if !have[a.Context] {
			bad = append(bad, "action "+a.Name+" declares "+a.Context)
		}
	}
	for _, r := range b.Roles {
		for _, c := range r.Contexts {
			if c == "none" {
				continue
			}
			if !have[c] {
				bad = append(bad, "role "+r.Name+" holds "+c)
			}
		}
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		offered := "nothing"
		if len(d.Contexts) > 0 {
			offered = strings.Join(d.Contexts, ", ")
		}
		rep.refuse("21", "context(s) the adapter does not describe: %s -- its describe offers %s",
			strings.Join(bad, "; "), offered)
	}
}
