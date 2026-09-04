import assert from "node:assert/strict";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { restoreReleaseAssets } from "../scripts/restore-release-assets.mjs";
import { planReleaseBinaryRecovery } from "../scripts/plan-release-binary-recovery.mjs";

const workflow = (
  await readFile(".github/workflows/release.yml", "utf8")
).replaceAll("\r\n", "\n");
const agentTemplate = await readFile("release/agent-install.md.tmpl", "utf8");
const commerceTemplate = await readFile(
  "release/commerce-skill-install.md.tmpl",
  "utf8",
);

test("keeps the base Agent installation contract independent from Product activation", () => {
  assert.match(
    agentTemplate,
    /cli\/releases\/v\{\{VERSION\}\}\/agent-release-manifest\.json/,
  );
  assert.match(agentTemplate, /agent-release-manifest\.sigstore\.json/);
  assert.match(
    agentTemplate,
    /upload or publish any file without the user's confirmation/,
  );
  assert.doesNotMatch(agentTemplate, /commerce skill install/);
  assert.doesNotMatch(agentTemplate, /<stable-name>/);
  assert.doesNotMatch(agentTemplate, /It can now:/);
  assert.doesNotMatch(agentTemplate, /send me the ZIP/i);
});

test("publishes one generic Commerce Skill activation contract to both regions", () => {
  assert.match(
    commerceTemplate,
    /viceme commerce skill install <stable-name> --agent auto --distribution DIRECT/,
  );
  assert.match(
    commerceTemplate,
    /viceme commerce skill install <stable-name> --agent workbuddy --distribution WORKBUDDY/,
  );
  assert.match(commerceTemplate, /same-region `agent-install\.md`/);
  assert.match(commerceTemplate, /COMMERCE_RUNTIME_VERSION_UNSUPPORTED/);
  assert.match(commerceTemplate, /does not require creator login/);
  assert.doesNotMatch(commerceTemplate, /mobile-recharge|photo-printing/);
  assert.match(workflow, /cosign sign-blob --bundle/);
  assert.match(workflow, /cosign verify-blob/);
  assert.match(workflow, /s3\.viceme\.cn\/start\/agent-install\.md/);
  assert.match(workflow, /s3\.viceme\.ai\/start\/agent-install\.md/);
  assert.match(workflow, /s3\.viceme\.cn\/start\/commerce-skill-install\.md/);
  assert.match(workflow, /s3\.viceme\.ai\/start\/commerce-skill-install\.md/);
  assert.match(
    workflow,
    /cmp --silent .*cn-agent-install\.md.*global-agent-install\.md/,
  );
  assert.match(
    workflow,
    /cmp --silent .*cn-commerce-skill-install\.md.*global-commerce-skill-install\.md/,
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
  assert.match(
    workflow,
    /--template \.release-workflow\/release\/commerce-skill-install\.md\.tmpl/,
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
  // dist also carries the hosted-skills staging tree (dist/skills); a raw
  // dist/* glob would hand gh release a directory and fail the publication.
  assert.match(
    workflow,
    /mapfile -t RELEASE_ASSETS < <\(find dist -maxdepth 1 -type f \| sort\)/,
  );
  assert.match(
    workflow,
    /gh release create "\$\{TAG\}" "\$\{RELEASE_ASSETS\[@\]\}" --verify-tag/,
  );
  assert.doesNotMatch(workflow, /gh release create "\$\{TAG\}" dist\/\*/);
  assert.match(workflow, /\[\[ -f "\$\{FILE\}" \]\] \|\| continue/);
});

test("recovery rebuilds only release binaries whose immutable bytes are absent", () => {
  const qualityStart = workflow.indexOf("\n  quality:\n");
  const tagStart = workflow.indexOf("\n  tag:\n", qualityStart);
  const binariesStart = workflow.indexOf("\n  binaries:\n");
  const contractStart = workflow.indexOf("\n  release-contract:\n", binariesStart);
  assert.notEqual(qualityStart, -1);
  assert.notEqual(tagStart, -1);
  assert.notEqual(binariesStart, -1);
  assert.notEqual(contractStart, -1);

  const qualityJob = workflow.slice(qualityStart, tagStart);
  const binariesJob = workflow.slice(binariesStart, contractStart);
  assert.match(
    qualityJob,
    /needs\.metadata\.outputs\.recovery == 'true' && github\.sha \|\| needs\.metadata\.outputs\.sha/,
  );
  assert.match(binariesJob, /id: recovery-assets/);
  assert.match(binariesJob, /needs_build == 'true'/);
  assert.match(binariesJob, /gh release download "\$\{TAG\}" --pattern "\$\{ASSET\}"/);
  assert.match(binariesJob, /gh release download "\$\{TAG\}" --pattern "\$\{ASSET\}\.sha256"/);
  assert.match(binariesJob, /sha256sum -c "\$\{ASSET\}\.sha256"/);
});

test("plans no-Release, partial-Release, and complete-Release binary recovery", () => {
  const asset = "viceme_0.17.0_linux_amd64";
  assert.deepEqual(planReleaseBinaryRecovery(asset, []), {
    binaryExists: false,
    checksumExists: false,
    needsBuild: true,
  });
  assert.deepEqual(planReleaseBinaryRecovery(asset, [asset]), {
    binaryExists: true,
    checksumExists: false,
    needsBuild: false,
  });
  assert.deepEqual(
    planReleaseBinaryRecovery(asset, [`${asset}.sha256`]),
    {
      binaryExists: false,
      checksumExists: true,
      needsBuild: true,
    },
  );
  assert.deepEqual(
    planReleaseBinaryRecovery(asset, [asset, `${asset}.sha256`]),
    {
      binaryExists: true,
      checksumExists: true,
      needsBuild: false,
    },
  );
});

test("recovery preserves an old Agent contract while adding a missing Commerce contract", async () => {
  const root = await mkdtemp(join(tmpdir(), "viceme-release-recovery-"));
  const existing = join(root, "existing");
  const dist = join(root, "dist");
  try {
    await Promise.all([
      mkdir(existing, { recursive: true }),
      mkdir(dist, { recursive: true }),
    ]);
    await writeFile(join(existing, "agent-install.md"), "old immutable bytes");
    await writeFile(join(existing, "viceme_1.2.3_linux_amd64"), "old trust ring binary");
    await writeFile(join(existing, "viceme_1.2.3_linux_amd64.sha256"), "old checksum");
    await writeFile(join(dist, "agent-install.md"), "new template bytes");
    await writeFile(join(dist, "viceme_1.2.3_linux_amd64"), "new trust ring binary");
    await writeFile(join(dist, "viceme_1.2.3_linux_amd64.sha256"), "new checksum");
    await writeFile(
      join(dist, "commerce-skill-install.md"),
      "new commerce contract",
    );

    await restoreReleaseAssets(existing, dist);

    assert.equal(
      await readFile(join(dist, "agent-install.md"), "utf8"),
      "old immutable bytes",
    );
    assert.equal(
      await readFile(join(dist, "commerce-skill-install.md"), "utf8"),
      "new commerce contract",
    );
    assert.equal(
      await readFile(join(dist, "viceme_1.2.3_linux_amd64"), "utf8"),
      "old trust ring binary",
    );
    assert.equal(
      await readFile(join(dist, "viceme_1.2.3_linux_amd64.sha256"), "utf8"),
      "old checksum",
    );
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test("recovery validates final binary bytes against the recovered signed Manifest", () => {
  const contractStart = workflow.indexOf("\n  release-contract:\n");
  const releaseStart = workflow.indexOf("\n  github-release:\n");
  const contractJob = workflow.slice(contractStart, releaseStart);
  assert.match(
    contractJob,
    /Verify recovered binary and signed Manifest consistency/,
  );
  assert.match(contractJob, /sha256sum -c "viceme_\$\{VERSION\}_\$\{TARGET\}\.sha256"/);
  assert.match(
    contractJob,
    /--output "\$\{EXPECTED_MANIFEST\}"[\s\S]*cmp --silent "\$\{EXPECTED_MANIFEST\}" dist\/agent-release-manifest\.json/,
  );
});

test("passes the complete recovered candidate to GitHub Release and S3", () => {
  const contractStart = workflow.indexOf("\n  release-contract:\n");
  const releaseStart = workflow.indexOf("\n  github-release:\n");
  const npmStart = workflow.indexOf("\n  npm-publish:\n");
  const s3Start = workflow.indexOf("\n  s3-publication:\n");
  const notifyStart = workflow.indexOf("\n  notify:\n", s3Start);
  assert.notEqual(contractStart, -1);
  assert.notEqual(releaseStart, -1);
  assert.notEqual(npmStart, -1);
  assert.notEqual(s3Start, -1);
  assert.notEqual(notifyStart, -1);

  const contractJob = workflow.slice(contractStart, releaseStart);
  const releaseJob = workflow.slice(releaseStart, npmStart);
  const s3Job = workflow.slice(s3Start, notifyStart);
  assert.match(contractJob, /name: release-contract[\s\S]*path: dist\/\*/);
  assert.doesNotMatch(releaseJob, /pattern: binaries-\*/);
  assert.doesNotMatch(s3Job, /pattern: binaries-\*/);
  assert.match(releaseJob, /name: release-contract\s+path: dist/);
  assert.match(s3Job, /name: release-contract\s+path: dist/);
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
    /if \[\[ "\$\{FILE\}" == \*\.md \]\]; then\s+put_immutable .*'text\/markdown; charset=utf-8'/,
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
  assert.match(workflow, /dist\/commerce-skill-install\.md/);
});
