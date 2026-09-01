#!/usr/bin/env bash
set -euo pipefail
: "${MODBUS_CONSOLE_TUNNEL_NAME:?set MODBUS_CONSOLE_TUNNEL_NAME}"
: "${MODBUS_CONSOLE_HOSTNAME:?set MODBUS_CONSOLE_HOSTNAME, e.g. modbus-core.example.com}"
LOCAL_URL="${MODBUS_CONSOLE_LOCAL_URL:-http://127.0.0.1:17777}"
command -v cloudflared >/dev/null 2>&1 || { echo 'cloudflared is required' >&2; exit 2; }
if ! cloudflared tunnel list --output json | jq -e --arg n "$MODBUS_CONSOLE_TUNNEL_NAME" '.[] | select(.name==$n)' >/dev/null; then
  echo "Tunnel '$MODBUS_CONSOLE_TUNNEL_NAME' not found. Run: cloudflared tunnel login && cloudflared tunnel create '$MODBUS_CONSOLE_TUNNEL_NAME'" >&2
  exit 3
fi
cloudflared tunnel route dns "$MODBUS_CONSOLE_TUNNEL_NAME" "$MODBUS_CONSOLE_HOSTNAME"
echo "Starting named tunnel https://$MODBUS_CONSOLE_HOSTNAME -> $LOCAL_URL"
exec cloudflared tunnel --url "$LOCAL_URL" run "$MODBUS_CONSOLE_TUNNEL_NAME"
