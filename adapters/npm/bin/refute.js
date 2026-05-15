#!/usr/bin/env node
require("./refute-tool").main(["run", "--", ...process.argv.slice(2)]);
