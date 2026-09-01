---
name: modbus-console-core
description: Build and validate the native Modbus Console Core Engine for TCP master/slave, serial discovery, secure API and tunnel-safe operation.
---
# Modbus Console Core

Use for all work under `core/`.

- Prefer the Go standard library for TCP, HTTP, lifecycle and concurrency.
- Keep protocol framing independent from transport so RTU can share PDU validation.
- TCP master/slave network operations must have explicit deadlines/timeouts.
- Validate MBAP protocol ID, transaction ID, unit ID, response FC and exception responses.
- Slave writes must be bounded and concurrency-safe.
- Never expose a Core control API without bearer auth except `/health`.
- Test real loopback TCP exchanges; unit-only frame tests are insufficient for master/slave changes.
- Serial enumeration is read-only. Opening a serial port requires an explicit user action through the API/UI.
