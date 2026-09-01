#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
required=(go node pnpm git cloudflared goreleaser golangci-lint staticcheck syft cosign jq curl)
missing=0
for t in "${required[@]}"; do
  if command -v "$t" >/dev/null 2>&1; then printf 'OK      %-16s %s\n' "$t" "$(command -v "$t")"; else printf 'MISSING %-16s\n' "$t"; missing=1; fi
done
GOVULN="$(go env GOPATH)/bin/govulncheck"
if [[ -x "$GOVULN" ]]; then echo "OK      govulncheck      $GOVULN"; else echo "MISSING govulncheck"; missing=1; fi
CORE_GO="$(cd "$ROOT/core" && go env GOVERSION 2>/dev/null || true)"
if [[ "$CORE_GO" == go1.27* ]]; then
  echo "OK      core-go          $CORE_GO"
else
  echo "MISMATCH core-go         ${CORE_GO:-unavailable} (requires go1.27.x)"
  missing=1
fi
exit "$missing"
