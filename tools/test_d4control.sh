#!/bin/sh
# MUTATION ARMS FOR d4control.py's TEXT ASSERTIONS.
#
# Every cell below takes the REAL v0.1.36 capture in fixtures/d4-v0136 and
# changes ONE line. The baseline cell exists to prove the fixture still passes
# unmutated -- without it a mutant that passes for the wrong reason, or a
# fixture that has rotted, reads as a healthy kill.
#
# WHY A FIXTURE AT ALL: the quiesce arms cannot be driven from any tag that
# exists. No release emits a non-zero `pending(s) open`, and an arm that only
# ever sees a zero has never been observed doing its job. These cells are the
# only place those two assertions are measured.
set -u
HERE=$(cd "$(dirname "$0")" && pwd)
FIX="$HERE/fixtures/d4-v0136"
WORK=$(mktemp -d)
pass=0; fail=0

cell() {                        # cell <name> <want-rc> <mutator...>
  name=$1; wantrc=$2; shift 2
  d="$WORK/$name"; mkdir -p "$d"; cp "$FIX"/* "$d"/
  ( cd "$d" && "$@" )
  D4_FIXTURE="$d" python3 "$HERE/d4control.py" v0.1.36 --expect same-finding \
    > "$d/log" 2>&1
  rc=$?
  if [ "$rc" -eq "$wantrc" ]; then
    pass=$((pass + 1)); printf '  ok   %-14s rc %s\n' "$name" "$rc"
  else
    fail=$((fail + 1))
    printf '  *** %-14s rc %s, wanted %s\n' "$name" "$rc" "$wantrc"
    sed 's/^/        /' "$d/log"
  fi
}

cellf() {                       # cellf <fixture-dir> <name> <want-rc> <mutator...>
  fixdir=$1; name=$2; wantrc=$3; shift 3
  d="$WORK/$name"; mkdir -p "$d"; cp "$fixdir"/seed-* "$d"/
  ( cd "$d" && "$@" )
  D4_FIXTURE="$d" python3 "$HERE/d4control.py" v0.1.38 --expect same-finding \
    > "$d/log" 2>&1
  rc=$?
  if [ "$rc" -eq "$wantrc" ]; then
    pass=$((pass + 1)); printf '  ok   %-18s rc %s\n' "$name" "$rc"
  else
    fail=$((fail + 1))
    printf '  *** %-18s rc %s, wanted %s\n' "$name" "$rc" "$wantrc"
    sed 's/^/        /' "$d/log"
  fi
}

echo "D4CONTROL ARMS (mutants of the real v0.1.36 capture)"

# A. THE BASELINE. Unmutated real output must pass, or every kill below is
#    meaningless -- a fixture that fails for its own reasons kills everything.
cell baseline 0 true

# B. THE QUIESCE COUNT. A pending left open at the end is precisely what D4
#    forbids, and it is spelled with the same word the permitted line uses.
cell q-open-2 1 sed -i '' 's/0 pending(s) open/2 pending(s) open/' seed-1.out

# C. A TRAILING pending OUTSIDE THE SUMMARY -- fable's "may not gain a
#    trailing `pending`", which the word-count arm must still catch now that
#    the summary line is exempt from it.
cell pending-line 1 sh -c 'printf "run: pending data.rumor.statute.finding at step 7\n" >> seed-2.out'

# D. THE FINDING'S OWN LINE. Same rc, same FOUND token, different step: the
#    weak version of this arm (rc + "FOUND" present) passes this mutant.
cell found-moved 1 sed -i '' 's/after step 4/after step 5/' seed-2.out

# E. A MALFORMED QUIESCE SUMMARY. The count cannot be read, which is NOT the
#    same as a count of zero, and must not be silently treated as one.
cell q-unparseable 1 sed -i '' 's/0 pending(s) open at the end/no pendings open at the end/' seed-1.out

# F. THE COMPLETENESS WITNESS. Drop the summary line and the output is a
#    truncation; every absence reading over it would be vacuous.
cell no-summary 1 sed -i '' '/toy -- seed/d' seed-1.out

# G. THE CLEAN SEED FINDING A BUG. The louder half of "no new noise", and the
#    half a presence-only check cannot see.
cell clean-finds 1 sh -c 'printf "run: FOUND bug after step 9 -- data.rumor.statute.finding, rests_on_planted false\n" >> seed-1.out'

# H. A NEGATED MENTION IS NOT A PENDING, and this cell is the partner of
#    pending-line: the SAME word, in a phrase asserting there is none, must be
#    ACCEPTED. 92's shrink line is "each settled with no pending left open", and
#    the arm read it as a pending until the mention -- not the line -- was
#    judged. Without this cell the generalisation is unmeasured in the
#    direction that matters.
cell negated-pending 0 sh -c 'printf "shrink: 8 candidate(s), each replayed on a verified clean world and each settled with no pending left open\n" >> seed-2.out'


# ============================================================================
# R7's TWO ADDED CONTROL SEEDS (fable rumor/1258), over the REAL d7ce321
# capture in fixtures/d4-r7-d7ce321. These cells exist because the
# pending-SENTENCE arm is new: seeds 2, 40 and 1 declare the empty set, so not
# one cell above can tell a correct pending line from a wrong one.
# ============================================================================
R7FIX="$HERE/fixtures/d4-r7-d7ce321"

# I. THE BASELINE, same reason as cell A: a rotted fixture kills everything.
cellf "$R7FIX" r7-baseline 0 true

# J. THE STEP MOVED. Same number of pending lines, one wrong step. A count arm
#    reads 2 and passes; this is the mutant that measures the difference.
cellf "$R7FIX" r7-step-moved 1 sed -i '' 's/held at step 11/held at step 12/' seed-66.out

# K. AN ACTOR SWAPPED. Count unchanged, shape unchanged, a stranger in the row
#    -- the same defect ratchet caught in my check-case arm as `case-swapped`.
cellf "$R7FIX" r7-actor-swapped 1 sed -i '' 's|member_1/event_tally|guest_9/event_tally|' seed-66.out

# L. A PENDING LINE DROPPED. The absence half: a cleared pending that stops
#    being reported is exactly what D1 exists to notice.
cellf "$R7FIX" r7-pending-dropped 1 sed -i '' '/held at step 20/d' seed-66.out

# M. THE VERDICT FLIPPED. `consistent` to a bug of kind stale on the line that
#    carries D1's whole claim -- same actor, same step, opposite meaning.
cellf "$R7FIX" r7-verdict-flipped 1 sed -i '' 's/after the last step: consistent/after the last step: bug of kind stale/' seed-66.out

# N. THE EXPLANATION DROPPED. Seed 12 mentions a pending in THREE lines and the
#    third is the stale finding's own explanation. A shape rule exempting
#    `pending -- ...` would never have noticed this one going missing.
cellf "$R7FIX" r7-explain-dropped 1 sed -i '' '/was held open as/d' seed-12.out

# O. DELETED, AND THE REASON KEPT. This cell asserted that a `clock` token in
#    an unclocked project's stdout FAILS the seed. Measured at 5e89313 -- the
#    commit that writes the clock -- that verdict is vacuous: a clock fact goes
#    to the LEDGER and nothing prints one to stdout, so the arm passed on every
#    build regardless of the writer. All this cell ever proved was that my own
#    regex matches text I had just written into a fixture, which is a test of
#    the fixture. D7's absence half is the ledger tier's (tests/clock).

# P. THE ADDITIONS LIST, DRIVEN AS A FUNCTION (R7.4). Every cell above drives
#    the SEED half from a capture, and no fixture carries a `rumor check`
#    output -- so `cases_verdict`'s additions rule was asserted by nothing.
#    rook's D7 drive at coop rumor/1411 refused on a stale ADDED_BY, declined
#    to edit the expectation at drive time, and that is what this cell exists
#    to catch next time: the map is per phase, and a phase that plants a case
#    without declaring it here must REFUSE rather than pass.
#
#    It is python because the subject is a python function over text, and the
#    alternative -- a captured `rumor check` output per phase -- would need a
#    new fixture every time the case set grows, which is the rot the SET
#    invariant exists to avoid.
addcell() {                     # addcell <name> <want-ok> <observed-extra...>
  name=$1; want=$2; shift 2
  # PYTHONDONTWRITEBYTECODE, AND IT IS NOT HOUSEKEEPING: importing d4control by
  # path writes tools/__pycache__/, which `rumor docs --check` refuses as a
  # path no coverage entry names -- measured the moment this cell first ran.
  # The same shape as `go build` dropping a binary in the tree.
  got=$(PYTHONDONTWRITEBYTECODE=1 python3 - "$HERE" "$@" <<'PYEOF'
import sys, pathlib, importlib.util
here = pathlib.Path(sys.argv[1])
spec = importlib.util.spec_from_file_location("d4", here / "d4control.py")
mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mod)
want = set(mod.CASES_31.read_text().split())
observed = set(want) | set(sys.argv[2:])
ok, _ = mod.cases_verdict(observed, len(observed), len(observed), 0, "same-finding")
print("ok" if ok else "refused")
PYEOF
)
  if [ "$got" = "$want" ]; then
    pass=$((pass + 1)); printf '  ok   %-18s %s\n' "$name" "$got"
  else
    fail=$((fail + 1)); printf '  *** %-18s %s, wanted %s\n' "$name" "$got" "$want"
  fi
}

# EVERY DECLARED ADDITION PRESENT: the only shape that passes. 103 plants TWO,
# which is why ADDED_BY's values are lists.
addcell add-all-declared ok toy/dropped-recount toy/invitation-expiry toy/invitation-honored-late toy/invitation-honored-at-the-expiry toy/accepted-then-the-clock-moved
# ONE MISSING: a phase's row lost is a refusal, not a pass.
addcell add-one-missing refused toy/dropped-recount toy/invitation-expiry
# AN UNDECLARED ADDITION: a case that arrives without an ADDED_BY entry, which
# is exactly rook's /1411 refusal.
addcell add-undeclared refused toy/dropped-recount toy/invitation-expiry toy/invitation-honored-late toy/invitation-honored-at-the-expiry toy/accepted-then-the-clock-moved toy/nobody-declared-me

printf 'D4CONTROL ARMS: %s passed, %s failed\n' "$pass" "$fail"
rm -rf "$WORK"
[ "$fail" -eq 0 ]
