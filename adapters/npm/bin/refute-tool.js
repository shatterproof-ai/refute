#!/usr/bin/env node
const fs = require("fs");
const path = require("path");
const crypto = require("crypto");
const zlib = require("zlib");
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
    ensureRealDirectory(path.dirname(ACTIVE));
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
  try {
    extractRefuteBinary(archive, cachedBinary);
  } catch (err) {
    console.error(err.message);
    return 1;
  }
  installFileAtomic(cachedBinary, ACTIVE, 0o755);
  writeFileAtomic(`${ACTIVE}.artifact-sha256`, `${artifactSha}\n`, 0o644);
  writeFileAtomic(`${ACTIVE}.binary-sha256`, `${sha256(ACTIVE)}\n`, 0o644);
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
  return isRegularNonSymlink(file) && fs.readFileSync(file, "utf8").trim() === digest;
}

function activeMatches(artifactDigest) {
  return isRegularNonSymlink(ACTIVE)
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

function isRegularNonSymlink(file) {
  try {
    return fs.lstatSync(file).isFile();
  } catch (err) {
    if (err.code === "ENOENT") return false;
    throw err;
  }
}

function extractRefuteBinary(archive, dest) {
  const tar = zlib.gunzipSync(fs.readFileSync(archive));
  for (const entry of tarEntries(tar)) {
    if (tarMemberBase(entry.name) !== "refute") continue;
    if (!safeTarMemberName(entry.name)) {
      throw new Error(`${archive} contains unsafe refute member ${JSON.stringify(entry.name)}`);
    }
    if (!isRegularTarType(entry.typeflag)) {
      throw new Error(`${archive} refute member is not a regular file`);
    }
    writeBufferAtomic(dest, entry.body, 0o755);
    return;
  }
  throw new Error(`${archive} does not contain refute`);
}

function tarEntries(tar) {
  const entries = [];
  for (let offset = 0; offset + 512 <= tar.length;) {
    const header = tar.subarray(offset, offset + 512);
    if (header.every((byte) => byte === 0)) break;
    const name = tarHeaderName(header);
    const size = parseTarSize(header.subarray(124, 136));
    const typeflag = header[156] === 0 ? "\0" : String.fromCharCode(header[156]);
    const bodyStart = offset + 512;
    const bodyEnd = bodyStart + size;
    if (bodyEnd > tar.length) throw new Error("truncated tar archive");
    entries.push({ name, typeflag, body: tar.subarray(bodyStart, bodyEnd) });
    offset = bodyStart + Math.ceil(size / 512) * 512;
  }
  return entries;
}

function tarHeaderName(header) {
  const name = tarString(header.subarray(0, 100));
  const prefix = tarString(header.subarray(345, 500));
  return prefix ? `${prefix}/${name}` : name;
}

function tarString(bytes) {
  const end = bytes.indexOf(0);
  return bytes.subarray(0, end === -1 ? bytes.length : end).toString("utf8");
}

function parseTarSize(bytes) {
  const value = tarString(bytes).trim();
  if (value === "") return 0;
  const size = Number.parseInt(value, 8);
  if (!Number.isFinite(size) || size < 0) throw new Error(`invalid tar member size ${JSON.stringify(value)}`);
  return size;
}

function isRegularTarType(typeflag) {
  return typeflag === "0" || typeflag === "\0";
}

function safeTarMemberName(name) {
  if (!name || name.startsWith("/") || name.startsWith("\\") || hasWindowsDrivePrefix(name)) return false;
  return !tarMemberParts(name).includes("..");
}

function tarMemberBase(name) {
  const parts = tarMemberParts(name);
  return parts.length === 0 ? "" : parts[parts.length - 1];
}

function tarMemberParts(name) {
  return name.split(/[\\/]+/).filter(Boolean);
}

function installFileAtomic(src, dest, mode) {
  const tmp = tempPath(dest);
  try {
    fs.copyFileSync(src, tmp);
    fs.chmodSync(tmp, mode);
    fs.renameSync(tmp, dest);
  } finally {
    fs.rmSync(tmp, { force: true });
  }
}

function writeBufferAtomic(dest, data, mode) {
  const tmp = tempPath(dest);
  try {
    fs.writeFileSync(tmp, data, { mode, flag: "wx" });
    fs.renameSync(tmp, dest);
  } finally {
    fs.rmSync(tmp, { force: true });
  }
}

function writeFileAtomic(dest, data, mode) {
  const tmp = tempPath(dest);
  try {
    fs.writeFileSync(tmp, data, { mode, flag: "wx" });
    fs.renameSync(tmp, dest);
  } finally {
    fs.rmSync(tmp, { force: true });
  }
}

function tempPath(dest) {
  const name = path.basename(dest);
  const suffix = `${process.pid}-${Date.now()}-${Math.random().toString(16).slice(2)}`;
  return path.join(path.dirname(dest), `.${name}-${suffix}`);
}

function platform() {
  return process.platform === "darwin" ? "darwin" : process.platform;
}

function arch(value) {
  return value === "amd64" ? "x64" : value;
}

if (require.main === module) process.exit(main());
module.exports = { main };
