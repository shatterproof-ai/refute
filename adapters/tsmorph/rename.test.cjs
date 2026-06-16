const assert = require("node:assert/strict");
const { spawn } = require("node:child_process");
const fs = require("node:fs/promises");
const os = require("node:os");
const path = require("node:path");
const test = require("node:test");

const adapterPath = path.join(__dirname, "rename.cjs");

async function runAdapter(request) {
  return new Promise((resolve, reject) => {
    const child = spawn(process.execPath, [adapterPath], {
      stdio: ["pipe", "pipe", "pipe"],
    });

    let stdout = "";
    let stderr = "";
    child.stdout.setEncoding("utf8");
    child.stderr.setEncoding("utf8");
    child.stdout.on("data", chunk => {
      stdout += chunk;
    });
    child.stderr.on("data", chunk => {
      stderr += chunk;
    });
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

test("unknown operations fail with a clear error", async t => {
  const workspaceRoot = await fs.mkdtemp(path.join(os.tmpdir(), "refute-tsmorph-"));
  t.after(() => fs.rm(workspaceRoot, { force: true, recursive: true }));

  const file = path.join(workspaceRoot, "sample.ts");
  await fs.writeFile(file, "export const message = 'hello';\n");

  await assert.rejects(
    runAdapter({
      operation: "deleteEverything",
      workspaceRoot,
      file,
      line: 1,
      column: 14,
      newName: "renamed",
    }),
    /unsupported operation: deleteEverything/,
  );
});

test("findSymbol returns a matching function candidate", async t => {
  const workspaceRoot = await fs.mkdtemp(path.join(os.tmpdir(), "refute-tsmorph-"));
  t.after(() => fs.rm(workspaceRoot, { force: true, recursive: true }));

  const file = path.join(workspaceRoot, "sample.ts");
  await fs.writeFile(file, "export function greet() {\n  return 'hello';\n}\n");

  const response = await runAdapter({
    operation: "findSymbol",
    workspaceRoot,
    file,
    qualifiedName: "sample:greet",
    kind: "function",
  });

  assert.deepEqual(response.candidates, [
    {
      file,
      line: 1,
      column: 17,
      name: "greet",
      kind: "function",
    },
  ]);
});

test("findSymbol reports UTF-16 columns for non-ASCII lines", async t => {
  const workspaceRoot = await fs.mkdtemp(path.join(os.tmpdir(), "refute-tsmorph-"));
  t.after(() => fs.rm(workspaceRoot, { force: true, recursive: true }));

  const file = path.join(workspaceRoot, "sample.ts");
  const line = 'const label = "é𝄞"; export function greet() { return "hello"; }';
  await fs.writeFile(file, `${line}\n`);

  const response = await runAdapter({
    operation: "findSymbol",
    workspaceRoot,
    file,
    qualifiedName: "sample:greet",
    kind: "function",
  });

  assert.deepEqual(response.candidates, [
    {
      file,
      line: 1,
      column: line.indexOf("greet") + 1,
      name: "greet",
      kind: "function",
    },
  ]);
});

test("rename returns full-file edits for changed files", async t => {
  const workspaceRoot = await fs.mkdtemp(path.join(os.tmpdir(), "refute-tsmorph-"));
  t.after(() => fs.rm(workspaceRoot, { force: true, recursive: true }));

  const file = path.join(workspaceRoot, "sample.ts");
  await fs.writeFile(
    file,
    "export function greet() {\n  return 'hello';\n}\n\nexport const message = greet();\n",
  );

  const response = await runAdapter({
    operation: "rename",
    workspaceRoot,
    file,
    line: 1,
    column: 17,
    newName: "salute",
  });

  assert.equal(response.fileEdits.length, 1);
  assert.equal(response.fileEdits[0].path, file);
  assert.equal(
    response.fileEdits[0].edits[0].newText,
    "export function salute() {\n  return 'hello';\n}\n\nexport const message = salute();\n",
  );
});

test("rename accepts UTF-16 columns on non-ASCII lines", async t => {
  const workspaceRoot = await fs.mkdtemp(path.join(os.tmpdir(), "refute-tsmorph-"));
  t.after(() => fs.rm(workspaceRoot, { force: true, recursive: true }));

  const file = path.join(workspaceRoot, "sample.ts");
  const line = 'const label = "é𝄞"; export function greet() { return "hello"; }';
  await fs.writeFile(file, line);

  const response = await runAdapter({
    operation: "rename",
    workspaceRoot,
    file,
    line: 1,
    column: line.indexOf("greet") + 1,
    newName: "welcome",
  });

  assert.equal(response.fileEdits.length, 1);
  assert.equal(response.fileEdits[0].path, file);
  assert.equal(
    response.fileEdits[0].edits[0].newText,
    'const label = "é𝄞"; export function welcome() { return "hello"; }',
  );
});

test("fallback project excludes node_modules sources from rename edits", async t => {
  const workspaceRoot = await fs.mkdtemp(path.join(os.tmpdir(), "refute-tsmorph-"));
  t.after(() => fs.rm(workspaceRoot, { force: true, recursive: true }));

  const appDir = path.join(workspaceRoot, "src");
  const dependencyDir = path.join(workspaceRoot, "node_modules", "pkg");
  await fs.mkdir(appDir, { recursive: true });
  await fs.mkdir(dependencyDir, { recursive: true });

  const appFile = path.join(appDir, "app.ts");
  const dependencyFile = path.join(dependencyDir, "index.ts");
  await fs.writeFile(
    appFile,
    "import { greet } from '../node_modules/pkg/index';\n\nexport const message = greet();\n",
  );
  await fs.writeFile(dependencyFile, "export function greet() {\n  return 'dependency';\n}\n");

  const response = await runAdapter({
    operation: "rename",
    workspaceRoot,
    file: appFile,
    line: 1,
    column: 10,
    newName: "salute",
  });

  assert.ok(
    response.fileEdits.every(fileEdit => !fileEdit.path.includes(`${path.sep}node_modules${path.sep}`)),
    "dependency sources must not be edited by fallback glob loading",
  );
});

test("project discovery loads nested tsconfig files", async t => {
  const workspaceRoot = await fs.mkdtemp(path.join(os.tmpdir(), "refute-tsmorph-"));
  t.after(() => fs.rm(workspaceRoot, { force: true, recursive: true }));

  await fs.mkdir(path.join(workspaceRoot, "src"), { recursive: true });
  await fs.mkdir(path.join(workspaceRoot, "packages", "pkg", "src"), { recursive: true });
  await fs.writeFile(
    path.join(workspaceRoot, "tsconfig.json"),
    JSON.stringify({ include: ["src/**/*.ts"] }),
  );
  await fs.writeFile(
    path.join(workspaceRoot, "packages", "pkg", "tsconfig.json"),
    JSON.stringify({ include: ["src/**/*.ts"] }),
  );

  const nestedFile = path.join(workspaceRoot, "packages", "pkg", "src", "nested.ts");
  await fs.writeFile(path.join(workspaceRoot, "src", "root.ts"), "export const root = true;\n");
  await fs.writeFile(nestedFile, "export function nested() {\n  return 'nested';\n}\n");

  const response = await runAdapter({
    operation: "findSymbol",
    workspaceRoot,
    file: nestedFile,
    qualifiedName: "packages/pkg/src/nested:nested",
    kind: "function",
  });

  assert.deepEqual(response.candidates, [
    {
      file: nestedFile,
      line: 1,
      column: 17,
      name: "nested",
      kind: "function",
    },
  ]);
});
