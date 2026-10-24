# M3TAL Traefik Gateway Plugin (`m3tal-traefik`)

[![Status: Planned](https://img.shields.io/badge/Status-Planned-orange.svg)](https://m3tal.io)
[![Process Identity: m3tal-traefik](https://img.shields.io/badge/Process-m3tal--traefik-brightgreen.svg)](https://m3tal.io)
[![Package: m3tal-plugin-traefik](https://img.shields.io/badge/Package-m3tal--plugin--traefik-blue.svg)](https://m3tal.io)

The **M3TAL Traefik Gateway Plugin** (`m3tal-traefik`) integrates Traefik v3 reverse proxy management, automated Let's Encrypt SSL/TLS certificate lifecycle, dynamic ingress route generation, and middleware security policies directly into M3TAL Core.

---

## 1. Architectural Overview & Identity

- **Canonical Process Identity**: `m3tal-traefik`
- **Package Name**: `m3tal-plugin-traefik`
- **Dashboard UI Route**: `/networking/traefik`
- **Capabilities**: `proxy_generator`, `route_provider`

---

## 2. Planned Features & Scope

- **Dynamic Ingress Discovery**: Automatically generates Traefik HTTP/TCP/UDP routers and services from Docker labels and M3TAL plugins.
- **Automated SSL/TLS (ACME)**: Zero-config Let's Encrypt certificates using HTTP-01 and DNS-01 (Cloudflare, DuckDNS, Route53) challenges.
- **Middleware Control**: One-click attachment of rate-limiting, IP whitelisting, OAuth2 / OIDC forward auth, and security headers.
- **Access & Latency Analytics**: Real-time request rates, HTTP status codes, p95 latency breakdowns, and TLS handshake metrics.
- **Visual Topology**: Interactive flow view showing client requests passing through entrypoints, routers, middlewares, and backend services.

---

## 3. IPC Methods Roadmap

| Method | Description |
|---|---|
| `m3tal.init` | Handshake verifying Traefik binary/container availability and entrypoints. |
| `m3tal.collect` | Collects active routers, TLS cert expiration dates, entrypoints, and throughput. |
| `m3tal.action` | Dispatches `renew_certs`, `reload_config`, and `test_route`. |
| `m3tal.shutdown` | Safely disconnects telemetry listeners. |

---

## 4. Debian Installation (When Released)

```bash
sudo apt update
sudo apt install -y m3tal-plugin-traefik
```
