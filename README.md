# M3TAL Plugins Repository (`m3tal-plugins`)

This repository contains all official and community plugins for the **M3TAL Core** platform (`jakej985-rgb/m3tal-core`).

## 1. Architectural Principles

- **Zero Core Footprint**: `m3tal-core` contains zero plugin-specific UI components, API endpoints, or models. It acts solely as the Host Supervisor and Dynamic UI mount point.
- **Standardized Plugin Folders (`m3tal-<plugin-name>`)**: Every plugin lives in its own top-level directory following the canonical format `m3tal-<plugin-name>` (e.g. `m3tal-docker/`, `m3tal-stacks/`, `m3tal-ai/`, `m3tal-smart/`). There are no intermediate wrapper folders or symlinks.
- **Complete Self-Containment**: All backend code, frontend UI assets, manifest (`plugin.yaml`), configuration schema, and build rules live strictly within the plugin's own folder. Each plugin directory includes its own dedicated `README.md`.
- **Canonical Process Identity & Direct Execution (`m3tal-<process>`)**:
  - Every running plugin subprocess, collector, and worker MUST identify as `m3tal-<process>` (e.g. `m3tal-docker`, `m3tal-stacks`, `m3tal-ai`, `m3tal-smart`) so operators and monitoring tools can instantly track high CPU, memory, or disk I/O usage in `top`, `htop`, or `m3tal-sysmon`.
  - **First-Party Plugins (Direct Execution)**: Compiles binaries canonically as `bin/m3tal-<process>`. M3TAL Core detects this and executes directly from its install location without creating symlinks, eliminating `/tmp` dependencies and avoiding `noexec` failures.
  - **Third-Party / Legacy Plugins (Runtime Symlink)**: If a plugin uses a non-canonical binary name (e.g. `server`, `app`), the supervisor automatically creates a canonical symlink inside `/run/m3tal/plugins-bin/` (the secure systemd runtime directory) instead of polluting `/tmp`.
- **Core API Key Inheritance (`M3TAL_API_KEY`)**: When M3TAL Core supervises a plugin process, it injects Core API environment variables such as `M3TAL_API_KEY`, `M3TAL_API_TOKEN`, `M3TAL_CORE_VERSION`, `M3TAL_PLUGIN_DIR`, and `M3TAL_PROCESS_NAME` into the subprocess environment. Plugins can use those variables to call Core APIs without embedding secrets in their manifests.
- **Debian Packaging & Central Signing**: All `.deb` packages (`m3tal-plugin-<name>_<version>_amd64.deb`) are built in this repository and dispatched to `jakej985-rgb/m3tal-apt-key` for official GPG signing and APT repository indexing.

---

## 2. Official & Planned Plugins

For full profiles, capabilities, and detailed documentation on every plugin, consult the **[Official Plugin Catalog (`PLUGINS.md`)](./PLUGINS.md)**.

| Plugin Folder | Title | Canonical Process | Debian Package | Status | What it does |
|---|---|---|---|---|---|
| [`m3tal-docker`](./m3tal-docker/) | **Containers** | `m3tal-docker` | `m3tal-plugin-docker-monitor` | 🟢 Available | Docker fleet monitor, log inspector, lifecycle controls, image pruning. |
| [`m3tal-mesh`](./m3tal-mesh/) | **Mesh Fleet** | `m3tal-mesh` | `m3tal-plugin-mesh` | 🟢 Available | Personal Cloud & Local Network Fleet Federation, peer discovery, and multi-node hardware telemetry reader. |
| [`m3tal-mesh-node`](./m3tal-mesh-node/) | **Mesh Node** | `m3tal-mesh-node` | `m3tal-plugin-mesh-node` | 🟢 Available | Headless M3TAL telemetry broadcaster and node metrics daemon for personal cloud clustering. |
| [`m3tal-compose`](./m3tal-compose/) | **Compose Editor** | `m3tal-compose` | `m3tal-plugin-compose` | � Available | Visual & YAML Compose editor, live syntax validator, stack deployer (`up`/`down`/`restart`). |
| [`m3tal-stacks`](./m3tal-stacks/) | **Compose Template DB** | `m3tal-stacks` | `m3tal-plugin-stacks` *(planned)* | ⚪ Planned | Curated database of verified Docker Compose templates, argument presets, 1-click apps. |
| [`m3tal-ai`](./m3tal-ai/) | **Local AI Assistant** | `m3tal-ai` *(prototype)* | `m3tal-plugin-ai` *(planned)* | 🟡 Developing | In-progress assistant/RAG prototype; not yet a complete installable Core plugin. |
| [`m3tal-smart`](./m3tal-smart/) | **Smart Disk Telemetry** | `m3tal-smart` | `m3tal-plugin-smart` | 🟡 Planned | Drive health telemetry, NVMe wear life, S.M.A.R.T. self-tests, temperature history. |
| [`m3tal-traefik`](./m3tal-traefik/) | **Traefik Gateway** | `m3tal-traefik` | `m3tal-plugin-traefik` | 🟡 Planned | Traefik v3 reverse proxy, automated Let's Encrypt SSL/TLS, dynamic routing. |
| [`m3tal-vpn`](./m3tal-vpn/) | **VPN Tunnel** | `m3tal-vpn` | `m3tal-plugin-vpn` | 🟡 Planned | WireGuard/OpenVPN tunnel connectivity with container egress routing and leak protection. |
| [`m3tal-samba-nfs`](./m3tal-samba-nfs/) | **File Shares** | `m3tal-samba-nfs` | `m3tal-plugin-samba-nfs` | 🟡 Developing | Graphical Samba/NFS share manager, guest permissions, user credentials, active mounts. |
| [`m3tal-bench`](./m3tal-bench/) | **System Benchmarks** | `m3tal-bench` | `m3tal-plugin-bench` | 🟡 Planned | One-click benchmarking for CPU multi-threading, GPU compute, and disk I/O throughput. |

---

## 3. Repository Layout

```text
m3tal-plugins/
├── m3tal-<plugin-name>/         # E.g. m3tal-docker, m3tal-stacks, m3tal-ai
│   ├── .agents/
│   │   └── agents/
│   │       └── plugin-maintainer/
│   │           └── agent.md     # In-repo custodian agent for this plugin
│   ├── plugin.yaml              # Standard manifest (apiVersion: m3tal.io/v1alpha1)
│   ├── README.md                # Plugin-specific documentation & architecture
│   ├── CHANGELOG.md             # Version history following Keep a Changelog
│   ├── Makefile                 # Plugin-specific build & test targets
│   ├── bin/                     # Backend binary or executable entrypoint
│   ├── ui/
│   │   └── dist/
│   │       ├── index.html       # Web UI micro-bundle mounted at /plugins/<name>/
│   │       └── assets/          # Relative static assets
│   ├── config/
│   │   └── config.schema.json   # Settings JSON schema
│   └── src/                     # Backend source code (Go, Python, Shell, Rust)
├── scripts/
│   ├── build-plugin-deb.sh      # Central Debian packaging script
│   └── smart-bump.py            # Monorepo version bumper & touched plugin detector
├── .github/
│   └── workflows/
│       ├── Build-m3tal-docker-Deb.yml # Dedicated per-plugin release workflow
│       └── ...
├── PLUGINS.md                   # Full catalog & description of all plugins
└── Makefile                     # Monorepo build orchestrator
```

---

## 4. Developing a Plugin

### Scaffolding a New Plugin
Use the `plugin-author` subagent from `m3tal-core`:
```bash
# In M3tal-core chat:
# "Scaffold a new plugin named 'm3tal-smart' in m3tal-plugins"
```
The agent automatically scaffolds the directory structure, JSON-RPC 2.0 loop, static UI bundle, `README.md`, and embeds `.agents/agents/plugin-maintainer/agent.md`.

### Smart Version Auto-Bumping & Change Detection
The repository includes a smart change detector that only bumps version and builds plugins that have been modified:
```bash
# Check which plugins have been touched without modifying files (dry-run):
make bump-check

# Auto-bump version in plugin.yaml and update CHANGELOG.md for touched plugins:
make bump

# Build only plugins that have been modified:
make build-changed

# Package Debian (.deb) only for modified plugins:
make package-changed
```

### Building Plugins
```bash
# Build a single plugin:
make build PLUGIN=m3tal-docker

# Build all active plugins:
make build-all

# Test a single plugin:
make test PLUGIN=m3tal-docker
```

### Packaging into Debian (`.deb`)
```bash
./scripts/build-plugin-deb.sh m3tal-docker
```
Generates `m3tal-plugin-docker-monitor_<version>_amd64.deb`.

---

## 5. Documentation

- [`PLUGINS.md`](./PLUGINS.md) — current plugin catalog, status, capabilities, and install commands.
- [`catalog.json`](./catalog.json) — machine-readable catalog consumed by Core.
- [`docs/IPC_PROTOCOL.md`](./docs/IPC_PROTOCOL.md) — JSON-RPC 2.0 stdio contract.
- [`docs/PLUGIN_AUTHORING.md`](./docs/PLUGIN_AUTHORING.md) — manifest, process, UI, settings, and action guidance.
- [`docs/RELEASING.md`](./docs/RELEASING.md) — versioning, packaging, GitHub Release, and APT ingestion runbook.
- [`docs/TROUBLESHOOTING.md`](./docs/TROUBLESHOOTING.md) — discovery, lifecycle, IPC, and packaging diagnostics.

---

## 6. Release & APT Ingestion Pipeline

Release automation uses one workflow per releasable plugin, named `Build-<plugin_name>-Deb.yml`:

- `m3tal-docker` → `Build-m3tal-docker-Deb.yml`
- `m3tal-compose` → `Build-m3tal-compose-Deb.yml`
- `m3tal-samba-nfs` → `Build-m3tal-samba-nfs-Deb.yml`
- `m3tal-mesh` → `Build-m3tal-mesh-Deb.yml` (intentionally shared: builds `m3tal-mesh`, `m3tal-mesh-node`, and `m3tal-plugin-mesh-cluster` together so cluster versions stay aligned)
- Planned plugins get a `Build-<plugin_name>-Deb.yml` workflow when they receive a `plugin.yaml` and build target.

For workflow-backed plugins:

1. Changes are pushed to `main` under the plugin directory, or the workflow is manually dispatched.
2. `scripts/smart-bump.py` updates `plugin.yaml` and `CHANGELOG.md` for touched plugins.
3. `scripts/build-plugin-deb.sh` builds the `.deb`; mesh additionally builds `m3tal-plugin-mesh-cluster`.
4. Assets are attached to the rolling GitHub Release tag `plugins-latest`.
5. The workflow dispatches `publish-deb` to `jakej985-rgb/m3tal-apt-key`:
   ```json
   {
     "package_id": "m3tal-plugin-<name>",
     "version": "<version>",
     "deb_url": "https://github.com/jakej985-rgb/m3tal-plugins/releases/download/plugins-latest/m3tal-plugin-<name>_<version>_amd64.deb",
     "allow_replace": true
   }
   ```
6. `m3tal-apt-key` validates, signs, indexes, commits, and deploys the package.
7. Users install with:
   ```bash
   sudo apt update
   sudo apt install m3tal-plugin-<name>
   ```

