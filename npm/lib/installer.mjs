import { spawn } from "node:child_process";
import { createHash, randomUUID } from "node:crypto";
import { access, chmod, mkdir, readFile, readdir, rename, rm, writeFile } from "node:fs/promises";
import { constants } from "node:fs";
import os from "node:os";
import path from "node:path";

const GITHUB_RELEASE_BASE_URL = "https://github.com/ViceMe-AI/cli/releases/download";
const NPM_REGISTRY_URL = "https://registry.npmjs.org";
const NPMMIRROR_BINARY_BASE_URL =
  "https://registry.npmmirror.com/-/binary/viceme-cli";
const CHECKSUMS_URL = new URL("../../checksums.txt", import.meta.url);
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

export function binaryDownloadURLs({
  packageVersion,
  asset,
  environment = process.env,
  sourceBaseURLs,
  allowInsecureURL = false,
}) {
  const bases = sourceBaseURLs ?? defaultBinarySourceBaseURLs(environment);
  const urls = [];
  const seen = new Set();
  for (const base of bases) {
    const parsed = parseDownloadBaseURL(base, allowInsecureURL);
    const normalizedBase = parsed.href.replace(/\/$/, "");
    const url = `${normalizedBase}/v${packageVersion}/${asset}`;
    if (!seen.has(url)) {
      seen.add(url);
      urls.push(url);
    }
  }
  if (urls.length === 0) {
    throw new Error("no valid binary download source is configured");
  }
  return urls;
}

export async function ensureBinary({
  packageVersion,
  environment = process.env,
  platform = process.platform,
  architecture = process.arch,
  downloadImplementation = downloadWithCurl,
  sourceBaseURLs,
  cacheDirectory,
  checksumsDocument,
  allowInsecureURL = false,
}) {
  if (environment.VICEME_BINARY_PATH) {
    const overridden = path.resolve(environment.VICEME_BINARY_PATH);
    await access(overridden, constants.X_OK);
    return overridden;
  }
  if (typeof downloadImplementation !== "function") {
    throw new Error("a binary download implementation is required");
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
  const checksumSource =
    checksumsDocument ?? (await readFile(CHECKSUMS_URL, "utf8"));
  const expectedChecksum = checksumForAsset(checksumSource, asset);
  const urls = binaryDownloadURLs({
    packageVersion,
    asset,
    environment,
    sourceBaseURLs,
    allowInsecureURL,
  });
  const binary = await downloadVerifiedBinary({
    asset,
    expectedChecksum,
    urls,
    downloadImplementation,
    allowInsecureURL,
  });

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

function defaultBinarySourceBaseURLs(environment) {
  const sources = [GITHUB_RELEASE_BASE_URL];
  const configuredRegistry =
    environment.npm_config_registry ?? environment.NPM_CONFIG_REGISTRY;
  const registryBinaryBase = customRegistryBinaryBaseURL(configuredRegistry);
  if (registryBinaryBase) {
    sources.push(registryBinaryBase);
  }
  sources.push(NPMMIRROR_BINARY_BASE_URL);
  return sources;
}

function customRegistryBinaryBaseURL(value) {
  if (!value) {
    return undefined;
  }
  let registry;
  try {
    registry = new URL(value);
  } catch {
    return undefined;
  }
  if (
    registry.protocol !== "https:" ||
    registry.username ||
    registry.password ||
    registry.origin === new URL(NPM_REGISTRY_URL).origin
  ) {
    return undefined;
  }
  return `${registry.href.replace(/\/$/, "")}/-/binary/viceme-cli`;
}

function parseDownloadBaseURL(value, allowInsecureURL) {
  let parsed;
  try {
    parsed = new URL(value);
  } catch {
    throw new Error(`invalid binary download source ${value}`);
  }
  if (parsed.username || parsed.password) {
    throw new Error("binary download sources must not contain credentials");
  }
  if (!allowInsecureURL && parsed.protocol !== "https:") {
    throw new Error(`refusing non-HTTPS binary download source ${value}`);
  }
  if (allowInsecureURL && !["http:", "https:"].includes(parsed.protocol)) {
    throw new Error(`unsupported binary download protocol ${parsed.protocol}`);
  }
  return parsed;
}

async function downloadVerifiedBinary({
  asset,
  expectedChecksum,
  urls,
  downloadImplementation,
  allowInsecureURL,
}) {
  const failures = [];
  for (const url of urls) {
    try {
      const downloaded = await downloadImplementation(url, { allowInsecureURL });
      const binary = Buffer.isBuffer(downloaded) ? downloaded : Buffer.from(downloaded);
      if (digest(binary) !== expectedChecksum) {
        throw new Error(`checksum mismatch for ${asset}`);
      }
      return binary;
    } catch (error) {
      failures.push(`- ${url}: ${errorMessage(error)}`);
    }
  }
  throw new Error(
    `could not download a verified ${asset}; attempted:\n${failures.join("\n")}`,
  );
}

async function downloadWithCurl(url, { allowInsecureURL = false } = {}) {
  const parsed = parseDownloadBaseURL(url, allowInsecureURL);
  const arguments_ = [
    "--fail",
    "--location",
    "--silent",
    "--show-error",
    "--connect-timeout",
    "10",
    "--max-time",
    "120",
    "--max-redirs",
    "5",
  ];
  if (!allowInsecureURL) {
    arguments_.push("--proto", "=https", "--proto-redir", "=https");
  }
  arguments_.push(parsed.href);
  return await new Promise((resolve, reject) => {
    const child = spawn("curl", arguments_, {
      stdio: ["ignore", "pipe", "pipe"],
      windowsHide: true,
    });
    const stdout = [];
    let stderr = "";
    child.stdout.on("data", (chunk) => stdout.push(chunk));
    child.stderr.on("data", (chunk) => {
      stderr += chunk;
    });
    child.on("error", (error) => {
      if (error.code === "ENOENT") {
        reject(new Error("curl is required to download the ViceMe CLI binary"));
        return;
      }
      reject(error);
    });
    child.on("close", (code, signal) => {
      if (code === 0) {
        resolve(Buffer.concat(stdout));
        return;
      }
      const reason = stderr.trim() || `curl exited ${code ?? `on signal ${signal}`}`;
      reject(new Error(reason));
    });
  });
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
    return path.join(environment.LOCALAPPDATA, "ViceMe", "Cache");
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

function checksumForAsset(document, asset) {
  let checksum;
  for (const line of document.split(/\r?\n/)) {
    if (line.trim() === "") {
      continue;
    }
    const match = line.match(/^([a-fA-F0-9]{64})\s+([^\s]+)$/);
    if (!match) {
      throw new Error("bundled checksum document is invalid");
    }
    if (match[2] === asset) {
      if (checksum) {
        throw new Error(`bundled checksum document contains duplicate ${asset}`);
      }
      checksum = match[1].toLowerCase();
    }
  }
  if (!checksum) {
    throw new Error(`bundled checksum document does not contain ${asset}`);
  }
  return checksum;
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

function errorMessage(error) {
  return error instanceof Error ? error.message : String(error);
}
