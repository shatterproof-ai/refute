const assert = require("assert");
const crypto = require("crypto");
const fs = require("fs");
const os = require("os");
const path = require("path");
const zlib = require("zlib");
const { spawnSync } = require("child_process");

function main() {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "refute-npm-bin-symlink-"));
  const archive = writeArchive(root);
  const digest = sha256(archive);
  writeLock(root, archive, digest);
  fs.mkdirSync(path.join(root, ".refute", "cache"), { recursive: true });
  const outside = path.join(root, "outside-bin");
  fs.mkdirSync(outside);
  try {
    fs.symlinkSync(outside, path.join(root, ".refute", "bin"), "dir");
  } catch (err) {
    console.log(`symlinked bin test skipped: ${err.message}`);
    return;
  }

  const result = spawnSync(process.execPath, [path.join(__dirname, "..", "bin", "refute-tool.js"), "sync"], {
    cwd: root,
    encoding: "utf8",
  });
  assert.notStrictEqual(result.status, 0, "sync unexpectedly accepted symlinked .refute/bin");
  assert.deepStrictEqual(fs.readdirSync(outside), [], "sync wrote through symlinked .refute/bin");
}

function writeArchive(root) {
  const body = Buffer.from("#!/bin/sh\necho synced\n");
  const padding = Buffer.alloc((512 - (body.length % 512)) % 512, 0);
  const archive = path.join(root, "archive.tar.gz");
  const tar = Buffer.concat([tarHeader("refute", body.length), body, padding, Buffer.alloc(1024, 0)]);
  fs.writeFileSync(archive, zlib.gzipSync(tar));
  return archive;
}

function tarHeader(name, bodyLength) {
  const header = Buffer.alloc(512, 0);
  header.write(name, 0, 100, "utf8");
  header.write("0000755\0", 100, 8, "ascii");
  header.write("0000000\0", 108, 8, "ascii");
  header.write("0000000\0", 116, 8, "ascii");
  header.write(bodyLength.toString(8).padStart(11, "0") + "\0", 124, 12, "ascii");
  header.write(Math.floor(Date.now() / 1000).toString(8).padStart(11, "0") + "\0", 136, 12, "ascii");
  header.fill(" ", 148, 156);
  header.write("0", 156, 1, "ascii");
  header.write("ustar\0", 257, 6, "ascii");
  header.write("00", 263, 2, "ascii");
  let sum = 0;
  for (const byte of header) sum += byte;
  header.write(sum.toString(8).padStart(6, "0") + "\0 ", 148, 8, "ascii");
  return header;
}

function writeLock(root, archive, digest) {
  const platform = process.platform === "darwin" ? "darwin" : process.platform;
  const arch = process.arch === "x64" ? "amd64" : process.arch;
  fs.writeFileSync(path.join(root, "refute.lock.json"), JSON.stringify({
    version: "v9.9.9",
    artifacts: [{ platform, architecture: arch, url: `file://${archive}`, sha256: digest, filename: "artifact.tar.gz" }],
  }));
}

function sha256(file) {
  return crypto.createHash("sha256").update(fs.readFileSync(file)).digest("hex");
}

main();
