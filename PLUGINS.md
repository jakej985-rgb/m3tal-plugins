# M3TAL Official Plugin Catalog (`PLUGINS.md`)

This document is the human-readable catalog for plugins in the **M3tal Plugins** monorepo. It describes what each plugin currently implements, how it is packaged, and whether it is published to the central APT repository.

Machine-readable catalog data lives in [`catalog.json`](./catalog.json), with a detached signature in [`catalog.json.sig`](./catalog.json.sig). Core can use that catalog for the Plugin Manager, but this document remains the authoritative explanation of plugin status and boundaries.

## Status vocabulary

| Status | Meaning |
|---|---|
| `available` | A `plugin.yaml` exists, the plugin has release automation or a published package, and the documented APT package is present in `m3tal-apt-key`. |
| `developing` | Implementation exists, but packaging, release automation, or production readiness is incomplete. |
| `planned` | Product direction or scaffold documentation exists, but there is no installable plugin implementation yet. |

## Ecosystem boundary

M3tal Core is only the dashboard, telemetry monitor, settings/API-key manager, and plugin host supervisor. Plugins own workload-specific behavior such as Docker, Compose, mesh federation, file shares, AI assistance, proxies, VPNs, storage health, and benchmarks.

Core and plugins are one ecosystem:

1. A plugin lives in `m3tal-plugins/m3tal-<name>/`.
2. Its `plugin.yaml` declares the process, capabilities, UI route, widgets, and settings.
3. Core discovers the manifest under a plugin search path.
4. Core launches the process and speaks JSON-RPC 2.0 over stdio.
5. Core mounts the plugin UI at `/plugins/<metadata.name>/` and the configured dashboard route.
6. Release automation packages the plugin as `m3tal-plugin-<name>` and dispatches it to `m3tal-apt-key`.
7. Users install the plugin with `sudo apt install m3tal-plugin-<name>`.

See [`docs/IPC_PROTOCOL.md`](./docs/IPC_PROTOCOL.md), [`docs/PLUGIN_AUTHORING.md`](./docs/PLUGIN_AUTHORING.md), and [`docs/RELEASING.md`](./docs/RELEASING.md) for the implementation contract.

## Catalog overview

| Folder | Title | Process | Debian package(s) | Status | Current version |
|---|---|---|---:|---|---:|
| [`m3tal-docker`](./m3tal-docker/) | Containers | `m3tal-docker` | `m3tal-plugin-docker-monitor` | available | 0.8.1 |
| [`m3tal-compose`](./m3tal-compose/) | Compose Editor | `m3tal-compose` | `m3tal-plugin-compose` | available | 0.6.0 |
| [`m3tal-mesh`](./m3tal-mesh/) | Mesh Fleet | `m3tal-mesh` | `m3tal-plugin-mesh`, `m3tal-plugin-mesh-cluster` | available | 0.5.0 |
| [`m3tal-mesh-node`](./m3tal-mesh-node/) | Mesh Node | `m3tal-mesh-node` | `m3tal-plugin-mesh-node` | available | 0.5.0 |
| [`m3tal-samba-nfs`](./m3tal-samba-nfs/) | File Shares | `m3tal-samba-nfs` | `m3tal-plugin-samba-nfs` *(planned package)* | developing | 0.1.0 |
| [`m3tal-ai`](./m3tal-ai/) | Local AI Assistant | `m3tal-ai` *(prototype)* | none yet | developing | n/a |
| [`m3tal-stacks`](./m3tal-stacks/) | Compose Template DB | `m3tal-stacks` | `m3tal-plugin-stacks` *(planned)* | planned | n/a |
| [`m3tal-smart`](./m3tal-smart/) | Smart Disk Telemetry | `m3tal-smart` | `m3tal-plugin-smart` *(planned)* | planned | n/a |
| [`m3tal-traefik`](./m3tal-traefik/) | Traefik Gateway | `m3tal-traefik` | `m3tal-plugin-traefik` *(planned)* | planned | n/a |
| [`m3tal-vpn`](./m3tal-vpn/) | VPN Tunnel | `m3tal-vpn` | `m3tal-plugin-vpn` *(planned)* | planned | n/a |
| [`m3tal-bench`](./m3tal-bench/) | System Benchmarks | `m3tal-bench` | `m3tal-plugin-bench` *(planned)* | planned | n/a |

## Published plugins

### Containers — `m3tal-docker`

- **Package**: `m3tal-plugin-docker-monitor`
- **Route**: `/containers`
- **Capabilities**: `collector`, `actions`, `ui_view`, `ui_widget`, `settings`
- **Implemented actions**: `start_container`, `stop_container`, `restart_container`, `prune_images`, `get_logs`, `inspect_container`, `set_settings`
- **What it does**: monitors Docker containers through the host Docker socket, shows fleet state and logs, and provides lifecycle controls for start/stop/restart plus image pruning.
- **Install**:

```bash
sudo apt update
sudo apt install -y m3tal-plugin-docker-monitor
```

### Compose Editor — `m3tal-compose`

- **Package**: `m3tal-plugin-compose`
- **Route**: `/containers` (`Compose` tab)
- **Capabilities**: `collector`, `actions`, `ui_view`, `settings`
- **Implemented actions**: `set_settings`, `compose_up`, `compose_down`, `compose_restart`, `save_stack`, `validate_yaml`, `get_logs`
- **What it does**: discovers Compose projects, edits Compose files, validates YAML, manages `.env`/project settings, and runs stack lifecycle commands.
- **Install**:

```bash
sudo apt update
sudo apt install -y m3tal-plugin-compose
```

### Mesh Fleet — `m3tal-mesh`

- **Packages**: `m3tal-plugin-mesh` for the reader/dashboard and `m3tal-plugin-mesh-cluster` for the full peer bundle.
- **Route**: `/mesh`
- **Capabilities**: `collector`, `actions`, `ui_view`, `settings`
- **Implemented actions**: `pair_node`, `test_node`, `update_node`, `forget_node`, `ping_node`, `scan_network`, `set_settings`, `get_fleet`, `get_peers`, `get_settings`
- **What it does**: federates telemetry across M3TAL nodes, discovers peers, pairs authenticated nodes, and renders fleet health.
- **Install**:

```bash
# Reader/dashboard package
sudo apt install -y m3tal-plugin-mesh

# Full peer bundle (recommended for a primary node)
sudo apt install -y m3tal-plugin-mesh-cluster
```

### Mesh Node — `m3tal-mesh-node`

- **Package**: `m3tal-plugin-mesh-node`
- **Route**: `/mesh-node` for node settings/status
- **Capabilities**: `collector`, `actions`, `ui_view`, `settings`
- **Implemented actions**: `get_telemetry`, `beacon_ping`, `set_settings`, `get_settings`
- **What it does**: runs as a telemetry broadcaster on servers/NAS/secondary machines, gathers local CPU/RAM/storage/GPU metrics, advertises over mDNS, and serves authenticated telemetry on TCP `9321` by default.
- **Install**:

```bash
sudo apt install -y m3tal-plugin-mesh-node
```

## Developing plugins

### File Shares — `m3tal-samba-nfs`

- **Manifest**: `plugin.yaml` version `0.1.0`
- **Route**: `/storage/shares`
- **Capabilities**: `collector`, `actions`, `ui_view`, `settings`
- **Implemented actions**: `get_shares`, `create_smb_share`, `delete_smb_share`, `create_nfs_export`, `delete_nfs_export`, `create_smb_user`, `delete_smb_user`, `set_smb_password`, `mount_share`, `unmount_share`, `restart_services`, `get_settings`, `set_settings`
- **Status**: implementing. A dedicated release workflow exists (`Build-m3tal-samba-nfs-Deb.yml`), but `m3tal-plugin-samba-nfs` is not yet published to APT.
- **Operational note**: the manifest declares `root: true` because Samba/NFS administration requires privileged access to `/etc/samba`, `/etc/exports`, service reloads, and mount operations.

### Local AI Assistant — `m3tal-ai`

- **Current state**: substantial Go assistant/RAG prototype under `cmd/m3tal-ai` and `internal/assistant`, but **not yet a complete Core plugin**.
- **Missing for plugin release**: `plugin.yaml`, plugin Makefile/build target, UI bundle decision, release workflow, and complete lifecycle packaging.
- **Current role**: deterministic M3TAL assistant that can answer from local documentation, query Core read-only, and propose confirmed Core actions. It can optionally use a model through a `Generator` interface.
- **Model relationship**: `m3tal-llm` is the research/training/serving repository. `m3tal-ai` is the assistant/plugin-side consumer. Model proposals remain schema-constrained, allowlisted, and individually confirmed.
- **Do not advertise** `m3tal-plugin-ai` as installable until the plugin manifest and release path exist.

## Planned plugins

These directories contain product/architecture READMEs but no installable implementation yet:

- `m3tal-stacks` — curated Compose template database.
- `m3tal-smart` — S.M.A.R.T./NVMe disk health telemetry.
- `m3tal-traefik` — Traefik reverse-proxy management.
- `m3tal-vpn` — WireGuard/OpenVPN tunnel management.
- `m3tal-bench` — CPU/GPU/disk benchmark runner.

Planned package names may be reserved in documentation, but they are not currently published to `m3tal-apt-key`.

## Installation sources

Official plugin Debian packages are distributed through the central repository:

```bash
curl -fsSL https://jakej985-rgb.github.io/m3tal-apt-key/install.sh | sudo bash
sudo apt update
sudo apt install -y m3tal-plugin-<name>
```

After installation, Core discovers the plugin automatically and also refreshes on `SIGHUP`/service restart.

## Catalog maintenance rules

When adding or changing a plugin:

1. Keep `plugin.yaml`, `README.md`, `CHANGELOG.md`, and `catalog.json` synchronized.
2. Use canonical process names (`m3tal-<process>`).
3. Declare only capabilities the implementation actually supports.
4. Do not claim IPC methods that are not implemented.
5. Mark `available` only after the APT package is published.
6. Regenerate/sign `catalog.json` through the release pipeline.
7. Update this file whenever status, package names, routes, or major capabilities change.
