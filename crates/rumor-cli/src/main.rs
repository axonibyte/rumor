//! `rumor` — the command line entry point.
//!
//! R0.1 creates exactly one subcommand, `docs --check`, which is the CLI
//! surface this tag's minor bump pays for. Everything else arrives with the
//! phase that owns it.

mod coverage;

use std::path::Path;
use std::process::ExitCode;

const USAGE: &str = "usage: rumor docs --check";

fn main() -> ExitCode {
    let args: Vec<String> = std::env::args().skip(1).collect();
    let as_str: Vec<&str> = args.iter().map(String::as_str).collect();
    match as_str.as_slice() {
        ["docs", "--check"] => match coverage::check(Path::new(".")) {
            Ok(n) => {
                println!("docs --check: {n} path(s), every one mapped in docs/coverage.toml");
                ExitCode::SUCCESS
            }
            Err(why) => {
                eprintln!("docs --check: {why}");
                ExitCode::from(1)
            }
        },
        _ => {
            eprintln!("{USAGE}");
            ExitCode::from(2)
        }
    }
}
