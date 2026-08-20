#!/usr/bin/env node

import { spawnSync } from "node:child_process";
import { readFile } from "node:fs/promises";
import { constants as osConstants } from "node:os";
import { fileURLToPath } from "node:url";
import process from "node:process";

import { ensureBinary } from "../lib/installer.mjs";
import { launcherEnvironment } from "../lib/launcher-environment.mjs";

function signalExitCode(signal) {
  const signalNumber = osConstants.signals[signal];
  if (!Number.isInteger(signalNumber)) {
    throw new Error(`unsupported child termination signal ${signal}`);
  }
  return 128 + signalNumber;
}

export async function main(args = process.argv.slice(2), environment = process.env) {
  const packageDocument = JSON.parse(
    await readFile(new URL("../../package.json", import.meta.url), "utf8"),
  );
  const binary = await ensureBinary({
    packageVersion: packageDocument.version,
    environment,
  });
  const child = spawnSync(binary, args, {
    stdio: "inherit",
    env: {
      ...launcherEnvironment(
        environment,
        fileURLToPath(import.meta.url),
        process.execPath,
      ),
      VICEME_NPM_PACKAGE_VERSION: packageDocument.version,
    },
    windowsHide: false,
  });
  if (child.error) {
    throw child.error;
  }
  if (child.signal) {
    return signalExitCode(child.signal);
  }
  return child.status ?? 1;
}

try {
  process.exitCode = await main();
} catch (error) {
  process.stderr.write(`viceme launcher: ${error.message}\n`);
  process.stdout.write(
    `${JSON.stringify({
      ok: false,
      error: {
        type: "internal",
        code: "LAUNCHER_FAILED",
        message: "ViceMe launcher could not start",
        retryable: false,
      },
    })}\n`,
  );
  process.exitCode = 1;
}
