# Changelog

All notable changes to the **Mesh Node Telemetry Sender** (`m3tal-mesh-node`) daemon for M3TAL Core will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]
### Added
- Register `settings` capability and `spec.settings` in `plugin.yaml` for M3TAL Core settings integration.
- Add `require_pairing_token` setting (default `true`) and safety warning banner when token requirement is disabled.

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
