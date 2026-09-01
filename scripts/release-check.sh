#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
if git remote get-url origin >/dev/null 2>&1; then
  goreleaser check
  echo RELEASE_CONFIG=PASS
  exit 0
fi
TMP=$(mktemp -d /tmp/modbus-console-release-check.XXXXXX)
cp .goreleaser.yaml "$TMP/"
cp -R core "$TMP/core"
cd "$TMP"
git init -q
git config user.email 'release-check@local.invalid'
git config user.name 'Modbus Console Release Check'
git add .
git commit -qm 'snapshot release check'
git remote add origin https://github.com/modbus-console/modbus-console.git
goreleaser check >/dev/null
goreleaser release --snapshot --clean --skip=publish >/dev/null
archives=$(find dist -maxdepth 1 \( -name '*.zip' -o -name '*.tar.gz' \) | wc -l | tr -d ' ')
sboms=$(find dist -maxdepth 1 -name '*.sbom.json' | wc -l | tr -d ' ')
[[ "$archives" == 6 && "$sboms" == 6 && -f dist/checksums.txt ]]
echo "RELEASE_SNAPSHOT=PASS archives=$archives sboms=$sboms checksums=1"
echo 'RELEASE_REMOTE=NOT_CONFIGURED (snapshot validation used; no publish attempted)'
