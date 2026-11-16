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


def read_events(path):
    """Answer (events, rebuilt_log_text, foreign_line_count)."""
    events, log, foreign = [], [], 0
    with open(path, "r", errors="replace") as handle:
        for line in handle:
            try:
                event = json.loads(line)
            except ValueError:
                foreign += 1
                log.append(line if line.endswith("\n") else line + "\n")
                continue
            if not isinstance(event, dict):
                foreign += 1
                log.append(line if line.endswith("\n") else line + "\n")
                continue
            events.append(event)
            if event.get("Output"):
                log.append(event["Output"])
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

    AND THE READ-TIME IS THE END, WHICH IS A PROPERTY OF HOW go BUFFERS. With
    `-v ./...` go runs packages concurrently but holds each package's output
    and prints it as one block WHEN THAT PACKAGE FINISHES. So the moment a
    verdict line is read is the moment the package ended, and its start is that
    minus the elapsed it reports. The overlap this recovers is therefore real,
    and it is the figure the flush had erased.
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
    args = ap.parse_args()

    try:
        events, log, foreign = read_events(args.json)
    except OSError as err:
        # NOT FATAL TO THE BUILD: this script reports on a suite that has
        # already run and whose verdict is carried elsewhere. Failing the step
        # here would turn a missing report into a missing result.
        print(f"TIMELINE UNAVAILABLE: cannot read {args.json}: {err}")
        return 0

    if args.log:
        try:
            with open(args.log, "w") as handle:
                handle.write(log)
        except OSError as err:
            print(f"TIMELINE: could not rebuild {args.log}: {err}")
            return 1

    rows = spans(events)
    if not rows:
        # The tee'd shape: no Package fields, names in the text. See
        # spans_from_output for why the read-time is the package's END.
        rows = spans_from_output(events)
    if not rows:
        print(f"TIMELINE: no package events in {args.json} "
              f"({len(events)} event(s), {foreign} non-JSON line(s))")
        return 0

    started = [r["start"] for r in rows.values() if r["start"]]
    ended = [r["end"] for r in rows.values() if r["end"]]
    first = min(started) if started else None
    wall = (max(ended) - first).total_seconds() if ended and first else 0.0
    best, at = peak_overlap(rows)

    print(f"TIMELINE {len(rows)} package(s), wall {wall:.1f}s, peak concurrency {best}"
          + (f", {foreign} non-JSON line(s) passed through" if foreign else ""))
    if at is not None:
        print(f"  peak at +{(at - first).total_seconds():.1f}s: "
              + ", ".join(shorten(p) for p in live_at(rows, at)))

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
