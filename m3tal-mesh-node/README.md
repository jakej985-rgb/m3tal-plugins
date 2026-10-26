# M3TAL Mesh Node Telemetry Sender (`m3tal-mesh-node`)

[![Status: Active](https://img.shields.io/badge/Status-Active-brightgreen.svg)](https://m3tal.io)
[![Process Identity: m3tal-mesh-node](https://img.shields.io/badge/Process-m3tal--mesh--node-brightgreen.svg)](https://m3tal.io)
[![Package: m3tal-plugin-mesh-node](https://img.shields.io/badge/Package-m3tal--plugin--mesh--node-blue.svg)](https://m3tal.io)

The **M3TAL Mesh Node Telemetry Broadcaster** (`m3tal-mesh-node`) is a lightweight headless daemon designed to run on remote servers, NAS devices, and secondary machines. It gathers local hardware telemetry and exposes it to `m3tal-mesh` cluster readers.

---

## 1. Architectural Overview & Identity

- **Canonical Process Identity**: `m3tal-mesh-node`
- **Package Name**: `m3tal-plugin-mesh-node`
- **Capabilities**: `collector`, `actions`
- **Execution Model**: Compiled directly to `bin/m3tal-mesh-node` for headless execution without desktop dependencies.

---

## 2. Key Features

- **Headless & Minimal Footprint**: Fast, low-memory Go daemon (~15MB RAM).
- **Direct System Metric Extraction**:
  - CPU cores, load averages (`/proc/loadavg`, `/proc/cpuinfo`).
  - Memory and Swap utilization (`/proc/meminfo`).
  - Storage mount points and filesystem sizes via `syscall.Statfs`.
  - GPU status detection (NVIDIA GPUs via `nvidia-smi` parser).
- **Clustering**: Pairs with `m3tal-mesh` for federated fleet management.

---

## 3. IPC Methods

| Method | Description |
|---|---|
| `m3tal.init` | Handshake returning node daemon capabilities and actions. |
| `m3tal.collect` | Emits local node telemetry and hardware metrics. |
| `m3tal.action` | Dispatches node actions (`get_telemetry`, `beacon_ping`). |
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
