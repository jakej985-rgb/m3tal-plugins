# M3TAL Samba & NFS Share Manager Plugin (`m3tal-samba`)

[![Status: Planned](https://img.shields.io/badge/Status-Planned-orange.svg)](https://m3tal.io)
[![Process Identity: m3tal-samba](https://img.shields.io/badge/Process-m3tal--samba-brightgreen.svg)](https://m3tal.io)
[![Package: m3tal-plugin-samba](https://img.shields.io/badge/Package-m3tal--plugin--samba-blue.svg)](https://m3tal.io)

The **M3TAL Samba & NFS Share Manager Plugin** (`m3tal-samba`) provides a graphical file sharing administrator for SMB/CIFS and NFS shares, managing user access credentials, guest permissions, filesystem ACLs, active client connections, and mounted network volumes.

---

## 1. Architectural Overview & Identity

- **Canonical Process Identity**: `m3tal-samba`
- **Package Name**: `m3tal-plugin-samba`
- **Dashboard UI Route**: `/storage/shares`
- **Capabilities**: `actions`, `ui_view`, `system_admin`

---

## 2. Planned Features & Scope

- **Visual Share Provisioning**: Create and modify SMB (`/etc/samba/smb.conf`) and NFS (`/etc/exports`) definitions with one click.
- **User & Group ACLs**: Manage Samba users, set read-only / read-write permissions, and configure public guest access safely.
- **Active Connection Monitor**: Real-time visibility into connected clients, IP addresses, open file handles, and locked files via `smbstatus`.
- **Time Machine Support**: One-click configuration of Apple Time Machine compatible backup shares (`vfs_fruit`).
- **Filesystem Mounts**: Mount and unmount remote network shares (SMB/NFS) directly to local mount points.

---

## 3. IPC Methods Roadmap

| Method | Description |
|---|---|
| `m3tal.init` | Handshake reporting installed Samba/NFS packages and active service states. |
| `m3tal.collect` | Collects active shares, connected clients, open file locks, and disk quotas. |
| `m3tal.action` | Dispatches `create_share`, `delete_share`, `create_user`, and `restart_services`. |
| `m3tal.shutdown` | Graceful shutdown without dropping active client connections. |

---

## 4. Debian Installation (When Released)

```bash
sudo apt update
sudo apt install -y m3tal-plugin-samba
```
