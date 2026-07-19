# Automated CLI releases

Viceme CLI uses an automated Release PR as the only normal production release
gate. Maintainers merge feature and fix PRs into `dev`; they do not edit version
files, create tags, write changelog entries, or run npm commands locally.

## Normal flow

1. Feature and fix PRs can merge into `dev` without starting release
   preparation.
2. A maintainer explicitly opens or marks ready a repository-owned `dev` to
   `main` PR. That release intent starts `Prepare Release PR`.
3. `npm/scripts/prepare-release.mjs` finds the newest reachable stable tag and
   reads all unreleased non-merge commits.
4. Conventional Commits select the next version:
   - a `BREAKING CHANGE` footer or `type!:` selects major;
   - `feat:` selects minor;
   - every other releasable change selects patch.
5. The workflow synchronizes `package.json`, `package-lock.json`, Go build
   metadata, bundled Skill metadata, command manifest, release digests, and
   `CHANGELOG.md`.
6. It runs `make check` and `make npm-package-check`, pushes the generated
   commit to `release/prepare`, explicitly dispatches the quality workflow for
   that head, and creates or updates a `release/prepare` to `dev` preparation
   PR.
7. A maintainer reviews and merges the preparation PR. The original `dev` to
   `main` PR then synchronizes, and the workflow updates its title and evidence
   to `chore(release): vX.Y.Z` without generating another commit.
8. A maintainer reviews and merges the original Release PR.
9. `Release CLI and npm launcher` tags the exact reviewed `dev` head, reruns
   the quality gates, builds six platform binaries and six checksums, creates
   the GitHub Release, publishes the npm launcher, and then sends an
   AI-generated release summary to the release notification group in Feishu.

## One-time repository setup

GitHub Actions needs `Read and write permissions` and permission to create pull
requests so the built-in `GITHUB_TOKEN` can maintain the preparation PR and
start the exact-head quality workflow. No GitHub App, maintainer PAT, Deploy Key,
or additional release credential is required.

The existing `dev` protection remains unchanged: every generated release commit
enters through a PR, one approving review, and the four CLI quality checks.
There is no bypass actor.

The general CLI quality workflow runs for pull requests, not branch pushes.
The preparation workflow explicitly dispatches the quality workflow for the
generated branch because events created by the built-in `GITHUB_TOKEN` do not
recursively start another workflow. This provides one set of required checks
for the exact prepared commit without restoring duplicate generic push and
pull-request runs.

Configure npm trusted publishing for:

- npm package: `@viceme-ai/cli`;
- GitHub organization/repository: `ViceMe-AI/cli`;
- workflow file: `.github/workflows/release.yml`.

Trusted publishing is the steady-state path and uses GitHub OIDC plus npm
provenance. If npm does not allow trusted publishing to create the package on
its first release, add a repository secret named `NPM_TOKEN` containing a
granular automation token limited to `@viceme-ai/cli` publication. Remove that
secret after the package exists and trusted publishing is confirmed.

`GITHUB_TOKEN` is provided by Actions. `NPM_TOKEN` is optional and should only
be retained when the npm account policy requires it.

The release notification job uses the same repository secrets as Viceme Web,
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

The original merged Release PR workflow run is safe to rerun from GitHub
Actions. Existing tags must point to the same reviewed commit. Existing GitHub
Release assets are compared byte-for-byte and never overwritten. Existing npm
versions must have the same registry integrity as the locally packed artifact;
otherwise the workflow fails closed. A rerun of an older version cannot move
the npm `latest` tag behind a newer release.

There is deliberately no tag-push or manual-dispatch production trigger. A
production release can only originate from merging the repository-owned `dev`
Release PR into `main`; recovery reruns that same authorized event and exact
reviewed SHA.
