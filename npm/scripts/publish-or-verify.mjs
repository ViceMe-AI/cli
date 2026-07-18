import { spawnSync } from "node:child_process";
import { readFile } from "node:fs/promises";

const registry = "https://registry.npmjs.org";
const registryArguments = [
  `--registry=${registry}`,
  `--@viceme-ai:registry=${registry}`,
];
const packageDocument = JSON.parse(await readFile(new URL("../../package.json", import.meta.url), "utf8"));
if (!/^\d+\.\d+\.\d+$/.test(packageDocument.version)) {
  throw new Error("the stable release workflow only publishes an exact x.y.z version");
}
const packageID = `${packageDocument.name}@${packageDocument.version}`;

const packed = run("npm", ["pack", "--json", "--dry-run"], false);
const packReport = JSON.parse(packed.stdout)[0];
if (packReport.id !== packageID || !packReport.integrity) {
  throw new Error(`local npm pack did not produce ${packageID}`);
}

const remote = spawnSync(
  "npm",
  ["view", packageID, "dist.integrity", "--json", ...registryArguments],
  { encoding: "utf8" },
);
if (remote.status === 0) {
  const remoteIntegrity = JSON.parse(remote.stdout);
  if (remoteIntegrity !== packReport.integrity) {
    throw new Error(
      `${packageID} is already published with different integrity; refusing to overwrite or treat it as recovered`,
    );
  }
  ensureLatestIsNotOlder();
  process.stdout.write(`${packageID} is already published with matching integrity\n`);
  process.exit(0);
}

const lookupFailure = `${remote.stdout}\n${remote.stderr}`;
if (!/E404|404 Not Found|is not in this registry/i.test(lookupFailure)) {
  throw new Error(`could not safely determine npm publication state:\n${lookupFailure}`);
}

run(
  "npm",
  ["publish", ...registryArguments, "--access", "public", "--provenance"],
  true,
);
ensureLatestIsNotOlder();

function ensureLatestIsNotOlder() {
  let latest = readLatest();
  if (compareStableVersions(latest, packageDocument.version) >= 0) {
    return;
  }
  run(
    "npm",
    ["dist-tag", "add", packageID, "latest", ...registryArguments],
    true,
  );
  latest = readLatest();
  if (latest !== packageDocument.version) {
    throw new Error(`npm latest remained ${latest} after promoting ${packageID}`);
  }
}

function readLatest() {
  const result = run(
    "npm",
    ["view", packageDocument.name, "dist-tags.latest", "--json", ...registryArguments],
    false,
  );
  const latest = JSON.parse(result.stdout);
  if (typeof latest !== "string" || !/^\d+\.\d+\.\d+$/.test(latest)) {
    throw new Error(`npm returned an invalid stable latest dist-tag for ${packageDocument.name}`);
  }
  return latest;
}

function compareStableVersions(left, right) {
  const a = left.split(".").map(Number);
  const b = right.split(".").map(Number);
  for (let index = 0; index < 3; index += 1) {
    if (a[index] !== b[index]) {
      return a[index] < b[index] ? -1 : 1;
    }
  }
  return 0;
}

function run(name, args, inherit) {
  const result = spawnSync(name, args, {
    encoding: "utf8",
    stdio: inherit ? "inherit" : ["ignore", "pipe", "pipe"],
  });
  if (result.error) {
    throw result.error;
  }
  if (result.status !== 0) {
    throw new Error(`${name} ${args.join(" ")} exited ${result.status}:\n${result.stderr ?? ""}`);
  }
  return result;
}
