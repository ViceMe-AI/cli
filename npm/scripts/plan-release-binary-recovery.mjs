import { pathToFileURL } from "node:url";

export function planReleaseBinaryRecovery(asset, existingAssets) {
  if (typeof asset !== "string" || asset.length === 0) {
    throw new Error("asset is required");
  }
  if (!Array.isArray(existingAssets)) {
    throw new Error("existing assets must be an array");
  }
  const names = new Set(existingAssets);
  const binaryExists = names.has(asset);
  const checksumExists = names.has(`${asset}.sha256`);
  return {
    binaryExists,
    checksumExists,
    needsBuild: !binaryExists,
  };
}

function main() {
  const [asset, encodedAssets = "[]"] = process.argv.slice(2);
  const plan = planReleaseBinaryRecovery(asset, JSON.parse(encodedAssets));
  process.stdout.write(
    [
      `binary_exists=${plan.binaryExists}`,
      `checksum_exists=${plan.checksumExists}`,
      `needs_build=${plan.needsBuild}`,
      "",
    ].join("\n"),
  );
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  main();
}
