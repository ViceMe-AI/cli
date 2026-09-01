#!/usr/bin/env node

const exactSemver = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/;

const requiredExports = {
  ".": "./dist/index.js",
  "./danmaku": "./dist/danmaku.js",
  "./tip": "./dist/tip.js",
  "./tip/testing": "./dist/tip/testing.js",
};

const requiredManifestFiles = ["index.js", "danmaku.js", "tip.js", "tip/testing.js"];

export function resolveSdkVersion(metadata) {
  if (!metadata || typeof metadata !== "object" || Array.isArray(metadata)) {
    throw new Error("npm metadata must be an object");
  }
  if (typeof metadata.version !== "string" || !exactSemver.test(metadata.version)) {
    throw new Error("npm metadata version must be an exact stable semver");
  }
  for (const [subpath, expectedImport] of Object.entries(requiredExports)) {
    if (metadata.exports?.[subpath]?.import !== expectedImport) {
      throw new Error(`npm package is missing the required ${subpath} export`);
    }
  }
  return metadata.version;
}

export function validateSdkManifest(manifest, expectedVersion) {
  if (!exactSemver.test(expectedVersion)) {
    throw new Error("expected version must be an exact stable semver");
  }
  if (!manifest || typeof manifest !== "object" || Array.isArray(manifest)) {
    throw new Error("SDK manifest must be an object");
  }
  if (manifest.version !== expectedVersion) {
    throw new Error("SDK manifest version does not match the resolved npm version");
  }
  if (manifest.apiMajor !== 1) {
    throw new Error("SDK manifest uses an unsupported API major");
  }
  if (
    manifest.features?.danmaku !== "danmaku.js" ||
    manifest.features?.tip !== "tip.js"
  ) {
    throw new Error("SDK manifest does not expose the required hosted features");
  }
  for (const file of requiredManifestFiles) {
    if (!manifest.files?.[file] || typeof manifest.files[file] !== "object") {
      throw new Error(`SDK manifest is missing the required ${file} artifact`);
    }
  }
}

export function validateSdkManifestResponse(response, expectedVersion) {
  const statusSeparator = response.lastIndexOf("\n");
  if (statusSeparator < 0) {
    throw new Error("SDK manifest response is missing its HTTP status");
  }
  const status = response.slice(statusSeparator + 1);
  if (status !== "200") {
    throw new Error(`SDK manifest returned HTTP ${status || "unknown"}`);
  }
  const manifest = JSON.parse(response.slice(0, statusSeparator));
  validateSdkManifest(manifest, expectedVersion);
}

async function readStdin() {
  let input = "";
  process.stdin.setEncoding("utf8");
  for await (const chunk of process.stdin) input += chunk;
  return input;
}

async function main() {
  const [mode, expectedVersion] = process.argv.slice(2);
  const input = await readStdin();
  if (mode === "package") {
    process.stdout.write(`${resolveSdkVersion(JSON.parse(input))}\n`);
    return;
  }
  if (mode === "manifest-response" && expectedVersion) {
    validateSdkManifestResponse(input, expectedVersion);
    return;
  }
  throw new Error("usage: validate-sdk-release.mjs package | manifest-response <version>");
}

if (process.argv[1]?.endsWith("validate-sdk-release.mjs")) {
  main().catch((error) => {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  });
}
