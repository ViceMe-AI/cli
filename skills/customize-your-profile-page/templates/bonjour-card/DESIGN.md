# Design contract

This file is normative. When this template is selected, use the supplied React/Vite project as the implementation. It is a read-only, Agent-driven preview. Do not rebuild, reinterpret, restyle, or replace it with a newly generated page.

## Locked visual structure

- Keep the 48 px translucent top bar and its compact brand mark.
- Keep the desktop two-column workspace: left profile rail, 1 px vertical divider, right white content canvas. At 760 px and below it becomes one column with a horizontal divider.
- Keep the left-rail order: rounded-square avatar, name, headline, bio, divider, “主题标签” label, tag pills.
- Keep the right-canvas order: “个人主页” title, explanatory subtitle, then the Block stack.
- Keep the Block anatomy: 48 px symbol/cover rail, blue type label, title and subtitle. A card with a valid URL may open the existing link-view dialog; a card without one remains display-only.
- Keep the authored inline SVG icons. Do not substitute emoji, text glyphs, a new icon library, or generated image icons.

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

The exact source values in `src/styles.css` are authoritative. Do not approximate them from this table or translate them into a different design system.

## Allowed changes

- Replace profile name, headline, bio, avatar and tags with user-confirmed content.
- Replace the data of existing work and contact cards: links, titles, descriptions and one cover image.
- Make a visual change only when the user explicitly asks for that exact change after seeing the local preview.

## Forbidden changes

- Do not start from a blank HTML file or another framework.
- Do not create a hero page, masonry gallery, portfolio grid, brutalist card, marketing landing page or another “inspired by” layout.
- Do not add manual authoring controls, browser-persisted drafts, runtime profile overrides or additional Block types.
- Do not add text, image-gallery, video, App, official-account, highlight, education, employment or award Blocks.
- Do not call image generation for decoration or invent user content.

## Local preview

Before upload, build the supplied source and show the resulting static page through the local nested-path preview. The source structure and `src/styles.css` are the only visual authority; do not reconstruct the page from an image or add a screenshot-comparison step. All content changes return to the Agent conversation, while the page remains a read-only preview.
