---
name: viceme-danmaku
description: Implement or adapt a production React and Tailwind CSS v4 danmaku overlay that matches the bundled ViceMe golden component. Use when a user asks Codex to build, reproduce, integrate, or repair the Loom-style danmaku component with scrolling messages, reactions, comment entry, emoji picker, collapse behavior, responsive layout, keyboard support, and reduced-motion handling.
---

# Build the ViceMe danmaku component

Implement the component from the bundled golden blueprint. Do not redraw it from a prose description.

## Required inputs

Before editing, inspect:

1. The target repository's local instructions, frontend conventions, installed icon library, styling system, i18n mechanism, and test runner.
2. [component-contract.md](references/component-contract.md) in full.
3. If the user wants the component copied into a React app, inspect every file under `assets/react-tailwind/`. Treat these files as the authoritative component structure, visual tokens, state machine, motion values, and behavior tests.
4. If the user wants four-line/static-site/CDN integration, inspect [cdn-sdk.md](references/cdn-sdk.md) and every file under `assets/cdn/`.

## Scope

There are two supported integration paths.

### Hosted CDN SDK

Use this path when the user asks for four-line integration, static page support,
or a ViceMe-hosted widget. Add only the script snippet described in
`references/cdn-sdk.md`; do not copy React or Tailwind source into the host.
The hosted widget app owns persistence, comment synchronization, login prompts,
fan/follow prompts, and rewards.

### React Tailwind component

Use this path when the user asks to build the component directly inside an
existing React and Tailwind CSS v4 project. Build only the reusable danmaku
overlay and its direct helpers/tests:

- scrolling danmaku stage and lane scheduler;
- bottom interaction bar;
- first-use greeting prompt;
- quick reactions and long-press repeat;
- text entry and Enter-to-send behavior;
- more-reactions popover and search;
- collapse/expand behavior;
- responsive and reduced-motion behavior.

For the React path, do not create an iframe, URL switcher, demo-site picker,
login flow, payment flow, API, or persistence layer. The host page owns its
content and passes messages and callbacks into the component.

## Hosted CDN SDK workflow

Use this workflow for static pages, blogs, product pages, local black-box
acceptance, or any request that says "four lines", "CDN", "script tag", or
"do not modify the host app".

1. Inspect [cdn-sdk.md](references/cdn-sdk.md) and `assets/cdn/`.
2. Add only the script snippet to the host page. Do not copy React, Tailwind,
   tests, API clients, or component source into the host page.
3. For local acceptance, use the local script origin from `cdn-sdk.md` if the
   user provides a local ViceMe Web server.
4. Verify in a browser that the loader mounted one fixed root with stage,
   controls, and modal iframes.
5. Verify the stage iframe has `pointer-events: none`, host page controls still
   receive clicks, the hosted widget renders the golden danmaku style, and
   messages persist through the hosted ViceMe API.

## React component workflow

Use this workflow only when the user asks to build the danmaku component
directly inside an existing React and Tailwind CSS v4 project.

1. Locate the component's real owner. Keep it route-local unless at least two real routes consume it.
2. Confirm that the target uses Tailwind CSS v4. If it uses v3, do not silently paste the blueprint: obtain approval to upgrade or translate every v4-only dynamic utility to a bracket value and prove the adapted CSS in a production build.
3. Copy the golden blueprint into the target repository, then adapt only repository boundaries: import aliases, shared Button/cn helpers, icons, i18n strings, file locations, and test APIs.
4. Preserve the contract's DOM responsibilities, state transitions, dimensions, spacing, colors, breakpoints, timing, easing, safe-area handling, and callback semantics. Do not replace measured values with design-system approximations.
5. Keep business authorization outside the component. Call `onRequestComposer` before the first send and cache only a successful permission result for the mounted instance.
6. Keep local optimistic display separate from persistence. Only emit a self-authored bullet after `onSend` returns a message.
7. Use the target repository's styling mechanism. For Tailwind repositories, keep utilities in the owning JSX and use Web Animations API for route-local motion when required by local rules.
8. Add or adapt the blueprint tests. Test user-visible state transitions and callback behavior, not private implementation details.
9. Run the target repository's format, lint, typecheck, focused tests, full relevant tests, and production build.
10. Perform responsive and keyboard verification. If browser tooling is available, compare screenshots and computed dimensions against the contract without changing unrelated UI.

## Adaptation limits

- Preserve the compact comment icon; do not add a visible text label beside it.
- Preserve the centered reaction cluster independently of the left collapse control.
- On narrow screens, hide only the last three quick reactions; keep comment and more controls reachable.
- Do not add background chips to quick reactions. Their pointer hover uses emoji lift; popover reactions use surface hover.
- Do not infer authentication from locale or UI text.
- Do not persist auth tokens or messages in browser storage.
- Do not introduce new image assets or inline SVG when the repository already has a suitable icon library.
- Do not turn the golden component into a general UI framework or add speculative variants.

## Handoff

Report:

- files created or changed;
- repository-specific adaptations made to the golden blueprint;
- checks run and their results;
- responsive, keyboard, and reduced-motion coverage;
- any visual or runtime boundary that remains unverified.
