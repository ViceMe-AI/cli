import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { chmod, mkdir, mkdtemp, readFile, stat, symlink, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import test from "node:test";

const localBinary = process.env.VICEME_TEST_BINARY;
const packageTarball = process.env.VICEME_TEST_PACKAGE_TARBALL;
const packageDocument = JSON.parse(
  await readFile(new URL("../../package.json", import.meta.url), "utf8"),
);
const packageVersion = packageDocument.version;
const packageArgumentPrefix = `${packageDocument.name}@`;

test(
  "packed launcher executes root install with a local Go build",
  { skip: !localBinary },
  async () => {
    const home = await mkdtemp(path.join(os.tmpdir(), "viceme-launcher-smoke-"));
    const codexHome = path.join(home, "codex");
    const configHome = path.join(home, "config");
    const launcher = fileURLToPath(new URL("../bin/viceme.mjs", import.meta.url));
    const linkedLauncher = path.join(home, "viceme.mjs");
    await symlink(launcher, linkedLauncher);
    const child = spawnSync(
      process.execPath,
      [linkedLauncher, "install", "--target", "codex"],
      {
        encoding: "utf8",
        env: {
          ...process.env,
          HOME: home,
          CODEX_HOME: codexHome,
          VICEME_CLI_CONFIG_DIR: configHome,
          VICEME_BINARY_PATH: path.resolve(localBinary),
        },
      },
    );
    assert.equal(child.status, 0, child.stderr);
    const result = JSON.parse(child.stdout);
    assert.equal(result.skill.all_succeeded, true);
    assert.equal("ok" in result, false);
    assert.equal("data" in result, false);
    await stat(path.join(codexHome, "skills", "viceme", "SKILL.md"));
    await stat(path.join(configHome, "config.json"));
  },
);

test(
  "packed cold-start persists a global launcher that works from a fresh PATH",
  { skip: !localBinary || !packageTarball || process.platform === "win32" },
  async () => {
    const home = await mkdtemp(path.join(os.tmpdir(), "viceme-packed-cold-start-"));
    const prefix = path.join(home, "npm-prefix");
    const fakeBin = path.join(home, "fake-bin");
    const fakeNPM = path.join(fakeBin, "npm");
    const marker = path.join(home, "global-install-invoked");
    const npmDebug = path.join(home, "global-install-debug.json");
    const npmCLI = process.env.npm_execpath;
    assert.ok(npmCLI, "npm test must expose npm_execpath");
    await mkdir(fakeBin, { recursive: true });
    await writeFile(
      fakeNPM,
      `#!/usr/bin/env node
const { appendFileSync, writeFileSync } = require("node:fs");
const { spawnSync } = require("node:child_process");
const args = process.argv.slice(2);
const packageIndex = args.findIndex((arg) =>
  arg.startsWith(process.env.VICEME_TEST_PACKAGE_PREFIX),
);
if (
  args[0] !== process.env.VICEME_TEST_NPM_CACHE_ARG ||
  args[1] !== "install" ||
  packageIndex < 0
) {
  process.stderr.write("unexpected npm invocation: " + args.join(" ") + "\\n");
  process.exit(91);
}
args[packageIndex] = process.env.VICEME_TEST_PACKAGE_TARBALL;
appendFileSync(process.env.VICEME_FAKE_NPM_MARKER, args.join(" ") + "\\n");
const child = spawnSync(process.execPath, [process.env.VICEME_REAL_NPM_CLI, ...args], {
  encoding: "utf8",
  env: process.env,
});
writeFileSync(process.env.VICEME_FAKE_NPM_DEBUG, JSON.stringify({
  status: child.status,
  signal: child.signal,
  error: child.error?.message,
  stdout: child.stdout,
  stderr: child.stderr,
}, null, 2));
process.exit(child.status ?? 1);
`,
      { mode: 0o700 },
    );
    await chmod(fakeNPM, 0o700);
    const isolatedEnvironment = {
      ...process.env,
      HOME: home,
      CODEX_HOME: path.join(home, "codex"),
      VICEME_CLI_CONFIG_DIR: path.join(home, ".viceme-cli"),
      NPM_CONFIG_CACHE: path.join(home, "npm-cache"),
      NPM_CONFIG_PREFIX: prefix,
      // npm itself launches this test and exports lower-case npm_config_*
      // values. Set both forms so the nested real npm process cannot fall
      // back to the developer or CI runner cache/prefix.
      npm_config_cache: path.join(home, "npm-cache"),
      npm_config_prefix: prefix,
      PATH: `${fakeBin}${path.delimiter}${process.env.PATH}`,
      VICEME_BINARY_PATH: path.resolve(localBinary),
      VICEME_INSTALL_METHOD: "npm",
      VICEME_REAL_NPM_CLI: npmCLI,
      VICEME_TEST_PACKAGE_TARBALL: path.resolve(packageTarball),
      VICEME_TEST_PACKAGE_PREFIX: packageArgumentPrefix,
      VICEME_TEST_NPM_CACHE_ARG: `--cache=${path.join(home, ".viceme-cli", "npm-cache")}`,
      VICEME_FAKE_NPM_MARKER: marker,
      VICEME_FAKE_NPM_DEBUG: npmDebug,
    };
    const first = spawnSync(
      process.execPath,
      [
        npmCLI,
        "exec",
        "--yes",
        `--package=${path.resolve(packageTarball)}`,
        "--",
        "viceme",
        "install",
        "--target",
        "codex",
      ],
      { encoding: "utf8", env: isolatedEnvironment },
    );
    let debug = "npm subprocess did not write diagnostics";
    try {
      debug = await readFile(npmDebug, "utf8");
    } catch {
      // Keep the primary assertion useful even if npm did not reach the shim.
    }
    assert.equal(first.status, 0, `${first.stdout}\n${first.stderr}\n${debug}`);
    const install = JSON.parse(first.stdout);
    assert.equal(install.skill.all_succeeded, true);
    assert.equal("ok" in install, false);
    assert.equal("data" in install, false);
    assert.match(
      await readFile(marker, "utf8"),
      /install --registry=https:\/\/registry\.npmjs\.org --@viceme-ai:registry=https:\/\/registry\.npmjs\.org --global/,
    );

    // Seed the exact verified release cache, then remove the development-only
    // binary override. The second invocation must reach the globally
    // installed npm launcher and resolve its versioned cache without any
    // source checkout or npx process remaining on PATH.
    const cacheRoot = path.join(home, "viceme-cache");
    const targetOS = process.platform === "darwin" ? "darwin" : "linux";
    const targetArch = process.arch === "arm64" ? "arm64" : "amd64";
    const asset = `viceme_${packageVersion}_${targetOS}_${targetArch}`;
    const cachedDirectory = path.join(
      cacheRoot,
      "cli",
      packageVersion,
      "generations",
      "generation-package-smoke",
    );
    const cachedBinary = path.join(cachedDirectory, asset);
    const binaryContents = await readFile(path.resolve(localBinary));
    const checksum = createHash("sha256").update(binaryContents).digest("hex");
    await mkdir(cachedDirectory, { recursive: true });
    await writeFile(cachedBinary, binaryContents, { mode: 0o700 });
    await chmod(cachedBinary, 0o700);
    await writeFile(`${cachedBinary}.sha256`, `${checksum}  ${asset}\n`, { mode: 0o600 });

    const freshPath = [
      path.join(prefix, "bin"),
      path.dirname(process.execPath),
      "/usr/bin",
      "/bin",
    ].join(path.delimiter);
    const freshEnvironment = { ...isolatedEnvironment };
    delete freshEnvironment.VICEME_BINARY_PATH;
    const second = spawnSync("viceme", ["--version"], {
      encoding: "utf8",
      env: {
        ...freshEnvironment,
        PATH: freshPath,
        VICEME_CACHE_DIR: cacheRoot,
      },
    });
    assert.equal(second.status, 0, `${second.stdout}\n${second.stderr}`);
    const version = JSON.parse(second.stdout);
    assert.equal(version.version, packageVersion);
    assert.equal("ok" in version, false);
    assert.equal("data" in version, false);
    assert.equal("meta" in version, false);
    await stat(path.join(prefix, "bin", "viceme"));
  },
);
