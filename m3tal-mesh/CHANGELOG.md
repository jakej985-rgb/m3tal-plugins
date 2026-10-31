# Changelog

All notable changes to the **Mesh Fleet & Federation** (`m3tal-mesh`) plugin for M3TAL Core will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [0.2.0] - 2026-10-31

### 🚀 Changes & Improvements
- **merge: resolve changelog conflicts with origin/main** (`7dd0e07`)
- **refactor(mesh): rely exclusively on nil-safe accessors for default configuration flags** (`9afbf1d`)
- **refactor(mesh): remove redundant fingerprint check and unreachable refresh error branch** (`a279f89`)
- **fix(mesh): prioritize replacement tokens, enforce policy on tokenless fetches, and share response parsing tail** (`2692e32`)
- **fix(mesh): preserve tokens on blank settings save, wire discovery toggle, and optimize unauthenticated flow** (`d13a6f8`)
- **feat(mesh): enforce pairing token by default and add settings to make optional with safety warnings** (`5da9b8f`)

---

## [Unreleased]
### Added
- Register `settings` capability and `spec.settings` in `plugin.yaml` so Mesh Fleet appears under M3TAL Core's "System Settings & Configurations → PLUGIN SETTINGS".
- Add `require_pairing_token` setting (default `true`), enforcing Pairing Token / Secret authentication by default across modal and background synchronization.
- Provide a configuration toggle to make Pairing Token / Secret optional with prominent safety warnings across Settings and the Pair Node modal.

---

## [0.1.5] - 2026-10-31

### 🚀 Changes & Improvements
- **fix(mesh): bind challenge-response proofs to TLS certificate fingerprints and client IP** (`8b2152b`)
- **fix(mesh): refresh mesh settings after pairing to update has_pairing_token** (`927c7f6`)
- **fix(mesh): restore payload merging in UI readers, restore node smoke tests, and format network.go** (`294ccd2`)
- **fix(mesh): read flat action responses in UI and assert full response shape in tests** (`3d728e3`)
- **refactor(mesh): emit flat action responses and read payload cleanly in UI** (`af69661`)
- **style(mesh): restore script indentation and fallback error messages** (`84affb4`)
- **fix(mesh): align action payload responses and ui readers with core passthrough** (`222bf33`)
- **fix(mesh): use flat action response data** (`746dc13`)
- **fix(mesh): improve UI error handling and cleanup** (`e6ca0ca`)
- **fix: handle nested payload data structures in mesh UI API responses** (`47a5a35`)
- **fix(mesh): flatten get_settings response and add test coverage** (`458404c`)
- **fix(mesh): use success:true in get_settings response to match ActionResult decoder** (`cd9e56e`)

---

## [0.1.4] - 2026-10-28
### Fixed
- Convert LAN discovery to concurrent verification to prevent slow peers from blocking discovery

## [0.1.3] - 2026-10-28
### Fixed
- Add client-issued nonce to pairing protocol to prevent replay attacks
- Implement per-client rate limiting on challenge requests (16 per client IP)
- Fix concurrent re-pair metadata preservation during peer refresh
- Configure HTTP client with connection pooling and proper timeouts

## [0.1.2] - 2026-10-27
### Added
- Mesh Fleet includes its own pairing settings view and stores its token and paired peers with owner-only file permissions.
- Remote pairing verifies the node and token, refreshes live telemetry, supports mDNS discovery, and persists fleet changes.

## [0.1.1] - 2026-10-26

### 🚀 Removed Dummy Mock Nodes & Real Hardware Telemetry
- **Zero Dummy Data**: Completely removed hardcoded mock entries (`node-home-server`, `node-nas-storage`, `node-alpha-r2`) from backend and frontend.
- **Genuine Local Host Telemetry**: Local workstation node now dynamically queries `/proc/loadavg`, `/proc/meminfo`, `syscall.Statfs`, local IPv4 addresses, and NVIDIA GPU stats (`nvidia-smi`).
- **Clean Fleet Discovery**: Fleet view starts strictly with genuine local workstation telemetry and an empty peer list until remote `m3tal-mesh-node` peers are paired.

## [0.1.0] - 2026-10-26

### Overview & Highlights
Initial release of the **Mesh Fleet & Federation Reader** plugin (`m3tal-mesh`) for M3TAL Core. Provides personal cloud multi-node discovery, peer telemetry aggregation, and a distributed cluster dashboard.

### 🚀 Capabilities & Features
- **JSON-RPC 2.0 Engine**: Standard stdio event loop responding to `m3tal.init`, `m3tal.collect`, `m3tal.action`, and `m3tal.shutdown`.
- **Canonical Process Identity**: Executes with `prctl(PR_SET_NAME, "m3tal-mesh")` and compiled to `bin/m3tal-mesh` for direct host execution.
- **Cluster Fleet Dashboard**:
  - Aggregated cluster metrics: Total Nodes Online, Total CPU Cores, Total Memory, and Total ZFS/Disk Storage.
  - Per-node health cards with real-time CPU meters, RAM usage, NVMe/SATA mount gauges, and GPU telemetry.
  - Node Pairing Modal: Add remote servers by IP/hostname or discover local LAN nodes via mDNS beacons.
- **Node Lifecycle Actions**:
  - `pair_node`: Authenticates and registers a remote `m3tal-mesh-node` peer.
  - `forget_node`: Removes a disconnected or decommissioned node from the fleet view.
  - `ping_node`: Measures round-trip network latency to a specific peer.
  - `scan_network`: Scans local subnet for active beacons.
