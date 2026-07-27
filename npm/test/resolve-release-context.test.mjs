import assert from "node:assert/strict";
import test from "node:test";

import { resolveReleaseContext } from "../scripts/resolve-release-context.mjs";

const repository = "ViceMe-AI/cli";
const mergeCommit = "a".repeat(40);
const releaseCommit = "b".repeat(40);

function releasePullRequest(overrides = {}) {
  return {
    number: 45,
    title: "chore(release): v0.9.0",
    merged_at: "2026-07-27T03:42:00Z",
    merge_commit_sha: mergeCommit,
    base: {
      ref: "main",
      repo: { full_name: repository },
    },
    head: {
      ref: "dev",
      sha: releaseCommit,
      repo: { full_name: repository },
    },
    ...overrides,
  };
}

test("push to main resolves the exact reviewed dev head", async () => {
  const result = await resolveReleaseContext({
    eventName: "push",
    event: { ref: "refs/heads/main", after: mergeCommit },
    recoveryTag: "",
    repository,
    workflowRef: "refs/heads/main",
    fetchPullRequests: async (commit) => {
      assert.equal(commit, mergeCommit);
      return [releasePullRequest()];
    },
  });

  assert.deepEqual(result, {
    release_ref: releaseCommit,
    release_pr_title: "chore(release): v0.9.0",
    requested_tag: "",
    recovery: "false",
    release_pr_number: "45",
  });
});

test("push resolution rejects unrelated or ambiguous PRs", async () => {
  await assert.rejects(
    resolveReleaseContext({
      eventName: "push",
      event: { ref: "refs/heads/main", after: mergeCommit },
      recoveryTag: "",
      repository,
      workflowRef: "refs/heads/main",
      fetchPullRequests: async () => [
        releasePullRequest({
          head: {
            ref: "feature",
            sha: releaseCommit,
            repo: { full_name: repository },
          },
        }),
      ],
    }),
    /found 0/,
  );

  await assert.rejects(
    resolveReleaseContext({
      eventName: "push",
      event: { ref: "refs/heads/main", after: mergeCommit },
      recoveryTag: "",
      repository,
      workflowRef: "refs/heads/main",
      fetchPullRequests: async () => [
        releasePullRequest(),
        releasePullRequest({ number: 46 }),
      ],
    }),
    /found 2/,
  );
});

test("manual recovery accepts only an exact stable tag", async () => {
  const result = await resolveReleaseContext({
    eventName: "workflow_dispatch",
    event: {},
    recoveryTag: "v0.9.0",
    repository,
    workflowRef: "refs/heads/main",
    fetchPullRequests: async () => {
      throw new Error("manual recovery must not query PRs");
    },
  });
  assert.deepEqual(result, {
    release_ref: "v0.9.0",
    release_pr_title: "",
    requested_tag: "v0.9.0",
    recovery: "true",
    release_pr_number: "",
  });

  await assert.rejects(
    resolveReleaseContext({
      eventName: "workflow_dispatch",
      event: {},
      recoveryTag: "latest",
      repository,
      workflowRef: "refs/heads/main",
      fetchPullRequests: async () => [],
    }),
    /exact stable tag/,
  );

  await assert.rejects(
    resolveReleaseContext({
      eventName: "workflow_dispatch",
      event: {},
      recoveryTag: "v0.9.0",
      repository,
      workflowRef: "refs/heads/dev",
      fetchPullRequests: async () => [],
    }),
    /protected main branch/,
  );
});
