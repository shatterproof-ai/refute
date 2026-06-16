// Contract tests: drive rename.cjs against the shared golden fixtures under
// testdata/adapter-contracts/tsmorph/ so the Node adapter and the Go driver are
// pinned to the same wire shape (issue #76). The Go side consumes the same
// files in internal/backend/tsmorph/contract_test.go.
const assert = require("node:assert/strict");
const { spawn } = require("node:child_process");
const fs = require("node:fs");
const fsp = require("node:fs/promises");
const os = require("node:os");
const path = require("node:path");
const test = require("node:test");

const adapterPath = path.join(__dirname, "rename.cjs");
const fixturesDir = path.join(__dirname, "..", "..", "testdata", "adapter-contracts", "tsmorph");
// Placeholder workspace root used in the golden fixtures.
const PLACEHOLDER_ROOT = "/workspace";
// The fixtures describe renaming greet -> welcome in this single file.
const SOURCE_BEFORE = 'export function greet() {\n  return "hello";\n}\n';

function loadFixture(name) {
  return JSON.parse(fs.readFileSync(path.join(fixturesDir, name), "utf8"));
}

// runAdapter feeds one JSON request to rename.cjs and resolves its parsed
// stdout response (or rejects with the stderr message on non-zero exit).
function runAdapter(request) {
  return new Promise((resolve, reject) => {
    const child = spawn(process.execPath, [adapterPath], { stdio: ["pipe", "pipe", "pipe"] });
    let stdout = "";
    let stderr = "";
    child.stdout.setEncoding("utf8");
    child.stderr.setEncoding("utf8");
    child.stdout.on("data", chunk => { stdout += chunk; });
    child.stderr.on("data", chunk => { stderr += chunk; });
    child.on("error", reject);
    child.on("close", code => {
      if (code !== 0) {
        reject(new Error(`adapter exited ${code}: ${stderr}`));
        return;
      }
      resolve(JSON.parse(stdout));
    });
    child.stdin.end(JSON.stringify(request));
  });
}

// rewriteToPlaceholder maps the temp workspace's absolute paths back to the
// fixture's "/workspace" placeholder so the response can be compared verbatim.
function rewriteToPlaceholder(value, realRoot) {
  return JSON.parse(JSON.stringify(value).split(realRoot).join(PLACEHOLDER_ROOT));
}

// materialize realises a fixture request against a fresh temp workspace,
// returning the request with real paths and the temp root.
async function materialize(t, fixtureRequest) {
  const root = await fsp.mkdtemp(path.join(os.tmpdir(), "refute-tsmorph-contract-"));
  t.after(() => fsp.rm(root, { force: true, recursive: true }));
  await fsp.writeFile(path.join(root, "greeter.ts"), SOURCE_BEFORE);
  await fsp.writeFile(path.join(root, "tsconfig.json"), '{"compilerOptions":{"strict":true},"include":["*.ts"]}\n');
  const realFile = path.join(root, "greeter.ts");
  const request = { ...fixtureRequest, workspaceRoot: root };
  if (fixtureRequest.file) {
    request.file = realFile;
  }
  return { root, request };
}

test("rename: adapter output matches the golden response fixture", async t => {
  const { root, request } = await materialize(t, loadFixture("rename.request.json"));
  const response = await runAdapter(request);
  assert.deepEqual(rewriteToPlaceholder(response, root), loadFixture("rename.response.json"));
});

test("findSymbol: adapter output matches the golden response fixture", async t => {
  const { root, request } = await materialize(t, loadFixture("find-symbol.request.json"));
  const response = await runAdapter(request);
  assert.deepEqual(rewriteToPlaceholder(response, root), loadFixture("find-symbol.response.json"));
});

test("a skewed protocol version is rejected, not executed", async t => {
  const { request } = await materialize(t, loadFixture("rename.request.json"));
  await assert.rejects(
    runAdapter({ ...request, protocolVersion: 999 }),
    /unsupported tsmorph protocol version: got 999, want 1/,
  );
});
