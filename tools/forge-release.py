#!/usr/bin/env python3
"""Create the forge release for a tag and attach its artefacts.

WHY A TOOL AND NOT CURL IN THE STEP (issue 27, v0.1.6). Two reasons, and the
second is the one that decided it:

  1. The release id has to be read out of a JSON response, and parsing JSON
     with grep is the defect this project has corrected twice on the forge's
     own API. `python3` is measured on the runner (release-gate.yml's probe
     reads 3.12.14) and the standing argument against python in CI is answered
     the same way there: measure it, and fail loudly when it is absent.
  2. The multipart upload is three files with their own content types. In sh
     that is a hand-rolled body; here it is a few lines that can be read.

WHAT CANNOT BE SELF-TESTED, said plainly: nothing here talks to the forge in
--self-test. What IS driven is the part that has been wrong before -- pulling
the changelog section for the version out of CHANGELOG.md -- plus the argument
checks. The network half is proven by the tag run itself, which is why issue
27's done-when is a tag and not a green local suite.
"""

import argparse
import json
import mimetypes
import os
import re
import sys
import urllib.error
import urllib.request
import uuid

UA = "rumor-tag-pipeline/1 (release publisher)"


class CannotRun(Exception):
    """The release could not be attempted. Distinct from a refusal."""


def changelog_section(text, version):
    """The body of CHANGELOG.md's section for `version`, without its heading.

    The heading shape is the one tools/release-gate.py asserts, `## X.Y.Z —
    DATE`, with an em dash or two hyphens. The section ends at the next `## `
    at column 0, NOT at the next blank line: every section here has blank
    lines inside it.
    """
    head = re.compile(
        r"^##\s+" + re.escape(version) + r"\s*(?:--|—|-)\s*\S+\s*$", re.M
    )
    found = head.search(text)
    if not found:
        return None
    rest = text[found.end():]
    nxt = re.search(r"^##\s", rest, re.M)
    body = rest[: nxt.start()] if nxt else rest
    return body.strip("\n")


def api(base, repo, token, path, method="GET", payload=None, raw=None,
        content_type=None, timeout=120):
    url = f"{base}/api/v1/repos/{repo}{path}"
    data = None
    headers = {"User-Agent": UA, "Authorization": f"token {token}"}
    if payload is not None:
        data = json.dumps(payload).encode()
        headers["Content-Type"] = "application/json"
    elif raw is not None:
        data = raw
        headers["Content-Type"] = content_type or "application/octet-stream"
    req = urllib.request.Request(url, data=data, method=method, headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:
            body = r.read()
            return r.status, (json.loads(body) if body else None)
    except urllib.error.HTTPError as exc:
        detail = exc.read()[:400].decode("utf-8", "replace")
        return exc.code, detail
    except OSError as exc:
        raise CannotRun(f"{method} {path} could not be sent: {exc}") from exc


def multipart(field, filename, blob):
    """One-file multipart/form-data body, and the boundary that goes with it."""
    boundary = "----rumor" + uuid.uuid4().hex
    ctype = mimetypes.guess_type(filename)[0] or "application/octet-stream"
    pre = (
        f"--{boundary}\r\n"
        f'Content-Disposition: form-data; name="{field}"; filename="{filename}"\r\n'
        f"Content-Type: {ctype}\r\n\r\n"
    ).encode()
    post = f"\r\n--{boundary}--\r\n".encode()
    return pre + blob + post, f"multipart/form-data; boundary={boundary}"


def publish(args):
    token = os.environ.get(args.token_env, "")
    if not token:
        raise CannotRun(
            f"{args.token_env} is empty, so this runner has no credential to create a "
            "release with. That is a PROVISIONING finding named rather than a silent "
            "skip: the artefacts are built and nothing is published"
        )
    version = args.tag[1:] if args.tag.startswith("v") else args.tag
    with open(args.body_from, encoding="utf-8") as fh:
        section = changelog_section(fh.read(), version)
    if not section:
        raise CannotRun(
            f"{args.body_from} has no section for {version}, and the epic requires the "
            "changelog section on the release. The release gate admits only a run whose "
            "changelog section exists, so this means the two disagree"
        )

    status, body = api(args.base, args.repo, token, "/releases", "POST", payload={
        "tag_name": args.tag,
        "name": args.tag,
        "body": section,
        "draft": False,
        "prerelease": version.startswith("0."),
    })
    if status not in (200, 201):
        print(f"creating the release for {args.tag} answered {status}: {body}", file=sys.stderr)
        return 1
    release_id = body.get("id")
    print(f"PROOF release {args.tag} created, id {release_id}, body {len(section)} bytes from {args.body_from}")

    for path in args.asset:
        name = os.path.basename(path)
        with open(path, "rb") as fh:
            blob = fh.read()
        if not blob:
            print(f"{name} is empty; attaching it would publish a zero-byte artefact", file=sys.stderr)
            return 1
        data, ctype = multipart("attachment", name, blob)
        status, resp = api(args.base, args.repo, token,
                           f"/releases/{release_id}/assets?name={name}",
                           "POST", raw=data, content_type=ctype)
        if status not in (200, 201):
            print(f"attaching {name} answered {status}: {resp}", file=sys.stderr)
            return 1
        print(f"PROOF attached {name}, {len(blob)} bytes")

    # READ IT BACK, because "the POST returned 201" and "the release carries
    # three assets" are different claims and only the second is the deliverable.
    status, resp = api(args.base, args.repo, token, f"/releases/{release_id}")
    if status != 200:
        print(f"reading the release back answered {status}: {resp}", file=sys.stderr)
        return 1
    names = sorted(a.get("name") for a in (resp.get("assets") or []))
    want = sorted(os.path.basename(p) for p in args.asset)
    if names != want:
        print(f"the release carries {names} and the run attached {want}: the upload "
              "reported success and the release disagrees", file=sys.stderr)
        return 1
    print(f"PROOF the release reads back with exactly {len(names)} asset(s): {', '.join(names)}")
    return 0


SECTION_CASES = (
    # (name, version, text, expected-first-line-or-None)
    ("em dash heading", "0.1.6",
     "# Changelog\n\n## 0.1.6 — 2026-10-14\n\nfirst line.\n\nsecond block.\n\n## 0.1.5 — 2026-10-14\n\nolder.\n",
     "first line."),
    ("two-hyphen heading", "0.1.6",
     "# Changelog\n\n## 0.1.6 -- 2026-10-14\n\nfirst line.\n",
     "first line."),
    ("the section is not the first", "0.1.5",
     "# Changelog\n\n## 0.1.6 — 2026-10-14\n\nnewer.\n\n## 0.1.5 — 2026-10-14\n\nolder.\n",
     "older."),
    ("blank lines inside do not end it", "0.1.6",
     "# Changelog\n\n## 0.1.6 — 2026-10-14\n\none.\n\ntwo.\n\n## 0.1.5 — 2026-10-14\n\nolder.\n",
     "one."),
    ("a missing version is None", "9.9.9",
     "# Changelog\n\n## 0.1.6 — 2026-10-14\n\none.\n", None),
    ("an empty section is None-ish", "0.1.6",
     "# Changelog\n\n## 0.1.6 — 2026-10-14\n\n## 0.1.5 — 2026-10-14\n\nolder.\n", ""),
    ("a heading with no date does not match", "0.1.6",
     "# Changelog\n\n## 0.1.6\n\none.\n", None),
)


def self_test():
    failures = 0
    examined = 0
    for name, version, text, want in SECTION_CASES:
        got = changelog_section(text, version)
        examined += 1
        first = None if got is None else (got.split("\n")[0] if got else "")
        if first == want:
            print(f"  ok   {name} -> {first!r}")
        else:
            failures += 1
            print(f"  FAIL {name} -> {first!r}, wanted {want!r}")

    # The multipart body is assembled by hand, so assert its shape rather than
    # trusting it: a missing CRLF pair is the classic silent upload corruption.
    body, ctype = multipart("attachment", "SHA256SUMS", b"abc\n")
    examined += 1
    boundary = ctype.split("boundary=")[1]
    ok = (body.startswith(f"--{boundary}\r\n".encode())
          and b'filename="SHA256SUMS"' in body
          and body.endswith(f"\r\n--{boundary}--\r\n".encode())
          and b"\r\n\r\nabc\n" in body)
    if ok:
        print("  ok   the multipart body opens, names the file and closes on its boundary")
    else:
        failures += 1
        print(f"  FAIL multipart body is malformed: {body!r}")

    print()
    print(f"  EXAMINED {examined} case(s), {failures} failed")
    if examined != 8:
        print(f"  the battery examined {examined} and declares 8: one stopped running, "
              "or one was added without moving this number")
        return 1
    if failures:
        return 1
    print("  PROOF the changelog reader and the multipart body are driven; the FORGE "
          "half of this tool is not, and the tag run is what proves it")
    return 0


def main(argv):
    parser = argparse.ArgumentParser()
    parser.add_argument("--self-test", action="store_true")
    parser.add_argument("--base", default="https://forge.axonibyte.com")
    parser.add_argument("--repo", default="axonibyte/rumor")
    parser.add_argument("--tag")
    parser.add_argument("--body-from", default="CHANGELOG.md")
    parser.add_argument("--token-env", default="RELEASE_TOKEN")
    parser.add_argument("--asset", action="append", default=[])
    args = parser.parse_args(argv)

    if args.self_test:
        return self_test()

    # USAGE ERRORS EXIT 3, NOT argparse's 2. The caller maps 2 to "could not be
    # attempted", a PROVISIONING finding, and a tool invoked wrongly is not
    # that: it is a defect in the workflow calling it. Two different causes
    # behind one status is the shape this whole pipeline keeps correcting.
    if not args.tag:
        print("forge-release: --tag is required", file=sys.stderr)
        return 3
    if not args.asset:
        print("forge-release: --asset is required at least once: a release with "
              "nothing attached is the gap this tool closes", file=sys.stderr)
        return 3
    try:
        return publish(args)
    except CannotRun as exc:
        print(f"forge-release: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
