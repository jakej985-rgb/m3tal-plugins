# M3TAL Docker Plugin (`m3tal-docker`)

[![Plugin Standard: v1alpha1](https://img.shields.io/badge/Plugin%20Standard-v1alpha1-blue.svg)](https://m3tal.io)
[![Process Identity: m3tal-docker](https://img.shields.io/badge/Process-m3tal--docker-brightgreen.svg)](https://m3tal.io)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

The **M3TAL Docker Plugin** (`m3tal-docker`) is the official container fleet manager and live telemetry monitor for the **M3TAL Core** Linux Supervisor. It provides real-time container metrics, live logs inspection, image pruning, and complete container lifecycle control (start, stop, restart, delete) with zero footprint inside `m3tal-core`.

---

## 1. Architectural Overview

```text
┌─────────────────────────────────────────────────────────────┐
│                       M3TAL Core Host                       │
│  Dynamic UI View (/containers)  ◄──►  Host Supervisor       │
└──────────────────────────────────────────────▲──────────────┘
                                               │ JSON-RPC 2.0
                                               │ (stdio pipe)
                                               ▼
┌─────────────────────────────────────────────────────────────┐
│                    m3tal-docker Plugin                      │
│                                                             │
│  • Canonical Process:   m3tal-docker                        │
│  • Micro-UI Bundle:     ui/dist/index.html                  │
│  • Socket Interface:    /var/run/docker.sock                │
│  • Execution Privs:     root: false (docker group)          │
└─────────────────────────────────────────────────────────────┘
```

- **Zero Core Footprint**: `m3tal-core` contains no Docker SDK or container-specific endpoints. All communication occurs over JSON-RPC 2.0 standard I/O.
- **Canonical Process Identity**: Always executes as `m3tal-docker` (via `prctl(PR_SET_NAME)`) for direct CPU and RAM tracking in `top`, `htop`, and system diagnostic monitors.
- **Micro-Frontend Integration**: Delivers pre-compiled static assets in `ui/dist/` mounted seamlessly into the host dashboard at `/containers`.
- **Rootless Security**: Connects directly to the host `/var/run/docker.sock` requiring only standard `docker` group membership.

---

## 2. Capabilities & IPC Methods

Declared in `plugin.yaml`:
- `collector`: Continuous telemetry of container counts, CPU/memory stats, and states.
- `actions`: Immediate operational commands for container management.
- `ui_view`: Dedicated web console embedded in the M3TAL dashboard.
- `websocket_stream`: Live log tailing and container event streaming.

### JSON-RPC 2.0 Stdio Contract

| Method | Direction | Description |
|---|---|---|
| `m3tal.init` | Request → Response | Handshake exchanging plugin version, capabilities, actions, and UI routes. |
| `m3tal.collect` | Request → Response | Periodic snapshot of running, paused, stopped containers, images, and disk usage. |
| `m3tal.action` | Request → Response | Dispatches operational tasks: `start`, `stop`, `restart`, `remove`, `prune_images`. |
| `m3tal.shutdown` | Request → Response | Gracefully terminates active background monitoring loops and flushes buffers. |

---

## 3. Directory Layout

```text
m3tal-docker/
├── .agents/
│   └── agents/
│       └── plugin-maintainer/
│           └── agent.md         # Dedicated AI custodian for m3tal-docker
├── bin/
│   └── m3tal-docker             # Compiled Go executable
├── config/
│   └── config.schema.json       # Settings validation schema
├── src/
│   ├── docker.go                # Docker Engine API client & actions
│   ├── go.mod                   # Go module definition
│   ├── main.go                  # JSON-RPC 2.0 loop & process naming
│   └── main_test.go             # Unit & IPC test coverage
├── ui/
│   └── dist/
│       └── index.html           # Micro-UI bundle
├── CHANGELOG.md                 # Semantic version history
├── Makefile                     # Build & verification targets
├── plugin.yaml                  # M3TAL Plugin v1alpha1 Manifest
└── README.md                    # This documentation
```

---

## 4. Development & Build Targets

Run all commands inside `m3tal-docker/` or from the repository root using `make <target> PLUGIN=m3tal-docker`:

```bash
# Build the backend executable (bin/m3tal-docker)
make build

# Run unit tests
make test

# Verify JSON-RPC 2.0 handshake, collection, action, and shutdown contracts
make test-ipc

# Clean build artifacts
make clean
```

---

## 5. Debian Packaging (`.deb`)

From the monorepo root:
```bash
./scripts/build-plugin-deb.sh m3tal-docker
```
Produces `m3tal-plugin-docker-monitor_<version>_amd64.deb` targeted for `/usr/lib/m3tal/plugins/m3tal-docker/`.
