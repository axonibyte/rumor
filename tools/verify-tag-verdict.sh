#!/bin/sh
# What `git verify-tag` actually said, classified into the three answers a
# release pipeline can act on.
#
# WHY THIS IS A FILE AND NOT FOUR LINES OF YAML (issue 39, v0.1.5). The logic
# lived inline in tag-pipeline.yml and was wrong in two directions at once, and
# neither could be driven: a workflow step is only exercised by pushing a tag.
# One implementation, called by the workflow, with a battery over the real
# strings this project has actually seen.
#
# THE THREE ANSWERS, because "non-zero" is not one of them:
#   0  VERIFIED          the signature checked out against this runner's store
#   1  REFUSED           a verdict ON THE TAG -- do not publish
#   2  PROVISIONING      the signature could not be checked HERE; not a verdict
#                        on the tag, and NOT a pass either
#
# DEFECT 1, which cost run 162 (v0.1.4, task 185). The inline discriminator
# looked for `allowed_signers` among other words. Git says:
#
#   error: gpg.ssh.allowedSignersFile needs to be configured and exist for
#   ssh signature verification
#
# `allowed_signers` does not occur in that sentence -- the file's name does,
# in camel case, as a config KEY. So an unconfigured runner fell through to
# "FAILED verification, which is a verdict on the tag", and the pipeline
# accused cal's correctly-signed tag of being bad. Same shape as the gate's
# `docs-stale` arm: a filter that does not match its own tool's words.
#
# DEFECT 2, which was unreachable until defect 1 was fixed, and is quill's
# finding (rumor/448, issue 25 comment 122). When the store IS configured and
# the signer is NOT on the list, git prints BOTH:
#
#   Good "git" signature with ED25519 key SHA256:...
#   No principal matched.
#
# The old alternation matched `no principal` and reported PROVISIONING -- it
# granted what it should refuse, which is the one direction a release gate must
# never fail in. A valid signature by an untrusted signer is a VERDICT. The
# discriminator is the `Good ... signature` line: trust material was read.
#
# So the clauses are ordered, and each one is NAMED on stdout, because a
# composite boolean is satisfied by its weakest clause and a control that reads
# only the exit code cannot tell which one answered.

set -u

usage() {
    echo "usage: verify-tag-verdict.sh <verify-log> <git-verify-tag-rc>" >&2
    echo "       verify-tag-verdict.sh --self-test" >&2
}

# classify <logfile> <rc> -- prints "CLAUSE <id>" then a human line; returns
# 0 verified, 1 refused, 2 provisioning.
classify() {
    log=$1
    rc=$2

    if [ ! -f "$log" ]; then
        echo "CLAUSE no-log"
        echo "git verify-tag wrote no log at $log, so nothing was read and no verdict exists. Publishing nothing."
        return 2
    fi

    if [ "$rc" -eq 0 ]; then
        echo "CLAUSE verified"
        echo "the signature verifies against this runner's trust store"
        return 0
    fi

    # DEFECT 2's clause, and it must come FIRST: a log carrying a good
    # signature line has been read against the store, so whatever follows is
    # about the SIGNER, not about this box.
    if grep -qE 'Good .*signature' "$log" 2>/dev/null; then
        if grep -qiE 'no principal matched|no principal' "$log" 2>/dev/null; then
            echo "CLAUSE untrusted-signer"
            echo "the signature is cryptographically GOOD and the signer is NOT on this runner's allowed-signers list. The store was read, so this is a VERDICT ON THE TAG and not a provisioning gap. Publishing nothing."
            return 1
        fi
        echo "CLAUSE good-but-nonzero"
        echo "git printed a good signature line and still exited $rc, which this classifier cannot explain. Treated as a verdict on the tag rather than waved through. Publishing nothing."
        return 1
    fi

    # DEFECT 1's clause: no signature was checked at all. The words are git's
    # and gpg's own, duplicated here deliberately -- a predicate that read its
    # vocabulary from the tool it judges would agree with it whatever it said.
    if grep -qiE 'allowedsignersfile|needs to be configured|no public key|can.t check signature|cannot find|unknown key|allowed_signers|no principal' "$log" 2>/dev/null; then
        echo "CLAUSE no-trust-store"
        echo "the signature is PRESENT and could NOT BE CHECKED on this runner: no trust material, or none configured. That is a PROVISIONING finding, not a verdict on the tag, and it is NOT a pass -- the epic's requirement is unmet until a verifier exists here."
        return 2
    fi

    echo "CLAUSE failed-verification"
    echo "the tag has a signature and it FAILED verification, which is a verdict on the tag. Publishing nothing."
    return 1
}

self_test() {
    tmp=$(mktemp -d) || { echo "could not make a scratch directory" >&2; exit 2; }
    trap 'rm -rf "$tmp"' EXIT
    pass=0
    fail=0
    examined=0

    # name | rc | want-clause | log body. Every body is a string this project
    # has really seen, with where it was seen.
    run_case() {
        name=$1; rc=$2; want=$3; body=$4
        printf '%s' "$body" > "$tmp/case.log"
        got=$(classify "$tmp/case.log" "$rc" | head -1 | sed 's/^CLAUSE //')
        examined=$((examined + 1))
        if [ "$got" = "$want" ]; then
            pass=$((pass + 1))
            echo "  ok   $name -> $got"
        else
            fail=$((fail + 1))
            echo "  FAIL $name -> $got, wanted $want"
        fi
    }

    # run 162 / task 185, v0.1.4: the string that produced the wrong verdict
    run_case "an unconfigured runner is PROVISIONING (run 162)" 1 no-trust-store \
"error: gpg.ssh.allowedSignersFile needs to be configured and exist for ssh signature verification
"
    # ibis rumor/446 + quill rumor/448: store read, signer not listed
    run_case "a good signature by an untrusted signer is a VERDICT (quill rumor/448)" 1 untrusted-signer \
'Good "git" signature with ED25519 key SHA256:aHxgEXAMPLEEXAMPLEEXAMPLE
No principal matched.
'
    # ibis rumor/446: the proof that the store works
    run_case "rc 0 is VERIFIED (ibis rumor/446)" 0 verified \
'Good "git" signature for cpower@axonibyte.com with ED25519 key SHA256:aHxgEXAMPLE
'
    # gpg's wording, kept because a pgp-signed tag is still legal here
    run_case "gpg with no public key is PROVISIONING" 1 no-trust-store \
"gpg: Signature made Tue Oct 14 18:00:00 2026 UTC
gpg: Can't check signature: No public key
"
    run_case "a genuinely bad signature is a VERDICT" 1 failed-verification \
"error: could not verify the tag 'v9.9.9'
"
    run_case "an empty log is a VERDICT, not a pass" 1 failed-verification ""
    run_case "a good line with an unexplained non-zero is a VERDICT" 1 good-but-nonzero \
'Good "git" signature for cpower@axonibyte.com with ED25519 key SHA256:aHxgEXAMPLE
some unexpected trailer
'
    # THE NEGATIVE CONTROL ON THE CONTROL: no log at all must not read as a
    # verdict about the tag.
    rm -f "$tmp/absent.log"
    got=$(classify "$tmp/absent.log" 1 | head -1 | sed 's/^CLAUSE //')
    examined=$((examined + 1))
    if [ "$got" = "no-log" ]; then
        pass=$((pass + 1)); echo "  ok   a missing log is no-log, not a verdict -> $got"
    else
        fail=$((fail + 1)); echo "  FAIL a missing log -> $got, wanted no-log"
    fi

    echo ""
    echo "  EXAMINED $examined case(s): $pass ok, $fail failed"
    # THE COUNT IS DECLARED, so a case that stops running cannot hide in a
    # self-consistent total (ibis main/352).
    if [ "$examined" -ne 8 ]; then
        echo "  the battery examined $examined cases and declares 8: one stopped running, or one was added without moving this number"
        return 1
    fi
    [ "$fail" -eq 0 ] || return 1
    echo "  PROOF all 8 clauses answered as expected, each read off the clause name"
    return 0
}

case "${1:-}" in
    --self-test) self_test ;;
    -h|--help) usage; exit 0 ;;
    "") usage; exit 2 ;;
    *)
        [ $# -eq 2 ] || { usage; exit 2; }
        classify "$1" "$2"
        ;;
esac
