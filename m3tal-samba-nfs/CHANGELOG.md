# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-11-04

### Added
- Initial release of `m3tal-samba-nfs`, the M3TAL Samba & NFS Share Manager.
- JSON-RPC 2.0 stdio IPC (`m3tal.init`, `m3tal.collect`, `m3tal.action`, `m3tal.shutdown`).
- SMB share management: list/create/delete shares in `smb.conf`, Samba user management via `pdbedit`/`smbpasswd`, `smbstatus` connection and locked-file monitoring.
- NFS export management: list/create/delete entries in `/etc/exports`, `exportfs` activation, client visibility via `showmount`.
- Network mount actions: mount/unmount remote CIFS (SMB) and NFS shares to local mount points.
- Service controls: reload/restart `smbd`, `nmbd`, `nfs-server`.
- Dashboard UI at `/storage/shares` with share tables, user management, and connection monitoring.


## [0.1.1] - 2026-11-06

### 🚀 Changes & Improvements
- **docs: plugin ecosystem manual, per-plugin docs, and release workflows** (`71f9401`)

---

