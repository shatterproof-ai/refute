const assert = require("assert");
const crypto = require("crypto");
const fs = require("fs");
const os = require("os");
const path = require("path");
const zlib = require("zlib");
const { spawnSync } = require("child_process");

function main() {
  testRejectsUnsafeTarMembers();
  testRejectsLinkTarMembers();
  testRejectsSymlinkedBinRoot();
  testReplacesSymlinkedActiveFiles();
  testSyncWalksUpToLockfile();
  testRefuteBinPropagatesFailureExit();
  testRefuteBinPropagatesSignalDeath();
}

// Sync invoked from a subdirectory must install into the lockfile directory's
// .refute, not the subdirectory's cwd.
function testSyncWalksUpToLockfile() {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "refute-npm-walkup-"));
  const archive = writeArchive(root);
  const digest = sha256(archive);
  writeLock(root, archive, digest);
  const nested = path.join(root, "a", "b");
  fs.mkdirSync(nested, { recursive: true });

  const result = spawnSync(process.execPath, [path.join(__dirname, "..", "bin", "refute-tool.js"), "sync"], {
    cwd: nested,
    encoding: "utf8",
  });
  assert.strictEqual(result.status, 0, result.stderr || result.stdout);
  assert.strictEqual(fs.existsSync(path.join(root, ".refute", "bin", "refute")), true, "sync did not install into lockfile root");
  assert.strictEqual(fs.existsSync(path.join(nested, ".refute")), false, "sync installed into the subdirectory");
}

// The `refute` bin must exit with the delegated binary's non-zero status
// instead of always succeeding.
function testRefuteBinPropagatesFailureExit() {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "refute-npm-exit-"));
  installFakeBinary(root, "#!/bin/sh\nexit 3\n");

  const result = spawnSync(process.execPath, [path.join(__dirname, "..", "bin", "refute.js"), "anything"], {
    cwd: root,
    encoding: "utf8",
  });
  assert.strictEqual(result.status, 3, `refute bin did not propagate exit 3, got ${result.status}`);
}

// A signal death must surface as a non-zero status, not exit 0.
function testRefuteBinPropagatesSignalDeath() {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "refute-npm-signal-"));
  installFakeBinary(root, "#!/bin/sh\nkill -TERM $$\n");

  const result = spawnSync(process.execPath, [path.join(__dirname, "..", "bin", "refute.js"), "anything"], {
    cwd: root,
    encoding: "utf8",
  });
  assert.notStrictEqual(result.status, 0, "refute bin reported signal death as success");
}

function installFakeBinary(root, script) {
  const binDir = path.join(root, ".refute", "bin");
  fs.mkdirSync(binDir, { recursive: true });
  const active = path.join(binDir, "refute");
  fs.writeFileSync(active, script, { mode: 0o755 });
  fs.chmodSync(active, 0o755);
}

function testRejectsUnsafeTarMembers() {
  for (const name of ["/tmp/refute", "C:/tmp/refute", "..\\..\\refute", "\\\\server\\share\\refute"]) {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), "refute-npm-unsafe-tar-"));
    const archive = writeArchive(root, { name });
    const digest = sha256(archive);
    writeLock(root, archive, digest);

    const result = runSync(root);
    assert.notStrictEqual(result.status, 0, `sync unexpectedly accepted unsafe tar member ${name}`);
    assert.strictEqual(fs.existsSync(path.join(root, ".refute", "bin", "refute")), false, "sync installed unsafe tar member");
  }
}

function testRejectsLinkTarMembers() {
  for (const typeflag of ["1", "2"]) {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), "refute-npm-link-tar-"));
    const archive = writeArchive(root, { typeflag, linkname: "outside-refute", body: Buffer.alloc(0) });
    const digest = sha256(archive);
    writeLock(root, archive, digest);

    const result = runSync(root);
    assert.notStrictEqual(result.status, 0, `sync unexpectedly accepted tar link member type ${typeflag}`);
    assert.strictEqual(fs.existsSync(path.join(root, ".refute", "bin", "refute")), false, "sync installed link tar member");
  }
}

function testRejectsSymlinkedBinRoot() {
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

  const result = runSync(root);
  assert.notStrictEqual(result.status, 0, "sync unexpectedly accepted symlinked .refute/bin");
  assert.deepStrictEqual(fs.readdirSync(outside), [], "sync wrote through symlinked .refute/bin");
}

function testReplacesSymlinkedActiveFiles() {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "refute-npm-active-symlink-"));
  const archive = writeArchive(root);
  const digest = sha256(archive);
  writeLock(root, archive, digest);
  const binRoot = path.join(root, ".refute", "bin");
  const outside = path.join(root, "outside-bin");
  fs.mkdirSync(path.join(root, ".refute", "cache"), { recursive: true });
  fs.mkdirSync(binRoot);
  fs.mkdirSync(outside);
  const links = new Map([
    [path.join(binRoot, "refute"), path.join(outside, "refute")],
    [path.join(binRoot, "refute.artifact-sha256"), path.join(outside, "artifact")],
    [path.join(binRoot, "refute.binary-sha256"), path.join(outside, "binary")],
  ]);
  try {
    for (const [link, target] of links) {
      fs.writeFileSync(target, "outside\n");
      fs.symlinkSync(target, link);
    }
  } catch (err) {
    console.log(`symlinked active files test skipped: ${err.message}`);
    return;
  }

  const result = runSync(root);
  assert.strictEqual(result.status, 0, result.stderr || result.stdout);
  for (const [link, target] of links) {
    assert.strictEqual(fs.lstatSync(link).isSymbolicLink(), false, `${link} is still a symlink`);
    assert.strictEqual(fs.readFileSync(target, "utf8"), "outside\n", `${target} was overwritten`);
  }
  assert.match(fs.readFileSync(path.join(binRoot, "refute"), "utf8"), /synced/);
  assert.strictEqual(fs.readFileSync(path.join(binRoot, "refute.artifact-sha256"), "utf8").trim(), digest);
}

function runSync(root) {
  return spawnSync(process.execPath, [path.join(__dirname, "..", "bin", "refute-tool.js"), "sync"], {
    cwd: root,
    encoding: "utf8",
  });
}

function writeArchive(root, options = {}) {
  const name = options.name || "refute";
  const body = options.body === undefined ? Buffer.from("#!/bin/sh\necho synced\n") : Buffer.from(options.body);
  const typeflag = options.typeflag || "0";
  const linkname = options.linkname || "";
  const bodyLength = typeflag === "0" ? body.length : 0;
  const storedBody = body.subarray(0, bodyLength);
  const padding = Buffer.alloc((512 - (storedBody.length % 512)) % 512, 0);
  const archive = path.join(root, "archive.tar.gz");
  const tar = Buffer.concat([tarHeader(name, bodyLength, typeflag, linkname), storedBody, padding, Buffer.alloc(1024, 0)]);
  fs.writeFileSync(archive, zlib.gzipSync(tar));
  return archive;
}

function tarHeader(name, bodyLength, typeflag, linkname) {
  const header = Buffer.alloc(512, 0);
  header.write(name, 0, 100, "utf8");
  header.write("0000755\0", 100, 8, "ascii");
  header.write("0000000\0", 108, 8, "ascii");
  header.write("0000000\0", 116, 8, "ascii");
  header.write(bodyLength.toString(8).padStart(11, "0") + "\0", 124, 12, "ascii");
  header.write(Math.floor(Date.now() / 1000).toString(8).padStart(11, "0") + "\0", 136, 12, "ascii");
  header.fill(" ", 148, 156);
  header.write(typeflag, 156, 1, "ascii");
  header.write(linkname, 157, 100, "utf8");
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
