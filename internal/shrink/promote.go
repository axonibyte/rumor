package shrink

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Promote appends a seed to the project's seeds file, in the format
// docs/contract.md fixes under "The seeds file": one non-negative integer per
// line, optionally followed by `#` and a comment naming the defect it found.
// It reports whether the seed was added.
//
// THE FORMAT IS R4's AND R5 MAY NOT CONTRADICT IT, which the document says in
// those words: "R5 may EXTEND it and may not contradict it". So this writes the
// minimal form and nothing else -- a richer promotion record belongs beside the
// lock file rather than inside a line arm 10b has to keep reading.
//
// IT APPENDS AND NEVER REWRITES. A seeds file's whole content may be a header
// explaining why it is empty -- examples/twin's is exactly that, "a file whose
// only content is why it is empty" -- and a promoter that rewrote the file
// would delete the one thing it had to say.
//
// A MISSING TRAILING NEWLINE IS THE HAZARD WORTH THE CODE. Appending "42\n" to
// a file whose last line has no newline produces `17 # why42`, which is one
// corrupted line rather than two good ones, and arm 10b would then refuse a
// file this function wrote. So the newline is added first when it is missing.
//
// WHOSE READING OF THE GRAMMAR IS AUTHORITATIVE, said plainly because this file
// holds a second one: internal/validate's arm 10b is the reader, and the
// pairing test runs that ARM over a file this function wrote rather than
// re-checking the format by eye. The parse below is used only to decide whether
// a seed is already present, so a disagreement with the arm costs a duplicate
// line -- which the arm accepts -- and never an unreadable one.
func Promote(path string, seed uint64, why string) (bool, error) {
	if path == "" {
		return false, fmt.Errorf("shrink: no seeds file named: platform.toml's seeds_file is where " +
			"a promotion goes, and writing one somewhere else would promote a seed no run replays")
	}
	if strings.ContainsAny(why, "\n\r") {
		return false, fmt.Errorf("shrink: the promotion comment holds a newline, which would write "+
			"a second line that is not a seed and that arm 10b refuses by name: %q", why)
	}

	raw, err := os.ReadFile(path)
	switch {
	case err == nil:
	case os.IsNotExist(err):
		raw = nil
	default:
		return false, fmt.Errorf("shrink: reading the seeds file %s: %w", path, err)
	}

	if seedPresent(string(raw), seed) {
		return false, nil
	}

	line := strconv.FormatUint(seed, 10)
	if w := strings.TrimSpace(why); w != "" {
		line += "  # " + w
	}

	// THE CREATE IS DELIBERATE: a promotion into a project whose seeds file is
	// absent should land, not fail. arm 10a is what tells a project the file is
	// missing; a run that found a real failure must not lose it to that.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return false, fmt.Errorf("shrink: opening the seeds file %s to append: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	out := line + "\n"
	if len(raw) > 0 && !strings.HasSuffix(string(raw), "\n") {
		out = "\n" + out
	}
	if _, err := f.WriteString(out); err != nil {
		return false, fmt.Errorf("shrink: appending seed %d to %s: %w", seed, path, err)
	}
	return true, nil
}

// seedPresent reads the file the way arm 10b does, for the single purpose of
// not promoting a seed twice.
func seedPresent(content string, seed uint64) bool {
	want := strconv.FormatUint(seed, 10)
	for _, line := range strings.Split(content, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		if h := strings.Index(t, "#"); h >= 0 {
			t = strings.TrimSpace(t[:h])
		}
		if t == want {
			return true
		}
	}
	return false
}
