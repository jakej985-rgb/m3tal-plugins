# M3TAL Compose Template Database & Hub (`m3tal-stacks`)

[![Status: Planned](https://img.shields.io/badge/Status-Planned-orange.svg)](https://m3tal.io)
[![Process Identity: m3tal-stacks](https://img.shields.io/badge/Process-m3tal--stacks-brightgreen.svg)](https://m3tal.io)
[![Package: m3tal-plugin-stacks](https://img.shields.io/badge/Package-m3tal--plugin--stacks-blue.svg)](https://m3tal.io)

The **M3TAL Compose Template Database & Hub Plugin** (`m3tal-stacks`) is a curated repository and hub of verified, production-ready Docker Compose service definitions, argument presets, recommended volume mappings, and environment variable schemas.

It feeds directly into the **`m3tal-compose`** editor, allowing operators to deploy complex multi-container stacks (Nextcloud, Vaultwarden, Plex, Traefik, PostgreSQL, Redis, etc.) with a single click.

---

## 1. Architectural Overview & Identity

- **Canonical Process Identity**: `m3tal-stacks`
- **Package Name**: `m3tal-plugin-stacks`
- **Dashboard UI Route**: `/stacks` or `/containers` (Sub-Tab: `Templates / Hub`)
- **Capabilities**: `collector`, `actions`, `ui_view`
- **Integration Partner**: `m3tal-compose` (receives exported templates for customization and execution)

---

## 2. Planned Features & Scope

- **Curated Template Catalog**: Offline-first, regularly synced database of popular self-hosted and cloud-native applications.
- **Service Blueprint Schemas**: Pre-configured port bindings, persistent volume paths, restart policies, and healthcheck commands.
- **Environment Variable Preset Matrix**: Interactive forms prompting for required passwords, domain names, and database secrets.
- **1-Click Export to Editor**: Transfers any template into `m3tal-compose` with full YAML syntax highlighting for instant customization before running `compose up`.
- **Custom Stack Import / Export**: Save running Compose configurations into your own local stack library or export them as shareable recipes.

---

## 3. IPC Methods Roadmap

| Method | Description |
|---|---|
| `m3tal.init` | Handshake declaring template catalog capabilities and categories. |
| `m3tal.collect` | Emits template catalog metadata (categories, available apps, versions, icon URLs). |
| `m3tal.action` | Dispatches `get_template_yaml`, `search_templates`, and `save_custom_template`. |
| `m3tal.shutdown` | Clean shutdown. |

---

## 4. Debian Installation (When Released)

```bash
sudo apt update
sudo apt install -y m3tal-plugin-stacks
```
