import { copyFile, mkdir, readdir } from "node:fs/promises";
import { basename, join, resolve } from "node:path";
import { pathToFileURL } from "node:url";

export async function restoreReleaseAssets(sourceDir, destinationDir) {
  await mkdir(destinationDir, { recursive: true });
  const entries = await readdir(sourceDir, { withFileTypes: true });
  for (const entry of entries) {
    if (!entry.isFile() || basename(entry.name) !== entry.name) {
      throw new Error(`Unsupported release asset: ${entry.name}`);
    }
    await copyFile(join(sourceDir, entry.name), join(destinationDir, entry.name));
  }
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(resolve(process.argv[1])).href
) {
  const [sourceDir, destinationDir] = process.argv.slice(2);
  if (!sourceDir || !destinationDir) {
    throw new Error(
      "Usage: restore-release-assets.mjs <source-dir> <destination-dir>",
    );
  }
  await restoreReleaseAssets(sourceDir, destinationDir);
}
