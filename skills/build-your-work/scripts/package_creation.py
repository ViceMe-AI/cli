#!/usr/bin/env python3
"""Validate a prepared Creation Skill tree and write a deterministic ZIP."""

from __future__ import annotations

import argparse
import hashlib
import json
import math
import os
import re
import shutil
import stat
import sys
import tempfile
import time
import zipfile
import unicodedata
from pathlib import Path, PurePosixPath


MIB = 1024 * 1024
MAX_ARCHIVE_BYTES = 50 * MIB
MAX_EXPANDED_BYTES = 50 * MIB
MAX_FILE_BYTES = 50 * MIB
MAX_FILES = 1000
MAX_PATH_BYTES = 512
MAX_COMPRESSION_RATIO = 200
COMPRESSION_RATIO_MIN_BYTES = 1 * MIB
ZIP_EPOCH = (1980, 1, 1, 0, 0, 0)
SKILL_NAME = re.compile(r"^[a-z0-9]+(?:-[a-z0-9]+)*$")
PUBLICATION_PREFIX = "viceme-publication"
PUBLICATION_JSON_MAX_BYTES = 64 * 1024
PUBLICATION_JSON_TOTAL_MAX_BYTES = 192 * 1024
PUBLICATION_MEDIA_MAX = 12

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
RESERVED_RUNTIME_PATHS = {"references/entry.md", "references/viceme-runtime.md", ".viceme/trial-body.md"}
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
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--root", help="prepared package root inside viceme-dist/staging")
    mode.add_argument("--update", help="existing viceme-dist/<skill-name>.zip to update")
    parser.add_argument("--publication-root", help="new publication files in viceme-dist/staging/publication-root")
    parser.add_argument("--expected-sha256", help="digest of the ZIP before an update")
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
    for position, line in enumerate(lines[1:end], start=1):
        match = re.match(r"^(name|description):\s*(.*?)\s*$", line)
        if not match:
            continue
        value = match.group(2)
        if value in {"|", "|-", ">", ">-"}:
            block = []
            for continuation in lines[position + 1:end]:
                if continuation and not continuation[0].isspace():
                    break
                block.append(continuation.strip())
            value = ("\n" if value.startswith("|") else " ").join(block)
        elif value.startswith("'") and value.endswith("'"):
            value = value[1:-1].replace("''", "'")
        elif value.startswith('"') and value.endswith('"'):
            try:
                value = json.loads(value)
            except json.JSONDecodeError as error:
                raise PackageError("SKILL.md contains an invalid quoted scalar") from error
        else:
            value = re.sub(r"\s+#.*$", "", value)
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
            if relative in RESERVED_RUNTIME_PATHS:
                raise PackageError(f"path is reserved for the buyer runtime: {relative}")
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
    identities = [unicodedata.normalize("NFKC", relative).casefold() for relative, _, _ in files]
    if len(identities) != len(set(identities)):
        raise PackageError("package contains Unicode/case-colliding paths")
    validate_publication_metadata(root, skill_name)
    return files, expanded_bytes, skill_name


def strict_json(path: Path) -> object:
    data = path.read_bytes()
    if len(data) > PUBLICATION_JSON_MAX_BYTES:
        raise PackageError(f"publication JSON exceeds 64 KiB: {path.name}")

    def unique_pairs(pairs: list[tuple[str, object]]) -> dict[str, object]:
        result: dict[str, object] = {}
        for key, value in pairs:
            if key in result:
                raise PackageError(f"duplicate publication JSON key: {key}")
            result[key] = value
        return result

    def reject_constant(value: str) -> object:
        raise PackageError(f"non-finite publication JSON number: {value}")

    try:
        value = json.loads(data.decode("utf-8"), object_pairs_hook=unique_pairs, parse_constant=reject_constant)
    except (UnicodeDecodeError, json.JSONDecodeError, RecursionError) as error:
        raise PackageError(f"invalid UTF-8 publication JSON: {path.name}") from error

    def check_depth(item: object, depth: int = 0) -> None:
        if depth > 16:
            raise PackageError("publication JSON nesting exceeds 16")
        if isinstance(item, dict):
            for nested in item.values():
                check_depth(nested, depth + 1)
        elif isinstance(item, list):
            for nested in item:
                check_depth(nested, depth + 1)
        elif isinstance(item, float) and not math.isfinite(item):
            raise PackageError("publication JSON contains a non-finite number")

    check_depth(value)
    return value


def publication_text(value: object, limit: int) -> bool:
    return isinstance(value, str) and bool(value.strip()) and len(unicodedata.normalize("NFC", value.strip())) <= limit


def media_type(data: bytes) -> str | None:
    if data.startswith(b"\x89PNG\r\n\x1a\n") or data.startswith(b"\xff\xd8\xff") or data.startswith((b"GIF87a", b"GIF89a")):
        return "image"
    if data.startswith(b"RIFF") and data[8:12] == b"WEBP":
        return "image"
    if data[4:8] == b"ftyp" and data[8:12] in {b"avif", b"avis"}:
        return "image"
    if data[4:8] == b"ftyp" or data.startswith(b"\x1a\x45\xdf\xa3"):
        return "video"
    return None


def validate_publication_sale(sale: object, region: str) -> None:
    if not isinstance(sale, dict) or set(sale) - {"buyout", "trial", "subscription"}:
        raise PackageError("publication sale is invalid")
    buyout = sale.get("buyout")
    trial = sale.get("trial")
    subscription = sale.get("subscription")
    if buyout is not None:
        if not isinstance(buyout, dict) or type(buyout.get("enabled")) is not bool:
            raise PackageError("publication buyout is invalid")
        if buyout["enabled"]:
            if (
                set(buyout) != {"enabled", "currency", "priceMinor"}
                or not isinstance(buyout["currency"], str)
                or buyout["currency"] not in {"CNY", "USD"}
                or type(buyout["priceMinor"]) is not int
                or not 0 <= buyout["priceMinor"] <= 10_000_000
            ):
                raise PackageError("publication buyout is invalid")
            if buyout["currency"] != ("CNY" if region == "CN" else "USD"):
                raise PackageError("publication buyout currency does not match market")
        elif set(buyout) != {"enabled"}:
            raise PackageError("disabled publication buyout must not include a price")
    if trial is not None:
        if not isinstance(trial, dict) or type(trial.get("enabled")) is not bool:
            raise PackageError("publication trial is invalid")
        if trial["enabled"]:
            if (
                set(trial) != {"enabled", "useLimit"}
                or type(trial["useLimit"]) is not int
                or not 2 <= trial["useLimit"] <= 50
            ):
                raise PackageError("publication trial is invalid")
        elif set(trial) != {"enabled"}:
            raise PackageError("disabled publication trial must not include a limit")
    if subscription is not None and (
        not isinstance(subscription, dict)
        or set(subscription) != {"enabled"}
        or type(subscription["enabled"]) is not bool
    ):
        raise PackageError("publication subscription is invalid")
    if isinstance(trial, dict) and trial.get("enabled"):
        paid_buyout = isinstance(buyout, dict) and buyout.get("enabled") and buyout["priceMinor"] > 0
        subscribed = isinstance(subscription, dict) and subscription.get("enabled")
        if not (paid_buyout or subscribed):
            raise PackageError("publication trial requires a paid buyout or subscription")


def validate_publication_metadata(root: Path, skill_name: str) -> None:
    publication = root / PUBLICATION_PREFIX
    if not publication.exists():
        return
    if not publication.is_dir() or publication.is_symlink():
        raise PackageError("viceme-publication must be a regular directory")
    json_paths = list(publication.rglob("*.json"))
    if sum(path.stat().st_size for path in json_paths) > PUBLICATION_JSON_TOTAL_MAX_BYTES:
        raise PackageError("publication JSON total exceeds 192 KiB")
    if not (publication / "manifest.json").is_file():
        raise PackageError("publication manifest.json is missing")
    manifest = strict_json(publication / "manifest.json")
    if not isinstance(manifest, dict) or set(manifest) - {"schemaVersion", "skillName", "marketRegion", "locales", "slug", "sale", "creatorSubscriptionSuggestion", "media"}:
        raise PackageError("publication manifest has unsupported fields")
    if type(manifest.get("schemaVersion")) is not int or manifest["schemaVersion"] != 1 or manifest.get("skillName") != skill_name:
        raise PackageError("publication schema version or Skill name does not match")
    region = manifest.get("marketRegion")
    locales = manifest.get("locales")
    if not isinstance(region, str) or region not in {"CN", "GLOBAL"} or not isinstance(locales, list) or not 1 <= len(locales) <= 2 or not all(isinstance(locale, str) for locale in locales) or len(set(locales)) != len(locales) or any(locale not in {"zh-CN", "en-US"} for locale in locales):
        raise PackageError("publication region or locales are invalid")
    allowed_json = {"manifest.json", *(f"locales/{locale}.json" for locale in locales)}
    if any(path.relative_to(publication).as_posix() not in allowed_json for path in json_paths):
        raise PackageError("publication contains an unlisted JSON file")
    slug = manifest.get("slug")
    if slug is not None and (not isinstance(slug, str) or not 2 <= len(slug) <= 64 or not SKILL_NAME.fullmatch(slug) or slug in {"works", "skills", "manage", "posts", "about"}):
        raise PackageError("publication slug is invalid")
    for locale in locales:
        locale_path = publication / "locales" / f"{locale}.json"
        if not locale_path.is_file():
            raise PackageError(f"publication locale is missing: {locale}")
        content = strict_json(locale_path)
        if not isinstance(content, dict) or set(content) - {"title", "summary", "usageInstructions"}:
            raise PackageError(f"publication locale has unsupported fields: {locale}")
        for key, limit in (("title", 20), ("summary", 100), ("usageInstructions", 2000)):
            if key in content and not publication_text(content[key], limit):
                raise PackageError(f"publication {locale} {key} is invalid")
        if locale == "zh-CN" and "usageInstructions" in content and not re.search(r"[\u3400-\u9fff]", content["usageInstructions"]):
            raise PackageError("Chinese publication usage instructions must contain Chinese text")
    sale = manifest.get("sale")
    if sale is not None:
        validate_publication_sale(sale, region)
    suggestion = manifest.get("creatorSubscriptionSuggestion")
    if suggestion is not None and (not isinstance(suggestion, dict) or set(suggestion) != {"currency", "priceMinor", "duration"} or not isinstance(suggestion["currency"], str) or suggestion["currency"] not in {"CNY", "USD"} or type(suggestion["priceMinor"]) is not int or not 0 <= suggestion["priceMinor"] <= 10_000_000 or not isinstance(suggestion["duration"], str) or suggestion["duration"] not in {"MONTHLY", "INDEFINITE"}):
        raise PackageError("publication subscription suggestion is invalid")
    media = manifest.get("media", [])
    if not isinstance(media, list) or len(media) > PUBLICATION_MEDIA_MAX:
        raise PackageError("publication media list is invalid")
    seen_media: set[str] = set()
    for entry in media:
        if not isinstance(entry, dict) or set(entry) != {"path"} or not isinstance(entry["path"], str):
            raise PackageError("publication media entry is invalid")
        relative = entry["path"]
        if not re.fullmatch(r"media/[A-Za-z0-9][A-Za-z0-9._/-]*", relative) or any(part in {"", ".", ".."} for part in relative.split("/")) or relative in seen_media:
            raise PackageError("publication media path is invalid")
        seen_media.add(relative)
        media_path = publication.joinpath(*relative.split("/"))
        if not media_path.is_file():
            raise PackageError(f"publication media is missing: {relative}")
        data = media_path.read_bytes()
        kind = media_type(data)
        if not kind or not data or len(data) > (10 * MIB if kind == "image" else 50 * MIB):
            raise PackageError(f"publication media type or size is invalid: {relative}")


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


def build(root: Path, dist_root: Path) -> dict[str, object]:
    started = time.perf_counter()
    files, expanded_bytes, skill_name = collect_files(root)
    collected = time.perf_counter()
    output = dist_root / f"{skill_name}.zip"
    if output.is_symlink():
        raise PackageError("output must not be a symbolic link")
    output.parent.mkdir(mode=0o755, parents=True, exist_ok=True)
    descriptor, temporary_name = tempfile.mkstemp(prefix=".package-", suffix=".zip", dir=output.parent)
    os.close(descriptor)
    temporary = Path(temporary_name)
    try:
        write_archive(files, temporary)
        compressed = time.perf_counter()
        validate_archive(temporary, files, expanded_bytes)
        validated = time.perf_counter()
        os.replace(temporary, output)
    finally:
        try:
            temporary.unlink()
        except FileNotFoundError:
            pass

    hasher = hashlib.sha256()
    with output.open("rb") as archive:
        for chunk in iter(lambda: archive.read(1024 * 1024), b""):
            hasher.update(chunk)
    digest = hasher.hexdigest()
    return {
        "ok": True,
        "skill_name": skill_name,
        "output": f"viceme-dist/{skill_name}.zip",
        "file_count": len(files),
        "expanded_bytes": expanded_bytes,
        "zip_bytes": output.stat().st_size,
        "sha256": digest,
        "timings_ms": {
            "collect": round((collected - started) * 1000),
            "compress": round((compressed - collected) * 1000),
            "validate": round((validated - compressed) * 1000),
        },
    }


def archive_digest(path: Path) -> str:
    hasher = hashlib.sha256()
    with path.open("rb") as archive:
        for chunk in iter(lambda: archive.read(1024 * 1024), b""):
            hasher.update(chunk)
    return hasher.hexdigest()


def update_archive(output: Path, publication_root: Path, expected_sha256: str, dist_root: Path) -> dict[str, object]:
    started = time.perf_counter()
    if output.is_symlink() or not output.is_file():
        raise PackageError("existing ZIP must be a regular file")
    if not re.fullmatch(r"[0-9a-f]{64}", expected_sha256) or archive_digest(output) != expected_sha256:
        raise PackageError("existing ZIP digest differs from --expected-sha256; re-read it before updating")
    if not publication_root.is_dir() or publication_root.is_symlink():
        raise PackageError("--publication-root must be a regular directory")
    with tempfile.TemporaryDirectory(prefix=".publication-update-", dir=dist_root) as temporary_name:
        root = Path(temporary_name) / "package-root"
        root.mkdir()
        if output.stat().st_size > MAX_ARCHIVE_BYTES:
            raise PackageError("existing ZIP exceeds 50 MiB")
        with zipfile.ZipFile(output) as archive:
            infos = archive.infolist()
            if len(infos) > MAX_FILES or archive.comment:
                raise PackageError("existing ZIP exceeds package limits")
            identities: set[str] = set()
            total = 0
            for info in infos:
                name = info.filename
                pure = PurePosixPath(name)
                identity = unicodedata.normalize("NFKC", name).casefold()
                if (not name or name.endswith("/") or pure.is_absolute() or "\\" in name or "\x00" in name or any(part in {"", ".", ".."} for part in name.split("/")) or identity in identities or len(name.encode("utf-8")) > MAX_PATH_BYTES or forbidden_path(name) or name in RESERVED_RUNTIME_PATHS or info.flag_bits & 0x1 or info.compress_type not in {zipfile.ZIP_STORED, zipfile.ZIP_DEFLATED}):
                    raise PackageError(f"existing ZIP contains an unsafe entry: {name}")
                identities.add(identity)
                mode = (info.external_attr >> 16) & 0o170000
                if mode not in {0, stat.S_IFREG} or info.file_size > MAX_FILE_BYTES:
                    raise PackageError(f"existing ZIP contains a special or oversized entry: {name}")
                if info.file_size >= COMPRESSION_RATIO_MIN_BYTES and (info.compress_size == 0 or info.file_size / info.compress_size > MAX_COMPRESSION_RATIO):
                    raise PackageError(f"existing ZIP contains an abnormal compression ratio: {name}")
                total += info.file_size
                if total > MAX_EXPANDED_BYTES:
                    raise PackageError("existing ZIP expands beyond 50 MiB")
                destination = root.joinpath(*pure.parts)
                destination.parent.mkdir(parents=True, exist_ok=True)
                data = archive.read(info)
                scan_secrets(name, data)
                destination.write_bytes(data)
        previous_name, _ = parse_frontmatter((root / "SKILL.md").read_bytes())
        if output.name != f"{previous_name}.zip":
            raise PackageError("existing ZIP filename does not match its Skill name")
        for source in sorted(publication_root.rglob("*")):
            relative = source.relative_to(publication_root)
            if source.is_symlink():
                raise PackageError("publication update contains a symbolic link")
            if source.is_dir():
                continue
            if not source.is_file():
                raise PackageError("publication update contains a special file")
            destination = root / PUBLICATION_PREFIX / relative
            destination.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(source, destination)
        files, expanded_bytes, skill_name = collect_files(root)
        if skill_name != previous_name:
            raise PackageError("publication update changed the Skill name")
        descriptor, candidate_name = tempfile.mkstemp(prefix=".package-", suffix=".zip", dir=dist_root)
        os.close(descriptor)
        candidate = Path(candidate_name)
        try:
            write_archive(files, candidate)
            validate_archive(candidate, files, expanded_bytes)
            if archive_digest(output) != expected_sha256:
                raise PackageError("existing ZIP changed during publication update")
            os.replace(candidate, output)
        finally:
            candidate.unlink(missing_ok=True)
    return {
        "ok": True,
        "skill_name": previous_name,
        "output": f"viceme-dist/{previous_name}.zip",
        "file_count": len(files),
        "expanded_bytes": expanded_bytes,
        "zip_bytes": output.stat().st_size,
        "previous_sha256": expected_sha256,
        "sha256": archive_digest(output),
        "timings_ms": {"update": round((time.perf_counter() - started) * 1000)},
    }


def main() -> int:
    try:
        args = parse_args()
        dist_root = require_project_path(
            "viceme-dist",
            PurePosixPath("viceme-dist"),
            "dist root",
            must_exist=False,
        )
        if args.root:
            if args.publication_root or args.expected_sha256:
                raise PackageError("--publication-root and --expected-sha256 require --update")
            root = require_project_path(args.root, PurePosixPath("viceme-dist/staging/package-root"), "--root", must_exist=True)
            if not root.is_dir():
                raise PackageError("--root must be a directory")
            result = build(root, dist_root)
        else:
            if not args.publication_root or not args.expected_sha256:
                raise PackageError("--update requires --publication-root and --expected-sha256")
            expected = PurePosixPath(args.update)
            if expected.parent != PurePosixPath("viceme-dist") or not expected.name.endswith(".zip"):
                raise PackageError("--update must name viceme-dist/<skill-name>.zip")
            output = require_project_path(args.update, expected, "--update", must_exist=True)
            publication_root = require_project_path(args.publication_root, PurePosixPath("viceme-dist/staging/publication-root"), "--publication-root", must_exist=True)
            result = update_archive(output, publication_root, args.expected_sha256, dist_root)
        print(json.dumps(result, ensure_ascii=False, sort_keys=True))
        return 0
    except (OSError, PackageError, zipfile.BadZipFile) as error:
        print(json.dumps({"ok": False, "error": str(error)}, ensure_ascii=False, sort_keys=True), file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
