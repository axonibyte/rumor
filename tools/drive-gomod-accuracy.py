#!/usr/bin/env python3
"""Drive ci.yml's go.mod accuracy step in four conditions, on this machine.

WHY IT EXISTS. tools/gomod-accuracy-verdict.sh's own battery covers the
READING -- nine clauses, run on the runner by ci.yml's verdict-scripts step.
Nothing covers the WIRING: that the step captures tidy's stdout and stderr into
the two files the verdict tells apart, passes them in the right order with the
real exit status, and propagates the verdict's code. That is the half ci.yml
itself says a push is the only way to exercise, and this is the half that
removes the push.

WHAT IT MEASURES, each in its own copy of the tree:

    1 POSITIVE     the tree as committed            -> 0, CLAUSE tidy
    2 INACCURATE   an unused require in go.mod      -> 3, CLAUSE inaccurate
    3 BLOCKED      GOPROXY=off, empty module cache  -> 1, could-not-read-the-graph
    4 VACUOUS      no requirements, empty go.sum    -> 1, nothing-to-be-accurate-about

Condition 3 is the one the arm is shaped around: an unreachable proxy must not
be reported as an inaccurate manifest.

NO PyYAML, AND THAT IS DELIBERATE. tools/check-workflows.py reads these files
with line logic for the reason docs/release.md gives about unmeasured
dependencies, and a driver that needed a wheel the rest of the tree does not
would be a second class of dependency for one tool. The step is extracted the
same way: by its `- name:` line and the indented block under `run: |`, so this
drives the step ci.yml actually carries rather than a copy that can drift.

Usage: python3 tools/drive-gomod-accuracy.py [--repo DIR]
Exits 0 when all four conditions answer as written, 1 otherwise, 2 if it could
not run at all -- which is not a pass.
"""
import argparse
import os
import pathlib
import shutil
import subprocess
import sys
import tempfile

STEP = "go.mod is ACCURATE, not merely present"
WORKFLOW = pathlib.Path(".forgejo/workflows/ci.yml")


def step_script(repo: pathlib.Path, name: str = STEP) -> str:
    """The shell of the step called `name`, dedented, read out of ci.yml."""
    lines = (repo / WORKFLOW).read_text().splitlines()
    want = f"- name: {name}"
    for i, line in enumerate(lines):
        if line.strip() != want:
            continue
        body, indent = [], None
        for rest in lines[i + 1 :]:
            if rest.strip() == "run: |":
                indent = len(rest) - len(rest.lstrip()) + 2
                continue
            if indent is None:
                # another key of the same step (`if:`, `env:`) -- keep looking
                if rest.strip() and len(rest) - len(rest.lstrip()) <= len(line) - len(line.lstrip()):
                    break
                continue
            if rest.strip() and len(rest) - len(rest.lstrip()) < indent:
                break
            body.append(rest[indent:] if len(rest) >= indent else "")
        if not body:
            raise SystemExit(f"could not read a `run: |` block for step {name!r}")
        return "\n".join(body)
    raise SystemExit(f"no step named {name!r} in {WORKFLOW}")


def tree(repo: pathlib.Path, dst: pathlib.Path) -> pathlib.Path:
    """A copy of the repo at dst/src-tree, which is where the step cd's to."""
    shutil.copytree(
        repo, dst / "src-tree",
        ignore=shutil.ignore_patterns(".git", "rumor-bin", "*.log", "*.diff"),
    )
    return dst / "src-tree"


def add_unused_require(src: pathlib.Path) -> None:
    gomod = src / "go.mod"
    text = gomod.read_text()
    if "require (" not in text:
        raise SystemExit("go.mod has no require block: this mutation cannot apply")
    gomod.write_text(text.replace(
        "require (", "require (\n\tgithub.com/fortytw2/leaktest v1.3.0", 1))


def empty_the_module(src: pathlib.Path) -> None:
    """A module with nothing to be accurate about: no requires, no sums."""
    (src / "go.sum").write_text("")
    (src / "go.mod").write_text(
        "module forge.axonibyte.com/axonibyte/rumor\n\ngo 1.26.0\n")
    for path in src.rglob("*.go"):
        path.unlink()
    (src / "main.go").write_text("package main\n\nfunc main() {}\n")


def run(repo, script, label, want_rc, want_clause, mutate=None, env=None):
    with tempfile.TemporaryDirectory() as td:
        scratch = pathlib.Path(td)
        src = tree(repo, scratch)
        if mutate:
            mutate(src)
        environ = dict(os.environ, GOTOOLCHAIN="local")
        environ.pop("GOFLAGS", None)
        if env:
            environ.update(env)
        proc = subprocess.run(
            ["bash", "-c", script], cwd=scratch, env=environ,
            capture_output=True, text=True, timeout=1800,
        )
        out = proc.stdout + proc.stderr
        clause = ""
        for line in out.splitlines():
            if line.startswith("CLAUSE "):
                clause = line[len("CLAUSE "):].strip()
                break
        ok = proc.returncode == want_rc and clause == want_clause
        print(f"  {'ok  ' if ok else 'FAIL'} {label}")
        print(f"         exit {proc.returncode} (want {want_rc}), "
              f"clause {clause or '(none)'} (want {want_clause})")
        if not ok:
            for line in out.splitlines()[-12:]:
                print("         |", line[:160])
        return ok


def main(argv):
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--repo", default=".", help="the checkout to drive")
    args = parser.parse_args(argv)
    repo = pathlib.Path(args.repo).resolve()
    if not (repo / WORKFLOW).is_file():
        print(f"drive-gomod-accuracy: no {WORKFLOW} under {repo}", file=sys.stderr)
        return 2
    if not shutil.which("go") and not shutil.which("go126") and not shutil.which("go125"):
        print("drive-gomod-accuracy: no go, go126 or go125 on PATH -- a "
              "PROVISIONING finding, and NOT a pass", file=sys.stderr)
        return 2

    script = step_script(repo)
    print(f"driving the step {STEP!r} as ci.yml carries it, "
          f"{len(script.splitlines())} line(s)")
    results = [
        run(repo, script, "1 POSITIVE: the tree as committed", 0, "tidy"),
        run(repo, script, "2 INACCURATE: an unused require added to go.mod",
            3, "inaccurate", mutate=add_unused_require),
        run(repo, script, "3 BLOCKED: GOPROXY=off over an empty module cache",
            1, "could-not-read-the-graph",
            env={"GOPROXY": "off", "GOMODCACHE": tempfile.mkdtemp()}),
        run(repo, script, "4 VACUOUS: no requirements and an empty go.sum",
            1, "nothing-to-be-accurate-about", mutate=empty_the_module),
    ]
    good = sum(1 for r in results if r)
    print(f"\n{len(results)} condition(s), {good} as written, "
          f"{len(results) - good} MISMATCH")
    return 0 if good == len(results) else 1


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
