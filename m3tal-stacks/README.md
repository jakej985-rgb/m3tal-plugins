# M3TAL Compose Stacks Plugin (`m3tal-stacks`)

[![Status: Planned](https://img.shields.io/badge/Status-Planned-orange.svg)](https://m3tal.io)
[![Process Identity: m3tal-stacks](https://img.shields.io/badge/Process-m3tal--stacks-brightgreen.svg)](https://m3tal.io)
[![Package: m3tal-plugin-stacks](https://img.shields.io/badge/Package-m3tal--plugin--stacks-blue.svg)](https://m3tal.io)

The **M3TAL Compose Stacks Plugin** (`m3tal-stacks`) manages multi-service Docker Compose stacks, automated Git repository synchronization for stack declarations, live inter-service dependency health checks, and one-click stack lifecycle deployments.

---

## 1. Architectural Overview & Identity

- **Canonical Process Identity**: `m3tal-stacks`
- **Package Name**: `m3tal-plugin-stacks`
- **Dashboard UI Route**: `/stacks`
- **Capabilities**: `orchestrator`, `actions`, `ui_view`

---

## 2. Planned Features & Scope

- **Multi-Service Compose Engine**: Native parsing and execution of `docker-compose.yml` / `compose.yaml` files.
- **Git-Ops Stack Sync**: Continuously watch a Git repository or local directory for stack definition updates and trigger rolling deployments.
- **Dependency Topology Graph**: Visual DAG of service dependencies (`depends_on`), network bindings, and volume mounts.
- **Stack Lifecycle Controls**: Up, Down, Restart, Rebuild, and Pull latest images across all services in a stack.
- **Environment Variable Vault**: Per-stack `.env` management with encrypted storage for secrets.

---

## 3. IPC Methods Roadmap

| Method | Description |
|---|---|
| `m3tal.init` | Handshake declaring stack orchestration capabilities and UI endpoints. |
| `m3tal.collect` | Collects active stacks, service statuses, health checks, and container mapping. |
| `m3tal.action` | Dispatches `stack_up`, `stack_down`, `stack_restart`, and `stack_pull`. |
| `m3tal.shutdown` | Graceful shutdown without interrupting running background compose services. |

---

## 4. Debian Installation (When Released)

```bash
sudo apt update
sudo apt install -y m3tal-plugin-stacks
```
