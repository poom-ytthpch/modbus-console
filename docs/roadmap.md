# Roadmap / delivery status

## MVP — COMPLETE (software)
- Vercel-ready Next.js UI
- Native Go Core for macOS/Windows/Linux
- Modbus TCP master/slave FC01/02/03/04/05/06/15/16
- Modbus RTU master/slave FC01/02/03/04/05/06/15/16
- USB serial discovery + configurable baud/data/parity/stop/timeout
- RTU CRC16, silent interval and transport-error close/reopen behavior
- Continuous read polling and raw TX/RX inspection
- SWS relay active-low compatibility profile shared by TCP/RTU slave
- bearer pairing + origin allow-list + loopback defaults
- Cloudflare Quick Tunnel dev helper + named Tunnel production helper
- GoReleaser Mac/Windows/Linux archives, checksums, SBOM
- CI, lint, static analysis and vulnerability scan

## Hardware acceptance — REQUIRED PER ADAPTER/DEVICE
- USB-RS485 adapter driver smoke test on target OS
- FC01..16 loop test against real adapter/device
- unplug/replug test
- long-running RTU poll soak test

## Optional post-MVP
- Web Serial browser-direct transport where supported
- Cloudflare Access enrollment UX and short-lived session exchange
- event stream/presence dashboard
- signed/notarized macOS and Authenticode Windows installers
- accounts/workspaces, saved cloud profiles and team audit history
