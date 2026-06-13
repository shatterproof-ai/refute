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
    def test_sync_rejects_traversal_member_without_writing_outside_cache(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            archive = root / "malicious.tar.gz"
            body = b"#!/bin/sh\necho escaped\n"

            buffer = io.BytesIO()
            with gzip_tar(buffer) as tar:
                info = tarfile.TarInfo("../../../outside/refute")
                info.mode = 0o755
                info.size = len(body)
                tar.addfile(info, io.BytesIO(body))
            archive.write_bytes(buffer.getvalue())
            digest = hashlib.sha256(buffer.getvalue()).hexdigest()

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
                                "filename": "artifact.tar.gz",
                            }
                        ],
                    }
                ),
                encoding="utf-8",
            )

            with chdir(root):
                result = cli.sync()

            self.assertEqual(result, 1)
            self.assertFalse((root / "outside" / "refute").exists())
            self.assertFalse((root / ".refute" / "bin" / "refute").exists())


@contextlib.contextmanager
def gzip_tar(buffer):
    with tarfile.open(fileobj=buffer, mode="w:gz") as tar:
        yield tar


if __name__ == "__main__":
    unittest.main()
