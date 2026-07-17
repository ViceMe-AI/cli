import { createHash, randomUUID } from "node:crypto";
import { access, chmod, mkdir, readFile, readdir, rename, rm, writeFile } from "node:fs/promises";
import { constants } from "node:fs";
import os from "node:os";
import path from "node:path";

const RELEASE_BASE_URL = "https://github.com/ViceMe-AI/cli/releases/download";
const VERSION_PATTERN = /^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/;
const GENERATIONS_DIRECTORY = "generations";
const GENERATION_PREFIX = "generation-";
const STAGING_PREFIX = ".staging-";

export function releaseTarget(platform = process.platform, architecture = process.arch) {
  const operatingSystems = {
    darwin: "darwin",
    linux: "linux",
    win32: "windows",
  };
  const architectures = {
    arm64: "arm64",
    x64: "amd64",
  };
  const operatingSystem = operatingSystems[platform];
  const goArchitecture = architectures[architecture];
  if (!operatingSystem || !goArchitecture) {
    throw new Error(`unsupported platform ${platform}/${architecture}`);
  }
  return {
    operatingSystem,
    architecture: goArchitecture,
    extension: platform === "win32" ? ".exe" : "",
  };
}

export function releaseAssetName(version, platform, architecture) {
  if (!VERSION_PATTERN.test(version)) {
    throw new Error(`invalid package version ${version}`);
  }
  const target = releaseTarget(platform, architecture);
  return `viceme_${version}_${target.operatingSystem}_${target.architecture}${target.extension}`;
}

export async function ensureBinary({
  packageVersion,
  environment = process.env,
  platform = process.platform,
  architecture = process.arch,
  fetchImplementation = globalThis.fetch,
  releaseBaseURL = RELEASE_BASE_URL,
  cacheDirectory,
  allowInsecureURL = false,
}) {
  if (environment.VICEME_BINARY_PATH) {
    const overridden = path.resolve(environment.VICEME_BINARY_PATH);
    await access(overridden, constants.X_OK);
    return overridden;
  }
  if (typeof fetchImplementation !== "function") {
    throw new Error("Node.js fetch support is required");
  }
  const asset = releaseAssetName(packageVersion, platform, architecture);
  const root = cacheDirectory ?? defaultCacheDirectory(environment, platform);
  const destinationDirectory = path.join(root, "cli", packageVersion);
  const generationsDirectory = path.join(destinationDirectory, GENERATIONS_DIRECTORY);
  const cached = await findValidGeneration(generationsDirectory, asset);
  if (cached) {
    return cached;
  }
  await mkdir(generationsDirectory, { recursive: true, mode: 0o700 });
  const versionURL = `${releaseBaseURL}/v${packageVersion}`;
  const checksumURL = `${versionURL}/${asset}.sha256`;
  const binaryURL = `${versionURL}/${asset}`;
  const [checksumDocument, binary] = await Promise.all([
    download(checksumURL, fetchImplementation, allowInsecureURL),
    download(binaryURL, fetchImplementation, allowInsecureURL),
  ]);
  const expectedChecksum = parseChecksum(checksumDocument.toString("utf8"));
  const actualChecksum = digest(binary);
  if (actualChecksum !== expectedChecksum) {
    throw new Error(`checksum mismatch for ${asset}`);
  }

  // If another cold start finished while this process downloaded, reuse its
  // complete immutable generation. Duplicate downloads are harmless; shared
  // partially-written binary/checksum pairs do not exist.
  const publishedByContender = await findValidGeneration(generationsDirectory, asset);
  if (publishedByContender) {
    return publishedByContender;
  }

  const generationID = `${expectedChecksum.slice(0, 16)}-${randomUUID()}`;
  const stagingDirectory = path.join(
    destinationDirectory,
    `${STAGING_PREFIX}${process.pid}-${generationID}`,
  );
  const generationDirectory = path.join(
    generationsDirectory,
    `${GENERATION_PREFIX}${generationID}`,
  );
  const stagedBinary = path.join(stagingDirectory, asset);
  const stagedChecksum = `${stagedBinary}.sha256`;
  try {
    await mkdir(stagingDirectory, { mode: 0o700 });
    await writeFile(stagedBinary, binary, { mode: 0o700 });
    await chmod(stagedBinary, 0o700);
    await writeFile(stagedChecksum, `${expectedChecksum}  ${asset}\n`, { mode: 0o600 });
    if (!(await cachedBinaryIsValid(stagedBinary, stagedChecksum))) {
      throw new Error(`staged binary verification failed for ${asset}`);
    }

    // Staging and generations share a parent filesystem. Renaming the complete
    // unique directory is the only publication step, and its destination is
    // never replaced or removed by another installer process.
    await rename(stagingDirectory, generationDirectory);
  } finally {
    await rm(stagingDirectory, { recursive: true, force: true });
  }
  const installedBinary = path.join(generationDirectory, asset);
  if (!(await cachedBinaryIsValid(installedBinary, `${installedBinary}.sha256`))) {
    throw new Error(`published binary verification failed for ${asset}`);
  }
  return installedBinary;
}

async function findValidGeneration(generationsDirectory, asset) {
  let entries;
  try {
    entries = await readdir(generationsDirectory, { withFileTypes: true });
  } catch (error) {
    if (error.code === "ENOENT") {
      return undefined;
    }
    throw error;
  }
  for (const entry of entries
    .filter((candidate) => candidate.isDirectory() && candidate.name.startsWith(GENERATION_PREFIX))
    .sort((left, right) => left.name.localeCompare(right.name))) {
    const binaryPath = path.join(generationsDirectory, entry.name, asset);
    if (await cachedBinaryIsValid(binaryPath, `${binaryPath}.sha256`)) {
      return binaryPath;
    }
  }
  return undefined;
}

function defaultCacheDirectory(environment, platform) {
  if (environment.VICEME_CACHE_DIR) {
    return path.resolve(environment.VICEME_CACHE_DIR);
  }
  if (environment.XDG_CACHE_HOME) {
    return path.join(environment.XDG_CACHE_HOME, "viceme");
  }
  if (platform === "win32" && environment.LOCALAPPDATA) {
    return path.join(environment.LOCALAPPDATA, "Viceme", "Cache");
  }
  return path.join(os.homedir(), ".cache", "viceme");
}

async function cachedBinaryIsValid(binaryPath, checksumPath) {
  try {
    const [binary, checksumDocument] = await Promise.all([
      readFile(binaryPath),
      readFile(checksumPath, "utf8"),
      access(binaryPath, constants.X_OK),
    ]);
    const expected = parseChecksum(checksumDocument);
    return digest(binary) === expected;
  } catch {
    return false;
  }
}

async function download(url, fetchImplementation, allowInsecureURL) {
  const parsed = new URL(url);
  if (!allowInsecureURL && parsed.protocol !== "https:") {
    throw new Error(`refusing non-HTTPS release URL ${url}`);
  }
  const response = await fetchImplementation(url, {
    redirect: "follow",
    headers: { "user-agent": "@viceme-ai/cli npm launcher" },
  });
  if (!response.ok) {
    throw new Error(`download failed (${response.status}) for ${url}`);
  }
  if (!allowInsecureURL && response.url && new URL(response.url).protocol !== "https:") {
    throw new Error(`release download redirected to a non-HTTPS URL`);
  }
  return Buffer.from(await response.arrayBuffer());
}

function parseChecksum(document) {
  const match = document.trim().match(/^([a-fA-F0-9]{64})(?:\s|$)/);
  if (!match) {
    throw new Error("release checksum document is invalid");
  }
  return match[1].toLowerCase();
}

function digest(buffer) {
  return createHash("sha256").update(buffer).digest("hex");
}
