import json
import os
import stat
import hashlib
import shutil
import subprocess
import sys
import tarfile
import tempfile
import urllib.request
from pathlib import Path
from sysconfig import get_platform

LOCKFILE = "refute.lock.json"
ACTIVE_REL = Path(".refute/bin/refute")
HEX_DIGEST_LENGTH = 64


def main(argv=None):
    argv = list(sys.argv[1:] if argv is None else argv)
    if not argv or argv[0] in {"-h", "--help"}:
        print("usage: refute-tool <sync|run|doctor>")
        return 0
    command = argv[0]
    root = project_root()
    if command == "sync":
        return sync(root)
    if command == "run":
        args = argv[2:] if len(argv) > 1 and argv[1] == "--" else argv[1:]
        return run(root, args)
    if command == "doctor":
        return doctor(root)
    print(f"unknown refute-tool command {command}", file=sys.stderr)
    return 2


def refute():
    raise SystemExit(run(project_root(), sys.argv[1:]))


def project_root():
    """Walk up from the working directory to the directory containing the
    lockfile so the shim resolves the same .refute/bin from any subdirectory.
    Falls back to the working directory when no lockfile is found."""
    directory = Path.cwd()
    for candidate in (directory, *directory.parents):
        if is_regular_non_symlink(candidate / LOCKFILE):
            return candidate
    return directory


def sync(root):
    lock_path = root / LOCKFILE
    active = root / ACTIVE_REL
    if not is_regular_non_symlink(lock_path):
        print(f"missing {LOCKFILE}", file=sys.stderr)
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
    try:
        tool_root = root / ".refute"
        ensure_real_directory(tool_root)
        cache_root = tool_root / "cache"
        ensure_real_directory(cache_root)
        ensure_real_directory(active.parent)
    except ValueError as err:
        print(err, file=sys.stderr)
        return 1
    if active_matches(active, artifact_sha):
        print(f"{ACTIVE_REL} is already current")
        return 0
    try:
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
    atomic_copy2(extracted, active, 0o755)
    atomic_write_text(active.with_name(active.name + ".artifact-sha256"), artifact_sha + "\n")
    atomic_write_text(active.with_name(active.name + ".binary-sha256"), sha256(active) + "\n")
    print(f"installed {ACTIVE_REL}")
    return 0


def doctor(root):
    active = root / ACTIVE_REL
    print("lockfile: present" if is_regular_non_symlink(root / LOCKFILE) else "lockfile: missing")
    if not active.exists():
        print(f"binary: missing ({ACTIVE_REL})")
        return 0
    print(f"binary: present ({ACTIVE_REL})")
    return run(root, ["doctor"])


def run(root, args):
    active = root / ACTIVE_REL
    try:
        completed = subprocess.run([os.fspath(active), *args], check=False)
    except FileNotFoundError:
        print(f"{ACTIVE_REL} is missing; run `refute-tool sync` first", file=sys.stderr)
        return 127
    code = completed.returncode
    # Propagate signal deaths as a non-zero status using the shell's 128+signal
    # convention instead of the negative value subprocess returns.
    if code < 0:
        return 128 + (-code)
    return code


def select_artifact(lock):
    platform = python_platform()
    arch = python_arch()
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


def python_arch():
    machine = os.uname().machine.lower() if hasattr(os, "uname") else ""
    if machine in {"x86_64", "amd64"}:
        return "amd64"
    if machine in {"aarch64", "arm64"}:
        return "arm64"
    return machine


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


def ensure_real_directory(path):
    try:
        info = path.lstat()
    except FileNotFoundError:
        path.mkdir(mode=0o755, parents=True)
        return
    if not path.is_dir() or stat.S_ISLNK(info.st_mode):
        raise ValueError(f"{path} is not a real directory")


def sha256(path):
    digest = hashlib.sha256()
    with path.open("rb") as file:
        for chunk in iter(lambda: file.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def marker_matches(path, digest):
    return is_regular_non_symlink(path) and path.read_text(encoding="utf-8").strip() == digest


def active_matches(active, artifact_digest):
    return (
        is_regular_non_symlink(active)
        and marker_matches(active.with_name(active.name + ".artifact-sha256"), artifact_digest)
        and marker_matches(active.with_name(active.name + ".binary-sha256"), sha256(active))
    )


def is_regular_non_symlink(path):
    try:
        info = path.lstat()
    except FileNotFoundError:
        return False
    return stat.S_ISREG(info.st_mode)


def atomic_copy2(src, dest, mode):
    handle, temp_name = tempfile.mkstemp(dir=dest.parent, prefix=f".{dest.name}-")
    temp_path = Path(temp_name)
    try:
        with os.fdopen(handle, "wb") as temp:
            with src.open("rb") as source:
                shutil.copyfileobj(source, temp)
        shutil.copystat(src, temp_path)
        temp_path.chmod(mode)
        os.replace(temp_path, dest)
    except Exception:
        temp_path.unlink(missing_ok=True)
        raise


def atomic_write_text(path, data):
    encoded = data.encode("utf-8")
    handle, temp_name = tempfile.mkstemp(dir=path.parent, prefix=f".{path.name}-")
    temp_path = Path(temp_name)
    try:
        with os.fdopen(handle, "wb") as temp:
            temp.write(encoded)
        os.replace(temp_path, path)
    except Exception:
        temp_path.unlink(missing_ok=True)
        raise


if __name__ == "__main__":
    raise SystemExit(main())
