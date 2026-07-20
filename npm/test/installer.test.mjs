import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { createHash } from "node:crypto";
import { access, mkdir, mkdtemp, readFile, readdir, writeFile } from "node:fs/promises";
import { createServer } from "node:http";
import { once } from "node:events";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import {
  binaryDownloadURLs,
  ensureBinary,
  releaseAssetName,
  releaseTarget,
} from "../lib/installer.mjs";

test("maps supported Node targets to Go release assets", () => {
  assert.deepEqual(releaseTarget("darwin", "arm64"), {
    operatingSystem: "darwin",
    architecture: "arm64",
    extension: "",
  });
  for (const [platform, architecture, asset] of [
    ["darwin", "x64", "viceme_0.1.0_darwin_amd64"],
    ["darwin", "arm64", "viceme_0.1.0_darwin_arm64"],
    ["linux", "x64", "viceme_0.1.0_linux_amd64"],
    ["linux", "arm64", "viceme_0.1.0_linux_arm64"],
    ["win32", "x64", "viceme_0.1.0_windows_amd64.exe"],
    ["win32", "arm64", "viceme_0.1.0_windows_arm64.exe"],
  ]) {
    assert.equal(releaseAssetName("0.1.0", platform, architecture), asset);
  }
  assert.throws(() => releaseTarget("freebsd", "x64"), /unsupported platform/);
});

test("orders GitHub, configured registry mirror, and public npmmirror sources", () => {
  const common = {
    packageVersion: "0.1.0",
    asset: "viceme_0.1.0_linux_amd64",
  };
  assert.deepEqual(binaryDownloadURLs({ ...common, environment: {} }), [
    "https://github.com/ViceMe-AI/cli/releases/download/v0.1.0/viceme_0.1.0_linux_amd64",
    "https://registry.npmmirror.com/-/binary/viceme-cli/v0.1.0/viceme_0.1.0_linux_amd64",
  ]);
  assert.deepEqual(
    binaryDownloadURLs({
      ...common,
      environment: { npm_config_registry: "https://npm.corp.example/repository/npm/" },
    }),
    [
      "https://github.com/ViceMe-AI/cli/releases/download/v0.1.0/viceme_0.1.0_linux_amd64",
      "https://npm.corp.example/repository/npm/-/binary/viceme-cli/v0.1.0/viceme_0.1.0_linux_amd64",
      "https://registry.npmmirror.com/-/binary/viceme-cli/v0.1.0/viceme_0.1.0_linux_amd64",
    ],
  );
});

test("does not add npmjs, insecure, invalid, or duplicate registry sources", () => {
  const common = {
    packageVersion: "0.1.0",
    asset: "viceme_0.1.0_linux_amd64",
  };
  for (const registry of [
    "https://registry.npmjs.org/",
    "http://npm.corp.example/",
    "not a url",
    "https://registry.npmmirror.com/",
  ]) {
    assert.deepEqual(
      binaryDownloadURLs({
        ...common,
        environment: { npm_config_registry: registry },
      }),
      [
        "https://github.com/ViceMe-AI/cli/releases/download/v0.1.0/viceme_0.1.0_linux_amd64",
        "https://registry.npmmirror.com/-/binary/viceme-cli/v0.1.0/viceme_0.1.0_linux_amd64",
      ],
    );
  }
});

test("downloads an exact-version binary, verifies checksum, and reuses cache", async () => {
  const cacheDirectory = await mkdtemp(path.join(os.tmpdir(), "viceme-npm-test-"));
  const binary = Buffer.from("verified test binary\n");
  const asset = "viceme_0.1.0_linux_amd64";
  let downloads = 0;
  const downloadImplementation = async () => {
    downloads += 1;
    return binary;
  };
  const options = {
    packageVersion: "0.1.0",
    platform: "linux",
    architecture: "x64",
    cacheDirectory,
    sourceBaseURLs: ["http://release.test"],
    allowInsecureURL: true,
    downloadImplementation,
    checksumsDocument: checksumDocument(asset, binary),
    environment: {},
  };
  const installed = await ensureBinary(options);
  assert.deepEqual(await readFile(installed), binary);
  assert.equal(downloads, 1);
  assert.equal(await ensureBinary(options), installed);
  assert.equal(downloads, 1);
});

test("falls back in order and verifies every source against the bundled checksum", async () => {
  const cacheDirectory = await mkdtemp(path.join(os.tmpdir(), "viceme-npm-fallback-"));
  const binary = Buffer.from("verified fallback binary\n");
  const asset = "viceme_0.1.0_linux_amd64";
  const attempted = [];
  const installed = await ensureBinary({
    packageVersion: "0.1.0",
    platform: "linux",
    architecture: "x64",
    cacheDirectory,
    sourceBaseURLs: ["https://first.example", "https://second.example"],
    checksumsDocument: checksumDocument(asset, binary),
    downloadImplementation: async (url) => {
      attempted.push(url);
      if (url.startsWith("https://first.example/")) {
        throw new Error("temporary network failure");
      }
      return binary;
    },
    environment: {},
  });
  assert.deepEqual(await readFile(installed), binary);
  assert.deepEqual(attempted, [
    `https://first.example/v0.1.0/${asset}`,
    `https://second.example/v0.1.0/${asset}`,
  ]);
});

test("rejects a release binary whose checksum does not match", async () => {
  const cacheDirectory = await mkdtemp(path.join(os.tmpdir(), "viceme-npm-test-"));
  const asset = "viceme_0.1.0_linux_amd64";
  await assert.rejects(
    ensureBinary({
      packageVersion: "0.1.0",
      platform: "linux",
      architecture: "x64",
      cacheDirectory,
      sourceBaseURLs: ["http://release.test"],
      allowInsecureURL: true,
      downloadImplementation: async () => Buffer.from("tampered"),
      checksumsDocument: `${"0".repeat(64)}  ${asset}\n`,
      environment: {},
    }),
    /attempted:[\s\S]*checksum mismatch/,
  );
});

test("reports every attempted source when no verified binary is available", async () => {
  const cacheDirectory = await mkdtemp(path.join(os.tmpdir(), "viceme-npm-failures-"));
  const asset = "viceme_0.1.0_linux_amd64";
  await assert.rejects(
    ensureBinary({
      packageVersion: "0.1.0",
      platform: "linux",
      architecture: "x64",
      cacheDirectory,
      sourceBaseURLs: ["https://first.example", "https://second.example"],
      checksumsDocument: `${"0".repeat(64)}  ${asset}\n`,
      downloadImplementation: async (url) => {
        throw new Error(url.includes("first") ? "HTTP 404" : "connection refused");
      },
      environment: {},
    }),
    /first\.example[\s\S]*HTTP 404[\s\S]*second\.example[\s\S]*connection refused/,
  );
});

test("publishes a new Windows generation instead of mutating a corrupted one", async () => {
  const cacheDirectory = await mkdtemp(path.join(os.tmpdir(), "viceme-npm-windows-"));
  const binary = Buffer.from("verified windows binary\n");
  const asset = "viceme_0.1.0_windows_amd64.exe";
  const options = {
    packageVersion: "0.1.0",
    platform: "win32",
    architecture: "x64",
    cacheDirectory,
    sourceBaseURLs: ["http://release.test"],
    allowInsecureURL: true,
    downloadImplementation: async () => binary,
    checksumsDocument: checksumDocument(asset, binary),
    environment: {},
  };
  const installed = await ensureBinary(options);
  await writeFile(installed, "corrupted cache entry\n");
  const replacement = await ensureBinary(options);
  assert.notEqual(replacement, installed);
  assert.deepEqual(await readFile(installed), Buffer.from("corrupted cache entry\n"));
  assert.deepEqual(await readFile(replacement), binary);
});

test("concurrent cold starts publish only complete immutable generations", async () => {
  const cacheDirectory = await mkdtemp(path.join(os.tmpdir(), "viceme-npm-concurrent-"));
  const binary = Buffer.from("one verified concurrent binary\n");
  const checksum = createHash("sha256").update(binary).digest("hex");
  let requests = 0;
  const server = createServer((request, response) => {
    requests += 1;
    setTimeout(() => {
      response.end(binary);
    }, 100);
  });
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  const address = server.address();
  const releaseBaseURL = `http://127.0.0.1:${address.port}`;
  const child = new URL("./installer-child.mjs", import.meta.url);
  const checksumsDocument = checksumDocument(
    "viceme_0.1.0_linux_amd64",
    binary,
  );
  try {
    const installed = await Promise.all([
      runNodeChild(child, cacheDirectory, releaseBaseURL, checksumsDocument),
      runNodeChild(child, cacheDirectory, releaseBaseURL, checksumsDocument),
      runNodeChild(child, cacheDirectory, releaseBaseURL, checksumsDocument),
      runNodeChild(child, cacheDirectory, releaseBaseURL, checksumsDocument),
    ]);
    assert.ok(requests >= 1 && requests <= 4);
    for (const installedPath of installed.map((value) => value.trim())) {
      assert.match(installedPath, /[/\\]generations[/\\]generation-/);
      assert.deepEqual(await readFile(installedPath), binary);
    }
    await assert.rejects(access(path.join(cacheDirectory, "cli", "0.1.0", ".install-lock")));
    await assertEveryPublishedGenerationIsComplete(cacheDirectory, binary, checksum);
  } finally {
    server.close();
    await once(server, "close");
  }
});

test("multiple contenders ignore crashed reaper state without fencing an old live owner", async () => {
  const cacheDirectory = await mkdtemp(path.join(os.tmpdir(), "viceme-npm-stale-lock-"));
  const destinationDirectory = path.join(cacheDirectory, "cli", "0.1.0");
  const lockDirectory = path.join(destinationDirectory, ".install-lock");
  const reapClaim = path.join(lockDirectory, ".reap-claim");
  await mkdir(reapClaim, { recursive: true, mode: 0o700 });
  const oldLiveOwner = `${JSON.stringify({
    pid: process.pid,
    token: "live-for-more-than-24-hours",
    created_at: new Date(Date.now() - 48 * 60 * 60 * 1_000).toISOString(),
  })}\n`;
  await writeFile(path.join(lockDirectory, "owner.json"), oldLiveOwner, { mode: 0o600 });
  await writeFile(
    path.join(reapClaim, "owner.json"),
    `${JSON.stringify({ pid: 2_147_483_647, token: "crashed-reaper" })}\n`,
    { mode: 0o600 },
  );
  const binary = Buffer.from("verified after stale lock\n");
  const checksum = createHash("sha256").update(binary).digest("hex");
  let requests = 0;
  const server = createServer((request, response) => {
    requests += 1;
    setTimeout(() => {
      response.end(binary);
    }, 100);
  });
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  const address = server.address();
  const child = new URL("./installer-child.mjs", import.meta.url);
  const checksumsDocument = checksumDocument(
    "viceme_0.1.0_linux_amd64",
    binary,
  );
  try {
    const installed = await Promise.all([
      runNodeChild(
        child,
        cacheDirectory,
        `http://127.0.0.1:${address.port}`,
        checksumsDocument,
      ),
      runNodeChild(
        child,
        cacheDirectory,
        `http://127.0.0.1:${address.port}`,
        checksumsDocument,
      ),
      runNodeChild(
        child,
        cacheDirectory,
        `http://127.0.0.1:${address.port}`,
        checksumsDocument,
      ),
    ]);
    assert.ok(requests >= 1 && requests <= 3);
    for (const installedPath of installed.map((value) => value.trim())) {
      assert.deepEqual(await readFile(installedPath), binary);
    }
    assert.equal(await readFile(path.join(lockDirectory, "owner.json"), "utf8"), oldLiveOwner);
    await access(path.join(reapClaim, "owner.json"));
    await assertEveryPublishedGenerationIsComplete(cacheDirectory, binary, checksum);
  } finally {
    server.close();
    await once(server, "close");
  }
});

test("ignores crash-left staging and corrupt generations without deleting them", async () => {
  const cacheDirectory = await mkdtemp(path.join(os.tmpdir(), "viceme-npm-crash-leftovers-"));
  const destinationDirectory = path.join(cacheDirectory, "cli", "0.1.0");
  const stagingDirectory = path.join(destinationDirectory, ".staging-crashed-owner");
  const corruptGeneration = path.join(
    destinationDirectory,
    "generations",
    "generation-corrupt",
  );
  const asset = "viceme_0.1.0_linux_amd64";
  await mkdir(stagingDirectory, { recursive: true, mode: 0o700 });
  await writeFile(path.join(stagingDirectory, asset), "partial write\n", { mode: 0o700 });
  await mkdir(corruptGeneration, { recursive: true, mode: 0o700 });
  await writeFile(path.join(corruptGeneration, asset), "corrupt generation\n", { mode: 0o700 });
  await writeFile(`${path.join(corruptGeneration, asset)}.sha256`, `${"0".repeat(64)}  ${asset}\n`);

  const binary = Buffer.from("verified after process crash\n");
  const checksum = createHash("sha256").update(binary).digest("hex");
  const installed = await ensureBinary({
    packageVersion: "0.1.0",
    platform: "linux",
    architecture: "x64",
    cacheDirectory,
    sourceBaseURLs: ["http://release.test"],
    allowInsecureURL: true,
    downloadImplementation: async () => binary,
    checksumsDocument: checksumDocument(asset, binary),
    environment: {},
  });
  assert.deepEqual(await readFile(installed), binary);
  assert.equal(await readFile(path.join(stagingDirectory, asset), "utf8"), "partial write\n");
  assert.equal(
    await readFile(path.join(corruptGeneration, asset), "utf8"),
    "corrupt generation\n",
  );
});

test("supports an explicit local binary for package smoke tests", async () => {
  const directory = await mkdtemp(path.join(os.tmpdir(), "viceme-npm-path-"));
  const binary = path.join(directory, "viceme");
  await writeFile(binary, "test\n", { mode: 0o700 });
  assert.equal(
    await ensureBinary({
      packageVersion: "0.1.0",
      environment: { VICEME_BINARY_PATH: binary },
    }),
    binary,
  );
});

function runNodeChild(moduleURL, cacheDirectory, releaseBaseURL, checksumsDocument) {
  return new Promise((resolve, reject) => {
    const child = spawn(
      process.execPath,
      [fileURLToPath(moduleURL), cacheDirectory, releaseBaseURL, checksumsDocument],
      { stdio: ["ignore", "pipe", "pipe"] },
    );
    let stdout = "";
    let stderr = "";
    child.stdout.on("data", (chunk) => {
      stdout += chunk;
    });
    child.stderr.on("data", (chunk) => {
      stderr += chunk;
    });
    child.on("error", reject);
    child.on("close", (code) => {
      if (code === 0) {
        resolve(stdout);
      } else {
        reject(new Error(`installer child exited ${code}: ${stderr}`));
      }
    });
  });
}

function checksumDocument(asset, binary) {
  const checksum = createHash("sha256").update(binary).digest("hex");
  return `${checksum}  ${asset}\n`;
}

async function assertEveryPublishedGenerationIsComplete(cacheDirectory, binary, checksum) {
  const generationsDirectory = path.join(cacheDirectory, "cli", "0.1.0", "generations");
  const entries = await readdir(generationsDirectory, { withFileTypes: true });
  const generations = entries.filter(
    (entry) => entry.isDirectory() && entry.name.startsWith("generation-"),
  );
  assert.ok(generations.length >= 1);
  for (const generation of generations) {
    const binaryPath = path.join(
      generationsDirectory,
      generation.name,
      "viceme_0.1.0_linux_amd64",
    );
    assert.deepEqual(await readFile(binaryPath), binary);
    assert.match(await readFile(`${binaryPath}.sha256`, "utf8"), new RegExp(`^${checksum}\\s`));
  }
}
