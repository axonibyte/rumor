#!/usr/bin/env python3
"""Drive fable's D1-D16 against `rumor validate` and print the table.

THE DRILLS ARE fable's, from coop breakout-r4/0 and /3, and the gate runs them.
This is the driver ibis was asked to run ONCE before fable does (r4/60),
committed so the table is reproducible rather than a figure in a post. fable may
replace it with the gate's own wiring; the drill definitions are theirs.

WHAT DRIVING THEM FOUND, which READING them did not:

  - D7: the client parsed a malformed describe with json.Unmarshal, left
    `contract` empty, and the version arm then REFUSED the project for a value
    its adapter never sent -- exit 1 blaming a project for its adapter. It uses
    the strict decoder now and reports could-not-run.
  - D8: a project with ZERO RULES passed. Arm 11 skips when rules.json declares
    nothing, sixteen arms evaluated over a statute whose rules nothing asserts,
    and the verdict was exit 0. It is a vacuity guard now.

THE BINARY IS BUILT HERE, NOT FOUND. This read a hard-coded
/tmp/claude-1001/rumor-bin, and on 2026-10-19 the contract version went to 2:
every fixture then declared 2 against a binary nobody had rebuilt, arm 13 fired
on all of them, and 18 of 23 drills reported MISMATCH -- for a staleness in the
harness rather than anything about the arms. A driver that drives whatever
happens to be lying around is not driving this tree. $RUMOR_BIN overrides for a
caller that has already built one; the default builds and FAILS LOUDLY if it
cannot, because a drive that could not build is not a drive that passed.
"""
import json, os, re, shutil, subprocess, tempfile, sys

ROOT = os.getcwd()
PATHDIR = tempfile.mkdtemp()


def _binary():
    """The rumor under test: $RUMOR_BIN, else built from this tree."""
    override = os.environ.get("RUMOR_BIN")
    if override:
        if not os.path.isfile(override):
            raise SystemExit("drive-validate-drills: $RUMOR_BIN=%s is not a file" % override)
        return override
    out = os.path.join(PATHDIR, "rumor-under-test")
    r = subprocess.run(["go", "build", "-o", out, "./cmd/rumor"], cwd=ROOT,
                       capture_output=True, text=True,
                       env=dict(os.environ, GOTOOLCHAIN="local"))
    if r.returncode != 0:
        raise SystemExit("drive-validate-drills: cannot build ./cmd/rumor, so nothing was "
                         "driven:\n" + (r.stderr or r.stdout))
    return out


BIN = _binary()
# The same binary under the name the projects' [adapter].command spawns.
shutil.copy(BIN, os.path.join(PATHDIR, "rumor"))
ENVP = dict(os.environ, PATH=PATHDIR + os.pathsep + os.environ["PATH"], GOTOOLCHAIN="local")

def run(project, env=None):
    r = subprocess.run([BIN, "validate", project], capture_output=True, text=True,
                       env=env or ENVP, cwd=ROOT, timeout=120)
    arms = sorted(set(re.findall(r"REFUSED arm ([0-9a-z]+):", r.stdout)))
    cnr  = len(re.findall(r"could not run:", r.stdout))
    return r.returncode, arms, cnr, r.stdout

def copytwin():
    d = tempfile.mkdtemp()
    p = os.path.join(d, "twin")
    shutil.copytree(os.path.join(ROOT, "examples", "twin"), p)
    return p

def edit(p, name, fn):
    f = os.path.join(p, name); s = open(f).read(); open(f, "w").write(fn(s))

rows = []
def row(drill, what, rc, arms, cnr, expect):
    ok = expect(rc, arms, cnr)
    rows.append((drill, what, rc, ",".join(arms) or "-", cnr, "OK" if ok else "MISMATCH"))

F = "tests/validate/fixtures"

# drills whose condition my own fixture already is
for drill, fx, want in (("D1", "1-type-not-emitted", ["1"]),
                        ("D2", "2-type-not-described", ["2"]),
                        ("D3", "3-no-precondition", ["3"]),
                        ("D4", "5-browser-reaches-api", ["5"]),
                        ("D10", "4-no-ui-flow", ["4"]),
                        ("D11", "6-component-uncovered", ["6"]),
                        ("D12", "8-clock-no-control", ["8"]),
                        ("D15", "11-no-mutants", ["11"])):
    rc, arms, cnr, _ = run(os.path.join(F, fx))
    row(drill, fx, rc, arms, cnr, lambda rc, a, c, w=want: rc == 1 and a == w and c == 0)

# D5 both halves, D13 three, D14 two, D16 two
for drill, pairs in (("D5", [("7a-eventual-no-bound", ["7a"]), ("7b-quiesce-no-probe", ["7b"])]),
                     ("D13", [("9a-fault-unavailable", ["9a"]), ("9b-volume-unavailable", ["9b"]),
                              ("9c-upgrade-unavailable", ["9c"])]),
                     ("D14", [("10a-no-seeds-file", ["10a"]), ("10b-unreadable-seed", ["10b"])]),
                     ("D16", [("12a-no-position", ["12a"]), ("12b-no-source", ["12b"])])):
    for fx, want in pairs:
        rc, arms, cnr, _ = run(os.path.join(F, fx))
        row(drill, fx, rc, arms, cnr, lambda rc, a, c, w=want: rc == 1 and a == w and c == 0)

# D6 VERSION: the ADAPTER's version alone, platform and engine untouched
p = copytwin()
edit(p, "platform.toml", lambda s: s.replace(
    'command = ["rumor", "fake-adapter", "--surface", "surface.json"]',
    'command = ["rumor", "fake-adapter", "--surface", "surface.json", "--contract", "9"]'))
rc, arms, cnr, out = run(p)
row("D6", "adapter --contract 9, platform at 1", rc, arms, cnr,
    lambda rc, a, c: rc == 1 and a == ["13"] and c == 0)

# D7 ENVELOPE: a required field omitted from the adapter's describe response
p = copytwin()
edit(p, "platform.toml", lambda s: s.replace(
    'command = ["rumor", "fake-adapter", "--surface", "surface.json"]',
    'command = ["rumor", "fake-adapter", "--surface", "surface.json", "--omit", "describe.contract"]'))
rc, arms, cnr, out = run(p)
row("D7", "describe missing a required field", rc, arms, cnr,
    lambda rc, a, c: rc == 2 and a == [] and c >= 1)

# D8 VACUITY: three ways, each a refusal or could-not-run, never a pass
# EXIT 1, A REFUSAL, ruled by fable at coop r4/79: could-not-run is the
# adapter's and the decoder's, where the project is not wrong; a project that
# has not said what it is IS wrong. attaint draws the same line.
p = copytwin(); open(os.path.join(p, "rules.json"), "w").write("[]")
rc, arms, cnr, _ = run(p)
row("D8", "zero rules", rc, arms, cnr, lambda rc, a, c: rc == 1 and c == 0)
p = copytwin(); os.remove(os.path.join(p, "bindings.toml"))
rc, arms, cnr, _ = run(p)
row("D8", "no bindings file", rc, arms, cnr, lambda rc, a, c: rc == 1 and c == 0)
p = copytwin(); open(os.path.join(p, "bindings.toml"), "w").write("# nothing\n")
rc, arms, cnr, _ = run(p)
row("D8", "zero actions", rc, arms, cnr, lambda rc, a, c: rc == 1 and c == 0)

# D9 TWIN: untouched, every arm evaluated, none skipped
rc, arms, cnr, out = run("examples/twin")
m = re.search(r"(\d+) of (\d+) arm\(s\) evaluated", out)
skipped = "skipped" in out
row("D9", "untouched twin (%s of %s, skips=%s)" % (m.group(1), m.group(2), skipped),
    rc, arms, cnr, lambda rc, a, c: rc == 0 and a == [] and c == 0 and m.group(1) == m.group(2) and not skipped)

w = max(len(r[1]) for r in rows)
print("%-5s %-*s %4s %-9s %4s %s" % ("drill", w, "condition", "exit", "arms", "cnr", "verdict"))
for d, what, rc, arms, cnr, v in rows:
    print("%-5s %-*s %4d %-9s %4d %s" % (d, w, what, rc, arms, cnr, v))
bad = [r for r in rows if r[5] != "OK"]
print("\n%d drill case(s), %d as written, %d MISMATCH" % (len(rows), len(rows)-len(bad), len(bad)))
# THE STATUS SAYS WHAT THE TEXT SAYS. This script printed MISMATCH and exited 0
# until 2026-10-19, so every caller that read its exit code -- including my own
# battery -- saw green over 18 failing drills. A driver whose verdict is only in
# its prose is one a CI step cannot gate on.
sys.exit(1 if bad else 0)
