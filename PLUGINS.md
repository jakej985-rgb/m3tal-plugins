# M3TAL Official Plugin Catalog (`PLUGINS.md`)

This document is the canonical directory of all official and planned plugins for the **M3TAL Core** Linux Supervisor and Control Plane (`jakej985-rgb/m3tal-core`).

All plugins strictly adhere to the **M3TAL Plugin Standardization Specification**:
- **Folder Format**: `m3tal-<plugin-name>`
- **Process Identity**: Every plugin executes under the canonical title `m3tal-<process>` for instant visibility in `top`, `htop`, and system diagnostic monitors.
- **IPC Protocol**: JSON-RPC 2.0 communication over standard input/output (`stdio`).
- **Debian Distribution**: Packaged as official Debian `.deb` archives and indexed via `jakej985-rgb/m3tal-apt-key`.

---

## 1. Plugin Catalog Overview

| Folder Name | Title | Category | Canonical Process | Debian Package | Status | Short Description |
|---|---|---|---|---|---|---|
| [`m3tal-docker`](./m3tal-docker/) | **Containers** | Container Fleet | `m3tal-docker` | `m3tal-plugin-docker-monitor` | 🟢 Available | Docker fleet monitor, live log inspector, lifecycle controls, image pruning. |
| [`m3tal-compose`](./m3tal-compose/) | **Compose Editor** | Container Management | `m3tal-compose` | `m3tal-plugin-compose` | 🟡 Developing | Visual & YAML Compose editor, live syntax validator, stack deployer (`up`/`down`). |
| [`m3tal-stacks`](./m3tal-stacks/) | **Compose Template DB** | App Store & Hub | `m3tal-stacks` | `m3tal-plugin-stacks` | 🟡 Planned | Curated database of verified Docker Compose templates, argument presets, and 1-click apps. |
| [`m3tal-ai`](./m3tal-ai/) | **Local AI & LLM** | Local AI & LLM | `m3tal-ai` | `m3tal-plugin-ai` | 🟡 Planned | Local LLM inference manager (Ollama, vLLM), GPU VRAM stats, job queue. |
| [`m3tal-smart`](./m3tal-smart/) | **Smart Disk Telemetry** | Hardware Diagnostics | `m3tal-smart` | `m3tal-plugin-smart` | 🟡 Planned | Drive health telemetry, NVMe wear life, S.M.A.R.T. self-tests, thermal history. |
| [`m3tal-traefik`](./m3tal-traefik/) | **Traefik Gateway** | Networking & Proxy | `m3tal-traefik` | `m3tal-plugin-traefik` | 🟡 Planned | Traefik v3 reverse proxy, automated Let's Encrypt SSL/TLS, dynamic routing. |
| [`m3tal-vpn`](./m3tal-vpn/) | **VPN Tunnel** | Network Privacy | `m3tal-vpn` | `m3tal-plugin-vpn` | 🟡 Planned | WireGuard/OpenVPN tunnel connectivity with container egress routing and leak protection. |
| [`m3tal-samba`](./m3tal-samba/) | **Samba Manager** | File Sharing | `m3tal-samba` | `m3tal-plugin-samba` | 🟡 Planned | Graphical Samba/NFS share manager, guest permissions, user credentials, active mounts. |
| [`m3tal-bench`](./m3tal-bench/) | **System Benchmarks** | Diagnostics | `m3tal-bench` | `m3tal-plugin-bench` | 🟡 Planned | One-click benchmarking for CPU multi-threading, GPU compute, and disk I/O throughput. |

---

## 2. Detailed Plugin Profiles

### 🐳 [`m3tal-docker`](./m3tal-docker/) — Containers & Docker Fleet Management
- **Directory**: `m3tal-docker/`
- **Process Identity**: `m3tal-docker`
- **Debian Package**: `m3tal-plugin-docker-monitor`
- **Dashboard Mount**: `/containers` (`/plugins/m3tal-docker/`) — Sub-Tab: `Containers Fleet`
- **Capabilities**: `collector`, `actions`, `ui_view`, `websocket_stream`
- **Status**: 🟢 **Available (v0.2.1)**
- **What it does**:
  Provides complete, real-time Docker container fleet monitoring and lifecycle management. Discovers all running, stopped, and paused containers directly through `/var/run/docker.sock`. Operators can inspect container stdout/stderr log streams live, view CPU/RAM/network usage metrics, trigger container actions (`start`, `stop`, `restart`, `remove`), and reclaim drive space with one-click unused image and builder cache pruning.
- **Install Command**:
  ```bash
  sudo apt install -y m3tal-plugin-docker-monitor
  ```

---

### 📝 [`m3tal-compose`](./m3tal-compose/) — Docker Compose Editor & Stack Manager
- **Directory**: `m3tal-compose/`
- **Process Identity**: `m3tal-compose`
- **Debian Package**: `m3tal-plugin-compose`
- **Dashboard Mount**: `/containers` (`/plugins/m3tal-compose/`) — Sub-Tab: `Compose Editor`
- **Capabilities**: `collector`, `actions`, `ui_view`
- **Status**: 🟡 **Developing (v0.1.0)**
- **What it does**:
  Visual and YAML editor for Docker Compose projects (`docker-compose.yml`, `compose.yaml`). Provides syntax validation, visual service creator, environment variable (`.env`) manager, and direct stack lifecycle actions (`compose up -d`, `compose down`, `compose restart`, `compose pull`, `compose logs`).
- **Install Command**:
  ```bash
  sudo apt install -y m3tal-plugin-compose
  ```

---

### 📚 [`m3tal-stacks`](./m3tal-stacks/) — Docker Compose Template Database & Hub
- **Directory**: `m3tal-stacks/`
- **Process Identity**: `m3tal-stacks`
- **Debian Package**: `m3tal-plugin-stacks`
- **Dashboard Mount**: `/stacks` / `/containers`
- **Capabilities**: `collector`, `actions`, `ui_view`
- **Status**: 🟡 **Planned**
- **What it does**:
  Curated database and template store of verified, production-ready Docker Compose definitions and argument presets (Nextcloud, Plex, Vaultwarden, Pi-hole, WireGuard, Traefik, PostgreSQL, Redis, etc.). Allows operators to browse community stacks, inspect required ports/volumes/environment variables, and export definitions directly into `m3tal-compose` for instant deployment.
- **Install Command**:
  ```bash
  sudo apt install -y m3tal-plugin-stacks
  ```

---

### 🧠 [`m3tal-ai`](./m3tal-ai/) — Local AI & LLM Inference Manager
- **Directory**: `m3tal-ai/`
- **Process Identity**: `m3tal-ai`
- **Debian Package**: `m3tal-plugin-ai`
- **Dashboard Mount**: `/ai`
- **Capabilities**: `inference_runner`, `job_queue`, `ui_view`
- **Status**: 🟡 **Planned**
- **What it does**:
  Brings unified local artificial intelligence and LLM acceleration to the M3TAL platform. Supervises local inference runtimes (Ollama, vLLM, llama.cpp), tracks GPU and VRAM saturation in real time, provides an OpenAI-compatible proxy with token authentication, queues background jobs, and allows one-click downloading and testing of open model weights.
- **Install Command**:
  ```bash
  sudo apt install -y m3tal-plugin-ai
  ```

---

### 💾 [`m3tal-smart`](./m3tal-smart/) — Smart Disk Telemetry & Hardware Diagnostics
- **Directory**: `m3tal-smart/`
- **Process Identity**: `m3tal-smart`
- **Debian Package**: `m3tal-plugin-smart`
- **Dashboard Mount**: `/storage/health`
- **Capabilities**: `collector`, `ui_widget`, `alerts`
- **Status**: 🟡 **Planned**
- **What it does**:
  Continuously monitors the physical health of NVMe drives, SATA SSDs, and HDDs. Tracks S.M.A.R.T. raw attributes, computes NVMe percentage wear life, records historic drive temperature curves to prevent thermal throttling, triggers automated background self-tests, and dispatches early warnings before disk failure occurs.
- **Install Command**:
  ```bash
  sudo apt install -y m3tal-plugin-smart
  ```

---

### 🌐 [`m3tal-traefik`](./m3tal-traefik/) — Traefik Ingress Gateway & SSL Proxy
- **Directory**: `m3tal-traefik/`
- **Process Identity**: `m3tal-traefik`
- **Debian Package**: `m3tal-plugin-traefik`
- **Dashboard Mount**: `/networking/traefik`
- **Capabilities**: `proxy_generator`, `route_provider`
- **Status**: 🟡 **Planned**
- **What it does**:
  Integrates Traefik v3 dynamic ingress management into the M3TAL dashboard. Automatically builds reverse proxy routes from container labels, manages Let's Encrypt SSL/TLS certificates via ACME (HTTP-01 / DNS-01), attaches security middlewares (rate limiting, forward auth, headers), and displays live request metrics and latency graphs.
- **Install Command**:
  ```bash
  sudo apt install -y m3tal-plugin-traefik
  ```

---

### 🛡️ [`m3tal-vpn`](./m3tal-vpn/) — VPN Tunnel & Container Egress Protection
- **Directory**: `m3tal-vpn/`
- **Process Identity**: `m3tal-vpn`
- **Debian Package**: `m3tal-plugin-vpn`
- **Dashboard Mount**: `/networking/vpn`
- **Capabilities**: `network_tunnel`, `status_checker`
- **Status**: 🟡 **Planned**
- **What it does**:
  Supervises WireGuard and OpenVPN tunnel connections (integrating with Gluetun). Provides a strict iptables kill-switch to prevent traffic leaks, allows routing specific container workloads through dedicated VPN namespaces, performs continuous public IP leak tests, and monitors port forwarding allocations.
- **Install Command**:
  ```bash
  sudo apt install -y m3tal-plugin-vpn
  ```

---

### 📁 [`m3tal-samba`](./m3tal-samba/) — Samba & NFS Share Manager
- **Directory**: `m3tal-samba/`
- **Process Identity**: `m3tal-samba`
- **Debian Package**: `m3tal-plugin-samba`
- **Dashboard Mount**: `/storage/shares`
- **Capabilities**: `actions`, `ui_view`, `system_admin`
- **Status**: 🟡 **Planned**
- **What it does**:
  Provides an intuitive visual interface for setting up and managing network shares on Linux. Manages SMB (`/etc/samba/smb.conf`) and NFS (`/etc/exports`) definitions, controls user permissions and guest access, monitors connected clients and active file locks in real time, and enables Apple Time Machine backup support.
- **Install Command**:
  ```bash
  sudo apt install -y m3tal-plugin-samba
  ```

---

### ⚡ [`m3tal-bench`](./m3tal-bench/) — System & Hardware Benchmarks
- **Directory**: `m3tal-bench/`
- **Process Identity**: `m3tal-bench`
- **Debian Package**: `m3tal-plugin-bench`
- **Dashboard Mount**: `/diagnostics/benchmark`
- **Capabilities**: `benchmark_runner`, `ui_view`
- **Status**: 🟡 **Planned**
- **What it does**:
  Offers one-click performance benchmarking for CPU integer and floating-point computations, sequential and 4K random disk read/write bandwidth (IOPS), GPU matrix operations, and network throughput. Maintains historic logs to evaluate hardware changes and automatically throttles tests if thermals exceed safe limits.
- **Install Command**:
  ```bash
  sudo apt install -y m3tal-plugin-bench
  ```
