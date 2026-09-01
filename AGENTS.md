# Modbus Console Agent Governance

These rules apply to every file under `modbus-console/`.

## Mandatory preflight — before analysis or edits

1. Read this `AGENTS.md` completely.
2. When OneLife/MCP tooling is available, read a compact inventory of active tools once per work session (`tool_list(activeOnly=true, includeDescription=true)` or the equivalent connector catalog). Do not load every heavy tool schema unless needed, but know what tools are available before choosing shell/manual work.
3. Run `./scripts/read-skills.sh`. It prints and fingerprints **every installed project skill** plus the parent workspace skills. Do not edit source until it succeeds.
4. Run `./scripts/tool-audit.sh` and resolve missing REQUIRED tools.
5. Run `graft check`. If Graft is unavailable because of a parser/tooling issue, record the failure and use scoped reads + `rg` only.
6. Read `docs/architecture.md` before changing a public API, transport, security boundary, tunnel behavior, or release packaging.

A changed skill invalidates the preflight fingerprint; rerun `./scripts/read-skills.sh`.

## Product boundaries

- `apps/web`: Vercel-hosted UI. It must never try to open raw TCP sockets.
- `core`: downloadable native engine. It owns raw Modbus TCP and OS serial/USB access.
- Cloudflare Tunnel is a control-plane path to the Core HTTP/WebSocket API, not the Modbus data path between local devices and the Core.
- Local Modbus TCP continues to work without Internet.
- Web Serial may be added as an optional browser-direct RTU transport, never as the only RTU path.

## Protocol contract

Support Modbus FC01, FC02, FC03, FC04, FC05, FC06, FC15, FC16. Slave IDs are 1..247; addresses are 0..65535. Validate Modbus quantity limits before network I/O.

## Security contract

- Core binds loopback by default.
- All `/api/v1/*` control endpoints require a bearer pairing token. `/health` may be public and bounded.
- Never log auth tokens, Cloudflare credentials, serial data marked secret, or user credentials.
- CORS is allow-list based; never ship `*` together with credentialed control APIs.
- Cloudflare API/account tokens are never embedded in downloadable binaries.
- Writes (FC05/06/15/16) must be visibly distinguishable from reads in UI and event logs.

## Quality gates

Before handoff run:

```bash
./scripts/preflight.sh
pnpm typecheck
pnpm test
pnpm build
(cd core && golangci-lint run ./...)
(cd core && staticcheck ./...)
(cd core && "$(go env GOPATH)/bin/govulncheck" ./...)
./scripts/release-check.sh
```

After structural changes, rerun `graft build && graft check` when Graft is functional.

## Git safety

Do not reset, clean, stash, commit, push, rebase or overwrite unrelated workspace WIP unless explicitly requested.


<!-- sws-root-governance:start -->
## SWS Root Governance (mandatory)

Before any work in this project:

1. Read this file and `../AGENTS.md`.
2. In a OneLife/MCP session, inspect the compact active tool inventory/token report before choosing tools.
3. Run `../.agents/scripts/sws-preflight.sh` from this project directory. Do not edit source until it reports `SWS_PREFLIGHT_OK`.
4. Read every skill printed as `APPLICABLE`; conditional testing/SQL/security skills become mandatory when the task matches them.
5. Use Graft/specialized tools before broad file reads or generic shell commands.
6. Preserve unrelated git WIP and never expose secrets.

The canonical project map is `../docs/agent/SWS_PROJECT_MAP.md`.
<!-- sws-root-governance:end -->
