# M3TAL Smart Disk Telemetry Plugin (`m3tal-smart`)

[![Status: Planned](https://img.shields.io/badge/Status-Planned-orange.svg)](https://m3tal.io)
[![Process Identity: m3tal-smart](https://img.shields.io/badge/Process-m3tal--smart-brightgreen.svg)](https://m3tal.io)
[![Package: m3tal-plugin-smart](https://img.shields.io/badge/Package-m3tal--plugin--smart-blue.svg)](https://m3tal.io)

The **M3TAL Smart Disk Telemetry Plugin** (`m3tal-smart`) provides hardware-level drive health diagnostics, S.M.A.R.T. telemetry, NVMe wear life estimation, continuous temperature trend tracking, and automated predictive failure notifications for storage arrays.

---

## 1. Architectural Overview & Identity

- **Canonical Process Identity**: `m3tal-smart`
- **Package Name**: `m3tal-plugin-smart`
- **Dashboard UI Route**: `/storage/health`
- **Capabilities**: `collector`, `ui_widget`, `alerts`

---

## 2. Planned Features & Scope

- **Comprehensive Drive Inspection**: Low-level telemetry for SATA HDDs/SSDs, NVMe drives, and hardware RAID controllers via `smartctl` and `nvme-cli`.
- **NVMe Wear Life & Endurance**: Real-time percentage used, available spare, data read/written, and power cycles.
- **Automated Self-Tests**: Scheduled short and extended S.M.A.R.T. self-tests with historic result archiving.
- **Thermal Threshold Alerts**: Continuous thermal sampling with configurable alert boundaries to prevent throttling and degradation.
- **Predictive Failure Detection**: Early warning alerts based on reallocated sectors, CRC errors, and media degradation signals.

---

## 3. IPC Methods Roadmap

| Method | Description |
|---|---|
| `m3tal.init` | Handshake reporting detected storage controllers and physical drives. |
| `m3tal.collect` | Collects temperature history, SMART attributes, NVMe metrics, and test progress. |
| `m3tal.action` | Dispatches `run_self_test`, `abort_test`, and `acknowledge_alert`. |
| `m3tal.shutdown` | Graceful termination of background polling routines. |

---

## 4. Debian Installation (When Released)

```bash
sudo apt update
sudo apt install -y m3tal-plugin-smart
```
