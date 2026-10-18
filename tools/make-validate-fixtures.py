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

    return sorted(os.listdir(ROOT))


if __name__ == "__main__":
    if not os.path.isdir(SRC):
        sys.exit("run from the repository root: %s is not there" % SRC)
    for a in build():
        print("  ", a)
