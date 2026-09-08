# Design contract

This file is normative. When this template is selected, use the supplied React/Vite project as the
implementation. Do not rebuild, reinterpret, restyle, or replace it with a newly generated page.

## Locked visual structure

- Keep the 48 px translucent top bar and its compact brand mark.
- Keep the desktop two-column workspace: left profile rail, 1 px vertical divider, right white content
  canvas. At 760 px and below it becomes one column with a horizontal divider.
- Keep the left-rail order: edit icon, rounded-square avatar, name, headline, bio, divider, “主题标签”
  label, tag pills.
- Keep the right-canvas order: “内容 Block” title, explanatory subtitle, blue “添加 Block” button,
  optional notice, then the Block stack.
- Keep the Block anatomy: 48 px symbol/cover rail, blue type label, title, subtitle, circular edit icon.
- Keep “添加 Block”的 popover、作品占位卡、个人资料弹窗、作品编辑弹窗、媒体与联系平台选择
  弹窗和链接确认弹窗。
- Keep the authored inline SVG icons. Do not substitute emoji, text glyphs, a new icon library, or
  generated image icons.

## Locked tokens

| Purpose | Required value |
| --- | --- |
| Workspace background | `#f5f5f7` |
| Content canvas | `#fff` |
| Primary text | `#1d1d1f` |
| Interaction blue | `#0071e3` / hover `#0077ed` |
| Structural divider | `1px solid rgba(0, 0, 0, .08)` |
| Block radius | `12px` |
| Dialog radius | `14px` |
| Desktop workspace | `min(1180px, 100%)` |
| Desktop columns | `minmax(255px, .78fr) minmax(0, 1.7fr)` |
| Desktop profile padding | `56px clamp(24px, 4vw, 52px)` |
| Desktop canvas padding | `58px clamp(24px, 5vw, 70px)` |
| Block gap | `10px` |
| Block minimum height | `92px` |

The exact source values in `src/styles.css` are authoritative. Do not approximate them from this
table or translate them into a different design system.

## Allowed changes

- Replace profile name, headline, bio, avatar and tags with user-confirmed content.
- Add, edit, reorder or remove only the supported Block data:
  - works: link, title, description and one cover image;
  - media/contact: Feishu link, X / Twitter link, email or GitHub profile link.
- Add the minimum ViceMe data adapter or export code without changing existing component structure,
  class names or CSS.
- Make a visual change only when the user explicitly asks for that exact change after seeing the local
  preview.

## Forbidden changes

- Do not start from a blank HTML file or another framework.
- Do not create a hero page, masonry gallery, portfolio grid, brutalist card, marketing landing page or
  another “inspired by” layout.
- Do not remove the add/edit controls, headings, subtitle, dividers, tags, labels, buttons or icons.
- Do not add text, image-gallery, video, App, official-account, highlight, education, employment or award
  Blocks.
- Do not call image generation for decoration or invent user content.

## Local preview

Before upload, run the supplied source directly and show that local page to the user. The source
structure and `src/styles.css` are the only visual authority; do not reconstruct the page from an
image or add a screenshot-comparison step. Keep “添加 Block”的“导入作品”和“媒体与联系”两个
入口；需要修复时直接沿用供应的源码，不重新设计。
