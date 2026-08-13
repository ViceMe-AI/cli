# Danmaku component contract

Read this contract together with every file under `assets/react-tailwind/`. The assets are the golden implementation blueprint; this document defines what may be adapted and what must remain stable.

## Public boundary

Use a host-owned callback contract equivalent to:

```ts
type DanmakuMessage = {
  id: string;
  text: string;
  self?: boolean;
};

type DanmakuProps = {
  className?: string;
  maxLength?: number;
  messages: DanmakuMessage[];
  onRequestComposer: () => Promise<boolean>;
  onSend: (text: string) => Promise<DanmakuMessage | null>;
};
```

The component owns visual state and animation only. The host owns authentication, entitlement, persistence, network requests, seed messages, and the content underneath the overlay.

## Required structure

```text
Danmaku overlay (fixed, viewport-sized, pointer-events none)
├── bullet stage (top 160px, five lanes, aria-hidden)
├── interaction bar (bottom, pointer-events enabled)
│   ├── collapse control (left anchored)
│   ├── centered active control
│   │   ├── greeting prompt
│   │   ├── quick reaction cluster
│   │   └── text input
│   └── more-reactions popover
├── collapsed launcher (bottom-center)
└── polite screen-reader status
```

Do not wrap, embed, navigate, or switch the host content.

## State machine

Use exactly these interaction states:

```text
greeting --3.5s--> reactions --4s idle--> collapsed
greeting --click/send--> reactions
reactions --comment--> typing --successful send--> reactions
reactions --more--> more --outside click/toggle--> reactions
any expanded state --collapse--> collapsed --expand--> reactions
```

The greeting appears only for the first mounted experience. Dismiss it permanently for that mount after its timer or any reaction. A denied `onRequestComposer` result does not send or emit a bullet. Cache a successful permission result; do not cache a denial as success.

## Measured visual tokens

### Overlay and bullet stage

- Root: `fixed inset-0`, above host content, pointer-events disabled except controls.
- Minimal stage: 160px high, starts at viewport top, overflow hidden.
- Five lanes: top 8px, lane height 28px.
- Bullet font: 20px, normal weight, line-height 1.125, white at 0.8 opacity.
- Bullet shadow: `0 1px 2px rgb(0 0 0 / 0.45)`.
- Self-authored bullet: subtle white border and translucent white background.
- Speed: 145px/s; lane gap: 40px; at most 40 active bullets.
- Seed interval: 2400ms plus random jitter from 900ms through 1799ms.
- Reduced motion: static at left 16px for 2400ms.

### Expanded interaction bar

- Fixed to bottom, full viewport width.
- Height: `56px + env(safe-area-inset-bottom, 0)`.
- Background: `#1f1f21`; foreground: `#cecfd2`.
- The content row itself is 56px high.
- Collapse control: 32x32px, left 16px, vertically centered, 6px radius, 20px icon.
- Center anchor: absolute 50%/50% with -50% translation; maximum width 366px on widths >=744px; narrow maximum is viewport minus 96px.
- Desktop reaction gap: 16px. Narrow reaction gap: 8px.
- Quick reaction buttons: 32x32px; emoji size 24px; no background chip at rest or pointer hover.
- Comment and more buttons: 32x32px, 6px radius, white 24px icon.
- On widths below 440px, hide quick reactions 4 through 6 and retain reactions 1 through 3, comment, and more.

### Collapsed launcher

- Keep the 32x32px launcher horizontally centered at every viewport width.
- Anchor it with left 50% and translateX -50%; do not move it to a corner.
- Keep its bottom offset at `12px + env(safe-area-inset-bottom, 0)`.
- Expanding returns to the centered reaction cluster without replaying a dismissed greeting.

### Greeting prompt

- Centered pill, 36px high, width up to 366px.
- Fully rounded, background `#2b2b2e`, foreground `#cecfd2`.
- Border: `rgb(227 228 242 / 0.14)`.
- Text: 14px medium.
- A right-origin dark progress layer begins at scaleX(0.5), reaches scaleX(1) linearly over 3500ms, then the prompt dismisses.
- Reduced motion shows the completed layer without animating.

### Text entry

- Centered, 32px high, maximum width 366px.
- Background `#2b2b2e`; text `#cecfd2`; placeholder `#a9aaad`.
- Inset one-pixel border `#7e8188`, changing to `#cecfd2` on focus.
- Horizontal padding 12px; 16px text on narrow screens and 14px at widths >=744px.
- Use `type=text`, `inputMode=text`, `enterKeyHint=send`, and `autoComplete=off`.
- Enter submits through a form. Do not submit while an IME composition is active or `keyCode` is 229.

### More-reactions popover

- Width: min(352px, viewport minus 32px); height 272px.
- Mobile: right 16px. Desktop >=744px: horizontally centered.
- Bottom offset: 12px above the interaction bar.
- Background `#2b2c2f`; 12px radius.
- Border `rgb(227 228 242 / 0.12)`; shadow `0 6px 24px rgb(0 0 0 / 0.1)`.
- Search row: 60px high with 16px top/side padding; search field 36px high.
- Emoji grid: nine columns, 4px gaps, 16px side padding; each control 32x32px with 22px emoji.
- Open motion: 250ms from opacity 0 / scale 0.97 to opacity 1 / scale 1.
- Close motion: 150ms from opacity 1 / scale 1 to opacity 0 / scale 0.99.
- Easing: `cubic-bezier(0.22, 1, 0.36, 1)`; disable movement for reduced motion.

### Collapse and hover motion

- Bar expand: 350ms, `cubic-bezier(0.34, 1.25, 0.64, 1)`.
- Bar collapse: 250ms, `cubic-bezier(0.22, 1, 0.36, 1)`.
- Surface hover: 150ms with smooth-out easing.
- Quick emoji hover-in: 250ms, scale 1.3, translateY -7.8px.
- Quick emoji hover-out: 320ms with `cubic-bezier(0.34, 3.85, 0.64, 1)`.
- Long press starts after 420ms and repeats every 220ms. Suppress the trailing click after repeating.

## Engineering requirements

- The golden blueprint targets Tailwind CSS v4. Do not paste its dynamic utilities into Tailwind v3 and assume they work. If the target is not on v4, either obtain approval to upgrade or translate every v4-only utility to an equivalent bracket value and add a production CSS build that proves the adaptation.
- Normalize whitespace and enforce `maxLength` before authorization and send.
- Prevent duplicate permission requests while one is pending.
- Do not emit an optimistic self bullet until `onSend` returns a message.
- Bound visible bullets and clean up all timeouts, intervals, animation frames, and Web Animations on unmount.
- Use pointer capture for long press and stop on pointer up, cancel, or lost capture.
- Keep controls keyboard reachable, labeled, and represented by semantic buttons and forms.
- Use a polite live region for send success or failure.
- Use repository i18n for visible text and accessible labels.
- Use the repository's icon library; do not paste inline SVG paths.
- Honor `prefers-reduced-motion` for bullet, bar, popover, prompt, and hover motion.

## Required behavioral tests

Adapt the bundled test blueprint to prove:

1. Greeting auto-dismisses at 3500ms and exposes reactions.
2. Clicking greeting sends one wave reaction.
3. The reaction cluster and collapsed launcher stay center-anchored, and narrow layout retains all essential controls.
4. A quick reaction sends once on click and repeatedly only after long-press delay.
5. Comment opens a focused input; Enter sends, but IME composition does not.
6. Successful send clears input and emits a self bullet; denial, authorization rejection, or send failure does not. Authorization rejection enters the failure live region and a later attempt can retry.
7. More popover opens, filters emoji, closes through toggle/outside layer, and remains mounted for close motion.
8. Reactions auto-collapse after 4000ms and expand without replaying the greeting.
9. Reduced-motion bullet behavior is static and all timers/animations clean up.
