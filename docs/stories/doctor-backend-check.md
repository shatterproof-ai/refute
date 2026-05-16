---
schema_version: 1
title: Check backend readiness with doctor
slug: doctor-backend-check
status: active
authority: observed
change_resistance: low
tests_applicable: true
locked_sections: []
---

# Check backend readiness with doctor

## Intent
The user runs `refute doctor` to learn which language backends are installed and ready on the current host.

## Story
Before using `refute` refactoring commands, a developer or CI script needs to know whether the required language servers are present. `refute doctor` probes the `PATH` for each supported backend binary, checks for adapter dependencies (such as the tsmorph Node adapter), and reports the status of each backend along with install hints for anything missing. The command runs without modifying any source files and exits cleanly regardless of what is missing, making it safe to run in any environment.

## Expected Behavior
Running `refute doctor` prints a table of backends with status labels (`ok`, `missing`, `experimental`, `planned`, `not-claimed`), the binary path when found, the operations each backend supports, and install hints for missing dependencies. With `--json`, the output is a structured `DoctorReport` JSON object with a `schemaVersion` field, a `command` field, and a `backends` array. The command exits `0` in both cases.

## Boundaries
This story does not validate that the detected binaries actually work correctly for refactoring; it only checks presence and reachability. It does not install missing binaries. It does not reflect language support added after the binary shipped.

## Auditable Claims
- `refute doctor` exits 0 whether all backends are present or all are missing.
- `--json` emits a JSON object with a `backends` array where each entry has at minimum `language`, `backend`, and `status` fields.
- Status values are drawn from the defined set: `ok`, `missing`, `experimental`, `planned`, `not-claimed`.
- When a backend binary is absent, `installHint` is populated in the JSON output.

## Evidence
### Tests
- `internal/cli/doctor_test.go`
- `internal/cli/doctor_tsmorph_test.go`
### Surface
- `cli: refute doctor`
- `cli: refute doctor --json`
### Docs
- `docs/support-matrix.md`
