import hashlib
import json
import os
import subprocess
import sys
import tempfile
import unittest
import zipfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "skills" / "build-your-work" / "scripts" / "package_creation.py"


class BuildYourWorkPackagerTest(unittest.TestCase):
    def project(self):
        temporary = tempfile.TemporaryDirectory()
        root = Path(temporary.name)
        package = root / "viceme-dist" / "staging" / "package-root"
        (package / "agents").mkdir(parents=True)
        (package / "site").mkdir()
        (package / "SKILL.md").write_text(
            "---\nname: demo-work\ndescription: Build a demo work.\n---\n\n# Demo\n",
            encoding="utf-8",
        )
        (package / "agents" / "openai.yaml").write_text(
            'interface:\n  display_name: "Demo"\n  short_description: "Build a complete demo website work"\n'
            '  default_prompt: "Use $demo-work to build my version."\n',
            encoding="utf-8",
        )
        (package / "site" / "index.html").write_text("<!doctype html><title>Demo</title>", encoding="utf-8")
        return temporary, root, package

    def run_script(self, root, *extra):
        command = [
            sys.executable,
            os.fspath(SCRIPT),
            "--root",
            "viceme-dist/staging/package-root",
            "--output",
            "viceme-dist/package.zip",
            *extra,
        ]
        return subprocess.run(command, cwd=root, text=True, capture_output=True, check=False)

    def test_writes_a_deterministic_rooted_archive(self):
        temporary, root, _ = self.project()
        self.addCleanup(temporary.cleanup)

        first = self.run_script(root)
        self.assertEqual(first.returncode, 0, first.stderr)
        first_result = json.loads(first.stdout)
        archive = root / "viceme-dist" / "package.zip"
        first_digest = hashlib.sha256(archive.read_bytes()).hexdigest()

        second = self.run_script(root)
        self.assertEqual(second.returncode, 0, second.stderr)
        second_result = json.loads(second.stdout)
        self.assertEqual(hashlib.sha256(archive.read_bytes()).hexdigest(), first_digest)
        self.assertEqual(first_result["sha256"], second_result["sha256"])
        with zipfile.ZipFile(archive) as package:
            self.assertEqual(
                package.namelist(),
                ["SKILL.md", "agents/openai.yaml", "site/index.html"],
            )
            self.assertNotIn("demo-work/SKILL.md", package.namelist())

    def test_rejects_a_suspected_secret_without_replacing_output(self):
        temporary, root, package = self.project()
        self.addCleanup(temporary.cleanup)
        output = root / "viceme-dist" / "package.zip"
        output.write_bytes(b"previous-good-package")
        (package / "secret.txt").write_text("api_key='real-production-secret-value'\n", encoding="utf-8")

        result = self.run_script(root)

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("secret.txt", result.stderr)
        self.assertEqual(output.read_bytes(), b"previous-good-package")

    def test_rejects_symbolic_links(self):
        temporary, root, package = self.project()
        self.addCleanup(temporary.cleanup)
        target = package / "site" / "index.html"
        try:
            (package / "site" / "linked.html").symlink_to(target)
        except (OSError, NotImplementedError) as error:
            self.skipTest(f"symbolic links unavailable: {error}")

        result = self.run_script(root)

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("symbolic links are forbidden", result.stderr)

    def test_requires_exact_project_relative_paths(self):
        temporary, root, _ = self.project()
        self.addCleanup(temporary.cleanup)
        command = [
            sys.executable,
            os.fspath(SCRIPT),
            "--root",
            os.fspath(root / "viceme-dist" / "staging" / "package-root"),
            "--output",
            "viceme-dist/package.zip",
        ]

        result = subprocess.run(command, cwd=root, text=True, capture_output=True, check=False)

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("project-relative", result.stderr)

    def test_rejects_a_symlinked_viceme_dist(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        root = Path(temporary.name) / "project"
        outside = Path(temporary.name) / "outside"
        root.mkdir()
        (outside / "staging" / "package-root").mkdir(parents=True)
        try:
            (root / "viceme-dist").symlink_to(outside, target_is_directory=True)
        except (OSError, NotImplementedError) as error:
            self.skipTest(f"symbolic links unavailable: {error}")

        result = self.run_script(root)

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("must not traverse a symbolic link", result.stderr)


if __name__ == "__main__":
    unittest.main()
