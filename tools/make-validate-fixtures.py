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
import json, os, shutil, sys

SRC = "examples/twin"
ROOT = "tests/validate/fixtures"


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
    p = os.path.join(d, "platform.toml"); s = open(p).read()
    open(p, "w").write(s.replace("browser = { enabled = false }", "browser = { enabled = true }", 1))
    p = os.path.join(d, "bindings.toml"); s = open(p).read()
    open(p, "w").write(s + '''
# THE CONDITION: a browser-stage action declaring no flow. `requires` is present
# so this action does not trip arm 3 as well -- a setup edit that trips an arm
# by itself would make this two fixtures (fable, coop r4/74).
[[action]]
name = "publish_doc_ui"
context = "browser"
requires = ["may_publish"]
''')
    open(os.path.join(d, "WHY-no-ui-flow.md"), "w").write(
        "# FIXTURE for arm 4\n\nSETUP: `stages.browser.enabled` goes true. Both browser arms are VACUOUS\n"
        "without it (docs/contract.md:419), so the flag is what makes the condition\nreachable rather than part of it.\n\n"
        "CONDITION: a browser-stage action declaring no flow.\n\n"
        "The added action carries `requires` on purpose: without it the setup would\ntrip arm 3 too, and fable's rule is that a fixture whose setup edit trips an\narm by itself is two fixtures.\n")

    d = fresh("5-browser-reaches-api")
    p = os.path.join(d, "platform.toml"); s = open(p).read()
    open(p, "w").write(s.replace("browser = { enabled = false }", "browser = { enabled = true }", 1))
    p = os.path.join(d, "bindings.toml"); s = open(p).read()
    open(p, "w").write(s + '''
# THE CONDITION: an actor holding BOTH a browser session and the api context,
# so it could reach publish_doc from the browser stage. docs/contract.md:69 --
# "could reach" is "its context is one that actor holds a session in".
[[role]]
name = "editor"
contexts = ["browser", "api"]
''')
    open(os.path.join(d, "WHY-browser-reaches-api.md"), "w").write(
        "# FIXTURE for arm 5\n\nSETUP: `stages.browser.enabled` goes true, without which the arm is vacuous.\n\n"
        "CONDITION: a role holding both `browser` and `api`, so a browser-stage actor\ncould take the api action the twin already declares. No browser ACTION is\n"
        "needed -- reachability is about the actor's contexts and the action's.\n\n"
        "This role is deliberately not in the twin: quill's agreement test refuses a\ndeclared role no case carries, and the twin's cases carry none.\n")

    d = fresh("7b-quiesce-no-probe")
    p = os.path.join(d, "bindings.toml"); s = open(p).read()
    s = s.replace('timing = "immediate"', 'timing = "eventual"\nbound = "quiesce"', 1)
    open(p, "w").write("# FIXTURE for arm 7b.\n#\n# SETUP: doc.published settles EVENTUALLY rather than immediately, without\n# which a bound says nothing (bindings.go refuses a bound on an immediate type).\n#\n# CONDITION: the bound is `quiesce` and no [quiesce] probe is declared.\n" + s)

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

    d = fresh("13-contract-mismatch")
    p = os.path.join(d, "platform.toml"); s = open(p).read()
    s = s.replace('contract = "1"', 'contract = "2"', 1)
    open(p, "w").write("# FIXTURE for arm 13: the twin with one change -- platform.toml states a\n"
                       "# contract version the engine and the adapter do not.\n" + s)

    return sorted(os.listdir(ROOT))


if __name__ == "__main__":
    if not os.path.isdir(SRC):
        sys.exit("run from the repository root: %s is not there" % SRC)
    for a in build():
        print("  ", a)
