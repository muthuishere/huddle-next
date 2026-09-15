#!/usr/bin/env bash
# Publish the npm launcher and platform packages via npm trusted publishing (OIDC).
# Platform packages go first so the launcher's optionalDependencies resolve.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [[ "${GITHUB_ACTIONS:-}" != "true" || -z "${ACTIONS_ID_TOKEN_REQUEST_TOKEN:-}" ]]; then
  echo "error: npm OIDC publish must run inside GitHub Actions with id-token: write" >&2
  exit 1
fi

# MIT requires the notice to travel with every distributed copy. The canonical
# LICENSE lives at the repo root; copy it in at publish time so the six package
# directories cannot drift from it.
stage_license() {
  local dir="$1"
  cp "${ROOT_DIR}/LICENSE" "${dir}/LICENSE"
  cp "${ROOT_DIR}/THIRD-PARTY-LICENSES.md" "${dir}/THIRD-PARTY-LICENSES.md"
}

publish_package() {
  local dir="$1"
  local name version

  name="$(node -p "require('${dir}/package.json').name")"
  version="$(node -p "require('${dir}/package.json').version")"

  if npm view "${name}@${version}" version >/dev/null 2>&1; then
    echo "skip ${name}@${version}: already published"
    return 0
  fi

  echo "publish ${name}@${version}"
  stage_license "$dir"
  (cd "$dir" && npm publish --access public --provenance)
}

for platform in darwin-arm64 darwin-x64 linux-arm64 linux-x64 windows-x64; do
  dir="${ROOT_DIR}/npm/platforms/${platform}"
  bin="huddle"
  [[ "$platform" == windows-* ]] && bin="huddle.exe"
  if [[ ! -f "${dir}/bin/${bin}" ]]; then
    echo "error: ${dir}/bin/${bin} missing — run goreleaser first" >&2
    exit 1
  fi
  publish_package "$dir"
done
publish_package "${ROOT_DIR}/npm"
