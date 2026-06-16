#!/usr/bin/env bash
# Pre-release verification harness (issue #97).
#
# Runs the full release-candidate check suite in one documented step:
# static analysis, formatting, vulnerability scan, unit tests, a build,
# a built-binary smoke test, and the optional integration and cross-shim
# conformance suites. It prints the environment and backend versions up
# front and marks every optional backend that is absent as SKIP (loudly,
# never silently).
#
# Exit codes distinguish the three outcomes the acceptance criteria require:
#   0  every required check passed (skips are allowed)
#   1  at least one required check FAILED
#   2  the environment is unsupported (no Go toolchain, not a git checkout)
#
# A skipped optional backend never turns a passing run into a failure; it is
# reported so the operator can decide whether the gap matters for the release.
set -euo pipefail

usage() {
  cat <<'USAGE'
usage: scripts/verify.sh [--no-integration] [--no-conformance]

Verify a release candidate with one command. Runs, in order:
  go vet, gofmt, govulncheck, unit tests, build, binary smoke test,
  integration tests, cross-shim conformance, docs link check.

Options:
  --no-integration   skip the tagged integration suite
  --no-conformance   skip the cross-shim conformance harness
  -h, --help         show this help

Environment:
  REFUTE_VERIFY_NO_INTEGRATION=1   same as --no-integration
  REFUTE_VERIFY_NO_CONFORMANCE=1   same as --no-conformance
USAGE
}

run_integration=1
run_conformance=1
[[ "${REFUTE_VERIFY_NO_INTEGRATION:-}" == "1" ]] && run_integration=0
[[ "${REFUTE_VERIFY_NO_CONFORMANCE:-}" == "1" ]] && run_conformance=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --no-integration) run_integration=0 ;;
    --no-conformance) run_conformance=0 ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      echo "verify: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
  shift
done

# Unsupported environment: the suite cannot meaningfully run without the Go
# toolchain or outside a git checkout. Exit 2 keeps this distinct from a real
# check failure (exit 1).
if ! command -v go >/dev/null 2>&1; then
  echo "verify: unsupported environment: go toolchain not found on PATH" >&2
  exit 2
fi
if ! repo_root="$(git rev-parse --show-toplevel 2>/dev/null)"; then
  echo "verify: unsupported environment: not inside a git checkout" >&2
  exit 2
fi
cd "${repo_root}"

pass=0
fail=0
skip=0
failed_checks=()

hr() { printf '%s\n' "------------------------------------------------------------"; }
note() { printf '%s\n' "$*"; }

# probe <label> <command...> — print "label: <version>" or a SKIP line when the
# tool is absent. Used only for the informational version banner; absence here
# does not count against the run.
probe() {
  local label="$1"
  shift
  local bin="$1"
  if command -v "${bin}" >/dev/null 2>&1; then
    local out
    # Capture the full output first; taking the first line through a pipe under
    # `set -o pipefail` would surface a SIGPIPE from the producer as a failure.
    if out="$("$@" 2>&1)"; then
      printf '  %-16s %s\n' "${label}:" "$(printf '%s\n' "${out}" | head -n 1)"
    else
      printf '  %-16s %s\n' "${label}:" "(present; version probe failed)"
    fi
  else
    printf '  %-16s %s\n' "${label}:" "not found (optional backend — checks will SKIP)"
  fi
}

# run_check <name> <command...> — run a required check and record PASS/FAIL.
run_check() {
  local name="$1"
  shift
  local out status
  hr
  note "RUN  ${name}"
  out="$("$@" 2>&1)" && status=0 || status=$?
  if [[ ${status} -eq 0 ]]; then
    note "PASS ${name}"
    pass=$((pass + 1))
  else
    note "FAIL ${name} (exit ${status})"
    printf '%s\n' "${out}"
    fail=$((fail + 1))
    failed_checks+=("${name}")
  fi
}

skip_check() {
  # skip_check <name> <reason>
  hr
  note "SKIP ${1}: ${2}"
  skip=$((skip + 1))
}

# ---------------------------------------------------------------------------
# Environment + backend version banner.
# ---------------------------------------------------------------------------
hr
note "refute pre-release verification"
hr
note "Environment"
printf '  %-16s %s\n' "go:" "$(go version)"
printf '  %-16s %s/%s\n' "platform:" "$(go env GOOS)" "$(go env GOARCH)"
printf '  %-16s %s\n' "commit:" "$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
printf '  %-16s %s\n' "branch:" "$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo unknown)"
note ""
note "Backend and tool versions"
probe "gopls" gopls version
probe "rust-analyzer" rust-analyzer --version
probe "node" node --version
probe "cargo" cargo --version
probe "java" java -version
probe "python3" python3 --version
probe "govulncheck" govulncheck -version

# ---------------------------------------------------------------------------
# Required checks.
# ---------------------------------------------------------------------------
run_check "go vet" go vet ./...

# gofmt -l prints offenders; an empty list means formatted. Wrap so a non-empty
# list becomes a non-zero exit that run_check records as FAIL.
gofmt_check() {
  local offenders
  offenders="$(gofmt -l .)"
  if [[ -n "${offenders}" ]]; then
    echo "unformatted files:"
    echo "${offenders}"
    return 1
  fi
}
run_check "gofmt" gofmt_check

if command -v govulncheck >/dev/null 2>&1; then
  run_check "govulncheck" govulncheck ./...
else
  skip_check "govulncheck" "govulncheck not on PATH (go install golang.org/x/vuln/cmd/govulncheck@latest)"
fi

run_check "unit tests" go test ./...

# Build check + smoke test of the built binary. The binary is the release
# artifact, so running it is the smallest real smoke test.
build_dir="$(mktemp -d)"
trap 'rm -rf "${build_dir}"' EXIT
refute_bin="${build_dir}/refute"
run_check "build (cmd/refute)" go build -buildvcs=false -o "${refute_bin}" ./cmd/refute

if [[ -x "${refute_bin}" ]]; then
  smoke() {
    "${refute_bin}" version >/dev/null
    "${refute_bin}" --help >/dev/null
  }
  run_check "smoke (built binary)" smoke
else
  skip_check "smoke (built binary)" "build did not produce a binary"
fi

# Integration + corpus coverage. The tagged integration suite drives the real
# language-server and rewrite backends against the testdata corpus; individual
# tests self-skip when their backend is absent. We gate the whole suite on the
# supported Go backend (gopls) so a machine with no backends reports SKIP for
# the category rather than a hollow pass.
if [[ ${run_integration} -eq 0 ]]; then
  skip_check "integration tests" "disabled via --no-integration"
elif command -v gopls >/dev/null 2>&1; then
  run_check "integration tests" go test -tags integration ./internal/...
else
  skip_check "integration tests" "gopls not on PATH; optional backends unavailable"
fi

# Cross-shim conformance harness. It self-skips per-toolchain and exits non-zero
# only on a real mismatch.
if [[ ${run_conformance} -eq 0 ]]; then
  skip_check "shim conformance" "disabled via --no-conformance"
elif [[ -x scripts/shim-conformance.sh ]]; then
  run_check "shim conformance" scripts/shim-conformance.sh
else
  skip_check "shim conformance" "scripts/shim-conformance.sh not found"
fi

# Docs check: every relative link in the documentation index must resolve to a
# file that exists, so a release never ships a dangling docs index.
docs_link_check() {
  local index="docs/README.md"
  if [[ ! -f "${index}" ]]; then
    echo "missing ${index}"
    return 1
  fi
  local broken=0 target resolved
  while IFS= read -r target; do
    # Skip absolute URLs and in-page anchors.
    case "${target}" in
      http://* | https://* | "#"* | "") continue ;;
    esac
    target="${target%%#*}"
    resolved="docs/${target}"
    if [[ ! -e "${resolved}" ]]; then
      echo "broken docs link in ${index}: ${target}"
      broken=$((broken + 1))
    fi
  done < <(grep -oE '\]\([^)]+\)' "${index}" | sed -E 's/^\]\(//; s/\)$//')
  [[ ${broken} -eq 0 ]]
}
run_check "docs links" docs_link_check

# ---------------------------------------------------------------------------
# Summary.
# ---------------------------------------------------------------------------
hr
note "verification summary: ${pass} passed, ${fail} failed, ${skip} skipped"
if [[ ${fail} -ne 0 ]]; then
  note "failed checks: ${failed_checks[*]}"
  note "result: FAIL"
  exit 1
fi
if [[ ${skip} -gt 0 ]]; then
  note "result: PASS (with ${skip} skipped optional check(s))"
else
  note "result: PASS"
fi
