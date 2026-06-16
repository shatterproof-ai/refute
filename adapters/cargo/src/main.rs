use std::env;
use std::path::{Path, PathBuf};
use std::process::{Command, ExitCode};

const LOCKFILE: &str = "refute.lock.json";

fn main() -> ExitCode {
    let mut args: Vec<String> = env::args().skip(1).collect();
    if args.first().map(String::as_str) == Some("refute") {
        args.remove(0);
    }
    if args.first().map(String::as_str) == Some("--") {
        args.remove(0);
    }
    if args.first().map(String::as_str) == Some("sync") {
        // Delegate sync to the canonical refute-tool, which performs the walk-up
        // and the actual sync. A clear hint replaces the bare OS error when the
        // tool is not installed.
        return delegate_sync();
    }

    let root = project_root();
    let binary = root.join(".refute").join("bin").join("refute");
    let arg_refs: Vec<&str> = args.iter().map(String::as_str).collect();
    exec_status(Command::new(&binary).args(&arg_refs))
}

fn delegate_sync() -> ExitCode {
    match Command::new("refute-tool").arg("sync").status() {
        Ok(status) => exit_code_from(status),
        Err(err) if err.kind() == std::io::ErrorKind::NotFound => {
            eprintln!(
                "refute-tool not found on PATH; the cargo shim delegates `sync` to refute-tool.\n\
                 Install a refute package-manager shim that ships refute-tool (npm: @shatterproof-ai/refute-tool, \
                 pip: refute-tool), or run `refute-tool sync` from a refute checkout. See INSTALL.md."
            );
            ExitCode::from(127)
        }
        Err(err) => {
            eprintln!("refute-tool sync: {err}");
            ExitCode::from(1)
        }
    }
}

fn exec_status(command: &mut Command) -> ExitCode {
    match command.status() {
        Ok(status) => exit_code_from(status),
        Err(err) if err.kind() == std::io::ErrorKind::NotFound => {
            eprintln!(
                ".refute/bin/refute is missing; run `cargo refute sync` (or `refute-tool sync`) first."
            );
            ExitCode::from(127)
        }
        Err(err) => {
            eprintln!("{err}");
            ExitCode::from(1)
        }
    }
}

// exit_code_from preserves the child's exit code, mapping a signal death to the
// shell's 128+signal convention so it never collapses to success.
fn exit_code_from(status: std::process::ExitStatus) -> ExitCode {
    if let Some(code) = status.code() {
        return ExitCode::from(code as u8);
    }
    #[cfg(unix)]
    {
        use std::os::unix::process::ExitStatusExt;
        if let Some(signal) = status.signal() {
            return ExitCode::from((128 + signal) as u8);
        }
    }
    ExitCode::from(1)
}

// project_root walks up from the working directory to the directory containing
// the lockfile so the shim resolves the same .refute/bin from any
// subdirectory. Falls back to the working directory when no lockfile is found.
fn project_root() -> PathBuf {
    let cwd = env::current_dir().unwrap_or_else(|_| PathBuf::from("."));
    let mut dir: &Path = &cwd;
    loop {
        if dir.join(LOCKFILE).is_file() {
            return dir.to_path_buf();
        }
        match dir.parent() {
            Some(parent) => dir = parent,
            None => return cwd.clone(),
        }
    }
}
