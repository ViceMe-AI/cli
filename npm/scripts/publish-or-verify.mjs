import { spawnSync } from "node:child_process";
import { readFile } from "node:fs/promises";
import { setTimeout as wait } from "node:timers/promises";

const registry = "https://registry.npmjs.org";
const registryArguments = [
  `--registry=${registry}`,
  `--@myc666:registry=${registry}`,
];
// npm can accept a first publish before the public metadata endpoint exposes
// it. Eleven reads wait at most 303 seconds with the default capped backoff.
const registryReadRetryAttempts = readNonNegativeInteger(
  "VICEME_NPM_VIEW_RETRY_ATTEMPTS",
  11,
  { minimum: 1 },
);
const registryReadRetryInitialDelayMS = readNonNegativeInteger(
  "VICEME_NPM_VIEW_RETRY_INITIAL_DELAY_MS",
  1_000,
);
const registryReadRetryMaximumDelayMS = 60_000;
const packageDocument = JSON.parse(await readFile(new URL("../../package.json", import.meta.url), "utf8"));
if (!/^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/.test(packageDocument.version)) {
  throw new Error("the release workflow requires an exact semantic version");
}
const packageID = `${packageDocument.name}@${packageDocument.version}`;
const releaseTag = packageDocument.publishConfig?.tag ?? "latest";

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
  await ensureReleaseTagIsNotOlder();
  process.stdout.write(`${packageID} is already published with matching integrity\n`);
  process.exit(0);
}

const lookupFailure = `${remote.stdout}\n${remote.stderr}`;
if (!isNotFound(lookupFailure)) {
  throw new Error(`could not safely determine npm publication state:\n${lookupFailure}`);
}

run(
  "npm",
  ["publish", ...registryArguments, "--access", "public", "--provenance", "--tag", releaseTag],
  true,
);
const publishedIntegrity = await readPublishedIntegrity();
if (publishedIntegrity !== packReport.integrity) {
  throw new Error(
    `${packageID} became visible after publish with different integrity; refusing to treat it as recovered`,
  );
}
await ensureReleaseTagIsNotOlder();

async function ensureReleaseTagIsNotOlder() {
  let taggedVersion = await readReleaseTag();
  if (compareVersions(taggedVersion, packageDocument.version) >= 0) {
    return;
  }
  run(
    "npm",
    ["dist-tag", "add", packageID, releaseTag, ...registryArguments],
    true,
  );
  taggedVersion = await readReleaseTag();
  if (taggedVersion !== packageDocument.version) {
    throw new Error(`npm ${releaseTag} remained ${taggedVersion} after promoting ${packageID}`);
  }
}

async function readPublishedIntegrity() {
  const result = await runRegistryViewWithRetry(
    ["view", packageID, "dist.integrity", "--json", ...registryArguments],
    `${packageID} integrity`,
  );
  const integrity = JSON.parse(result.stdout);
  if (typeof integrity !== "string" || integrity === "") {
    throw new Error(`npm returned an invalid integrity for ${packageID}`);
  }
  return integrity;
}

async function readReleaseTag() {
  const result = await runRegistryViewWithRetry(
    ["view", packageDocument.name, `dist-tags.${releaseTag}`, "--json", ...registryArguments],
    `${packageDocument.name} ${releaseTag} dist-tag`,
  );
  const taggedVersion = JSON.parse(result.stdout);
  if (typeof taggedVersion !== "string" || !/^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/.test(taggedVersion)) {
    throw new Error(`npm returned an invalid ${releaseTag} dist-tag for ${packageDocument.name}`);
  }
  return taggedVersion;
}

async function runRegistryViewWithRetry(args, description) {
  for (let attempt = 1; attempt <= registryReadRetryAttempts; attempt += 1) {
    const result = spawnSync("npm", args, {
      encoding: "utf8",
      stdio: ["ignore", "pipe", "pipe"],
    });
    if (result.error) {
      throw result.error;
    }
    if (result.status === 0) {
      return result;
    }

    const failure = `${result.stdout ?? ""}\n${result.stderr ?? ""}`;
    if (!isNotFound(failure) || attempt === registryReadRetryAttempts) {
      throw commandFailure("npm", args, result);
    }

    const delayMS = Math.min(
      registryReadRetryInitialDelayMS * 2 ** (attempt - 1),
      registryReadRetryMaximumDelayMS,
    );
    process.stderr.write(
      `npm registry has not exposed ${description} yet; retrying in ${delayMS}ms (${attempt}/${registryReadRetryAttempts})\n`,
    );
    await wait(delayMS);
  }
  throw new Error(`exhausted npm registry reads for ${description}`);
}

function isNotFound(output) {
  return /E404|404 Not Found|is not in this registry/i.test(output);
}

function readNonNegativeInteger(name, fallback, { minimum = 0 } = {}) {
  const raw = process.env[name];
  if (raw === undefined || raw === "") {
    return fallback;
  }
  if (!/^\d+$/.test(raw) || Number(raw) < minimum) {
    throw new Error(`${name} must be an integer greater than or equal to ${minimum}`);
  }
  return Number(raw);
}

function compareVersions(left, right) {
  const [leftCore, leftPrerelease] = left.split("-", 2);
  const [rightCore, rightPrerelease] = right.split("-", 2);
  const a = leftCore.split(".").map(Number);
  const b = rightCore.split(".").map(Number);
  for (let index = 0; index < 3; index += 1) {
    if (a[index] !== b[index]) {
      return a[index] < b[index] ? -1 : 1;
    }
  }
  if (leftPrerelease === rightPrerelease) return 0;
  if (leftPrerelease === undefined) return 1;
  if (rightPrerelease === undefined) return -1;
  return leftPrerelease.localeCompare(rightPrerelease, "en", { numeric: true });
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
    throw commandFailure(name, args, result);
  }
  return result;
}

function commandFailure(name, args, result) {
  return new Error(`${name} ${args.join(" ")} exited ${result.status}:\n${result.stderr ?? ""}`);
}
