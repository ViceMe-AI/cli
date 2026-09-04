"use strict";

const JSZip = require("./jszip.min.cjs");
const crypto = require("node:crypto");
const fsp = require("node:fs/promises");
const os = require("node:os");
const path = require("node:path");
const { execFile } = require("node:child_process");
const { promisify } = require("node:util");

const execFileAsync = promisify(execFile);
const MAX_ARCHIVE_BYTES = 100 * 1024 * 1024;
const MAX_EXPANDED_BYTES = 500 * 1024 * 1024;
const MAX_FILE_BYTES = 100 * 1024 * 1024;
const MAX_FILE_COUNT = 10_000;
const MAX_ENTRY_COUNT = 20_000;
const MAX_PATH_BYTES = 4096;
const MAX_PATH_DEPTH = 128;
const MAX_SEGMENT_BYTES = 255;
const MAX_COMPRESSION_RATIO = 100;
const MAX_GUIDE_BYTES = 256 * 1024;
const LICENSE_SCHEMA = "website-replica-license/v2";
const LICENSE_TYPE = "viceme-replica-license+jws";
const INSTRUCTION_PATTERN = /VICEME-REPLICA:VMR-[A-Z0-9]{20}/gu;
const UUID_PATTERN =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/iu;

class WorkflowError extends Error {
  constructor(code, message, details = {}, exitCode = 1) {
    super(message);
    this.code = code;
    this.details = details;
    this.exitCode = exitCode;
  }
}

function result(data) {
  process.stdout.write(`${JSON.stringify({ ok: true, data }, null, 2)}\n`);
}

function fail(error) {
  const normalized =
    error instanceof WorkflowError
      ? error
      : new WorkflowError(
          "MAKE_COPY_INTERNAL",
          "The let-me-make-a-copy workflow failed",
        );
  process.stdout.write(
    `${JSON.stringify(
      {
        ok: false,
        error: {
          code: normalized.code,
          message: normalized.message,
          ...(Object.keys(normalized.details).length > 0
            ? { details: normalized.details }
            : {}),
        },
      },
      null,
      2,
    )}\n`,
  );
  return normalized.exitCode;
}

function parseArgs(argv) {
  const [command, ...rest] = argv;
  if (!new Set(["inspect", "install"]).has(command)) {
    throw new WorkflowError(
      "MAKE_COPY_COMMAND_INVALID",
      "Expected inspect or install",
    );
  }
  const options = {};
  for (let index = 0; index < rest.length; index += 1) {
    const flag = rest[index];
    if (flag === "--payment-presented") {
      options.paymentPresented = true;
      continue;
    }
    if (!flag?.startsWith("--") || index + 1 >= rest.length) {
      throw new WorkflowError("MAKE_COPY_ARGUMENT_INVALID", "Invalid argument");
    }
    const value = rest[index + 1];
    index += 1;
    if (flag === "--work-url") options.workUrl = value;
    else if (flag === "--target") options.target = value;
    else if (flag === "--accept-price-cents") {
      options.acceptPriceCents = Number(value);
    } else {
      throw new WorkflowError("MAKE_COPY_ARGUMENT_INVALID", "Unknown argument");
    }
  }
  if (!options.workUrl) {
    throw new WorkflowError(
      "MAKE_COPY_WORK_URL_REQUIRED",
      "--work-url is required",
    );
  }
  if (
    command === "install" &&
    (!Number.isInteger(options.acceptPriceCents) ||
      options.acceptPriceCents < 0)
  ) {
    throw new WorkflowError(
      "MAKE_COPY_PRICE_REQUIRED",
      "--accept-price-cents must be a non-negative integer",
    );
  }
  return { command, options };
}

function authorityForWorkUrl(raw) {
  let workUrl;
  try {
    workUrl = new URL(raw);
  } catch {
    throw new WorkflowError(
      "MAKE_COPY_WORK_URL_INVALID",
      "Work URL is invalid",
    );
  }
  if (
    workUrl.protocol !== "https:" ||
    !workUrl.pathname.endsWith(".md") ||
    workUrl.username ||
    workUrl.password ||
    workUrl.port ||
    !new Set(["viceme.cn", "www.viceme.cn", "viceme.ai", "www.viceme.ai"]).has(
      workUrl.hostname,
    )
  ) {
    throw new WorkflowError(
      "MAKE_COPY_WORK_URL_INVALID",
      "Work URL must be an official ViceMe HTTPS .md URL",
    );
  }
  workUrl.hash = "";
  const canonicalHost = workUrl.hostname.endsWith("viceme.ai")
    ? "viceme.ai"
    : "viceme.cn";
  return {
    workUrl,
    webOrigin: `https://${canonicalHost}`,
    apiBaseUrl: `https://${canonicalHost}/api/v1`,
  };
}

async function fetchWorkInstruction(authority, fetchImpl = fetch) {
  const response = await fetchImpl(authority.workUrl, {
    headers: { accept: "text/markdown" },
    redirect: "error",
    signal: AbortSignal.timeout(15_000),
  });
  if (!response.ok) {
    throw new WorkflowError(
      "MAKE_COPY_WORK_UNAVAILABLE",
      "The ViceMe Work could not be read",
    );
  }
  const markdown = await response.text();
  const lines = markdown.split(/\r?\n/u);
  const sectionStart = lines.findIndex(
    (line) =>
      line === "## Platform-controlled complete-source replica entry" ||
      line === "## 平台控制的完整源码做同款入口",
  );
  if (sectionStart < 0) {
    throw new WorkflowError(
      "MAKE_COPY_ENTRY_INVALID",
      "The Work has no platform-controlled let-me-make-a-copy entry",
    );
  }
  const nextSection = lines.findIndex(
    (line, index) => index > sectionStart && line.startsWith("## "),
  );
  const controlledSection = lines
    .slice(sectionStart + 1, nextSection < 0 ? undefined : nextSection)
    .join("\n");
  const instructions = [
    ...new Set(controlledSection.match(INSTRUCTION_PATTERN) ?? []),
  ];
  if (instructions.length !== 1) {
    throw new WorkflowError(
      "MAKE_COPY_ENTRY_INVALID",
      "The Work must expose exactly one Replica instruction",
    );
  }
  return instructions[0];
}

async function apiRequest(
  authority,
  endpoint,
  options = {},
  fetchImpl = fetch,
) {
  const response = await fetchImpl(`${authority.apiBaseUrl}${endpoint}`, {
    method: options.method ?? "GET",
    headers: {
      accept: "application/json",
      ...(options.body === undefined
        ? {}
        : { "content-type": "application/json" }),
      ...(options.token ? { authorization: `Bearer ${options.token}` } : {}),
    },
    ...(options.body === undefined
      ? {}
      : { body: JSON.stringify(options.body) }),
    redirect: "error",
    signal: AbortSignal.timeout(options.timeoutMs ?? 20_000),
  });
  let body;
  try {
    body = JSON.parse(await response.text());
  } catch {
    body = null;
  }
  if (!response.ok) {
    const code =
      typeof body?.code === "string" ? body.code : "MAKE_COPY_API_FAILED";
    const message =
      typeof body?.message === "string"
        ? body.message
        : "ViceMe returned an invalid response";
    throw new WorkflowError(code, message, { statusCode: response.status });
  }
  return body;
}

function assertResolution(value) {
  if (
    !value ||
    !UUID_PATTERN.test(value.replicaId) ||
    !/^VMR-[A-Z0-9]{20}$/u.test(value.shortCode) ||
    typeof value.title !== "string" ||
    typeof value.creator?.displayName !== "string" ||
    typeof value.viceMeWorkUrl !== "string" ||
    !UUID_PATTERN.test(value.product?.id) ||
    !UUID_PATTERN.test(value.product?.skuId) ||
    value.product.currency !== "CNY" ||
    !Number.isInteger(value.product.priceCents) ||
    value.product.priceCents < 0
  ) {
    throw new WorkflowError(
      "MAKE_COPY_RESPONSE_INVALID",
      "ViceMe returned an invalid Replica description",
    );
  }
  return value;
}

async function resolveWork(authority, fetchImpl = fetch) {
  const instruction = await fetchWorkInstruction(authority, fetchImpl);
  const replica = assertResolution(
    await apiRequest(
      authority,
      "/website-replicas/resolve",
      { method: "POST", body: { instruction } },
      fetchImpl,
    ),
  );
  const resolvedWork = new URL(replica.viceMeWorkUrl);
  const expectedPath = authority.workUrl.pathname.slice(0, -3);
  if (
    resolvedWork.origin !== authority.webOrigin ||
    resolvedWork.pathname !== expectedPath ||
    resolvedWork.search ||
    resolvedWork.hash
  ) {
    throw new WorkflowError(
      "MAKE_COPY_RESPONSE_INVALID",
      "Replica Work belongs to a different ViceMe authority",
    );
  }
  return { instruction, replica };
}

function safeTargetName(title) {
  const value = title
    .normalize("NFKC")
    .toLowerCase()
    .replace(/[^\p{Letter}\p{Number}]+/gu, "-")
    .replace(/^-+|-+$/gu, "")
    .slice(0, 48);
  return value || "website-copy";
}

async function resolveTarget(rawTarget, title) {
  const candidate = path.resolve(
    rawTarget ?? path.join(process.cwd(), safeTargetName(title)),
  );
  const parent = path.dirname(candidate);
  const parentReal = await fsp.realpath(parent).catch(() => null);
  if (
    !parentReal ||
    path.basename(candidate) !== path.basename(path.normalize(candidate))
  ) {
    throw new WorkflowError(
      "REPLICA_TARGET_PARENT_INVALID",
      "Target parent must be a real existing directory",
    );
  }
  const info = await fsp.lstat(parentReal);
  if (!info.isDirectory() || info.isSymbolicLink()) {
    throw new WorkflowError(
      "REPLICA_TARGET_PARENT_INVALID",
      "Target parent must be a real existing directory",
    );
  }
  return path.join(parentReal, path.basename(candidate));
}

function stateRoot() {
  if (process.platform === "win32") {
    const base = process.env.LOCALAPPDATA;
    if (!base) {
      throw new WorkflowError(
        "MAKE_COPY_PRIVATE_STATE_UNAVAILABLE",
        "LOCALAPPDATA is required for private recovery state",
      );
    }
    return path.join(base, "ViceMe", "make-copy");
  }
  if (process.platform === "darwin") {
    return path.join(
      os.homedir(),
      "Library",
      "Application Support",
      "ViceMe",
      "make-copy",
    );
  }
  return path.join(
    process.env.XDG_STATE_HOME || path.join(os.homedir(), ".local", "state"),
    "viceme",
    "make-copy",
  );
}

async function currentWindowsSid() {
  const { stdout } = await execFileAsync(
    "whoami",
    ["/user", "/fo", "csv", "/nh"],
    {
      windowsHide: true,
      timeout: 10_000,
    },
  );
  const match = stdout.match(/"(S-1-[0-9-]+)"/u);
  if (!match) throw new Error("could not resolve current Windows SID");
  return match[1];
}

async function protectWindows(target, directory) {
  const sid = await currentWindowsSid();
  const inheritance = directory ? "(OI)(CI)F" : "F";
  await execFileAsync(
    "icacls",
    [
      target,
      "/inheritance:r",
      "/grant:r",
      `*${sid}:${inheritance}`,
      `*S-1-5-18:${inheritance}`,
    ],
    { windowsHide: true, timeout: 15_000 },
  );
}

async function ensurePrivateDirectory(directory) {
  await fsp.mkdir(directory, { recursive: true, mode: 0o700 });
  const info = await fsp.lstat(directory);
  if (!info.isDirectory() || info.isSymbolicLink()) {
    throw new WorkflowError(
      "MAKE_COPY_PRIVATE_STATE_UNAVAILABLE",
      "Recovery state path is not a real directory",
    );
  }
  try {
    if (process.platform === "win32") await protectWindows(directory, true);
    else await fsp.chmod(directory, 0o700);
  } catch {
    throw new WorkflowError(
      "MAKE_COPY_PRIVATE_STATE_UNAVAILABLE",
      "Could not establish private recovery state permissions",
    );
  }
}

async function atomicPrivateWrite(filename, value) {
  const temporary = `${filename}.tmp-${crypto.randomUUID()}`;
  const handle = await fsp.open(temporary, "wx", 0o600);
  try {
    await handle.writeFile(`${JSON.stringify(value)}\n`, "utf8");
    await handle.sync();
  } finally {
    await handle.close();
  }
  try {
    if (process.platform === "win32") await protectWindows(temporary, false);
    else await fsp.chmod(temporary, 0o600);
    await fsp.rename(temporary, filename);
  } catch (error) {
    await fsp.rm(temporary, { force: true });
    throw error;
  }
}

function stateIdentity(authority, shortCode, target) {
  return crypto
    .createHash("sha256")
    .update(`${authority.apiBaseUrl}\n${shortCode}\n${target}`)
    .digest("hex");
}

function paidReceiptPath(authority, shortCode, root = stateRoot()) {
  const paidId = crypto
    .createHash("sha256")
    .update(`${authority.apiBaseUrl}\n${shortCode}`)
    .digest("hex");
  return path.join(root, `paid-${paidId}.json`);
}

async function recoverablePaidReceipt(authority, replica, root) {
  const receipt = await readState(
    paidReceiptPath(authority, replica.shortCode, root),
  );
  if (!receipt) return null;
  if (
    receipt.schemaVersion !== 1 ||
    receipt.replicaId !== replica.replicaId ||
    typeof receipt.orderNo !== "string" ||
    receipt.orderNo.length === 0 ||
    !/^[A-Za-z0-9_-]{43}$/u.test(receipt.recoverySecret ?? "")
  ) {
    throw new WorkflowError(
      "MAKE_COPY_STATE_INVALID",
      "Paid Replica recovery state is invalid",
    );
  }
  return receipt;
}

async function stateStore(authority, shortCode, target) {
  const root = stateRoot();
  await ensurePrivateDirectory(root);
  const id = stateIdentity(authority, shortCode, target);
  const paidReceiptFilename = paidReceiptPath(authority, shortCode, root);
  return {
    filename: path.join(root, `${id}.json`),
    completionFilename: path.join(root, `completed-${id}.json`),
    paidFilename: paidReceiptFilename.replace(/\.json$/u, ".zip"),
    paidReceiptFilename,
    lockDirectory: path.join(root, `${id}.lock`),
  };
}

async function withLock(store, run) {
  const ownerFilename = path.join(store.lockDirectory, "owner.json");
  try {
    await fsp.mkdir(store.lockDirectory, { mode: 0o700 });
  } catch (error) {
    if (error?.code === "EEXIST") {
      const owner = await readLockOwner(ownerFilename);
      if (!owner || processExists(owner.pid)) {
        throw new WorkflowError(
          "MAKE_COPY_ACTIVE",
          "Another let-me-make-a-copy operation is active for this target",
        );
      }
      const staleDirectory = `${store.lockDirectory}.stale-${crypto.randomUUID()}`;
      try {
        await fsp.rename(store.lockDirectory, staleDirectory);
        await fsp.rm(staleDirectory, { recursive: true, force: true });
        await fsp.mkdir(store.lockDirectory, { mode: 0o700 });
      } catch {
        throw new WorkflowError(
          "MAKE_COPY_ACTIVE",
          "Another let-me-make-a-copy operation is active for this target",
        );
      }
    }
    if (error?.code !== "EEXIST") throw error;
  }
  try {
    await ensurePrivateDirectory(store.lockDirectory);
    await atomicPrivateWrite(ownerFilename, { pid: process.pid });
    return await run();
  } finally {
    await fsp.rm(store.lockDirectory, { recursive: true, force: true });
  }
}

async function readLockOwner(filename) {
  try {
    const value = JSON.parse(await fsp.readFile(filename, "utf8"));
    return Number.isSafeInteger(value.pid) && value.pid > 0 ? value : null;
  } catch {
    return null;
  }
}

function processExists(pid) {
  try {
    process.kill(pid, 0);
    return true;
  } catch (error) {
    return error?.code === "EPERM";
  }
}

async function readState(filename) {
  let info;
  try {
    info = await fsp.lstat(filename);
  } catch (error) {
    if (error?.code === "ENOENT") return null;
    throw error;
  }
  if (!info.isFile() || info.isSymbolicLink() || info.size > 64 * 1024) {
    throw new WorkflowError(
      "MAKE_COPY_STATE_INVALID",
      "Recovery state is invalid",
    );
  }
  if (process.platform !== "win32" && (info.mode & 0o077) !== 0) {
    throw new WorkflowError(
      "MAKE_COPY_STATE_INVALID",
      "Recovery state permissions are not private",
    );
  }
  try {
    return JSON.parse(await fsp.readFile(filename, "utf8"));
  } catch {
    throw new WorkflowError(
      "MAKE_COPY_STATE_INVALID",
      "Recovery state is invalid",
    );
  }
}

function newSecret() {
  return crypto.randomBytes(32).toString("base64url");
}

function initialState(authority, instruction, replica, target) {
  return {
    schemaVersion: 1,
    apiBaseUrl: authority.apiBaseUrl,
    instruction,
    shortCode: replica.shortCode,
    replicaId: replica.replicaId,
    productId: replica.product.id,
    skuId: replica.product.skuId,
    priceCents: replica.product.priceCents,
    target,
    sessionClientRequestId: crypto.randomUUID(),
    sessionReplaySecret: newSecret(),
    quoteClientRequestId: crypto.randomUUID(),
    orderClientRequestId: crypto.randomUUID(),
    downloadRecoverySecret: newSecret(),
    createdAt: new Date().toISOString(),
    updatedAt: new Date().toISOString(),
  };
}

function validateState(state, authority, replica, target) {
  if (
    !state ||
    state.schemaVersion !== 1 ||
    state.apiBaseUrl !== authority.apiBaseUrl ||
    state.shortCode !== replica.shortCode ||
    state.replicaId !== replica.replicaId ||
    state.productId !== replica.product.id ||
    state.skuId !== replica.product.skuId ||
    state.priceCents !== replica.product.priceCents ||
    state.target !== target ||
    !UUID_PATTERN.test(state.sessionClientRequestId) ||
    !UUID_PATTERN.test(state.quoteClientRequestId) ||
    typeof state.orderClientRequestId !== "string" ||
    !/^[A-Za-z0-9_-]{43}$/u.test(state.sessionReplaySecret) ||
    !/^[A-Za-z0-9_-]{43}$/u.test(state.downloadRecoverySecret)
  ) {
    throw new WorkflowError(
      "MAKE_COPY_STATE_INVALID",
      "Recovery state is invalid",
    );
  }
  return state;
}

async function persistState(store, state) {
  state.updatedAt = new Date().toISOString();
  await atomicPrivateWrite(store.filename, state);
}

function checkoutResponse(value) {
  if (
    !value ||
    typeof value.orderNo !== "string" ||
    !new Set(["PENDING", "PAID", "CLOSED", "FAILED", "CANCELLED"]).has(
      value.status,
    ) ||
    typeof value.checkoutUrl !== "string"
  ) {
    throw new WorkflowError(
      "MAKE_COPY_RESPONSE_INVALID",
      "ViceMe returned an invalid checkout",
    );
  }
  return value;
}

async function ensureCheckout(authority, state, store, fetchImpl = fetch) {
  const session = await apiRequest(
    authority,
    "/website-replica-sessions",
    {
      method: "POST",
      body: {
        instruction: state.instruction,
        clientRequestId: state.sessionClientRequestId,
        replaySecret: state.sessionReplaySecret,
      },
    },
    fetchImpl,
  );
  if (
    !UUID_PATTERN.test(session?.sessionId) ||
    typeof session?.token !== "string"
  ) {
    throw new WorkflowError(
      "MAKE_COPY_RESPONSE_INVALID",
      "ViceMe returned an invalid anonymous session",
    );
  }
  state.sessionId = session.sessionId;
  state.sessionToken = session.token;
  state.sessionExpiresAt = session.expiresAt;
  await persistState(store, state);
  const checkout = checkoutResponse(
    await apiRequest(
      authority,
      `/website-replica-sessions/${encodeURIComponent(state.sessionId)}/checkout`,
      {
        method: "POST",
        token: state.sessionToken,
        body: {
          acceptedPriceCents: state.priceCents,
          quoteClientRequestId: state.quoteClientRequestId,
          orderClientRequestId: state.orderClientRequestId,
          downloadRecoverySecret: state.downloadRecoverySecret,
          locale: "zh-CN",
        },
      },
      fetchImpl,
    ),
  );
  state.orderNo = checkout.orderNo;
  state.orderExpiresAt = checkout.expiresAt;
  state.checkoutUrl = checkout.checkoutUrl;
  await persistState(store, state);
  await atomicPrivateWrite(store.paidReceiptFilename, {
    schemaVersion: 1,
    replicaId: state.replicaId,
    orderNo: state.orderNo,
    recoverySecret: state.downloadRecoverySecret,
    updatedAt: new Date().toISOString(),
  });
  return checkout;
}

async function tryRecoverDownload(authority, state, fetchImpl = fetch) {
  if (!state.orderNo) return null;
  try {
    return await apiRequest(
      authority,
      "/website-replica-sessions/recover-download-v2",
      {
        method: "POST",
        body: {
          orderNo: state.orderNo,
          recoverySecret: state.downloadRecoverySecret,
        },
      },
      fetchImpl,
    );
  } catch (error) {
    if (
      error instanceof WorkflowError &&
      error.code === "WEBSITE_REPLICA_NOT_FOUND" &&
      error.details.statusCode === 404
    ) {
      return null;
    }
    throw error;
  }
}

async function waitForPayment(
  authority,
  state,
  fetchImpl = fetch,
  sleep = delay,
) {
  for (let attempt = 1; attempt <= 3; attempt += 1) {
    await sleep(60_000);
    const status = await apiRequest(
      authority,
      `/website-replica-sessions/${encodeURIComponent(state.sessionId)}/orders/${encodeURIComponent(state.orderNo)}/status`,
      { token: state.sessionToken },
      fetchImpl,
    );
    if (status?.payment?.status === "PAID") return;
    if (
      new Set(["CLOSED", "FAILED", "CANCELLED"]).has(status?.payment?.status)
    ) {
      throw new WorkflowError(
        "REPLICA_PAYMENT_TERMINAL",
        "Website Replica payment did not complete",
        { orderNo: state.orderNo, status: status.payment.status },
      );
    }
  }
  throw new WorkflowError(
    "REPLICA_PAYMENT_TIMEOUT",
    "Website Replica payment was not observed before the wait deadline",
    { nextAction: "PAYMENT_PENDING", orderNo: state.orderNo },
  );
}

function delay(milliseconds) {
  return new Promise((resolve) => setTimeout(resolve, milliseconds));
}

function decodeJwsPart(value) {
  return JSON.parse(Buffer.from(value, "base64url").toString("utf8"));
}

async function verifyLicense(authority, download, state, fetchImpl = fetch) {
  const parts = String(download?.licenseJws ?? "").split(".");
  if (parts.length !== 3) {
    throw new WorkflowError(
      "REPLICA_LICENSE_INVALID",
      "Replica license is invalid",
    );
  }
  let header;
  let claims;
  try {
    header = decodeJwsPart(parts[0]);
    claims = decodeJwsPart(parts[1]);
  } catch {
    throw new WorkflowError(
      "REPLICA_LICENSE_INVALID",
      "Replica license is invalid",
    );
  }
  if (
    header.alg !== "EdDSA" ||
    header.typ !== LICENSE_TYPE ||
    typeof header.kid !== "string" ||
    claims.schemaVersion !== LICENSE_SCHEMA ||
    claims.entitlementId === undefined ||
    claims.replicaId !== download.replicaId ||
    claims.replicaId !== state.replicaId ||
    claims.versionId !== download.versionId ||
    claims.version !== download.version ||
    claims.orderNo !== state.orderNo ||
    claims.artifactDigest !== download.artifactDigest ||
    typeof claims.licenseTermsVersion !== "string" ||
    Number.isNaN(Date.parse(claims.issuedAt))
  ) {
    throw new WorkflowError(
      "REPLICA_LICENSE_IDENTITY_MISMATCH",
      "Replica license does not match the purchased source",
    );
  }
  const trust = await apiRequest(
    authority,
    `/commerce-skill-trust-keys/${encodeURIComponent(header.kid)}`,
    {},
    fetchImpl,
  );
  if (
    trust?.keyId !== header.kid ||
    trust?.algorithm !== "Ed25519" ||
    typeof trust?.publicKey !== "string"
  ) {
    throw new WorkflowError(
      "REPLICA_LICENSE_SIGNING_KEY_UNTRUSTED",
      "Replica signing key is not trusted",
    );
  }
  let verified = false;
  try {
    verified = crypto.verify(
      null,
      Buffer.from(`${parts[0]}.${parts[1]}`),
      crypto.createPublicKey({
        key: Buffer.from(trust.publicKey, "base64url"),
        format: "der",
        type: "spki",
      }),
      Buffer.from(parts[2], "base64url"),
    );
  } catch {
    verified = false;
  }
  if (!verified) {
    throw new WorkflowError(
      "REPLICA_LICENSE_SIGNATURE_INVALID",
      "Replica license signature is invalid",
    );
  }
  return claims;
}

async function downloadArchive(download, store, fetchImpl = fetch) {
  if (
    !Number.isInteger(download?.sizeBytes) ||
    download.sizeBytes < 1 ||
    download.sizeBytes > MAX_ARCHIVE_BYTES ||
    !/^[a-f0-9]{64}$/u.test(download.artifactDigest) ||
    !/^[^/\\\0]+\.zip$/iu.test(download.fileName)
  ) {
    throw new WorkflowError(
      "REPLICA_DOWNLOAD_RESPONSE_INVALID",
      "Replica download metadata is invalid",
    );
  }
  let url;
  try {
    url = new URL(download.downloadUrl);
  } catch {
    throw new WorkflowError(
      "REPLICA_DOWNLOAD_RESPONSE_INVALID",
      "Replica download URL is invalid",
    );
  }
  if (url.protocol !== "https:") {
    throw new WorkflowError(
      "REPLICA_DOWNLOAD_RESPONSE_INVALID",
      "Replica download URL must use HTTPS",
    );
  }
  const response = await fetchImpl(url, {
    redirect: "error",
    signal: AbortSignal.timeout(120_000),
  });
  if (!response.ok || !response.body) {
    throw new WorkflowError(
      "REPLICA_DOWNLOAD_FAILED",
      "Replica source download failed",
    );
  }
  const temporary = `${store.paidFilename}.download-${crypto.randomUUID()}`;
  const handle = await fsp.open(temporary, "wx", 0o600);
  const hash = crypto.createHash("sha256");
  let written = 0;
  try {
    try {
      for await (const chunk of response.body) {
        written += chunk.length;
        if (written > download.sizeBytes || written > MAX_ARCHIVE_BYTES) {
          throw new WorkflowError(
            "REPLICA_DOWNLOAD_INVALID",
            "Replica source size changed",
          );
        }
        hash.update(chunk);
        await handle.write(chunk);
      }
      await handle.sync();
    } finally {
      await handle.close();
    }
  } catch (error) {
    await fsp.rm(temporary, { force: true });
    throw error;
  }
  if (
    written !== download.sizeBytes ||
    hash.digest("hex") !== download.artifactDigest
  ) {
    await fsp.rm(temporary, { force: true });
    throw new WorkflowError(
      "REPLICA_ARTIFACT_DIGEST_MISMATCH",
      "Replica source does not match its signed digest",
    );
  }
  if (process.platform === "win32") await protectWindows(temporary, false);
  else await fsp.chmod(temporary, 0o600);
  await fsp.rm(store.paidFilename, { force: true });
  await fsp.rename(temporary, store.paidFilename);
  return store.paidFilename;
}

const WINDOWS_RESERVED = /^(?:con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\..*)?$/iu;

function validateArchivePath(name) {
  if (
    !name ||
    name.includes("\\") ||
    name.includes("\0") ||
    name.startsWith("/") ||
    name.normalize("NFC") !== name ||
    Buffer.byteLength(name) > MAX_PATH_BYTES
  ) {
    throw new WorkflowError(
      "REPLICA_ARCHIVE_INVALID",
      "Replica ZIP path is unsafe",
    );
  }
  const trimmed = name.endsWith("/") ? name.slice(0, -1) : name;
  const segments = trimmed.split("/");
  if (
    segments.length === 0 ||
    segments.length > MAX_PATH_DEPTH ||
    segments.some(
      (segment) =>
        !segment ||
        segment === "." ||
        segment === ".." ||
        Buffer.byteLength(segment) > MAX_SEGMENT_BYTES ||
        segment.endsWith(".") ||
        segment.endsWith(" ") ||
        segment.includes(":") ||
        WINDOWS_RESERVED.test(segment),
    ) ||
    segments[0].toLowerCase() === ".viceme"
  ) {
    throw new WorkflowError(
      "REPLICA_ARCHIVE_INVALID",
      "Replica ZIP path is unsafe",
    );
  }
  return segments.join("/");
}

function zipEntrySizes(entry) {
  const data = entry._data ?? {};
  return {
    compressed: Number(data.compressedSize ?? 0),
    expanded: Number(data.uncompressedSize ?? 0),
  };
}

function isZipSymlink(entry) {
  return (
    typeof entry.unixPermissions === "number" &&
    (entry.unixPermissions & 0o170000) === 0o120000
  );
}

function isUnsupportedZipType(entry) {
  if (typeof entry.unixPermissions !== "number") return false;
  const type = entry.unixPermissions & 0o170000;
  return type !== 0 && type !== 0o100000 && type !== 0o040000;
}

async function installArchive(archivePath, target, download) {
  try {
    await fsp.lstat(target);
    throw new WorkflowError(
      "REPLICA_TARGET_EXISTS",
      "Refusing to overwrite target",
      {
        target,
      },
    );
  } catch (error) {
    if (error instanceof WorkflowError) throw error;
    if (error?.code !== "ENOENT") throw error;
  }
  const archive = await fsp.readFile(archivePath);
  const zip = await JSZip.loadAsync(archive, {
    checkCRC32: true,
    createFolders: false,
  });
  const entries = Object.values(zip.files);
  if (entries.length > MAX_ENTRY_COUNT) {
    throw new WorkflowError(
      "REPLICA_ARCHIVE_INVALID",
      "Replica ZIP has too many entries",
    );
  }
  const planned = [];
  const collisionKeys = new Set();
  let fileCount = 0;
  let expandedBytes = 0;
  for (const entry of entries) {
    const unsafeName = entry.unsafeOriginalName;
    if (unsafeName !== undefined && unsafeName !== entry.name) {
      throw new WorkflowError(
        "REPLICA_ARCHIVE_INVALID",
        "Replica ZIP path was sanitized",
      );
    }
    const relative = validateArchivePath(entry.name);
    const key = relative.normalize("NFC").toLocaleLowerCase("und");
    if (collisionKeys.has(key)) {
      throw new WorkflowError(
        "REPLICA_ARCHIVE_INVALID",
        "Replica ZIP paths collide",
      );
    }
    collisionKeys.add(key);
    if (isZipSymlink(entry) || isUnsupportedZipType(entry)) {
      throw new WorkflowError(
        "REPLICA_ARCHIVE_INVALID",
        "Replica ZIP contains an unsupported file type",
      );
    }
    if (!entry.dir) {
      fileCount += 1;
      const sizes = zipEntrySizes(entry);
      if (
        fileCount > MAX_FILE_COUNT ||
        !Number.isSafeInteger(sizes.expanded) ||
        sizes.expanded > MAX_FILE_BYTES ||
        (sizes.compressed === 0 && sizes.expanded > 0) ||
        (sizes.compressed > 0 &&
          sizes.expanded > sizes.compressed * MAX_COMPRESSION_RATIO)
      ) {
        throw new WorkflowError(
          "REPLICA_ARCHIVE_INVALID",
          "Replica ZIP exceeds limits",
        );
      }
      expandedBytes += sizes.expanded;
      if (expandedBytes > MAX_EXPANDED_BYTES) {
        throw new WorkflowError(
          "REPLICA_ARCHIVE_INVALID",
          "Replica ZIP expands too large",
        );
      }
    }
    planned.push({ entry, relative });
  }
  const guide = planned.find(
    ({ entry, relative }) => !entry.dir && relative === "VICEME-REPLICA.md",
  );
  if (!guide) {
    throw new WorkflowError(
      "REPLICA_DEPLOYMENT_GUIDE_INVALID",
      "Replica ZIP has no root VICEME-REPLICA.md",
    );
  }
  const staging = path.join(
    path.dirname(target),
    `.viceme-replica-stage-${crypto.randomUUID()}`,
  );
  await fsp.mkdir(staging, { mode: 0o700 });
  try {
    for (const { entry, relative } of planned) {
      const destination = path.join(staging, ...relative.split("/"));
      if (entry.dir) {
        await fsp.mkdir(destination, { recursive: true, mode: 0o755 });
        continue;
      }
      await fsp.mkdir(path.dirname(destination), {
        recursive: true,
        mode: 0o755,
      });
      const body = await entry.async("nodebuffer");
      const expected = zipEntrySizes(entry).expanded;
      if (body.length !== expected) {
        throw new WorkflowError(
          "REPLICA_ARCHIVE_INVALID",
          "Replica ZIP entry size changed",
        );
      }
      await fsp.writeFile(destination, body, { flag: "wx", mode: 0o600 });
      const executable =
        typeof entry.unixPermissions === "number" &&
        (entry.unixPermissions & 0o111) !== 0;
      if (process.platform !== "win32") {
        await fsp.chmod(destination, executable ? 0o755 : 0o644);
      }
    }
    const guideBody = await fsp.readFile(
      path.join(staging, "VICEME-REPLICA.md"),
    );
    const guideText = guideBody.toString("utf8");
    if (
      guideBody.length < 1 ||
      guideBody.length > MAX_GUIDE_BYTES ||
      Buffer.from(guideText, "utf8").compare(guideBody) !== 0 ||
      guideText.trim() === ""
    ) {
      throw new WorkflowError(
        "REPLICA_DEPLOYMENT_GUIDE_INVALID",
        "Replica deployment guide is invalid",
      );
    }
    await fsp.mkdir(path.join(staging, ".viceme"), { mode: 0o700 });
    await fsp.writeFile(
      path.join(staging, ".viceme", "replica-license.json"),
      `${JSON.stringify({
        schemaVersion: 1,
        replicaId: download.replicaId,
        versionId: download.versionId,
        version: download.version,
        artifactDigest: download.artifactDigest,
        licenseJws: download.licenseJws,
      })}\n`,
      { flag: "wx", mode: 0o600 },
    );
    await fsp.rename(staging, target);
  } catch (error) {
    await fsp.rm(staging, { recursive: true, force: true });
    throw error;
  }
  return { target, fileCount, expandedBytes };
}

async function completeInstall(
  authority,
  state,
  store,
  download,
  fetchImpl = fetch,
) {
  const claims = await verifyLicense(authority, download, state, fetchImpl);
  const archivePath = await downloadArchive(download, store, fetchImpl);
  await atomicPrivateWrite(store.paidReceiptFilename, {
    schemaVersion: 1,
    replicaId: download.replicaId,
    versionId: download.versionId,
    version: download.version,
    orderNo: state.orderNo,
    artifactDigest: download.artifactDigest,
    sizeBytes: download.sizeBytes,
    licenseJws: download.licenseJws,
    recoverySecret: state.downloadRecoverySecret,
    paidAt: claims.issuedAt,
  });
  const installed = await installArchive(archivePath, state.target, download);
  const completion = {
    schemaVersion: 1,
    ...installed,
    replicaId: download.replicaId,
    versionId: download.versionId,
    version: download.version,
    orderNo: state.orderNo,
    artifactDigest: download.artifactDigest,
    completedAt: new Date().toISOString(),
  };
  await atomicPrivateWrite(store.completionFilename, completion);
  await fsp.rm(store.filename, { force: true });
  return completion;
}

async function inspect(options, dependencies = {}) {
  const authority = authorityForWorkUrl(options.workUrl);
  const { instruction, replica } = await resolveWork(
    authority,
    dependencies.fetch,
  );
  return {
    nextAction: "OPEN_WORK_PREVIEW",
    workUrl: replica.viceMeWorkUrl,
    instruction,
    standaloneRecoveryAvailable: Boolean(
      await recoverablePaidReceipt(authority, replica, dependencies.stateRoot),
    ),
    replica,
  };
}

async function install(options, dependencies = {}) {
  const fetchImpl = dependencies.fetch ?? fetch;
  const authority = authorityForWorkUrl(options.workUrl);
  const { instruction, replica } = await resolveWork(authority, fetchImpl);
  if (replica.product.priceCents !== options.acceptPriceCents) {
    throw new WorkflowError(
      "REPLICA_PRICE_CHANGED",
      "Replica price changed; show the Work again and ask for confirmation",
      {
        nextAction: "OPEN_WORK_PREVIEW",
        workUrl: replica.viceMeWorkUrl,
        priceCents: replica.product.priceCents,
      },
      10,
    );
  }
  const target = await resolveTarget(options.target, replica.title);
  const store = await stateStore(authority, replica.shortCode, target);
  return withLock(store, async () => {
    const completion = await readState(store.completionFilename);
    if (completion) {
      const targetInfo = await fsp.lstat(completion.target).catch(() => null);
      if (!targetInfo?.isDirectory()) {
        throw new WorkflowError(
          "REPLICA_COMPLETION_TARGET_INVALID",
          "Completed Replica target is unavailable",
        );
      }
      return { ...completion, nextAction: "DEPLOY" };
    }
    let state = await readState(store.filename);
    if (state) state = validateState(state, authority, replica, target);
    else {
      const targetInfo = await fsp.lstat(target).catch(() => null);
      if (targetInfo) {
        throw new WorkflowError(
          "REPLICA_TARGET_EXISTS",
          "Refusing to overwrite target",
          {
            target,
          },
        );
      }
      state = initialState(authority, instruction, replica, target);
      await persistState(store, state);
    }
    const paidReceipt = await recoverablePaidReceipt(authority, replica);
    if (!state.orderNo && paidReceipt) {
      state.orderNo = paidReceipt.orderNo;
      state.downloadRecoverySecret = paidReceipt.recoverySecret;
      await persistState(store, state);
    }
    let download = await tryRecoverDownload(authority, state, fetchImpl);
    if (download)
      return {
        ...(await completeInstall(
          authority,
          state,
          store,
          download,
          fetchImpl,
        )),
        nextAction: "DEPLOY",
      };
    const checkout = await ensureCheckout(authority, state, store, fetchImpl);
    if (checkout.status === "PAID") {
      download = await tryRecoverDownload(authority, state, fetchImpl);
      if (!download) {
        throw new WorkflowError(
          "REPLICA_DOWNLOAD_PENDING",
          "Paid Replica download is not available yet",
        );
      }
      return {
        ...(await completeInstall(
          authority,
          state,
          store,
          download,
          fetchImpl,
        )),
        nextAction: "DEPLOY",
      };
    }
    if (checkout.status !== "PENDING") {
      throw new WorkflowError(
        "REPLICA_PAYMENT_TERMINAL",
        "Website Replica payment did not complete",
      );
    }
    if (!options.paymentPresented) {
      throw new WorkflowError(
        "REPLICA_PAYMENT_REQUIRED",
        "Open the hosted ViceMe payment page",
        { nextAction: "OPEN_PAYMENT_PAGE", checkoutUrl: checkout.checkoutUrl },
        10,
      );
    }
    await waitForPayment(
      authority,
      state,
      fetchImpl,
      dependencies.sleep ?? delay,
    );
    download = await tryRecoverDownload(authority, state, fetchImpl);
    if (!download) {
      throw new WorkflowError(
        "REPLICA_DOWNLOAD_PENDING",
        "Paid Replica download is not available yet",
      );
    }
    return {
      ...(await completeInstall(authority, state, store, download, fetchImpl)),
      nextAction: "DEPLOY",
    };
  });
}

async function main(argv = process.argv.slice(2)) {
  try {
    const { command, options } = parseArgs(argv);
    result(
      command === "inspect" ? await inspect(options) : await install(options),
    );
    return 0;
  } catch (error) {
    return fail(error);
  }
}

if (require.main === module) {
  main().then(
    (code) => {
      process.exitCode = code;
    },
    () => {
      process.exitCode = fail(
        new WorkflowError(
          "MAKE_COPY_INTERNAL",
          "The let-me-make-a-copy workflow failed",
        ),
      );
    },
  );
}

module.exports = {
  WorkflowError,
  authorityForWorkUrl,
  ensureCheckout,
  fetchWorkInstruction,
  inspect,
  install,
  installArchive,
  parseArgs,
  validateArchivePath,
  verifyLicense,
  waitForPayment,
  withLock,
};
