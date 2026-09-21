# ViceMe CLI shared presentation resources

The CLI owns onboarding HTML and the local payment PNG encoder. Purchase flows
own order status, entitlement, installation and recovery. The shared
[host presentation guide](../payments/host-presentation.md)
is authoritative for chat image formats and in-app browser behavior.

## Payment

Payment returns a local PNG (`imagePath`, with `imageChatSrc` for hosts that
support it) and an official `checkoutUrl` or authenticated `paymentUrl`.
Always show the clickable official link, prefer one local chat QR image where
supported, and open that same link with an available in-app browser.
No local payment HTML or result HTML is generated or distributed.
The hosted cashier polls a restricted read-only order snapshot and changes to
Paid in place. It cannot install Skills, consume trials or grant entitlements.
Historical chat images are static, and QR expiry does not prove an order failed.

Website Replica's `--payment-result-first` still returns the server-confirmed
`PRESENT_SUPPORT_RESULT` before downloading, with plain `presentation.title`
and `presentation.description`. Report it in chat; the open cashier updates
itself. Execute the returned recovery-only continuation for the same purchase,
preserving `--expected-order-no`. Presentation cannot initiate another order.

## Onboarding

Hosts may remove script elements and evaluate their contents separately. Templates
locate an uninitialized card through the host-scoped `document.querySelector`,
then use that element's `ownerDocument` for DOM methods and lifecycle listeners.
Do not depend on `document.currentScript`, script adjacency, or prototype methods
being present on a host's scoped document/window facade. Initialization is marked
per card so replaying a script does not duplicate examples or timers.


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
