package validate

import (
	"context"
	"sort"
	"strings"

	"forge.axonibyte.com/axonibyte/rumor/internal/bindings"
	"forge.axonibyte.com/axonibyte/rumor/internal/platform"
	"forge.axonibyte.com/axonibyte/rumor/internal/wire"
)

// live asks the adapter to describe itself, once, for the four arms that cannot
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
	if err := wire.CheckVersion(d.Contract, p.Contract); err != nil {
		rep.refuse("13", "%v", err)
	}
}
