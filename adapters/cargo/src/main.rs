use std::env;
use std::process::{Command, ExitCode};

fn main() -> ExitCode {
    let mut args: Vec<String> = env::args().skip(1).collect();
    if args.first().map(String::as_str) == Some("refute") {
        args.remove(0);
    }
    if args.first().map(String::as_str) == Some("--") {
        args.remove(0);
    }
    if args.first().map(String::as_str) == Some("sync") {
        return run("refute-tool", &["sync"]);
    }
    run(".refute/bin/refute", &args.iter().map(String::as_str).collect::<Vec<_>>())
}

fn run(program: &str, args: &[&str]) -> ExitCode {
    match Command::new(program).args(args).status() {
        Ok(status) => ExitCode::from(status.code().unwrap_or(1) as u8),
        Err(err) => {
            eprintln!("{err}");
            ExitCode::from(1)
        }
    }
}
