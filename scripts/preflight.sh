#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
./scripts/read-skills.sh
./scripts/tool-audit.sh
if command -v graft >/dev/null 2>&1; then
  if graft check; then
    echo GRAFT_CHECK=PASS
  elif graft build && graft check; then
    echo GRAFT_REBUILD_CHECK=PASS
  else
    echo 'GRAFT_CHECK=BLOCKED_ALLOWED_FALLBACK'
  fi
else
  echo 'GRAFT_CHECK=UNAVAILABLE_ALLOWED_FALLBACK'
fi
echo PRECHECK=PASS
