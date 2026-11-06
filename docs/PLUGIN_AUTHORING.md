# M3TAL Plugin Authoring Guide

This guide explains how to build a plugin that works with M3tal Core's current implementation. For the wire protocol, see [`IPC_PROTOCOL.md`](./IPC_PROTOCOL.md). For release packaging, see [`RELEASING.md`](./RELEASING.md).

## What a plugin is

A plugin is an external process managed by Core. It owns workload-specific behavior and communicates with Core through JSON-RPC 2.0 over stdio. Core remains responsible only for the dashboard, host telemetry, settings/API keys, plugin discovery, lifecycle, and UI mounting.

Do not put Docker, Compose, mesh, VPN, proxy, storage, AI, or benchmark logic into Core. Put it in a plugin.

## Minimum viable plugin

```text
m3tal-example/
├── plugin.yaml
├── README.md
├── CHANGELOG.md
├── Makefile
├── config/
│   └── config.schema.json
├── src/
│   ├── go.mod
│   └── main.go
├── ui/
│   └── dist/
│       └── index.html
└── .agents/
    └── agents/
        └── plugin-maintainer/
            └── agent.md
```

For Go plugins, the binary should be built to `bin/m3tal-<name>` and `plugin.yaml` should use `command: "./bin/m3tal-<name>"`.

## Manifest requirements

Use `apiVersion: m3tal.io/v1alpha1` and `kind: Plugin`.

Required practical fields:

- `metadata.name`: stable plugin ID used by Core and the catalog.
- `metadata.version`: semantic version without a `v` prefix.
- `metadata.title` and `metadata.description`: shown in Core/catalog UI.
- `spec.command`: executable path relative to the plugin directory.
- `spec.process_name`: canonical process name, normally `m3tal-<name>`.
- `spec.capabilities`: only capabilities actually implemented.
- `spec.ui`: required for dashboard views/widgets.
- `spec.settings`: required for configurable plugins.
- `spec.rpc`: declare `jsonrpc-2.0` and `stdio`.

Supported Core capabilities today:

- `collector`
- `ui_view`
- `ui_widget`
- `actions`
- `settings`
- `exporter`

Do not declare unsupported capability names such as `websocket_stream`, `inference_runner`, or `job_queue` unless Core support is added first.

## Process naming

Set the process name to the canonical `m3tal-<name>` value. Go plugins can use `prctl(PR_SET_NAME)` or the repo's existing naming helper so `top`, `htop`, systemd-cgls, and Core process views show the canonical name.

## UI guidance

Core mounts plugin UI under `/plugins/<metadata.name>/` and registers `spec.ui.route` as the dashboard route. UI bundles should be static files under `ui/dist` and should not depend on Core internals beyond documented APIs.

Keep UI routes unique. Compose currently shares the `/containers` route with Docker but uses a separate tab ID; new plugins should prefer a dedicated route unless intentionally contributing a tab to an existing area.

## Settings guidance

Declare settings in `plugin.yaml` and persist plugin-specific values under Core's plugin config directory. Read environment overrides for tests, but document them in the plugin README.

## Actions guidance

Every action must be declared in `m3tal.init` or static manifest data before the GUI/API exposes it. Destructive or host-changing actions should set `confirm: true` where supported and must validate arguments server-side in the plugin.

## Testing checklist

Before release:

1. `plugin.yaml` parses and uses `m3tal.io/v1alpha1`.
2. Binary builds with the documented Go/Node versions.
3. `m3tal.init` returns name/version/capabilities/tabs/actions/settings.
4. `m3tal.collect` returns valid `status`, `metrics`, and `tables`.
5. Every documented action returns `ActionResult`.
6. UI loads from `/plugins/<name>/` and route registration appears in `/api/v2/plugins/tabs`.
7. `make audit PLUGIN=<name>` passes repository checks.
8. Debian package installs, starts, collects, and stops cleanly.

## Security checklist

- Do not run as root unless required; document the reason.
- Validate all action arguments.
- Do not expose secrets in metrics, tables, logs, or UI.
- Treat `M3TAL_API_KEY` as sensitive.
- For plugins that mutate host services, require confirmation and produce clear errors.
