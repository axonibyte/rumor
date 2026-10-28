#!/usr/bin/env python3
"""Drive D4's control on a TAG, from a fresh clone, and report rather than judge.

D4 (fable, breakout-r6/0): "seeds 2, 40 and 1 against v0.1.31: byte-identical
through 89 and 90; same rule, value and attribution through 91 and 92, and no
`pending` in any of the three outputs."

TWO CONDITIONS ARE TWO DRILLS, so the expectation is the CALLER's and never
inferred from the tag: --expect identical (89, 90) or --expect same-finding
(91, 92). A script that guessed which one applied would be deciding the gate's
verdict, which is mine and not a default.

THE INVOCATION OF RECORD (ratchet breakout-r6/2, on fable rumor/1133):
    rumor run examples/toy --seed N --steps 10
stdout ONLY, sha256 truncated to 16, with `toy` and `toy-adapter` built and on
PATH beside `rumor`. ratchet's own trap at breakout-r6/1: `go build ./cmd/rumor`
alone reads rc 2, "spawning toy-adapter: executable file not found in $PATH",
and a figure reported without reading stderr would have published that
v0.1.31's control no longer finds the bug. So all three binaries are built and
their presence is ASSERTED before a single run.

THE TWO HALVES (fable rumor/1315): the BYTES are measured against
`examples/toy` FROZEN at 93084b0 and extracted per run with `git archive`; the
FIVE FIELDS of rumor/1039 are measured against the LIVE toy at whatever head is
being driven. A re-based control would agree with the toy by construction; this
one cannot, because its subject does not move.

THE SEAM THAT PROVES THEY ARE INDEPENDENT IS HAND-DRIVEN, NOT ARMED, and saying
so is the point -- it needs a clonable ref and a build, which a fixture cell
cannot assume. **Re-drive it by hand whenever the frozen SUBJECT is advanced**:
it is the only check that the two halves are not the same measurement twice.

BOTH BRANCHES, AND THE REF DECIDES WHICH. Since it asserts that appending
`[clock]` actually MOVED the witness seed's bytes, a ref whose tree writes no
clock fact now refuses instead of announcing a pass it did not measure:

    # TAKES -- a tree that writes clock facts
    python3 d4control.py v0.1.39 --expect same-finding --clock-the-live-toy
    rc 0: "it TOOK -- seed 12's live bytes moved 60705760d6ef48dd ->
    8d76ddcf7670f2b6 in this run", frozen bytes unchanged, all five seeds met
    --expect. That is ibis rumor/1301's figure to the character, reached here by
    a different route and MEASURED in the run rather than asserted as a constant.

    # REFUSES -- `[clock]` parses, nothing writes a clock fact
    python3 d4control.py v0.1.38 --expect same-finding --clock-the-live-toy
    rc 2: "DID NOT TAKE on v0.1.38: seed 12's live bytes are 60705760d6ef48dd
    both with `[clock]` appended and without it."

A TAG NEEDS NO `--source`, which is why one spelling is above where two used to
be. A clone copies refs/heads and refs/tags, so `d4control.py v0.1.39` reaches
the tree from any source holding the tag -- and the run says which source served
it ("frozen subject: 93084b02:examples/toy extracted by git archive from the
clone of v0.1.39"), because reading this code cannot tell you.

THE BRANCH NAME THAT STOOD HERE IS GONE: the forge dropped r7.1-clock-fact-2
when PR 104 merged, so the invocation that named it now fails for a reason the
text did not mention -- the same defect this block was written to avoid, one
release later. Measured rather than inferred from the merge: `ls-remote --heads
<url> 'r7.1-clock-fact*'` returns NOTHING on a call where r7-d4-control-field-2
returns its sha (rook coop rumor/1340, ratchet rumor/1341, and here).

A BARE SHA IS THE ONE SPELLING THAT STILL DEPENDS ON THE SOURCE. The commit
whose tree moves the bytes is 93084b0; `d4control.py 93084b0` drives the whole
control from a source that holds it on a LOCAL ref (ratchet, coop rumor/1333)
and fails from one where it is only a remote-tracking tip -- refs/remotes is not
copied, and this source refuses a bare-sha fetch (rc 128). The tag is the form
that works from either, so prefer it.

EXIT CODES, DECLARED so a reader of rc is never guessing:
  0  every seed met --expect, and the pending-absence arm had its presence partner
  1  a seed did NOT meet --expect; every seed's reading is printed anyway
  2  NO VERDICT -- the measurement cannot be trusted: a binary missing, a build
     failure, a dirty or wrong tree, empty stdout from a run, or an unreadable
     clone. These are not "the control changed" and must never read as it.
"""
import argparse, hashlib, os, re, shutil, subprocess, sys, tempfile, pathlib

# v0.1.31's figures, captured by ratchet at 7dfa641 (breakout-r6/1) and
# reproduced by me from a rebuild at rumor/1134. FOUR facts per seed, not one:
# a hash alone cannot say whether a mismatch moved the finding or only the text.
_FOUND = "run: FOUND bug after step {n} -- data.rumor.statute.finding, rests_on_planted false"
_STALE = "run: FOUND bug of kind stale after step 24 -- engine.stale, rests_on_planted false"

# EACH SEED CARRIES ITS OWN BASE AND ITS OWN INVOCATION, and both are printed.
#
# WHY THE BASE IS PER SEED: 2, 40 and 1 are v0.1.31's figures. Seeds 12 and 66
# have NO v0.1.31 reading -- 66's cleared pending did not exist before eb3b045
# and 12's stale witness before 94c413b -- so their base is R6's close,
# d7ce321, measured by me at the tag on 2026-10-28. A single hardcoded
# "(v0.1.31 ...)" label would have published their base as a tag that never
# produced them, which is this session's recurring defect: a figure quoted
# without the base it was measured on.
#
# WHY THE INVOCATION IS PER SEED: 2, 40 and 1 are the 10-step invocation of
# record; 12 and 66 are `--steps 24 --profile settling`, which is what fable
# named at rumor/1258 ("now with seeds 12 and 66 under `settling`"). One
# hardcoded `--steps 10` applied to seed 66 runs a DIFFERENT drill and reports
# it under this one's name -- it would simply print no pending line and read
# as a pass.
#
# WHY `pending` IS A LIST OF SENTENCES AND NOT A COUNT: a count is not a
# property of its inputs. Two pending lines with the wrong actors, the wrong
# steps or the wrong verdicts count 2 and pass a count arm. The sentences are
# compared as a SET for legibility and the sha16 pins their ORDER, which is two
# oracles rather than one.
CONTROL = {
    2:  {"base": "v0.1.31", "steps": 10, "profile": None,
         "sha16": "786416e6925acf7c", "rc": 3, "lines": 7,
         "fields": {"kind": None, "step": 4,
                    "rule": "data.rumor.statute.finding", "planted": "false"},
         "found": _FOUND.format(n=4), "pending": []},
    40: {"base": "v0.1.31", "steps": 10, "profile": None,
         "sha16": "dff917a0f6c74983", "rc": 3, "lines": 6,
         "fields": {"kind": None, "step": 3,
                    "rule": "data.rumor.statute.finding", "planted": "false"},
         "found": _FOUND.format(n=3), "pending": []},
    1:  {"base": "v0.1.31", "steps": 10, "profile": None,
         "sha16": "ba42d0333035a0aa", "rc": 0, "lines": 12,
         "fields": None,          # a clean seed finds nothing: the absence IS the field
         "found": None, "pending": []},
    12: {"base": "d7ce321", "steps": 24, "profile": "settling",
         "sha16": "60705760d6ef48dd", "rc": 3, "lines": 55,
         "fields": {"kind": "stale", "step": 24, "rule": "engine.stale",
                    "planted": "false"},
         "found": _STALE, "pending": [
             "pending -- member_2/event_tally held at step 19, re-checked at step 24: consistent",
             "pending -- guest_1/event_tally held at step 22, re-checked after the last step: bug of kind stale",
             '"event.tallied" is eventual bounded "quiesce"; the disagreement on '
             "the claim at position 24 was held open as `pending` at seal 22, the "
             "system has since read quiet, and it still disagrees: the bound is past",
         ]},
    66: {"base": "d7ce321", "steps": 24, "profile": "settling",
         "sha16": "8344458094e50a37", "rc": 0, "lines": 29,
         "fields": None,          # clears its pendings, finds nothing
         "found": None, "pending": [
             "pending -- member_1/event_tally held at step 11, re-checked after the last step: consistent",
             "pending -- member_2/event_tally held at step 20, re-checked after the last step: consistent",
         ]},
}
# THE CONTROL'S SUBJECT IS FROZEN, AND THAT IS THE WHOLE POINT (fable
# rumor/1315, ruling on rook rumor/1305 with ratchet /1307 and quill /1303).
#
# `examples/toy` AS IT STOOD AT 93084b0 is the project these five seeds run,
# extracted with `git archive` per invocation, NOT the clone's own working copy.
# WHY: ibis measured (rumor/1301) that appending `[clock]` to the live toy moves
# seed 12's bytes -- 60705760d6ef48dd -> 8d76ddcf7670f2b6 -- because clock facts
# take LEDGER POSITIONS and seed 12's stale sentence prints two of them
# ("position 24 ... seal 22" becomes "position 71 ... seal 66").
#
# THE SHIFT IS A PREFIX COUNT AND NOT A RATE, which matters because a rate
# invites arithmetic that cannot work: a fact's clocked position is its
# unclocked one PLUS the clock facts standing before it, a count that grows
# with the seals already taken. So the two deltas are UNEQUAL by construction
# -- +47 on the claim's position and +44 on the seal's -- and no multiplier
# fits both. ibis read it off the source at 93084b0 (rumor/1323): `Pending.At`
# comes from `l.Seal()` (pending.go:500) and `SealAt(p)` "seals the prefix
# ending at position p" (ledger.go:151), so a SEAL IS A POSITION in the same
# series as the claim's -- they are one series, not two kinds of number.
#
# AND THE LEDGER COUNT ITSELF IS STILL UNREAD by any of us: `Record.Facts`
# would give it directly, stdout gives none, and "three facts between 22 and
# 24" is 47 minus 44 rather than a count anybody took (quill rumor/1324). Issue 101's
# done-when declares `[clock]` in the live toy, and 103's changes the toy again
# while asserting "the R5 and R6 rows unchanged" -- a sentence a control
# re-derived from the toy cannot read, because it would re-take its figure from
# the subject that just moved and report agreement.
#
# SO RULE 3 HAS TWO HALVES HERE AND BOTH ARE KEPT:
#   BYTES  against the FROZEN subject -- the renderer and the walk did not move
#   FIELDS against the LIVE toy at every head -- what HAPPENED did not move
# ibis's measurement is exactly the gap between them: the fields equal, the
# printed positions not, which is why one of five moved and four did not.
SUBJECT = "93084b02"          # examples/toy frozen here; named in every report
SUBJECT_PATH = "examples/toy"

# The order the drills are driven and reported in: the three 10-step seeds
# first, as every earlier record of this control reads, then the two settling
# seeds R7 adds.
ORDER = (2, 40, 1, 12, 66)
BINS = ("rumor", "toy", "toy-adapter")

# D6's SECOND HALF AND D12, AND THE FIGURE IS DERIVED RATHER THAN QUOTED.
# Both drills were written on 2026-10-25 saying `rumor check` reads "29 agreed",
# and 91's own done-when ADDED a ledger row, so the literal 29 would fail a
# correct release. fable ruled it at rumor/1190: "29 was the figure of the day
# D6 and D12 were written, not a literal" -- the gate reads the R1-R5 case set
# UNCHANGED against v0.1.31 plus exactly R6.3's own row, every case agreeing,
# which is 30 of 30 at the gate, and 92 adds no case.
#
# SO THE INVARIANT IS THE SET AND THE COUNT IS ITS CONSEQUENCE. A stored number
# rots the next time a phase plants a row; a stored SET says which row arrived
# and names the one that should. v0.1.31's 29 case names are in
# fixtures/check-cases-v0.1.31.txt, captured from the tag itself.
CASES_31 = pathlib.Path(__file__).parent / "fixtures" / "check-cases-v0.1.31.txt"
ADDED_BY = {"91": "toy/dropped-recount"}      # phase -> the one row it plants


def refuse(msg, detail=""):
    print(f"D4 CONTROL: *** NO VERDICT -- {msg}")
    if detail:
        print(detail.rstrip())
    print("    rc 2 is NOT 'the control changed'. Nothing was measured.")
    sys.exit(2)


def run(cmd, cwd=None, env=None):
    return subprocess.run(cmd, cwd=cwd, env=env, capture_output=True, text=True)


def cases_verdict(observed, cases, agreed, disagreed, expect):
    """D6's second half and D12, judged from TEXT so the arms can drive it.

    `observed` is the set of "pkg/case" names `rumor check` printed; the three
    numbers are its own summary line. Returns (ok, [lines to print]).
    """
    want = set(CASES_31.read_text().split())
    out, ok = [], True

    # THE SUMMARY MUST AGREE WITH ITSELF FIRST. A case set is worthless beside
    # a summary that counted something else, and "30 agreed" over 29 printed
    # cases is a tool bug rather than a release's.
    if not (cases == agreed == len(observed)) or disagreed != 0:
        ok = False
        out.append(f"  *** summary disagrees with the rows: {cases} case(s), "
                   f"{agreed} agreed, {disagreed} disagreed, {len(observed)} "
                   f"row(s) printed")

    missing = sorted(want - observed)
    added = sorted(observed - want)
    if missing:                     # ruling 3: the R5 rows are the control
        ok = False
        out.append(f"  *** {len(missing)} case(s) LOST since v0.1.31: {missing}"
                   f" -- ruling 3 is the R1-R5 set unchanged")

    allowed = sorted(ADDED_BY.values()) if expect == "same-finding" else []
    if added != allowed:
        ok = False
        out.append(f"  *** additions are {added}, expected {allowed} "
                   f"(--expect {expect})")

    if ok:
        out.append(f"  ok  rumor check: {cases} case(s), {agreed} agreed, 0 "
                   f"disagreed; v0.1.31's {len(want)} all present, additions "
                   f"{added or 'none'} -- the set is the invariant, {cases} is "
                   f"its consequence")
    return ok, out


def run_check(rumor, tree, env):
    """Drive `rumor check` and reduce it to what cases_verdict judges."""
    p = run([rumor, "check"], cwd=tree, env=env)
    if p.returncode != 0:
        refuse(f"`rumor check` exited {p.returncode}", p.stdout + p.stderr)
    obs, nums = set(), None
    for line in p.stdout.splitlines():
        f = line.split()
        if line.startswith("check:"):
            # check: N case(s) over M package(s) from ledger files, N agreed, 0 disagreed
            m = re.search(r"check: (\d+) case\(s\).*?(\d+) agreed, (\d+) disagreed",
                          line)
            if not m:
                refuse("`rumor check`'s summary line did not parse", line)
            nums = tuple(int(x) for x in m.groups())
        elif len(f) >= 3:
            obs.add(f"{f[0]}/{f[1]}")
    if nums is None:
        refuse("`rumor check` printed no summary line", p.stdout[-400:])
    return obs, nums


def _sha16(out):
    """ONE definition of the control's digest, used by both halves.

    It was inline in judge() and the live-toy half needed it too; two copies of
    a hashing rule is how a truncation or an encoding choice drifts between the
    figure you publish and the figure you compare.
    """
    return hashlib.sha256(out.encode()).hexdigest()[:16]


def fields_of(out):
    """The FIVE of rumor/1039 read off the FOUND line, or None when none fires.

    Returns {kind, step, rule, planted} -- four read fields plus the STEP, which
    is the fifth of the control and the one a looser arm drops. `kind` is None
    for the undifferentiated form (`FOUND bug after step N`), a string for
    `FOUND bug of kind stale after step N`.

    WHY THIS IS NOT A SUBSTRING CHECK ON THE FOUND LINE: the whole point of the
    live-toy half is that the LINE may differ while the FIELDS do not. ibis
    measured bytes moving under a clocked toy with the finding unchanged, so an
    arm comparing the sentence would fail on exactly the case this half exists
    to pass.
    """
    line = next((l for l in out.splitlines() if "FOUND" in l), None)
    if line is None:
        return None
    m = re.search(r"FOUND bug(?: of kind (\S+))? after step (\d+) -- "
                  r"(\S+?), rests_on_planted (\S+)", line)
    if not m:
        return {"unparseable": line.strip()[:90]}
    return {"kind": m.group(1), "step": int(m.group(2)),
            "rule": m.group(3), "planted": m.group(4)}


def judge(seed, out, rc, expect):
    """One seed's verdict, from TEXT ALONE -- so the fixture path and the
    real path are judged by the same code and cannot drift apart."""
    want = CONTROL[seed]
    sha16 = _sha16(out)
    lines = len(out.splitlines())

    # THE QUIESCE LINE IS PERMITTED AND A TRAILING `pending` IS NOT, which
    # is fable's own distinction: "the output may gain the quiesce line and
    # may not gain a trailing `pending`". Counting the WORD cannot tell
    # them apart -- from 91 the summary reads
    #   run:   quiesce -- 0 pending(s) open at the end; reconciliation: ...
    # so a word-count arm reads 1 and calls D4 violated on the strength of
    # the very line that proves nothing is open. My first version did
    # exactly that against a shipped release. Parse the NUMBER, and count
    # the word only OUTSIDE that summary.
    qline = [l for l in out.splitlines() if "quiesce --" in l]
    q_open, q_bad = 0, ""
    if qline:
        m = re.search(r"(\d+) pending\(s\) open", qline[0])
        if not m:
            q_bad = ("the quiesce summary does not state a pending count in "
                     "the form `N pending(s) open`: " + qline[0].strip())
        else:
            q_open = int(m.group(1))
    # A MENTION THAT SAYS THERE IS NONE IS NOT A PENDING, and exempting a whole
    # LINE is the wrong shape -- that is "declare what counts as fine", and a
    # shrink line could legitimately report a pending it left open. So each
    # mention is judged: acceptable iff the phrase asserts ZERO or NO pending,
    # counted otherwise.
    #
    # THIS ARM HAS NOW BEEN FOOLED TWICE BY THE SAME SHAPE. First 91's
    # `quiesce -- 0 pending(s) open at the end`, which I fixed by exempting that
    # summary LINE; then 92's `... each settled with no pending left open`, a
    # different line with a different negation. Exempting lines one at a time
    # loses the race against the product's vocabulary. Judging the MENTION
    # generalises, and it FAILS SAFE: an unrecognised phrasing counts as a
    # pending and is reported, which errs toward my reading the output rather
    # than toward silence.
    counted = [l.strip() for l in out.splitlines()
               if "pending" in l.lower()
               and not re.search(r"\b(?:0|no)\s+pending", l, re.I)]
    pend = len(counted)

    # EVERY COUNTED MENTION IS DECLARED VERBATIM, and nothing is exempted by
    # SHAPE. R7's control adds seeds whose whole point is that they DO print a
    # pending line (fable rumor/1258), so the original "no mention at all" rule
    # would fail them by design. The weak fix is to exempt anything matching
    # `pending -- ...`; this file already says why that is wrong -- "exempting a
    # whole LINE is the wrong shape... each mention is judged".
    #
    # SO THE DECLARATION IS THE WHOLE SET. Seed 12 mentions a pending in THREE
    # lines: its two pending lives and the stale finding's own explanation
    # ("...was held open as `pending` at seal 22..."), which is output I had not
    # read when I first wrote this table and which a shape rule would have
    # waved through forever. Seeds 2, 40 and 1 declare the EMPTY set, so for
    # them this is exactly the original rule: any mention at all fails.
    def _norm(s):
        return re.sub(r"\s+", " ",
                      re.sub(r"^(?:run|shrink):\s*", "", s.strip())).strip()
    observed = {_norm(l) for l in counted}
    expected = {_norm(l) for l in want["pending"]}
    pend_ok = observed == expected
    pend_why = ""
    if observed != expected:
        missing, extra = sorted(expected - observed), sorted(observed - expected)
        pend_why = " *** pending lines DIFFER"
        if missing:
            pend_why += f"; absent: {missing[0][:70]}"
        if extra:
            pend_why += f"; unexpected: {extra[0][:70]}"

    # THE ABSENCE ARM NEEDS ITS PRESENCE PARTNER, and the partner is
    # MEASURED, not guessed. My first version used "agreed" for the clean
    # seed, which `rumor check` prints and `rumor run` never does: the arm
    # read ABSENT on an output whose hash matched v0.1.31 byte-for-byte.
    # Every run of all three seeds ends on its own summary line --
    # `run: toy -- seed N, M step(s), ...` -- so that is the universal
    # witness that the output is a COMPLETE run and not a truncation, which
    # is what makes `pending == 0` mean anything at all.
    complete = "step(s)" in out

    # THE FINDING IS ASSERTED BOTH WAYS, not merely looked for. FOUND must
    # be PRESENT on the two bug seeds and ABSENT on the clean one; checking
    # only presence would pass a clean seed that silently started finding
    # bugs, which is the louder half of "no new noise".
    found = "FOUND" in out
    finding_right = found == (want["rc"] == 3)

    # R7's D7, THE ABSENCE HALF -- REPORTED HERE AND JUDGED ELSEWHERE, and the
    # demotion is the point.
    #
    # I wrote this as a VERDICT first: a `clock` token in the stdout of an
    # unclocked project fails the seed. Then I measured it at 5e89313, the
    # commit that writes the clock, and it cannot work: **a clock fact goes to
    # the LEDGER and is never printed to stdout** -- `grep` for a printed clock
    # literal across cmd/ and internal/ finds nothing, and `rumor run` takes
    # only --seed, --steps and --profile, so no reader can ask it for facts.
    # quill's own arms read `[]ledger.Fact` in process (tests/clock).
    #
    # SO AS A VERDICT IT IS VACUOUS: it passes on every build whether or not
    # the writer is correct, which is this session's recurring defect -- an
    # instrument that cannot separate the two states it exists to distinguish.
    # An arm that can never fire is one to delete, not to keep for the count.
    #
    # IT STAYS AS AN OBSERVATION because a cheap scan that would notice clock
    # material appearing on a channel nobody expects it on costs nothing; it is
    # printed, it is labelled, and it decides nothing. **D7's absence half is
    # the ledger tier's, and the gate's duty there is to confirm THOSE arms can
    # be made to fail, not to re-implement a weaker one over stdout.**
    clock_seen = bool(re.search(r"\bclock\b", out, re.I))

    # SAME FINDING MEANS THE FINDING'S OWN LINE, byte for byte -- fable
    # asked for "same rule, value and attribution", and an rc plus the
    # presence of the token FOUND is a far weaker claim than that.
    found_line = next((l for l in out.splitlines() if "FOUND" in l), None)
    same_found = (found_line == want["found"]) if want["found"] else (
        found_line is None)

    if expect == "identical":
        ok = sha16 == want["sha16"] and rc == want["rc"]
    else:
        ok = rc == want["rc"] and same_found
    ok = (ok and pend_ok and q_open == 0 and not q_bad
          and complete and finding_right)

    mark = "ok " if ok else "***"
    print(f"  {mark} seed {seed:<3} rc {rc} (want {want['rc']})  "
          f"sha16 {sha16} ({want['base']} {want['sha16']})  {lines} lines "
          f"(was {want['lines']})  "
          f"pending={pend}/{len(expected)} declared{pend_why}  "
          f"summary={'present' if complete else 'ABSENT'}  "
          f"FOUND={'present' if found else 'absent'}"
          f"{'' if finding_right else ' *** WRONG WAY FOR THIS SEED'}"
          f"{'' if same_found else ' *** FINDING LINE DIFFERS'}"
          f"  quiesce={'absent' if not qline else str(q_open) + ' open'}"
          f"{' *** ' + q_bad if q_bad else ''}"
          f"{' [observed: a clock token on stdout -- not a verdict]' if clock_seen else ''}")
    return ok


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("tag")
    ap.add_argument("--expect", required=True,
                    choices=("identical", "same-finding"),
                    help="identical for 89/90; same-finding for 91/92")
    # THE DEFAULT IS THIS REPO, DERIVED FROM THE FILE'S OWN LOCATION. It was
    # `/home/rook/rumor` while this tool lived outside the tree, which is one
    # person's clone; in `tools/` the repo containing the tool IS the source,
    # and `--source` stays for driving a different clone or the forge by URL.
    ap.add_argument("--source", default=str(pathlib.Path(__file__).resolve().parent.parent),
                    help="clone source (default: the repo holding this tool). "
                         "A forge URL works too: the ref is cloned from it and "
                         "the frozen subject is then archived from that clone, "
                         "since `git archive` cannot read a URL")
    # RESOLVED, NOT HARD-CODED: this jail has go126 and go125 and no bare
    # `go`, another box has only `go`, and tools/release-gate.py:888 already
    # settled the order for this repo. Hard-defaulting to `go126` here would
    # make the tool refuse on a toolchain it could have used.
    ap.add_argument("--go", default="",
                    help="go binary (default: the first of go, go126, go125 on "
                         "PATH, as tools/release-gate.py resolves it)")
    ap.add_argument("--clock-the-live-toy", action="store_true",
                    help="SELF-TEST ONLY: append `[clock]` to the CLONE's "
                         "examples/toy before the live-toy half runs, which is "
                         "what issue 101's done-when will do. It proves the two "
                         "halves are independent -- bytes against the frozen "
                         "subject unchanged, five fields against a clocked live "
                         "toy still equal. Without it the fields half has never "
                         "been seen distinguishing anything.")
    ap.add_argument("--drop-bin", default="",
                    help="SELF-TEST ONLY: delete this binary after building, to "
                         "prove the missing-binary refusal fires")
    a = ap.parse_args()

    if not a.go:
        for _cand in ("go", "go126", "go125"):
            if shutil.which(_cand):
                a.go = _cand
                break
    if not a.go:
        refuse("no go, go126 or go125 on PATH -- this control builds the "
               "subject, so there is nothing to measure without a toolchain")
    if not shutil.which(a.go):
        refuse(f"{a.go} is not on PATH; this repo's tools resolve go, go126, "
               f"go125 in that order")

    # A FIXTURE SEAM, OPT-IN ONLY: a directory of `seed-<N>.out` and
    # `seed-<N>.rc`, read INSTEAD of cloning and running. It exists because
    # every arm below is an assertion about text, and an assertion never
    # observed failing is of unmeasured value -- the quiesce arms in particular
    # cannot be driven from any tag that exists, since no release emits a
    # non-zero `pending(s) open` yet.
    #
    # THE FIXTURES CARRY REAL CAPTURED OUTPUT, byte for byte from v0.1.31 and
    # v0.1.36, with one line changed per cell. ratchet's r6/145 is why that is
    # written down: their stubs emitted a shape the real graph never had, and
    # 31 arms passed over a fixture that differed from reality in exactly the
    # place the bug lived. A fixture of invented shape tests the fixture.
    fix = os.environ.get("D4_FIXTURE")
    if fix:
        print(f"D4 CONTROL on FIXTURE {fix}, expect={a.expect} "
              f"-- NOT A TAG, NOT A VERDICT, arms only")
        # THE FIXTURE DIRECTORY DECLARES WHICH SEEDS IT CARRIES, by the files
        # it holds. A hardcoded (2, 40, 1) here meant a fixture for seeds 12
        # and 66 was silently IGNORED -- the arms would have passed while
        # driving nothing, which is the failure this seam exists to prevent.
        seeds = sorted(int(f.stem.split("-")[1])
                       for f in pathlib.Path(fix).glob("seed-*.out"))
        if not seeds:
            refuse(f"{fix} holds no seed-N.out file, so nothing would be judged")
        unknown = [s for s in seeds if s not in CONTROL]
        if unknown:
            refuse(f"{fix} carries seed(s) {unknown} with no CONTROL entry; a "
                   f"fixture whose seed is undeclared cannot be judged")
        bad = []
        for seed in seeds:
            out = pathlib.Path(fix, f"seed-{seed}.out").read_text()
            rc = int(pathlib.Path(fix, f"seed-{seed}.rc").read_text().strip())
            if not judge(seed, out, rc, a.expect):
                bad.append(seed)
        if bad:
            print(f"D4 CONTROL: *** {len(bad)} seed(s) did not meet "
                  f"--expect {a.expect}: {bad}")
            return 1
        # WHAT THIS PATH DID NOT COVER, DERIVED FROM THE FIXTURE rather than
        # written as prose. A sentence naming which seeds are missing is wrong
        # the day a fixture is added; this cannot be.
        uncovered = [s for s in ORDER if s not in seeds]
        print(f"D4 CONTROL: all {len(seeds)} fixture seed(s) {seeds} met "
              f"--expect {a.expect}.")
        if uncovered:
            print(f"  NOT COVERED by this fixture: seed(s) {uncovered} -- this "
                  f"path proves nothing about them.")
        return 0

    work = tempfile.mkdtemp(prefix="d4-")
    tree, bindir = os.path.join(work, "src"), os.path.join(work, "bin")
    os.mkdir(bindir)

    # A FRESH CLONE OF THE TAG, which fable required explicitly -- "not from
    # these worktrees". A worktree shares an index and a config with whatever
    # else is using the repo; a drill that can be perturbed by another writer
    # is not a control.
    # A TAG OR A BRANCH CLONES DIRECTLY; A SHA NEEDS THE SECOND PATH, because
    # `git clone --branch` does not take one. That mattered the moment the clock
    # seam below grew a real assertion: the ONLY ref whose tree writes a clock
    # fact is 93084b0, which is untagged, so without this the seam could reach
    # its refuse branch and nothing else -- a guard whose PASS branch is
    # undrivable, which is the same defect as an arm that cannot fail, inverted.
    # ratchet rumor/1328 named it: "the tag where it would take is untagged
    # while the tool cannot take a sha".
    #
    # THE FROZEN SUBJECT IS UNAFFECTED EITHER WAY: it is archived from
    # --source, never from this clone, so a shallow tree here cannot reach it.
    c = run(["git", "clone", "--quiet", "--no-local", "--branch", a.tag,
             "--depth", "1", a.source, tree])
    if c.returncode != 0:
        # NOT A FALLBACK THAT HIDES THE FIRST FAILURE: the full clone is tried
        # and a failure of EITHER is named, with the sha path's own stderr, so
        # "that ref does not exist" never reads as "shas are unsupported".
        c2 = run(["git", "clone", "--quiet", "--no-local", a.source, tree])
        if c2.returncode != 0:
            refuse(f"clone of {a.tag} from {a.source} failed as a tag or branch "
                   f"AND a full clone for a sha failed too", c.stderr + c2.stderr)
        co = run(["git", "checkout", "--quiet", "--detach", a.tag], cwd=tree)
        if co.returncode != 0:
            refuse(f"{a.tag} is not a tag, a branch, or a commit in "
                   f"{a.source} -- nothing was measured", c.stderr + co.stderr)

    head = run(["git", "rev-parse", "HEAD"], cwd=tree).stdout.strip()
    dirty = run(["git", "status", "--porcelain"], cwd=tree).stdout.strip()
    if dirty:
        refuse("the fresh clone is NOT clean, so the tree is not the tag", dirty)

    env = dict(os.environ, PATH=bindir + os.pathsep + os.environ["PATH"])
    for b in BINS:
        r = run([a.go, "build", "-o", os.path.join(bindir, b), f"./cmd/{b}"],
                cwd=tree, env=env)
        if r.returncode != 0:
            refuse(f"building cmd/{b} failed", r.stderr)

    if a.drop_bin:                      # self-test seam, opt-in only
        os.remove(os.path.join(bindir, a.drop_bin))

    # ASSERT THE BINARIES BEFORE RUNNING, not after reading a surprising rc.
    missing = [b for b in BINS if not os.path.exists(os.path.join(bindir, b))]
    if missing:
        refuse("binaries absent from the bindir after the build: "
               + ", ".join(missing)
               + " -- a missing toy-adapter makes the run read rc 2 and the "
                 "control read as lost (ratchet breakout-r6/1)")

    print(f"D4 CONTROL on {a.tag} ({head[:12]}), expect={a.expect}")
    print("  invocation of record: rumor run examples/toy --seed N --steps 10"
          " (seeds 2, 40, 1)")
    print("  and R7's addition (fable rumor/1258): --steps 24 --profile"
          " settling (seeds 12, 66), base d7ce321")
    # loadavg beside any figure: rook, ratchet and plumb share one 4-CPU box.
    print(f"  loadavg {run(['sysctl', '-n', 'vm.loadavg']).stdout.strip()}")

    # MATERIALISE THE FROZEN SUBJECT (fable rumor/1315). `git archive` and not a
    # checkout, so the clone's own working copy is untouched and the extract
    # cannot be perturbed by anything the head did to examples/toy. The path is
    # preserved, so the project directory's BASENAME is still `toy` -- which
    # matters, because the run prints it and the digest covers it (r6/185).
    subjdir = os.path.join(work, "subject")
    os.mkdir(subjdir)
    tarpath = os.path.join(work, "subject.tar")
    # WHERE THE FROZEN SUBJECT IS ARCHIVED FROM, and why it can be either place:
    # git objects are content-addressed, so `SUBJECT:SUBJECT_PATH` is the same
    # bytes out of any repository that holds the commit. The control's integrity
    # comes from the SHA, not from which clone served it.
    #
    # THIS USED TO ARCHIVE FROM --source UNCONDITIONALLY, and `--source` is
    # documented as accepting "the forge by URL" -- which `git -C <url> archive`
    # cannot do at all. The promise in the help was false for this half: driven
    # with a forge URL, every run refused with "could not git archive the frozen
    # subject", a capability failure wearing a missing-subject message.
    _src_is_local = os.path.isdir(os.path.join(a.source, ".git")) or os.path.isdir(
        os.path.join(a.source, "objects"))
    _from, ar = a.source, None
    if _src_is_local:
        ar = run(["git", "-C", a.source, "archive", "--format=tar", "-o", tarpath,
                  SUBJECT, SUBJECT_PATH])
    if ar is None or ar.returncode != 0:
        # THE CLONE AS THE SECOND SOURCE, deepened first if it is shallow: a
        # `--depth 1` clone of a later tag does not contain this commit, and
        # unshallowing is the difference between "the subject is not here" and
        # "the subject is not here YET".
        if run(["git", "-C", tree, "cat-file", "-e", SUBJECT + "^{commit}"]).returncode != 0:
            run(["git", "-C", tree, "fetch", "--quiet", "--unshallow"])
            run(["git", "-C", tree, "fetch", "--quiet", "origin",
                 "+refs/heads/*:refs/remotes/origin/*"])
        ar2 = run(["git", "-C", tree, "archive", "--format=tar", "-o", tarpath,
                   SUBJECT, SUBJECT_PATH])
        if ar2.returncode != 0:
            _why = ("local" if _src_is_local else
                    "NOT a local repository, so `git archive` cannot read it "
                    "however reachable the URL is")
            refuse(f"could not `git archive` the frozen subject {SUBJECT}:"
                   f"{SUBJECT_PATH} -- not from {a.source} ({_why}), and not "
                   f"from the clone of {a.tag} after unshallowing it. The "
                   f"control has no subject, so there is nothing to compare "
                   f"bytes against", (ar.stderr if ar else "") + ar2.stderr)
        _from = f"the clone of {a.tag}"
    import tarfile
    with tarfile.open(tarpath) as tf:
        tf.extractall(subjdir)
    subj_toy = os.path.join(subjdir, SUBJECT_PATH)
    if not os.path.isdir(subj_toy):
        refuse(f"the archive of {SUBJECT} did not yield {SUBJECT_PATH}")
    print(f"  frozen subject: {SUBJECT}:{SUBJECT_PATH} extracted by git archive "
          f"from {_from} -- the BYTES below are against it, the FIELDS against "
          f"the head's own")

    # THE WITNESS SEED, and why it is this one: ibis rumor/1301 measured that a
    # clocked toy moves seed 12's bytes because its stale sentence prints two
    # LEDGER POSITIONS, and clock facts take positions. The other four seeds
    # need not move, so requiring them to would refuse a correct tree.
    WITNESS_SEED = 12

    def _seed_cmd(seed, project):
        spec = CONTROL[seed]
        c = [os.path.join(bindir, "rumor"), "run", project,
             "--seed", str(seed), "--steps", str(spec["steps"])]
        return c + (["--profile", spec["profile"]] if spec["profile"] else [])

    if a.clock_the_live_toy:          # self-test seam, opt-in only
        # THE SEAM NOW PROVES IT TOOK, AND THE PROOF IS RELATIVE (ratchet
        # rumor/1328 with the cure corrected at /1330, ibis /1329).
        #
        # WHAT WAS WRONG: this appended `[clock]`, announced that the live half
        # "now runs 101's done-when", and asserted nothing. On v0.1.38 -- the
        # only tag the tool can be pointed at -- `[clock]` PARSES and nothing
        # writes a clock fact, R7.1's writer not being in that tree, so the
        # clocked and unclocked live lines are identical and the announcement
        # was false. A pass could not be told from the seam not taking: an
        # independence check that cannot fail is not one.
        #
        # AND THE ASSERTION IS AGAINST THIS RUN'S OWN UNCLOCKED BYTES, never
        # against a digest. ratchet's first wording named 8d76ddcf7670f2b6,
        # which ibis identified as a 93084b0 reading -- `runClock` is in three
        # files under internal/schedule there and ZERO at v0.1.38, reproduced
        # from here with `PoolKinds` at 6 in pool.go on both as the control. A
        # constant from another tree would make this reachable only in its
        # refuse branch. A digest, like a line number, is a fact about a TREE.
        _pre = run(_seed_cmd(WITNESS_SEED, "examples/toy"), cwd=tree, env=env)
        with open(os.path.join(tree, "examples", "toy", "bindings.toml"), "a") as fh:
            fh.write("\n[clock]\n")
        _post = run(_seed_cmd(WITNESS_SEED, "examples/toy"), cwd=tree, env=env)
        _a, _b = _sha16(_pre.stdout), _sha16(_post.stdout)
        if _a == _b:
            refuse(f"--clock-the-live-toy DID NOT TAKE on {a.tag}: seed "
                   f"{WITNESS_SEED}'s live bytes are {_a} both with `[clock]` "
                   f"appended and without it. `[clock]` parses on this ref and "
                   f"nothing in it writes a clock fact, so the two halves' "
                   f"independence is UNMEASURED here rather than demonstrated "
                   f"-- which is the whole claim this seam exists to make. "
                   f"Point it at a ref whose tree writes clock facts; the "
                   f"known mover is 93084b0, where seed {WITNESS_SEED} reads "
                   f"8d76ddcf7670f2b6 clocked (ibis rumor/1301) -- that figure "
                   f"is a fact about THAT tree and is not asserted here.")
        print(f"  SELF-TEST: `[clock]` appended to the CLONE's examples/toy and "
              f"it TOOK -- seed {WITNESS_SEED}'s live bytes moved {_a} -> {_b} "
              f"in this run, which is what makes the live half a measurement "
              f"rather than an announcement. The frozen subject is untouched: "
              f"that is the property under test.")

    bad = []
    for seed in ORDER:
        spec = CONTROL[seed]
        def _cmd(project):
            c = [os.path.join(bindir, "rumor"), "run", project,
                 "--seed", str(seed), "--steps", str(spec["steps"])]
            return c + (["--profile", spec["profile"]] if spec["profile"] else [])

        # HALF ONE: the BYTES, against the frozen subject.
        pf = run(_cmd(SUBJECT_PATH), cwd=subjdir, env=env)
        if not pf.stdout.strip():
            refuse(f"seed {seed} produced EMPTY stdout on the frozen subject, so "
                   f"there is nothing to hash and a zero `pending` count would "
                   f"be vacuous", pf.stderr)
        if not judge(seed, pf.stdout, pf.returncode, a.expect):
            bad.append(seed)

        # HALF TWO: the FIVE FIELDS, against the LIVE toy at this head. The
        # bytes here may legitimately differ -- ibis rumor/1301 -- so nothing
        # about them is asserted; only what HAPPENED.
        pl = run(_cmd("examples/toy"), cwd=tree, env=env)
        got, want = fields_of(pl.stdout), spec["fields"]
        if pl.returncode != spec["rc"] or got != want:
            bad.append(seed)
            print(f"  *** seed {seed} LIVE TOY: rc {pl.returncode} (want "
                  f"{spec['rc']}), fields {got} (want {want}) -- what happened "
                  f"MOVED, which no amount of re-basing a digest would show")
        else:
            # NAME THE LIVE DIGEST WHEN IT DIFFERS, so the figure is
            # comparable with whoever else measured it. ibis read
            # 8d76ddcf7670f2b6 for seed 12 under a clocked toy (rumor/1301);
            # a report saying only "bytes differ" cannot be checked against it.
            same = ("same bytes too" if _sha16(pl.stdout) == _sha16(pf.stdout)
                    else f"bytes differ ({_sha16(pl.stdout)}), fields equal -- "
                         f"the ibis /1301 case")
            print(f"  ok  seed {seed} LIVE TOY: rc {pl.returncode}, five fields "
                  f"equal ({same})")

    # D6's SECOND HALF AND D12 RIDE HERE, beside the control's invocation,
    # which is where fable asked for the figure to live (rumor/1190).
    obs, (cases, agreed, disagreed) = run_check(
        os.path.join(bindir, "rumor"), tree, env)
    cases_ok, lines = cases_verdict(obs, cases, agreed, disagreed, a.expect)
    for l in lines:
        print(l)

    if bad or not cases_ok:
        if bad:
            print(f"D4 CONTROL: *** {len(bad)} seed(s) did not meet "
                  f"--expect {a.expect}: {bad}")
        if not cases_ok:
            print("D4 CONTROL: *** the check case set did not meet "
                  f"--expect {a.expect}")
        return 1
    print(f"D4 CONTROL: all {len(ORDER)} seeds met --expect {a.expect}, "
          f"every pending line the declared one, each absence arm partnered.")
    # NAMED, NOT CLAIMED: this half of D7 has no presence partner until R7.1
    # writes a clock fact, so it can go vacuous without saying so -- which is
    # exactly what an absence assertion does when its subject is renamed. It is
    # reported as UNPARTNERED until a clocked project can be driven beside it.
    print("  D7's absence half is NOT judged here: a clock fact goes to the "
          "ledger and is never printed to stdout, so a stdout arm passes on "
          "every build. It belongs to the ledger tier (tests/clock); this "
          "control reports a token scan as an observation only.")
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except SystemExit:
        raise
    except Exception as exc:                       # never rc 1 by accident
        refuse(f"{type(exc).__name__}: {exc}")
