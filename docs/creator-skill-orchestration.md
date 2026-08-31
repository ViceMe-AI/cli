# Creator Skill orchestration

## Public names and routes

| User goal | Official Skill ID | Public AI route | Status |
| --- | --- | --- | --- |
| 申请创作者资格 | `become-a-creator` | `https://viceme.cn/viceme/become-a-creator` | Available |
| 发布付费 Skill | `sell-a-skill` | `https://viceme.cn/viceme/sell-a-skill` | Available |
| 让网站开始收费 | `charge-for-your-work` | `https://viceme.cn/viceme/charge-for-your-work` | Available |
| 给网站加弹幕和赞赏 | `let-people-interact` | `https://viceme.cn/viceme/let-people-interact` | Available |
| 让别人做你的网站同款 | `let-others-make-a-copy` | `https://viceme.cn/viceme/let-others-make-a-copy` | Not available |
| 基础设施 | `creator-tools` | `https://viceme.cn/viceme/creator-tools` | Available |

This is a breaking rename. The old `viceme-*` creator Skill IDs and old Markdown routes are not
aliases. `let-others-make-a-copy` reserves a public name and fail-closed status route only; it is not
bundled as a Skill until a real workflow exists.

## Ownership

The official creator Skills expose one user goal each:

| Skill | Owns | Does not own |
| --- | --- | --- |
| `become-a-creator` | Login for creator onboarding, qualification checks, application/claim state, Merchant selection | Publishing Works, Products, or website integrations |
| `sell-a-skill` | Downloadable Skill packaging, preview, confirmation, publication, and updates | Websites, services, physical/custom goods, appointments, generic Products |
| `charge-for-your-work` | Host-code integration for an already published and configured Website Work | Login, creator application, Website Work publication, access configuration |
| `let-people-interact` | Website Work publication plus hosted danmaku, tip configuration, and embed integration | Creator qualification or downloadable Skill publication |
| `creator-tools` | CLI installation, ordinary login, updates, and diagnostics | Any creator gameplay |

Buyer-side `viceme-skill-use` does not use creator qualification.

Service and generic-product CLI/API commands remain available for future product work, but no
official Skill advertises or invokes them in this delivery.

## Reusable qualification contract

Every creator gameplay invokes `$become-a-creator` before its first platform write and
reuses the Merchant selected by that Skill. A gameplay must not repeat `auth status`, `auth login`,
`merchant accounts`, or Merchant Onboarding commands.

The onboarding Skill supports two invocation contexts:

- **Direct application**: the user invoked the official website's “become a creator” prompt. This is
  explicit authorization to apply. When login is needed, run `viceme auth login --purpose
  creator-onboarding`; after login, read current onboarding and submit immediately when there is no
  existing application. Do not ask for application consent again.
- **Qualification guard**: a publish or integration Skill needs creator permission. The gameplay
  request authorizes a qualification check, not an application. If there is no active OWNER Merchant
  and no application, ask once before applying.

Both contexts stop at human review. Only a valid active Merchant returned from
`MerchantAccountMember(role=OWNER)` allows the caller to continue.

## Minimum application input

`merchant onboarding apply` does not require display name or handle. The Shop derives them from the
signed-in user and the stable DRAFT CreatorAccount. The Agent asks for a handle only when the API
returns `MERCHANT_APPLICATION_HANDLE_REQUIRED`; a supplied display name or handle remains an optional
override.

After application, the Skill reads `merchant onboarding status` once as a write readback. That
response returns `creatorIdentity`, including the stable `markdownPath`. The Skill reports that route
as “申请中” and stops until a later run observes an approved OWNER Merchant.

## Login purpose

`viceme auth login` accepts `--purpose creator-onboarding`. The CLI sends the typed device-login
purpose to Shop; Shop uses it only to render “登录并申请成为创作者”. The flag does not create an
application and does not grant business permissions. The onboarding Skill performs the application
write after the same waiting login process succeeds.

Ordinary `viceme auth login` remains unchanged and sends the default purpose.

## CLI command placement

Commands remain grouped by use case under `internal/command`:

- `auth`: device login and credential lifecycle;
- `merchant onboarding` and `merchant accounts`: creator qualification and application;
- `skill publish` and `publication`: downloadable Skill publication;
- `merchant work`, `sdk-access`, and commerce application commands: website and commercial
  primitives.

Reusable transport and types stay in `internal/api`; credential storage stays in `internal/auth`;
publication package identity and recovery stay in `internal/publication`. There is no generic
`viceme publish` command and no publish logic in `creator-tools`.

## Skill call graph

```text
website direct-application prompt
          |
          v
become-a-creator
          ^
          | qualification guard
          |
          +-- sell-a-skill
          +-- charge-for-your-work
          `-- let-people-interact
```

`charge-for-your-work` may be invoked after `let-people-interact` or another website publication flow has returned a
published Work configuration, but it still performs the qualification guard before changing creator
host code. It does not mutate Shop publication resources itself.

## Breaking migration

- Replace `viceme-shared`, `viceme-creator-onboarding`, `viceme-paid-skill`, `viceme-access`, and
  `viceme-tip` with the new official IDs in one release; do not install aliases.
- Precisely retire only managed installs whose manifest and current bytes match an identity from a published release manifest;
  preserve user-modified same-name directories.
- Keep only the downloadable workflow and publication error contract in `sell-a-skill`.
- Route Website Work publication plus danmaku and tips through `let-people-interact` while delegating qualification.
- Update all official-Skill installation, manifest, metadata, and behavioral tests atomically so an
  update never treats an old creator Skill ID as active.
