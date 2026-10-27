# Changelog

All notable changes to the **Mesh Node Telemetry Sender** (`m3tal-mesh-node`) daemon for M3TAL Core will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

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
