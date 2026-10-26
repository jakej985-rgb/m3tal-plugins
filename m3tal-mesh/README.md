# M3TAL Mesh Fleet Federation Plugin (`m3tal-mesh`)

[![Status: Active](https://img.shields.io/badge/Status-Active-brightgreen.svg)](https://m3tal.io)
[![Process Identity: m3tal-mesh](https://img.shields.io/badge/Process-m3tal--mesh-brightgreen.svg)](https://m3tal.io)
[![Package: m3tal-plugin-mesh](https://img.shields.io/badge/Package-m3tal--plugin--mesh-blue.svg)](https://m3tal.io)

The **M3TAL Mesh Fleet Federation Plugin** (`m3tal-mesh`) provides personal cloud and local network fleet federation for M3TAL Core. It enables peer discovery, multi-node clustering, and telemetry federation across M3TAL workstations, servers, and NAS nodes.

---

## 1. Architectural Overview & Identity

- **Canonical Process Identity**: `m3tal-mesh`
- **Package Name**: `m3tal-plugin-mesh` (metapackage: `m3tal-plugin-mesh-cluster`)
- **Dashboard UI Route**: `/mesh` (served at `/plugins/mesh/`)
- **Capabilities**: `collector`, `actions`, `ui_view`
- **Execution Model**: Compiled directly to `bin/m3tal-mesh` for direct host execution without `/tmp` symlinks.

---

## 2. Key Features

- **Personal Cloud Multi-Node Discovery**: Scans local LAN and Tailscale networks for active `m3tal-mesh-node` peers.
- **Aggregated Fleet Dashboard**: Real-time aggregate view of Total Nodes, Total CPU Cores, Total RAM, and Storage Pools (ZFS/NVMe).
- **Per-Node Hardware Telemetry**: Live CPU meter, RAM usage, storage pool allocations, and GPU temperature/utilization for each peer.
- **Peer Management Actions**: `pair_node`, `forget_node`, `ping_node`, and `scan_network`.

---

## 3. IPC Methods

| Method | Description |
|---|---|
| `m3tal.init` | Handshake declaring capabilities, tabs, and action declarations. |
| `m3tal.collect` | Emits aggregate cluster metrics and node health status table (`mesh_nodes`). |
| `m3tal.action` | Dispatches node lifecycle actions (`pair_node`, `forget_node`, `ping_node`, `scan_network`). |
| `m3tal.shutdown` | Graceful shutdown. |

---

## 4. Development & Build Commands

```bash
# Build binary
make build

# Run unit tests
make test

# Verify JSON-RPC loop
make test-ipc

# Package into Debian .deb
make package
```
