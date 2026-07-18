#!/usr/bin/env node

import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { pathToFileURL } from "node:url";

const semverPattern = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/;

export function parseVersion(raw) {
  const match = semverPattern.exec(raw.trim());
  if (!match) {
    throw new Error(`invalid stable semantic version: ${raw}`);
  }
  return {
    major: Number(match[1]),
    minor: Number(match[2]),
    patch: Number(match[3]),
  };
}

export function incrementVersion(raw, bump) {
  const version = parseVersion(raw);
  if (bump === "major") {
    return `${version.major + 1}.0.0`;
  }
  if (bump === "minor") {
    return `${version.major}.${version.minor + 1}.0`;
  }
  if (bump === "patch") {
    return `${version.major}.${version.minor}.${version.patch + 1}`;
  }
  throw new Error(`unsupported version bump: ${bump}`);
}

export function compatibilityRange(raw) {
  const version = parseVersion(raw);
  const upper = version.major === 0
    ? `0.${version.minor + 1}.0`
    : `${version.major + 1}.0.0`;
  return `>=${raw} <${upper}`;
}

export function parseConventionalCommit(commit) {
  const subject = commit.subject.trim();
  const conventional = /^([a-zA-Z][\w-]*)(?:\([^)]*\))?(!)?:\s+(.+)$/.exec(subject);
  const type = conventional?.[1]?.toLowerCase() ?? "other";
  const summary = conventional?.[3]?.trim() ?? subject;
  const breaking = Boolean(conventional?.[2]) || /(^|\n)BREAKING[ -]CHANGE:\s+/i.test(commit.body);
  let group = "other";
  if (type === "feat") {
    group = "features";
  } else if (type === "fix" || type === "perf") {
    group = "fixes";
  }
  return { ...commit, type, summary, breaking, group };
}

export function selectBump(commits) {
  if (commits.some((commit) => commit.breaking)) {
    return "major";
  }
  if (commits.some((commit) => commit.type === "feat")) {
    return "minor";
  }
  return "patch";
}

export function renderChangelog(version, commits, previous, date) {
  const parsed = commits.map(parseConventionalCommit);
  const sections = [
    ["Breaking Changes", parsed.filter((commit) => commit.breaking)],
    ["Features", parsed.filter((commit) => !commit.breaking && commit.group === "features")],
    ["Fixes", parsed.filter((commit) => !commit.breaking && commit.group === "fixes")],
    ["Other Changes", parsed.filter((commit) => !commit.breaking && commit.group === "other")],
  ];
  const lines = ["# Changelog", "", `## [${version}] - ${date}`, ""];
  for (const [heading, entries] of sections) {
    if (entries.length === 0) {
      continue;
    }
    lines.push(`### ${heading}`, "");
    for (const entry of entries) {
      lines.push(`- ${entry.summary} (\`${entry.sha.slice(0, 7)}\`)`);
    }
    lines.push("");
  }
  const oldBody = previous
    .replace(/^# Changelog\s*/u, "")
    .trim();
  if (oldBody !== "") {
    lines.push(oldBody, "");
  }
  return `${lines.join("\n").trimEnd()}\n`;
}

function git(args, options = {}) {
  return execFileSync("git", args, {
    encoding: "utf8",
    stdio: ["ignore", "pipe", options.allowFailure ? "pipe" : "inherit"],
  }).trim();
}

function gitOptional(args) {
  try {
    return git(args, { allowFailure: true });
  } catch {
    return "";
  }
}

function latestReachableReleaseTag() {
  const tags = gitOptional(["tag", "--merged", "HEAD", "--list", "v[0-9]*", "--sort=-v:refname"])
    .split("\n")
    .map((tag) => tag.trim())
    .filter(Boolean);
  return tags.find((tag) => semverPattern.test(tag.slice(1))) ?? "";
}

function commitsSince(ref) {
  const record = "%H%x1f%s%x1f%b%x1e";
  const output = gitOptional(["log", "--no-merges", `--format=${record}`, `${ref}..HEAD`]);
  if (output === "") {
    return [];
  }
  return output
    .split("\x1e")
    .map((entry) => entry.trim())
    .filter(Boolean)
    .map((entry) => {
      const [sha, subject, body = ""] = entry.split("\x1f");
      return { sha, subject, body };
    })
    .filter((commit) => !/^chore\(release\):\s+v\d+\.\d+\.\d+$/i.test(commit.subject));
}

function readAtRef(ref, filename) {
  return gitOptional(["show", `${ref}:${filename}`]);
}

function writeJSON(filename, value) {
  writeFileSync(filename, `${JSON.stringify(value, null, 2)}\n`);
}

function replaceRequired(source, pattern, replacement, label) {
  if (!pattern.test(source)) {
    throw new Error(`could not update ${label}`);
  }
  return source.replace(pattern, replacement);
}

function prepareRelease() {
  const fallbackRefIndex = process.argv.indexOf("--fallback-ref");
  const fallbackRef = fallbackRefIndex >= 0 ? process.argv[fallbackRefIndex + 1] : "origin/main";
  const releaseTag = latestReachableReleaseTag();
  const baseRef = releaseTag || fallbackRef;
  if (!gitOptional(["rev-parse", "--verify", baseRef])) {
    throw new Error(`release base ref does not exist: ${baseRef}`);
  }

  const commits = commitsSince(baseRef);
  if (commits.length === 0) {
    throw new Error(`no unreleased commits found after ${baseRef}`);
  }

  const packageDocument = JSON.parse(readFileSync("package.json", "utf8"));
  let previousVersion = "";
  const basePackage = readAtRef(baseRef, "package.json");
  if (basePackage !== "") {
    previousVersion = JSON.parse(basePackage).version;
  } else if (releaseTag !== "") {
    previousVersion = releaseTag.slice(1);
  }
  const parsedCommits = commits.map(parseConventionalCommit);
  const bump = previousVersion === "" ? "initial" : selectBump(parsedCommits);
  const version = previousVersion === "" ? packageDocument.version : incrementVersion(previousVersion, bump);
  parseVersion(version);
  const compatibility = compatibilityRange(version);

  packageDocument.version = version;
  writeJSON("package.json", packageDocument);

  const skillPackage = JSON.parse(readFileSync("skills/viceme/skill-package.json", "utf8"));
  skillPackage.skill_version = version;
  skillPackage.minimum_cli_version = version;
  skillPackage.cli_compatibility = compatibility;
  writeJSON("skills/viceme/skill-package.json", skillPackage);

  let buildinfo = readFileSync("internal/buildinfo/buildinfo.go", "utf8");
  buildinfo = replaceRequired(buildinfo, /ReleaseVersion = "[^"]+"/, `ReleaseVersion = "${version}"`, "ReleaseVersion");
  buildinfo = replaceRequired(buildinfo, /SkillVersion\s+= "[^"]+"/, `SkillVersion      = "${version}"`, "SkillVersion");
  buildinfo = replaceRequired(buildinfo, /MinimumCLIVersion = "[^"]+"/, `MinimumCLIVersion = "${version}"`, "MinimumCLIVersion");
  buildinfo = replaceRequired(buildinfo, /CLICompatibility\s+= "[^"]+"/, `CLICompatibility  = "${compatibility}"`, "CLICompatibility");
  writeFileSync("internal/buildinfo/buildinfo.go", buildinfo);

  const previousChangelog = readAtRef(baseRef, "CHANGELOG.md");
  const date = new Date().toISOString().slice(0, 10);
  writeFileSync("CHANGELOG.md", renderChangelog(version, commits, previousChangelog, date));

  process.stdout.write(`${JSON.stringify({ version, bump, base_ref: baseRef, commit_count: commits.length })}\n`);
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    prepareRelease();
  } catch (error) {
    process.stderr.write(`${error instanceof Error ? error.message : String(error)}\n`);
    process.exitCode = 1;
  }
}
