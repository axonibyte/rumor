# The planted-lie example (R1.2, issue 29).
#
# WHAT IT CHECKS, and it is a check on the ENGINE and not on any system under
# test (docs/vocabulary.md, `planted`): a `bug` whose derivation rests on a
# planted claim is the CONTROL PASSING -- the engine found the lie. The
# vocabulary requires both outcomes from one example: the honest-only set is
# `consistent` and the set carrying the planted claim is `bug`.
#
# NO print() ANYWHERE, per ADR 0001 (docs/adr/0001-engine.md): a statute
# carries no diagnostic scaffolding. The harness enforces it rather than
# trusting it, with two oracles: a static scan of the parsed module, which
# sees a print on a path no case reaches, and a hook on the real evaluator
# under rego.EnablePrintStatements -- without that flag OPA erases print
# calls and a hook alone reports zero for a statute full of them.
#
# SUB-RULES ARE NAMED so the platform can evaluate each alone: `held_claims`,
# `believed`, `honest_total` and `rests_on_planted` are each a rule the engine
# can ask for by name without asking for `finding`.
#
# `explained` IS DELIBERATELY ABSENT. Its definition and its own illustration
# disagree in vocabulary v1 (asked at r1.2/2) and this example does not need
# it, so nothing here rests on the unsettled reading.
package rumor.statute

# The claims the observer holds at the observation's position: the ones it made
# and the ones it was told, flattened by the engine and handed in as `known`
# (R2 does the flattening in Go, internal/flatten; rules never follow tells).
held_claims contains c if {
	some c in input.claims
	c.ref in input.known
	c.position <= input.observation.position
}

# What the observer should believe the count to be. A planted claim the actor
# made itself IS believed by that actor -- that is what makes the lie bite.
believed := count({c.ref |
	some c in held_claims
	c.type == input.observation.of
})

# Every honest claim in the sealed prefix, regardless of who knows it. Named so
# the strip half of the control can be read without `finding`.
honest_total := count({c.ref |
	some c in input.claims
	# DRILL D2: `not c.planted` was REMOVED here, so a planted claim is
	# counted as honest -- the m5 mutant's shape, applied to the statute.
	c.position <= input.observation.position
	c.type == input.observation.of
})

# THE FINDING. Two of the four v1 findings are reachable here by construction:
# this example declares no `eventual` type, so `pending` cannot arise, and
# `explained` is out of scope above.
default finding := "bug"

finding := "consistent" if input.observation.value == believed

# THE CONTROL. True when the bug is attributable to a planted claim the
# observer holds -- the engine finding the lie rather than a defect. `rumor
# attaint` will require this of every planted lie in a fixture (R3).
# DEFAULTED so it is always DEFINED. Without the default an undefined rule and
# a false one are the same answer to a caller, which is the absence-reads-as-a-
# verdict shape; the platform can now ask for this alone and get true or false.
default rests_on_planted := false

rests_on_planted if {
	finding == "bug"
	some c in held_claims
	c.planted
	c.type == input.observation.of
}
