const js = require("@eslint/js");
const tsParser = require("@typescript-eslint/parser");
const tsPlugin = require("@typescript-eslint/eslint-plugin");
const sonarjs = require("eslint-plugin-sonarjs");

const nodeGlobals = {
  Buffer: "readonly",
  __dirname: "readonly",
  __filename: "readonly",
  clearTimeout: "readonly",
  console: "readonly",
  module: "readonly",
  process: "readonly",
  require: "readonly",
  setTimeout: "readonly",
};

module.exports = [
  {
    ignores: ["node_modules/**"],
  },
  js.configs.recommended,
  {
    files: ["**/*.cjs"],
    languageOptions: {
      ecmaVersion: 2022,
      globals: nodeGlobals,
      parser: tsParser,
      sourceType: "commonjs",
    },
    plugins: {
      "@typescript-eslint": tsPlugin,
      sonarjs,
    },
    rules: {
      ...tsPlugin.configs.recommended.rules,
      ...sonarjs.configs.recommended.rules,
      "@typescript-eslint/no-require-imports": "off",
    },
  },
];
