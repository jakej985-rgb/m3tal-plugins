# m3tal-mesh Documentation

`m3tal-mesh` is the fleet federation reader/dashboard plugin for M3tal Core. It pairs with `m3tal-mesh-node` senders and is bundled for full peers by `m3tal-plugin-mesh-cluster`.

## Identity

- Plugin name: `mesh`
- Process: `m3tal-mesh`
- Packages: `m3tal-plugin-mesh`, `m3tal-plugin-mesh-cluster`
- Route: `/mesh`
- Status: available

## Capabilities

`collector`, `actions`, `ui_view`, `settings`

## Actions

`pair_node`, `test_node`, `update_node`, `forget_node`, `ping_node`, `scan_network`, `set_settings`, `get_fleet`, `get_peers`, `get_settings`

## Settings

Pairing token/password policy, default node port `9321`, and LAN mDNS discovery.

## Build and release

```bash
make build PLUGIN=m3tal-mesh
make test PLUGIN=m3tal-mesh
make audit PLUGIN=m3tal-mesh
make package PLUGIN=m3tal-mesh
```

`Build-m3tal-mesh-Deb.yml` intentionally builds `m3tal-mesh`, `m3tal-mesh-node`, and the `m3tal-plugin-mesh-cluster` metapackage together so cluster versions stay aligned.

See the plugin [`README.md`](../README.md), [`plugin.yaml`](../plugin.yaml), and repository [`docs/IPC_PROTOCOL.md`](../../docs/IPC_PROTOCOL.md).
