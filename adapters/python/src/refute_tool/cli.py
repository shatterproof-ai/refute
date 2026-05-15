import json
import os
import hashlib
import shutil
import subprocess
import sys
import tarfile
import urllib.request
from pathlib import Path
from sysconfig import get_platform

ACTIVE = Path(".refute/bin/refute")


def main(argv=None):
    argv = list(sys.argv[1:] if argv is None else argv)
    if not argv or argv[0] in {"-h", "--help"}:
        print("usage: refute-tool <sync|run|doctor>")
        return 0
    command = argv[0]
    if command == "sync":
        return sync()
    if command == "run":
        args = argv[2:] if len(argv) > 1 and argv[1] == "--" else argv[1:]
        return run(args)
    if command == "doctor":
        return doctor()
    print(f"unknown refute-tool command {command}", file=sys.stderr)
    return 2


def refute():
    raise SystemExit(run(sys.argv[1:]))


def sync():
    lock_path = Path("refute.lock.json")
    if not lock_path.exists():
        print("missing refute.lock.json", file=sys.stderr)
        return 1
    with lock_path.open(encoding="utf-8") as file:
        lock = json.load(file)
    artifact = select_artifact(lock)
    if artifact is None:
        print(f"unsupported platform for refute {lock.get('version')}", file=sys.stderr)
        return 1
    if active_matches(artifact["sha256"]):
        print(f"{ACTIVE} is already current")
        return 0
    cache_dir = Path(".refute/cache") / artifact["sha256"]
    shutil.rmtree(cache_dir, ignore_errors=True)
    cache_dir.mkdir(parents=True, exist_ok=True)
    archive = cache_dir / artifact.get("filename", "artifact.tar.gz")
    download(artifact["url"], archive)
    got = sha256(archive)
    if got != artifact["sha256"]:
        print(f"checksum mismatch for {artifact['url']}: got {got}, want {artifact['sha256']}", file=sys.stderr)
        return 1
    with tarfile.open(archive, "r:gz") as tar:
        member = next((item for item in tar.getmembers() if Path(item.name).name == "refute"), None)
        if member is None:
            print(f"{archive} does not contain refute", file=sys.stderr)
            return 1
        tar.extract(member, cache_dir)
        extracted = cache_dir / member.name
    ACTIVE.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(extracted, ACTIVE)
    ACTIVE.chmod(0o755)
    ACTIVE.with_name(ACTIVE.name + ".artifact-sha256").write_text(artifact["sha256"] + "\n", encoding="utf-8")
    ACTIVE.with_name(ACTIVE.name + ".binary-sha256").write_text(sha256(ACTIVE) + "\n", encoding="utf-8")
    print(f"installed {ACTIVE}")
    return 0


def doctor():
    print("lockfile: present" if Path("refute.lock.json").exists() else "lockfile: missing")
    if not ACTIVE.exists():
        print(f"binary: missing ({ACTIVE})")
        return 0
    print(f"binary: present ({ACTIVE})")
    return run(["doctor"])


def run(args):
    try:
        completed = subprocess.run([os.fspath(ACTIVE), *args], check=False)
    except FileNotFoundError:
        print(f"{ACTIVE} is missing; run `refute-tool sync` first", file=sys.stderr)
        return 1
    return completed.returncode


def select_artifact(lock):
    platform = python_platform()
    machine = os.uname().machine.lower() if hasattr(os, "uname") else ""
    arch = "amd64" if machine in {"x86_64", "amd64"} else "arm64" if machine in {"aarch64", "arm64"} else machine
    for artifact in lock.get("artifacts", []):
        if artifact.get("platform") == platform and artifact.get("architecture") == arch:
            return artifact
    return None


def python_platform():
    value = get_platform()
    if value.startswith("macosx"):
        return "darwin"
    if value.startswith("linux"):
        return "linux"
    if value.startswith("win"):
        return "windows"
    return value


def download(url, dest):
    if url.startswith("file://"):
        shutil.copyfile(url[7:], dest)
        return
    urllib.request.urlretrieve(url, dest)


def sha256(path):
    digest = hashlib.sha256()
    with path.open("rb") as file:
        for chunk in iter(lambda: file.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def marker_matches(path, digest):
    return path.exists() and path.read_text(encoding="utf-8").strip() == digest


def active_matches(artifact_digest):
    return (
        ACTIVE.exists()
        and marker_matches(ACTIVE.with_name(ACTIVE.name + ".artifact-sha256"), artifact_digest)
        and marker_matches(ACTIVE.with_name(ACTIVE.name + ".binary-sha256"), sha256(ACTIVE))
    )
