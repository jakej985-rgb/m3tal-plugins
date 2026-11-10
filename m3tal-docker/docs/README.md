# m3tal-docker Documentation

`m3tal-docker` is the official Docker container fleet plugin for M3tal Core.

## Identity

- Plugin name: `docker-monitor`
- Process: `m3tal-docker`
- Package: `m3tal-plugin-docker-monitor`
- Route: `/containers`
- Status: available

## Capabilities

`collector`, `actions`, `ui_view`, `ui_widget`, `settings`

## Actions

`start_container`, `stop_container`, `restart_container`, `prune_images`, `get_logs`, `inspect_container`, `set_settings`

## Settings

Docker host endpoint, default log tail, stopped-container visibility, and metrics poll interval.

## Build and release

```bash
make build PLUGIN=m3tal-docker
make test PLUGIN=m3tal-docker
make audit PLUGIN=m3tal-docker
make package PLUGIN=m3tal-docker
```

Release workflow: `.github/workflows/Build-m3tal-docker-Deb.yml`

See the plugin [`README.md`](../README.md), [`plugin.yaml`](../plugin.yaml), and repository [`docs/IPC_PROTOCOL.md`](../../docs/IPC_PROTOCOL.md).
