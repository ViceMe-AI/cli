---
name: viceme-danmaku
description: Add the ViceMe-hosted danmaku widget to a website with the official four-line CDN snippet and a public workKey. Use for ordinary requests such as "add ViceMe danmaku". Default to the hosted fast path; use the bundled React blueprint only when the user explicitly asks to own or customize the component source.
---

# Add ViceMe danmaku

For an ordinary request to add ViceMe danmaku, finish through the hosted SDK.
ViceMe owns the widget UI, behavior, persistence, and styles. The host page owns
only one four-line script tag.

## Choose the path

- **Hosted fast path, default:** Use for any generic request to add ViceMe
  danmaku, including static HTML, blogs, product pages, and React applications.
- **React source path, explicit only:** Use only when the user explicitly asks
  to copy, fork, restyle, or directly maintain the React/Tailwind component.

Never choose the React source path merely because the host uses React or already
contains an old danmaku implementation.

## Hosted fast path

Target completion within about 10 seconds when the user is already signed in
and the ViceMe API is reachable. Authentication and network waiting are outside
that target.

This POC installer owns `$HOME/.local/bin/viceme`. Use that exact executable
for every workflow command; do not use a same-named npm or Homebrew executable
found earlier on `PATH`.

### Hard limits

- Read only [cdn-sdk.md](references/cdn-sdk.md), repository instructions, and
  the narrow host entry file needed for the script tag.
- Change only `.viceme/access.yaml` and one existing HTML or root-layout entry.
- Add the official four-line snippet exactly once. Do not implement, copy,
  restyle, or adapt the widget in the host repository.
- Do not add React components, CSS, local SDK files, API clients, tests, demo
  pages, build dependencies, or temporary server scripts.
- Do not rewrite, reformat, or redesign host-page content.
- Do not start a dev server, open a browser, take screenshots, or run the host
  test suite unless the user explicitly asks for acceptance testing.
- If authentication, the API, or work creation fails, stop and report the
  exact next command. Never fall back to a local or hand-written widget.
- Do not invoke the `viceme-shared` setup or repair workflow during this fast
  path. Do not run `viceme doctor`, `viceme version`, `viceme install`, search
  the filesystem for another binary, or inspect configuration internals.
- After any ViceMe command exits unsuccessfully, stop immediately. Do not run a
  second diagnostic or mutation command in the same turn.

### Workflow

1. Read [cdn-sdk.md](references/cdn-sdk.md) and locate the single HTML or root
   layout entry that owns the closing body area. Do not survey the whole app.
2. If `.viceme/access.yaml` is absent, run:

   ```bash
   "$HOME/.local/bin/viceme" access init --name "<website name>" --danmaku
   ```

   The command creates and activates the work in one operation. If it reports
   that sign-in is required, run `"$HOME/.local/bin/viceme" auth login` and
   wait for the user; do not edit the host page before authentication succeeds.
   For every other error, report the command error and stop.
3. If `.viceme/access.yaml` exists and already has an active public `danmaku`
   feature, reuse its `workKey`. Otherwise make only the required config change
   and run `viceme access apply` once.
4. Insert the four-line snippet from `cdn-sdk.md`, replacing `WORK_KEY` with the
   public key. Preserve the exact hosted URL and attributes.
5. Check only that the entry contains exactly one hosted ViceMe script and that
   the edited files have no whitespace errors. Then stop.

### Handoff

Respond briefly: state that ViceMe danmaku was added, name the edited entry
file, and mention login only if it remains required. Do not narrate repository
inspection or propose extra verification.

## React source path

Use this section only after the user explicitly requests a source-owned React
component. Before editing, read [component-contract.md](references/component-contract.md)
and every file under `assets/react-tailwind/`; those files are authoritative.

Build only the reusable overlay and direct helpers/tests. Preserve the supplied
DOM responsibilities, state transitions, dimensions, colors, breakpoints,
timing, easing, safe-area behavior, callbacks, keyboard behavior, and reduced
motion behavior. Adapt only repository boundaries such as imports, icons,
i18n, and test APIs.

Do not create an iframe, URL switcher, login flow, payment flow, API, or
persistence layer for the React source path. Do not turn the component into a
general UI framework or add speculative variants.
