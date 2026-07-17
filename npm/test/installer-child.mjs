import { ensureBinary } from "../lib/installer.mjs";

const [cacheDirectory, releaseBaseURL] = process.argv.slice(2);
const installed = await ensureBinary({
  packageVersion: "0.1.0",
  platform: "linux",
  architecture: "x64",
  cacheDirectory,
  releaseBaseURL,
  allowInsecureURL: true,
  environment: {},
});
process.stdout.write(`${installed}\n`);
