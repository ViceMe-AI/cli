import { ensureBinary } from "../lib/installer.mjs";

const [cacheDirectory, releaseBaseURL, checksumsDocument] = process.argv.slice(2);
const installed = await ensureBinary({
  packageVersion: "0.1.0",
  platform: "linux",
  architecture: "x64",
  cacheDirectory,
  sourceBaseURLs: [releaseBaseURL],
  checksumsDocument,
  allowInsecureURL: true,
  environment: {},
});
process.stdout.write(`${installed}\n`);
