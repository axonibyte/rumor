package shrink

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"forge.axonibyte.com/axonibyte/rumor/internal/platform"
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

	// A PRESENT SEED WHOSE COMMENT DISAGREES WITH THE FINDING IS A REFUSAL, not
	// a quiet skip (fable, coop r6/15, on ratchet's write-once point).
	//
	// THIS RETURNED EARLY ON ANY PRESENT SEED. seeds.lock is the one artifact
	// of a shrink that gets checked in, and its comment names the defect the
	// seed caught; a seed promoted for a plain `bug` that later produces a
	// `stale` would keep a comment that was true when written, in the file a
	// later run replays from, and nothing would say so. A disagreement between
	// the lock and the run is a thing to SEE.
	//
	// AN ABSENT COMMENT IS NOT A DISAGREEMENT, and nor is an empty `why`:
	// refusing on those would refuse every promotion into a lock whose lines
	// carry no reason, which arm 10b accepts and the twin's own file is. The
	// refusal is for two reasons that are both present and differ.
	if have, present := seedComment(string(raw), seed); present {
		want := strings.TrimSpace(why)
		if have != "" && want != "" && have != want {
			return false, fmt.Errorf("shrink: %s already carries seed %d with the comment %q, "+
				"and this run would record %q: a promoted seed's comment names the defect it "+
				"caught, so the two disagreeing means either the lock is stale or this is a "+
				"different finding under the same seed -- refusing rather than leaving the "+
				"older reason standing in the file a later run replays from",
				path, seed, have, want)
		}
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

// seedComment reads the file the way arm 10b does and answers two things about
// a seed: whether it is already there, and the comment beside it if so.
//
// ONE PASS AND ONE FUNCTION, replacing seedPresent, which answered only the
// first half. A caller that asked the two separately could be told the seed is
// present and then read a comment from a file that had changed in between; and
// leaving both functions would have left a reader to work out which one to
// reach for.
func seedComment(content string, seed uint64) (string, bool) {
	want := strconv.FormatUint(seed, 10)
	for _, line := range strings.Split(content, "\n") {
		// THE PARSE IS platform's, which is what makes this correct for a line
		// carrying a TOKEN. It compared the whole non-comment text to the
		// digits, so `12 profile=settling` read as "seed 12 is not here" and
		// this function's caller would have promoted it a second time -- a
		// duplicate line in the one artifact that says what a regression suite
		// covers (R6.3).
		parsed, isSeed, err := platform.ParseSeedLine(line)
		if !isSeed || err != nil {
			continue
		}
		if parsed.Seed == want {
			return parsed.Comment, true
		}
	}
	return "", false
}
