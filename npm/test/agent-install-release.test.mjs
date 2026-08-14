import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const workflow = await readFile(".github/workflows/release.yml", "utf8");
const template = await readFile("release/agent-install.md.tmpl", "utf8");

test("publishes one exact signed Agent installation contract to both regions", () => {
  assert.match(template, /cli\/releases\/v\{\{VERSION\}\}\/release-manifest\.json/);
  assert.match(template, /release-manifest\.sigstore\.json/);
  assert.match(template, /upload or publish any file without the user's confirmation/);
  assert.match(workflow, /cosign sign-blob --bundle/);
  assert.match(workflow, /cosign verify-blob/);
  assert.match(workflow, /s3\.viceme\.cn\/start\/agent-install\.md/);
  assert.match(workflow, /s3\.viceme\.ai\/start\/agent-install\.md/);
  assert.match(workflow, /cmp --silent .*cn-agent-install\.md.*global-agent-install\.md/);
});

test("keeps the start bucket anonymous read policy narrow and cache-aware", () => {
  assert.match(workflow, /policy-probe\/\$\{GITHUB_RUN_ID\}/);
  assert.match(workflow, /test "\$\{PROBE_STATUS\}" != "200"/);
  assert.match(workflow, /\?list-type=2/);
  assert.match(workflow, /max-age=31536000.*immutable/);
  assert.match(workflow, /max-age=300/);
});
