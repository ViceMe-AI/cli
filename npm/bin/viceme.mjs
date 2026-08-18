#!/usr/bin/env node

import { spawnSync } from "node:child_process";
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import process from "node:process";

import { ensureBinary } from "../lib/installer.mjs";
import { launcherEnvironment } from "../lib/launcher-environment.mjs";

// POSIX shells report a signal death as 128 + signal number; wrappers and CI
// steps branch on those statuses, so every signal must keep its own code.
const SIGNAL_NUMBERS = new Map([
  ["SIGHUP", 1],
  ["SIGINT", 2],
  ["SIGQUIT", 3],
  ["SIGABRT", 6],
  ["SIGKILL", 9],
  ["SIGALRM", 14],
  ["SIGTERM", 15],
]);

function signalExitCode(signal) {
  return 128 + (SIGNAL_NUMBERS.get(signal) ?? 0);
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
