import { copyFileSync, mkdirSync, rmSync } from "node:fs";
import { fileURLToPath } from "node:url";

const sourceDirectory = fileURLToPath(
  new URL("../../../skills/viceme-danmaku/assets/", import.meta.url),
);
const targetDirectory = fileURLToPath(
  new URL("./.generated/", import.meta.url),
);
const goldenFiles = [
  "react-tailwind/danmaku-blueprint.test.tsx",
  "react-tailwind/danmaku-blueprint.tsx",
  "react-tailwind/danmaku-motion.ts",
  "react-tailwind/use-presence-motion.ts",
];

rmSync(targetDirectory, { recursive: true, force: true });
mkdirSync(targetDirectory, { recursive: true });
for (const filename of goldenFiles) {
  const targetFilename = filename.replace("react-tailwind/", "");
  const parent = targetFilename.split("/").slice(0, -1).join("/");
  if (parent) mkdirSync(`${targetDirectory}/${parent}`, { recursive: true });
  copyFileSync(
    `${sourceDirectory}/${filename}`,
    `${targetDirectory}/${targetFilename}`,
  );
}

console.log(`Prepared ${goldenFiles.length} golden danmaku fixture files.`);
