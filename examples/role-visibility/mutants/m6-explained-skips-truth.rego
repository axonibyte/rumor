# The role-visibility example (R1.2, issue 29).
#
# WHAT IT CHECKS: that an observation is judged against what the observer
# should be able to SEE, not merely against what it knows. Knowing of a thing
# and being permitted to observe it are different, and a statute that conflates
# them reports a correctly hidden record as a missing one.
#
# WHERE THE VISIBILITY COMES FROM, said plainly because vocabulary v1 does not
# define roles: it is carried on the claim as `visible_to`, a list of roles, and
# on the observation as the observer's `role`. v1 fixes the four findings, the
# fact kinds and the three observation sources; it says nothing about roles, so
# this example does not borrow a word it has not been given. When a later phase
# defines them, this statute's `visible_to_observer` is the single rule that
# changes.
#
# AND THE LEAK CASE IS `bug` ON PURPOSE. v1 NAMES `trespass` for R18 -- an
# action the statute forbids that succeeds -- without defining it, and tells the
# examples to avoid the word. A role seeing what it may not see is reported here
# as `bug`, which is what v1 gives me; when R18 lands, that case is the natural
# first `trespass` and this comment is the pointer to it.
#
# NO print(), per ADR 0001, enforced by the harness on this and every mutant.
package rumor.statute

# The claims the observer holds at the observation's position.
held_claims contains c if {
	some c in input.claims
	c.ref in input.known
	c.position <= input.observation.position
}

# Of those, the ones its role is permitted to observe. THIS is the rule the
# example exists for: holding a claim and being able to see it are separate.
visible_to_observer contains c if {
	some c in held_claims
	input.observation.role in c.visible_to
}

# What the observer should expect the query to return: the held claims its role
# may see. An actor told of a record it may not see still KNOWS of it and should
# not expect to count it.
believed_visible := count({c.ref |
	some c in visible_to_observer
	c.type == input.observation.of
})

# Everything in the sealed prefix the observer's role may see, told or not.
# Separate from `believed_visible` for the same reason transitive-count keeps
# `truth` apart: the knowledge and the ledger are two inputs and one cannot
# stand in for the other.
visible_truth := count({c.ref |
	some c in input.claims
	not c.planted
	c.position <= input.observation.position
	c.type == input.observation.of
	input.observation.role in c.visible_to
})

default finding := "bug"

finding := "consistent" if input.observation.value == believed_visible

# The gap is accounted for when every claim the observer may see but was not
# told of covers it exactly -- the transitive-count account, narrowed to what
# the role can see.
finding := "explained" if {
	input.observation.value != believed_visible
	true  # MUTANT: the ledger is not consulted
	input.observation.value - believed_visible == untold_visible
}

untold_visible := count({c.ref |
	some c in input.claims
	not c.planted
	c.position <= input.observation.position
	c.type == input.observation.of
	input.observation.role in c.visible_to
	not c.ref in input.known
})
