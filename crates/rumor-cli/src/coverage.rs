//! The doc coverage map and the check over it.
//!
//! `docs/coverage.toml` maps path globs to the documents that describe them.
//! `check` walks the workspace and fails on the first path no entry matches.
//!
//! Two deliberate choices, both about not hiding anything:
//!
//! * The parser is STRICT. A line it does not recognise is an error, not a
//!   skip. A lenient reader would turn one mistyped key into source that is
//!   silently uncovered while the check still reports success.
//! * The walk skips only `.git` and `target`, and says so here rather than
//!   growing a list of conveniences. Every other path has to be mapped.

use std::fs;
use std::path::{Path, PathBuf};

/// One mapping from a path glob to the docs describing those paths.
#[derive(Debug, PartialEq, Eq)]
pub struct Entry {
    pub path: String,
    pub docs: Vec<String>,
}

/// Directories the walk does not descend into. `.git` is not source and
/// `target` is build output; nothing else is exempt.
const SKIP_DIRS: [&str; 2] = [".git", "target"];

/// Parse the coverage map. Strict: an unrecognised line is an error naming
/// its number, and an entry missing either key is an error naming the entry.
pub fn parse(text: &str) -> Result<Vec<Entry>, String> {
    let mut entries: Vec<Entry> = Vec::new();
    let mut path: Option<String> = None;
    let mut docs: Option<Vec<String>> = None;
    let mut open = false;

    fn flush(
        open: &mut bool,
        path: &mut Option<String>,
        docs: &mut Option<Vec<String>>,
        entries: &mut Vec<Entry>,
        at: usize,
    ) -> Result<(), String> {
        if !*open {
            return Ok(());
        }
        let p = path
            .take()
            .ok_or_else(|| format!("entry ending at line {at} has no `path` key"))?;
        let d = docs
            .take()
            .ok_or_else(|| format!("entry `{p}` has no `docs` key"))?;
        if d.is_empty() {
            return Err(format!("entry `{p}` lists no docs"));
        }
        entries.push(Entry { path: p, docs: d });
        *open = false;
        Ok(())
    }

    for (i, raw) in text.lines().enumerate() {
        let n = i + 1;
        let line = raw.trim();
        if line.is_empty() || line.starts_with('#') {
            continue;
        }
        if line == "[[entry]]" {
            flush(&mut open, &mut path, &mut docs, &mut entries, n)?;
            open = true;
            continue;
        }
        if !open {
            return Err(format!("line {n}: `{line}` is outside any [[entry]]"));
        }
        if let Some(rest) = line.strip_prefix("path = ") {
            if path.is_some() {
                return Err(format!("line {n}: a second `path` in one entry"));
            }
            path = Some(
                unquote(rest)
                    .ok_or_else(|| format!("line {n}: `path` value is not a quoted string"))?,
            );
            continue;
        }
        if let Some(rest) = line.strip_prefix("docs = ") {
            if docs.is_some() {
                return Err(format!("line {n}: a second `docs` in one entry"));
            }
            docs = Some(unquote_list(rest).ok_or_else(|| {
                format!("line {n}: `docs` value is not a list of quoted strings")
            })?);
            continue;
        }
        return Err(format!("line {n}: unrecognised `{line}`"));
    }
    let last = text.lines().count();
    flush(&mut open, &mut path, &mut docs, &mut entries, last)?;
    if entries.is_empty() {
        return Err("the coverage map holds no entries".to_string());
    }
    Ok(entries)
}

fn unquote(s: &str) -> Option<String> {
    let s = s.trim();
    let inner = s.strip_prefix('"')?.strip_suffix('"')?;
    if inner.contains('"') {
        return None;
    }
    Some(inner.to_string())
}

fn unquote_list(s: &str) -> Option<Vec<String>> {
    let s = s.trim();
    let inner = s.strip_prefix('[')?.strip_suffix(']')?;
    let mut out = Vec::new();
    for part in inner.split(',') {
        let part = part.trim();
        if part.is_empty() {
            continue;
        }
        out.push(unquote(part)?);
    }
    Some(out)
}

/// Match one path against one glob. `*` matches within a single segment and
/// `**` matches zero or more whole segments.
pub fn glob_match(pattern: &str, path: &str) -> bool {
    let pat: Vec<&str> = pattern.split('/').collect();
    let seg: Vec<&str> = path.split('/').collect();
    seg_match(&pat, &seg)
}

fn seg_match(pat: &[&str], seg: &[&str]) -> bool {
    match (pat.first(), seg.first()) {
        (None, None) => true,
        (None, Some(_)) => false,
        (Some(&"**"), _) => {
            // zero segments, or one segment consumed and `**` kept
            if seg_match(&pat[1..], seg) {
                return true;
            }
            !seg.is_empty() && seg_match(pat, &seg[1..])
        }
        (Some(_), None) => false,
        (Some(p), Some(s)) => one_segment(p, s) && seg_match(&pat[1..], &seg[1..]),
    }
}

fn one_segment(pattern: &str, name: &str) -> bool {
    let parts: Vec<&str> = pattern.split('*').collect();
    if parts.len() == 1 {
        return pattern == name;
    }
    let mut rest = name;
    for (i, part) in parts.iter().enumerate() {
        if part.is_empty() {
            continue;
        }
        if i == 0 {
            match rest.strip_prefix(part) {
                Some(r) => rest = r,
                None => return false,
            }
        } else if i == parts.len() - 1 {
            return rest.len() >= part.len() && rest.ends_with(part);
        } else {
            match rest.find(part) {
                Some(at) => rest = &rest[at + part.len()..],
                None => return false,
            }
        }
    }
    true
}

/// Every file under `root`, as `/`-joined paths relative to it, sorted.
pub fn walk(root: &Path) -> Result<Vec<String>, String> {
    let mut out = Vec::new();
    descend(root, root, &mut out)?;
    out.sort();
    Ok(out)
}

fn descend(root: &Path, dir: &Path, out: &mut Vec<String>) -> Result<(), String> {
    let listing = fs::read_dir(dir).map_err(|e| format!("{}: {e}", dir.display()))?;
    let mut kids: Vec<PathBuf> = Vec::new();
    for item in listing {
        kids.push(item.map_err(|e| format!("{}: {e}", dir.display()))?.path());
    }
    kids.sort();
    for kid in kids {
        let name = match kid.file_name().and_then(|n| n.to_str()) {
            Some(n) => n.to_string(),
            None => return Err(format!("{}: name is not UTF-8", kid.display())),
        };
        if kid.is_dir() {
            if SKIP_DIRS.contains(&name.as_str()) {
                continue;
            }
            descend(root, &kid, out)?;
        } else {
            let rel = kid
                .strip_prefix(root)
                .map_err(|e| format!("{}: {e}", kid.display()))?;
            let mut parts: Vec<String> = Vec::new();
            for c in rel.components() {
                match c.as_os_str().to_str() {
                    Some(p) => parts.push(p.to_string()),
                    None => return Err(format!("{}: name is not UTF-8", kid.display())),
                }
            }
            out.push(parts.join("/"));
        }
    }
    Ok(())
}

/// Check every path under `root` against `root/docs/coverage.toml`.
/// Returns how many paths were checked, or the first failure.
pub fn check(root: &Path) -> Result<usize, String> {
    let map_path = root.join("docs").join("coverage.toml");
    let text = fs::read_to_string(&map_path).map_err(|e| format!("{}: {e}", map_path.display()))?;
    let entries = parse(&text).map_err(|e| format!("{}: {e}", map_path.display()))?;
    let paths = walk(root)?;
    for p in &paths {
        if !entries.iter().any(|e| glob_match(&e.path, p)) {
            return Err(format!(
                "{p} matches no entry in docs/coverage.toml; add one naming the doc that describes it"
            ));
        }
    }
    // The other direction, and it is not symmetry for its own sake. Without
    // it the check is one-directional: every path must match an entry, but no
    // entry need match a path, so a mistyped glob is a DEAD ENTRY the check
    // cannot see. Today that is masked, because no two entries overlap and so
    // a typo surfaces as an unmapped path instead. Add one deliberate overlap
    // and the mask goes with it. An entry matching nothing is a coverage map
    // claiming to describe source that is not there.
    for e in &entries {
        if !paths.iter().any(|p| glob_match(&e.path, p)) {
            return Err(format!(
                "coverage entry `{}` matches no path in the workspace; it is dead, and a dead entry hides a typo",
                e.path
            ));
        }
    }
    Ok(paths.len())
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::atomic::{AtomicUsize, Ordering};

    fn scratch(tag: &str) -> PathBuf {
        static N: AtomicUsize = AtomicUsize::new(0);
        let n = N.fetch_add(1, Ordering::SeqCst);
        let d = std::env::temp_dir().join(format!(
            "rumor-coverage-{}-{}-{}",
            std::process::id(),
            tag,
            n
        ));
        let _ = fs::remove_dir_all(&d);
        fs::create_dir_all(d.join("docs")).unwrap();
        d
    }

    fn repo_root() -> PathBuf {
        Path::new(env!("CARGO_MANIFEST_DIR"))
            .parent()
            .unwrap()
            .parent()
            .unwrap()
            .to_path_buf()
    }

    // The arm issue 2 asks for: the committed tree passes.
    #[test]
    fn committed_tree_is_fully_mapped() {
        let n = check(&repo_root()).expect("the committed tree must be fully mapped");
        assert!(n > 20, "expected the whole workspace, counted {n}");
    }

    // The other arm issue 2 asks for: an unmapped path fails, by name.
    #[test]
    fn an_unmapped_path_fails_and_is_named() {
        let d = scratch("unmapped");
        fs::write(
            d.join("docs/coverage.toml"),
            "[[entry]]\npath = \"docs/*.toml\"\ndocs = [\"docs/release.md\"]\n",
        )
        .unwrap();
        fs::write(d.join("stowaway.rs"), "// mapped by nothing\n").unwrap();
        let err = check(&d).expect_err("an unmapped path must fail the check");
        assert!(
            err.contains("stowaway.rs"),
            "the failure must name it: {err}"
        );
        fs::remove_dir_all(&d).unwrap();
    }

    // A check that cannot fail is not a check: prove the positive arm moves.
    #[test]
    fn the_same_tree_passes_once_the_path_is_mapped() {
        let d = scratch("mapped");
        fs::write(
            d.join("docs/coverage.toml"),
            "[[entry]]\npath = \"docs/*.toml\"\ndocs = [\"docs/release.md\"]\n\
             \n[[entry]]\npath = \"*.rs\"\ndocs = [\"docs/cli.md\"]\n",
        )
        .unwrap();
        fs::write(d.join("stowaway.rs"), "// now mapped\n").unwrap();
        assert_eq!(check(&d).unwrap(), 2);
        fs::remove_dir_all(&d).unwrap();
    }

    // quill's finding in r0.1/4: without this the check is one-directional and a
    // mistyped glob is invisible as soon as two entries overlap.
    #[test]
    fn a_dead_entry_fails_and_is_named() {
        let d = scratch("dead");
        fs::write(
            d.join("docs/coverage.toml"),
            "[[entry]]\npath = \"docs/*.toml\"\ndocs = [\"docs/release.md\"]\n\
             \n[[entry]]\npath = \"docs/*.tml\"\ndocs = [\"docs/release.md\"]\n",
        )
        .unwrap();
        let err = check(&d).expect_err("a dead entry must fail the check");
        assert!(
            err.contains("docs/*.tml") && err.contains("dead"),
            "the failure must name the dead glob: {err}"
        );
        fs::remove_dir_all(&d).unwrap();
    }

    // And the pair that stops the arm above from passing for the wrong reason:
    // the same tree with the typo corrected must pass, so "fails" is about the
    // dead entry and not about the tree.
    #[test]
    fn the_same_tree_passes_once_the_typo_is_corrected() {
        let d = scratch("undead");
        fs::write(
            d.join("docs/coverage.toml"),
            "[[entry]]\npath = \"docs/*.toml\"\ndocs = [\"docs/release.md\"]\n",
        )
        .unwrap();
        assert_eq!(check(&d).unwrap(), 1);
        fs::remove_dir_all(&d).unwrap();
    }

    #[test]
    fn parser_is_strict() {
        assert!(parse("[[entry]]\npath = \"a\"\ndocs = [\"d\"]\n").is_ok());
        for (bad, why) in [
            ("path = \"a\"\n", "a key outside any entry"),
            ("[[entry]]\ndocs = [\"d\"]\n", "no path key"),
            ("[[entry]]\npath = \"a\"\n", "no docs key"),
            ("[[entry]]\npath = \"a\"\ndocs = []\n", "an empty docs list"),
            ("[[entry]]\npath = a\ndocs = [\"d\"]\n", "an unquoted path"),
            (
                "[[entry]]\npath = \"a\"\npth = \"b\"\ndocs = [\"d\"]\n",
                "a typo'd key",
            ),
            (
                "[[entry]]\npath = \"a\"\npath = \"b\"\ndocs = [\"d\"]\n",
                "a duplicate key",
            ),
            ("# only a comment\n", "no entries at all"),
        ] {
            assert!(parse(bad).is_err(), "must reject {why}: {bad:?}");
        }
    }

    #[test]
    fn globs_match_by_segment() {
        for (pat, path) in [
            ("Cargo.toml", "Cargo.toml"),
            ("docs/*.md", "docs/design.md"),
            ("crates/*/Cargo.toml", "crates/rumor-cli/Cargo.toml"),
            (
                "crates/rumor-cli/src/**/*.rs",
                "crates/rumor-cli/src/main.rs",
            ),
            (
                "crates/rumor-cli/src/**/*.rs",
                "crates/rumor-cli/src/a/b/c.rs",
            ),
            ("**/*.md", "README.md"),
            ("*.rs", "stowaway.rs"),
            // quill r0.1/4: one_segment's middle branch needs two `*` in one
            // segment, which no live entry has, so it was unreachable and
            // untested. These two reach it.
            ("docs/*cover*.toml", "docs/coverage.toml"),
            ("*a*e.md", "agent-guide.md"),
        ] {
            assert!(glob_match(pat, path), "{pat} should match {path}");
        }
        for (pat, path) in [
            ("docs/*.md", "docs/adr/README.md"), // * is one segment
            ("docs/*.md", "docs/coverage.toml"),
            ("Cargo.toml", "crates/rumor-cli/Cargo.toml"),
            ("crates/*/Cargo.toml", "crates/a/b/Cargo.toml"),
            ("*.rs", "src/main.rs"),
            ("docs/*cover*.toml", "docs/design.md"),
            ("*a*e.md", "README.md"),
        ] {
            assert!(!glob_match(pat, path), "{pat} should NOT match {path}");
        }
    }
}
