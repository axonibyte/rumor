// Package coverage holds the doc coverage map and the check over it.
//
// docs/coverage.toml maps path globs to the documents that describe them.
// Check walks the workspace and fails on the first path no entry matches.
//
// Two deliberate choices, both about not hiding anything:
//
//   - The parser is STRICT. A line it does not recognise is an error, not a
//     skip. A lenient reader would turn one mistyped key into source that is
//     silently uncovered while the check still reports success.
//   - The walk skips only .git and target, and says so here rather than
//     growing a list of conveniences. Every other path has to be mapped.
//
// Ported from the Rust original at 20675b3 (crates/rumor-cli/src/coverage.rs)
// for the pivot to Go, G1/issue 32. The behaviour is carried unchanged,
// including every message a test asserts on; nothing here is a redesign.
package coverage

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Entry is one mapping from a path glob to the docs describing those paths.
type Entry struct {
	Path string
	Docs []string
}

// skipDirs are the directories the walk does not descend into. .git is not
// source and target is build output; nothing else is exempt.
//
// target is Cargo's output and stays in this list while the Rust tree does.
// G4 removes that tree; the entry goes with it, not before, because a stale
// target/ during G1 to G3 would otherwise bury the check in build artefacts.
var skipDirs = []string{".git", "target"}

// Parse reads the coverage map. Strict: an unrecognised line is an error
// naming its number, and an entry missing either key is an error naming the
// entry.
func Parse(text string) ([]Entry, error) {
	var entries []Entry
	var path *string
	var docs *[]string
	open := false

	flush := func(at int) error {
		if !open {
			return nil
		}
		if path == nil {
			return fmt.Errorf("entry ending at line %d has no `path` key", at)
		}
		p := *path
		path = nil
		if docs == nil {
			return fmt.Errorf("entry `%s` has no `docs` key", p)
		}
		d := *docs
		docs = nil
		if len(d) == 0 {
			return fmt.Errorf("entry `%s` lists no docs", p)
		}
		entries = append(entries, Entry{Path: p, Docs: d})
		open = false
		return nil
	}

	lines := splitLines(text)
	for i, raw := range lines {
		n := i + 1
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if line == "[[entry]]" {
			if err := flush(n); err != nil {
				return nil, err
			}
			open = true
			continue
		}
		if !open {
			return nil, fmt.Errorf("line %d: `%s` is outside any [[entry]]", n, line)
		}
		if rest, ok := cutPrefix(line, "path = "); ok {
			if path != nil {
				return nil, fmt.Errorf("line %d: a second `path` in one entry", n)
			}
			v, ok := unquote(rest)
			if !ok {
				return nil, fmt.Errorf("line %d: `path` value is not a quoted string", n)
			}
			path = &v
			continue
		}
		if rest, ok := cutPrefix(line, "docs = "); ok {
			if docs != nil {
				return nil, fmt.Errorf("line %d: a second `docs` in one entry", n)
			}
			v, ok := unquoteList(rest)
			if !ok {
				return nil, fmt.Errorf("line %d: `docs` value is not a list of quoted strings", n)
			}
			docs = &v
			continue
		}
		return nil, fmt.Errorf("line %d: unrecognised `%s`", n, line)
	}
	if err := flush(len(lines)); err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("the coverage map holds no entries")
	}
	return entries, nil
}

// splitLines splits on \n the way Rust's str::lines does: no trailing empty
// element for a final newline, and a \r before the \n is dropped. Written out
// rather than reached for, because strings.Split would add a phantom last line
// and shift every line number the parser reports.
func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	out := strings.Split(text, "\n")
	if len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	for i, l := range out {
		out[i] = strings.TrimSuffix(l, "\r")
	}
	return out
}

func cutPrefix(s, prefix string) (string, bool) {
	if strings.HasPrefix(s, prefix) {
		return s[len(prefix):], true
	}
	return "", false
}

func unquote(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if len(s) < 2 || !strings.HasPrefix(s, `"`) || !strings.HasSuffix(s, `"`) {
		return "", false
	}
	inner := s[1 : len(s)-1]
	if strings.Contains(inner, `"`) {
		return "", false
	}
	return inner, true
}

func unquoteList(s string) ([]string, bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "[") || !strings.HasSuffix(s, "]") {
		return nil, false
	}
	inner := s[1 : len(s)-1]
	out := []string{}
	for _, part := range strings.Split(inner, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		v, ok := unquote(part)
		if !ok {
			return nil, false
		}
		out = append(out, v)
	}
	return out, true
}

// GlobMatch matches one path against one glob. * matches within a single
// segment and ** matches zero or more whole segments.
func GlobMatch(pattern, path string) bool {
	return segMatch(strings.Split(pattern, "/"), strings.Split(path, "/"))
}

func segMatch(pat, seg []string) bool {
	switch {
	case len(pat) == 0 && len(seg) == 0:
		return true
	case len(pat) == 0:
		return false
	case pat[0] == "**":
		// zero segments, or one segment consumed and ** kept
		if segMatch(pat[1:], seg) {
			return true
		}
		return len(seg) > 0 && segMatch(pat, seg[1:])
	case len(seg) == 0:
		return false
	default:
		return oneSegment(pat[0], seg[0]) && segMatch(pat[1:], seg[1:])
	}
}

func oneSegment(pattern, name string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == name
	}
	rest := name
	for i, part := range parts {
		if part == "" {
			continue
		}
		switch {
		case i == 0:
			r, ok := cutPrefix(rest, part)
			if !ok {
				return false
			}
			rest = r
		case i == len(parts)-1:
			return len(rest) >= len(part) && strings.HasSuffix(rest, part)
		default:
			at := strings.Index(rest, part)
			if at < 0 {
				return false
			}
			rest = rest[at+len(part):]
		}
	}
	return true
}

// Walk returns every file under root, as /-joined paths relative to it,
// sorted.
func Walk(root string) ([]string, error) {
	var out []string
	if err := descend(root, root, &out); err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

func descend(root, dir string, out *[]string) error {
	items, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("%s: %w", dir, err)
	}
	names := make([]string, 0, len(items))
	isDir := make(map[string]bool, len(items))
	for _, item := range items {
		names = append(names, item.Name())
		// Stat rather than trusting the dirent: a symlink to a directory
		// reports as a link here, and the Rust original used is_dir(), which
		// follows. Matching that keeps the two trees' walks identical.
		info, err := os.Stat(filepath.Join(dir, item.Name()))
		if err != nil {
			return fmt.Errorf("%s: %w", filepath.Join(dir, item.Name()), err)
		}
		isDir[item.Name()] = info.IsDir()
	}
	sort.Strings(names)
	for _, name := range names {
		kid := filepath.Join(dir, name)
		if isDir[name] {
			if skipped(name) {
				continue
			}
			if err := descend(root, kid, out); err != nil {
				return err
			}
			continue
		}
		rel, err := filepath.Rel(root, kid)
		if err != nil {
			return fmt.Errorf("%s: %w", kid, err)
		}
		*out = append(*out, filepath.ToSlash(rel))
	}
	return nil
}

func skipped(name string) bool {
	for _, s := range skipDirs {
		if s == name {
			return true
		}
	}
	return false
}

// Check tests every path under root against root/docs/coverage.toml. It
// returns how many paths were checked, or the first failure.
func Check(root string) (int, error) {
	mapPath := filepath.Join(root, "docs", "coverage.toml")
	text, err := os.ReadFile(mapPath)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", mapPath, err)
	}
	entries, err := Parse(string(text))
	if err != nil {
		return 0, fmt.Errorf("%s: %w", mapPath, err)
	}
	paths, err := Walk(root)
	if err != nil {
		return 0, err
	}
	for _, p := range paths {
		if !anyEntryMatches(entries, p) {
			return 0, fmt.Errorf(
				"%s matches no entry in docs/coverage.toml; add one naming the doc that describes it", p)
		}
	}
	// The other direction, and it is not symmetry for its own sake. Without
	// it the check is one-directional: every path must match an entry, but no
	// entry need match a path, so a mistyped glob is a DEAD ENTRY the check
	// cannot see. Today that is masked, because no two entries overlap and so
	// a typo surfaces as an unmapped path instead. Add one deliberate overlap
	// and the mask goes with it. An entry matching nothing is a coverage map
	// claiming to describe source that is not there.
	for _, e := range entries {
		if !anyPathMatches(paths, e.Path) {
			return 0, fmt.Errorf(
				"coverage entry `%s` matches no path in the workspace; it is dead, and a dead entry hides a typo",
				e.Path)
		}
	}
	return len(paths), nil
}

func anyEntryMatches(entries []Entry, path string) bool {
	for _, e := range entries {
		if GlobMatch(e.Path, path) {
			return true
		}
	}
	return false
}

func anyPathMatches(paths []string, pattern string) bool {
	for _, p := range paths {
		if GlobMatch(pattern, p) {
			return true
		}
	}
	return false
}
