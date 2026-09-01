# Modbus Console

Web-first Modbus Poll + Modbus Slave console with a downloadable native Core Engine.

## What works

- Next.js web UI deployable on Vercel.
- Go Core single binary for macOS, Windows and Linux.
- Modbus TCP master + slave: FC01/02/03/04/05/06/15/16.
- Modbus RTU master + slave over USB-RS485 with baud/data/parity/stop/timeout configuration and CRC16 framing.
- Continuous read polling (100 ms–60 s) plus one-shot reads/writes.
- Shared TCP/RTU slave register bank with SWS active-low relay compatibility.
- Serial-port discovery and safe close/reopen after USB transport errors.
- Bearer pairing token, CORS allow-list, loopback defaults.
- Optional Cloudflare Quick Tunnel for development and named Tunnel helper for remote control.
- GoReleaser archives for darwin/windows/linux amd64+arm64, SHA-256 checksums and SBOMs.

## Built-in SWS virtual lab

After pairing the Web UI with Core, the **Virtual Lab** panel exposes a deterministic SWS device bus on the Core Modbus TCP slave. It includes XY-MD02 (slave 8), EC4400 (2), factory sensor presets (1/3/6), chlorine (4), and the 8-channel active-low relay (5).

Use **Inject device state** to make a simulated sensor change its raw registers, then **Load in Poll** to query that device through the real Modbus master/slave path. Input/Discrete registers remain read-only to Modbus clients; state injection uses a separate authenticated simulator control-plane endpoint. **Reset lab** restores the known baseline.

## Architecture

Vercel hosts only the UI. Raw TCP and USB stay in the native Core. For remote operation, Cloudflare Tunnel exposes the Core control API over HTTPS; it does not expose raw Modbus TCP.

## Start

```bash
./scripts/preflight.sh
pnpm install
(cd core && go mod download)
pnpm test
pnpm build

# terminal 1
pnpm core:run

# show the pairing token after first Core start
./scripts/show-token.sh

# terminal 2
pnpm dev
```

Core defaults: control API `127.0.0.1:17777`, Modbus TCP slave `127.0.0.1:1502`. See `docs/production.md` for Vercel, named Cloudflare Tunnel, LAN slave binding and RTU deployment.
