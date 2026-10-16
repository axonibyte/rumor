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
import shutil
import subprocess
import sys
import typing
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
# consumer of this map in this file is arm 9's exemption lookup, which reads the
# KEYS; the gate never prints a reason, and nothing else in the repository reads
# the trailer at all. (The lookup is now keyed by (commit, doc) rather than by
# doc -- see arm 9 -- but it still reads only keys.) So the corrupted value had no consumer here, and no
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


class Trailers(typing.NamedTuple):
    """What a commit message's trailers grant, AND what they claimed in vain.

    `malformed` exists because arm 9's finding used to say "with no
    `Docs-Unchanged` trailer in the run for them", which is flatly FALSE when a
    trailer is present and unreadable -- ibis r0.3/22: an author whose reason
    wrapped reads that, sees the reason sitting in their own commit message, and
    concludes the gate is broken. Refusing the exemption is right; refusing it
    while describing the message incorrectly is a second defect, and the author
    is the person least able to tell them apart.
    """

    exempt: dict[str, str]
    malformed: list[str]


def docs_unchanged(message: str) -> Trailers:
    """{doc: reason} from a commit message, folding wrapped reasons as git does.

    A reason continues onto any following line that begins with whitespace, which
    is git's own trailer-continuation rule. A trailer whose reason folds to
    nothing is not an exemption and is dropped, so an empty `Docs-Unchanged:`
    cannot silently excuse a doc.

    A MESSAGE WHOSE TRAILER BLOCK IS BROKEN EXEMPTS NOTHING, not merely the
    broken trailer. Measured against `git interpret-trailers --parse` on my own
    two controls: for the column-0 wrap git emits NO trailers at all -- an
    unindented non-trailer line ends the block, it does not truncate one entry --
    while this reader had kept the OTHER, well-formed trailer in the same
    message. That made it WIDER than git on exactly the malformed input, and the
    exemption is the one direction worth being strict in (ibis r0.3/8). So the
    whole message is refused. The valid shape is pinned to git by a case in the
    battery that runs both parsers and asserts they agree, rather than to my
    reading of git's documentation.
    """
    out: dict[str, str] = {}
    malformed: list[str] = []
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
            # A HARD-WRAPPED TRAILER BREAKS THE BLOCK, AND A BROKEN BLOCK
            # EXEMPTS NOTHING. git's continuation is an INDENTED line; a reason
            # wrapped to column 0 is not one, and `git interpret-trailers
            # --parse` on that message emits NO trailers whatever -- the
            # unindented line ends the trailer block rather than truncating one
            # entry. Measured on my own two controls. Dropping only the wrapped
            # trailer left this reader WIDER than git on malformed input, since
            # the other, well-formed trailer in the same message survived here
            # and not there; widening the exemption is the one direction worth
            # refusing. So the message is abandoned whole, which makes arm
            # docs-unchanged-no-trailer fire for every doc it claimed.
            nxt = lines[idx + 1] if idx + 1 < len(lines) else ""
            if nxt.strip() and not nxt[:1].isspace() and not _TRAILER_HEAD.match(nxt.strip()):
                print(
                    f"release-gate: the Docs-Unchanged trailer for {doc} is "
                    f"followed by an unindented line, which ends the trailer "
                    f"block -- `git interpret-trailers` reads NO trailers from "
                    f"this message. Nothing in it exempts. Put each reason on "
                    f"one line, or indent the continuation.",
                    file=sys.stderr,
                )
                # EVERY doc the broken message claimed is reported malformed,
                # not just the wrapped one: git discards the whole block, so the
                # well-formed sibling's author is equally entitled to be told why
                # their trailer did nothing.
                claimed = [doc]
                for other in lines[idx + 1 :]:
                    if other[:1].isspace():
                        continue
                    head = _TRAILER_HEAD.match(other.strip())
                    if head:
                        claimed.append(head.group("doc"))
                return Trailers({}, sorted(set(list(out) + claimed)))
            continue
        if doc is not None and line[:1].isspace() and line.strip():
            parts.append(line)
            continue
        if doc is not None:
            flush()
            doc, parts = None, []
    flush()
    return Trailers(out, malformed)


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


_VERSION_LINE = re.compile(r"^(?P<ver>[0-9]+(?:\.[0-9]+)*)$")


def project_version(text: str | None) -> str | None:
    """The version from the VERSION file. None when it cannot be read.

    WHY A FILE AND NOT THE MANIFEST, now that the project is Go (G1, issue 32).
    go.mod has no version field -- a Go module takes its version from the tag --
    so the manifest cannot be the second source. Dropping to a two-way tag
    against changelog check was the other option and it WEAKENS the gate: the
    epic's bullet refuses a run where "the tag version, the manifest version and
    the changelog heading disagree", which is a three-way agreement, and two of
    three cannot express it. So the version lives in one committed file that
    nothing else writes.

    STRICT ON PURPOSE, and the strictness is the point rather than tidiness: a
    lenient reader turning "0.1.4 " or "v0.1.4" or two lines into a version
    would let a malformed bump agree with a tag by accident. Anything but a
    single line of dot-separated digits reads as unreadable, which trips the
    version-unchanged arm by name instead of passing quietly.
    """
    if not text:
        return None
    # NO .strip() ON EITHER LINE, and that is the fix rather than an oversight.
    # Both were there and the pair test found what they cost: the tag pipeline's
    # shell reader REFUSED "0.1.4 " while this one ACCEPTED it, so a release
    # could clear this gate and then fail at the tag -- late, and for a reason
    # the gate had already seen and swallowed. `if line` rather than
    # `if line.strip()` for the same reason: `grep -c .` on the other side
    # counts a whitespace-only line, so filtering it here would make the two
    # disagree about how many lines the file has.
    # split("\n"), NOT splitlines(): splitlines treats a lone \r as a line
    # terminator and would read "0.1.4\r\n" as clean, while the tag pipeline's
    # `head -1` keeps the \r and refuses it. A CRLF version file is malformed
    # on this project either way; the point is that both readers must say so,
    # and the pair test found this one too.
    lines = [line for line in text.split("\n") if line]
    if len(lines) != 1:
        return None
    found = _VERSION_LINE.match(lines[0])
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
    head_version = project_version(show(repo, bump_rev, "VERSION"))
    base_version = project_version(show(repo, merge_base, "VERSION"))

    # --- arm 1: the bump does not change the workspace version ------------
    if head_version is None:
        add(
            "version-unchanged",
            "the project version could not be read from VERSION at the "
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
            f"the subject says v{found.group('ver')} and VERSION says "
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
    # A RUN'S TRAILERS ARE ITS OWN RUN'S, AND A WIDE BASE LENDS YOU ANOTHER'S.
    #
    # Keyed by doc and unioned over the WHOLE range, as this was until quill
    # measured it (thread:rumor/645), an exemption written for one release
    # answered for a later one: with 6a27f60 in range, rook's v0.1.12 bump
    # trailer for docs/release.md covered ibis's unexplained VERSION change at
    # 8feb0db, two releases on. Same head, only the base moved.
    #
    # THE COMMIT IS THE WRONG BOUNDARY AND THE MEASUREMENT SAYS SO. Requiring
    # the trailer's own commit to have touched the path refuses the project's
    # convention outright: three of four trailers in v0.1.10..main sit on a
    # BUMP that touched only CHANGELOG.md and VERSION, because a reason is
    # written about the RUN and the bump is the one commit that exists per run
    # and is written last, when the range is known. Driven against each
    # release's own tool as the control, that rule refused v0.1.12 (7 paths),
    # v0.1.13 (3) and the queued v0.1.14 (1) -- all three accepted or green
    # (rook /659 /663, fable's withdrawal at /683).
    #
    # SO THE BOUNDARY IS THE RELEASE. A bump from an EARLIER release is only in
    # range because the base was stale or the branch was cut from another
    # release's branch; it is not part of this run in any sense the convention
    # means. The union therefore starts after the last bump strictly before this
    # run's own, and the bump-for-the-run convention stands untouched. Driven
    # twice independently, rook /666 and quill /668, agreeing on every row:
    # v0.1.11..v0.1.14 admitted, 8feb0db's stale-base lending refused on four.
    prior_bump = None
    if bump_idx is not None:
        for idx, rev in enumerate(revs[:bump_idx]):
            if _SUBJECT.match(subjects[rev]):
                prior_bump = idx
    run_revs = revs if prior_bump is None else revs[prior_bump + 1 :]
    trailers: dict[str, str] = {}
    malformed_trailers: list[str] = []
    for rev in run_revs:
        read = docs_unchanged(bodies[rev])
        trailers.update(read.exempt)
        malformed_trailers.extend(read.malformed)
    # THE BUMP'S OWN MECHANICS ARE NOT A SOURCE CHANGE, and this narrowing is
    # exactly as wide as its reason. A bump commit must change the version file
    # (and under Cargo, the lock with it); if that counted as "a source path
    # changed", EVERY well-formed release run would need a `Docs-Unchanged`
    # trailer for whatever that file maps to, and the gate would refuse the
    # thing it exists to admit. Driving it against the accepted v0.1.2 run is
    # what showed this. The exemption is narrow in two ways: only these paths,
    # and only when the BUMP is the sole commit that touched them -- a run that
    # edits one of them in its body still answers for its mapped docs.
    #
    # VERSION IS THE ONLY ENTRY (G1 added it, G4 narrowed to it): it is the file
    # a bump must change, so it inherits the whole of the argument above and
    # nothing more. Cargo.toml and Cargo.lock were here while the Rust tree was,
    # and went out with it in the same commit -- an exemption for a file that
    # cannot change is an exemption nobody can audit, and a reader would have to
    # go to git to learn whether it still covered anything.
    MECHANICS = ("VERSION",)
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
            # NAME THE MALFORMED TRAILER RATHER THAN DENYING IT EXISTS. "no
            # trailer in the run for them" is false when one is present and
            # unreadable, and the author -- who can see the reason in their own
            # message -- would read the finding as a bug in the gate.
            claimed = [doc for doc in missing if doc in malformed_trailers]
            if claimed:
                # a SENTENCE BREAK, not a comma: this clause is a second
                # statement about the run, and ", A trailer IS present" read as
                # a splice in the finding the author has to act on.
                why = (
                    ". "
                    f"A `Docs-Unchanged` trailer for {', '.join(claimed)} IS "
                    f"present in the run but was not readable: a reason wrapped "
                    f"to column 0 ends the trailer block, so `git "
                    f"interpret-trailers` reads no trailers from that message "
                    f"at all (exit 0, no output) and nothing in it exempts "
                    f"anything. Indent the continuation, or put the reason on "
                    f"one line. This is not a missing trailer"
                )
            else:
                why = (
                    ", with no `Docs-Unchanged: <doc> -- <reason>` trailer in "
                    "the run for them"
                )
            add(
                "docs-unchanged-no-trailer",
                f"{path} changed and its mapped doc(s) {', '.join(missing)} did "
                f"not{why}. The mapping is read from docs/coverage.toml, which "
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


# THE CHECK'S OWN REFUSAL LANGUAGE, AND IT WAS WRONG UNTIL A FIXTURE RAN.
#
# This predicate was `not mapped|out of date|unmapped|stale`, and `rumor docs
# --check` emits NONE of those words. Its two refusals are:
#
#   "<path> matches no entry in docs/coverage.toml; add one naming the doc ..."
#   "coverage entry `<glob>` matches no path in the workspace; it is dead ..."
#
# so the arm could never fire on real output -- in the Rust tree either, since
# the message is the same there. It was invisible because docs-stale had never
# been measured: no toolchain on the runner, and the self-test's synthetic repos
# have no project to run. A filter that cannot match its own tool's words is the
# "always UNMEASURED" shape, which reads exactly like "always passes".
#
# The phrases below are DUPLICATED FROM THE BINARY DELIBERATELY rather than
# imported or derived: a predicate that read its vocabulary from the thing it
# judges would agree with it whatever either said. The duplication is the check,
# and the battery pins it against both real messages and against the
# broken-tool output it must NOT accept.
_DOCS_VERDICT = re.compile(
    r"matches no entry|matches no path|not mapped|unmapped|out of date|stale",
    re.I,
)


def _docs_check_reached_a_verdict(out: str) -> bool:
    """Did `docs --check` refuse, as opposed to failing to run?

    A non-zero status alone does not say: `go run` exits non-zero when the
    package is missing or the module is broken, and none of that is a finding
    about documentation.
    """
    return bool(_DOCS_VERDICT.search(out))


def _docs_check(repo: Path, ref: str) -> str | None:
    """`rumor docs --check` for the run at `ref`, or None when it cannot answer.

    CANNOT RUN IS NOT A FINDING. If no Go toolchain is present -- true of any
    box without one -- the arm reports nothing rather than refusing, and says so
    on stderr. A gate that refuses because its own tool is missing is the
    "always-refuse" mutant that makes every other arm meaningless.

    THE TOOLCHAIN IS RESOLVED, NOT NAMED (G1, issue 32). FreeBSD ports installs
    versioned binaries, so this jail has go126 and go125 and NO bare `go`, while
    forge-01_jail has the bare one. The candidate list is the same order ci.yml
    uses, and GOTOOLCHAIN=local goes with it for the reason recorded there: with
    the default `auto`, a go directive above the installed toolchain is
    satisfied by silently downloading another, so a gate could measure a tree
    with a compiler nobody chose.

    THIS ARM READS THE CHECKOUT, SO IT VERIFIES THE CHECKOUT IS `ref`. Every
    other arm asks git about the range and so is independent of what is checked
    out; this one has to build and run `rumor`, and there is no way to ask a
    compiled binary about a ref without putting that ref in a tree. `git
    worktree` is not available as a route: `docs --check` fails inside one, because `.git`
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
    # go.mod, not Cargo.toml: this arm runs the real `rumor docs --check`, and
    # since G1 that binary is built from the Go module. A synthetic fixture with
    # neither is not a project and the arm reports UNMEASURED rather than
    # passing on it.
    if not (Path(repo) / "go.mod").exists():
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
    go = None
    for candidate in ("go", "go126", "go125"):
        if shutil.which(candidate):
            go = candidate
            break
    if go is None:
        print(
            "release-gate: no go, go126 or go125 on PATH; arm docs-stale "
            "UNMEASURED, not passed",
            file=sys.stderr,
        )
        return None
    env = dict(os.environ, GOTOOLCHAIN="local")
    try:
        done = subprocess.run(
            [go, "run", "./cmd/rumor", "docs", "--check"],
            cwd=str(repo),
            capture_output=True,
            text=True,
            timeout=300,
            check=False,
            env=env,
        )
    except subprocess.TimeoutExpired:
        print(
            "release-gate: docs --check exceeded 300s; arm docs-stale UNMEASURED",
            file=sys.stderr,
        )
        return None
    except OSError:
        print(
            f"release-gate: {go} could not be executed; arm docs-stale "
            "UNMEASURED, not passed",
            file=sys.stderr,
        )
        return None
    if done.returncode == 0:
        return None
    out = (done.stdout or "") + (done.stderr or "")
    tail = out.strip().splitlines()
    # A NON-ZERO STATUS IS NOT YET A FINDING, and this is the arm where that
    # distinction is easiest to lose. `go run` exits non-zero when the package
    # does not exist, the module is broken, or the toolchain refuses -- none of
    # which is "a generated doc is out of date". The gate's own self-test found
    # this: a fixture repo with a manifest but no project made docs-stale fire on
    # EVERY case including the clean one, which would have made the other
    # eleven arms unmeasurable. So the arm fires only on the check's OWN
    # language, and anything else is UNMEASURED and says so.
    if not _docs_check_reached_a_verdict(out):
        print(
            "release-gate: docs --check exited "
            f"{done.returncode} without naming an unmapped or stale path, so it "
            "did not reach a verdict; arm docs-stale UNMEASURED, not passed",
            file=sys.stderr,
        )
        return None
    # THE LINE THAT ANSWERED, NOT THE LAST LINE. `go run` appends its own
    # "exit status 1" after the binary's output, so tail[-1] named go run's
    # status and nothing about the tree -- measured on the first fixture run:
    # "REFUSED [docs-stale] `rumor docs --check` exited 1: exit status 1", a
    # finding nobody can act on without re-running it by hand. Report the line
    # the verdict predicate matched, which is the one carrying the path.
    reason = next(
        (line.strip() for line in tail if _docs_check_reached_a_verdict(line)),
        tail[-1] if tail else "no output",
    )
    return f"`rumor docs --check` exited {done.returncode}: {reason}"


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
    _mk(repo, "VERSION", '0.2.0\n')
    _mk(repo, "CHANGELOG.md", f"# Changelog\n\n## 0.2.0 -- {today}\n\nfirst.\n")
    _mk(repo, "docs/coverage.toml", '[[entry]]\npath = "src/**"\ndocs = ["docs/design.md"]\n')
    _mk(repo, "docs/design.md", "design\n")
    _mk(repo, "src/lib.rs", "// one\n")
    _commit(repo, "release: v0.1.0")
    git(repo, "tag", "v0.1.0")
    _mk(repo, "docs/design.md", "design, as shipped\n")
    _commit(repo, "docs: a second commit so main can move under a run")
    return repo


def _amend_trailer(repo: Path, trailer: str) -> None:
    """Put a trailer on the commit just made, without rewriting its subject.

    The fixtures need a reason ON THE BUMP, which is where this project writes
    them; _clean_run commits first, so the trailer is amended in rather than
    threaded through every caller.
    """
    # THE IDENTITY IS PASSED, AS _commit DOES, AND NOT INHERITED. A bare amend
    # takes the committer from configuration, so this passed in a clone that has
    # one and exited 128 on the runner -- "Committer identity unknown ... got
    # 'act_runner@ci.(none)'" -- which failed the gate's own self-test and so
    # refused a PR for a reason that was never about the PR (run 238, #57).
    msg = git(repo, "log", "-1", "--format=%B").rstrip("\n")
    git(repo, "-c", "user.name=t", "-c", "user.email=t@t",
        "commit", "-q", "--amend", "-m", msg + "\n\n" + trailer.rstrip("\n"))


def _clean_run(repo: Path, today: str, ver: str = "0.2.1") -> None:
    _mk(repo, "VERSION", f"{ver}\n")
    text = (repo / "CHANGELOG.md").read_text()
    _mk(repo, "CHANGELOG.md", text.replace("# Changelog\n", f"# Changelog\n\n## {ver} -- {today}\n\na line.\n", 1))
    _commit(repo, f"release: v{ver}")


# THE SELF-TEST ASSERTS ITS OWN SIZE, EXACTLY, IN BOTH DIRECTIONS.
#
# ibis r0.3/25: the summary line counted the REPO cases only, so a battery that
# silently stopped running still printed "14 of 14 cases ok, 0 failed" and
# satisfied the workflow's grep -- the uncounted-UNMEASURED hole from r0.3/22,
# relocated one level out. Driven by stubbing the battery to a no-op: rc 0, grep
# PASS.
#
# AN EXPECTED COUNT, NOT A FLOOR (rook r0.3/26, who runs EXPECT = 54 and
# EXPECT = 102 in his own suites). A floor catches a deletion but accepts any
# addition, which lets the constant go stale until it no longer describes
# anything -- and a count is a two-sided quantity, so `>=` is the wrong guard
# for it: too few means checks vanished, too many means checks were added
# without the declaration being updated, and each is its own failure with its
# own message. Any self-consistent `N of N` is satisfiable by shrinking N, which
# is why the number is absolute and lives here rather than being a shape the
# workflow's grep happens to match.
#
# Changing either number is a deliberate edit that has to say what it added or
# removed.
# 14 -> 16 with G1's two VERSION arms: version_file_is_malformed, which drives
# project_version's strictness, and a_mapped_version_file_is_still_exempt, which
# drives the MECHANICS exemption. Both were added because a mutant survived
# without them, so the count moves by exactly two and names them.
# THE DOCS-VERDICT BATTERY. Real strings, copied from `rumor docs --check` by
# hand, against the broken-tool output the predicate must refuse. This is the
# arm that would have caught the filter matching none of its tool's words.
DOCS_VERDICT_CASES = (
    ("stowaway.txt matches no entry in docs/coverage.toml; add one naming the "
     "doc that describes it", True),
    ("coverage entry `docs/*.tml` matches no path in the workspace; it is dead, "
     "and a dead entry hides a typo", True),
    ("docs --check: 42 path(s), every one mapped in docs/coverage.toml", False),
    ("no required module provides package ./cmd/rumor", False),
    ("go: cannot find main module", False),
    ("build failed", False),
    ("", False),
)


def _docs_verdict_battery() -> tuple[int, int]:
    """Drive _docs_check_reached_a_verdict over real strings. (failures, ran)"""
    failures = 0
    for out, want in DOCS_VERDICT_CASES:
        got = _docs_check_reached_a_verdict(out)
        if got is not want:
            failures += 1
            print(f"\n  FAIL docs-verdict: {out[:60]!r} read as "
                  f"{'a verdict' if got else 'not a verdict'}, wanted "
                  f"{'a verdict' if want else 'not a verdict'}")
    return failures, len(DOCS_VERDICT_CASES)


# 7, and it moves only with DOCS_VERDICT_CASES: two real refusals, the real
# success line, three broken-tool outputs and the empty string.
EXPECT_DOCS_VERDICT_CHECKS = 7

EXPECT_CASES = 19
EXPECT_TRAILER_CHECKS = 15


def _trailer_battery() -> tuple[int, int]:
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
            "a BROKEN block exempts nothing, not even its well-formed sibling",
            "release: v1\n\nDocs-Unchanged: docs/a.md -- the edit is a comment and changes no\n"
            "command, flag or output\n"
            "Docs-Unchanged: docs/b.md -- a perfectly good reason\n",
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
    ran = 0
    print("  docs_unchanged() shapes:")
    for name, message, want in cases:
        got = docs_unchanged(message).exempt
        ran += 1
        if got == want:
            print(f"    ok   {name}")
        else:
            print(f"    FAIL {name}\n         want {want}\n         got  {got}")
            failures += 1

    # THE REFUSAL MUST ALSO BE EXPLAINED. Exempting nothing is correct and
    # insufficient: the author can see the reason in their own message, so the
    # finding has to say the trailer was present and unreadable (ibis r0.3/22).
    # Asserted on the DOCS NAMED, since that is what reaches the finding text.
    for name, message, want in (
        ("the wrapped doc is named malformed",
         "release: v1\n\nDocs-Unchanged: docs/a.md -- a reason that\nwrapped\n",
         ["docs/a.md"]),
        ("a well-formed SIBLING in a broken block is named too",
         "release: v1\n\nDocs-Unchanged: docs/a.md -- a reason that\nwrapped\n"
         "Docs-Unchanged: docs/b.md -- a good reason\n",
         ["docs/a.md", "docs/b.md"]),
        ("a well-formed message names nothing malformed",
         "release: v1\n\nDocs-Unchanged: docs/a.md -- a reason\n",
         []),
    ):
        got = docs_unchanged(message).malformed
        ran += 1
        if got == want:
            print(f"    ok   {name}")
        else:
            print(f"    FAIL {name}\n         want {want}\n         got  {got}")
            failures += 1

    # PINNED TO GIT, NOT TO MY READING OF GIT. The folding rule is a claim about
    # what another tool does, so it is asserted against that tool: both parsers
    # run on the same two messages and must agree on the set of exempted docs
    # AND on each reason, byte for byte. This is what caught the reader being
    # WIDER than git on the malformed shape -- reasoning from git's docs had
    # given me a rule that kept a trailer git discards.
    for name, message in (
        ("folded", "release: v1\n\nDocs-Unchanged: docs/a.md -- "
                   "the edit is a comment and changes no\n  command, flag or output\n"),
        # THE BROKEN MESSAGE CARRIES A WELL-FORMED SIBLING ON PURPOSE. With a
        # single trailer both the right rule and the WIDER one yield {}, so the
        # agreement check could not see the divergence that actually happened
        # here -- it was the sibling shape case that caught it. Measured by
        # restoring the wider rule and watching which case failed (ibis
        # r0.3/22's first vacuity question). The cross-check now covers the real
        # defect rather than a neighbouring one.
        ("broken, with a well-formed sibling",
         "release: v1\n\nDocs-Unchanged: docs/a.md -- "
         "the edit is a comment and changes no\ncommand, flag or output\n"
         "Docs-Unchanged: docs/b.md -- a perfectly good reason\n"),
    ):
        ran += 1
        try:
            done = subprocess.run(
                ["git", "interpret-trailers", "--parse"],
                input=message, capture_output=True, text=True, timeout=60, check=False,
            )
        except (OSError, subprocess.TimeoutExpired) as exc:
            # GIT ABSENT IS A FAILED SELF-TEST, NOT AN UNMEASURED ARM. Elsewhere
            # in this file "cannot run" is reported rather than counted, because
            # a missing toolchain must not refuse a release. Here the opposite holds:
            # every other arm asks git about the range, so a gate on a box
            # without git cannot reach any verdict at all. Letting this case
            # print UNMEASURED and pass would have let the workflow's
            # "0 failed" grep succeed with the agreement unchecked -- ibis
            # r0.3/22's second vacuity question, and it was a real hole.
            print(f"    FAIL git agreement, {name} shape: git could not be run: {exc}")
            failures += 1
            continue
        theirs = {}
        for line in done.stdout.splitlines():
            found = _TRAILER_HEAD.match(line.strip())
            if found:
                theirs[found.group("doc")] = found.group("rest").strip()
        mine = docs_unchanged(message).exempt
        if mine == theirs:
            print(f"    ok   git agreement, {name} shape: both read {sorted(mine) or '[]'}")
        else:
            print(f"    FAIL git agreement, {name} shape\n"
                  f"         git  {theirs}\n         mine {mine}")
            failures += 1

    return failures, ran


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

    # ADDED WITH G1's MOVE TO A VERSION FILE, and both exist because the
    # mutants SURVIVED without them. Making project_version accept any first
    # line, and dropping VERSION from MECHANICS, both left this suite at 14 of
    # 14 -- two paths written and justified in this file with nothing reaching
    # them. rook, go-port.
    @case("version-unchanged")
    def version_file_is_malformed(repo):
        # The strictness project_version's docstring claims. `v0.2.1` is the
        # shape that matters: it is what a tag looks like, so a lenient reader
        # would make the tag agree with the version file by accident -- the one
        # disagreement this gate exists to catch.
        _mk(repo, "VERSION", "v0.2.1\n")
        text = (repo / "CHANGELOG.md").read_text()
        _mk(repo, "CHANGELOG.md",
            text.replace("# Changelog\n", f"# Changelog\n\n## 0.2.1 -- {today}\n\na line.\n", 1))
        _commit(repo, "release: v0.2.1")

    @case(None)
    def a_mapped_version_file_is_still_exempt(repo):
        # The MECHANICS exemption, which the stock fixture cannot reach: its
        # coverage map names only src/**, so VERSION answers for no docs and the
        # arm has nothing to demand. Mapped, a bump touching VERSION would
        # otherwise need a Docs-Unchanged trailer for docs/release.md -- which is
        # the gate refusing the thing it exists to admit. Drop VERSION from
        # MECHANICS and this case goes red; that is the whole point of it.
        _mk(repo, "docs/coverage.toml",
            '[[entry]]\npath = "src/**"\ndocs = ["docs/design.md"]\n'
            '\n[[entry]]\npath = "VERSION"\ndocs = ["docs/release.md"]\n')
        _commit(repo, "docs: map the version file")
        _clean_run(repo, today)

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
        _mk(repo, "VERSION", '0.2.1\n')
        _commit(repo, "release: v0.2.1")

    @case("changelog-section-empty")
    def changelog_empty(repo):
        _mk(repo, "VERSION", '0.2.1\n')
        text = (repo / "CHANGELOG.md").read_text()
        _mk(repo, "CHANGELOG.md", text.replace("# Changelog\n", f"# Changelog\n\n## 0.2.1 -- {today}\n\n", 1))
        _commit(repo, "release: v0.2.1")

    @case("changelog-date-wrong")
    def changelog_date(repo):
        _mk(repo, "VERSION", '0.2.1\n')
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
    #   docs-stale            needs a Go toolchain AND a tree holding the
    #                         module, which a synthetic repo is not; UNMEASURED
    #                         here by construction, and _docs_check reports
    #                         UNMEASURED rather than passing
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
        _mk(repo, "VERSION", '0.2.1\n')
        text = (repo / "CHANGELOG.md").read_text()
        _mk(repo, "CHANGELOG.md",
            text.replace("# Changelog\n", f"# Changelog\n\n## 0.2.1 -- {today}\n\na line.\n", 1))
        _commit(repo, "release: v0.2.1",
                "Docs-Unchanged: docs/design.md -- the edit is a comment and changes no\n"
                "  command, flag or output\n")

    @case("docs-unchanged-no-trailer", "a trailer wrapped at column 0 does NOT exempt")
    def trailer_wrapped(repo):
        _mk(repo, "src/lib.rs", "// three\n")
        _mk(repo, "VERSION", '0.2.1\n')
        text = (repo / "CHANGELOG.md").read_text()
        _mk(repo, "CHANGELOG.md",
            text.replace("# Changelog\n", f"# Changelog\n\n## 0.2.1 -- {today}\n\na line.\n", 1))
        _commit(repo, "release: v0.2.1",
                "Docs-Unchanged: docs/design.md -- the edit is a comment and changes no\n"
                "command, flag or output\n")

        # THE PAIR THAT PINS THE RELEASE BOUNDARY, both arms on one shape: a range
    # holding an EARLIER release's bump, whose trailer is the only one offered
    # for the later run's unexplained path. It must not lend. The twin is the
    # same range with the trailer on THIS run's own bump, which must still
    # exempt -- the project's convention, and a rule that broke it would refuse
    # every release already accepted (fable /683, after rook's accepted-release
    # control refused v0.1.12, v0.1.13 and the queued v0.1.14).
    @case("docs-unchanged-no-trailer", "an earlier bump's trailer does NOT lend to a later run")
    def earlier_bump_does_not_lend(repo):
        # The earlier release's own change touches an UNMAPPED path on purpose:
        # a mapped one would be unexplained in this range too and the fixture
        # would trip its arm twice, which issue 4's rule forbids.
        _mk(repo, "notes/one.txt", "the earlier release's own change\n")
        _clean_run(repo, today, ver="0.2.1")          # bump 1, carries the reason
        _amend_trailer(repo, "Docs-Unchanged: docs/design.md -- the earlier run's reason\n")
        _mk(repo, "src/two.rs", "// the later release's change, unexplained\n")
        _clean_run(repo, today, ver="0.2.2")          # bump 2, no trailer

    @case(None, "this run's own bump trailer DOES exempt, with an earlier bump in range")
    def own_bump_trailer_exempts(repo):
        _mk(repo, "notes/one.txt", "the earlier release's own change\n")
        _clean_run(repo, today, ver="0.2.1")
        _amend_trailer(repo, "Docs-Unchanged: docs/design.md -- the earlier run's reason\n")
        _mk(repo, "src/two.rs", "// the later release's change\n")
        _clean_run(repo, today, ver="0.2.2")
        _amend_trailer(repo, "Docs-Unchanged: docs/design.md -- THIS run's reason\n")

    # THE ACCEPTED-RELEASE CONTROL, which is the arm this project proved a
    # tightening needs (fable /683): the ordinary shape of every run in
    # v0.1.10..main -- a feature commit touching a source path, and the reason
    # on the BUMP, which touched only VERSION and CHANGELOG.md. A rule that
    # refuses this refuses the history, and two did before this one.
    @case(None, "the convention itself: a feature commit, the reason on the bump")
    def the_conventional_run(repo):
        _mk(repo, "src/two.rs", "// a change whose doc does not move\n")
        _clean_run(repo, today)
        _amend_trailer(repo, "Docs-Unchanged: docs/design.md -- the edit is a comment\n")

    battery_failures, trailer_checks = _trailer_battery()
    docs_failures, docs_checks = _docs_verdict_battery()
    failures = 0
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
    print("  UNMEASURED is not covered: docs-stale needs a Go toolchain AND a tree")
    print("  holding the module, which a synthetic fixture repo is not, so it is")
    print("  driven by the fixture branch. _docs_check reports UNMEASURED, not pass.")
    # THE SIZE ASSERTIONS, BEFORE THE SUMMARY, so a wrong count cannot be
    # reported as a clean run. Each direction gets its own sentence: a count
    # below the expectation means a case stopped running, above it means one was
    # added without updating the declaration that makes the number load-bearing.
    size_failures = 0
    for label, got, want in (("case", ran, EXPECT_CASES),
                             ("trailer check", trailer_checks, EXPECT_TRAILER_CHECKS),
                             ("docs-verdict check", docs_checks,
                              EXPECT_DOCS_VERDICT_CHECKS)):
        if got == want:
            continue
        how = "FEWER than" if got < want else "MORE than"
        print(f"\n  FAIL {got} {label}(s) ran, {how} the declared "
              f"{want}. "
              + ("A case that stops running is invisible in a self-consistent "
                 "count, so the number is declared and asserted."
                 if got < want else
                 "Raise the declared count in the same change that adds the "
                 "case, or the number stops describing anything."))
        size_failures += 1

    # BOTH TALLIES ON ONE LINE AND KEPT SEPARATE. Folding battery failures into
    # the case counter made a failed shape check report "13 of 14 cases ok" when
    # all 14 cases passed -- a count that says something untrue, which is the
    # defect this whole round is about.
    total = failures + battery_failures + docs_failures + size_failures
    print(f"\n  {ran - failures} of {ran} cases ok, "
          f"{trailer_checks - battery_failures} of {trailer_checks} trailer "
          f"checks ok, {docs_checks - docs_failures} of {docs_checks} "
          f"docs-verdict checks ok, {total} failed")
    return RC_FINDINGS if total else RC_CLEAN


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
