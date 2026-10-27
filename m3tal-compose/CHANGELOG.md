# Changelog

All notable changes to the **Compose Editor** (`m3tal-compose`) plugin for M3TAL Core will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [0.4.0] - 2026-10-26

### Added
- Configurable recursive Compose project discovery, defaulting to `~/docker`.
- Settings for new stack storage layout and backups.
- Searchable project list, source paths, runtime service states, responsive layout, and edit/stop confirmations.

### Changed
- Existing projects save to their discovered Compose file; new stacks use the selected folder or flat-file layout.
- Compose actions retain project names and all registered config files.
- YAML validation and service extraction use a YAML parser.

### Fixed
- Stack names are validated before paths are created, and saved files are replaced atomically.
- Replacements and backups preserve existing Compose file permissions, including mode 0600.
- Existing projects are selected by their Compose file path, preventing display-name collisions from targeting the wrong file.
- Local Compose actions leave declared and interpolated project names to Docker Compose.
- Compose discovery follows a symlinked search root, filters unrelated and override YAML files, and keeps duplicate names deterministic.
- Settings updates validate all values before applying any of them, and action lookup avoids a full discovery/status scan.
- Registered secondary Compose files remain distinct save targets and are not listed as standalone projects.
- Compose projects under ordinary folders named `dev`, `tmp`, or similar remain discoverable.
- New project selection follows the saved file ID after refresh, and stack IDs are assigned in one place.
- Flat-layout saves return the path actually written, even when a folder with the same name exists.
- Save responses no longer retarget the editor after selection changes, and stale responses keep newer edits marked unsaved.
- Registered projects sharing a configuration file receive distinct action IDs tied to their Compose project names.
- Invalid initialization settings are reported to stderr.
- Existing symlinked Compose files are replaced through their resolved targets, preserving the link and backing up the target.
- Refresh rebinds a registered project selection to its local file identity after its Docker registry entry disappears.
- SaveStack returns the actual path written, which is used directly for the UI's saved project identity.
- Symlinked projects retain their discovered link identity in the UI while saves update the resolved target.
- Removed the declared but unimplemented `auto_pull` option and synchronized the plugin version.

## [0.3.0] - 2026-10-26

### 🚀 Changes & Improvements
- **feat(mesh): add m3tal-mesh reader and m3tal-mesh-node telemetry broadcaster** (`31db524`)

---

## [0.2.2] - 2026-10-26

### 🚀 Capabilities & Enhancements
- **feat(discovery): host-wide compose stack discovery**: Integrated `docker compose ls -a --format json` in `ListStacks()` to automatically discover and monitor compose stacks running anywhere on the host system, not just within `/etc/m3tal/compose/`.
- **fix(discovery): resolve dynamic stack filepaths**: Added `resolveStackPath()` to dynamically map lifecycle actions (`compose_up`, `compose_down`, `compose_restart`, `get_logs`) to the exact file path of any discovered stack across the machine.
- **fix(safety): eliminate mock data leak in production**: Fixed `ListStacks()` so it cleanly returns an empty collection when no compose projects exist, rather than leaking phantom mock stacks.
- **feat(ui): empty state handling & version sync**: Added descriptive empty state in `ui/dist/index.html` when no compose stacks are present and synced version badges to `0.2.2`.
- **chore(metadata): catalog & manifest registration**: Added explicit `package_name: "m3tal-plugin-compose"` to `plugin.yaml` and registered `compose` in root `catalog.json`.

---

## [0.2.1] - 2026-10-24

### 🚀 Changes & Improvements
- **feat(tabs): align under Containers category with Compose subtab**: Grouped tab under `containers` category with title `Compose` to render as a top sub-tab alongside `Fleet`.

---

## [0.2.0] - 2026-10-24

### 🚀 Changes & Improvements
- **feat(compose): add m3tal-compose editor plugin and realign m3tal-stacks roadmap** (`07d2a48`)

---

## [0.1.0] - 2026-10-24

### Overview & Highlights
Initial release of the standardized Docker Compose Editor & Stack Manager plugin (`m3tal-compose`) for M3TAL Core. Provides visual/YAML editing, syntax verification, and stack lifecycle deployment controls.

### 🚀 Capabilities & Features
- **JSON-RPC 2.0 Engine**: Standard stdio event loop responding to `m3tal.init`, `m3tal.collect`, `m3tal.action`, and `m3tal.shutdown`.
- **Canonical Process Identity**: Executes with `prctl(PR_SET_NAME, "m3tal-compose")` and compiled to `bin/m3tal-compose` for direct host execution without `/tmp` symlinks.
- **Compose Stack Management**:
  - `compose_up`: Deploys stacks in detached mode (`docker compose up -d`).
  - `compose_down`: Shuts down active stack services and networks.
  - `compose_restart`: Restarts all stack services.
  - `save_stack`: Writes validated YAML to disk.
  - `validate_yaml`: Validates Compose schema syntax before deployment.
- **Dynamic Micro-Frontend (`ui/dist/index.html`)**:
  - Dark-mode syntax-highlighted YAML editor and service visualizer.
  - Quick action toolbar (`Deploy`, `Stop`, `Restart`, `Validate`, `Logs`).
  - Terminal log drawer for live deployment output.
- **Debian Packaging**: Automated Debian `.deb` packaging via `scripts/build-plugin-deb.sh`.
