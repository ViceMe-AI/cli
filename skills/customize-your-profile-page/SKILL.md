---
name: customize-your-profile-page
description: 用 AI 为任意 ViceMe 用户创建、导入、预览、发布、更新或回滚个人主页；不要求创作者资格，也不处理作品详情页。
---

# 制作与发布 ViceMe 个人主页

首次调用 CLI 或判断 CLI 不存在前，必须先按 [creator-tools 的 CLI 定位流程](../creator-tools/SKILL.md#cli-定位) 查找现有 CLI；后续使用返回的完整路径。不得只凭 `command not found` 判定未安装。

本 Skill 服务所有已登录用户，不调用 `$become-a-creator`，不检查或申请创作者资格，不要求 Merchant。页面由当前 Agent 使用正常前端工具制作；ViceMe CLI 只负责取得当前用户唯一主页地址、校验静态包、保存可编辑源码、发布和回滚。接口按需阅读 [runtime-api.md](references/runtime-api.md)，导入或构建按需阅读 [import-routing.md](references/import-routing.md)。

面向用户的文字跟随当前语言，只说“选择制作方式”“准备预览”“发布”等业务动作；不展示命令、scope、内部 ID、错误码、原始 JSON 或鉴权结构。除非技术问题需要用户提供新来源，不逐条播报内部排查过程。

## 权威目标与当前状态

1. 严格调用 `creator-tools` 的统一登录编排完成普通登录，不复制登录命令或创作者入驻流程。
2. 运行 `viceme profile page describe`，只采用响应中的 `profileUrl`、`markdownUrl` 和 `target`。不得要求用户提供 handle，不得由客户端拼接或猜测主页地址。
3. 运行 `viceme profile page status` 判断是否有 active release；不能用空白页面、作品数或历史聊天代替状态响应。
4. 已有 active release 时，先运行 `viceme profile page source status`，然后只问：

```text
你的主页已经发布。你想：
1. 修改当前版本；
2. 重新制作一版。
```

用户已经明确选择时不重复提问。修改当前版本仅在 `RESTORABLE` 时运行 `viceme profile page source restore --destination <不存在的新目录>`；`LOCAL_SOURCE_REQUIRED` 时请用户提供原项目/ZIP，或选择重新制作。旧主页在新版本最终确认前保持不变。

## 首次创建的固定选择

没有 active release 且用户尚未明确选择时，第一问固定为且只能是：

```text
1. 模板中心
2. 自定义
```

- WorkBuddy 且 `AskUserQuestion` 可用时，必须用一个原生单选弹窗，选项标签固定为“模板中心”“自定义”。
- 其他宿主使用等义的原生选择能力；没有时才用上述简短编号文本。
- 用户已说“用模板”“导入这个项目”“按这个网页做”等清楚意图时，直接承接，不再展示同义选择器。

选择“自定义”后，若用户还未给出明确来源，第二问使用一个原生单选弹窗，选项固定为且只能是：

```text
1. 导入已有项目或静态包
2. 按网页或截图重做
3. 从零自定义
```

WorkBuddy 必须继续使用 `AskUserQuestion`。不要把简历、头像、项目、链接、联系方式拆成弹窗或多轮表单；复杂资料始终让用户在对话中自由输入。

## 四条制作路径

### 模板中心

仅在用户选择后运行 `viceme template list` 并打开返回的 `catalog_url`。只展示响应中 production 且已验证的模板，不凭记忆虚构。用户选择后运行 `viceme template fetch <id> --version <version> --destination <当前工作区>`，只使用响应中的 `source_path`，并完整阅读模板 `README.md` 与 `DESIGN.md`。

保留模板技术栈、组件结构和视觉规则；从响应得到的真实 ID/version 上传。模板不可用时说明可稍后重试或改为自定义，不拿内置目录冒充线上模板。

### 导入已有项目或静态包

只接受用户拥有或明确授权的本地目录、ZIP 或 GitHub 仓库。

- HTML/CSS/JS 静态目录或已构建 ZIP：保留内容、结构和视觉，直接做安全解压、路径适配与本地预览；没有 `package.json` 不要求 build。
- React/Vite/Next 等前端工程或 GitHub 仓库：读取项目文档和脚本，使用项目自己的依赖与静态 build/export 流程。最终上传物只能是静态目录生成的 ZIP，不上传源码、SSR 服务或 Node 运行时。
- 构建失败时定位真实错误；无法产出静态文件就请用户补充可构建源码或已导出的静态包，不新增云端构建器。

### 按网页或截图重做

网页 URL 和截图只能作为结构、交互与视觉参考，不能作为可执行源码或 1:1 导入来源。只使用用户自己的文字、头像、图片、作品和品牌；不得复制参考站的文案、人物、私有素材或仓库。先检查参考页面，再在当前工作区创建可静态构建的页面。

### 从零自定义

让用户直接描述目标风格、栏目和资料；根据已提供内容制作可静态构建的页面。不得强迫用户按字段逐项回答，也不创建低代码编辑器、云端草稿或第二个公开地址。

## 资料整理与写入

模板、参考重做、从零自定义都必须一次邀请用户给出已有资料：

> 不用按格式整理。把你已经有的资料一次发给我就行：简历、GitHub、自媒体主页、项目链接、作品图片、头像、个人介绍、联系方式和外部链接都可以。我会先统一整理哪些能直接使用、哪些需要你确认公开、还缺哪些，再只集中问你必要的补充。

导入已有项目先读取页面已经包含的信息，不要求用户重复填写。统一输出“可直接使用 / 待确认公开 / 缺失（必须修复）”三栏；联系方式、第三方链接和来源不明的展示信息放入“待确认公开”。“缺失”只列会导致页面无法正常展示、身份无法辨识或用户明确要求却未加入的必要内容。

用户确认公开范围后，同一轮直接创建或修改本机项目，不再询问目录、文件写入或草稿确认。只有将覆盖不属于本次任务的同名目录时才停下确认。

## 构建与唯一一次确认

1. 阅读 [import-routing.md](references/import-routing.md)，修正嵌套静态托管下的资源路径和客户端路由。HTML 包直接使用；React/Vite/Next/GitHub 工程先 build/export 到静态目录。
2. 在待上传 ZIP 根目录放置 `viceme-page.json`：`kind` 必须是 `CreatorPage`，入口是真实 HTML，capabilities 只声明页面实际调用的能力。
3. 运行 `viceme profile page inspect --path <zip>`。
4. 用 `python3 <本Skill目录>/scripts/preview_import.py --root <待上传目录>` 启动嵌套路径本地预览。Agent 自己检查首页、内容、资源请求、至少一个内部跳转、前进/后退和 iframe 文档刷新。
5. WorkBuddy 使用 `present_files` 展示同一份待上传静态产物；其他宿主用真实浏览器打开同一本地预览。最终本机预览是唯一一次用户确认，提示固定为：

```text
请查看这一版预览。若符合你的预期，回复“确认发布主页”；我会更新你的原主页链接。
```

用户提出修改时继续修改并重新展示；没有明确确认前不上传。不要调用线上 preview API，也不把 HTTP 200、标题或 SDK READY 当成视觉验收。

## 上传、发布、更新与回滚

用户确认最终本机预览后：

1. 运行 `viceme profile page upload --path <zip> --source <项目根目录>`；模板路径额外传真实的 `--template-id` 与 `--template-version`。一个 release 同时保存公开静态包与 owner-only 可编辑源稿。
2. 上传返回 `VALIDATED` 后运行 `viceme profile page status`，读取当前 `activeReleaseId` 与 `concurrencyToken`。
3. 立即运行 `viceme profile page publish <release-id> --expected-active <当前ID或none> --expected-concurrency <token>`；本地预览已经确认，不再二次追问。
4. 打开 `describe` 返回的原 `profileUrl`，实际检查首页、内容、资源、内部跳转和刷新。成功后只交付这一条原地址，不生成第二个公开 URL。

更新时重复 source status/restore、修改、构建、本地确认、upload、status、publish。用户明确回滚时，先读 status，再运行 `viceme profile page activate <历史release-id> --expected-active <当前ID> --expected-concurrency <token>`。

## 不变量

- 只通过 `viceme` 访问 ViceMe，不读取 Token、Cookie 或凭证文件。
- 所有登录用户都可发布个人主页；创作者资格只影响经营能力，不是主页制作或发布前置条件。
- 不新增低代码编辑器、文件上传后台、线上草稿页、私有预览 URL 或第二公开域名。
- URL/截图只作参考；静态包可直接托管；前端源码必须先构建为静态产物。
- 不自行拼接主页地址，不把一个用户的 release、源码或并发令牌用于另一个用户。
- 页面源码不是权限边界；平台能力以本次 `describe` 返回为准。
