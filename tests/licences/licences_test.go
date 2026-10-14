// Package licences_test is the licence gate over the dependency surface.
//
// G3, issue 34. It replaces `cargo deny check licenses`, and it is a Go test
// rather than a tool so that `go test ./...` runs it: ci.yml already does that,
// and adding another CI dependency to gate licences would be a new unmeasured
// thing in the release path.
//
// THE ORACLE IS `go list -deps -test ./...`, ruled at go-port/5 and measured on
// a one-dependency fixture at go-port/4. The two alternatives are both wrong:
//
//	go list -m all        the MODULE GRAPH, which over-counts. It listed 124
//	                      modules here against 24 that are actually built, and
//	                      it includes requirements of requirements' own tests --
//	                      code that never ships. An allowlist over it judges
//	                      licences of software this project does not distribute,
//	                      and that noise is what someone later silences.
//	go list -deps ./...   BLIND to a test-only import. On the fixture it reported
//	                      zero for a dependency imported from a _test.go, and
//	                      G2's spike imported OPA only from tests -- so it would
//	                      have read a dependency-free tree and passed on an
//	                      absence.
package licences_test

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// allowed is R0.2's licence allowlist, carried from deny.toml.
//
// DUPLICATED, AND THE DUPLICATION IS CHECKED rather than assumed: while the
// Rust tree still exists, TestTheAllowlistsAgree parses deny.toml and asserts
// these two lists are the same set. Two readers of one policy that nobody
// compares are two policies, which is the defect the VERSION readers had this
// morning until they were driven against each other. G4 removes deny.toml and
// that test then names itself retired; this list becomes the only source.
var allowed = []string{
	"Apache-2.0",
	"BSD-2-Clause",
	"BSD-3-Clause",
	"ISC",
	"MIT",
	"MPL-2.0",
	"UPL-1.0",
	"Unicode-3.0",
}

// classify names the licence families in one licence file.
//
// FAMILIES ARE NOT INDEPENDENT MARKERS, and that is the correction this
// function exists for. The first version used one phrase per licence and
// reported five BSD-3-Clause modules as carrying BOTH BSD-2-Clause and
// BSD-3-Clause -- because BSD-3-Clause's text CONTAINS BSD-2-Clause's: the
// no-endorsement clause is additive, not alternative. The gate refused them,
// correctly on its own terms and wrongly about the licences.
//
// Worse, my shell version of this classifier got those five RIGHT, by matching
// Apache first and stopping. It was right by ordering rather than by
// discriminating, which is the same thing as being wrong and lucky.
//
// So the containment is encoded: BSD-2-Clause is the redistribution clause
// WITHOUT the no-endorsement clause, and MIT is its own grant without Apache's
// header. Multiplicity now means genuinely different families -- MIT beside
// Apache-2.0 -- and not one family read twice.
func classify(text string) []string {
	has := func(sub string) bool { return strings.Contains(text, sub) }
	var ids []string
	apache := has("Apache License") && has("Version 2.0")
	if apache {
		ids = append(ids, "Apache-2.0")
	}
	if has("Mozilla Public License") {
		ids = append(ids, "MPL-2.0")
	}
	if has("Permission is hereby granted, free of charge") {
		ids = append(ids, "MIT")
	}
	if has("Permission to use, copy, modify, and/or distribute this software") {
		ids = append(ids, "ISC")
	}
	if has("Redistribution and use in source and binary forms") {
		if has("Neither the name of") || has("neither the name of") {
			ids = append(ids, "BSD-3-Clause")
		} else {
			ids = append(ids, "BSD-2-Clause")
		}
	}
	// THE COPYLEFT FAMILIES, WHICH ARE HERE TO BE REFUSED BY NAME. Without
	// them every id this function can produce is on the allowlist, so the
	// "outside the allowlist" clause below was UNREACHABLE: a GPL dependency
	// would have been refused as "unclassified", which reads like a gap in the
	// classifier rather than a licence the project cannot ship. A gate should
	// name what it found.
	//
	// THE SAME CONTAINMENT TRAP AS BSD, so the same treatment: Affero's and
	// Lesser's titles both contain "GENERAL PUBLIC LICENSE", and LGPL-3.0's
	// text additionally incorporates the GPL by reference. The discriminator
	// is therefore the most specific title present, not the presence of the
	// general phrase.
	switch {
	case has("GNU AFFERO GENERAL PUBLIC LICENSE"):
		ids = append(ids, "AGPL-3.0")
	case has("GNU LESSER GENERAL PUBLIC LICENSE"):
		ids = append(ids, "LGPL")
	case has("GNU GENERAL PUBLIC LICENSE"):
		if has("Version 2") {
			ids = append(ids, "GPL-2.0")
		} else {
			ids = append(ids, "GPL-3.0")
		}
	}
	sort.Strings(ids)
	return ids
}

// dualApproved names modules whose licence file carries MORE THAN ONE licence,
// with the decision recorded here by a human who read it.
//
// WHY MULTIPLICITY IS NOT CLASSIFIED AUTOMATICALLY, and this is the defect that
// produced this map: a first-match classifier reported `go.yaml.in/yaml/v3` as
// Apache-2.0, and that file's second line reads "This project is covered by two
// different licenses: MIT and Apache." Both are allowed, so the verdict was
// right by luck -- a module under "MIT OR GPL-3.0" would have been reported MIT
// and admitted. So a module with several markers is REFUSED unless it appears
// here, which forces the reading to happen once and be written down.
var dualApproved = map[string]string{
	"go.yaml.in/yaml/v3": "MIT and Apache-2.0. Its LICENSE says so in its second " +
		"line: the files ported from libyaml's C are MIT, the rest Apache-2.0. " +
		"Both are in the allowlist, so either reading is permitted and no choice " +
		"between them has to be made.",
}

func goBin(t *testing.T) string {
	t.Helper()
	for _, c := range []string{"go", "go126", "go125"} {
		if p, err := exec.LookPath(c); err == nil {
			return p
		}
	}
	t.Fatal("no go, go126 or go125 on PATH: this gate cannot enumerate anything, which is UNMEASURED and not a pass")
	return ""
}

// surface returns the third-party modules actually built, tests included.
func surface(t *testing.T) []string {
	t.Helper()
	cmd := exec.Command(goBin(t), "list", "-deps", "-test",
		"-f", "{{if .Module}}{{.Module.Path}}{{end}}", "./...")
	cmd.Dir = "../.."
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps -test failed, so the surface is unmeasured: %v", err)
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		m := strings.TrimSpace(line)
		if m == "" || strings.HasPrefix(m, "forge.axonibyte.com/") {
			continue // the module itself, which carries the project's own licence
		}
		seen[m] = true
	}
	mods := make([]string, 0, len(seen))
	for m := range seen {
		mods = append(mods, m)
	}
	sort.Strings(mods)
	return mods
}

func licenceFile(t *testing.T, module string) (string, bool) {
	t.Helper()
	cmd := exec.Command(goBin(t), "list", "-m", "-f", "{{.Dir}}", module)
	cmd.Dir = "../.."
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	dir := strings.TrimSpace(string(out))
	if dir == "" {
		return "", false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	want := regexp.MustCompile(`(?i)^(LICENSE|LICENCE|COPYING)`)
	for _, e := range entries {
		if want.MatchString(e.Name()) {
			body, err := os.ReadFile(dir + "/" + e.Name())
			if err != nil {
				return "", false
			}
			return string(body), true
		}
	}
	return "", false
}

func isAllowed(id string) bool {
	for _, a := range allowed {
		if a == id {
			return true
		}
	}
	return false
}

func TestTheSurfaceIsNotEmpty(t *testing.T) {
	// A gate over nothing passes. The count is asserted before any verdict:
	// if the enumeration broke, this fails rather than reporting a clean
	// licence surface over an empty list.
	mods := surface(t)
	if len(mods) < 10 {
		t.Fatalf("the built surface is %d module(s), which is too few to be this project's real dependency set: %v", len(mods), mods)
	}
	t.Logf("PROOF %d third-party module(s) are actually built, tests included", len(mods))
}

// The gate's clauses, named. A control must read the verdict off the CLAUSE
// THAT ANSWERED and not off a boolean: "refused" is reached four different ways
// here, and a control satisfied by any refusal would pass against a gate that
// refuses everything (cal's rule, and the lesson of the Rust gate's Test 3).
const (
	clauseNoFile       = "no-licence-file"
	clauseUnclassified = "unclassified"
	clauseSeveral      = "several-licences"
	clauseDualApproved = "several-but-approved"
	clauseOutside      = "outside-allowlist"
	clauseAllowed      = "allowed"
)

type verdict struct {
	clause string
	ids    []string
	detail string
}

func (v verdict) passes() bool {
	return v.clause == clauseAllowed || v.clause == clauseDualApproved
}

// judge is the whole decision for one module, extracted from the loop below so
// that the negative controls drive THE SAME CLAUSES the real surface does. A
// control against a re-implementation of this logic would agree with itself.
func judge(module, text string, haveFile bool) verdict {
	if !haveFile {
		return verdict{clause: clauseNoFile,
			detail: "no LICENSE, LICENCE or COPYING file, so its terms are unknown and cannot be allowed"}
	}
	ids := classify(text)
	switch {
	case len(ids) == 0:
		return verdict{clause: clauseUnclassified,
			detail: "its licence file matches no marker this gate knows, so it is UNCLASSIFIED rather than allowed. Read it and add a marker, or record it as approved"}
	case len(ids) > 1:
		if why, ok := dualApproved[module]; ok {
			return verdict{clause: clauseDualApproved, ids: ids,
				detail: fmt.Sprintf("carries %s, approved: %s", strings.Join(ids, " and "), why)}
		}
		return verdict{clause: clauseSeveral, ids: ids,
			detail: fmt.Sprintf("its licence file carries SEVERAL licences (%s) and this gate will not pick one for you -- a module under one allowed and one forbidden licence would otherwise pass on whichever matched first. Read it and record the decision", strings.Join(ids, ", "))}
	default:
		if !isAllowed(ids[0]) {
			return verdict{clause: clauseOutside, ids: ids,
				detail: fmt.Sprintf("%s is outside R0.2's allowlist", ids[0])}
		}
		return verdict{clause: clauseAllowed, ids: ids}
	}
}

func TestEveryBuiltModuleCarriesAnAllowedLicence(t *testing.T) {
	mods := surface(t)
	classified := 0
	for _, m := range mods {
		text, ok := licenceFile(t, m)
		v := judge(m, text, ok)
		if !v.passes() {
			t.Errorf("%s [%s]: %s", m, v.clause, v.detail)
			continue
		}
		classified++
		if v.clause == clauseDualApproved {
			t.Logf("PROOF %s %s", m, v.detail)
		}
	}
	if classified != len(mods) {
		t.Errorf("classified %d of %d module(s); an unclassified module is not an allowed one", classified, len(mods))
	}
	t.Logf("PROOF %d of %d built module(s) carry a licence inside R0.2's allowlist", classified, len(mods))
}

// TestTheAllowlistsAgree compares this file's list against deny.toml's while
// the Rust tree still exists. G4 removes deny.toml, and this test then reports
// itself retired rather than passing silently over a file that is gone.
// TestTheAllowlistsAgree compares this file's list against THE DOCUMENT's.
//
// IT USED TO READ deny.toml AND THEN SKIP. G4 removed that file with the Rust
// tree, leaving the list in one place -- here -- with nothing able to notice it
// drifting from R0.2's decision. I recorded the loss and called it deliberate;
// ratchet's review (thread:go-port/54) made the better point, that recording a
// loss is not keeping the property, and the fix is a second source that cannot
// be swept: the decision's home is docs/design.md.
//
// SO THIS ARM IS LIVE AGAIN RATHER THAN SKIPPED, and that matters for the CI
// step too: asserting the SKIP by name -- which is what the review first asked
// for -- would now assert something that cannot happen. The step asserts the
// agreement instead, and refuses any skip at all.
func TestTheAllowlistsAgree(t *testing.T) {
	body, err := os.ReadFile("../../docs/design.md")
	if err != nil {
		t.Fatalf("docs/design.md is the allowlist's home and it is unreadable: %v. This is a FAILURE and not a skip -- a comparison that cannot run proves nothing, and the last version of this test skipped in exactly that situation", err)
	}
	const begin, end = "ALLOWLIST BEGIN", "ALLOWLIST END"
	text := string(body)
	i := strings.Index(text, begin)
	j := strings.Index(text, end)
	if i < 0 || j < 0 || j < i {
		t.Fatalf("docs/design.md carries no %q..%q block, so the second source is gone again", begin, end)
	}
	var theirs []string
	for _, line := range strings.Split(text[i+len(begin):j], "\n") {
		if id := strings.TrimSpace(line); id != "" {
			theirs = append(theirs, id)
		}
	}
	if len(theirs) == 0 {
		t.Fatal("the allowlist block in docs/design.md is empty, so this comparison proved nothing -- which is worse than a mismatch because it reads as agreement")
	}
	sort.Strings(theirs)
	mine := append([]string(nil), allowed...)
	sort.Strings(mine)
	if strings.Join(theirs, ",") != strings.Join(mine, ",") {
		t.Fatalf("the document and this file disagree about R0.2's allowlist, so one of them is not the project's policy:\n  docs/design.md: %v\n  this file:      %v", theirs, mine)
	}
	t.Logf("PROOF the document and this file name the same %d licence(s)", len(mine))
}

// TestClassifyTellsTheFamiliesApart drives classify directly, because the
// allowlist arm above CANNOT.
//
// MEASURED, AND IT IS WHY THIS TEST EXISTS: removing the BSD-3 discriminator --
// so every BSD file reads as BSD-2-Clause -- left the suite GREEN, because both
// ids are in the allowlist and the verdict is the same either way. Two arms that
// always agree cannot be told apart by any fixture, which is the release gate's
// own lesson arriving here. So the discriminator is driven on its own.
//
// The snippets are WRITTEN OUT rather than read from the module cache: a test
// whose fixtures come from the corpus it classifies would agree with that corpus
// whatever either said. They are the clauses that matter, not whole licences.
func TestClassifyTellsTheFamiliesApart(t *testing.T) {
	const redistribution = "Redistribution and use in source and binary forms, " +
		"with or without modification, are permitted provided that the following " +
		"conditions are met:"
	const noEndorsement = "Neither the name of the copyright holder nor the names " +
		"of its contributors may be used to endorse or promote products"
	const mitGrant = "Permission is hereby granted, free of charge, to any person " +
		"obtaining a copy of this software"
	const apacheHeader = "Apache License\nVersion 2.0, January 2004"

	for _, c := range []struct {
		name string
		text string
		want []string
	}{
		// The pair the surviving mutant could not distinguish. They differ in
		// exactly one clause, which is the whole point.
		{"bsd-2 is the redistribution clause alone", redistribution, []string{"BSD-2-Clause"}},
		{"bsd-3 adds no-endorsement", redistribution + "\n" + noEndorsement, []string{"BSD-3-Clause"}},
		{"mit alone", mitGrant, []string{"MIT"}},
		{"apache alone", apacheHeader, []string{"Apache-2.0"}},
		// Genuine multiplicity, which must still be reported as two.
		{"mit beside apache", mitGrant + "\n" + apacheHeader, []string{"Apache-2.0", "MIT"}},
		{"nothing recognised", "All rights reserved. Ask us.", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := classify(c.text)
			if strings.Join(got, ",") != strings.Join(c.want, ",") {
				t.Fatalf("classify gave %v, wanted %v", got, c.want)
			}
		})
	}
}

// TestTheGateRefuses is this gate's negative control.
//
// WHY IT EXISTS, from the Rust gate it replaces: `cargo deny check licenses`
// passed from its very first run having examined no dependency licence at all,
// and a gate never seen to refuse is indistinguishable from one that is not
// wired up (docs/release.md, "Test 3"). The port must not lose that, and
// "24 of 24 allowed" is exactly the shape of output a gate that always says yes
// would produce.
//
// EACH CONTROL ASSERTS THE CLAUSE, NOT THE BOOLEAN. Four clauses refuse here,
// and a control that only checked "not allowed" would be satisfied by the
// always-refuse mutant -- the one that makes every other arm meaningless. So
// the want is a clause id, and the positive control sits in the same table so
// the table cannot pass by refusing everything.
func TestTheGateRefuses(t *testing.T) {
	const gpl3 = `                    GNU GENERAL PUBLIC LICENSE
                       Version 3, 29 June 2007

 Copyright (C) 2007 Free Software Foundation, Inc.
`
	const agpl = `                    GNU AFFERO GENERAL PUBLIC LICENSE
                       Version 3, 19 November 2007
`
	const mit = "Permission is hereby granted, free of charge, to any person obtaining a copy"
	const bsd2 = "Redistribution and use in source and binary forms, with or without modification, are permitted"

	cases := []struct {
		name     string
		module   string
		text     string
		haveFile bool
		want     string
		wantID   string // the id the refusal must NAME, where it names one
	}{
		{"a GPL-3.0 module is refused by name", "example.test/gpl", gpl3, true, clauseOutside, "GPL-3.0"},
		{"an AGPL module is refused by name", "example.test/agpl", agpl, true, clauseOutside, "AGPL-3.0"},
		{"a module with no licence file is refused", "example.test/bare", "", false, clauseNoFile, ""},
		{"an unreadable licence is not silently allowed", "example.test/odd", "All rights reserved.", true, clauseUnclassified, ""},
		{"two families are not picked between", "example.test/dual", mit + "\n" + gpl3, true, clauseSeveral, ""},
		{"an approved pair still passes", "go.yaml.in/yaml/v3", mit + "\nApache License\nVersion 2.0\n", true, clauseDualApproved, ""},
		{"THE POSITIVE CONTROL: a BSD-2 module passes", "example.test/ok", bsd2, true, clauseAllowed, "BSD-2-Clause"},
	}

	refusals, passes := 0, 0
	for _, c := range cases {
		v := judge(c.module, c.text, c.haveFile)
		if v.clause != c.want {
			t.Errorf("%s: the clause that answered was %q, wanted %q (detail: %s)", c.name, v.clause, c.want, v.detail)
			continue
		}
		if c.wantID != "" && (len(v.ids) == 0 || !containsID(v.ids, c.wantID)) {
			t.Errorf("%s: clause %s was right but it named %v, not %s -- a refusal that cannot name the licence sends the reader to the classifier", c.name, v.clause, v.ids, c.wantID)
			continue
		}
		if v.passes() {
			passes++
		} else {
			refusals++
		}
	}
	// BOTH DIRECTIONS, COUNTED. All-refuse and all-pass are the two mutants
	// this table exists to kill, and either would satisfy a one-sided count.
	if refusals != 5 || passes != 2 {
		t.Errorf("drove %d refusal(s) and %d pass(es), expected 5 and 2: a table that only refuses cannot tell a discriminating gate from a loud one", refusals, passes)
	}
	t.Logf("PROOF the gate was seen to refuse via %d distinct clause(s) and to pass %d control(s)", refusals, passes)
}

func containsID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
