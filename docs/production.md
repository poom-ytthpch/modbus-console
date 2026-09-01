# Production deployment

## Web on Vercel

Deploy `apps/web` as the Vercel project root. The web app is static/client-side for Core communication; it never opens raw TCP or serial devices itself.

## Core on the operator machine

Run the release archive matching macOS, Windows, or Linux. Core binds `127.0.0.1:17777` for control and `127.0.0.1:1502` for the TCP slave by default. Set `MODBUS_CONSOLE_MODBUS_LISTEN=0.0.0.0:1502` only when another machine on the trusted LAN must reach the slave.

The first start creates `~/.modbus-console/pairing-token` with owner-only permissions. On macOS/Linux use `./scripts/show-token.sh` to deliberately display it for pairing. Never paste the token into logs or a shared ticket.

## Named Cloudflare Tunnel

Authenticate `cloudflared` interactively once, create a named tunnel, and then run:

```bash
export MODBUS_CONSOLE_TUNNEL_NAME=modbus-console
export MODBUS_CONSOLE_HOSTNAME=modbus-core.example.com
./scripts/named-tunnel.sh
```

The public hostname terminates HTTPS at Cloudflare and forwards only the Core HTTP control API. Do not expose port 1502 through the tunnel. Add Cloudflare Access in front of the hostname for production; Core bearer authentication remains mandatory as a second gate.

## USB-RS485 / RTU

Use the Web UI to refresh ports, select serial settings, then choose either RTU Master or RTU Slave. A serial port is exclusive. Master transport supports FC01/02/03/04/05/06/15/16. Slave uses the same register store and SWS active-low relay profile as the TCP slave. On unplug/read/write failure the master session closes; refresh/replug and reopen the port.

## Hardware acceptance

Software tests validate framing, CRC, PDU semantics and cross-platform compilation. A release used with a specific USB-RS485 adapter should still run a hardware loop test for that adapter/driver and target device before production control writes are enabled.
