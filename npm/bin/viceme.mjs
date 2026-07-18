#!/usr/bin/env node

import { spawnSync } from "node:child_process";
import { readFile } from "node:fs/promises";
import process from "node:process";

import { ensureBinary } from "../lib/installer.mjs";

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
      ...environment,
      VICEME_INSTALL_METHOD:
        environment.VICEME_INSTALL_METHOD === "npm"
          ? "npm"
          : environment.VICEME_BINARY_PATH
            ? "development"
            : "npm",
      VICEME_NPM_PACKAGE_VERSION: packageDocument.version,
    },
    windowsHide: false,
  });
  if (child.error) {
    throw child.error;
  }
  if (child.signal) {
    return 128;
  }
  return child.status ?? 1;
}

try {
  process.exitCode = await main();
} catch (error) {
  process.stderr.write(`viceme launcher: ${error.message}\n`);
  process.exitCode = 1;
}
