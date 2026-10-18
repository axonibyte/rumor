#!/bin/sh
# What `go mod tidy -diff` actually said, classified into the answers a CI job
# can act on.
#
# WHY THIS IS A FILE AND NOT FIVE LINES OF YAML. The same reason
# govulncheck-verdict.sh and verify-tag-verdict.sh exist, and ci.yml's battery
# step states it: a workflow step is only exercised by pushing, so a
# classification written inline cannot be driven. This one has four answers and
# nine clauses, which is four more than a step should decide in prose.
#
# WHAT IT EXISTS TO CLOSE. Until v0.1.19 the gate asserted `go.mod` EXISTS and
# never that it was RIGHT: ci.yml's fetch step carries `test -f go.mod || exit
# 1`, which is a wrong-tree guard that reads none of the file's content, and
# tag-pipeline.yml mentions the file only in a comment. So an unused require, a
# missing one, or a go.sum short an entry could travel to a tag with nothing in
# the project to say so -- including through v0.1.18, which this author cut.
#
# THE MISATTRIBUTION THIS IS SHAPED AROUND. `go mod tidy -diff` exits non-zero
# for BOTH "these files are not what tidying would write" and "I could not read
# the module graph", and tidying needs the network: it resolves the test-only
# dependencies of dependencies, 32 modules on a cold cache here. A step reading
# only the exit code would blame the manifest for an unreachable proxy -- the
# same defect v0.1.18 fixed in the doc blocks, where a jail with no Go read as
# documentation drift, and the same one run 192 made visible in the vulnerability
# step, where nine CVEs were reported as a database outage. **The diff is on
# stdout and the excuse is on stderr**, so the two are captured apart and the
# answer is read from which file has content.
#
# THE ANSWERS, because "non-zero" is not one of them:
#   0  TIDY         the manifest is what tidying would write
#   3  INACCURATE   a verdict ON THE TREE: the diff is the finding, apply it
#   2  STEP DEFECT  this script was invoked wrongly, or the tool contradicted
#                   its own documented contract
#   1  BLOCKED      no answer was reached. NOT a pass, and not a verdict on the
#                   manifest either
#
# Every clause is NAMED on stdout, because a composite boolean is satisfied by
# its weakest clause and a control reading only the exit code cannot tell which
# one answered.

set -u

usage() {
    echo "usage: gomod-accuracy-verdict.sh <tidy-diff-file> <tidy-err-file> <rc> <go.sum-path>" >&2
    echo "       gomod-accuracy-verdict.sh --self-test" >&2
}

# classify <difffile> <errfile> <rc> <gosum> -- prints "CLAUSE <id>" then a
# human line; returns 0 tidy, 3 inaccurate, 2 step defect, 1 blocked.
classify() {
    diff_f=$1
    err_f=$2
    rc=$3
    gosum=$4

    if [ ! -f "$diff_f" ]; then
        echo "CLAUSE no-capture"
        echo "there is no file at $diff_f, so tidy's stdout was never captured and no comparison happened. BLOCKED, not passed."
        return 1
    fi

    if [ ! -f "$err_f" ]; then
        echo "CLAUSE no-stderr-capture"
        echo "there is no file at $err_f. This script tells a finding from an excuse by which stream carried it, so a step that does not capture stderr separately has defeated it. STEP DEFECT."
        return 2
    fi

    if [ "$rc" -eq 124 ]; then
        echo "CLAUSE timed-out"
        echo "tidy hit its timeout. rc 124 proves only that the time elapsed -- it cannot tell a slow module proxy from a hung one -- so this blocks the arm rather than failing it. RUNNER finding."
        [ -s "$err_f" ] && cat "$err_f"
        return 1
    fi

    if [ "$rc" -ne 0 ]; then
        # THE WHOLE POINT OF THE SPLIT. A diff on stdout is a verdict about the
        # files; anything else with a non-zero status is the tool saying it
        # could not look.
        if [ -s "$diff_f" ]; then
            echo "CLAUSE inaccurate"
            echo "tidy exited $rc and wrote a diff, so go.mod or go.sum is not what tidying would write. This is a verdict ON THE TREE and the diff below IS the finding: applying it is the fix."
            cat "$diff_f"
            return 3
        fi
        if [ -s "$err_f" ]; then
            echo "CLAUSE could-not-read-the-graph"
            echo "tidy exited $rc with an empty diff and an explanation below, so it never finished reading the module graph. A PROVISIONING finding about the proxy, the cache or the route -- it says NOTHING about go.mod."
            cat "$err_f"
            return 1
        fi
        echo "CLAUSE nonzero-and-silent"
        echo "tidy exited $rc and wrote neither a diff nor a word of explanation. Nothing here can be acted on, so the arm is BLOCKED; read the step's own output whole."
        return 1
    fi

    # rc 0 from here down.
    if [ -s "$diff_f" ]; then
        # A CONTRADICTION, AND IT IS OURS AND NOT THE TREE'S. `go mod tidy
        # -diff` documents a non-zero status when the files would change, so a
        # clean exit beside a diff means this script's reading of the tool is
        # wrong -- as govulncheck-verdict.sh's `vulnerable-without-a-heading`
        # arm is for the mirror case. Reporting the diff as a finding would be
        # acting on a contract we have just been shown we do not understand.
        echo "CLAUSE zero-with-a-diff"
        echo "tidy exited 0 and wrote a diff anyway, which its documented contract says cannot happen. One of the two is wrong and neither can be acted on. STEP DEFECT: re-read the tool before trusting this arm again."
        cat "$diff_f"
        return 2
    fi

    # A PASS OVER AN EMPTY GRAPH IS NOT A PASS. tidy exits 0 on a module with
    # no requirements at all, so the known positive is that there was something
    # to be accurate about -- the vacuity guard `rumor validate` carries for a
    # project that declares no rule, in the one place here that can be vacuous.
    if [ ! -s "$gosum" ]; then
        echo "CLAUSE nothing-to-be-accurate-about"
        echo "tidy exited 0 with an empty diff, and $gosum is empty or absent: there were no module sums to be accurate about, so this arm proved nothing. BLOCKED, not passed."
        return 1
    fi

    echo "CLAUSE tidy"
    echo "tidy exited 0, wrote no diff, and $gosum holds $(grep -c . "$gosum") line(s): go.mod and go.sum are what tidying would write."
    return 0
}

self_test() {
    tmp=$(mktemp -d) || { echo "self-test: cannot make a scratch directory" >&2; return 1; }
    pass=0
    fail=0

    # case <name> <expected-clause> <expected-rc> <rc-in> <diff-body> <err-body>
    # <gosum-body>. The three bodies take __ABSENT__ for a file that is not
    # there and __EMPTY__ for one that exists and is empty, which are different
    # states to this script and were conflated by its first cut.
    case_run() {
        name=$1; want_clause=$2; want_rc=$3; rc_in=$4
        d="$tmp/$name.diff"; e="$tmp/$name.err"; g="$tmp/$name.sum"
        write() {
            path=$1; body=$2
            case "$body" in
            __ABSENT__) rm -f "$path" ;;
            __EMPTY__) : > "$path" ;;
            *) printf '%s\n' "$body" > "$path" ;;
            esac
        }
        write "$d" "$5"
        write "$e" "$6"
        write "$g" "$7"
        out=$(classify "$d" "$e" "$rc_in" "$g")
        got_rc=$?
        got_clause=$(printf '%s\n' "$out" | sed -n 's/^CLAUSE //p' | head -1)
        if [ "$got_clause" = "$want_clause" ] && [ "$got_rc" -eq "$want_rc" ]; then
            echo "  ok   $name -> $got_clause, rc $got_rc"
            pass=$((pass + 1))
            printf '%s\n' "$got_clause" >> "$tmp/clauses.seen"
        else
            echo "  FAIL $name -> clause '$got_clause' rc $got_rc; wanted clause '$want_clause' rc $want_rc"
            fail=$((fail + 1))
        fi
    }

    # THE STRINGS BELOW ARE REAL. The diff is what `go mod tidy -diff` printed
    # over this tree with `github.com/fortytw2/leaktest v1.3.0` added to the
    # require block; the proxy error is what it printed with GOPROXY=off and an
    # empty module cache. Both measured 2026-10-18.
    real_diff='diff current/go.mod tidy/go.mod
--- current/go.mod
+++ tidy/go.mod
@@ -9,7 +9,6 @@
 go 1.26.0

 require (
-	github.com/fortytw2/leaktest v1.3.0
 	github.com/BurntSushi/toml v1.6.0
 	github.com/open-policy-agent/opa v1.21.2
 )'
    real_proxy_off='go: downloading github.com/open-policy-agent/opa v1.21.2
go: forge.axonibyte.com/axonibyte/rumor/internal/attaint imports
	github.com/open-policy-agent/opa/v1/rego: module lookup disabled by GOPROXY=off'
    a_sum='github.com/BurntSushi/toml v1.6.0 h1:gQ7p4W3Y+6dS8vOBBqBkqVRDRgY2yWWL5RXdgQhGZ7c='

    case_run inaccurate inaccurate 3 1 "$real_diff" __EMPTY__ "$a_sum"
    case_run proxy_off could-not-read-the-graph 1 1 __EMPTY__ "$real_proxy_off" "$a_sum"

    # NON-ZERO WITH NOTHING TO SHOW FOR IT, which neither of the two above is:
    # a tool that failed and explained nothing cannot be read either way.
    case_run silent_failure nonzero-and-silent 1 1 __EMPTY__ __EMPTY__ "$a_sum"

    case_run timed_out timed-out 1 124 __EMPTY__ "$real_proxy_off" "$a_sum"
    case_run tidy tidy 0 0 __EMPTY__ __EMPTY__ "$a_sum"

    # rc 0 WITH NOTHING TO BE ACCURATE ABOUT. This is the arm that makes the
    # tidy verdict mean something: a module with no requirements also exits 0.
    case_run empty_graph nothing-to-be-accurate-about 1 0 __EMPTY__ __EMPTY__ __EMPTY__
    case_run absent_sum nothing-to-be-accurate-about 1 0 __EMPTY__ __EMPTY__ __ABSENT__

    # THE TOOL CONTRADICTING ITS OWN CONTRACT.
    case_run zero_with_diff zero-with-a-diff 2 0 "$real_diff" __EMPTY__ "$a_sum"

    case_run no_capture no-capture 1 1 __ABSENT__ __EMPTY__ "$a_sum"
    case_run no_stderr no-stderr-capture 2 1 __EMPTY__ __ABSENT__ "$a_sum"

    echo "  EXAMINED $((pass + fail)) case(s): $pass ok, $fail failed"
    if [ "$fail" -ne 0 ]; then
        rm -rf "$tmp"
        return 1
    fi
    # Every clause this script can print must be reached by a case above, and
    # DISTINCT clauses rather than passing cases: two cases reach
    # `nothing-to-be-accurate-about`, for an empty go.sum and an absent one,
    # and both are worth keeping. govulncheck-verdict.sh's arm caught the same
    # thing about itself at 10 against 9.
    declared=$(grep -cE '^[[:space:]]*echo "CLAUSE ' "$0")
    reached=$(sort -u "$tmp/clauses.seen" 2>/dev/null | grep -c . || echo 0)
    if [ "$declared" -ne "$reached" ]; then
        echo "  FAIL $declared clause(s) in this script, $reached reached by a case: every clause needs one"
        sort -u "$tmp/clauses.seen" 2>/dev/null | sed 's/^/    reached: /'
        rm -rf "$tmp"
        return 1
    fi
    echo "  PROOF all $declared clauses answered as expected over $pass case(s), each read off the clause name"
    rm -rf "$tmp"
    return 0
}

if [ "${1-}" = "--self-test" ]; then
    self_test
    exit $?
fi

if [ "$#" -ne 4 ]; then
    usage
    exit 2
fi

classify "$1" "$2" "$3" "$4"
exit $?
