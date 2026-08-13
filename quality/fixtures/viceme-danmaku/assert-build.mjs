import { readdirSync, readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

const assetsDirectory = fileURLToPath(
  new URL("../../../.cache/viceme-danmaku-fixture/assets", import.meta.url),
);
const css = readdirSync(assetsDirectory)
  .filter((filename) => filename.endsWith(".css"))
  .map((filename) => readFileSync(`${assetsDirectory}/${filename}`, "utf8"))
  .join("\n");

const requiredTailwindV4Output = [
  ["z-60", ".z-60{z-index:60}"],
  ["z-1", ".z-1{z-index:1}"],
  ["h-68", ".h-68{height:calc(var(--spacing) * 68)}"],
  ["h-15", ".h-15{height:calc(var(--spacing) * 15)}"],
  ["max-w-91.5", ".max-w-91\\.5{max-width:calc(var(--spacing) * 91.5)}"],
  [
    "duration-350",
    ".duration-350{--tw-duration:.35s;transition-duration:.35s}",
  ],
  [
    "duration-320",
    ".duration-320{--tw-duration:.32s;transition-duration:.32s}",
  ],
];

for (const [utility, declaration] of requiredTailwindV4Output) {
  if (!css.includes(declaration)) {
    throw new Error(
      `Tailwind v4 fixture did not emit ${utility} (${declaration})`,
    );
  }
}

console.log("Tailwind v4 fixture emitted all golden dynamic utilities.");
