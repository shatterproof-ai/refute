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
