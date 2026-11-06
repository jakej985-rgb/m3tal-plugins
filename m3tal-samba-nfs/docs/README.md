# m3tal-samba-nfs Documentation

`m3tal-samba-nfs` is the Samba/NFS share management plugin for M3tal Core. It is implemented but still developing and requires privileged host access.

## Identity

- Plugin name: `samba-nfs`
- Process: `m3tal-samba-nfs`
- Package: `m3tal-plugin-samba-nfs` *(planned)*
- Route: `/storage/shares`
- Status: developing

## Capabilities

`collector`, `actions`, `ui_view`, `settings`

## Actions

`get_shares`, `create_smb_share`, `delete_smb_share`, `create_nfs_export`, `delete_nfs_export`, `create_smb_user`, `delete_smb_user`, `set_smb_password`, `mount_share`, `unmount_share`, `restart_services`, `get_settings`, `set_settings`

## Settings

Samba config path, NFS exports path, and auto-reload behavior. Environment overrides: `M3TAL_SMB_CONF`, `M3TAL_EXPORTS`.

## Build and release

```bash
make build PLUGIN=m3tal-samba-nfs
make test PLUGIN=m3tal-samba-nfs
make test-ipc PLUGIN=m3tal-samba-nfs
make audit PLUGIN=m3tal-samba-nfs
make package PLUGIN=m3tal-samba-nfs
```

Release workflow: `.github/workflows/Build-m3tal-samba-nfs-Deb.yml`

See the plugin [`README.md`](../README.md), [`plugin.yaml`](../plugin.yaml), and repository [`docs/IPC_PROTOCOL.md`](../../docs/IPC_PROTOCOL.md).
