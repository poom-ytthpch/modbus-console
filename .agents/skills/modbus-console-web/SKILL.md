---
name: modbus-console-web
description: Build the Vercel-hosted Modbus Console UI that controls a paired Core Engine without raw browser TCP.
---
# Modbus Console Web

Use for all work under `apps/web/`.

- The browser talks to Core through HTTPS/WSS only.
- Engine URL and pairing token stay client-side; do not send them through a Vercel server function by default.
- Store pairing tokens in memory/sessionStorage, never localStorage by default.
- Feature-detect Web Serial; do not assume Safari/Firefox support.
- Present FC/address/quantity and raw TX/RX evidence clearly.
- Poll loops must be cancellable and must not overlap requests.
- Vercel deployment must remain stateless; engine state belongs in Core.
