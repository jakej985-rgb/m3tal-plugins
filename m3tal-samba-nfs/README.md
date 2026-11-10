# M3TAL Samba & NFS Share Manager Plugin (`m3tal-samba-nfs`)

[![Status: Developing](https://img.shields.io/badge/Status-Developing-yellow.svg)](https://m3tal.io)
[![Process Identity: m3tal-samba-nfs](https://img.shields.io/badge/Process-m3tal--samba--nfs-brightgreen.svg)](https://m3tal.io)
[![Package: m3tal-plugin-samba-nfs](https://img.shields.io/badge/Package-m3tal--plugin--samba--nfs-blue.svg)](https://m3tal.io)

The **M3TAL Samba & NFS Share Manager Plugin** (`m3tal-samba-nfs`) provides a graphical file sharing administrator for SMB/CIFS and NFS shares: share provisioning, Samba user credentials, guest permissions, active client monitoring, locked-file visibility, and network share mounts.

---

## 1. Architectural Overview & Identity

- **Canonical Process Identity**: `m3tal-samba-nfs`
- **Package Name**: `m3tal-plugin-samba-nfs`
- **Dashboard UI Route**: `/storage/shares`
- **Capabilities**: `collector`, `actions`, `ui_view`, `settings`
- **Privilege**: Runs as root — required to write `/etc/samba/smb.conf`, `/etc/exports`, manage `smbpasswd`/`pdbedit` accounts, and perform mounts.

---

## 2. Features & Scope

- **SMB Share Management**: Create/delete `[share]` sections in `smb.conf` with path, comment, read-only, guest access, and `valid users`. Validated with `testparm` when available; config backed up before every write.
- **NFS Export Management**: Append/remove `client(options)` entries in `/etc/exports`, activated via `exportfs -ra`.
- **Samba Users**: Create/update accounts via `smbpasswd -a -s`, delete via `pdbedit -x`.
- **Active Connection Monitor**: Live SMB sessions and locked files parsed from `smbstatus`; NFS client visibility via `showmount -a`.
- **Network Mounts**: Mount/unmount remote CIFS (SMB) and NFS shares to local mount points.
- **Service Control**: Reload/restart `smbd`, `nmbd`, and `nfs-server`.

---

## 3. IPC Methods

| Method | Description |
|---|---|
| `m3tal.init` | Handshake reporting installed Samba/NFS packages, service states, and config paths. |
| `m3tal.collect` | Collects shares, Samba users, SMB sessions, locked files, and NFS clients plus metrics/tables. |
| `m3tal.action` | Dispatches share/user/mount/service actions (see below). |
| `m3tal.shutdown` | Graceful shutdown. |

### Actions

`get_shares`, `create_smb_share`, `delete_smb_share`, `create_nfs_export`, `delete_nfs_export`, `create_smb_user`, `delete_smb_user`, `set_smb_password`, `mount_share`, `unmount_share`, `restart_services`, `get_settings`, `set_settings`.

---

## 4. Configuration

Plugin settings live at `~/.config/m3tal/plugins/samba-nfs/config.json`:

- `smb_conf_path` — path to `smb.conf` (default `/etc/samba/smb.conf`)
- `exports_path` — path to NFS exports file (default `/etc/exports`)
- `auto_reload` — reload/restart services after changes (default `true`)

Environment overrides for testing/nonstandard layouts: `M3TAL_SMB_CONF`, `M3TAL_EXPORTS`.

---

## 5. Debian Installation (When Released)

```bash
sudo apt update
sudo apt install -y m3tal-plugin-samba-nfs
```
