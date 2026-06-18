#!/usr/bin/env bash
#
# corpus-fetch.sh — materialize the pinned real-world refactoring corpus.
#
# Reads testdata/corpus/manifest.json and clones each target repository at its
# pinned commit into the gitignored cache directory (default .corpus-cache/).
# Materialization is idempotent: a target already checked out at the pinned
# commit is left untouched.
#
# Usage:
#   scripts/corpus-fetch.sh                # fetch every target
#   scripts/corpus-fetch.sh go-x-example-hello rust-itoa   # fetch named targets
#
# This is the network-dependent half of the corpus lane. It is deliberately
# separate from `go test ./...`; the //go:build corpus tests call back into this
# script to materialize a single target on demand.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
manifest="$repo_root/testdata/corpus/manifest.json"

if [[ ! -f "$manifest" ]]; then
  echo "corpus-fetch: manifest not found at $manifest" >&2
  exit 1
fi
if ! command -v git >/dev/null 2>&1; then
  echo "corpus-fetch: git is required but not on PATH" >&2
  exit 1
fi
if ! command -v python3 >/dev/null 2>&1; then
  echo "corpus-fetch: python3 is required to parse the manifest" >&2
  exit 1
fi

cache_dir="$repo_root/$(python3 -c '
import json, sys
with open(sys.argv[1]) as f:
    print(json.load(f).get("cacheDir", ".corpus-cache"))
' "$manifest")"

# Emit "name<TAB>repo<TAB>commit<TAB>subdir" for the requested targets (all when
# no names are given). Unknown names are a hard error so a typo cannot silently
# fetch nothing.
mapfile -t rows < <(python3 -c '
import json, sys
manifest, *want = sys.argv[1:]
with open(manifest) as f:
    targets = {t["name"]: t for t in json.load(f)["targets"]}
names = want or list(targets)
missing = [n for n in names if n not in targets]
if missing:
    sys.stderr.write("corpus-fetch: unknown target(s): %s\n" % ", ".join(missing))
    sys.stderr.write("available: %s\n" % ", ".join(targets))
    sys.exit(2)
for n in names:
    t = targets[n]
    print("\t".join([t["name"], t["repo"], t["commit"], t.get("subdir", ".")]))
' "$manifest" "$@")

mkdir -p "$cache_dir"

for row in "${rows[@]}"; do
  IFS=$'\t' read -r name repo commit subdir <<<"$row"
  dest="$cache_dir/$name"
  stamp="$dest/.corpus-commit"

  if [[ -f "$stamp" && "$(cat "$stamp")" == "$commit" ]]; then
    echo "corpus-fetch: $name already at $commit (skip)"
    continue
  fi

  echo "corpus-fetch: materializing $name from $repo @ $commit"
  rm -rf "$dest"
  git init -q "$dest"
  git -C "$dest" remote add origin "$repo"

  # Fetch the exact pinned commit. GitHub allows fetching a SHA directly, which
  # keeps the clone shallow and reproducible regardless of branch movement.
  if ! git -C "$dest" fetch -q --depth 1 origin "$commit"; then
    rm -rf "$dest"
    echo "corpus-fetch: failed to fetch $commit from $repo (network or pin error)" >&2
    exit 1
  fi
  git -C "$dest" checkout -q FETCH_HEAD
  echo "$commit" >"$stamp"

  if [[ "$subdir" != "." && ! -d "$dest/$subdir" ]]; then
    echo "corpus-fetch: subdir '$subdir' missing in $name @ $commit (stale pin?)" >&2
    exit 1
  fi
  echo "corpus-fetch: $name ready"
done
