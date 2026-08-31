# Creator Skill orchestration

## Ownership

The official creator Skills expose one user goal each:

| Skill | Owns | Does not own |
| --- | --- | --- |
| `viceme-creator-onboarding` | Login for creator onboarding, qualification checks, application/claim state, Merchant selection | Publishing Works, Products, or website integrations |
| `viceme-paid-skill` | Downloadable Skill packaging, preview, confirmation, publication, and updates | Websites, services, physical/custom goods, appointments, generic Products |
| `viceme-access` | Host-code integration for an already published and configured Website Work | Login, creator application, Website Work publication, access configuration |
| `viceme-tip` | Website Work publication plus hosted tip configuration and embed integration | Creator qualification or downloadable Skill publication |
| `viceme-shared` | CLI installation, ordinary login, updates, and diagnostics | Any creator gameplay |

Buyer-side `viceme-skill-use` does not use creator qualification.

Service and generic-product CLI/API commands remain available for future product work, but no
official Skill advertises or invokes them in this delivery.

## Reusable qualification contract

Every creator gameplay invokes `$viceme-creator-onboarding` before its first platform write and
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

After application, the command returns `creatorIdentity`, including the stable `markdownPath`. The
Skill reports that route as “申请中” and stops until a later run observes an approved OWNER Merchant.

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
`viceme publish` command and no publish logic in `viceme-shared`.

## Skill call graph

```text
website direct-application prompt
          |
          v
viceme-creator-onboarding
          ^
          | qualification guard
          |
          +-- viceme-paid-skill
          +-- viceme-access
          `-- viceme-tip
```

`viceme-access` may be invoked after `viceme-tip` or another website publication flow has returned a
published Work configuration, but it still performs the qualification guard before changing creator
host code. It does not mutate Shop publication resources itself.

## Migration

- Retire `viceme-publish` from the official installed set.
- Move only its downloadable Skill workflow and publication error contract to
  `viceme-paid-skill`.
- Remove the generic-product and website workflow references from that bundle.
- Route Website Work publication needed by tips inside `viceme-tip` while delegating qualification.
- Update all official-Skill installation, manifest, metadata, and behavioral tests atomically so an
  update never installs both the retired and replacement publish Skills.
