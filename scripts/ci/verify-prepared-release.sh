#!/usr/bin/env bash
set -euo pipefail
# Rebuild the entire candidate tree from its accepted parent. Never execute this
# with a write token; a marker/email alone is not release evidence.
head="${PR_HEAD_SHA:-$(git rev-parse HEAD)}"
[[ "$head" =~ ^[a-f0-9]{40}$ ]]
PR_HEAD_SUBJECT="$(git show -s --format=%s "$head")"
PR_HEAD_BODY="$(git show -s --format=%B "$head")"
PR_HEAD_AUTHOR_EMAIL="$(git show -s --format=%ae "$head")"
export PR_HEAD_SUBJECT PR_HEAD_BODY PR_HEAD_AUTHOR_EMAIL
[[ "$(bash scripts/ci/validate-pr-target.sh --is-prepared-release-commit)" == true ]]
[[ "$(git rev-list --parents -n 1 "$head" | wc -w | tr -d ' ')" == 2 ]]
parent="$(git rev-parse "${head}^")"
# Both the allowlist and whole-tree comparison are required: changing generator
# code, dependencies, runtime templates or business code is not an exception.
while IFS= read -r path; do
  case "$path" in
    CHANGELOG.md|package.json|package-lock.json|internal/buildinfo/buildinfo.go|quality/release-manifest.json) ;;
    skills/*/skill-package.json) ;;
    *) echo "Non-generated change in release commit: $path" >&2; exit 1 ;;
  esac
done < <(git diff --name-only "$parent" "$head")
release_date="$(git show -s --format=%B "$head" | git interpret-trailers --parse | sed -n 's/^ViceMe-Release-Date: //p')"
[[ "$release_date" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]]
run_url="$(git show -s --format=%B "$head" | git interpret-trailers --parse | sed -n 's/^ViceMe-Release-Run: //p')"
[[ "$run_url" == "https://github.com/${GITHUB_REPOSITORY}/actions/runs/"* ]]
run_id="${run_url##*/}"
[[ "$run_id" =~ ^[0-9]+$ ]]
# The authenticated Actions API ties evidence to this repository and original
# source SHA, rather than trusting a user-controlled commit author field.
evidence="$(gh api "repos/${GITHUB_REPOSITORY}/actions/runs/${run_id}")"
EVIDENCE="$evidence" EXPECTED_PARENT="$parent" node --input-type=module -e '
 const r=JSON.parse(process.env.EVIDENCE);
 if(r.path!==".github/workflows/release-pr.yml" ||
    !["pull_request","workflow_dispatch"].includes(r.event) ||
    r.head_sha!==process.env.EXPECTED_PARENT ||
    (r.status==="completed" && r.conclusion!=="success"))
   throw new Error("Release preparation evidence does not match accepted parent");
'
# A synchronize run may start while its successful producer finishes updating
# the PR. Reproducibility below remains mandatory even when that run is active.
scratch="$(mktemp -d)"
cleanup() { git worktree remove --force "$scratch/source" >/dev/null 2>&1 || true; rm -rf "$scratch"; }
trap cleanup EXIT
git worktree add --detach "$scratch/source" "$parent" >/dev/null
(
  cd "$scratch/source"
  RELEASE_DATE="$release_date" make release-prepare
  git add --all
  actual="$(git write-tree)"
  expected="$(git rev-parse "${head}^{tree}")"
  [[ "$actual" == "$expected" ]] || {
    echo "Release commit differs from reproducible generated tree" >&2
    git diff --stat "$head" --cached >&2
    exit 1
  }
)
echo "Prepared release matches generated tree"
