# Tooling baseline

This is the required engineering toolchain for Modbus Console. `scripts/tool-audit.sh` is the executable source of truth for local availability.

| Tool | Purpose | Baseline observed 2026-08-30 |
|---|---|---|
| Go | Native Core Engine | Core toolchain Go 1.27.x |
| Node.js | Web/Vercel tooling | 22.x |
| pnpm | Workspace + supply-chain install policy | 11.10.x |
| cloudflared | Development tunnel / production named-tunnel connector | 2026.8.x |
| GoReleaser | Cross-platform native release archives | 2.18.x |
| golangci-lint | Go lint aggregation | 2.13.x |
| staticcheck | Go static analysis | 2026.2.x |
| govulncheck | Reachable Go vulnerability analysis | v1.7.x, built with Go 1.27 |
| Syft | Release SBOM generation | 1.51.x |
| Cosign | Release signing/attestation | 3.1.x |
| Vercel CLI | Web build/deploy tooling | 59.10.x project dependency |
| Graft | Structural code graph | project graph must be fresh before handoff |

## Dependency execution policy

pnpm build scripts are denied by default. The current workspace explicitly permits only the native build steps required by `esbuild` and `unrs-resolver` in `pnpm-workspace.yaml`. Do not broaden this allow-list without reviewing why a package needs install-time code execution.

## Security baseline

The Core module targets Go 1.27.x because the initial Go 1.25 baseline was rejected by `govulncheck` due to reachable standard-library vulnerabilities. A clean handoff requires `govulncheck ./...` to report zero vulnerabilities affecting executed symbols.
