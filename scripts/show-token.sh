#!/usr/bin/env bash
set -euo pipefail
TOKEN_FILE="${MODBUS_CONSOLE_TOKEN_FILE:-$HOME/.modbus-console/pairing-token}"
if [[ ! -f "$TOKEN_FILE" ]]; then
  echo "Pairing token does not exist yet. Start Core once first." >&2
  exit 2
fi
cat "$TOKEN_FILE"
