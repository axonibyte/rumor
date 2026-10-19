# MUTANT: the precondition holds for everyone, including an actor who has already joined. `[[action]].requires` would then name a rule that permits what the application refuses.
#
# One change against examples/toy/statute.rego and nothing else, which is what
# makes `rumor attaint`'s kill attributable to the rule it names.
# The toy system under test (R5.2, issue 73).
#
# WHY THIS PACKAGE EXISTS. The other four examples are fixtures: their cases
# were authored to exercise the engine. This one is the LAW OF A RUNNING
# APPLICATION -- cmd/toy -- so every rule has to be a thing the statute can say
# about what an actor saw through that application's own API, from facts the
# ledger can carry.
#
# WHAT THE LEDGER CAN CARRY DECIDED TWO RULES OUT OF THIS FILE, and the reason
# is written here rather than left as a gap:
#
#   1 THERE IS NO CAPACITY FIELD. internal/ledger's claim carries type, ref,
#     handles, expect, planted and visible_to, and nothing else; `expect` is
#     "the actor's expectation of what a later observation should show"
#     (docs/vocabulary.md) and co-opting it for a declared capacity would be a
#     misuse of a field the three R1 examples deliberately leave unused. So an
#     event's capacity is not a ledger fact, a capacity OVERRUN is not nameable
#     from a ledger case, and this statute does not pretend otherwise. That is
#     the data model agreeing with fable's ruling at coop breakout-r5/1: R5
#     PLANTS the capacity race and R8's `concurrency` stage catches it. The
#     defect lives in cmd/toy with a by-hand test, not here.
#   2 A LEAK IS NOT A FINDING OF ITS OWN. docs/vocabulary.md:80 fixes v1's
#     reading: "an observer's expectation is what it knows restricted to what
#     its `role` may see", and "a leak is a `bug` only when it MOVES the
#     observed value; a leak that preserves the value is invisible to v1 and is
#     the `trespass` of R18". So `role_sees_subject` restricts the expectation
#     and does not produce a verdict, and the API visibility leak surfaces the
#     way v1 says it must: as a value the observer's role cannot account for.
#
# NO print() ANYWHERE, per ADR 0001.
#
# THE PRECONDITION IS `may_join`, in the sense `[[action]].requires` means: a
# condition on the world that must hold BEFORE the action is taken, checkable
# from the ledger alone. An actor may join an event it knows of and has not
# already joined. cmd/toy enforces both, plus the capacity check that is not a
# ledger fact -- so the rule is the ledger-expressible half of the application's
# own precondition, which is all a statute can hold it to.
#
# SUB-RULES ARE NAMED so the platform can evaluate each alone, and the helpers
# are deliberately NOT in rules.json: attaint's coverage is per rule in that
# file, and a helper whose every reading is already pinned through a listed rule
# would demand a mutant that only restates one of theirs.
package rumor.statute

# THERE IS NO SUBJECT HERE, AND THAT IS THE ENGINE'S DOING RATHER THAN A
# SIMPLIFICATION. A statute sees each claim as `{ref, actor, type, position}`
# plus `planted` and `visible_to` when present -- internal/engine's
# prefixFacts, which builds the input -- and it does NOT pass `handles`, though
# internal/ledger carries them on every claim. So no rule can ask which entity
# a claim is about: the only identifying field a statute gets is the claim's
# own ref.
#
# EVERY PACKAGE IN examples/ IS THEREFORE SINGLE-SUBJECT, and this one is too:
# one event per case, so "the event" is unambiguous and the counts below need no
# filter. The observation's query still carries `event_count/<handle>` because
# that is what a run records and what the adapter needs; this statute cannot
# read a subject out of it and does not pretend to.
#
# IT IS A LIMIT ON RUNS, NOT ON CASES, which is why it is reported in coop
# r5.toy rather than worked around: a walk that creates two events writes joins
# for both into one ledger, and a statute that cannot tell them apart would
# count across both. R5.3 needs `handles` in the projection, or an observation
# that names its subject in a field of its own.

# The claims the observer holds at the observation's position: the ones it made
# and the ones it was told, flattened by the engine and handed in as `known`.
held_claims contains c if {
	some c in input.claims
	c.ref in input.known
	c.position <= input.observation.position
}

# Every claim in the sealed prefix, told or not: the whole truth the observer is
# measured against, as distinct from what it believes.
prefix_claims contains c if {
	some c in input.claims
	c.position <= input.observation.position
}

# WHETHER THE OBSERVER'S ROLE MAY SEE THE SUBJECT AT ALL, off the creation
# claim's `visible_to` and the observation's `role` -- the two fields v1 puts
# this question in. It RESTRICTS the expectation below and is never a verdict on
# its own.
#
# DEFAULTED TO FALSE: an event whose creation claim names no role of the
# observer's is not one the observer has been shown, and a default of true would
# read an absence as a permission.
default role_sees_subject := false

role_sees_subject if {
	some c in prefix_claims
	c.type == "event.created"
	input.observation.role in c.visible_to
}

joined_by_observer contains c.ref if {
	some c in held_claims
	c.type == "event.joined"
}

left_by_observer contains c.ref if {
	some c in held_claims
	c.type == "event.left"
}

joined_in_prefix contains c.ref if {
	some c in prefix_claims
	c.type == "event.joined"
}

left_in_prefix contains c.ref if {
	some c in prefix_claims
	c.type == "event.left"
}

# WHAT THE OBSERVER SHOULD BELIEVE: the joins it holds, less the leaves it
# holds, RESTRICTED to a subject its role may see. An observer whose role may
# not see the event should believe nothing about it -- which is what makes a
# leak move the value rather than pass unnoticed.
default believed := 0

believed := count(joined_by_observer) - count(left_by_observer) if role_sees_subject

# WHAT THE WHOLE TRUTH SUPPORTS, under the same restriction. `explained` is
# measured against this and not against the unrestricted count: a gap the
# observer was never TOLD about is explained, and a gap it may not SEE is not.
# Without the restriction here, the visibility leak would read as `explained`,
# which is the error this rule exists to prevent.
default visible_truth := 0

visible_truth := count(joined_in_prefix) - count(left_in_prefix) if role_sees_subject

# A DELETED SUBJECT, from the claims: the stale-count row is exactly the
# disagreement between a delete the ledger carries and a count the application
# still reports.
deleted if {
	some c in prefix_claims
	c.type == "event.deleted"
}

# THE PRECONDITION. `[[action]].requires = ["may_join"]` names this rule.
# DEFAULTED so it is always DEFINED: without the default, an undefined rule and
# a false one are the same answer to a caller.
default may_join := false

may_join if true

knows_of_subject if {
	some c in held_claims
	c.type == "event.created"
}

already_joined if {
	some c in held_claims
	c.type == "event.joined"
	c.actor == input.observation.actor
}

# THE FINDING. Every outcome below is reachable from this package's own cases,
# which the vocabulary requires of an example, and `bug` is reached by the two
# ledger rows a run can catch in R5: the visibility leak and the stale count.
default finding := "bug"

finding := "consistent" if {
	not deleted
	input.observation.value == believed
}

# THE UNTOLD CLAIMS BALANCE IT: the observer saw more than it believes, and the
# gap is exactly the joins it was never told of -- not a defect, which is
# docs/vocabulary.md's `explained`.
finding := "explained" if {
	not deleted
	input.observation.value != believed
	input.observation.value == visible_truth
}

# THE CORROBORATION CONTROL. True when the bug is attributable to a planted
# claim the observer holds -- the engine finding the lie rather than a defect.
# `rumor attaint` requires this of every planted lie in a fixture, and fable
# refused exempting a package from that guard (coop r4/63), so this package
# EARNS it with a planted case of its own.
#
# DEFAULTED so it is always DEFINED.
default rests_on_planted := false

rests_on_planted if {
	finding == "bug"
	some c in held_claims
	c.planted
}
