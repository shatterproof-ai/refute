#!/usr/bin/env node
// Propagate the delegated exit code: main() returns refute's exit status
// (including non-zero signal deaths via refute-tool's run()), so the `refute`
// bin must exit with it instead of always succeeding.
process.exit(require("./refute-tool").main(["run", "--", ...process.argv.slice(2)]));
