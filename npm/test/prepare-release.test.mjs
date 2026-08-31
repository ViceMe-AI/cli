import assert from "node:assert/strict";
import { readdirSync } from "node:fs";
import test from "node:test";

import {
  compatibilityRange,
  incrementVersion,
  officialSkillNames,
  parseConventionalCommit,
  renderChangelog,
  requestedVersionFromTitle,
  selectBump,
  selectReleaseVersion,
} from "../scripts/prepare-release.mjs";

const commit = (subject, body = "", sha = "1234567890abcdef") => ({ subject, body, sha });

test("official skill list matches the bundled skills directory", () => {
  const bundled = readdirSync(new URL("../../skills/", import.meta.url), { withFileTypes: true })
    .filter((entry) => entry.isDirectory())
    .map((entry) => entry.name)
    .sort();
  assert.deepEqual([...officialSkillNames].sort(), bundled);
});

test("selects semantic version bumps from conventional commits", () => {
  assert.equal(selectBump([parseConventionalCommit(commit("fix: repair login"))]), "patch");
  assert.equal(selectBump([parseConventionalCommit(commit("feat(cli): add inspect"))]), "minor");
  assert.equal(selectBump([parseConventionalCommit(commit("feat!: replace output contract"))]), "major");
  assert.equal(selectBump([parseConventionalCommit(commit("fix: repair\n", "BREAKING CHANGE: incompatible"))]), "major");
});

test("increments stable versions and computes conservative compatibility", () => {
  assert.equal(incrementVersion("0.1.0", "patch"), "0.1.1");
  assert.equal(incrementVersion("0.1.9", "minor"), "0.2.0");
  assert.equal(incrementVersion("0.9.0", "major"), "1.0.0");
  assert.equal(compatibilityRange("0.2.3"), ">=0.2.3 <0.3.0");
  assert.equal(compatibilityRange("1.4.0"), ">=1.4.0 <2.0.0");
});

test("accepts an exact next release version from the promotion PR title", () => {
  const commits = [parseConventionalCommit(commit("feat(cli): add inspect"))];

  assert.equal(requestedVersionFromTitle("release: v0.19.1"), "0.19.1");
  assert.equal(requestedVersionFromTitle("chore(release): v0.19.1"), "0.19.1");
  assert.equal(requestedVersionFromTitle("feat(cli): ordinary change"), "");
  assert.throws(
    () => requestedVersionFromTitle("release: next"),
    /release PR title must be exactly/,
  );
  assert.deepEqual(selectReleaseVersion("0.19.0", commits, "0.19.1"), {
    version: "0.19.1",
    bump: "patch",
    source: "pull_request_title",
  });
  assert.throws(
    () => selectReleaseVersion("0.19.0", commits, "0.19.2"),
    /must be the next patch, minor, or major/,
  );
});

test("renders a deterministic changelog section and preserves history", () => {
  const changelog = renderChangelog(
    "0.2.0",
    [
      commit("fix: reject stale target", "", "aaaaaaaaaa"),
      commit("feat(cli): add release automation", "", "bbbbbbbbbb"),
      commit("docs: explain install", "", "cccccccccc"),
    ],
    "# Changelog\n\n## [0.1.0] - 2026-07-17\n\n- Initial release.\n",
    "2026-07-18",
  );
  assert.match(changelog, /## \[0\.2\.0\] - 2026-07-18/);
  assert.match(changelog, /### Features\n\n- add release automation \(`bbbbbbb`\)/);
  assert.match(changelog, /### Fixes\n\n- reject stale target \(`aaaaaaa`\)/);
  assert.match(changelog, /### Other Changes\n\n- explain install \(`ccccccc`\)/);
  assert.match(changelog, /## \[0\.1\.0\] - 2026-07-17/);
});
