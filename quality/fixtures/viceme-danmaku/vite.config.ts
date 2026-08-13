import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { fileURLToPath } from "node:url";
import { defineConfig } from "vite";

const fixtureRoot = fileURLToPath(new URL(".", import.meta.url));
const outputDirectory = fileURLToPath(
  new URL("../../../.cache/viceme-danmaku-fixture", import.meta.url),
);

export default defineConfig({
  root: fixtureRoot,
  plugins: [react(), tailwindcss()],
  build: {
    emptyOutDir: true,
    outDir: outputDirectory,
  },
});
