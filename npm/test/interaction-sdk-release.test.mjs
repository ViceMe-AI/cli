import assert from "node:assert/strict";
import test from "node:test";

import {
  resolveSdkVersion,
  validateSdkManifest,
  validateSdkManifestResponse,
} from "../../skills/let-people-interact/scripts/validate-sdk-release.mjs";

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

test("validates the manifest body and direct status from one response", () => {
  const body = JSON.stringify(compatibleManifest());
  assert.doesNotThrow(() => validateSdkManifestResponse(`${body}\n200`, "12.34.56"));
  assert.throws(
    () => validateSdkManifestResponse(`${body}\n302`, "12.34.56"),
    /HTTP 302/,
  );
  assert.throws(
    () => validateSdkManifestResponse(body, "12.34.56"),
    /missing its HTTP status/,
  );
});
