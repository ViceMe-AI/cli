#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""skills/use-a-skill/scripts/trial.py 的行为测试(unittest,零依赖)。

运行:python3 quality/trial-script_test.py(make check 已接线)。

重点回归:
- use 必须在进程锁内重读状态:锁外快照错过其他进程写入的未确认幂等键时,
  沿用快照会对同一使用生成新 requestId、重复扣次(评审 F3)。
- 所有安装目标都被拒绝覆盖时必须报错,不得谎报安装成功(评审 F4)。
- 锁的 O_EXCL 争抢在 Windows 可能以 PermissionError 冒出,须按"锁被占"处理(评审 F5)。
"""

import importlib.util
import io
import json
import os
import stat
import sys
import tempfile
import unittest
import urllib.parse
import zipfile
from contextlib import redirect_stdout
from unittest import mock

REPOSITORY_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
SCRIPT_PATH = os.path.join(REPOSITORY_ROOT, "skills", "use-a-skill", "scripts", "trial.py")

spec = importlib.util.spec_from_file_location("viceme_trial_script", SCRIPT_PATH)
trial = importlib.util.module_from_spec(spec)
sys.modules["viceme_trial_script"] = trial
spec.loader.exec_module(trial)

PRODUCT_ID = "33709ab2-2246-4033-a41e-7b21d96bccb7"


class TrialScriptTestCase(unittest.TestCase):
    def setUp(self):
        self.home = tempfile.mkdtemp(prefix="viceme-trial-test-")
        self._home_patcher = mock.patch.dict(os.environ, {"HOME": self.home})
        self._home_patcher.start()
        self.addCleanup(self._home_patcher.stop)

    # ------------------------------------------------------------------
    # 纯函数
    # ------------------------------------------------------------------

    def test_slugify_matches_cli_semantics(self):
        self.assertEqual(trial.slugify("Canghe Article Illustrator!"), "canghe-article-illustrator")
        self.assertEqual(trial.slugify("中文标题"), "")
        self.assertEqual(trial.slugify("  A -- B  "), "a-b")

    def test_frontmatter_name_extraction(self):
        files = {"SKILL.md": (b"---\nname: my-skill\ndescription: x\n---\n\nbody", 0o644)}
        self.assertEqual(trial.frontmatter_name(files), "my-skill")
        quoted = {"SKILL.md": (b'---\nname: "quoted name"\n---\nbody', 0o644)}
        self.assertEqual(trial.frontmatter_name(quoted), "quoted name")
        plain = {"SKILL.md": (b"no frontmatter", 0o644)}
        self.assertEqual(trial.frontmatter_name(plain), "")

    def test_gate_injection_is_idempotent_and_anchored(self):
        files = {"SKILL.md": (b"---\nname: my-skill\n---\n\nbody", 0o644)}
        trial.inject_trial_gate(files, "cn", PRODUCT_ID)
        content = files["SKILL.md"][0].decode("utf-8")
        self.assertIn(trial.GATE_MARKER + " product=%s -->" % PRODUCT_ID, content)
        self.assertIn(trial.GATE_TAIL, content)
        self.assertIn("python3 - use --product %s --market cn" % PRODUCT_ID, content)
        # 门禁必须位于 frontmatter 之后、正文之前。
        self.assertLess(content.index("---\n", 4), content.index(trial.GATE_MARKER))
        once = files["SKILL.md"][0]
        trial.inject_trial_gate(files, "cn", PRODUCT_ID)
        self.assertEqual(files["SKILL.md"][0], once)

    def test_gate_rule_four_guides_cli_conversion_honestly(self):
        files = {"SKILL.md": (b"no frontmatter", 0o644)}
        trial.inject_trial_gate(files, "cn", PRODUCT_ID)
        content = files["SKILL.md"][0].decode("utf-8")
        # 脚本路无法安装已购版本:第 4 条必须引导安装 CLI 转正,不得声称
        # 「重新运行本命令即可转正」。
        self.assertIn("viceme skill install %s" % PRODUCT_ID, content)
        self.assertNotIn("按同一命令的输出转正", content)

    def test_zip_extraction_rejects_traversal_and_missing_manifest(self):
        evil = io.BytesIO()
        with zipfile.ZipFile(evil, "w") as archive:
            archive.writestr("SKILL.md", "x")
            archive.writestr("../evil.txt", "pwn")
        with self.assertRaises(trial.Failure) as caught:
            trial.extract_skill_package(evil.getvalue())
        self.assertEqual(caught.exception.code, "ARCHIVE_UNSAFE")

        empty = io.BytesIO()
        with zipfile.ZipFile(empty, "w") as archive:
            archive.writestr("README.md", "x")
        with self.assertRaises(trial.Failure) as caught:
            trial.extract_skill_package(empty.getvalue())
        self.assertEqual(caught.exception.code, "MANIFEST_MISSING")

    def test_state_file_permissions_and_roundtrip(self):
        trial.save_trial_state(PRODUCT_ID, {"installId": "i", "secret": "s"})
        self.assertEqual(trial.load_trial_state(PRODUCT_ID)["secret"], "s")
        mode = stat.S_IMODE(os.stat(trial.trial_state_path(PRODUCT_ID)).st_mode)
        self.assertEqual(oct(mode), "0o600")

    # ------------------------------------------------------------------
    # F3 回归:锁内重读 + 未确认幂等键重放
    # ------------------------------------------------------------------

    def _write_state(self, **extra):
        state = {"installId": "install-1", "secret": "secret-1", "productId": PRODUCT_ID, "market": "cn"}
        state.update(extra)
        trial.save_trial_state(PRODUCT_ID, state)

    def test_use_replays_unconfirmed_request_id(self):
        self._write_state(pendingRequestId="req-1")
        captured = []

        def fake_api(market, method, path, body=None):
            captured.append((path, body))
            return {"allowed": True, "remainingUses": 2, "limitUses": 5}

        with mock.patch.object(trial, "api_request", side_effect=fake_api):
            output = io.StringIO()
            with redirect_stdout(output):
                trial.command_use("cn", PRODUCT_ID)
        path, body = captured[-1]
        self.assertEqual(body["requestId"], "req-1")
        result = json.loads(output.getvalue())
        self.assertTrue(result["allowed"])
        # 权威结果送达后未确认键必须清除。
        self.assertNotIn("pendingRequestId", trial.load_trial_state(PRODUCT_ID))

    def test_use_reloads_state_inside_the_product_lock(self):
        # 模拟评审 F3 的竞态:锁外快照(第一次 load)没有 pendingRequestId,
        # 而磁盘上的权威状态带着另一个进程刚写入的未确认键。锁内必须重读并
        # 重放该键;沿用锁外快照会生成新键、对同一使用重复扣次。
        self._write_state(pendingRequestId="req-from-other-process")
        real_load = trial.load_trial_state
        loads = []

        def racing_load(product_id):
            loads.append(product_id)
            if len(loads) == 1:
                return {"installId": "install-1", "secret": "secret-1"}
            return real_load(product_id)

        captured = []

        def fake_api(market, method, path, body=None):
            captured.append(body)
            return {"allowed": True, "remainingUses": 2, "limitUses": 5}

        with mock.patch.object(trial, "load_trial_state", side_effect=racing_load), \
                mock.patch.object(trial, "api_request", side_effect=fake_api):
            output = io.StringIO()
            with redirect_stdout(output):
                trial.command_use("cn", PRODUCT_ID)
        self.assertGreaterEqual(len(loads), 2, "state must be reloaded inside the lock")
        self.assertEqual(captured[-1]["requestId"], "req-from-other-process")

    def test_use_keeps_pending_when_response_is_lost(self):
        self._write_state(pendingRequestId="req-1")

        def lost_response(market, method, path, body=None):
            raise trial.Failure("NETWORK_ERROR", "lost")

        with mock.patch.object(trial, "api_request", side_effect=lost_response):
            with self.assertRaises(trial.Failure):
                trial.command_use("cn", PRODUCT_ID)
        # 响应丢失:未确认键保留,重试必须复用同一键。
        self.assertEqual(trial.load_trial_state(PRODUCT_ID)["pendingRequestId"], "req-1")

    # ------------------------------------------------------------------
    # F5 回归:Windows 共享冲突按"锁被占"处理
    # ------------------------------------------------------------------

    def test_lock_treats_permission_error_as_contended(self):
        real_open = os.open

        def contended_open(path, flags, *args, **kwargs):
            if path.endswith(".lock") and not getattr(contended_open, "raised", False):
                contended_open.raised = True
                raise PermissionError("sharing violation")
            return real_open(path, flags, *args, **kwargs)

        with mock.patch.object(trial.os, "open", side_effect=contended_open):
            with trial.ProductLock(PRODUCT_ID):
                pass
        self.assertFalse(os.path.exists(trial.trial_state_path(PRODUCT_ID) + ".lock"))

    def test_lock_steals_stale_lock_files(self):
        lock_path = trial.trial_state_path(PRODUCT_ID) + ".lock"
        os.makedirs(os.path.dirname(lock_path), mode=0o700, exist_ok=True)
        with open(lock_path, "w", encoding="utf-8") as handle:
            handle.write("dead-pid")
        stale = time_old_mtime()
        os.utime(lock_path, (stale, stale))
        with trial.ProductLock(PRODUCT_ID):
            self.assertTrue(os.path.exists(lock_path))
        self.assertFalse(os.path.exists(lock_path))


def time_old_mtime():
    import time

    return time.time() - trial.LOCK_STALE_SECONDS - 60


class InstallFlowTestCase(unittest.TestCase):
    """command_install 的端到端桩测试:成功、门禁、凭证与 F4 全跳过报错。"""

    def setUp(self):
        self.home = tempfile.mkdtemp(prefix="viceme-trial-install-")
        self._home_patcher = mock.patch.dict(os.environ, {"HOME": self.home})
        self._home_patcher.start()
        self.addCleanup(self._home_patcher.stop)
        self.archive = io.BytesIO()
        with zipfile.ZipFile(self.archive, "w") as archive:
            info = zipfile.ZipInfo("scripts/run.sh")
            info.external_attr = 0o100755 << 16
            archive.writestr(info, b"echo hi")
            archive.writestr("SKILL.md", "---\nname: my-skill\ndescription: d\n---\n\nbody")
        self.archive_bytes = self.archive.getvalue()
        import hashlib

        self.digest = hashlib.sha256(self.archive_bytes).hexdigest()
        self.release_id = "release-1"

    def _api(self, market, method, path, body=None):
        quoted = urllib.parse.quote(PRODUCT_ID, safe="")
        pure, _, query = path.partition("?")
        if pure == "/v1/skills/%s/access" % quoted:
            return {
                "productId": PRODUCT_ID,
                "isFree": False,
                "owned": False,
                "purchaseAvailable": True,
                "trial": {"available": True, "limitUses": 5},
                "edition": {"key": "pro", "title": "专业版", "sortOrder": 0, "highlights": []},
                "release": {"id": self.release_id, "artifactDigest": self.digest, "fileName": "pro.zip"},
            }
        if pure == "/v1/skills/%s/trial-grants" % quoted:
            return {
                "installId": body["installId"],
                "limitUses": 5,
                "remainingUses": 5,
                "secret": "grant-secret",
            }
        if pure == "/v1/downloads/trial/%s" % quoted:
            self.assertIn("installId=", query)
            return {
                "url": "https://storage.invalid/pro.zip",
                "fileName": "pro.zip",
                "releaseId": self.release_id,
                "artifactDigest": self.digest,
                "expiresAt": "2027-01-01T00:00:00Z",
            }
        raise AssertionError("unexpected API call %s %s" % (method, path))

    def test_install_success_writes_gate_manifest_and_credential(self):
        with mock.patch.object(trial, "api_request", side_effect=self._api), \
                mock.patch.object(trial, "http_download", return_value=self.archive_bytes):
            output = io.StringIO()
            with redirect_stdout(output):
                exit_code = trial.main(["install", "--product", PRODUCT_ID, "--market", "cn"])
        self.assertEqual(exit_code, 0)
        result = json.loads(output.getvalue())
        self.assertTrue(result["ok"])
        self.assertEqual(result["kind"], "trial")
        skill_dir = os.path.join(self.home, ".agents", "skills", "my-skill")
        with open(os.path.join(skill_dir, "SKILL.md"), encoding="utf-8") as handle:
            content = handle.read()
        self.assertIn(trial.GATE_MARKER + " product=%s -->" % PRODUCT_ID, content)
        with open(os.path.join(skill_dir, ".viceme", "install-manifest.json"), encoding="utf-8") as handle:
            manifest = json.load(handle)
        self.assertEqual(manifest["product_id"], PRODUCT_ID)
        self.assertEqual(manifest["release_id"], self.release_id)
        self.assertEqual(trial.load_trial_state(PRODUCT_ID)["secret"], "grant-secret")
        self.assertEqual(oct(stat.S_IMODE(os.stat(trial.trial_state_path(PRODUCT_ID)).st_mode)), "0o600")

    def test_install_reports_failure_when_every_target_is_skipped(self):
        # 唯一目标 .agents/skills/my-skill 已被非 ViceMe 管理目录占用:
        # 一个文件都没落盘时必须报 INSTALL_FAILED,不得谎报成功。
        foreign = os.path.join(self.home, ".agents", "skills", "my-skill")
        os.makedirs(foreign)
        with open(os.path.join(foreign, "README.md"), "w", encoding="utf-8") as handle:
            handle.write("user content")
        with mock.patch.object(trial, "api_request", side_effect=self._api), \
                mock.patch.object(trial, "http_download", return_value=self.archive_bytes):
            output = io.StringIO()
            with redirect_stdout(output):
                try:
                    exit_code = trial.main(["install", "--product", PRODUCT_ID, "--market", "cn"])
                except trial.Failure as failure:
                    exit_code = trial.emit_failure(failure)
        self.assertEqual(exit_code, 1)
        result = json.loads(output.getvalue())
        self.assertFalse(result["ok"])
        self.assertEqual(result["code"], "INSTALL_FAILED")
        self.assertTrue(result["skipped"])
        with open(os.path.join(foreign, "README.md"), encoding="utf-8") as handle:
            self.assertEqual(handle.read(), "user content")


if __name__ == "__main__":
    unittest.main(verbosity=2)
