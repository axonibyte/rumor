#!/usr/bin/env python3
"""Refuse a workflow that can hang the runner, or that cannot report why.

WHY THIS EXISTS, in one sentence: on 2026-10-10 rook's `probe-pkg` drove three
unbounded `pkg` calls on forge-01_jail, ran for 17+ minutes against 0-2 s for
every other probe in the batch, and -- because that runner is
single-concurrency -- head-of-line blocked three queued runs and every push
anyone else would have made. The epic's standing rule already said "any task
that drives an external process carries a timeout and writes a hang report; a
hang is recorded as a finding, never waited out." The rule was written down and
still broken, in the probe AND in ci.yml itself, so the rule needed an
instrument rather than another reader.

WHAT IT CHECKS
  1. every job declares `timeout-minutes`, so the JOB is bounded whatever hangs
     inside it -- including a hang in something this checker does not know about
  2. every invocation of a network or build tool carries its own bound, either a
     `timeout` wrapper or the tool's own hard equivalent (curl -m)
  3. any `run:` block containing a PIPE sets `set -o pipefail`. Without it a
     pipeline's status is its LAST stage's, so a failing gate piped to a
     formatter exits 0 and the step goes GREEN. Driven, not reasoned: under
     `bash -e`, a script whose gate exits 4 piped to `tail -5` continues and
     exits 0; add `set -o pipefail` and the same script exits 4. ratchet found
     this check missing at r0.2/12 and the gap is the instructive part -- the
     first three checks refused the CORRECTION I had made to the pipe defect
     (a bare `rc=$?`) and not the DEFECT ITSELF, so a pipeline was still
     green-on-failure after the fix, by a different route, with the identical
     symptom: no log and a passing step. `pipefail` rather than "do not pipe",
     because the formatting is worth keeping and one line at the top of the
     block makes the pipeline honest.
  4. no EARLY-CLOSING CONSUMER in a gate pipeline. ratchet r0.2/14 drew the
     distinction and it is the one that makes `pipefail` safe to require:

       a GATE pipeline's status IS the verdict. `pipefail` on, and nothing in it
       that can close the pipe before the producer finishes -- no `head`, no
       `grep -q`, no `grep -m`. Format from a FILE afterwards instead.
       an EXTRACTION pipeline's status is noise and the oracle is the VALUE.
       `head` is fine there once the substitution is guarded (`|| true`) and the
       value asserted (`test -n "$VER"`).

     The discriminator here is the PRODUCER: if the first command of the
     pipeline is one of BOUNDED_TOOLS, it is a gate and an early-closing
     consumer can SIGPIPE it to 141, which under `pipefail` fails a step that
     SUCCEEDED. If the pipeline reads a file (`sed ... ../meta.json | head -1`),
     it is an extraction and is left alone. This is how requiring `pipefail`
     avoids converting a benign race into a red gate -- the cost ratchet named
     after handing me the flag without it.
  5. no BARE `rc=$?` on its own line. The runner runs `run:` blocks under
     `bash -e`, where a failing command aborts the step immediately -- so a
     capture on the NEXT line never executes, and neither does the `tail` of the
     log or the message distinguishing a 124 hang from a real failure. Measured
     on 2026-10-10, not assumed: under `bash -e`, `false; rc=$?; echo "$rc"`
     prints nothing and the shell exits 1, and `timeout 1 sleep 5; rc=$?; test
     "$rc" -eq 124 && echo HANG` exits 124 WITHOUT printing HANG. -e is also in
     force inside a group on the right of `||`, so `cmd || { cmd2; rc=$?; }` is
     broken the same way. The form that works is `rc=0` then `cmd ... || rc=$?`,
     because a command on the left of `||` is exempt from -e.

     This one is why the check is worth having rather than a style note: the
     ORIGINAL ci.yml used the bare form at every step, which means the four
     pipe-status fixes it was praised for -- redirect, read the status, then
     format -- could not run on failure. The pipe bug was replaced by an -e bug
     with the same symptom: no log when it matters.

Both, deliberately. (1) is declarative and bounds the unknown, but it is a
runner feature and nobody here has ever observed act_runner honouring it -- the
run logs are 404 to us. (2) is plain POSIX and works whatever the runner
implements. Relying on either alone means relying on something unverified.

WHAT IT IS, AND WHAT IT IS NOT: this is a TEXTUAL check over files we author by
hand, not a semantic analysis of shell. It finds a tool name in command
position on a logical line. It does NOT follow `sh -c`, a variable holding a
command name, a script the workflow writes and then executes, or anything
reached through `eval`. A determined author routes around it in one line. That
is acceptable for what it is for -- catching the omission we actually made,
twice, by hand -- and it is written here rather than only in a commit message
so the next reader does not mistake its silence for proof that nothing can
hang.

NO EXEMPTION FOR "INSTANT" COMMANDS, and that is a choice with a cost: it means
`cargo --version` must be written `timeout 10 cargo --version`, which looks
fussy. The alternative is a carve-out list, and then every line is a judgment
about whether this particular invocation can block. `cargo --version` on a
broken toolchain, an NFS-stalled binary, or a jail mid-`pkg` is not reliably
instant -- and "it returns in milliseconds in practice" is exactly the
reasoning that produced the 17-minute run. A uniform rule needs no judgment at
the call site.

WHERE THIS SHOULD LIVE EVENTUALLY: as a `rumor` subcommand beside `docs
--check`, in Rust, needing no interpreter on the runner. It is python today for
one reason -- the runner cannot compile anything (probe-rustc fails on a single
file with no manifest), so a Rust gate could not run there, while this can run
locally the moment anyone edits a workflow. Revisit when that jail has a
toolchain.
"""

import re
import sys
from pathlib import Path

# Tools that reach the network, the package system, or a compiler: the ones that
# can block for minutes rather than fail. Not a list of "dangerous" commands --
# a list of commands whose DURATION is not under this repo's control.
BOUNDED_TOOLS = {
    "curl", "wget", "fetch", "git", "cargo", "rustc", "rustup",
    "pkg", "pkgin", "ssh", "scp", "rsync", "npm", "pnpm", "yarn",
    "pip", "pip3", "make", "docker", "cargo-deny",
}

# Where one shell command ends and the next begins. A tool name is only an
# invocation if it is the FIRST word after one of these -- which is what keeps
# `for t in git cargo rustc; do` from being read as three invocations, since
# there those names are arguments to `in`, not commands.
# NOTE `in` IS DELIBERATELY ABSENT. It reads like a separator and is not one:
# what follows `for t in ...` is a WORD LIST, not a command, so including it
# made this checker report ci.yml's own `for t in git cargo rustc node curl` as
# three unbounded invocations. The self-test arm below caught that on the first
# run, which is the only reason it is not in the shipped checker.
SEPARATORS = {";", "&&", "||", "|", "(", ")", "{", "}", "!",
              "then", "else", "elif", "do", "done", "fi"}

# WRAPPERS DO NOT CONSUME COMMAND POSITION, and getting this wrong made the
# whole bounds check vacuous. The first version treated any unrecognised word
# as the command, so in `timeout 900 cargo build` the word `timeout` took
# command position and `cargo` was NEVER SEEN -- the call passed not because a
# bound was found but because the tool was never detected, and `bounded()`
# below was nearly dead code. The same hole passed `sudo pkg install -y foo`,
# which is genuinely unbounded, in total silence. Found when a new arm for
# ratchet's gate-pipeline rule failed to fire and I asked why rather than
# adjusting the arm.
WRAPPERS = {"timeout", "sudo", "doas", "env", "nice", "nohup", "stdbuf",
            "time", "ionice", "setsid", "command"}

# `command` IS a wrapper -- `command cargo build` runs cargo, and before this
# was added the checker never saw the cargo at all, so the call passed in
# silence exactly as `timeout 900 cargo build` once did. But `command -v NAME`
# and `command -V NAME` LOOK UP a name without running it, so the tool that
# follows them is an OPERAND, not an invocation. quill predicted that the naive
# fix (adding `command` to WRAPPERS alone) would false-positive on the lookup
# form, and it is not hypothetical: ci.yml:236 is
# `command -v cargo-deny >/dev/null 2>&1`, and `cargo-deny` is in
# BOUNDED_TOOLS, so the naive fix flags the repo's own capability probe as an
# unbounded build. POSIX allows the options to be clustered (`-pv`), so the
# test is "an option cluster of only -p/-v/-V that contains v or V".
COMMAND_LOOKUP_OPT = re.compile(r"^-[pvV]+$")

TIMEOUT_RE = re.compile(r"^timeout$")
# curl's own hard ceiling on the whole operation. ratchet r0.2/7 used this form
# and it is a real bound, not a connect-only one: --connect-timeout would NOT
# count, because a stalled transfer after a successful connect is precisely the
# hang we are bounding.
CURL_MAXTIME_RE = re.compile(r"(?:^|\s)(?:-m|--max-time)(?:[=\s]+\d+)")


def logical_lines(script: str):
    """Join backslash continuations, so a wrapped `git clone \\` reads as one
    command. Yields (first_physical_lineno, text)."""
    out, buf, start = [], "", None
    for n, raw in enumerate(script.splitlines(), 1):
        if start is None:
            start = n
        stripped = raw.rstrip()
        if stripped.endswith("\\"):
            buf += stripped[:-1] + " "
            continue
        out.append((start, buf + stripped))
        buf, start = "", None
    if buf:
        out.append((start or 1, buf))
    return out


def invocations(line: str):
    """Tool names appearing in COMMAND POSITION, with the token index of each."""
    # Treat $( and ` as separators too: a tool inside a substitution is still
    # being run, and `echo "HEAD $(git rev-parse HEAD)"` really can hang.
    norm = line.replace("$(", " ( ").replace("`", " ( ")
    for ch in (";", "&&", "||", "|", ")", "{", "}"):
        norm = norm.replace(ch, f" {ch} ")
    toks = norm.split()
    found, cmd_pos = [], True
    saw_command = lookup_only = False
    for i, t in enumerate(toks):
        bare = t.strip("'\"")
        if bare in SEPARATORS or t in SEPARATORS:
            cmd_pos = True
            saw_command = lookup_only = False
            continue
        if cmd_pos:
            name = bare.split("/")[-1]
            # Stay in command position across anything that merely PREFIXES the
            # real command: a VAR=value assignment, a wrapper, a wrapper's
            # options, or a bare duration (timeout's). The command is the first
            # token that is none of these.
            if (re.match(r"^[A-Za-z_][A-Za-z0-9_]*=", bare)
                    or name in WRAPPERS
                    or bare.startswith("-")
                    or re.match(r"^\d+(?:\.\d+)?[smhd]?$", bare)):
                if name == "command":
                    saw_command = True
                elif (saw_command and COMMAND_LOOKUP_OPT.match(bare)
                        and ("v" in bare or "V" in bare)):
                    lookup_only = True
                continue
            if name in BOUNDED_TOOLS and not lookup_only:
                found.append((i, name, toks))
            cmd_pos = False
            saw_command = lookup_only = False
    return found


def bounded(idx: int, name: str, toks: list, line: str) -> bool:
    """Is this invocation bounded? Either a `timeout` wrapper immediately
    governing it, or the tool's own hard ceiling."""
    # walk left past timeout's own arguments (-k 5, --signal=TERM, the duration)
    # Walk left past timeout's options and duration, AND past any further
    # WRAPPER, because a wrapper by definition does not consume command
    # position: `timeout 10 command cargo build` and `timeout 10 sudo pkg
    # install` are both genuinely bounded, and without this the `command` fix
    # above would turn the first into a false positive.
    # NOT FIXED, and named rather than silently left: an interpreter is not a
    # wrapper, so `timeout 60 sh -c 'cargo fetch'` still reports the inner
    # cargo as unbounded. It is absent from this repo's workflows (grepped), and
    # treating arbitrary interpreters as transparent needs its own evidence.
    j = idx - 1
    while j >= 0:
        t = toks[j].strip("'\"")
        if TIMEOUT_RE.match(t.split("/")[-1]):
            return True
        if (re.match(r"^-|^\d+[smhd]?$|^--", t)
                or t.split("/")[-1] in WRAPPERS):
            j -= 1
            continue
        break
    if name == "curl" and CURL_MAXTIME_RE.search(line):
        return True
    return False


# The four defect CLASSES this checker finds. They are not one class, and the
# summary used to call all of them "unbounded invocation(s) or job(s)" --
# quill measured a 24-finding run in which 6 were pipeline-class, so the
# headline misattributed a quarter of them to a cause they did not have. A
# reader triaging by that line would have gone looking for missing timeouts in
# six places that had none missing.
KIND_BOUND_JOB = "bound-job"
KIND_BOUND_CALL = "bound-call"
KIND_PIPELINE = "pipeline"
KIND_EXIT_STATUS = "exit-status"

KIND_HEADLINES = {
    KIND_BOUND_CALL: "invocation with no bound -- on a single-concurrency "
                     "runner a hang blocks every queued job",
    KIND_BOUND_JOB:  "job with no timeout-minutes -- nothing bounds the job "
                     "itself, whatever hangs inside it",
    KIND_PIPELINE:   "pipeline that reports the wrong status -- a failing "
                     "command can leave the step GREEN",
    KIND_EXIT_STATUS: "dead exit-status capture under bash -e",
}


def check_file(path: Path):
    """(findings, jobs_seen). Each finding is (lineno, kind, msg).
    Textual: see the module docstring for the bound."""
    text = path.read_text("utf-8")
    findings = []

    # --- job-level timeout-minutes -------------------------------------------
    # Jobs are keys at one indent level under `jobs:`; their bodies are deeper.
    lines = text.splitlines()
    in_jobs = False
    jobs = []           # (name, lineno, body_lines)
    cur = None
    for n, raw in enumerate(lines, 1):
        if re.match(r"^jobs:\s*$", raw):
            in_jobs = True
            continue
        if in_jobs:
            if raw and not raw.startswith((" ", "\t")):   # left `jobs:`
                in_jobs = False
                continue
            m = re.match(r"^  ([A-Za-z0-9_-]+):\s*$", raw)
            if m:
                cur = [m.group(1), n, []]
                jobs.append(cur)
                continue
            if cur is not None:
                cur[2].append(raw)
    for name, lineno, body in jobs:
        if not any(re.match(r"^\s+timeout-minutes:\s*\d+", b) for b in body):
            findings.append((lineno, KIND_BOUND_JOB,
                             f"job `{name}` has no timeout-minutes: "
                             f"nothing bounds the job itself"))

    # --- per-invocation bounds ----------------------------------------------
    # Pull each `run:` block scalar with its starting line number.
    for m in re.finditer(r"^(\s*)-?\s*run:\s*\|\s*\n((?:\1\s+.*\n?|\s*\n)*)",
                         text, re.M):
        indent, body = m.group(1), m.group(2)
        base = text[:m.start()].count("\n") + 1
        offset = text[m.start():m.start(2)].count("\n")
        for rel, line in logical_lines(body):
            code = line.strip()
            if not code or code.startswith("#"):
                continue
            for idx, name, toks in invocations(code):
                if not bounded(idx, name, toks, code):
                    findings.append(
                        (base + offset + rel - 1, KIND_BOUND_CALL,
                         f"`{name}` runs unbounded: no timeout wrapper "
                         f"(or -m for curl) -- {code[:70]}"))
    # --- a pipe without pipefail --------------------------------------------
    # Quoted spans are stripped first: `grep -E 'a|b'` contains a `|` that is
    # not a pipe, and flagging it would train a reader to ignore this check.
    for m in re.finditer(r"^(\s*)-?\s*run:\s*\|\s*\n((?:\1\s+.*\n?|\s*\n)*)",
                         text, re.M):
        body = m.group(2)
        base = text[:m.start()].count("\n") + 1
        offset = text[m.start():m.start(2)].count("\n")
        has_pipefail = re.search(r"^\s*set\s+-o\s+pipefail\s*$", body, re.M) \
            or re.search(r"^\s*set\s+-[a-zA-Z]*o\s+pipefail", body, re.M)
        for rel, raw in enumerate(body.splitlines(), 1):
            code = raw.strip()
            if not code or code.startswith("#"):
                continue
            # strip quoted spans, then collapse `||` so only real pipes remain
            bare = re.sub(r"'[^']*'", "", code)
            bare = re.sub(r'"[^"]*"', "", bare)
            bare = bare.replace("||", "")
            if "|" in bare and not has_pipefail:
                findings.append(
                    (base + offset + rel - 1, KIND_PIPELINE,
                     "pipe in a run block with no `set -o pipefail`: the "
                     "pipeline reports its LAST stage's status, so a failing "
                     "command piped to a formatter exits 0 and the step goes "
                     f"GREEN -- {code[:60]}"))

    # --- an early-closing consumer in a GATE pipeline -----------------------
    EARLY_CLOSE = re.compile(r"(?:^|\|\s*)(head\b|grep\s+(?:-\w*[qm]\w*\s))")
    for m in re.finditer(r"^(\s*)-?\s*run:\s*\|\s*\n((?:\1\s+.*\n?|\s*\n)*)",
                         text, re.M):
        body = m.group(2)
        base = text[:m.start()].count("\n") + 1
        offset = text[m.start():m.start(2)].count("\n")
        for rel, line in logical_lines(body):
            code = line.strip()
            if not code or code.startswith("#"):
                continue
            stripped = re.sub(r"'[^']*'", "", code)
            stripped = re.sub(r'"[^"]*"', "", stripped)
            if "|" not in stripped.replace("||", ""):
                continue
            # the PRODUCER: first command of the pipeline, after any `$(`,
            # assignment prefix or `timeout` wrapper
            inv = invocations(code)
            if not inv:
                continue            # reads a file -> extraction, leave it
            if EARLY_CLOSE.search(stripped):
                findings.append(
                    (base + offset + rel - 1, KIND_PIPELINE,
                     "early-closing consumer (`head`/`grep -q`) in a pipeline "
                     "whose producer is an external tool: that is a GATE "
                     "pipeline, and closing the pipe early SIGPIPEs the "
                     "producer to 141, which under pipefail fails a step that "
                     f"succeeded -- save to a file, then extract -- {code[:56]}"))

    # --- bare `rc=$?` under bash -e -----------------------------------------
    # Only a capture on its OWN line is a defect: `cmd || rc=$?` is the fix and
    # must not be flagged. Scoped to run-blocks so a bare capture in a comment
    # or elsewhere in the YAML is not reported.
    for m in re.finditer(r"^(\s*)-?\s*run:\s*\|\s*\n((?:\1\s+.*\n?|\s*\n)*)",
                         text, re.M):
        body = m.group(2)
        base = text[:m.start()].count("\n") + 1
        offset = text[m.start():m.start(2)].count("\n")
        for rel, raw in enumerate(body.splitlines(), 1):
            if re.match(r"^\s*rc=\$\?\s*$", raw):
                findings.append(
                    (base + offset + rel - 1, KIND_EXIT_STATUS,
                     "bare `rc=$?` on its own line: under the runner's bash -e "
                     "the failing command aborts the step first, so this capture "
                     "and every line after it is dead code -- write `rc=0` then "
                     "`cmd ... || rc=$?`"))
    return findings, [j[0] for j in jobs]


# --------------------------------------------------------------------------- #
# SELF-TEST. A lint nobody has watched fail is indistinguishable from a lint
# that passes everything -- which is the trap this whole epic keeps walking
# into, and the one I walked into to produce the defect above. So the fixtures
# are not invented: BAD_PKG is probe-pkg.yml AS IT ACTUALLY SHIPPED, the file
# that wedged the runner. If this checker does not refuse that byte-for-byte,
# it would not have prevented the thing it exists to prevent.
# --------------------------------------------------------------------------- #

BAD_PKG = """\
name: probe-pkg
on:
  push:
    branches:
      - r0.2-ci
jobs:
  freebsd:
    runs-on: freebsd-15-jail
    steps:
      - name: can pkg query the repo and plan an install (no mutation)
        run: |
          pkg --version
          pkg rquery '%n %v' cargo-deny
          pkg install -n -y cargo-deny
"""

GOOD_MIRROR = """\
name: probe-mirror-egress
on:
  workflow_dispatch:
jobs:
  freebsd:
    runs-on: freebsd-15-jail
    timeout-minutes: 2
    steps:
      - name: can the runner reach the FreeBSD package mirror
        run: |
          curl -sf -m 10 -o /dev/null https://pkg.FreeBSD.org/x/meta.conf
"""

# The false positive that a naive word-scan produces, and the reason
# `invocations` tracks command position at all. ci.yml's probe step really does
# contain the line `for t in git cargo rustc node curl; do` -- six tool names,
# zero invocations.
WORDLIST_NOT_INVOCATION = """\
jobs:
  freebsd:
    timeout-minutes: 5
    steps:
      - name: names in a word list are not commands
        run: |
          for t in git cargo rustc node curl; do
            echo "$t"
          done
"""

# A connect-only bound must NOT satisfy the check: a transfer that stalls after
# a successful connect is exactly the hang being bounded.
CONNECT_TIMEOUT_IS_NOT_A_BOUND = """\
jobs:
  freebsd:
    timeout-minutes: 5
    steps:
      - name: connect-timeout is not a ceiling on the transfer
        run: |
          curl -sf --connect-timeout 5 -o /dev/null https://example.invalid/x
"""

SUBSTITUTION_STILL_RUNS = """\
jobs:
  freebsd:
    timeout-minutes: 5
    steps:
      - name: a tool inside a substitution is still executed
        run: |
          echo "HEAD $(git rev-parse HEAD)"
"""

CONTINUATION_IS_ONE_COMMAND = """\
jobs:
  freebsd:
    timeout-minutes: 5
    steps:
      - name: a wrapped command is bounded by the timeout on its first line
        run: |
          timeout 120 git clone --quiet --depth 1 \\
            https://forge.example.org/x/y.git src-tree
"""

# The shape the ORIGINAL ci.yml used at every step. Under bash -e the tail and
# the 124 discrimination below the capture cannot run.
BARE_CAPTURE_UNDER_E = """\
jobs:
  freebsd:
    timeout-minutes: 5
    steps:
      - name: redirect, read the status, then format -- except it never gets there
        run: |
          timeout 900 cargo build --locked > ../build.log 2>&1
          rc=$?
          tail -20 ../build.log
          test "$rc" -eq 124 && { echo "HANG"; exit 124; }
"""

# -e is in force inside a group on the right of `||` too, measured 2026-10-10.
BARE_CAPTURE_INSIDE_GROUP = """\
jobs:
  freebsd:
    timeout-minutes: 5
    steps:
      - name: a group on the right of || is not exempt
        run: |
          command -v cargo-deny >/dev/null 2>&1 \\
            || { timeout 1200 cargo install cargo-deny --locked > ../i.log 2>&1
                 rc=$?
                 tail -20 ../i.log }
"""

GUARDED_CAPTURE = """\
jobs:
  freebsd:
    timeout-minutes: 5
    steps:
      - name: the form that survives -e
        run: |
          rc=0
          timeout 900 cargo build --locked > ../build.log 2>&1 || rc=$?
          tail -20 ../build.log
          test "$rc" -eq 124 && { echo "HANG: build exceeded 900s"; exit 124; }
"""

# The ORIGINAL defect, as opposed to the broken correction for it. A gate
# exiting 4 piped to a formatter leaves the step green.
PIPE_WITHOUT_PIPEFAIL = """\
jobs:
  freebsd:
    timeout-minutes: 5
    steps:
      - name: a failing gate piped to a formatter goes green
        run: |
          timeout 300 cargo deny check licenses | tail -20
          timeout 900 cargo test --locked | tee ../test.log | tail -20
"""

# ratchet r0.2/14's GATE pipeline: the producer is an external tool and `head`
# can SIGPIPE it. This exact line survived every earlier pass and was still in
# the first draft of the commit that fixed the other three defects.
# The hole that made `bounded()` nearly dead code: an unrecognised prefix word
# used to take command position, so the tool behind it was never checked.
# `sudo pkg install` is unbounded and used to pass in total silence.
WRAPPED_BUT_UNBOUNDED = """\
jobs:
  freebsd:
    timeout-minutes: 5
    steps:
      - name: a prefix word is not a bound
        run: |
          sudo pkg install -y cargo-deny
          nice cargo build --locked
          env RUSTFLAGS=-g cargo test
"""

# and the same shape WITH a real bound must still pass -- for the right reason
# this time, namely that `bounded()` walked left and found `timeout`.
WRAPPED_AND_BOUNDED = """\
jobs:
  freebsd:
    timeout-minutes: 5
    steps:
      - name: timeout governs the command behind it
        run: |
          timeout 900 cargo build --locked
          timeout -k 5 120 git clone --depth 1 https://x/y.git t
"""

GATE_PIPELINE_WITH_HEAD = """\
jobs:
  freebsd:
    timeout-minutes: 5
    steps:
      - name: a gate piped into an early-closing consumer
        run: |
          set -o pipefail
          VER=$(timeout 120 cargo metadata --no-deps | sed -n 's/x/y/p' | head -1)
"""

# An EXTRACTION pipeline: it reads a FILE, so its status is noise and `head` is
# correct. Flagging this would make the gate/extraction rule unusable.
EXTRACTION_FROM_FILE = """\
jobs:
  freebsd:
    timeout-minutes: 5
    steps:
      - name: extraction from a saved file, status disclaimed, value asserted
        run: |
          set -o pipefail
          VER=$(sed -n 's/x/y/p' ../meta.json | head -1 || true)
          test -n "$VER" || { echo "could not derive the version"; exit 1; }
"""

PIPE_WITH_PIPEFAIL = """\
jobs:
  freebsd:
    timeout-minutes: 5
    steps:
      - name: one line at the top makes the pipeline honest
        run: |
          set -o pipefail
          timeout 300 cargo deny check licenses | tail -20
"""

# A `|` inside quotes is not a pipe. Flagging it would teach a reader to skim
# past this check, which is worse than not having it.
QUOTED_PIPE_IS_NOT_A_PIPE = """\
jobs:
  freebsd:
    timeout-minutes: 5
    steps:
      - name: an alternation in a regex is not a pipeline
        run: |
          timeout 60 grep -E 'ok|FAILED' ../test.log
"""

JOB_WITHOUT_TIMEOUT = """\
jobs:
  freebsd:
    runs-on: freebsd-15-jail
    steps:
      - name: bounded command, unbounded job
        run: |
          timeout 10 cargo --version
"""


# quill's two findings on this checker, as fixtures. `command` is a WRAPPER, so
# before the fix the cargo in `command cargo build` was never reached and the
# call passed in silence -- the same shape as the `timeout 900 cargo build` hole
# that made bounded() nearly dead code.
COMMAND_WRAPS_A_BUILD = """\
jobs:
  freebsd:
    timeout-minutes: 5
    steps:
      - name: command does not bound anything
        run: |
          command cargo build --locked
"""

# And the false positive quill PREDICTED for the naive fix. This is not an
# invented case: ci.yml:236 ships `command -v cargo-deny >/dev/null 2>&1`, and
# `cargo-deny` is in BOUNDED_TOOLS, so adding `command` to WRAPPERS without the
# lookup exception flags the repo's own capability probe as an unbounded build.
COMMAND_LOOKUP_IS_NOT_A_RUN = """\
jobs:
  freebsd:
    timeout-minutes: 5
    steps:
      - name: a lookup runs nothing
        run: |
          command -v cargo-deny >/dev/null 2>&1 && echo present
          command -V rustc >/dev/null 2>&1 || echo absent
          p=$(command -v cargo) && echo "cargo at $p"
"""

# A wrapper to the LEFT of the tool must not hide the timeout: both of these
# are genuinely bounded, and the `command` fix would have turned the first into
# a false positive without the matching change in bounded().
WRAPPER_CHAIN_IS_STILL_BOUNDED = """\
jobs:
  freebsd:
    timeout-minutes: 5
    steps:
      - name: bounded through a wrapper chain
        run: |
          timeout 600 command cargo build --locked
          timeout 300 sudo pkg install -y curl
          timeout 60 env -i nice -n 5 git fetch --all
"""

# A file carrying BOTH a bound-class and a pipeline-class finding, for the
# headline. quill measured that the summary attributed all 24 findings to the
# bound class when 6 were pipeline-class, so the arm asserts the CLASSIFIER
# rather than the wording of the print.
MIXED_CLASSES = """\
jobs:
  freebsd:
    steps:
      - name: one unbounded call and one unguarded pipe
        run: |
          pkg install -y curl
          timeout 300 cargo deny check licenses | tail -20
          rc=$?
"""


def self_test() -> int:
    import tempfile
    cases = [
        # (name, yaml, must_flag_substrings, must_not_flag)
        ("probe-pkg as it shipped wedges the runner", BAD_PKG,
         ["`pkg` runs unbounded", "no timeout-minutes"], []),
        ("ratchet's bounded mirror probe passes", GOOD_MIRROR, [], ["unbounded"]),
        ("a word list is not six invocations", WORDLIST_NOT_INVOCATION,
         [], ["unbounded"]),
        ("--connect-timeout is not a transfer ceiling",
         CONNECT_TIMEOUT_IS_NOT_A_BOUND, ["`curl` runs unbounded"], []),
        ("a tool inside $( ) is still an invocation",
         SUBSTITUTION_STILL_RUNS, ["`git` runs unbounded"], []),
        ("a backslash continuation is one bounded command",
         CONTINUATION_IS_ONE_COMMAND, [], ["unbounded"]),
        ("a bounded command does not excuse an unbounded job",
         JOB_WITHOUT_TIMEOUT, ["no timeout-minutes"], ["unbounded"]),
        ("the original ci.yml's bare capture is dead code under -e",
         BARE_CAPTURE_UNDER_E, ["bare `rc=$?`"], ["unbounded"]),
        ("a group on the right of || is not exempt from -e either",
         BARE_CAPTURE_INSIDE_GROUP, ["bare `rc=$?`"], []),
        # The third must-not here was ADDED after mutation testing: removing the
        # `||`-collapsing in the pipe scan left every self-test arm green,
        # because no arm asserted that `cmd || rc=$?` is not read as a pipe. A
        # surviving mutant is an untested line, and the untested line was the
        # one that keeps the recommended fix from being flagged by the very
        # checker that recommends it.
        ("`cmd || rc=$?` is the fix and must not be flagged",
         GUARDED_CAPTURE, [], ["bare `rc=$?`", "unbounded", "pipefail"]),
        ("the ORIGINAL pipe defect: a failing gate piped to a formatter",
         PIPE_WITHOUT_PIPEFAIL, ["no `set -o pipefail`"], []),
        ("one `set -o pipefail` at the top clears the block",
         PIPE_WITH_PIPEFAIL, [], ["pipefail"]),
        ("a `|` inside quotes is an alternation, not a pipeline",
         QUOTED_PIPE_IS_NOT_A_PIPE, [], ["pipefail"]),
        ("a prefix word (sudo/nice/env) is not a bound",
         WRAPPED_BUT_UNBOUNDED,
         ["`pkg` runs unbounded", "`cargo` runs unbounded"], []),
        ("`timeout N cmd` passes because a bound was FOUND, not unseen",
         WRAPPED_AND_BOUNDED, [], ["unbounded"]),
        ("`head` on a gate pipeline can SIGPIPE the producer",
         GATE_PIPELINE_WITH_HEAD, ["early-closing consumer"], []),
        ("`head` on an extraction from a file is correct and must pass",
         EXTRACTION_FROM_FILE, [], ["early-closing consumer", "pipefail"]),
        # quill's finding 1: `command` wraps a real build and did not bound it
        ("`command cargo build` is an unbounded build, not a lookup",
         COMMAND_WRAPS_A_BUILD, ["`cargo` runs unbounded"], []),
        # quill's PREDICTED false positive for the naive fix, from ci.yml:236
        ("`command -v NAME` looks up and must not read as an invocation",
         COMMAND_LOOKUP_IS_NOT_A_RUN, [], ["unbounded"]),
        ("a wrapper chain does not hide the timeout to its left",
         WRAPPER_CHAIN_IS_STILL_BOUNDED, [], ["unbounded"]),
        # Asserts the CLASSIFICATION, not just that each finding appears: a
        # checker that reported all four as bound-class would satisfy the
        # message-only version of this arm, which is exactly the defect quill
        # measured in the headline.
        ("a mixed file classifies all four kinds, not all as bound-class",
         MIXED_CLASSES,
         ["[bound-call] `pkg` runs unbounded",
          "[bound-job] job `freebsd` has no timeout-minutes",
          "[pipeline] pipe in a run block",
          "[exit-status] bare `rc=$?`"], []),
    ]
    failures = 0
    with tempfile.TemporaryDirectory() as d:
        for name, yml, must, mustnot in cases:
            p = Path(d) / "case.yml"
            p.write_text(yml)
            findings, _ = check_file(p)
            # the KIND is in the blob deliberately: it lets an arm assert how
            # a finding was CLASSIFIED and not merely that it was reported.
            blob = " | ".join(f"[{kind}] {msg}" for _, kind, msg in findings)
            ok = True
            for needle in must:
                if needle not in blob:
                    print(f"  FAIL {name}\n       expected to flag {needle!r}\n"
                          f"       got: {blob or '(nothing)'}")
                    ok = False
            for needle in mustnot:
                if needle in blob:
                    print(f"  FAIL {name}\n       must NOT flag {needle!r}\n"
                          f"       got: {blob}")
                    ok = False
            if ok:
                print(f"  ok   {name}")
            else:
                failures += 1
    print(f"\nself-test: {len(cases) - failures}/{len(cases)} arms green")
    return 1 if failures else 0


def main(argv) -> int:
    if "--self-test" in argv:
        return self_test()
    root = Path(argv[1]) if len(argv) > 1 else Path(".forgejo/workflows")
    files = sorted(root.glob("*.yml")) + sorted(root.glob("*.yaml"))
    if not files:
        print(f"no workflow files under {root}: nothing checked, and that is a "
              f"refusal rather than a pass -- an empty corpus is how a check "
              f"goes quiet without anyone deciding it should")
        return 1
    total = 0
    counts = {}
    for f in files:
        findings, jobs = check_file(f)
        for lineno, kind, msg in findings:
            print(f"{f}:{lineno}: {msg}")
            counts[kind] = counts.get(kind, 0) + 1
        total += len(findings)
    if total:
        print(f"\n{total} finding(s) across {len(files)} workflow file(s), "
              f"BY CLASS -- they do not share a cause:")
        for kind in (KIND_BOUND_CALL, KIND_BOUND_JOB, KIND_PIPELINE,
                     KIND_EXIT_STATUS):
            if counts.get(kind):
                print(f"  {counts[kind]:>4}  {KIND_HEADLINES[kind]}")
        return 1
    print(f"{len(files)} workflow file(s), every job and invocation bounded")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
