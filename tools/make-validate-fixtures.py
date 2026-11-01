#!/usr/bin/env python3
"""Generate tests/validate/fixtures/ from examples/twin.

EVERY FIXTURE IS THE PERMITTED TWIN WITH ONE CHANGE, and generating them is
what keeps that true. Hand-maintained copies go stale the moment the twin gains
a file: when the twin earned its seeds.lock, six fixtures predating it began
tripping arm 10a as well as their own, and the arm-isolation invariant broke
without anyone editing a fixture.

Run after ANY change to examples/twin, and tests/validate asserts the result --
see TestEveryFixtureIsTheTwinPlusItsOwnChange, which fails if a fixture differs
from the twin anywhere its table does not allow.
"""
import json, os, re, shutil, sys

SRC = "examples/twin"
ROOT = "tests/validate/fixtures"


def contract_version():
    """internal/wire's ContractVersion, read rather than remembered."""
    # The repo root, which is this script's cwd: ROOT is relative to it, and
    # walking up from ROOT was off by one level on the first cut.
    src = open(os.path.join("internal", "wire", "wire.go")).read()
    m = re.search(r'const ContractVersion = "([^"]+)"', src)
    if not m:
        raise SystemExit("internal/wire/wire.go does not declare ContractVersion")
    return m.group(1)


def fresh(arm):
    d = os.path.join(ROOT, arm)
    shutil.rmtree(d, ignore_errors=True)
    shutil.copytree(SRC, d)
    return d


def head(path, note):
    s = open(path).read()
    open(path, "w").write(note + s)


def case(path, fn, why):
    c = json.load(open(path))
    fn(c)
    c["why"] = why
    json.dump(c, open(path, "w"), indent=2)
    open(path, "a").write("\n")


def build():
    os.makedirs(ROOT, exist_ok=True)

    d = fresh("1-type-not-emitted")
    case(os.path.join(d, "cases", "fresh-publisher.json"),
         lambda c: c["input"]["observation"].__setitem__("of", "doc.archived"),
         "FIXTURE for arm 1: the twin with one change -- this case is judged on `doc.archived`, which no action emits.")

    d = fresh("2-type-not-described")
    head(os.path.join(d, "bindings.toml"),
         "# FIXTURE for arm 2: the twin with one change -- publish_doc also emits\n# `doc.archived`, which no [claim.<type>] table describes.\n")
    p = os.path.join(d, "bindings.toml"); s = open(p).read()
    open(p, "w").write(s.replace('emits = ["doc.published"]', 'emits = ["doc.published", "doc.archived"]', 1))

    d = fresh("3-no-precondition")
    head(os.path.join(d, "bindings.toml"),
         "# FIXTURE for arm 3: the twin with one change -- `requires` is gone, so the\n# action has no precondition.\n")
    p = os.path.join(d, "bindings.toml"); s = open(p).read()
    open(p, "w").write(s.replace('requires = ["may_publish"]\n', '', 1))

    d = fresh("7a-eventual-no-bound")
    head(os.path.join(d, "bindings.toml"),
         "# FIXTURE for arm 7a: the twin with one change -- doc.published settles\n# EVENTUALLY and declares no bound.\n")
    p = os.path.join(d, "bindings.toml"); s = open(p).read()
    open(p, "w").write(s.replace('timing = "immediate"', 'timing = "eventual"', 1))

    d = fresh("8-clock-no-control")
    head(os.path.join(d, "statute.rego"),
         "# FIXTURE for arm 8: the twin with one change -- may_publish reads\n# input.clock, and bindings.toml declares no [clock] table.\n")
    p = os.path.join(d, "statute.rego"); s = open(p).read()
    open(p, "w").write(s.replace("may_publish if not actor_has_published",
                                 "may_publish if {\n\tnot actor_has_published\n\tinput.clock.now > 0\n}", 1))

    # ARM 20's FIXTURE: the twin with one change -- a `[deadline]` table and no
    # `[clock]` one. The twin declares neither, so the ONE edit is the deadline
    # table; the absence that pairs with it is the twin's own and needs no edit,
    # which is what makes this one change rather than two (fable's rule, coop
    # r4/74).
    d = fresh("20-deadline-no-clock")
    head(os.path.join(d, "bindings.toml"),
         "# FIXTURE for arm 20: the twin with one change -- a [deadline] table,\n"
         "# against the twin's own absence of a [clock] one.\n")
    with open(os.path.join(d, "bindings.toml"), "a") as fh:
        fh.write('\n[deadline]\n# TYPED as of R7.4: each name is an OBSERVATION whose value is an instant.\n'
                 '# `doc.count` is the twin\'s own and only observation, and naming it is\n'
                 '# what keeps this fixture ONE change -- a declared deadline against the\n'
                 '# twin\'s absence of a [clock] table. The PAIRING of these names\n'
                 '# against the declared observations is validate\'s to refuse and is\n'
                 '# NOT armed yet, which is said rather than implied: naming a real\n'
                 '# observation is what keeps this fixture from tripping that arm when\n'
                 '# it lands, and a fixture tripping two arms shows neither. The\n'
                 '# table\'s CONTENTS are arm 20\'s condition, so what this key MEANS\n'
                 '# is not read here (fable, coop rumor/1359).\n'
                 'instants = ["doc.count"]\n')

    # ARM 19's CELL 7 FIXTURE: a `[deadline].instants` name no observation
    # declares. TWO TABLES ARE ADDED AND THAT IS ONE CHANGE, with the reason
    # exactly as wide as what it covers: the DEFECT is the dangling name, and
    # `[clock]` is the PRECONDITION that keeps arm 20 quiet -- a `[deadline]`
    # with contents in a project with no clock control is arm 20's refusal
    # (fable, coop rumor/1359), so without the clock table this fixture would
    # trip two arms and show neither. `faked` is the form that asks nothing of
    # an adapter, which is right for a fixture validate never walks.
    d = fresh("19/deadline-instant-unknown-observation")
    head(os.path.join(d, "bindings.toml"),
         "# FIXTURE for arm 19 cell 7: the twin with one change -- [deadline]\n"
         "# names an instant no [[observation]] declares. The [clock] table\n"
         "# beside it is the precondition that keeps arm 20 quiet, not a\n"
         "# second defect.\n")
    with open(os.path.join(d, "bindings.toml"), "a") as fh:
        fh.write('\n[clock]\nform = "faked"\n'
                 '\n[deadline]\ninstants = ["doc.expiry"]\n')

    d = fresh("10a-no-seeds-file")
    os.remove(os.path.join(d, "seeds.lock"))
    open(os.path.join(d, "WHY-no-seeds-file.md"), "w").write(
        "# FIXTURE for arm 10a\n\nThe twin with one change: `seeds.lock` is removed. platform.toml does not\n"
        "name `seeds_file`, so it defaults to that path (internal/platform:164) and\n"
        "the arm fires on the default too -- arms_platform.go says why that is not\nnarrowed.\n")

    d = fresh("11-no-mutants")
    shutil.rmtree(os.path.join(d, "mutants"))
    open(os.path.join(d, "WHY-no-mutants.md"), "w").write(
        "# FIXTURE for arm 11\n\nThe twin with one change: `mutants/` is removed. rules.json still declares\n"
        "four rules, so four are left without a mutation fixture.\n\n"
        "Arm 11 is the STRUCTURAL half only (fable, coop r4/69): whether a KILLING\n"
        "mutant exists per rule is attaint's question, answerable only by running them.\n")

    d = fresh("12a-no-position")
    case(os.path.join(d, "cases", "miscount.json"),
         lambda c: c["input"]["observation"].pop("position", None),
         "FIXTURE for arm 12a: the twin with one change -- this case's observation declares no position.")

    d = fresh("12b-no-source")
    case(os.path.join(d, "cases", "untold-claim.json"),
         lambda c: c["input"]["observation"].pop("source", None),
         "FIXTURE for arm 12b: the twin with one change -- this case's observation declares no source.")


    # ---- arms whose CONDITION has a PRECONDITION ------------------------------
    # fable, coop r4/74: "one change" is one CONDITION, not one byte. Where the
    # refusing condition needs the ground prepared -- the browser stage ON before
    # a flow can be missing -- the enabling edit is SETUP and the removing edit is
    # the CONDITION, and a setup edit that trips an arm by itself would mean two
    # fixtures rather than one. Each README below names which edit is which.

    d = fresh("4-no-ui-flow")
    p = os.path.join(d, "bindings.toml"); s = open(p).read()
    # the twin's browser action keeps its name, its context and its precondition
    # and loses its flow: ONE change, and no setup edit is needed now that the
    # twin carries the browser stage and the action itself (fable's D9 required
    # that, and it made this fixture simpler rather than harder).
    s = re.sub(r"\n  \[\[action\.flow\]\]\n(?:  [a-z]+ = [^\n]*\n)+", "\n", s)
    open(p, "w").write("# FIXTURE for arm 4: the twin with one change -- publish_doc_ui loses its UI\n"
                       "# flow, so a browser-stage action has nothing for the driver to execute.\n" + s)
    open(os.path.join(d, "WHY-no-ui-flow.md"), "w").write(
        "# FIXTURE for arm 4\n\nONE CHANGE, no setup: the twin already enables the browser stage and declares\n"
        "`publish_doc_ui` with a flow, because fable's D9 drill requires the untouched\n"
        "examples to pass with every arm EVALUATED and none skipped. So this fixture\n"
        "only removes the flow.\n")

    d = fresh("5-browser-reaches-api")
    p = os.path.join(d, "bindings.toml"); s = open(p).read()
    open(p, "w").write(s + '''
# THE CONDITION: an actor holding BOTH a browser session and the api context,
# so it could reach publish_doc from the browser stage. docs/contract.md:69 --
# "could reach" is "its context is one that actor holds a session in".
#
# THE NAME IS THE VENIRE'S OWN, and that is arm 19 talking: it was `editor`
# until arm 19 landed, and a project declaring SOME role while its [[venire]]
# draws another is exactly arm 19's venire cell -- so this fixture tripped two
# arms and the isolation invariant said so. `author` is the role
# platform.toml already draws, so the only defect here is the reachability.
[[role]]
name = "author"
contexts = ["browser", "api"]
''')
    open(os.path.join(d, "WHY-browser-reaches-api.md"), "w").write(
        "# FIXTURE for arm 5\n\nONE CHANGE, no setup: the twin already enables the browser stage. This adds a\n"
        "role holding both `browser` and `api`, so a browser-stage actor could take the\n"
        "api action the twin declares.\n\n"
        "The role is deliberately not in the twin: tests/contract's agreement test\nrefuses a declared role no case carries, and the twin's cases carry none.\n\n"
        "It is named `author` -- the role platform.toml's venire already draws --\nbecause arm 19's venire cell refuses a project that declares some role while\n"
        "drawing another, and with any other name this fixture would trip two arms.\n")

    d = fresh("7b-quiesce-no-probe")
    p = os.path.join(d, "bindings.toml"); s = open(p).read()
    s = s.replace('timing = "immediate"', 'timing = "eventual"\nbound = "quiesce"', 1)
    open(p, "w").write("# FIXTURE for arm 7b.\n#\n# SETUP: doc.published settles EVENTUALLY rather than immediately, without\n# which a bound says nothing (bindings.go refuses a bound on an immediate type).\n#\n# CONDITION: the bound is `quiesce` and no [quiesce] probe is declared.\n" + s)

    # 7c AND 7d ADD THE TABLE THE TWIN DOES NOT HAVE. The twin declares no
    # [quiesce] at all, so the table is the SETUP and the one field inside it is
    # the condition -- fable's r4/74 rule. 7d's probe carries a timeout it does
    # not need, so that it cannot also trip 7c; 7c's carries a known kind, so it
    # cannot also trip 7d. Neither trips 7b: the twin's doc.published is
    # immediate, so no claim is bounded on quiesce.
    d = fresh("7c-probe-without-a-timeout")
    p = os.path.join(d, "bindings.toml"); s = open(p).read()
    open(p, "w").write(
        "# FIXTURE for arm 7c.\n#\n# SETUP: the twin, plus a [quiesce] table declaring one probe.\n#\n"
        "# CONDITION: the probe declares no `timeout`. bindings.go reads `timeout` with\n"
        "# required=false DELIBERATELY -- required at parse, the file is refused as\n"
        "# malformed and this arm's refusal is unreachable, which was measured before\n"
        "# this fixture existed.\n"
        + s.rstrip("\n") + '\n\n[quiesce]\n\n  [[quiesce.probe]]\n  kind = "queue_depth"\n  quiet = 0\n')

    d = fresh("7d-probe-of-an-unknown-kind")
    p = os.path.join(d, "bindings.toml"); s = open(p).read()
    open(p, "w").write(
        "# FIXTURE for arm 7d.\n#\n# SETUP: the twin, plus a [quiesce] table declaring one probe WITH a timeout,\n"
        "# so 7c cannot also fire -- one fixture trips exactly its own arm.\n#\n"
        "# CONDITION: `kind` is a name no adapter reports. The parser requires the KEY\n"
        "# and validate owns its VALUE, so an unknown kind reaches here rather than\n"
        "# being refused as a malformed file.\n"
        + s.rstrip("\n") + '\n\n[quiesce]\n\n  [[quiesce.probe]]\n  kind = "stares_at_it"\n  timeout = "30s"\n  quiet = 0\n')

    # 7e CARRIES A TIMEOUT AND A KNOWN KIND so it trips neither 7c nor 7d, and
    # its kind is deliberately one of the two that do NOT read an observation.
    d = fresh("7e-probe-query-on-a-wrong-kind")
    p = os.path.join(d, "bindings.toml"); s = open(p).read()
    open(p, "w").write(
        "# FIXTURE for arm 7e.\n#\n# SETUP: the twin, plus a [quiesce] table declaring one probe that is\n"
        "# well-formed for 7c and 7d -- it has a timeout and a known kind.\n#\n"
        "# CONDITION: that kind is `worker_idle`, which the adapter REPORTS, and the\n"
        "# probe also declares `query`, which only the `direct` kind reads. bindings.go\n"
        "# said this was refused long before anything refused it.\n"
        + s.rstrip("\n") + '\n\n[quiesce]\n\n  [[quiesce.probe]]\n  kind = "worker_idle"\n'
        '  timeout = "30s"\n  query = "item.count"\n  quiet = true\n')

    d = fresh("6-component-uncovered")
    p = os.path.join(d, "platform.toml"); s = open(p).read()
    s = s.replace('covers = ["doc_store"]', 'covers = ["some_other_store"]', 1)
    open(p, "w").write("# FIXTURE for arm 6: the twin with one change -- the rollback provider covers\n# a component nothing names, leaving doc_store uncovered.\n" + s)

    d = fresh("10b-unreadable-seed")
    p = os.path.join(d, "seeds.lock")
    open(p, "a").write("not-a-seed  # FIXTURE for arm 10b: the twin with one change --\n"
                       "# this line is neither blank, nor a comment, nor a non-negative integer,\n"
                       "# so it is a promoted seed a replay would skip.\n")

    # ---- the live arms: the ask is changed, never the adapter ----------------
    # Each of these asks for a capability the twin's own surface.json does not
    # offer. The SURFACE is left alone on purpose: changing what the adapter can
    # do would test the fake adapter, and the arm is about the project's ASK.
    for arm, key, want in (("9a", "faults", "partition"),
                           ("9b", "volume", "enormous"),
                           ("9c", "upgrade", "v1-to-v9")):
        d = fresh("%s-%s-unavailable" % (arm, key.rstrip("s")))
        p = os.path.join(d, "bindings.toml"); s = open(p).read()
        s = s.replace('%s = ["%s"]' % (key, {"faults": "latency", "volume": "small",
                                             "upgrade": "v1-to-v1"}[key]),
                      '%s = ["%s"]' % (key, want), 1)
        open(p, "w").write("# FIXTURE for arm %s: the twin with one change -- [uses] asks for a\n"
                           "# %s the adapter's describe does not offer.\n" % (arm, key.rstrip("s")) + s)

    # ---- arm 19's cells: a name the project uses and does not declare -------
    # ONE ARM, SEVERAL CELLS (fable, coop r4/113), so these live under 19/ and
    # tests/validate declares how many there are. Each breaks exactly ONE
    # reference and must trip arm 19 and nothing else.

    # ARM 21's FIXTURE, beside the live arms above because it is one: it needs
    # the adapter to answer a describe. It follows their convention and changes
    # the project's ASK rather than surface.json -- the arm is about what the
    # project declares, not about what the fake adapter can be made to say.
    #
    # THE ACTION PATH, which is the one docs/contract.md:222 states in words.
    # The role path and the `none` exemption are cells in tests/validate: a
    # fixture asserts WHICH ARM refused and no fixture oracle reads the refusal
    # TEXT, and `none` is a positive -- a fixture cannot say "this is not
    # refused" (ratchet, coop r8/149).
    d = fresh("21-context-not-described")
    p = os.path.join(d, "bindings.toml"); s = open(p).read()
    before = 'name = "publish_doc"\ncontext = "api"'
    assert s.count(before) == 1, "the twin's publish_doc no longer declares context = api"
    s = s.replace(before, 'name = "publish_doc"\ncontext = "kiosk"', 1)
    open(p, "w").write("# FIXTURE for arm 21: the twin with one change -- publish_doc declares a\n"
                       "# context the adapter's describe does not offer. `kiosk` is not the browser\n"
                       "# context on purpose: arms 4 and 5 both gate on isBrowser, so a browser\n"
                       "# context here would trip one of them as well and the fixture would show\n"
                       "# neither.\n" + s)

    d = fresh("19/requires-unknown-rule")
    p = os.path.join(d, "bindings.toml"); s = open(p).read()
    s = s.replace('requires = ["may_publish"]', 'requires = ["may_publish", "may_publish_absent"]', 1)
    open(p, "w").write("# FIXTURE for arm 19: the twin with one change -- publish_doc requires a rule\n"
                       "# name rules.json does not declare. rook found this one against examples/toy\n"
                       "# (coop r5/22) and every one of the eighteen arms passed it.\n"
                       "#\n"
                       "# THE DANGLING NAME IS SECOND, after a declared one, and that is the fixture's\n"
                       "# REACH: with it first, a cell that read only `Requires[0]` passed this fixture\n"
                       "# -- measured. A list-shaped reference needs its defect past element one.\n" + s)

    d = fresh("19/weight-unknown-action")
    p = os.path.join(d, "platform.toml"); s = open(p).read()
    s = s.replace("publish_doc = 1", "publish_doc = 1\npublish_nothing = 1", 1)
    open(p, "w").write("# FIXTURE for arm 19: the twin with one change -- [weights.default] weights an\n"
                       "# action bindings.toml does not declare. The PARSER refuses the same shape for\n"
                       "# an undeclared STAGE and cannot refuse this, being handed one file.\n" + s)

    d = fresh("19/action-without-a-weight")
    p = os.path.join(d, "platform.toml"); s = open(p).read()
    s = s.replace("publish_doc_ui = 1\n", "", 1)
    open(p, "w").write("# FIXTURE for arm 19: the twin with one change -- publish_doc_ui loses its\n"
                       "# weight, so a declared action the walk can draw has none. ABSENT IS NOT\n"
                       "# ZERO: zero means never drawn and the project decided it (fable, r4/125).\n" + s)

    # THE NINTH CELL'S FIXTURE, and it existed as a refusal nothing tripped.
    # Cell 2's reverse has TWO loops -- declared ACTIONS without a weight, and
    # the engine's POOL KINDS without one -- and only the first had a fixture.
    # That is rook's arm-19 finding in the arm that taught us the shape: a
    # refusal no fixture reaches is deletable in silence.
    #
    # `tell` IS THE ONE TO REMOVE, deliberately. It is an ENGINE kind rather
    # than a nemesis one (fable, coop r5/114) and it is NOT an action, so its
    # absence cannot also trip cell 2, which refuses a weight naming no action.
    # `essoin` would be the wrong choice for the opposite reason: it carries a
    # declared ZERO on purpose, and a fixture deleting that would be a fixture
    # about the zero convention rather than about an absent weight.
    d = fresh("19/kind-without-a-weight")
    p = os.path.join(d, "platform.toml"); s = open(p).read()
    s = s.replace("tell = 1\n", "", 1)
    open(p, "w").write("# FIXTURE for arm 19: the twin with one change -- the engine kind `tell`\n"
                       "# loses its weight, so a kind THE WALK CAN DRAW has none. ABSENT IS NOT\n"
                       "# ZERO (fable, r4/125): zero means never drawn and the project decided it,\n"
                       "# absent means the walk reaches a kind nobody weighted.\n"
                       "#\n"
                       "# NOT AN ACTION, so this cannot also trip cell 2 -- which is why `tell`\n"
                       "# rather than one of the nemesis kinds, and why not `essoin`, whose\n"
                       "# declared zero is a convention a fixture should not be about.\n" + s)

    d = fresh("19/venire-undeclared-role")
    p = os.path.join(d, "bindings.toml"); s = open(p).read()
    open(p, "w").write(s + '''
# SETUP, not the condition: the twin declares no roles at all, and arm 19's
# venire cell pairs only where a project declares some -- an absent [[role]]
# table is a project that does not model roles. `author` is the role the twin's
# venire already draws, and it holds `api` alone so arm 5 stays quiet.
[[role]]
name = "author"
contexts = ["api"]
''')
    p = os.path.join(d, "platform.toml"); s = open(p).read()
    open(p, "w").write(s + '''
# THE CONDITION: a venire drawing a role no [[role]] declares.
[[venire]]
role = "wizard"
count = 1
''')
    open(os.path.join(d, "WHY-venire-undeclared-role.md"), "w").write(
        "# FIXTURE for arm 19, venire cell\n\nSETUP: `[[role]] author` in bindings.toml, because the cell pairs only where\n"
        "the project declares roles AT ALL (ibis, coop r4/116; the twin declares none\n"
        "and refusing it would refuse an example fable's D9 requires to pass).\n\n"
        "CONDITION: `[[venire]] role = \"wizard\"`, which no [[role]] declares.\n")

    # THE TWO PROFILE-VENIRE CELLS (fable, coop rumor/1662). Both need the same
    # ground as the venire cell above -- the twin declares no roles and no
    # profiles -- so each declares a role, a profile, and the one row that is
    # its condition. That is one CONDITION with three edits under fable's r4/74
    # rule, and each README names which edit is which.
    d = fresh("19/profile-venire-undeclared-role")
    p = os.path.join(d, "bindings.toml"); s = open(p).read()
    open(p, "w").write(s + '''
# SETUP, not the condition: the twin declares no roles, and the venire cell
# pairs only where a project declares some. `author` is the role the twin's own
# venire draws, holding `api` alone so arm 5 stays quiet.
[[role]]
name = "author"
contexts = ["api"]
''')
    p = os.path.join(d, "platform.toml"); s = open(p).read()
    open(p, "w").write(s + '''
# SETUP: a profile, because an APPENDED venire row is the subject and the twin
# declares no profiles. The row goes after this table's own keys -- an
# array-of-tables header directly under the header would take `stages`, `seeds`
# and `budget` into the row and the file would stop parsing.
[profiles.cell]
stages = ["api"]
seeds = 1
budget = "1m"

# THE CONDITION: an appended row drawing a role no [[role]] declares. Before
# fable's rumor/1662 ruling the cell read the platform's own venire alone, so
# this row was refused by nobody.
[[profiles.cell.venire]]
role = "wizard"
count = 1
''')
    open(os.path.join(d, "WHY-profile-venire-undeclared-role.md"), "w").write(
        "# FIXTURE for arm 19, the profile-venire role cell\n\n"
        "SETUP: `[[role]] author` in bindings.toml, because the cell pairs only where the\n"
        "project declares roles AT ALL; and `[profiles.cell]` in platform.toml, because an\n"
        "APPENDED row is the subject and the twin declares no profiles.\n\n"
        "CONDITION: `[[profiles.cell.venire]] role = \"wizard\"`, which no [[role]] declares.\n"
        "The refusal NAMES THE PROFILE, because `draws role wizard` with no table sends a\n"
        "reader to the platform's own venire, which is not where it is written.\n")

    d = fresh("19/profile-venire-context-no-stage")
    p = os.path.join(d, "bindings.toml"); s = open(p).read()
    open(p, "w").write(s + '''
# SETUP, not the condition: TWO roles.
#
# `viewer` holds the BROWSER context and nothing else -- nothing else, so arm 5
# stays quiet, since a browser-stage actor that could also reach an api action
# is arm 5's refusal and would make this two fixtures.
#
# `author` IS HERE BECAUSE DECLARING ANY ROLE TURNS THE VENIRE CELL ON. That
# cell pairs only where a project declares roles at all, so with `viewer` alone
# the twin's OWN `[[venire]] role = "author"` became unresolved and this fixture
# tripped two cells of arm 19 -- measured, not foreseen, and invisible to the
# one-fixture-one-ARM oracle because both notes are arm 19's.
[[role]]
name = "author"
contexts = ["api"]

[[role]]
name = "viewer"
contexts = ["browser"]
''')
    p = os.path.join(d, "platform.toml"); s = open(p).read()
    open(p, "w").write(s + '''
# SETUP: a profile selecting the api stage ALONE. The twin enables the browser
# stage, so the profile's own selection is what makes the row undrawable --
# which is the point: the pairing is against the stages THIS profile selects,
# not against what the platform enables.
[profiles.cell]
stages = ["api"]
seeds = 1
budget = "1m"

# THE CONDITION: the profile draws an actor whose only context is one no stage
# it selects drives.
[[profiles.cell.venire]]
role = "viewer"
count = 1
''')
    open(os.path.join(d, "WHY-profile-venire-context-no-stage.md"), "w").write(
        "# FIXTURE for arm 19, the profile-venire context cell\n\n"
        "SETUP: `[[role]] viewer` holding `browser` and nothing else, so arm 5 stays quiet;\n"
        "and `[profiles.cell]` selecting `api` ALONE, which is what makes the row\n"
        "undrawable -- the twin ENABLES the browser stage, so the pairing is against the\n"
        "stages this profile selects rather than against the platform's flag.\n\n"
        "CONDITION: the appended row draws an actor whose only context no selected stage\n"
        "drives. The refusal names the profile, the role, the contexts it holds and the\n"
        "profile's stages (fable, coop rumor/1662).\n\n"
        "WHY IT IS AN ARM: a parse refusal cannot see it. A row names a ROLE and a role's\n"
        "contexts are in bindings.toml, which internal/platform does not read and cannot\n"
        "import.\n")

    # THE TWO PROFILE-OBSERVATION CELLS (fable, coop rumor/1678). The
    # observations level is the venire's one declaration over, and it has the
    # same two pairings: a name nothing declares, and a name whose source no
    # stage the profile selects can drive.
    #
    # ONE EDIT FOR THE FIRST AND THREE FOR THE SECOND, and the asymmetry is the
    # pairing's. A name nothing declares needs no ground in bindings.toml --
    # what makes it wrong is the absence. A source that cannot be driven needs a
    # browser-source observation to exist at all, and the twin's only one is
    # `direct`.
    d = fresh("19/profile-observation-undeclared")
    p = os.path.join(d, "platform.toml"); s = open(p).read()
    open(p, "w").write(s + '''
# SETUP: a profile, because a profile's CLAIM is the subject and the twin
# declares none. `observations` sits among this table's own keys, after them for
# the reason the venire fixtures' rows sit after them -- a header directly under
# the table header would take `stages`, `seeds` and `budget` with it.
[profiles.cell]
stages = ["api"]
seeds = 1
budget = "1m"

# THE CONDITION: a claim on a name bindings.toml does not declare.
observations = ["nosuch"]
''')
    open(os.path.join(d, "WHY-profile-observation-undeclared.md"), "w").write(
        "# FIXTURE for arm 19, the profile-observation name cell\n\n"
        "SETUP: `[profiles.cell]` in platform.toml, because a profile's claim is the\n"
        "subject and the twin declares no profiles.\n\n"
        "CONDITION: `observations = [\"nosuch\"]`, a name no [[observation]] declares.\n\n"
        "WHY THIS CELL IS THE DANGEROUS ONE: a profile's claim is also what takes a name\n"
        "OUT of the project's set (platform.Observes), so a misspelled claim removes\n"
        "nothing and leaves the real observation taken by EVERY profile -- restoring the\n"
        "reading the level was ruled in to prevent, while reading as a working\n"
        "declaration from inside the profile that wanted it.\n")

    d = fresh("19/profile-observation-source-no-stage")
    p = os.path.join(d, "bindings.toml"); s = open(p).read()
    open(p, "w").write(s + '''
# SETUP, not the condition: an observation whose source is `browser`. The twin
# declares one and its source is `direct`, which is read from a component rather
# than driven by a stage -- so until this row exists the cell has nothing to
# pair. It names no component and no args, so it reaches no other arm.
[[observation]]
name = "page.count"
source = "browser"
of = "doc.published"
''')
    p = os.path.join(d, "surface.json")
    surf = json.load(open(p))
    # AND THE ADAPTER DESCRIBES IT, which is setup and not decoration: a
    # bindings observation the surface omits is 19/observation-not-in-describe's
    # own cell, so leaving the surface alone would trip two cells of this arm
    # and show neither.
    surf["observations"].append({"name": "page.count", "source": "browser"})
    open(p, "w").write(json.dumps(surf, indent=2) + "\n")
    p = os.path.join(d, "platform.toml"); s = open(p).read()
    open(p, "w").write(s + '''
# SETUP: a profile selecting the api stage ALONE. The twin enables the browser
# stage, so the profile's own selection is what makes the observation
# undrivable -- the pairing is against the stages THIS profile selects, not
# against what the platform enables.
[profiles.cell]
stages = ["api"]
seeds = 1
budget = "1m"

# THE CONDITION: the profile takes an observation whose source no stage it
# selects drives.
observations = ["page.count"]
''')
    open(os.path.join(d, "WHY-profile-observation-source-no-stage.md"), "w").write(
        "# FIXTURE for arm 19, the profile-observation source cell\n\n"
        "SETUP: `[[observation]] page.count` with source `browser` in bindings.toml and\n"
        "the same name in surface.json, because the twin's only observation is `direct`\n"
        "and an observation the surface omits trips the describe cell instead; and\n"
        "`[profiles.cell]` selecting `api` ALONE, which is what makes the source\n"
        "undrivable -- the twin ENABLES the browser stage, so the pairing is against the\n"
        "stages this profile selects rather than against the platform's flag.\n\n"
        "CONDITION: `observations = [\"page.count\"]` under a profile selecting no stage\n"
        "that drives `browser`.\n\n"
        "WHY IT IS AN ARM: a parse refusal cannot see it. The claim names an observation\n"
        "and an observation's SOURCE lives in bindings.toml, which internal/platform does\n"
        "not read and cannot import -- cell 8's reason one declaration over.\n")

    d = fresh("19/observe-arg-unknown-handle")
    p = os.path.join(d, "bindings.toml"); s = open(p).read()
    s = s.replace('of = "doc.published"\ncomponent = "doc_store"',
                  'of = "doc.published"\nargs = ["doc", "nonesuch"]\ncomponent = "doc_store"', 1)
    open(p, "w").write("# FIXTURE for arm 19: the twin with one change -- the observation takes an arg\n"
                       "# naming a handle no action creates, so the engine would resolve it to nothing.\n"
                       "# `doc` IS created by publish_doc and comes first: the dangling name is SECOND\n"
                       "# so a cell reading only Args[0] cannot pass this fixture (measured -- it did).\n" + s)

    d = fresh("19/action-arg-unknown-handle")
    p = os.path.join(d, "bindings.toml"); s = open(p).read()
    # AFTER `response`, not before it: [[action.args]] is a sub-table, so every
    # scalar key of the action has to precede it or TOML reads the scalar as
    # the sub-table's. Caught by generating it the wrong way round first.
    s = s.replace('touches = ["doc_store"]\nresponse = { status = 201 }',
                  'touches = ["doc_store"]\nresponse = { status = 201 }\n\n'
                  '  [[action.args]]\n  name = "doc"\n  source = "handle"\n  of = "doc"\n\n'
                  '  [[action.args]]\n  name = "parent"\n  source = "handle"\n  of = "nonesuch"', 1)
    open(p, "w").write("# FIXTURE for arm 19: the twin with one change -- an action takes a handle\n"
                       "# argument whose `of` names a kind no action creates, so the walk could only\n"
                       "# resolve it to nothing and the action would sit inapplicable in silence.\n"
                       "# `doc` IS created by publish_doc and comes FIRST: the dangling name is second,\n"
                       "# so a cell reading only Args[0] cannot pass this fixture -- the same control\n"
                       "# the observation-arg cell needed, and for the same reason.\n" + s)

    d = fresh("19/flow-placeholder-unknown-handle")
    p = os.path.join(d, "bindings.toml"); s = open(p).read()
    s = s.replace('to = "/docs/new"', 'to = "/docs/{doc}/{nonesuch}"', 1)
    open(p, "w").write("# FIXTURE for arm 19: the twin with one change -- a UI flow step names a\n"
                       "# handle in braces that no action creates. `{doc}` is created by publish_doc\n"
                       "# and comes first, so the dangling name is the SECOND placeholder in the same\n"
                       "# field -- a cell reading only the first survived this fixture before.\n" + s)

    d = fresh("19/claim-table-unemitted-type")
    p = os.path.join(d, "bindings.toml"); s = open(p).read()
    s = s.replace('[claim."doc.published"]', '[claim."doc.archived"]\ntiming = "immediate"\n\n[claim."doc.published"]', 1)
    open(p, "w").write("# FIXTURE for arm 19: the twin with one change -- a [claim.<type>] table for a\n"
                       "# type no action emits. Arm 2 holds the other direction, and fable moved this\n"
                       "# one here rather than widening arm 2 (coop r4/113).\n" + s)

    d = fresh("19/observation-not-in-describe")
    p = os.path.join(d, "bindings.toml"); s = open(p).read()
    s = s.replace('''[[observation]]
name = "doc.count"''', '''[[observation]]
name = "doc.count"''', 1)
    open(p, "w").write("# FIXTURE for arm 19: the twin with one change -- the bindings declare a\n"
                       "# SECOND observation the adapter's surface.json does not. The SURFACE is left\n"
                       "# alone, as for 9a-9c: the arm is about the project's claim, not the adapter.\n"
                       "#\n"
                       "# THE DANGLING ONE IS SECOND and `doc.count` stays declared, because with the\n"
                       "# only observation renamed a cell reading just Observations[0] passed this\n"
                       "# fixture -- measured. The added observation names no component and no args,\n"
                       "# so it reaches no other arm.\n" + s + '''
[[observation]]
name = "doc.count.absent"
source = "api"
of = "doc.published"
''')

    # `contended`'s TWO REFERENCES, ONE CELL EACH, generated here since R8 and
    # hand-written before it. They were the only two fixtures in the corpus
    # this script did not build, and the listing it prints at the end reads the
    # directory rather than what it built, so they read as generated for as
    # long as the twin did not change. The twin changing is what found them.
    def contended_head(defect):
        # THE HEADER THESE TWO CARRIED DESCRIBED A DIFFERENT FIXTURE -- "[deadline]
        # names an instant no [[observation]] declares" -- which is the instants
        # cell's defect and not either of these. It came along with the copy, and
        # the port is where it stops being reproduced.
        return ("# FIXTURE for arm 19, a `contended` cell: the twin with one change --\n"
                "# %s\n"
                "#\n"
                "# THE [clock] AND [deadline] TABLES BELOW ARE CARRIED AND NOT REQUIRED. This\n"
                "# cell was written by hand as a copy of 19/deadline-instant-unknown-observation\n"
                "# with that fixture's planted instant repaired to a name that resolves, and the\n"
                "# contended cell is gated on neither table: internal/validate/arms_names.go\n"
                "# reads the action's own two references and nothing else. They trip no arm --\n"
                "# `instants` resolves and [clock] is what keeps arm 20 quiet -- so the material\n"
                "# is left as rook wrote it (coop rumor/1528) and only the header is corrected.\n" % defect)
    contended_tail = ('\n[clock]\nform = "faked"\n\n'
                      '[deadline]\ninstants = ["doc.count"]\n')
    obs_comment = '# source = "direct" is what makes'

    d = fresh("19/contended-limit-unknown-argument")
    p = os.path.join(d, "bindings.toml"); s = open(p).read()
    assert s.count(obs_comment) == 1, "the twin's first observation comment moved"
    s = s.replace(obs_comment,
                  "# THE PLANTED DEFECT: `limit` names an argument no action declares. `held`\n"
                  "# resolves, so this fixture trips the limit cell and nothing else.\n"
                  'contended = { limit = "no_such_action.size", held = "doc.count" }\n\n'
                  + obs_comment, 1)
    open(p, "w").write(contended_head("`limit` names an argument no action declares.")
                       + s + contended_tail)

    d = fresh("19/contended-held-unknown-observation")
    p = os.path.join(d, "bindings.toml"); s = open(p).read()
    assert s.count(obs_comment) == 1, "the twin's first observation comment moved"
    s = s.replace(obs_comment,
                  "# THE PLANTED DEFECT: `held` names no declared observation. `limit` resolves to\n"
                  "# the argument below, so this fixture trips the held cell and nothing else.\n"
                  'contended = { limit = "publish_doc.size", held = "no_such_query" }\n'
                  "\n"
                  "  [[action.args]]\n"
                  '  name = "size"\n'
                  '  source = "literal"\n'
                  "  value = 5\n"
                  "\n"
                  + obs_comment, 1)
    open(p, "w").write(contended_head("`held` names no declared observation.")
                       + s + contended_tail)

    # THE MISMATCH IS DERIVED FROM THE ENGINE'S OWN VERSION, not typed. This
    # wrote `contract = "2"` as "a version nobody declares", and the 2026-10-19
    # bump to contract 2 turned the fixture into an AGREEMENT -- arm 13 then
    # passed on the fixture built to trip it, which is a fixture going stale
    # rather than an arm breaking. A mismatch is "not what the engine says", so
    # it is spelled that way and survives every later bump.
    d = fresh("13-contract-mismatch")
    p = os.path.join(d, "platform.toml"); s = open(p).read()
    engine = contract_version()
    before = 'contract = "%s"' % engine
    assert s.count(before) == 1, "the twin's platform.toml does not state the engine's version"
    s = s.replace(before, 'contract = "%s-not-the-engines"' % engine, 1)
    open(p, "w").write("# FIXTURE for arm 13: the twin with one change -- platform.toml states a\n"
                       "# contract version the engine and the adapter do not. The value is derived\n"
                       "# from internal/wire's own constant, so a bump cannot make it agree.\n" + s)

    out = []
    for name in sorted(os.listdir(ROOT)):
        path = os.path.join(ROOT, name)
        if os.path.isdir(path) and not os.path.exists(os.path.join(path, "bindings.toml")):
            # A GROUP, not a fixture: arm 19 carries several cells, so its
            # fixtures live one level down and a listing that stopped here
            # would report seven as one.
            out += ["%s/%s" % (name, child) for child in sorted(os.listdir(path))]
        else:
            out.append(name)
    return out


if __name__ == "__main__":
    if not os.path.isdir(SRC):
        sys.exit("run from the repository root: %s is not there" % SRC)
    for a in build():
        print("  ", a)
