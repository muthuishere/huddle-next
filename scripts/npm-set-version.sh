#!/usr/bin/env bash
# Set one version across the npm launcher, its optionalDependencies, and all platform packages.
# Usage: scripts/npm-set-version.sh 0.2.0
set -euo pipefail

VERSION="${1:?usage: scripts/npm-set-version.sh <version>}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

VERSION="$VERSION" ROOT_DIR="$ROOT_DIR" node -e '
const fs = require("fs");
const path = require("path");
const v = process.env.VERSION;
const root = process.env.ROOT_DIR;
const files = [path.join(root, "npm/package.json")];
for (const p of fs.readdirSync(path.join(root, "npm/platforms"))) {
  files.push(path.join(root, "npm/platforms", p, "package.json"));
}
for (const f of files) {
  const pkg = JSON.parse(fs.readFileSync(f, "utf8"));
  pkg.version = v;
  if (pkg.optionalDependencies) {
    for (const k of Object.keys(pkg.optionalDependencies)) pkg.optionalDependencies[k] = v;
  }
  fs.writeFileSync(f, JSON.stringify(pkg, null, 2) + "\n");
  console.log(`${pkg.name}@${v}`);
}
'
