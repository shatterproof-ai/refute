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

function parseQualifiedName(qualifiedName) {
  let modulePath = "";
  let remainder = qualifiedName;
  const moduleSep = qualifiedName.indexOf(":");
  if (moduleSep >= 0) {
    modulePath = qualifiedName.slice(0, moduleSep);
    remainder = qualifiedName.slice(moduleSep + 1);
  }

  let containerName = "";
  let symbolName = remainder;
  const memberSep = remainder.lastIndexOf(".");
  if (memberSep >= 0) {
    containerName = remainder.slice(0, memberSep);
    symbolName = remainder.slice(memberSep + 1);
  }

  return {
    modulePath: normalizeModulePath(modulePath),
    containerName,
    symbolName,
  };
}

function normalizeModulePath(name) {
  return String(name || "")
    .replace(/\\/g, "/")
    .replace(/^\.\//, "")
    .replace(/\.(tsx?|jsx?)$/, "");
}

function sourceFileModulePath(workspaceRoot, sourceFile) {
  return normalizeModulePath(path.relative(workspaceRoot, sourceFile.getFilePath()));
}

function matchesModulePath(workspaceRoot, sourceFile, modulePath) {
  if (!modulePath) {
    return true;
  }
  const rel = sourceFileModulePath(workspaceRoot, sourceFile);
  return rel === modulePath || rel.endsWith(`/${modulePath}`);
}

function locationForNamedNode(nameNode, name, kind) {
  const sourceFile = nameNode.getSourceFile();
  const lc = sourceFile.getLineAndColumnAtPos(nameNode.getStart());
  return {
    file: sourceFile.getFilePath(),
    line: lc.line,
    column: lc.column,
    name,
    kind,
  };
}

function pushNamedNode(candidates, namedNode, name, kind) {
  if (!namedNode) {
    return;
  }
  candidates.push(locationForNamedNode(namedNode, name, kind));
}

function collectMatches(sourceFile, query) {
  const candidates = [];

  switch (query.kind) {
    case "method":
      for (const cls of sourceFile.getClasses()) {
        if (query.containerName && cls.getName() !== query.containerName) {
          continue;
        }
        for (const method of cls.getMethods()) {
          if (method.getName() === query.symbolName) {
            pushNamedNode(candidates, method.getNameNode(), method.getName(), "method");
          }
        }
      }
      break;
    case "class":
      for (const cls of sourceFile.getClasses()) {
        if (cls.getName() === query.symbolName) {
          pushNamedNode(candidates, cls.getNameNode(), cls.getName(), "class");
        }
      }
      break;
    case "type":
      for (const iface of sourceFile.getInterfaces()) {
        if (iface.getName() === query.symbolName) {
          pushNamedNode(candidates, iface.getNameNode(), iface.getName(), "type");
        }
      }
      for (const alias of sourceFile.getTypeAliases()) {
        if (alias.getName() === query.symbolName) {
          pushNamedNode(candidates, alias.getNameNode(), alias.getName(), "type");
        }
      }
      break;
    case "variable":
      for (const decl of sourceFile.getVariableDeclarations()) {
        if (decl.getName() === query.symbolName) {
          pushNamedNode(candidates, decl.getNameNode(), decl.getName(), "variable");
        }
      }
      break;
    case "function":
      for (const fn of sourceFile.getFunctions()) {
        if (fn.getName() === query.symbolName) {
          pushNamedNode(candidates, fn.getNameNode(), fn.getName(), "function");
        }
      }
      break;
    default:
      candidates.push(...collectMatches(sourceFile, { ...query, kind: "function" }));
      candidates.push(...collectMatches(sourceFile, { ...query, kind: "class" }));
      candidates.push(...collectMatches(sourceFile, { ...query, kind: "type" }));
      candidates.push(...collectMatches(sourceFile, { ...query, kind: "variable" }));
      candidates.push(...collectMatches(sourceFile, { ...query, kind: "method" }));
      break;
  }

  return candidates;
}

function findSymbol(project, workspaceRoot, req) {
  const parsed = parseQualifiedName(req.qualifiedName || "");
  const allFiles = project.getSourceFiles().filter(sf => matchesModulePath(workspaceRoot, sf, parsed.modulePath));

  const preferredFile = req.file ? path.resolve(req.file) : "";
  const searchScopes = [];
  if (preferredFile && !parsed.modulePath) {
    const preferred = project.getSourceFile(preferredFile);
    if (preferred) {
      searchScopes.push([preferred]);
    }
  }
  searchScopes.push(allFiles);

  for (const scope of searchScopes) {
    const candidates = [];
    for (const sourceFile of scope) {
      candidates.push(...collectMatches(sourceFile, {
        kind: req.kind || "unknown",
        containerName: parsed.containerName,
        symbolName: parsed.symbolName,
      }));
    }
    if (candidates.length > 0) {
      return candidates;
    }
  }

  return [];
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
  const project = createProject(workspaceRoot);
  switch (req.operation) {
    case "findSymbol": {
      const candidates = findSymbol(project, workspaceRoot, req);
      process.stdout.write(JSON.stringify({ candidates }));
      return;
    }
    case "rename":
    default: {
      const filePath = path.resolve(req.file);
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
      return;
    }
  }
}

main().catch(err => {
  process.stderr.write(String(err && err.message ? err.message : err));
  process.exit(1);
});
