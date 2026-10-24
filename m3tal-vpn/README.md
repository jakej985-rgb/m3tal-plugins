# M3TAL VPN Tunnel Plugin (`m3tal-vpn`)

[![Status: Planned](https://img.shields.io/badge/Status-Planned-orange.svg)](https://m3tal.io)
[![Process Identity: m3tal-vpn](https://img.shields.io/badge/Process-m3tal--vpn-brightgreen.svg)](https://m3tal.io)
[![Package: m3tal-plugin-vpn](https://img.shields.io/badge/Package-m3tal--plugin--vpn-blue.svg)](https://m3tal.io)

The **M3TAL VPN Tunnel Plugin** (`m3tal-vpn`) provides WireGuard and OpenVPN tunnel supervision (powered by Gluetun integration), strict container egress kill-switch protection, DNS leak prevention, and dynamic endpoint failover.

---

## 1. Architectural Overview & Identity

- **Canonical Process Identity**: `m3tal-vpn`
- **Package Name**: `m3tal-plugin-vpn`
- **Dashboard UI Route**: `/networking/vpn`
- **Capabilities**: `network_tunnel`, `status_checker`

---

## 2. Planned Features & Scope

- **Multi-Protocol Support**: Seamless WireGuard and OpenVPN connection management with major commercial providers and custom servers.
- **Strict Killswitch**: IP tables-based egress blocking ensuring zero network traffic leaks if the VPN tunnel drops.
- **Per-Container Routing**: Visual interface to route specific Docker containers through the VPN network namespace.
- **Public IP & Leak Testing**: Continuous validation of public egress IP, geographical location, ISP attribution, and DNS leak diagnostics.
- **Port Forwarding Telemetry**: Automatic retrieval and renewal of forwarded inbound ports from supported VPN providers.

---

## 3. IPC Methods Roadmap

| Method | Description |
|---|---|
| `m3tal.init` | Handshake verifying network tunnel backends and kernel WireGuard modules. |
| `m3tal.collect` | Collects tunnel status, public egress IP, ping latency, and transfer stats. |
| `m3tal.action` | Dispatches `reconnect`, `switch_server`, `run_leak_test`, and `toggle_killswitch`. |
| `m3tal.shutdown` | Safely disconnects tunnel or maintains persistent killswitch based on settings. |

---

## 4. Debian Installation (When Released)

```bash
sudo apt update
sudo apt install -y m3tal-plugin-vpn
```
