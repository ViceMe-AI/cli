#!/usr/bin/env node

import { execFile } from "node:child_process";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);
const exactSemver = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/;
const requiredExports = {
  ".": "./dist/index.js",
  "./danmaku": "./dist/danmaku.js",
  "./tip": "./dist/tip.js",
  "./tip/testing": "./dist/tip/testing.js",
};
const requiredManifestFiles = ["index.js", "danmaku.js", "tip.js", "tip/testing.js"];
const sdkOrigins = {
  cn: "https://s3.viceme.cn",
  global: "https://s3.viceme.ai",
};
const routeConfigs = {
  danmaku: { regions: ["cn", "global"], files: ["index.js", "danmaku.js"] },
  tip: { regions: ["cn"], files: ["index.js", "tip.js"] },
  combined: {
    regions: ["cn"],
    files: ["index.js", "danmaku.js", "tip.js"],
  },
};

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
  if (manifest.integrations?.engagement !== "danmaku-tip-v1") {
    throw new Error("SDK manifest does not expose integrated engagement capability");
  }
  for (const file of requiredManifestFiles) {
    if (!manifest.files?.[file] || typeof manifest.files[file] !== "object") {
      throw new Error(`SDK manifest is missing the required ${file} artifact`);
    }
  }
}

async function defaultRunNpm(args) {
  try {
    const { stdout } = await execFileAsync("npm", args, {
      encoding: "utf8",
      maxBuffer: 256 * 1024,
      timeout: 20_000,
    });
    return stdout;
  } catch {
    throw new Error("SDK npm metadata lookup failed");
  }
}

async function request(fetchImpl, url) {
  return fetchImpl(url, {
    headers: { Accept: "application/json, text/javascript;q=0.9, */*;q=0.1" },
    redirect: "error",
    signal: AbortSignal.timeout(15_000),
  });
}

async function readManifest(fetchImpl, origin, version) {
  const url = `${origin}/viceme-sdk/${version}/manifest.json`;
  const response = await request(fetchImpl, url);
  if (response.status !== 200) {
    await response.body?.cancel();
    throw new Error(`SDK manifest returned HTTP ${response.status}`);
  }
  const manifest = JSON.parse(await response.text());
  validateSdkManifest(manifest, version);
  return url;
}

async function checkAsset(fetchImpl, url) {
  const response = await request(fetchImpl, url);
  await response.body?.cancel();
  if (response.status !== 200) {
    throw new Error(`SDK asset returned HTTP ${response.status}: ${url}`);
  }
  return url;
}

export async function preflightSdkRelease({
  route,
  region,
  runNpm = defaultRunNpm,
  fetchImpl = globalThis.fetch,
}) {
  if (!Object.hasOwn(routeConfigs, route)) {
    throw new Error("route must be danmaku, tip, or combined");
  }
  if (!Object.hasOwn(sdkOrigins, region)) {
    throw new Error("region must be cn or global");
  }
  const routeConfig = routeConfigs[route];
  if (!routeConfig.regions.includes(region)) {
    throw new Error("Tip routes require the cn region");
  }
  if (typeof fetchImpl !== "function") {
    throw new Error("this Node.js runtime does not provide fetch");
  }

  const npmArgs = [
    "view",
    "@viceme-ai/sdk@latest",
    "version",
    "exports",
    "--json",
    "--fetch-timeout=15000",
    "--fetch-retries=0",
    "--registry=https://registry.npmjs.org",
    "--@viceme-ai:registry=https://registry.npmjs.org",
  ];
  const metadata = JSON.parse(await runNpm(npmArgs));
  const version = resolveSdkVersion(metadata);
  const selectedOrigin = sdkOrigins[region];
  const assetURLs = routeConfig.files.map(
    (file) => `${selectedOrigin}/viceme-sdk/${version}/${file}`,
  );
  const [manifest, ...assets] = await Promise.all([
    readManifest(fetchImpl, selectedOrigin, version),
    ...assetURLs.map((url) => checkAsset(fetchImpl, url)),
  ]);

  return {
    route,
    region,
    sdk_version: version,
    sdk_origin: selectedOrigin,
    checked: { manifest, assets },
  };
}

function parseOptions(args) {
  const options = {};
  for (let index = 0; index < args.length; index += 2) {
    const flag = args[index];
    const value = args[index + 1];
    if ((flag !== "--route" && flag !== "--region") || !value) {
      throw new Error(
        "usage: preflight-sdk-release.mjs --route <danmaku|tip|combined> --region <cn|global>",
      );
    }
    options[flag.slice(2)] = value;
  }
  return options;
}

async function main() {
  const { route, region } = parseOptions(process.argv.slice(2));
  const result = await preflightSdkRelease({ route, region });
  process.stdout.write(`${JSON.stringify(result)}\n`);
}

if (process.argv[1]?.endsWith("preflight-sdk-release.mjs")) {
  main().catch((error) => {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  });
}
