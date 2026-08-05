# ViceMe CLI

ViceMe CLI 用于把创作者已有的项目接入 ViceMe 平台能力。它是一个确定性的 Go 协议客户端，可供用户直接使用，也可由 Codex、Claude Code 等 AI 编程平台调用。

1.0 版本启用全新的命令合同；旧的 Skill Agent 发布、Target 与 `job` 状态机已明确移除。

## 安装

```bash
npm install -g @viceme-ai/cli
viceme install --target auto
```

npm 包只提供轻量启动器。`viceme install` 会下载经过 checksum 验证的平台二进制，原子安装同版本的 `viceme` Agent Skill，并初始化 CN 或 GLOBAL Profile。npm 包不包含安装生命周期脚本。

## 登录

人工交互：

```bash
viceme auth login
```

AI Host 使用可跨回合的 Device Authorization：

```bash
viceme auth login --no-wait --json
# 必须原样使用上一步返回的 continue_args，包括 --profile。
viceme --profile <original-profile> auth login --device-code <opaque-device-code> --json
```

浏览器复用 ViceMe 现有登录页。Access Token 与轮换 Refresh Token 只保存到系统凭据管理器或 CLI 的加密降级存储中，项目文件不会出现 Token。

## 绑定已有项目

```bash
viceme app link --dir . --origin http://localhost:3000
```

命令会创建或选择一个 EXTERNAL Creator App，绑定 TEST 环境，注册精确的浏览器 Origin，并写入 `.viceme/app.json`：

```json
{
  "schemaVersion": 1,
  "appId": "550e8400-e29b-41d4-a716-446655440000",
  "hostingMode": "EXTERNAL",
  "environment": "TEST",
  "publishableKey": "app_pk_test_...",
  "origin": "http://localhost:3000",
  "capabilities": {}
}
```

Manifest 只包含公开标识，可以提交 Git；严禁写入 CLI Token、Server Credential、Webhook Secret、Provider Key、存储凭据或签名 URL。

## 增加与诊断 Capability

```bash
viceme capability catalog
viceme capability add commerce --dir .
viceme app doctor --dir .
viceme capability doctor commerce --dir .
```

Slice A 只提供 DRAFT 状态的 Commerce 基座。Checkout 会在下一段实现；CLI 不会在客户端伪造尚未上线的能力。

外部项目可以使用公开 Web SDK 挂载 App Context Shell，全程不下发 CLI Credential：

```ts
import { mountAppContextWidget } from "@viceme/web-sdk";

mountAppContextWidget(document.querySelector("#viceme-app"), {
  apiBaseUrl: "https://api.viceme.cn/v1",
  publishableKey: "app_pk_test_...",
});
```

API 会解析公开 Key、验证已登记 Origin，只返回公开 App Context。

## 命令面

```text
viceme install | update | version
viceme auth login | status | logout
viceme profile list | add | use | configure | rename | remove
viceme skills list | read | install | doctor
viceme app link | get | list | doctor
viceme capability catalog | add | get | doctor
```

机器可读命令面位于 `skills/viceme/references/command-manifest.json`，CI 会将其与 Cobra 命令树逐项比对。CLI 与内置 Agent Skill 同版本并共同固定 Digest。

## 开发

```bash
go test ./...
npm test
make quality-check
```

Checksum、npm provenance 与发布流程见 `docs/releasing.md`。
