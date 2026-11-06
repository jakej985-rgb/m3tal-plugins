# m3tal-compose Documentation

`m3tal-compose` is the Docker Compose editor and stack lifecycle plugin for M3tal Core.

## Identity

- Plugin name: `compose`
- Process: `m3tal-compose`
- Package: `m3tal-plugin-compose`
- Route: `/containers` (`Compose` tab)
- Status: available

## Capabilities

`collector`, `actions`, `ui_view`, `settings`

## Actions

`set_settings`, `compose_up`, `compose_down`, `compose_restart`, `save_stack`, `validate_yaml`, `get_logs`

## Settings

Compose search folder, new-stack layout, and backup-on-save behavior. `M3TAL_COMPOSE_DIR` can override the configured folder.

## Build and release

```bash
make build PLUGIN=m3tal-compose
make test PLUGIN=m3tal-compose
make audit PLUGIN=m3tal-compose
make package PLUGIN=m3tal-compose
```

Release workflow: `.github/workflows/Build-m3tal-compose-Deb.yml`

See the plugin [`README.md`](../README.md), [`plugin.yaml`](../plugin.yaml), and repository [`docs/IPC_PROTOCOL.md`](../../docs/IPC_PROTOCOL.md).
