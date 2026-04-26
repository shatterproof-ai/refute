#!/usr/bin/env node

const fs = require("fs");
const path = require("path");
const { Project } = require("ts-morph");

function readInput() {
  return new Promise((resolve, reject) => {
    let data = "";
    process.stdin.setEncoding("utf8");
    process.stdin.on("data", chunk => {
      data += chunk;
    });
    process.stdin.on("end", () => {
      try {
        resolve(JSON.parse(data));
      } catch (err) {
        reject(err);
      }
    });
    process.stdin.on("error", reject);
  });
}

function projectConfigPath(workspaceRoot) {
  const tsconfig = path.join(workspaceRoot, "tsconfig.json");
  if (fs.existsSync(tsconfig)) {
    return tsconfig;
  }
  const jsconfig = path.join(workspaceRoot, "jsconfig.json");
  if (fs.existsSync(jsconfig)) {
    return jsconfig;
  }
  return "";
}

function createProject(workspaceRoot) {
  const configPath = projectConfigPath(workspaceRoot);
  if (configPath !== "") {
    return new Project({ tsConfigFilePath: configPath });
  }

  const project = new Project();
  project.addSourceFilesAtPaths([
    path.join(workspaceRoot, "**/*.ts"),
    path.join(workspaceRoot, "**/*.tsx"),
    path.join(workspaceRoot, "**/*.js"),
    path.join(workspaceRoot, "**/*.jsx"),
  ]);
  return project;
}

function positionToIndex(sourceFile, line, column) {
  const text = sourceFile.getFullText();
  const lines = text.split("\n");
  if (line < 1 || line > lines.length) {
    throw new Error(`line out of range: ${line}`);
  }
  const targetLine = lines[line - 1];
  if (column < 1 || column > targetLine.length + 1) {
    throw new Error(`column out of range: ${column}`);
  }

  let pos = 0;
  for (let i = 0; i < line - 1; i++) {
    pos += lines[i].length + 1;
  }
  pos += column - 1;
  return pos;
}

function renameTargetAt(sourceFile, pos) {
  let node = sourceFile.getDescendantAtPos(pos);
  while (node) {
    if (typeof node.rename === "function") {
      return node;
    }
    node = node.getParent();
  }
  return undefined;
}

function fullFileEdit(beforeText, afterText, filePath) {
  if (beforeText === afterText) {
    return undefined;
  }
  const lines = beforeText.split("\n");
  const lastLine = lines.length - 1;
  const lastCharacter = lines[lastLine].length;
  return {
    path: filePath,
    edits: [
      {
        range: {
          start: { line: 0, character: 0 },
          end: { line: lastLine, character: lastCharacter },
        },
        newText: afterText,
      },
    ],
  };
}

async function main() {
  const req = await readInput();
  const workspaceRoot = path.resolve(req.workspaceRoot);
  const filePath = path.resolve(req.file);

  const project = createProject(workspaceRoot);
  const sourceFile = project.getSourceFile(filePath);
  if (!sourceFile) {
    throw new Error(`source file not found: ${filePath}`);
  }

  const before = new Map();
  for (const sf of project.getSourceFiles()) {
    before.set(sf.getFilePath(), sf.getFullText());
  }

  const pos = positionToIndex(sourceFile, req.line, req.column);
  const target = renameTargetAt(sourceFile, pos);
  if (!target) {
    throw new Error(`rename target not found at ${filePath}:${req.line}:${req.column}`);
  }

  target.rename(req.newName);

  const fileEdits = [];
  for (const sf of project.getSourceFiles()) {
    const edit = fullFileEdit(before.get(sf.getFilePath()) || "", sf.getFullText(), sf.getFilePath());
    if (edit) {
      fileEdits.push(edit);
    }
  }

  process.stdout.write(JSON.stringify({ fileEdits }));
}

main().catch(err => {
  process.stderr.write(String(err && err.message ? err.message : err));
  process.exit(1);
});
