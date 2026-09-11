# ViceMe CLI shared Widgets

Onboarding is a host-rendered HTML fragment. Payment is a complete WeChat Pay
cashier page opened with `present_files`, not a chat Widget. Neither is a
payment authority. The CLI owns the templates; callers own their workflows. No
external scripts, images, clipboard API, HTTP polling, order creation or
business actions belong inside a Widget.

Hosts may remove script elements and evaluate their contents separately. Templates
locate an uninitialized card through the host-scoped `document.querySelector`,
then use that element's `ownerDocument` for DOM methods and lifecycle listeners.
Do not depend on `document.currentScript`, script adjacency, or prototype methods
being present on a host's scoped document/window facade. Initialization is marked
per card so replaying a script does not duplicate examples or timers.

## Payment

The command returns `paymentPresentation.widgetPath` (a complete WeChat Pay
page), `paymentPresentation.imagePath` (a local PNG), and
`paymentPresentation.imageChatSrc` (`local-file://` plus that absolute path).
Chat bubbles embed the PNG with Markdown `![微信支付二维码]` followed by
parentheses around `imageChatSrc`. WorkBuddy does not render a bare filesystem
path as an image. Hosts without the `local-file://` protocol may still use
`imagePath`. WorkBuddy opens only the HTML page with
`present_files([widgetPath])`; never pass the PNG to `present_files`. Do not
Read the HTML or PNG, do not paste HTML into chat, and do not call
`show_widget` for payment. The page background is transparent so the cashier
card can sit in the center of the host preview.

Do not copy the provider URI into chat, an external QR service or a URL.
The page already contains an encoded inline SVG QR; do not use `<img src>`
inside the page and do not redraw or guess missing QR paths.

Show the image and open the page before starting the caller's bounded payment
wait. Use the caller's original command and saved order, never create a
different order to poll. The page never says that installation or any other
business task has completed. Only a server-confirmed order status may display
payment success. Visible copy is the green WeChat poster, amount, countdown, QR, and merchant title. The countdown uses the order's absolute `expiresAt` and hides the QR at expiry. Expiry is not proof of failure or a reason to create a new order.

If `present_files` is unavailable, still display `imageChatSrc` in the chat bubble
with the Markdown image. If neither can be displayed, report that accurately; do
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
