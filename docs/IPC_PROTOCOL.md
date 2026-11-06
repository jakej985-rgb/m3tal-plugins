# M3TAL Plugin IPC Protocol

This document describes the plugin contract implemented by M3tal Core today. The authoritative Go types are in `M3tal-core/pkg/plugins/protocol.go`; plugin manifests are validated against `m3tal.io/v1alpha1`.

## Transport

- Transport: newline-delimited JSON-RPC 2.0 over the plugin process's stdin/stdout.
- The plugin is launched as a child process by Core.
- Core writes one JSON object per line to stdin and reads one JSON object per line from stdout.
- Plugin logs must go to stderr; stdout is reserved for protocol messages.
- Core injects environment variables including `M3TAL_API_KEY`, `M3TAL_API_TOKEN`, `M3TAL_CORE_VERSION`, `M3TAL_PLUGIN_DIR`, and `M3TAL_PROCESS_NAME`.

## Manifest

Every installable plugin needs `plugin.yaml`:

```yaml
apiVersion: m3tal.io/v1alpha1
kind: Plugin
metadata:
  name: example
  version: "0.1.0"
  title: "Example"
  description: "What the plugin does"
  author: "you"
  license: "MIT"
spec:
  command: "./bin/m3tal-example"
  process_name: "m3tal-example"
  capabilities: [collector, actions, ui_view, settings]
  ui:
    entry: "ui/dist/index.html"
    route: "/example"
  rpc:
    protocol: "jsonrpc-2.0"
    transport: "stdio"
```

Core discovers manifests under configured plugin directories, including `/usr/lib/m3tal/plugins`, `/usr/local/lib/m3tal/plugins`, `~/.local/lib/m3tal/plugins`, and `~/.config/m3tal/plugins`.

## Core → plugin methods

### `m3tal.init`

Sent once after process startup.

Params:

```json
{
  "core_version": "0.24.6",
  "config": {}
}
```

Expected result:

```json
{
  "name": "example",
  "version": "0.1.0",
  "capabilities": ["collector", "actions"],
  "tabs": [],
  "actions": [],
  "widgets": [],
  "settings": []
}
```

`tabs`, `actions`, `widgets`, and `settings` may be declared statically in `plugin.yaml` or returned dynamically during init.

### `m3tal.collect`

Sent on Core's collection cycle for plugins with the `collector` capability.

Params:

```json
{ "timestamp": 1760000000 }
```

Expected result:

```json
{
  "status": "ok",
  "metrics": [
    {
      "name": "example_temperature",
      "value": 42.5,
      "unit": "C",
      "category": "hardware",
      "status": "ok"
    }
  ],
  "tables": [
    {
      "id": "example_items",
      "title": "Items",
      "headers": ["Name", "State"],
      "rows": [["alpha", "running"]]
    }
  ],
  "message": ""
}
```

`status` should be `ok`, `degraded`, or `error`.

### `m3tal.action`

Sent when a user or API client executes a declared action.

Params:

```json
{
  "action_id": "restart_service",
  "target": "optional-target",
  "args": {
    "force": true
  }
}
```

Expected result:

```json
{
  "success": true,
  "message": "Service restarted",
  "error": "",
  "data": {},
  "payload": {}
}
```

`payload` preserves plugin-specific fields for the GUI/API.

## Lifecycle and shutdown

Core starts the process, calls `m3tal.init`, periodically calls `m3tal.collect`, and calls `m3tal.action` on demand. On stop, Core closes the plugin's stdin, sends SIGINT, and escalates if needed. A plugin may implement its own `m3tal.shutdown` handler for cleanup, but Core's host currently does not send that method as part of its normal stop path.

## Error handling

Use JSON-RPC error objects for protocol failures:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "error": {
    "code": -32601,
    "message": "Unknown action",
    "data": "Unsupported action_id: example"
  }
}
```

Recommended codes:

- `-32700`: parse error
- `-32600`: invalid request
- `-32601`: method not found / unknown action
- `-32602`: invalid params
- `-32603`: internal error

## Security rules

- Never log secrets or API tokens to stdout/stderr in normal operation.
- Treat `M3TAL_API_KEY` as sensitive; it is injected for Core API access.
- Actions that mutate host state should require explicit user confirmation in the UI/API layer.
- Plugins that need root must declare it honestly and document why.
