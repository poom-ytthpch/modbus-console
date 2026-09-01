# Modbus Console architecture

```text
Vercel Web UI
   | HTTPS (paired + bearer authenticated)
Cloudflare Tunnel (optional remote control)
   | outbound connector
Go Core Engine on Mac / Windows / Linux
   |-- Modbus TCP master -> PLC / sensor / gateway
   |-- Modbus TCP slave  -> Device Simulator / PLC master
   |-- Modbus RTU master -> USB-RS485 -> device
   `-- Modbus RTU slave  -> USB-RS485 -> external master
```

The TCP and RTU slave transports share the same in-memory register Store, including the SWS active-low relay compatibility profile. The RTU master and RTU slave use an exclusive serial port session; the UI prevents them from owning the same port simultaneously.

Core defaults:
- control API: `127.0.0.1:17777`
- Modbus TCP slave: `127.0.0.1:1502`
- bearer pairing token required for all `/api/v1/*`
- `/health` is bounded and unauthenticated
- RTU serial ports are closed until explicitly opened from the authenticated control API

Cloudflare is never required for local Device Simulator, TCP or RTU traffic. When remote UI control is required, Tunnel exposes only the Core HTTP API; raw Modbus ports remain local/LAN-side.
