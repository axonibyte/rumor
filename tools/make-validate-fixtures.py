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
[[role]]
name = "editor"
contexts = ["browser", "api"]
''')
    open(os.path.join(d, "WHY-browser-reaches-api.md"), "w").write(
        "# FIXTURE for arm 5\n\nONE CHANGE, no setup: the twin already enables the browser stage. This adds a\n"
        "role holding both `browser` and `api`, so a browser-stage actor could take the\n"
        "api action the twin declares.\n\n"
        "The role is deliberately not in the twin: tests/contract's agreement test\nrefuses a declared role no case carries, and the twin's cases carry none.\n")

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

    return sorted(os.listdir(ROOT))


if __name__ == "__main__":
    if not os.path.isdir(SRC):
        sys.exit("run from the repository root: %s is not there" % SRC)
    for a in build():
        print("  ", a)
