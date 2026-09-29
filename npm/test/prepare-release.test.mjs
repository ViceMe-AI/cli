import assert from "node:assert/strict";
import test from "node:test";

import {
  compatibilityRange,
  incrementVersion,
  parseConventionalCommit,
  renderChangelog,
  selectBump,
} from "../scripts/prepare-release.mjs";

const commit = (subject, body = "", sha = "1234567890abcdef") => ({ subject, body, sha });

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
