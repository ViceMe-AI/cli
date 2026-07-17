import assert from "node:assert/strict";
import { chmod, mkdir, mkdtemp, readFile, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import test from "node:test";

test(
  "release recovery verifies prior integrity or publishes only a missing version",
  { skip: process.platform === "win32" },
  async () => {
    const directory = await mkdtemp(path.join(os.tmpdir(), "viceme-publish-recovery-"));
    const fakeBin = path.join(directory, "bin");
    const fakeNPM = path.join(fakeBin, "npm");
    const marker = path.join(directory, "published");
    const distTagMarker = path.join(directory, "dist-tagged");
    await mkdir(fakeBin, { recursive: true });
    await writeFile(
      fakeNPM,
      `#!/usr/bin/env node
const { appendFileSync, existsSync, writeSync } = require("node:fs");
const args = process.argv.slice(2);
if (args[0] === "pack") {
  writeSync(1, JSON.stringify([{ id: "@viceme-ai/cli@0.1.0", integrity: "sha512-local" }]));
  process.exit(0);
}
if (args[0] === "view") {
  if (args.includes("dist-tags.latest")) {
    const latest = existsSync(process.env.DIST_TAG_MARKER)
      ? "0.1.0"
      : process.env.REMOTE_LATEST;
    writeSync(1, JSON.stringify(latest));
    process.exit(0);
  }
  if (process.env.REMOTE_MODE === "missing") {
    writeSync(2, "npm error E404 Not Found\\n");
    process.exit(1);
  }
  writeSync(1, JSON.stringify(process.env.REMOTE_INTEGRITY));
  process.exit(0);
}
if (args[0] === "publish") {
  appendFileSync(process.env.PUBLISH_MARKER, args.join(" ") + "\\n");
  process.exit(0);
}
if (args[0] === "dist-tag") {
  appendFileSync(process.env.DIST_TAG_MARKER, args.join(" ") + "\\n");
  process.exit(0);
}
process.exit(90);
`,
      { mode: 0o700 },
    );
    await chmod(fakeNPM, 0o700);
    const script = fileURLToPath(new URL("../scripts/publish-or-verify.mjs", import.meta.url));
    const baseEnvironment = {
      ...process.env,
      PATH: `${fakeBin}${path.delimiter}${path.dirname(process.execPath)}${path.delimiter}/usr/bin${path.delimiter}/bin`,
      PUBLISH_MARKER: marker,
      DIST_TAG_MARKER: distTagMarker,
      REMOTE_LATEST: "0.1.0",
    };

    const matching = spawnSync(process.execPath, [script], {
      encoding: "utf8",
      env: { ...baseEnvironment, REMOTE_MODE: "existing", REMOTE_INTEGRITY: "sha512-local" },
    });
    assert.equal(matching.status, 0, matching.stderr);

    const mismatch = spawnSync(process.execPath, [script], {
      encoding: "utf8",
      env: { ...baseEnvironment, REMOTE_MODE: "existing", REMOTE_INTEGRITY: "sha512-other" },
    });
    assert.notEqual(mismatch.status, 0);

    const newerLatest = spawnSync(process.execPath, [script], {
      encoding: "utf8",
      env: {
        ...baseEnvironment,
        REMOTE_MODE: "existing",
        REMOTE_INTEGRITY: "sha512-local",
        REMOTE_LATEST: "0.2.0",
      },
    });
    assert.equal(newerLatest.status, 0, newerLatest.stderr);
    await assert.rejects(readFile(distTagMarker, "utf8"));

    const olderLatest = spawnSync(process.execPath, [script], {
      encoding: "utf8",
      env: {
        ...baseEnvironment,
        REMOTE_MODE: "existing",
        REMOTE_INTEGRITY: "sha512-local",
        REMOTE_LATEST: "0.0.9",
      },
    });
    assert.equal(olderLatest.status, 0, olderLatest.stderr);
    assert.match(
      await readFile(distTagMarker, "utf8"),
      /dist-tag add @viceme-ai\/cli@0\.1\.0 latest/,
    );

    const missing = spawnSync(process.execPath, [script], {
      encoding: "utf8",
      env: { ...baseEnvironment, REMOTE_MODE: "missing", REMOTE_INTEGRITY: "" },
    });
    assert.equal(missing.status, 0, missing.stderr);
    assert.match(
      await readFile(marker, "utf8"),
      /publish --registry=https:\/\/registry\.npmjs\.org --@viceme-ai:registry=https:\/\/registry\.npmjs\.org/,
    );
  },
);
