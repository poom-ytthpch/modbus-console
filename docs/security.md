# Security model

## Trust boundaries

1. Modbus devices/PLCs communicate with the native Core Engine over local TCP or serial.
2. The browser communicates with Core only through its HTTP control API. Local development may use loopback HTTP; remote production uses HTTPS terminated by Cloudflare Tunnel.
3. A Cloudflare Tunnel may publish that control API remotely; it does not bypass Core authentication.
4. Vercel stores no device credentials or Core pairing token in the default design.

## Pairing token

If `MODBUS_CONSOLE_AUTH_TOKEN` is not supplied, Core creates `~/.modbus-console/pairing-token` with mode `0600`. The token is never printed by Core. The user explicitly copies it into the Web UI, which keeps it in `sessionStorage` only.

## Production tunnel

Use a named Cloudflare Tunnel and scoped tunnel credential. Never bundle a Cloudflare account API token into Core. Add Cloudflare Access or an equivalent identity gate in front of the tunnel in addition to Core bearer auth.

## Writes

FC05/06/15/16 are control operations. UI styling and audit/event work must keep writes visibly distinct from FC01–04 reads.
