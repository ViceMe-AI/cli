"""Contract checks for publication metadata inside a single Skill ZIP."""

from __future__ import annotations

import hashlib
import json
import os
import tempfile
import unittest
import zipfile
from pathlib import Path

from package_creation import PackageError, build, update_archive


class PublicationPackageTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.project = Path(self.temporary.name)
        self.previous_cwd = Path.cwd()
        os.chdir(self.project)
        self.addCleanup(os.chdir, self.previous_cwd)
        self.dist = self.project / "viceme-dist"
        self.root = self.dist / "staging" / "package-root"
        self.root.mkdir(parents=True)
        (self.root / "SKILL.md").write_text("---\nname: photo-wall\ndescription: Build a photo wall\n---\n", encoding="utf-8")
        self.publication = self.root / "viceme-publication"
        (self.publication / "locales").mkdir(parents=True)
        self.manifest = {
            "schemaVersion": 1,
            "skillName": "photo-wall",
            "marketRegion": "CN",
            "locales": ["zh-CN"],
        }
        self.write_metadata()

    def write_metadata(self) -> None:
        (self.publication / "manifest.json").write_text(json.dumps(self.manifest), encoding="utf-8")
        if "zh-CN" in self.manifest["locales"]:
            (self.publication / "locales" / "zh-CN.json").write_text(
                json.dumps({"title": "互动照片墙", "summary": "制作自己的照片墙"}, ensure_ascii=False),
                encoding="utf-8",
            )

    def test_first_zip_and_same_path_update(self) -> None:
        first = build(self.root, self.dist)
        output = self.dist / "photo-wall.zip"
        self.assertEqual(first["output"], "viceme-dist/photo-wall.zip")
        update_root = self.dist / "staging" / "publication-root"
        update_root.mkdir()
        (update_root / "locales").mkdir()
        (update_root / "locales" / "zh-CN.json").write_text(
            json.dumps({"title": "照片墙", "summary": "新的简介", "usageInstructions": "请上传照片"}, ensure_ascii=False),
            encoding="utf-8",
        )
        second = update_archive(output, update_root, first["sha256"], self.dist)
        self.assertNotEqual(second["sha256"], first["sha256"])
        self.assertEqual(second["output"], first["output"])
        with zipfile.ZipFile(output) as archive:
            self.assertEqual(json.loads(archive.read("viceme-publication/locales/zh-CN.json"))["usageInstructions"], "请上传照片")
            self.assertEqual(archive.read("SKILL.md"), (self.root / "SKILL.md").read_bytes())

    def test_global_package_keeps_explicit_english_text(self) -> None:
        self.manifest["marketRegion"] = "GLOBAL"
        self.manifest["locales"] = ["en-US"]
        (self.publication / "locales" / "zh-CN.json").unlink()
        (self.publication / "locales" / "en-US.json").write_text(
            json.dumps({"title": "Photo wall", "summary": "Build your own photo wall"}),
            encoding="utf-8",
        )
        self.write_metadata()
        built = build(self.root, self.dist)
        self.assertEqual(built["skill_name"], "photo-wall")
        with zipfile.ZipFile(self.dist / "photo-wall.zip") as archive:
            self.assertEqual(json.loads(archive.read("viceme-publication/locales/en-US.json"))["title"], "Photo wall")

    def test_invalid_update_preserves_previous_zip(self) -> None:
        first = build(self.root, self.dist)
        output = self.dist / "photo-wall.zip"
        update_root = self.dist / "staging" / "publication-root"
        update_root.mkdir()
        (update_root / "manifest.json").write_text('{"schemaVersion":1,"schemaVersion":1}', encoding="utf-8")
        with self.assertRaises(PackageError):
            update_archive(output, update_root, first["sha256"], self.dist)
        self.assertEqual(hashlib.sha256(output.read_bytes()).hexdigest(), first["sha256"])
        with self.assertRaises(PackageError):
            update_archive(output, update_root, "0" * 64, self.dist)

    def test_metadata_rejects_unknown_version_and_media_mismatch(self) -> None:
        self.manifest["schemaVersion"] = 2
        self.write_metadata()
        with self.assertRaises(PackageError):
            build(self.root, self.dist)
        self.manifest["schemaVersion"] = 1
        self.manifest["media"] = [{"path": "media/fake.png"}]
        (self.publication / "media").mkdir()
        (self.publication / "media" / "fake.png").write_bytes(b"not an image")
        self.write_metadata()
        with self.assertRaises(PackageError):
            build(self.root, self.dist)

    def test_trial_and_currency_follow_market_rules(self) -> None:
        self.manifest["sale"] = {"trial": {"enabled": True, "useLimit": 3}}
        self.write_metadata()
        with self.assertRaises(PackageError):
            build(self.root, self.dist)
        self.manifest["sale"] = {"buyout": {"enabled": True, "currency": "USD", "priceMinor": 100}}
        self.write_metadata()
        with self.assertRaises(PackageError):
            build(self.root, self.dist)

    def test_malformed_json_and_case_collisions_are_rejected(self) -> None:
        (self.publication / "manifest.json").write_text('{"schemaVersion":true,"skillName":"photo-wall","marketRegion":"CN","locales":["zh-CN"]}', encoding="utf-8")
        with self.assertRaises(PackageError):
            build(self.root, self.dist)
        self.write_metadata()
        build(self.root, self.dist)
        output = self.dist / "photo-wall.zip"
        with zipfile.ZipFile(output, "a") as archive:
            archive.writestr("Preview.txt", "one")
            archive.writestr("preview.txt", "two")
        update_root = self.dist / "staging" / "publication-root"
        update_root.mkdir()
        with self.assertRaises(PackageError):
            update_archive(output, update_root, hashlib.sha256(output.read_bytes()).hexdigest(), self.dist)


if __name__ == "__main__":
    unittest.main()
