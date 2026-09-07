import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { chmod, mkdir, mkdtemp, readFile, stat, symlink, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { spawn, spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import test from "node:test";

import { launcherEnvironment } from "../lib/launcher-environment.mjs";

const localBinary = process.env.VICEME_TEST_BINARY;
const packageTarball = process.env.VICEME_TEST_PACKAGE_TARBALL;
const packageDocument = JSON.parse(
  await readFile(new URL("../../package.json", import.meta.url), "utf8"),
);
const packageVersion = packageDocument.version;
const packageArgumentPrefix = `${packageDocument.name}@`;

test("launcher gives the CLI exact re-execution authority", () => {
  const launcherPath = fileURLToPath(new URL("../bin/viceme.mjs", import.meta.url));
  const environment = launcherEnvironment(
    { VICEME_INSTALL_METHOD: "npm" },
    launcherPath,
    process.execPath,
  );
  assert.equal(environment.VICEME_INSTALL_METHOD, "npm");
  assert.equal(environment.VICEME_NPM_LAUNCHER_RUNTIME, process.execPath);
  assert.equal(environment.VICEME_NPM_LAUNCHER_PATH, launcherPath);
});

test("launcher classifies npm-managed launches instead of trusting foreign inherited state", () => {
  const launcherPath = fileURLToPath(new URL("../bin/viceme.mjs", import.meta.url));
  // A stale standalone marker from an earlier installation must not
  // reclassify an npm-launched process; only an explicit npm value combined
  // with a binary override intentionally keeps the npm classification for
  // release smoke tests.
  const staleInherited = launcherEnvironment(
    { VICEME_INSTALL_METHOD: "binary" },
    launcherPath,
    process.execPath,
  );
  assert.equal(staleInherited.VICEME_INSTALL_METHOD, "npm");
  const developmentOverride = launcherEnvironment(
    { VICEME_BINARY_PATH: "/tmp/viceme-dev-binary" },
    launcherPath,
    process.execPath,
  );
  assert.equal(developmentOverride.VICEME_INSTALL_METHOD, "development");
});

test(
  "launcher preserves every platform signal exit code",
  { skip: process.platform === "win32" },
  () => {
    const launcher = fileURLToPath(new URL("../bin/viceme.mjs", import.meta.url));
    const child = spawnSync(
      process.execPath,
      [launcher, "-c", 'kill -s SEGV "$$"'],
      {
        env: {
          ...process.env,
          VICEME_BINARY_PATH: "/bin/sh",
        },
      },
    );
    assert.equal(child.status, 128 + os.constants.signals.SIGSEGV);
  },
);

async function startHealthServer() {
  const script = fileURLToPath(new URL("./health-server.mjs", import.meta.url));
  const child = spawn(process.execPath, [script], {
    stdio: ["ignore", "pipe", "inherit"],
  });
  const url = await new Promise((resolve, reject) => {
    let output = "";
    child.once("error", reject);
    child.once("exit", (code) => reject(new Error(`health server exited with ${code}`)));
    child.stdout.on("data", (chunk) => {
      output += chunk.toString("utf8");
      const newline = output.indexOf("\n");
      if (newline >= 0) {
        resolve(output.slice(0, newline));
      }
    });
  });
  return {
    url,
    stop() {
      child.kill("SIGTERM");
    },
  };
}

test("launcher failure preserves the JSON stdout contract", async () => {
  const home = await mkdtemp(path.join(os.tmpdir(), "viceme-launcher-failure-"));
  const launcher = fileURLToPath(new URL("../bin/viceme.mjs", import.meta.url));
  const child = spawnSync(process.execPath, [launcher, "version"], {
    encoding: "utf8",
    env: {
      ...process.env,
      VICEME_BINARY_PATH: path.join(home, "missing-viceme"),
    },
  });
  assert.notEqual(child.status, 0);
  const result = JSON.parse(child.stdout);
  assert.equal(result.ok, false);
  assert.equal(result.error.code, "LAUNCHER_FAILED");
  assert.match(child.stderr, /viceme launcher:/);
});

test(
  "cached launcher resumes the committed generation through the configured npm prefix",
  { skip: !localBinary },
  async () => {
    const home = await mkdtemp(path.join(os.tmpdir(), "viceme-npx-resume-"));
    const prefix = path.join(home, "npm prefix with spaces");
    const configHome = path.join(home, "config");
    const packageRoot = path.join(
      prefix,
      ...(process.platform === "win32" ? [] : ["lib"]),
      "node_modules", "@viceme-ai", "cli",
    );
    const targetVersion = `${Number(packageVersion.split(".")[0]) + 1}.0.0`;
    await mkdir(path.join(packageRoot, "npm", "bin"), { recursive: true });
    await mkdir(configHome, { recursive: true });
    await writeFile(path.join(packageRoot, "package.json"), JSON.stringify({
      name: packageDocument.name, version: targetVersion,
    }));
    await writeFile(path.join(packageRoot, "npm", "bin", "viceme.mjs"), `
import fs from "node:fs";
console.log(JSON.stringify({
  args: process.argv.slice(2), input: fs.readFileSync(0, "utf8"),
  target: process.env.VICEME_AUTO_UPDATE_TO,
}));
process.exitCode = 7;
`);
    await writeFile(path.join(configHome, "active-generation.json"), JSON.stringify({
      schemaVersion: 1, version: targetVersion, installMethod: "npm",
      identity: createHash("sha256").update(`npm\0${packageArgumentPrefix}${targetVersion}`).digest("hex"),
    }));
    const launcher = fileURLToPath(new URL("../bin/viceme.mjs", import.meta.url));
    const args = ["version", "--profile", "profile with spaces"];
    const child = spawnSync(process.execPath, [launcher, ...args], {
      encoding: "utf8", input: "original stdin",
      env: {
        ...process.env,
        VICEME_CLI_CONFIG_DIR: configHome,
        VICEME_BINARY_PATH: path.resolve(localBinary),
        VICEME_INSTALL_METHOD: "npm",
        VICEME_AUTO_UPDATE_REEXEC: "",
        NPM_CONFIG_PREFIX: prefix, npm_config_prefix: prefix,
        NPM_CONFIG_CACHE: path.join(home, "npm-cache"),
        npm_config_cache: path.join(home, "npm-cache"),
        CI: "1",
      },
    });
    assert.equal(child.status, 7, `${child.stdout}\n${child.stderr}`);
    assert.deepEqual(JSON.parse(child.stdout), {
      args, input: "original stdin", target: targetVersion,
    });
    assert.equal(child.stderr, "");
  },
);

test(
  "packed launcher executes root install with a local Go build",
  { skip: !localBinary },
  async (context) => {
    const health = await startHealthServer();
    context.after(() => health.stop());
    const home = await mkdtemp(path.join(os.tmpdir(), "viceme-launcher-smoke-"));
    const codexHome = path.join(home, "codex");
    const configHome = path.join(home, "config");
    const launcher = fileURLToPath(new URL("../bin/viceme.mjs", import.meta.url));
    const linkedLauncher = path.join(home, "viceme.mjs");
    await symlink(launcher, linkedLauncher);
    const child = spawnSync(
      process.execPath,
      [linkedLauncher, "install", "--agent", "codex"],
      {
        encoding: "utf8",
        env: {
          ...process.env,
          HOME: home,
          CODEX_HOME: codexHome,
          VICEME_AGENTS_SKILLS_DIR: path.join(home, ".agents", "skills"),
          VICEME_CLI_CONFIG_DIR: configHome,
          VICEME_BINARY_PATH: path.resolve(localBinary),
          CI: "1",
          VICEME_API_BASE_URL: health.url,
        },
      },
    );
    assert.equal(child.status, 0, `${child.stdout}\n${child.stderr}`);
    const result = JSON.parse(child.stdout);
    assert.equal(result.ok, true);
    await assert.rejects(stat(path.join(codexHome, "skills")), { code: "ENOENT" });
    assert.equal(
      result.data.skills.every((skill) => skill.results.length === 1 && skill.results[0].target === "agents"),
      true,
    );
    assert.equal(result.data.skills.length, 9);
    assert.equal(result.data.skills.every((skill) => skill.all_succeeded), true);
    await stat(path.join(home, ".agents", "skills", "creator-tools", "SKILL.md"));
    await stat(path.join(home, ".agents", "skills", "become-a-creator", "SKILL.md"));
    await stat(path.join(home, ".agents", "skills", "customize-your-page", "SKILL.md"));
    await stat(path.join(home, ".agents", "skills", "sell-a-skill", "SKILL.md"));
    await stat(path.join(home, ".agents", "skills", "use-a-skill", "SKILL.md"));
    await stat(path.join(home, ".agents", "skills", "charge-for-your-work", "SKILL.md"));
    await stat(path.join(home, ".agents", "skills", "let-people-interact", "SKILL.md"));
    await stat(path.join(home, ".agents", "skills", "let-others-make-a-copy", "SKILL.md"));
    await stat(path.join(home, ".agents", "skills", "let-me-make-a-copy", "SKILL.md"));
    await stat(
      path.join(home, ".agents", "skills", "let-people-interact", "templates", "single-html.html"),
    );
    await stat(
      path.join(
        home,
        ".agents",
        "skills",
        "let-people-interact",
        "templates",
        "mounted-combination.html",
      ),
    );
    await stat(
      path.join(
        home,
        ".agents",
        "skills",
        "let-people-interact",
        "scripts",
        "preflight-sdk-release.mjs",
      ),
    );
    await stat(path.join(configHome, "config.json"));
  },
);

test(
  "packed cold-start persists a global launcher that works from a fresh PATH",
  { skip: !localBinary || !packageTarball || process.platform === "win32" },
  async (context) => {
    const health = await startHealthServer();
    context.after(() => health.stop());
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
if (args[0] === process.env.VICEME_TEST_NPM_CACHE_ARG &&
    ["root", "prefix"].includes(args[1]) && args.includes("--global")) {
  const query = spawnSync(process.execPath, [process.env.VICEME_REAL_NPM_CLI, ...args], {
    encoding: "utf8", env: process.env,
  });
  process.stdout.write(query.stdout || "");
  process.stderr.write(query.stderr || "");
  process.exit(query.status ?? 1);
}
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
      VICEME_API_BASE_URL: health.url,
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
        "--agent",
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
    assert.equal(install.ok, true);
    assert.equal(install.data.skills.length, 9);
    assert.equal(install.data.skills.every((skill) => skill.all_succeeded), true);
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
    // The bundled checksum manifest is the launcher's trust root, including
    // the warm cache path. Align the globally installed package's manifest
    // with the locally built smoke binary exactly as a real release does,
    // because the committed placeholder manifest only describes 0.2.0.
    const installedPackageRoot = path.join(
      prefix,
      "lib",
      "node_modules",
      ...packageDocument.name.split("/"),
    );
    await writeFile(
      path.join(installedPackageRoot, "checksums.txt"),
      `${checksum}  ${asset}\n`,
    );

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
    assert.equal(version.ok, true);
    assert.equal(version.data.version, packageVersion);
    assert.equal(version.meta.executingCliVersion, packageVersion);
    assert.equal("cliVersion" in version.meta, false);
    await stat(path.join(prefix, "bin", "viceme"));
  },
);
