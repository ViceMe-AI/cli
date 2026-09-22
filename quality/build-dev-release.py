#!/usr/bin/env python3
"""Build isolated, immutable dev delivery from a verified checkout.
Only artifact snapshots are environment-rendered; production sources are untouched.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import zipfile

PLATFORMS = [(osname, arch) for osname in ("darwin", "linux", "windows") for arch in ("amd64", "arm64")]


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def run(args, cwd, env=None):
    subprocess.run(args, cwd=cwd, env=env, check=True)


def render_dev_assets(root, build_id):
    """Render only official distribution content, not Go activation/update code."""
    def render(path):
        if path.suffix not in (".md", ".py", ".html", ".json", ".tmpl", ".sh", ".ps1"):
            return
        text = path.read_text()
        for region in ("cn", "ai"):
            text = re.sub(r"https://s3\.viceme\." + region + r"(?=/skills/|/start/|[\"\']|$)",
                          f"https://s3.viceme.{region}/dev/builds/{build_id}", text)
            text = text.replace("https://viceme." + region, "https://dev.viceme." + region)
        if path.name == "SKILL.md":
            marker = "---"
            parts = text.split(marker, 2)
            notice = ("\n> dev 测试分发：先确认 dev CLI 与 dev Profile。安装、更新、恢复由本构建的 "
                      f"https://s3.viceme.cn/dev/builds/{build_id}/start/agent-install.md 指导；"
                      "本分发中通用的 viceme update 生产更新步骤不适用。不得向生产环境重试。\n\n")
            if len(parts) == 3:
                text = marker.join(parts[:2]) + marker + notice + parts[2]
        path.write_text(text)
    for directory in ("skills", "widgets", "payments"):
        if (root / directory).exists():
            for path in (root / directory).rglob("*"):
                if path.is_file():
                    render(path)
    render(root / "release/trial-bootstrap.py.tmpl")


def archive(root, output):
    with zipfile.ZipFile(output, "w", compression=zipfile.ZIP_DEFLATED) as z:
        for path in sorted(root.rglob("*")):
            if path.is_file():
                info = zipfile.ZipInfo(path.relative_to(root).as_posix(), (1980, 1, 1, 0, 0, 0))
                info.compress_type = zipfile.ZIP_DEFLATED
                info.external_attr = (0o100755 if path.stat().st_mode & 0o111 else 0o100644) << 16
                z.writestr(info, path.read_bytes())


def build(source, output, build_id, commit, platforms, allow_dirty=False):
    if not re.fullmatch(r"[a-f0-9]{40}", commit):
        raise ValueError("full commit required")
    if not re.fullmatch(r"dev-[0-9]+-[0-9]+-" + commit[:12], build_id):
        raise ValueError("build ID must be dev-RUN_ID-RUN_ATTEMPT-SHA12")
    actual = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=source, text=True).strip()
    if actual != commit:
        raise ValueError("commit must match the source checkout HEAD")
    dirty = bool(subprocess.check_output(["git", "status", "--porcelain"], cwd=source, text=True).strip())
    if dirty and not allow_dirty:
        raise ValueError("source checkout must be clean for a deliverable build")
    if output.is_relative_to(source):
        raise ValueError("output must be outside the source checkout")
    if output.exists():
        raise ValueError("output must not exist")
    output.mkdir(parents=True)
    with tempfile.TemporaryDirectory(prefix="viceme-dev-source-") as temp:
        root = Path(temp) / "source"
        shutil.copytree(source, root, ignore=shutil.ignore_patterns(".git", ".cache", "node_modules", "bin", "dist", "__pycache__"))
        render_dev_assets(root, build_id)
        run(["make", "release-manifest"], root)
        run(["go", "run", "./cmd/skills-archive", "--version", "dev", "--output", str(output / "skills")], root)
        delivery = {"schemaVersion": 1, "buildId": build_id, "commit": commit, "channel": "dev", "sourceDirty": dirty, "packages": []}
        for goos, arch in platforms:
            package = Path(temp) / (goos + "-" + arch)
            package.mkdir()
            suffix = ".exe" if goos == "windows" else ""
            flags = f"-s -w -X github.com/ViceMe-AI/cli/internal/buildinfo.Version=dev -X github.com/ViceMe-AI/cli/internal/buildinfo.Commit={commit}"
            for name, key in [("CommerceSkillTrustKeys", "COMMERCE_SKILL_TRUST_KEYS"), ("TemplateCatalogTrustKeys", "TEMPLATE_CATALOG_TRUST_KEYS")]:
                if os.environ.get(key):
                    flags += f" -X github.com/ViceMe-AI/cli/internal/buildinfo.{name}={os.environ[key]}"
            env = dict(os.environ, GOOS=goos, GOARCH=arch, CGO_ENABLED="0")
            for command, name in [("viceme", "viceme"), ("dev-setup", "viceme-dev-setup")]:
                run(["go", "build", "-trimpath", "-ldflags", flags, "-o", str(package / (name + suffix)), "./cmd/" + command], root, env)
            metadata = {"schemaVersion": 1, "buildId": build_id, "commit": commit, "os": goos, "arch": arch, "sourceDirty": dirty,
                        "files": {p.name: sha(p) for p in sorted(package.iterdir())}}
            (package / "BUILD.json").write_text(json.dumps(metadata, indent=2) + "\n")
            (package / "README.md").write_text(
                "# ViceMe dev 测试包\n\n仅用于测试，不是生产发布。先校验整个 ZIP 的 SHA-256，再解压到安装目录以外。\n\n"
                f"运行本目录的 ./viceme-dev-setup{suffix} install --region cn --agent auto --replace-standalone。\n"
                "工具会核验平台与摘要，退役当前独立安装，调用正式 bootstrap 激活，并切到 dev Profile。\n"
                "npm 安装必须先用 npm 卸载，不支持直接改变安装归属。不要执行 viceme update 更新 dev；安装下一份测试包。\n\n"
                "恢复生产：停止 CLI 任务，使用本工具 uninstall --sha256 <本包 BUILD.json 中 viceme 的摘要>，"
                "成功后按生产官方安装说明安装，再明确切回生产 Profile。卸载保留 Profile、凭据、官方及购买的 Skills。\n"
                "如报告恢复日志或归属不匹配，保留现场；不要手工删日志。用户作品及许可证不属于清理范围。\n")
            (package / "SHA256SUMS").write_text("".join(f"{sha(p)}  {p.name}\n" for p in sorted(package.iterdir()) if p.is_file()))
            filename = f"viceme-{build_id}-{goos}-{arch}.zip"
            archive(package, output / filename)
            delivery["packages"].append({"os": goos, "arch": arch, "sourceDirty": dirty, "file": filename, "sha256": sha(output / filename)})
        (output / "delivery.json").write_text(json.dumps(delivery, indent=2) + "\n")
        guide = ["# ViceMe dev CLI 安装\n", f"构建：{build_id}；源码：{commit}。\n",
                 "本入口仅用于 dev 测试。选择当前操作系统与架构，下载并校验下表 ZIP 后解压。",
                 "运行包内 viceme-dev-setup install --region cn --agent auto --replace-standalone；GLOBAL 使用 --region global。",
                 "使用工具返回的 CLI 绝对路径，确认 dev Profile 后，从 Shop dev 网页复制原始口令测试。",
                 "恢复生产前使用同包工具 uninstall --sha256 <BUILD.json 中 viceme 的摘要>，再执行生产官方安装器并切回生产 Profile。",
                 "不要对 dev 使用生产 update，不要删除恢复日志。npm 安装先使用 npm uninstall -g @viceme-ai/cli。\n",
                 "| 平台 | 下载 | SHA-256 |\n|---|---|---|"]
        for item in delivery["packages"]:
            guide.append(f"| {item['os']}/{item['arch']} | [CN](https://s3.viceme.cn/dev/builds/{build_id}/{item['file']}) / [GLOBAL](https://s3.viceme.ai/dev/builds/{build_id}/{item['file']}) | {item['sha256']} |")
        (output / "start").mkdir()
        (output / "start/agent-install.md").write_text("\n\n".join(guide) + "\n")
        (output / "start/commerce-skill-install.md").write_text(
            "# dev 购买技能安装\n\n先按同目录 agent-install.md 安装本次 dev CLI 并切换 dev Profile，再遵循 API 返回的 commerce install 命令。签名验证仍由 CLI 完成；不得降级到生产或关闭签名校验。\n")
        (output / "SHA256SUMS").write_text("".join(f"{sha(p)}  {p.relative_to(output).as_posix()}\n" for p in sorted(output.rglob("*")) if p.is_file()))


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--source", type=Path, default=Path("."))
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--build-id", required=True)
    parser.add_argument("--commit", required=True)
    parser.add_argument("--allow-dirty", action="store_true", help="local smoke only; not publishable")
    parser.add_argument("--platform", choices=[a+"-"+b for a,b in PLATFORMS], action="append")
    args = parser.parse_args()
    platforms = [tuple(p.split("-")) for p in args.platform] if args.platform else PLATFORMS
    build(args.source.resolve(), args.output.resolve(), args.build_id, args.commit, platforms, args.allow_dirty)

if __name__ == "__main__":
    main()
