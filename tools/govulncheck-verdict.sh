#!/bin/sh
# What govulncheck actually said, classified into the answers a CI job can act
# on.
#
# WHY THIS IS A FILE AND NOT FIVE LINES OF YAML (R2.3, issue 44). It is the
# same reason verify-tag-verdict.sh exists: the logic lived inline in ci.yml,
# was wrong, and could not be driven -- a workflow step is only exercised by
# pushing. One implementation, called by the workflow, with a battery over the
# real strings this project has seen.
#
# THE DEFECT, which run 192 made visible. The inline line read:
#
#   test "$rc" -eq 3 && { echo "govulncheck exited 3: it could not load the
#   vulnerability database. Route finding, not a vulnerable tree"; exit 3; }
#
# govulncheck's own source says the opposite. golang.org/x/vuln v1.9.0,
# internal/scan/errors.go:
#
#   errVulnerabilitiesFound = &exitCodeError{message: "vulnerabilities found",
#                                            code: 3}
#
# and cmd/govulncheck/doc.go: "Govulncheck exits successfully (exit code 0) if
# there are no vulnerabilities, and exits unsuccessfully if there are." So 3 is
# the ONE code that means the gate found what it exists to find, and the step
# announced a route problem. Run 192's log carries both lines, four apart:
#
#   Your code is affected by 9 vulnerabilities from the Go standard library.
#   govulncheck exited 3: it could not load the vulnerability database.
#
# The job failed either way, so the defect was the DIAGNOSIS and not the
# verdict -- which is the worse kind, because the reason is what gets acted on,
# and this one sent a reader to the network instead of to nine CVEs.
#
# THE ANSWERS, because "non-zero" is not one of them:
#   0  CLEAN           the scan ran and this tree's code reaches nothing
#   3  VULNERABLE      a verdict ON THE TREE (or on its toolchain) -- act on it
#   2  STEP DEFECT     govulncheck refused this step's own invocation
#   1  BLOCKED         the scan did not happen, or its result cannot be read;
#                      NOT a pass, and not a verdict on the tree either
#
# Every clause is NAMED on stdout, because a composite boolean is satisfied by
# its weakest clause and a control reading only the exit code cannot tell which
# one answered.

set -u

usage() {
    echo "usage: govulncheck-verdict.sh <govulncheck-log> <govulncheck-rc>" >&2
    echo "       govulncheck-verdict.sh --self-test" >&2
}

# classify <logfile> <rc> -- prints "CLAUSE <id>" then a human line; returns
# 0 clean, 3 vulnerable, 2 step defect, 1 blocked.
classify() {
    log=$1
    rc=$2

    if [ ! -f "$log" ]; then
        echo "CLAUSE no-log"
        echo "govulncheck wrote no log at $log, so nothing was scanned and no verdict exists. The gate is BLOCKED, not passed."
        return 1
    fi

    if [ "$rc" -eq 124 ]; then
        echo "CLAUSE timed-out"
        echo "govulncheck hit its timeout. rc 124 proves only that the time elapsed, so this blocks the vulnerability gate rather than failing it. RUNNER finding."
        return 1
    fi

    if [ "$rc" -eq 3 ]; then
        # The heading is grepped rather than assumed: rc 3 with no vulnerability
        # in the log is a contradiction, and reporting nine CVEs that are not
        # there would be as bad as reporting none that are.
        if grep -qE '^Vulnerability #' "$log"; then
            echo "CLAUSE vulnerabilities-found"
            echo "govulncheck exited 3, which is its code for vulnerabilities found, and the headings below are in its log. This is a verdict on the tree: a Standard library entry is fixed by the runner's toolchain, a Module entry by a bump."
            grep -E '^Vulnerability #|^[[:space:]]*(Standard library|Module:)|^[[:space:]]*(Found|Fixed) in:' "$log"
            return 3
        fi
        echo "CLAUSE vulnerable-without-a-heading"
        echo "govulncheck exited 3 -- its vulnerabilities-found code -- but its log carries no 'Vulnerability #' heading. One of the two is wrong and neither can be acted on, so this blocks the gate. Read the log whole."
        return 1
    fi

    if [ "$rc" -eq 2 ]; then
        echo "CLAUSE step-usage-error"
        echo "govulncheck exited 2, which it uses for a usage error and for no package matching the pattern. That is a defect in the step that invoked it, not a finding about the tree."
        return 2
    fi

    if [ "$rc" -eq 0 ]; then
        # A CLEAN EXIT IS NOT ENOUGH. The summary line is the value only an
        # execution produces; without it, rc 0 could be a scan that never ran.
        if grep -qiE 'affected by|no vulnerabilities|vulnerabilit' "$log"; then
            echo "CLAUSE clean"
            echo "govulncheck exited 0 and its own summary line is in the log, so the scan ran and this tree's code reaches nothing it knows about."
            return 0
        fi
        echo "CLAUSE clean-without-a-summary"
        echo "govulncheck exited 0 but printed no summary of its own, so nothing proves a scan happened. The gate is BLOCKED, not passed."
        return 1
    fi

    # Everything else is rc 1: the tool failed. WHICH failure matters, because
    # an unreachable vuln.go.dev is a route finding and a broken invocation is
    # not, and the two are told apart by the tool's own words.
    if grep -qiE 'vuln\.go\.dev|no such host|connection refused|i/o timeout|certificate|x509|TLS handshake|could not|database' "$log"; then
        echo "CLAUSE database-unreachable"
        echo "govulncheck exited $rc and its output names the vulnerability database or the route to it. RUNNER/route finding: the gate is BLOCKED and this says nothing about the tree."
        return 1
    fi
    echo "CLAUSE tool-failed"
    echo "govulncheck exited $rc and its output names no database or route problem, so the tool failed over a tree that was never scanned. BLOCKED, not passed."
    return 1
}

self_test() {
    tmp=$(mktemp -d) || { echo "self-test: cannot make a scratch directory" >&2; return 1; }
    pass=0
    fail=0

    # case <name> <expected-clause> <expected-rc> <rc-in> <log-contents>
    case_run() {
        name=$1; want_clause=$2; want_rc=$3; rc_in=$4; body=$5
        f="$tmp/$name.log"
        if [ "$body" = "__NOLOG__" ]; then
            f="$tmp/$name.absent"
            rm -f "$f"
        else
            printf '%s\n' "$body" > "$f"
        fi
        out=$(classify "$f" "$rc_in")
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

    # The strings below are REAL. The rc-3 body is run 192's own log, trimmed to
    # the lines this script reads; the rc-0 body is 5de30c8's.
    case_run vulnerable vulnerabilities-found 3 3 'Vulnerability #1: GO-2026-6617
    HTTP/2 server crash due to HPACK encoder race in net/http
  Standard library
    Found in: net/http@go1.26.8
    Fixed in: net/http@go1.26.9

Your code is affected by 9 vulnerabilities from the Go standard library.'

    case_run vulnerable_module vulnerabilities-found 3 3 'Vulnerability #1: GO-2025-0001
  Module: example.com/dep
    Found in: example.com/dep@v1.0.0
    Fixed in: example.com/dep@v1.0.1

Your code is affected by 1 vulnerability.'

    # THE CONTRADICTION: the tool says it found something and its log does not
    # say what. Reporting CVEs that are not there is as bad as missing ones.
    case_run three_without_heading vulnerable-without-a-heading 1 3 'Scanning your code and 42 packages across 7 dependent modules for known vulnerabilities...'

    case_run clean clean 0 0 'Scanning your code and 213 packages across 29 dependent modules for known vulnerabilities...

Your code is affected by 0 vulnerabilities.
This scan also found 1 vulnerability in packages you import and 12
vulnerabilities in modules you require, but your code does not appear to call
these vulnerabilities.'

    # rc 0 WITH NOTHING TO SHOW FOR IT. This is the arm that makes the clean
    # verdict mean something: a step that never ran also exits 0.
    case_run zero_without_summary clean-without-a-summary 1 0 'go: downloading golang.org/x/vuln v1.9.0'

    case_run usage step-usage-error 2 2 'govulncheck: no package patterns provided

To scan the current module, run: govulncheck ./...'

    case_run db_unreachable database-unreachable 1 1 'govulncheck: loading vulnerability database: Get "https://vuln.go.dev/index/db.json": dial tcp: lookup vuln.go.dev: no such host'

    case_run tool_failed tool-failed 1 1 'govulncheck: package forge.axonibyte.com/axonibyte/rumor/internal/engine: build constraints exclude all Go files'

    case_run timeout timed-out 1 124 'Scanning your code and 213 packages across 29 dependent modules for known vulnerabilities...'

    case_run missing_log no-log 1 3 '__NOLOG__'

    echo "  EXAMINED $((pass + fail)) case(s): $pass ok, $fail failed"
    if [ "$fail" -ne 0 ]; then
        rm -rf "$tmp"
        return 1
    fi
    # Every clause the script can print must be reached by a case above. A
    # clause no case exercises is one that could stop firing unnoticed.
    #
    # DISTINCT clauses, not passing cases. This arm's first run compared the
    # pass count against the clause count and failed at 10 against 9 -- two
    # cases reach `vulnerabilities-found`, one for a standard-library entry and
    # one for a module entry, and both are worth keeping. A count of cases was
    # the wrong measure of coverage, and the arm caught that about itself.
    declared=$(grep -cE '^[[:space:]]*echo "CLAUSE ' "$0")
    reached=$(sort -u "$tmp/clauses.seen" 2>/dev/null | grep -c . || echo 0)
    if [ "$declared" -ne "$reached" ]; then
        echo "  FAIL $declared clause(s) in this script, $reached reached by a case: every clause needs one"
        sort -u "$tmp/clauses.seen" 2>/dev/null | sed 's/^/    reached: /'
        return 1
    fi
    echo "  PROOF all $declared clauses answered as expected over $((pass)) case(s), each read off the clause name"
    rm -rf "$tmp"
    return 0
}

if [ "${1-}" = "--self-test" ]; then
    self_test
    exit $?
fi

if [ "$#" -ne 2 ]; then
    usage
    exit 2
fi

classify "$1" "$2"
exit $?
