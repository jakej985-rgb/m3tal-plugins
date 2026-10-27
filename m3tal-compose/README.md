# M3TAL Compose Editor Plugin (`m3tal-compose`)

[![Status: Active](https://img.shields.io/badge/Status-Active-brightgreen.svg)](https://m3tal.io)
[![Process Identity: m3tal-compose](https://img.shields.io/badge/Process-m3tal--compose-brightgreen.svg)](https://m3tal.io)
[![Package: m3tal-plugin-compose](https://img.shields.io/badge/Package-m3tal--plugin--compose-blue.svg)](https://m3tal.io)

The **M3TAL Compose Editor Plugin** (`m3tal-compose`) is an interactive Docker Compose visual editor, syntax validator, stack deployer, and lifecycle manager for M3TAL Core.

It provides a rich dark-mode micro-frontend embedded as a sub-tab within the **`/containers`** view, complementing the `m3tal-docker` fleet manager.

---

## 1. Architectural Overview & Identity

- **Canonical Process Identity**: `m3tal-compose`
- **Package Name**: `m3tal-plugin-compose`
- **Dashboard UI Route**: `/containers` (Sub-Tab: `Compose Editor`, served at `/plugins/compose/`)
- **Capabilities**: `collector`, `actions`, `ui_view`, `settings`
- **Execution Model**: Compiled directly to `bin/m3tal-compose` for direct host execution without `/tmp` symlinks.

---

## 2. Key Features

- **Visual & YAML Editor**: Split-screen or full-editor view for `docker-compose.yml` / `compose.yaml` files.
- **Quick Service Add**: Append a starter Redis service to the current YAML file.
- **YAML Validation**: Server-side YAML syntax and required `services` mapping validation before saving.
- **Stack Lifecycle Controls**:
  - `compose up -d`: Launches containers in detached mode.
  - `compose down`: Gracefully stops and cleans up stack networks and containers.
  - `compose restart`: Restarts all services within the stack.
  - `compose logs`: Fetches live tail output from all services in the active stack.
- **Multi-Stack Directory Management**: Finds Compose files in the configured folder and lists projects registered with Docker Compose.

### Project discovery and settings

The plugin recursively searches `~/docker` for Compose projects by default and also lists projects registered with Docker Compose. Change the search folder in plugin settings. The existing `M3TAL_COMPOSE_DIR` environment variable overrides that setting when present. New projects use a per-stack folder by default (`<folder>/<name>/compose.yaml`); select **flat** to save them as `<folder>/<name>.yaml`. Existing projects remain in their original location when saved. Backups are enabled by default and use the `.bak` suffix.

---

## 3. IPC Methods

| Method | Description |
|---|---|
| `m3tal.init` | Handshake declaring capabilities, tabs, and actions (`compose_up`, `compose_down`, `compose_restart`, `save_stack`, `validate_yaml`). |
| `m3tal.collect` | Emits active stack telemetry, service counts, and health status table (`compose_stacks`). |
| `m3tal.action` | Dispatches lifecycle actions and saves stack YAML content. |
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
