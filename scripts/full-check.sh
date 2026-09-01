#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo '== preflight =='
./scripts/preflight.sh

echo '== typecheck =='
pnpm typecheck

echo '== lint =='
pnpm lint

echo '== tests =='
pnpm test

echo '== production build =='
pnpm build

echo '== static analysis / vulnerabilities =='
(cd core && staticcheck ./...)
(cd core && "$(go env GOPATH)/bin/govulncheck" ./...)

echo '== release snapshot =='
./scripts/release-check.sh

echo '== graft refresh =='
graft build
graft check

echo FULL_CHECK=PASS
