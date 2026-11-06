# m3tal-mesh-node Documentation

`m3tal-mesh-node` is the headless telemetry broadcaster plugin for servers, NAS devices, and secondary machines.

## Identity

- Plugin name: `mesh-node`
- Process: `m3tal-mesh-node`
- Package: `m3tal-plugin-mesh-node`
- Route: `/mesh-node` (settings/status)
- Status: available

## Capabilities

`collector`, `actions`, `ui_view`, `settings`

## Actions

`get_telemetry`, `beacon_ping`, `set_settings`, `get_settings`

## Settings

Pairing token/password, authentication requirement, telemetry port `9321`, and mDNS broadcast.

## Build and release

```bash
make build PLUGIN=m3tal-mesh-node
make test PLUGIN=m3tal-mesh-node
make audit PLUGIN=m3tal-mesh-node
make package PLUGIN=m3tal-mesh-node
```

`m3tal-mesh-node` is released by the shared `Build-m3tal-mesh-Deb.yml` workflow together with `m3tal-mesh` and the cluster metapackage.

See the plugin [`README.md`](../README.md), [`plugin.yaml`](../plugin.yaml), and repository [`docs/IPC_PROTOCOL.md`](../../docs/IPC_PROTOCOL.md).
