#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE' >&2
usage: scripts/release.sh v0.1.0

Builds refute release archives for linux and macOS on amd64 and arm64.

Environment:
  DIST_DIR    output directory, defaults to dist
  BUILD_DATE  RFC3339 UTC build date override
USAGE
}

version="${1:-${VERSION:-}}"
if [[ -z "${version}" ]]; then
  usage
  exit 2
fi

if [[ ! "${version}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$ ]]; then
  echo "release version must look like v0.1.0, got: ${version}" >&2
  exit 2
fi

root="$(git rev-parse --show-toplevel)"
cd "${root}"

commit="$(git rev-parse --short HEAD)"
build_date="${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
dist_dir="${DIST_DIR:-dist}"
module="github.com/shatterproof-ai/refute/internal/cli"
ldflags="-s -w -X ${module}.Version=${version} -X ${module}.Commit=${commit} -X ${module}.BuildDate=${build_date}"

case "${dist_dir}" in
  "" | "." | ".." | "../"* | "/" | "${root}" | "${root}/")
    echo "refusing unsafe DIST_DIR: ${dist_dir}" >&2
    exit 2
    ;;
esac

rm -rf "${dist_dir}"
mkdir -p "${dist_dir}"

targets=(
  "linux/amd64"
  "linux/arm64"
  "darwin/amd64"
  "darwin/arm64"
)

for target in "${targets[@]}"; do
  goos="${target%/*}"
  goarch="${target#*/}"
  name="refute_${version}_${goos}_${goarch}"
  staging="${dist_dir}/${name}"
  archive="${dist_dir}/${name}.tar.gz"

  mkdir -p "${staging}"
  CGO_ENABLED=0 GOOS="${goos}" GOARCH="${goarch}" go build \
    -trimpath \
    -ldflags "${ldflags}" \
    -o "${staging}/refute" \
    ./cmd/refute

  tar -C "${staging}" -czf "${archive}" refute
  rm -rf "${staging}"
done

(
  cd "${dist_dir}"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum ./*.tar.gz > checksums.txt
  else
    shasum -a 256 ./*.tar.gz > checksums.txt
  fi
)

cat > "${dist_dir}/release-notes.md" <<EOF
# refute ${version}

CLI-only dogfood release.

Artifacts cover linux and macOS on amd64 and arm64. Checksums are published in \`checksums.txt\`.

Support scope:
- Go/gopls is the supported v0.1 path.
- Rust, TypeScript, and JavaScript remain experimental.
- Java and Kotlin are not claimed for v0.1.
- Python is planned.
EOF

echo "Built ${version} artifacts in ${dist_dir}"
