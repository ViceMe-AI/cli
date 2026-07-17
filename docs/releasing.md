# Automated CLI releases

Viceme CLI uses an automated Release PR as the only normal production release
gate. Maintainers merge feature and fix PRs into `dev`; they do not edit version
files, create tags, write changelog entries, or run npm commands locally.

## Normal flow

1. A non-bot push to `dev` starts `Prepare Release PR`.
2. `npm/scripts/prepare-release.mjs` finds the newest reachable stable tag and
   reads all unreleased non-merge commits.
3. Conventional Commits select the next version:
   - a `BREAKING CHANGE` footer or `type!:` selects major;
   - `feat:` selects minor;
   - every other releasable change selects patch.
4. The workflow synchronizes `package.json`, `package-lock.json`, Go build
   metadata, bundled Skill metadata, command manifest, release digests, and
   `CHANGELOG.md`.
5. It runs `make check` and `make npm-package-check`, commits the generated
   release files to `dev`, and creates or updates one `dev` to `main` PR titled
   `chore(release): vX.Y.Z`.
6. A maintainer reviews and merges that PR using the repository's preferred
   merge method.
7. `Release CLI and npm launcher` tags the exact reviewed PR head, reconnects
   the merged `main` commit into `dev` when necessary, reruns the quality gates,
   builds six platform binaries and six checksums, creates the GitHub Release,
   and publishes the npm launcher.

## One-time repository setup

GitHub Actions needs `Read and write permissions` and permission to create pull
requests so the built-in `GITHUB_TOKEN` can update `dev` and maintain the
Release PR. No maintainer PAT is required by these workflows.

Configure npm trusted publishing for:

- npm package: `@viceme-ai/cli`;
- GitHub organization/repository: `ViceMe-AI/cli`;
- workflow file: `.github/workflows/release.yml`.

Trusted publishing is the steady-state path and uses GitHub OIDC plus npm
provenance. If npm does not allow trusted publishing to create the package on
its first release, add a repository secret named `NPM_TOKEN` containing a
granular automation token limited to `@viceme-ai/cli` publication. Remove that
secret after the package exists and trusted publishing is confirmed.

No release-specific environment variables are required. `GITHUB_TOKEN` is
provided by Actions. `NPM_TOKEN` is optional and should only be retained when
the npm account policy requires it.

## Recovery

The release workflow is safe to rerun from GitHub Actions. Existing tags must
point to the same reviewed commit. Existing GitHub Release assets are compared
byte-for-byte and never overwritten. Existing npm versions must have the same
registry integrity as the locally packed artifact; otherwise the workflow
fails closed. A rerun of an older version cannot move the npm `latest` tag
behind a newer release.

The tag-push trigger remains as a recovery path for an immutable tag created by
an operator, but it is not part of the normal release flow.
