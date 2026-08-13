import { fileURLToPath } from "node:url";
import { defineConfig } from "vitest/config";

const fixtureRoot = fileURLToPath(new URL(".", import.meta.url));

export default defineConfig({
  root: fixtureRoot,
  esbuild: {
    jsx: "automatic",
  },
  test: {
    clearMocks: true,
    environment: "jsdom",
    include: [".generated/danmaku-blueprint.test.tsx"],
    setupFiles: ["setup-tests.ts"],
  },
});
