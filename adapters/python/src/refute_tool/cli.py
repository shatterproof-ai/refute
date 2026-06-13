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
HEX_DIGEST_LENGTH = 64


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
    validation_error = validate_artifact(artifact)
    if validation_error is not None:
        print(validation_error, file=sys.stderr)
        return 1
    artifact_sha = artifact["sha256"]
    if active_matches(artifact_sha):
        print(f"{ACTIVE} is already current")
        return 0
    try:
        cache_root = Path(".refute/cache")
        cache_dir = path_under(cache_root, artifact_sha)
        archive = path_under(cache_dir, artifact.get("filename") or "artifact.tar.gz")
    except ValueError as err:
        print(err, file=sys.stderr)
        return 1
    shutil.rmtree(cache_dir, ignore_errors=True)
    cache_dir.mkdir(parents=True, exist_ok=True)
    download(artifact["url"], archive)
    got = sha256(archive)
    if got != artifact_sha:
        print(f"checksum mismatch for {artifact['url']}: got {got}, want {artifact_sha}", file=sys.stderr)
        return 1
    with tarfile.open(archive, "r:gz") as tar:
        member = find_refute_member(tar)
        if member is None:
            print(f"{archive} does not contain refute", file=sys.stderr)
            return 1
        if not is_safe_archive_member(member.name):
            print(f"{archive} contains unsafe refute member {member.name!r}", file=sys.stderr)
            return 1
        extracted = cache_dir / "refute"
        source = tar.extractfile(member)
        if source is None:
            print(f"{archive} refute member is not a regular file", file=sys.stderr)
            return 1
        with source, extracted.open("wb") as output:
            shutil.copyfileobj(source, output)
    ACTIVE.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(extracted, ACTIVE)
    ACTIVE.chmod(0o755)
    ACTIVE.with_name(ACTIVE.name + ".artifact-sha256").write_text(artifact_sha + "\n", encoding="utf-8")
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


def find_refute_member(tar):
    return next((item for item in tar.getmembers() if item.isfile() and tar_member_basename(item.name) == "refute"), None)


def is_safe_archive_member(name):
    if not name or name.startswith(("/", "\\")) or has_drive_prefix(name):
        return False
    return ".." not in tar_member_parts(name)


def tar_member_basename(name):
    parts = tar_member_parts(name)
    return parts[-1] if parts else ""


def tar_member_parts(name):
    return [part for part in name.replace("\\", "/").split("/") if part]


def validate_artifact(artifact):
    digest = artifact.get("sha256", "")
    if not is_sha256_hex(digest):
        return f"invalid artifact sha256 {digest!r}"
    filename = artifact.get("filename")
    if filename is not None and not is_safe_lock_filename(filename):
        return f"unsafe artifact filename {filename!r}"
    return None


def is_sha256_hex(value):
    return isinstance(value, str) and len(value) == HEX_DIGEST_LENGTH and all(char in "0123456789abcdefABCDEF" for char in value)


def is_safe_lock_filename(name):
    return isinstance(name, str) and name != "" and not has_drive_prefix(name) and "/" not in name and "\\" not in name and ".." not in name


def has_drive_prefix(name):
    return len(name) >= 2 and name[0].isalpha() and name[1] == ":"


def path_under(root, child):
    candidate = root / child
    try:
        candidate.resolve(strict=False).relative_to(root.resolve(strict=False))
    except ValueError as err:
        raise ValueError(f"path {candidate} escapes {root}") from err
    return candidate


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
