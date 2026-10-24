# Changelog

All notable changes to the **Containers** (`docker-monitor`) plugin for M3TAL Core will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [0.1.2] - 2026-10-24

### 🚀 Changes & Improvements
- **refactor(layout): standardize directory structure to `m3tal-docker`**: Migrated repository directory layout to canonical `m3tal-<name>` naming scheme.
- **feat(ui): dynamic standalone micro-frontend**: Enhanced `ui/dist/index.html` to live-poll M3TAL Core API (`/api/v2/plugins/docker-monitor/collect`), render real-time container metrics cards and status table, trigger lifecycle actions (`start`, `stop`, `restart`), and view logs.
- **build(go): update Go module path**: Updated module definition in `src/go.mod` to `github.com/jakej985-rgb/m3tal-plugins/m3tal-docker`.
- **refactor(docker-monitor): purge legacy tui_widget and m3tal.render handler** (`be7e866`)

---

## [0.1.1] - 2026-10-24

### 🚀 Changes & Improvements
- **docs(plugins): require and package CHANGELOG.md for all plugins (added to docker-monitor)** (`361edac`)
- **refactor(plugins): purge legacy `tui_widget` capability and `m3tal.render` handler**: Align with the zero-TUI / minimal-CLI platform architecture; all presentation is now exclusively handled by dynamic Web GUI micro-frontends (`ui_view`).

---

## [0.1.0] - 2026-10-24

### Overview & Highlights
Initial release of the standardized standalone Docker management plugin for the M3TAL Core platform (`jakej985-rgb/m3tal-core`), fully extracted and decoupled to fulfill the **Zero Core Footprint** architectural mandate.

### 🚀 Capabilities & Features
- **JSON-RPC 2.0 IPC Protocol**:
  - Implemented standard stdio event loop handling `m3tal.init`, `m3tal.collect`, `m3tal.render`, `m3tal.action`, and `m3tal.shutdown`.
- **High-Usage Attribution & Canonical Process Identity**:
  - Bootstraps with Linux `prctl(PR_SET_NAME, "m3tal-docker")` so system monitors (`top`, `htop`, `m3tal-sysmon`) attribute CPU, memory, and disk I/O usage directly to `m3tal-docker`.
- **Telemetry Collection (`m3tal.collect`)**:
  - Emits real-time metrics for total, running, and stopped containers.
  - Generates structured table payloads (`docker_ps`) with ID, Name, Image, Status, Ports, State, CPU%, and Memory usage.
- **Action Dispatcher (`m3tal.action`)**:
  - Supports container lifecycle controls: `start_container`, `stop_container`, `restart_container`.
  - Image maintenance: `prune_images` reclaiming space from dangling images.
  - Streaming log retrieval: `get_logs` fetching tail logs with configurable lines.
- **Terminal & TUI Rendering (`m3tal.render`)**:
  - Formatted plain text dashboard table for CLI and TUI integration.
- **Dynamic Micro-Frontend Web UI (`ui/dist/index.html`)**:
  - Dark-mode interface matching M3TAL Core design system (`#0d1117`, `#161b22`, emerald accents).
  - Live metric summary cards for running, stopped, and total containers.
  - Interactive container fleet table with real-time status badges, port mappings, and action triggers.
  - Sliding terminal console drawer for real-time log inspection.
- **Fail-Safe Operation**:
  - Automatically activates simulated mock telemetry if the Docker socket is unreachable, preventing supervisor crashes.

### 📦 Debian Packaging & Distribution
- Automated `.deb` packaging via `scripts/build-plugin-deb.sh` producing `m3tal-plugin-docker-monitor_0.1.0_amd64.deb`.
- Target deployment directory: `/usr/lib/m3tal/plugins/docker-monitor/` and `/etc/m3tal/plugins/docker-monitor/`.

### 🤖 Custodian Agent
- Embedded dedicated maintainer agent at `.agents/agents/plugin-maintainer/agent.md` for continuous maintenance, audit compliance, and feature iteration.
