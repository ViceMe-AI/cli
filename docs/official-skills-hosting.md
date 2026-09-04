# 官方 Skills 云端托管

官方 Skills(仓库 `skills/` 下的全部技能)随 CLI Release Workflow 托管到
独立的 skills 桶,提供**不经过 ViceMe CLI 就能直读或安装**的稳定入口。生成器是
`cmd/skills-archive`,消费方式与 `agent-install.md` 的区域规则一致:
按访问域名选区域——`s3.viceme.cn` 对应 CN,`s3.viceme.ai` 对应 GLOBAL。

## URL 结构

```
https://s3.viceme.cn/skills/manifest.json                  稳定清单(随最高稳定版更新)
https://s3.viceme.cn/skills/<skill>.zip                    稳定技能包(整包下载)
https://s3.viceme.cn/skills/<skill>/SKILL.md               单文件直读(按原路径平铺)
https://s3.viceme.cn/skills/<skill>/scripts/<file>.py      单文件直读(同上)
https://s3.viceme.cn/start/cli/releases/v<version>/skills/...   不可变版本化副本(start 桶)
```

托管物放在**独立的公开 `skills` 桶**(与 start 桶同实例、同发布凭证):
`https://s3.viceme.cn/skills/<file>` 就是该桶对象的直读地址,没有路径前缀。
除了整包 zip,每个技能的**全部文件同时按原路径平铺**——「云端说明书」用
法(agent 直接读 SKILL.md、按 URL 执行单个脚本,不下载整包)与整包安装
并存。单文件路径就是 `manifest.json` 里 `files` 列表中的相对路径,字节
与 zip 内同名条目同源。

稳定路径的 `cache-control` 为 `max-age=300`;版本化副本
`max-age=31536000, immutable`,且同版本重发布做字节一致校验。

## manifest.json 形状

```json
{
  "schema_version": 1,
  "cli_version": "0.27.0",
  "skills": {
    "use-a-skill": {
      "skill_version": "0.27.0",
      "minimum_cli_version": "0.27.0",
      "cli_compatibility": ">=0.27.0 <0.28.0",
      "zip": "use-a-skill.zip",
      "zip_sha256": "sha256:...",
      "full_skill_bundle_digest": "sha256:...",
      "embedded_content_digest": "sha256:...",
      "files": ["SKILL.md", "agents/openai.yaml", "..."]
    }
  }
}
```

- `zip` 是相对名,消费方按所选区域拼 `https://s3.viceme.<cn|ai>/skills/<zip>`;
- `zip_sha256` 是 zip 字节的 SHA-256,下载后即可校验;
- `full_skill_bundle_digest` / `embedded_content_digest` 与
  `release-manifest.json` 同源(同一内嵌 FS 的 `Bundle.Digests`):CLI
  内嵌安装与云端下载安装的完整性校验口径完全一致。

## 非 CLI 安装方式

zip 以技能目录名为根(`use-a-skill/SKILL.md`),解压到技能根目录即完成
安装:

```bash
curl -fsSL https://s3.viceme.cn/skills/use-a-skill.zip -o /tmp/use-a-skill.zip
unzip -q /tmp/use-a-skill.zip -d ~/.agents/skills/
```

- 与 CLI 安装同路径(`~/.agents/skills/<skill>`):用户之后安装 ViceMe
  CLI 时,官方 Skills 以同一目录名自然接管覆盖,不存在双份漂移。
- macOS/Linux 与 Windows(PowerShell `Expand-Archive`)均可解压。

## 发布链路

- `cmd/skills-archive` 在 Release Workflow 的
  `Assemble exact-version agent installation contract` 步骤随版本清单
  一起生成 `dist/skills/<skill>.zip` 与 `dist/skills/manifest.json`;
- `CN and Global S3 publication` 步骤把 `dist/skills/*` 发布到
  `cli/releases/v<version>/skills/`(不可变)与桶根 `skills/`(稳定),
  并对稳定 `manifest.json` 做匿名读取与字节一致的发布后验证;
- zip 为确定性产物:固定时间戳(epoch)、固定权限(0644)、排序遍历、
  不写目录条目——同版本重发布字节必须一致,否则视为篡改并失败。

## 桶策略

- 公开 `skills` 桶:匿名读取 allowlist 为 `arn:aws:s3:::skills/*`
  (GetObject),桶内只有托管 zip 与 manifest;列桶(ListBucket)不放行。
- start 桶:匿名 allowlist 维持原有五项(install 脚本/文档与
  `cli/releases/*`),版本化副本落在其中的 `cli/releases/v<version>/skills/`。
- 两桶均已实施(CN+GLOBAL,2026-09-04)并通过行为级验证:
  `/skills/<obj>` 匿名 200、未放行对象 403、两桶列桶均 403。
