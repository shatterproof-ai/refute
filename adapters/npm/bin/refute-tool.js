#!/usr/bin/env node
const fs = require("fs");
const path = require("path");
const crypto = require("crypto");
const { spawnSync } = require("child_process");

const ACTIVE = path.join(".refute", "bin", "refute");

function main(argv = process.argv.slice(2)) {
  const cmd = argv[0];
  if (!cmd || cmd === "-h" || cmd === "--help") {
    console.log("usage: refute-tool <sync|run|doctor>");
    return 0;
  }
  if (cmd === "sync") return sync();
  if (cmd === "doctor") return doctor();
  if (cmd === "run") {
    const args = argv[1] === "--" ? argv.slice(2) : argv.slice(1);
    return run(args);
  }
  console.error(`unknown refute-tool command ${cmd}`);
  return 2;
}

function sync() {
  const lock = readLock();
  const artifact = lock.artifacts.find((item) => item.platform === platform() && arch(item.architecture) === process.arch);
  if (!artifact) {
    console.error(`unsupported platform ${platform()}/${process.arch} for refute ${lock.version}`);
    return 1;
  }
  const validationError = validateArtifact(artifact);
  if (validationError) {
    console.error(validationError);
    return 1;
  }
  const artifactSha = artifact.sha256;
  try {
    ensureRealDirectory(".refute");
    ensureRealDirectory(path.join(".refute", "cache"));
  } catch (err) {
    console.error(err.message);
    return 1;
  }
  if (activeMatches(artifactSha)) {
    console.log(`${ACTIVE} is already current`);
    return 0;
  }
  let cacheDir;
  let archive;
  let cachedBinary;
  try {
    cacheDir = pathUnder(path.join(".refute", "cache"), artifactSha);
    archive = pathUnder(cacheDir, artifact.filename || "artifact.tar.gz");
    cachedBinary = pathUnder(cacheDir, "refute");
  } catch (err) {
    console.error(err.message);
    return 1;
  }
  fs.rmSync(cacheDir, { recursive: true, force: true });
  fs.mkdirSync(cacheDir, { recursive: true });
  if (!copyArtifact(artifact.url, archive)) return 1;
  const got = sha256(archive);
  if (got !== artifactSha) {
    console.error(`checksum mismatch for ${artifact.url}: got ${got}, want ${artifactSha}`);
    return 1;
  }
  const extract = spawnSync("tar", ["-xzf", archive, "-C", cacheDir, "refute"], { stdio: "inherit" });
  if (extract.status !== 0) return extract.status || 1;
  fs.mkdirSync(path.dirname(ACTIVE), { recursive: true });
  fs.copyFileSync(cachedBinary, ACTIVE);
  fs.chmodSync(ACTIVE, 0o755);
  fs.writeFileSync(`${ACTIVE}.artifact-sha256`, `${artifactSha}\n`);
  fs.writeFileSync(`${ACTIVE}.binary-sha256`, `${sha256(ACTIVE)}\n`);
  console.log(`installed ${ACTIVE}`);
  return 0;
}

function doctor() {
  console.log(fs.existsSync("refute.lock.json") ? "lockfile: present" : "lockfile: missing");
  if (!fs.existsSync(ACTIVE)) {
    console.log(`binary: missing (${ACTIVE})`);
    return 0;
  }
  console.log(`binary: present (${ACTIVE})`);
  return run(["doctor"]);
}

function run(args) {
  const child = spawnSync(path.resolve(ACTIVE), args, { stdio: "inherit" });
  if (child.error) {
    console.error(child.error.message);
    return 1;
  }
  return child.status ?? 0;
}

function readLock() {
  return JSON.parse(fs.readFileSync("refute.lock.json", "utf8"));
}

function copyArtifact(rawUrl, dest) {
  if (rawUrl.startsWith("file://")) {
    fs.copyFileSync(new URL(rawUrl), dest);
    return true;
  }
  const fetch = globalThis.fetch;
  if (!fetch) {
    console.error("https downloads require Node.js with fetch support");
    return false;
  }
  const child = spawnSync(process.execPath, ["-e", `
    fetch(process.argv[1]).then(async (r) => {
      if (!r.ok) throw new Error(r.status + " " + r.statusText);
      const b = Buffer.from(await r.arrayBuffer());
      require("fs").writeFileSync(process.argv[2], b);
    }).catch((e) => { console.error(e.message); process.exit(1); });
  `, rawUrl, dest], { stdio: "inherit" });
  return child.status === 0;
}

function sha256(file) {
  return crypto.createHash("sha256").update(fs.readFileSync(file)).digest("hex");
}

function markerMatches(file, digest) {
  return fs.existsSync(file) && fs.readFileSync(file, "utf8").trim() === digest;
}

function activeMatches(artifactDigest) {
  return fs.existsSync(ACTIVE)
    && markerMatches(`${ACTIVE}.artifact-sha256`, artifactDigest)
    && markerMatches(`${ACTIVE}.binary-sha256`, sha256(ACTIVE));
}

function validateArtifact(artifact) {
  if (!isSHA256Hex(artifact.sha256)) return `invalid artifact sha256 ${JSON.stringify(artifact.sha256)}`;
  if (artifact.filename !== undefined && !safeLockFilename(artifact.filename)) {
    return `unsafe artifact filename ${JSON.stringify(artifact.filename)}`;
  }
  return "";
}

function isSHA256Hex(value) {
  return typeof value === "string" && /^[0-9a-fA-F]{64}$/.test(value);
}

function safeLockFilename(name) {
  return typeof name === "string"
    && name !== ""
    && !hasWindowsDrivePrefix(name)
    && !name.includes("/")
    && !name.includes("\\")
    && !name.includes("..");
}

function hasWindowsDrivePrefix(name) {
  return /^[A-Za-z]:/.test(name);
}

function pathUnder(root, child) {
  const rootPath = path.resolve(root);
  const candidate = path.resolve(root, child);
  const rel = path.relative(rootPath, candidate);
  if (rel === ".." || rel.startsWith(`..${path.sep}`) || path.isAbsolute(rel)) {
    throw new Error(`path ${candidate} escapes ${rootPath}`);
  }
  return candidate;
}

function ensureRealDirectory(dir) {
  try {
    const info = fs.lstatSync(dir);
    if (info.isSymbolicLink() || !info.isDirectory()) {
      throw new Error(`${dir} is not a real directory`);
    }
  } catch (err) {
    if (err.code !== "ENOENT") throw err;
    fs.mkdirSync(dir);
  }
}

function platform() {
  return process.platform === "darwin" ? "darwin" : process.platform;
}

function arch(value) {
  return value === "amd64" ? "x64" : value;
}

if (require.main === module) process.exit(main());
module.exports = { main };
