#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE' >&2
usage: scripts/release.sh v0.1.0
       scripts/release.sh nightly-20260507-abc1234-123456789

Builds refute release archives, release manifests, and registryless adapter artifacts.

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

if [[ ! "${version}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$ && ! "${version}" =~ ^nightly-[0-9]{8}-[0-9a-f]{7,40}-[0-9]+$ ]]; then
  echo "release version must look like v0.1.0 or nightly-20260507-abc1234-123456789, got: ${version}" >&2
  exit 2
fi

root="$(git rev-parse --show-toplevel)"
cd "${root}"

commit="$(git rev-parse --short HEAD)"
build_date="${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
dist_dir="${DIST_DIR:-dist}"
module="github.com/shatterproof-ai/refute/internal/cli"
ldflags="-s -w -X ${module}.Version=${version} -X ${module}.Commit=${commit} -X ${module}.BuildDate=${build_date}"
package_version="${version#v}"
python_package_version="${package_version}"
if [[ "${version}" == nightly-* ]]; then
  nightly_tail="${version#nightly-}"
  nightly_date="${nightly_tail%%-*}"
  nightly_rest="${nightly_tail#*-}"
  nightly_sha="${nightly_rest%-*}"
  nightly_run="${nightly_rest##*-}"
  package_version="0.0.0-nightly.${nightly_date}.${nightly_sha}.${nightly_run}"
  python_package_version="0.0.0.dev${nightly_date}+${nightly_sha}.${nightly_run}"
fi

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
    -buildvcs=false \
    -trimpath \
    -ldflags "${ldflags}" \
    -o "${staging}/refute" \
    ./cmd/refute

  tar -C "${staging}" -czf "${archive}" refute
  rm -rf "${staging}"
done

tool_staging="${dist_dir}/refute-tool_${version}"
mkdir -p "${tool_staging}"
CGO_ENABLED=0 GOOS="$(go env GOOS)" GOARCH="$(go env GOARCH)" go build \
  -buildvcs=false \
  -trimpath \
  -ldflags "${ldflags}" \
  -o "${tool_staging}/refute-tool" \
  ./cmd/refute-tool
tar -C "${tool_staging}" -czf "${dist_dir}/refute-tool_${version}_$(go env GOOS)_$(go env GOARCH).tar.gz" refute-tool
rm -rf "${tool_staging}"

pkg_dir="${dist_dir}/packages"
mkdir -p "${pkg_dir}"

npm_staging="${dist_dir}/npm-package"
mkdir -p "${npm_staging}"
cp -R adapters/npm/. "${npm_staging}/"
sed -i "s/\"version\": \"0.0.0-dev\"/\"version\": \"${package_version}\"/" "${npm_staging}/package.json"
tar -C "${npm_staging}" -czf "${dist_dir}/refute-tool-npm-${package_version}.tgz" .
rm -rf "${npm_staging}"

python3 - "${python_package_version}" "${dist_dir}/refute_tool-${python_package_version}-py3-none-any.whl" <<'PY'
import base64
import csv
import hashlib
import pathlib
import sys
import zipfile

version, wheel_path = sys.argv[1], pathlib.Path(sys.argv[2])
root = pathlib.Path("adapters/python/src/refute_tool")
dist = f"refute_tool-{version}.dist-info"
files = {
    "refute_tool/__init__.py": (root / "__init__.py").read_bytes(),
    "refute_tool/cli.py": (root / "cli.py").read_bytes(),
    f"{dist}/METADATA": f"Metadata-Version: 2.1\nName: refute-tool\nVersion: {version}\nSummary: Registryless Python shim for refute\nLicense: Apache-2.0\n".encode(),
    f"{dist}/WHEEL": b"Wheel-Version: 1.0\nGenerator: refute release script\nRoot-Is-Purelib: true\nTag: py3-none-any\n",
    f"{dist}/entry_points.txt": b"[console_scripts]\nrefute=refute_tool.cli:refute\nrefute-tool=refute_tool.cli:main\n",
}
records = []
with zipfile.ZipFile(wheel_path, "w", zipfile.ZIP_DEFLATED) as zf:
    for name, data in files.items():
        zf.writestr(name, data)
        digest = base64.urlsafe_b64encode(hashlib.sha256(data).digest()).rstrip(b"=").decode()
        records.append((name, f"sha256={digest}", str(len(data))))
    record_name = f"{dist}/RECORD"
    rows = [*records, (record_name, "", "")]
    text = "\n".join(",".join(row) for row in rows) + "\n"
    zf.writestr(record_name, text)
PY

tar -C adapters/cargo --exclude ./target -czf "${dist_dir}/cargo-refute-${package_version}.tar.gz" .

jvm_repo="${dist_dir}/maven-repository/ai/shatterproof/refute-tool/${package_version}"
mkdir -p "${jvm_repo}/classes"
javac -d "${jvm_repo}/classes" adapters/jvm/src/main/java/ai/shatterproof/refute/RefuteTool.java
jar --create --file "${jvm_repo}/refute-tool-${package_version}.jar" -C "${jvm_repo}/classes" .
sed "s/<version>0.0.0-dev<\\/version>/<version>${package_version}<\\/version>/" adapters/jvm/pom.xml > "${jvm_repo}/refute-tool-${package_version}.pom"
rm -rf "${jvm_repo}/classes"
tar -C "${dist_dir}/maven-repository" -czf "${dist_dir}/refute-tool-maven-repository-${package_version}.tar.gz" .

(
  cd "${dist_dir}"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum ./*.tar.gz ./*.tgz ./*.whl > checksums.txt
  else
    shasum -a 256 ./*.tar.gz ./*.tgz ./*.whl > checksums.txt
  fi
)

python3 - "${version}" "${package_version}" "${dist_dir}" <<'PY'
import hashlib
import json
import os
import pathlib
import sys

version, package_version, dist = sys.argv[1], sys.argv[2], pathlib.Path(sys.argv[3])
owner = os.environ.get("GITHUB_REPOSITORY", "shatterproof-ai/refute")
base = f"https://github.com/{owner}/releases/download/{version}"
items = []
for path in sorted(dist.iterdir()):
    if path.suffixes[-2:] == [".tar", ".gz"] and path.name.startswith("refute_"):
        _, item_version, platform, arch_ext = path.name.split("_", 3)
        arch = arch_ext.removesuffix(".tar.gz")
        data = path.read_bytes()
        items.append({
            "version": item_version,
            "package_adapter_version": package_version,
            "platform": platform,
            "architecture": arch,
            "artifact_filename": path.name,
            "sha256": hashlib.sha256(data).hexdigest(),
            "size": len(data),
            "download_url": f"{base}/{path.name}",
        })
manifest = {
    "version": version,
    "package_adapter_version": package_version,
    "artifacts": items,
}
manifest_path = dist / f"refute-manifest-{version}.json"
manifest_path.write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
PY

(
  cd "${dist_dir}"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "refute-manifest-${version}.json" >> checksums.txt
  else
    shasum -a 256 "refute-manifest-${version}.json" >> checksums.txt
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
