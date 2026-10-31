# Changelog

All notable changes to the **Mesh Node Telemetry Sender** (`m3tal-mesh-node`) daemon for M3TAL Core will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [0.2.0] - 2026-10-31

### 🚀 Changes & Improvements
- **merge: resolve changelog conflicts with origin/main** (`7dd0e07`)
- **refactor(mesh): rely exclusively on nil-safe accessors for default configuration flags** (`9afbf1d`)
- **fix(mesh): prioritize replacement tokens, enforce policy on tokenless fetches, and share response parsing tail** (`2692e32`)
- **fix(mesh): preserve tokens on blank settings save, wire discovery toggle, and optimize unauthenticated flow** (`d13a6f8`)
- **feat(mesh): enforce pairing token by default and add settings to make optional with safety warnings** (`5da9b8f`)

---

## [Unreleased]
### Added
- Register `settings` capability and `spec.settings` in `plugin.yaml` for M3TAL Core settings integration.
- Add `require_pairing_token` setting (default `true`) and safety warning banner when token requirement is disabled.

---

## [0.1.5] - 2026-10-31

### 🚀 Changes & Improvements
- **fix(mesh): bind challenge-response proofs to TLS certificate fingerprints and client IP** (`8b2152b`)
- **fix(mesh): restore payload merging in UI readers, restore node smoke tests, and format network.go** (`294ccd2`)
- **fix(mesh): read flat action responses in UI and assert full response shape in tests** (`3d728e3`)
- **refactor(mesh): emit flat action responses and read payload cleanly in UI** (`af69661`)
- **fix(mesh): align action payload responses and ui readers with core passthrough** (`222bf33`)
- **fix(mesh): use flat action response data** (`746dc13`)
- **fix(mesh): improve UI error handling and cleanup** (`e6ca0ca`)
- **fix: handle nested payload data structures in mesh UI API responses** (`47a5a35`)

---

## [0.1.4] - 2026-10-28

### Fixed
- Version bump to align with m3tal-mesh cluster metapackage

## [0.1.3] - 2026-10-28

### Fixed
- Add client-issued nonce to pairing protocol to prevent replay attacks
- Implement per-client rate limiting on challenge requests (16 per client IP)
- Configure HTTP server with proper timeouts and connection limits

## [0.1.2] - 2026-10-27

### Added
- Authenticated HTTP telemetry endpoint and mDNS service advertisement for remote Mesh Fleet pairing.
- Plugin-provided Mesh Node Settings view for pairing token, port, and LAN discovery configuration.

## [0.1.1] - 2026-10-27

### Fixed
- Align the node package version with the Mesh cluster metapackage dependency.

## [0.1.0] - 2026-10-26

### Overview & Highlights
Initial release of the headless **Mesh Node Telemetry Broadcaster** (`m3tal-mesh-node`) daemon. Designed to run on secondary servers, NAS devices, and edge machines without requiring GUI dependencies.

### 🚀 Capabilities & Features
- **Headless Daemon**: Lightweight compiled Go binary (~15MB RAM footprint) operating without any display server or desktop dependencies.
- **Canonical Process Identity**: Executes with `prctl(PR_SET_NAME, "m3tal-mesh-node")` and compiled to `bin/m3tal-mesh-node` for direct host execution.
- **Real-Time System Metrics Inspection**:
  - Gathers CPU load averages and online core topology from `/proc/loadavg` and `/proc/cpuinfo`.
  - Parses real memory usage, buffers, and swap allocations from `/proc/meminfo`.
  - Calculates mountpoint filesystem statistics (used, free, total) via `syscall.Statfs`.
  - Detects hardware accelerators (NVIDIA GPU utilization, VRAM, and temperatures).
- **JSON-RPC 2.0 Engine**:
  - Handles `m3tal.init`, `m3tal.collect`, `m3tal.action` (`get_telemetry`, `beacon_ping`), and `m3tal.shutdown`.
