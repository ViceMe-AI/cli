export function launcherEnvironment(environment, launcherPath, launcherRuntime) {
  return {
    ...environment,
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
