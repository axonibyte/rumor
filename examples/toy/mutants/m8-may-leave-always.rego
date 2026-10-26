# MUTANT: an actor may leave an event it never joined. cmd/toy answers 409 for exactly that, so the rule and the application would disagree about the same state.
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
#
# AND IT LANDED: fable chose the projection at coop r5/16, and R5.3's 226acbb
# ("engine: project a claim's handles") emits `handles` on a claim whenever the
# fact carries any -- internal/engine/engine.go, present only when non-empty.
# So the sentence that stood here until R6.2, "the engine does not pass them
# yet", was true when written on 2026-10-19 and false six days later, and the
# rules below still read as if there were one subject by their own choice
# rather than for want of the field.
#
# WHAT THAT LEAVES OPEN, said plainly so this comment is not the next stale
# one: the counts here do NOT filter on `handles`, which is safe only while
# every package in examples/ is single-subject, as this one is. A walk creating
# two events would need the filter, and nothing here refuses such a project --
# that remains R5.3's note and is not closed by the field arriving.

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

# AN OBSERVATION WITH NO ROLE IS NOT RESTRICTED, which is v1's conditional read
# exactly: "WHERE A PROJECT HAS ROLES, an observer's expectation is what it
# knows restricted to what its `role` may see". A `direct` observation is the
# engine's and never an actor's, so it bears no role and there is nothing to
# restrict by -- the second oracle reads the store, not a principal's view.
#
# `not input.observation.role` and not `== ""`: the engine emits the key only
# when the role is non-empty (internal/engine's prefixFacts), so for a direct
# observation the field is UNDEFINED rather than blank.
role_sees_subject if not input.observation.role

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

may_join if {
	knows_of_subject
	not already_joined
}

knows_of_subject if {
	some c in held_claims
	c.type == "event.created"
}

already_joined if {
	some c in held_claims
	c.type == "event.joined"
	c.actor == input.observation.actor
}

# THE OTHER FOUR PRECONDITIONS, one per remaining action, because validate's
# arm 3 refuses ANY action that declares none: "an action with no precondition
# is one the walk may take from any state". That is the arm doing its job rather
# than a nuisance -- a walk is a sequence of applicable actions, and an action
# applicable always is one the scheduler can never schedule WRONGLY, so it tests
# nothing about the world's state.
#
# EVERY ONE IS GROUNDED IN THE LEDGER, which is the constraint that shaped them:
# a rule naming something the ledger cannot say would be a precondition in name
# only. All four are defaulted, so an undefined rule and a false one are not the
# same answer to a caller.

# One user per actor, which is cmd/toy's own rule: it refuses a name already
# taken, and an actor that has made its user has no second one to make.
default may_create_user := false

may_create_user if not made_user

made_user if {
	some c in held_claims
	c.type == "user.created"
	c.actor == input.observation.actor
}

# THE SESSION ORDERING, which every `api` action but create_user requires.
#
# WHY IT IS A PRECONDITION AND NOT A 401. cmd/toy keys a session by user NAME,
# so an actor can speak through the API only after some create_user made a user
# of its own name. Driving seed 4242 to step 3, guest_1 was refused outright
# because it had no user (quill, coop r5/89). The walk could have caught that as
# a fault, but an action the actor cannot take is an action that was never
# applicable: applicability is where this belongs, and fable ruled it so at
# r5/92. The engine then never asks for a session an actor cannot hold.
#
# IT IS `made_user` READ ON THE PRECONDITION PATH, and a listed rule of its own
# rather than a second use of the helper, for two reasons that both come from
# other files: `requires` may only name a rule rules.json declares
# (internal/validate arm 19, cell 1), and the helpers are deliberately unlisted
# so attaint does not demand a mutant restating a listed rule's reading. This
# one is listed, so examples/toy/mutants carries m10 for it.
#
# DEFAULTED, like the five around it: an undefined rule and a false one must not
# be the same answer to the scheduler.
default has_user := false

has_user if made_user

# One event per actor. The same shape as the twin's `may_publish`, and for the
# same reason: a precondition has to be false SOMETIMES or the arm that reads it
# is decoration.
default may_create_event := false

may_create_event if not made_event

made_event if {
	some c in held_claims
	c.type == "event.created"
	c.actor == input.observation.actor
}

# Leaving needs a join to leave, which is `already_joined` read the other way.
default may_leave := false

may_leave if true

# Deleting is the admin's, and only once: the actor made the event and nothing
# has deleted it yet. cmd/toy enforces both -- 403 for a non-admin, 404 for an
# event already gone.
default may_delete := false

may_delete if {
	made_event
	not deleted
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
