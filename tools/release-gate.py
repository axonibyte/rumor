#!/usr/bin/env python3
"""The release gate -- R0.3, issue 4.

Refuses a release run for each reason the epic's "Release gate" list names.
Nine bullets, twelve arms: the version/subject bullet is a disjunction with two
arms, and the changelog bullet has three failure modes (section absent, present
but empty, right version wrong date).

WHY PYTHON AND LOCAL, not a `rumor` subcommand: the same reason
tools/check-workflows.py is. The runner could not compile anything until
2026-10-11, and a gate that cannot run where it is needed is not a gate. It
becomes a subcommand beside `docs --check` when that is worth a migration, and
until then it is driveable on any box with no toolchain at all -- which is what
let the twelve arms be measured before a single fixture branch was pushed.

THE GATE DOES NOT SHORT-CIRCUIT, and that is the load-bearing decision in this
file rather than any individual check. Issue 4's test is that each fixture
branch "shows the gate red with the refusal reason it names, AND NO OTHER
REASON". A gate that exits at its first finding cannot answer the second half:
a fixture tripping three arms looks identical to one tripping its own. So every
check runs, every finding prints with a stable id, and the exit status says only
whether the set is empty. The fixtures are then falsifiable against the set.

AND THE POSITIVE CONTROL IS WHY ARM 9 IS SPLIT FROM THE EXEMPTION. A gate that
refuses everything satisfies all twelve refusal arms; twelve reds prove the gate
is loud, not that it discriminates (plumb coop main/110, rook main/112 -- five
arms of theirs survived both an always-refuse and an always-pass subject). So
`docs-unchanged-no-trailer` and the `Docs-Unchanged` exemption are the same
bullet in opposite directions, and the control branch that must PASS is built
first.

EXIT STATUS, and 124 is not among them on purpose: this tool makes no external
call that can hang. 0 clean, 1 one or more findings, 2 the gate could not run
(bad invocation, missing file, git refused) -- which is NOT a refusal and must
never be read as one, because a gate that cannot run is not a gate that passed.
"""

from __future__ import annotations

import argparse
import datetime as _dt
import os
import re
import subprocess
import sys
import tempfile
from pathlib import Path

RC_CLEAN = 0
RC_FINDINGS = 1
RC_CANNOT_RUN = 2

# Stable ids. A fixture branch names one of these and the gate's output is
# checked against it, so renaming one is a breaking change to the fixtures.
ARMS = (
    "version-unchanged",
    "subject-mismatch",
    "commit-after-bump",
    "version-not-greater",
    "changelog-section-absent",
    "changelog-section-empty",
    "changelog-date-wrong",
    "docs-stale",
    "docs-unchanged-no-trailer",
    "revision-entry-missing",
    "doc-run-block-failed",
    "not-rebased",
)

_SUBJECT = re.compile(r"\Arelease: v(?P<ver>\d+\.\d+\.\d+)\Z")
_WS_VERSION = re.compile(r"^version\s*=\s*\"(?P<ver>[^\"]+)\"", re.M)
# THE TRAILER IS A GIT TRAILER AND FOLDS LIKE ONE. `re.M` makes `$` end of
# LINE, so a wrapped reason was captured only as far as the break -- ibis r0.3/13
# measured it on my own control: "the edit is a comment and changes no" where the
# sentence continues "command, flag or output", which REVERSES it. Continuation
# lines are folded as git folds them: a following line that begins with
# whitespace continues the reason.
#
# WHAT THE TRUNCATION REACHED, MEASURED RATHER THAN ASSUMED -- because my first
# version of this comment asserted a consequence I had not checked. The only
# consumer of this map in this file is arm 9's `doc not in trailers`, which reads
# the KEYS; the gate never prints a reason, and nothing else in the repository
# reads the trailer at all. So the corrupted value had no consumer here, and no
# arm could have gone red for it -- which is exactly why twelve green arms said
# nothing about it. The epic's "the reasons are copied into the PR summary" and
# rook's sign-off on each reason are both hand steps, worked from the commit
# message, where the whole sentence is present.
#
# IT IS STILL A DEFECT, FOR A REASON THAT NEEDS NO CONSUMER: git does not treat a
# column-0 continuation as part of the value, so the trailer git sees and the
# sentence the author wrote are two different texts. Anything extracting trailers
# with git semantics -- `git interpret-trailers`, or a forge that renders them --
# gets the truncation. Folding makes those two texts the same when the
# continuation is indented; refusing makes the disagreement loud when it is not.
# Truncating silently is the one option that publishes a sentence nobody wrote.
#
# A LONE HYPHEN IS STILL NOT A SEPARATOR (ibis r0.3/8): accepting `-` beside `--`
# and the em dash widens the EXEMPTION, which is the one direction worth being
# strict in, and it is ambiguous against a hyphenated filename. NOTE this applies
# to the TRAILER ONLY -- changelog_section deliberately keeps `-`, because arm 4's
# fixture section is `## 0.1.1 -- 2026-10-12` and tightening both would break it.
_TRAILER_HEAD = re.compile(r"\ADocs-Unchanged:\s*(?P<doc>\S+)\s*(?:--|\u2014)\s*(?P<rest>.*)\Z")


def docs_unchanged(message: str) -> dict[str, str]:
    """{doc: reason} from a commit message, folding wrapped reasons.

    A reason continues onto any following line that begins with whitespace, which
    is git's own trailer-continuation rule. A trailer whose reason folds to
    nothing is not an exemption and is dropped, so an empty `Docs-Unchanged:`
    cannot silently excuse a doc.
    """
    out: dict[str, str] = {}
    doc = None
    parts: list[str] = []

    def flush():
        if doc is not None:
            reason = " ".join(p.strip() for p in parts if p.strip()).strip()
            if reason:
                out[doc] = reason

    lines = message.splitlines()
    for idx, line in enumerate(lines):
        found = _TRAILER_HEAD.match(line.strip()) if not line[:1].isspace() else None
        if found:
            flush()
            doc = found.group("doc")
            parts = [found.group("rest")]
            # A HARD-WRAPPED TRAILER IS MALFORMED AND MUST NOT EXEMPT. git's
            # continuation is an INDENTED line; a reason wrapped to column 0 is
            # not a continuation, and git itself would read the value as the
            # first line alone. Folding by whitespace therefore does NOT rescue
            # it -- measured on my own control, where "changes no" is where the
            # sentence stopped and "command, flag or output" began a new line at
            # column 0. So such a trailer is DROPPED rather than truncated,
            # which makes arm docs-unchanged-no-trailer fire for that doc: the
            # run claimed an exemption and did not receive one, loudly, with no
            # new arm and no sentence published that nobody wrote.
            nxt = lines[idx + 1] if idx + 1 < len(lines) else ""
            if nxt.strip() and not nxt[:1].isspace() and not _TRAILER_HEAD.match(nxt.strip()):
                print(
                    f"release-gate: the Docs-Unchanged trailer for {doc} is "
                    f"followed by an unindented line, so its reason is wrapped "
                    f"and git would read only the first line. It does NOT "
                    f"exempt. Put the reason on one line, or indent the "
                    f"continuation.",
                    file=sys.stderr,
                )
                doc, parts = None, []
            continue
        if doc is not None and line[:1].isspace() and line.strip():
            parts.append(line)
            continue
        if doc is not None:
            flush()
            doc, parts = None, []
    flush()
    return out


class CannotRun(Exception):
    """The gate could not reach a verdict. Never a refusal."""


def git(repo: Path, *args: str) -> str:
    """Run git and return stdout. A git failure is CannotRun, never a finding.

    Bounded: a gate that can hang is a gate that wedges a single-concurrency
    runner, which is the harm ci.yml's bounds exist to prevent.
    """
    try:
        done = subprocess.run(
            ["git", "-C", str(repo), *args],
            capture_output=True,
            text=True,
            timeout=120,
            check=False,
        )
    except subprocess.TimeoutExpired as exc:
        raise CannotRun(f"git {' '.join(args)} exceeded 120s") from exc
    except OSError as exc:
        raise CannotRun(f"git could not be run: {exc}") from exc
    if done.returncode != 0:
        raise CannotRun(
            f"git {' '.join(args)} exited {done.returncode}: {done.stderr.strip()}"
        )
    return done.stdout


def git_ok(repo: Path, *args: str) -> bool:
    """True when git exits 0. For questions whose answer IS the status."""
    try:
        done = subprocess.run(
            ["git", "-C", str(repo), *args],
            capture_output=True,
            text=True,
            timeout=120,
            check=False,
        )
    except (subprocess.TimeoutExpired, OSError) as exc:
        raise CannotRun(f"git {' '.join(args)} could not be run: {exc}") from exc
    return done.returncode == 0


def show(repo: Path, ref: str, path: str) -> str | None:
    """A file's content at a ref, or None when it is absent there."""
    try:
        return git(repo, "show", f"{ref}:{path}")
    except CannotRun:
        return None


def workspace_version(text: str | None) -> str | None:
    """The [workspace.package] version. None when it cannot be read.

    Keyed on the FIRST `version = "..."` after the [workspace.package] header
    rather than the first in the file: the root manifest's own [package] block
    would otherwise answer, and in this workspace it does not carry one at all.
    """
    if not text:
        return None
    head = text.split("[workspace.package]", 1)
    if len(head) != 2:
        found = _WS_VERSION.search(text)
        return found.group("ver") if found else None
    found = _WS_VERSION.search(head[1])
    return found.group("ver") if found else None


def version_tuple(ver: str) -> tuple[int, ...]:
    return tuple(int(part) for part in ver.split("."))


def latest_tag_version(repo: Path, exclude: str | None = None) -> str | None:
    """The greatest v* tag by version order, not by date.

    Date order is wrong here: a tag pushed late for an earlier version would
    otherwise become "the latest" and a correct bump would read as going
    backwards.
    """
    out = git(repo, "tag", "--list", "v*")
    versions = []
    for line in out.splitlines():
        name = line.strip()
        if not name or name == exclude:
            continue
        if re.fullmatch(r"v\d+\.\d+\.\d+", name):
            versions.append(version_tuple(name[1:]))
    if not versions:
        return None
    return ".".join(str(part) for part in max(versions))


def changelog_section(text: str | None, ver: str) -> tuple[bool, str, str] | None:
    """(found, date_text, body) for the section headed with `ver`.

    None when no heading for that version exists at all. The three changelog
    arms are told apart here rather than by three separate scans, so a section
    that is both empty and wrongly dated reports both and the fixture that
    means to trip one is falsifiable.
    """
    if not text:
        return None
    pattern = re.compile(
        r"^##\s+" + re.escape(ver) + r"\s*(?:--|—|-)\s*(?P<date>\S+)?\s*$", re.M
    )
    found = pattern.search(text)
    if not found:
        bare = re.compile(r"^##\s+" + re.escape(ver) + r"\s*$", re.M).search(text)
        if not bare:
            return None
        start = bare.end()
        date = ""
    else:
        start = found.end()
        date = (found.group("date") or "").strip()
    rest = text[start:]
    nxt = re.search(r"^##\s+", rest, re.M)
    body = rest[: nxt.start()] if nxt else rest
    return (True, date, body.strip())


def mapped_docs(coverage: str | None, path: str) -> list[str]:
    """Docs mapped to a path by docs/coverage.toml.

    A deliberately small reader of the same file `rumor docs --check` parses:
    [[entry]] blocks with `path` and `docs`. `*` matches inside one segment,
    `**` matches whole segments. Parsing a subset of a format another tool owns
    is a hazard, so the gate's arm 9 states that bound in its finding text.
    """
    if not coverage:
        return []
    out: list[str] = []
    for block in coverage.split("[[entry]]")[1:]:
        pat = re.search(r"^path\s*=\s*\"([^\"]+)\"", block, re.M)
        docs = re.search(r"^docs\s*=\s*\[(.*?)\]", block, re.M | re.S)
        if not pat or not docs:
            continue
        glob = pat.group(1)
        rx = (
            "\\A"
            + glob.replace(".", "\\.")
            .replace("**/", "\x00")
            .replace("**", "\x01")
            .replace("*", "[^/]*")
            .replace("\x00", "(?:.*/)?")
            .replace("\x01", ".*")
            + "\\Z"
        )
        if re.match(rx, path):
            out.extend(re.findall(r"\"([^\"]+)\"", docs.group(1)))
    return out


def run_doc_blocks(repo: Path, ref: str) -> list[str]:
    """Doc fenced blocks marked `run`, executed. Returns the failures.

    Bounded per block. A block that cannot answer in 60s is a failure of this
    arm rather than a hang of the gate: the alternative is a gate that wedges
    the runner, which ci.yml's whole bound discipline exists to prevent.
    """
    failures: list[str] = []
    names = git(repo, "ls-tree", "-r", "--name-only", ref).splitlines()
    for name in names:
        if not name.startswith("docs/") or not name.endswith(".md"):
            continue
        text = show(repo, ref, name) or ""
        for found in re.finditer(
            r"^```(?:sh|bash|console)\s+run\s*$(?P<body>.*?)^```\s*$",
            text,
            re.M | re.S,
        ):
            script = found.group("body")
            try:
                done = subprocess.run(
                    ["sh", "-c", script],
                    cwd=str(repo),
                    capture_output=True,
                    text=True,
                    timeout=60,
                    check=False,
                )
            except subprocess.TimeoutExpired:
                failures.append(f"{name}: a `run` block exceeded 60s")
                continue
            except OSError as exc:
                failures.append(f"{name}: a `run` block could not be started: {exc}")
                continue
            if done.returncode != 0:
                failures.append(
                    f"{name}: a `run` block exited {done.returncode}: "
                    f"{(done.stderr or done.stdout).strip().splitlines()[-1] if (done.stderr or done.stdout).strip() else 'no output'}"
                )
    return failures


def evaluate(repo: Path, base: str, head: str, today: str | None = None) -> list[tuple[str, str]]:
    """Every arm, in id order. Returns [(arm, detail)] -- empty means clean."""
    findings: list[tuple[str, str]] = []

    def add(arm: str, detail: str) -> None:
        assert arm in ARMS, f"unknown arm {arm}"
        findings.append((arm, detail))

    # PEEL BOTH REFS TO COMMITS. `rev-parse v0.1.1` on an ANNOTATED tag returns
    # the tag OBJECT's sha, not the commit's, so comparing it to a merge-base
    # made `not-rebased` fire on a perfectly rebased run -- which is how I found
    # it, gating main against v0.1.1. Every tag this project makes is annotated
    # and signed by rule, so the unpeeled form was wrong for every base a
    # release run would actually use.
    merge_base = git(repo, "merge-base", base, head).strip()
    base_sha = git(repo, "rev-parse", f"{base}^{{commit}}").strip()

    # --- arm 12: not rebased on main -------------------------------------
    # Asked first because every other arm's meaning depends on the run being
    # the tail of main. Not short-circuited, though: see the module docstring.
    if merge_base != base_sha:
        behind = git(repo, "rev-list", "--count", f"{merge_base}..{base}").strip()
        add(
            "not-rebased",
            f"the branch is not rebased on {base}: merge-base {merge_base[:12]} "
            f"is not {base}'s tip {base_sha[:12]}, so a fast-forward merge is "
            f"impossible -- {behind} commit(s) of {base} are missing from this run",
        )

    revs = [
        line
        for line in git(repo, "rev-list", "--reverse", f"{merge_base}..{head}")
        .strip()
        .splitlines()
        if line
    ]
    if not revs:
        raise CannotRun(
            f"no commits between {base} and {head}: there is no release run to gate"
        )

    # The bump commit is the one whose subject claims a release. The LAST such
    # commit is the candidate, so that "a commit follows the bump" is a finding
    # about position rather than an absence of a bump.
    subjects = {
        rev: git(repo, "log", "-1", "--format=%s", rev).strip() for rev in revs
    }
    bodies = {rev: git(repo, "log", "-1", "--format=%B", rev) for rev in revs}
    bump_idx = None
    for idx, rev in enumerate(revs):
        if _SUBJECT.match(subjects[rev]):
            bump_idx = idx
    head_rev = revs[-1]

    # --- arm 3: a commit follows the bump commit --------------------------
    if bump_idx is not None and bump_idx != len(revs) - 1:
        after = [subjects[rev] for rev in revs[bump_idx + 1 :]]
        add(
            "commit-after-bump",
            f"{len(after)} commit(s) follow the bump commit "
            f"{revs[bump_idx][:12]}: {'; '.join(after)}. The bump must be the "
            f"head of the run so that a fast-forward puts it at the head of {base}",
        )

    # ARMS 1 AND 2 ARE ABOUT THE BUMP COMMIT, NOT THE RUN'S HEAD. The epic says
    # "the branch's last commit", which is the same thing in a well-formed run.
    # It is NOT the same when a commit follows the bump, and keying 1 and 2 on
    # the head made arm 3's fixture trip subject-mismatch too -- two arms that
    # cannot fire independently, which issue 4's test forbids. Found by this
    # file's own self-test before a branch was cut.
    bump_rev = revs[bump_idx] if bump_idx is not None else head_rev
    head_version = workspace_version(show(repo, bump_rev, "Cargo.toml"))
    base_version = workspace_version(show(repo, merge_base, "Cargo.toml"))

    # --- arm 1: the bump does not change the workspace version ------------
    if head_version is None:
        add(
            "version-unchanged",
            "the workspace version could not be read from Cargo.toml at the "
            "head of the run, so no bump can be shown",
        )
    elif head_version == base_version:
        add(
            "version-unchanged",
            f"the run's last commit leaves the workspace version at "
            f"{head_version}, unchanged from {base}: a release run must change it",
        )

    # --- arm 2: the subject is not `release: vX.Y.Z` matching the version --
    found = _SUBJECT.match(subjects[bump_rev])
    if not found:
        add(
            "subject-mismatch",
            f"the run's bump commit subject is {subjects[bump_rev]!r}, not "
            f"`release: vX.Y.Z`",
        )
    elif head_version is not None and found.group("ver") != head_version:
        add(
            "subject-mismatch",
            f"the subject says v{found.group('ver')} and Cargo.toml says "
            f"{head_version}: the two must agree",
        )

    # --- arm 4: the new version is not greater than the latest tag --------
    # ARM 4 PRESUPPOSES A NEW VERSION, which is the ruling ibis asked for at
    # r0.3/1 point 1. Their objection is exact: a fixture that leaves the
    # version unchanged also leaves it EQUAL to the latest tag, so arm 1 and
    # arm 4 fire together by construction and neither can show "its own reason
    # and no other". Of the three ways out, this is the only one that weakens
    # nothing: NOT a short-circuit (that would destroy the "no other reason"
    # test for every arm), and NOT a re-reading of the epic's bullet. The
    # bullet says "the NEW version is not greater" -- when the version did not
    # change there IS no new version, so arm 4 has no subject and does not
    # apply. A fixture for arm 4 therefore CHANGES the version, downward or
    # sideways, which is the only shape that isolates it.
    version_changed = head_version is not None and head_version != base_version
    if version_changed:
        # EXCLUDE A TAG THAT POINTS AT THIS RUN'S OWN BUMP. The gate runs on a
        # PR, before the tag exists -- but re-run afterwards (which is how I
        # first drove it against the accepted v0.1.2 run) the tag for this very
        # version is "the latest" and a correct run reads as going backwards.
        # Found by gating the real 6c5583d..5e8a639, which cal had already
        # tagged; a self-test over synthetic repos could not have shown it.
        own = None
        for line in git(repo, "tag", "--points-at", bump_rev).splitlines():
            if re.fullmatch(r"v\d+\.\d+\.\d+", line.strip()):
                own = line.strip()
        latest = latest_tag_version(repo, exclude=own)
        if latest is not None and version_tuple(head_version) <= version_tuple(latest):
            add(
                "version-not-greater",
                f"the new version {head_version} is not greater than the latest "
                f"tag v{latest}. Versions are compared in version order, not by "
                f"tag date, so a late tag for an earlier version cannot move this",
            )

    # --- arms 5, 6, 7: the changelog section ------------------------------
    if head_version is not None:
        section = changelog_section(show(repo, bump_rev, "CHANGELOG.md"), head_version)
        if section is None:
            add(
                "changelog-section-absent",
                f"CHANGELOG.md has no section headed {head_version}",
            )
        else:
            _, date_text, body = section
            if not body:
                add(
                    "changelog-section-empty",
                    f"CHANGELOG.md's {head_version} section is present and empty: "
                    f"a heading with no body documents nothing",
                )
            stamp = today or _dt.datetime.now(_dt.timezone.utc).strftime("%Y-%m-%d")
            if date_text != stamp:
                add(
                    "changelog-date-wrong",
                    f"CHANGELOG.md's {head_version} section is dated "
                    f"{date_text or '(no date)'}, not today ({stamp})",
                )

    # --- arm 10: a contract/vocabulary constant with no revision entry ----
    changed = [
        line
        for line in git(repo, "diff", "--name-only", f"{merge_base}..{head_rev}")
        .strip()
        .splitlines()
        if line
    ]
    for which in ("contract", "vocabulary"):
        const = f"{which.upper()}_VERSION"
        touched = any(
            const in (show(repo, head_rev, path) or "")
            and (show(repo, head_rev, path) or "") != (show(repo, merge_base, path) or "")
            for path in changed
            if path.endswith(".rs")
        )
        if not touched:
            continue
        doc = f"docs/{which}.md"
        before = show(repo, merge_base, doc) or ""
        after = show(repo, head_rev, doc) or ""
        if _revisions(after) == _revisions(before):
            add(
                "revision-entry-missing",
                f"{const} changed without a new entry under \"Revisions\" in {doc}",
            )

    # --- arm 9: a source path changed with no mapped doc, and no trailer ---
    coverage = show(repo, head_rev, "docs/coverage.toml")
    trailers: dict[str, str] = {}
    for rev in revs:
        trailers.update(docs_unchanged(bodies[rev]))
    # THE BUMP'S OWN MECHANICS ARE NOT A SOURCE CHANGE, and this narrowing is
    # exactly as wide as its reason. A bump commit must change Cargo.toml (and
    # with it Cargo.lock); if that counted as "a source path changed", EVERY
    # well-formed release run would need a `Docs-Unchanged` trailer for whatever
    # Cargo.toml maps to, and the gate would refuse the thing it exists to
    # admit. Driving it against the accepted v0.1.2 run is what showed this.
    # The exemption is narrow in two ways: only these two paths, and only when
    # the BUMP is the sole commit that touched them -- a run that edits
    # Cargo.toml in its body still answers for its mapped docs.
    MECHANICS = ("Cargo.toml", "Cargo.lock")
    body_changed = set()
    if bump_idx is not None and bump_idx > 0:
        prior = revs[bump_idx - 1]
        body_changed = {
            line
            for line in git(repo, "diff", "--name-only", f"{merge_base}..{prior}")
            .strip()
            .splitlines()
            if line
        }
    for path in changed:
        if path.startswith("docs/") or path == "CHANGELOG.md":
            continue
        if path in MECHANICS and path not in body_changed:
            continue
        docs = mapped_docs(coverage, path)
        if not docs:
            continue
        unchanged = [doc for doc in docs if doc not in changed]
        if not unchanged:
            continue
        missing = [doc for doc in unchanged if doc not in trailers]
        if missing:
            add(
                "docs-unchanged-no-trailer",
                f"{path} changed and its mapped doc(s) {', '.join(missing)} did "
                f"not, with no `Docs-Unchanged: <doc> -- <reason>` trailer in the "
                f"run for them. The mapping is read from docs/coverage.toml, which "
                f"`rumor docs --check` owns; a mapping this reader misparses is a "
                f"defect in this arm rather than in the run",
            )

    # --- arm 8: generated docs out of date --------------------------------
    stale = _docs_check(repo, head_rev)
    if stale is not None:
        add("docs-stale", stale)

    # --- arm 11: a doc command block marked `run` fails -------------------
    for failure in run_doc_blocks(repo, head_rev):
        add("doc-run-block-failed", failure)

    order = {arm: idx for idx, arm in enumerate(ARMS)}
    findings.sort(key=lambda pair: order[pair[0]])
    return findings


def _revisions(doc: str) -> list[str]:
    """The lines under a "Revisions" heading, as the entry set."""
    found = re.search(r"^#{1,4}\s*Revisions\s*$(?P<body>.*?)(?=^#{1,4}\s|\Z)", doc, re.M | re.S)
    if not found:
        return []
    return [
        line.strip()
        for line in found.group("body").splitlines()
        if line.strip().startswith(("-", "*", "|"))
    ]


def _docs_check(repo: Path, ref: str) -> str | None:
    """`rumor docs --check` for the run at `ref`, or None when it cannot answer.

    CANNOT RUN IS NOT A FINDING. If cargo is absent -- which was true of this
    runner until 2026-10-11 and is true of any box without a toolchain -- the
    arm reports nothing rather than refusing, and says so on stderr. A gate
    that refuses because its own tool is missing is the "always-refuse" mutant
    that makes every other arm meaningless.

    THIS ARM READS THE CHECKOUT, SO IT VERIFIES THE CHECKOUT IS `ref`. Every
    other arm asks git about the range and so is independent of what is checked
    out; this one has to build and run `rumor`, and there is no way to ask a
    cargo binary about a ref without putting that ref in a tree. `git worktree`
    is not available as a route: `docs --check` fails inside one, because `.git`
    is a file there (rumor issue 6, filed and not started). So the arm states
    its premise and measures it, rather than taking `ref` and quietly ignoring
    it -- which is what it did until now.

    That was a SILENT WRONG ANSWER, not a limitation. Run from another branch,
    the arm read THAT branch's tree and reported docs-stale clean for the
    fixture whose unmapped path exists only on the fixture branch. The fixture
    verification could not see it: it checked each branch out before gating, so
    the premise held by accident every time it was tested. Found by running the
    same fixture from a different checkout.

    One hazard remains and is the safe direction. `docs --check` walks the
    filesystem, so a gitignored stray (a `tools/__pycache__` from running this
    very file as a module) is named as unmapped even though it is in no commit,
    and the arm then REFUSES a clean run naming that path. A false refusal that
    prints the offending filename is recoverable in seconds; a false pass is
    not. The disagreement is rumor's -- `docs --check` and its own
    committed_tree_is_fully_mapped test read different sets -- and is filed
    rather than worked around here.
    """
    if not (Path(repo) / "Cargo.toml").exists():
        return None
    try:
        checked_out = git(repo, "rev-parse", "HEAD").strip()
        wanted = git(repo, "rev-parse", f"{ref}^{{commit}}").strip()
    except CannotRun as exc:
        print(
            f"release-gate: {exc}; arm docs-stale UNMEASURED, not passed",
            file=sys.stderr,
        )
        return None
    if checked_out != wanted:
        print(
            "release-gate: arm docs-stale reads the CHECKOUT, and the checkout "
            f"is {checked_out[:12]} while the run being gated ends at "
            f"{wanted[:12]}. `rumor docs --check` would answer about the wrong "
            "tree, so the arm is UNMEASURED, not passed. Check out the run (or "
            "let CI, which gates its own checkout) to measure it.",
            file=sys.stderr,
        )
        return None
    dirty = git(repo, "status", "--porcelain", "--untracked-files=no").strip()
    if dirty:
        print(
            "release-gate: arm docs-stale reads the checkout and the checkout "
            f"has uncommitted changes to tracked files ({len(dirty.splitlines())} "
            "path(s)), so `rumor docs --check` would answer about a tree that is "
            "in no commit. UNMEASURED, not passed.",
            file=sys.stderr,
        )
        return None
    try:
        done = subprocess.run(
            ["cargo", "run", "--locked", "-q", "--", "docs", "--check"],
            cwd=str(repo),
            capture_output=True,
            text=True,
            timeout=300,
            check=False,
        )
    except subprocess.TimeoutExpired:
        print(
            "release-gate: docs --check exceeded 300s; arm docs-stale UNMEASURED",
            file=sys.stderr,
        )
        return None
    except OSError:
        print(
            "release-gate: cargo is absent; arm docs-stale UNMEASURED, not passed",
            file=sys.stderr,
        )
        return None
    if done.returncode == 0:
        return None
    out = (done.stdout or "") + (done.stderr or "")
    tail = out.strip().splitlines()
    # A NON-ZERO STATUS IS NOT YET A FINDING, and this is the arm where that
    # distinction is easiest to lose. `cargo run` exits non-zero when there is
    # no such binary, no lockfile, or no toolchain -- none of which is "a
    # generated doc is out of date". The gate's own self-test found this: a
    # fixture repo with a Cargo.toml but no project made docs-stale fire on
    # EVERY case including the clean one, which would have made the other
    # eleven arms unmeasurable. So the arm fires only on the check's OWN
    # language, and anything else is UNMEASURED and says so.
    if not re.search(r"not mapped|out of date|unmapped|stale", out, re.I):
        print(
            "release-gate: docs --check exited "
            f"{done.returncode} without naming an unmapped or stale path, so it "
            "did not reach a verdict; arm docs-stale UNMEASURED, not passed",
            file=sys.stderr,
        )
        return None
    return f"`rumor docs --check` exited {done.returncode}: {tail[-1] if tail else 'no output'}"


# --------------------------------------------------------------------------
# self-test: every arm driven, each asserted to fire ALONE
# --------------------------------------------------------------------------


def _mk(repo: Path, rel: str, text: str) -> None:
    path = repo / rel
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text)


def _commit(repo: Path, subject: str, body: str = "") -> None:
    git(repo, "add", "-A")
    message = subject if not body else f"{subject}\n\n{body}"
    git(repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", message)


def _world(tmp: Path, today: str) -> Path:
    """A minimal repo: tag v0.1.0, and main's version ABOVE it at 0.2.0.

    MAIN SITS ABOVE THE LATEST TAG ON PURPOSE. With main's version equal to the
    tag, a fixture that leaves the version unchanged ALSO fails
    `version-not-greater`, and the two arms can never be told apart -- which
    issue 4's "and no other reason" forbids. The gate's self-test found that
    before any branch was cut; the repair is in the fixture, not the arm.
    """
    repo = tmp / "r"
    repo.mkdir()
    git(repo, "init", "-q", "-b", "main")
    _mk(repo, "Cargo.toml", '[workspace]\n\n[workspace.package]\nversion = "0.2.0"\n')
    _mk(repo, "CHANGELOG.md", f"# Changelog\n\n## 0.2.0 -- {today}\n\nfirst.\n")
    _mk(repo, "docs/coverage.toml", '[[entry]]\npath = "src/**"\ndocs = ["docs/design.md"]\n')
    _mk(repo, "docs/design.md", "design\n")
    _mk(repo, "src/lib.rs", "// one\n")
    _commit(repo, "release: v0.1.0")
    git(repo, "tag", "v0.1.0")
    _mk(repo, "docs/design.md", "design, as shipped\n")
    _commit(repo, "docs: a second commit so main can move under a run")
    return repo


def _clean_run(repo: Path, today: str, ver: str = "0.2.1") -> None:
    _mk(repo, "Cargo.toml", f'[workspace]\n\n[workspace.package]\nversion = "{ver}"\n')
    text = (repo / "CHANGELOG.md").read_text()
    _mk(repo, "CHANGELOG.md", text.replace("# Changelog\n", f"# Changelog\n\n## {ver} -- {today}\n\na line.\n", 1))
    _commit(repo, f"release: v{ver}")


def _trailer_battery() -> int:
    """Drive docs_unchanged() over every shape a trailer arrives in.

    This is a unit battery and not a repo case because the shapes are a property
    of one function and a repo case can only show one of them per run -- and
    because the defect ibis found at r0.3/13 was INVISIBLE at the arm level: a
    truncated reason still exempts the doc, and nothing in this file reads the
    reason's value at all, so no arm could have gone red for it. What a repo case
    CAN measure is the pair below: exempt and not-exempt on the same tree, which
    is the refusal of a column-0 trailer -- the half of the fix that does change
    a verdict.

    Both directions are here. A guard that refuses a wrapped trailer can also
    eat a legitimate one -- a trailer followed immediately by another trailer at
    column 0 is the case that would break -- so that is asserted too.
    """
    long_reason = "the edit is a comment and changes no command, flag or output"
    cases: list[tuple[str, str, dict[str, str]]] = [
        (
            "one line",
            f"release: v1\n\nDocs-Unchanged: docs/a.md -- {long_reason}\n",
            {"docs/a.md": long_reason},
        ),
        (
            "an em dash separates",
            "release: v1\n\nDocs-Unchanged: docs/a.md \u2014 a reason\n",
            {"docs/a.md": "a reason"},
        ),
        (
            "an INDENTED continuation folds, whole",
            "release: v1\n\nDocs-Unchanged: docs/a.md -- the edit is a comment and changes no\n"
            "  command, flag or output\n",
            {"docs/a.md": long_reason},
        ),
        (
            "a continuation at COLUMN 0 does not exempt",
            "release: v1\n\nDocs-Unchanged: docs/a.md -- the edit is a comment and changes no\n"
            "command, flag or output\n",
            {},
        ),
        (
            "an empty reason does not exempt",
            "release: v1\n\nDocs-Unchanged: docs/a.md --\n",
            {},
        ),
        (
            "a lone hyphen is not a separator",
            "release: v1\n\nDocs-Unchanged: docs/a.md - a reason\n",
            {},
        ),
        (
            "two trailers, adjacent, BOTH survive the wrap guard",
            "release: v1\n\nDocs-Unchanged: docs/a.md -- reason one\n"
            "Docs-Unchanged: docs/b.md -- reason two\n",
            {"docs/a.md": "reason one", "docs/b.md": "reason two"},
        ),
        (
            "a folded trailer followed by another trailer",
            "release: v1\n\nDocs-Unchanged: docs/a.md -- reason one\n"
            "  folded on\n"
            "Docs-Unchanged: docs/b.md -- reason two\n",
            {"docs/a.md": "reason one folded on", "docs/b.md": "reason two"},
        ),
        (
            "prose after the trailer block ends the reason",
            "release: v1\n\nDocs-Unchanged: docs/a.md -- a reason\n\nSome closing prose.\n",
            {"docs/a.md": "a reason"},
        ),
    ]
    failures = 0
    print("  docs_unchanged() shapes:")
    for name, message, want in cases:
        got = docs_unchanged(message)
        if got == want:
            print(f"    ok   {name}")
        else:
            print(f"    FAIL {name}\n         want {want}\n         got  {got}")
            failures += 1
    return failures


def self_test() -> int:
    today = "2026-01-01"
    cases: list[tuple[str | None, str, callable]] = []

    def case(arm: str | None, label: str | None = None):
        """Register a case. `label` names it when two cases share one arm.

        The trailer pair below is two cases on the same subject -- one that
        must fire and one that must NOT -- so the arm alone stopped being a
        usable name for a case.
        """

        def wrap(fn):
            cases.append((arm, label or arm or "clean", fn))
            return fn

        return wrap

    @case(None)
    def clean(repo):
        _clean_run(repo, today)

    @case("version-unchanged")
    def version_unchanged(repo):
        # a release subject and a changelog section, and the version left alone
        _mk(repo, "src/lib.rs", "// touched\n")
        _mk(repo, "docs/design.md", "design, touched\n")
        _commit(repo, "release: v0.2.0")

    @case("subject-mismatch")
    def subject_mismatch(repo):
        _clean_run(repo, today)
        git(repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q",
            "--amend", "-m", "chore: bump the version")

    @case("commit-after-bump")
    def commit_after_bump(repo):
        _clean_run(repo, today)
        _mk(repo, "docs/design.md", "design, again\n")
        _commit(repo, "docs: a word")

    @case("version-not-greater")
    def version_not_greater(repo):
        git(repo, "tag", "v9.9.9")
        _clean_run(repo, today)

    @case("changelog-section-absent")
    def changelog_absent(repo):
        _mk(repo, "Cargo.toml", '[workspace]\n\n[workspace.package]\nversion = "0.2.1"\n')
        _commit(repo, "release: v0.2.1")

    @case("changelog-section-empty")
    def changelog_empty(repo):
        _mk(repo, "Cargo.toml", '[workspace]\n\n[workspace.package]\nversion = "0.2.1"\n')
        text = (repo / "CHANGELOG.md").read_text()
        _mk(repo, "CHANGELOG.md", text.replace("# Changelog\n", f"# Changelog\n\n## 0.2.1 -- {today}\n\n", 1))
        _commit(repo, "release: v0.2.1")

    @case("changelog-date-wrong")
    def changelog_date(repo):
        _mk(repo, "Cargo.toml", '[workspace]\n\n[workspace.package]\nversion = "0.2.1"\n')
        text = (repo / "CHANGELOG.md").read_text()
        _mk(repo, "CHANGELOG.md", text.replace("# Changelog\n", "# Changelog\n\n## 0.2.1 -- 1999-12-31\n\na line.\n", 1))
        _commit(repo, "release: v0.2.1")

    @case("docs-unchanged-no-trailer")
    def docs_no_trailer(repo):
        _mk(repo, "src/lib.rs", "// two\n")
        _clean_run(repo, today)

    @case("not-rebased")
    def not_rebased(repo):
        # main MOVES under the run. No reset: rewinding main needed a second
        # commit to exist and made the fixture depend on main's shape, where
        # what the arm is about is simply that main has advanced since.
        _clean_run(repo, today)
        run_head = git(repo, "rev-parse", "HEAD").strip()
        git(repo, "checkout", "-q", "main")
        _mk(repo, "docs/design.md", "main moved on\n")
        _commit(repo, "docs: main moved under the run")
        git(repo, "checkout", "-q", run_head)

    # Arms with no in-repo fixture, and why -- stated rather than skipped
    # silently, since an unmeasured arm counted as covered is the defect this
    # whole file is about:
    #   docs-stale            needs a cargo toolchain and the real `rumor`
    #                         binary; UNMEASURED here by construction, and
    #                         _docs_check reports UNMEASURED rather than passing
    #   doc-run-block-failed  measured below in its own case, since a `run`
    #                         block needs only sh
    #   revision-entry-missing measured below
    @case("doc-run-block-failed")
    def doc_run_fails(repo):
        _mk(repo, "docs/design.md", "design\n\n```sh run\nexit 3\n```\n")
        _clean_run(repo, today)
        # the doc changed too, so arm 9 cannot also fire
    @case("revision-entry-missing")
    def revision_missing(repo):
        # THE DOC AND ITS REVISIONS LIST MUST ALREADY BE ON MAIN. My first
        # version created both inside the run, so the gate saw an entry ADDED
        # (none existed before) and correctly did not fire -- the fixture was
        # testing the opposite of what it named. The arm is about a constant
        # moving with the list standing still.
        git(repo, "checkout", "-q", "main")
        _mk(repo, "docs/coverage.toml",
            '[[entry]]\npath = "src/**"\ndocs = ["docs/contract.md"]\n')
        _mk(repo, "docs/contract.md", "contract\n\n## Revisions\n\n- one\n")
        _commit(repo, "docs: the contract doc and its revisions list")
        git(repo, "checkout", "-q", "run")
        git(repo, "reset", "-q", "--hard", "main")
        _mk(repo, "src/lib.rs", "pub const CONTRACT_VERSION: u32 = 2;\n")
        # the mapped doc DOES change, so arm 9 cannot also fire; what is absent
        # is a new entry under Revisions
        _mk(repo, "docs/contract.md", "contract, edited\n\n## Revisions\n\n- one\n")
        _clean_run(repo, today)

    # THE TRAILER PAIR. Same tree in both -- src/lib.rs changed, its mapped
    # docs/design.md not -- differing ONLY in the trailer's shape, which is
    # ibis r0.3/1 point 3 applied to the self-test rather than only to the
    # fixture branches. Without the first case the gate could refuse every
    # trailer and still show twelve green arms.
    @case(None, "a folded trailer EXEMPTS")
    def trailer_exempts(repo):
        _mk(repo, "src/lib.rs", "// three\n")
        _mk(repo, "Cargo.toml", '[workspace]\n\n[workspace.package]\nversion = "0.2.1"\n')
        text = (repo / "CHANGELOG.md").read_text()
        _mk(repo, "CHANGELOG.md",
            text.replace("# Changelog\n", f"# Changelog\n\n## 0.2.1 -- {today}\n\na line.\n", 1))
        _commit(repo, "release: v0.2.1",
                "Docs-Unchanged: docs/design.md -- the edit is a comment and changes no\n"
                "  command, flag or output\n")

    @case("docs-unchanged-no-trailer", "a trailer wrapped at column 0 does NOT exempt")
    def trailer_wrapped(repo):
        _mk(repo, "src/lib.rs", "// three\n")
        _mk(repo, "Cargo.toml", '[workspace]\n\n[workspace.package]\nversion = "0.2.1"\n')
        text = (repo / "CHANGELOG.md").read_text()
        _mk(repo, "CHANGELOG.md",
            text.replace("# Changelog\n", f"# Changelog\n\n## 0.2.1 -- {today}\n\na line.\n", 1))
        _commit(repo, "release: v0.2.1",
                "Docs-Unchanged: docs/design.md -- the edit is a comment and changes no\n"
                "command, flag or output\n")

    failures = _trailer_battery()
    ran = 0
    for arm, label, build in cases:
        with tempfile.TemporaryDirectory() as raw:
            tmp = Path(raw)
            repo = _world(tmp, today)
            git(repo, "checkout", "-q", "-b", "run")
            build(repo)
            try:
                got = evaluate(repo, "main", "HEAD", today=today)
            except CannotRun as exc:
                print(f"  FAIL {label}: the gate could not run: {exc}")
                failures += 1
                ran += 1
                continue
            ran += 1
            ids = [found[0] for found in got]
            if arm is None:
                if ids:
                    print(f"  FAIL {label}: a run that must pass was refused: {ids}")
                    failures += 1
                else:
                    print(f"  ok   {label} passes")
                continue
            if ids == [arm]:
                print(f"  ok   {label} fires ALONE")
            elif arm in ids:
                print(f"  FAIL {label}: fired with others -- {ids}. A fixture must trip its own arm and no other")
                failures += 1
            else:
                print(f"  FAIL {label}: did not fire. got {ids or '[]'}")
                failures += 1

    covered = {arm for arm, _, _ in cases if arm}
    unmeasured = [arm for arm in ARMS if arm not in covered]
    print(f"\n  arms: {len(ARMS)}  measured here: {len(covered)}  UNMEASURED: {unmeasured}")
    print("  UNMEASURED is not covered: docs-stale needs a cargo toolchain and the")
    print("  real `rumor docs --check`, so it is driven by the fixture branch on the")
    print("  runner, not here. _docs_check reports UNMEASURED rather than passing.")
    print(f"\n  {ran - failures} of {ran} cases ok, {failures} failed")
    return RC_FINDINGS if failures else RC_CLEAN


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--repo", default=".", help="the checkout to gate")
    parser.add_argument("--base", default="origin/main", help="the ref to land on")
    parser.add_argument("--head", default="HEAD", help="the run's head")
    parser.add_argument("--today", default=None, help="override the date (tests)")
    parser.add_argument("--self-test", action="store_true")
    parser.add_argument(
        "--expect",
        default=None,
        help="exactly this arm must fire, and no other; for a fixture branch",
    )
    parser.add_argument(
        "--ids-to",
        default=None,
        metavar="PATH",
        help="write the finding ids, one per line, sorted and unique. ROOK r0.3/9: "
        "a second oracle that parses this tool's HUMAN log shares a corpus with "
        "it, and their display gate poisoned its own corpus exactly that way. "
        "The file boundary is the fix, not more parsing.",
    )
    args = parser.parse_args(argv)

    if args.self_test:
        return self_test()

    try:
        found = evaluate(Path(args.repo), args.base, args.head, today=args.today)
    except CannotRun as exc:
        print(f"release-gate: CANNOT RUN -- {exc}", file=sys.stderr)
        print(
            "release-gate: this is NOT a pass and NOT a refusal; the gate reached "
            "no verdict",
            file=sys.stderr,
        )
        return RC_CANNOT_RUN

    if args.ids_to:
        try:
            with open(args.ids_to, "w", encoding="utf-8") as fh:
                for arm in sorted({arm for arm, _ in found}):
                    fh.write(arm + "\n")
        except OSError as exc:
            print(f"release-gate: could not write {args.ids_to}: {exc}", file=sys.stderr)
            return RC_CANNOT_RUN

    for arm, detail in found:
        print(f"REFUSED [{arm}] {detail}")

    if args.expect is not None:
        if args.expect not in ARMS:
            print(f"release-gate: --expect {args.expect!r} is not an arm", file=sys.stderr)
            return RC_CANNOT_RUN
        ids = [arm for arm, _ in found]
        if ids == [args.expect]:
            print(f"PROOF the gate refused [{args.expect}] and nothing else")
            return RC_CLEAN
        print(
            f"EXPECTED exactly [{args.expect}] and got {ids or '[]'}: a fixture "
            f"branch must trip its own arm and no other",
            file=sys.stderr,
        )
        return RC_FINDINGS

    if not found:
        print("PROOF the release gate found nothing: every arm ran and none refused")
        return RC_CLEAN
    print(f"\n{len(found)} finding(s): {', '.join(arm for arm, _ in found)}")
    return RC_FINDINGS


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
