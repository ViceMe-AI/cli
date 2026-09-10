# 创作者主页对话化基础改造 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将创作者申请后的个人页制作收敛为 Agent 主导、Bonjour 只读预览、静态 ZIP 交付的可靠流程，并杜绝首个页面发布前展示 404 个人页链接。

**Architecture:** 仅修改 CLI 内嵌的官方 Skill、Bonjour 模板及模板目录元数据；不增加平台 API、云端构建器或宿主 Widget。become-a-creator 决定首次入口时机，customize-your-profile-page 负责两级材料路径（模板中心 vs 自定义个人主页；自定义下再分原样导入与参考重制）与静态交付，Bonjour 只展示已由 Agent 生成的数据。

**Tech Stack:** Go 内容回归测试、Markdown Skill、React 19 / Vite 7、JSON template catalog、现有 ViceMe CLI。

**Spec:** `docs/superpowers/specs/2026-09-09-creator-page-conversation-and-static-delivery-design.md`

## Global Constraints

- 从最新 `origin/dev` 创建普通功能分支；不使用 fork，PR 目标为 `dev`。
- 本 PR 仅做基础改造；`viceme.template.use` / 内置模板选择 Widget 属于宿主验证后的第二 PR。
- URL 只使用本次 CLI 的结构化返回；不拼接、反推、编码或改写。
- 首次申请成功但没有 active release 时，绝不展示、打开或称 `profileUrl` 为可访问个人页。
- 最终上传物必须是含根级 `viceme-page.json` 和 HTML entry 的静态 ZIP；不得上传 React/Next 源码。
- 未获得用户明确授权前，不 commit、push、创建 PR、合并或发布。

---

### Task 1: 确认正式来源并建立基线

**Files:**
- Read: `AGENTS.md`
- Read: `skills/become-a-creator/SKILL.md`
- Read: `skills/customize-your-profile-page/SKILL.md`
- Read: `internal/skillcontent/official_content_test.go`
- Read: `internal/skillcontent/bonjour_page_template_test.go`

**Produces:** 最新 `dev` commit、干净工作树、内嵌 Skill 源目录与安装显示名称的对应记录。当前正式 source 是 `customize-your-profile-page`；不得修改已退役的 `customize-your-page` 历史目录。

- [ ] **Step 1: 建立标准工作树**

  Run:

  ```bash
  git fetch origin dev
  git worktree add -b feat(skills)/creator-page-onboarding-foundation <new-worktree> origin/dev
  git -C <new-worktree> status --short
  git -C <new-worktree> rev-parse origin/dev
  ```

  Expected: 工作树为空，分支基于最新 `origin/dev`。

- [ ] **Step 2: 验证嵌入源与安装别名**

  Run:

  ```bash
  rg -n 'customize-your-profile-page' skills internal/skillcontent
  go test ./internal/skillcontent -run 'TestCreatorPersonalCard|TestBonjourCard' -count=1
  ```

  Expected: 记录 source 到安装名的映射；不以本机安装目录为编辑目标。

- [ ] **Step 3: 运行修改前窄基线**

  Run:

  ```bash
  make skill-check
  (cd skills/customize-your-profile-page/templates/bonjour-card && npm ci && npm run build)
  ```

  Expected: 均通过；已有失败先单独记录。

### Task 2: 测试驱动地收紧首次 welcome、材料入口与 URL 输出

**Files:**
- Modify: `internal/skillcontent/official_content_test.go`
- Modify: `skills/become-a-creator/SKILL.md`
- Modify: `skills/customize-your-profile-page/SKILL.md`
- Modify: `skills/customize-your-profile-page/references/import-routing.md`

**Interfaces:**
- Consumes: CLI 结构化 `creatorIdentity.profileUrl`、`creatorIdentity.markdownUrl`、active release 状态。
- Produces: 首创不暴露预留链接；WorkBuddy 两级选择弹窗（先 Bonjour／自定义，自定义后再原样导入／参考重制；无宿主能力时同样两级编号回退）；仅在发布验证后输出 profileUrl。

- [ ] **Step 1: 先添加失败的内容回归测试**

  在 `TestCreatorPersonalCardUsesCreatorFacingWelcomeCopy`、`TestCreatorFirstPageUsesTwoLevelChoiceHierarchy` 与 `TestCreatorCardImportAndUpdateFlowKeepSafeGates` 增加精确的首创禁止项、WorkBuddy 两级选择、自然语言旁路、URL/Markdown 分离和静态 ZIP 规则。已有 `active release` 的更新流程仍可展示当前页。不得再要求一次三选一弹窗。

  ```go
  for _, forbidden := range []string{
      "我已经把仅你可见的个人页和当前内容页放到右侧",
      "查看个人页（真实 profileUrl）",
      "查看当前内容（真实 markdownUrl）",
  } {
      if strings.Contains(firstCreationCopy, forbidden) {
          t.Fatalf("first creation exposed an unavailable page: %q", forbidden)
      }
  }
  ```

- [ ] **Step 2: 运行测试确认现状失败**

  Run:

  ```bash
  go test ./internal/skillcontent -run 'TestCreatorPersonalCard' -count=1
  ```

  Expected: FAIL，原因是当前 Skill 虽然已避免打开空 `profileUrl`，但没有要求以原生两级选择弹窗分流，也仍在首创欢迎样例中提及 `markdownUrl`。

- [ ] **Step 3: 最小化更新 Skill 规则**

  保留当前 `active release` 分流。首次申请/审核中且无 active release 时，只说明可以准备名片，不展示 `profileUrl` 或 `markdownUrl`；以 WorkBuddy `AskUserQuestion`（无能力时同样两级编号回退）先呈现“查看模板中心 / 自定义个人主页”，选择自定义后再呈现“原样导入我的主页 / 参考网页／截图重制”。用户已自然语言选择模板中心、某个已验证模板、原样导入或参考重制时不重复弹窗；简历、头像、项目、链接一次自由输入。模板中心仅在该路径被选定后才 list／展示；用户选择的 production 模板才 fetch／打开已验证预览。已有 release 的更新分支才保留查看当前页。

  导入规则必须写明：ZIP、授权源码/仓库可原样导入；仅 URL 或截图走“参考重制”；图片可作确认公开的素材；服务端运行时、私有 API、SSR/Node 不能随静态 release 上传。

- [ ] **Step 4: 写入链接安全的可测试规则**

  `markdownUrl` 仅在用户明确要求查看原始内容时出现，首创欢迎文本不得预告或展示它；禁止 `**URL**` 及把中文文案、Markdown 标记或手工编码混入 URL。首次发布只有在 build、inspect、upload、publish 和渲染验证成功后，才能从本次 CLI 返回给出标准 `profileUrl` 链接。

- [ ] **Step 5: 跑聚焦回归**

  Run:

  ```bash
  go test ./internal/skillcontent -run 'TestCreator(Onboarding|PersonalCard|CardImport)' -count=1
  ```

  Expected: PASS。

### Task 3: 将 Bonjour 改成可证明的只读预览

**Files:**
- Modify: `internal/skillcontent/bonjour_page_template_test.go`
- Modify: `internal/skillcontent/official_content_test.go`
- Modify: `skills/customize-your-profile-page/templates/bonjour-card/src/App.jsx`
- Modify: `skills/customize-your-profile-page/templates/bonjour-card/src/data.js`
- Modify: `skills/customize-your-profile-page/templates/bonjour-card/src/styles.css`
- Modify: `skills/customize-your-profile-page/templates/bonjour-card/public/viceme-page.json`

**Interfaces:**
- Consumes: Agent 写入的 `initialProfile` 和 `initialBlocks`。
- Produces: 无编辑状态、无 `context.read` 覆盖的页面；保留有有效 URL 的作品/联系方式查看、打开和实际使用的站内跳转。

- [ ] **Step 1: 将现有反向测试改成只读失败测试**

  重命名 `TestBonjourCardKeepsTheSuppliedBlockEditorPrototype`。保留视觉基线类名断言，禁止编辑器文字、`localStorage`、`window.viceme.context.get()`、编辑图标、`FileReader` 和输入控件；断言 `LinkPreviewModal` 和 `window.viceme.navigation.openWork` 仍存在。

  ```go
  for _, forbidden := range []string{
      "添加 Block", "编辑个人资料", "localStorage",
      "window.viceme.context.get()", "<Icon name=\\"edit\\"",
      "FileReader", "<input",
  } {
      if strings.Contains(app, forbidden) {
          t.Fatalf("read-only template retained editor surface %q", forbidden)
      }
  }
  ```

- [ ] **Step 2: 运行测试确认当前模板失败**

  Run:

  ```bash
  go test ./internal/skillcontent -run TestBonjourCard -count=1
  ```

  Expected: FAIL，当前实现仍有编辑器和平台资料静默覆盖。

- [ ] **Step 3: 最小化删除编辑器，不重做视觉**

  从 `App.jsx` 删除编辑 state、持久化、context 覆盖、Block picker、编辑/上传/保存/删除 modal 及 handler。`BlockCard` 仅对有有效 URL 的条目提供打开/预览；没有 URL 的卡片为不可交互展示。保留 `LinkPreviewModal`、键盘关闭和焦点恢复。删除仅服务编辑器的 CSS，不调整保留区块的网格、字体、颜色或间距。

- [ ] **Step 4: 收紧 manifest 与数据**

  从 manifest 移除 `context.read`；只有实际调用时保留 `navigation.open`。从 `data.js` 删除 `STORAGE_KEY`、编辑类型定义和编辑导向占位文案，保留身份、作品与联系方式形状。

- [ ] **Step 5: 构建并跑测试**

  Run:

  ```bash
  go test ./internal/skillcontent -run 'TestBonjourCard|TestCreatorPersonalCard' -count=1
  (cd skills/customize-your-profile-page/templates/bonjour-card && npm run build)
  ```

  Expected: PASS，且模板无可写控件。

### Task 4: 验证静态 ZIP 和嵌套路径

**Files:**
- Modify only if build 暴露真实资源路径问题: `skills/customize-your-profile-page/templates/bonjour-card/package.json` 或已有 Vite 配置。
- Test: `npm/test/page_import_preview_test.py`

- [ ] **Step 1: 组装待上传目录**

  从 `dist/` 复制静态产物和根级 `viceme-page.json` 到临时目录；确认 HTML entry、manifest、JS/CSS/资源均在同一静态根下。

- [ ] **Step 2: 用现有工具检查嵌套 iframe 路径**

  Run:

  ```bash
  python3 skills/customize-your-profile-page/scripts/preview_import.py --root <staged-page-root>
  ```

  Expected: 首页、至少一个有效链接、iframe 内刷新正常；开发服务器根路径不是交付证据。

- [ ] **Step 3: 完成人工桌面/窄屏/键盘走查**

  记录截图路径：焦点只经过实际链接和预览关闭；不得出现添加、编辑、保存、删除或上传按钮。

### Task 5: 发布 Bonjour 1.0.2 的 catalog 输入

**Files:**
- Modify: `templates/creator-pages/production.json`
- Modify: `templates/creator-pages/previews/bonjour-card.html`
- Test: `internal/templatecatalog/catalog_test.go`
- Test: `cmd/template-catalog/main_test.go`

- [ ] **Step 1: 先写 1.0.2 的 catalog 测试**

  扩展 fixture/assertion：历史 `1.0.1` release 仍可读取，新 production 输入生成/获取 `1.0.2`，不得把新 source 写回旧 release 路径。

- [ ] **Step 2: 运行 catalog 测试，确认旧 catalog 无法满足新版本**

  Run:

  ```bash
  go test ./internal/templatecatalog ./cmd/template-catalog -run 'Template|DetachedDocument' -count=1
  ```

  Expected: FAIL，直到 production metadata 声明 `1.0.2`。

- [ ] **Step 3: 更新受控输入**

  将现有 `1.0.1` production descriptor 升至 `1.0.2`，描述为“只读预览，资料由 Agent 对话修改”。更新版本控制内的预览输入。不得手写签名、source ZIP 或 release digest；正式 CI 才持有 signing key。

- [ ] **Step 4: 验证 catalog**

  Run:

  ```bash
  make template-catalog-check
  ```

  Expected: PASS。

### Task 6: 完整验证与用户审查包

**Files:**
- Review: `git diff --check`
- Review: `git diff --stat origin/dev...HEAD`

- [ ] **Step 1: 运行所有本地质量门**

  Run:

  ```bash
  make check
  make npm-package-check
  ```

  Expected: PASS。若失败，先定位是否由本 PR 引入，不跳过测试。

- [ ] **Step 2: 审查范围**

  Run:

  ```bash
  git diff --check origin/dev...HEAD
  git diff --name-only origin/dev...HEAD
  git status --short
  ```

  Expected: 仅包含本计划的 Skill、模板、测试、catalog 输入和文档；没有凭据、签名产物或无关格式化。

- [ ] **Step 3: 交付审查包，不创建 PR**

  交付改动摘要、测试输出、桌面/窄屏截图与限制说明：右侧内置模板选择尚未实现，仍需真实 WorkBuddy 验证。等待用户明确同意后，才 commit、push 并创建目标为 `dev` 的基础 PR。

## Follow-up PR: 宿主模板回传增强（不属于本计划）

仅在基础 PR 已发布、且真实 WorkBuddy 验证成功后单独设计与实现。它只能回传 `{ type: "viceme.template.use", id, version }`；Agent 重新核验 production manifest。任何失败都回退到右侧只读预览，不得伪造“已选择模板”。
