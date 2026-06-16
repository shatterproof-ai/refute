#!/usr/bin/env bash
# Cross-shim conformance harness for the registryless toolchain shims.
#
# Drives every package-manager shim (Go refute-tool, npm, python, cargo, jvm)
# against ONE shared file:// artifact + lockfile and asserts each installs an
# identical .refute/bin/refute. This is the guard against the implementations
# drifting again (issue #75).
#
# A shim whose toolchain is absent is reported as SKIP (loudly, never silently)
# rather than failing, so the harness is usable locally; CI installs every
# toolchain so nothing is skipped there.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
work="$(mktemp -d)"
tools="${work}/tools"
mkdir -p "${tools}"
trap 'rm -rf "${work}"' EXIT

marker="refute-conformance-fake"
pass=0
fail=0
skip=0

note() { printf '%s\n' "$*"; }

# Build the canonical Go refute-tool and put it on PATH; cargo/jvm sync delegate
# to it, and it is itself one of the conformance subjects.
note "building Go refute-tool"
( cd "${repo_root}" && go build -o "${tools}/refute-tool" ./cmd/refute-tool )
export PATH="${tools}:${PATH}"

# Platform/arch names come straight from the Go runtime, which is exactly the
# lockfile schema convention the shims must match.
platform="$(cd "${repo_root}" && go env GOOS)"
arch="$(cd "${repo_root}" && go env GOARCH)"
note "platform/arch: ${platform}/${arch}"

# Build the shared fixture: a fake refute binary archived with a member named
# "refute", plus its sha256.
fixture="${work}/fixture"
mkdir -p "${fixture}"
# The fake binary echoes a marker normally, but exits 7 for the "boom" argument
# so the run-path cases can assert non-zero exit propagation. $1 is literal: it
# belongs to the generated script, not this one.
# shellcheck disable=SC2016
printf '#!/bin/sh\nif [ "$1" = "boom" ]; then exit 7; fi\necho %s\n' "${marker}" > "${fixture}/refute"
chmod +x "${fixture}/refute"
archive="${work}/refute_${platform}_${arch}.tar.gz"
tar -C "${fixture}" -czf "${archive}" refute
if command -v sha256sum >/dev/null 2>&1; then
  digest="$(sha256sum "${archive}" | cut -d' ' -f1)"
else
  digest="$(shasum -a 256 "${archive}" | cut -d' ' -f1)"
fi

write_lock() {
  # write_lock <dir>
  cat > "${1}/refute.lock.json" <<JSON
{
  "version": "v9.9.9-conformance",
  "manifest_url": "file://${work}/manifest.json",
  "artifacts": [
    {
      "platform": "${platform}",
      "architecture": "${arch}",
      "url": "file://${archive}",
      "sha256": "${digest}",
      "filename": "refute_${platform}_${arch}.tar.gz"
    }
  ]
}
JSON
}

run_case() {
  # run_case <name> <command...>
  local name="${1}"
  shift
  local proj="${work}/proj-${name}"
  mkdir -p "${proj}"
  write_lock "${proj}"
  local out status
  out="$(cd "${proj}" && "$@" 2>&1)" && status=0 || status=$?
  local installed="${proj}/.refute/bin/refute"
  if [ "${status}" -ne 0 ]; then
    note "FAIL ${name}: sync exited ${status}"
    note "${out}"
    fail=$((fail + 1))
    return
  fi
  if [ ! -f "${installed}" ]; then
    note "FAIL ${name}: .refute/bin/refute not installed"
    fail=$((fail + 1))
    return
  fi
  if ! "${installed}" | grep -q "${marker}"; then
    note "FAIL ${name}: installed binary did not run the fixture"
    fail=$((fail + 1))
    return
  fi
  note "PASS ${name}"
  pass=$((pass + 1))
}

run_exit_case() {
  # run_exit_case <name> <run-command... that execs the installed binary with "boom">
  # Asserts the shim's run path resolves the walked-up binary and propagates its
  # non-zero (7) exit instead of collapsing to success.
  local name="${1}"
  shift
  local proj="${work}/proj-${name}"
  local status
  ( cd "${proj}" && "$@" >/dev/null 2>&1 ) && status=0 || status=$?
  if [ "${status}" -ne 7 ]; then
    note "FAIL ${name}-run: expected exit 7 from run path, got ${status}"
    fail=$((fail + 1))
    return
  fi
  note "PASS ${name}-run"
  pass=$((pass + 1))
}

# Go refute-tool (canonical).
run_case "go-refute-tool" refute-tool sync
run_exit_case "go-refute-tool" refute-tool run -- boom

# npm shim. The run case drives bin/refute.js to also cover its exit propagation.
if command -v node >/dev/null 2>&1; then
  run_case "npm" node "${repo_root}/adapters/npm/bin/refute-tool.js" sync
  run_exit_case "npm" node "${repo_root}/adapters/npm/bin/refute.js" boom
else
  note "SKIP npm: node not found"
  skip=$((skip + 1))
fi

# python shim.
if command -v python3 >/dev/null 2>&1; then
  run_case "python" python3 "${repo_root}/adapters/python/src/refute_tool/cli.py" sync
  run_exit_case "python" python3 "${repo_root}/adapters/python/src/refute_tool/cli.py" run -- boom
else
  note "SKIP python: python3 not found"
  skip=$((skip + 1))
fi

# cargo shim (sync delegates to refute-tool on PATH; run execs the binary).
if command -v cargo >/dev/null 2>&1; then
  note "building cargo-refute"
  ( cd "${repo_root}/adapters/cargo" && cargo build --quiet )
  run_case "cargo" "${repo_root}/adapters/cargo/target/debug/cargo-refute" sync
  run_exit_case "cargo" "${repo_root}/adapters/cargo/target/debug/cargo-refute" -- boom
else
  note "SKIP cargo: cargo not found"
  skip=$((skip + 1))
fi

# jvm shim (sync delegates to refute-tool on PATH; run execs the binary).
if command -v javac >/dev/null 2>&1 && command -v java >/dev/null 2>&1; then
  note "compiling jvm shim"
  classes="${work}/jvm-classes"
  mkdir -p "${classes}"
  javac -d "${classes}" "${repo_root}/adapters/jvm/src/main/java/ai/shatterproof/refute/RefuteTool.java"
  run_case "jvm" java -cp "${classes}" ai.shatterproof.refute.RefuteTool sync
  run_exit_case "jvm" java -cp "${classes}" ai.shatterproof.refute.RefuteTool boom
else
  note "SKIP jvm: javac/java not found"
  skip=$((skip + 1))
fi

note ""
note "conformance summary: ${pass} passed, ${fail} failed, ${skip} skipped"
if [ "${fail}" -ne 0 ]; then
  exit 1
fi
