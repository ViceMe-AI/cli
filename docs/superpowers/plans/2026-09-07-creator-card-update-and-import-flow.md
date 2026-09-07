# Creator Card Update And Import Flow Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make personal-card imports complete the same material, disclosure and local-preview flow as templates, and add a truthful update-card entry with a safe source-recovery fallback.

**Architecture:** The change stays in the official `customize-your-page` Skill and its embedded-content contract tests. A common completion stage begins only after either a verified template source or an authorized imported source is available. Before a source-status/restore Contract exists, the Skill must ask for the original project or route the user to a rebuild rather than inventing recovery from published HTML.

**Tech Stack:** Go test, embedded official Skill content, Markdown Skill instructions, generated release manifest.

**Spec:** `docs/superpowers/specs/2026-09-07-creator-card-updates-design.md`

## Global Constraints

- Work only on `feat(skills)/creator-page-onboarding`; never modify or merge `dev`.
- Do not add an API command that the current CLI does not implement.
- Do not scrape, decompile, or call a rendered public page a recoverable source project.
- Keep template-first behaviour; do not add a blank/freeform page path.
- All user choices and inputs stay in chat; the right side only opens current pages, template samples and local previews.
- A changed public scope is confirmed once before local creation; the final local-preview confirmation is the only publish confirmation.
- An import must not ask to publish until it has completed the same three-column information review as a template.
- Regenerate `quality/release-manifest.json`; never edit its digest by hand.

---

### Task 1: Lock the onboarding and update behaviour with embedded-Skill tests

**Files:**
- Modify: `internal/skillcontent/official_content_test.go`
- Test: `internal/skillcontent/official_content_test.go`

**Interfaces:**
- Consumes: `cliembed.EmbeddedSkills()` and `readOfficialSkillBundle`.
- Produces: `TestCreatorCardImportAndUpdateFlowKeepSafeGates`, which prevents a future embedded Skill from restoring the old import-to-publish shortcut or claiming unsupported source recovery.

- [ ] **Step 1: Write the failing test**

Add this test near the other `customize-your-page` content-contract tests:

```go
func TestCreatorCardImportAndUpdateFlowKeepSafeGates(t *testing.T) {
	t.Parallel()

	content, err := fs.ReadFile(cliembed.EmbeddedSkills(), "customize-your-page/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, required := range []string{
		"无论选择真实模板、原样导入自己的主页或参考别人的主页",
		"可直接使用 / 待确认公开 / 缺失",
		"当前个人页尚未变更",
		"不得在完成这三栏整理和最终本机预览前询问是否发布",
		"修改当前版本", "重新制作一版",
		"不得从线上 HTML、截图或渲染后的 Profile Blocks 反推",
		"提供原始项目或 ZIP", "重新制作一版",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("creator-card flow omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"viceme merchant page source status",
		"viceme merchant page source restore",
		"这版名片可以发布吗？",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("creator-card flow claimed unsupported or premature action %q", forbidden)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/skillcontent -run TestCreatorCardImportAndUpdateFlowKeepSafeGates -count=1`

Expected: FAIL because the current embedded Skill scopes the three-column review to a selected real template and has no update-card fallback.

- [ ] **Step 3: Commit the red test**

Run: `git add internal/skillcontent/official_content_test.go && git commit -m "test: lock creator card import update gates"`

### Task 2: Make import and update-card state transitions explicit in the Skill

**Files:**
- Modify: `skills/customize-your-page/SKILL.md:22-69`
- Modify: `docs/superpowers/specs/2026-09-07-creator-card-updates-design.md`
- Test: `internal/skillcontent/official_content_test.go:TestCreatorCardImportAndUpdateFlowKeepSafeGates`

**Interfaces:**
- Consumes: the current qualification result, exact `profileUrl`, current `merchant page describe` result, verified template source or user-authorized import source.
- Produces: a common material-review stage, an import-safe final-preview gate, and a source-honest update-card route.

- [ ] **Step 1: Amend the design specification**

Add that imports join the same common completion stage as templates. For a direct import, analyse the imported page first; list its existing public content as reusable, confirm ambiguous information, and request only genuinely missing information. Do not require a creator to re-enter data already present in their imported page.

- [ ] **Step 2: Update the personal-card entry rule**

Expand the personal-card trigger list to include “更新我的名片”“修改个人名片” and close synonyms. For an already published card, after the same qualification and `describe` checks, show exactly:

```text
你的名片正在使用中。你想：
1. 修改当前版本：更新头像、介绍、作品、链接或局部内容；
2. 重新制作一版：从模板或导入主页开始，完成后替换现在的名片。
```

For option 1, state that the current CLI cannot retrieve a published project. If the user provides the original project or ZIP, continue as an authorized import; otherwise offer option 2. Prohibit treating online HTML, screenshots, or Profile Blocks as restored source.

- [ ] **Step 3: Replace the template-only collection gate with a common completion stage**

Replace “选定真实模板后” with a condition covering a verified template source, an authorised own-page import, and a reference-page implementation. Require all three paths to analyse source/content, show `可直接使用 / 待确认公开 / 缺失`, collect every missing item in one message, treat public-scope confirmation as local-draft authority, and open/check the final local preview before the only publish confirmation. For raw imports, preserve content, structure and visual design except for hosting adaptations and user-requested changes.

- [ ] **Step 4: Add explicit no-premature-publish copy**

Require this state after import and local compatibility checks:

```text
我已把你的主页导入并完成本机预览；当前个人页尚未变更。
我会先整理哪些内容可直接保留、哪些公开信息需要你确认，以及是否有必要补充的名片信息。
```

Prohibit “这版名片可以发布吗？” and upload/publish prompts before the three-column review and final local preview. At the final preview, require:

```text
请查看这一版预览。若符合你的预期，回复“确认更新名片”；我会更新原来的个人页链接。
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/skillcontent -run TestCreatorCardImportAndUpdateFlowKeepSafeGates -count=1`

Expected: PASS.

- [ ] **Step 6: Commit Skill and design changes**

Run: `git add skills/customize-your-page/SKILL.md docs/superpowers/specs/2026-09-07-creator-card-updates-design.md && git commit -m "feat: guide creator card imports and updates"`

### Task 3: Rebuild embedded-release evidence and run regression gates

**Files:**
- Modify: `quality/release-manifest.json` (generated)
- Test: `internal/skillcontent/official_content_test.go`

**Interfaces:**
- Consumes: the revised Skill and content contracts.
- Produces: a matching manifest and a verified official Skill bundle.

- [ ] **Step 1: Regenerate the release manifest**

Run: `make release-manifest`

Expected: `quality/release-manifest.json` changes only through the repository generator.

- [ ] **Step 2: Run focused and bundle tests**

Run: `go test ./internal/skillcontent -run 'TestCreatorCardImportAndUpdateFlowKeepSafeGates|TestOfficialSkillsKeepOneChineseSourceAndMachineContracts' -count=1 && make release-manifest-check`

Expected: both commands pass.

- [ ] **Step 3: Run repository regression gates**

Run: `go test ./... && go vet ./... && make trial-script-test && make build && git diff --check`

Expected: every command exits zero. If the trial test creates `skills/use-a-skill/scripts/__pycache__/trial.cpython-314.pyc`, remove only that generated test artifact before inspecting the final diff.

- [ ] **Step 4: Commit generated evidence**

Run: `git add quality/release-manifest.json internal/skillcontent/official_content_test.go && git commit -m "chore: refresh creator card skill manifest"`

- [ ] **Step 5: Inspect the final boundary**

Run: `git status --short && git diff origin/dev...HEAD -- skills/customize-your-page/SKILL.md internal/skillcontent/official_content_test.go docs/superpowers/specs/2026-09-07-creator-card-updates-design.md quality/release-manifest.json`

Expected: only creator-card update/import instructions, contract tests, and generated manifest evidence appear. Preserve pre-existing template-catalog work and untracked `docs/superpowers/plans/` content.
