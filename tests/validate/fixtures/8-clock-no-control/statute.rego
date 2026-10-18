# FIXTURE for arm 8: the twin with one change -- may_publish reads
# input.clock, and bindings.toml declares no [clock] table.
# The permitted twin (R4.3, issue 64).
#
# WHY THIS PACKAGE EXISTS, and it is not a fourth ledger example: `rumor
# validate`'s arm 3 refuses an action with no precondition, and the R1 three
# cannot supply case material for it. Their statutes judge CLAIMS -- counts and
# a finding -- so a `requires` in their bindings could only point at something
# that is not a precondition, and adding one would make a ledger fixture
# pretend to be this one (fable's ruling, coop r4/45, on quill r4/37). So the
# twin carries a REAL precondition rule and arm 3's fixture is a copy of this
# package with one change, derived rather than invented.
#
# THE PRECONDITION IS `may_publish`, and it is a precondition in the sense
# `[[action]].requires` means: a condition on the state of the world that must
# hold BEFORE the action is taken. One publish per actor -- an actor who has
# already published a doc in the sealed prefix may not publish another. It is
# checkable from the ledger alone, which is what lets an action name it.
#
# NO print() ANYWHERE, per ADR 0001, and the harness enforces that rather than
# trusting it.
#
# SUB-RULES ARE NAMED so the platform can evaluate each alone. `held_claims`
# and `actor_has_published` are helpers and are deliberately NOT in rules.json:
# attaint's coverage is per rule in that file, and a helper whose every reading
# is already pinned through `believed` and `may_publish` would demand a mutant
# that only restates one of theirs.
package rumor.statute

# The claims the observer holds at the observation's position: the ones it made
# and the ones it was told, flattened by the engine and handed in as `known`.
held_claims contains c if {
	some c in input.claims
	c.ref in input.known
	c.position <= input.observation.position
}

# What the observer should believe the count to be.
believed := count({c.ref |
	some c in held_claims
	c.type == input.observation.of
})

# Whether the OBSERVING actor has already published a claim of the observed
# type in the sealed prefix. Over `input.claims` and not `held_claims`: a
# precondition is about what the ledger holds, not about what this observer was
# told, and an actor is not licensed to publish twice by not having been told
# of their own first one.
actor_has_published if {
	some c in input.claims
	c.actor == input.observation.actor
	c.type == input.observation.of
	c.position <= input.observation.position
}

# THE PRECONDITION. `[[action]].requires = ["may_publish"]` names this rule.
# DEFAULTED so it is always DEFINED: without the default, an undefined rule and
# a false one are the same answer to a caller, which is the
# absence-reads-as-a-verdict shape.
default may_publish := false

may_publish if {
	not actor_has_published
	input.clock.now > 0
}

# THE FINDING. Both outcomes are reachable from this package's own cases, which
# the vocabulary requires of an example: `miscount` is the bug and the other
# three are consistent.
default finding := "bug"

finding := "consistent" if input.observation.value == believed

# THE CORROBORATION CONTROL. True when the bug is attributable to a planted
# claim the observer holds -- the engine finding the lie rather than a defect.
# `rumor attaint` requires this of every planted lie in a fixture, and fable
# refused exempting the twin from that guard (coop r4/63): a package that
# skips a control is a special case waved through, so the twin EARNS it with a
# planted case of its own instead.
#
# DEFAULTED so it is always DEFINED: an undefined rule and a false one are the
# same answer to a caller.
default rests_on_planted := false

rests_on_planted if {
	finding == "bug"
	some c in held_claims
	c.planted
	c.type == input.observation.of
}
