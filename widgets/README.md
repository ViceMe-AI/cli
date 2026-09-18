# ViceMe CLI shared Widgets

Onboarding is a host-rendered HTML fragment. Payment provides a complete WeChat
Pay cashier page and a PNG QR image. The Agent presents them through a channel
the current host explicitly supports. Neither is a payment authority.
The CLI owns the templates; callers own their workflows. No
external scripts, images, clipboard API, HTTP polling, order creation or
business actions belong inside a Widget.

Hosts may remove script elements and evaluate their contents separately. Templates
locate an uninitialized card through the host-scoped `document.querySelector`,
then use that element's `ownerDocument` for DOM methods and lifecycle listeners.
Do not depend on `document.currentScript`, script adjacency, or prototype methods
being present on a host's scoped document/window facade. Initialization is marked
per card so replaying a script does not duplicate examples or timers.

## Payment

When local generation succeeds, the command returns `paymentPresentation.widgetPath` (a complete WeChat Pay
page), `paymentPresentation.imagePath` (a local PNG), and
`paymentPresentation.imageChatSrc` (`local-file://` plus that absolute path).
The Agent chooses using the current host's documented image syntax and available
presentation tools. Platform detection provides a preference, not a capability
decision; it must not exclude a channel the current host explicitly supports.
The caller's workflow still defines permitted channels (for example,
`AGENT_PLATFORM`); capability checks do not relax those restrictions.

- Image channel: embed `imagePath` using the host's documented syntax. When the
  host supports absolute-path Markdown (as current Codex Desktop does), write
  `![微信支付二维码](<imagePath>)` with the actual absolute path. Use `imageChatSrc`
  only when the host supports `local-file://`; do not copy that protocol to
  another host or assume every Markdown renderer supports local images.
- Page channel: pass `widgetPath` to an available tool that explicitly supports
  local HTML. Do not guess tool names. Having a browser tool does not prove it
  accepts local files or `file://` URLs.
- If one channel fails, respect the host's restrictions and use another
  independently supported channel. Only when no hosted entry is usable and neither local image nor page can be
  displayed, state the `widgetPath` absolute path verbatim and ask the user to
  open it. A bare path is not a displayed QR and must not start the payment wait.

Anonymous trial purchases may also return **outer** `checkoutUrl` (the official
login-free cashier) and `checkoutImageUrl` (its HTTPS QR image); these are not
members of `paymentPresentation`. Always provide the returned checkout URL as a
clickable Markdown link. Embed at most one QR image per order in each chat
response: choose either a supported local image or the hosted HTTPS image.
Use checkoutImageUrl only when the HTTPS image channel is selected; do not
append it as a second QR image when a usable local image is already selected.
Switch sources only if the selected image channel is unavailable or explicitly
fails. The HTML payment preview may accompany the single selected chat image.
Do not replace the link with an image or assume every chat can render images.
Use only returned fields, never construct URLs.

WorkBuddy and Doubao Work prefer their available local platform channel, with
the hosted pair as fallback; retain the hosted link alongside local display.
Other hosts prefer the hosted image and link, then their independently supported
local channels. Local artifact IO can fail without losing a valid hosted entry;
`paymentPresentation` is absent in that case, so do not invent local paths.
With an older API that returns neither URL, use the local rules above unchanged.
Only when neither hosted nor local presentation is available, hand over an
existing local HTML path. Respect the caller's permitted channels throughout.

Known host preferences follow the same capability rules:

- WorkBuddy: prefer embedding the PNG in the chat bubble with Markdown `![微信支付二维码]`
  followed by parentheses around `imageChatSrc` (a bare filesystem path does
  not render as an image), and, when available, open only the HTML page with
  `present_files([widgetPath])`; never pass the PNG to `present_files`. When
  `present_files` is unavailable on WorkBuddy, the chat image alone still
  counts as displayed. If the local image channel is unavailable or explicitly
  fails, use the hosted image when supported; do not embed both QR images.
- Doubao Work: prefer delivering `widgetPath` with `present_files` when
  available. Do not pass the PNG to `present_files` or infer chat image support
  from the platform name. Another explicitly supported channel remains usable.
- Codex, Claude, and unknown hosts: choose either supported channel above;
  identifying a platform does not prove or disprove local image support.

For presentation, use the returned HTML page or PNG directly; reading their
contents as text is unnecessary. Source files may be inspected when needed
for a security review. Do not paste HTML into chat or call `show_widget` for
payment. The page background is transparent so the cashier card can sit in
the center of the host preview.

Do not copy the provider URI into chat, an external QR service or a URL.
The page already contains an encoded inline SVG QR; do not use `<img src>`
inside the page and do not redraw or guess missing QR paths.

Display the QR through at least one supported channel or deliver a clickable
official checkoutUrl before starting the caller's bounded payment wait.
Do not start the wait in the background before delivering the payment entry. A hosted
link is a payment entry, not evidence that a QR was displayed. Both local image
and page are not required. Merely
generating artifacts or returning a tool response does not prove display.
Use the caller's original command and saved order, never create a
different order to poll. The local page never says that installation or any other
business task has completed. Only a server-confirmed order status may display
payment success. Visible copy is the green WeChat poster, amount, countdown, QR, and merchant title. The countdown uses the order's absolute `expiresAt` and hides the QR at expiry. Expiry is not proof of failure or a reason to create a new order.

The official hosted cashier may poll its restricted read-only order snapshot
and show server-confirmed payment status. It cannot install Skills, consume
trials, or grant entitlements. Local Widgets remain network-free.

If no supported channel displays anything, report that accurately; do
not claim the user has seen a QR.

Callers may supply `resultTitle` and `resultDescription` for a confirmed `PAID`
snapshot. The template then hides the entire cashier (including the QR,
countdown and scan instructions) and shows a plain acknowledgement card. These
fields have no effect while payment is pending or unconfirmed. They never
trigger a business action.

Website Replica's opt-in `--payment-result-first` wait returns
`PRESENT_SUPPORT_RESULT` before downloading. The host explicitly presents its
`presentation.widgetPath`, a separate `.support.html` file, to replace the
active preview; rewriting a previously opened file does not prove a host has
refreshed it. The caller then executes the returned recovery-only continuation
for the same purchase, preserving `--expected-order-no` as the returned
`orderNo`. A different local purchase must stop instead of taking over delivery.
Presentation failure retains the payment fact and must
not trigger another purchase. The historical chat PNG is not a live status UI.

## Onboarding

The caller's `use-a-skill` installation guide decides when to show onboarding,
including after confirming an existing installation. This section only owns
example presentation; it does not install Skills or decide usage eligibility.
Once the caller selects onboarding, read the installed `SKILL.md` and generate
2–3 genuinely supported, complete prompts and short titles. Use the same layout
for every Skill: no invented capabilities, categories or bespoke UI.

Replace `__WIDGET_DATA__` in `onboarding.html` with JSON containing `skillName`,
`locale` (`zh-CN` or `en-US`), and `examples: [{title, prompt}]`. JSON-encode
strings and escape `<`, `>`, `&`, U+2028 and U+2029 as Unicode escapes so authored
content cannot end the script tag. Never insert prompt text with `innerHTML`.
On WorkBuddy call `read_me({modules:["interactive"]})`, then actually invoke
`show_widget` with the filled template. A text list is not a rendered Widget.
`sendPrompt(string)` sends the chosen prompt as a new conversation message.
Displaying examples does not execute a task or consume quota; the receiving
Agent follows the installed trial precheck before executing the chosen task.

Without Widget support, present the same 2–3 prompts as ordinary text. Clipboard
access is not required. A failed template request or rendering error is not proof
that the host lacks Widget support; report it instead of silently falling back.
Source templates are published with the CLI release at
`/skills/_widgets/`; this is a shared resource directory, not an installed Skill.

Installers bundle these templates, guides, the Python runtime and QR encoder in
each product's `.viceme/` directory. Use the returned local paths after install;
do not fetch static resources or probe updates during onboarding, use or payment.
Only first-time no-CLI bootstrap downloads one content-addressed runtime archive
and verifies its pinned SHA-256. `qrcodegen.py` is MIT code from
Project Nayuki with trailing whitespace normalized and no logic changes,
revision `3c6d0b3cefb4e049dc337e82237c9644399716a8`:
https://github.com/nayuki/QR-Code-generator/blob/3c6d0b3cefb4e049dc337e82237c9644399716a8/python/qrcodegen.py
Its license is retained in the source. Payment URIs are encoded locally and are
never sent to a third-party QR service. Content-addressed hosted resources must
remain available for older scripts; do not delete previous hashes at release.
