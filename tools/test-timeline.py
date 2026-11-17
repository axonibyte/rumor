#!/usr/bin/env python3.12
"""Turn `go test -json` into a per-package TIMELINE, and rebuild the plain log.

WHY IT EXISTS. The job log is the only artifact that outlives a runner's
workspace, and until now it could not answer WHEN a package ran -- only how
long it took. The runner emits a step's output in one flush, so every line of
a 1400-second suite carries the same wall-clock stamp: diagnosing task 1240
meant reading per-package elapsed figures and GUESSING at overlap, because
overlap is exactly what the stamps had erased. `go test -json` carries a real
Time on every event, so the timeline is recoverable -- but only if the stream
is stamped while the suite runs, which is why ci.yml TEES the suite into
`go tool test2json -t` rather than parsing ../test.log afterwards.

TWO INPUT SHAPES, AND THE TEE'D ONE IS WHAT CI FEEDS IT. `go test -json` sets
a Package on every event and this reads it directly; a tee'd stream through
standalone test2json sets NO Package at all, and the names come from the
verdict lines instead (see spans_from_output). Both are supported because the
richer shape is what a human runs by hand.

../test.log IS NOT THIS SCRIPT'S TO WRITE IN CI. The failure view and the
rc-124 diagnostic mine it, every pattern they use is driven by an arm in
tests/hygiene, and so the log stays the output of `go test` through `tee` --
never of an interpreter, a parser, or the module building. --log exists for
the by-hand case, where the JSON is the only copy.

AND A LINE THAT IS NOT JSON IS PASSED THROUGH, NEVER DROPPED. `go test -json`
puts its own framing on stdout but a BUILD failure arrives as plain text, and
the test step redirects stderr into the same file. A rebuild that parsed
strictly would turn a compile error into an empty log -- the same shape as the
failure view that searched nothing and printed a reassuring sentence (ci.yml's
own record, run 1012). Unparsable lines therefore reach the log verbatim and
are counted, and the count is reported so a reader knows the stream was mixed.
"""

import argparse
import json
import re
import sys
from datetime import datetime, timedelta, timezone


def parse_time(raw):
    """RFC3339 with up to nanoseconds, which fromisoformat will not take.

    Go emits nine fractional digits; fromisoformat accepts six. Truncating is
    right rather than rounding: these figures are read at second resolution and
    a rounded nanosecond cannot change a verdict, where a parse failure would
    silently drop the event that carries a package's start.
    """
    if not raw:
        return None
    text = raw.strip().replace("Z", "+00:00")
    if "." in text:
        head, _, tail = text.partition(".")
        digits = ""
        for ch in tail:
            if ch.isdigit():
                digits += ch
            else:
                tail = tail[len(digits):]
                break
        else:
            tail = ""
        text = f"{head}.{digits[:6]}{tail}"
    try:
        stamp = datetime.fromisoformat(text)
    except ValueError:
        return None
    if stamp.tzinfo is None:
        stamp = stamp.replace(tzinfo=timezone.utc)
    return stamp


# WHAT THE RENDERING SAYS WHERE THE RECORD STOPS BEING ONE. A line that is not
# a test event is marked where it sits rather than passed through bare, and a
# record whose LAST line is unterminated gets a closing note: that is the
# signature of a suite cut mid-write, and the failure mode being removed here is
# a silently short log (fable's condition, coop r10/18).
#
# MARKED, NOT ASSERTED AS TRUNCATION. A foreign line can also appear mid-stream
# without the record ending -- tests/hygiene drives exactly that case -- so the
# inline marker says only what is known, and the truncation note is reserved for
# a final line with no newline on it.
NOT_AN_EVENT = ("*** the line below is not a test event, so it is shown raw and "
                "is not part of the record ***\n")
CUT_NOTE = ("*** test.json ENDS MID-LINE: the suite was cut while writing this "
            "record, so nothing after this point was ever written. The rendering "
            "stops here because the record does. ***\n")


def read_events(path):
    """Answer (events, rebuilt_log_text, foreign_line_count)."""
    events, log, foreign = [], [], 0
    unterminated = False
    with open(path, "r", errors="replace") as handle:
        for line in handle:
            unterminated = not line.endswith("\n")
            try:
                event = json.loads(line)
            except ValueError:
                foreign += 1
                log.append(NOT_AN_EVENT)
                log.append(line if line.endswith("\n") else line + "\n")
                continue
            if not isinstance(event, dict):
                foreign += 1
                log.append(NOT_AN_EVENT)
                log.append(line if line.endswith("\n") else line + "\n")
                continue
            unterminated = False
            events.append(event)
            if event.get("Output"):
                log.append(event["Output"])
    if unterminated:
        log.append(CUT_NOTE)
    return events, "".join(log), foreign


# A package's life ends on one of these, and `skip` counts: a package that
# skipped still occupied the box for its elapsed and still competed for the
# browsers that tests/run needs.
TERMINAL = {"pass", "fail", "skip"}


def spans(events):
    """First-seen and terminal time per package, with its reported elapsed."""
    out = {}
    for event in events:
        pkg = event.get("Package")
        if not pkg:
            continue
        when = parse_time(event.get("Time"))
        row = out.setdefault(pkg, {"start": None, "end": None, "elapsed": None,
                                   "action": None})
        if when is not None and (row["start"] is None or when < row["start"]):
            row["start"] = when
        if event.get("Action") in TERMINAL:
            # THE TERMINAL EVENT AND NOT THE LAST ONE SEEN: go emits trailing
            # output after a verdict, so "last event" would stretch a package
            # past the point it stopped competing for the box.
            if when is not None and (row["end"] is None or when > row["end"]):
                row["end"] = when
            if event.get("Elapsed") is not None:
                row["elapsed"] = event["Elapsed"]
            row["action"] = event["Action"]
    return out


# A package's verdict line as `go test -v` prints it, which is the only place
# a package is NAMED when the stream was tee'd rather than produced by
# `go test -json`. Tab-separated in practice; whitespace here so a future
# spacing change cannot silently empty the timeline.
VERDICT = re.compile(r"^(ok|FAIL|\?)\s+(\S+)(?:\s+([0-9.]+)s)?")


def spans_from_output(events):
    """Derive the timeline from verdict lines when no Package field exists.

    WHY THIS PATH EXISTS AND IS NOT A FALLBACK IN NAME ONLY: `go tool test2json`
    invoked standalone has no `-p`, so it attributes nothing -- every event
    arrives with a Time and no Package. The test step feeds it that way on
    purpose, because the alternative is making ../test.log depend on a parser.
    So the package names come from the text, and the clock from the event.

    AND THE READ-TIME IS A PRINT TIME, NOT AN END, which I had backwards and
    published a figure on. `go test` emits package blocks in LISTED order, so
    a slow package holds back the print of every package behind it: in task
    1245 all 46 shared FOUR print instants. A start computed as
    print-minus-elapsed is therefore shifted by however long that package
    waited to be printed, and overlap computed from it is an artifact. These
    rows are still useful -- each package's OWN elapsed and verdict are exact,
    and the order of the batches is real -- but they cannot answer what ran
    beside what, and main() refuses to.
    """
    out = {}
    for event in events:
        text = event.get("Output") or ""
        match = VERDICT.match(text)
        if not match:
            continue
        action, pkg, elapsed = match.group(1), match.group(2), match.group(3)
        end = parse_time(event.get("Time"))
        secs = float(elapsed) if elapsed else None
        start = end - timedelta(seconds=secs) if (end and secs is not None) else None
        out[pkg] = {
            "start": start,
            "end": end,
            "elapsed": secs,
            "action": {"ok": "pass", "FAIL": "fail", "?": "none"}[action],
        }
    return out


def peak_overlap(rows):
    """The most packages in flight at once, and when.

    A SWEEP OVER ENDPOINTS rather than a sampled clock: sampling can step over
    a short-lived peak entirely, and the peak is the figure a contention
    argument rests on.
    """
    marks = []
    for row in rows.values():
        if row["start"] is None or row["end"] is None:
            continue
        marks.append((row["start"], 1))
        marks.append((row["end"], -1))
    if not marks:
        return 0, None
    marks.sort(key=lambda m: (m[0], -m[1]))
    live = best = 0
    at = None
    for when, delta in marks:
        live += delta
        if live > best:
            best, at = live, when
    return best, at


def live_at(rows, moment):
    return sorted(p for p, r in rows.items()
                  if r["start"] and r["end"] and r["start"] <= moment <= r["end"])


def shorten(pkg):
    return pkg.split("/rumor/", 1)[1] if "/rumor/" in pkg else pkg


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--json", required=True, help="the go test -json stream")
    ap.add_argument("--log", help="rebuild the plain `go test -v` log here")
    ap.add_argument("--min-seconds", type=float, default=0.0,
                    help="only list packages at or above this elapsed")
    ap.add_argument("--reported", action="store_true",
                    help="print only how many packages reached a verdict")
    ap.add_argument("--passed", action="store_true",
                    help="print only how many packages passed")
    ap.add_argument("--cached", action="store_true",
                    help="print any package whose verdict came from the test cache")
    ap.add_argument("--failures", action="store_true",
                    help="print each failing package and test with its own output")
    args = ap.parse_args()

    try:
        events, log, foreign = read_events(args.json)
    except OSError as err:
        # NOT FATAL TO THE BUILD: this script reports on a suite that has
        # already run and whose verdict is carried elsewhere. Failing the step
        # here would turn a missing report into a missing result.
        print(f"TIMELINE UNAVAILABLE: cannot read {args.json}: {err}")
        return 0

    # THE COUNT COMES OUT OF THE RECORD'S FIELDS AND NOT OUT OF ITS TEXT, which
    # is the point of the mode. ci's "N of M packages reported" used to count
    # `^(ok|FAIL|\?)` lines in the rendered log, and those anchors match NOTHING
    # in test.json -- measured, `grep -cE '^ok[[:space:]]' test.json` is 0 -- so
    # a fraction repointed at the record without changing its instrument would
    # have read zero and called it a shortfall. A package reaches a verdict when
    # it emits pass, fail or skip with a Package and NO Test, which is also true
    # of a package that failed to BUILD, so the count includes what no text
    # pattern of ours ever matched reliably.
    # THE INSTRUMENTS BELOW READ THE RECORD AND NOT THE RENDERING, which is
    # condition (5) of the ruling at coop r10/22: test.log is for eyes, and
    # anything that DECIDES reads test.json. Every one of them replaced an
    # anchored grep -- `^ok`, `^(--- )?FAIL`, `^panic:` -- and anchors match
    # nothing in a record whose every line is a JSON object: measured, a
    # `grep -cE '^ok[[:space:]]'` over test.json answers 0. So each claim is
    # computed from the fields that carry it instead of repointed blindly.
    if args.passed:
        # A PACKAGE PASSED when it emits pass with a Package and no Test. The
        # grep this replaces was `^ok<TAB>forge...`, and its claim -- "the
        # harness did not run any package" -- is the same one.
        print(len({e["Package"] for e in events
                   if e.get("Action") == "pass" and e.get("Package") and not e.get("Test")}))
        return 0

    if args.cached:
        # A CACHED VERDICT IS A PRESENT ok-LINE FOR A PACKAGE THAT DID NOT RUN,
        # which is the claim the ok-line check cannot make for itself. The
        # marker lives in the package's own output text and in no field, so this
        # reads the text of OUTPUT EVENTS ONLY -- not the whole file -- because
        # a bare `grep '(cached)'` over the record would also match a test that
        # printed those nine characters itself, and this check EXITS the build.
        for event in events:
            if event.get("Action") != "output" or event.get("Test"):
                continue
            text = event.get("Output") or ""
            if "(cached)" in text and text.startswith("ok"):
                print(f"{event.get('Package')}\t{text.strip()}")
        return 0

    if args.failures:
        # FAILURE CONTEXT SCOPED TO WHAT FAILED, which is strictly better than
        # the five lines either side the grep gave: under package parallelism
        # those neighbours belong to whatever else was printing, and the ones
        # that matter may be fifty lines back. Here a failing package's own
        # output is its context, by construction.
        failed = [e for e in events if e.get("Action") == "fail"]
        if not failed:
            return 0
        # THE COMPILER'S TEXT FIRST, AND UNATTRIBUTED BECAUSE IT IS. build-output
        # events carry NO Package -- measured 2026-11-17 -- so scoping context by
        # package drops the one thing a build failure needs explained. Printing
        # them under their own header was the fix for exactly that: the first cut
        # of this mode showed `FAIL m/broken [build failed]` and not a word about
        # why it would not compile.
        build = [e.get("Output") or "" for e in events
                 if e.get("Action") == "build-output" and e.get("Output")]
        if build:
            print("--- BUILD OUTPUT (carried by no package, so it is shown whole):")
            for text in build:
                print("    " + text.rstrip("\n"))
        for event in failed:
            pkg, test = event.get("Package"), event.get("Test")
            where = f"{pkg} {test}".strip() if test else f"{pkg}"
            print(f"--- FAILED: {where}")
            for other in events:
                if other.get("Action") not in ("output", "build-output"):
                    continue
                if other.get("Package") != pkg:
                    continue
                if test and other.get("Test") != test:
                    continue
                text = other.get("Output") or ""
                if text:
                    print("    " + text.rstrip("\n"))
        return 0

    if args.reported:
        print(len({e["Package"] for e in events
                   if e.get("Action") in TERMINAL and e.get("Package") and not e.get("Test")}))
        return 0

    if args.log:
        try:
            with open(args.log, "w") as handle:
                handle.write(log)
        except OSError as err:
            print(f"TIMELINE: could not rebuild {args.log}: {err}")
            return 1

    rows = spans(events)
    timed = bool(rows)
    if not rows:
        # The tee'd shape: no Package fields, names in the text. These rows
        # carry PRINT times and not completion times -- see spans_from_output.
        rows = spans_from_output(events)
    if not rows:
        print(f"TIMELINE: no package events in {args.json} "
              f"({len(events)} event(s), {foreign} non-JSON line(s))")
        return 0

    started = [r["start"] for r in rows.values() if r["start"]]
    ended = [r["end"] for r in rows.values() if r["end"]]
    first = min(started) if started else None
    wall = (max(ended) - first).total_seconds() if ended and first else 0.0
    mixed = f", {foreign} non-JSON line(s) passed through" if foreign else ""

    # CONCURRENCY IS REPORTED ONLY FROM TIMES THAT ARE REAL, which the tee'd
    # shape's are not. WITHDRAWN AFTER PUBLISHING A FIGURE THAT WAS AN
    # ARTIFACT (quill r9/418, ibis r9/419): `go test` emits package blocks in
    # LISTED order, so a slow package holds back the print of everything
    # behind it and a batch lands at one instant. In task 1245 all 46 packages
    # shared FOUR end times -- 19 at +198.9, 15 at +199.0, 7 at +1558.3, 5 at
    # +9.7 -- and the overlap computed from them read "peak concurrency 30" on
    # a box whose -p caps real parallelism at GOMAXPROCS, which is 8 there.
    # Nineteen packages counted as running together had finished and were
    # queued to print.
    #
    # SO THE FIGURE IS REMOVED RATHER THAN LABELLED. A number that is right
    # under `go test -json` and wrong under the tee reads the same in both,
    # and a reader cannot tell which they have. `-json` does NOT batch, driven
    # on matrix: tests/clock listed first ends +10.5 while internal/decl
    # second ends +0.0. If CI ever feeds this that shape, `timed` is true and
    # the figure comes back honestly.
    if timed:
        best, at = peak_overlap(rows)
        print(f"TIMELINE {len(rows)} package(s), wall {wall:.1f}s, "
              f"peak concurrency {best}{mixed}")
        if at is not None:
            print(f"  peak at +{(at - first).total_seconds():.1f}s: "
                  + ", ".join(shorten(p) for p in live_at(rows, at)))
    else:
        print(f"TIMELINE {len(rows)} package(s), wall {wall:.1f}s{mixed}")
        print("  times below are PRINT times: `go test` emits package blocks in "
              "listed order, so these cannot be read as overlap")

    # UNREPORTED PACKAGES ARE NAMED FIRST, because they are the ones a timeout
    # is about: `go test` reports a package when it finishes, so a package with
    # a start and no terminal event was still running when the budget expired.
    running = sorted(p for p, r in rows.items() if r["end"] is None)
    if running:
        print(f"  STILL RUNNING AT THE END ({len(running)}): "
              + ", ".join(shorten(p) for p in running))

    print(f"  {'start':>8} {'end':>8} {'elapsed':>8}  {'verdict':<7} package")
    for pkg, row in sorted(rows.items(),
                           key=lambda kv: (kv[1]["start"] or first)):
        elapsed = row["elapsed"]
        if elapsed is not None and elapsed < args.min_seconds:
            continue
        start = f"+{(row['start'] - first).total_seconds():.1f}" if row["start"] else "?"
        end = f"+{(row['end'] - first).total_seconds():.1f}" if row["end"] else "RUNNING"
        shown = f"{elapsed:.1f}" if elapsed is not None else "?"
        print(f"  {start:>8} {end:>8} {shown:>8}  {row['action'] or '?':<7} {shorten(pkg)}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
