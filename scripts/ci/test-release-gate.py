"""Real Git history tests for independent release admission and generated trees."""
import os
import json
import pathlib
import shutil
import subprocess
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[2]

class ReleaseGateTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = pathlib.Path(self.tmp.name)
        self.repo = self.root / "repo"
        self.repo.mkdir()
        self.env = dict(os.environ, GITHUB_EVENT_NAME="pull_request", PR_BASE_REF="main",
                        PR_HEAD_REF="feat(cli)/example", PR_HEAD_REPOSITORY="example/cli",
                        GITHUB_REPOSITORY="example/cli")
        self.git("init", "-b", "main")
        self.git("config", "user.name", "Test")
        self.git("config", "user.email", "test@example.com")
        shutil.copytree(ROOT / "scripts/ci", self.repo / "scripts/ci")
        (self.repo / "package.json").write_text('{"version":"1.0.0"}\n')
        (self.repo / "Makefile").write_text('release-prepare:\n\tprintf \'{"version":"1.0.1"}\\n\' > package.json\n')
        self.git("add", ".")
        self.git("commit", "-m", "initial")
        self.git("branch", "dev")
        remote = self.root / "remote.git"
        self.run_cmd(["git", "clone", "--bare", str(self.repo), str(remote)])
        self.git("remote", "add", "origin", str(remote))
        self.parent = self.git("rev-parse", "HEAD").stdout.strip()
        self.env["PR_HEAD_SHA"] = self.parent

    def run_cmd(self, args, ok=True):
        result = subprocess.run(args, cwd=self.repo, env=self.env, text=True,
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        if ok and result.returncode:
            self.fail(f"{args}: {result.stderr} {result.stdout}")
        return result

    def git(self, *args):
        return self.run_cmd(["git", *args])

    def gate(self):
        return self.run_cmd(["bash", "scripts/ci/validate-pr-target.sh"], ok=False)

    def prepared(self):
        self.run_cmd(["make", "release-prepare"])
        self.git("add", "package.json")
        self.git("-c", "user.email=123+viceme-release-bot[bot]@users.noreply.github.com",
                 "commit", "-m", "chore(release): v1.0.1", "-m",
                 "ViceMe-Release-Prepared: true\nViceMe-Release-Run: https://github.com/example/cli/actions/runs/123\nViceMe-Release-Date: 2026-09-22")
        self.env["PR_HEAD_SHA"] = self.git("rev-parse", "HEAD").stdout.strip()
        bindir = self.root / "bin"
        bindir.mkdir()
        gh = bindir / "gh"
        gh.write_text("#!/bin/sh\nprintf '%s\\n' '" + json.dumps(dict(path=".github/workflows/release-pr.yml", event="pull_request", head_sha=self.parent, status="completed", conclusion="success")) + "'\n")
        gh.chmod(0o755)
        self.env["PATH"] = str(bindir) + os.pathsep + self.env["PATH"]

    def verify(self):
        return self.run_cmd(["bash", "scripts/ci/verify-prepared-release.sh"], ok=False)

    def test_accepted_feature_and_bulk(self):
        self.assertEqual(self.gate().returncode, 0)
        self.env["PR_HEAD_REF"] = "dev"
        self.assertEqual(self.gate().returncode, 0)

    def test_unaccepted_business_commit(self):
        self.git("commit", "--allow-empty", "-m", "feat(cli): untested")
        self.env["PR_HEAD_SHA"] = self.git("rev-parse", "HEAD").stdout.strip()
        self.assertNotEqual(self.gate().returncode, 0)

    def test_fork_and_integration(self):
        self.env["PR_HEAD_REPOSITORY"] = "fork/cli"
        self.assertNotEqual(self.gate().returncode, 0)
        self.env["PR_HEAD_REPOSITORY"] = "example/cli"
        self.env["PR_HEAD_REF"] = "chore(repo)/integrate-example"
        self.assertNotEqual(self.gate().returncode, 0)

    def test_generated_exception(self):
        self.prepared()
        self.assertEqual(self.gate().returncode, 0)
        result = self.verify()
        self.assertEqual(result.returncode, 0, result.stderr + result.stdout)

    def test_tampered_allowed_file(self):
        self.prepared()
        (self.repo / "package.json").write_text('{"version":"9.0.0","scripts":{"bad":"payload"}}')
        self.git("add", ".")
        self.git("commit", "--amend", "--no-edit")
        self.env["PR_HEAD_SHA"] = self.git("rev-parse", "HEAD").stdout.strip()
        self.assertNotEqual(self.verify().returncode, 0)

    def test_failed_workflow_evidence(self):
        self.prepared()
        gh = self.root / "bin/gh"
        gh.write_text(gh.read_text().replace('"success"', '"failure"'))
        self.assertNotEqual(self.verify().returncode, 0)

    def test_wrong_workflow_source(self):
        self.prepared()
        gh = self.root / "bin/gh"
        gh.write_text(gh.read_text().replace(self.parent, "a" * 40))
        self.assertNotEqual(self.verify().returncode, 0)

    def test_fetches_new_dev_acceptance(self):
        self.git("commit", "--allow-empty", "-m", "feat(cli): pending acceptance")
        self.env["PR_HEAD_SHA"] = self.git("rev-parse", "HEAD").stdout.strip()
        self.assertNotEqual(self.gate().returncode, 0)
        self.git("push", "origin", "HEAD:dev")
        self.assertEqual(self.gate().returncode, 0)

    def test_business_change_disguised_as_bot(self):
        self.prepared()
        (self.repo / "business.go").write_text("unexpected")
        self.git("add", ".")
        self.git("commit", "--amend", "--no-edit")
        self.env["PR_HEAD_SHA"] = self.git("rev-parse", "HEAD").stdout.strip()
        self.assertNotEqual(self.verify().returncode, 0)

if __name__ == "__main__":
    unittest.main()
