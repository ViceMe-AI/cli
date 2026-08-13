import { copyFileSync, mkdirSync } from "node:fs";
import { fileURLToPath } from "node:url";

const sourceDirectory = fileURLToPath(
  new URL(
    "../../../skills/viceme-danmaku/assets/react-tailwind/",
    import.meta.url,
  ),
);
const targetDirectory = fileURLToPath(
  new URL("./.generated/", import.meta.url),
);
const goldenFiles = [
  "danmaku-blueprint.test.tsx",
  "danmaku-blueprint.tsx",
  "danmaku-motion.ts",
  "use-presence-motion.ts",
];

mkdirSync(targetDirectory, { recursive: true });
for (const filename of goldenFiles) {
  copyFileSync(
    `${sourceDirectory}/${filename}`,
    `${targetDirectory}/${filename}`,
  );
}

console.log(`Prepared ${goldenFiles.length} golden danmaku fixture files.`);
