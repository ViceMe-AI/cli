#!/usr/bin/env python3
"""Verify the one-time bootstrap and local runtime packaging boundary."""
import contextlib
import hashlib
import importlib.util
import io
import json
import os
import subprocess
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
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
            self.assertEqual(env["apiBaseUrl"], "https://viceme.ai/api")
            return {"run": lambda args: 19 if args == argv[1:] else 99}
        with mock.patch.object(sys, "argv", argv), mock.patch.object(self.module.runpy, "run_path", side_effect=inspect):
            return self.module.main()

    def test_official_bundle_is_local_and_matches_sources(self):
        archive = (SCRIPTS / "trial-runtime.zip").read_bytes()
        self.assertEqual(hashlib.sha256(archive).hexdigest(), self.module.RUNTIME_SHA256)
        with zipfile.ZipFile(io.BytesIO(archive)) as bundle:
            self.assertEqual(bundle.read("guides/host-presentation.md"), (ROOT / "skills/use-a-skill/references/host-presentation.md").read_bytes())
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

    def test_no_cli_installs_purchase_entry_before_order_and_restores_same_directory(self):
        product = "11111111-1111-4111-8111-111111111111"
        release = "22222222-2222-4222-8222-222222222222"
        package = io.BytesIO()
        with zipfile.ZipFile(package, "w") as archive:
            archive.writestr("SKILL.md", "---\nname: paid-demo\ndescription: Test purchase\n---\nFull paid Skill")
        archive_bytes = package.getvalue()
        digest = hashlib.sha256(archive_bytes).hexdigest()
        paid = [False]
        requests = []
        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *args):
                pass
            def respond(self, data):
                content = json.dumps(data).encode()
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(content)))
                self.end_headers()
                self.wfile.write(content)
            def do_GET(self):
                requests.append(self.path)
                if self.path == "/package.zip":
                    if not paid[0]:
                        self.send_error(403)
                        return
                    self.send_response(200)
                    self.end_headers()
                    self.wfile.write(archive_bytes)
                    return
                if self.path.startswith("/v1/products/" + product + "?"):
                    self.respond({"id": product, "market": "CN", "title": "Paid example", "summary": "Test purchase", "slug": "public-name"})
                    return
                if self.path != "/v1/skills/" + product + "/access":
                    self.send_error(404)
                    return
                self.respond({"productId": product, "isFree": False, "trial": None, "purchaseAvailable": True,
                    "release": {"id": release, "artifactDigest": digest}})
            def do_POST(self):
                requests.append(self.path)
                body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
                if not self.path.startswith("/v1/skills/" + product + "/purchase"):
                    self.send_error(404)
                    return
                if self.path.endswith("/download"):
                    if not paid[0]:
                        self.send_error(404)
                        return
                    self.respond({"access": {"productId": product, "owned": True, "installKind": "OWNED_PAID",
                        "release": {"id": release, "artifactDigest": digest, "fileName": "paid.zip"}},
                        "download": {"releaseId": release, "artifactDigest": digest,
                        "url": "http://127.0.0.1:%s/package.zip" % self.server.server_port}})
                    return
                self.respond({"productId": product, "orderNo": "DIRECT_ORDER_01", "title": "Test paid Skill",
                    "status": "PAID" if paid[0] else "PENDING", "amountCents": 990, "currency": "CNY",
                    "expiresAt": "2099-01-01T00:00:00Z", "checkoutUrl": None, "checkoutImageUrl": None,
                    "paymentAction": None if paid[0] else {"type": "QR_CODE", "content": "weixin://pay/test-only"}})
        server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        worker = threading.Thread(target=server.serve_forever, daemon=True)
        worker.start()
        try:
            with tempfile.TemporaryDirectory() as temporary:
                directory = Path(temporary)
                environment = {**os.environ, "HOME": temporary, "PATH": str(directory / "no-cli"),
                    "CI": "true", "VICEME_CLI_CONFIG_DIR": str(directory / "unused-cli-config")}
                for key in ("VICEME_ACCESS_TOKEN", "CODEX_THREAD_ID", "CODEX_SESSION_ID", "CODEBUDDY_SESSION_ID", "CLAUDECODE"):
                    environment.pop(key, None)
                runner = directory / "run-bootstrap.py"
                runner.write_text("import runpy, sys\nm = runpy.run_path(sys.argv[1])\nm['API_ORIGIN']['cn'] = sys.argv[2]\nsys.argv = sys.argv[1:2] + sys.argv[3:]\nraise SystemExit(m['main']())\n")
                first = subprocess.run([sys.executable, str(runner), str(SCRIPTS / "trial.py"),
                    "http://127.0.0.1:%s" % server.server_port, "install", "--product", product,
                    "--market", "cn", "--agent", "agents"], cwd=temporary, env=environment,
                    capture_output=True, text=True, timeout=20)
                self.assertEqual(first.returncode, 0, first.stderr + first.stdout)
                entry = json.loads(first.stdout)
                self.assertEqual(entry["kind"], "purchase")
                self.assertEqual(entry["nextAction"], "PURCHASE_REQUIRED")
                self.assertFalse(entry["allowed"])
                runtime = Path(entry["runtimePath"])
                skill_path = Path(entry["skillPath"])
                self.assertTrue(runtime.is_file(), "bootstrap cleanup must leave installed dependencies")
                self.assertTrue(skill_path.is_file())
                self.assertIn("viceme-purchase-required:v1", skill_path.read_text())
                self.assertIn("## 使用前必读", skill_path.read_text())
                self.assertIn("references/purchase.md", skill_path.read_text())
                self.assertIn("不算完成支付展示", skill_path.read_text())
                self.assertIn("开场白", skill_path.read_text())
                self.assertIn("无试用开场白", skill_path.read_text())
                self.assertIn("正式内容尚未安装", skill_path.read_text())
                self.assertNotIn("imageChatSrc", skill_path.read_text())
                self.assertIn("/viceme-purchase-required:v1", skill_path.read_text())
                self.assertNotIn("Full paid Skill", skill_path.read_text())
                guide = skill_path.parent / "references/purchase.md"
                self.assertTrue(guide.is_file())
                self.assertIn("## 通用支付展示", guide.read_text())
                self.assertIn("## 无试用开场白", guide.read_text())
                self.assertIn("正式内容尚未安装", guide.read_text())
                self.assertIn("正式内容安装完成后，重新读取实际 SKILL.md 并继续原任务", guide.read_text())
                self.assertIn("没有原任务时，简短说明怎么开始，等待用户提供任务内容", guide.read_text())
                self.assertIn("host-presentation.md", guide.read_text())
                self.assertEqual((guide.parent / "host-presentation.md").read_bytes(), (ROOT / "skills/use-a-skill/references/host-presentation.md").read_bytes())
                self.assertFalse((skill_path.parent / ".viceme/trial-body.md").exists())
                self.assertFalse((directory / ".agents/skills/paid-demo").exists())
                self.assertFalse((directory / ".viceme/trial" / (product + ".json")).exists())
                state_path = directory / ".viceme/purchases" / (product + ".json")
                self.assertFalse(state_path.exists())
                self.assertFalse(any("purchase" in path or path == "/package.zip" for path in requests))
                before = list(requests)
                for command in ("ready", "status"):
                    check = subprocess.run([sys.executable, str(runtime), command, "--product", product,
                        "--market", "cn"], cwd=temporary, env=environment, capture_output=True, text=True, timeout=20)
                    self.assertEqual(check.returncode, 0, check.stdout + check.stderr)
                    ready = json.loads(check.stdout)
                    self.assertTrue(ready["ready"])
                    self.assertFalse(ready["allowed"])
                    self.assertEqual(ready["nextAction"], "PURCHASE_REQUIRED")
                    self.assertNotIn("remainingUses", ready)
                self.assertEqual(requests, before, "readiness cannot create a trial or order")
                order = subprocess.run([sys.executable, str(runtime), "use", "--product", product,
                    "--market", "cn", "--agent", "agents"], cwd=temporary,
                    env=environment, capture_output=True, text=True, timeout=20)
                self.assertEqual(order.returncode, 0, order.stdout + order.stderr)
                pending = json.loads(order.stdout)
                self.assertEqual(pending["nextAction"], "PRESENT_PAYMENT_WIDGET")
                self.assertEqual(pending["runtimePath"], str(runtime))
                self.assertEqual(pending["skillPath"], str(skill_path))
                self.assertNotIn("Full paid Skill", skill_path.read_text())
                identity = state_path.read_bytes()
                for file in skill_path.parent.rglob("*"):
                    if file.is_file():
                        self.assertNotIn(json.loads(identity)["secret"].encode(), file.read_bytes())
                paid[0] = True
                second = subprocess.run([sys.executable, str(runtime), "purchase", "--product", product,
                    "--market", "cn", "--agent", "agents", "--wait", "0"], cwd=temporary,
                    env=environment, capture_output=True, text=True, timeout=20)
                self.assertEqual(second.returncode, 0, second.stderr + second.stdout)
                installed = json.loads(second.stdout)
                self.assertEqual(installed["skillPath"], str(skill_path))
                self.assertFalse((directory / ".agents/skills/paid-demo").exists())
                self.assertTrue(installed["owned"])
                self.assertEqual(installed["nextAction"], "CONTINUE_ORIGINAL_TASK_WITH_INSTALLED_SKILL")
                content = Path(installed["skillPath"]).read_text()
                self.assertIn("Full paid Skill", content)
                self.assertNotIn("viceme-trial:v1", content)
                self.assertNotIn("viceme-purchase-required:v1", content)
                self.assertEqual(state_path.read_bytes(), identity)
                self.assertFalse(any("trial-grants" in path or "trial-purchase" in path for path in requests))
                self.assertEqual(requests.count("/v1/skills/" + product + "/purchase"), 1)
        finally:
            server.shutdown()
            worker.join(timeout=3)
            server.server_close()

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
