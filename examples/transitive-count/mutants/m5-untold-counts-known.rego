# The transitive-count example (R1.2, issue 29).
#
# WHAT IT CHECKS: that a gap between what an actor SEES and what it KNOWS is
# `explained` when an untold claim accounts for it, and `bug` when nothing
# does. docs/vocabulary.md's own illustration -- Charlie, told only by Alice of
# the two users Alice and Bob created, observes 2 -- is the first case here.
#
# THE READING IS fable r1.2/5, after the v1 clause and its illustration were
# found to disagree (asked at r1.2/2, settled by quill r1.2/3):
#
#   `explained`  agrees with the WHOLE TRUTH, disagrees with the observer's
#                flattened knowledge, and the gap is ACCOUNTED FOR by claims
#                the observer was never told
#   `bug`        that same disagreement with no such account
#
# AND SEPARATING THEM NEEDS BOTH INPUTS, which is quill's deeper point and the
# reason `truth` exists as its own rule: the observer's knowledge alone cannot
# tell Charlie's 2 from Charlie's 3. Both disagree with what he knows; only one
# agrees with the ledger.
#
# NO print(), per ADR 0001, enforced by the harness on this and every mutant.
# Sub-rules are named so the platform can evaluate each alone.
package rumor.statute

# What the observer holds at the observation's position: claims it made plus
# claims it was told, flattened by the engine and handed in as `known` (R2).
held_claims contains c if {
	some c in input.claims
	c.ref in input.known
	c.position <= input.observation.position
}

believed := count({c.ref |
	some c in held_claims
	c.type == input.observation.of
})

# The whole truth in the sealed prefix: every honest claim, told or not.
truth := count({c.ref |
	some c in input.claims
	not c.planted
	c.position <= input.observation.position
	c.type == input.observation.of
})

# The claims that could account for a gap: honest, in the prefix, of the
# observed type, and NOT known to the observer. Named so the account can be
# read without asking for the finding.
untold_accounts := count({c.ref |
	some c in input.claims
	not c.planted
	c.position <= input.observation.position
	c.type == input.observation.of
	true  # MUTANT: untold counts claims the observer knows
})

default finding := "bug"

finding := "consistent" if input.observation.value == believed

finding := "explained" if {
	input.observation.value != believed
	input.observation.value == truth
	input.observation.value - believed == untold_accounts
}
