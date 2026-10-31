# Changelog

All notable changes to the **Mesh Fleet & Federation** (`m3tal-mesh`) plugin for M3TAL Core will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [0.3.0] - 2026-10-31

### 🚀 Changes & Improvements
- **fix(mesh): resolve review-orchestrator findings for catalog, schemas, auth, and credentials** (`0dfa5cb`)
- **fix(mesh): rollback peer persistence if saving pairing token fails** (`39d1a5e`)
- **fix(mesh): drop unneeded clear_token/clear_password parsing in test_node** (`94bdf93`)
- **fix(mesh): renegotiate test_node probe mode when test credentials change** (`93fa26a`)
- **fix(mesh): drop redundant prevPasswordOnly save/restore in update_node** (`c9eae7e`)
- **fix(mesh): renegotiate probe mode on credential changes and use isPairing directly** (`936854d`)
- **fix(mesh): persist paired peer before updating pairing token and introduce persistPairedPeer** (`d7d3935`)
- **fix(mesh): check pairing ID conflicts atomically under peerMutationMu in persistPeerUpsert** (`ef28e43`)
- **fix(mesh): detect node ID conflicts in pairing path to prevent overwriting moved peers** (`8ca09cd`)
- **fix(mesh): honor clear_password in update_node, keep DefaultPassword dynamic on peers, and clean up effectiveCredentials** (`77722dc`)
- **fix(mesh): require passwords configured on peers and remove DefaultPassword from effectiveCredentials** (`c12442c`)
- **fix(mesh): persist peer fingerprint safely and restrict DefaultPassword fallback** (`6623206`)
- **fix(mesh): negotiate and retain pin during refresh and ping** (`4388952`)
- **fix(mesh): clear discovery credentials on switch, drop redundant cfg reload, and clean up test coverage** (`557a7a5`)
- **fix(mesh): normalize node addresses on update/test and restrict DefaultPassword to mesh token peers** (`aa2c3fe`)
- **fix(mesh): block DefaultPassword retry for custom token peers and preserve PasswordOnly mode on probe failure** (`c767d89`)
- **fix(mesh): restrict DefaultPassword fallback to mesh token peers and make passwordOnly non-variadic** (`cf8445f`)
- **fix(mesh): pin authenticated fingerprint on retry and enforce non-variadic fingerprint** (`30111d8`)
- **fix(mesh): withhold DefaultPassword across all paths unless fingerprint is pinned and add complete test coverage** (`b2eabe4`)
- **fix(mesh): protect default password from exfiltration, clear stored credentials on address change, and harden test assertions** (`edbf88d`)
- **fix(mesh): restrict saved credentials fallback in test_node strictly to matching IP** (`65a8ef0`)
- **fix(mesh): reuse saved credentials in test_node on address change and wrap ErrTokenRejected on unsigned response** (`5b238eb`)
- **fix(mesh): preserve PasswordOnly on probe failure, remove persistPeerUpsert override, and do not wrap ErrTokenRejected on proof failure** (`f6fb1c8`)
- **fix(mesh): wrap ErrTokenRejected sentinel, negotiate mode on credential change, and fix PasswordOnly preservation** (`34be222`)
- **fix(mesh): unify connection negotiation helper, infer PasswordOnly, and serialize to disk** (`605553d`)
- **fix(mesh): authenticate password-required discovery candidates with HMAC proof and support secondary password inheritance** (`a957e88`)
- **fix(mesh): handle password-required candidates using typed sentinel error for explicit pairing** (`b29bc36`)
- **fix(mesh): restrict discovery strictly to token-authenticated candidates** (`276f1e1`)
- **fix(mesh): unify credential selection helper, remove PasswordOnly field, and present password-required candidates** (`feba96b`)
- **fix(mesh): resolve password-only inheritance, discovery password leak, per-node tokens, and MeshNode state duplication** (`9c5aeac`)
- **fix(mesh): restore discovery results container display in scanNetwork** (`47fdddc`)
- **fix(mesh): remove clear_token alias, restore IsPairingTokenRequired helper, and enforce HMAC auth** (`a07070e`)
- **fix(mesh): remove alias sprawl and standardize on pairing_token and require_pairing_token** (`f549567`)
- **fix(mesh): verify TLS certificate during handshake before sending credentials** (`3d534a0`)
- **fix(mesh): restrict saved credentials to matching IP in test_node, pin fingerprint with password, and avoid stale credentials on IP reuse** (`d959231`)
- **fix(mesh): support peer re-pairing and refactor discovery result clearing** (`7317a30`)
- **fix(mesh): reject duplicate peer IP on update and clear discovery results in edit mode** (`799ce8f`)
- **fix(mesh): pass default password in discovery and preserve inherited credentials on pair** (`9a32366`)
- **fix(mesh): unpin fingerprint on address change and clean up pairing API** (`bbba39d`)
- **fix(mesh): resolve security review findings for credentials, challenge auth, and UI elements** (`a247eff`)
- **feat(mesh): support REST API auth with optional password and add remote connections card** (`c5d0664`)

---

## [0.2.0] - 2026-10-31

### 🚀 Changes & Improvements
- **merge: resolve changelog conflicts with origin/main** (`7dd0e07`)
- **refactor(mesh): rely exclusively on nil-safe accessors for default configuration flags** (`9afbf1d`)
- **refactor(mesh): remove redundant fingerprint check and unreachable refresh error branch** (`a279f89`)
- **fix(mesh): prioritize replacement tokens, enforce policy on tokenless fetches, and share response parsing tail** (`2692e32`)
- **fix(mesh): preserve tokens on blank settings save, wire discovery toggle, and optimize unauthenticated flow** (`d13a6f8`)
- **feat(mesh): enforce pairing token by default and add settings to make optional with safety warnings** (`5da9b8f`)

---

## [Unreleased]
### Added
- Register `settings` capability and `spec.settings` in `plugin.yaml` so Mesh Fleet appears under M3TAL Core's "System Settings & Configurations → PLUGIN SETTINGS".
- Add `require_pairing_token` setting (default `true`), enforcing Pairing Token / Secret authentication by default across modal and background synchronization.
- Provide a configuration toggle to make Pairing Token / Secret optional with prominent safety warnings across Settings and the Pair Node modal.

---

## [0.1.5] - 2026-10-31

### 🚀 Changes & Improvements
- **fix(mesh): bind challenge-response proofs to TLS certificate fingerprints and client IP** (`8b2152b`)
- **fix(mesh): refresh mesh settings after pairing to update has_pairing_token** (`927c7f6`)
- **fix(mesh): restore payload merging in UI readers, restore node smoke tests, and format network.go** (`294ccd2`)
- **fix(mesh): read flat action responses in UI and assert full response shape in tests** (`3d728e3`)
- **refactor(mesh): emit flat action responses and read payload cleanly in UI** (`af69661`)
- **style(mesh): restore script indentation and fallback error messages** (`84affb4`)
- **fix(mesh): align action payload responses and ui readers with core passthrough** (`222bf33`)
- **fix(mesh): use flat action response data** (`746dc13`)
- **fix(mesh): improve UI error handling and cleanup** (`e6ca0ca`)
- **fix: handle nested payload data structures in mesh UI API responses** (`47a5a35`)
- **fix(mesh): flatten get_settings response and add test coverage** (`458404c`)
- **fix(mesh): use success:true in get_settings response to match ActionResult decoder** (`cd9e56e`)

---

## [0.1.4] - 2026-10-28
### Fixed
- Convert LAN discovery to concurrent verification to prevent slow peers from blocking discovery

## [0.1.3] - 2026-10-28
### Fixed
- Add client-issued nonce to pairing protocol to prevent replay attacks
- Implement per-client rate limiting on challenge requests (16 per client IP)
- Fix concurrent re-pair metadata preservation during peer refresh
- Configure HTTP client with connection pooling and proper timeouts

## [0.1.2] - 2026-10-27
### Added
- Mesh Fleet includes its own pairing settings view and stores its token and paired peers with owner-only file permissions.
- Remote pairing verifies the node and token, refreshes live telemetry, supports mDNS discovery, and persists fleet changes.

## [0.1.1] - 2026-10-26

### 🚀 Removed Dummy Mock Nodes & Real Hardware Telemetry
- **Zero Dummy Data**: Completely removed hardcoded mock entries (`node-home-server`, `node-nas-storage`, `node-alpha-r2`) from backend and frontend.
- **Genuine Local Host Telemetry**: Local workstation node now dynamically queries `/proc/loadavg`, `/proc/meminfo`, `syscall.Statfs`, local IPv4 addresses, and NVIDIA GPU stats (`nvidia-smi`).
- **Clean Fleet Discovery**: Fleet view starts strictly with genuine local workstation telemetry and an empty peer list until remote `m3tal-mesh-node` peers are paired.

## [0.1.0] - 2026-10-26

### Overview & Highlights
Initial release of the **Mesh Fleet & Federation Reader** plugin (`m3tal-mesh`) for M3TAL Core. Provides personal cloud multi-node discovery, peer telemetry aggregation, and a distributed cluster dashboard.

### 🚀 Capabilities & Features
- **JSON-RPC 2.0 Engine**: Standard stdio event loop responding to `m3tal.init`, `m3tal.collect`, `m3tal.action`, and `m3tal.shutdown`.
- **Canonical Process Identity**: Executes with `prctl(PR_SET_NAME, "m3tal-mesh")` and compiled to `bin/m3tal-mesh` for direct host execution.
- **Cluster Fleet Dashboard**:
  - Aggregated cluster metrics: Total Nodes Online, Total CPU Cores, Total Memory, and Total ZFS/Disk Storage.
  - Per-node health cards with real-time CPU meters, RAM usage, NVMe/SATA mount gauges, and GPU telemetry.
  - Node Pairing Modal: Add remote servers by IP/hostname or discover local LAN nodes via mDNS beacons.
- **Node Lifecycle Actions**:
  - `pair_node`: Authenticates and registers a remote `m3tal-mesh-node` peer.
  - `forget_node`: Removes a disconnected or decommissioned node from the fleet view.
  - `ping_node`: Measures round-trip network latency to a specific peer.
  - `scan_network`: Scans local subnet for active beacons.
