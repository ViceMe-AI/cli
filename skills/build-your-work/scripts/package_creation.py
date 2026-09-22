#!/usr/bin/env python3
"""Validate a prepared Creation Skill tree and write a deterministic ZIP."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import stat
import sys
import tempfile
import zipfile
from pathlib import Path, PurePosixPath


MIB = 1024 * 1024
MAX_ARCHIVE_BYTES = 10 * MIB
MAX_EXPANDED_BYTES = 10 * MIB
MAX_FILE_BYTES = 10 * MIB
MAX_FILES = 1000
MAX_PATH_BYTES = 512
MAX_COMPRESSION_RATIO = 1000
COMPRESSION_RATIO_MIN_BYTES = 1 * MIB
ZIP_EPOCH = (1980, 1, 1, 0, 0, 0)
SKILL_NAME = re.compile(r"^[a-z0-9]+(?:-[a-z0-9]+)*$")

FORBIDDEN_SEGMENTS = {
    ".git",
    ".hg",
    ".svn",
    "node_modules",
    ".next",
    ".turbo",
    ".cache",
    "__pycache__",
    "coverage",
    ".venv",
    "venv",
}
FORBIDDEN_NAMES = {".npmrc", ".pypirc", ".DS_Store", "Thumbs.db", "id_rsa", "id_ed25519"}
PRIVATE_KEY_MARKERS = (
    b"-----BEGIN PRIVATE KEY-----",
    b"-----BEGIN RSA PRIVATE KEY-----",
    b"-----BEGIN OPENSSH PRIVATE KEY-----",
    b"-----BEGIN EC PRIVATE KEY-----",
)
TOKEN_PATTERNS = (
    ("AWS access key", re.compile(rb"\b(?:AKIA|ASIA)[A-Z0-9]{16}\b")),
    ("GitHub token", re.compile(rb"\bgh[opsu]_[A-Za-z0-9]{30,}\b")),
    ("OpenAI-style token", re.compile(rb"\bsk-[A-Za-z0-9_-]{20,}\b")),
)
ASSIGNMENT_PATTERN = re.compile(
    r"(?im)\b(?:api[_-]?key|access[_-]?token|auth[_-]?token|client[_-]?secret|password)\b"
    r"\s*[:=]\s*[\"']([^\"'\r\n]{8,})[\"']"
)
PLACEHOLDER_MARKERS = (
    "replace",
    "example",
    "placeholder",
    "your_",
    "your-",
    "dummy",
    "test_",
    "test-",
    "changeme",
    "redacted",
    "masked",
    "xxxx",
    "<",
    "${",
    "***",
)


class PackageError(Exception):
    pass


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", required=True, help="prepared package root inside viceme-dist/staging")
    parser.add_argument("--output", required=True, help="final ZIP path; must be viceme-dist/package.zip")
    return parser.parse_args()


def require_project_path(raw: str, expected: PurePosixPath, label: str, *, must_exist: bool) -> Path:
    if "\\" in raw or os.path.isabs(raw):
        raise PackageError(f"{label} must be a project-relative POSIX path")
    pure = PurePosixPath(raw)
    if pure != expected or any(part in {"", ".", ".."} for part in pure.parts):
        raise PackageError(f"{label} must be exactly {expected.as_posix()}")

    project_root = Path.cwd().resolve()
    dist_root = project_root / "viceme-dist"
    candidate = project_root.joinpath(*pure.parts)
    current = project_root
    for part in pure.parts:
        current = current / part
        if current.is_symlink():
            raise PackageError(f"{label} must not traverse a symbolic link: {current.relative_to(project_root)}")
    resolved = candidate.resolve(strict=must_exist)
    try:
        resolved.relative_to(dist_root.resolve(strict=False))
    except ValueError as error:
        raise PackageError(f"{label} resolves outside viceme-dist") from error
    return resolved


def forbidden_path(relative: str) -> str | None:
    parts = PurePosixPath(relative).parts
    for part in parts:
        if part in FORBIDDEN_SEGMENTS:
            return part
        if part in FORBIDDEN_NAMES or part == ".env" or part.startswith(".env."):
            return part
    return None


def scan_secrets(relative: str, data: bytes) -> None:
    for marker in PRIVATE_KEY_MARKERS:
        if marker in data:
            raise PackageError(f"suspected private key in {relative}")
    for label, pattern in TOKEN_PATTERNS:
        if pattern.search(data):
            raise PackageError(f"suspected {label} in {relative}")

    try:
        text = data.decode("utf-8")
    except UnicodeDecodeError:
        return
    for match in ASSIGNMENT_PATTERN.finditer(text):
        value = match.group(1).strip().lower()
        if not any(marker in value for marker in PLACEHOLDER_MARKERS):
            raise PackageError(f"suspected credential assignment in {relative}")


def parse_frontmatter(data: bytes) -> tuple[str, str]:
    try:
        text = data.decode("utf-8")
    except UnicodeDecodeError as error:
        raise PackageError("SKILL.md must be UTF-8") from error
    lines = text.splitlines()
    if not lines or lines[0].strip() != "---":
        raise PackageError("SKILL.md must start with YAML frontmatter")
    try:
        end = next(index for index, line in enumerate(lines[1:], start=1) if line.strip() == "---")
    except StopIteration as error:
        raise PackageError("SKILL.md frontmatter is not closed") from error

    values: dict[str, str] = {}
    for line in lines[1:end]:
        match = re.match(r"^(name|description):\s*(.*?)\s*$", line)
        if not match:
            continue
        value = match.group(2)
        if len(value) >= 2 and value[0] == value[-1] and value[0] in {"'", '"'}:
            value = value[1:-1]
        values[match.group(1)] = value.strip()

    name = values.get("name", "")
    description = values.get("description", "")
    if not name or len(name) > 64 or not SKILL_NAME.fullmatch(name):
        raise PackageError("SKILL.md name must be 1-64 lowercase letters, digits, or single hyphens")
    if not description or len(description) > 500:
        raise PackageError("SKILL.md description must be present and at most 500 characters")
    return name, description


def collect_files(root: Path) -> tuple[list[tuple[str, Path, int]], int, str]:
    files: list[tuple[str, Path, int]] = []
    expanded_bytes = 0

    def visit(directory: Path, prefix: PurePosixPath) -> None:
        nonlocal expanded_bytes
        with os.scandir(directory) as iterator:
            entries = sorted(iterator, key=lambda entry: entry.name)
        for entry in entries:
            relative_path = prefix / entry.name
            relative = relative_path.as_posix()
            if len(relative.encode("utf-8")) > MAX_PATH_BYTES:
                raise PackageError(f"path exceeds {MAX_PATH_BYTES} UTF-8 bytes: {relative}")
            forbidden = forbidden_path(relative)
            if forbidden:
                raise PackageError(f"forbidden path component {forbidden!r}: {relative}")
            info = entry.stat(follow_symlinks=False)
            if stat.S_ISLNK(info.st_mode):
                raise PackageError(f"symbolic links are forbidden: {relative}")
            if stat.S_ISDIR(info.st_mode):
                visit(Path(entry.path), relative_path)
                continue
            if not stat.S_ISREG(info.st_mode):
                raise PackageError(f"special files are forbidden: {relative}")
            if info.st_size > MAX_FILE_BYTES:
                raise PackageError(f"file exceeds {MAX_FILE_BYTES} bytes: {relative}")
            files.append((relative, Path(entry.path), info.st_size))
            if len(files) > MAX_FILES:
                raise PackageError(f"package contains more than {MAX_FILES} files")
            expanded_bytes += info.st_size
            if expanded_bytes > MAX_EXPANDED_BYTES:
                raise PackageError(f"expanded package exceeds {MAX_EXPANDED_BYTES} bytes")

    visit(root, PurePosixPath())
    skill_files = [item for item in files if PurePosixPath(item[0]).name == "SKILL.md"]
    if len(skill_files) != 1 or skill_files[0][0] != "SKILL.md":
        raise PackageError("package must contain exactly one SKILL.md at the ZIP root")

    skill_name = ""
    for relative, path, _ in files:
        data = path.read_bytes()
        scan_secrets(relative, data)
        if relative == "SKILL.md":
            skill_name, _ = parse_frontmatter(data)
    return files, expanded_bytes, skill_name


def write_archive(files: list[tuple[str, Path, int]], temporary: Path) -> None:
    with zipfile.ZipFile(temporary, "w", compression=zipfile.ZIP_DEFLATED, compresslevel=9) as archive:
        for relative, path, _ in sorted(files, key=lambda item: item[0]):
            info = zipfile.ZipInfo(relative, ZIP_EPOCH)
            info.compress_type = zipfile.ZIP_DEFLATED
            info.create_system = 3
            info.external_attr = (stat.S_IFREG | 0o644) << 16
            with path.open("rb") as source, archive.open(info, "w", force_zip64=False) as destination:
                while True:
                    chunk = source.read(1024 * 1024)
                    if not chunk:
                        break
                    destination.write(chunk)


def validate_archive(path: Path, expected_files: list[tuple[str, Path, int]], expected_total: int) -> None:
    if path.stat().st_size > MAX_ARCHIVE_BYTES:
        raise PackageError(f"ZIP exceeds {MAX_ARCHIVE_BYTES} bytes")
    expected_names = [item[0] for item in sorted(expected_files, key=lambda item: item[0])]
    with zipfile.ZipFile(path, "r") as archive:
        infos = archive.infolist()
        names = [info.filename for info in infos]
        if names != expected_names or len(names) != len(set(names)):
            raise PackageError("ZIP entry order or identity is not deterministic")
        if archive.comment:
            raise PackageError("ZIP comments are forbidden")
        total = 0
        for info in infos:
            pure = PurePosixPath(info.filename)
            if info.flag_bits & 0x1:
                raise PackageError(f"encrypted ZIP entry is forbidden: {info.filename}")
            if info.is_dir() or pure.is_absolute() or "\\" in info.filename or ".." in pure.parts:
                raise PackageError(f"unsafe ZIP entry: {info.filename}")
            if len(info.filename.encode("utf-8")) > MAX_PATH_BYTES:
                raise PackageError(f"ZIP path exceeds {MAX_PATH_BYTES} bytes: {info.filename}")
            mode = (info.external_attr >> 16) & 0o170000
            if mode not in {0, stat.S_IFREG}:
                raise PackageError(f"non-regular ZIP entry is forbidden: {info.filename}")
            if info.file_size > MAX_FILE_BYTES:
                raise PackageError(f"ZIP entry exceeds {MAX_FILE_BYTES} bytes: {info.filename}")
            if info.file_size >= COMPRESSION_RATIO_MIN_BYTES:
                if info.compress_size == 0 or info.file_size / info.compress_size > MAX_COMPRESSION_RATIO:
                    raise PackageError(f"abnormal compression ratio: {info.filename}")
            total += info.file_size
        if total != expected_total or total > MAX_EXPANDED_BYTES:
            raise PackageError("ZIP expanded size does not match the validated source")
        parse_frontmatter(archive.read("SKILL.md"))


def build(root: Path, output: Path) -> dict[str, object]:
    files, expanded_bytes, skill_name = collect_files(root)
    output.parent.mkdir(mode=0o755, parents=True, exist_ok=True)
    descriptor, temporary_name = tempfile.mkstemp(prefix=".package-", suffix=".zip", dir=output.parent)
    os.close(descriptor)
    temporary = Path(temporary_name)
    try:
        write_archive(files, temporary)
        validate_archive(temporary, files, expanded_bytes)
        os.replace(temporary, output)
    finally:
        try:
            temporary.unlink()
        except FileNotFoundError:
            pass

    digest = hashlib.sha256(output.read_bytes()).hexdigest()
    return {
        "ok": True,
        "skill_name": skill_name,
        "output": "viceme-dist/package.zip",
        "file_count": len(files),
        "expanded_bytes": expanded_bytes,
        "zip_bytes": output.stat().st_size,
        "sha256": digest,
    }


def main() -> int:
    try:
        args = parse_args()
        root = require_project_path(
            args.root,
            PurePosixPath("viceme-dist/staging/package-root"),
            "--root",
            must_exist=True,
        )
        if not root.is_dir():
            raise PackageError("--root must be a directory")
        output = require_project_path(
            args.output,
            PurePosixPath("viceme-dist/package.zip"),
            "--output",
            must_exist=False,
        )
        print(json.dumps(build(root, output), ensure_ascii=False, sort_keys=True))
        return 0
    except (OSError, PackageError, zipfile.BadZipFile) as error:
        print(json.dumps({"ok": False, "error": str(error)}, ensure_ascii=False, sort_keys=True), file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
