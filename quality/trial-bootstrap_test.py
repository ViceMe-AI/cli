#!/usr/bin/env python3
"""Verify the one-time bootstrap and local runtime packaging boundary."""
import contextlib
import hashlib
import importlib.util
import io
import json
from pathlib import Path
import sys
import tempfile
import unittest
from unittest import mock
import zipfile

ROOT = Path(__file__).resolve().parents[1]
SCRIPTS = ROOT / "skills/use-a-skill/scripts"


class BootstrapTests(unittest.TestCase):
    def setUp(self):
        spec = importlib.util.spec_from_file_location("bootstrap", SCRIPTS / "trial.py")
        self.module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.module)

    def run_bundle(self, argv):
        def inspect(filename):
            directory = Path(filename).parents[1]
            self.assertEqual(set(str(p.relative_to(directory)) for p in directory.rglob("*") if p.is_file()), self.module.RUNTIME_FILES | {"environment.json"})
            env = json.loads((directory / "environment.json").read_text())
            self.assertEqual(env["market"], "global")
            self.assertEqual(env["apiBaseUrl"], "https://api.viceme.ai")
            return {"run": lambda args: 19 if args == argv[1:] else 99}
        with mock.patch.object(sys, "argv", argv), mock.patch.object(self.module.runpy, "run_path", side_effect=inspect):
            return self.module.main()

    def test_official_bundle_is_local_and_matches_sources(self):
        archive = (SCRIPTS / "trial-runtime.zip").read_bytes()
        self.assertEqual(hashlib.sha256(archive).hexdigest(), self.module.RUNTIME_SHA256)
        with zipfile.ZipFile(io.BytesIO(archive)) as bundle:
            self.assertEqual(bundle.read("scripts/trial.py"), (SCRIPTS / "trial_runtime.py").read_bytes())
            self.assertEqual(bundle.read("widgets/payment.html"), (ROOT / "widgets/payment.html").read_bytes())
        with mock.patch.object(self.module.urllib.request, "build_opener", side_effect=AssertionError("static HTTP after official install")):
            self.assertEqual(self.run_bundle(["trial.py", "ready", "--product", "example", "--market=global"]), 19)

    def test_streamed_bootstrap_fetches_exactly_one_digest_addressed_bundle(self):
        archive = (SCRIPTS / "trial-runtime.zip").read_bytes()
        with tempfile.TemporaryDirectory() as temporary:
            self.module.__file__ = str(Path(temporary) / "trial.py")
            response = contextlib.closing(io.BytesIO(archive))
            opener = mock.Mock()
            opener.open.return_value = response
            with mock.patch.object(self.module.urllib.request, "build_opener", return_value=opener):
                self.assertEqual(self.run_bundle(["trial.py", "ready", "--market", "global"]), 19)
            opener.open.assert_called_once_with("https://s3.viceme.ai/skills/use-a-skill/scripts/sha256-" + self.module.RUNTIME_SHA256 + "/trial-runtime.zip", timeout=30)

    def test_tampered_bundle_never_executes(self):
        with tempfile.TemporaryDirectory() as temporary:
            self.module.__file__ = str(Path(temporary) / "trial.py")
            (Path(temporary) / "trial-runtime.zip").write_bytes(b"tampered")
            with mock.patch.object(self.module.runpy, "run_path") as execute, mock.patch.object(sys, "argv", ["trial.py"]):
                with self.assertRaisesRegex(ValueError, "digest mismatch"):
                    self.module.main()
                execute.assert_not_called()

    def test_unsafe_members_never_extract_or_execute(self):
        for name in ("../escape", "scripts/trial.py"):
            with self.subTest(name=name), tempfile.TemporaryDirectory() as temporary:
                buffer = io.BytesIO()
                with zipfile.ZipFile(buffer, "w") as bundle:
                    bundle.writestr(name, b"invalid")
                raw = buffer.getvalue()
                self.module.__file__ = str(Path(temporary) / "trial.py")
                self.module.RUNTIME_SHA256 = hashlib.sha256(raw).hexdigest()
                (Path(temporary) / "trial-runtime.zip").write_bytes(raw)
                with mock.patch.object(self.module.runpy, "run_path") as execute, mock.patch.object(sys, "argv", ["trial.py"]):
                    with self.assertRaisesRegex(ValueError, "invalid runtime members"):
                        self.module.main()
                    execute.assert_not_called()

    def test_bootstrap_error_writes_utf8_on_legacy_windows_stdout(self):
        class LegacyStdout:
            encoding = "cp1252"

            def __init__(self):
                self.buffer = io.BytesIO()

            def write(self, text):
                return self.buffer.write(text.encode("cp1252"))

            def flush(self):
                pass

        fake = LegacyStdout()
        payload = {"ok": False, "code": "RUNTIME_BOOTSTRAP_FAILED", "message": "运行资源未完整取得或校验失败"}
        with mock.patch.object(sys, "stdout", fake):
            self.module.emit_line(payload)
        self.assertEqual(fake.buffer.getvalue(), (json.dumps(payload, ensure_ascii=False) + "\n").encode("utf-8"))


if __name__ == "__main__":
    unittest.main()
