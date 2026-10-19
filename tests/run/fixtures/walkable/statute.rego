# The smallest statute a walk can be scheduled against.
#
# IT IS ITS OWN AND NOT A COPY OF THE TOY'S, deliberately: a copied statute
# drifts from the original the moment the original learns something, and this
# fixture is about the `run` verb rather than about any rule. Two preconditions
# are enough to make the walk's order observable -- one true from the empty
# world so a run can start, and one that only becomes true after a creation, so
# the pool is not the same set at every step.
package rumor.statute

# An event can always be created: a run has to be able to take a first step
# from a world where nothing has happened.
default may_create_event := true

# An event can be joined only once one exists in the sealed prefix, which is
# the ledger-expressible half of "an actor can join only an event it knows of".
default may_join := false

may_join if {
	some c in input.claims
	c.type == "event.created"
	c.position <= input.observation.position
}
