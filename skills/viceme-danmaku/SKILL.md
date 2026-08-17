---
name: viceme-danmaku
description: Install the ViceMe-hosted danmaku SDK into a website or adapt the bundled React and Tailwind CSS v4 golden component. Use for four-line CDN integration, workKey setup, scrolling messages, reactions, comment entry, collapse behavior, responsive layout, keyboard support, and reduced-motion handling.
---

# Build the ViceMe danmaku component

Implement the component from the bundled golden blueprint. Do not redraw it from a prose description.

## Required inputs

Before editing, inspect:

1. The target repository's local instructions, frontend conventions, installed icon library, styling system, i18n mechanism, and test runner.
2. [component-contract.md](references/component-contract.md) in full.
3. If the user wants the component copied into a React app, inspect every file under `assets/react-tailwind/`. Treat these files as the authoritative component structure, visual tokens, state machine, motion values, and behavior tests.
4. If the user wants four-line/static-site/CDN integration, inspect [cdn-sdk.md](references/cdn-sdk.md). The browser runtime is owned by `@viceme-ai/sdk`; it is intentionally not copied into this Skill.

## Scope

There are two supported integration paths.

### Hosted CDN SDK

Use this path when the user asks for four-line integration, static page support,
or a ViceMe-hosted widget. Add only the script snippet described in
`references/cdn-sdk.md`; do not copy React or Tailwind source into the host.
The hosted widget app owns persistence, comment synchronization, login prompts,
and rendering. Tipping and rewards are outside this Skill.

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

1. Inspect [cdn-sdk.md](references/cdn-sdk.md).
2. Run `viceme auth status`; if the user is not authenticated or lacks SDK work
   access, run `viceme auth login` and wait for browser authorization.
3. If `.viceme/access.yaml` is absent, run
   `viceme access init --name "<website name>" --danmaku`. If it exists, ensure
   it contains an active `danmaku` feature with `policy.type: PUBLIC`, set the
   work status to `ACTIVE`, and run `viceme access apply`.
4. Read the resulting public `workKey` from `.viceme/access.yaml` or
   `viceme access inspect`, then add only the four-line script snippet to the
   host page. Do not copy React, Tailwind, tests, API clients, or SDK source.
5. Verify in a browser that the SDK mounted one fixed root with stage,
   controls, and modal iframes.
6. Verify the stage iframe has `pointer-events: none`, host page controls still
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
