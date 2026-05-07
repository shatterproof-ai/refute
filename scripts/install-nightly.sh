#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE' >&2
usage: scripts/install-nightly.sh [--project DIR] [--install-dir DIR] [--repo OWNER/REPO]
                                [--archive FILE]

Install the latest unofficial refute nightly into a project-local tool
directory. The default install path is:

  <project>/.agents/bin/refute

Options:
  --project DIR      project root, defaults to the current directory
  --install-dir DIR  destination directory, defaults to <project>/.agents/bin
  --repo OWNER/REPO  GitHub repository, defaults to shatterproof-ai/refute
  --archive FILE     install from a local release archive instead of GitHub
  -h, --help         show this help

GitHub installs require the GitHub CLI (`gh`) with access to the repository.
USAGE
}

project_dir="$(pwd)"
install_dir=""
repo="shatterproof-ai/refute"
archive=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --project)
      project_dir="${2:-}"
      shift 2
      ;;
    --install-dir)
      install_dir="${2:-}"
      shift 2
      ;;
    --repo)
      repo="${2:-}"
      shift 2
      ;;
    --archive)
      archive="${2:-}"
      shift 2
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage
      exit 2
      ;;
  esac
done

if [[ -z "${project_dir}" ]]; then
  echo "--project requires a directory" >&2
  exit 2
fi

if [[ -z "${install_dir}" ]]; then
  install_dir="${project_dir}/.agents/bin"
fi

case "$(uname -s)" in
  Linux)
    os="linux"
    ;;
  Darwin)
    os="darwin"
    ;;
  *)
    echo "unsupported OS: $(uname -s)" >&2
    exit 2
    ;;
esac

case "$(uname -m)" in
  x86_64 | amd64)
    arch="amd64"
    ;;
  arm64 | aarch64)
    arch="arm64"
    ;;
  *)
    echo "unsupported architecture: $(uname -m)" >&2
    exit 2
    ;;
esac

if [[ -z "${archive}" ]] && ! command -v gh >/dev/null 2>&1; then
  echo "missing dependency: gh" >&2
  exit 127
fi

tmp="$(mktemp -d)"
cleanup() {
  rm -rf "${tmp}"
}
trap cleanup EXIT

pattern="refute_*_${os}_${arch}.tar.gz"
mkdir -p "${install_dir}"

if [[ -n "${archive}" ]]; then
  if [[ ! -f "${archive}" ]]; then
    echo "archive does not exist: ${archive}" >&2
    exit 1
  fi
  cp "${archive}" "${tmp}/"
else
  gh release download nightly \
    --repo "${repo}" \
    --pattern "${pattern}" \
    --dir "${tmp}"
fi

archive_count="$(find "${tmp}" -maxdepth 1 -name "${pattern}" | wc -l | tr -d ' ')"
if [[ "${archive_count}" != "1" ]]; then
  echo "expected exactly one archive matching ${pattern}, found ${archive_count}" >&2
  exit 1
fi

archive="$(find "${tmp}" -maxdepth 1 -name "${pattern}" -print -quit)"
tar -xzf "${archive}" -C "${tmp}"
install -m 0755 "${tmp}/refute" "${install_dir}/refute"

"${install_dir}/refute" version
echo "Installed refute to ${install_dir}/refute"
