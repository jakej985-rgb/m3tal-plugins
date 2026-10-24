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
- **Capabilities**: `collector`, `actions`, `ui_view`
- **Execution Model**: Compiled directly to `bin/m3tal-compose` for direct host execution without `/tmp` symlinks.

---

## 2. Key Features

- **Visual & YAML Editor**: Split-screen or full-editor view for `docker-compose.yml` / `compose.yaml` files.
- **Service Generator**: Quick wizard to add services, image tags, container names, exposed port mappings, volumes, and environment variables.
- **Syntax & Schema Validation**: Live client and server-side validation against Docker Compose specification schemas before deployment.
- **Stack Lifecycle Controls**:
  - `compose up -d`: Pulls images and launches containers in detached mode.
  - `compose down`: Gracefully stops and cleans up stack networks and containers.
  - `compose restart`: Restarts all services within the stack.
  - `compose pull`: Updates service images to latest tags.
  - `compose logs`: Fetches live tail output from all services in the active stack.
- **Multi-Stack Directory Management**: Manages stacks in `/etc/m3tal/compose/` or user-defined directories.
- **Template DB Integration**: Ready to import pre-configured templates from `m3tal-stacks`.

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
