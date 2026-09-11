import path from "node:path";

export function installedNPMPrefix(launcherPath, platform = process.platform) {
  if (typeof launcherPath !== "string" || launcherPath === "") {
    return undefined;
  }
  // A POSIX global npm package has one unambiguous layout. Derive the prefix
  // from the launcher that is actually executing so a foreign Node/npm on PATH
  // cannot redirect an update into another global installation. npx caches do
  // not include lib/node_modules and intentionally keep npm's configured
  // global prefix for activation and re-execution.
  if (platform === "win32") {
    return undefined;
  }
  const normalized = path.posix.normalize(launcherPath);
  const suffix = path.posix.join(
    "lib",
    "node_modules",
    "@viceme-ai",
    "cli",
    "npm",
    "bin",
    "viceme.mjs",
  );
  const marker = `${path.posix.sep}${suffix}`;
  if (!path.posix.isAbsolute(normalized) || !normalized.endsWith(marker)) {
    return undefined;
  }
  const prefix = normalized.slice(0, -marker.length);
  return prefix || path.posix.sep;
}

export function launcherEnvironment(environment, launcherPath, launcherRuntime) {
  const npmPrefix = installedNPMPrefix(launcherPath);
  return {
    ...environment,
    ...(npmPrefix
      ? {
          NPM_CONFIG_PREFIX: npmPrefix,
          npm_config_prefix: npmPrefix,
        }
      : {}),
    VICEME_INSTALL_METHOD:
      environment.VICEME_INSTALL_METHOD === "npm"
        ? "npm"
        : environment.VICEME_BINARY_PATH
          ? "development"
          : "npm",
    VICEME_NPM_LAUNCHER_PATH: launcherPath,
    VICEME_NPM_LAUNCHER_RUNTIME: launcherRuntime,
  };
}
