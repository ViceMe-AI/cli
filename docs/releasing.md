# Automated CLI releases

ViceMe CLI uses an automated Release PR as the only normal production release
gate. Maintainers merge feature and fix PRs into `dev`; they do not edit version
files, create tags, write changelog entries, or run npm commands locally.

## Normal flow

1. Feature and fix PRs can merge into `dev` without starting release
   preparation.
2. A maintainer explicitly opens or marks ready a repository-owned `dev` to
   `main` PR. That release intent starts `CLI release preparation`.
3. `npm/scripts/prepare-release.mjs` finds the newest reachable stable tag and
   reads all unreleased non-merge commits.
4. Conventional Commits select the next version:
   - a `BREAKING CHANGE` footer or `type!:` selects major;
   - `feat:` selects minor;
   - every other releasable change selects patch.
5. The workflow synchronizes `package.json`, `package-lock.json`, Go build
   metadata, bundled Skill metadata, command manifest, release digests, and
   `CHANGELOG.md`.
6. It runs `make check` and `make npm-package-check`, creates a short-lived
   installation token for the repository-scoped ViceMe Release GitHub App, and
   commits only the generated files directly to protected `dev`.
7. The existing `dev` to `main` PR synchronizes, runs its required quality
   checks, and is updated to `chore(release): vX.Y.Z` with exact run and commit
   evidence. No internal preparation PR is created.
8. A maintainer reviews and merges that same Release PR.
9. Merging the Release PR pushes its merge commit to `main`.
   `CLI release publication` resolves that commit back to exactly one merged,
   repository-owned `dev` to `main` PR, then tags the exact reviewed `dev`
   head, reruns the quality gates, builds six platform binaries and six
   checksums, creates the GitHub Release, bundles those exact checksums into
   the npm launcher, publishes it, and then sends an
   AI-generated release summary to the release notification group in Feishu.

## One-time repository setup

Register a private organization-owned GitHub App named `ViceMe CLI Release Bot`.
Install it only on `ViceMe-AI/cli` with repository `Contents: read and write`;
leave every other optional permission disabled. Webhooks and user authorization
are not required. Configure:

- repository variable `RELEASE_APP_ID`: the numeric App ID;
- repository secret `RELEASE_APP_PRIVATE_KEY`: the complete generated PEM key.

Protect `dev` with an active branch ruleset that retains the normal pull request,
one approving review, the `PR quality` check, all three `PR npm installer
(<runner>)` checks, deletion protection, and force push protection. Add `ViceMe
CLI Release Bot` and the organization-admin role to
the bypass list with `Always allow`; the latter preserves the legacy rule's
existing `enforce_admins: false` behavior. Do not leave the legacy
branch-protection rule active beside the ruleset because it cannot recognize the
ruleset's App bypass.

The App installation token is scoped to the current repository and
`Contents: write`, expires after at most one hour, and is revoked automatically
when the job finishes. The workflow still stages an explicit allowlist of
generated files and validates the complete release before pushing. No
maintainer PAT or Deploy Key is used.

The general `CLI PR checks` workflow runs for pull requests, not branch pushes. A
Release App push synchronizes the already-open `dev` to `main` PR, producing one
set of required checks for the exact prepared commit without duplicate generic
push and pull-request runs. The synchronize event may run release preparation a
second time; that run is intentionally idempotent and produces no new commit.

Protect `main` with the same four PR checks plus `Release candidate
preparation`. The checks from `CLI release publication` are deliberately not
required for merging: that workflow starts only after the release PR has been
merged and performs the tag, binary, GitHub Release, npm, and notification
steps.

Configure npm trusted publishing for:

- npm package: `@viceme-ai/cli`;
- GitHub organization/repository: `ViceMe-AI/cli`;
- workflow filename: `release.yml`.

Trusted publishing is the only publication credential path and uses GitHub OIDC
plus npm provenance. Do not configure `NPM_TOKEN`; the publication job does not
generate an npm auth file or expose a long-lived token.

The npm tarball contains `checksums.txt`, generated from the six immutable
GitHub Release checksum assets immediately before publication. The launcher
uses that bundled manifest as its trust root whether the matching binary is
transported by GitHub Release, a configured npm registry binary mirror, or the
public npmmirror binary mirror. Registering `viceme-cli` with cnpmcore enables
the public `/-/binary/viceme-cli/` mirror; it does not create another npm
package.

The CN and Global release mirrors are S3-compatible origins rather than Amazon
S3 itself. The publication job sets AWS CLI request and response checksum
calculation to `WHEN_REQUIRED`: immutable artifacts are still compared
byte-for-byte on recovery, while optional AWS streaming checksum trailers that
the origins do not implement are not sent.

Configure the repository secret `CN_S3_HTTPS_PROXY` with the authenticated
HTTPS forward-proxy URL used by GitHub Actions to reach the CN S3 endpoint.
The release job applies it only inside the CN publication subshell; Global S3
publication remains a direct connection. Keep the proxy credentials in the
secret value and never print the URL in workflow logs.

`GITHUB_TOKEN` is provided by Actions and is used to maintain the Release PR
and resolve a merged `main` commit back to its reviewed Release PR.
`RELEASE_APP_ID` and `RELEASE_APP_PRIVATE_KEY` authenticate the narrowly scoped
Release App.

The release notification job uses the same repository secrets as ViceMe Web,
API, and Engine:

- `FEISHU_RELEASE_WEBHOOK`: webhook for the release notification group;
- `AI_API_KEY`: API key used to generate the release summary;
- `AI_MODEL`: optional model override, defaulting to `deepseek-chat`;
- `AI_BASE_URL`: optional OpenAI-compatible endpoint override, defaulting to
  `https://api.deepseek.com/v1`.

The notification runs only after the GitHub Release and npm publication have
both succeeded, so a failed or incomplete release is not announced as
successful.

## Recovery

The original `push` publication run is safe to rerun from GitHub Actions.
Existing tags must point to the same reviewed commit. Existing GitHub Release
assets are compared byte-for-byte and never overwritten. Existing npm versions
must have the same registry integrity as the locally packed artifact; otherwise
the workflow fails closed. A rerun of an older version cannot move the npm
`latest` tag behind a newer release.

If the tag and GitHub Release succeeded but npm publication did not, a
maintainer may manually dispatch `CLI release publication` with that exact
stable tag. Recovery refuses missing tags, version mismatches, draft or missing
GitHub Releases, changed release assets, and npm integrity mismatches. It cannot
create a new release identity. Normal production releases still originate only
from merging the repository-owned `dev` Release PR into `main`.
