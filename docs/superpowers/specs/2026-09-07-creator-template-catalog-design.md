# 云端个人名片模板册设计

## 目标

让创作者在 Agent 对话中查看正式模板，并在右侧浏览器打开云端模板详情或预览；Agent
只下载用户在对话中选定、已校验的版本。以后上新模板不依赖 CLI 发版。

第一期仅发布现有的 Bonjour Card。当前开发样例只保留在开发环境，不能进入正式模板册或
被 Agent 下载、创建、上传或发布。

## 非目标

- 不创建在线草稿、模板编辑器或 Agent 与右侧网页的共享草稿。
- 不让模板详情页直接创建、上传或发布个人页。
- 不把模板选择、资料收集或发布确认移出对话。
- 不在第一期假设 Codex 或 ViceMe 右侧浏览器可以把网页点击直接写回 Agent 对话。

## 架构

模板源码继续由当前 CLI 仓库维护；独立的模板发布工作流在模板变更合并后生成静态云端
模板册，并同时发布到 CN 和 Global 区域。模板发布不等待 CLI 版本变更。

```text
模板源码与 catalog 定义
        │
        ▼
模板发布 CI ──校验/打包/签名──► CN 与 Global 静态目录
                                      │
                                      ├─ index.html（查看所有模板）
                                      ├─ manifest.json + manifest.sig
                                      └─ releases/<id>/<version>/preview、source.zip
        │
        ▼
Agent 读取并验证 manifest ──► 对话展示与右侧预览 ──► 用户在对话中选择
                                                                  │
                                                                  ▼
                                                    下载并校验 source.zip
```

正式目录使用下列区域根地址：

```text
https://s3.viceme.cn/templates/
https://s3.viceme.ai/templates/
```

稳定目录使用短缓存；每个版本的预览和源码都使用不可变路径：

```text
templates/index.html
templates/manifest.json
templates/manifest.sig
templates/releases/bonjour-card/1.0.0/preview/index.html
templates/releases/bonjour-card/1.0.0/source.zip
```

CN 与 Global 的同一版本产物必须字节一致。

## 模板目录契约

`manifest.json` 只列出 `status: "production"` 的模板。它由发布 CI 生成并签名，不能由
Agent、浏览器或用户输入生成。每个模板至少包含：

```json
{
  "schema_version": 1,
  "templates": [
    {
      "id": "bonjour-card",
      "status": "production",
      "name": "Bonjour Card",
      "version": "1.0.0",
      "scenario": "作品、资料与公开联系方式",
      "description": "带 Block 编辑能力的个人名片，适合持续补充作品与社交链接。",
      "preview_url": "https://s3.viceme.cn/templates/releases/bonjour-card/1.0.0/preview/index.html",
      "source_url": "https://s3.viceme.cn/templates/releases/bonjour-card/1.0.0/source.zip",
      "source_sha256": "sha256:<digest>",
      "license": "ViceMe template license"
    }
  ]
}
```

发布 CI 必须拒绝重复的 `id + version`、缺失文件、非 HTTPS URL、预览页或 ZIP 与清单不
一致、缺失 SHA-256、未获授权的来源，以及 `dev_mock`。ZIP 采用确定性打包；其根目录为
模板 ID，供 Agent 解压到当前工作区使用。

`manifest.sig` 使用官方模板发布公钥对应的 Ed25519 签名。Agent 在信任 manifest 前先验证
签名，再验证所选 `source.zip` 的 SHA-256；签名、schema、模板状态或哈希任一项失败即停止，
不下载或执行模板。

## 发布流程

1. 模板贡献者提交源码、预览构建输入、封面和目录条目。
2. CI 构建预览、确定性打包源码、计算 SHA-256，并校验目录 schema 与授权字段。
3. CI 生成并签名 manifest，先上传不可变的版本化文件，再更新稳定 manifest 与 index。
4. CI 从 CN 和 Global 分别读取文件，验证状态码、内容哈希与 manifest 签名。
5. 只有两区均验证通过，模板才可在稳定目录中出现。

下架模板时，稳定 manifest 移除该模板；历史不可变文件保留，不再被 Agent 发现或下载。

## Agent 与创作者流程

用户选择“查看模板”后，Agent：

1. 读取并验证区域对应的云端 manifest；本地 registry 只可作为开发或离线测试 fixture，
   不得向正式用户称为模板来源。
2. 先输出“查看所有模板”的 `index.html` 链接，再列出 manifest 中的正式模板名称、适用
   场景、简介和预览链接。
3. 右侧只打开预览或详情链接。用户在对话中说“我选 <模板名>”后，Agent 才以模板 ID
   解析对应版本并下载源码。
4. 下载成功且校验通过后，沿用现有四类资料收集、本机预览、用户确认和发布流程。

当云端目录不可读取、签名无效或没有正式模板时，Agent 只说明“模板册暂时无法打开，请稍后
再试或导入已有主页”；不得展示本地开发样例、虚构模板或自动选择 Bonjour。

## 模板详情页与“使用此模板”

详情页主按钮为“使用此模板”，次级按钮为“下载模板源码”。第一期主按钮只明确引导用户回到
对话确认模板名称；它不能声称已选中模板或已开始创建。

第二期可接入受限的网页回传：右侧宿主验证页面 origin 后接收
`{ type: "viceme.template.use", id, version }`，并把它作为一条可见的用户对话输入插入当前
会话。静态详情页在宿主未声明该能力时必须保留第一期回退行为。网页不得传递任意 URL、源码
路径、Merchant、资料或发布参数。

## 验收

- 正式用户只看到云端 manifest 中的 production 模板；开发样例永不出现。
- “查看所有模板”与每个预览链接在相应区域可打开。
- Agent 不提“本机模板册”，不展示本地绝对路径，不默认选择 Bonjour。
- Agent 只下载用户在对话中选择的 `id + version`，且签名与 SHA-256 均通过。
- 新模板从合并到两区可用不要求 CLI 版本发布。
- 右侧“使用此模板”在没有回传能力时不改变 Agent 状态；有回传能力时只写入经校验的
  `id + version`。
