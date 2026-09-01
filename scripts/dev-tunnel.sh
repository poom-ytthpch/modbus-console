#!/usr/bin/env bash
set -euo pipefail
if ! command -v cloudflared >/dev/null 2>&1; then
  echo 'cloudflared is required' >&2
  exit 2
fi
URL="${MODBUS_CONSOLE_LOCAL_URL:-http://127.0.0.1:17777}"
echo 'Starting a Cloudflare Quick Tunnel for DEVELOPMENT only.'
echo "Forwarding to $URL"
echo 'The Core bearer token is still required; this script never prints it.'
exec cloudflared tunnel --url "$URL"
