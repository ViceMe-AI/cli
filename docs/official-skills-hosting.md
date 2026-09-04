# 官方 Skills 云端托管

官方 Skills(仓库 `skills/` 下的全部技能)随 CLI Release Workflow 托管到
start 桶,提供**不经过 ViceMe CLI 就能安装**的稳定下载入口。生成器是
`cmd/skills-archive`,消费方式与 `agent-install.md` 的区域规则一致:
按访问域名选区域——`s3.viceme.cn` 对应 CN,`s3.viceme.ai` 对应 GLOBAL。

## URL 结构

```
https://s3.viceme.cn/skills/manifest.json            稳定清单(随最高稳定版更新)
https://s3.viceme.cn/skills/<skill>.zip              稳定技能包(同上)
https://s3.viceme.cn/cli/releases/v<version>/skills/...   不可变版本化副本
```

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

start 桶的匿名读取 allowlist 需要放行 `skills/` 前缀(manifest 与 zip):
只新增 `skills/*`,不得放开列桶或其他业务对象。发布验证里的
`policy-probe` 断言保持不变(列桶必须仍然被拒)。
