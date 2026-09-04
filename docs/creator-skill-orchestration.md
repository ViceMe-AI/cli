# Creator Skill orchestration

## Public names and routes

| User goal | Official Skill ID | Public AI route | Status |
| --- | --- | --- | --- |
| 申请创作者资格 | `become-a-creator` | `https://viceme.cn/viceme/become-a-creator` | Available |
| 自定义作者页或作品页 | `customize-your-page` | `https://viceme.cn/viceme/customize-your-page` | Available |
| 发布付费 Skill | `sell-a-skill` | `https://viceme.cn/viceme/sell-a-skill` | Available |
| 让网站开始收费 | `charge-for-your-work` | `https://viceme.cn/viceme/charge-for-your-work` | Available |
| 接入弹幕、开放赞赏或两者 | `let-people-interact` | `https://viceme.cn/viceme/let-people-interact` | Available |
| 让别人做你的网站同款 | `let-others-make-a-copy` | `https://viceme.cn/viceme/let-others-make-a-copy` | Available |
| 接受网站做同款邀请 | `let-me-make-a-copy` | 作品 Markdown 中的平台控制入口 | Available |
| 基础设施 | `creator-tools` | `https://viceme.cn/viceme/creator-tools` | Available |

This is a breaking rename. The old `viceme-*` creator Skill IDs and old Markdown routes are not
aliases.

## Ownership

The official creator Skills expose one user goal each:

| Skill | Owns | Does not own |
| --- | --- | --- |
| `become-a-creator` | Login for creator onboarding, qualification checks, application state, Merchant selection | Publishing Works, Products, or website integrations |
| `customize-your-page` | Target ownership, capability discovery, flexible page packaging, preview, explicit publication, updates, and rollback for creator and Work pages | Creator qualification or source-code security review |
| `sell-a-skill` | Downloadable Skill packaging, preview, confirmation, publication, and updates | Websites, services, physical/custom goods, appointments, generic Products |
| `charge-for-your-work` | One-pass website input collection, internal Website Work/access provisioning, and host-code integration for follow or paid unlock | Login and creator application |
| `let-people-interact` | Three-way danmaku/tip routing, hosted SDK access, default Mounted integration, explicit custom-UI Headless integration, and Website Work selection/publication with an exact canonical Origin for danmaku-bearing routes | Creator qualification, downloadable Skill publication, Website ownership verification, or Website Widget mutation for Tip |
| `let-others-make-a-copy` | Complete website source packaging, root project handoff, immutable Replica publication, and recoverable local binding | Creator-site mutation or buyer preview, checkout, payment, download, and installation; buyer flows belong to the independent `let-me-make-a-copy` Skill |
| `let-me-make-a-copy` | Regional S3 single-file reading, buyer preview, one confirmation, CLI account or anonymous checkout, hosted Python 3.9+ recovery, verified source installation, and deployment handoff | Creator qualification, Replica publication, or creator-site mutation |
| `creator-tools` | CLI installation, ordinary login, updates, and diagnostics | Any creator gameplay |

Buyer-side `use-a-skill` and `let-me-make-a-copy` do not use creator qualification.

Service and generic-product CLI/API commands remain available for future product work, but no
official Skill advertises or invokes them in this delivery.

## Reusable qualification contract

Every creator gameplay except Website Replica publication invokes `$become-a-creator` before its
first platform write and reuses the Merchant selected by that Skill. A gameplay must not repeat
`auth status`, `auth login`, `merchant accounts`, or Merchant Onboarding commands.

`let-others-make-a-copy` is the narrow exception because its high-level Publication protocol must
finish local source checks, preview, and freezing before identity or qualification can interrupt the
flow. `viceme replica publish` owns the optional-auth create and structured qualification actions,
keeps one main `clientRequestId`, and may submit one idempotent application only after the user adds
`--auto-apply-creator`. Authentication, review, or rejection never grants an upload capability;
the exact final review remains the sole authorization to send frozen source bytes.

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
- `replica publish` and `replica install`: Website Replica publication, quote confirmation, purchase recovery, and atomic source installation;
- `merchant page`: target capability discovery, structural ZIP inspection, preview, publication, and rollback for custom pages;
- `merchant work`, `sdk-access`, and commerce application commands: website and commercial
  primitives.

Reusable transport and types stay in `internal/api`; credential storage stays in `internal/auth`;
downloadable-Skill package identity and recovery stay in `internal/publication`, while Website
Replica Publication state and project binding stay in `internal/replicapublication`. There is no
generic `viceme publish` command and no publish logic in `creator-tools`.

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
          +-- customize-your-page
          +-- charge-for-your-work
          +-- let-people-interact
          `-- let-others-make-a-copy

work Markdown copy invitation
          |
          `-- let-me-make-a-copy
```

`charge-for-your-work` performs the qualification guard, then internally reuses or provisions the Website Work and
complete access configuration required by the requested follow or paid unlock before changing creator host code.
Website Work identity, publication state, access keys, configuration commands, and readback are never user-facing
inputs or output.

## Breaking migration

- Replace `viceme-shared`, `viceme-creator-onboarding`, `viceme-paid-skill`, `viceme-access`, and
  `viceme-tip` with the new official IDs in one release; do not install aliases.
- Precisely retire only managed installs whose manifest and current bytes match an identity from a published release manifest;
  preserve user-modified same-name directories.
- Keep only the downloadable workflow and publication error contract in `sell-a-skill`.
- Route danmaku-only, open-Tip-only, and combined requests through `let-people-interact` while delegating qualification; danmaku-bearing routes require a published Website Work with an exact canonical Origin, and no engagement route requires Website ownership verification.
- Route complete website source publication through `let-others-make-a-copy` without modifying the creator's external site; route buyer preview, purchase, recovery, and installation through `let-me-make-a-copy`, which must retain an existing standalone recovery before preferring CLI.
- Publish the `let-me-make-a-copy` flat `SKILL.md` and Python script to both regional S3 origins before the Shop Web starts emitting those hosted Skill URLs. Deploy the compatible Shop API before buyers can execute the newly hosted workflow.
- Route both creator-page and Work-page UI customization through `customize-your-page`; discover target-specific capabilities before editing, validate only archive structure, preview on the real route, and require explicit confirmation before publication.
- Update all official-Skill installation, manifest, metadata, and behavioral tests atomically so an
  update never treats an old creator Skill ID as active.
