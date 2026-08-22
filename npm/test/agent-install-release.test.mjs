import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const workflow = (
  await readFile(".github/workflows/release.yml", "utf8")
).replaceAll("\r\n", "\n");
const template = await readFile("release/agent-install.md.tmpl", "utf8");

test("publishes one exact signed Agent installation contract to both regions", () => {
  assert.match(
    template,
    /cli\/releases\/v\{\{VERSION\}\}\/agent-release-manifest\.json/,
  );
  assert.match(template, /agent-release-manifest\.sigstore\.json/);
  assert.match(
    template,
    /upload or publish any file without the user's confirmation/,
  );
  assert.match(
    template,
    /viceme commerce skill install <stable-name> --agent auto --distribution DIRECT/,
  );
  assert.match(
    template,
    /Do not manually extract or execute a Product Skill ZIP/,
  );
  assert.match(template, /does not require creator CLI login/);
  assert.match(workflow, /cosign sign-blob --bundle/);
  assert.match(workflow, /cosign verify-blob/);
  assert.match(workflow, /s3\.viceme\.cn\/start\/agent-install\.md/);
  assert.match(workflow, /s3\.viceme\.ai\/start\/agent-install\.md/);
  assert.match(
    workflow,
    /cmp --silent .*cn-agent-install\.md.*global-agent-install\.md/,
  );
});

test("runs trusted recovery generators as source files outside the release module", () => {
  assert.match(
    workflow,
    /go run \.\/\.release-workflow\/cmd\/agent-release-manifest\/main\.go --version/,
  );
  assert.match(
    workflow,
    /go run \.\/\.release-workflow\/cmd\/agent-install-doc\/main\.go --version/,
  );
  assert.doesNotMatch(
    workflow,
    /go run \.\/\.release-workflow\/cmd\/agent-(?:release-manifest|install-doc) --version/,
  );
});

test("recovery can create a missing GitHub Release after an earlier job failed", () => {
  assert.match(
    workflow,
    /if \[\[ "\$\{RECOVERY\}" == "true" \]\] && gh release view "\$\{TAG\}" >\/dev\/null 2>&1; then/,
  );
  assert.match(workflow, /gh release create "\$\{TAG\}" dist\/\* --verify-tag/);
});

test("gates S3 publication through the cdn environment", () => {
  const start = workflow.indexOf("\n  s3-publication:\n");
  const end = workflow.indexOf("\n  notify:\n", start);
  assert.notEqual(start, -1);
  assert.notEqual(end, -1);
  assert.match(workflow.slice(start, end), /^    environment: cdn$/m);
});

test("keeps the start bucket anonymous read policy narrow and cache-aware", () => {
  assert.match(workflow, /policy-probe\/\$\{GITHUB_RUN_ID\}/);
  assert.match(workflow, /test "\$\{PROBE_STATUS\}" != "200"/);
  assert.match(workflow, /\?list-type=2/);
  assert.match(workflow, /max-age=31536000.*immutable/);
  assert.match(workflow, /max-age=300/);
  assert.match(
    workflow,
    /if \[\[ .*dist\/agent-install\.md.*\n\s+put_immutable .*'text\/markdown; charset=utf-8'/,
  );
  assert.match(
    workflow,
    /s3:\/\/\$\{BUCKET\}\/\$\{PREFIX\}\/\$\{FILE\}.*\n\s+--content-type 'text\/markdown; charset=utf-8'/,
  );
  assert.match(
    workflow,
    /s3:\/\/\$\{BUCKET\}\/\$\{FILE\}.*\n\s+--content-type 'text\/markdown; charset=utf-8'/,
  );
  assert.match(
    workflow,
    /\^content-type:\[\[:space:\]\]\*text\/markdown;\[\[:space:\]\]\*charset=utf-8/,
  );
});
