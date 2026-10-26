#!/usr/bin/env python3
"""Regenerate examples/toy/mutants/ as the statute plus ONE declared change each.

WHY IT EXISTS, measured rather than feared. Every mutant carries a header
saying "One change against examples/toy/statute.rego and nothing else, which is
what makes `rumor attaint`'s kill attributable to the rule it names" -- and on
2026-10-26 NINE of the ten were false. Each was cut from the statute as it
stood at the time and never re-cut, so they had drifted: all nine lacked
`role_sees_subject if not input.observation.role`, and m1, m2, m3, m4 and m5
lacked the whole may_create_user/has_user/may_create_event/may_leave/may_delete
precondition block as well.

WHAT THAT COST, from attaint's own output on the committed corpus:

    toy  believed          killed by m1-believed-ignores-visibility
    toy  finding           killed by m1-believed-ignores-visibility
    toy  has_user          killed by m1-believed-ignores-visibility
    toy  may_create_event  killed by m1-believed-ignores-visibility
    toy  may_create_user   killed by m1-believed-ignores-visibility
    toy  may_delete        killed by m1-believed-ignores-visibility
    toy  may_leave         killed by m1-believed-ignores-visibility
    toy  role_sees_subject killed by m1-believed-ignores-visibility

ONE MUTANT CREDITED WITH EIGHT KILLS, seven of them for rules that have mutants
of their own. "10 rule(s) killed, 0 unmutated" was green on that, so the figure
measured a corpus that could not say which mutation killed what -- a mutant
missing a rule kills it by ABSENCE, which is not the deviation the fixture is
supposed to assert. fable named four of these at coop r5/170 (m1, m2, m4, m10)
from the stale comment they carried; the class measured out at nine, and m10 --
the only honest one -- was in the handed list while m3, m5, m6, m7, m8 and m9
were not.

SO THE MUTANTS ARE GENERATED AND NOT MAINTAINED, the same answer
tools/make-validate-fixtures.py gives for the validate fixtures: a hand-cut
copy of a moving file is stale from its second edit. Each entry below is the
mutant's OWN change and nothing else, and each `old` is asserted UNIQUE in the
statute so a rename cannot silently mutate a different rule.

THE HEADER IS PRESERVED, not regenerated: it states what the mutation means and
why, which is the mutant author's prose and not derivable from the diff.
"""
import os
import sys

STATUTE = "examples/toy/statute.rego"
MUTANTS = "examples/toy/mutants"

# name -> (old, new). ONE change each, `old` unique in the statute.
MUTATIONS = {
    "m1-believed-ignores-visibility": (
        "believed := count(joined_by_observer) - count(left_by_observer) if role_sees_subject",
        "believed := count(joined_by_observer) - count(left_by_observer)"),
    "m2-finding-ignores-delete": (
        'finding := "consistent" if {\n\tnot deleted\n\tinput.observation.value == believed\n}',
        'finding := "consistent" if {\n\tinput.observation.value == believed\n}'),
    "m3-may-join-always": (
        "may_join if {\n\tknows_of_subject\n\tnot already_joined\n}",
        "may_join if true"),
    "m4-control-always": (
        'rests_on_planted if {\n\tfinding == "bug"\n\tsome c in held_claims\n\tc.planted\n}',
        'rests_on_planted if finding == "bug"'),
    "m5-role-sees-everything": (
        "default role_sees_subject := false",
        "default role_sees_subject := true"),
    "m6-may-create-user-always": (
        "may_create_user if not made_user",
        "may_create_user if true"),
    "m7-may-create-event-always": (
        "may_create_event if not made_event",
        "may_create_event if true"),
    "m8-may-leave-always": (
        "may_leave if already_joined",
        "may_leave if true"),
    "m9-may-delete-ignores-delete": (
        "may_delete if {\n\tmade_event\n\tnot deleted\n}",
        "may_delete if made_event"),
    "m10-has-user-always": (
        "has_user if made_user",
        "has_user if true"),
}


def main():
    statute = open(STATUTE).read()
    anchor = statute.split("\n", 1)[0]
    if not anchor.startswith("#"):
        sys.exit("the statute's first line is not a comment, so the header split is wrong")
    for name, (old, new) in sorted(MUTATIONS.items()):
        path = os.path.join(MUTANTS, name + ".rego")
        if not os.path.exists(path):
            sys.exit("no mutant at %s: this tool regenerates, it does not invent" % path)
        current = open(path).read()
        cut = current.find(anchor)
        if cut < 1:
            sys.exit("%s does not contain the statute's first line, so its header "
                     "cannot be told from its body" % name)
        header = current[:cut]
        n = statute.count(old)
        if n != 1:
            sys.exit("%s: its `old` occurs %d time(s) in the statute, not once -- a "
                     "non-unique anchor would mutate a rule this mutant is not about"
                     % (name, n))
        open(path, "w").write(header + statute.replace(old, new))
        print("regenerated %s" % name)
    print("%d mutant(s), each the statute plus one change" % len(MUTATIONS))


if __name__ == "__main__":
    main()
