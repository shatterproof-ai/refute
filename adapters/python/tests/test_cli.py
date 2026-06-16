import contextlib
import hashlib
import io
import json
import os
import sys
import tarfile
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

from refute_tool import cli


@contextlib.contextmanager
def chdir(path):
    previous = Path.cwd()
    os.chdir(path)
    try:
        yield
    finally:
        os.chdir(previous)


def current_arch():
    machine = os.uname().machine.lower() if hasattr(os, "uname") else ""
    return "amd64" if machine in {"x86_64", "amd64"} else "arm64" if machine in {"aarch64", "arm64"} else machine


class SyncTests(unittest.TestCase):
    def test_sync_rejects_malicious_sha256_before_cache_path_use(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            archive, _ = write_archive(root)
            write_lock(root, archive, "../outside")

            with chdir(root):
                result = cli.sync(cli.project_root())

            self.assertEqual(result, 1)
            self.assertFalse((root / ".refute").exists())
            self.assertFalse((root / "outside").exists())

    def test_sync_rejects_malicious_filename_before_archive_path_use(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            archive, digest = write_archive(root)
            write_lock(root, archive, digest, filename="../artifact.tar.gz")

            with chdir(root):
                result = cli.sync(cli.project_root())

            self.assertEqual(result, 1)
            self.assertFalse((root / ".refute").exists())
            self.assertFalse((root / "artifact.tar.gz").exists())

    def test_sync_rejects_symlinked_cache_root(self):
        if not hasattr(os, "symlink"):
            self.skipTest("symlink unavailable")
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            archive, digest = write_archive(root)
            write_lock(root, archive, digest)
            tool_root = root / ".refute"
            outside = root / "outside-cache"
            tool_root.mkdir()
            outside.mkdir()
            os.symlink(outside, tool_root / "cache")

            with chdir(root):
                result = cli.sync(cli.project_root())

            self.assertEqual(result, 1)
            self.assertTrue((tool_root / "cache").is_symlink())
            self.assertEqual(list(outside.iterdir()), [])
            self.assertFalse((root / ".refute" / "bin" / "refute").exists())

    def test_sync_rejects_symlinked_bin_root(self):
        if not hasattr(os, "symlink"):
            self.skipTest("symlink unavailable")
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            archive, digest = write_archive(root)
            write_lock(root, archive, digest)
            tool_root = root / ".refute"
            outside = root / "outside-bin"
            tool_root.mkdir()
            (tool_root / "cache").mkdir()
            outside.mkdir()
            os.symlink(outside, tool_root / "bin")

            with chdir(root):
                result = cli.sync(cli.project_root())

            self.assertEqual(result, 1)
            self.assertTrue((tool_root / "bin").is_symlink())
            self.assertEqual(list(outside.iterdir()), [])

    def test_sync_replaces_symlinked_active_files_without_writing_outside_bin(self):
        if not hasattr(os, "symlink"):
            self.skipTest("symlink unavailable")
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            archive, digest = write_archive(root)
            write_lock(root, archive, digest)
            bin_root = root / ".refute" / "bin"
            cache_root = root / ".refute" / "cache"
            outside = root / "outside-bin"
            cache_root.mkdir(parents=True)
            bin_root.mkdir()
            outside.mkdir()
            targets = {
                bin_root / "refute": outside / "refute",
                bin_root / "refute.artifact-sha256": outside / "artifact",
                bin_root / "refute.binary-sha256": outside / "binary",
            }
            for link, target in targets.items():
                target.write_text("outside\n", encoding="utf-8")
                os.symlink(target, link)

            with chdir(root):
                result = cli.sync(cli.project_root())

            self.assertEqual(result, 0)
            for link, target in targets.items():
                self.assertFalse(link.is_symlink(), link)
                self.assertEqual(target.read_text(encoding="utf-8"), "outside\n")
            self.assertIn("synced", (bin_root / "refute").read_text(encoding="utf-8"))
            self.assertEqual((bin_root / "refute.artifact-sha256").read_text(encoding="utf-8").strip(), digest)

    def test_sync_rejects_traversal_member_without_writing_outside_cache(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            archive, digest = write_archive(root, "../../../outside/refute")
            write_lock(root, archive, digest)

            with chdir(root):
                result = cli.sync(cli.project_root())

            self.assertEqual(result, 1)
            self.assertFalse((root / "outside" / "refute").exists())
            self.assertFalse((root / ".refute" / "bin" / "refute").exists())

    def test_sync_rejects_platform_independent_unsafe_tar_members(self):
        for name in ["/tmp/refute", "C:/tmp/refute", r"..\..\refute"]:
            with self.subTest(name=name):
                with tempfile.TemporaryDirectory() as temp:
                    root = Path(temp)
                    archive, digest = write_archive(root, name)
                    write_lock(root, archive, digest)

                    with chdir(root):
                        result = cli.sync(cli.project_root())

                    self.assertEqual(result, 1)
                    self.assertFalse((root / ".refute" / "bin" / "refute").exists())


class RootResolutionTests(unittest.TestCase):
    def test_project_root_walks_up_to_lockfile_directory(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp).resolve()
            archive, digest = write_archive(root)
            write_lock(root, archive, digest)
            nested = root / "a" / "b"
            nested.mkdir(parents=True)

            with chdir(nested):
                self.assertEqual(cli.project_root().resolve(), root)
                # sync from a subdirectory installs into the lockfile's .refute,
                # not the subdirectory's.
                self.assertEqual(cli.sync(cli.project_root()), 0)

            self.assertTrue((root / ".refute" / "bin" / "refute").exists())
            self.assertFalse((nested / ".refute").exists())


class ExitPropagationTests(unittest.TestCase):
    def test_run_translates_signal_death_to_non_zero(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            active = root / cli.ACTIVE_REL
            active.parent.mkdir(parents=True)
            # A script that kills itself with SIGTERM, so subprocess reports a
            # negative return code (signal death).
            active.write_text("#!/bin/sh\nkill -TERM $$\n", encoding="utf-8")
            active.chmod(0o755)

            code = cli.run(root, [])

            self.assertEqual(code, 128 + 15)  # SIGTERM


@contextlib.contextmanager
def gzip_tar(buffer):
    with tarfile.open(fileobj=buffer, mode="w:gz") as tar:
        yield tar


def write_archive(root, name="refute"):
    archive = root / "archive.tar.gz"
    body = b"#!/bin/sh\necho synced\n"
    buffer = io.BytesIO()
    with gzip_tar(buffer) as tar:
        info = tarfile.TarInfo(name)
        info.mode = 0o755
        info.size = len(body)
        tar.addfile(info, io.BytesIO(body))
    archive.write_bytes(buffer.getvalue())
    return archive, hashlib.sha256(buffer.getvalue()).hexdigest()


def write_lock(root, archive, digest, filename="artifact.tar.gz"):
    (root / "refute.lock.json").write_text(
        json.dumps(
            {
                "version": "v9.9.9",
                "artifacts": [
                    {
                        "platform": cli.python_platform(),
                        "architecture": current_arch(),
                        "url": archive.as_uri(),
                        "sha256": digest,
                        "filename": filename,
                    }
                ],
            }
        ),
        encoding="utf-8",
    )


if __name__ == "__main__":
    unittest.main()
