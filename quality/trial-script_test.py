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
import time
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

    def test_install_preserves_unconfirmed_pending(self):
        # 三轮评审 F1:use 结果未知留下的 pendingRequestId 必须在重新
        # install 后原样保留,否则下一次 use 换新键、同一次使用被扣两次。
        self._write_state(pendingRequestId="req-survives")
        grant_calls = []

        def grant_api(market, method, path, body=None):
            if path.endswith("/trial-grants"):
                grant_calls.append(body["installId"])
                return {"installId": body["installId"], "limitUses": 5, "remainingUses": 5}
            raise AssertionError("unexpected %s %s" % (method, path))

        with mock.patch.object(trial, "api_request", side_effect=grant_api):
            trial.ensure_trial_grant("cn", PRODUCT_ID)
        self.assertEqual(grant_calls, ["install-1"])
        state = trial.load_trial_state(PRODUCT_ID)
        self.assertEqual(state["pendingRequestId"], "req-survives")

        # 换新凭证(installId 变化)时旧键无法在新 grant 上回放,允许丢弃。
        with mock.patch.object(trial, "api_request", side_effect=lambda market, method, path, body=None:
                               {"installId": "install-2", "limitUses": 5, "remainingUses": 5, "secret": "s2"}):
            os.remove(trial.trial_state_path(PRODUCT_ID))
            self._write_state(install_id="install-old", pendingRequestId="req-old")
            trial.ensure_trial_grant("cn", PRODUCT_ID)
        self.assertNotIn("pendingRequestId", trial.load_trial_state(PRODUCT_ID))

    def test_save_trial_state_never_exposes_torn_writes(self):
        # 三轮评审 F2:Python 与 Go 共改同一 JSON,写入必须原子替换——
        # 并发读者要么看到旧文件要么看到新文件,永远不会读到半份 JSON。
        import threading

        self._write_state()
        stop = threading.Event()
        errors = []

        def writer():
            flip = False
            while not stop.is_set():
                flip = not flip
                state = {"installId": "install-1", "secret": "secret-1", "productId": PRODUCT_ID, "market": "cn"}
                if flip:
                    state["pendingRequestId"] = "req-%d" % threading.get_ident()
                try:
                    trial.save_trial_state(PRODUCT_ID, state)
                except Exception as error:  # noqa: BLE001
                    errors.append(error)
                    return

        reader_stop = threading.Event()

        def reader():
            while not reader_stop.is_set():
                state = trial.load_trial_state(PRODUCT_ID)
                # None 仅允许出现在"尚未写入"窗口;一旦存在必须是完整合法状态。
                if state is None and os.path.exists(trial.trial_state_path(PRODUCT_ID)):
                    errors.append(AssertionError("torn or missing state observed"))
                    return

        threads = [threading.Thread(target=writer), threading.Thread(target=reader)]
        for thread in threads:
            thread.start()
        time.sleep(0.5)
        stop.set()
        reader_stop.set()
        for thread in threads:
            thread.join()
        self.assertFalse(errors, str(errors))

    def test_state_dir_permission_failure_is_structured(self):
        # 三轮评审 F3:状态目录不可写必须输出结构化 Failure,
        # 不得打印 Python 堆栈。
        def denied_makedirs(path, mode=None, exist_ok=False):
            raise PermissionError("denied")

        with mock.patch.object(trial.os, "makedirs", side_effect=denied_makedirs):
            with self.assertRaises(trial.Failure) as caught:
                with trial.ProductLock(PRODUCT_ID):
                    pass
        self.assertEqual(caught.exception.code, "STATE_DIR_PERMISSION_DENIED")

    def test_api_network_errors_never_leak_reason(self):
        # 五轮评审 P1:api_request 的 URLError 分支与下载分支同规——
        # reason 可能携带代理地址/凭证/本机路径,只保留异常类型名。
        import urllib.error

        denial = urllib.error.URLError(OSError("proxy corp.local:3128 /Users/private/.viceme"))
        with mock.patch.object(trial.urllib.request, "urlopen", side_effect=denial):
            with self.assertRaises(trial.Failure) as caught:
                trial.api_request("cn", "GET", "/v1/skills/%s/access" % PRODUCT_ID)
        self.assertEqual(caught.exception.code, "NETWORK_ERROR")
        emitted = caught.exception.message + json.dumps(caught.exception.fields, default=str)
        self.assertNotIn("corp.local", emitted)
        self.assertNotIn("Users", emitted)

    def test_api_http_error_omits_non_json_bodies(self):
        # 网关/代理错误页(非 JSON)可能携带内部主机名,不得原样回显。
        import urllib.error

        body = io.StringIO("<html>gateway internalhost.corp /Users/private</html>")
        error = urllib.error.HTTPError("https://api.invalid/x", 502, "Bad Gateway", {}, body)  # type: ignore[arg-type]
        with mock.patch.object(trial.urllib.request, "urlopen", side_effect=error):
            with self.assertRaises(trial.Failure) as caught:
                trial.api_request("cn", "GET", "/v1/skills/%s/access" % PRODUCT_ID)
        self.assertEqual(caught.exception.code, "API_ERROR")
        emitted = caught.exception.message + json.dumps(caught.exception.fields, default=str)
        self.assertNotIn("internalhost", emitted)
        self.assertNotIn("<html>", emitted)
        self.assertNotIn("Users", emitted)

    def test_api_http_error_keeps_canonical_json_messages(self):
        # 标准错误契约的 message 面向客户端,应当保留。
        import urllib.error

        body = io.StringIO('{"statusCode":403,"code":"FORBIDDEN","message":"该款不可购买"}')
        error = urllib.error.HTTPError("https://api.invalid/x", 403, "Forbidden", {}, body)  # type: ignore[arg-type]
        with mock.patch.object(trial.urllib.request, "urlopen", side_effect=error):
            with self.assertRaises(trial.Failure) as caught:
                trial.api_request("cn", "GET", "/v1/skills/%s/access" % PRODUCT_ID)
        self.assertEqual(caught.exception.code, "API_ERROR")
        self.assertIn("该款不可购买", caught.exception.message)

    def test_download_errors_never_leak_the_signed_url(self):
        # 四轮评审 P1:下载地址是短期签名凭证(X-Amz-Signature 等),
        # 错误输出不得携带 URL——那会进 AI 对话与日志。
        signed = "https://storage.invalid/pro.zip?X-Amz-Signature=deadbeef&X-Amz-Credential=AKIA%2F20260904"
        response = io.StringIO("{}")
        import urllib.error

        error = urllib.error.HTTPError(signed, 403, "Forbidden", {}, response)  # type: ignore[arg-type]
        with mock.patch.object(trial.urllib.request, "urlopen", side_effect=error):
            with self.assertRaises(trial.Failure) as caught:
                trial.http_download(signed)
        failure = caught.exception
        self.assertEqual(failure.code, "DOWNLOAD_ERROR")
        self.assertNotIn("url", failure.fields)
        self.assertNotIn("X-Amz-Signature", failure.message)
        self.assertNotIn("storage.invalid", failure.message + json.dumps(failure.fields, default=str))

        # URLError 分支同样只留异常类型名,不带原始 reason 文字。
        denial = urllib.error.URLError(OSError("proxy corp.local:3128 /Users/secret/.viceme"))
        with mock.patch.object(trial.urllib.request, "urlopen", side_effect=denial):
            with self.assertRaises(trial.Failure) as caught:
                trial.http_download(signed)
        self.assertEqual(caught.exception.code, "NETWORK_ERROR")
        self.assertNotIn("corp.local", caught.exception.message)
        self.assertNotIn("Users", caught.exception.message)

    def test_unexpected_error_output_never_leaks_local_details(self):
        # 兜底输出:原始异常文字(含 HOME 路径)不得出现在单行 JSON 里。
        self._write_state()

        def exploding_api(market, method, path, body=None):
            raise FileNotFoundError("/Users/secret/.viceme/trial/leak.json")

        with mock.patch.object(trial, "api_request", side_effect=exploding_api):
            output = io.StringIO()
            with redirect_stdout(output):
                exit_code = trial.run(["use", "--product", PRODUCT_ID, "--market", "cn"])
        self.assertEqual(exit_code, 1)
        result = json.loads(output.getvalue().strip())
        self.assertEqual(result["code"], "UNEXPECTED_ERROR")
        self.assertNotIn("/Users/secret", output.getvalue())
        self.assertNotIn(self.home, output.getvalue())

    def test_state_permission_failures_omit_home_paths(self):
        def denied_makedirs(path, mode=None, exist_ok=False):
            raise PermissionError("denied")

        with mock.patch.object(trial.os, "makedirs", side_effect=denied_makedirs):
            with self.assertRaises(trial.Failure) as caught:
                with trial.ProductLock(PRODUCT_ID):
                    pass
        self.assertEqual(caught.exception.code, "STATE_DIR_PERMISSION_DENIED")
        self.assertNotIn(self.home, caught.exception.message)

    def test_unexpected_exception_still_emits_single_line_json(self):
        # 脚本契约:任何异常都以单行 JSON 收场。
        captured = []

        def exploding_api(market, method, path, body=None):
            raise RuntimeError("boom")

        self._write_state()
        with mock.patch.object(trial, "api_request", side_effect=exploding_api):
            output = io.StringIO()
            with redirect_stdout(output):
                exit_code = trial.run(["use", "--product", PRODUCT_ID, "--market", "cn"])
        self.assertEqual(exit_code, 1)
        lines = [line for line in output.getvalue().splitlines() if line.strip()]
        self.assertEqual(len(lines), 1)
        result = json.loads(lines[0])
        self.assertFalse(result["ok"])
        self.assertEqual(result["code"], "UNEXPECTED_ERROR")
        self.assertNotIn("boom", result["message"])

    def test_state_file_permissions_and_roundtrip(self):
        trial.save_trial_state(PRODUCT_ID, {"installId": "i", "secret": "s"})
        self.assertEqual(trial.load_trial_state(PRODUCT_ID)["secret"], "s")
        mode = stat.S_IMODE(os.stat(trial.trial_state_path(PRODUCT_ID)).st_mode)
        self.assertEqual(oct(mode), "0o600")

    # ------------------------------------------------------------------
    # F3 回归:锁内重读 + 未确认幂等键重放
    # ------------------------------------------------------------------

    def _write_state(self, install_id="install-1", **extra):
        state = {"installId": install_id, "secret": "secret-1", "productId": PRODUCT_ID, "market": "cn"}
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

    def test_lock_treats_shared_violation_as_contended_and_times_out(self):
        # Windows 共享冲突:os.open 报 PermissionError 但锁文件存在 →
        # 按"被占"重试,并在有界时间内超时,而不是卡死或空转。
        import types

        lock_path = trial.trial_state_path(PRODUCT_ID) + ".lock"
        os.makedirs(os.path.dirname(lock_path), mode=0o700, exist_ok=True)
        with open(lock_path, "w", encoding="utf-8") as handle:
            handle.write("other-pid")
        fresh = trial.time.time()
        real_stat = trial.os.stat

        def fresh_lock_stat(path, *args, **kwargs):
            if str(path).endswith(".lock"):
                return types.SimpleNamespace(st_mtime=fresh)
            return real_stat(path, *args, **kwargs)

        with mock.patch.object(trial.os, "open", side_effect=PermissionError("sharing violation")), \
                mock.patch.object(trial.os, "stat", side_effect=fresh_lock_stat), \
                mock.patch.object(trial, "LOCK_WAIT_SECONDS", 0.4):
            started = trial.time.time()
            with self.assertRaises(trial.Failure) as caught:
                with trial.ProductLock(PRODUCT_ID):
                    pass
        self.assertEqual(caught.exception.code, "STATE_LOCK_BUSY")
        self.assertLess(trial.time.time() - started, 5, "must time out promptly")

    def test_lock_fails_fast_when_creation_is_not_permitted(self):
        # 无权创建(目录不可写):锁文件不存在,必须立即报权限错误,
        # 不得进入重试循环。
        real_stat = trial.os.stat

        def stat_missing(path, *args, **kwargs):
            if str(path).endswith(".lock"):
                raise FileNotFoundError(path)
            return real_stat(path, *args, **kwargs)

        with mock.patch.object(trial.os, "open", side_effect=PermissionError("denied")), \
                mock.patch.object(trial.os, "stat", side_effect=stat_missing):
            started = trial.time.time()
            with self.assertRaises(trial.Failure) as caught:
                with trial.ProductLock(PRODUCT_ID):
                    pass
        self.assertEqual(caught.exception.code, "STATE_LOCK_PERMISSION_DENIED")
        self.assertLess(trial.time.time() - started, 2, "must fail fast")

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
