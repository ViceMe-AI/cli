"use strict";

const assert = require("node:assert/strict");
const crypto = require("node:crypto");
const fsp = require("node:fs/promises");
const os = require("node:os");
const path = require("node:path");
const test = require("node:test");
const JSZip = require("../../skills/let-me-make-a-copy/scripts/jszip.min.cjs");
const {
  WorkflowError,
  authorityForWorkUrl,
  ensureCheckout,
  fetchWorkInstruction,
  inspect,
  installArchive,
  verifyLicense,
  waitForPayment,
  withLock,
} = require("../../skills/let-me-make-a-copy/scripts/make-copy.cjs");

test("accepts only official ViceMe work markdown authorities", () => {
  assert.equal(
    authorityForWorkUrl("https://viceme.cn/alice/site.md").apiBaseUrl,
    "https://viceme.cn/api/v1",
  );
  assert.throws(
    () => authorityForWorkUrl("https://example.com/alice/site.md"),
    (error) =>
      error instanceof WorkflowError &&
      error.code === "MAKE_COPY_WORK_URL_INVALID",
  );
});

test("extracts one instruction only from the platform-controlled block", async () => {
  const authority = authorityForWorkUrl("https://viceme.cn/alice/site.md");
  const fetchImpl = async () =>
    new Response(
      "## 平台控制的完整源码做同款入口\n\nInstruction: VICEME-REPLICA:VMR-ABCDEFGHIJKLMNOPQRST",
      { status: 200 },
    );
  assert.equal(
    await fetchWorkInstruction(authority, fetchImpl),
    "VICEME-REPLICA:VMR-ABCDEFGHIJKLMNOPQRST",
  );
});

test("reports an existing standalone paid recovery without exposing it", async () => {
  const root = await fsp.mkdtemp(path.join(os.tmpdir(), "make-copy-test-"));
  const replicaId = "11111111-1111-4111-8111-111111111111";
  const shortCode = "VMR-ABCDEFGHIJKLMNOPQRST";
  const apiBaseUrl = "https://viceme.cn/api/v1";
  const paidId = crypto
    .createHash("sha256")
    .update(`${apiBaseUrl}\n${shortCode}`)
    .digest("hex");
  try {
    await fsp.writeFile(
      path.join(root, `paid-${paidId}.json`),
      `${JSON.stringify({
        schemaVersion: 1,
        replicaId,
        orderNo: "VMO-20260904-000001",
        recoverySecret: crypto.randomBytes(32).toString("base64url"),
      })}\n`,
      { mode: 0o600 },
    );
    const fetchImpl = async (url) =>
      String(url).endsWith(".md")
        ? new Response(
            `## 平台控制的完整源码做同款入口\n\nInstruction: VICEME-REPLICA:${shortCode}`,
          )
        : new Response(
            JSON.stringify({
              replicaId,
              shortCode,
              title: "Replica",
              creator: { displayName: "Creator" },
              viceMeWorkUrl: "https://viceme.cn/alice/site",
              product: {
                id: "22222222-2222-4222-8222-222222222222",
                skuId: "33333333-3333-4333-8333-333333333333",
                title: "Replica",
                currency: "CNY",
                priceCents: 100,
              },
            }),
          );
    const inspected = await inspect(
      { workUrl: "https://viceme.cn/alice/site.md" },
      { fetch: fetchImpl, stateRoot: root },
    );
    assert.equal(inspected.standaloneRecoveryAvailable, true);
    assert.equal(inspected.instruction, `VICEME-REPLICA:${shortCode}`);
    assert.equal(JSON.stringify(inspected).includes("recoverySecret"), false);
  } finally {
    await fsp.rm(root, { recursive: true, force: true });
  }
});

test("installs a bounded archive into a new target and records its license", async () => {
  const root = await fsp.mkdtemp(path.join(os.tmpdir(), "make-copy-test-"));
  try {
    const zip = new JSZip();
    zip.file("VICEME-REPLICA.md", "# Deploy\n\nRun the verified build.\n");
    zip.file("src/index.js", "console.log('ok');\n");
    const archive = await zip.generateAsync({
      type: "nodebuffer",
      platform: "UNIX",
    });
    const archivePath = path.join(root, "source.zip");
    const target = path.join(root, "copy");
    await fsp.writeFile(archivePath, archive);
    const installed = await installArchive(archivePath, target, {
      replicaId: "11111111-1111-4111-8111-111111111111",
      versionId: "22222222-2222-4222-8222-222222222222",
      version: 1,
      artifactDigest: "a".repeat(64),
      licenseJws: "header.payload.signature",
    });
    assert.equal(installed.fileCount, 2);
    assert.equal(
      await fsp.readFile(path.join(target, "src/index.js"), "utf8"),
      "console.log('ok');\n",
    );
    assert.match(
      await fsp.readFile(
        path.join(target, ".viceme/replica-license.json"),
        "utf8",
      ),
      /header\.payload\.signature/u,
    );
  } finally {
    await fsp.rm(root, { recursive: true, force: true });
  }
});

test("rejects a ZIP path that JSZip had to sanitize", async () => {
  const root = await fsp.mkdtemp(path.join(os.tmpdir(), "make-copy-test-"));
  try {
    const zip = new JSZip();
    zip.file("VICEME-REPLICA.md", "# Deploy\n");
    zip.file("../escape.txt", "escape");
    const archivePath = path.join(root, "source.zip");
    await fsp.writeFile(
      archivePath,
      await zip.generateAsync({ type: "nodebuffer", platform: "UNIX" }),
    );
    await assert.rejects(
      installArchive(archivePath, path.join(root, "copy"), {
        replicaId: "11111111-1111-4111-8111-111111111111",
        versionId: "22222222-2222-4222-8222-222222222222",
        version: 1,
        artifactDigest: "a".repeat(64),
        licenseJws: "header.payload.signature",
      }),
      (error) =>
        error instanceof WorkflowError &&
        error.code === "REPLICA_ARCHIVE_INVALID",
    );
  } finally {
    await fsp.rm(root, { recursive: true, force: true });
  }
});

test("verifies the JWS against the platform trust key and exact purchase", async () => {
  const { privateKey, publicKey } = crypto.generateKeyPairSync("ed25519");
  const header = Buffer.from(
    JSON.stringify({
      alg: "EdDSA",
      kid: "test-key",
      typ: "viceme-replica-license+jws",
    }),
  ).toString("base64url");
  const claims = {
    schemaVersion: "website-replica-license/v2",
    entitlementId: "33333333-3333-4333-8333-333333333333",
    replicaId: "11111111-1111-4111-8111-111111111111",
    versionId: "22222222-2222-4222-8222-222222222222",
    version: 1,
    orderNo: "VMO-20260904-000001",
    artifactDigest: "a".repeat(64),
    licenseTermsVersion: "website-replica-license/v1",
    issuedAt: "2026-09-04T00:00:00.000Z",
  };
  const payload = Buffer.from(JSON.stringify(claims)).toString("base64url");
  const signature = crypto
    .sign(null, Buffer.from(`${header}.${payload}`), privateKey)
    .toString("base64url");
  const download = {
    replicaId: claims.replicaId,
    versionId: claims.versionId,
    version: claims.version,
    artifactDigest: claims.artifactDigest,
    licenseJws: `${header}.${payload}.${signature}`,
  };
  const fetchImpl = async () =>
    new Response(
      JSON.stringify({
        keyId: "test-key",
        algorithm: "Ed25519",
        publicKey: publicKey
          .export({ format: "der", type: "spki" })
          .toString("base64url"),
      }),
      { status: 200 },
    );
  await assert.doesNotReject(
    verifyLicense(
      { apiBaseUrl: "https://viceme.cn/api/v1" },
      download,
      { replicaId: claims.replicaId, orderNo: claims.orderNo },
      fetchImpl,
    ),
  );
});

test("waits before each of the three bounded payment checks", async () => {
  let sleeps = 0;
  let requests = 0;
  const fetchImpl = async () => {
    requests += 1;
    return new Response(JSON.stringify({ payment: { status: "PENDING" } }), {
      status: 200,
    });
  };
  await assert.rejects(
    waitForPayment(
      { apiBaseUrl: "https://viceme.cn/api/v1" },
      { sessionId: "session", sessionToken: "token", orderNo: "order" },
      fetchImpl,
      async (milliseconds) => {
        assert.equal(milliseconds, 60_000);
        sleeps += 1;
      },
    ),
    (error) =>
      error instanceof WorkflowError &&
      error.code === "REPLICA_PAYMENT_TIMEOUT",
  );
  assert.equal(sleeps, 3);
  assert.equal(requests, 3);
});

test("recovers a target lock left by a terminated process", async () => {
  const root = await fsp.mkdtemp(path.join(os.tmpdir(), "make-copy-test-"));
  const lockDirectory = path.join(root, "target.lock");
  try {
    await fsp.mkdir(lockDirectory);
    await fsp.writeFile(
      path.join(lockDirectory, "owner.json"),
      `${JSON.stringify({ pid: 2_147_483_647 })}\n`,
      { mode: 0o600 },
    );
    assert.equal(
      await withLock({ lockDirectory }, async () => "recovered"),
      "recovered",
    );
    await assert.rejects(fsp.access(lockDirectory));
  } finally {
    await fsp.rm(root, { recursive: true, force: true });
  }
});

test("persists the original order recovery credential before payment", async () => {
  const root = await fsp.mkdtemp(path.join(os.tmpdir(), "make-copy-test-"));
  const store = {
    filename: path.join(root, "target.json"),
    paidReceiptFilename: path.join(root, "recovery.json"),
  };
  const state = {
    instruction: "VICEME-REPLICA:VMR-ABCDEFGHIJKLMNOPQRST",
    sessionClientRequestId: crypto.randomUUID(),
    sessionReplaySecret: crypto.randomBytes(32).toString("base64url"),
    quoteClientRequestId: crypto.randomUUID(),
    orderClientRequestId: crypto.randomUUID(),
    downloadRecoverySecret: crypto.randomBytes(32).toString("base64url"),
    priceCents: 100,
    replicaId: "11111111-1111-4111-8111-111111111111",
  };
  let request = 0;
  const fetchImpl = async () => {
    request += 1;
    return new Response(
      JSON.stringify(
        request === 1
          ? {
              sessionId: "22222222-2222-4222-8222-222222222222",
              token: "session-token",
              expiresAt: "2026-09-04T01:00:00.000Z",
            }
          : {
              orderNo: "VMO-20260904-000001",
              status: "PENDING",
              checkoutUrl: "https://viceme.cn/replica-checkout/session",
              expiresAt: "2026-09-04T00:30:00.000Z",
            },
      ),
      { status: 201 },
    );
  };
  try {
    await ensureCheckout(
      { apiBaseUrl: "https://viceme.cn/api/v1" },
      state,
      store,
      fetchImpl,
    );
    const recovery = JSON.parse(
      await fsp.readFile(store.paidReceiptFilename, "utf8"),
    );
    assert.equal(recovery.orderNo, "VMO-20260904-000001");
    assert.equal(recovery.replicaId, state.replicaId);
    assert.equal(recovery.recoverySecret, state.downloadRecoverySecret);
  } finally {
    await fsp.rm(root, { recursive: true, force: true });
  }
});
