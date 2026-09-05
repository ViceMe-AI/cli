# ViceMe CLI shared Widgets

These are host-rendered HTML fragments, not a website or a payment authority.
The CLI owns the templates; callers own their workflows. No external scripts,
images, clipboard API, HTTP polling, order creation or business actions belong
inside a Widget.

## Payment

The command returns `paymentPresentation.widgetPath` alongside a local image
fallback. Read the HTML file, then on WorkBuddy call `read_me` with
`modules: ["interactive"]` and `show_widget` with the exact fragment. Do not use
an `<img>` for the Widget QR: the fragment already contains an encoded inline SVG.
Do not copy the provider URI into chat, an external QR service or a URL.

Show the Widget before starting the caller's bounded payment wait. Use the
caller's original command and saved order, never create a different order to
poll. A payment Widget never says that installation or any other business task
has completed. Only a server-confirmed order status may display payment success.
The countdown is display-only, uses the order's absolute `expiresAt`, and hides
the QR at expiry. Expiry is not proof of failure or a reason to create a new order.
On payment confirmation replace/close the old host Widget when supported; the
fragment cannot poll or update another host message by itself.

No Widget capability: display the generated local image (CLI PNG, Python SVG) using the host's native
image tool and the existing order payment link if one was returned. If neither
can be displayed, report that accurately; do not claim the user has seen a QR.

## Onboarding

Read the installed `SKILL.md`. If the user supplied a concrete task, perform its
normal precheck and continue it; do not require picking an example. Otherwise
generate 2–3 genuinely supported, complete prompts and short titles. Use the
same layout for every Skill: no invented capabilities, categories or bespoke UI.

Replace `__WIDGET_DATA__` in `onboarding.html` with JSON containing `skillName`,
`locale` (`zh-CN` or `en-US`), and `examples: [{title, prompt}]`. JSON-encode
strings and escape `<`, `>`, `&`, U+2028 and U+2029 as Unicode escapes so authored
content cannot end the script tag. Never insert prompt text with `innerHTML`.
Read WorkBuddy's `read_me` interactive module before rendering with `show_widget`.
`sendPrompt(string)` sends the chosen prompt as a new conversation message.
Displaying examples does not execute a task or consume quota; the receiving
Agent follows the installed trial precheck before executing the chosen task.

Without Widget support, present the same 2–3 prompts as ordinary text. Clipboard
access is not required. Source templates are published with the CLI release at
`/skills/_widgets/`; this is a shared resource directory, not an installed Skill.

The standalone Python route fetches shared resources by content-addressed URL
and verifies pinned SHA-256 before using them. `qrcodegen.py` is MIT code from
Project Nayuki with trailing whitespace normalized and no logic changes,
revision `3c6d0b3cefb4e049dc337e82237c9644399716a8`:
https://github.com/nayuki/QR-Code-generator/blob/3c6d0b3cefb4e049dc337e82237c9644399716a8/python/qrcodegen.py
Its license is retained in the source. Payment URIs are encoded locally and are
never sent to a third-party QR service. Content-addressed hosted resources must
remain available for older scripts; do not delete previous hashes at release.
