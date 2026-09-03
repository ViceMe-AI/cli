import assert from "node:assert/strict";
import test from "node:test";

import {
  preflightSdkRelease,
  resolveSdkVersion,
  validateSdkManifest,
} from "../../skills/let-people-interact/scripts/preflight-sdk-release.mjs";

function compatiblePackage(version = "12.34.56") {
  return {
    version,
    exports: {
      ".": { import: "./dist/index.js" },
      "./danmaku": { import: "./dist/danmaku.js" },
      "./tip": { import: "./dist/tip.js" },
      "./tip/testing": { import: "./dist/tip/testing.js" },
    },
  };
}

function compatibleManifest(version = "12.34.56") {
  return {
    version,
    apiMajor: 1,
    features: { danmaku: "danmaku.js", tip: "tip.js" },
    integrations: { engagement: "danmaku-tip-v1" },
    files: {
      "index.js": {},
      "danmaku.js": {},
      "tip.js": {},
      "tip/testing.js": {},
    },
  };
}

async function assertRejectedBeforeNetwork(route, region, pattern) {
  let called = false;
  await assert.rejects(
    preflightSdkRelease({
      route,
      region,
      runNpm: async () => {
        called = true;
        return JSON.stringify(compatiblePackage());
      },
      fetchImpl: async () => {
        called = true;
        return new Response(null, { status: 200 });
      },
    }),
    pattern,
  );
  assert.equal(called, false);
}

test("resolves a compatible exact stable SDK version", () => {
  assert.equal(resolveSdkVersion(compatiblePackage()), "12.34.56");
});

test("rejects old package metadata without Headless Tip", () => {
  const oldPackage = compatiblePackage("0.4.0");
  delete oldPackage.exports["./tip/testing"];
  assert.throws(() => resolveSdkVersion(oldPackage), /\.\/tip\/testing/);
});

test("rejects non-exact and prerelease versions", () => {
  for (const version of ["latest", "1.2", "01.2.3", "1.2.3-beta.1"]) {
    assert.throws(() => resolveSdkVersion(compatiblePackage(version)), /exact stable semver/);
  }
});

test("accepts a matching API v1 release manifest", () => {
  assert.doesNotThrow(() => validateSdkManifest(compatibleManifest(), "12.34.56"));
});

test("rejects incompatible or incomplete release manifests", () => {
  const wrongMajor = compatibleManifest();
  wrongMajor.apiMajor = 2;
  assert.throws(() => validateSdkManifest(wrongMajor, "12.34.56"), /unsupported API major/);

  const missingHeadless = compatibleManifest();
  delete missingHeadless.files["tip/testing.js"];
  assert.throws(
    () => validateSdkManifest(missingHeadless, "12.34.56"),
    /tip\/testing\.js/,
  );

  const separateEngagement = compatibleManifest();
  delete separateEngagement.integrations.engagement;
  assert.throws(
    () => validateSdkManifest(separateEngagement, "12.34.56"),
    /integrated engagement/,
  );

  assert.throws(
    () => validateSdkManifest(compatibleManifest("12.34.57"), "12.34.56"),
    /does not match/,
  );
});

test("preflights a combined release in one bounded operation", async () => {
  const requests = [];
  const npmCalls = [];
  const result = await preflightSdkRelease({
    route: "combined",
    region: "cn",
    runNpm: async (args) => {
      npmCalls.push(args);
      return JSON.stringify(compatiblePackage());
    },
    fetchImpl: async (url, options) => {
      requests.push({ url, options });
      if (url.endsWith("/manifest.json")) {
        return new Response(JSON.stringify(compatibleManifest()), { status: 200 });
      }
      return new Response("export {};", { status: 200 });
    },
  });

  assert.equal(result.sdk_version, "12.34.56");
  assert.equal(result.sdk_origin, "https://s3.viceme.cn");
  assert.equal(
    result.checked.manifest,
    "https://s3.viceme.cn/viceme-sdk/12.34.56/manifest.json",
  );
  assert.equal(npmCalls.length, 1);
  assert.deepEqual(npmCalls[0], [
    "view",
    "@viceme-ai/sdk@latest",
    "version",
    "exports",
    "--json",
    "--fetch-timeout=15000",
    "--fetch-retries=0",
    "--registry=https://registry.npmjs.org",
    "--@viceme-ai:registry=https://registry.npmjs.org",
  ]);
  assert.deepEqual(
    requests.map(({ url }) => url).sort(),
    [
      "https://s3.viceme.cn/viceme-sdk/12.34.56/danmaku.js",
      "https://s3.viceme.cn/viceme-sdk/12.34.56/index.js",
      "https://s3.viceme.cn/viceme-sdk/12.34.56/manifest.json",
      "https://s3.viceme.cn/viceme-sdk/12.34.56/tip.js",
    ].sort(),
  );
  assert.equal(requests.every(({ options }) => options.redirect === "error"), true);
  assert.equal(requests.every(({ options }) => options.signal instanceof AbortSignal), true);
});

test("preflights global Danmaku only in the selected region", async () => {
  const requests = [];
  const result = await preflightSdkRelease({
    route: "danmaku",
    region: "global",
    runNpm: async () => JSON.stringify(compatiblePackage()),
    fetchImpl: async (url) => {
      requests.push(url);
      return url.endsWith("/manifest.json")
        ? new Response(JSON.stringify(compatibleManifest()), { status: 200 })
        : new Response("export {};", { status: 200 });
    },
  });

  assert.equal(result.sdk_origin, "https://s3.viceme.ai");
  assert.deepEqual(requests.sort(), [
    "https://s3.viceme.ai/viceme-sdk/12.34.56/danmaku.js",
    "https://s3.viceme.ai/viceme-sdk/12.34.56/index.js",
    "https://s3.viceme.ai/viceme-sdk/12.34.56/manifest.json",
  ]);
});

test("preflights standalone Tip without unused Danmaku assets", async () => {
  const requests = [];
  await preflightSdkRelease({
    route: "tip",
    region: "cn",
    runNpm: async () => JSON.stringify(compatiblePackage()),
    fetchImpl: async (url) => {
      requests.push(url);
      return url.endsWith("/manifest.json")
        ? new Response(JSON.stringify(compatibleManifest()), { status: 200 })
        : new Response("export {};", { status: 200 });
    },
  });

  assert.deepEqual(requests.sort(), [
    "https://s3.viceme.cn/viceme-sdk/12.34.56/index.js",
    "https://s3.viceme.cn/viceme-sdk/12.34.56/manifest.json",
    "https://s3.viceme.cn/viceme-sdk/12.34.56/tip.js",
  ]);
});

test("preflight rejects unsupported Tip regions before network access", async () => {
  await assertRejectedBeforeNetwork(
    "tip",
    "global",
    /Tip routes require the cn region/,
  );
});

test("preflight rejects inherited route and region names before network access", async () => {
  for (const route of ["toString", "constructor", "__proto__"]) {
    await assertRejectedBeforeNetwork(route, "cn", /route must be danmaku, tip, or combined/);
  }
  for (const region of ["toString", "constructor", "__proto__"]) {
    await assertRejectedBeforeNetwork("danmaku", region, /region must be cn or global/);
  }
});

test("preflight fails closed on a non-200 immutable asset", async () => {
  await assert.rejects(
    preflightSdkRelease({
      route: "danmaku",
      region: "cn",
      runNpm: async () => JSON.stringify(compatiblePackage()),
      fetchImpl: async (url) =>
        url.endsWith("/danmaku.js")
          ? new Response(null, { status: 503 })
          : url.endsWith("/manifest.json")
            ? new Response(JSON.stringify(compatibleManifest()), { status: 200 })
            : new Response("export {};", { status: 200 }),
    }),
    /returned HTTP 503/,
  );
});
