import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const workflow = await readFile(".github/workflows/poc-release.yml", "utf8");
const shellInstaller = await readFile("installers/install-poc.sh", "utf8");
const powerShellInstaller = await readFile("installers/install-poc.ps1", "utf8");
const template = await readFile("release/agent-install-poc.md.tmpl", "utf8");

test("POC release is manual, exact-branch, prerelease-only, and standalone", () => {
  assert.match(workflow, /workflow_dispatch:/);
  assert.match(workflow, /test "\$\{GITHUB_REF\}" = "refs\/heads\/poc"/);
  assert.match(workflow, /version must match x\.y\.z-poc\.N/);
  assert.match(workflow, /test "\$\{VERSION%-poc\.\*\}" = "\$SOURCE_VERSION"/);
  assert.doesNotMatch(workflow, /npm publish|NPM_TOKEN|NODE_AUTH_TOKEN/);
  assert.match(workflow, /gh release create .*--prerelease/);
});

test("POC assets use only the POC start prefix and enforce anonymous-read boundaries", () => {
  assert.match(workflow, /PREFIX='poc\/cli\/releases'/);
  assert.match(workflow, /s3:\/\/\$S3_BUCKET\/poc\/\$FILE/);
  assert.match(workflow, /POC_PUBLIC_ORIGIN\/start\/\$PREFIX\/v\$VERSION/);
  assert.match(workflow, /poc\/private-probe\/\$\{GITHUB_RUN_ID\}/);
  assert.match(workflow, /\?list-type=2/);
  assert.match(workflow, /max-age=31536000,immutable/);
  assert.doesNotMatch(workflow, /s3\.viceme\.(?:cn|ai)\/start\/cli\/releases/);
});

test("POC installers atomically select the POC API and updater without another command name", () => {
  for (const installer of [shellInstaller, powerShellInstaller]) {
    assert.match(installer, /viceme-shop-web-poc\.preview\.tencent-zeabur\.cn\/api/);
    assert.match(installer, /viceme-shop-storage-poc\.preview\.tencent-zeabur\.cn\/start\/poc\/cli\/releases/);
    assert.match(installer, /release-channel poc/);
    assert.match(installer, /allow-channel-switch/);
    assert.doesNotMatch(installer, /destination[^\n]*viceme-poc(?:\.exe)?/);
  }
});

test("POC agent contract is exact-host and signed by the POC workflow", () => {
  assert.match(template, /start\/poc\/agent-install\.md/);
  assert.match(template, /cli\/releases\/v\{\{VERSION\}\}/);
  assert.match(template, /poc-release\.yml@refs\/heads\/poc/);
});

test("POC release recovery reuses and verifies the immutable Sigstore bundle", () => {
  assert.match(workflow, /NAME" != agent-release-manifest\.sigstore\.json/);
  assert.match(
    workflow,
    /--bundle existing-release\/agent-release-manifest\.sigstore\.json/,
  );
  assert.match(
    workflow,
    /dist\/agent-release-manifest\.json\r?\n\s+cp existing-release\/agent-release-manifest\.sigstore\.json dist\/agent-release-manifest\.sigstore\.json/,
  );
});
