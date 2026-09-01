# Architecture research — 2026-08-30

Official-source decisions used for the initial architecture:

- Cloudflare Tunnel: outbound-only connector model; no inbound router port-forward required. WebSockets are supported through Tunnel. Quick Tunnels are development/testing convenience and are not the production identity model. Production should use a named tunnel and scoped credentials.
  - https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/
  - https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/do-more-with-tunnels/trycloudflare/
- Vercel: web UI is a good fit; do not model raw Modbus TCP as a Vercel Function listener. WebSocket/long-lived compute has lifecycle limits, so the browser/Core protocol must reconnect and resume and Core remains authoritative for device state.
  - https://vercel.com/docs/functions
- Web Serial: secure-context browser API, limited browser availability, and available in Dedicated Workers where implemented. This is an optional direct-RTU path, not the universal hardware path.
  - https://developer.mozilla.org/en-US/docs/Web/API/Web_Serial_API
- GoReleaser: cross-platform Go artifact pipeline. Release plan includes checksums and SBOM; platform-native code signing/notarization stays an explicit credentialed gate.
  - https://goreleaser.com/

## Decision

Vercel hosts `apps/web`. A downloadable Go Core owns raw TCP and OS serial access. Cloudflare Tunnel exposes only Core HTTPS/WebSocket control APIs when remote control is needed. Local Modbus device traffic never needs Cloudflare or Vercel.
