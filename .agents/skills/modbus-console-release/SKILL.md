---
name: modbus-console-release
description: Package Modbus Console Core for macOS, Windows and Linux with GoReleaser, checksums, SBOM and signing/notarization gates.
---
# Modbus Console Release

- GoReleaser is the canonical cross-platform packager.
- Target at minimum darwin/arm64, darwin/amd64, windows/amd64, windows/arm64, linux/amd64, linux/arm64.
- Produce SHA-256 checksums and Syft SBOMs.
- Cosign attest/sign CI artifacts when release identity is configured.
- macOS Developer ID signing/notarization and Windows Authenticode are separate credentialed release gates; do not claim them complete without credentials and observed verification.
- Never place signing keys or Cloudflare tokens in repository files.
