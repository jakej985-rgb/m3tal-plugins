# Changelog

All notable changes to the **Mesh Fleet & Federation** (`m3tal-mesh`) plugin for M3TAL Core will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

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
